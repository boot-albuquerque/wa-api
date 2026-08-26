package message_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/media/opengraph"
)

// fetchVideoMaxBytesForTest espelha a constante interna fetchVideoMaxBytes
// de send_video.go — 100MB, paridade com o wuzapi histórico (ver comentário
// naquele arquivo). Não exportada do pacote message de propósito; duplicada
// aqui só para a asserção do valor passado a MediaFetcher.FetchBytes.
const fetchVideoMaxBytesForTest int64 = 100 * 1024 * 1024

const videoURL = "https://exemplo.com/clipe.mp4"

// mp4Bytes is a binary payload NOT recognized by Go's sniffer as a specific
// video type (http.DetectContentType returns "application/octet-stream") —
// good enough for tests that don't depend on exact sniffing. Must be at
// least minVideoBytes (256, F240) to pass the minimum size check.
var mp4Bytes = func() []byte {
	b := make([]byte, 300)
	b[0], b[1], b[2], b[3] = 0x00, 0x00, 0x00, 0x18
	b[4], b[5], b[6], b[7] = 'f', 't', 'y', 'p'
	return b
}()

func videoDataURI(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// --- validação de campos obrigatórios ------------------------------------

func TestSendVideo_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendVideoRequest
	}{
		{"Phone", domain.SendVideoRequest{Video: videoURL}},
		{"Video", domain.SendVideoRequest{Phone: "5511987654321"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request sem %s foi aceito", tc.name)
			}
			if n := len(mm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Errorf("validacao falhou mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendVideoCalls); n != 0 {
				t.Errorf("validacao falhou mas SendVideo foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendVideo_SessionFailurePropagates(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Errorf("sem sessao, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Errorf("sem sessao, mas SendVideo foi chamado %d vez(es)", n)
	}
}

func TestSendVideo_InvalidPhoneNeverFetchesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, jr, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "lixo", Video: videoURL})

	if err == nil {
		t.Fatal("JID invalido foi aceito")
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Errorf("JID invalido, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Errorf("JID invalido, mas SendVideo foi chamado %d vez(es)", n)
	}
}

// --- discriminação de transporte (a MAIS FROUXA das quatro) --------------

// TestSendVideo_UnsupportedSource_Rejected prova que nem base64 cru sem
// prefixo nem scheme proibido são aceitos — só "data" (4 chars) e URL
// http(s).
func TestSendVideo_UnsupportedSource_Rejected(t *testing.T) {
	cases := map[string]string{
		"raw_base64_no_prefix": "AAAA",
		"unsupported_scheme":   "ftp://exemplo.com/clipe.mp4",
		"short_dat_no_panic":   "dat",
	}
	for name, video := range cases {
		t.Run(name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: video})

			if err == nil {
				t.Fatalf("fonte nao suportada %q foi aceita", video)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Errorf("fonte nao suportada, mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendVideoCalls); n != 0 {
				t.Errorf("fonte nao suportada, mas SendVideo foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendVideo_DataPrefix_WideDiscrimination_NonVideoMimeAccepted prova a
// discriminação FROUXA (ao contrário de Audio/Image): uma data URI com MIME
// não-vídeo declarado ("data:application/pdf") ainda bate no prefixo "data"
// e é decodificada — o histórico nunca filtrou por MIME neste ramo.
func TestSendVideo_DataPrefix_WideDiscrimination_NonVideoMimeAccepted(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	raw := videoDataURI("application/pdf", mp4Bytes)
	result, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: raw})

	if err != nil {
		t.Fatalf("data URI com MIME nao-video foi recusada pela discriminacao larga: %v", err)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
}

func TestSendVideo_ShortVideoNoPanic(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Video de 3 caracteres panicou: %v", r)
			}
		}()
		_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
			Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: "dat"})
		if err == nil {
			t.Fatal("Video de 3 caracteres foi aceito")
		}
	}()
}

func TestSendVideo_FetchFailurePropagates(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("fetch: recusado")
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err == nil {
		t.Fatal("falha no fetch foi engolida")
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Errorf("fetch falhou, mas SendVideo foi chamado %d vez(es)", n)
	}
}

func TestSendVideo_EmptyBodyRejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return []byte{}, "video/mp4", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err == nil {
		t.Fatal("corpo vazio foi aceito")
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Errorf("corpo vazio, mas SendVideo foi chamado %d vez(es)", n)
	}
}

// --- precedência de MIME: dois níveis, igual a Image ----------------------

// TestSendVideo_MimeType_SniffedFromBytes proves that without a client
// MimeType, the resolved MIME comes from byte sniffing — NOT the remote
// Content-Type. With the F240 MIME family check, a payload that sniffs as
// text/plain is now correctly rejected (it is not a video type).
func TestSendVideo_MimeType_SniffedFromBytes(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	plainText := []byte(strings.Repeat("isto sniffa como text/plain. ", 10))
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return plainText, "video/mp4", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err == nil {
		t.Fatal("expected rejection: text/plain payload is not a valid video type")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "invalid_video_mime_type" {
		t.Fatalf("expected invalid_video_mime_type, got: %v", err)
	}
	if len(mm.SendVideoCalls) != 0 {
		t.Fatal("SendVideo should not have been called for non-video MIME")
	}
}

// TestSendVideo_MimeType_RemoteContentTypeNeverWins prova explicitamente
// que o Content-Type remoto do ramo URL NUNCA ganha precedência sobre o
// sniffing — divergência deliberada de Document, onde ele ganha.
func TestSendVideo_MimeType_RemoteContentTypeNeverWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return mp4Bytes, "video/quicktime", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendVideoCalls[0].Payload.MimeType; got == "video/quicktime" {
		t.Errorf("MimeType = %q: o Content-Type remoto venceu, mas nao deveria ter precedencia nenhuma", got)
	}
}

// --- Caption ---------------------------------------------------------------

func TestSendVideo_Caption_ForwardedFromRequest(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return mp4Bytes, "video/mp4", nil },
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL, Caption: "legenda do clipe"})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendVideoCalls[0].Payload.Caption; got != "legenda do clipe" {
		t.Errorf("Caption: got %q, want %q", got, "legenda do clipe")
	}
}

func TestSendVideo_Caption_EmptyWhenAbsent(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return mp4Bytes, "video/mp4", nil },
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendVideoCalls[0].Payload.Caption; got != "" {
		t.Errorf("Caption: got %q, want vazio", got)
	}
}

// --- teste da causa: fluxo completo ----------------------------------------

func TestSendVideo_CausalSuccess(t *testing.T) {
	sentAt := int64(1755500080)
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-video", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, resourceURL string, limit int64) ([]byte, string, error) {
			if resourceURL != videoURL {
				t.Errorf("URL buscada: got %q, want %q", resourceURL, videoURL)
			}
			if limit != fetchVideoMaxBytesForTest {
				t.Errorf("limite: got %d, want %d", limit, fetchVideoMaxBytesForTest)
			}
			return mp4Bytes, "video/mp4", nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{
			Phone: "5511987654321", Video: videoURL, Caption: "clipe",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mm.SendVideoCalls); n != 1 {
		t.Fatalf("SendVideo chamado %d vez(es), quero exatamente 1", n)
	}
	call := mm.SendVideoCalls[0]
	if call.Target != domain.JID("5511987654321@s.whatsapp.net") {
		t.Errorf("destinatario: got %q, want %q", call.Target, "5511987654321")
	}
	if string(call.Payload.Bytes) != string(mp4Bytes) {
		t.Errorf("bytes repassados a SendVideo divergem dos buscados")
	}
	if call.Payload.Caption != "clipe" {
		t.Errorf("Caption: got %q, want %q", call.Payload.Caption, "clipe")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-video" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "wire-id-video")
	}
	if result.Timestamp != sentAt {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt)
	}
}

func TestSendVideo_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return mp4Bytes, "video/mp4", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{
			Phone: "5511987654321", Video: videoURL, ID: "id-do-cliente",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendVideo_DownstreamFailureNeverProducesSent: SendVideo (upload e/ou
// envio) falhando nunca pode virar Status=StatusSent nem resultado não-nil
// — a garantia central anti-falso-sucesso.
func TestSendVideo_DownstreamFailureNeverProducesSent(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return mp4Bytes, "video/mp4", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err == nil {
		t.Fatal("falha da porta foi engolida")
	}
	if !errors.Is(err, errDownstream) {
		t.Fatalf("causa nao propagada: got %#v", err)
	}
	if result != nil {
		t.Errorf("resultado nao-nil devolvido junto com erro: %+v", result)
	}
}

// TestSendVideo_AcquisitionOK_UploadOK_SendFail_NeverSent é o caso
// obrigatório: aquisição (fetch/decode) OK e upload OK (simulado dentro do
// SendVideoFunc, que representa a porta upload+envio) seguidos de falha de
// envio NÃO produzem Status=sent — sem rollback de upload inventado.
func TestSendVideo_AcquisitionOK_UploadOK_SendFail_NeverSent(t *testing.T) {
	sendErr := errors.New("sendmessage: boom apos upload bem-sucedido")
	uploadThenSendCalled := false
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			uploadThenSendCalled = true
			return domain.MessageSendResult{}, sendErr
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return mp4Bytes, "video/mp4", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if !uploadThenSendCalled {
		t.Fatal("upload+envio nunca foi chamado — teste nao exercita o caso aquisicao-ok-upload-ok-send-fail")
	}
	if !errors.Is(err, sendErr) {
		t.Fatalf("falha de envio nao propagada: got %#v", err)
	}
	if result != nil {
		t.Errorf("resultado nao-nil com envio falho: %+v", result)
	}
}

// --- ramo data URI ----------------------------------------------------------

// TestSendVideo_DataURI_CausalSuccess é o teste da causa para o ramo data
// URI: prova que uma data URI válida é decodificada LOCALMENTE (o
// MediaFetcher nunca é tocado), os bytes decodificados batem com os
// originais, e o resultado só vira StatusSent depois que SendVideo confirma
// sucesso.
func TestSendVideo_DataURI_CausalSuccess(t *testing.T) {
	sentAt := int64(1755500090)
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-video-datauri", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	raw := videoDataURI("video/mp4", mp4Bytes)
	result, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: raw})

	if err != nil {
		t.Fatalf("caminho feliz (data URI) falhou: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("data URI, mas MediaFetcher foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendVideoCalls); n != 1 {
		t.Fatalf("SendVideo chamado %d vez(es), quero exatamente 1", n)
	}
	if string(mm.SendVideoCalls[0].Payload.Bytes) != string(mp4Bytes) {
		t.Error("bytes decodificados divergem dos originais")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-video-datauri" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "wire-id-video-datauri")
	}
}

func TestSendVideo_DataURI_MalformedBase64_Rejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: "data:video/mp4;base64,***nao-e-base64***"})

	if err == nil {
		t.Fatal("base64 malformado foi aceito")
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Errorf("base64 malformado, mas SendVideo foi chamado %d vez(es)", n)
	}
}

func TestSendVideo_URLBranch_NotCapturedByDataDiscrimination(t *testing.T) {
	sentAt := int64(1755500100)
	mm := &contractsfake.MediaMessenger{
		SendVideoFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-url-conservation", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return mp4Bytes, "video/mp4", nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err != nil {
		t.Fatalf("URL comum foi recusada pela discriminacao: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 1 {
		t.Fatalf("URL comum devia acionar MediaFetcher exatamente 1 vez, foi %d", n)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
}

// --- limite de 100MB (ramo URL, erro propagado) -----------------------------

func TestSendVideo_URL_FetchTooLarge_Rejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("corpo excede o limite de bytes")
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err == nil {
		t.Fatal("fetch acima do limite foi aceito")
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Errorf("fetch acima do limite, mas SendVideo foi chamado %d vez(es)", n)
	}
}

// --- integração com fetch real / SSRF ---------------------------------------

// TestSendVideo_RealFetchIntegration exercita o use case com um
// appport.MediaFetcher REAL (opengraph.URLFetcher) contra um
// httptest.Server — não um dublê do fetch. Cobre a integração completa,
// incluindo a passagem pela seam SSRF-safe já provada em CAP-02.
func TestSendVideo_RealFetchIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(mp4Bytes)
	}))
	defer srv.Close()

	mm := &contractsfake.MediaMessenger{}
	mf := opengraph.NewURLFetcher(srv.Client())
	logger := &contractsfake.Logger{}

	result, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: srv.URL})

	if err != nil {
		t.Fatalf("integracao com fetch real falhou: %v", err)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if n := len(mm.SendVideoCalls); n != 1 {
		t.Fatalf("SendVideo chamado %d vez(es), quero exatamente 1", n)
	}
	if string(mm.SendVideoCalls[0].Payload.Bytes) != string(mp4Bytes) {
		t.Error("bytes buscados pelo fetch real divergem do corpo servido")
	}
}

// TestSendVideo_SSRF_LoopbackBlocked prova que Video passa pela MESMA seam
// TestSendVideo_MimeTypeAndThumbnailFlowToPayload (F118) proves two things:
//  1. SendVideoRequest.MimeType reaches resolveMimeType as level-1 precedence
//     (the reconstructed route hardcoded "" there, dropping client-supplied MIME).
//  2. SendVideoRequest.JPEGThumbnail reaches MediaPayload.JPEGThumbnail.
func TestSendVideo_MimeTypeAndThumbnailFlowToPayload(t *testing.T) {
	thumb := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}

	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	uri := videoDataURI("video/mp4", mp4Bytes)
	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{
			Phone: "5511987654321", Video: uri,
			MimeType:      "video/mp4",
			JPEGThumbnail: thumb,
		})

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n := len(mm.SendVideoCalls); n != 1 {
		t.Fatalf("SendVideo chamado %d vez(es), quero 1", n)
	}
	call := mm.SendVideoCalls[0]
	if call.Payload.MimeType != "video/mp4" {
		t.Errorf("MimeType: got %q, want %q — req.MimeType nao teve precedencia (nivel 1)", call.Payload.MimeType, "video/mp4")
	}
	if string(call.Payload.JPEGThumbnail) != string(thumb) {
		t.Errorf("JPEGThumbnail: got %v, want %v — o campo nao fluiu do request ate' a porta", call.Payload.JPEGThumbnail, thumb)
	}
}

// --- F240: minimum size and MIME family validation -------------------------

// TestSendVideo_F240_TooSmallPayloadRejected proves that a payload below
// minVideoBytes is rejected before reaching SendVideo.
func TestSendVideo_F240_TooSmallPayloadRejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	tinyPayload := make([]byte, 32)
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return tinyPayload, "video/mp4", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err == nil {
		t.Fatal("32-byte video was accepted; expected video_too_small")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "video_too_small" {
		t.Fatalf("expected video_too_small, got: %v", err)
	}
	if len(mm.SendVideoCalls) != 0 {
		t.Fatal("SendVideo should not have been called for tiny payload")
	}
}

// TestSendVideo_F240_NonVideoMimeRejected proves that when sniffing resolves
// to a determinate non-video type (e.g. image/png), the request is rejected.
func TestSendVideo_F240_NonVideoMimeRejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	// PNG magic: \x89PNG\r\n\x1a\n — sniffs as "image/png"
	pngPayload := make([]byte, 300)
	pngPayload[0] = 0x89
	copy(pngPayload[1:], []byte("PNG\r\n\x1a\n"))
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return pngPayload, "", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err == nil {
		t.Fatal("PNG payload was accepted as video; expected invalid_video_mime_type")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "invalid_video_mime_type" {
		t.Fatalf("expected invalid_video_mime_type, got: %v", err)
	}
	if len(mm.SendVideoCalls) != 0 {
		t.Fatal("SendVideo should not have been called for non-video MIME")
	}
}

// TestSendVideo_F240_OctetStreamAllowedThrough proves that
// "application/octet-stream" (Go's "I don't know") is NOT rejected, because
// http.DetectContentType cannot recognize MP4 containers.
func TestSendVideo_F240_OctetStreamAllowedThrough(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return mp4Bytes, "video/mp4", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{Phone: "5511987654321", Video: videoURL})

	if err != nil {
		t.Fatalf("octet-stream payload was rejected: %v", err)
	}
	if len(mm.SendVideoCalls) != 1 {
		t.Fatalf("SendVideo chamado %d vez(es), quero 1", len(mm.SendVideoCalls))
	}
}

// SSRF-safe já provada em CAP-02 — não um caminho de fetch paralelo.
func TestSendVideo_SSRF_LoopbackBlocked(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := opengraph.NewURLFetcher(http.DefaultClient)
	logger := &contractsfake.Logger{}

	_, err := message.NewSendVideoUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendVideoRequest{
			Phone: "5511987654321", Video: "http://127.0.0.1:1/clipe.mp4",
		})

	if err == nil {
		t.Fatal("URL loopback foi aceita")
	}
	if n := len(mm.SendVideoCalls); n != 0 {
		t.Errorf("loopback deveria ser bloqueado antes de SendVideo, mas foi chamado %d vez(es)", n)
	}
}
