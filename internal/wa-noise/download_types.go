// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waHistorySync"
	"wa-api/internal/wa-noise/proto/waServerSync"
	"wa-api/internal/wa-noise/types"
)

// MediaType represents a type of uploaded file on WhatsApp.
// The value is the key which is used as a part of generating the encryption keys.
type MediaType string

// The known media types
const (
	MediaImage    MediaType = "WhatsApp Image Keys"
	MediaVideo    MediaType = "WhatsApp Video Keys"
	MediaAudio    MediaType = "WhatsApp Audio Keys"
	MediaDocument MediaType = "WhatsApp Document Keys"
	MediaHistory  MediaType = "WhatsApp History Keys"
	MediaAppState MediaType = "WhatsApp App State Keys"

	MediaStickerPack   MediaType = "WhatsApp Sticker Pack Keys"
	MediaLinkThumbnail MediaType = "WhatsApp Link Thumbnail Keys"
)

// DownloadableMessage represents a protobuf message that contains attachment info.
//
// All of the downloadable messages inside a Message struct implement this interface
// (ImageMessage, VideoMessage, AudioMessage, DocumentMessage, StickerMessage).
type DownloadableMessage interface {
	GetDirectPath() string
	GetMediaKey() []byte
	GetFileSHA256() []byte
	GetFileEncSHA256() []byte
}

type MediaTypeable interface {
	GetMediaType() MediaType
}

// DownloadableThumbnail represents a protobuf message that contains a thumbnail attachment.
//
// This is primarily meant for link preview thumbnails in ExtendedTextMessage.
type DownloadableThumbnail interface {
	proto.Message
	GetThumbnailDirectPath() string
	GetThumbnailSHA256() []byte
	GetThumbnailEncSHA256() []byte
	GetMediaKey() []byte
}

// All the message types that are intended to be downloadable
var (
	_ DownloadableMessage   = (*waE2E.ImageMessage)(nil)
	_ DownloadableMessage   = (*waE2E.AudioMessage)(nil)
	_ DownloadableMessage   = (*waE2E.VideoMessage)(nil)
	_ DownloadableMessage   = (*waE2E.DocumentMessage)(nil)
	_ DownloadableMessage   = (*waE2E.StickerMessage)(nil)
	_ DownloadableMessage   = (*waE2E.StickerPackMessage)(nil)
	_ DownloadableMessage   = (*waHistorySync.StickerMetadata)(nil)
	_ DownloadableMessage   = (*waE2E.HistorySyncNotification)(nil)
	_ DownloadableMessage   = (*waServerSync.ExternalBlobReference)(nil)
	_ DownloadableThumbnail = (*waE2E.ExtendedTextMessage)(nil)
	_ DownloadableMessage   = (*types.StickerPackItem)(nil)
)

type downloadableMessageWithLength interface {
	DownloadableMessage
	GetFileLength() uint64
}

type downloadableMessageWithSizeBytes interface {
	DownloadableMessage
	GetFileSizeBytes() uint64
}

type downloadableMessageWithURL interface {
	DownloadableMessage
	GetURL() string
}

var classToMediaType = map[protoreflect.Name]MediaType{
	"ImageMessage":    MediaImage,
	"AudioMessage":    MediaAudio,
	"VideoMessage":    MediaVideo,
	"DocumentMessage": MediaDocument,
	"StickerMessage":  MediaImage,
	"StickerMetadata": MediaImage,

	"StickerPackMessage":      MediaStickerPack,
	"HistorySyncNotification": MediaHistory,
	"ExternalBlobReference":   MediaAppState,
}

var classToThumbnailMediaType = map[protoreflect.Name]MediaType{
	"ExtendedTextMessage": MediaLinkThumbnail,
}

var mediaTypeToMMSType = map[MediaType]string{
	MediaImage:    "image",
	MediaAudio:    "audio",
	MediaVideo:    "video",
	MediaDocument: "document",
	MediaHistory:  "md-msg-hist",
	MediaAppState: "md-app-state",

	MediaStickerPack:   "sticker-pack",
	MediaLinkThumbnail: "thumbnail-link",
}

func getSize(msg DownloadableMessage) int {
	switch sized := msg.(type) {
	case downloadableMessageWithLength:
		return int(sized.GetFileLength())
	case downloadableMessageWithSizeBytes:
		return int(sized.GetFileSizeBytes())
	default:
		return -1
	}
}

// GetMediaType returns the MediaType value corresponding to the given protobuf message.
func GetMediaType(msg DownloadableMessage) MediaType {
	switch typedMsg := msg.(type) {
	case *types.StickerPackItem:
		return MediaImage
	case proto.Message:
		return classToMediaType[typedMsg.ProtoReflect().Descriptor().Name()]
	case MediaTypeable:
		return typedMsg.GetMediaType()
	default:
		return ""
	}
}
