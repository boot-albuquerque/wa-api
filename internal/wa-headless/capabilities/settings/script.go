package settings

import (
	"strconv"
	"strings"
)

// The page modules and the accessor names, as constants rather than literals
// sprinkled through the scripts. Two copies of "WAWebUserPrefsGeneral" are the
// same bug waiting to diverge (ADR-0004).
const (
	modGeneral       = "WAWebUserPrefsGeneral"
	modNotifications = "WAWebUserPrefsNotifications"

	fnSyncGet = "getGlobalOfflineNotifications"
	fnSyncSet = "setGlobalOfflineNotifications"
)

// accessors names the getter and setter for one category. The page spells each
// category into the function name — there is no getAutoDownload(kind) — so the
// mapping is data here instead of string building at the call site.
//
// Measured present on this build 2026-08-22 (probe_settings_test.go): all four
// getters and setAutoDownloadAudio answered as functions.
var accessors = map[Kind][2]string{
	KindAudio:     {"getAutoDownloadAudio", "setAutoDownloadAudio"},
	KindDocuments: {"getAutoDownloadDocuments", "setAutoDownloadDocuments"},
	KindPhotos:    {"getAutoDownloadPhotos", "setAutoDownloadPhotos"},
	KindVideos:    {"getAutoDownloadVideos", "setAutoDownloadVideos"},
}

// preamble parks on the key THIS call was given.
//
// IT WAS A const AND HAD TO STOP BEING ONE (H177): a const bakes ONE page global
// into every script here, which is exactly the shared state two concurrent calls
// overwrite.
func preamble(key string) string {
	return `
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
`
}

func readScript(key string) string {
	var reads strings.Builder
	for _, k := range Kinds {
		reads.WriteString(`		auto[` + strconv.Quote(string(k)) + `] = !!G.` + accessors[k][0] + `();
`)
	}
	return `(() => {` + preamble(key) + `
	try {
		const G = window.require("` + modGeneral + `");
		const N = window.require("` + modNotifications + `");
		const auto = {};
` + reads.String() + `		park({ ok: true, auto: auto, sync: !!N.` + fnSyncGet + `() });
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	return "kicked";
	})()`
}

func autoDownloadScript(kind Kind, on bool, key string) string {
	get, set := accessors[kind][0], accessors[kind][1]
	return writeScript(modGeneral, get, set, on, key)
}

func backgroundSyncScript(on bool, key string) string {
	return writeScript(modNotifications, fnSyncGet, fnSyncSet, on, key)
}

// writeScript reads, writes only when needed, and READS BACK.
//
// The read-back is the whole point. The reference ends these functions with
// `return flag`, which reports the caller's own request as if it were the
// page's answer — a silent success. Here `after` comes from the getter, so a
// page that ignored the write says so.
func writeScript(mod, get, set string, on bool, key string) string {
	want := strconv.FormatBool(on)
	return `(() => {` + preamble(key) + `
	(async () => {
	try {
		const M = window.require("` + mod + `");
		const before = !!M.` + get + `();
		let skipped = false;
		// PEDIDO REDUNDANTE E' EVITADO, e dito. A H55 mediu um pedido redundante
		// sendo a CAUSA de uma falha, entao nao se reescreve o que ja' esta' no
		// lugar — mas "nao precisou" e "escreveu" sao respostas diferentes.
		if (before === ` + want + `) {
			skipped = true;
		} else {
			await M.` + set + `(` + want + `);
		}
		const after = !!M.` + get + `();
		park({ ok: true, before: before, after: after, skipped: skipped });
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	})();
	return "kicked";
	})()`
}
