// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
)

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
	resp.FileLength = uint64(len(data))
	hash := sha256.Sum256(data)
	resp.FileSHA256 = hash[:]
	err = cli.rawUpload(ctx, bytes.NewReader(data), resp.FileLength, resp.FileSHA256, appInfo, true, &resp)
	return
}

// UploadNewsletterReader uploads the given attachment to WhatsApp servers without encrypting it first.
//
// This is otherwise identical to [UploadNewsletter], but it reads the plaintext from an [io.Reader] instead of a byte slice.
// Unlike [UploadReader], this does not require a temporary file. However, the data needs to be hashed first,
// so an [io.ReadSeeker] is required to be able to read the data twice.
func (cli *Client) UploadNewsletterReader(ctx context.Context, data io.ReadSeeker, appInfo MediaType) (resp UploadResponse, err error) {
	hasher := sha256.New()
	var fileLength int64
	fileLength, err = io.Copy(hasher, data)
	resp.FileLength = uint64(fileLength)
	resp.FileSHA256 = hasher.Sum(nil)
	_, err = data.Seek(0, io.SeekStart)
	if err != nil {
		err = fmt.Errorf("failed to seek to start of data: %w", err)
		return
	}
	err = cli.rawUpload(ctx, data, resp.FileLength, resp.FileSHA256, appInfo, true, &resp)
	return
}
