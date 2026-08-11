package spa

// The module inventory: what this stack asks web.whatsapp.com for, in one
// place, verified at session start.
//
// The integration surface is window.require('<ModuleName>'), and those names
// are undocumented Meta contract — they change when Meta ships a build, with no
// notice and no deprecation. ADR-0006 D4 governs this, and its non-obvious rule
// is the one implemented here: the inventory is verified AT STARTUP, failing
// loudly with the list of missing names.
//
// Without that, a rename surfaces as an exception halfway through a dispatch,
// on whichever capability happened to run first, with a message about a
// property of undefined. With it, the boot stops in a predictable place and
// names the cause.
//
// Names are constants HERE and nowhere else. A literal at a call site is the
// defect this file exists to prevent (ADR-0004).
//
// Source: whatsapp-web.js 1.34.7, as installed in the product
// (services/wa-worker/node_modules/whatsapp-web.js). Read from that tree, not
// from memory. Note that ../spa/doc.go cites main @ 942d236a11ad; the installed
// version is what the product actually runs, and it is what this list follows.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"wa-api/internal/wa-headless/engine"
)

// Module is a name passed to window.require.
type Module string

// The modules whatsapp-web.js resolves in its AuthStore at startup.
//
// This is a deliberate MINIMUM, not the library's full surface. wwebjs 1.34.7
// requires 41 distinct modules across its whole feature set; this stack needs
// six capabilities, so importing all 41 would be inventing dependencies to
// break on. Each capability adds the modules it actually uses, and the parity
// matrix says which capabilities there are.
//
// These eight are the ones that make a session a session — the socket, the
// connection state, the command channel, the identity. If any is gone, nothing
// above it can work, so they are the right thing to fail the boot on.
const (
	ModuleBase64                = Module("WABase64")
	ModuleAdvSignatureAPI       = Module("WAWebAdvSignatureApi")
	ModuleCmd                   = Module("WAWebCmd")
	ModuleCompanionRegClientUtl = Module("WAWebCompanionRegClientUtils")
	ModuleConnModel             = Module("WAWebConnModel")
	ModuleSignalStoreAPI        = Module("WAWebSignalStoreApi")
	ModuleSocketModel           = Module("WAWebSocketModel")
	ModuleUserPrefsInfoStore    = Module("WAWebUserPrefsInfoStore")
)

// RequiredAtStartup is verified before any capability runs.
var RequiredAtStartup = []Module{
	ModuleBase64,
	ModuleAdvSignatureAPI,
	ModuleCmd,
	ModuleCompanionRegClientUtl,
	ModuleConnModel,
	ModuleSignalStoreAPI,
	ModuleSocketModel,
	ModuleUserPrefsInfoStore,
}

// ErrModulesMissing is a boot that stopped because the page no longer exposes
// what this stack needs.
//
// It is a distinct error because the operational response is distinct: a
// missing module is Meta shipping a build, and no amount of retrying or
// recycling fixes it. Treating it as a transient failure would make a fleet
// restart every session, forever, over something only a code change resolves.
type ErrModulesMissing struct {
	Missing []Module
}

func (e *ErrModulesMissing) Error() string {
	names := make([]string, len(e.Missing))
	for i, m := range e.Missing {
		names[i] = string(m)
	}
	return fmt.Sprintf("spa: web.whatsapp.com no longer exposes %d module(s): %s. "+
		"These names are undocumented Meta contract and change when Meta ships a build; "+
		"this is a code change, not a retry", len(e.Missing), strings.Join(names, ", "))
}

// resolveScript asks the page which of the given names resolve.
//
// It returns the MISSING ones, not the present ones, so the answer is short and
// carries no module contents — only names this code already knew. window.require
// throws for an unknown name, hence the try/catch; a name that resolves to a
// falsy value counts as missing too, because a module that is there but empty
// is not a module we can use.
func resolveScript(modules []Module) string {
	names := make([]string, len(modules))
	for i, m := range modules {
		names[i] = `'` + string(m) + `'`
	}
	return `JSON.stringify((() => {
		const wanted = [` + strings.Join(names, ",") + `];
		if (typeof window.require !== 'function') return wanted;
		const missing = [];
		for (const name of wanted) {
			try {
				if (!window.require(name)) missing.push(name);
			} catch (e) {
				missing.push(name);
			}
		}
		return missing;
	})())`
}

// VerifyInventory checks the modules at session start.
//
// It runs under the Query budget rather than StateProbe: this is a real
// question for the page to answer, not a liveness poke, and a page still
// booting can legitimately take longer than a probe allows.
func VerifyInventory(ctx context.Context, r *engine.Runner, eval Evaluator, modules []Module) error {
	if len(modules) == 0 {
		return nil
	}

	var raw string
	if err := r.Do(ctx, engine.OpQuery, "spa/verify-inventory", func(ctx context.Context) error {
		return eval(ctx, resolveScript(modules), &raw)
	}); err != nil {
		return fmt.Errorf("spa: verifying the module inventory: %w", err)
	}

	var missing []Module
	if err := json.Unmarshal([]byte(raw), &missing); err != nil {
		return fmt.Errorf("spa: the inventory check answered %q, which is not the agreed "+
			"shape: %w", raw, err)
	}
	if len(missing) > 0 {
		return &ErrModulesMissing{Missing: missing}
	}
	return nil
}
