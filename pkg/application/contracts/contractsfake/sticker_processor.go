package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
)

// DefaultProcessedStickerBytes é o corpo que StickerProcessor devolve sem
// ProcessStickerFunc configurada — bytes não vazios e distintos de
// qualquer input plausível de teste, para que um teste que dependa do
// conteúdo processado sem configurar o fake falhe de forma óbvia em vez de
// silenciosa (mesmo racional de DefaultFetchedImageBytes).
var DefaultProcessedStickerBytes = []byte("contractsfake-default-processed-sticker-webp")

// DefaultProcessedStickerMimeType é o MIME que StickerProcessor devolve sem
// ProcessStickerFunc configurada.
const DefaultProcessedStickerMimeType = "image/webp"

// StickerProcessorCall é uma chamada a ProcessSticker.
type StickerProcessorCall struct {
	Ctx           context.Context
	DataURI       string
	MimeOverride  string
	PackID        string
	PackName      string
	PackPublisher string
	Emojis        []string
}

// StickerProcessor é o fake de port.StickerProcessor.
type StickerProcessor struct {
	ProcessStickerFunc  func(ctx context.Context, dataURI, mimeOverride, packID, packName, packPublisher string, emojis []string) ([]byte, string, error)
	ProcessStickerCalls []StickerProcessorCall
}

var _ port.StickerProcessor = (*StickerProcessor)(nil)

// ProcessSticker implementa port.StickerProcessor.
func (f *StickerProcessor) ProcessSticker(ctx context.Context, dataURI, mimeOverride, packID, packName, packPublisher string, emojis []string) ([]byte, string, error) {
	f.ProcessStickerCalls = append(f.ProcessStickerCalls, StickerProcessorCall{
		Ctx: ctx, DataURI: dataURI, MimeOverride: mimeOverride,
		PackID: packID, PackName: packName, PackPublisher: packPublisher, Emojis: emojis,
	})
	if f.ProcessStickerFunc != nil {
		return f.ProcessStickerFunc(ctx, dataURI, mimeOverride, packID, packName, packPublisher, emojis)
	}
	return DefaultProcessedStickerBytes, DefaultProcessedStickerMimeType, nil
}
