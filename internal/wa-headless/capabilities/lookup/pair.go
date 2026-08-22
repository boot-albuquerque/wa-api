package lookup

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"wa-api/internal/wa-headless/spa"
)

// Pair is both identities of one person.
type Pair struct {
	// LID and PN are the two names the same person has on this build. Either can
	// be empty, and empty means "the page did not produce it" — never "there is
	// none". Guessing one from the other is what this type exists to prevent.
	LID, PN string
	// Queried says the server was asked, as opposed to the answer coming from
	// what the page already held. A caller looping over a roster wants to know
	// it just made N network round trips.
	Queried bool
}

// Complete says both identities came back.
func (p Pair) Complete() bool { return p.LID != "" && p.PN != "" }

// String reports shape, never identity.
func (p Pair) String() string {
	return fmt.Sprintf("lookup.Pair(lid=%t pn=%t queried=%t)", p.LID != "", p.PN != "", p.Queried)
}

// LidAndPhone reports both identities of one user.
//
// It is the reference's getContactLidAndPhone, and it is NOT a thin wrapper over
// it: the reference's own helper does not work on this build.
//
// WHAT THE REFERENCE DOES (wwebjs_util.js:1694-1716): branch on whether the input
// is a LID; take the missing half from WAWebApiContact.getCurrentLid or
// getPhoneNumber; and when that misses, call queryWidExists and ask getCurrentLid
// AGAIN.
//
// WHAT WAS MEASURED HERE (H157): the second getCurrentLid still comes back empty.
// Asked about the lab peer's phone jid, the query ran (queried true) and
// getCurrentLid produced nothing — so the reference's helper would return {} for
// a user this module resolves successfully every day. The LID is in the QUERY
// RESULT, which is where spa.ResolveIdentityExpr already reads it.
//
// The isLid branch is kept, and it is not style: calling getPhoneNumber with a
// phone jid throws "WaWebLidPnCache - Invalid get call (not lid)" — measured by
// making that mistake.
//
// IT REUSES THE SHARED RESOLUTION rather than querying on its own, for the reason
// the package doc gives: a second opinion competing with the one send actually
// uses would drift, and the drift would show as a message delivered to an
// identity this reader denies.
func (r *Resolver) LidAndPhone(ctx context.Context, jid, label string) (Pair, error) {
	if strings.TrimSpace(jid) == "" {
		return Pair{}, ErrNoJID
	}
	raw, err := r.parked(ctx, pairScript(jid), label+"/pair")
	if err != nil {
		return Pair{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		LID     string `json:"lid"`
		PN      string `json:"pn"`
		Queried bool   `json:"queried"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Pair{}, fmt.Errorf("lookup: unexpected pair answer: %w", e)
	}
	if !out.OK {
		if out.Why == whyNotOnWhatsApp {
			return Pair{}, ErrNotOnWhatsApp
		}
		return Pair{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	return Pair{LID: out.LID, PN: out.PN, Queried: out.Queried}, nil
}

func pairScript(jid string) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	const ser = v => (v && v._serialized) ? v._serialized : (typeof v === "string" ? v : "");
	const resolve = ` + spa.ResolveIdentityExpr + `;
	(async () => {
	try {
		const W = window.require("WAWebWidFactory");
		const asked = W.createWid(` + strconv.Quote(jid) + `);
		const isLid = asked.server === "lid";

		if (isLid) {
			// O LADO CONHECIDO E' O DE ENTRADA. O telefone vem do cache de
			// mapeamento, e getPhoneNumber SO' aceita lid — com um pn ela lanca
			// "WaWebLidPnCache - Invalid get call (not lid)".
			let pn = null;
			try { pn = window.require("WAWebApiContact").getPhoneNumber(asked); } catch (e) {}
			park({ ok: true, lid: ser(asked), pn: ser(pn), queried: false });
			return;
		}

		// PN NA ENTRADA: o LID vem da RESOLUCAO, nao do getCurrentLid. Medido: o
		// getCurrentLid continua vazio mesmo depois da consulta, que e' o passo
		// em que a referencia confia.
		const got = await resolve(` + strconv.Quote(jid) + `);
		if (!got || !got.ok) {
			park({ ok: false, why: (got && got.why) || "RESOLVE_FAILED", queried: true });
			return;
		}
		park({ ok: true, lid: got.jid || "", pn: ser(asked), queried: true });
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	})();
	return "kicked";
	})()`
}
