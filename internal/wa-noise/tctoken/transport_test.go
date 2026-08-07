// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tctoken

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/util/log"
)

// ------------------------------------------------------------------ dubles

// fakeLIDStore resolve PN -> LID em memoria.
type fakeLIDStore struct {
	lid types.JID
	err error
}

func (f *fakeLIDStore) GetLIDForPN(context.Context, types.JID) (types.JID, error) {
	return f.lid, f.err
}
func (f *fakeLIDStore) GetPNForLID(context.Context, types.JID) (types.JID, error) {
	return types.EmptyJID, nil
}
func (f *fakeLIDStore) PutManyLIDMappings(context.Context, []store.LIDMapping) error { return nil }
func (f *fakeLIDStore) PutLIDMapping(context.Context, types.JID, types.JID) error    { return nil }
func (f *fakeLIDStore) GetManyLIDsForPNs(context.Context, []types.JID) (map[types.JID]types.JID, error) {
	return nil, nil
}

// fakePrivacyTokenStore guarda tokens em memoria e conta as chamadas.
type fakePrivacyTokenStore struct {
	mu sync.Mutex

	token     *store.PrivacyToken
	getErr    error
	putErr    error
	put       []store.PrivacyToken
	deleted   int
	deleteErr error
	deletes   int
}

func (f *fakePrivacyTokenStore) PutPrivacyTokens(_ context.Context, tokens ...store.PrivacyToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.put = append(f.put, tokens...)
	return f.putErr
}

func (f *fakePrivacyTokenStore) GetPrivacyToken(context.Context, types.JID) (*store.PrivacyToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.token == nil {
		return nil, nil
	}
	cp := *f.token
	return &cp, nil
}

func (f *fakePrivacyTokenStore) DeleteExpiredPrivacyTokens(context.Context, time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	return int64(f.deleted), f.deleteErr
}

func (f *fakePrivacyTokenStore) pruneCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deletes
}

// fakeTransport e' o duble de Transport: sem socket e sem banco.
type fakeTransport struct {
	mu sync.Mutex

	device  *store.Device
	state   State
	iqQueue []iqResult
	iqCalls []IQ
}

type iqResult struct {
	node *waBinary.Node
	err  error
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{device: &store.Device{
		PrivacyTokens: &fakePrivacyTokenStore{},
		LIDs:          &fakeLIDStore{},
	}}
}

func (f *fakeTransport) tokens() *fakePrivacyTokenStore {
	return f.device.PrivacyTokens.(*fakePrivacyTokenStore)
}

func (f *fakeTransport) lids() *fakeLIDStore { return f.device.LIDs.(*fakeLIDStore) }

func (f *fakeTransport) enqueueIQ(node *waBinary.Node, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.iqQueue = append(f.iqQueue, iqResult{node, err})
}

func (f *fakeTransport) Store() *store.Device           { return f.device }
func (f *fakeTransport) State() *State                  { return &f.state }
func (f *fakeTransport) Log() waLog.Logger              { return waLog.Noop }
func (f *fakeTransport) BackgroundCtx() context.Context { return context.Background() }

func (f *fakeTransport) SendIQ(_ context.Context, query IQ) (*waBinary.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.iqCalls = append(f.iqCalls, query)
	if len(f.iqQueue) == 0 {
		return nil, errors.New("fakeTransport: no queued IQ response")
	}
	res := f.iqQueue[0]
	f.iqQueue = f.iqQueue[1:]
	return res.node, res.err
}

func (f *fakeTransport) snapshotIQs() []IQ {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]IQ(nil), f.iqCalls...)
}

var _ Transport = (*fakeTransport)(nil)

func pn(user string) types.JID  { return types.NewJID(user, types.DefaultUserServer) }
func lid(user string) types.JID { return types.NewJID(user, types.HiddenUserServer) }

// -------------------------------------------------------- ResolveStorageLID

func TestResolveStorageLID(t *testing.T) {
	t.Run("PN resolve para LID", func(t *testing.T) {
		tr := newFakeTransport()
		tr.lids().lid = lid("12345")

		if got := ResolveStorageLID(t.Context(), tr, pn("5511999999999")); got != lid("12345") {
			t.Errorf("= %v, esperado o LID mapeado", got)
		}
	})

	t.Run("JID que ja e LID passa direto", func(t *testing.T) {
		tr := newFakeTransport()
		want := lid("999")
		if got := ResolveStorageLID(t.Context(), tr, want); got != want {
			t.Errorf("= %v, esperado %v", got, want)
		}
	})

	t.Run("device e sempre removido", func(t *testing.T) {
		tr := newFakeTransport()
		withDevice := lid("999")
		withDevice.Device = 4
		if got := ResolveStorageLID(t.Context(), tr, withDevice); got.Device != 0 {
			t.Errorf("= %v, esperado sem device", got)
		}
	})

	// Falha de resolucao, LID vazio ou store ausente: fica com o PN. E' o
	// fallback que evita perder o token quando o mapeamento ainda nao existe.
	t.Run("fallbacks mantem o PN", func(t *testing.T) {
		want := pn("5511999999999")

		for name, mutate := range map[string]func(*fakeTransport){
			"erro de resolucao": func(tr *fakeTransport) { tr.lids().err = errors.New("db fora do ar") },
			"LID vazio":         func(tr *fakeTransport) {},
			"sem sub-store LID": func(tr *fakeTransport) { tr.device.LIDs = nil },
			"sem store":         func(tr *fakeTransport) { tr.device = nil },
		} {
			t.Run(name, func(t *testing.T) {
				tr := newFakeTransport()
				mutate(tr)
				if got := ResolveStorageLID(t.Context(), tr, want); got != want {
					t.Errorf("= %v, esperado %v", got, want)
				}
			})
		}
	})
}

// ------------------------------------------------------------------ Ensure

func TestEnsure(t *testing.T) {
	jid := pn("5511999999999")

	t.Run("token valido volta", func(t *testing.T) {
		tr := newFakeTransport()
		tr.tokens().token = &store.PrivacyToken{Token: []byte("tok"), Timestamp: time.Now()}

		got, err := Ensure(t.Context(), tr, jid)
		if err != nil {
			t.Fatalf("Ensure: %v", err)
		}
		if string(got) != "tok" {
			t.Errorf("= %q, esperado \"tok\"", got)
		}
	})

	t.Run("token expirado nao volta", func(t *testing.T) {
		tr := newFakeTransport()
		tr.tokens().token = &store.PrivacyToken{
			Token: []byte("tok"), Timestamp: CurrentCutoffTimestamp().Add(-time.Hour),
		}

		got, err := Ensure(t.Context(), tr, jid)
		if err != nil || got != nil {
			t.Errorf("= (%q, %v), esperado (nil, nil)", got, err)
		}
	})

	t.Run("token vazio nao volta", func(t *testing.T) {
		tr := newFakeTransport()
		tr.tokens().token = &store.PrivacyToken{Timestamp: time.Now()}

		if got, _ := Ensure(t.Context(), tr, jid); got != nil {
			t.Errorf("= %q, esperado nil", got)
		}
	})

	t.Run("sem token armazenado", func(t *testing.T) {
		tr := newFakeTransport()
		got, err := Ensure(t.Context(), tr, jid)
		if err != nil || got != nil {
			t.Errorf("= (%q, %v), esperado (nil, nil)", got, err)
		}
	})

	t.Run("erro de leitura e embrulhado", func(t *testing.T) {
		tr := newFakeTransport()
		sentinel := errors.New("db fora do ar")
		tr.tokens().getErr = sentinel

		_, err := Ensure(t.Context(), tr, jid)
		if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "failed to get privacy token") {
			t.Fatalf("erro = %v", err)
		}
	})

	// O timestamp de emissao vindo do banco entra no cache em memoria.
	t.Run("adota o sender timestamp do banco", func(t *testing.T) {
		tr := newFakeTransport()
		senderTS := time.Now().Truncate(time.Second)
		tr.tokens().token = &store.PrivacyToken{
			Token: []byte("tok"), Timestamp: time.Now(), SenderTimestamp: senderTS,
		}

		if _, err := Ensure(t.Context(), tr, jid); err != nil {
			t.Fatalf("Ensure: %v", err)
		}
		if got := tr.State().SenderTS(jid); !got.Equal(senderTS) {
			t.Errorf("cache = %v, esperado %v", got, senderTS)
		}
	})
}

// ----------------------------------------------------------- DeleteExpired

// esperaPoda aguarda a goroutine de poda registrar a chamada.
func esperaPoda(t *testing.T, tr *fakeTransport, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if tr.tokens().pruneCalls() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("poda rodou %d vezes, esperado %d", tr.tokens().pruneCalls(), want)
}

func TestDeleteExpiredPodaUmaVezPorIntervalo(t *testing.T) {
	tr := newFakeTransport()
	tr.tokens().deleted = 3

	DeleteExpired(tr)
	esperaPoda(t, tr, 1)

	// A segunda chamada cai no intervalo e nao roda.
	DeleteExpired(tr)
	time.Sleep(20 * time.Millisecond)
	if got := tr.tokens().pruneCalls(); got != 1 {
		t.Errorf("poda rodou %d vezes, esperado 1 (a segunda esta' dentro do intervalo)", got)
	}
}

// Um erro na poda e' logado e engolido; o lock volta pelo defer.
func TestDeleteExpiredComErro(t *testing.T) {
	tr := newFakeTransport()
	tr.tokens().deleteErr = errors.New("db fora do ar")

	DeleteExpired(tr)
	esperaPoda(t, tr, 1)

	// Com o lock liberado, uma poda futura (fora do intervalo) volta a rodar.
	tr.State().dbPruneLock.Lock()
	tr.State().dbPruneLock.Unlock()
}

// Enquanto uma poda esta' em andamento o TryLock falha e a chamada desiste
// calada — sem bloquear o caminho de envio de mensagem.
func TestDeleteExpiredNaoBloqueiaQuandoJaEstaPodando(t *testing.T) {
	var s State
	if !s.TryStartDBPrune(DBPruneInterval) {
		t.Fatal("a primeira poda deveria comecar")
	}
	if s.TryStartDBPrune(DBPruneInterval) {
		t.Error("a segunda poda nao deveria comecar com o lock segurado")
	}
	s.FinishDBPrune()

	// Liberado o lock, o intervalo passa a ser o que barra.
	if s.TryStartDBPrune(DBPruneInterval) {
		t.Error("nao deveria repodar dentro do intervalo")
	}
	// Com intervalo zero, roda de novo.
	if !s.TryStartDBPrune(0) {
		t.Error("com intervalo zero a poda deveria comecar")
	}
	s.FinishDBPrune()
}

// ------------------------------------------------------- Issue/IssueAndSave

func TestIssueMontaONo(t *testing.T) {
	tr := newFakeTransport()
	tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)
	ts := time.Unix(1700000000, 0)
	jid := pn("5511999999999")

	if _, err := Issue(t.Context(), tr, jid, ts); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	call := tr.snapshotIQs()[0]
	if call.Namespace != "privacy" || call.Type != IQSet || call.To != types.ServerJID {
		t.Errorf("IQ = %+v, esperado set/privacy para o servidor", call)
	}
	token := call.Content.([]waBinary.Node)[0].Content.([]waBinary.Node)[0]
	if token.Attrs["type"] != TokenType {
		t.Errorf("type = %v, esperado %q", token.Attrs["type"], TokenType)
	}
	// O timestamp vai em SEGUNDOS, como string.
	if token.Attrs["t"] != "1700000000" {
		t.Errorf("t = %v, esperado \"1700000000\" (segundos)", token.Attrs["t"])
	}
	if token.Attrs["jid"] != jid {
		t.Errorf("jid = %v, esperado %v", token.Attrs["jid"], jid)
	}
}

func TestIssueAndSave(t *testing.T) {
	jid := pn("5511999999999")
	ts := time.Now().Truncate(time.Second)

	t.Run("caminho feliz persiste o sender timestamp", func(t *testing.T) {
		tr := newFakeTransport()
		tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)
		tr.tokens().token = &store.PrivacyToken{Token: []byte("tok")}

		IssueAndSave(tr, jid, ts)

		if got := tr.State().SenderTS(jid); !got.Equal(ts) {
			t.Errorf("cache = %v, esperado %v", got, ts)
		}
		if len(tr.tokens().put) != 1 || !tr.tokens().put[0].SenderTimestamp.Equal(ts) {
			t.Errorf("persistido = %+v, esperado uma gravacao com SenderTimestamp=%v", tr.tokens().put, ts)
		}
	})

	t.Run("falha na emissao nao toca o cache", func(t *testing.T) {
		tr := newFakeTransport()
		tr.enqueueIQ(nil, errors.New("socket fechado"))

		IssueAndSave(tr, jid, ts)

		if !tr.State().SenderTS(jid).IsZero() {
			t.Error("o cache nao deveria ter sido atualizado")
		}
		if len(tr.tokens().put) != 0 {
			t.Error("nao deveria ter persistido nada")
		}
	})

	// O cache em memoria e' atualizado ANTES da releitura do banco: uma falha
	// ali (ou a ausencia de token) nao desfaz a atualizacao.
	t.Run("falha na releitura mantem o cache", func(t *testing.T) {
		tr := newFakeTransport()
		tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)
		tr.tokens().getErr = errors.New("db fora do ar")

		IssueAndSave(tr, jid, ts)

		if got := tr.State().SenderTS(jid); !got.Equal(ts) {
			t.Errorf("cache = %v, esperado %v", got, ts)
		}
		if len(tr.tokens().put) != 0 {
			t.Error("nao deveria ter persistido")
		}
	})

	t.Run("sem token ou com token vazio nao persiste", func(t *testing.T) {
		for name, token := range map[string]*store.PrivacyToken{
			"sem token":   nil,
			"token vazio": {},
		} {
			t.Run(name, func(t *testing.T) {
				tr := newFakeTransport()
				tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)
				tr.tokens().token = token

				IssueAndSave(tr, jid, ts)

				if len(tr.tokens().put) != 0 {
					t.Errorf("persistiu %+v, esperado nada", tr.tokens().put)
				}
			})
		}
	})

	t.Run("falha ao persistir e so logada", func(t *testing.T) {
		tr := newFakeTransport()
		tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)
		tr.tokens().token = &store.PrivacyToken{Token: []byte("tok")}
		tr.tokens().putErr = errors.New("db fora do ar")

		IssueAndSave(tr, jid, ts)

		if got := tr.State().SenderTS(jid); !got.Equal(ts) {
			t.Error("o cache em memoria deveria continuar atualizado")
		}
	})
}
