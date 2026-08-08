package user

import (
	"context"
	"errors"
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

func TestNewUserAdapter(t *testing.T) {
	if NewUserAdapter(testkit.GetterWith(nil)) == nil {
		t.Fatal("NewUserAdapter returned nil")
	}
}

// fakeLIDStore é um LIDStore mínimo com mapa pré-carregado.
type fakeLIDStore struct {
	mapping  map[types.JID]types.JID
	errOnGet error
}

func (f *fakeLIDStore) PutManyLIDMappings(ctx context.Context, mappings []store.LIDMapping) error {
	return nil
}
func (f *fakeLIDStore) PutLIDMapping(ctx context.Context, lid, jid types.JID) error {
	if f.mapping == nil {
		f.mapping = map[types.JID]types.JID{}
	}
	f.mapping[lid] = jid
	return nil
}
func (f *fakeLIDStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	if f.errOnGet != nil {
		return types.JID{}, f.errOnGet
	}
	if pn, ok := f.mapping[lid]; ok {
		return pn, nil
	}
	return types.JID{}, nil
}
func (f *fakeLIDStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	if f.errOnGet != nil {
		return types.JID{}, f.errOnGet
	}
	for lid, mapped := range f.mapping {
		if mapped == pn {
			return lid, nil
		}
	}
	return types.JID{}, nil
}
func (f *fakeLIDStore) GetManyLIDsForPNs(ctx context.Context, pns []types.JID) (map[types.JID]types.JID, error) {
	out := map[types.JID]types.JID{}
	// map[PN]LID, como o CachedLIDMap real (`result[pn] = lid`,
	// sqlstore/lidmap.go:148). Este dublê já devolveu map[LID]PN, e foi
	// isso que escondeu a F65: ele contradizia a implementação que dubla, e
	// o adapter foi escrito contra ele.
	for _, pn := range pns {
		for lid, mapped := range f.mapping {
			if mapped == pn {
				out[pn] = lid
			}
		}
	}
	return out, nil
}

// storeWith monta um *store.Device só com os campos que vamos usar.
func storeWith(lids store.LIDStore, contacts store.ContactStore) *store.Device {
	return &store.Device{LIDs: lids, Contacts: contacts}
}

// TestUserAdapter_IsOnWhatsApp_NoSession.
func TestUserAdapter_IsOnWhatsApp_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))
	_, err := a.IsOnWhatsApp(context.Background(), "u1", []string{"5511"})
	assertAppErr(t, err, codeUserSessionUnavailable, apperr.CategoryValidation)
}

// TestUserAdapter_IsOnWhatsApp_OK mapeia resposta do SDK para domain.
func TestUserAdapter_IsOnWhatsApp_OK(t *testing.T) {
	fake := &testkit.Fake{IsOnWhatsAppFn: func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
		return []types.IsOnWhatsAppResponse{{
			Query: "5511",
			IsIn:  true,
			JID:   types.NewJID("5511", types.DefaultUserServer),
		}}, nil
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.IsOnWhatsApp(context.Background(), "u1", []string{"5511"})
	if err != nil {
		t.Fatalf("IsOnWhatsApp = %v", err)
	}
	if len(got) != 1 || !got[0].IsIn {
		t.Errorf("IsOnWhatsApp = %+v", got)
	}
	if got[0].JID != "5511@s.whatsapp.net" {
		t.Errorf("IsOnWhatsApp[0].JID = %q", got[0].JID)
	}
}

// TestUserAdapter_IsOnWhatsApp_PropagatesError.
func TestUserAdapter_IsOnWhatsApp_PropagatesError(t *testing.T) {
	sdkErr := errors.New("sdk fail")
	fake := &testkit.Fake{IsOnWhatsAppFn: func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
		return nil, sdkErr
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.IsOnWhatsApp(context.Background(), "u1", nil)
	if err == nil {
		t.Fatal("IsOnWhatsApp não propagou erro")
	}
	assertAppErr(t, err, "user_is_on_whatsapp_failed", apperr.CategoryInternal)
	if !errors.Is(err, sdkErr) {
		t.Error("apperr.New descartou o erro de origem do SDK")
	}
}

// assertAppErr afirma que err é um *apperr.AppError com code e category
// esperados, via errors.As — o wrap tem de sobreviver a quem o inspecione
// pela cadeia, não só a uma asserção de tipo direta.
func assertAppErr(t *testing.T, err error, code string, category apperr.Category) {
	t.Helper()
	var ae *apperr.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("erro = %v (%T), queria *apperr.AppError", err, err)
	}
	if ae.Code != code {
		t.Errorf("code = %q, queria %q", ae.Code, code)
	}
	if ae.Category != category {
		t.Errorf("category = %v, queria %v", ae.Category, category)
	}
}

// TestUserAdapter_GetUserInfo_JIDInvalido é o quinto site envolvido: um JID
// que não parseia não pode subir cru do adapter.
func TestUserAdapter_GetUserInfo_JIDInvalido(t *testing.T) {
	fake := &testkit.Fake{}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.GetUserInfo(context.Background(), "u1", []domain.JID{"@s.whatsapp.net"})
	assertAppErr(t, err, "user_info_targets_invalid", apperr.CategoryValidation)
}

// TestUserAdapter_GetUserInfo_NoSession.
func TestUserAdapter_GetUserInfo_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))
	_, err := a.GetUserInfo(context.Background(), "u1", nil)
	assertAppErr(t, err, codeUserSessionUnavailable, apperr.CategoryValidation)
}

// TestUserAdapter_GetUserInfo_OK.
func TestUserAdapter_GetUserInfo_OK(t *testing.T) {
	called := false
	fake := &testkit.Fake{GetUserInfoFn: func(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
		called = true
		return map[types.JID]types.UserInfo{}, nil
	}}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.GetUserInfo(context.Background(), "u1", []domain.JID{"x@s.whatsapp.net"})
	if err != nil {
		t.Fatalf("GetUserInfo = %v", err)
	}
	if !called {
		t.Fatal("GetUserInfo não invocou o SDK")
	}
}

// TestUserAdapter_GetAllContacts_NoSession.
func TestUserAdapter_GetAllContacts_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))
	_, _, err := a.GetAllContacts(context.Background(), "u1")
	assertAppErr(t, err, codeUserSessionUnavailable, apperr.CategoryValidation)
}

// TestUserAdapter_GetAllContacts_OK.
func TestUserAdapter_GetAllContacts_OK(t *testing.T) {
	cs := &testkit.ContactStore{Contacts: map[types.JID]types.ContactInfo{
		types.NewJID("5511", types.DefaultUserServer): {Found: true, PushName: "Alice"},
	}}
	fake := &testkit.Fake{StoreFn: func() *store.Device { return storeWith(nil, cs) }}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, count, err := a.GetAllContacts(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetAllContacts = %v", err)
	}
	if count != 1 {
		t.Errorf("GetAllContacts count = %d, want 1", count)
	}
	m, ok := got.(map[types.JID]types.ContactInfo)
	if !ok || len(m) != 1 {
		t.Errorf("GetAllContacts returned %+v", got)
	}
}

// TestUserAdapter_GetLIDForPN_NoSession.
func TestUserAdapter_GetLIDForPN_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))
	_, err := a.GetLIDForPN(context.Background(), "u1", "x@s.whatsapp.net")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetLIDForPN code = %q", testkit.AppErrCode(err))
	}
}

// TestUserAdapter_GetLIDForPN_NilStore_RawViaGetCachedPNForLID: o adapter
// não protege contra Store()==nil (caminho de produção nunca atingido
// porque client() garante não-nil antes); aqui validamos diretamente
// que getCachedPNForLID devolve erro em vez de panic.
func TestUserAdapter_GetLIDForPN_NilStore_RawViaGetCachedPNForLID(t *testing.T) {
	fake := &testkit.Fake{StoreFn: func() *store.Device { return nil }}
	_, err := getCachedPNForLID(context.Background(), fake, types.NewJID("x", types.HiddenUserServer))
	if err == nil {
		t.Fatal("getCachedPNForLID com nil store = nil")
	}
}

// TestUserAdapter_GetLIDForPN_OK.
func TestUserAdapter_GetLIDForPN_OK(t *testing.T) {
	// mapping: lid → pn (a chave é o LID, o valor é o PN)
	dev := storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{
		types.NewJID("lid-x", types.HiddenUserServer): types.NewJID("1234", types.DefaultUserServer),
	}}, nil)
	fake := &testkit.Fake{StoreFn: func() *store.Device { return dev }}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.GetLIDForPN(context.Background(), "u1", "1234@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetLIDForPN = %v", err)
	}
	if got != "lid-x@lid" {
		t.Errorf("GetLIDForPN = %q", got)
	}
}

// TestUserAdapter_GetLIDForPN_NotMapped devolve "", nil.
func TestUserAdapter_GetLIDForPN_NotMapped(t *testing.T) {
	dev := storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{}}, nil)
	fake := &testkit.Fake{StoreFn: func() *store.Device { return dev }}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.GetLIDForPN(context.Background(), "u1", "x@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetLIDForPN = %v", err)
	}
	if got != "" {
		t.Errorf("GetLIDForPN = %q, want empty", got)
	}
}

// TestUserAdapter_GetManyLIDsForPNs_NoSession.
func TestUserAdapter_GetManyLIDsForPNs_NoSession(t *testing.T) {
	a := NewUserAdapter(testkit.GetterWith(nil))
	_, err := a.GetManyLIDsForPNs(context.Background(), "u1", []domain.JID{"x@s.whatsapp.net"})
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("GetManyLIDsForPNs code = %q", testkit.AppErrCode(err))
	}
}

// TestUserAdapter_GetManyLIDsForPNs_OK — PORTADO FIELMENTE do caminho antigo
// (commit 3ac8073) na integração da branch.
//
// LEIA A F65 ANTES DE MEXER. Este teste passa por um motivo errado: o
// fakeLIDStore acima devolve map[LID]PN (`out[lid] = pn`, linha ~64),
// enquanto o store REAL devolve map[PN]LID
// (CachedLIDMap.GetManyLIDsForPNs, `result[pn] = lid`). O dublê contradiz a
// implementação que ele dubla, e o adapter foi escrito contra o dublê.
//
// Consequência: em produção o mapa sai invertido e normalizeToLID nunca
// casa — a normalização LID↔PN vira no-op silencioso. Corrigir só o adapter
// deixa este teste vermelho; corrigir só o fake também. Os dois lados têm
// que ser acertados na mesma mudança, e é por isso que não foi feito dentro
// do merge.
func TestUserAdapter_GetManyLIDsForPNs_OK(t *testing.T) {
	dev := storeWith(&fakeLIDStore{mapping: map[types.JID]types.JID{
		types.NewJID("lid-x", types.HiddenUserServer): types.NewJID("1234", types.DefaultUserServer),
		types.NewJID("lid-y", types.HiddenUserServer): types.NewJID("5678", types.DefaultUserServer),
	}}, nil)
	fake := &testkit.Fake{StoreFn: func() *store.Device { return dev }}
	a := NewUserAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.GetManyLIDsForPNs(context.Background(), "u1", []domain.JID{
		"1234@s.whatsapp.net", "5678@s.whatsapp.net", "9999@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("GetManyLIDsForPNs = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 entradas (9999 sem mapeamento fica de fora)", got)
	}
	if got["1234@s.whatsapp.net"] != "lid-x@lid" {
		t.Errorf("1234@s.whatsapp.net = %q, want lid-x@lid", got["1234@s.whatsapp.net"])
	}
	if got["5678@s.whatsapp.net"] != "lid-y@lid" {
		t.Errorf("5678@s.whatsapp.net = %q, want lid-y@lid", got["5678@s.whatsapp.net"])
	}
}
