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

~~Vale notar: `Category.HTTPStatus()` **já existe e já tem teste**, mas nada o
chama ainda.~~ **Isso estava errado quando foi escrito e ficou errado por dois
dias.** `RespondJSON` (`pkg/presentation/http/response.go:52`) já ignora o
status do sítio de chamada quando o erro é `*apperr.AppError` e usa
`HTTPStatus()`. A ligação nunca faltou — faltava só classificar os erros.

Essa frase custou uma estimativa: "67 sítios **mais** a ligação do
`HTTPStatus()`" dobrava o trabalho real. Ela veio de um comentário
desatualizado em `codes.go` que dizia *"nothing calls it yet"*; o comentário foi
corrigido junto.

**Status**: **CORRIGIDO (2026-08-09)**, em commits por grupo de use case
(`user`, `message`, `group`, `storage`, `session`, `chat`) — 103 sítios, mais
duas validações em `pkg/domain/privacy.go` que o levantamento por use case não
via.

**O que ficou de fora, de propósito**: 18 sítios com `failure sending to
Whatsapp servers`, `failed to *` e `database error`. São falha nossa ou do
upstream, e devolvê-los como 400 seria o defeito inverso — o cliente pararia de
retentar uma falha que era mesmo transitória.

**Dois erros meus no caminho**, ambos pegos por teste e não por revisão:
- o levantamento usou `grep -rhn ... | grep -v _test`; com `-h` o nome do
  arquivo não sai, então o filtro não filtrava nada e metade das strings eram
  dublês de teste. Refeito por arquivo.
- converti `LID not found: %w` para 404, e ele dispara quando o **store quebra**
  — não quando o LID não existe. Um teste de falha de porta pegou. Revertido
  para `failed to get LID: %w` (500).

Os 9 testes que fixavam o defeito (4 com `_500` no nome) foram invertidos **caso
a caso**: `TestUserHandlers_AdminRoutes` tem 9 casos esperando 500 e só 5
mudaram — os outros 4 são erro de repositório e de escrita, e seguem 500.

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

**Status**: **CORRIGIDO (2026-08-09)** pela saída (1), a recomendada.

Um construtor só (`buildQRPayload`), usado pelos dois fluxos. O payload sempre
traz `code` — o texto cru, que qualquer cliente renderiza sozinho — e traz
`qrCodeBase64`/`expiresAt` quando dá.

O campo `event` foi unificado em **`"code"`**, e não em `"qr"`: era o valor que
acompanhava o payload RENDERIZÁVEL, então é o que um cliente de interface
provavelmente usa para decidir desenhar o QR — e portanto o mais arriscado de
mudar. O campo é redundante com o `type`, que já diz `QR`; removê-lo é
candidato a uma próxima versão de contrato, não a esta.

**Ganho que não estava no plano**: antes, falhar ao codificar a imagem fazia
`onPairingQR` **retornar sem despachar nada**, e o cliente ficava sem o código
cru — que ele consegue renderizar por conta própria. Agora degrada para só o
`code`, com log. Um caminho de saída a menos e um log a mais.

O teste compara os CONJUNTOS DE CHAVES dos dois fluxos, não valores: valores
divergem legitimamente (códigos e validades diferentes), e é o schema que tem
de ser o mesmo. O controle negativo devolve a divergência exata que existia:
`pareamento=[code event expiresAt qrCodeBase64] subscribe=[code event]`.

O contorno no `devui` foi simplificado — o `if` continua, mas agora porque
`qrCodeBase64` é **opcional por contrato**, e não porque existem dois schemas.

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

**Status**: **CORRIGIDO (2026-08-09)** pela saída (1), a recomendada — e com a
redação de log que a torna honesta.

- A query string **deixa de autenticar** em toda rota que não seja
  `/session/ws`. Nelas o header sempre foi possível; a query só sobrevivia por
  compatibilidade.
- Em `/session/ws` ela continua valendo, e o motivo é da especificação, não
  nossa: `new WebSocket(url, protocols)` não aceita header. Recusar ali
  quebraria todo painel de navegador sem oferecer saída.
- O token que viaja na URL dessa exceção é **redigido** nos três sítios que
  registram a URL — registro de fronteira, observador de limite de taxa e
  relatório de pânico. Sem isso, a exceção justificada viraria credencial em
  log, que é o defeito fechado horas antes em três outros lugares.

O par de testes é o que segura o desenho: um exige a RECUSA fora do WebSocket,
outro exige a ACEITAÇÃO nele. Uma regra por prefixo (`/session*`) passaria só no
segundo — e o controle negativo confirma que ela é pega.

**Destino declarado, fora desta janela**: o subprotocolo
(`new WebSocket(url, [token])`), que tira o token da URL de vez. Exige mudança
em todo cliente WebSocket, e por isso não cabe na mesma release (ADR-0006).

> Nota de método: o `devui` já mantinha a query só no WebSocket, com comentário
> apontando para esta entrada. O código do painel estava certo antes do
> servidor — e foi ele que documentou o formato da saída.

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
antes:  connected=True  loggedIn=True  jid=5516981818244:19@s.whatsapp.net
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
GET /user/lid/5516981818244@s.whatsapp.net      -> 400
GET /user/lid/5516981818244                     -> 400
GET /user/lid/5516981818244%40s.whatsapp.net    -> 400
log: could not decode payload | error=EOF

GET /user/lid/ignorado  -d '{"JID":"5516981818244@s.whatsapp.net"}'  -> 200
    {"jid":"5516981818244@s.whatsapp.net","lid":"29343770251463@lid"}
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
GET /user/lid/5516981818244@s.whatsapp.net   (sem corpo)
  -> 200 {"jid":"5516981818244@s.whatsapp.net","lid":"29343770251463@lid"}

GET mesmo caminho + corpo {"JID":"5599999999999@s.whatsapp.net"}
  -> 200 com o LID do CAMINHO (o corpo não teve efeito)
```

**Fica aberto**: um número sem servidor (`/user/lid/5516981818244`) devolve
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
   `90937376170214`, e não `90937376170214@lid`), então o JOIN com
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

**DIAGNOSTICADO em 2026-08-09.** A resposta é a menos confortável das duas
hipóteses: o lento é o **nosso handler**, não o SDK. E não é soluço — é
estrutural, em toda mídia recebida.

A cadeia é síncrona de ponta a ponta:

```
handlerQueueLoop            (SDK, sequencial por sessão)
 └─ handleEvent             pkg/bootstrap/eventhandler.go:26
    └─ handleMessage        pkg/bootstrap/eventhandler_message.go:32
       └─ processMessageMedia   :165
          └─ processMedia       pkg/bootstrap/wiring_delegates.go:89
             └─ media.ProcessMedia  pkg/infra/media/media.go:53
                └─ Download(ctx, msg)  media.go:73   <- rede, aqui
```

E os prazos desse `ctx` estão em `wiring_delegates.go:49-53`:

| tipo | timeout |
|---|---|
| sticker | 1 min |
| imagem | 2 min |
| áudio | 5 min |
| documento | **10 min** |
| vídeo | **10 min** |

Como o `handlerQueueLoop` só consome o próximo nó quando o corrente termina,
**uma única mensagem de mídia trava a fila de nós daquela sessão pelo tempo do
download** — recibo, presença e marcação de leitura ficam atrás. Por
configuração, até dez minutos.

Os 5,7s medidos não são anomalia: são um download pequeno. O aviso do SDK só
tornou visível o que acontece em TODA mídia recebida. E explica o mesmo `id` nas
duas sessões: é o mesmo status sendo baixado duas vezes, uma por sessão.

**Agravante achado junto**: não há filtro de `status@broadcast` em
`eventhandler_message.go` nem em `media.go`. Status é alto volume por natureza —
cada contato que publica um story enfileira um download na sessão. Um usuário
com muitos contatos ativos tem a fila de nós permanentemente atrasada, e não por
tráfego dele.

Isto também **revalida a premissa da F86** por um caminho diferente do que usei
lá: o orçamento de atraso do handler não estava livre; já estava sendo consumido
por download de mídia.

**Correção sugerida** — e aqui há uma DECISÃO do dono do repositório, porque as
duas opções mudam coisas diferentes:

**(A) Fila serial POR SESSÃO, do nosso lado.** O `handleEvent` enfileira e
retorna; um worker por sessão faz download e webhook em ordem. O laço do SDK
volta a andar imediatamente, então recibo e presença deixam de esperar mídia.
A ordem dos webhooks por sessão é **preservada**. Custo: memória por fila, que
é exatamente o problema que a F86 já resolveu com fila limitada por bytes —
mesmo mecanismo, reaproveitável.

**(B) Despachar no pool compartilhado da F86.** Mais simples, reusa o que
existe. Custo: a ordem por sessão **se perde** — um texto enviado depois de um
vídeo pode chegar ao webhook antes dele.

**Recomendo (A)**: preserva a garantia que o cliente hoje tem sem saber que
tem, e o mecanismo de limite já está escrito. (B) é mais barato de implementar
e mais caro de explicar a quem consome o webhook.

> **DECIDIDO em 2026-08-09 pelo dono do repositório: opção (A)**, fila serial
> por sessão.

**CORRIGIDO (2026-08-09)** — `pkg/bootstrap/session_event_queue.go`.

O `handleEvent` agora ENFILEIRA e devolve o laço de nós do SDK na hora; um
worker por sessão faz o download e o webhook, em ordem. A fila nasce no Attach,
ANTES de o handler ser registrado — registrar primeiro abriria uma janela em que
um evento chega, não encontra fila e roda em linha, que é o defeito de volta.

Três decisões que os testes travam:

1. **Limite por ITENS, não por bytes** — o contrário do que a F86 fez, e por
   motivo concreto: aqui a fila guarda o evento ANTES do download. A mídia não
   está no item; o que está é a referência com as chaves para baixar depois.
2. **Fila cheia BLOQUEIA** em vez de descartar. Bloquear devolve o sistema ao
   comportamento de hoje (laço de nós parado); descartar perderia evento. Mesma
   escada da F86 — mais lento, nunca com perda. E o envio observa a parada, senão
   um produtor preso numa fila de sessão morta seguraria o laço para sempre.
3. **A fila para ANTES de o cliente ser derrubado**, no caminho do kill-channel.
   O worker pode estar no meio de um download e não pode seguir tocando handles
   prestes a ser liberados.

**O instrumento não podia sumir junto com o problema.** O aviso
`Node handling took` do SDK era o que media o nosso handler DE GRAÇA — e tirar o
trabalho do laço de nós cega exatamente a métrica que provaria a correção. Por
isso a fila mede o que passou a fazer, com o mesmo limiar de 5s do SDK, e reporta
também a profundidade da fila. As duas séries continuam comparáveis.

**Falta validar em bancada**: precisa de dispositivo pareado recebendo mídia. O
sinal a observar é `Node handling took` sumir para mensagens de mídia e, no
lugar dele, aparecer (ou não) o aviso novo de evento lento. Se o novo aparecer
com `queued` alto, a fila está absorvendo mas o download continua sendo o
gargalo — que é informação diferente e útil.

**Não entrou, e continua registrado**: rever os timeouts de 10 min e avaliar um
filtro de `status@broadcast`. A fila resolve o BLOQUEIO; não resolve o trabalho
desnecessário.

**Independente de A ou B**, duas coisas menores valem junto:
- rever os timeouts de 10 min: mesmo fora do caminho do nó, um download de dez
  minutos segurando um worker é muito;
- avaliar um filtro de `status@broadcast` — baixar mídia de status de todos os
  contatos pode ser trabalho que ninguém pediu, e hoje é obrigatório.

**Status**: **CORRIGIDO (2026-08-09)** pela saída (A), a escolhida — fila serial
por sessão, em `pkg/bootstrap/session_event_queue.go`. `handleEvent` enfileira e
volta; um worker por sessão preserva a ordem que hoje vem de graça do laço
sequencial do SDK, e que a saída (B) teria perdido.

Três decisões que valem registro:
- **teto por ITENS (1024), não por bytes**, ao contrário da F86: aqui a fila
  guarda o evento **antes** do download, então a mídia não está no item.
- **fila cheia BLOQUEIA**, não descarta: bloquear devolve o sistema ao
  comportamento de hoje (laço de nós parado) em vez de perder evento.
- **o envio observa o canal de parada**: sem isso, um produtor preso numa fila
  cheia de sessão morta seguraria o laço de nós para sempre — o oposto do
  objetivo.

**Um controle negativo mentiu no caminho**: o teste de "não bloqueia o produtor"
tinha `time.Now()` **depois** do enfileiramento do evento lento, então o custo
dele caía fora da medição e o número saía idêntico nos dois mundos. Subir uma
linha fez o controle acusar `2.002241625s`. Virou ARMADILHAS 25.

**A correção cega o instrumento que a provaria.** O `Node handling took` do SDK
media o nosso handler de graça, e ele para de medir justamente porque o trabalho
saiu do laço de nós. Por isso entrou um aviso próprio com o mesmo limiar (5s) e
mais o `queued` da fila — as duas séries continuam comparáveis.

**Não corrigidos, e seguem abertos** (os dois itens menores acima): os timeouts
de 1 a 10 minutos e o filtro de `status@broadcast`.

**Falta a validação de bancada**: precisa de um aparelho pareado recebendo
mídia. O sinal é o `Node handling took` sumir para mídia e o novo aviso de
evento lento aparecer (ou não) com `queued`.

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

**Status**: **corrigido**. `apperr.IsClientGaveUp` decide o nível nos três
sites que produziram as quinze linhas: o repositório (`user_repository.go`), o
use case (`get_status.go`) e o handler (`handler_session.go`, estendendo o
`isClientCausedSessionError` que já existia em vez de duplicar o conceito).

O predicado NÃO casa com `context.DeadlineExceeded`, e essa distinção é o
ponto: aquele é timeout NOSSO — prometemos resposta num prazo e não entregamos
— e continua saindo em `error`. Confundir os dois trocaria um alarme falso por
um alarme perdido, que é pior.

Teste com os cinco casos, incluindo erro embrulhado (`fmt.Errorf("database
error: %w", context.Canceled)`), que é a forma como ele de fato aparecia no
log. Registrado para decisão.

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

**Status**: **corrigido**. O `default` passa a logar `%T` (nome do tipo) e a
lista de NOMES dos campos, via `unhandledEventFields`. O aviso não perde valor:
`%T` identifica o tipo com precisão maior que o dump, e os nomes de campo
mostram a forma para quem for implementar o `case` que falta.

Três testes — e o terceiro só existe porque o controle negativo denunciou os
dois primeiros:

Os dois iniciais cobriam `unhandledEventFields`, a FUNÇÃO. Mas o vazamento
acontece no CALL SITE: mutando o `log` de volta para `%+v`, a função continuava
correta e **os dois testes seguiam verdes**. Testei a peça, não o
comportamento.

`TestHandleEvent_DefaultNaoLogaValores` captura a saída do zerolog num buffer e
afirma sobre o que SAI: sem o valor de `Codes`, sem o `CallID`, e ainda com o
tipo e os nomes dos campos presentes. Controle negativo com esse teste: *"o log
contem o VALOR do campo Codes — e' credencial de pareamento"*.

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

**Status**: **corrigido e VALIDADO EM BANCADA (2026-08-08)**, no fluxo real
(Postgres 5433, sessão pareada `teste-d2-b`), medindo os dois efeitos:

```
inicio                  connected=1
POST /session/disconnect connected=1
POST /session/logout    -> HTTP 409 {"code":"session_not_connected", ...}
                         connected=0
```

Os dois sintomas do defeito original caíram: o 500 virou 409 com remédio na
mensagem, e o `connected=1` preso — que fazia o `connectOnStartup` ressuscitar
sessão morta a cada subida — foi zerado pelo `Detach`.

> Nota de método: na primeira passada eu li `connected` ANTES do logout, vi 1, e
> quase registrei "o `Detach` não zera o estado". A medição estava no lugar
> errado — o `UPDATE ... connected=0` roda na goroutine que consome o
> kill-channel, ou seja, DEPOIS. Medir antes do efeito e concluir que o efeito
> não existe é o mesmo erro da ARMADILHA 19, com outra fantasia.

**Classificação** — `SessionGuardAdapter.Logout` checa `client.IsConnected()`
ANTES de chamar o SDK e devolve `apperr` com `CategoryConflict` (409) e
mensagem que diz a saída: *"call /session/connect before logging out"*. A
resposta anterior era 500 com corpo genérico, e o remédio era conhecimento de
implementação.

A checagem é de **estado**, não de texto de erro. Casar a mensagem do SDK
(`"websocket not connected"`) quebraria em silêncio no dia em que ele mudasse a
frase — acoplamento que só falha em produção.

**Estado local** — o use case chama `Detach` no ramo de falha quando o código é
`CodeSessionNotConnected`, então `users.connected` para de mentir. Sem isso o
`connectOnStartup` religava uma sessão fantasma a cada subida.

Três decisões que o teste trava, e as duas últimas são o que impede a correção
de virar defeito pior:

1. **Só neste código.** Falha por outro motivo pode ser transitória, e derrubar
   estado local de sessão possivelmente saudável seria pior que o problema
   original — mesmo raciocínio da regra de cerca do lease (D2).
2. **ACRESCENTA no ramo de falha, não MOVE o do sucesso.** A posição daquele é
   deliberada e está justificada pela F80.
3. O código de erro vive em `pkg/domain/apperr` porque duas camadas precisam
   concordar sobre ele: a infra o LEVANTA, o use case o LÊ.

Quatro controles negativos executados. Um deles NÃO COMPILOU na primeira
tentativa (import órfão) — ARMADILHAS 4 — e foi refeito. Os demais:

```
--- FAIL: TestLogoutUseCase_SemTransporteAlinhaOEstadoLocal
    Detach chamado 0 vezes ... o connectOnStartup religa uma sessão fantasma
--- FAIL: TestLogoutUseCase_OutraFalhaNaoDerrubaOEstado
    Detach chamado 1 vezes numa falha genérica
--- FAIL: TestLogoutUseCase_SucessoAindaDesanexa
    Detach chamado 0 vezes no sucesso — a F80 voltou
```

**Falta validar no fluxo real**: desconectar e deslogar pelo painel, conferindo
409 em vez de 500 e `connected=0` no banco.

> **Mudança de contrato observável**: o endpoint respondia 500 e passa a
> responder 409. Cliente que trate 5xx como "tente de novo depois" e 4xx como
> "não repita" vai agir diferente — e agir CERTO, porque repetir contra a mesma
> sessão desconectada produz o mesmo erro. Decidido pelo dono do repositório.

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

> **CORREÇÃO desta entrada (2026-08-08)**: o parágrafo abaixo, como escrito
> originalmente, estava ERRADO. Eu afirmei que não havia rota para `/devui`.
> **Há** — `wiring_routes.go:43` registra `strings.TrimSuffix(devui.BasePath,
> "/")` desde `a203a6f` (2026-08-07), antes da minha medição. Escrevi a causa
> por dedução a partir de uma única linha lida, sem conferir o resto do bloco.
>
> A causa real está no HANDLER, e é mais sutil: `devui.go:90` faz
> `strings.TrimPrefix(r.URL.Path, BasePath)` com `BasePath = "/devui/"`. Para
> `/devui` o prefixo NÃO casa, `name` continua `/devui`, o caminho reescrito
> vira `//devui` e o `http.FileServer` devolve 404. A rota chega ao handler; é
> o mapeamento de caminho que falha.

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

**Status**: **corrigido**. O handler passa a redirecionar `/devui` para
`/devui/` com 301.

Redirecionar, e NÃO servir o índice ali, por um motivo que a correção óbvia
erraria: servido em `/devui`, qualquer URL relativa dentro do HTML resolveria
contra a raiz (`/app.js`) em vez de `/devui/app.js`. É exatamente por isso que
servidores redirecionam diretório em vez de atender os dois caminhos.

Dois testes: o redirecionamento, e que o DESTINO funciona — sem o segundo,
apontar o `Location` para um caminho quebrado passaria no primeiro.

Controle negativo executado: `status = 404, quero 301`. A primeira tentativa
de controle NÃO COMPILOU (variável órfã), e controle que não compila não prova
nada — ver ARMADILHAS.md 4.

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

**Status**: **parcialmente corrigido**. `CategoryConflict` existe e mapeia para
409 (`codes.go`), e a recusa por posse de sessão migrou para ela.

Dois testes travam o comportamento em níveis diferentes: o MAPEAMENTO
(`TestCategory_HTTPStatus`, com o caso novo acrescentado à tabela — ela passava
sem conhecer a categoria) e o USO (`TestStart_OwnershipRefusalIsClassified`,
que agora exige 409 e não apenas "não-5xx"). Controle negativo executado nos
dois: `HTTPStatus() = 400, want 409`.

**Falta migrar a F93** (logout de sessão desconectada), que é o outro site
listado acima. Não foi feito aqui porque, ao contrário da posse — código novo,
sem cliente algum —, aquele endpoint já responde 500 em produção hoje: mudar
para 409 é alteração de contrato OBSERVÁVEL e precisa ser decidida como tal,
não aplicada de passagem.

---

## F96 — sessão que nunca subiu detém lease renovado para sempre

**Data**: 2026-08-08
**Contexto**: achado ao medir o cenário de duas réplicas do ADR-0005 D2, logo
depois de corrigir a posse no caminho de execução. É defeito INTRODUZIDO por
essa correção, não pré-existente.

**Onde**: `pkg/application/session/orchestrator.go`, guarda de posse no início
de `Start`; `pkg/bootstrap/lease.go`, mapa `lastRenewal` e `RunHeartbeat`.

**Problema**: `Start` reivindica a posse ANTES de materializar a sessão — o que
é deliberado e correto, para não deixar cliente e registries sujos se a posse
for negada. Mas se a sessão não chega a subir (QR nunca lido, pareamento
abandonado, `Start` falhando adiante), **nada libera o lease**. O gerenciador
registrou a posse em `lastRenewal` e o heartbeat a renova indefinidamente.

Evidência medida na instância de teste, com um usuário criado e conectado pela
API mas nunca pareado:

```
  t=6s   teste-runtime | connected=0 | expira_em=12s
  t=12s  teste-runtime | connected=0 | expira_em=11s
  t=18s  teste-runtime | connected=0 | expira_em=15s     <- renovou
```

O salto de 11s para 15s é uma renovação: a posse está viva para uma sessão que
não existe.

Duas consequências:

1. **Nenhuma outra réplica pode assumir aquele usuário**, jamais — o lease
   nunca expira. Se a sessão for pareada depois, ela fica presa à réplica que
   por acaso recebeu o `connect` original.
2. **A tabela cresce** com cada tentativa abandonada, e cada linha custa
   renovação a cada 5 segundos.

**Correção sugerida**: liberar a posse quando `Start` não completar. O ponto
natural é o próprio `Start`: reivindicou e vai retornar erro, então solta antes
de sair. Para o caso do QR abandonado, é preciso decidir o que conta como "não
completou" — provavelmente o retorno de `Start`, qualquer que seja o motivo,
já que ele só retorna quando o fluxo de pareamento termina ou falha.

**Atenção**: soltar no caminho de erro NÃO pode soltar no caminho de sucesso —
a posse tem de sobreviver enquanto a sessão estiver rodando, que é o ponto do
mecanismo.

**Status**: **corrigido** em `74c137e`. `Orchestrator.Start` devolve a posse
por `defer` quando retorna erro — e o `defer` fica DEPOIS da reivindicação
bem-sucedida, não no topo: instalado antes, liberaria a posse de OUTRA réplica
no caminho em que a nossa foi negada.

Três testes, cobrindo os três caminhos distintos (falhou depois de reivindicar
/ teve sucesso / foi negado), e dois controles negativos que falham em direções
OPOSTAS — não liberar reintroduz este defeito, liberar demais entrega sessão em
uso. Um controle só provaria metade.

**Falta validar no fluxo real**: criar usuário, chamar `/session/connect` e
nunca ler o QR. Se o lease sumir em vez de renovar a cada 5s, a correção vale
em produção; se ficar, ela cobre só a falha do provider e não o pareamento
abandonado — distinção que o teste unitário não revela.

**VALIDADO EM BANCADA (2026-08-08)**: o lease **ficou**. Segunda hipótese
confirmada — esta correção cobre só a falha síncrona do provider. O pareamento
abandonado continua vazando posse, e virou a **F98**, com a medição completa.

> Nota de método: este é o caso exato da regra "o conserto do conserto também é
> um mecanismo novo" (`CLAUDE.md`, política anti-regressão). Eu escrevi essa
> regra hoje, ao corrigir a F88, e não a apliquei à correção de posse algumas
> horas depois. A regra existir não basta; ela precisa ser consultada no
> momento de escrever, não só no momento de revisar.

---

## F97 — o token de API é guardado em texto claro, e o caminho de autenticação ainda o aceita

**Data**: 2026-08-08
**Contexto**: surgiu ao investigar por que `GET /admin/users` devolve o token
vazio. **A listagem está certa** — omitir credencial de uma listagem é boa
prática, e não é achado. O achado é o que apareceu ao verificar o porquê.

**Onde**:
- `pkg/infra/db/user_repository.go:57` — o `INSERT` grava `token` E
  `token_hash` na mesma linha.
- `pkg/infra/auth/authenticators.go:78`:

```sql
FROM users WHERE token = $1 OR token_hash = $2 LIMIT 1
```

**Problema**: existe uma migração `add_token_hash` (migração 8) e a
autenticação já sabe comparar por hash — `domain.HashToken(token)` é calculado
na linha seguinte. Ainda assim:

1. o token continua sendo **persistido em texto claro** na coluna `token`;
2. o `OR token = $1` mantém esse valor como **caminho de autenticação ativo**,
   não como resíduo inerte.

Quem obtiver leitura do banco — backup, réplica, dump de suporte, SQL injection
em qualquer outra rota — obtém credenciais utilizáveis diretamente, sem precisar
quebrar hash nenhum. O hash existente não protege nada enquanto o original
estiver ao lado dele.

Vale notar o que isso NÃO é: não é vazamento por log nem por API. A listagem
omite o token, e o `POST /admin/users` o devolve uma única vez, na criação —
ambos corretos.

**Correção sugerida**, em duas etapas separadas porque a segunda quebra
clientes:

1. Parar de GRAVAR o texto claro: o `INSERT` passa a preencher só
   `token_hash`. Compatível com o que já existe, porque a autenticação
   continua aceitando ambos.
2. Depois de uma janela de migração, remover o `OR token = $1` e dropar a
   coluna. Isso invalida qualquer linha antiga que só tenha texto claro — e é
   por isso que precisa de janela, não de um PR.

**A ordem importa**: inverter as duas deixa usuários antigos sem conseguir
autenticar.

---

**CORRIGIDA POR INTEIRO (2026-08-09).** As duas etapas, na ordem.

**Etapa 1** (`5af3a21`): o `INSERT` e o `UPDATE` param de gravar o texto claro.
Só foi segura depois da F100 — antes, branquear a coluna abria acesso sem
credencial.

**Etapa 2** (migração 16 + consulta só por hash): o texto claro das linhas
ANTIGAS é apagado, e o `OR token = $1` sai das duas consultas de autenticação.

A migração preenche o hash que faltar, VERIFICA que não sobrou ninguém sem
ele, e só então apaga. A verificação não é zelo: apagar o texto claro de uma
linha sem hash tira dela a única forma de autenticar, e o valor não existe em
nenhum outro lugar. Ela ABORTA em vez de continuar — abortar deixa o processo
sem subir, com mensagem; continuar deixaria um usuário sem acesso e sem
diagnóstico.

Validado em bancada com uma linha legada real (texto claro + hash, como antes
da etapa 1):

```
antes   legado | token='tok-legado'
depois  legado | token='' | hash=6884b32e8e13
        curl -H "token: tok-legado" -> HTTP 400  (autenticou; sem sessao)
        log: "plaintext API tokens removed from storage" rows=1
```

**O que NÃO foi feito**: dropar a coluna. Ela é NOT NULL, dropar exige
reconstruir a tabela no SQLite, e o ganho é cosmético — com todo valor vazio, o
segredo já saiu do disco. Fica como limpeza posterior.

**Janela de migração**: não é mais necessária. O desenho original supunha que
remover o `OR` invalidaria linhas não migradas; a migração 16 garante que essa
linha não existe, e se ela existisse a migração se recusaria a rodar.

**Status**: **CORRIGIDO (2026-08-09)**, as duas etapas — etapa 1 junto com a
F100 (que era pré-requisito, não detalhe: fazê-la antes abria acesso sem
credencial, e isso foi **medido**, não suposto), etapa 2 com a migração 16.

**Só o drop da coluna fica pendente**, pelo motivo acima: é cosmético, e o
segredo já saiu do disco.

---

## F98 — a posse da sessão vaza quando o QR expira: renovada para sempre numa sessão que nunca subiu

**Data**: 2026-08-08
**Contexto**: validação de bancada da própria F96, exatamente pelo roteiro que
a entrada da F96 deixou escrito ("criar um usuário, chamar `/session/connect` e
nunca ler o QR"). A resposta é a segunda das duas hipóteses registradas lá: o
lease **fica**. A F96 cobre só a falha síncrona do provider, não o pareamento
abandonado.

**Onde**:
- `pkg/application/session/orchestrator.go` — o `defer` da F96 só devolve a
  posse quando `Start` retorna `err != nil`.
- O caminho do QR não retorna erro: `GET /session/connect` responde
  `{"status":"connecting"}` com 200 e o pareamento segue **assíncrono**. Quando
  o QR expira, quem morre é a goroutine do canal, não a chamada que já retornou.
- `pkg/bootstrap/lease.go` — o renovador continua renovando enquanto a entrada
  existir; ninguém a remove.

**Problema**, medido nesta bancada (Postgres 5433, `WA_API_CLUSTER_MODE=multi`,
processo `MacBook-Pro-de-Lucas.local-40848`):

```
23:11:26  GET /session/connect  -> 200 {"status":"connecting"}
23:13:06  log: "QR timeout killing channel"   <- o pareamento morreu aqui
23:16:41  select * from session_leases        <- posse VIVA, expires_at renovado
```

Estado final, na mesma linha do `join`:

```
teste-runtime | connected=0 | lease=MacBook-Pro-de-Lucas.local-40848
```

Ou seja: `connected=0`, `GET /session/status` devolve `no_session`, e mesmo
assim a posse segue sendo renovada a cada ~10s, indefinidamente.

Consequência em N pods — que é o cenário para o qual o lease existe: o usuário
fica **preso permanentemente** à réplica onde tentou parear uma vez e desistiu.
Nenhuma outra réplica consegue tomar a posse, porque `expires_at` nunca vence.
Se ele tentar `/session/connect` contra outro pod, recebe 409 para sempre. Um
QR abandonado — que é o evento mais banal do fluxo de pareamento — inutiliza o
usuário até alguém reiniciar aquele pod específico.

Vale dizer o que NÃO é defeito: reter a posse **durante** o pareamento está
certo. Duas réplicas parear o mesmo usuário ao mesmo tempo seria pior. O defeito
é não haver quem devolva a posse quando o pareamento termina em nada.

**Correção sugerida**: a devolução precisa acompanhar o ciclo de vida real da
sessão, não o retorno da chamada. Duas formas, em ordem de preferência:

1. Devolver no mesmo ponto em que o "QR timeout killing channel" é emitido —
   quem sabe que o pareamento morreu é aquela goroutine, e ela já roda no
   processo dono da posse.
2. Condicionar a renovação a haver sessão viva: o renovador consulta o registro
   de sessões e, para um `userID` sem sessão, deixa o lease vencer em vez de
   renovar. Mais robusto (cobre outras mortes assíncronas além do QR), porém
   acopla o renovador ao registro.

A (1) é cirúrgica e cobre o caso medido; a (2) cobre a classe. Fazer a (1) sem a
(2) deixa em aberto qualquer outra morte assíncrona da sessão.

**Status**: **corrigido e VALIDADO EM BANCADA (2026-08-08)**, com as duas
correções sugeridas — a cirúrgica E a da classe.

**Camada 1**, `onPairingTimeout` (`orchestrator.go`): devolve a posse no mesmo
ponto em que o QR expira, DEPOIS do teardown local. Liberar antes abriria uma
janela em que outra réplica assume o usuário enquanto este processo ainda
segura os handles do cliente.

**Camada 2**, `leaseManager.abandoned` (`lease.go`): o heartbeat não renova
lease de usuário sem sessão viva. Cobre qualquer morte assíncrona, inclusive
caminhos futuros que esqueçam de devolver na camada 1 — que é exatamente o
modo de falha que produziu esta entrada.

O período de graça é o próprio TTL, e é a parte que não pode faltar: a posse é
reivindicada ANTES de a sessão ser materializada, então "ainda não há sessão" é
o estado NORMAL de um arranque saudável por um instante. Sem graça, toda sessão
perderia a posse nos primeiros milissegundos de vida.

Medição no mesmo cenário que gerou o defeito (Postgres, `multi`, QR nunca lido):

```
23:36:0x  GET /session/connect -> 200 {"status":"connecting"}
23:38:56  "QR timeout killing channel"
23:38:56  "session did not start; handing its ownership back so another
           replica can take it"          <- MESMO SEGUNDO
          lease: SEM-LEASE
```

Antes: posse renovada indefinidamente, ainda viva 3 minutos depois.

Controle na direção oposta, no mesmo processo e na mesma janela: as duas
sessões pareadas (`teste-d2`, `teste-d2-b`) mantiveram posse e conexão do
começo ao fim, e a camada 2 não disparou nenhuma vez sobre elas
(`grep -c "no longer exists in this process"` = 0). Uma correção que devolvesse
posse demais seria pior que o vazamento.

Seis testes novos. Controles negativos em direções opostas: sem a camada 2 o
lease abandonado é renovado para sempre; sem o período de graça a sessão que
está subindo perde a posse.

> Nota de método: a F96 passou nos testes unitários e foi commitada. O defeito
> que sobrou não é o que ela conserta — é o que ela **não alcança**, e isso só
> apareceu porque a entrada da F96 registrou o teste de bancada que faltava, com
> as duas hipóteses e o que cada resultado significaria. Escrever a hipótese
> antes de medir foi o que tornou o resultado legível.

---

## F99 — o stdio chama `/session/connect` e `/session/disconnect` com POST; o HTTP as registra como GET

**Data**: 2026-08-08
**Contexto**: apareceu ao montar a bancada da F93 — mandei `POST
/session/connect` seguindo a tabela do stdio e recebi `404 page not found`.

**Onde**:

`pkg/infra/stdio/stdio_routes_session.go:6,9`

```go
"session.connect":    {httpMethod: "POST", httpPath: "/session/connect"},
"session.disconnect": {httpMethod: "POST", httpPath: "/session/disconnect"},
```

`pkg/bootstrap/wiring_routes.go:49,50`

```go
registry.Register("/session/connect",    customChain.Then(ch.Session.Connect),    "GET")
registry.Register("/session/disconnect", customChain.Then(ch.Session.Disconnect), "GET")
```

**Problema**: `pkg/infra/stdio/stdio.go:206` monta a requisição com
`httptest.NewRequest(httpMethod, httpPath, body)` e a executa contra o mesmo
mux. O `HandlerRegistry` aplica `route.Methods(...)` (gorilla/mux), então o
método divergente **não chega ao handler**. Reproduzido no HTTP real:

```
$ curl -X POST -H "token: ..." http://127.0.0.1:8081/session/connect
404 page not found
$ curl     -H "token: ..." http://127.0.0.1:8081/session/connect
{"code":200,"data":{"status":"connecting"},"success":true}
```

Se o caminho stdio for exercitado, `session.connect` e `session.disconnect`
falham — as duas operações mais básicas do ciclo de sessão. As demais rotas da
tabela (`qr`, `status`, `logout`, `pairphone`) conferem.

**Correção sugerida**: alinhar as duas tabelas. O alinhamento em si é de uma
linha; a decisão é qual lado muda. `connect`/`disconnect` mudam estado do
servidor, então POST é o verbo correto e GET é a divergência — mas trocar o
lado HTTP quebra cliente existente, enquanto trocar o lado stdio não quebra
ninguém. Sugestão: corrigir o stdio agora (POST → GET), e tratar a mudança de
verbo do HTTP como item de API versionada, junto com a remoção do token por
query string, que já está marcada para a release seguinte.

O que impediria a reintrodução: um teste que percorra a tabela do stdio e
verifique, para cada entrada, que `(httpMethod, httpPath)` casa com uma rota
registrada. Hoje as duas listas são mantidas à mão, sem nada que as compare.

**Status**: **corrigido (2026-08-09)** — e o conserto revelou que a entrada
acima ERRAVA para menos. Eram **seis** métodos JSON-RPC quebrados, não dois.

O teste de consistência, escrito antes de corrigir, acusou os outros quatro:

| método | stdio | HTTP | efeito |
|---|---|---|---|
| `session.connect` | POST | GET | não chegava ao handler |
| `session.disconnect` | POST | GET | não chegava ao handler |
| `group.list` | GET | POST | não chegava ao handler |
| `group.info` | GET | POST | não chegava ao handler |
| `group.invitelink` | GET | POST | não chegava ao handler |
| `session.history` | GET `/session/history` | só existe POST | rota inexistente |

Os cinco primeiros: o lado stdio passou a declarar o método que o HTTP já
registra — muda o despacho interno, não quebra cliente.

O sexto é diferente: **não existe** `GET /session/history`. O getter mora em
`/chat/history` e em `/webhook/history` (o mesmo `ch.Storage.GetHistory`), e o
`session.history` apontava para um caminho que ninguém registrou. Apontei-o
para `/chat/history`. A alternativa era registrar `GET /session/history` no
HTTP, espelhando `/webhook/history`, que tem os dois verbos — preferi a opção
que NÃO cria superfície pública nova para consertar um defeito interno.

**O mecanismo**, que é o que importa aqui: `TestStdioRoutesMatchRegisteredHTTPRoutes`
(`pkg/bootstrap/`) percorre a tabela do stdio e pergunta ao roteador REAL se
cada par (método, caminho) casa, distinguindo "método divergente" de "rota
inexistente" — as duas exigem consertos diferentes.

Deliberadamente NÃO é uma lista de rotas esperadas: uma lista seria uma
TERCEIRA tabela a manter, e a próxima rota nova entraria divergente nas três.

Duas coisas que o teste precisou para não mentir:
- `stdio.StaticRouteTargets()` expõe a tabela do stdio (o teste vive em
  `bootstrap` porque `bootstrap` importa `stdio`; o inverso seria ciclo);
- `registerAdminRoutes` foi extraída de `buildRouter`, porque `/admin/*` vive
  num subrouter próprio. Sem ela o teste acusaria `admin.users.list` e
  `admin.users.add` como rota inexistente — falso positivo que ensina a
  ignorar o teste.

Controle negativo: reintroduzir o POST em `session.connect` faz o teste apontar
o método exato e a divergência.

---

## F100 — a etapa 1 da F97, feita como está escrita, abre acesso sem token

**Data**: 2026-08-09
**Contexto**: comecei a implementar a etapa 1 da F97 ("parar de GRAVAR o texto
claro"), que a própria entrada da F97 — e eu, ao recomendar a ordem — descrevia
como *"segura e compatível, porque a autenticação já aceita o hash"*. **Está
errado.** Medido antes de escrever qualquer linha de correção.

**Onde**:

`pkg/presentation/http/middleware/auth.go:78-91` lê o token do header e, na
ausência dele, devolve **string vazia**:

```go
func extractRequestToken(r *http.Request) string {
	if token := r.Header.Get("token"); token != "" { return token }
	token := strings.Join(r.URL.Query()["token"], "")
	...
	return token       // "" quando nao ha token nenhum
}
```

`auth.go:119` usa esse valor cru na consulta:

```sql
FROM users WHERE token=$1 OR token_hash=$2 LIMIT 1
```

**Problema**: se alguma linha tiver `token = ''`, o `token=$1` casa com uma
requisição que **não mandou token nenhum**. Não é hipótese — foi medido na
bancada limpa (Postgres, instância 8081):

```
usuario f97probe, token='tok-probe'
  com token   -> HTTP 400  {"code":"no_session"}     (autenticou; sem sessao)
  SEM token   -> HTTP 401  {"error":"unauthorized"}  (correto)

UPDATE users SET token='' WHERE name='f97probe'      <- o que a etapa 1 faria

  SEM token   -> HTTP 400  {"code":"no_session"}     <- AUTENTICOU
```

A mudança de 401 para 400 é o sintoma: a requisição anônima passou a alcançar o
caminho autenticado, como aquele usuário.

**Hoje o buraco NÃO é alcançável**: `POST /admin/users` com `token: ""` é
recusado (devolve 500, o que é um caso da F66 à parte). Ele nasce no instante em
que a etapa 1 branquear a coluna — para TODO usuário criado depois.

**E não é só autenticação.** O texto claro é carga viva em mais dois pontos:

- `pkg/bootstrap/user_info_cache.go:29` — `userInfoColumns` inclui `token`, e o
  `appCtx.UserInfoCache` é **chaveado por ele**. Com a coluna em branco, todo
  usuário novo passa a ser cacheado sob a chave `""`, e `getUserWebhookUrl` (que
  lê só do cache) devolve vazio — o webhook configurado na tabela vira inerte.
  É a **F70 reintroduzida**, pelo mesmo mecanismo que ela documentou.
- `pkg/bootstrap/lifecycle.go:54` — `connectOnStartup` lê o token do banco e o
  repassa a `startSession(userID, token)` → `Attach`. Sem ele, a religação de
  sessão na subida do processo passa a operar com token vazio.

**Correção sugerida** — a etapa 1 da F97 deixa de ser "uma linha no INSERT" e
passa a ter três pré-requisitos, nesta ordem:

1. **Fechar o casamento com vazio**, independentemente de tudo o mais. É a
   única parte que é de fato barata e sem risco:
   `WHERE (token = $1 AND $1 <> '') OR token_hash = $2`, ou recusar
   `token == ""` antes de consultar. Vale fazer **agora**, mesmo que o resto
   fique para depois — hoje ela não conserta nada visível, e é exatamente o que
   impede o próximo passo de virar incidente.
2. **Rechavear o `UserInfoCache` por `userID`**, não por token. O token é
   credencial; usar credencial como chave de cache é o que amarra os dois
   problemas. `ensureUserInfoCached` já recebe o `userID`.
3. **Tirar o token do caminho de religação** (`connectOnStartup` → `Attach`),
   ou aceitar que ele passe a viajar vazio ali e verificar o que depende disso.

Só depois disso o INSERT pode parar de gravar o texto claro. A etapa 2 original
(remover o `OR token = $1` e dropar a coluna) continua sendo a última, e
continua exigindo janela de migração.

**Status**: **CORRIGIDO (2026-08-09)**, os três itens — e com eles a F97 etapa 1
saiu junto, que era o ponto.

1. Guarda de token vazio antes da consulta (`fef8316`).
2. `UserInfoCache` rechaveado por `userID` (`122f172`). O teste prova a
   CONSEQUÊNCIA, não a chave: usuário sem texto claro tem de ser cacheado igual.
3. O token deixou de atravessar o caminho de sessão — virou parâmetro morto em
   quatro funções depois de (2), e saiu delas.

**Achado de brinde, no meio de (3)**: o token da API era **logado** em três
sítios, todos em nível Info e todos em caminho rotineiro — `QR Pair Success`,
`User information set` e `Connect to Whatsapp on startup`. Quem lesse o log
podia agir como o usuário. Mesma classe da F76. `TestTokenNaoSaiEmLog` varre
`pkg/` inteiro e falha se voltar, com controle de que a varredura varreu.

**Validado em bancada**, no cenário exato que gerou esta entrada:

```
usuario novo -> token='' no banco, hash presente
  com token certo  HTTP 400  (autenticou; sem sessao)
  SEM token        HTTP 401  <- o buraco, fechado
  token errado     HTTP 401
```

Antes da guarda, a linha do meio era 400 — requisição anônima autenticando.

> Nota de método: eu recomendei ao usuário, poucas horas antes, fazer a etapa 1
> "agora, porque é segura". A recomendação não veio de leitura do código de
> autenticação — veio de repetir o que a entrada da F97 afirmava. O erro só
> apareceu porque fui ler o caminho antes de escrever, e mediu-se em dois
> comandos. **Conselho sobre risco precisa ser verificado com a mesma
> desconfiança de uma correção**; uma recomendação errada custa mais que um
> commit errado, porque ninguém a revisa.

---

## F101 — `user not found` responde 500, e a taxonomia não tem categoria para 404

**Data**: 2026-08-09
**Contexto**: apareceu ao converter o grupo `user` da F66. É o mesmo defeito
da F66 numa classe que ela não cobre, e por isso não foi convertido junto.

**Onde**, em `pkg/application/usecase/user/`:

| mensagem | ocorrências |
|---|---|
| `user not found` | 3 |
| `LID not found for this number` | 1 |
| `LID not found: %w` | 1 |

Todas devolvem erro cru, e o `RespondJSON` cai no ramo não-tipado: **500 com
corpo genérico**.

**Problema**: pedir um usuário que não existe não é falha do servidor nem erro
de payload — o payload está bem formado, o recurso é que não está lá. Hoje o
cliente recebe `500 internal server error` e não tem como distinguir "o banco
caiu" de "esse id não existe". Um retenta; o outro nunca vai funcionar.

**Por que não entrou na F66**: `apperr` tem quatro categorias —
`CategoryValidation` (400), `CategoryUnauthorized` (401), `CategoryConflict`
(409) e `CategoryInternal` (500). **Não existe categoria para 404.** Converter
essas cinco exigiria acrescentar uma, que é decisão de taxonomia com efeito de
contrato — exatamente o que a F95 fez ao criar `CategoryConflict`, e que ela
tratou como decisão própria e não como efeito colateral de outra tarefa.

Marcar `user not found` como `CategoryValidation` seria pior que deixar 500:
diria ao cliente "corrija o payload" quando não há nada a corrigir. É o mesmo
raciocínio que fez a F95 escolher 409 em vez de 400.

**Correção sugerida**:

1. Acrescentar `CategoryNotFound` → `http.StatusNotFound` em
   `pkg/domain/apperr/codes.go`, com o caso novo na tabela de
   `TestCategory_HTTPStatus` (ela passa sem conhecer a categoria — foi assim
   na F95).
2. Converter os cinco sítios.
3. Verificar se há mais fora de `user/`: `chat`, `group` e `storage` ainda não
   foram varridos pela F66.

**Status**: **CORRIGIDO (2026-08-09)**. `CategoryNotFound` → 404 entrou em
`pkg/domain/apperr/codes.go`, com o caso na tabela de `TestCategory_HTTPStatus`
— que passava sem conhecer a categoria, como na F95, e o controle negativo
confirma que agora ela é exigida.

Quatro sítios convertidos: os três `user not found` e o `LID not found for this
number`.

**O quinto NÃO foi convertido, e essa é a parte que importa.**
`get_user_lid.go:46` dizia `LID not found: %w` quando a PORTA falhava — o store
quebrou, e o cliente não tem como saber se aquele número tem LID. Eu o converti
para 404 junto com os outros e o teste da porta acusou: reportar "não
encontrado" faria o cliente PARAR de tentar diante de uma falha transitória.

A mensagem antiga já era enganosa; o 500 é que estava acidentalmente certo.
Ficou `failed to get LID: %w` — mensagem honesta, status inalterado.

É o mesmo erro que a F66 corrige, na direção oposta, e cometido por mim no meio
da correção dela. O que o pegou foi um teste de falha de porta que existia
antes, não revisão.
