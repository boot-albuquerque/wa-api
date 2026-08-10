# Handoff — Fase 5 em andamento

Documento de retomada. A fonte de verdade das fases anteriores é
`RELATORIO-FASE-4C.md`; este arquivo cobre apenas o que aconteceu **depois** dele.

---

## Onde estamos

```
Etapa 1-2  ler 4C, confirmar não repetir      FEITO
Etapa 3    InteractionPolicy V1               REPROVADA (excesso de recusa)
Etapa 4    Hostile suite                      EXECUTADA
Etapa 5    InteractionPolicy V2..V5           APROVADA, suite de 9 casos PASS
Etapa 6    CPU boundary (alvo real)           BLOQUEADO, causa desconhecida
Etapa 7-14 RAM, Pod Recovery, capacity,
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

## V3 — resíduo 1 fechado (identidade de nó)

| caso | V2 | V3 |
|---|---|---|
| `node-replacement` | correct_failure, **wrong=1** | correct_failure, **wrong=0** |

Cada elemento carrega `__p5id` (propriedade expando, não atributo — atributo
entra no DOM serializado e pode casar com seletor de terceiro). A estabilidade
exige **identidade além de geometria**, a troca vira `NODE_CHURN` separado de
`UNSTABLE_GEOMETRY` (esperar não resolve re-render), e há reconferência colada
no `Act`.

A janela não fecha: entre a última sondagem e o `DispatchMouseEvent` a troca
ainda é possível, e nenhum protocolo elimina isso. Caiu de ~1 amostra de
estabilidade para uma ida ao browser, e virou detectável. Quem fecha o caso
continua sendo o `Verify`.

Veredito segue PASS. Commit `9eb1e6e`.

---

## Sessão do WhatsApp — VIVA, e o susto que veio junto

`waopen` com o UA correto devolveu `state: app`, `matched_by: #pane-side`,
perfil 154,5 MB, `sync_settle` 11 ms. **A credencial da 4C sobreviveu; nenhum QR
foi gasto.**

Dois aprendizados do caminho, os dois caros se repetidos:

**1. Sem `-wa-ua` o alvo recusa.** Sai a tela "atualize o Google Chrome",
`canvases: 0`, e o modo termina em `not logged in` — que parece sessão perdida e
não é. O UA está na 4B §8:

```
UA="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"
```

**2. F94 — o harness desligava por sinal.** Ver `HOUSEKEEP.md`. Os dois `waopen`
desta sessão saíram sujos (3 `Singleton` no perfil). A credencial aguentou, mas
a 4C viu logout na 4ª iteração desse regime. **Corrigido e travado por teste
antes de qualquer outra execução contra a conta** — commit `a66db7a`.

**3. A credencial da 4C acabou caindo mesmo assim**, e foi REPAREADA em
2026-08-10. A queda veio depois de um `waopen` saudável, com o perfil encolhendo
de 153 para 146 MB — o marcador da 4C. Melhor explicação: dano acumulado dos
dois ciclos sujos acima (`INFERRED`, MEDIUM). A hipótese de que o `cpubound`
derrubava a sessão foi **refutada** — ver a seção da sessão estável.

Critério operacional para quem retomar: um `waopen` deve imprimir
`stopped_via=browser.close` e deixar **zero** `Singleton`. Se imprimir algo
começando com `DIRTY_`, pare e investigue antes de seguir.

---

## V4/V5 — o que só o alvo real revelou

Dois defeitos da policy que **nenhuma versão da hostile suite pegava**, porque
as páginas de teste sempre tinham o alvo confortavelmente dentro da tela.

**1. Ponto de clique no centro geométrico.** O `inView` testava se o elemento
*intersecta* o viewport, e o `Act` clicava no centro. Medido no WhatsApp: a
caixa de busca em `left=-70, top=-25, 592×28` — 25 dos 28 px acima da dobra.
O teste dizia visível, o centro caía em `(226,-11)`, fora da tela,
`elementFromPoint` devolvia `null`, e a recusa saía como
`OCCLUDED_OR_REPLACED` — mandando o diagnóstico para overlay quando a causa era
geometria.

No caminho ingênuo o efeito é pior: clique numa coordenada fora da tela,
acertando outra coisa. **`wrong_target` 5/5 no alvo real, a 3 CPUs, sem
starvation nenhuma.** MEASURED, HIGH.

Corrigido: o ponto é o centro da **interseção** do elemento com o viewport.

**2. Scroll-into-view.** Deixou de ser resíduo e virou requisito medido. Rola
uma vez por `Validate`, antes da janela de estabilidade, só quando não há ponto
de clique útil, e aplicando o mesmo filtro de actionability do `Resolve` — para
a rolagem não escolher alvo por baixo dos panos.

**Mudança de expectativa na suite, deliberada.** `fora-do-viewport` passou a
`ExpectAct=true`: com rolagem, recusá-lo é over-refusal, a mesma família do FAIL
da V1. A cobertura antiga **não foi perdida** — migrou para
`fora-da-tela-fixo` (`position:fixed` acima da tela, que rolagem não alcança).
Trocar expectativa sem repor cobertura seria enfraquecer a suite para caber no
código.

Suite: 9 casos, PASS, `cs=3 cf=6 fs=wt=ff=0`.

---

## O CPU boundary está BLOQUEADO — causa desconhecida

**NÃO MEDIDO.** Não extrapolar, não inferir a partir do sweep da carga
controlada.

O `#side` do WhatsApp renderiza fora da área visível, e a coisa toda é estática:

| janela | viewport | `#side` |
|---|---|---|
| 1280×800 (canônico) | 1280×657 | `(-165,-42)` 715×830 |
| 1600×1200 | 1600×1057 | `(-229,-122)` 671×1390 |

Os cinco elementos clicáveis da lateral têm `x` negativo e `hit=false` em todos.
Ampliar a janela **piorou**. Quatro amostras ao longo de 8 s deram geometria
idêntica ao pixel, então **não é assentamento** — o requisito nº 5 da 4C não
explica isto. `doc_scroll=[0,0]`, `zoom=1`, transform de ancestral é identidade.

Não é seletor errado nem defeito da policy. `OBSERVED`, HIGH quanto ao fato;
**causa desconhecida**, e deliberadamente não inventada.

**Próxima hipótese a testar** (não testada): `--window-size` pode não definir o
viewport de layout que o app usa em `headless=new` — `outerWidth/outerHeight`
voltam `[0,0]`. O caminho padrão é fixar o viewport por CDP com
`Emulation.setDeviceMetricsOverride` em vez de depender da flag. Uma execução
resolve a dúvida.

Instrumentação já pronta para isso, em `p5_cpubound.go`: dump de `layout:`
(viewport, scroll, zoom, transform, rects), `clicaveis_no_side:` (só estrutura —
tag, role, data-icon, geometria, hit — nunca texto) e `side_settle[i]` para
distinguir layout de animação.

---

## Sessão: estável sob o regime limpo

Sobreviveu a ~8 execuções seguidas nesta sessão. Perfil **172 → 183 MB**, sempre
crescendo, `stopped_via=browser.close` em todas, zero `Singleton`.

Isso **refuta a hipótese B** (o `cpubound` derrubaria a sessão). Sobra a
hipótese A — dano acumulado dos dois ciclos sujos pré-F94 — como a melhor
explicação da queda anterior. `INFERRED`, MEDIUM.

---

## Dois resíduos honestos da V2 — AMBOS FECHADOS

Nenhum dos dois afeta o veredito, e nenhum produz falso sucesso. Mas os dois
importam para o alvo real e não devem ser redescobertos.

**1. ~~Em `node-replacement` a policy ainda emite o clique errado.~~ FECHADO
na V3** — ver seção acima. Ficava `ground_truth_wrong_clicks = 1` porque o nó
substituído tem geometria idêntica: `Validate` o considerava parado e quem
recusava era o `Verify`, depois do clique. Resolvido com identidade de nó.

**2. ~~A policy não rola a página.~~ FECHADO na V5** — ver seção acima. Era
`OUT_OF_VIEWPORT` em tudo abaixo da dobra, e a lista de conversas do WhatsApp
exige rolagem. Resolvido com scroll-into-view no `Validate`, mais o caso novo
`fora-da-tela-fixo` para preservar a cobertura de "recusar o inalcançável".

---

## Etapa 6, parte QR-free — a policy sob starvation de CPU

Feito antes de gastar pareamento, para provar que o braço executa sob pressão de
CPU em vez de descobrir isso com a credencial na mesa. A hostile suite serve
como carga controlada porque **carrega ground truth**; o sweep varia só
`--cpus`.

| cpus | policy | chromedp direto (fs / wt / cs) | parede |
|---|---|---|---|
| 3.0 | PASS | 2 / 2 / 4 | 28 s |
| 2.0 | PASS | 2 / 2 / 4 | 29 s |
| 1.0 | PASS | 2 / 2 / 4 | 34 s |
| 0.5 | PASS | 2 / 2 / 3 | 72 s |
| 0.35 | PASS | 2 / 2 / 4 | 60 s |
| 0.25 | PASS | 1 / 2 / 3 | 91 s |

**A garantia da policy (`fs=wt=ff=0`, `cs=2`) não se rompeu em nenhum teto,
até 0,25 CPU** — MEASURED, confiança HIGH *para esta carga*. O caminho ingênuo,
no mesmo intervalo, oscila: perde sucessos e troca falso sucesso por recusa
conforme a lentidão desloca as janelas de corrida.

Um "eu não teria adivinhado" do sweep, visível só por causa do `decision_ms`:
**custo de sondagem e cronograma da página escalam de forma diferente.** De 3,0
para 0,25 CPU, a decisão em `overlay` (limitada por sondagem) foi de 189 ms para
703 ms, ~3,7×; já `disabled-vira-enabled` ficou em ~800–1040 ms nos dois
extremos, porque quem manda ali é um `setTimeout` da página, que a starvation
quase não afeta. Ou seja, sob CPU escassa o agente perde margem **contra
relógios que não desaceleram junto com ele**. É esse descompasso que decide o
budget, não a lentidão absoluta.

**O que este sweep NÃO estabelece.** Ele mede a policy contra páginas estáticas
de poucos nós. O WhatsApp Web é a carga pesada, e a Fase 3 já mostrou que a
degradação sob starvation aparece na completude do DOM — 7 de 24 jobs
"passaram" contra página incompleta. O boundary do alvo real continua **NÃO
MEDIDO**, exatamente como o 4C §8 registrou. Não extrapolar esta tabela para ele.

Artefatos: `results-p4c/hostile-cpu-{3.0,2.0,1.0,0.5,0.35,0.25}.json`.

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
| `p5_interaction.go` | InteractionPolicy — Resolve/Validate/Act/Verify, budget, identidade de nó, interseção com viewport, scroll |
| `p5_hostile.go` | suite dos 9 cenários, ground truth em `window.__truth` |
| `p5_cpubound.go` | boundary contra o alvo real + instrumentação de layout |
| `shutdown_policy_test.go` | trava a F94: nenhum modo com credencial desliga por sinal |

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

**Viva** — verificado nesta sessão, sem gastar QR. Ver a seção "Sessão do
WhatsApp — VIVA" acima para o UA obrigatório e para a F94. Protocolo mantido:
**avisar antes de exibir o QR e esperar confirmação explícita.**

---

## Próximos passos, em ordem

1. ~~InteractionPolicy V2~~ — **FEITO**, PASS 3/3 com controle negativo.
2. ~~Resíduo 1, identidade de nó~~ — **FEITO** (V3).
3. ~~Verificar a F94 contra a conta~~ — **FEITO**: `stopped_via=browser.close`,
   `Singleton` 3 → 0, perfil 148 → 153 MB.
4. ~~Resíduo 2, scroll-into-view~~ — **FEITO** (V5).
5. **DESTRAVAR o CPU boundary.** Único item que motivou o pareamento e não
   fechou. Hipótese a testar: `Emulation.setDeviceMetricsOverride` em vez de
   `--window-size`. Uma execução decide. Se destravar, medir 3,0 e 0,5
   primeiro — são as duas pontas que mais informam, e colhê-las cedo protege o
   resultado caso a credencial caia no meio.
6. RAM/sessão + pico de recovery → Pod Recovery graceful e abrupto → blast
   radius → capacidade 1/2/3 → recycling → timeouts por p95/p99 → economia →
   decisão.

O `-wa-chat` **não é necessário**: o job da busca é ciclo completo, read-only,
sem abrir conversa.

Critério do boundary, quando ele rodar: contra o alvo real não existe
`window.__truth`, então a testemunha são ouvintes de `click` (fase de captura) e
`focusin` instalados antes de agir. Todo caso é `ExpectAct=true`, logo recusa é
sempre `false_failure` — sem isso, "recusou tudo sob 0,5 CPU" apareceria como
ausência de falso sucesso e daria o resultado degenerado de que a policy é
perfeita a 0,1 CPU porque nunca age.

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
