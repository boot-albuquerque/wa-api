package main

// Fase 4C §21 — quantos ciclos de vida do browser a sessão do WhatsApp
// sobrevive, e por quê.
//
// A ablação de §17 refutou a explicação anterior: a perda de credencial NÃO é
// causada por SIGKILL, porque reproduziu sob parada graciosa. Sobraram duas
// explicações, e elas exigem respostas opostas em produção:
//
//	(1) o WhatsApp invalida o dispositivo por religações repetidas
//	    -> bloqueador real: um Deployment reinicia pods por rotina
//	(2) o harness corrompe o perfil ao apagar os arquivos Singleton a cada boot
//	    -> o achado evapora e o recovery de 10,4 s vale como está
//
// Este experimento é a variável (2) ligada e desligada. Nada mais muda: mesma
// parada graciosa, mesmo perfil, mesmo alvo. Se a sessão sobreviver muito além
// dos ~9-10 ciclos observados com reclaim ligado, a causa é o harness.
//
// Uma iteração é deliberadamente MÍNIMA — subir, verificar, descer. Sem SIGKILL
// e sem segunda aba, porque o Track B já mostrou que abrir uma segunda aba
// derruba a sessão da primeira, e isso contaminaria a contagem.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// LifecycleStop escolhe COMO o browser é desligado a cada iteração.
//
//	sigterm:      SIGTERM no processo principal e espera (o gracefulStop atual)
//	browserclose: Browser.close via CDP, que é o desligamento que o Chromium
//	              entende como limpo e no qual ele descarrega o IndexedDB
//
// É a variável independente do experimento que separa "o WhatsApp invalida o
// dispositivo" de "o desligamento do harness perde a escrita da sessão". A
// pista que motivou isto: o perfil encolheu de 123 para 118 MB exatamente na
// primeira degradação, e invalidação remota não reduz armazenamento local.
var LifecycleStop = "sigterm"

// closeBrowserViaCDP desliga o Chromium pelo protocolo, pela porta que o
// chromedp abre para isso.
//
// A primeira tentativa mandava browser.Close() direto no executor do browser e
// foi recusada em 6/6 iterações — browser.go:185 devolve "to close the browser
// gracefully, use chromedp.Cancel". A corrida inteira caiu no SIGTERM de
// fallback e virou réplica do braço antigo em vez de comparação.
//
// Cancel é o caminho sancionado, e a leitura do código mostra por quê:
//
//	chromedp.go:134  c.first = c.Browser == nil   -> true no 1o ctx do allocator
//	chromedp.go:252  graceful := c.first && c.Browser != nil
//	chromedp.go:254  close(c.Browser.closingGracefully)  -> destrava a guarda
//	chromedp.go:255  execute(browser.CommandClose)
//
// Duas pré-condições que o chamador precisa respeitar, e ambas valem aqui: o
// contexto tem de ser o PRIMEIRO criado a partir do allocator (senão first é
// false e o caminho vira cancelamento seco), e o browser já tem de estar
// materializado por um Run anterior — que é o papel do primeTab.
// A sonda §22 fechou a questão por medida direta, e contra a minha aposta:
//
//	CDP cru          Browser.close aceito em 2 ms, 10 -> 0 processos em <1 s
//	chromedp.Cancel  sem erro em 19 ms, todos os processos vivos após 15 s
//
// Se o Cancel tivesse enviado o comando, os processos teriam morrido em menos
// de um segundo. Não morreram: com RemoteAllocator o Cancel cai no ramo seco de
// c.first == false e devolve nil sem nada ir ao fio. Os 19 ms eram teardown de
// goroutines — ler latência como prova de envio foi erro meu.
//
// Por isso o desligamento passa pela conexão administrativa crua, que fala com
// o BROWSER e não depende de nenhuma heurística de biblioteca.
func closeBrowserViaCDP(wsURL string) error {
	admin, err := dialAdmin(wsURL)
	if err != nil {
		return fmt.Errorf("conexao administrativa: %w", err)
	}
	if _, err := admin.c.call("", "Browser.close", nil); err != nil {
		return fmt.Errorf("Browser.close recusado: %w", err)
	}
	return nil
}

type lifecycleIteration struct {
	Iter          int     `json:"iteration"`
	Class         string  `json:"class"`
	AppReadySec   float64 `json:"app_ready_sec"`
	ProfileBytes  int64   `json:"profile_bytes"`
	SingletonsPre int     `json:"singletons_present_before_boot"`
	StopVia       string  `json:"stopped_via,omitempty"`
	Note          string  `json:"note,omitempty"`
}

// waitExit espera o processo sair por conta própria após Browser.close.
//
// Sem isto o experimento não distingue nada: se o SIGTERM entrasse logo em
// seguida, toda iteração teria o desligamento antigo por baixo e o resultado
// seria o mesmo por construção.
func waitExit(l *launched, d time.Duration) bool {
	if l == nil || l.cmd == nil || l.cmd.Process == nil {
		return true
	}
	done := make(chan struct{})
	go func() { _, _ = l.cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
		l.killedByUs = true
		return true
	case <-time.After(d):
		return false
	}
}

// RunSessionLifecycle sobe e desce o browser até a sessão degradar ou até maxIter.
func RunSessionLifecycle(maxIter int, outPath string) error {
	wd := StartWatchdog("session-lifecycle", time.Duration(maxIter)*90*time.Second+2*time.Minute)
	defer wd.Stop()
	r := NewRunner()

	dir := waSessionDir()
	var iters []lifecycleIteration
	degradedAt, logoutAt := 0, 0

	for i := 1; i <= maxIter; i++ {
		if wd.Expired() {
			break
		}
		it := lifecycleIteration{Iter: i, SingletonsPre: countSingletons(dir)}

		PersistentProfileDir = dir
		browsers, err := launchBrowsers(1, waProfile(), 9980)
		PersistentProfileDir = ""
		if err != nil {
			it.Class = "LAUNCH_ERROR"
			it.Note = err.Error()
			iters = append(iters, it)
			break
		}
		time.Sleep(2 * time.Second)

		start := time.Now()
		closedViaCDP := false
		func() {
			alloc, cancelAlloc := chromedp.NewRemoteAllocator(wd.Ctx(), browsers[0].WSURL)
			defer cancelAlloc()
			tab, cancelTab := chromedp.NewContext(alloc)
			defer cancelTab()

			if err := primeTab(tab); err != nil {
				it.Class = "TAB_ERROR"
				it.Note = err.Error()
				return
			}
			if err := navigateTarget(tab, r, fmt.Sprintf("life%d/navigate", i)); err != nil {
				it.Class = "NAV_ERROR"
				it.Note = err.Error()
				return
			}
			readyErr := waitAppReady(tab, r, fmt.Sprintf("life%d/ready", i))
			it.AppReadySec = time.Since(start).Seconds()

			snap := snapshot(tab, r, fmt.Sprintf("life%d/state", i))

			switch {
			case readyErr == nil && snap.Class == classAppReady:
				it.Class = string(classAppReady)
			case snap.HasQR || snap.Class == classLoginRequired:
				it.Class = string(classLoginRequired)
			default:
				it.Class = string(snap.Class)
				if readyErr != nil {
					it.Note = readyErr.Error()
				}
			}
		}()

		// O desligamento pelo protocolo vai por conexão PRÓPRIA, depois que as
		// sondas terminaram e o contexto da aba já foi desfeito. A conexão
		// administrativa é independente da do chromedp, então não importa que
		// aquela já tenha caído.
		if LifecycleStop == "browserclose" {
			if err := closeBrowserViaCDP(browsers[0].WSURL); err != nil {
				it.Note = "Browser.close falhou: " + err.Error()
			} else {
				closedViaCDP = true
			}
		}

		// Com Browser.close bem-sucedido, o processo deve sair sozinho. O
		// gracefulStop só entra se ele NÃO sair — e nesse caso o registro diz
		// que o desligamento pelo protocolo não bastou, o que é resultado.
		if closedViaCDP && waitExit(browsers[0], 15*time.Second) {
			it.StopVia = "browser.close"
		} else {
			gracefulStop(browsers[0])
			if closedViaCDP {
				it.StopVia = "browser.close+sigterm"
			} else {
				it.StopVia = "sigterm"
			}
		}
		it.ProfileBytes = dirSize(dir)
		iters = append(iters, it)

		fmt.Fprintf(os.Stderr, "iter %2d: %-16s ready=%5.1fs singletons_antes=%d perfil=%dMB\n",
			i, it.Class, it.AppReadySec, it.SingletonsPre, it.ProfileBytes/1048576)

		if it.Class == string(classLoginRequired) && logoutAt == 0 {
			logoutAt = i
		}
		if it.Class != string(classAppReady) && degradedAt == 0 {
			degradedAt = i
		}
		// Logout é terminal: a credencial não volta, e continuar só gastaria
		// tempo medindo o mesmo estado.
		if logoutAt != 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}

	healthy := 0
	for _, it := range iters {
		if it.Class == string(classAppReady) {
			healthy++
		}
	}

	fmt.Fprintf(os.Stderr,
		"\nSESSION LIFECYCLE (reclaim=%v): %d/%d iteracoes saudaveis | 1a degradacao=%d | logout=%d\n",
		ReclaimSingletons, healthy, len(iters), degradedAt, logoutAt)

	return writeJSON(outPath, map[string]any{
		"experiment":          "session-lifecycle-vs-reclaim",
		"reclaim_singletons":  ReclaimSingletons,
		"stop_mode":           "graceful",
		"max_iterations":      maxIter,
		"iterations_run":      len(iters),
		"healthy_iterations":  healthy,
		"first_degradation":   degradedAt,
		"logout_at_iteration": logoutAt,
		"iterations":          iters,
		"watchdog":            wd.Verdict(r.Log),
		"operations":          r.Log.Records(),
		"started_utc":         time.Now().UTC().Format(time.RFC3339),
	})
}

var _ = context.Background
