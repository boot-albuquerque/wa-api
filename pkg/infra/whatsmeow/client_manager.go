package whatsmeow

import (
	"context"
	"crypto/tls"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"

	port "wa-api/pkg/application/contracts"
)

// webhookTLSSkipVerify reports whether outgoing webhook deliveries should
// skip TLS certificate verification. Mirrors pkg/bootstrap/lifecycle.go's
// webhookTLSSkipVerify (same env var, same insecure-opt-in semantics) —
// duplicated here rather than imported because pkg/infra/whatsmeow must not
// depend on pkg/bootstrap (see SessionAttachHook design in the plan).
var webhookTLSSkipVerify = sync.OnceValue(func() bool {
	v := strings.ToLower(os.Getenv("WA_API_WEBHOOK_TLS_SKIP_VERIFY"))
	skip := v == "true" || v == "1"
	if skip {
		log.Warn().
			Str("env", "WA_API_WEBHOOK_TLS_SKIP_VERIFY").
			Msg("INSECURE: webhook TLS certificate verification is DISABLED by explicit configuration. " +
				"Webhook deliveries are vulnerable to man-in-the-middle attacks. Unset this variable in production.")
	}
	return skip
})

// MyClient defines the interface for WhatsApp client wrappers.
type MyClient interface {
	GetWAClient() *whatsmeow.Client
	GetUserID() string
}

type ClientManager struct {
	sync.RWMutex
	whatsmeowClients map[string]*whatsmeow.Client
	httpClients      map[string]*resty.Client
	myClients        map[string]MyClient
	// pollOptions stores the plaintext options sent for each poll, keyed on
	// userID then on the poll's message ID. This lets the event handler
	// SHA-256-match incoming vote hashes back to the original option text
	// before emitting the webhook payload. Entries are best-effort and
	// in-memory only — if wa-api restarts between send and vote, plaintext
	// resolution is skipped and the webhook falls back to hashes only.
	pollOptions map[string]map[string][]string
	// wsConns tracks live /session/ws connections per userID. Set semantics
	// (not a single *Conn) because nothing stops a client from opening more
	// than one WS to the same session — e.g. a reconnect racing the old
	// connection's close.
	wsConns map[string]map[*websocket.Conn]struct{}
	// sessions backs the port.SessionRegistry implementation (Fase 2c):
	// CRUD of Session handles for SessionOrchestrator, kept separate from
	// whatsmeowClients since a Session (port.Session) wraps more than the
	// raw *whatsmeow.Client during the migration.
	sessions map[string]port.Session
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		whatsmeowClients: make(map[string]*whatsmeow.Client),
		httpClients:      make(map[string]*resty.Client),
		myClients:        make(map[string]MyClient),
		pollOptions:      make(map[string]map[string][]string),
		wsConns:          make(map[string]map[*websocket.Conn]struct{}),
		sessions:         make(map[string]port.Session),
	}
}

// Register associa a Session ao userID, satisfazendo port.SessionRegistry.
//
// Também publica o *whatsmeow.Client subjacente em whatsmeowClients: os
// adapters de domínio (e o SessionAttachHook) resolvem o cliente por
// GetWhatsmeowClient, e o orchestrator — que só conhece port.Session — não
// teria como preenchê-lo.
func (cm *ClientManager) Register(userID string, sess port.Session) {
	cm.Lock()
	defer cm.Unlock()
	cm.sessions[userID] = sess
	if exposer, ok := sess.(interface{ WhatsmeowClient() *whatsmeow.Client }); ok {
		if client := exposer.WhatsmeowClient(); client != nil {
			cm.whatsmeowClients[userID] = client
		}
	}
}

// Unregister remove o handle de Session associado a userID, se houver.
func (cm *ClientManager) Unregister(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.sessions, userID)
}

// Get devolve a Session registrada para userID.
func (cm *ClientManager) Get(userID string) (port.Session, bool) {
	cm.RLock()
	defer cm.RUnlock()
	sess, ok := cm.sessions[userID]
	return sess, ok
}

// ProvisionWebhookClient monta o cliente HTTP de entrega de webhook para
// userID, seguindo a mesma configuração hoje montada inline em
// lifecycle.go:234-291 (redirect policy, timeout, TLS, proxy opcional) e
// registra o resultado via SetHTTPClient. proxyURL vazio entrega sem proxy.
func (cm *ClientManager) ProvisionWebhookClient(userID string, proxyURL string) error {
	webhookClient := resty.New()
	webhookClient.SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
	webhookClient.SetTimeout(30 * time.Second)
	webhookClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: webhookTLSSkipVerify()}) //nolint:gosec // opt-in explícito via env var, ver webhookTLSSkipVerify
	webhookClient.OnError(func(req *resty.Request, err error) {
		if v, ok := err.(*resty.ResponseError); ok {
			log.Debug().Str("response", v.Response.String()).Msg("resty error")
			log.Error().Err(v.Err).Msg("resty error")
		}
	})

	if proxyURL != "" {
		webhookClient.SetProxy(proxyURL)
	}

	cm.SetHTTPClient(userID, webhookClient)
	return nil
}

// Verificação em tempo de compilação de que ClientManager implementa o port.
var _ port.SessionRegistry = (*ClientManager)(nil)

func (cm *ClientManager) SetWhatsmeowClient(userID string, client *whatsmeow.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.whatsmeowClients[userID] = client
}

func (cm *ClientManager) GetWhatsmeowClient(userID string) *whatsmeow.Client {
	cm.RLock()
	defer cm.RUnlock()
	return cm.whatsmeowClients[userID]
}

func (cm *ClientManager) DeleteWhatsmeowClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.whatsmeowClients, userID)
}

func (cm *ClientManager) SetHTTPClient(userID string, client *resty.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.httpClients[userID] = client
}

func (cm *ClientManager) GetHTTPClient(userID string) *resty.Client {
	cm.RLock()
	defer cm.RUnlock()
	return cm.httpClients[userID]
}

func (cm *ClientManager) DeleteHTTPClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.httpClients, userID)
}

// SetMyClient stores a MyClient instance.
func (cm *ClientManager) SetMyClient(userID string, client MyClient) {
	cm.Lock()
	defer cm.Unlock()
	cm.myClients[userID] = client
}

// GetMyClient retrieves a MyClient instance.
func (cm *ClientManager) GetMyClient(userID string) MyClient {
	cm.RLock()
	defer cm.RUnlock()
	return cm.myClients[userID]
}

// DeleteMyClient removes a user's MyClient entry and clears any cached
// poll options associated with that user.
func (cm *ClientManager) DeleteMyClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.myClients, userID)
	delete(cm.pollOptions, userID)
}

// SetPollOptions remembers the plaintext options of a poll we just sent so
// that incoming votes (which arrive as SHA-256 hashes of the option text)
// can be resolved back to readable strings.
func (cm *ClientManager) SetPollOptions(userID, msgID string, options []string) {
	cm.Lock()
	defer cm.Unlock()
	if cm.pollOptions[userID] == nil {
		cm.pollOptions[userID] = make(map[string][]string)
	}
	stored := make([]string, len(options))
	copy(stored, options)
	cm.pollOptions[userID][msgID] = stored
}

// GetPollOptions returns the plaintext options associated with a poll
// message, or nil if none were recorded (e.g. wa-api restarted after the
// poll was sent).
func (cm *ClientManager) GetPollOptions(userID, msgID string) []string {
	cm.RLock()
	defer cm.RUnlock()
	if byUser := cm.pollOptions[userID]; byUser != nil {
		return byUser[msgID]
	}
	return nil
}

// GetAllClients returns a snapshot of all whatsmeow clients (read-only copy of keys)
func (cm *ClientManager) GetAllClients() map[string]*whatsmeow.Client {
	cm.RLock()
	defer cm.RUnlock()
	result := make(map[string]*whatsmeow.Client)
	for k, v := range cm.whatsmeowClients {
		result[k] = v
	}
	return result
}

// GetWhatsmeowClientsCount returns the count of whatsmeow clients
func (cm *ClientManager) GetWhatsmeowClientsCount() int {
	cm.RLock()
	defer cm.RUnlock()
	return len(cm.whatsmeowClients)
}

// IterateWhatsmeowClients safely iterates over all whatsmeow clients with a callback
func (cm *ClientManager) IterateWhatsmeowClients(callback func(*whatsmeow.Client) bool) {
	cm.RLock()
	defer cm.RUnlock()
	for _, client := range cm.whatsmeowClients {
		if !callback(client) {
			break
		}
	}
}

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
