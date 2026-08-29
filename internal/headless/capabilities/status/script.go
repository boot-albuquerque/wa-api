package status

import "strconv"

// The page side. Parked answers, synchronous outer functions, no clock.

const (
	modCollections   = "WAWebCollections"
	modStatusGetters = "WAWebStatusGetters"
	modWidFactory    = "WAWebWidFactory"
)

// prelude reads ONE feed through the page's own getters.
//
// THE GETTERS ARE USED RATHER THAN THE FIELDS, and that is the lesson H51 paid
// for: a model keeps its properties behind "__x_" and exposes them by getter,
// and reaching past the getter is how a reader ends up answering for 1 record in
// 384. WAWebStatusGetters exports getId, getT, getUnreadCount, getTotalCount,
// getReadCount and getIsLoading — every field this package reports.
// prelude parks on the key THIS call was given.
//
// IT WAS A const AND HAD TO STOP BEING ONE (H177): a const bakes ONE page global
// into every script here, which is exactly the shared state two concurrent calls
// overwrite.
func prelude(key string) string {
	return `
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 130);
	const G = window.require("` + modStatusGetters + `");
	const num = (fn, s) => { try { const v = fn(s); return typeof v === "number" ? v : 0; } catch (e) { return 0; } };
	const readFeed = (s) => {
		let id = "";
		try {
			const w = G.getId ? G.getId(s) : (s && s.id);
			id = (w && w._serialized) || (typeof w === "string" ? w : "");
		} catch (e) {}
		return {
			id: id,
			total: num(G.getTotalCount, s),
			unread: num(G.getUnreadCount, s),
			read: num(G.getReadCount, s),
			t: num(G.getT, s),
			loading: (() => { try { return !!(G.getIsLoading && G.getIsLoading(s)); } catch (e) { return false; } })(),
		};
	};
	const statuses = () => {
		const C = window.require("` + modCollections + `");
		const S = C && C.Status;
		if (!S || typeof S.getModelsArray !== "function") { return null; }
		return S;
	};
`
}

func listScript(key string) string {
	return `(() => {` + prelude(key) + `
	try {
		const S = statuses();
		if (!S) { park({ ok: false, why: "NO_STATUS_COLLECTION" }); return "kicked"; }
		const feeds = [];
		for (const s of S.getModelsArray()) {
			try { feeds.push(readFeed(s)); } catch (e) {}
		}
		park({ ok: true, feeds: feeds });
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	return "kicked";
	})()`
}

func byContactScript(contactJID string, key string) string {
	return `(() => {` + prelude(key) + `
	try {
		const S = statuses();
		if (!S) { park({ ok: false, why: "NO_STATUS_COLLECTION" }); return "kicked"; }
		// BOTH FORMS ARE TRIED, because this build files under LID and a caller
		// holds whichever identity it was given. Asking twice costs nothing and
		// answering "not found" for the wrong key would be a lie about the
		// account rather than about the argument.
		let s = null;
		try { s = S.get(` + strconv.Quote(contactJID) + `); } catch (e) {}
		if (!s) {
			try {
				const wid = window.require("` + modWidFactory + `").createWid(` + strconv.Quote(contactJID) + `);
				s = S.get(wid);
			} catch (e) {}
		}
		if (!s) { park({ ok: true, found: false }); return "kicked"; }
		park({ ok: true, found: true, feed: readFeed(s) });
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	return "kicked";
	})()`
}
