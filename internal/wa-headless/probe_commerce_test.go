package waheadless

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
	"wa-api/internal/wa-headless/spa"
)

// TestProbeCommerce measures the commerce surface (catalog / product / order /
// payment) BEFORE anything is designed against it. The LEDGER marks the whole
// family MISSING and never attacked.
//
// THE REFERENCE (whatsapp-web.js @ f935b500117e264c2b3abc25b63a280bd98182a7)
// answers "what does commerce look like" in one sentence: there is no
// Client-level catalog listing at all. `grep -i "product\|catalog\|order\|
// payment" src/Client.js` returns NOTHING. Every commerce read hangs off a
// MESSAGE, not off the client:
//
//   - Message#getOrder()   -> only when msg.type === 'order', calls
//     WWebJS.getOrderDetail(orderId, token, chatId), which is
//     WAWebBizOrderBridge.queryOrder(chatWid, orderId, 80, 80, token) — five
//     arguments, two of them hardcoded thumbnail dimensions.
//   - Message#getPayment() -> only when msg.type === 'payment', re-reads the
//     message itself from the collection and re-serializes it — no bridge
//     call at all, the payment fields already live on the message.
//   - Product#getData()    -> WWebJS.getProductMetadata(productId), which is
//     WAWebBizProductCatalogBridge.queryProduct(sellerId, productId) — two
//     arguments, sellerId from WAWebConnModel.Conn.wid (the account's own
//     wid, never a parameter).
//
// whatsapp-web.js's own MessageTypes name the three message kinds this reads:
// 'order', 'product', 'payment' (src/util/Constants.js). Order messages carry
// raw `orderId` and `token` fields on the message model; product messages
// carry `productId`.
//
// Baileys' Socket/business.ts additionally proves the ACCOUNT ITSELF can have
// a catalog independent of any message (`getCatalog`, `getCollections`,
// queried by JID, defaulting to the account's own). Baileys builds that as a
// raw WASmax IQ, which says nothing about what this build's PAGE calls it —
// that is exactly the kind of question only window.require can answer, so
// this probe also enumerates catalog-shaped module names besides the two the
// reference names, in case this build has a first-class listing whatsapp-web.js
// had no equivalent for (the same shape as the calls probe's VOIP surface).
//
// READ ONLY. Nothing is created, nothing is sent, no product/order/payment is
// mutated, and PAYMENT ESPECIALLY: no call here can move value — the payment
// read (if it ever fires) only re-serializes a message already in the
// collection, exactly like the reference.
func TestProbeCommerce(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_COMMERCE") == "" {
		t.Skip("set WA_PROBE_COMMERCE=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	kick := `(() => {
		window.__commerce = null;
		const park = v => { window.__commerce = JSON.stringify(v); };
		// Redact before reporting: an error message on this page quotes the
		// identifier it refused, and nothing here carries an identity.
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, '<redacted>').slice(0, 160);
		// H51's lesson, applied before it costs anything: an OBVIOUS field or
		// getter can answer for one record out of hundreds while another one
		// answers for all of them. Report BOTH raw fields and getter names —
		// never trust the raw field alone.
		const getterNames = obj => {
			const names = new Set();
			let proto = obj && Object.getPrototypeOf(obj);
			while (proto && proto !== Object.prototype) {
				for (const n of Object.getOwnPropertyNames(proto)) {
					if (/^get[A-Z]/.test(n)) {
						try { if (typeof proto[n] === 'function') names.add(n); } catch (e) {}
					}
				}
				proto = Object.getPrototypeOf(proto);
			}
			return [...names].sort();
		};
		(async () => {
		const out = { modules: {}, arity: {}, catalogAlternatives: {}, collection: {}, invocations: {} };
		const load = name => {
			try {
				const m = window.require(name);
				out.modules[name] = m ? Object.keys(m).slice(0, 40) : 'falsy';
				return m;
			} catch (e) {
				out.modules[name] = 'ABSENT: ' + safe(e);
				return null;
			}
		};
		try {
			// THE TWO BRIDGES THE REFERENCE NAMES.
			const OrderBridge = load('` + string(spa.Module("WAWebBizOrderBridge")) + `');
			const ProductBridge = load('` + string(spa.Module("WAWebBizProductCatalogBridge")) + `');
			const Conn = load('` + string(spa.Module("WAWebConnModel")) + `');
			load('` + string(spa.ModuleWidFactory) + `');

			// ARITY the reference USES vs the arity the function DECLARES.
			// queryOrder(chatWid, orderId, 80, 80, token) -> 5 args passed.
			// queryProduct(sellerId, productId) -> 2 args passed.
			if (OrderBridge && typeof OrderBridge.queryOrder === 'function') {
				out.arity.queryOrder = { declared: OrderBridge.queryOrder.length, referencePasses: 5 };
			} else {
				out.arity.queryOrder = 'absent';
			}
			if (ProductBridge && typeof ProductBridge.queryProduct === 'function') {
				out.arity.queryProduct = { declared: ProductBridge.queryProduct.length, referencePasses: 2 };
			} else {
				out.arity.queryProduct = 'absent';
			}

			// THE ACCOUNT'S OWN SELLER WID — the parameter the reference never
			// exposes as one (it reads Conn.wid internally). Reported only as
			// PRESENT/ABSENT, never the value.
			try {
				out.sellerWidPresent = !!(Conn && Conn.Conn && Conn.Conn.wid);
			} catch (e) {
				out.sellerWidPresent = 'threw: ' + safe(e);
			}

			// WHAT BAILEYS KNOWS THAT THE REFERENCE DOES NOT: this account can
			// own a catalog independent of any message. If this build's PAGE has
			// a first-class "list my own catalog" module, it will have a name
			// none of the two bridges above carries.
			for (const name of [
				'WAWebBizCatalogCollection', 'WAWebCatalogCollection',
				'WAWebBizProductCollection', 'WAWebProductCollection',
				'WAWebBizCatalogBridge', 'WAWebBizCollectionsBridge',
				'WAWebBizOrderCollection', 'WAWebOrderCollection',
				'WAWebPaymentCollection', 'WAWebBizPaymentCollection',
			]) {
				try {
					const m = window.require(name);
					out.catalogAlternatives[name] = m ? Object.keys(m).slice(0, 30) : 'falsy';
				} catch (e) {
					out.catalogAlternatives[name] = 'absent';
				}
			}
		} catch (e) {
			out.fatal = safe(e);
		}
		park(out);

		// THE MESSAGE COLLECTION — where the reference says order/product/
		// payment actually live. Scanned exactly the way messagemeta.go
		// already reads it in this build: window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection,
		// same module this codebase already trusts, not WAWebCollections (the
		// reference's name, which is a DIFFERENT module in this build).
		try {
			const c = window.require('` + string(spa.ModuleMsgCollection) + `');
			const coll = c && c.MsgCollection;
			out.collection.hasCollection = !!coll;
			out.collection.hasModelsArray = typeof (coll && coll.getModelsArray) === 'function';
			if (coll && typeof coll.getModelsArray === 'function') {
				const models = coll.getModelsArray();
				out.collection.totalMessages = models.length;
				const counts = { order: 0, product: 0, payment: 0 };
				const sampleFields = { order: null, product: null, payment: null };
				const sampleGetters = { order: null, product: null, payment: null };
				let firstOrder = null, firstPayment = null;
				for (const m of models) {
					const t = (typeof m.type === 'string') ? m.type : '';
					if (t !== 'order' && t !== 'product' && t !== 'payment') continue;
					counts[t]++;
					if (!sampleFields[t]) {
						sampleFields[t] = Object.keys(m).filter(k => !k.startsWith('_')).sort();
						sampleGetters[t] = getterNames(m);
					}
					if (t === 'order' && !firstOrder) firstOrder = m;
					if (t === 'payment' && !firstPayment) firstPayment = m;
				}
				out.collection.counts = counts;
				out.collection.sampleFields = sampleFields;
				out.collection.sampleGetters = sampleGetters;

				// ONLY IF A REAL COMMERCE MESSAGE EXISTS: exercise the read with
				// ITS OWN identifiers (never fabricated ones), the same
				// restraint the calls probe used for real call-link shapes.
				// Guarded by a timeout so a hang reports as a hang, not as a
				// silent stall (H92's lesson).
				const withTimeout = (p, ms) => Promise.race([
					p.then(v => ({ ok: true, v })).catch(e => ({ ok: false, e })),
					new Promise(res => setTimeout(() => res({ ok: false, timeout: true }), ms)),
				]);
				if (firstOrder && firstOrder.orderId && firstOrder.token) {
					out.invocations.queryOrder = 'PENDING';
					park(out);
					const OrderBridge = window.require('` + string(spa.Module("WAWebBizOrderBridge")) + `');
					const W = window.require('` + string(spa.ModuleWidFactory) + `');
					const chatWid = W.createWid(firstOrder.id.remote);
					const r = await withTimeout(
						OrderBridge.queryOrder(chatWid, firstOrder.orderId, 80, 80, firstOrder.token), 10000);
					if (r.timeout) out.invocations.queryOrder = 'TIMED_OUT_10s';
					else if (r.ok) out.invocations.queryOrder = 'resolved: ' + typeof r.v +
						(r.v && typeof r.v === 'object' ? ' keys=[' + Object.keys(r.v).join(',') + ']' : '');
					else out.invocations.queryOrder = 'threw: ' + safe(r.e);
					park(out);
				} else {
					out.invocations.queryOrder = 'skipped: no order message with orderId+token found';
				}
				if (!firstPayment) {
					out.invocations.getPayment = 'skipped: no payment message found';
				} else {
					out.invocations.getPayment = 'found payment message; not re-read (re-serialize is redundant with sampleFields above)';
				}
			}
		} catch (e) {
			out.collection.fatal = safe(e);
		}

		out.finished = true;
		park(out);
		})();
		return 'kicked';
	})()`

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/commerce-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}

	var raw string
	for i := 0; i < 60; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/commerce-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__commerce || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(raw, `"finished":true`) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if raw == "" {
		t.Fatal("the probe never parked anything at all")
	}
	if !strings.Contains(raw, `"finished":true`) {
		t.Logf("the probe did NOT finish; whatever is marked PENDING below is the call that hung")
	}
	t.Logf("%s", raw)
}
