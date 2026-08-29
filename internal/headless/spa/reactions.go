package spa

// ReactionsForMessageExpr reads the reactions on ONE message model.
//
// IT LIVES HERE BECAUSE TWO PACKAGES NEED THE SAME ANSWER, and two copies of a
// page query are two lists free to drift — the reason ResolveIdentityExpr lives
// here too. capabilities/message reports reactions to a caller;
// capabilities/react verifies its own removal. If those two ever disagreed, the
// failure would look like a removal that verified and then read back as present.
//
// THREE THINGS ABOUT IT ARE NOT OBVIOUS, and each cost a finding:
//
//	Reactions.find   is an ASYNC FETCH, not a lookup over loaded models. The
//	                 collection's getModelsArray stays at 0 even after a
//	                 reaction this module verified, which is what made two
//	                 earlier findings conclude the build has no source (H83,
//	                 H124). The record from find never lands in that array.
//	the KEY          is the id OBJECT. The reference passes msg.id._serialized,
//	                 which is NULL on this LID-first build: measured side by
//	                 side, find(_serialized) throws "called find without an id",
//	                 find(id.id) returns null, and find(id) returns the record.
//	hasReactionByMe  is the page's own flag and is used as such. Deriving it by
//	                 comparing this account's jid against the senders is the
//	                 comparison that has gone wrong here twice (H136, H148).
//
// It is a JavaScript function of (msgModel) returning
// {ok, why, groups:[{emoji, byMe, senders:[jid]}]}.
const ReactionsForMessageExpr = `(async function (m) {
	const jid = v => (v && v._serialized) ? v._serialized : (typeof v === "string" ? v : "");
	try {
		if (!m || !m.id) { return { ok: false, why: "NO_MESSAGE", groups: [] }; }
		const C = window.require("WAWebCollections");
		if (!C || !C.Reactions || typeof C.Reactions.find !== "function") {
			return { ok: false, why: "NO_REACTIONS_COLLECTION", groups: [] };
		}
		const rec = await C.Reactions.find(m.id);
		if (!rec || !rec.reactions) { return { ok: true, why: "", groups: [] }; }
		const ser = (typeof rec.reactions.serialize === "function")
			? rec.reactions.serialize() : rec.reactions;
		if (!Array.isArray(ser)) { return { ok: true, why: "", groups: [] }; }
		const groups = [];
		for (const g of ser) {
			if (!g) { continue; }
			const senders = [];
			if (Array.isArray(g.senders)) {
				for (const s of g.senders) {
					const who = jid(s && s.senderUserJid);
					if (who) { senders.push(who); }
				}
			}
			groups.push({
				emoji: (typeof g.aggregateEmoji === "string") ? g.aggregateEmoji : "",
				byMe: !!g.hasReactionByMe,
				senders: senders
			});
		}
		return { ok: true, why: "", groups: groups };
	} catch (e) {
		return { ok: false, why: String((e && e.message) || e).slice(0, 140), groups: [] };
	}
})`
