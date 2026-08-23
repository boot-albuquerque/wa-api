package message_test

import (
	"context"
	"encoding/base64"
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

// TestSendImage_UnsupportedSource_Rejected: fontes que não são nem URL
// http(s) (CAP-02) nem data URI de imagem (CAP-03, prefixo "data:image")
// precisam ser recusadas explicitamente, nunca produzir falso-sucesso nem
// ser tratadas como um dos dois transportes suportados. Inclui base64 cru
// sem o prefixo "data:" — historicamente NÃO suportado (item 2 do bloco
// CAP-03) — e um MIME não-imagem em data URI, que falha o prefixo
// "data:image" e cai aqui também (item 3 do bloco).
func TestSendImage_UnsupportedSource_Rejected(t *testing.T) {
	cases := []string{
		"ftp://exemplo.com/foto.png",
		"file:///etc/passwd",
		"gopher://exemplo.com/foto.png",
		"nao-e-uma-url",
		"/9j/4AAQSkZJRgABAQEASABIAAD=",         // base64 cru, sem prefixo "data:"
		"data:application/pdf;base64,aGVsbG8=", // MIME nao-imagem em data URI
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
		SendImageFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
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
	if call.Target != domain.JID("5511987654321@s.whatsapp.net") {
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
		SendImageFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
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
		SendImageFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
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

// dataImageURI monta uma data URI "data:<mime>;base64,<payload>" a partir
// de bytes crus — helper de teste, não duplica o decoder de produção.
func dataImageURI(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// TestSendImage_DataURI_CausalSuccess é o teste da causa para o ramo
// data URI: prova que uma data URI válida é decodificada LOCALMENTE (o
// MediaFetcher nunca é tocado), os bytes decodificados batem com os bytes
// originais, o MIME final vem do sniffing dos bytes reais (nunca do rótulo
// declarado pela data URI — ver TestSendImage_DataURI_MimeType_LabelIsNotTrusted),
// e o resultado só vira StatusSent depois que SendImage confirma sucesso.
func TestSendImage_DataURI_CausalSuccess(t *testing.T) {
	sentAt := int64(1755500010)
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-datauri", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	uri := dataImageURI("image/png", pngBytes)
	result, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{
			Phone: "5511987654321", Image: uri, Caption: "legenda-datauri",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("data URI, mas MediaFetcher foi chamado %d vez(es) — devia decodificar local", n)
	}
	if n := len(mm.SendImageCalls); n != 1 {
		t.Fatalf("SendImage chamado %d vez(es), quero exatamente 1", n)
	}
	call := mm.SendImageCalls[0]
	if string(call.Payload.Bytes) != string(pngBytes) {
		t.Errorf("bytes decodificados divergem dos originais")
	}
	if call.Payload.MimeType != "image/png" {
		t.Errorf("MimeType: got %q, want %q (sniffado dos bytes decodificados)", call.Payload.MimeType, "image/png")
	}
	if call.Payload.Caption != "legenda-datauri" {
		t.Errorf("Caption: got %q, want %q", call.Payload.Caption, "legenda-datauri")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-datauri" {
		t.Errorf("MessageID: got %q, want o devolvido pela porta", result.MessageID)
	}
}

// TestSendImage_DataURI_MimeType_LabelIsNotTrusted é o teste de MIME-spoofing
// que faltava (achado do avaliador independente em CAP-03): uma data URI cujo
// rótulo declara "image/png" mas cujos bytes reais NÃO são PNG (são bytes de
// PDF) não pode sair com Mimetype=image/png. Sem req.MimeType, o resultado
// tem de vir do sniffing dos bytes DECODIFICADOS reais — nunca do rótulo, que
// é controlado pelo cliente e pode mentir exatamente como o Content-Type de
// um servidor remoto no ramo URL.
func TestSendImage_DataURI_MimeType_LabelIsNotTrusted(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	pdfBytes := []byte("%PDF-1.4 nao e imagem, apesar do rotulo dizer image/png")
	uri := dataImageURI("image/png", pdfBytes)

	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{
			Phone: "5511987654321", Image: uri,
		})

	// http.DetectContentType nos bytes de PDF acima não reconhece assinatura
	// de imagem nenhuma — cai no filtro de MIME não-imagem do Execute, que é
	// exatamente o comportamento correto: o rótulo mentiroso não deve
	// conseguir disfarçar um payload não-imagem como imagem aceitável.
	if err == nil {
		t.Fatalf("data URI com rotulo mentiroso (image/png sobre bytes de PDF) foi aceita; MimeType enviado: %q", mm.SendImageCalls[0].Payload.MimeType)
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("rotulo mentiroso, mas SendImage foi chamado %d vez(es) com Mimetype=%q — MIME-spoofing nao bloqueado",
			n, mm.SendImageCalls[0].Payload.MimeType)
	}
}

// TestSendImage_DataURI_MimeType_SniffedOverridesLabel é o segundo caso de
// MIME-spoofing: um rótulo de data URI mentiroso ("image/jpeg") sobre bytes
// que SÃO uma imagem de verdade, só que de outro tipo (PNG). Sem
// req.MimeType, o resultado enviado tem de ser o tipo REAL sniffado
// ("image/png"), nunca o rótulo declarado — reproduz o cenário exato que o
// avaliador independente provou aceito incorretamente antes desta correção.
func TestSendImage_DataURI_MimeType_SniffedOverridesLabel(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-spoof-check"}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	uri := dataImageURI("image/jpeg", pngBytes) // rotulo mente: diz jpeg, bytes sao png
	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{
			Phone: "5511987654321", Image: uri,
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendImageCalls[0].Payload.MimeType; got != "image/png" {
		t.Errorf("MimeType: got %q, want %q (sniffado dos bytes reais, nao o rotulo 'image/jpeg')", got, "image/png")
	}
}

// TestSendImage_DataURI_MimeType_RequestFieldWinsOverLabel prova a
// precedência no ramo data URI, espelhando
// TestSendImage_MimeType_RequestFieldWinsOverRemoteContentType no ramo URL:
// req.MimeType, quando presente, vence tanto o rótulo declarado pela data
// URI quanto o sniffing dos bytes reais.
func TestSendImage_DataURI_MimeType_RequestFieldWinsOverLabel(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	uri := dataImageURI("image/png", pngBytes) // rotulo diz png, bytes tambem sao png
	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{
			Phone: "5511987654321", Image: uri, MimeType: "image/webp",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendImageCalls[0].Payload.MimeType; got != "image/webp" {
		t.Errorf("MimeType: got %q, want %q (req.MimeType vence rotulo e sniffing)", got, "image/webp")
	}
}

// TestSendImage_DataURI_MalformedBase64_Rejected: base64 quebrado dentro de
// uma data URI "data:image/..." vira o erro canônico de decode, nunca
// sucesso nem 500 — item 5 do bloco.
func TestSendImage_DataURI_MalformedBase64_Rejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{
			Phone: "5511987654321", Image: "data:image/png;base64,%%%nao-e-base64%%%",
		})

	if err == nil {
		t.Fatal("base64 malformado foi aceito")
	}
	if n := len(mm.SendImageCalls); n != 0 {
		t.Fatalf("base64 malformado, mas SendImage foi chamado %d vez(es)", n)
	}
}

// TestSendImage_DataURI_SizeBoundary congela a fronteira do teto de 16MB
// (divergência D1) no ramo data URI: exatamente 16MB de payload decodificado
// é aceito, 16MB+1 byte é recusado. As duas rodadas usam o MESMO gerador de
// bytes (só varia em 1 byte) para que a fronteira, e não outra diferença,
// seja o que muda o resultado.
func TestSendImage_DataURI_SizeBoundary(t *testing.T) {
	const maxBytes = 16 * 1024 * 1024

	makePNGLikePayload := func(n int) []byte {
		data := make([]byte, n)
		copy(data, pngBytes) // mantém a assinatura PNG no início, resto é enchimento
		return data
	}

	t.Run("exactly16MB_accepted", func(t *testing.T) {
		mm := &contractsfake.MediaMessenger{
			SendImageFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
				return domain.MessageSendResult{ID: "wire-id-exact-16mb"}, nil
			},
		}
		mf := &contractsfake.MediaFetcher{}
		logger := &contractsfake.Logger{}

		payload := makePNGLikePayload(maxBytes)
		uri := dataImageURI("image/png", payload)

		result, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
			Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: uri})

		if err != nil {
			t.Fatalf("payload de exatamente 16MB foi recusado: %v", err)
		}
		if result.Status != domain.StatusSent {
			t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
		}
		if n := len(mm.SendImageCalls[0].Payload.Bytes); n != maxBytes {
			t.Errorf("bytes repassados: got %d, want %d", n, maxBytes)
		}
	})

	t.Run("16MBplus1_rejected", func(t *testing.T) {
		mm := &contractsfake.MediaMessenger{}
		mf := &contractsfake.MediaFetcher{}
		logger := &contractsfake.Logger{}

		payload := makePNGLikePayload(maxBytes + 1)
		uri := dataImageURI("image/png", payload)

		_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
			Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: uri})

		if err == nil {
			t.Fatal("payload de 16MB+1 foi aceito")
		}
		if n := len(mm.SendImageCalls); n != 0 {
			t.Fatalf("payload acima do limite, mas SendImage foi chamado %d vez(es)", n)
		}
	})
}

// TestSendImage_URLBranch_NotCapturedByDataURIDiscrimination é o teste de
// conservação central deste slice: a nova discriminação (data URI vs URL)
// NÃO pode capturar uma URL http(s) comum como se fosse data URI ou base64
// cru — o ramo URL do CAP-02 continua funcionando exatamente como antes.
func TestSendImage_URLBranch_NotCapturedByDataURIDiscrimination(t *testing.T) {
	sentAt := int64(1755500011)
	mm := &contractsfake.MediaMessenger{
		SendImageFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-url-conservation", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return pngBytes, "image/png", nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{Phone: "5511987654321", Image: imageURL})

	if err != nil {
		t.Fatalf("URL comum foi recusada pela nova discriminacao: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 1 {
		t.Fatalf("URL comum devia acionar MediaFetcher exatamente 1 vez, foi %d", n)
	}
	if n := len(mm.SendImageCalls); n != 1 {
		t.Fatalf("URL comum devia acionar SendImage exatamente 1 vez, foi %d", n)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
}

// TestSendImage_JPEGThumbnailFlowsToPayload (F115) proves that
// SendImageRequest.JPEGThumbnail reaches MediaPayload.JPEGThumbnail and
// from there arrives at the MediaMessenger port. The historical handler
// wired req.JPEGThumbnail into ImageMessage.JpegThumbnail; the
// reconstructed route lost it.
func TestSendImage_JPEGThumbnailFlowsToPayload(t *testing.T) {
	thumb := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}

	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	uri := dataImageURI("image/png", pngBytes)
	_, err := message.NewSendImageUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendImageRequest{
			Phone: "5511987654321", Image: uri, JPEGThumbnail: thumb,
		})

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n := len(mm.SendImageCalls); n != 1 {
		t.Fatalf("SendImage chamado %d vez(es), quero 1", n)
	}
	got := mm.SendImageCalls[0].Payload.JPEGThumbnail
	if string(got) != string(thumb) {
		t.Errorf("JPEGThumbnail: got %v, want %v — o campo nao fluiu do request ate' a porta", got, thumb)
	}
}
