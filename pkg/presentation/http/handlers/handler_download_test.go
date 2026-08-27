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

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// CAP-09B/CAP-10/F297: as cinco capabilities de download baixam de verdade.
// Este arquivo as exercita pela ROTA gorilla/mux REGISTRADA (ARMADILHA 2 do
// repo — defeito de rota só aparece pela rota, nunca por handler.ServeHTTP
// cru), com a mesma cadeia hlog que router.go instala.
//
// As cinco rotas legadas por-kind (/chat/downloadimage etc.) foram removidas
// em 2026-08-27 (HOUSEKEEP.md F297, corte-limpo explícito). Este arquivo
// exercita agora os mesmos oito eixos pela rota consolidada única,
// /chats/download/{kind}, variando o segmento {kind} — o handler e o use
// case por trás são os mesmos que os handlers removidos usavam
// (DownloadMediaUseCase apenas despacha para eles, ver
// download_media_unified.go).
//
// A tabela é ENUMERADA e cada coluna é verificável POR NOME: capability,
// media kind, MIME esperado e prefixo esperado de Data. Não há
// `for _, route := range downloadRoutes` anônimo — a duplicação entre as
// cinco cópias foi o defeito original.

// downloadRouteCase é uma das cinco capabilities na fronteira HTTP.
type downloadRouteCase struct {
	capability string           // nome da capability
	kind       domain.MediaKind // media kind que a porta tem de receber, e o segmento {kind} da rota
	mime       string           // MIME do payload e da resposta
	wantPrefix string           // prefixo esperado de Data
}

func downloadRouteCases() []downloadRouteCase {
	return []downloadRouteCase{
		{capability: "Download Image", kind: domain.MediaKindImage, mime: "image/jpeg", wantPrefix: "data:image/jpeg;base64,"},
		{capability: "Download Video", kind: domain.MediaKindVideo, mime: "video/mp4", wantPrefix: "data:video/mp4;base64,"},
		{capability: "Download Audio", kind: domain.MediaKindAudio, mime: "audio/ogg", wantPrefix: "data:audio/ogg;base64,"},
		{capability: "Download Document", kind: domain.MediaKindDocument, mime: "application/pdf", wantPrefix: "data:application/pdf;base64,"},
		{capability: "Download Sticker", kind: domain.MediaKindSticker, mime: "image/webp", wantPrefix: "data:image/webp;base64,"},
	}
}

// route é a rota consolidada real, com o {kind} desta capability já
// preenchido — exatamente o que um cliente chamaria.
func (c downloadRouteCase) route() string {
	return "/chats/download/" + string(c.kind)
}

// downloadRouter registra o DownloadMediaHandler na rota consolidada com
// padrão {kind}, como wiring_routes.go faz.
func (c downloadRouteCase) router(md appport.MediaDownloader) http.Handler {
	uc := message.NewDownloadMediaUseCase(
		message.NewDownloadImageUseCase(md, silentLogger{}),
		message.NewDownloadVideoUseCase(md, silentLogger{}),
		message.NewDownloadAudioUseCase(md, silentLogger{}),
		message.NewDownloadDocumentUseCase(md, silentLogger{}),
		message.NewDownloadStickerUseCase(md, silentLogger{}),
	)
	r := mux.NewRouter()
	r.Handle("/chats/download/{kind}", NewDownloadMediaHandler(uc)).Methods(http.MethodPost)
	return r
}

// body monta um payload com os SETE campos, com o MIME da capability. Os
// []byte vão em base64, que é como encoding/json os decodifica.
func (c downloadRouteCase) body() string {
	return `{"Url":"https://mmg.whatsapp.net/d/f/AbCdEf.enc",` +
		`"DirectPath":"/v/t62.7118-24/12345_678_90.enc",` +
		`"MediaKey":"` + base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04}) + `",` +
		`"Mimetype":"` + c.mime + `",` +
		`"FileEncSHA256":"` + base64.StdEncoding.EncodeToString([]byte{0xaa, 0xbb}) + `",` +
		`"FileSHA256":"` + base64.StdEncoding.EncodeToString([]byte{0xcc, 0xdd}) + `",` +
		`"FileLength":4242}`
}

// downloadServe executa a requisição pela rota registrada.
func (c downloadRouteCase) serve(md appport.MediaDownloader, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, c.route(), strings.NewReader(body))
	c.router(md).ServeHTTP(rec, mut(req))
	return rec
}

// downloadServeCapturingLog é o serve com a saída de log da requisição, para
// asseverar a CAUSA (co-gate D) e a ausência de segredo.
func (c downloadRouteCase) serveCapturingLog(t *testing.T, md appport.MediaDownloader, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(c.router(md))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, c.route(), strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

type downloadResultBody struct {
	Mimetype string `json:"Mimetype"`
	Data     string `json:"Data"`
}

// TestDownload_Success_ViaRegisteredRoute prova o caminho HTTP -> handler ->
// use case -> MediaDownloader.Download pela rota REGISTRADA, para cada uma das
// cinco capabilities: kind correto, MIME correto, Data com o prefixo esperado
// e carregando os bytes reais.
func TestDownload_Success_ViaRegisteredRoute(t *testing.T) {
	payload := []byte{0x00, 0x01, 0xff, 0xfe, 'o', 'k'}

	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(_ context.Context, txtID string, desc domain.MediaDescriptor) ([]byte, error) {
					if txtID != "user-1" {
						t.Errorf("txtID: got %q, want %q (o Id do contexto)", txtID, "user-1")
					}
					if desc.Kind != c.kind {
						t.Errorf("Kind: got %q, want %q", desc.Kind, c.kind)
					}
					if desc.URL != "https://mmg.whatsapp.net/d/f/AbCdEf.enc" {
						t.Errorf("URL: got %q", desc.URL)
					}
					if desc.DirectPath != "/v/t62.7118-24/12345_678_90.enc" {
						t.Errorf("DirectPath: got %q", desc.DirectPath)
					}
					if string(desc.MediaKey) != string([]byte{0x01, 0x02, 0x03, 0x04}) {
						t.Errorf("MediaKey: got %x", desc.MediaKey)
					}
					if desc.Mimetype != c.mime {
						t.Errorf("Mimetype: got %q, want %q", desc.Mimetype, c.mime)
					}
					if string(desc.FileEncSHA256) != string([]byte{0xaa, 0xbb}) {
						t.Errorf("FileEncSHA256: got %x", desc.FileEncSHA256)
					}
					if string(desc.FileSHA256) != string([]byte{0xcc, 0xdd}) {
						t.Errorf("FileSHA256: got %x", desc.FileSHA256)
					}
					if desc.FileLength != 4242 {
						t.Errorf("FileLength: got %d, want 4242", desc.FileLength)
					}
					return payload, nil
				},
			}

			rec := c.serve(md, c.body(), msgAuthed)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status %d (corpo: %s)", c.route(), rec.Code, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if !env.Success {
				t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
			}

			var data downloadResultBody
			if err := json.Unmarshal(env.Data, &data); err != nil {
				t.Fatalf("envelope.data invalido: %v", err)
			}
			if data.Mimetype != c.mime {
				t.Errorf("Mimetype: got %q, want %q", data.Mimetype, c.mime)
			}
			if !strings.HasPrefix(data.Data, c.wantPrefix) {
				t.Fatalf("Data: got %q, want prefixo %q", data.Data, c.wantPrefix)
			}
			// Mimetype e o prefixo de Data TÊM de concordar.
			if got := strings.TrimSuffix(strings.TrimPrefix(c.wantPrefix, "data:"), ";base64,"); got != data.Mimetype {
				t.Errorf("Data anuncia %q, campo Mimetype diz %q", got, data.Mimetype)
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data.Data, c.wantPrefix))
			if err != nil {
				t.Fatalf("payload de Data nao e' base64 valido: %v", err)
			}
			if string(decoded) != string(payload) {
				t.Errorf("Data decodificado: got %x, want %x", decoded, payload)
			}
			if n := len(md.DownloadCalls); n != 1 {
				t.Fatalf("Download chamado %d vez(es) pela rota registrada, quero 1", n)
			}
		})
	}
}

// TestDownload_RejectUnauthenticated: sem o middleware de auth, 401 e a porta
// NÃO é alcançada.
func TestDownload_RejectUnauthenticated(t *testing.T) {
	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{}

			rec := c.serve(md, c.body(), func(r *http.Request) *http.Request { return r })

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			if n := len(md.EnsureSessionCalls); n != 0 {
				t.Errorf("requisicao nao autenticada alcancou EnsureSession %d vez(es)", n)
			}
			if n := len(md.DownloadCalls); n != 0 {
				t.Errorf("requisicao nao autenticada alcancou Download %d vez(es)", n)
			}
		})
	}
}

// TestDownload_RejectEmptySessionID: userinfo presente mas sem Id -> 400.
func TestDownload_RejectEmptySessionID(t *testing.T) {
	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{}

			rec := c.serve(md, c.body(), func(r *http.Request) *http.Request { return withUser(r, "") })

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			if n := len(md.DownloadCalls); n != 0 {
				t.Errorf("sem Id, Download foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestDownload_RejectMalformedBody: corpo ilegível é rejeição de CLIENTE (400)
// e tem de sair registrada em warn com a causa e o req_id.
func TestDownload_RejectMalformedBody(t *testing.T) {
	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{}

			rec, recs := c.serveCapturingLog(t, md, `{"Url":`, msgAuthed)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			if n := len(md.EnsureSessionCalls); n != 0 {
				t.Errorf("corpo ilegivel alcancou EnsureSession %d vez(es)", n)
			}
			got := logassert.OutcomeLogged(t, recs)
			if got.str("level") != "warn" {
				t.Errorf("payload ilegivel e' rejeicao de cliente: nivel %q", got.str("level"))
			}
		})
	}
}

// TestDownload_RejectMissingRequiredField: Url E DirectPath ambos ausentes é
// 400 (erro do cliente, não 500), e a porta não é consultada. Antes da F127
// bastava Url ausente; a primitive aceita DirectPath sozinho.
func TestDownload_RejectMissingRequiredField(t *testing.T) {
	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{}

			rec, recs := c.serveCapturingLog(t, md, `{"Mimetype":"`+c.mime+`"}`, msgAuthed)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			if n := len(md.EnsureSessionCalls); n != 0 {
				t.Errorf("use case checou sessao antes de validar o payload (%d chamada(s))", n)
			}
			if n := len(md.DownloadCalls); n != 0 {
				t.Errorf("payload invalido alcancou Download %d vez(es)", n)
			}
			logassert.OutcomeLogged(t, recs)
		})
	}
}

// TestDownload_DirectPathOnly_Accepted: payload com DirectPath e sem Url é
// aceito pela rota registrada. A primitive do SDK baixa por DirectPath
// sozinho (download.go:82), então a rota não pode recusar.
func TestDownload_DirectPathOnly_Accepted(t *testing.T) {
	payload := []byte{0x00, 0x01, 0xff, 0xfe}

	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(_ context.Context, _ string, _ domain.MediaDescriptor) ([]byte, error) {
					return payload, nil
				},
			}

			body := `{"DirectPath":"/v/t62.7118-24/12345_678_90.enc",` +
				`"MediaKey":"` + base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04}) + `",` +
				`"Mimetype":"` + c.mime + `",` +
				`"FileEncSHA256":"` + base64.StdEncoding.EncodeToString([]byte{0xaa, 0xbb}) + `",` +
				`"FileSHA256":"` + base64.StdEncoding.EncodeToString([]byte{0xcc, 0xdd}) + `",` +
				`"FileLength":4242}`

			rec := c.serve(md, body, msgAuthed)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status %d (corpo: %s)", c.route(), rec.Code, rec.Body.String())
			}
			if n := len(md.DownloadCalls); n != 1 {
				t.Fatalf("Download chamado %d vez(es) pela rota registrada, quero 1", n)
			}
			got := md.DownloadCalls[0].Descriptor
			if got.DirectPath != "/v/t62.7118-24/12345_678_90.enc" {
				t.Errorf("DirectPath: got %q", got.DirectPath)
			}
			if got.URL != "" {
				t.Errorf("URL devia ser vazio, got %q", got.URL)
			}
		})
	}
}

// TestDownload_SessionFailure_500: a sessão recusa, o use case propaga, o
// handler responde 500 logando a causa em error, e o download não acontece.
func TestDownload_SessionFailure_500(t *testing.T) {
	const cause = "download-session-refused"

	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{SessionGuard: contractsfake.FailSession(errors.New(cause))}

			rec, recs := c.serveCapturingLog(t, md, c.body(), msgAuthed)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			if n := len(md.DownloadCalls); n != 0 {
				t.Fatalf("sessao recusada mas Download foi chamado %d vez(es)", n)
			}
			got := logassert.OutcomeLogged(t, recs, cause)
			if got.str("level") != "error" {
				t.Errorf("falha real de sessao tem de ser error, foi %q", got.str("level"))
			}
		})
	}
}

// TestDownload_DownloaderFailure_NotOK é o eixo que o estado anterior a
// CAP-09B não podia ter: a porta baixa e FALHA. Isso não pode virar 200 —
// nem 200 com corpo vazio.
func TestDownload_DownloaderFailure_NotOK(t *testing.T) {
	const cause = "downloader-refused-by-sdk"

	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(context.Context, string, domain.MediaDescriptor) ([]byte, error) {
					return nil, errors.New(cause)
				},
			}

			rec, recs := c.serveCapturingLog(t, md, c.body(), msgAuthed)

			if rec.Code == http.StatusOK {
				t.Fatalf("falha do downloader virou 200: %s", rec.Body.String())
			}
			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			got := logassert.OutcomeLogged(t, recs, cause)
			if got.str("level") != "error" {
				t.Errorf("falha de download tem de ser error, foi %q", got.str("level"))
			}
		})
	}
}

// TestDownload_EmptyBytes_NotOK trava na fronteira HTTP a decisão sobre bytes
// vazios com erro nil (HOUSEKEEP.md F126): 500, não 200 com
// `"Data":"data:image/jpeg;base64,"` como o fluxo histórico respondia.
func TestDownload_EmptyBytes_NotOK(t *testing.T) {
	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(context.Context, string, domain.MediaDescriptor) ([]byte, error) {
					return []byte{}, nil
				},
			}

			rec := c.serve(md, c.body(), msgAuthed)

			if rec.Code == http.StatusOK {
				t.Fatalf("download vazio virou 200: %s", rec.Body.String())
			}
			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
		})
	}
}

// TestDownload_NoSecretLeak planta os três segredos da F9.4 no caminho do
// handler — corpo, Id de sessão e cabeçalho — e exige que o log de saída não
// os carregue.
func TestDownload_NoSecretLeak(t *testing.T) {
	for _, c := range downloadRouteCases() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{SessionGuard: contractsfake.FailSession(errors.New("download-session-refused"))}

			body := `{"Url":"https://example.invalid/` + logassertGlobalEncryptionKey + `"}`
			mut := func(r *http.Request) *http.Request {
				r.Header.Set("Authorization", logassertAdminToken)
				return withUser(r, logassertGlobalHMACKey)
			}

			rec, recs := c.serveCapturingLog(t, md, body, mut)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.NoSecrets(t, recs)
		})
	}
}
