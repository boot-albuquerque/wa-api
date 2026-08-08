# Housekeep

Registro de bugs, gaps e dívidas técnicas encontrados durante sessões de
trabalho, mas fora do escopo do que estava sendo feito naquele momento.
Não é backlog de features — é achado incidental que alguém precisa decidir
o que fazer (corrigir, ignorar, virar issue formal).

Cada entrada deve ter: data, contexto de onde foi encontrado, arquivo(s) e
linha(s) exatos, descrição do problema, e se possível o caminho de correção
sugerido. Sem isso, o achado se perde ou vira arqueologia de código na
próxima vez que alguém tropeçar nele.

**Escopo:** o arquivo mora em `internal/wa-noise/` porque é de lá que vem a
grande maioria dos achados, mas o registro é do **repositório inteiro** — há
entradas sobre o `Makefile` (o bug de locale do `coverage-gate`), sobre
`pkg/infra/` e sobre `pkg/domain/apperr`. Não filtre por diretório ao
registrar.

## Índice

Situação em 2026-08-07, depois das levas de saneamento (lotes A–I) e da
pesquisa comparativa com o evolution-api/Baileys. São 49 achados:
**43 resolvidos** (42 corrigidos + F35 fechado como "não corrigir"),
**6 abertos** — 2 travados por falta de informação externa (F42, F44), 3 da
reorganização de `pkg/infra/wa-noise/` (F59, F60, F61) e o inventário de TODOs
(F64), que é registro de decisão e não trabalho pendente — ela cataloga
item a item os 38 `// TODO` do projeto, com o critério que resolveria cada um.

> **Correção de registro (2026-08-07):** este índice ficou desatualizado em
> relação às próprias entradas. Ele listava 6 abertos, mas `user_info_failed`,
> F22, F32/F31 e F37 já constavam como **CORRIGIDO**/**RESOLVIDO** no corpo de
> cada entrada. Um índice que contradiz as entradas é pior que não ter índice —
> ele é o que se lê primeiro. Ao fechar um achado, atualize os dois.

Os 2 abertos têm em comum não faltar trabalho, e sim faltar **informação que não
está no código**. Nenhum é resolvível por leitura ou refactor:

| Achado                                     | O que falta para resolver                                                                                                                                                                                                                                                                   |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| F42 — atributo `v` numérico vs string      | **Captura de tráfego.** É formato de fio no caminho de criptografia; decidir por leitura de código é palpite. O Baileys usa string `'2'` nos três pontos onde monta `<enc>`, mas não implementa o caminho v3/FB, que é justamente onde o nosso é numérico.                                   |
| F44 — envio bloqueante de history sync     | **Decisão de política de produto.** A ordem de partida do loop foi corrigida; resta o timeout de download. As duas saídas óbvias **descartam** notificações de history sync, perdendo histórico em silêncio — pior que o travamento raro que evitam. O Baileys não tem fila: trata inline.   |

Fora da lista, uma pendência que não é achado: **o smoke test manual**
(pareamento por QR, envio, avatar, criação de grupo, reconexão) continua sem ser
executado — exige ambiente com sessão real, e é bloqueador de merge.

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

**Status**: **CORRIGIDO** — na verdade já havia sido, num lote posterior da
Fase F/G, e só o status aqui ficou desatualizado. O código de hoje
(`internal/wa-noise/capabilities/newsletter/mex.go:88-91`) faz
`t.Log().Errorf(...)` seguido de `return nil, fmt.Errorf(...)`: sai pelo logger
do cliente e devolve erro, sem `os.Exit`. Verificado em 2026-08-07 com
`grep -rn "log.Fatalf" internal/wa-noise/` — nenhuma ocorrência em código,
só citações históricas em `PATCHES.md`. Texto original preservado abaixo
como registro de por que era grave. (Restante da entrada original: deixar
para a fase que tratar erros de
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
  todas do estilo do wa-noise upstream (ex:
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
   `gocyclo` máximo do wa-noise é muito acima do baseline do repo).
4. `COVER_PKGS`/`TEST_PKGS`: só quando houver testes reais, por
   subdiretório, à medida que as Fases B/C do ADR-0004 forem cobrindo.

**Status**: **CORRIGIDO PARCIALMENTE** (lote H, 2026-08-07) — e a premissa
registrada aqui estava **errada**.

A entrada supunha que incluir o módulo quebraria o gate de lint, porque "o
`gocyclo` máximo do wa-noise é muito acima do baseline do repo". Medido:
a maior função de `internal/wa-noise/` tem complexidade **46**, contra os **56**
do baseline. O gate aguenta sem afrouxar nada. E `go vet ./internal/wa-noise/...`
já saía limpo (exit 0).

Então:

- **vet**: incluído (`VET_TARGETS` passou a ser `ALL_PKGS`). Custo zero.
- **lint**: incluído (`LINT_TARGETS` deixou de filtrar o módulo). A trava real
  (`max_complexity`) não mudou; a **contagem** subiu de 83 para 284, mas ela é
  informativa por construção — o gate de lint trava na complexidade máxima, não
  no número de issues.
- **cobertura**: continua de fora, agora por um motivo positivo em vez de
  inércia. Incluir o módulo mudaria o denominador e obrigaria a **baixar**
  `min_coverage` — afrouxar a catraca em troca de medir mais pacotes. Só faz
  sentido quando a cobertura do módulo subir por conta própria.

---

## F18 — `processData` corrompe frame cujo payload chega picado em mais de um websocket message

**Data**: 2026-08-06
**Contexto**: Fase A do ADR-0004, metade `internal/waclient/socket/`. Achado ao
escrever o primeiro teste de remontagem de frame do pacote — o bug é do
wa-noise upstream, não introduzido por nós.

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

**Status**: **CORRIGIDO** (lote C, 2026-08-07). `receivedLength` passou a ser
contado depois do descarte do cabeçalho. `TestProcessDataSplitPayload` saiu do
`t.Skip` e passa; foi acrescentado
`TestProcessDataSplitPayloadEmTodosOsPontosDeCorte`, que varre todos os pontos
de corte possíveis em vez de confiar num só. Controle negativo executado:
reintroduzir a ordem antiga faz o teste falhar.

**Correção sugerida (texto original)**: mover a contagem para depois do descarte do cabeçalho —

```go
length := decodeFrameLength(msg)
fs.incomingLength = length
msg = msg[FrameLengthSize:]
fs.receivedLength = len(msg)
```

E, no mesmo commit, remover o `t.Skip` de `TestProcessDataSplitPayload`, que
passa a ser a prova da correção. Vale também mandar o patch para o upstream
(`wa-api/internal/wa-noise`), já que o bug não é nosso.

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
wa-noise upstream, não introduzido por nós.

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
(`wa-api/internal/wa-noise`), já que o bug não é nosso.

**Status**: **CORRIGIDO** (lote A da leva de saneamento, 2026-08-07). Os
quatro cortes passam agora por `trailingValueMAC`/`splitValueMAC` em
`internal/wa-noise/protocol/appstate/mutation_blob.go`, que devolvem
`ErrShortMutationBlob` com o índice da mutação na mensagem.
`generatePatchMAC` mudou de assinatura para `([]byte, error)` (dois chamadores
ajustados). `splitValueMAC` exige `macLength+cbcIVLength`, não só o MAC: quem
chama corta `content[:cbcIVLength]` logo depois, e validar só o MAC apenas
mudaria o panic de lugar. Travado por `TestSplitValueMACExigeIVEMac`,
`TestTrailingValueMACExigeApenasOMac`,
`TestGeneratePatchMACRejeitaBlobCurtoSemPanic` e
`TestUpdateHashRejeitaBlobCurtoSemPanic`.

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
no upstream (`git log`/issues de `wa-api/internal/wa-noise` em torno de
`fakeIndexesToRemove`). Dois desfechos possíveis: (a) a feature nunca foi
ligada e o parâmetro deve ser removido das três assinaturas, simplificando;
(b) deveria estar populado, e aí é bug de verdade no upstream. Não dá para
escolher sem a intenção.

**Status**: **CORRIGIDO** (lote F, 2026-08-07) pela saída (a). A escolha entre
(a) e (b) dependia de descobrir a intenção no upstream — e essa dependência
deixou de existir: o código é nosso, então a decisão é nossa. Um parâmetro que
nenhum chamador popula é pior que a ausência da funcionalidade, porque sugere um
comportamento que não existe.

O parâmetro saiu das três assinaturas, junto com `indexMACToArray` (que só ele
usava). `TestDecodeMutationsRemovesFakeIndex` e `TestIndexMACToArray` foram
substituídos por `TestDecodeMutationsRemoveRemoveApenasUmMAC`, que trava o
comportamento que de fato existe. Quem precisar da limpeza de MACs legados
reintroduz com um chamador de verdade.

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
deleteIdentityQuery = `DELETE FROM wa-noise_identity_keys WHERE our_jid=$1 AND their_id=$2`
```

**Problema**: `DeleteIdentity` recebe um endereço Signal completo
(`<user>:<device>`), sem sufixo curinga, então na prática o `LIKE` degenera em
igualdade e o efeito observável coincide — há teste provando isso
(`TestDeleteIdentityRemovesOnlyThatAddress` em `store_identity_test.go`, que
confere que `erin:1` sobrevive a `DeleteIdentity("erin:0")`). Duas consequências
mesmo assim:

**Status**: **CORRIGIDO** (lote C, 2026-08-07). `DeleteIdentity` usa
`deleteIdentityQuery` (igualdade). A entrada dizia que o efeito coincidia; isso
vale só enquanto nenhum endereço contiver `_` ou `%`, que são curingas do
`LIKE`. Travado por `TestDeleteIdentityNaoTrataCuringaDeLike`, cujo controle
negativo mostra que com a query antiga `DeleteIdentity("user_1")` apaga também
`userX1` e `userY1`.

Nota sobre a escrita do teste: a primeira versão foi **vacuosa** e passava dos
dois jeitos, porque `IsTrustedIdentity` devolve `true` para endereço
desconhecido — uma linha apagada respondia igual a uma presente. A asserção
correta consulta com uma chave **diferente** da gravada: só uma linha presente
responde `false`.

**Consequências originais**:

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

**Status**: **CORRIGIDO** (2026-08-07) — e a afirmação central desta entrada
estava **errada**.

A entrada dizia: _"não há predicado exportado para checar isso sem chamar
`Serialize()`"_. Há dois, no próprio `go.mau.fi/libsignal@v0.2.1`:

- `func (r *Session) IsFresh() bool` — `SessionRecord.go:113`. É exatamente a
  procedência que faltava: `true` para `NewSession`, `false` para
  `NewSessionFromBytes`.
- `func (s *State) HasSenderChain() bool` — `SessionState.go:222`, que checa um
  dos quatro campos nil que fazem `structure()` panicar.

`PutCachedSessions` passou a pular entradas com `IsFresh()`. Não precisa de PR
upstream nem de vendorizar o libsignal.

O que muda de fato: a proteção **já existia**, mas era acidental — `Dirty` só é
setado por `putCachedSession`, que o libsignal chama depois do handshake. Nada
no código declarava essa invariante, e uma mudança no libsignal ou no fluxo de
handshake a quebraria em silêncio. Agora ela é explícita.

`TestFreshSessionIsNotSerializable` continua versionado: o defeito do libsignal
não mudou, só deixou de ser alcançável por nós.
`TestPutCachedSessionsIgnoraRecordFrescoMarcadoComoSujo` força a situação que a
invariante acidental tornava impossível (record fresco marcado como sujo);
controle negativo executado — sem a guarda, entra em panic.

Comparação: o Baileys resolve o mesmo problema com `session.haveOpenSession()`
em `validateSession`. As duas implementações têm o predicado; só a nossa não o
usava.

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

**Status**: **CORRIGIDO** (lote H, 2026-08-07), por um caminho diferente do
sugerido. A entrada propunha subir um Postgres efêmero dentro do `make check`
via testcontainers; isso trocaria a política Docker-free dos testes do
repositório inteiro por causa de três ramos.

`postgres_test.go` cobre os três pontos (mais as migrações do zero) contra um
Postgres **de verdade**, e dá `t.Skip` quando `WA_TEST_POSTGRES_DSN` não está no
ambiente. `make check` local fica exatamente como estava; o ramo passa a ser
executável em CI ou na máquina de quem estiver mexendo nessas queries:

```
WA_TEST_POSTGRES_DSN='postgres://user:pass@host:5432/wa_test?sslmode=disable' \
  go test ./internal/wa-noise/persistence/store/sqlstore/ -run Postgres -v
```

**Os testes foram executados de verdade** contra `postgres:16-alpine` antes de
serem commitados — e isso pegou três erros nos próprios testes que um `t.Skip`
teria escondido para sempre: uma asserção errada sobre `GetManySessions` (ele
pré-popula o mapa com `nil` para cada endereço pedido, por contrato, então a
chave ausente aparece), e dois `CHECK`/FK do schema que os dados de teste
violavam (`length(index_mac) = 32` e a FK para `wanoise_app_state_version`).
Nenhum defeito no código de produção — os três ramos de dialeto se comportam
como o esperado.

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

**Status**: **CORRIGIDO** (lote A, 2026-08-07). A tag do nó passou a usar
assertion com `ok` devolvendo `ErrInvalidNode`; os quatro pontos de JID passam
pelo helper `jidString`, que devolve `ErrInvalidJIDType` nomeando o campo. Os
dois testes que travavam o panic foram reescritos como prova do erro:
`TestReadNodeRejeitaTagNaoStringSemPanic`,
`TestJIDReadersRejeitamUserNaoStringSemPanic` e
`TestJIDStringClassificaTipoErrado`. Também foram cobertos os
`server.(string)` de `readJIDPair`, que a entrada original não listava.

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

**Status**: **CORRIGIDO** (lote A, 2026-08-07). `Unpack` devolve
`io.ErrUnexpectedEOF` com frame vazio. Travado por
`TestUnpackRejeitaFrameVazioSemPanic`, que cobre `nil` e `[]byte{}`.

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
o `ok`, e o resto do wa-noise em geral faz isso — mas a armadilha não está
escrita em lugar nenhum.

**Correção sugerida**: nenhuma no código. É API pública do upstream e mudar o
retorno quebraria silenciosamente qualquer chamador que hoje dependa de
receber o nó de partida. O que faltava era documentação.

**Status**: **CORRIGIDO** (lote F, 2026-08-07). `GetOptionalChildByTag`
devolve o `Node` zero na falha, não o nó alcançado até ali.

O aviso da entrada — "mudar o retorno quebraria silenciosamente qualquer
chamador que hoje dependa de receber o nó de partida" — **estava certo, e
aconteceu**: `ParseBotInfo` e `ParseMetaInfo` faziam
`node.GetChildByTag("bot")` / `("meta")` sobre um nó que **já era** o `<bot>` /
`<meta>`. O lookup sempre errava, e o defeito devolvia o nó de partida — que era
exatamente o que se queria. Com a correção, todos os atributos passaram a sair
vazios, e quatro testes de parse acusaram.

A tolerância às duas formas de chamada (o próprio nó ou o pai que o contém) foi
mantida, agora declarada num helper `nodeOrChild` em vez de herdada de um bug.
Os outros 63 call sites de `GetChildByTag` passaram sem alteração.

`TestGetChildByTagReturnsTheStartNodeWhenMissing` virou
`TestGetChildByTagDevolveNodeZeroQuandoNaoAcha`, mais
`TestGetOptionalChildByTagSinalizaAusenciaPeloOk`.

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

**Status**: **CORRIGIDO** (lote F, 2026-08-07) pela primeira opção: o
`ReplaceAll` saiu do ramo `[]byte`, com comentário no lugar explicando por que
ele era inalcançável. A segunda opção (fazer `printable` aceitar `\n`) mudaria
a saída de log de mensagens de texto multilinha, que hoje saem em hex — mudança
observável sem benefício claro.

`TestNewlinesAreEscapedWhenNotIndenting` continua válido sem alteração: ele já
provava que `[]byte` com quebra de linha vai para hex, que é justamente a razão
de o ramo removido nunca executar.

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

**Status**: **CORRIGIDO** (lote C, 2026-08-07) pela segunda opção sugerida, a
menos invasiva: `GraphQLError` ganhou um método `Is(error) bool` que compara por
`Extensions.ErrorCode`. O tipo continua não comparável e a serialização JSON não
mudou.

O critério é o código, não a igualdade estrutural, e isso é deliberado: duas
ocorrências do mesmo erro chegam com `Message` e `Path` diferentes (carregam
detalhe da requisição), então comparar por eles faria `errors.Is` falhar
justamente no caso que ele existe para atender. Travado por
`TestGraphQLErrorIsComparaPeloCodigo`;
`TestGraphQLErrorsUnwrapExposesEveryError` foi reescrito e agora exige que
`errors.Is` alcance os dois erros da lista.

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

**Status**: **CORRIGIDO** (lote E, 2026-08-07), incluindo os dois adendos
abaixo e um terceiro bug que só apareceu depois do fix.

1. A lista literal virou `sourceFileNames()`: varredura de `*.go` menos
   `internals.go`, `internals_generate.go` e `*_test.go`, ordenada. Não há mais
   lista para esquecer de atualizar.
2. O bloco de import passou por `mergedImports()`, que junta os imports de
   **todos** os arquivos processados (dedup por caminho, alias preservado) em
   vez de copiar só os de `files[0]` — é o que resolve o adendo do `msgattrs`.
   Confirmado: o `internals.go` regenerado traz
   `"wa-api/internal/wa-noise/protocol/msgattrs"` e compila.
3. **Bug novo, exposto pelo fix**: o gerador copiava os nomes de parâmetro
   verbatim nos dois lugares onde eles aparecem. Para um parâmetro chamado `_`
   a assinatura fica válida, mas a chamada vira `int.c.decryptBotMessage(_, ...)`,
   que não compila. Só apareceu porque a varredura passou a alcançar métodos que
   usam `_`. Resolvido por `effectiveParamNames()`, que sintetiza `arg0`,
   `arg1`… posicionalmente — assinatura e chamada leem a mesma lista, então
   concordam por construção.

Resultado: `internals.go` foi de 178 para **206 wrappers**, superset estrito do
commitado (`comm -23` entre os dois conjuntos de nomes é vazio).

**O gate que faltava agora existe**: `internals_sync_test.go` afere a invariante
que o gerador mantém, sem precisar rodá-lo (o gerador escreveria no diretório do
pacote durante o teste). `TestInternalsGeradoCobreTodoMetodoElegivel` reclama de
método sem wrapper; `TestInternalsGeradoNaoTemWrapperOrfao` reclama do inverso.
Verificado com controle negativo: acrescentar um `func (cli *Client)
probeMetodoSemWrapper()` faz o teste falhar nomeando o método; removê-lo faz
voltar a passar.

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
O gerador monta o bloco de import copiando apenas os imports do _primeiro_
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

### Adendo ao F29 — a Fase H moveu o par para `core/` e **não** piorou o bug, 2026-08-07

**Contexto**: Fase H etapa 5 (reorganização estrutural de `internal/wa-noise/`),
que moveu os 114 `.go` da raiz para `core/`. O inventário da etapa 1
(`internal/wa-noise/docs/FASE_H_INVENTORY.md` §3) pediu explicitamente que o
impacto sobre o F29 fosse registrado aqui ao executar a etapa de `core/`. Este
adendo fecha esse pedido — ele estava documentado em `PATCHES.md` (etapa 5,
seção "Impacto no bug F29") mas nunca replicado neste bloco.

**Onde**: `internal/wa-noise/core/internals.go` e
`internal/wa-noise/core/internals_generate.go` (antes na raiz do fork).

**Constatação**: os dois arquivos foram movidos por `git mv` **sem uma única
edição de conteúdo**. O `//go:generate` roda a partir de `core/`, e os 32 nomes
da lista hardcoded (`internals_generate.go:101-110`) continuam resolvendo
relativo ao diretório do gerador. Ou seja: a lista **não** ficou mais quebrada do
que já estava, e não houve dessincronização adicional entre `internals.go` e o
seu gerador.

**Problema**: nenhum novo. O F29 permanece **exatamente** como descrito acima —
`go generate` ainda derrubaria 96 dos 178 wrappers, e o adendo da Fase D sobre o
bloco de import continua valendo.

**Correção sugerida**: inalterada (ver "Correção sugerida (revisada)" acima).

**Status**: **não corrigido**, por decisão explícita da Fase H. Corrigir o F29
é escopo próprio: toca arquivo gerado, tem duas dependências acopladas, e a
regra do projeto proíbe corrigir de graça bug pré-existente fora do escopo da
tarefa. Documentado para quem vier depois em
`internal/wa-noise/docs/ARCHITECTURE.md` §6 e `docs/CONTRIBUTING.md` regra 9.

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

**Status**: **CORRIGIDO** (lote C, 2026-08-07) pelo caminho de menor risco que
a própria entrada indicava: a **interface** passou a exigir `int64`, alinhando-se
ao único candidato real. Mudar `StickerPackItem` para `uint64` mexeria num tipo
consumido fora do pacote e perderia a distinção de "tamanho não informado".

O teste do ramo deixou de usar um tipo falso: `TestGetSize` agora passa um
`types.StickerPackItem` **de produção** e confere que `getSize` devolve o
tamanho em vez de cair no `default` — que era exatamente o defeito.

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

**Status**: **RESOLVIDO junto da F32** (2026-08-07), por desativação. A
comparação de ponteiros inerte e as duas constantes de desktop foram
comentadas em bloco, com o registro do porquê: o ramo nunca dispara em
produção (`BaseClientPayload` sempre preenche `WebInfo`) e o caminho Argo que
indexava aquelas IDs já está desligado no fork (`decodeArgoResult` retorna
`ErrArgoDecodingBroken` de saída). `ConvertQueryID` passou a ser identidade.

O status anterior ("não corrigido, o lote 2 é constantes e testes") ficou
desatualizado quando o achado foi de fato fechado — ver a nota de correção de
registro no índice.

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
(ou de uma versão mais nova do wa-noise upstream) e substituir as duas
constantes. Não há como derivar os valores corretos a partir do que está
vendorizado.

**Status**: **RESOLVIDO por desativação** (2026-08-07). Continuamos sem os
valores corretos — mas deixou de importar, porque o ramo desktop inteiro foi
desativado. Isso fecha F32 e F31 de uma vez.

**Três fatos verificados que sustentam a decisão:**

1. **O ramo nunca dispara com o nosso cliente.** A condição era
   `plataforma == MACOS || GetWebInfo() == nil`, e as duas metades são falsas
   aqui: a comparação de plataforma é entre ponteiros (F31), e
   `store.BaseClientPayload` sempre preenche `WebInfo` —
   `getRegistrationPayload` e `getLoginPayload` clonam dele e nada zera o campo.
2. **O motivo de existir já não vale.** As IDs de desktop existiam para indexar
   o wire type Argo (`argo/name-to-queryids.json` só tem as de desktop; as web
   não estão lá). Mas `decodeArgoResult` começa com
   `if true { return ErrArgoDecodingBroken }` — o caminho Argo inteiro já está
   desligado no fork. Isso foi conferido **antes** de desativar, justamente
   porque seria a forma de a desativação quebrar algo.
3. **O Baileys não tem essa separação.** Um único enum `QueryIds`, zero
   ramificação por plataforma. A outra implementação aberta do protocolo, que o
   `evolution-api` roda em produção, nunca precisou disso.

**A forma:** o bloco foi **comentado, não apagado** (decisão do usuário), em
`queryids.go`, com as duas constantes erradas anotadas individualmente e um
roteiro do que precisa acontecer para reabilitar.

**Testes:** `TestConvertQueryIDDesktopMapeiaTodasAsIDs` e os dois que travavam
as anomalias foram substituídos por
`TestConvertQueryIDEIdentidadeParaQualquerPayload` (prova a identidade para
cinco formatos de payload, inclusive os que antes escolhiam o ramo desktop) e
`TestAnomaliasDeQueryIDDesktopContinuamNoArgo`, que trava as duas anomalias por
**valor literal** — o conhecimento sobrevive à remoção das constantes, e quem
for reabilitar é avisado de que os dados de origem continuam errados.

**Nota de divergência, para quem for reabilitar:** a ID web de `subscribers`
também difere do Baileys — nossa `9800646650009898` contra `9783111038412085`.
Não foi mexido: é caminho vivo e mudar exige evidência, não comparação.

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

**Status**: **CORRIGIDO** (lote B, 2026-08-07) pela primeira opção: o
`close(qrc.stopQRs)` foi movido para **dentro** do `CompareAndSwap`, que é
exatamente a exclusão que faltava. A mudança de ordem observável é segura — o
emissor só lê `stopQRs` no `select` do laço de emissão, e o item final vai para
um canal com buffer próprio.

**Correção sugerida (texto original)**: mover o `close(qrc.stopQRs)` para
**dentro** do `CompareAndSwap` que já existe logo abaixo, ou proteger com um
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

**Status**: **CORRIGIDO** (lote D, 2026-08-07). As duas variáveis e o bloco
`if protoMsg != nil` foram removidos, com comentário no lugar registrando o que
havia ali e por que era inalcançável. Nenhum ramo do `switch` as atribuía; cada
um já decodifica direto para `dec.Message`.

**Status original**: Deleção é segura, mas o lote 4 não tocou em
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

**Status**: **FECHADO COMO "NÃO CORRIGIR", por decisão** (2026-08-07) — não é
mais pendência.

O raciocínio já estava escrito na própria entrada e continua valendo depois da
revisão: são políticas de **protocolos distintos** (trusted contact token ×
contact safety token) que coincidem hoje por coincidência, não por serem a mesma
regra. Extrair um helper comum acopla os dois: no dia em que um dos protocolos
mudar de política, quem editar o helper muda os dois sem perceber — que é
exatamente o risco que a entrada queria evitar, só que na direção oposta.

A duplicação está protegida onde importa: `TestShouldSendTokenPerJIDType` roda a
mesma tabela de 7 tipos de JID nas duas funções, então uma divergência
acidental aparece como falha de teste.

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

**Status**: **CORRIGIDO** (lote I, 2026-08-07). A política escolhida combina as
duas primeiras sugestões da entrada: `counterMap` (em
`internal/wa-noise/capabilities/retry/counters.go`) guarda um `time.Time` junto
do contador e varre entradas paradas há mais de `counterTTL`, com
`counterTTL = 1h` — exatamente o teto de `recreateSessionTimeout` que a entrada
apontava como natural.

A consequência que travava a decisão (um retry legítimo depois do despejo volta
a ser contado como primeiro) é aceita e está documentada no código. Recibos de
retry legítimos chegam em segundos ou minutos, então a janela de uma hora nunca
é alcançada por tráfego normal; e o mesmo já acontece hoje a cada reinício de
processo.

O TTL sozinho não cobre tudo: um par pode inundar com IDs distintos **dentro**
da janela. Daí o teto rígido `counterMaxEntries = 8192`.

**Achado durante a implementação, que virou parte da correção**: a primeira
versão descia só até o teto, e a inserção seguinte estourava de novo e disparava
outra ordenação — ou seja, quem inundasse pagaria `O(n log n)` **por mensagem**,
e a defesa contra abuso viraria o vetor de amplificação. O teste de inundação
levou **74 segundos**. Com a marca d'água `counterEvictTarget` (75% do teto), a
ordenação amortiza em uma a cada ~2048 inserções e o mesmo teste roda em ~1,6 s.
Travado por `TestCounterMapInundacaoNaoEhQuadratica`, que falha se o custo
voltar a explodir.

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

**Status**: **CORRIGIDO** (2026-08-07), por um caminho que torna a pergunta
desnecessária em vez de respondê-la.

A entrada dizia que confirmar a divergência de JID exige tráfego real. Exige
mesmo — mas não é preciso saber a resposta para fechar o buraco. `DeviceCache`
ganhou TTL (`deviceCacheTTL = 24h`) mais varredura periódica: se a chave órfã
existe, ela expira; se não existe, nada muda.

O padrão veio do `evolution-api`, que injeta no Baileys
`new NodeCache({ stdTTL: 300000 })` como `userDevicesCache` — cache de
dispositivos com TTL. Eles também não responderam a pergunta; fizeram ela deixar
de importar.

Escolha do valor documentada no código: 24h é longo de propósito porque
`GetDevices` segura o lock do cache **atravessando** a consulta usync ao
servidor, então expirar agressivamente troca memória por ida à rede com mutex
segurado. A invalidação primária continua sendo push do servidor; o TTL é rede
para notificação perdida e chave órfã.

Travado por `TestDeviceCacheEntradaExpiraNaLeitura`,
`TestDeviceCacheVarreduraRemoveChaveOrfa` e
`TestDeviceCacheVarreNoMaximoUmaVezPorIntervalo`.

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

**Status**: **CORRIGIDO** (lote C, 2026-08-07) pela saída (b): o comentário
passou a descrever o que a função faz. A saída (a) — reintroduzir o filtro —
mudaria **para quem a mensagem é cifrada**, e os chamadores do caminho de envio
já contam com a lista completa; trocar isso sem evidência de que o contrato
documentado é o desejado seria palpite num caminho de criptografia.

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

**Status**: **CORRIGIDO** (lote D, 2026-08-07). A mensagem passou a dizer
"business name", que é o que `PutBusinessName` grava — quem investigasse o log
era mandado para o caminho errado.

**Status original**: É um caractere de risco quase zero, mas altera
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

**Status**: **CORRIGIDO NA RAIZ** (lote C, 2026-08-07). `group.GetOrFetch`
devolve `ErrNotFound` (embrulhado, com o JID consultado na mensagem) em vez de
`(nil, nil)`. As guardas nos chamadores continuam onde estão — são defesa em
profundidade —, mas a raiz deixou de devolver "sucesso sem resultado", que é a
forma que o compilador não ajuda a tratar e que o próximo chamador esqueceria de
novo. `TestGetOrFetchReturnsNilNilOnEchoedDifferentID` foi reescrito como
`TestGetOrFetchDevolveErroQuandoOServidorEcoaOutroID`.

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

**Status**: **CORRIGIDO** (lote E, 2026-08-07), depois de a F29 destravar a
regeneração do `internals.go`. `send.MakeDeviceIdentityNode` devolve
`(waBinary.Node, error)`, e a mudança propagou por `send.MessageContent`,
`retry.Transport.MessageContent`, `core.Client.getMessageContent`,
`core.retryTransport.MessageContent` e o call site em `retry/handle.go` — todos
já devolviam erro ou passaram a devolver.

O **caso silencioso citado na entrada foi corrigido junto**, e era o pior dos
dois: com `Store.Account` nil, `proto.Marshal` devolve bytes vazios _sem erro_ e
o `<device-identity>` ia vazio para o fio. Agora sai `ErrNoDeviceIdentity`.
Travado por `TestMakeDeviceIdentityNodeSemContaDevolveErro` e
`TestMessageContentSemContaDevolveErro`.

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

**Status**: **RESOLVIDO por documentacao** (2026-08-07), depois de a medicao mostrar que a pergunta original era a errada.

Nao se uniformizou o `v`, e a razao nao e' mais "falta captura de trafego": o ramo v3/FB de ENVIO e' **inalcancavel** a partir do wa-api (ver a secao de evidencias no fim deste arquivo). Sem o ramo rodar, nenhum teste de integracao poderia confirmar que trocar para string e' inocuo — mexer as cegas em codigo morto seria pior que anotar.

`fb_encrypt.go` traz agora a constatacao completa e um **criterio de reavaliacao** explicito: no dia em que alguma rota expuser `SendFBMessage`, a forma do `v` deixa de ser academica e precisa ser confirmada contra o fio antes de ir a producao.
levantada em 2026-08-07, para a decisão não recomeçar do zero.

**O que o Baileys faz** (`src/Socket/messages-send.ts`): usa `v: '2'` como
**string**, uniformemente, nos três pontos em que monta um `<enc>`:

```typescript
attrs: { v: '2', type, ...(extraAttrs || {}) }              // mensagem normal
attrs: { v: '2', type: 'skmsg', ...extraAttrs }             // sender key de grupo
attrs: { v: '2', type, count: participant!.count.toString() } // reenvio de retry
```

Nenhuma variante numérica em lugar nenhum. E o Baileys é a implementação que o
`evolution-api` roda em produção em escala, então a forma string é
comprovadamente aceita pelo servidor.

**Ressalva que impede fechar direto**: isso cobre o caminho waE2E. Não foi
encontrada implementação do caminho v3/FB (Armadillo) no Baileys — que é
justamente onde o nosso `v` é numérico. A evidência diz "no caminho que as duas
implementações têm, string é a forma correta", **não** "o caminho v3/FB deveria
ser string".

**O que decide**: saber se o caminho v3/FB é exercitado na prática. Se não for,
uniformizar é barato e sem risco; se for, precisa de captura antes.

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

**Status**: **CORRIGIDO** (lote F, 2026-08-07). A taxonomia passou a ter um
dono: `msgattrs` exporta `TypeText`, `TypePoll`, `TypeMedia` e `TypeReaction`, e
tanto `GetTypeFromMessage` quanto os consumidores usam as mesmas constantes —
`send/constants.go` virou alias delas, e `retry/handle.go` deixou de escrever o
literal `"text"`.

Não confundidas com as outras ocorrências de `"media"`/`"text"`/`"reaction"` no
módulo: o atributo `media` de presença de chat, o `media` de chamada e a tag
`<reaction>` de newsletter são taxonomias diferentes que por acaso usam as
mesmas palavras, e ficaram intocadas.

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
travamento raro que evitam. O caminho provável é iniciar o loop _antes_ do envio
e dar timeout ao download, mas isso é decisão de projeto sobre a política de
history sync, não patch pontual.

**Status**: **PARCIALMENTE corrigido; o resto ABERTO por decisao** — agora com medicao de carga real.

### Medicao (2026-08-07, 4 sessoes pareadas por interface, 40.786 mensagens)

| Fato | Valor |
| --- | --- |
| Lotes de history sync processados | 10 |
| Falhas de download | **0** |
| Sinais de fila cheia ou produtor bloqueado | **0** |
| Loop religado apos parada | **0** |

O ultimo e' o mais informativo. Existe um `Warnf("New history sync
notifications appeared after loop stopped, restarting loop...")`
(`history_sync.go:64`) exatamente para o caso em que a fila recebe algo
enquanto o consumidor esta' parado. Ele **nao disparou nenhuma vez** no uso
mais pesado que este projeto viu: o pareamento inicial de quatro contas.

O cenario que esta entrada teme — buffer cheio com consumidor pendurado num
Download — nao se materializou sob essa carga.

### Um numero que NAO significa o que parece

Os intervalos entre lotes chegaram a **127 segundos**. E' tentador ler isso
como "o consumidor ficou dois minutos ocupado", e seria errado:
`historySyncLoopIdleTimeout` e' 1 minuto, e o `case <-time.After(...)` faz o
loop SAIR quando fica ocioso. Um intervalo de 127s e' igualmente compativel
com "ocioso, saiu, foi religado pela notificacao seguinte" — o caminho
saudavel.

Os timestamps dos lotes nao separam "processando" de "ocioso". Quem for
retomar esta entrada nao deve usar esse numero como evidencia de lentidao.

### O que falta para decidir sobre o timeout

A pergunta da entrada — vale dar timeout ao download? — depende de quanto
tempo um download demora, e **nenhuma linha de log cronometra
`DownloadHistorySync`**. Sem isso, qualquer valor de timeout seria escolhido
no escuro.

Duas linhas de instrumentacao (inicio e fim do download, com duracao)
resolveriam, ao custo de mexer no caminho de recepcao do modulo de protocolo.

A alternativa e' fechar esta entrada registrando que o risco nao se
manifestou em carga real — que ja' e' insumo melhor do que ela tinha quando
foi aberta, quando o risco era inteiramente teorico.

**Feito** (2026-08-07): `EnqueueHistorySync` passou a ligar o consumidor
**antes** de enfileirar. Era o contrário — enfileirava e só então ligava o loop,
ou seja, o primeiro envio acontecia sem consumidor nenhum existir. Verificado
antes de inverter que não há lost wakeup: o loop bloqueia no `select` esperando
o canal e não checa `Len()` na entrada, e o `defer` dele já religa se algo
entrar entre a saída do select e o `Store(false)`.

**O que sobra**: o consumidor pendurado num `Download` HTTP com o buffer cheio.
Isso ainda bloqueia o goroutine que trata o stanza, e depende de dar timeout ao
download.

**O que o Baileys faz** — e é um contraste arquitetural, não um detalhe:
ele **não tem fila**. `processMessage` trata
`HISTORY_SYNC_NOTIFICATION` chamando `downloadAndProcessHistorySyncNotification`
**inline**, gated por um booleano de config (`shouldProcessHistoryMsg`). Sem
canal, sem goroutine de fundo, sem produtor/consumidor. O `evolution-api` só
acrescenta um callback (`shouldSyncHistoryMessage`) que decide se processa.

Ou seja: o risco descrito aqui é criado pela arquitetura do fork (canal com
buffer 32 + goroutine consumidora), não é inerente ao protocolo.

**As duas saídas, com o custo de cada uma**:

| Saída                                | Trade-off                                                                                                                                                                              |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Timeout no download, mantendo a fila | Menor mudança, preserva o desacoplamento. Falta escolher o valor — e instrumentar ocupação do buffer diria qual                                                                        |
| Adotar o modelo do Baileys (inline)  | Elimina a classe inteira do bug, mas o download HTTP passa a rodar no caminho de processamento de stanza: troca "goroutine bloqueada raramente" por "processamento serializado sempre" |

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

**Status**: **CORRIGIDO** (lote C, 2026-08-07). `if len(lidPairs) == 0 { return }`
antes da gravação, como `storeHistoricalMessageSecrets` já fazia.

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

**Status**: **CORRIGIDO** (lote B, 2026-08-07). O bloqueio registrado aqui
deixou de valer: não há mais upstream contra o qual manter compatibilidade, e
`grep -rn "AutoReconnectErrors\|LastSuccessfulConnect"` mostra que **nenhum**
consumidor fora de `internal/wa-noise/core/` lia os dois campos — o módulo está
sob `internal/`, então "API pública" aqui nunca passou dos limites deste repo.

Os campos viraram `lastSuccessfulConnectUnixNano atomic.Int64` e
`autoReconnectErrors atomic.Int64`, privados, com os leitores
`Client.LastSuccessfulConnect() time.Time` e `Client.AutoReconnectErrors() int`
em `client_reconnect_state.go` — o `AutoReconnectHook` continua conseguindo ler
a contagem, agora sem corrida. Travado por
`TestContadoresDeReconexaoSuportamAcessoConcorrente`,
`TestLastSuccessfulConnectZeroQuandoNuncaConectou` e
`TestLastSuccessfulConnectRoundTrip`.

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

**Status**: **CORRIGIDO** (lote C, 2026-08-07). O erro de `io.Copy` é checado
antes de preencher `resp`. Travado por
`TestUploadNewsletterReaderPropagaErroDeLeitura`, que usa exatamente a
reprodução descrita acima (reader que devolve `n>0` e depois falha, com `Seek`
funcional) e confere as três coisas: o erro sai embrulhado, `resp` fica no zero,
e **nenhum upload acontece**. Controle negativo executado.

**Status original**: fora do escopo do lote 1 da Fase F/G, que é
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
- **Status**: **RESOLVIDO** (2026-08-07) — o ramo inteiro foi desativado, o que
  torna a comparação de ponteiros irrelevante. Ver F32 abaixo.

  **Status original**: fora do escopo de uma extração; o teste
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
- **Status**: **CORRIGIDO** (lote C, 2026-08-07) pela primeira opção.
  `EncodePatch` devolve `ErrNilMutationValue` nomeando qual mutação veio sem
  valor. Travado por `TestEncodePatchRejeitaMutacaoSemValor`.
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
- **Status**: **CORRIGIDO** (lote B, 2026-08-07). `State.linking` é
  `atomic.Pointer[LinkingCache]`; `Linking()` virou `Load()` e `SetLinking()`
  virou `Store()`. Atômico e não mutex porque a semântica que o código quer é
  exatamente "último escritor ganha", sem seção crítica em volta. Travado por
  `TestStateLinkingSuportaLeituraEEscritaConcorrentes` (que falha sob `-race`
  na versão antiga) e `TestStateZeroValueTemLinkingNil`.
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
- **Status**: **CORRIGIDO** (lote A, 2026-08-07). `Upload` sai cedo com
  `Warnf` quando `GetOrGenPreKeys` devolve fatia vazia, antes do `Infof` e do
  IQ — o que também evita mandar um `<list>` vazio ao servidor. Travado por
  `TestUploadComListaVaziaNaoEntraEmPanicNemEnviaIQ`.

---

## F52 — o throttle de expurgo do store de retry é código morto: `lastRetryStoreClear` nunca é escrito

**Data / contexto**: 2026-08-07, Fase F/G lote 5 do ADR-0004
(notificação/retry/recibo). Achado ao mover o campo para `retry.State`, quando
o compilador não acusou nenhum ponto de escrita.

**Onde**: antes da extração, `internal/wa-noise/client.go:143` (declaração do
campo) e `internal/wa-noise/retry_recent_messages.go:59` (a única leitura).
Hoje, `internal/wa-noise/retry/state.go` (campo `lastStoreClear`) e
`internal/wa-noise/retry/recent.go` (a leitura, dentro de `AddRecent`).

```go
// retry_recent_messages.go:59, no HEAD anterior ao lote
if time.Since(cli.lastRetryStoreClear) > retryStoreClearInterval {
    err = cli.Store.EventBuffer.DeleteOldOutgoingEvents(ctx)
    ...
}
```

**Problema**: `git grep -n "lastRetryStoreClear" internal/wa-noise/` no HEAD
anterior devolve exatamente duas linhas — a declaração e a leitura acima.
**Nenhuma atribuição, em lugar nenhum.** O campo fica permanentemente no zero
de `time.Time`, `time.Since(zero)` é da ordem de dois mil anos, e a comparação
contra `retryStoreClearInterval` (12h) é sempre verdadeira. Consequência
observada: `DeleteOldOutgoingEvents` roda a **cada** mensagem enviada com
`UseRetryMessageStore` ligado, e não uma vez a cada 12 horas.

Evidência empírica produzida nesta sessão:
`TestAddRecentWithStore` (`internal/wa-noise/retry/recent_test.go`) afirma
`deleteOldCalls == 1` depois da primeira gravação e `== 2` depois da segunda —
duas gravações consecutivas, dois expurgos, sem qualquer espera.

Não é corrupção de dados nem pânico: é um DELETE indo ao banco por mensagem
enviada, num caminho quente (envio), com o custo escondido atrás de uma
condição que aparenta ser um throttle.

**Correção sugerida**: escrever o carimbo logo depois do expurgo bem-sucedido,
dentro do mesmo `if`:

```go
if err = t.Store().EventBuffer.DeleteOldOutgoingEvents(ctx); err != nil {
    return fmt.Errorf(...)
}
t.State().SetLastStoreClear(time.Now())
```

O campo precisaria ganhar um setter e, muito provavelmente, um lock próprio —
a leitura de hoje já é feita sem sincronização nenhuma, e `AddRecent` é
chamada de qualquer goroutine que envie mensagem, então o campo é uma corrida
de dados latente além de um throttle morto. (Hoje a corrida é benigna na
prática porque nada escreve; corrigir o throttle a torna real.)

**Status**: **CORRIGIDO** (lote C, 2026-08-07). `AddRecent` carimba
`State.MarkStoreCleared` depois de cada `DeleteOldOutgoingEvents` bem sucedido —
o throttle de `StoreClearInterval` passou a valer, em vez de o expurgo rodar em
toda gravação de mensagem enviada.

O campo virou `atomic.Int64` no mesmo movimento: ele não tinha sincronização
nenhuma, o que era inofensivo enquanto ninguém escrevia, mas `AddRecent` roda no
caminho de envio, que é concorrente — ligar a escrita criaria a corrida.
`TestAddRecentWithStore` foi reescrito e cobre os três estados: primeiro
expurgo (carimbo zero), segundo segurado pelo throttle, e terceiro liberado
depois que o intervalo passa.

**Status original**: o lote 5 é extração, e ligar o throttle muda o
comportamento observável contra o banco (de "expurga sempre" para "expurga a
cada 12h"), além de exigir uma decisão sobre sincronização. Preservado bit a
bit, com o comportamento travado por teste para que a correção futura seja
consciente. Pendente de decisão.

---

## Nota sobre F36 após a Fase F/G lote 5

F36 (os dois contadores de retry que crescem sem limite) continua **em
aberto**. O lote 5 **moveu** os dois mapas de `*Client` para
`internal/wa-noise/retry.State` — `incomingRetryRequestCounter` virou
`State.incomingCounter` e `messageRetries` virou `State.messageRetries` — e
**não** implementou política de despejo nenhuma.

A movimentação não resolve nem atenua o problema: as chaves continuam vindo do
servidor, nada esvazia os mapas, e não há reset no `Disconnect`. O que mudou é
onde a dívida está documentada — o doc do tipo `retry.State`
(`internal/wa-noise/retry/state.go`) traz o aviso por extenso, e os dois campos
carregam a referência a F36 individualmente.

Os endereços de código citados no corpo de F36 acima (`retry.go:110-113`,
`retry_receipt_send.go:29-37`, `client.go:233,239`) referem-se ao HEAD anterior
ao lote 5 e não existem mais nessa forma; os pontos equivalentes hoje são
`retry/handle.go` (chamada a `IncrementIncoming`), `retry/send.go` (chamada a
`BumpMessageRetries`) e `retry/state.go` (os dois mapas e suas seções críticas).

---

## F53 — `*groupMetaCache` escapa do lock e é lido sem sincronização pelo caminho de envio

**Data / contexto**: 2026-08-07, durante a Fase F/G lote 6 (extração de
`internal/wa-noise/group/`). Achado pela revisão de concorrência independente do
lote, ao auditar o cache de grupo. **Não faz parte do escopo do lote** — é
pré-existente, herdado do upstream, e idêntico no HEAD anterior à extração.

**Onde** (endereços de hoje, pós-lote 6):

- `internal/wa-noise/group/info.go:200-212` — `GetOrFetch` toma o lock do cache,
  obtém o `*Meta` da entrada **viva** do mapa e o devolve ao chamador; o
  `defer cache.Unlock()` roda no return, ou seja, o ponteiro sai da seção
  crítica.
- `internal/wa-noise/send_prepare.go:176-195` — lê `cachedData.AddressingMode`,
  `cachedData.CommunityAnnouncementGroup` e devolve `cachedData.Members`, tudo
  **sem lock**.
- `internal/wa-noise/sendfb_transport.go:41-101` — mesma coisa com
  `groupMeta.Members`, passado adiante a `prepareMessageNodeV3`.
- `internal/wa-noise/group/notification.go:198-224` — `UpdateParticipantCache`
  muta `cached.Members` **no lugar** (append nas entradas, swap-remove nas
  saídas), sob o lock.

**Problema**: o ponteiro `*Meta` é compartilhado entre o caminho de envio (que
lê sem lock, depois que `GetOrFetch` retornou) e o handler de notificação `w:gp2`
(que muta `Members` sob o lock). O lock não protege nada nesse par, porque os
dois lados não o seguram ao mesmo tempo: um envio para um grupo concorrente a
uma notificação de entrada/saída de participante pode ler um slice header
rasgado, ou uma lista de membros parcialmente atualizada. É uma corrida de dados
no sentido do modelo de memória de Go, não só uma inconsistência lógica.

Evidência de que é pré-existente, e não introduzido pela extração:
`git show HEAD:internal/wa-noise/group.go` (linhas 185-196) mostra o
`getCachedGroupData` original devolvendo o mesmo `*groupMetaCache` vivo, de
dentro do mesmo `defer cli.groupCacheLock.Unlock()`, para os mesmos dois
chamadores. A extração preservou a estrutura bit a bit.

Por que a suíte com `-race` não pega: os testes atuais não dirigem
`GetOrFetch` e `UpdateParticipantCache` concorrentemente contra o mesmo JID de
grupo. O detector só reporta o que o teste executa.

**Correção sugerida** (duas opções, em ordem de preferência):

1. `GetOrFetch` devolve uma **cópia** do `Meta`, com `Members` copiado
   (`slices.Clone`), em vez do ponteiro vivo. É o menor diff e elimina a corrida
   na origem. Custo: uma alocação por envio de grupo com cache quente; o slice
   de membros de um grupo grande pode ter milhares de entradas, então convém
   medir antes.
2. Manter o ponteiro e dar a `Meta` o seu próprio `sync.RWMutex`, com os
   chamadores do caminho de envio tomando o lock de leitura. Mais invasivo e
   espalha sincronização por três arquivos da raiz.

A opção 1 muda a semântica observável de `DangerousInternalClient.GetCachedGroupData`
(deixaria de devolver o ponteiro vivo), o que é aceitável dado o nome do método,
mas precisa de decisão.

**Status**: **CORRIGIDO** (lote G, 2026-08-07) pela opção 1, depois de **fazer
a medição que a entrada pedia**.

`internal/wa-noise/capabilities/group/cache_bench_test.go` compara devolver o
ponteiro vivo com devolver uma cópia, em tamanhos de 2 a 4096 membros. No teto
real do WhatsApp (1024 membros): **~5,5 µs e UMA alocação de 41 KB** por envio
de grupo com cache quente. O mesmo envio faz criptografia Signal por dispositivo
para os mesmos 1024 membros, o que custa milissegundos — a cópia é ruído perto
disso, e o benchmark fica versionado para quem quiser reavaliar.

`Meta.Clone()` copia `Members` com `slices.Clone`, e `GetOrFetch` devolve o
clone nos dois caminhos (cache quente e pós-consulta). `GetLocked` continua
devolvendo o ponteiro vivo para uso interno sob o lock, que é o que
`UpdateParticipantCache` precisa.

Consequência aceita e documentada: `DangerousInternalClient.GetCachedGroupData`
deixa de devolver o ponteiro vivo. Travado por
`TestGetOrFetchDevolveCopiaIndependente`, que muta os dois lados e prova que
nenhum alcança o outro.

## F54 — `DangerousInternalClient.GetFBIDDevices` escreve no cache de dispositivos sem tomar o lock

**Data / contexto**: 2026-08-07, durante a Fase F/G lote 7 (extração de
`internal/wa-noise/user/`). Achado pela revisão independente de concorrência do
lote, e confirmado por leitura direta do HEAD pré-refactor.

**Onde**:

- Antes: `internal/wa-noise/user_devices.go:149-167` (`getFBIDDevices`), que faz
  `cli.userDevicesCache[jid] = userDevices` na linha 162 **sem** tomar
  `userDevicesCacheLock`.
- Depois: `internal/wa-noise/user/devices.go:176` (`user.GetFBIDDevices`, via
  `cache.SetLocked`), alcançado pela fachada `cli.getFBIDDevices`
  (`internal/wa-noise/user.go:126`).
- Exposição pública: `internal/wa-noise/internals.go:739`,
  `DangerousInternalClient.GetFBIDDevices`.

**Problema**: `getFBIDDevices` é seguro pelo seu único chamador de produção,
`GetUserDevices`/`user.GetDevices`, que segura o lock do cache durante toda a
chamada — é por isso que ele não toma o lock por conta própria (um segundo
`Lock()` num `sync.Mutex` não reentrante seria deadlock imediato). Mas
`internals.go` o expõe **diretamente** em `DangerousInternalClient`, e por esse
caminho a escrita no mapa acontece sem nenhuma sincronização. Um chamador externo
que use a fachada `Dangerous` concorrentemente com o caminho de envio ou com
`handleDeviceNotification` tem uma corrida real de escrita em mapa (que em Go
pode virar `fatal error: concurrent map writes`, não recuperável).

**Pré-existente, não introduzido pelo lote 7**: o `getFBIDDevices` original
escrevia no mapa igualmente sem lock, e `internals.go` já o expunha. O lote 7
mudou uma coisa marginal: como o mapa passou a ser criado preguiçosamente em
`SetLocked` (antes `NewClient` o criava), a janela de corrida agora inclui também
a **criação** do mapa, não só a escrita de uma chave. Não é uma classe nova de
bug — o caminho já era racy — mas a janela é um pouco maior.

**Correção sugerida**: duas opções, nenhuma aplicada aqui por ser fora do escopo
do lote:

1. Fazer `DangerousInternalClient.GetFBIDDevices` tomar o lock antes de delegar
   (`int.c.userDevicesCache.Lock(); defer ...Unlock()`), deixando o contrato
   "chamado com o lock segurado" explícito na função de domínio. É a correção
   mínima e não toca no caminho de produção.
2. Renomear a função de domínio para `GetFBIDDevicesLocked`, tornando o contrato
   impossível de ignorar por leitura. Exige regenerar `internals.go` (F29).

**Status**: **CORRIGIDO** (lote E, 2026-08-07). A trava foi para a **fachada**
`core.Client.getFBIDDevices`, não para `user.GetFBIDDevices`.

A assimetria é o ponto: `user.GetFBIDDevices` não pode travar por conta própria
(seu chamador de produção, `user.GetDevices`, já segura o lock, e `sync.Mutex`
não é reentrante), mas a fachada **não está no caminho de produção** —
`grep -rn "getFBIDDevices\\b"` devolve exatamente a definição e a chamada em
`internals.go`, nada mais. Então travar lá fecha o único caminho sem
sincronização (o `DangerousInternalClient`) sem nenhum risco de deadlock.

**Status original**: `internals.go`/`internals_generate.go` estão fora do
escopo por designação (F29), e a regra do projeto proíbe corrigir de graça bug
pré-existente fora do escopo da tarefa. Registrado para decisão do usuário.

---

## F55 — `SetMediaHTTPClient(nil)` (e irmaos) transforma qualquer chamada de proxy posterior em panic

**Data / contexto**: 2026-08-07, durante a Fase F/G lote 10 (extracao de
`client_proxy.go` para `internal/wa-noise/proxyconf/`). Achado ao decidir se a
funcao extraida deveria ganhar guarda de nil.

**Onde**: `internal/wa-noise/client_proxy.go:96-110` (os tres setters) e
`internal/wa-noise/proxyconf/proxyconf.go:150-160` (`Apply`, que e' o antigo
`setTransport`).

```go
// client_proxy.go
func (cli *Client) SetMediaHTTPClient(h *http.Client) { cli.mediaHTTP = h }

// proxyconf.go — Apply
if !opt.NoMedia {
    c.Media.Transport = transport   // nil deref se Media for nil
}
```

**Problema**: os tres setters aceitam qualquer `*http.Client`, inclusive `nil`,
e nao validam. `NewClient` sempre preenche os tres campos, entao o estado
inicial e' seguro; mas depois de `cli.SetMediaHTTPClient(nil)` qualquer
`SetProxy` / `SetSOCKSProxy` / `SetProxyAddress` subsequente desreferencia nil e
derruba o processo. Nao ha' nada na documentacao dos setters que diga que nil e'
proibido, e passar nil e' a forma intuitiva de "voltar ao padrao".

Reproducao (nao adicionada a suite, por ser fora do escopo):

```go
cli := wa-noise.NewClient(store, nil)
cli.SetMediaHTTPClient(nil)
cli.SetProxy(nil)  // panic: runtime error: invalid memory address
```

O mesmo vale para `SetWebsocketHTTPClient(nil)` + `SetProxy` e para
`SetPreLoginHTTPClient(nil)` + `SetProxy`.

**Pre-existente, nao introduzido pelo lote 10**: o `setTransport` original
(`client_proxy.go:125-135` antes da extracao) escrevia nos tres campos
exatamente do mesmo jeito, sem guarda. `proxyconf.Apply` foi mantido **literal**
de proposito, com o defeito anotado no proprio comentario da funcao, para que a
extracao nao mudasse comportamento.

**Correcao sugerida**: duas opcoes, nenhuma aplicada:

1. Guarda de nil em `proxyconf.Apply` (`if c.Media != nil { ... }` nos tres). E'
   a correcao minima, mas silencia o erro: quem passou nil por engano fica sem
   proxy e sem aviso.
2. Fazer os tres setters da raiz tratarem nil como "volte ao cliente padrao",
   substituindo por um `&http.Client{Transport: (http.DefaultTransport.(*http.Transport)).Clone()}`.
   E' o que a intencao do chamador provavelmente e', e mantem o invariante "os
   tres campos nunca sao nil", que o resto do codigo (`unlockedConnect`, em
   `client_connection.go:118-121`) ja' assume.

A opcao 2 e' a preferida: `unlockedConnect` le `cli.websocketHTTP` /
`cli.preLoginHTTP` e passa direto para `socket.NewFrameSocket`, entao um nil ali
tambem quebra a conexao, nao so' o proxy.

**Status**: **CORRIGIDO** (lote A, 2026-08-07) pela opcao 2, a preferida da
entrada. Os tres setters passam por `orDefaultHTTPClient`, que traduz `nil`
para um `http.Client` padrao novo — mantendo o invariante "os tres campos nunca
sao nil" que `proxyconf.Apply` e `unlockedConnect` assumem. Travado por
`TestSettersDeHTTPClientTraduzemNilParaPadrao` (que termina chamando
`SetProxyAddress`, o nil deref original) e
`TestSettersDeHTTPClientPreservamOPonteiroRecebido`.

---

## F56 — `fs.OnDisconnect` é escrito sem sincronização enquanto o read pump já pode lê-lo

**Data / contexto**: 2026-08-07, Fase F/G lote 10. Achado pela revisão de
concorrência independente da extração do handshake (item C1 do relatório), ao
mapear os goroutines que `nh.Finish` dispara.

**Onde**: `internal/wa-noise/socket/noisesocket.go:48-50` (escrita) e
`internal/wa-noise/socket/framesocket.go:85-87` (leitura), com o goroutine leitor
iniciado em `internal/wa-noise/socket/framesocket.go:112`.

```go
// noisesocket.go:48 — escrita, sem lock
fs.OnDisconnect = func(ctx context.Context, remote bool) {
    disconnectHandler(ctx, ns, remote)
}

// framesocket.go:85 — leitura, de dentro de Close(), que roda no goroutine do read pump
if fs.OnDisconnect != nil {
    go fs.OnDisconnect(fs.parentCtx, code == statusForceClose)
}
```

**Problema**: `fs.Connect` (`framesocket.go:112`) já disparou
`go fs.readPump(...)` **antes** de `newNoiseSocket` escrever `fs.OnDisconnect`.
O read pump chama `fs.Close(statusForceClose)` no seu `defer`
(`framesocket.go:203`), e `Close` lê `fs.OnDisconnect`. Escrita e leitura de um
campo de struct, em goroutines diferentes, sem mutex nem atomic — é data race
pelo modelo de memória do Go.

`Close` toma `fs.lock` (`framesocket.go:63`), mas a **escrita** em
`noisesocket.go:48` não toma lock nenhum, então o mutex não sincroniza o par.

Consequência prática: se o websocket cair durante a janela entre
`fs.Connect` e `newNoiseSocket` — janela que inclui o handshake Noise inteiro,
ou seja, um round trip de rede —, o `OnDisconnect` pode ser lido como nil e a
desconexão passa despercebida, ou o `-race` acusa em produção instrumentada.

**Status**: **CORRIGIDO** (lote B, 2026-08-07). O campo virou privado
(`onDisconnect`) e só se mexe nele por `FrameSocket.SetOnDisconnect`, que toma o
mesmo `fs.lock` que `Close` já segura — é isso que dá o happens-before que
faltava. Os dois pontos de escrita (`newNoiseSocket` e `NoiseSocket.Stop`, este
último zerando o callback) passaram a usar o setter. Travado por
`TestSetOnDisconnectEConcorrenteComClose`.

**Pré-existente, não introduzido pelo lote 10**: `git status` mostra que nada sob
`internal/wa-noise/socket/` foi tocado por este lote. A ordem
`Connect` → `readPump` → `Finish` → escrita de `OnDisconnect` é a do upstream.

**Correção sugerida**: mover a atribuição de `fs.OnDisconnect` para dentro de
`fs.lock`, expondo um setter em `FrameSocket`
(`func (fs *FrameSocket) SetOnDisconnect(f func(context.Context, bool))` que
tome `fs.lock`), e fazer `Close` ler o campo ainda sob o lock que ele já segura.
É correção local ao pacote `socket/`, sem efeito na API do fork.

**Status**: não corrigido. Fora do escopo do lote 10 (que não tocou `socket/`), e
a regra do projeto proíbe corrigir de graça bug pré-existente fora do escopo.
Registrado para decisão do usuário.

---

## F57 — `SetProxy*` concorrente com `Connect()` é corrida sobre o `http.Client` da conexão

**Data / contexto**: 2026-08-07, Fase F/G lote 10. Achado pela revisão de
concorrência independente (item C2 do relatório).

**Onde**: escrita em `internal/wa-noise/proxyconf/proxyconf.go:150-160`
(`Apply`, chamado por `Client.setTransport` em
`internal/wa-noise/client_proxy.go:84-90`); leitura em
`internal/wa-noise/client_connection.go:118-121`.

```go
// client_connection.go:118-121 — leitura, sob socketLock
client := cli.websocketHTTP
if cli.Store.ID == nil {
    client = cli.preLoginHTTP
}
fs := socket.NewFrameSocket(cli.Log.Sub("Socket"), client)
```

**Problema**: `SetProxy` / `SetSOCKSProxy` / `SetProxyAddress` escrevem
`preLoginHTTP.Transport`, `websocketHTTP.Transport` e `mediaHTTP.Transport`
**sem tomar lock nenhum**. `unlockedConnect` lê os mesmos ponteiros sob
`socketLock`. Como a escrita não toma o lock, o `socketLock` não sincroniza o
par: chamar `SetProxy` de um goroutine enquanto outro chama `Connect()` é data
race, e o resultado pode ser uma conexão que sai pelo transport antigo — ou
seja, **sem o proxy que acabou de ser pedido**, o que vaza o endereço real do
cliente.

A documentação de `SetProxy` (`client_proxy.go:51-54`) diz _"Must be called
before Connect() to take effect in the websocket connection"_, o que cobre o
caso sequencial, mas não diz nada sobre concorrência.

**Pré-existente, não introduzido pelo lote 10**: o `setTransport` original
(antes da extração) escrevia nos mesmos três campos, do mesmo goroutine do
chamador, sem lock. A extração para `proxyconf.Apply` é literal.

**Correção sugerida**: fazer `Client.setTransport` tomar `cli.socketLock.Lock()`
antes de delegar para `proxyconf.Apply`. Os três `*http.Client` já são lidos sob
esse lock em `unlockedConnect`, então usar o mesmo mutex fecha o par sem
introduzir um segundo. Como `setTransport` não faz I/O, a seção crítica é de
três atribuições — não há risco de segurar o lock de escrita por muito tempo.
Alternativa mais barata: documentar explicitamente que os setters de proxy só
podem ser chamados antes do primeiro `Connect()`.

**Status**: **CORRIGIDO** (lote G, 2026-08-07) pela correção sugerida, não pela
alternativa barata: documentar "só chame antes do Connect" não fecha a corrida,
só a transfere para o usuário da biblioteca.

`setTransport` toma `socketLock` em escrita — o mesmo mutex sob o qual
`unlockedConnect` lê `websocketHTTP`/`preLoginHTTP`. A seção crítica são três
atribuições, sem I/O.

Estendido além do que a entrada pedia: os **três setters de `*http.Client`**
(`SetMediaHTTPClient` e irmãos) também tomam o lock. Eles trocam o **ponteiro**,
e é o ponteiro que `unlockedConnect` lê — a corrida era a mesma, só que numa
linha diferente.

Verificado que nenhum caminho interno chama esses métodos com `socketLock` já
segurado: os únicos chamadores são os setters públicos e o wrapper gerado em
`internals.go`. Travado por
`TestSettersDeProxySaoConcorrentesComLeituraSobSocketLock`.

## F58 — `messageSendLock` é declarado em `core` e emprestado por ponteiro para `capabilities/send`: única violação de "estado e lock viajam juntos" no fork

**Data**: 2026-08-07. **Contexto**: Fase H etapa 7 (final), ao escrever
`internal/wa-noise/docs/LOCKS.md` — o documento que torna verificável a regra
"estado compartilhado e o mutex que o protege moram no mesmo pacote, na mesma
struct". Ao inventariar os locks por inspeção do código, este é o único que não
satisfaz a regra. O achado não é novo: o inventário da etapa 1
(`internal/wa-noise/docs/FASE_H_INVENTORY.md`, seção "O único par estado/lock já
separado: `messageSendLock`") já o havia identificado e pedido explicitamente que
fosse registrado aqui e citado em `docs/LOCKS.md`. A citação foi feita; **o
registro neste arquivo nunca tinha sido feito**. Esta entrada fecha a pendência.

**Onde** (caminhos pós-Fase H, verificados no HEAD):

```go
// internal/wa-noise/core/client.go:117 — o mutex é campo de *Client
	messageSendLock sync.Mutex

// internal/wa-noise/core/send_adapter.go:68 — core empresta o ponteiro
func (t sendTransport) SendLock() *sync.Mutex { return &t.cli.messageSendLock }

// internal/wa-noise/capabilities/send/transport.go:141 — send declara a porta
	SendLock() *sync.Mutex

// internal/wa-noise/capabilities/send/message.go:80 e fb_message.go:129 — uso
	lock := t.SendLock()
	lock.Lock()
	resp.DebugTimings.Queue = time.Since(start)
	defer lock.Unlock()
```

**Problema**: o mutex que serializa **todos** os envios do cliente é declarado
por um pacote (`core`) e adquirido por outro (`capabilities/send`), através de
uma interface que exporta um `*sync.Mutex`. Nenhum dos dois pacotes é dono
completo do par estado/lock:

- `core` declara o mutex mas não sabe qual é a seção crítica que ele protege —
  ela vive inteira em `send/message.go:80-…` e `send/fb_message.go:129-…`;
- `send` define e executa a seção crítica mas não pode garantir nada sobre o
  ciclo de vida do mutex, que pertence a uma struct de outro pacote.

Nada disso está quebrado hoje, e vale ser explícito sobre o que **não** é
problema: não há ciclo de import (a interface é declarada em `send/` e `core` a
satisfaz, então a dependência de compilação é só `core → send`); e o ponteiro é
estável — `sendTransport` embrulha o `*Client`, então o mutex nunca é copiado
por valor, o que seria o erro clássico. Os dois pontos estão documentados no
código, em `send/transport.go:133-140` e `core/send_adapter.go:65-67`.

O risco é de manutenção, não de corrida: um refator futuro que mova
`messageSendLock` para dentro de outra struct, ou que faça `sendTransport`
deixar de embrulhar um ponteiro, quebra a garantia **em silêncio** — `go vet`
pega cópia de mutex por valor em alguns casos, mas não pega a mudança de dono.
E, por ser a exceção à regra, ele é o precedente que alguém vai citar para
abrir a segunda exceção.

**Correção sugerida**: mover a serialização de envio inteira para
`capabilities/send`, de modo que o mutex e a sua seção crítica fiquem no mesmo
pacote. Concretamente: `send` passa a possuir um `State` (como `retry.State`,
`prekeys.State` e `tctoken.State` já fazem) com o mutex privado, e a interface
`send.Transport` perde o método `SendLock()`. O `Client` deixa de ter o campo
`messageSendLock` e passa a guardar o `*send.State`.

O que precisa ser verificado antes: se algum caminho **fora** de `send/` adquire
o mesmo mutex. Pela inspeção desta etapa não adquire — os únicos dois call sites
de `SendLock()` são `send/message.go:80` e `send/fb_message.go:129`, e
`grep -rn "messageSendLock" internal/wa-noise` só devolve a declaração e o
adaptador. Se isso se confirmar sob revisão, a correção é local e barata.

**Status**: **CORRIGIDO** (lote E, 2026-08-07) pelo caminho que a própria
entrada sugeria: `capabilities/send/` ganhou o seu `State`, dono do mutex, como
`retry`, `prekeys` e `tctoken` já tinham. `Transport.SendLock() *sync.Mutex`
virou `Transport.State() *State`, e o campo `messageSendLock` saiu de
`core.Client` (que agora carrega `sendState send.State`).

A seção crítica não mudou de forma — continua cobrindo da gravação da mensagem
recente até o fim do envio. O que muda é quem é o dono: emprestar o ponteiro
não era incorreto, mas deixava quem lê `core/client.go` vendo um mutex sem saber
o que ele serializa, e quem lê `send/` vendo um lock sem dono aparente.

**Status original**: fora do escopo da Fase H. A Fase H é
reorganização de diretórios; isto é mudança de design de API (a interface
`send.Transport` perde um método e o `Client` perde um campo). Além disso é
mudança em código crítico de concorrência, e a regra 10 de
`internal/wa-noise/docs/CONTRIBUTING.md` exige revisão independente para esse
tipo de mexida — revisão que não cabe no fechamento desta etapa. Está citado
como exceção conhecida na tabela de `internal/wa-noise/docs/LOCKS.md` e na regra
6 do `CONTRIBUTING.md`. Registrado para decisão do usuário.

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

## F62 — cifragem que falha para TODOS os dispositivos devolvia sucesso, e o stanza ia sem destinatário

**Data**: 2026-08-07.
**Contexto**: varredura dos `// TODO` do projeto. O TODO original
(`send/encrypt.go:93` e `send/fb_encrypt.go:74`) dizia: *"return these errors
if it's a fatal one (like context cancellation or database)"*.

**Onde**: `internal/wa-noise/capabilities/send/encrypt.go` (`EncryptForDevices`),
`internal/wa-noise/capabilities/send/fb_encrypt.go` (`EncryptForDevicesV3`) e o
consumidor em `internal/wa-noise/capabilities/send/node_build.go:197`.

**Problema**: o laço de cifragem por dispositivo tolerava qualquer falha com
`continue`. Tolerar falha *parcial* está certo — um device sem sessão Signal é
rotina, e pular só ele é o comportamento correto. O que não estava tratado é o
caso degenerado em que **nenhum** dispositivo sobra:

```go
participantNodes, includeIdentity, err := EncryptForDevices(...)  // err == nil
if err != nil { return nil, nil, err }
participantNode := waBinary.Node{
    Tag:     participantsNodeTag,
    Content: participantNodes,   // <-- vazio, sem checagem
}
```

O `<participants>` sai vazio, o stanza vai para o fio sem destinatário nenhum,
o servidor responde, e `SendMessage` devolve **sucesso com ID de mensagem**.
Ninguém recebe, e nada no caminho de retorno diz isso.

Não é hipotético: `bundles := t.FetchPreKeysNoError(ctx, retryDevices)` engole
falha de busca de pre-key por design (está no nome). Se o servidor recusa as
pre-keys, nenhum dispositivo tem sessão, todos falham com `ErrNoSession`, e o
resultado é exatamente o acima.

**Por que o TODO original não era acionável como escrito**: ele pede para
distinguir erro fatal (banco) de erro por dispositivo. Isso não é possível por
`errors.Is`. `ContainsSession` devolve erro cru do `sql`, mas `ProcessBundle` e
`cipher.Encrypt` tocam o store por dentro do libsignal e devolvem tudo
embrulhado como `"failed to process prekey bundle"`. Metade do TODO, aliás, já
estava feita: `if ctx.Err() != nil { return }` já tratava cancelamento.

**Correção aplicada**: sentinela `ErrAllDevicesFailedEncryption`, disparado
quando `attempted > 0 && failed == attempted`. `attempted` conta só os
dispositivos que entraram na cifragem — o device do próprio usuário é pulado
antes, e contá-lo faria "todos falharam" nunca ser verdade numa lista que só
tem ele. A causa por dispositivo é preservada via `%w` duplo.

A checagem fica **depois** de `PutCachedSessions`: as sessões tocadas na
tentativa precisam ser persistidas mesmo quando a cifragem falha, senão o
próximo envio refaz o mesmo trabalho.

**Contrato que mudou**: `TestEncryptForDevicesSkipsDevicesWithoutSession`
afirmava, no próprio comentário, que com todos falhando *"a lista sai vazia —
mas sem erro, que é o contrato"*. Esse contrato era o defeito. O teste foi
reescrito como `TestEncryptForDevicesTodosSemSessaoEhErro`, e
`TestPrepareMessageNodeDropsHostedDevicesOnlyInGroups` passou a provar o mesmo
que provava antes (que o dispositivo hospedado não é removido em DM) pelo JID
que o erro novo cita.

**Status**: **CORRIGIDO** (2026-08-07). `make check` verde.

## F63 — `retryFrame` devolvia `ErrIQTimedOut` para qualquer tipo de requisição

**Data**: 2026-08-07. **Contexto**: mesma varredura de `// TODO`.

**Onde**: `internal/wa-noise/core/request.go:235`, que trazia um FIXME:
*"this error isn't technically correct (but works for now - the timeout param
is only used from sendIQ)"*.

**Problema**: a premissa **se sustenta hoje**, verificada nos três chamadores
de `retryFrame`: `sendIQ` (`request.go:174`) passa `query.Timeout`; o caminho
de envio de mensagem (`send/ack.go:47`, via `sendTransport.RetryFrame`) passa
`0` — e com `0` o `case` é inalcançável, porque `timeoutChan` vira um canal
que nunca entrega.

O que torna a premissa frágil é o terceiro: `DangerousInternalClient.RetryFrame`
(`core/internals.go:674`) expõe o parâmetro publicamente. Qualquer chamador
pode passar timeout > 0 de um contexto que não é IQ e receber um erro dizendo
"info query timed out".

**Correção aplicada**: `reqType` entra no texto do erro, para que a mensagem
não minta nesse caso. O sentinela `ErrIQTimedOut` foi **preservado como causa**
de propósito — quem chama `sendIQ` compara com `errors.Is`, e trocar o valor
quebraria essa comparação em silêncio.

**Status**: **CORRIGIDO** (2026-08-07).

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

## F69 — janela de pareamento diverge do WhatsApp Web oficial em validade e em política de expiração

**Data**: 2026-08-07.
**Contexto**: a página de `devui/` precisava de retry, e a pergunta "quanto
tempo vale um QR" não estava respondida em lugar nenhum do repositório.
Medido diretamente no `web.whatsapp.com` com automação de navegador,
observando o atributo `data-ref` de `div[data-testid="link-device-qr-code"]`.

**Medição do oficial** (2026-08-07, 324s de observação, 12 trocas):

- Intervalos entre códigos: `18, 21, 21, 21, 21, 20` segundos — **~20s
  uniformes**, sem primeiro código mais longo.
- Aos **122 segundos**, após **6 códigos**, a rotação **parou** e apareceu o
  overlay com o botão **"Selecione para recarregar o QR code"**.
- Depois disso os intervalos ficam irregulares (`45, 1, 61, 21, 64`) — é o
  comportamento pós-expiração, sob demanda.

**Nosso** (`internal/wa-noise/core/pair_constants.go:13-18`,
`core/qrchan.go:70-73`):

```go
qrCodeTimeout        = 20 * time.Second  // do 2º em diante
qrCodeFirstTimeout   = 60 * time.Second  // o 1º
qrCodeFirstBatchSize = 6
```

| | Oficial | wa-api |
| --- | --- | --- |
| Códigos por janela | ~6 | 6 |
| Validade | ~20s uniformes | **60s o 1º**, 20s os demais |
| Janela total | ~120s | ~160s |
| Ao esgotar | para e espera clique | `QRTimeout` + **desmonta a sessão** |

**Duas divergências, de naturezas diferentes:**

1. **O primeiro código dura 3× mais no wa-api.** Não é bug — herdado do
   upstream — mas é um QR válido por 60s onde o oficial expira em 20s. QR de
   pareamento é credencial: quem tiver a tela à vista nesse intervalo pode
   vincular um aparelho. Vale decidir se os 60s são intencionais.

2. **Ao esgotar, nós destruímos a sessão** (`onPairingTimeout` →
   `registry.Unregister` + `attach.Detach`), enquanto o oficial só para de
   girar e oferece recarregar. O nosso é mais seguro por padrão; a
   consequência é que reabrir exige um `/session/connect` novo, e não um
   simples "novo QR". Qualquer cliente precisa saber disso — foi o que
   determinou o desenho do retry na página.

**Erro de documentação encontrado junto**: o comentário em
`orchestrator.go:263-265` afirma que "wa-noise emite 20s para os 5 primeiros
e 60s para o último". É o **inverso** do código: `qrchan.go:72` aplica
`qrCodeFirstTimeout` quando ainda restam `qrCodeFirstBatchSize` códigos, ou
seja, no **primeiro**. Quem programar um cliente a partir desse comentário
vai errar a barra de progresso do primeiro QR por 40 segundos.

**Correção sugerida**: corrigir o comentário (uma linha, sem risco) e decidir
separadamente sobre os 60s do primeiro código.

**Status**: **item 1 (comentario invertido) CORRIGIDO** (2026-08-07). **item 2 (a validade de 60s do primeiro codigo) ABERTO** — e' decisao de seguranca, nao de implementacao.
arquivo fora do escopo desta tarefa; a validade de 60s é decisão de
segurança, não de implementação.

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

**Status**: **não corrigido** — descoberto ao validar a F79, e a correção
mexe no ciclo de vida da sessão, que não era o alvo.

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

**Status**: **não corrigido** — fora do escopo da rodada de testes.
