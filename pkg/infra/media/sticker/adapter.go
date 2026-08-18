package sticker

import (
	"context"

	appport "wa-api/pkg/application/contracts"

	"github.com/rs/zerolog/log"
)

// Processor implementa appport.StickerProcessor chamando ProcessStickerData
// diretamente — nenhuma lógica de conversão vive aqui, só o adapter que
// expõe o pacote (CAP-07) à camada de aplicação por uma porta estreita.
type Processor struct{}

// NewProcessor cria um Processor.
func NewProcessor() *Processor {
	return &Processor{}
}

var _ appport.StickerProcessor = (*Processor)(nil)

// ProcessSticker implementa appport.StickerProcessor. ctx não é usado —
// ProcessStickerData é CPU-bound mais um exec.Command sem suporte a
// cancelamento; aceitar ctx aqui é só para manter a assinatura consistente
// com as demais portas (MediaFetcher, MediaMessenger).
func (p *Processor) ProcessSticker(_ context.Context, dataURI, mimeOverride, packID, packName, packPublisher string, emojis []string) ([]byte, string, error) {
	log.Debug().Int("dataURILen", len(dataURI)).Str("mimeOverride", mimeOverride).Msg("processing sticker via appport.StickerProcessor")
	data, mimeType, err := ProcessStickerData(dataURI, mimeOverride, packID, packName, packPublisher, emojis)
	if err != nil {
		log.Warn().Err(err).Int("dataURILen", len(dataURI)).Msg("StickerProcessor: sticker processing failed")
		return nil, "", err
	}
	return data, mimeType, nil
}
