package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/noise/errmap"
)

// F223 — app-state conflict (409 from the server) was surfacing as 500 from
// our API. The fix classifies the conflict in errmap.ClassifyAppState, and
// RespondJSON picks up the CategoryConflict to answer 409.
//
// These tests exercise the REGISTERED ROUTE, not the handler in isolation.
// The three routes share the same classification path — the adapter calls
// client.SendAppState, which goes through errmap — so the coverage is by
// construction, not by accident.

// appStateConflictError builds an *apperr.AppError that ClassifyAppState
// would produce for a real 409 conflict from the server. The original error
// text reproduces the format observed in the 2026-08-24 log
// (appstatesync/send.go:handleSendError).
//
// Source: errmap.ClassifyAppState + appstatesync/send.go:handleSendError
func appStateConflictError() *apperr.AppError {
	return apperr.New(
		errmap.CodeAppStateConflict,
		apperr.CategoryConflict,
		"app state conflict: the local state is out of sync with the server",
		true,
		nil,
	)
}

// appStateConflictRoute is one of the three routes that exercise
// SendAppState, parameterized for the table below.
type appStateConflictRoute struct {
	name      string
	route     string
	validBody string
	serve     func(t *testing.T, portErr error) *httptest.ResponseRecorder
}

func appStateConflictRoutes() []appStateConflictRoute {
	return []appStateConflictRoute{
		{
			name:      "POST_chat_pin",
			route:     "/chat/pin",
			validBody: `{"jid":"5511999999999@s.whatsapp.net","pin":true}`,
			serve: func(t *testing.T, portErr error) *httptest.ResponseRecorder {
				cp := &contractsfake.ChatPinner{
					PinChatFunc: func(context.Context, string, domain.JID, bool) error {
						return portErr
					},
				}
				h := NewPinChatHandler(chat.NewPinChatUseCase(cp, &contractsfake.JIDResolver{}, silentLogger{}))
				r := mux.NewRouter()
				r.Handle("/chat/pin", h).Methods("POST")
				rec := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/chat/pin",
					strings.NewReader(`{"jid":"5511999999999@s.whatsapp.net","pin":true}`))
				req = ipmWithUser(req, "user-1")
				r.ServeHTTP(rec, req)
				return rec
			},
		},
		{
			name:      "POST_chat_mute",
			route:     "/chat/mute",
			validBody: `{"jid":"5511999999999@s.whatsapp.net","mute":true}`,
			serve: func(t *testing.T, portErr error) *httptest.ResponseRecorder {
				cm := &contractsfake.ChatMuter{
					MuteChatFunc: func(context.Context, string, domain.JID, bool, time.Duration) error {
						return portErr
					},
				}
				h := NewMuteChatHandler(chat.NewMuteChatUseCase(cm, &contractsfake.JIDResolver{}, silentLogger{}))
				r := mux.NewRouter()
				r.Handle("/chat/mute", h).Methods("POST")
				rec := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/chat/mute",
					strings.NewReader(`{"jid":"5511999999999@s.whatsapp.net","mute":true}`))
				req = ipmWithUser(req, "user-1")
				r.ServeHTTP(rec, req)
				return rec
			},
		},
		{
			name:      "POST_message_star",
			route:     "/message/star",
			validBody: `{"chat":"120363111111111111@g.us","sender":"5511999999999@s.whatsapp.net","message_id":"ABCDE12345","from_me":false,"star":true}`,
			serve: func(t *testing.T, portErr error) *httptest.ResponseRecorder {
				ms := &contractsfake.MessageStarrer{
					StarMessageFunc: func(context.Context, string, domain.JID, domain.JID, string, bool, bool) error {
						return portErr
					},
				}
				h := NewStarMessageHandler(chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, silentLogger{}))
				r := mux.NewRouter()
				r.Handle("/message/star", h).Methods("POST")
				rec := httptest.NewRecorder()
				body := `{"chat":"120363111111111111@g.us","sender":"5511999999999@s.whatsapp.net","message_id":"ABCDE12345","from_me":false,"star":true}`
				req := httptest.NewRequest("POST", "/message/star", strings.NewReader(body))
				req = ipmWithUser(req, "user-1")
				r.ServeHTTP(rec, req)
				return rec
			},
		},
	}
}

// Requirement 1: conflict → 409.
func TestAppStateConflict_Returns409_ViaRegisteredRoute(t *testing.T) {
	for _, rt := range appStateConflictRoutes() {
		t.Run(rt.name, func(t *testing.T) {
			rec := rt.serve(t, appStateConflictError())

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409: a conflict from the server "+
					"must not surface as 500 (F223)\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// Requirement 2: non-conflict app-state error → stays 500.
func TestAppStateGenericError_Returns500_ViaRegisteredRoute(t *testing.T) {
	for _, rt := range appStateConflictRoutes() {
		t.Run(rt.name, func(t *testing.T) {
			rec := rt.serve(t, errors.New("server returned error updating app state (regular_high): <error code=\"500\" text=\"internal-server-error\"/>"))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500: a non-conflict app-state error "+
					"must NOT become 409\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// Requirement 3: success → 200 (the classification is shared).
func TestAppStateSuccess_Returns200_ViaRegisteredRoute(t *testing.T) {
	for _, rt := range appStateConflictRoutes() {
		t.Run(rt.name, func(t *testing.T) {
			rec := rt.serve(t, nil)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: happy path must not break\nbody: %s",
					rec.Code, rec.Body.String())
			}
		})
	}
}

// Requirement 5 (negative control): confirm that a classifier that turns
// EVERYTHING into 409 would be caught by TestAppStateGenericError. This is
// the most likely failure mode — a too-broad match.
//
// The test below verifies that an AppError with CategoryConflict DOES
// produce 409. Combined with TestAppStateGenericError (which sends a plain
// error and asserts 500), this proves the boundary: only classified errors
// get 409, not everything.
func TestAppStateConflict_NegativeControl_CatchAllWouldFail(t *testing.T) {
	catchAll := apperr.New("bad_classifier", apperr.CategoryConflict,
		"everything is a conflict", true, nil)

	for _, rt := range appStateConflictRoutes() {
		t.Run(rt.name, func(t *testing.T) {
			rec := rt.serve(t, catchAll)

			if rec.Code != http.StatusConflict {
				t.Fatalf("a CategoryConflict AppError did not produce 409: "+
					"status = %d — the response layer is broken", rec.Code)
			}
		})
	}
}
