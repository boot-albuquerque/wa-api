# Relatório — Fase 3

Validação de produção da arquitetura `chromedp + Chromium`.
Data: 2026-08-09. Continuação de `RELATORIO-FASE-1-E-2.md`, mesmo diretório.

---

## 0. Respostas diretas

```text
Controller:                    chromedp (mantido; ver §1 — o motivo mudou)
Topologia:                     2 Chromium : contexto default : 1 page/slot : 4 slots
                               (T4; 1 conexão CDP por slot)
Knee na SPA real:              4 slots com 3 vCPU — colapso em 8
Recurso limitante:             CPU. memory.events = 0 em TODAS as concorrências
Successful jobs/core-hour:     1.039 (SPA real, conc 4)  /  8.730 (sintético, T4, 6 vCPU)
Successful jobs/GiB-hour:      8.268 (SPA real, conc 4)  /  53.438 (sintético, T4, 6 vCPU)
p99 no knee:                   8.419 ms (SPA real, conc 4)
False-success rate:            0/96 até conc 4  |  7/24 (29,2%) em conc 8
Soak 24h:                      NÃO EXECUTADO — 60 min executados (§6)
Recovery após browser crash:   NÃO MEDIDO — Track G não executado (§8)
Fast paths aprovados:          NENHUM — Track C não executado; política mantida
InteractionPolicy necessária:  SIM, com evidência parcial (§5) — Playwright não medido
```

**Decisão:** `GO WITH CONDITIONS`. Critérios e condições em §10.

---

## 1. Q1/D1 — A causa do teto do Rod

**Resposta: SIM, parcialmente demonstrada.** Duas causas identificadas e confirmadas
por ablação; uma terceira permanece aberta.

### 1.1 A primeira hipótese foi refutada pelo próprio braço de controle

A primeira hipótese testada foi topologia de conexão. Ela nasceu de um achado no
código do chromedp, não de um palpite: `NewContext` herda o `*Browser` do pai
(`chromedp.go:129`), mas um contexto derivado direto de um `RemoteAllocator` não
tem nenhum — e `RemoteAllocator.Allocate` disca o websocket a cada vez
(`allocate.go:587`). Ou seja, **as Fases 1 e 2 compararam chromedp com uma conexão
CDP POR JOB contra rod com uma conexão para todos os jobs.** Isso é diferença de
topologia, não de biblioteca.

Dois braços foram construídos para testar a hipótese nas duas direções:

| braço | conc 4, 80 jobs, 8 CPU | previsão | resultado |
|---|---|---|---|
| `chromedp` (1 conexão/job) | 14,44–20,42 j/s | rápido | rápido ✔ |
| `chromedp-shared-conn` (1 conexão total) | 19,82–20,70 j/s | **cair** | **não caiu** ✘ |
| `rod-canon` (1 conexão total) | 3,87–3,99 j/s | lento | lento ✔ |
| `rod-multiconn-4` (1 conexão/slot) | 0,81–3,18 j/s | **subir** | **não subiu** ✘ |

As duas previsões falharam. **Hipótese de conexão: REFUTADA** (MEASURED, HIGH — as
duas direções foram testadas e ambas contradizem).

Registro de honestidade: um smoke inicial (20 jobs, 6 CPU) mostrou
`chromedp-shared-conn` a 1,06 j/s e teria "confirmado" a hipótese. Com 80 jobs e
2 repetições o número virou ~20 j/s. O smoke era artefato de N pequeno e container
frio. Se a Fase 3 tivesse parado no smoke, teria publicado uma causa falsa.

### 1.2 "Mutex global": refutado com número

| perfil (conc 8, 80 jobs) | bloqueio acumulado | delay de mutex |
|---|---|---|
| `rod-canon` | **960,4 s** | **0,93 s** (0,097%) |
| `chromedp` | 187,9 s | 0,43 s (0,23%) |

O mutex responde por menos de 0,1% do tempo bloqueado do rod. **Não é o mutex**
(MEASURED, HIGH).

### 1.3 A causa real: `WaitStableRAF` dentro de cada interação

O block profile aponta para onde o rod realmente espera:

```text
rod.(*Element).Input      137,05 s
rod.(*Element).Focus      136,62 s
  └─ 99,91% em ScrollIntoView   136,49 s
```

Lendo a fonte a partir do perfil (não antes dele):

```go
// element.go:72
func (el *Element) ScrollIntoView() error { err := el.WaitStableRAF(); ... }

// page.go:851
func (p *Page) WaitRepaint() error {
    _, err := p.root.Eval(`() => new Promise(r => requestAnimationFrame(r))`)
}
```

Todo `Input()` e todo `Click()` do rod esperam, **dentro do renderer**, por pelo
menos dois animation frames. A Fase 1 já havia medido que `requestAnimationFrame`
não é entregue com cadência garantida em target CDP em background — foi por isso
que o marcador de readiness teve de ser reescrito de forma síncrona.

Isso prevê exatamente a forma observada: p50 saudável, p99 catastrófico. E explica
por que as duas correções anteriores falharam: mais conexões não aceleram uma
espera que acontece no renderer, e um Sleeper mais rápido também não, porque isso
é uma Promise e não um laço de polling.

### 1.4 Ablação — remove-se só a espera de repaint

`rod-noraf` mantém rod, sua conexão e sua resolução de elementos. Troca apenas o
`WaitStableRAF` pelo que o chromedp faz no mesmo ponto: `DOM.scrollIntoViewIfNeeded`
seguido de eventos reais no centroide medido. **A semântica é preservada** — os
eventos continuam passando por hit-test, então um overlay continua interceptando.

| braço (conc 4, 8 CPU, 80 jobs) | jobs/s | p50 | p99 | bloqueio total |
|---|---|---|---|---|
| `rod-canon` | 3,87–3,99 | 350–352 ms | **19.751–20.234 ms** | 960,4 s |
| `rod-noraf` | 4,36–6,09 | 285–401 ms | **5.371–5.896 ms** | 317,3 s |
| `rod-noraf-fastpoll` | 9,29–10,82 | 161–189 ms | 5.251–5.360 ms | — |
| `chromedp` | 14,44–20,42 | 190–264 ms | 263–467 ms | 187,9 s |

Efeito da ablação: **p99 cai 3,6×** (20,2 s → 5,6 s) e o bloqueio total cai 3,0×.
Somando a política de Sleeper, throughput sobe **2,4–2,8×** (3,9 → 9,3–10,8 j/s).

**Conclusão D1:** duas causas confirmadas por ablação —
(a) `WaitStableRAF` (rAF em target background) e (b) `BackoffSleeper(100ms, 1s)` —
com efeito conjunto de ~2,7× em throughput e 3,6× em p99 (MEASURED, HIGH).
Um resíduo permanece: mesmo corrigido, o rod fica ~2× abaixo do chromedp e mantém
p99 de ~5,3 s. O perfil residual concentra-se em `Browser.Event.func1` (54,8 s),
o fan-out de eventos do rod, bloqueado em envio de canal. **Isso é HYPOTHESIS,
LOW** — não foi ablacionado, e não deve ser citado como causa.

**Isso muda a decisão de controller?** Não. Mas muda o *motivo*: o rod não é
"arquiteturalmente mais lento", ele tem duas políticas de espera caras e
reconfiguráveis. A vantagem do chromedp que sobrevive é ~2× com defaults, não os
3–3,5× que as Fases 1–2 reportaram.

---

## 2. Q2/D2 — Topologia

Política mantida constante: **uma conexão CDP por slot concorrente**, em todas as
topologias. Fixá-la é o que impede este track de re-medir o efeito do Track A e
rotulá-lo como "contextos".

Workload: página hostil sintética com ground truth. 48 jobs por topologia,
4 slots, arm64.

### 6 vCPU

| topologia | corretos | falsos | corr/s | p50 | p99 | CPU | mem | rend | **/core-h** | **/GiB-h** |
|---|---|---|---|---|---|---|---|---|---|---|
| T1 — 1 browser, 1 slot | 48 | 0 | 4,25 | 204 | 535 | 17,6 s | 289 MB | 3 | **9.825** | 54.151 |
| T2 — 1 browser, 4 slots | 44 | 0 | 6,95 | 508 | 886 | 20,3 s | 326 MB | 6 | 7.806 | **78.441** |
| **T4 — 2 browsers, 4 slots** | **48** | **0** | **10,25** | **334** | **736** | 19,8 s | 707 MB | 8 | 8.730 | 53.438 |
| T5 — 4 browsers, 4 slots | 48 | 0 | 8,77 | 425 | 799 | 25,1 s | 1.159 MB | 12 | 6.895 | 27.891 |
| T3 — 1 browser, 4 contextos | 38 | **9** | 0,71 | 3.644 | 11.400 | **236,0 s** | 844 MB | 7 | 580 | 3.121 |
| T6 — 2 browsers, 2 ctx cada | 29 | **17** | 0,37 | 1.550 | 3.230 | 131,9 s | 865 MB | 9 | 791 | 1.593 |

### O resultado mais forte: BrowserContext é caro E incorreto aqui

T3 e T6 não são apenas lentos — consomem **7–12× mais CPU** e produzem
**9 e 17 falsos sucessos em 48 jobs**. Nas topologias sem contexto explícito, o
falso-sucesso foi zero.

Isso é OBSERVED, MEDIUM: o custo está medido e é grande, mas o mecanismo do
falso-sucesso não foi isolado. A explicação plausível — não confirmada — é que
cada BrowserContext instancia targets `browser_ui` próprios (visíveis no trace:
"Omnibox Popup" por contexto), o que multiplica renderers e, sob starvation, faz a
página perder a corrida contra a substituição de nó da página hostil.

**Não use BrowserContext por sessão sem repetir esta medição.** É o oposto do que
a intuição de isolamento sugere.

### Escolha

**T4 — 2 Chromium, contexto default, 4 slots, 1 page por slot.** Vence em
throughput (10,25 corr/s, +47% sobre T2) e em p99 (736 ms) com correção perfeita.

Trade-off explícito: T2 entrega **47% mais jobs por GiB-hora** (78.441 vs 53.438).
Se a restrição do cluster for memória e não CPU, **T2 é a escolha certa** — e a
Fase 3 não tem dado para decidir isso sem saber o perfil do nó de produção.

T1 (serial) tem o melhor jobs/core-hour absoluto (9.825): concorrência custa
eficiência por core mesmo quando aumenta o throughput.

### Escalonamento com CPU

3 vCPU vs 6 vCPU, mesma matriz: T3/T6 sequer completaram em 3 vCPU. O knee do
workload sintético permaneceu em 4 slots nos dois casos. **12 vCPU não foi testado**
— o maior nó disponível (sbx/plt/tbr) tem 6–8 cores físicos compartilhados com as
aplicações reais da empresa; pedir 12 vCPU não era possível sem impactar produção.

---

## 3. Q5/D7 — A SPA real

**Alvo:** `https://filarapida.sbx.decolapps.com/` — Next.js/React da própria
empresa, ambiente sandbox. SSR + hidratação, code splitting, 3 origens
(`filarapida.sbx.decolapps.com`, `cdn.prod.website-files.com`, `i.pravatar.cc`),
~27 recursos e ~1.015 nós de DOM por job.

**Ground truth sem credenciais e sem escrita:** o botão de submit carrega
`disabled=""` e só é habilitado pelo React quando o campo de código é válido.

```text
atribuir input.value   -> React não re-renderiza -> botão continua disabled
eventos de tecla reais -> React commita          -> botão habilita
```

O fluxo digita um código inexistente e lê a resposta de "não encontrado".
**Nada é criado, alterado ou apagado.** Nenhuma credencial foi usada.

### Resultados (container 3 vCPU / 3 GiB, 24 jobs por ponto)

| conc | corretos | **falsos** | corr/s | p50 | p99 | CPU | **throttling** | mem | /slot | rend | /core-h | /GiB-h |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 24 | 0 | 0,34 | 2.826 | 6.519 | 112,3 s | 7,1% | 314 MB | 64 MB | 4 | 769 | 3.981 |
| 2 | 24 | 0 | 0,65 | 3.593 | 4.215 | 85,1 s | 35,2% | 325 MB | 38 MB | 5 | 1.016 | 7.389 |
| **4** | **24** | **0** | **0,80** | 4.366 | **8.419** | 83,2 s | 79,3% | 358 MB | 27 MB | 7 | **1.039** | **8.268** |
| 8 | 17 | **7 (29,2%)** | 0,13 | **47.943** | 78.164 | 338,3 s | 69,3% | 492 MB | 44 MB | 14 | 181 | 947 |

`memory.events` = `{low:0, high:0, max:0, oom:0, oom_kill:0}` em **todos** os pontos.
Pico de memória: 795 MB de um limite de 3 GiB.

### D7 — CPU ou RAM satura primeiro?

**CPU, sem ambiguidade.** Throttling do CFS sobe de 7,1% para 79,3% entre conc 1 e
4, enquanto a memória nunca encosta no teto. O knee na SPA real é **4 com 3 vCPU** —
o mesmo do sintético — mas o comportamento **após** o knee é qualitativamente
diferente e é isso que importa:

> No sintético, passar do knee degradava latência.
> Na SPA real, passar do knee **produz resultados errados**.

Em conc 8, 7 de 24 jobs passaram no controller e falharam na verdade da aplicação.
A contagem média de nós de DOM caiu de 1.015 para **752**: sob starvation de CPU a
aplicação não termina de renderizar, e o controller "tem sucesso" contra uma página
incompleta. Esse é o mecanismo do falso sucesso, e ele é induzido por saturação —
não por um seletor mal escrito.

**Consequência de dimensionamento: o headroom de CPU não é conforto, é correção.**

### Confound declarado

O tier web do alvo tem limite de 500m CPU. Medido com `kubectl top` durante a
corrida de conc 4: **290m de 500m (58%)**. Não saturou em conc 4, mas em conc 8 é
provável que tenha throttle, e parte da degradação de conc 8 pode ser do servidor,
não do browser. Os números de conc 8 devem ser lidos como "além do knee o sistema
como um todo colapsa", não como uma medida limpa do cliente.

### §16 — Site isolation na SPA real

Mesmo perfil, mesma concorrência (2), mesmo alvo. A única diferença são as duas
flags de site isolation.

| | corretos | corr/s | p50 | p99 | **CPU** | mem | pico | rend | /core-h |
|---|---|---|---|---|---|---|---|---|---|
| isolation OFF (canônico) | 24 | **0,65** | 3.593 | 4.215 | **85,1 s** | 325 MB | 398 MB | 5 | **1.016** |
| isolation ON | 24 | 0,21 | 9.310 | 17.386 | **244,1 s** | 352 MB | 404 MB | 5 | 354 |

**O custo do site isolation nesta SPA é CPU, não memória.** 2,9× mais CPU e 2,6×
mais latência p50, contra apenas **+27 MB (+8%)** de memória e a **mesma contagem
de renderers**.

Isso contraria o enquadramento usual ("site isolation custa RAM") e é consistente
com a Fase 2, que mediu 13 MiB no piso sintético — a diferença de memória depende
da diversidade de origens (aqui, 3), e o custo real aparece em CPU.

**Trade-off de segurança, explicitado:** desligar site isolation remove a barreira
de processo entre origens. Numa automação que carrega apenas a aplicação alvo e
suas CDNs conhecidas, o modelo de ameaça é diferente do de um navegador de usuário.
Mas o custo medido — 2,9× CPU — é grande o bastante para que a decisão seja
econômica de verdade, e não uma otimização de rotina. Ela deve ser registrada como
redução de segurança deliberada, com o número ao lado.

---

## 4. Q3/D5 — Soak

**24h: NÃO EXECUTADO. 6h: NÃO EXECUTADO. 60 min: executado.** Resultado em §11.

Não extrapole 60 min para 24h. A Fase 3 não estabelece estabilidade de 24 horas.

---

## 5. D4 — Actionability e InteractionPolicy

O Track D completo (suíte hostil de 15 cenários + baseline Playwright-Go) **não foi
executado**. O que existe é evidência parcial, mas convergente, de três fontes
independentes desta fase:

1. **Falso sucesso por saturação (§3).** 29,2% em conc 8 na SPA real. Nenhuma
   biblioteca detecta isso: a página respondeu, o elemento existia, o clique
   aconteceu. Só a verificação contra o estado da aplicação pegou.
2. **`chromedp.Nodes(sel, ByQueryAll, NodeVisible)` não significa "os visíveis".**
   Medido: a página tinha 4 âncoras `/como-funciona` renderizadas (456×20 px,
   confirmado com `mode=linkprobe`) e a query nunca satisfez, porque o chromedp
   aplica a espera ao conjunto inteiro e havia cópias permanentemente ocultas no
   menu mobile. O sintoma foi um timeout de 53 s, não um erro de seletor.
   Antes disso, `chromedp.Click(sel)` pegou a primeira ocorrência — oculta — e
   esperou o deadline inteiro.
3. **`WithNewBrowserContext()` está quebrado neste Chromium** (§7).

Nenhum desses três é resolvido por "escolher a biblioteca certa". Os três são
política de interação: escolher entre N candidatos, verificar renderização real
antes de agir, e verificar o efeito depois de agir.

**D4: SIM, uma InteractionPolicy própria é necessária** (INFERRED, MEDIUM).
O custo adicional do Playwright **não foi medido**, então a alternativa "comprar
robustez do Playwright em ações críticas" continua aberta e não deve ser descartada
com base nesta fase.

---

## 6. D3 — Fast paths cdproto

Track C (breakdown por primitiva) **não foi executado**. Nenhuma primitiva foi
avaliada individualmente, portanto **nenhum fast path é autorizado**.

Política mantida: **100% chromedp high-level**. Isso é um resultado válido, e é o
mesmo da Fase 2 — só que agora por ausência de medição, não por refutação.

---

## 7. Defeitos encontrados nas ferramentas

### 7.1 `chromedp.WithNewBrowserContext()` falha no Chromium 151

```text
Failed to open new tab - no browser is open (-32000)
```

Isolado com trace de wire (`mode=ctxprobe`):

- os mesmos dois comandos replicados num websocket cru **funcionam**, incluindo
  todos os campos opcionais explicitamente `false` que o chromedp serializa;
- continuam funcionando **depois** que o chromedp conectou — não é que o chromedp
  deixe o browser num estado ruim;
- mas o mesmo comando emitido **através da conexão de browser do chromedp** falha.

O fator discriminante não foi identificado. O que está estabelecido é que
**isolamento por BrowserContext não é alcançável pela API do chromedp neste
browser** — quem depender disso precisa possuir esse caminho de código.
(MEASURED, HIGH quanto ao fato; a causa é UNKNOWN.)

Contorno adotado no Track B: ciclo de vida de contexto/target numa conexão CDP de
controle dedicada por browser, com o chromedp anexando por `targetID`. Isso não é
um remendo — é a forma que uma runtime de produção deveria ter de qualquer modo:
a política de lançamento é dona dos contextos, a biblioteca de driving é dona das
páginas.

### 7.2 `chromedp.Nodes(..., ByQueryAll, NodeVisible)`

Ver §5, item 2. Não é bug — é semântica documentada mal alinhada com a intuição, e
o modo de falha é um timeout longo, não um erro.

### 7.3 Achados incidentais na infra sandbox (fora do escopo)

Registrados aqui porque foram encontrados de lado, não porque façam parte do estudo:

- `filarapida-web` tem **645 restarts** (último há 2d14h) com limite de 512Mi/500m.
- `filarapida-admin-b5d75dd78-dkqc8` em **CrashLoopBackOff há 120 dias**
  (23.987 restarts) e `filarapida-api-86645bb78b-z7scs` em Error (18.052 restarts) —
  ReplicaSets antigos que nunca foram removidos.
- A home de sbx renderiza links de auth apontando para **`http://localhost:3001/auth/sign-in`**
  (13 ocorrências) — configuração de ambiente vazando para o HTML servido.

Nenhum foi corrigido. Não são do repositório `wa-api`.

---

## 8. O que NÃO foi executado

Declarado explicitamente, como a Fase 1 e 2 exigiram:

| item | status |
|---|---|
| Track C — breakdown por primitiva | **não executado** |
| Track D — suíte hostil de 15 cenários | **não executado** |
| Track D — baseline Playwright-Go | **não executado** |
| Track G — fault domain / recovery / blast radius | **não executado** |
| Track H — custo econômico além de core-h e GiB-h | **não executado** |
| Soak 6h e 24h | **não executado** (60 min executados) |
| Escalonamento a 12 vCPU | **impossível** — nó máximo disponível: 8 cores compartilhados |
| Fluxo de escrita com verificação de estado externo (§14 do pedido) | **não executado** — sem credenciais; o fluxo real é read-only |
| Idempotência / `operation_id` / exactly-once percebido | **não executado** |
| Concorrência 64/128/256 | não executado (e desaconselhado sem mais CPU) |
| Topologia com contexto COMPARTILHADO entre slots | não executado (bloqueado por §7.1) |

As três que o pedido mandou não sacrificar eram **SPA real, topologia e soak**.
SPA real e topologia foram executados. Soak foi executado em 60 min, não em 24h.

---

## 9. Arquitetura atualizada

```text
Scheduler
   ↓
Durable Queue
   ↓
Worker Pod  (3 vCPU / 3 GiB medidos; CPU é o recurso limitante)
   ↓
chromedp  (high-level, sem fast path cdproto)
   ↓
Internal Browser Runtime
   ├── LaunchPolicy        CanonicalBrowserProfileV1, explícita, 27 flags
   ├── ConnectionPolicy    1 websocket CDP por slot concorrente
   ├── WaitPolicy          sem rAF-stability; timeout obrigatório por job
   ├── InteractionPolicy   NECESSÁRIA — não implementada, não medida
   ├── RecoveryPolicy      NÃO MEDIDA
   └── FastPathPolicy      vazia (nenhuma primitiva aprovada)
   ↓
N = 2 Chromium
   ↓
M = contexto default (NÃO usar BrowserContext por sessão — §2)
   ↓
K = 2 pages por browser  (4 slots no total, com 3–6 vCPU)
   ↓
SPA real
```

Valores medidos: **N=2, M=default, K=2** (total 4 slots concorrentes).
Com restrição de memória em vez de CPU: **N=1, K=4** (T2, +47% jobs/GiB-hora).

---

## 10. Decisão: GO WITH CONDITIONS

**Base para o GO:**

- controller decidido, com mecanismo do concorrente agora explicado e ablacionado (§1);
- topologia medida com ground truth e sem falso sucesso até o knee (§2, §3);
- CPU identificada como recurso limitante na SPA **real**, com `memory.events`
  zerado em todas as concorrências — o modelo de capacidade deixa de ser LOW e
  passa a MEDIUM para SPAs deste peso (§3);
- knee reproduzido em dois workloads independentes (sintético e real) e em duas
  quantidades de CPU.

**Condições, todas bloqueantes para produção:**

1. **Headroom de CPU é requisito de correção, não de conforto.** Dimensionar no
   knee (4 slots / 3 vCPU) ou abaixo. Em conc 8 a taxa de falso sucesso foi 29,2%.
2. **Todo job crítico precisa de verificação contra o estado da aplicação.**
   Sem isso, 7 dos 24 jobs de conc 8 teriam sido contabilizados como sucesso.
3. **Executar o Track G antes do primeiro deploy.** Não há nenhuma medição de
   recovery, blast radius ou processos órfãos nesta fase. `RecoveryPolicy` é hoje
   uma caixa vazia no diagrama.
4. **Executar soak de 24h antes de tráfego sustentado.** 60 min não estabelecem
   estabilidade de um dia.
5. **Não usar BrowserContext por sessão** sem repetir a medição de §2.
6. **Construir a InteractionPolicy** ou medir o Playwright como alternativa (§5).
7. **Revalidar tudo contra o WhatsApp Web.** O spike inicial mediu 474–790 MB por
   sessão; a SPA medida aqui consome 27–64 MB por slot acima do piso do browser —
   **uma ordem de grandeza de diferença.** O gargalo pode inverter de CPU para RAM
   nesse workload, e aí a topologia vencedora muda.

---

## 11. Reprodutibilidade

Ambiente: Docker Desktop 27.3.1 no macOS, VM Linux 6.10.11-linuxkit aarch64,
10 CPUs / 6,2 GB. Chromium 151.0.7922.108 (Debian bookworm), Go 1.26.
cgroup v2 para toda a contabilidade de memória e CPU.

```bash
docker build -t chromium-study:p3 .

# Track A — ablação (por braço, container próprio)
./run-trackA.sh                                   # ITERS/CONC/CPUS por env

# Track B — topologia
docker run --rm -e SKIP_BROWSER=1 --cpus=6 --memory=5g -v $PWD/results-p3:/out \
  chromium-study:p3 -mode topology -concurrency 4 -iters 48 -warmup 3 \
  -out /out/trackB/topo-cpu6.json

# Track E — SPA real
docker run --rm -e SKIP_BROWSER=1 --cpus=3 --memory=3g -v $PWD/results-p3:/out \
  chromium-study:p3 -mode spa -spa-url https://filarapida.sbx.decolapps.com/ \
  -concurrency 4 -iters 24 -warmup 2 -out /out/trackE/spa-c4.json

# Track F — soak
docker run --rm -e SKIP_BROWSER=1 --cpus=3 --memory=3g -v $PWD/results-p3:/out \
  chromium-study:p3 -mode soak -spa-url https://filarapida.sbx.decolapps.com/ \
  -concurrency 2 -browsers 1 -soak 60m -out /out/trackF/soak-60m.json

# Defeito do BrowserContext
docker run --rm -e CDP_TRACE=1 chromium-study:p3 -mode ctxprobe
```

JSONs em `results-p3/{trackA,trackA-recheck,trackB,trackE,trackF}/`.
Perfis pprof em `results-p3/trackA*/pprof-*/` (`cpu|heap|allocs|block|mutex|goroutine.pb.gz`).

---

## 12. Seção pronta para ADR

### Context

O wa-headless precisa dirigir uma SPA real em Kubernetes com corretude verificável.
As Fases 1 e 2 escolheram chromedp provisoriamente e deixaram cinco lacunas:
mecanismo do teto do rod, topologia, soak, fault domain e workload real.

### Decision

- Controller: **chromedp high-level**, sem fast path cdproto.
- Conexão: **um websocket CDP por slot concorrente**.
- Topologia: **2 Chromium × 2 pages** (4 slots) com 3–6 vCPU; **1 × 4** se a
  restrição for memória.
- **Contexto default. Não usar BrowserContext por sessão.**
- Dimensionar **no knee**, nunca acima.
- Todo job crítico verifica o estado da aplicação, não o retorno do controller.

### Evidence

`RELATORIO-FASE-3.md` §1–§3. Medições em `results-p3/`, perfis pprof inclusos.

### Rejected alternatives

- **rod** — 2× mais lento mesmo após ablação das duas políticas de espera caras;
  defaults custam 5× em p99. Não rejeitado por arquitetura: por política de espera.
- **cdproto como data-plane** — refutado na Fase 2 (fazia menos trabalho); não
  reavaliado por primitiva nesta fase.
- **BrowserContext por sessão** — 7–12× CPU e falso sucesso em 19–35% dos jobs.
- **`Runtime.evaluate` para simular interação** — falso sucesso por construção.

### Trade-offs

- T4 dá +47% de throughput sobre T2 e custa +117% de memória.
- Concorrência aumenta throughput e **reduz** jobs por core-hora (T1: 9.825; T4: 8.730).
- Headroom de CPU compra corretude, não latência.

### Known risks

- Recovery e blast radius **não medidos**.
- Estabilidade além de 60 min **não medida**.
- `WithNewBrowserContext()` quebrado no Chromium 151 — o caminho de isolamento é
  código próprio.
- O modelo de capacidade vem de uma SPA de ~1.015 nós e 3 origens. O WhatsApp Web
  mediu 474–790 MB/sessão no spike: **uma ordem de grandeza acima**.

### Operational limits

```text
3 vCPU / 3 GiB  ->  4 slots concorrentes, p99 ~8,4 s, 1.039 jobs corretos/core-hora
                    nunca 8 slots: 29,2% de falso sucesso
```

### Revisit triggers

- mudança de major do Chromium (o defeito de §7.1 é específico da versão);
- mudança de comportamento major do chromedp;
- `memory.events` deixando de ser zero, ou throttling < 20% no knee — sinal de que
  o workload virou memory-bound;
- taxa de falso sucesso > 0 em qualquer concorrência ≤ knee;
- p99 > 10 s no knee;
- SPA nova com perfil diferente — em especial > 5 origens ou > 3.000 nós de DOM;
- overhead do Playwright caindo materialmente;
- correção do `WithNewBrowserContext()` upstream, que reabre a topologia T3/T6.
