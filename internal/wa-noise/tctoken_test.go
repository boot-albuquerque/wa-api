// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"
	"time"

	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/util/log"
)

// A janela de validade e' de tcTokenNumBuckets buckets de tcTokenBucketDuration
// segundos cada (4 x 7 dias = ~28 dias). O cutoff e' o inicio do bucket mais
// antigo ainda valido, nao "agora menos 28 dias".
func TestCurrentTCTokenCutoffTimestampIsBucketAligned(t *testing.T) {
	cutoff := currentTCTokenCutoffTimestamp()

	if cutoff.Unix()%tcTokenBucketDuration != 0 {
		t.Errorf("cutoff %d nao esta alinhado ao inicio de um bucket", cutoff.Unix())
	}
	age := time.Since(cutoff)
	minAge := time.Duration(tcTokenNumBuckets-1) * tcTokenBucketDuration * time.Second
	maxAge := time.Duration(tcTokenNumBuckets) * tcTokenBucketDuration * time.Second
	if age < minAge || age > maxAge {
		t.Errorf("idade do cutoff = %v, esperado entre %v e %v", age, minAge, maxAge)
	}
}

func TestIsTCTokenExpired(t *testing.T) {
	cutoff := currentTCTokenCutoffTimestamp()

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
			if got := isTCTokenExpired(tc.ts); got != tc.want {
				t.Errorf("= %v, esperado %v", got, tc.want)
			}
		})
	}
}

// Um novo token so' e' emitido quando o relogio cruza a fronteira de um bucket,
// nao a cada tcTokenBucketDuration segundos desde a ultima emissao.
func TestShouldSendNewTCToken(t *testing.T) {
	now := time.Now()
	currentBucketStart := time.Unix(now.Unix()/tcTokenBucketDuration*tcTokenBucketDuration, 0)

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
			if got := shouldSendNewTCToken(tc.senderTS); got != tc.want {
				t.Errorf("= %v, esperado %v", got, tc.want)
			}
		})
	}
}

// shouldSendTCTokenInChatAction e shouldSendCsToken decidem se o token
// acompanha a mensagem. Hoje os dois tem corpo identico (ver PATCHES.md); o
// teste roda a mesma tabela nos dois para que uma divergencia futura apareca.
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
			if got := shouldSendTCTokenInChatAction(tc.jid); got != tc.want {
				t.Errorf("shouldSendTCTokenInChatAction = %v, esperado %v", got, tc.want)
			}
			if got := shouldSendCsToken(tc.jid); got != tc.want {
				t.Errorf("shouldSendCsToken = %v, esperado %v", got, tc.want)
			}
		})
	}
}

// O mapa em memoria e chaveado por JID sem device: mensagens do mesmo contato
// vindas de aparelhos diferentes precisam compartilhar o timestamp.
func TestTCTokenSenderTSIsKeyedWithoutDevice(t *testing.T) {
	cli := newTCTokenTestClient()
	base := types.NewJID("5511999999999", types.DefaultUserServer)
	withDevice := base
	withDevice.Device = 7
	ts := time.Now().Truncate(time.Second)

	cli.setTCTokenSenderTS(withDevice, ts)

	if got := cli.getTCTokenSenderTS(base); !got.Equal(ts) {
		t.Errorf("= %v, esperado %v", got, ts)
	}
}

func TestValidateAndSetTCTokenSenderTS(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)

	t.Run("timestamp valido e aceito e memorizado", func(t *testing.T) {
		cli := newTCTokenTestClient()
		ts := time.Now()

		if !cli.validateAndSetTCTokenSenderTS(jid, ts) {
			t.Fatal("recusou timestamp valido")
		}
		if got := cli.getTCTokenSenderTS(jid); !got.Equal(ts) {
			t.Errorf("nao memorizou: = %v, esperado %v", got, ts)
		}
	})

	t.Run("timestamp zerado e recusado", func(t *testing.T) {
		cli := newTCTokenTestClient()
		if cli.validateAndSetTCTokenSenderTS(jid, time.Time{}) {
			t.Error("aceitou timestamp zerado")
		}
	})

	t.Run("timestamp expirado e recusado", func(t *testing.T) {
		cli := newTCTokenTestClient()
		if cli.validateAndSetTCTokenSenderTS(jid, currentTCTokenCutoffTimestamp().Add(-time.Hour)) {
			t.Error("aceitou timestamp anterior ao cutoff")
		}
	})

	t.Run("entrada ja em memoria e aceita sem revalidar", func(t *testing.T) {
		cli := newTCTokenTestClient()
		cli.setTCTokenSenderTS(jid, time.Now())

		// Mesmo com um timestamp invalido, a entrada existente ganha.
		if !cli.validateAndSetTCTokenSenderTS(jid, time.Time{}) {
			t.Error("recusou apesar de ja haver entrada em memoria")
		}
	})
}

// A limpeza do mapa so' roda uma vez por bucket; entradas anteriores ao cutoff
// somem, as demais ficam.
func TestUnlockedCleanupTCTokenSenderTSMapDropsExpiredEntries(t *testing.T) {
	cli := newTCTokenTestClient()
	fresh := types.NewJID("111", types.DefaultUserServer)
	stale := types.NewJID("222", types.DefaultUserServer)
	cli.tcTokenSenderTS[fresh] = time.Now()
	cli.tcTokenSenderTS[stale] = currentTCTokenCutoffTimestamp().Add(-time.Hour)
	// Força a limpeza a rodar: sem isso ela sai cedo pelo intervalo.
	cli.lastTCTokenSenderTSCleanup = time.Time{}

	cli.unlockedCleanupTCTokenSenderTSMap()

	if _, ok := cli.tcTokenSenderTS[stale]; ok {
		t.Error("entrada expirada sobreviveu a limpeza")
	}
	if _, ok := cli.tcTokenSenderTS[fresh]; !ok {
		t.Error("entrada valida foi removida")
	}
}

// newTCTokenTestClient monta o minimo de Client que as funcoes de tctoken em
// memoria exigem: o mapa e o logger. Sem store e sem socket.
func newTCTokenTestClient() *Client {
	return &Client{
		Log:             waLog.Noop,
		tcTokenSenderTS: make(map[types.JID]time.Time),
	}
}
