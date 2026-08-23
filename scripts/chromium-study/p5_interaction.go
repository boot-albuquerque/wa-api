package main

// Fase 5 §11–§15 — InteractionPolicy V1.
//
// O problema que esta camada existe para resolver foi medido, não suposto: a
// Fase 1-2 registrou FALSO SUCESSO, e a Fase 3 mostrou que sob starvation de
// CPU o controlador "tinha sucesso" contra uma página incompleta — 7 de 24 jobs
// passaram enquanto a contagem de nós do DOM caía de 1015 para 752.
//
// A causa é sempre a mesma: retorno do CDP sem erro é tratado como verdade. Não
// é. "O comando foi aceito" e "a aplicação mudou de estado" são afirmações
// diferentes, e só a segunda interessa.
//
// Quatro estágios, e cada um recusa uma forma distinta de mentira:
//
//	Resolve   qual nó, entre os que casam com o seletor, é O alvo
//	Validate  esse nó pode ser interagido AGORA, no ponto onde o clique cairá
//	Act       input real do browser, não atalho de JS
//	Verify    a aplicação observavelmente mudou para o estado esperado
//
// Sem Verify os três primeiros não bastam: um clique legítimo num botão que a
// aplicação ignora é indistinguível de sucesso pelo protocolo.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// InteractionOutcome é a classificação de uma tentativa de interação.
//
// As categorias são as de §16, e a distinção entre elas é o produto desta
// camada. Em particular FalseSuccess não é um erro do alvo: é um erro NOSSO,
// de ter afirmado sucesso sem base.
type InteractionOutcome string

const (
	OutcomeCorrectSuccess InteractionOutcome = "correct_success"
	OutcomeCorrectFailure InteractionOutcome = "correct_failure"
	OutcomeFalseSuccess   InteractionOutcome = "false_success"
	OutcomeFalseFailure   InteractionOutcome = "false_failure"
	OutcomeWrongTarget    InteractionOutcome = "wrong_target"
	OutcomeTimeout        InteractionOutcome = "timeout"
	OutcomeAmbiguous      InteractionOutcome = "ambiguous_target"
)

// InteractionError distingue a RAZÃO da recusa, porque uma recusa correta é um
// resultado desejável e não pode ser confundida com falha de execução.
type InteractionError struct {
	Stage  string // resolve | validate | act | verify
	Reason string
	Detail string
}

func (e *InteractionError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s: %s", e.Stage, e.Reason)
	}
	return fmt.Sprintf("%s: %s (%s)", e.Stage, e.Reason, e.Detail)
}

func refuse(stage, reason, detail string) error {
	return &InteractionError{Stage: stage, Reason: reason, Detail: detail}
}

// candidate é um nó que casou com o seletor, com as propriedades que decidem
// se ele é interagível. Nada aqui é conveniência: cada campo elimina uma classe
// de falso sucesso observada.
type candidate struct {
	NodeID   cdp.NodeID `json:"node_id"`
	Attached bool       `json:"attached"`
	Visible  bool       `json:"visible"`
	Enabled  bool       `json:"enabled"`
	W, H     float64    `json:"-"`
	CX, CY   float64    `json:"-"`
	InView   bool       `json:"in_viewport"`
}

func (c candidate) actionable() bool {
	return c.Attached && c.Visible && c.Enabled && c.W > 0 && c.H > 0
}

// InteractionPolicy executa o ciclo Resolve -> Validate -> Act -> Verify.
//
// Carrega o Runner da Fase 4C em vez de reimplementar prazos: aquilo está
// FECHADO e auditado, e duplicar a lógica de deadline criaria justamente o
// caminho não coberto que a auditoria não veria.
type InteractionPolicy struct {
	R *Runner

	// StableFrames é quantas amostras consecutivas de geometria idêntica
	// contam como "parado". Existe por causa de modal animando: um elemento
	// em movimento aceita o clique numa posição e o processa em outra.
	StableFrames int
	StableGap    time.Duration

	// SettleBudget é quanto tempo a policy REAMOSTRA antes de recusar.
	//
	// V1 não tinha este campo, e foi por isso que reprovou: sondava uma vez no
	// Resolve e no máximo StableFrames*4 vezes no Validate. Nas duas recusas
	// indevidas medidas na etapa 4, o estado correto chegava DEPOIS da janela —
	// botão habilitando em 800 ms, modal parando em ~1,6 s.
	//
	// "Não encontrei agora" e "não existe" são afirmações diferentes, e só a
	// segunda justifica recusa. O budget é o que separa as duas.
	//
	// Ele não substitui a DeadlinePolicy: cada sondagem individual continua sob
	// OpQuery (15 s). Este é o teto do LAÇO, e é deliberadamente menor, porque
	// uma recusa custa o budget inteiro e a suite tem seis cenários que recusam.
	SettleBudget time.Duration
	SettleGap    time.Duration
}

func NewInteractionPolicy(r *Runner) *InteractionPolicy {
	p := &InteractionPolicy{
		R:            r,
		StableFrames: 3,
		StableGap:    60 * time.Millisecond,
		SettleBudget: 5 * time.Second,
		SettleGap:    100 * time.Millisecond,
	}
	// Ponto de controle negativo. Zerar o budget reproduz a V1 — sondagem única
	// no Resolve, janela curta no Validate — SEM reverter o commit, o que
	// mantém o controle executável no MESMO binário que produziu o PASS.
	//
	// Um controle negativo que exige checkout de outra versão quase nunca é
	// reexecutado; este custa uma variável de ambiente.
	if v := os.Getenv(envSettleBudget); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			p.SettleBudget = d
		}
	}
	return p
}

// envSettleBudget sobrescreve InteractionPolicy.SettleBudget (ex.: "0s").
const envSettleBudget = "P5_SETTLE_BUDGET"

// jsCandidates coleta, numa única avaliação, tudo que decide actionability.
//
// Uma avaliação só, de propósito: fazer várias idas ao browser abre janela para
// o DOM mudar entre elas, que é exatamente o defeito (node replacement) que
// esta camada precisa detectar em vez de sofrer.
//
// Cada elemento recebe uma IDENTIDADE (`__p5id`), gravada como propriedade
// expando e não como atributo: atributo entra no DOM serializado e pode casar
// com seletor de terceiro, propriedade não. Um nó substituído é outro objeto,
// logo recebe id novo — e é assim que node replacement fica visível para o
// Validate ANTES do clique, em vez de só para o Verify depois dele.
const jsCandidates = `(() => {
  const reg = (window.__p5 = window.__p5 || {n: 0});
  const out = [];
  for (const el of document.querySelectorAll(SELECTOR)) {
    if (!el.__p5id) { el.__p5id = ++reg.n; }
    const r = el.getBoundingClientRect();
    const cs = getComputedStyle(el);
    const visible = cs.visibility !== 'hidden' && cs.display !== 'none' &&
                    parseFloat(cs.opacity || '1') > 0.01;
    const disabled = el.disabled === true ||
                     el.getAttribute('aria-disabled') === 'true';
    // Ponto de clique na INTERSEÇÃO do elemento com o viewport, não no centro
    // geométrico.
    //
    // O centro geométrico foi medido errado contra o WhatsApp real: a caixa de
    // busca aparecia com left=-70, top=-25, 592x28, ou seja 25 dos 28 px acima
    // da dobra. O teste antigo — "o elemento intersecta o viewport" — dizia
    // visível, e o centro caía em (226, -11), FORA da tela. elementFromPoint
    // devolve null ali, então a policy recusava por OCCLUDED_OR_REPLACED,
    // mandando o diagnóstico para overlay quando a causa era geometria.
    //
    // Pior no caminho ingênuo: o clique ia para uma coordenada fora da tela e
    // acertava outra coisa. Foi wrong_target 5/5 no alvo real, sem starvation
    // nenhuma.
    const ix0 = Math.max(r.left, 0), iy0 = Math.max(r.top, 0);
    const ix1 = Math.min(r.right, innerWidth), iy1 = Math.min(r.bottom, innerHeight);
    const inView = ix1 > ix0 && iy1 > iy0;
    const cx = inView ? (ix0 + ix1) / 2 : r.left + r.width / 2;
    const cy = inView ? (iy0 + iy1) / 2 : r.top + r.height / 2;
    // hit-test no ponto REAL de interação: se o topo naquele ponto não for o
    // elemento nem descendente dele, algo está por cima.
    let hit = false;
    if (inView && r.width > 0 && r.height > 0) {
      const top = document.elementFromPoint(cx, cy);
      hit = !!top && (top === el || el.contains(top));
    }
    out.push({id: el.__p5id, attached: el.isConnected, visible, enabled: !disabled,
              w: r.width, h: r.height, cx, cy, inView, hit});
  }
  return JSON.stringify(out);
})()`

type rawCandidate struct {
	ID       int64   `json:"id"`
	Attached bool    `json:"attached"`
	Visible  bool    `json:"visible"`
	Enabled  bool    `json:"enabled"`
	W        float64 `json:"w"`
	H        float64 `json:"h"`
	CX       float64 `json:"cx"`
	CY       float64 `json:"cy"`
	InView   bool    `json:"inView"`
	Hit      bool    `json:"hit"`
}

func (p *InteractionPolicy) probe(ctx context.Context, sel, label string) ([]rawCandidate, error) {
	var raw string
	expr := "(" + jsSubstituteSelector(jsCandidates, sel) + ")"
	err := p.R.Do(ctx, OpQuery, label, func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Evaluate(expr, &raw))
	})
	if err != nil {
		return nil, err
	}
	var cs []rawCandidate
	if err := json.Unmarshal([]byte(raw), &cs); err != nil {
		return nil, fmt.Errorf("resposta de sondagem ilegivel: %w", err)
	}
	return cs, nil
}

func jsSubstituteSelector(js, sel string) string {
	b, _ := json.Marshal(sel)
	return replaceAll(js, "SELECTOR", string(b))
}

func replaceAll(s, old, new string) string {
	out := ""
	for {
		i := indexOf(s, old)
		if i < 0 {
			return out + s
		}
		out += s[:i] + new
		s = s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// resolveOnce é UMA sondagem: escolhe O alvo entre os candidatos, ou recusa.
//
// §12 é explícito: nunca nodes[0] automaticamente. Filtrar por actionability é
// legítimo — um nó invisível não é candidato — mas se depois disso ainda
// sobrar mais de um, a escolha seria arbitrária e a policy recusa. Escolher em
// silêncio é como se produz wrong_target.
//
// Esta função não espera. Quem espera é Resolve, e a separação é o conserto da
// V1: misturar as duas responsabilidades foi o que fez "ainda não" virar "não".
func (p *InteractionPolicy) resolveOnce(ctx context.Context, sel, label string) (rawCandidate, error) {
	cs, err := p.probe(ctx, sel, label+"/resolve")
	if err != nil {
		return rawCandidate{}, refuse("resolve", "sondagem falhou", err.Error())
	}
	if len(cs) == 0 {
		return rawCandidate{}, refuse("resolve", "NO_MATCH", sel)
	}
	var viable []rawCandidate
	for _, c := range cs {
		if c.Attached && c.Visible && c.Enabled && c.W > 0 && c.H > 0 {
			viable = append(viable, c)
		}
	}
	switch len(viable) {
	case 0:
		return rawCandidate{}, refuse("resolve", "NO_ACTIONABLE_CANDIDATE",
			fmt.Sprintf("%d casaram, nenhum interagivel", len(cs)))
	case 1:
		return viable[0], nil
	default:
		return rawCandidate{}, refuse("resolve", "AMBIGUOUS_TARGET",
			fmt.Sprintf("%d candidatos interagiveis", len(viable)))
	}
}

// jsScrollIntoView rola o único candidato interagível para dentro da vista.
//
// Existe porque o alvo real exigiu: a caixa de busca do WhatsApp aparecia em
// top=-25 com 28 px de altura, ou seja rolada para fora, e nenhum ponto de
// clique honesto existia. Sem rolagem a policy não alcança nada abaixo (ou
// acima) da dobra, e a lista de conversas é justamente uma lista rolável.
//
// Rolar é AÇÃO, não observação, então três cuidados:
//
//  1. só rola quando o alvo NÃO está utilmente visível — rolar sempre mexeria
//     na página a cada sondagem e criaria o movimento que o Validate existe
//     para detectar;
//  2. rola no máximo uma vez por Validate, antes da janela de estabilidade, de
//     modo que a estabilidade seja medida DEPOIS de a página assentar;
//  3. não escolhe alvo: aplica o mesmo filtro de actionability do Resolve e só
//     age se houver exatamente um, senão a rolagem decidiria por baixo dos
//     panos qual é o alvo — que é o defeito que o AMBIGUOUS_TARGET impede.
const jsScrollIntoView = `(() => {
  const els = [...document.querySelectorAll(SELECTOR)].filter(el => {
    const r = el.getBoundingClientRect();
    const cs = getComputedStyle(el);
    const visible = cs.visibility !== 'hidden' && cs.display !== 'none' &&
                    parseFloat(cs.opacity || '1') > 0.01;
    const disabled = el.disabled === true ||
                     el.getAttribute('aria-disabled') === 'true';
    return el.isConnected && visible && !disabled && r.width > 0 && r.height > 0;
  });
  if (els.length !== 1) { return 'skip:' + els.length; }
  els[0].scrollIntoView({block: 'center', inline: 'center'});
  return 'scrolled';
})()`

// scrollTargetIntoView devolve true se chegou a rolar.
func (p *InteractionPolicy) scrollTargetIntoView(ctx context.Context, sel, label string) bool {
	var out string
	err := p.R.Do(ctx, OpAction, label+"/scroll", func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Evaluate(
			jsSubstituteSelector(jsScrollIntoView, sel), &out))
	})
	return err == nil && out == "scrolled"
}

// Resolve reamostra até SettleBudget antes de recusar.
//
// Toda recusa de resolveOnce é retentada, inclusive AMBIGUOUS_TARGET: numa SPA
// real a duplicidade costuma ser o nó antigo ainda no DOM enquanto o novo já
// entrou. Retentar não pode produzir falso sucesso — o desfecho de um alvo
// genuinamente duplicado continua sendo recusa, só que depois do budget.
//
// O laço para cedo se o contexto morreu: nesse caso a espera não é "ainda não",
// é o prazo da operação tendo estourado, e insistir só queima o orçamento do
// experimento.
func (p *InteractionPolicy) Resolve(ctx context.Context, sel, label string) (rawCandidate, error) {
	return p.resolveUntil(ctx, sel, label, time.Now().Add(p.SettleBudget))
}

func (p *InteractionPolicy) resolveUntil(ctx context.Context, sel, label string, deadline time.Time) (rawCandidate, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		c, err := p.resolveOnce(ctx, sel, fmt.Sprintf("%s/r%d", label, attempt))
		if err == nil {
			return c, nil
		}
		lastErr = err
		if ctx.Err() != nil || !time.Now().Add(p.SettleGap).Before(deadline) {
			return rawCandidate{}, lastErr
		}
		time.Sleep(p.SettleGap)
	}
}

// Validate confirma que o alvo continua interagível AGORA e que o ponto de
// clique atinge o próprio elemento.
//
// A checagem de estabilidade é temporal de propósito: geometria idêntica em N
// amostras consecutivas. Um modal animando passa em qualquer verificação
// instantânea e ainda assim recebe o clique na posição errada.
//
// V1 amostrava StableFrames*4 vezes e recusava. Isso confunde "está em
// movimento agora" com "nunca vai parar" — o modal de /modal se move por ~1,6 s
// e a janela fixa acabava em 0,72 s. V2 amostra até o SettleBudget, e só então
// declara UNSTABLE_GEOMETRY. Uma recusa por movimento passa a significar que o
// elemento não parou dentro do budget, que é a afirmação que interessa.
//
// Uma sondagem que falha no meio do laço NÃO encerra a validação: zera o
// contador de estabilidade e continua. Um nó substituído (node replacement)
// some e reaparece, e desistir na primeira ausência era outra forma da mesma
// pressa.
func (p *InteractionPolicy) Validate(ctx context.Context, sel, label string) (rawCandidate, error) {
	deadline := time.Now().Add(p.SettleBudget)

	// Primeiro, esperar que exista alvo. Compartilha o mesmo budget para que a
	// espera total do Validate seja o budget, e não o dobro dele.
	first, err := p.resolveUntil(ctx, sel, label+"/settle", deadline)
	if err != nil {
		return rawCandidate{}, err
	}

	// Se o alvo existe mas não tem ponto de clique utilizável, rolar é a
	// resposta certa — recusar seria over-refusal, a mesma família do FAIL da
	// V1. Uma vez só, e antes da janela de estabilidade.
	if !first.InView || !first.Hit {
		if p.scrollTargetIntoView(ctx, sel, label) {
			if c, err := p.resolveUntil(ctx, sel, label+"/postscroll", deadline); err == nil {
				first = c
			}
		}
	}

	last := first
	stable := 0
	idChanges := 0
	var lastErr error
	for i := 0; stable < p.StableFrames; i++ {
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(p.StableGap)
		c, err := p.resolveOnce(ctx, sel, fmt.Sprintf("%s/v%d", label, i))
		if err != nil {
			lastErr = err
			stable = 0
			continue
		}
		lastErr = nil
		// Identidade JUNTO com geometria. Só geometria era insuficiente e isso
		// foi medido: em node-replacement o nó novo nasce na mesma posição e com
		// o mesmo tamanho, então a V2 o considerava parado, clicava, e quem
		// recusava era o Verify — depois do clique já ter caído no nó errado.
		if c.ID != last.ID {
			idChanges++
		}
		if c.ID == last.ID &&
			c.CX == last.CX && c.CY == last.CY && c.W == last.W && c.H == last.H {
			stable++
		} else {
			stable = 0
		}
		last = c
	}
	if stable < p.StableFrames {
		// A razão importa para o diagnóstico: um nó que sumiu no meio do laço
		// não "está em movimento", e reportar UNSTABLE_GEOMETRY nesse caso
		// mandaria a próxima investigação para o lugar errado.
		if lastErr != nil {
			return rawCandidate{}, lastErr
		}
		if idChanges > 0 {
			// Distinto de UNSTABLE_GEOMETRY de propósito: o elemento não está se
			// movendo, está sendo TROCADO. São causas diferentes e levam a fixes
			// diferentes no alvo — esperar não resolve churn de re-render.
			return rawCandidate{}, refuse("validate", "NODE_CHURN",
				fmt.Sprintf("%d trocas de identidade em %s", idChanges, p.SettleBudget))
		}
		return rawCandidate{}, refuse("validate", "UNSTABLE_GEOMETRY",
			fmt.Sprintf("nao parou em %s", p.SettleBudget))
	}
	if !last.InView {
		return rawCandidate{}, refuse("validate", "OUT_OF_VIEWPORT", "")
	}
	if !last.Hit {
		// Coberto por overlay, ou substituído entre a consulta e agora. Ambos
		// produziriam clique em outro elemento — que o CDP reportaria como
		// sucesso.
		return rawCandidate{}, refuse("validate", "OCCLUDED_OR_REPLACED", "hit-test nao retornou o alvo")
	}
	return last, nil
}

// Act envia input real do browser no ponto validado.
//
// §14 proíbe element.click() e atribuição direta de value: são os caminhos que
// já demonstraram produzir falso sucesso, porque não passam pelo tratamento de
// evento da aplicação nem respeitam overlay.
func (p *InteractionPolicy) Act(ctx context.Context, c rawCandidate, label string) error {
	return p.R.Do(ctx, OpAction, label+"/act", func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			if err := input.DispatchMouseEvent(input.MousePressed, c.CX, c.CY).
				WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
				return err
			}
			return input.DispatchMouseEvent(input.MouseReleased, c.CX, c.CY).
				WithButton(input.Left).WithClickCount(1).Do(ctx)
		}))
	})
}

// Verify espera a póscondição observável.
//
// É o estágio que separa esta camada de uma conveniência: sem ele, um clique
// legítimo que a aplicação ignorou é sucesso. O predicado é do CHAMADOR porque
// só ele sabe o que a ação deveria provocar.
func (p *InteractionPolicy) Verify(ctx context.Context, jsPredicate, label string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var ok bool
		err := p.R.Do(ctx, OpStateProbe, label+"/verify", func(ctx context.Context) error {
			return chromedp.Run(ctx, chromedp.Evaluate("!!("+jsPredicate+")", &ok))
		})
		if err != nil {
			return refuse("verify", "SONDAGEM_FALHOU", err.Error())
		}
		if ok {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return refuse("verify", "POSTCONDITION_NOT_MET", jsPredicate)
}

// Click é o ciclo completo. É a única forma sancionada de interação crítica.
func (p *InteractionPolicy) Click(ctx context.Context, sel, postcondition, label string) error {
	c, err := p.Validate(ctx, sel, label)
	if err != nil {
		return err
	}
	// Reconferência de identidade colada no Act. Não fecha a janela — entre esta
	// sondagem e o DispatchMouseEvent o nó ainda pode ser trocado, e nenhum
	// protocolo elimina isso — mas reduz a janela de ~1 amostra de estabilidade
	// para uma ida ao browser, e torna a troca DETECTÁVEL em vez de silenciosa.
	// O que fecha o caso continua sendo o Verify.
	if again, err := p.resolveOnce(ctx, sel, label+"/preact"); err != nil {
		return refuse("validate", "TARGET_LOST_BEFORE_ACT", err.Error())
	} else if again.ID != c.ID {
		return refuse("validate", "REPLACED_BEFORE_ACT",
			fmt.Sprintf("identidade %d -> %d entre validate e act", c.ID, again.ID))
	} else {
		c = again
	}
	if err := p.Act(ctx, c, label); err != nil {
		return refuse("act", "INPUT_FALHOU", err.Error())
	}
	if postcondition == "" {
		return refuse("verify", "SEM_POSTCONDICAO",
			"interacao critica exige postcondicao observavel")
	}
	return p.Verify(ctx, postcondition, label, 5*time.Second)
}

var _ = dom.Enable
