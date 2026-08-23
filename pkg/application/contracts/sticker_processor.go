package port

import "context"

// StickerProcessor converte um payload de sticker (data URI) para o WebP
// pronto para o WhatsApp, embutindo metadata de pacote via EXIF quando
// fornecida.
//
// Porta estreita sobre pkg/infra/media/sticker (CAP-07): o pipeline de
// conversão (encode WebP via ffmpeg + injeção de chunk EXIF) é infra — essa
// interface é o seam que mantém SendStickerUseCase sem importar pkg/infra
// diretamente, mesma disciplina de MediaFetcher/MediaMessenger.
type StickerProcessor interface {
	// ProcessSticker decodifica dataURI (tem de começar com o prefixo "data",
	// não necessariamente "data:") e converte para um WebP de sticker do
	// WhatsApp. mimeOverride, quando não vazio, tem precedência sobre
	// sniffing para decidir o caminho de conversão (imagem vs vídeo/gif).
	// packID/packName/packPublisher/emojis alimentam a metadata EXIF do
	// pacote — SendStickerUseCase hoje sempre passa vazio/nil (achado CAP-07:
	// domain.SendStickerRequest não tem campos para eles). Devolve os bytes
	// processados e o MIME final — o chamador TEM de usar os dois (não os
	// bytes/MIME originais) para o upload.
	ProcessSticker(ctx context.Context, dataURI, mimeOverride, packID, packName, packPublisher string, emojis []string) ([]byte, string, error)
}
