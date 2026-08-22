package message

import (
	"context"
	"strings"

	"github.com/vincent-petithory/dataurl"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// dataStickerPrefix é o discriminador do ramo data URI de
// domain.SendStickerRequest.Sticker: os 4 primeiros caracteres têm de ser
// exatamente "data" (sem os dois-pontos) — igual a dataPrefix/isDataVideo em
// send_video.go, e pela MESMA razão histórica: o fluxo original
// (`git show 41bc8e2^:handlers.go`, em torno da linha 1417) normaliza o
// ramo URL PARA DENTRO de uma data URI e então chama processStickerData
// incondicionalmente sobre o resultado — não há dois ramos de decode
// separados, só um discriminador de ENTRADA (URL vs "já é data URI/outra
// coisa") antes de convergir num único pipeline.
const dataStickerPrefix = "data"

// SendStickerUseCase envia um sticker de verdade: obtém os bytes (por URL
// externa via infra SSRF-safe, normalizada para dentro de uma data URI, ou
// por data URI já pronta vinda do cliente), converte via
// appport.StickerProcessor (que envolve pkg/infra/media/sticker —
// ProcessStickerData, byte a byte igual ao processStickerData histórico:
// conversão para WebP via ffmpeg + injeção de EXIF de pacote), sobe o
// resultado CONVERTIDO e envia a mensagem pelo wa-noise. Só devolve
// domain.StatusSent depois que o envio (não o upload, não a conversão)
// retorna sucesso — mesma disciplina de SendImageUseCase/SendVideoUseCase.
type SendStickerUseCase struct {
	media     appport.MediaMessenger
	jids      appport.JIDResolver
	fetcher   appport.MediaFetcher
	processor appport.StickerProcessor
	logger    appport.Logger
}

// NewSendStickerUseCase cria uma nova instância do usecase.
func NewSendStickerUseCase(mm appport.MediaMessenger, jr appport.JIDResolver, mf appport.MediaFetcher, sp appport.StickerProcessor, l appport.Logger) *SendStickerUseCase {
	return &SendStickerUseCase{
		media:     mm,
		jids:      jr,
		fetcher:   mf,
		processor: sp,
		logger:    l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário, obtém o
// sticker (fetch por URL, normalizado para data URI, ou a data URI que já
// veio do cliente), converte via appport.StickerProcessor, sobe o WebP
// CONVERTIDO (nunca o input cru) e envia a mensagem pela porta de verdade.
func (uc *SendStickerUseCase) Execute(ctx context.Context, txtID string, req domain.SendStickerRequest) (*domain.SendStickerResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Sticker == "" {
		return nil, apperr.New("missing_sticker", apperr.CategoryValidation, "missing Sticker in payload", false, nil)
	}

	if err := uc.media.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send sticker payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	// stickerData é o payload que vai para ProcessSticker — sempre uma data
	// URI (ou, na ausência de um discriminador reconhecido, o que quer que
	// req.Sticker seja, deixando ProcessSticker rejeitar). Ramo URL: busca
	// SSRF-safe com o MESMO teto de Image (fetchImageMaxBytes — sticker não
	// tem teto próprio, ver o comentário do achado abaixo) e normaliza para
	// dentro de uma data URI, replicando `git show 41bc8e2^:handlers.go` em
	// torno da linha 1417: mimeType vem do Content-Type remoto, com
	// fallback "image/webp" quando ele não começa com "image/".
	stickerData := req.Sticker

	switch {
	// isHTTPImageURL (send_image.go), não uma cópia isHTTPStickerURL: a
	// regra é literalmente idêntica (scheme http/https, host não vazio) e
	// não tem nada de específico de imagem — reaproveitada de propósito,
	// mesmo racional de resolveMimeType (send_image.go, reaproveitada por
	// send_video.go).
	case isHTTPImageURL(req.Sticker):
		data, contentType, err := uc.fetcher.FetchBytes(ctx, req.Sticker, fetchImageMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "failed to fetch sticker url", "txtID", txtID, "error", err)
			return nil, apperr.New("sticker_fetch_failed", apperr.CategoryValidation, "failed to fetch sticker from url", false, err)
		}
		mimeType := contentType
		if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			mimeType = "image/webp"
		}
		stickerData = dataurl.New(data, mimeType).String()

	case isDataSticker(req.Sticker):
		// CAP-07 (divergência D1, mesmo racional de CAP-03/CAP-04/CAP-05/
		// CAP-06): historicamente o teto de 16MB (fetchImageMaxBytes) só
		// valia para o ramo URL — este projeto aplica o MESMO teto ao ramo
		// data URI, sem o quê o limite do ramo URL vira um bypass trivial.
		// checkStickerDataURISize é intencionalmente uma checagem PURA
		// sobre o texto base64, sem decodificar: ProcessSticker decodifica
		// e converte na mesma chamada (sticker.ProcessStickerData), e
		// decodificar aqui só para medir tamanho decodificaria duas vezes e
		// reimplementaria parte do pipeline — o que CLAUDE.md proíbe.
		if err := checkStickerDataURISize(stickerData, fetchImageMaxBytes); err != nil {
			uc.logger.Warn(ctx, "data uri sticker payload exceeds size limit", "txtID", txtID, "limit", fetchImageMaxBytes)
			return nil, err
		}

	default:
		// Nenhum discriminador reconhecido (nem URL http(s), nem prefixo
		// "data"): stickerData segue como req.Sticker, sem checagem de
		// tamanho (não há o que medir com confiança), e ProcessSticker vai
		// rejeitar com a mensagem histórica ("data should start with
		// \"data:mime/type;base64,\"") — mesmo comportamento do fluxo
		// original, que nunca teve um terceiro ramo de erro explícito para
		// este caso.
	}

	// ProcessSticker é o ÚNICO ponto de conversão: decodifica stickerData,
	// converte para WebP (imagem via libwebp lossless, vídeo/gif via
	// ffmpeg 512x512@15fps) e embute EXIF de pacote quando packID/packName/
	// packPublisher/emojis não são todos vazios. O MIME final é o QUE O
	// PIPELINE devolve, não req.MimeType (que entra só como mimeOverride,
	// parâmetro 2) nem sniffing feito por este usecase — a precedência vive
	// dentro do pacote sticker.
	processed, detectedMimeType, err := uc.processor.ProcessSticker(ctx, stickerData, req.MimeType, req.PackID, req.PackName, req.PackPublisher, req.Emojis)
	if err != nil {
		uc.logger.Warn(ctx, "failed to process sticker", "txtID", txtID, "error", err)
		// Mapeamento de categoria embutido aqui (não numa função à parte):
		// reproduz a distinção histórica (`git show 41bc8e2^:handlers.go`,
		// em torno da linha 1470) entre falha de CONVERSÃO (ffmpeg
		// ausente/erro, formato não suportado) — erro de SERVIDOR (500,
		// CategoryInternal), o cliente não corrige o payload para fazer o
		// ffmpeg funcionar — e o resto (data URI malformada, base64
		// inválido, prefixo "data" ausente) — erro de VALIDAÇÃO (400,
		// CategoryValidation). A distinção usa a MESMA string sentinela do
		// histórico ("failed to convert"), presente nos dois pontos de wrap
		// de erro de ffmpeg em pkg/infra/media/sticker/exif.go:
		// ConvertToWebPSticker ("failed to convert video/gif sticker to
		// webp" / "failed to convert image sticker to webp"), que por sua
		// vez embrulha qualquer falha de RunFFmpegConversion — incluindo
		// ffmpeg ausente do PATH (exec.Command falha ao iniciar) e ffmpeg
		// presente mas retornando erro (exit code != 0).
		if strings.Contains(err.Error(), "failed to convert") {
			return nil, apperr.New("sticker_conversion_failed", apperr.CategoryInternal, "failed to convert sticker to webp", false, err)
		}
		return nil, apperr.New("sticker_processing_failed", apperr.CategoryValidation, "failed to process sticker payload", false, err)
	}

	payload := domain.MediaPayload{Bytes: processed, MimeType: detectedMimeType, PngThumbnail: req.PngThumbnail}

	sent, err := uc.media.SendSticker(ctx, txtID, recipient, payload, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send sticker message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendStickerResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "sticker sent", "msgID", result.MessageID)
	return result, nil
}

// checkStickerDataURISize é a checagem PURA de fronteira (sem I/O, sem
// decodificar) por trás do teto de 16MB do ramo data URI de
// SendStickerUseCase.Execute — mesma estratégia de estimativa barata e
// exata (para base64 bem formado, comprimento múltiplo de 4) de
// decodedBase64Len (send_image.go), usada aqui sozinha (sem uma segunda
// contagem pós-decode, ver o comentário no ponto de chamada) porque
// ProcessSticker é quem decodifica de verdade. limit é parâmetro — não a
// constante fetchImageMaxBytes direto — para que o teste de fronteira
// (16MB/16MB+1) use um limite sintético pequeno sem alocar payload de
// 16MB.
func checkStickerDataURISize(raw string, limit int64) error {
	idx := strings.IndexByte(raw, ',')
	if idx < 0 {
		return nil
	}
	meta, encoded := raw[:idx], raw[idx+1:]
	if !strings.Contains(meta, ";base64") {
		return nil
	}
	if n, ok := decodedBase64Len(encoded); ok && n > limit {
		return apperr.New("sticker_too_large", apperr.CategoryValidation,
			"decoded sticker payload exceeds the maximum allowed size", false, nil)
	}
	return nil
}

// isDataSticker reconhece o discriminador "data" (4 chars, sem os
// dois-pontos) que stickerData assume quando não vem de URL — chamada
// explicitamente por Execute (segundo case do switch de transporte), e fixa
// em código, com teste próprio, a MESMA regra seca que ProcessStickerData
// aplica internamente (strings.HasPrefix(stickerData, "data"),
// pkg/infra/media/sticker/exif.go) — dataStickerPrefix é a constante
// nomeada que a documenta.
func isDataSticker(raw string) bool {
	return len(raw) >= len(dataStickerPrefix) && raw[:len(dataStickerPrefix)] == dataStickerPrefix
}
