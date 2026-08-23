# Fase 5 — InteractionPolicy: eliminar o falso sucesso

**Veredito da policy:** `APROVADA`
**CPU correctness boundary do alvo real:** `NÃO MEDIDO` — bloqueado, causa
desconhecida, registrado em §6 com a evidência.

A Fase 4C entregou um harness confiável e a topologia do alvo. Faltava a camada
que decide se uma interação **aconteceu de verdade**, que é o defeito que a Fase
1-2 registrou como falso sucesso e a Fase 3 mediu sob starvation: 7 de 24 jobs
"passaram" contra uma página cuja contagem de nós caía de 1015 para 752.

---

## 1. O que a policy é, e o que ela recusa

Quatro estágios, cada um contra uma forma distinta de mentira:

| estágio | pergunta |
|---|---|
| `Resolve` | qual nó, entre os que casam, é O alvo |
| `Validate` | ele pode ser interagido AGORA, no ponto onde o clique cai |
| `Act` | input real do browser, nunca `element.click()` |
| `Verify` | a aplicação observavelmente mudou de estado |

Sem `Verify` os três primeiros não bastam: um clique legítimo num botão que a
aplicação ignora é indistinguível de sucesso pelo protocolo. "O comando foi
aceito" e "a aplicação mudou de estado" são afirmações diferentes, e só a
segunda interessa.

---

## 2. O critério de aprovação, e por que a versão curta dele é falsa

A hostile suite dá **ground truth na própria página**: `window.__truth` registra
se o clique legítimo no alvo pretendido aconteceu. Sem isso a suite mediria a
opinião do controlador sobre si mesmo, que é o defeito sob teste.

A primeira execução imprimiu **PASS, e era falso**. O critério de §16
(`false_success = 0`) é necessário mas **não suficiente**: uma policy que recusa
tudo o satisfaz trivialmente, e meu classificador mapeava "deveria agir e
recusou" para `correct_failure`, escondendo a solução degenerada.

Foi a **terceira** ocorrência do mesmo padrão nesta linha de trabalho:

- self-test do `Poll` passando por condição nunca avaliada (4C §7)
- classificador do Track C engolindo a perda de auth (4C §4)
- este

O critério passou a ser de dois lados — `fs = 0` **e** `wt = 0` **e** `ff = 0` —
e agora exclui as duas soluções degeneradas: a que nunca age reprova por
`false_failure`, a que sempre age reprova por `false_success`/`wrong_target`.

> **Regra, válida para toda fase seguinte:** todo critério de aprovação precisa
> ser testado contra a solução degenerada — *o que passaria se o agente não
> fizesse nada?*

---

## 3. As quatro versões, e o que cada uma custou

| versão | mudança | resultado |
|---|---|---|
| V1 | quatro estágios, janelas fixas | **FAIL** — `ff=2` |
| V2 | reamostragem até `SettleBudget` | PASS 3/3 |
| V3 | identidade de nó além de geometria | `node-replacement` sem clique errado |
| V4 | ponto de clique = interseção com viewport | achado do alvo real |
| V5 | scroll-into-view | achado do alvo real |

**V1 reprovou por excesso de recusa.** `Resolve` sondava uma vez; `Validate`
amostrava `StableFrames*4 × 60 ms = 0,72 s`. Nos dois cenários em que um agente
correto deve agir, o estado certo chegava DEPOIS: o botão habilita em 800 ms, o
modal para em ~1,6 s. Não era defeito de desenho — faltava o laço.

**O PASS da V2 é atribuível, não coincidente.** Cada caso registra `decision_ms`:

| caso | a página muda em | policy decidiu em (3 reps) |
|---|---|---|
| `disabled-vira-enabled` | 800 ms | 1028 / 1025 / 1040 ms |
| `modal-animando` | ~1,6 s | 1857 / 1812 / 1828 ms |

Se esses sucessos saíssem em ~0 ms, o veredito seria verde e a explicação
errada.

**Controle negativo executado no mesmo binário.** `P5_SETTLE_BUDGET=0s` colapsa
o budget, reproduz a V1 e devolve `FAIL` com `ff=2`. O ponto de controle ficou
atrás de variável de ambiente de propósito: controle negativo que exige checkout
de outra versão quase nunca é reexecutado.

**V3.** Em `node-replacement` a V2 recusava corretamente, mas só DEPOIS de já ter
clicado no nó errado (`wrong=1` nas 3 réplicas) — geometria idêntica fazia o
`Validate` considerar o substituto "parado", e quem recusava era o `Verify`. Com
identidade de nó (`__p5id`, propriedade expando e não atributo, para não casar
com seletor de terceiro), a defesa passou para ANTES do clique. A janela não
fecha por completo, e o código diz isso: entre a última sondagem e o
`DispatchMouseEvent` a troca ainda é possível, e nenhum protocolo elimina isso.

**Resultado final: 9 casos, PASS, `cs=3 cf=6 fs=wt=ff=0`.** MEASURED, HIGH.

---

## 4. O que só o alvo real revelou

Dois defeitos que **nenhuma versão da hostile suite pegava**, porque as páginas
de teste sempre tinham o alvo confortavelmente dentro da tela.

**Ponto de clique no centro geométrico.** O `inView` testava se o elemento
*intersecta* o viewport, e o `Act` clicava no centro. Medido no WhatsApp: a
caixa de busca em `left=-70, top=-25, 592×28` — 25 dos 28 px acima da dobra. O
teste dizia visível, o centro caía em `(226,-11)`, fora da tela,
`elementFromPoint` devolvia `null`, e a recusa saía como
`OCCLUDED_OR_REPLACED` — mandando o diagnóstico para overlay quando a causa era
geometria.

O efeito no caminho ingênuo é o achado que mais importa desta fase:

> **`wrong_target` 5/5 no alvo real, a 3 CPUs, sem starvation nenhuma.**

O chromedp direto clicou numa coordenada fora da tela, acertou outra coisa e
**afirmou sucesso nas cinco tentativas**. O problema que a Fase 3 atribuía a
falta de CPU aparece aqui só por geometria. MEASURED, HIGH.

**Scroll-into-view.** Deixou de ser refinamento e virou requisito medido: sem
ele a policy não alcança nada abaixo da dobra, e a lista de conversas é uma
lista rolável.

**Uma expectativa de teste mudou, e isso merece escrutínio.**
`fora-do-viewport` passou a `ExpectAct=true`, porque com rolagem recusá-lo é
over-refusal — a mesma família do FAIL da V1. A cobertura antiga **não foi
perdida**: migrou para `fora-da-tela-fixo`, um `position:fixed` acima da tela
que rolagem não alcança. Trocar expectativa sem repor cobertura seria
enfraquecer a suite para caber no código.

---

## 5. A policy sob starvation, na carga controlada

| cpus | policy | chromedp direto (fs / wt / cs) | parede |
|---|---|---|---|
| 3.0 | PASS | 2 / 2 / 4 | 28 s |
| 2.0 | PASS | 2 / 2 / 4 | 29 s |
| 1.0 | PASS | 2 / 2 / 4 | 34 s |
| 0.5 | PASS | 2 / 2 / 3 | 72 s |
| 0.35 | PASS | 2 / 2 / 4 | 60 s |
| 0.25 | PASS | 1 / 2 / 3 | 91 s |

A garantia não se rompeu em nenhum teto até 0,25 CPU. O caminho ingênuo, no
mesmo intervalo, oscila.

**O "eu não teria adivinhado", visível só por causa do `decision_ms`:** custo de
sondagem e cronograma da página escalam de forma diferente. De 3,0 para 0,25
CPU, a decisão em `overlay` (limitada por sondagem) foi de 189 ms para 703 ms,
~3,7×; já `disabled-vira-enabled` ficou em ~800–1040 ms nos dois extremos,
porque ali quem manda é um `setTimeout` da página, que a starvation quase não
afeta. Sob CPU escassa o agente perde margem **contra relógios que não
desaceleram junto com ele** — e é esse descompasso que dimensiona o budget, não
a lentidão absoluta.

**Isto NÃO é o boundary do alvo real** e não deve ser extrapolado: são páginas
estáticas de poucos nós.

---

## 6. O boundary do alvo real: NÃO MEDIDO

O `#side` do WhatsApp renderiza fora da área visível, e o estado é estático.

| viewport | `#side` x | y | w | h |
|---|---|---|---|---|
| 1280×657 (`--window-size` canônico) | −165 | −42 | 715 | 830 |
| 1600×1057 (`--window-size` ampliado) | −229 | −122 | 671 | 1390 |
| 1280×900 (`Emulation.setDeviceMetricsOverride`) | −165 | −90 | 715 | 1170 |

Todos os cinco elementos clicáveis da lateral têm `x` negativo e `hit=false`.
Quatro amostras ao longo de 8 s deram geometria **idêntica ao pixel**, então não
é assentamento. `doc_scroll=[0,0]`, `zoom=1`, transform de ancestral é
identidade.

**Duas hipóteses testadas e refutadas:**

1. *Janela pequena demais.* Ampliar para 1600×1200 **piorou**.
2. *`--window-size` não define o viewport de layout em `headless=new`.* O
   override por CDP funcionou — `vw/vh` mudaram como pedido — e o deslocamento
   permaneceu.

`x` e `w` dependem só da largura (idênticos nos dois casos de 1280) e `h` escala
com a altura: o layout é responsivo e ainda assim sistematicamente deslocado.
Isso elimina a família inteira de hipóteses "janela/viewport".

`OBSERVED`, HIGH quanto ao fato; **causa desconhecida**, deliberadamente não
inventada. A investigação foi interrompida por decisão explícita, para não
entrar em segunda rodada de adivinhação sobre layout de terceiro.

---

## 7. F94 — o requisito que não saiu do experimento que o mediu

A 4C mediu que desligar por sinal corrompe o estado de sessão e escreveu isso
como requisito nº 1 de produção. **O requisito nunca foi propagado**: `waprep`,
`waopen`, `wacap`, `wasession` e `watabs` seguiram em `gracefulStop`.

Descoberto ao abrir o WhatsApp nesta fase — dois `waopen` deixaram os 3 arquivos
`Singleton` no perfil, a assinatura de saída suja da 4C. Três das nove chamadas
eram piores que as outras: em `oneRecoveryTrial` o desligamento sujo está em
**caminho de erro**, e um deles roda justamente quando o baseline falha, sujando
o perfil quando a credencial já está frágil.

Corrigido com `cleanStop` e **travado por teste estático sobre a AST**. Estático
de propósito: o defeito não é "o desligamento não funciona" —
`closeBrowserViaCDP` sempre funcionou — e sim "o call site chama a função
errada", que um teste de comportamento sobre `cleanStop` não veria. Controle
negativo executado.

Verificado em produção: `stopped_via=browser.close`, `Singleton` 3 → 0, perfil
148 → 153 MB.

**Nota de método:** a primeira versão do teste isentava funções inteiras por
nome e teria deixado passar os três defeitos de caminho de erro — só uma das
quatro chamadas em `oneRecoveryTrial` era ablação legítima. A isenção passou a
ser por linha (`//ablation:stop-form`). Isenção por nome de função cobre também
o código que ainda vai ser escrito ali dentro.

---

## 8. Sessão do WhatsApp

A credencial da 4C sobreviveu ao intervalo e foi confirmada viva. Caiu depois,
com o perfil encolhendo de 153 para 146 MB — o marcador da 4C — e foi
**repareada em 2026-08-10**.

Melhor explicação da queda: dano acumulado dos dois ciclos sujos anteriores à
F94. `INFERRED`, MEDIUM. A hipótese de que o `cpubound` a derrubava foi
**refutada**: a sessão nova sobreviveu a ~8 execuções seguidas, com o perfil
crescendo de 172 para 183 MB e `stopped_via=browser.close` em todas.

Armadilha que custou uma execução: **sem `-wa-ua` o alvo recusa** com a tela
"atualize o Google Chrome" e o modo termina em `not logged in` — que parece
sessão perdida e não é. Já estava documentado na 4B §8, e eu não consultei antes
de rodar.

---

## 9. Requisitos de produção acrescentados por esta fase

Somam-se aos seis da 4C §7.

7. **Interação crítica só pela policy completa.** O ganho não é teórico: o
   caminho ingênuo produziu `wrong_target` 5/5 contra o alvo real a 3 CPUs.
8. **Póscondição observável obrigatória.** Sem `Verify`, um clique que a
   aplicação ignorou é sucesso.
9. **Ponto de clique na interseção com o viewport**, nunca no centro
   geométrico.
10. **Scroll-into-view antes de validar**, com re-validação depois.
11. **Identidade de nó, não só geometria**, entre `Validate` e `Act`.
12. **Budget de reamostragem dimensionado pelo descompasso** entre custo de
    sondagem e relógios da aplicação — §5 mostra que os dois não escalam junto.

---

## 10. Não executado

Registrado como limitação, não como sucesso:

- **CPU correctness boundary do alvo real — NÃO MEDIDO** (§6).
- **Soak de 24 h — NOT EXECUTED**, restrição operacional.
- **Etapas 7-14** — RAM/sessão, pico de recovery, Pod Recovery, blast radius,
  capacidade 1/2/3, recycling, timeouts por p95/p99, economia por core-hour e
  GiB-hour: **não iniciadas**.
- **Limite superior de ciclos de vida** — segue desconhecido.

A decisão de produção de capacidade e custo **não pode ser tomada com esta
fase**: ela valida a camada de interação, não o envelope operacional.
