package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/coder/websocket"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
)

// Controller is one way of driving the SAME externally launched browser.
//
// Every implementation must perform the same nine observable steps, so that a
// difference in the numbers is a difference in the controller and not in what
// the controller was asked to do.
type Controller interface {
	Name() string
	Connect(ctx context.Context, wsURL string) error
	RunJob(ctx context.Context, pageURL string, rec *Rec) error
	Close() error
}

// Rec accumulates per-operation latencies for one controller.
type Rec struct {
	mu    sync.Mutex
	Ops   map[string]*Lat
	Total Lat
	Fails int
}

func NewRec() *Rec { return &Rec{Ops: map[string]*Lat{}} }

func (r *Rec) op(name string, start time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.Ops[name]
	if !ok {
		l = &Lat{}
		r.Ops[name] = l
	}
	l.Add(time.Since(start))
}

// timed runs f, records its duration under name, and propagates the error.
//
// With STEP_TRACE set it also narrates each step to stderr. A job that times out
// otherwise reports only "context deadline exceeded", which names the deadline
// and not the step that consumed it.
func (r *Rec) timed(name string, f func() error) error {
	t := time.Now()
	err := f()
	r.op(name, t)
	if os.Getenv("STEP_TRACE") != "" {
		fmt.Fprintf(os.Stderr, "   step %-14s %7.0fms err=%v\n", name,
			float64(time.Since(t).Microseconds())/1000, err)
	}
	return err
}

// ---------------------------------------------------------------------------
// 1. cdp-min — the lower bound: Go -> WebSocket -> CDP -> Chrome.
//
// It is deliberately minimal, and that makes it a FLOOR rather than a peer:
// DOM operations go through Runtime.evaluate instead of the DOM domain's node
// ids, so it does less work than the real controllers, not the same work more
// cheaply. Reported as a baseline of protocol cost, never as a winner.
// ---------------------------------------------------------------------------

type cdpMin struct {
	conn    *websocket.Conn
	ctx     context.Context
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan json.RawMessage
	closed  chan struct{}
}

type cdpMsg struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    any             `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *cdpMin) Name() string { return "cdp-min" }

func (c *cdpMin) Connect(ctx context.Context, wsURL string) error {
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return err
	}
	// CDP payloads (a serialized DOM, a screenshot) exceed the library default.
	conn.SetReadLimit(64 << 20)
	c.conn, c.ctx = conn, ctx
	c.pending = map[int64]chan json.RawMessage{}
	c.closed = make(chan struct{})
	go c.readLoop()
	return nil
}

func (c *cdpMin) readLoop() {
	for {
		_, data, err := c.conn.Read(c.ctx)
		if err != nil {
			close(c.closed)
			return
		}
		var m cdpMsg
		if json.Unmarshal(data, &m) != nil || m.ID == 0 {
			continue // an event, not a reply: this baseline ignores events
		}
		c.mu.Lock()
		ch, ok := c.pending[m.ID]
		delete(c.pending, m.ID)
		c.mu.Unlock()
		if !ok {
			continue
		}
		if m.Error != nil {
			ch <- json.RawMessage(`{"__error":` + strconvQuote(m.Error.Message) + `}`)
			continue
		}
		ch <- m.Result
	}
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

func (c *cdpMin) call(sessionID, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan json.RawMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	b, err := json.Marshal(cdpMsg{ID: id, Method: method, Params: params, SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	if err := c.conn.Write(c.ctx, websocket.MessageText, b); err != nil {
		return nil, err
	}
	select {
	case res := <-ch:
		if strings.Contains(string(res), `"__error"`) {
			return nil, fmt.Errorf("cdp: %s", res)
		}
		return res, nil
	case <-c.closed:
		return nil, fmt.Errorf("cdp: connection closed")
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("cdp: timeout on %s", method)
	}
}

// eval runs an expression in the page and returns the JSON value.
func (c *cdpMin) eval(sess, expr string) (json.RawMessage, error) {
	res, err := c.call(sess, "Runtime.evaluate", map[string]any{
		"expression": expr, "returnByValue": true, "awaitPromise": true,
	})
	if err != nil {
		return nil, err
	}
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, err
	}
	return r.Result.Value, nil
}

// waitFor polls an expression until it yields true. Polling from the client is
// the naive approach; the real controllers use DOM events or Runtime bindings.
// The difference is part of what this baseline exists to expose.
func (c *cdpMin) waitFor(sess, expr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		v, err := c.eval(sess, expr)
		if err == nil && string(v) == "true" {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("cdp-min: timeout waiting for %s", expr)
}

func (c *cdpMin) RunJob(ctx context.Context, pageURL string, rec *Rec) error {
	var targetID, sessionID string

	if err := rec.timed("page_create", func() error {
		res, err := c.call("", "Target.createTarget", map[string]any{"url": "about:blank"})
		if err != nil {
			return err
		}
		var r struct {
			TargetID string `json:"targetId"`
		}
		if err := json.Unmarshal(res, &r); err != nil {
			return err
		}
		targetID = r.TargetID
		res, err = c.call("", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true})
		if err != nil {
			return err
		}
		var a struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(res, &a); err != nil {
			return err
		}
		sessionID = a.SessionID
		return nil
	}); err != nil {
		return err
	}
	defer func() {
		_, _ = c.call("", "Target.closeTarget", map[string]any{"targetId": targetID})
	}()

	if err := rec.timed("navigate", func() error {
		_, err := c.call(sessionID, "Page.navigate", map[string]any{"url": pageURL})
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("wait_ready", func() error {
		return c.waitFor(sessionID, `!!document.getElementById('app-ready')`, 30*time.Second)
	}); err != nil {
		return err
	}
	if err := rec.timed("dom_read", func() error {
		_, err := c.eval(sessionID, `document.getElementById('title').textContent`)
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("fill", func() error {
		_, err := c.eval(sessionID, `(()=>{const e=document.getElementById('email');e.value='a@b.c';return true})()`)
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("click", func() error {
		_, err := c.eval(sessionID, `(()=>{document.getElementById('submit').click();return true})()`)
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("wait_result", func() error {
		return c.waitFor(sessionID, `!!document.querySelector('#result[data-done="1"]')`, 30*time.Second)
	}); err != nil {
		return err
	}
	return rec.timed("evaluate", func() error {
		_, err := c.eval(sessionID, `document.querySelectorAll('.item').length`)
		return err
	})
}

func (c *cdpMin) Close() error {
	if c.conn != nil {
		return c.conn.Close(websocket.StatusNormalClosure, "")
	}
	return nil
}

// ---------------------------------------------------------------------------
// 2. chromedp — high-level API (Query/WaitVisible/SendKeys/Click).
// 3. chromedp-cdproto — same library, but hot-path ops issued as cdproto
//    commands, bypassing the node-id machinery. Part 17 of the study.
// ---------------------------------------------------------------------------

type chromeDP struct {
	allocCancel context.CancelFunc
	allocCtx    context.Context
	lowLevel    bool

	// shared makes every job reuse ONE browser websocket instead of dialling its
	// own. It is the control arm of the Track A ablation.
	//
	// It exists because of a confound found while reading chromedp's allocator:
	// NewContext inherits the parent's *Browser (chromedp.go:129), but a context
	// derived straight from a RemoteAllocator has none, so Run allocates one —
	// and RemoteAllocator.Allocate dials the websocket every time
	// (allocate.go:587). The Phase 2 comparison therefore ran chromedp with one
	// CDP connection PER JOB against rod with one connection for all jobs. That
	// is a topology difference, not a library difference, and it has to be
	// controlled before any statement about either library's ceiling is made.
	shared     bool
	sharedCtx  context.Context
	sharedStop context.CancelFunc
	sharedOnce sync.Once
	sharedErr  error
}

func (c *chromeDP) Name() string {
	switch {
	case c.shared:
		return "chromedp-shared-conn"
	case c.lowLevel:
		return "chromedp-cdproto"
	}
	return "chromedp"
}

func (c *chromeDP) Connect(ctx context.Context, wsURL string) error {
	// The remote allocator attaches to a browser this process did not launch,
	// which is exactly the isolation the controller-only measurement needs.
	allocCtx, cancel := chromedp.NewRemoteAllocator(ctx, wsURL)
	c.allocCtx, c.allocCancel = allocCtx, cancel
	return nil
}

// parentCtx returns the context jobs derive their tab from. For the shared arm
// it is a single context whose Browser is allocated exactly once, so every
// later NewContext inherits that one connection.
func (c *chromeDP) parentCtx() (context.Context, error) {
	if !c.shared {
		return c.allocCtx, nil
	}
	c.sharedOnce.Do(func() {
		ctx, cancel := chromedp.NewContext(c.allocCtx)
		c.sharedCtx, c.sharedStop = ctx, cancel
		// Force allocation now: until something runs, Browser is still nil and
		// the first two concurrent jobs would race to dial two connections.
		c.sharedErr = chromedp.Run(ctx)
	})
	return c.sharedCtx, c.sharedErr
}

func (c *chromeDP) RunJob(ctx context.Context, pageURL string, rec *Rec) error {
	parent, err := c.parentCtx()
	if err != nil {
		return err
	}
	var tabCtx context.Context
	var tabCancel context.CancelFunc
	if err := rec.timed("page_create", func() error {
		tabCtx, tabCancel = chromedp.NewContext(parent)
		// A no-op action forces the target to be created now, so the cost
		// lands in page_create instead of leaking into navigate.
		return chromedp.Run(tabCtx)
	}); err != nil {
		return err
	}
	defer tabCancel()

	// chromedp's wait primitives have NO default timeout: they block until the
	// context is cancelled. A job deadline is therefore mandatory, not optional
	// hygiene — measured here by a page that hung every controller forever.
	opCtx, opCancel := context.WithTimeout(tabCtx, JobTimeout)
	defer opCancel()

	if c.lowLevel {
		return c.runLowLevel(opCtx, pageURL, rec)
	}
	return c.runHighLevel(opCtx, pageURL, rec)
}

func (c *chromeDP) runHighLevel(ctx context.Context, pageURL string, rec *Rec) error {
	if err := rec.timed("navigate", func() error {
		return chromedp.Run(ctx, chromedp.Navigate(pageURL))
	}); err != nil {
		return err
	}
	if err := rec.timed("wait_ready", func() error {
		return chromedp.Run(ctx, chromedp.WaitReady("#app-ready", chromedp.ByQuery))
	}); err != nil {
		return err
	}
	var title string
	if err := rec.timed("dom_read", func() error {
		return chromedp.Run(ctx, chromedp.Text("#title", &title, chromedp.ByQuery))
	}); err != nil {
		return err
	}
	if err := rec.timed("fill", func() error {
		return chromedp.Run(ctx, chromedp.SendKeys("#email", "a@b.c", chromedp.ByQuery))
	}); err != nil {
		return err
	}
	if err := rec.timed("click", func() error {
		return chromedp.Run(ctx, chromedp.Click("#submit", chromedp.ByQuery))
	}); err != nil {
		return err
	}
	if err := rec.timed("wait_result", func() error {
		return chromedp.Run(ctx, chromedp.WaitReady(`#result[data-done="1"]`, chromedp.ByQuery))
	}); err != nil {
		return err
	}
	var n int
	return rec.timed("evaluate", func() error {
		return chromedp.Run(ctx, chromedp.Evaluate(`document.querySelectorAll('.item').length`, &n))
	})
}

// runLowLevel issues the same steps as cdproto commands inside a single
// ActionFunc per step: Page.navigate and Runtime.evaluate, no DOM node ids, no
// chromedp query polling.
func (c *chromeDP) runLowLevel(ctx context.Context, pageURL string, rec *Rec) error {
	evalRaw := func(ctx context.Context, expr string) (json.RawMessage, error) {
		var out json.RawMessage
		err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			v, exc, err := runtime.Evaluate(expr).WithReturnByValue(true).WithAwaitPromise(true).Do(ctx)
			if err != nil {
				return err
			}
			if exc != nil {
				return fmt.Errorf("js exception: %s", exc.Text)
			}
			if v != nil {
				out = json.RawMessage(v.Value)
			}
			return nil
		}))
		return out, err
	}
	waitFor := func(ctx context.Context, expr string) error {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			v, err := evalRaw(ctx, expr)
			if err == nil && string(v) == "true" {
				return nil
			}
			time.Sleep(5 * time.Millisecond)
		}
		return fmt.Errorf("timeout: %s", expr)
	}

	if err := rec.timed("navigate", func() error {
		return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, _, _, err := page.Navigate(pageURL).Do(ctx)
			return err
		}))
	}); err != nil {
		return err
	}
	if err := rec.timed("wait_ready", func() error {
		return waitFor(ctx, `!!document.getElementById('app-ready')`)
	}); err != nil {
		return err
	}
	if err := rec.timed("dom_read", func() error {
		_, err := evalRaw(ctx, `document.getElementById('title').textContent`)
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("fill", func() error {
		_, err := evalRaw(ctx, `(()=>{const e=document.getElementById('email');e.value='a@b.c';return true})()`)
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("click", func() error {
		_, err := evalRaw(ctx, `(()=>{document.getElementById('submit').click();return true})()`)
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("wait_result", func() error {
		return waitFor(ctx, `!!document.querySelector('#result[data-done="1"]')`)
	}); err != nil {
		return err
	}
	return rec.timed("evaluate", func() error {
		_, err := evalRaw(ctx, `document.querySelectorAll('.item').length`)
		return err
	})
}

func (c *chromeDP) Close() error {
	if c.allocCancel != nil {
		c.allocCancel()
	}
	return nil
}

// ---------------------------------------------------------------------------
// 4/5. rod — measured twice on purpose.
//
// rod-default keeps the library's defaults, which apply
// devices.LaptopWithMDPIScreen.Landscape() to every new page (browser.go:78 and
// :300): a device-metrics override, a touch-emulation call and a User-Agent
// override to a hardcoded Chrome/114 string.
//
// rod-canon calls NoDefaultDevice(), which is the configuration that is
// comparable to the other controllers. Reporting only one of the two would
// either slander the library or hide a real default.
// ---------------------------------------------------------------------------

type rodCtl struct {
	browser   *rod.Browser
	cancel    context.CancelFunc
	noDefault bool
	fastPoll  bool

	// pool is the experimental arm of the Track A ablation: N independent rod
	// Browsers, each with its own CDP websocket, against the same Chromium.
	//
	// The prediction being tested is directional and therefore refutable — if
	// rod's ceiling is per-connection serialization, throughput must rise with
	// the pool and chromedp must FALL when forced onto one connection. If rod
	// stays flat with N connections, the serialization is somewhere else and
	// this hypothesis is dead.
	pool     []*rod.Browser
	poolSize int
	next     atomic.Int64
}

func (r *rodCtl) Name() string {
	switch {
	case r.poolSize > 0:
		return fmt.Sprintf("rod-multiconn-%d", r.poolSize)
	case r.fastPoll:
		return "rod-fastpoll"
	case r.noDefault:
		return "rod-canon"
	}
	return "rod-default"
}

// pick returns the browser this job should use, round-robin over the pool.
func (r *rodCtl) pick() *rod.Browser {
	if r.poolSize == 0 {
		return r.browser
	}
	i := r.next.Add(1) - 1
	return r.pool[int(i)%len(r.pool)]
}

// fastSleeper replaces rod's default retry policy for the fastpoll variant.
//
// rod's default is BackoffSleeper(100ms, 1s) (utils.go:76), and its own comment
// explains the choice: DOM events or rAF could flood the program if a retry
// never ends. The consequence is a latency floor — a state change that lands in
// 10ms is discovered on the next backoff tick. This variant exists to test
// whether that policy, and not the library, explains rod's latency: same
// library, same browser, same flow, only the sleeper differs.
func fastSleeper() utils.Sleeper {
	return utils.BackoffSleeper(2*time.Millisecond, 20*time.Millisecond, nil)
}

func (r *rodCtl) Connect(ctx context.Context, wsURL string) error {
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	dial := func() (*rod.Browser, error) {
		b := rod.New().ControlURL(wsURL).Context(ctx)
		if r.noDefault {
			b = b.NoDefaultDevice()
		}
		if err := b.Connect(); err != nil {
			return nil, err
		}
		return b, nil
	}
	if r.poolSize > 0 {
		for i := 0; i < r.poolSize; i++ {
			b, err := dial()
			if err != nil {
				return err
			}
			r.pool = append(r.pool, b)
		}
		r.browser = r.pool[0]
		return nil
	}
	b, err := dial()
	if err != nil {
		return err
	}
	r.browser = b
	return nil
}

func (r *rodCtl) RunJob(ctx context.Context, pageURL string, rec *Rec) error {
	var p *rod.Page
	var err error
	br := r.pick()
	if err = rec.timed("page_create", func() error {
		p, err = br.Page(proto.TargetCreateTarget{URL: "about:blank"})
		return err
	}); err != nil {
		return err
	}
	defer func() { _ = p.Close() }()

	// rod's Element() also waits without a default deadline; Timeout applies one
	// to every subsequent call on this page.
	p = p.Timeout(JobTimeout)
	if r.fastPoll {
		p = p.Sleeper(fastSleeper)
	}

	if err := rec.timed("navigate", func() error { return p.Navigate(pageURL) }); err != nil {
		return err
	}
	if err := rec.timed("wait_ready", func() error {
		_, err := p.Element("#app-ready")
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("dom_read", func() error {
		el, err := p.Element("#title")
		if err != nil {
			return err
		}
		_, err = el.Text()
		return err
	}); err != nil {
		return err
	}
	if err := rec.timed("fill", func() error {
		el, err := p.Element("#email")
		if err != nil {
			return err
		}
		return el.Input("a@b.c")
	}); err != nil {
		return err
	}
	if err := rec.timed("click", func() error {
		el, err := p.Element("#submit")
		if err != nil {
			return err
		}
		return el.Click(proto.InputMouseButtonLeft, 1)
	}); err != nil {
		return err
	}
	if err := rec.timed("wait_result", func() error {
		_, err := p.Element(`#result[data-done="1"]`)
		return err
	}); err != nil {
		return err
	}
	return rec.timed("evaluate", func() error {
		_, err := p.Eval(`() => document.querySelectorAll('.item').length`)
		return err
	})
}

// Close detaches WITHOUT calling rod's Browser.Close.
//
// Measured, then confirmed in rod's source (browser.go): Close() issues
// proto.BrowserClose when BrowserContextID is empty, which terminates the whole
// browser — including one this process did not launch. In the shared-browser
// topology every controller here uses, calling it killed Chromium for every
// controller scheduled after rod, in all three repetitions. Rotating the
// controller order is what made that visible; a fixed order with rod last would
// have hidden it entirely.
func (r *rodCtl) Close() error {
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}
