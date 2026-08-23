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

// Este arquivo cobre POST /chat/send/buttons desde a migração do CAP-21 para
// port.InteractiveMessenger — o handler que efetivamente monta o
// InteractiveMessage/NativeFlow e o envia pelo wa-noise, e não mais um
// "validated" sem fazer nada. Mesma estrutura de
// handler_send_template_test.go (rota gorilla/mux registrada).
//
// O eixo próprio deste bloco é `Buttons`: o DTO tinha ficado com
// {Phone, Body, Id} e perdido tudo que dá sentido à capability (HOUSEKEEP
// F147), então a rota tem de recusar o corpo sem botão, tem de entregar os
// botões NORMALIZADOS à porta e tem de DESCARTAR EM SILÊNCIO o de tipo
// desconhecido — comportamento histórico preservado por decisão (F148). Qual
// `Name` e quais parâmetros cada tipo vira está medido no adapter
// (pkg/infra/wa-noise/adapters/chat/messenger_buttons_test.go).

const sendButtonsSentinelToken = "send-buttons-sentinel-cause-9d24af"

const sendButtonsPhone = "5511999999999@s.whatsapp.net"

// sendButtonsBody é o menor corpo VÁLIDO da rota: um botão já basta.
const sendButtonsBody = `{"Phone":"` + sendButtonsPhone + `","Body":"Escolha",` +
	`"Buttons":[{"type":"reply","title":"Sim","id":"btn-sim"}]}`

var errSendButtonsSentinel = errors.New(sendButtonsSentinelToken)

// sendButtonsRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendButtonsRouter(im *contractsfake.InteractiveMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher) http.Handler {
	uc := message.NewSendButtonsUseCase(im, jr, mf, silentLogger{})
	h := NewSendButtonsHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/buttons", h).Methods(http.MethodPost)
	return r
}

func sendButtonsServe(t *testing.T, im *contractsfake.InteractiveMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/buttons", strings.NewReader(body))
	sendButtonsRouter(im, jr, &contractsfake.MediaFetcher{}).ServeHTTP(rec, mut(req))
	return rec
}

// sendButtonsServeCapturingLog é o sendButtonsServe com a saída de log da
// requisição — a mesma cadeia hlog que router.go instala, via
// logassert.Wrap. Existe para que os caminhos de erro possam asseverar a
// CAUSA (co-gate D), e não só o status: o envelope de erro deste repo é o
// genérico "bad request", então duas rejeições diferentes com o mesmo status
// são indistinguíveis pelo corpo.
func sendButtonsServeCapturingLog(t *testing.T, im *contractsfake.InteractiveMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(sendButtonsRouter(im, jr, &contractsfake.MediaFetcher{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/buttons", strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

type sendButtonsResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// TestSendButtons_Success_ViaRegisteredRoute prova o caminho HTTP -> handler
// -> usecase -> InteractiveMessenger.SendButtons pela rota gorilla/mux
// REGISTRADA, com Status="sent", MessageID igual ao devolvido pela porta e
// Timestamp vindo do envio real. Os QUATRO tipos de botão atravessam a
// fronteira HTTP aqui, na ordem, com os campos que cada um usa — e com as
// duas cadeias de fallback exercitadas pelo caminho de SUCESSO, e não só
// pelas recusas.
func TestSendButtons_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500141)
	body := `{"Phone":"` + sendButtonsPhone + `","Body":"Escolha uma opcao","Title":"Cabecalho","Footer":"Equipe wa-api","Buttons":[` +
		`{"type":"reply","title":"Sim","id":"cta-42"},` +
		`{"type":"cta_url","text":"Site","url":"https://example.invalid/promo"},` +
		`{"type":"cta_call","buttonText":"Ligar","buttonId":"call-1","phone_number":"+5511987654321"},` +
		`{"type":"COPY","title":"Copiar","copy_code":"PROMO10"}]}`

	im := &contractsfake.InteractiveMessenger{
		SendButtonsFunc: func(_ context.Context, _ string, target domain.JID, payload domain.ButtonsPayload, _ *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			if target != domain.JID(sendButtonsPhone) {
				t.Errorf("target: got %q, want %q", target, sendButtonsPhone)
			}
			if payload.Body != "Escolha uma opcao" {
				t.Errorf("Body: got %q, want o Body do corpo", payload.Body)
			}
			if payload.Title != "Cabecalho" {
				t.Errorf("Title: got %q, want o Title do corpo", payload.Title)
			}
			if payload.Footer != "Equipe wa-api" {
				t.Errorf("Footer: got %q, want o Footer do corpo", payload.Footer)
			}
			want := []domain.InteractiveButton{
				{Type: domain.ButtonTypeReply, Title: "Sim", ID: "cta-42"},
				{Type: domain.ButtonTypeCTAURL, Title: "Site", ID: "Site", URL: "https://example.invalid/promo"},
				{Type: domain.ButtonTypeCTACall, Title: "Ligar", ID: "call-1", PhoneNumber: "+5511987654321"},
				{Type: domain.ButtonTypeCopy, Title: "Copiar", ID: "Copiar", CopyCode: "PROMO10"},
			}
			if len(payload.Buttons) != len(want) {
				t.Fatalf("Buttons: got %d, want %d", len(payload.Buttons), len(want))
			}
			for i := range want {
				if payload.Buttons[i] != want[i] {
					t.Errorf("Buttons[%d]: got %+v, want %+v (a ORDEM importa)", i, payload.Buttons[i], want[i])
				}
			}
			return domain.MessageSendResult{ID: "wire-id-buttons-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendButtonsServe(t, im, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendButtonsResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-buttons-999" {
		t.Errorf("message_id: got %q, want o que a porta devolveu", data.MessageID)
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(im.SendButtonsCalls); n != 1 {
		t.Fatalf("SendButtons chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

// TestSendButtons_UnknownTypeIsSilentlyDiscarded_ViaRegisteredRoute é a
// trava do comportamento preservado por decisão (HOUSEKEEP F148), medida na
// FRONTEIRA HTTP: o cliente manda TRÊS botões, um com erro de digitação no
// `type`, recebe 200 — e saem DOIS. Nada avisa.
//
// O teste vive também aqui, e não só no use case, porque é isso que o
// cliente observa: 200 com menos botões do que ele mandou. Quem quiser
// mudar o contrato (recusar, ou tratar como reply) falha nos dois níveis.
func TestSendButtons_UnknownTypeIsSilentlyDiscarded_ViaRegisteredRoute(t *testing.T) {
	body := `{"Phone":"` + sendButtonsPhone + `","Body":"Escolha","Buttons":[` +
		`{"type":"reply","title":"Sim"},` +
		`{"type":"cta_urll","title":"Erro de digitacao","url":"https://example.invalid/x"},` +
		`{"type":"reply","title":"Nao"}]}`

	im := &contractsfake.InteractiveMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendButtonsServeCapturingLog(t, im, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (o descarte e' SILENCIOSO; corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(im.SendButtonsCalls); n != 1 {
		t.Fatalf("SendButtons chamado %d vez(es), quero 1", n)
	}
	got := im.SendButtonsCalls[0].Payload.Buttons
	if len(got) != 2 {
		t.Fatalf("chegaram %d botao(oes) a porta, quero 2 — o de tipo desconhecido SOME", len(got))
	}
	if got[0].Title != "Sim" || got[1].Title != "Nao" {
		t.Errorf("sobraram %q e %q, quero \"Sim\" e \"Nao\"", got[0].Title, got[1].Title)
	}
	// E o silêncio é literal: nada no log de saída da requisição.
	assertNoOutcomeLog(t, recs)
}

func TestSendButtons_RejectUnauthenticated(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendButtonsServe(t, im, jr, sendButtonsBody, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(im.SendButtonsCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendButtons %d vez(es)", n)
	}
}

// TestSendButtons_RejectMissingRequiredField cobre as recusas de validação
// do histórico pela rota registrada, cada uma pela sua CAUSA no log.
//
// As TRÊS primeiras compartilham a MESMA causa de propósito: o histórico
// tinha uma recusa só para Phone, Body e Buttons ("missing Phone, Body or
// Buttons"), e não uma por campo. A quarta é a outra recusa, a que pega o
// payload em que todos os botões foram descartados na normalização.
func TestSendButtons_RejectMissingRequiredField(t *testing.T) {
	const buttons = `,"Buttons":[{"type":"reply","title":"Sim"}]`

	cases := map[string]struct{ body, cause string }{
		"Phone":           {`{"Body":"Escolha"` + buttons + `}`, "missing Phone, Body or Buttons"},
		"Body":            {`{"Phone":"` + sendButtonsPhone + `"` + buttons + `}`, "missing Phone, Body or Buttons"},
		"Buttons_ausente": {`{"Phone":"` + sendButtonsPhone + `","Body":"Escolha"}`, "missing Phone, Body or Buttons"},
		"Buttons_vazio":   {`{"Phone":"` + sendButtonsPhone + `","Body":"Escolha","Buttons":[]}`, "missing Phone, Body or Buttons"},
		"Buttons_todos_descartados": {
			`{"Phone":"` + sendButtonsPhone + `","Body":"Escolha","Buttons":[{"type":"nao-existe","title":"X"}]}`,
			"no valid buttons parsed"},
	}
	for field, tc := range cases {
		t.Run(field, func(t *testing.T) {
			im := &contractsfake.InteractiveMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec, recs := sendButtonsServeCapturingLog(t, im, jr, tc.body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload invalido (%s) produziu status de sucesso %d", field, rec.Code)
			}
			logassert.OutcomeLogged(t, recs, tc.cause)
			if n := len(im.SendButtonsCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendButtons foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendButtons_SessionFailure(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{SessionGuard: contractsfake.FailSession(errSendButtonsSentinel)}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendButtonsServeCapturingLog(t, im, jr, sendButtonsBody, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	// A CAUSA da porta tem de chegar ao log — o token sentinela é único
	// neste arquivo, então nenhuma outra falha o produziria por acidente.
	logassert.OutcomeLogged(t, recs, sendButtonsSentinelToken)
	if n := len(im.SendButtonsCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendButtons foi chamado %d vez(es)", n)
	}
}

func TestSendButtons_InvalidPhoneNeverSends(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}

	body := `{"Phone":"lixo","Body":"Escolha","Buttons":[{"type":"reply","title":"Sim"}]}`
	rec := sendButtonsServe(t, im, jr, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(im.SendButtonsCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendButtons foi chamado %d vez(es)", n)
	}
}

// TestSendButtons_DownstreamFailureNeverReturns200 é o eixo do defeito
// original: a rota devolvia 200 sem enviar nada. Aqui o envio FALHA, e um
// 200 seria a mentira de volta.
func TestSendButtons_DownstreamFailureNeverReturns200(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{
		SendButtonsFunc: func(context.Context, string, domain.JID, domain.ButtonsPayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendButtonsSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendButtonsServe(t, im, jr, sendButtonsBody, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

func TestSendButtons_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{
		SendButtonsFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ButtonsPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"` + sendButtonsPhone + `","Body":"Escolha","Id":"id-do-cliente",` +
		`"Buttons":[{"type":"reply","title":"Sim"}]}`
	rec := sendButtonsServe(t, im, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendButtonsResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendButtons_NoSecretLeak segue o mesmo padrão de
// TestSendTemplate_NoSecretLeak: o Body, o Title, o Footer e o texto dos
// botões e o header Authorization carregam segredos da F9.4 (a sessão usa um
// id NÃO secreto, de propósito); a sessão falha e o log de saída da rota
// REGISTRADA não pode carregar nenhum deles.
func TestSendButtons_NoSecretLeak(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-buttons-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendButtonsRouter(im, jr, &contractsfake.MediaFetcher{}))

	body := `{"Phone":"` + sendButtonsPhone + `","Body":"` + logassertGlobalHMACKey + `","Title":"` +
		logassertGlobalHMACKey + `","Footer":"` + logassertGlobalHMACKey +
		`","Buttons":[{"type":"reply","title":"` + logassertGlobalHMACKey + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/buttons", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}

// TestSendButtons_MalformedBody_ViaRegisteredRoute recupera o eixo CORPO
// MALFORMADO que as tabelas de handler_message_test.go e
// handler_interactive_test.go cobriam antes da migração do CAP-21.
// Requisição AUTENTICADA de propósito: sem isso o 401 mascara o 400 e o
// teste não mede o decode.
//
// A segunda requisição trava a CAUSA, não só o sintoma: o envelope de erro é
// o genérico "bad request", e o payload truncado também produziria 400 se o
// decode fosse ignorado (o use case rejeitaria por campo ausente). Só o
// registro de saída distingue os dois.
//
// A causa asseverada é "unexpected EOF", e não "could not decode payload":
// ESTE handler loga o erro CRU do decoder (handler_message_buttons.go,
// `Err(err)`), como handler_message_template.go, enquanto os handlers que
// ficaram em handler_interactive.go logam o erro-sentinela genérico. A
// diferença está registrada em HOUSEKEEP F141 e é a forma MAIS operável das
// duas — o erro cru diz onde o JSON quebrou.
func TestSendButtons_MalformedBody_ViaRegisteredRoute(t *testing.T) {
	const malformed = `{"Phone":"55119`

	im := &contractsfake.InteractiveMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendButtonsServe(t, im, jr, malformed, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(im.SendButtonsCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendButtons %d vez(es)", n)
	}

	imLog := &contractsfake.InteractiveMessenger{}
	rec2, recs := sendButtonsServeCapturingLog(t, imLog, jr, malformed, msgAuthed)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", rec2.Code, rec2.Body.String())
	}
	logassert.OutcomeLogged(t, recs, "unexpected EOF")
	if n := len(imLog.SendButtonsCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendButtons %d vez(es)", n)
	}
}

// TestSendButtons_MalformedButtons_ViaRegisteredRoute é o eixo próprio deste
// bloco: `Buttons` com o tipo errado no JSON (objeto onde o schema pede
// lista) é 400 do cliente, não pânico e não 200. É a forma de corpo que o
// DTO só passou a poder receber no CAP-21.
func TestSendButtons_MalformedButtons_ViaRegisteredRoute(t *testing.T) {
	const body = `{"Phone":"` + sendButtonsPhone + `","Body":"Escolha","Buttons":{"title":"Sim"}}`

	im := &contractsfake.InteractiveMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendButtonsServeCapturingLog(t, im, jr, body, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	// A causa nomeia o CAMPO: sem `Buttons` no DTO o decoder nem chegaria a
	// reclamar dele, e o corpo seria aceito com a lista simplesmente
	// ausente — que é exatamente o estado anterior ao CAP-21 (F147).
	logassert.OutcomeLogged(t, recs, "cannot unmarshal object", "SendButtonsRequest.Buttons")
	if n := len(im.SendButtonsCalls); n != 0 {
		t.Fatalf("Buttons malformado alcancou SendButtons %d vez(es)", n)
	}
}

// TestSendButtons_MissingSessionID_ViaRegisteredRoute recupera o eixo
// MISSING SESSION ID. A requisição é AUTENTICADA — userinfo presente no
// contexto — mas com o `Id` VAZIO: é 400 do cliente, não 401, e a distinção
// entre os dois é o ponto do teste. O corpo é VÁLIDO de propósito: se a
// guarda não disparar, nada mais impede o envio, e a porta é alcançada.
func TestSendButtons_MissingSessionID_ViaRegisteredRoute(t *testing.T) {
	noSessionID := func(r *http.Request) *http.Request { return withUser(r, "") }

	im := &contractsfake.InteractiveMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendButtonsServe(t, im, jr, sendButtonsBody, noSessionID)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(im.SendButtonsCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendButtons %d vez(es)", n)
	}

	imLog := &contractsfake.InteractiveMessenger{}
	rec2, recs := sendButtonsServeCapturingLog(t, imLog, jr, sendButtonsBody, noSessionID)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", rec2.Code, rec2.Body.String())
	}
	logassert.OutcomeLogged(t, recs, "missing session id")
	if n := len(imLog.SendButtonsCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendButtons %d vez(es)", n)
	}
}

// TestSendButtons_WrongTypeInContext_ViaRegisteredRoute recupera o eixo
// WRONG TYPE IN CONTEXT. A chave do contexto é tipada, mas o VALOR é `any`:
// um valor que não satisfaz userInfo tem de virar 401, não pânico.
func TestSendButtons_WrongTypeInContext_ViaRegisteredRoute(t *testing.T) {
	wrongType := func(r *http.Request) *http.Request {
		return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
	}

	im := &contractsfake.InteractiveMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendButtonsServe(t, im, jr, sendButtonsBody, wrongType)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(im.SendButtonsCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendButtons %d vez(es)", n)
	}

	imLog := &contractsfake.InteractiveMessenger{}
	rec2, recs := sendButtonsServeCapturingLog(t, imLog, jr, sendButtonsBody, wrongType)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401 (corpo: %s)", rec2.Code, rec2.Body.String())
	}
	logassert.OutcomeLogged(t, recs, "unauthorized")
	if n := len(imLog.SendButtonsCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendButtons %d vez(es)", n)
	}
}

// TestSendButtons_SuccessEmitsNoOutcomeLog recupera o eixo AUSÊNCIA DE LOG
// NO CAMINHO FELIZ. É o eixo que os outros não pegam: um handler que logasse
// TODO request em warn passaria em cada asserção de caminho de erro deste
// arquivo e ainda assim seria o ruído que a Fase 12 existe para evitar. A
// asserção é por NÍVEL do registro (assertNoOutcomeLog), e não por presença
// do campo "error": um Warn de ruído no caminho feliz não traz "error"
// nenhum, e has("error") o deixaria passar.
func TestSendButtons_SuccessEmitsNoOutcomeLog(t *testing.T) {
	im := &contractsfake.InteractiveMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendButtonsServeCapturingLog(t, im, jr, sendButtonsBody, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(im.SendButtonsCalls); n != 1 {
		t.Fatalf("SendButtons chamado %d vez(es), quero 1", n)
	}
	assertNoOutcomeLog(t, recs)
}

// TestSendButtons_AuthenticatedSessionReachesPort recupera o eixo do txtID
// que a tabela de handler_message_test.go travava
// (`EnsureSessionCalls[0].TxtID`) — o eixo do CAP-16, que a migração do
// CAP-15 tinha perdido para o template e que aqui vem junto. Sem ele, trocar
// `txtID` por qualquer outra coisa no handler deixa a suíte inteira verde —
// e uma mensagem sairia pela SESSÃO ERRADA sem que nada acuse.
//
// O id da sessão é um sentinela DIFERENTE do "user-1" de msgAuthed de
// propósito: com "user-1" um handler que ignorasse o contexto e usasse uma
// constante ainda passaria. Aqui só passa quem lê o contexto autenticado.
//
// Os DOIS lados são asseverados, porque são chamadas distintas da porta e um
// defeito pode atingir só uma: a guarda de sessão (EnsureSession) e o envio
// (SendButtons) têm de receber o MESMO id, o do contexto.
func TestSendButtons_AuthenticatedSessionReachesPort(t *testing.T) {
	const sessionID = "send-buttons-session-6b2e07"

	im := &contractsfake.InteractiveMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendButtonsServe(t, im, jr, sendButtonsBody, func(r *http.Request) *http.Request {
		return withUser(r, sessionID)
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(im.EnsureSessionCalls); n != 1 {
		t.Fatalf("EnsureSession chamado %d vez(es), quero 1", n)
	}
	if got := im.EnsureSessionCalls[0].TxtID; got != sessionID {
		t.Errorf("EnsureSession recebeu txtID %q, quero %q (o do contexto autenticado)", got, sessionID)
	}
	if n := len(im.SendButtonsCalls); n != 1 {
		t.Fatalf("SendButtons chamado %d vez(es), quero 1", n)
	}
	if got := im.SendButtonsCalls[0].TxtID; got != sessionID {
		t.Errorf("SendButtons recebeu txtID %q, quero %q (o do contexto autenticado)", got, sessionID)
	}
}
