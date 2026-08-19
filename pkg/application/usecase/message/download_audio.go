package message

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DownloadAudioUseCase baixa o áudio de verdade: monta o descritor com os sete
// campos do payload, chama a porta appport.MediaDownloader (que embrulha
// `Client.Download` com a sub-mensagem AudioMessage) e devolve os bytes
// já codificados em Data URL. Antes de CAP-09B este use case validava a
// requisição e devolvia um domain.DownloadResult VAZIO sem nunca baixar nada.
type DownloadAudioUseCase struct {
	flow mediaDownloadFlow
}

// NewDownloadAudioUseCase cria uma nova instância do usecase.
func NewDownloadAudioUseCase(md appport.MediaDownloader, l appport.Logger) *DownloadAudioUseCase {
	return &DownloadAudioUseCase{
		flow: mediaDownloadFlow{downloader: md, logger: l, kind: domain.MediaKindAudio},
	}
}

// Execute valida os campos obrigatórios, garante a sessão e baixa o áudio.
func (uc *DownloadAudioUseCase) Execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	return uc.flow.execute(ctx, txtID, req)
}
