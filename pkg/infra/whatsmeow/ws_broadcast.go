package whatsmeow

import (
	"context"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog/log"
)

// AddWSConn registers a live /session/ws connection for userID. Call
// RemoveWSConn (same userID+conn) once the handler's read loop exits, in a
// defer — never let a connection leak beyond its own handler's lifetime.
func (cm *ClientManager) AddWSConn(userID string, conn *websocket.Conn) {
	cm.Lock()
	defer cm.Unlock()
	if cm.wsConns[userID] == nil {
		cm.wsConns[userID] = make(map[*websocket.Conn]struct{})
	}
	cm.wsConns[userID][conn] = struct{}{}
	// Debug, not Info: the handler already logs the connect/disconnect
	// event itself (with req_id) — this is purely the resulting fan-out
	// width, useful when debugging a broadcast that reached fewer clients
	// than expected.
	log.Debug().Str("userID", userID).Int("wsConnCount", len(cm.wsConns[userID])).
		Msg("websocket connection registered")
}

// RemoveWSConn unregisters one connection. Safe to call even if it was
// never added (e.g. Accept() failed before AddWSConn ran).
func (cm *ClientManager) RemoveWSConn(userID string, conn *websocket.Conn) {
	cm.Lock()
	defer cm.Unlock()
	conns := cm.wsConns[userID]
	if conns == nil {
		return
	}
	delete(conns, conn)
	remaining := len(conns)
	if remaining == 0 {
		delete(cm.wsConns, userID)
	}
	log.Debug().Str("userID", userID).Int("wsConnCount", remaining).
		Msg("websocket connection unregistered")
}

// wsBroadcastTimeout bounds how long BroadcastToUser waits on a single slow
// client before giving up on it — a wedged reader on the other end must
// never stall event delivery to every other connection.
const wsBroadcastTimeout = 5 * time.Second

// BroadcastToUser pushes payload (JSON-encoded) to every live WS connection
// for userID. Best-effort: a write failure drops that one connection
// (removed from the registry, closed) without affecting siblings or the
// caller — mirrors the webhook delivery path's fire-and-forget semantics.
// Call via safeGo from the same places that already call
// sendEventWithWebHook, never synchronously from the whatsmeow event loop.
func (cm *ClientManager) BroadcastToUser(userID string, payload interface{}) {
	cm.RLock()
	conns := make([]*websocket.Conn, 0, len(cm.wsConns[userID]))
	for c := range cm.wsConns[userID] {
		conns = append(conns, c)
	}
	cm.RUnlock()

	for _, c := range conns {
		ctx, cancel := context.WithTimeout(context.Background(), wsBroadcastTimeout)
		err := wsjson.Write(ctx, c, payload)
		cancel()
		if err != nil {
			log.Warn().Err(err).Str("userID", userID).
				Msg("websocket broadcast write failed; dropping connection")
			cm.RemoveWSConn(userID, c)
			c.Close(websocket.StatusInternalError, "broadcast write failed")
		}
	}
}
