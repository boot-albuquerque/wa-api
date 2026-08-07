// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"net/http"

	"wa-api/internal/wa-noise/capabilities/media"
)

// Metodos finos que mantem a superficie interna historica de *Client viva para
// DangerousInternalClient (internals.go, gerado). A implementacao esta' em
// internal/wa-noise/media/download_transport.go.

func (cli *Client) downloadAndDecrypt(
	ctx context.Context,
	url string,
	mediaKey []byte,
	appInfo MediaType,
	fileLength int,
	fileEncSHA256,
	fileSHA256 []byte,
) (data []byte, err error) {
	return media.DownloadAndDecrypt(ctx, cli.mediaT(), url, mediaKey, appInfo, fileLength, fileEncSHA256, fileSHA256)
}

func (cli *Client) downloadPossiblyEncryptedMediaWithRetries(ctx context.Context, url string, checksum []byte) (file, mac []byte, err error) {
	return media.DownloadPossiblyEncryptedWithRetries(ctx, cli.mediaT(), url, checksum)
}

func (cli *Client) doMediaDownloadRequest(ctx context.Context, url string) (*http.Response, error) {
	return media.DoDownloadRequest(ctx, cli.mediaT(), url)
}

func (cli *Client) downloadMedia(ctx context.Context, url string) ([]byte, error) {
	return media.DownloadRaw(ctx, cli.mediaT(), url)
}

func (cli *Client) downloadEncryptedMedia(ctx context.Context, url string, checksum []byte) (file, mac []byte, err error) {
	return media.DownloadEncrypted(ctx, cli.mediaT(), url, checksum)
}
