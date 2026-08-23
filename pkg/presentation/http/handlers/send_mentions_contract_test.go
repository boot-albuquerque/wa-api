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
	mentionsJSON := `,"MentionedJid":["5511888888888@s.whatsapp.net","5511777777777@s.whatsapp.net"]`
	result := domain.MessageSendResult{ID: "wire-mention", Timestamp: time.Unix(1755500200, 0)}

	return []mentionsCase{
		{
			nome:     "text",
			bodyWith: `{"Phone":"5511999999999","Body":"@Alice @Bob"` + mentionsJSON + `}`,
			bodyNo:   `{"Phone":"5511999999999","Body":"plain text"}`,
			serve: func(t *testing.T, cap *mentionsCapture) serveFunc {
				tm := &contractsfake.TextMessenger{
					SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, mentionedJID []string, _ string) (domain.MessageSendResult, error) {
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
			bodyWith: `{"Phone":"5511999999999","Image":"` + sendImageTestURL + `","Caption":"@Alice"` + mentionsJSON + `}`,
			bodyNo:   `{"Phone":"5511999999999","Image":"` + sendImageTestURL + `","Caption":"leg"}`,
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
			bodyWith: `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `","Caption":"@Alice"` + mentionsJSON + `}`,
			bodyNo:   `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `","Caption":"leg"}`,
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
			bodyWith: `{"Phone":"5511999999999","Document":"` + sendDocumentTestURL + `","FileName":"a.pdf","Caption":"@Alice"` + mentionsJSON + `}`,
			bodyNo:   `{"Phone":"5511999999999","Document":"` + sendDocumentTestURL + `","FileName":"a.pdf","Caption":"leg"}`,
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
			bodyWith: `{"Phone":"5511999999999","Content":"@Alice","Footer":"f"` + mentionsJSON + `,"Buttons":[{"DisplayText":"Ok","Type":"reply"}]}`,
			bodyNo:   `{"Phone":"5511999999999","Content":"text","Footer":"f","Buttons":[{"DisplayText":"Ok","Type":"reply"}]}`,
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
			bodyWith: `{"Phone":"5511999999999","Body":"@Alice","Title":"T"` + mentionsJSON + `,"Buttons":[{"type":"reply","title":"Ok"}]}`,
			bodyNo:   `{"Phone":"5511999999999","Body":"text","Title":"T","Buttons":[{"type":"reply","title":"Ok"}]}`,
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
			bodyWith: `{"Phone":"5511999999999","Body":"@Alice"` + mentionsJSON + `,"Cards":[{"Body":"C","Buttons":[{"type":"reply","title":"Y"}]}]}`,
			bodyNo:   `{"Phone":"5511999999999","Body":"text","Cards":[{"Body":"C","Buttons":[{"type":"reply","title":"Y"}]}]}`,
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
			bodyWith: `{"Phone":"5511999999999","Body":"@Alice","ButtonText":"Ver"` + mentionsJSON +
				`,"Sections":[{"title":"S","rows":[{"title":"R","RowId":"r1"}]}]}`,
			bodyNo: `{"Phone":"5511999999999","Body":"text","ButtonText":"Ver"` +
				`,"Sections":[{"title":"S","rows":[{"title":"R","RowId":"r1"}]}]}`,
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
