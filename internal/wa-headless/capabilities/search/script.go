package search

import "strconv"

const (
	modCollections = "WAWebCollections"
	// pageCount is what the page is ASKED for. Every measured call answered 20
	// regardless, so this is a request and not a promise — Result.Returned
	// reports what actually came back.
	pageCount = 50
)

// searchScript asks the page and parks ADDRESSES.
//
// THE BODY NEVER LEAVES THE PAGE. The projection here is the guard: only chat,
// id, direction, type and timestamp are copied out of each match, so a body
// cannot reach Go even by accident. Serialising the message and filtering in Go
// would put the body in the answer first, which is the same mistake H107
// refused for rawData.
func searchScript(query string, page int) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	const str = v => (typeof v === "string" ? v : "");
	(async () => {
		try {
			const C = window.require("` + modCollections + `");
			// SEM ESCOPO DE CONVERSA. Passar o jid do chat como quarto argumento
			// — que e' o que a referencia faz com options.chatId — devolveu 0
			// resultados com eof, enquanto a forma sem escopo devolveu 20.
			const r = await C.Msg.search(
				` + strconv.Quote(query) + `,
				` + strconv.Itoa(page) + `,
				` + strconv.Itoa(pageCount) + `,
				undefined);
			const ms = (r && r.messages) || [];
			const hits = [];
			for (const m of ms) {
				try {
					const id = m.id;
					const chat = id && id.remote;
					hits.push({
						chat: (chat && chat._serialized) ? chat._serialized : str(chat),
						id: str(id && id.id),
						fromMe: !!(id && id.fromMe),
						type: str(m.type),
						t: (typeof m.t === "number") ? m.t : 0,
					});
				} catch (e) {}
			}
			park({ ok: true, eof: !!(r && r.eof), hits: hits });
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}
