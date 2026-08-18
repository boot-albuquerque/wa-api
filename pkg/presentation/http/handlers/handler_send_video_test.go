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

// Este arquivo cobre POST /chat/send/video desde a migração do CAP-06 para
// port.MediaMessenger/port.MediaFetcher — o handler que efetivamente busca
// a URL ou decodifica a data URI, resolve o MIME pela precedência de dois
// níveis (igual a Image), sobe o anexo e entrega o vídeo ao wa-noise, e não
// mais um "validated" sem fazer nada. Mesma estrutura de
// handler_send_audio_test.go (POST /chat/send/audio, CAP-05).

const sendVideoSentinelToken = "send-video-sentinel-cause-9a2b6e"

var errSendVideoSentinel = errors.New(sendVideoSentinelToken)

const sendVideoTestURL = "https://exemplo.com/clipe.mp4"

// sendVideoMP4Bytes é binário (não texto) — usado como corpo de vídeo
// genérico nos testes de rota.
var sendVideoMP4Bytes = []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x01, 0x02, 0x03, 0x04}

func defaultSendVideoFetcher() *contractsfake.MediaFetcher {
	return &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return sendVideoMP4Bytes, "video/mp4", nil
		},
	}
}

// sendVideoRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendVideoRouter(mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher) http.Handler {
	uc := message.NewSendVideoUseCase(mm, jr, mf, silentLogger{})
	h := NewSendVideoHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/video", h).Methods(http.MethodPost)
	return r
}

func sendVideoServe(t *testing.T, mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/video", strings.NewReader(body))
	sendVideoRouter(mm, jr, mf).ServeHTTP(rec, mut(req))
	return rec
}

type sendVideoResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// TestSendVideo_Success_ViaRegisteredRoute prova o caminho HTTP -> handler
// -> usecase -> MediaFetcher.FetchBytes -> MediaMessenger.SendVideo pela
// rota gorilla/mux REGISTRADA (transporte URL), com Status="sent",
// MessageID igual ao devolvido pela porta e Timestamp vindo do envio real.
func TestSendVideo_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500110)
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999") {
				t.Errorf("target: got %q", target)
			}
			if string(payload.Bytes) != string(sendVideoMP4Bytes) {
				t.Errorf("bytes: divergem dos buscados")
			}
			return domain.MessageSendResult{ID: "wire-id-video-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendVideoFetcher()

	body := `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendVideoResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-video-999" {
		t.Errorf("message_id: got %q, want %q (o que a porta devolveu)", data.MessageID, "wire-id-video-999")
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(mf.FetchBytesCalls); n != 1 {
		t.Fatalf("fetch chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if got := mf.FetchBytesCalls[0].ResourceURL; got != sendVideoTestURL {
		t.Errorf("URL buscada: got %q, want %q", got, sendVideoTestURL)
	}
	if n := len(mm.SendVideoCalls); n != 1 {
		t.Fatalf("SendVideo chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

func TestSendVideo_RejectUnauthenticated(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendVideoFetcher()

	body := `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `"}`
	rec := sendVideoServe(t, mm, jr, mf, body, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou o fetch %d vez(es)", n)
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendVideo %d vez(es)", n)
	}
}

func TestSendVideo_RejectMissingRequiredField(t *testing.T) {
	bodies := map[string]string{
		"Phone": `{"Video":"` + sendVideoTestURL + `"}`,
		"Video": `{"Phone":"5511999999999"}`,
	}
	for field, body := range bodies {
		t.Run(field, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendVideoFetcher()

			rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload sem %s produziu status de sucesso %d", field, rec.Code)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Fatalf("payload invalido, mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendVideoCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendVideo foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendVideo_DataURI_Success_ViaRegisteredRoute prova o SEGUNDO
// transporte de POST /chat/send/video (data URI) pela mesma superfície de
// rota do ramo URL: data URI chega decodificada localmente ao
// MediaMessenger — MediaFetcher nunca é tocado.
func TestSendVideo_DataURI_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500120)
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999") {
				t.Errorf("target: got %q", target)
			}
			return domain.MessageSendResult{ID: "wire-id-video-datauri-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendVideoFetcher()

	encoded := base64.StdEncoding.EncodeToString(sendVideoMP4Bytes)
	body := `{"Phone":"5511999999999","Video":"data:video/mp4;base64,` + encoded + `"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendVideoResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-video-datauri-999" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "wire-id-video-datauri-999")
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("data URI, mas MediaFetcher foi chamado %d vez(es) pela rota registrada", n)
	}
	if n := len(mm.SendVideoCalls); n != 1 {
		t.Fatalf("SendVideo chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if string(mm.SendVideoCalls[0].Payload.Bytes) != string(sendVideoMP4Bytes) {
		t.Error("bytes decodificados divergem dos originais")
	}
}

// TestSendVideo_ShortDat_Rejected_NoPanic prova, pela rota registrada, que
// Video="dat" (3 caracteres, abaixo do prefixo "data" de 4) não panica e
// vira erro — o bug latente histórico (`t.Video[0:4]`) não é reproduzido.
func TestSendVideo_ShortDat_Rejected_NoPanic(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendVideoFetcher()

	body := `{"Phone":"5511999999999","Video":"dat"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("Video de 3 caracteres produziu 200 falso: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true para Video de 3 caracteres: %s", rec.Body.String())
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Fatalf("Video de 3 caracteres, mas SendVideo foi chamado %d vez(es)", n)
	}
}

// TestSendVideo_UnsupportedSource_Rejected: fontes que não são nem "data"
// (4+ chars) nem URL http(s) viram erro pela rota registrada, nunca o 200
// "validated" falso que o stub anterior produzia.
func TestSendVideo_UnsupportedSource_Rejected(t *testing.T) {
	cases := map[string]string{
		"raw_base64_no_prefix": "JVBERi0xLjQK",
		"unsupported_scheme":   "ftp://exemplo.com/clipe.mp4",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendVideoFetcher()

			body := `{"Phone":"5511999999999","Video":"` + doc + `"}`
			rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

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
			if n := len(mm.SendVideoCalls); n != 0 {
				t.Fatalf("fonte nao suportada %q, mas SendVideo foi chamado %d vez(es)", doc, n)
			}
		})
	}
}

func TestSendVideo_SessionFailure(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSendVideoSentinel)}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendVideoFetcher()

	body := `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("sessao invalida, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendVideo foi chamado %d vez(es)", n)
	}
}

func TestSendVideo_InvalidPhoneNeverFetchesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	mf := defaultSendVideoFetcher()

	body := `{"Phone":"lixo","Video":"` + sendVideoTestURL + `"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("JID invalido, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendVideo foi chamado %d vez(es)", n)
	}
}

func TestSendVideo_FetchFailure_NeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("fetch: recusado")
		},
	}

	body := `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no fetch produziu 200: %s", rec.Body.String())
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Fatalf("fetch falhou, mas SendVideo foi chamado %d vez(es)", n)
	}
}

// TestSendVideo_DownstreamFailureNeverReturns200: SendVideo (upload e/ou
// envio) falhando NUNCA produz 200 nem Status=sent — a garantia central do
// CAP-06 contra falso-sucesso, pela rota registrada.
func TestSendVideo_DownstreamFailureNeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(context.Context, string, domain.JID, domain.MediaPayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendVideoSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendVideoFetcher()

	body := `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

func TestSendVideo_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendVideoFetcher()

	body := `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `","Id":"id-do-cliente"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendVideoResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendVideo_Caption_ForwardedFromRequest pela rota registrada: Caption
// preenchido chega intacto ao payload de envio.
func TestSendVideo_Caption_ForwardedFromRequest(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(_ context.Context, _ string, _ domain.JID, payload domain.MediaPayload, _ string) (domain.MessageSendResult, error) {
			if payload.Caption != "legenda do clipe" {
				t.Errorf("Caption: got %q, want %q", payload.Caption, "legenda do clipe")
			}
			return domain.MessageSendResult{ID: "wire-id-caption"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendVideoFetcher()

	body := `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `","Caption":"legenda do clipe"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(mm.SendVideoCalls); n != 1 {
		t.Fatalf("SendVideo chamado %d vez(es), quero 1", n)
	}
}

// TestSendVideo_MimeType_SniffedFromBytes_ViaRegisteredRoute: pela rota
// registrada, sem Content-Type de vídeo remoto tendo precedência, o MIME
// final vem do sniffing dos bytes.
func TestSendVideo_MimeType_SniffedFromBytes_ViaRegisteredRoute(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(_ context.Context, _ string, _ domain.JID, payload domain.MediaPayload, _ string) (domain.MessageSendResult, error) {
			if payload.MimeType == "video/quicktime" {
				t.Errorf("mimetype: Content-Type remoto (%q) venceu, mas nao deveria ter precedencia", payload.MimeType)
			}
			return domain.MessageSendResult{ID: "wire-id-mime-sniff"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return sendVideoMP4Bytes, "video/quicktime", nil
		},
	}

	body := `{"Phone":"5511999999999","Video":"` + sendVideoTestURL + `"}`
	rec := sendVideoServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(mm.SendVideoCalls); n != 1 {
		t.Fatalf("SendVideo chamado %d vez(es), quero 1", n)
	}
}

// TestSendVideo_NoSecretLeak restaura o eixo perdido pela delecao de
// handler_media_test.go (a tabela sobre port.MessageComposer ficou vazia
// quando CAP-07 migrou o ultimo handler para longe dela — ver
// HOUSEKEEP.md). Phone, o campo Video e o header Authorization carregam os
// tres segredos da F9.4 (a sessao usa um id NAO secreto, de proposito: em
// producao ele nunca coincide com um dos tres); a sessao falha e o log de
// saida da rota REGISTRADA nao pode carregar nenhum dos tres.
func TestSendVideo_NoSecretLeak(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-video-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendVideoFetcher()

	wrapped, capture := logassert.Wrap(sendVideoRouter(mm, jr, mf))

	body := `{"Phone":"` + logassertGlobalHMACKey + `","Video":"` + logassertGlobalEncryptionKey + `"}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/video", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}
