# Relatório — Fase 1 e 2

Estudo de controllers CDP em Go para automação determinística de SPAs Chromium
em alta escala, sobre backend Go em Kubernetes.

Todos os números deste documento vêm dos JSONs em `results/` (arm64) e
`results-amd64/` (amd64/SBX). Nada foi estimado; o que não foi medido está
listado na seção "O que não foi medido".

---

## Sumário executivo

Três conclusões, em ordem de impacto.

**1. A arquitetura candidata foi refutada na sua premissa.** A hipótese
`chromedp control-plane + cdproto data-plane nos hot paths` se apoiava num ganho
de 1,8–2,3× medido na Fase 1. Esse ganho **não é real**: ele vinha de o caminho
cdproto fazer *menos trabalho*, não o mesmo trabalho mais barato. Numa página
que reporta o que de fato aconteceu, o caminho rápido entregou **zero jobs
corretos** — em arm64 e em amd64. Com garantias equivalentes, o **chromedp
high-level venceu** o cdproto escrito à mão nas duas arquiteturas.

**2. A escolha do controller se mantém: chromedp.** Não pelo motivo original.
Sob concorrência, o chromedp entrega **3,1–3,3× mais throughput** que o rod em
amd64 real, e o rod colapsa em confiabilidade (18,8% de falhas em concorrência
32). O rod também apresenta um comportamento destrutivo: fechar sua conexão
derruba um browser externo compartilhado.

**3. O gargalo é CPU do renderer, não memória nem controller.** Provado por
cgroup v2 em Kubernetes real: 23% dos períodos CFS throttled, e `memory.events`
inteiramente zerado. Memória sobra em ~40×. O overhead do controller é de 13–27
MB — irrelevante frente ao browser.

---

## Regra epistemológica adotada

| classificação | significado |
|---|---|
| **MEASURED** | número saiu de uma execução registrada em JSON |
| **SOURCE-CONFIRMED** | lido no código-fonte da biblioteca, com arquivo e linha |
| **OBSERVED** | visto em execução, sem N suficiente para estatística |
| **INFERRED** | dedução consistente com a medição, sem prova direta |
| **HYPOTHESIS** | não testado |

Correlação não foi convertida em causalidade em nenhum ponto sem experimento de
ablação. Onde a causa não foi demonstrada, está dito.

---

## As quatro camadas

O erro central que este estudo existe para evitar é atribuir à biblioteca o que
pertence à política de lançamento do browser.

| camada | o que é | como foi fixada |
|---|---|---|
| browser | Chromium 151.0.7922.108 | **mesma build** nos dois ambientes |
| launch policy | `CanonicalBrowserProfileV1` (27 flags) | lançada pelo `entrypoint.sh` **antes** de qualquer controller conectar |
| controller | chromedp / rod / cdproto / CDP cru | o que está sob teste |
| workload | SPA leve, SPA pesada, página hostil | embutidas no binário, servidas em loopback |

**Nenhum controller lança browser.** Todos recebem a URL de um browser já de pé
via `--remote-debugging-port`. É isso que torna a comparação uma comparação de
controllers.

---

## Ambientes

### ENV-A — local (Fase 1 e parte da Fase 2)

```
host        macOS 15.6, arm64, 10 cores (4P+6E), 16 GiB
container   Docker 27.3.1, linux/arm64, cgroup v2
kernel      6.10.11-linuxkit aarch64
limites     --cpus=4 --memory=4g --memory-swap=4g
```

### ENV-B — Kubernetes real (Fase 2)

```
cluster     SBX (decolapps) — sandbox/homologação
node        sbx-node, Contabo VPS20, amd64
SO/kernel   Ubuntu 24.04.4 LTS / 6.8.0-110-generic
k3s         v1.34.5+k3s1
runtime     containerd 2.1.5-k3s1
node        6 vCPU / 12,25 GiB
pod cgroup  cpu.max=300000 100000 (3 vCPU)  memory.max=3 GiB
```

Guardrails: namespace efêmero, `ResourceQuota` 3 CPU/3 Gi, `PriorityClass -10`
com `preemptionPolicy: Never`. Fora do GitOps de propósito — versionar faria o
Argo CD recriar o namespace após a limpeza. Imagem pública + `kubectl cp`, sem
tocar no `registry.gitlab.com`.

### Versões, idênticas nos dois ambientes

```
Chromium     151.0.7922.108 (Debian bookworm)
Go           1.25.12
chromedp     v0.16.0
cdproto      v0.0.0-20260804232424-e85f50dbfd32
rod          v0.116.2
playwright-go v0.6000.0 (driver Playwright 1.60.0) — auditado, não executado
coder/websocket v1.8.12
```

O Chromium ser a **mesma build** nas duas arquiteturas é o que autoriza comparar
direção de resultado entre elas.

> **Atenção ao ler números absolutos entre ENV-A e ENV-B.** São máquinas
> diferentes (Mac com 4 CPUs dedicadas × VPS Contabo com 3 vCPU compartilhados).
> Comparar throughput absoluto entre elas **mede a máquina, não a arquitetura**.
> Só a *direção* dos resultados é comparável.

---

## Fase 1 — resultados

### Auditoria arquitetural (SOURCE-CONFIRMED)

```
chromedp / rod:   Go → biblioteca → CDP/WebSocket → Chromium
playwright-go:    Go → playwright-go → pipe stdio → processo Node.js
                     → Playwright JS → CDP → Chromium
```

O `playwright-go` **embarca um runtime Node.js** (`run.go:388`) e fala por pipe
stdio (`transport.go:104`), driver Playwright 1.60.0 — um processo e um runtime
extra por instância.

**Achado operacional:** `playwright-go v0.6100.0`, a release mais recente, **não
é instalável** — o `go.mod` dela declara `github.com/mxschmitt/playwright-go`,
o path antigo do autor original. Fixamos v0.6000.0.

### Decisões implícitas das bibliotecas (SOURCE-CONFIRMED)

| mutação | chromedp v0.16.0 | rod v0.116.2 |
|---|---|---|
| User-Agent | não altera | **implícito/hardcoded** — `Chrome/114.0.0.0` em toda página (`browser.go:78`→`:300`, `devices/list.go`) |
| viewport / device metrics | não altera | **implícito** — 1280×800, DPR 1 |
| Accept-Language | não altera | **implícito** — `en` |
| touch emulation | não altera | **implícito** |
| `enable-automation` | explícito, default on | explícito, default on |
| sandbox | não toca | **implícito e condicional** — `inContainer` ⇒ `--no-sandbox` (`launcher.go:105`) |
| site isolation | não toca | **desliga por default** |
| fechar browser externo | não | **destrutivo por default** |
| processo auxiliar | nenhum | `leakless` |

Pelo princípio "a biblioteca não deve alterar silenciosamente identidade,
comportamento ou política de segurança do browser": **o rod viola em quatro
pontos, o chromedp em nenhum.** Todos configuráveis — mas o default é o que roda
em produção.

O `--no-sandbox` automático do rod é o mais grave: a biblioteca **reduz política
de segurança do browser ao detectar container**, sem que o código peça.

### Memória — por que `sum(RSS)` mente (MEASURED)

Mesmo estado, mesmo instante:

```
sum(RSS) dos processos Chromium    ~1,1 GiB
cgroup v2 memory.current             268 MiB
```

**4× de erro.** Processos do Chromium compartilham centenas de MB de mapeamentos;
somar RSS conta isso uma vez por processo. Toda métrica de memória deste estudo
usa `memory.current`.

### Alavanca de site isolation (MEASURED)

Piso do browser ocioso, mesmo controller, só a launch policy mudando:

| política | piso |
|---|---|
| canônica (isolation desligado) | **240 MiB** |
| sem `disable-site-isolation-trials` | **253 MiB** |

Apenas **13 MiB** — contra ~200 MB observados no spike inicial com
`web.whatsapp.com`. A razão: `site-per-process` só cria renderers extras com
**múltiplas origens**, e o workload sintético tem uma só.

> **A alavanca é real, mas sua magnitude é função da diversidade de origens do
> workload, não da flag.** Um SPA real, com CDN e terceiros, paga muito mais que
> uma página de origem única.

### Overhead do controller (MEASURED)

Todos entre **13 e 27 MB** de RSS, 4–8 goroutines, 6–7 threads, 10–12 FDs,
contra um piso de browser de 240 MiB. **Overhead de controller não é critério de
decisão** neste problema.

---

## Fase 2 — resultados

### Parte H — o ganho do cdproto com semântica equivalente

**Desenho.** Os três níveis rodam na **mesma biblioteca, mesmo contexto, mesma
conexão**; só a implementação da operação muda. Round-trips CDP contados por um
**proxy WebSocket** entre controller e browser — medição idêntica para todos,
nenhuma biblioteca instrumentada.

O L1 replica o chromedp **chamada por chamada**, lido da fonte:

- poll a 5 ms (`query.go:136`)
- `GetBoxModel` + o predicado de `js/visible.js`
- `ScrollIntoViewIfNeeded` → `GetContentQuads` → centróide → `Input.dispatchMouseEvent` (`input.go:57`)
- `dom.Focus` + eventos de tecla por caractere (`KeyEventNode`, `input.go:184`)

A página reporta a verdade em `window.__truth`, então um job que **retornou sem
erro** pode ainda ser contado como **falso sucesso**.

**ENV-B (amd64, SBX), N=40:**

| nível | jobs/s | **corretos/s** | falso sucesso | CDP cmd/job | bytes/job | p50 | p95 | allocs/job |
|---|---|---|---|---|---|---|---|---|
| L0 raw evaluate | 4,79 | **0,00** | **40/40** | 25,9 | 24.736 | 206,6 ms | 273,5 | 1.898 |
| L1 cdproto equivalente | 2,73 | 2,73 | 0 | 62,9 | 55.871 | 359,8 ms | 462,2 | 3.238 |
| L2 chromedp high-level | **3,05** | **3,05** | 0 | 60,4 | **43.307** | **320,4 ms** | **391,9** | 3.060 |

**ENV-A (arm64, local), N=40:**

| nível | jobs/s | **corretos/s** | falso sucesso | CDP cmd/job | p50 | p95 |
|---|---|---|---|---|---|---|
| L0 raw evaluate | 11,69 | **0,00** | **40/40** | 26,2 | 70,0 ms | 128,0 |
| L1 cdproto equivalente | 3,67 | 3,67 | 0 | 64,0 | 132,0 ms | 682,3 |
| L2 chromedp high-level | **7,09** | **7,09** | 0 | 62,2 | 135,6 ms | **170,6** |

**Leitura.** O L0 entregou **zero jobs corretos** nas duas arquiteturas. O ganho
de 1,8–2,3× da Fase 1 era artefato de semântica reduzida — não é um ganho.

Com garantias equivalentes e o mesmo número de round-trips (60 vs 63), o
**chromedp high-level venceu o cdproto manual**: 1,12× em amd64, 1,93× em arm64,
com cauda melhor nos dois (p95 391,9 × 462,2 e 170,6 × 682,3). O chromedp ainda
transmite **menos bytes** por job com o mesmo número de comandos (43 KB × 56 KB).

**Ressalva honesta:** o L1 é a *nossa* implementação. Um L1 que cacheie o nó do
documento e use eventos de DOM em vez de re-`GetDocument` a cada poll pode virar
o resultado. O que a medição estabelece com firmeza é que **o ônus da prova
mudou de lado**: o fast path precisa se justificar, não se presumir.

### Parte U — correção sob DOM hostil

Página com input controlado (estilo React, estado reaplicado a cada render) e
overlay transparente sobre o botão:

| nível | overlay cobrindo o botão |
|---|---|
| L0 | **12/12 falso sucesso** — atravessa o overlay, valor nunca commitado |
| L1 | falha com timeout |
| L2 | falha com timeout |

**Nenhum acerta, mas os modos de falha são opostos.** L1/L2 falham **alto** (job
vai para retry, sistema se recupera); o L0 **retorna sucesso tendo corrompido
silenciosamente** — o pior desfecho possível em automação crítica.

Mecanismo, SOURCE-CONFIRMED:

- `element.click()` via `Runtime.evaluate` ignora hit-testing: dispara no botão
  mesmo coberto por overlay, mesmo com `pointer-events: none`, mesmo com tamanho
  zero.
- `el.value = 'x'` **não dispara o `onChange` sintético do React**. Num input
  controlado o valor é revertido no próximo render — foi exatamente o que a
  página registrou.

**Achado adicional:** nem chromedp nem rod fazem *occlusion / actionability
check*. O chromedp clica coordenadas (`GetContentQuads` → centróide →
`dispatchMouseEvent`) sem verificar se o elemento é o topo naquele ponto. O
Playwright tem essa checagem. Para automações críticas, é uma lacuna real da
arquitetura candidata.

### Parte C/D — curva de escala em Kubernetes real (chromedp, 3 vCPU)

| conc | jobs/s | p50 | p95 | p99 | falhas |
|---|---|---|---|---|---|
| 1 | 2,93 | 331,6 ms | 418,3 | 435,6 | 0 |
| 2 | 4,53 | 431,2 ms | 527,3 | 560,4 | 0 |
| 4 | **5,87** | 613,6 ms | 934,5 | 1.060,6 | 0 |
| 8 | **6,33** (pico) | 1.264,5 ms | 1.346,8 | 1.559,4 | 0 |
| 16 | 5,89 | 2.682,0 ms | 3.287,4 | 3.375,7 | 0 |
| 32 | 5,60 | 5.467,4 ms | 8.095,2 | 8.200,2 | 0 |

- **Região linear:** 1 → 4
- **Knee: concorrência 4** — 5,87 jobs/s = 93% do pico, com p99 ~1 s
- **Saturação: 8**
- **Além de 8:** throughput cai enquanto p50 cresce proporcionalmente
  (1.264 → 2.682 → 5.467 ms). Congestionamento clássico: a fila cresce, a vazão
  não. Sem colapso.

**Qual recurso satura — provado, não inferido.** cgroup v2 do pod ao fim da
bateria:

```
cpu.stat        nr_periods 3871   nr_throttled 893   (23% dos períodos)
                throttled_usec 60.868.792  (60,9 s)
                usage_usec 694.595.499     (694,6 s)
memory.events   low 0  high 0  max 0  oom 0  oom_kill 0  oom_group_kill 0
```

**CPU do renderer, com 23% de throttling no CFS. Memória nunca foi pressionada.**
Consumo sobre o ocioso: 224 / 307 / 375 MiB para 8 / 16 / 32 páginas
concorrentes — dentro de um limite de 3 GiB, sobrando ~8×.

### Parte Q1 — chromedp × rod em amd64

| conc | chromedp | rod-canon | razão | falhas do rod |
|---|---|---|---|---|
| 8 | **6,33** | 1,99 | 3,2× | 0 |
| 16 | **5,89** | 1,77 | 3,3× | 0 |
| 32 | **5,60** | 1,78 | 3,1× | **30/160 (18,8%)** |

E o mesmo padrão em arm64:

| conc | chromedp | rod-canon | p99 do rod |
|---|---|---|---|
| 2 | 10,79 | 3,54 | 2.313 ms |
| 4 | 14,00 | 3,68 | 16.014 ms |
| 8 | 15,82 | 3,59 | 33.169 ms |
| 16 | 33,51 | 4,29 | 55.318 ms |

**O rod fica plano em ~1,8–4,3 jobs/s independentemente da concorrência, nas duas
arquiteturas.** Em amd64 ele ainda perde confiabilidade sob carga.

Uma hipótese causal foi **testada e descartada**: reconfigurar o *sleeper* do rod
(de `BackoffSleeper(100ms, 1s)` — `utils.go:76` — para 2 ms/20 ms) corrige a
latência single-job (p50 382 → 181,9 ms, empatando com o chromedp em 175,5 ms na
mesma corrida), mas **não corrige a concorrência**: 3,91 jobs/s em conc 8 contra
4,16 do rod padrão, com o chromedp em 18,17.

**Logo o teto de concorrência do rod não é a política de espera.** O mecanismo
real **não foi demonstrado** — ver "O que não foi medido".

### Parte O — jobs/core-hora e jobs/GiB-hora (no knee, conc 4, amd64)

| métrica | valor |
|---|---|
| successful jobs/s | 5,87 (0 falhas) |
| **jobs/core-hora** | **7.044** |
| **jobs/GiB-hora** | **~96.500** |
| jobs/s por vCPU | 1,96 |

### Parte N — sizing recomendado

O gargalo é CPU e a memória sobra ~40×, então o dimensionamento é puramente de
CPU. Usando o **knee** (não o pico) e 75% de headroom:

| node | pods de 3 vCPU | jobs concorrentes | jobs/h | headroom |
|---|---|---|---|---|
| 16 vCPU / 64 GiB | 4 | 16 | ~84.500 | 25% CPU, ~97% RAM |
| 32 vCPU / 128 GiB | 8 | 32 | ~169.000 | 25% CPU, ~98% RAM |

**A RAM fica ociosa nessa configuração.** Para esta carga, nodes com melhor razão
CPU:RAM entregam mais por real gasto.

> Ressalva: o workload sintético custa ~4,7 MiB por página. O spike inicial com
> `web.whatsapp.com` custou **474–790 MB por sessão**. Com SPA real o gargalo
> **inverte para RAM** e esta tabela deixa de valer. **Não dimensionar produção
> com ela sem remedir no SPA de verdade.**

---

## Correções aplicadas durante o estudo

Registradas porque cada uma teria virado uma conclusão errada.

**1. `rod-default` × `rod-canon`.** A primeira repetição deu 874 ms contra 355 ms,
sugerindo que a emulação de device do rod custa 2,5×. As repetições 2 e 3 deram
346 e 355 ms — **idênticas ao canon**. Era outlier. A emulação de device do rod
**não custa throughput mensurável**.

**2. Ordem dos controllers.** Rotacionar a ordem entre repetições revelou que
`rod.Browser.Close()` emite `proto.BrowserClose` quando `BrowserContextID` está
vazio (`browser.go`) — **derrubando um browser externo compartilhado**. Com ordem
fixa e rod por último, isso teria passado despercebido.

**3. Robustez sob crash — conclusão retirada.** A Fase 1 reportou "sem vazamento
de goroutines ou FDs" e uma comparação favorável ao chromedp. **Ambas foram
retiradas**: o JSON nunca foi gravado (o container foi morto antes do fim), e a
repetição do mesmo cenário **não completou em 17,5 min** contra ~31 s na
primeira. O cenário tem variância alta e o injetor de falhas degrada a si mesmo
(cada `Page.crash` deixa uma chamada pendente por 30 s). **Robustez sob crash
não foi estabelecida.**

**4. `requestAnimationFrame` em target de background.** O marcador de readiness
nunca aparecia e travava todos os controllers numa página já renderizada. Bug do
workload, não dos controllers — targets CDP criados em background não produzem
frames.

**5. Alavanca de site isolation.** Reportada como ~200 MB no spike inicial; medida
em 13 MiB no workload sintético. A diferença é a diversidade de origens, não a
flag.

**6. Hipótese "mutex global no rod" — não confirmada.** Levantada na Fase 1 a
partir da leitura de `lib/cdp/websocket.go:26,134`. O experimento de ablação do
sleeper mostrou que a explicação era insuficiente, e nenhum profiling foi feito.
**Permanece HYPOTHESIS.**

---

## Respostas às perguntas da Fase 2

| # | pergunta | resposta |
|---|---|---|
| **Q1** | A vantagem do chromedp permanece em amd64 e concorrência alta? | **Sim** em amd64 até conc 32 (3,1–3,3×). Conc 64/128 **não testada** — em 3 vCPU o sistema satura em 8, então mediria enfileiramento, não capacidade. |
| **Q2** | Qual mecanismo degrada o rod sob concorrência? | **Não respondida.** A hipótese do sleeper foi testada e **descartada**. Falta profiling (mutex/block/goroutine/CPU). |
| **Q3** | O ganho de 1,8–2,3× do cdproto sobrevive à equivalência semântica? | **Não. O ganho não existe.** Com semântica igual o chromedp high-level venceu nas duas arquiteturas. |
| **Q4** | Qual topologia maximiza jobs/core-h e jobs/GiB-h? | **Parcial.** 1 browser : N páginas em 3 vCPU, operando no knee (conc 4): 7.044 jobs/core-h. Comparação browser-por-job × contexts **não executada**. |
| **Q5** | Há problema de longo prazo? | **Não respondida.** Nenhum soak executado. |

---

## Decisão arquitetural

### 1. Controller

**chromedp.** Motivos, em ordem de força:

- 3,1–3,3× de throughput sobre o rod sob concorrência em amd64 real (MEASURED);
- rod colapsa em confiabilidade sob carga — 18,8% de falhas em conc 32 (MEASURED);
- rod tem quatro mutações implícitas da superfície do browser, incluindo desligar
  o sandbox ao detectar container (SOURCE-CONFIRMED);
- rod derruba browser externo compartilhado ao fechar (SOURCE-CONFIRMED + MEASURED).

### 2. Hot path — quais operações justificam cdproto direto

**Nenhuma, com a evidência atual.** A recomendação da Fase 1 está **suspensa**.

Um fast path só entra se, para aquela primitiva específica, demonstrar:

1. ganho de throughput **com correção verificada** contra página hostil;
2. equivalência de garantias (visibilidade, retry, staleness, timeout);
3. contagem de round-trips medida, não estimada.

O breakdown por primitiva (Parte I) **não foi feito** — é o pré-requisito.

### 3. High-level como padrão

Todas as operações permanecem em chromedp high-level até que a regra acima seja
satisfeita, individualmente. Em especial `Click` e `SendKeys`, cujas versões
"rápidas" via `Runtime.evaluate` são **comprovadamente incorretas** em SPA com
componentes controlados e com overlay.

### 4. Topologia

**1 Chromium : N páginas**, com N no knee. Em 3 vCPU, **N = 4**. Comparação com
browser-por-job e com BrowserContexts **não foi medida** — recomendação
provisória.

### 5. Sizing

Ver Parte N. **CPU é o único recurso que importa** neste workload.

### 6. Menor custo por job concluído com sucesso

chromedp high-level, no knee: **7.044 jobs/core-hora, 0 falhas, 0 falsos
sucessos**. O L0 tem throughput nominal maior e **custo por job correto
infinito** — ele não produz jobs corretos.

### Arquitetura recomendada

```
Scheduler
   ↓
Worker Pod  (3 vCPU / 3 GiB, cgroup v2, priority baixa)
   ↓
chromedp — lifecycle E data-plane
   ↓
Internal Browser Abstraction
   ├── caminhos seguros: chromedp high-level          ← padrão
   └── fast paths cdproto: NENHUM habilitado          ← exige prova por primitiva
   ↓
1 Chromium : 4 páginas concorrentes  (knee medido)
   ↓
Chromium 151, CanonicalBrowserProfileV1 explícito
```

A diferença para a arquitetura candidata original é o segundo ramo: **o
data-plane cdproto não foi habilitado**, porque a evidência que o justificava não
sobreviveu à verificação.

---

## Grau de confiança por conclusão

| conclusão | grau | justificativa |
|---|---|---|
| O ganho de 1,8–2,3× do cdproto não é real | **HIGH** | medido nas duas arquiteturas + mecanismo (0% de correção) |
| L0 (`Runtime.evaluate`) é incorreto em SPA controlado e sob overlay | **HIGH** | medido + mecanismo SOURCE-CONFIRMED |
| RAM por sessão é launch policy, não biblioteca | **HIGH** | medido nos dois sentidos + fonte |
| `sum(RSS)` superestima memória em ~4× | **HIGH** | medido contra cgroup |
| Overhead do controller não é critério | **HIGH** | 13–27 MB contra 240 MiB de browser |
| chromedp > rod sob concorrência | **HIGH** | reproduzido em duas arquiteturas, 7 configurações |
| CPU do renderer é o recurso que satura | **HIGH** | cgroup: 23% throttling, `memory.events` zerado |
| knee em conc 4 / saturação em 8 (3 vCPU) | **HIGH** | curva de 6 pontos, 0 falhas |
| chromedp high-level > cdproto manual equivalente | **MEDIUM** | direção reproduzida; magnitude depende da qualidade do L1 |
| `rod.Browser.Close()` destrói browser externo | **HIGH** | 3/3 repetições + fonte |
| Emulação de device do rod não custa throughput | **MEDIUM** | corrigido por repetição, N=3 |
| Modelo de capacidade Kubernetes | **LOW** | workload sintético; SPA real inverte o gargalo |
| Mecanismo da degradação do rod | **HYPOTHESIS** | ablação descartou o sleeper; sem profiling |
| Robustez sob crash de renderer | **LOW** | conclusão retirada; cenário com variância alta |

---

## O que não foi medido

Declarado para que nenhuma tabela seja lida além do que sustenta.

- **Profiling causal** (CPU/heap/mutex/block/goroutine pprof, Go trace) — Q2 aberta.
- **Soak** de 1 h / 6 h / 24 h — Q5 aberta. Nenhum dado de leak, drift ou zumbi.
- **Concorrência 64 / 128 / 256** — em 3 vCPU o sistema satura em 8.
- **Topologia** browser-por-job × BrowserContexts (Parte K).
- **Fault domain** por topologia (Parte L) e restart/recovery (Parte Q).
- **Playwright-Go executado** (Parte T) — auditado na fonte apenas.
- **Breakdown por primitiva** (Parte I) — pré-requisito para qualquer fast path.
- **Workload W3 longo** (5–15 min com auth e navegações múltiplas).
- **SPA real.** Todos os workloads são sintéticos e de origem única. É a
  limitação mais importante desta lista.

---

## Reprodutibilidade

```bash
# ENV-A (local, container)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o study-linux-arm64 .
docker build -t chromium-study:v2 .
./run-matrix.sh
docker run --rm --cpus=4 --memory=4g -v "$PWD/results:/out" \
  chromium-study:v2 -mode levels -iters 40 -warmup 5 -out /out/levels.json
python3 analyze.py results

# ENV-B (Kubernetes)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o study-linux-amd64 .
kubectl --context=sbx apply -f k8s-bench.yaml
kubectl --context=sbx -n chromium-study cp study-linux-amd64 bench:/study
# ... apt install chromium; /entrypoint.sh -mode levels ...
kubectl --context=sbx delete namespace chromium-study
kubectl --context=sbx delete priorityclass bench-lowest
```

Variáveis: `CPUS`, `MEM`, `OUT`, `IMG`, `PROFILE=canonical|no-isolation`,
`OVERLAY=1` (ativa o teste de oclusão).

### Estado do cluster após o teste

Namespace e PriorityClass removidos, verificado por `kubectl get` retornando
`NotFound`. SBX com 31 pods, os mesmos do início. Três pods em estado anormal no
cluster são **pré-existentes e sem relação com o teste**:
`aulapratica-migrate` (criado 2026-06-05, `ImagePullBackOff` na tag
`:placeholder`) e dois pods do `filarapida` em crashloop desde 2026-06-06, com
18.045 restarts acumulados.

### Arquivos

```
results/           JSONs do ENV-A (arm64)
results-amd64/     JSONs do ENV-B (amd64/SBX) — 7 arquivos
METHODOLOGY.md     método, perfil canônico, limitações
RELATORIO-FASE-1-E-2.md   este documento
Dockerfile  entrypoint.sh  k8s-bench.yaml  run-matrix.sh  analyze.py
main.go  controllers.go  levels.go  levels_run.go  proxy.go  chaos.go
metrics/  workload/
```
