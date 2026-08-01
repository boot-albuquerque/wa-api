package profile_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/profile"
	"wa-api/pkg/domain"
)

// mockProfileProvider é a fake de appport.ProfileAccessProvider. Antes da
// ADR-001 ela devolvia um cliente concreto do SDK, e o teste ainda precisava
// de uma fábrica separada para transformá-lo num ProfileDataAccess; agora
// entrega o ProfileDataAccess direto.
type mockProfileProvider struct {
	da  appport.ProfileDataAccess
	err error
}

func (m *mockProfileProvider) EnsureSession(context.Context, string) error {
	return m.err
}

func (m *mockProfileProvider) ProfileAccess(context.Context, string) (appport.ProfileDataAccess, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.da, nil
}

type mockLogger struct {
	infos  []string
	warns  []string
	errors []string
}

func (m *mockLogger) Info(_ context.Context, msg string, keyvals ...any) {
	m.infos = append(m.infos, msg)
}
func (m *mockLogger) Warn(_ context.Context, msg string, keyvals ...any) {
	m.warns = append(m.warns, msg)
}
func (m *mockLogger) Error(_ context.Context, msg string, keyvals ...any) {
	m.errors = append(m.errors, msg)
}

// mockDataAccess implementa appport.ProfileDataAccess para testes.
type mockDataAccess struct {
	pushName               string
	jidStr                 string
	hasJID                 bool
	avatarURL, avatarID    string
	avatarErr              error
	fullName, businessName string
	contactErr             error
}

func (m *mockDataAccess) PushName() string { return m.pushName }
func (m *mockDataAccess) OwnJID() (domain.JID, bool) {
	if !m.hasJID {
		return "", false
	}
	return domain.JID(m.jidStr), true
}
func (m *mockDataAccess) ProfilePictureURL(ctx context.Context, jid domain.JID) (string, string, error) {
	return m.avatarURL, m.avatarID, m.avatarErr
}
func (m *mockDataAccess) ContactInfo(ctx context.Context, jid domain.JID) (string, string, error) {
	return m.fullName, m.businessName, m.contactErr
}

func TestGetProfileExecute_NoClient(t *testing.T) {
	provider := &mockProfileProvider{err: errors.New("no session")}
	logger := &mockLogger{}
	uc := profile.NewGetProfileUseCase(provider, logger)

	_, err := uc.Execute(context.Background(), "test-user")
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !strings.Contains(err.Error(), "no session") {
		t.Errorf("expected 'no session', got %q", err.Error())
	}
}

func TestGetProfileExecute_ProviderError(t *testing.T) {
	provider := &mockProfileProvider{err: errors.New("connection refused")}
	logger := &mockLogger{}
	uc := profile.NewGetProfileUseCase(provider, logger)

	_, err := uc.Execute(context.Background(), "test-user")
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

func TestGetProfileExecute_Success(t *testing.T) {
	da := &mockDataAccess{
		pushName:     "John",
		jidStr:       "5511987654321@s.whatsapp.net",
		hasJID:       true,
		avatarURL:    "https://img.example.com/1.jpg",
		avatarID:     "img-1",
		fullName:     "John Full",
		businessName: "Biz",
	}
	provider := &mockProfileProvider{da: da}
	logger := &mockLogger{}
	uc := profile.NewGetProfileUseCase(provider, logger)

	result, err := uc.Execute(context.Background(), "test-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// O corpo de GET /session/profile e' esta string, escrita direto no
	// ResponseWriter por ProfileHandler. Entao as chaves abaixo SAO o
	// contrato publico da rota, e nao um detalhe de serializacao.
	//
	// Estas assercoes vieram de pkg/domain/profile_test.go, deletado nesta
	// fase: la elas testavam as tags de domain.Profile, um tipo que NENHUM
	// codigo de producao constroi. Apontadas para ProfileResult, que e' o que
	// de fato vai para o cliente, elas passam a valer alguma coisa. A
	// assercao que estava aqui antes (len(result) >= 10) nao distinguia o
	// perfil correto de "{\"a\":1234}".
	var got map[string]string
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("resultado nao e' um objeto JSON de strings: %v (%q)", err, result)
	}

	want := map[string]string{
		"pushname":      "John",
		"jid":           "5511987654321@s.whatsapp.net",
		"avatar_url":    "https://img.example.com/1.jpg",
		"avatar_id":     "img-1",
		"full_name":     "John Full",
		"business_name": "Biz",
	}
	for key, wantVal := range want {
		gotVal, present := got[key]
		if !present {
			t.Errorf("chave %q sumiu do corpo da rota (contrato publico): %s", key, result)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("%s: got %q, want %q", key, gotVal, wantVal)
		}
	}
	for key := range got {
		if _, esperada := want[key]; !esperada {
			t.Errorf("chave inesperada %q no corpo da rota: %s", key, result)
		}
	}
}

// panicDataAccess panics inside PushName to exercise buildProfile's recover
// path — before Fase 4b this panic was swallowed with zero logging
// (erros/F7).
type panicDataAccess struct{ mockDataAccess }

func (m *panicDataAccess) PushName() string { panic("boom") }

func TestGetProfileExecute_RecoversFromPanicAndLogs(t *testing.T) {
	da := &panicDataAccess{}
	provider := &mockProfileProvider{da: da}
	logger := &mockLogger{}
	uc := profile.NewGetProfileUseCase(provider, logger)

	result, err := uc.Execute(context.Background(), "test-user")
	if err != nil {
		t.Fatalf("panic inside buildProfile should be recovered, not surfaced as an error: %v", err)
	}
	if result == "" {
		t.Fatalf("expected a JSON result even with a partially-built profile, got %q", result)
	}
	if len(logger.errors) != 1 {
		t.Fatalf("expected exactly 1 error log for the recovered panic, got %d: %v", len(logger.errors), logger.errors)
	}
	if !strings.Contains(logger.errors[0], "panic") {
		t.Errorf("error log %q should mention the panic", logger.errors[0])
	}
}

func TestGetProfileNoPIIInLogs(t *testing.T) {
	da := &mockDataAccess{pushName: "John"}
	logger := &mockLogger{}
	provider := &mockProfileProvider{da: da}
	uc := profile.NewGetProfileUseCase(provider, logger)
	_, _ = uc.Execute(context.Background(), "test-user")
	for _, log := range append(append(logger.infos, logger.warns...), logger.errors...) {
		if strings.Contains(log, "5511") || strings.Contains(log, "John") {
			t.Errorf("logger contains PII: %s", log)
		}
	}
}
