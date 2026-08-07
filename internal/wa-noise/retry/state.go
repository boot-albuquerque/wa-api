// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package retry

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"wa-api/internal/wa-noise/types"
)

// IncomingKey identifica um pedido de retry recebido: quem pediu e de qual
// mensagem. As duas partes vem do servidor — ver F36.
type IncomingKey struct {
	JID       types.JID
	MessageID types.MessageID
}

// RecentKey identifica uma mensagem no buffer circular de enviadas.
type RecentKey struct {
	To types.JID
	ID types.MessageID
}

// State e' o estado mutavel do dominio de retry. Reune os campos que viviam
// soltos em *Client antes desta extracao:
//
//	sessionRecreateHistory / sessionRecreateHistoryLock
//	incomingRetryRequestCounter / incomingRetryRequestCounterLock
//	messageRetries / messageRetriesLock / retrySema
//	recentMessagesMap / recentMessagesList / recentMessagesPtr / recentMessagesLock
//	lastRetryStoreClear
//	pendingPhoneRerequests / pendingPhoneRerequestsLock
//
// Sao CINCO locks distintos, e continuam sendo cinco, com os mesmos pontos de
// aquisicao e liberacao de antes. Cada metodo abaixo reproduz uma secao critica
// que ja' existia, inteira: nenhum statement passou de dentro para fora de um
// lock nem o contrario.
//
// O zero value e' usavel; os mapas sao criados preguicosamente sob o lock de
// escrita (mesma mudanca, e mesmo racional, do lote 3 e do tctoken do lote 4:
// uma leitura antes de qualquer gravacao enxerga mapa nil, o que em Go e'
// leitura valida e devolve o zero — mesmo resultado que um mapa vazio).
//
// Por conter mutexes, State NUNCA pode ser copiado por valor depois de usado —
// sempre passe *State.
//
// AVISO DE DIVIDA HERDADA (F36 em HOUSEKEEP.md): incomingCounter e
// messageRetries crescem sem limite, chaveados por dado do servidor, e nao sao
// limpos em lugar nenhum — nem no Disconnect. A extracao MOVEU o problema para
// ca'; NAO o resolveu. Corrigi-lo exige escolher uma politica de despejo, que e'
// mudanca de comportamento observavel, e por isso segue pendente de decisao.
// Contraste deliberado: o buffer de recentes logo abaixo e' circular de
// RecentMessagesSize justamente para nao ter esse problema.
type State struct {
	// --- recriacao de sessao Signal ---
	sessionRecreateHistory map[types.JID]time.Time
	sessionRecreateLock    sync.Mutex

	// --- contador de retries recebidos (F36) ---
	incomingCounter     map[IncomingKey]int
	incomingCounterLock sync.Mutex

	// --- contador de recibos de retry enviados (F36) ---
	messageRetries     map[string]int
	messageRetriesLock sync.Mutex
	// sema limita quantos recibos de retry sao tratados em paralelo. nil =
	// ilimitado, que e' o padrao.
	sema *semaphore.Weighted

	// --- buffer circular de mensagens enviadas ---
	recentMap  map[RecentKey]RecentMessage
	recentList [RecentMessagesSize]RecentKey
	recentPtr  int
	recentLock sync.RWMutex

	// lastStoreClear e' o carimbo do ultimo expurgo do store de retry.
	//
	// NUNCA e' escrito — nem antes nem depois da extracao. Ver F52 em
	// HOUSEKEEP.md: o throttle de StoreClearInterval que o le' e' portanto
	// codigo morto, e DeleteOldOutgoingEvents roda em toda gravacao. Nao ganhou
	// lock proprio porque a leitura original tambem nao tinha nenhum.
	lastStoreClear time.Time

	// --- pedidos de reenvio pendentes ao telefone ---
	pendingPhone     map[types.MessageID]context.CancelFunc
	pendingPhoneLock sync.RWMutex
}

// --- recriacao de sessao Signal ---

// LockSessionRecreate/UnlockSessionRecreate substituem o par
// `cli.sessionRecreateHistoryLock.Lock()` / `defer ...Unlock()` que abria o
// corpo de shouldRecreateSession. O lock e' segurado pelo corpo inteiro,
// incluindo a consulta ContainsSession ao store, exatamente como antes.
func (s *State) LockSessionRecreate() { s.sessionRecreateLock.Lock() }

// UnlockSessionRecreate libera o lock tomado por LockSessionRecreate.
func (s *State) UnlockSessionRecreate() { s.sessionRecreateLock.Unlock() }

// LastSessionRecreate devolve quando a sessao com jid foi recriada pela ultima
// vez, e se ha registro.
//
// So' pode ser chamado com o lock de LockSessionRecreate segurado. Nao toma
// lock por conta propria de proposito: no codigo original leitura e escrita
// viviam na mesma secao critica, e um lock proprio criaria um segundo ponto de
// sincronizacao que nao existia.
func (s *State) LastSessionRecreate(jid types.JID) (time.Time, bool) {
	t, ok := s.sessionRecreateHistory[jid]
	return t, ok
}

// MarkSessionRecreated registra que a sessao com jid foi recriada agora.
//
// Mesma condicao de LastSessionRecreate: exige o lock segurado.
func (s *State) MarkSessionRecreated(jid types.JID, t time.Time) {
	if s.sessionRecreateHistory == nil {
		s.sessionRecreateHistory = make(map[types.JID]time.Time)
	}
	s.sessionRecreateHistory[jid] = t
}

// --- contadores (F36) ---

// IncrementIncoming incrementa e devolve o contador interno de pedidos de retry
// para a chave dada. Reproduz, inteira, a secao critica de
// incomingRetryRequestCounterLock que existia em handleRetryReceipt.
//
// F36: este mapa nunca e' esvaziado. Ver o doc de State.
func (s *State) IncrementIncoming(key IncomingKey) int {
	s.incomingCounterLock.Lock()
	defer s.incomingCounterLock.Unlock()
	if s.incomingCounter == nil {
		s.incomingCounter = make(map[IncomingKey]int)
	}
	s.incomingCounter[key]++
	return s.incomingCounter[key]
}

// BumpMessageRetries incrementa o contador de recibos de retry enviados para a
// mensagem id e devolve o valor resultante.
//
// countInMsg e' o `count` que veio no <enc> da mensagem recebida. A regra do
// upstream, preservada verbatim: se este e' o nosso primeiro recibo (contador
// = 1) mas a mensagem ja' diz ser um retry, reiniciamos o contador a partir do
// valor do servidor — foi o processo que reiniciou no meio, nao o par que
// parou de insistir. A gravacao de volta no mapa acontece DENTRO da mesma secao
// critica, como antes.
//
// F36: este mapa nunca e' esvaziado. Ver o doc de State.
func (s *State) BumpMessageRetries(id string, countInMsg int) int {
	s.messageRetriesLock.Lock()
	defer s.messageRetriesLock.Unlock()
	if s.messageRetries == nil {
		s.messageRetries = make(map[string]int)
	}
	s.messageRetries[id]++
	count := s.messageRetries[id]
	if count == 1 && countInMsg > 0 {
		count = countInMsg + 1
		s.messageRetries[id] = count
	}
	return count
}

// Sema devolve o semaforo de paralelismo, ou nil quando ilimitado.
func (s *State) Sema() *semaphore.Weighted { return s.sema }

// SetMaxParallel define quantos recibos de retry podem ser tratados em
// paralelo; n <= 0 significa ilimitado.
//
// Nao toma lock, como o SetMaxParallelRetryReceiptHandling original: o godoc
// desse metodo publico ja' diz que ele so' pode ser chamado antes de conectar.
func (s *State) SetMaxParallel(n int64) {
	if n <= 0 {
		s.sema = nil
	} else {
		s.sema = semaphore.NewWeighted(n)
	}
}

// --- buffer circular de mensagens recentes ---

// AddRecent grava msg no buffer circular, despejando a entrada mais antiga
// quando o anel da a volta.
//
// A secao critica e' a mesma de antes, do Lock ao Unlock explicito (o original
// nao usava defer aqui, e a ordem — despejo, gravacao, avanco do ponteiro,
// wrap — e' load-bearing: o despejo tem de ver o ponteiro ANTES do avanco).
func (s *State) AddRecent(key RecentKey, msg RecentMessage) {
	s.recentLock.Lock()
	if s.recentMap == nil {
		s.recentMap = make(map[RecentKey]RecentMessage, RecentMessagesSize)
	}
	if s.recentList[s.recentPtr].ID != "" {
		delete(s.recentMap, s.recentList[s.recentPtr])
	}
	s.recentMap[key] = msg
	s.recentList[s.recentPtr] = key
	s.recentPtr++
	if s.recentPtr >= len(s.recentList) {
		s.recentPtr = 0
	}
	s.recentLock.Unlock()
}

// GetRecent devolve a mensagem em cache, ou o zero de RecentMessage.
func (s *State) GetRecent(key RecentKey) RecentMessage {
	s.recentLock.RLock()
	defer s.recentLock.RUnlock()
	return s.recentMap[key]
}

// LastStoreClear devolve o carimbo do ultimo expurgo do store de retry.
//
// Devolve sempre o zero na pratica: nada escreve neste campo. Ver F52 em
// HOUSEKEEP.md e o doc do campo.
func (s *State) LastStoreClear() time.Time { return s.lastStoreClear }

// --- pedidos de reenvio ao telefone ---

// RegisterPendingPhone registra cancel como o cancelador do pedido pendente de
// id, e devolve false quando ja' havia um pedido em andamento (caso em que nada
// e' gravado). Reproduz a secao critica de escrita de delayedRequestMessageFromPhone.
func (s *State) RegisterPendingPhone(id types.MessageID, cancel context.CancelFunc) bool {
	s.pendingPhoneLock.Lock()
	defer s.pendingPhoneLock.Unlock()
	if s.pendingPhone == nil {
		s.pendingPhone = make(map[types.MessageID]context.CancelFunc)
	}
	if _, alreadyRequesting := s.pendingPhone[id]; alreadyRequesting {
		return false
	}
	s.pendingPhone[id] = cancel
	return true
}

// UnregisterPendingPhone remove o pedido pendente de id.
func (s *State) UnregisterPendingPhone(id types.MessageID) {
	s.pendingPhoneLock.Lock()
	defer s.pendingPhoneLock.Unlock()
	delete(s.pendingPhone, id)
}

// CancelPendingPhone cancela o pedido pendente de id, se houver.
//
// O cancel() e' chamado COM o RLock segurado, como no original. Isso e' seguro
// porque a funcao de cancelamento de um context.WithCancel nao reentra neste
// State — quem observa o Done() e' a goroutine do pedido, que so' volta a tocar
// o mapa depois, pelo UnregisterPendingPhone diferido. Manter a chamada dentro
// do lock preserva a semantica original.
func (s *State) CancelPendingPhone(id types.MessageID) {
	s.pendingPhoneLock.RLock()
	cancelPendingRequest, ok := s.pendingPhone[id]
	if ok {
		cancelPendingRequest()
	}
	s.pendingPhoneLock.RUnlock()
}

// CancelAllPendingPhone cancela todos os pedidos pendentes.
//
// Nao apaga o mapa, como o clearDelayedMessageRequests original: cada goroutine
// cancelada remove a propria entrada pelo defer dela.
func (s *State) CancelAllPendingPhone() {
	s.pendingPhoneLock.Lock()
	defer s.pendingPhoneLock.Unlock()
	for _, cancel := range s.pendingPhone {
		cancel()
	}
}
