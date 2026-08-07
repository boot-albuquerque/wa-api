package media

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMediaTransport"
	"wa-api/internal/wa-noise/protocol/types"
)

// DownloadAny percorre as partes baixaveis da mensagem e baixa a primeira nao
// nula.
func DownloadAny(ctx context.Context, t Transport, msg *waE2E.Message) ([]byte, error) {
	if msg == nil {
		return nil, ErrNothingDownloadableFound
	}
	switch {
	case msg.ImageMessage != nil:
		return DownloadMessage(ctx, t, msg.ImageMessage)
	case msg.VideoMessage != nil:
		return DownloadMessage(ctx, t, msg.VideoMessage)
	case msg.AudioMessage != nil:
		return DownloadMessage(ctx, t, msg.AudioMessage)
	case msg.DocumentMessage != nil:
		return DownloadMessage(ctx, t, msg.DocumentMessage)
	case msg.StickerMessage != nil:
		return DownloadMessage(ctx, t, msg.StickerMessage)
	default:
		return nil, ErrNothingDownloadableFound
	}
}

// DownloadThumbnail baixa o thumbnail de uma mensagem — principalmente o
// preview de link em ExtendedTextMessage.
func DownloadThumbnail(ctx context.Context, t Transport, msg DownloadableThumbnail) ([]byte, error) {
	mediaType, ok := classToThumbnailMediaType[msg.ProtoReflect().Descriptor().Name()]
	if !ok {
		return nil, fmt.Errorf("%w '%s'", ErrUnknownMediaType, string(msg.ProtoReflect().Descriptor().Name()))
	} else if len(msg.GetThumbnailDirectPath()) > 0 {
		return DownloadWithPath(
			ctx, t, msg.GetThumbnailDirectPath(), msg.GetThumbnailEncSHA256(), msg.GetThumbnailSHA256(),
			msg.GetMediaKey(), UnknownFileLength, mediaType, mediaTypeToMMSType[mediaType],
		)
	} else {
		return nil, ErrNoURLPresent
	}
}

// FetchStickerPack busca os metadados de um pacote de figurinhas no endpoint
// estatico (que nao passa pela media connection).
func FetchStickerPack(ctx context.Context, t HTTPTransport, packID string) (*types.StickerPack, error) {
	url := fmt.Sprintf(stickerPackMetadataURLFormat, packID)
	resp, err := DoDownloadRequest(ctx, t, url)
	if err != nil {
		return nil, err
	}
	var packs []types.StickerPack
	err = json.NewDecoder(resp.Body).Decode(&packs)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	} else if len(packs) == 0 {
		return nil, fmt.Errorf("no sticker pack found in response")
	}
	return &packs[0], nil
}

// DownloadMessage baixa o anexo da mensagem protobuf dada.
func DownloadMessage(ctx context.Context, t Transport, msg Downloadable) ([]byte, error) {
	mediaType := GetType(msg)
	if mediaType == "" {
		return nil, fmt.Errorf("%w %T", ErrUnknownMediaType, msg)
	}
	url, isWebWhatsappNetURL := directURL(msg)
	if len(url) > 0 && !isWebWhatsappNetURL {
		return DownloadAndDecrypt(ctx, t, url, msg.GetMediaKey(), mediaType, getSize(msg), msg.GetFileEncSHA256(), msg.GetFileSHA256())
	} else if len(msg.GetDirectPath()) > 0 {
		return DownloadWithPath(
			ctx, t, msg.GetDirectPath(), msg.GetFileEncSHA256(), msg.GetFileSHA256(),
			msg.GetMediaKey(), getSize(msg), mediaType, mediaTypeToMMSType[mediaType],
		)
	} else {
		if isWebWhatsappNetURL {
			t.Log().Warnf("Got a media message with a web.whatsapp.net URL (%s) and no direct path", url)
		}
		return nil, ErrNoURLPresent
	}
}

// directURL devolve a URL declarada na mensagem (se houver) e se ela e' uma
// URL web.whatsapp.net, que nao e' baixavel diretamente.
func directURL(msg Downloadable) (url string, isWebWhatsappNetURL bool) {
	urlable, ok := msg.(downloadableWithURL)
	if !ok {
		return "", false
	}
	url = urlable.GetURL()
	return url, strings.HasPrefix(url, webWhatsappNetURLPrefix)
}

// DownloadFB baixa um anexo descrito por um WAMediaTransport do Messenger.
func DownloadFB(
	ctx context.Context,
	t Transport,
	transport *waMediaTransport.WAMediaTransport_Integral,
	mediaType Type,
) ([]byte, error) {
	return DownloadWithPath(
		ctx, t, transport.GetDirectPath(), transport.GetFileEncSHA256(), transport.GetFileSHA256(),
		transport.GetMediaKey(), UnknownFileLength, mediaType, mediaTypeToMMSType[mediaType],
	)
}

// DownloadWithPath baixa um anexo informando manualmente o caminho e os
// detalhes de cifragem, percorrendo os hosts da media connection ate' o
// primeiro sucesso (ou ate' um erro que nao adianta repetir em outro host).
func DownloadWithPath(
	ctx context.Context,
	t Transport,
	directPath string,
	encFileHash, fileHash, mediaKey []byte,
	fileLength int,
	mediaType Type,
	mmsType string,
) (data []byte, err error) {
	if !strings.HasPrefix(directPath, "/") {
		return nil, fmt.Errorf("media download path does not start with slash: %s", directPath)
	}
	var mediaConn *Conn
	mediaConn, err = RefreshConn(ctx, t, false)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh media connections: %w", err)
	}
	if len(mmsType) == 0 {
		mmsType = mediaTypeToMMSType[mediaType]
	}
	for i, host := range mediaConn.Hosts {
		// TODO omit hash for unencrypted media?
		mediaURL := buildDownloadURL(host.Hostname, directPath, encFileHash, mmsType)
		data, err = DownloadAndDecrypt(ctx, t, mediaURL, mediaKey, mediaType, fileLength, encFileHash, fileHash)
		if isTerminalDownloadResult(err) {
			return
		} else if i >= len(mediaConn.Hosts)-1 {
			return nil, fmt.Errorf("failed to download media from last host: %w", err)
		}
		t.Log().Warnf("Failed to download media: %s, trying with next host...", err)
	}
	return
}

// buildDownloadURL monta a URL de download num host da media connection.
func buildDownloadURL(hostname, directPath string, encFileHash []byte, mmsType string) string {
	return fmt.Sprintf(
		mediaDownloadURLFormat, hostname, directPath,
		base64.URLEncoding.EncodeToString(encFileHash), mmsType,
	)
}

// isTerminalDownloadResult diz se o resultado de uma tentativa num host encerra
// o laco: ou deu certo, ou o erro nao muda ao tentar outro host.
func isTerminalDownloadResult(err error) bool {
	return err == nil ||
		errors.Is(err, ErrFileLengthMismatch) ||
		errors.Is(err, ErrInvalidMediaSHA256) ||
		errors.Is(err, ErrMediaDownloadFailedWith403) ||
		errors.Is(err, ErrMediaDownloadFailedWith404) ||
		errors.Is(err, ErrMediaDownloadFailedWith410) ||
		errors.Is(err, context.Canceled)
}
