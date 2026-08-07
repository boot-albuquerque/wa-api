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
