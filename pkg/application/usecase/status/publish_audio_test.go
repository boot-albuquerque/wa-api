package status_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/status"
	"wa-api/pkg/domain"
)

func TestPublishStatusAudio_TargetIsStatusBroadcastJID(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return mp3Bytes(), "audio/mpeg", nil
		},
	}

	_, err := status.NewPublishStatusAudioUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusAudioRequest{
			Audio: "https://example.com/clip.mp3",
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if len(mm.SendAudioCalls) != 1 {
		t.Fatalf("SendAudio called %d times, want 1", len(mm.SendAudioCalls))
	}
	if got := mm.SendAudioCalls[0].Target; got != domain.StatusBroadcastJID {
		t.Errorf("target = %q, want %q", got, domain.StatusBroadcastJID)
	}
}

func TestPublishStatusAudio_MissingAudio(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	_, err := status.NewPublishStatusAudioUseCase(mm, &contractsfake.MediaFetcher{}, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusAudioRequest{})
	if err == nil {
		t.Fatal("expected error for missing Audio")
	}
	if len(mm.SendAudioCalls) != 0 {
		t.Fatalf("SendAudio called %d times with missing Audio", len(mm.SendAudioCalls))
	}
}

func TestPublishStatusAudio_SessionFailure(t *testing.T) {
	boom := errors.New("no session")
	mm := &contractsfake.MediaMessenger{
		SessionGuard: contractsfake.FailSession(boom),
	}
	_, err := status.NewPublishStatusAudioUseCase(mm, &contractsfake.MediaFetcher{}, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusAudioRequest{Audio: "https://x.com/a.mp3"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestPublishStatusAudio_SDKFailurePropagated(t *testing.T) {
	boom := errors.New("sdk refused")
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(context.Context, string, domain.JID, domain.AudioPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, boom
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return mp3Bytes(), "audio/mpeg", nil
		},
	}

	_, err := status.NewPublishStatusAudioUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusAudioRequest{Audio: "https://x.com/a.mp3"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestPublishStatusAudio_MimeTypeForwarded(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return mp3Bytes(), "audio/mpeg", nil
		},
	}

	_, err := status.NewPublishStatusAudioUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusAudioRequest{
			Audio:    "https://x.com/a.ogg",
			MimeType: "audio/ogg",
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if got := mm.SendAudioCalls[0].Payload.MimeType; got != "audio/ogg" {
		t.Errorf("mimeType = %q, want %q", got, "audio/ogg")
	}
}

func TestPublishStatusAudio_ReplyToIsNil(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
			return mp3Bytes(), "audio/mpeg", nil
		},
	}

	_, err := status.NewPublishStatusAudioUseCase(mm, mf, &contractsfake.Logger{}).
		Execute(context.Background(), "u1", domain.PublishStatusAudioRequest{
			Audio: "https://x.com/a.mp3",
		})
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if mm.SendAudioCalls[0].ReplyTo != nil {
		t.Errorf("ReplyTo = %v, want nil", mm.SendAudioCalls[0].ReplyTo)
	}
}

// mp3Bytes returns the MP3 sync word — enough for http.DetectContentType
// to return "audio/mpeg" (per the Go stdlib sniffer, 3 bytes starting with
// 0xFF 0xFB is the MPEG audio frame header).
func mp3Bytes() []byte {
	return []byte{0xFF, 0xFB, 0x90, 0x00, 0x00, 0x00, 0x00, 0x00}
}
