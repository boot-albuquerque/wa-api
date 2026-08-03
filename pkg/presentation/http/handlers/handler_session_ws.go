package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/coder/websocket"
	"github.com/rs/zerolog/hlog"
)

// wsConnRegistry is the subset of *whatsmeow.ClientManager this handler
// needs — kept as an interface so handler tests can supply a fake without
// pulling in the real whatsmeow client machinery.
type wsConnRegistry interface {
	AddWSConn(userID string, conn *websocket.Conn)
	RemoveWSConn(userID string, conn *websocket.Conn)
}

// WSHandler handles GET /session/ws — the real-time counterpart of
// /session/status and /session/qr. It pushes the SAME postmap events
// sendEventWithWebHook already dispatches to per-user webhooks (QR
// code/timeout, Connected/Disconnected/LoggedOut) — see
// pkg/bootstrap/lifecycle_webhook.go, which calls BroadcastToUser as one
// more delivery branch alongside the existing webhook/RabbitMQ fan-out.
//
// This handler never itself decides WHEN to push — it only accepts the
// upgrade, registers the connection so BroadcastToUser can reach it, and
// blocks reading (discarding client frames) until the connection closes.
// REST (/session/status, /session/qr) stays fully intact and unchanged —
// this is a purely additive channel, not a replacement.
type WSHandler struct {
	registry wsConnRegistry
}

func NewWSHandler(registry wsConnRegistry) *WSHandler {
	return &WSHandler{registry: registry}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		// Accept() already wrote the HTTP error response on failure.
		hlog.FromRequest(r).Warn().Err(err).Str("handler", "WS").Str("user_id", id).
			Msg("websocket upgrade failed")
		return
	}
	// 1MB is generous for a JSON status/QR postmap (largest field is the
	// base64 QR PNG, already bounded by skip2/go-qrcode's fixed 256px size).
	conn.SetReadLimit(1 << 20)

	h.registry.AddWSConn(id, conn)
	defer h.registry.RemoveWSConn(id, conn)

	hlog.FromRequest(r).Info().Str("handler", "WS").Str("user_id", id).Msg("websocket connected")

	// No inbound protocol today — this loop exists purely to detect the
	// client going away (Read returns an error on close/network drop) and
	// to answer control-frame pings, both handled internally by
	// (*websocket.Conn).Read. Discarding whatever a client sends is
	// deliberate: this is a push-only channel.
	ctx := r.Context()
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			closeStatus := websocket.CloseStatus(err)
			logEvt := hlog.FromRequest(r).Info()
			if closeStatus == -1 && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
				logEvt = hlog.FromRequest(r).Warn()
			}
			logEvt.Err(err).Str("handler", "WS").Str("user_id", id).Msg("websocket disconnected")
			return
		}
	}
}
