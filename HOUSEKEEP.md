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

---

## F19 — cortes de MAC em `appstate/` dão panic com blob mais curto que 32 bytes

**Data**: 2026-08-06
**Contexto**: Fase B do ADR-0004, metade `internal/wa-noise/appstate/`. Achado
na leitura linha a linha para a auditoria de magic numbers; o código é do
whatsmeow upstream, não introduzido por nós.

**Onde**: quatro cortes que assumem, sem checar, que o blob tem pelo menos
`macLength` (32) bytes:

- `internal/wa-noise/appstate/decode_mutation.go:60` (`Processor.decodeMutation`)

  ```go
  content := bytes.Clone(mutation.GetRecord().GetValue().GetBlob())
  content, valueMAC = content[:len(content)-macLength], content[len(content)-macLength:]
  ```

- `internal/wa-noise/appstate/hash.go:46` (`HashState.updateHash`)

  ```go
  value := mutation.GetRecord().GetValue().GetBlob()
  added = append(added, value[len(value)-macLength:])
  ```

- `internal/wa-noise/appstate/hash.go:90` (`generatePatchMAC`)
- `internal/wa-noise/appstate/decode.go:100` (callback de `validatePatch`)

**Problema**: `len(blob)` menor que 32 faz `len(blob)-macLength` ficar negativo
e o slice dar panic (`slice bounds out of range`). O blob vem direto de um
`SyncdValue` desserializado do servidor — ou seja, é entrada não confiável.
Um patch de app state malformado (ou um blob externo adulterado) derruba a
goroutine que processa app state em vez de devolver erro. Não há validação de
tamanho mínima em nenhum ponto do caminho: `ParsePatchList` só desserializa o
protobuf, e `decodeMutation` já corta na primeira linha útil.

Não há reprodução em produção; o caminho exige um servidor (ou um MITM pós-Noise)
mandando `SyncdValue.Blob` truncado. Nenhum teste foi escrito para ele
justamente porque o teste seria um panic, não uma falha de asserção.

**Correção sugerida**: validar antes de cortar, no ponto de entrada
(`decodeMutation`), e devolver erro sentinela novo em `errors.go`:

```go
// errors.go
ErrShortMutationBlob = errors.New("mutation value blob shorter than MAC length")

// decode_mutation.go, antes do corte
blob := mutation.GetRecord().GetValue().GetBlob()
if len(blob) < macLength+cbcIVLength {
    err = fmt.Errorf("failed to decode mutation #%d: %w", i+1, ErrShortMutationBlob)
    return
}
```

O mesmo teto vale para `updateHash`/`generatePatchMAC`, que rodam **antes** de
`decodeMutation` no fluxo de `validatePatch` — então a validação precisa
acontecer nos dois lugares, ou `validatePatch` precisa varrer as mutações uma
vez antes de chamar `updateHash`. Vale mandar o patch para o upstream
(`go.mau.fi/whatsmeow`), já que o bug não é nosso.

**Status**: **não corrigido**. A Fase B do ADR-0004 é estrutural por contrato
(`PATCHES.md` declara "comportamento não mudou" nas entradas de divisão) e
trocar panic por erro é mudança de comportamento observável. Pendente de
decisão do usuário — candidato natural à mesma leva de F16/F18.

---

## F20 — `fakeIndexesToRemove` é sempre nil: ramo morto em `decodeMutations`

**Data**: 2026-08-06
**Contexto**: Fase B do ADR-0004, metade `internal/wa-noise/appstate/`. Mesmo
racional de F19: código do upstream, achado na leitura para a auditoria.

**Onde**: `internal/wa-noise/appstate/decode.go:48` e
`internal/wa-noise/appstate/decode.go:155` — as duas declarações:

```go
var fakeIndexesToRemove map[[macLength]byte][]byte
```

O mapa é declarado e passado a `Processor.decodeMutations` sem nunca ser
inicializado nem populado, nos dois chamadores (`decodeSnapshot` e o laço de
`DecodePatches`). Consumidor em
`internal/wa-noise/appstate/decode_mutation.go:118`:

```go
altIndexMAC, ok := fakeIndexesToRemove[indexMACToArray(indexMAC)]
if ok && len(indexMAC) == macLength {
    out.RemoveMAC(altIndexMAC)
}
```

**Problema**: leitura de mapa nil sempre devolve `ok == false`, então o ramo de
remoção do "index MAC alternativo" nunca executa. Não é um bug de corretude
observável hoje (o comportamento é o de não ter a feature), mas é um parâmetro
que atravessa três funções sem fazer nada — e, se a intenção original era
remover MACs de índices "falsos"/legados gerados por outra plataforma, então há
uma limpeza de estado que simplesmente não acontece, e o app state acumula MACs
órfãos no banco.

**Correção sugerida**: nenhuma imediata — antes é preciso descobrir a intenção
no upstream (`git log`/issues de `go.mau.fi/whatsmeow` em torno de
`fakeIndexesToRemove`). Dois desfechos possíveis: (a) a feature nunca foi
ligada e o parâmetro deve ser removido das três assinaturas, simplificando;
(b) deveria estar populado, e aí é bug de verdade no upstream. Não dá para
escolher sem a intenção.

**Status**: **não corrigido**. O ramo fica coberto por
`TestDecodeMutationsRemovesFakeIndex` (que passa o mapa direto para
`decodeMutations`), de modo que ele não se degrade silenciosamente caso venha a
ser ligado. Pendente de investigação no upstream.

---

## F21 — `SQLStore.DeleteIdentity` usa a query de `LIKE`, não a de igualdade

**Data**: 2026-08-06. **Contexto**: segunda metade da Fase B do ADR-0004
(refactor de `internal/wa-noise/store/` + `store/sqlstore/`), mesmo racional de
F18/F19/F20: código do upstream, achado na leitura para dividir o arquivo.

**Onde**: `internal/wa-noise/store/sqlstore/store_identity.go:36` (era
`store.go:92` antes da divisão desta fase):

```go
func (s *SQLStore) DeleteIdentity(ctx context.Context, address string) error {
	_, err := s.db.Exec(ctx, deleteAllIdentitiesQuery, s.JID, address)
	return err
}
```

`deleteAllIdentitiesQuery` é `... WHERE our_jid=$1 AND their_id LIKE $2`.
A query de igualdade existe logo acima, declarada e **sem nenhum uso**:

```go
deleteIdentityQuery = `DELETE FROM whatsmeow_identity_keys WHERE our_jid=$1 AND their_id=$2`
```

**Problema**: `DeleteIdentity` recebe um endereço Signal completo
(`<user>:<device>`), sem sufixo curinga, então na prática o `LIKE` degenera em
igualdade e o efeito observável coincide — há teste provando isso
(`TestDeleteIdentityRemovesOnlyThatAddress` em `store_identity_test.go`, que
confere que `erin:1` sobrevive a `DeleteIdentity("erin:0")`). Duas consequências
mesmo assim:

1. `deleteIdentityQuery` é código morto — uma constante declarada que nenhum
   caminho executa, exatamente o tipo de coisa que confunde na próxima leitura.
2. Um endereço que contenha `%` ou `_` seria interpretado como padrão de `LIKE`
   e apagaria mais linhas que o pedido. Hoje inalcançável, porque o endereço vem
   de `types.JID.SignalAddressUser()` (dígitos + `:` + dígitos), mas é uma
   dependência silenciosa numa função de apagar chave criptográfica.

**Correção sugerida**: trocar para `deleteIdentityQuery`. Uma linha:

```go
_, err := s.db.Exec(ctx, deleteIdentityQuery, s.JID, address)
```

**Status**: **não corrigido**. A Fase B é estrutural por contrato, e isto é
mudança de comportamento em entrada patologica (ainda que a intenção do autor
seja evidente pela existência da constante não usada). Pendente de decisão do
usuário.

---

## F22 — `record.Session` recém-criado dá panic em `Serialize()`

**Data**: 2026-08-06. **Contexto**: escrita dos testes de
`internal/wa-noise/store/sessioncache.go` e `signal.go` na Fase B do ADR-0004.

**Onde**: não é código nosso — é `go.mau.fi/libsignal@v0.2.1`,
`state/record/SessionState.go:517` (`State.structure()`), alcançado por
`record.Session.Serialize()`. O caminho no nosso código:

- `internal/wa-noise/store/signal.go:96` e `signal.go:115`
  (`LoadSession` devolve `record.NewSession(...)` quando não há sessão; e
  `StoreSession` chama `record.Serialize()`).
- `internal/wa-noise/store/sessioncache.go:87` e `sessioncache.go:113`
  (`WithCachedSessions` cria o mesmo record vazio; `PutCachedSessions` serializa
  as entradas `Dirty`).

**Problema**: um `record.NewSession(...)` "fresco" tem `localIdentityPublic`,
`remoteIdentityPublic`, `senderBaseKey` e `senderChain` nil. `Serialize()` os
desreferencia sem checar e dá `SIGSEGV` — panic, não erro. Reprodução (é um
teste versionado, `TestFreshSessionIsNotSerializable` em `sessioncache_test.go`):

```go
_ = record.NewSession(SignalProtobufSerializer.Session, SignalProtobufSerializer.State).Serialize()
// panic: runtime error: invalid memory address or nil pointer dereference
//   go.mau.fi/libsignal/keys/identity.(*Key).Serialize(...)
//   .../state/record/SessionState.go:517
```

**Por que não explode em produção hoje**: `PutCachedSessions` só serializa
entradas marcadas `Dirty`, e `Dirty` só é setado por `putCachedSession`, que o
libsignal chama depois do handshake — quando o state já está preenchido.
`StoreSession` idem. É um panic latente atrás de uma invariante não declarada,
não um bug ativo.

**Correção sugerida**: nenhuma no libsignal (é dependência externa). Do nosso
lado, a defesa barata seria `PutCachedSessions` pular entradas cujo record ainda
não tem state utilizável, mas não há predicado exportado para checar isso sem
chamar `Serialize()` — que é exatamente o que panica. A alternativa honesta é
`recover()` localizado, que é pior do que o problema.

**Status**: **não corrigido**, documentado. Ficam versionados dois testes que
travam a assimetria para quem for mexer no cache de sessão:
`TestFreshSessionIsNotSerializable` (prova o panic) e
`TestStoredSessionFixtureRoundTrips` (mostra a forma mínima de sessão que
sobrevive ao round trip, usada como duplo nos demais testes).

---

## F23 — caminho Postgres de `store/sqlstore/` não é exercitado por teste

**Data**: 2026-08-06. **Contexto**: cobertura da Fase B do ADR-0004
(`store/sqlstore/` fechou em 89.7% de statements).

**Onde**: os três pontos que ramificam por dialeto:

- `internal/wa-noise/store/sqlstore/store_session.go:66`
  (`GetManySessions`, ramo `PostgresArrayWrapper != nil`)
- `internal/wa-noise/store/sqlstore/store_appstate.go:106`
  (`DeleteAppStateMutationMACs`, mesmo ramo)
- `internal/wa-noise/store/sqlstore/lidmap.go:171`
  (`GetManyLIDsForPNs`, mesmo ramo)

**Problema**: os testes rodam sobre `modernc.org/sqlite` (sem CGO, sem Docker,
como o resto do repositório), então só o ramo genérico com placeholders `$N`
expandidos é executado. O ramo Postgres — que usa `= ANY($2)` com o array
envolvido por `PostgresArrayWrapper` — nunca roda em `make check`. **Produção
usa Postgres** (`pkg/bootstrap/main.go:339`), ou seja: o caminho testado e o
caminho executado em produção são ramos diferentes do mesmo `if`.

Não há evidência de defeito neles — são o código original do upstream, e a
divergência entre os dois ramos é só a sintaxe do `IN`. Mas é a maior lacuna de
cobertura do pacote, e é justamente na metade que roda em produção.

**Correção sugerida**: subir um Postgres efêmero para o `make check` (testcontainers
ou um serviço no CI) e parametrizar `newTestContainer` por dialeto, rodando a
mesma suíte duas vezes. É decisão de infraestrutura de CI, não de código — hoje
o repositório inteiro é deliberadamente Docker-free nos testes.

**Status**: **não corrigido**, registrado como lacuna consciente. Também anotado
na seção "Fora do escopo" da entrada da Fase B em
`internal/wa-noise/PATCHES.md`.

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

## F24 — `binary/` dá panic com bytes da rede em três type assertions sem checagem

**Data**: 2026-08-06. **Contexto**: Fase C do ADR-0004 (cobertura de
`internal/wa-noise/binary/`). Apareceu ao escrever os testes de frame
malformado, não por relato de produção.

**Onde**:

- `internal/wa-noise/binary/decoder_node.go:137` — `ret.Tag = rawDesc.(string)`
- `internal/wa-noise/binary/jid.go:113` — `types.NewADJID(user.(string), ...)`
- `internal/wa-noise/binary/jid.go:82` e `:99` — `User: user.(string)` em
  `readInteropJID` e `readFBJID`

**Problema**: `read()` devolve `interface{}` e pode legitimamente devolver
`nil` (tag `ListEmpty`), `types.JID` ou `[]Node`. As quatro linhas acima
assumem `string` sem checar o `ok`. Um frame que traga qualquer um desses
tipos na posição da tag do nó ou na posição do usuário de um JID faz o
decoder entrar em **panic**, com bytes vindos direto do socket.

Reproduz:

```go
// panic: interface conversion: interface {} is nil, not string
newDecoder([]byte{0xF8, 0x01, 0x00}).readNode()
```

O caminho é alcançável de fora: `binary.Unmarshal` é chamado sobre todo
frame recebido, e o pacote não recupera panic em lugar nenhum. O efeito
prático depende de haver `recover` na goroutine de leitura do socket — se
não houver, é derrubada de processo por frame malformado.

**Correção sugerida**: trocar por type assertion com `ok` e devolver
`ErrInvalidNode` / `ErrInvalidJIDType`. As duas sentinelas já existem em
`binary/errors.go` e os chamadores já tratam erro nesses pontos, então o
fix é local e não muda a assinatura de nada.

**Status**: **não corrigido**. É mudança de comportamento (panic vira erro)
e a Fase C é movimentação mais cobertura. O comportamento atual está travado
por `TestReadNodePanicsOnNonStringTag` e `TestJIDReadersPanicOnNonStringUser`
em `decoder_node_test.go`, que falham com uma mensagem apontando para esta
entrada se o panic deixar de acontecer — ou seja, quem corrigir vai ser
avisado de que precisa reescrever os dois testes como prova do fix.

---

## F25 — `binary.Unpack` dá panic com frame vazio

**Data**: 2026-08-06. **Contexto**: mesmo da F24.

**Onde**: `internal/wa-noise/binary/unpack.go:22`

```go
dataType, data := data[0], data[1:]
```

**Problema**: `data[0]` sem checar `len(data)`. Um frame de zero bytes vindo
do websocket dá `panic: runtime error: index out of range [0] with length 0`.
É o mesmo tipo de exposição da F24, num caminho ainda mais raso: `Unpack` é
a primeira coisa que roda sobre o payload decifrado.

**Correção sugerida**: `if len(data) == 0 { return nil, io.ErrUnexpectedEOF }`
no topo. `Unpack` já devolve erro, então nenhum chamador muda.

**Status**: **não corrigido**, mesmo racional da F24. Travado por
`TestUnpackPanicsOnEmptyFrame` em `unpack_test.go`.

---

## F26 — `Node.GetChildByTag` devolve o nó de partida quando não acha, não um nó zero

**Data**: 2026-08-06. **Contexto**: mesmo da F24.

**Onde**: `internal/wa-noise/binary/node.go` — `GetOptionalChildByTag` e
`GetChildByTag`.

**Problema**: `GetOptionalChildByTag` usa retorno nomeado (`val Node, ok bool`),
inicializa `val = *n` e, quando não encontra a tag, faz um `return` nu — que
devolve `val` com o **último nó alcançado**, não o zero. `GetChildByTag`
descarta o `ok`, então:

```go
n := &binary.Node{Tag: "iq"}
n.GetChildByTag("ausente").Tag   // devolve "iq", não ""
```

Quem checar o resultado por `.Tag != ""` para saber se achou está checando
algo que é sempre verdadeiro. O padrão correto é `GetOptionalChildByTag` com
o `ok`, e o resto do whatsmeow em geral faz isso — mas a armadilha não está
escrita em lugar nenhum.

**Correção sugerida**: nenhuma no código. É API pública do upstream e mudar o
retorno quebraria silenciosamente qualquer chamador que hoje dependa de
receber o nó de partida. O que faltava era documentação.

**Status**: **não corrigido, documentado**.
`TestGetChildByTagReturnsTheStartNodeWhenMissing` em `node_test.go` trava e
explica o comportamento.

---

## F27 — ramo inalcançável em `Node.contentString` (escape de `\n` em conteúdo `[]byte`)

**Data**: 2026-08-06. **Contexto**: mesmo da F24.

**Onde**: `internal/wa-noise/binary/xml.go`, ramo `case []byte` de
`contentString`.

**Problema**: o ramo só é alcançado quando `printable(content)` devolve
string não vazia, e `printable` rejeita qualquer rune que não passe em
`unicode.IsPrint` — `\n` é um deles. Logo o `strings.ReplaceAll(..., "\n",
"\\n")` de dentro desse ramo nunca executa: conteúdo `[]byte` com quebra de
linha sempre cai no ramo de hex. O escape só acontece de fato no `default`,
para conteúdo que não é nem `[]Node` nem `[]byte`.

É cosmético (só afeta o XML de debug), mas é código morto que sugere um
comportamento que não existe.

**Correção sugerida**: ou remover o `ReplaceAll` do ramo `[]byte`, ou fazer
`printable` aceitar `\n` — a segunda muda a saída de log de mensagens de
texto multilinha, que hoje saem em hex.

**Status**: **não corrigido**. Documentado no comentário de
`TestNewlinesAreEscapedWhenNotIndenting` em `xml_test.go`, que exercita os
dois ramos e mostra qual é qual.

---

## F28 — `errors.Is` não funciona com `types.GraphQLError` como alvo

**Data**: 2026-08-06. **Contexto**: Fase C do ADR-0004, cobertura de
`internal/wa-noise/types/`.

**Onde**: `internal/wa-noise/types/newsletter.go` — `GraphQLError` e
`GraphQLErrors.Unwrap`.

**Problema**: `GraphQLErrors` implementa `Unwrap() []error` justamente para
que `errors.Is`/`errors.As` atravessem a lista. Mas `GraphQLError` tem um
campo `Path []string`, o que torna o tipo **não comparável**, e `errors.Is`
só compara alvos comparáveis (`reflectlite.TypeOf(target).Comparable()`).
Resultado: `errors.Is(resp, algumGraphQLError)` percorre a árvore inteira e
devolve `false` **sempre**, inclusive para um erro que está na lista.

O `Unwrap` não é inútil — `errors.As` funciona — mas a assimetria é
invisível: nada em compilação nem em runtime avisa, o `Is` só responde
`false`.

**Correção sugerida**: trocar `Path []string` por um tipo comparável, ou
adicionar um método `Is(error) bool` em `GraphQLError` que compare por
`Extensions.ErrorCode`. A segunda é a menos invasiva e não mexe na
serialização JSON.

**Status**: **não corrigido** (mexer no tipo é mudança de API pública).
`TestGraphQLErrorsUnwrapExposesEveryError` em `newsletter_test.go` mostra que
`errors.As` funciona, que `errors.Is` não, e falha avisando para atualizar
esta entrada se `GraphQLError` virar comparável.

---

## F29 — `internals_generate.go` tem lista de arquivos hardcoded: `go generate` hoje derruba 96 dos 178 wrappers de `DangerousInternals`

**Data**: 2026-08-06. **Contexto**: avaliação do pedido de reorganizar a raiz
de `internal/wa-noise/` em subpacotes (resolução da decisão que a Fase C
deixou pendente em `PATCHES.md`). O achado apareceu ao verificar por que a
raiz é sensível a movimentação de arquivos.

**Onde**: `internal/wa-noise/internals_generate.go:101-110` — o `main()` não
escaneia o diretório, ele carrega uma lista literal de 32 nomes de arquivo:

```go
fileNames := []string{
    "appstate.go", "armadillomessage.go", "broadcast.go", "call.go", "client.go",
    ...
    "presence.go", "privacysettings.go", "push.go", "qrchan.go", "receipt.go", "request.go",
    "retry.go", "sendfb.go", "send.go", "upload.go", "user.go", "reportingtoken.go",
}
```

Esses 32 nomes são os do **upstream**, de antes da Fase A. A Fase A dividiu a
raiz em 94 arquivos e criou nomes novos (`send_encrypt.go`,
`message_decrypt.go`, `retry_recent_messages.go`, `client_connection.go`,
`user_devices.go`, ...) que **nunca foram acrescentados à lista**.

**Problema**: `internals.go` está commitado correto (756 linhas, 178
wrappers) porque foi gerado antes da Fase A e ninguém rodou `go generate`
depois. Mas o gerador e a árvore estão dessincronizados: dos 197 métodos não
exportados de `*Client` que existem hoje, só 82 estão em arquivos que a lista
alcança. Rodar `go generate` **agora** regenera um `internals.go` menor e
derruba 96 wrappers em silêncio — sem erro de compilação, porque
`DangerousInternals` não tem consumidor no repo.

Evidência (reproduzível, sem tocar no repo — copiar `internal/wa-noise/*.go`
para um diretório temporário com um `go.mod` mínimo requerendo
`go.mau.fi/util v0.9.9` e rodar o gerador lá):

```
committed  internals.go: 756 linhas, 178 wrappers
regenerado internals.go: 361 linhas,  82 wrappers   -> 96 perdidos
```

Entre os perdidos estão `Connect`, `AutoReconnect`, `DecryptDM`,
`DecryptGroupMsg`, `DecryptMessages`, `AddRecentMessage`,
`ClearUntrustedIdentity`, `DispatchAppState`.

Agrava que `PATCHES.md` (Fase A, seção "Fora do escopo") isenta
`internals.go` do teto de 300 linhas com a justificativa de que "é recriado
por `go generate`". Na prática **não é** — recriá-lo hoje o quebra. A isenção
continua correta (é código gerado), mas o motivo declarado está falso.

**Correção sugerida**: trocar a lista literal por varredura do diretório —
`filepath.Glob("*.go")` menos `internals.go`, `internals_generate.go` e
`*_test.go`. São ~10 linhas. Risco de runtime zero: o arquivo é
`//go:build ignore`, não entra em nenhum binário. Depois do fix, rodar
`go generate` e commitar o `internals.go` resultante, que passaria a cobrir
os 197 métodos em vez de 82.

**Status**: **não corrigido** — achado incidental, fora do escopo da tarefa
que o encontrou (CLAUDE.md: registrar e perguntar antes de corrigir de
graça). Nenhum gate detecta a regressão hoje: `go generate` não roda em
`make check`, e não há teste que compare `internals.go` com o que o gerador
produziria.

### Adendo ao F29 — a Fase D acrescentou uma segunda dependência ao fix, 2026-08-06

**Contexto**: extração de `msgpad/`, `paircrypto/` e `msgattrs/` da raiz de
`internal/wa-noise/` (commit `d904e78`, documentada em `PATCHES.md` como
Fase D).

Aquela mudança **editou `internals.go` à mão**, que é arquivo gerado. Três
assinaturas passaram a referenciar o tipo movido:

```
internal/wa-noise/internals.go:19    "wa-api/internal/wa-noise/msgattrs"
internal/wa-noise/internals.go:647   SendGroupV3(..., msgAttrs msgattrs.MessageAttrs, ...)
internal/wa-noise/internals.go:651   SendDMV3(..., msgAttrs msgattrs.MessageAttrs, ...)
internal/wa-noise/internals.go:655   PrepareMessageNodeV3(..., msgAttrs msgattrs.MessageAttrs, ...)
```

**Por que isto importa para quem for corrigir o F29**: a correção sugerida
acima (trocar a lista literal por varredura do diretório) **não basta mais**.
O gerador monta o bloco de import copiando apenas os imports do *primeiro*
arquivo da lista — `internals_generate.go:123`:

```go
for _, i := range files[0].Imports {
```

`files[0]` é `appstate.go`, que **não importa `msgattrs`**. Então, mesmo com a
varredura de diretório funcionando, o `internals.go` regenerado sairia com
`msgattrs.MessageAttrs` nas três assinaturas e **sem o import correspondente**
— e desta vez o erro é de compilação, não silencioso como os 96 wrappers
perdidos.

**Correção sugerida (revisada)**: além da varredura de diretório, o gerador
precisa juntar os imports de **todos** os arquivos processados, deduplicando
por caminho e preservando os aliases (`waBinary`, `armadillo`), ou então
delegar isso ao `goimports` que o `//go:generate` já roda logo depois —
emitindo um bloco de import vazio e deixando o `goimports -local` resolver.
A segunda opção é menor e usa maquinário que já está no lugar.

**Status**: **não corrigido**. O `internals.go` commitado está correto e
compila; o risco é exclusivamente para quem rodar `go generate`. Continua sem
gate que detecte: `go generate` não roda em `make check`.

## F30 — `downloadableMessageWithSizeBytes` não tem nenhum implementador: `getSize` cai no default para `StickerPackItem`

**Data / contexto**: 2026-08-07, durante a Fase E do ADR-0004 (lote 1, mídia),
ao escrever teste para os três ramos de `getSize`.

**Onde**:

```go
// internal/wa-noise/download_types.go:82-85
type downloadableMessageWithSizeBytes interface {
	DownloadableMessage
	GetFileSizeBytes() uint64
}

// internal/wa-noise/download_types.go:121-130
func getSize(msg DownloadableMessage) int {
	switch sized := msg.(type) {
	case downloadableMessageWithLength:
		return int(sized.GetFileLength())
	case downloadableMessageWithSizeBytes:
		return int(sized.GetFileSizeBytes())
	default:
		return -1 // hoje unknownFileLength
	}
}
```

```go
// internal/wa-noise/types/sticker.go:51
func (spi *StickerPackItem) GetFileSizeBytes() int64 {
```

**Problema**: a interface exige `uint64`; o único tipo do fork com um método
de mesmo nome, `types.StickerPackItem`, devolve `int64`. Assinaturas
diferentes, logo a interface **não é satisfeita por nenhum tipo de produção**.
`StickerPackItem` também não tem `GetFileLength()`, então `getSize` cai no
`default` e devolve `-1` para todo item de pacote de figurinhas — ou seja, a
validação de tamanho do download (`ErrFileLengthMismatch`) fica desligada
justamente para esse tipo, silenciosamente.

Verificação: `grep -rn "GetFileSizeBytes" internal/wa-noise` devolve só a
declaração da interface, o uso em `getSize` e o método `int64` de
`sticker.go` — nenhum outro implementador. O ramo é inalcançável.

**Correção sugerida**: alinhar as assinaturas. O caminho de menor risco é
mudar a interface para `GetFileSizeBytes() int64` e converter no `getSize`
(`int(sized.GetFileSizeBytes())` continua compilando), já que `int64` é o que
o único candidato real expõe. A alternativa — mudar `StickerPackItem` para
`uint64` — mexe em tipo público consumido fora do fork e ainda perde a
distinção de "não informado".

**Status**: **não corrigido**. Fora do escopo do lote 1 (que é constantes,
logging e testes, sem mudança de comportamento), e a correção altera uma
assinatura pública. O teste do ramo em
`internal/wa-noise/download_types_test.go` usa um tipo falso local com
comentário apontando para este achado.

## F31 — `convertQueryID` compara ponteiros de enum: o ramo de MacOS é inerte

**Data / contexto**: 2026-08-07, Fase E lote 2 (newsletter) do ADR-0004.

**Onde**: `internal/wa-noise/newsletter_mex.go:71`

```go
if payload := cli.Store.GetClientPayload(); payload.GetUserAgent().Platform == waWa6.ClientPayload_UserAgent_MACOS.Enum() || payload.GetWebInfo() == nil {
```

**Problema**: `Platform` é `*ClientPayload_UserAgent_Platform` e
`MACOS.Enum()` **aloca um ponteiro novo** a cada chamada. A comparação é de
endereço, nunca de valor, então o primeiro operando do `||` é **sempre
falso**. Na prática a escolha entre as query IDs web e as de desktop depende
exclusivamente de `payload.GetWebInfo() == nil`.

Verificação: `TestConvertQueryIDPlatformMacOSNaoDecideSozinho`
(`internal/wa-noise/newsletter_mex_test.go`) põe
`store.BaseClientPayload.UserAgent.Platform = MACOS.Enum()` mantendo o
`WebInfo` presente e observa que `convertQueryID` continua devolvendo a ID
web.

**Correção sugerida**: comparar valores, não ponteiros —
`payload.GetUserAgent().GetPlatform() == waWa6.ClientPayload_UserAgent_MACOS`.
Atenção: isso **muda comportamento** para clientes MacOS que ainda mandem
`WebInfo`, que passariam a usar as query IDs de desktop.

**Status**: **não corrigido**. O lote 2 é constantes, logging e testes, sem
mudança de comportamento; corrigir aqui alteraria qual query ID vai para o
servidor. O teste acima trava o estado atual.

## F32 — duas query IDs de desktop de newsletter estão erradas no upstream

**Data / contexto**: 2026-08-07, Fase E lote 2 (newsletter) do ADR-0004.

**Onde**: `internal/wa-noise/newsletter_mex.go:47,50`

```go
mutationUnfollowNewsletterDesktop  = "8782612271820087"
mutationFollowNewsletterDesktop    = "8621797084555037"
querySubscribedNewslettersDesktop  = "8621797084555037" // mesmo valor
```

**Problema**: duas anomalias no mesmo bloco, ambas herdadas do upstream:

1. `mutationFollowNewsletterDesktop` é **idêntica** a
   `querySubscribedNewslettersDesktop`. Em cliente desktop,
   `FollowNewsletter` dispara a consulta de "canais assinados" em vez da
   mutation de seguir — a operação silenciosamente não faz nada.
2. `mutationUnfollowNewsletterDesktop` mapeia, em
   `argo/name-to-queryids.json`, para `WamoSubCancelSubscription`
   (cancelamento de assinatura paga do WhatsApp, não "deixar de seguir
   canal") e esse nome **não existe** no wire type store Argo — é a única
   das dez IDs de desktop sem wire type.

Verificação: `TestQueryIDsDesktopTemWireTypeArgo` cobre as outras nove;
`TestQueryIDUnfollowDesktopSemWireTypeArgo` e
`TestQueryIDsDesktopDuplicadaConhecida`
(`internal/wa-noise/newsletter_mex_test.go`) travam as duas anomalias.

**Correção sugerida**: capturar as IDs corretas de um cliente desktop real
(ou de uma versão mais nova do whatsmeow upstream) e substituir as duas
constantes. Não há como derivar os valores corretos a partir do que está
vendorizado.

**Status**: **não corrigido**. Não temos os valores corretos, e chutar IDs
quebraria também o caminho que hoje ao menos falha de forma previsível. Os
três testes acima falham de propósito se o upstream mudar, forçando revisão.

## F33 — `close` de canal fora do CAS em `qrchan.go` pode fechar duas vezes

**Data / contexto**: 2026-08-07, Fase E lote 4 (pareamento/prekeys/tokens/misc)
do ADR-0004.

**Onde**: `internal/wa-noise/qrchan.go:144`

```go
func (qrc *qrChannel) handleEvent(rawEvt interface{}) {
	if atomic.LoadUint32(&qrc.closed) == 1 {   // <- checagem
		return
	}
	...
	close(qrc.stopQRs)                          // <- fora do CAS
	if atomic.CompareAndSwapUint32(&qrc.closed, 0, 1) {
		...
	}
}
```

**Problema**: entre o `LoadUint32` e o `close`, outra goroutine pode entrar
na mesma função com o mesmo resultado na checagem. Os dois caminhos chegam ao
`close(qrc.stopQRs)` e o segundo entra em pânico com `close of closed
channel`. O `close` está deliberadamente **fora** do `CompareAndSwap` logo
abaixo — que é justamente o que serializaria os dois.

Alcançável quando dois eventos terminais chegam próximos (ex.:
`*events.PairError` seguido de `*events.Disconnected`, que é a sequência
normal de uma falha de pareamento, já que `handlePairSuccess` chama
`cli.Disconnect()` no ramo de erro).

Não é fatal para o processo: `handleEvent` é registrado via
`AddEventHandler` e roda dentro de `dispatchEvent`, que tem `recover()`
(`client_events.go:227`). O dano é local — o emissor de QR morre, o canal
devolvido a `GetQRChannel` fica sem item final, e quem estiver lendo dele
bloqueia até o contexto expirar.

**Correção sugerida**: mover o `close(qrc.stopQRs)` para **dentro** do
`CompareAndSwap` que já existe logo abaixo, ou proteger com um
`sync.Once`. A primeira opção é a menor, mas muda a ordem observável
(hoje o emissor recebe o sinal de parada antes de o item final ir para o
canal).

**Status**: **não corrigido**. O lote 4 é constantes, logging e testes; a
única mudança de comportamento feita nele foi o panic de `handlePairSuccess`,
que é fatal para o processo. Este é recuperável e a correção mexe em ordem de
operações num caminho concorrente — precisa de decisão consciente.

## F34 — código morto em `decodeFBArmadillo`

**Data / contexto**: 2026-08-07, Fase E lote 4 do ADR-0004.

**Onde**: `internal/wa-noise/armadillomessage.go:88-111`

```go
var protoMsg proto.Message
var subData *waCommon.SubProtocol
switch subProtocol := typedContent.SubProtocol.GetSubProtocol().(type) {
	// ... nenhum ramo atribui protoMsg nem subData
}
if protoMsg != nil {                       // <- sempre falso
	err = proto.Unmarshal(subData.GetPayload(), protoMsg)
	...
}
```

**Problema**: `protoMsg` e `subData` são declarados e nunca recebem valor em
nenhum dos 7 ramos do `switch`. O `if protoMsg != nil` é portanto sempre
falso, e o `proto.Unmarshal` dentro dele é inalcançável. Se algum dia o ramo
fosse atingido com `subData` nil, `subData.GetPayload()` ainda funcionaria
(getter de protobuf tolera receptor nil), mas o `proto.Unmarshal` receberia
payload vazio.

Herdado do upstream — parece resíduo de uma refatoração em que a
desserialização passou a acontecer dentro de cada `subProtocol.Decode()`.

**Correção sugerida**: remover as duas declarações e o bloco `if`. É deleção
pura de código inalcançável, sem mudança de comportamento observável.

**Status**: **não corrigido**. Deleção é segura, mas o lote 4 não tocou em
`armadillomessage.go` além da auditoria, e remover código do upstream aumenta
a divergência de reconciliação sem ganho funcional. Fica para decisão.

## F35 — `shouldSendCsToken` e `shouldSendTCTokenInChatAction` são idênticas

**Data / contexto**: 2026-08-07, Fase E lote 4 do ADR-0004.

**Onde**: `internal/wa-noise/cstoken.go:17` e `internal/wa-noise/tctoken.go:50`

As duas funções têm corpo byte a byte idêntico:

```go
jid = jid.ToNonAD()
return (jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer) &&
	jid.User != types.PSAJID.User &&
	!jid.IsBot()
```

**Problema**: duplicação exata. Uma mudança de política aplicada a uma e
esquecida na outra passa despercebida.

**Correção sugerida**: manter as duas assinaturas (são o vocabulário de dois
protocolos) e fazer uma delegar à outra, ou ambas a um helper comum
`isDirectHumanChat(jid)`.

**Status**: **não corrigido, e provavelmente não deve ser**. São políticas de
protocolos distintos (trusted contact token × contact safety token) que
coincidem hoje por acaso; unificar acopla os dois. `TestShouldSendTokenPerJIDType`
(`internal/wa-noise/tctoken_test.go`) roda a mesma tabela de 7 tipos de JID
nas duas, então uma divergência futura aparece como falha de teste em vez de
passar batido. Registrado para visibilidade, não como dívida a pagar.

---

## F36 — os dois contadores de retry crescem sem limite, chaveados por dado do servidor

**Data / contexto**: 2026-08-07, Fase E lote 5 do ADR-0004 (notificação/retry/recibo).

**Onde**:

- `internal/wa-noise/retry.go:110-113` — `cli.incomingRetryRequestCounter`,
  `map[incomingRetryKey]int` com `incomingRetryKey{jid, messageID}`.
- `internal/wa-noise/retry_receipt_send.go:29-37` — `cli.messageRetries`,
  `map[string]int` chaveado pelo ID da mensagem.

```go
// retry.go
retryKey := incomingRetryKey{receipt.Sender, messageID}
cli.incomingRetryRequestCounterLock.Lock()
cli.incomingRetryRequestCounter[retryKey]++
```

**Problema**: nenhum dos dois mapas é limpo em lugar nenhum do pacote
(`grep -rn "incomingRetryRequestCounter\|messageRetries" internal/wa-noise/*.go`
só acha a criação em `client.go:233,239` e os incrementos acima). As chaves
são o remetente e o ID da mensagem — **os dois vêm do servidor**. Uma sessão
longa acumula uma entrada por mensagem que já precisou de retry, para sempre;
um par malicioso acumula uma entrada por ID inventado. Não é panic, é
crescimento de memória monotônico proporcional ao que o outro lado mandar, e
não há reset nem no `Disconnect`.

Contraste: o buffer de mensagens recentes (`recentMessagesList`), no mesmo
domínio, é circular de `recentMessagesSize` justamente para não ter esse
problema — o despejo está travado por
`TestAddRecentMessageEvictsOldestAfterFullCircle`.

**Correção sugerida**: dar aos dois a mesma disciplina do buffer circular, ou
guardar `time.Time` junto do contador e varrer entradas mais velhas que uma
janela (a mesma `recreateSessionTimeout` de 1h seria um teto natural), ou no
mínimo esvaziar ambos em `Disconnect`/`ResetConnection`.

**Status**: **não corrigido**. Herdado do upstream, exige escolher uma política
de despejo — o que é mudança de comportamento observável (um retry legítimo
depois do despejo volta a ser aceito), e o lote 5 é de qualidade estrutural
por contrato. Pendente de decisão.

---

## F37 — `GetUserDevices` grava o cache numa chave que pode nunca ser consultada

**Data / contexto**: 2026-08-07, Fase E lote 7 (usuário, raiz de
`internal/wa-noise/`).

**Onde**: `internal/wa-noise/user_devices.go:34` e `:60`.

```go
cached, ok := cli.userDevicesCache[jid]        // :34 — lookup pelo JID de ENTRADA
...
cli.userDevicesCache[jid] = deviceCache{...}   // :60 — escrita pelo JID da RESPOSTA
```

**Problema**: o hit de cache é procurado pelo JID que o chamador passou; a
entrada é gravada com o JID que veio no `<user jid=...>` da resposta usync. Se o
servidor responder com o outro lado do par LID/PN (o que o protocolo permite —
`usync` aceita LID como entrada por `user_usync.go:44`), a entrada fica numa
chave que o `lookup` nunca consulta: o cache nunca acerta, e cada envio para
aquele contato refaz a consulta de dispositivos. Não é panic nem dado errado, é
trabalho repetido silenciosamente — e o mapa cresce com as duas chaves.

**Correção sugerida**: gravar sob as duas chaves quando os JIDs divergirem, ou
resolver o JID de entrada para a forma canônica (via `cli.Store.LIDs`) antes do
lookup e da escrita.

**Status**: **não corrigido**. Confirmar se o servidor de fato responde com JID
diferente do consultado exige tráfego real; sem isso a correção é especulação, e
mudaria a chave de um cache quente. Pendente de decisão.

---

## F38 — godoc de `GetUserDevices` promete exclusão do próprio dispositivo que o código não faz

**Data / contexto**: 2026-08-07, Fase E lote 7.

**Onde**: `internal/wa-noise/user_devices.go:22-24`.

> The local device will not be included in the output even if the user's JID is
> included in the input.

**Problema**: não há nenhuma filtragem do próprio dispositivo no corpo da
função. `parseDeviceList` devolve todos os `<device>` que o servidor mandou, e
`GetUserDevices` só concatena. Ou o comentário está desatualizado desde alguma
versão do upstream, ou a exclusão passou a morar no chamador — em qualquer caso
o godoc mente hoje.

**Correção sugerida**: rastrear quem consome `GetUserDevices` (`send.go`,
`sendfb.go`) e decidir entre (a) reimplementar o filtro aqui, restaurando o
contrato documentado, ou (b) corrigir o comentário para descrever o que a função
faz.

**Status**: **não corrigido**. As duas saídas mudam algo observável (o
comportamento ou o contrato público). Pendente de decisão.

---

## F39 — `updateBusinessName` loga "push name" ao falhar gravando business name

**Data / contexto**: 2026-08-07, Fase E lote 7.

**Onde**: `internal/wa-noise/user_business.go:121-123`.

```go
_, _, err = cli.Store.Contacts.PutBusinessName(ctx, userAlt, name)
if err != nil {
	cli.Log.Errorf("Failed to save push name of %s in device store: %v", userAlt, err)
}
```

**Problema**: mensagem copiada de `updatePushName` (`user.go:160`) e nunca
ajustada — a chamada que falhou é `PutBusinessName`. O log manda investigar o
lugar errado. As outras duas mensagens da mesma função (`:114` e `:126`) dizem
"business name" corretamente; só esta divergiu.

**Correção sugerida**: trocar `push name` por `business name` na string.

**Status**: **não corrigido**. É um caractere de risco quase zero, mas altera
uma string de log que pode estar sendo casada em alerta/dashboard, e o lote 7 é
de qualidade estrutural por contrato. Pendente de decisão — trivial de aplicar.

---

## F40 — `getCachedGroupData` devolve `(nil, nil)` quando o servidor ecoa outro `id`

**Data / contexto**: 2026-08-07, Fase E lote 8 (envio de mensagem).

**Onde**: `internal/wa-noise/group.go:185-196` e `group.go:144`.

```go
// group.go:144 — grava sob a chave que o SERVIDOR devolveu
cli.groupCache[groupInfo.JID] = &groupMetaCache{...}

// group.go:185-196 — le sob a chave CONSULTADA
func (cli *Client) getCachedGroupData(ctx context.Context, jid types.JID) (*groupMetaCache, error) {
	if val, ok := cli.groupCache[jid]; ok {
		return val, nil
	}
	_, err := cli.getGroupInfo(ctx, jid, false)
	if err != nil {
		return nil, err
	}
	return cli.groupCache[jid], nil // <- nil, nil se groupInfo.JID != jid
}
```

**Problema**: a escrita usa `groupInfo.JID` (o `id` que o servidor devolveu no
`<group>`) e a leitura usa o `jid` consultado. Se o servidor responder com um
`id` diferente, o cache é gravado sob outra chave e a última linha devolve
`nil, nil` — sucesso aparente com ponteiro nulo. Os dois consumidores do lote 8
desreferenciavam esse nil direto (`cachedData.AddressingMode` em
`send_prepare.go:181`, `groupMeta.Members` em `sendfb_transport.go:96`): panic
remoto no caminho de envio. O primeiro ramo (`val, ok := ...`) tem o mesmo
problema se algum dia um `nil` for gravado no mapa.

**Corrigido nesta sessão apenas nos chamadores** (guarda de `nil` devolvendo
`ErrGroupNotFound`, travada por `TestResolveGroupSendTargetNilCachedData`,
`TestSendGroupV3NilCachedData` e `TestSendGroupV3NonGroupDestination`). A causa
raiz continua em `group.go`.

**Correção sugerida**: em `getCachedGroupData`, devolver
`nil, ErrGroupNotFound` (ou um erro dedicado) em vez de `nil, nil` quando o
lookup pós-`getGroupInfo` falhar; ou alinhar a chave de escrita com a de
consulta. `group.go` é escopo do lote 6, já fechado.

**Status**: **não corrigido na raiz** (fora do escopo do lote 8; os chamadores
do lote 8 estão protegidos). Pendente de decisão.

---

## F41 — `makeDeviceIdentityNode` faz `panic` dentro do caminho de envio

**Data / contexto**: 2026-08-07, Fase E lote 8.

**Onde**: `internal/wa-noise/send_node_build.go:225-229`.

```go
deviceIdentity, err := proto.Marshal(cli.Store.Account)
if err != nil {
	panic(fmt.Errorf("failed to marshal device identity: %w", err))
}
```

**Problema**: `panic` cru num helper chamado de `getMessageContent` e de
`preparePeerMessageNode` — ou seja, dentro de `SendMessage`. Não é
disparável por dado do servidor (o argumento é local) e `proto.Marshal` de um
`*waAdv.ADVSignedDeviceIdentity` bem formado não falha, então na prática é
inalcançável. Mas é o único `panic` do caminho de envio, e derruba o processo em
vez de devolver erro. Caso correlato, silencioso: se `cli.Store.Account` for
`nil`, `proto.Marshal` devolve bytes vazios **sem erro** e o nó
`<device-identity>` vai vazio para o fio.

**Correção sugerida**: mudar a assinatura para `(waBinary.Node, error)` e
propagar; os dois chamadores já devolvem erro. Custo: `getMessageContent` passa
a devolver erro e o `internals.go` gerado precisa ser regerado.

**Status**: **não corrigido**. Muda assinatura de função usada pelo
`DangerousInternalClient` gerado, e o lote 8 é de qualidade estrutural.
Pendente de decisão.

---

## F42 — atributo `v` do nó `<enc>` é numérico no caminho v3/FB e string no waE2E

**Data / contexto**: 2026-08-07, Fase E lote 8.

**Onde**: `internal/wa-noise/sendfb_encrypt.go:158` vs
`internal/wa-noise/send_encrypt.go:180` e `sendfb_transport.go:107`.

```go
// sendfb_encrypt.go — int
encAttrs := waBinary.Attrs{encAttrVersion: FBMessageVersion, ...} // 3 (int)
// send_encrypt.go / sendfb_transport.go — string
Attrs: waBinary.Attrs{encAttrVersion: encVersionSignal, ...}      // "2"
Attrs: waBinary.Attrs{encAttrVersion: encVersionFB, ...}          // "3"
```

**Problema**: o mesmo atributo do mesmo tipo de nó é escrito ora como `int`, ora
como `string`. O encoder binário aceita ambos, mas a codificação de saída pode
diferir (token vs. inteiro), e é a única inconsistência do tipo no caminho de
envio. Ou o servidor tolera as duas formas (e a divergência é acidental), ou uma
das duas está errada e nunca foi notada porque o caminho v3/FB é pouco usado.

**Correção sugerida**: confirmar contra captura de tráfego real qual forma o
cliente oficial usa e uniformizar. Não dá para decidir por leitura de código.

**Status**: **não corrigido**. É formato de fio no caminho de criptografia —
mudar sem evidência é exatamente o tipo de palpite que o lote 8 proíbe.
Registrado com comentário no código apontando a divergência.

---

## F43 — nomes de tipo de mensagem duplicados entre `msgattrs` e a raiz

**Data / contexto**: 2026-08-07, Fase E lote 8.

**Onde**: `internal/wa-noise/msgattrs/message.go:32-40` (produz `"reaction"`,
`"poll"`, `"media"`, `"text"` como literais) e
`internal/wa-noise/send_constants.go` (`msgTypeText`, `msgTypePoll`,
`msgTypeReaction`, que comparam contra esses valores).

**Problema**: a taxonomia de tipo de mensagem tem duas fontes — `msgattrs`
produz literais crus, a raiz compara contra constantes próprias. Se um valor
mudar de um lado, o outro compila e falha em silêncio (o `if` simplesmente
para de casar, e o `<meta polltype>` some do nó sem erro nenhum).

**Correção sugerida**: exportar as constantes de `msgattrs`
(`msgattrs.TypeText` etc.) e fazer tanto `GetTypeFromMessage` quanto a raiz
usarem as mesmas. `msgattrs` é escopo da Fase D, já fechada.

**Status**: **não corrigido** (fora do escopo do lote 8, que é a raiz).
Pendente de decisão.

## F44 — envio bloqueante para `historySyncNotifications` antes de iniciar o loop

**Data / contexto**: 2026-08-07, Fase E lote 9 (recepção/decriptação).

**Onde**: `internal/wa-noise/message.go:26-32`, com o canal declarado em
`internal/wa-noise/client.go:81` e criado em `client.go:241`.

```go
if !cli.ManualHistorySyncDownload {
    cli.historySyncNotifications <- protoMsg.HistorySyncNotification
    if cli.historySyncHandlerStarted.CompareAndSwap(false, true) {
        go cli.handleHistorySyncNotificationLoop()
    }
}
```

**Problema**: o envio é bloqueante e acontece **antes** de o consumidor ser
iniciado. O canal tem buffer 32. Se o buffer encher e o consumidor estiver
travado — `DownloadHistorySync` faz um `cli.Download` HTTP —, o goroutine que
trata a mensagem recebida bloqueia indefinidamente, segurando o processamento
do stanza. Na prática o `defer`/`recover` de `handleHistorySyncNotificationLoop`
(`message_history_sync.go:49-63`) religa o loop quando sobra algo no canal, o
que drena o buffer no caso normal; o risco é o consumidor pendurado.

**Correção sugerida**: nenhuma das duas saídas óbvias serve como está — envio
não-bloqueante (`select` com `default`) e envio com timeout ambos **descartam**
notificações de history sync, perdendo histórico em silêncio, o que é pior que o
travamento raro que evitam. O caminho provável é iniciar o loop *antes* do envio
e dar timeout ao download, mas isso é decisão de projeto sobre a política de
history sync, não patch pontual.

**Status**: **não corrigido** (fora do escopo do lote 9, que é higienização, e
qualquer correção aqui muda política de entrega). Pendente de decisão.

## F45 — `PutManyLIDMappings` chamado com fatia vazia

**Data / contexto**: 2026-08-07, Fase E lote 9.

**Onde**: `internal/wa-noise/message_secrets_store.go:182`.

**Problema**: quando **todos** os pares do history sync falham no
`types.ParseJID` (os dois `continue` logo acima), `lidPairs` fica vazia e
`PutManyLIDMappings` é chamado assim mesmo, gerando uma escrita inútil e uma
linha de log `"Stored PN-LID mappings from history sync"` com `pair_count: 0` —
que lida sozinha sugere sucesso quando na verdade nada foi mapeado.

**Correção sugerida**: `if len(lidPairs) == 0 { return }` antes da chamada, como
`storeHistoricalMessageSecrets` já faz para `secrets` e `privacyTokens`.

**Status**: **não corrigido** — inofensivo, e o lote 9 se limitou a corrigir o
que tem consequência real. Pendente de decisão.

## F46 — corrida de dados em `LastSuccessfulConnect` e `AutoReconnectErrors`

**Data / contexto**: 2026-08-07, Fase E lote 10 (núcleo do client/conexão).

**Onde**: `internal/wa-noise/connectionevents.go:160-161` (escrita) e
`internal/wa-noise/client_connection.go:194,196` (leitura e escrita).

```go
// connectionevents.go, handleConnectSuccess — goroutine do handler de nó
cli.LastSuccessfulConnect = time.Now()
cli.AutoReconnectErrors = 0

// client_connection.go, autoReconnect — outro goroutine
autoReconnectDelay := time.Duration(cli.AutoReconnectErrors) * autoReconnectDelayStep
cli.AutoReconnectErrors++
```

**Problema**: os dois campos são escritos por `handleConnectSuccess`, que roda
no goroutine do `handlerQueueLoop`, e lidos/escritos por `autoReconnect`, que
roda em outro goroutine (disparado por `go cli.autoReconnect(ctx)` em
`onDisconnect` e em `ConnectContext`). Não há mutex nem `atomic` protegendo
nenhum dos dois. É corrida de dados pelo modelo de memória do Go: o backoff pode
ser calculado sobre um contador obsoleto, e `LastSuccessfulConnect` pode ser lido
rasgado. Na prática o efeito visível é um atraso de reconexão errado, não perda
de dados — mas é comportamento indefinido, e `go test -race` não acusa porque
nenhum teste exercita os dois caminhos ao mesmo tempo (o que exigiria socket
vivo).

**Correção sugerida**: `AutoReconnectErrors` viraria `atomic.Int64` e
`LastSuccessfulConnect` seria guardado sob o `socketLock` já existente, ou num
`atomic.Pointer[time.Time]`.

**Status**: **não corrigido**. Os dois campos são **exportados** — fazem parte
da API pública do fork, e `AutoReconnectErrors` é explicitamente documentado em
`client.go:57-58` como legível pelo `AutoReconnectHook`. Trocar o tipo quebra
consumidores e cria divergência permanente contra o upstream, que é exatamente o
que `PATCHES.md` existe para minimizar. Precisa de decisão do usuário sobre
aceitar a mudança de API.

---

## F47 — `UploadNewsletterReader` engole o erro de `io.Copy`

**Data / contexto**: 2026-08-07, durante a Fase F/G lote 1 do ADR-0004
(extração de `internal/wa-noise/media/`). Achado ao mover o arquivo, não ao
procurar bugs.

**Onde**: `internal/wa-noise/media/upload_newsletter.go:35-44` (era
`internal/wa-noise/upload_newsletter.go:60-72` antes da extração).

```go
hasher := sha256.New()
var fileLength int64
fileLength, err = io.Copy(hasher, data)   // <- err atribuido aqui...
resp.FileLength = uint64(fileLength)
resp.FileSHA256 = hasher.Sum(nil)
_, err = data.Seek(0, io.SeekStart)       // <- ...e sobrescrito aqui, sem ser checado
if err != nil {
    err = fmt.Errorf("failed to seek to start of data: %w", err)
    return
}
```

**Problema**: o erro de `io.Copy` nunca é checado — é sobrescrito pela
atribuição seguinte. Se a leitura do `io.ReadSeeker` do chamador falhar no meio,
`resp.FileLength` e `resp.FileSHA256` ficam calculados sobre um conteúdo
**parcial**, o `Seek` volta ao início e o upload segue normalmente. O servidor
recebe o conteúdo completo (o `RawUpload` relê o mesmo reader) mas com um
`FileSHA256` e um `FileLength` que descrevem só o pedaço lido antes da falha.
O resultado é uma mídia de newsletter publicada com hash errado: o destinatário
que validar o `FileSHA256` recusa o anexo, e o erro original desaparece sem
rastro nenhum — nem log.

**Reprodução**: passar um `io.ReadSeeker` que devolve `n>0` e depois um erro
(um `iotest.TimeoutReader` sobre um `bytes.Reader`, com `Seek` funcional).
`UploadNewsletterReader` devolve `err == nil` e um `FileSHA256` que não é o hash
do conteúdo completo.

**Correção sugerida**: checar o erro logo após o `io.Copy`, antes de preencher
`resp`:

```go
fileLength, err = io.Copy(hasher, data)
if err != nil {
    err = fmt.Errorf("failed to hash data: %w", err)
    return
}
```

Duas linhas, sem mudança de assinatura. O caminho equivalente com cifra
(`UploadReader`) já faz isso: checa o erro de `cbcutil.EncryptStream` antes de
seguir.

**Status**: **não corrigido**. Está fora do escopo do lote 1 da Fase F/G, que é
extração de pacote com equivalência de comportamento — corrigir aqui misturaria
uma mudança de comportamento numa movimentação que precisa ser auditável como
"nada mudou". Conforme `CLAUDE.md`, fica registrado para decisão do usuário
sobre corrigir agora ou depois.

## F48 — `ConvertQueryID` acessa `.Platform` em vez de `GetPlatform()`

- **Data / contexto**: 2026-08-07, durante a extração do subpacote
  `internal/wa-noise/newsletter/` (Fase F/G, lote 2). O bug apareceu quando o
  duble de teste montou um `*waWa6.ClientPayload` sem `UserAgent`.
- **Onde**: `internal/wa-noise/newsletter/queryids.go:50` (era
  `internal/wa-noise/newsletter_mex.go:71` antes da extração).

  ```go
  if payload.GetUserAgent().Platform == waWa6.ClientPayload_UserAgent_MACOS.Enum() || payload.GetWebInfo() == nil {
  ```

- **Problema**: `GetUserAgent()` devolve `nil` quando o campo está ausente, e
  `.Platform` é acesso a CAMPO, não ao getter gerado — o programa estoura
  `SIGSEGV` em vez de ler o zero. Reproduzido em teste:
  `ConvertQueryID(&waWa6.ClientPayload{}, queryFetchNewsletter)` panica em
  `queryids.go:50`. Em produção o payload vem de
  `store.Device.GetClientPayload()`, que sempre preenche `UserAgent`, então o
  caminho não é alcançado hoje — é fragilidade latente, não falha ativa.

  Note que a comparação em si já é inerte: ela compara dois PONTEIROS
  diferentes e é sempre falsa (F31). Ou seja, o acesso arriscado não influencia
  o resultado — só pode panicar.
- **Correção sugerida**: trocar `.Platform` por `.GetPlatform()` e comparar com
  `waWa6.ClientPayload_UserAgent_MACOS` (valor, não ponteiro), o que de quebra
  corrige F31. Atenção: corrigir F31 MUDA comportamento — clientes com
  plataforma MACOS passariam a usar as query IDs de desktop mesmo com WebInfo
  presente. Precisa ser decisão deliberada, não conserto de passagem.
- **Status**: não corrigido. Fora do escopo de uma extração; o teste
  `TestConvertQueryIDPlatformMacOSNaoDecideSozinho` trava o comportamento atual e
  os helpers `webPayload`/`desktopPayload` documentam a pré-condição.

## F49 — `EncodePatch` escreve em `MutationInfo.Value` sem checar nil

- **Data / contexto**: 2026-08-07, durante a extração do subpacote
  `internal/wa-noise/appstatesync/` (Fase F/G, lote 3). Apareceu quando um teste
  do envio montou um `appstate.PatchInfo` com `Mutations[i].Value` nil.
- **Onde**: `internal/wa-noise/appstate/encode.go:50`.

  ```go
  for _, mutationInfo := range patchInfo.Mutations {
      mutationInfo.Value.Timestamp = proto.Int64(patchInfo.Timestamp.UnixMilli())
  ```

- **Problema**: `Value` é `*waSyncAction.SyncActionValue` e a linha grava um
  campo dele sem checar nil — `SIGSEGV` se o chamador não preencher. Não é
  acesso via getter gerado (que toleraria nil), é escrita direta. Reproduzido:

  ```
  panic: runtime error: invalid memory address or nil pointer dereference
  wa-api/internal/wa-noise/appstate.(*Processor).EncodePatch(...)
      internal/wa-noise/appstate/encode.go:50
  ```

  Em produção os patches vêm dos construtores `appstate.Build*`
  (`BuildMute`, `BuildPin`, ...), que sempre preenchem `Value`, então o caminho
  não é alcançado hoje. É fragilidade de API pública: `PatchInfo` é tipo
  exportado e qualquer chamador pode montá-lo à mão — inclusive por
  `Client.SendAppState`, que é API pública do fork.
- **Correção sugerida**: `if mutationInfo.Value == nil { return nil, fmt.Errorf(...) }`
  no topo do laço, devolvendo erro em vez de panicar. Alternativa mais
  permissiva: criar um `&waSyncAction.SyncActionValue{}` vazio. A primeira é
  preferível — um patch sem valor é erro do chamador, não algo a preencher em
  silêncio.
- **Status**: **não corrigido**. Está em `internal/wa-noise/appstate/`, que o
  lote 3 explicitamente não toca (`git diff --stat internal/wa-noise/appstate/`
  tem de continuar vazio). Registrado para decisão do usuário. O teste
  `sendPatch()` em `appstatesync/send_test.go` documenta a pré-condição num
  comentário citando `encode.go:50`.

## F50 — `phoneLinkingCache` é lido e escrito de goroutines diferentes sem sincronização

- **Data**: 2026-08-07.
- **Contexto**: Fase F/G lote 4, ao extrair `internal/wa-noise/pairing/`. O
  achado é do código ORIGINAL (upstream), não da extração.
- **Onde**: no `HEAD` anterior ao lote, `internal/wa-noise/client.go:168`
  (campo `phoneLinkingCache *phoneLinkingCache`), com escrita em
  `pair-code.go:137` (`PairPhone`) e leitura em `pair-code.go:161`
  (`handleCodePairNotification`). Hoje o mesmo estado vive em
  `internal/wa-noise/pairing/state.go` (`State.linking`), com o mesmo desenho.
- **Problema**: `PairPhone` é chamado pela aplicação; `handleCodePairNotification`
  roda a partir de um handler de notificação, em **outra goroutine**. O campo é
  um ponteiro comum — sem mutex, sem atômico, sem canal. É corrida de dados pelo
  modelo de memória de Go: sem happens-before entre a escrita e a leitura, a
  goroutine de notificação pode enxergar `nil` (e devolver "received code pair
  notification without a pending pairing") ou, em tese, um `*LinkingCache`
  parcialmente publicado. `git grep -n phoneLinkingCache HEAD -- 'internal/wa-noise/*.go'`
  devolve exatamente quatro ocorrências (declaração do campo, declaração do tipo,
  a escrita e a leitura) — nenhuma perto de um lock. Confirmado por revisor
  independente durante o lote 4.
  Na prática a janela é estreita: o servidor só manda a notificação depois de
  responder ao `companion_hello`, e a resposta desse IQ é o que destrava
  `PairPhone` — mas essa ordenação é do protocolo, não do código, e não
  estabelece happens-before nenhum para o compilador nem para o hardware.
- **Correção sugerida**: trocar `State.linking` por `atomic.Pointer[LinkingCache]`
  (`Linking()` vira `Load()`, `SetLinking()` vira `Store()`). É a menor mudança
  que fecha a corrida: mesma semântica de "último escritor ganha", sem lock e sem
  alterar o fluxo. Um `sync.RWMutex` também serviria, com mais cerimônia. Um
  teste sob `-race` com `PairPhone` e `HandleCodeNotification` concorrentes
  falharia hoje e passaria depois.
- **Status**: **não corrigido**. É bug pré-existente fora do escopo do lote 4,
  que era extração; alterar sincronização em código de criptografia de
  pareamento sem pedir é exatamente o que o CLAUDE.md manda não fazer. O
  comportamento foi preservado bit a bit e a decisão está documentada no doc de
  `pairing.State` (`state.go`) e em `PATCHES.md`, seção do lote 4.

## F51 — `prekeys.Upload` indexa `preKeys[len(preKeys)-1]` sem checar lista vazia

- **Data**: 2026-08-07.
- **Contexto**: Fase F/G lote 4, ao extrair `internal/wa-noise/prekeys/`. Achado
  do código ORIGINAL.
- **Onde**: `internal/wa-noise/prekeys/upload.go`, no fim de `Upload`:

  ```go
  preKeys, err := t.Store().PreKeys.GetOrGenPreKeys(ctx, uint32(wantedCount))
  if err != nil { ...; return }
  ...
  err = t.Store().PreKeys.MarkPreKeysAsUploaded(ctx, preKeys[len(preKeys)-1].KeyID)
  ```

  Era `internal/wa-noise/prekeys.go:89` antes da extração, com o mesmo corpo.
- **Problema**: se `GetOrGenPreKeys` devolver slice vazia com `err == nil`, a
  indexação `preKeys[len(preKeys)-1]` é `preKeys[-1]` e entra em pânico. O
  `wantedCount` é sempre ≥ 50, então a implementação SQL não devolve vazio hoje;
  mas `PreKeyStore` é interface pública (`store.PreKeyStore`) e nada no contrato
  dela proíbe uma implementação de devolver `(nil, nil)`. `Upload` roda em
  goroutine a partir de `connectionevents.go` e de `notification.go`, sem
  recover, então o pânico derruba o processo.
- **Correção sugerida**: `if len(preKeys) == 0 { t.Log().Warnf(...); return }`
  logo depois da checagem de erro, antes do `Infof` de "Uploading %d new
  prekeys". Também evita enviar um `<list>` vazio ao servidor.
- **Status**: **não corrigido**. Bug pré-existente fora do escopo do lote 4, que
  era extração pura; preservado bit a bit. Registrado para decisão do usuário.
