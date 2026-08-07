// Package registry mantém as instâncias e o estado compartilhado por
// userID da camada de integração.
//
// # Por que existem sub-registries
//
// Até a quebra, ClientManager era um struct com um sync.RWMutex EMBUTIDO
// guardando cinco mapas de propósitos distintos. Duas consequências:
//
//  1. Contenção sem relação de causa: o fan-out WebSocket segurava o mesmo
//     lock que o registro de sessões, então um cliente WS lento atrasava
//     um Register de outro usuário.
//  2. O mutex embutido era API PÚBLICA — qualquer chamador podia congelar
//     o registro inteiro com cm.Lock().
//
// Cada mapa passou a viver no seu próprio pacote, com o seu próprio mutex
// não exportado. ClientManager virou a composição dos quatro e delega.
//
// # O invariante que sustenta a divisão
//
// NENHUM método adquire mais de um lock. Não há ordem de aquisição, logo
// não há ordem errada — nem hoje nem em código futuro.
//
// É por isso que o corte não seguiu os nomes. Os pares (sessions,
// clientes do SDK) e (UserClient, opções de enquete) ficaram juntos porque
// Register e Delete escrevem nos dois membros de cada par sob o mesmo
// lock; separá-los por afinidade de nome obrigaria esses métodos a tomar
// dois locks e destruiria o invariante.
//
// # O que a quebra NÃO mudou
//
// Nada observável. Em particular, ela não pode ter quebrado atomicidade
// entre mapas porque essa atomicidade nunca foi observável: nenhum método
// público lê dois mapas, então qualquer leitor já precisava de duas
// chamadas com dois locks e já podia intercalar. Register e Delete apenas
// ESCREVEM em dois mapas.
//
// O baseline dessa afirmação está em concurrency_test.go, escrito contra o
// ClientManager monolítico antes da quebra e mantido depois dela.
package registry

import (
	wanoise "wa-api/internal/wa-noise"
	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/infra/wa-noise/registry/broadcast"
	"wa-api/pkg/infra/wa-noise/registry/clients"
	"wa-api/pkg/infra/wa-noise/registry/userclients"
	"wa-api/pkg/infra/wa-noise/registry/webhook"

	"github.com/coder/websocket"
	"github.com/go-resty/resty/v2"
)

// UserClient é o wrapper de cliente WhatsApp mantido por userID.
//
// Alias, e não uma segunda declaração: o tipo passou a viver em
// registry/userclients, e um alias mantém registry.UserClient válido para quem
// já o referenciava sem criar dois tipos incompatíveis.
type UserClient = userclients.UserClient

// ClientManager é a fachada sobre os quatro sub-registries.
//
// Os campos são ponteiros para os Registry de cada sub-pacote, e não mapas:
// o estado e o lock que o protege viajam juntos, dentro do pacote que
// entende aquele estado.
type ClientManager struct {
	clients     *clients.Registry
	userClients *userclients.Registry
	webhooks    *webhook.Registry
	wsConns     *broadcast.Registry
}

// NewClientManager devolve um ClientManager com os quatro sub-registries
// prontos.
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients:     clients.New(),
		userClients: userclients.New(),
		webhooks:    webhook.New(),
		wsConns:     broadcast.New(),
	}
}

// -- port.SessionRegistry ---------------------------------------------------

// Register associa a Session ao userID, satisfazendo port.SessionRegistry.
func (cm *ClientManager) Register(userID string, sess port.Session) {
	cm.clients.Register(userID, sess)
}

// Unregister remove o handle de Session associado a userID, se houver.
func (cm *ClientManager) Unregister(userID string) {
	cm.clients.Unregister(userID)
}

// Get devolve a Session registrada para userID.
func (cm *ClientManager) Get(userID string) (port.Session, bool) {
	return cm.clients.Session(userID)
}

// Verificação em tempo de compilação de que ClientManager implementa o port.
var _ port.SessionRegistry = (*ClientManager)(nil)

// -- clientes do SDK --------------------------------------------------------

// SetWaNoiseClient publica o cliente do SDK de userID.
func (cm *ClientManager) SetWaNoiseClient(userID string, client *wanoise.Client) {
	cm.clients.SetClient(userID, client)
}

// GetWaNoiseClient devolve o cliente do SDK de userID, ou nil.
func (cm *ClientManager) GetWaNoiseClient(userID string) *wanoise.Client {
	return cm.clients.GetClient(userID)
}

// DeleteWaNoiseClient remove o cliente do SDK de userID.
func (cm *ClientManager) DeleteWaNoiseClient(userID string) {
	cm.clients.DeleteClient(userID)
}

// GetAllClients devolve uma cópia do mapa de clientes do SDK.
func (cm *ClientManager) GetAllClients() map[string]*wanoise.Client {
	return cm.clients.Snapshot()
}

// GetWaNoiseClientsCount devolve quantos clientes do SDK estão registrados.
func (cm *ClientManager) GetWaNoiseClientsCount() int {
	return cm.clients.Count()
}

// IterateWaNoiseClients percorre os clientes do SDK sob lock de leitura,
// parando quando callback devolve false.
func (cm *ClientManager) IterateWaNoiseClients(callback func(*wanoise.Client) bool) {
	cm.clients.Iterate(callback)
}

// -- UserClient e enquetes ----------------------------------------------------

// SetUserClient guarda o UserClient de userID.
func (cm *ClientManager) SetUserClient(userID string, client UserClient) {
	cm.userClients.Set(userID, client)
}

// GetUserClient devolve o UserClient de userID, ou nil.
func (cm *ClientManager) GetUserClient(userID string) UserClient {
	return cm.userClients.Get(userID)
}

// DeleteUserClient remove o UserClient de userID e descarta junto o cache de
// enquetes dele.
func (cm *ClientManager) DeleteUserClient(userID string) {
	cm.userClients.Delete(userID)
}

// SetPollOptions memoriza o texto em claro das opções de uma enquete.
func (cm *ClientManager) SetPollOptions(userID, msgID string, options []string) {
	cm.userClients.SetPollOptions(userID, msgID, options)
}

// GetPollOptions devolve as opções em claro de uma enquete, ou nil.
func (cm *ClientManager) GetPollOptions(userID, msgID string) []string {
	return cm.userClients.GetPollOptions(userID, msgID)
}

// -- clientes HTTP de webhook -----------------------------------------------

// ProvisionWebhookClient monta e registra o cliente HTTP de entrega de
// webhook para userID. proxyURL vazio entrega sem proxy.
func (cm *ClientManager) ProvisionWebhookClient(userID string, proxyURL string) error {
	return cm.webhooks.Provision(userID, proxyURL)
}

// SetHTTPClient guarda o cliente HTTP de userID.
func (cm *ClientManager) SetHTTPClient(userID string, client *resty.Client) {
	cm.webhooks.Set(userID, client)
}

// GetHTTPClient devolve o cliente HTTP de userID, ou nil.
func (cm *ClientManager) GetHTTPClient(userID string) *resty.Client {
	return cm.webhooks.Get(userID)
}

// DeleteHTTPClient remove o cliente HTTP de userID.
func (cm *ClientManager) DeleteHTTPClient(userID string) {
	cm.webhooks.Delete(userID)
}

// -- fan-out WebSocket ------------------------------------------------------

// AddWSConn registra uma conexão /session/ws viva para userID.
func (cm *ClientManager) AddWSConn(userID string, conn *websocket.Conn) {
	cm.wsConns.Add(userID, conn)
}

// RemoveWSConn desregistra uma conexão de userID.
func (cm *ClientManager) RemoveWSConn(userID string, conn *websocket.Conn) {
	cm.wsConns.Remove(userID, conn)
}

// BroadcastToUser empurra payload para toda conexão WS viva de userID.
func (cm *ClientManager) BroadcastToUser(userID string, payload interface{}) {
	cm.wsConns.Broadcast(userID, payload)
}
