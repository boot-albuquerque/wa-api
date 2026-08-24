package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain"
)

func starRouter(h http.Handler) *mux.Router {
	r := mux.NewRouter()
	r.Handle("/message/star", h).Methods("POST")
	return r
}

func TestStarMessageHandler_Success_Star(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	h := NewStarMessageHandler(chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, silentLogger{}))

	body := `{"chat":"120363111111111111@g.us","sender":"5511999999999@s.whatsapp.net","message_id":"ABCDE12345","from_me":false,"star":true}`
	req := httptest.NewRequest("POST", "/message/star", strings.NewReader(body))
	req = ipmWithUser(req, "user-1")
	rec := httptest.NewRecorder()
	starRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("success=false on a 200: %s", rec.Body.String())
	}
	if len(ms.StarMessageCalls) != 1 {
		t.Fatalf("StarMessage calls = %d, want 1", len(ms.StarMessageCalls))
	}
	c := ms.StarMessageCalls[0]
	if c.FromMe != false {
		t.Error("fromMe should be false")
	}
	if c.Star != true {
		t.Error("star should be true")
	}
}

func TestStarMessageHandler_Success_Unstar(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	h := NewStarMessageHandler(chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, silentLogger{}))

	body := `{"chat":"120363111111111111@g.us","sender":"5511999999999@s.whatsapp.net","message_id":"ABCDE12345","from_me":true,"star":false}`
	req := httptest.NewRequest("POST", "/message/star", strings.NewReader(body))
	req = ipmWithUser(req, "user-1")
	rec := httptest.NewRecorder()
	starRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(ms.StarMessageCalls) != 1 {
		t.Fatalf("StarMessage calls = %d, want 1", len(ms.StarMessageCalls))
	}
	c := ms.StarMessageCalls[0]
	if c.FromMe != true {
		t.Error("fromMe should be true")
	}
	if c.Star != false {
		t.Error("star should be false for unstar")
	}
}

func TestStarMessageHandler_MissingFields(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	h := NewStarMessageHandler(chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, silentLogger{}))

	body := `{"star":true}`
	req := httptest.NewRequest("POST", "/message/star", strings.NewReader(body))
	req = ipmWithUser(req, "user-1")
	rec := httptest.NewRecorder()
	starRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for missing fields (body: %s)", rec.Code, rec.Body.String())
	}
	if len(ms.StarMessageCalls) != 0 {
		t.Error("StarMessage should not be called with missing fields")
	}
}

func TestStarMessageHandler_PortError(t *testing.T) {
	ms := &contractsfake.MessageStarrer{
		StarMessageFunc: func(context.Context, string, domain.JID, domain.JID, string, bool, bool) error {
			return errors.New("sdk down")
		},
	}
	h := NewStarMessageHandler(chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, silentLogger{}))

	body := `{"chat":"120363111111111111@g.us","sender":"5511999999999@s.whatsapp.net","message_id":"ABCDE12345","from_me":false,"star":true}`
	req := httptest.NewRequest("POST", "/message/star", strings.NewReader(body))
	req = ipmWithUser(req, "user-1")
	rec := httptest.NewRecorder()
	starRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for port error (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestStarMessageHandler_BadJSON(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	h := NewStarMessageHandler(chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, silentLogger{}))

	req := httptest.NewRequest("POST", "/message/star", strings.NewReader("{broken"))
	req = ipmWithUser(req, "user-1")
	rec := httptest.NewRecorder()
	starRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for bad JSON", rec.Code)
	}
	if len(ms.StarMessageCalls) != 0 {
		t.Error("StarMessage should not be called with bad JSON")
	}
}
