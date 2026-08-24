package status_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/status"
	"wa-api/pkg/domain"
)

func TestPublishStatusImage_TargetIsStatusBroadcastJID(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return pngBytes(), "image/png", nil
		},
	}

	_, err := status.NewPublishStatusImageUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusImageRequest{
			Image: "https://example.com/photo.png",
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if len(mm.SendImageCalls) != 1 {
		t.Fatalf("SendImage called %d times, want 1", len(mm.SendImageCalls))
	}
	if got := mm.SendImageCalls[0].Target; got != domain.StatusBroadcastJID {
		t.Errorf("target = %q, want %q", got, domain.StatusBroadcastJID)
	}
}

func TestPublishStatusImage_MissingImage(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	_, err := status.NewPublishStatusImageUseCase(mm, &contractsfake.MediaFetcher{}, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusImageRequest{})
	if err == nil {
		t.Fatal("expected error for missing Image")
	}
	if len(mm.SendImageCalls) != 0 {
		t.Fatalf("SendImage called %d times with missing Image", len(mm.SendImageCalls))
	}
}

func TestPublishStatusImage_SessionFailure(t *testing.T) {
	boom := errors.New("no session")
	mm := &contractsfake.MediaMessenger{
		SessionGuard: contractsfake.FailSession(boom),
	}
	_, err := status.NewPublishStatusImageUseCase(mm, &contractsfake.MediaFetcher{}, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusImageRequest{Image: "https://x.com/a.png"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestPublishStatusImage_SDKFailurePropagated(t *testing.T) {
	boom := errors.New("sdk refused")
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, boom
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return pngBytes(), "image/png", nil
		},
	}

	_, err := status.NewPublishStatusImageUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusImageRequest{Image: "https://x.com/a.png"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestPublishStatusImage_CaptionAndMimeTypeForwarded(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return pngBytes(), "image/png", nil
		},
	}

	_, err := status.NewPublishStatusImageUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusImageRequest{
			Image:    "https://x.com/a.png",
			Caption:  "hello",
			MimeType: "image/jpeg",
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	call := mm.SendImageCalls[0]
	if call.Payload.Caption != "hello" {
		t.Errorf("caption = %q, want %q", call.Payload.Caption, "hello")
	}
	if call.Payload.MimeType != "image/jpeg" {
		t.Errorf("mimeType = %q, want %q", call.Payload.MimeType, "image/jpeg")
	}
}

func TestPublishStatusImage_ReplyToAndMentionsAreNil(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return pngBytes(), "image/png", nil
		},
	}

	_, err := status.NewPublishStatusImageUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusImageRequest{
			Image: "https://x.com/a.png",
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	call := mm.SendImageCalls[0]
	if call.ReplyTo != nil {
		t.Errorf("ReplyTo = %v, want nil", call.ReplyTo)
	}
	if call.MentionedJID != nil {
		t.Errorf("MentionedJID = %v, want nil", call.MentionedJID)
	}
}

// pngBytes returns minimal bytes that http.DetectContentType identifies as
// image/png (the PNG signature).
func pngBytes() []byte {
	return []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
}
