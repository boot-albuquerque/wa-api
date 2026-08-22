package addressbook

import "strconv"

// The page side. Every script parks its answer on one global and Go polls it;
// the outer function is synchronous because engine.Evaluate does not await.

const (
	modWidFactory  = "WAWebWidFactory"
	modSaveContact = "WAWebSaveContactAction"
	modDelContact  = "WAWebDeleteContactAction"
	modDeviceList  = "WAWebApiDeviceList"
	modContactColl = "WAWebContactCollection"
)

// prelude parks on the key THIS call was given.
//
// IT WAS A const AND HAD TO STOP BEING ONE (H177): a const bakes ONE page global
// into every script here, which is exactly the shared state two concurrent calls
// overwrite.
func prelude(key string) string {
	return `
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const W = window.require("` + modWidFactory + `");
	const CC = window.require("` + modContactColl + `").ContactCollection;
	// A NAME IS A NAME WHEREVER IT LANDS. The record keeps several, and which
	// one a save fills is not something this module gets to assume: the probe
	// found the peer with name, pushname and shortName all empty, so "has a
	// name now" has to mean any of them.
	const named = c => !!(c && (c.name || c.shortName || c.formattedName));
	const find = pn => {
		try {
			const wid = W.createWid(pn + "@c.us");
			return CC.get(wid) || CC.get(pn + "@c.us") || null;
		} catch (e) { return null; }
	};
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

func saveScript(phone, first, last string, sync bool, key string) string {
	return `(() => {` + prelude(key) + `
	(async () => {
		try {
			const had = named(find(` + strconv.Quote(phone) + `));
			await window.require("` + modSaveContact + `").saveContactAction({
				firstName: ` + strconv.Quote(first) + `,
				lastName: ` + strconv.Quote(last) + `,
				phoneNumber: ` + strconv.Quote(phone) + `,
				// prevPhoneNumber is what the record is keyed on TODAY. It is
				// the same number here because this call names a contact rather
				// than renumbering one; an edit that changes the number is a
				// different operation and does not exist yet.
				prevPhoneNumber: ` + strconv.Quote(phone) + `,
				syncToAddressbook: ` + strconv.FormatBool(sync) + `,
				username: undefined,
			});
			// THE ANSWER IS READ BACK, not assumed from the acceptance — but
			// the WAITING happens in Go, not here.
			//
			// The first version of this script polled the record with
			// setTimeout, which would have worked and would have broken
			// invariant 6: no clock in the page. The rule is not fussiness. A
			// page that decides how long to wait decides it during a reload, a
			// throttled tab and a hung renderer, in a place where Go cannot see
			// the decision or cancel it — and every deadline this module has is
			// on the Go side precisely so that a caller's context means
			// something.
			//
			// So this parks as soon as the page accepts, reports what the
			// record looked like BEFORE, and Manager.Save polls nameStateScript
			// under the caller's budget.
			park({ ok: true, had: had, has: named(find(` + strconv.Quote(phone) + `)) });
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}

func deleteScript(phone string, key string) string {
	return `(() => {` + prelude(key) + `
	(async () => {
		try {
			// THE SUFFIX IS REQUIRED HERE AND NOT IN THE SAVE, which is not a
			// symmetry anybody would guess. saveContactAction takes
			// phoneNumber as BARE DIGITS; deleteContactAction takes a wid, and
			// createWid on bare digits fails on this build with
			// "wid error: invalid wid". The reference passes the same value to
			// both, and the first live delete here died on exactly that.
			const wid = W.createWid(` + strconv.Quote(phone) + ` + "@c.us");
			await window.require("` + modDelContact + `").deleteContactAction({ phoneNumber: wid });
			park({ ok: true });
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}

func devicesScript(userJID string, key string) string {
	return `(() => {` + prelude(key) + `
	(async () => {
		try {
			const wid = W.createWid(` + strconv.Quote(userJID) + `);
			const rows = await window.require("` + modDeviceList + `").getDeviceIds([wid]);
			const row = Array.isArray(rows) && rows.length ? rows[0] : null;
			// "NO RECORD" AND "ZERO DEVICES" ARE DIFFERENT ANSWERS, and folding
			// them into the number 0 would let a user this account has never
			// exchanged keys with look like a user with no phone.
			if (!row || typeof row.devices !== "object" || row.devices === null) {
				park({ ok: true, known: false, count: 0 });
				return;
			}
			const n = Array.isArray(row.devices)
				? row.devices.length : Object.keys(row.devices).length;
			park({ ok: true, known: true, count: n });
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}

// nameStateScript is the read Go polls after a save. Synchronous: it asks the
// model what it holds right now and answers in one evaluation, which is what
// keeps the clock on the Go side.
func nameStateScript(phone string, key string) string {
	return `(() => {` + prelude(key) + `
	try {
		return JSON.stringify({ ok: true, named: named(find(` + strconv.Quote(phone) + `)) });
	} catch (e) {
		return JSON.stringify({ ok: false, why: describe(e) });
	}
	})()`
}
