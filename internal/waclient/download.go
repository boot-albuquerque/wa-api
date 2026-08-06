// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"wa-api/internal/waclient/proto/waE2E"
	"wa-api/internal/waclient/proto/waMediaTransport"
	"wa-api/internal/waclient/types"
)

// DownloadAny loops through the downloadable parts of the given message and downloads the first non-nil item.
//
// Deprecated: it's recommended to find the specific message type you want to download manually and use the Download method instead.
func (cli *Client) DownloadAny(ctx context.Context, msg *waE2E.Message) (data []byte, err error) {
	if msg == nil {
		return nil, ErrNothingDownloadableFound
	}
	switch {
	case msg.ImageMessage != nil:
		return cli.Download(ctx, msg.ImageMessage)
	case msg.VideoMessage != nil:
		return cli.Download(ctx, msg.VideoMessage)
	case msg.AudioMessage != nil:
		return cli.Download(ctx, msg.AudioMessage)
	case msg.DocumentMessage != nil:
		return cli.Download(ctx, msg.DocumentMessage)
	case msg.StickerMessage != nil:
		return cli.Download(ctx, msg.StickerMessage)
	default:
		return nil, ErrNothingDownloadableFound
	}
}

// ReturnDownloadWarnings controls whether the Download function returns non-fatal validation warnings.
// Currently, these include [ErrFileLengthMismatch] and [ErrInvalidMediaSHA256].
var ReturnDownloadWarnings = true

// DownloadThumbnail downloads a thumbnail from a message.
//
// This is primarily intended for downloading link preview thumbnails, which are in ExtendedTextMessage:
//
//	var msg *waE2E.Message
//	...
//	thumbnailImageBytes, err := cli.DownloadThumbnail(msg.GetExtendedTextMessage())
func (cli *Client) DownloadThumbnail(ctx context.Context, msg DownloadableThumbnail) ([]byte, error) {
	mediaType, ok := classToThumbnailMediaType[msg.ProtoReflect().Descriptor().Name()]
	if !ok {
		return nil, fmt.Errorf("%w '%s'", ErrUnknownMediaType, string(msg.ProtoReflect().Descriptor().Name()))
	} else if len(msg.GetThumbnailDirectPath()) > 0 {
		return cli.DownloadMediaWithPath(ctx, msg.GetThumbnailDirectPath(), msg.GetThumbnailEncSHA256(), msg.GetThumbnailSHA256(), msg.GetMediaKey(), -1, mediaType, mediaTypeToMMSType[mediaType])
	} else {
		return nil, ErrNoURLPresent
	}
}

func (cli *Client) FetchStickerPack(ctx context.Context, packID string) (*types.StickerPack, error) {
	url := fmt.Sprintf("https://static.whatsapp.net/sticker?lottie=1&cat=sticker_pack_data&id=%s&lg=en", packID)
	resp, err := cli.doMediaDownloadRequest(ctx, url)
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
	mediaType := GetMediaType(msg)
	if mediaType == "" {
		return nil, fmt.Errorf("%w %T", ErrUnknownMediaType, msg)
	}
	urlable, ok := msg.(downloadableMessageWithURL)
	var url string
	var isWebWhatsappNetURL bool
	if ok {
		url = urlable.GetURL()
		isWebWhatsappNetURL = strings.HasPrefix(url, "https://web.whatsapp.net")
	}
	if len(url) > 0 && !isWebWhatsappNetURL {
		return cli.downloadAndDecrypt(ctx, url, msg.GetMediaKey(), mediaType, getSize(msg), msg.GetFileEncSHA256(), msg.GetFileSHA256())
	} else if len(msg.GetDirectPath()) > 0 {
		return cli.DownloadMediaWithPath(ctx, msg.GetDirectPath(), msg.GetFileEncSHA256(), msg.GetFileSHA256(), msg.GetMediaKey(), getSize(msg), mediaType, mediaTypeToMMSType[mediaType])
	} else {
		if isWebWhatsappNetURL {
			cli.Log.Warnf("Got a media message with a web.whatsapp.net URL (%s) and no direct path", url)
		}
		return nil, ErrNoURLPresent
	}
}

func (cli *Client) DownloadFB(
	ctx context.Context,
	transport *waMediaTransport.WAMediaTransport_Integral,
	mediaType MediaType,
) ([]byte, error) {
	return cli.DownloadMediaWithPath(ctx, transport.GetDirectPath(), transport.GetFileEncSHA256(), transport.GetFileSHA256(), transport.GetMediaKey(), -1, mediaType, mediaTypeToMMSType[mediaType])
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
	if !strings.HasPrefix(directPath, "/") {
		return nil, fmt.Errorf("media download path does not start with slash: %s", directPath)
	}
	var mediaConn *MediaConn
	mediaConn, err = cli.refreshMediaConn(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh media connections: %w", err)
	}
	if len(mmsType) == 0 {
		mmsType = mediaTypeToMMSType[mediaType]
	}
	for i, host := range mediaConn.Hosts {
		// TODO omit hash for unencrypted media?
		mediaURL := fmt.Sprintf("https://%s%s&hash=%s&mms-type=%s&__wa-mms=", host.Hostname, directPath, base64.URLEncoding.EncodeToString(encFileHash), mmsType)
		data, err = cli.downloadAndDecrypt(ctx, mediaURL, mediaKey, mediaType, fileLength, encFileHash, fileHash)
		if err == nil ||
			errors.Is(err, ErrFileLengthMismatch) ||
			errors.Is(err, ErrInvalidMediaSHA256) ||
			errors.Is(err, ErrMediaDownloadFailedWith403) ||
			errors.Is(err, ErrMediaDownloadFailedWith404) ||
			errors.Is(err, ErrMediaDownloadFailedWith410) ||
			errors.Is(err, context.Canceled) {
			return
		} else if i >= len(mediaConn.Hosts)-1 {
			return nil, fmt.Errorf("failed to download media from last host: %w", err)
		}
		cli.Log.Warnf("Failed to download media: %s, trying with next host...", err)
	}
	return
}
