package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/status"
	"wa-api/pkg/domain"
)

// --- helpers -----------------------------------------------------------------

func statusImageRouter(mm *contractsfake.MediaMessenger, mf *contractsfake.MediaFetcher) http.Handler {
	uc := status.NewPublishStatusImageUseCase(mm, mf, silentLogger{})
	h := NewPublishStatusImageHandler(uc)
	r := mux.NewRouter()
	r.Handle("/status/set/image", h).Methods(http.MethodPost)
	return r
}

func statusVideoRouter(mm *contractsfake.MediaMessenger, mf *contractsfake.MediaFetcher) http.Handler {
	uc := status.NewPublishStatusVideoUseCase(mm, mf, silentLogger{})
	h := NewPublishStatusVideoHandler(uc)
	r := mux.NewRouter()
	r.Handle("/status/set/video", h).Methods(http.MethodPost)
	return r
}

func statusAudioRouter(mm *contractsfake.MediaMessenger, mf *contractsfake.MediaFetcher) http.Handler {
	uc := status.NewPublishStatusAudioUseCase(mm, mf, silentLogger{})
	h := NewPublishStatusAudioHandler(uc)
	r := mux.NewRouter()
	r.Handle("/status/set/audio", h).Methods(http.MethodPost)
	return r
}

func statusMediaServe(t *testing.T, router http.Handler, path, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if mut != nil {
		req = mut(req)
	}
	router.ServeHTTP(rec, req)
	return rec
}

var statusMediaPNGFetcher = &contractsfake.MediaFetcher{
	FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
		return []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, "image/png", nil
	},
}

var statusMediaVideoFetcher = &contractsfake.MediaFetcher{
	FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
		return []byte{0x00, 0x00, 0x00, 0x1C, 0x66, 0x74, 0x79, 0x70}, "video/mp4", nil
	},
}

var statusMediaAudioFetcher = &contractsfake.MediaFetcher{
	FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
		return []byte{0xFF, 0xFB, 0x90, 0x00, 0x00, 0x00, 0x00, 0x00}, "audio/mpeg", nil
	},
}

type statusMediaResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// --- /status/set/image -------------------------------------------------------

func TestPublishStatusImage_SuccessViaRegisteredRoute(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	router := statusImageRouter(mm, statusMediaPNGFetcher)

	body := `{"Image":"https://example.com/photo.png","Caption":"hello"}`
	rec := statusMediaServe(t, router, "/status/set/image", body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("success = false, want true")
	}

	var result statusMediaResultBody
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if result.Status != "sent" {
		t.Errorf("Status = %q, want %q", result.Status, "sent")
	}
	if result.MessageID == "" {
		t.Error("MessageID is empty")
	}

	if len(mm.SendImageCalls) != 1 {
		t.Fatalf("SendImage called %d times, want 1", len(mm.SendImageCalls))
	}
	if got := mm.SendImageCalls[0].Target; got != domain.StatusBroadcastJID {
		t.Errorf("target = %q, want %q", got, domain.StatusBroadcastJID)
	}
}

func TestPublishStatusImage_RejectUnauthenticated(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	router := statusImageRouter(mm, statusMediaPNGFetcher)

	body := `{"Image":"https://example.com/photo.png"}`
	rec := statusMediaServe(t, router, "/status/set/image", body, nil)

	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 for unauthenticated request, got 200")
	}
	if len(mm.SendImageCalls) != 0 {
		t.Fatal("SendImage called without auth")
	}
}

func TestPublishStatusImage_MalformedBody(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	router := statusImageRouter(mm, statusMediaPNGFetcher)

	rec := statusMediaServe(t, router, "/status/set/image", "{invalid json", msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if len(mm.SendImageCalls) != 0 {
		t.Fatal("SendImage called with malformed body")
	}
}

func TestPublishStatusImage_SDKFailureReturns500(t *testing.T) {
	boom := errors.New("sdk boom")
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, boom
		},
	}
	router := statusImageRouter(mm, statusMediaPNGFetcher)

	body := `{"Image":"https://example.com/photo.png"}`
	rec := statusMediaServe(t, router, "/status/set/image", body, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusInternalServerError)
}

// --- /status/set/video -------------------------------------------------------

func TestPublishStatusVideo_SuccessViaRegisteredRoute(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	router := statusVideoRouter(mm, statusMediaVideoFetcher)

	body := `{"Video":"https://example.com/clip.mp4","Caption":"check this out"}`
	rec := statusMediaServe(t, router, "/status/set/video", body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("success = false, want true")
	}

	var result statusMediaResultBody
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if result.Status != "sent" {
		t.Errorf("Status = %q, want %q", result.Status, "sent")
	}

	if len(mm.SendVideoCalls) != 1 {
		t.Fatalf("SendVideo called %d times, want 1", len(mm.SendVideoCalls))
	}
	if got := mm.SendVideoCalls[0].Target; got != domain.StatusBroadcastJID {
		t.Errorf("target = %q, want %q", got, domain.StatusBroadcastJID)
	}
}

func TestPublishStatusVideo_RejectUnauthenticated(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	router := statusVideoRouter(mm, statusMediaVideoFetcher)

	rec := statusMediaServe(t, router, "/status/set/video", `{"Video":"https://example.com/v.mp4"}`, nil)

	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 for unauthenticated request, got 200")
	}
	if len(mm.SendVideoCalls) != 0 {
		t.Fatal("SendVideo called without auth")
	}
}

func TestPublishStatusVideo_MalformedBody(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	router := statusVideoRouter(mm, statusMediaVideoFetcher)

	rec := statusMediaServe(t, router, "/status/set/video", "{bad", msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
}

// --- /status/set/audio -------------------------------------------------------

func TestPublishStatusAudio_SuccessViaRegisteredRoute(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	router := statusAudioRouter(mm, statusMediaAudioFetcher)

	body := `{"Audio":"https://example.com/clip.mp3"}`
	rec := statusMediaServe(t, router, "/status/set/audio", body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("success = false, want true")
	}

	var result statusMediaResultBody
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if result.Status != "sent" {
		t.Errorf("Status = %q, want %q", result.Status, "sent")
	}

	if len(mm.SendAudioCalls) != 1 {
		t.Fatalf("SendAudio called %d times, want 1", len(mm.SendAudioCalls))
	}
	if got := mm.SendAudioCalls[0].Target; got != domain.StatusBroadcastJID {
		t.Errorf("target = %q, want %q", got, domain.StatusBroadcastJID)
	}
}

func TestPublishStatusAudio_RejectUnauthenticated(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	router := statusAudioRouter(mm, statusMediaAudioFetcher)

	rec := statusMediaServe(t, router, "/status/set/audio", `{"Audio":"https://example.com/a.mp3"}`, nil)

	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 for unauthenticated request, got 200")
	}
	if len(mm.SendAudioCalls) != 0 {
		t.Fatal("SendAudio called without auth")
	}
}

func TestPublishStatusAudio_MalformedBody(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	router := statusAudioRouter(mm, statusMediaAudioFetcher)

	rec := statusMediaServe(t, router, "/status/set/audio", "{nope", msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
}
