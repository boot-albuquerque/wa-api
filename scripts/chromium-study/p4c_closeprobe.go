package main

// Fase 4C §22 — o Browser.close chega ao fio, e o que ele derruba?
//
// A corrida com -stop browserclose registrou stopped_via=browser.close+sigterm:
// o chromedp.Cancel retornou sem erro em 19 ms, mas o processo que o harness
// observa continuou vivo por 15 s. Três leituras ficaram de pé:
//
//	(a) o comando foi enviado e o Chromium demora mais que 15 s para sair
//	(b) c.first era false, o Cancel só cancelou o contexto, nada foi ao fio
//	(c) o comando foi aceito, mas encerra a SESSÃO do browser sem derrubar o
//	    processo que o harness segura — que é lançado externamente e tem grupo
//	    de processos próprio
//
// Nenhuma delas precisa do WhatsApp para ser testada, e escolher entre elas por
// plausibilidade seria repetir o erro que esta fase já cometeu duas vezes.
//
// Esta sonda tira o chromedp do caminho: envia Browser.close numa conexão CDP
// CRUA e observa três coisas que as separam — a resposta do próprio comando, o
// destino do processo principal, e a contagem de processos Chromium antes e
// depois. Se o comando responde e os processos somem mas o pai fica, é (c). Se
// o comando responde e tudo morre devagar, é (a). Se o comando falha, era (b).

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"

	"chromium-study/metrics"
)

type closeProbeResult struct {
	CommandSent      bool   `json:"browser_close_sent"`
	CommandOK        bool   `json:"browser_close_acknowledged"`
	CommandMS        int64  `json:"browser_close_ms"`
	CommandErr       string `json:"browser_close_error,omitempty"`
	Response         string `json:"raw_response,omitempty"`
	ProcsBefore      int    `json:"chromium_procs_before"`
	ProcsAfter1s     int    `json:"chromium_procs_after_1s"`
	ProcsAfter5s     int    `json:"chromium_procs_after_5s"`
	ProcsAfter20s    int    `json:"chromium_procs_after_20s"`
	MainExited       bool   `json:"main_process_exited"`
	MainExitSec      float64 `json:"main_process_exit_sec"`
	MainAliveBySig   bool   `json:"main_alive_by_signal0"`
	Verdict          string `json:"verdict"`
}

// RunCloseProbe executa §22.
func RunCloseProbe(outPath string) error {
	wd := StartWatchdog("browser-close-probe", 3*time.Minute)
	defer wd.Stop()

	browsers, err := launchBrowsers(1, CanonicalBrowserProfileV1, 9990)
	if err != nil {
		return err
	}
	defer killAll(browsers)
	time.Sleep(2 * time.Second)

	res := &closeProbeResult{ProcsBefore: metrics.ChromiumProcs()["total"]}
	pid := browsers[0].cmd.Process.Pid

	admin, err := dialAdmin(browsers[0].WSURL)
	if err != nil {
		return fmt.Errorf("nao conectou no endpoint do browser: %w", err)
	}

	// O comando cru. Sem chromedp, sem guarda de biblioteca: se o Chromium
	// responder, ele chegou ao fio, e a leitura (b) morre.
	res.CommandSent = true
	start := time.Now()
	raw, cerr := admin.c.call("", "Browser.close", nil)
	res.CommandMS = time.Since(start).Milliseconds()
	if cerr != nil {
		res.CommandErr = cerr.Error()
	} else {
		res.CommandOK = true
		if b, e := json.Marshal(raw); e == nil {
			res.Response = tailStr(string(b), 200)
		}
	}

	// O destino do processo principal, observado por sinal 0 — que pergunta se
	// o processo existe sem interferir nele.
	exitStart := time.Now()
	for time.Since(exitStart) < 20*time.Second {
		if err := syscall.Kill(pid, 0); err != nil {
			res.MainExited = true
			res.MainExitSec = time.Since(exitStart).Seconds()
			break
		}
		switch d := time.Since(exitStart); {
		case d > 19*time.Second && res.ProcsAfter20s == 0:
			res.ProcsAfter20s = metrics.ChromiumProcs()["total"]
		case d > 5*time.Second && res.ProcsAfter5s == 0:
			res.ProcsAfter5s = metrics.ChromiumProcs()["total"]
		case d > time.Second && res.ProcsAfter1s == 0:
			res.ProcsAfter1s = metrics.ChromiumProcs()["total"]
		}
		time.Sleep(250 * time.Millisecond)
	}
	if res.ProcsAfter20s == 0 {
		res.ProcsAfter20s = metrics.ChromiumProcs()["total"]
	}
	res.MainAliveBySig = syscall.Kill(pid, 0) == nil

	switch {
	case !res.CommandOK:
		res.Verdict = "B_COMANDO_NAO_ACEITO"
	case res.MainExited:
		res.Verdict = "A_COMANDO_ACEITO_PROCESSO_SAI"
	case res.ProcsAfter20s < res.ProcsBefore:
		res.Verdict = "C_SESSAO_ENCERRA_PROCESSO_PAI_FICA"
	default:
		res.Verdict = "D_COMANDO_ACEITO_NADA_MORRE"
	}

	fmt.Fprintf(os.Stderr,
		"\nBROWSER.CLOSE PROBE: %s\n  aceito=%v (%dms) err=%q\n  procs: antes=%d 1s=%d 5s=%d 20s=%d\n  processo principal saiu=%v em %.1fs (vivo agora=%v)\n",
		res.Verdict, res.CommandOK, res.CommandMS, res.CommandErr,
		res.ProcsBefore, res.ProcsAfter1s, res.ProcsAfter5s, res.ProcsAfter20s,
		res.MainExited, res.MainExitSec, res.MainAliveBySig)

	return writeJSON(outPath, map[string]any{
		"experiment":  "browser-close-wire-probe",
		"result":      res,
		"watchdog":    wd.Verdict(NewOpLog()),
		"started_utc": time.Now().UTC().Format(time.RFC3339),
	})
}
