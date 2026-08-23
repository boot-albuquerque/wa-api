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



## Convenção de status

Toda entrada termina com um `**Status**:` cujo **veredito vem em negrito**,
para que uma varredura mecânica o encontre. Ele pode estar no início da linha
ou como item de lista (`- **Status**: ...`) — os dois layouts convivem no
arquivo, e **uma varredura tem de aceitar os dois**:

- `**Status**: **corrigido**` — com os testes que o travam e o controle
  negativo executado (ver a política anti-regressão em `CLAUDE.md`).
- `**Status**: **não corrigido**` — seguido do motivo.
- `**Status**: **fechado — não corrigir**` — decisão registrada, não pendência.
- `**Status**: **parcialmente corrigido**` — com o que ficou aberto e por quê.

O formato importa, e a varredura também: em 2026-08-08 três varreduras
seguidas minhas erraram — uma leu cinco entradas como "sem status" porque o
veredito estava em texto simples, outra perdeu quatro porque o `**Status**`
era item de lista. Em todos os casos **o documento estava certo e o método
errado**, e eu quase "corrigi" entradas íntegras.

Entradas com MAIS de um `**Status**` são legítimas: o achado tem sub-itens
com desfechos diferentes (ver F29, F49, F69). O veredito que vale é o do
sub-item; não existe um status único para elas.

Seções que são **nota** e não achado — evidência nova para entradas
existentes, observação de acompanhamento — não levam status. Dê a elas um
título que diga isso ("Nota sobre…", "Evidências novas…"), para que a
varredura as distinga de um achado que esqueceu o status.

Referências entre entradas são por **título**, nunca por número de linha —
ver a F61, cuja própria referência ficou obsoleta quando estes arquivos
foram divididos.

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

**Status**: **corrigido** (branch `feature/vendor-wa-noise`, ainda não
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

**Status**: **corrigido** nesta branch. **Atenção**: como `ace7770` é de outra
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

**Status**: **não corrigido** — decisão de quando implementar pendente com o
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

**Status**: **não corrigido** — plano aprovado fixa código e categoria; mudar
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

**Status**: **corrigido** nesta sessão (commits `c196a68` e `d005ab1`).
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
a entrada "2026-08-06 — data race real em `pkg/infra/wa-noise/safe_go_test.go`",
que a divisão dos HOUSEKEEP (2026-08-08) moveu para a RAIZ.

> A referência original era `internal/wa-noise/HOUSEKEEP.md:874` — ficou
> errada no arquivo E na linha. É a própria F61 acontecendo de novo, agora
> por causa da divisão: **referência por número de linha nasce obsoleta**.
> Cite por título.

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

> **Atualização (2026-08-08)**: a F83 corrigiu UMA instância desta família
> — `/session/profile` passou a derivar o status da categoria do `apperr`
> em vez de devolver 500 fixo. O padrão está demonstrado ali
> (`GetProfileUseCase.Execute` devolve `apperr.New(..., CategoryValidation, ...)`
> e `RespondJSON` faz o resto); as demais rotas seguem abertas.

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

**Correção de registro (2026-08-08)**: a versão anterior desta entrada dizia
que "um novo HistorySync repovoaria". Era **falso**, e teria custado um
pareamento inteiro num falso negativo.

`SaveMessageToHistory` usava `ON CONFLICT (user_id, message_id) DO NOTHING`,
e o logout NÃO apaga `message_history`. Um HistorySync novo traz as MESMAS
`message_id`, o insert inteiro era descartado, e as linhas gravadas sem nome
ficariam sem nome para sempre — **nenhuma instalação existente se curaria ao
atualizar**. Medido no `TesteQR` antes do fix: 23.836 mensagens, 0 com nome.

A cláusula virou `DO UPDATE` com guarda dupla, e as duas metades importam:

```sql
ON CONFLICT (user_id, message_id) DO UPDATE
   SET sender_push_name = EXCLUDED.sender_push_name
 WHERE EXCLUDED.sender_push_name <> ''
   AND (message_history.sender_push_name IS NULL
        OR message_history.sender_push_name = '')
```

- `EXCLUDED.<> ''` — nunca apagar um nome com vazio (Evolution API #2426).
- `IS NULL OR = ''` — só PREENCHER o que falta, nunca sobrescrever: um
  re-sync de histórico ANTIGO não pode rebaixar um nome mais recente.

Nenhuma outra coluna é tocada — a idempotência do resto do insert é o motivo
de #292 ter posto o `ON CONFLICT` ali, e não podia ser desfeita.

Quatro testes novos em `push_name_column_test.go` (`..._Resync*`), com
controles negativos executados:

```
CN-1 volta a DO NOTHING       -> sender_push_name = ""; o re-sync nao curou a linha sem nome
CN-2 sem a guarda de vazio    -> sender_push_name = ""; o vazio apagou um nome que existia
CN-2 (mesma remocao)          -> sender_push_name = "Nome Antigo"; o re-sync rebaixou um nome que ja existia
```

**Segundo defeito, descoberto NA MEDIÇÃO ao vivo (2026-08-08)**: depois do
pareamento o banco tinha 7.273 mensagens nomeadas e a lista mostrava **11**.

A lista normaliza as chaves de atividade para `@lid` (`normalizeToLID`), mas
120 dos 135 chats com pushName foram gravados como `@s.whatsapp.net` — o
histórico preserva o JID original de cada conversa. Os dois lados nunca se
encontravam.

E o comentário de `nomesDoHistorico` afirmava que `montarResumo` "tenta as
duas formas". **Não tentava** — comentário descrevendo comportamento que não
existe.

Corrigido com `aliasarParaLID`, que acrescenta às tabelas de consulta
(roster E pushName) uma entrada sob o LID equivalente de cada chave PN.
Aliasa em vez de reescrever, porque nem toda entrada de atividade é
normalizada, e não sobrescreve entrada que já veio chaveada por LID — essa é
mais direta que a derivada de um PN.

**Por que os testes não pegaram**: os dublês chaveavam tudo no mesmo espaço
de identidade. Três testes novos reproduzem a condição real (atividade
normalizada para `@lid`, tabela de consulta em `@s.whatsapp.net`), com
controle negativo:

```
CN remover o alias -> name = ""; o nome gravado como PN nao alcancou o chat normalizado para LID
```

**RESULTADO MEDIDO EM PRODUÇÃO** (pareamento de 2026-08-08):

```
                              antes      depois
mensagens com nome          0/23.836   7.273/23.856
conversas individuais         23/481       254/480   (5% -> 53%)
grupos                         19/20         19/20
```

Só 20 mensagens NOVAS entraram — as outras 7.253 são linhas antigas curadas
pelo `DO UPDATE`, o que prova que instalações existentes se recuperam ao
atualizar.

**Teto real**: 226 conversas (47%) seguem sem nome. Não é falha do
mecanismo — são contatos que nunca mandaram mensagem com pushName no
histórico sincronizado e não estão na agenda.

**Não implementado do plano original**: o item 3 da correção sugerida
(LID→PN antes da junção, +73 conversas medidas). Com o pushName do
histórico funcionando, ele deixa de ser o caminho principal e vira ganho
marginal — vale remedir depois de um HistorySync novo, quando se souber
quanto o pushName já resolve.

**Nota operacional**: um laço de 6 chamadas a `/user/profile` disparou
`429: rate-overlimit` do usync do WhatsApp. Aquela rota faz chamadas de rede
por consulta e NÃO deve ser usada em laço sobre uma lista.

## F85 — a rajada de HistorySync mata o WebSocket do painel, que fica cego no momento em que mais serve

**Data / contexto**: 2026-08-08, restauração das quatro sessões desvinculadas
nos testes do dia. O painel estava aberto acompanhando os pareamentos.

**Onde**: dois lados, e a correção de cada um é independente.

1. `pkg/presentation/http/devui/assets/sessions.html:183-196` (`log()`) e
   `:346-353` (`ws.onmessage`)
2. `pkg/infra/wa-noise/registry/broadcast/broadcast.go:27` (`writeTimeout`)

**Problema**: logo depois de um pareamento o HistorySync despeja lotes de
mensagens, e cada uma vira um broadcast para todas as conexões WS inscritas.
O painel não consome nessa velocidade, a janela TCP enche, a escrita do
servidor bloqueia, estoura `writeTimeout` e a conexão é **derrubada**.

Resultado prático: **o painel fica cego exatamente quando o operador quer
observar** — o pareamento e a sincronização que vêm logo em seguida. E como
não há reconexão automática (decisão deliberada, `sessions.html:341` — "um
socket que reabre sozinho esconde o sintoma"), ele só volta com um clique em
Conectar, que dispara `/session/connect` (ver F77).

**Medido na restauração** (log, janela de 32s):

```
13:27:47 -> 13:28:19   7 quedas de conexão durante 13 lotes de HistorySync
  5x  use of closed network connection   (socket já morto)
  2x  context deadline exceeded          (escrita estourou os 5s)
```

**As duas causas, em cadeia:**

- **O painel guarda tudo.** `log()` faz `appendChild` por evento e **nunca
  remove nada** — zero ocorrências de poda no arquivo inteiro. O DOM cresce
  sem teto durante a rajada.
- **E guarda o payload inteiro.** `onmessage` faz
  `JSON.stringify(resto)` do evento completo; só `qrCodeBase64` é abreviado.
  Um evento `Message` carrega `Info` + `RawMessage` inteiros, então cada
  linha do log é um bloco de JSON de milhares de caracteres.
- **O broadcast não tem estratégia para cliente lento.** `writeTimeout = 5s`
  por conexão, e ao estourar a conexão é removida. Para um webhook isso é
  correto — cliente morto não deve segurar o fan-out (foi o ponto da F74).
  Para um painel de diagnóstico, derrubar é perder a observação inteira.

**Correção sugerida**, na ordem em que resolve mais:

1. **Podar e truncar no painel** (barato, resolve o caso medido): teto de
   linhas no `#log` com remoção do mais antigo, e truncar `carga` a algumas
   centenas de caracteres com o tamanho original indicado — como
   `qrCodeBase64` já faz. Um painel de diagnóstico precisa do TIPO e do
   contorno do evento, não do payload completo.
2. **Coalescer no cliente**: durante rajada, agrupar eventos do mesmo tipo
   numa linha com contador em vez de uma linha por evento.
3. **Backpressure no servidor** (maior, decidir depois): fila limitada por
   conexão com descarte do mais ANTIGO em vez de derrubar a conexão. Muda o
   contrato do broadcast e afeta o webhook também — não fazer junto com (1).

**Anti-regressão**: a correção (1) precisa de teste que prove o teto — gerar
N+M eventos e verificar que o `#log` tem no máximo N linhas — e de um que
prove a truncagem, com a saída original preservada no atributo. O teste do
`devui` já trava marcadores da página (`devui_test.go`), e é onde isso entra.

**Status**: **não corrigido** — descoberto durante a restauração das
sessões, fora do escopo do que estava em andamento.

> **Nota**: o comportamento do servidor aqui é a F74 funcionando como
> projetada — o fan-out paralelo derrubou a conexão morta em vez de travar.
> O defeito não é a queda; é o painel ter provocado a queda, e a
> consequência dela ser perder a observação.

## F86 — rajada de eventos vira goroutines sem teto: não há backpressure nem circuit breaker em nenhum caminho de entrega

**Data / contexto**: 2026-08-08, ao investigar as quedas de WebSocket da F85.
A queda do painel era o sintoma visível de algo maior.

**Onde**: a cadeia inteira de entrega de eventos.

- `pkg/infra/wa-noise/runtime/safego/safego.go:13-26` — `SafeGo`
- `pkg/bootstrap/lifecycle_webhook.go:58,146,179,181` — os quatro despachos
- `pkg/infra/wa-noise/registry/broadcast/broadcast.go:111-125` — fan-out
- `pkg/bootstrap/config.go:39-41` — retry do webhook

**Problema**: um único evento de domínio dispara **até quatro goroutines
irrestritas**, e cada uma faz E/S de rede:

```go
safeGo("callHookWithHmac",     ...)  // HTTP para o webhook do usuário
safeGo("sendToWS",             ...)  // fan-out WebSocket
safeGo("sendToGlobalWebHook",  ...)  // HTTP para o webhook global
safeGo("sendToGlobalRabbit",   ...)  // publicação no RabbitMQ
```

`SafeGo` **não tem teto**: é `go func()` com `recover`, sem semáforo, sem
pool, sem fila. O `recover` protege contra pânico, não contra volume — e o
comentário dele fala só de pânico, então a ausência de limite não está
declarada em lugar nenhum.

O fan-out do WebSocket acrescenta **mais uma goroutine por conexão**, por
evento (a paralelização da F74, correta em si).

**Os multiplicadores, medidos no repositório:**

| fator | valor | onde |
|---|---|---|
| sites que marcam `dowebhook = 1` | **38** | `eventhandler_*.go` |
| goroutines por evento entregue | até **4** | `lifecycle_webhook.go` |
| goroutines adicionais por conexão WS | 1 cada | `broadcast.go:113` |
| tentativas por webhook que falha | **5**, a cada 30s | `config.go:40-41` |

Um webhook lento ou fora do ar multiplica cada evento por 5 ao longo de 150
segundos, **sem que nada perceba que ele está fora do ar** — não há circuit
breaker em lugar nenhum do projeto (`grep -riE "circuit|breaker|semaphore|
rate.?limit"` em `lifecycle_webhook.go` e `infra/messaging/` não devolve
nada).

**As rajadas que os usuários provocam, sem nenhuma intenção:**

1. **HistorySync pós-pareamento** — o caso medido. Foi o que derrubou 7
   conexões WS em 32 segundos (F85). Todo pareamento novo produz um.
2. **Grupo movimentado** — cada mensagem passa por `handleMessage`, marca
   `dowebhook = 1` e vira 4+N goroutines. Um grupo com centenas de
   participantes em conversa ativa é uma rajada contínua.
3. **`OfflineSyncPreview`/`OfflineSyncCompleted`** — o acúmulo de quem ficou
   desconectado chega de uma vez. Medido nesta sessão: `{"Total":71,...}`
   numa reconexão comum.
4. **App-state sync** — os eventos de `appstate.go` (Archive, Mute, Pin,
   Star, Label*) chegam em lote a cada sincronização.
5. **Muitas sessões simultâneas** — os multiplicadores acima são POR SESSÃO.
   Cinco sessões pareando juntas, como aconteceu hoje, multiplicam tudo por
   cinco. O projeto não tem teto de sessões.

**Por que isso não apareceu antes**: em operação normal o volume é baixo e o
GC absorve. O problema é de CAUDA — aparece no pareamento, na reconexão e no
pico —, que é exatamente quando a observabilidade importa mais.

**Correção sugerida**, em camadas independentes:

1. **Teto de concorrência no despacho** (menor risco, maior efeito): semáforo
   com capacidade fixa em torno dos quatro `safeGo` de entrega, e métrica de
   quantas vezes o teto foi atingido. Sem mudar o contrato — o despacho
   continua fire-and-forget, só deixa de ser ilimitado.
2. **Circuit breaker por destino** (webhook do usuário, webhook global,
   RabbitMQ): depois de N falhas consecutivas, abrir por T segundos e
   descartar em vez de tentar. Hoje um endpoint morto consome 5 tentativas
   por evento indefinidamente.
3. **Backpressure no broadcast** — ver F85 item 3. Fila limitada por conexão
   com descarte do mais antigo. Muda o contrato; decidir separado.

**Anti-regressão**: os três precisam de teste que prove o LIMITE, não só o
caminho feliz — disparar mais trabalho que o teto e verificar que a
concorrência observada não passa dele; e, para o breaker, que depois de N
falhas o destino deixa de ser chamado. Teste de teto sem `-race` não vale:
`make check` já roda `-race` nos pacotes de infra.

**Medições de 2026-08-08** (harness em `pkg/bootstrap/dispatch_carga_test.go`,
3 rodadas, 800 entregas, servidor a 100ms, payload de 8KB):

| | goroutines | duração | heap pico |
|---|---|---|---|
| sem teto | ~4.000 | ~207ms | **~32MB** |
| teto=16 | ~97 | 5,04s | ~24MB |
| **teto=64** | ~350 | 1,31s | **~13MB** |
| teto=256 | ~1.320 | 409ms | ~20MB |

**Três coisas que a análise estática não mostrava:**

1. **A amplificação é ~5×, não 1×**: 800 entregas viram ~4.000 goroutines. O
   transporte HTTP cria goroutines internas por conexão. A contagem de "4
   goroutines por evento" desta entrada subestimava o efeito.
2. **O primeiro harness inverteria a decisão.** Com `time.Sleep` o heap era
   4,7MB COM e SEM teto — o que diria "não faça nada". `Sleep` não aloca.
3. **O teto custa latência proporcional**: teto=16 é 24× mais lento.

**Tamanho real do payload** (20.000 eventos de `message_history`):

```
mediana 2.089 B | média 2.899 B | p90 3.998 B | p99 10.094 B | máx 120.302 B
```

Copiar 2.000 eventos custa **5,5MB** — ~6× menos que os 32MB das goroutines
sem teto. Isso responde à pergunta de desenho: **a cópia não é o custo
dominante**, e outbox/storage resolveriam DURABILIDADE, não memória. O
RabbitMQ já está no projeto se durabilidade virar requisito.

Mas o máximo é 120KB, **40× a mediana**: uma fila limitada por CONTAGEM
estoura com outliers. O teto tem de ser por BYTES.

**Problema de desenho encontrado ao implementar a camada 1**: a aquisição
BLOQUEIA o chamador, e em produção o chamador é a goroutine do handler de
eventos do SDK. Sob saturação, um webhook lento atrasaria o processamento de
TODOS os eventos da sessão, inclusive os que nem vão para webhook — trocando
"goroutines demais" por "handler parado". Descoberto porque um teste próprio
deadlockou por 600s, não por revisão.

Por isso o limitador entrou com **padrão 0 (desligado)**: o código é
mensurável e não muda comportamento até a forma de aquisição ser decidida
entre bloquear, descartar ou enfileirar.

### Segunda rodada (2026-08-08): a forma de aquisição, decidida por medição

O teto bloqueante da primeira rodada **saiu**. Ele foi medido e perdeu.

**Quatro formas, mesma rajada, HTTP real, 3 rodadas** (2.000 entregas):

| | handler | atraso p95 | dreno | goroutines | heap | perdidas |
|---|---|---|---|---|---|---|
| sem teto | 121ms | 55µs | 290ms | ~7.931 | 63,0MB | 0% |
| bloqueia-64 | 3,165s | 51,2ms | 3,266s | ~344 | 39,4MB | 0% |
| descarta-64 | 128ms | 3µs | 216ms | ~330 | 11,6MB | **93,6%** |
| fila-64w | 113ms | 3µs | 3,244s | ~341 | 18,0MB | 0% |

Bloquear custa **26×** no handler. Descartar perde **93,6%** — e 70,5% mesmo
com fila de 8MB, porque o handler enfileira ordens de magnitude mais rápido do
que a rede entrega.

**A prova de que só muda quem espera**: `bloqueia-256` e `fila-256w` têm dreno
idêntico (3,232s contra 3,234s) e goroutines quase iguais. A única diferença é
o handler — 3,131s contra 402ms. O custo de entrega não muda com a forma de
aquisição; muda sobre quem ele recai.

**Fila limitada NÃO é uma terceira opção**: com orçamento apertado ela vira a
forma bloqueante (handler 9,257s com 8MB, 401ms com 32MB, mesma implementação).
É bloquear com amortecedor, e o orçamento move o ponto onde o backpressure
começa.

**Calibração do orçamento** (8.000 entregas, pool 256): a troca é uma RETA, sem
joelho. ~1,4MB de heap compra ~110ms a menos de handler travado, até cobrir a
rajada (26,8MB de marca d'água), depois não compra mais nada. O dreno é
CONSTANTE (~3,22s) em todos os orçamentos.

**Não existe orçamento "suficiente"**: a demanda cresce com a rajada (7,4MB →
13,8MB → ≥16MB), então o orçamento só escolhe em que tamanho o handler começa
a absorver.

### A rajada real, medida em produção (15,1h, 5 sessões)

O que decidiu os padrões. Estado estacionário: **0,030 evento/s**. Pico de
**PAREAMENTO**, que é o caso perigoso:

| janela | pico de eventos |
|---|---|
| 100ms | **23** |
| 1s | 38 |
| 120s (1 pareamento) | **129** |

O HistorySync entra como **13 eventos em 5 min**, não milhares: o LOTE vira
evento, não a mensagem — 23.856 mensagens chegaram em 44 lotes ao longo de 15h.

Um pareamento é ~40× o estado estacionário, e N pareamentos simultâneos
multiplicam linearmente. Isso recalibra a rajada de laboratório: **8.000
entregas ≈ 87 pareamentos simultâneos**. Não era cenário inventado; era o
cenário de novos usuários lendo QR ao mesmo tempo.

### O que foi implementado

Pool de workers com fila limitada por BYTES, esperando na saturação. É uma
ESCADA de degradação, e em nenhum degrau se descarta entrega:

| condição | comportamento |
|---|---|
| rajada ≤ pool | indistinguível de não ter mecanismo |
| rajada > pool | enfileira: entrega mais lenta, COMPLETA |
| rajada > orçamento | handler absorve backpressure, ainda SEM PERDA |

**Sem detector de rajada**, de propósito: a escada degrada sozinha, e um
detector seria limiar para calibrar, histerese para acertar e oscilação na
fronteira para depurar — para produzir o comportamento que a estrutura já
produz.

Padrões (`WA_API_DISPATCH_WORKERS=256`, `WA_API_DISPATCH_QUEUE_BYTES=32MB`)
dimensionados para que o estado estacionário e um pareamento isolado **nunca
encostem no mecanismo**. `WORKERS=0` desliga e é o rollback.
`TestPool_PadroesCobremARajadaMedida` trava os padrões contra a medição que os
escolheu.

O `json.Marshal` subiu para antes do primeiro despacho
(`lifecycle_webhook.go:150`): a contabilidade é por bytes, e `len(jsonData)` é
a única medida honesta do payload que cada closure mantém vivo. Sem isso o
despacho do WS — justamente quem carrega os lotes de HistorySync, os maiores
medidos — entraria com tamanho estimado, furando a proteção na rajada que ela
existe para conter. Custa um Marshal a mais no modo Stdio.

O `recover` é **por trabalho**, não por goroutine como em `SafeGo`: num pool
permanente, a semântica do SafeGo encolheria o pool a cada pânico até não
sobrar worker. Controle negativo executado:

```
--- FAIL: TestPool_PanicoNaoMataOWorker (10.00s)
    so' 4 de 12 panicos rodaram: os workers morreram e o pool encolheu
```

`4 de 12` = exatamente o número de workers. Outros dois controles executados:

```
--- FAIL: TestPool_NuncaDescarta (30.00s)
    so' 3 de 500 executaram: o despacho travou
--- FAIL: TestPool_RespeitaOOrcamentoDeBytes (0.00s)
    bytes em voo chegaram a 131072 com orcamento 8192
```

**Status**: **corrigido** — pool com fila por bytes implementado, LIGADO por
padrão, 10 testes verdes sob `-race`, três controles negativos executados com
saída registrada acima, e padrões derivados da rajada medida em produção.

O limitador bloqueante foi preservado como INSTRUMENTO em
`dispatch_carga_test.go` (`limitadorBloqueante`), porque os números dele são a
evidência que levou ao pool.

**Segue aberto**: camada 2 (circuit breaker por destino) e camada 3
(backpressure no broadcast). Nenhuma das duas tem, hoje, evidência de produção
de que seja necessária — o webhook do usuário está desligado nesta instalação,
então o caminho caro (HTTP com retry) não foi exercitado em nenhuma janela
medida. **Medir com webhook ligado antes de implementar qualquer uma.**

> **Ponteiros e corrida**: `Broadcast` já documenta que `payload` é lido e
> nunca escrito, e que o produtor não muta o mapa depois de despachar
> (`broadcast.go:108-110`). Essa garantia passa a valer para os quatro
> despachos quando houver fila: enfileirar um `map` que o produtor ainda
> pode tocar transforma backpressure em corrida. Qualquer fila aqui guarda
> cópia ou valor imutável.

---

## F87 — o handler de nós já trava 5,7s em produção, e a fila de nós da sessão é SEQUENCIAL

**Data**: 2026-08-08
**Contexto**: surgiu enquanto eu media o atraso induzido no handler para
decidir a forma de aquisição da F86. O Monitor do log de sessão disparou
sozinho, com o teto de despacho **desligado** (`WA_API_DISPATCH_MAX_CONCURRENCY`
não definida, padrão 0) — ou seja, isto **não** é efeito do limitador.

**Onde**:
- `internal/wa-noise/core/client_events.go:155-180` — `handlerQueueLoop`
- `pkg/bootstrap/eventhandler.go:25` — `handleEvent`, registrado em
  `pkg/bootstrap/session_attach_hook_adapter.go:68`

**Problema**, em duas partes.

A primeira é o dado observado. Duas ocorrências no log de sessão:

```
[warn] Node handling took 5.270184959s for <message from="status@broadcast"
       id="ACCE95964615FA4DEBCA5B06749E8B7F" ... type="media">
[warn] Node handling took 5.748452125s for <message from="status@broadcast"
       id="ACCE95964615FA4DEBCA5B06749E8B7F" ... type="media">
```

Mesmo `id` nas duas: é o mesmo evento chegando em duas sessões, e as duas
levaram >5s. A causa não está diagnosticada — é mídia de `status@broadcast`,
então download ou decriptação são suspeitos, mas isso é hipótese, não achado.

A segunda parte é estrutural, e é a que importa para a F86:

```go
case node := <-cli.handlerQueue:
    doneChan := make(chan struct{}, 1)
    go func() { cli.nodeHandlers[node.Tag](evtCtx, node); ...; doneChan <- struct{}{} }()
    for i := 0; i < handlerQueueSlowNodeMaxWarnings; i++ {
        select {
        case <-doneChan:
            ticker.Stop()
            continue Loop        // <- só pega o PRÓXIMO nó quando este termina
```

O laço só volta a consumir a fila quando o nó corrente termina. **Um nó lento
segura todos os nós seguintes daquela sessão** — inclusive recibo, presença e
marcação de leitura, que nada têm a ver com o nó lento.

Isso confirma com `file:line` a premissa em que a medição da F86 foi
construída, e que até aqui era suposição minha: o chamador do despacho é
sequencial, então segurá-lo não atrasa uma entrega — atrasa a sessão. O aviso
do próprio SDK (`handlerQueueSlowNodeThreshold`) é a métrica de produção que
valida qualquer mudança nessa área, e ela já existe: não é preciso instrumentar
nada para medir o efeito de ligar o teto.

**Correção sugerida**:
1. Diagnosticar os 5,7s — instrumentar `handleEvent` por tipo de evento para
   separar "nó de mídia é lento" de "nosso handler é lento". Sem isso não dá
   para saber se o alvo é o SDK ou o nosso código.
2. Usar `Node handling took` como linha de base ANTES de ligar o teto da F86.
   Com o padrão em 0 hoje, a distribuição atual é a linha de base limpa.
3. Não introduzir nada que segure `handleEvent` enquanto (1) não estiver
   respondido: o orçamento de atraso do handler já está sendo consumido por
   outra coisa.

**Status**: **não corrigido** — registrado no momento em que apareceu.
Diagnóstico dos 5,7s não iniciado; a parte estrutural está confirmada e já
está em uso como premissa da F86.

---

## F88 — a espera do backoff de webhook dormia dentro de um worker do pool

**Data**: 2026-08-08
**Contexto**: regressão introduzida pela F86 nesta mesma sessão, encontrada ao
preparar a medição com webhook ligado. Não é achado incidental — é dívida da
mudança anterior.

**Onde**: `pkg/bootstrap/dispatch_callhook.go` (laço de retry com
`time.Sleep`), `pkg/bootstrap/config.go:41` (base do backoff).

**Problema**, por aritmética dos padrões que já estavam no código
(`webhookretry=true`, `retrycount=5`, `retrydelay=30`, backoff exponencial):

```
esperas: 30 + 60 + 120 + 240 = 450s = 7,5 min por evento
```

Desde a F86 essa espera acontece dentro de um worker do pool de 256. Logo:

- 256 eventos para um destino morto saturam o pool inteiro por 7,5 min;
- o pareamento medido são 129 eventos, então **2 pareamentos simultâneos
  bastam**;
- os quatro canais dividem o pool, então saturado ele para **também**
  WebSocket, webhook global e RabbitMQ — que nada têm a ver com o destino
  quebrado;
- passado isso, o degrau 3 da escada entra e o handler da sessão trava.

Antes do pool eram 258 goroutines dormindo: feio e inofensivo. **O mecanismo
que existia para dar garantia de funcionamento tornou este cenário
estritamente pior.**

**Dois agravantes achados junto:**

1. **Não havia jitter.** `backoffFactor := 1 << uint(attempt-1)` multiplica um
   delay fixo. N sessões que falham juntas — a tempestade de QR — repetiam em
   uníssono nos mesmos instantes, martelando um destino já com problema.
2. **A fila de erro é terminal sem consumidor conhecido.** Esgotadas as
   tentativas, o payload vai para `PublishDataErrorToQueue`. Dentro deste
   repositório NÃO existe consumidor (verificado com controle positivo: a
   mesma varredura acha o produtor). E o retry não sobrevive a restart — as
   tentativas pendentes somem em silêncio, porque a fila de erro só recebe
   DEPOIS de esgotadas.

**Corrigido assim:**

- Uma tentativa por chamada; a próxima é AGENDADA com `time.AfterFunc`. O
  worker devolve o slot imediatamente e o trabalho volta ao pool no vencimento.
  Timer pendente não custa goroutine.
- O conjunto de pendentes tem teto **por BYTES** (`WA_API_WEBHOOK_RETRY_MAX_
  PENDING_BYTES`, 32MB), pelo mesmo motivo do pool: trocar "goroutine dormindo"
  por "timer pendente" sem teto só mudaria ONDE a memória cresce sem limite.
- Jitter de ±25%.
- Base do backoff 30s → 8s: janela 450s → 120s. A fórmula não mudou, então
  quem já ajustou a variável mantém o significado. O custo de RECURSO da janela
  sumiu, mas o de RELEVÂNCIA não — evento de mensagem entregue 7 min atrasado
  já não serve para boa parte dos usos.

**Controles negativos executados:**

```
--- FAIL: TestRetry_AgendarNaoBloqueiaOChamador (67.40s)
    agendar segurou o chamador por 1m7.396923708s: a espera voltou para dentro do worker
--- FAIL: TestRetry_OrcamentoDePendentesLimita
    agendou todos os 20 com teto de 4KB: o orcamento nao esta' limitando
--- FAIL: TestRetry_JitterEspalha
    so' 1 valores distintos em 50 sorteios: o jitter nao esta' espalhando
```

**Status**: **corrigido** — 7 testes verdes sob `-race`, três controles
negativos com saída registrada acima.
`TestRetry_JanelaTotalCabeNoOrcamentoDeRelevancia` trava a janela contra a
decisão que a escolheu, no molde de `TestPool_PadroesCobremARajadaMedida`.

> **AMENDADA pelo [ADR-0005](docs/adr/0005-obrigatoriedade-de-stack-e-posse-de-sessao.md)
> (2026-08-08, algumas horas depois).** A decisão abaixo partia de que retry
> durável exigiria RabbitMQ e um consumidor. **Está errado**: durabilidade
> exige armazenamento TRANSACIONAL, e SQLite é um. Um outbox na base SQL que
> existir dá sobrevivência a restart em qualquer configuração, inclusive na
> catastrófica de um pod só — sem broker nenhum. Ver D3 e D4 do ADR.
>
> O que continua valendo do texto abaixo: o RabbitMQ segue **opcional** e não
> vira dependência dura. O que muda: ele deixa de ser o caminho da
> durabilidade e passa a ser distribuição.

**DECISÃO ORIGINAL (2026-08-08), superada pelo ADR-0005**: o retry NÃO será
durável por ora. O RabbitMQ **continua opcional** (`RabbitEnabled` pode ser `false`) e não
vira dependência dura do caminho de entrega.

Consequência aceita conscientemente: **o retry não sobrevive a restart**. Se o
processo cai dentro da janela de 120s, as tentativas pendentes somem em
silêncio — não chegam nem à fila de erro, que só recebe depois de esgotadas.
O que se perde é entrega de evento durante uma reinicialização que coincida
com um destino fora do ar.

Por que assim: retry durável exigiria fila com TTL + dead-letter E um
consumidor, que não existe neste repositório; e tornaria o broker obrigatório
para uma funcionalidade que hoje funciona sem ele. É troca de complexidade
arquitetural por uma janela de perda de 120 segundos.

Quem for reabrir isto: traga o dado que falta, que é a frequência real de
restart coincidindo com destino fora do ar. Não reabra por intuição de que
"durável é melhor" — ver a regra "Medir antes de projetar" em CLAUDE.md.

---

## F89 — não existe dono de sessão: o segundo processo derruba o primeiro

**Data**: 2026-08-08
**Contexto**: surgiu ao avaliar o requisito de rodar em N pods (k8s/k3s) com
retry e WebSocket funcionando entre réplicas. É pré-requisito dos dois, e
independente de ambos.

**Onde**: `pkg/bootstrap/lifecycle.go:54`

```go
rows, err := s.DB.Queryx("SELECT " + userInfoColumns + " FROM users WHERE connected=1")
```

**Problema**: cada processo assume **todas** as sessões marcadas como
conectadas. Não há lease, leader election, shard ou filtro por instância —
procurado em `pkg/` por `lease|leader|shard|ownership|instance_id|pod_name|
owner`, nenhuma ocorrência relacionada.

Isso importa porque uma sessão é **stateful**: o cliente whatsmeow segura um
socket vivo com o WhatsApp dentro de UM processo. Duas réplicas conectando as
mesmas credenciais fazem o WhatsApp derrubar uma delas — e a prova de que o
modo de falha é real está no próprio código, que já trata
`events.StreamReplaced` (`pkg/bootstrap/eventhandler.go:73`).

Com duas réplicas, ambas conectam tudo e se derrubam mutuamente em laço. Quebra
no pod nº 2, antes de qualquer preocupação com entrega.

**Dois bloqueadores multi-pod que apareceram junto:**

1. **Fallback para SQLite** (`pkg/infra/db/connection.go:81`): quando as
   variáveis de Postgres estão PARCIALMENTE definidas, o processo cai em SQLite
   em vez de falhar. SQLite não sustenta N réplicas. Num perfil k8s isso tem de
   falhar alto, não degradar em silêncio — uma variável esquecida no manifesto
   viraria corrupção silenciosa de estado compartilhado.
2. **Armadilha do retry durável sob N pods**: `tentarWebhook` pega o cliente
   HTTP de `clientManager.GetHTTPClient(userID)`, que é estado EM MEMÓRIA do
   processo. Num pod que não é dono daquela sessão isso devolve `nil` e o
   código faz `log.Warn` e **retorna em silêncio**. Ou seja, fila de retry
   compartilhada entre réplicas descartaria caladamente tudo que fosse
   consumido pelo pod "errado". A entrega de webhook em si NÃO é presa à
   sessão (é um POST HTTP; proxy e chave HMAC estão no banco), então o conserto
   é montar o cliente a partir do banco — mas tem de ser feito JUNTO com a fila
   durável, não depois.

**Correção sugerida**: lease de posse por sessão. Cabe em Postgres
(`FOR UPDATE SKIP LOCKED` ou advisory lock) sem exigir Redis. Resolvida a
posse, o problema do WebSocket entre réplicas fica quase de graça: basta rotear
`/session/ws` para o pod dono, em vez de precisar de barramento de fan-out.

**A validar ANTES de codificar** (ordem acordada com o usuário: posse primeiro):

1. **Conexão duplicada**: subir dois processos contra o mesmo banco e medir o
   que o WhatsApp faz — quantos `StreamReplaced`, em que intervalo, se entra em
   laço. Decide se posse é "importante" ou "inegociável".
2. **Handoff**: matar o pod dono e medir QUANTO TEMPO até outro assumir e a
   sessão voltar a receber evento. Esse número define se lease em Postgres
   basta.
3. **O store aguarda a troca?** O estado do dispositivo vive no banco
   compartilhado, então em tese a segunda réplica retoma. "Em tese" é o que
   esta sessão inteira ensinou a não aceitar — ver "Medir antes de projetar"
   em CLAUDE.md.

### Validação executada (2026-08-08) — ambiente isolado, sessão descartável

Postgres + RabbitMQ em `infra/compose.yaml`, instância de teste na 8081 contra
Postgres (a produção roda em SQLite), usuário `descartavel-f89` pareado por QR.

**Exp 3 — o store aguenta a troca de processo? SIM, 6/6.** Duas execuções
independentes de 3 rodadas. Processo novo retoma a sessão SEM QR em todas:
zero menções a QR nos logs, socket com o WhatsApp restabelecido, JID idêntico.
`users.connected` permanece `1` após desligamento gracioso, que é o que faz o
`connectOnStartup` seguinte pegar a sessão.

**Exp 2 — tempo de retomada: 1,16s a 2,29s** (6 rodadas). O religamento da
sessão NÃO é o gargalo. Um lease em Postgres, que resolve em dezenas de
milissegundos, é folgado — não há necessidade de Redis nem de eleição rápida.
Se a retomada custasse 30s a conversa seria outra.

> Instrumento corrigido no caminho: a primeira medição deu "1,00s" nas três
> rodadas, redondo demais. Era quantização — os timestamps do log têm resolução
> de SEGUNDO. Os números acima vêm de cronômetro próprio.

**Exp 1 — duas réplicas na mesma sessão. O resultado CORRIGE a análise acima.**

Esta entrada dizia que as réplicas "se derrubam mutuamente em laço". **Está
errado.** Medido com A na 8081 e B na 8082 contra o mesmo Postgres, observado
por 90s:

| | |
|---|---|
| `StreamReplaced` em A | **1** (não voltou a subir em 90s) |
| `StreamReplaced` em B | 0 |
| logout | **0** — credenciais preservadas |
| A se recupera depois que B morre? | **não**, nem após 45s |
| `users.connected` no banco | continua **1** |

Não há laço. Há **um único chute, e o perdedor fica morto para sempre**: o
processo A segue vivo e respondendo HTTP, mas sua sessão fica
`loggedIn=false, connected=false` e nunca tenta reconectar — nenhuma tentativa
no log depois do `StreamReplaced`.

Isso é PIOR que o laço que eu supus, por um motivo: laço é barulhento e
observável. Isto é **silencioso**. Em k8s, duas réplicas de vida longa
disputando a mesma sessão deixam uma delas zumbi — processo saudável, HTTP
respondendo, liveness verde, sessão morta — e o banco ainda afirma
`connected=1`, então nada sinaliza o problema.

**Consequência para o desenho**: além do lease, é preciso um sinal de saúde que
distinga "processo de pé" de "sessão de pé". O `connected` da tabela `users` é
intenção, não estado observado, e hoje qualquer readiness probe baseada nele
mentiria.

### D1 implementado (2026-08-08)

`pkg/bootstrap/cluster.go`: `WA_API_CLUSTER_MODE` (`single` padrão | `multi`),
validação de stack e trava de instância única por `flock` no diretório de
dados, tudo ANTES de o banco ser aberto — configuração impossível morre sem ter
tocado em estado.

- **`multi` sem Postgres é FATAL**, não `warn`. A mensagem diz quais variáveis
  definir, e um teste trava isso: erro que só informa que falhou obriga o
  operador a ler o código.
- **Valor desconhecido é erro**, não cai no padrão. `WA_API_CLUSTER_MODE=mutli`
  num manifesto não pode virar `single` em silêncio.
- **`single` recusa o segundo processo** por `flock`, que o SO libera sozinho
  inclusive em `kill -9` — arquivo com PID exigiria detectar trava obsoleta, e
  é aí que esse tipo de mecanismo costuma falhar liberando quando não devia.

Verificação ponta a ponta com dois processos reais, **em portas diferentes de
propósito** (mesma porta faria o segundo morrer por colisão e eu estaria
medindo outra coisa):

| | antes (Exp 1) | depois |
|---|---|---|
| segundo processo sobe? | sim | **não** — `fatal` com motivo e saída |
| o incumbente sobrevive? | **não**, sessão morta para sempre | **sim**, HTTP 200 |

Três controles negativos executados: typo caindo no padrão, `multi` aceitando
SQLite, e ausência de `flock` — os três produziram falha com mensagem.

**Limitação conhecida e documentada no código**: o `flock` protege contra um
segundo processo NA MESMA MÁQUINA. Duas máquinas apontando para o mesmo
Postgres em modo `single` não são detectadas — é para isso que existe `multi`
com lease (D2). O caso coberto é o que de fato acontece: alguém sobe um segundo
processo sem perceber.

**Status**: **parcialmente corrigido** — D1 do ADR-0005 implementado e
verificado. D2 (lease), D3 (outbox) e D6 (saúde separada) seguem abertos. Em
`multi` o processo avisa em `Warn` que a posse por lease ainda não existe e que
não se deve subir mais de uma réplica.

---

## F90 — desconexão do cliente é logada como `error`, e parece falha de banco

**Data**: 2026-08-08
**Contexto**: apareceu no monitor durante a validação da F89, como uma rajada
de 15 erros seguidos que aparentavam falha de banco em produção. Não era.

**Onde**: caminho de leitura de sessão/usuários — as mensagens são
`failed to list users`, `failed to read session record` e
`session use case failed`, todas com `error=context canceled` e
`error=database error: context canceled`.

**Problema**: `context canceled` aqui significa que o CLIENTE desistiu da
requisição — no caso observado, o navegador cancelando o polling do devui ao
trocar de aba. Isso não é erro do servidor: nada falhou, ninguém precisa agir.

Evidência de que é o cliente: todas as 15 ocorrências têm
`user_agent: Mozilla/...` e `req_id` do polling do devui, e o processo seguiu
saudável (HTTP 200) durante e depois.

O dano é de sinal, não de função. Em nível `error`, com a mensagem
`database error`, uma navegação rotineira de aba fica indistinguível de banco
fora do ar — e num ambiente com alerta por nível de log isso acorda alguém de
madrugada por causa de um F5.

**Correção sugerida**: classificar `errors.Is(err, context.Canceled)` como
`Info`/`Debug` no caminho HTTP, e não como `error`. O cancelamento pelo cliente
é resultado esperado, não defeito. Atenção para NÃO confundir com
`context.DeadlineExceeded`, que é timeout NOSSO e continua sendo erro de
verdade.

**Status**: **não corrigido** — fora do escopo da F89, que era o que estava em
andamento. Registrado para decisão.

---

## F91 — o ramo `default` do handler despeja a struct inteira no log

**Data**: 2026-08-08
**Contexto**: apareceu ao investigar os avisos `Unhandled event` que o monitor
mostrava durante a validação da F89.

**Onde**: `pkg/bootstrap/eventhandler.go`, ramo `default` do type-switch de
`handleEvent`.

**Problema**: o `default` loga o evento com `%+v` da struct — VALORES, não
forma. Exemplo real colhido do log de sessão:

```
{"level":"warn","event":"&{Codes:[https://wa.me/settings/linked_devices#2@B+o1E1qrt...
 ...,3K5amX5Az3wjYJr4T/UuAf0CO+0eHW6p5mhXDdzN4ws=,...]}","message":"Unhandled event"}
```

São **códigos de pareamento** no log. Quem os lê vincula um aparelho à conta.

A F76 já tinha registrado exatamente isso e foi corrigida — mas **só para
`*events.QR`**, com um `case` dedicado. O ramo `default` continua despejando
valores de qualquer OUTRO tipo que caia nele. A correção tratou a instância,
não a classe.

**Evidência de que a correção da F76 funciona e o resto não**: a linha acima é
de `2026-08-07T23:52:32`, anterior ao `case *events.QR`. Depois dela não há
mais vazamento de `Codes` — e há, no mesmo log, outros eventos caindo no
`default` com dump completo, entre eles um de chamada com
`BasicCallMeta:{From:...@lid CallCreator:...@lid CallID:...}`.

**Números** (17,5h de log, uma instalação):
- 11 eventos caíram no `default`;
- 43 tipos `*events.*` têm `case` próprio;
- 69 structs declaradas em `internal/wa-noise/protocol/types/events/` —
  **limite superior**, não contagem de não-tratados: nem toda struct daquele
  pacote é evento despachado. Quem for corrigir tem de levantar a lista real,
  não subtrair 43 de 69.

**Correção sugerida**: o `default` loga o NOME DO TIPO e os NOMES DOS CAMPOS,
nunca os valores — foi o que o watcher desta sessão
(`scratchpad/watch.py:88-99`) precisou fazer para ser seguro de ler. Isso
preserva o valor do aviso (descobrir tipo não previsto) e elimina a classe
inteira do vazamento, em vez de fechar um tipo por vez conforme cada um
vaza algo.

**Status**: **não corrigido** — registrado. Fora do escopo da F89, que estava
em andamento.

> Nota de método: eu quase registrei que a aplicação já logava só nomes de
> campo, porque o monitor exibia `campos=JID,Timestamp,Action,FromFullSync`.
> Aquilo era renderização do meu próprio watcher, não do wa-api — o log não
> contém a string `campos` uma única vez. Ver ARMADILHAS.md 13.

---

## F92 — 71% dos arquivos Go têm comentário em português, contra a política de idioma

**Data**: 2026-08-08
**Contexto**: o usuário definiu o padrão de idioma do código (identificadores,
comentários, nomes de arquivo e mensagens de log/erro em inglês) e pediu que
ficasse documentado para qualquer AI ou sessão seguir. A política entrou em
`CLAUDE.md` e `AGENTS.md`. Esta entrada registra o passivo.

**Onde**: `pkg/` e `cmd/`.

**Números**, medidos e não estimados:

```
arquivos .go com comentário em português : 329
total de arquivos .go em pkg/ e cmd/     : 465
                                           ~71%
```

Heurística usada: acentuação e palavras-função (`ção`, `não`, `porque`, `que a`)
dentro de linha `//`. É limite superior aproximado — arquivo com uma única
linha em português conta igual a um totalmente em português. Quem for atacar
precisa levantar a lista real, não usar este número como plano de trabalho.

**Problema**: não é defeito de funcionamento. É custo de leitura, e ele cresce
com o tamanho do time e com a quantidade de agentes trabalhando no repositório.
Identificador em português no meio de expressão com palavra-chave em inglês
obriga alternância de idioma a cada linha.

**Correção sugerida — e explicitamente NÃO uma conversão em massa.** A política
já diz: ao TOCAR num arquivo, converta o que você mexeu, não o arquivo inteiro.
Assim a dívida drena por contato, e nenhum diff mistura renomeação com mudança
de comportamento — que é onde defeito passa despercebido em revisão.

Se um dia se quiser um mutirão, ele tem de ser: commit separado, só renomeação,
zero mudança de comportamento, e `make check` verde antes e depois com os
mesmos números de gate.

**Status**: **não corrigido, por decisão**. Os arquivos do trabalho corrente
(`lease.go`, `session_lease.go` e testes) já nasceram em inglês; `cluster.go` e
`dispatch*.go`, escritos nesta sessão antes da política, ficam para um commit
de conversão pura. O restante aguarda decisão sobre mutirão.

---

## F93 — logout de sessão desconectada devolve 500 e deixa `users.connected=1` preso

**Data**: 2026-08-08

**Contexto**: o usuário usou o devui para **desconectar** e, dois segundos
depois, **deslogar** a sessão `TesteQR` (`1dde2515346fa4c2c3e4a7b2eda351c2`).
O disconnect respondeu 200; o logout respondeu **500**. Investigação feita
sobre o log da instância viva (`/tmp/wa-ses.log`, 77 MB) e sobre
`dbdata/users.db` em modo somente leitura.

**Onde**:
- `pkg/infra/wa-noise/runtime/session/guard.go:88-94` —
  `SessionGuardAdapter.Logout` devolve o erro **cru** do SDK
  (`client.Logout(...)`), sem embrulhar em `apperr`.
- `internal/wa-noise/core/client_session.go:25-46` — `Client.Logout` manda um
  IQ `remove-companion-device` **antes** de qualquer coisa; sem websocket o
  `sendIQ` falha e a função retorna cedo:
  `return fmt.Errorf("error sending logout request: %w", err)`.
- `pkg/application/usecase/session/logout.go:40-57` — se `sessions.Logout`
  falha, o use case retorna e **nunca chega em `uc.detacher.Detach(txtID)`**
  (linha 57).
- `pkg/bootstrap/session_attach_hook_adapter.go:84` — `Detach` é o único
  escritor de `users.connected=0` neste caminho
  (`UPDATE users SET qrcode='', connected=0 WHERE id=$1`).
- `pkg/presentation/http/handlers/handler_session.go:151-160` — os dois ramos
  do `if err != nil` chamam `RespondJSON(w, 500, nil, err)`; o status só é
  corrigido para 4xx por `pkg/presentation/http/response.go:50-57`, que lê a
  categoria **quando o erro é um `*apperr.AppError`**. Como o erro que vem de
  `guard.go:93` é `fmt.Errorf` puro, o 500 literal prevalece e o corpo vira a
  mensagem genérica de `genericErrorMessage(500)`.

**Problema**: a sequência exata, reconstruída por `req_id` (linhas reais do
log, não paráfrase):

```
{"req_id":"d9rqgo6hokiibhhb282g","txtID":"1dde2515346fa4c2c3e4a7b2eda351c2",
 "time":"2026-08-08T18:06:24-04:00","message":"disconnected"}
{"req_id":"d9rqgo6hokiibhhb282g","method":"GET","url":"/session/disconnect",
 "status":200,"outcome":"success","time":"2026-08-08T18:06:24-04:00","message":"Got API Request"}
{"level":"warn","req_id":"d9rqgomhokiibhhb286g","txtID":"1dde2515346fa4c2c3e4a7b2eda351c2",
 "error":"error sending logout request: websocket not connected",
 "time":"2026-08-08T18:06:26-04:00","message":"logout failed"}
{"level":"error","req_id":"d9rqgomhokiibhhb286g","handler":"Logout",
 "error":"error sending logout request: websocket not connected",
 "time":"2026-08-08T18:06:26-04:00","message":"session use case failed"}
{"req_id":"d9rqgomhokiibhhb286g","method":"POST","url":"/session/logout",
 "status":500,"size":61,"outcome":"server_error","time":"2026-08-08T18:06:26-04:00","message":"Got API Request"}
```

Não é caso isolado. Todas as ocorrências de `logout failed` no log (4 no
total, contadas — não estimadas), com 4 respostas 500 correspondentes em
`/session/logout` contra 5 respostas 200:

```
2026-08-08T00:48:22 c3ad762d... | error sending logout request: websocket not connected
2026-08-08T00:48:22 dffc90b2... | error sending logout request: websocket not connected
2026-08-08T01:07:12 c3ad762d... | the store doesn't contain a device JID
2026-08-08T18:06:26 1dde2515... | error sending logout request: websocket not connected
```

Os dois efeitos colaterais:

1. **Estado divergente e persistido.** Como `Detach` não roda, `users.connected`
   fica em 1 para uma sessão que o próprio runtime reporta desconectada.
   Evidência cruzada — leitura do banco de produção:

   ```
   id                                name      connected  jid
   1dde2515346fa4c2c3e4a7b2eda351c2  TesteQR   1          554192421234:20@s.whatsapp.net
   ```

   contra o `/session/status` da mesma sessão, no mesmo minuto:

   ```
   {"txtID":"1dde2515346fa4c2c3e4a7b2eda351c2","connected":false,"loggedIn":true,
    "time":"2026-08-08T18:08:52-04:00","message":"get status validated"}
   ```

   O flag mentiroso não é cosmético: `connectOnStartup`
   (`pkg/bootstrap/lifecycle.go:54`) itera `SELECT ... FROM users WHERE
   connected=1` para decidir o que religar na subida do processo.

2. **O usuário não tem caminho óbvio para sair.** Não existe rota de logout
   forçado: `/session/logout` é o único caminho, e ele exige websocket vivo
   por construção do SDK. **Não é beco sem saída absoluto** — `GET
   /session/connect` chama `Orchestrator.Start`
   (`pkg/application/session/orchestrator.go:139-167`), que cria uma sessão
   nova a partir das credenciais ainda presentes no store e a re-registra;
   com o transporte de pé, o logout volta a funcionar. Mas isso é
   conhecimento de implementação: a API responde 500 com corpo genérico
   ("internal server error"), sem dizer que o remédio é reconectar antes.
   Quem só olha a resposta conclui, razoavelmente, que a sessão travou.

**O 500 é a categoria errada.** "Cliente pediu logout de uma sessão que ele
mesmo acabou de desconectar" é pré-condição violada pelo chamador, não falha
do servidor. A taxonomia para isso já existe e está em uso no repositório —
`pkg/domain/apperr/codes.go` mapeia `CategoryValidation` para 400 e
`response.go:50-57` aplica esse mapa sozinho **desde que o erro seja um
`AppError`**. O caminho de logout simplesmente não participa: `guard.go:93`
devolve erro cru. Repare que `wanoiseSession.Logout`
(`pkg/infra/wa-noise/runtime/session/session.go:57-62`) até embrulha em
`apperr`, mas escolhe `CategoryInternal` com `Retryable: true` — ou seja,
mesmo o caminho que já migrou classificaria isto como 500 e ainda diria ao
cliente que vale a pena repetir a mesma requisição, que produzirá o mesmo
erro. Não é o adaptador que está no caminho desta rota (o texto de erro do log
é o cru, não a `Message` do `AppError`), mas é a mesma decisão errada escrita
duas vezes.

**Correção sugerida**, em três pedaços independentes:

1. **Classificar.** Em `guard.go:88-94`, embrulhar a falha em `apperr` e
   distinguir os dois casos observados: websocket ausente
   (`session_not_connected`, `CategoryValidation`, `Retryable: false`) e store
   sem device JID (`session_not_logged_in`, `CategoryValidation`). Ajustar
   junto `session.go:57-62`, que hoje classifica tudo como
   `CategoryInternal`/retryable. Com isso o status vira 409/400 e o corpo passa
   a carregar `code` e `message` úteis, sem tocar em `handler_session.go` — o
   mapa de `response.go` faz o resto.

2. **Não deixar estado preso.** Quando o logout falhar por ausência de
   transporte, o use case (`logout.go:40-46`) ainda deve alinhar o estado
   local antes de retornar o erro: chamar `uc.detacher.Detach(txtID)` zera
   `users.connected` e limpa os registries, de modo que o banco pare de
   afirmar `connected=1` para uma sessão morta e `connectOnStartup` pare de
   religá-la. O pareamento no telefone continua existindo — e é exatamente
   isso que a resposta 4xx precisa dizer.

3. **Dar saída explícita.** Ou o `LogoutUseCase` reconecta antes de deslogar
   quando detecta transporte caído (o SDK preserva credenciais; o custo é a
   latência de um Connect), ou a mensagem de erro instrui a chamar
   `/session/connect` primeiro. A primeira opção é a que remove a pegadinha;
   a segunda é a barata. Qualquer uma é melhor que 500 opaco.

**Status**: **não corrigido** — só diagnosticado, a pedido do dono do
repositório. Nada foi alterado no código nem no banco.

> **Verificação da F93 (2026-08-08)** — o diagnóstico acima foi produzido por
> um agente delegado e as afirmações que o sustentam foram conferidas contra o
> código e o banco, não aceitas de saída:
>
> - `guard.go:88-94` devolve mesmo `client.Logout(context.Background())` cru,
>   sem `apperr`. **Confere.**
> - `logout.go` retorna no `if err != nil` **antes** de `uc.detacher.Detach`.
>   **Confere** — e o comentário do próprio código afirma que `Detach` é "o
>   único escritor de `users.connected` neste caminho", o que fecha a terceira
>   afirmação sem precisar de outra busca.
> - `users.connected=1` para `1dde2515…` (`TesteQR`). **Confere**, lido do
>   `dbdata/users.db` em cópia somente leitura.
>
> **ATENÇÃO ao aplicar a correção 2** (chamar `Detach` também quando o logout
> falha por falta de transporte): a posição atual do `Detach` é DELIBERADA e
> está justificada pela F80 — ele fica depois do sucesso porque o logout
> iniciado pelo TELEFONE já passa pelo kill-channel, e duplicar escrita
> quebraria o alinhamento dos dois fluxos. A correção não é mover o `Detach`,
> é ADICIONAR uma chamada no ramo de falha por transporte ausente,
> preservando a do ramo de sucesso. Quem trocar a ordem reabre a F80.
>
> Observação colhida na verificação: há **sete** sessões com `connected=1`, não
> cinco — além das cinco de trabalho, `TesteQR` (presa pelo defeito acima) e
> `instanciaA`, pareada por engano durante a validação da F89.

---

## F94 — `/devui` sem barra final devolve 404, sem redirecionar

**Data**: 2026-08-08
**Contexto**: o usuário tentou abrir o painel durante a validação do ADR-0005
D2 e recebeu 404. Não era o servidor fora do ar nem o devui desligado.

**Onde**: `pkg/bootstrap/wiring_routes.go:42` e
`pkg/presentation/http/devui/devui.go:46`.

```go
BasePath = "/devui/"                                   // devui.go:46
registry.Register(devui.BasePath+"{rest:.*}", devChain, "GET")  // wiring_routes.go:42
```

O padrão registrado é `/devui/{rest:.*}`, que casa com `/devui/` (rest vazio) e
**não casa** com `/devui`. Não há rota nem redirecionamento para a forma sem
barra.

**Problema**: medido na instância viva, com o devui habilitado e a mesma
instância respondendo normalmente nas demais rotas:

```
/devui/         -> 200
/devui          -> 404
/session/status -> 200
```

O dano é de usabilidade, e ele é desproporcional ao tamanho: `/devui` é a forma
que se digita naturalmente, e o 404 é indistinguível de "o devui está
desligado" ou "a instância caiu" — que foi exatamente a hipótese levantada
quando aconteceu. Custa uma rodada de diagnóstico para descobrir que o único
problema é uma barra.

**Correção sugerida**: registrar `/devui` com um `http.RedirectHandler` para
`/devui/` (301 ou 308), ao lado do registro atual. É o comportamento que a
maioria dos servidores tem por padrão para diretório, e resolve a classe
inteira em uma linha.

**Status**: **não corrigido** — fora do escopo do D2, que estava em andamento.
Registrado assim que aconteceu.

---

## F95 — a taxonomia de `apperr` não tem categoria para conflito (409)

**Data**: 2026-08-08
**Contexto**: apareceu ao classificar a recusa por posse de sessão (ADR-0005
D2). Registrada, e não resolvida na hora, para não expandir taxonomia no meio
de outra tarefa.

**Onde**: `pkg/domain/apperr/codes.go:21-44`.

```go
CategoryValidation   Category = "validation"    -> 400
CategoryUnauthorized Category = "unauthorized"  -> 401
CategoryInternal     Category = "internal"      -> 500
```

**Problema**: são três categorias, e nenhuma descreve "a requisição está
correta, mas não pode ser atendida NESTE estado ou NESTA réplica". Dois casos
concretos já esbarram nisso:

1. **Posse de sessão** (`pkg/application/session/orchestrator.go`, constante
   `codeSessionOwnedByAnotherReplica`): a sessão pertence a outra réplica. O
   cliente não errou nada — a requisição chegou no pod errado. Hoje sai **400**,
   que diz ao cliente que ele mandou algo inválido. O correto é **409** (ou
   **421 Misdirected Request**, que descreve exatamente isto).
2. **Logout de sessão desconectada** (F93): mesma natureza — pré-condição
   violada, não requisição malformada. A F93 sugere `CategoryValidation` pela
   mesma falta de opção.

O dano é de contrato: um cliente que trate 400 como "corrija o payload e não
repita" vai fazer a coisa errada nos dois casos, porque em ambos a resposta
certa é "repita contra o dono" ou "reconecte antes".

**Correção sugerida**: acrescentar `CategoryConflict` mapeando para
`http.StatusConflict`, e migrar os dois sites acima. É mudança pequena em
`codes.go` mais o teste de mapeamento que já existe
(`apperr_test.go:127 TestCategory_HTTPStatus`), que vai pedir a linha nova.

**Atenção ao migrar**: mudar a categoria de um erro existente MUDA O STATUS HTTP
que clientes já recebem. Para a posse de sessão isso é seguro (código novo,
sem cliente ainda); para a F93 é mudança de contrato observável e precisa ser
decidida como tal.

**Status**: **não corrigido** — registrado com os dois sites que já sofrem.

---

## F94 — o harness do estudo desligava o Chromium por sinal nos modos que carregam a credencial do WhatsApp

**Data**: 2026-08-09.

**Contexto**: apareceu na Fase 5 do estudo `scripts/chromium-study`, ao abrir o
WhatsApp Web para medir o CPU correctness boundary. Não é escopo da Fase 5 — é
dívida deixada pela Fase 4C.

**Onde**: `scripts/chromium-study/p4_wa.go:187,236,442`,
`p4b_wasession.go:65`, `p4c_target.go:275,451,457,477,539`.

```go
defer gracefulStop(browsers[0])   // = SIGTERM
```

**Problema**: a Fase 4C mediu que desligar por sinal corrompe o estado de sessão
do WhatsApp (logout na 4ª e na 6ª iteração, contra 40 iterações limpas com
`Browser.close`), e escreveu isso como requisito nº 1 de produção no
`RELATORIO-FASE-4C.md` §7. **O requisito nunca saiu do experimento que o
mediu.** Todos os modos que carregam a credencial — `waprep`, `waopen`,
`wacap`, `wasession`, `watabs` — continuaram em `gracefulStop`.

Evidência medida nesta sessão: após dois `waopen`, o perfil ficou com os **3
arquivos `Singleton` presentes**, que é a assinatura de saída suja estabelecida
pela 4C (o Chromium remove os próprios `Singleton` ao sair limpo; sob SIGTERM
ficavam em 3 sempre). O perfil ainda estava em 148 MB, acima dos 118 MB que
marcam degradação, então a credencial sobreviveu — mas por margem, e a 4C viu
logout já na 4ª iteração desse regime.

Três das nove chamadas eram piores que as demais: em `oneRecoveryTrial`
(`p4c_target.go:451,457,477`) o desligamento sujo está em **caminho de erro**, e
o da linha 477 roda justamente quando o baseline falha — ou seja, suja o perfil
exatamente quando a credencial já está frágil.

**Correção**: aplicada nesta sessão. Novo `cleanStop`
(`p4c_lifecycle.go`) envia `Browser.close` via CDP, espera a saída, e imprime
sempre o caminho usado (`stopped_via=`), porque os call sites usam `defer` e
descartariam o retorno. Os nove call sites migraram, exceto os dois em que a
forma de parada É a variável sob ablação (`p4c_target.go:502` sob
`RecoveryFault`, `p4c_lifecycle.go:240` sob `LifecycleStop`), marcados
`//ablation:stop-form`.

**Testes que travam** (`scripts/chromium-study/shutdown_policy_test.go`):

1. `TestCredentialModesNeverStopBySignal` — estático sobre a AST: qualquer
   função que atribua `PersistentProfileDir` e chame `gracefulStop` sem o
   marcador falha. É estático de propósito: o defeito não é "o desligamento não
   funciona" (`closeBrowserViaCDP` sempre funcionou), é "o call site chama a
   função errada", e um teste de comportamento sobre `cleanStop` passaria com
   os cinco modos ainda quebrados.
2. `TestCleanStopGoesThroughBrowserClose` — impede que esvaziar `cleanStopVia`
   deixe a suíte verde com todos os modos desligando por sinal por dentro.

**Controle negativo EXECUTADO**: reintroduzido `gracefulStop` em
`p4_wa.go:236`:

```
--- FAIL: TestCredentialModesNeverStopBySignal (0.01s)
    shutdown_policy_test.go:114: modo que carrega a credencial desliga por sinal
    — SIGTERM corrompe o estado de sessao (Fase 4C §7 req. 1). Use cleanStop.
      p4_wa.go:236:8 em RunWAOpen
```

Restaurado, volta a `ok`.

**Nota de método**: a primeira versão do teste isentava funções inteiras por
nome, e teria deixado passar as três chamadas de caminho de erro em
`oneRecoveryTrial` — só uma das quatro ali era ablação legítima. A isenção
passou a ser por linha (`//ablation:stop-form`). Isenção por nome de função
cobre também o código que ainda vai ser escrito lá dentro.

**Status**: **corrigido** nesta sessão, com os dois testes acima e controle
negativo colado.

## F96 — `make check` está VERMELHO por toolchain, e não por código: `covdata` ausente

**Data**: 2026-08-20 · **Contexto**: fechamento da CAP-07 (`sendText`) no
`internal/wa-headless`; o gate foi rodado antes de commitar e reprovou.

**Onde**: `Makefile:132-133` (alvo `coverage-gate`).

**Sintoma**: o alvo falha, e a falha é por pacote SEM arquivo de teste:

```
# wa-api/cmd/core
go: no such tool "covdata"
# wa-api/cmd/listroutes
go: no such tool "covdata"
# wa-api/cmd/wss
go: no such tool "covdata"
# wa-api/pkg/infra/wa-noise/client/testkit
go: no such tool "covdata"
make: *** [coverage-gate] Error 1
```

**Causa medida**: a toolchain em uso é a BAIXADA por `GOTOOLCHAIN=auto`, e o
diretório de ferramentas dela não tem `covdata`:

```
$ go env GOROOT
/Users/albuquerque/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.darwin-arm64
$ ls $(go env GOROOT)/pkg/tool/darwin_arm64/
asm cgo compile cover fix link preprofile vet
```

O `go test -coverprofile` chama `covdata` para produzir o perfil VAZIO de um
pacote sem testes. Pacote com testes não passa por esse caminho — que é por que
o gate reprova exatamente nos quatro pacotes sem `_test.go`.

**Não é do código.** Reproduzido em `wa-api/cmd/core`, pacote que a tarefa não
tocou:

```
$ go test -count=1 -coverprofile=/tmp/c3.out wa-api/cmd/core
# wa-api/cmd/core
go: no such tool "covdata"
```

Cobertura de pacote único e cobertura multi-pacote com `-coverpkg` funcionam
normalmente; só o caminho do pacote sem testes quebra.

**Correção sugerida** (uma das três, e é decisão de ambiente, não de código):

1. Fixar `GOTOOLCHAIN` numa instalação local completa em vez da baixada.
2. Reinstalar/completar a toolchain do módulo.
3. Excluir do `COVER_PKGS` os pacotes sem testes — **o pior dos três**, porque
   troca um problema de ambiente por uma mentira permanente no denominador da
   cobertura.

**Impacto no fechamento desta sessão**: as etapas `build`, `vet` e `test -race`
passam; `coverage-gate` reprova. Ou seja, `make check` NÃO está verde, e a razão
está integralmente fora do diff — registrada aqui para que o commit desta sessão
não seja lido como "gate verde".

### Resolvido (2026-08-20) — e NÃO é correção de toolchain, é contorno local

**O diagnóstico inicial estava errado, e medir derrubou-o.** A hipótese era
download truncado da toolchain de módulo. Executei a correção: movi a toolchain
de lado (reversível) e forcei re-download. Vieram **exatamente as mesmas 8
ferramentas**.

**A causa real**: a toolchain de MÓDULO do Go 1.26 publica 8 ferramentas
(`asm cgo compile cover fix link preprofile vet`) e constrói as outras 10 —
`covdata`, `pprof`, `trace`, `test2json`, `nm`, `objdump`, `pack`, `doc`,
`addr2line`, `buildid` — **sob demanda no `GOCACHE`**. A prova: `go tool -n
covdata` resolve para `~/Library/Caches/go-build/.../covdata`, e
`go tool covdata` funciona.

Onde quebra: cobertura de pacote COM teste funciona; de pacote SEM arquivo de
teste falha, porque esse caminho procura em `GOTOOLDIR` em vez de usar a
resolução sob demanda. **É bug do Go 1.26**, não ambiente incompleto e não
código nosso.

`GOTOOLCHAIN=local` não serve: `/usr/local/go` é **go1.24.2** — o `go version`
dizia 1.26 porque o `auto` já trocava — e o `go.mod` exige ≥1.26. A 1.24.2 local
É completa (18 ferramentas), só velha demais.

**A alternativa tentadora foi MEDIDA e MENTE.** Manter o pacote sem teste apenas
em `-coverpkg`, sem ser alvo de teste, produz perfil com ZERO linhas dele e a
cobertura SOBE artificialmente — num experimento de dois pacotes, de 80,0% para
83,3%. Não foi proposta.

**A saída aplicada**: os quatro pacotes ganharam testes REAIS, o que faz o
caminho quebrado deixar de ser exercido sem tocar no denominador.

| pacote | propriedade travada |
|---|---|
| `cmd/listroutes` | a saída é ORDENADA e estável — o harness golden compara por diff, e ordem de mapa em Go é aleatória por desenho |
| `cmd/wss` | o argv do operador SOBREVIVE (substituir em vez de estender perderia o nome do programa e toda flag), e `--mode=stdio` não duplica |
| `cmd/core` | ele **não** injeta modo — a única diferença para o `wss` — e a tabela de rotas não é vazia |
| `testkit` | asserção em tempo de COMPILAÇÃO de que os dublês satisfazem `waclient.Client` e `store.ContactStore` (ARMADILHAS §1) |

> **Um desses testes achou algo e o defeito era MEU.** A asserção "toda rota
> declara método" falhou em `/admin`. Não é defeito do router: é
> `PathPrefix("/admin").Subrouter()` (`pkg/bootstrap/router.go:268`), ponto de
> montagem cujos métodos vivem nos filhos. A asserção foi trocada pela que
> realmente vale: rota sem método precisa ter algo montado SOB ela, porque folha
> sem método é inalcançável. 101 rotas, 1 ponto de montagem.

**Denominador CONFERIDO depois**, que era condição explícita: os quatro pacotes
continuam presentes em `coverage.out`. Cobertura 83,2% → **83,8%**, e o piso foi
subido para 838 no mesmo commit, como o próprio gate manda.

**Como registrar isto**: é **contorno local para um bug do Go 1.26** no caminho
de cobertura de pacote sem arquivo de teste — NÃO correção da toolchain. Se o Go
corrigir, os testes continuam valendo por si.

**Status**: corrigido — `make check` verde ponta a ponta (build, vet, race,
lint, coverage 838/838, log-coverage em ratchet, facade e filesize).


## F97 — o teste do launcher perdia uma corrida com a própria limpeza que ele testa

**Data**: 2026-08-20 · **Contexto**: fechamento da F96, ao rodar o gate completo.

**Onde**: `internal/wa-headless/engine/launcher_test.go`,
`TestLaunchStopsTheBrowserWhenTheEndpointNeverAnswers`.

**O sintoma**: falha com *"the browser never started, so this test did not
exercise the cleanup"* — ou seja, a PRECONDIÇÃO do teste, não o comportamento.

**A medição é o que separa flake de defeito:**

| execução | falhas |
|---|---|
| isolado, sob `-cover` | **0 de 6** |
| dentro do run de cobertura do repo inteiro | **2 de 2** |

Não é raro: é determinístico naquele modo.

**A causa**: o teste corre contra si mesmo. O `Launch` MATA o navegador quando o
endpoint não responde — que é o comportamento sob teste — mas o falso escreve o
marcador de um shell que precisa ser escalonado antes. Com orçamento de boot de
`300ms`, instrumentar todo o repositório deixa a partida de processo lenta o
bastante para o shell morrer antes do `touch`, toda vez.

**Correção**: `300ms` → `2s`, com a razão escrita no código. Nada é enfraquecido:
o endpoint continua nunca respondendo, o `Launch` continua falhando, e a limpeza
continua sendo o que se assere. O teste IRMÃO logo abaixo já usava `5s`.

**Verificado**: o run completo de cobertura, que falhava 2/2, passou limpo.

**Nota de método**: o valor `300ms` não estava protegendo nada — era só "rápido".
Um número escolhido por conveniência dentro de um teste que mata processos é um
prazo disfarçado de constante.

**Status**: corrigido — causa medida nos dois regimes, correção verificada no
regime que falhava.


## F98 — dois arquivos fora de `gofmt` desde 2026-08-08, deixados de propósito

**Data**: 2026-08-20 · **Contexto**: fechamento da F96, quando o `gofmt -l` do
gate os listou.

**Onde**: `pkg/bootstrap/config.go` e
`pkg/presentation/http/handlers/handler_session_test.go`.

**O que é**: deriva de formatação pré-existente. Confirmado que NÃO foram
tocados por este trabalho (`git show --name-only` do commit não os lista) e que
o último commit a mexer no primeiro é de 2026-08-08. O `lint` os conta como
informativo, então nada trava.

**Decisão da orquestração, e a razão dela**: NÃO formatar agora. Estão fora do
write set desta feature, e um commit mecânico aumentaria desnecessariamente a
superfície de conflito do merge futuro — que já tem 34 conflitos conhecidos com
`feature/macbook-lucas`.

**Correção sugerida**: `gofmt -w` nos dois, num commit isolado, **no closeout**,
e só se a política final exigir árvore globalmente `gofmt`-clean.

**Status**: aberto — deliberadamente adiado, com a razão registrada.

## F99 — o `coverage-gate` joga fora a evidência de que precisa quando falha

**Data**: 2026-08-20 · **Contexto**: uma execução do `make check` reprovou no
`coverage-gate` e não deixou NADA para diagnosticar.

**Onde**: `Makefile:133`.

```make
@$(GOTEST) -count=1 $(COVER_PKGS) -coverpkg=... -coverprofile=$(COVERAGE_OUT) > /dev/null
```

**Problema**: o `> /dev/null` existe para não poluir a saída no caminho feliz —
e no caminho de falha ele apaga exatamente o que se precisa ler. O log do gate
mostrou `make: *** [coverage-gate] Error 1` logo depois do lint, sem uma única
linha de `--- FAIL`, sem nome de pacote, sem nada.

Reexecutado isolado logo em seguida, passou (`coverage: 838/838`), e o
`make check` completo seguinte também (`MAKE_CHECK_EXIT=0`). Ou seja: a falha
era transitória — provavelmente da mesma família de contenção da F97 — e o gate
tornou impossível confirmar isso, porque a evidência foi descartada no instante
em que passou a importar.

**Custo concreto nesta sessão**: escrevi num commit que o gate estava "verde
ponta a ponta" tendo lido só a ausência de linhas `FAIL` — que o `> /dev/null`
garante mesmo quando há falha. A afirmação acabou verdadeira, verificada depois
com `make check` completo e `exit 0`, mas foi feita sem evidência. **O gate
convida a esse erro.**

**Correção sugerida**: mandar o stdout para um arquivo em vez do vazio, e
imprimi-lo apenas quando o passo falhar.

```make
@$(GOTEST) ... > $(COVERAGE_OUT).log 2>&1 || { cat $(COVERAGE_OUT).log; exit 1; }
```

Mantém o caminho feliz silencioso e devolve o diagnóstico no único momento em
que ele vale alguma coisa.

**Vale para os vizinhos**: o mesmo padrão `> /dev/null` deve ser procurado nos
outros alvos antes de aparecer de novo — a F97 e esta entrada são a mesma
lição vista de dois lados, uma sobre prazo escolhido por conveniência e outra
sobre silêncio escolhido por conveniência.

### Corrigido (2026-08-20), e pagou-se na PRIMEIRA execução

Aplicado sob a autorização de "hardening oportunista" da orquestração, depois de
o silêncio me morder uma SEGUNDA vez: outra execução reprovou no `coverage-gate`
mostrando apenas `make: *** [coverage-gate] Error 1`.

O stdout passou a ir para `$(COVERAGE_OUT).log` e só é impresso quando o passo
FALHA. Caminho feliz continua silencioso.

**A primeira execução com o conserto revelou uma REGRESSÃO REAL** que as duas
investigações cegas anteriores não tinham como ver:

```
coverage: 836 decimos de % (piso declarado 838) — atual 83.6%
FALHA: a cobertura caiu (83.6% < piso declarado).
```

Eu havia acrescentado código de produção (`media.go`, `resolve.go`, o ramo de
grupo) cujo `resolve.go` ficava **0%** no gate, porque só o teste AO VIVO o
exercitava — e testes ao vivo não rodam ali. O gate estava certo e eu não sabia.

**A correção foi cobrir, não baixar o piso**: oito testes novos para `Resolve`,
`Resolution.String`, `Result.String` e `kindOf`, incluindo o caso que importa —
`kindOf` recusando `notification_template`, `revoked` e `poll_creation`, para
que uma mensagem de sistema nunca sirva de prova de que a conta enviou algo.
Cobertura de volta a 838.

**A lição, e ela é a razão de esta entrada existir**: um gate que descarta a
própria saída não é só inconveniente — ele esconde regressão verdadeira e faz o
leitor gastar as investigações em fantasmas. As duas cegas anteriores foram
atribuídas a contenção (F100), e uma delas era; esta não era.

**Status**: corrigido — saída preservada e impressa na falha, com uma regressão
real de cobertura encontrada e consertada na primeira execução.

## F100 — o gate é sensível à CARGA da máquina, e isso já produziu quatro falsas falhas num dia

**Data**: 2026-08-20 · **Contexto**: quarta ocorrência em uma sessão.

**O padrão**, todas as quatro medidas nos dois regimes (isolado × sob carga):

| teste | isolado | sob carga |
|---|---|---|
| `TestBrowserChainVerifiesTheModuleInventory` | 2,08 s | **travou 18m28s** (H36) |
| `TestLaunchStopsTheBrowserWhenTheEndpointNeverAnswers` | 0/6 falhas | **2/2 falhas** (F97) |
| `TestStartSession_ConcurrentStartOnSameProfileEndToEnd` | 3/3 verdes | falhou |
| `TestStartSession_NotReadyFailure_PreservesFinalSnapshot` | 3/3 verdes | falhou |

> **ATUALIZAÇÃO 2026-08-22 (decisão 79): a CAUSA foi medida, e não era "a
> máquina estava ocupada".** Esta entrada tratou a carga como ambiental. Ela é
> produzida pelo PRÓPRIO gate: o `go test` paraleliza pacotes até ao número de
> CPUs, e quatro pacotes desta árvore lançam browsers sem coordenação alguma.
> Medido durante uma execução: **24 browsers vivos ao mesmo tempo, 4 binários de
> teste, 10 CPUs, load average 59,58.**
>
> E havia uma segunda causa, independente da carga: o helper `freePort` era
> bind-`:0`-e-fecha, um TOCTOU clássico. Com a máquina JÁ serializada e a carga
> em 9,63, um teste ainda estourou 2m30 porque a porta reservada foi tomada
> antes de o Chromium se ligar a ela — a colisão ficou visível no log como um
> `httptest.Server` ainda a segurá-la.
>
> **Correção aplicada (F103)**: os quatro pacotes de browser correm com `-p 1`,
> e os 185 sítios que pediam porta reservada passaram a usar porta efêmera, que
> não tem alocação para disputar. Resultado medido:
>
> | | browsers simultâneos | pico de carga | órfãos após | `runtime` |
> |---|---|---|---|---|
> | antes | 24 | 59,58 | 28 | 220 s (com falha) |
> | depois | 8 | 9,63 | **0** | **30,8 s** |
>
> As quatro falhas desta tabela são, portanto, candidatas a ter a MESMA causa —
> e não "a máquina estava ocupada". Antes de voltar a atribuir uma falha do gate
> a carga externa, reproduza-a com o teto ativo: se ainda acontecer, é outra
> coisa, e é essa outra coisa que precisa de ser medida.

**A causa comum**: esses testes sobem CHROME DE VERDADE. Sob carga, o navegador
não responde no `/json/version` dentro do orçamento de boot de 30 s, e o teste
reporta honestamente `launch failure` — que não é o estágio que ele queria
exercitar.

Medido no momento da falha: **load average 6,73 / 9,43 / 8,93**, sem nenhum
Chrome vazado (`pgrep -f headless=new` = 0). Ou seja, não era vazamento — era a
máquina ocupada, em boa parte pelos meus próprios `make check` sucessivos com
`-race` somados aos testes contra a SPA real.

**O que NÃO é**: não é defeito desses testes. Os dois últimos falham RÁPIDO e
com diagnóstico — exatamente o comportamento que a H36 pediu. O `launch failure`
é uma leitura correta do mundo naquele instante.

**Prática de trabalho, que é a correção real**: rodar o `make check` numa
máquina quieta, e não em paralelo com trabalho contra a SPA real. Uma execução
concorrente mede a máquina, não o código — que é literalmente a lição do
"Medir antes de projetar" no `CLAUDE.md`, item 2, aplicada ao próprio gate.

**Correção sugerida no código, se a prática não bastar**: os testes que sobem
navegador real poderiam declarar isso e ser puláveis por variável de ambiente
num modo "gate sob carga", em vez de reprovar. Mas isso troca sinal por
conveniência, e por isso NÃO está sendo proposta como padrão — só registrada
como opção consciente.

**Como distinguir na hora**: falha com `launch: browser never answered on
/json/version` ou `Boot(...): deadline of 30s exceeded` é candidata a carga.
Reexecute isolado ANTES de investigar o código; se passar 3/3, o alvo era a
máquina.

**Status**: aberto — padrão medido quatro vezes com os dois regimes, causa
identificada, correção é de prática e não de código.

**Quinta ocorrência — 2026-08-21**, e é a mais limpa como evidência:

```
--- FAIL: TestHolder_StoppedHolderRefusesToBootAgain (30.00s)
```

`git status --porcelain internal/wa-headless/runtime/` estava **vazio** — o
pacote não foi tocado na sessão. Reexecutado isolado com `-count=3 -race`:
**4,5 s, verde nas três**. Trinta segundos é o teto do teste; 4,5 s é o custo
real. A distância entre os dois é a máquina, não o código.

O que esta ocorrência acrescenta às quatro anteriores: as outras foram medidas
com o pacote alterado na mesma sessão, então "não é o código" era inferência.
Aqui é observação — árvore limpa naquele diretório.

**Sexta ocorrência — 2026-08-21**, mesmo padrão em outro pacote:

```
--- FAIL: TestStartSession_SuspectMarkerComposedPath_ClearedOnSuccess (30.00s)
```

`git status --porcelain internal/wa-headless/core/` vazio; isolado com
`-count=3 -race`: **4,6 s, verde**. Duas ocorrências observadas (não inferidas)
em pacotes diferentes no mesmo dia, ambas em testes com teto de 30 s. O padrão é
o TETO, não o pacote: 30 s é generoso para o custo real e apertado para uma
máquina carregada rodando o gate inteiro com `-race`.

Isso muda a correção sugerida: em vez de investigar cada teste, **subir o teto
dos testes que cronometram boot de navegador** — o que eles medem é
comportamento, não latência, e latência é o que a máquina carregada altera.

**Sétima ocorrência, 2026-08-21**, terceira no mesmo dia:
`TestStartSession_SettleLoopRespectsTheBudgetItWasGiven` estourou 30 s no gate e
passou 3× isolado (16,5 s no total). Três testes diferentes, dois pacotes, o
MESMO teto.

**Custo medido**: três execuções de `make check` perdidas nesta sessão. Cada uma
custa minutos e, pior, cada vermelho exige decidir se é real antes de seguir. O
ruído já é maior que o defeito.

**Não corrigido de propósito**: é defeito pré-existente fora do escopo da tarefa,
e a regra do `CLAUDE.md` manda registrar e PERGUNTAR em vez de consertar de
graça. A correção sugerida é uma linha por teste — subir o teto de 30 s para 90 s
nos que cronometram boot de navegador — e está esperando decisão.

---

### CORRIGIDO em 2026-08-21, com a distinção que a decisão exigiu

A orquestração autorizou subir para 90 s **somente nos testes de integração que
cronometram boot**, e foi explícita: *"registre 90 s como orçamento de harness,
não SLA do produto"*.

Isso mudou o conserto. O caminho óbvio era subir `engine.DefaultDeadlines.Boot`
— e teria sido **errado**: 30 s ali é decisão de produto apoiada numa medição
(SPA utilizável pré-login em 6,1–8,5 s, 30 s de folga para cache frio). Subir
esse número para calar o gate teria mudado o que o produto promete, como efeito
colateral de conveniência de teste.

O conserto real usa `StartConfig.Runner`, que já existia: os helpers de teste
(`core.baseConfig`, `runtime.holderConfig`) injetam um Runner com
`Policy.Boot = 90s` e tudo o mais em produção.

**Travado por dois testes**, e o segundo foi exigência explícita da decisão:

| teste | o que impede |
|---|---|
| `TestTheHarnessBudgetIsNotTheProductBudget` | alguém subir o deadline de PRODUTO para calar o gate; falha se `DefaultDeadlines.Boot` sair de 30 s, ou se o runner do harness mexer em qualquer outro deadline |
| `TestAHungBootStillTerminatesBounded` | o teto virar ausência de teto — dá um orçamento minúsculo a um boot que nunca fica pronto e prova que ele **desiste dentro do orçamento** |

O segundo usa orçamento pequeno de propósito: a propriedade é "o deadline limita
a espera", e prová-la com 90 s custaria 90 s para aprender o mesmo.

**Status**: corrigido.

### OITAVA OCORRÊNCIA, 2026-08-21 — e ela mostra que o conserto foi ESTREITO demais

```
--- FAIL: TestBrowserChainReportsAWedgedPageAsUnresponsive (50.00s)
--- FAIL: TestStartSession_FailureTearsDownDeterministicallyWithNoOrphan (32.05s)
```

Árvore limpa nos dois pacotes; isolados, passam em **9,5 s** contra tetos de 50 s
e 32 s. Na execução seguinte do `make check`, sem tocar em nada, verde.

O conserto anterior injetou o orçamento de harness em `core.baseConfig` e
`runtime.holderConfig`. **Estes dois testes não passam por lá** — têm prazos
próprios, escritos no corpo deles.

Ou seja: eu consertei os três testes que estavam falhando, não a CLASSE de
problema. O `TestTheHarnessBudgetIsNotTheProductBudget` protege o deadline de
produto e não diz nada sobre testes que trazem o próprio relógio.

**Correção sugerida, não aplicada**: os prazos escritos dentro de testes de
integração deveriam vir do mesmo `harnessBootBudget`, para que exista UM número a
ajustar. Fica registrado em vez de emendado agora, porque mexer nos prazos de
dois testes que acabaram de falhar é o momento errado para decidir qual é o valor
certo.

---

## H142 — o zero da H106 era do DADO, não da página: menções fechadas produzindo uma

**Data**: 2026-08-22
**Contexto**: fechar as três últimas linhas `MISSING` do `LEDGER-WWEBJS.md`
(`getMentions`, `getGroupMentions`, `MESSAGE_REVOKED_ME`).

**Onde**: `internal/wa-headless/capabilities/message/script.go` (`mentionsScript`),
`internal/wa-headless/capabilities/message/message.go` (`MentionsOf`).

**Problema**: a H106 mediu 395 mensagens carregadas e achou ZERO menções sob
cinco nomes de campo candidatos, e concluiu — corretamente — que embarcar um
leitor nunca visto devolvendo algo seria a armadilha H93. A entrada ficou
`MISSING` com essa medição anexada e parecia encerrada.

**O que estava errado não era a medição, era a leitura dela.** O zero não dizia
"a página não expõe menções". Dizia "ninguém nesta conta jamais mencionou
ninguém" — e essas são afirmações diferentes, com remédios diferentes. A
primeira é um beco; a segunda é falta de fixture.

**Evidência**: produzida UMA menção no grupo de laboratório, mencionando o par
resolvido por `lookup.NumberID` e o próprio grupo. A varredura seguinte, sobre
62 mensagens:

```
mentionedJidList  viaGetter 2  viaUnderscore 2
                  entrada: object, keys [_serialized, server, user]
groupMentions     viaGetter 2  viaUnderscore 2
                  entrada: object, keys [groupJid, groupSubject]
```

Os dois campos respondem igual pelo getter e pelo `__x_`, o que torna o getter o
caminho estável.

**A armadilha da H131 apareceu de novo, e o primeiro instrumento caiu nela.**
A varredura inicial listou NOMES de campo e reportou `__x_nonJidMentions` como
"preenchido" em **46 de 62** mensagens — inclusive em mensagens que não mencionam
ninguém. O campo não é array: é sentinela preguiçosa. Um leitor construído sobre
aquela varredura diria que quase toda mensagem menciona alguém. O conserto foi
medir a FORMA em vez do nome, e a regra virou código: `mentionsScript` só itera
o que passa em `Array.isArray`.

**Correção aplicada**: `Reader.MentionsOf` devolve `Mentions{People, Groups}`.
Pessoas e grupos são campos separados porque a página os carrega em campos
separados com formas diferentes — fundi-los perderia o assunto e mentiria sobre
a espécie. O assunto do grupo vem **congelado na mensagem**, não resolvido do
grupo de hoje: é o que foi dito na hora.

**Divergência consciente da referência**: o `getMentions` de lá mapeia cada id
por `getContactById` e devolve `Contact`. Aqui devolve identidades, porque
`contacts.Recipients` (H132) já resolve lista de jids reportando encontrados e
ausentes SEPARADAMENTE — distinção que a versão da referência transforma em
objeto meio vazio.

**Status**: corrigido. Travado por:
- `TestTheMentionsScriptDoesNotReadTheSentinelField` — CN: trocar
  `m.mentionedJidList` por `m.nonJidMentions`. Falha com *"the script reads
  nonJidMentions, which is not an array and was measured as present on 46 of 62
  messages including ones that mention nobody"*.
- `TestTheMentionsScriptTakesOnlyArrays` — CN: `const list = v => v || []`.
  Falha com *"the script does not check Array.isArray before iterating"*.
- `TestPeopleAndGroupsDoNotShareAList` — CN: `m.People = append(m.People, g.JID)`.
  Falha.
- `TestTheMentionsRenderingIsQuiet` — CN: imprimir `m.Groups`. Falha com
  *"the rendering leaks \"120363\""*.
- `TestAnUnloadedMessageHasNoMentionsAnswer`,
  `TestAnEmptyIDNeverReachesThePageForMentions`.

Prova em SPA real: `TestProbeProduceMention` envia 1 pessoa + 1 grupo e lê de
volta por `MentionsOf` — `people=1 groups=1`, identidade batendo com a resolvida
e assunto não vazio.

**Lição, e é a terceira vez que ela cobra**: *uma ausência medida precisa dizer
se é ausência de CAPACIDADE ou ausência de DADO.* A H114 já tinha ensinado que
um zero precisa de controle positivo; aqui o controle positivo era produzir o
fato. As linhas ficaram três meses `MISSING` por uma medição correta lida como
conclusão errada.

---

## H143 — `MESSAGE_REVOKED_ME`: o diagnóstico estava certo, e a pós-condição estava no lugar errado

**Data**: 2026-08-22
**Contexto**: idem H142 — última linha `MISSING`.

**Onde**: `internal/wa-headless/capabilities/revoke/revoke.go` (`ForMe`,
`waitGone`, `loadedScript`), `internal/wa-headless/events/events.go`
(`MessageRemoved`), `internal/wa-headless/events/ingress.go` (`onRemove`).

**Problema**: a nota da H88 dizia *"não temos 'apagar para mim'; é falta de
MÉTODO antes de ser falta de evento"* — e estava certa. O `revoke` só oferecia
`ForEveryone`, apesar de o próprio `ErrNotRevocable` já dizer ao chamador
*"apague localmente"*: o pacote nomeava um remédio que não vendia.

**Nome enumerado, não adivinhado.** Três nomes de função inventados a partir da
referência não existiam neste build esta semana (H134, H137). Desta vez o módulo
foi enumerado antes:

```
Cmd delete surface: clearChat, clearCurrentChatConversationHistory,
clearSelectedChats, deleteOrExitChat, deleteOrExitChatFromEntryPoint,
newsletterDeleteDrawer, sendDeleteMsgs
sendDeleteMsgs: aridade 6   (a referência passa 3; o resto tem padrão)
```

**A medição que eu não teria adivinhado**: a primeira versão verificava a
pós-condição DENTRO da página, imediatamente depois do `await sendDeleteMsgs`.
Contra produção, falhou:

```
sent: send.Result(id=3EB0078A0EE0866F6826CC ack=1 waited=1ms)
ForMe: revoke: the page accepted the local deletion and the message is still loaded
```

`sendDeleteMsgs` **resolve antes de a coleção soltar o modelo**. Movida a espera
para o Go — que é onde a invariante 6 manda o relógio ficar —, o tempo real
medido foi **4,539 s**. A verificação na página estava garantida a falhar; um
`sleep` na página teria "consertado" o sintoma violando a invariante.

Por isso o script deixou de responder `loaded`: uma página que reportasse a
pós-condição a reportaria no único instante em que ela está garantidamente
errada.

**O evento é `message.removed`, não `message.revoked`, e a distinção é o motivo
de ele existir.** Revogar é fato da CONVERSA — some do telefone de todo mundo e
os dois lados veem. Apagar local é fato deste APARELHO — ninguém mais percebe.
Emitir um pelo outro diria ao assinante que uma mensagem sumiu para todos quando
sumiu só aqui. O ouvinte é `MsgCollection.on('remove')` filtrado por `isNewMsg`,
o mesmo filtro da referência e pelo mesmo motivo: sem ele, cada despejo de
conversa antiga viraria "alguém apagou isto".

**`ForMe` não consulta `canSenderRevokeMsg`**, e isso é decisão: aquela pergunta
é sobre as cópias DE OUTRAS PESSOAS. Consultá-la aqui recusaria — em nome delas —
um ato que nunca sai deste aparelho.

**Status**: corrigido. Travado por:
- `TestALocallyDeletedMessageStillLoadedIsAFailure` — CN: remover a chamada a
  `waitGone`. Falha.
- `TestTheLocalDeleteDoesNotConsultTheRevokeEntitlement` — CN: introduzir
  `canSenderRevokeMsg` no script. Falha.
- `TestTheLocalDeleteVerifiesAgainstTheCollection` — CN: idem `waitGone`. Falha
  com *"the postcondition never asked the collection whether the message is still
  there"*.
- `TestALocalDeleteThatLandsReportsItself`,
  `TestAnEmptyIDNeverReachesThePageForALocalDelete`,
  `TestAMessageNotLoadedCannotBeDeletedLocally`.

Prova em SPA real: `TestProbeDeleteForMe` envia, mede a LINHA DE BASE
(`message.removed before the delete: 0`), apaga, e observa `message.removed`
nomeando exatamente a mensagem apagada (`total 1, was 0`).

**Lição**: *a pós-condição tem de ser lida no relógio de quem espera.* A
invariante 6 é normalmente enunciada como "não decida na página"; este caso
mostra a outra metade — **não VERIFIQUE na página**, porque verificar cedo demais
é decidir com a informação errada.

---

## H144 — a sessão dupla não destravou a presença, e o que ela produziu foi melhor: a causa medida

**Data**: 2026-08-22
**Contexto**: reauditar os 56 `PARTIAL` procurando os que ficaram acionáveis por
causa das capacidades novas de hoje.

**Onde**: `internal/wa-headless/probe_presence2_test.go` (novo),
linhas `sendPresenceAvailable` e `sendPresenceUnavailable` do `LEDGER-WWEBJS.md`.

**Hipótese**: as duas linhas estavam `PARTIAL` desde a H50 com a nota
*"observação não provada; exige as duas contas na agenda uma da outra"*. Aquilo
era uma SUPOSIÇÃO sobre a causa — plausível, nunca medida —, escrita quando não
havia como acordar as duas contas ao mesmo tempo. A sessão dupla (H135) parecia
ser exatamente a peça que faltava.

**Não era.** Com conta-A e conta-B acordadas simultaneamente, conta-B anunciando
disponível e conta-A observando, `Observe` nunca chega a `subscribed` em 45 s de
tentativa.

**A medição que valeu a rodada** foi a que perguntou POR QUÊ, em vez de concluir
a partir da falha:

```
conta-B in conta-A's address book: found=true
record: contacts.Contact(identity=<redacted> pn=true lid=true merged=true)
address-book flags: {"inCollection":true, "isMyContact":false,
                     "isAddressBookContact":false, "isWAContact":false}
```

O par ESTÁ na coleção, com PN e LID fundidos — a identidade está resolvida e
correta. O que falta é o vínculo de AGENDA, e `isMyContact` e
`isAddressBookContact` leem `false` nos dois. A assinatura de presença exige esse
vínculo, e ele se cria no TELEFONE, salvando o contato.

**O `Contact` deste módulo não carrega esses sinalizadores**, e foi por isso que
a leitura teve de ser crua: `contacts.ByJID` respondeu `found=true`, que é
resposta a *"existe no WhatsApp"* — pergunta diferente de *"está na agenda"*.
Ler a primeira como a segunda é o erro que teria mantido a hipótese viva.

**Correção aplicada**: nenhuma no código. As duas linhas continuam `PARTIAL`, mas
a nota deixou de ser hipótese e virou medição, e a pendência mudou de categoria:
é **dependência humana** (salvar o contato no telefone), não trabalho parado
deste módulo. Fica na lista de dependências humanas junto com foto de perfil e
status.

**Status**: não corrigido — e o "não corrigido" agora tem causa em vez de
suspeita.

**Lição**: *uma capacidade nova não destrava o que ela não toca, e descobrir isso
depressa vale a rodada.* A tentação era registrar "a sessão dupla não resolveu" e
seguir. O que fez a rodada valer foi a segunda pergunta — POR QUE não —, que
transformou uma nota de três meses em fato verificável. A H142 é o mesmo
movimento com o sinal trocado: lá, perguntar por que o zero era zero abriu a
linha; aqui, fechou a dúvida.

**Achado incidental**: `contacts.Contact` não expõe `isMyContact` /
`isAddressBookContact`, e a distinção entre "existe no WhatsApp" e "está na minha
agenda" é real e útil — o `isMyContact` da referência existe exatamente para
isso. **Correção sugerida, não aplicada**: acrescentar os dois campos ao
`Contact`, medindo antes a distribuição deles sobre as 944 entradas desta conta
(um campo que lê `false` em 944 de 944 não é campo, é ruído). Fora do escopo da
tarefa atual.

---

## H145 — `description`: a metade aberta do leitor não é trabalho parado, é consequência de escrita bloqueada

**Data**: 2026-08-22
**Contexto**: continuação da reauditoria dos `PARTIAL` (H144).

**Onde**: `internal/wa-headless/probe_gdesc2_test.go` (novo), linha `description`
do `LEDGER-WWEBJS.md`.

**Hipótese**: a linha `description` diz que o leitor está entregue e **nunca foi
observado não-vazio** — no grupo de laboratório `desc` e `displayedDesc` leem
`undefined`, o que é compatível com *"o grupo não tem descrição"* E com *"o
leitor olha o campo errado"*. Era candidata natural à manobra da H142: produzir o
dado com `group.SetDescription` (H126) e observar o leitor.

**Medição**:

```
BEFORE: descLen=0 source="none"
SetDescription: group: the description did not take:
                asked for 35 bytes and the server reports 0
```

A escrita reproduziu, pela **terceira vez independente**, o bloqueio já medido em
canal (H113) e em grupo (H126): a página ACEITA a chamada e o servidor nunca
armazena. A pós-condição do `SetDescription` mordeu corretamente e recusou
declarar sucesso — invariante 14 funcionando.

**O que isto estabelece, e o ledger não dizia**: não existe, dentro deste módulo,
produtor para o dado que o leitor precisaria observar. A metade aberta da linha
`description` não é trabalho pendente — é consequência de uma linha `BLOCKED`.

Isso muda a classificação prática de *"PARTIAL, talvez acionável"* para
*"PARTIAL, comprovadamente não acionável aqui"*, que é exatamente a distinção que
o critério de encerramento da Fase 1 (decisão 52/62) pede e que não estava
registrada.

**Status**: não corrigido, por não haver o que corrigir. A sonda ficou no repo e
faz `Skip` com a mensagem do bloqueio em vez de falhar: se o servidor um dia
passar a armazenar, ela deixa de pular e a linha volta a ser acionável sozinha.

**Lição**: *nem toda metade aberta é trabalho; algumas são sombra de outra
linha.* Varrer `PARTIAL` procurando o que ficou acionável só é honesto se
também registrar o que ficou provado INACIONÁVEL — senão a mesma linha é
reexaminada a cada varredura, e cada varredura paga de novo o custo de descobrir
o mesmo bloqueio.

---

## H146 — `getBlockedContacts`: "não é exposto" era afirmação sobre a nossa superfície, não sobre a página

**Data**: 2026-08-22
**Contexto**: varredura sistemática dos 56 `PARTIAL` (continuação de H144/H145).

**Onde**: `internal/wa-headless/capabilities/block/block.go` (`List`, `listScript`),
linha `getBlockedContacts` do `LEDGER-WWEBJS.md`.

**Problema**: a linha dizia *"bloquear/desbloquear provados; LISTAR os bloqueados
não é exposto"*. Isso é verdade sobre o NOSSO módulo e nunca foi conferido contra
a página. Enumerar — a técnica que achou o `sendDeleteMsgs` na H143 depois de três
nomes inventados falharem — respondeu em uma chamada:

```
collections: {"Blocklist": 0}
WAWebBlocklistCollection: ["BlocklistCollection"]
WAWebBlockContactAction: ["blockContact","unblockContact","updatePSAUserBlockingStatus"]
WAWebContactBlockStore: empty
WAWebBlocklistStore: empty
contactFlag: {contacts: 945, withIsBlocked: 0, blocked: 0}
```

**Dois achados, e o segundo é o que salva o leitor.** A coleção EXISTE e responde
`getModelsArray` — lia 0 porque ninguém está bloqueado nesta conta, não porque
esteja ausente. E o caminho que um implementador apressado tomaria — filtrar o
roster por `isBlocked` — **não existe**: de 945 contatos, ZERO carregam o campo.
Uma versão assim devolveria vazio para sempre e pareceria saudável fazendo isso.

**Correção aplicada**: `Blocker.List`. É um alargamento deliberado do que o
pacote emite: o `Result` sempre carregou `Before`/`After` como TAMANHOS, com um
"nunca as entradas" escrito — narrowing correto quando a única pergunta era "a
mudança pegou". Um número não diz a quem desbloquear.

**Status**: corrigido. Travado por:
- `TestARefusedReadIsNotAnEmptyBlocklist` — CN: remover a checagem de `out.OK`.
  Falha. É a distinção que torna o método usável: "ninguém bloqueado" e "a
  leitura falhou" têm a mesma forma depois que um erro é engolido, e quem agisse
  sobre a primeira desbloquearia ninguém acreditando ter conferido.
- `TestTheListReadsTheBlocklistAndNotTheRoster` — CN: trocar
  `BlocklistCollection` por `ContactCollection`. Falha.
- `TestAnEmptyBlocklistIsNotAnError` — CN: erro quando a lista é vazia. Falha.
- `TestTheListNamesWhoIsBlocked`.

Prova em SPA real (`TestProbeBlocklistNamed`), com restauração no padrão da H27:
`0 → 1` nomeando o par → `0`. O desbloqueio é agendado ANTES de qualquer
asserção — registrá-lo depois deixaria o par bloqueado justamente quando uma
verificação falhasse, que é quando o fixture mais importa.

**Lição**: *uma nota que diz "não é exposto" precisa dizer POR QUEM.* "Nós não
expomos" e "a página não oferece" são fatos diferentes com custos diferentes, e
a linha os fundia havia meses.

---

## H147 — `isRegisteredUser`: a nota estava metade obsoleta e metade certa por outro motivo

**Data**: 2026-08-22
**Contexto**: idem H146.

**Onde**: `internal/wa-headless/probe_notreg_test.go` (novo), linha
`isRegisteredUser` do `LEDGER-WWEBJS.md`.

**Problema**: a linha dizia *"é passo interno de todo envio; não exposto"* e
apontava para `spa.ResolveIdentityExpr`. A H127 criou `capabilities/lookup`
exatamente para essa classe de nota — o doc do pacote diz, com todas as letras,
que cinco linhas estavam `PARTIAL` pelo mesmo motivo. Esta ficou para trás.

**Mas corrigir o mapeamento não bastava**, e é aqui que a linha ganhou algo que
não tinha: a referência define `isRegisteredUser(id)` como
`Boolean(await getNumberId(id))`, e toda prova ao vivo desta resolução até hoje
perguntou sobre um número que **EXISTE**. Uma capacidade só vista dizendo "sim"
passa em qualquer teste que só pergunte por números reais — inclusive uma que
respondesse "sim" para tudo.

**Medição**, com controle positivo na MESMA sessão (a regra da H114):

```
positive control:    lookup.Identity(jid=true group=false resolved=true)
implausible number:  lookup.Identity(jid=false ...) err=lookup: that number is not on WhatsApp
```

O controle positivo não é cerimônia: sem ele, um "não existe" é compatível com a
resolução inteira estar quebrada.

**Sobre o alcance da consulta**: `queryWidExists` é a mesma pergunta que todo
envio faz antes de despachar. Nenhuma mensagem sai, nenhuma conversa é aberta, e
o número usado é sintaticamente válido e deliberadamente implausível — nenhuma
pessoa real é implicada pela pergunta.

**Status**: corrigido, linha para `PROVEN` apontando para `lookup.NumberID`.

**Lição**: *quando uma capacidade tem duas respostas, provar uma prova metade.*
Vale para toda linha que devolve booleano ou erro-como-resposta, e esta varredura
deveria procurar outras: uma guarda só exercitada pelo caminho de recusa foi a
armadilha nº 2 do `ARMADILHAS.md`; esta é a mesma armadilha com o sinal trocado.

---

## H148 — `getContactDeviceCount`: o registro sempre esteve lá, sob a outra identidade

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`, seguindo a lição da H147 (procurar linhas
provadas por metade).

**Onde**: `internal/wa-headless/probe_devcount_test.go` (novo),
`internal/wa-headless/capabilities/addressbook/addressbook.go` (doc de
`DeviceCount`), linha `getContactDeviceCount` do `LEDGER-WWEBJS.md`.

**Hipótese**: a linha dizia *"o caminho funciona e o par NÃO tem registro de
dispositivo nesta conta"* (H90). A hipótese desta rodada era que o registro
apareceria com o par ACORDADO e depois de tráfego real — daí a sessão dupla e uma
mensagem.

**A hipótese era irrelevante.** O registro já existia antes da mensagem, e a
sessão dupla não teve nada a ver com isso. Medidos os dois jids lado a lado, na
mesma sessão:

```
BEFORE (resolved):  count=5   err=<nil>
BEFORE (asked-for): count=0   err=... the page has no device record for that user
AFTER  (resolved):  count=5   err=<nil>
AFTER  (asked-for): count=0   err=... the page has no device record for that user
```

A H90 mediu contra o jid de TELEFONE num build LID-first — a mesma armadilha que
a H136 nomearia meses depois, cometida antes de ela existir. O caminho estava
provado e a linha ficou `PARTIAL` por causa da identidade usada na medição, não
por causa da capacidade.

**A decisão original continua certa**: não fundir "sem registro" no número 0 foi
o que tornou este diagnóstico possível. Se a implementação tivesse devolvido 0
para o jid de telefone, as duas leituras teriam sido `0` e `5`, e a diferença
pareceria variação de dado em vez de erro de identidade.

**Correção aplicada**: doc de `DeviceCount` passa a dizer que a identidade tem de
ser a resolvida, com os dois números medidos, porque **as duas respostas são bem
formadas** — e é isso que torna o erro invisível: a resposta do jid de telefone
parece um fato sobre a pessoa e é um fato sobre o argumento.

**Status**: corrigido (linha para `PROVEN`).

**Achado incidental, NÃO corrigido**: `DeviceCount` aceita jid não resolvido e
responde "sem registro", indistinguível de um usuário genuinamente desconhecido.
**Correção sugerida**: ou resolver internamente via `capabilities/lookup`, ou
recusar um jid `@c.us` com erro próprio (`ErrUnresolvedIdentity`) em vez de
responder sobre ele. A primeira esconde uma chamada de rede dentro de um leitor;
a segunda quebra chamadores existentes. **Não aplicado sem perguntar**, conforme
a regra do projeto — e a pergunta vale para TODO leitor deste módulo que aceite
jid cru, não só para este. É decisão de superfície, não conserto pontual.

**Lição**: *quando duas identidades nomeiam a mesma pessoa, toda medição precisa
dizer com qual foi feita.* A H136 aprendeu isso num envio; esta linha mostra que
o mesmo erro contamina MEDIÇÕES antigas que ninguém suspeita — e que o custo é
uma linha parada por meses com diagnóstico errado.

---

## H149 — `getAbout`: a hipótese da H148 foi testada e DESCARTADA

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`, aplicando a lição da H148.

**Onde**: `internal/wa-headless/probe_about2_test.go` (novo), linha `getAbout`.

**Hipótese**: depois da H148 — em que uma medição antiga contra o jid de telefone
tinha produzido um diagnóstico errado por meses — era natural suspeitar do mesmo
em `getAbout`, cuja nota (H70) diz que o par tem recado vazio e em cache.

**Resultado: negativo, e por isso está aqui.** Os dois jids, na mesma sessão:

```
resolved:  contacts.About(len=0 fetched=false waited=510ms)
asked-for: contacts.About(len=0 fetched=false waited=515ms)
```

Idênticos. A identidade NÃO é a causa aqui, e o dado que importa é o
`fetched=false` nos **dois**: o caminho de servidor não é exercitado por nenhuma
das identidades. A nota da H70 está intacta.

**Status**: não corrigido, e a linha continua `PARTIAL` com a mesma causa de
antes — agora com uma hipótese a menos.

**Lição, e é a razão de gastar uma entrada num resultado negativo**: *uma
generalização recém-aprendida é exatamente o que vai ser aplicada em excesso.* A
H148 ensinou "toda medição precisa dizer com qual identidade foi feita", e a
tentação imediata é reler todo `PARTIAL` como se fosse o mesmo erro. Testar e
registrar o descarte é o que impede a próxima varredura de pagar de novo pela
mesma suspeita — do mesmo jeito que a regra do `ARMADILHAS.md` sobre dublês
permissivos existe porque a lição sem o contra-exemplo vira superstição.

---

## H150 — `chat.changed` refinado por campo: tentado, medido, e a página não sustenta

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`. `CHAT_ARCHIVED` e `UNREAD_COUNT` estavam
`PARTIAL` pela MESMA causa — *"o nosso evento é grosso: diz que a conversa mudou,
não QUAL campo"* (H87) —, o que fazia delas o item de maior alcance da varredura:
uma medição, duas linhas.

**Onde**: `internal/wa-headless/probe_chatfields_test.go` (novo), linhas
`CHAT_ARCHIVED` e `UNREAD_COUNT` do `LEDGER-WWEBJS.md`.

**Hipótese**: a referência emite um evento por campo. Se a coleção entregasse os
atributos alterados ao ouvinte, a classificação iria para o Go — exatamente o
padrão dos subtipos `gp2` da H119, que respeita a invariante 6: a página carrega
a palavra, o Go decide o que ela significa.

**Primeira medição, e a primeira armadilha**: `model.changed`, que é o nome
Backbone, é **nulo em 40 de 40** eventos. Ler o nome que a referência usaria
daria zero para sempre. Mas o modelo expõe `__fired` e `__changes`, contabilidade
própria do build, e `__fired` vinha populado — o que parecia a saída:

```
40 eventos de UM envio: msgsChanged:24, msgsLength:8, lastReceivedKey:8
```

**Segunda medição, com os atos SEPARADOS, e é ela que decide.** A primeira
juntava tudo numa contagem só, e uma lista de nomes sem dono não responde à
pergunta — que é justamente de quem é cada nome. Rotulando cada ato:

```
send:      typing, msgsChanged, msgsLength, createdLocally,
           markedUnread, ftsCache, lastReceivedKey
archive:   showUnreadInTitle ×2
unarchive: showUnreadInTitle ×2
unread:    pendingAction ×2
```

**Três fatos que matam o desenho:**

1. **Arquivar e desarquivar são INDISTINGUÍVEIS** — os dois disparam
   `showUnreadInTitle` e nada mais. Não existe `archive` em `__fired`.
2. **Marcar não-lida dispara `pendingAction`**, não `markedUnread`.
3. **`markedUnread` dispara durante um ENVIO** — ou seja, o nome existe e é
   emitido pelo ato errado.

`__fired` carrega campos derivados de UI, não o campo semântico. Um classificador
construído sobre ele diria "arquivada" para um desarquivamento.

**Status**: não corrigido, por medição. As duas linhas continuam `PARTIAL` e
agora dizem POR QUÊ, em vez de descrever o sintoma.

**Lição**: *quando um instrumento junta várias causas numa contagem, ele mede a
soma e não a correspondência.* A primeira rodada teria sustentado a hipótese —
`markedUnread` estava lá, afinal. Separar os atos mostrou que ele veio do envio.
É a mesma disciplina da regra "meça onde deveria PIORAR": aqui, meça onde o nome
deveria APARECER, e veja se aparece pelo motivo certo.

---

## H151 — identidade crua na superfície: três capacidades, três respostas enganosas diferentes

**Data**: 2026-08-22
**Contexto**: consolidação de um padrão que apareceu três vezes hoje em lugares
independentes.

**Onde**: `capabilities/addressbook.DeviceCount`, `capabilities/chats.ByJID`,
`capabilities/chats.MarkUnread`. Sonda em
`internal/wa-headless/probe_unreadid_test.go`.

**O padrão**: este build arquiva sob LID. Um chamador que tenha o jid de telefone
— que é o que um humano digita e o que a maior parte das APIs recebe — recebe
resposta **bem formada e errada**, e cada capacidade erra de um jeito diferente:

| capacidade | com jid de telefone | com identidade resolvida |
|---|---|---|
| `addressbook.DeviceCount` | erro "sem registro de dispositivo" | **5** dispositivos |
| `chats.ByJID` | `ErrNoChat` ("no conversation for that jid (384 in this session)") | encontrado |
| `chats.MarkUnread` | `ErrNoChat` ("no such conversation") | executa (`before=21`) |

**Hipótese testada e DESCARTADA**: suspeitei que `chats` se contradissesse
internamente — `ByJID` documentado como casando PN ou LID e `MarkUnread` não.
Não é o caso: os dois são exatos e concordam. A promessa de casar PN **ou** LID é
do `contacts.ByJID`, que é outro pacote. Registro o descarte para ninguém
reexaminar.

**O defeito real não é de implementação, é de SUPERFÍCIE**: o erro é honesto
sobre a COLEÇÃO e enganoso sobre o MUNDO. *"Não há conversa para esse jid"* é
verdade sobre o que está indexado e falso sobre a pessoa, e o chamador não tem
como saber a diferença.

**Correção sugerida, NÃO aplicada** — e é decisão de superfície, não conserto
pontual, então vale para todo leitor do módulo que aceite jid cru:

- **(a)** resolver internamente via `capabilities/lookup`. Esconde uma ida à rede
  dentro de um leitor, e um leitor que faz E/S surpreende quem o chama em laço.
- **(b)** recusar `@c.us` com erro próprio (`ErrUnresolvedIdentity`) dizendo
  "resolva primeiro". Honesto e quebra chamadores existentes.
- **(c)** aceitar as duas formas na busca, como `contacts.ByJID` já faz. Consistente
  com um precedente do próprio módulo, e o mais barato — mas espalha a regra de
  identidade por cada leitor em vez de centralizá-la.

**Status**: não corrigido. Escalado como pergunta de desenho — a regra do projeto
proíbe consertar de graça fora do escopo, e as três opções têm custos
diferentes o bastante para que a escolha não seja minha.

**Lição**: *o mesmo erro cometido em três lugares não é três bugs, é uma decisão
que ninguém tomou.* Cada um sozinho pareceria um conserto de uma linha; juntos
mostram que o módulo nunca decidiu quem resolve identidade — o chamador ou a
capacidade — e a ausência dessa decisão é o que produz três comportamentos
diferentes para a mesma pergunta.

---

## H152 — `MESSAGE_CREATE`: a nota errava sobre a referência, e o teste unitário fixava o campo que decidia

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`.

**Onde**: `internal/wa-headless/probe_msgcreate_test.go` (novo),
`internal/wa-headless/events/ingress_test.go` (`rowFrom`,
`TestBothDirectionsSurviveTheBoundary`, `TestTheIngressScriptReadsFromMe`),
linha `MESSAGE_CREATE` do `LEDGER-WWEBJS.md`.

**Problema**: a linha dizia *"o mesmo evento cobre os dois; o upstream distingue
criada de recebida e nós não"*. A segunda metade é falsa, e ler a referência
resolve em dez linhas (`client.js:648-664`):

```js
this.emit(Events.MESSAGE_CREATE, message);
if (msg.id.fromMe) return;
this.emit(Events.MESSAGE_RECEIVED, message);
```

O único discriminador é `msg.id.fromMe` — **o mesmo campo que o nosso
`message.added` já carrega**. Nosso evento não "cobre os dois" por imprecisão:
ele É o `MESSAGE_CREATE`, um para um, e o `MESSAGE_RECEIVED` é ele filtrado por
um campo que já viaja. A linha `MESSAGE_RECEIVED` já estava `PROVEN` com o mesmo
mapeamento, o que torna a assimetria ainda mais claramente um erro de nota.

**Mas corrigir a nota não bastava, e o que faltava é o achado desta entrada.**
Para que a afirmação seja verdadeira, o campo tem de **variar** — e nada provava
isso:

- **Nenhum teste unitário jamais viu `fromMe:false`.** O helper `row()` deste
  pacote fixa `"fromMe":true` em toda linha de fixture, então todo teste de
  ingresso via mensagens desta conta. Um campo que só carrega um valor é um campo
  que nenhum teste está conferindo.
- **Nenhuma medição ao vivo tinha as duas direções**, porque até a sessão dupla
  (H135) não havia como fazer o par falar enquanto observávamos.

**Medição** (sessão dupla, barramento em conta-A, uma mensagem em cada direção):

```
baseline: fromMe=0 notFromMe=0
PROVEN:   message.added carried BOTH values of FromMe (own=27, incoming=14)
```

**Status**: corrigido, linha para `PROVEN`. Travado por:
- `TestBothDirectionsSurviveTheBoundary` — CN: marcar o campo `FromMe` do decode
  como `json:"-"`. Falha com *"the two directions did not arrive as two:
  fromMe=0 incoming=2"*.
- `TestTheIngressScriptReadsFromMe` — CN: trocar `fromMe: !!(id && id.fromMe)`
  por `fromMe: false` no script. Falha.
- `rowFrom`, que existe para que o próximo teste deste pacote possa escolher a
  direção em vez de herdar `true`.

**Lição**: *um fixture que fixa um campo esconde o campo.* O `row()` deste pacote
foi escrito para testar OUTRA coisa, e o `true` era uma escolha inocente de
conveniência — que depois virou a razão de uma linha do ledger ficar `PARTIAL`
por meses. Vale a pergunta em toda suíte: **que campo dos meus fixtures nunca
mudou de valor?** Esse é o campo que ninguém está testando.

---

## H153 — `getInfo`: a medição da H71 estava certa e a conclusão dela, errada

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`.

**Onde**: `internal/wa-headless/capabilities/message/message.go` (`InfoOf`,
`Info`, `ErrNotMine`), `.../script.go` (`infoScript`),
`internal/wa-headless/probe_msginfo_test.go` (novo), linha `getInfo`.

**Problema**: a linha dizia *"MsgInfoCollection VAZIA (0 de 368); temos ack, não
'quem leu'"* (H71). A contagem estava correta — a coleção lê **0 ainda hoje** —
e a conclusão tirada dela não: a referência **nunca lê essa coleção**. Ela chama
`WAWebApiMessageInfoStore.queryMsgInfo(msg.id)` (`wwebjs_message.js:758-781`), e
a coleção é populada **pela** consulta, não em vez dela.

**É a armadilha da H142 com outra roupa**: medir um cache antes de alguém
enchê-lo. Lá o zero era ausência de dado produzível; aqui é ausência de dado
*solicitável*. Nos dois casos a leitura correta do zero era "ninguém pediu", não
"não existe".

**Enumeração antes da chamada** (regra da H143):

```
WAWebApiMessageInfoStore: [RetryEligibilityResult, createOrMergeReceiptRecords,
                           isRetryEligible, queryMsgInfo, queryMsgInfos,
                           getHighestMsgAcks]
WAWebMsgInfoCollection:   [MsgInfoCollection]      msgInfoSize: 0
```

**Medição da forma**, numa mensagem própria enviada ao grupo de laboratório:

```
delivery: array[1]   deliveryRemaining: number
read:     array[0]   readRemaining:     number
played:   array[0]   playedRemaining:   number
```

**Correção aplicada**: `Reader.InfoOf`, devolvendo `Info` com as três LISTAS e os
três contadores de restantes. Três decisões que o tipo carrega:

1. **Listas, não contagens.** Em um-para-um o `ack` já diz tudo; em grupo,
   *"dois de cinco leram"* é fato diferente de *"estes dois leram"*, e só o
   segundo permite agir.
2. **Os "restantes" vêm da página, não de subtração.** Derivá-los exigiria o
   número de participantes NO MOMENTO DO ENVIO, que não é o tamanho do grupo
   hoje — erraria calado assim que alguém saísse. `-1` distingue "a página não
   disse" de zero, distinção que este módulo já pagou duas vezes (H90, H108).
3. **`Answered` separa "ninguém recebeu" de "ninguém perguntou".** Sem ele, uma
   mensagem jovem demais voltaria como três listas vazias, que se lê como falha
   de entrega.

**A guarda de posse fica ANTES da consulta**, como na referência
(`if (!msg.id.fromMe) return null`). Não é cortesia: o servidor responde sobre
entrega do que ESTA conta mandou, e perguntar sobre mensagem alheia é pergunta
sem sentido, não erro de permissão — daí `ErrNotMine` e não `ErrRead`.

**Status**: corrigido, linha para `PROVEN`. Travado por cinco controles negativos,
todos EXECUTADOS e todos falhando:
- `TestTheInfoScriptChecksOwnershipBeforeQuerying` — CN: mover a guarda para
  depois da consulta. Falha (é teste de ORDEM, que passa em todos os outros).
- `TestTheInfoScriptDoesNotReadTheEmptyCollection` — CN: trocar `queryMsgInfo`
  por leitura da `MsgInfoCollection`. Falha.
- `TestTheRemainingCountsComeFromThePage` — CN: derivar os restantes de
  `len(lista)`. Falha.
- `TestAnUnansweredInfoIsNotThreeEmptyLists` — CN: fixar `Answered: true`. Falha.
- `TestInfoAboutSomebodyElsesMessageIsItsOwnError` — CN: remover o ramo
  `NotMine`. Falha.

Prova em SPA real: `answered=true delivered=1 read=0 played=0 remaining=0/1/1`
numa mensagem própria de grupo, **e a recusa** com `ErrNotMine` sobre mensagem
alheia — porque uma capacidade só vista aceitando não está provada (H147, aplicada
no mesmo dia em que foi escrita).

**Lição**: *uma medição correta pode sustentar uma conclusão errada, e o jeito de
descobrir é perguntar como a REFERÊNCIA obtém o dado, não se ele está onde
procuramos.* A H71 procurou no lugar plausível e o encontrou vazio; dez linhas do
upstream diziam que o lugar plausível não é o lugar.

---

## H154 — `getReactions`: duas medições certas, duas conclusões erradas, e a causa era a CHAVE

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`. A pergunta que abriu esta entrada foi a
mesma que fechou a H153 — *como a REFERÊNCIA obtém o dado?* — aplicada às
reações porque três linhas dependiam da mesma resposta.

**Onde**: `internal/wa-headless/capabilities/message/message.go` (`ReactionsOf`,
`Reactions`, `Reaction`), `.../script.go` (`reactionsScript`),
`internal/wa-headless/probe_reactread_test.go` (novo). Linhas `getReactions`,
`sendReaction`, `react` e `MESSAGE_REACTION`.

**A história das duas medições anteriores**, e ambas foram honestas:

- **H83**: concluiu que o agregado *"não tem fonte neste build"*.
- **H124**: remediu a H83 com instrumento melhor — `WAWebCollections.Reactions`
  EXISTE, com `on` e `getModelsArray` —, mediu **0 mesmo depois de uma reação que
  a capacidade tinha verificado**, e concluiu: *"não é falta de coleção — a
  coleção não enche"*.

**A H124 estava certa sobre o fato e errada sobre o que ele significa.** A
coleção não enche mesmo — medida de novo hoje, `size: 0` depois de uma reação
verificada. Mas `Reactions.find` **não é busca nos modelos carregados**: é um
fetch assíncrono, que a referência aguarda (`wwebjs_message.js:848-853`), e cujo
registro nunca aterrissa no `getModelsArray`. Medir o array era medir o lugar
errado — exatamente a forma da H153, no mesmo dia.

**E havia uma segunda camada, que é por que isto levou três tentativas: a CHAVE
da referência não existe aqui.** Ela chama `find(msg.id._serialized)`, e
`_serialized` é **nulo** neste build LID-first — a família de enquetes já tinha
tido de pular a mesma conversão. Medidos lado a lado, sobre UMA mensagem que
carrega uma reação:

```
find(m.id._serialized)  →  threw "called find without an id"
find(m.id.id)           →  null
find(m.id)              →  o registro, reactions=1
```

**O objeto `id` é a chave.** Essa linha é a diferença inteira entre *"este build
não reporta reações"* e a função que agora existe.

**Forma medida** antes de projetar o tipo:

```
entrada:   {aggregateEmoji, hasReactionByMe, id, senders}
remetente: {ack, id, msgKey, orphan, parentMsgKey, reactionText,
            read, senderUserJid, timestamp}
```

**Correção aplicada**: `Reader.ReactionsOf`, agrupado por emoji porque é assim
que a página tem — achatar perderia o agrupamento sem ganhar nada, já que quem
quer lista plana a constrói e quem quer "quantos curtiram" não reconstrói os
grupos a partir dela. `ByMe` vem do `hasReactionByMe` da página e **não** de
comparar jids: essa comparação é precisamente a que já deu errado aqui (H136,
H148).

**Status**: corrigido, `getReactions` de `BLOCKED` para `PROVEN`. Travado por
quatro controles negativos, todos EXECUTADOS:
- `TestTheReactionsScriptKeysByTheIdObject` — CN: chavear por `_serialized`,
  a chave da referência. Falha.
- `TestTheReactionsScriptDoesNotReadTheModelsArray` — CN: ler
  `getModelsArray()[0]`. Falha.
- `TestReactionsComeBackGroupedByEmoji` — CN: achatar os grupos num só. Falha.
- `TestTheReactionsRenderingIsQuiet` — CN: imprimir `r.Groups`. Falha.

Prova em SPA real **por transição**, porque uma leitura não-vazia sozinha é
compatível com um leitor que devolve constante: `groups=1 senders=1` depois de
reagir, `groups=0 senders=0` depois de retirar.

**O que NÃO foi fechado por associação**: `sendReaction` e `react` continuam
`PARTIAL` porque `react.Remove` ainda devolve `Verified:false`. A CAUSA disso
caiu — havia como saber quais reações existem — mas usar a fonte para verificar o
`Remove` é trabalho em outro pacote, com seus próprios testes e prova. Ficou
registrado como **acionável**, que é a informação honesta, em vez de a linha ser
movida porque uma vizinha andou. `MESSAGE_REACTION` idem: a afirmação *"o
agregado não tem fonte neste build"* está refutada na nota, e enriquecer o evento
segue pendente.

**Lição**: *quando duas pessoas medem a mesma ausência e concluem coisas
diferentes, a pergunta certa não é "quem mediu melhor" — é "o que a referência
faz que nós não fazemos".* As duas mediram bem. Nenhuma perguntou pela CHAVE, e
era ali que estava. E a chave é justamente o tipo de detalhe que só aparece
comparando com a implementação que funciona — que é a razão de a regra deste
repositório mandar consultar as referências ANTES de projetar, e não depois de
concluir que algo é impossível.

---

## H155 — `react.Remove` verifica: a assimetria era falta de FONTE, não de rigor

**Data**: 2026-08-22
**Contexto**: completar o que a H154 destravou. Registrei ali que usar a fonte
para verificar o `Remove` era trabalho ACIONÁVEL e não impedimento; esta entrada
é esse trabalho.

**Onde**: `internal/wa-headless/spa/reactions.go` (novo,
`ReactionsForMessageExpr`), `capabilities/react/mine.go` (novo, `mineOn`),
`capabilities/react/react.go` (o ramo de remoção),
`capabilities/message/script.go` (passa a embutir a expressão compartilhada).
Linhas `sendReaction` e `react`.

**Problema**: `Remove` devolvia `Verified:false` **por medição honesta** — a H53
verificou em três sessões FRESCAS que a remoção funciona, e que na sessão que
removeu `hasReaction` fica pegajoso por pelo menos 30 s. Esperar por uma flag que
não vira produziria uma capacidade que sempre falha em algo que sempre funciona.
O raciocínio estava certo; faltava um sinal.

**Correção aplicada**: `mineOn`, que pergunta ao registro de reações se ESTA
conta ainda tem reação na mensagem, e que o `Remove` passa a esperar.

**A expressão foi para o `spa/` em vez de copiada**, seguindo o precedente do
`capabilities/lookup` com o `ResolveIdentityExpr`. O motivo é concreto: o
`capabilities/message` reporta o mesmo fato a quem chama, e duas cópias
divergiriam **na única forma de defeito que se esconde** — uma remoção que
verifica de um lado e relê como presente do outro, com as duas metades parecendo
corretas isoladamente.

**Os dois lados esperam por sinais DIFERENTES, e isso é desenho**: `Add` espera
pela flag pegajosa (que vira em menos de um segundo ao adicionar), `Remove`
espera pelo registro. O dublê do teste ganhou dois campos separados por isso — um
dublê que respondesse aos dois a partir de um campo só deixaria um `Remove` que
consultasse a flag parecer verificado.

**Status**: corrigido. `sendReaction` e `react` para `PROVEN`. Travado por:
- `TestBothAddingAndRemovingAreVerified` — CN1: voltar `Verified:false`. Falha.
  CN2: trocar `mineOn` por `waitFor` (a flag pegajosa). Falha. Ambos compilam.
- `TestARefusedVerificationLeavesRemoveUnverified` — CN: verificação que falha
  virar `Verified:true`. Falha.
- `TestTheMineScriptUsesThePagesOwnFlag`, `TestTheMineScriptEmbedsTheSharedExpression`.
- Do lado do `message`: `TestTheReactionsScriptEmbedsTheSharedExpression`.

Prova em SPA real: `react.Remove` devolveu `had=true has=false verified=true`, e o
leitor independente confirmou `groups=0 senders=0`.

**Dois controles negativos falharam como controles antes de morder, e os dois
são as armadilhas do catálogo, encontradas de novo no mesmo dia:**

1. **O controle não COMPILAVA.** Trocar `spa.ReactionsForMessageExpr` por uma
   cópia inline deixava o import sem uso, e `[build failed]` não é falha de
   teste — é armadilha nº 3 do `ARMADILHAS.md`. Corrigido acrescentando
   `var _ = spa.ReactionsForMessageExpr` para manter o import vivo, e aí o teste
   falhou com a mensagem certa. *Um controle que quebra o build passa por
   "mordeu" numa leitura apressada da saída.*
2. **A asserção media o parser.** O primeiro controle da regra do `byMe` mudou a
   lógica do script e o teste continuou verde, porque o dublê responde `mine`
   diretamente e o script nunca roda. A regra teve de ser afirmada sobre o
   SCRIPT — a mesma correção que este módulo já fez seis vezes.

**Lição**: *uma decisão registrada como "impossível" merece ser reexaminada quando
a razão dela muda, e não quando a paciência acaba.* A H53 não errou: ela mediu, e
recusou verificar o que não podia. O que mudou não foi o critério — foi o mundo
disponível. O ledger só permite distinguir os dois casos porque a H53 escreveu
POR QUE não verificava, em vez de apenas que não verificava.

---

## H156 — linhas de "delegação literal": provadas no PONTO DE CHAMADA, e duas deliberadamente NÃO movidas

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`. Dez linhas do ledger dizem *"delegação
literal"* — o upstream não tem lógica própria ali, apenas encaminha para um
método do `Client`.

**Onde**: `internal/wa-headless/probe_delegation_test.go` (novo). Linhas
`Chat.getContact`, `GroupNotification.getChat`, `GroupNotification.getContact`.

**A tentação, e por que ela é errada**: `getChatById` e `getContactById` estão
`PROVEN`. Como as linhas de delegação apenas encaminham para eles, movê-las
pareceria papelada — atualizar o estado porque o par mudou.

**Não é papelada, e a razão é a lição mais cara deste dia**: uma delegação está
provada quando a capacidade funciona **NA ENTRADA QUE AQUELE PONTO DE CHAMADA
PASSA**, e as entradas são diferentes. `GroupNotification.getContact` passa
`author` — um PARTICIPANTE de grupo, não o chat. Neste build LID-first, *"a
capacidade funciona"* e *"a capacidade funciona neste jid"* já se separaram três
vezes (H136, H148, H151). Fechar por herança é exatamente onde essa separação
passaria despercebida.

**Medição**, cada ponto de chamada com a entrada que ele realmente usa:

```
GroupNotification.getChat    jid do grupo          → resolve
GroupNotification.getContact author de gp2         → resolve
Chat.getContact              contraparte 1:1       → resolve
```

**O terceiro exigiu produzir o fato.** Nenhuma notificação `gp2` carregava autor
(a única no store era `membership_approval_mode`, sem autor), então uma foi
produzida renomeando o grupo de laboratório — e o nome restaurado em `defer`,
verificado pela pós-condição do próprio `SetSubject`. A primeira tentativa usou
`nome + " "` e a página recusou com *"Could not perform action"*: aparado, o
assunto era o mesmo, e um rename que não renomeia é recusado.

**Status**: corrigido — três linhas para `PROVEN`.

**Duas linhas foram deliberadamente NÃO movidas**, e isto é o conteúdo real da
entrada: `Broadcast.getChat` e `Broadcast.getContact` também delegam para os
mesmos dois métodos provados, e continuam `PARTIAL`. Elas passam um id de
**STATUS**, não de conversa, e as linhas de identidade de broadcast são `PARTIAL`
pela medição delas mesmas (H100). Movê-las seria fechar por associação — o mesmo
que recusei fazer com `sendReaction` na H154, no dia em que a associação teria
acertado por sorte.

**Lição**: *"delegação literal" descreve o upstream, não prova o nosso lado.* A
anotação é útil porque diz onde NÃO procurar lógica; ela não diz que a entrada
daquele ponto de chamada já foi exercitada. Toda linha de delegação restante
merece a mesma pergunta: **que jid, exatamente, esse ponto de chamada passa?**

---

## H157 — `getContactLidAndPhone`: o helper da referência não funciona aqui, e isso é a resposta

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`. Esta linha foi escolhida porque toca
diretamente a pergunta que a H151 deixou aberta — *quem resolve identidade neste
módulo* — e uma primitiva que devolva AS DUAS identidades é o que qualquer das
três respostas precisaria.

**Onde**: `internal/wa-headless/capabilities/lookup/pair.go` (novo, `LidAndPhone`,
`Pair`), `internal/wa-headless/probe_lidpn_test.go` (novo).

**O que a referência faz** (`wwebjs_util.js:1694-1716`): ramifica por `isLid`,
pega a metade que falta em `WAWebApiContact.getCurrentLid` ou `getPhoneNumber`, e
quando isso falha chama `queryWidExists` e pergunta **`getCurrentLid` de novo**.

**O que foi medido aqui**: a segunda chamada continua vazia.

```
fromPhone: {ok:true, hasLid:false, hasPn:true, pnServer:"c.us",
            queried:true, askedServer:"c.us"}
```

A consulta rodou — `queried: true` — e `getCurrentLid` não produziu nada, sobre o
par de laboratório que este módulo resolve com sucesso todos os dias. **O helper
da referência devolveria `{}` para alguém perfeitamente alcançável.**

O LID está no RESULTADO da consulta, que é exatamente de onde o
`spa.ResolveIdentityExpr` já o tira. É o mesmo padrão do `sendText` registrado no
`CLAUDE.md`: copiar o caminho da referência teria falhado, e o que serve é copiar
o ENTENDIMENTO — aqui, "resolver identidade é perguntar ao servidor", não "chamar
`getCurrentLid`".

**A guarda `isLid` foi mantida, e não é estilo.** Chamar `getPhoneNumber` com um
jid de telefone lança `WaWebLidPnCache - Invalid get call (not lid)` — medido
porque a primeira sonda cometeu exatamente esse erro, e a mensagem de erro é que
explicou por que a referência ramifica.

**Correção aplicada**: `Resolver.LidAndPhone`, devolvendo `Pair{LID, PN, Queried}`.
Três decisões:
1. **Metade ausente fica ausente.** Vazio significa "a página não produziu",
   nunca "não existe" — preencher com a entrada reportaria um número inventado.
2. **`Queried` é reportado** porque quem varre um roster quer saber que acabou de
   fazer N idas à rede.
3. **Entrada LID não consulta.** A identidade entregue já é metade da resposta.

**Status**: corrigido, linha para `PROVEN`. Travado por quatro controles
negativos, todos compilando e falhando:
- `TestAnAbsentHalfIsNotFilledIn` — CN: preencher `PN` vazio com a entrada.
- `TestThePairScriptGuardsGetPhoneNumber` — CN: chamar `getPhoneNumber` antes da
  guarda (teste de ORDEM).
- `TestThePairScriptTakesTheLidFromTheResolution` — CN: tirar o LID de
  `getCurrentLid`, como a referência faz.
- `TestALidInputIsNotResolvedAgain` — CN: resolver antes do ramo `isLid`.

Prova em SPA real nas duas direções: do telefone, `lid=true pn=true queried=true`;
do LID devolvido, `lid=true pn=true queried=false` — o telefone vem do cache de
mapeamento sem custar consulta.

**Duas armadilhas do catálogo apareceram de novo nesta entrada, e as duas já
tinham aparecido hoje:**

1. **A guarda casou com o próprio comentário** — décima ocorrência. A asserção
   *"o script não chama `getCurrentLid`"* falhou contra um comentário
   EXPLICANDO por que ele não chama. Corrigido com `withoutComments`, que este
   pacote não tinha e agora tem.
2. **A asserção media o parser.** O CN de "entrada LID não consulta" não mordeu,
   porque o dublê devolve `queried` do próprio campo. Refeito como asserção
   ESTRUTURAL sobre o script — a posição do `await resolve(` contra o retorno do
   ramo `isLid` —, e aí mordeu.

**Lição**: *quando a referência tem um helper para exatamente o seu problema, a
primeira coisa a medir é se ele funciona aqui.* Ele não funcionava, e descobrir
isso levou uma sonda; assumir que funcionava teria produzido um leitor que
devolve vazio para todo mundo e parece correto, porque `{}` é uma resposta
plausível para "não achei".

---

## H158 — `getBroadcasts`: o zero foi interrogado e desta vez é o mundo

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`. Três linhas caíram hoje porque um zero
significava *"ninguém pediu"* e não *"não existe"* — mensões (H142), informação de
mensagem (H153) e reações (H154). Interrogar este zero era obrigatório.

**Onde**: `internal/wa-headless/probe_status2_test.go` (novo), linhas
`getBroadcasts` e `getBroadcastById`.

**Por que a suspeita era razoável**: os feeds de status são de OUTRAS pessoas.
Se qualquer um dos 947 contatos tivesse status ativo, uma coleção hidratada
mostraria — e um zero passaria a significar que falta buscar, não que o mundo
está vazio. A superfície reforçava a suspeita: a `Status` tem `sync`, `hasSynced`,
`find`, `_serverQuery` e até um `findImpl` (que o `Chat` deste build **não** tem),
mais um `WAWebApiStatus.getAllStatuses` independente.

**Medição**:

```
sizeBefore: 0        hasSyncedBefore: true
syncOk: true         hasSyncedAfter:  true
sizeAfterSync: 0     getAllStatuses:  array[0]
sizeFinal: 0         contatos:        947
```

**`hasSynced` já era `true` ANTES**, o `sync()` rodou sem erro e nada mudou, e o
caminho **independente** da coleção devolveu lista vazia. Dois instrumentos que
não compartilham cache concordando é o que transforma este zero em fato.

**Status**: não corrigido, e não há o que corrigir. A linha continua `PARTIAL`
com a causa agora MEDIDA: a coleção está hidratada e o mundo está vazio. Provar
um feed não-vazio exige postar um status desta conta — visível a 944 contatos, já
escalado ao humano e confirmado legítimo pela decisão 61.

`getBroadcastById` herda a mesma medição por um motivo que não é herança: sem
feed de NINGUÉM, não há entrada para buscar por contato.

**Lição, e é o contrapeso da H142**: *interrogar um zero é obrigatório; concluir
que todo zero é do instrumento é o excesso.* A H149 já tinha registrado esse
excesso uma vez hoje, com a identidade. Uma generalização recém-aprendida vale
pelo teste que ela sobrevive, e esta sobreviveu ao contrário: a suspeita era boa,
a evidência disse não, e o custo de descobrir foi uma sonda. **O que torna o
resultado utilizável é ter medido com DOIS caminhos independentes** — se só a
coleção tivesse respondido, o zero continuaria ambíguo.

---

## H159 — `sendSeen` medido pelo lado certo: o recibo não chega ao remetente

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`. A H82 rebaixou esta linha porque a
pós-condição do `MarkRead` afirma que `unreadCount` mudou NA MESMA SESSÃO, e a
H78 mediu esse contador como cross-session — *"se generaliza é pergunta em
aberto"*.

**Onde**: `internal/wa-headless/probe_seen_test.go` (novo).

**O contador é a testemunha errada, de qualquer forma.** O que `sendSeen`
produz que alguém pode observar é dizer ao REMETENTE que sua mensagem foi lida:
ack 3 na cópia dele. Isso é fato sobre a outra sessão — exatamente o que nenhuma
sessão sozinha checa, e para o que a sessão dupla (H135) existe.

**Medição**: conta-B manda, conta-A marca lida, o ack em conta-B fica em **2** por
60 s.

**O confundidor foi atacado, não ignorado.** *"O recibo não saiu"* e *"esta conta
tem recibo de leitura desligado"* produzem o mesmo ack. Tentei ler a
configuração e os quatro nomes de módulo que chutei não existem neste build —
armadilha da H137 de novo. Em vez de concluir com o confundidor de pé, inverti o
experimento: **conta-A manda e conta-B marca lida**. Falha igual, ack em 2.

Falhar simetricamente com DUAS contas torna a explicação de privacidade bem menos
provável, mas — e isto fica dito — não a exclui: as duas são contas de laboratório
configuradas do mesmo jeito.

**Status**: não corrigido nesta entrada (ver H160 para o que FOI corrigido).

**Lição**: *quando duas causas produzem a mesma observação e você não consegue
medir uma delas, inverta o experimento.* Procurar a configuração custou uma sonda
e quatro nomes errados; trocar os papéis das contas custou uma perna a mais no
mesmo teste e respondeu.

---

## H160 — `MarkRead` usava a primitiva errada, e a pós-condição da H82 agora vale

**Data**: 2026-08-22
**Contexto**: continuação da H159. Depois de medir que o recibo não sai, li a
referência para ver o que ela faz de diferente — que é a ordem correta: medir,
depois comparar.

**Onde**: `internal/wa-headless/capabilities/chats/markread.go`,
`internal/wa-headless/spa/modules.go` (`ModuleUpdateUnreadChatAction`,
`ModuleStreamModel`), `internal/wa-headless/probe_seenstream_test.go` (novo).

**O que a referência faz** (`wwebjs_util.js:131-143`):

```js
Stream.markAvailable();
await UpdateUnreadChatAction.sendSeen({chat, threadId: undefined});
Stream.markUnavailable();
```

Duas diferenças: um MÓDULO diferente do nosso (`WAWebChatSendConversationSeen`) e
o anúncio de presença em volta.

**A hipótese testada primeiro foi a errada, e testá-la crua foi o que salvou.**
Achei que o `markAvailable` fosse a chave — recibo de cliente offline não sai.
Testei a sequência **crua na página**, sem tocar na capacidade, porque mudar a
capacidade e remedir confundiria *"o bracketing ajudou"* com *"a repetição
ajudou"*. Resultado:

```
unreadBefore: 1   →   unreadAfter: 0
ack em conta-B:   2 (continuou 2)
```

**Hipótese do recibo REFUTADA. E, no mesmo experimento, um achado que eu não
procurava**: o `unreadCount` foi de 1 a 0 — coisa que a nossa primitiva nunca
fez. O módulo é que estava errado, não o bracketing.

**Enumeração incidental**, do mesmo módulo: `markUnread`, `sendSeenDebounced`,
`sendSeen`, `markSeen`, `markUnseen`, `updateUnreadCountMD`, `clearUnreadMentions`.
O `markUnread` aí dentro é candidato direto para a linha `markChatUnread`, que
está `BLOCKED` desde a H78 por *"duas primitivas medidas, nenhuma marca"* —
**registrado como pista, não perseguido nesta sessão**.

**Correção aplicada**: `MarkRead` passa a chamar
`WAWebUpdateUnreadChatAction.sendSeen({chat, threadId})`, com o
`markAvailable`/`markUnavailable` em volta. O bracketing ficou **mesmo tendo sido
refutado** para o recibo: é o que a referência faz, não custa nada, e divergir
sem motivo é o que o `CLAUDE.md` proíbe. O desfazer da presença está num
`finally` — sair deste caminho anunciado como online é efeito colateral que nada
pediu, e sobreviveria ao erro.

**Status**: corrigido. `before=1 after=0 changed=true`, sem erro, **nas duas
contas**. Travado por:
- `TestTheAcknowledgementUsesThePrimitiveThatMoves` — CN: voltar a
  `sendConversationSeen`. Falha. Substituiu
  `TestTheAcknowledgementNamesWhatWasRead`, que era asserção correta sobre a
  primitiva ERRADA: a nova chamada não recebe `lastReceivedKey`, e exigir uma
  chave que ela não tem seria pedir o que não existe.
- `TestThePresenceAnnouncementIsUndoneOnFailure` — CN: tirar o desfazer do
  `finally`. Falha. É teste de ORDEM.

**A linha continua `PARTIAL`, e agora por um motivo medido em vez de duvidoso**:
o reconhecimento local está provado; o recibo ao remetente não foi observado.

**Duas armadilhas minhas, registradas:**
1. **Crases dentro de raw string.** Escrevi `` `finally` `` num comentário JS
   dentro de uma raw string Go, e as crases a terminaram. O build acusou
   `expected ';', found finally`.
2. **A guarda casou com o próprio comentário** — décima primeira vez, segunda
   hoje. A asserção de ORDEM sobre `markUnavailable` falhou contra o comentário
   que EXPLICA a ordem. `withoutComments` acrescentado a este pacote também.

**Lição**: *teste a hipótese crua antes de embuti-la.* A hipótese estava errada e
o experimento estava certo — e foi justamente por rodá-lo cru, medindo tudo que
mudou em vez de só o que eu esperava, que o achado verdadeiro apareceu. Se eu
tivesse mudado a capacidade e olhado só o ack, teria concluído "não adiantou" e
descartado a correção junto com a hipótese.

---

## H161 — `markChatUnread`: a melhor pista já disponível foi gasta, e o `BLOCKED` sai reforçado

**Data**: 2026-08-22
**Contexto**: seguir a pista que a H160 registrou e deliberadamente não perseguiu.

**Onde**: `internal/wa-headless/probe_markunread2_test.go` (novo), linha
`markChatUnread`.

**Por que a pista era boa**: a H160 consertou o `MarkRead` trocando de MÓDULO —
chamávamos `sendConversationSeen`, a referência chama
`WAWebUpdateUnreadChatAction.sendSeen`, e o contador passou a se mexer. O mesmo
módulo exporta `markUnread`. Uma linha `BLOCKED` desde a H78 por *"duas
primitivas medidas, nenhuma marca"* ganhou, pela primeira vez, um candidato
específico com precedente de sucesso no mesmo dia.

**Medição**, três primitivas no mesmo chat, cada uma em separado:

```
inicial                                        markedUnread=false unreadCount=0
Cmd.markChatUnread(chat, true)          ok     markedUnread=false unreadCount=undef
UpdateUnreadChatAction.markUnread(...)  ok     markedUnread=false unreadCount=undef
markUnread com presença anunciada       ok     markedUnread=false unreadCount=undef
restauro (sendSeen)                            markedUnread=false unreadCount=0
```

**As três retornam sem erro e nenhuma vira `markedUnread`.** O `BLOCKED` continua,
com evidência três vezes melhor do que tinha.

**A forma da terceira NÃO foi chutada.** A primeira tentativa passou `{chat}` e
levou `Cannot read properties of undefined (reading 'markUnread')` — de dentro da
função, não do require. Em vez de tentar variações, li a assinatura da própria
função:

```js
function h(e, t, n) {
  return n === void 0 && (n = !0),
    E({allowAction: n, chat: o("WAWebStateUtils").unproxy(e), unread: t})
}
```

`markUnread(chat, unread, allowAction = true)` — posicional, três argumentos.
Isso é mais barato e mais confiável que enumerar nomes, e é a evolução natural da
regra da H143: **quando o nome existe e a chamada falha, leia a assinatura.**

**Um defeito do meu instrumento, e ele já estava catalogado por mim mesmo.** A
primeira versão guardou o modelo do chat numa variável e leu dela; depois da
primeira chamada, `unreadCount` virou não-numérico. Não era a página quebrando —
`markUnread` passa o chat por `WAWebStateUtils.unproxy`, e o objeto em mãos deixa
de ser o vivo. É exatamente o *"A COLEÇÃO É RELIDA, não se confia no modelo em
mãos"* que escrevi na H143, aplicado a um caso novo. Custou uma rodada.

**Status**: não corrigido, e agora com a pista específica gasta. Fica registrado
que `unreadCount` vira **indefinido** (não zero) depois de qualquer das três — o
que é uma mudança de estado real, só não a que o verbo pede.

**Lição**: *uma pista boa merece ser gasta, e gastá-la é resultado.* A linha
estava `BLOCKED` com duas primitivas; agora está `BLOCKED` com três, incluindo a
da referência e a da família que consertou a vizinha no mesmo dia. Isso não muda
o estado e muda o que a próxima pessoa precisa tentar — que é o único jeito de um
`BLOCKED` não virar dívida permanente.

---

## H162 — `pin` não estava bloqueado: a H81 mediu do único lado que não podia ver

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`, começando por `getPinnedMessages`, que
parecia ser mais um caso de "sombra de escrita `BLOCKED`" como o `description` da
H145.

**Onde**: `internal/wa-headless/probe_pinned_test.go` e
`probe_pindual_test.go` (novos). Linhas `pin` (era `BLOCKED`) e
`getPinnedMessages` (duas).

**O que eu ia registrar, e por que teria sido errado.** A primeira sonda mandou
uma mensagem, tentou fixá-la e leu a lista: 0 antes, 0 depois. A conclusão pronta
— "a escrita continua bloqueada, o leitor não tem produtor, linha não acionável"
— estava a uma frase de ser escrita.

**O que impediu foi ler o doc do tipo que eu mesmo estava usando:**

> `Verified` is false for a real change: this build does not show the session its
> own pin.

Ou seja: dentro da sessão que age, *"funcionou e eu não vejo"* e *"não fez nada"*
produzem **a mesma leitura**. A H81 mediu do lado que não podia responder, e
qualquer remedição do mesmo lado reproduziria a ambiguidade em vez de resolvê-la.

**É a forma da H86/H135**, com outro verbo: um zero que era sobre a sessão ATORA,
não sobre o mundo. Lá foram eventos de participante; aqui é o pin.

**Medição, com o observador decidindo:**

```
BEFORE (visto por conta-B): 0 fixados
conta-A pin.Message:        verified=false   ← honesto, não falho
AFTER  (visto por conta-B): 1 fixado
```

**conta-B VÊ o pin.** A escrita funciona. A linha `BLOCKED` estava errada não por
falta de rigor, mas por um ponto cego que só a sessão dupla abre.

**Correção aplicada** (só no ledger; o código estava certo o tempo todo):
- `pin` sai de `BLOCKED` para `PARTIAL`, **não** para `PROVEN` — pela convenção
  que o `addParticipants` (H58) já estabeleceu para exatamente esta situação:
  confirmação só entre sessões. Promover além disso seria inventar um critério
  novo para uma linha só.
- `getPinnedMessages` (duas) vão para `PROVEN`: o leitor foi exercitado
  **não-vazio**, rodando em conta-B, e ele não depende de sessão dupla — a sessão
  dupla foi o que produziu o DADO, como nas menções (H142).

**Status**: corrigido. Placar: `PROVEN 123`, `PARTIAL 44`, `BLOCKED 47`.

**A linha de base veio do OBSERVADOR, não do ator**, e isso foi deliberado: é a
leitura de conta-B que decide, então é dela que o "antes" tem de vir. Um "antes"
lido em conta-A compararia coisas diferentes.

**Lição, e é a mais cara desta varredura**: *quando uma capacidade documenta que
não consegue se verificar, toda medição feita por ela é inconclusiva — inclusive
a que a declarou impossível.* O aviso estava escrito no campo `Verified`, no
mesmo arquivo, e sobreviveu a uma reclassificação para `BLOCKED` (H140, decisão
60) sem ninguém cruzar as duas coisas. **Vale reler todo `BLOCKED` cujo veredito
venha de uma pós-condição que o próprio tipo diz não conseguir observar.**

---

## H163 — a auditoria que a H162 gerou: um ponto cego real, e o resto confirmado

**Data**: 2026-08-22
**Contexto**: a H162 terminou com uma ação sistemática, não com uma linha
fechada: *"vale reler todo `BLOCKED` cujo veredito venha de uma pós-condição que
o próprio tipo diz não conseguir observar"*. Esta entrada é essa auditoria.

**Onde**: `internal/wa-headless/probe_descdual_test.go` (novo), linhas
`setDescription` (duas) e `description`.

**A varredura dos 47 `BLOCKED`** procurou vereditos que dependem de não-observação
na mesma sessão. Sobraram três candidatos, e só um era de verdade:

| linha | veredito | resultado |
|---|---|---|
| `pin` | "chamada aceita e nada fixado" | **era ponto cego** — H162, agora `PARTIAL` |
| `setDescription` (×2) | "a página aceita e o servidor nunca armazena" | testado aqui |
| `mute` | já corrigido na H134 por medição própria | nada a fazer |

**`setDescription` era o candidato mais forte**: as TRÊS medições que o
bloquearam — canal (H113), grupo (H126) e a minha reprodução (H145) — foram
todas da sessão que agiu, exatamente o padrão que acabara de enganar o `pin`.

**Medição**, com o observador decidindo e a linha de base lida DELE:

```
BEFORE (conta-B):  descLen=0  source="none"
conta-A SetDescription: err="asked for 43 bytes and the server reports 0"
AFTER  (conta-B, 60s):  descLen=0  source="none"
```

**conta-B não vê.** O `BLOCKED` sai reforçado, agora do lado que nunca tinha sido
perguntado — e a não-acionabilidade que a H145 registrou para o leitor
`description` foi reconfirmada por um caminho independente.

**O erro não encerrou a medição.** A pós-condição do `SetDescription` lê do lado
que a H162 mostrou ser cego, então parar no erro dela teria repetido o mesmo
engano com outro nome. O experimento continuou depois da falha, de propósito.

**Um efeito colateral ficou dito em vez de escondido**: o grupo não tinha
descrição antes, e limpar exigiria a mesma escrita que está sob teste. Como ela
não armazena nada, nada ficou para trás — mas o `defer` reporta isso em vez de
supor.

**Status**: não corrigido; as linhas continuam `BLOCKED`, com evidência dos dois
lados.

**Lição, e é o contrapeso necessário da H162**: *nem todo "o ator não enxerga" é
ponto cego.* A H162 encontrou um caso real e produziu uma generalização
poderosa; aplicá-la sem testar teria convertido três linhas `BLOCKED` bem medidas
em falsos positivos. O mesmo par apareceu hoje com as identidades — H148 achou o
caso, H149 testou a generalização e a descartou. **A regra é: gere a hipótese
pela analogia, decida pela medição.**

---

## H164 — `acceptInvite`: o fixture foi construído em vez de emprestado

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`. A linha tinha uma metade provada ao vivo
(o caminho de APROVAÇÃO, H89) e a outra intocada.

**Onde**: `internal/wa-headless/probe_directjoin_test.go` (novo), linha
`acceptInvite`.

**Por que a metade faltante nunca tinha sido feita**: o grupo de laboratório
exige aprovação de entrada. Desligar isso num fixture de que todo outro teste
depende não é mudança para fazer por causa de uma linha. Então o fixture foi
**construído**: conta-A cria um grupo descartável, conta-B sai dele, e volta a
entrar pelo CÓDIGO de convite. As duas saem no fim, em `defer`.

**Medição**:

```
created: participants=2 created=true   approval_mode=false
invite:  code=true len=22 revoked=false
conta-B JoinByInvite: pending=false
conta-B conta 2 participantes
```

`pending=false` é o que distingue este caminho do da H89, em que a página recusa
com `UnexpectedJoinGroupViaInviteResponse` e a recusa É o pedido sendo criado.

**Três tropeços, e cada um ensinou algo utilizável:**

1. **`Ensure` recusa criar um grupo vazio** — *"a group needs at least one
   participant"*. É a página falando, não escolha nossa, então o grupo nasce com
   conta-B e ela SAI antes de entrar por convite. Assim o que fica sob teste
   continua sendo a entrada por código, não a adição por participante.

2. **A propagação tem de ser esperada, não presumida.** A criação volta com 2
   participantes do lado de quem criou, e conta-B ainda não tem o grupo: pedir a
   saída nesse instante responde `NO_CHAT` — verdade sobre a SESSÃO, não sobre o
   grupo. É a mesma família de erro da H162, em miniatura.

3. **`Ensure` é idempotente POR ASSUNTO**, e isso envenenou uma rodada inteira.
   Com nome fixo, a segunda execução reusou o grupo abandonado pela primeira — do
   qual conta-A já tinha saído —, e o sintoma foi `this account is not an admin
   of that group` sobre um grupo recém-"criado". **A resposta já dizia**:
   `created=false`. Corrigido com assunto único por execução E com uma asserção
   sobre `Created`, para que a próxima vez falhe alto em vez de confundir.

**Status**: corrigido, linha para `PROVEN`.

**Achado incidental, NÃO corrigido**: as duas tentativas frustradas deixaram
grupos descartáveis abandonados no servidor — ninguém é membro deles, então este
módulo não tem como apagá-los. Não afetam nenhum teste (o assunto agora é único),
mas ficam registrados em vez de silenciados. **Correção sugerida**: quando um
probe criar fixture no servidor, o nome deve ser único E o `defer` deve rodar
antes de qualquer `Fatal` que o preceda — foi por um `Fatal` antes do `defer` que
o primeiro grupo ficou órfão.

**Lição**: *um helper idempotente é uma armadilha em teste de fixture.* `Ensure`
faz exatamente o que promete, e é o certo em produção; num probe que precisa de
um grupo NOVO, "achei um igual" e "criei" são fatos diferentes e a diferença
estava no valor de retorno o tempo todo. Ler o que a chamada devolve custa menos
que depurar o efeito dela.

---

## H165 — `rejectGroupMembershipRequests`: o motivo de nunca ter sido testado deixou de valer

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`, aplicando a técnica que a H164 produziu.

**Onde**: `internal/wa-headless/probe_reject_test.go` (novo), linhas
`rejectGroupMembershipRequests` (duas).

**Por que estava `PARTIAL`**: implementado e travado por teste unitário, mas
**nunca exercitado ao vivo**, por uma razão concreta e boa — rejeitar conta-B a
expulsaria do grupo de laboratório, que é o fixture de que todo o resto depende.
`Approve` foi exercitado; `Reject` não, e *"mesma RPC, chave diferente"* é
argumento, não medição.

**O que mudou não foi o risco, foi a alternativa.** A H164 estabeleceu:
**não empreste o fixture, construa um**. Um grupo descartável com aprovação
ligada dá a conta-B o que pedir e a conta-A o que rejeitar, sem tocar em nada.

**Medição**:

```
conta-B JoinByInvite: pending=true kind=UnexpectedJoinGroupViaInviteResponse
conta-A vê:           1 pedido pendente
Reject:               ok=true code=0
depois:               0 pendentes, grupo ainda com 1 participante
```

**A segunda metade da última linha é o que faz disto uma prova.** "0 pendentes"
sozinho é igual para rejeição e para aprovação — o pedido some nos dois casos. É
o grupo **não crescer** que distingue os dois, e o teste falha explicitamente se
crescer: *"that is an APPROVAL, not a rejection"*.

O achado da H89 — a recusa da página com `UnexpectedJoinGroupViaInviteResponse`
**é** o pedido sendo criado — foi reusado aqui como PRÉ-CONDIÇÃO em vez de
conclusão, que é o melhor destino de um achado antigo.

**Um erro meu, e é o mesmo da H162 num disfarce menor.** A primeira versão
esperava `Count` responder do lado de conta-B e seguia; `Count` respondeu e o
`Leave` seguinte disse *"this account is not a member of that group"*. **Ler o
grupo e pertencer a ele são fatos diferentes.** A espera passou a ser pela
PRÓPRIA operação — tenta sair até conseguir —, que é a única condição que
significa o que o teste precisa.

**E a correção sugerida na H164 foi aplicada aqui**: o `defer` de limpeza é
registrado ANTES de qualquer `Fatal` que o siga. Foi por registrá-lo tarde que a
H164 deixou dois grupos órfãos; este não deixou nenhum.

**Status**: corrigido, duas linhas para `PROVEN`.

**Lição**: *"não dá para testar sem estragar o fixture" é uma afirmação sobre o
fixture, não sobre a capacidade.* Ela era verdadeira e passou meses parecendo
definitiva. O que a derrubou não foi coragem nem um risco aceito — foi perceber
que o fixture podia ser construído. Vale reler toda linha cujo impedimento seja
**custo colateral** e não impossibilidade.

---

## H166 — a auditoria do "custo colateral": três linhas caem, e uma medição salva um falso defeito

**Data**: 2026-08-22
**Contexto**: a H165 terminou pedindo uma auditoria — *"vale reler toda linha
cujo impedimento seja CUSTO COLATERAL e não impossibilidade"*. Esta é ela.

**Onde**: `internal/wa-headless/probe_lifecycle_test.go` (novo), linhas
`clearMessages`, `delete` e `leave`.

**A varredura** achou cinco linhas cujo impedimento é custo, não impossibilidade:

| linha | impedimento | destino |
|---|---|---|
| `clearMessages` | "destruiria o fixture de todos os outros testes" (H66) | **fechada** |
| `delete` | idem, "recusa DELIBERADA" (H66) | **fechada** |
| `leave` | "quem sai de grupo que criou não volta sem convite" (H65) | **fechada** |
| `revokeStatusMessage` | postar status é visível a 944 contatos | continua — decisão humana |
| `getData` (catálogo) | exigiria acrescentar produto real ao perfil comercial | continua |

As três primeiras caem com a mesma resposta da H164/H165: **um grupo descartável**,
criado, enchido, esvaziado, abandonado e apagado. As duas últimas **não** caem, e
a diferença é real: um status e um produto são artefatos que saem para o mundo
além do par de laboratório.

**A medição que importa é a do `clearMessages`, porque ela quase virou defeito.**
A primeira asserção exigiu zero mensagens depois do `Clear` e o chat parou em
**1**. "Clear deixou uma" é compatível com falha da capacidade E com comportamento
correto do app — diagnósticos opostos, e a diferença está no TIPO do que sobrou.
Medido:

```
antes: 4    depois: 1
sobrevivente: {"type":"e2e_notification","subtype":"encrypt","fromMe":false}
```

Notificação de sistema, não mensagem de conversa. O `Clear` fez o que devia.
Exigir zero teria registrado um defeito que não existe — e é exatamente o erro
que a H150 evitou de outra forma, separando os atos antes de acusar.

**Achado incidental, NÃO corrigido**: `chats.Clear` **não verifica a própria
pós-condição**. O `Emptied` carrega `MessagesBefore`, `KeptStarred` e `Waited` —
nenhum "depois". Um `Clear` que não apagasse nada devolveria exatamente o mesmo
valor de sucesso. Foi por ler de volta na sonda que os números apareceram.
**Correção sugerida**: acrescentar `MessagesAfter` e recusar quando não diminuir
— com o cuidado de tratar a notificação de sistema como sobrevivente legítima,
senão a nova pós-condição falharia sempre. Não aplicado: mudar uma capacidade de
"nunca falha" para "pode falhar" é mudança de contrato, e a regra do projeto manda
perguntar.

**Status**: corrigido — três linhas para `PROVEN`. `PROVEN 129`, `PARTIAL 38`.

**Lição**: *uma recusa deliberada envelhece.* As três eram decisões corretas
quando foram tomadas, e continuaram escritas como se fossem propriedades da
capacidade. O que mudou não foi o julgamento sobre o risco — foi a existência de
uma alternativa que ninguém tinha procurado. **Toda linha cujo motivo comece com
"não dá para" merece a pergunta: não dá para QUEM, e sob quais condições?**

---

## H167 — `syncHistory` fecha, e o `require` deste build mente sobre módulos ausentes

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`. Duas linhas foram escolhidas juntas porque
pediam a mesma técnica — enumerar e ler a assinatura, que já pagou quatro vezes
hoje.

**Onde**: `internal/wa-headless/capabilities/fetchmessages/synchistory.go` (novo),
`internal/wa-headless/probe_synchist_test.go` (novo). Linhas `syncHistory` (duas)
e `reject` (chamada).

### `syncHistory`: a nota estava certa sobre a diferença e calada sobre o módulo

*"Buscamos histórico de uma conversa; sincronizar não"* descreve corretamente
dois atos diferentes — ler o que esta sessão já tem (`Fetch`) e PEDIR ao telefone
pareado que mande mais. O que a nota não dizia é se o segundo era possível aqui.

A referência faz algo pequeno e específico (`wwebjs_client.js:3173-3189`):
guarda em `chat.endOfHistoryTransferType === 0` e chama
`WAWebSendNonMessageDataRequest.sendPeerDataOperationRequest(3, {chatId})`.

**Medido**: o módulo existe, a função tem aridade 3, e o campo da guarda está em
383 dos 389 chats — **378 em 0** (há o que pedir), 5 em outro valor, 6 sem o
campo.

**Correção aplicada**: `Fetcher.SyncHistory`, devolvendo `SyncRequest{Requested,
TransferType, HasTransferType}`. Três decisões:

1. **O tipo se chama `SyncRequest`, não `SyncResult`.** O histórico chega depois,
   pelo socket; não há nada para reler que prove que funcionou. Um nome que
   prometesse resultado convidaria o sucesso silencioso que a invariante 14
   proíbe, então o tipo reporta o que FOI FEITO.
2. **A guarda vem antes do pedido**, e é teste de ORDEM: invertida, passa em todos
   os outros. Pedir a uma conversa que já entregou tudo devolveria "sucesso"
   indistinguível de um pedido útil.
3. **Ausente não é zero** — 6 de 389 chats não têm o campo, e zero é justamente o
   valor que significa "peça". Fundi-los reportaria pedido possível sobre
   conversa que a página nunca descreveu.

**Provado com AS DUAS respostas** (regra da H147), e as duas existem no mesmo
roster: `requested=true type=0` num elegível, `requested=false type=1` num já
transferido. Quatro controles negativos, todos compilando e falhando.

### `reject` (chamada): a H130 está certa, e a minha lista estava errada

A H130 mediu quatro módulos de ação de chamada como ausentes. Quatro nomes é
amostra, não censo, então reenumerei nove. O primeiro relatório disse **todos
presentes** — e estava errado, por defeito do meu próprio predicado.

**O `require` deste build NÃO LANÇA para um módulo inexistente: devolve objeto
VAZIO.** Separando `require lançou` de `require devolveu {}`:

```
WAWebCallCollection    → 8 exports reais
WAWebCallModel         → 2 exports
WAWebApiCall, WAWebCallActions, WAWebRejectCallAction,
WAWebEndCallAction, WAWebOfferCallAction, WAWebCallSignaling,
WAWebCallState         → require OK, ZERO exports
```

A H130 está confirmada. **E isto é um fato sobre o INSTRUMENTO que vale para toda
enumeração deste repositório**: `try { require(m) } catch` classifica módulos
inexistentes como presentes. Só listar as CHAVES distingue. As enumerações
anteriores (H143, H146, H160) escaparam por listarem chaves; a regra passa a ser
explícita.

**Status**: `syncHistory` (duas linhas) para `PROVEN`; `reject` continua
`PARTIAL` com a medição confirmada por um censo maior.

**Lição**: *um predicado de existência precisa ser testado contra algo que
sabidamente não existe.* Eu enumerei nove módulos e não incluí nenhum nome
inventado como controle — se tivesse, `WAWebNaoExisteMesmo` teria aparecido
"presente" na primeira leitura e o defeito do instrumento apareceria antes da
conclusão errada.

---

## H168 — `AUTHENTICATION_FAILURE`: a classe da página passou a viajar, e o controle negativo achou o elo sem teste

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`.

**Onde**: `internal/wa-headless/core/session.go` (`BootFailure.PageClass`,
`failClass`), `core/lifecycle.go` (`LifecycleFact.PageClass`),
`runtime/lifecycle.go`, `events/events.go` (`Event.PageClass`,
`PublishSessionStateWithClass`), `probe_authfail_test.go` (novo).

**A objeção da linha era exata**: o evento carregava o ESTÁGIO do boot, e o
estágio sozinho não distingue um boot que morre em `not_ready` contra uma tela de
pareamento de um que morre contra página quebrada — os dois chegam ao mesmo
lugar por motivos opostos, e o primeiro é o que o upstream chama de falha de
autenticação. A classe existia, mas **dentro da mensagem de erro**, e mensagem de
erro não entra neste barramento (H88) — regra certa, que deixava a informação
inacessível.

**Correção aplicada**: a classe viaja como campo próprio, de vocabulário FECHADO
(`spa.PageClass`), por todo o caminho. `Event.PageClass` é campo SEPARADO do
`Reason`: um diz até onde o boot foi, o outro diz o que o parou.

`PublishSessionState` continua existindo e não inventa classe — quem não tem o
que dizer sobre a página continua não dizendo, em vez de mandar um `"UNKNOWN"`
que se leria como medição.

**Duas correções minhas, e a segunda é a que ensina:**

1. **`failClass` é variável, não parâmetro.** `fail()` tem uma dúzia de
   chamadores e onze não têm classe alguma para passar; enfiar `""` em todos
   poria ruído onde não há informação.

2. **A asserção da prova estava errada, não o código.** Exigi `LOGIN_REQUIRED` de
   um perfil vazio e recebi `PAIRING_LOADING`, em 2,9 s de um orçamento de 90 s —
   o classificador reconhece a tela de pareamento e **não espera o QR renderizar**,
   que é o comportamento certo. Os dois valores dizem *"esta página quer
   autenticação"*. Exigir um deles seria medir o tempo de renderização do QR e
   chamar isso de significado. A asserção passou a ser sobre a FAMÍLIA.

**A prova é a DISTINÇÃO, não um valor**: perfil não pareado dá pareamento; página
em branco dá `REDIRECT`. Se as duas dessem o mesmo, o campo não separaria nada, e
o teste falha explicitamente nesse caso.

**Status**: corrigido, linha para `PROVEN`. Quatro controles negativos, todos
compilando e falhando.

**O achado sobre método**: um dos controles — apagar o repasse no `runtime` —
**compilou e não foi pego por nenhum teste unitário**. O elo `core → runtime →
barramento` não tinha guarda. A asserção foi então acrescentada ao teste de
`runtime` que já sobe navegador contra página em branco, e os dois controles que
atravessam o caminho passaram a morder.

**Lição**: *um controle negativo que não é pego revela um teste que falta, não um
controle ruim.* A tentação é ajustar o controle até algo falhar; o certo foi
perguntar POR QUE nada falhou — e a resposta foi um elo inteiro sem cobertura,
que agora tem.

---

## H169 — `GROUP_MEMBERSHIP_REQUEST`: o pedido tem palavra própria, e ninguém tinha perguntado qual

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`.

**Onde**: `internal/wa-headless/events/group.go` (`GroupMembershipRequest`, o
mapa de subtipos), `probe_memreq_test.go` (novo), linha
`GROUP_MEMBERSHIP_REQUEST`.

**A medição anterior estava certa e parou uma pergunta cedo.** A H89 mediu o que
CHEGA quando um pedido entra — 9 `chat.changed` mais 1 `message.added`, contra 5
`chat.changed` de uma saída — e concluiu que `chat.changed` é grosso demais para
ser o evento. Correto. Mas **nunca perguntou o que o `message.added` diz**.

Ele diz:

```
group.updated  kind=gp2  subtype=membership_approval_request   x1
chat.changed                                                    x7
```

Uma palavra própria, atravessando a fronteira, dentro da maquinaria que a H119 já
construiu: a página carrega o subtipo, o Go classifica, a invariante 6 intacta.

**Correção aplicada**: `GroupMembershipRequest` como tipo dedicado. Um pedido é
uma DECISÃO PENDENTE dirigida a um admin; os outros subtipos sob `group.updated`
são o grupo anunciando algo já decidido. Quem quisesse agir sobre pedidos tinha
de receber toda mudança de assunto e de foto para achá-los.

**Duas coisas NÃO foram movidas, e as duas asimetrias são deliberadas:**

- `membership_approval_mode` fica em `group.updated`. É a política sendo ligada
  ou desligada — o grupo anunciando uma mudança feita. Arrastá-la junto diria a um
  assinante que alguém pediu para entrar quando ninguém pediu.
- `created_membership_requests` **também fica**, e essa é a assimetria que custa
  explicar: só o subtipo acima foi OBSERVADO. Mover o irmão seria classificar
  pelo NOME — e este mesmo arquivo já recusa o `else` catch-all da referência
  exatamente por isso. Um teste trava a recusa.

**Status**: corrigido, linha para `PROVEN`. Três controles negativos, todos
compilando e falhando.

Prova em SPA real, num grupo descartável: o pedido chega **1 vez e sozinho**, sem
`group.updated` junto — asserção que existe porque um assinante dos dois contaria
o mesmo fato duas vezes.

**Uma escolha de instrumento que valeu**: o barramento só foi ligado DEPOIS de
todo o preparo (criar, sair, ligar aprovação, pegar convite). Ligá-lo antes teria
enchido a medição com três atos alheios — que é precisamente o erro que a H150
cometeu e levou uma rodada para desfazer.

**Lição**: *"este evento é grosso demais" é uma conclusão sobre o evento que
você olhou, não sobre os que chegaram junto.* A H89 tinha o `message.added` nas
mãos, contado e registrado, e a pergunta seguinte — *o que ele diz?* — ficou por
fazer por meses. Toda medição que termina em "não dá para distinguir" merece uma
última passada pelos campos que ela já coletou.

---

## H170 — `sendStateRecording`: o bloqueio confirmado por um caminho que não é a assinatura

**Data**: 2026-08-22
**Contexto**: varredura dos `PARTIAL`.

**Onde**: `internal/wa-headless/probe_chatstate_test.go` (novo), linha
`sendStateRecording`.

**A hipótese**: a linha diz que a prova ao vivo esbarra no bloqueio da observação
de presença, e a H144 transformou esse bloqueio em causa medida — a assinatura
exige vínculo de AGENDA, que se cria no telefone, e o par de laboratório não o
tem (`isMyContact:false` nos dois).

Mas isso é afirmação sobre `presence.Observe`, **não necessariamente sobre o
estado de conversa**. A H150 mediu `typing` disparando no modelo da própria
sessão que age, o que diz que o campo existe e se move. Se o estado chegasse ao
modelo do PAR por outro caminho, a linha fecharia sem depender da assinatura.

**Medição**, conta-A anuncia gravação e conta-B lê o modelo de presença cru:

```
BEFORE: {found:true, hasChatstate:true, type:"",  typing:-1}
AFTER : {found:true, hasChatstate:true, type:"",  typing:-1}   (45s)
```

Nada se moveu. O `type:""` é coerente com a H94, que já tinha medido
`chatstate.type` como indefinido neste build.

**Status**: não corrigido. A linha continua `PARTIAL`, e a causa passa de
*"esbarra no bloqueio de presença"* — que era referência a outra linha — para
**dependência de agenda medida por dois caminhos independentes**: a assinatura
(H144) e o modelo cru (aqui).

**Lição**: *herdar um bloqueio de uma linha vizinha é uma hipótese, não um
diagnóstico.* Custou uma sonda descobrir que a herança estava certa — e teria
custado o mesmo descobrir que estava errada, que foi o que aconteceu com o `pin`
na H162. A diferença entre as duas é só a medição.

---

## H171 — `attachEventListeners` é um agregado, e dizer isso encerra a varredura dos `PARTIAL`

**Data**: 2026-08-22
**Contexto**: última linha `PARTIAL` sem causa registrada.

**Onde**: linha `attachEventListeners` do `LEDGER-WWEBJS.md`.

**A nota — "dois fluxos de 31 eventos" — descrevia o que temos e não o que
falta**, que é a única coisa que uma varredura precisa saber.

Lendo a referência: `attachEventListeners` é uma função que instala **25**
ouvintes via `exposeFunctionIfAbsent`, um por evento. Não é uma capacidade: é o
ato de ligar as outras. O nosso equivalente é o ingresso mais os ouvintes de
capacidade.

**Não há aqui nada próprio a provar.** O estado desta linha é a conjunção das
linhas de evento, e ela fecha quando elas fecharem. Registrar isso vale porque
impede a próxima varredura de gastar uma sonda procurando o que medir — que foi
exatamente o que eu ia fazer.

**Status**: não corrigido, e agora com o motivo certo escrito.

### O estado da varredura, para quem vier depois

Com esta entrada, **todas as 34 linhas `PARTIAL` têm causa registrada**. A
distribuição:

| categoria | o que significa |
|---|---|
| **dependência humana** | presença e estado de conversa (agenda no telefone, H144/H170); foto de perfil e status (visíveis a 944 contatos, decisão 61) |
| **sombra de linha `BLOCKED`** | `description` (H145/H163) |
| **mundo vazio** | broadcasts/status — coleção sincronizada e ninguém postou (H158) |
| **limite do build** | confirmação só entre sessões: participantes (H58/H65) e `pin` (H162) |
| **agregado** | `attachEventListeners` (esta entrada) |
| **herança** | delegações que herdam pares abertos — a enquete que não sai (H98) e o recibo não observado (H159) |
| **vocabulário deliberado** | `DISCONNECTED`, `STATE_CHANGED` — o nosso é transição de liveness com classe anexada, não o vocabulário do upstream |

**O que sobra de genuinamente acionável por este módulo, sozinho, é zero.** Isso
não é o mesmo que dizer que a Fase 1 fechou: as linhas de *vocabulário
deliberado* são candidatas a `INTENTIONAL_DIFFERENCE` em vez de `PARTIAL`, e essa
reclassificação muda o placar — é decisão de critério, não medição, e o critério
é da orquestração (decisões 52/60/62). **A decisão 64 foi pedida e não voltou.**

**Lição**: *uma varredura termina quando toda linha sabe por que está onde está,
não quando não há mais linhas.* O valor do dia não foi só as linhas fechadas —
foi que nenhuma das restantes precisa ser reinvestigada do zero.

---

## H172 / H173 — a decisão 64 chegou, e a condição dela pegou duas justificativas podres

**Data**: 2026-08-22
**Contexto**: a orquestração respondeu a decisão 64 — *"Escolha b, reclassifique
diferenças deliberadas e então encerre a Fase 1 com os BLOCKED devidamente
justificados"*. A segunda metade dessa frase é que produziu esta entrada.

### Primeiro: por que a 64 demorou, e não era demora dela

O pedido foi ao mailbox do ORCA e **nunca chegou**. Conferido o inbox: em quatro
runs independentes, **toda** mensagem substantiva está com `delivered_at` nulo;
só os heartbeats `alive` entregam. Aberta a thread do ChatGPT, a última mensagem
era a resposta 60–63 — a pergunta nunca tinha sido feita ali. Reenviada pelo
canal que funciona, a resposta veio em segundos.

**Registrado como fato operacional**: o mailbox do ORCA não é o canal de decisão;
a thread do ChatGPT é. Um `orchestration send` que retorna `ok` com
`delivered_at: null` **não** entregou.

### A reclassificação (64 b)

`DISCONNECTED` e `STATE_CHANGED` foram para `INTENTIONAL_DIFFERENCE`: a
capacidade É entregue, por desenho próprio e registrado — transição de liveness
com a classe da página anexada, vocabulário nosso, e o motivo escrito
("repetir 'ainda vivo' a cada tique é heartbeat vestido de evento").

**`MEDIA_UPLOADED` e `vote` NÃO foram movidos**, e a recusa é o conteúdo da
decisão: o primeiro é limite do build (não existe momento distinto de "upload
concluído"), o segundo herda a enquete que não sai. Nenhum dos dois é escolha
nossa, e alargar a categoria para caber neles a esvaziaria.

### A auditoria dos `BLOCKED` — e as duas que não sobreviveram

Dos 47, **duas tinham justificativa refutada pelo trabalho desta mesma semana**:

**H172 — `unpin`.** Dizia *"idem `pin` — chamada aceita, nada desfixado (H81)"*,
e a H162 mostrou que a H81 media o pin do lado cego. Remedido, e o resultado é
uma assimetria fina:

```
conta-A fixa    → conta-B (aberta) VÊ            0 → 1
conta-A desfixa → conta-B (aberta) continua vendo por 5 MINUTOS
                → conta-B RECÉM-ABERTA lê 0 logo depois
```

O unpin chega ao **servidor** e não ao modelo de sessão já aberta. Sai de
`BLOCKED` para `PARTIAL`, pela convenção do `addParticipants`/`pin`.

**Uma correção minha no caminho**: a primeira medição deu 60 s e concluiu
bloqueio; minutos depois uma leitura independente mostrou 0. **O prazo era o
instrumento**, e um prazo curto demais produz o mesmo texto que um bloqueio de
verdade. Foi preciso separar "não propagou" de "não propagou AINDA", e o que
separou foi a sessão fresca.

**H173 — `CHAT_REMOVED`.** Dizia que provar exigiria apagar uma conversa e
destruir o fixture — e a H166 apagou uma, num grupo descartável. Remedido: o
apagamento produzia **14 `chat.changed`** e nada dizendo que a conversa sumiu,
porque o ingresso abria só a porta `change` da `ChatCollection`. Aberta a porta
`remove` — o mesmo que a H143 fez para mensagens — o apagamento emite
`chat.removed` uma vez. Linha para `PROVEN`.

**Status**: `PROVEN 134 (61%)`, `PARTIAL 33`, `BLOCKED 45`,
`INTENTIONAL_DIFFERENCE 8`, `MISSING 0`.

**Lição**: *"devidamente justificados" é uma condição com dentes.* A tentação era
ler a frase como formalidade e declarar o encerramento. Auditar de verdade custou
duas remedições — e as duas justificativas podres eram podres **por causa do
trabalho de hoje**, o que significa que um dia produtivo envelhece as próprias
notas mais depressa do que se atualiza. Toda linha que herda veredito de outra
("idem X") é dívida esperando o X mudar.

---

## H174 — Fase 2 abre com a linha de base de teardown, e o instrumento falhou primeiro

**Data**: 2026-08-22
**Contexto**: primeira medição da Fase 2 (decisão 65). O enunciado pede provar
"sem leaks/orphans/races", e a regra do projeto manda medir ANTES de projetar.

**Onde**: `internal/wa-headless/probe_teardown_test.go` (novo).

**A lacuna que abriu esta entrada**: `NumGoroutine` aparece em **zero** testes
deste módulo. A suíte de shutdown prova o CAMINHO do protocolo — que `CleanStop`
sinaliza, espera e rotula uma recusa — e não diz nada sobre **o que sobra depois**.

**Medição**, três ciclos de boot/stop contra o SPA real:

```
BASELINE:  goroutines=2   chromes(perfil)=0
ciclo 1    LIVE 13 / 9     AFTER 2 / 0    (stop via browser.close)
ciclo 2    LIVE 13 / 9     AFTER 2 / 0
ciclo 3    LIVE 13 / 9     AFTER 2 / 0
```

**Sem vazamento e sem órfão.** As 13 goroutines vivas voltam a 2 — o valor da
linha de base — e os 9 processos de Chrome do perfil são todos ceifados. Três
ciclos idênticos separam vazamento de aquecimento de runtime, que uma medição
única não distingue.

**Duas decisões de instrumento que valeram:**

1. **Contar Chrome só DESTE PERFIL.** Contar todo Chrome da máquina mediria o
   navegador do usuário — erro que quase cometi hoje ao investigar carga, e que
   só não virou dano porque conferi antes de matar processo.
2. **Deixar o runtime assentar antes de contar.** Ler `NumGoroutine` logo após um
   `Stop` conta as que ainda estão morrendo, e isso produz um "vazamento" que
   some sozinho.

**E o instrumento falhou antes de acertar, pela armadilha que este repositório já
tem catalogada.** A primeira versão contava com `grep -c`, que **sai com código 1
quando conta zero** — e zero é justamente a resposta que o teste precisa poder
ler. Todo teardown devolveu `-1`, e eu quase li isso como "não deu para medir"
quando era o resultado certo com o instrumento errado. Trocado por `grep -F | wc -l`.

**Status**: linha de base estabelecida. Nenhum defeito encontrado — o que é
resultado, não ausência dele: agora existe número contra o qual comparar quando
reconexão, multi-sessão e carga entrarem.

**Lição**: *a primeira medição de uma fase mede o instrumento tanto quanto o
sistema.* Duas das três decisões acima existem porque a versão ingênua teria
mentido, e a terceira porque ela mentiu.

---

## Decisões 66 e 67 da orquestração — 2026-08-22

Pedidas como primeira parada da Fase 2 (as dívidas internas acionáveis), ambas
mudanças de CONTRATO e por isso não decididas aqui.

> **66: Escolha b; identidade não resolvida deve falhar explicitamente, sem rede
> oculta nem regras duplicadas.**

Ou seja: leitores que recebem `@c.us` num build LID-first **recusam** com erro
próprio, em vez de (a) resolver internamente — que esconderia ida à rede dentro
de um leitor — ou (c) aceitar as duas formas — que duplicaria a regra de
identidade em cada leitor. Aplica-se a `addressbook.DeviceCount`, `chats.ByJID` e
`chats.MarkUnread` (H148, H151).

> **67: Escolha a; Clear deve provar redução e falhar quando a pós-condição não
> ocorrer.**

`chats.Clear` ganha `MessagesAfter` e recusa quando não diminuir — com a
notificação de sistema (`e2e_notification`) tratada como sobrevivente legítima,
senão a nova pós-condição falharia sempre (H166).

---

## H175 — decisão 67 aplicada: `Clear` prova redução, e a borda foi MEDIDA antes de virar regra

**Data**: 2026-08-22
**Contexto**: primeira dívida interna da Fase 2, decidida pela orquestração
(*"67: Escolha a; Clear deve provar redução e falhar quando a pós-condição não
ocorrer"*).

**Onde**: `internal/wa-headless/capabilities/chats/lifecycle.go`
(`Emptied.MessagesAfter`, `ErrNotEmptied`, `residualKind`, o passo de verificação
do `lifecycleScript`), `probe_clearedge_test.go` (novo).

**O comentário que segurava o defeito**, no próprio script:

> COUNTED BEFORE, because after is meaningless: the point of both acts is that
> there is nothing left to count.

É **falso**, e foi essa frase que manteve a capacidade sem pós-condição por
meses — um `Clear` que não apagasse nada devolvia exatamente o mesmo valor de
sucesso.

**A regra óbvia — `after < before` — está errada, e medir mostrou por quê.** Três
clears no mesmo grupo descartável:

```
A) grupo virgem     2 (e2e_notification, gp2)   -> 1 (e2e_notification)
B) com 2 mensagens  3 (e2e_notification, chat)  -> 1 (e2e_notification)
C) já limpo         1 (e2e_notification)        -> 1 (e2e_notification)
```

O caso **C** é a borda: limpar o que já está limpo remove zero, **corretamente**,
e `after < before` chamaria isso de falha. A decisão 67 teria virado um defeito.

O caso **A** matou a outra hipótese que eu carregava: eu esperava que um chat só
com sistema não reduzisse — reduziu, porque o `gp2` **é** limpo. Só o
`e2e_notification` sobrevive, nos três.

**A regra encodada**: *não sobrou nada LIMPÁVEL*, com `residualKind` nomeando o
tipo medido. Se um build futuro deixar outro tipo para trás, isto **falha alto**
em vez de passar quieto — a direção que faz alguém remedir.

**Assimetria deliberada**: `Delete` **não** ganha essa verificação. Ele remove a
conversa, então recontar mensagens dela não tem o que ler; a pós-condição dele é
o chat sair da coleção, provada na H166. Um teste trava a assimetria.

**Status**: corrigido. Cinco controles negativos, todos compilando e falhando:
remover a pós-condição; exigir zero total (o resíduo viraria falha); usar
"diminuiu" em vez de "não sobrou limpável" (quebra o caso C); verificar ANTES de
aplicar (teste de ORDEM); e fazer o `Delete` verificar também.

Prova em SPA real: os três casos passam pela capacidade — `2->1`, `3->1`, `1->1`.

**Um detalhe do dublê que virou comentário**: os testes antigos de `Clear`
continuaram verdes porque o dublê não declarava `clearable`, e zero é o caso de
sucesso. Isso é correto para eles e seria armadilha se não estivesse dito — o
campo agora tem comentário avisando que **um teste que quer exercitar a
pós-condição TEM de declará-lo**.

**Lição**: *a borda decide a forma da regra, e só a medição conhece a borda.* Eu
tinha DUAS hipóteses sobre o caso difícil — "chat só com sistema não reduz" e
"limpar duas vezes não reduz" — e a medição derrubou a primeira e confirmou a
segunda. Encodar qualquer uma delas sem medir teria produzido uma pós-condição
que falha em uso normal, que é pior que não ter pós-condição nenhuma.

---

## H176 — decisão 66 aplicada, e o quarto leitor foi POUPADO por medição

**Data**: 2026-08-22
**Contexto**: segunda dívida interna da Fase 2 (*"66: Escolha b; identidade não
resolvida deve falhar explicitamente, sem rede oculta nem regras duplicadas"*).

**Onde**: `internal/wa-headless/spa/jid.go` (novo, `IsUnresolvedIdentity`),
`capabilities/chats/chats.go` e `markunread.go`,
`capabilities/addressbook/addressbook.go`, `probe_identity66_test.go` e
`probe_markread66_test.go` (novos).

**A regra mora em UM lugar**, que é metade da decisão: `spa.IsUnresolvedIdentity`.
Três cópias de uma função de três linhas seriam exatamente as "regras
duplicadas" que a 66 proíbe — e a primeira a divergir seria a que ninguém releu.

**O erro é próprio e distinguível**, que é a outra metade. `ErrUnresolvedIdentity`
NÃO é `ErrNoChat` nem `ErrDevices`: responder *"no conversation for that jid (384
in this session)"* sobre alguém com quem a sessão fala todo dia é honesto sobre a
COLEÇÃO e falso sobre o MUNDO, e a mensagem antiga não dizia qual dos dois.

**E a recusa vem ANTES do trabalho.** O `ByJID` lê as 384 conversas para
responder sobre uma; gastar isso para devolver uma resposta que ele não sabe dar
é desperdício em cima de resposta errada. Um controle negativo trava a ordem.

### O quarto leitor: medido, e POUPADO

`chats.MarkRead` tem a MESMA FORMA dos três — procura a conversa na coleção — e
**não foi alterado**, porque inferir por forma é precisamente o que produziu esta
dívida.

A primeira medição foi **inconclusiva** e quase me convenceu: as duas formas
devolveram `before=0`, porque a conversa não tinha não-lidas. *Sem não-lida,
no-op e falha silenciosa são indistinguíveis.* (E o meu comparador ainda incluía
`waited`, que difere por 1 ms — bug meu, não do sistema.)

Com a não-lida PRODUZIDA por conta-B, e o telefone testado **primeiro** para que
a ordem fosse o experimento:

```
MarkRead(phone) = before=1 after=0 changed=true
MarkRead(lid)   = before=0 after=0 changed=false   (já não havia o que fazer)
```

**O jid de telefone FUNCIONA.** `MarkRead` resolve por um caminho que os outros
três não usam. Estendê-lo a recusa teria quebrado uma capacidade que funciona.

**Status**: corrigido. Quatro controles negativos, todos compilando e falhando —
incluindo o de ORDEM e o que faz grupos serem recusados (que morde em dois
pacotes).

**Quebra de chamador, aceita pela decisão**: seis testes quebraram porque usavam
`@c.us` como fixture INCIDENTAL — mediam outra coisa e o jid era enfeite. Trocados
para `@lid`, o que eles medem continua medido. Um deles, o
`TestAnUnknownUserIsNotZeroDevices`, é justamente o que separa "sem registro" de
"zero dispositivos" — a distinção que tornou a H148 diagnosticável.

**Lição**: *a regra certa não é "todo leitor recusa", é "responda se puder, recuse
explicitamente se não puder, nunca responda errado".* Essas duas formulações
parecem a mesma até você medir o quarto caso. A primeira teria custado uma
capacidade funcional; a segunda é uma regra só, e os leitores diferem apenas em
qual metade dela se aplica.

---

## H177 — SEVERIDADE ALTA: duas leituras concorrentes trocavam de resposta

**Data**: 2026-08-22
**Contexto**: Fase 2, item "concorrência/multi-sessão". Primeiro defeito real da
fase, e o mais grave achado neste módulo desde que o ledger existe.

**Onde**: `internal/wa-headless/capabilities/message/message.go`
(`stateKeyPrefix`, `nextStateKey`, `parked`), `script.go` (os sete scripts),
`probe_concurrency_test.go` (novo).

**O defeito**: todo leitor de `capabilities/message` estacionava a resposta no
MESMO global de página — `__waHeadlessMessage`. Duas chamadas concorrentes na
mesma sessão escreviam a mesma variável, e cada uma consultava até ela ficar
não-vazia. Quem consultasse primeiro levava a resposta da OUTRA.

**Medição**, contra o build real, com duas mensagens de CONVERSAS DIFERENTES —
o chat de cada uma é a etiqueta que denuncia a troca:

```
12 rodadas concorrentes: 12 respostas TROCADAS de 24, 0 erros
```

**Cinquenta por cento, e todas as rodadas.** Não é uma corrida rara: é o
comportamento normal de duas leituras simultâneas.

**E a resposta errada era BEM FORMADA**, que é por que nada pegou. Um `OriginOf`
devolvia um chat, um remetente e um timestamp perfeitamente válidos — de outra
mensagem. Nenhum teste podia notar, porque todos chamavam uma capacidade por vez.

**Por que isto é da Fase 2 e não da Fase 1**: toda prova de paridade chamou uma
capacidade de cada vez, que é o cenário em que o mecanismo PAGA. A regra do
projeto escrita depois do pool da F86 manda medir onde ele COBRA — e para um
global compartilhado, cobrar são dois chamadores.

**Correção**: a chave vira PREFIXO, e cada leitura recebe a sua, de um contador
atômico. O nonce vem do Go e não da página: `Math.random` ou `Date.now` lá dentro
poriam uma decisão — e um relógio — onde a invariante 6 proíbe.

**A chave é LIBERADA assim que a resposta é tomada**, e isso não é zelo: sem
isso, a correção trocaria uma resposta cruzada por um global de página POR
LEITURA, que uma sessão longa acumula. Trocar um defeito por outro é o que a
Regra 4 do `CLAUDE.md` manda verificar, e um teste trava a liberação.

**Status**: corrigido em `capabilities/message`. Três controles negativos, todos
compilando e falhando: voltar a chave única, o script ignorar a chave e escrever
o global, e não liberar. Prova em SPA real: **80 leituras concorrentes, 0
trocas**, onde antes eram 12 de 12.

### O que NÃO está corrigido, e é o mesmo defeito

**Vinte e três capacidades declaram um `stateKey` único**, com o mesmo padrão
estacionar-e-consultar: `addressbook`, `avatar`, `block`, `catalog`, `channel`,
`call`, `chatstate`, `edit`, `forward`, `lookup`, `pin`, entre outras. A troca
não foi MEDIDA nelas, mas a estrutura é idêntica e a de `message` foi medida em
50% — presumir que as outras estão a salvo seria a inferência por forma que este
mesmo dia já derrubou duas vezes.

`lookup` merece nota: ele é chamado DE DENTRO de outras capacidades, então uma
resolução de identidade concorrente com uma leitura é o padrão de produção mais
provável de todos.

**Isto é achado acionável de severidade alta em aberto**, e o critério de
encerramento da Fase 2 (decisão 65) exige zero deles.

**Lição**: *um teste que só chama uma coisa por vez não testa um recurso
compartilhado — testa a ausência de concorrência.* O padrão estacionar-e-consultar
foi escrito para respeitar a invariante 6 (o relógio fica no Go) e resolveu esse
problema bem; ninguém perguntou o que ele faz quando há dois. A pergunta da Fase
2 — *qual entrada faz esta proteção virar o problema?* — respondeu em uma sonda.

---

## H178 — a correção da H177 estendida a mais três, e a varredura mecânica REVERTIDA

**Data**: 2026-08-22
**Contexto**: propagar a correção da H177 (chave por chamada) para as 23
capacidades restantes.

**Onde**: `capabilities/lookup`, `capabilities/catalog`, `capabilities/phone`,
`capabilities/search`.

**Corrigidas e provadas: 5 de 24** — `message` (H177), mais `lookup`, `catalog`,
`phone` e `search`.

`lookup` foi primeiro por um motivo registrado na H177: ele roda **dentro** de
outras capacidades, então uma resolução de identidade concorrente com uma leitura
é o padrão de produção mais provável, não uma corrida artificial. Três controles
negativos, todos compilando e falhando.

### A varredura mecânica foi tentada e revertida — e isso é o achado

Os 23 restantes parecem uniformes: `window.` + stateKey para estacionar, um
`parked(ctx, kick, label)` para consultar. Escrevi um transform e apliquei a sete
de uma vez. **Quatro quebraram o build**, e as causas foram todas de forma:

- `call` e `settings` têm um `const prelude` que embute a chave; um `const` não
  pode receber parâmetro, então ele tem de virar função — e com ele todos os
  `const xxxScript` que o usam.
- `status` tem a mesma forma.
- `groupreq` declara a chave dentro de um bloco `const (...)`, que o meu regex
  não via.

Revertidos os quatro, o build voltou. **Mantive apenas o que está verde e
verificado**, porque meia-correção espalhada por sete pacotes é pior que nenhuma:
o build quebrado é visível, mas um pacote parcialmente convertido que compila não
é.

**Dois erros meus no caminho, os dois de ferramenta:**

1. **`zsh` não divide `$var` em palavras.** O `set -- $spec` passou
   `"addressbook m"` como argumento único, e os transforms falharam sem tocar
   arquivo nenhum — barulho, não dano, e só porque olhei a saída.
2. **Ordem de substituição no meu próprio script**: um `replace` genérico rodou
   depois de um específico e produziu `stateKeyPrefixPrefix`. O compilador pegou.

**Status**: 5 de 24 corrigidas. **19 continuam com o defeito da H177**, que segue
sendo achado acionável de severidade alta em aberto — o critério de encerramento
da Fase 2 exige zero.

**Lição**: *"os arquivos parecem iguais" é uma hipótese sobre a forma, e forma se
mede lendo, não olhando.* Cinco pacotes seguiram o molde e quatro não, com quatro
motivos diferentes. O transform economizou tempo nos cinco e o teria custado com
juros se eu tivesse confiado nele sem construir depois de cada um — que é a mesma
regra que este repositório aplica a dublês e a controles negativos, agora aplicada
à ferramenta que escreve o código.

---

## H179 — 16 de 24 corrigidas, e o molde parou de servir duas vezes

**Data**: 2026-08-22
**Contexto**: propagar a chave por chamada (H177) ao resto do módulo.

**Corrigidas e travadas: 16 de 24.** `message`, `lookup`, `catalog`, `phone`,
`search`, `call`, `settings`, `status`, `groupreq`, `block`, `avatar`, `edit`,
`forward`, `chatstate`, `mute`, `profile`. Cada uma com uma guarda
(`conckey_test.go`) que falha se duas chamadas voltarem a compartilhar chave.

**Restam 8**: `addressbook`, `channel`, `media`, `messagemeta`, `pin`, `poll`,
`presence`, `react`, `revoke`, `star`.

### As formas que o molde não cobriu

Duas conversões diferentes tiveram de ser escritas, e nenhuma serviu ao resto:

**Forma B** (`window.` + stateKey): resolvida para nove pacotes. Três deles —
`call`, `status`, `groupreq` — tinham um `const prelude` embutindo a chave; um
`const` não aceita parâmetro, então virou função, e com ele todos os
`const xxxScript` que o usavam.

**`settings` foi o caso mais instrutivo**: ele montava o script ANTES de a chave
existir. Inverti para o `write` receber um CONSTRUTOR `func(key string) string`.
A alternativa — o chamador gerar a chave e passá-la duas vezes — é convite a
passar chaves diferentes, o que reintroduziria o defeito de forma mais difícil de
ver que a original.

**Forma A** (`window[strconv.Quote(stateKey)]` + um `resultScript` const):
resolvida para seis. Aplicada a doze de uma vez, **nove quebraram o build** —
`avatar` chama o seu de `pollScript`, e os outros têm variações próprias.

**Revertidos os nove, mantidos os dois verdes**, pela regra que a H178 já custou:
meia-correção espalhada é pior que nenhuma, porque o build quebrado é visível e
um pacote parcialmente convertido que COMPILA não é.

### Um erro meu que sete pacotes esconderam

Ao ensinar os dublês sobre a chave nova, pus a liberação **no mesmo ramo da
leitura**. Ela passou a contar como leitura, e o `groupreq` — o único com teste
que afere o NÚMERO DE VOLTAS do laço — falhou com "4 reads".

Os outros seis estavam verdes **pelo motivo errado**. Na forma A o erro era pior:
sem ramo próprio, a liberação caía no `default` e virava `lastScript`, de modo que
todo teste que afirma sobre o script passava a inspecionar o script de limpeza.

**É a armadilha do dublê permissivo do `ARMADILHAS.md`**, cometida por mim,
minutos depois de escrever a correção que ela deveria proteger. Sete dublês
corrigidos com ramo próprio, e um comentário em cada dizendo por quê.

**Status**: `make check` verde sobre as 16; 35 pacotes de capacidade passam.
**As 8 restantes continuam com o defeito da H177** — achado acionável de
severidade alta em aberto, e a decisão 65 exige zero.

**Lição**: *um transform é uma hipótese sobre a forma, e como toda hipótese ele
precisa de controle.* O controle aqui é o build depois de CADA pacote, não
depois do lote — foi o que separou 16 corrigidas de 24 quebradas, duas vezes.

---

## H180 — 19 de 26, e a contagem original estava errada

**Data**: 2026-08-22
**Contexto**: continuação da H179.

**Primeiro, uma correção de número**: a H177 disse "23 capacidades restantes" e a
H179 disse "24 no total". **São 26.** A contagem original veio de um `grep` por
`const stateKey = "` que não via as declarações dentro de blocos `const (...)` —
o mesmo motivo que fez o `groupreq` escapar do primeiro transform. Um número
errado num achado de severidade alta é pior que nenhum, porque dá a impressão de
que o fim está mais perto do que está.

**Corrigidas e travadas: 19 de 26.** Somam-se às 16 da H179: `poll`,
`addressbook` e `channel`.

- **`addressbook`** trouxe um caso novo: `nameStateScript` NÃO estaciona — devolve
  o JSON direto — mas usa o `prelude` pelos helpers, e o prelude **cria o global
  mesmo assim**. A chave é gerada e liberada ali, senão a correção deixaria um
  órfão por chamada num caminho que nem consulta o estacionado.
- **`channel`** tem DOIS laços de espera, em `channel.go` e `owner.go`, cada um
  com o seu conjunto de chamadores.

**Restam 7**: `media`, `messagemeta`, `pin`, `presence`, `react`, `revoke`,
`star`.

### Por que os 7 pararam, e o que aprendi ao insistir

Tentei convertê-los com os transforms já validados e eles **colidiram entre si**:
o de forma A já acrescentava `(key)` ao `resultScript`, e o de sítios extras
acrescentava outro — `too many arguments`. Cada um desses pacotes tem chamadas de
script FORA do sítio do kick, e o número e a forma delas variam.

Revertidos os 7, o build voltou e os 35 pacotes de capacidade passam.

**Um resíduo que quase escapou**: a reversão devolveu o código, mas os arquivos de
guarda (`conckey_test.go`) que eu tinha criado ficaram para trás, referindo
símbolos que já não existiam. **Dois pacotes ficaram sem compilar por causa da
minha limpeza, não da minha mudança** — e o gate que eu rodei em cima disso falhou
em `cmd/core` por essa razão, não por defeito real. Removidos os órfãos.

**Status**: 19 de 26. **7 continuam com o defeito da H177** — achado acionável de
severidade alta em aberto.

**Lição**: *reverter não é desfazer.* Um `git checkout` devolve os arquivos
rastreados e deixa os que você CRIOU, e o que sobra costuma referir o que sumiu.
A verificação depois de reverter tem de ser a mesma que depois de mudar — build e
teste —, e eu só a fiz porque o `go test` reclamou; se tivesse confiado no
`checkout`, teria commitado uma árvore que não compila em dois pacotes.

---

## H181 — 25 de 26, e a 26ª está fora POR DESENHO

**Data**: 2026-08-22
**Contexto**: fechamento do defeito de severidade alta da H177.

**Corrigidas e travadas: 25 de 26.** As sete últimas — `star`, `revoke`, `media`,
`pin`, `presence`, `react` e `messagemeta` — feitas à mão, lendo cada arquivo,
depois de os transforms terem falhado três vezes na cauda.

### A 26ª não é dívida: `messagemeta` fica com chave única, de propósito

**A diferença é O QUE A CHAVE GUARDA.** Nas 25, ela guarda a RESPOSTA de uma
chamada, e duas chamadas concorrentes escreviam a mesma variável — 12 cruzamentos
em 12 rodadas. No `messagemeta` ela guarda uma **ASSINATURA** de longa duração:
`installScript` instala uma vez (e sai cedo se já estiver instalada) e
`drainScript` esvazia o buffer. **Há uma assinatura por sessão por desenho**, e
dar-lhe chave por chamada quebraria exatamente isso — o install escreveria uma
chave e o drain leria outra.

Registrado no próprio arquivo, para que a próxima varredura não o "conserte".

### O que a cauda ensinou, caso por caso

- **`revoke`**: o `loadedScript` — que eu mesmo escrevi na H143 — **não estaciona**,
  devolve JSON direto. O transform deu-lhe chave só por casar `\w+Script`.
  Assinatura revertida. Mesmo caso em `pin.pinnedInScript` e `react.readScript`.
- **`presence`**: mesma forma do `settings` — `run` gerava a chave e o script vinha
  pronto do chamador. Invertido para construtor.
- **`addressbook`** (H180): script que não estaciona mas usa o `prelude` pelos
  helpers, e o prelude cria o global mesmo assim.

**Três dessas correções foram REVERSÕES de algo que o transform tinha feito.** Um
nome terminado em `Script` não diz se a função estaciona; só o corpo diz.

### Um dublê que escondia o erro em dois de três

O `revoke` tem **três** dublês e o meu ajuste entrou só no primeiro. O
`TestTheLocalDeleteDoesNotConsultTheRevokeEntitlement` falhou porque a liberação
virou `lastScript` no `localDouble`. Corrigido para percorrer TODOS os dublês de
cada arquivo.

**Status**: `make check` verde, 35 pacotes de capacidade passam, e a sonda de
concorrência contra o build real segue com **0 respostas trocadas**. O achado de
severidade alta da H177 está **fechado**.

**Lição**: *automação cobre o meio da distribuição e a cauda é onde a decisão
mora.* Os transforms fizeram 19 de 26 e falharam em todas as sete restantes, cada
uma por um motivo distinto — e três delas precisavam do OPOSTO do que o transform
fazia. O tempo que economizaram foi real; o tempo que teriam custado se eu tivesse
insistido também.

---

## H182 — a chamada em voo quando o navegador morre: medida, e o erro que ela devolve merece uma decisão

**Data**: 2026-08-22
**Contexto**: Fase 2, item "reconexão / recuperação determinística".

**Onde**: `internal/wa-headless/runtime/inflight_test.go` (novo).

**A lacuna**: o contrato do `Holder` para sessão morta está documentado e testado
— `ErrSessionDied`, e ele recusa rebootar em vez de esconder um navegador que
morre sempre. O que nada media é a chamada **já em voo** quando o processo some.
Duas falhas são possíveis e só uma é aceitável: voltar classificada dentro do
prazo, ou pendurar.

**A escolha de fixture é parte do achado.** O teste usa perfil TEMPORÁRIO e página
local, não o de laboratório: `SIGKILL` contra um perfil **pareado** arrisca
corrompê-lo, e repareamento exige um humano com o telefone — custo que esta
medição não tem direito de gastar. O mecanismo sob teste (`Runner.Do`, o
transporte CDP, o prazo) é o mesmo; muda só a credencial em risco.

**Medição**, com uma avaliação que ocupa a página por 120 s e o `SIGKILL` chegando
aos 2 s:

```
a chamada em voo voltou em 2.027s com err=context canceled
goroutines: 15 antes, 3 depois
Session() seguinte: ErrSessionDied
```

**Não pendura, não vaza, e a sessão seguinte diz que morreu.** A recuperação é
determinística. Esta é a linha de base do item.

### O que sobra, e é decisão e não defeito

O erro é **`context canceled`** — o vocabulário de um cancelamento pedido pelo
CHAMADOR —, e o contexto do chamador aqui era `context.Background()`, que ninguém
cancelou. Quem receber isso não tem, na mensagem, nada que diga "o navegador
morreu".

**E isso é consistente com um princípio já escrito no `Runner`**: *"o contexto é
consultado, não o erro: um driver é livre para reportar um prazo estourado como o
erro que quiser, e confiar na redação dele põe a classificação nas mãos de
outro"*. O `Runner` deliberadamente NÃO interpreta erro de driver, e por isso o
`context canceled` do chromedp passa cru.

Um chamador PODE distinguir — se o próprio contexto não está encerrado e mesmo
assim veio `context canceled`, não foi ele. Mas isso é implícito, e o módulo tem
precedente para tornar explícito: o `TimeoutError` existe exatamente para que
"não respondeu" não se confunda com "respondeu com erro".

**Registrado como pergunta de desenho, não como defeito**, porque mexer nisso
contraria uma razão que já está escrita — e trocar um princípio por outro é
decisão de contrato. Opções: (a) deixar como está e documentar a discriminação
implícita; (b) o `Runner` passar a classificar "alvo sumiu" quando o processo já
não existe, o que exige consultar o PID e não a mensagem; (c) o `Holder` expor um
sinal que o chamador consulta ao ver erro inesperado.

**Status**: linha de base estabelecida, sem defeito encontrado. Uma pergunta de
contrato aberta.

**Lição**: *"não encontrei defeito" só é resultado se a medição podia tê-lo
encontrado.* Este teste falha se a chamada pendurar, se voltar dizendo sucesso, se
deixar goroutine, ou se a sessão seguinte rebootar em silêncio — quatro modos, e
por isso o verde significa alguma coisa.

---

## H183 — decisões 68 e 69 aplicadas, e a sonda de vida deadlockou duas vezes antes de acertar

**Data**: 2026-08-22
**Contexto**: Fase 2. A orquestração respondeu as duas perguntas que a H182
deixou abertas.

> **68: Escolha b e registre a recuperação ponta a ponta como bloqueada por
> exigir risco humano no fixture pareado.**
>
> **69: Escolha b; classifique alvo desaparecido estruturalmente pelo estado do
> processo, nunca pela mensagem do driver.**

**Onde**: `engine/runner.go` (`TargetGoneError`, `Runner.TargetAlive`),
`engine/browser.go` (`Browser.Exited`), `runtime/holder.go` (`browserGone`,
`targetGoneGrace`), `runtime/inflight_test.go`.

### Carga, antes das decisões

1000 chamadas concorrentes numa sessão: **1,553 s, p50 40 ms, p95 233 ms, pior
451 ms, zero erros**, com os globais `__waHeadless*` em **0 antes e 0 depois** e
goroutines 12 → 12. A liberação de chave da H177 aguenta volume — e a primeira
medição, 72 chamadas em 110 ms, era amostra e não carga; foi refeita por isso.

### A sonda de vida: dois deadlocks e uma corrida

A 69 pede o estado do PROCESSO, nunca a mensagem. Chegar lá custou três versões:

1. **Pegava `h.mu`.** `Session()` segura esse lock durante TODO o boot, e o boot
   usa o Runner — a sonda travaria a si mesma.
2. **Chamava `sess.ProcessAlive`**, que pega o lock da SESSÃO. Quando o navegador
   morre alguém já o está segurando: **o teste pendurou por 3 minutos**.
3. **Consultava o SO com `ProcessAlive(pid)`** e dizia "vivo": um processo morto
   há um instante é **ZUMBI** até o pai ceifá-lo, e **sinal 0 a um zumbi
   SUCEDE**.

**A regra que sobra vale além daqui: uma sonda de vida não pode compartilhar lock
com aquilo cuja vida ela reporta.**

O sinal autoritativo é o canal que o reaper fecha, exposto como `Browser.Exited`
— sem lock, e muda exatamente uma vez. Mas mesmo ele perdia a corrida: a chamada
volta **2,015 s** depois de um kill agendado para 2 s, ou seja no instante exato,
e o reaper ainda não rodou. Daí `targetGoneGrace`, uma janela de 500 ms que vive
**só no caminho de erro** — o Runner consulta a sonda apenas quando a operação já
falhou e nenhum dos dois prazos explica. Chamada saudável não paga nada.

**Resultado**: `engine: the browser is gone (StateProbe inflight/kill)`, em 2,017 s,
sem vazamento, e a `Session()` seguinte continua dando `ErrSessionDied`.

### Quatro controles negativos, e TRÊS não morderam de primeira

- **Classificar pela mensagem**: o teste usava um erro qualquer, que a redação
  nunca acusaria. Refeito com `context.Canceled` — a entrada que a mensagem
  ACUSARIA —, e aí mordeu.
- **Acusar sem sonda**: o teste usava `errors.Is`, e `TargetGoneError` desembrulha
  para a causa, então passava. A asserção tem de ser sobre o TIPO.
- **Reclassificar antes do prazo**: a ordem está protegida em DOIS lugares — a
  posição do bloco e o `!timedOut` na condição. Quebrar um só não muda nada;
  quebrando os dois, o teste falha.

**Os três eram testes fracos meus, não controles ruins** — que é exatamente o que
a H168 já tinha registrado sobre controles que não mordem.

### 68: registrado como bloqueado

Recuperação de perfil sujo ponta a ponta (marcador suspeito → verificação →
ready) **fica sem medição**, por decisão: produzi-la exige matar o navegador num
perfil PAREADO, e o pior caso é repareamento por um humano com o telefone. O que
foi medido usa perfil temporário e mede o mecanismo, não o caminho completo.

**Lição**: *o instante em que uma coisa morre não é o instante em que o sistema
sabe disso.* Três camadas discordaram por centenas de milissegundos — o driver, o
sistema operacional e o reaper —, e a correta é a que muda uma vez só.

---

## H184 — auditoria de sucesso silencioso: nenhuma escrita mente, e há DOIS padrões de honestidade

**Data**: 2026-08-22
**Contexto**: Fase 2, item "observabilidade sem sucesso silencioso".

**A pergunta**: existe alguma escrita que devolve sucesso sem ter lido nada de
volta? Foi o defeito da H175 (`chats.Clear`) e o da H155 (`react.Remove`), os dois
já corrigidos — e a pergunta é se sobrou mais algum.

**Resultado: nenhum.** Das 49 escritas exportadas, zero devolvem sucesso sem
poder justificá-lo.

### O instrumento errou primeiro, e foi corrigido antes de virar achado

A primeira varredura acusou **35 de 49**. Estava errada: quase todas são
invólucros de uma linha que delegam a um helper que VERIFICA — `block.Block` tem
39 bytes e é `return b.set(...)`. Reportar isso teria produzido 35 falsos
positivos, que é exatamente a classe de achado que este repositório chama de pior
que nenhum. Corrigido para **seguir a delegação** até três níveis.

**E o instrumento foi validado por dois casos discriminantes**, escolhidos porque
eu sabia a resposta:

- `chatstate.SetArchived` → delega para `set`, que relê e **falha** com
  `ErrUnchanged` quando o campo não mexeu. Auditor certo.
- `group.Promote` → delega para `setAdmin`, que devolve `Verified: false` para
  mudança real, porque este build não confirma na mesma sessão (H65). Auditor
  certo de novo, e este era o caso capaz de desmenti-lo.

Sem esses dois, a auditoria seria um regex opinando.

### Os dois padrões, e a diferença que importa

| padrão | pacotes | o que faz |
|---|---|---|
| **erro** | `block`, `channel`, `chats`, `chatstate`, `edit`, `revoke` | recusa quando a pós-condição não ocorre; o chamador **tem** de tratar |
| **sinalizador** | `channel`, `chats`, `group`, `pin`, `react` | devolve `Verified: false`; o chamador **pode** ignorar |

O sinalizador é usado exatamente onde verificar é impossível — confirmação só
entre sessões (H58, H65, H162) —, e nesses casos ele é a resposta honesta: erro
seria mentir sobre uma operação que provavelmente funcionou.

**Mas a honestidade do sinalizador é DISPONÍVEL, não IMPOSTA.** Um chamador que
não lê o campo recebe um sucesso indistinguível de um verificado. Do lado dele, é
sucesso silencioso outra vez — só que a culpa mudou de lugar.

**Registrado como observação de desenho, não como defeito**: a escolha entre os
dois padrões está correta caso a caso, e transformar sinalizador em erro faria
capacidades que funcionam passarem a falhar. Se a Fase 2 quiser fechar essa
brecha, o caminho não é o tipo de retorno — é um teste de fronteira no `pkg/` que
recuse ignorar o campo.

**Status**: item medido, nenhum defeito. Uma brecha de contrato descrita.

**Lição**: *uma auditoria por padrão textual precisa de dois casos cuja resposta
você já sabe — um de cada lado.* Eu tinha os dois de graça, do trabalho da própria
Fase 1, e sem eles teria publicado 35 falsos positivos ou confiado num verde que
não medi.

---

## H185 — long-running e memória: nove minutos, e a memória DESCE

**Data**: 2026-08-22
**Contexto**: Fase 2, itens "long-running" e "limites de CPU/RAM".

**Onde**: `internal/wa-headless/probe_longrun_test.go` (novo).

**A pergunta que a linha de base de teardown NÃO responde**: a H174 mediu que uma
sessão que ACABA não deixa nada. Se uma sessão que FICA cresce enquanto fica é
outra coisa, e o enunciado pede as duas.

**Medição**, uma sessão sob trabalho constante por nove minutos, amostrando a cada
10 s:

```
RSS médio (terço inicial → terço final):  727088KB → 638168KB   (-12%)
goroutines:                                10 → 10, constante
globais __waHeadless*:                     0 em todas as amostras
latência:                                  6ms a 60ms, quase toda abaixo de 20ms
```

**A memória DESCE.** O RSS oscila entre ~590MB e ~675MB e termina 12% abaixo de
onde começou — a queda é a hidratação inicial sendo devolvida, não um efeito da
carga. Goroutines não se movem. Globais ficam em zero, o que confirma sob duração
o que a carga já confirmara sob volume (H183): a liberação de chave da H177
aguenta.

**Número para capacidade, que é o item de RAM**: uma sessão custa da ordem de
**600–700MB de RSS** somando toda a árvore de processos do Chrome. Quem
dimensionar quantas sessões cabem numa máquina precisa desse número, e ele não
existia escrito em lugar nenhum.

**Duas decisões de instrumento, e as duas mudam a resposta:**

1. **O RSS soma a ÁRVORE, não o processo pai.** O Chrome é multiprocesso; medir
   só o pai reportaria estabilidade enquanto um renderer cresce — exatamente o
   vazamento que este teste existe para pegar.
2. **A comparação é entre TERÇOS, não entre extremos.** O primeiro minuto ainda
   tem aquecimento e uma amostra final isolada é ruído. Vazamento é tendência, e
   tendência não se lê em dois pontos.

**E a medição rodou sozinha.** Nada pesado em paralelo, de propósito: contenção
falsearia o número, que é o erro que a própria Fase 2 já cometeu hoje quando 72
chamadas em 110 ms se passaram por carga.

**Status**: os dois itens medidos, nenhum defeito. O teste falha se as goroutines
crescerem, se o RSS subir mais de 50%, ou se sobrar qualquer global — três modos,
para que o verde signifique algo.

**Lição**: *uma métrica que só sobe é fácil de julgar; uma que oscila precisa de
uma regra de leitura decidida ANTES de olhar.* Terços e árvore de processos foram
escolhidos antes da primeira amostra. Escolhidos depois, seriam a mesma coisa que
escolher a conclusão.

---

## H186 — CPU: ociosa custa 6% de um núcleo, e a carga é limitada pela conexão, não pelo processador

**Data**: 2026-08-22
**Contexto**: Fase 2, item "limites de CPU". Último item mensurável do enunciado.

**Onde**: `internal/wa-headless/probe_cpu_test.go` (novo).

**A suspeita por trás do item**, que é o que o tornava vale a pena: toda
capacidade usa estacionar-e-consultar — a página estaciona a resposta e o Go
CONSULTA, porque `Evaluate` não aguarda promessa (invariante 6). Consultar é um
laço, e muitas chamadas concorrentes são muitos laços. Se o custo por chamada
crescesse com a concorrência, apareceria aqui.

**Medição**, duas janelas de 30 s do mesmo tamanho:

```
ocioso   1.69s de CPU em 30.2s de relógio    (6% de um núcleo)
carga   52.75s de CPU em 30.1s de relógio   (176% de um núcleo)
razão   31.3x
```

**O número que decide capacidade é o OCIOSO.** Uma sessão parada custa 6% de um
núcleo — cerca de dezesseis sessões ociosas por núcleo. É ele que multiplica por
sessão numa máquina, não o pico; e por isso é ele que o teste afere, com teto em
25%.

**E o pico é SUB-LINEAR, o que é a informação interessante.** Doze trabalhadores
em laço contínuo produziram 1,76 núcleo, não doze. O gargalo não é o processador:
é a **conexão CDP única**, que serializa. Isso explica também a latência da carga
da H183 (p95 de 233 ms com 40 trabalhadores) — a fila é da conexão. Quem quiser
mais vazão por máquina precisa de mais SESSÕES, não de mais concorrência dentro
de uma.

**Duas decisões de instrumento:**

1. **CPU como DELTA de tempo acumulado, não `%cpu`.** A coluna `%cpu` do `ps` é
   média desde que o processo NASCEU; numa sessão de minutos ela achataria
   exatamente a rajada que este teste quer ver.
2. **A janela ociosa tem o mesmo tamanho da janela de carga.** Comparar 5 s de
   ocioso com 30 s de carga compararia janelas, não estados.

**Uma ressalva dita, não escondida**: a carga aqui é SATURANTE — doze
trabalhadores em laço sem pausa. É o teto, não um perfil de produção. O número
serve para dimensionar o pior caso, e chamá-lo de "uso típico" seria mentir sobre
o que foi medido.

**Status**: item medido, nenhum defeito.

**Lição**: *quando um número cresce menos do que devia, a explicação é tão útil
quanto o número.* 1,76 núcleo para doze trabalhadores parece bom até se perguntar
por quê — e a resposta (uma conexão serializa) muda a recomendação de capacidade
de "adicione concorrência" para "adicione sessões".

## F101 — `ParseJID("")` faz panic, e a lista de participantes chega até ele sem validação de elemento

**Data**: 2026-08-22.
**Contexto**: achado incidental na Fase 3 do `internal/wa-headless` (decisão 72),
ao medir o comportamento do `JIDResolver` do `wa-noise` antes de extrair a regra
pura para um pacote compartilhado. Não faz parte do escopo da Fase 3.

**Onde**: `pkg/infra/wa-noise/mapping/jid/parse.go:12-13`

```go
func ParseJID(arg string) (types.JID, bool) {
	if arg[0] == '+' {      // <-- index out of range quando arg == ""
```

**Problema**: `arg[0]` numa string vazia entra em pânico. O porto
`appport.JIDResolver` expõe isso: `JIDResolverAdapter.ResolveJID(ctx, "")`
morre em vez de devolver erro — apesar de a assinatura prometer `(domain.JID, error)`.

Evidência MEDIDA (sonda descartável, removida após a medição):

```
panic: runtime error: index out of range [0] with length 0
wa-api/pkg/infra/wa-noise/mapping/jid.ParseJID(...)
	pkg/infra/wa-noise/mapping/jid/parse.go:13
wa-api/pkg/infra/wa-noise/mapping/jid.JIDResolverAdapter.ResolveJID(...)
	pkg/infra/wa-noise/mapping/jid/resolver.go:22
```

Alcançabilidade — os 11 chamadores de `ResolveJID` foram enumerados um a um:

| chamador | protegido por |
| --- | --- |
| `get_avatar.go:35` | `len(req.Phone) < 1` |
| `get_group_info.go:36`, `get_group_invite_link.go:36` | validação de campo |
| `subscribe_presence.go:34`, `chat_presence.go:38`, `react.go:40` | validação de campo |
| `mark_read.go:35,49` | `len(...) > 0` / `!= ""` |
| `react.go:65` | `req.Participant != ""` |
| `get_user_profile.go:85` | **nada no use case** — protegido pela FORMA da rota (`/user/profile/{jid}`: segmento vazio não casa) |
| `group_management.go:37` (via `parseJIDs`, linha 48) | **nada** |

O caminho aberto, LIDO no código (não medido ponta a ponta):
`handler_group_mgmt.go:95` valida `len(req.Participants) < 1` — o TAMANHO da
lista, não os ELEMENTOS. Então `POST /group/create` com
`{"name":"x","participants":[""]}` desce por `CreateGroup → parseJIDs →
parseJID → ResolveJID → ParseJID("")` e entra em pânico. O middleware de
`recover` (`pkg/bootstrap/router.go:149,184`) transforma isso em 500, então o
processo não morre — mas o erro é 500 onde deveria ser 400, e a rota
`/group/updateparticipants` tem a mesma forma de validação.

**Correção sugerida**: guardar na origem da regra, não em cada chamador —
`if arg == "" { return types.JID{}, false }` no topo de `ParseJID`. Isso já
transforma o panic no erro que o porto promete. Separadamente, validar
elemento vazio em `parseJIDs` com o índice (a mensagem de log ali já expõe
`index`, então a informação existe) para que a resposta seja 400 com a posição.

**Anti-regressão exigida** (política do projeto): teste que chama
`ResolveJID(ctx, "")` e exige `error` (hoje ele entra em pânico — o controle
negativo é remover a guarda e ver o panic voltar), mais teste pela ROTA
registrada `/group/create` com `participants:[""]` exigindo 400.

**Status**: **CORRIGIDO** nesta sessão, por decisão 73 da orquestração
("corrija o F101 antes da extração, com erro explícito para JID vazio e teste
da rota exigindo 400, depois mova a regra já saneada"). A correção precede a
extração da decisão 72 justamente para não dar duas casas ao mesmo defeito.

Três guardas, e a primeira é a que trava a CAUSA:

1. `pkg/infra/wa-noise/mapping/jid/parse.go` — `if arg == "" { return types.JID{}, false }`
   no topo de `ParseJID`, na ORIGEM da regra e não nos onze chamadores.
2. `handler_group_mgmt.go` — `firstEmpty()` + `rejectEmptyElement()` nas duas
   rotas com lista (`/group/create`, `/group/updateparticipants`), com o ÍNDICE
   da entrada reprovada, para que a resposta seja 400 e diga qual falhou.
3. `handler_group_mgmt.go` — `req.GroupJID == ""` em
   `handleUpdateGroupParticipants`, que nunca validou esse campo.

**Testes que travam o achado**:

- `pkg/infra/wa-noise/mapping/jid/parse_empty_test.go` — a CAUSA, pelo
  adaptador REAL: `TestParseJIDRejectsEmptyInsteadOfPanicking`,
  `TestResolveJIDOnEmptyReturnsErrorNotPanic` e
  `TestResolveJIDStillAcceptsBareNumber` (o caminho de SUCESSO, para que a
  guarda não atropele o número nu que `/user/profile` promete aceitar).
- `pkg/presentation/http/handlers/handler_group_mgmt_test.go` — o SINTOMA,
  pela rota, em 4 casos novos na tabela `missing`. Um deles põe o vazio na
  SEGUNDA posição (`"empty participants at index 1"`), porque um índice
  constante zero passaria no caso trivial.

**Controles negativos EXECUTADOS** (três, um por guarda):

```
CONTROLE 1 — guarda removida do ParseJID
--- FAIL: TestParseJIDRejectsEmptyInsteadOfPanicking (0.00s)
panic: runtime error: index out of range [0] with length 0 [recovered, repanicked]
	pkg/infra/wa-noise/mapping/jid/parse.go:16

CONTROLE 2 — guardas do handler removidas
--- FAIL: .../CreateGroup/participante_vazio: status: got 200, want 400
--- FAIL: .../CreateGroup/participante_vazio_no_meio: status: got 200, want 400
--- FAIL: .../UpdateGroupParticipants/sem_GroupJID: status: got 200, want 400

CONTROLE 3 — guarda de elemento de Phone removida
--- FAIL: .../UpdateGroupParticipants/phone_vazio: status: got 200, want 400
```

**O que o controle 2 revelou, e vale mais que o próprio teste**: sem a guarda o
handler responde **200**, não 500. O `contractsfake.JIDResolver` aceita a
string vazia de bom grado, então o teste de fronteira NUNCA veria o panic —
é a armadilha nº 1 do `ARMADILHAS.md` (dublê mais permissivo que a produção)
acontecendo outra vez, e é a prova de que o teste no porto real não era
redundante com o teste de rota. Os dois travam coisas diferentes: remover a
guarda de `ParseJID` deixa a suíte de rota inteiramente verde.

**Relação com a decisão 72**: este defeito é exatamente o que a extração da
regra pura para um pacote compartilhado propagaria em silêncio para o segundo
adaptador. Corrigido ANTES da extração, o que se move é a regra já saneada —
que é a ordem que a decisão 73 impôs.

## F102 — a Fase 3 precisa de N browsers, e o launcher exige uma porta que ele não precisaria exigir

**Data**: 2026-08-22.
**Contexto**: Fase 3 (decisão 71), ao projetar o registry que mapeia `txtID` →
sessão headless. Não é defeito em produção: nada em produção fia o
`wa-headless` ainda — é exatamente isso que a Fase 3 vai fazer. Fica
registrado porque a fiação passaria por cima do problema sem vê-lo.

**Onde**: `internal/wa-headless/engine/flags.go:133-136` e
`internal/wa-headless/engine/launcher.go:113-140`

**Problema, em duas metades.**

*Metade 1 — a porta é obrigatória sem precisar ser.*

```go
if cfg.DebuggingPort <= 0 {
    return nil, fmt.Errorf("launch: DebuggingPort is required; without it there is "+
        "no CDP endpoint and no way to stop the browser cleanly (%s)", flagRemotePort)
}
```

A justificativa está errada, e isso foi MEDIDO, não deduzido. Com
`--remote-debugging-port=0` o Chromium sobe, escolhe porta efêmera e escreve
`<ProfileDir>/DevToolsActivePort` com DUAS linhas: a porta real e o caminho ws
do browser.

Medido em 2026-08-22, Google Chrome 151.0.7922.170, macOS 15.6, com perfil
temporário descartado depois (nunca o perfil pareado):

```
=== DevToolsActivePort existe? ===
SIM. conteudo:
55077
/devtools/browser/954157ae-38a1-4ef0-b7d5-f03f3c1f7d03
```

Ou seja: existe endpoint CDP sem o chamador escolher porta nenhuma.

*Metade 2 — e por isso o `awaitEndpoint` confia em quem atender.*

```go
url := fmt.Sprintf("http://%s:%d%s", localEndpointHost, port, versionEndpoint)
```

Ele faz GET em `127.0.0.1:PORT/json/version` e aceita QUALQUER browser que
responda. Não há verificação de identidade — nada amarra o endpoint ao processo
que acabamos de lançar nem ao `ProfileDir` que pedimos.

Hoje isso é inofensivo porque só um browser sobe por vez e a porta vem de
`freePort(t)` nos testes. Com N sessões, deixa de ser: `freePort` é
bind-`:0`-e-fecha (`integration_test.go:60-70`), TOCTOU clássico, e duas
sessões arrancando ao mesmo tempo podem receber a mesma porta. O modo de falha
não é "falha ao subir" — é a **sessão B anexar-se ao browser da sessão A**,
dirigindo a conta errada em silêncio. Num stack cujo escopo são duas contas de
WhatsApp distintas, isso é troca de conta, não erro de infraestrutura.

**Correção sugerida**: parar de escolher porta. Lançar com
`--remote-debugging-port=0` e ler `<ProfileDir>/DevToolsActivePort` para
descobrir a porta e o caminho ws. Isso resolve as duas metades de uma vez: não
há porta para colidir, e o endpoint fica amarrado ao NOSSO `ProfileDir`, que é
justamente a identidade que falta hoje. `DebuggingPort` continua útil como
override explícito, mas deixa de ser obrigatório.

É também o que a referência faz: puppeteer (e portanto o whatsapp-web.js) lê o
`DevToolsActivePort` do user-data-dir em vez de confiar na porta pedida. O
ENTENDIMENTO veio de lá; o comportamento foi medido aqui, porque este projeto
já teve quatro nomes de módulo do wwebjs que não existiam no nosso build.

**Anti-regressão exigida**: teste que sobe com porta 0 e prova que o endpoint
sai do arquivo, mais teste da metade 2 — dois perfis, e o segundo NÃO pode
terminar apontando para o ws do primeiro. O controle negativo é reintroduzir a
leitura por porta fixa e ver o segundo anexar-se ao primeiro.

**Status**: **CORRIGIDO** nesta sessão, por decisão 75 ("troque agora para
porta 0 mais DevToolsActivePort, mantendo DebuggingPort apenas como override
explícito e validando que o endpoint pertence ao ProfileDir lançado").

O que mudou:

1. `flags.go` — `DebuggingPort == 0` deixa de ser erro e passa a ser o caso
   NORMAL; positivo continua override explícito; negativo continua erro, com
   mensagem que diz as três coisas.
2. `launcher.go` — `awaitEndpoint` passa a ter DOIS caminhos, e qual existe
   não é preferência nossa: é decidido pelo Chromium, e foi medido.

   **CORREÇÃO DE UMA MEDIÇÃO MINHA INCOMPLETA.** A primeira medição usou porta
   0 e concluiu "o Chromium publica o arquivo". Ao rodar a suíte, oito testes
   de integração falharam — eles fixam porta. Fui medir o caso fixado, com o
   conjunto canônico completo de flags:

   | `--remote-debugging-port` | escreve `DevToolsActivePort`? | responde HTTP? |
   | --- | --- | --- |
   | `0` | **sim** | sim |
   | fixada | **NÃO** | sim |

   Com porta explícita o Chromium **não escreve o arquivo de todo**. Logo o
   override só pode ser servido por HTTP, e o caminho do perfil só existe no
   caso efêmero. A assimetria é a razão de preferir porta 0 — e é a razão de
   o registry preferir. Fixar porta passa a ser o chamador assumindo o risco
   explicitamente, que é o que um override deve ser.

   Descartei junto o teste que eu havia escrito para "porta fixada discorda do
   arquivo": é um estado que o Chrome real nunca produz, e travá-lo seria a
   armadilha nº 1 do `ARMADILHAS.md` — dublê que inventa uma forma que a
   produção não tem.
3. `launcher.go` — o arquivo é APAGADO antes do arranque. Sem isso, um
   `DevToolsActivePort` deixado por um browser que morreu reproduziria o
   próprio defeito que a mudança remove.
4. A discordância entre porta fixada e porta publicada vira ERRO, e não uma
   preferência silenciosa por uma das duas.
5. O caminho HTTP morto (`readEndpoint`, `versionEndpoint`, o campo
   `HTTPClient` e o dublê `devToolsServer`) foi removido: deixá-lo daria a
   impressão de que o launcher ainda fala HTTP.

**Testes que travam o achado** (`engine/launcher_test.go`, `engine/flags_test.go`):

- `TestLaunchIgnoresAStaleEndpointFileFromAPreviousRun` — o caso de identidade.
- `TestLaunchUsesTheHTTPEndpointWhenThePortIsPinned` — o caminho de override,
  que também exige que NADA seja escrito no perfil, porque o Chromium real não
  escreve.
- `TestLaunchWaitsThroughAHalfWrittenEndpointFile` — substitui o antigo teste
  do "ws vazio" pela corrida REAL: o leitor chega no meio da escrita.
- `TestBuildFlagsAcceptsAnEphemeralPort`, `TestBuildFlagsKeepsAPinnedPort`,
  `TestBuildFlagsRefusesANegativePort` — substituem
  `TestBuildFlagsRequiresADebuggingPort`, que travava a regra ANTIGA.

O dublê passou a PUBLICAR o endpoint no perfil, no formato medido contra o
Chrome real (duas linhas, porta e caminho ws). Um dublê que publicasse outra
forma provaria só que o parser lê o que o dublê escreve.

**Controles negativos EXECUTADOS** (dois, depois de a medição corrigida descartar o terceiro):

```
CONTROLE 1 — não apaga o arquivo obsoleto
WebSocketURL = "ws://127.0.0.1:40001/devtools/browser/STALE",
          want "ws://127.0.0.1:55080/devtools/browser/fresh" — the stale file was believed

CONTROLE 3 — aceita arquivo pela metade
WebSocketURL = "ws://127.0.0.1:55079/devtools/browser/guessed",
          want "ws://127.0.0.1:55079/devtools/browser/late" — the half-written file was accepted
```

O controle 1 não é uma asserção sobre a correção: ele **demonstra a falha**. O
launcher, sem a limpeza, conecta-se ao endpoint de outra execução — que é o
"dirigir o browser errado" descrito acima, acontecendo num teste.

## F103 — a suíte vaza browsers, e o vazamento se auto-amplifica até derrubar o gate

**Data**: 2026-08-22.
**Contexto**: achado ao investigar uma falha do `make check` na decisão 77. Não
é da decisão 77, e provavelmente não é de hoje.

**Sintoma medido**: o gate falhou em

```
--- FAIL: TestHolder_ConcurrentFirstUseBootsExactlyOneSession (181.67s)
    core: boot failed at open_tab (stopped_via=DIRTY_signal_close_refused pid=69278):
    core: opening tab: Boot(open_tab/prime): deadline of 2m30s exceeded
```

O MESMO teste, isolado, passa em **2,4 s**. Fator de 75×, o que não é ruído.

**Causa medida**: havia **28 processos Chrome órfãos** vivos na máquina, todos
com perfil temporário de teste:

```
--user-data-dir=/var/folders/…/T/TestHolder_ConcurrentFirstUseBootsExactlyOneSession340446440/001
--user-data-dir=/var/folders/…/T/TestLifecycle_ObserverMayCallBackIntoTheSession3814543593/001
```

**41 deles com mais de uma hora de idade, e dois com mais de um dia** — ou seja,
sobreviventes de execuções anteriores, anteriores às mudanças desta sessão.
`load average` estava em 9,25; após terminá-los, caiu para 3,88.

**O mecanismo, e ele é o que torna isto grave.** Os testes FAZEM
`defer h.Stop(...)` — não é esquecimento. O vazamento acontece no caminho de
FALHA: quando o arranque estoura o prazo, o `CleanStop` sinaliza, o browser
recusa fechar (`DIRTY_signal_close_refused`) e o processo sobrevive à execução.

E daí em diante o defeito se **auto-amplifica**: cada browser vazado sobe a
carga da máquina, o que torna o próximo arranque mais lento, o que torna o
próximo estouro mais provável, o que vaza mais um. O gate fica
progressivamente mais frágil a cada execução, sem que nada no repositório mude.

**A CAUSA RAIZ, medida depois e mais grave que o vazamento.** Ao repetir o gate
numa máquina limpa, medi durante a execução:

```
browsers de teste vivos ao mesmo tempo: 24
binários de teste em execução:           4
CPUs da máquina:                        10
load average:                        59,58
```

O `go test` paraleliza PACOTES até ao número de CPUs, e vários pacotes desta
árvore lançam browsers. **Ninguém coordena isso.** Vinte e quatro Chromes em dez
CPUs é saturação, e um arranque de SPA sob saturação estoura o prazo de 2m30.

Isto inverte o diagnóstico da primeira metade desta entrada: o vazamento não é a
causa da saturação, é **consequência** dela. A cadeia é:

1. o gate lança pacotes de browser em paralelo, sem teto;
2. a máquina satura;
3. um arranque estoura o prazo;
4. o `CleanStop` sinaliza, o browser recusa fechar, e o processo VAZA;
5. o vazado soma-se à carga da próxima execução — e aí sim, auto-amplifica.

> **CORREÇÃO 2026-08-22, e o erro de diagnóstico é meu (decisões 84 e 85).**
>
> Depois da serialização da 79 eu voltei a encontrar "dez órfãos com mais de uma
> hora" e concluí que o vazamento continuava. **Estava errado.** Fui verificar o
> processo em vez de confiar na contagem, e ele era `Google Chrome for Testing`
> de `~/.agent-browser/browsers/`, com `--user-data-dir=…/T/agent-browser-chrome-<uuid>`
> — o browser da FERRAMENTA MCP do agente, cujo servidor se desconectou a meio da
> sessão. Os dez eram UMA instância dela mais os nove auxiliares.
>
> Como o erro aconteceu: um comando contou tudo com `user-data-dir` sob
> `/var/folders`, que apanha as duas coisas; outro procurou `T/Test` e achou um
> `TestHolder_…` que era um browser LEGÍTIMO em voo, da fase serial do gate a
> correr naquele instante. Juntei os dois resultados e li como um conjunto só.
>
> Medido com o recorte certo: **zero** browsers de teste do wa-api sobreviventes.
>
> O que fica de pé: o achado ORIGINAL desta entrada continua válido — os 28
> órfãos daquela ocasião tinham caminhos `T/TestHolder_` e `T/TestLifecycle_`
> explícitos. O que NÃO está provado é que o vazamento persista depois da 79.
>
> Por isso a decisão 85 recusou a escalada do `CleanStop`: **sem reproduzir
> vazamento real, não se mexe no caminho de desligamento**. Ficou só a varredura.

**Relação com a F100** (o gate é sensível à CARGA da máquina, quatro falsas
falhas num dia): é quase certamente a MESMA causa. A F100 atribuiu as falhas a
carga externa; o gate produz a sua própria carga, e depois deixa parte dela para
trás. Vale reabrir a F100 com esta medição antes de continuar a tratar as falhas
como ambientais.

**Correção sugerida para a causa raiz**: os pacotes que lançam browser precisam
de um teto GLOBAL, e não por pacote — `-p 1` para eles, ou um semáforo entre
processos (trava de arquivo), porque o paralelismo do `go test` é entre
processos e um semáforo em memória não o vê. Escolher o número exige medir, e a
medição da decisão 76 já dá o ponto de partida: ~650 MB por browser, e o
arranque só degrada quando a máquina satura.

**Correção sugerida**, em duas partes:

1. **Varredura antes do gate**: terminar processos Chrome cujo `--user-data-dir`
   esteja sob `/var/folders/*/T/Test*`. O recorte é essencial — perfil de teste
   é descartável, e `SIGKILL` contra um perfil PAREADO arrisca corrompê-lo, o
   que exigiria um humano com o telefone para reparear.
2. **Escalada no `CleanStop` para browser de teste**: um `DIRTY_signal_close_refused`
   hoje termina em desistência. Para perfil temporário, desistir é deixar lixo;
   escalar para `SIGKILL` no grupo de processos é seguro e é o que o próprio
   `reapBrowser` dos testes do `engine` já faz.

**Anti-regressão exigida**: teste que, após um arranque que estoure o prazo,
exija que nenhum processo com aquele `ProfileDir` sobreviva. O controle negativo
é remover a escalada e ver o processo sobreviver.

**Dado que separa "sabemos" de "achamos"**: repetido o gate com a máquina limpa
e SEM nenhuma outra mudança, ele passou (`exit=0`). Ou seja, a limpeza sozinha
recupera — mas a carga voltou a 59,58 durante essa mesma execução, então passou
perto. O teto do paralelismo é prevenção justificada, não teoria: sem ele, a
aprovação depende de a máquina estar limpa naquele instante.

**CORRIGIDO EM PARTE, por decisão 79** ("ponha teto agora nos pacotes que lançam
browser, preferindo serialização explícita no gate, e corrija a F100 para esta
causa medida").

O que foi feito:

1. **Serialização explícita no gate.** `BROWSER_PKGS` nomeia os quatro pacotes
   que sobem Chrome, e eles correm num `go test` próprio com `-p 1`. O `-p 1`
   BASTA, e isso foi medido em vez de suposto: os quatro têm ZERO chamadas a
   `t.Parallel()`, logo os testes dentro de cada um já eram sequenciais e toda a
   concorrência era entre pacotes. A trava de arquivo que o paralelismo entre
   processos exigiria seria complexidade sem problema a resolver.

2. **Guarda contra o modo de falha silencioso** (`test-split-check`). O
   `$(filter)` do make descarta em SILÊNCIO o que não casa: um typo em
   `BROWSER_PKGS` devolveria o pacote ao grupo paralelo e a proteção sumiria sem
   aviso. A guarda falha se algum nome não casar, e também se a soma dos dois
   grupos não bater com `TEST_PKGS` — um pacote perdido na divisão não é
   testado. Dois controles negativos executados, ambos com mensagem que diz o
   que corrigir.

3. **A SEGUNDA causa, que só apareceu depois de serializar.** Com a carga já em
   9,63, um teste do `core` ainda estourou 2m30. Não era saturação: o helper
   `freePort` é bind-`:0`-e-fecha, TOCTOU, e a porta reservada foi tomada antes
   de o Chromium se ligar a ela — a colisão ficou visível no log como um
   `httptest.Server` ainda a segurá-la. **A serialização não escondeu esse
   defeito: expô-lo.**

   Os 185 sítios passaram a usar `ephemeralPort`, que devolve 0. O nome antigo
   mentia — `freePort` devolvia uma porta que podia não estar livre no instante
   que importava. Porta 0 não tem alocação para disputar, e é o caminho que a
   produção usa (decisão 75), então a suíte passou a exercitar o MESMO caminho.

**Resultado medido**, mesma máquina, antes e depois:

| | browsers simultâneos | pico de carga | órfãos após | `runtime` |
| --- | --- | --- | --- | --- |
| antes | 24 | 59,58 | 28 | 220 s (com falha) |
| depois | 8 | 9,63 | **0** | **30,8 s** |

E a serialização NÃO custou tempo: o pacote raiz manteve os mesmos ~100 s. O
gargalo nunca foi paralelismo útil — era contenção.

**F100 atualizada** com esta causa, como a decisão 79 pediu.

**Varredura acrescentada (decisão 84/85)**: `scripts/orphan-browser-check.sh`,
ligada ao alvo `test` do Makefile. Ela **FALHA** em vez de limpar, e isso é a
decisão: limpar em silêncio esconderia o vazamento — que foi exatamente o erro
cometido ao ler a serialização da 79 como se tivesse removido o problema.

O recorte é estrito: só `--user-data-dir` sob `/var/folders/*/T/Test*`. Por
construção não apanha o browser do agent-browser nem um perfil PAREADO — e o
segundo importa, porque `SIGKILL` contra perfil pareado arrisca corrompê-lo e
reparear exige um humano com o telefone.

Controle negativo EXECUTADO: com um órfão de teste vivo, `make orphan-browser-check`
sai com **2**; depois de terminado, **0**. A primeira medição do controle deu
`exit=0` enganosamente porque eu tinha canalizado a saída por `head` — a
armadilha "gate dentro de pipe não é gate", que também está catalogada.

**Status**: o que continua PENDENTE é o item 2 da correção sugerida original —
a escalada no `CleanStop` para browser de perfil temporário, agora RECUSADA por
falta de evidência (decisão 85) e não por falta de tempo. Com as duas causas
removidas os estouros pararam, e sem estouro não há vazamento; mas a defesa em
profundidade continua a faltar, e um estouro por outra razão voltaria a deixar
lixo. Não foi feito aqui porque mexer no `CleanStop` é mexer no caminho de
desligamento, que tem invariante própria (invariante 3), e mexer no
`CleanStop` é mexer no caminho de desligamento, que tem invariantes próprias
(invariante 3: nada de parada por sinal vinda de fora do caminho de shutdown).
Os 28 órfãos foram terminados nesta sessão para desbloquear o gate, e isso está
registrado aqui para que a limpeza não seja confundida com correção.

## H141 — número escrito à mão ao lado de número medido deriva

**Data**: 2026-08-23. **Contexto**: fase 3, ao fechar o port `GroupRequests`.

**Onde**: `internal/wa-headless/ESTADO.md` (seções de `GroupDirectory` e
`GroupLifecycle`) e as mensagens dos commits `a7d29fb` e `7294701`, contra
`pkg/infra/wa-headless/phase3_inventory_test.go:45-51`.

**Problema**: a prosa vinha **+1** sobre a medição, por pelo menos dois commits.

```
$ git show 7294701:pkg/.../phase3_inventory_test.go | grep -c 'satisfeito: true'
14          # a prosa do mesmo commit diz "15 de 22"
$ git show a7d29fb:pkg/.../phase3_inventory_test.go | grep -c 'satisfeito: true'
13          # a prosa do mesmo commit diz "14 de 22"
```

O teste `TestOTotalDePortsEOMedidoENaoOAnunciado` foi escrito exatamente para o
número ser lido do código e não anunciado — e o anúncio voltou a existir do
lado dele, em texto. **Duas fontes de verdade para o mesmo número derivam;** a
única pergunta é quando.

**Correção sugerida**: não repetir em prosa nenhum número que um teste imprime.
Onde o texto precisar do valor, citar o teste que o produz e o comando que o lê
(`go test ./pkg/infra/wa-headless/ -run Total -v`), em vez do dígito. Aplicável
a qualquer contagem futura (ports, capabilities, itens do LEDGER).

**Status**: corrigido nesta sessão para o valor corrente (15/5/2, soma 22), com
a correção registrada na seção do `GroupRequests` do ESTADO.md em vez de
reescrita silenciosa das seções antigas — commit passado não se reescreve, e
apagar o erro apagaria a evidência de que a duplicação deriva.

**Sem teste que o trave**, e dito em voz alta: um gate que proibisse dígitos em
prosa daria falso positivo em toda citação legítima de medição. O que trava
metade disto já existe — o teste de inventário falha se a tabela ficar atrás do
código. O que não está travado é a prosa, e a mitigação é a regra acima.

## H142 — o teste do logcov está a 86% do timeout, e falharia como travamento

**Data**: 2026-08-23. **Contexto**: fase 3, ao diagnosticar um gate lento.

**Onde**: `cmd/logcov` sob `make test`, que corre com `-race -timeout=20m`
(`Makefile:137` e o alvo paralelo).

**Problema**: a duração do pacote sob `-race` cresceu muito entre execuções da
mesma sessão, medida nos logs do gate:

```
/tmp/check98.log :  ok  wa-api/cmd/logcov   357.053s
/tmp/check102.log:  ok  wa-api/cmd/logcov  1037.425s   <- 86% do timeout de 1200s
/tmp/check103.log:  FAIL wa-api/cmd/logcov  533.335s   (falhou por golden, nao por tempo)
```

A dispersão é grande e depende de carga (a máquina esteve com load 12–20). O
pacote analisa a árvore inteira, então cresce com o repositório: cada capability
ligada acrescenta pacotes ao universo.

**Por que importa mais do que parece**: se ele estourar, o modo de falha é
`panic: test timed out`, que se lê como travamento e manda quem investiga
procurar deadlock — não teste lento. Foi exatamente essa a leitura errada que
custou tempo nesta sessão quando o gate parou de imprimir por sete minutos: a
suspeita imediata foi processo morto, e a resposta era `logcov.test` a 218% de
CPU, trabalhando.

**Correção sugerida**: dar timeout próprio ao pacote (`-timeout` maior só para
ele, via um alvo separado), ou medir e reduzir o custo de `Analyze` — hoje ele
recarrega e reanalisa tudo por teste que chame `measure`. A segunda é a boa; a
primeira compra tempo.

**Status**: não corrigido — está fora do escopo da fatia (o gate reprovou por
golden, não por tempo) e mexer no timeout durante uma correção de métrica
misturaria duas mudanças no mesmo diff. Registado para decisão.

## H143 — `pkg/bootstrap/config.go` está fora do gofmt

**Data**: 2026-08-23. **Contexto**: ao formatar arquivos novos da decisão 94,
`gofmt -l pkg/bootstrap/` acusou um arquivo que eu não tinha tocado.

**Onde**: `pkg/bootstrap/config.go:39-40`.

**Problema**: um comentário inserido no meio do bloco `var` quebrou o grupo de
alinhamento, e o gofmt quer reencostar as duas declarações acima dele:

```
-	webhookRetryEnabled      = flag.Bool("webhookretry", true, ...)
-	webhookRetryCount        = flag.Int("retrycount", 5, ...)
+	webhookRetryEnabled = flag.Bool("webhookretry", true, ...)
+	webhookRetryCount   = flag.Int("retrycount", 5, ...)
```

`git status` confirma o arquivo intocado nesta sessão, então é anterior.

**Por que passou**: o `make check` roda `lint` como INFORMATIVO, e nenhum alvo
roda `gofmt -l` como trava. Um repositório que passasse a exigir formatação
falharia no primeiro dia por um arquivo que ninguém mexeu.

**Correção sugerida**: `gofmt -w pkg/bootstrap/config.go`, e — a parte que vale
mais — acrescentar `gofmt -l` ao gate, falhando se a saída não for vazia. Sem
isso o próximo desalinhamento entra do mesmo jeito.

**Status**: NÃO corrigido. É cosmético e fora do escopo da fatia, e a regra
deste repositório é registrar em vez de consertar de graça. Pergunta ao usuário
em aberto: conserto agora junto com a trava, ou fica pendente?
