package whatsmeow

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"

	"wa-api/internal/waclient/store"
	"wa-api/internal/waclient/types"
)

func TestNewUserAdapter(t *testing.T) {
	if NewUserAdapter(getterWith(nil)) == nil {
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
	for _, pn := range pns {
		for lid, mapped := range f.mapping {
			if mapped == pn {
				out[lid] = pn
			}
		}
	}
	return out, nil
}

// fakeContactStore devolve um mapa pré-carregado de contatos.
type fakeContactStore struct {
	contacts map[types.JID]types.ContactInfo
	errOnGet error
}

func (f *fakeContactStore) PutPushName(ctx context.Context, user types.JID, pushName string) (bool, string, error) {
	return true, "", nil
}
func (f *fakeContactStore) PutBusinessName(ctx context.Context, user types.JID, businessName string) (bool, string, error) {
	return true, "", nil
}
func (f *fakeContactStore) PutContactName(ctx context.Context, user types.JID, fullName, firstName string) error {
	return nil
}
func (f *fakeContactStore) PutAllContactNames(ctx context.Context, contacts []store.ContactEntry) error {
	return nil
}
func (f *fakeContactStore) PutManyRedactedPhones(ctx context.Context, entries []store.RedactedPhoneEntry) error {
	return nil
}
func (f *fakeContactStore) GetContact(ctx context.Context, user types.JID) (types.ContactInfo, error) {
	if c, ok := f.contacts[user]; ok {
		return c, nil
	}
	return types.ContactInfo{}, nil
}
func (f *fakeContactStore) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	if f.errOnGet != nil {
		return nil, f.errOnGet
	}
	return f.contacts, nil
}

// storeWith monta um *store.Device só com os campos que vamos usar.
func storeWith(lids store.LIDStore, contacts store.ContactStore) *store.Device {
	return &store.Device{LIDs: lids, Contacts: contacts}
}

// TestUserAdapter_IsOnWhatsApp_NoSession.
func TestUserAdapter_IsOnWhatsApp_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, err := a.IsOnWhatsApp(context.Background(), "u1", []string{"5511"})
	assertAppErr(t, err, codeUserSessionUnavailable, apperr.CategoryValidation)
}

// TestUserAdapter_IsOnWhatsApp_OK mapeia resposta do SDK para domain.
func TestUserAdapter_IsOnWhatsApp_OK(t *testing.T) {
	fake := &fakeWAClient{IsOnWhatsAppFn: func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
		return []types.IsOnWhatsAppResponse{{
			Query: "5511",
			IsIn:  true,
			JID:   types.NewJID("5511", types.DefaultUserServer),
		}}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
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
	fake := &fakeWAClient{IsOnWhatsAppFn: func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
		return nil, sdkErr
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
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
	fake := &fakeWAClient{}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.GetUserInfo(context.Background(), "u1", []domain.JID{"@s.whatsapp.net"})
	assertAppErr(t, err, "user_info_failed", apperr.CategoryInternal)
}

// TestUserAdapter_GetUserInfo_NoSession.
func TestUserAdapter_GetUserInfo_NoSession(t *testing.T) {
	a := NewUserAdapter(getterWith(nil))
	_, err := a.GetUserInfo(context.Background(), "u1", nil)
	assertAppErr(t, err, codeUserSessionUnavailable, apperr.CategoryValidation)
}

// TestUserAdapter_GetUserInfo_OK.
func TestUserAdapter_GetUserInfo_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{GetUserInfoFn: func(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
		called = true
		return map[types.JID]types.UserInfo{}, nil
	}}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
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
	a := NewUserAdapter(getterWith(nil))
	_, _, err := a.GetAllContacts(context.Background(), "u1")
	assertAppErr(t, err, codeUserSessionUnavailable, apperr.CategoryValidation)
}

// TestUserAdapter_GetAllContacts_OK.
func TestUserAdapter_GetAllContacts_OK(t *testing.T) {
	cs := &fakeContactStore{contacts: map[types.JID]types.ContactInfo{
		types.NewJID("5511", types.DefaultUserServer): {Found: true, PushName: "Alice"},
	}}
	fake := &fakeWAClient{StoreFn: func() *store.Device { return storeWith(nil, cs) }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
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
	a := NewUserAdapter(getterWith(nil))
	_, err := a.GetLIDForPN(context.Background(), "u1", "x@s.whatsapp.net")
	if appErrCode(err) != "no_session" {
		t.Errorf("GetLIDForPN code = %q", appErrCode(err))
	}
}

// TestUserAdapter_GetLIDForPN_NilStore_RawViaGetCachedPNForLID: o adapter
// não protege contra Store()==nil (caminho de produção nunca atingido
// porque client() garante não-nil antes); aqui validamos diretamente
// que getCachedPNForLID devolve erro em vez de panic.
func TestUserAdapter_GetLIDForPN_NilStore_RawViaGetCachedPNForLID(t *testing.T) {
	fake := &fakeWAClient{StoreFn: func() *store.Device { return nil }}
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
	fake := &fakeWAClient{StoreFn: func() *store.Device { return dev }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
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
	fake := &fakeWAClient{StoreFn: func() *store.Device { return dev }}
	a := NewUserAdapter(getterWith(map[string]waClient{"u1": fake}))
	got, err := a.GetLIDForPN(context.Background(), "u1", "x@s.whatsapp.net")
	if err != nil {
		t.Fatalf("GetLIDForPN = %v", err)
	}
	if got != "" {
		t.Errorf("GetLIDForPN = %q, want empty", got)
	}
}
