// Package testutil fornece mocks compartilhados para testes unitários
// das camadas domain, application e interfaces do disparazaap-wuzapi.
//
// Use em arquivos _test.go:
//
//	import "disparazap/internal/testutil"
//	da := testutil.NewMockProfileDataAccess("John", "5511@s.whatsapp.net")
package testutil

import (
	"context"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"

	"go.mau.fi/whatsmeow"
)

// MockProfileDataAccess implementa port.ProfileDataAccess para testes.
type MockProfileDataAccess struct {
	PushNameVal  string
	JIDStr       string
	HasJID       bool
	AvatarURL    string
	AvatarID     string
	AvatarErr    error
	FullName     string
	BusinessName string
	ContactErr   error
}

// NewMockProfileDataAccess cria um mock com pushName e JID preenchidos.
func NewMockProfileDataAccess(pushName, jid string) *MockProfileDataAccess {
	return &MockProfileDataAccess{
		PushNameVal: pushName,
		JIDStr:      jid,
		HasJID:      jid != "",
	}
}

var _ port.ProfileDataAccess = (*MockProfileDataAccess)(nil)

func (m *MockProfileDataAccess) PushName() string { return m.PushNameVal }

func (m *MockProfileDataAccess) OwnJID() (domain.JID, bool) {
	if !m.HasJID {
		return "", false
	}
	return domain.JID(m.JIDStr), true
}

func (m *MockProfileDataAccess) ProfilePictureURL(ctx context.Context, jid domain.JID) (string, string, error) {
	return m.AvatarURL, m.AvatarID, m.AvatarErr
}

func (m *MockProfileDataAccess) ContactInfo(ctx context.Context, jid domain.JID) (string, string, error) {
	return m.FullName, m.BusinessName, m.ContactErr
}

// MockClientProvider implementa port.ClientProvider para testes.
type MockClientProvider struct {
	Client *whatsmeow.Client
	Err    error
}

var _ port.ClientProvider = (*MockClientProvider)(nil)

func (m *MockClientProvider) GetWhatsmeowClient(ctx context.Context, txtID string) (*whatsmeow.Client, error) {
	return m.Client, m.Err
}

// MockLogger implementa port.Logger para testes.
// Captura msg E keyvals para validação de PII.
type MockLogger struct {
	Entries []LogEntry
}

// LogEntry representa uma chamada de log com nível, mensagem e keyvals.
type LogEntry struct {
	Level   string
	Msg     string
	Keyvals []any
}

var _ port.Logger = (*MockLogger)(nil)

func (m *MockLogger) Info(msg string, keyvals ...any) {
	m.Entries = append(m.Entries, LogEntry{Level: "info", Msg: msg, Keyvals: keyvals})
}

func (m *MockLogger) Warn(msg string, keyvals ...any) {
	m.Entries = append(m.Entries, LogEntry{Level: "warn", Msg: msg, Keyvals: keyvals})
}

func (m *MockLogger) Error(msg string, keyvals ...any) {
	m.Entries = append(m.Entries, LogEntry{Level: "error", Msg: msg, Keyvals: keyvals})
}

// HasPII verifica se as entradas de log contêm padrões de PII.
func (m *MockLogger) HasPII(patterns ...string) bool {
	for _, entry := range m.Entries {
		for _, p := range patterns {
			if stringContains(entry.Msg, p) {
				return true
			}
			for _, kv := range entry.Keyvals {
				if s, ok := kv.(string); ok && stringContains(s, p) {
					return true
				}
			}
		}
	}
	return false
}

func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && containsAt(s, substr)
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
