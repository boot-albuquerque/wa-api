package lookup

import (
	"strconv"

	"wa-api/internal/wa-headless/spa"
)

// whyNotOnWhatsApp is the reason string spa.ResolveIdentityExpr answers for a
// number the server does not know. It is a CONSTANT here and a literal there;
// keeping the two in sync is what the test guards, because a rename on the spa
// side would silently turn "not on WhatsApp" into a generic read failure.
const whyNotOnWhatsApp = "NOT_ON_WHATSAPP"

// resolveScript runs the module's OWN identity resolution and parks the answer.
//
// THE EXPRESSION IS NOT COPIED, IT IS EMBEDDED. spa.ResolveIdentityExpr is the
// same text send runs before dispatching; pasting a copy here would create two
// resolutions free to drift, and the drift would show up as a send failing for a
// number this package had just approved.
//
// The store-and-poll wrapper exists because Evaluate does not await promises
// (invariant 6), and the resolution is async.
func resolveScript(jid, key string) string {
	return `(() => {
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	(async () => {
		try {
			const resolve = ` + spa.ResolveIdentityExpr + `;
			const r = await resolve(` + strconv.Quote(jid) + `);
			park({
				ok: !!(r && r.ok),
				why: (r && r.why) || "",
				// SO O JID SERIALIZADO CRUZA. O objeto wid tem metodos e estado
				// interno, e nada disso serviria a quem chama.
				jid: (r && typeof r.jid === "string") ? r.jid : "",
				isGroup: !!(r && r.isGroup),
			});
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}
