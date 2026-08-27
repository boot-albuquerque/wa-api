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

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

const sendPollVoteSentinelToken = "send-pollvote-sentinel-cause-b7d3f1"

const sendPollVoteGroup = "120363313346913103@g.us"

const sendPollVoteSender = "5511999999999@s.whatsapp.net"

const sendPollVoteBody = `{"phone":"` + sendPollVoteGroup + `","sender":"` + sendPollVoteSender + `","poll_message_id":"3EB0POLL1","poll_message_timestamp":1755500100,"options":["12h"]}`

var errSendPollVoteSentinel = errors.New(sendPollVoteSentinelToken)

func sendPollVoteRouter(cm *contractsfake.ChatMessenger, jr *contractsfake.JIDResolver) http.Handler {
	uc := message.NewSendPollVoteUseCase(cm, jr, silentLogger{})
	h := NewSendPollVoteHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/pollvote", h).Methods(http.MethodPost)
	return r
}

func sendPollVoteServe(t *testing.T, cm *contractsfake.ChatMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/pollvote", strings.NewReader(body))
	sendPollVoteRouter(cm, jr).ServeHTTP(rec, mut(req))
	return rec
}

func sendPollVoteServeCapturingLog(t *testing.T, cm *contractsfake.ChatMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(sendPollVoteRouter(cm, jr))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/pollvote", strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

type sendPollVoteResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

func TestSendPollVote_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500115)
	cm := &contractsfake.ChatMessenger{
		SendPollVoteFunc: func(_ context.Context, _ string, target domain.JID, payload domain.PollVotePayload, _ string) (domain.MessageSendResult, error) {
			if target != domain.JID(sendPollVoteGroup) {
				t.Errorf("target: got %q, want %q", target, sendPollVoteGroup)
			}
			if payload.PollChat != domain.JID(sendPollVoteGroup) {
				t.Errorf("PollChat: got %q, want %q", payload.PollChat, sendPollVoteGroup)
			}
			if payload.PollSender != domain.JID(sendPollVoteSender) {
				t.Errorf("PollSender: got %q, want %q", payload.PollSender, sendPollVoteSender)
			}
			if payload.PollMessageID != "3EB0POLL1" {
				t.Errorf("PollMessageID: got %q, want %q", payload.PollMessageID, "3EB0POLL1")
			}
			if payload.PollTimestamp != 1755500100 {
				t.Errorf("PollTimestamp: got %d, want %d", payload.PollTimestamp, 1755500100)
			}
			if len(payload.OptionNames) != 1 || payload.OptionNames[0] != "12h" {
				t.Errorf("OptionNames: got %q, want [12h]", payload.OptionNames)
			}
			return domain.MessageSendResult{ID: "wire-id-pollvote-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollVoteServe(t, cm, jr, sendPollVoteBody, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendPollVoteResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-pollvote-999" {
		t.Errorf("message_id: got %q, want %q (o que a porta devolveu)", data.MessageID, "wire-id-pollvote-999")
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(cm.SendPollVoteCalls); n != 1 {
		t.Fatalf("SendPollVote chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

func TestSendPollVote_RejectUnauthenticated(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollVoteServe(t, cm, jr, sendPollVoteBody, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendPollVote %d vez(es)", n)
	}
}

func TestSendPollVote_RejectMissingRequiredField(t *testing.T) {
	cases := map[string]struct{ body, cause string }{
		"phone":                {`{"sender":"` + sendPollVoteSender + `","poll_message_id":"3EB0POLL1","poll_message_timestamp":1755500100,"options":["12h"]}`, "missing Phone in payload"},
		"poll_message_id":        {`{"phone":"` + sendPollVoteGroup + `","sender":"` + sendPollVoteSender + `","poll_message_timestamp":1755500100,"options":["12h"]}`, "missing PollMessageId in payload"},
		"poll_message_timestamp": {`{"phone":"` + sendPollVoteGroup + `","sender":"` + sendPollVoteSender + `","poll_message_id":"3EB0POLL1","options":["12h"]}`, "missing PollMessageTimestamp in payload"},
		"sender":               {`{"phone":"` + sendPollVoteGroup + `","poll_message_id":"3EB0POLL1","poll_message_timestamp":1755500100,"options":["12h"]}`, "missing Sender in payload"},
		"Options_nenhuma":      {`{"phone":"` + sendPollVoteGroup + `","sender":"` + sendPollVoteSender + `","poll_message_id":"3EB0POLL1","poll_message_timestamp":1755500100}`, "at least 1 option is required"},
	}
	for field, tc := range cases {
		t.Run(field, func(t *testing.T) {
			cm := &contractsfake.ChatMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec, recs := sendPollVoteServeCapturingLog(t, cm, jr, tc.body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload invalido (%s) produziu status de sucesso %d", field, rec.Code)
			}
			logassert.OutcomeLogged(t, recs, tc.cause)
			if n := len(cm.SendPollVoteCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendPollVote foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendPollVote_SessionFailure(t *testing.T) {
	cm := &contractsfake.ChatMessenger{SessionGuard: contractsfake.FailSession(errSendPollVoteSentinel)}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendPollVoteServeCapturingLog(t, cm, jr, sendPollVoteBody, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.OutcomeLogged(t, recs, sendPollVoteSentinelToken)
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendPollVote foi chamado %d vez(es)", n)
	}
}

func TestSendPollVote_InvalidPhoneNeverSends(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			if raw == "lixo" {
				return "", errors.New("jid invalido")
			}
			return domain.JID(raw), nil
		},
	}

	rec := sendPollVoteServe(t, cm, jr, `{"phone":"lixo","sender":"`+sendPollVoteSender+`","poll_message_id":"3EB0POLL1","poll_message_timestamp":1755500100,"options":["12h"]}`, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendPollVote foi chamado %d vez(es)", n)
	}
}

func TestSendPollVote_InvalidSenderNeverSends(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			if raw == "sender-lixo" {
				return "", errors.New("jid invalido")
			}
			return domain.JID(raw), nil
		},
	}

	rec := sendPollVoteServe(t, cm, jr, `{"phone":"`+sendPollVoteGroup+`","sender":"sender-lixo","poll_message_id":"3EB0POLL1","poll_message_timestamp":1755500100,"options":["12h"]}`, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("Sender invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Fatalf("Sender invalido, mas SendPollVote foi chamado %d vez(es)", n)
	}
}

func TestSendPollVote_DownstreamFailureNeverReturns200(t *testing.T) {
	cm := &contractsfake.ChatMessenger{
		SendPollVoteFunc: func(context.Context, string, domain.JID, domain.PollVotePayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendPollVoteSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollVoteServe(t, cm, jr, sendPollVoteBody, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

func TestSendPollVote_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	cm := &contractsfake.ChatMessenger{
		SendPollVoteFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.PollVotePayload, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"phone":"` + sendPollVoteGroup + `","sender":"` + sendPollVoteSender + `","poll_message_id":"3EB0POLL1","poll_message_timestamp":1755500100,"options":["12h"],"id":"id-do-cliente"}`
	rec := sendPollVoteServe(t, cm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendPollVoteResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

func TestSendPollVote_NoSecretLeak(t *testing.T) {
	cm := &contractsfake.ChatMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-pollvote-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendPollVoteRouter(cm, jr))

	body := `{"phone":"` + sendPollVoteGroup + `","sender":"` + logassertGlobalHMACKey + `","poll_message_id":"3EB0POLL1","poll_message_timestamp":1755500100,"options":["` + logassertGlobalHMACKey + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/pollvote", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}

func TestSendPollVote_MalformedBody_ViaRegisteredRoute(t *testing.T) {
	const malformed = `{"phone":"12036`

	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollVoteServe(t, cm, jr, malformed, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendPollVote %d vez(es)", n)
	}

	cmLog := &contractsfake.ChatMessenger{}
	wrapped, capture := logassert.Wrap(sendPollVoteRouter(cmLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, msgAuthed(httptest.NewRequest(http.MethodPost, "/chat/send/pollvote", strings.NewReader(malformed))))

	if logRec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "unexpected EOF")
	if n := len(cmLog.SendPollVoteCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendPollVote %d vez(es)", n)
	}
}

func TestSendPollVote_MissingSessionID_ViaRegisteredRoute(t *testing.T) {
	noSessionID := func(r *http.Request) *http.Request { return withUser(r, "") }

	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollVoteServe(t, cm, jr, sendPollVoteBody, noSessionID)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendPollVote %d vez(es)", n)
	}

	cmLog := &contractsfake.ChatMessenger{}
	wrapped, capture := logassert.Wrap(sendPollVoteRouter(cmLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, noSessionID(httptest.NewRequest(http.MethodPost, "/chat/send/pollvote", strings.NewReader(sendPollVoteBody))))

	if logRec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "missing session id")
	if n := len(cmLog.SendPollVoteCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendPollVote %d vez(es)", n)
	}
}

func TestSendPollVote_WrongTypeInContext_ViaRegisteredRoute(t *testing.T) {
	wrongType := func(r *http.Request) *http.Request {
		return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
	}

	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollVoteServe(t, cm, jr, sendPollVoteBody, wrongType)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(cm.SendPollVoteCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendPollVote %d vez(es)", n)
	}

	cmLog := &contractsfake.ChatMessenger{}
	wrapped, capture := logassert.Wrap(sendPollVoteRouter(cmLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, wrongType(httptest.NewRequest(http.MethodPost, "/chat/send/pollvote", strings.NewReader(sendPollVoteBody))))

	if logRec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "unauthorized")
	if n := len(cmLog.SendPollVoteCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendPollVote %d vez(es)", n)
	}
}

func TestSendPollVote_SuccessEmitsNoOutcomeLog(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendPollVoteRouter(cm, jr))
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, msgAuthed(httptest.NewRequest(http.MethodPost, "/chat/send/pollvote", strings.NewReader(sendPollVoteBody))))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(cm.SendPollVoteCalls); n != 1 {
		t.Fatalf("SendPollVote chamado %d vez(es), quero 1", n)
	}
	assertNoOutcomeLog(t, capture.Records(t))
}
