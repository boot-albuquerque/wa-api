package call

import "strconv"

// The page side. Parked answers, synchronous outer functions, no clock.

const (
	modEventCallLink = "WAWebGenerateEventCallLink"
	modWap           = "WAWap"
	modSendIq        = "WADeprecatedSendIq"
	modMeUser        = "WAWebUserPrefsMeUser"
	modCallColl      = "WAWebCallCollection"
)

const prelude = `
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const describe = e => {
		if (e === null || e === undefined) return "threw " + String(e);
		if (typeof e === "string") return "string: " + e;
		const name = (e.constructor && e.constructor.name) || typeof e;
		const parts = [name];
		if (e.message) parts.push("message=" + e.message);
		if (e.status !== undefined) parts.push("status=" + e.status);
		if (e.code !== undefined) parts.push("code=" + e.code);
		try { parts.push("keys=[" + Object.keys(e).join(",") + "]"); } catch (_) {}
		return parts.join(" ");
	};
`

func linkScript(startUnix int64, kind Kind) string {
	return `(() => {` + prelude + `
	(async () => {
		try {
			// THE START TIME ARRIVES AS UNIX SECONDS FROM GO. The page is never
			// asked what time it is (invariant 6), and the upstream's own
			// Math.floor(startTime.getTime()/1000) happens on the Go side.
			const link = await window.require("` + modEventCallLink + `")
				.createEventCallLink(` + strconv.FormatInt(startUnix, 10) + `, ` + strconv.Quote(string(kind)) + `);
			park({ ok: true, link: typeof link === "string" ? link : "" });
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}

func rejectScript(callerJID, callID string) string {
	return `(() => {` + prelude + `
	(async () => {
		try {
			const Wap = window.require("` + modWap + `");
			const me = window.require("` + modMeUser + `").getMaybeMePnUser();
			if (!me || !me._serialized) { park({ ok: false, why: "NO_ME_USER" }); return; }
			// A HAND-BUILT STANZA, AND THAT IS NOT THE REFERENCE IMPROVISING.
			// This build exports no reject action at all — WAWebRejectCallAction,
			// WAWebEndCallAction, WAWebCallActions and WAWebOfferCallAction are
			// all absent (measured). The stanza is the only door.
			const stanza = Wap.wap("call", {
				id: Wap.generateId(),
				from: me._serialized,
				to: ` + strconv.Quote(callerJID) + `,
			}, [
				Wap.wap("reject", {
					"call-id": ` + strconv.Quote(callID) + `,
					"call-creator": ` + strconv.Quote(callerJID) + `,
					count: "0",
				}),
			]);
			await window.require("` + modSendIq + `").deprecatedCastStanza(stanza);
			park({ ok: true });
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}

const pendingScript = `(() => {` + prelude + `
	try {
		const C = window.require("` + modCallColl + `");
		const holder = C.CallCollection || C.default || C;
		let n = 0;
		if (holder && typeof holder.getModelsArray === "function") {
			n = (holder.getModelsArray() || []).length;
		} else {
			// The collection keeps its models in a Map under a minified name.
			// COUNTED, never read: a call model carries the caller's identity.
			for (const k of Object.keys(C)) {
				try { if (C[k] instanceof Map) { n += C[k].size; } } catch (_) {}
			}
		}
		park({ ok: true, count: n });
	} catch (e) {
		park({ ok: false, why: describe(e) });
	}
	return "kicked";
	})()`
