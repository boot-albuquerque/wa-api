package message_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow"
)

type mockSendClientProvider struct {
	client *whatsmeow.Client
	err    error
}

func (m *mockSendClientProvider) GetWhatsmeowClient(ctx context.Context, txtID string) (*whatsmeow.Client, error) {
	return m.client, m.err
}

type sendMockLogger struct {
	entries []string
}

func (m *sendMockLogger) Info(msg string, keyvals ...any)  { m.entries = append(m.entries, msg) }
func (m *sendMockLogger) Warn(msg string, keyvals ...any)  { m.entries = append(m.entries, msg) }
func (m *sendMockLogger) Error(msg string, keyvals ...any) { m.entries = append(m.entries, msg) }

func TestSendMessageUseCase_Execute_NoClient(t *testing.T) {
	provider := &mockSendClientProvider{client: nil, err: nil}
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
	provider := &mockSendClientProvider{client: nil, err: errors.New("db error")}
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
	provider := &mockSendClientProvider{client: &whatsmeow.Client{}}
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
	provider := &mockSendClientProvider{client: &whatsmeow.Client{}}
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
	provider := &mockSendClientProvider{client: &whatsmeow.Client{}}
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
	provider := &mockSendClientProvider{client: &whatsmeow.Client{}}
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
	provider := &mockSendClientProvider{client: nil, err: nil}
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
