# Handoff — Fase 5 em andamento

Documento de retomada. A fonte de verdade das fases anteriores é
`RELATORIO-FASE-4C.md`; este arquivo cobre apenas o que aconteceu **depois** dele.

---

## Onde estamos

```
Etapa 1-2  ler 4C, confirmar não repetir      FEITO
Etapa 3    InteractionPolicy V1               REPROVADA (excesso de recusa)
Etapa 4    Hostile suite                      EXECUTADA
Etapa 5    InteractionPolicy V2               APROVADA, 3/3 PASS
Etapa 6-14 CPU, RAM, Pod Recovery, capacity,
           recycling, timeouts, economia      NÃO INICIADAS
```

Branch: `feature/internal-wa-headless`.

---

## Resultado da etapa 5 — InteractionPolicy V2: PASS

O FAIL da V1 era o previsto pelo diagnóstico: **a policy não esperava**.
`Resolve` sondava uma vez; `Validate` amostrava uma janela fixa de
`StableFrames*4 × 60 ms = 0,72 s`. Nos dois casos `ExpectAct=true` o estado
correto chegava depois dessa janela.

A V2 troca as janelas fixas por **reamostragem até um budget** (`SettleBudget`,
5 s), com `resolveOnce` (uma sondagem, não espera) separado de `Resolve`
(espera). Nada mais mudou: os quatro estágios são os mesmos.

| agente | false_success | wrong_target | correct_success | correct_failure | false_failure |
|---|---|---|---|---|---|
| chromedp direto | **2** | **2** | 4 | 0 | — |
| InteractionPolicy V1 | 0 | 0 | **0** | 6 | **2** |
| **InteractionPolicy V2** | **0** | **0** | **2** | **6** | **0** |

**3 réplicas, 3 PASS.** MEASURED, confiança HIGH.

### O PASS é atribuível, não coincidente

Cada caso agora registra `decision_ms` — quanto tempo o agente levou entre
página carregada e decisão. Os dois sucessos aparecem com espera compatível com
o cronograma da própria página, que é o que liga o resultado à causa alegada:

| caso | a página muda em | policy decidiu em (3 reps) |
|---|---|---|
| `disabled-vira-enabled` | 800 ms | 1028 / 1025 / 1040 ms |
| `modal-animando` | ~1,6 s | 1857 / 1812 / 1828 ms |

Se esses sucessos tivessem saído em ~0 ms, a explicação estaria errada mesmo
com o veredito verde.

### Controle negativo EXECUTADO

`P5_SETTLE_BUDGET=0s` colapsa o budget e reproduz a V1 **no mesmo binário** que
produziu o PASS — sem checkout de outra versão, que é o motivo pelo qual
controles negativos costumam nunca ser reexecutados.

```
interaction-policy: false_success=0 wrong_target=0 correct_success=0 correct_failure=6
false_failure=2  ->  FAIL
```

Reproduz exatamente a linha da V1. A mutação compila e falha com mensagem.

### O critério agora é de dois lados

`false_success=0 E wrong_target=0 E false_failure=0`. As duas soluções
degeneradas ficam de fora: a que nunca age reprova por `false_failure`, a que
sempre age reprova por `false_success`/`wrong_target`. A versão curta do
critério (`false_success=0`) já produziu um PASS falso nesta suite.

---

## Dois resíduos honestos da V2 — entram na V3, não são bloqueadores

Nenhum dos dois afeta o veredito, e nenhum produz falso sucesso. Mas os dois
importam para o alvo real e não devem ser redescobertos.

**1. Em `node-replacement` a policy ainda emite o clique errado.**
`ground_truth_wrong_clicks = 1` nas 3 réplicas. O nó substituído tem geometria
idêntica, então `Validate` o considera estável e o hit-test passa; quem recusa é
o `Verify`, depois do clique. Ou seja: **a proteção contra node replacement vem
do Verify, não do Validate.** Isso basta para não MENTIR sobre o resultado, mas
não impede o efeito colateral — e no WhatsApp um clique no nó errado pode
significar abrir a conversa errada. Fix candidato: carregar identidade do nó
(`DOM.getNodeId` / backendNodeId) entre `Validate` e `Act`, não só geometria.

**2. A policy não rola a página.** Em `fora-do-viewport` o chromedp direto
acerta (ele faz scroll-into-view) e a policy recusa com `OUT_OF_VIEWPORT`. O
ground truth da suite trata isso como recusa correta, então não conta contra o
veredito — mas a lista de conversas do WhatsApp **exige rolagem**, e sem
scroll-into-view a policy não alcança nada abaixo da dobra. É requisito, não
detalhe.

---

## Diagnóstico do FAIL da V1 — histórico, já resolvido

Mantido só como registro: `Resolve` recusava com `NO_ACTIONABLE_CANDIDATE` na
primeira sondagem, e `Validate` esgotava a janela fixa antes de a geometria
estabilizar. Os quatro estágios estavam corretos; faltava o laço.

---

## Armadilha encontrada — vale como regra geral

A primeira execução da suite imprimiu **PASS**, e era falso.

O critério de §16 (`false_success = 0`) é **necessário mas não suficiente**:
uma policy que recusa tudo o satisfaz trivialmente. Meu classificador mapeava
"deveria agir e recusou" para `correct_failure`, escondendo a solução
degenerada.

Foi a terceira ocorrência do mesmo padrão nesta linha de trabalho:

- self-test do `Poll` passando por condição **nunca avaliada** (4C §7)
- classificador do Track C engolindo a perda de auth (4C §4)
- este

**Regra:** todo critério de aprovação precisa ser testado contra a solução
degenerada — *o que passaria se o agente não fizesse nada?*

Corrigido em `p5_hostile.go`: esse caso vira `false_failure` e o veredito exige
`ff = 0` além de `fs = 0` e `wt = 0`.

---

## Arquivos desta fase

| arquivo | conteúdo |
|---|---|
| `p5_interaction.go` | InteractionPolicy V1 — Resolve/Validate/Act/Verify |
| `p5_hostile.go` | suite dos 8 cenários, ground truth em `window.__truth` |

Modo: `-mode hostile`. Roda em ~40 s, **não toca no WhatsApp**, não exige
pareamento. Dá para iterar na V2 sem gastar QR.

```bash
cd scripts/chromium-study
go build ./... && GOOS=linux GOARCH=arm64 go build -o study-linux-arm64 .
docker build -q -t chromium-study:p5 .
docker run --rm -e SKIP_BROWSER=1 --cpus=3 --memory=3g \
  -v "$PWD/results-p4c:/out" chromium-study:p5 -mode hostile -out /out/hostile.json
```

Em host amd64, trocar o alvo de build para `study-linux-amd64` (o Dockerfile
copia ambos).

---

## Regras herdadas que continuam valendo

- **Não repetir a Fase 4C**: harness/deadlines, single-tab, causa da perda de
  credencial, `Browser.close` vs `chromedp.Cancel`, benchmarks de controller.
- Nenhum `chromedp.Poll` sem `WithPollingInterval` explícito (rAF não dispara em
  aba de fundo headless).
- `primeTab` antes de qualquer operação sob prazo — o primeiro `Run` cria o
  target e o prende ao contexto daquele `Run`.
- Shutdown de produção é `Browser.close` via CDP cru, nunca `chromedp.Cancel`
  nem sinal.
- Soak de 24 h: **NOT EXECUTED**, restrição operacional. Não pedir de novo.
- Sem stealth, anti-ban, fingerprint evasion.
- Antes de gastar pareamento, **provar que o braço executa**. As duas vezes em
  que isso foi pulado custaram um pareamento cada.

---

## Sessão do WhatsApp

Ao fim da Fase 4C a credencial estava saudável (40 ciclos limpos com
`Browser.close`). Assuma que expirou e planeje pareamento apenas quando chegar
nas etapas que tocam o alvo real (5 em diante). Protocolo: **avisar antes de
exibir o QR e esperar confirmação explícita.**

---

## Próximos passos, em ordem

1. ~~InteractionPolicy V2~~ — **FEITO**, PASS 3/3 com controle negativo.
2. **CPU correctness boundary** contra o WhatsApp real — **exige pareamento**.
   Antes de gastá-lo, provar que o braço executa: rodar o mesmo harness contra
   a hostile suite sob os mesmos tetos de CPU e confirmar que a policy decide,
   e não que o container simplesmente engasga.
3. RAM/sessão + pico de recovery → Pod Recovery graceful e abrupto → blast
   radius → capacidade 1/2/3 → recycling → timeouts por p95/p99 → economia →
   decisão.

Resíduos da V2 (§ acima) entram na V3: identidade de nó entre Validate e Act, e
scroll-into-view.

### Como reproduzir a etapa 5

```bash
STUDY=scripts/chromium-study
GOOS=linux GOARCH=arm64 go -C "$STUDY" build -o study-linux-arm64 .
docker build -q -t chromium-study:p5 "$STUDY"

# PASS
docker run --rm -e SKIP_BROWSER=1 --cpus=3 --memory=3g \
  -v "$PWD/$STUDY/results-p4c:/out" chromium-study:p5 -mode hostile -out /out/hostile-v2.json

# controle negativo -> FAIL com false_failure=2
docker run --rm -e SKIP_BROWSER=1 -e P5_SETTLE_BUDGET=0s --cpus=3 --memory=3g \
  -v "$PWD/$STUDY/results-p4c:/out" chromium-study:p5 -mode hostile -out /out/hostile-v2-negctl.json
```

Artefatos: `results-p4c/hostile-v2.json`, `hostile-v2-rep2.json`,
`hostile-v2-rep3.json`, `hostile-v2-negctl.json`. Cada um carrega
`settle_budget`, que é a variável que separa V1 de V2.
