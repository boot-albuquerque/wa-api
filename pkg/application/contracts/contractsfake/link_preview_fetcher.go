package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// LinkPreviewFetcherCall é uma chamada a FetchLinkPreview.
type LinkPreviewFetcherCall struct {
	Ctx  context.Context
	Text string
}

// LinkPreviewFetcher é o fake de port.LinkPreviewFetcher. Sem
// FetchLinkPreviewFunc configurada, devolve found=false — nenhuma URL
// encontrada — que é o comportamento neutro para os testes de use case que
// não exercitam LinkPreview.
type LinkPreviewFetcher struct {
	FetchLinkPreviewFunc  func(ctx context.Context, text string) (domain.LinkPreviewData, bool)
	FetchLinkPreviewCalls []LinkPreviewFetcherCall
}

var _ port.LinkPreviewFetcher = (*LinkPreviewFetcher)(nil)

// FetchLinkPreview implementa port.LinkPreviewFetcher.
func (f *LinkPreviewFetcher) FetchLinkPreview(ctx context.Context, text string) (domain.LinkPreviewData, bool) {
	f.FetchLinkPreviewCalls = append(f.FetchLinkPreviewCalls, LinkPreviewFetcherCall{Ctx: ctx, Text: text})
	if f.FetchLinkPreviewFunc != nil {
		return f.FetchLinkPreviewFunc(ctx, text)
	}
	return domain.LinkPreviewData{}, false
}
