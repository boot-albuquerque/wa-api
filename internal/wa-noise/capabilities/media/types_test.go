// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package media

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/protocol/types"
)

func TestGetTypeMapsCadaMensagemBaixavel(t *testing.T) {
	tests := []struct {
		name string
		msg  Downloadable
		want Type
	}{
		{"imagem", &waE2E.ImageMessage{}, TypeImage},
		{"audio", &waE2E.AudioMessage{}, TypeAudio},
		{"video", &waE2E.VideoMessage{}, TypeVideo},
		{"documento", &waE2E.DocumentMessage{}, TypeDocument},
		{"figurinha", &waE2E.StickerMessage{}, TypeImage},
		{"metadado de figurinha", &waHistorySync.StickerMetadata{}, TypeImage},
		{"pacote de figurinhas", &waE2E.StickerPackMessage{}, TypeStickerPack},
		{"history sync", &waE2E.HistorySyncNotification{}, TypeHistory},
		{"app state blob", &waServerSync.ExternalBlobReference{}, TypeAppState},
		{"item de pacote de figurinhas", &types.StickerPackItem{}, TypeImage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetType(tt.msg); got != tt.want {
				t.Fatalf("GetType() = %q, esperado %q", got, tt.want)
			}
		})
	}
}

// notDownloadable implementa Downloadable sem ser proto.Message nem Typeable —
// o caso default de GetType.
type notDownloadable struct{}

func (notDownloadable) GetDirectPath() string    { return "" }
func (notDownloadable) GetMediaKey() []byte      { return nil }
func (notDownloadable) GetFileSHA256() []byte    { return nil }
func (notDownloadable) GetFileEncSHA256() []byte { return nil }

func TestGetTypeDevolveVazioParaTipoDesconhecido(t *testing.T) {
	if got := GetType(notDownloadable{}); got != "" {
		t.Fatalf("GetType() = %q, esperado string vazia", got)
	}
}

// mediaTypeable exercita o ramo Typeable de GetType.
type mediaTypeable struct{ notDownloadable }

func (mediaTypeable) GetMediaType() Type { return TypeDocument }

func TestGetTypeUsaTypeable(t *testing.T) {
	if got := GetType(mediaTypeable{}); got != TypeDocument {
		t.Fatalf("GetType() = %q, esperado %q", got, TypeDocument)
	}
}

// sizeBytesMessage cobre o ramo downloadableWithSizeBytes de getSize.
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
		if got := getSize(notDownloadable{}); got != UnknownFileLength {
			t.Fatalf("getSize() = %d, esperado UnknownFileLength (%d)", got, UnknownFileLength)
		}
	})
}

// UnknownFileLength precisa continuar negativo: e' assim que DownloadAndDecrypt
// e DownloadAndDecryptToFile decidem pular a validacao de tamanho.
func TestUnknownFileLengthDesligaValidacaoDeTamanho(t *testing.T) {
	if UnknownFileLength >= 0 {
		t.Fatalf("UnknownFileLength = %d, precisa ser negativo", UnknownFileLength)
	}
}

// Todo Type que uma mensagem baixavel pode produzir precisa ter mms-type,
// senao a URL de download sai com "mms-type=" vazio e o servidor recusa.
func TestTodoTypeTemMMSType(t *testing.T) {
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

func TestMMSType(t *testing.T) {
	if got := MMSType(TypeImage); got != "image" {
		t.Fatalf("MMSType(TypeImage) = %q, esperado \"image\"", got)
	}
	if got := MMSType("inexistente"); got != "" {
		t.Fatalf("MMSType() de tipo desconhecido = %q, esperado vazio", got)
	}
}

// Os valores de Type entram no HKDF como info, entao dois tipos distintos
// nao podem colidir — colisao daria a mesma chave para midias diferentes.
func TestTypesSaoDistintos(t *testing.T) {
	all := []Type{
		TypeImage, TypeVideo, TypeAudio, TypeDocument,
		TypeHistory, TypeAppState, TypeStickerPack, TypeLinkThumbnail,
	}
	seen := make(map[Type]bool, len(all))
	for _, mt := range all {
		if mt == "" {
			t.Error("Type vazio na lista de tipos conhecidos")
		}
		if seen[mt] {
			t.Errorf("Type duplicado: %q", mt)
		}
		seen[mt] = true
	}
}
