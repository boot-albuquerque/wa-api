// Package catalog reads a seller's product catalog.
//
// IT READS SOMEBODY ELSE'S SHOP WINDOW, and that framing is what made the family
// implementable without asking a human for anything. queryCatalog exists for a
// CUSTOMER opening a seller's catalog. The obvious route — create a product on
// this account so there is something to read — would have left a real item
// visible on a real business profile. Reading a catalog that already exists
// creates nothing (H103).
//
// WHAT THE MEASUREMENT CORRECTED. The reference's Product structure suggested a
// products field; the real answer is shaped
//
//	{ data, catalog_id, catalog_name, catalog_type, paging }
//
// with the items under data. And a seller with no catalog answers
// ServerStatusCodeError — which is a refusal about that seller, not about the
// call. Two of the three sellers tried refused that way and the third returned
// one product, which is the only reason the refusal can be read correctly: a
// reader never seen returning non-zero cannot tell "empty" from "broken" (H93).
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

var (
	// ErrNoSeller is an empty seller jid.
	ErrNoSeller = fmt.Errorf("catalog: no seller given")
	// ErrRead is the page refusing or failing the read.
	ErrRead = fmt.Errorf("catalog: the page refused the catalog read")
	// ErrNoCatalog is a seller the server has no catalog for.
	//
	// IT IS NOT A FAILURE OF THE CALL. The page answers ServerStatusCodeError,
	// which is the server declining for THAT seller — measured on two of three
	// sellers tried, while the third returned a product. Reporting it as a read
	// error would make every shopless seller look like a broken capability.
	ErrNoCatalog = fmt.Errorf("catalog: that seller has no catalog")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 250 * time.Millisecond
)

// stateKeyPrefix names the page global a call parks its answer on.
//
// IT IS A PREFIX, NOT A KEY (H177). One shared global meant two concurrent
// calls on the same session overwrote each other and each polled until
// non-empty, so one could take the other's answer — measured in
// capabilities/message at 12 crossings in 12 rounds. The nonce comes from Go:
// a page-side Math.random or Date.now would put a decision and a clock where
// invariant 6 forbids them.
const stateKeyPrefix = "__waHeadlessCatalog"

var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Product is one item in a catalog.
//
// PRICE AND NAME ARE CONTENT, and they are carried because a catalog reader
// exists to show them — but String renders none of it. The image urls are
// deliberately absent: they are large, they expire, and nothing this module does
// needs them.
type Product struct {
	// ID is the product's own identifier; RetailerID is the seller's SKU.
	ID, RetailerID string
	Name           string
	Description    string
	// Price is the page's own integer, in the currency's smallest unit, with
	// Currency beside it. It is NOT converted here: a price divided by the
	// wrong power of ten is a bug nobody notices until somebody is charged.
	Price    int64
	Currency string
	// Hidden says the seller has it in the catalog but not on display.
	Hidden bool
	// Availability is the page's own word ("in stock", …), carried verbatim
	// rather than mapped, because a value this package has not seen must not be
	// turned into one it has.
	Availability string
	// Images is how many image urls the item carries. The urls themselves are
	// not kept.
	Images int
}

func (p Product) String() string {
	return fmt.Sprintf("catalog.Product(id=%t name=%t price=%t currency=%s hidden=%t images=%d)",
		p.ID != "", p.Name != "", p.Price != 0, p.Currency, p.Hidden, p.Images)
}

// Catalog is one seller's shop window.
type Catalog struct {
	SellerJID string
	// ID and Name are the catalog's own, as the page reports them.
	ID, Name string
	Kind     string
	Products []Product
	// MorePages says the answer was paged and this is not all of it. Reported
	// rather than followed: paging is a decision for the caller, and following
	// it silently would turn one read into an unbounded number.
	MorePages bool
}

func (c Catalog) String() string {
	return fmt.Sprintf("catalog.Catalog(seller=%t id=%t name=%t kind=%s products=%d morePages=%t)",
		c.SellerJID != "", c.ID != "", c.Name != "", c.Kind, len(c.Products), c.MorePages)
}

// Reader reads catalogs.
type Reader struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Reader {
	return &Reader{runner: runner, eval: eval}
}

// Of reads one seller's catalog.
func (r *Reader) Of(ctx context.Context, sellerJID, label string) (Catalog, error) {
	if strings.TrimSpace(sellerJID) == "" {
		return Catalog{}, ErrNoSeller
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, catalogScript(sellerJID, key), key, label+"/catalog")
	if err != nil {
		return Catalog{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		NoShop   bool   `json:"noShop"`
		ID       string `json:"id"`
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		More     bool   `json:"more"`
		Products []struct {
			ID           string `json:"id"`
			RetailerID   string `json:"retailerId"`
			Name         string `json:"name"`
			Description  string `json:"description"`
			Price        int64  `json:"price"`
			Currency     string `json:"currency"`
			Hidden       bool   `json:"hidden"`
			Availability string `json:"availability"`
			Images       int    `json:"images"`
		} `json:"products"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Catalog{}, fmt.Errorf("catalog: unexpected answer: %w", e)
	}
	if out.NoShop {
		return Catalog{}, ErrNoCatalog
	}
	if !out.OK {
		return Catalog{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	c := Catalog{SellerJID: sellerJID, ID: out.ID, Name: out.Name, Kind: out.Kind, MorePages: out.More}
	for _, p := range out.Products {
		c.Products = append(c.Products, Product{
			ID: p.ID, RetailerID: p.RetailerID, Name: p.Name, Description: p.Description,
			Price: p.Price, Currency: p.Currency, Hidden: p.Hidden,
			Availability: p.Availability, Images: p.Images,
		})
	}
	return c, nil
}

func (r *Reader) parked(ctx context.Context, kick, key, label string) (string, error) {
	var started string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return r.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(Budget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/read", func(c context.Context) error {
			return r.eval(c, `window.`+key+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			// A CHAVE E' LIBERADA ao ser lida: sem isso a correcao troca uma
			// resposta cruzada por um global de pagina POR CHAMADA (H177).
			var ignored string
			_ = r.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return r.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			return raw, nil
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("the page never settled within %s", Budget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(Tick):
		}
	}
}
