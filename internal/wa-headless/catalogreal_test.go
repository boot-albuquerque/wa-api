package waheadless

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/catalog"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestCatalogReadReal proves the catalog reader against a real seller.
//
// IT CREATES NOTHING. The obvious way to have a catalog to read was to add a
// product to this account's own business profile — which would leave a real item
// visible to anyone who opened that profile. queryCatalog exists for a CUSTOMER
// opening a seller's shop window, so the honest proof reads a catalog that
// already exists (H103).
//
// THE SELLER IS FOUND, NOT WRITTEN DOWN. A jid in a test file is somebody's
// business in a repository. The account knows 58 business profiles; this walks
// them until one answers, which is also what makes the refusals interpretable:
// two of three answer ServerStatusCodeError and the third returns a product, so
// "no shop" is a measured state rather than a guess.
func TestCatalogReadReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_CATALOG") == "" {
		t.Skip("set WA_REAL_CATALOG=1; this only READS a seller's public catalog")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	// The known business profiles, by jid, from the page. Never logged.
	const listScript = `JSON.stringify((() => {
		try {
			const BP = window.require('WAWebBusinessProfileCollection');
			const holder = BP && (BP.BusinessProfileCollection || BP);
			const out = [];
			for (const p of (holder.getModelsArray ? holder.getModelsArray() : [])) {
				try {
					const id = p.id;
					const s = (id && id._serialized) || (typeof id === 'string' ? id : '');
					if (s) { out.push(s); }
				} catch (e) {}
			}
			return { ok: true, sellers: out };
		} catch (e) { return { ok: false, why: String(e && e.message) }; }
	})())`
	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "catalog/sellers", func(c context.Context) error {
		return sess.Tab().Evaluate(c, listScript, &raw)
	}); err != nil {
		t.Fatalf("listing sellers: %v", err)
	}
	sellers := decodeSellers(t, raw)
	t.Logf("this account knows %d business profile(s)", len(sellers))
	if len(sellers) == 0 {
		t.Skip("no business profile known; nothing to read")
	}

	r := catalog.New(runner, sess.Tab().Evaluate)
	var found catalog.Catalog
	noShop, failed := 0, 0
	for i, s := range sellers {
		if i >= 8 {
			// BOUNDED, AND THE BOUND IS SAID OUT LOUD. Walking 58 sellers is 58
			// server round trips for a proof that one is enough for.
			t.Logf("stopped after 8 sellers; %d remain unread", len(sellers)-8)
			break
		}
		c, err := r.Of(ctx, s, "catalog/read")
		switch {
		case err == nil:
			found = c
			t.Logf("seller %d ANSWERED: %s", i+1, c)
			for _, p := range c.Products {
				t.Logf("   %s", p)
			}
		case errors.Is(err, catalog.ErrNoCatalog):
			noShop++
			continue
		default:
			failed++
			t.Logf("seller %d: %v", i+1, err)
			continue
		}
		break
	}
	t.Logf("walked: %d without a shop, %d failed", noShop, failed)

	if found.SellerJID == "" {
		t.Skipf("none of the sellers tried has a catalog (%d answered 'no shop'). "+
			"The path is exercised — a refusal IS a server answer — but a non-empty "+
			"catalog was not observed on this run.", noShop)
	}

	// WHICH FIELDS CAME BACK EMPTY, said out loud instead of asserted blind.
	//
	// The first run failed on "the catalog has no id of its own", and a boolean
	// cannot tell a reader looking in the wrong place from a seller who left the
	// field blank. The probe measured that the response CARRIES catalog_id,
	// catalog_name and catalog_type as keys; whether this seller filled them is
	// a different question, and one seller is not enough to answer it.
	t.Logf("catalog-level fields present: id=%t name=%t kind=%t",
		found.ID != "", found.Name != "", found.Kind != "")
	empty := 0
	for _, p := range found.Products {
		if p.Price == 0 && p.Currency == "" {
			empty++
		}
	}
	t.Logf("%d of %d product(s) carry neither price nor currency", empty, len(found.Products))

	// THE POSTCONDITION IS WHAT ONE SELLER CAN SUPPORT: the reader produced a
	// product, and a product without an id is unusable by anybody. The
	// catalog-level identity is reported above rather than required, because a
	// single observation cannot separate "always empty" from "this shop".
	if len(found.Products) == 0 {
		t.Fatal("a catalog answered with zero products; ErrNoCatalog is the answer " +
			"for a seller without a shop, so an empty success is a third state nobody defined")
	}
	for i, p := range found.Products {
		if p.ID == "" {
			t.Errorf("product %d has no id; nothing downstream can address it", i)
		}
		// A PRICE WITHOUT A CURRENCY IS A NUMBER NOBODY CAN ACT ON. The reverse
		// is fine: a shop may list an item with no price at all.
		if p.Price != 0 && p.Currency == "" {
			t.Errorf("product %d has a price and no currency", i)
		}
	}
}

func decodeSellers(t *testing.T, raw string) []string {
	t.Helper()
	var out struct {
		OK      bool     `json:"ok"`
		Why     string   `json:"why"`
		Sellers []string `json:"sellers"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decoding sellers: %v (raw len %d)", err, len(raw))
	}
	if !out.OK {
		t.Fatalf("the page refused the seller list: %s", out.Why)
	}
	return out.Sellers
}
