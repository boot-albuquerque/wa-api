package whatsmeow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// dialTestWS sobe um servidor WS de teste e devolve a conexão do lado
// cliente (a que o ClientManager registraria) mais uma função que lê uma
// mensagem do lado servidor.
func dialTestWS(t *testing.T) (*websocket.Conn, func() (map[string]string, error)) {
	t.Helper()
	received := make(chan map[string]string, 1)
	readErr := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			readErr <- err
			return
		}
		defer func() { _ = c.Close(websocket.StatusNormalClosure, "") }()
		var payload map[string]string
		if err := wsjson.Read(r.Context(), c, &payload); err != nil {
			readErr <- err
			return
		}
		received <- payload
	}))
	t.Cleanup(srv.Close)

	conn, _, err := websocket.Dial(context.Background(), "ws"+srv.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })

	return conn, func() (map[string]string, error) {
		select {
		case p := <-received:
			return p, nil
		case err := <-readErr:
			return nil, err
		}
	}
}

// TestClientManager_WSConnLifecycle: Add → Remove, incluindo o ramo de
// remoção do último conn (que apaga a entrada do usuário).
func TestClientManager_WSConnLifecycle(t *testing.T) {
	cm := NewClientManager()
	conn, _ := dialTestWS(t)

	cm.AddWSConn("u1", conn)
	cm.RLock()
	n := len(cm.wsConns["u1"])
	cm.RUnlock()
	if n != 1 {
		t.Fatalf("wsConns after AddWSConn = %d, want 1", n)
	}

	cm.RemoveWSConn("u1", conn)
	cm.RLock()
	_, still := cm.wsConns["u1"]
	cm.RUnlock()
	if still {
		t.Error("wsConns entry survived removal of last connection")
	}
}

// TestClientManager_RemoveWSConn_Unknown é no-op e não pode entrar em pânico.
func TestClientManager_RemoveWSConn_Unknown(t *testing.T) {
	cm := NewClientManager()
	conn, _ := dialTestWS(t)
	cm.RemoveWSConn("u-inexistente", conn)
}

// TestClientManager_BroadcastToUser entrega o payload à conexão registrada.
func TestClientManager_BroadcastToUser(t *testing.T) {
	cm := NewClientManager()
	conn, readOne := dialTestWS(t)
	cm.AddWSConn("u1", conn)

	cm.BroadcastToUser("u1", map[string]string{"event": "ping"})

	got, err := readOne()
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	if got["event"] != "ping" {
		t.Errorf("payload = %v, want event=ping", got)
	}
}

// TestClientManager_BroadcastToUser_NoConns é no-op silencioso.
func TestClientManager_BroadcastToUser_NoConns(t *testing.T) {
	cm := NewClientManager()
	cm.BroadcastToUser("u1", map[string]string{"event": "ping"})
}

// TestClientManager_BroadcastToUser_DropsDeadConn: escrever numa conexão já
// fechada derruba a conexão do registro.
func TestClientManager_BroadcastToUser_DropsDeadConn(t *testing.T) {
	cm := NewClientManager()
	conn, _ := dialTestWS(t)
	cm.AddWSConn("u1", conn)
	_ = conn.Close(websocket.StatusNormalClosure, "")

	cm.BroadcastToUser("u1", map[string]string{"event": "ping"})

	cm.RLock()
	_, still := cm.wsConns["u1"]
	cm.RUnlock()
	if still {
		t.Error("dead connection was not dropped from the registry")
	}
}
