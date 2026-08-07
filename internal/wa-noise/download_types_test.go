// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waHistorySync"
	"wa-api/internal/wa-noise/proto/waServerSync"
	"wa-api/internal/wa-noise/types"
)

func TestGetMediaTypeMapsCadaMensagemBaixavel(t *testing.T) {
	tests := []struct {
		name string
		msg  DownloadableMessage
		want MediaType
	}{
		{"imagem", &waE2E.ImageMessage{}, MediaImage},
		{"audio", &waE2E.AudioMessage{}, MediaAudio},
		{"video", &waE2E.VideoMessage{}, MediaVideo},
		{"documento", &waE2E.DocumentMessage{}, MediaDocument},
		{"figurinha", &waE2E.StickerMessage{}, MediaImage},
		{"metadado de figurinha", &waHistorySync.StickerMetadata{}, MediaImage},
		{"pacote de figurinhas", &waE2E.StickerPackMessage{}, MediaStickerPack},
		{"history sync", &waE2E.HistorySyncNotification{}, MediaHistory},
		{"app state blob", &waServerSync.ExternalBlobReference{}, MediaAppState},
		{"item de pacote de figurinhas", &types.StickerPackItem{}, MediaImage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetMediaType(tt.msg); got != tt.want {
				t.Fatalf("GetMediaType() = %q, esperado %q", got, tt.want)
			}
		})
	}
}

// notDownloadable implementa DownloadableMessage sem ser proto.Message nem
// MediaTypeable — o caso default de GetMediaType.
type notDownloadable struct{}

func (notDownloadable) GetDirectPath() string    { return "" }
func (notDownloadable) GetMediaKey() []byte      { return nil }
func (notDownloadable) GetFileSHA256() []byte    { return nil }
func (notDownloadable) GetFileEncSHA256() []byte { return nil }

func TestGetMediaTypeDevolveVazioParaTipoDesconhecido(t *testing.T) {
	if got := GetMediaType(notDownloadable{}); got != "" {
		t.Fatalf("GetMediaType() = %q, esperado string vazia", got)
	}
}

// mediaTypeable exercita o ramo MediaTypeable de GetMediaType.
type mediaTypeable struct{ notDownloadable }

func (mediaTypeable) GetMediaType() MediaType { return MediaDocument }

func TestGetMediaTypeUsaMediaTypeable(t *testing.T) {
	if got := GetMediaType(mediaTypeable{}); got != MediaDocument {
		t.Fatalf("GetMediaType() = %q, esperado %q", got, MediaDocument)
	}
}

// sizeBytesMessage cobre o ramo downloadableMessageWithSizeBytes de getSize.
// Nao ha' tipo de producao que o satisfaca hoje: types.StickerPackItem tem
// GetFileSizeBytes() int64, e a interface exige uint64 (ver PATCHES.md,
// Fase E lote 1).
type sizeBytesMessage struct{ notDownloadable }

func (sizeBytesMessage) GetFileSizeBytes() uint64 { return 99 }

func TestGetSize(t *testing.T) {
	t.Run("usa FileLength quando existe", func(t *testing.T) {
		msg := &waE2E.ImageMessage{FileLength: proto.Uint64(4242)}
		if got := getSize(msg); got != 4242 {
			t.Fatalf("getSize() = %d, esperado 4242", got)
		}
	})
	t.Run("usa FileSizeBytes quando e' o campo disponivel", func(t *testing.T) {
		if got := getSize(sizeBytesMessage{}); got != 99 {
			t.Fatalf("getSize() = %d, esperado 99", got)
		}
	})
	t.Run("devolve o sentinela quando nao ha' campo de tamanho", func(t *testing.T) {
		if got := getSize(notDownloadable{}); got != unknownFileLength {
			t.Fatalf("getSize() = %d, esperado unknownFileLength (%d)", got, unknownFileLength)
		}
	})
}

// unknownFileLength precisa continuar negativo: e' assim que downloadAndDecrypt
// e downloadAndDecryptToFile decidem pular a validacao de tamanho.
func TestUnknownFileLengthDesligaValidacaoDeTamanho(t *testing.T) {
	if unknownFileLength >= 0 {
		t.Fatalf("unknownFileLength = %d, precisa ser negativo", unknownFileLength)
	}
}

// Todo MediaType que uma mensagem baixavel pode produzir precisa ter mms-type,
// senao a URL de download sai com "mms-type=" vazio e o servidor recusa.
func TestTodoMediaTypeTemMMSType(t *testing.T) {
	for name, mediaType := range classToMediaType {
		if _, ok := mediaTypeToMMSType[mediaType]; !ok {
			t.Errorf("classToMediaType[%q] = %q nao tem entrada em mediaTypeToMMSType", name, mediaType)
		}
	}
	for name, mediaType := range classToThumbnailMediaType {
		if _, ok := mediaTypeToMMSType[mediaType]; !ok {
			t.Errorf("classToThumbnailMediaType[%q] = %q nao tem entrada em mediaTypeToMMSType", name, mediaType)
		}
	}
}

// Os valores de MediaType entram no HKDF como info, entao dois tipos distintos
// nao podem colidir — colisao daria a mesma chave para midias diferentes.
func TestMediaTypesSaoDistintos(t *testing.T) {
	all := []MediaType{
		MediaImage, MediaVideo, MediaAudio, MediaDocument,
		MediaHistory, MediaAppState, MediaStickerPack, MediaLinkThumbnail,
	}
	seen := make(map[MediaType]bool, len(all))
	for _, mt := range all {
		if mt == "" {
			t.Error("MediaType vazio na lista de tipos conhecidos")
		}
		if seen[mt] {
			t.Errorf("MediaType duplicado: %q", mt)
		}
		seen[mt] = true
	}
}
