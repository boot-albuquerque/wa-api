package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DefaultDownloadedBytes é o conteúdo que MediaDownloader devolve sem
// DownloadFunc configurada. Não-vazio de propósito: o zero-value do fake tem
// de representar um download BEM-SUCEDIDO com conteúdo, porque bytes vazios
// são um caso de erro em MediaDownloadFlow (o contrato público promete
// conteúdo) e um fake que devolvesse nil por padrão faria todo teste de
// caminho feliz cair no ramo de erro.
var DefaultDownloadedBytes = []byte("fake-media-bytes")

// MediaDownloaderDownloadCall é uma chamada a Download.
type MediaDownloaderDownloadCall struct {
	Ctx        context.Context
	TxtID      string
	Descriptor domain.MediaDescriptor
}

// MediaDownloader é o fake de port.MediaDownloader.
type MediaDownloader struct {
	SessionGuard

	DownloadFunc  func(ctx context.Context, txtID string, descriptor domain.MediaDescriptor) ([]byte, error)
	DownloadCalls []MediaDownloaderDownloadCall
}

var _ port.MediaDownloader = (*MediaDownloader)(nil)

// Download implementa port.MediaDownloader.
func (f *MediaDownloader) Download(ctx context.Context, txtID string, descriptor domain.MediaDescriptor) ([]byte, error) {
	f.DownloadCalls = append(f.DownloadCalls, MediaDownloaderDownloadCall{Ctx: ctx, TxtID: txtID, Descriptor: descriptor})
	if f.DownloadFunc != nil {
		return f.DownloadFunc(ctx, txtID, descriptor)
	}
	return DefaultDownloadedBytes, nil
}
