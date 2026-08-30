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

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// Este arquivo cobre POST /chat/send/sticker desde a migração do CAP-07
// para port.MediaMessenger/port.MediaFetcher/port.StickerProcessor — o
// handler que efetivamente busca a URL (normalizando para data URI) ou
// aceita a data URI do cliente, converte via appport.StickerProcessor
// (pipeline real de pkg/infra/media/sticker por trás do fake nestes
// testes), sobe o WebP CONVERTIDO e entrega o sticker ao noise — não
// mais um "validated" sem fazer nada. Mesma estrutura de
// handler_send_video_test.go (POST /chat/send/video, CAP-06).

var errSendStickerSentinel = errors.New("send-sticker-sentinel-cause-7c4f1a")

const sendStickerTestURL = "https://exemplo.com/figurinha.webp"

// sendStickerWebPBytes é o corpo bruto (não convertido) de um sticker nos
// testes de rota.
var sendStickerWebPBytes = []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00}

// sendStickerProcessedBytes é o que o StickerProcessor fake devolve — usado
// para provar que é ISSO que sobe, não sendStickerWebPBytes.
var sendStickerProcessedBytes = []byte{0xDE, 0xAD, 0xBE, 0xEF}

const sendStickerProcessedMime = "image/webp"

func defaultSendStickerFetcher() *contractsfake.MediaFetcher {
	return &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return sendStickerWebPBytes, "image/webp", nil
		},
	}
}

func defaultSendStickerProcessor() *contractsfake.StickerProcessor {
	return &contractsfake.StickerProcessor{
		ProcessStickerFunc: func(context.Context, string, string, string, string, string, []string) ([]byte, string, error) {
			return sendStickerProcessedBytes, sendStickerProcessedMime, nil
		},
	}
}

// sendStickerRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendStickerRouter(mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher, sp *contractsfake.StickerProcessor) http.Handler {
	uc := message.NewSendStickerUseCase(mm, jr, mf, sp, silentLogger{})
	h := NewSendStickerHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/sticker", h).Methods(http.MethodPost)
	return r
}

func sendStickerServe(t *testing.T, mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher, sp *contractsfake.StickerProcessor, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/sticker", strings.NewReader(body))
	sendStickerRouter(mm, jr, mf, sp).ServeHTTP(rec, mut(req))
	return rec
}

type sendStickerResultBody struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// TestSendSticker_Success_ViaRegisteredRoute prova o caminho HTTP -> handler
// -> usecase -> MediaFetcher.FetchBytes -> StickerProcessor.ProcessSticker
// -> MediaMessenger.SendSticker pela rota gorilla/mux REGISTRADA (transporte
// URL), com Status="sent", MessageID igual ao devolvido pela porta, e os
// bytes/MIME que chegam a SendSticker são os PROCESSADOS, não os buscados.
func TestSendSticker_Success_ViaRegisteredRoute(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendStickerFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999@s.whatsapp.net") {
				t.Errorf("target: got %q", target)
			}
			if string(payload.Bytes) != string(sendStickerProcessedBytes) {
				t.Errorf("bytes: chegaram os originais/buscados, nao os processados")
			}
			if payload.MimeType != sendStickerProcessedMime {
				t.Errorf("mimetype: got %q, want %q (o que o pipeline devolveu)", payload.MimeType, sendStickerProcessedMime)
			}
			return domain.MessageSendResult{ID: "wire-id-sticker-999"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendStickerFetcher()
	sp := defaultSendStickerProcessor()

	body := `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendStickerResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-sticker-999" {
		t.Errorf("message_id: got %q, want %q (o que a porta devolveu)", data.MessageID, "wire-id-sticker-999")
	}
	if n := len(mf.FetchBytesCalls); n != 1 {
		t.Fatalf("fetch chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if got := mf.FetchBytesCalls[0].ResourceURL; got != sendStickerTestURL {
		t.Errorf("URL buscada: got %q, want %q", got, sendStickerTestURL)
	}
	if n := len(sp.ProcessStickerCalls); n != 1 {
		t.Fatalf("ProcessSticker chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if got := sp.ProcessStickerCalls[0].DataURI; !strings.HasPrefix(got, "data:image/webp;base64,") {
		t.Errorf("stickerData recebida por ProcessSticker nao foi normalizada para data URI: %q", got)
	}
	if n := len(mm.SendStickerCalls); n != 1 {
		t.Fatalf("SendSticker chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

func TestSendSticker_RejectUnauthenticated(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendStickerFetcher()
	sp := defaultSendStickerProcessor()

	body := `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou o fetch %d vez(es)", n)
	}
	if n := len(mm.SendStickerCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendSticker %d vez(es)", n)
	}
}

func TestSendSticker_RejectMissingRequiredField(t *testing.T) {
	bodies := map[string]string{
		"phone":   `{"sticker":"` + sendStickerTestURL + `"}`,
		"sticker": `{"phone":"5511999999999"}`,
	}
	for field, body := range bodies {
		t.Run(field, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendStickerFetcher()
			sp := defaultSendStickerProcessor()

			rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload sem %s produziu status de sucesso %d", field, rec.Code)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Fatalf("payload invalido, mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendStickerCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendSticker foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendSticker_DataURI_Success_ViaRegisteredRoute prova o SEGUNDO
// transporte de POST /chat/send/sticker (data URI) pela mesma superfície de
// rota do ramo URL: data URI chega intacta ao StickerProcessor —
// MediaFetcher nunca é tocado.
func TestSendSticker_DataURI_Success_ViaRegisteredRoute(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendStickerFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999@s.whatsapp.net") {
				t.Errorf("target: got %q", target)
			}
			return domain.MessageSendResult{ID: "wire-id-sticker-datauri-999"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendStickerFetcher()
	sp := defaultSendStickerProcessor()

	encoded := base64.StdEncoding.EncodeToString(sendStickerWebPBytes)
	rawDataURI := "data:image/webp;base64," + encoded
	body := `{"phone":"5511999999999","sticker":"` + rawDataURI + `"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendStickerResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "wire-id-sticker-datauri-999" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "wire-id-sticker-datauri-999")
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("data URI, mas MediaFetcher foi chamado %d vez(es) pela rota registrada", n)
	}
	if n := len(sp.ProcessStickerCalls); n != 1 {
		t.Fatalf("ProcessSticker chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if got := sp.ProcessStickerCalls[0].DataURI; got != rawDataURI {
		t.Errorf("stickerData divergiu do que o cliente mandou: got %q, want %q", got, rawDataURI)
	}
}

// TestSendSticker_UnsupportedSource_Rejected: fontes que não são nem "data"
// (4+ chars) nem URL http(s) — aqui, o ProcessSticker fake reproduz a
// rejeição real do pacote infra ("data should start with...") — viram erro
// pela rota registrada, nunca o 200 "validated" falso que o stub anterior
// produzia.
func TestSendSticker_UnsupportedSource_Rejected(t *testing.T) {
	cases := map[string]string{
		"raw_base64_no_prefix": "AAAA",
		"unsupported_scheme":   "ftp://exemplo.com/figurinha.webp",
	}
	for name, sticker := range cases {
		t.Run(name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendStickerFetcher()
			sp := &contractsfake.StickerProcessor{
				ProcessStickerFunc: func(context.Context, string, string, string, string, string, []string) ([]byte, string, error) {
					return nil, "", errors.New(`data should start with "data:mime/type;base64,"`)
				},
			}

			body := `{"phone":"5511999999999","sticker":"` + sticker + `"}`
			rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

			if rec.Code == http.StatusOK {
				t.Fatalf("fonte nao suportada %q produziu 200 falso: %s", sticker, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if env.Success {
				t.Fatalf("envelope.success=true para fonte nao suportada %q: %s", sticker, rec.Body.String())
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Fatalf("fonte nao suportada %q, mas o fetch foi chamado %d vez(es)", sticker, n)
			}
			if n := len(mm.SendStickerCalls); n != 0 {
				t.Fatalf("fonte nao suportada %q, mas SendSticker foi chamado %d vez(es)", sticker, n)
			}
		})
	}
}

func TestSendSticker_SessionFailure(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSendStickerSentinel)}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendStickerFetcher()
	sp := defaultSendStickerProcessor()

	body := `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("sessao invalida, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendStickerCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendSticker foi chamado %d vez(es)", n)
	}
}

func TestSendSticker_InvalidPhoneNeverFetchesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	mf := defaultSendStickerFetcher()
	sp := defaultSendStickerProcessor()

	body := `{"phone":"lixo","sticker":"` + sendStickerTestURL + `"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("JID invalido, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendStickerCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendSticker foi chamado %d vez(es)", n)
	}
}

func TestSendSticker_FetchFailure_NeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("fetch: recusado")
		},
	}
	sp := defaultSendStickerProcessor()

	body := `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no fetch produziu 200: %s", rec.Body.String())
	}
	if n := len(mm.SendStickerCalls); n != 0 {
		t.Fatalf("fetch falhou, mas SendSticker foi chamado %d vez(es)", n)
	}
}

// TestSendSticker_ConversionFailure_Returns500_NeverSent prova o item 4 do
// packet: falha de conversao (ffmpeg ausente/erro, aqui simulada pelo fake
// StickerProcessor com a string sentinela "failed to convert") vira 500,
// NUNCA 200/"sent" — pela rota registrada.
func TestSendSticker_ConversionFailure_Returns500_NeverSent(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendStickerFetcher()
	sp := &contractsfake.StickerProcessor{
		ProcessStickerFunc: func(context.Context, string, string, string, string, string, []string) ([]byte, string, error) {
			return nil, "", errors.New("failed to convert image sticker to webp: exec: \"ffmpeg\": executable file not found in $PATH")
		},
	}

	body := `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d (falha de conversao/ffmpeg e' erro de servidor)", rec.Code, http.StatusInternalServerError)
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com falha de conversao: %s", rec.Body.String())
	}
	if n := len(mm.SendStickerCalls); n != 0 {
		t.Fatalf("conversao falhou, mas SendSticker foi chamado %d vez(es)", n)
	}
}

// TestSendSticker_DownstreamFailureNeverReturns200: SendSticker (upload
// e/ou envio) falhando NUNCA produz 200 nem Status=sent — a garantia
// central contra falso-sucesso, pela rota registrada.
func TestSendSticker_DownstreamFailureNeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendStickerFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendStickerSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendStickerFetcher()
	sp := defaultSendStickerProcessor()

	body := `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

func TestSendSticker_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendStickerFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendStickerFetcher()
	sp := defaultSendStickerProcessor()

	body := `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `","id":"id-do-cliente"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendStickerResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendSticker_MimeOverride_ForwardedFromRequest pela rota registrada:
// MimeType do request chega intacto a ProcessSticker como mimeOverride
// (parâmetro 2) — não é sniffado nem descartado pelo handler.
func TestSendSticker_MimeOverride_ForwardedFromRequest(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendStickerFetcher()
	sp := &contractsfake.StickerProcessor{
		ProcessStickerFunc: func(_ context.Context, dataURI, mimeOverride, _, _, _ string, _ []string) ([]byte, string, error) {
			if mimeOverride != "image/png" {
				t.Errorf("mimeOverride: got %q, want %q", mimeOverride, "image/png")
			}
			return sendStickerProcessedBytes, sendStickerProcessedMime, nil
		},
	}

	body := `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `","mime_type":"image/png"}`
	rec := sendStickerServe(t, mm, jr, mf, sp, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(sp.ProcessStickerCalls); n != 1 {
		t.Fatalf("ProcessSticker chamado %d vez(es), quero 1", n)
	}
}

// TestSendSticker_NoSecretLeak restaura o eixo perdido pela delecao de
// handler_media_test.go (a tabela sobre port.MessageComposer ficou vazia
// quando CAP-07 migrou o sticker, o ultimo handler que ainda estava sobre
// ela, para longe dela — ver HOUSEKEEP.md). Phone, o campo Sticker e o
// header Authorization carregam os tres segredos da F9.4 (a sessao usa um
// id NAO secreto, de proposito: em producao ele nunca coincide com um dos
// tres); a sessao falha e o log de saida da rota REGISTRADA nao pode
// carregar nenhum dos tres.
func TestSendSticker_NoSecretLeak(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-sticker-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendStickerFetcher()
	sp := defaultSendStickerProcessor()

	wrapped, capture := logassert.Wrap(sendStickerRouter(mm, jr, mf, sp))

	body := `{"phone":"` + logassertGlobalHMACKey + `","sticker":"` + logassertGlobalEncryptionKey + `"}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/sticker", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}
