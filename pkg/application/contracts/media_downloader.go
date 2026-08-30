package port

import (
	"context"

	"wa-api/pkg/domain"
)

// MediaDownloader baixa e decifra o anexo descrito por um
// domain.MediaDescriptor, devolvendo os BYTES CRUS.
//
// Porta ÚNICA para os cinco tipos de mídia (CAP-09B), e não cinco portas:
// a primitive do SDK é uma só — `Client.Download(ctx, DownloadableMessage)`,
// internal/noise/core/download.go:62 — e o que varia entre imagem, vídeo,
// áudio, documento e figurinha é apenas QUAL sub-mensagem protobuf embrulha os
// mesmos sete campos de cifragem/localização. Esse "qual" é
// domain.MediaDescriptor.Kind, e a montagem correspondente
// (ImageMessage/VideoMessage/AudioMessage/DocumentMessage/StickerMessage) vive
// no adapter, junto do SDK.
//
// A porta devolve []byte de propósito: NÃO base64, NÃO data URL, NÃO um DTO de
// resposta. A codificação em `data:<mime>;base64,<payload>` é preocupação de
// wa-api (use case), e o SDK vendorizado não recebe nenhuma noção de HTTP —
// mesma separação que pkg/infra/media/media.go:73 já pratica ao usar
// `Download` para gravar bytes num arquivo temporário.
//
// SessionGuard embutido porque download é operação PROTOCOLAR: sem cliente
// noise não há media connection para resolver os hosts de download
// (internal/noise/capabilities/media/download.go, DownloadWithPath ->
// RefreshConn), então a checagem de sessão não é cerimônia — é pré-condição.
type MediaDownloader interface {
	SessionGuard

	// Download devolve os bytes decifrados do anexo. Um erro do SDK é
	// propagado como está — nunca convertido em sucesso vazio; o contrato
	// público de /chat/download* promete conteúdo.
	Download(ctx context.Context, txtID string, descriptor domain.MediaDescriptor) ([]byte, error)
}
