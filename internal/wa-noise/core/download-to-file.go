package wanoise

import (
	"context"
	"io"

	"wa-api/internal/wa-noise/capabilities/media"
	"wa-api/internal/wa-noise/protocol/proto/waMediaTransport"
)

// A implementacao vive em internal/wa-noise/media/download_file.go
// (ADR-0004, Fase F/G lote 1).

// File is the destination of a download-to-file. *os.File satisfies it.
type File = media.File

// DownloadToFile downloads the attachment from the given protobuf message.
//
// This is otherwise identical to [Download], but writes the attachment to a file instead of returning it as a byte slice.
func (cli *Client) DownloadToFile(ctx context.Context, msg DownloadableMessage, file File) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return media.DownloadMessageToFile(ctx, cli.mediaT(), msg, file)
}

// DownloadFBToFile is [DownloadFB], writing to a file instead of returning bytes.
func (cli *Client) DownloadFBToFile(
	ctx context.Context,
	transport *waMediaTransport.WAMediaTransport_Integral,
	mediaType MediaType,
	file File,
) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return media.DownloadFBToFile(ctx, cli.mediaT(), transport, mediaType, file)
}

// DownloadMediaWithPathToFile is [DownloadMediaWithPath], writing to a file.
func (cli *Client) DownloadMediaWithPathToFile(
	ctx context.Context,
	directPath string,
	encFileHash, fileHash, mediaKey []byte,
	fileLength int,
	mediaType MediaType,
	mmsType string,
	file File,
) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return media.DownloadWithPathToFile(
		ctx, cli.mediaT(), directPath, encFileHash, fileHash, mediaKey, fileLength, mediaType, mmsType, file,
	)
}

func (cli *Client) downloadAndDecryptToFile(
	ctx context.Context,
	url string,
	mediaKey []byte,
	appInfo MediaType,
	fileLength int,
	fileEncSHA256, fileSHA256 []byte,
	file File,
) error {
	return media.DownloadAndDecryptToFile(ctx, cli.mediaT(), url, mediaKey, appInfo, fileLength, fileEncSHA256, fileSHA256, file)
}

func (cli *Client) downloadPossiblyEncryptedMediaWithRetriesToFile(ctx context.Context, url string, checksum []byte, file File) (mac []byte, err error) {
	return media.DownloadPossiblyEncryptedWithRetriesToFile(ctx, cli.mediaT(), url, checksum, file)
}

func (cli *Client) downloadMediaToFile(ctx context.Context, url string, file io.Writer) (int64, []byte, error) {
	return media.DownloadRawToFile(ctx, cli.mediaT(), url, file)
}

func (cli *Client) downloadEncryptedMediaToFile(ctx context.Context, url string, checksum []byte, file File) ([]byte, error) {
	return media.DownloadEncryptedToFile(ctx, cli.mediaT(), url, checksum, file)
}
