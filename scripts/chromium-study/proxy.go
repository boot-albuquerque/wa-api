package main

import (
	"bytes"
	"context"
	"os"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// CountingProxy sits between a controller and the browser and counts CDP
// messages in each direction.
//
// It exists because round-trip count is the quantity that actually explains the
// cost difference between abstraction levels, and no controller exposes it.
// Counting at the wire is the only method that is identical for chromedp, rod
// and hand-written CDP — instrumenting each library would measure three
// different things.
type CountingProxy struct {
	ln         net.Listener
	upstreamWS string
	srv        *http.Server

	sent     atomic.Int64 // controller -> browser (commands)
	received atomic.Int64 // browser -> controller (replies + events)
	bytesOut atomic.Int64
	bytesIn  atomic.Int64
}

// StartProxy listens on loopback and forwards to the real browser endpoint.
// httpBase is like http://127.0.0.1:9222.
func StartProxy(ctx context.Context, httpBase string) (*CountingProxy, error) {
	upstream, _, err := browserWS(httpBase)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &CountingProxy{ln: ln, upstreamWS: upstream}

	mux := http.NewServeMux()
	// Controllers discover the browser through /json/version. The proxy must
	// answer it with its OWN websocket URL, or they would connect straight to
	// the browser and bypass the counter entirely.
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Browser":              "proxied",
			"Protocol-Version":     "1.3",
			"webSocketDebuggerUrl": "ws://" + p.ln.Addr().String() + "/devtools/browser/proxy",
		})
	})
	mux.HandleFunc("/", p.relay)

	p.srv = &http.Server{Handler: mux}
	go func() { _ = p.srv.Serve(ln) }()
	return p, nil
}

func (p *CountingProxy) URL() string { return "http://" + p.ln.Addr().String() }
func (p *CountingProxy) WS() string {
	return "ws://" + p.ln.Addr().String() + "/devtools/browser/proxy"
}
func (p *CountingProxy) Sent() int64 { return p.sent.Load() }
func (p *CountingProxy) Recv() int64 { return p.received.Load() }
func (p *CountingProxy) Reset() {
	p.sent.Store(0)
	p.received.Store(0)
	p.bytesOut.Store(0)
	p.bytesIn.Store(0)
}
func (p *CountingProxy) BytesOut() int64 { return p.bytesOut.Load() }
func (p *CountingProxy) BytesIn() int64  { return p.bytesIn.Load() }

func (p *CountingProxy) relay(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket") {
		http.NotFound(w, r)
		return
	}
	client, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	client.SetReadLimit(256 << 20)
	ctx := context.Background()

	upstream, _, err := websocket.Dial(ctx, p.upstreamWS, nil)
	if err != nil {
		_ = client.Close(websocket.StatusInternalError, "upstream")
		return
	}
	upstream.SetReadLimit(256 << 20)

	done := make(chan struct{}, 2)
	go func() {
		for {
			typ, data, err := client.Read(ctx)
			if err != nil {
				done <- struct{}{}
				return
			}
			p.sent.Add(1)
			p.bytesOut.Add(int64(len(data)))
			if os.Getenv("CDP_TRACE") != "" && bytes.Contains(data, []byte("Target.create")) {
				fmt.Fprintf(os.Stderr, "-> %s\n", data)
			}
			if upstream.Write(ctx, typ, data) != nil {
				done <- struct{}{}
				return
			}
		}
	}()
	go func() {
		for {
			typ, data, err := upstream.Read(ctx)
			if err != nil {
				done <- struct{}{}
				return
			}
			p.received.Add(1)
			p.bytesIn.Add(int64(len(data)))
			if os.Getenv("CDP_TRACE") != "" && (bytes.Contains(data, []byte("browserContextId")) || bytes.Contains(data, []byte("error"))) {
				fmt.Fprintf(os.Stderr, "<- %s\n", data)
			}
			if client.Write(ctx, typ, data) != nil {
				done <- struct{}{}
				return
			}
		}
	}()
	<-done
	_ = client.Close(websocket.StatusNormalClosure, "")
	_ = upstream.Close(websocket.StatusNormalClosure, "")
}

// Close stops the proxy.
func (p *CountingProxy) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return p.srv.Shutdown(ctx)
}

var _ = fmt.Sprintf
