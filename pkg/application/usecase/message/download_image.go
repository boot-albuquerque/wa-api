package message

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DownloadImageUseCase baixa a imagem de verdade: monta o descritor com os sete
// campos do payload, chama a porta appport.MediaDownloader (que embrulha
// `Client.Download` com a sub-mensagem ImageMessage) e devolve os bytes
// já codificados em Data URL. Antes de CAP-09B este use case validava a
// requisição e devolvia um domain.DownloadResult VAZIO sem nunca baixar nada.
type DownloadImageUseCase struct {
	flow mediaDownloadFlow
}

// NewDownloadImageUseCase cria uma nova instância do usecase.
func NewDownloadImageUseCase(md appport.MediaDownloader, l appport.Logger) *DownloadImageUseCase {
	return &DownloadImageUseCase{
		flow: mediaDownloadFlow{downloader: md, logger: l, kind: domain.MediaKindImage},
	}
}

// Execute valida os campos obrigatórios, garante a sessão e baixa a imagem.
func (uc *DownloadImageUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	return uc.flow.execute(ctx, txtID, req)
}
