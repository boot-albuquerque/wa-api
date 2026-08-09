package main

// Fase 4C §3–§6 — DeadlinePolicy central, watchdog e instrumentação.
//
// O harness foi declarado NÃO CONFIÁVEL na Fase 4B, e com razão: três execuções
// travaram pela mesma causa — chamada CDP sem prazo — e cada correção pontual
// permitiu que o defeito voltasse no arquivo seguinte. A lição não é "faltou um
// timeout ali"; é que o prazo não pode ser responsabilidade do call site.
//
// Aqui o prazo passa a ser propriedade do TIPO de operação. Nenhum caminho novo
// pode esquecer de aplicá-lo, porque não existe função pública que execute algo
// remoto sem derivar um contexto da policy.
//
// Três camadas, e cada uma pega o que a anterior deixa passar:
//
//	1. DeadlinePolicy  prazo por classe de operação
//	2. Watchdog        orçamento do experimento inteiro
//	3. OpLog           registro de toda operação, para saber ONDE parou

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// DeadlinePolicy é o prazo de cada classe de operação remota.
//
// Os valores não são arbitrários: derivam do que as fases anteriores mediram.
// Navigate cobre o pior caso observado no alvo real (~5 s no p95 da SPA de
// referência, com folga). StateProbe é curto de propósito — uma aba que não
// responde em 5 s É o resultado, não uma espera a ser estendida.
type DeadlinePolicy struct {
	Navigate      time.Duration `json:"navigate"`
	Query         time.Duration `json:"query"`
	Evaluate      time.Duration `json:"evaluate"`
	Action        time.Duration `json:"action"`
	StateProbe    time.Duration `json:"state_probe"`
	RecoveryProbe time.Duration `json:"recovery_probe"`
	Shutdown      time.Duration `json:"shutdown"`
}

// DefaultDeadlines é a policy usada por todo experimento da Fase 4C.
var DefaultDeadlines = DeadlinePolicy{
	Navigate:      30 * time.Second,
	Query:         15 * time.Second,
	Evaluate:      10 * time.Second,
	Action:        15 * time.Second,
	StateProbe:    5 * time.Second,
	RecoveryProbe: 2 * time.Second,
	Shutdown:      10 * time.Second,
}

// OpKind identifica a classe de operação e, com ela, o prazo aplicável.
type OpKind string

const (
	OpNavigate      OpKind = "Navigate"
	OpQuery         OpKind = "Query"
	OpEvaluate      OpKind = "Evaluate"
	OpAction        OpKind = "Action"
	OpStateProbe    OpKind = "StateProbe"
	OpRecoveryProbe OpKind = "RecoveryProbe"
	OpShutdown      OpKind = "Shutdown"
)

func (p DeadlinePolicy) For(k OpKind) time.Duration {
	switch k {
	case OpNavigate:
		return p.Navigate
	case OpQuery:
		return p.Query
	case OpEvaluate:
		return p.Evaluate
	case OpAction:
		return p.Action
	case OpStateProbe:
		return p.StateProbe
	case OpRecoveryProbe:
		return p.RecoveryProbe
	case OpShutdown:
		return p.Shutdown
	}
	return p.Evaluate
}

// OpRecord é uma operação executada, com o que ela prometeu e o que cumpriu.
//
// Nenhum campo carrega conteúdo da página: só classe, rótulo, tempos e
// resultado. É o suficiente para localizar onde um experimento parou sem
// registrar nada da conta sob teste.
type OpRecord struct {
	Op         string `json:"operation"`
	Label      string `json:"label,omitempty"`
	StartMS    int64  `json:"start_ms"`
	DurationMS int64  `json:"duration_ms"`
	DeadlineMS int64  `json:"deadline_ms"`
	Result     string `json:"result"` // ok | deadline_exceeded | error
	Err        string `json:"error,omitempty"`
}

// OpLog acumula o histórico de operações de um experimento.
type OpLog struct {
	mu      sync.Mutex
	t0      time.Time
	records []OpRecord
	echo    bool
}

func NewOpLog() *OpLog {
	return &OpLog{t0: time.Now(), echo: os.Getenv("OP_TRACE") != ""}
}

func (l *OpLog) add(r OpRecord) {
	l.mu.Lock()
	l.records = append(l.records, r)
	l.mu.Unlock()
	if l.echo {
		fmt.Fprintf(os.Stderr, "  op=%-14s %-22s %6dms / %6dms  %s%s\n",
			r.Op, r.Label, r.DurationMS, r.DeadlineMS, r.Result, errSuffix(r.Err))
	}
}

func errSuffix(e string) string {
	if e == "" {
		return ""
	}
	if len(e) > 70 {
		e = e[:70]
	}
	return "  (" + e + ")"
}

func (l *OpLog) Records() []OpRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]OpRecord, len(l.records))
	copy(out, l.records)
	return out
}

// Timeouts conta quantas operações estouraram o prazo — o número que diz se um
// experimento foi conduzido ou apenas sobreviveu.
func (l *OpLog) Timeouts() int {
	n := 0
	for _, r := range l.Records() {
		if r.Result == "deadline_exceeded" {
			n++
		}
	}
	return n
}

// Runner executa operações remotas sob a policy. É o ÚNICO caminho permitido:
// um experimento que chame chromedp.Run diretamente está fora da política e a
// auditoria de §4 o trata como defeito.
type Runner struct {
	Policy DeadlinePolicy
	Log    *OpLog
}

func NewRunner() *Runner {
	return &Runner{Policy: DefaultDeadlines, Log: NewOpLog()}
}

// Do executa f com o prazo da classe k e registra o resultado.
//
// A função recebe um contexto JÁ limitado; ela não pode escolher esperar mais.
// Retorna o erro de f, ou o de prazo excedido — distinguidos no registro para
// que "a página respondeu com erro" nunca seja confundido com "a página não
// respondeu".
func (r *Runner) Do(parent context.Context, k OpKind, label string, f func(context.Context) error) error {
	deadline := r.Policy.For(k)
	ctx, cancel := context.WithTimeout(parent, deadline)
	defer cancel()

	start := time.Now()
	err := f(ctx)
	dur := time.Since(start)

	rec := OpRecord{
		Op:         string(k),
		Label:      label,
		StartMS:    start.Sub(r.Log.t0).Milliseconds(),
		DurationMS: dur.Milliseconds(),
		DeadlineMS: deadline.Milliseconds(),
		Result:     "ok",
	}
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		rec.Result = "deadline_exceeded"
		if err != nil {
			rec.Err = err.Error()
		}
	case err != nil:
		rec.Result = "error"
		rec.Err = err.Error()
	}
	r.Log.add(rec)
	if rec.Result == "deadline_exceeded" {
		return fmt.Errorf("%s(%s): deadline of %s exceeded", k, label, deadline)
	}
	return err
}

// Watchdog é o orçamento do experimento inteiro (§5).
//
// Existe porque prazos por operação não bastam: um laço que repete operações
// que respeitam o prazo pode consumir horas sem violar nenhum deles. Foi
// exatamente essa a forma do travamento de 38 minutos num teste de 8.
type Watchdog struct {
	Budget   time.Duration
	start    time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	name     string
	expired  bool
	expireMu sync.Mutex
}

func StartWatchdog(name string, budget time.Duration) *Watchdog {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	w := &Watchdog{Budget: budget, start: time.Now(), ctx: ctx, cancel: cancel, name: name}
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			w.expireMu.Lock()
			w.expired = true
			w.expireMu.Unlock()
			fmt.Fprintf(os.Stderr,
				"WATCHDOG: experimento %q excedeu o orcamento de %s — resultado marcado INVALID/TIMEOUT\n",
				name, budget)
		}
	}()
	return w
}

// Ctx é o contexto raiz do experimento. Toda operação deriva dele, então o
// estouro do orçamento cancela o que estiver em voo.
func (w *Watchdog) Ctx() context.Context { return w.ctx }

func (w *Watchdog) Stop() { w.cancel() }

func (w *Watchdog) Expired() bool {
	w.expireMu.Lock()
	defer w.expireMu.Unlock()
	return w.expired
}

func (w *Watchdog) Elapsed() time.Duration { return time.Since(w.start) }

// Verdict é o veredito de um experimento sob watchdog.
func (w *Watchdog) Verdict(log *OpLog) map[string]any {
	status := "COMPLETED"
	if w.Expired() {
		status = "INVALID_TIMEOUT"
	}
	return map[string]any{
		"experiment":       w.name,
		"status":           status,
		"budget_sec":       w.Budget.Seconds(),
		"elapsed_sec":      w.Elapsed().Seconds(),
		"operations":       len(log.Records()),
		"deadline_exceeded": log.Timeouts(),
	}
}

func writeJSON(path string, v any) error {
	b, _ := json.MarshalIndent(v, "", "  ")
	if path == "" {
		fmt.Println(string(b))
		return nil
	}
	return os.WriteFile(path, b, 0o644)
}
