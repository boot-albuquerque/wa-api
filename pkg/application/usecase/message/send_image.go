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

// fetchImageMaxBytes é o teto de bytes aceito ao buscar uma imagem por URL
// externa — paridade com o limite histórico do wuzapi (constante
// fetchImageMaxBytes, ver `git grep -n "fetchImageMaxBytes\s*=" 41bc8e2^ --
// '*.go'`). appport.MediaFetcher aplica este limite DURANTE a leitura
// (io.LimitReader em opengraph.FetchURLBytes), não só contra um
// Content-Length declarado pelo servidor remoto.
//
// CAP-03 (divergência D1, deliberada): historicamente o ramo data URI não
// tinha teto nenhum (fetchImageMaxBytes só valia para o ramo URL, ver
// `git show 41bc8e2^:handlers.go`). Este projeto passa a aplicar o MESMO
// teto aos dois ramos — sem isso, o limite do ramo URL vira um bypass
// trivial (basta o cliente mandar a imagem como data URI em vez de URL).
const fetchImageMaxBytes int64 = 16 * 1024 * 1024

// dataImagePrefix é o discriminador histórico do ramo data URI de
// SendImageRequest.Image (`git show 41bc8e2^:handlers.go`, em torno da
// linha 1280): exatamente os 10 primeiros bytes "data:image", não "qualquer
// data URI" nem "data:" genérico. Um MIME não-imagem em data URI (ex.:
// "data:application/pdf;base64,...") falha este prefixo e cai para
// isHTTPImageURL — que também falha — terminando em unsupported_image_source.
// Isso responde, sem código extra, ao requisito de rejeitar MIME não-imagem
// no ramo data URI: a própria discriminação já é o filtro.
const dataImagePrefix = "data:image"

// SendImageUseCase envia uma imagem de verdade: obtém os bytes (por URL
// externa via infra SSRF-safe, ou por decode local de data URI), sobe o
// anexo e envia a mensagem pelo wa-noise. Só devolve domain.StatusSent
// depois que o envio retorna sucesso — nunca antes (mesma disciplina de
// SendMessageUseCase, CAP-01).
//
// domain.SendImageRequest.Image é uma união de dois transportes de OBTENÇÃO
// dos bytes — URL http(s) (CAP-02, via appport.MediaFetcher) e data URI
// (CAP-03, decode local, sem tocar a rede) — que convergem no mesmo
// protocolo de envio (appport.MediaMessenger). Ver isDataURIImage e
// isHTTPImageURL para a discriminação.
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

// Execute valida os campos obrigatórios, resolve o destinatário, obtém a
// imagem (fetch por URL ou decode local de data URI, conforme o transporte
// discriminado), sobe o anexo e envia a mensagem pela porta de verdade.
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

	var data []byte
	var mimeType string

	switch {
	case isDataURIImage(req.Image):
		// Decode LOCAL — nunca passa por MediaFetcher. MediaFetcher
		// representa fetch EXTERNO (SSRF-safe); data URI já é o payload
		// inteiro na requisição, sem envolver rede nenhuma, e manter as
		// duas coisas separadas é o que preserva as preocupações de SSRF
		// restritas ao ramo URL (arquitetura exigida do slice).
		var err error
		data, err = uc.decodeDataURIImage(ctx, txtID, req.Image)
		if err != nil {
			return nil, err
		}
		mimeType = resolveMimeType(req.MimeType, data)

	case isHTTPImageURL(req.Image):
		var err error
		// contentType (segundo valor de retorno) é intencionalmente
		// descartado: o Content-Type que o servidor remoto declara não é
		// confiável (ele pode mentir), então o tipo final vem de
		// req.MimeType ou, na ausência dele, de sniffing dos bytes reais
		// — nunca do cabeçalho HTTP. Mesmo comportamento do handlers.go
		// histórico, que montava ImageMessage.Mimetype a partir de
		// t.MimeType ou http.DetectContentType(filedata), nunca do
		// Content-Type da resposta.
		data, _, err = uc.fetcher.FetchBytes(ctx, req.Image, fetchImageMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "failed to fetch image url", "txtID", txtID, "error", err)
			return nil, apperr.New("image_fetch_failed", apperr.CategoryValidation, "failed to fetch image from url", false, err)
		}
		mimeType = resolveMimeType(req.MimeType, data)

	default:
		return nil, apperr.New("unsupported_image_source", apperr.CategoryValidation,
			`Image data should start with "data:image/png;base64," or be an http(s) URL`, false, nil)
	}

	if len(data) == 0 {
		return nil, apperr.New("empty_image_body", apperr.CategoryValidation, "image body is empty", false, nil)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, apperr.New("invalid_image_mime_type", apperr.CategoryValidation, "resolved MIME type is not an image type", false, nil)
	}

	payload := domain.MediaPayload{Bytes: data, MimeType: mimeType, Caption: req.Caption, JPEGThumbnail: req.JPEGThumbnail}

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

// resolveMimeType é o ÚNICO ponto de decisão de MIME final para os dois
// transportes de SendImageRequest.Image, aplicando a mesma precedência
// canônica do handlers.go histórico (`git show 41bc8e2^:handlers.go`, campo
// Mimetype de ImageMessage, montado por uma função anônima válida para os
// dois ramos DEPOIS do if/else de discriminação):
//  1. reqMimeType (SendImageRequest.MimeType), quando não vazio, tem
//     precedência — nem o Content-Type declarado pelo servidor remoto (ramo
//     URL) nem o rótulo declarado pela própria data URI (ramo data URI) são
//     fonte confiável de MIME: ambos podem mentir sobre o que os bytes
//     realmente são.
//  2. Na ausência de reqMimeType, o tipo vem de sniffing dos bytes JÁ
//     decodificados/buscados (http.DetectContentType) — nunca do rótulo ou
//     cabeçalho declarado.
func resolveMimeType(reqMimeType string, data []byte) string {
	if reqMimeType != "" {
		return reqMimeType
	}
	return http.DetectContentType(data)
}

// isDataURIImage reconhece o ramo data URI de domain.SendImageRequest.Image
// pela mesma regra literal do handlers.go histórico: os 10 primeiros bytes
// têm de ser exatamente "data:image" — não "qualquer data URI" (um
// "data:application/pdf;base64,..." não bate aqui, cai para
// isHTTPImageURL, falha, e termina em unsupported_image_source).
func isDataURIImage(raw string) bool {
	return len(raw) >= len(dataImagePrefix) && raw[:len(dataImagePrefix)] == dataImagePrefix
}

// decodeDataURIImage decodifica uma data URI localmente (sem tocar rede) e
// devolve só os bytes decodificados. O rótulo de MIME que a própria data URI
// declara NÃO é a fonte do MIME final — não é mais confiável que o
// Content-Type de um servidor remoto no ramo URL, e aceitá-lo cegamente abre
// superfície de MIME-spoofing (rótulo "image/png" sobre bytes de PDF, por
// exemplo). A regra histórica (`git show 41bc8e2^:handlers.go`, a função
// anônima que monta ImageMessage.Mimetype logo APÓS o if/else de
// discriminação de transporte — portanto valendo para os dois ramos) é:
// req.MimeType tem precedência quando não vazio, senão sniffing dos bytes
// DECODIFICADOS via http.DetectContentType. Ver resolveMimeType, chamado
// pelo Execute com os bytes que esta função devolve.
//
// CAP-03 (divergência D1): aplica fetchImageMaxBytes aqui também, o que o
// histórico não fazia — ver o comentário na constante fetchImageMaxBytes.
// O tamanho decodificado é medido de duas formas:
//  1. Uma estimativa BARATA e exata a partir do comprimento do texto
//     base64 (sem decodificar), para rejeitar cedo payloads grosseiramente
//     acima do limite sem alocar o buffer decodificado inteiro — base64
//     infla ~4/3, então usar len(encodedString) como proxy do tamanho
//     decodificado sem essa conta subestimaria o payload real em ~33%.
//  2. A contagem real de bytes decodificados, que é a autoridade — cobre
//     os casos em que a estimativa não se aplica (payload não é múltiplo
//     de 4 caracteres, por exemplo) e é o que define o limite exato na
//     fronteira 16MB / 16MB+1.
func (uc *SendImageUseCase) decodeDataURIImage(ctx context.Context, txtID, raw string) ([]byte, error) {
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		meta, encoded := raw[:idx], raw[idx+1:]
		if strings.Contains(meta, ";base64") {
			if n, ok := decodedBase64Len(encoded); ok && n > fetchImageMaxBytes {
				uc.logger.Warn(ctx, "data uri image payload exceeds size limit before decode", "txtID", txtID, "decodedBytes", n, "limit", fetchImageMaxBytes)
				return nil, apperr.New("image_too_large", apperr.CategoryValidation,
					"decoded image payload exceeds the maximum allowed size", false, nil)
			}
		}
	}

	decoded, err := dataurl.DecodeString(raw)
	if err != nil {
		uc.logger.Warn(ctx, "failed to decode data uri image", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_data_uri", apperr.CategoryValidation,
			"could not decode base64 encoded data from payload", false, err)
	}
	if int64(len(decoded.Data)) > fetchImageMaxBytes {
		uc.logger.Warn(ctx, "data uri image payload exceeds size limit after decode", "txtID", txtID, "decodedBytes", len(decoded.Data), "limit", fetchImageMaxBytes)
		return nil, apperr.New("image_too_large", apperr.CategoryValidation,
			"decoded image payload exceeds the maximum allowed size", false, nil)
	}
	return decoded.Data, nil
}

// decodedBase64Len calcula o tamanho decodificado EXATO de uma string
// base64 padrão (RFC 4648, a variante usada em data URI) sem decodificá-la
// — só olha o comprimento e o padding ('=') final. Só é exata para entrada
// bem formada (comprimento múltiplo de 4); para qualquer outra coisa
// devolve ok=false e deixa dataurl.DecodeString ser a autoridade (ele vai
// rejeitar como base64 inválida de qualquer forma).
func decodedBase64Len(encoded string) (int64, bool) {
	if len(encoded)%4 != 0 {
		return 0, false
	}
	return int64(len(encoded))/4*3 - int64(len(encoded)-len(strings.TrimRight(encoded, "="))), true
}

// isHTTPImageURL reconhece o ramo URL de domain.SendImageRequest.Image —
// mesma regra do isHTTPURL histórico (`git show 41bc8e2^:helpers.go`, linha
// 160): scheme http ou https e host não vazio. Qualquer outro scheme
// (file://, ftp://, gopher://, base64 cru sem o prefixo "data:image") cai
// fora dos dois ramos suportados e vira unsupported_image_source em vez de
// ser aceito silenciosamente.
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
