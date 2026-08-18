package message_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/media/opengraph"
)

// fetchImageMaxBytesForTest espelha a constante interna fetchImageMaxBytes
// de send_image.go — 16MB, paridade com o wuzapi histórico (ver comentário
// naquele arquivo). Não exportada do pacote message de propósito (é
// detalhe de implementação); duplicada aqui só para a asserção do valor
// passado a MediaFetcher.FetchBytes.
const fetchImageMaxBytesForTest int64 = 16 * 1024 * 1024

// pngBytes é uma imagem PNG mínima válida (1x1, transparente) — grande o
// bastante para http.DetectContentType reconhecer "image/png" pela
// assinatura de bytes, sem depender de decodificação completa.
var pngBytes = []byte{
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

const imageURL = "https://exemplo.com/foto.png"

// TestSendImage_MissingRequiredField: Phone ou Image ausente é recusado
// antes de qualquer porta ser tocada.
func TestSendImage_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendImageRequest
	}{
		{"Phone", domain.SendImageRequest{Image: imageURL}},
		{"Image", domain.SendImageRequest{Phone: "5511987654321"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
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
			if n := len(mm.SendImageCalls); n != 0 {
				t.Errorf("validacao falhou mas SendImage foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendImage_SessionFailurePropagates: sem sessão, o erro da porta chega
// ao chamador por identidade e nem fetch nem SendImage são tocados.
func TestSendImage_SessionFailurePropagates(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: imageURL})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Errorf("sem sessao, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Errorf("sem sessao, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_InvalidPhoneNeverFetchesOrSends: JID que não resolve é
// recusado antes do fetch — nunca alcança rede externa nem envio.
func TestSendImage_InvalidPhoneNeverFetchesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}

	_, err := message.NewSendImageUseCase(mm, jr, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{Phone: "lixo", Image: imageURL})

	if err == nil {
		t.Fatal("telefone invalido foi aceito")
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("JID invalido, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_UnsupportedSource_DataURIRejected: o ramo data URI é CAP-03,
// fora de escopo — precisa ser recusado explicitamente, nunca produzir
// falso-sucesso (o stub anterior devolvia Status="validated" sem enviar
// nada) nem ser tratado como uma URL http(s) qualquer.
func TestSendImage_UnsupportedSource_DataURIRejected(t *testing.T) {
	cases := []string{
		"data:image/png;base64,aGVsbG8=",
		"ftp://exemplo.com/foto.png",
		"file:///etc/passwd",
		"gopher://exemplo.com/foto.png",
		"nao-e-uma-url",
	}
	for _, img := range cases {
		t.Run(img, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: img})

			if err == nil {
				t.Fatalf("fonte nao-http %q foi aceita", img)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Errorf("fonte nao-http %q, mas o fetch foi chamado %d vez(es)", img, n)
			}
			if n := len(mm.SendImageCalls); n != 0 {
				t.Errorf("fonte nao-http %q, mas SendImage foi chamado %d vez(es)", img, n)
			}
		})
	}
}

// TestSendImage_FetchFailurePropagates: falha ao buscar a URL (SSRF
// bloqueado, timeout, host inexistente, etc.) nunca alcança SendImage nem
// produz sucesso.
func TestSendImage_FetchFailurePropagates(t *testing.T) {
	fetchErr := errors.New("fetch: recusado")
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return nil, "", fetchErr },
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: imageURL})

	if !errors.Is(err, fetchErr) {
		t.Fatalf("causa do fetch nao propagada: got %#v", err)
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("fetch falhou, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_EmptyBodyRejected: corpo vazio (200 OK sem bytes) não pode
// virar um envio de mídia vazia.
func TestSendImage_EmptyBodyRejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return []byte{}, "image/png", nil },
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: imageURL})

	if err == nil {
		t.Fatal("corpo vazio foi aceito")
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("corpo vazio, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_MimeType_RequestFieldWinsOverRemoteContentType prova a
// precedência de MIME herdada de handlers.go pré-refactor: o campo
// MimeType do request vence, e o Content-Type que o servidor remoto
// declarou (aqui deliberadamente mentiroso: "text/plain" para bytes PNG)
// NUNCA chega a payload.MimeType — a defesa contra servidor remoto que
// mente no Content-Type.
func TestSendImage_MimeType_RequestFieldWinsOverRemoteContentType(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return pngBytes, "text/plain", nil // Content-Type mentiroso do servidor remoto.
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{
			Phone: "5511987654321", Image: imageURL, MimeType: "image/webp",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mm.SendImageCalls); n != 1 {
		t.Fatalf("SendImage chamado %d vez(es), quero exatamente 1", n)
	}
	if got := mm.SendImageCalls[0].Payload.MimeType; got != "image/webp" {
		t.Errorf("MimeType: got %q, want %q (o do request, nao o Content-Type remoto)", got, "image/webp")
	}
}

// TestSendImage_MimeType_SniffedFromBytesWhenAbsent: sem MimeType no
// request, o tipo vem de sniffing dos bytes reais — nunca do Content-Type
// declarado pelo servidor remoto.
func TestSendImage_MimeType_SniffedFromBytesWhenAbsent(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return pngBytes, "application/octet-stream", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: imageURL})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendImageCalls[0].Payload.MimeType; got != "image/png" {
		t.Errorf("MimeType sniffado: got %q, want %q", got, "image/png")
	}
}

// TestSendImage_InvalidMimeType_Rejected: bytes que não sniffam como imagem
// e nenhum MimeType de imagem no request são recusados — o endpoint de
// imagem não envia qualquer coisa que o servidor remoto entregou.
func TestSendImage_InvalidMimeType_Rejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return []byte("%PDF-1.4 nao e imagem"), "application/pdf", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: imageURL})

	if err == nil {
		t.Fatal("MIME nao-imagem foi aceito")
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("MIME invalido, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_CausalSuccess é o teste da causa: um envio bem-sucedido
// OBRIGATORIAMENTE busca a URL com o limite correto, sobe exatamente os
// bytes buscados com o destinatário e a legenda corretos, e Status só vale
// StatusSent DEPOIS que a porta de envio devolveu sucesso.
func TestSendImage_CausalSuccess(t *testing.T) {
	sentAt := int64(1755500002)
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-image", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, resourceURL string, limit int64) ([]byte, string, error) {
			if resourceURL != imageURL {
				t.Errorf("URL buscada: got %q, want %q", resourceURL, imageURL)
			}
			if limit != fetchImageMaxBytesForTest {
				t.Errorf("limite: got %d, want %d", limit, fetchImageMaxBytesForTest)
			}
			return pngBytes, "image/png", nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{
			Phone: "5511987654321", Image: imageURL, Caption: "legenda",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mm.SendImageCalls); n != 1 {
		t.Fatalf("SendImage chamado %d vez(es), quero exatamente 1", n)
	}
	call := mm.SendImageCalls[0]
	if call.Target != domain.JID("5511987654321") {
		t.Errorf("destinatario: got %q, want %q", call.Target, "5511987654321")
	}
	if string(call.Payload.Bytes) != string(pngBytes) {
		t.Errorf("bytes repassados a SendImage divergem dos buscados")
	}
	if call.Payload.Caption != "legenda" {
		t.Errorf("Caption: got %q, want %q", call.Payload.Caption, "legenda")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-image" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "wire-id-image")
	}
	if result.Timestamp != sentAt {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt)
	}
}

// TestSendImage_ClientSuppliedIDIsForwardedButServerIDWins: o Id do cliente
// é repassado a SendImage, mas o message_id publicado é o que a porta
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
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return pngBytes, "image/png", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{
			Phone: "5511987654321", Image: imageURL, ID: "id-do-cliente",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendImage_DownstreamFailureNeverProducesSent: SendImage (upload e/ou
// envio) falhando nunca pode virar Status=StatusSent nem resultado não-nil
// — a mesma garantia anti-falso-sucesso do CAP-01, agora para mídia.
func TestSendImage_DownstreamFailureNeverProducesSent(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(context.Context, string, domain.JID, domain.MediaPayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return pngBytes, "image/png", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: imageURL})

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

// TestSendImage_RealFetchIntegration exercita o use case com um
// appport.MediaFetcher REAL de verdade (opengraph.URLFetcher) contra um
// httptest.Server — não um dublê do fetch. Prova que a integração completa
// (URL -> bytes reais -> use case) funciona, cobrindo o caminho que os
// testes com MediaFetcher fake não alcançam.
func TestSendImage_RealFetchIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()

	mm := &contractsfake.MediaMessenger{}
	mf := opengraph.NewURLFetcher(srv.Client())
	logger := &contractsfake.Logger{}

	result, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: srv.URL})

	if err != nil {
		t.Fatalf("integracao com fetch real falhou: %v", err)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if n := len(mm.SendImageCalls); n != 1 {
		t.Fatalf("SendImage chamado %d vez(es), quero exatamente 1", n)
	}
	if string(mm.SendImageCalls[0].Payload.Bytes) != string(pngBytes) {
		t.Error("bytes buscados pelo fetch real divergem do PNG servido")
	}
}
