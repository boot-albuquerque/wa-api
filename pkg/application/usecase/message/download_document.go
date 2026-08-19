package message

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DownloadDocumentUseCase baixa o documento de verdade: monta o descritor com os sete
// campos do payload, chama a porta appport.MediaDownloader (que embrulha
// `Client.Download` com a sub-mensagem DocumentMessage) e devolve os bytes
// já codificados em Data URL. Antes de CAP-09B este use case validava a
// requisição e devolvia um domain.DownloadResult VAZIO sem nunca baixar nada.
type DownloadDocumentUseCase struct {
	flow mediaDownloadFlow
}

// NewDownloadDocumentUseCase cria uma nova instância do usecase.
func NewDownloadDocumentUseCase(md appport.MediaDownloader, l appport.Logger) *DownloadDocumentUseCase {
	return &DownloadDocumentUseCase{
		flow: mediaDownloadFlow{downloader: md, logger: l, kind: domain.MediaKindDocument},
	}
}

// Execute valida os campos obrigatórios, garante a sessão e baixa o documento.
func (uc *DownloadDocumentUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	return uc.flow.execute(ctx, txtID, req)
}
