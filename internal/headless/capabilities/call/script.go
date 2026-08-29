package call

import "strconv"

// The page side. Parked answers, synchronous outer functions, no clock.

const (
	modEventCallLink = "WAWebGenerateEventCallLink"
	modWap           = "WAWap"
	modSendIq        = "WADeprecatedSendIq"
	modMeUser        = "WAWebUserPrefsMeUser"
	modCallColl      = "WAWebCallCollection"
	modQueryExists   = "WAWebQueryExistsJob"
	modWidFactory    = "WAWebWidFactory"
	modStartCall     = "WAWebVoipStartCall"
	modCancelCall    = "WAWebVoipCancelOutgoingCall"
	modEnsureVoip    = "WAWebEnsureVoipInited"
)

// prelude parks on the key THIS call was given.
//
// IT WAS A const AND HAD TO STOP BEING ONE (H177/H178). A const bakes ONE page
// global into every script in this file, which is exactly the shared state two
// concurrent calls overwrite — and it is why the mechanical sweep could not
// convert this package: a const takes no parameter.
func prelude(key string) string {
	return `
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
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
}

func linkScript(startUnix int64, kind Kind, key string) string {
	return `(() => {` + prelude(key) + `
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

func rejectScript(callerJID, callID string, key string) string {
	return `(() => {` + prelude(key) + `
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

func pendingScript(key string) string {
	return `(() => {` + prelude(key) + `
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
}

func placeScript(peerJID string, video bool, key string) string {
	return `(() => {` + prelude(key) + `
	(async () => {
		try {
			// RESOLVE FIRST, then call — which is what the app's own call sites
			// do (queryWidExists(...).then(e => startWAWebVoipCall(e.wid, ...)))
			// and what H34 cost this repository to learn. This build files under
			// LID; the phone number is not what the server routes on.
			const ex = await window.require("` + modQueryExists + `")
				.queryWidExists(window.require("` + modWidFactory + `").createWid(` + strconv.Quote(peerJID) + `));
			if (!ex || !ex.wid) { park({ ok: false, why: "NOT_ON_WHATSAPP" }); return; }
			// INITIALISE THE VOIP STACK FIRST, and this line is the whole
			// difference between a call and silence.
			//
			// The first live attempt dialled without it: startWAWebVoipCall
			// resolved, reported nothing wrong, and NO HANDSET RANG — confirmed
			// by the only instrument that could confirm it, a human saying the
			// phone stayed quiet. That is the NOTHING class (H82) for the third
			// time in this repository.
			//
			// The environment was not the cause and that was measured, not
			// assumed: isCallingEnabled true, browser supported, crossOriginIsolated
			// true, SharedArrayBuffer and WebAssembly both present. What the app
			// has that a headless driver does not is the UI path that runs
			// ensureVoipInitialized before any call button works — the same shape
			// as H34, where the fix was to call the resolution the page already
			// has, BEFORE acting.
			await window.require("` + modEnsureVoip + `").ensureVoipInitialized();

			// Two arguments, which is the SHORTEST form the app itself uses.
			// The other call sites add a CALL_FROM_UI source and a lobby entry
			// point; both are telemetry, and passing an invented value would put
			// this module's fingerprints into somebody's analytics.
			await window.require("` + modStartCall + `").startWAWebVoipCall(ex.wid, ` + strconv.FormatBool(video) + `);
			park({ ok: true });
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}

func cancelScript(key string) string {
	return `(() => {` + prelude(key) + `
	(async () => {
		try {
			await window.require("` + modCancelCall + `").cancelPendingOutgoingCall();
			park({ ok: true });
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}

func ensureVoipScript(key string) string {
	return `(() => {` + prelude(key) + `
	(async () => {
		try {
			await window.require("` + modEnsureVoip + `").ensureVoipInitialized();
			park({ ok: true });
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}
