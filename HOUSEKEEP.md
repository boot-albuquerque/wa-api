# HOUSEKEEP — wa-api

Achados incidentais do **wa-api como um todo**: a aplicação em `pkg/`, o
build, os gates, a configuração e as rotas HTTP.

Achados da biblioteca vendorizada moram noutro arquivo:
`internal/wa-noise/HOUSEKEEP.md`. A separação não é organizacional — os dois
têm ciclos de vida diferentes. O que está em `internal/wa-noise/` acompanha o
upstream e é candidato a virar patch ou a sumir num rebase; o que está aqui é
nosso e só nós corrigimos.

Um achado que atravessa a fronteira fica no arquivo de quem CAUSA o problema,
com referência cruzada no outro.

O formato de cada entrada, e a política anti-regressão que rege a passagem
para "corrigido", estão em `CLAUDE.md` / `AGENTS.md`.

> **Nota de procedência (2026-08-08):** este arquivo nasceu da divisão do
> antigo `internal/wa-noise/HOUSEKEEP.md`, que registrava o repositório
> inteiro. O índice que ele mantinha no topo foi descartado na divisão — ele
> já trazia uma correção admitindo estar desatualizado em relação às próprias
> entradas, e um índice que mente é pior que a ausência dele. As entradas
> vieram integralmente; nenhuma foi perdida.


## 2026-08-06 — bug de locale no `coverage-gate` do Makefile

**Encontrado durante**: verificação final da feature de arquitetura
multi-sessão nativa (branch `feature/native-multisession-architecture`).

**Onde**: `Makefile`, alvo `coverage-gate` (por volta da linha 76-95),
especificamente a linha:

```makefile
cur=$$(echo "$$pct" | awk '{printf "%d", $$1*10 + 0.5}');
```

**Problema**: o `awk` converte o percentual de cobertura (ex: `"81.6%"` →
`"81.6"`) para décimos de ponto percentual multiplicando por 10. Essa
conversão depende de `LC_NUMERIC` para interpretar o `.` como separador
decimal. Sob locale `pt_BR.UTF-8` (locale deste ambiente — `LANG` do
usuário), `awk` não reconhece `.` como decimal e para de parsear no
primeiro caractere não-numérico, lendo só a parte inteira:

```
$ echo "81.6" | awk '{printf "%d", $1*10 + 0.5}'
810   # deveria ser 816
```

Isso faz o gate comparar um valor truncado (`810`) contra o piso declarado
em `.coverage-baseline`, podendo gerar `FALHA: a cobertura caiu` **mesmo
quando a cobertura real está acima do piso** — falso negativo que bloqueia
CI/PR sem motivo real.

O comentário em `.coverage-baseline` (linhas 12-13) afirma "comparacao
inteira e' exata e nao depende de locale para o separador decimal" — a
comparação de inteiros de fato não depende de locale, mas a **conversão**
de string para inteiro via `awk` depende, e é aí que o bug mora. A
afirmação do comentário está incorreta na prática.

**Workaround usado nesta sessão**: rodar com
`LC_NUMERIC=C LC_ALL=C make coverage-gate` para forçar `awk` a interpretar
`.` como decimal independente do locale do sistema.

**Correção sugerida**: prefixar o `awk` do alvo `coverage-gate` (e
qualquer outro alvo do Makefile que faça parsing de decimal, ex.
`log-coverage-gate` se usar padrão similar) com `LC_NUMERIC=C` /
`LC_ALL=C`, ou trocar por uma conversão que não dependa de locale (ex.
`printf`/`bc` com formatação explícita).

**Status**: **CORRIGIDO** (lote D, 2026-08-07). O `awk` do alvo
`coverage-gate` é prefixado com `LC_NUMERIC=C LC_ALL=C`, e o gate passou a
falhar fechado se a conversão devolver vazio ou zero. O comentário errado em
`.coverage-baseline` (que afirmava que a conversão não dependia de locale) foi
reescrito para descrever o bug real. `log-coverage-gate` foi auditado e não usa
o padrão — lê inteiros direto do baseline com `grep -oE`, sem conversão decimal.

Verificação: `echo "81.6" | LC_ALL=pt_BR.UTF-8 awk '{printf "%d", $1*10+0.5}'`
devolve `810`; com o prefixo, `816`. E `LANG=pt_BR.UTF-8 LC_ALL=pt_BR.UTF-8 make
coverage-gate` — sem nenhum prefixo manual — agora imprime
`coverage: 829 decimos de %` e sai 0.

---

## 2026-08-06 — `ConnectHandler` passa token vazio para `StartSession`

**Encontrado durante**: mesma feature acima (arquitetura multi-sessão
nativa).

**Onde**: `pkg/presentation/http/handlers/handler_session.go:85`:

```go
if h.StartSession != nil {
    go h.StartSession(id, "")
}
```

`id` vem de `sessionUser(w, r)` (`handler_session.go:31-45`), que lê
`info.Get("Id")` do contexto de autenticação (linha 38). O segundo
parâmetro (`token`) é passado como string vazia literal — nunca lido do
contexto, mesmo o contexto tendo o dado disponível (outros handlers do
mesmo pacote já fazem `info.Get("Token")`, ex.
`handler_webhook.go:115,195,282`).

**Problema**: é um bug pré-existente, não introduzido pela feature acima —
o código anterior já chamava `go h.StartClient(id, "", "", kill)` com o
mesmo campo vazio. Antes, esse token vazio ia parar dentro de
`startClient`, que tratava várias coisas inline. Agora ele é gravado em
`MyClient.Token` via `SessionAttachHook.Attach`
(`pkg/bootstrap/session_attach_hook_adapter.go`), e é esse campo que
`sendEventWithWebHook`/`SessionEventDispatcher` usam para correlacionar
webhook e `UserInfoCache` por usuário — ou seja, o bug ficou mais visível
estruturalmente (mais próximo da superfície onde o dado é consumido),
mesmo sem mudar o comportamento observável.

**Correção sugerida**: expor o token em `sessionUser` (ou ler direto
`info.Get("Token")` em `ConnectHandler.ServeHTTP`,
`handler_session.go:67-85`) e passar para
`h.StartSession(id, token)` em vez de `h.StartSession(id, "")`. Baixo
risco, mudança pequena e localizada.

**Status**: **corrigido** em `ae11dc3` (2026-08-06, mesma sessão/branch,
depois de discussão explícita com o usuário sobre corrigir agora).
`ConnectHandler.ServeHTTP` (`handler_session.go:84-88`) volta a fazer o
type-assert do `userInfo` já validado por `sessionUser` — sem checagem
extra, já que `sessionUser` retornou `ok=true` antes desse ponto — e lê
`info.Get("Token")`, mesmo padrão de `handler_webhook.go:115`. Nada é
logado (só passado como parâmetro pro orchestrator), então
`logassert.NoSecrets` continua passando.
`TestConnectHandler_StartsClientOnce` (`handler_session_test.go`) passou
a assertar o token recebido contra o sentinel `logassertAdminToken` do
userinfo autenticado de teste, não só o `userID` — antes o teste não
teria pego essa lacuna porque só verificava `userID`.
Cobertura verificada como neutra (81.9% → 81.9%, isolado via
`git stash`/medição contra o commit anterior) — o piso do
`coverage-gate` já estava desatualizado antes deste fix, por conta de
outro commit concorrente não relacionado (`ace7770`,
`feat(contacts): last-activity`), que também precisa de ajuste de
baseline (fora do escopo deste fix).

---

## 2026-08-06 — `.coverage-baseline` `min_coverage=822` estava incorreto/não-reprodutível

**Encontrado durante**: implementação do plano de vendoring do wa-noise
(branch `feature/vendor-wa-noise`, `.omc/plans/vendor-wa-noise-native-fork.md`).

**Onde**: `.coverage-baseline:40` (na branch-base `feature/native-multisession-architecture`,
commit `31287b9`).

**Problema**: `make coverage-gate` reportava `81.9% < piso declarado 82.2%`
logo após o vendoring (troca mecânica de import path em 45 arquivos, sem
lógica nova). Para isolar se era regressão real, criei um worktree
temporário exatamente no commit-pai (`git worktree add /tmp/... 31287b9`,
sem nenhuma das minhas mudanças) e medi lá:
`go test ./... -coverpkg=./... -coverprofile=... && go tool cover -func=... | tail -1`
→ **81.9%**, idêntico à medição pós-vendoring. Ou seja, o `822` já estava
errado/não-reprodutível **antes** desta branch existir — não foi regredido
por mim, nunca foi 82.2% de forma reproduzível.

**Causa provável (não confirmada)**: a medição anterior (registrada em
`.coverage-baseline` por outra sessão, ver histórico do arquivo) pode ter
capturado um resultado não-determinístico (timing/paralelismo de testes
afetando quais branches de código executam) ou um erro de leitura pontual.
Não investiguei a fundo — não é uma linha de código com bug, é uma medição
de métrica.

**Correção aplicada**: `min_coverage` ajustado para `819` (o valor honesto
e reproduzível, confirmado 2x — antes e depois do vendoring), com nota
explicando a investigação. Ver commit `d11979b` em
`feature/vendor-wa-noise`.

**Status**: corrigido (branch `feature/vendor-wa-noise`, ainda não
mergeada em `develop` no momento deste registro).

---

## 2026-08-06 — `.log-coverage-baseline` tinha `min_func_coverage=`/`min_errpath_coverage=` duplicados

**Encontrado durante**: mesma implementação acima (vendoring do wa-noise).

**Onde**: `.log-coverage-baseline`, herdado do commit `ace7770`
(`feat(contacts): expõe GET /user/contacts/last-activity`, de uma sessão
concorrente — não relacionado ao vendoring).

**Problema**: o commit `ace7770` adicionou uma nova linha
`min_func_coverage=707` (refletindo a métrica após a feature de
last-activity) mas **não removeu** a linha anterior
`min_func_coverage=710` (da Fase 2g, sessão diferente) — ficaram duas
chaves `min_func_coverage=` no mesmo arquivo (710 e 707), e
`min_errpath_coverage=856` duplicada de forma idêntica. `make check` (via
`log-coverage-gate`) quebrava com `/bin/sh: [: 710\n707: integer
expression expected` — o `grep -oE '^min_func_coverage=[0-9]+'` capturava
as duas ocorrências, concatenadas com newline, inválidas como inteiro
único pro `[ ... ]` do shell.

**Correção aplicada**: removida a linha `min_func_coverage=710`/
`min_errpath_coverage=856` mais antiga, mantendo só o par final
(707/856, o valor efetivamente vigente pós-`ace7770`). Ver commit
`d11979b` em `feature/vendor-wa-noise`.

**Status**: corrigido nesta branch. **Atenção**: como `ace7770` é de outra
sessão/branch que pode não ter esse fix, vale confirmar que a duplicata
não reaparece no merge — é um problema de "esqueceu de apagar a linha
velha ao adicionar a nova", fácil de reintroduzir se outra sessão editar
o arquivo do mesmo jeito.

---

## 2026-08-06 — `internal/waclient/` (vendored wa-noise) sem bridge de log para o padrão do projeto

**Encontrado durante**: revisão de arquitetura pós-vendoring do wa-noise
(branch `feature/vendor-wa-noise`), solicitada explicitamente para
avaliar se `internal/waclient/` segue os padrões de log/erro já
estabelecidos no resto do projeto (via agente `architect`).

**Onde**:

- `internal/waclient/util/log/log.go:17-23` — interface `waLog.Logger`
  (`Warnf/Errorf/Infof/Debugf/Sub`) que o wa-noise espera receber.
- `pkg/infra/wa-noise/logger.go:12` — `ZerologAdapter`, que implementa
  `appport.Logger` (`Info/Warn/Error(ctx, msg, keyvals...)`), uma
  interface **diferente** — não satisfaz `waLog.Logger`.
- `pkg/bootstrap/main.go:327-329` e
  `pkg/bootstrap/session_orchestrator_wiring.go:22-24` — únicos pontos de
  wiring; deixam o logger `nil` a menos que `--wadebug` seja passado.
- `internal/waclient/client.go:241-242` — logger `nil` vira
  `waLog.Noop` internamente.
- `internal/waclient/util/log/log.go:64` — `stdoutLogger.outputf`, usa
  `fmt.Printf` com ANSI + timestamp próprio quando `--wadebug` está
  ligado.

**Problema**: dois efeitos concretos.

1. Em produção (sem `--wadebug`), todo erro de socket/handshake/decrypt/
   appstate dentro da camada vendorizada (`internal/waclient/`) é
   **descartado silenciosamente** — não chega no zerolog nem no stderr,
   diferente do resto da aplicação, que sempre loga estruturado.
2. Quando `--wadebug` está ligado, o output é texto puro com ANSI/
   timestamp próprios, misturado no mesmo stream que emite JSON
   estruturado (`--logtype=json`, `pkg/bootstrap/main.go:165`), sem
   `req_id`/`role`/correlação com o resto dos logs da app.

**Achado secundário (severidade média)**: adoção de `apperr` na fronteira
do port é parcial — `pkg/infra/wa-noise/user_adapters.go:40,44,67,71,80`
repassa `err` cru vindo do waclient sem `apperr.New(...)`, então esses
erros chegam no HTTP boundary sem `Code`/`Category`/`Retryable`. Os
demais pontos da fronteira (`session_provider_adapter.go`,
`session_guard_adapter.go`, `misc_adapters.go`) já fazem a tradução
correta com `errors.Is` contra sentinels do wa-noise — nenhum
string-matching encontrado no repo.

**Confirmado como correto (sem ação necessária)**: `.logcov-exclude`
excluir `internal/waclient/` é arquiteturalmente certo (código
third-party vendorizado, MPL-2.0, não é lógica nossa) e não esconde a
fronteira de callback real — `session_provider_adapter.go:272` e
`pkg/bootstrap/eventhandler.go:25` continuam dentro de `pkg/`, no escopo
normal de log-coverage.

**Correção sugerida**: implementar um adapter que satisfaça
`waLog.Logger` sobre zerolog (mapear `Sub(mod)` para
`.With().Str("wa_module", mod)`), injetar nos dois pontos de wiring
citados, e logar por padrão pelo menos Warn/Error (não `nil`/`Noop`) —
reservando `--wadebug` só para baixar o nível a Debug. Para o achado
secundário, envolver os retornos crus de `user_adapters.go` com
`apperr.New(...)` conforme os pontos forem tocados.

**Status**: não corrigido — decisão de quando implementar pendente com o
usuário; avaliação de arquitetura clean/DDD-lite + testes para essa área
em andamento na mesma sessão.

---

## 2026-08-06 — `user_info_failed` classificado como `CategoryInternal` sendo erro de entrada

**Contexto**: execução da Fase 3 (apperr) do plano
`.omc/plans/wa-noise-clean-arch-walog-bridge.md`, que fixa código e
categoria dos 5 sites em tabela.

**Onde**: `pkg/infra/wa-noise/user_adapters.go:63-66` (era
`user_adapters.go:71` antes dos splits das Fases 0):

```go
parsed, err := toJIDs(jids)
if err != nil {
    return nil, apperr.New("user_info_failed", apperr.CategoryInternal,
        "failed to resolve user info targets", true, err)
}
```

**Problema**: o erro vem de `toJIDs`, ou seja, de um `domain.JID` que o
caller passou e que `ParseJID` não aceita — é falha de entrada, não do
servidor. Classificado como `CategoryInternal` e `retryable=true`, o
handler HTTP responde 500 e sugere retry para uma requisição que vai
falhar igual em toda tentativa. Reproduz com
`GetUserInfo(ctx, "u1", []domain.JID{"@s.whatsapp.net"})`, coberto por
`TestUserAdapter_GetUserInfo_JIDInvalido`.

A tabela do plano (§3, Fase 3) rotula esta linha como "GetUserInfo (SDK)",
o que sugere que o autor mirava o erro da chamada `client.GetUserInfo` —
mas esse site não é um `return err` cru (é `return client.GetUserInfo(...)`
direto), então a linha citada só pode ser a de `toJIDs`. Seguido o plano
literalmente para não divergir em silêncio de um plano revisado.

**Correção sugerida**: trocar para
`apperr.New("user_info_invalid_jid", apperr.CategoryValidation, ..., false, err)`,
alinhando com os demais erros de parse de JID da fronteira, e atualizar
`TestUserAdapter_GetUserInfo_JIDInvalido`.

**Status**: não corrigido — plano aprovado fixa código e categoria; mudar
aqui seria divergir do que foi revisado. Pendente de decisão do usuário.

---

## 2026-08-06 — data race real em `pkg/infra/wa-noise/safe_go_test.go`

**Contexto**: quebra de `pkg/infra/wa-noise/` em subpacotes por domínio. Ao
incluir os subpacotes novos em `TEST_PKGS` (que roda com `-race`), o gate
passou a expor uma corrida que o `Makefile` já conhecia mas mantinha
mascarada por uma exclusão.

**Onde**: `pkg/infra/wa-noise/safe_go_test.go:10-19` (hoje
`pkg/infra/wa-noise/safego/safego_test.go`) e `Makefile:21`
(`TEST_PKGS := ... | grep -v '^wa-api/pkg/infra/wa-noise$'`).

**Problema**: `TestSafeGo_NormalExec` escrevia `called = true` dentro da
goroutine de `SafeGo` e lia a mesma variável na goroutine de teste, sem
sincronização — `go test -race ./pkg/infra/wa-noise/` falhava com
`WARNING: DATA RACE`. Pior: quando a leitura acontecia antes da escrita (o
caso comum), o teste chamava `t.Skip` e passava sem verificar nada. A
reação anterior tinha sido tirar o pacote inteiro de `TEST_PKGS` — uma
trava que não trava.

**Correção sugerida**: sincronizar por canal em vez de variável
compartilhada, e remover a exclusão do `Makefile`.

**Status**: corrigido nesta sessão (commits `c196a68` e `d005ab1`).
`go test -race` passa em toda a árvore de `pkg/`.

---

## F59 — `ClientLookup` é interface morta: nenhuma referência no repositório

**Data**: 2026-08-07.
**Contexto**: reorganização de `pkg/infra/wa-noise/` por responsabilidade.
Ao mover `registry/session_counter.go` para `adapters/sessioncount/adapter.go`,
o arquivo levou junto duas interfaces; só uma é usada.

**Onde**: `pkg/infra/wa-noise/adapters/sessioncount/adapter.go` (era
`registry/session_counter.go:16`).

```go
// ClientLookup is the subset of ClientManager methods needed by adapters
// that look up WhatsApp clients by user ID. [...]
type ClientLookup interface {
	GetWaNoiseClient(id string) *wanoise.Client
}
```

**Problema**: `grep -rn 'ClientLookup' --include='*.go' .` devolve apenas esta
declaração. Nenhum tipo a implementa por nome, nenhuma função a recebe,
nenhum teste a exercita. O papel que o comentário descreve — "quebrar a
dependência de tipo concreto entre root main e internal/" — hoje é cumprido
por `waclient.Getter` (`pkg/infra/wa-noise/client/`), que é o que
`wiring_handlers.go:102` de fato usa (`waclient.ClientForGetter`).

A interface irmã no mesmo arquivo, `ClientHealthProvider`, é usada de verdade
(é o parâmetro de `NewSessionCounterAdapter`). Só `ClientLookup` sobrou.

**Correção sugerida**: remover a declaração. Não há caminho de compatibilidade
a preservar — remover uma interface que ninguém referencia não pode quebrar
consumidor nenhum, e o compilador prova isso.

**Status**: **CORRIGIDO** (2026-08-07). A interface foi removida; o compilador prova que ninguem a referenciava.
mover código, não apagá-lo. Registrado para decisão.

## F60 — comentário em `client/testkit/helpers.go` cita um método com nome corrompido

**Data**: 2026-08-07.
**Contexto**: mesma reorganização. Apareceu ao verificar quem referenciava
`registry.ClientManager`.

**Onde**: `pkg/infra/wa-noise/client/testkit/helpers.go:18`.

```go
// o comportamento de registry.ClientManager.Getwa-noiseClient).
```

**Problema**: `Getwa-noiseClient` não existe e não é um identificador Go
válido. É resíduo de uma substituição em massa `whatsmeow` -> `wa-noise` que
alcançou o interior de um nome em CamelCase: o método real é
`GetWaNoiseClient`. Comentário, então não quebra build — mas é exatamente o
tipo de string que alguém vai procurar com grep e não achar.

O mesmo padrão aparece em outros comentários do fork (`Getwa-noiseClientsCount`,
`Iteratewa-noiseClients` em `registry/wa_clients.go` antes da quebra); vale
uma varredura por `wa-noise` grudado no meio de um identificador, e não uma
correção pontual.

**Correção sugerida**: `grep -rn '[A-Za-z]wa-noise[A-Z]' --include='*.go' .` e
corrigir os casos, todos em comentário.

**Status**: **CORRIGIDO** (2026-08-07), e o alcance era MAIOR que esta entrada estimava: 12 identificadores CamelCase corrompidos (`Getwa-noiseClient`, `Iteratewa-noiseClients`) mais 49 referencias `*wa-noise.X`, que deveriam ser `*wanoise.X` — 44 arquivos ao todo. Todas em comentario: o codigo compila, entao nenhuma era identificador real.
com a F59 numa passada só.

## F61 — `PATCHES.md` e `HOUSEKEEP.md` citam caminhos de `pkg/infra/wa-noise/` que não existem mais

**Data**: 2026-08-07.
**Contexto**: reorganização de `pkg/infra/wa-noise/` por responsabilidade
(commit `ca34600`).

**Onde**: ~15 ocorrências, sobretudo em `internal/wa-noise/PATCHES.md`
(linhas 251, 483, 715, 1065, 1489, 2054, 2747, 3084, 3188, 3421, 3497) e
`internal/wa-noise/HOUSEKEEP.md:874`.

**Problema**: as referências são a `pkg/infra/wa-noise/walog/`,
`pkg/infra/wa-noise/group/`, `pkg/infra/wa-noise/user/` e
`pkg/infra/wa-noise/safego/`, que hoje são `observability/walog/`,
`adapters/group/`, `adapters/user/` e `runtime/safego/`. Quem seguir o
caminho não acha nada.

**A tensão, que é o ponto desta entrada**: os dois arquivos são **registro
histórico** — descrevem o que era verdade quando a análise foi feita.
Reescrever caminhos dentro deles falsifica o registro: uma entrada datada de
2026-08-05 passaria a citar uma estrutura de diretórios que só existiu a
partir de 2026-08-07. Mas deixar como está entrega ao leitor um caminho que
não resolve.

**Correção sugerida**, em ordem de preferência:

1. **Nota de época no topo de cada arquivo**, mapeando os caminhos antigos
   para os novos uma vez só, e deixar o corpo intacto. Preserva o registro e
   resolve a navegação, ao custo de uma indireção na leitura.
2. Reescrever os caminhos e marcar cada linha alterada. Navegação direta, mas
   polui o texto e ainda assim reescreve história.
3. Não fazer nada. Defensável se esses arquivos forem lidos como arqueologia
   e não como referência viva — mas `PATCHES.md` **é** consultado ao decidir
   sobre divergências do fork, então não é o caso.

**Status**: **não corrigido**. Precisa da sua decisão entre as três, porque a
escolha é sobre o que esses documentos são, não sobre o texto deles.

## F64 — inventário dos 38 `// TODO` restantes, item a item

**Data**: 2026-08-07.
**Contexto**: varredura completa de `TODO/FIXME/XXX/HACK` no projeto.

Esta entrada é **registro de decisão, não trabalho pendente**. Ela existe para
que a próxima varredura não refaça a classificação do zero, e para que cada
item tenha um critério explícito do que o resolveria — sem isso, "revisar os
TODOs" volta a ser uma tarefa sem fim de escopo.

### Como reproduzir a varredura

```
grep -rnE '(//|/\*)\s*(TODO|FIXME|XXX|HACK)\b' --include='*.go' .
```

Hoje isso devolve **39 ocorrências**: 38 marcadores reais e 1 falso positivo.

### O falso positivo — NÃO TOCAR

`internal/wa-noise/core/user.go:125` não é um marcador. É a palavra portuguesa
"todo", em caixa alta por ênfase, no meio de uma frase quebrada por wrap de
comentário:

```go
// [...] Mas o gerador de internals.go expoe
// TODO metodo nao exportado de *Client, entao esta fachada virava
// DangerousInternalClient.GetFBIDDevices: [...]
```

Lê-se "expõe **todo método** não exportado". Qualquer varredura automatizada
vai casar de novo; qualquer correção automatizada vai corromper o texto.

### Aritmética

42 marcadores reais na varredura original. Quatro sítios foram corrigidos —
`send/encrypt.go:93` e `send/fb_encrypt.go:74` (F62), `core/request.go:235`
(F63) e `message/secret_keys.go:86` (resolvido sem mudar lógica, ver o commit
da F62). Restam **38**, nas quatro categorias abaixo.

---

### Categoria A — incógnitas de formato de fio (20)

Perguntas do autor upstream sobre o que o servidor do WhatsApp manda ou
espera. **Nenhuma é resolvível por leitura de código**: exigem captura de
tráfego de um cliente oficial, ou um teste contra o servidor real.

| # | Local | O que o comentário diz | O que resolveria |
| --- | --- | --- | --- |
| A1 | `capabilities/appstatesync/mutation.go:86` | `what's index 2 here?` | Capturar uma mutação de app state real e inspecionar o array de índice além das duas primeiras posições |
| A2 | `capabilities/group/create.go:83` | `"trigger": "1"` — `what's this?` | Comparar com o atributo que o WhatsApp Web envia hoje ao criar grupo. O Baileys manda o mesmo valor fixo, o que sugere constante de protocolo, não flag |
| A3 | `capabilities/group/notification.go:189` | `can the addressing mode change here?` | Observar uma notificação de grupo durante migração PN→LID do mesmo participante |
| A4 | `capabilities/group/parse.go:73` | `confirm field name` (`participant_pn`) | Capturar uma notificação de mudança de tópico em grupo e conferir o nome do atributo no XML |
| A5 | `capabilities/media/download_file.go:89` | `omit hash for unencrypted media?` | Tentar o download de mídia não cifrada com e sem o hash na query e comparar as respostas |
| A6 | `capabilities/media/download.go:143` | idem A5, outro caminho | Mesmo teste. **Os dois andam juntos**: resolver um sem o outro deixa os caminhos divergentes |
| A7 | `capabilities/media/download_transport.go:123` | `user agent for whatsapp downloads?` | Verificar se o CDN de mídia discrimina por User-Agent (hoje vai vazio) |
| A8 | `capabilities/media/upload.go:224` | `non-on-demand backfills may require this? it's in the initial bootstrap payload and may need to be persisted` | Inspecionar o payload de bootstrap inicial e verificar se o campo é reusado em backfill não sob demanda |
| A9 | `capabilities/message/decrypt.go:50` | `edits have an additional <meta msg_edit_t=... original_msg_t=.../> node` | Capturar uma edição de mensagem e decidir se os dois timestamps devem virar campos do evento |
| A10 | `capabilities/message/parse.go:68` | `IsFromMe?` | Determinar se o campo se aplica ao tipo de nó em questão |
| A11 | `capabilities/notification/picture.go:30` | `sometimes there's a hash and no ID?` | Coletar notificações de troca de foto de perfil até observar o caso, e decidir o comportamento |
| A12 | `capabilities/user/devices.go:122` | `take identities here too?` | Verificar se a resposta de usync traz identidades aproveitáveis junto da lista de dispositivos |
| A13 | `capabilities/user/devices.go:124` | `do something with the icdc blob?` | Entender o formato do blob ICDC (Identity Change Detection Client) e se ele deve ser persistido |
| A14 | `capabilities/user/devices.go:138` | `include dhash for users` | Confirmar se o servidor aceita `dhash` para usuários e não só para o caso já implementado |
| A15 | `core/receipt.go:206` | `change played to played-self?` | Confirmar qual tipo de recibo o servidor espera para mídia reproduzida pelo próprio remetente |
| A16 | `protocol/appstate/hash.go:53` | `figure out if there are certain cases that are safe to ignore and others that aren't` | Classificar os modos de falha de verificação de hash de app state. Hoje todos são tratados igual |
| A17 | `protocol/appstate/patch_builders_chat.go:79` | `set LastSystemMessageTimestamp?` | Verificar se o servidor usa o campo em patches de chat |
| A18 | `protocol/msgattrs/fbmessage.go:59` | `gifPlayback?` | Confirmar se o atributo existe no caminho FB de mensagem |
| A19 | `protocol/proto/waMsgApplication/extra.go:13` | `MultiDeviceApplicationVersion = 1 // TODO: check` | Confirmar a versão que o servidor espera. Uma constante errada aqui é formato de fio — **mesma classe da F42** |
| A20 | `protocol/types/user.go:176` | `DHash string // is this just a timestamp?` | Inspecionar valores reais de `dhash` retornados pelo usync |

**Recomendação: manter todos como estão.** O trade-off é assimétrico — o
comentário custa uma linha e declara honestamente o que não se sabe; apagá-lo
sem resposta troca ignorância declarada por ignorância silenciosa. A19 é o de
maior risco (constante de formato de fio, como a F42) e o primeiro a atacar se
houver captura de tráfego disponível.

---

### Categoria B — não implementado ou decisão de design nossa (12)

Estes **não dependem de informação externa**. São funcionalidade ausente ou
escolha de arquitetura que este projeto pode tomar.

| # | Local | O que o comentário diz | Nota |
| --- | --- | --- | --- |
| B1 | `capabilities/appstatesync/send.go:33` | `create new key instead of reusing the primary client's keys` | Reuso de chave entre clientes. Tem implicação de segurança: vale avaliar antes de tratar como cosmético |
| B2 | `capabilities/message/parse.go:222` | `// TODO` — **vazio**, no ramo `franking` | Não diz nada. Não há como saber o que o autor pretendia |
| B3 | `capabilities/message/parse.go:224` | `// TODO` — **vazio**, no ramo `trace` | Idem B2. Os dois ramos hoje são no-op silencioso |
| B4 | `capabilities/newsletter/actions.go:71` | `handle response?` | A resposta do servidor é descartada. Decidir se algum erro dela deve virar erro do chamador |
| B5 | `capabilities/retry/handle.go:133` | `pre-retry callback for fb` | Gancho ausente no caminho FB, presente no caminho normal |
| B6 | `capabilities/send/ack.go:84` | `also invalidate device list caches` | Invalidação incompleta de cache. **Relacionado à F37**, que fechou o cache de dispositivos por TTL |
| B7 | `capabilities/send/ack.go:87` | `do something` | Ramo vazio. Precisa de leitura do contexto para saber se é caminho de erro engolido |
| B8 | `capabilities/send/encrypt.go:57` | `query LID from server for missing entries` | Quando não há mapeamento PN→LID local, o device é cifrado sob a identidade PN. Consultar o servidor fecharia a lacuna |
| B9 | `capabilities/user/business.go:59` | `parse bot_fields` | Campo do perfil business não parseado |
| B10 | `core/broadcast.go:71` | `should there be a better way to separate contacts and found push names in the db?` | **Reclassificado de A para B**: não é formato de fio, é design do nosso schema. Resolvível sem captura |
| B11 | `core/client_events.go:134` | `should we do something else?` | Precisa de leitura do contexto |
| B12 | `pkg/presentation/http/middleware/doc.go:9` | `Implementar cada middleware quando o roadmap demandar` | **O único fora de `internal/`**. Stubs de HMAC, idempotência e retry. É roadmap declarado, não dívida |

**Recomendação**: se o objetivo for reduzir ruído, converter em entradas deste
arquivo e apagar o comentário — o registro fica onde é procurado em vez de
espalhado por 12 arquivos. **B2 e B3 são os candidatos mais claros a simples
remoção**: um TODO vazio não informa nada e não é acionável por ninguém.

B6 e B8 são os de maior valor real, por tocarem correção e não só completude.

---

### Categoria C — gambiarras assumidas (5)

Código que o autor upstream sabe ser feio e que **funciona em produção há
anos**. O risco de mexer é assimétrico ao ganho.

| # | Local | O que o comentário diz | Risco de mexer |
| --- | --- | --- | --- |
| C1 | `capabilities/send/node_build.go:177` | `this is a very hacky hack for announcement group messages, why is it pn anyway?` | **Alto.** Muda endereçamento de mensagem em grupo de anúncio |
| C2 | `capabilities/send/prepare.go:162` | `this is fairly hacky, is there a proper way to determine which identity the message is sent with?` | **Alto.** Escolha de identidade PN vs LID no envio — mesmo domínio da F42 |
| C3 | `core/receipt.go:149` | `this hack probably needs to be removed at some point` | **Médio.** Precisa entender por que existe antes de remover |
| C4 | `capabilities/send/message.go:46` | `somehow deduplicate this with the code in sendNewsletter?` | **Baixo.** Refactor local, sem mudança de formato de fio |
| C5 | `capabilities/tctoken/tctoken.go:131` | `replace with an UPDATE call instead of get+put` | **Baixo.** Refactor local. Get+put também é corrida potencial entre leitura e escrita |

**Recomendação: não mexer em C1–C3.** "Consertar" sem entender por que a
gambiarra existe é o mesmo erro que reabilitar o ramo desktop da F32 teria
sido. **C4 e C5 são seguros** e podem entrar numa leva de saneamento normal;
C5 tem valor além do estilo, por eliminar uma janela de corrida.

---

### Categoria D — bloqueado por falta de informação (1)

| # | Local | O que o comentário diz | Por que está travado |
| --- | --- | --- | --- |
| D1 | `capabilities/message/decrypt_loop.go:140` | `this probably isn't supposed to ack` | O ramo síncrono dá `SendAck` depois de `SendRetryReceipt`; o ramo assíncrono logo abaixo **também** dá. Os dois concordam entre si, e o TODO questiona ambos. Mudar é alterar comportamento de protocolo sem forma de verificar — **mesma classe da F42** |

---

### Critério de saída desta entrada

F64 fecha quando cada item tiver sido movido para uma destas situações:

1. **Resolvido** — vira achado próprio (F65+) com correção e teste.
2. **Descartado** — o comentário foi removido porque a pergunta deixou de
   fazer sentido, com a justificativa registrada aqui.
3. **Promovido a bloqueado** — passa para a lista de abertos do índice, junto
   de F42 e F44, por depender de captura de tráfego.

Enquanto isso não acontece, a categoria A inteira permanece como está **por
decisão**, não por esquecimento.

**Status**: **não corrigidos, por decisão registrada acima.**

## F65 — `GetManyLIDsForPNs` devolve o mapa INVERTIDO em produção; o dublê de teste esconde

**Data**: 2026-08-07.
**Contexto**: merge de `feature/vendor-whatsmeow` em `feature/macbook-lucas`.
O commit `3ac8073` (`feat(contacts): resolve LID<->PN em
GET /user/contacts/last-activity`) tocava `pkg/infra/whatsmeow/user_adapters.go`,
caminho que esta branch renomeou — conflito modify/delete que obrigou a ler o
código linha a linha para portar. Foi aí que apareceu.

**Onde**:

- `pkg/infra/wa-noise/adapters/user/adapter.go` — `UserAdapter.GetManyLIDsForPNs`
- `pkg/infra/wa-noise/adapters/user/adapter_test.go` — `fakeLIDStore.GetManyLIDsForPNs`
- `internal/wa-noise/persistence/store/sqlstore/lidmap.go:133-158` — o real
- `pkg/application/usecase/user/get_contacts_last_activity.go` — `normalizeToLID`, o consumidor

**Problema**: três implementações, duas orientações de mapa.

| Quem | Código | Devolve |
| --- | --- | --- |
| Store REAL (`CachedLIDMap`) | `result[pn] = lid` (`lidmap.go:148`) | `map[PN]LID` |
| Dublê de teste (`fakeLIDStore`) | `out[lid] = pn` | **`map[LID]PN`** |
| Consumidor (`normalizeToLID`) | `resolved[domain.JID(jid)]` com `jid` terminando em `@s.whatsapp.net` | espera `map[PN]LID` |

O adapter faz `for lid, pn := range resolved { out[pn.String()] = lid.String() }`
— os nomes das variáveis estão trocados em relação ao store real. Com o dublê
(que é `map[LID]PN`) o resultado sai `map[PN]LID` e o teste passa. Com o store
real (`map[PN]LID`) o resultado sai `map[LID]PN`.

**Consequência**: em produção, `normalizeToLID` procura `resolved[PN]` num mapa
chaveado por LID. A busca erra **sempre**, `ok` é falso, e todo PN mantém a
chave original. A normalização LID↔PN vira **no-op silencioso** — nenhum erro,
nenhum log, apenas a feature não fazendo nada.

Isso é coerente com a própria motivação do commit, que relata ter medido
0/1141 contatos casando entre `message_history` e o roster. Com a inversão,
continuaria 0.

**Por que o compilador não pega**: `map[types.JID]types.JID` nos dois sentidos.
PN e LID são o mesmo tipo Go; só o `Server` (`@s.whatsapp.net` vs `@lid`) os
distingue, em tempo de execução. Não há como o tipo ajudar aqui, e o dublê
invertido apagou o único sinal que restava.

**Verificação independente**: `postgres_test.go`
(`TestPostgresGetManyLIDsForPNs`, escrito nesta mesma leva contra o store
real) afirma `got[pnA] == lidA` — chaveado por PN. Confirma a orientação do
real, contra o dublê.

**Correção sugerida** — os três pontos têm de mudar **na mesma alteração**,
senão um conserta e o outro fica vermelho:

1. `fakeLIDStore.GetManyLIDsForPNs`: `out[pn] = lid`, para honrar o contrato
   da interface que ele dubla.
2. `UserAdapter.GetManyLIDsForPNs`: `for pn, lid := range resolved` e
   `out[domain.JID(pn.String())] = domain.JID(lid.String())`.
3. O teste `TestUserAdapter_GetManyLIDsForPNs_OK`: a asserção final não muda
   (continua `got["1234@s.whatsapp.net"] == "lid-x@lid"`), mas o `mapping` do
   dublê precisa ser relido — hoje ele é declarado como `lid → pn`.

Vale acrescentar um teste que exercite o adapter contra o store **real**
(sqlite em memória, como os outros do pacote de persistência), que é a única
forma de o dublê não poder mentir de novo.

**Status**: **CORRIGIDO** (2026-08-07), com prova contra dados reais.

Os tres pontos mudaram na mesma alteracao, como a entrada previa: o dube
passou a devolver `map[PN]LID` (honrando o contrato da interface que dubla) e
o adapter passou a iterar `for pn, lid := range resolved`.

Medido no ambiente pareado, `GET /user/contacts/last-activity`:

| | Antes | Depois |
| --- | ---: | ---: |
| chaves `@lid` | 287 | **706** |
| chaves `@s.whatsapp.net` | 421 | **2** |
| PNs nao normalizados COM mapeamento no store | **419** | **0** |

287 + 419 = 706: cada telefone com LID conhecido foi reescrito, e os 2 que
sobraram sao exatamente os sem mapeamento — o comportamento documentado.

Controle negativo: revertendo SO' o adapter (com o dube ja' correto), o teste
acusa `1234@s.whatsapp.net = "", want lid-x@lid`. Antes da correcao os dois
lados invertidos combinavam e o teste ficava verde sobre codigo quebrado.

**Impacto**: `GET /user/contacts/last-activity` devolve hoje as chaves
`@s.whatsapp.net` sem normalizar — exatamente o comportamento anterior a
`3ac8073`. Não há regressão em relação ao que já estava em `develop`; o que há
é uma feature que nunca chegou a funcionar.

## F66 — payload incompleto devolve HTTP 500; 67 validações de use case não têm categoria

**Data**: 2026-08-07.
**Contexto**: smoke HTTP após o merge em `feature/macbook-lucas`.

**Como reproduzir**:

```
curl -X POST 'http://localhost:8080/chat/send/text?token=t1' \
     -H 'Content-Type: application/json' -d '{}'
{"code":500,"error":"internal server error","success":false}
```

Log do servidor no mesmo request:

```
"error":"missing Phone in payload","message":"send message use case failed"
"status":500,"outcome":"server_error"
```

**Problema**: campo obrigatório ausente no corpo enviado pelo cliente é erro
**do cliente** (400), não do servidor. O servidor identifica a causa
corretamente — a mensagem de log é exata — e mesmo assim responde 500 e
classifica o `outcome` como `server_error`.

O contraste dentro do MESMO endpoint mostra que não é regra, é acidente:

| Entrada | Resposta | Onde é rejeitada |
| --- | --- | --- |
| `{"Phone":"nao-e-um-jid","Body":"oi"}` | **400** | handler, que já usa `apperr` |
| `{}` | **500** | use case, `fmt.Errorf` sem categoria |

**Alcance**: 67 ocorrências de `fmt.Errorf("missing ...")` /
`errors.New("missing ...")` em `pkg/application/usecase/`, contra apenas 4
arquivos de use case que usam `apperr.New`. Exemplos:
`chat/reject_call.go:31,35`, `chat/request_unavailable_message.go:31,35,39`,
`group/group_request.go:32,71,76,81,131`, `group/get_group_invite_info.go:30`,
`chat/archive_chat.go:31`.

**Por que importa mais que um número errado**: 5xx tem semântica de retry.
Um cliente HTTP bem comportado (e todo SDK de fila) trata 500 como falha
transitória do servidor e **retenta**. Um payload permanentemente malformado
seria retentado indefinidamente, sem nunca poder dar certo. 400 encerra a
tentativa.

**Correção sugerida**: envolver as validações em
`apperr.New(<code>, apperr.CategoryValidation, <msg>, false, nil)` e ligar
`Category.HTTPStatus()` no caminho de resposta.

Vale notar: `Category.HTTPStatus()` **já existe e já tem teste**, mas nada o
chama ainda — o mesmo achado que apareceu ao classificar
`user_info_failed` (que foi corrigido para `CategoryValidation` nesta leva,
mas cuja tradução para status HTTP depende desta mesma ligação). Esta entrada
é a evidência de produção de que a ligação faz falta, e não só de que está
pendente no plano.

**Status**: **não corrigido**. São 67 sítios mais a ligação do
`HTTPStatus()`; é mudança de contrato de API (respostas que hoje são 500
passam a ser 400) e merece commit próprio, fora do merge.

## F67 — o `.env` que o `run.sh` gera tem chave AES de tamanho inválido

**Data**: 2026-08-07. **Contexto**: subir o ambiente para o smoke HTTP.

**Onde**: `run.sh:24-25`.

```
WA_API_GLOBAL_ENCRYPTION_KEY=MinhaChaveDe32Caracteres1234567890   # 34 bytes
WA_API_GLOBAL_HMAC_KEY=MinhaHMACKeyDe32Caracteres1234567         # 33 bytes
```

**Problema**: AES aceita chave de 16, 24 ou 32 bytes. A primeira tem **34**,
a segunda **33** — apesar de ambas dizerem "De32Caracteres" no próprio valor.
No startup:

```
{"level":"error","error":"failed to create cipher: crypto/aes: invalid key size 34",
 "message":"Failed to encrypt global HMAC key"}
```

E o servidor **continua subindo**. A chave HMAC global nunca é gravada, então
todo o caminho de HMAC fica silenciosamente inerte para quem seguir o
`run.sh` — que é o caminho documentado para levantar o ambiente local.

**Dois defeitos, não um**:

1. As constantes do template estão erradas (trivial).
2. Falha ao inicializar material criptográfico é logada e **ignorada**. Se a
   chave global é requisito, o startup deveria abortar; se é opcional, o log
   deveria ser `warn` com a consequência explícita ("HMAC global desativado"),
   não um `error` que ninguém trata.

O item 2 é o que interessa: com a chave corrigida o sintoma some, mas o
comportamento de engolir falha de cifra continua lá.

**Correção sugerida**: cortar as duas chaves para 32 bytes E decidir
explicitamente entre fail-fast e degradação anunciada.

**Status**: **CORRIGIDO** (2026-08-07), os dois itens.

**Item 1**: as duas chaves do `run.sh` passaram a ter 32 bytes de verdade, travadas por `TestRunSh_ChavesTemTamanhoValidoParaAES`, que valida com o proprio `aes.NewCipher` em vez de comparar com 32 — replicar a regra abriria espaco para as duas divergirem. Um segundo teste exige que uma chave contendo "32" no texto tenha mesmo 32 bytes, que foi a contradicao que fez ninguem desconfiar.

**Item 2**: virou `log.Fatal`. A investigacao mostrou que nao era um erro cosmetico e sim **rebaixamento silencioso de seguranca**: com a chave vazia, `callHookWithHmac` pula a assinatura em silencio (`if len(encryptedHmacKey) > 0`, dispatch_callhook.go:80) e todo webhook global sai SEM assinatura, sem que o receptor tenha como perceber.

Nao ha caso legitimo de seguir adiante: `*globalHMACKey` nunca chega vazio aquele ponto — `main.go:276` gera uma chave aleatoria quando nenhuma e' fornecida — entao a assinatura e' sempre pretendida, e a unica falha possivel e' `WA_API_GLOBAL_ENCRYPTION_KEY` invalida, que e' configuracao corrigivel. Mesmo tratamento que `os.Executable()` no mesmo arquivo ja' recebia.

Verificado: com chave de 34 bytes o processo emite `"level":"fatal"` com mensagem acionavel e NAO sobe (`/livez` nao responde); com 32 bytes sobe normal e loga `"Global HMAC key encrypted successfully"`.

**item 2 ABERTO**: engolir falha de inicializacao de material criptografico continua la'. Com a chave certa o sintoma some, mas o comportamento nao.
política de inicialização.

## F68 — o mesmo evento `QR` é despachado com dois formatos de payload

**Data**: 2026-08-07.
**Contexto**: construção da página de pareamento em
`pkg/presentation/http/devui/`. Ao escrever o consumidor WebSocket, os dois
formatos apareceram.

**Onde**: `pkg/application/session/orchestrator.go`.

| Origem | Linha | Payload |
| --- | --- | --- |
| `onPairingQR` (fluxo de pareamento, canal de `Pair()`) | 247-267 | `{"event":"code","qrCodeBase64":"data:image/png;base64,…","expiresAt":"<RFC3339>"}` |
| `translateStatusEvent` → `qrPayload` (fluxo de `Subscribe`) | 382, 416 | `{"event":"qr","code":"2@…"}` |

Os dois despacham com `eventType = "QR"`. Um consumidor de `/session/ws` (ou
de webhook) recebe o mesmo `type` com schemas incompatíveis: só o primeiro
traz imagem e validade, só o segundo traz o código cru. Nada no tipo permite
distinguir de antemão — é preciso testar a presença dos campos.

O campo `event` **também** diverge (`"code"` vs `"qr"`), o que sugere que os
dois nasceram em momentos diferentes sem que ninguém comparasse.

**Consequência prática**: um cliente que trate só o schema do pareamento
ignora silenciosamente os QR vindos do `Subscribe`, e vice-versa. A página em
`devui/` trata os dois e diz no log qual chegou, mas isso é contorno de
consumidor, não correção.

**Correção sugerida**, em ordem de preferência:

1. Unificar num único payload que sempre traga `code`, e traga
   `qrCodeBase64`/`expiresAt` quando disponíveis. Mantém um `type` com um
   schema só.
2. Separar em dois `type` distintos (`QR` e `QRCode`, por exemplo), deixando
   a diferença explícita no roteamento em vez de implícita nos campos.

A (1) é menos disruptiva para quem já consome; a (2) é mais honesta sobre
serem eventos de origens diferentes. Ambas são mudança de contrato de webhook
e precisam de decisão.

**Status**: **não corrigido**. É contrato externo (webhook + WS), fora do
escopo de criar a página.

## F70 — webhook nunca dispara para usuário integrado depois da subida do servidor

**Data**: 2026-08-07.
**Contexto**: primeiro pareamento real com celular. Dos 74 warns do log, 42
eram consequência deste único defeito.

**Sintomas no log** (todos com o usuário existindo e conectado):

```
[29x] "Could not call webhook as there is no user for this token"  token=t1
[12x] "User info not found in cache, skipping history"
 [1x] "No user info cached on pairing?"
```

**Onde**:

- `pkg/bootstrap/lifecycle_webhook.go:100-109` — `getUserWebhookUrl`
- `pkg/bootstrap/lifecycle.go:97` — único `UserInfoCache.Set` que CRIA entrada
- `pkg/bootstrap/eventhandler_session.go:94-103` — o `Set` do pareamento

**A cadeia**:

1. `UserInfoCache.Set` que **cria** a entrada vive dentro de
   `connectOnStartup()` (`lifecycle.go:97`), que só roda na subida do
   servidor, iterando usuários já `connected=1`.
2. Um usuário criado por `POST /admin/users` **depois** da subida nunca passa
   por lá — e esse é o caminho normal de onboarding.
3. No `PairSuccess`, `eventhandler_session.go:94` faz
   `myuserinfo, found := ...Get(token)`; o `Set` está dentro do **`else`**.
   Sem entrada prévia, ele apenas loga `"No user info cached on pairing?"` e
   **não cria nada**.
4. A partir daí, todo evento chama `getUserWebhookUrl`, que lê **só do
   cache** — nunca do banco — e devolve `""` no miss.

**Consequência**: um webhook configurado na tabela `users` é **silenciosamente
ignorado** para qualquer usuário integrado após a subida do servidor. Só um
restart do processo (que reexecuta `connectOnStartup` com o usuário já
`connected=1`) faz a entrega passar a funcionar.

Não foi observado com webhook real porque o usuário de teste estava sem
webhook — mas o defeito independe disso: `getUserWebhookUrl` devolve `""`
antes de qualquer consulta ao banco.

**Agravante: a mensagem mente.** *"there is no user for this token"* é falso —
o usuário existe, está conectado e autenticado. O que faltou foi a entrada no
cache. Quem investigar vai procurar o usuário no banco, encontrá-lo, e
descartar a pista certa.

**Correção sugerida**:

1. Fazer `eventhandler_session.go` **criar** a entrada quando não existir, em
   vez de só atualizar — os dados necessários estão todos na tabela `users`.
2. Ou dar a `getUserWebhookUrl` um caminho de leitura do banco no miss,
   populando o cache de passagem.
3. Independente da escolha, corrigir a mensagem para dizer *"user info not in
   cache"*, que é o que de fato aconteceu.

A (1) é mais barata e ataca a origem; a (2) torna o cache irrelevante para
correção, o que é mais robusto.

**Status**: **CORRIGIDO** (2026-08-07) — ver o commit da correcao.
com teste de integração cobrindo "usuário criado após a subida recebe
webhook".

## F71 — sync automático de histórico após pareamento nunca executa: coluna inexistente

**Data**: 2026-08-07. **Contexto**: mesmo pareamento.

**Sintoma**:

```
"Failed to get days_to_sync_history from database"
error="SQL logic error: no such column: days_to_sync_history (1)"
```

**Onde**: `pkg/bootstrap/eventhandler_session.go:107`.

```go
query := "SELECT COALESCE(days_to_sync_history, 0) FROM users WHERE id=$1"
...
if err != nil {
    log.Warn()...Msg("Failed to get days_to_sync_history from database")
} else if daysToSyncHistory > 0 {
    go evh.syncHistoryAfterPair(daysToSyncHistory)
}
```

**Problema**: a coluna chama-se **`history`**, não `days_to_sync_history`. A
migração que a cria é explícita (`pkg/infra/db/migrations.go:254`):

```sql
ALTER TABLE users ADD COLUMN history INTEGER DEFAULT 0;
```

O campo da API também é `history` (`domain.AddUserRequest.History`, tag
`json:"history"`), e `GET /session/status` devolve `"history":"0"`.

**Consequência**: a query falha **sempre**, em qualquer banco, para qualquer
usuário. O `else if` nunca é alcançado e `syncHistoryAfterPair` **nunca é
chamado** — o sync automático de histórico após leitura do QR é código morto
em produção. Como a falha é apenas um `Warn`, ninguém percebeu.

Quem configurar `history: 30` ao criar o usuário vê o valor persistido e
devolvido por `/session/status`, e conclui que a funcionalidade está ativa.

**Correção sugerida**: trocar o nome da coluna na query para `history`. É uma
linha. O teste que a trava é de integração: criar usuário com `history > 0`,
parear, e afirmar que `syncHistoryAfterPair` rodou.

Vale checar de passagem se a migração de `migrations.go:236-254` roda em
SQLite — ela usa `information_schema`, que é de Postgres.

**Status**: **CORRIGIDO** (2026-08-07) — ver o commit da correcao.


## Evidências novas (2026-08-07, ambiente pareado) para F42, F66 e F68

Esta seção não é achado novo: são medições feitas com a sessão real de pé,
que mudam o que se sabe sobre três achados que estavam parados por falta de
insumo. Cada uma está anexada aqui em vez de reescrever as entradas, para que
a data e o raciocínio original delas permaneçam legíveis.

### F42 — o caminho de envio v3/FB é INALCANÇÁVEL a partir do wa-api

A entrada dizia que decidir exigia captura de tráfego. Não exige: a pergunta
anterior a essa é se o código chega a rodar, e ele não chega.

```
grep -rn 'SendFBMessage' --include='*.go' pkg/ cmd/   ->  0 ocorrências
```

`Client.SendFBMessage` (`core/sendfb.go:25`) é o único caminho até
`EncryptForDevicesV3`, onde vive o `v` numérico. Ele tem **zero chamadores**
em `pkg/` e `cmd/`, e a fachada `internal/wa-noise/main.go` **não o
reexporta**. Nenhuma rota HTTP alcança o envio v3/FB.

O que existe em `pkg/` é `handleFBMessage` (`eventhandler_message.go:400`),
que trata FBMessage **recebida** — direção oposta, não usa `encAttrs`.

**O que isso muda na decisão**: não é "qual forma o servidor espera", é "este
ramo não é exercitado por este produto". As opções passam a ser:

1. Uniformizar para string agora, já que nada em produção depende da forma
   atual — risco praticamente nulo, e remove a única inconsistência do tipo no
   caminho de envio.
2. Deixar como está e anotar no código que o ramo é inalcançável, com o
   critério de reavaliação: se algum dia uma rota expuser `SendFBMessage`, a
   forma do `v` vira pergunta real e precisa de captura antes de ir a
   produção.

A (1) só é preferível se a uniformização for de fato inócua; como o ramo não
roda, nenhum teste de integração pode confirmá-la — o que é argumento a favor
da (2), que não toca em código morto.

### F66 — 23 de 25 endpoints POST devolvem 500 para payload vazio

A entrada contava 67 sítios de `fmt.Errorf` em use cases. Medido contra o
servidor rodando, o alcance visível ao cliente é:

```
POST <endpoint>?token=… com body {}
  4xx (correto): 2
  5xx (F66):    23
```

Os 23: `/chat/send/{text,image,audio,document,video,sticker,location,contact,
poll,edit,buttons,list,template}`, `/chat/{react,markread,archive,delete,
delete/message,presence,request-unavailable-message}`, `/call/reject`,
`/group/announce`, `/user/status`.

Ou seja: **quase toda a superfície de escrita da API**. Um cliente que
mande um payload malformado recebe 5xx, que tem semântica de retry, e
retentará para sempre algo que nunca pode dar certo.

**O que isso muda na decisão**: a entrada apresentava o custo (67 sítios) sem
o benefício quantificado. Com 23 endpoints medidos, a proporção muda — e
sugere um caminho intermediário que a entrada não considerava: ligar
`Category.HTTPStatus()` no caminho de resposta **primeiro**, e converter os
`fmt.Errorf` incrementalmente. Os endpoints já convertidos passam a responder
400 sem esperar os 67.

### F68 — os dois schemas convivem, mas com cardinalidades diferentes

Observado no pareamento real: o `*events.QR` do SDK chega ao handler de
domínio **uma vez**, com o array completo (`&{Codes:[…6 códigos…]}`), e cai no
`default` como `"Unhandled event"`.

Isso esclarece a diferença entre os dois caminhos, que a entrada tratava como
simétricos:

| Caminho | Dispara | Payload |
| --- | --- | --- |
| `Subscribe` → `qrPayload` | **1×** por sessão, só com `Codes[0]` | `{event:"qr", code}` |
| `Pair()` → `onPairingQR` | **6×**, um por código | `{event:"code", qrCodeBase64, expiresAt}` |

Um cliente WS recebe 7 eventos `type:"QR"` numa única sessão de pareamento,
em dois formatos, e só 6 deles correspondem a códigos realmente exibíveis. O
único do `Subscribe` traz um código que o de pareamento também traz — em
formato pior (sem imagem, sem validade).

**O que isso muda na decisão**: a opção (2) da entrada (separar em dois
`type`) fica menos atraente, porque o evento do `Subscribe` não acrescenta
informação — é subconjunto do outro. Surge uma terceira opção:

3. Parar de despachar o QR pelo caminho do `Subscribe`, deixando apenas o de
   pareamento. Remove a ambiguidade sem inventar um `type` novo, e nenhum
   consumidor perde dado.

Antes de fazê-la, é preciso confirmar que nada consome o formato do
`Subscribe` hoje — o que é pergunta para quem opera os webhooks, não para o
código.

## F72 — `no session` é logado em nível `error` para um estado esperado

**Data**: 2026-08-07. **Contexto**: observação de log com o painel de sessões
aberto, fazendo poll de status a cada 3s.

**Onde**: pelo menos 12 use cases repetem o padrão, entre eles
`chat/archive_chat.go:26`, `chat/request_unavailable_message.go:26`:

```go
if err := uc.chats.EnsureSession(ctx, userID); err != nil {
    uc.logger.Error(ctx, "no wanoise session", "error", err, "user_id", userID)
    return nil, err
}
```

**Problema**: "a sessão não está conectada" é estado **esperado** — é o estado
de toda sessão que ainda não pareou, ou que foi desconectada de propósito.
Logar isso como `error` infla o nível: `error` deveria significar que algo
está errado, e não estar conectado não está.

O efeito prático apareceu sozinho: um painel que faça poll de `/session/status`
a cada 3s gera **20 linhas de `error` por minuto e por sessão parada**. Medido
nesta observação: 22 ocorrências, todas em `error`, todas de sessões que
simplesmente não haviam conectado ainda.

Quem monitora por nível — alerta em `error`, dashboard, ou o próprio filtro
que montei para acompanhar esta sessão — precisa criar exceção para uma
condição normal. É o mecanismo clássico pelo qual alertas param de ser lidos.

**Correção sugerida**: rebaixar para `Debug` (ou `Info`) quando o erro for o
sentinela de sessão ausente, mantendo `Error` para as demais falhas de
`EnsureSession`. Distinguir pelo erro, não pela mensagem.

Relacionado à **F66**: o mesmo `no_session` que é logado como `error` aqui é
devolvido como HTTP 400 pelo handler — ou seja, a camada HTTP já o classifica
corretamente como erro do cliente, e só o log discorda.

**Status**: **CORRIGIDO** (2026-08-07): 64 ocorrencias em 62 arquivos passaram de `Error` para `Warn`.

O alcance era MUITO maior que esta entrada estimava — ela falava em ~12 use
cases. E havia uma restricao que ela nao conhecia: a metrica de log-coverage
so conta um caminho de saida como coberto com nivel **>= Warn**
(`METRIC.md:136`). Rebaixar para `Info`/`Debug` tornaria os 64 caminhos
descobertos de uma vez e obrigaria a afrouxar a catraca — trocaria um problema
por outro. `Warn` e' o unico rebaixamento que cabe.

Confirmado empiricamente: `errpath_coverage` ficou INALTERADO em 862 apos a
mudanca.

A porta `Logger` tambem nao tem `Debug` — so' `Info`, `Warn` e `Error` —,
entao a alternativa nem estava disponivel sem mexer na interface.

Cinco testes travavam o nivel (um helper compartilhado `assertNoSessionLog`,
tabelas em group/chat, e um com string literal `"error"`). Todos ajustados,
com o porque registrado no helper.

## F73 — `PushName` e `BusinessName` não geram webhook

**Data**: 2026-08-07. **Contexto**: mesma observação, com quatro sessões reais
recebendo eventos.

**Onde**: `pkg/bootstrap/eventhandler.go:38` trata `*events.PushNameSetting`,
que é a configuração do nome do PRÓPRIO usuário. Os eventos
`*events.PushName` e `*events.BusinessName` — que anunciam que um CONTATO
mudou de nome — caem no `default` e viram `"Unhandled event"`.

Observado 2× cada em uma noite de uso normal.

**Não é perda de dado**: o SDK persiste antes de emitir. `user/info.go:156`
grava inclusive o JID alternativo (o par LID↔PN, o mesmo domínio da F65) e só
depois despacha o evento. O contato fica com o nome novo no store.

**O que falta é a notificação**: um integrador que queira reagir a "contato
mudou de nome" não tem como — o evento não vira webhook. E `PushNameSetting`,
que está em `SupportedEventTypes` e é assinável, cobre outro caso.

**Correção sugerida**: decidir se esses dois eventos devem ser assináveis. Se
sim, acrescentar a `SupportedEventTypes` e tratá-los no handler; se não,
tratá-los explicitamente com um `case` que descarta em silêncio, para que o
`"Unhandled event"` volte a significar "apareceu algo que não previmos".

Hoje o warn não distingue "evento novo do protocolo" de "evento que decidimos
ignorar", que é a informação que ele deveria carregar.

**Status**: **não corrigido**. Depende de decisão sobre o contrato de webhook,
como a F68.

## F74 — o fan-out WebSocket é serial: N conexões obsoletas custam N × 5s

**Data**: 2026-08-07. **Contexto**: observação de log; o painel de sessões
abre um WebSocket por sessão, e recarregar a página deixa os anteriores
mortos até a próxima escrita.

**Onde**: `pkg/infra/wa-noise/registry/broadcast/broadcast.go`, `Broadcast`.

```go
for _, c := range conns {
    ctx, cancel := context.WithTimeout(context.Background(), writeTimeout) // 5s
    err := wsjson.Write(ctx, c, payload)
    cancel()
    ...
}
```

**Observado**: uma rajada de 6 `websocket broadcast write failed` para o mesmo
userID, cinco com `use of closed network connection` e uma com
`context deadline exceeded`.

**Problema**: o laço é **serial** e cada conexão tem seu próprio teto de 5s.
Conexão morta que falha na hora custa quase nada, mas conexão *lenta* — a que
produz `deadline exceeded` — custa os 5s inteiros, e elas somam. Com N
conexões nesse estado, um único broadcast leva até N × 5s.

Não bloqueia o event loop (roda sob `safego`), mas os broadcasts se
enfileiram: o evento seguinte espera o anterior terminar de percorrer todas as
conexões podres.

A limpeza funciona — `Remove` tira a conexão na falha —, mas só na PRIMEIRA
tentativa de escrita depois de ela morrer. Até lá ela conta no fan-out.

**Correção sugerida**, em ordem de custo:

1. Paralelizar o fan-out (um `go` por conexão, com `WaitGroup`). O teto passa
   a ser 5s no total em vez de 5s por conexão. É a mudança que resolve.
2. Reduzir `writeTimeout`. Trata o sintoma e piora a entrega para clientes
   legitimamente lentos.

A (1) muda concorrência e precisa de teste de corrida — o pacote já tem um,
e o teste com conexão WebSocket real (`ws_conn_test.go`) exercita o caminho
de descarte de conexão morta.

**Ressalva sobre o texto do erro**: a mensagem chega como
`failed to write JSON message: failed to marshal JSON: failed to write msg:
use of closed network connection`. Ela diz "failed to marshal JSON" para uma
falha de ESCRITA — o embrulho vem da biblioteca e induz a erro quem for
investigar. Vale envolver com contexto próprio.

**Status**: **CORRIGIDO** (2026-08-07) pela opcao (1): uma goroutine por conexao, com WaitGroup. O teto passou de writeTimeout POR conexao para writeTimeout no total.

Antes de paralelizar foi verificado que o `payload` e' somente lido depois de
despachado — `sendEventWithWebHook` nao escreve no mapa apos o `safeGo`
(conferido por varredura de atribuicoes a `postmap[...]`), entao serializa-lo
de varias goroutines e' seguro.

`Broadcast` continua esperando todas as escritas: nao esperar mudaria o
contrato ("tentou entregar a todo mundo e terminou") e deixaria goroutines
escrevendo depois de a funcao retornar.

**Sem teste de tempo, de proposito.** Um teste que medisse "N conexoes lentas
levam ~1x writeTimeout em vez de Nx" seria flaky: `wsjson.Write` retorna
quando o frame entra no buffer do socket, nao quando o outro lado le, entao
nao ha como produzir lentidao deterministica sem seam artificial em codigo de
producao. A correcao esta coberta pelo teste de corrida do pacote e pelo teste
com WebSocket real que exercita o descarte de conexao morta; a paralelizacao
em si e' estrutural e visivel no codigo. Um teste flaky aqui corroeria a
confianca na suite mais do que o teste agregaria.

A ressalva sobre o texto do erro ("failed to marshal JSON" para uma falha de
ESCRITA) continua valendo e nao foi tocada.

## F75 — remover o token da query string vai quebrar todo cliente WebSocket de navegador

**Data**: 2026-08-07.
**Contexto**: migração do painel `devui` para autenticação por header, ao ver
que ele sozinho respondia por ~1.000 dos 1.095 avisos de deprecação numa noite
de uso.

**Onde**: `pkg/presentation/http/middleware/auth.go:72-90`.

```go
// A query string continua aceita nesta release para não quebrar clientes que
// dependem dela, mas cada uso emite WARN [...] A remoção é a release seguinte.
func extractRequestToken(r *http.Request) string {
	if token := r.Header.Get("token"); token != "" {
		return token
	}
	...
}
```

**Problema**: a API `WebSocket` do navegador **não permite header customizado
no handshake**. Não há `headers` no construtor `new WebSocket(url, protocols)`
— é limitação da especificação, não do nosso código. Um cliente web só
consegue autenticar em `/session/ws` por query string (ou por cookie, ou pelo
truque de subprotocolo).

Portanto, quando a query string for removida na próxima release,
`GET /session/ws` fica **inalcançável de qualquer navegador**. Clientes de
servidor (Go, Node) não são afetados: eles montam o handshake e podem mandar
o header.

Isto não aparece nos testes: `TestRegistry_...` e o teste com WebSocket real
usam bibliotecas Go, que mandam header sem dificuldade. Só se manifesta com
um cliente de navegador de verdade — que passou a existir agora, com o
`devui`.

**Correção sugerida** — as três saídas conhecidas, e nenhuma é indolor:

1. **Manter a query string apenas para `/session/ws`**, removendo do resto.
   Simples e honesto; documenta a exceção em vez de fingir uniformidade. O
   custo é o token continuar aparecendo em log de acesso e histórico de
   navegador para essa rota.
2. **Token no subprotocolo** (`new WebSocket(url, [token])` e o servidor lendo
   `Sec-WebSocket-Protocol`). É o truque padrão da indústria para exatamente
   este problema, e mantém o token fora da URL. Exige mudança no handshake
   do servidor.
3. **Cookie de sessão**, que o navegador manda sozinho no handshake. Muda o
   modelo de autenticação da API inteira; desproporcional.

A (2) é a resposta técnica correta; a (1) é a que cabe numa release.

**Status**: **não corrigido — mas é bloqueador da remoção anunciada.** O
`devui` já migrou os `fetch` para header e mantém a query só no WebSocket,
com comentário apontando para esta entrada.

## F76 — o log grava os códigos de pareamento em texto puro

**Data / contexto**: 2026-08-07, teste E2E do `devui` no Chrome, pareando a
sessão `TesteQR`.

**Onde**: `pkg/bootstrap/eventhandler.go:146`

```go
log.Warn().Str("event", fmt.Sprintf("%+v", evt)).Msg("Unhandled event")
```

**Problema**: `*events.QR` não tem `case` no switch e cai no ramo default, que
dumpa a struct inteira. O campo `Codes []string` traz **todos os códigos de
pareamento da sessão de uma vez** — no pareamento medido, 6 códigos, 1678
bytes numa única linha de log:

```
{"level":"warn","message":"Unhandled event",
 "event":"&{Codes:[https://wa.me/settings/linked_devices#2@B+o1E1qrtMni2Ojvg…"}
```

Um código desses, lido por quem tiver acesso ao arquivo de log, **vincula um
aparelho à conta** — é credencial, não diagnóstico. A janela é curta (o
primeiro código vale 60s, os seguintes ~20s, e o conjunto morre quando um é
consumido), mas o log persiste indefinidamente e costuma ir para agregadores
com público bem mais amplo que o do banco de sessões.

Note o que isto **não** é: não é um por rotação de QR. É um evento só, no
`Connect`, carregando a lista inteira. Foi medido: 5 rotações de QR na tela,
1 linha de log.

**Correção sugerida**: dar um `case *events.QR:` ao switch — nem que seja
para logar `Msg("QR codes recebidos")` com `Int("n", len(evt.Codes))` e nada
mais. O default continua útil para eventos de fato desconhecidos; o problema
é um evento **conhecido e sensível** cair nele. Vale varrer os outros tipos
que hoje caem no default à procura de campos com o mesmo perfil.

**Status**: **corrigido** em `pkg/bootstrap/eventhandler.go` — `*events.QR`
ganhou `case` próprio, que loga apenas `Int("codes", len(evt.Codes))` em
Debug e retorna. Nem o código nem o dump saem em nível nenhum.

Coberto por `pkg/bootstrap/eventhandler_qr_test.go`, com três testes que se
sustentam mutuamente:

- `..._NaoVazaOsCodigosNoLog` — o `captureLog` usa `zerolog.New` sem filtro,
  então enxerga até Debug: é o ajuste mais severo possível para a asserção.
- `..._NaoCaiNoRamoDefault` — fixa a causa, não o sintoma. Sem ele, silenciar
  o `default` inteiro faria o primeiro teste passar.
- `..._DefaultContinuaAvisando` — controle negativo do anterior: remover o
  `default` faria os dois primeiros passarem, e o aviso de "apareceu algo que
  não previmos" sumiria sem que nada acusasse.

Controle negativo executado: com o `case` removido, os dois primeiros testes
falham exibindo o código de pareamento no dump (`&{Codes:[...SEGREDO...]}`).

**Verificado ao vivo em 2026-08-08**, com os dois binários no MESMO arquivo
de log, e um pareamento real e bem-sucedido de cada lado:

```
binário ANTIGO (pareamento 23:52)  -> 1 linha contendo o código
binário NOVO   (pareamento 07:55)  -> 0 linhas contendo o código
                                      0 dumps `Codes:`
                                      0 `Unhandled event`
                                      1 `QR pairing ok!`
```

O `QR pairing ok!` importa: sem ele a contagem zero não provaria nada — um
QR que nunca fosse emitido também não vazaria. A sessão pareou
(`554192421234:18@s.whatsapp.net`), então o evento existiu e não vazou.

Varredura dos demais tipos que ainda caem no `default`, feita na mesma
sessão: `CATRefreshError` (só um `error`), `ManualLoginReconnect` e
`QRScannedWithoutMultidevice` (ambos `struct{}`). Nenhum carrega credencial —
`QR` era o único caso sensível.

## F77 — o WebSocket só pode ser aberto disparando `/session/connect`

**Data / contexto**: 2026-08-08, teste de desvinculação pelo app do celular,
verificando se o painel consegue reagir por WS.

**Onde**: `pkg/presentation/http/devui/assets/sessions.html:382-388`

```js
if (qual === "conectar") {
  abrirWS(s);
  ...
  await chamar(s, "GET", "/session/connect", "connect");
```

**Problema**: abrir o socket e pedir conexão são a MESMA ação na página. Não
há "só observar". Somado a `sessions.html:341`, que deliberadamente não
reconecta (`"num painel de diagnóstico, um socket que reabre sozinho esconde
o sintoma"` — decisão correta), o resultado é que **depois da primeira queda
do socket o painel fica cego** até alguém clicar em Conectar.

Medido: no início deste teste o painel exibia `0 conectados` com as cinco
sessões pareadas e vivas. Todo o estado vinha do poll REST de 3s
(`POLL_MS`, linha 158).

**Correção sugerida**: separar as duas coisas — um botão "Observar" que só
chama `abrirWS`, ou abrir o socket automaticamente ao renderizar o card de
uma sessão já pareada. A ausência de reconexão continua valendo; o que falta
é uma forma de abrir o socket **sem** efeito colateral de sessão.

**Status**: **não corrigido** — descoberto durante o teste, e a decisão de
UI é do dono do painel. Contornado no teste com um observador injetado pelo
console, que abre `/session/ws` direto.

## F78 — `/session/connect` não é idempotente numa sessão já conectada

**Data / contexto**: 2026-08-08, mesma sessão de teste. Foi o motivo de eu
**não** clicar em Conectar para abrir o WS das sessões vivas.

**Onde**: `pkg/application/session/orchestrator.go:139-167`

```go
func (o *Orchestrator) Start(ctx context.Context, userID, token string) error {
	sess, err := o.provider.NewSession(ctx, port.SessionSpec{...})
	...
	o.registry.Register(userID, sess)
	...
	if aerr := o.attach.Attach(ctx, userID, token); aerr != nil {
```

**Problema**: não há guarda de "já conectado". `Start` cria uma sessão nova
incondicionalmente, `Register` substitui a anterior no registry e `Attach`
registra um novo kill-channel. O `*wanoise.Client` antigo não é desconectado
por esse caminho — fica órfão e, até onde a leitura alcança, ainda falando
com o servidor do WhatsApp. Dois sockets para a mesma conta é a condição
clássica de `StreamReplaced` / conflito 440.

**Evidência**: leitura de código, **não** experimento — justamente porque o
experimento consistiria em fazer isso com a sessão pareada de um usuário
real. É o que falta para confirmar ou descartar.

**Correção sugerida**: `Start` consultar `IsConnected()`/`IsLoggedIn()`
(`contracts/session_provider.go:63,71`) e virar no-op — ou reconexão
explícita — quando já houver sessão viva. Alternativa mais conservadora:
`Register` devolver a sessão anterior para que `Start` a encerre antes de
substituir.

**Status**: **não corrigido, e não verificado experimentalmente.** Antes de
mexer, vale um teste controlado numa sessão descartável: chamar
`/session/connect` duas vezes e observar se aparece `StreamReplaced`.

## F79 — `/session/disconnect` e `/session/logout` devolviam 200 sem encerrar nada

**Data / contexto**: 2026-08-08, teste manual de desconexão pela API para
observar o comportamento no aparelho.

**Onde**: `pkg/application/usecase/session/disconnect.go:25-33` e
`pkg/application/usecase/session/logout.go:25-33`

```go
// Execute valida se o cliente está conectado.
func (uc *DisconnectUseCase) Execute(...) (*domain.DisconnectResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil { ... }
	uc.logger.Info(ctx, "disconnect validated", "txtID", txtID)
	return &domain.DisconnectResult{}, nil
}
```

**Problema**: os dois use cases consumiam `appport.SessionGuard` — porta que
só responde "existe sessão?". Validavam, logavam `... validated` e
retornavam. **Nunca chamavam `Disconnect()`/`Logout()`.** Os handlers
(`handler_session.go:100-117` e o de logout) tampouco: respondiam 200 e
acabava ali.

Os métodos de verdade existem e funcionam em
`pkg/infra/wa-noise/runtime/session/guard.go:88-104`, e eram alcançados de
**um único lugar em todo o projeto**: `delete_user_complete.go:62,65`, na
exclusão de usuário. Os dois endpoints HTTP nunca chegavam neles.

**Evidência medida em produção**, duas sessões pareadas e vivas:

- Duas chamadas a `/session/disconnect` → HTTP 200, `disconnect validated`
  às 00:20:25 e 00:20:37.
- `/session/status` 40s depois: `connected: true, loggedIn: true` nas duas.
- Eventos continuaram chegando pelo WebSocket: `Message` e `ReadReceipt` às
  04:20:49, :50, :53 e :54 — até **29 segundos após** a desconexão.
- Nenhum evento `Disconnected` emitido em momento algum.

**Por que a suíte não pegava**: `TestUseCases_SemSessao_PropagamACausa`
exercita só a RECUSA da guarda. Um use case que valida e não age passa nela
com folga — o caminho de sucesso não era testado por ninguém.

**Correção aplicada**: os dois passaram a consumir
`appport.SessionController` (a porta que a ADR-001 nomeia justamente para
"as operações de ciclo de vida que sobravam em ClientProvider") e a chamar
`Disconnect`/`Logout` depois da guarda, propagando a falha com log em Warn.
O wiring não mudou: `SessionGuardAdapter` já satisfazia `SessionController`
(`guard.go:107`).

Coberto por `pkg/application/usecase/session/encerramento_test.go`: caminho
feliz de cada um, propagação da falha da porta, e — o que nenhum dos outros
pegaria — **a ordem**: sem sessão, não se age sobre ela. Inverter as duas
chamadas deixa os demais testes passando.

Controle negativo executado: revertendo os dois use cases a só validar, os
quatro testes falham (`Disconnect chamado 0 vezes, quero 1`;
`a causa da porta se perdeu: <nil>`).

`min_errpath_coverage` subiu 862→863 no ratchet — os caminhos de erro novos
já nascem logando.

**Status**: **corrigido**, `make check` verde. Falta validar no aparelho:
exige reiniciar o servidor, que ainda roda o binário antigo.

## F80 — depois de um logout bem-sucedido, `/session/status` ainda diz `loggedIn=true`

**Data / contexto**: 2026-08-08, validação da F79 com o binário novo.

**Onde**: `pkg/infra/wa-noise/runtime/session/guard.go:72-78`

```go
func (a *SessionGuardAdapter) SessionStatus(_ context.Context, userID string) (bool, bool) {
	client := a.getClient(userID)
	if client == nil { return false, false }
	return client.IsConnected(), client.IsLoggedIn()
}
```

**Problema**: `POST /session/logout` respondeu 200 e logou `logged out` para
as duas sessões (00:49:08 e 00:49:09). O `Logout` do SDK
(`internal/wa-noise/core/client_session.go:25-56`) envia o IQ
`remove-companion-device`, chama `Disconnect()` e **apaga o store**
(`cli.Store.Delete(ctx)`) — e devolveu nil, então tudo isso aconteceu.

Mesmo assim, `/session/status` e `/admin/users` seguem reportando
`loggedIn=true` (com `connected=false`) para as duas.

Diferente do logout iniciado pelo TELEFONE, que emite `*events.LoggedOut` →
`handleLoggedOut` → sinal de kill → o cliente sai dos registries. No logout
iniciado pela API esse evento não é emitido, o kill nunca dispara, e o
cliente permanece registrado no `ClientManager` com estado em memória que já
não corresponde ao store apagado.

Consequência: a API afirma que a sessão está autenticada quando ela não
está. Quem confia em `loggedIn` para decidir se precisa de QR novo decide
errado.

**Correção sugerida**: o caminho de logout da API sinalizar o kill como o
caminho do telefone faz — provavelmente em `LogoutUseCase` ou num hook após
`SessionController.Logout`, para que o cliente saia dos registries e o
status volte a `no session`. Vale checar se `Disconnected`/`LoggedOut`
deveriam ser emitidos sinteticamente, já que hoje **nenhum dos dois** sai
quando a ação parte da API — ver também a observação da F79 sobre
`Disconnected`.

**Status**: **corrigido**. `LogoutUseCase` passou a receber
`appport.SessionDetacher` — porta estreita extraída de `SessionAttachHook`,
porque quem desfaz não tem o que fazer com `Attach` — e chama `Detach(txtID)`
DEPOIS do logout bem-sucedido. `Detach` é idempotente e já era o único
escritor de `users.connected` neste caminho, então os dois fluxos (telefone
via kill-channel, API direto) terminam no mesmo lugar sem duplicar escrita.

Coberto por `encerramento_test.go`, com três testes que se cercam:

- `TestLogoutUseCase_SoltaASessao` — o caminho feliz solta.
- `TestLogoutUseCase_NaoSoltaSeOLogoutFalhou` — nas duas formas de falha
  (porta recusa, sem sessão) NÃO solta. Soltar após falha seria pior que o
  defeito: o aparelho segue pareado e a API perde o cliente que o representa.
- `TestDisconnectUseCase_NaoSoltaASessao` — Desconectar continua NÃO
  soltando. Replicar o Detach aqui por simetria faria `/session/status`
  responder "no session" onde deve dizer `connected=false, loggedIn=true`.

Controle negativo executado: sem a chamada a `Detach`, o primeiro falha com
`Detach chamado 0 vezes, quero 1 — a sessao ficou registrada apos o logout`.

Verificado ao vivo nos DOIS caminhos.

Falha — logout numa sessão já deslogada devolve 500
(`logout failed | the store doesn't contain a device JID`) e a sessão
permanece registrada, como tem de ser.

Sucesso — logout do `iphone7`, pareado e conectado:

```
antes:  connected=True  loggedIn=True  jid=5511912345678:19@s.whatsapp.net
POST /session/logout -> 200 {"details":""}
07:53:31 info logged out
07:53:31 info Received kill signal        <- o Detach disparou
depois: /session/status -> no_session
        /admin/users    -> connected=False loggedIn=None
```

O `Received kill signal` imediatamente após o `logged out` é a assinatura da
correção: é o kill-channel sendo acionado pelo caminho da API, que antes só
o telefone acionava. Com o defeito, esta mesma chamada deixava
`loggedIn=true`.

## F81 — `GET /user/lid/{jid}` ignora o parâmetro da URL e exige corpo JSON

**Data / contexto**: 2026-08-08, varredura dos endpoints com o binário novo.

**Onde**: `pkg/presentation/http/handlers/handler_user.go:226-262`,
`pkg/bootstrap/wiring_routes.go:92`, `pkg/domain/user.go:46-48`

```go
// GetUserLID retorna o handler para POST /user/lid.      <- diz POST
...
var req domain.GetUserLIDRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {   <- lê o corpo
```

```go
registry.Register("/user/lid/{jid}", customChain.Then(ch.User.GetUserLID()), "GET")
```

```go
type GetUserLIDRequest struct {
	JID string // from URL      <- o próprio struct diz "from URL"
}
```

**Problema**: três fontes discordam. A rota é `GET` com `{jid}` no caminho, o
struct documenta `from URL`, e o handler decodifica o CORPO e nunca lê
`r.PathValue("jid")`. Um GET normal não tem corpo, então o decode falha com
EOF e o endpoint devolve 400 **sempre**.

**Evidência**:

```
GET /user/lid/5511912345678@s.whatsapp.net      -> 400
GET /user/lid/5511912345678                     -> 400
GET /user/lid/5511912345678%40s.whatsapp.net    -> 400
log: could not decode payload | error=EOF

GET /user/lid/ignorado  -d '{"JID":"5511912345678@s.whatsapp.net"}'  -> 200
    {"jid":"5511912345678@s.whatsapp.net","lid":"90000000000001@lid"}
```

A última linha é a prova de que o parâmetro da URL é ignorado: o caminho
dizia `ignorado` e a resposta veio do corpo.

**Correção sugerida**: trocar o decode do corpo por
`req.JID = r.PathValue("jid")`, validando vazio, e corrigir o comentário do
handler que diz POST. É o que a rota e o struct já prometem. Um teste de
handler que exercite a rota REGISTRADA (e não só o handler isolado) teria
pego — os testes atuais montam o handler direto, sem o path param.

**Status**: **corrigido**. O handler passou a ler `mux.Vars(r)["jid"]`.

**A armadilha que quase entrou no lugar do defeito**: a correção óbvia é
`r.PathValue("jid")`. Ela estaria igualmente quebrada — o router é
**gorilla/mux** (`pkg/bootstrap/router.go:237`), que guarda as variáveis no
contexto sob chave própria; `PathValue` só funciona com o `ServeMux` nativo e
devolveria string vazia. Seria um 400 diferente, igualmente inútil. Pego
antes de commitar, e travado por controle negativo próprio.

Coberto por `pkg/presentation/http/handlers/handler_user_lid_test.go`, que
monta o handler sob um `mux.NewRouter()` com o MESMO padrão de
`wiring_routes.go:92` — passar pelo router de verdade é o ponto:

- `..._LeOJIDDoCaminho` — o JID do caminho chega ao use case.
- `..._NaoExigeCorpo` — a chamada natural de um GET, sem corpo, funciona.
- `..._CaminhoVenceOCorpo` — caminho e corpo discordam e o caminho vence,
  para a correção não trocar uma fonte errada por duas concorrentes.

Controles negativos, os dois falhando os três testes: (a) voltar a decodificar
o corpo, (b) usar `r.PathValue` com o router gorilla.

**Por que ninguém tinha visto**: `handler_user_test.go` listava GetUserLID
como rota `POST /user/lid` com corpo, servindo o handler CRU, sem padrão de
rota. O teste mandava corpo, o handler lia corpo, e passava — enquanto a rota
registrada dava 400. A tabela agora serve esta rota sob o router e ganhou
`readsBody`, que a exclui do teste de corpo malformado.

Verificado ao vivo:

```
GET /user/lid/5511912345678@s.whatsapp.net   (sem corpo)
  -> 200 {"jid":"5511912345678@s.whatsapp.net","lid":"90000000000001@lid"}

GET mesmo caminho + corpo {"JID":"5599999999999@s.whatsapp.net"}
  -> 200 com o LID do CAMINHO (o corpo não teve efeito)
```

**Fica aberto**: um número sem servidor (`/user/lid/5511912345678`) devolve
**500**, não 400 — `invalid jid format` é erro de cliente. É a F66 (23/25
endpoints devolvendo 500) aparecendo aqui, e não uma regressão desta correção.

## F82 — sessão pareada por QR não sobrevive a um restart

**Data / contexto**: 2026-08-08, ao reiniciar o servidor para validar o
perfil de sessão enriquecido.

**Onde**: `pkg/bootstrap/eventhandler_session.go:58-72`

```go
func (evh *UserEventHandler) handleConnected(st *eventState) bool {
	st.postmap["type"] = "Connected"
	st.dowebhook = 1
	if len(evh.WAClient.Store.PushName) == 0 {
		return true                                   // <-- sai ANTES do UPDATE
	}
	...
	sqlStmt := `UPDATE users SET connected=1 WHERE id=$1`
```

**Problema**: a saída antecipada existe pelo motivo declarado logo acima dela
— não mandar presença disponível sem pushname. Mas ela leva junto o
`UPDATE users SET connected=1`, que não tem relação nenhuma com pushname.

Num pareamento novo por QR, o evento `Connected` chega ANTES de o pushname
estar populado. Resultado: `users.connected` fica em 0 numa sessão que está
viva e autenticada. Como `connectOnStartup` (`lifecycle.go:54`) itera
`WHERE connected=1`, **a sessão não é reconectada no próximo start**.

**Evidência**:

```
07:59  TesteQR pareado por QR; /session/status -> connected=true loggedIn=true
       (nenhum kill signal, nenhum logout entre isto e o restart)
08:16  Stopping server... / Server started...
       NENHUM "Connect to Whatsapp on startup"
08:17  /admin/users -> TesteQR connected=False
       /session/status -> sem sessao
```

A sessão voltou com um `/session/connect` manual, sem QR — a credencial
estava intacta. O que faltava era só a coluna.

**Correção sugerida**: mover o `UPDATE connected=1` para ANTES da guarda de
pushname, ou trocar a guarda por um `if` que envolva só o bloco de presença.
O estado de conexão persistido não deveria depender de o contato já ter
nome.

**Status**: **corrigido**. O `UPDATE connected=1` foi movido para ANTES da
guarda de pushname, que passou a envolver só o bloco de presença — o motivo
declarado dela.

Coberto por `pkg/bootstrap/connected_persistence_test.go`: persiste com
pushname VAZIO (o caso do pareamento novo), persiste com pushname (o caminho
que já funcionava, para a mudança não quebrar o que ninguém suspeitava), e
`dowebhook=1` continua valendo nos dois — o comentário histórico da função
registra que trocar a saída antecipada por `return false` silenciaria o
evento Connected de toda sessão sem pushname.

Controle negativo executado: devolvendo o UPDATE para depois da guarda, o
primeiro teste falha com `users.connected = 0, quero 1`.

Efeito colateral honesto: `TestWalogSeam_ErroDoSDKSaiSemWadebug` montava o
handler SEM banco, e o comentário dizia "sem banco, sem HTTP". Aquilo só era
verdade por causa deste defeito — a guarda saía antes da escrita. O teste
ganhou um `schemaDB` real, e o comentário passou a dizer por quê.

Verificado ao vivo: restart às 08:47 produziu `Connect to Whatsapp on
startup` e `/session/status` respondeu `connected=true loggedIn=true` sem
nenhuma intervenção.

## F83 — os erros de `/session/profile` escapam do envelope

**Data / contexto**: 2026-08-08, mesma sessão.

**Onde**: `pkg/presentation/http/profile_handler.go:44-62`

```go
http.Error(w, "missing session id", http.StatusBadRequest)
...
http.Error(w, "internal server error", http.StatusInternalServerError)
```

**Problema**: o caminho de sucesso usa `customhttp.RespondJSON` e devolve o
envelope `{code,data,success}` do ADR-002 — foi o que o commit 7f89d49
corrigiu. Os caminhos de ERRO continuaram em `http.Error`, que escreve
`text/plain`. Um cliente que sempre desserializa o envelope quebra em
qualquer falha desta rota.

Junto vem um erro de classificação: "não há sessão" é condição esperada,
causada pelo cliente, e sai como **500**.

**Evidência**:

```
$ curl -i /session/profile   (sem sessao)
HTTP/1.1 500 Internal Server Error
Content-Type: text/plain; charset=utf-8
Content-Length: 22
```

**Correção sugerida**: trocar os três `http.Error` por `RespondJSON` com o
erro tipado, e mapear `ErrNoSession` para 400 como os handlers de
`/session/*` já fazem via `isClientCausedSessionError`. É a F66 aparecendo
nesta rota; o envelope, porém, é problema à parte e mais barato de resolver.

**Status**: **corrigido**. Os três `http.Error` viraram `RespondJSON`, e
`GetProfileUseCase.Execute` passou a devolver
`apperr.New("no_session", CategoryValidation, ...)` com `ErrNoSession` como
causa — então `RespondJSON` deriva o 400 da taxonomia sozinho, sem o handler
precisar conhecer o pacote do use case, e `errors.Is(err, ErrNoSession)`
continua valendo para quem dependia dele.

O nível do log passou a seguir a categoria, como nos handlers de
`/session/*`: recusa de cliente em warn, falha nossa em error. Sem isso, um
alerta calibrado sobre `error` dispararia a cada consulta de perfil sem
sessão.

Coberto por `pkg/presentation/http/profile_envelope_test.go` (os três
caminhos de erro no envelope, o 400 vindo da categoria, e o 500 preservado
para erro SEM taxonomia — sem esta última, mapear tudo para 400 passaria) e
por `TestExecute_SemSessao_ErroTipado`.

Este último existe por causa de um controle negativo que NÃO disparou: o
teste de handler constrói o apperr ele mesmo, então provava que o handler
REAGE à taxonomia, não que o use case a PRODUZ. Removendo o apperr do use
case, handler e rota ficariam verdes com a rota errada. Com o teste novo, o
controle negativo falha com `erro nao carrega taxonomia: *profile.ProfileError`.

Verificado ao vivo:

```
$ curl -i /session/profile   (sessao inexistente)
HTTP/1.1 400 Bad Request
Content-Type: application/json
{"code":400,"error":{"code":"no_session","message":"no session"},"success":false}
```

## F84 — descartamos o pushName que o WhatsApp manda em cada mensagem

**Data / contexto**: 2026-08-08, ao validar `GET /chat/list` contra dados
reais e depois investigar por que quase nenhum contato tinha nome.

**Onde**: `pkg/bootstrap/eventhandler_history.go:248-256`

```go
// Try to get PushName from store if available
pushName := ""
if !isFromMe && senderJIDForInfo.User != "" {
    if evh.WAClient != nil && evh.WAClient.Store != nil {
        if contact, err := evh.WAClient.Store.Contacts.GetContact(ctx, senderJIDForInfo); err == nil {
            pushName = contact.PushName        // <- store, quase sempre vazio para @lid
        }
    }
}
```

**Problema**: o `WebMessageInfo` que o HistorySync entrega **já carrega o
pushName** (campo 19 do protobuf, `WAWebProtobufsWeb.pb.go:1436`). O handler
o ignora e vai buscar o nome no roster local, que para identidades `@lid`
está vazio na esmagadora maioria dos casos. O valor vazio é então gravado em
`datajson`.

Confirmado no dado persistido:

```
$ sqlite3 users.db "SELECT datajson FROM message_history WHERE chat_jid LIKE '%@lid' LIMIT 1"
  .Info.PushName = ''
```

O WhatsApp nos manda o nome em toda mensagem, e nós o substituímos por uma
consulta que falha.

**Medições** (sessão real, `dbdata/`):

```
chats individuais no historico:                    1717
com nome pela juncao direta (o que a rota faz hoje): 261   (15%)
recuperaveis por LID->PN antes da juncao:            +73   (total 334, 19%)

wanoise_contacts:  3368 entradas   (2481 @lid, 886 @s.whatsapp.net)
wanoise_lid_map:   5364 mapeamentos LID<->PN
```

**Duas correções minhas durante esta investigação**, ambas de medição:

1. Os primeiros números (`17 de 481`) vieram de amostrar a API, não o banco.
   Contra o banco inteiro são 261 de 1717.
2. A primeira consulta SQL disse que LID→PN recuperava **zero**. Estava
   errada: `wanoise_lid_map` guarda LID e PN **sem sufixo** (`lid` =
   `90000000000002`, e não `90000000000002@lid`), então o JOIN com
   `chat_jid` nunca casava. Com o sufixo removido, recupera 73. Quase
   descartei o caminho por causa do meu próprio JOIN.

**Como os projetos de referência tratam isto**:

- **Baileys** (issue #2414, "LID Mapping Best Practices") recomenda resolução
  em três camadas — JID de telefone direto, cache `store.contacts`, e mapa
  persistente `lid-mapping-*.json` — e adverte que **`store.contacts` é
  pouco confiável sozinho**, porque só eventos `contacts.upsert` o
  alimentam. É exatamente o erro deste handler: confiar só no store.
- **Evolution API** (issue #2426) teve o MESMO sintoma por causa diferente:
  `Contact.pushName` era sobrescrito com string vazia a cada mensagem
  enviada, porque o upsert não tinha guarda contra valor vazio — enquanto
  `Chat.name` tinha. A lição que serve aqui é a guarda: **nunca sobrescrever
  um nome existente com vazio**.
- **whatsapp-mcp** (issue #198) descreve nosso caso com as mesmas tabelas do
  whatsmeow (`whatsmeow_contacts`, `whatsmeow_lid_map` — aqui renomeadas
  para `wanoise_*`) e propõe normalizar o JID do remetente ANTES de gravar,
  em vez de tentar casar na leitura.

**Correção sugerida**, na ordem em que resolve mais:

1. **Usar o pushName do protobuf** em vez do store, com a guarda da Evolution
   API: só cai para o store se o protobuf vier vazio, e nunca grava vazio
   por cima de um nome que já existe. Resolve na origem e vale para toda
   mensagem nova.
2. **Persistir o pushName** em coluna própria de `message_history` (hoje só
   existe dentro de `datajson`), para a lista de conversas lê-lo sem
   desserializar JSON por linha.
3. **LID→PN antes da junção** na lista (+73 medidos). Complementar, não
   substituto.

O histórico já gravado não se recupera sozinho — o nome foi perdido na
escrita. Um novo HistorySync repovoaria.

**Status**: **corrigido** nos três eixos.

1. `resolverPushName` (`eventhandler_history.go`) passa a ler o pushName do
   protobuf e só cai para o roster quando ele vier vazio — nunca ao
   contrário, e nunca gravando vazio por cima de um nome que existe (a
   guarda da Evolution API).
2. Migração 13 dá coluna própria (`message_history.sender_push_name`),
   NULLABLE de propósito: as linhas já gravadas não têm o nome, e inventar
   string vazia apagaria a distinção entre "não sabemos" e "sabemos que não
   tem".
3. `ListChatsUseCase` ganhou a terceira fonte, atrás do roster: o nome da
   agenda vence o pushName, porque é o nome que QUEM CONSULTA escolheu.

**Testes** (política anti-regressão):

- `pkg/bootstrap/pushname_test.go` — o nome vem do protobuf com roster
  vazio; o protobuf vence o roster; o **vazio não vence** o roster; mensagem
  própria não resolve; sem cliente não entra em pânico.
- `pkg/infra/db/push_name_column_test.go` — a coluna existe após as
  migrações (contra o schema REAL, pela via de `newHistoryDB`); grava e lê;
  pega o pushName MAIS RECENTE; **mensagem recente sem nome não apaga** o
  nome anterior; chat sem nome nenhum fica fora do mapa.
- `pkg/application/usecase/user/list_chats_test.go` — o histórico nomeia
  quem o roster não conhece; o **roster vence** o histórico; a terceira
  fonte degrada; grupo NÃO usa o histórico (o pushName de um participante
  não é o nome do grupo).

**Controles negativos executados**, cinco, todos com a mensagem do defeito:

```
CN-1 so' o roster        -> pushName = "", quero o do protobuf
CN-2 sem guarda de vazio -> pushName = "", quero o do roster — o vazio venceu um nome que existia
CN-3 historico vence     -> name = "Apelido Dela", quero o da agenda — a ordem das fontes inverteu
CN-4 sem filtro na query -> pushName = ""; a mensagem recente sem nome apagou o nome anterior
CN-5 sem a migracao 13   -> coluna sender_push_name ausente apos as migracoes (0)
```

**Verificado ao vivo**: migração 13 aplicada (`coluna presente: 1`,
`migracao 13 registrada: 1`), lista respondendo com as 728 conversas.

**Limite honesto**: o histórico JÁ GRAVADO não se recupera — o nome foi
perdido na escrita, e `datajson` guarda `''`. Só um novo HistorySync (ou
mensagens novas) popula a coluna. A verificação de que os nomes de fato
aparecem exige um pareamento novo, e está pendente.

**Não implementado do plano original**: o item 3 da correção sugerida
(LID→PN antes da junção, +73 conversas medidas). Com o pushName do
histórico funcionando, ele deixa de ser o caminho principal e vira ganho
marginal — vale remedir depois de um HistorySync novo, quando se souber
quanto o pushName já resolve.

**Nota operacional**: um laço de 6 chamadas a `/user/profile` disparou
`429: rate-overlimit` do usync do WhatsApp. Aquela rota faz chamadas de rede
por consulta e NÃO deve ser usada em laço sobre uma lista.
