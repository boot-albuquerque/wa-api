# ADR-0007: Decisão final sobre `headless` — engine, escopo e o limite do que a medição pode decidir

- **Status**: accepted
- **Data**: 2026-08-10
- **Substitui parcialmente**: [ADR-0006](0006-headless-engine-de-browser-e-custo-por-sessao.md),
  cujo status era `proposed` e cujas perguntas em aberto D6/D7 são respondidas
  aqui.
- **Base empírica**: `scripts/chromium-study/RELATORIO-FASE-{1-E-2,3,4,4B,4C,5}.md`

## Contexto

O ADR-0006 abriu a segunda pilha — conduzir o SPA do `web.whatsapp.com` num
browser — com números de um spike pré-login em macOS arm64, e deixou explícito
que capacidade e custo operacional continuavam sem base.

As Fases 1 a 5 do estudo (`scripts/chromium-study/`) mediram o que faltava para
**decidir arquitetura**. Não mediram o que falta para **precificar**, e esta ADR
separa as duas coisas de propósito.

## O que o estudo acrescentou ao ADR-0006

| pergunta em aberto do 0006 | resposta |
|---|---|
| RSS de uma sessão **pareada** com histórico | **~900 MB anônimos, pico 1058 MB** (`OBSERVED`, MEDIUM). Contra 474–790 MB pré-login |
| os números valem em contêiner Linux? | medidos em contêiner Linux arm64. **amd64 continua NÃO medido** |
| chromedp vs rod, além dos defaults | chromedp **3,1–3,3× mais throughput** em amd64; rod com **18,8% de falhas em concorrência 32** |

E acrescentou três achados que o 0006 não tinha como antecipar:

1. **Uma sessão ativa por perfil.** Duas abas do WhatsApp no mesmo perfil não
   coexistem: a nova fica `APP_READY` e a anterior cai para `SESSION_CONFLICT`
   (3/3, `MEASURED`, HIGH). **Não há amortização de browser entre sessões.**
2. **Desligamento sujo destrói a credencial.** `SIGTERM` produziu logout na 4ª e
   na 6ª iteração; `Browser.close` via CDP fez 40 iterações sem degradação.
3. **Automação ingênua mente contra este alvo.** O caminho `chromedp.Click`
   direto produziu **`wrong_target` 5/5 no WhatsApp real, a 3 CPUs, sem
   starvation alguma** — clicou numa coordenada fora da tela e afirmou sucesso
   nas cinco.

## Decisão

### D1 — A engine é `chromedp`. Confirmado.

O ADR-0006 D1 escolheu por "quando cada default é uma assinatura, a engine certa
é a que não decide por nós". A medição chega ao mesmo lugar por outro caminho:
throughput 3,1–3,3× maior em amd64 real, e o rod colapsando em confiabilidade
sob concorrência, além de derrubar browser externo compartilhado ao fechar a
conexão. A Fase 3 isolou a causa do teto do rod — `WaitStableRAF` →
`Page.WaitRepaint`, e `requestAnimationFrame` não dispara em aba de fundo
headless.

Duas razões independentes apontando na mesma direção. **Fechado.**

### D2 — `noise` é a pilha primária. `headless` não é caminho geral.

A assimetria de custo é de **duas ordens de grandeza**, e é estrutural:

| | `noise` (protocolo) | `headless` (browser) |
|---|---|---|
| por sessão | WebSocket + estado de cripto | **~900 MB, pico 1058 MB** |
| compartilhamento | muitas sessões por processo | **uma sessão por perfil** |
| ordem de grandeza num host de 16 GiB | milhares | **dezenas** |

O que torna isso irreversível por engenharia é o achado 1: sem amortização de
browser, cada sessão é um Chromium inteiro. Não é constante a otimizar — é a
topologia do alvo.

### D3 — A pilha de browser é condicionada a uma lista explícita de recursos

O ADR-0006 justificou a pilha por duas razões. **Uma delas já está morta**: o
D3 daquele ADR admitiu que `navigator.webdriver === true` nas duas engines, e o
estudo foi explicitamente fora de escopo para stealth. A mitigação de ban
**não é entregue** por esta pilha hoje.

Resta a razão nº 1 — recursos que exigem o SPA real. Portanto:

> `headless` só se constrói contra uma **lista nomeada** de recursos que o
> `noise` não entrega. Essa lista é o business case, e ela precisa justificar
> uma diferença de custo por sessão de duas ordens de grandeza.

Se a lista for vazia ou marginal, a pilha não deve existir. **Esta é decisão de
produto, não de engenharia**, e vem da experiência de produção com o `wwebjs` —
não dos números deste estudo.

### D4 — Se existir, é tier de baixa densidade com preço próprio

Nunca como caminho padrão, nunca no mesmo pool do `noise`. Densidade da ordem
de **dezenas de sessões por host de 16 GiB**, contra milhares.

### D5 — A InteractionPolicy é obrigatória no caminho de browser

Medido: o caminho ingênuo produziu `wrong_target` 5/5 contra o alvo real, em
condições confortáveis de CPU. Automação de browser sem verificação de
póscondição observável não é mais barata — é só menos verificável.

A policy está implementada e aprovada: 9 cenários hostis com ground truth na
própria página, `false_success = wrong_target = false_failure = 0`, com controle
negativo executado. Requisitos que ela impõe estão no `RELATORIO-FASE-5.md` §9.

### D6 — Desligamento por `Browser.close` via CDP é requisito, não recomendação

`SIGTERM` corrompe o estado de sessão. O `preStop` do pod precisa fazer a chamada
CDP; nem sinal, nem `chromedp.Cancel` — este último **não envia o comando** sobre
`RemoteAllocator` e devolve `nil` em silêncio.

Travado por teste estático em `scripts/chromium-study/shutdown_policy_test.go`,
com controle negativo executado. Ver F94 no `HOUSEKEEP.md`.

### D7 — O cenário ideal de medição NÃO é pré-condição desta decisão

Ver a seção seguinte. É a decisão mais importante deste ADR.

---

## O cenário ideal pode ser uma farsa

Existe um cenário de medição correto: nó amd64 dedicado com taint, N contas
pareadas, proxy calibrada contra o alvo real, triangulação de memória por três
métodos, canário instrumentado. Ele é tecnicamente certo — e é **exatamente o
tipo de coisa que vira farsa** se tratado como pré-requisito.

Três modos de falha, todos observáveis em projetos reais:

**1. Vira desculpa de bloqueio.** A decisão de arquitetura fica esperando um
ambiente que ninguém prioriza, enquanto o produto é construído assim mesmo — só
que sem decisão registrada. O pior dos dois mundos: o custo do rigor sem o
benefício dele.

**2. Vira teatro.** Monta-se o ambiente, produz-se uma planilha impecável, e o
viés sistemático apenas mudou de lugar — o "nó dedicado" é um VPS compartilhado,
a "proxy calibrada" nunca teve a tolerância declarada antes, o soak virou 40
minutos. O número fica **preciso e continua inexato**, agora com aparência de
autoridade. Este estudo já produziu um PASS falso exatamente assim.

**3. Cria confiança falsa.** Um eixo não listado difere, ninguém percebe, e a
decisão herda um erro que a aparência de rigor protege de revisão.

### O antídoto: casar o custo da medição com a sensibilidade da decisão

A pergunta certa não é "esta medição é rigorosa?", e sim **"a decisão muda se o
número estiver errado por 3×?"**

| decisão | sensibilidade | medição necessária |
|---|---|---|
| protocolo vs browser como pilha primária | **nenhuma** — a diferença é ~100× | **já temos** |
| engine chromedp vs rod | **nenhuma** — duas razões independentes | **já temos** |
| policy obrigatória no browser | **nenhuma** — 5/5 é categórico | **já temos** |
| densidade por nó e preço por sessão | **alta** — 30% de erro vira 30% no preço | **falta tudo** |

**Conclusão operacional: a decisão de arquitetura está tomada e não depende de
mais nenhuma medição.** Adiá-la esperando o ambiente ideal seria confundir
rigor com procrastinação.

O ambiente caro fica **reservado** para a única classe de decisão que é sensível
a ele — densidade e preço — e só quando essa decisão estiver na mesa de verdade.

### E o que é impossível medir antes vira portão de operação

Deriva de memória ao longo de dias não se mede em laboratório. Em vez de
extrapolar corridas curtas, o primeiro pod real sobe **instrumentado**, com
gatilhos definidos antes:

- `memory.events.high` deixando de ser zero → reduzir densidade
- deriva sustentada acima do limite por hora → reciclar sessão por idade
- qualquer `oom_kill` → parar e rever o teto

Isso converte a incerteza em condição de operação, em vez de fingir que ela foi
resolvida por um número de laboratório.

---

## Consequências

**Boas.** A decisão de arquitetura sai agora, com base categórica e robusta a
ruído de ambiente. A engine está fechada por duas evidências independentes. A
camada de interação existe, está aprovada e tem requisitos escritos. O
desligamento correto está travado por teste.

**Custos.** `headless` carrega acoplamento estrutural ao SPA da Meta — o
ADR-0006 D4 inventariou dezenas de nomes de módulo, e a Fase 5 encontrou ainda
um estado de layout do próprio SPA em headless que não tem explicação. Dirigir
SPA de terceiro é herdar a renderização dele como dependência.

**Aceito conscientemente.** Nenhum preço por sessão é declarado aqui. A ordem de
grandeza — centenas de MB por sessão, dezenas por host — é suficiente para D2 e
D4, e insuficiente para faturar.

## Não decidido aqui

- **Preço por sessão e por nó.** Depende de medição em amd64 dedicado que não
  existe. É o assunto da próxima conversa, e entra nela **declarado como
  estimativa por ordem de grandeza**, não como número.
- **Baseline do `wwebjs`.** Continua fora do repositório, então "superar o
  `wwebjs`" permanece objetivo, não critério — como o ADR-0006 já registrava.
- **Mitigação de detecção.** Frente de trabalho própria, com medição própria.
- **CPU correctness boundary do alvo real.** `NÃO MEDIDO`; bloqueado por um
  estado de layout do SPA cuja causa é desconhecida (`RELATORIO-FASE-5.md` §6).
