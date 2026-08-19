package message

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DownloadVideoUseCase baixa o vídeo de verdade: monta o descritor com os sete
// campos do payload, chama a porta appport.MediaDownloader (que embrulha
// `Client.Download` com a sub-mensagem VideoMessage) e devolve os bytes
// já codificados em Data URL. Antes de CAP-09B este use case validava a
// requisição e devolvia um domain.DownloadResult VAZIO sem nunca baixar nada.
type DownloadVideoUseCase struct {
	flow mediaDownloadFlow
}

// NewDownloadVideoUseCase cria uma nova instância do usecase.
func NewDownloadVideoUseCase(md appport.MediaDownloader, l appport.Logger) *DownloadVideoUseCase {
	return &DownloadVideoUseCase{
		flow: mediaDownloadFlow{downloader: md, logger: l, kind: domain.MediaKindVideo},
	}
}

// Execute valida os campos obrigatórios, garante a sessão e baixa o vídeo.
func (uc *DownloadVideoUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	return uc.flow.execute(ctx, txtID, req)
}
