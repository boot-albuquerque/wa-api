// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"wa-api/internal/wa-noise/capabilities/media"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMediaTransport"
	"wa-api/internal/wa-noise/protocol/types"
)

// A implementacao vive em internal/wa-noise/media; estes metodos sao apenas a
// fachada historica de *Client (ADR-0004, Fase F/G lote 1). Todos checam o
// receiver nil antes de tocar em qualquer campo — antes da extracao so'
// Download e DownloadToFile faziam isso e os demais estouravam em nil deref.

// ReturnDownloadWarnings controls whether the Download function returns non-fatal validation warnings.
// Currently, these include [ErrFileLengthMismatch] and [ErrInvalidMediaSHA256].
var ReturnDownloadWarnings = true

// DownloadAny loops through the downloadable parts of the given message and downloads the first non-nil item.
//
// Deprecated: it's recommended to find the specific message type you want to download manually and use the Download method instead.
func (cli *Client) DownloadAny(ctx context.Context, msg *waE2E.Message) (data []byte, err error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return media.DownloadAny(ctx, cli.mediaT(), msg)
}

// DownloadThumbnail downloads a thumbnail from a message.
//
// This is primarily intended for downloading link preview thumbnails, which are in ExtendedTextMessage:
//
//	var msg *waE2E.Message
//	...
//	thumbnailImageBytes, err := cli.DownloadThumbnail(msg.GetExtendedTextMessage())
func (cli *Client) DownloadThumbnail(ctx context.Context, msg DownloadableThumbnail) ([]byte, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return media.DownloadThumbnail(ctx, cli.mediaT(), msg)
}

// FetchStickerPack fetches the metadata of a sticker pack from the static endpoint.
func (cli *Client) FetchStickerPack(ctx context.Context, packID string) (*types.StickerPack, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return media.FetchStickerPack(ctx, cli.mediaT(), packID)
}

// Download downloads the attachment from the given protobuf message.
//
// The attachment is a specific part of a Message protobuf struct, not the message itself, e.g.
//
//	var msg *waE2E.Message
//	...
//	imageData, err := cli.Download(msg.GetImageMessage())
//
// You can also use DownloadAny to download the first non-nil sub-message.
func (cli *Client) Download(ctx context.Context, msg DownloadableMessage) ([]byte, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return media.DownloadMessage(ctx, cli.mediaT(), msg)
}

// DownloadFB downloads an attachment described by a Messenger media transport.
func (cli *Client) DownloadFB(
	ctx context.Context,
	transport *waMediaTransport.WAMediaTransport_Integral,
	mediaType MediaType,
) ([]byte, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return media.DownloadFB(ctx, cli.mediaT(), transport, mediaType)
}

// DownloadMediaWithPath downloads an attachment by manually specifying the path and encryption details.
func (cli *Client) DownloadMediaWithPath(
	ctx context.Context,
	directPath string,
	encFileHash, fileHash, mediaKey []byte,
	fileLength int,
	mediaType MediaType,
	mmsType string,
) (data []byte, err error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return media.DownloadWithPath(
		ctx, cli.mediaT(), directPath, encFileHash, fileHash, mediaKey, fileLength, mediaType, mmsType,
	)
}
