package status_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/status"
	"wa-api/pkg/domain"
)

func TestPublishStatusVideo_TargetIsStatusBroadcastJID(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return mp4Bytes(), "video/mp4", nil
		},
	}

	_, err := status.NewPublishStatusVideoUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusVideoRequest{
			Video: "https://example.com/clip.mp4",
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if len(mm.SendVideoCalls) != 1 {
		t.Fatalf("SendVideo called %d times, want 1", len(mm.SendVideoCalls))
	}
	if got := mm.SendVideoCalls[0].Target; got != domain.StatusBroadcastJID {
		t.Errorf("target = %q, want %q", got, domain.StatusBroadcastJID)
	}
}

func TestPublishStatusVideo_MissingVideo(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	_, err := status.NewPublishStatusVideoUseCase(mm, &contractsfake.MediaFetcher{}, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusVideoRequest{})
	if err == nil {
		t.Fatal("expected error for missing Video")
	}
	if len(mm.SendVideoCalls) != 0 {
		t.Fatalf("SendVideo called %d times with missing Video", len(mm.SendVideoCalls))
	}
}

func TestPublishStatusVideo_SessionFailure(t *testing.T) {
	boom := errors.New("no session")
	mm := &contractsfake.MediaMessenger{
		SessionGuard: contractsfake.FailSession(boom),
	}
	_, err := status.NewPublishStatusVideoUseCase(mm, &contractsfake.MediaFetcher{}, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusVideoRequest{Video: "https://x.com/a.mp4"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestPublishStatusVideo_SDKFailurePropagated(t *testing.T) {
	boom := errors.New("sdk refused")
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, boom
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return mp4Bytes(), "video/mp4", nil
		},
	}

	_, err := status.NewPublishStatusVideoUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusVideoRequest{Video: "https://x.com/a.mp4"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestPublishStatusVideo_CaptionForwarded(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return mp4Bytes(), "video/mp4", nil
		},
	}

	_, err := status.NewPublishStatusVideoUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusVideoRequest{
			Video:   "https://x.com/a.mp4",
			Caption: "status caption",
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if got := mm.SendVideoCalls[0].Payload.Caption; got != "status caption" {
		t.Errorf("caption = %q, want %q", got, "status caption")
	}
}

func TestPublishStatusVideo_ReplyToAndMentionsAreNil(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return mp4Bytes(), "video/mp4", nil
		},
	}

	_, err := status.NewPublishStatusVideoUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusVideoRequest{
			Video: "https://x.com/a.mp4",
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	call := mm.SendVideoCalls[0]
	if call.ReplyTo != nil {
		t.Errorf("ReplyTo = %v, want nil", call.ReplyTo)
	}
	if call.MentionedJID != nil {
		t.Errorf("MentionedJID = %v, want nil", call.MentionedJID)
	}
}

// mp4Bytes returns minimal bytes recognizable as a non-empty payload.
// http.DetectContentType would return "application/octet-stream" for these
// bytes, but the video use case resolves MIME via resolveMimeType which
// falls back to sniffing the data — the important property is non-empty.
func mp4Bytes() []byte {
	return []byte{0x00, 0x00, 0x00, 0x1C, 0x66, 0x74, 0x79, 0x70}
}
