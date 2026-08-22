package message

import "strconv"

const modMsgCollection = "WAWebMsgCollection"

func originScript(messageID string) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	const jid = v => (v && v._serialized) ? v._serialized : (typeof v === "string" ? v : "");
	try {
		const MC = window.require("` + modMsgCollection + `").MsgCollection;
		let m = null;
		try { m = MC.get(` + strconv.Quote(messageID) + `); } catch (e) {}
		if (!m) {
			// O id que quem chama tem e o CRU (msg.id.id), que e o que este
			// modulo reporta em todo lugar. A colecao e chaveada pela chave
			// inteira, entao a varredura e a busca honesta em vez de exigir um
			// segundo identificador que ninguem tem.
			const all = typeof MC.getModelsArray === "function" ? MC.getModelsArray() : [];
			for (const c of all) {
				try { if (c.id && c.id.id === ` + strconv.Quote(messageID) + `) { m = c; break; } } catch (e) {}
			}
		}
		if (!m) { park({ ok: true, notFound: true }); return "kicked"; }

		const id = m.id;
		const chat = jid(id && id.remote);
		// GRUPO E PERGUNTADO, NAO INFERIDO DO SUFIXO. Inferir de "@g.us" e ficar
		// a uma mudanca de build de estar errado, e este build ja trocou o
		// namespace de identidade uma vez (LID).
		let isGroup = false;
		try {
			const CC = window.require("WAWebChatCollection").ChatCollection;
			const c = CC.get(chat);
			isGroup = !!(c && window.require("WAWebChatGetters").getIsGroup(c));
		} catch (e) {
			isGroup = chat.indexOf("@g.us") >= 0;
		}

		// O REMETENTE DE UMA MENSAGEM DE GRUPO NAO E O CHAT. Em um-para-um o
		// participante e nulo e o remetente E o chat; em grupo o participante e
		// quem falou. Confundir os dois atribui a fala ao grupo.
		let sender = jid(id && id.participant);
		if (!sender && !isGroup && !(id && id.fromMe)) { sender = chat; }

		park({
			ok: true, notFound: false,
			chat: chat, group: isGroup, sender: sender,
			fromMe: !!(id && id.fromMe),
			t: (typeof m.t === "number") ? m.t : 0,
		});
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	return "kicked";
	})()`
}

// shapeScript reports the FIELD NAMES of a message's raw model, never a value.
//
// THIS IS THE DELIBERATE DIVERGENCE. Message.rawData in the reference hands back
// the whole raw object, which for a chat message IS the body. This module keeps
// bodies out structurally (invariant 12, enforced in capabilities/messagemeta by
// TestNoBodyFieldExistsAnywhere), so returning rawData as-is would defeat a
// guard rather than deliver a feature.
//
// What rawData is actually FOR — seeing what the page has on a message when a
// capability does not behave — is served by the shape, and the shape carries no
// identity: key names only, sorted, with the value side never read.
func shapeScript(messageID string) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	try {
		const MC = window.require("` + modMsgCollection + `").MsgCollection;
		let m = null;
		try { m = MC.get(` + strconv.Quote(messageID) + `); } catch (e) {}
		if (!m) {
			const all = typeof MC.getModelsArray === "function" ? MC.getModelsArray() : [];
			for (const c of all) {
				try { if (c.id && c.id.id === ` + strconv.Quote(messageID) + `) { m = c; break; } } catch (e) {}
			}
		}
		if (!m) { park({ ok: true, notFound: true }); return "kicked"; }

		// SO AS CHAVES. Nao ha leitura de m[k] em lugar nenhum deste script, e e
		// isso que o mantem honesto: uma versao que lesse valores para "decidir
		// se e vazio" ja teria o corpo em maos.
		const seen = {};
		const push = o => {
			if (!o) { return; }
			for (const k of Object.keys(o)) { seen[k] = true; }
		};
		push(m);
		push(m.attributes);
		park({ ok: true, notFound: false, keys: Object.keys(seen).sort() });
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	return "kicked";
	})()`
}

// currentScript re-reads the fields of a message that CHANGE after it exists.
//
// It is the equivalent of Message.reload, and what it reports was chosen from a
// measurement rather than from the reference's field list (H108): over the 395
// messages this account had loaded, ack took all four values (0:25, 1:19, 2:205,
// 3:141) and was ABSENT on 5, star was false on all 395, and type took eleven
// values. So ack earns a re-read, and "absent" earns being distinguishable from
// zero — merging them is the mistake H90 and the event-freshness work each made
// once already.
//
// IT DOES NOT ANSWER "WAS THIS REVOKED". That vocabulary — isRevokedMsg,
// type === 'revoked', revokeSender — is owned by capabilities/revoke, and a
// second copy here would be two lists free to drift. The type is reported raw,
// so a caller sees 'revoked' when the page says it.
func currentScript(messageID string) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	try {
		const MC = window.require("` + modMsgCollection + `").MsgCollection;
		let m = null;
		try { m = MC.get(` + strconv.Quote(messageID) + `); } catch (e) {}
		if (!m) {
			const all = typeof MC.getModelsArray === "function" ? MC.getModelsArray() : [];
			for (const c of all) {
				try { if (c.id && c.id.id === ` + strconv.Quote(messageID) + `) { m = c; break; } } catch (e) {}
			}
		}
		if (!m) { park({ ok: true, notFound: true }); return "kicked"; }

		// AUSENTE E ZERO SAO RESPOSTAS DIFERENTES, e 5 das 395 mensagens medidas
		// nao tinham ack nenhum. Fundi-las em 0 diria "nao saiu" sobre uma
		// mensagem sobre a qual a pagina nao disse nada.
		const hasAck = (typeof m.ack === "number");
		park({
			ok: true, notFound: false,
			hasAck: hasAck, ack: hasAck ? m.ack : 0,
			starred: !!m.star,
			type: (typeof m.type === "string") ? m.type : "",
		});
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	return "kicked";
	})()`
}
