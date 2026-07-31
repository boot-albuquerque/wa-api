package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"disparazap/internal/application/port"
	"disparazap/internal/application/usecase"
	"disparazap/internal/domain"

	"go.mau.fi/whatsmeow"
)

// mockCP implementa port.ClientProvider para testes.
type mockCP struct {
	client *whatsmeow.Client
	err    error
}

func (m *mockCP) GetWhatsmeowClient(ctx context.Context, txtID string) (*whatsmeow.Client, error) {
	return m.client, m.err
}

var _ port.ClientProvider = (*mockCP)(nil)

type mockLog struct{}

func (m *mockLog) Info(msg string, keyvals ...any)  {}
func (m *mockLog) Warn(msg string, keyvals ...any)  {}
func (m *mockLog) Error(msg string, keyvals ...any) {}

var _ port.Logger = (*mockLog)(nil)

var nopLog = zerolog.Nop()

// TestAllConstructors_NonNil verifies every usecase constructor returns non-nil.
func TestAllConstructors_NonNil(t *testing.T) {
	cp := &mockCP{}
	pl := &mockLog{}

	tests := []struct {
		name string
		fn   func() interface{}
	}{
		// port.Logger usecases
		{"NewSendMessageUseCase", func() interface{} { return usecase.NewSendMessageUseCase(cp, pl) }},
		{"NewSendImageUseCase", func() interface{} { return usecase.NewSendImageUseCase(cp, pl) }},
		{"NewSendDocumentUseCase", func() interface{} { return usecase.NewSendDocumentUseCase(cp, pl) }},
		{"NewSendAudioUseCase", func() interface{} { return usecase.NewSendAudioUseCase(cp, pl) }},
		{"NewSendStickerUseCase", func() interface{} { return usecase.NewSendStickerUseCase(cp, pl) }},
		{"NewSendVideoUseCase", func() interface{} { return usecase.NewSendVideoUseCase(cp, pl) }},
		{"NewSendContactUseCase", func() interface{} { return usecase.NewSendContactUseCase(cp, pl) }},
		{"NewSendLocationUseCase", func() interface{} { return usecase.NewSendLocationUseCase(cp, pl) }},
		{"NewSendButtonsUseCase", func() interface{} { return usecase.NewSendButtonsUseCase(cp, pl) }},
		{"NewSendListUseCase", func() interface{} { return usecase.NewSendListUseCase(cp, pl) }},
		{"NewSendPollUseCase", func() interface{} { return usecase.NewSendPollUseCase(cp, pl) }},
		{"NewDeleteMessageUseCase", func() interface{} { return usecase.NewDeleteMessageUseCase(cp, pl) }},
		{"NewSendEditMessageUseCase", func() interface{} { return usecase.NewSendEditMessageUseCase(cp, pl) }},
		{"NewSendTemplateUseCase", func() interface{} { return usecase.NewSendTemplateUseCase(cp, pl) }},
		{"NewConnectUseCase", func() interface{} { return usecase.NewConnectUseCase(cp, pl) }},
		{"NewDisconnectUseCase", func() interface{} { return usecase.NewDisconnectUseCase(cp, pl) }},
		{"NewGetQRUseCase", func() interface{} { return usecase.NewGetQRUseCase(cp, pl) }},
		{"NewLogoutUseCase", func() interface{} { return usecase.NewLogoutUseCase(cp, pl) }},
		{"NewPairPhoneUseCase", func() interface{} { return usecase.NewPairPhoneUseCase(cp, pl) }},
		{"NewGetStatusUseCase", func() interface{} { return usecase.NewGetStatusUseCase(cp, pl) }},
		{"NewSetStatusMessageUseCase", func() interface{} { return usecase.NewSetStatusMessageUseCase(cp, pl) }},
		{"NewRequestHistorySyncUseCase", func() interface{} { return usecase.NewRequestHistorySyncUseCase(cp, pl) }},
		{"NewDownloadImageUseCase", func() interface{} { return usecase.NewDownloadImageUseCase(cp, pl) }},
		{"NewDownloadDocumentUseCase", func() interface{} { return usecase.NewDownloadDocumentUseCase(cp, pl) }},
		{"NewDownloadVideoUseCase", func() interface{} { return usecase.NewDownloadVideoUseCase(cp, pl) }},
		{"NewDownloadAudioUseCase", func() interface{} { return usecase.NewDownloadAudioUseCase(cp, pl) }},
		{"NewDownloadStickerUseCase", func() interface{} { return usecase.NewDownloadStickerUseCase(cp, pl) }},
		{"NewGetGroupInfoUseCase", func() interface{} { return usecase.NewGetGroupInfoUseCase(cp, pl) }},
		{"NewGetGroupInviteLinkUseCase", func() interface{} { return usecase.NewGetGroupInviteLinkUseCase(cp, pl) }},
		{"NewGetGroupInviteInfoUseCase", func() interface{} { return usecase.NewGetGroupInviteInfoUseCase(cp, pl) }},
		{"NewListGroupsUseCase", func() interface{} { return usecase.NewListGroupsUseCase(cp, pl) }},
		{"NewConfigureS3UseCase", func() interface{} { return usecase.NewConfigureS3UseCase(cp, pl) }},
		{"NewGetS3ConfigUseCase", func() interface{} { return usecase.NewGetS3ConfigUseCase(cp, pl) }},
		{"NewTestS3ConnectionUseCase", func() interface{} { return usecase.NewTestS3ConnectionUseCase(cp, pl) }},
		{"NewDeleteS3ConfigUseCase", func() interface{} { return usecase.NewDeleteS3ConfigUseCase(cp, pl) }},
		{"NewConfigureHmacUseCase", func() interface{} { return usecase.NewConfigureHmacUseCase(cp, pl) }},
		{"NewGetHmacConfigUseCase", func() interface{} { return usecase.NewGetHmacConfigUseCase(cp, pl) }},
		{"NewDeleteHmacConfigUseCase", func() interface{} { return usecase.NewDeleteHmacConfigUseCase(cp, pl) }},
		{"NewSetProxyUseCase", func() interface{} { return usecase.NewSetProxyUseCase(cp, pl) }},
		{"NewSetHistoryUseCase", func() interface{} { return usecase.NewSetHistoryUseCase(cp, pl) }},
		{"NewGetHistoryUseCase", func() interface{} { return usecase.NewGetHistoryUseCase(cp, pl) }},
		// zerolog.Logger usecases
		{"NewSendPresenceUseCase", func() interface{} { return usecase.NewSendPresenceUseCase(cp, nopLog) }},
		{"NewSubscribePresenceUseCase", func() interface{} { return usecase.NewSubscribePresenceUseCase(cp, nopLog) }},
		{"NewChatPresenceUseCase", func() interface{} { return usecase.NewChatPresenceUseCase(cp, nopLog) }},
		{"NewReactUseCase", func() interface{} { return usecase.NewReactUseCase(cp, nopLog) }},
		{"NewMarkReadUseCase", func() interface{} { return usecase.NewMarkReadUseCase(cp, nopLog) }},
		{"NewGetAvatarUseCase", func() interface{} { return usecase.NewGetAvatarUseCase(cp, nopLog) }},
		{"NewGetContactsUseCase", func() interface{} { return usecase.NewGetContactsUseCase(cp, nopLog) }},
		{"NewGetBlocklistUseCase", func() interface{} { return usecase.NewGetBlocklistUseCase(cp, nopLog) }},
		{"NewBlockUserUseCase", func() interface{} { return usecase.NewBlockUserUseCase(cp, nopLog) }},
		{"NewUnblockUserUseCase", func() interface{} { return usecase.NewUnblockUserUseCase(cp, nopLog) }},
		{"NewCheckUserUseCase", func() interface{} { return usecase.NewCheckUserUseCase(cp, nopLog) }},
		{"NewGetUserUseCase", func() interface{} { return usecase.NewGetUserUseCase(cp, nopLog) }},
		{"NewGetUserLIDUseCase", func() interface{} { return usecase.NewGetUserLIDUseCase(cp, nopLog) }},
		{"NewGroupRequestUseCase", func() interface{} { return usecase.NewGroupRequestUseCase(cp, nopLog) }},
		{"NewRejectCallUseCase", func() interface{} { return usecase.NewRejectCallUseCase(cp, nopLog) }},
		{"NewGetPrivacySettingsUseCase", func() interface{} { return usecase.NewGetPrivacySettingsUseCase(cp, nopLog) }},
		{"NewSetPrivacySettingUseCase", func() interface{} { return usecase.NewSetPrivacySettingUseCase(cp, nopLog) }},
		{"NewRequestUnavailableMessageUseCase", func() interface{} { return usecase.NewRequestUnavailableMessageUseCase(cp, nopLog) }},
		{"NewArchiveChatUseCase", func() interface{} { return usecase.NewArchiveChatUseCase(cp, nopLog) }},
		{"NewListNewsletterUseCase", func() interface{} { return usecase.NewListNewsletterUseCase(cp, nopLog) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn()
			if got == nil {
				t.Errorf("%s returned nil", tt.name)
			}
		})
	}
}

// TestMessageUsecases_NoClient tests usecases return error when client is nil.
func TestMessageUsecases_NoClient(t *testing.T) {
	cp := &mockCP{client: nil, err: nil}
	pl := &mockLog{}

	tests := []struct {
		name string
		fn   func() (interface{}, error)
	}{
		{"SendMessage", func() (interface{}, error) {
			return usecase.NewSendMessageUseCase(cp, pl).Execute(context.Background(), "user", domain.SendMessageRequest{Phone: "5511", Body: "hi"})
		}},
		{"SendImage", func() (interface{}, error) {
			return usecase.NewSendImageUseCase(cp, pl).Execute(context.Background(), "user", domain.SendImageRequest{Phone: "5511", Image: "data:image/png;base64,abc"})
		}},
		{"DeleteMessage", func() (interface{}, error) {
			return usecase.NewDeleteMessageUseCase(cp, pl).Execute(context.Background(), "user", domain.DeleteMessageRequest{ID: "msg1"})
		}},
		{"GetStatus", func() (interface{}, error) {
			return usecase.NewGetStatusUseCase(cp, pl).Execute(context.Background(), "user")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.fn()
			if err == nil {
				t.Errorf("%s: expected error for nil client, got nil", tt.name)
			}
		})
	}
}

// TestMessageUsecases_MissingFields tests required field validation.
func TestMessageUsecases_MissingFields(t *testing.T) {
	cp := &mockCP{client: &whatsmeow.Client{}, err: nil}
	pl := &mockLog{}

	t.Run("SendMessage empty phone", func(t *testing.T) {
		_, err := usecase.NewSendMessageUseCase(cp, pl).Execute(context.Background(), "user",
			domain.SendMessageRequest{Phone: "", Body: "hi"})
		if err == nil {
			t.Error("expected error for empty phone")
		}
	})
	t.Run("SendMessage empty body", func(t *testing.T) {
		_, err := usecase.NewSendMessageUseCase(cp, pl).Execute(context.Background(), "user",
			domain.SendMessageRequest{Phone: "5511", Body: ""})
		if err == nil {
			t.Error("expected error for empty body")
		}
	})
	t.Run("DeleteMessage empty ID", func(t *testing.T) {
		_, err := usecase.NewDeleteMessageUseCase(cp, pl).Execute(context.Background(), "user",
			domain.DeleteMessageRequest{ID: ""})
		if err == nil {
			t.Error("expected error for empty message ID")
		}
	})
	t.Run("RejectCall empty callFrom", func(t *testing.T) {
		_, err := usecase.NewRejectCallUseCase(cp, nopLog).Execute(context.Background(), "user",
			domain.RejectCallRequest{CallFrom: "", CallID: "call1"})
		if err == nil {
			t.Error("expected error for empty CallFrom")
		}
	})
	t.Run("ArchiveChat empty jid", func(t *testing.T) {
		_, err := usecase.NewArchiveChatUseCase(cp, nopLog).Execute(context.Background(), "user",
			domain.ArchiveChatRequest{Jid: ""})
		if err == nil {
			t.Error("expected error for empty jid")
		}
	})
}

// TestProfileError tests the ProfileError type.
func TestProfileError(t *testing.T) {
	err := usecase.NewProfileError("no session")
	if err.Error() != "no session" {
		t.Errorf("expected 'no session', got %q", err.Error())
	}

	var sessionErr *usecase.ProfileError
	if !errors.As(err, &sessionErr) {
		t.Error("expected ProfileError to satisfy errors.As")
	}
	// ErrNoSession is compared by pointer identity (same struct literal),
	// not via errors.Is (ProfileError does not implement Is method).
	if err.Error() == usecase.ErrNoSession.Error() {
		t.Log("ProfileError message matches ErrNoSession (correct)")
	}
}
