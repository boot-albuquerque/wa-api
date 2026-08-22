package message

import (
	"strconv"

	"wa-api/internal/wa-headless/spa"
)

const modMsgCollection = "WAWebMsgCollection"

func originScript(messageID, key string) string {
	return `(() => {
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
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
func shapeScript(messageID, key string) string {
	return `(() => {
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
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
func currentScript(messageID, key string) string {
	return `(() => {
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
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

// quotedScript reads WHICH message a message quotes.
//
// THE FIELD IS NOT WHERE THE MODEL SUGGESTS. A message model carries
// __x_fromQuotedMsg, __x_isQuotedMsgAvailable and __x_questionReplyQuotedMessage
// on EVERY row — measured on all 110 of a hydration — and all three hold a lazy
// sentinel, not data. Reading them as truth counts every message as quoting
// something, which is exactly what a first, wrong instrument reported (H131).
//
// The field that carries the reference is quotedStanzaID, which is the same one
// capabilities/send uses to PROVE a reply carried its quote. Using the same
// field is deliberate: two ways of asking "does this quote something" would
// drift, and the drift would show as a reply that verified and then could not be
// read back.
func quotedScript(messageID, key string) string {
	return `(() => {
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	const jid = v => (v && v._serialized) ? v._serialized : (typeof v === "string" ? v : "");
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

		const sid = (typeof m.quotedStanzaID === "string") ? m.quotedStanzaID : "";
		if (!sid) { park({ ok: true, notFound: false, quotes: false }); return "kicked"; }

		// O ALVO E' PROCURADO NA COLECAO, e o resultado diz se ele esta' aqui.
		// "Cita a mensagem X" e "X esta carregada" sao fatos diferentes, e um
		// leitor que os fundisse diria que nao ha citacao quando o alvo apenas
		// nao foi hidratado.
		let loaded = false;
		try {
			const all = typeof MC.getModelsArray === "function" ? MC.getModelsArray() : [];
			for (const c of all) {
				if (c.id && c.id.id === sid) { loaded = true; break; }
			}
		} catch (e) {}

		park({
			ok: true, notFound: false, quotes: true,
			quotedId: sid,
			quotedLoaded: loaded,
			quotedChat: jid(m.quotedRemoteJid),
			quotedSender: jid(m.quotedParticipant),
		});
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	return "kicked";
	})()`
}

// mentionsScript reads WHO and WHICH GROUPS a message names.
//
// THE FIELD NAMES WERE MEASURED, NOT GUESSED. H106 looked for mentions across
// 395 loaded messages, found ZERO under five candidate names, and correctly
// refused to ship a reader nobody had seen return anything. The way out was not
// a better guess: it was PRODUCING a mention in the lab group and reading what
// the page then held (H142). Two fields answered, and both answer identically
// through the getter and through the __x_ backing field:
//
//	mentionedJidList  array of Wid   {_serialized, server, user}
//	groupMentions     array of pairs {groupJid, groupSubject}
//
// __x_nonJidMentions IS NOT READ, and that is the H131 lesson paid a second
// time. The first instrument here listed field NAMES and reported that field as
// "populated" on 46 of 62 messages — including messages nobody had mentioned
// anything in. It is not an array; it is a lazy sentinel, and counting it as
// data would have shipped a reader that says every message mentions somebody.
// So this script asks for the SHAPE and takes only arrays.
func mentionsScript(messageID, key string) string {
	return `(() => {
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	const jid = v => (v && v._serialized) ? v._serialized : (typeof v === "string" ? v : "");
	// SO' ARRAY CONTA. Um valor que nao e' lista e' sentinela, nao mencao.
	const list = v => Array.isArray(v) ? v : [];
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

		const people = [];
		for (const w of list(m.mentionedJidList)) {
			const s = jid(w);
			if (s) { people.push(s); }
		}

		const groups = [];
		for (const g of list(m.groupMentions)) {
			if (!g) { continue; }
			const s = jid(g.groupJid);
			// O ASSUNTO VEM CONGELADO NA MENSAGEM, nao do grupo de hoje. Sao
			// fatos diferentes assim que alguem renomear o grupo, e o que a
			// mensagem diz e' o que foi mencionado na hora.
			if (s) { groups.push({ jid: s, subject: (typeof g.groupSubject === "string") ? g.groupSubject : "" }); }
		}

		park({ ok: true, notFound: false, people: people, groups: groups });
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	return "kicked";
	})()`
}

const modMessageInfoStore = "WAWebApiMessageInfoStore"

// infoScript asks WHO received, read and played a message this account sent.
//
// THE COLLECTION WAS THE WRONG INSTRUMENT, and that is why this arrives late.
// H71 measured WAWebMsgInfoCollection empty — 0 of 368 — and concluded this
// build gives ack and not "who read it". The collection is still 0 today, and it
// is not where the answer lives: the reference never reads it. It calls
// WAWebApiMessageInfoStore.queryMsgInfo(msg.id), and the collection is populated
// BY that query rather than instead of it. Measuring a cache before anyone has
// filled it is the same mistake H142 made about mentions, in a different dress.
//
// IT IS ONLY ABOUT OWN MESSAGES, and the reference enforces that before asking
// (`if (!msg || !msg.id.fromMe) return null`). That is not a courtesy: the server
// answers about delivery of things this account sent, and asking about somebody
// else's message is a question with no meaning rather than a permission error.
//
// The reference sleeps INSIDE the page for messages younger than 1250ms. That
// wait belongs to the caller here — invariant 6 keeps the clock on the Go side —
// so this script asks once and reports what it got.
func infoScript(messageID, key string) string {
	return `(() => {
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	const jid = v => (v && v._serialized) ? v._serialized : (typeof v === "string" ? v : "");
	// SO' ARRAY CONTA, pela mesma razao do mentionsScript: um valor que nao e'
	// lista e' sentinela, e conta-la como gente inventa leitores.
	const people = v => {
		if (!Array.isArray(v)) { return []; }
		const out = [];
		for (const e of v) {
			// A entrada e' um recibo, nao um wid: a identidade esta em .id.
			const s = jid(e && e.id) || jid(e);
			if (s) { out.push(s); }
		}
		return out;
	};
	const num = v => (typeof v === "number") ? v : -1;
	(async () => {
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
		if (!m) { park({ ok: true, notFound: true }); return; }
		if (!(m.id && m.id.fromMe)) { park({ ok: true, notFound: false, notMine: true }); return; }

		const info = await window.require("` + modMessageInfoStore + `").queryMsgInfo(m.id);
		if (!info) { park({ ok: true, notFound: false, notMine: false, answered: false }); return; }
		park({
			ok: true, notFound: false, notMine: false, answered: true,
			delivered: people(info.delivery), read: people(info.read), played: people(info.played),
			deliveredRemaining: num(info.deliveryRemaining),
			readRemaining: num(info.readRemaining),
			playedRemaining: num(info.playedRemaining),
		});
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	})();
	return "kicked";
	})()`
}

const modCollections = "WAWebCollections"

// reactionsScript reads WHICH reactions a message carries.
//
// TWO MEASUREMENTS SAID THIS WAS IMPOSSIBLE, and both were right about what they
// measured. H83 concluded the aggregate has no source; H124 remedied that with a
// better instrument, found WAWebCollections.Reactions present with
// on/getModelsArray, saw it read ZERO even after a reaction the capability had
// verified, and concluded the collection does not fill. It does not — measured
// again today. The record comes from an async fetch keyed by the id OBJECT, and
// never lands in that array. See spa.ReactionsForMessageExpr for the full
// measurement, including why the reference's key does not exist here.
//
// IT EMBEDS THE SHARED EXPRESSION rather than carrying its own copy, for the
// reason capabilities/lookup embeds spa.ResolveIdentityExpr: capabilities/react
// verifies its removals against the same fact, and a second copy here would let
// the two drift into a removal that verifies and then reads back as present.
func reactionsScript(messageID, key string) string {
	return `(() => {
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	const readReactions = ` + spa.ReactionsForMessageExpr + `;
	(async () => {
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
		if (!m) { park({ ok: true, notFound: true }); return; }
		const r = await readReactions(m);
		park({ ok: !!r.ok, why: r.why || "", notFound: false, groups: r.groups || [] });
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	})();
	return "kicked";
	})()`
}
