package main

// A minimal probe answering one question: does THIS Chromium build accept a
// target created inside a non-default BrowserContext?
//
// It exists because chromedp reported "Failed to open new tab - no browser is
// open (-32000)" for that operation, and an error surfaced through a library is
// not evidence about the browser until the same call is made without the
// library.
import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
)

func RunCtxProbe(cdpBase string) error {
	ws, ver, err := browserWS(cdpBase)
	if err != nil {
		return err
	}
	fmt.Printf("browser: %v\n", ver["Browser"])
	c := &cdpMin{}
	if err := c.Connect(context.Background(), ws); err != nil {
		return err
	}
	defer c.Close()

	// disposeOnDetach is the ONLY parameter chromedp adds that this probe did
	// not: WithNewBrowserContext sets it (chromedp.go:502). Testing both forms
	// here is what turns "chromedp fails" into a named difference.
	for _, dod := range []bool{false, true} {
		r0, e0 := c.call("", "Target.createBrowserContext", map[string]any{"disposeOnDetach": dod})
		var rr struct {
			BrowserContextID string `json:"browserContextId"`
		}
		_ = json.Unmarshal(r0, &rr)
		if e0 != nil {
			fmt.Printf("disposeOnDetach=%v createBrowserContext err=%v\n", dod, e0)
			continue
		}
		_, e1 := c.call("", "Target.createTarget", map[string]any{
			"url": "about:blank", "browserContextId": rr.BrowserContextID,
		})
		fmt.Printf("disposeOnDetach=%v -> createTarget err=%v\n", dod, e1)
	}

	res, err := c.call("", "Target.createBrowserContext", map[string]any{})
	if err != nil {
		return fmt.Errorf("createBrowserContext: %w", err)
	}
	var r struct {
		BrowserContextID string `json:"browserContextId"`
	}
	_ = json.Unmarshal(res, &r)
	fmt.Printf("browserContextId=%s\n", r.BrowserContextID)

	for _, params := range []map[string]any{
		{"url": "about:blank", "browserContextId": r.BrowserContextID},
		{"url": "about:blank", "browserContextId": r.BrowserContextID, "newWindow": true},
		{"url": "about:blank", "browserContextId": r.BrowserContextID, "forTab": true},
		// chromedp serializes every optional field, so it sends newWindow:false
		// explicitly where this probe omitted it. If Chromium treats "omitted"
		// and "explicitly false" differently for a context that owns no window
		// yet, that single field is the whole failure.
		{"url": "about:blank", "browserContextId": r.BrowserContextID, "newWindow": false,
			"background": false, "forTab": false, "hidden": false, "focus": false,
			"enableBeginFrameControl": false},
	} {
		res, err := c.call("", "Target.createTarget", params)
		fmt.Printf("createTarget(%v) -> %s err=%v\n", params, string(res), err)
	}
	// Same operation through chromedp, to separate "the browser refuses" from
	// "the library asks for it differently".
	// Route chromedp through the counting proxy so the exact frames it sends can
	// be read off the wire when CDP_TRACE is set.
	if px, e := StartProxy(context.Background(), cdpBase); e == nil {
		defer px.Close()
		ws = px.WS()
	}
	alloc, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), ws)
	defer cancelAlloc()
	for _, tc := range []struct {
		name string
		opts []chromedp.ContextOption
	}{
		{"chromedp default context", nil},
		{"chromedp WithNewBrowserContext", []chromedp.ContextOption{chromedp.WithNewBrowserContext()}},
	} {
		cctx, cancel := chromedp.NewContext(alloc, tc.opts...)
		err := chromedp.Run(cctx, chromedp.Navigate("about:blank"))
		fmt.Printf("%-32s -> err=%v\n", tc.name, err)
		cancel()
	}
	// Repeat the RAW sequence after chromedp has been connected. Ordering is the
	// last uncontrolled variable: in this probe the raw calls ran first and
	// succeeded, while inside the topology harness chromedp connects first and
	// the identical raw calls fail. If the raw sequence fails here too, the
	// trigger is chromedp's presence on the browser, not the frames themselves.
	res2, err2 := c.call("", "Target.createBrowserContext", map[string]any{})
	var r2 struct {
		BrowserContextID string `json:"browserContextId"`
	}
	_ = json.Unmarshal(res2, &r2)
	if err2 != nil {
		fmt.Printf("AFTER chromedp: createBrowserContext err=%v\n", err2)
		return nil
	}
	_, err3 := c.call("", "Target.createTarget", map[string]any{
		"url": "about:blank", "browserContextId": r2.BrowserContextID,
	})
	fmt.Printf("AFTER chromedp: raw createTarget in new context err=%v\n", err3)
	return nil
}

// RunLinkProbe lists the page's anchors with their measured boxes, to settle
// which of several identical hrefs is the one a user could actually click.
func RunLinkProbe(url string) error {
	browsers, err := launchBrowsers(1, CanonicalBrowserProfileV1, 9500)
	if err != nil {
		return err
	}
	defer killAll(browsers)
	alloc, cancel := chromedp.NewRemoteAllocator(context.Background(), browsers[0].WSURL)
	defer cancel()
	ctx, cancel2 := chromedp.NewContext(alloc)
	defer cancel2()
	var raw string
	err = chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("#establishment-code", chromedp.ByQuery),
		chromedp.Evaluate(`JSON.stringify([...document.querySelectorAll('a[href]')].map(a => ({
			href: a.getAttribute('href'),
			w: a.offsetWidth, h: a.offsetHeight,
			rects: a.getClientRects().length,
			text: (a.textContent||'').trim().slice(0,30)
		})))`, &raw))
	if err != nil {
		return err
	}
	fmt.Println(raw)
	return nil
}
