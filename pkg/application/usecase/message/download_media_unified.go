package message

import (
	"context"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// CodeUnknownMediaKind é a recusa de cliente quando Kind não é um dos cinco
// valores conhecidos. Constante nomeada (ADR-0004): usada aqui e no teste que
// trava o controlo negativo.
const CodeUnknownMediaKind = "unknown_media_kind"

// mediaKindExecutor é o que as cinco DownloadXUseCase já satisfazem — não uma
// interface nova para elas implementarem, uma que descreve a forma que
// Execute já tem.
type mediaKindExecutor interface {
	Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error)
}

// DownloadMediaUseCase é a rota consolidada (CAP-10, POST /chats/download):
// um Kind explícito no payload escolhe qual das cinco capabilities de
// download executar, no lugar de cinco rotas com o kind implícito no nome.
//
// Não duplica a lógica de mediaDownloadFlow — delega para o use case já
// existente do kind pedido, então o comportamento (ordem de validação,
// bytes vazios, MIME preservado) é idêntico ao das rotas /chats/download*.
type DownloadMediaUseCase struct {
	byKind map[domain.MediaKind]mediaKindExecutor
}

// NewDownloadMediaUseCase monta o despacho a partir das cinco capabilities já
// existentes, para não reconstruir o mapeamento kind -> protobuf noutro
// lugar.
func NewDownloadMediaUseCase(
	image *DownloadImageUseCase,
	video *DownloadVideoUseCase,
	audio *DownloadAudioUseCase,
	document *DownloadDocumentUseCase,
	sticker *DownloadStickerUseCase,
) *DownloadMediaUseCase {
	return &DownloadMediaUseCase{byKind: map[domain.MediaKind]mediaKindExecutor{
		domain.MediaKindImage:    image,
		domain.MediaKindVideo:    video,
		domain.MediaKindAudio:    audio,
		domain.MediaKindDocument: document,
		domain.MediaKindSticker:  sticker,
	}}
}

// Execute valida o Kind ANTES de tudo — inclusive antes da validação de
// Url/DirectPath que o use case do kind faria — porque um Kind desconhecido
// não tem para quem delegar, e é erro de cliente como os outros 400 desta
// família.
func (uc *DownloadMediaUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	executor, ok := uc.byKind[req.Kind]
	if !ok {
		return nil, apperr.New(
			CodeUnknownMediaKind,
			apperr.CategoryValidation,
			"Kind must be one of image, video, audio, document, sticker",
			false,
			nil,
		)
	}
	return executor.Execute(ctx, txtID, req)
}
