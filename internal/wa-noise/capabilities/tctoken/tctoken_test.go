// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tctoken

import (
	"testing"
	"time"

	"wa-api/internal/wa-noise/protocol/types"
)

// A janela de validade e' de NumBuckets buckets de BucketDuration
// segundos cada (4 x 7 dias = ~28 dias). O cutoff e' o inicio do bucket mais
// antigo ainda valido, nao "agora menos 28 dias".
func TestCurrentTCTokenCutoffTimestampIsBucketAligned(t *testing.T) {
	cutoff := CurrentCutoffTimestamp()

	if cutoff.Unix()%BucketDuration != 0 {
		t.Errorf("cutoff %d nao esta alinhado ao inicio de um bucket", cutoff.Unix())
	}
	age := time.Since(cutoff)
	minAge := time.Duration(NumBuckets-1) * BucketDuration * time.Second
	maxAge := time.Duration(NumBuckets) * BucketDuration * time.Second
	if age < minAge || age > maxAge {
		t.Errorf("idade do cutoff = %v, esperado entre %v e %v", age, minAge, maxAge)
	}
}

func TestIsTCTokenExpired(t *testing.T) {
	cutoff := CurrentCutoffTimestamp()

	for name, tc := range map[string]struct {
		ts   time.Time
		want bool
	}{
		"zerado":              {time.Time{}, true},
		"antes do cutoff":     {cutoff.Add(-time.Second), true},
		"exatamente o cutoff": {cutoff, false},
		"depois do cutoff":    {cutoff.Add(time.Hour), false},
		"agora":               {time.Now(), false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := IsExpired(tc.ts); got != tc.want {
				t.Errorf("= %v, esperado %v", got, tc.want)
			}
		})
	}
}

// Um novo token so' e' emitido quando o relogio cruza a fronteira de um bucket,
// nao a cada BucketDuration segundos desde a ultima emissao.
func TestShouldSendNewTCToken(t *testing.T) {
	now := time.Now()
	currentBucketStart := time.Unix(now.Unix()/BucketDuration*BucketDuration, 0)

	for name, tc := range map[string]struct {
		senderTS time.Time
		want     bool
	}{
		"nunca emitido":              {time.Time{}, true},
		"emitido no bucket atual":    {currentBucketStart, false},
		"emitido agora":              {now, false},
		"emitido no bucket anterior": {currentBucketStart.Add(-time.Second), true},
		"emitido muito tempo atras":  {currentBucketStart.Add(-100 * 24 * time.Hour), true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ShouldSendNew(tc.senderTS); got != tc.want {
				t.Errorf("= %v, esperado %v", got, tc.want)
			}
		})
	}
}

// ShouldSendInChatAction decide se o token acompanha a mensagem. A gemea
// shouldSendCsToken (cstoken.go, na raiz) tem corpo identico hoje e roda a
// mesma tabela do lado de la', para que uma divergencia futura apareca.
func TestShouldSendTokenPerJIDType(t *testing.T) {
	for name, tc := range map[string]struct {
		jid  types.JID
		want bool
	}{
		"usuario comum":       {types.NewJID("5511999999999", types.DefaultUserServer), true},
		"usuario oculto/LID":  {types.NewJID("12345", types.HiddenUserServer), true},
		"com device":          {types.JID{User: "5511999999999", Server: types.DefaultUserServer, Device: 3}, true},
		"grupo":               {types.NewJID("123-456", types.GroupServer), false},
		"broadcast de status": {types.StatusBroadcastJID, false},
		"newsletter":          {types.NewJID("123", types.NewsletterServer), false},
		"PSA":                 {types.PSAJID, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ShouldSendInChatAction(tc.jid); got != tc.want {
				t.Errorf("ShouldSendInChatAction = %v, esperado %v", got, tc.want)
			}
		})
	}
}

// O mapa em memoria e chaveado por JID sem device: mensagens do mesmo contato
// vindas de aparelhos diferentes precisam compartilhar o timestamp.
func TestTCTokenSenderTSIsKeyedWithoutDevice(t *testing.T) {
	var s State
	base := types.NewJID("5511999999999", types.DefaultUserServer)
	withDevice := base
	withDevice.Device = 7
	ts := time.Now().Truncate(time.Second)

	s.SetSenderTS(withDevice, ts)

	if got := s.SenderTS(base); !got.Equal(ts) {
		t.Errorf("= %v, esperado %v", got, ts)
	}
}

func TestValidateAndSetTCTokenSenderTS(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)

	t.Run("timestamp valido e aceito e memorizado", func(t *testing.T) {
		var s State
		ts := time.Now()

		if !s.ValidateAndSet(jid, ts) {
			t.Fatal("recusou timestamp valido")
		}
		if got := s.SenderTS(jid); !got.Equal(ts) {
			t.Errorf("nao memorizou: = %v, esperado %v", got, ts)
		}
	})

	t.Run("timestamp zerado e recusado", func(t *testing.T) {
		var s State
		if s.ValidateAndSet(jid, time.Time{}) {
			t.Error("aceitou timestamp zerado")
		}
	})

	t.Run("timestamp expirado e recusado", func(t *testing.T) {
		var s State
		if s.ValidateAndSet(jid, CurrentCutoffTimestamp().Add(-time.Hour)) {
			t.Error("aceitou timestamp anterior ao cutoff")
		}
	})

	t.Run("entrada ja em memoria e aceita sem revalidar", func(t *testing.T) {
		var s State
		s.SetSenderTS(jid, time.Now())

		// Mesmo com um timestamp invalido, a entrada existente ganha.
		if !s.ValidateAndSet(jid, time.Time{}) {
			t.Error("recusou apesar de ja haver entrada em memoria")
		}
	})
}

// A limpeza do mapa so' roda uma vez por bucket; entradas anteriores ao cutoff
// somem, as demais ficam.
func TestUnlockedCleanupTCTokenSenderTSMapDropsExpiredEntries(t *testing.T) {
	var s State
	// O mapa e' criado preguicosamente sob o lock; este teste chama a poda
	// direto, entao inicializa a mao.
	s.senderTS = map[types.JID]time.Time{}
	fresh := types.NewJID("111", types.DefaultUserServer)
	stale := types.NewJID("222", types.DefaultUserServer)
	s.senderTS[fresh] = time.Now()
	s.senderTS[stale] = CurrentCutoffTimestamp().Add(-time.Hour)
	// Força a limpeza a rodar: sem isso ela sai cedo pelo intervalo.
	s.lastCleanup = time.Time{}

	s.unlockedCleanup()

	if _, ok := s.senderTS[stale]; ok {
		t.Error("entrada expirada sobreviveu a limpeza")
	}
	if _, ok := s.senderTS[fresh]; !ok {
		t.Error("entrada valida foi removida")
	}
}

// A poda do mapa e' throttled: dentro de um bucket ela sai cedo, e uma entrada
// expirada gravada logo depois sobrevive ate' a proxima janela.
func TestUnlockedCleanupRespeitaOIntervalo(t *testing.T) {
	var s State
	fresh := types.NewJID("111", types.DefaultUserServer)
	stale := types.NewJID("222", types.DefaultUserServer)

	// A primeira gravacao roda a poda (lastCleanup zerado) e a marca.
	s.SetSenderTS(fresh, time.Now())

	s.senderTSLock.Lock()
	s.senderTS[stale] = CurrentCutoffTimestamp().Add(-time.Hour)
	s.senderTSLock.Unlock()

	// A segunda gravacao cai dentro do intervalo: a poda sai cedo.
	s.SetSenderTS(fresh, time.Now())

	s.senderTSLock.Lock()
	defer s.senderTSLock.Unlock()
	if _, ok := s.senderTS[stale]; !ok {
		t.Error("a entrada expirada nao deveria ter sido podada dentro do intervalo")
	}
}
