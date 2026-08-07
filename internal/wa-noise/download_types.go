// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"wa-api/internal/wa-noise/media"
)

// Os tipos do dominio de midia vivem em internal/wa-noise/media; aqui ficam
// apelidos que preservam a API historica do pacote raiz (ADR-0004, Fase F/G
// lote 1). Como sao apelidos (=), e nao definicoes novas, qualquer valor
// atravessa a fronteira dos dois pacotes sem conversao.

// MediaType represents a type of uploaded file on WhatsApp.
// The value is the key which is used as a part of generating the encryption keys.
type MediaType = media.Type

// The known media types
const (
	MediaImage    = media.TypeImage
	MediaVideo    = media.TypeVideo
	MediaAudio    = media.TypeAudio
	MediaDocument = media.TypeDocument
	MediaHistory  = media.TypeHistory
	MediaAppState = media.TypeAppState

	MediaStickerPack   = media.TypeStickerPack
	MediaLinkThumbnail = media.TypeLinkThumbnail
)

// DownloadableMessage represents a protobuf message that contains attachment info.
//
// All of the downloadable messages inside a Message struct implement this interface
// (ImageMessage, VideoMessage, AudioMessage, DocumentMessage, StickerMessage).
type DownloadableMessage = media.Downloadable

// MediaTypeable is implemented by messages that know their own MediaType.
type MediaTypeable = media.Typeable

// DownloadableThumbnail represents a protobuf message that contains a thumbnail attachment.
//
// This is primarily meant for link preview thumbnails in ExtendedTextMessage.
type DownloadableThumbnail = media.DownloadableThumbnail

// GetMediaType returns the MediaType value corresponding to the given protobuf message.
func GetMediaType(msg DownloadableMessage) MediaType {
	return media.GetType(msg)
}
