package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/internal/wa-noise/protocol/types"
)

// NoopStore existe para dois usos com contratos OPOSTOS:
//
//   - Device.Delete o instala com ErrDeviceDeleted: toda operacao tem que
//     FALHAR, para que uso posterior de uma sessao encerrada seja barrado.
//   - Alguns metodos, porem, sao no-op de verdade e devolvem nil mesmo assim
//     (buffer de evento, buffer de retry, GetAllAppStateSyncKeys) — sao caches
//     opcionais, e falhar neles quebraria o fluxo de mensagem em vez de
//     protege-lo.
//
// Os dois grupos abaixo travam exatamente essa divisao.

var errNoop = errors.New("noop de teste")

func newNoop() *NoopStore { return &NoopStore{Error: errNoop} }

func TestNoopStoreSatisfiesAllStoreInterfaces(t *testing.T) {
	// Redundante com os asserts de compilacao em noop.go, mas explicita a
	// intencao: NoopStore precisa cobrir AllStores e DeviceContainer inteiros.
	var _ AllStores = newNoop()
	var _ DeviceContainer = newNoop()
}

func TestNoopStorePropagatesErrorOnFailingMethods(t *testing.T) {
	ctx := context.Background()
	n := newNoop()
	jid := types.JID{User: "1", Server: types.DefaultUserServer}
	var key [32]byte
	var hash [128]byte

	checks := map[string]func() error{
		"PutIdentity":               func() error { return n.PutIdentity(ctx, "a:0", key) },
		"DeleteAllIdentities":       func() error { return n.DeleteAllIdentities(ctx, "a") },
		"DeleteIdentity":            func() error { return n.DeleteIdentity(ctx, "a:0") },
		"IsTrustedIdentity":         func() error { _, err := n.IsTrustedIdentity(ctx, "a:0", key); return err },
		"GetSession":                func() error { _, err := n.GetSession(ctx, "a:0"); return err },
		"HasSession":                func() error { _, err := n.HasSession(ctx, "a:0"); return err },
		"GetManySessions":           func() error { _, err := n.GetManySessions(ctx, nil); return err },
		"PutSession":                func() error { return n.PutSession(ctx, "a:0", nil) },
		"PutManySessions":           func() error { return n.PutManySessions(ctx, nil) },
		"DeleteAllSessions":         func() error { return n.DeleteAllSessions(ctx, "a") },
		"DeleteSession":             func() error { return n.DeleteSession(ctx, "a:0") },
		"MigratePNToLID":            func() error { return n.MigratePNToLID(ctx, jid, jid) },
		"GetOrGenPreKeys":           func() error { _, err := n.GetOrGenPreKeys(ctx, 1); return err },
		"GenOnePreKey":              func() error { _, err := n.GenOnePreKey(ctx); return err },
		"GetPreKey":                 func() error { _, err := n.GetPreKey(ctx, 1); return err },
		"RemovePreKey":              func() error { return n.RemovePreKey(ctx, 1) },
		"MarkPreKeysAsUploaded":     func() error { return n.MarkPreKeysAsUploaded(ctx, 1) },
		"UploadedPreKeyCount":       func() error { _, err := n.UploadedPreKeyCount(ctx); return err },
		"PutSenderKey":              func() error { return n.PutSenderKey(ctx, "g", "u", nil) },
		"GetSenderKey":              func() error { _, err := n.GetSenderKey(ctx, "g", "u"); return err },
		"PutAppStateSyncKey":        func() error { return n.PutAppStateSyncKey(ctx, nil, AppStateSyncKey{}) },
		"GetAppStateSyncKey":        func() error { _, err := n.GetAppStateSyncKey(ctx, nil); return err },
		"GetLatestAppStateSyncKey":  func() error { _, err := n.GetLatestAppStateSyncKeyID(ctx); return err },
		"PutAppStateVersion":        func() error { return n.PutAppStateVersion(ctx, "n", 1, hash) },
		"GetAppStateVersion":        func() error { _, _, err := n.GetAppStateVersion(ctx, "n"); return err },
		"DeleteAppStateVersion":     func() error { return n.DeleteAppStateVersion(ctx, "n") },
		"PutAppStateMutationMACs":   func() error { return n.PutAppStateMutationMACs(ctx, "n", 1, nil) },
		"DeleteAppStateMutationMAC": func() error { return n.DeleteAppStateMutationMACs(ctx, "n", nil) },
		"GetAppStateMutationMAC":    func() error { _, err := n.GetAppStateMutationMAC(ctx, "n", nil); return err },
		"PutPushName":               func() error { _, _, err := n.PutPushName(ctx, jid, "x"); return err },
		"PutBusinessName":           func() error { _, _, err := n.PutBusinessName(ctx, jid, "x"); return err },
		"PutContactName":            func() error { return n.PutContactName(ctx, jid, "a", "b") },
		"PutAllContactNames":        func() error { return n.PutAllContactNames(ctx, nil) },
		"PutManyRedactedPhones":     func() error { return n.PutManyRedactedPhones(ctx, nil) },
		"GetContact":                func() error { _, err := n.GetContact(ctx, jid); return err },
		"GetAllContacts":            func() error { _, err := n.GetAllContacts(ctx); return err },
		"PutMutedUntil":             func() error { return n.PutMutedUntil(ctx, jid, time.Now()) },
		"PutPinned":                 func() error { return n.PutPinned(ctx, jid, true) },
		"PutArchived":               func() error { return n.PutArchived(ctx, jid, true) },
		"GetChatSettings":           func() error { _, err := n.GetChatSettings(ctx, jid); return err },
		"PutMessageSecrets":         func() error { return n.PutMessageSecrets(ctx, nil) },
		"PutMessageSecret":          func() error { return n.PutMessageSecret(ctx, jid, jid, "id", nil) },
		"GetMessageSecret":          func() error { _, _, err := n.GetMessageSecret(ctx, jid, jid, "id"); return err },
		"PutPrivacyTokens":          func() error { return n.PutPrivacyTokens(ctx) },
		"GetPrivacyToken":           func() error { _, err := n.GetPrivacyToken(ctx, jid); return err },
		"DeleteExpiredPrivacyToken": func() error { _, err := n.DeleteExpiredPrivacyTokens(ctx, time.Now()); return err },
		"PutNCTSalt":                func() error { return n.PutNCTSalt(ctx, nil) },
		"GetNCTSalt":                func() error { _, err := n.GetNCTSalt(ctx); return err },
		"DeleteNCTSalt":             func() error { return n.DeleteNCTSalt(ctx) },
		"PutDevice":                 func() error { return n.PutDevice(ctx, nil) },
		"DeleteDevice":              func() error { return n.DeleteDevice(ctx, nil) },
		"GetLIDForPN":               func() error { _, err := n.GetLIDForPN(ctx, jid); return err },
		"GetPNForLID":               func() error { _, err := n.GetPNForLID(ctx, jid); return err },
		"GetManyLIDsForPNs":         func() error { _, err := n.GetManyLIDsForPNs(ctx, nil); return err },
		"PutManyLIDMappings":        func() error { return n.PutManyLIDMappings(ctx, nil) },
		"PutLIDMapping":             func() error { return n.PutLIDMapping(ctx, jid, jid) },
	}

	for name, call := range checks {
		if err := call(); !errors.Is(err, errNoop) {
			t.Errorf("%s deveria devolver o Error do NoopStore, veio %v", name, err)
		}
	}
}

// Estes metodos ignoram NoopStore.Error de proposito: sao buffers/caches
// opcionais, e devolver erro faria o pipeline de mensagem falhar em vez de
// simplesmente operar sem cache.
func TestNoopStoreBuffersSucceedSilently(t *testing.T) {
	ctx := context.Background()
	n := newNoop()
	var hash [32]byte
	jid := types.JID{User: "1", Server: types.DefaultUserServer}

	if buf, err := n.GetBufferedEvent(ctx, hash); err != nil || buf != nil {
		t.Errorf("GetBufferedEvent = (%v, %v), esperava (nil, nil)", buf, err)
	}
	if err := n.PutBufferedEvent(ctx, hash, nil, time.Now()); err != nil {
		t.Errorf("PutBufferedEvent = %v, esperava nil", err)
	}
	if err := n.ClearBufferedEventPlaintext(ctx, hash); err != nil {
		t.Errorf("ClearBufferedEventPlaintext = %v, esperava nil", err)
	}
	if err := n.DeleteOldBufferedHashes(ctx); err != nil {
		t.Errorf("DeleteOldBufferedHashes = %v, esperava nil", err)
	}
	if format, data, err := n.GetOutgoingEvent(ctx, jid, jid, "id"); err != nil || format != "" || data != nil {
		t.Errorf("GetOutgoingEvent = (%q, %v, %v), esperava (\"\", nil, nil)", format, data, err)
	}
	if err := n.AddOutgoingEvent(ctx, jid, "id", "f", nil); err != nil {
		t.Errorf("AddOutgoingEvent = %v, esperava nil", err)
	}
	if err := n.DeleteOldOutgoingEvents(ctx); err != nil {
		t.Errorf("DeleteOldOutgoingEvents = %v, esperava nil", err)
	}
	// GetAllAppStateSyncKeys tambem devolve nil: quem chama trata lista vazia
	// como "nenhuma chave conhecida", que e' verdade num store nulo.
	if all, err := n.GetAllAppStateSyncKeys(ctx); err != nil || all != nil {
		t.Errorf("GetAllAppStateSyncKeys = (%v, %v), esperava (nil, nil)", all, err)
	}
}

// DoDecryptionTxn nao abre transacao nenhuma, so' executa o callback. Sem isso,
// um Device sem banco nao conseguiria decifrar mensagem alguma.
func TestNoopStoreDoDecryptionTxnRunsCallback(t *testing.T) {
	called := false
	err := newNoop().DoDecryptionTxn(context.Background(), func(ctx context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("DoDecryptionTxn: %v", err)
	}
	if !called {
		t.Fatal("DoDecryptionTxn deveria executar o callback")
	}
}

func TestNoopStoreDoDecryptionTxnPropagatesCallbackError(t *testing.T) {
	boom := errors.New("boom")
	err := newNoop().DoDecryptionTxn(context.Background(), func(ctx context.Context) error {
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do callback, veio %v", err)
	}
}

// NoopDevice e' o device sentinela usado quando nao ha' sessao: precisa ter
// todos os stores preenchidos, senao qualquer acesso viraria panic de nil.
func TestNoopDeviceHasEveryStoreWired(t *testing.T) {
	d := NoopDevice
	if d.Identities == nil || d.Sessions == nil || d.PreKeys == nil || d.SenderKeys == nil ||
		d.AppStateKeys == nil || d.AppState == nil || d.Contacts == nil || d.ChatSettings == nil ||
		d.MsgSecrets == nil || d.PrivacyTokens == nil || d.NCTSalt == nil || d.EventBuffer == nil ||
		d.LIDs == nil || d.Container == nil {
		t.Fatalf("NoopDevice tem store nil: %+v", d)
	}
	if d.NoiseKey == nil || d.IdentityKey == nil {
		t.Fatal("NoopDevice deveria ter chaves nao-nil (zeradas) para nao panicar")
	}
	if d.GetJID() != types.EmptyJID {
		t.Fatalf("NoopDevice.GetJID = %s, esperava EmptyJID", d.GetJID())
	}
}

func TestNoopDeviceOperationsFailWithStoreIsNil(t *testing.T) {
	err := NoopDevice.Sessions.PutSession(context.Background(), "a:0", nil)
	if err == nil {
		t.Fatal("as operacoes do NoopDevice deveriam falhar")
	}
	if err.Error() != "store is nil" {
		t.Fatalf("erro = %q, esperava \"store is nil\"", err)
	}
}
