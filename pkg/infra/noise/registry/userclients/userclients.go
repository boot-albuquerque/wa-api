// Package userclients guarda, por userID, o wrapper UserClient e o cache de
// opções de enquete.
//
// Os dois mapas vivem juntos pelo mesmo motivo que sessions e clients vivem
// juntos em registry/clients: Delete apaga os DOIS sob o mesmo lock, e o
// invariante da família é que nenhum método adquira mais de um lock.
//
// O agrupamento vem do acoplamento medido no código, não da semântica dos
// nomes — "UserClient" e "opções de enquete" não parecem pertencer ao mesmo
// pacote, mas o ciclo de vida do cache de enquete é o do cliente do usuário,
// e é isso que o Delete comum expressa.
package userclients

import (
	"sync"

	"wa-api/internal/noise"
)

// UserClient é o wrapper de cliente WhatsApp mantido por userID.
type UserClient interface {
	GetWAClient() *noise.Client
	GetUserID() string
}

// Registry é o registro de UserClient e de opções de enquete por userID.
type Registry struct {
	mu      sync.RWMutex
	clients map[string]UserClient
	// polls guarda o texto em claro das opções enviadas em cada enquete,
	// chaveado por userID e depois pelo ID da mensagem. É isso que permite
	// ao event handler casar o hash SHA-256 de um voto recebido de volta com
	// o texto original antes de emitir o webhook. As entradas são
	// best-effort e só em memória — se o wa-api reiniciar entre o envio e o
	// voto, a resolução é pulada e o webhook sai só com os hashes.
	polls map[string]map[string][]string
}

// New devolve um Registry vazio e pronto para uso.
func New() *Registry {
	return &Registry{
		clients: make(map[string]UserClient),
		polls:   make(map[string]map[string][]string),
	}
}

// Set guarda o UserClient de userID.
func (r *Registry) Set(userID string, c UserClient) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[userID] = c
}

// Get devolve o UserClient de userID, ou nil.
func (r *Registry) Get(userID string) UserClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clients[userID]
}

// Delete remove o UserClient de userID e descarta junto o cache de enquetes
// dele — o cache não sobrevive ao cliente que o originou.
func (r *Registry) Delete(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, userID)
	delete(r.polls, userID)
}

// SetPollOptions memoriza o texto em claro das opções de uma enquete recém
// enviada, para que os votos — que chegam como hashes SHA-256 do texto —
// possam ser resolvidos de volta.
//
// Copia o slice do chamador de propósito: guardar o slice recebido deixaria
// o chamador dono do array por baixo, e uma escrita dele seria corrida com
// qualquer leitor daqui — fora do lock, portanto invisível para o -race dos
// testes deste pacote.
func (r *Registry) SetPollOptions(userID, msgID string, options []string) {
	stored := make([]string, len(options))
	copy(stored, options)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.polls[userID] == nil {
		r.polls[userID] = make(map[string][]string)
	}
	r.polls[userID][msgID] = stored
}

// GetPollOptions devolve as opções em claro associadas a uma mensagem de
// enquete, ou nil se nenhuma foi registrada (por exemplo, o wa-api
// reiniciou depois de a enquete ter sido enviada).
func (r *Registry) GetPollOptions(userID, msgID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if byUser := r.polls[userID]; byUser != nil {
		return byUser[msgID]
	}
	return nil
}
