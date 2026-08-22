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
