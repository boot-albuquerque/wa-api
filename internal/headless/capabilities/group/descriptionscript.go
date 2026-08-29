package group

import "strconv"

const (
	describeKey = "__headlessGroupDescribe"

	modModifyInfo = "WAWebGroupModifyInfoJob"
	modMsgKey     = "WAWebMsgKey"
	modWidFactory = "WAWebWidFactory"
)

// describeScript sets a group's description.
//
// FOUR ARGUMENTS, AND THE FOURTH IS THE INTERESTING ONE. setGroupDescription
// takes (wid, text, newId, descId). `descId` identifies the description being
// REPLACED and comes from the group's own metadata — it is `undefined` the first
// time, which is exactly what the lab group measured (H126). Passing something
// invented there would be replacing a description that does not exist.
//
// The metadata field is read under BOTH spellings, because this build keeps
// group metadata under __x_-prefixed state on live models and under bare names
// on query results — the same split that cost a run on channels (H112).
func describeScript(groupJID, description string) string {
	return `(() => {
	window.` + describeKey + ` = null;
	const park = v => { window.` + describeKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 150);
	(async () => {
		try {
			const JID = ` + strconv.Quote(groupJID) + `;
			const wid = window.require("` + modWidFactory + `").createWid(JID);

			// O descId vem da metadata do proprio grupo, e e' undefined na
			// primeira vez. Inventar um valor aqui seria substituir uma
			// descricao que nao existe.
			let descId = undefined;
			try {
				const CC = window.require("WAWebChatCollection").ChatCollection;
				const c = CC.get(JID);
				const md = c && (c.groupMetadata || c.__x_groupMetadata);
				if (md) { descId = md.descId || md.__x_descId || undefined; }
			} catch (e) {}

			const newId = await window.require("` + modMsgKey + `").newId();
			// UM OBJETO, NAO QUATRO POSICIONAIS. A referencia chama
			// setGroupDescription(wid, text, newId, descId); neste build a funcao
			// tem aridade 1 e recebe um objeto. Medido: a forma positional morre
			// com "Cannot read properties of undefined (reading 'toJid')", e
			// {groupWid, description, newId, prevDescId} passa (H126).
			await window.require("` + modModifyInfo + `").setGroupDescription({
				groupWid: wid,
				description: ` + strconv.Quote(description) + `,
				newId: newId,
				prevDescId: descId,
			});
			park({ ok: true });
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}
