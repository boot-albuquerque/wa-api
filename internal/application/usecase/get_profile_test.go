package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	appport "disparazap/internal/application/port"
	"disparazap/internal/application/usecase"
	"disparazap/internal/domain"

	"go.mau.fi/whatsmeow"
)

type mockClientProvider struct {
	client *whatsmeow.Client
	err    error
}

func (m *mockClientProvider) GetWhatsmeowClient(ctx context.Context, txtID string) (*whatsmeow.Client, error) {
	return m.client, m.err
}

type mockLogger struct {
	infos  []string
	warns  []string
	errors []string
}

func (m *mockLogger) Info(msg string, keyvals ...any)  { m.infos = append(m.infos, msg) }
func (m *mockLogger) Warn(msg string, keyvals ...any)  { m.warns = append(m.warns, msg) }
func (m *mockLogger) Error(msg string, keyvals ...any) { m.errors = append(m.errors, msg) }

// mockDataAccess implementa port.ProfileDataAccess para testes.
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

func mockDataAccessFactory(da *mockDataAccess) func(*whatsmeow.Client) appport.ProfileDataAccess {
	return func(c *whatsmeow.Client) appport.ProfileDataAccess { return da }
}

func TestGetProfileExecute_NoClient(t *testing.T) {
	da := &mockDataAccess{}
	provider := &mockClientProvider{client: nil, err: nil}
	logger := &mockLogger{}
	uc := usecase.NewGetProfileUseCase(provider, mockDataAccessFactory(da), logger)

	_, err := uc.Execute(context.Background(), "test-user")
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !strings.Contains(err.Error(), "no session") {
		t.Errorf("expected 'no session', got %q", err.Error())
	}
}

func TestGetProfileExecute_ProviderError(t *testing.T) {
	da := &mockDataAccess{}
	provider := &mockClientProvider{client: nil, err: errors.New("connection refused")}
	logger := &mockLogger{}
	uc := usecase.NewGetProfileUseCase(provider, mockDataAccessFactory(da), logger)

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
	provider := &mockClientProvider{client: &whatsmeow.Client{}}
	logger := &mockLogger{}
	uc := usecase.NewGetProfileUseCase(provider, mockDataAccessFactory(da), logger)

	result, err := uc.Execute(context.Background(), "test-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" || len(result) < 10 {
		t.Errorf("expected JSON result, got %q", result)
	}
}

func TestGetProfileNoPIIInLogs(t *testing.T) {
	da := &mockDataAccess{pushName: "John"}
	logger := &mockLogger{}
	provider := &mockClientProvider{client: &whatsmeow.Client{}}
	uc := usecase.NewGetProfileUseCase(provider, mockDataAccessFactory(da), logger)
	_, _ = uc.Execute(context.Background(), "test-user")
	for _, log := range append(append(logger.infos, logger.warns...), logger.errors...) {
		if strings.Contains(log, "5511") || strings.Contains(log, "John") {
			t.Errorf("logger contains PII: %s", log)
		}
	}
}
