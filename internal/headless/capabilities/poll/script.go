package poll

import "strconv"

// The page side. Parked answers, synchronous outer functions, no clock.

const (
	modMsgCollection = "WAWebMsgCollection"
	modVotesSchema   = "WAWebPollsVotesSchema"
	modSendVote      = "WAWebPollsSendVoteMsgAction"
)

// prelude finds the poll and refuses anything else, once, for both scripts.
//
// THE KEY IS THE MESSAGE'S OWN, NOT ONE REBUILT FROM A STRING. The reference
// does MsgKey.fromString(msg.id._serialized) and that throws on this build. The
// model's id already IS the key; its toString is what the votes table indexes.
// prelude parks on the key THIS call was given.
//
// IT WAS A const AND HAD TO STOP BEING ONE (H177): a const bakes ONE page global
// into every script here, which is exactly the shared state two concurrent calls
// overwrite.
func prelude(key string) string {
	return `
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, '<redacted>').slice(0, 140);
	const findPoll = (id) => {
		const MC = window.require("` + modMsgCollection + `").MsgCollection;
		let m = null;
		try { m = MC.get(id); } catch (e) {}
		if (!m) {
			// The id a caller holds is the RAW per-message id (msg.id.id), which
			// is what this module reports everywhere else. The collection is
			// keyed by the full key, so a scan is the honest lookup rather than
			// a second identifier nobody has.
			const all = typeof MC.getModelsArray === "function" ? MC.getModelsArray() : [];
			for (const c of all) {
				try { if (c.id && c.id.id === id) { m = c; break; } } catch (e) {}
			}
		}
		return m;
	};
	const isPoll = (m) => !!(m && m.pollOptions && m.pollOptions.length);
`
}

func votesScript(messageID string, key string) string {
	return `(() => {` + prelude(key) + `
	(async () => {
		try {
			const m = findPoll(` + strconv.Quote(messageID) + `);
			if (!m) { park({ ok: false, why: "NO_MESSAGE" }); return; }
			if (!isPoll(m)) { park({ ok: false, why: "NOT_A_POLL" }); return; }

			const rows = await window.require("` + modVotesSchema + `")
				.getTable().equals(["parentMsgKey"], m.id.toString());
			const list = Array.isArray(rows) ? rows : [];

			// EVERY OPTION IS PRESENT, INCLUDING THE UNVOTED ONES. A tally that
			// omitted them would make "nobody chose this" indistinguishable from
			// "this build did not report it".
			const options = {};
			for (const o of m.pollOptions) {
				if (o && typeof o.localId === "number") { options[String(o.localId)] = 0; }
			}
			const voters = {};
			for (const r of list) {
				try {
					// The selection is a byte array of local ids.
					const sel = r.selectedOptionLocalIds
						? Array.from(new Uint8Array(r.selectedOptionLocalIds)) : [];
					for (const id of sel) {
						const k = String(id);
						options[k] = (options[k] || 0) + 1;
					}
					// COUNTED, NEVER CARRIED: a voter is a phone number.
					const who = r.senderTimestampMs !== undefined && r.sender
						? String(r.sender) : String(r.sender || "");
					if (who) { voters[who] = true; }
				} catch (e) {}
			}
			park({ ok: true, options: options, voters: Object.keys(voters).length, rows: list.length });
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}

func voteScript(messageID string, options []string, key string) string {
	list := "["
	for i, o := range options {
		if i > 0 {
			list += ","
		}
		list += strconv.Quote(o)
	}
	list += "]"
	return `(() => {` + prelude(key) + `
	(async () => {
		try {
			const m = findPoll(` + strconv.Quote(messageID) + `);
			if (!m) { park({ ok: false, why: "NO_MESSAGE" }); return; }
			if (!isPoll(m)) { park({ ok: false, why: "NOT_A_POLL" }); return; }

			// NAMES IN, LOCAL IDS OUT, and a name that matches nothing is a
			// refusal rather than an omission: the page's send takes a SET, and
			// an unmatched name would quietly produce a smaller one — a vote for
			// fewer things than the caller asked for, reported as success.
			const wanted = ` + list + `;
			const ids = new Set();
			let matched = 0;
			for (const name of wanted) {
				let hit = false;
				for (const o of m.pollOptions) {
					if (o && o.name === name) { ids.add(o.localId); hit = true; break; }
				}
				if (hit) { matched++; }
			}
			if (matched === 0) { park({ ok: false, why: "NO_OPTION_MATCHED" }); return; }
			await window.require("` + modSendVote + `").sendVote(m, ids);
			park({ ok: true, matched: matched });
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}
