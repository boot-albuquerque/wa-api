// Package settings reads and writes this account's own preferences: whether
// media auto-downloads, and whether background sync is on.
//
// WHY IT VERIFIES WHEN THE REFERENCE DOES NOT. In the reference every one of
// these setters ends with `return flag` — it hands back the value it was asked
// for, whether or not the page took it. That is a silent success, which
// invariant 14 forbids, so every write here reads the value back and reports
// what the page actually holds.
//
// THE REDUNDANT WRITE IS REPORTED, NOT HIDDEN. The reference short-circuits when
// the setting is already where it was asked to go. This module does the same,
// because H55 measured a redundant request being the CAUSE of a failure — but it
// says so in Outcome.Changed instead of returning a value indistinguishable from
// a real write.
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// Kind is one auto-download category.
type Kind string

// The four categories the page keeps separately. Measured 2026-08-22 against
// the lab account, which held them at MIXED values (audio on, documents off,
// photos on, videos off) — which is what makes a per-kind reader provable.
const (
	KindAudio     Kind = "audio"
	KindDocuments Kind = "documents"
	KindPhotos    Kind = "photos"
	KindVideos    Kind = "videos"
)

// Kinds is every category, in a fixed order so output compares run to run.
var Kinds = []Kind{KindAudio, KindDocuments, KindPhotos, KindVideos}

var (
	// ErrUnknownKind is a category this page does not keep.
	ErrUnknownKind = fmt.Errorf("settings: unknown auto-download kind")
	// ErrRead is the page refusing or failing a read.
	ErrRead = fmt.Errorf("settings: the page refused the read")
	// ErrNotTaken is the postcondition: the page was asked and did not move.
	//
	// IT IS ITS OWN ERROR because the repair differs. A read that failed is a
	// broken page; a write the page ignored is a setting that silently stayed
	// where it was, which is the failure mode the reference cannot report at all.
	ErrNotTaken = fmt.Errorf("settings: the page did not take the new value")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 200 * time.Millisecond
)

const stateKeyPrefix = "__headlessSettings"

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other and each polled until
// non-empty, so one could take the other's answer. The nonce comes from Go: a
// page-side Math.random or Date.now would put a decision and a clock where
// invariant 6 forbids them.
var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// State is every preference this package owns, read at one instant.
type State struct {
	// AutoDownload maps each category to whether it downloads by itself.
	AutoDownload map[Kind]bool
	// BackgroundSync is the global offline-notification preference.
	BackgroundSync bool
}

func (s State) String() string {
	return fmt.Sprintf("settings.State(kinds=%d backgroundSync=%t)",
		len(s.AutoDownload), s.BackgroundSync)
}

// Outcome is what a write did.
type Outcome struct {
	// Now is the value the page holds AFTER the write, read back rather than
	// assumed.
	Now bool
	// Changed is false when the setting was already where it was asked to go.
	// The reference cannot tell those apart; this does.
	Changed bool
}

func (o Outcome) String() string {
	return fmt.Sprintf("settings.Outcome(now=%t changed=%t)", o.Now, o.Changed)
}

// Manager reads and writes the preferences.
type Manager struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Manager {
	return &Manager{runner: runner, eval: eval}
}

// Read reports every preference at once.
//
// ONE ROUND TRIP ON PURPOSE: five separate reads would be five moments, and a
// caller comparing them would be comparing a state that never existed.
func (m *Manager) Read(ctx context.Context, label string) (State, error) {
	key := nextStateKey()
	raw, err := m.parked(ctx, readScript(key), key, label+"/read")
	if err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK   bool            `json:"ok"`
		Why  string          `json:"why"`
		Auto map[string]bool `json:"auto"`
		Sync bool            `json:"sync"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return State{}, fmt.Errorf("settings: unexpected answer: %w", e)
	}
	if !out.OK {
		return State{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	st := State{AutoDownload: map[Kind]bool{}, BackgroundSync: out.Sync}
	for _, k := range Kinds {
		if v, ok := out.Auto[string(k)]; ok {
			st.AutoDownload[k] = v
		}
	}
	return st, nil
}

// SetAutoDownload moves one category and PROVES it moved.
func (m *Manager) SetAutoDownload(ctx context.Context, kind Kind, on bool, label string) (Outcome, error) {
	if !knownKind(kind) {
		return Outcome{}, fmt.Errorf("%w: %q", ErrUnknownKind, kind)
	}
	return m.write(ctx, func(key string) string { return autoDownloadScript(kind, on, key) }, on, label+"/auto")
}

// SetBackgroundSync moves the global offline-notification preference and PROVES
// it moved.
//
// The reference notes that this takes effect only after the client restarts.
// That is about the EFFECT, not about the stored value: the value is readable
// immediately, and that is what the postcondition checks. Nothing here claims
// the running session changed behaviour.
func (m *Manager) SetBackgroundSync(ctx context.Context, on bool, label string) (Outcome, error) {
	return m.write(ctx, func(key string) string { return backgroundSyncScript(on, key) }, on, label+"/sync")
}

// write takes a script BUILDER, not a script.
//
// A CHAVE SO' EXISTE AQUI DENTRO (H177): cada chamada estaciona a resposta na sua
// propria global, e quem monta o script precisa dela. Receber a string pronta
// obrigaria o chamador a gerar a chave e a passa-la duas vezes, o que e' um
// convite a passar chaves diferentes.
func (m *Manager) write(ctx context.Context, build func(key string) string, want bool, label string) (Outcome, error) {
	key := nextStateKey()
	raw, err := m.parked(ctx, build(key), key, label)
	if err != nil {
		return Outcome{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Before  bool   `json:"before"`
		After   bool   `json:"after"`
		Skipped bool   `json:"skipped"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Outcome{}, fmt.Errorf("settings: unexpected answer: %w", e)
	}
	if !out.OK {
		return Outcome{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	// THE POSTCONDITION. The reference returns the requested flag here without
	// looking; this compares the value READ BACK against what was asked.
	if out.After != want {
		return Outcome{Now: out.After}, fmt.Errorf("%w: asked for %t, the page still holds %t",
			ErrNotTaken, want, out.After)
	}
	return Outcome{Now: out.After, Changed: !out.Skipped && out.Before != out.After}, nil
}

func knownKind(k Kind) bool {
	for _, known := range Kinds {
		if k == known {
			return true
		}
	}
	return false
}

func (m *Manager) parked(ctx context.Context, kick, key, label string) (string, error) {
	var started string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return m.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(Budget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/poll", func(c context.Context) error {
			return m.eval(c, `window.`+key+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			// A CHAVE E' LIBERADA ao ser lida (H177).
			var ignored string
			_ = m.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return m.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			return raw, nil
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("the page never settled within %s", Budget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(Tick):
		}
	}
}
