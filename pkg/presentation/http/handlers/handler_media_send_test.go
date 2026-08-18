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

// Este arquivo cobre POST /chat/send/image desde a migração do CAP-02 para
// port.MediaMessenger/port.MediaFetcher — o handler que efetivamente busca a
// URL, sobe o anexo e entrega a imagem ao wa-noise, e não mais um
// "validated" sem fazer nada (o stub que este arquivo substitui).

const sendImageSentinelToken = "send-image-sentinel-cause-2b7c1a"

var errSendImageSentinel = errors.New(sendImageSentinelToken)

const sendImageTestURL = "https://exemplo.com/foto.png"

var sendImagePNGBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

// defaultSendImageFetcher devolve um MediaFetcher fake que já resolve com
// bytes PNG válidos — o caminho neutro para testes que não exercitam o
// próprio fetch.
func defaultSendImageFetcher() *contractsfake.MediaFetcher {
	return &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return sendImagePNGBytes, "image/png", nil
		},
	}
}

// sendImageRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendImageRouter(mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher) http.Handler {
	uc := message.NewSendImageUseCase(mm, jr, mf, silentLogger{})
	h := NewSendImageHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/image", h).Methods(http.MethodPost)
	return r
}

func sendImageServe(t *testing.T, mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/image", strings.NewReader(body))
	sendImageRouter(mm, jr, mf).ServeHTTP(rec, mut(req))
	return rec
}

// TestSendImage_Success_ViaRegisteredRoute prova o caminho HTTP -> handler
// -> usecase -> MediaFetcher.FetchBytes -> MediaMessenger.SendImage pela
// rota gorilla/mux REGISTRADA, com Status="sent", MessageID igual ao
// devolvido pela porta (não ao pedido) e Timestamp vindo do envio real.
func TestSendImage_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500003)
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999") {
				t.Errorf("target: got %q", target)
			}
			if string(payload.Bytes) != string(sendImagePNGBytes) {
				t.Errorf("bytes: divergem dos buscados")
			}
			if payload.Caption != "legenda" {
				t.Errorf("caption: got %q", payload.Caption)
			}
			return domain.MessageSendResult{ID: "wire-id-img-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendImageFetcher()

	body := `{"Phone":"5511999999999","Image":"` + sendImageTestURL + `","Caption":"legenda"}`
	rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data struct {
		MessageID string `json:"message_id"`
		Timestamp int64  `json:"timestamp"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-img-999" {
		t.Errorf("message_id: got %q, want %q (o que a porta devolveu)", data.MessageID, "wire-id-img-999")
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(mf.FetchBytesCalls); n != 1 {
		t.Fatalf("fetch chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if got := mf.FetchBytesCalls[0].ResourceURL; got != sendImageTestURL {
		t.Errorf("URL buscada: got %q, want %q", got, sendImageTestURL)
	}
	if n := len(mm.SendImageCalls); n != 1 {
		t.Fatalf("SendImage chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

// TestSendImage_RejectUnauthenticated: sem userInfo no contexto é 401 e nem
// fetch nem SendImage são tocados.
func TestSendImage_RejectUnauthenticated(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendImageFetcher()

	body := `{"Phone":"5511999999999","Image":"` + sendImageTestURL + `"}`
	rec := sendImageServe(t, mm, jr, mf, body, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou o fetch %d vez(es)", n)
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendImage %d vez(es)", n)
	}
}

// TestSendImage_RejectMissingRequiredField: Phone ou Image ausente é 400 e
// nem fetch nem SendImage são tocados.
func TestSendImage_RejectMissingRequiredField(t *testing.T) {
	bodies := map[string]string{
		"Phone": `{"Image":"` + sendImageTestURL + `"}`,
		"Image": `{"Phone":"5511999999999"}`,
	}
	for field, body := range bodies {
		t.Run(field, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendImageFetcher()

			rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload sem %s produziu status de sucesso %d", field, rec.Code)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Fatalf("payload invalido, mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendImageCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendImage foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendImage_DataURI_Success_ViaRegisteredRoute prova o SEGUNDO
// transporte de POST /chat/send/image (CAP-03) pela mesma superfície de
// rota do ramo URL: data URI chega decodificada localmente ao
// MediaMessenger — MediaFetcher nunca é tocado — com Status="sent" e
// MessageID vindo da porta de envio.
func TestSendImage_DataURI_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500020)
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999") {
				t.Errorf("target: got %q", target)
			}
			if payload.MimeType != "image/png" {
				t.Errorf("mimetype: got %q, want image/png", payload.MimeType)
			}
			return domain.MessageSendResult{ID: "wire-id-datauri-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendImageFetcher()

	encoded := base64.StdEncoding.EncodeToString(sendImagePNGBytes)
	body := `{"Phone":"5511999999999","Image":"data:image/png;base64,` + encoded + `","Caption":"legenda"}`
	rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data struct {
		MessageID string `json:"message_id"`
		Timestamp int64  `json:"timestamp"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-datauri-999" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "wire-id-datauri-999")
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("data URI, mas MediaFetcher foi chamado %d vez(es) pela rota registrada", n)
	}
	if n := len(mm.SendImageCalls); n != 1 {
		t.Fatalf("SendImage chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if string(mm.SendImageCalls[0].Payload.Bytes) != string(sendImagePNGBytes) {
		t.Error("bytes decodificados divergem dos originais")
	}
}

// TestSendImage_UnsupportedSource_Rejected: fontes que não são nem URL
// http(s) nem data URI de imagem — incluindo base64 cru sem prefixo
// "data:" e MIME não-imagem em data URI — viram erro pela rota registrada,
// nunca o 200 "validated" falso que o stub anterior produzia.
func TestSendImage_UnsupportedSource_Rejected(t *testing.T) {
	cases := map[string]string{
		"raw_base64_no_prefix": "/9j/4AAQSkZJRgABAQEASABIAAD=",
		"non_image_mime":       "data:application/pdf;base64,aGVsbG8=",
		"unsupported_scheme":   "ftp://exemplo.com/foto.png",
	}
	for name, img := range cases {
		t.Run(name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendImageFetcher()

			body := `{"Phone":"5511999999999","Image":"` + img + `"}`
			rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

			if rec.Code == http.StatusOK {
				t.Fatalf("fonte nao suportada %q produziu 200 falso: %s", img, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if env.Success {
				t.Fatalf("envelope.success=true para fonte nao suportada %q: %s", img, rec.Body.String())
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Fatalf("fonte nao suportada %q, mas o fetch foi chamado %d vez(es)", img, n)
			}
			if n := len(mm.SendImageCalls); n != 0 {
				t.Fatalf("fonte nao suportada %q, mas SendImage foi chamado %d vez(es)", img, n)
			}
		})
	}
}

// TestSendImage_SessionFailure: sessão inexistente vira erro (não 200) e
// nem fetch nem SendImage são chamados.
func TestSendImage_SessionFailure(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSendImageSentinel)}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendImageFetcher()

	body := `{"Phone":"5511999999999","Image":"` + sendImageTestURL + `"}`
	rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("sessao invalida, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_InvalidPhoneNeverFetchesOrSends: JID que não resolve nunca
// vira 200 nem toca o fetch/SendImage.
func TestSendImage_InvalidPhoneNeverFetchesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	mf := defaultSendImageFetcher()

	body := `{"Phone":"lixo","Image":"` + sendImageTestURL + `"}`
	rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("JID invalido, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_FetchFailure_NeverReturns200: falha ao buscar a URL (SSRF
// bloqueado, timeout, 404, corpo vazio, acima do limite, etc.) nunca produz
// 200 nem Status=sent.
func TestSendImage_FetchFailure_NeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("fetch: recusado")
		},
	}

	body := `{"Phone":"5511999999999","Image":"` + sendImageTestURL + `"}`
	rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no fetch produziu 200: %s", rec.Body.String())
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("fetch falhou, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_DownstreamFailureNeverReturns200: SendImage (upload e/ou
// envio) falhando NUNCA produz 200 nem Status=sent — a garantia central do
// CAP-02 contra falso-sucesso, pela rota registrada.
func TestSendImage_DownstreamFailureNeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(context.Context, string, domain.JID, domain.MediaPayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendImageSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendImageFetcher()

	body := `{"Phone":"5511999999999","Image":"` + sendImageTestURL + `"}`
	rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

// TestSendImage_ClientSuppliedIDIsForwardedButServerIDWins: o Id do cliente
// é repassado a SendImage, mas o message_id da resposta é o que a porta
// devolveu — nunca o do request usado às cegas.
func TestSendImage_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendImageFetcher()

	body := `{"Phone":"5511999999999","Image":"` + sendImageTestURL + `","Id":"id-do-cliente"}`
	rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendImage_InvalidMimeType_Rejected: bytes que não sniffam como imagem
// e nenhum MimeType de imagem no request são recusados pela rota registrada
// — não apenas no use case isolado.
func TestSendImage_InvalidMimeType_Rejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return []byte("%PDF-1.4 nao e imagem"), "application/pdf", nil
		},
	}

	body := `{"Phone":"5511999999999","Image":"` + sendImageTestURL + `"}`
	rec := sendImageServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("MIME nao-imagem produziu 200: %s", rec.Body.String())
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("MIME invalido, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_NoSecretLeak restaura o eixo perdido pela delecao de
// handler_media_test.go (a tabela sobre port.MessageComposer ficou vazia
// quando CAP-07 migrou o ultimo handler para longe dela — ver
// HOUSEKEEP.md). Phone, o campo Image e o header Authorization carregam os
// tres segredos da F9.4 (a sessao usa um id NAO secreto, de proposito: em
// producao ele nunca coincide com um dos tres); a sessao falha e o log de
// saida da rota REGISTRADA nao pode carregar nenhum dos tres.
func TestSendImage_NoSecretLeak(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-image-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendImageFetcher()

	wrapped, capture := logassert.Wrap(sendImageRouter(mm, jr, mf))

	body := `{"Phone":"` + logassertGlobalHMACKey + `","Image":"` + logassertGlobalEncryptionKey + `"}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/image", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}
