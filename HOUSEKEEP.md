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

### Reprodução em bancada (2026-08-10), com o mecanismo exato

Pareamento novo no pod limpo, HistorySync de ~15.700 mensagens em quatro
lotes. A linha do tempo, do log do servidor:

```
00:09:53  websocket connected                   (painel abre /session/ws)
00:12:12  QR Pair Success                       (rajada comeca)
00:12:24  Saved HistorySync ... savedCount=922
00:12:34  savedCount=4959
00:12:42  savedCount=4949
00:12:48  savedCount=4917
00:13:01  websocket broadcast write failed; dropping connection
          error="... failed to write frame: context deadline exceeded"
00:13:01  websocket disconnected                (duration_ms=187106)
```

**O mecanismo que faltava na entrada original**: a conexão não cai por erro
do cliente nem por fechamento do navegador — cai por **prazo de escrita
estourado**. O `context deadline exceeded` na escrita do quadro é o painel
não conseguindo drenar a rajada, e o servidor cumprindo a F74 ao derrubar
quem não acompanha. As outras duas linhas do mesmo segundo já são
`use of closed network connection`: consequência, não causa.

**E o pior detalhe, que não estava registrado**: o painel **não reconecta**.
Minutos depois da rajada o cabeçalho seguia em `0 conectados`, com a sessão
pareada e viva. Não é uma cegueira momentânea durante a rajada — é
permanente até alguém recarregar a página. Isso muda a prioridade das
correções sugeridas acima: podar e truncar (1) reduz a chance de cair, mas
**não** resolve o caso em que a queda acontece mesmo assim. Falta um item:

4. **Reconexão automática no painel**, com recuo exponencial e um aviso
   visível de "reconectando". Sem isso, qualquer queda — por rajada, por
   suspensão do notebook, por rede — deixa o painel mentindo em silêncio: a
   sessão aparece conectada (isso vem do REST) e o fluxo de eventos está
   morto (isso vem do WebSocket).

**Evidência incidental da F75 na mesma linha**: o registro de fronteira saiu
como `"url":"/session/ws?token=REDACTED"`. A redação está funcionando no
caminho real, e não só no teste.

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

### Bancada (2026-08-10): o que ela estabeleceu, e o que NÃO

Aparelho pareado, mídia real recebida — 4 fotos em álbum (~1s de download cada)
e um vídeo (3s).

**Estabelecido:**
- a fila está no caminho de produção, e não só no teste: uma mensagem de texto
  com carimbo `00:33:13` só foi registrada pelo nosso handler às `00:33:15`,
  logo após o `Media processed` do vídeo. Ela esperou atrás do download.
- a ordem é preservada com mídia real: as 4 fotos foram processadas exatamente
  na ordem de chegada.
- sem regressão: download, `Media processed` e remoção do arquivo temporário
  seguem funcionando.

**NÃO estabelecido — e é o ponto principal da F87:** que o laço de nós do SDK
fica livre. O limiar de aviso é 5s dos dois lados, e o download mais lento da
bancada levou 3s. Abaixo de 5s, **o código ANTIGO também não teria avisado
nada** — então `Node handling took = 0` aqui é compatível com a correção, e não
evidência dela. Um teste cujo controle negativo passaria igual não testou nada
(ARMADILHAS 25).

A propriedade de não bloquear está provada de forma determinística em
`TestSessionQueue_NaoBloqueiaOProdutor`, com evento de 2s e cronômetro antes do
enfileiramento. O que falta é só a confirmação em campo.

**Para fechar**: um download que passe de 5s — vídeo grande ou link lento. Aí o
sinal é binário: ou sai `evento de sessao demorou` com `queued` (fila no
caminho, laço livre) ou sai `Node handling took` (fila fora do caminho).

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

---

## F102 — mensagem `type: media` que o handler não conhece é ignorada em silêncio, sem log

**Data**: 2026-08-10
**Contexto**: validação de bancada da F87 — mídia real chegando numa sessão
recém-pareada.

**Onde**: `pkg/bootstrap/eventhandler_message.go:165-214`
(`processMessageMedia`).

**Problema**: a função é uma sequência de `if` por tipo concreto — imagem,
áudio, documento, vídeo, figurinha. Não há ramo `else`, e nenhuma linha de log
no caminho em que **nenhum** dos cinco casa. Uma mensagem classificada como
mídia pelo SDK que não seja um desses cinco simplesmente não produz nada: nem
download, nem aviso, nem erro.

**Evidência** (log da bancada, cinco mensagens de mídia do mesmo remetente):

```
00:22:08  Message Received  2A3BF272306F7466EA82  type: media     <- sem par
00:22:09  Message Received  2A35D5F80159ED7EDAB4  type: media
00:22:09  Media processed   2A35D5F80159ED7EDAB4.jpe
00:22:09  Message Received  2AC75DDDF3AE23FD05AC  type: media
00:22:10  Media processed   2AC75DDDF3AE23FD05AC.jpe
00:22:10  Message Received  2A0B4735D74DBC97AF1E  type: media
00:22:11  Media processed   2A0B4735D74DBC97AF1E.jpe
00:22:11  Message Received  2A248CB70855A6DD2317  type: media
00:22:11  Media processed   2A248CB70855A6DD2317.jpe
```

Cinco recebidas, quatro processadas. A primeira não tem `Media processed` e
não tem erro. `grep` por `download|media` filtrado por `error|fail` no período
não devolve nada relacionado a ela.

**Causa CONFIRMADA pelo remetente (2026-08-10)**: eram 4 fotos enviadas como
um álbum. O quinto evento é o cabeçalho do álbum. `AlbumMessage` Ele existe no proto vendorizado
(`internal/wa-noise/protocol/proto/waE2E/WAWebProtobufsE2E.pb.go:11240`) e
**não** tem ramo em `processMessageMedia`. Um álbum de 4 fotos chega como um
cabeçalho de álbum mais 4 mensagens de imagem — que é exatamente 5 recebidas e
4 baixadas — que foi exatamente o que aconteceu.

**Nenhuma mídia foi perdida.** O defeito é de observabilidade, não de entrega:
o comportamento (não baixar um cabeçalho de álbum, que não tem mídia) está
certo; o silêncio é que não está.

**Por que importa, independentemente da causa**: o operador vê
`Message Received ... type: media` e nada depois. Esse silêncio é
indistinguível de um download que falhou sem registrar. Numa investigação de
"o cliente diz que mandou foto e não chegou webhook", não há como separar
"não havia mídia para baixar" de "a mídia sumiu no caminho" — e as duas exigem
ações opostas.

**Correção sugerida**, nesta ordem:

1. **Um `else` que registra** o tipo não tratado, com o ID da mensagem e o nome
   do campo presente em `evt.Message`. Barato, e resolve a cegueira mesmo sem
   decidir nada sobre álbum. É o item que vale sozinho.
2. **Decidir sobre `AlbumMessage`**: se o cabeçalho de álbum deve virar evento
   próprio no webhook (com a contagem esperada, que ele carrega) ou ser
   descartado explicitamente. Descartar em silêncio é o estado atual e é o
   único que não deveria continuar.

**Anti-regressão**: teste que monta um `events.Message` com um tipo de mídia
não tratado e verifica que a linha de log sai — não que o download acontece.
O defeito é de observabilidade, e o teste tem de fixar a observabilidade.

**Status**: **CORRIGIDO (2026-08-10)**, com autorização explícita — a
correção (1), o `else` com log.

Sai `Warn` com `message_id`, `type` e `media_type`. O `message_id` é o que
permite cruzar com a linha de `Message Received`; o `media_type` (que o SDK já
derivava e ninguém usava) diz O QUE chegou que não tratamos.

**O cabeçalho de álbum passou a ser reconhecido explicitamente**, e isso NÃO é
a correção (2) disfarçada. É o que impede o aviso de disparar em todo álbum
enviado — o caso medido, e frequente. Um diagnóstico que aparece no caminho
normal deixa de ser lido, e eu teria trocado uma cegueira por um ruído.

Se o cabeçalho deve virar evento próprio no webhook (ele carrega
`ExpectedImageCount`) continua em aberto: é decisão de contrato.

**Dois controles negativos**, e o primeiro precisou ser refeito:

| mutação | resultado |
|---|---|
| aviso removido | `TipoNaoTratadoDeixaRastro` falha |
| álbum deixa de ser reconhecido | `CabecalhoDeAlbumNaoViraRuido` falha, mostrando a linha que poluiria |

A primeira tentativa do controle 1 trocou `if !tratou` por `if false` e
**quebrou o build** — `tratou` ficou sem uso, o teste nem rodou, e o `FAIL` que
apareceu era de compilação. Controle que não compila não é controle: ele não
diz nada sobre o teste. Refeito com `tratou = true` antes da checagem, que
compila e desliga o aviso de verdade.

---

## F103 — mensagem reenviada pelo WhatsApp é processada duas vezes: dois downloads, dois webhooks

**Data**: 2026-08-10
**Contexto**: validação de bancada da F87, com vídeo real. Achado de lado —
eu procurava aviso de lentidão e encontrei o mesmo `messageID` duas vezes.

**Onde**: `pkg/bootstrap/eventhandler_message.go` (todo o caminho de
`handleMessage`) — não há deduplicação por `messageID` em lugar nenhum.
`grep` por `dedup|duplicate|alreadyProcessed|seenMessage` em `pkg/` só devolve
código de migração, nada no caminho de mensagem.

**Problema**: quando o WhatsApp reentrega uma mensagem — o que ele faz depois de
uma falha de decriptação, e o SDK registra como `Unavailable message` seguido de
pedido de reenvio — o nosso handler processa a segunda cópia como se fosse nova.
Baixa a mídia de novo e despacha o webhook de novo.

**Evidência** (log da bancada, mesmo `messageID`, 4 minutos de intervalo):

```
00:33:11  warn  Unavailable message 4ABAFB6B41B6E00C6835 from 150147912749158@lid
00:33:12  info  Message Received      4ABAFB6B41B6E00C6835   (sem campo type:)
00:33:15  info  Media processed       4ABAFB6B41B6E00C6835.m4v
00:33:15  info  Temporary file deleted
00:33:15  warn  No webhook set for user          <- ponto de despacho, 1a vez
00:35:44  info  Message was read
00:37:06  info  Message Received      4ABAFB6B41B6E00C6835   (type: media)
00:37:08  info  Media processed       4ABAFB6B41B6E00C6835.m4v
00:37:08  warn  No webhook set for user          <- ponto de despacho, 2a vez
```

Dois downloads completos do mesmo vídeo, dois arquivos temporários criados e
removidos, e os dois ciclos alcançaram o despacho de webhook.

**Limite desta evidência, declarado**: o usuário da bancada não tinha webhook
configurado, então o que está provado é que **o caminho de despacho foi
alcançado duas vezes** — não que dois POST saíram. A conclusão de entrega
dupla é inferência do código, não observação direta. Confirmar com um sink
local antes de tratar como certo.

**Por que importa**: para o cliente, a mesma mensagem chega duas vezes com o
mesmo `messageID`. Quem usa o webhook para criar registro (pedido, atendimento,
cobrança) duplica, a menos que deduplique por conta própria — e nada na nossa
documentação diz que ele precisa. O custo de banda também dobra, e num vídeo
grande isso não é desprezível.

**Correção sugerida**:

1. **Deduplicar por `messageID`** antes de processar, com janela de tempo
   limitada (um cache com expiração de minutos, não um registro perpétuo). O
   `messageID` do WhatsApp é único por mensagem e estável entre reentregas —
   foi exatamente por isso que este achado apareceu.
2. ~~**Decidir o que fazer com a primeira cópia**~~ — **RESPONDIDO em
   2026-08-10**, por leitura de código, e a resposta muda a correção (1).

   **A primeira cópia É a incompleta, por construção.** A cadeia, verificada:

   1. Chega um `<message>` com filho `<unavailable>` e nenhum `<enc>`. O SDK
      registra `Unavailable message`, pede reenvio ao telefone
      (`ImmediateRequestMessageFromPhone`), despacha `UndecryptableMessage` e
      **retorna** — nenhum `events.Message` é produzido
      (`capabilities/message/decrypt_loop.go:36-46`).
   2. O telefone responde, e a resposta vira evento por
      `HandlePlaceholderResendResponse` →
      **`ParseWebMessage`** (`capabilities/message/history_sync.go:247`).
   3. `ParseWebMessage` (`core/client_session.go:75-84`) monta o `MessageInfo`
      **sem `Type` e sem `MediaType`**, e o `PushName` vem de
      `webMsg.GetPushName()`, que pode vir vazio.

   O caminho AO VIVO, em contraste, lê `info.Type = ag.OptionalString("type")`
   do atributo do nó (`capabilities/message/parse.go:196`).

   **Portanto: deduplicar mantendo a PRIMEIRA cópia entregaria webhook sem
   `pushName` e sem informação de tipo** — e a F84 foi uma entrada inteira
   sobre justamente não descartar o `pushName`. A política tem de ser manter a
   ÚLTIMA, ou mesclar preferindo campo não-vazio.

   **E a janela de dedup precisa ser larga.** No caso observado, as duas
   cópias chegaram com **~4 minutos** de intervalo. Um cache de 30 segundos —
   que é o que se escolhe por instinto — não teria pego este caso.

   **Limite desta resposta**: o mecanismo está verificado no código; a linha do
   tempo (00:33:12 incompleta, 00:37:06 completa) vem do registro desta sessão,
   porque eu **destruí o log original** ao reiniciar o pod da bancada com `>` em
   vez de `>>`. Reproduzir exige forçar uma falha de decifragem, que não é
   trivial sob demanda.

**Anti-regressão**: teste que entrega o mesmo `events.Message` duas vezes e
verifica um único despacho de webhook e um único download. E um segundo teste
que prove a expiração da janela — senão o cache cresce sem teto, que é o
defeito da F86 voltando por outra porta.

**Status**: **CORRIGIDO (2026-08-11)**, com autorização explícita — em
`pkg/bootstrap/message_dedup.go`, com a saída no topo de `handleMessage`.

**A política ficou sendo "guarda a PRIMEIRA", e isso INVERTE o que eu havia
recomendado.** A resposta à pergunta (2) — a primeira cópia é a incompleta — me
levou a dizer "manter a última". Essa recomendação não sobrevive à linha do
tempo: quando a segunda cópia chega, ~4 minutos depois, a primeira já foi
entregue. "Manter a última" exigiria DESENTREGAR, que não existe.

A escolha real é entre dois danos: entregar duas vezes (cliente duplica pedido
ou cobrança) ou entregar uma vez com metadado pobre (falta `pushName` e
`type`). O primeiro é pior por uma margem grande — duplicar dinheiro é
incidente, metadado ausente é degradação.

**O custo ficou MEDIDO, não escondido**: ao suprimir, os campos que a cópia
descartada trazia a mais vão para o log em `metadado_perdido`. Sem isso a
decisão seria aposta permanente; com isso dá para revisar com dado.

**TTL de 10 minutos**, e o número não é arbitrário: o intervalo medido entre as
duas cópias foi de ~4 min. Trinta segundos — o valor que se escolhe por
instinto — não teria pegado nada.

**A chave inclui o `userID`.** O `messageID` do WhatsApp é único por remetente,
não globalmente; chave só pelo id faria a mensagem de um usuário suprimir a de
outro, que é perda silenciosa — muito pior que a duplicata sendo corrigida.

**O controle negativo achou um buraco nos meus próprios testes.** Com a chamada
REMOVIDA de `handleMessage`, **nenhum** dos seis testes falhava: eles cobriam a
função, não o caminho. Uma dedup que existe e não está ligada não deduplica
nada (ARMADILHAS 24). Escrevi `TestDedup_SeamDoHandleMessage`, que verifica o
efeito observável — `st.dowebhook` continuar 0 na segunda cópia — e aí sim o
controle acusou.

---

## F104 — o D1 só protege quem declarou `multi`; escalar réplicas sem declarar nada não é detectado

**Data**: 2026-08-10
**Contexto**: recapitulação da branch e revisão do ADR-0005. Achado ao verificar
uma afirmação minha sobre a configuração de produção — que estava errada, e o
erro escondia um problema maior que o alegado.

**Onde**:
- `pkg/bootstrap/cluster.go:64-74` (`clusterModeFromEnv`) — ausência de
  `WA_API_CLUSTER_MODE` resolve para `single`.
- `pkg/bootstrap/cluster.go:82-89` (`validateStackForMode`) — só recusa a
  combinação quando o modo é `multi`.
- `pkg/bootstrap/cluster.go:101-115` (`lockSingleInstance`) — `flock`, que
  protege contra segundo processo **na mesma máquina**.
- `pkg/infra/db/connection.go:63-91` — sem nenhuma `DB_*`, resolve `sqlite`
  sem qualquer aviso.

**Problema**: o D1 do ADR-0005 recusa subir com `WA_API_CLUSTER_MODE=multi` e
SQLite, e essa guarda funciona. Mas ela está condicionada a que o operador
tenha declarado `multi` — que é justamente o que ele mais provavelmente
esquece. O caminho não coberto:

```
WA_API_CLUSTER_MODE ausente  ->  single (padrao)
DB_* ausentes                ->  sqlite (padrao, sem aviso)
single + sqlite              ->  combinacao LEGAL, sobe normalmente
replicas: 2 no orquestrador  ->  ninguem reclama
```

O `flock` não salva: réplicas em containers distintos não compartilham
namespace de PID, e o lock é por processo na mesma máquina.

**Pior no arranjo atual de produção do disparazaap**: o serviço monta um volume
NOMEADO (`wa-api-data:/app/dbdata`). Duas réplicas no mesmo host Docker montam
o **mesmo** volume — não é "cada réplica com um banco próprio", é dois
processos escrevendo no mesmo arquivo SQLite. Corrupção, não divergência.

**O que NÃO é o problema, e eu afirmei que era**: produção não roda em SQLite
por fallback de configuração parcial. O `.env` e o `compose.yaml` não definem
nenhuma `DB_*`, e o anchor `common-env` só carrega `TZ`. Roda SQLite por padrão
declarado. O `warn` de `incomplete_postgres_env` que me levou ao erro veio de
um pod de bancada, cuja máquina tem `DB_*` parciais no shell. Corrigido também
no ADR-0005.

Isso muda o tamanho do achado: **não há configuração errada em produção hoje**.
Há um padrão que serve para 1 pod e uma armadilha para o dia em que virarem 2.

**Correção sugerida**, da mais barata para a mais invasiva:

1. **D7, o relatório de capacidades no arranque** — decisão do próprio ADR-0005
   que não foi implementada. Um bloco único e legível
   (`database=sqlite mode=single multi_pod=NOT_SUPPORTED`) transforma um estado
   que hoje é inferido em algo declarado. Não impede nada; torna visível.
2. **Expor `db_type` e `cluster_mode` no corpo do `/health/ready`** — barato, e
   deixa o estado auditável de fora do processo, por quem for escalar.
3. **Tornar `DB_*` parcial um erro fatal, em qualquer modo** — um subconjunto
   nunca é intencional. Custo: uma instância com variável residual passa a
   recusar subir no próximo deploy. Em produção o custo é zero (não há
   nenhuma); em máquina de desenvolvimento com `DB_*` parciais no shell, quebra
   na hora. **Precisa de decisão do dono do repositório.**

Nenhuma delas detecta réplicas. Detectar de fora (contar pods, ler a API do
orquestrador) é frágil e falha nos dois sentidos; a alternativa honesta é (1) +
(2) e documentar que `multi` é declaração obrigatória, não inferência.

**Anti-regressão**: teste que fixa a tabela de combinações — `single`+sqlite
sobe, `multi`+sqlite recusa, e o relatório de capacidades sai com os campos
esperados. Hoje só o segundo caso tem teste.

**Pré-requisito que ninguém planejou**: migrar para Postgres não é mudança de
configuração, é **migração de dados**. As sessões pareadas e o
`message_history` de produção estão no SQLite daquele volume. Qualquer caminho
para multi-pod passa por essa migração, e ela não existe.

**Status**: **não corrigido** — registrado. As opções (1) e (2) são aditivas e
podem entrar sem decisão; a (3) muda comportamento de arranque e é decisão do
dono do repositório.

---

## F105 — produção está 5 migrações atrás, e o próximo deploy roda a irreversível

**Data**: 2026-08-10
**Contexto**: backup de produção e ensaio de restauração, primeiro item do
ADR-0007.

**Onde**: volume `disparazaap-mvp_wa-api-data`, tabela `migrations`.

**Problema**: produção está na migração **11**; a branch está na **16**. O
próximo deploy aplica 12, 13, 14, 15 e 16 de uma vez, mais a renomeação
`whatsmeow_*` → `wanoise_*` no store do protocolo.

A migração 16 (`blank_plaintext_token`) é **irreversível**: apaga o token em
texto claro depois de preencher e conferir o hash. Produção tem 7 usuários,
todos com `token_hash` preenchido **e** todos ainda com texto claro. Ou seja:
a 16 não aborta, e apaga os 7.

**Duas consequências que ninguém tinha notado:**

1. **O D3 (outbox) não existe em produção.** A migração 15 nunca foi aplicada
   lá. A durabilidade de entrega que o ADR-0005 registra como PRONTA está na
   branch, não no ar. Webhook pendente em produção ainda morre com o processo.

2. **Depois do deploy, o backup vira o único lugar com os tokens em texto
   claro.** Não é defeito — é o efeito pretendido da F97 —, mas muda o
   procedimento: perder o backup passa a significar retokenizar os 7 clientes.

**Evidência** — ensaio de restauração contra a cópia, com o binário da branch:

```
           antes    depois
migração      11        16
usuários       7         7
texto claro    7         0     <- migração 16, irreversível
token_hash     7         7
histórico 167500    167500
device         1         1     <- credencial do aparelho
identity_keys  3         3
sessions       3         3
pre_keys     809       809
```

Autenticação sobreviveu: token real do backup devolveu **400** (autenticou, sem
sessão), contra **401** para token inválido e **401** sem token.

**Correção sugerida**: nenhuma no código — o caminho de migração está correto e
foi ensaiado. O que falta é **operacional**:

1. Fazer o deploy com o backup em mãos e o manifesto de restauração ao lado
   (feito: `~/backups/wa-api/<timestamp>/MANIFESTO.md`).
2. Guardar os 7 tokens em texto claro num cofre antes do deploy, ou aceitar
   explicitamente que o backup é a única cópia.
3. Não deixar a distância crescer de novo. Cinco migrações acumuladas
   transformam um deploy de rotina num evento — e foi só por acaso que a
   irreversível estava entre elas numa hora em que alguém olhava.

### Quando fazer o deploy, e por quê (2026-08-10, revisto)

Eu havia recomendado "deploy logo, por causa do outbox". **Estava errado, e a
suposição era minha.** Consultei o backup:

```
7 usuários, TODOS sem webhook configurado, TODOS com connected=0
main.db: 1 device pareado, 3 sessions
arquivos sem escrita desde 08/08
```

**O outbox não compra nada em produção hoje** — não há webhook para entregar.

**O que o deploy realmente compra**: produção roda código de 06/08 ou antes —
afirmável com precisão, porque o banco tem tabelas `whatsmeow_*` e a renomeação
para `wanoise_*` é a migração 12, de 07/08. Logo, ela está **antes da
vendorização inteira**, e não tem:

- as ~25 correções de pânico alcançável por dado da rede feitas no fork (os 11
  do `GetBotProfiles`, `handlePairSuccess`, `<failure reason=413>`, a guarda de
  `MessageIDs[0]`) — eram bugs reais do upstream, e produção roda em cima do
  upstream;
- logout que de fato encerra a sessão (F79), status correto após logout (F80),
  `/user/lid` utilizável (F81), sessão QR sobrevivendo a restart (F82);
- token só em hash (F97).

**Mas nada disso está em risco AGORA**: zero usuários conectados, nada sendo
processado. Não há urgência.

**O argumento honesto é o custo, não o risco.** Deployar agora custa menos que
em qualquer outro momento: sem sessão ativa não há reconexão a perder, não há
janela que incomode ninguém, e a migração irreversível roda com o backup fresco
e o ensaio recente. Esperar até alguém parear e usar significa pagar o mesmo
deploy com sessão viva em cima.

**Classificação: oportunista, não prioritário.** Fazer quando for conveniente,
aproveitando a ociosidade.

**Status**: **não corrigido** — é decisão operacional, não mudança de código.
O ensaio está feito e passou; o deploy em si é sua decisão.

---

## F106 — uma linha de payload ilegível trava o outbox de cabeça de fila, para sempre

**Data**: 2026-08-10
**Contexto**: cobertura do ramo `FOR UPDATE SKIP LOCKED` (Fase 1 do ADR-0007).
Achado de lado, ao ler o caminho de reivindicação para escrever o teste.

**Onde**: `pkg/infra/db/webhook_outbox.go:220-226`, em `scanOutboxRows`.

**Problema**: o comentário e o código dizem coisas opostas, e o código está do
lado errado.

```go
if err := json.Unmarshal([]byte(raw), &e.Payload); err != nil {
    // Uma linha ilegível não pode derrubar o lote inteiro: ela ficaria
    // para sempre bloqueando entregas saudáveis atrás dela. Sai do lote
    // e continua vencida, para aparecer na varredura seguinte — e o erro
    // vai para quem chama, que decide se loga.
    return nil, fmt.Errorf("webhook outbox: payload ilegivel em %s: %w", e.ID, err)
}
```

O comentário **nomeia exatamente o modo de falha** que afirma estar evitando, e
a linha seguinte o implementa: `return nil, err` aborta o lote inteiro.
`pushDueAt` não roda, nenhum `due_at` é empurrado, nada é commitado.

É um comentário que documenta uma correção que nunca foi escrita — o que é pior
que não ter comentário, porque quem revisar vai ler a intenção e conferir que
ela está descrita.

**Cenário concreto de falha**: uma linha com `payload` JSON inválido — restauração
parcial, escrita truncada, ou um payload antigo com escape quebrado. Ela tem o
`due_at` mais antigo, então `ORDER BY due_at LIMIT 64` a coloca no lote **toda
vez**. A partir daí:

- toda `ClaimDue` falha no scan;
- nenhuma das outras 63 entregas saudáveis é reivindicada;
- e isso não se resolve sozinho, porque o `due_at` da linha ruim nunca é
  empurrado para frente.

O outbox para de entregar **tudo**, permanentemente, por causa de uma linha.

**Por que ninguém notou**: os 9 testes existentes montam payload válido. Não há
teste com payload corrompido, e o caminho de erro nunca foi exercitado.

**Correção sugerida**: fazer o que o comentário já promete — pular a linha,
registrar em log com o `id`, e seguir com o resto do lote. E empurrar o `due_at`
da linha ruim junto com as demais, senão ela volta no lote seguinte e o custo
vira permanente de outra forma.

Vale decidir também o que fazer com ela no fim: uma linha que nunca vai ser
entregável precisa de uma saída (contador de tentativas, ou tabela de descarte),
senão fica vencida para sempre gerando log.

**Anti-regressão**: teste que insere uma linha com payload inválido junto com N
válidas e verifica que as N válidas SÃO reivindicadas. Hoje esse teste falharia.

**Status**: **CORRIGIDO (2026-08-10)**, com autorização explícita.

A linha ilegível agora sai do lote de verdade: log em `Error` com o `outbox_id`
e o `user_id`, e o lote segue com as demais.

**A metade não-óbvia da correção** foi empurrar o `due_at` da linha ruim JUNTO
com as reivindicadas. Pular sem empurrar a deixaria na cabeça do
`ORDER BY due_at` para sempre — voltando em todo lote, ocupando uma vaga das 64
e gerando uma linha de log por varredura, indefinidamente. Trocaria um
travamento total por um vazamento permanente de vaga.

Por isso `pushDueAt` passou a receber ids em vez de `[]OutboxEntry`, e o
early-return de `ClaimDue` passou a considerar as duas listas: um lote em que
TODAS as linhas são ilegíveis ainda precisa empurrar e commitar, senão trava do
mesmo jeito.

`Error` e não `Warn` de propósito: o payload foi escrito por nós, então
ilegível aqui é corrupção de dado ou defeito de escrita — não é condição
esperada de operação.

**Controle negativo executado**: restaurando o `return nil, err`, o teste falha
com *"ClaimDue devolveu erro por causa de UMA linha ilegivel; o lote inteiro
morreu"*. Restaurado por `cp`, conferido por `grep` na linha que implementa.

**O que ficou de fora, e continua em aberto**: o destino final da linha
ilegível. Hoje ela é empurrada a cada varredura e logada a cada varredura —
melhor que travar tudo, mas é ruído perpétuo. Falta decidir a saída: contador
de tentativas com descarte, tabela de mortos, ou intervenção manual. Isso é
decisão de contrato, não de implementação.

---

## F107 — a expiração do lease PERMITE o failover, mas nada o dispara: sessão órfã fica órfã

**Data**: 2026-08-10
**Contexto**: medição M1 da Fase 1 do ADR-0007 — quanto tempo o Postgres leva
para liberar um lease de pod morto por `kill -9`. A resposta veio, e junto veio
um achado maior que a pergunta.

**Onde**: os **dois** — e só dois — sítios que reivindicam posse:
- `pkg/bootstrap/lifecycle.go:91` (`connectOnStartup`), cuja consulta é
  `SELECT ... FROM users WHERE connected=1` (`lifecycle.go:54`);
- `pkg/bootstrap/session_orchestrator_wiring.go:49`, no caminho de requisição
  HTTP.

**Problema**: não existe varredura de leases expirados. Nenhum laço periódico,
nenhum trabalho de fundo. A cláusula `WHERE session_leases.expires_at < now()`
em `session_lease.go:68` deixa a linha DISPONÍVEL, mas alguém precisa tentar
reivindicá-la — e as duas únicas coisas que tentam são o arranque do processo e
uma requisição HTTP para aquele usuário.

Consequência: **um pod já de pé, sem tráfego para aquele usuário, não recupera a
sessão órfã.** O lease expira, a linha fica disponível, e ninguém a pega.

**Evidência** (bancada, Postgres real, modo `multi`): numa rodada em que a
sonda estava quebrada, o lease ficou expirado por **~119 segundos** com o pod B
**rodando**, e o `owner_id` continuou sendo o do pod A morto. O pod B só assumiu
quando chegou um `GET /session/connect`.

**Por que isso muda a leitura do ADR-0005**: o documento trata o TTL como o
custo do failover. Ele não é. **O TTL é o piso do downtime, não o downtime.** O
tempo real até a sessão voltar é o TTL mais o intervalo até alguém bater naquele
usuário — que pode ser minutos, horas, ou nunca, se o tráfego for de entrada
(mensagem chegando do WhatsApp) em vez de saída.

E o caso pior é o mais provável: uma sessão que só RECEBE mensagem não gera
requisição HTTP nenhuma. Ela fica órfã até o próximo restart do pod.

**LIMITE DESTA EVIDÊNCIA, e ele importa.** A medição usou um usuário de teste
com `connected` NULL. O caminho `connectOnStartup` com `connected=1` — que é o
que valeria num restart real de produção — **não foi exercitado**.

A frase "fica órfã até o próximo restart", acima, portanto contém uma suposição
minha, não uma medição: ela pressupõe que o restart recupera. É plausível, e é
o que o código sugere, mas ninguém mediu. Se `connectOnStartup` também não
recuperar, a sessão órfã fica órfã até intervenção humana — que é uma entrada
bem pior que esta.

Antes de a F107 virar decisão, é esse o experimento que falta: derrubar o dono
com `connected=1` no banco, reiniciar o outro pod, e medir se ele assume.

**Correção sugerida**: um laço que varre leases expirados e tenta reivindicar os
que pertencem a usuários com `connected=1`. É o mesmo trabalho que
`connectOnStartup` já faz, só que periódico em vez de uma vez.

Decidir junto: com que intervalo, e se todo pod varre (mais rápido, mais
disputa) ou só um (menos disputa, precisa de eleição — que é exatamente o que
não temos).

**Anti-regressão**: teste que expira um lease com o pod B de pé e SEM tráfego
para o usuário, e verifica que o pod B assume dentro de um prazo. Hoje esse
teste falharia, e é a prova de que o comportamento não existe.

**Status**: **não corrigido** — é lacuna de desenho do D2/D5, não defeito de
implementação. Precisa de decisão antes de código.

---

## F108 — `/session/connect` responde 200 "connecting" mesmo com a posse NEGADA

**Data**: 2026-08-10
**Contexto**: achado de lado na medição M1.

**Onde**: `pkg/presentation/http/handlers/handler_session.go:90-91`.

**Problema**: o handler dispara a conexão numa goroutine (fire-and-forget) e
responde **antes** de o resultado ser conhecido. Quando a posse é negada porque
outra réplica é dona, o cliente recebe:

```
HTTP 200 {"status":"connecting"}
```

O log do processo registra corretamente `session owned by another replica;
skipping`. O cliente não fica sabendo de nada.

**Por que importa**: o cliente não tem como distinguir "está conectando" de
"este pod recusou e nunca vai conectar". Ele vai esperar por um estado que não
vem, e a única saída é sondar `/session/status` até desistir por conta própria.

Em `single` isso nunca aparece — não há com quem competir —, o que explica não
ter sido notado. Em `multi` é o caminho comum enquanto não houver roteamento por
dono (D5): a requisição cai em qualquer pod, e a maioria dos pods não é o dono.

**Correção sugerida**: decidir a posse **antes** de responder, e devolver o que
o D5 vai precisar de qualquer forma — 409 com o dono, ou encaminhar. Isso se
resolve junto com a decisão 2 do ADR-0007, não separado.

**Status**: **não corrigido** — está no caminho da Fase 4 do ADR-0007, e
corrigir antes seria decidir o roteamento por acidente.

---

## F109 — retomada de lease EXPIRADO é indistinguível de renovação normal, e não deixa rastro

**Data**: 2026-08-10
**Contexto**: medição M2 da Fase 1 do ADR-0007 (despejo falso sob `SIGSTOP`).

**Onde**: `pkg/bootstrap/lease.go:231-235`, o ramo `err == nil && owned` de
`renewOne`.

```go
case err == nil && owned:
    m.mu.Lock()
    m.lastRenewal[userID] = m.now()
    m.mu.Unlock()
    return true
```

**Problema**: esse ramo trata dois eventos muito diferentes como o mesmo:

1. **renovei um lease que eu ainda tinha** — o caso normal, a cada 5s;
2. **retomei um lease que já tinha EXPIRADO** — permitido por
   `session_lease.go:68` (`WHERE session_leases.expires_at < now()`), e que só
   acontece depois de uma janela em que qualquer réplica podia legitimamente
   ter assumido a sessão.

Os dois carimbam `lastRenewal` e devolvem `true`, em silêncio. Nada no log
distingue um do outro.

**Evidência** (bancada, TTL 15s, pausa de 20s sem competidor): o lease expirou —
confirmado no banco com `valido=false` — e ao acordar o pod retomou a posse e
seguiu servindo com **zero linhas de log**. `final dono=...-29215
restante=10,47s`, `connected=true`.

**Por que importa**: não é inseguro em si — naquele momento ninguém mais tinha a
sessão. O problema é que o pod **serviu durante uma janela em que não era
legitimamente dono**, e não há como saber disso depois. Numa investigação de
mensagem duplicada ou de ordem trocada, essa janela é exatamente a hipótese que
alguém precisaria considerar, e ela é invisível.

**A máquina para consertar já existe no arquivo.** O ramo `default` logo abaixo
(`lease.go:242-257`) já calcula `elapsed := m.now().Sub(last)` para decidir sem
poder falar com o banco. Falta só usar o mesmo cálculo no caminho de sucesso:
quando `elapsed > m.ttl`, emitir WARN — o lease foi retomado, não renovado.

**Cenário observado que é a mesma raiz** (o agente reportou como defeito
separado; não é): pod B tomou o lease expirado de A, o pareamento de B falhou 3s
depois, B devolveu a posse — e isso **é** logado, em
`lease_wiring.go:156` — e o A congelado retomou tudo ao acordar sem saber que
tinha sido despejado. O resultado final é seguro; o que falta é o A registrar
que perdeu e retomou. Mesmo `case`, mesma correção.

**Anti-regressão**: teste que expira o lease no relógio injetado, deixa o mesmo
dono reivindicar de novo, e verifica que sai WARN. Hoje não sai nada.

**Status**: **CORRIGIDO (2026-08-10)**, com autorização explícita.

O ramo `owned` passou a comparar a lacuna desde a última renovação com o TTL, e
emite WARN quando ela passou — com `gap` e `ttl` no registro, porque saber QUE
houve janela sem saber de quanto não ajuda quem investiga.

A máquina era mesmo a que já existia: o cálculo é o mesmo `agora.Sub(anterior)`
que o ramo `default` usa para decidir sem poder falar com o banco.

**`anterior` zerado não conta como lacuna**: é a primeira renovação depois do
`Claim` inicial, e avisar ali seria falso positivo em todo arranque.

**Dois controles negativos, em direções opostas** — e é o par que torna a
correção honesta:

| mutação | resultado |
|---|---|
| aviso removido | `TestLease_RetomadaAposExpirarDeixaRastro` falha |
| aviso em TODA renovação (`lacuna >= 0`) | `TestLease_RenovacaoNormalNaoAvisa` falha, mostrando `gap=5000` |

O segundo é o que impede a correção de virar ruído: com heartbeat de 5s, um
aviso por renovação seriam 12 linhas por minuto **por sessão**. Sem esse teste,
a mutação passaria despercebida e o registro viraria lixo — que é uma forma
mais lenta de perder a mesma informação.

---

## F110 — `TestOutboxWiring_VarreduraRetomaOVencido` é instável sob carga

**Data**: 2026-08-10
**Contexto**: observado uma vez durante o `make check` da correção da F109 —
que não toca em nada do outbox.

**Onde**: `pkg/bootstrap/dispatch_outbox_test.go`, por volta da linha 285.

~~**Problema**: o teste depende de relógio real
(`DueAt: time.Now().UTC().Add(-time.Minute)`) e da varredura do sweeper, que
roda a cada 1s.~~

**ESSE DIAGNÓSTICO ESTAVA ERRADO, nos dois pontos**, e eu o escrevi a partir de
um `grep` que mostrou `time.Now()` — sem ler o teste. O `DueAt` no passado é
determinístico, e a varredura é chamada EXPLICITAMENTE (`sweepOutboxOnce`), não
pelo tick de 1s.

**A causa real**: `esperarOutboxVazio` (`dispatch_outbox_test.go:218`) tinha um
prazo fixo de 3s contando `PendingCount`, mas `sweepOutboxOnce` apenas
DESPACHA a entrega — `dispatchGo("outbox-retry", ...)` em
`dispatch_outbox.go:235`. Quem liquida a linha é o worker do pool. Contar antes
de o worker terminar mede um estado intermediário, e o prazo virava corrida
contra a carga da máquina.

**Evidência**: 1 falha em 1 execução do `make check`; depois **8/8 verde** (5×
isolado, 3× na suíte completa do pacote). Não reproduzível sob demanda.

**Por que importa mais do que parece**: um gate que falha por acaso ensina a
rodar de novo em vez de investigar. O custo não é o minuto perdido — é que a
próxima falha REAL vai ser tratada como esta.

**Correção sugerida**: injetar o relógio, como o `leaseManager` já faz
(`m.now func() time.Time`), e disparar a varredura explicitamente em vez de
esperar o tick. O padrão já existe no repositório.

**Status**: **CORRIGIDO (2026-08-10)**, as duas coisas.

**1. A espera virou determinística.** `esperarOutboxVazio` passou a drenar o
pool de despacho antes de contar, com o helper que já existia no repositório
(`esperarDespachoDrenar`, que lê `dispatch.Metrics()`). O prazo de 3s continua,
mas como rede de segurança e não como mecanismo: se a liquidação não acontecer
nem depois de o pool drenar, é defeito de verdade e o teste deve falhar.

**2. O gate deixou de ser mudo.** `coverage-gate` não manda mais a saída dos
testes para `/dev/null`; captura em `coverage.out.log` (já coberto por `*.log`
no `.gitignore`) e imprime as linhas de falha quando falha.

**Controle executado**: acrescentei um teste que falha de propósito e rodei
`make coverage-gate`. Antes sairia só `Error 1`. Agora sai:

```
FALHA: os testes do coverage-gate falharam. Saida abaixo (F110):
--- FAIL: TestControleTemporarioF110 (0.00s)
```

Teste temporário removido em seguida.

### Segunda ocorrência (mesma data), e um achado maior junto

Reapareceu no `make check` da F102 — **2 falhas em ~6 execuções**. Frequência
alta o bastante para atrapalhar de verdade.

**E o modo como ela apareceu é pior que ela.** O alvo `coverage-gate` do
Makefile (linha 105) roda os testes com a saída para `/dev/null`:

```make
@$(GOTEST) -count=1 $(COVER_PKGS) ... -coverprofile=$(COVERAGE_OUT) > /dev/null
```

Quando um teste falha ali, o `make` para com `Error 1` e **nenhuma linha diz
qual teste, nem por quê**. Foi exatamente o que aconteceu: `grep FAIL` no log
devolveu zero, porque o `FAIL` foi para `/dev/null`, e o gate ficou parecendo
quebrado sem motivo.

**Correção sugerida** (segunda, e independente da primeira): não silenciar a
saída dos testes no `coverage-gate`, ou capturá-la em arquivo e imprimir só em
caso de falha. Um gate que falha sem dizer por quê ensina a rodar de novo — e é
assim que a próxima falha REAL passa despercebida.

Isso agrava o próprio motivo da entrada: teste instável mais gate mudo é a
combinação que transforma "rodar de novo" em hábito.

---

## F111 — `TestLease_RetomadaAposExpirarDeixaRastro` corre com a goroutine de outro teste sob `-race`

**Data**: 2026-08-18
**Contexto**: achado incidental durante o CAP-01 (POST /chat/send/text passa a
enviar de verdade). Não é escopo da task — nenhum arquivo de
`pkg/bootstrap/lease*.go` foi tocado nesta sessão.

**Onde**: `pkg/bootstrap/lease_test.go:527` (write, dentro de
`TestLease_RetomadaAposExpirarDeixaRastro`) vs `pkg/bootstrap/lease.go:260`
(`log.Warn()` dentro de `leaseManager.renewOne`, chamado pelo `RunHeartbeat`
que `TestLease_WithoutLiveSessionCheckKeepsRenewing` disparou numa goroutine
própria em `lease_test.go:448`).

**Problema**: `go test -race ./pkg/bootstrap/...` (dentro de `make check`)
falhou com:

```
WARNING: DATA RACE
Write at 0x0001021e5d60 by goroutine 2266:
  wa-api/pkg/bootstrap.TestLease_RetomadaAposExpirarDeixaRastro()
      lease_test.go:527
Previous read at 0x0001021e5d60 by goroutine 2262:
  github.com/rs/zerolog.(*Logger).disabled()
  ...
  wa-api/pkg/bootstrap.(*leaseManager).renewOne()
      lease.go:260
  wa-api/pkg/bootstrap.(*leaseManager).RunHeartbeat()
      lease_test.go:448 (goroutine criada por TestLease_WithoutLiveSessionCheckKeepsRenewing)
--- FAIL: TestLease_RetomadaAposExpirarDeixaRastro (0.00s)
    testing.go:1617: race detected during execution of test
```

`TestLease_WithoutLiveSessionCheckKeepsRenewing` sobe uma goroutine de
heartbeat e aparentemente não garante o encerramento dela (join/cancel) antes
de retornar; quando `TestLease_RetomadaAposExpirarDeixaRastro` roda em
seguida no mesmo processo, a goroutine ainda viva do teste anterior toca
memória que o teste seguinte também toca — o detector de `-race` não
distingue "teste terminou" de "goroutine que ele soltou terminou".

**Reprodução**: intermitente sob a suíte completa do pacote — falhou 1 de 4
execuções de `make check` nesta sessão. Isolado
(`go test ./pkg/bootstrap/... -race -run TestLease_RetomadaAposExpirarDeixaRastro -count=5`)
e a suíte inteira (`go test ./pkg/bootstrap/... -race -count=3`) passaram
100% das vezes — a race só aparece quando os dois testes específicos correm
próximos o bastante no mesmo binário de teste.

**Correção sugerida**: `TestLease_WithoutLiveSessionCheckKeepsRenewing`
precisa sincronizar o fim da goroutine de `RunHeartbeat` (contexto cancelado
+ `<-done` ou `sync.WaitGroup`) antes de retornar, em vez de deixá-la morrer
por conta própria depois que o teste já saiu.

**Status**: **NÃO CORRIGIDO** — fora do escopo do CAP-01, e a correção mexe
em `pkg/bootstrap/lease_test.go`, arquivo que esta task não toca. Registrado
para decisão do usuário sobre quando corrigir.

---

## F112 — `pkg/infra/media/media_utils.go` duplica a infra de Open Graph que CAP-01.1 conectou

**Data**: 2026-08-18
**Contexto**: CAP-01.1 (fechar `domain.SendMessageRequest.LinkPreview`, que
a API aceitava e ignorava em silêncio desde a primeira versão do struct —
ver `git log -S LinkPreview`, commit `542e707` do wuzapi original: "Add
LinkPreview support to SendMessage and improve Open Graph data fetching").

**Onde**: `pkg/infra/media/media_utils.go:1-306` (pacote `media`, funções
`GetOpenGraphData`, `ExtractFirstURL`, `fetchOpenGraphDataInternal`,
`UserSemaphoreManager`) vs `pkg/infra/media/opengraph/fetch.go:1-182`
(pacote `opengraph`, mesma lógica, comentário próprio: "extracted from root
wa-api/helpers.go (Phase 12b)").

**Problema**: as duas implementações fazem a MESMA coisa — buscar HTML,
parsear meta tags Open Graph, baixar e redimensionar a imagem — com a mesma
origem (`helpers.go` do wuzapi). `media_utils.go` tem ZERO chamadores fora
do próprio arquivo e de `media_utils_test.go` (confirmado por
`grep -rn "GetOpenGraphData\|ExtractFirstURL" pkg/ cmd/` antes desta task).
CAP-01.1 conectou `opengraph.Fetcher` (novo, em
`pkg/infra/media/opengraph/adapter.go`) ao `SendMessageUseCase` via
`appport.LinkPreviewFetcher` — `media_utils.go` continua morto, e agora
tecnicamente enganoso: quem procurar "onde é a busca de Open Graph deste
repo" pode achar o pacote errado primeiro.

O comentário de topo de `pkg/infra/egress/egress.go` já registra que UMA
função de `media_utils.go` (`IsHTTPURL`) foi removida na Fase 2 — o arquivo
já vinha sendo esvaziado aos poucos, só não até o fim.

**Correção sugerida**: deletar `pkg/infra/media/media_utils.go` e
`pkg/infra/media/media_utils_test.go` inteiros, com todos os testes
migrando (se ainda não cobertos) para `pkg/infra/media/opengraph`. Baixo
risco — zero chamador de produção — mas fora do escopo fechado do
CAP-01.1, que é aditivo (só ligou o fio até então solto), não uma faxina de
dead code em arquivo que a task não tinha motivo para tocar.

**Status**: **NÃO CORRIGIDO** — registrado para decisão do usuário.

---

## F113 — `opengraph.Fetcher` sem cache nem limite de concorrência por sessão

**Data**: 2026-08-18
**Contexto**: mesma task do F112 (CAP-01.1).

**Onde**: `pkg/infra/media/opengraph/adapter.go` (`Fetcher.FetchLinkPreview`,
usado por `SendMessageUseCase.Execute` quando `LinkPreview=true`).

**Problema**: cada chamada com `LinkPreview=true` dispara uma busca de rede
nova (fetch de página + fetch de imagem), mesmo que a mesma URL já tenha
sido resolvida há 1 segundo, e sem limite de fetches concorrentes por sessão
(`txtID`). O wuzapi original (commit `542e707`, e o que ficou em
`media_utils.go`) tinha as duas proteções: `singleflight.Group` + cache TTL
de 5 minutos (`openGraphCache`) e um semáforo de até 20 fetches concorrentes
por usuário (`UserSemaphoreManager`). `opengraph.Fetcher` não tem nenhuma
das duas — um cliente que manda várias mensagens com `LinkPreview=true`
para a mesma URL, ou em rajada, gera uma busca de rede por chamada, sem
teto de concorrência.

O client injetado (`bootstrap.NewSafeHTTPClient()`) tem timeout de 60s por
requisição mas nenhum limite de QUANTAS requisições simultâneas saem por
sessão — o teto de recurso que a Regra 1 do HOUSEKEEP (seção "regressão
introduzida pela própria correção") pede para todo mecanismo que possa
segurar um slot: aqui não há slot nenhum, é fetch direto, então o risco não
é "trava o pool" (não há pool), é amplificação de tráfego de saída sem teto
por sessão.

**Correção sugerida**: portar `singleflight.Group` + cache TTL +
`UserSemaphoreManager` de `media_utils.go` (F112) para
`opengraph.Fetcher`, parametrizado por `txtID` (o `userID` que
`media_utils.go` já usava). Isso resolveria F112 e F113 juntas.

**Status**: **NÃO CORRIGIDO** — CAP-01.1 é escopo fechado (fechar o campo
ignorado, não construir rate limiting novo); registrado para decisão do
usuário sobre quando endurecer.

## F114 — o preview enviado é só a thumbnail inline; o card grande do WhatsApp não é montado

**Data / contexto**: 2026-08-18, no gate adversarial (GATE 0) de CAP-01.1 —
o bloco que religou o campo público `LinkPreview`, inerte desde a deleção do
`handlers.go` em `41bc8e2`.

**Onde**: `pkg/infra/wa-noise/adapters/chat/messenger.go`
(`ChatMessengerAdapter.SendText`, ramo `preview != nil`), que preenche
`ExtendedTextMessage{Text, MatchedText, Title, Description, JPEGThumbnail}`.

**Problema**: a implementação histórica recuperada de
`git show 41bc8e2^:handlers.go` fazia mais do que isso. Depois de montar a
mensagem, ela subia a imagem em alta resolução pelo protocolo:

```go
if len(og.HQImageData) > 0 {
    uploaded, upErr := client.Upload(ctx, og.HQImageData, whatsmeow.MediaLinkThumbnail)
    if upErr != nil {
        log.Warn()... // "sending inline thumbnail only"
    } else {
        etm.ThumbnailDirectPath = ...; etm.ThumbnailSHA256 = ...
        etm.ThumbnailEncSHA256 = ...; etm.MediaKey = ...
        etm.MediaKeyTimestamp = ...; etm.ThumbnailWidth = ...
        etm.ThumbnailHeight = ...
    }
}
```

O comentário do próprio código histórico diz o efeito da ausência desses
campos, e é a evidência de que a diferença é visível para o usuário final:

> "Upload the high-res thumbnail so clients render the large preview card;
> without these fields only the small inline thumbnail shows."

Ou seja: hoje o preview funciona, mas renderiza como thumbnail pequena
inline, não como o card grande. Não é falso-sucesso — a mensagem é enviada e
o preview aparece —, é uma capability entregue com fidelidade menor que a
histórica, e isso não estava registrado em lugar nenhum.

**Correção sugerida**: expor `Upload` na interface estreita
`pkg/infra/wa-noise/client/client.go` (hoje ela não o expõe) e, no adapter,
subir `HQImageData` como `MediaLinkThumbnail`, preenchendo os sete campos
acima. Falha de upload MUST degradar para a thumbnail inline com log em
`Warn`, exatamente como o original — nunca falhar o envio. Repare que
`domain.LinkPreviewData` hoje só carrega `ThumbnailJPEG`; seria preciso
carregar também a imagem HQ e suas dimensões.

**Relação com CAP-02**: `Upload` na interface estreita é exatamente a
primitiva que Send Media URL vai precisar. Se CAP-02 a expuser, esta entrada
fica barata de fechar depois — vale reavaliar F114 logo após CAP-02.

**Status**: **NÃO CORRIGIDO** — fora do escopo fechado de CAP-01.1
(religar o campo ignorado, não reconstruir a fidelidade completa do card).
Registrado como REQUIRED_FIX doc-only do GATE 0, que quanto ao resto deu
PASS.

## F115 — `SendImage` (CAP-02) envia sem `JPEGThumbnail`, divergindo do
`handlers.go` histórico

**Data/contexto**: 2026-08-18, CAP-02 (POST /chat/send/image, ramo URL).

**Onde**: `pkg/infra/wa-noise/adapters/chat/messenger.go`,
`ChatMessengerAdapter.SendImage` — a `waE2E.ImageMessage` montada não
preenche `JPEGThumbnail`. O `handlers.go` pré-refactor
(`git show 41bc8e2^:handlers.go`, trecho do branch de imagem) decodificava
`filedata` com `image.Decode`, gerava uma miniatura 72x72 com
`jpegThumbnail(img, 72, 72)` e a atribuía a `ImageMessage.JPEGThumbnail`
antes de enviar.

**Problema**: sem `JPEGThumbnail`, o cliente WhatsApp perde o preview de
baixa resolução que aparece na lista de conversas e na bolha da mensagem
antes do download completo do anexo — mesma classe de perda de fidelidade
que a F114 documentou para link preview (thumbnail pequena vs. nenhuma).
Não é falso-sucesso: a imagem chega, só sem a miniatura instantânea.

**Correção sugerida**: no adapter, decodificar `payload.Bytes` com
`image/*` (mesmos pacotes que `opengraph.FetchOpenGraphImage` já importa:
`image`, `image/jpeg`, `image/png`, `image/gif`), gerar uma miniatura
72x72 e preencher `ImageMessage.JPEGThumbnail`. Falha de decode/thumbnail
MUST degradar para envio sem miniatura com log em `Warn` — nunca falhar o
envio por causa disso, mesmo princípio que a F114 já registrou para o
upload da thumbnail HQ do link preview.

**Status**: **NÃO CORRIGIDO** — fora do escopo fechado de CAP-02 (entregar o
ramo URL ponta a ponta com upload+envio reais; ACCEPTANCE_CRITERIA da task
não menciona thumbnail). Decisão consciente: registrada aqui em vez de
implementada sem pedir, por ser scope creep sobre uma task já grande.

## F116 — `SendAudioRequest.Caption` é aceito pela API e não tem representação no protocolo

**Data / contexto**: 2026-08-18, durante CAP-05 (ligar o envio real de áudio).
Descoberto pelo executor e reconferido pelo avaliador independente no `.pb.go`
gerado.

**Onde**: `pkg/domain/message.go` (`SendAudioRequest.Caption`, tag JSON
`Caption,omitempty`), consumido — ou melhor, **não** consumido — por
`pkg/application/usecase/message/send_audio.go`.

**Problema**: a rota `POST /chat/send/audio` aceita `Caption` no payload
público, mas `waE2E.AudioMessage` **não possui campo Caption**. Não é um caso
de "esquecemos de ligar": não existe onde ligar. O campo atravessa a fronteira
HTTP, é decodificado, e morre ali. O handler histórico (`41bc8e2^:handlers.go`)
também não o enviava — ele montava `AudioMessage` sem qualquer caption.

Isto é a mesma **classe** dos achados de `LinkPreview` (F-anterior, corrigido em
CAP-01.1) e do `MimeType` de imagem (corrigido em CAP-03), com uma diferença
importante: aqueles dois tinham representação protocolar e só não estavam
ligados. Este não tem. Por isso **não** foi corrigido em CAP-05 — não há
correção possível sem inventar comportamento.

**Correção sugerida**: é decisão de CONTRATO, não de implementação. Três
caminhos, nenhum trivial:
1. remover `Caption` do DTO de áudio — mudança de contrato público, quebra
   clientes que hoje o enviam (mesmo sem efeito);
2. documentar explicitamente como aceito-e-ignorado, o que ao menos torna a
   mentira honesta;
3. investigar se o WhatsApp representa legenda de áudio por outro mecanismo
   (mensagem de texto associada, `ContextInfo`), e se isso é desejável.

**PROIBIDO**: inventar uma forma de "enviar caption em áudio" empurrando o
texto para outro campo. Seria fabricar semântica que o protocolo não tem.

**Status**: **NÃO CORRIGIDO** — registrado como dívida de contrato por decisão
do ciclo. Não bloqueia nenhuma capability.

## F117 — `Waveform` sumiu da superfície pública de áudio, mas o histórico a enviava

**Data / contexto**: 2026-08-18, durante CAP-05. É o inverso exato da F116.

**Onde**: `pkg/domain/message.go` (`SendAudioRequest`, que **não** tem campo
`Waveform`) contra `git show 41bc8e2^:handlers.go`, onde o `audioStruct` tinha
o campo e ele era enviado:

```go
AudioMessage: &waE2E.AudioMessage{
    ...
    PTT:      &ptt,
    Seconds:  proto.Uint32(t.Seconds),
    Waveform: t.Waveform,
}
```

**Problema**: `waE2E.AudioMessage.Waveform` existe no protocolo e era
preenchido a partir do request. Quando o `handlers.go` de 232KB foi deletado
(`41bc8e2`), o DTO reconstruído perdeu o campo. Ou seja: a superfície atual é
**mais pobre** que a histórica, e nada registrava isso.

O waveform é a barrinha de amplitude que o WhatsApp desenha num voice note.
Sem ele, o cliente recebedor tende a exibir uma forma de onda genérica ou
achatada — a mensagem funciona, mas a fidelidade visual do voice note é menor.
Não medi esse efeito contra um aparelho real; a afirmação vem do protocolo e do
comentário histórico, não de observação.

**Relação com F115 e F114**: são a mesma família — metadata de fidelidade que a
reconstrução perdeu (thumbnail HQ do link preview, `JPEGThumbnail` de imagem, e
agora `Waveform` de áudio). Vale tratá-las num único pass de fidelidade em vez
de uma a uma.

**Correção sugerida**: reintroduzir `Waveform []byte` em `SendAudioRequest` e
repassá-lo ao `AudioPayload` até o adapter. É acréscimo de campo opcional,
portanto compatível para trás. Custo baixo; o motivo de não ter sido feito em
CAP-05 é escopo, não dificuldade.

**Status**: **NÃO CORRIGIDO** — CAP-05 tinha escopo fechado (ligar o envio real
e recuperar PTT/MIME), e ampliar a superfície pública não era parte dele.

## F118 — `SendVideoRequest` perdeu `MimeType` e `JPEGThumbnail` do contrato histórico

**Data / contexto**: 2026-08-18, durante CAP-06 (ligar o envio real de vídeo).
Registrado como follow-up documental, sem reabrir o slice.

**Onde**: `pkg/domain/message.go` (`SendVideoRequest`, hoje
`{Phone, Video, Caption, ID}`) contra o `imageStruct` que o handler de vídeo
usava em `git show 41bc8e2^:handlers.go` (linha ~1585):

```go
type imageStruct struct {
    Phone         string
    Video         string
    Caption       string
    Id            string
    JPEGThumbnail []byte   // <- perdido
    MimeType      string   // <- perdido
    ContextInfo   waE2E.ContextInfo
    QuotedMessage *waE2E.Message
}
```

**Problema**: são duas perdas com naturezas diferentes, e vale distingui-las.

**`MimeType`** — o suporte existe em *toda* a pilha, menos na porta de entrada.
A implementação de CAP-06 já traz a precedência histórica de dois níveis
(`req.MimeType` → senão `http.DetectContentType`), e o nível 1 está escrito e
testado. O que falta é o campo público: hoje nenhum cliente consegue
selecioná-lo, então o mimetype de vídeo vem **sempre** do sniffing dos bytes.
Ou seja:

> protocolo e aplicação suportam; a superfície pública não consegue escolher.

Reintroduzir o campo é acréscimo opcional, compatível para trás, e o nível 1
passa a funcionar sem tocar em mais nada.

**`JPEGThumbnail`** — o handler histórico aceitava a thumbnail **fornecida pelo
request** e a colocava em `VideoMessage.JPEGThumbnail`. Hoje o vídeo é enviado
sem preview. Repare no que isto **não** é: não é um pedido de gerar thumbnail.
O histórico nunca gerou — ele apenas repassava o que o cliente mandava. Não há
ffmpeg envolvido, não há probing de vídeo, e introduzir qualquer um dos dois
seria inventar comportamento que nunca existiu.

**Correção sugerida**: reintroduzir os dois campos em `SendVideoRequest` e
repassá-los pela seam já existente até o adapter. Custo baixo; o motivo de não
ter sido feito em CAP-06 é escopo — ampliar superfície pública não pertencia
àquele slice.

**Família**: esta é a quarta entrada da mesma causa. F114 (thumbnail HQ do link
preview), F115 (`JPEGThumbnail` de imagem), F117 (`Waveform` de áudio) e agora
F118 nasceram todas do mesmo evento: a deleção do `handlers.go` de 232KB em
`41bc8e2`, em que os DTOs foram reconstruídos e as implementações não. Vale
tratá-las num único pass de fidelidade, e não uma a uma — a decisão de adiar
esse pass foi consciente, para ganhar largura de capabilities primeiro.

**Status**: **NÃO CORRIGIDO** — dívida de contrato registrada, não trabalho
pendente deste ciclo.

## F119 — `SendStickerRequest` perdeu cinco campos, e a infra que os consome está inteira

**Data / contexto**: 2026-08-18, durante CAP-07 (ligar o envio real de sticker).
É a maior perda de superfície de um único contrato encontrada até aqui.

**Onde**: `pkg/domain/message.go` (`SendStickerRequest`, hoje
`{Phone, Sticker, ID, MimeType}`) contra o `stickerStruct` de
`git show 41bc8e2^:handlers.go` (linha ~1417):

```go
type stickerStruct struct {
    Phone         string
    Sticker       string
    Id            string
    PngThumbnail  []byte   // <- perdido
    MimeType      string
    PackId        string   // <- perdido
    PackName      string   // <- perdido
    PackPublisher string   // <- perdido
    Emojis        []string // <- perdido
    ContextInfo   waE2E.ContextInfo
    QuotedMessage *waE2E.Message
}
```

**Problema**: os cinco campos não estão apenas ausentes — eles são
**parâmetros de uma pipeline que continua existindo e funcionando** no
repositório. `pkg/infra/media/sticker.ProcessStickerData` tem esta assinatura:

```go
func ProcessStickerData(stickerData, mimeOverride, packID, packName,
    packPublisher string, emojis []string) ([]byte, string, error)
```

CAP-07 a religou passando `"", "", "", nil` nos quatro últimos. O sticker é
enviado, é WebP válido, e `EmbedStickerEXIF` roda — só que grava metadata de
pacote vazia. O resultado prático: o sticker chega, mas sem identidade de
pacote (nome, autor, emojis associados), e sem `PngThumbnail`.

O que torna esta entrada diferente de F117 e F118: ali faltava o campo E o
consumo. Aqui o consumo está pronto, testado (`exif_test.go`, 23KB) e
conectado — falta só a porta de entrada. É a dívida de menor custo de fechar
das quatro.

**Correção sugerida**: reintroduzir os cinco campos em `SendStickerRequest` e
repassá-los pelo `StickerProcessor` até `ProcessStickerData`. São acréscimos
opcionais, compatíveis para trás, e o pipeline não muda em nada — apenas para
de receber string vazia.

**Família**: quinta entrada da mesma causa, junto de F114 (thumbnail HQ do link
preview), F115 (`JPEGThumbnail` de imagem), F117 (`Waveform` de áudio) e F118
(`MimeType`/`JPEGThumbnail` de vídeo). Todas nasceram da deleção do
`handlers.go` de 232KB em `41bc8e2`, quando os DTOs foram reconstruídos e as
implementações não. Somam **doze campos públicos** perdidos.

**Status**: **NÃO CORRIGIDO** — dívida de contrato. O pass de fidelidade que
trata a família inteira foi adiado conscientemente para ganhar largura de
capabilities primeiro.

## F120 — `SendMessageHandler` (texto) loga `user_id` (o Id de sessão) nos
ramos de erro; os cinco handlers de mídia não logam nada equivalente

**Data**: 2026-08-18. **Contexto**: FIX-07b, restaurando o eixo de
no-secret-leak perdido pela deleção de `handler_media_test.go` (achado do
Chief na revisão de stage do CAP-07).

**Onde**: `pkg/presentation/http/handlers/handler_message_send.go:51,61` —
```go
hlog.FromRequest(r).Warn().Err(err).
    Str("path", r.URL.Path).
    Str("user_id", txtID).
    Msg("...")
```
Comparar com os seis handlers de mídia (`handler_media.go`,
`handler_media_ext.go`), que nos mesmos ramos de erro só logam `path`, nunca
o Id de sessão.

**Problema**: ao escrever `TestSendText_NoSecretLeak` reusando a técnica do
teste deletado (plantar um dos três segredos da F9.4 como Id de sessão via
`withUser`), o teste falhou de verdade — `user_id` carregava o valor
plantado. Investigando: não é uma reincidência da F9.4 (o Id de sessão não é
`admin_token`/`global_encryption_key`/`global_hmac_key`, e nenhum dos três
pode legitimamente coincidir com um Id de sessão gerado pelo sistema), mas é
uma inconsistência real entre os seis handlers de envio — um loga o
identificador de sessão do chamador em toda falha, os outros cinco não. Um
Id de sessão em log é uma superfície de correlação/hijacking mais branda que
um segredo global, mas ainda assim uma escolha que os outros cinco handlers
deliberadamente não fazem.

Por isso o teste final (`TestSendText_NoSecretLeak` e as outras cinco
variações, em `handler_*_send_test.go`) planta os três segredos em
`Phone`/campo de mídia/header `Authorization`, e usa um Id de sessão NÃO
secreto (`"no-secret-leak-session"`) — o que é fiel à produção, onde o Id de
sessão nunca é um dos três segredos globais.

**Correção sugerida**: decidir, conscientemente, se `user_id` deveria ou não
aparecer no log de erro do handler de texto — e se sim, replicar a mesma
decisão nos outros cinco (paridade), ou removê-la de texto para alinhar com
o resto. Não é urgente (não é vazamento dos três segredos da F9.4), mas é
divergência de comportamento entre handlers irmãos que deveria ser
deliberada, não acidental.

**Status**: **NÃO CORRIGIDO** — fora do escopo do FIX-07b (só teste). Fica
pendente de decisão do usuário sobre se `user_id` deve ou não ser logado.

## F121 — `SendLocationRequest.Latitude`/`Longitude` == 0 é indistinguível de
"campo ausente"; um ponto sobre o equador ou o meridiano de Greenwich é
inendereçável pela API

**Data**: 2026-08-18. **Contexto**: CAP-08A, POST /chat/send/location
passou a montar o LocationMessage e enviar de verdade.

**Onde**: `pkg/application/usecase/message/send_location.go`, validação de
`SendLocationUseCase.Execute`:
```go
if req.Latitude == 0 {
    return nil, apperr.New("missing_latitude", apperr.CategoryValidation, "missing Latitude in payload", false, nil)
}
if req.Longitude == 0 {
    return nil, apperr.New("missing_longitude", apperr.CategoryValidation, "missing Longitude in payload", false, nil)
}
```
Comportamento HISTÓRICO idêntico — `git show 41bc8e2^:handlers.go`, em
torno da linha 1913 (`if t.Latitude == 0 { ... "missing Latitude in
Payload" }`).

**Problema**: `domain.SendLocationRequest.Latitude`/`Longitude` são
`float64` sem ponteiro. Na desserialização JSON, um campo ausente e um
campo explicitamente enviado como `0` produzem o MESMO valor Go (`0.0`),
então a validação `== 0` não consegue diferenciar "o cliente esqueceu de
mandar Latitude" de "o cliente mandou Latitude=0 de propósito". Consequência
concreta, não hipotética: qualquer ponto EXATAMENTE sobre o equador
(latitude 0) ou sobre o meridiano de Greenwich (longitude 0) é rejeitado com
400 `missing_latitude`/`missing_longitude`, mesmo sendo coordenadas
geograficamente válidas. Não é um caso de borda raro tipo "Null Island"
(0,0) — são duas linhas INTEIRAS do globo (todo o equador, todo o
meridiano), qualquer ponto sobre qualquer uma delas.

Reproduzido e travado em teste (`TestSendLocation_ZeroLatitudeRejected` e
`TestSendLocation_ZeroLongitudeRejected`,
`pkg/application/usecase/message/send_location_test.go`), e pela rota
registrada (`TestSendLocation_ZeroLatitudeOrLongitude_Rejected_
ViaRegisteredRoute`, `pkg/presentation/http/handlers/
handler_send_location_test.go`) — os dois DOCUMENTAM o comportamento atual
(400 para lat/lon zero), não o comportamento desejado.

**Correção sugerida**: trocar `Latitude`/`Longitude` de `float64` para
`*float64` em `domain.SendLocationRequest`, validando `== nil` em vez de
`== 0`. Isto é MUDANÇA DE CONTRATO PÚBLICO (o corpo JSON aceito não muda,
mas o comportamento de validação muda — um payload com `Latitude: 0` que
hoje é 400 passaria a ser aceito), por isso não foi decidida nem aplicada
nesta sessão — fica para decisão consciente do usuário, com o trade-off
registrado aqui.

**Status**: **NÃO CORRIGIDO** — comportamento histórico preservado de
propósito (fora do escopo autorizado do CAP-08A: corrigir contrato público
não é decisão do executor). Pendente de decisão do usuário.

---

## F122 — a guarda `missing session id` de `SendContactHandler`/
`SendLocationHandler` ficou sem NENHUM teste depois da migração CAP-08A/08B;
o comentário de `handler_interactive_test.go` afirma o contrário

**Data**: 2026-08-18. **Contexto**: FIX-08, restaurando o eixo CORPO
MALFORMADO que a avaliação EVAL-08 apontou como perdido nas duas ServeHTTP.
Ao recontar a cobertura depois do conserto, o eixo restaurado explicou
apenas PARTE da queda — sobrou um segundo bloco descoberto, que o packet
do FIX-08 não tinha diagnosticado.

**Onde**: `pkg/presentation/http/handlers/handler_interactive.go:38-43`
(SendContact) e `:83-88` (SendLocation) — o mesmo bloco nos dois:
```go
txtID := info.Get("Id")
if txtID == "" {
    hlog.FromRequest(r).Warn().Err(errMissingSessionID).Str("route", route).Msg("request rejected")
    customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
    return
}
```
A afirmação falsa está em `handler_interactive_test.go:96-103`: o comentário
diz que os eixos "unauthorized, missing session id, malformed body, ..."
foram *realocados* para `handler_send_location_test.go` e
`handler_send_contact_test.go`, e que "Nenhum eixo foi removido, só
realocado". `missing session id` NÃO foi realocado — não existe nenhum
teste dele nos dois arquivos novos.

**Problema**: o bloco não é exercitado por nenhum teste. Evidência medida
nesta sessão, com os dois testes do FIX-08 já no lugar:
```
$ go tool cover -func=/tmp/fix08.cov | grep -E 'handler_interactive.go:(28|73)'
handler_interactive.go:28:  ServeHTTP    86.4%
handler_interactive.go:73:  ServeHTTP    86.4%
$ grep handler_interactive.go /tmp/fix08.cov | grep ' 0$'
handler_interactive.go:39.17,43.3 3 0
handler_interactive.go:84.17,88.3 3 0
```
Ou seja: 22 statements por função, 16 cobertos antes do FIX-08 (72,7%), 19
depois (86,4%), e os 3 que faltam em cada uma são exatamente esse bloco. É
por isso que a cobertura NÃO volta aos 100,0% que o EVAL-08 mediu antes da
migração — o eixo `missing session id` é a outra metade da queda.

Consequência prática: uma sessão autenticada mas sem `Id` (o caso que o
bloco existe para tratar) devolveria hoje 400 por acidente do use case
(`missing_phone` etc.) ou 500, e nenhum teste notaria — é o mesmo padrão da
ARMADILHA 2 deste repo, guarda sem cobertura pela rota registrada.

**Correção sugerida**: acrescentar
`TestSendLocation_MissingSessionID_ViaRegisteredRoute` e
`TestSendContact_MissingSessionID_ViaRegisteredRoute` nos dois arquivos, no
mesmo padrão do teste de corpo malformado desta sessão: rota gorilla/mux
REGISTRADA (helpers `sendLocationServe`/`sendContactServe`), requisição
autenticada com `Id` VAZIO (`withUser(r, "")`), `assertErrorEnvelope(...,
http.StatusBadRequest)`, causa travada no log via `logassert.OutcomeLogged
(t, recs, "missing session id")` — sem a asserção da CAUSA o teste não morde,
porque o use case também produz 400 (foi o que o controle negativo do
FIX-08 mostrou) — e `len(sm.SendXxxCalls) == 0`. Isso leva as duas ServeHTTP
de volta a 100,0%. Corrigir junto o comentário de
`handler_interactive_test.go:96-103`, que hoje é falso.

**Status**: **CORRIGIDO** nesta mesma sessão (FIX-08, segundo dispatch, depois
da autorização explícita do coordenador — o primeiro `ask`, thread
`msg_aadf364a3add`, tinha expirado em 900s sem resposta, e por isso nada
havia sido corrigido de graça).

Travado por `TestSendLocation_MissingSessionID_ViaRegisteredRoute`
(`pkg/presentation/http/handlers/handler_send_location_test.go`) e
`TestSendContact_MissingSessionID_ViaRegisteredRoute`
(`pkg/presentation/http/handlers/handler_send_contact_test.go`): rota
gorilla/mux REGISTRADA, requisição AUTENTICADA com `Id` vazio, corpo VÁLIDO
de propósito (se a guarda não disparar, nada mais impede o envio),
`assertErrorEnvelope(..., 400)`, `len(sm.SendXxxCalls) == 0` e a CAUSA
travada por `logassert.OutcomeLogged(t, recs, "missing session id")`.

Controle negativo EXECUTADO nos dois (`if txtID == "" && false {`, edição de
uma linha, revertida por edição localizada):
```
--- FAIL: TestSendLocation_MissingSessionID_ViaRegisteredRoute (0.00s)
    handler_send_location_test.go:330: status: got 200, want 400 (corpo: {"code":200,"data":{"message_id":"sent-location-message-id","timestamp":-62135596800,"status":"sent"},"success":true})
--- FAIL: TestSendContact_MissingSessionID_ViaRegisteredRoute (0.00s)
    handler_send_contact_test.go:301: status: got 200, want 400 (corpo: {"code":200,"data":{"message_id":"sent-contact-message-id","timestamp":-62135596800,"status":"sent"},"success":true})
```
A mutação não produz um 400 por outra causa: ela deixa a requisição SEM
session id chegar até a porta e devolver 200 com mensagem enviada — que é
exatamente o defeito que a guarda existe para impedir.

Cobertura das duas `ServeHTTP` de volta a **100,0%** (de 72,7% antes do
FIX-08 e 86,4% depois de restaurado só o eixo de corpo malformado), sem
nenhum bloco descoberto restante em `handler_interactive.go` nas duas
funções.

**Auditoria do comentário inteiro** (terceiro dispatch do FIX-08): o
comentário de `handler_interactive_test.go:96-103` afirmava OITO eixos
realocados. Auditados um a um, **três** não tinham vindo junto — os dois
acima mais `wrong type in context` (contexto com valor que não satisfaz
`userInfo` tem de virar 401, não pânico) — e um quarto, `ausência de log no
caminho feliz`, também não existia nos arquivos novos. Os quatro passaram a
existir:

- `TestSendXxx_WrongTypeInContext_ViaRegisteredRoute`. Controle negativo
  EXECUTADO (asserção de duas variáveis trocada pela de uma, `info :=
  ...(userInfo)`, que COMPILA — `go build` OK — e falha):
  `panic: interface conversion: int is not handlers.userInfo: missing method
  Get`, em `handler_interactive.go:76` (Location) e `:31` (Contact). É o
  pânico exato que o eixo existe para impedir.
- `TestSendXxx_SuccessEmitsNoOutcomeLog`. Controle negativo EXECUTADO (o
  handler passa a logar todo request servido em warn com campo `error`):
  `caminho de sucesso emitiu registro de erro: {"level":"warn",...,"error":
  "unauthorized","route":"/chat/send/contact","message":"request served"}` e
  o equivalente para `/chat/send/location`. É o Cenário 2 da Fase 12 — um
  handler que loga tudo passa em TODA asserção de caminho de erro e ainda
  assim é ruído.

Uma premissa do diagnóstico inicial estava errada e fica corrigida aqui:
supôs-se que os eixos de log não pudessem ser exercitados nos arquivos novos
porque os helpers usam `silentLogger{}`. `silentLogger` é o logger do USE
CASE; o registro do handler sai por `hlog.FromRequest`, que
`logassert.Wrap` captura. Por isso `no-secret-leak` já estava de fato
realocado (`TestSendXxx_NoSecretLeak`) e `ausência de log no caminho feliz`
pôde ser realocado de verdade, sem trocar o dublê.

O comentário foi reescrito para listar os oito eixos **nome por nome**, com
o teste de destino de cada um — verificável por `grep`, em vez de uma
afirmação em bloco.

**Diferença residual fechada** (quarto dispatch do FIX-08): os destinos de
`session failure` e `campo obrigatório ausente` asseveravam status e porta
intocada, mas não a CAUSA no log (co-gate D), que a tabela original exigia
nos dois. Os quatro testes (`TestSendXxx_SessionFailure` e
`TestSendXxx_RejectMissingRequiredField`) passaram a usar o helper novo
`sendXxxServeCapturingLog` — o `sendXxxServe` acrescido da saída de log — e
a asseverar `logassert.OutcomeLogged` com a causa: o token sentinela do
arquivo para a falha de sessão, e a causa por campo (`missing Phone/
Latitude/Longitude in payload`, `missing Phone/Name/Vcard in payload`) na
tabela de campo obrigatório, que virou `map[string]struct{ body, cause }`.

Controle negativo EXECUTADO, desenhado para ISOLAR a asserção nova — a
mutação preserva o status e troca só a causa registrada
(`Error().Err(err)` -> `Err(errUnauthorized)` em `handler_interactive.go:54`
e `:99`), de modo que a checagem de status antiga continua passando e só o
co-gate D morde. Compila (`go build` OK) e falha nas oito sub-asserções:
```
--- FAIL: TestSendContact_RejectMissingRequiredField/Phone
    co-gate D: campo `error` ("unauthorized") nao contem "missing Phone in payload"
--- FAIL: TestSendContact_RejectMissingRequiredField/Name
    co-gate D: campo `error` ("unauthorized") nao contem "missing Name in payload"
--- FAIL: TestSendContact_RejectMissingRequiredField/Vcard
    co-gate D: campo `error` ("unauthorized") nao contem "missing Vcard in payload"
--- FAIL: TestSendContact_SessionFailure
    co-gate D: campo `error` ("unauthorized") nao contem "send-contact-sentinel-cause-4e8a2b"
--- FAIL: TestSendLocation_RejectMissingRequiredField/{Phone,Latitude,Longitude}
    co-gate D: campo `error` ("unauthorized") nao contem "missing {Phone,Latitude,Longitude} in payload"
--- FAIL: TestSendLocation_SessionFailure
    co-gate D: campo `error` ("unauthorized") nao contem "send-location-sentinel-cause-7f3c1d"
```
Os oito eixos ficam preservados — mas **não** "sem ressalva", que foi como
esta entrada e o comentário afirmaram por um tempo. A medição: **cinco** dos
oito destinos asseveram também a CAUSA no log (`missing session id`,
`malformed body`, `campo obrigatório ausente`, `session failure`, `wrong type
in context`). Os outros três não asseveram, e por motivos diferentes:

- `no-secret-leak` e `ausência de log no caminho feliz` asseveram AUSÊNCIA
  (nenhum segredo em nenhum registro; nenhum registro de erro num 200), e por
  isso não comportam `logassert.OutcomeLogged`, que exige a PRESENÇA de um
  registro de saída. Não é lacuna: é o eixo oposto.
- `unauthorized` (`TestSendXxx_RejectUnauthenticated`) assevera só status e
  porta intocada. A causa desse ramo fica travada por
  `TestSendXxx_WrongTypeInContext_ViaRegisteredRoute`, porque a guarda é UMA
  só (`if !ok || info == nil`, `handler_interactive.go:32` e `:77`) e emite
  UMA linha de log — os dois casos caem no mesmo ramo.

**Como o erro apareceu**: pela RE-EVAL-08, que reprovou as duas capabilities
por este único ponto — o comentário afirmava "os oito destinos asseveram
também a CAUSA no log ... não há diferença residual", e são cinco. A prova
não é leitura, é o controle negativo NC-2 do avaliador: corrompida a causa do
ramo `unauthorized`, `TestSendXxx_RejectUnauthenticated` **PASSOU** e só
`WrongTypeInContext_ViaRegisteredRoute` mordeu. Evidência medida, não
opinião — e exatamente o padrão que o teste do co-gate D existe para expor.

O comentário de `handler_interactive_test.go` (hoje 96-140) foi corrigido
para dizer isto, incluindo que só DOIS destinos usam o helper
`sendXxxServeCapturingLog` (`campo obrigatório ausente` e `session failure`)
enquanto os demais montam `logassert.Wrap` inline — outra imprecisão da mesma
rodada. Esta entrada e aquele comentário estão alinhados: nenhum dos dois
afirma mais equivalência total entre os oito destinos e a tabela original.

A lição, que é o motivo de estar registrada aqui e não só no comentário: as
duas afirmações falsas desta sessão ("nenhum eixo foi removido, só realocado"
e "os oito destinos asseveram a causa") eram do MESMO tipo — resumo em bloco
de um conjunto, escrito sem enumerar o conjunto. As duas passaram por testes
verdes e por `make check`. O que as pegou foi contar item a item; o que as
produziu foi descrever em vez de contar.

## F123

**Data**: 2026-08-18. **Contexto**: CURRENT_STATE do CAP-09A (Chat Read
Path), antes de qualquer implementação.

**Onde**: `pkg/domain/entities.go:44` e `pkg/infra/db/connection.go:144`.

**Problema**: existem DOIS tipos chamados `HistoryMessage`, com formas
DIFERENTES, e o órfão é o que "parece certo".

- `db.HistoryMessage` é o que o contrato público sempre serializou:
  `id, user_id, chat_jid, sender_jid, message_id, timestamp, message_type,
  text_content, media_link, quoted_message_id, data_json`. Tem uso real
  (`pkg/bootstrap/wiring_delegates.go:26` faz `type HistoryMessage =
  db.HistoryMessage`).
- `domain.HistoryMessage` tem ZERO usos e uma forma que nunca existiu no
  wire: `id, user_id, jid, from, body, timestamp, direction, media_url,
  status`.

Evidência de que o do `db` é o histórico:
`git show 3dafae0:handlers.go` (linhas 5012+) faz
`s.db.Select(&messages, query, ...)` sobre exatamente essas colunas e
serializa o slice direto na resposta.

**Por que isto é armadilha e não só duplicação**: quem for implementar a
leitura de histórico vai procurar um tipo no `domain` — é onde a arquitetura
hexagonal manda olhar — encontrar `domain.HistoryMessage`, e usá-lo por
parecer a escolha arquitetural correta. O resultado seria mudança silenciosa
de contrato público: `text_content` viraria `body`, `chat_jid` viraria `jid`,
`sender_jid` viraria `from`, e apareceriam `direction` e `status` que nunca
existiram. Nenhum teste atual pegaria isso, porque não há teste de contrato
da rota — ela devolve stub.

**Correção sugerida**: apagar `domain.HistoryMessage` (não tem uso), ou, se
houver intenção de futuramente promovê-lo, documentar no próprio tipo que ele
NÃO é o formato do wire e apontar para `db.HistoryMessage`. A decisão está com
o Orchestrator.

**Status**: não corrigido — e o CAP-09A NÃO caiu na armadilha: a
implementação criou uma representação própria na fronteira de application
(`appport.ChatHistoryMessage`, `pkg/application/contracts/chat_history_port.go`),
com os MESMOS campos e as MESMAS tags JSON de `db.HistoryMessage`, e o
adapter de persistência mapeia campo a campo. `domain.HistoryMessage` continua
órfão e continua candidato a cleanup explícito — apagá-lo é decisão do
Orchestrator, não trabalho de graça do CAP-09A.

## F124

**Data**: 2026-08-18. **Contexto**: mesmo CURRENT_STATE.

**Onde**: `pkg/bootstrap/wiring_routes.go:129` e `:197`.

```go
registry.Register("/webhook/history", customChain.Then(ch.Storage.GetHistory), "GET")
registry.Register("/chat/history",    customChain.Then(ch.Storage.GetHistory), "GET")
```

**Problema**: duas rotas de significado público diferente compartilham o
mesmo handler, e o nome do handler (`Storage.GetHistory`) descreve a rota
ERRADA. `Storage.GetHistory` nunca foi leitor de configuração de webhook: veio
do commit `3dafae0` ("Implement message history logging"), que criou a tabela
`message_history` e o dashboard de histórico. É o leitor de histórico de
MENSAGENS, com nome de configuração.

Importa registrar que **isto não é regressão da migração**: o
`custom_routes.go` histórico já fazia igual (linha 118 para
`/webhook/history`, linha 170 para `/chat/history`). A migração reproduziu o
acoplamento fielmente. Verificável por
`git show 41bc8e2^:custom_routes.go | grep -n history`.

**Correção sugerida**: quando o CAP-09A separar as duas semânticas, deixar um
teste ESTRUTURAL provando que as duas rotas não voltam a apontar para o mesmo
handler por engano — um teste sobre o registro de rotas, não sobre resposta,
porque o defeito é de fiação e sobreviveria a qualquer teste de payload.

**Status**: CORRIGIDO no CAP-09A (2026-08-18). `/chat/history` passou a
apontar para `ch.ChatHistory.GetChatHistory`
(`pkg/presentation/http/handlers/handler_chat_history.go`), e
`/webhook/history` continua em `ch.Storage.GetHistory`, inalterado.

Testes que travam o achado:

- `TestChatHistoryAndWebhookHistoryAreDistinctHandlers`
  (`pkg/bootstrap/chat_history_route_test.go`) — teste ESTRUTURAL de fiação:
  monta o roteador REAL via `registerCustomRoutes` e prova que cada rota
  devolve algo que só o SEU handler sabe produzir (`/chat/history`, mensagem
  vinda do banco; `/webhook/history`, o literal
  `History configuration retrieved`). Identidade de ponteiro não serviria: a
  chain de middleware embrulha os dois handlers, e dois wrappers distintos
  comparam diferente mesmo quando o handler embrulhado é o mesmo.

Efeito colateral obrigatório da rota nova, registrado porque custou duas
falhas de build: `emptyCustomHandlers` (`pkg/bootstrap/router.go:129`) e
`newRouterForRouteCheck` (`pkg/bootstrap/stdio_route_consistency_test.go`)
precisam ganhar o grupo `ChatHistory`; sem isso `registerCustomRoutes`
desreferencia nil e `TestBoundaryLog`, `TestStdioRoutesMatchRegisteredHTTPRoutes`
e `go run ./cmd/listroutes` morrem com SIGSEGV. **Grupo de handler novo exige
atualizar os DOIS conjuntos vazios junto com a rota.**

## F125

**Data**: 2026-08-18. **Contexto**: CURRENT_STATE do CAP-09A, antes de
implementar a recuperação de `GET /chat/history`.

**Onde**: histórico, `git show 3dafae0:handlers.go`, linhas 5053-5080 —
ramo `chat_jid=index` do handler `GetHistory`.

**Problema**: vazamento de dados entre tenants no contrato histórico.

```go
// If chat_jid is "index", return mapping of all instances to their chat_jids
query = `SELECT user_id, chat_jid, MAX(timestamp) as last_message_time
         FROM message_history
         GROUP BY user_id, chat_jid
         ORDER BY user_id, last_message_time DESC`
...
err := s.db.Select(&mappings, query)
```

Sem `WHERE`, sem argumentos. Qualquer usuário autenticado recebia o mapa de
chats de TODOS os usuários — JIDs de conversa de outros tenants com o
timestamp da última mensagem. O comentário do autor original diz "all
instances", então era intencional, não descuido de digitação.

Não é padrão do sistema: no MESMO handler, o ramo de mensagens é escopado
(`WHERE user_id = $1 AND chat_jid = $2`). A falha é só do ramo `index`.

**Decisão (Orchestrator, 2026-08-18)**: a reconstrução do CAP-09A
**deliberadamente NÃO preserva** esse comportamento. Precedência declarada:
isolamento de tenant / fail-closed **acima de** compatibilidade com
comportamento historicamente inseguro. Um consumidor que dependia de
enumerar JIDs de outros usuários dependia de uma violação de isolamento.

Os dois comportamentos, enumerados:

- **HISTÓRICO**: query sem `WHERE user_id`; múltiplas chaves `user_id`
  possíveis na resposta.
- **ATUAL**: `WHERE user_id = <caller>`; no máximo a chave do caller.

O wire shape `map[user_id][]ChatInfo` é PRESERVADO; só o conteúdo é
restringido. O isolamento tem de estar **na query**, não em filtragem em
memória depois de uma consulta global — a filtragem tardia deixa o dado
passar por log, tracing e mapeamento, e não sobrevive a refactor.

**Proibido explicitamente**: escrever teste que assevere o vazamento. O
código histórico é evidência arqueológica, não contrato a restaurar.

**Status**: CORRIGIDO no CAP-09A (2026-08-18). O isolamento está NA QUERY, em
`ChatHistoryRepository.ChatIndexByUser`
(`pkg/infra/db/chat_history_repository.go`), com `WHERE user_id = ?`.

Os quatro casos, travados em teste contra SQLite REAL com o schema de produção
(fake de repositório não serve: ele não tem `WHERE` para esquecer):

1. tenant A e B com histórico → só a chave A —
   `TestChatHistoryRepositoryChatIndexByUser_OnlyTheCallerKey` e
   `TestChatHistoryRoute_IndexReturnsOnlyCallerTenant` (20 voltas: ordem de
   mapa em Go é aleatória por desenho).
2. `chat_jid` coincidente entre tenants → nada de B aparece, e o
   `MAX(timestamp)` de B não contamina o de A —
   `TestChatHistoryRepositoryChatIndexByUser_SameChatJIDAcrossTenants` e
   `TestChatHistoryRoute_IndexIsolatesOnUserIDNotChatJID`.
3. A sem histórico, B com histórico → mapa vazio para A —
   `TestChatHistoryRepositoryChatIndexByUser_EmptyMapWhenCallerHasNoHistory` e
   `TestChatHistoryRoute_IndexEmptyMapWhenCallerHasNoHistory`.
4. identidade ausente → 401 pelo mecanismo canônico dos handlers, sem corpo de
   tenant nenhum — `TestChatHistoryRoute_MissingIdentityIsRejected` (e
   `TestChatHistoryRoute_MissingSessionIDIsRejected` para o id de sessão
   vazio).

**Controle negativo EXECUTADO** (remoção do `WHERE user_id = ?` do ramo
`index`): seis testes falham, e a saída mostra o vazamento textualmente —

```
--- FAIL: TestChatHistoryRepositoryChatIndexByUser_OnlyTheCallerKey (0.01s)
    chat_history_repository_test.go:177: volta 0: chaves = 2 ([B A]), quero SO' a chave do caller
--- FAIL: TestChatHistoryRepositoryChatIndexByUser_SameChatJIDAcrossTenants (0.01s)
    chat_history_repository_test.go:217: got = map[A:[{mesmo@s.whatsapp.net 2026-08-18T12:00:00Z}] B:[{mesmo@s.whatsapp.net 2026-08-18T22:00:00Z}]], quero exatamente um chat sob a chave A
--- FAIL: TestChatHistoryRepositoryChatIndexByUser_EmptyMapWhenCallerHasNoHistory (0.01s)
    chat_history_repository_test.go:242: got = map[B:[{b1@s.whatsapp.net 2026-08-18T12:00:00Z}]], quero mapa vazio — nenhum dado de B pode aparecer para A
--- FAIL: TestChatHistoryRoute_IndexReturnsOnlyCallerTenant (0.01s)
    chat_history_route_test.go:284: volta 0: dados do tenant B na resposta de A: {"code":200,"data":{"A":[{"chat_jid":"a1@s.whatsapp.net","last_updated":"2026-08-18T12:00:00Z"}],"B":[{"chat_jid":"b1@s.whatsapp.net","last_updated":"2026-08-18T13:00:00Z"}]},"success":true}
--- FAIL: TestChatHistoryRoute_IndexIsolatesOnUserIDNotChatJID (0.01s)
    chat_history_route_test.go:316: index = map[A:[{mesmo@s.whatsapp.net 2026-08-18T12:00:00Z}] B:[{mesmo@s.whatsapp.net 2026-08-18T22:00:00Z}]], quero exatamente um chat sob a chave A
--- FAIL: TestChatHistoryRoute_IndexEmptyMapWhenCallerHasNoHistory (0.01s)
    chat_history_route_test.go:338: dados do tenant B na resposta de A: {"code":200,"data":{"B":[{"chat_jid":"b1@s.whatsapp.net","last_updated":"2026-08-18T12:00:00Z"}]},"success":true}
```

Nota de método: a primeira tentativa da mutação removeu o `WHERE` E o
argumento, e o pacote deixou de COMPILAR (`declared and not used: query`) —
controle negativo que não compila não prova nada (ARMADILHA 3). Foi ajustada
até compilar E falhar. Reversão por edição localizada; árvore restaurada
conferida por `shasum -a 256` idêntico ao de antes da mutação.

## F126 — download com zero byte e erro nil: o histórico respondia 200 com Data URL vazia

**Data**: 2026-08-18
**Contexto**: CAP-09B — habilitar as cinco capabilities de download
(Download Image, Download Video, Download Audio, Download Document,
Download Sticker), que até então devolviam `&domain.DownloadResult{}` vazio.

**Onde**:
- histórico: `git show 41bc8e2^:handlers.go`, `DownloadImage` na linha 3836
  (e as quatro funções irmãs), no trecho
  `dataURL := dataurl.New(imgdata, mimetype)` seguido de
  `s.Respond(w, r, http.StatusOK, ...)`;
- hoje: `pkg/application/usecase/message/download_media.go:74`
  (`if len(data) == 0`).

**Problema**: `imgdata` é declarado `var imgdata []byte` e só é preenchido
dentro do `if img != nil`. Com `Client.Download` devolvendo `([]byte{}, nil)`,
`dataurl.New(nil, "image/jpeg").String()` produz `"data:image/jpeg;base64,"` —
uma Data URL sintaticamente válida e sem conteúdo — e a resposta sai **200**.
O cliente não tem como distinguir isso de mídia legítima de zero byte, e o
contrato público da rota promete conteúdo.

Investigação da primitive (não suposição): em
`internal/wa-noise/capabilities/media/download_transport.go`,
`DownloadAndDecrypt` só devolve `(data, nil)` depois de `ValidateMedia` e
`cbcutil.Decrypt`; o único caminho que produz zero byte sem erro é o ramo de
mídia **não cifrada** (`mediaKey == nil && fileEncSHA256 == nil && mac == nil`)
com corpo de resposta vazio. Nenhum download de `/chat/download*` passa por
esse ramo, porque todos carregam `MediaKey`. Ou seja: o caso é alcançável pela
assinatura, e patológico na prática.

**Correção aplicada (divergência CONSCIENTE do histórico)**: bytes vazios com
erro nil deixam de ser sucesso. `mediaDownloadFlow.execute` devolve
`apperr.New("empty_media", apperr.CategoryInternal, ...)` → **500**, e registra
`media download returned no bytes` em error. Não é conversão automática de
erro em sucesso nem o contrário: é a recusa de nomear "sucesso" uma resposta
sem conteúdo.

**Divergência registrada**: Baileys e Evolution API entregam o buffer como
veio, sem checar tamanho; a diferença aqui é deliberada e vale só na fronteira
HTTP do wa-api — o SDK vendorizado não foi tocado.

**Status**: **corrigido nesta sessão**, travado por teste em duas camadas:
- `pkg/application/usecase/message/download_media_test.go`,
  `TestDownloadUseCases_EmptyBytes_NotSuccess` (cinco capabilities, nome por
  nome);
- `pkg/presentation/http/handlers/handler_download_test.go`,
  `TestDownload_EmptyBytes_NotOK` (pela rota gorilla/mux REGISTRADA, exigindo
  500 e não 200).

Controle negativo executado no mesmo par de testes está registrado na entrada
F127 abaixo, junto do controle da Data URL.

## F127 — `/chat/download*` recusa payload que traga só `DirectPath`, embora a primitive o aceite

**Data**: 2026-08-18
**Contexto**: CAP-09B, achado incidental — **não corrigido**, por ser mudança
de contrato público fora do escopo do dispatch.

**Onde**: `pkg/application/usecase/message/download_media.go:41`
(`if req.URL == ""` → `missing_url`, 400), herdado literalmente dos cinco stubs
anteriores (`pkg/application/usecase/message/download_image.go` e irmãos, antes
deste commit).

**Problema**: a primitive do SDK trata `URL` e `DirectPath` como **caminhos
alternativos**, não como um obrigatório mais um opcional. Em
`internal/wa-noise/capabilities/media/download.go:79-91`, `DownloadMessage`
faz:

```go
url, isWebWhatsappNetURL := directURL(msg)
if len(url) > 0 && !isWebWhatsappNetURL {
    return DownloadAndDecrypt(...)
} else if len(msg.GetDirectPath()) > 0 {
    return DownloadWithPath(...)
}
```

Ou seja, um payload com `DirectPath`, `MediaKey`, `FileEncSHA256` e
`FileSHA256` e **sem** `Url` seria baixável — e é exatamente a forma em que os
metadados chegam em vários eventos de mensagem. O handler histórico
(`git show 41bc8e2^:handlers.go:3836`) **não validava `Url`**: montava o
protobuf com o que viesse e deixava o SDK decidir, então esta rejeição nasceu
com a migração para use case, não com o produto.

Também vale para `Url` apontando para `web.whatsapp.net`: o SDK ignora essa URL
(`isWebWhatsappNetURL`) e cai no ramo de `DirectPath` — nós aceitamos a
requisição por ela ser não-vazia, e ela funciona ou não conforme `DirectPath`
tenha vindo junto.

**Correção sugerida**: trocar a guarda por "pelo menos um entre `Url` e
`DirectPath`", mantendo 400 quando ambos faltarem. É relaxamento de validação
(nenhum payload hoje aceito passaria a ser recusado), mas ainda assim muda o
contrato observável da rota e merece decisão explícita.

**Status**: **não corrigido**. O comportamento atual está travado por teste
(`TestDownload_RejectMissingRequiredField` e
`TestDownloadUseCases_MissingURL_NoPortCall`, nas cinco capabilities), de modo
que a mudança, quando vier, será deliberada e não acidental.

## F128 — a invalidação do cache de userinfo do gate de History NÃO foi preservada

**Data**: 2026-08-18. **Contexto**: CAP-09A, recuperação de
`GET /chat/history`. Achado de lado, ao reconstruir o gate de History a partir
de `git show 3dafae0:handlers.go` (linha 5012+).

**Onde**: histórico, `handlers.go:5019-5024`; atual,
`pkg/application/usecase/chat/get_chat_history.go` (`Execute`, ramo
`cachedHistory == 0`).

**Problema**: o handler histórico fazia TRÊS coisas quando o `History` do
userinfo vinha 0, e a reconstrução preservou duas:

```go
userinfocache.Delete(token)                                   // (1) NÃO preservado
err := s.db.QueryRow("SELECT COALESCE(history, 0) FROM users WHERE id = $1", txtid)  // (2) preservado
if historyLimit == 0 { ...501... }                            // (3) preservado
```

O passo (1) apagava a entrada do cache, para que a PRÓXIMA requisição já
enxergasse o valor fresco em vez de repetir a revalidação. Sem ele, um usuário
que acabou de ligar o histórico paga uma consulta extra à tabela `users` em
cada requisição até o TTL do cache expirar (`userCacheTTL`,
`pkg/presentation/http/middleware/auth.go`). A CORRETUDE da requisição atual
não muda — a revalidação (2) resolve o valor antes de decidir —, só o custo.

Foi deixado de fora deliberadamente: o cache vive em `pkg/bootstrap` e é
chaveado por TOKEN, então invalidá-lo do use case exigiria uma porta nova
(`Invalidate(token)`) e levar o token — um segredo — até a camada de
aplicação só para apagar uma entrada de cache. A troca não se paga por uma
consulta a mais durante o TTL.

**Correção sugerida**: se a medição mostrar que a consulta extra importa,
invalidar no lado da ESCRITA (o caminho que altera `users.history`), não no da
leitura — lá o token já está em mãos e a invalidação acontece uma vez, não uma
vez por leitura.

**Status**: não corrigido, e é divergência CONSCIENTE do histórico, registrada
aqui para não ser redescoberta como bug. Os três estados do gate que importam
para o contrato estão travados em
`TestChatHistoryRoute_GateState*` (`pkg/bootstrap/chat_history_route_test.go`).

## F129 — chave duplicada no baseline DESATIVOU o piso de `func_coverage` do `log-coverage-gate`

**Data**: 2026-08-18.
**Contexto**: FIX-GATE, dispatch dedicado ao gate durante a sessão de CAP-09
(histórico de conversa / download unificado). Achado incidental ao conferir
por que `make check` vinha verde com o baseline de log em ratchet.

**Onde**: `.log-coverage-baseline`, DUAS atribuições da mesma chave, ambas
commitadas:

- linha 748 — `min_func_coverage=676` (entrou com FIX-07, commit de CAP-07)
- linha 768 — `min_func_coverage=674` (entrou com **6fa6270**, CAP-08A/08B)

O executor de CAP-08 **acrescentou** a chave em vez de **editar** a existente.

O leitor, em `Makefile` (alvo `log-coverage-gate`), era:

```make
min_func=$$(grep -oE '^min_func_coverage=[0-9]+' $(LOGCOV_BASELINE_FILE) | grep -oE '[0-9]+'); \
```

**Problema**: `grep -oE` devolve **todas** as ocorrências. Com a duplicata,
`$min_func` vira a string de duas linhas `"676\n674"`, e a comparação seguinte
`[ "$func_cov" -lt "$min_func" ]` aborta. O `if` do shell trata o **erro** do
`test` como **FALSO** — isto é, como "não caiu" — e o piso simplesmente
desaparece.

**Comando que reproduz** (rodado na raiz do worktree, antes da correção):

```sh
min_func=$(grep -oE '^min_func_coverage=[0-9]+' .log-coverage-baseline | grep -oE '[0-9]+')
func_cov=1
if [ "$func_cov" -lt "$min_func" ]; then echo FALHARIA; else echo PASSARIA; fi
```

**Saída observada**:

```
min_func=[676
674]
(eval):[:1: integer expression expected: 676\n674
PASSARIA
```

Com `func_cov=1` — cobertura de log absurdamente baixa — o gate ainda diz
PASSARIA. Esse é o tamanho do buraco.

**Desde quando**: desde **6fa6270** (`feat(message): habilita envio de
localizacao e de contato (CAP-08A/08B)`, 2026-08-18), até esta sessão do mesmo
dia. Nesse intervalo, **qualquer** regressão de `func_coverage` passaria sem
falhar, e o `EXIT:0` dos gates não significava nada para essa trava.
Passaram com o piso desativado, sem exceção: **EVAL-08**, **RE-EVAL-08**,
**CONFIRM-08** e os `make check` do Chief. As outras três travas
(`min_errpath_coverage`, `min_eligible`, `max_exempt_annotations`) continuaram
funcionando — a duplicata era só de `min_func_coverage` (confirmado com
`grep -oE '^[a-z_]+=' .log-coverage-baseline | sort | uniq -c`: `2` apenas para
`min_func_coverage`, `1` para todas as outras; `.coverage-baseline` tem
`min_coverage` uma única vez).

**Correção aplicada — frente 1 (dado)**: `.log-coverage-baseline` passa a ter
**uma** chave `min_func_coverage`, com o valor **medido**, não copiado.
Os quatro números, medidos nesta sessão e comparados no mesmo instante:

| | eligible | covered | func_coverage | décimos |
|---|---|---|---|---|
| HEAD 6fa6270 (árvore limpa, `git archive HEAD \| tar -x` em dir temporário) | 629 | 424 | 67.4086% | 674 |
| estado atual da árvore (CAP-09A + CAP-09B) | 632 | 425 | 67.2468% | 672 |

A razão CAI 674 → 672 por **denominador**: `covered` **SOBE** 424 → 425. O
conjunto de elegíveis teve 8 entradas e 5 saídas (net +3), enumerado nome por
nome no comentário da própria chave no `.log-coverage-baseline` (diff de
`logcov -golden`, coluna `ELIGIBLE`, HEAD vs atual). As duas funções novas sem
log L1 são `pkg/infra/wa-noise/adapters/chat.MediaDownloaderAdapter.Download` e
`pkg/infra/wa-noise/adapters/chat.downloadableFor`; a ausência de log nelas é a
convenção "adapter delegante não loga", confirmada por medição e não por
opinião — `grep -cE 'log\.|hlog\.|logger|Logger'
pkg/infra/wa-noise/adapters/chat/messenger.go` devolve **0**. Nenhum log foi
plantado para inflar métrica (COV-4; já foi REQUIRED_FIX nesta sessão, ver
F119/FIX-07).

**Correção aplicada — frente 2 (gate)**: `Makefile` ganha
`BASELINE_KEY_READER`, uma função de shell `baseline_key <arquivo> <chave>
[num]` que **falha alto** e **nomeia a chave** quando ela está duplicada,
ausente ou vazia (e, com `num`, quando o valor não é inteiro). Alvos
protegidos, nome por nome:

- **`log-coverage-gate`** — chaves `stage`, `min_func_coverage`,
  `min_errpath_coverage`, `min_eligible`, `max_exempt_annotations`
  (`.log-coverage-baseline`)
- **`coverage-gate`** — chave `min_coverage` (`.coverage-baseline`), que lia
  com exatamente o mesmo padrão `grep -oE '^chave=[0-9]+' | grep -oE '[0-9]+'`
  e tinha o mesmo buraco, ainda que sem duplicata hoje

**Correção aplicada — frente 3, achada ao verificar a 2**: existe um SEGUNDO
leitor do mesmo arquivo, em Go — `readBaselineForTest`
(`cmd/logcov/main_test.go:113`), usado por `TestBaselineBateComAMedicao`, que
existe justamente para exigir que os quatro números do baseline sejam os
medidos. Ele era duplicata-cego de outra forma: montava um `map[string]int` e
a segunda ocorrência **sobrescrevia** a primeira em silêncio ("vale a
última"). Por isso o teste passou em 6fa6270 — leu 674, o valor medido na
época — enquanto o gate lia as duas linhas e não comparava nada. Agora ele
rastreia as chaves já vistas e dá `t.Fatalf` nomeando a chave e as duas
linhas.

Nenhum piso foi baixado por conveniência e nenhuma exceção foi acrescentada —
a mudança só FECHA o gate (§120.6).

**Controles negativos, EXECUTADOS** (todos revertidos por edição localizada;
`diff` contra a cópia pré-mutação confirmou retorno idêntico após cada um):

(a) duplicata reintroduzida (`min_func_coverage=999` acrescentada ao fim):

```
log-coverage: estagio do gate = ratchet (ADR-008)
FALHA: a chave 'min_func_coverage' aparece 2 vezes em .log-coverage-baseline (linhas 814 1044 ).
       Chave DUPLICADA e' ambigua: o gate le por grep e receberia as duas linhas juntas,
       o que faria a comparacao numerica abortar e o piso desaparecer em silencio (F129).
       EDITE a chave existente em vez de acrescentar outra. Gate FALHA FECHADO.
make: *** [log-coverage-gate] Error 1
```

(b) piso mordendo de novo (chave única, piso subido 672 → 673) — prova que a
comparação voltou a acontecer de verdade:

```
func_coverage    = 672 decimos de % (piso 673)
errpath_coverage = 858 decimos de % (piso 854)
eligible         = 632 (piso exato 629)
exempt           = 0 (teto 0)
FALHA: func_coverage caiu (672 < 673).
...
NOTA: estagio ratchet — regressao encontrada, gate FALHA FECHADO.
make: *** [log-coverage-gate] Error 1
```

(c) chave vazia (`min_func_coverage=`):

```
log-coverage: estagio do gate = ratchet (ADR-008)
FALHA: a chave 'min_func_coverage' em .log-coverage-baseline esta' VAZIA. Gate FALHA FECHADO.
make: *** [log-coverage-gate] Error 1
```

(c2) chave removida por completo:

```
log-coverage: estagio do gate = ratchet (ADR-008)
FALHA: a chave 'min_func_coverage' nao existe em .log-coverage-baseline. Gate FALHA FECHADO:
       chave ausente nao e' licenca para passar.
make: *** [log-coverage-gate] Error 1
```

(d) duplicata reintroduzida contra o leitor Go (frente 3):

```
--- FAIL: TestBaselineBateComAMedicao (1.65s)
    main_test.go:85: chave "min_func_coverage" duplicada em ../../.log-coverage-baseline (linhas 814 e 1044): chave ambigua desativa o gate em silencio (F129)
FAIL
FAIL	wa-api/cmd/logcov	1.902s
```

E a mesma função aplicada a `.coverage-baseline` (leitura boa e leitura
duplicada, num probe que faz `include Makefile`):

```
-- arquivo OK:
valor=839
-- arquivo DUPLICADO:
FALHA: a chave 'min_coverage' aparece 2 vezes em /tmp/cb.dup (linhas 162 169 ).
       ...
(gate abortaria: exit 1)
```

**Lição reutilizável** — e é a parte que sobrevive a este achado:
**acrescentar uma chave num arquivo de baseline em vez de EDITAR a existente
desativa o gate em silêncio.** Leitura de arquivo `chave=valor` por `grep` não
é fail-closed por natureza: `grep` devolve o conjunto, o shell aceita a string
multi-linha na atribuição, e o `test` numérico converte o erro em "não houve
regressão". **Todo gate que ler baseline assim tem o mesmo buraco**, e a defesa
não é disciplina de quem edita, é o leitor recusar chave ambígua. Corolário
prático: o gate imprimir o baseline inteiro em toda execução (que este já
fazia, "por design") **não** revelou a duplicata — ninguém lê duas linhas
iguais separadas por 20 linhas de comentário. Contagem explícita revela;
impressão não.

**CORREÇÃO DESTA ENTRADA (FIX-09, 2026-08-18): a varredura de "todo gate"
estava INCOMPLETA.** A lição acima diz "todo gate que ler baseline assim tem o
mesmo buraco", mas a correção original protegeu só `coverage-gate` e
`log-coverage-gate` e **deixou o alvo `lint` de fora**. O leitor que sobrou era,
em `Makefile:213-214`:

```make
base_max=$$(grep -oE '^max_complexity=[0-9]+' $(BASELINE_FILE) | grep -oE '[0-9]+'); \
base_count=$$(grep -oE '^count=[0-9]+' $(BASELINE_FILE) | grep -oE '[0-9]+'); \
```

Exatamente o padrão da F129, sobre `.golangci-baseline`, e sobre a chave que é
a **única trava** daquele alvo. Com `max_complexity` duplicada, `base_max` vira
a string de duas linhas, `[ "$$max" -gt "$$base_max" ]` aborta com "integer
expression expected", o `if` do shell trata o erro como FALSO e a trava de
complexidade some em silêncio — a mesma mecânica, um arquivo diferente.

Os dois leitores passaram a usar `baseline_key`, fail-closed. O `if [ -z
"$$base_max" ]` que existia logo abaixo foi removido junto: `baseline_key` já
recusa chave ausente, vazia e não-inteira, então aquele teste virou código
morto que sugeria proteção maior do que havia.

**Varredura completa do Makefile, enumerada** — chaves hoje lidas por
`baseline_key`, alvo por alvo:

| alvo | arquivo | chaves protegidas |
|---|---|---|
| `coverage-gate` | `.coverage-baseline` | `min_coverage` |
| `lint` | `.golangci-baseline` | `max_complexity`, `count` |
| `log-coverage-gate` | `.log-coverage-baseline` | `stage`, `min_func_coverage`, `min_errpath_coverage`, `min_eligible`, `max_exempt_annotations` |

Oito chaves, três arquivos, três alvos. **Não sobrou leitor desprotegido**, e a
verificação foi por busca exaustiva, não por leitura: `grep -n "grep -oE '\^"
Makefile` devolve só duas linhas, o comentário da linha 54 e a extração da
contagem de issues de `.lint.out` (que não é baseline e já falha fechado por
conta própria); `grep -n 'BASELINE_FILE)' Makefile` devolve, além das oito
chamadas de `baseline_key`, apenas ocorrências dentro de `echo` de mensagem de
erro e o `grep -E '^(stage|min_|max_)' $(LOGCOV_BASELINE_FILE)` da linha 296,
que **imprime** o baseline para humanos e não deriva valor nenhum para
comparação — não é leitor.


**Status**: **corrigido** nesta sessão, nas três frentes, com os cinco
controles negativos acima executados e colados. Travas anti-regressão: o
próprio `make log-coverage-gate` / `make coverage-gate`, que agora falham
fechado — controles (a), (b), (c) e (c2) —, e `TestBaselineBateComAMedicao`,
que agora recusa chave duplicada — controle (d).

**Pendência que NÃO era deste achado, e foi fechada em seguida no mesmo
dispatch, por ordem do coordenador**: com o piso voltando a comparar,
`make check` fechou em `EXIT:2` numa única falha —
`TestBaselineBateComAMedicao` (`cmd/logcov/main_test.go:80`, erro em
`main_test.go:94`), em `min_errpath_coverage = 854 no baseline, medido 858` e
`min_eligible = 629 no baseline, medido 632`. Os dois deltas são de
CAP-09A/CAP-09B (histórico de conversa e download unificado), código real que
está na árvore: **ratchet-UP honesto**, não afrouxamento. As duas chaves foram
subidas — `min_errpath_coverage` 854 → 858 e `min_eligible` 629 → 632 — com
justificativa medida e enumerada nome por nome no próprio
`.log-coverage-baseline`, que registra explicitamente que essas duas subidas
são das capabilities e **não** do FIX-GATE. Ponto que a medição desmentiu e
vale guardar: o conjunto de elegíveis não "cresceu 3" — teve **8 entradas e 5
saídas** (as cinco `Download*UseCase.Execute` viraram delegação de uma linha e
caíram por X1), e os caminhos de saída não foram "4 novos" — foram +12 líquidos
distribuídos por cinco pacotes, com `errpaths_covered` subindo +15, mais que o
total, porque 10 caminhos (5 cobertos, 5 descobertos) **saíram** junto.
`make check` fecha agora em **`EXIT:0`**.

## F130

**Data**: 2026-08-18, **diagnostico REESCRITO em 2026-08-19** depois de medir.
**Contexto**: `make check` do Chief antes de commitar CAP-09A/CAP-09B, falha
intermitente sob `-race`.

**Onde**: cinco lancamentos de `RunHeartbeat` em `pkg/bootstrap/lease_test.go`
(linhas originais 177, 363, 402, 425, 448) — todos PRE-EXISTENTES, nenhum no
diff do CAP-09.

### O que esta entrada AFIRMAVA, e por que parecia certo

A versao original desta entrada dizia que a causa do `make check` vermelho era
a goroutine vazada do heartbeat: `go manager.RunHeartbeat(ctx)` com apenas
`defer cancel()`, sobrevivendo ao teste e lendo o `log.Logger` global que o
teste seguinte escreve. Parecia certo porque o stack colado no `make check`
REALMENTE mostrava isso:

```
Write at 0x000106b1e140 by goroutine 3019:
  TestLease_RetomadaAposExpirarDeixaRastro()  lease_test.go:527
Previous read at 0x000106b1e140 by goroutine 3015:
  zerolog.(*Logger).disabled()
  bootstrap.(*leaseManager).renewOne()        lease.go:260
  bootstrap.(*leaseManager).RunHeartbeat()    lease.go:321
```

O erro nao foi ler o stack errado. Foi generalizar UMA ocorrencia para a causa
do gate vermelho, sem medir a distribuicao.

### O que a medicao mostrou

`go test ./pkg/bootstrap/ -race -count=20`, atribuindo cada bloco de corrida
pelos dois lados do stack:

| | corridas em 20 execucoes | blocos citando `renewOne`/`RunHeartbeat` |
|---|---|---|
| ANTES da correcao | 19 | **0** |
| DEPOIS da correcao | 20 | **0** |

Os **39 blocos** das duas execucoes tem o MESMO lado sobrevivente:
`tentarWebhook` (`pkg/bootstrap/dispatch_callhook.go:107`, `:108` e `:112`),
rodando dentro do pool global de despacho e dos timers de retry. Pares
observados:

```
 7  TestHandleEvent_DefaultNaoLogaValores.func1@eventhandler_test.go:91   x tentarWebhook@dispatch_callhook.go:107
 6  TestSessionEventDispatcher_HandleDeOutroTipo.func1@session_adapters_test.go:53 x tentarWebhook@dispatch_callhook.go:107
 5  TestWalogSeam_ErroDoSDKSaiSemWadebug.func1@walog_seam_test.go:36      x tentarWebhook@dispatch_callhook.go:107
 1  TestLease_RetomadaAposExpirarDeixaRastro@lease_test.go:567            x tentarWebhook@dispatch_callhook.go:112
 1  capturarLog@eventhandler_media_test.go:29                             x tentarWebhook@dispatch_callhook.go:112
```

E o heartbeat vazado nao reproduz nem sob MUTACAO. Reintroduzido o padrao
vazado em `TestLease_WithoutLiveSessionCheckKeepsRenewing`:

- `-run TestLease -race -count=50` → 0 corridas
- `-run TestLease -race -count=500 -cpu 1` → 0 corridas
- pacote inteiro `-race -count=20` → 26 corridas, **0** citando `renewOne`

**Conclusao honesta**: a goroutine vazada do heartbeat e um defeito REAL e foi
corrigida, mas e minoritaria a ponto de nao ser reproduzivel sob mutacao em 570
execucoes dirigidas. Ela **NAO** e a causa do `make check` vermelho. A causa
real esta na **F132**.

### A correcao aplicada

Os cinco lancamentos passaram a ESPERAR a goroutine, nao so cancela-la:

```go
ctx, cancel := context.WithCancel(context.Background())
var wg sync.WaitGroup
wg.Add(1)
go func() { defer wg.Done(); manager.RunHeartbeat(ctx) }()
defer wg.Wait()
defer cancel()
```

A ordem dos `defer` e portadora de carga: LIFO faz `cancel()` rodar ANTES de
`wg.Wait()`. Invertida, o teste espera por uma goroutine que ninguem mandou
parar. Producao (`pkg/bootstrap/lease.go`) NAO foi tocada.

Linhas corrigidas (numeracao apos a edicao): 178, 372, 419, 450, 481.

### Os testes que travam o defeito

`TestLease_HeartbeatNaoSobreviveAoTeste` (`pkg/bootstrap/lease_test.go`), com o
auxiliar `countHeartbeatGoroutines`, que conta goroutines vivas dentro de
`RunHeartbeat` lendo o dump do proprio runtime (`runtime.Stack(buf, true)`).

O dump e o instrumento certo justamente porque o defeito e "a goroutine
sobrevive ao teste": nenhuma assercao sobre o estado do gerenciador consegue
ver uma goroutine da qual o gerenciador ja se desinteressou. E o `-race` nao
serve de guarda aqui — foi medido acima que ele nao morde este produtor.

O teste tem DUAS assercoes, e a primeira existe para que a segunda nao passe
por vacuidade: (1) enquanto o heartbeat roda, o dump o enxerga; (2) depois que
o bloco lancador retorna, restam ZERO.

### Controles negativos, EXECUTADOS

**Controle A — padrao vazado dentro da guarda.** Trocado o bloco lancador de
volta para `defer cancel(); go manager.RunHeartbeat(ctx)`:

```
$ go test ./pkg/bootstrap/ -run TestLease_HeartbeatNaoSobreviveAoTeste -race -count=20
--- FAIL: TestLease_HeartbeatNaoSobreviveAoTeste (0.00s)
    lease_test.go:696: 1 heartbeat goroutine(s) survived the launcher: this is
    F130 exactly — the survivor keeps reading the global log.Logger that the
    next test writes
FAIL    wa-api/pkg/bootstrap    0.385s
```

Falhou nas 20 execucoes. Revertido; 20/20 PASS depois.

**Controle B — ordem dos `defer` invertida** em
`TestLease_WithoutLiveSessionCheckKeepsRenewing` (`wg.Wait()` antes de
`cancel()`):

```
$ go test ./pkg/bootstrap/ -run TestLease_WithoutLiveSessionCheckKeepsRenewing -race -timeout 30s
panic: test timed out after 30s
goroutine 12 [sync.WaitGroup.Wait]:
    .../pkg/bootstrap/lease_test.go:496 +0x558
goroutine 13 [select]:
wa-api/pkg/bootstrap.(*leaseManager).RunHeartbeat(...)
    .../pkg/bootstrap/lease_test.go:482 +0x84
FAIL    wa-api/pkg/bootstrap    30.341s
```

Travou, como o comentario diz que travaria. Revertido.

**Status**: CORRIGIDO no que esta entrada cobre — os cinco lancamentos vazados
—, travado por `TestLease_HeartbeatNaoSobreviveAoTeste` com os dois controles
acima. O `make check` vermelho NAO era isto: ver **F132**.

## F131

**Data**: 2026-08-19. **Contexto**: CURRENT_STATE do CAP-10 (Delete/Update
Message), antes de implementar.

**Onde**: `pkg/domain/message.go` — todos os `*Result` da superfície de envio.

**Problema**: a forma da resposta de TODA a superfície de envio diverge do
contrato histórico, e a divergência nunca foi registrada como decisão.

- **Histórico** (`git show 41bc8e2^:handlers.go`, SendMessage linha ~132,
  DeleteMessage linha 2825, SendEditMessage): a resposta era
  `{"Details": "Sent"|"Deleted", "Timestamp": <unix>, "Id": <msgid>}`.
- **Atual**: `{"message_id": ..., "timestamp": ..., "status": "sent"}`.

Três nomes de campo diferentes e um campo a menos: `Details` sumiu, `Id`
virou `message_id`, e apareceu `status`.

A divergência é **anterior** às capabilities recuperadas nesta sessão: os
DTOs já tinham essa forma quando eram stubs `"validated"`. As oito
capabilities entregues (CAP-01 a CAP-08) preencheram esses DTOs, então a
forma nova já está em produção em text, image, audio, video, document,
sticker, location e contact.

**Por que registro agora**: o CAP-10 vai recuperar Delete e Edit, cujos DTOs
atuais (`DeleteMessageResult`, `SendEditMessageResult`) também têm a forma
nova. Implementar sem decidir significaria escolher por omissão — e é
exatamente o modo de falha da F123, em que o tipo "que parece certo" muda o
contrato em silêncio.

**As duas saídas, e o custo de cada uma**:

1. **Manter a forma atual** (`message_id`/`timestamp`/`status`): consistente
   com as oito capabilities já entregues; um cliente do wuzapi original
   continua quebrado, mas já estava — a rota devolvia `"validated"` sem
   enviar.
2. **Voltar à forma histórica** (`Details`/`Timestamp`/`Id`): fidelidade ao
   contrato original, mas exigiria mudar as oito capabilities já commitadas,
   o que é mudança de contrato público em massa.

**Correção sugerida**: manter a forma atual e PARAR de tratá-la como
acidente — documentá-la como o contrato do wa-api, com um teste de forma de
wire por capability, no padrão do
`chat_history_wire_contract_test.go` criado no CAP-09A. Hoje nenhuma das oito
tem trava de nome de campo: renomear `message_id` para qualquer coisa passa
com a suíte verde, que foi exatamente o REQUIRED_FIX da EVAL-09.

**Status**: não corrigido. Levado ao canal de decisão junto com o
CURRENT_STATE do CAP-10.

## F132

**Data**: 2026-08-19. **Contexto**: descoberto ao medir a F130 — e a causa REAL
do `make check` intermitentemente vermelho que a F130 atribuia ao heartbeat.

**Onde**:
- `pkg/bootstrap/dispatch.go:112` — `newDispatchPool` lanca N workers
  (`dispatchDefaultWorkers = 256`) que so morrem quando o canal `jobs` fecha.
- `pkg/bootstrap/dispatch.go:221` — `dispatchGo` cria esse pool por
  `sync.Once` num par de globais (`dispatchOnce`, `dispatch`). **Nao existe
  shutdown**: nada fecha `jobs`, nada espera os workers.
- `pkg/bootstrap/dispatch_retry.go:161` — `time.AfterFunc(atraso, ...)`
  reagenda a entrega pelo pool. Com o backoff exponencial de base 30s, ha
  trabalho pendente por MINUTOS.
- `pkg/bootstrap/dispatch_callhook.go:107`, `:108`, `:112` — `tentarWebhook`
  chama `log.Warn()` nesse periodo.

**Problema**: o pool e um recurso de PROCESSO, criado pelo primeiro teste que
despacha um webhook e vivo ate o binario de teste morrer. Enquanto isso, dez
pontos de teste em oito arquivos trocam o `log.Logger` global. Toda troca e uma
corrida contra qualquer worker que esteja dentro de `log.Warn()`.

Os dez escritores do `log.Logger` global em `pkg/bootstrap` (confirmados por
`grep -rn "log\.Logger *=" --include="*.go" .`, alem dos dois de producao em
`main.go:170` e `main.go:201`):

1. `capabilities_test.go:88`
2. `eventhandler_session_test.go:72`
3. `eventhandler_media_test.go:29`
4. `lease_test.go:527` (hoje `:567` apos a correcao da F130)
5. `lease_test.go:566` (hoje `:606`)
6. `eventhandler_test.go:90`
7. `session_adapters_test.go:24`
8. `session_adapters_test.go:52`
9. `walog_seam_test.go:35`
10. `wiring_delegates_test.go:27`

(Existem outros escritores em `pkg/infra/messaging` e `pkg/presentation/http`,
mas sao binarios de teste distintos e nao disputam com este pool.)

**Evidencia medida** (`go test ./pkg/bootstrap/ -race -count=20`):

- 19 corridas em 20 execucoes antes da correcao da F130, 20 depois.
- **39 de 39** blocos tem `tentarWebhook` como lado sobrevivente.
- **0 de 39** citam `renewOne` ou `RunHeartbeat`.
- Numa passada UNICA de `make check` a corrida as vezes nao aparece: o Chief
  observou EXIT:2 numa execucao e EXIT:0 na seguinte. Sob `-count=20` ela
  reproduz quase sempre. Detector acusando prova que a corrida existe; nao
  acusar nao prova nada.

**Correcao sugerida** — duas saidas, ambas mudanca de PRODUCAO:

1. Dar shutdown ao pool: fechar `jobs` e esperar os workers, com um seam que
   o `TestMain` do pacote possa chamar. Precisa tambem cancelar os
   `time.AfterFunc` pendentes, senao o retry ressuscita o pool depois do
   shutdown.
2. Injetar o logger no caminho de despacho, eliminando a leitura da global.
   Mais alinhado ao resto do repo, mas superficie maior.

**Status**: NAO corrigido, e deliberadamente fora deste bloco. Nao e escopo do
FIX-F130 e nao foi autorizado. Alem disso, a linhagem deste pool ja custou caro
(F86, F88): converter recurso ilimitado em limitado exige inventario de
detentores e medicao do cenario que PIORA, pelas quatro regras do `CLAUDE.md`.
Merece packet proprio. Referencia cruzada: **F130**.

## F133

**Data**: 2026-08-19. **Contexto**: verificacao final do FIX-F130 (`gofmt -l`
sobre `pkg/bootstrap/`).

**Onde**: `pkg/bootstrap/config.go`.

**Problema**: o arquivo nao esta formatado por `gofmt`. Pre-existente em HEAD
(e5b2528) e nao tocado por este bloco:

```
$ gofmt -l pkg/bootstrap/
pkg/bootstrap/config.go
$ git show HEAD:pkg/bootstrap/config.go | gofmt -l /dev/stdin
/dev/stdin
$ git diff --stat HEAD -- pkg/bootstrap/config.go   # vazio
```

O `make check` passou com EXIT:0 mesmo assim, o que significa que o gate atual
**nao verifica formatacao** — entao qualquer arquivo pode divergir sem que nada
avise, e o proximo diff que tocar `config.go` vai misturar reformatacao com
mudanca de comportamento.

**Correcao sugerida**: `gofmt -w pkg/bootstrap/config.go` num commit isolado, e
acrescentar `gofmt -l` (falhando se a saida for nao-vazia) ao alvo `check` do
Makefile, para que isto nao volte em silencio.

**Adendo (FIX-10, 2026-08-19)**: `config.go` nao e o unico. Rodando `gofmt -l`
sobre a arvore inteira aparecem TRES arquivos, os tres nao formatados ja em
HEAD (e5b2528) e nenhum deles tocado pelo FIX-10:

```
$ gofmt -l ./pkg ./cmd
pkg/application/usecase/message/send_video_internal_test.go
pkg/bootstrap/config.go
pkg/presentation/http/handlers/handler_boundary_test.go
$ git show HEAD:pkg/application/usecase/message/send_video_internal_test.go | gofmt -l /dev/stdin
/dev/stdin
$ git show HEAD:pkg/presentation/http/handlers/handler_boundary_test.go | gofmt -l /dev/stdin
/dev/stdin
```

Reforca a correcao sugerida: sem `gofmt -l` no alvo `check`, o numero cresce
em silencio — era um arquivo na F133 e sao tres uma sessao depois.

**Status**: nao corrigido. E pre-existente e fora do escopo do FIX-F130; pela
politica do `CLAUDE.md`, nao corrijo de graca sem perguntar.

## F134 — `SendEditMessage` perdeu o `ContextInfo` que o payload histórico aceitava

**Data**: 2026-08-19. **Contexto**: CAP-10, recuperação de Delete + Edit
Message.

**Onde**: `pkg/domain/message.go` (`SendEditMessageRequest`, em torno da linha
305) contra `git show 41bc8e2^:handlers.go` (`SendEditMessage`, o campo
`ContextInfo waE2E.ContextInfo` do `editStruct`).

**Problema**: o payload histórico de `POST /chat/send/edit` aceitava um quarto
campo, `ContextInfo`, com três usos reais:

- `ContextInfo.StanzaID` + `ContextInfo.Participant` — a edição virava uma
  resposta CITADA (o handler montava `ContextInfo` com `QuotedMessage`);
- `ContextInfo.MentionedJID` — menções dentro do texto editado.

O DTO atual não tem esse campo. Um cliente que enviava `ContextInfo` hoje tem
o campo silenciosamente descartado pelo `json.Decode` e recebe 200: a edição
acontece, mas sem citação e sem menções. Isto **não é regressão do CAP-10** —
o campo já não existia no DTO antes desta task, quando a rota nem editava; o
CAP-10 apenas tornou a perda observável, porque agora a mensagem é realmente
enviada.

Evidência (o campo não existe no DTO atual):

```
$ grep -n "ContextInfo" pkg/domain/message.go
(sem saída)
$ git show 41bc8e2^:handlers.go | grep -n "ContextInfo waE2E.ContextInfo"
2894:		ContextInfo waE2E.ContextInfo
```

**Correcao sugerida**: acrescentar a `SendEditMessageRequest` os três campos
que o histórico usava (`StanzaID`, `Participant`, `MentionedJID`, como
ponteiros para preservar a distinção ausente/vazio que o handler antigo fazia
com `!= nil`), propagá-los pelo use case até
`ChatMessengerAdapter.EditMessage` e montar o `ContextInfo` do
`ExtendedTextMessage` lá. É mudança de contrato de ENTRADA (aditiva), então
merece decisão explícita.

**Status**: não corrigido. Fora do escopo do CAP-10, que era fazer a rota
mutar de verdade; pela política do `CLAUDE.md`, não corrijo de graça sem
perguntar. Registrado em comentário no código, em
`pkg/application/usecase/message/send_edit_message.go` (doc de `Execute`) e em
`pkg/infra/wa-noise/adapters/chat/messenger.go` (doc de `EditMessage`).

## F135 — `Id` de mensagem inexistente ou inválido em delete/edit vira 200 silencioso

**Data**: 2026-08-19. **Contexto**: CAP-10 — o packet pedia explicitamente
para descobrir e RELATAR o que a primitive faz com `Id` inexistente/inválido.

**Onde**: `internal/wa-noise/capabilities/message/builders.go:39`
(`BuildRevoke`) e `:101` (`BuildEdit`), alcançados por
`internal/wa-noise/core/message_builders.go:34` e `:75`.

**Problema**: **os dois construtores não validam nada e não consultam
armazenamento nenhum**. Recebem o `id` como `types.MessageID` — que é um alias
de `string`, sem validação — e o copiam para dentro da `MessageKey` do
protobuf. Não há lookup da mensagem alvo em lugar nenhum do caminho.

Consequência para a fronteira HTTP: `POST /chat/delete`,
`/chat/delete/message` e `/chat/send/edit` com um `Id` que não existe (ou que
é lixo sintático) montam uma mensagem BEM FORMADA, o envio é aceito, e a rota
devolve **200 com `status` de sucesso** — o servidor do WhatsApp simplesmente
ignora a revogação/edição de uma chave que ele não conhece, e essa recusa
NUNCA volta pelo `SendMessage`. A API não tem como distinguir "apagou" de "não
existia".

Evidência (medida, não deduzida): `TestChatMessengerAdapter_Mutation_UnknownIDIsNotValidated`
em `pkg/infra/wa-noise/adapters/chat/messenger_mutation_test.go` roda os ids
`""`, `"id-que-nao-existe"`, `"not a message id at all"` e
`"../../etc/passwd"`; nenhum é recusado, e todos chegam crus a
`MessageKey.ID`. (O `""` só não é alcançável pela rota porque o use case
rejeita `Id` vazio antes — a primitive em si o aceita.)

**Correcao sugerida**: nenhuma no nível da primitive — o comportamento é do
protocolo, e o Baileys tem a mesma propriedade (a revogação é uma mensagem
como outra qualquer, não um RPC com resposta). O que dá para fazer, se o
produto exigir, é a API guardar os IDs que ela mesma enviou e recusar 404 para
um `Id` que ela nunca emitiu — o que é uma feature nova (persistência de IDs
enviados), não um conserto.

**Status**: não corrigido, por desenho. Travado como comportamento CONHECIDO
pelo teste citado acima, para que ninguém o redescubra em produção. Documentado
na entrada porque um "sucesso" que não distingue do fracasso é exatamente o
tipo de coisa que vira diagnóstico errado seis meses depois.
