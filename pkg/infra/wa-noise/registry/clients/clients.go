// Package clients guarda, por userID, a Session da aplicação e o
// *wanoise.Client do SDK que ela embrulha.
//
// Os dois mapas vivem juntos porque Register escreve nos DOIS sob o mesmo
// lock. Separá-los exigiria que Register adquirisse dois locks, e o
// invariante que este pacote e seus irmãos mantêm é justamente que NENHUM
// método adquire mais de um lock — sem ordem de aquisição, não há ordem
// errada, hoje ou em qualquer código futuro.
package clients

import (
	"sync"

	wanoise "wa-api/internal/wa-noise"
	port "wa-api/pkg/application/contracts"
)

// Registry é o registro de sessões e clientes do SDK por userID.
type Registry struct {
	mu       sync.RWMutex
	sessions map[string]port.Session
	clients  map[string]*wanoise.Client
}

// New devolve um Registry vazio e pronto para uso.
func New() *Registry {
	return &Registry{
		sessions: make(map[string]port.Session),
		clients:  make(map[string]*wanoise.Client),
	}
}

// Register associa a Session ao userID.
//
// Também publica o *wanoise.Client subjacente: os adapters de domínio (e o
// SessionAttachHook) resolvem o cliente por GetClient, e o orchestrator —
// que só conhece port.Session — não teria como preenchê-lo.
func (r *Registry) Register(userID string, sess port.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[userID] = sess
	if exposer, ok := sess.(interface{ WaNoiseClient() *wanoise.Client }); ok {
		if c := exposer.WaNoiseClient(); c != nil {
			r.clients[userID] = c
		}
	}
}

// Unregister remove o handle de Session associado a userID, se houver.
func (r *Registry) Unregister(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, userID)
}

// Session devolve a Session registrada para userID.
func (r *Registry) Session(userID string) (port.Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sess, ok := r.sessions[userID]
	return sess, ok
}

// SetClient publica o cliente do SDK de userID.
func (r *Registry) SetClient(userID string, c *wanoise.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[userID] = c
}

// GetClient devolve o cliente do SDK de userID, ou nil.
func (r *Registry) GetClient(userID string) *wanoise.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clients[userID]
}

// DeleteClient remove o cliente do SDK de userID.
func (r *Registry) DeleteClient(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, userID)
}

// Snapshot devolve uma cópia do mapa de clientes. É cópia, e não o mapa
// vivo, porque devolver o mapa interno deixaria o chamador lendo sem lock.
func (r *Registry) Snapshot() map[string]*wanoise.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]*wanoise.Client, len(r.clients))
	for k, v := range r.clients {
		out[k] = v
	}
	return out
}

// Count devolve quantos clientes do SDK estão registrados.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

// Iterate percorre os clientes sob RLock, parando quando callback devolve
// false.
//
// O lock fica seguro durante toda a iteração: um callback que chame de volta
// um método de escrita deste Registry trava. É o mesmo contrato de antes da
// quebra em sub-registries — o único chamador em produção
// (SessionCounterAdapter) só lê estado do cliente.
func (r *Registry) Iterate(callback func(*wanoise.Client) bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.clients {
		if !callback(c) {
			break
		}
	}
}
