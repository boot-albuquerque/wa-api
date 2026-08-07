package whatsmeow

import (
	"context"
	"io"

	"wa-api/internal/wa-noise/capabilities/media"
)

// A implementacao vive em internal/wa-noise/media/upload_newsletter.go
// (ADR-0004, Fase F/G lote 1).

// UploadNewsletter uploads the given attachment to WhatsApp servers without encrypting it first.
//
// Newsletter media works mostly the same way as normal media, with a few differences:
// * Since it's unencrypted, there's no MediaKey or FileEncSHA256 fields.
// * There's a "media handle" that needs to be passed in SendRequestExtra.
//
// Example:
//
//	resp, err := cli.UploadNewsletter(context.Background(), yourImageBytes, whatsmeow.MediaImage)
//	// handle error
//
//	imageMsg := &waE2E.ImageMessage{
//		// Caption, mime type and other such fields work like normal
//		Caption:  proto.String("Hello, world!"),
//		Mimetype: proto.String("image/png"),
//
//		// URL and direct path are also there like normal media
//		URL:        &resp.URL,
//		DirectPath: &resp.DirectPath,
//		FileSHA256: resp.FileSHA256,
//		FileLength: &resp.FileLength,
//		// Newsletter media isn't encrypted, so the media key and file enc sha fields are not applicable
//	}
//	_, err = cli.SendMessage(context.Background(), newsletterJID, &waE2E.Message{
//		ImageMessage: imageMsg,
//	}, whatsmeow.SendRequestExtra{
//		// Unlike normal media, newsletters also include a "media handle" in the send request.
//		MediaHandle: resp.Handle,
//	})
//	// handle error again
func (cli *Client) UploadNewsletter(ctx context.Context, data []byte, appInfo MediaType) (resp UploadResponse, err error) {
	if cli == nil {
		return resp, ErrClientIsNil
	}
	return media.UploadNewsletter(ctx, cli.mediaT(), data, appInfo)
}

// UploadNewsletterReader uploads the given attachment to WhatsApp servers without encrypting it first.
//
// This is otherwise identical to [UploadNewsletter], but it reads the plaintext from an [io.Reader] instead of a byte slice.
// Unlike [UploadReader], this does not require a temporary file. However, the data needs to be hashed first,
// so an [io.ReadSeeker] is required to be able to read the data twice.
func (cli *Client) UploadNewsletterReader(ctx context.Context, data io.ReadSeeker, appInfo MediaType) (resp UploadResponse, err error) {
	if cli == nil {
		return resp, ErrClientIsNil
	}
	return media.UploadNewsletterReader(ctx, cli.mediaT(), data, appInfo)
}
