package message_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// mockComposer é a fake de appport.MessageComposer. Antes da ADR-001 ela
// tinha que produzir um cliente concreto do SDK para que o use case pudesse
// chamar GenerateMessageID() nele; agora declara apenas as duas capacidades
// que o use case de fato exerce.
type mockComposer struct {
	err error
}

func (m *mockComposer) EnsureSession(context.Context, string) error {
	return m.err
}

func (m *mockComposer) NewMessageID(context.Context, string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return "generated-message-id", nil
}

type sendMockLogger struct {
	entries []string
}

func (m *sendMockLogger) Info(_ context.Context, msg string, keyvals ...any) {
	m.entries = append(m.entries, msg)
}
func (m *sendMockLogger) Warn(_ context.Context, msg string, keyvals ...any) {
	m.entries = append(m.entries, msg)
}
func (m *sendMockLogger) Error(_ context.Context, msg string, keyvals ...any) {
	m.entries = append(m.entries, msg)
}

func TestSendMessageUseCase_Execute_NoClient(t *testing.T) {
	provider := &mockComposer{err: errors.New("no session")}
	logger := &sendMockLogger{}
	uc := message.NewSendMessageUseCase(provider, logger)

	_, err := uc.Execute(context.Background(), "test-user", domain.SendMessageRequest{
		Phone: "5511987654321",
		Body:  "Hello",
	})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestSendMessageUseCase_Execute_ProviderError(t *testing.T) {
	provider := &mockComposer{err: errors.New("db error")}
	logger := &sendMockLogger{}
	uc := message.NewSendMessageUseCase(provider, logger)

	_, err := uc.Execute(context.Background(), "test-user", domain.SendMessageRequest{
		Phone: "5511987654321",
		Body:  "Hello",
	})
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

func TestSendMessageUseCase_Execute_Success(t *testing.T) {
	provider := &mockComposer{}
	logger := &sendMockLogger{}
	uc := message.NewSendMessageUseCase(provider, logger)

	result, err := uc.Execute(context.Background(), "test-user", domain.SendMessageRequest{
		Phone: "5511987654321",
		Body:  "Hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MessageID == "" {
		t.Error("expected non-empty MessageID")
	}
	if result.Status != "validated" {
		t.Errorf("expected status 'validated', got %q", result.Status)
	}
}

func TestSendMessageUseCase_Execute_EmptyPhone(t *testing.T) {
	provider := &mockComposer{}
	logger := &sendMockLogger{}
	uc := message.NewSendMessageUseCase(provider, logger)

	_, err := uc.Execute(context.Background(), "test-user", domain.SendMessageRequest{
		Phone: "",
		Body:  "Hello",
	})
	if err == nil {
		t.Fatal("expected error for empty phone")
	}
}

func TestSendMessageUseCase_Execute_EmptyBody(t *testing.T) {
	provider := &mockComposer{}
	logger := &sendMockLogger{}
	uc := message.NewSendMessageUseCase(provider, logger)

	_, err := uc.Execute(context.Background(), "test-user", domain.SendMessageRequest{
		Phone: "5511987654321",
		Body:  "",
	})
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestSendImageUseCase_Execute_Success(t *testing.T) {
	provider := &mockComposer{}
	logger := &sendMockLogger{}
	uc := message.NewSendImageUseCase(provider, logger)

	result, err := uc.Execute(context.Background(), "test-user", domain.SendImageRequest{
		Phone: "5511987654321",
		Image: "data:image/png;base64,abc123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "validated" {
		t.Errorf("expected status 'validated', got %q", result.Status)
	}
}

func TestSendImageUseCase_Execute_NoClient(t *testing.T) {
	provider := &mockComposer{err: errors.New("no session")}
	logger := &sendMockLogger{}
	uc := message.NewSendImageUseCase(provider, logger)

	_, err := uc.Execute(context.Background(), "test-user", domain.SendImageRequest{
		Phone: "5511987654321",
		Image: "data:image/png;base64,abc123",
	})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}
