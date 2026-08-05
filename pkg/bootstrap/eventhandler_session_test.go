package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// fakeContactStore devolve um mapa pré-carregado de contatos, ou um erro
// forçado. Só os métodos que o handler exercita (GetAllContacts) recebem
// comportamento interessante; o resto satisfaz store.ContactStore com um
// no-op mínimo.
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

// clientWithContacts monta um *whatsmeow.Client real (sem conexão de rede)
// cujo Store.Contacts é o fake acima. NewClient só popula campos internos a
// partir do deviceStore — não conecta a nada — então isso é seguro em teste
// unitário, no mesmo espírito de pkg/infra/whatsmeow/user_adapters_test.go.
func clientWithContacts(cs store.ContactStore) *whatsmeow.Client {
	return whatsmeow.NewClient(&store.Device{Contacts: cs}, nil)
}

// captureLog troca o logger global por um buffer durante fn e devolve o que
// foi escrito, revertendo o logger original ao final (padrão usado em
// TestProcessMediaGuardsNilCtx).
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = orig })
	fn()
	return buf.String()
}

// TestHandleAppStateSyncComplete_ContactRoster_LogsCount cobre o caso feliz
// do patch que carrega a agenda de contatos: o handler deve logar em Info
// com patch, version e contact_count — sem nunca logar o conteúdo do mapa.
func TestHandleAppStateSyncComplete_ContactRoster_LogsCount(t *testing.T) {
	cs := &fakeContactStore{contacts: map[types.JID]types.ContactInfo{
		types.NewJID("5511900000001", types.DefaultUserServer): {Found: true, PushName: "Alice"},
		types.NewJID("5511900000002", types.DefaultUserServer): {Found: true, PushName: "Bob"},
	}}
	mycli := &MyClient{WAClient: clientWithContacts(cs), UserID: "user-42"}
	evt := &events.AppStateSyncComplete{Name: appstate.WAPatchCriticalUnblockLow, Version: 7}

	out := captureLog(t, func() {
		mycli.handleAppStateSyncComplete(evt, &eventState{})
	})

	if !strings.Contains(out, `"level":"info"`) {
		t.Fatalf("expected info-level log, got: %q", out)
	}
	if !strings.Contains(out, `"patch":"critical_unblock_low"`) {
		t.Fatalf("expected patch field, got: %q", out)
	}
	if !strings.Contains(out, `"version":7`) {
		t.Fatalf("expected version field = 7, got: %q", out)
	}
	if !strings.Contains(out, `"contact_count":2`) {
		t.Fatalf("expected contact_count field = 2, got: %q", out)
	}
	if !strings.Contains(out, `"userid":"user-42"`) {
		t.Fatalf("expected userid field, got: %q", out)
	}
	// Nunca deve vazar JID, telefone ou nome de contato no log.
	if strings.Contains(out, "5511900000001") || strings.Contains(out, "Alice") || strings.Contains(out, "Bob") {
		t.Fatalf("log leaked contact PII: %q", out)
	}
}

// TestHandleAppStateSyncComplete_CriticalBlock_PresenceUnaffected é a
// regressão: o ramo pré-existente (WAPatchCriticalBlock com PushName
// preenchido) precisa continuar marcando presença disponível exatamente como
// antes, sem interferência do novo ramo.
func TestHandleAppStateSyncComplete_CriticalBlock_PresenceUnaffected(t *testing.T) {
	deviceStore := &store.Device{Contacts: &fakeContactStore{}}
	deviceStore.PushName = "Alice"
	client := whatsmeow.NewClient(deviceStore, nil)
	mycli := &MyClient{WAClient: client, UserID: "user-42"}
	evt := &events.AppStateSyncComplete{Name: appstate.WAPatchCriticalBlock}

	out := captureLog(t, func() {
		mycli.handleAppStateSyncComplete(evt, &eventState{})
	})

	// SendPresence falha sem uma conexão real (client não conectado), então o
	// ramo original loga WARN "Failed to send available presence" — o mesmo
	// comportamento que tinha antes desta mudança. O que importa aqui é que
	// esse ramo ainda executa (a condição len(PushName) > 0 && Block ainda
	// dispara) e que o novo ramo (contact_count) não interfere nele.
	if !strings.Contains(out, "available presence") {
		t.Fatalf("expected presence branch to still execute, got: %q", out)
	}
	if strings.Contains(out, "contact_count") {
		t.Fatalf("contact roster branch must not run for WAPatchCriticalBlock, got: %q", out)
	}
}

// TestHandleAppStateSyncComplete_OtherPatch_NoOp garante que um patch que não
// seja nem WAPatchCriticalBlock nem WAPatchCriticalUnblockLow não aciona
// nenhum dos dois ramos (nem log, nem chamada de presença).
func TestHandleAppStateSyncComplete_OtherPatch_NoOp(t *testing.T) {
	deviceStore := &store.Device{Contacts: &fakeContactStore{}}
	deviceStore.PushName = "Alice"
	client := whatsmeow.NewClient(deviceStore, nil)
	mycli := &MyClient{WAClient: client, UserID: "user-42"}
	evt := &events.AppStateSyncComplete{Name: appstate.WAPatchRegularLow}

	out := captureLog(t, func() {
		mycli.handleAppStateSyncComplete(evt, &eventState{})
	})

	if out != "" {
		t.Fatalf("expected no log output for unrelated patch, got: %q", out)
	}
}

// TestHandleAppStateSyncComplete_ContactRoster_GetAllContactsError cobre o
// caminho de erro: GetAllContacts falhando deve produzir um WARN com userid
// e patch, sem crash e sem logar dados de contato.
func TestHandleAppStateSyncComplete_ContactRoster_GetAllContactsError(t *testing.T) {
	boom := errors.New("boom")
	cs := &fakeContactStore{errOnGet: boom}
	mycli := &MyClient{WAClient: clientWithContacts(cs), UserID: "user-42"}
	evt := &events.AppStateSyncComplete{Name: appstate.WAPatchCriticalUnblockLow, Version: 3}

	out := captureLog(t, func() {
		mycli.handleAppStateSyncComplete(evt, &eventState{})
	})

	if !strings.Contains(out, `"level":"warn"`) {
		t.Fatalf("expected warn-level log, got: %q", out)
	}
	if !strings.Contains(out, `"userid":"user-42"`) {
		t.Fatalf("expected userid field, got: %q", out)
	}
	if !strings.Contains(out, `"patch":"critical_unblock_low"`) {
		t.Fatalf("expected patch field, got: %q", out)
	}
	if !strings.Contains(out, "boom") {
		t.Fatalf("expected underlying error in log, got: %q", out)
	}
	if strings.Contains(out, "contact_count") {
		t.Fatalf("error path must not log contact_count, got: %q", out)
	}
}
