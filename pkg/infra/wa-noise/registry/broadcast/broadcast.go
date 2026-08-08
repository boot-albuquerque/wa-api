// Package broadcast guarda as conexões WebSocket vivas de /session/ws por
// userID e faz o fan-out de eventos para elas.
//
// Único mapa, nenhum acesso cruzado a outro sub-registry: como
// registry/webhook, saiu inteiro de ClientManager sem ressalva de
// atomicidade.
//
// É também onde a quebra mais rende: antes, Broadcast segurava o RLock
// global do ClientManager para copiar as conexões, bloqueando escritas em
// sessions, clientes do SDK e clientes HTTP de TODOS os usuários enquanto
// isso. Agora o lock que ele toma só cobre este mapa.
package broadcast

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog/log"
)

// writeTimeout limita quanto Broadcast espera por um cliente WS lento antes
// de desistir dele — um leitor travado do outro lado não pode atrasar a
// entrega para todas as outras conexões.
const writeTimeout = 5 * time.Second

// Registry é o registro de conexões WebSocket vivas por userID.
type Registry struct {
	mu sync.RWMutex
	// conns tem semântica de conjunto (e não um único *Conn) porque nada
	// impede um cliente de abrir mais de um WS para a mesma sessão — por
	// exemplo, uma reconexão correndo com o fechamento da conexão antiga.
	conns map[string]map[*websocket.Conn]struct{}
}

// New devolve um Registry vazio e pronto para uso.
func New() *Registry {
	return &Registry{conns: make(map[string]map[*websocket.Conn]struct{})}
}

// Add registra uma conexão /session/ws viva para userID. Chame Remove
// (mesmo userID e conn) assim que o read loop do handler sair, num defer —
// nunca deixe uma conexão vazar além do tempo de vida do próprio handler.
func (r *Registry) Add(userID string, conn *websocket.Conn) {
	r.mu.Lock()
	if r.conns[userID] == nil {
		r.conns[userID] = make(map[*websocket.Conn]struct{})
	}
	r.conns[userID][conn] = struct{}{}
	count := len(r.conns[userID])
	r.mu.Unlock()

	// Debug, e não Info: o handler já loga o evento de connect/disconnect
	// (com req_id) — isto aqui é puramente a largura do fan-out resultante,
	// útil quando se depura um broadcast que alcançou menos clientes que o
	// esperado.
	log.Debug().Str("userID", userID).Int("wsConnCount", count).
		Msg("websocket connection registered")
}

// Remove desregistra uma conexão. Seguro chamar mesmo que ela nunca tenha
// sido adicionada (por exemplo, se Accept() falhou antes de Add rodar).
func (r *Registry) Remove(userID string, conn *websocket.Conn) {
	r.mu.Lock()
	conns := r.conns[userID]
	if conns == nil {
		r.mu.Unlock()
		return
	}
	delete(conns, conn)
	remaining := len(conns)
	if remaining == 0 {
		delete(r.conns, userID)
	}
	r.mu.Unlock()

	log.Debug().Str("userID", userID).Int("wsConnCount", remaining).
		Msg("websocket connection unregistered")
}

// Broadcast empurra payload (codificado em JSON) para toda conexão WS viva
// de userID. Best-effort: uma falha de escrita derruba aquela conexão
// (removida do registro e fechada) sem afetar as irmãs nem o chamador —
// espelha a semântica fire-and-forget do caminho de entrega de webhook.
// Chame via safego dos mesmos pontos que já chamam sendEventWithWebHook,
// nunca de forma síncrona a partir do event loop do SDK.
func (r *Registry) Broadcast(userID string, payload interface{}) {
	r.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(r.conns[userID]))
	for c := range r.conns[userID] {
		conns = append(conns, c)
	}
	r.mu.RUnlock()

	// A escrita acontece fora do lock, e é por isso que o snapshot acima
	// existe: wsjson.Write pode bloquear até writeTimeout por conexão, e
	// segurar o lock durante isso serializaria todo Add/Remove atrás do
	// cliente mais lento.
	//
	// E acontece em PARALELO, uma goroutine por conexão (F74). O laço era
	// serial, e como cada conexão tem seu próprio teto de writeTimeout, N
	// clientes lentos custavam N × writeTimeout ao broadcast inteiro —
	// observado em produção com 6 conexões obsoletas do mesmo usuário, uma
	// delas estourando o deadline. Agora o teto é writeTimeout no total.
	//
	// payload é lido, nunca escrito: json.Marshal concorrente sobre o mesmo
	// valor é seguro. Verificado que o produtor (sendEventWithWebHook) não
	// muta o mapa depois de despachar.
	var wg sync.WaitGroup
	wg.Add(len(conns))
	for _, c := range conns {
		go func(c *websocket.Conn) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
			defer cancel()
			if err := wsjson.Write(ctx, c, payload); err != nil {
				log.Warn().Err(err).Str("userID", userID).
					Msg("websocket broadcast write failed; dropping connection")
				r.Remove(userID, c)
				c.Close(websocket.StatusInternalError, "broadcast write failed")
			}
		}(c)
	}
	// Espera todas: Broadcast continua significando "tentou entregar a todo
	// mundo e terminou". Não esperar mudaria o contrato e deixaria goroutines
	// escrevendo depois de a função retornar.
	wg.Wait()
}
