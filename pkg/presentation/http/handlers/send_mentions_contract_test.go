package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/domain"
)

// CAP-47 — MentionedJid across the 8 send routes that carry user-visible
// text. Each case tests via the gorilla/mux registered route (ARMADILHA 2):
// the handler decodes JSON into the domain DTO, the use case forwards
// req.MentionedJID to the port, and we capture it in the fake. Two legs per
// route: WITH MentionedJid and WITHOUT.

type mentionsCase struct {
	nome     string
	bodyWith string
	bodyNo   string
	serve    func(t *testing.T, capture *mentionsCapture) serveFunc
}

type mentionsCapture struct {
	got []string
	set bool
}

func mentionsCases() []mentionsCase {
	mentionsJSON := `,"mentioned_jid":["5511888888888@s.whatsapp.net","5511777777777@s.whatsapp.net"]`
	result := domain.MessageSendResult{ID: "wire-mention", Timestamp: time.Unix(1755500200, 0)}

	return []mentionsCase{
		{
			nome:     "text",
			bodyWith: `{"phone":"5511999999999","body":"@Alice @Bob"` + mentionsJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","body":"plain text"}`,
			serve: func(t *testing.T, cap *mentionsCapture) serveFunc {
				tm := &contractsfake.TextMessenger{
					SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, mentionedJID []string, _ *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
						cap.got = mentionedJID
						cap.set = true
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
			bodyWith: `{"phone":"5511999999999","image":"` + sendImageTestURL + `","caption":"@Alice"` + mentionsJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","image":"` + sendImageTestURL + `","caption":"leg"}`,
			serve: func(t *testing.T, cap *mentionsCapture) serveFunc {
				mm := &contractsfake.MediaMessenger{
					SendImageFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, _ *domain.ReplyContext, mentionedJID []string, _ string) (domain.MessageSendResult, error) {
						cap.got = mentionedJID
						cap.set = true
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
			nome:     "video",
			bodyWith: `{"phone":"5511999999999","video":"` + sendVideoTestURL + `","caption":"@Alice"` + mentionsJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","video":"` + sendVideoTestURL + `","caption":"leg"}`,
			serve: func(t *testing.T, cap *mentionsCapture) serveFunc {
				mm := &contractsfake.MediaMessenger{
					SendVideoFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, _ *domain.ReplyContext, mentionedJID []string, _ string) (domain.MessageSendResult, error) {
						cap.got = mentionedJID
						cap.set = true
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
			bodyWith: `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.pdf","caption":"@Alice"` + mentionsJSON + `}`,
			bodyNo:   `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.pdf","caption":"leg"}`,
			serve: func(t *testing.T, cap *mentionsCapture) serveFunc {
				mm := &contractsfake.MediaMessenger{
					SendDocumentFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, _ *domain.ReplyContext, mentionedJID []string, _ string) (domain.MessageSendResult, error) {
						cap.got = mentionedJID
						cap.set = true
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
			nome:     "template",
			bodyWith: `{"phone":"5511999999999","content":"@Alice","footer":"f"` + mentionsJSON + `,"buttons":[{"display_text":"Ok","type":"reply"}]}`,
			bodyNo:   `{"phone":"5511999999999","content":"text","footer":"f","buttons":[{"display_text":"Ok","type":"reply"}]}`,
			serve: func(t *testing.T, cap *mentionsCapture) serveFunc {
				sm := &contractsfake.SimpleMessenger{
					SendTemplateFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.TemplatePayload, _ *domain.ReplyContext, mentionedJID []string, _ string) (domain.MessageSendResult, error) {
						cap.got = mentionedJID
						cap.set = true
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
			bodyWith: `{"phone":"5511999999999","body":"@Alice","title":"T"` + mentionsJSON + `,"buttons":[{"type":"reply","title":"Ok"}]}`,
			bodyNo:   `{"phone":"5511999999999","body":"text","title":"T","buttons":[{"type":"reply","title":"Ok"}]}`,
			serve: func(t *testing.T, cap *mentionsCapture) serveFunc {
				im := &contractsfake.InteractiveMessenger{
					SendButtonsFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ButtonsPayload, _ *domain.ReplyContext, mentionedJID []string, _ string) (domain.MessageSendResult, error) {
						cap.got = mentionedJID
						cap.set = true
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
			bodyWith: `{"phone":"5511999999999","body":"@Alice"` + mentionsJSON + `,"cards":[{"body":"C","buttons":[{"type":"reply","title":"Y"}]}]}`,
			bodyNo:   `{"phone":"5511999999999","body":"text","cards":[{"body":"C","buttons":[{"type":"reply","title":"Y"}]}]}`,
			serve: func(t *testing.T, cap *mentionsCapture) serveFunc {
				im := &contractsfake.InteractiveMessenger{
					SendCarouselFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.CarouselPayload, _ *domain.ReplyContext, mentionedJID []string, _ string) (domain.MessageSendResult, error) {
						cap.got = mentionedJID
						cap.set = true
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
			nome: "list",
			bodyWith: `{"phone":"5511999999999","body":"@Alice","button_text":"Ver"` + mentionsJSON +
				`,"sections":[{"title":"S","rows":[{"title":"R","row_id":"r1"}]}]}`,
			bodyNo: `{"phone":"5511999999999","body":"text","button_text":"Ver"` +
				`,"sections":[{"title":"S","rows":[{"title":"R","row_id":"r1"}]}]}`,
			serve: func(t *testing.T, cap *mentionsCapture) serveFunc {
				sm := &contractsfake.SimpleMessenger{
					SendListFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ListPayload, _ *domain.ReplyContext, mentionedJID []string, _ string) (domain.MessageSendResult, error) {
						cap.got = mentionedJID
						cap.set = true
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

func TestSendMentions_ViaRegisteredRoute(t *testing.T) {
	cases := mentionsCases()
	if len(cases) != 8 {
		t.Fatalf("suite covers %d routes, want 8", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.nome+"/with_mentions", func(t *testing.T) {
			cap := &mentionsCapture{}
			serve := tc.serve(t, cap)
			rec := serve(tc.bodyWith)

			if rec.code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rec.code, rec.body)
			}
			if !cap.set {
				t.Fatal("MentionedJID not forwarded through the registered route")
			}
			if len(cap.got) != 2 {
				t.Fatalf("MentionedJID len = %d, want 2", len(cap.got))
			}
			if cap.got[0] != "5511888888888@s.whatsapp.net" {
				t.Errorf("MentionedJID[0] = %q", cap.got[0])
			}
			if cap.got[1] != "5511777777777@s.whatsapp.net" {
				t.Errorf("MentionedJID[1] = %q", cap.got[1])
			}
		})

		t.Run(tc.nome+"/without_mentions", func(t *testing.T) {
			cap := &mentionsCapture{}
			serve := tc.serve(t, cap)
			rec := serve(tc.bodyNo)

			if rec.code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rec.code, rec.body)
			}
			if len(cap.got) != 0 {
				t.Errorf("MentionedJID = %v, want nil/empty (no mentions in request)", cap.got)
			}
		})
	}
}
