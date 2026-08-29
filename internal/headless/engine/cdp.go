package engine

// A raw CDP connection to the BROWSER endpoint.
//
// It exists because one operation cannot be delegated to the driver library:
// shutting the browser down. Phase 4C section 22 measured both paths side by
// side, and the result went against the prediction:
//
//	raw CDP           Browser.close accepted in 2 ms, 10 -> 0 processes in <1 s
//	chromedp.Cancel   returned nil in 19 ms, every process still alive after 15 s
//
// Had Cancel put the command on the wire, the processes would have died in
// under a second. They did not: with a remote allocator, Cancel takes the dry
// branch (its `first` guard is false) and returns nil with nothing sent. The
// 19 ms was goroutine teardown, and reading latency as proof of delivery was
// the mistake that cost a whole comparison run — the browserclose arm silently
// fell back to SIGTERM and became a replica of the arm it was supposed to be
// compared against.
//
// So the shutdown talks to the browser directly, over a connection this package
// owns, and depends on no library heuristic.
//
// Study origin: scripts/chromium-study/{controllers.go,p3_topology.go,p4c_target.go}.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/coder/websocket"
)

// cdpReadLimit bounds one CDP frame. The default of the websocket library is
// far below what this protocol carries in general (a serialised DOM, a
// screenshot), and a frame over the limit closes the connection — which would
// surface as "the browser went away" during an operation that was fine.
const cdpReadLimit = 64 << 20

type cdpMessage struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    any             `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// browserConn is a control connection to one browser. It is not a driver: it
// sends a command and waits for the matching reply, and ignores events.
type browserConn struct {
	conn    *websocket.Conn
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan cdpMessage
	closed  chan struct{}
	readErr error
}

// dialBrowser opens the control connection. The dial itself is bounded by ctx;
// so is every call made on the result.
func dialBrowser(ctx context.Context, wsURL string) (*browserConn, error) {
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp dial: %w", err)
	}
	conn.SetReadLimit(cdpReadLimit)
	c := &browserConn{
		conn:    conn,
		pending: map[int64]chan cdpMessage{},
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// readLoop runs on the connection's own lifetime, not on a caller's context: a
// call that gives up must not tear down the connection other calls are using.
func (c *browserConn) readLoop() {
	for {
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			c.mu.Lock()
			c.readErr = err
			for id, ch := range c.pending {
				delete(c.pending, id)
				close(ch)
			}
			c.mu.Unlock()
			close(c.closed)
			return
		}
		var m cdpMessage
		if json.Unmarshal(data, &m) != nil || m.ID == 0 {
			continue // an event, not a reply
		}
		c.mu.Lock()
		ch, ok := c.pending[m.ID]
		delete(c.pending, m.ID)
		c.mu.Unlock()
		if ok {
			ch <- m
		}
	}
}

// call sends a command and waits for its reply under ctx.
//
// There is no timeout of its own here on purpose. A fixed internal timeout is a
// deadline the call site cannot see and cannot override, which is the shape of
// defect the DeadlinePolicy exists to remove; the caller passes the budget in.
func (c *browserConn) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.readErr != nil {
		err := c.readErr
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp %s: connection closed: %w", method, err)
	}
	c.nextID++
	id := c.nextID
	ch := make(chan cdpMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	b, err := json.Marshal(cdpMessage{ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	if err := c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		return nil, fmt.Errorf("cdp %s: write: %w", method, err)
	}

	select {
	case m, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("cdp %s: connection closed before reply", method)
		}
		if m.Error != nil {
			return nil, fmt.Errorf("cdp %s: rejected: %s", method, m.Error.Message)
		}
		return m.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *browserConn) Close() {
	_ = c.conn.Close(websocket.StatusNormalClosure, "")
}
