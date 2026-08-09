# Relatório — Fase 4

Fechamento da arquitetura de produção. Continuação de `RELATORIO-FASE-3.md`.
Data: 2026-08-09.

> **ESTADO: PARCIAL.** Track J (WhatsApp Web) está preparado e validado até o
> passo imediatamente anterior ao pareamento por QR, aguardando o operador.
> Tracks D, E, F, I e K não foram executados. §7 lista tudo.

---

## 0. Respostas diretas

```text
Controller:                    chromedp
ConnectionPolicy:              SHARED_PER_BROWSER — por simplicidade, NÃO por medição (§1)
InteractionPolicy:             NÃO IMPLEMENTADA — Track E não executado
BrowserContext policy:         PROIBIDO — agora com mecanismo medido (§2)
Production topology:           2 Chromium : contexto default : 2 pages : 4 slots (Fase 3, mantida)
CPU headroom:                  NÃO DETERMINADO — Track D não executado
RAM headroom:                  pico 795 MB em limite de 3 GiB na SPA de referência
Site Isolation:                pendente de replicação (N=1 até agora)
Target workload knee:          PENDENTE — Track J aguardando pareamento
Successful jobs/core-hour:     1.039 (SPA de referência, conc 4)
Successful jobs/GiB-hour:      8.268 (SPA de referência, conc 4)
Cost/1000 successful jobs:     NÃO CALCULADO — depende do Track J
p99:                           8.419 ms no knee da SPA de referência
False-success rate:            0 até o knee; 29,2% em 2× o knee
24h soak:                      NOT EXECUTED — operational constraint
                               SHORT SOAK de 60 min executado (§3)
Browser crash blast radius:    NÃO MEDIDO — Track I não executado
Recovery time:                 NÃO MEDIDO — Track I não executado
```

```text
FINAL DECISION: GO WITH CONDITIONS
```

Bloqueadores em §8.

---

## 1. Q1 — ConnectionPolicy

15 corridas (5 por política), 2 Chromium, 4 slots, ordem rotacionada a cada
repetição, workload hostil com ground truth.

| política | conexões | mediana corr/s | faixa observada | p99 med | CPU med | bytes CDP/job |
|---|---|---|---|---|---|---|
| C1 SHARED_PER_BROWSER | 2 | 6,34 | **0,48 – 22,49** | 672 ms | 16,5 s | 47,4 KB |
| C2 PER_SLOT | 4 | 10,55 | **0,50 – 23,38** | 761 ms | 12,5 s | 48,7 KB |
| C3 HYBRID | 4 | 10,99 | 6,70 – 21,60 | 684 ms | 13,8 s | 47,0 KB |

**A variância intra-política é maior que a diferença inter-política.** Nenhum
vencedor é distinguível com n=5 nessa dispersão.

Causa da dispersão, identificada e não especulada: a máquina de teste estava com
load average **20,37 em 10 CPUs**, com serviços de longa duração do próprio
operador (zitadel, quatro postgres, rabbitmq, minio, lobby, dois containers de
aplicação) rodando durante toda a janela. O ambiente nunca esteve limpo.

Dois fatos sobrevivem:

1. Na única repetição não contendida (rep 5), as três políticas ficaram **dentro
   de 4%**: 22,49 / 23,38 / 21,60 corr/s.
2. O tráfego CDP por job é **idêntico** entre políticas (~47 KB). A política de
   conexão não altera o protocolo, apenas quem carrega os bytes.

Aplicando o critério de decisão definido para este track — preferir a mais
simples quando a diferença não for significativa:

```text
ConnectionPolicy: SHARED_PER_BROWSER
Evidence:         rep não-contendida com as 3 dentro de 4%; bytes CDP/job iguais;
                  variância intra >> inter em 15 corridas
Confidence:       LOW — exige re-run em host dedicado antes de virar ADR
```

Isto **corrige** a Fase 3, que registrou `PER_SLOT` como política. Aquele registro
não estava sustentado pelos próprios dados da Fase 3, cujos intervalos também se
sobrepunham.

### Achado incidental (HYPOTHESIS, LOW)

As três corridas catastróficas (0,48 / 0,50 / 0,50 corr/s) tiveram **170–294 KB de
CDP por job** contra ~47 KB nas normais, e 11–20 mil alocações por job contra
~2.900. Ocorreram em C1 duas vezes e em C2 uma vez — não é efeito de política.
Hipótese não testada: o `retryInterval` de 5 ms do chromedp amplifica tráfego CDP
quando a CPU está contendida, transformando contenção em tempestade de polling.
Se confirmada, é um risco operacional real sob saturação — exatamente o regime em
que a Fase 3 mediu 29,2% de falso sucesso.

---

## 2. Q2 — BrowserContext: caro de verdade, e o motivo não é o chromedp

A Fase 3 concluiu "não use BrowserContext" a partir de T3/T6, mas aquela medição
confundia três coisas: o contexto do Chromium, o defeito do chromedp e o caminho
de contorno usado. Refeito **inteiramente por CDP cru, sem chromedp em lugar
nenhum**, passo a passo:

| passo | targets | renderers | memória | CPU acum. |
|---|---|---|---|---|
| baseline | 3 | 2 | 392 MB | 0,2 s |
| +4 BrowserContexts (vazios) | 3 | 2 | 395 MB | 0,5 s |
| +1 página em cada contexto | 15 | 10 | 960 MB | 7,7 s |
| tudo descartado | 3 | 2 | 411 MB | 8,1 s |
| +4 páginas no contexto **default** | 7 | 6 | 453 MB | 10,6 s |

**Criar um BrowserContext custa essencialmente nada** (+3 MB, 0 targets, 0
processos). O custo aparece na primeira página dentro dele:

| | por página em contexto próprio | por página no default |
|---|---|---|
| targets | +3 (1 page + **2 `browser_ui`**) | +1 |
| renderers | **+2** | +1 |
| memória | **~142 MB** | ~10 MB |
| CPU | ~2,0 s | ~0,4 s |

Os `browser_ui` são os "Omnibox Popup" que apareceram no trace da Fase 3: o
Chromium instancia UI própria por contexto, e ela é renderer de verdade.

```text
BrowserContext policy: PROIBIDO por sessão
Evidence:              ~14× memória e ~5× CPU por página, medido em CDP cru
Confidence:            HIGH — chromedp fora do caminho, controle no mesmo
                       comando e na mesma conexão
```

A conclusão da Fase 3 se mantém; o **mecanismo** é novo, e a atribuição estava
errada. O defeito do `WithNewBrowserContext()` deixou de importar para a
arquitetura: não usaremos contextos de qualquer forma.

### §6 — reprodutor mínimo do defeito

Testado em conexões cruas e novas, uma por caso, o setup que o chromedp faz antes
do comando: `setDiscoverTargets`, `setAutoAttach(flatten:true)` e
`setAutoAttach(flatten:false)` — este último o próprio Chromium rejeita
("Only flatten protocol is supported with browser level auto-attach").
**Nenhum reproduz a falha.**

Estado final: frames idênticos, conexão nova, mesma ordem — cru funciona, via
conexão do chromedp falha com `-32000 Failed to open new tab - no browser is
open`. Defeito **reproduzível, causa desconhecida**, não bloqueante.

---

## 3. Q5 parcial — SHORT SOAK de 60 minutos

> **Rótulo obrigatório: SHORT SOAK. Não equivalente a 24h.**
> O soak de 24h **não foi executado** — restrição operacional do operador.

SPA de referência, 2 slots, 1 browser, 3 vCPU. 3.577 jobs, 1 falha, 1 falso
sucesso, 240 amostras a cada 15 s.

| série | resultado | classificação |
|---|---|---|
| goroutines | 24 → 24 (+0,10/h) | stable |
| file descriptors | 11 → 11 (−0,00/h) | stable |
| renderers | 6 → 6 (−0,07/h) | stable |
| Go heap | +1,04 MB/h | stable |
| `memory.current` | +93,6 MB/h (ajuste linear) | **ver abaixo** |
| `memory.events` | tudo zero | — |

O `+93,6 MB/h` é enganoso e não deve ser citado como vazamento. A decomposição do
`memory.stat`:

```text
anon   −4,9 MB/h     239–250 MB, estável
file   +74,4 MB/h    38 -> 165 MB nos primeiros 15 min, depois 165 -> 158 -> 160
```

Todo o crescimento é **page cache, e ele satura**. Ajustar uma reta a uma curva
que satura produz a inclinação positiva. A memória anônima — a que vazaria — está
estável ou em leve queda.

```text
Classificação: bounded growth na janela de 60 min, sem sinal de leak
Confidence:    MEDIUM para 60 min | NENHUMA para 24h
```

**Ressalva:** o throughput SUBIU durante a corrida (0,50 → 1,28 jobs/s) e o p50
caiu (3.112 → 1.333 ms). Isso não é a arquitetura melhorando: havia
testcontainers ativos no início da janela. O primeiro quarto está contaminado e
**drift de latência não é mensurável nesta corrida**.

---

## 4. Q3 — Pareto da topologia

Não refeita nesta fase. A fronteira da Fase 3 permanece, com a ressalva de que
T3/T6 (contextos) agora estão **eliminadas por mecanismo**, não só por medição:

| perfil | topologia | métrica que otimiza |
|---|---|---|
| PERFORMANCE | 2 browsers × 2 slots | 10,25 corr/s, p99 736 ms |
| EFFICIENCY | 1 browser × 1 slot | 9.825 jobs/core-hora |
| MEMORY-DENSE | 1 browser × 4 slots | 78.441 jobs/GiB-hora |

`blast radius` e `recovery cost` continuam **ausentes** das três linhas, porque o
Track I não foi executado. A fronteira está, portanto, **incompleta**: duas das
dimensões que o próprio critério de dominância exige não foram medidas.

---

## 5. Q4 — Correctness boundary de CPU

**NÃO EXECUTADO.** Track D exigia remover o servidor como variável (réplica com
backend amplamente dimensionado) e varrer 1,5 → 6 vCPU. Nada disso foi feito.

O que existe continua sendo o par de pontos da Fase 3 — 0 falsos sucessos em
conc 4, 29,2% em conc 8 — com o confound do tier web do alvo (limite de 500m,
medido em 290m durante conc 4) **não removido**.

Portanto: `CPU headroom: NÃO DETERMINADO`. O número de 25% da Fase 2 continua sem
respaldo, e não deve ser usado.

---

## 6. Track J — preparado, aguardando pareamento

Harness completo e validado **sem tocar no WhatsApp** (`mode=waprep`):

| verificação | resultado |
|---|---|
| perfil persistente gravável e reutilizado entre 2 boots | OK, 2,7 MB escritos |
| piso do browser | 231–245 MB, 11 processos, 3 renderers |
| captura de screenshot para o volume de saída | OK |
| encerramento SIGTERM com espera (libera o perfil) | implementado |

Fingerprint de automação medido:

```text
navigator.webdriver      false
userAgent                ...HeadlessChrome/151.0.0.0
plugins                  5
hardwareConcurrency      10
```

Corrige uma afirmação anterior deste estudo: `navigator.webdriver` é **false**
(nem o perfil canônico nem o chromedp passam `--enable-automation`). A superfície
de detecção é o **User-Agent**, que declara `HeadlessChrome` explicitamente.
Mascaramento é a frente separada da ADR-0006 D3 e não existe.

Garantias implementadas em código, não apenas em conduta:

- não há caminho que envie mensagem;
- nenhuma conversa é aberta sem `-wa-chat` explícito;
- screenshot **apenas** na tela de QR; nada é capturado após o login;
- relatórios contêm contagens, tamanhos e durações — nunca conteúdo, contato,
  cookie ou token;
- três modos separados (`waprep` / `waopen` / `wacap`) para que o QR nunca
  apareça como efeito colateral.

---

## 7. O que NÃO foi executado

| item | status |
|---|---|
| Track D — correctness boundary de CPU | **não executado** |
| Track E — InteractionPolicy V1 | **não executado** |
| Track F — suíte hostil + baseline Playwright | **não executado** |
| Track G — replicação do site isolation (N≥5) | **não executado** (N=1 da Fase 3) |
| Track H — soak 24h | **NOT EXECUTED — operational constraint** |
| Track I — fault domain / recovery / blast radius | **não executado** |
| Track J — workload final | preparado; aguardando pareamento |
| Track K — modelo econômico | **não executado** (depende de J) |

---

## 8. Decisão: GO WITH CONDITIONS

Os critérios de `GO` exigem que **todos** sejam verdadeiros. Estado atual:

| critério | estado |
|---|---|
| 24h soak sem crescimento não limitado | ✘ não executado (60 min: bounded) |
| RecoveryPolicy medida | ✘ não executado |
| False success = 0 no operating point | ✔ 0/96 até o knee (SPA de referência) |
| InteractionPolicy implementada | ✘ não implementada |
| ConnectionPolicy decidida por medição | ✘ decidida por simplicidade, LOW |
| Target final medido | ✘ aguardando pareamento |
| CPU/RAM headroom conhecido | ✘ CPU não determinado |
| Blast radius aceito | ✘ não medido |
| Custo/node conhecido ou modelado | ✘ não calculado |

Um de nove satisfeito. **`GO WITH CONDITIONS`**, com os oito restantes como
bloqueadores explícitos. Não é `NO-GO`: nada medido até aqui contradiz a
arquitetura — o que falta é medição, não correção.

---

## 9. Revisit triggers

```text
false success > 0 em qualquer concorrência <= knee
p99 > 10 s no knee
CPU throttling > 60% no operating point
memory.current > 70% do limite do pod
slope de anon > 5 MB/h em soak
renderer count > 2 x slots
browser restart rate > 1/hora
recovery > 30 s
bytes CDP/job > 2 x baseline (sinal da tempestade de polling de §1)
upgrade de major do Chromium
upgrade do chromedp
novo SPA alvo, ou > 5 origens, ou > 3.000 nós de DOM
mudança de arquitetura do alvo
```
