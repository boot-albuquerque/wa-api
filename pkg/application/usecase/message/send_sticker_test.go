package message_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/media/opengraph"
)

const stickerURL = "https://exemplo.com/figurinha.webp"

// webpBytes é um corpo binário genérico usado como corpo de sticker (não
// precisa ser um WebP válido — o fake StickerProcessor não decodifica).
var webpBytes = []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00}

func stickerDataURI(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// statusOf devolve o HTTP status que o boundary daria a err — a mesma
// tradução que pkg/presentation/http/response.go faz via
// errors.As(err, &appErr); appErr.Category.HTTPStatus().
func statusOf(t *testing.T, err error) int {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro nao e' um *apperr.AppError: %#v", err)
	}
	return appErr.Category.HTTPStatus()
}

// --- validação de campos obrigatórios ------------------------------------

func TestSendSticker_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendStickerRequest
	}{
		{"Phone", domain.SendStickerRequest{Sticker: stickerURL}},
		{"Sticker", domain.SendStickerRequest{Phone: "5511987654321"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			sp := &contractsfake.StickerProcessor{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request sem %s foi aceito", tc.name)
			}
			if n := len(mm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(sp.ProcessStickerCalls); n != 0 {
				t.Errorf("validacao falhou mas ProcessSticker foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendStickerCalls); n != 0 {
				t.Errorf("validacao falhou mas SendSticker foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendSticker_SessionFailurePropagates(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: stickerURL})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(sp.ProcessStickerCalls); n != 0 {
		t.Errorf("sem sessao, mas ProcessSticker foi chamado %d vez(es)", n)
	}
}

func TestSendSticker_InvalidPhoneNeverProcessesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendStickerUseCase(mm, jr, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "lixo", Sticker: stickerURL})

	if err == nil {
		t.Fatal("JID invalido foi aceito")
	}
	if n := len(sp.ProcessStickerCalls); n != 0 {
		t.Errorf("JID invalido, mas ProcessSticker foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendStickerCalls); n != 0 {
		t.Errorf("JID invalido, mas SendSticker foi chamado %d vez(es)", n)
	}
}

// --- discriminação de transporte (URL -> data URI; senão, deixa o pipeline recusar) ---

func TestSendSticker_UnsupportedSource_LetsProcessStickerReject(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{
		ProcessStickerFunc: func(context.Context, string, string, string, string, string, []string) ([]byte, string, error) {
			return nil, "", errors.New(`data should start with "data:mime/type;base64,"`)
		},
	}
	logger := &contractsfake.Logger{}

	cases := map[string]string{
		"raw_base64_no_prefix": "AAAA",
		"unsupported_scheme":   "ftp://exemplo.com/figurinha.webp",
	}
	for name, sticker := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
				Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: sticker})

			if err == nil {
				t.Fatalf("fonte nao suportada %q foi aceita", sticker)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Errorf("fonte nao suportada, mas o fetch foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendSticker_URLBranch_NormalizesToDataURI prova o item 2 do packet: o
// ramo URL busca os bytes e normaliza para dentro de uma data URI ANTES de
// chamar ProcessSticker — stickerData que chega ao processor não é a URL
// original, é "data:<mime>;base64,<bytes buscados>".
func TestSendSticker_URLBranch_NormalizesToDataURI(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return webpBytes, "image/webp", nil
		},
	}
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: stickerURL})

	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}
	if n := len(sp.ProcessStickerCalls); n != 1 {
		t.Fatalf("ProcessSticker chamado %d vez(es), quero 1", n)
	}
	got := sp.ProcessStickerCalls[0].DataURI
	if !strings.HasPrefix(got, "data:image/webp;base64,") {
		t.Errorf("stickerData nao foi normalizado para data URI: %q", got)
	}
}

// TestSendSticker_URLBranch_NonImageContentType_FallsBackToWebP prova o
// fallback do item 2: Content-Type remoto que não começa com "image/" vira
// "image/webp" na data URI normalizada.
func TestSendSticker_URLBranch_NonImageContentType_FallsBackToWebP(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return webpBytes, "application/octet-stream", nil
		},
	}
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: stickerURL})

	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}
	got := sp.ProcessStickerCalls[0].DataURI
	if !strings.HasPrefix(got, "data:image/webp;base64,") {
		t.Errorf("Content-Type nao-imagem nao caiu para o fallback image/webp: %q", got)
	}
}

// TestSendSticker_DataURIBranch_PassedThroughUnchanged prova o item 2: uma
// data URI que já vem do cliente é passada tal e qual (sem normalização,
// sem fetch) para ProcessSticker.
func TestSendSticker_DataURIBranch_PassedThroughUnchanged(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	raw := stickerDataURI("image/webp", webpBytes)
	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: raw})

	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Errorf("data URI, mas o fetch foi chamado %d vez(es)", n)
	}
	if got := sp.ProcessStickerCalls[0].DataURI; got != raw {
		t.Errorf("stickerData divergiu do que o cliente mandou: got %q, want %q", got, raw)
	}
}

// TestSendSticker_DataURIBranch_OverLimit_Rejected_ViaExecute cobre o
// branch de checkStickerDataURISize DENTRO de Execute (não só a função pura
// — já provada em isolamento com limite sintético em
// send_sticker_internal_test.go). A string base64 é construída por
// repetição de um bloco curto (strings.Repeat), sem decodificar nem alocar
// um payload de mídia de verdade — só um texto longo o bastante para a
// ESTIMATIVA (decodedBase64Len, que só olha o comprimento) acusar mais de
// 16MB decodificados.
func TestSendSticker_DataURIBranch_OverLimit_Rejected_ViaExecute(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	// (16MB+4)/3*4 caracteres base64 (múltiplo de 4) decodificam para
	// pouco mais de 16MB — acima de fetchImageMaxBytes.
	const overLimitEncodedLen = ((16*1024*1024 + 4) / 3) * 4
	raw := "data:image/webp;base64," + strings.Repeat("A", overLimitEncodedLen)

	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: raw})

	if err == nil {
		t.Fatal("data URI acima de 16MB foi aceita")
	}
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", got, http.StatusBadRequest)
	}
	if n := len(sp.ProcessStickerCalls); n != 0 {
		t.Errorf("payload acima do limite, mas ProcessSticker foi chamado %d vez(es)", n)
	}
}

func TestSendSticker_FetchFailurePropagates(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("fetch: recusado")
		},
	}
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: stickerURL})

	if err == nil {
		t.Fatal("falha no fetch foi engolida")
	}
	if n := len(sp.ProcessStickerCalls); n != 0 {
		t.Errorf("fetch falhou, mas ProcessSticker foi chamado %d vez(es)", n)
	}
}

// --- item 4/7 do packet: mapeamento de erro de ProcessSticker -------------

// TestSendSticker_ConversionFailure_Maps500 prova que um erro contendo
// "failed to convert" (a string sentinela que
// pkg/infra/media/sticker/exif.go usa para os dois wraps de erro de ffmpeg)
// vira CategoryInternal (500) — a mesma distinção do handlers.go histórico.
func TestSendSticker_ConversionFailure_Maps500(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{
		ProcessStickerFunc: func(context.Context, string, string, string, string, string, []string) ([]byte, string, error) {
			return nil, "", errors.New("failed to convert image sticker to webp: exec: \"ffmpeg\": executable file not found in $PATH")
		},
	}
	logger := &contractsfake.Logger{}

	raw := stickerDataURI("image/png", webpBytes)
	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: raw})

	if err == nil {
		t.Fatal("falha de conversao foi engolida")
	}
	if got := statusOf(t, err); got != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d (falha de conversao/ffmpeg e' erro de servidor)", got, http.StatusInternalServerError)
	}
	if n := len(mm.SendStickerCalls); n != 0 {
		t.Errorf("conversao falhou, mas SendSticker foi chamado %d vez(es)", n)
	}
}

// TestSendSticker_OtherProcessError_Maps400 prova que um erro de
// ProcessSticker SEM a string "failed to convert" (data URI malformada,
// base64 invalido) vira CategoryValidation (400) — o cliente pode corrigir
// o payload.
func TestSendSticker_OtherProcessError_Maps400(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{
		ProcessStickerFunc: func(context.Context, string, string, string, string, string, []string) ([]byte, string, error) {
			return nil, "", errors.New("could not decode base64 encoded data from payload")
		},
	}
	logger := &contractsfake.Logger{}

	raw := stickerDataURI("image/png", webpBytes)
	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: raw})

	if err == nil {
		t.Fatal("falha de processamento foi engolida")
	}
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", got, http.StatusBadRequest)
	}
}

// --- item 3 do packet: upload com bytes/mime PROCESSADOS, não crus --------

// TestSendSticker_CausalSuccess prova a cadeia causal completa: aquisição
// (data URI direta, sem fetch) -> ProcessSticker chamado com o mimeOverride
// certo (req.MimeType) -> bytes/MIME PROCESSADOS (não os originais) vão
// para SendSticker -> resultado publico vem do SDK (sent.ID), Status=sent.
func TestSendSticker_CausalSuccess(t *testing.T) {
	processedBytes := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE}
	const processedMime = "image/webp"

	mm := &contractsfake.MediaMessenger{
		SendStickerFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999@s.whatsapp.net") {
				return domain.MessageSendResult{}, errors.New("target inesperado")
			}
			if string(payload.Bytes) != string(processedBytes) {
				return domain.MessageSendResult{}, errors.New("bytes originais chegaram ao envio, nao os processados")
			}
			if payload.MimeType != processedMime {
				return domain.MessageSendResult{}, errors.New("mimetype nao veio do pipeline de conversao")
			}
			return domain.MessageSendResult{ID: "wire-id-sticker-999"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(_ context.Context, phone string) (domain.JID, error) {
			if phone != "5511999999999" {
				t.Errorf("phone passado ao resolver: got %q", phone)
			}
			return domain.JID("5511999999999@s.whatsapp.net"), nil
		},
	}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{
		ProcessStickerFunc: func(_ context.Context, dataURI, mimeOverride, packID, packName, packPublisher string, emojis []string) ([]byte, string, error) {
			if mimeOverride != "image/png" {
				t.Errorf("mimeOverride: got %q, want %q (req.MimeType)", mimeOverride, "image/png")
			}
			if packID != "" || packName != "" || packPublisher != "" || emojis != nil {
				t.Errorf("request sem metadata de pacote, got %q/%q/%q/%v", packID, packName, packPublisher, emojis)
			}
			return processedBytes, processedMime, nil
		},
	}
	logger := &contractsfake.Logger{}

	raw := stickerDataURI("image/png", webpBytes)
	result, err := message.NewSendStickerUseCase(mm, jr, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{
			Phone: "5511999999999", Sticker: raw, MimeType: "image/png",
		})

	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-sticker-999" {
		t.Errorf("MessageID: got %q, want %q (o que o SDK devolveu)", result.MessageID, "wire-id-sticker-999")
	}
	if n := len(mm.SendStickerCalls); n != 1 {
		t.Fatalf("SendSticker chamado %d vez(es), quero 1", n)
	}
}

// TestSendSticker_PackMetadataAndThumbnailFlow (F119) proves that:
//  1. req.PackID/PackName/PackPublisher/Emojis reach ProcessSticker
//     (the reconstructed route passed empty strings and nil).
//  2. req.PngThumbnail reaches MediaPayload.PngThumbnail at SendSticker.
func TestSendSticker_PackMetadataAndThumbnailFlow(t *testing.T) {
	pngThumb := []byte{0x89, 0x50, 0x4E, 0x47}
	processedBytes := []byte{0xDE, 0xAD}
	const processedMime = "image/webp"

	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{
		ProcessStickerFunc: func(_ context.Context, _, _ string, packID, packName, packPublisher string, emojis []string) ([]byte, string, error) {
			if packID != "pack-42" {
				t.Errorf("packID: got %q, want %q", packID, "pack-42")
			}
			if packName != "Gatos" {
				t.Errorf("packName: got %q, want %q", packName, "Gatos")
			}
			if packPublisher != "Lucas" {
				t.Errorf("packPublisher: got %q, want %q", packPublisher, "Lucas")
			}
			if len(emojis) != 1 || emojis[0] != "😺" {
				t.Errorf("emojis: got %v, want [😺]", emojis)
			}
			return processedBytes, processedMime, nil
		},
	}
	logger := &contractsfake.Logger{}

	raw := stickerDataURI("image/png", webpBytes)
	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{
			Phone:         "5511987654321",
			Sticker:       raw,
			PngThumbnail:  pngThumb,
			PackID:        "pack-42",
			PackName:      "Gatos",
			PackPublisher: "Lucas",
			Emojis:        []string{"😺"},
		})

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n := len(mm.SendStickerCalls); n != 1 {
		t.Fatalf("SendSticker chamado %d vez(es), quero 1", n)
	}
	call := mm.SendStickerCalls[0]
	if string(call.Payload.PngThumbnail) != string(pngThumb) {
		t.Errorf("PngThumbnail: got %v, want %v — o campo nao fluiu do request ate' a porta", call.Payload.PngThumbnail, pngThumb)
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
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	raw := stickerDataURI("image/png", webpBytes)
	result, err := message.NewSendStickerUseCase(mm, jr, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: raw, ID: "id-do-cliente"})

	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendSticker_DownstreamFailureNeverProducesSent: SendSticker (upload
// e/ou envio) falhando NUNCA produz Status=sent — a garantia central contra
// falso-sucesso.
func TestSendSticker_DownstreamFailureNeverProducesSent(t *testing.T) {
	sendErr := errors.New("sendsticker: boom")
	mm := &contractsfake.MediaMessenger{
		SendStickerFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, sendErr
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{}
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	raw := stickerDataURI("image/png", webpBytes)
	result, err := message.NewSendStickerUseCase(mm, jr, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: raw})

	if !errors.Is(err, sendErr) {
		t.Fatalf("erro do envio nao chegou ao chamador: got %#v", err)
	}
	if result != nil {
		t.Errorf("falha de envio produziu resultado nao-nil: %+v", result)
	}
}

// --- integração com fetch real / SSRF (ramo URL) ---------------------------

// TestSendSticker_RealFetchIntegration exercita o use case com um
// appport.MediaFetcher REAL (opengraph.URLFetcher) contra um
// httptest.Server, e um StickerProcessor fake (a conversão real precisa de
// ffmpeg — coberta em pkg/infra/media/sticker, não aqui).
func TestSendSticker_RealFetchIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(webpBytes)
	}))
	defer srv.Close()

	mm := &contractsfake.MediaMessenger{}
	mf := opengraph.NewURLFetcher(srv.Client())
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{Phone: "5511987654321", Sticker: srv.URL})

	if err != nil {
		t.Fatalf("integracao com fetch real falhou: %v", err)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if n := len(sp.ProcessStickerCalls); n != 1 {
		t.Fatalf("ProcessSticker chamado %d vez(es), quero exatamente 1", n)
	}
}

// TestSendSticker_SSRF_LoopbackBlocked prova que Sticker passa pela MESMA
// seam SSRF-safe já provada em CAP-02/CAP-06 — não um caminho de fetch
// paralelo.
func TestSendSticker_SSRF_LoopbackBlocked(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := opengraph.NewURLFetcher(http.DefaultClient)
	sp := &contractsfake.StickerProcessor{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendStickerUseCase(mm, &contractsfake.JIDResolver{}, mf, sp, logger).
		Execute(context.Background(), userID, domain.SendStickerRequest{
			Phone: "5511987654321", Sticker: "http://127.0.0.1:1/figurinha.webp",
		})

	if err == nil {
		t.Fatal("URL loopback foi aceita")
	}
	if n := len(sp.ProcessStickerCalls); n != 0 {
		t.Errorf("loopback deveria ser bloqueado antes de ProcessSticker, mas foi chamado %d vez(es)", n)
	}
}
