package main

// Fase 4C §7 — o teste do próprio DeadlinePolicy.
//
// Antes de tocar no WhatsApp, a policy precisa ser demonstrada contra páginas
// que deliberadamente NUNCA respondem. Sem isso, "agora tem timeout" é uma
// afirmação sobre o código, não sobre o comportamento — e foi exatamente esse
// tipo de afirmação que falhou três vezes na Fase 4B.
//
// Três formas distintas de travar, uma por classe de operação, porque elas
// bloqueiam em camadas diferentes do browser:
//
//	/hang       o servidor aceita a conexão e nunca responde  -> trava no Navigate
//	/noelement  página carrega, o seletor jamais existe        -> trava no Query
//	/promise    página carrega, a Promise jamais resolve       -> trava no Evaluate
//
// Critério: cada operação termina dentro do prazo + overhead pequeno. Se
// qualquer uma exceder, o modo sai com erro e a fase PARA.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// hangServer serve as três armadilhas.
type hangServer struct {
	ln  net.Listener
	srv *http.Server
}

func startHangServer() (*hangServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()

	// Aceita a conexão e nunca escreve. O browser fica preso no carregamento —
	// não é erro de rede, é ausência de resposta, que é o caso difícil.
	mux.HandleFunc("/hang", func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	// Página válida e completa; o seletor procurado simplesmente não existe.
	mux.HandleFunc("/noelement", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><body><div id=other>ok</div></body></html>`)
	})
	// Página válida cujo JS devolve uma Promise que nunca resolve.
	mux.HandleFunc("/promise", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><body><div id=ready>ok</div></body></html>`)
	})
	s := &hangServer{ln: ln, srv: &http.Server{Handler: mux}}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func (s *hangServer) URL() string { return "http://" + s.ln.Addr().String() }
func (s *hangServer) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
}

type selfTestCase struct {
	Name        string  `json:"name"`
	Kind        string  `json:"operation"`
	DeadlineMS  int64   `json:"deadline_ms"`
	ObservedMS  int64   `json:"observed_ms"`
	OverheadMS  int64   `json:"overhead_ms"`
	TimedOut    bool    `json:"timed_out_as_expected"`
	WithinBound bool    `json:"within_bound"`
	Verdict     string  `json:"verdict"`
	Ratio       float64 `json:"observed_over_deadline"`
}

// RunDeadlineSelfTest executa §7.
//
// Usa prazos curtos e próprios: o que está sob teste é o MECANISMO — se o prazo
// é honrado — e não os valores de produção. Um teste com os prazos reais levaria
// minutos para provar a mesma coisa.
func RunDeadlineSelfTest(outPath string) error {
	const overheadBudget = 2500 * time.Millisecond

	policy := DeadlinePolicy{
		Navigate:      3 * time.Second,
		Query:         3 * time.Second,
		Evaluate:      3 * time.Second,
		Action:        3 * time.Second,
		StateProbe:    2 * time.Second,
		RecoveryProbe: 2 * time.Second,
		Shutdown:      3 * time.Second,
	}

	srv, err := startHangServer()
	if err != nil {
		return err
	}
	defer srv.Close()

	wd := StartWatchdog("deadline-selftest", 2*time.Minute)
	defer wd.Stop()

	browsers, err := launchBrowsers(1, CanonicalBrowserProfileV1, 9950)
	if err != nil {
		return err
	}
	defer killAll(browsers)
	time.Sleep(2 * time.Second)

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(wd.Ctx(), browsers[0].WSURL)
	defer cancelAlloc()

	runner := &Runner{Policy: policy, Log: NewOpLog()}
	var cases []selfTestCase

	check := func(name string, kind OpKind, f func(context.Context) error) {
		// Cada caso ganha uma aba nova: uma aba deixada num estado travado
		// contaminaria o caso seguinte, e o objetivo é medir o prazo, não a
		// herança de estado.
		tab, cancelTab := chromedp.NewContext(alloc)
		defer cancelTab()

		start := time.Now()
		err := runner.Do(tab, kind, name, f)
		observed := time.Since(start)
		deadline := policy.For(kind)
		overhead := observed - deadline

		c := selfTestCase{
			Name: name, Kind: string(kind),
			DeadlineMS: deadline.Milliseconds(),
			ObservedMS: observed.Milliseconds(),
			OverheadMS: overhead.Milliseconds(),
			TimedOut:   err != nil,
			Ratio:      observed.Seconds() / deadline.Seconds(),
		}
		c.WithinBound = overhead <= overheadBudget
		switch {
		case !c.TimedOut:
			// A armadilha não prendeu: o caso não testou nada.
			c.Verdict = "INVALID_DID_NOT_BLOCK"
		case !c.WithinBound:
			c.Verdict = "FAIL_OVERSHOOT"
		default:
			c.Verdict = "PASS"
		}
		cases = append(cases, c)
		fmt.Fprintf(os.Stderr, "%-26s %-10s deadline=%5dms observed=%5dms overhead=%+5dms  %s\n",
			name, kind, c.DeadlineMS, c.ObservedMS, c.OverheadMS, c.Verdict)
	}

	check("navigate-never-responds", OpNavigate, func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Navigate(srv.URL()+"/hang"))
	})

	check("query-element-never-exists", OpQuery, func(ctx context.Context) error {
		return chromedp.Run(ctx,
			chromedp.Navigate(srv.URL()+"/noelement"),
			chromedp.WaitReady("#never-appears", chromedp.ByQuery))
	})

	check("evaluate-promise-never-resolves", OpEvaluate, func(ctx context.Context) error {
		// runtime.Evaluate direto: é o awaitPromise que trava, e chromedp.Evaluate
		// não o liga por padrão para uma expressão sem destino tipado.
		return chromedp.Run(ctx,
			chromedp.Navigate(srv.URL()+"/promise"),
			chromedp.ActionFunc(func(ctx context.Context) error {
				_, _, err := runtime.Evaluate(`new Promise(() => {})`).
					WithAwaitPromise(true).WithReturnByValue(true).Do(ctx)
				return err
			}))
	})

	check("poll-condition-never-true", OpStateProbe, func(ctx context.Context) error {
		return chromedp.Run(ctx,
			chromedp.Navigate(srv.URL()+"/promise"),
			// Timeout de polling generoso de propósito: quem deve disparar aqui é
			// o prazo da policy, não o do Poll. Se o Poll vencesse antes, o teste
			// estaria medindo o chromedp e não a DeadlinePolicy.
			chromedp.Poll(`false`, nil, chromedp.WithPollingTimeout(60*time.Second)))
	})

	pass := true
	for _, c := range cases {
		if c.Verdict != "PASS" {
			pass = false
		}
	}
	verdict := "PASS"
	if !pass {
		verdict = "FAIL"
	}

	out := map[string]any{
		"experiment":        "deadline-policy-selftest",
		"verdict":           verdict,
		"policy_under_test": policy,
		"overhead_budget_ms": overheadBudget.Milliseconds(),
		"cases":             cases,
		"watchdog":          wd.Verdict(runner.Log),
		"operations":        runner.Log.Records(),
		"started_utc":       time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeJSON(outPath, out); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\nHARNESS DEADLINE AUDIT: %s\n", verdict)
	if !pass {
		return fmt.Errorf("deadline self-test FAILED — a fase para aqui, conforme §7")
	}
	return nil
}
