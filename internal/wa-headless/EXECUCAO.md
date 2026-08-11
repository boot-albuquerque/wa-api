# wa-headless — diário de execução

Checkpoint operacional do autopilot. **Não substitui** o `HANDOFF-INICIATIVA.md`,
que é a fonte de verdade de objetivo, arquitetura, invariantes e Definition of
Done. Aqui fica só o estado: que loop rodou, com que commit, validado como.

Branch: `feature/wa-headless-foundation`.

## Current

CAP: 03 — sessão sobe e classifica
Loop: 03.1
Objective: um `BrowserProcess` real (lançar Chromium com perfil persistente,
`WebSocketURL`/`WaitExit`/`SignalStop`), fechando a interface que a CAP-02
deixou sem implementação

## Completed

### CAP-02 — fundação do motor · **DONE**

| loop | objetivo | commit |
|---|---|---|
| 02.1 | `DeadlinePolicy` produtizada | `6c46f1a` |
| 02.2 | `Runner` aplicando prazo por classe | `1dc5559` |
| 02.3 | `OpLog` / rastreabilidade de onde parou | `67d3e07` |
| 02.4 | `CloseBrowserViaCDP` por conexão própria | `daffa96` |
| 02.5 | `CleanStop` espera a saída real do processo | `2c04656` |
| 02.6 | gate estático de shutdown + controle negativo | `e155a62` |
| 02.7 | `PrimeTab` | `7101ecd` |
| 02.8 | gates de módulo + doc.go + HOUSEKEEP | `e47c5b7` |

**CAP GATE executado** (2026-08-11, HEAD `e47c5b7`, árvore limpa):

```
go build ./...                          OK
go vet ./...                            OK
go test -race ./internal/wa-headless/...
  wa-api/internal/wa-headless              ok  1.345s
  wa-api/internal/wa-headless/engine       ok  2.800s
  wa-api/internal/wa-headless/observability ok 1.315s
scripts/chromium-study: build+vet+test  ok  0.193s
```

16 controles negativos executados, um por invariante, cada um com a saída da
falha colada na mensagem do commit que o introduziu.

Invariantes do handoff cobertas por esta CAP: **2** (shutdown por
`Browser.close`), **3** (nada mata browser por sinal), **4** (`stopped_via`
sempre registrado), **5** e **6** (prazo do lado Go em todo caminho CDP),
**7** (nenhuma espera com relógio na página).

### CAP-01 — inventário real de paridade · **DONE**

| loop | objetivo | resultado |
|---|---|---|
| 01.1 | ler a interface `WaClientAdapter` | 14 métodos: 6 obrigatórios, 8 opcionais |
| 01.2 | achar os call sites reais no produto | todos os 14 têm call site no `runner.ts` — nenhum declarado e não usado |
| 01.3 | consolidar a matriz | `PARIDADE-WWEBJS.md`, commit abaixo |

Validação: cada número de linha da matriz foi conferido contra o arquivo
citado; `disparazaap` permaneceu com `git status --porcelain` vazio (leitura
apenas — aquele repo é da sessão C0/C1).

## Next

* **LOOP 03.1** — `BrowserProcess` concreto: lançar Chromium com perfil
  persistente e reclaim de `Singleton`, implementando a interface que a CAP-02
  definiu. *Done quando*: sobe e desce um Chromium real com
  `stopped_via=browser.close`, e a invariante 15 (reclaim no boot) está travada
  em teste.
* **LOOP 03.2** — navegar até o alvo e classificar a página
  (`qr` / `ready` / `login_required` / `unresponsive`), tudo sob `Runner.Do`.
  *Done quando*: a classificação é derivada de sinal estrutural + `Evaluate`
  com prazo, e um alvo que não responde sai como `unresponsive`, nunca como
  saudável.

## Findings

* **F-04 · a fenda não é o `wa-api-adapter.ts` — é a superfície HTTP do
  `wa-api`.** O `wa-api` já é `AdapterKind` de primeira classe
  (`disparazaap` `services/wa-worker/src/index.ts:279`, flag
  `WA_WA_API_ENABLED`) e o adapter TS já fala HTTP/WS com este serviço, hoje
  servido pelo `wa-noise`. O `wa-headless` vira um **segundo motor atrás de
  rotas que já existem e já têm consumidor**. Fecha a incerteza nº 1 do handoff
  §7 na **leitura B**, por evidência. Reorganiza a CAP-09: não é fachada nova,
  é servir contrato existente.
* **F-05 · o escopo real são SEIS capacidades, não catorze.** Oito das 14 já
  são servidas pelo `wa-noise`. Só `fetchMessages`, `onMessageMeta`,
  `livenessCheck`, `refreshOwner`, `getBrowserPid` e `backupNow` exigem o motor
  de browser. Detalhe e ordem em `PARIDADE-WWEBJS.md` §3.
* **F-06 · `sendText` deixa de ser a primeira capacidade de produto.** O
  `wa-api` já envia pelo `wa-noise`; o envio pelo `wa-headless` só é necessário
  quando uma conta rodar no motor de browser, ou seja, é consequência da
  CAP-09. O fluxo Resolve → Validate → Act → Verify continua obrigatório quando
  chegar.
* **F-07 · o runner degrada em silêncio.** Cada opcional tem guarda
  `if (!adapter.X) return` (`runner.ts:1230`, `:2129`, `:2680`, `:2791`,
  `:717`). Capacidade faltando não vira erro — vira funcionalidade que sumiu
  sem aviso. Vale para a CAP-10: paridade tem de ser medida, não observada.

* **F-01 · o `OpLog` não tem política de redação de erro.** `OpRecord.Err` pode
  citar o que a página lançou. Truncado em 200 caracteres, o que limita o raio e
  não o conteúdo. Débito, não vazamento — nada consome o log hoje. Registrado
  como **H1** no `HOUSEKEEP.md` do módulo; decisão fica para a CAP-04.
* **F-02 · o `CleanStop` diverge do estudo de propósito.** O
  `p4c_lifecycle.go` sinalizava na hora quando o comando CDP errava; aqui espera
  primeiro, porque o browser foi medido respondendo em 2 ms e derrubando o
  socket — resposta perdida é o caso ordinário, não recusa. Daí a classe
  `browser.close_unconfirmed`.
* **F-03 · dublê mais permissivo que a produção, encontrado e corrigido.** O
  teste de conexão caída dependia de `CloseClientConnections`, que não fecha
  conexão sequestrada por WebSocket: passava por sorte de timing. O dublê ganhou
  `actionDrop`. ARMADILHAS §1.

## Blockers

* **B-01 · o gate de `make lint` está quebrado nesta branch.** O `chromedp
  v0.16.0` exige Go 1.26; o `golangci-lint v2.5.0` fixado no `ci.yml:37` foi
  compilado com go1.25 e o `go/types` embutido nele não lê o `chromedp/cdproto`
  (`panic: package requires newer Go version go1.26`). Três tentativas de
  conserto medidas e descartadas — detalhe e evidência em **H2** do
  `HOUSEKEEP.md` do módulo.

  **Não bloqueia a CAP-01**, que é investigação. Bloqueia o merge.
  Decisão humana necessária: subir o linter junto do `.golangci-baseline` num
  PR próprio, ou voltar para `chromedp v0.14.2` + Go 1.25.

* **B-02 · `make coverage-gate` falha localmente** (`go: no such tool
  "covdata"`). **Pré-existente e não atribuível a este trabalho**: nem
  `go1.25.12` nem `go1.26.0` trazem `covdata` em `pkg/tool` quando o toolchain
  vem do module cache. O CI instala distribuição completa e não vê isso.
