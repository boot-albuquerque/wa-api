# wa-headless — diário de execução

Checkpoint operacional do autopilot. **Não substitui** o `HANDOFF-INICIATIVA.md`,
que é a fonte de verdade de objetivo, arquitetura, invariantes e Definition of
Done. Aqui fica só o estado: que loop rodou, com que commit, validado como.

Branch: `feature/wa-headless-foundation`.

## Current

CAP: — · LOOP H6.1, a guarda de PII deixa de depender de seletor · **DONE**

O `spa.Probe` não traz mais texto da página. O segundo `Evaluate` recebe um
conjunto FECHADO das nossas strings e responde quais viu; `knownMarkers` descarta
na chegada o que estiver fora do conjunto. `PageSnapshot.TextSample` foi
**removido** — não existe mais campo exportado de texto livre.

**A correção prevista pela entrada H6 não foi a adotada, e o motivo é uma
descoberta deste loop.** Gatear a leitura pela AUSÊNCIA de identidade mataria a
`ClassSessionConflict`: a tela de conflito só existe num perfil pareado — é isso
que a torna conflito — e o M3.3 mediu identidade persistida presente em T+0,01s,
antes do socket. A guarda fecharia a leitura exatamente na tela que a leitura
existe para reconhecer, e essa classe é terminal. Detalhe, falsificador e os
quatro controles negativos na resolução do **H6** em `HOUSEKEEP.md`.

Quatro controles negativos executados; o NC-3 (as duas camadas removidas) é o
único que prova que a asserção morde — contra Chrome real, o snapshot volta com
`Mum — see you at 8` e `+55 11 99999-0000`. O NC-4 **passou na primeira
tentativa por não ter aplicado**, e está registrado como nota de método.

Gate: build+vet do repo e do estudo OK, `-race` verde nos 4 pacotes,
`make lint` 269 contra 269 do pai medido com `stash` na MESMA árvore — delta 0.

Loop anterior: **04.3B** (`ec8063e`), abaixo.

### Loop anterior — CAP 04, LOOP 04.3B, o SPA percebe a queda SOZINHO? · **DONE**
Objective: a saída do socket medida no 04.3A foi percepção do SPA, ou reação ao
evento `offline` que a nossa emulação dispara? Medido cortando **só o
transporte** (`emulateNetworkConditionsByRule` sozinho), com `navigator.onLine`
intocado.

**Resposta: percebe sozinho — e leva ~34 s, não ~3 s.** O socket é
discriminador para as falhas que a produção enfrenta (rota em buraco negro,
upstream morto, portal cativo, servidor pendurado), que não disparam evento
nenhum. A CAP-04 tem fundação; o preço é uma ordem de grandeza de latência
acima do que o 04.3A tinha publicado. `EVIDENCIA-SPA.md` M5, finding **F-22**.

Loop anterior: **04.3A** (`437316e`), que mediu que a sonda de liveness reporta
saudável uma sessão sem servidor — finding **F-21**, que continua inteiro.

Ainda aberto e no `Next`: "o corte do READY honesto" (entre `meReadyTriggered`
e o socket `CONNECTED`), que continua bloqueado pelo **F-20**.

Sem dependência humana. O perfil pareado está disponível e autorizado para
observação só-leitura.

### LOOP 04.3B — controles negativos EXECUTADOS

Três mutações, todas revertidas antes do commit. As duas primeiras provam que a
precondição da perna nova morde; a terceira, que o teste de WebSocket morde.

**(A) o corte não acontece** — `severNetwork` passa `offline=false`. A
precondição positiva (o `fetch` da página tem de passar de `OK` para `FAIL`)
mata a corrida. Janela encurtada para 6 s só para não gastar 90 s de perfil:

```
REACHABILITY — with the network untouched: OK · with the transport cut: OK
DOM NETWORK EVENTS over the window: offline=0 online=0
  navigator.onLine went false     NEVER
  socket left CONNECTED           NEVER
the transport cut never landed: the reachability probe still answered "OK" with
the emulation active, so nothing measured here is attributable to a lost server
--- FAIL: TestRealSPALivenessUnderSeveredNetwork/transport-only (25.25s)
```

**(B) a perna nova vira a perna do 04.3A** — a tabela aponta `transportLeg`
para `SetNetworkOffline`. Aqui o corte CHEGA (`FAIL`), então a precondição
positiva passa e quem mata é a guarda "nada anunciou o corte":

```
REACHABILITY — with the network untouched: OK · with the transport cut: FAIL
DOM NETWORK EVENTS over the window: offline=1 online=0
  navigator.onLine went false     +1.4s after the reference (T+8.56s)
  socket left CONNECTED           +1.4s after the reference (T+8.56s) (to "OPENING")
navigator.onLine went false at T+8.56s; this leg exists to leave it alone, so its
whole premise is gone and the socket timeline cannot be read as self-detection
--- FAIL: TestRealSPALivenessUnderSeveredNetwork/transport-only (25.08s)
```

O (B) faz **dois** serviços. Além de mostrar que a guarda morde, o
`offline=1` é o **controle positivo do contador de eventos**: sem ele, o
`offline=0` das três corridas de medição poderia ser um listener quebrado em
vez de uma medição. E, de quebra, mostra o socket saindo do `CONNECTED` em
+1,4 s — no MESMO instante em que o `navigator.onLine` vira —, que é a
corroboração mais direta de que a saída rápida é reação ao evento.

**(C) o WebSocket não é severado** — `severWebSocketLeg` passa `offline=false`.
As duas asserções de entrega falham, nos dois formatos de corte:

```
full: the server received 10 frames while the page was supposed to be severed;
  the emulation does not reach an open WebSocket
full: the page received 10 frames while severed; the emulation does not reach an
  open WebSocket
full: severed window — page sent 10, server received 10, page received 10,
  readyState=1 closed=false
transport-only: (as três linhas idênticas)
--- FAIL: TestBrowserChainSeversTheWebSocketTransport (13.84s)
```

### Nota sobre a frase de lint do `437316e` — o número não é portátil

A mensagem do `437316e` afirma: *"`make lint` em 269, exatamente o mesmo do HEAD
sem estas mudanças (medido com stash)"*. **A validação independente não
reproduziu esse número**: mediu **283** para o mesmo commit, por dois métodos.

Reconferido nesta sessão, neste worktree, com `git stash`:

```
437316e (pai, árvore limpa)   269 issues   internal/wa-headless: 2 gocyclo
HEAD + as mudanças deste loop 269 issues   internal/wa-headless: as MESMAS 2
```

As duas gocyclo são `(*readinessMarks).record` (14) e `logTimeline` (11), ambas
pré-existentes e nenhuma tocada por este trabalho. O
`TestBrowserChainSeversTheWebSocketTransport` (15) e o `observeSeveredSession`
(11) que este loop introduziu **foram refatorados até sair da lista**, não
absorvidos no baseline.

**A lição, para comparações futuras:** o TOTAL do `golangci-lint` depende de
onde ele é invocado — a resolução de módulo dele não se limita à árvore para a
qual ele é apontado, então dois worktrees do mesmo commit dão totais diferentes
(269 aqui, 283 lá). **Um total absoluto numa mensagem de commit não é
verificável por quem não estiver na mesma árvore.** O que é verificável, e o que
deve ser afirmado daqui em diante, são duas coisas:

1. o **delta** medido com `stash` na mesma árvore, na mesma corrida;
2. o **conjunto de issues restrito ao subdiretório sob trabalho**, listado por
   arquivo e regra — que é invariante e é o que de fato responde "esta branch
   piorou alguma coisa?".

### CAP GATE do LOOP 04.3B (2026-08-12)

```
go build ./...                                   OK
go vet ./...                                     OK
gofmt -l internal/wa-headless/                   vazio
go test -race -count=1 ./internal/wa-headless/...
  wa-api/internal/wa-headless               ok  97.559s
  wa-api/internal/wa-headless/engine        ok   5.263s
  wa-api/internal/wa-headless/observability ok   1.603s
  wa-api/internal/wa-headless/spa           ok   1.953s
as 6 sondas TestRealSPA* continuam puladas por padrão
make lint                                        269 (pai: 269, idêntico)
```

Contra a conta real, sete corridas, todas com `stopped_via=browser.close`:

```
transport-only   ×3   PASS   (medição: +34,2s · +33,2s · +34,2s desde o corte)
transport-only   ×2   FAIL   (controles negativos A e B, mutações revertidas)
severed          ×1   PASS   (+3,0s, offline=1)
control          ×1   PASS   (nada se moveu, offline=0)
```

O contador de eventos entrou nas TRÊS pernas, então a severada e a de controle
foram reexecutadas — nenhuma asserção nova ficou sem exercício contra a conta.

**Perfil pareado: 2572 → 2686 arquivos.** Cresceu em todas as corridas, não
encolheu em nenhuma. `disparazaap` intocado (`ed9e651`, 0 linhas).

### FASE B1 — encerrada em 2026-08-12

| loop | objetivo | commit |
|---|---|---|
| B1.4-PARAM | sonda de SPA real deixa de ser presa ao perfil de laboratório | `2b776b1` |
| B1.4-CLASSIFY | classificar o `wa-session` com o instrumento do módulo | *(medição do Chief, sem código)* |
| B1.4-IDENTITY | achar onde a identidade do dono realmente mora | `17d951c` |
| B1.4-CORRECTIONS | as cinco falhas achadas pela validação independente | `b34c5db` |
| — | H5 (vazamento de browser) + método da H4 | `5b3c0af` |

**Validação independente executada** (sessão Opus separada, só-leitura):
VERDICT **PASS** com cinco correções, todas aplicadas em `b34c5db`. O validator
reproduziu por conta própria os dois pontos frágeis — rodou o mesmo instrumento
nos DOIS perfis (C-D) e tirou uma segunda amostra da linha do tempo (C-F), que
bateu com a primeira em 60 ms no painel e 0 ms no socket. A variância de
7,40s vs 15,61s que motivou a dúvida **não é ruído de execução**.

**CAP GATE executado** (2026-08-12, HEAD `b34c5db`, árvore limpa):

```
go build ./...                                   OK
go vet ./...                                     OK
gofmt -l internal/wa-headless/                   vazio
go test -race -count=1 ./internal/wa-headless/...
  wa-api/internal/wa-headless              ok  83.108s
  wa-api/internal/wa-headless/engine       ok   4.829s
  wa-api/internal/wa-headless/observability ok  1.163s
  wa-api/internal/wa-headless/spa          ok   2.026s
as 5 sondas TestRealSPA* continuam puladas por padrão
```

Perfil pareado ao longo de toda a fase: **375M/2160 → 437M/2467 arquivos**.
Cresceu em todas as corridas; `stopped_via=browser.close` em todas.
`disparazaap` intocado (`features/macbook-lucas`, `ed9e651`, 0 linhas).

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
| 04.3A | a sonda percebe sessão que perdeu o servidor? | `437316e` |
| 04.3B | o SPA percebe a queda SOZINHO, ou só reage ao evento? | *(este commit)* |

**Verificado contra browser real**: página com `#pane-side` no DOM e
`for(;;)` na thread principal classifica `UNRESPONSIVE` enquanto
`ProcessAlive` confirma o processo vivo. O controle negativo (tirar o
`for(;;)`) faz o teste reprovar com `probed as "APP_READY"` — prova que ele
mede travamento, não presença de elemento.

**Limite declarado, agora MEDIDO EM CAMPO**: a sonda ainda não é a viagem
autenticada que o contrato descreve ("força I/O ao contexto autenticado").
Exige o inventário de módulos do CAP-06. Descarta o modo de falha medido; **não
descarta UI montada sobre socket morto** — e o 04.3A produziu esse estado contra
a conta real e confirmou que a sonda reporta `Alive=true`/`APP_READY` nas 90
amostras do corte. Deixou de ser limite escrito por prudência e passou a ser
fato com evidência (`EVIDENCIA-SPA.md` M4.3).

**A fundação da CAP-04 está provada** (04.3B, `EVIDENCIA-SPA.md` M5): o SPA
percebe a queda do transporte **sozinho**, com `navigator.onLine` verdadeiro e
zero eventos `offline`. Logo o socket discrimina as falhas que a produção
enfrenta — rota em buraco negro, upstream morto, portal cativo, servidor
pendurado — e não só a queda de rede que se anuncia. **O preço está medido:
~33–34 s** de latência de detecção, e não os ~3 s que o 04.3A tinha publicado
(aqueles eram reação ao evento `offline` da nossa própria emulação).

**Falta para fechar a CAP-04**: o valor de queda (`OPENING`) é igual ao do boot,
então o veredito precisa de DURAÇÃO — e esse número continua não medido, agora
com um piso conhecido: tem de ser maior que os ~34 s de detecção somados ao
tempo de `OPENING` de um boot saudável. Ver **F-21**, **F-22** e o `Next`.

### FASE B0 — resolver o blocker do linter · **DONE**

| loop | objetivo | commit |
|---|---|---|
| B0.1–B0.4 | compilar o linter fixado com o Go do repo | `e5ee22e` |
| — | corrigir as 14 issues desta branch | `2aa304d` |

```
GO VERSION        1.26 (go.mod); go1.26.0 local; CI via go-version-file
OLD LINTER        v2.5.0, compilada com go1.25.1  -> não carrega o módulo
NEW LINTER        v2.12.2, COMPILADA com go1.26.0 por `make lint-tool`
OLD BASELINE      count=263  max_complexity=56
NEW BASELINE      count=267  max_complexity=56
NEW FINDINGS      +4, todos de código PRÉ-EXISTENTE (2 gofmt, 2 staticcheck),
                  medidos rodando a v2.12.2 em 4d2532e
REMOVED FINDINGS  nenhum
CONFIG CHANGES    nenhuma em .golangci.yml — nenhuma regra reduzida
```

**B-01 = CLOSED.** O mesmo gate roda local e no CI, com a versão nova, sem
reduzir exigência. As 14 issues introduzidas por esta branch foram
corrigidas, não escondidas: a contagem voltou ao baseline declarado.

> **CORREÇÃO (2026-08-12, LOOP 04.3B).** A frase original era "a contagem voltou
> a **267 exatos**". Ela **não se reproduz**: rodando `make lint` neste worktree
> hoje, o `437316e` (pai deste commit, árvore limpa) mede **269**, e o gate
> imprime `NOTA: a contagem de issues mudou (267 -> 269)` em toda corrida.
>
> **Isto é deriva PRÉ-EXISTENTE e não foi causada por este trabalho** — está
> medida no pai, com a árvore limpa, antes de qualquer mudança desta sessão. As
> duas issues a mais são de código pré-existente. A entrada fica corrigida aqui
> em vez de no número do baseline porque mexer no `.golangci-baseline` é decisão
> à parte, e ela não é desta sessão.

### FASE B1 — conta de teste e SPA real · **parcial**

| loop | objetivo | commit |
|---|---|---|
| B1.1 | observar o SPA real com perfil vazio | `946dfcb` |
| B1.2 | seletor de QR independente de idioma | `5158077` |
| B1.3 | mecanismo de pareamento (headful) | `c8691dd` |
| B1.4/B1.4a | instrumento de prontidão + eliminação de candidatos | `ca61ebb` |
| — | gate da REGRA DE DADOS na fronteira do SPA | `ea62bed` |

Evidência congelada em `EVIDENCIA-SPA.md`.

### CAP-06 — integração controlada com o SPA · **mecanismo pronto**

| loop | objetivo | commit |
|---|---|---|
| 06.1 | inventário de módulos verificado no arranque | `3c24062` |

Fonte: `whatsapp-web.js` **1.34.7**, lido de
`services/wa-worker/node_modules/` no `disparazaap` — a versão que o produto
roda. Ela usa **41 módulos distintos**; ficam os **8** que o próprio wwebjs
resolve no `AuthStore` ao arrancar. Cada capacidade acrescenta os seus.

**Verificado contra motor JS real**: página cujo `window.require` conhece os
oito e LANÇA para qualquer outro. Renomear um módulo derruba o boot nomeando
o que se moveu — o controle que a ADR-0006 D4 pede.

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

**Nenhum item abaixo depende de ação humana.** O perfil pareado existe, está
medido e a observação só-leitura está autorizada. O texto anterior desta seção
dizia "trabalho seguro esgotado" — estava errado, e por quê está na **F-19**.

> **Correção de nomenclatura (2026-08-12, LOOP H6.1).** O primeiro item abaixo
> estava rotulado `LOOP 04.3B`, mas esse número foi consumido pelo loop de
> auto-detecção que já está em `ec8063e`. A PERGUNTA continua aberta e correta;
> só o rótulo estava tomado, e quem lesse apenas esta seção concluiria que o
> trabalho ainda não foi feito. Renumerado para **04.3D**.

* **LOOP 04.3D** (o próximo) — **quanto tempo em `OPENING` significa morto?**
  O M4 mostrou que o socket reage em ~3 s mas cai para `OPENING`, que é o mesmo
  estado do boot: o valor instantâneo não separa "subindo" de "perdeu o
  servidor", só a DURAÇÃO separa. Sem esse número, um liveness que leia o socket
  escolhe entre matar sessão que está subindo e manter sessão morta.
  *Done quando*: existe uma distribuição de "tempo em `OPENING` num boot
  saudável" e outra de "tempo em `OPENING` sob corte", com mais de uma amostra
  cada, e o corte proposto sai delas — não de escolha de mesa. Repare que isto
  é medir onde o mecanismo PIORA (Regra 2): a pergunta é qual boot lento vira
  falso positivo.
* **LOOP 04.3C** — o corte do READY honesto: onde, entre `meReadyTriggered` e o
  socket `CONNECTED`, a sessão passa a poder AGIR. Era o 04.3A original e
  continua **bloqueado pelo F-20**: o tick de 250 ms não resolve uma janela de
  ~500 ms, e a leitura atual pode ser quantização, não medida. O desenho que
  escapa é carimbar a transição DENTRO da página e colher a linha do tempo
  pronta numa avaliação só — o que não fere a invariante 6, porque carimbar não
  é esperar e o prazo continua do lado Go. *Done quando*: a largura do corte sai
  com resolução menor que ela mesma, e o instrumento declara sua própria
  resolução.
* **Corte longo** — a janela do M4 foi de 90 s. Não se sabe o que o SPA faz em
  10 minutos sem servidor: desiste, muda de estado, mostra QR, ou fica em
  `OPENING` para sempre. É a mesma sonda com outra janela, e o custo é tempo de
  browser, não desenho novo.
* **LOOP 03.9** — confirmar que a marcação real ainda casa com o classificador
  e fechar a **H4** (formato do `SingletonLock`). *Método corrigido*: um perfil
  parado de forma limpa NUNCA mostra o arquivo — exige inspeção com o Chromium
  em execução, ou depois de parada suja. Ver a nota de 2026-08-12 na H4.
* **LOOP B1.5** — restart/restore sem QR, medindo parada, saída do processo e
  tempo até `app-ready`. Comparar com o `MEASURED` do handoff §F2 (p50 10,4s,
  p95 13,8s, observado até 15,8s).
* **LOOP 04.3** — `refreshOwner`: primeira capacidade de produto (nº 3 na ordem
  do `PARIDADE-WWEBJS.md` §3). O módulo de identidade já é conhecido pela M3.
  *Done quando*: os campos saem contra a página, e o inventário cresce só com o
  que esta capacidade usa.
* **LOOP 04.4** — `getBrowserPid` na fachada. O `engine.Browser.PID()` já
  existe; falta expor pelo contrato, e isso é CAP-09.
* **H5, passos 1 e 2** — travar a terminação com `WaitExit`/`ProcessAlive` e
  medir se o culpado é o `Browser.close` ou o `CleanStop` desistindo. O passo 3
  (política de escalada) é decisão humana e continua pendente.

## Findings

* **F-22 · o socket percebe a queda SOZINHO, mas leva ~34 s — e os "3 s" do
  F-21 eram reação à NOSSA emulação.** LOOP 04.3B, medido contra a conta real
  cortando **só o transporte** (`EVIDENCIA-SPA.md` M5).

  O 04.3A cortou com os dois comandos, e o `overrideNetworkState` faz o
  navegador emitir o evento `offline` na página. Como a validação independente
  provou que **o Chrome não fecha o socket** (quadros engolidos, `readyState`
  OPEN, `onclose` nunca), a saída do `CONNECTED` era decisão do próprio SPA — e
  havia duas causas possíveis que aquela perna não separa, porque dispara as
  duas: o EVENTO, ou o SPA notando o próprio tráfego parar.

  Cortando só os bytes, com `navigator.onLine` verdadeiro e **zero** eventos
  `offline` contados por listeners da própria página:

  | corte | evento `offline` | saída do `CONNECTED` |
  |---|---|---|
  | os dois comandos (04.3A) | dispara (`offline=1`) | +1,4 s a +3,1 s |
  | só transporte (04.3B) | **não dispara** (`offline=0`) | **+33,2 a +34,2 s** |

  Três corridas do mesmo perfil: +34,2 s, +33,2 s, +34,2 s contados do corte —
  estáveis dentro da grade de amostragem de 1 s.

  **Por que isto decidia a CAP-04 inteira.** Se o SPA só reagisse ao evento, o
  socket não seria discriminador em produção: as falhas que uma frota enfrenta
  — rota em buraco negro, upstream morto, portal cativo, servidor pendurado —
  deixam `navigator.onLine` **verdadeiro** e não disparam evento nenhum. A
  sessão ficaria em `CONNECTED` para sempre com o servidor morto, e a CAP-04
  ficaria sem sinal algum, com os sinais de tela já eliminados pelo F-18.
  **Percebe. A fundação existe** — com ~34 s de latência em vez de ~3 s, o que
  é uma ordem de grandeza e muda qualquer prazo escrito em cima disso.

  **O que NÃO foi medido**: qual mecanismo dá os ~34 s. Cheira a temporizador
  (keepalive, timeout de ping), mas ninguém olhou o tráfego — é leitura, não
  medição. E o F-21 continua inteiro: a sonda respondeu `Alive=true`/`APP_READY`
  nas 90 amostras das três corridas, e `#pane-side`, identidade e
  `meReadyTriggered` ficaram verdadeiros os 90 s também sob corte silencioso.

  **Correções que este achado obrigou** (feitas, não anotadas para depois):
  `EVIDENCIA-SPA.md` M4.2 dizia que o socket reage "sem depender de keepalive"
  — afirmação não medida e errada; M4.7 dava a saída como "+3 s estável" com
  três amostras, e a faixa real conhecida é +1,4 s a +3,1 s em seis;
  `spa/liveness.go` repetia os "três segundos".

* **F-21 · a sonda de liveness reporta SAUDÁVEL uma sessão que perdeu o
  servidor — e o socket que a perceberia cai num estado ambíguo.** LOOP 04.3A,
  medido contra a conta real cortando a rede da página
  (`EVIDENCIA-SPA.md` M4).

  Com o servidor inalcançável por 90 s, `spa.Monitor.Check` respondeu
  `Alive=true`/`APP_READY` nas **90** amostras, e `spa.Probe` respondeu
  `APP_READY` nas 90. Numa frota com standby e reciclagem, essa sessão é contada
  como capacidade e recebe trabalho. É o risco de fase 6 na forma nova: tudo por
  fora diz saudável, e desta vez o renderer até responde.

  Não é defeito de implementação — é exatamente o limite que `spa/liveness.go`
  já declarava. **O que mudou é o estatuto**: era prudência escrita, virou fato
  com evidência.

  Três coisas que não se adivinhariam da mesa, e as duas primeiras contrariam o
  que se esperaria:

  1. **`#pane-side`, identidade e `meReadyTriggered` ficam VERDADEIROS os 90 s
     inteiros.** O F-18 dizia que a identidade chega cedo demais para provar
     sessão viva; agora se sabe que ela **permanece** depois que a sessão morre.
     Raciocínio de boot virou fato medido.
  2. **O socket reage — mas para `OPENING`, que é o estado do boot normal**
     (M3.3). Logo `socket != CONNECTED` **não** distingue "está subindo" de
     "perdeu o servidor". O discriminador existe, mas o valor instantâneo não é
     ele: é o valor MAIS a duração. Um liveness escrito só sobre o enum escolhe
     entre matar sessão que sobe e manter sessão morta.
     *(Esta linha dizia "reage rápido — 3 s". **Corrigido pelo F-22**: os 3 s
     eram reação ao evento `offline` que a nossa própria emulação dispara. Sem
     o anúncio, a percepção leva ~34 s.)*
  3. **A volta é sozinha e em 2–3 s**, sem QR, sem renavegar, sem reiniciar o
     browser. Qualquer política de reciclagem que agisse dentro do primeiro
     minuto destruiria uma sessão que ia se recuperar.

  **Escopo do que foi medido**: corte de REDE, que é o caso mais benigno da
  família — o servidor continua existindo e aceitando o mesmo credential.
  Revogação, deslogamento e expiração continuam sem medição, e medi-los
  destruiria o ativo.

  **Controle negativo EXECUTADO, duas pernas**: a mesma sonda, no mesmo perfil,
  no mesmo minuto, sem cortar nada — todos os sinais imóveis por 90 s, socket em
  `CONNECTED` o tempo todo. Sem essa perna, a saída para `OPENING` não seria
  atribuível ao corte. A perna severada tem ainda a precondição travada em
  código: se `navigator.onLine` não ficar falso, o teste FALHA dizendo que o
  corte não chegou à página — senão "nada mudou" seria afirmação sobre a nossa
  emulação, não sobre a sessão.

  O instrumento de corte também foi provado **antes** de tocar na conta, contra
  Chrome real e servidor local
  (`TestBrowserChainSeversAndRestoresThePageNetwork`), incluindo a asserção de
  que o servidor **não recebe** a requisição durante o corte.

* **F-20 · o instrumento não resolve a janela que ele foi medir.** O
  `readinessTick` é de **250 ms** e a janela entre `meReadyTriggered` (T+5,32s)
  e o socket `CONNECTED` (T+5,82s) tem **500 ms — exatamente dois ticks**. A
  corrida independente do validator deu 5,31s → 5,82s, mas isso **não confirma
  nada**: as duas amostras caíram na mesma grade de 250 ms. O intervalo
  verdadeiro está em algum ponto entre ~250 ms e ~750 ms, e o número "500 ms"
  é provavelmente **quantização, não medida**.

  Apertar o tick não resolve sozinho: cada amostra é um round-trip CDP, e num
  tick pequeno o custo da amostra compete com o intervalo e borra a medida — o
  próprio comentário do arquivo já alertava que "a coarse tick would smear the
  very gap it exists to detect", e a recíproca também vale. O desenho que
  escapa é carimbar a transição DENTRO da página e colher a linha do tempo
  numa avaliação só.

  Isso **não** fere a invariante 6 ("nenhuma espera com relógio na página"):
  carimbar não é esperar, e o prazo continua do lado Go, que é o que a
  invariante protege. A distinção precisa ficar escrita no loop, senão a
  próxima revisão a lê como violação.

  Mesma classe do erro da Fase 6, em que o primeiro harness mediu com
  `time.Sleep` e teria invertido a decisão: **o instrumento precisa conseguir
  resolver o que se pede a ele.**

* **F-19 · "trabalho seguro esgotado" estava errado, e o modo de erro é
  reutilizável.** Os loops 02.1/02.2/02.3 (F-15/F-16/F-17) concluíram que não
  restava caminho de observação para decidir o B-04 sem interação humana. Os
  três interrogaram o **harness do estudo** (`scripts/chromium-study -mode
  wasession`), que trava no macOS e no Docker. Nenhum considerou o instrumento
  **do próprio módulo** (`realspa_test.go`) — que já estava provado neste
  ambiente, porque foi ele que produziu o M1 e o M2 do `EVIDENCIA-SPA.md`.

  Apontá-lo para o perfil candidato exigia mudar **uma constante**. Feito isso,
  o veredito saiu em 98 segundos e o B-04 caiu sem nenhum QR.

  A lição não é sobre este harness: **quando uma ferramenta se recusa a
  responder, verifique se você não construiu uma melhor desde então.** Três
  loops investigativos foram gastos refinando a pergunta para o instrumento
  errado.

* **F-18 · a identidade do dono é PERSISTIDA, logo prova pareamento e não
  sessão viva.** Volta em T+0,01s, antes de o socket abrir, porque sai de um
  store de preferências e não da conexão. Uma máquina offline desde a semana
  passada responderia igual de rápido. Mesma desqualificação que o inventário
  de módulos sofreu no M2.1 — descoberta desta vez medindo, não apanhando.

  **E o instrumento corrigido repetiu o defeito que consertou**: como
  identidade e inventário já estão de pé em T+0,00s, o `record()` parava no
  instante em que o painel renderizava (do cache), e um perfil **pareado porém
  offline** passava verde com `socket CONNECTED at never`. Um veredito
  alcançável por classe de perfil, igual à sonda `__x_wid` que ele substituiu.
  Achado pela validação independente; corrigido em `b34c5db` com controle
  negativo de duas pernas — a perna que roda a MESMA mutação contra o gate
  ANTIGO e passa é o que torna aquilo prova, e não asserção sobre si mesmo.

  Detalhe de método que vale além deste loop: **a contagem de ARQUIVOS é o
  sinal estável do perfil, não os bytes.** O `du` arredonda e o LevelDB
  compacta — os bytes caíram alguns KB duas vezes enquanto a contagem subia.
  A heurística "perfil encolhendo = corrupção" precisa olhar arquivos.

* **F-17 · o mesmo travamento acontece no ambiente Linux/Docker documentado
  — o harness `-mode wasession` não é hoje um observador confiável, nem no
  seu próprio ambiente de referência.** MINI-LOOP 02.3/B1.4-DOCKER-CONTROL,
  investigativo, sem implementação. Complementa **F-15** e **F-16**.

  ```
  CONTROL PROFILE: internal/wa-headless/.lab/test-account-profile (mesmo da
    F-16; UNPAIRED por EVIDENCIA-SPA.md M1/M2). O SOURCE nunca foi montado
    diretamente: uma cópia descartável foi feita com `cp -R` para
    /tmp/wa-control-profile-<timestamp> (fora do repositório, impossível de
    ser rastreada pelo Git — confirmado com `git check-ignore` recusando o
    path por estar fora da árvore). Fonte conferida intocada antes e depois
    (118M nos dois momentos, sem SingletonLock, sem processo).
  DISPOSABLE COPY: /tmp/wa-control-profile-<timestamp> — montada em
    /session dentro do container; removida ao final do loop (rm -rf), nunca
    entrou em Git.
  ENVIRONMENT: Docker Desktop confirmado operacional (`docker info` →
    linux/aarch64, casando com o host Apple Silicon). Nenhuma imagem
    `chromium-study` existia antes deste loop (`docker image ls` vazio para
    esse nome).
  IMAGE: `chromium-study:p6` — mesma tag do último exemplo documentado
    (RELATORIO-FASE-6.md §9), construída SEM alterar Dockerfile:
      1. `GOOS=linux GOARCH=arm64 go -C scripts/chromium-study build -o
         study-linux-arm64 .` — passo de reprodução documentado no mesmo
         §9, arquitetura casando com o binário que o Dockerfile já copia
         (`COPY study-linux-arm64 /study/study`).
      2. `docker build -q -t chromium-study:p6 scripts/chromium-study` →
         PASS, sha256:e7f027bd25a7bdf38b759a8bb1d8eecf76e0a9ae5ad46b5a690524725d39efbd.
  METHOD: mesma composição de flags já documentada nos exemplos de
    `docker run` do RELATORIO-FASE-6.md/4B.md (`--rm -e SKIP_BROWSER=1
    --cpus --memory -v <profile>:/session -v <out>:/out chromium-study:<tag>
    -mode <mode> ... -out /out/<arquivo>.json`), estendida ao `-mode
    wasession` — NENHUM exemplo `docker run` para este modo específico está
    documentado nos relatórios (só `waprep`/`waopen`/`targets` têm), mas o
    padrão de flags é idêntico e uniforme em todas as invocações
    documentadas de todos os modos, então compor para `wasession` não é
    inventar comando novo, é aplicar o padrão já estabelecido. Registrado
    como divergência de documentação, não como ambiguidade que bloqueasse o
    loop.
  RESULT: início 2026-08-11T16:16:45Z. `docker logs` confirmou o
    entrypoint.sh reconhecendo `SKIP_BROWSER=1` e chamando `exec
    /study/study -mode wasession ...` corretamente. `docker exec ... ps aux`
    confirmou Chromium real rodando DENTRO do container contra o profile
    montado (`--user-data-dir=/session`), consumindo CPU ativamente em
    múltiplos renderers. Mesmo assim, nenhuma linha de classificação
    ("session restored" ou "session not restored") foi produzida em mais de
    3 minutos — muito além do prazo interno de 120s do Poll que decide
    PAIRED/UNPAIRED. Container interrompido deliberadamente (processo da
    própria investigação, container `--rm` removido automaticamente ao
    parar).
  ELAPSED: ~198s+ até a interrupção (16:16:45Z → confirmado ainda rodando
    às 16:20:03Z), sem nenhum sinal.
  SIGNAL: nenhum — idêntico em espécie ao F-15/F-16: o processo chega a
    lançar o Chromium real contra o profile, mas nunca alcança o próprio
    Poll de classificação dentro do prazo esperado.
  CONCLUSION: DOCKER_CONTROL_UNSTABLE. O mesmo travamento acontece mesmo no
    ambiente Linux/arm64/Docker para o qual o harness foi originalmente
    escrito e medido — não é peculiaridade do macOS nativo (F-15/F-16), é o
    mecanismo `-mode wasession` (RunWASession, p4b_wasession.go) que não é
    hoje um observador confiável para este propósito, em nenhum dos dois
    ambientes testados. Isso é evidência de tooling quebrado, não evidência
    sobre o estado de nenhum profile.
  IMPACT ON B-04: nenhum novo. B-04 continua **AUTH_INTERACTION_REQUIRED**;
    `wa-session` continua UNKNOWN. O caminho "reproduzir no Docker/Linux
    documentado" que os loops 02.1/02.2 apontavam como próximo passo foi
    tentado e também não produziu observação — não há mais um caminho de
    observação indireta conhecido e não tentado para decidir isso sem
    interação humana.
  ```

* **F-16 · o controle negativo conhecido também trava no `-mode wasession`
  nativo — a instabilidade é do harness, não do `wa-session`.** MINI-LOOP
  02.2/B1.4-NATIVE-CONTROL, investigativo, sem implementação. Complementa a
  **F-15**.

  ```
  CONTROL PROFILE: internal/wa-headless/.lab/test-account-profile — o path
    exato que produziu a evidência limpa de UNPAIRED em EVIDENCIA-SPA.md
    (M1: QR aos ~15s; M2.3: socket_state=UNPAIRED aos 6,08s) e é o
    `labProfileDir` de internal/wa-headless/realspa_test.go:51. Confirmado
    existente e não estava em uso (sem SingletonLock, sem processo Chrome
    aberto) antes deste loop.
  KNOWN STATE: UNPAIRED, medido por TestRealSPAUnpairedBootObservation /
    TestRealSPADisqualifyReadinessSignalsOnLogin contra a SPA real
    (EVIDENCIA-SPA.md M1/M2) — não por este harness.
  METHOD: mesmo mecanismo do F-15 — scripts/chromium-study -mode wasession,
    mesmo Chrome 151.0.7922.76 local, mesmo user-agent, WA_SESSION_DIR
    apontado para o profile controle acima em vez do `wa-session`.
  RESULT (RUN 1, única execução): início 2026-08-11T15:51:09Z. Processos
    Chrome confirmados abertos contra o profile controle (`ps aux`). Nenhuma
    linha de log/stderr produzida. Interrompido deliberadamente
    (TaskStop, processo da própria investigação) em 2026-08-11T15:54:22Z —
    ~193s decorridos, ~60% além do prazo interno de 120s do Poll de
    classificação que a função usa para decidir PAIRED/UNPAIRED. Confirmado
    limpo depois: nenhum processo Chrome remanescente, sem SingletonLock.
    Segunda execução NÃO realizada: o padrão observado (travamento
    sustentado, sem sinal) já é o padrão DOMINANTE do F-15 (2 das 3
    execuções contra o `wa-session` travaram da mesma forma; só 1
    apresentou erro CDP rápido e transitório) — repetir não responderia a
    nenhuma pergunta nova.
  COMPARISON WITH WA-SESSION: idêntico ao padrão majoritário observado no
    F-15 contra o `wa-session` (travamento além do deadline interno, zero
    sinal de classificação produzido). Nenhuma das duas execuções teve
    resultado diferente entre os dois profiles.
  CONCLUSION: CONTROL_HARNESS_UNSTABLE. O profile com estado UNPAIRED
    conhecido e medido independentemente também não pôde ser classificado
    por este mecanismo neste ambiente. Isso aponta a causa para o CAMINHO —
    `scripts/chromium-study` nativo em macOS, fora do container Linux para
    o qual foi escrito e medido — e não para o estado do `wa-session`
    especificamente. `wa-session` permanece UNKNOWN (não vira PAIRED nem
    UNPAIRED por esta comparação: a ausência de classificação do controle
    apenas remove a hipótese de que a falha contra `wa-session` fosse
    explicada por algo específico daquele profile).
  ```

* **F-15 · o candidato `scripts/chromium-study/wa-session` não pôde ser
  classificado — nem PAIRED nem UNPAIRED — pelo único mecanismo existente que
  não arrisca exibir/capturar QR.** MINI-LOOP 02.1/B1.4-PRECHECK, investigativo,
  sem implementação.

  ```
  PROFILE: scripts/chromium-study/wa-session/profile
  RESULT: UNKNOWN (harness indisponível de forma confiável neste ambiente)
  METHOD: scripts/chromium-study -mode wasession (RunWASession, p4b_wasession.go),
    o único dos três modos do Track J que NÃO pode exibir QR — comentário do
    próprio código em main.go: "Track J. Three separate modes on purpose:
    only `waopen` can display a QR." Invocado via os mecanismos JÁ existentes
    no código, sem alteração: env var WA_SESSION_DIR (p4_wa.go:73, default
    "/session/profile") apontado para o profile candidato, e CHROME_BIN
    (p3_launcher.go:87, default "/usr/bin/chromium") apontado para o Chrome
    151.0.7922.76 local. UA explícito reaproveitado de
    internal/wa-headless/realspa_test.go's `realSPAUserAgent` (mesmo texto,
    nenhuma variável nova). Três execuções, `-soak 3s` (mínimo):
      1ª: falhou em segundos com erro CDP "Inspected target navigated or
          closed (-32000)" — não é o "session not restored" limpo que a
          função produz depois do Poll de 120s; é um erro de outra camada.
      2ª: travou além do timeout de 200s da própria ferramenta de execução
          (Bash), sem NENHUMA linha de log (nem stderr), e foi morta pelo
          timeout — nenhum processo Chrome sobrou ligado a este profile
          depois.
      3ª: travou por mais de 4 minutos sem produzir nenhuma linha de log —
          muito além do prazo de 120s do Poll interno que decidiria
          PAIRED/UNPAIRED — e foi interrompida deliberadamente
          (TaskStop) por ser o processo da própria investigação, não um
          processo de terceiro. Confirmado limpo depois: nenhum processo
          Chrome remanescente no profile, SingletonLock ausente.
  SIGNALS: nenhum sinal de classificação chegou a ser produzido em nenhuma
    das três tentativas — nem "session restored in Xs" (que exigiria
    #pane-side), nem o erro limpo "session not restored: ... deadline
    exceeded" que a função emite depois de 120s sem #pane-side (evidência de
    QR/tela de login). O harness trava ou falha ANTES de alcançar o próprio
    Poll de classificação.
  CONCLUSION: este harness (scripts/chromium-study) foi escrito e só foi
    medido dentro de container Linux com cgroup v2 e chamado via `docker run`
    (README.md, RELATORIO-FASE-4B.md, RELATORIO-FASE-6.md) — nunca nativo em
    macOS. Rodar nativo aqui expôs instabilidade real (uma falha rápida, dois
    travamentos), não um veredito. Isso é limitação do AMBIENTE de execução
    deste mecanismo específico, não uma medição do estado do profile.
    Conforme a REGRA DE CLASSIFICAÇÃO deste loop, UNKNOWN não pode virar
    PAIRED nem UNPAIRED — fica UNKNOWN.
  IMPACT ON B-04: nenhum. B-04 continua **aberto e não resolvido**: não há
    evidência comportamental, nem a favor nem contra, de que o profile
    candidato está pareado. O caminho que RESOLVERIA isto sem ambiguidade —
    rodar o mesmo `-mode wasession` (ou o modo Docker documentado, via
    `docker run ... chromium-study:p6/p4`) dentro do container Linux para o
    qual o harness foi escrito — está fora do escopo deste mini-loop
    investigativo (envolveria compilar e rodar uma imagem Docker, uma ação
    maior do que a checagem rápida que este loop pediu) e fica registrado
    como o próximo passo, não executado aqui.
  ```

* **F-12 · o inventário de módulos NÃO é sinal de prontidão.** `window.require`
  e os 8 módulos do `RequiredAtStartup` resolvem em T+0,01s na tela de LOGIN.
  Eu ia usá-los como metade da condição de READY. `EVIDENCIA-SPA.md` M2.1.
* **F-13 · o socket se nomeia.** `WAWebSocketModel.__x_state` transiciona
  `OPENING` → `PAIRING` → `UNPAIRED` sem sessão. É veredito da Meta, não
  interpretação nossa, e é o melhor candidato a discriminador de READY ~~junto
  com a presença de `__x_wid`~~. M2.2 e M2.3.
  **CORRIGIDO em 2026-08-12 pelo F-18:** `__x_wid` sai — o campo não existe
  nesta build. A parte do socket segue de pé e foi medida chegando a
  `CONNECTED` em T+5,82s no perfil pareado (M3.3).
* **F-14 · instrumento também sofre da armadilha do dublê permissivo.** A
  primeira sonda aceitava `ref` como prova de conexão viva — e `ref` é o campo
  do próprio QR, então ela ficava verdadeira na tela que deveria excluir. Só
  apareceu porque o controle negativo foi executado contra o alvo real.
* **F-18 · o F-14 ao contrário: sonda que NUNCA fica verdadeira.** MINI-LOOP
  B1.4b, investigativo. A sonda de prontidão comparava `#pane-side` contra
  `WAWebConnModel.__x_wid` — campo que **não existe nesta build, nem no perfil
  pareado**. Com um único veredito alcançável (`EARLY_MARKER`), ela não estava
  medindo: falhava por construção em qualquer perfil. A identidade do dono mora
  em `WAWebUserPrefsMeUser.getMaybeMePnUser()`/`getMaybeMeLidUser()`, onde o
  `whatsapp-web.js` 1.34.7 a lê (`src/Client.js:351-364`), e ali ela
  **discrimina**: `EMPTY` por 75 s no perfil não pareado, `PRESENT` em T+0,01s
  no pareado. `EVIDENCIA-SPA.md` M3.

  Duas lições que valem além deste caso:

  1. **Ausência num só perfil não é diagnóstico.** O M2.2 concluiu que `__x_wid`
     "só materializa com sessão" a partir de não vê-lo na tela de login. Sem o
     caso positivo, "ainda não apareceu" e "não existe" são a mesma observação.
  2. **Sonda com um único veredito alcançável não é instrumento.** Vale a
     pergunta em toda revisão de sonda: *qual entrada faria isto responder o
     contrário?* Se não houver, ela não mede — decide.

  Efeito colateral medido: a identidade é **persistida** (T+0,01s, antes de o
  socket abrir), logo prova PAREAMENTO, não sessão viva — mesma desqualificação
  do F-12, e ainda não há corte medido para o READY honesto (M3.5).

* **F-10 · o `spa/doc.go` cita um commit do wwebjs que não é o que roda.** Ele
  aponta `main @ 942d236a11ad (2026-07-27)`; o produto tem `1.34.7` instalado.
  O inventário segue a versão INSTALADA. O `doc.go` fica desatualizado de
  propósito por ora — corrigi-lo é mexer em texto fora do escopo do loop.

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

* ~~**B-04**~~ · **RESOLVIDO** em 2026-08-12, sem QR. O perfil
  `scripts/chromium-study/wa-session/profile` **ESTÁ pareado**, medido com o
  instrumento do próprio módulo contra o SPA real:

  ```
  socket        OPENING -> CONNECTED         (controle não pareado: OPENING ->
                                              PAIRING -> UNPAIRED aos 6,08s)
  QR            nunca apareceu               (o teste teria abortado)
  #pane-side    presente                     nós no DOM: 2691 (tela de QR: 347)
  identidade    getMaybeMePnUser() PRESENT   (controle: EMPTY por 75s)
  desligamento  stopped_via=browser.close    perfil cresceu, não encolheu
  ```

  Reproduzido de forma independente pelo validator, nos dois perfis, com o
  mesmo binário. O que destravou não foi evidência nova sobre o perfil — foi
  parar de perguntar ao instrumento errado (**F-19**).

  Autorização do usuário registrada: abrir o `wa-session` direto, só leitura,
  headless, sem envio. Vale para a observação continuada.

  Texto original abaixo, mantido porque as notas de controle F-15/F-16/F-17 o
  referenciam.

* **B-04 · AUTH_INTERACTION_REQUIRED — o perfil NÃO está pareado, e isso foi
  medido.** Em 2026-08-11 recebi a informação de que o pareamento havia sido
  concluído. Verifiquei antes de agir, e o perfil mostra QR aos ~6,1s com
  `socket_state=UNPAIRED` — veredito da própria Meta. O que veio como
  "evidência experimental" era saída de outro projeto (extensão Chrome,
  service worker, `npm run test:longevity`), não de `TestRealSPAPairing`.

  Sem sessão, a pergunta do B1.4 não tem resposta e eu não vou inventá-la.
  Comando de pareamento no relatório desta parada.

  **Nota sobre o candidato `scripts/chromium-study/wa-session`** (MINI-LOOP
  02.1/B1.4-PRECHECK): investigado antes de pedir novo QR, para ver se esse
  perfil dispensava a interação humana. Não dispensou — ver **F-15**. O único
  mecanismo existente que observa sem risco de expor/capturar QR
  (`-mode wasession`) não produziu veredito neste ambiente (nativo em
  macOS, fora do container Linux para o qual foi escrito). B-04 continua
  **AUTH_INTERACTION_REQUIRED**.

  **Nota de controle** (MINI-LOOP 02.2/B1.4-NATIVE-CONTROL, ver **F-16**): o
  mesmo mecanismo, contra o profile com UNPAIRED já medido de forma
  independente (`.lab/test-account-profile`), também travou sem produzir
  veredito. Isso descarta a hipótese de que a falha do F-15 fosse algo
  específico do `wa-session` — é o harness nativo em macOS que não é
  confiável para esta observação, não uma pista sobre o estado do profile.
  `wa-session` segue **UNKNOWN**; o caminho que resolveria isso sem
  ambiguidade é o ambiente Linux/Docker documentado, não tentado nestes dois
  loops. B-04 continua **AUTH_INTERACTION_REQUIRED**.

  **Nota de controle 2** (MINI-LOOP 02.3/B1.4-DOCKER-CONTROL, ver **F-17**):
  o caminho apontado pela nota acima FOI tentado — imagem `chromium-study:p6`
  construída sem alterar nenhum arquivo de build, rodada em container
  Linux/arm64 real contra uma cópia descartável do mesmo profile controle —
  e travou do mesmo jeito, sem produzir veredito, mesmo com Chromium real
  visivelmente rodando dentro do container. Não é mais uma questão de
  macOS-vs-Linux: `-mode wasession` não é hoje um observador confiável em
  nenhum dos dois ambientes testados. Não existe mais um caminho de
  observação indireta conhecido e não tentado. `wa-session` segue
  **UNKNOWN**. B-04 continua **AUTH_INTERACTION_REQUIRED** — a única
  pergunta em aberto é humana, não de tooling.

* ~~**B-01**~~ · **RESOLVIDO** em `e5ee22e` + `2aa304d`. Detalhe na FASE B0.
  Texto original abaixo, mantido porque a H2 do `HOUSEKEEP.md` o referencia.

* **B-01 · o gate de `make lint` está quebrado nesta branch.** O `chromedp
  v0.16.0` exige Go 1.26; o `golangci-lint v2.5.0` fixado no `ci.yml:37` foi
  compilado com go1.25 e o `go/types` embutido nele não lê o `chromedp/cdproto`
  (`panic: package requires newer Go version go1.26`). Três tentativas de
  conserto medidas e descartadas — detalhe e evidência em **H2** do
  `HOUSEKEEP.md` do módulo.

  **Não bloqueia a CAP-01**, que é investigação. Bloqueia o merge.
  Decisão humana necessária: subir o linter junto do `.golangci-baseline` num
  PR próprio, ou voltar para `chromedp v0.14.2` + Go 1.25.

* ~~**B-03**~~ · **RESOLVIDO** em 2026-08-12. A decisão humana que ele pedia —
  autorizar abrir o perfil pareado só-leitura, ou parear um novo por QR — foi
  tomada: **autorizado abrir o `wa-session` direto**, headless, sem envio. A
  marcação real foi confirmada contra a conta: `#pane-side` casa, o seletor de
  QR casa (M1.1), e o classificador sai correto nos dois estados.

  Fica **parcialmente aberto** só o formato do `SingletonLock` (**H4**), e por
  um motivo de método descoberto agora: o desligamento limpo remove o arquivo,
  então um perfil parado de forma limpa nunca o mostra. Exige Chromium em
  execução ou parada suja — o plano antigo ("matar e inspeccionar no 03.4")
  não produziria nada.

  Texto original abaixo.

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
