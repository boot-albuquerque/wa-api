package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

type double struct {
	answer       string
	pendingReads int

	reads      int
	kicks      int
	lastScript string
}

func (d *double) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "delete window."+stateKeyPrefix) || strings.HasPrefix(expr, "window."+stateKeyPrefix) {
		d.reads++
		if d.reads <= d.pendingReads {
			*out = ""
			return nil
		}
		*out = d.answer
		return nil
	}
	d.kicks++
	d.lastScript = expr
	*out = "kicked"
	return nil
}

func rd(d *double) *Reader { return New(engine.NewRunner(), d.eval) }

func withoutComments(script string) string {
	var b strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

const seller = "5511999999999@c.us"

// An empty seller never reaches the page.
func TestAnEmptySellerIsRefused(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).Of(context.Background(), "   ", "t"); !errors.Is(err, ErrNoSeller) {
		t.Errorf("err = %v, want ErrNoSeller", err)
	}
	if d.kicks != 0 {
		t.Error("an empty seller reached the page")
	}
}

// A SELLER WITHOUT A SHOP IS NOT A BROKEN CALL.
//
// The server declines per seller with ServerStatusCodeError — measured on two of
// three sellers tried, while the third returned a product. Folding that into a
// read error would make every shopless seller look like a defect.
func TestASellerWithNoShopIsItsOwnAnswer(t *testing.T) {
	d := &double{answer: `{"ok":true,"noShop":true}`}
	_, err := rd(d).Of(context.Background(), seller, "t")
	if !errors.Is(err, ErrNoCatalog) {
		t.Fatalf("err = %v, want ErrNoCatalog", err)
	}
	// And the page script must be the one that decides it, by the error the
	// server actually sends.
	d2 := &double{answer: `{"ok":true,"noShop":true}`}
	_, _ = rd(d2).Of(context.Background(), seller, "t")
	code := withoutComments(d2.lastScript)
	if !strings.Contains(code, "ServerStatusCode") {
		t.Error("the script does not recognise the server's per-seller refusal")
	}
}

// THE ITEMS LIVE UNDER data, and the reference's shape would read zero forever.
//
// The measured answer is {data, catalog_id, catalog_name, catalog_type, paging}.
// Reading products instead of data is a reader that never returns anything —
// the exact defect class this repository has met three times.
func TestTheItemsAreReadFromTheMeasuredField(t *testing.T) {
	d := &double{answer: `{"ok":true,"id":"CAT1","name":"loja","kind":"NORMAL","products":[` +
		`{"id":"P1","retailerId":"SKU1","name":"item","description":"d",` +
		`"price":1990,"currency":"BRL","hidden":false,"availability":"in stock","images":3}]}`}
	got, err := rd(d).Of(context.Background(), seller, "t")
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	if len(got.Products) != 1 {
		t.Fatalf("%d products, want 1", len(got.Products))
	}
	p := got.Products[0]
	if p.Price != 1990 || p.Currency != "BRL" {
		t.Errorf("price/currency lost: %+v", p)
	}
	if p.Images != 3 {
		t.Errorf("images = %d, want 3", p.Images)
	}
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "Array.isArray(r.data)") {
		t.Error("the script does not read the items from data")
	}
}

// THE PRICE IS NOT CONVERTED. It is the page's integer in the currency's
// smallest unit; dividing by the wrong power of ten is a defect nobody notices
// until somebody is charged.
func TestThePriceIsNotConverted(t *testing.T) {
	d := &double{answer: `{"ok":true,"products":[{"id":"P1","price":1990,"currency":"BRL"}]}`}
	got, err := rd(d).Of(context.Background(), seller, "t")
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	if got.Products[0].Price != 1990 {
		t.Fatalf("price = %d; it must cross verbatim", got.Products[0].Price)
	}
	code := withoutComments(d.lastScript)
	for _, banned := range []string{"/ 100", "/100", "* 0.01", "toFixed("} {
		if strings.Contains(code, banned) {
			t.Errorf("the script converts the price with %q", banned)
		}
	}
}

// PAGING IS REPORTED, NOT FOLLOWED. Following it silently turns one read into an
// unbounded number of them.
func TestPagingIsReportedNotFollowed(t *testing.T) {
	d := &double{answer: `{"ok":true,"more":true,"products":[]}`}
	got, err := rd(d).Of(context.Background(), seller, "t")
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	if !got.MorePages {
		t.Fatal("the paging flag was dropped")
	}
	code := withoutComments(d.lastScript)
	if strings.Contains(code, "while") || strings.Contains(code, "for (;;)") {
		t.Error("the script loops; paging must be the caller's decision")
	}
}

// IMAGE URLS DO NOT CROSS. They are large, they expire, and nothing here needs
// them — only how many there are.
func TestImageUrlsDoNotCross(t *testing.T) {
	d := &double{answer: `{"ok":true,"products":[{"id":"P1","images":2}]}`}
	if _, err := rd(d).Of(context.Background(), seller, "t"); err != nil {
		t.Fatalf("Of: %v", err)
	}
	code := withoutComments(d.lastScript)
	if strings.Contains(code, "image_cdn_urls[") || strings.Contains(code, "urls.join") {
		t.Error("the script carries image urls across the boundary")
	}
	if !strings.Contains(code, "image_cdn_urls.length") &&
		!strings.Contains(code, "image_cdn_urls) ? p.image_cdn_urls.length") {
		t.Error("the script does not count the images")
	}
}

// A page failure keeps its reason and is NOT a shopless seller.
func TestAPageFailureIsNotAShoplessSeller(t *testing.T) {
	d := &double{answer: `{"ok":false,"why":"TypeError message=nope"}`}
	_, err := rd(d).Of(context.Background(), seller, "t")
	if !errors.Is(err, ErrRead) || errors.Is(err, ErrNoCatalog) {
		t.Fatalf("err = %v, want ErrRead and not ErrNoCatalog", err)
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("the reason was dropped: %v", err)
	}
}

// The renderings carry no content.
func TestTheRenderingsAreQuiet(t *testing.T) {
	c := Catalog{SellerJID: seller, ID: "CAT1", Name: "Loja do Fulano",
		Products: []Product{{ID: "P1", Name: "Camiseta", Price: 1990, Currency: "BRL"}}}
	if strings.Contains(c.String(), "Loja") || strings.Contains(c.String(), "5511") {
		t.Errorf("Catalog.String carries content: %s", c.String())
	}
	if strings.Contains(c.Products[0].String(), "Camiseta") ||
		strings.Contains(c.Products[0].String(), "1990") {
		t.Errorf("Product.String carries content: %s", c.Products[0].String())
	}
}

// The parked loop is bounded.
func TestTheParkedLoopIsBounded(t *testing.T) {
	ob, ot := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = ob, ot }()

	d := &double{pendingReads: 1 << 30}
	if _, err := rd(d).Of(context.Background(), seller, "t"); err == nil ||
		!strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}
