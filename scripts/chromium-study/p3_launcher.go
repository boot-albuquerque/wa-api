package main

// In-process launcher for N Chromium instances.
//
// Phases 1 and 2 used ONE browser started by the entrypoint script, because the
// question then was "which controller", and a controller that launches its own
// browser would have smuggled its own flag policy into the comparison. Track B
// asks a different question — how many browsers, contexts and pages — so the
// study has to own the launching. The flag list stays CanonicalBrowserProfileV1
// for every instance, so the topology is the only thing that varies.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// launched is one Chromium instance owned by this process.
type launched struct {
	Index      int    `json:"index"`
	Port       int    `json:"port"`
	PID        int    `json:"pid"`
	UserData   string `json:"user_data_dir"`
	HTTPBase   string `json:"http_base"`
	WSURL      string `json:"ws_url"`
	cmd        *exec.Cmd
	logPath    string
	killedByUs bool
}

// launchBrowsers starts n instances on consecutive ports and waits for each to
// answer /json/version.
//
// Each gets its own --user-data-dir: sharing one profile directory between
// instances is not a supported configuration and produces failures that look
// like controller bugs.
func launchBrowsers(n int, profile []string, basePort int) ([]*launched, error) {
	var out []*launched
	for i := 0; i < n; i++ {
		l, err := launchOne(i, profile, basePort+i)
		if err != nil {
			for _, prev := range out {
				prev.Kill()
			}
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

// PersistentProfileDir, when set, makes instance 0 reuse a fixed profile
// directory instead of a temp one.
//
// Session restoration is part of the workload for a logged-in application: a
// runtime that must re-authenticate on every restart is not a runtime. Temp
// profiles are still the default everywhere else, because a benchmark that
// inherits state from the previous run measures the state, not the run.
var PersistentProfileDir string

// ReclaimSingletons controla se o boot apaga SingletonLock/Cookie/Socket do
// perfil persistente. É a variável independente do experimento §21: uma das
// duas explicações vivas para a sessão do WhatsApp morrer após ~9-10 ciclos de
// vida do browser é que este reclaim corrompe o perfil. A outra é invalidação
// do lado do WhatsApp. Só um experimento que LIGUE E DESLIGUE isto separa as
// duas — sem ele, a atribuição seria a mesma correlação que já falhou uma vez
// nesta fase, quando a perda foi atribuída ao SIGKILL.
var ReclaimSingletons = true

func launchOne(idx int, profile []string, port int) (*launched, error) {
	var dir string
	var err error
	if PersistentProfileDir != "" && idx == 0 {
		dir = PersistentProfileDir
		if err = os.MkdirAll(dir, 0o700); err == nil && ReclaimSingletons {
			err = reclaimProfile(dir)
		}
	} else {
		dir, err = os.MkdirTemp("", fmt.Sprintf("chrome-%d-", idx))
	}
	if err != nil {
		return nil, err
	}
	bin := os.Getenv("CHROME_BIN")
	if bin == "" {
		bin = "/usr/bin/chromium"
	}
	args := append([]string{}, profile...)
	args = append(args,
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir="+dir,
		"about:blank",
	)
	logPath := filepath.Join(dir, "chromium.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	// Own process group: killing the group is how an orphaned renderer gets
	// cleaned up. Track G measures exactly that, so it cannot be left to chance.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	l := &launched{
		Index: idx, Port: port, PID: cmd.Process.Pid, UserData: dir,
		HTTPBase: fmt.Sprintf("http://127.0.0.1:%d", port), cmd: cmd, logPath: logPath,
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		ws, _, err := browserWS(l.HTTPBase)
		if err == nil {
			l.WSURL = ws
			return l, nil
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			b, _ := os.ReadFile(logPath)
			return nil, fmt.Errorf("chromium %d exited during startup: %s", idx, tailStr(string(b), 400))
		}
		time.Sleep(150 * time.Millisecond)
	}
	l.Kill()
	return nil, fmt.Errorf("chromium %d never answered CDP on %d", idx, port)
}

// Kill terminates the whole process group, then reaps.
func (l *launched) Kill() {
	if l == nil || l.cmd == nil || l.cmd.Process == nil {
		return
	}
	l.killedByUs = true
	_ = syscall.Kill(-l.cmd.Process.Pid, syscall.SIGKILL)
	_ = l.cmd.Process.Kill()
	_, _ = l.cmd.Process.Wait()
}

// SIGKILL sends SIGKILL to the browser process only, leaving its children to be
// reparented — the fault Track G calls "Chromium SIGKILL".
func (l *launched) SIGKILL() error {
	if l == nil || l.cmd == nil || l.cmd.Process == nil {
		return fmt.Errorf("no process")
	}
	l.killedByUs = true
	return l.cmd.Process.Signal(syscall.SIGKILL)
}

// reclaimProfile clears Chromium's process-singleton lock from a profile whose
// previous owner is gone.
//
// This is REQUIRED for any persistent profile that outlives its container, and
// it is a production finding rather than a test convenience. The lock is a
// symlink named after hostname-PID:
//
//	SingletonLock -> 410f829ba5be-171
//
// Chromium compares that hostname to its own. A container — or a Kubernetes pod
// — gets a new hostname on every start, so the check always concludes "another
// computer holds this profile" and refuses to boot:
//
//	The profile appears to be in use by another Chromium process (171) on
//	another computer (410f829ba5be).
//
// Consequence for the runtime: a pod that restarts with a mounted session volume
// will NOT come back up unless it reclaims the profile first. Clearing the lock
// is only safe because the design gives one runtime exclusive ownership of a
// profile (the lease of ADR-0005). It would be unsafe for a shared volume, and
// nothing here makes it safe — the exclusivity does.
func reclaimProfile(dir string) error {
	for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(p); err == nil {
			if err := os.Remove(p); err != nil {
				return fmt.Errorf("reclaim %s: %w", name, err)
			}
			fmt.Fprintf(os.Stderr, "reclaimed stale %s from persistent profile\n", name)
		}
	}
	return nil
}

func killAll(ls []*launched) {
	for _, l := range ls {
		l.Kill()
		// A persistent profile is deliberately NOT removed: it holds the session.
		if l.UserData != PersistentProfileDir {
			_ = os.RemoveAll(l.UserData)
		}
	}
}

// gracefulStop sends SIGTERM and waits, so Chromium flushes its profile.
//
// SIGKILL leaves IndexedDB and Local Storage mid-write, which is exactly how a
// paired session gets lost and a QR scan gets wasted. Track I will measure what
// SIGKILL costs; this path exists so normal shutdown does not pay it.
func gracefulStop(l *launched) {
	if l == nil || l.cmd == nil || l.cmd.Process == nil {
		return
	}
	l.killedByUs = true
	_ = l.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _, _ = l.cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = syscall.Kill(-l.cmd.Process.Pid, syscall.SIGKILL)
	}
}

func tailStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
