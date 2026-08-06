package whatsmeow

import (
	"sync"

	whatsmeow "wa-api/internal/waclient"

	"github.com/coder/websocket"
	"github.com/go-resty/resty/v2"

	port "wa-api/pkg/application/contracts"
)

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

// Verificação em tempo de compilação de que ClientManager implementa o port.
var _ port.SessionRegistry = (*ClientManager)(nil)
