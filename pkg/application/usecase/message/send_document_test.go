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

// fetchDocumentMaxBytesForTest espelha a constante interna
// fetchDocumentMaxBytes de send_document.go — 100MB, paridade com o wuzapi
// histórico (ver comentário naquele arquivo). Não exportada do pacote
// message de propósito (é detalhe de implementação); duplicada aqui só
// para a asserção do valor passado a MediaFetcher.FetchBytes.
const fetchDocumentMaxBytesForTest int64 = 100 * 1024 * 1024

// pdfBytes é um corpo qualquer com assinatura PDF — usado para provar que
// o sniffing de MIME (quando aplicável) reconhece "application/pdf" e que
// documentos não-imagem são aceitos sem restrição de MIME.
var pdfBytes = []byte("%PDF-1.4\n%busca de assinatura de bytes\n")

const documentURL = "https://exemplo.com/relatorio.pdf"

// documentDataURI monta uma data URI "data:<mime>;base64,<payload>" a
// partir de bytes crus — helper de teste, não duplica o decoder de
// produção.
func documentDataURI(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// TestSendDocument_MissingRequiredField: Phone, Document ou FileName
// ausente é recusado antes de qualquer porta ser tocada.
func TestSendDocument_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendDocumentRequest
	}{
		{"Phone", domain.SendDocumentRequest{Document: documentURL, FileName: "a.pdf"}},
		{"Document", domain.SendDocumentRequest{Phone: "5511987654321", FileName: "a.pdf"}},
		{"FileName", domain.SendDocumentRequest{Phone: "5511987654321", Document: documentURL}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
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
			if n := len(mm.SendDocumentCalls); n != 0 {
				t.Errorf("validacao falhou mas SendDocument foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendDocument_SessionFailurePropagates: sem sessão, o erro da porta
// chega ao chamador por identidade e nem fetch nem SendDocument são
// tocados.
func TestSendDocument_SessionFailurePropagates(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{Phone: "5511987654321", Document: documentURL, FileName: "a.pdf"})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Errorf("sem sessao, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Errorf("sem sessao, mas SendDocument foi chamado %d vez(es)", n)
	}
}

// TestSendDocument_InvalidPhoneNeverFetchesOrSends: JID que não resolve é
// recusado antes do fetch — nunca alcança rede externa nem envio.
func TestSendDocument_InvalidPhoneNeverFetchesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, jr, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{Phone: "lixo", Document: documentURL, FileName: "a.pdf"})

	if err == nil {
		t.Fatal("JID invalido foi aceito")
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Errorf("JID invalido, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Errorf("JID invalido, mas SendDocument foi chamado %d vez(es)", n)
	}
}

// TestSendDocument_UnsupportedSource_Rejected: fontes que não são nem "data:"
// nem URL http(s) — incluindo base64 cru sem prefixo "data:" e scheme
// proibido — viram erro, nunca o 200 "validated" falso que o stub anterior
// produzia.
func TestSendDocument_UnsupportedSource_Rejected(t *testing.T) {
	cases := map[string]string{
		"raw_base64_no_prefix": "JVBERi0xLjQK",
		"unsupported_scheme":   "ftp://exemplo.com/relatorio.pdf",
		"empty_after_trim":     "not-a-uri-not-a-url",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, domain.SendDocumentRequest{Phone: "5511987654321", Document: doc, FileName: "a.pdf"})

			if err == nil {
				t.Fatalf("fonte nao suportada %q foi aceita", doc)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Errorf("fonte nao suportada, mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendDocumentCalls); n != 0 {
				t.Errorf("fonte nao suportada, mas SendDocument foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendDocument_FetchFailurePropagates: falha ao buscar a URL nunca vira
// sucesso.
func TestSendDocument_FetchFailurePropagates(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("fetch: recusado")
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{Phone: "5511987654321", Document: documentURL, FileName: "a.pdf"})

	if err == nil {
		t.Fatal("falha no fetch foi engolida")
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Errorf("fetch falhou, mas SendDocument foi chamado %d vez(es)", n)
	}
}

// TestSendDocument_EmptyBodyRejected: corpo vazio (fetch ou decode
// resultando em zero bytes) é recusado antes do upload/envio.
func TestSendDocument_EmptyBodyRejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return []byte{}, "application/pdf", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{Phone: "5511987654321", Document: documentURL, FileName: "a.pdf"})

	if err == nil {
		t.Fatal("corpo vazio foi aceito")
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Errorf("corpo vazio, mas SendDocument foi chamado %d vez(es)", n)
	}
}

// TestSendDocument_MimeType_RequestFieldWinsOverRemoteContentType prova a
// precedência de MIME no ramo URL: req.MimeType, quando presente, vence
// SEMPRE — nem o Content-Type remoto nem o sniffing chegam a valer.
func TestSendDocument_MimeType_RequestFieldWinsOverRemoteContentType(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return pdfBytes, "text/plain", nil // Content-Type mentiroso do servidor remoto.
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: documentURL, FileName: "a.pdf", MimeType: "application/x-custom",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mm.SendDocumentCalls); n != 1 {
		t.Fatalf("SendDocument chamado %d vez(es), quero exatamente 1", n)
	}
	if got := mm.SendDocumentCalls[0].Payload.MimeType; got != "application/x-custom" {
		t.Errorf("MimeType: got %q, want %q (o do request, nao o Content-Type remoto)", got, "application/x-custom")
	}
}

// TestSendDocument_MimeType_RemoteContentTypeWinsOverSniffingWhenReqEmpty
// prova o degrau de precedência A MAIS que SendDocument tem sobre SendImage:
// sem req.MimeType, o Content-Type declarado pelo servidor remoto no ramo
// URL vence o sniffing dos bytes — histórico: `git show 41bc8e2^:handlers.go`,
// linhas em torno de 926-928 ("if t.MimeType == "" { t.MimeType = ct }").
func TestSendDocument_MimeType_RemoteContentTypeWinsOverSniffingWhenReqEmpty(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			// pdfBytes sniffaria como "text/plain; charset=utf-8" via
			// http.DetectContentType (não começa com a assinatura binária
			// que o sniffer reconhece como PDF) — o Content-Type remoto
			// declarado aqui ("application/pdf") tem de vencer, provando
			// que a precedência realmente usa o valor remoto, e não caiu
			// no sniffing por acidente.
			return pdfBytes, "application/pdf", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: documentURL, FileName: "a.pdf",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendDocumentCalls[0].Payload.MimeType; got != "application/pdf" {
		t.Errorf("MimeType: got %q, want %q (Content-Type remoto, req.MimeType vazio)", got, "application/pdf")
	}
}

// TestSendDocument_MimeType_SniffedFromBytesWhenReqAndRemoteEmpty: sem
// req.MimeType e sem Content-Type remoto (string vazia), o tipo vem de
// sniffing dos bytes reais — o último degrau da precedência.
func TestSendDocument_MimeType_SniffedFromBytesWhenReqAndRemoteEmpty(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return []byte("plain text content"), "", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: documentURL, FileName: "a.pdf",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	got := mm.SendDocumentCalls[0].Payload.MimeType
	if got == "" || got == "application/pdf" {
		t.Errorf("MimeType sniffado inesperado: got %q", got)
	}
	if got != http.DetectContentType([]byte("plain text content")) {
		t.Errorf("MimeType: got %q, want o sniffing real dos bytes", got)
	}
}

// TestSendDocument_GenericMimeTypeAccepted prova que MIME genérico
// (application/zip, text/plain, ...) é aceito sem restrição — diferente de
// SendImage, que exige prefixo "image/". Documentos legitimamente carregam
// qualquer MIME.
func TestSendDocument_GenericMimeTypeAccepted(t *testing.T) {
	cases := []string{"application/zip", "text/plain", "application/octet-stream", "video/mp4"}
	for _, mime := range cases {
		t.Run(mime, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{
				FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
					return []byte("conteudo qualquer"), "application/octet-stream", nil
				},
			}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, domain.SendDocumentRequest{
					Phone: "5511987654321", Document: documentURL, FileName: "a.pdf", MimeType: mime,
				})

			if err != nil {
				t.Fatalf("MIME generico %q foi recusado: %v", mime, err)
			}
			if got := mm.SendDocumentCalls[0].Payload.MimeType; got != mime {
				t.Errorf("MimeType: got %q, want %q", got, mime)
			}
		})
	}
}

// TestSendDocument_CausalSuccess é o teste da causa: um envio bem-sucedido
// OBRIGATORIAMENTE busca a URL com o limite correto, sobe exatamente os
// bytes buscados com destinatário, legenda e FileName corretos, e Status
// só vale StatusSent DEPOIS que a porta de envio devolveu sucesso.
func TestSendDocument_CausalSuccess(t *testing.T) {
	sentAt := int64(1755500030)
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-document", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, resourceURL string, limit int64) ([]byte, string, error) {
			if resourceURL != documentURL {
				t.Errorf("URL buscada: got %q, want %q", resourceURL, documentURL)
			}
			if limit != fetchDocumentMaxBytesForTest {
				t.Errorf("limite: got %d, want %d", limit, fetchDocumentMaxBytesForTest)
			}
			return pdfBytes, "application/pdf", nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: documentURL, FileName: "relatorio-mensal.pdf", Caption: "legenda",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mm.SendDocumentCalls); n != 1 {
		t.Fatalf("SendDocument chamado %d vez(es), quero exatamente 1", n)
	}
	call := mm.SendDocumentCalls[0]
	if call.Target != domain.JID("5511987654321") {
		t.Errorf("destinatario: got %q, want %q", call.Target, "5511987654321")
	}
	if string(call.Payload.Bytes) != string(pdfBytes) {
		t.Errorf("bytes repassados a SendDocument divergem dos buscados")
	}
	if call.Payload.Caption != "legenda" {
		t.Errorf("Caption: got %q, want %q", call.Payload.Caption, "legenda")
	}
	if call.Payload.FileName != "relatorio-mensal.pdf" {
		t.Errorf("FileName: got %q, want %q", call.Payload.FileName, "relatorio-mensal.pdf")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-document" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "wire-id-document")
	}
	if result.Timestamp != sentAt {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt)
	}
}

// TestSendDocument_EmptyCaptionPreserved: Caption vazio no request chega
// vazio a SendDocument — não confundido com "ausente" nem substituído.
func TestSendDocument_EmptyCaptionPreserved(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return pdfBytes, "application/pdf", nil },
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: documentURL, FileName: "a.pdf",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendDocumentCalls[0].Payload.Caption; got != "" {
		t.Errorf("Caption: got %q, want vazio", got)
	}
}

// TestSendDocument_ClientSuppliedIDIsForwardedButServerIDWins: o Id do
// cliente é repassado a SendDocument, mas o message_id publicado é o que a
// porta devolveu.
func TestSendDocument_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return pdfBytes, "application/pdf", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: documentURL, FileName: "a.pdf", ID: "id-do-cliente",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendDocument_DownstreamFailureNeverProducesSent: SendDocument (upload
// e/ou envio) falhando nunca pode virar Status=StatusSent nem resultado
// não-nil — a garantia central anti-falso-sucesso, agora para documentos.
func TestSendDocument_DownstreamFailureNeverProducesSent(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(context.Context, string, domain.JID, domain.MediaPayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return pdfBytes, "application/pdf", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{Phone: "5511987654321", Document: documentURL, FileName: "a.pdf"})

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

// TestSendDocument_AcquisitionOK_UploadOK_SendFail_NeverSent é o caso
// obrigatório: aquisição (fetch/decode) OK e upload OK (simulado dentro do
// SendDocumentFunc, que representa a porta upload+envio) seguidos de falha
// de envio NÃO produzem Status=sent — sem rollback de upload inventado.
func TestSendDocument_AcquisitionOK_UploadOK_SendFail_NeverSent(t *testing.T) {
	sendErr := errors.New("sendmessage: boom apos upload bem-sucedido")
	uploadThenSendCalled := false
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(context.Context, string, domain.JID, domain.MediaPayload, string) (domain.MessageSendResult, error) {
			uploadThenSendCalled = true
			return domain.MessageSendResult{}, sendErr
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return pdfBytes, "application/pdf", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{Phone: "5511987654321", Document: documentURL, FileName: "a.pdf"})

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

// TestSendDocument_RealFetchIntegration exercita o use case com um
// appport.MediaFetcher REAL (opengraph.URLFetcher) contra um
// httptest.Server — não um dublê do fetch. Cobre a integração completa,
// incluindo a passagem pela seam SSRF-safe já provada em CAP-02.
func TestSendDocument_RealFetchIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBytes)
	}))
	defer srv.Close()

	mm := &contractsfake.MediaMessenger{}
	mf := opengraph.NewURLFetcher(srv.Client())
	logger := &contractsfake.Logger{}

	result, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{Phone: "5511987654321", Document: srv.URL, FileName: "a.pdf"})

	if err != nil {
		t.Fatalf("integracao com fetch real falhou: %v", err)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if n := len(mm.SendDocumentCalls); n != 1 {
		t.Fatalf("SendDocument chamado %d vez(es), quero exatamente 1", n)
	}
	if string(mm.SendDocumentCalls[0].Payload.Bytes) != string(pdfBytes) {
		t.Error("bytes buscados pelo fetch real divergem do corpo servido")
	}
}

// TestSendDocument_SSRF_LoopbackBlocked prova que Document passa pela MESMA
// seam SSRF-safe já provada em CAP-02 — não um caminho de fetch paralelo.
// Loopback bloqueado é o caso mínimo desta garantia (as dezenas de casos já
// cobertos por opengraph.URLFetcher em CAP-02 não são duplicados aqui).
func TestSendDocument_SSRF_LoopbackBlocked(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := opengraph.NewURLFetcher(http.DefaultClient)
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: "http://127.0.0.1:1/relatorio.pdf", FileName: "a.pdf",
		})

	if err == nil {
		t.Fatal("URL loopback foi aceita")
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Errorf("loopback deveria ser bloqueado antes de SendDocument, mas foi chamado %d vez(es)", n)
	}
}

// --- ramo data URI ------------------------------------------------------

// TestSendDocument_DataURI_CausalSuccess é o teste da causa para o ramo
// data URI: prova que uma data URI válida é decodificada LOCALMENTE (o
// MediaFetcher nunca é tocado), os bytes decodificados batem com os bytes
// originais, o MIME final vem de req.MimeType (nunca do rótulo declarado
// pela data URI), e o resultado só vira StatusSent depois que SendDocument
// confirma sucesso.
func TestSendDocument_DataURI_CausalSuccess(t *testing.T) {
	sentAt := int64(1755500040)
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-document-datauri", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	uri := documentDataURI("application/pdf", pdfBytes)
	result, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: uri, FileName: "relatorio.pdf", Caption: "legenda",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("data URI, mas MediaFetcher foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendDocumentCalls); n != 1 {
		t.Fatalf("SendDocument chamado %d vez(es), quero exatamente 1", n)
	}
	call := mm.SendDocumentCalls[0]
	if string(call.Payload.Bytes) != string(pdfBytes) {
		t.Error("bytes decodificados divergem dos originais")
	}
	if call.Payload.FileName != "relatorio.pdf" {
		t.Errorf("FileName: got %q, want %q", call.Payload.FileName, "relatorio.pdf")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
}

// TestSendDocument_DataURI_GenericMimeAccepted prova que "data:" genérico
// (não só "data:image") é o discriminador — application/pdf, application/zip
// e text/plain em data URI são todos aceitos, diferente de SendImage (que
// exige literalmente "data:image").
func TestSendDocument_DataURI_GenericMimeAccepted(t *testing.T) {
	cases := []string{"application/pdf", "application/zip", "text/plain", "application/octet-stream"}
	for _, mime := range cases {
		t.Run(mime, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			logger := &contractsfake.Logger{}

			uri := documentDataURI(mime, []byte("conteudo qualquer"))
			_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, domain.SendDocumentRequest{
					Phone: "5511987654321", Document: uri, FileName: "a.bin",
				})

			if err != nil {
				t.Fatalf("data URI com MIME %q foi recusada: %v", mime, err)
			}
			if n := len(mm.SendDocumentCalls); n != 1 {
				t.Fatalf("SendDocument chamado %d vez(es), quero exatamente 1", n)
			}
		})
	}
}

// TestSendDocument_DataURI_MimeType_LabelIsNotTrusted prova que o rótulo
// de MIME embutido NA PRÓPRIA data URI nunca alimenta o Mimetype final —
// nem quando req.MimeType está ausente. Sem req.MimeType, o valor final
// vem do sniffing dos bytes DECODIFICADOS, não do rótulo declarado (que
// aqui mente: "application/pdf" sobre bytes de texto puro).
func TestSendDocument_DataURI_MimeType_LabelIsNotTrusted(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	plainText := []byte("isto NAO e um PDF, e texto puro")
	uri := documentDataURI("application/pdf", plainText) // rótulo mentiroso.

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: uri, FileName: "a.txt",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	got := mm.SendDocumentCalls[0].Payload.MimeType
	want := http.DetectContentType(plainText)
	if got != want {
		t.Errorf("MimeType: got %q, want %q (sniffing dos bytes, nao o rotulo da data URI)", got, want)
	}
	if got == "application/pdf" {
		t.Error("MimeType usou o rotulo mentiroso da data URI")
	}
}

// TestSendDocument_DataURI_MimeType_RequestFieldWinsOverLabel: req.MimeType
// vence tanto o rótulo da data URI quanto o sniffing.
func TestSendDocument_DataURI_MimeType_RequestFieldWinsOverLabel(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	uri := documentDataURI("application/zip", pdfBytes) // rótulo diferente do req.MimeType.

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: uri, FileName: "a.pdf", MimeType: "application/x-custom",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendDocumentCalls[0].Payload.MimeType; got != "application/x-custom" {
		t.Errorf("MimeType: got %q, want %q (req.MimeType tem de vencer)", got, "application/x-custom")
	}
}

// TestSendDocument_DataURI_MalformedBase64_Rejected: base64 malformado
// nunca vira sucesso.
func TestSendDocument_DataURI_MalformedBase64_Rejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{
			Phone: "5511987654321", Document: "data:application/pdf;base64,%%%nao-e-base64%%%", FileName: "a.pdf",
		})

	if err == nil {
		t.Fatal("base64 malformado foi aceito")
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Fatalf("base64 malformado, mas SendDocument foi chamado %d vez(es)", n)
	}
}

// TestSendDocument_URLBranch_NotCapturedByDataURIDiscrimination é o teste
// de conservação: uma URL http(s) comum não pode ser capturada pela
// discriminação "data:" genérica.
func TestSendDocument_URLBranch_NotCapturedByDataURIDiscrimination(t *testing.T) {
	sentAt := int64(1755500050)
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-url-conservation", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return pdfBytes, "application/pdf", nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendDocumentRequest{Phone: "5511987654321", Document: documentURL, FileName: "a.pdf"})

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

// TestSendDocument_FileName_NeverTouchesFilesystem prova que um FileName
// hostil ("../../etc/passwd", caminho absoluto, etc.) é metadata pura: o
// use case nunca a usa para abrir/ler/escrever nada no filesystem local —
// só a repassa como campo de domain.MediaPayload.FileName.
func TestSendDocument_FileName_NeverTouchesFilesystem(t *testing.T) {
	hostileNames := []string{
		"../../etc/passwd",
		"/etc/passwd",
		"../../../../../../etc/shadow",
		"C:\\Windows\\System32\\config\\SAM",
	}
	for _, name := range hostileNames {
		t.Run(name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{
				FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return pdfBytes, "application/pdf", nil },
			}
			logger := &contractsfake.Logger{}

			// Se o use case tentasse abrir/criar um arquivo com este nome,
			// o teste rodaria em um diretório de trabalho arbitrário (o do
			// `go test`) sem qualquer chamada de os.Open/os.Create — a
			// ausência de qualquer efeito colateral de filesystem (o teste
			// não falha por I/O, não cria arquivo nenhum no diretório de
			// teste) é a prova de que FileName é só o campo de metadata
			// repassado adiante.
			_, err := message.NewSendDocumentUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, domain.SendDocumentRequest{
					Phone: "5511987654321", Document: documentURL, FileName: name,
				})

			if err != nil {
				t.Fatalf("FileName hostil %q foi recusado como se fosse invalido (deveria ser metadata pura): %v", name, err)
			}
			if got := mm.SendDocumentCalls[0].Payload.FileName; got != name {
				t.Errorf("FileName repassado: got %q, want %q (metadata pura, sem sanitizacao)", got, name)
			}
		})
	}
}
