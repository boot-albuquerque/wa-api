package usecase_test

import (
	"context"
	"testing"

	"disparazap/internal/application/port"
	"disparazap/internal/application/usecase"
	"disparazap/internal/shared/domain"

	"go.mau.fi/whatsmeow"
)

type testCP struct{ client *whatsmeow.Client }

func (m *testCP) GetWhatsmeowClient(ctx context.Context, txtID string) (*whatsmeow.Client, error) {
	return m.client, nil
}

var _ port.ClientProvider = (*testCP)(nil)

func TestMessageValidation(t *testing.T) {
	p := &mockLog{}
	vc := &testCP{&whatsmeow.Client{}}

	type tc struct {
		name string
		fn   func() error
	}
	tests := []tc{
		{"SendAudio_NoPhone", func() error {
			_, err := usecase.NewSendAudioUseCase(vc, p).Execute(context.TODO(), "u", domain.SendAudioRequest{Audio: "data:"})
			return err
		}},
		{"SendDocument_NoPhone", func() error {
			_, err := usecase.NewSendDocumentUseCase(vc, p).Execute(context.TODO(), "u", domain.SendDocumentRequest{Document: "data:", FileName: "f.pdf"})
			return err
		}},
		{"SendEditMsg_NoPhone", func() error {
			_, err := usecase.NewSendEditMessageUseCase(vc, p).Execute(context.TODO(), "u", domain.SendEditMessageRequest{Body: "hi", ID: "msg1"})
			return err
		}},
		{"SendEditMsg_NoBody", func() error {
			_, err := usecase.NewSendEditMessageUseCase(vc, p).Execute(context.TODO(), "u", domain.SendEditMessageRequest{Phone: "5511", ID: "msg1"})
			return err
		}},
		{"SendImage_NoPhone", func() error {
			_, err := usecase.NewSendImageUseCase(vc, p).Execute(context.TODO(), "u", domain.SendImageRequest{Image: "data:image/png;base64,abc"})
			return err
		}},
		{"SendSticker_NoPhone", func() error {
			_, err := usecase.NewSendStickerUseCase(vc, p).Execute(context.TODO(), "u", domain.SendStickerRequest{Sticker: "data:"})
			return err
		}},
		{"SendVideo_NoPhone", func() error {
			_, err := usecase.NewSendVideoUseCase(vc, p).Execute(context.TODO(), "u", domain.SendVideoRequest{Video: "data:"})
			return err
		}},
		{"SendTemplate_NoPhone", func() error {
			_, err := usecase.NewSendTemplateUseCase(vc, p).Execute(context.TODO(), "u", domain.SendTemplateRequest{Content: "hi", Footer: "bye"})
			return err
		}},
		{"SendContact_NoPhone", func() error {
			_, err := usecase.NewSendContactUseCase(vc, p).Execute(context.TODO(), "u", domain.SendContactRequest{Name: "A", Vcard: "BEGIN:VCARD"})
			return err
		}},
		{"SendLocation_NoLat", func() error {
			_, err := usecase.NewSendLocationUseCase(vc, p).Execute(context.TODO(), "u", domain.SendLocationRequest{Phone: "5511", Longitude: -46})
			return err
		}},
		{"SendPoll_NoGroup", func() error {
			_, err := usecase.NewSendPollUseCase(vc, p).Execute(context.TODO(), "u", domain.SendPollRequest{Header: "V", Options: []string{"A", "B"}})
			return err
		}},
		{"SendPoll_FewOpts", func() error {
			_, err := usecase.NewSendPollUseCase(vc, p).Execute(context.TODO(), "u", domain.SendPollRequest{Group: "g", Header: "V", Options: []string{"A"}})
			return err
		}},
		{"DeleteMsg_NoPhone", func() error {
			_, err := usecase.NewDeleteMessageUseCase(vc, p).Execute(context.TODO(), "u", domain.DeleteMessageRequest{ID: "m1"})
			return err
		}},
		{"PairPhone_NoPhone", func() error {
			_, err := usecase.NewPairPhoneUseCase(vc, p).Execute(context.TODO(), "u", domain.PairPhoneRequest{})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestUsecaseNoClient(t *testing.T) {
	c := &mockCP{client: nil, err: nil}
	pl := &mockLog{}
	zl := nopLog

	type tc struct {
		name string
		fn   func() error
	}
	tests := []tc{
		{"Connect", fnErr(func() (any, error) { return usecase.NewConnectUseCase(c, pl).Execute(context.TODO(), "u", domain.ConnectRequest{}) })},
		{"Disconnect", fnErr(func() (any, error) { return usecase.NewDisconnectUseCase(c, pl).Execute(context.TODO(), "u", domain.DisconnectRequest{}) })},
		{"GetQR", fnErr(func() (any, error) { return usecase.NewGetQRUseCase(c, pl).Execute(context.TODO(), "u") })},
		{"Logout", fnErr(func() (any, error) { return usecase.NewLogoutUseCase(c, pl).Execute(context.TODO(), "u", domain.LogoutRequest{}) })},
		{"GetStatus", fnErr(func() (any, error) { return usecase.NewGetStatusUseCase(c, pl).Execute(context.TODO(), "u") })},
		{"SetStatusMsg", fnErr(func() (any, error) { return usecase.NewSetStatusMessageUseCase(c, pl).Execute(context.TODO(), "u", domain.SetStatusMessageRequest{Body: "hi"}) })},
		{"HistorySync", fnErr(func() (any, error) { return usecase.NewRequestHistorySyncUseCase(c, pl).Execute(context.TODO(), "u", domain.RequestHistorySyncRequest{}) })},
		{"DownloadImg", fnErr(func() (any, error) {
			return usecase.NewDownloadImageUseCase(c, pl).Execute(context.TODO(), "u", domain.DownloadRequest{URL: "http://x", DirectPath: "/v1", MediaKey: []byte{1}, Mimetype: "image/jpeg"})
		})},
		{"GetProfile", func() error {
			_, err := usecase.NewGetProfileUseCase(c, func(wc *whatsmeow.Client) port.ProfileDataAccess { return nil }, pl).Execute(context.TODO(), "u")
			return err
		}},
		{"ArchiveChat", func() error {
			_, err := usecase.NewArchiveChatUseCase(c, zl).Execute(context.TODO(), "u", domain.ArchiveChatRequest{Jid: "5511@s.whatsapp.net"})
			return err
		}},
		{"RejectCall", func() error {
			_, err := usecase.NewRejectCallUseCase(c, zl).Execute(context.TODO(), "u", domain.RejectCallRequest{CallFrom: "5511@s.whatsapp.net", CallID: "c1"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Errorf("%s: expected error for nil client", tt.name)
			}
		})
	}
}

func fnErr(fn func() (any, error)) func() error {
	return func() error { _, err := fn(); return err }
}
