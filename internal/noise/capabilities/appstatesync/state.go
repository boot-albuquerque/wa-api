package appstatesync

import (
	"encoding/hex"
	"sync"
	"time"
)

// State e' o estado mutavel da sincronizacao de app state. Reune os tres campos
// que viviam soltos em *Client antes desta extracao:
//
//	appStateSyncLock        sync.Mutex     -> syncLock
//	appStateKeyRequests     map[string]time.Time -> keyRequests
//	appStateKeyRequestsLock sync.RWMutex   -> keyRequestsLock
//
// Os dois locks continuam sendo dois locks distintos, e os pontos de aquisicao
// e liberacao sao os mesmos de antes — ver o doc de cada metodo. Reuni-los num
// tipo e' o mesmo movimento que o lote 1 fez com media.ConnCache.
//
// O zero value e' usavel: o mapa e' criado sob o lock de escrita na primeira
// gravacao. Por conter mutexes, State NUNCA pode ser copiado por valor depois
// de usado — sempre passe *State.
type State struct {
	// syncLock serializa fetches de app state entre si.
	syncLock sync.Mutex

	// keyRequestsLock protege keyRequests.
	keyRequestsLock sync.RWMutex
	// keyRequests guarda, por key ID em hex, quando aquela chave foi pedida ao
	// dispositivo primario pela ultima vez.
	keyRequests map[string]time.Time
}

// LockSync/UnlockSync serializam a sincronizacao de app state. Substituem o par
// `cli.appStateSyncLock.Lock()` / `defer cli.appStateSyncLock.Unlock()` que
// abria o corpo de fetchAppState; o lock e' segurado pela busca inteira,
// incluindo as consultas ao servidor, exatamente como antes.
func (s *State) LockSync() { s.syncLock.Lock() }

// UnlockSync libera o lock tomado por LockSync.
func (s *State) UnlockSync() { s.syncLock.Unlock() }

// FilterKeyIDs decide quais chaves ainda podem ser pedidas e registra o pedido,
// tudo sob o lock de escrita.
//
// `rawKeyIDs` e' uma funcao, e nao uma slice, de proposito: no codigo original
// a chamada a `appStateProc.GetMissingKeyIDs` acontecia DENTRO da secao critica
// (entre o Lock e o Unlock). Receber o resultado pronto moveria essa chamada
// para fora e mudaria o comportamento sob concorrencia — duas goroutines
// poderiam calcular a mesma lista de chaves faltantes e as duas passarem no
// filtro. Passando a funcao, o ponto de aquisicao e o de liberacao ficam
// identicos aos de antes.
//
// O envio em si (RequestKeys) fica FORA do lock, tambem como antes.
func (s *State) FilterKeyIDs(now time.Time, rawKeyIDs func() [][]byte) [][]byte {
	s.keyRequestsLock.Lock()
	defer s.keyRequestsLock.Unlock()
	if s.keyRequests == nil {
		s.keyRequests = make(map[string]time.Time)
	}
	raw := rawKeyIDs()
	filtered := make([][]byte, 0, len(raw))
	for _, keyID := range raw {
		stringKeyID := hex.EncodeToString(keyID)
		lastRequestTime := s.keyRequests[stringKeyID]
		if lastRequestTime.IsZero() || lastRequestTime.Add(KeyRequestInterval).Before(now) {
			s.keyRequests[stringKeyID] = now
			filtered = append(filtered, keyID)
		}
	}
	return filtered
}

// ReadKeyRequests executa fn com o lock de LEITURA segurado do inicio ao fim, e
// entrega a fn um predicado que diz se uma chave (em hex) ja' foi pedida.
//
// A forma de callback existe para preservar a duracao da secao critica: o
// chamador (handleAppStateSyncKeyShare, em message_history_sync.go) segurava o
// RLock por todo o laco de chaves, incluindo as gravacoes no store, e nao so'
// pela consulta ao mapa. Um metodo que respondesse uma chave por vez encurtaria
// o hold e mudaria o comportamento sob concorrencia.
func (s *State) ReadKeyRequests(fn func(wasRequested func(hexKeyID string) bool)) {
	s.keyRequestsLock.RLock()
	defer s.keyRequestsLock.RUnlock()
	fn(func(hexKeyID string) bool {
		_, ok := s.keyRequests[hexKeyID]
		return ok
	})
}
