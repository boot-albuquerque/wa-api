package handlers

import (
	"context"
	"encoding/base64"
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

// Este arquivo cobre POST /chat/send/audio desde a migração do CAP-05 para
// port.MediaMessenger/port.MediaFetcher — o handler que efetivamente busca
// a URL ou decodifica a data URI, resolve PTT/MIME pela precedência
// histórica, sobe o anexo e entrega o áudio ao wa-noise, e não mais um
// "validated" sem fazer nada. Mesma estrutura de handler_send_document_test.go
// (POST /chat/send/document, CAP-04).

const sendAudioSentinelToken = "send-audio-sentinel-cause-7c1a4d"

var errSendAudioSentinel = errors.New(sendAudioSentinelToken)

const sendAudioTestURL = "https://exemplo.com/nota-de-voz.ogg"

// sendAudioOggBytes é binário (não texto) e não é reconhecido pelo sniffer
// do Go (http.DetectContentType devolve "application/octet-stream") — usado
// para exercitar o fallback de MIME por PTT nos testes de rota.
var sendAudioOggBytes = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C}

func defaultSendAudioFetcher() *contractsfake.MediaFetcher {
	return &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return sendAudioOggBytes, "audio/ogg", nil
		},
	}
}

// sendAudioRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendAudioRouter(mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher) http.Handler {
	uc := message.NewSendAudioUseCase(mm, jr, mf, &contractsfake.TextMessenger{}, silentLogger{})
	h := NewSendAudioHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/audio", h).Methods(http.MethodPost)
	return r
}

func sendAudioServe(t *testing.T, mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/audio", strings.NewReader(body))
	sendAudioRouter(mm, jr, mf).ServeHTTP(rec, mut(req))
	return rec
}

type sendAudioResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// TestSendAudio_Success_ViaRegisteredRoute prova o caminho HTTP -> handler
// -> usecase -> MediaFetcher.FetchBytes -> MediaMessenger.SendAudio pela
// rota gorilla/mux REGISTRADA (transporte URL), com Status="sent",
// MessageID igual ao devolvido pela porta, Timestamp vindo do envio real e
// PTT default (ausente no request) chegando como true.
func TestSendAudio_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500110)
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(_ context.Context, _ string, target domain.JID, payload domain.AudioPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999@s.whatsapp.net") {
				t.Errorf("target: got %q", target)
			}
			if string(payload.Bytes) != string(sendAudioOggBytes) {
				t.Errorf("bytes: divergem dos buscados")
			}
			if payload.PTT != true {
				t.Errorf("PTT: got %v, want true (default, ausente no request)", payload.PTT)
			}
			return domain.MessageSendResult{ID: "wire-id-audio-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendAudioFetcher()

	body := `{"Phone":"5511999999999","Audio":"` + sendAudioTestURL + `"}`
	rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendAudioResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-audio-999" {
		t.Errorf("message_id: got %q, want %q (o que a porta devolveu)", data.MessageID, "wire-id-audio-999")
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(mf.FetchBytesCalls); n != 1 {
		t.Fatalf("fetch chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if got := mf.FetchBytesCalls[0].ResourceURL; got != sendAudioTestURL {
		t.Errorf("URL buscada: got %q, want %q", got, sendAudioTestURL)
	}
	if n := len(mm.SendAudioCalls); n != 1 {
		t.Fatalf("SendAudio chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

func TestSendAudio_RejectUnauthenticated(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendAudioFetcher()

	body := `{"Phone":"5511999999999","Audio":"` + sendAudioTestURL + `"}`
	rec := sendAudioServe(t, mm, jr, mf, body, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou o fetch %d vez(es)", n)
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendAudio %d vez(es)", n)
	}
}

func TestSendAudio_RejectMissingRequiredField(t *testing.T) {
	bodies := map[string]string{
		"Phone": `{"Audio":"` + sendAudioTestURL + `"}`,
		"Audio": `{"Phone":"5511999999999"}`,
	}
	for field, body := range bodies {
		t.Run(field, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendAudioFetcher()

			rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload sem %s produziu status de sucesso %d", field, rec.Code)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Fatalf("payload invalido, mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendAudioCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendAudio foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendAudio_DataURI_Success_ViaRegisteredRoute prova o SEGUNDO
// transporte de POST /chat/send/audio (data URI) pela mesma superfície de
// rota do ramo URL: data URI chega decodificada localmente ao
// MediaMessenger — MediaFetcher nunca é tocado — com Status="sent",
// MessageID vindo da porta de envio, e PTT explícito false respeitado
// (nil e false NAO sao equivalentes).
func TestSendAudio_DataURI_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500120)
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(_ context.Context, _ string, target domain.JID, payload domain.AudioPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999@s.whatsapp.net") {
				t.Errorf("target: got %q", target)
			}
			if payload.MimeType != "audio/ogg" {
				t.Errorf("mimetype: got %q, want audio/ogg", payload.MimeType)
			}
			if payload.PTT != false {
				t.Errorf("PTT: got %v, want false (explicito no request)", payload.PTT)
			}
			return domain.MessageSendResult{ID: "wire-id-audio-datauri-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendAudioFetcher()

	encoded := base64.StdEncoding.EncodeToString(sendAudioOggBytes)
	body := `{"Phone":"5511999999999","Audio":"data:audio/ogg;base64,` + encoded + `","ptt":false}`
	rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendAudioResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-audio-datauri-999" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "wire-id-audio-datauri-999")
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("data URI, mas MediaFetcher foi chamado %d vez(es) pela rota registrada", n)
	}
	if n := len(mm.SendAudioCalls); n != 1 {
		t.Fatalf("SendAudio chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if string(mm.SendAudioCalls[0].Payload.Bytes) != string(sendAudioOggBytes) {
		t.Error("bytes decodificados divergem dos originais")
	}
}

// TestSendAudio_UnsupportedSource_Rejected: fontes que não são nem
// "data:audio/" nem URL http(s) — incluindo base64 cru sem prefixo,
// "data:" de outro MIME e scheme proibido — viram erro pela rota
// registrada, nunca o 200 "validated" falso que o stub anterior produzia.
func TestSendAudio_UnsupportedSource_Rejected(t *testing.T) {
	cases := map[string]string{
		"raw_base64_no_prefix":    "JVBERi0xLjQK",
		"unsupported_scheme":      "ftp://exemplo.com/nota.ogg",
		"data_uri_non_audio_mime": "data:application/pdf;base64,AAAA",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendAudioFetcher()

			body := `{"Phone":"5511999999999","Audio":"` + doc + `"}`
			rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

			if rec.Code == http.StatusOK {
				t.Fatalf("fonte nao suportada %q produziu 200 falso: %s", doc, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if env.Success {
				t.Fatalf("envelope.success=true para fonte nao suportada %q: %s", doc, rec.Body.String())
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Fatalf("fonte nao suportada %q, mas o fetch foi chamado %d vez(es)", doc, n)
			}
			if n := len(mm.SendAudioCalls); n != 0 {
				t.Fatalf("fonte nao suportada %q, mas SendAudio foi chamado %d vez(es)", doc, n)
			}
		})
	}
}

func TestSendAudio_SessionFailure(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSendAudioSentinel)}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendAudioFetcher()

	body := `{"Phone":"5511999999999","Audio":"` + sendAudioTestURL + `"}`
	rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("sessao invalida, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendAudio foi chamado %d vez(es)", n)
	}
}

func TestSendAudio_InvalidPhoneNeverFetchesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	mf := defaultSendAudioFetcher()

	body := `{"Phone":"lixo","Audio":"` + sendAudioTestURL + `"}`
	rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("JID invalido, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendAudio foi chamado %d vez(es)", n)
	}
}

func TestSendAudio_FetchFailure_NeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("fetch: recusado")
		},
	}

	body := `{"Phone":"5511999999999","Audio":"` + sendAudioTestURL + `"}`
	rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no fetch produziu 200: %s", rec.Body.String())
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Fatalf("fetch falhou, mas SendAudio foi chamado %d vez(es)", n)
	}
}

// TestSendAudio_DownstreamFailureNeverReturns200: SendAudio (upload e/ou
// envio) falhando NUNCA produz 200 nem Status=sent — a garantia central do
// CAP-05 contra falso-sucesso, pela rota registrada.
func TestSendAudio_DownstreamFailureNeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(context.Context, string, domain.JID, domain.AudioPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendAudioSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendAudioFetcher()

	body := `{"Phone":"5511999999999","Audio":"` + sendAudioTestURL + `"}`
	rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

func TestSendAudio_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.AudioPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendAudioFetcher()

	body := `{"Phone":"5511999999999","Audio":"` + sendAudioTestURL + `","Id":"id-do-cliente"}`
	rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendAudioResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendAudio_MimeType_FallbackByPTT_ViaRegisteredRoute: pela rota
// registrada, sem MimeType e sem Content-Type de áudio remoto, e com
// bytes que o sniffer do Go não reconhece, o MIME final vem do fallback
// por PTT — nível 4 da precedência.
func TestSendAudio_MimeType_FallbackByPTT_ViaRegisteredRoute(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(_ context.Context, _ string, _ domain.JID, payload domain.AudioPayload, _ *domain.ReplyContext, _ string) (domain.MessageSendResult, error) {
			if payload.MimeType != "audio/mpeg" {
				t.Errorf("mimetype: got %q, want %q (fallback ptt=false)", payload.MimeType, "audio/mpeg")
			}
			return domain.MessageSendResult{ID: "wire-id-mime-fallback"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return sendAudioOggBytes, "", nil
		},
	}

	body := `{"Phone":"5511999999999","Audio":"` + sendAudioTestURL + `","ptt":false}`
	rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(mm.SendAudioCalls); n != 1 {
		t.Fatalf("SendAudio chamado %d vez(es), quero 1", n)
	}
}

// TestSendAudio_NoSecretLeak restaura o eixo perdido pela delecao de
// handler_media_test.go (a tabela sobre port.MessageComposer ficou vazia
// quando CAP-07 migrou o ultimo handler para longe dela — ver
// HOUSEKEEP.md). Phone, o campo Audio e o header Authorization carregam os
// tres segredos da F9.4 (a sessao usa um id NAO secreto, de proposito: em
// producao ele nunca coincide com um dos tres); a sessao falha e o log de
// saida da rota REGISTRADA nao pode carregar nenhum dos tres.
func TestSendAudio_NoSecretLeak(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-audio-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendAudioFetcher()

	wrapped, capture := logassert.Wrap(sendAudioRouter(mm, jr, mf))

	body := `{"Phone":"` + logassertGlobalHMACKey + `","Audio":"` + logassertGlobalEncryptionKey + `"}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/audio", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}

// TestSendAudio_CaptionAcceptedButInert_ViaRegisteredRoute trava o defeito
// HISTÓRICO da F116 pela rota registrada: POST /chat/send/audio aceita
// Caption no JSON (200 OK, áudio enviado), mas o valor NUNCA chega ao
// protocolo — waE2E.AudioMessage não tem Caption. O handler histórico
// (`git show 41bc8e2^:handlers.go`) também não o montava.
//
// A mentira é documentada aqui: o campo existe no contrato público e um
// cliente que o envie recebe 200, mas a legenda é silenciosamente
// descartada. Divergência mantida por decisão de contrato (HOUSEKEEP F116).
func TestSendAudio_CaptionAcceptedButInert_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500130)
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(_ context.Context, _ string, _ domain.JID, payload domain.AudioPayload, _ *domain.ReplyContext, _ string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-caption-inert", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendAudioFetcher()

	body := `{"Phone":"5511999999999","Audio":"` + sendAudioTestURL + `","Caption":"legenda que morre aqui (F116)"}`
	rec := sendAudioServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("Caption preenchido causou rejeicao pela rota registrada — o contrato publico aceita o campo (F116): status=%d corpo=%s",
			rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false com Caption preenchido: %s", rec.Body.String())
	}
	if n := len(mm.SendAudioCalls); n != 1 {
		t.Fatalf("SendAudio chamado %d vez(es), quero 1 — o audio tem de ser enviado mesmo com Caption (F116)", n)
	}
	// AudioPayload não tem campo Caption — structuralmente impossível de
	// propagar. Se alguém acrescentar e ligar, a trava de wire
	// (TestSendWireContract_FieldNames) acusará a nova chave na resposta.
}
