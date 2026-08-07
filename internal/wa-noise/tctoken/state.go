// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tctoken

import (
	"sync"
	"time"

	"wa-api/internal/wa-noise/protocol/types"
)

// State e' o estado mutavel do dominio de tctoken. Reune os cinco campos que
// viviam soltos em *Client antes desta extracao:
//
//	tcTokenSenderTS            map[types.JID]time.Time -> senderTS
//	tcTokenSenderTSLock        sync.Mutex              -> senderTSLock
//	lastTCTokenSenderTSCleanup time.Time               -> lastCleanup
//	tcTokenDBPruneLock         sync.Mutex              -> dbPruneLock
//	lastTCTokenDBPrune         time.Time               -> lastDBPrune
//
// Os dois locks continuam sendo dois locks distintos, com os mesmos pontos de
// aquisicao e liberacao de antes. Por conter mutexes, State NUNCA pode ser
// copiado por valor depois de usado — sempre passe *State.
//
// O zero value e' usavel: o mapa e' criado preguicosamente sob o lock de
// escrita. Antes ele era criado eagerly em NewClient; uma leitura antes de
// qualquer gravacao enxerga mapa nil, o que em Go devolve o zero — mesmo
// resultado que um mapa vazio. E' a mesma mudanca (e o mesmo racional) do lote 3.
type State struct {
	// senderTSLock protege senderTS e lastCleanup.
	senderTSLock sync.Mutex
	// senderTS guarda, por JID sem device, quando o token daquele contato foi
	// emitido pela ultima vez.
	senderTS map[types.JID]time.Time
	// lastCleanup e' quando o mapa foi podado pela ultima vez.
	lastCleanup time.Time

	// dbPruneLock serializa a poda de tokens expirados no banco. E' usado com
	// TryLock e liberado na goroutine que faz a poda — ver Prune.
	dbPruneLock sync.Mutex
	// lastDBPrune e' quando a poda no banco rodou pela ultima vez. Protegido
	// por dbPruneLock.
	lastDBPrune time.Time
}

// SenderTS le' o timestamp de emissao em memoria de um JID.
//
// Era Client.getTCTokenSenderTS.
func (s *State) SenderTS(jid types.JID) time.Time {
	s.senderTSLock.Lock()
	defer s.senderTSLock.Unlock()

	return s.senderTS[jid.ToNonAD()]
}

// ValidateAndSet adota o timestamp que veio do banco quando ainda nao ha' nada
// em memoria para aquele JID e o valor nao esta' expirado. Devolve true quando,
// ao final, existe um timestamp valido em memoria.
//
// Era Client.validateAndSetTCTokenSenderTS. A poda continua acontecendo sob o
// mesmo lock, logo apos a gravacao.
func (s *State) ValidateAndSet(jid types.JID, storedSenderTimestamp time.Time) bool {
	s.senderTSLock.Lock()
	defer s.senderTSLock.Unlock()

	key := jid.ToNonAD()
	if _, ok := s.senderTS[key]; ok {
		return true
	}
	if storedSenderTimestamp.IsZero() || storedSenderTimestamp.Before(CurrentCutoffTimestamp()) {
		return false
	}
	if s.senderTS == nil {
		s.senderTS = make(map[types.JID]time.Time)
	}
	s.senderTS[key] = storedSenderTimestamp
	s.unlockedCleanup()
	return true
}

// SetSenderTS grava o timestamp de emissao em memoria de um JID.
//
// Era Client.setTCTokenSenderTS.
func (s *State) SetSenderTS(jid types.JID, ts time.Time) {
	s.senderTSLock.Lock()
	defer s.senderTSLock.Unlock()

	if s.senderTS == nil {
		s.senderTS = make(map[types.JID]time.Time)
	}
	s.senderTS[jid.ToNonAD()] = ts
	s.unlockedCleanup()
}

// unlockedCleanup poda as entradas expiradas do mapa. So' pode ser chamada com
// senderTSLock segurado — era Client.unlockedCleanupTCTokenSenderTSMap, com o
// mesmo contrato.
func (s *State) unlockedCleanup() {
	if time.Since(s.lastCleanup) < BucketDuration*time.Second {
		return
	}
	s.lastCleanup = time.Now()
	cutoffTimestamp := CurrentCutoffTimestamp()
	for jid, ts := range s.senderTS {
		if ts.Before(cutoffTimestamp) {
			delete(s.senderTS, jid)
		}
	}
}

// TryStartDBPrune decide se a poda no banco deve rodar agora.
//
// Reproduz exatamente o inicio de Client.deleteExpiredPrivacyTokens: pega o
// lock com TryLock (se outra poda esta' em andamento, desiste calado), confere
// o intervalo e, quando desiste, LIBERA o lock. Quando devolve true, o lock
// CONTINUA SEGURADO — quem chama e' responsavel por libera-lo com
// FinishDBPrune, tipicamente num `defer` dentro da goroutine de poda, como o
// original fazia.
func (s *State) TryStartDBPrune(interval time.Duration) bool {
	if !s.dbPruneLock.TryLock() {
		return false
	}
	if time.Since(s.lastDBPrune) < interval {
		s.dbPruneLock.Unlock()
		return false
	}
	s.lastDBPrune = time.Now()
	return true
}

// FinishDBPrune libera o lock tomado por um TryStartDBPrune que devolveu true.
func (s *State) FinishDBPrune() { s.dbPruneLock.Unlock() }
