package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/domain"
)

// CAP-46B — reply-to (quote) across ALL 13 send routes. Each case tests via
// the gorilla/mux registered route (ARMADILHA 2): the handler decodes JSON
// into the domain DTO, the use case forwards req.ReplyTo to the port, and
// we capture it in the fake. Two legs per route: WITH ReplyTo and WITHOUT.

type replyToCase struct {
	nome     string
	bodyWith string // JSON with ReplyTo
	bodyNo   string // JSON without ReplyTo
	serve    func(t *testing.T, capture *replyToCapture) serveFunc
}

type replyToCapture struct {
	got *domain.ReplyContext
}

type serveFunc func(body string) *replyToResult

type replyToResult struct {
	code int
	body string
}

func replyToCases() []replyToCase {
	replyJSON := `,"reply_to":{"stanza_id":"q-123","participant":"5511888888888@s.whatsapp.net","quoted_text":"original"}`
	result := domain.MessageSendResult{ID: "wire-reply", Timestamp: time.Unix(1755500123, 0)}

	return []replyToCase{
		{
			nome:     "text",
			bodyWith: `{"phone":"5511999999999","body":"reply"` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","body":"plain"}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				tm := &contractsfake.TextMessenger{
					SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, replyTo *domain.ReplyContext, _ []string, _ *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendTextRouter(tm, &contractsfake.JIDResolver{})
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/text", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "image",
			bodyWith: `{"phone":"5511999999999","image":"` + sendImageTestURL + `","caption":"leg"` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","image":"` + sendImageTestURL + `","caption":"leg"}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				mm := &contractsfake.MediaMessenger{
					SendImageFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, replyTo *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendImageRouter(mm, &contractsfake.JIDResolver{}, defaultSendImageFetcher())
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/image", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "audio",
			bodyWith: `{"phone":"5511999999999","audio":"` + sendAudioTestURL + `"` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","audio":"` + sendAudioTestURL + `"}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				mm := &contractsfake.MediaMessenger{
					SendAudioFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.AudioPayload, replyTo *domain.ReplyContext, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendAudioRouter(mm, &contractsfake.JIDResolver{}, defaultSendAudioFetcher())
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/audio", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "video",
			bodyWith: `{"phone":"5511999999999","video":"` + sendVideoTestURL + `"` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","video":"` + sendVideoTestURL + `"}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				mm := &contractsfake.MediaMessenger{
					SendVideoFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, replyTo *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendVideoRouter(mm, &contractsfake.JIDResolver{}, defaultSendVideoFetcher())
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/video", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "document",
			bodyWith: `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.pdf"` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.pdf"}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				mm := &contractsfake.MediaMessenger{
					SendDocumentFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, replyTo *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendDocumentRouter(mm, &contractsfake.JIDResolver{}, defaultSendDocumentFetcher())
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/document", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "sticker",
			bodyWith: `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `"` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `"}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				mm := &contractsfake.MediaMessenger{
					SendStickerFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, replyTo *domain.ReplyContext, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendStickerRouter(mm, &contractsfake.JIDResolver{}, defaultSendStickerFetcher(), defaultSendStickerProcessor())
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/sticker", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "location",
			bodyWith: `{"phone":"5511999999999","name":"Praca","latitude":-23.55,"longitude":-46.63` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","name":"Praca","latitude":-23.55,"longitude":-46.63}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				sm := &contractsfake.SimpleMessenger{
					SendLocationFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.LocationPayload, replyTo *domain.ReplyContext, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendLocationRouter(sm, &contractsfake.JIDResolver{})
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/location", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "contact",
			bodyWith: `{"phone":"5511999999999","name":"Alice","vcard":"BEGIN:VCARD\nVERSION:3.0\nFN:Alice\nEND:VCARD"` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","name":"Alice","vcard":"BEGIN:VCARD\nVERSION:3.0\nFN:Alice\nEND:VCARD"}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				sm := &contractsfake.SimpleMessenger{
					SendContactFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ContactPayload, replyTo *domain.ReplyContext, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendContactRouter(sm, &contractsfake.JIDResolver{})
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/contact", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "poll",
			bodyWith: `{"group":"120363313346913103@g.us","header":"Almoco?","options":["12h","13h"]` + replyJSON + `}`,
			bodyNo:   `{"group":"120363313346913103@g.us","header":"Almoco?","options":["12h","13h"]}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				sm := &contractsfake.SimpleMessenger{
					SendPollFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.PollPayload, replyTo *domain.ReplyContext, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendPollRouter(sm, &contractsfake.JIDResolver{})
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/poll", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "template",
			bodyWith: `{"phone":"5511999999999","content":"Escolha","footer":"Equipe","buttons":[{"display_text":"Sim","type":"quickreply"}]` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","content":"Escolha","footer":"Equipe","buttons":[{"display_text":"Sim","type":"quickreply"}]}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				sm := &contractsfake.SimpleMessenger{
					SendTemplateFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.TemplatePayload, replyTo *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendTemplateRouter(sm, &contractsfake.JIDResolver{})
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/template", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "buttons",
			bodyWith: `{"phone":"5511999999999","body":"Escolha","buttons":[{"type":"reply","title":"Sim","id":"btn-sim"}]` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","body":"Escolha","buttons":[{"type":"reply","title":"Sim","id":"btn-sim"}]}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				im := &contractsfake.InteractiveMessenger{
					SendButtonsFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ButtonsPayload, replyTo *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendButtonsRouter(im, &contractsfake.JIDResolver{}, &contractsfake.MediaFetcher{})
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/buttons", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "carousel",
			bodyWith: `{"phone":"5511999999999","body":"Escolha","cards":[{"body":"C","buttons":[{"type":"reply","title":"Sim"}]}]` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","body":"Escolha","cards":[{"body":"C","buttons":[{"type":"reply","title":"Sim"}]}]}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				im := &contractsfake.InteractiveMessenger{
					SendCarouselFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.CarouselPayload, replyTo *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendCarouselRouter(im, &contractsfake.JIDResolver{}, &contractsfake.MediaFetcher{})
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/carousel", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
		{
			nome:     "list",
			bodyWith: `{"phone":"5511999999999","desc":"Escolha","sections":[{"title":"Sec","rows":[{"title":"Item"}]}]` + replyJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","desc":"Escolha","sections":[{"title":"Sec","rows":[{"title":"Item"}]}]}`,
			serve: func(t *testing.T, cap *replyToCapture) serveFunc {
				sm := &contractsfake.SimpleMessenger{
					SendListFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ListPayload, replyTo *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
						cap.got = replyTo
						return result, nil
					},
				}
				r := sendListRouter(sm, &contractsfake.JIDResolver{})
				return func(body string) *replyToResult {
					rec := sendWirePost(t, r, "/chat/send/list", body)
					return &replyToResult{code: rec.Code, body: rec.Body.String()}
				}
			},
		},
	}
}

// TestSendReplyTo_ViaRegisteredRoute: ReplyTo in JSON payload reaches the
// port fake through the full HTTP -> handler -> usecase -> port chain, via
// the gorilla/mux registered route. Covers all 13 send capabilities.
func TestSendReplyTo_ViaRegisteredRoute(t *testing.T) {
	cases := replyToCases()
	if len(cases) != 13 {
		t.Fatalf("suite covers %d routes, want 13", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.nome+"/with_reply_to", func(t *testing.T) {
			cap := &replyToCapture{}
			serve := tc.serve(t, cap)
			rec := serve(tc.bodyWith)

			if rec.code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rec.code, rec.body)
			}
			if cap.got == nil {
				t.Fatal("ReplyTo not forwarded through the registered route")
			}
			if cap.got.StanzaID != "q-123" {
				t.Errorf("StanzaID = %q, want %q", cap.got.StanzaID, "q-123")
			}
			if cap.got.Participant != "5511888888888@s.whatsapp.net" {
				t.Errorf("Participant = %q, want %q", cap.got.Participant, "5511888888888@s.whatsapp.net")
			}
			if cap.got.QuotedText != "original" {
				t.Errorf("QuotedText = %q, want %q", cap.got.QuotedText, "original")
			}
		})

		t.Run(tc.nome+"/without_reply_to", func(t *testing.T) {
			cap := &replyToCapture{}
			serve := tc.serve(t, cap)
			rec := serve(tc.bodyNo)

			if rec.code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rec.code, rec.body)
			}
			if cap.got != nil {
				t.Errorf("ReplyTo = %+v, want nil (no ReplyTo in request)", cap.got)
			}
		})
	}
}
