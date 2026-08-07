package whatsmeow

import (
	"context"
	"io"

	"wa-api/internal/wa-noise/capabilities/media"
)

// A implementacao vive em internal/wa-noise/media/upload.go
// (ADR-0004, Fase F/G lote 1).

// UploadResponse contains the data from the attachment upload, which can be put into a message to send the attachment.
type UploadResponse = media.UploadResponse

// Upload uploads the given attachment to WhatsApp servers.
//
// You should copy the fields in the response to the corresponding fields in a protobuf message.
//
// For example, to send an image:
//
//	resp, err := cli.Upload(context.Background(), yourImageBytes, whatsmeow.MediaImage)
//	// handle error
//
//	imageMsg := &waE2E.ImageMessage{
//		Caption:  proto.String("Hello, world!"),
//		Mimetype: proto.String("image/png"), // replace this with the actual mime type
//		// you can also optionally add other fields like ContextInfo and JpegThumbnail here
//
//		URL:           &resp.URL,
//		DirectPath:    &resp.DirectPath,
//		MediaKey:      resp.MediaKey,
//		FileEncSHA256: resp.FileEncSHA256,
//		FileSHA256:    resp.FileSHA256,
//		FileLength:    &resp.FileLength,
//	}
//	_, err = cli.SendMessage(context.Background(), targetJID, &waE2E.Message{
//		ImageMessage: imageMsg,
//	})
//	// handle error again
//
// The same applies to the other message types like DocumentMessage, just replace the struct type and Message field name.
func (cli *Client) Upload(ctx context.Context, plaintext []byte, appInfo MediaType) (resp UploadResponse, err error) {
	if cli == nil {
		return resp, ErrClientIsNil
	}
	return media.Upload(ctx, cli.mediaT(), plaintext, appInfo)
}

// UploadReader uploads the given attachment to WhatsApp servers.
//
// This is otherwise identical to [Upload], but it reads the plaintext from an [io.Reader] instead of a byte slice.
// A temporary file is required for the encryption process. If tempFile is nil, a temporary file will be created
// and deleted after the upload.
//
// To use only one file, pass the same file as both plaintext and tempFile. This will cause the file to be overwritten with encrypted data.
func (cli *Client) UploadReader(ctx context.Context, plaintext io.Reader, tempFile io.ReadWriteSeeker, appInfo MediaType) (resp UploadResponse, err error) {
	if cli == nil {
		return resp, ErrClientIsNil
	}
	return media.UploadReader(ctx, cli.mediaT(), plaintext, tempFile, appInfo)
}

func (cli *Client) rawUpload(ctx context.Context, dataToUpload io.Reader, uploadSize uint64, fileHash []byte, appInfo MediaType, newsletter bool, resp *UploadResponse) error {
	return media.RawUpload(ctx, cli.mediaT(), dataToUpload, uploadSize, fileHash, appInfo, newsletter, resp)
}

// DeleteMedia deletes the media at the given direct path from WhatsApp servers.
//
// This is only used for things like history syncs, which should be deleted after processing.
func (cli *Client) DeleteMedia(ctx context.Context, appInfo MediaType, directPath string, encFileHash []byte, encHandle string) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return media.Delete(ctx, cli.mediaT(), appInfo, directPath, encFileHash, encHandle)
}
