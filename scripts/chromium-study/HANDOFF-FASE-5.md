# Handoff — Fase 5 em andamento

Documento de retomada. A fonte de verdade das fases anteriores é
`RELATORIO-FASE-4C.md`; este arquivo cobre apenas o que aconteceu **depois** dele.

---

## Onde estamos

```
Etapa 1-2  ler 4C, confirmar não repetir      FEITO
Etapa 3    InteractionPolicy V1               IMPLEMENTADA, REPROVADA
Etapa 4    Hostile suite                      EXECUTADA
Etapa 5-14 CPU, RAM, Pod Recovery, capacity,
           recycling, timeouts, economia      NÃO INICIADAS
```

Working tree limpo. Último commit: `4e8ac73`.
Branch: `feature/internal-wa-headless`.

---

## Resultado medido da etapa 4

```
InteractionPolicy V1:  FAIL
Hostile suite:         6/8 corretos
False-success rate:    0
Wrong-target rate:     0
```

| agente | false_success | wrong_target | correct_success | correct_failure | false_failure |
|---|---|---|---|---|---|
| chromedp direto | **2** | **2** | 4 | 0 | — |
| InteractionPolicy V1 | 0 | 0 | **0** | 6 | **2** |

**A policy funciona no que era o bloqueador**: elimina os 4 desfechos perigosos
do chromedp direto — falso sucesso em `disabled-vira-enabled` e `zero-size`,
alvo errado em `overlay` e `selector-duplicado`.

**Reprova por excesso de recusa**: recusa também os 2 casos que deveria
executar (`disabled-vira-enabled`, `modal-animando`, ambos `ExpectAct=true`).

---

## Diagnóstico do FAIL — já feito, não repetir

A causa é a mesma nos dois casos: **a policy não espera.**

1. `InteractionPolicy.Resolve` (`p5_interaction.go`) recusa com
   `NO_ACTIONABLE_CANDIDATE` na **primeira** sondagem. No cenário
   `disabled-vira-enabled` o botão habilita em 800 ms, mas a policy já
   desistiu.
2. `InteractionPolicy.Validate` amostra no máximo `StableFrames*4 = 12` vezes
   com intervalo de 60 ms. O modal de `/modal` se move por ~1,6 s, então a
   janela acaba antes da geometria estabilizar e sai `UNSTABLE_GEOMETRY`.

**Não é defeito de desenho.** Os quatro estágios (Resolve → Validate → Act →
Verify) estão corretos e provaram valor. Falta o laço de reamostragem até o
deadline da `DeadlinePolicy` (`OpQuery`, 15 s), em vez de janelas fixas curtas.

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

1. **InteractionPolicy V2** — reamostragem até o deadline no `Resolve` e no
   `Validate`. Alvo: `correct_success = 2` mantendo `fs = wt = ff = 0`.
2. Reexecutar `-mode hostile` e confirmar `PASS` **legítimo**.
3. Só então: CPU correctness boundary → RAM/sessão + pico de recovery → Pod
   Recovery graceful e abrupto → blast radius → capacidade 1/2/3 → recycling →
   timeouts por p95/p99 → economia → decisão.
