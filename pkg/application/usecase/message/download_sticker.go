package message

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DownloadStickerUseCase baixa a figurinha de verdade: monta o descritor com os sete
// campos do payload, chama a porta appport.MediaDownloader (que embrulha
// `Client.Download` com a sub-mensagem StickerMessage) e devolve os bytes
// já codificados em Data URL. Antes de CAP-09B este use case validava a
// requisição e devolvia um domain.DownloadResult VAZIO sem nunca baixar nada.
type DownloadStickerUseCase struct {
	flow mediaDownloadFlow
}

// NewDownloadStickerUseCase cria uma nova instância do usecase.
func NewDownloadStickerUseCase(md appport.MediaDownloader, l appport.Logger) *DownloadStickerUseCase {
	return &DownloadStickerUseCase{
		flow: mediaDownloadFlow{downloader: md, logger: l, kind: domain.MediaKindSticker},
	}
}

// Execute valida os campos obrigatórios, garante a sessão e baixa a figurinha.
func (uc *DownloadStickerUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	return uc.flow.execute(ctx, txtID, req)
}
