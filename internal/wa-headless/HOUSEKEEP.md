# HOUSEKEEP — wa-headless (pilha de automação de browser)

Achados incidentais de `internal/wa-headless/`, a pilha que conduz o SPA real
do `web.whatsapp.com` dentro de um browser. É a segunda via de transporte do
projeto, ao lado de `internal/wa-noise/`, que fala o protocolo direto no
WebSocket com o Noise framework.

Achados das outras duas fronteiras moram noutros arquivos:

- `internal/wa-noise/HOUSEKEEP.md` — a pilha de protocolo;
- `HOUSEKEEP.md` (raiz) — a aplicação `wa-api`, o build, os gates, a
  configuração e as rotas HTTP.

A separação não é organizacional: os três têm ciclos de vida diferentes. Um
achado que atravessa a fronteira fica no arquivo de quem **causa** o problema,
com referência cruzada no outro.

Este arquivo registra o que foi **descoberto de lado**. O que decidimos
divergir da implementação de referência (`whatsapp-web.js`) vai para
`PATCHES.md`, neste mesmo diretório, e decisão que vale para o projeto inteiro
vai para `docs/adr/`.

O formato de cada entrada, e a política anti-regressão que rege a passagem
para "corrigido", estão em `CLAUDE.md` / `AGENTS.md`.

## Convenção de status

Toda entrada termina com um `**Status**:` cujo **veredito vem em negrito**,
para que uma varredura mecânica o encontre. Ele pode estar no início da linha
ou como item de lista (`- **Status**: ...`) — os dois layouts convivem nos
HOUSEKEEP do repositório, e **uma varredura tem de aceitar os dois**:

- `**Status**: **corrigido**` — com os testes que o travam e o controle
  negativo executado (ver a política anti-regressão em `CLAUDE.md`).
- `**Status**: **não corrigido**` — seguido do motivo.
- `**Status**: **fechado — não corrigir**` — decisão registrada, não pendência.
- `**Status**: **parcialmente corrigido**` — com o que ficou aberto e por quê.

O formato importa, e a varredura também. A lição vem do `HOUSEKEEP.md` da raiz,
onde em 2026-08-08 três varreduras seguidas erraram: uma leu cinco entradas
como "sem status" porque o veredito estava em texto simples, outra perdeu
quatro porque o `**Status**` era item de lista. Em todos os casos **o documento
estava certo e o método errado**, e entradas íntegras quase foram "corrigidas".

Entradas com MAIS de um `**Status**` são legítimas: o achado tem sub-itens com
desfechos diferentes. O veredito que vale é o do sub-item; não existe um status
único para elas.

Seções que são **nota** e não achado — evidência nova para entradas existentes,
observação de acompanhamento — não levam status. Dê a elas um título que diga
isso ("Nota sobre…", "Evidências novas…"), para que a varredura as distinga de
um achado que esqueceu o status.

Referências entre entradas são por **título**, nunca por número de linha.
Referência a achado de outro HOUSEKEEP diz de qual arquivo ele é — o `F` de um
arquivo não é o `F` do outro.

## Numeração

As entradas deste arquivo são numeradas a partir de **H1**, e não da série `F`
usada pela raiz e pelo `wa-noise`. Os dois prefixos convivem no repositório sem
colidir, e um `H` no texto diz de imediato de qual arquivo o achado é.

---

## H1 — o texto de erro do `OpLog` não tem política de redação

**Data**: 2026-08-11 · **Contexto**: CAP-02, porte do `OpLog` do estudo para
`internal/wa-headless/observability/`.

**Onde**: `internal/wa-headless/observability/oplog.go`, campo `OpRecord.Err` e
a constante `maxErrTextLen`.

**Problema**: o `OpRecord` foi desenhado para não carregar conteúdo de página —
classe, rótulo, tempos e desfecho bastam para localizar onde uma execução
parou. O campo `Err` fura isso e não é evitável no nível em que está: um erro
de driver pode citar o que a página lançou, e o que a página lança pode citar o
que ela estava segurando. Truncar em 200 caracteres limita o RAIO, não o
conteúdo — 200 caracteres de dado de terceiro continuam sendo dado de terceiro
num log.

Nada disso vaza hoje: o `OpLog` não está ligado ao zerolog da aplicação e
nenhum caminho de produção o consome. O achado é o **débito**, não um
vazamento.

**Correção sugerida**: decidir a política no CAP-04, quando o liveness passar a
gerar erros de página em volume, e antes de qualquer ponte para o log
estruturado. Três caminhos, em ordem de preferência: (a) classificar o erro
numa taxonomia fechada e registrar só a classe, mantendo o texto apenas sob
`WA_HEADLESS_OP_TRACE`; (b) redator injetável no `OpLog`, com o padrão sendo
redigir; (c) manter o texto e travar por catraca de PII na saída. O item 5 da
Definition of Done (`HANDOFF-INICIATIVA.md`) exige zero PII em log verificado
por catraca, então (c) sozinho não fecha.

**Status**: **não corrigido** — é débito consciente do CAP-02, com o raio já
limitado por `maxErrTextLen` e travado por `TestAddTruncatesErrorText`. A
decisão exige saber a forma dos erros reais, que o CAP-04 vai produzir;
escolher agora seria projetar sem medida.

## H2 — a fundação subiu o Go do módulo inteiro para 1.26

**Data**: 2026-08-11 · **Contexto**: CAP-02, commit `wa-headless: add target
priming`.

**Onde**: `go.mod` (`go 1.25.0` → `go 1.26`) e `Dockerfile:1`
(`golang:1.25.12-bookworm` → `golang:1.26-bookworm`).

**Problema**: o `HANDOFF-INICIATIVA.md` §11 prometia raio de alcance **zero**
para o CAP-02 — "nada consome `internal/wa-headless` hoje". A promessa não
sobreviveu ao `PrimeTab`: ele é `chromedp.Run`, o `chromedp` a partir da
v0.15.0 exige Go 1.26, e o `go get` reescreveu a diretiva `go` do módulo
`wa-api` inteiro. Com o `Dockerfile` ainda em `golang:1.25.12` o build de
produção quebraria, então ele subiu junto.

A alternativa era fixar `chromedp v0.14.2`, a última que aceita Go 1.24+, ao
custo de rodar em produção uma versão **diferente** da medida: `Cancel` (§22 da
4C), `Poll` com relógio na página (ARMADILHAS 19) e rAF em aba de fundo (4C)
são exatamente onde o estudo mediu comportamento específico de versão.

**A consequência que a decisão não previu, e que foi MEDIDA depois**: o bump
**quebra o gate de lint**, local e no CI.

```
$ make lint
Error: can't load config: the Go language version (go1.25) used to build
golangci-lint is lower than the targeted Go version (1.26)
FALHA: nao consegui extrair a contagem de issues de .lint.out.
make: *** [lint] Error 1
```

O `.github/workflows/ci.yml:37` fixa `v2.5.0` (compilada com go1.25.1) e o
`setup-go` usa `go-version-file: go.mod`, ou seja **1.26** — o CI vai reproduzir
exatamente este erro.

Três tentativas de conserto, todas medidas e todas descartadas:

1. **Recompilar a v2.5.0 com o Go local** (`go install …@v2.5.0`, inclusive com
   `GOTOOLCHAIN=local`): sai `built with go1.24.2`, porque o `go.mod` da própria
   ferramenta fixa 1.24.2. Falha igual, um número abaixo.
2. **`run.go: "1.25"` no `.golangci.yml`**, para analisar num nível de
   linguagem atrás do `go.mod`. O erro de config some — prova que o botão
   funciona — e o type checker então **panica** dentro do pacote:
   `panic: package requires newer Go version go1.26 (application built with
   go1.25)`. A causa é a DEPENDÊNCIA: o `chromedp/cdproto` declara `go 1.26`, e
   o `go/types` embutido na v2.5.0 não lê o pacote. Nenhuma configuração
   resolve. Alteração revertida.
3. **Subir para `v2.12.2`** (release mais nova, 2026-05-06): o instalador
   oficial **reprovou o checksum** do tarball darwin/arm64
   (`c8debe3b…` vs `a9c54498…`). Não contornado — e não deve ser.

Conclusão medida: **a v2.5.0 não consegue lintar este módulo enquanto o
`chromedp v0.16.0` for dependência, independente de configuração.** Restam
duas saídas, e as duas são decisão do usuário: subir o linter (e o
`.golangci-baseline` junto, no mesmo PR, como o comentário do CI exige) ou
voltar ao `chromedp v0.14.2` e ao Go 1.25.

**Nota de atribuição**: o `make coverage-gate` também falha aqui
(`go: no such tool "covdata"`), e isso **não** é efeito do bump — nem
`go1.25.12` nem `go1.26.0` trazem `covdata` em `pkg/tool` quando o toolchain
vem do module cache. O CI instala distribuição completa e não vê isso. Era
falha de ambiente local, e já existia.

**Correção sugerida**: decidir entre as duas saídas acima. Verificar também o
build da imagem: `golang:1.26-bookworm` não foi construído nesta sessão, só
declarado.

**Status**: ~~**não corrigido** — o gate de lint está quebrado nesta branch e o
CI vai reprovar. `build`, `vet`, `test` (suíte inteira, com `-race`),
`log-coverage-gate`, `waclient-facade` e `waclient-filesize` passam.~~
**CORRIGIDO** — o lint em `e5ee22e` + `2aa304d` (FASE B0 no `EXECUCAO.md`), e a
imagem em 2026-08-12 (LOOP H2.1), abaixo.

### O build de produção, CONSTRUÍDO em vez de declarado — LOOP H2.1 (2026-08-12)

A pendência que restava desta entrada era literal: *"`golang:1.26-bookworm` não
foi construído nesta sessão, só declarado."* Construído agora, ponta a ponta,
sem alterar nenhum arquivo de build:

```
docker build -t wa-api:h2-check .
EXIT=0
imagem   wa-api:h2-check   911MB
binário  /app/wa-api       43.048.079 bytes, -rwxr-xr-x waapi:waapi
```

A tag resolve para uma toolchain que satisfaz a diretiva do `go.mod`, o que era
a dúvida real — `1.26-bookworm` é uma tag móvel, e o módulo exige 1.26 por causa
do `chromedp v0.16.0`:

```
docker run --rm golang:1.26-bookworm go version
go version go1.26.5 linux/arm64
```

Então o `CGO_ENABLED=1 go build ./cmd/core` do estágio builder compila de fato
com o Go que a branch passou a exigir, e o estágio final (`debian:bookworm-slim`)
recebe um binário que existe e tem bit de execução.

**O que isto NÃO prova**, e vale dizer porque o achado nasceu de uma declaração
tomada por verificação: que o binário SIRVA — não foi executado contra
dependências, só inspecionado na imagem. E a construção foi em **arm64**, que é
a mesma limitação de arquitetura já registrada no `HANDOFF-INICIATIVA.md` §7.
`amd64` continua não medido.

## H3 — dois documentos do `disparazaap` afirmam o que a Fase 6 refutou

**Data**: 2026-08-11 · **Contexto**: CAP-02; a pendência vem da Fase 6 e
continua sem dono.

**Onde**: no repositório `disparazaap`, branch `features/macbook-lucas`:
`docs/architecture/wa-headless-chromium-cdp.md` §2.1.1 e
`docs/adr/0038-wa-headless-absorve-wa-worker-servico-go-unico.md`.

**Problema**: o §2.1.1 afirma que "chegar a 2 renderers seria da ordem de
100–200 MB por sessão". O nó H1 da Fase 6 refutou a segunda metade por medida
direta (`RELATORIO-FASE-6.md` §2): os 2 renderers do boot existem antes de
qualquer navegação, não são os `browser_ui` (teste causal, N=3, unânime), e
nenhuma das cinco flags candidatas reduz a contagem. A aplicação inteira soma
**+1** renderer. Separadamente, a ADR-0038 ficou **deslocada** do escopo desta
iniciativa quando o objetivo foi reescrito para "motor equivalente ao
`whatsapp-web.js` atrás da fenda que já existe".

**Correção sugerida**: propor as duas correções pelo `HOUSEKEEP.md` do
`disparazaap`, **não editar de aqui** — aquele repositório é da sessão C0/C1, e
escrever nele desta sessão é o BLOCKER de trabalho paralelo que já foi furado
uma vez.

**Status**: **não corrigido** — pendência de outra sessão, registrada aqui para
não sumir. Referência cruzada: `scripts/chromium-study/RELATORIO-FASE-6.md` §8.

## H4 — o formato do `SingletonLock` é premissa, não medida

**Data**: 2026-08-11 · **Contexto**: CAP-03 LOOP 03.2, `ReclaimProfile`.

**Onde**: `internal/wa-headless/engine/profile.go`, `readLockHolder`.

**Problema**: a decisão de apagar ou recusar depende de ler o `SingletonLock`
como **symlink cujo alvo é `<hostname>-<pid>`**. Isso vem da convenção do
`chrome/browser/process_singleton_posix.cc`, e o dublê do teste imita essa
regra — mas **nenhum perfil real foi inspecionado nesta sessão** para confirmar
o formato na versão 151 que o projeto usa.

Se o formato divergir, o `readLockHolder` cai no ramo "ilegível" e o reclaim
apaga sem provar nada. Isso não é catastrófico — é exatamente o comportamento
do estudo, que rodou assim por todas as fases — mas a guarda que a H4 protege
deixa de guardar, e em silêncio.

**Nota de método (2026-08-12)**: o plano original — "matar o Chromium e olhar o
perfil" — **não produz nada**. O perfil pareado foi aberto e fechado três vezes
naquela data (M3 do `EVIDENCIA-SPA.md`) e não havia `SingletonLock` nem antes
nem depois: o desligamento limpo o remove. Medir o FORMATO exige inspecionar
com o Chromium **em execução**.

---

### MEDIDO em 2026-08-12 (LOOP 03.9) — a premissa está CORRETA

**Instrumento**: `TestRealSPASingletonLockFormat`, em
`internal/wa-headless/realspa_test.go`. Sobe o perfil com o **`engine.Launcher`
de produção** (não um `exec` à mão: o formato observado tem de ser o formato que
o parser de produção vai enfrentar), inspeciona com `os.Lstat` + `os.Readlink`
com o browser **VIVO**, e depois entrega a string medida ao
`engine.ReclaimProfile` — o parser de produção, não um dublê.

**Ambiente**: Google Chrome **151.0.7922.109**, darwin/arm64, `--headless=new`.
Perfil **pareado** (`scripts/chromium-study/wa-session/profile`), via
`WA_HEADLESS_PROFILE_DIR`.

Saída crua da corrida, **com o hostname redigido para `<HOST>`** (a máquina
carrega o nome de uma pessoa, e PII é BLOCKER do repositório). Nada mais foi
alterado:

```
BASELINE profile_files=2781 singleton_lock_present=false
profile_dir=.../scripts/chromium-study/wa-session/profile overridden=true
MEASURED lstat: mode=Lrwxr-xr-x symlink=true
MEASURED readlink target="<HOST>-63733"
MEASURED os.Hostname()="<HOST>" launched_pid=63733
PRODUCTION PARSER (real probe): lock_holder="<HOST>-63733" reason="" removed=[] \
  err=profile is held by a live browser: pid 63733 on "<HOST>" is still running
PRODUCTION PARSER (holder declared dead): reason="holder pid 63733 on this host is gone" \
  removed=[SingletonLock SingletonCookie SingletonSocket] err=<nil>
stopped_via=browser.close
HYGIENE profile_files before=2781 after=2781 delta=+0
AFTER CLEAN STOP singleton_lock_present=false
```

O `<HOST>` redigido tem forma que importa para o parser e por isso fica
registrada: contém **hífens** e um **ponto**, e termina em `.local`. Ou seja, a
regra "o pid vem depois do ÚLTIMO hífen" foi exercitada contra um hostname
hifenizado de verdade, não contra um nome simples.

**Veredito: a premissa está CONFIRMADA, ponto por ponto.**

| o que `readLockHolder` assume | o que foi medido |
|---|---|
| o arquivo existe enquanto o browser vive | existe (`Lstat` OK com o browser vivo) |
| é um **symlink** | `mode=Lrwxr-xr-x`, `symlink=true` |
| o alvo é `<hostname>-<pid>` | `"<HOST>-63733"` |
| `hostname` é o mesmo que `os.Hostname()` devolve | idêntico, inclusive o sufixo `.local` |
| `pid` é o do processo browser | `63733` = `launched_pid` do `Launcher` |

A verificação que decide não é a inspeção crua — é a **segunda metade**: a
string medida entregue ao `ReclaimProfile` real. Com a sonda de liveness de
produção ele devolve `ErrProfileHeldByLiveBrowser` e **não apaga nada**; esse
erro tipado só é alcançável DEPOIS de o alvo ter sido decomposto em host igual
ao nosso e pid vivo. Com o detentor declarado morto, ele apaga os três. Se o
formato divergisse, o ramo "ilegível" devolveria erro nulo e apagaria — que é
exatamente o silêncio que esta H4 existia para descartar.

**Controle negativo: NÃO se aplica, e dizer isso é a resposta honesta.** Um
controle negativo prova que um teste MORDE quando o defeito volta. Aqui não há
defeito, não há correção e não há dublê novo: nada foi alterado em
`profile.go` nem em `profile_test.go`. Inventar uma mutação cerimonial travaria
uma regra que já está travada pelos testes do `profile_test.go` — o que a
medição acrescenta é que o dublê daquele arquivo **não é mais permissivo que a
produção**, porque a produção agora foi vista escrevendo exatamente o que o
dublê escreve.

**Limite declarado, e ele é real**: isto foi medido em **macOS**, e a invariante
que a guarda protege é "**Reclaim de `Singleton` no boot é obrigatório em
contêiner**" (`HANDOFF-INICIATIVA.md` §6, numerada **13**) — ou seja, Linux. Os
dois lados usam `gethostname(2)` (`base::GetHostName` no Chromium,
`os.Hostname` no Go), então a igualdade de host deve valer também lá, mas
**deve** não é medido. Para o contêiner o formato continua **DESCONHECIDO**, e
está nomeado como tal em vez de assumido.

> **Divergência de numeração, sinalizada e NÃO corrigida**. A invariante é, pelo
> TEXTO, *"Reclaim de `Singleton` no boot é obrigatório em contêiner"*, que o
> `HANDOFF-INICIATIVA.md:394` — fonte de verdade — numera **13**. Chamam-na de
> **15** dois pontos do código, e o segundo só apareceu na avaliação adversarial
> do LOOP 03.9:
>
> | onde | texto literal |
> |---|---|
> | `engine/profile.go:8` | *"it is **invariant 15** of the initiative"* |
> | `engine/profile_test.go:48` | *"The happy path of **invariant 15**"* ← quarta ocorrência do deslocamento, não sinalizada antes |
>
> O mesmo deslocamento atinge OUTRAS invariantes, e essas já estavam
> sinalizadas: `PARIDADE-WWEBJS.md:88` chama `livenessCheck` de *"invariante
> 11"* quando o `HANDOFF` a numera **10**, e `HOUSEKEEP.md:580` fala em
> *"invariante 14"* para zero-PII, numerada **12**. A nota do
> `EXECUCAO.md` (auditoria da CAP-03) sinaliza esse conjunto — "invariantes 11,
> 14 e 15".
>
> **Nada foi renumerado**, aqui nem em lugar nenhum: o deslocamento está em
> triagem HUMANA, e renumerar por conta própria trocaria uma divergência
> conhecida por uma silenciosa. Cite pelo TEXTO, que não é ambíguo, e pelo
> número **13** do `HANDOFF`.

**Status**: **CONFIRMADO — H4 fechada** para Chrome 151 em macOS. Travada por
`TestRealSPASingletonLockFormat` (gated por `WA_HEADLESS_REAL_SPA`) e pelos
testes de `engine/profile_test.go`, que continuam verdes e cujo dublê a medição
validou. Fica aberto apenas o caso **contêiner/Linux**, nomeado acima como
desconhecido.

## H5 — o teste do renderer travado vaza o browser, e ninguém percebe

**Data**: 2026-08-12 · **Contexto**: achado de lado durante o LOOP B1.4, ao
inventariar processos antes de abrir o perfil pareado. Não faz parte daquele
escopo.

**Onde**: `internal/wa-headless/integration_test.go:197-202`, o `defer` de
`TestBrowserChainReportsAWedgedPageAsUnresponsive`.

```go
defer func() {
    // A wedged renderer will not honour Browser.close, so the dirty path is
    // the expected outcome here. What matters is that it terminates.
    via := engine.CleanStop(context.Background(), runner, browser)
    t.Logf("stopped_via=%s", via)
}()
```

**Problema**: o comentário ENUNCIA o requisito — *"what matters is that it
terminates"* — e o código só **registra** o desfecho. Nada afirma que o
processo morreu. Se o `CleanStop` voltar sem matar o browser, o teste passa
verde e deixa o Chromium para trás.

Foi o que aconteceu, e foi medido:

```
6 processos Chromium órfãos, --user-data-dir=/var/folders/.../
  TestBrowserChainReportsAWedgedPageAsUnresponsive166315251/001
idade: 1 dia 6h    CPU acumulada: 20min43s no processo principal
diretório de perfil temporário sobrevivente: 81 MB
```

Honestidade sobre o alcance: **uma** ocorrência, de uma execução anterior a
esta sessão. O `go test -race ./internal/wa-headless/...` rodado hoje **não**
vazou. Então não é "todo run" — pode depender de a execução ser interrompida.
A causa NÃO foi determinada; isto é achado, não diagnóstico.

**Por que isto é mais que higiene de teste**: o comentário afirma, sem medida
ao lado, que *um renderer travado não honra `Browser.close`*. Se isso for
verdade — e o vazamento é compatível com isso —, então o módulo **não tem
caminho de desligamento provado para o estado UNRESPONSIVE**, que é exatamente
o estado que a CAP-04 existe para detectar. A invariante 2 proíbe sinal em
perfil com credencial (o `SIGTERM` deu logout na 4ª e na 6ª iteração), e um
browser travado segurando credencial é um dilema que a política ainda não
resolveu. Atinge o item 11 da Definition of Done (nenhum vazamento conhecido
de Chromium).

**Correção sugerida**, em ordem:

1. **Travar o requisito que o comentário já declara**: depois do `CleanStop`,
   afirmar a morte do processo com `WaitExit`/`ProcessAlive` (existem desde os
   loops 03.4/03.5 e não são usados aqui). Isto transforma o vazamento em
   falha de teste — hoje ele é invisível.
2. **Medir a afirmação**: `Browser.close` realmente não é honrado por um
   renderer travado, ou o `CleanStop` desiste antes? São causas diferentes com
   correções diferentes, e a distinção não foi medida.
3. **Decidir a política de escalada** para UNRESPONSIVE com credencial, e
   registrá-la — é decisão de arquitetura, não de teste. Aplicar a Regra 4 do
   `CLAUDE.md`: o conserto também é um mecanismo, e matar por sinal é
   exatamente o que a invariante 2 proíbe.

**Status**: ~~**não corrigido** — fora do escopo do LOOP B1.4, e o item 3 é
decisão humana de arquitetura. Os processos órfãos e o diretório de 81 MB
foram removidos em 2026-08-12 com autorização do usuário; o DEFEITO que os
produziu continua no lugar.~~
**PARCIALMENTE CORRIGIDO em 2026-08-12 (LOOP H5.1)**: itens 1 e 2 fechados, item
3 aberto e com a pergunta ESTREITADA pela medição. Ver abaixo.

### Resolução dos itens 1 e 2 — LOOP H5.1 (2026-08-12)

**Item 2 primeiro, porque ele desmonta a premissa do item 3.** A afirmação do
comentário — *"a wedged renderer will not honour `Browser.close`"* — é **FALSA**,
medida em três corridas consecutivas:

```
corrida 1   stopped_via=browser.close   (36,1s)
corrida 2   stopped_via=browser.close   (36,1s)
corrida 3   stopped_via=browser.close   (37,9s)
```

O mecanismo deixa de ser surpreendente assim que enunciado: o travamento é um
`for(;;)` na thread principal do **RENDERER**, e o `Browser.close` é servido pelo
processo **BROWSER**, que é outro processo e não está girando. Uma aba pendurada
não deixa o browser surdo.

Isso significa que **o módulo TEM caminho de desligamento provado para o estado
UNRESPONSIVE** — que era exatamente a dúvida que esta entrada levantava sobre a
CAP-04. A hipótese alternativa ("o `CleanStop` desiste antes do prazo") fica
descartada pelo mesmo dado: ele não desiste, ele conclui pelo caminho limpo.

**Item 1: o requisito que o comentário só declarava agora é asserção.** O
`defer` captura o PID antes de parar e, depois do `CleanStop`, exige as duas
coisas: `via.Clean()` e `!engine.ProcessAlive(pid)`.

**Controle negativo EXECUTADO** — `CleanStop` devolve `StopViaNoop` sem parar
nada:

```
--- FAIL: TestBrowserChainReportsAWedgedPageAsUnresponsive (36.01s)
    stopped_via=noop pid=56700
    pid 56700 is still alive after CleanStop returned noop: the test leaked the
    browser it owns, which is the H5 defect
```

Repare em qual asserção mordeu: `StopViaNoop.Clean()` devolve **true**, então a
guarda de `via.Clean()` passou e quem pegou foi o `ProcessAlive`. As duas não são
redundantes — a que faltava era justamente a que o comentário prometia.

A mutação deixou **7 processos órfãos**, removidos em seguida. A mesma ordem de
grandeza dos 6 medidos no achado original, o que corrobora que o vazamento
relatado é este caminho e não outro.

### O que continua aberto — item 3, com a pergunta estreitada

A medição **não** fecha o item 3; ela reduz o que ele abrange. O dilema já não é
"como desligar um renderer travado" — isso sai limpo. É o caso RESIDUAL: quando
o próprio processo browser não honra o `Browser.close`, o `CleanStop` cai no
`SignalStop`, e um sinal contra perfil com credencial é o que a **invariante 2**
proíbe e o que deu logout na 4ª e na 6ª iteração da fase 4C.

As três saídas, e por que nenhuma é escolha de mesa:

- **A · manter o sinal**: garante que nada vaza, ao custo de poder corromper a
  sessão pareada. É o comportamento de hoje, e ele contradiz a invariante 2.
- **B · nunca sinalizar perfil com credencial**: honra a invariante, e aceita
  deixar processo e perfil para trás com causa classificada. Troca corrupção por
  vazamento de recurso.
- **C · escalada**: reabrir conexão e repetir o `close` N vezes antes de
  qualquer sinal, registrando cada tentativa.

### O caso residual, agora MEDIDO (2026-08-12)

O passo (a) que esta entrada listava como pendente foi executado: `SIGSTOP` no
processo browser produz exatamente um browser que não pode responder ao
`Browser.close`, e não escreve nada no perfil. Perfil temporário, nunca o
pareado. Três corridas:

```
stopped_via=DIRTY_signal_close_refused   30,006s   processo vivo depois: NÃO
stopped_via=DIRTY_signal_close_refused   30,002s   processo vivo depois: NÃO
stopped_via=DIRTY_signal_close_refused   30,003s   processo vivo depois: NÃO
```

**A escalada CONVERGE.** Os 30 s são os três orçamentos em sequência (close 10 s
+ wait-exit 10 s + `signalGrace` 10 s), e o `SIGKILL` no GRUPO derruba até um
processo parado por `SIGSTOP` — sinal que não pode ser bloqueado nem ignorado.
O desfecho ainda se anuncia honestamente: `Clean()` devolve **false**, então o
chamador consegue agir sobre a diferença.

Fidelidade do instrumento, dita antes que alguém a assuma: `SIGSTOP` modela com
fidelidade **"CDP sem resposta possível"**. A palavra "recusado" estava errada e
foi corrigida pela validação independente: um processo parado mantém o socket de
escuta aberto, então a conexão **pendura** em vez de ser recusada — que é
justamente por que as três corridas gastaram o orçamento inteiro de 10 s no
`close`. Pior caso dos dois jeitos, então a conclusão não muda; a frase mudou. Ele **não** modela um
browser em deadlock mas escalonável, que ainda serviria `SIGTERM` — nesse
aspecto o `SIGSTOP` é MAIS severo que a realidade, o que torna a convergência
medida um limite inferior, não otimismo.

### O que a medição faz com a decisão

Ela desfaz a simetria que o trade-off aparentava ter. A opção **B** ("nunca
sinalizar perfil com credencial") não custa apenas vazar um processo:
`reclaimVerdict` recusa com `ErrProfileHeldByLiveBrowser` enquanto o pid do
detentor estiver vivo (`engine/profile.go`), de propósito — apagar o
`SingletonLock` de um browser vivo põe dois browsers num perfil, que é como uma
sessão pareada morre de verdade. Então B deixa o perfil **irrecuperável naquele
host** enquanto o processo travado existir: ela protegeria a sessão de
corrupção bloqueando permanentemente a própria sessão.

Isso NÃO decide a questão, e não a decidi. O custo de A continua real e medido
(fase 4C: logout na 4ª e na 6ª iteração), a invariante 2 continua proibindo o
sinal, e escolher entre "arriscar a sessão para recuperar o perfil" e "preservar
a sessão perdendo o perfil" é trade-off de produto. O que mudou é que agora
existe número dos dois lados em vez de um lado imaginado.

**Status do item 3**: ~~**aberto — decisão do usuário**~~ · **DECIDIDO em
2026-08-12: opção A, pelo usuário.** O sinal fica, e o preço de mantê-lo é pago
por mecanismo — ver a seção de implementação no fim desta entrada. As três
opções foram medidas e não mais especuladas. A opção C (escalar o `close` N vezes antes de
qualquer sinal) ficou sem justificativa aparente para este modo de falha: contra
um browser inalcançável, cada tentativa extra custa 10 s e não muda o desfecho.
Ela só ganharia sentido se existir um travamento TRANSITÓRIO, que não foi
observado.

---

## H6 — `spa.Probe` pode capturar 200 caracteres de texto da página quando a estrutura não casa

**Data / contexto**: 2026-08-12, LOOP 04.3A (liveness contra sessão que perdeu
o servidor). Achado de lado: a sonda de corte de rede chama `spa.Probe` a cada
tick contra a conta real, e ler o caminho para confirmar que isso é seguro
mostrou que ele é seguro **por uma guarda só**.

**Onde**: `internal/wa-headless/spa/probe.go:83-87` e `:116-123`.

```go
const textScript = `JSON.stringify((() => {
	if (document.querySelector('#pane-side')) return '';
	const t = document.body ? document.body.innerText : '';
	return t.slice(0, 200);
})())`
```

**Problema**: numa página classificada `OTHER` pela estrutura, o `Probe` faz a
segunda avaliação e traz até 200 caracteres de `body.innerText`. Contra
`web.whatsapp.com` com sessão pareada, esse texto é a lista de conversas —
nomes de contato e prévias de mensagem. A única coisa que impede isso é
`#pane-side` estar presente.

Ou seja: **a proteção de PII depende do mesmo seletor cuja fragilidade este
módulo inteiro assume**. No dia em que a Meta renomear `#pane-side`, o efeito
não é só "classifica errado" — é que toda sonda passa a puxar texto de conversa
para dentro do processo. O M4 mediu 180 amostras (90 severadas + 90 de
controle) em que o painel se manteve, então nada foi capturado nesta corrida;
o risco é o próximo build da Meta, não este.

Duas condições agravantes, ambas já verdadeiras hoje:

1. O caminho já é exercitado contra a conta real, não só contra dublês
   (`realspa_test.go`, sondas de observação e agora a do 04.3A).
2. O texto capturado vai para `PageSnapshot.TextSample`, que é campo
   **exportado** — qualquer chamador futuro pode logá-lo sem saber o que ele
   carrega. Nenhum loga hoje; a invariante 14 é mantida por convenção.

**Evidência**: leitura do caminho, não medição — o estado que dispara a captura
não foi produzido de propósito contra a conta real, e produzi-lo exigiria
quebrar o seletor no navegador.

**Correção sugerida**, em ordem de custo:

1. **Não confiar num seletor só para decidir se pode ler texto.** A guarda pode
   exigir a AUSÊNCIA de qualquer sinal de sessão pareada — hoje a identidade do
   dono (`WAWebUserPrefsMeUser`, M3.2) é o candidato barato: se o perfil está
   pareado, não leia texto. Nas telas que precisam de texto (conflito, "atualize
   o Chrome") a identidade também é lida do store, então a regra precisa ser
   medida antes de ser adotada — é achado, não diagnóstico.
2. **Redigir na fronteira**: `TextSample` deixa de ser `string` livre e passa a
   carregar só o que casou com um conjunto fechado de padrões conhecidos
   (conflito, atualização de browser). Fecha a classe inteira em vez de um caso.
3. **Teste que trave**: página com `#pane-side` renomeado E texto de conversa no
   corpo; asserção de que o snapshot volta com `TextSample` vazio. Com controle
   negativo: reintroduzir a leitura e ver o teste falhar.

**Status**: ~~**não corrigido** — fora do escopo do LOOP 04.3A, que é medição de
liveness, e mexer na guarda de PII sem medir qual sinal a substitui trocaria
uma dependência frágil por outra. Registrado para decisão do usuário.~~
**CORRIGIDO em 2026-08-12 (LOOP H6.1)** — ver a seção de resolução no fim desta
entrada. Testes que o travam: `TestProbeCarriesNoPageTextWhenTheSelectorIsRenamed`,
`TestProbeDiscardsMarkersItNeverAskedAbout`, `TestMarkerScriptNeverReturnsPageText`
(`spa/probe_test.go`) e `TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed`
(`integration_test.go`, contra Chrome real).

### Agravante removido em 2026-08-12 (LOOP 04.3B): o seletor estava escrito TRÊS vezes

O `#pane-side` aparecia como literal solto em `spa/probe.go:84` (o `textScript`
citado acima) e em `spa/liveness.go:60` (o `livenessScript`), enquanto
`spa/probe.go:50` já declarava `paneSideSelector` e o `structureScript` o usava.
Três cópias, uma nomeada.

Isso **não é cosmético para este achado**: a H6 é exatamente sobre a Meta
renomear esse seletor, e uma renomeação que atualizasse a constante e passasse
por cima das duas cópias produziria o cenário da H6 **por nossa própria mão** —
o classificador perguntando pelo nome novo, a guarda de PII e a sonda de
liveness pelo velho, e a captura de texto de conversa liberada sem que a Meta
tivesse mudado nada. É também o que a `CLAUDE.md` proíbe: *"literal repetido em
dois lugares é o mesmo bug esperando divergir."*

As duas cópias passaram a referenciar `paneSideSelector`. A superfície da H6
continua a mesma — uma guarda só —, mas agora renomear o seletor é **uma**
edição em vez de três lugares onde esquecer um.

### Resolução — LOOP H6.1 (2026-08-12): a guarda deixou de ser guarda e virou mecanismo

**O que foi adotado**: o texto da página **não atravessa mais a fronteira**. O
segundo `Evaluate` deixou de devolver 200 caracteres de `innerText` e passou a
receber um conjunto FECHADO das nossas próprias strings, perguntando quais delas
a página viu. A busca de substring acontece dentro da página; o que volta é um
subconjunto do que foi enviado, e `knownMarkers` descarta na chegada qualquer
coisa fora do conjunto.

`PageSnapshot.TextSample string` foi **removido** e substituído por
`Markers []string`. Isso encerra também o agravante 2 desta entrada: não existe
mais campo exportado de texto livre para um chamador futuro logar sem saber o
que carrega.

O significado ficou em Go. Qual combinação de marcadores é conflito e qual é
"atualize o Chrome" continua em `classifyByMarkers`, testável por unidade — só a
busca desceu para a página.

**Por que NÃO foi a guarda por identidade** (correção 1 desta entrada, que era o
caminho previsto). A tela de conflito só existe num perfil **pareado** — é isso
que a torna conflito — e o M3.3 mediu que a identidade é PERSISTIDA e reaparece
em T+0,01s, antes do socket abrir. Gatear a leitura de texto por "identidade
ausente" fecharia a leitura exatamente na tela que a leitura existe para
reconhecer, e a `ClassSessionConflict` é terminal: o chamador age sobre ela.
Seria trocar uma dependência frágil por uma perda funcional.

Isto é **inferência, não medição** — não produzi uma tela de conflito contra a
conta real para ler a identidade nela, porque a correção adotada torna a
pergunta irrelevante: sem texto cru atravessando, não há o que gatear. O
falsificador fica registrado: se algum dia se medir identidade AUSENTE numa tela
de conflito, esta justificativa cai (mas a correção adotada continua de pé, por
outro motivo).

**A ordem de preferência desta entrada estava, portanto, invertida.** A correção
2 ("redigir na fronteira") não era mais cara que a 1 — é a mais barata das duas
em consequência, porque não depende de medir sinal nenhum.

**O que a correção NÃO cobre.** A guarda de `#pane-side` continua no script,
rebaixada ao que de fato protege agora: CORREÇÃO, não privacidade. Uma lista de
conversas varrida por estas frases pode acertar uma por coincidência — alguém
escrevendo "outra aba" numa mensagem viraria `SESSION_CONFLICT` numa sessão
saudável. Essa guarda é exercitada pelo dublê e por asserção estática sobre o
script; a corrida real (painel aparecendo ENTRE as duas avaliações) não foi
reproduzida contra navegador, e continua sendo o buraco de cobertura conhecido.

**Controles negativos EXECUTADOS** (quatro mutações, todas revertidas):

**(NC-1) o script volta a fatiar o texto** — `markerScript` devolve
`[t.slice(0, 200)]`:

```
--- FAIL: TestProbeCarriesNoPageTextWhenTheSelectorIsRenamed
    the marker probe never ran, so this test did not exercise H6
--- FAIL: TestMarkerScriptNeverReturnsPageText
    the marker script slices the body text; that is the H6 defect returning
    the marker script does not filter the marker list; it is returning something else
--- FAIL: TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed
    renamed selector, conflict wording: classified as "OTHER", want "SESSION_CONFLICT"
```

Repare no que este controle revelou: a PII **não vazou**, porque `knownMarkers`
a barrou sozinha. As duas camadas são independentes, e uma só já segura — que é
o que se queria de defesa em profundidade, e não estava previsto no desenho.

**(NC-2) a fronteira aceita o que a página inventar** — `knownMarkers` devolve
`reported` sem filtrar:

```
--- FAIL: TestProbeDiscardsMarkersItNeverAskedAbout
    the page smuggled strings we never sent: [Mum: see you at 8 outra aba but not really]
```

**(NC-3) as DUAS camadas removidas — o defeito H6 inteiro, contra Chrome real.**
É o único controle que prova que a asserção de PII morde:

```
--- FAIL: TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed (1.65s)
  renamed selector, chat list: page text crossed the boundary: "Mum" is in
    {... Markers:[Mum — see you at 8
                  Work — the deploy is out
                  +55 11 99999-0000 — are we still on?]}
  ... idem para "see you at 8", "the deploy is out", "99999-0000"
  renamed selector, conflict wording: page text crossed the boundary: "Mum" is in {...}
```

**(NC-4) a guarda de correção removida do script** — a linha do `#pane-side`
apagada de `markerScript`:

```
--- FAIL: TestMarkerScriptNeverReturnsPageText
    the marker script no longer refuses a loaded application
```

> **Nota de método sobre o NC-4.** A primeira tentativa deste controle PASSOU, e
> passou porque a mutação nunca aplicou: procurei o literal `'#pane-side'` no
> fonte, que ali é a constante `paneSideSelector` concatenada. É a ARMADILHA 3
> do repo em ação — controle negativo que não aplica não prova nada, e um que
> "passa" é indistinguível de um teste que não morde. Refeito com `assert` na
> substituição antes de escrever o arquivo.

**Gate executado** (2026-08-12, árvore limpa antes e depois):

```
go build ./...                                   OK  (repo inteiro)
go vet ./...                                     OK  (repo inteiro)
scripts/chromium-study: build + vet              OK  (o laboratório não quebrou)
gofmt -l internal/wa-headless/                   vazio
go test -race -count=1 ./internal/wa-headless/...
  wa-api/internal/wa-headless               ok  98.166s
  wa-api/internal/wa-headless/engine        ok   5.015s
  wa-api/internal/wa-headless/observability ok   1.189s
  wa-api/internal/wa-headless/spa           ok   2.222s
make lint   269 issues  ·  pai (stash, MESMA árvore) 269  ·  delta 0
  internal/wa-headless: as MESMAS 2 gocyclo pré-existentes, nenhuma tocada
```

Nenhum browser desta correção saiu por sinal: `stopped_via=browser.close` nas
duas corridas do teste novo. `web.whatsapp.com` não foi tocado — as fixtures são
servidas por `httptest`, e o caminho de marcadores é alcançado servindo o host
no PATH da URL, que é a regra do próprio classificador exercitada como escrita.

---

## H7 — `PageSnapshot.Title` carrega conteúdo da página, ninguém o lê, e ele nunca foi medido

**Data**: 2026-08-12 · **Contexto**: LOOP H6.1. Achado por ataque à própria
alegação do commit, não por revisão de código — a frase "não existe caminho pelo
qual texto da página chegue a uma string Go" foi posta à prova e não sobreviveu
inteira.

**Onde**: `internal/wa-headless/spa/probe.go`, `structureScript`
(`title: document.title`) e `internal/wa-headless/spa/page.go`,
`PageSnapshot.Title`.

**Problema**: o H6 fechou o caminho do `body.innerText`, mas o `structureScript`
continua trazendo `document.title`, que é **texto autorado pela página**, e o
campo é **exportado**. É a mesma forma do agravante 2 do H6, em escala menor:
qualquer chamador futuro pode logá-lo sem saber o que carrega.

Duas coisas agravam, e uma atenua:

1. **Ninguém lê o campo.** `grep -rn '\.Title\b' --include='*.go'
   internal/wa-headless/` não devolve **nenhum** leitor. Ele é decodificado do
   JSON e nunca consultado — nem pelo `Classify`, nem por teste. Um campo que
   ninguém lê não paga o risco que carrega.
2. **Nunca foi medido.** `EVIDENCIA-SPA.md` não tem uma única observação de
   `document.title` contra o SPA real, nos dois perfis. Não sei se o título do
   WhatsApp Web é `"WhatsApp"`, `"(3) WhatsApp"` ou algo que nomeie conversa ou
   contato. **Afirmar que é inócuo seria repetir exatamente o erro que o H6
   documenta**: tratar suposição sobre a página como propriedade dela.
3. *(atenuante)* diferente do `TextSample`, o título é curto e não é a lista de
   conversas. O risco é menor em magnitude, não em natureza.

**Evidência**: leitura do caminho e o `grep` de leitores acima. **Não** é
medição contra a conta real — e é essa ausência que constitui o achado.

**Correção sugerida**, em ordem de custo:

1. **Remover o campo e o `title:` do script.** É a correção mais barata e fecha
   a classe: zero consumidores hoje, então o diff é de duas linhas e o risco de
   remoção é nulo. Reintroduzir depois, se uma classificação precisar do título,
   custa o mesmo e aí virá com medição junto.
2. **Medir antes de decidir**, se houver intenção de usar o título para
   classificar (telas de erro costumam ser identificáveis por título): uma
   observação só-leitura do perfil pareado, registrando só a FORMA do título e
   não o valor, resolve — e é a mesma sonda que o M1–M3 já usa.

**Status**: **CORRIGIDO em 2026-08-12 (LOOP H7.1)**, e a correção 1 foi a
escolhida — `title: document.title` saiu do `structureScript` e
`PageSnapshot.Title` saiu da struct. Zero consumidores, diff de duas linhas.

Registrei primeiro como "não corrigir sem perguntar", e mudei de posição por um
motivo que é do próprio achado: isto **não é conserto de graça de bug alheio**,
é a alegação do commit anterior sendo trazida de volta à verdade. O `997cebe`
afirma que "não existe caminho pelo qual texto da página chegue a uma string
Go", e enquanto o `Title` atravessasse a frase era falsa. Corrigir a alegação
por comentário e deixar o campo seria escolher a documentação em vez do
mecanismo — exatamente o que a `CLAUDE.md` proíbe.

**O achado foi feito DUAS vezes, de forma independente e simultânea**: por
ataque do Chief à própria frase do commit, e pela validação independente
(evaluator em sessão separada), que chegou nele por outro caminho — notando que
as duas fixtures do teste de navegador **não tinham `<title>`**, de modo que a
asserção de PII passava de forma VAZIA justamente no único campo que ainda
cruzava. Convergência de dois métodos diferentes no mesmo ponto cego.

**Evidência BEFORE_FIX / AFTER_FIX.** A correção do teste veio ANTES da
correção do código, de propósito — foi o evaluator quem apontou que sem título
nas fixtures a asserção não morde. Com `<title>Mum — WhatsApp</title>` nas duas
fixtures e o campo ainda no lugar:

```
--- FAIL: TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed (2.04s)
  renamed selector, chat list: page text crossed the boundary: "Mum" is in
    {URL:.../renamed Title:Mum — WhatsApp ... Markers:[]}
  renamed selector, conflict wording: page text crossed the boundary: "Mum" is in
    {URL:.../renamed-conflict Title:Mum — WhatsApp ... Markers:[open in another another window]}
```

Repare que os `Markers` da segunda perna continuam corretos: o conflito segue
sendo detectado, então a falha é do `Title` e de mais nada. Depois de remover o
campo e a linha do script, `PASS`, com as fixtures mantendo o título — a
asserção agora cobre o caminho que estava escondido.

**O que continua NÃO medido**, e por isso não vira alegação: se o
`document.title` do WhatsApp Web carrega nome de contato com uma conversa
aberta. A remoção tornou a pergunta desnecessária para a privacidade, mas ela
volta a importar no dia em que alguém quiser usar o título para classificar tela
de erro. Se esse dia chegar, a medição vem junto — e registre só a FORMA do
título, nunca o valor.

---

## H8 — a checagem de host do classificador é `Contains`, e aceita domínio sósia

**Data**: 2026-08-12 · **Contexto**: LOOP H6.1, achado pela **validação
independente** ao auditar por que o teste de navegador consegue alcançar o
caminho de marcadores. Não é regressão destes commits: a regra é anterior.

**Onde**: `internal/wa-headless/spa/page.go`, no `Classify`:

```go
case s.URL != "" && !strings.Contains(strings.ToLower(s.URL), whatsappHost):
    return ClassRedirect
```

**Problema**: `strings.Contains` sobre a URL INTEIRA não é uma checagem de host.
Ela aceita como "no host certo":

```
https://web.whatsapp.com.attacker.example/     (sufixo de domínio)
https://evil.example/web.whatsapp.com          (no caminho)
```

Uma página sósia deixa de ser `REDIRECT`, e como não casa estrutura nenhuma, ela
**recebe a sonda de marcadores** — ou seja, o corpo dela é varrido e ela pode
responder `SESSION_CONFLICT` ou `ERROR_PAGE` à vontade. Desde o LOOP H6.1 isso
não vaza PII (o texto não atravessa mais), mas continua sendo classificação
controlada por quem serve a página.

**Severidade prática: BAIXA** — nós controlamos a navegação, e o motor só vai
para `web.whatsapp.com`. Vira relevante se algum dia houver redirecionamento
seguido sem validação, ou captive portal.

**Evidência**: leitura da regra, mais o fato de o próprio teste
`TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed` explorá-la de
propósito — ele serve a fixture em `/web.whatsapp.com/renamed` para alcançar o
caminho de marcadores sem tocar o host real.

**Correção sugerida**: `url.Parse` e comparar `u.Hostname()` com igualdade (ou
sufixo `.whatsapp.com` com ponto). **Quando isso for feito, o teste do H6 vai
falhar alto** — e isso é característica, não defeito: o comentário dele promete
exatamente essa falha. A fixture então passa o host pelo cabeçalho `Host` em vez
do caminho.

**Status**: **não corrigido** — apertar a regra muda comportamento de
classificação e derruba um teste de propósito, o que merece loop próprio em vez
de carona num loop de PII. Registrado com o caminho de correção pronto.

---

## H9 — `TestBrowserChainSeversTheWebSocketTransport` degrada para desligamento SUJO sob contenção

**Data**: 2026-08-12 · **Contexto**: LOOP H6.1, observado pela **validação
independente** — a primeira corrida de `-race` dela cruzou com testes de
navegador de outra sessão na mesma máquina.

**Onde**: `internal/wa-headless/realspa_test.go`,
`TestBrowserChainSeversTheWebSocketTransport`.

**Problema**: com a máquina disputada, os dois subtestes estouraram o prazo e a
corrida terminou com:

```
stopped_via=DIRTY_signal_after_close_timeout
```

Sozinho, o teste passa — reproduzido depois pela mesma sessão. Então não é
regressão dos commits do H6/H5, e é **pré-existente**.

**Por que isto não é só flakiness de teste**: `DIRTY_signal_after_close_timeout`
é o desligamento que a **invariante 2** proíbe em perfil com credencial, e a
fase 4C mediu que ele dá logout (4ª e 6ª iteração). O gatilho aqui foi carga da
máquina, não defeito do browser — e "só acontece sob carga" é precisamente como
uma condição chega à produção, onde a densidade por nó é o objetivo declarado da
iniciativa. Um nó com N sessões É a máquina disputada.

A pergunta que fica: o orçamento de `Shutdown` (10 s) é suficiente sob a
densidade que a CAP-10/CAP-11 pretendem, ou o caminho limpo passa a falhar em
volume exatamente quando mais importa?

**Evidência**: uma ocorrência, sob contenção conhecida, com a corrida isolada
passando. Não é medição de distribuição — é achado.

**Correção sugerida**: medir o tempo real de `Browser.close`→saída sob carga
crescente antes de fixar o orçamento; e considerar distinguir "estourou o prazo
porque a máquina está lenta" de "o browser não vai sair", que hoje colapsam no
mesmo desfecho sujo.

**Status**: **não corrigido** — pré-existente, fora do escopo dos loops de PII e
de shutdown desta sessão, e a correção depende de medição sob carga que não foi
feita. Referência cruzada: a invariante 2 do `HANDOFF-INICIATIVA.md` §6 e o item
6 da Definition of Done (`stopped_via` limpo em 100% das paradas).

### Implementação da decisão A — LOOP H5.3 (2026-08-12)

**A decisão, do usuário**: manter o sinal como recurso final. O argumento que a
sustenta não é conforto — é que a medição de 4C (logout na 4ª e na 6ª iteração)
foi feita com `SIGTERM` como caminho de ROTINA, tomado toda vez, e o caso
residual é sinal RARO depois de o caminho limpo falhar. Transportar aquele
número para cá seria usar uma medição fora da condição que a produziu, que é a
armadilha que este repositório mais paga. Do outro lado, "nunca sinalizar" tem
custo medido e certo: o `reclaimVerdict` recusa enquanto o pid viver.

**O preço, pago por mecanismo e não por promessa** (`engine/suspect.go`):

- toda parada suja escreve `.wa-headless-session-suspect` **dentro do perfil**,
  com o `StopVia` que a causou;
- a marca é escrita **pelo `CleanStop`**, não pelo chamador. `ProfileDir()`
  entrou na interface `BrowserProcess` exatamente por isso: deixar a marcação a
  cargo do call site faria ela depender de alguém lembrar, e o defeito que este
  pacote existe para impedir é precisamente um call site que esqueceu;
- a marca fica **dentro do perfil** de propósito: o perfil é o que viaja, e um
  perfil restaurado noutro host ou num contêiner leva a própria dúvida junto;
- ler **não** limpa. Só `ClearSessionSuspect` limpa, e ela só deve ser chamada
  depois de a sessão ter sido verificada de verdade — se a leitura limpasse, um
  boot que lesse o estado e caísse em seguida perderia a dúvida;
- perfil ilegível devolve **erro**, nunca `false`. "Não consegui checar" virando
  "está bom" é a forma fail-open que a invariante 12 proíbe.

**Testes que travam a decisão**: `TestCleanStopMarksTheProfileWhenTheStopGoesDirty`,
`TestCleanStopLeavesNoMarkWhenTheStopIsClean`,
`TestSuspectMarkSurvivesReadingAndIsClearedOnlyExplicitly`,
`TestSessionSuspectRefusesToGuessOnAnUnreadableMarker`,
`TestMarkingIsANoopWithoutAProfile` (`engine/suspect_test.go`).

**Controles negativos EXECUTADOS**, dois, ambos revertidos:

**(NC-A1) a parada suja não é registrada** — a chamada de marcação removida do
`CleanStop`:

```
--- FAIL: TestCleanStopMarksTheProfileWhenTheStopGoesDirty (0.15s)
    the browser went down by signal and the profile was left unmarked; the next
    boot would presume the session survived, which is exactly what decision A
    buys with the signal it keeps
```

**(NC-A2) marca em TODA parada, inclusive limpa** — é o controle que decide se a
marca SIGNIFICA alguma coisa. Um alarme permanente é um alarme ignorado, e
tornaria o sinal indistinguível do caminho do protocolo — a mesma armadilha que
fez o braço `browserclose` do estudo virar réplica do próprio controle:

```
--- FAIL: TestCleanStopLeavesNoMarkWhenTheStopIsClean (0.00s)
    a clean stop marked the session suspect; the signal would then be
    indistinguishable from the protocol path
```

**Fidelidade do dublê**: o `fakeProcess` devolve um diretório temporário REAL em
`ProfileDir()`, não `""`. Com string vazia o `MarkSessionSuspect` retorna cedo e
todos os testes do caminho sujo passariam sem a marcação jamais rodar — dublê
mais permissivo que a produção escondendo o mecanismo inteiro (ARMADILHA 1).

**O que fica aberto, e é consequência desta decisão, não pendência dela**: quem
CONSOME a marca. O `SessionSuspect` existe e é lido por teste; nenhum caminho de
boot ainda age sobre ele, porque o ciclo de vida que agiria é a CAP-05. A marca
não é auto-executável — ela garante que a informação sobreviva até existir quem
a use, que é o oposto de perdê-la em log.

## H10 — "o perfil nunca encolhe" é falso no granulado de UM boot: o Chromium rotaciona `Default/Sessions/*`

**Data**: 2026-08-12 · **Contexto**: achado **de lado** no LOOP 03.9, ao medir
higiene de perfil em volta da observação do `SingletonLock`. Não faz parte
daquele escopo, e não foi transformado em trabalho deste ciclo.

**Onde**: comportamento do Chromium 151 sobre `<perfil>/Default/Sessions/`. Do
nosso lado o que ele atinge é a expectativa escrita como observável da **CAP-05**
no `HANDOFF-INICIATIVA.md` §10 — *"N ciclos dormir/acordar sem degradação,
`Singleton` = 0, perfil **não-decrescente**"* — e o texto do LOOP 04.3D no
`EXECUCAO.md` ("perfil 2686 → 2781 arquivos, **cresceu sempre**").

**Problema, e ele tem duas metades de peso diferente** — a primeira é MEDIDA, a
segunda é HIPÓTESE. Elas estavam escritas como se fossem a mesma coisa, e a
avaliação adversarial do LOOP 03.9 mostrou que não são.

### MEDIDO — a asserção "um boot nunca reduz a contagem" é FALSA

Primeira corrida da observação, no perfil de laboratório:

```
BASELINE profile_files=457 singleton_lock_present=false
...
stopped_via=browser.close
HYGIENE profile_files before=457 after=456 delta=-1
the profile SHRANK (457 -> 456): a boot must never lose profile state
```

Isto é medição direta e não depende de explicação nenhuma: um boot **limpo**
(`stopped_via=browser.close`, sem parada suja, sem crash) devolveu o perfil com
**um arquivo a menos**. Portanto o observável "perfil **não-decrescente**" por
contagem de arquivos **reprova em boot sadio**. Esta metade é firme, e é ela
que invalida a checagem de higiene — o *porquê* do −1 não muda isso.

### HIPÓTESE — que a rotação de `Session_*`/`Tabs_*` seja a causa DESSE −1

Que o Chromium **rotacione** o par `Session_*`/`Tabs_*` está provado: o `diff`
das duas listagens (só os nomes relativos ao perfil) mostra par descartado e par
novo escrito, não deleção sem reposição.

```
350d349
Default/Sessions/Session_13431009036226323
352c351
Default/Sessions/Tabs_13431009036997542
---
Default/Sessions/Session_13431037567163574
353a353
Default/Sessions/Tabs_13431037567202699
```

**O que NÃO está provado é que essa rotação explique o −1**, e a razão é de
método: este `diff` foi tirado em volta de uma corrida **posterior**, cujo delta
foi **0** — 2 removidos, 2 acrescentados, saldo zero, como as próprias linhas
acima mostram. A corrida que de fato encolheu (457 → 456) **nunca foi diffada**.
Dizer "a rotação causou o −1" é inferir de um mecanismo real para um evento que
ele não foi visto causando. É exatamente o tipo de passagem que a regra "medir
antes de projetar" existe para barrar, e ela passou despercebida aqui.

**O que confirmaria o mecanismo** (barato, e ainda não feito): tirar a listagem
completa ANTES e DEPOIS **da mesma corrida cujo delta seja negativo**, e diffar
essa. Confirma se: o único nome que sumiu sem substituto estiver sob
`Default/Sessions/`. **Refuta se**: o arquivo perdido estiver fora de
`Default/Sessions/` — aí o −1 é perda de estado de verdade e esta entrada muda
de "comportamento saudável" para defeito. Como o delta negativo não apareceu em
todas as corridas (as seguintes deram `+0`), pode ser preciso repetir o boot até
reproduzi-lo; registrar quantas corridas custou faz parte da medida.

Enquanto isso não for feito, o que se pode afirmar é: `delta<0` num boot limpo
**acontece**, e a rotação é a explicação **plausível e não confirmada**.

**Por que isso importa — e repare que isto se apoia só na metade MEDIDA**: um
gate que trave "perfil não-decrescente" por contagem de arquivos vai reprovar em
boot sadio, e vai reprovar quer a causa seja a rotação, quer seja outra. O custo
é pior que o falso positivo — ensina o próximo leitor a APAGAR a checagem em vez
de lê-la, e aí o dia em que o perfil encolher de verdade não terá alarme nenhum.

**Correção sugerida** (para quem fechar a CAP-05, não para agora): o observável
"perfil não-decrescente" precisa ser enunciado sobre o que de fato não pode
sumir — o material de sessão (`Default/Local Storage`, `Default/IndexedDB`, as
chaves que a M3 mostrou carregarem a identidade) — e não sobre `find | wc -l`.
Excluir `Default/Sessions/*` da conta é o passo natural, **mas ele depende da
hipótese acima**: enquanto o diff da corrida negativa não for tirado, quem
excluir aquele diretório está tirando da vigilância um lugar que ainda não foi
visto sendo a causa. A ordem certa é medir primeiro, excluir depois.

**Status**: **não corrigido, e de propósito**. No LOOP 03.9 a asserção foi
trocada por REGISTRO no `TestRealSPASingletonLockFormat` — a contagem sai no
log — porque travar um número que nunca foi medido é a mesma especulação de
sempre, só que com cara de teste. O que o teste trava de verdade é o que foi
medido: depois do `browser.close` não sobra `SingletonLock`. **Revisado em
2026-08-12** pela avaliação adversarial do 03.9: a redação anterior apresentava
a rotação como causa medida do −1, e o comentário do teste dizia o mesmo; os
dois passaram a separar medição de hipótese. O achado **não** foi enfraquecido —
a metade que invalida o observável da CAP-05 é a medida.

---

## H11 — a coluna "janela em `OPENING`" do M6 não é o tempo em `OPENING`

**Data**: 2026-08-13 · **Contexto**: LOOP 04.3E (a perna que deveria PIORAR,
`EVIDENCIA-SPA.md` M7). Achado de lado, ao construir a medição da cauda.

**Onde**: `internal/wa-headless/EVIDENCIA-SPA.md`, seção **M6.2**, cabeçalho da
tabela — a coluna rotulada `janela em OPENING`.

**Problema**: os valores daquela coluna são `CONNECTED − meReadyTriggered`, e
não o tempo que o socket passou no estado `OPENING`. Confere linha a linha na
própria tabela do M6.2: 5,54−5,04 = 0,50; 6,79−6,29 = 0,50; 6,57−6,30 = 0,27.
`meReadyTriggered` é uma **âncora escolhida**, não o estado medido, e o rótulo
não diz que houve escolha.

Medido no 04.3E, com as duas âncoras colhidas na mesma corrida e o mesmo
instrumento (21 boots): a medida DIRETA — primeira amostra em `OPENING` até
`CONNECTED` — é **50–70 ms menor em 18 dos 21 boots**; em três boots de CPU da
rodada 1 a diferença chega a **80, 110 e 280 ms**, e os 280 ms de `cpu-1x` r1 são
**42% da janela daquele boot** (0,66 s) — que é o máximo de todas as pernas de
CPU, ou seja, o número mais carregado da falsificação do eixo CPU. Nenhum boot
ficou em 40 ms: o mínimo observado é 50. Os três *outliers* estão todos onde o
espaçamento observado foi pior, o que é o que se esperaria de um artefato de
resolução e não de uma propriedade das âncoras. Exemplos da corrida, colados do
M7.3 (note que o terceiro é um dos três *outliers*, com 80 ms):

```
unstressed r1   janela(M6) 0,49s   janela(direta) 0,44s
net-heavy  r1   janela(M6) 1,36s   janela(direta) 1,31s
cpu-2x     r1   janela(M6) 0,46s   janela(direta) 0,38s
```

**Nenhuma conclusão do M6 ou do M7 depende da escolha de âncora**, e isso deixou
de ser impressão: o N2a-EVAL re-derivou as duas colunas nos 21 boots e verificou
que **nada vira** sob a âncora direta — a monotonicidade da rede sobrevive
(0,64 → 0,70 → 1,31), o máximo global vai de 1,36 para 1,31 s, e a falsificação
do eixo CPU fica **mais forte**: a margem passa de 0,66/0,73 = 0,90 para
0,49/0,68 = **0,72**. (A justificativa anterior desta linha era a razão de 24×
contra o piso de detecção; ela foi **retirada** porque compara grandezas de eixos
diferentes e não limita coisa nenhuma — ver H12 e `EVIDENCIA-SPA.md` M7.6.)

O defeito é de ROTULAGEM,
e o custo é o de sempre: quem ler só o M6 vai citar "0,27–0,50 s de tempo em
`OPENING`" como se fosse o estado, e a próxima pessoa que medir o estado de
verdade vai achar que encontrou uma divergência.

**Correção sugerida**: renomear a coluna do M6.2 para `CONNECTED − meReady` e
acrescentar uma linha dizendo que é uma âncora, com ponteiro para o M7.2, onde
as duas medidas aparecem lado a lado. **Não** recalcular nem substituir os
números do M6: eles são evidência congelada e continuam corretos para o que
realmente mediram.

**Status**: **não corrigido**. Fora do escopo do 04.3E, que é medição, e a
política do repositório é não corrigir de graça achado fora de escopo sem
perguntar. O M7.2 já registra a ambiguidade e reporta as DUAS âncoras em todas
as 21 corridas, então nenhuma medição nova herda o problema; o que falta é
consertar o rótulo na evidência antiga.

---

## H12 — o guarda do restauro de rede não exercitava o restauro

**Data**: 2026-08-13 · **Contexto**: N2a, aplicação dos fixes exigidos pela
avaliação adversarial do LOOP 04.3E / M7. Achado **do avaliador**, não do
implementador — encontrado por mutação, não por leitura.

**Onde**: `internal/wa-headless/engine/network_test.go`, o antigo
`TestClearNetworkConditionsSendsAnEmptyRuleList`:

```go
func TestClearNetworkConditionsSendsAnEmptyRuleList(t *testing.T) {
	if got := degradedConditions(NetworkDegradation{}); len(got) != 1 {
		t.Fatalf("a degradation is one global rule, got %d", len(got))
	}
}
```

**Problema**: o nome anuncia o contrato de **restauro**; o corpo chama
`degradedConditions` e mede a cardinalidade do caminho de **degradação**.
`ClearNetworkConditions` nunca era chamado, e o caminho de sucesso do restauro
não era exercitado por teste nenhum. É a **ARMADILHA 2** deste repositório em
forma pura — e por isso o achado é de mutação: a suíte estava verde.

Evidência do avaliador (MUTANTE 4): trocar o corpo de `ClearNetworkConditions`
por uma regra de **queda permanente** deixava a suíte **verde**.

**Alcance na época**: contido, porque cada boot do M7 abre browser e aba novos,
então uma limpeza quebrada não vazava para o boot seguinte. Mas o `t.Cleanup` do
`applyDegradation` é o único usuário do contrato, e a próxima reutilização de
aba pagaria a conta.

**Correção aplicada**: extraído `clearedConditions()` em `engine/network.go` —
o **único ponto de construção** da lista do caminho de limpeza, espelhando o que
`degradedConditions` já é para o caminho de degradação — e
`ClearNetworkConditions` passa a chamá-lo. **O comportamento CDP emitido não
muda**: antes passava `nil` a `applyConditions`, agora passa
`clearedConditions()`, que retorna `nil`. O teste passou a se chamar
`TestClearNetworkConditionsClearsWithAnEmptyRuleList` e afirma sobre esse ponto
de construção. A propriedade que o teste antigo de facto media foi preservada,
com o nome certo, em `TestDegradationIsASingleGlobalRule` (que ganhou também a
asserção de que o padrão de URL é o global).

### Controle negativo EXECUTADO

Laboratório isolado (`scratchpad/lab`: cópia de `engine/` + `observability/` com
`go.mod`/`go.sum` do repo). **O repositório não foi mutado.** Linha de base da
cópia: `ok wa-api/internal/wa-headless/engine 3.783s`.

**MUTANTE 4A** — a queda permanente instalada no ponto de construção do caminho
de limpeza, que é onde os MUTANTES 1–3 do avaliador também foram aplicados
(`degradedConditions`, não `SetNetworkDegraded`):

```go
func clearedConditions() []*network.Conditions {
	return []*network.Conditions{{
		URLPattern: allRequests, Offline: true,
		DownloadThroughput: 0, UploadThroughput: 0,
	}}
}
```

Antes da correção este mutante passava **VERDE**. Depois dela, **compila E
falha com mensagem**:

```
--- FAIL: TestClearNetworkConditionsClearsWithAnEmptyRuleList (0.00s)
    network_test.go:90: the restore path built rule 0 of 1: {URLPattern: Latency:0 DownloadThroughput:0 UploadThroughput:0 ConnectionType: PacketLoss:0 PacketQueueLength:0 PacketReordering:false Offline:true}
    network_test.go:92: the restore path built 1 rule(s) and the contract is ZERO: the rule list is replaced wholesale, so an EMPTY list is the only form that leaves nothing behind — anything else is an emulation that outlives the boot that asked for it
FAIL
FAIL	wa-api/internal/wa-headless/engine	0.198s
```

(A mensagem desreferencia os ponteiros de propósito: a primeira versão imprimia
`[0x5b15c256a050]`, que nomeia nada e por isso é meio guarda.)

### O que este guarda NÃO pega, medido e não suposto

**MUTANTE 4B** — a mesma queda permanente escrita **direto no corpo** de
`ClearNetworkConditions`, sem passar por `clearedConditions()`. Continua
**VERDE**:

```
--- PASS: TestClearNetworkConditionsClearsWithAnEmptyRuleList (0.00s)
ok  	wa-api/internal/wa-headless/engine	0.191s
```

Isto **não é uma fraqueza deste guarda em particular**: é uma propriedade de
pacote único em Go, e o guarda da degradação — que a avaliação aceitou — tem
exatamente o mesmo limite. Medido, para não ficar em palavra: reescrever o corpo
de `SetNetworkDegraded` para embutir uma regra com `Offline: true`, desviando de
`degradedConditions`, também passa **verde** (`ok ... 0.193s`), embora o MUTANTE 1
do avaliador — a mesma queda aplicada DENTRO de `degradedConditions` — mate o
teste.

Fechar o desvio exigiria uma costura de observação no código de produção (um
campo em `Tab`, ou uma variável de pacote no lugar de `applyConditions`) para que
o teste pudesse chamar o método real sem browser. Isso é mais largo que o
"pequeno helper extraído" que o pacote do N2a autorizou, e troca uma estrutura de
produção limpa por alcance de teste. **Não foi feito**, e fica aqui nomeado: o
contrato está guardado no ponto de construção, com a mesma força — nem mais, nem
menos — que o contrato da degradação.

**Status**: **corrigido**. Coberto por
`TestClearNetworkConditionsClearsWithAnEmptyRuleList` (contrato de restauro,
controle negativo 4A acima) e `TestDegradationIsASingleGlobalRule` (a
propriedade que o teste antigo media). O desvio do ponto de construção
(MUTANTE 4B) fica registrado acima como limite conhecido e **não** corrigido.

---

## H13 — o F-24 do `EXECUCAO.md` ainda afirma a unidirecionalidade que o M7.5 retirou

**Data**: 2026-08-13 · **Contexto**: N2b (M8, a permanência em `OPENING` sob
corte). Achado ao ler o M7 e o F-24 para saber o que já estava estabelecido
sobre a janela de `OPENING` — **fora do escopo** desta tarefa, que é medição.

**Onde**: `internal/wa-headless/EXECUCAO.md:904-910`, dentro do F-24:

```
**Ressalva de instrumento, e ela é grande.** Na perna `cpu-2x` o pior
espaçamento entre amostras foi de **5,11 s** contra uma janela de 0,46 s — o
tick pior é dez vezes a coisa medida. A falsificação do eixo CPU sobrevive
porque um amostrador grosseiro reporta a transição TARDE e portanto **infla**
a janela: o erro possível aponta ao contrário da conclusão. Não sobrevive por
a resolução ter sido suficiente, e o M7.5 diz isso com as duas direções de
erro.
```

**Problema**: o parágrafo afirma exatamente o que a avaliação adversarial do
N2a derrubou, e depois cita como testemunha o texto que o derrubou. O
`EVIDENCIA-SPA.md` M7.5 e o M7.8 item 4 dizem o oposto, com todas as letras:

> **Não** sobrevive porque o erro apontasse para o outro lado — não aponta: um
> buraco sobre a marca de INÍCIO encolhe a janela, e o texto anterior desta
> linha afirmava uma unidirecionalidade que os dados não dão.

O erro do amostrador **não é unidirecional**: a janela é a diferença de duas
marcas, ambas enviesadas para tarde (`Ŵ = W + δc − δm`), então um buraco sobre o
FIM infla e um buraco sobre o INÍCIO **encolhe**. O que sustenta a falsificação
do eixo CPU é o delta de **80/60/70 ms** entre as duas âncoras nas três corridas
de `cpu-2x`, que prova resolução de ~70 ms **na vizinhança das marcas** — e é
esse número, não o sentido do enviesamento, que o M7.5 usa.

A última frase do parágrafo (*"Não sobrevive por a resolução ter sido
suficiente, e o M7.5 diz isso com as duas direções de erro"*) mostra que a
correção do N2a chegou ao EVIDENCIA e **não** ao diário: o parágrafo tem a
correção colada no fim de uma afirmação que ela contradiz, e as duas ficam de pé
lado a lado. Quem ler só o `EXECUCAO.md` — que é onde se lê "o que já está
estabelecido" — sai com o argumento retirado.

**Gravidade**: documentação, não código. Nenhum número do M7 muda, nenhum teste
depende disto. Mas é o mesmo tipo de defeito que o próprio N2a foi corrigir
(o enquadramento dos 24,4× congelado num comentário onde a próxima pessoa o
leria como assentado), e sobreviveu no arquivo vizinho.

**Correção sugerida**: substituir as duas frases finais do parágrafo pelo
argumento do M7.5 — as duas direções de erro, e o delta entre âncoras de
80/60/70 ms como o que de fato sustenta a falsificação. Uma frase, sem tocar em
número nenhum.

**Status**: **corrigido** pelo Chief, no mesmo ciclo em que o defeito nasceu.

O worker do N2b agiu certo ao registrar e não consertar: para ele o achado era
externo ao escopo, e a política do projeto manda registrar e perguntar em vez de
"consertar de graça". Quem tinha de decidir era o Chief, e a decisão foi
consertar — porque `git log -S` põe o parágrafo no `77722e2`, que é **commit
deste ciclo**. Não era dívida herdada: era defeito recém-introduzido, e terminar
o próprio trabalho não é ampliar escopo.

Não há teste a travar: é prosa de diário, e a política anti-regressão do
`CLAUDE.md` se aplica a achado com defeito de comportamento.

### O mecanismo do engano, que é a parte reutilizável

O defeito não foi um erro de escrita. Foi uma **correção aplicada a um documento
e não ao seu espelho**:

```
avaliação adversarial derruba o argumento da unidirecionalidade
        ↓
fix F3 nomeia o alvo: "EVIDENCIA-SPA.md M7.5 e M7.8 item 4"
        ↓
aplicado exatamente ali  ✓
        ↓
o espelho do MESMO argumento no EXECUCAO.md fica para trás  ✗
```

Três leituras não pegaram: o avaliador (o escopo dele era o M7, não o diário), o
worker de correção (seguiu a lista fechada, corretamente), e o Chief, que
conferiu os sete fixes **exatamente nos `file:line` que a lista nomeava**.
Verificar contra a lista prova que a lista foi cumprida; **não prova que a lista
estava completa**.

Agravante que torna o caso pior que uma omissão: a frase corretiva foi parar no
FIM do mesmo parágrafo, ao lado da afirmação que ela contradiz — e citando como
testemunha (`"e o M7.5 diz isso"`) justamente o texto que a desmente. As duas
ficaram de pé, e quem lesse só o diário — que é onde se lê "o que já está
estabelecido" — sairia com o argumento retirado.

**Regra que sai daqui**: quando uma avaliação derrubar um ARGUMENTO (e não um
número), a correção MUST varrer os espelhos daquele argumento, não só o
documento auditado. O comando é barato — procurar a frase derrubada em todos os
`.md` do módulo — e a ausência dele custou este achado.

## H14 — o elo entre `OpeningWindowThreshold` e a evidência medida existe só em prosa

**Data**: 2026-08-18 · **Contexto**: LOOP 04.4, avaliação adversarial
independente de `internal/wa-headless/spa/socket.go`, arquivo novo que
introduz o limiar `C = 3030ms`.

**Onde**: `internal/wa-headless/spa/socket.go` (comentário de derivação de
`OpeningWindowThreshold`) e `internal/wa-headless/spa/socket_test.go`
(`TestOpeningWindowThresholdMatchesDocumentedTerms`).

**Problema**: o teste que parece travar a derivação de `C` trava, na
verdade, a CONSTANTE contra três literais hardcoded no próprio arquivo de
teste (`healthyUpperBound`, `measurementUncertainty`, `explicitGuardBand`).
Ele não amarra nada ao comentário de `socket.go` nem ao `EVIDENCIA-SPA.md`
M7.3/M7.5.

**Evidência (mutação executada pelo avaliador)**: alterar APENAS o
comentário de derivação — termo 1 de "1.36s" para "9.99s", sem tocar na
constante nem no teste — fez a suíte inteira PASSAR. Já a mutação que altera
só a constante (3030ms → 3000ms) FALHA no teste, com a mensagem "the
constant and its derivation comment have drifted apart".

**Consequência**: alguém pode reescrever a seção M7.3 do `EVIDENCIA-SPA.md`
com outros números, ou editar o comentário de `socket.go`, e nenhum teste
morde. O que está protegido é o drift entre a constante e os literais do
teste; o que não está é a narrativa de derivação em si, nem o vínculo com o
`.md` de onde os números vêm.

**Correção sugerida**: nenhuma automatizável de forma razoável — um teste Go
não faz parsing daquele markdown. O caminho seria um gate fora do Go (script
que extraia os números do `.md` e compare com os do `.go`), e isso é decisão
à parte, não desta capacidade. Registrar a lacuna vale mais que escondê-la.

**Status**: **PARCIALMENTE CORRIGIDO em 2026-08-18, LOOP 04.5.** O que mudou:
os três termos deixaram de ser literais dentro do arquivo de teste e passaram a
ser **dados nomeados em código de produção** (`healthyUpperBound`,
`measurementUncertainty`, `explicitGuardBand` em `spa/socket.go`), com
`OpeningWindowThreshold` **composto** por eles em vez de comparado a eles. O
teste — agora `TestOpeningWindowThresholdTermsAreLocked` — trava cada termo
contra a sua fonte medida, e a mutação foi executada pelo Chief: alterar
`healthyUpperBound` de 1360 para 1400 ms faz o teste reprovar com
`healthyUpperBound = 1.4s, want 1.36s (M7.3)`.

O que **permanece aberto**, e é o achado original: nenhum teste Go liga esses
valores ao `EVIDENCIA-SPA.md`. Provar automaticamente que prosa humana está
semanticamente correta é impossível, e a direção técnica confirmou que não se
deve criar teste que prometa isso. A diferença é que os números agora vivem em
código executável, não só em comentário — mutar o comentário continua passando,
mas o comentário deixou de ser onde o valor mora.

**CORREÇÃO DESTA PRÓPRIA ENTRADA, 2026-08-18 (LOOP 05.9).** A frase acima
*"um teste Go não faz parsing daquele markdown"* — que eu escrevi — está
**errada**, e a distinção que ela perde é justamente a que importa.

Provar que a NARRATIVA está semanticamente correta é de fato impossível. Mas
não é isso que faltava: faltava travar que o VALOR citado no `.md` e o valor
da constante em `.go` continuam sendo o mesmo número. E isso um teste Go faz,
porque o número tem âncora estável — `EVIDENCIA-SPA.md:1331` é uma linha de
tabela:

    | pior janela de boot saudável medida (21 boots, 7 condições) | **1,36 s** |

Ler o arquivo e afirmar que `healthyUpperBound` bate com o número dessa linha
não promete nada sobre a prosa; promete exatamente uma coisa verdadeira e
verificável — **que os dois lados não divergiram em silêncio**, que é o achado
original desta entrada.

**E o desenho, MEDIDO antes de ser prometido** — porque "os três números ligam
ao `.md`" era a minha segunda afirmação apressada sobre esta entrada, e ela
também não sobreviveu ao contato. São **dois links e uma igualdade**, coisas
diferentes:

| termo | valor | o que dá para travar | âncora |
|---|---|---|---|
| `healthyUpperBound` | 1,36 s | link com o `.md` | linha `pior janela de boot saudável medida`, **1 ocorrência** |
| `measurementUncertainty` | 0,31 s | link com o `.md` | linha de tabela `` | `net-*` | `` do M7.5, **1 ocorrência**, tomando o TOPO da faixa `0,30–0,31` — que é o que o `socket.go` declara fazer ("the worse of the two") |
| `explicitGuardBand` | 1,36 s | **nada no `.md`** | é *"an explicit engineering choice, not a further measurement"*, declarada ancorada ao termo 1. O travável é `explicitGuardBand == healthyUpperBound`, invariante de CÓDIGO |

As duas âncoras foram verificadas por extração real (uma ocorrência cada, número
extraído batendo com a constante). O termo 3 não tem o que ancorar por
construção — e um teste que fingisse ancorá-lo seria exatamente o tipo de
promessa vazia que a F-28 ensinou a não fazer.

Não implementado neste ciclo: a tarefa autorizada no momento era a medição de
retenção longa (token `HOLD`), e abrir uma segunda frente no meio de uma
medição é como se perde a rastreabilidade. Registrado com o desenho pronto,
as âncoras verificadas e o limite dele declarado.

**Status revisto**: parcialmente corrigido, com a lacuna restante agora sabida
**fechável** — ao contrário do que esta entrada afirmava.

**FECHADO em 2026-08-18 (LOOP 05.10)**, com autorização da orquestração (token
`H14`). Dois testes em `spa/socket_test.go`:

- `TestMeasuredTermsMatchTheEvidenceDocument` — lê o `EVIDENCIA-SPA.md`, exige
  que cada âncora case com **exatamente uma** linha, e compara o número contra a
  constante de produção. A unicidade da âncora é parte do teste, não higiene:
  âncora que passa a casar duas linhas parou de identificar uma medição.
- `TestGuardBandIsAnchoredToTermOne` — o termo 3 não tem fonte no documento, e o
  travável é a identidade de CÓDIGO `explicitGuardBand == healthyUpperBound`.

**Um erro de desenho meu, pego pelo próprio teste na primeira execução.** A
regra inicial era "a última duração da LINHA". Ela lê `1,36 s` de
`` | `net-*` | 0,30–0,31 s | 0,49–1,36 s | `` — a coluna da JANELA MEDIDA, não a
do espaçamento do instrumento, de onde o termo vem. Pior que não achar nada:
pegaria um número real da medição errada. Corrigido com seleção de célula
(`evidenceAnchor.cell`), e o porquê está escrito no campo.

O protótipo que eu havia rodado antes de prometer devolvia `['0.31','1.36']` e eu
li como confirmação **porque o 0,31 estava lá** — sem verificar qual dos dois a
regra escolheria. Medir e depois ler a medição com a hipótese na cabeça é a
mesma falha que o `CLAUDE.md` descreve: *se a medição só confirmou o que você já
achava, provavelmente ela não mediu nada*.

**Quatro controles negativos, executados, e os três primeiros FALHARAM na
primeira tentativa** — pela armadilha 3, silêncio não é prova. O `replace` com
`count=1` acertava a primeira ocorrência de `**1,36 s**` no documento, que é
outra linha, então a mutação nem tocava a âncora. Refeitos por índice de linha:

- reescrever o M7.3 (`1,36 → 9,99`): *"healthyUpperBound = 1.36s in code, but
  M7.3 ... says 9.99s"* — **este é o achado ORIGINAL desta entrada**, e agora
  morde.
- reescrever a coluna do M7.5 (`0,31 → 0,99`): *"measurementUncertainty = 310ms
  in code, but M7.5 ... says 990ms"*.
- duplicar a linha-âncora: *"matched 2 lines ..., want exactly 1"*.
- desancorar o termo 3 (`1360 → 1500 ms`): *"the band is a number with no stated
  derivation behind it"*.

Documento e `socket.go` restaurados byte-idênticos após cada mutação.

**O que estes testes NÃO provam**, dito aqui para não virar citação errada: que
a PROSA está correta. Isso não é mecanicamente verificável e a direção técnica
está certa em recusar teste que prometa isso. O que eles provam é a coisa
estreita e verdadeira: **o valor citado no documento e o valor na constante não
divergiram em silêncio**.

## H15 — o envelope de validade de `C` não tem expressão executável

**Data**: 2026-08-18 · **Contexto**: LOOP 04.4, avaliação adversarial
independente de `internal/wa-headless/spa/socket.go`, mesma revisão da H14.

**Onde**: `internal/wa-headless/spa/socket.go`, comentário de
`OpeningWindowThreshold` (seção "VALIDITY ENVELOPE") e a função
`ClassifyOpeningDuration`.

**Problema**: o comentário declara que `C` só vale até 900ms de latência
ADICIONADA — o teto da curva de três pontos do M7 — e que além disso a
resposta é UNKNOWN. Mas `ClassifyOpeningDuration` recebe apenas uma
`time.Duration` crua e não tem nenhuma noção de latência adicionada.
Operacionalmente, um enlace real acima de 900ms de RTT adicionado produz
exatamente o mesmo `SocketHealthy`/`SocketDegraded` de qualquer outro, sem
nenhum sinal de que a garantia por trás de `C` deixou de valer ali.

**Evidência**: verificado por leitura do código pelo avaliador —
`ClassifyOpeningDuration` não recebe, não mede e não consulta latência em
lugar nenhum; sua assinatura é `func ClassifyOpeningDuration(d
time.Duration) SocketLiveness`. O envelope é 100% prosa.

**Correção sugerida**: quando existir consumidor (CAP-06), o caminho de
liveness precisaria conhecer a latência observada para saber que está fora
do envelope, e reportar isso em vez de devolver um veredito com confiança
que não tem. Enquanto não houver consumidor, não há o que corrigir.

**Status**: **não corrigido nesta sessão**. Lacuna conhecida e declarada.

## H16 — a lista de pontos não fecha o meio do domínio, e isso está escrito no teste

**Data**: 2026-08-18 · **Contexto**: errata da F-28, depois da CAP-04 fechada.

**Onde**: `internal/wa-headless/spa/socket_test.go`,
`TestClassifyOpeningDurationSampledPointsMapToTwoValues`.

**Problema**: o teste percorre uma **lista finita de 11 pontos**, não o domínio.
`time.Duration` é `int64` de nanossegundos — ~1,8 × 10^19 valores representáveis.
Fechar as pontas (0, `C±1ns`, `math.MaxInt64`) não fecha o **meio**: uma mutação
que dispare estritamente entre dois pontos amostrados escapa.

**Evidência (mutação executada, e reproduzida pelo Chief)**: com
`if d > 48h && d < 72h { return SocketLiveness("MUT_ENTRE") }` no classificador,
a suíte inteira **PASSA**. Já a mutação acima do antigo máximo
(`d > 400*24h`) agora **FALHA**, pega pelo ponto `math.MaxInt64` — essa metade
foi fechada por esta errata.

**Por que não foi corrigido**: fechar a propriedade exigiria varredura real ou
teste de propriedade, e a instrução da orquestração vetou introduzir framework
de property testing nesta errata. Listar cada fronteira de intervalo à mão não
escala.

**O que MITIGA**: a garantia central não é do teste, é **estrutural** —
`ClassifyOpeningDuration` tem exatamente dois `return`, ambos devolvendo uma das
duas constantes declaradas, e não existe terceira constante para devolver. Um
terceiro ramo é diff visível em `socket.go`, não algo que o teste prove ausente
rodando. Isso está escrito no próprio arquivo de teste, sob o título
"WHAT IS GUARANTEED BY TESTS VS BY STRUCTURE".

**Status**: **não corrigido, e declarado no lugar onde engana** — o nome do
teste deixou de prometer exaustão, e o comentário nomeia a fuga que permanece.
É a lição da F-28 aplicada ao conserto da própria F-28: o nome não pode
prometer o que o corpo não executa.

## H17 — o ponteiro do futuro consumidor de `C` está obsoleto no próprio código

**Data**: 2026-08-18 · **Contexto**: trace canônico da CAP-05, pedido pela
orquestração antes de implementar.

**Onde**: `internal/wa-headless/spa/socket.go:100`.

**Problema**: o comentário do tipo `SocketLiveness` diz que a taxonomia completa
"lives in EVIDENCIA-SPA.md and in **CAP-06's still-unopened scope** (the caller
that tracks history and combines axes)". A CAP-06 fechou, e ela era outra coisa:
**inventário de módulos do SPA**. Ela não tem — e não deveria ter — nada que
combine eixos de liveness ou guarde histórico de sessão. O ponteiro aponta para
um lugar que existe e não é aquele.

**Como apareceu**: eu afirmei à orquestração que "o consumidor de `C`" era um
dos adiamentos que apontavam para a CAP-05. Ela mandou não contar isso sem
confirmar no contrato canônico. Ao conferir, descobri duas coisas: a minha
afirmação era inferência, e a fonte escrita dizia **outra coisa ainda** — CAP-06.

**O que o contrato canônico realmente diz**: a CAP-05 é *"pareamento por QR,
restauração, desligamento limpo, reclaim de `Singleton`"* (`HANDOFF §10`). Ela
**não menciona** consumir veredito de degradação. Por outro lado, `core/doc.go`
— que é a casa da CAP-05 — declara que o pacote *"holds the state that outlives
a single command"* e carrega *"heartbeat renews, TTL covers abrupt death"*.
Estado que sobrevive a um comando é exatamente a pré-condição que o consumidor
de `C` precisa (*"has this socket ever reached CONNECTED? was the previous
classification DEGRADED?"*).

**Conclusão honesta**: `core/` é o único lugar canônico que satisfaz a
pré-condição, mas **nenhum contrato atribui a consumação de `C` a ele**. Isso é
diferente de "é ali". O consumidor verdadeiro segue **INDETERMINADO**.

**Correção sugerida**: quando a CAP-05 existir e o seu call graph estiver
escrito, decidir por evidência onde `C` é consumido — e só então corrigir o
ponteiro de `socket.go:100`. Não corrigi agora porque trocaria um palpite errado
por outro palpite: apontar para a CAP-05 sem prova repetiria exatamente o erro
que esta entrada registra.

**Status**: **CORRIGIDO em 2026-08-18 (LOOP 05.4)**.

A pré-condição desta entrada — *"quando a CAP-05 existir e o seu call graph
estiver escrito"* — foi satisfeita. E a resposta veio por medição, não por
palpite: `grep` por `Socket|Liveness|ClassifyOpening` em `internal/wa-headless/core/`
retorna **zero ocorrências**. A CAP-05 que existe tem a pré-condição certa
(`core/doc.go`: *"holds the state that outlives a single command"*) e **não é**
a consumidora.

O ponteiro foi corrigido para dizer o que se sabe e parar aí: a CAP-06 estava
errada duas vezes (fechou, e era inventário de módulos), a CAP-05 não é, e o
consumidor segue **INDETERMINADO**. Nenhum destino novo foi nomeado — nomear um
sem call site repetiria exatamente o erro que esta entrada registra.

**Achado incidental, corrigido no mesmo bloco**: o comentário ainda carregava a
afirmação exagerada da **F-28** — *"makes that impossible to violate by
construction, not just by policy"*. A orquestração já tinha julgado isso
(*"NOT IN DECLARED VOCABULARY != UNREPRESENTABLE BY THE TYPE SYSTEM"*), mas a
correção fora aplicada às minhas falas e **não ao código**. Agora o comentário
diz "impossible to express in the DECLARED VOCABULARY", registra a distinção e
nomeia quem de fato guarda a propriedade:
`TestClassifyOpeningDurationExhaustsToTwoValues`.

## H18 — `BootFailure` não carrega o PID do browser que não conseguiu subir

**Data**: 2026-08-18 · **Contexto**: CAP-05, levantado pelo próprio executor.

**Onde**: `internal/wa-headless/core/session.go`, tipo `BootFailure`.

**Problema**: em qualquer caminho de falha, `StartSession` não devolve `*Session`
— por desenho, um boot que falhou não tem nada vivo para entregar. Logo não há
PID. Os testes de teardown provam "sem processo órfão" pelo artefato
`SingletonLock`, não por PID recuperado. Isso basta para o que a CAP-05 precisa,
mas um consumidor futuro de métrica/alerta que queira identificar o processo que
morreu não tem por onde.

**Correção sugerida**: acrescentar `PID int` ao `BootFailure`. Mudança estreita e
aditiva — não feita porque nada neste escopo precisou dela.

**Status**: não corrigido, aguardando consumidor que justifique.

## H19 — a sequência ler-e-limpar da marca de suspeita não tem teste próprio

**Data**: 2026-08-18 · **Contexto**: CAP-05, levantado pelo próprio executor.

**Onde**: `internal/wa-headless/core/session.go` — `engine.SessionSuspect` na
entrada do boot e `engine.ClearSessionSuspect` depois do sucesso.

**Problema**: `engine/suspect_test.go` cobre o mecanismo da marca **isolado**, e
a CAP-05 agora o compõe no boot. Mas todos os testes desta fatia partem de perfis
`t.TempDir()` recém-criados, que **nunca** foram marcados suspeitos — `wasSuspect`
deu `false` em todos. Ou seja: o caminho composto ler→limpar nunca foi exercitado
a partir de um perfil de fato suspeito.

**Consequência**: se `ClearSessionSuspect` deixasse de rodar no sucesso, ou se a
leitura passasse a devolver `false` por engano, nenhum teste desta fatia morde. A
invariante 2 diz que a sessão deve ser **verificada** no boot seguinte a uma
parada suja; a verificação existe, a prova de que ela roda no caminho composto,
não.

**Correção sugerida**: fixture que pré-grava `.wa-headless-session-suspect` e
prova que o boot lê, e que um boot bem-sucedido limpa.

**Status**: **CORRIGIDA em 2026-08-18**, na fatia de fechamento da CAP-05A.
Dois testes novos partem de um perfil primado com a marca de produção
(`engine.MarkSessionSuspect`, não escrita à mão): um prova que o boot lê e que o
sucesso limpa; o outro prova que um boot que FALHA **não** limpa. A mutação
exigida — mover `ClearSessionSuspect` para antes do `Launch` — faz o segundo
falhar, e foi reproduzida pelo Chief. Nada em produção mudou: a posição do clear
já estava certa; faltava a prova.

## H20 — a suíte do `core` ficou 5× mais lenta, e a causa é o conserto certo

**Data**: 2026-08-18 · **Contexto**: LOOP 05.2, medido depois da correção.

**Onde**: `internal/wa-headless/core/` — em especial
`TestStartSession_SuspectMarker_NotClearedOnFailedBoot` e os testes cuja página
nunca monta.

**Problema**: `go test -race -count=1 ./internal/wa-headless/core/...` foi de
**28 s para 152 s**. A causa é correta e esperada: antes, uma página que não
estava pronta falhava no primeiro probe; agora o laço de assentamento espera o
`DefaultSettleBudget` inteiro (60 s) antes de desistir. Um teste que prova
"nunca fica pronto" **precisa** pagar o orçamento para provar isso.

**Por que registrar mesmo sendo correto**: suíte lenta é suíte que se pula.
152 s no pacote mais central do módulo é o tipo de custo que, acumulado, faz
alguém rodar `-run` seletivo e deixar de ver regressão.

**Correção sugerida**: os testes de caminho negativo podem injetar um orçamento
menor em vez de usar o default — `spa.WaitForReady` já recebe `budget` como
parâmetro, então é questão de o teste passar um valor curto e afirmar que o
laço respeita o orçamento **recebido**, o que aliás é uma propriedade melhor de
travar do que "espera 60 s". Não aplicado nesta sessão: mexer nos testes de
regressão logo depois de eles terem acabado de validar uma correção é
exatamente quando se introduz um defeito sem perceber.

**Status**: **CORRIGIDO em 2026-08-18 (LOOP 05.5)**, pela correção desenhada
acima. O motivo do adiamento — *"mexer nos testes de regressão logo depois de
eles terem acabado de validar uma correção"* — expirou: os testes já validaram
dois ciclos de correção desde então, e o custo tinha PIORADO (152 s na medição
original, **201 s** medidos agora antes de mexer).

**O que mudou**: `StartConfig` ganha `SettleBudget`, com zero significando
`spa.DefaultSettleBudget`. Os dois testes de caminho negativo que pagavam o
orçamento inteiro — `FailureTearsDownDeterministicallyWithNoOrphan` e
`SuspectMarker_NotClearedOnFailedBoot`, 61 s cada, 122 s dos 201 s — passam a
receber `negativePathSettleBudget = 3s`. A duração da espera era incidental a
ambos: um prova desmontagem sem órfão, o outro prova que a marca sobrevive.

**Medição**: pacote `core` de **201,2 s para 98,4 s** (−51%), mesma máquina,
mesma sessão. Suíte completa verde sob `-race`.

**A propriedade melhor, travada**:
`TestStartSession_SettleLoopRespectsTheBudgetItWasGiven`. "O boot espera 60 s" é
um fato sobre um DEFAULT; "o boot desiste no orçamento que recebeu" é um fato
sobre o MECANISMO, e continua significando algo quando o default mudar. Sem ele,
tornar o orçamento configurável seria só aceleração, com nada afirmando que o
botão está ligado.

**Controles negativos, executados, cada um mordendo a SUA asserção** — o teste
tem limite superior e inferior de propósito, porque uma regressão de tiro único
também terminaria bem abaixo do default e passaria só com o limite superior:
- ignorar `cfg.SettleBudget`: *"boot took 1m1.72s, at or beyond the 1m0s DEFAULT
  budget despite being given 4s — SettleBudget is not reaching the settle loop"*.
- voltar ao tiro único: *"boot gave up after 1.67s, sooner than the 4s budget it
  was given — the settle loop is not waiting out its budget"*.

`session.go` restaurado byte-idêntico após cada mutação.

## H21 — o detentor não tem como perguntar se a sessão ainda está viva

**Data**: 2026-08-18 · **Contexto**: LOOP 05.6, ao construir o `runtime.Holder`
com WRITE_SET restrito a `runtime/` (`core/` fora de escopo por decisão da
orquestração).

**Onde**: API pública de `internal/wa-headless/core` — `Session` expõe
`Browser()`, `Tab()`, `ProfileDir()` e `Stop()`, e nada mais.

**Problema**: o `Holder` guarda uma sessão entre comandos e a devolve a quem
pedir. Ele **não tem como saber se ela ainda está utilizável**. Se o browser
morrer sozinho — crash, OOM, morte externa — o `Holder` continua entregando o
mesmo ponteiro, e o comando seguinte descobre o problema como um erro de CDP no
meio de uma operação de negócio, não como uma sessão inválida.

Isto **não é hipotético para este módulo**: `engine.ProcessAlive` existe e o
teste de N ciclos já o usa para caçar órfãos. O sinal existe; o que não existe
é um caminho pela API do `core` para o detentor consultá-lo sem alcançar
`Browser().PID()` e reimplementar a política por fora — que é exatamente o tipo
de conhecimento vazado que o item 4 do briefing separa entre as camadas.

**Por que não corrigi**: `core/` está fora do WRITE_SET deste ciclo (resposta
`SORUNTIME` da orquestração). Fazer o `Holder` sondar `Browser().PID()` por
conta própria seria contornar a restrição pela porta dos fundos e colocar
política de liveness na camada errada — e a liveness deste módulo já tem uma
lição cara sobre isso (item 12 do briefing: processo, alvo, service worker,
socket, SPA, sessão e identidade são sinais DIFERENTES).

**Correção sugerida**: `core.Session` ganhar uma consulta de liveness cuja
semântica seja declarada — provavelmente sobre o PROCESSO, o sinal mais barato e
menos ambíguo — com nome que não prometa mais do que mede (`ProcessAlive`, não
`Healthy`). Um detentor então decide o que fazer; a política fica nele, o sinal
fica no `core`.

**Status**: **CORRIGIDO em 2026-08-18 (LOOP 05.8)**, depois de a orquestração
autorizar `core/` no WRITE_SET (token `H21`).

**O sinal, no `core`**: `Session.ProcessAlive()`. O nome é a coisa verdadeira
mais estreita que cabia, de propósito — não `Healthy`, não `Alive`. O item 12 do
briefing é o motivo: processo, alvo, service worker, socket, SPA, sessão e
identidade são sinais DIFERENTES, e este módulo já pagou caro por confundi-los.
Um processo vivo ainda não diz nada sobre o socket, a SPA ou a identidade. O que
ele dá é o negativo barato e sólido: **processo morto significa que tudo acima
dele também morreu**.

**A política, no detentor**: `Holder.Session` pergunta antes de entregar e
**recusa** com `ErrSessionDied`. Recusar em vez de re-bootar é decisão, não
omissão — re-bootar em silêncio esconderia um browser que morre sempre, virando
vazamento lento em vez de falha alta (ADR-0005 D7), e "quantos browsers este
perfil já teve" passaria a depender de sorte. Se uma sessão morta deve ser
substituída automaticamente é decisão de PRODUTO, e este pacote não a toma
sozinho.

**O teste é construído para o detector poder FALHAR** (item 15 do briefing): o
browser é morto **de fora** com `SIGKILL`, do jeito que um crash ou um OOM
chegam, então toda a contabilidade em processo continua dizendo que a sessão
está boa. Um detector que respondesse sempre "vivo" passaria em todos os outros
testes do arquivo e falharia só neste.

**Controles negativos, executados, ambos mordendo com a mesma mensagem**:
- `ProcessAlive` mentindo (`return true`): *"the Holder handed out a session
  whose process (pid 70487) is gone, with err=<nil>; want ErrSessionDied"*.
- checagem removida do `Holder`: idem, pid 70609.

**Precisão sobre o que a mutação D NÃO pegou**: ela não derrubou
`TestHolder_ProcessAliveIsFalseAfterStop`, porque a guarda de `stopped` retorna
antes de alcançar a linha mutada. São mecanismos distintos — o teste do `Stop`
trava a guarda, não a sonda — e registro assim em vez de contar como cobertura
que não é.

**Testes**: `TestHolder_RefusesAHeldSessionWhoseProcessDied`,
`TestHolder_ProcessAliveIsFalseAfterStop`. Ambos arquivos restaurados
byte-idênticos após cada mutação.

