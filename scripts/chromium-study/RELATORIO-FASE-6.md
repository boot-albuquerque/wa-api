# Fase 6 — H1: o que são os renderers de uma sessão

**Veredito do H1:** a premissa do nó estava **errada**. Não existe o ganho de
100–200 MB que eu projetei ao abri-lo.

Este é um resultado negativo, e é o produto do nó: ele impede a equipe de
perseguir um fantasma num plano que tem uma pessoa e nenhum prazo.

---

## 1. A premissa, e por que ela era plausível

A Fase 5 mediu **12 processos, 4 renderers**, para uma única sessão do WhatsApp,
com site isolation já desligada. O spike da ADR-0006 já havia levado 9→6
processos com três flags (~198 MB). Como o heap de JS é só 65 MB de 651 MB de
`anon`, **memória aqui é contagem de processo**, e reduzir renderers parecia o
lever óbvio.

O nó H1 foi aberto com a estimativa: *"chegar a 2 renderers seria da ordem de
100–200 MB por sessão"*.

## 2. O que foi medido

Sonda `-mode targets` (`p6_targets.go`): enumera targets por CDP cru e processos
por `/proc`, em estágios, atribuindo memória por **PSS** (`smaps_rollup`) e não
por RSS — somar RSS da árvore conta páginas compartilhadas várias vezes.

### 2.1 Os renderers não são da aplicação

| estágio | renderers | targets |
|---|--:|---|
| `boot` (nada navegado) | **2** | 2× `browser_ui` (`chrome://omnibox-popup.top-chrome/`), 1 `page about:blank` |
| `primed` (após `primeTab`) | 3 | + a `page` anexada |

**Dois renderers existem antes de qualquer navegação.** A aplicação soma +1.
`MEASURED`, HIGH.

### 2.2 Os 2 renderers do boot NÃO são os `browser_ui` — teste causal

Correlacionar "2 renderers ≈ 2 targets `browser_ui`" seria suposição, e ela
decide se removê-los economiza alguma coisa. Fechar por `Target.closeTarget` e
recontar responde direto:

| rep | fechados | renderers | PSS |
|---|--:|---|---|
| 1 | 2 | 2 → **2** | 387 → 388 MB |
| 2 | 2 | 2 → **2** | 388 → 394 MB |
| 3 | 2 | 2 → **2** | 394 → 395 MB |

**N=3, unânime: refutado.** Os `browser_ui` aparecem na lista de targets mas não
detêm os processos de renderer. `MEASURED`, HIGH.

### 2.3 Cinco flags candidatas, nenhuma reduz a contagem

| flag acrescentada | renderers |
|---|--:|
| *(baseline)* | 2 |
| **controle**: lista canônica explícita | 2 |
| `OmniboxPopupWebUI` | 2 |
| `WebUIOmniboxPopup` | 2 |
| `TopChromeWebUIUsesSpareRenderer` | 2 |
| `PreloadTopChromeWebUI` | 2 |
| `SpareRendererForSitePerProcess` (N=3) | 2 |

**Armadilha evitada, e ela quase custou o experimento:** `--disable-features`
aparecendo duas vezes faz o Chromium honrar **só a última ocorrência**, o que
apagaria em silêncio o `site-per-process` do perfil canônico e mudaria a memória
sob medição. Por isso cada candidato foi passado como a **lista canônica
completa + o candidato**, e o **controle** — lista canônica explícita — confirma
que a sobreposição preserva o baseline.

## 3. Achado de método: PSS não é atribuível neste N

Com **configuração idêntica**, o PSS variou:

```
baseline / controle / candidatos:  355 – 441 MB
spare renderer (N=3):              381 · 402 · 432 MB
```

**~50 a 86 MB de dispersão sem mudar nada.** Se eu tivesse rodado N=1 por flag,
teria "descoberto" que `PreloadTopChromeWebUI` economiza 40 MB e que
`TopChromeWebUIUsesSpareRenderer` gasta 46 MB — os dois seriam ruído.

Regra que fica: neste harness, **só a contagem (renderer, target) é afirmável**;
qualquer efeito de memória exige N≥3 e magnitude acima de ~50 MB.

## 4. Uma corrida inválida, e o sinal que a denunciou

A primeira execução do censo completo estourou o watchdog (10 min) **antes** do
estágio `appready`, e o cancelamento matou a aba. O estágio foi medido com a aba
já morta.

O sinal foi o próprio dado: **`PSS da aplicação: −32 MB`**, negativo. Aplicação
não subtrai memória.

Corrigido: orçamento para 25 min e **tempo por estágio** registrado — sem ele não
dá para saber onde o orçamento foi. Um número absurdo é sorte; o próximo pode ser
plausível e errado.

## 5. O que fica aberto

- **O que são os 2 renderers do boot.** Não sei. `browser_ui` e spare renderer
  foram refutados. Não inventar.
- **A composição com o app vivo.** A hipótese é `2 (boot) + 1 (página) +
  1 (service worker) = 4`, casando com a Fase 5 — mas é `INFERRED` até o censo
  completo válido, que estava em execução quando este relatório foi escrito.

## 6. Consequência para os documentos do `disparazaap`

O doc `docs/architecture/wa-headless-chromium-cdp.md` §2.1.1 afirma:

> *"são 4 renderers para uma sessão (…) chegar a 2 seria da ordem de 100–200 MB
> por sessão"*

**A segunda metade está refutada** por este nó. A primeira segue válida.

Correção a propor pelo `HOUSEKEEP.md` do `disparazaap`, **não editada daqui**: o
repo é da sessão C0/C1 e escrever nele desta sessão é o BLOCKER de trabalho
paralelo. Esta é a primeira vez que o processo é seguido em vez de furado.

## 7. Reprodução

```bash
S=scripts/chromium-study
GOOS=linux GOARCH=arm64 go -C "$S" build -o study-linux-arm64 .
docker build -q -t chromium-study:p6 "$S"

# boot-only: não toca no WhatsApp, ~15 s, inclui o teste causal
docker run --rm -e SKIP_BROWSER=1 -e WA_SESSION_DIR=/session/throwaway \
  --cpus=3 --memory=3g -v /tmp/census-throwaway:/session \
  -v "$PWD/$S/results-p4c:/out" chromium-study:p6 \
  -mode targets -boot-only -out /out/h1-boot.json

# censo completo: exige sessão pareada e -wa-ua
docker run --rm -e SKIP_BROWSER=1 --cpus=3 --memory=3g \
  -v "$PWD/$S/wa-session:/session" -v "$PWD/$S/results-p4c:/out" \
  chromium-study:p6 -mode targets -wa-ua "$UA" -out /out/h1-targets.json
```

Artefatos: `results-p4c/h1-causal-{1,2,3}.json`, `h1-spare-{1,2,3}.json`,
`h1-boot.json`, `h1-targets.json`.
