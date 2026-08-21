package groupreq

import "strconv"

// The page side.
//
// Every script here parks its answer on one global and Go polls it. The outer
// function is SYNCHRONOUS on purpose: engine.Evaluate does not await, so an
// async outer function would hand Go a Promise and the read would come back as
// an object nobody can use.

// modules this family needs. They are named once, here, because a module name
// repeated in two scripts is the same bug waiting to diverge (ADR-0004).
const (
	modWidFactory = "WAWebWidFactory"
	modWidToJid   = "WAWebWidToJid"
	modQueryJob   = "WAWebGroupQueryJob"
	modReqStore   = "WAWebApiMembershipApprovalRequestStore"
	modActionRPC  = "WASmaxGroupsMembershipRequestsActionRPC"
)

// prelude parks the result holder and defines the helpers both scripts use.
const prelude = `
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const W = window.require(` + `"` + modWidFactory + `"` + `);
	const W2J = window.require(` + `"` + modWidToJid + `"` + `);
`

func listScript(groupJID string) string {
	return `(() => {` + prelude + `
	(async () => {
		try {
			const gwid = W.createWid(` + strconv.Quote(groupJID) + `);
			// REFRESH FIRST. A session holds the metadata it was given at boot,
			// and a request that arrived since is not in it. Skipping this makes
			// the read answer "none pending" for a group that has three.
			await window.require("` + modQueryJob + `")
				.queryAndUpdateGroupMetadataById({ id: ` + strconv.Quote(groupJID) + ` });
			const got = await window.require("` + modReqStore + `")
				.getMembershipApprovalRequests(gwid);
			const rows = Array.isArray(got) ? got : [];
			// The FIELD NAMES of what came back, never the values. The shape
			// could not be measured on a group with no pending requests, so the
			// first real one is what says what is there.
			const fields = rows.length ? Object.keys(rows[0]) : [];
			const jidOf = v => {
				if (!v) return "";
				if (typeof v === "string") return v;
				if (v._serialized) return v._serialized;
				return "";
			};
			park({
				ok: true,
				fields: fields,
				requests: rows.map(r => ({
					id: jidOf(r.id),
					addedBy: jidOf(r.addedBy),
					t: typeof r.t === "number" ? r.t : 0,
					method: typeof r.requestMethod === "string" ? r.requestMethod : "",
				})),
			});
		} catch (e) {
			park({ ok: false, why: String(e && e.message) });
		}
	})();
	return "kicked";
	})()`
}

func actionScript(groupJID string, requesters []string, approve bool) string {
	list := "["
	for i, r := range requesters {
		if i > 0 {
			list += ","
		}
		list += strconv.Quote(r)
	}
	list += "]"
	argsKey := "rejectArgs"
	if approve {
		argsKey = "approveArgs"
	}
	valueKey := "membershipRequestsActionReject"
	mixinsKey := "membershipRequestsActionRejectParticipantMixins"
	if approve {
		valueKey = "membershipRequestsActionApprove"
		mixinsKey = "membershipRequestsActionAcceptParticipantMixins"
	}
	return `(() => {` + prelude + `
	(async () => {
		const results = [];
		try {
			const gwid = W.createWid(` + strconv.Quote(groupJID) + `);
			const groupJid = W2J.widToGroupJid(gwid);
			const RPC = window.require("` + modActionRPC + `");
			// ONE CALL PER REQUESTER, which is what the reference does and what
			// the RPC's own shape asks for. It also means a partial outcome is
			// normal: the loop keeps going and every requester gets a row, so
			// "the second of three failed" is visible instead of being the
			// error that hides the other two.
			for (const id of ` + list + `) {
				const wid = W.createWid(id);
				const participant = { participantArgs: [{ participantJid: W2J.widToUserJid(wid) }] };
				try {
					const response = await RPC.sendMembershipRequestsActionRPC({
						iqTo: groupJid,
						` + argsKey + `: participant,
					});
					if (response && response.name === "MembershipRequestsActionResponseSuccess") {
						const value = response.value && response.value.` + valueKey + `;
						const p = value && value.participant && value.participant[0];
						const mix = p && p.` + mixinsKey + `;
						const err = mix && mix.value && mix.value.error;
						results.push(err
							? { id: id, ok: false, code: +err, why: "PARTICIPANT_ERROR" }
							: { id: id, ok: true, code: 0, why: "" });
					} else {
						results.push({ id: id, ok: false, code: 0,
							why: (response && response.name) || "NO_RESPONSE" });
					}
				} catch (e) {
					results.push({ id: id, ok: false, code: 0, why: String(e && e.message) });
				}
			}
			park({ ok: true, results: results });
		} catch (e) {
			park({ ok: false, why: String(e && e.message), results: results });
		}
	})();
	return "kicked";
	})()`
}
