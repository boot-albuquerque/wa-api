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

// fetchVideoMaxBytes é o teto de bytes aceito para um anexo de vídeo —
// paridade com o limite histórico do wuzapi (constante fetchVideoMaxBytes,
// ver `git grep -n "fetchVideoMaxBytes\s*=" 41bc8e2^ -- '*.go'`, valor
// 100*1024*1024).
//
// CAP-06 (divergência D1, mesmo racional de CAP-03/CAP-04/CAP-05):
// historicamente o teto só valia para o ramo URL (`git show
// 41bc8e2^:handlers.go`, em torno da linha 1583 — fetchURLBytes(...,
// fetchVideoMaxBytes) só é chamado no ramo isHTTPURL). Este projeto aplica
// o MESMO teto ao ramo data URI — sem isso, o limite do ramo URL vira um
// bypass trivial.
const fetchVideoMaxBytes int64 = 100 * 1024 * 1024

// minVideoBytes is the minimum payload size for a valid video file. Real
// video containers (MP4, WebM, MKV) carry headers and codec data that make
// even an empty-track file larger than this. A 32-byte MP4, for instance,
// is structurally impossible — http.DetectContentType maps it to
// "application/octet-stream", and resolveMimeType falls back to that,
// which is not a video/* type. The floor catches truncated or garbage
// payloads before they reach the upload path.
const minVideoBytes = 256

// dataPrefix é o discriminador do ramo data URI de SendVideoRequest.Video
// (`git show 41bc8e2^:handlers.go`, em torno da linha 1583:
// `t.Video[0:4] == "data"`) — a MAIS FROUXA das quatro discriminações de
// data URI deste projeto: os 4 primeiros caracteres têm de ser "data",
// SEM os dois-pontos, diferente de dataImagePrefix ("data:image",
// send_image.go), dataAudioPrefix ("data:audio/", send_audio.go) e
// dataURIPrefix ("data:", send_document.go).
//
// BUG LATENTE HISTÓRICO NÃO REPRODUZIDO: `t.Video[0:4]` sem checar o
// tamanho de Video antes de indexar PANICA quando Video tem 1..3
// caracteres (Video vazio já é barrado antes pela checagem de
// req.Video == "", mas "dat" não). isDataVideo usa uma checagem segura de
// prefixo (comparação por fatia, que já valida o tamanho) para preservar a
// SEMÂNTICA (prefixo "data") sem o defeito. TestSendVideo_ShortVideoNoPanic
// prova que "dat" não panica e vira 400.
const dataPrefix = "data"

// SendVideoUseCase envia um vídeo de verdade: obtém os bytes (por URL
// externa via infra SSRF-safe, ou por decode local de data URI), resolve o
// MIME pela precedência histórica de dois níveis (igual a Image), sobe o
// anexo e envia a mensagem pelo noise. Só devolve domain.StatusSent
// depois que o envio retorna sucesso — mesma disciplina de
// SendImageUseCase/SendDocumentUseCase/SendAudioUseCase.
//
// FORA DE ESCOPO, explicitamente: PTV, video note, round video. Nenhuma
// flag desse tipo existe em domain.SendVideoRequest nem em nenhum outro
// caminho de ENVIO em pkg/ — só há IsViewOnce no lado de RECEBIMENTO
// (eventhandler_*), que este slice não toca.
type SendVideoUseCase struct {
	media   appport.MediaMessenger
	jids    appport.JIDResolver
	fetcher appport.MediaFetcher
	logger  appport.Logger
}

// NewSendVideoUseCase cria uma nova instância do usecase.
func NewSendVideoUseCase(mm appport.MediaMessenger, jr appport.JIDResolver, mf appport.MediaFetcher, l appport.Logger) *SendVideoUseCase {
	return &SendVideoUseCase{
		media:   mm,
		jids:    jr,
		fetcher: mf,
		logger:  l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário, obtém o
// vídeo (fetch por URL ou decode local de data URI, conforme o transporte
// discriminado), resolve o MIME (dois níveis, ver resolveMimeType em
// send_image.go — reaproveitada aqui de propósito: a precedência histórica
// de Video é idêntica à de Image, MimeType do request senão sniffing dos
// bytes, e o Content-Type remoto NUNCA ganha precedência), sobe o anexo e
// envia a mensagem pela porta de verdade.
func (uc *SendVideoUseCase) Execute(ctx context.Context, txtID string, req domain.SendVideoRequest) (*domain.SendVideoResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Video == "" {
		return nil, apperr.New("missing_video", apperr.CategoryValidation, "missing Video in payload", false, nil)
	}

	if err := uc.media.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send video payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	var data []byte

	switch {
	case isDataVideo(req.Video):
		var err error
		data, err = uc.decodeDataVideo(ctx, txtID, req.Video)
		if err != nil {
			return nil, err
		}

	case isHTTPVideoURL(req.Video):
		var err error
		// contentType (segundo valor de retorno) é intencionalmente
		// descartado: o nível 1 real é SendVideoRequest.MimeType, e o
		// nível 2 é sniffing dos bytes (resolveMimeType).
		data, _, err = uc.fetcher.FetchBytes(ctx, req.Video, fetchVideoMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "failed to fetch video url", "txtID", txtID, "error", err)
			return nil, apperr.New("video_fetch_failed", apperr.CategoryValidation, "failed to fetch video from url", false, err)
		}

	default:
		return nil, apperr.New("unsupported_video_source", apperr.CategoryValidation,
			`data should start with "data:mime/type;base64,"`, false, nil)
	}

	if len(data) == 0 {
		return nil, apperr.New("empty_video_body", apperr.CategoryValidation, "video body is empty", false, nil)
	}
	if len(data) < minVideoBytes {
		return nil, apperr.New("video_too_small", apperr.CategoryValidation, "video payload too small to be valid", false, nil)
	}

	mimeType := resolveMimeType(req.MimeType, data)
	// Reject when the resolved MIME is determinate and NOT a video type.
	// "application/octet-stream" (Go's "I don't know") is allowed through
	// because http.DetectContentType cannot recognize MP4 containers.
	if mimeType != "application/octet-stream" && !strings.HasPrefix(mimeType, "video/") {
		return nil, apperr.New("invalid_video_mime_type", apperr.CategoryValidation, "resolved MIME type is not a video type", false, nil)
	}

	payload := domain.MediaPayload{Bytes: data, MimeType: mimeType, Caption: req.Caption, JPEGThumbnail: req.JPEGThumbnail}

	sent, err := uc.media.SendVideo(ctx, txtID, recipient, payload, req.ReplyTo, req.MentionedJID, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send video message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendVideoResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "video sent", "msgID", result.MessageID)
	return result, nil
}

// isDataVideo reconhece o ramo data URI de domain.SendVideoRequest.Video —
// a discriminação MAIS FROUXA das quatro deste projeto: os 4 primeiros
// caracteres têm de ser exatamente "data" (sem os dois-pontos), aceitando
// qualquer MIME declarado após o "data:" (inclusive um "data:image/..."
// disfarçado de vídeo — o histórico nunca filtrou por MIME neste ramo, e
// reproduzir uma checagem mais estreita aqui divergiria do contrato
// original sem necessidade). Comparação por fatia — não indexação direta
// (`raw[0:4]`) — para não reproduzir o panic histórico em entradas de
// 1..3 caracteres.
func isDataVideo(raw string) bool {
	return len(raw) >= len(dataPrefix) && raw[:len(dataPrefix)] == dataPrefix
}

// decodeDataVideo decodifica uma data URI localmente (sem tocar rede) e
// devolve só os bytes decodificados — o rótulo de MIME que a própria data
// URI declara não é usado (ver o comentário de resolveMimeType em
// Execute). O teto aplicado é SEMPRE fetchVideoMaxBytes (100MB) em
// produção — ver decodeVideoDataURIWithLimit para a lógica de fronteira
// pura, exercitada em teste com um limite sintético pequeno sem alocar
// payloads de 100MB.
func (uc *SendVideoUseCase) decodeDataVideo(ctx context.Context, txtID, raw string) ([]byte, error) {
	data, err := decodeVideoDataURIWithLimit(raw, fetchVideoMaxBytes)
	if err != nil {
		if appErr, ok := err.(*apperr.AppError); ok && appErr.Code == "video_too_large" {
			uc.logger.Warn(ctx, "data uri video payload exceeds size limit", "txtID", txtID, "limit", fetchVideoMaxBytes)
		} else {
			uc.logger.Warn(ctx, "failed to decode data uri video", "txtID", txtID, "error", err)
		}
		return nil, err
	}
	return data, nil
}

// decodeVideoDataURIWithLimit é a lógica de fronteira PURA (sem logging,
// sem dependência do use case) por trás de decodeDataVideo — extraída para
// que o teste da fronteira 100MB/100MB+1 possa injetar um limit sintético
// pequeno em vez de alocar payloads de 100MB só para provar uma
// comparação (ver CLAUDE.md, "Teste de boundary sem desperdiçar RAM").
// TestSendVideo_DataURIMaxBytesConstant prova, separadamente e sem
// alocação, que fetchVideoMaxBytes vale exatamente 100MB. Mesma estratégia
// de duas medidas (estimativa barata pré-decode + contagem real
// pós-decode) de decodeDataURIWithLimit (send_document.go),
// decodeDataURIImage (send_image.go) e decodeAudioDataURIWithLimit
// (send_audio.go).
func decodeVideoDataURIWithLimit(raw string, limit int64) ([]byte, error) {
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		meta, encoded := raw[:idx], raw[idx+1:]
		if strings.Contains(meta, ";base64") {
			if n, ok := decodedBase64Len(encoded); ok && n > limit {
				return nil, apperr.New("video_too_large", apperr.CategoryValidation,
					"decoded video payload exceeds the maximum allowed size", false, nil)
			}
		}
	}

	decoded, err := dataurl.DecodeString(raw)
	if err != nil {
		return nil, apperr.New("invalid_data_uri", apperr.CategoryValidation,
			"could not decode base64 encoded data from payload", false, err)
	}
	if int64(len(decoded.Data)) > limit {
		return nil, apperr.New("video_too_large", apperr.CategoryValidation,
			"decoded video payload exceeds the maximum allowed size", false, nil)
	}
	return decoded.Data, nil
}

// isHTTPVideoURL reconhece o ramo URL de domain.SendVideoRequest.Video —
// mesma regra do isHTTPURL histórico (`git show 41bc8e2^:helpers.go`,
// linha 160) e de isHTTPImageURL/isHTTPDocumentURL/isHTTPAudioURL: scheme
// http ou https e host não vazio.
func isHTTPVideoURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}
