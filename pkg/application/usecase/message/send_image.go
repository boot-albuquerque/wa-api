package message

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// fetchImageMaxBytes é o teto de bytes aceito ao buscar uma imagem por URL
// externa — paridade com o limite histórico do wuzapi (constante
// fetchImageMaxBytes, ver `git grep -n "fetchImageMaxBytes\s*=" 41bc8e2^ --
// '*.go'`). appport.MediaFetcher aplica este limite DURANTE a leitura
// (io.LimitReader em opengraph.FetchURLBytes), não só contra um
// Content-Length declarado pelo servidor remoto.
const fetchImageMaxBytes int64 = 16 * 1024 * 1024

// SendImageUseCase envia uma imagem de verdade: busca a URL externa por
// infra SSRF-safe com limite de tamanho, sobe o anexo e envia a mensagem
// pelo wa-noise. Só devolve domain.StatusSent depois que o envio retorna
// sucesso — nunca antes (mesma disciplina de SendMessageUseCase, CAP-01).
//
// Escopo: apenas o ramo de URL http(s) de domain.SendImageRequest.Image. O
// ramo data URI (envio de arquivo/base64) é CAP-03 e fica fora — ver
// isHTTPImageURL.
type SendImageUseCase struct {
	media   appport.MediaMessenger
	jids    appport.JIDResolver
	fetcher appport.MediaFetcher
	logger  appport.Logger
}

// NewSendImageUseCase cria uma nova instância do usecase.
func NewSendImageUseCase(mm appport.MediaMessenger, jr appport.JIDResolver, mf appport.MediaFetcher, l appport.Logger) *SendImageUseCase {
	return &SendImageUseCase{
		media:   mm,
		jids:    jr,
		fetcher: mf,
		logger:  l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário, busca a
// imagem pela URL informada, sobe o anexo e envia a mensagem pela porta de
// verdade.
func (uc *SendImageUseCase) Execute(ctx context.Context, txtID string, req domain.SendImageRequest) (*domain.SendImageResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Image == "" {
		return nil, apperr.New("missing_image", apperr.CategoryValidation, "missing Image in payload", false, nil)
	}

	if err := uc.media.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send image payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	if !isHTTPImageURL(req.Image) {
		return nil, apperr.New("unsupported_image_source", apperr.CategoryValidation,
			"Image must be an http(s) URL in this endpoint; data URI support is out of scope (CAP-03)", false, nil)
	}

	// contentType (segundo valor de retorno) é intencionalmente
	// descartado: o Content-Type que o servidor remoto declara não é
	// confiável (ele pode mentir), então o tipo final vem de req.MimeType
	// ou, na ausência dele, de sniffing dos bytes reais — nunca do
	// cabeçalho HTTP. Mesmo comportamento do handlers.go histórico, que
	// montava ImageMessage.Mimetype a partir de t.MimeType ou
	// http.DetectContentType(filedata), nunca do Content-Type da resposta.
	data, _, err := uc.fetcher.FetchBytes(ctx, req.Image, fetchImageMaxBytes)
	if err != nil {
		uc.logger.Warn(ctx, "failed to fetch image url", "txtID", txtID, "error", err)
		return nil, apperr.New("image_fetch_failed", apperr.CategoryValidation, "failed to fetch image from url", false, err)
	}
	if len(data) == 0 {
		return nil, apperr.New("empty_image_body", apperr.CategoryValidation, "fetched image body is empty", false, nil)
	}

	mimeType := req.MimeType
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, apperr.New("invalid_image_mime_type", apperr.CategoryValidation, "resolved MIME type is not an image type", false, nil)
	}

	payload := domain.MediaPayload{Bytes: data, MimeType: mimeType, Caption: req.Caption}

	sent, err := uc.media.SendImage(ctx, txtID, recipient, payload, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send image message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendImageResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "image sent", "msgID", result.MessageID)
	return result, nil
}

// isHTTPImageURL reconhece o ramo URL de domain.SendImageRequest.Image —
// mesma regra do isHTTPURL histórico (`git show 41bc8e2^:helpers.go`, linha
// 160): scheme http ou https e host não vazio. Qualquer outro scheme
// (file://, ftp://, gopher://, ou o prefixo "data:image" do ramo CAP-03)
// cai fora deste endpoint em vez de ser aceito silenciosamente.
func isHTTPImageURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}
