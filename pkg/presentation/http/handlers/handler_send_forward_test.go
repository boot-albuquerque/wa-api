package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

const sendForwardSentinelToken = "send-forward-sentinel-cause-a9c2e4"

const sendForwardBody = `{"Phone":"5511999999999","Body":"forwarded text"}`

var errSendForwardSentinel = errors.New(sendForwardSentinelToken)

func sendForwardRouter(tm *contractsfake.TextMessenger, jr *contractsfake.JIDResolver) http.Handler {
	uc := message.NewSendForwardUseCase(
		tm,
		&contractsfake.ForwardedMessageSender{},
		&contractsfake.StoredMessageReader{},
		jr,
		silentLogger{},
	)
	h := NewSendForwardHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/forward", h).Methods(http.MethodPost)
	return r
}

func sendForwardServe(t *testing.T, tm *contractsfake.TextMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/forward", strings.NewReader(body))
	sendForwardRouter(tm, jr).ServeHTTP(rec, mut(req))
	return rec
}

type sendForwardResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

func TestSendForward_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500200)
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, target domain.JID, text string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, fwd *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			if target != "5511999999999@s.whatsapp.net" {
				t.Errorf("target: got %q, want %q", target, "5511999999999@s.whatsapp.net")
			}
			if text != "forwarded text" {
				t.Errorf("text: got %q, want %q", text, "forwarded text")
			}
			if fwd == nil {
				t.Fatal("ForwardContext is nil — forwarding not applied")
			}
			if fwd.ForwardingScore != 1 {
				t.Errorf("ForwardingScore: got %d, want 1 (default)", fwd.ForwardingScore)
			}
			return domain.MessageSendResult{ID: "wire-fwd-1", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendForwardServe(t, tm, jr, sendForwardBody, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	var got sendForwardResultBody
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if got.MessageID != "wire-fwd-1" {
		t.Errorf("message_id: got %q, want %q", got.MessageID, "wire-fwd-1")
	}
	if got.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", got.Timestamp, sentAt)
	}
	if got.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", got.Status, domain.StatusSent)
	}
}

func TestSendForward_RejectUnauthenticated(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendForwardServe(t, tm, jr, sendForwardBody, func(r *http.Request) *http.Request { return r })

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if n := len(tm.SendTextCalls); n != 0 {
		t.Fatalf("unauthenticated request reached SendText %d time(s)", n)
	}
}

func TestSendForward_RejectMissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"Phone", `{"Body":"fwd"}`},
		{"Body", `{"Phone":"5511999999999"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tm := &contractsfake.TextMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec := sendForwardServe(t, tm, jr, tc.body, msgAuthed)

			if rec.Code == http.StatusOK {
				t.Fatal("missing field accepted as 200")
			}
			if n := len(tm.SendTextCalls); n != 0 {
				t.Fatalf("invalid payload but SendText was called %d time(s)", n)
			}
		})
	}
}

func TestSendForward_SessionFailure(t *testing.T) {
	tm := &contractsfake.TextMessenger{SessionGuard: contractsfake.FailSession(errSendForwardSentinel)}
	jr := &contractsfake.JIDResolver{}

	rec := sendForwardServe(t, tm, jr, sendForwardBody, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatal("session failure returned 200")
	}
	if n := len(tm.SendTextCalls); n != 0 {
		t.Fatalf("no session but SendText was called %d time(s)", n)
	}
}

func TestSendForward_DownstreamFailureNeverReturns200(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(context.Context, string, domain.JID, string, *domain.LinkPreviewData, *domain.ReplyContext, []string, *domain.ForwardContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendForwardSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendForwardServe(t, tm, jr, sendForwardBody, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatal("downstream failure returned 200")
	}
}

func TestSendForward_MalformedBody(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendForwardServe(t, tm, jr, `{invalid`, msgAuthed)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for malformed JSON", rec.Code)
	}
}

func TestSendForward_MissingSessionID(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendForwardServe(t, tm, jr, sendForwardBody, func(r *http.Request) *http.Request {
		return withUser(r, "")
	})

	if rec.Code == http.StatusOK {
		t.Fatal("empty session ID returned 200")
	}
}

func TestSendForward_NoSecretLeak(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(context.Context, string, domain.JID, string, *domain.LinkPreviewData, *domain.ReplyContext, []string, *domain.ForwardContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendForwardSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","Body":"` + logassertGlobalHMACKey + `"}`
	wrapped, capture := logassert.Wrap(sendForwardRouter(tm, jr))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/forward", strings.NewReader(body))
	wrapped.ServeHTTP(rec, msgAuthed(req))

	for _, line := range capture.Records(t) {
		for _, secret := range logassertSecretValues {
			if strings.Contains(line.Raw, secret) {
				t.Errorf("secret %q leaked in log line: %s", secret, line.Raw)
			}
		}
	}
}

func TestSendForward_SuccessEmitsNoOutcomeLog(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(context.Context, string, domain.JID, string, *domain.LinkPreviewData, *domain.ReplyContext, []string, *domain.ForwardContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-fwd-nolog", Timestamp: time.Unix(1755500200, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendForwardRouter(tm, jr))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/forward", strings.NewReader(sendForwardBody))
	wrapped.ServeHTTP(rec, msgAuthed(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	for _, line := range capture.Records(t) {
		lvl := line.str("level")
		if lvl == "error" || lvl == "warn" {
			t.Errorf("unexpected %s log on success: %s", lvl, line.Raw)
		}
	}
}

func TestSendForward_ClientSuppliedIDForwarded(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, _ *domain.ForwardContext, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id forwarded to port: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "wire-id-server"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","Body":"fwd","Id":"id-do-cliente"}`
	rec := sendForwardServe(t, tm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	var got sendForwardResultBody
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if got.MessageID != "wire-id-server" {
		t.Errorf("message_id: got %q, want server-assigned %q", got.MessageID, "wire-id-server")
	}
}

func TestSendForward_CustomForwardingScore(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, fwd *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			if fwd == nil {
				t.Fatal("ForwardContext is nil")
			}
			if fwd.ForwardingScore != 7 {
				t.Errorf("ForwardingScore: got %d, want 7", fwd.ForwardingScore)
			}
			return domain.MessageSendResult{ID: "wire-fwd-7"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","Body":"fwd","ForwardingScore":7}`
	rec := sendForwardServe(t, tm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

// --- CAP-55: Forward by key, via registered route ---

func sendForwardByKeyRouter(fwd *contractsfake.ForwardedMessageSender, smr *contractsfake.StoredMessageReader, jr *contractsfake.JIDResolver) http.Handler {
	uc := message.NewSendForwardUseCase(
		&contractsfake.TextMessenger{},
		fwd,
		smr,
		jr,
		silentLogger{},
	)
	h := NewSendForwardHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/forward", h).Methods(http.MethodPost)
	return r
}

func sendForwardByKeyServe(t *testing.T, fwd *contractsfake.ForwardedMessageSender, smr *contractsfake.StoredMessageReader, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/forward", strings.NewReader(body))
	sendForwardByKeyRouter(fwd, smr, jr).ServeHTTP(rec, mut(req))
	return rec
}

func TestSendForwardByKey_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1756137600)
	fakeJSON := `{"Message":{"extendedTextMessage":{"text":"hello"}}}`

	fwd := &contractsfake.ForwardedMessageSender{
		SendForwardedMessageFunc: func(_ context.Context, _ string, target domain.JID, gotJSON string, _ string) (domain.MessageSendResult, error) {
			if target != "5511999999999@s.whatsapp.net" {
				t.Errorf("target: got %q", target)
			}
			if gotJSON != fakeJSON {
				t.Error("dataJSON mismatch")
			}
			return domain.MessageSendResult{ID: "wire-fwd-bykey", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	smr := &contractsfake.StoredMessageReader{
		GetStoredMessageFunc: func(_ context.Context, _, messageID string) (*domain.StoredMessageData, error) {
			if messageID != "MSG_ABC" {
				t.Errorf("messageID: got %q, want MSG_ABC", messageID)
			}
			return &domain.StoredMessageData{DataJSON: fakeJSON, ChatJID: "chat@g.us"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","MessageID":"MSG_ABC"}`
	rec := sendForwardByKeyServe(t, fwd, smr, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	var got sendForwardResultBody
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if got.MessageID != "wire-fwd-bykey" {
		t.Errorf("message_id: got %q, want wire-fwd-bykey", got.MessageID)
	}
	if got.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", got.Timestamp, sentAt)
	}
	if got.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", got.Status, domain.StatusSent)
	}
}

func TestSendForwardByKey_MessageNotFound_Returns404(t *testing.T) {
	fwd := &contractsfake.ForwardedMessageSender{}
	smr := &contractsfake.StoredMessageReader{
		GetStoredMessageFunc: func(_ context.Context, _, _ string) (*domain.StoredMessageData, error) {
			return nil, apperr.New("message_not_found", apperr.CategoryNotFound, "message not found in history", false, nil)
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","MessageID":"NONEXISTENT"}`
	rec := sendForwardByKeyServe(t, fwd, smr, jr, body, msgAuthed)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
	if n := len(fwd.SendForwardedMessageCalls); n != 0 {
		t.Fatalf("not found but SendForwardedMessage called %d time(s)", n)
	}
}

func TestSendForwardByKey_BackwardCompat_BodyWithoutMessageID(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, text string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, fwd *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			if text != "old style text" {
				t.Errorf("text: got %q", text)
			}
			if fwd == nil || fwd.ForwardingScore != 1 {
				t.Errorf("expected default score 1")
			}
			return domain.MessageSendResult{ID: "wire-compat"}, nil
		},
	}
	fwd := &contractsfake.ForwardedMessageSender{}
	smr := &contractsfake.StoredMessageReader{}
	jr := &contractsfake.JIDResolver{}

	uc := message.NewSendForwardUseCase(tm, fwd, smr, jr, silentLogger{})
	h := NewSendForwardHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/forward", h).Methods(http.MethodPost)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/forward",
		strings.NewReader(`{"Phone":"5511999999999","Body":"old style text"}`))
	r.ServeHTTP(rec, msgAuthed(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if n := len(fwd.SendForwardedMessageCalls); n != 0 {
		t.Fatalf("by-content path should NOT call SendForwardedMessage, called %d", n)
	}
}
