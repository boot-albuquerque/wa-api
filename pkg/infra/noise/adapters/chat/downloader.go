package chat

import (
	"context"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise"
	"wa-api/internal/noise/protocol/proto/waE2E"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/noise/client"
	wasession "wa-api/pkg/infra/noise/runtime/session"
)

// MediaDownloaderAdapter implementa appport.MediaDownloader.
//
// É a ÚNICA tradução domain.MediaDescriptor -> sub-mensagem protobuf do SDK
// (CAP-09B). Vive aqui, junto do SDK, e não na application layer, porque
// waE2E.ImageMessage e companhia são detalhe do protocolo — a porta atravessa
// domain.MediaDescriptor, que não conhece protobuf.
type MediaDownloaderAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewMediaDownloaderAdapter cria o adapter com a função de lookup.
func NewMediaDownloaderAdapter(getClient client.Getter) *MediaDownloaderAdapter {
	return &MediaDownloaderAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// Download monta a sub-mensagem correspondente ao kind e chama
// Client.Download, devolvendo os bytes decifrados.
func (a *MediaDownloaderAdapter) Download(ctx context.Context, txtID string, descriptor domain.MediaDescriptor) ([]byte, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}

	msg, err := downloadableFor(descriptor)
	if err != nil {
		return nil, err
	}

	return client.Download(ctx, msg)
}

// downloadableFor monta a sub-mensagem protobuf do kind pedido, com os SETE
// campos do descritor.
//
// A montagem é campo a campo igual à do fluxo histórico
// (`git show 41bc8e2^:handlers.go`: DownloadImage na linha 3836,
// DownloadDocument, DownloadVideo e DownloadAudio logo abaixo, DownloadSticker
// na linha 7264): URL/DirectPath/Mimetype como proto.String, MediaKey/
// FileEncSHA256/FileSHA256 como []byte cru, FileLength como ponteiro para
// uint64. A cinco-a-cinco os handlers históricos eram literalmente o mesmo
// corpo com o tipo trocado — aqui isso vira um switch, não cinco cópias.
//
// FileLength recebe uma cópia local: `&descriptor.FileLength` apontaria para o
// parâmetro, o que é seguro em Go mas amarra o protobuf ao tempo de vida do
// descritor; a cópia deixa a montagem autocontida.
func downloadableFor(descriptor domain.MediaDescriptor) (noise.DownloadableMessage, error) {
	fileLength := descriptor.FileLength

	switch descriptor.Kind {
	case domain.MediaKindImage:
		return &waE2E.ImageMessage{
			URL:           proto.String(descriptor.URL),
			DirectPath:    proto.String(descriptor.DirectPath),
			MediaKey:      descriptor.MediaKey,
			Mimetype:      proto.String(descriptor.Mimetype),
			FileEncSHA256: descriptor.FileEncSHA256,
			FileSHA256:    descriptor.FileSHA256,
			FileLength:    &fileLength,
		}, nil
	case domain.MediaKindVideo:
		return &waE2E.VideoMessage{
			URL:           proto.String(descriptor.URL),
			DirectPath:    proto.String(descriptor.DirectPath),
			MediaKey:      descriptor.MediaKey,
			Mimetype:      proto.String(descriptor.Mimetype),
			FileEncSHA256: descriptor.FileEncSHA256,
			FileSHA256:    descriptor.FileSHA256,
			FileLength:    &fileLength,
		}, nil
	case domain.MediaKindAudio:
		return &waE2E.AudioMessage{
			URL:           proto.String(descriptor.URL),
			DirectPath:    proto.String(descriptor.DirectPath),
			MediaKey:      descriptor.MediaKey,
			Mimetype:      proto.String(descriptor.Mimetype),
			FileEncSHA256: descriptor.FileEncSHA256,
			FileSHA256:    descriptor.FileSHA256,
			FileLength:    &fileLength,
		}, nil
	case domain.MediaKindDocument:
		return &waE2E.DocumentMessage{
			URL:           proto.String(descriptor.URL),
			DirectPath:    proto.String(descriptor.DirectPath),
			MediaKey:      descriptor.MediaKey,
			Mimetype:      proto.String(descriptor.Mimetype),
			FileEncSHA256: descriptor.FileEncSHA256,
			FileSHA256:    descriptor.FileSHA256,
			FileLength:    &fileLength,
		}, nil
	case domain.MediaKindSticker:
		return &waE2E.StickerMessage{
			URL:           proto.String(descriptor.URL),
			DirectPath:    proto.String(descriptor.DirectPath),
			MediaKey:      descriptor.MediaKey,
			Mimetype:      proto.String(descriptor.Mimetype),
			FileEncSHA256: descriptor.FileEncSHA256,
			FileSHA256:    descriptor.FileSHA256,
			FileLength:    &fileLength,
		}, nil
	default:
		// Kind desconhecido é defeito de PROGRAMAÇÃO deste repositório (o
		// kind nunca vem do cliente HTTP — é fixado no construtor de cada
		// use case), então o erro não pode passar por sucesso vazio nem
		// virar um download com sub-mensagem nil, que o SDK rejeitaria com
		// ErrUnknownMediaType numa mensagem sem pista de onde veio.
		return nil, apperr.New("unknown_media_kind", apperr.CategoryInternal, "unknown media kind", false, nil)
	}
}

// Verificação em tempo de compilação de que o adapter implementa a porta.
var _ appport.MediaDownloader = (*MediaDownloaderAdapter)(nil)
