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

type lifecycleIteration struct {
	Iter          int     `json:"iteration"`
	Class         string  `json:"class"`
	AppReadySec   float64 `json:"app_ready_sec"`
	ProfileBytes  int64   `json:"profile_bytes"`
	SingletonsPre int     `json:"singletons_present_before_boot"`
	Note          string  `json:"note,omitempty"`
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

		// Sempre parada graciosa: a forma de parada NÃO é a variável aqui, e a
		// ablação anterior já mostrou que ela não explica a perda.
		gracefulStop(browsers[0])
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
