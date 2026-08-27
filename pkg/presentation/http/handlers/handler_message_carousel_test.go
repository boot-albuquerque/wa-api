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
)

// This file covers POST /chat/send/carousel: the full chain from HTTP through
// the handler and use case to InteractiveMessenger.SendCarousel, by the
// REGISTERED gorilla/mux route (ARMADILHA 2).

const sendCarouselPhone = "5511999999999@s.whatsapp.net"

const sendCarouselBody = `{"phone":"` + sendCarouselPhone + `","body":"Escolha",` +
	`"cards":[{"body":"Cartao 1","buttons":[{"type":"reply","title":"Sim"}]},` +
	`{"body":"Cartao 2","buttons":[{"type":"reply","title":"Nao"}]}]}`

func sendCarouselRouter(im *contractsfake.InteractiveMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher) http.Handler {
	uc := message.NewSendCarouselUseCase(im, jr, mf, silentLogger{})
	h := NewSendCarouselHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/carousel", h).Methods(http.MethodPost)
	return r
}

func sendCarouselServe(t *testing.T, im *contractsfake.InteractiveMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/carousel", strings.NewReader(body))
	sendCarouselRouter(im, jr, &contractsfake.MediaFetcher{}).ServeHTTP(rec, mut(req))
	return rec
}

// TestSendCarousel_Success_ViaRegisteredRoute exercises the success path with
// two cards, each with one reply button, and confirms the response carries
// {message_id, timestamp, status}.
func TestSendCarousel_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500199)
	body := `{"phone":"` + sendCarouselPhone + `","body":"Escolha uma opcao","footer":"wa-api",` +
		`"cards":[` +
		`{"title":"Cartao A","body":"Corpo A","footer":"pe A","buttons":[{"type":"reply","title":"Sim","id":"btn-a"}]},` +
		`{"title":"Cartao B","body":"Corpo B","buttons":[{"type":"cta_url","title":"Site","url":"https://example.invalid"}]}` +
		`]}`

	im := &contractsfake.InteractiveMessenger{
		SendCarouselFunc: func(_ context.Context, _ string, _ domain.JID, payload domain.CarouselPayload, _ *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			if len(payload.Cards) != 2 {
				t.Fatalf("payload.Cards = %d, want 2", len(payload.Cards))
			}
			if payload.CardType != domain.CarouselHScrollCards {
				t.Errorf("CardType = %q, want %q", payload.CardType, domain.CarouselHScrollCards)
			}
			if payload.Body != "Escolha uma opcao" {
				t.Errorf("Body = %q", payload.Body)
			}
			if payload.Footer != "wa-api" {
				t.Errorf("Footer = %q", payload.Footer)
			}
			if payload.Cards[0].Title != "Cartao A" {
				t.Errorf("Cards[0].Title = %q", payload.Cards[0].Title)
			}
			if payload.Cards[0].Buttons[0].Type != domain.ButtonTypeReply {
				t.Errorf("Cards[0].Buttons[0].Type = %q", payload.Cards[0].Buttons[0].Type)
			}
			if payload.Cards[1].Buttons[0].Type != domain.ButtonTypeCTAURL {
				t.Errorf("Cards[1].Buttons[0].Type = %q", payload.Cards[1].Buttons[0].Type)
			}
			return domain.MessageSendResult{
				ID:        "carousel-msg-42",
				Timestamp: time.Unix(sentAt, 0),
			}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendCarouselServe(t, im, jr, body, msgAuthed)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	env := decodeEnvelope(t, rec)
	var result struct {
		MessageID string `json:"message_id"`
		Timestamp int64  `json:"timestamp"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatalf("data decode: %v", err)
	}
	if result.MessageID != "carousel-msg-42" {
		t.Errorf("message_id = %q, want carousel-msg-42", result.MessageID)
	}
	if result.Timestamp != sentAt {
		t.Errorf("timestamp = %d, want %d", result.Timestamp, sentAt)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("status = %q, want %q", result.Status, domain.StatusSent)
	}
}

// TestSendCarousel_RejectMissingFields rejects when Phone, Body or Cards are
// missing.
func TestSendCarousel_RejectMissingFields(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"no phone", `{"body":"x","cards":[{"body":"c","buttons":[{"type":"reply","title":"y"}]}]}`},
		{"no body", `{"phone":"5511999999999","cards":[{"body":"c","buttons":[{"type":"reply","title":"y"}]}]}`},
		{"no cards", `{"phone":"5511999999999","body":"x","cards":[]}`},
		{"null cards", `{"phone":"5511999999999","body":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := sendCarouselServe(t, &contractsfake.InteractiveMessenger{}, &contractsfake.JIDResolver{}, tc.body, msgAuthed)
			if rec.Code == http.StatusOK {
				t.Errorf("expected rejection, got 200")
			}
		})
	}
}

// TestSendCarousel_CardWithNoButtonsIsDropped: a card whose buttons all get
// discarded is silently dropped; if ALL cards are dropped the request is
// rejected.
func TestSendCarousel_CardWithNoButtonsIsDropped(t *testing.T) {
	body := `{"phone":"` + sendCarouselPhone + `","body":"Corpo",` +
		`"cards":[{"body":"Cartao sem botao valido","buttons":[{"type":"DESCONHECIDO","title":"X"}]}]}`
	rec := sendCarouselServe(t, &contractsfake.InteractiveMessenger{}, &contractsfake.JIDResolver{}, body, msgAuthed)
	if rec.Code == http.StatusOK {
		t.Error("all cards dropped but got 200")
	}
}

// TestSendCarousel_CardWithEmptyBodyIsDropped: a card with an empty Body is
// silently dropped.
func TestSendCarousel_CardWithEmptyBodyIsDropped(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{
		SendCarouselFunc: func(_ context.Context, _ string, _ domain.JID, payload domain.CarouselPayload, _ *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			if len(payload.Cards) != 1 {
				t.Fatalf("expected 1 surviving card, got %d", len(payload.Cards))
			}
			if payload.Cards[0].Body != "Valido" {
				t.Errorf("surviving card body = %q", payload.Cards[0].Body)
			}
			return domain.MessageSendResult{ID: "ok"}, nil
		},
	}
	body := `{"phone":"` + sendCarouselPhone + `","body":"Corpo",` +
		`"cards":[` +
		`{"body":"  ","buttons":[{"type":"reply","title":"X"}]},` +
		`{"body":"Valido","buttons":[{"type":"reply","title":"Y"}]}]}`
	rec := sendCarouselServe(t, im, &contractsfake.JIDResolver{}, body, msgAuthed)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestSendCarousel_ButtonNormalisationMatchesButtons: the same normalisation
// rules apply — title fallback, truncation, id fallback, unknown-type discard.
func TestSendCarousel_ButtonNormalisationMatchesButtons(t *testing.T) {
	longTitle := strings.Repeat("A", 25)
	body := `{"phone":"` + sendCarouselPhone + `","body":"Corpo",` +
		`"cards":[{"body":"Cartao","buttons":[{"type":"reply","title":"` + longTitle + `"}]}]}`

	im := &contractsfake.InteractiveMessenger{
		SendCarouselFunc: func(_ context.Context, _ string, _ domain.JID, payload domain.CarouselPayload, _ *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			btn := payload.Cards[0].Buttons[0]
			if got := len([]rune(btn.Title)); got != 20 {
				t.Errorf("button title length = %d runes, want 20 (truncated)", got)
			}
			if btn.ID != btn.Title {
				t.Errorf("ID = %q, want title fallback %q", btn.ID, btn.Title)
			}
			return domain.MessageSendResult{ID: "ok"}, nil
		},
	}
	rec := sendCarouselServe(t, im, &contractsfake.JIDResolver{}, body, msgAuthed)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestSendCarousel_AlbumImageIsNeverExposed: even if someone constructs a
// request, the use case always hardcodes HSCROLL_CARDS.
func TestSendCarousel_AlbumImageIsNeverExposed(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{
		SendCarouselFunc: func(_ context.Context, _ string, _ domain.JID, payload domain.CarouselPayload, _ *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			if payload.CardType != domain.CarouselHScrollCards {
				t.Errorf("CardType = %q, want hscroll_cards — ALBUM_IMAGE must not leak", payload.CardType)
			}
			return domain.MessageSendResult{ID: "ok"}, nil
		},
	}
	rec := sendCarouselServe(t, im, &contractsfake.JIDResolver{}, sendCarouselBody, msgAuthed)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

// TestSendCarousel_SessionFailure: EnsureSession error propagates.
func TestSendCarousel_SessionFailure(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{
		SessionGuard: contractsfake.SessionGuard{
			EnsureSessionFunc: func(context.Context, string) error {
				return errors.New("no session")
			},
		},
	}
	rec := sendCarouselServe(t, im, &contractsfake.JIDResolver{}, sendCarouselBody, msgAuthed)
	if rec.Code == http.StatusOK {
		t.Error("expected session failure, got 200")
	}
}

// TestSendCarousel_RejectUnauthenticated: no auth context => 401.
func TestSendCarousel_RejectUnauthenticated(t *testing.T) {
	rec := sendCarouselServe(t, &contractsfake.InteractiveMessenger{}, &contractsfake.JIDResolver{},
		sendCarouselBody, func(r *http.Request) *http.Request { return r })
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// TestSendCarousel_MalformedBody: invalid JSON => 400.
func TestSendCarousel_MalformedBody(t *testing.T) {
	rec := sendCarouselServe(t, &contractsfake.InteractiveMessenger{}, &contractsfake.JIDResolver{},
		`{broken`, msgAuthed)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestSendCarousel_ClientSuppliedIDIsForwarded confirms the optional `Id`
// field reaches the port.
func TestSendCarousel_ClientSuppliedIDIsForwarded(t *testing.T) {
	body := `{"phone":"` + sendCarouselPhone + `","body":"Corpo","id":"client-id-99",` +
		`"cards":[{"body":"C","buttons":[{"type":"reply","title":"Y"}]}]}`
	im := &contractsfake.InteractiveMessenger{
		SendCarouselFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.CarouselPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			if id != "client-id-99" {
				t.Errorf("id = %q, want client-id-99", id)
			}
			return domain.MessageSendResult{ID: "server-id"}, nil
		},
	}
	rec := sendCarouselServe(t, im, &contractsfake.JIDResolver{}, body, msgAuthed)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	env := decodeEnvelope(t, rec)
	var result struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatal(err)
	}
	if result.MessageID != "server-id" {
		t.Errorf("message_id = %q, want server-id (server wins)", result.MessageID)
	}
}
