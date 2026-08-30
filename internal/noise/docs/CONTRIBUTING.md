# Como mexer em `internal/noise/`

Onze regras. Cada uma existe porque a Fase F/G (10 lotes) ou a Fase H (7 etapas)
pagou o preço de descobri-la. Onde uma regra tem exceção, a exceção está aqui —
não fica implícita.

Leia `ARCHITECTURE.md` antes para saber *o que* cada diretório é; este documento
diz *o que fazer* quando você for escrever código.

---

## 1. Pacotes por papel técnico e por capacidade, não por camada DDD

Não existe `domain/`, `application/`, `infra/`, `presentation/` aqui e não vão
existir. Isto é um SDK de protocolo. Ver `ARCHITECTURE.md` §1.

A árvore é: `core/`, `capabilities/`, `protocol/`, `security/`, `persistence/`,
`runtime/`, `observability/`. Se o seu código não cabe em nenhum, a resposta
provável não é criar uma oitava categoria — é que você classificou errado.
Reveja com as perguntas de decisão do `ARCHITECTURE.md` §2.

## 2. Cada capacidade declara as interfaces que consome

O consumidor é dono da interface. Uma capacidade que precisa falar com o cliente
declara **a sua própria** `Transport` no seu próprio pacote, listando só os
métodos que ela usa de fato:

```
internal/noise/capabilities/notification/transport.go:27  → 2 métodos
internal/noise/capabilities/media/transport.go:44         → 3 métodos
internal/noise/capabilities/message/transport.go:111      → 36 métodos
```

O doc comment de `notification/transport.go` diz por quê, e é o teste que
importa:

> Deliberadamente não expõe nada do `*noise.Client` além disso: é o que
> permite que este pacote não importe o pacote raiz e que os testes usem um
> dublê em vez de um cliente com socket e sessão Noise.

Se você não consegue testar a sua capacidade com um dublê de ~5 métodos, a sua
`Transport` está larga demais.

## 3. `core.Client` implementa essas portas

Uma por arquivo em `core/`, com asserção em tempo de compilação:

```go
// core/notification_transport.go:25
var _ notification.Transport = notifTransport{}
```

São 15 asserções hoje (13 `Transport` + `media.HTTPTransport` + os adaptadores
de `send`/`message`). Se você adicionar um método à `Transport` de uma
capacidade, o build quebra no `var _` do adaptador correspondente — que é
exatamente onde você quer que quebre.

## 4. Capacidade nunca importa `core`

Sem exceção. É a invariante A de `DEPENDENCIES.md`, e é **travada por gate**:
`scripts/waclient-facade-check.sh` falha o build. Se você precisa de algo do
`Client`, adicione o método à `Transport` da sua capacidade — não importe `core`.

Corolário para consumidores **fora** do fork: importe
`wa-api/internal/noise` (a fachada `main.go`). Se o símbolo não está lá,
adicione o alias na fachada. Nunca contorne por baixo.

## 5. Não existe uma `Transport` gigante compartilhada

Treze interfaces estreitas, não uma com 100 métodos. A contagem atual
(`grep -rn "type Transport interface" internal/noise/capabilities/*/`):
notification 2, media 3, prekeys 4, tctoken 5, keepalive 8, group 9,
newsletter 10, user 10, appstatesync 13, pairing 14, retry 21, send 33,
message 36.

Uma interface compartilhada faria toda capacidade recompilar (e todo dublê de
teste crescer) quando qualquer outra precisasse de um método novo. Cada uma tem
a sua e paga só pelo que usa.

## 6. Estado e o lock que o protege moram no mesmo pacote

Estado compartilhado e o seu mutex viajam **juntos**, na mesma struct, no mesmo
pacote. `group.Cache`, `user.DeviceCache`, `retry.State`, `media.ConnCache` são
todos assim.

De onde veio a regra: os call sites que **acessavam campo direto** —
`send_ack.go:96-98` fazia `cli.groupCacheLock.Lock(); delete(cli.groupCache, to);
cli.groupCacheLock.Unlock()` — foram os únicos que quebraram nas extrações da
Fase F/G. Chamada de método sobrevive a uma extração; acesso direto a campo, não.
Ver `DEPENDENCIES.md` §2.

**A exceção que existiu e foi fechada**: até a Fase H, `messageSendLock` era
campo de `core.Client`, emprestado por ponteiro para `capabilities/send` via
`SendLock() *sync.Mutex`. Registrada em `HOUSEKEEP.md` **F58**, foi
deliberadamente **não corrigida** na Fase H (corrigir era mudança de design,
não de diretório) e **corrigida depois**, em commit próprio (`ef5c599`):
`capabilities/send/state.go` passou a possuir o mutex privadamente
(`sendLock`), exposto por `State.SendLock()`, como `retry`/`prekeys`/`tctoken`
já faziam. `core.Client` não tem mais o campo. Ver `LOCKS.md` e `HOUSEKEEP.md`
F58 para a evidência completa. Não há mais exceção viva à regra 6.

Corolário: quando a struct dona precisa expor o lock (porque o chamador já o
segura), exporte um `Lock()` explícito e **documente o porquê no ponto de
declaração**, como `group/cache.go:38` e `user/cache.go:40` fazem. `sync.Mutex`
não é reentrante; um segundo `Lock()` é deadlock imediato.

## 7. Import entre capacidades precisa de justificativa

Não é proibido — é caro, e precisa ser deliberado. As nove arestas que existem
hoje estão todas listadas em `DEPENDENCIES.md` §2. As duas formas de resolver:

- **Fachada** (preferida quando o chamador é o núcleo): o método de `*Client`
  sobrevive e delega. Foi assim que quatro dos seis call sites reversos de
  `group` e cinco dos sete de `user` não mudaram **uma letra**.
- **Import direto** (quando o acoplamento é semântico e irredutível): `send`
  importa `group`, `retry` e `tctoken`; `message` importa `send`, `media`,
  `user` e `appstatesync`. Não existe "enviar mensagem" que não conheça grupo,
  dispositivo e política de retry. Os lotes 8 e 9 aceitaram isso explicitamente,
  registrando que nenhum dos dois introduziu concorrência nova
  (`PATCHES.md:7951`, `:8274`).

Antes de adicionar uma aresta nova, cheque se ela mantém a ordem topológica de
`DEPENDENCIES.md` §2. Uma aresta que a inverte é um ciclo esperando o próximo
refactor.

## 8. Crie um arquivo antes de criar um subpacote

Um pacote novo custa: uma `Transport`, um adaptador em `core/`, uma entrada nos
gates (`scripts/waclient-filesize-check.sh` DIRS, `WACLIENT_TEST_PKGS` no
`Makefile`), e uma aresta nova no grafo. Um arquivo novo custa um `git add`.

O teto de 300 linhas por arquivo de produção é travado por
`scripts/waclient-filesize-check.sh` (299 arquivos em 33 diretórios hoje). Ele
existe para forçar **arquivos** novos, não pacotes novos.

## 9. `protocol/proto/` é gerado — não reestruture à mão

`protocol/proto/` (+69 subpacotes) e `protocol/binary/proto/` são gerados
(`protoc-gen-go` e `generatelegacy.sh`). Idem `core/internals.go` (757 linhas) e
o seu gerador `core/internals_generate.go`.

A única edição permitida é reescrita de import path em movimentação de
diretório. Dividir arquivo gerado não tem valor: o gerador os recria. Por isso
os dois `proto/` também ficam fora do gate de tamanho.

**Sobre o bug F29 do gerador** (`HOUSEKEEP.md`): `internals_generate.go` tem uma
lista de arquivos **hardcoded**, e rodar `go generate` hoje derruba boa parte dos
wrappers de `DangerousInternals`. É um problema **conhecido e deliberadamente
não corrigido** — a Fase H registrou o impacto em vez de consertar de graça. Não
rode `go generate` sobre `internals.go` esperando um resultado correto, e não
"aproveite" um refactor para corrigir o F29: pergunte antes.

## 10. Nunca mova código crítico de concorrência por estética

Se a razão para mover algo é "fica mais bonito lá", não mova. `LOCKS.md` é a
lista do que essa regra protege.

O caso concreto: o núcleo do client é **um pacote só** (`core/`) por causa de
`socketLock` — 6 `Lock()` e 8 `RLock()` espalhados por `client_connection.go`,
`connectionevents.go`, `client_events.go` e `request.go`. A análise do F/G lote
10 (`PATCHES.md:8334+`) concluiu que quebrar isso em dois pacotes é inviável, e
a Fase H etapa 5 respeitou a conclusão: moveu o pacote inteiro de `.` para
`./core`, sem dividi-lo.

Mudança que toca lock exige, no mínimo: `make check` verde com exit code
observado, cobertura sob `-race`, e **revisão independente** — feita por quem
**não** escreveu o código. Comparação manual do autor não conta, e `PATCHES.md`
registra literalmente quando não contou. Ver o placar em `LOCKS.md` §3.

## 11. Sem `Service`/`Repository`/`Manager` sem necessidade concreta

Não introduza uma abstração porque ela é um padrão conhecido. Introduza porque
existe um segundo implementador, ou um teste que não roda sem ela.

O fork usa funções livres e structs de estado com métodos. As interfaces que
existem — as treze `Transport` — existem por uma razão verificável: sem elas a
capacidade importaria `core`, e os testes precisariam de um cliente com socket e
sessão Noise em vez de um dublê. Essa é a barra.

---

## Antes de abrir o commit

```bash
go build ./...
bash scripts/waclient-facade-check.sh      # invariante "nada importa core"
bash scripts/waclient-filesize-check.sh    # teto de 300 linhas
LC_NUMERIC=C LC_ALL=C make check           # e observe o exit code, não a última linha
```

Se você moveu diretório, atualize também: `DIRS` em
`scripts/waclient-filesize-check.sh`, `WACLIENT_TEST_PKGS` no `Makefile`, e
regenere `cmd/logcov/testdata/eligible.golden`
(`go run ./cmd/logcov -golden > cmd/logcov/testdata/eligible.golden`).
`.logcov-exclude` já cobre `internal/noise/` por prefixo.

Achado fora do escopo da sua tarefa vai para `HOUSEKEEP.md`, não para o diff.
