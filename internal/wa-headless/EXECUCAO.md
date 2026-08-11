# wa-headless — diário de execução

Checkpoint operacional do autopilot. **Não substitui** o `HANDOFF-INICIATIVA.md`,
que é a fonte de verdade de objetivo, arquitetura, invariantes e Definition of
Done. Aqui fica só o estado: que loop rodou, com que commit, validado como.

Branch: `feature/wa-headless-foundation`.

## Current

CAP: 04 — liveness e causas · **em andamento**
Loop: 04.3
Objective: `refreshOwner` — primeira leitura do SPA, que exercita o inventário
de módulos num caminho de baixo risco

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

### CAP-03 — sessão sobe e classifica · **parcial**

| loop | objetivo | commit |
|---|---|---|
| 03.1 | conjunto de flags de lançamento | `aaf6fe9` |
| 03.2 | reclaim de `Singleton` só quando provadamente obsoleto | `726d465` |
| 03.3 | isenção de sinal por FUNÇÃO, não por contagem | `093cf09` |
| 03.4 | `Browser`: handle de processo (`WaitExit`/`SignalStop`/`PID`) | `4b9b14b` |
| 03.5 | gate distingue sinal 0; `ProcessAlive` | `cdee0e1` |
| 03.6 | `Launcher`: reclaim → exec → esperar endpoint | `1c02445` |
| 03.7 | classificador de página sem ler mensagens | `dd65f24` |
| 03.8 | `Tab` + cadeia ponta a ponta contra Chrome real | `fac7fa1` |

**Verificado contra browser real** (Chrome local, páginas falsas em
`httptest`, nunca `web.whatsapp.com`):

```
TestBrowserChainLaunchesNavigatesAndClassifies  PASS   stopped_via=browser.close
TestBrowserChainReportsAWedgedPageAsUnresponsive PASS  (for(;;) na thread principal)
```

Invariantes cobertas por esta CAP: **11** (liveness por `Evaluate` com prazo),
**14** (zero PII — página pronta nunca tem o texto lido), **15** (reclaim no
boot). A **12** (toda morte com causa classificada) está parcial: as classes
existem, o ciclo de reciclagem que age sobre elas é CAP-04/05.

**Falta para fechar**: a asserção "contra conta real". Ver **B-03**.

### CAP-04 — liveness e causas · **em andamento**

| loop | objetivo | commit |
|---|---|---|
| 04.1 | `livenessCheck`: sonda por `Evaluate`, streak, latência | `e0ee05a` |
| 04.2 | verificação contra renderer travado de verdade | `c0793ad` |

**Verificado contra browser real**: página com `#pane-side` no DOM e
`for(;;)` na thread principal classifica `UNRESPONSIVE` enquanto
`ProcessAlive` confirma o processo vivo. O controle negativo (tirar o
`for(;;)`) faz o teste reprovar com `probed as "APP_READY"` — prova que ele
mede travamento, não presença de elemento.

**Limite declarado**: a sonda ainda não é a viagem autenticada que o contrato
descreve ("força I/O ao contexto autenticado"). Exige o inventário de módulos
do CAP-06. Descarta o modo de falha medido; não descarta UI montada sobre
socket morto.

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

* **LOOP 03.9** (bloqueado por B-03) — subir a cadeia contra o perfil pareado
  em `scripts/chromium-study/wa-session` e confirmar que `#pane-side` e o
  seletor de QR ainda casam com a marcação real. *Done quando*: as classes
  saem corretas contra o alvo, e o formato do `SingletonLock` fica verificado
  (fecha **H4**).
* **LOOP 04.3** — `refreshOwner`: primeira leitura do SPA. Exercita o
  inventário de módulos (CAP-06) num caminho de baixo risco, sem envio.
  *Done quando*: os nomes de módulo estão num lugar só, resolvidos no arranque,
  e renomear um de propósito derruba o boot com mensagem que nomeia a causa.
* **LOOP 04.4** — `getBrowserPid` na fachada. O `engine.Browser.PID()` já
  existe; falta expor pelo contrato, e isso é CAP-09.

## Findings

* **F-08 · guarda duplicada é camuflagem, não defesa — TRÊS vezes no mesmo
  dia.** No `Launcher` (recusa de URL vazia em `readEndpoint` e no laço), no
  `spa` (erro de sonda em `Classify` e em `ClassifyProbe`) e no gate de
  shutdown (contagem de marcadores como proxy da regra real). Em todos, o
  controle negativo passou verde porque apagar UMA das duas não mudava
  comportamento. Padrão a procurar em toda revisão de diff: **se remover a
  guarda não quebra nada, ela não está sendo testada — e provavelmente a outra
  também não.**
* **F-09 · o dublê tem de imitar o CONTRATO, não só a forma.** O falso
  avaliador codificava em JSON duas vezes; o `chromedp.Evaluate` real também —
  confirmado por medição contra Chrome, não por leitura. A convergência foi
  sorte: o teste de integração é que provou o contrato.

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

* **B-03 · a verificação final da CAP-03 precisa de sessão real.** Toda a
  cadeia está provada contra Chrome de verdade, mas contra páginas que EU
  escrevi. O que falta é confirmar que a marcação real do WhatsApp ainda casa
  com `#pane-side` e com `canvas[aria-label*="Scan"]` — e isso exige abrir o
  perfil pareado, ou exibir um QR.

  A restrição desta iniciativa é explícita: **avisar antes de precisar exibir o
  QR e esperar confirmação**. Nada aqui foi executado contra conta real.

  Decisão humana necessária: autorizar abrir o perfil pareado em
  `scripts/chromium-study/wa-session` (somente leitura, sem envio), ou parear
  um novo por QR. Enquanto isso não acontece, a CAP-03 fica **parcial** e o
  trabalho segue pela CAP-04, que não depende disso para ser escrita.

* **B-02 · `make coverage-gate` falha localmente** (`go: no such tool
  "covdata"`). **Pré-existente e não atribuível a este trabalho**: nem
  `go1.25.12` nem `go1.26.0` trazem `covdata` em `pkg/tool` quando o toolchain
  vem do module cache. O CI instala distribuição completa e não vê isso.
