package spa

// The web build's own version string, read rather than pinned.
//
// This sits beside the module inventory on purpose. The reference pins the web
// version with initWebVersionCache; this stack deliberately does not (that row
// is an INTENTIONAL_DIFFERENCE in the ledger) and uses the inventory as its
// guard instead. Reading the version does not change that decision — it makes
// it reportable: when the inventory fails, the version is the single most useful
// thing to put next to the list of missing names.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"wa-api/internal/wa-headless/engine"
)

// ErrNoWebVersion is a page that does not expose one.
//
// IT IS NOT AN EMPTY STRING. A caller that got "" could not tell "this build
// hides it" from "the read failed", and those have different repairs.
var ErrNoWebVersion = fmt.Errorf("spa: this page does not expose a web version")

// webVersionScript reads window.Debug.VERSION.
//
// Measured present on this build 2026-08-22 (probe_settings_test.go). It is read
// through the same one-shot expression the inventory uses rather than a parked
// global, because there is nothing to await: it is a property, not a query.
const webVersionScript = `(() => {
	try {
		const v = (window.Debug && window.Debug.VERSION) || "";
		return JSON.stringify({ ok: true, version: (typeof v === "string") ? v : "" });
	} catch (e) {
		return JSON.stringify({ ok: false });
	}
})()`

// WebVersion reports the version string the page names for itself.
func WebVersion(ctx context.Context, r *engine.Runner, eval Evaluator) (string, error) {
	var raw string
	if err := r.Do(ctx, engine.OpQuery, "spa/web-version", func(ctx context.Context) error {
		return eval(ctx, webVersionScript, &raw)
	}); err != nil {
		return "", fmt.Errorf("spa: reading the web version: %w", err)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return "", fmt.Errorf("spa: the web version check answered %q, which is not the "+
			"agreed shape: %w", raw, err)
	}
	if !out.OK || strings.TrimSpace(out.Version) == "" {
		return "", ErrNoWebVersion
	}
	return out.Version, nil
}
