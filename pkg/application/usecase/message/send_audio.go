package message

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/vincent-petithory/dataurl"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// fetchAudioMaxBytes é o teto de bytes aceito para um anexo de áudio —
// paridade com o limite histórico do wuzapi (constante fetchAudioMaxBytes,
// ver `git grep -n "fetchAudioMaxBytes\s*=" 41bc8e2^ -- '*.go'`, valor
// 16*1024*1024). Deliberadamente MENOR que fetchDocumentMaxBytes (100MB,
// send_document.go) — áudio tem regra própria, não herdada de Document.
//
// CAP-05 (divergência D1, mesmo racional de CAP-03/CAP-04): historicamente
// o teto só valia para o ramo URL (`git show 41bc8e2^:handlers.go`, em
// torno da linha 1097 — fetchURLBytes(..., fetchAudioMaxBytes) só é chamado
// no ramo isHTTPURL). Este projeto aplica o MESMO teto ao ramo data URI —
// sem isso, o limite do ramo URL vira um bypass trivial.
const fetchAudioMaxBytes int64 = 16 * 1024 * 1024

// dataAudioPrefix é o discriminador do ramo data URI de
// SendAudioRequest.Audio (`git show 41bc8e2^:handlers.go`, em torno da
// linha 1088: `strings.HasPrefix(t.Audio, "data:audio/")`) — ESTREITO como
// dataImagePrefix em send_image.go ("data:audio/" literal), diferente de
// dataURIPrefix em send_document.go ("data:" genérico). Uma data URI com
// MIME não-áudio (ex.: "data:application/pdf;base64,...") não bate aqui,
// cai para isHTTPAudioURL — que também falha — e termina em
// unsupported_audio_source.
const dataAudioPrefix = "data:audio/"

// SendAudioUseCase envia um áudio de verdade: obtém os bytes (por URL
// externa via infra SSRF-safe, ou por decode local de data URI), resolve
// PTT e MIME pela precedência histórica, sobe o anexo e envia a mensagem
// pelo wa-noise. Só devolve domain.StatusSent depois que o envio retorna
// sucesso — mesma disciplina de SendImageUseCase/SendDocumentUseCase.
type SendAudioUseCase struct {
	media   appport.MediaMessenger
	jids    appport.JIDResolver
	fetcher appport.MediaFetcher
	logger  appport.Logger
}

// NewSendAudioUseCase cria uma nova instância do usecase.
func NewSendAudioUseCase(mm appport.MediaMessenger, jr appport.JIDResolver, mf appport.MediaFetcher, l appport.Logger) *SendAudioUseCase {
	return &SendAudioUseCase{
		media:   mm,
		jids:    jr,
		fetcher: mf,
		logger:  l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário, obtém o
// áudio (fetch por URL ou decode local de data URI), resolve PTT (default
// TRUE quando ausente) e MIME (precedência de 4 níveis, ver
// resolveAudioMimeType), sobe o anexo e envia a mensagem pela porta de
// verdade.
func (uc *SendAudioUseCase) Execute(ctx context.Context, txtID string, req domain.SendAudioRequest) (*domain.SendAudioResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Audio == "" {
		return nil, apperr.New("missing_audio", apperr.CategoryValidation, "missing Audio in payload", false, nil)
	}

	if err := uc.media.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send audio payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	var data []byte
	// detectedMime é o MIME "descoberto" durante a aquisição — o rótulo
	// declarado pela PRÓPRIA data URI (ramo data URI) ou o Content-Type
	// remoto, mas SOMENTE quando ele começa com "audio/" (ramo URL). Nível
	// 2 da precedência de resolveAudioMimeType — ver ali para os 4 níveis
	// completos. Diferente de SendDocument/SendImage: aqui o rótulo da
	// própria data URI É usado (`git show 41bc8e2^:handlers.go`, linha
	// 1088-1091: `detectedMime = dataURL.ContentType()`), porque o
	// histórico de Audio nunca desconfiou do rótulo — só desconfia do
	// resultado do sniffing "application/octet-stream" (nível 3).
	var detectedMime string

	switch {
	case isDataURIAudio(req.Audio):
		var err error
		data, detectedMime, err = uc.decodeDataURIAudio(ctx, txtID, req.Audio)
		if err != nil {
			return nil, err
		}

	case isHTTPAudioURL(req.Audio):
		var contentType string
		var err error
		data, contentType, err = uc.fetcher.FetchBytes(ctx, req.Audio, fetchAudioMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "failed to fetch audio url", "txtID", txtID, "error", err)
			return nil, apperr.New("audio_fetch_failed", apperr.CategoryValidation, "failed to fetch audio from url", false, err)
		}
		// Precedência histórica do ramo URL (`git show 41bc8e2^:handlers.go`,
		// linhas 1097-1103): o Content-Type remoto só alimenta detectedMime
		// quando começa com "audio/" — um servidor que declara
		// "text/html" (página de erro, redirect mal resolvido, etc.) não
		// contamina o MIME final.
		if strings.HasPrefix(strings.ToLower(contentType), "audio/") {
			detectedMime = contentType
		}

	default:
		return nil, apperr.New("unsupported_audio_source", apperr.CategoryValidation,
			"audio must be base64 (data:audio/) or valid HTTP URL", false, nil)
	}

	if len(data) == 0 {
		return nil, apperr.New("empty_audio_body", apperr.CategoryValidation, "audio body is empty", false, nil)
	}

	// ptt: nil no request significa TRUE (voice note) — não false. Inverter
	// este default mudaria o comportamento de todo cliente que hoje omite
	// o campo. Ver `git show 41bc8e2^:handlers.go`, linhas 1112-1115.
	ptt := true
	if req.PTT != nil {
		ptt = *req.PTT
	}

	mimeType := resolveAudioMimeType(req.MimeType, detectedMime, data, ptt)

	payload := domain.AudioPayload{Bytes: data, MimeType: mimeType, PTT: ptt, Seconds: req.Seconds}

	sent, err := uc.media.SendAudio(ctx, txtID, recipient, payload, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send audio message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendAudioResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "audio sent", "msgID", result.MessageID)
	return result, nil
}

// resolveAudioMimeType aplica a precedência histórica de QUATRO níveis
// exclusiva de Audio (`git show 41bc8e2^:handlers.go`, linhas 1105-1119):
//  1. reqMimeType (SendAudioRequest.MimeType), quando não vazio.
//  2. detectedMime: o rótulo da data URI (ramo data URI) ou o Content-Type
//     remoto quando começa com "audio/" (ramo URL) — ver o comentário de
//     detectedMime em Execute.
//  3. http.DetectContentType(data), SOMENTE se o resultado não for
//     "application/octet-stream" — o sniffing do Go não reconhece
//     OGG/Opus, MP3, AAC, M4A de forma confiável, então
//     "application/octet-stream" é tratado como "não consegui detectar" e
//     cai para o nível 4, em vez de virar o MIME final.
//  4. Fallback por PTT: "audio/ogg; codecs=opus" quando ptt, senão
//     "audio/mpeg" — o comportamento de voice note vs. arquivo de áudio
//     comum quando nada mais identificou o tipo.
func resolveAudioMimeType(reqMimeType, detectedMime string, data []byte, ptt bool) string {
	switch {
	case reqMimeType != "":
		return reqMimeType
	case detectedMime != "":
		return detectedMime
	}
	if sniffed := http.DetectContentType(data); sniffed != "application/octet-stream" {
		return sniffed
	}
	if ptt {
		return "audio/ogg; codecs=opus"
	}
	return "audio/mpeg"
}

// isDataURIAudio reconhece o ramo data URI de domain.SendAudioRequest.Audio
// — ESTREITO como isDataURIImage (send_image.go): os 11 primeiros bytes
// têm de ser exatamente "data:audio/", não "qualquer data URI" nem
// "data:image" (imagem em disfarce de áudio não passa).
func isDataURIAudio(raw string) bool {
	return len(raw) >= len(dataAudioPrefix) && raw[:len(dataAudioPrefix)] == dataAudioPrefix
}

// decodeDataURIAudio decodifica uma data URI localmente (sem tocar rede) e
// devolve os bytes decodificados junto com o rótulo de MIME que a PRÓPRIA
// data URI declara (dataurl.DecodeString(...).ContentType()) — ao
// contrário de SendImage/SendDocument, o histórico de Audio usa esse
// rótulo como nível 2 da precedência de MIME (ver resolveAudioMimeType).
// O teto aplicado é SEMPRE fetchAudioMaxBytes (16MB) em produção — ver
// decodeAudioDataURIWithLimit para a lógica de fronteira pura, exercitada
// em teste com um limite sintético pequeno sem alocar payloads de 16MB.
func (uc *SendAudioUseCase) decodeDataURIAudio(ctx context.Context, txtID, raw string) ([]byte, string, error) {
	data, mime, err := decodeAudioDataURIWithLimit(raw, fetchAudioMaxBytes)
	if err != nil {
		if appErr, ok := err.(*apperr.AppError); ok && appErr.Code == "audio_too_large" {
			uc.logger.Warn(ctx, "data uri audio payload exceeds size limit", "txtID", txtID, "limit", fetchAudioMaxBytes)
		} else {
			uc.logger.Warn(ctx, "failed to decode data uri audio", "txtID", txtID, "error", err)
		}
		return nil, "", err
	}
	return data, mime, nil
}

// decodeAudioDataURIWithLimit é a lógica de fronteira PURA (sem logging,
// sem dependência do use case) por trás de decodeDataURIAudio — extraída
// para que o teste da fronteira 16MB/16MB+1 possa injetar um limit
// sintético pequeno em vez de alocar payloads de 16MB só para provar uma
// comparação (ver CLAUDE.md, "Teste de boundary sem desperdiçar RAM").
// TestSendAudio_DataURIMaxBytesConstant prova, separadamente e sem
// alocação, que fetchAudioMaxBytes vale exatamente 16MB. Mesma estratégia
// de duas medidas (estimativa barata pré-decode + contagem real
// pós-decode) de decodeDataURIWithLimit (send_document.go) e
// decodeDataURIImage (send_image.go).
func decodeAudioDataURIWithLimit(raw string, limit int64) ([]byte, string, error) {
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		meta, encoded := raw[:idx], raw[idx+1:]
		if strings.Contains(meta, ";base64") {
			if n, ok := decodedBase64Len(encoded); ok && n > limit {
				return nil, "", apperr.New("audio_too_large", apperr.CategoryValidation,
					"decoded audio payload exceeds the maximum allowed size", false, nil)
			}
		}
	}

	decoded, err := dataurl.DecodeString(raw)
	if err != nil {
		return nil, "", apperr.New("invalid_data_uri", apperr.CategoryValidation,
			"could not decode base64 encoded data from payload", false, err)
	}
	if int64(len(decoded.Data)) > limit {
		return nil, "", apperr.New("audio_too_large", apperr.CategoryValidation,
			"decoded audio payload exceeds the maximum allowed size", false, nil)
	}
	return decoded.Data, decoded.ContentType(), nil
}

// isHTTPAudioURL reconhece o ramo URL de domain.SendAudioRequest.Audio —
// mesma regra do isHTTPURL histórico (`git show 41bc8e2^:helpers.go`,
// linha 160) e de isHTTPImageURL/isHTTPDocumentURL: scheme http ou https e
// host não vazio.
func isHTTPAudioURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}
