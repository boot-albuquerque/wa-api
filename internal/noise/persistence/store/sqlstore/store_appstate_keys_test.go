package sqlstore

import (
	"bytes"
	"context"
	"testing"

	"wa-api/internal/noise/persistence/store"
)

func syncKey(data string, ts int64) store.AppStateSyncKey {
	return store.AppStateSyncKey{
		Data:        []byte(data),
		Fingerprint: []byte("fp-" + data),
		Timestamp:   ts,
	}
}

func TestGetAppStateSyncKeyMissingReturnsNil(t *testing.T) {
	key, err := newTestStore(t).GetAppStateSyncKey(context.Background(), []byte("nao-existe"))
	if err != nil {
		t.Fatalf("GetAppStateSyncKey: %v", err)
	}
	if key != nil {
		t.Fatalf("chave inexistente deveria devolver nil, veio %+v", key)
	}
}

func TestPutGetAppStateSyncKeyRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	want := syncKey("material", 100)
	if err := s.PutAppStateSyncKey(ctx, []byte("k1"), want); err != nil {
		t.Fatalf("PutAppStateSyncKey: %v", err)
	}
	got, err := s.GetAppStateSyncKey(ctx, []byte("k1"))
	if err != nil {
		t.Fatalf("GetAppStateSyncKey: %v", err)
	}
	if got == nil {
		t.Fatal("GetAppStateSyncKey devolveu nil")
	}
	if !bytes.Equal(got.Data, want.Data) || !bytes.Equal(got.Fingerprint, want.Fingerprint) || got.Timestamp != want.Timestamp {
		t.Fatalf("round trip divergiu: %+v != %+v", got, want)
	}
}

// O ON CONFLICT tem um WHERE excluded.timestamp > ...: uma chave mais VELHA
// para o mesmo key_id e' ignorada. Isso protege contra uma re-entrega fora de
// ordem sobrescrever a chave boa com uma antiga.
func TestPutAppStateSyncKeyIgnoresOlderTimestamp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutAppStateSyncKey(ctx, []byte("k1"), syncKey("nova", 200)); err != nil {
		t.Fatalf("PutAppStateSyncKey nova: %v", err)
	}
	if err := s.PutAppStateSyncKey(ctx, []byte("k1"), syncKey("velha", 100)); err != nil {
		t.Fatalf("PutAppStateSyncKey velha: %v", err)
	}
	got, err := s.GetAppStateSyncKey(ctx, []byte("k1"))
	if err != nil {
		t.Fatalf("GetAppStateSyncKey: %v", err)
	}
	if string(got.Data) != "nova" {
		t.Fatalf("uma chave mais velha sobrescreveu a mais nova: %q", got.Data)
	}
}

func TestPutAppStateSyncKeyAcceptsNewerTimestamp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutAppStateSyncKey(ctx, []byte("k1"), syncKey("velha", 100)); err != nil {
		t.Fatalf("PutAppStateSyncKey velha: %v", err)
	}
	if err := s.PutAppStateSyncKey(ctx, []byte("k1"), syncKey("nova", 200)); err != nil {
		t.Fatalf("PutAppStateSyncKey nova: %v", err)
	}
	got, err := s.GetAppStateSyncKey(ctx, []byte("k1"))
	if err != nil {
		t.Fatalf("GetAppStateSyncKey: %v", err)
	}
	if string(got.Data) != "nova" {
		t.Fatalf("a chave mais nova deveria ter substituido: %q", got.Data)
	}
}

func TestGetLatestAppStateSyncKeyIDPicksNewestTimestamp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutAppStateSyncKey(ctx, []byte("antiga"), syncKey("a", 100)); err != nil {
		t.Fatalf("Put antiga: %v", err)
	}
	if err := s.PutAppStateSyncKey(ctx, []byte("recente"), syncKey("b", 300)); err != nil {
		t.Fatalf("Put recente: %v", err)
	}
	if err := s.PutAppStateSyncKey(ctx, []byte("media"), syncKey("c", 200)); err != nil {
		t.Fatalf("Put media: %v", err)
	}
	id, err := s.GetLatestAppStateSyncKeyID(ctx)
	if err != nil {
		t.Fatalf("GetLatestAppStateSyncKeyID: %v", err)
	}
	if string(id) != "recente" {
		t.Fatalf("esperava a chave de maior timestamp, veio %q", id)
	}
}

func TestGetLatestAppStateSyncKeyIDEmptyReturnsNil(t *testing.T) {
	id, err := newTestStore(t).GetLatestAppStateSyncKeyID(context.Background())
	if err != nil {
		t.Fatalf("GetLatestAppStateSyncKeyID: %v", err)
	}
	if id != nil {
		t.Fatalf("sem chaves deveria devolver nil, veio %q", id)
	}
}

func TestGetAllAppStateSyncKeysOrdersByTimestampDesc(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for id, ts := range map[string]int64{"a": 100, "b": 300, "c": 200} {
		if err := s.PutAppStateSyncKey(ctx, []byte(id), syncKey(id, ts)); err != nil {
			t.Fatalf("Put %s: %v", id, err)
		}
	}
	all, err := s.GetAllAppStateSyncKeys(ctx)
	if err != nil {
		t.Fatalf("GetAllAppStateSyncKeys: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("esperava 3 chaves, veio %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Timestamp < all[i].Timestamp {
			t.Fatalf("a ordem nao e' decrescente por timestamp: %v", all)
		}
	}
}

// GetAllAppStateSyncKeys filtra explicitamente key_data vazio: uma chave sem
// material nao serve para decifrar nada e so' confundiria o consumidor.
func TestGetAllAppStateSyncKeysSkipsEmptyData(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if err := s.PutAppStateSyncKey(ctx, []byte("vazia"), syncKey("", 100)); err != nil {
		t.Fatalf("Put vazia: %v", err)
	}
	if err := s.PutAppStateSyncKey(ctx, []byte("cheia"), syncKey("x", 200)); err != nil {
		t.Fatalf("Put cheia: %v", err)
	}
	all, err := s.GetAllAppStateSyncKeys(ctx)
	if err != nil {
		t.Fatalf("GetAllAppStateSyncKeys: %v", err)
	}
	if len(all) != 1 || string(all[0].Data) != "x" {
		t.Fatalf("a chave de key_data vazio deveria ter sido filtrada: %v", all)
	}
}

func TestAppStateSyncKeysAreScopedPerJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	if err := s.PutAppStateSyncKey(ctx, []byte("k1"), syncKey("segredo", 100)); err != nil {
		t.Fatalf("PutAppStateSyncKey: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	got, err := other.GetAppStateSyncKey(ctx, []byte("k1"))
	if err != nil {
		t.Fatalf("GetAppStateSyncKey outro jid: %v", err)
	}
	if got != nil {
		t.Fatalf("a app state key de um jid vazou para outro: %+v", got)
	}
}
