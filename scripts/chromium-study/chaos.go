package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// Chaos crashes renderers underneath a running controller, from an INDEPENDENT
// CDP connection.
//
// Doing it from outside the controller is what makes the result meaningful: the
// controller is not told, exactly as in production, where a renderer dies from
// an OOM kill or a browser bug. What the study measures is how each library
// discovers that, how long it takes, and whether the process it leaves behind is
// still usable.
type Chaos struct {
	conn    *cdpMin
	every   time.Duration
	crashes atomic.Int64
	stop    chan struct{}
	stopped chan struct{}
}

// StartChaos connects a separate CDP client and crashes one page target every
// interval. It never crashes the browser process itself: that failure mode is
// covered separately by killing the process from the shell, because a browser
// death is not something any controller can recover from without relaunching.
func StartChaos(ctx context.Context, wsURL string, every time.Duration) (*Chaos, error) {
	c := &cdpMin{}
	if err := c.Connect(ctx, wsURL); err != nil {
		return nil, err
	}
	ch := &Chaos{conn: c, every: every, stop: make(chan struct{}), stopped: make(chan struct{})}
	go ch.loop()
	return ch, nil
}

func (ch *Chaos) loop() {
	defer close(ch.stopped)
	t := time.NewTicker(ch.every)
	defer t.Stop()
	for {
		select {
		case <-ch.stop:
			return
		case <-t.C:
			if err := ch.crashOnePage(); err != nil {
				fmt.Fprintf(os.Stderr, "   chaos: %v\n", err)
			}
		}
	}
}

// crashOnePage picks a live page target and crashes its renderer via
// Page.crash, the CDP method that exists for exactly this purpose.
func (ch *Chaos) crashOnePage() error {
	res, err := ch.conn.call("", "Target.getTargets", map[string]any{})
	if err != nil {
		return err
	}
	var r struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			URL      string `json:"url"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return err
	}
	for _, t := range r.TargetInfos {
		// Only pages carrying the workload: crashing about:blank or a
		// browser_ui target would prove nothing about the controllers.
		if t.Type != "page" || len(t.URL) < 5 || t.URL == "about:blank" {
			continue
		}
		sess, err := ch.conn.call("", "Target.attachToTarget",
			map[string]any{"targetId": t.TargetID, "flatten": true})
		if err != nil {
			continue
		}
		var a struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(sess, &a) != nil {
			continue
		}
		// Page.crash never replies: the renderer is gone before it can. The
		// error from the timeout is the expected outcome, not a failure.
		go func() { _, _ = ch.conn.call(a.SessionID, "Page.crash", map[string]any{}) }()
		ch.crashes.Add(1)
		return nil
	}
	return fmt.Errorf("no workload page target to crash")
}

// Stop ends the chaos loop and reports how many renderers were killed.
func (ch *Chaos) Stop() int64 {
	close(ch.stop)
	<-ch.stopped
	_ = ch.conn.Close()
	return ch.crashes.Load()
}
