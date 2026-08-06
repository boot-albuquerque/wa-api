# Housekeep

Registro de bugs, gaps e dívidas técnicas encontrados durante sessões de
trabalho, mas fora do escopo do que estava sendo feito naquele momento.
Não é backlog de features — é achado incidental que alguém precisa decidir
o que fazer (corrigir, ignorar, virar issue formal).

Cada entrada deve ter: data, contexto de onde foi encontrado, arquivo(s) e
linha(s) exatos, descrição do problema, e se possível o caminho de correção
sugerido. Sem isso, o achado se perde ou vira arqueologia de código na
próxima vez que alguém tropeçar nele.

---

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

**Status**: não corrigido — fora do escopo da feature que o encontrou.

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

**Encontrado durante**: implementação do plano de vendoring do whatsmeow
(branch `feature/vendor-whatsmeow`, `.omc/plans/vendor-whatsmeow-native-fork.md`).

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
`feature/vendor-whatsmeow`.

**Status**: corrigido (branch `feature/vendor-whatsmeow`, ainda não
mergeada em `develop` no momento deste registro).

---

## 2026-08-06 — `.log-coverage-baseline` tinha `min_func_coverage=`/`min_errpath_coverage=` duplicados

**Encontrado durante**: mesma implementação acima (vendoring do whatsmeow).

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
`d11979b` em `feature/vendor-whatsmeow`.

**Status**: corrigido nesta branch. **Atenção**: como `ace7770` é de outra
sessão/branch que pode não ter esse fix, vale confirmar que a duplicata
não reaparece no merge — é um problema de "esqueceu de apagar a linha
velha ao adicionar a nova", fácil de reintroduzir se outra sessão editar
o arquivo do mesmo jeito.

---

## 2026-08-06 — `internal/waclient/` (vendored whatsmeow) sem bridge de log para o padrão do projeto

**Encontrado durante**: revisão de arquitetura pós-vendoring do whatsmeow
(branch `feature/vendor-whatsmeow`), solicitada explicitamente para
avaliar se `internal/waclient/` segue os padrões de log/erro já
estabelecidos no resto do projeto (via agente `architect`).

**Onde**:
- `internal/waclient/util/log/log.go:17-23` — interface `waLog.Logger`
  (`Warnf/Errorf/Infof/Debugf/Sub`) que o whatsmeow espera receber.
- `pkg/infra/whatsmeow/logger.go:12` — `ZerologAdapter`, que implementa
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
do port é parcial — `pkg/infra/whatsmeow/user_adapters.go:40,44,67,71,80`
repassa `err` cru vindo do waclient sem `apperr.New(...)`, então esses
erros chegam no HTTP boundary sem `Code`/`Category`/`Retryable`. Os
demais pontos da fronteira (`session_provider_adapter.go`,
`session_guard_adapter.go`, `misc_adapters.go`) já fazem a tradução
correta com `errors.Is` contra sentinels do whatsmeow — nenhum
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
`.omc/plans/whatsmeow-clean-arch-walog-bridge.md`, que fixa código e
categoria dos 5 sites em tabela.

**Onde**: `pkg/infra/whatsmeow/user_adapters.go:63-66` (era
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

## F16 — `log.Fatalf` do stdlib dentro de `sendMexIQ` derruba o processo

**Data**: 2026-08-06
**Contexto**: auditoria de logging da Fase A do ADR-0004 (divisão da raiz
de `internal/waclient/` por responsabilidade). A tarefa pedia verificar se
o código vendorizado bypassa o padrão `cli.Log`/`cli.Log.Sub(...)` que o
`walog.Bridge` (commits `2848388`/`279a68a`) instrumenta. Achado de lado —
o escopo da fase era estrutural.

**Onde**: `internal/waclient/newsletter_mex.go:137` (era
`internal/waclient/newsletter.go` antes do split desta sessão), dentro de
`(*Client).sendMexIQ`:

```go
data, err := decoder.ArgoToMap(wt)
if err != nil {
    log.Fatalf("argo to map error: %v", err)
}
```

**Problema**: dois defeitos no mesmo `if`.

1. É o **único** ponto de toda a raiz de `internal/waclient/` que loga fora
   de `cli.Log` — usa o logger global do stdlib (`"log"`), então a mensagem
   não passa pelo `walog.Bridge` e não aparece no zerolog estruturado do
   `wa-api`. Todos os demais `fmt.Print*` do pacote estão dentro de
   comentários de exemplo de godoc; este é código real.
2. Pior que o log: `log.Fatalf` chama `os.Exit(1)`. Um payload argo
   malformado vindo do servidor do WhatsApp — entrada remota, não
   controlada por nós — **derruba o processo inteiro do `wa-api`**,
   matando todas as demais sessões. Todo o resto de `sendMexIQ` trata erro
   por `return nil, err` (linhas 122-134, 140-142, 146-151); esta linha é a
   única fora do padrão.

Não há teste cobrindo o caminho (o pacote está fora de `TEST_PKGS`), então
não há reprodução automatizada; o caminho é alcançável por qualquer
resposta MEX com codificação argo inválida.

**Correção sugerida**: alinhar com as linhas vizinhas —

```go
data, err := decoder.ArgoToMap(wt)
if err != nil {
    return nil, fmt.Errorf("argo to map error: %w", err)
}
```

e remover o import `"log"` do arquivo. Se por algum motivo o erro precisar
ser tolerado, o mínimo é `cli.Log.Errorf(...)` **com** `return nil, err` —
nunca seguir adiante com `data == nil`, que hoje faria `json.Marshal`
devolver o literal `null` como se fosse sucesso.

**Status**: **não corrigido**. A Fase A do ADR-0004 é estrutural por
contrato (`PATCHES.md` declara "comportamento não mudou" em todas as
entradas) e trocar `os.Exit` por retorno de erro é mudança de
comportamento observável. Pendente de decisão do usuário: corrigir agora
em commit próprio, ou deixar para a fase que tratar erros de
`internal/waclient/` com `apperr`.

---

## F17 — `internal/waclient/` continua fora de cobertura/lint/vet mesmo virando fork mantido

**Data**: 2026-08-06
**Contexto**: Fase A do ADR-0004. A tarefa pedia reavaliar a exclusão de
`internal/waclient/` dos gates agora que o diretório é mantido ativamente,
e incluí-lo **apenas se** `make check` continuasse verde.

**Onde**: `Makefile:17,21,22,29` (`COVER_PKGS`, `TEST_PKGS`, `VET_TARGETS`,
`LINT_TARGETS`, todos com `grep -v '^wa-api/internal/waclient'`) e
`.logcov-exclude:29` (`internal/waclient/`).

**Problema**: a justificativa escrita nesses arquivos ("cópia fiel de
código de terceiros, não código nosso a instrumentar") **está obsoleta**
desde o ADR-0004 — o diretório deixou de ser cópia fiel. O texto do
comentário agora contradiz o ADR vigente.

Medido nesta sessão, com os splits já aplicados:

- `go vet ./internal/waclient/` → **limpo** (sairia de graça).
- `golangci-lint run ./internal/waclient/` → **92 issues**, distribuídas em
  `gocyclo: 69`, `staticcheck: 19`, `ineffassign: 2`, `errcheck: 1`,
  `goimports: 1`. **Nenhuma** vem dos arquivos criados nesta fase — são
  todas do estilo do whatsmeow upstream (ex:
  `download-to-file.go:185` errcheck em `resp.Body.Close`;
  `message_decrypt.go:282` ST1012 em `EventAlreadyProcessed`;
  `client_test.go:16` goimports, arquivo não tocado).
- Cobertura: o pacote tem **zero testes** (só `client_test.go`, que é um
  `Example()` de godoc sem asserção). Entrar em `COVER_PKGS` adicionaria
  ~15.8k linhas com 0% ao denominador e derrubaria o `coverage-gate` na
  hora — `min_coverage=818` (décimos de %) é um piso de ratchet, não um
  alvo.

**Correção sugerida**: faseada, não de uma vez.
1. Agora: atualizar o **comentário** de `Makefile:12-16` e
   `.logcov-exclude:22-29` para citar o ADR-0004 e dizer que a exclusão é
   temporária/por dívida, não por o código ser de terceiros.
2. `VET_TARGETS` pode ser desacoplado de `COVER_PKGS` e passar a incluir
   `internal/waclient/...` já — vet está limpo hoje.
3. `LINT_TARGETS`: incluir só depois de um baseline próprio para o
   diretório (o gate de lint hoje trava por `max_complexity`, e o
   `gocyclo` máximo do whatsmeow é muito acima do baseline do repo).
4. `COVER_PKGS`/`TEST_PKGS`: só quando houver testes reais, por
   subdiretório, à medida que as Fases B/C do ADR-0004 forem cobrindo.

**Status**: **não corrigido** — a instrução da tarefa era explícita em não
incluir se aparecesse enxurrada de lint pré-existente, e apareceram 92
issues não relacionadas às mudanças. O teto de 300 linhas, esse sim, ficou
travado por gate novo (`make waclient-filesize`,
`scripts/waclient-filesize-check.sh`), que roda independente de
`COVER_PKGS`/`LINT_TARGETS` justamente por causa desta exclusão.

---

## F18 — `processData` corrompe frame cujo payload chega picado em mais de um websocket message

**Data**: 2026-08-06
**Contexto**: Fase A do ADR-0004, metade `internal/waclient/socket/`. Achado ao
escrever o primeiro teste de remontagem de frame do pacote — o bug é do
whatsmeow upstream, não introduzido por nós.

**Onde**: `internal/waclient/socket/framesocket.go:161-200`
(`(*FrameSocket).processData`), especificamente a linha 170:

```go
length := decodeFrameLength(msg)
fs.incomingLength = length
fs.receivedLength = len(msg)   // <-- conta os 3 bytes de cabecalho
msg = msg[FrameLengthSize:]    // <-- cabecalho so' e' descartado DEPOIS
```

**Problema**: `incomingLength` é o tamanho do **payload**, e todos os
`copy(fs.incoming[fs.receivedLength:], ...)` do ramo de continuação são
relativos ao payload. Mas `receivedLength` é inicializado com `len(msg)`
**incluindo** os `FrameLengthSize` (3) bytes de cabeçalho. Resultado: quando o
payload de um frame chega dividido em mais de um websocket message, o segundo
pedaço é escrito 3 bytes adiante do lugar correto, deixando 3 bytes zerados no
meio do frame remontado (e truncando os 3 últimos).

Reprodução (`internal/waclient/socket/framesocket_test.go`,
`TestProcessDataSplitPayload`, hoje `t.Skip`ado). Rodar sem o skip:

```
go test -run TestProcessDataSplitPayload ./internal/waclient/socket/
```

Saída observada vs esperada:

```
frame 0 = "pay\x00\x00\x00load longo dividido em peda"
esperado  "payload longo dividido em pedacos"
```

O caminho do cabeçalho parcial (`fs.partialHeader`) **não** tem o bug, porque lá
o `msg` é remontado e reprocessado do zero pelo ramo normal — por isso
`TestProcessDataPartialHeader` passa. Só o ramo de payload picado é afetado.

Alcançabilidade real: depende de o `conn.Read` do `coder/websocket` entregar um
frame WhatsApp partido em mais de uma leitura. Como a lib remonta fragmentação
de websocket antes de devolver, o caso mais provável é um frame WhatsApp grande
atravessando mais de uma mensagem websocket, ou vários frames por mensagem com
o último cortado. Não observado em produção até agora; o comentário do próprio
upstream na linha 163 ("This probably doesn't happen a lot (if at all), so the
code is unoptimized") sugere que o caminho nunca foi exercitado a sério.

**Correção sugerida**: mover a contagem para depois do descarte do cabeçalho —

```go
length := decodeFrameLength(msg)
fs.incomingLength = length
msg = msg[FrameLengthSize:]
fs.receivedLength = len(msg)
```

E, no mesmo commit, remover o `t.Skip` de `TestProcessDataSplitPayload`, que
passa a ser a prova da correção. Vale também mandar o patch para o upstream
(`go.mau.fi/whatsmeow`), já que o bug não é nosso.

**Status**: **não corrigido**. A Fase A do ADR-0004 é estrutural por contrato
(`PATCHES.md` declara "comportamento não mudou" em todas as entradas) e este é
justamente um caso em que o comportamento observável muda. Pendente de decisão
do usuário: corrigir agora em commit próprio, ou tratar junto com F16 numa
leva de correções de comportamento do fork.
