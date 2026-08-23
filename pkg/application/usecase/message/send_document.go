package message

import (
	"context"
	"net/url"
	"strings"

	"github.com/vincent-petithory/dataurl"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// fetchDocumentMaxBytes é o teto de bytes aceito ao buscar um documento por
// URL externa — paridade com o limite histórico do wuzapi (constante
// fetchDocumentMaxBytes, ver `git grep -n "fetchDocumentMaxBytes\s*=" 41bc8e2^
// -- '*.go'`, valor 100 * 1024 * 1024). appport.MediaFetcher aplica este
// limite DURANTE a leitura (io.LimitReader em opengraph.FetchURLBytes), não
// só contra um Content-Length declarado pelo servidor remoto.
//
// CAP-04 (divergência D1, deliberada, mesmo racional de CAP-03 para
// SendImage): historicamente o ramo data URI não tinha teto nenhum
// (fetchDocumentMaxBytes só valia para o ramo URL, ver
// `git show 41bc8e2^:handlers.go`). Este projeto passa a aplicar o MESMO
// teto aos dois ramos — sem isso, o limite do ramo URL vira um bypass
// trivial (basta o cliente mandar o documento como data URI em vez de URL).
const fetchDocumentMaxBytes int64 = 100 * 1024 * 1024

// dataURIPrefix é o discriminador do ramo data URI de
// SendDocumentRequest.Document (`git show 41bc8e2^:handlers.go`, em torno da
// linha 916): QUALQUER data URI ("data:" genérico), diferente do
// dataImagePrefix ("data:image") usado por SendImageRequest.Image — um
// documento legitimamente pode ser "data:application/pdf;base64,...",
// "data:application/zip;base64,...", etc. Não restringe por MIME aqui: a
// discriminação de transporte e a validação de MIME são responsabilidades
// separadas neste use case (diferente de SendImage, onde a discriminação
// "data:image" faz dupla função).
const dataURIPrefix = "data:"

// SendDocumentUseCase envia um documento de verdade: obtém os bytes (por URL
// externa via infra SSRF-safe, ou por decode local de data URI), sobe o
// anexo e envia a mensagem pelo wa-noise. Só devolve domain.StatusSent
// depois que o envio retorna sucesso — nunca antes (mesma disciplina de
// SendImageUseCase, CAP-02/CAP-03).
//
// domain.SendDocumentRequest.Document é uma união de dois transportes de
// OBTENÇÃO dos bytes — URL http(s) (via appport.MediaFetcher) e data URI
// (decode local, sem tocar a rede) — que convergem no mesmo protocolo de
// envio (appport.MediaMessenger). Ver isDataURI e isHTTPDocumentURL para a
// discriminação.
type SendDocumentUseCase struct {
	media   appport.MediaMessenger
	jids    appport.JIDResolver
	fetcher appport.MediaFetcher
	logger  appport.Logger
}

// NewSendDocumentUseCase cria uma nova instância do usecase.
func NewSendDocumentUseCase(mm appport.MediaMessenger, jr appport.JIDResolver, mf appport.MediaFetcher, l appport.Logger) *SendDocumentUseCase {
	return &SendDocumentUseCase{
		media:   mm,
		jids:    jr,
		fetcher: mf,
		logger:  l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário, obtém o
// documento (fetch por URL ou decode local de data URI, conforme o
// transporte discriminado), sobe o anexo e envia a mensagem pela porta de
// verdade.
func (uc *SendDocumentUseCase) Execute(ctx context.Context, txtID string, req domain.SendDocumentRequest) (*domain.SendDocumentResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Document == "" {
		return nil, apperr.New("missing_document", apperr.CategoryValidation, "missing Document in payload", false, nil)
	}
	if req.FileName == "" {
		return nil, apperr.New("missing_filename", apperr.CategoryValidation, "missing FileName in payload", false, nil)
	}

	if err := uc.media.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send document payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	var data []byte
	// reqMimeType começa como req.MimeType e, apenas no ramo URL e apenas
	// quando vazio, recebe o Content-Type declarado pelo servidor remoto —
	// um degrau de precedência A MAIS do que SendImage (que nunca confia no
	// Content-Type remoto). O rótulo embutido na PRÓPRIA data URI nunca
	// alimenta reqMimeType, nos dois ramos: dataurl.DecodeString devolve o
	// rótulo declarado, mas resolveMimeType só recebe req.MimeType e os
	// bytes decodificados — nunca decoded.ContentType().
	reqMimeType := req.MimeType

	switch {
	case isDataURI(req.Document):
		// Decode LOCAL — nunca passa por MediaFetcher. MediaFetcher
		// representa fetch EXTERNO (SSRF-safe); data URI já é o payload
		// inteiro na requisição, sem envolver rede nenhuma, e manter as
		// duas coisas separadas é o que preserva as preocupações de SSRF
		// restritas ao ramo URL (arquitetura exigida do slice).
		var err error
		data, err = uc.decodeDataURIDocument(ctx, txtID, req.Document)
		if err != nil {
			return nil, err
		}

	case isHTTPDocumentURL(req.Document):
		var contentType string
		var err error
		data, contentType, err = uc.fetcher.FetchBytes(ctx, req.Document, fetchDocumentMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "failed to fetch document url", "txtID", txtID, "error", err)
			return nil, apperr.New("document_fetch_failed", apperr.CategoryValidation, "failed to fetch document from url", false, err)
		}
		// Precedência histórica do ramo URL (`git show 41bc8e2^:handlers.go`,
		// linhas em torno de 926-928): só o Content-Type remoto preenche
		// reqMimeType quando o chamador não declarou um — e só depois disso
		// entra em jogo o sniffing dos bytes (resolveMimeType). O
		// Content-Type remoto NUNCA sobrepõe um req.MimeType explícito.
		if reqMimeType == "" {
			reqMimeType = contentType
		}

	default:
		return nil, apperr.New("unsupported_document_source", apperr.CategoryValidation,
			`document data should start with "data:" or be a valid HTTP URL`, false, nil)
	}

	if len(data) == 0 {
		return nil, apperr.New("empty_document_body", apperr.CategoryValidation, "document body is empty", false, nil)
	}

	mimeType := resolveMimeType(reqMimeType, data)

	payload := domain.MediaPayload{Bytes: data, MimeType: mimeType, Caption: req.Caption, FileName: req.FileName}

	sent, err := uc.media.SendDocument(ctx, txtID, recipient, payload, req.ReplyTo, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send document message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendDocumentResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "document sent", "msgID", result.MessageID)
	return result, nil
}

// isDataURI reconhece o ramo data URI de domain.SendDocumentRequest.Document
// — QUALQUER data URI ("data:" genérico), diferente de isDataURIImage em
// send_image.go (que exige "data:image" especificamente). Um documento
// legitimamente carrega MIME genérico (application/pdf, application/zip,
// text/plain, ...), então restringir o prefixo por MIME aqui rejeitaria
// casos de uso válidos.
func isDataURI(raw string) bool {
	return strings.HasPrefix(raw, dataURIPrefix)
}

// decodeDataURIDocument decodifica uma data URI localmente (sem tocar rede)
// e devolve só os bytes decodificados. O rótulo de MIME que a própria data
// URI declara NÃO é usado como fonte do MIME final em nenhum lugar deste
// use case — ver o comentário de reqMimeType em Execute. O teto aplicado é
// SEMPRE fetchDocumentMaxBytes (100MB) em produção — ver
// decodeDataURIWithLimit para a lógica de fronteira em si, extraída como
// função pura para ser exercitada em teste com um limite sintético pequeno
// sem alocar payloads de 100MB (ver TestDecodeDataURIWithLimit_Boundary,
// arquivo interno ao pacote).
func (uc *SendDocumentUseCase) decodeDataURIDocument(ctx context.Context, txtID, raw string) ([]byte, error) {
	data, err := decodeDataURIWithLimit(raw, fetchDocumentMaxBytes)
	if err != nil {
		if appErr, ok := err.(*apperr.AppError); ok && appErr.Code == "document_too_large" {
			uc.logger.Warn(ctx, "data uri document payload exceeds size limit", "txtID", txtID, "limit", fetchDocumentMaxBytes)
		} else {
			uc.logger.Warn(ctx, "failed to decode data uri document", "txtID", txtID, "error", err)
		}
		return nil, err
	}
	return data, nil
}

// decodeDataURIWithLimit é a lógica de fronteira PURA (sem logging, sem
// dependência do use case) por trás de decodeDataURIDocument — extraída
// para que o teste da fronteira 100MB/100MB+1 possa injetar um limit
// sintético pequeno em vez de alocar payloads de 100MB só para provar uma
// comparação (ver CLAUDE.md, "Teste de boundary sem desperdiçar RAM"). A
// constante fetchDocumentMaxBytes só entra em jogo em produção, via
// decodeDataURIDocument — TestSendDocument_DataURIMaxBytesConstant prova,
// separadamente e sem alocação, que ela vale exatamente 100MB.
//
// O tamanho decodificado é medido de duas formas, mesmo racional de
// decodeDataURIImage (send_image.go):
//  1. Uma estimativa BARATA e exata a partir do comprimento do texto
//     base64 (sem decodificar), para rejeitar cedo payloads grosseiramente
//     acima do limite sem alocar o buffer decodificado inteiro.
//  2. A contagem real de bytes decodificados, que é a autoridade — cobre os
//     casos em que a estimativa não se aplica e define o limite exato na
//     fronteira.
func decodeDataURIWithLimit(raw string, limit int64) ([]byte, error) {
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		meta, encoded := raw[:idx], raw[idx+1:]
		if strings.Contains(meta, ";base64") {
			if n, ok := decodedBase64Len(encoded); ok && n > limit {
				return nil, apperr.New("document_too_large", apperr.CategoryValidation,
					"decoded document payload exceeds the maximum allowed size", false, nil)
			}
		}
	}

	decoded, err := dataurl.DecodeString(raw)
	if err != nil {
		return nil, apperr.New("invalid_data_uri", apperr.CategoryValidation,
			"could not decode base64 encoded data from payload", false, err)
	}
	if int64(len(decoded.Data)) > limit {
		return nil, apperr.New("document_too_large", apperr.CategoryValidation,
			"decoded document payload exceeds the maximum allowed size", false, nil)
	}
	return decoded.Data, nil
}

// isHTTPDocumentURL reconhece o ramo URL de domain.SendDocumentRequest.Document
// — mesma regra do isHTTPURL histórico (`git show 41bc8e2^:helpers.go`, linha
// 160) e de isHTTPImageURL em send_image.go: scheme http ou https e host
// não vazio.
func isHTTPDocumentURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}
