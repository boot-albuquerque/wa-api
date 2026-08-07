package media

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/protocol/types"
)

// Type representa um tipo de arquivo enviado ao WhatsApp. O valor e' a chave
// usada como parte da derivacao das chaves de cifragem.
type Type string

// Os tipos de midia conhecidos.
const (
	TypeImage    Type = "WhatsApp Image Keys"
	TypeVideo    Type = "WhatsApp Video Keys"
	TypeAudio    Type = "WhatsApp Audio Keys"
	TypeDocument Type = "WhatsApp Document Keys"
	TypeHistory  Type = "WhatsApp History Keys"
	TypeAppState Type = "WhatsApp App State Keys"

	TypeStickerPack   Type = "WhatsApp Sticker Pack Keys"
	TypeLinkThumbnail Type = "WhatsApp Link Thumbnail Keys"
)

// Downloadable representa uma mensagem protobuf que contem info de anexo.
//
// Todas as mensagens baixaveis dentro de um Message implementam esta interface
// (ImageMessage, VideoMessage, AudioMessage, DocumentMessage, StickerMessage).
type Downloadable interface {
	GetDirectPath() string
	GetMediaKey() []byte
	GetFileSHA256() []byte
	GetFileEncSHA256() []byte
}

// Typeable e' implementada por mensagens que sabem dizer o proprio Type.
type Typeable interface {
	GetMediaType() Type
}

// DownloadableThumbnail representa uma mensagem protobuf que contem um
// thumbnail anexado — principalmente o preview de link em ExtendedTextMessage.
type DownloadableThumbnail interface {
	proto.Message
	GetThumbnailDirectPath() string
	GetThumbnailSHA256() []byte
	GetThumbnailEncSHA256() []byte
	GetMediaKey() []byte
}

// Todos os tipos de mensagem que devem ser baixaveis.
var (
	_ Downloadable          = (*waE2E.ImageMessage)(nil)
	_ Downloadable          = (*waE2E.AudioMessage)(nil)
	_ Downloadable          = (*waE2E.VideoMessage)(nil)
	_ Downloadable          = (*waE2E.DocumentMessage)(nil)
	_ Downloadable          = (*waE2E.StickerMessage)(nil)
	_ Downloadable          = (*waE2E.StickerPackMessage)(nil)
	_ Downloadable          = (*waHistorySync.StickerMetadata)(nil)
	_ Downloadable          = (*waE2E.HistorySyncNotification)(nil)
	_ Downloadable          = (*waServerSync.ExternalBlobReference)(nil)
	_ DownloadableThumbnail = (*waE2E.ExtendedTextMessage)(nil)
	_ Downloadable          = (*types.StickerPackItem)(nil)
)

type downloadableWithLength interface {
	Downloadable
	GetFileLength() uint64
}

type downloadableWithSizeBytes interface {
	Downloadable
	GetFileSizeBytes() uint64
}

type downloadableWithURL interface {
	Downloadable
	GetURL() string
}

var classToMediaType = map[protoreflect.Name]Type{
	"ImageMessage":    TypeImage,
	"AudioMessage":    TypeAudio,
	"VideoMessage":    TypeVideo,
	"DocumentMessage": TypeDocument,
	"StickerMessage":  TypeImage,
	"StickerMetadata": TypeImage,

	"StickerPackMessage":      TypeStickerPack,
	"HistorySyncNotification": TypeHistory,
	"ExternalBlobReference":   TypeAppState,
}

var classToThumbnailMediaType = map[protoreflect.Name]Type{
	"ExtendedTextMessage": TypeLinkThumbnail,
}

var mediaTypeToMMSType = map[Type]string{
	TypeImage:    "image",
	TypeAudio:    "audio",
	TypeVideo:    "video",
	TypeDocument: "document",
	TypeHistory:  "md-msg-hist",
	TypeAppState: "md-app-state",

	TypeStickerPack:   "sticker-pack",
	TypeLinkThumbnail: "thumbnail-link",
}

// MMSType devolve o mms-type do tipo de midia dado, ou "" se nao houver.
func MMSType(mediaType Type) string {
	return mediaTypeToMMSType[mediaType]
}

func getSize(msg Downloadable) int {
	switch sized := msg.(type) {
	case downloadableWithLength:
		return int(sized.GetFileLength())
	case downloadableWithSizeBytes:
		return int(sized.GetFileSizeBytes())
	default:
		return UnknownFileLength
	}
}

// GetType devolve o Type correspondente a mensagem protobuf dada.
func GetType(msg Downloadable) Type {
	switch typedMsg := msg.(type) {
	case *types.StickerPackItem:
		return TypeImage
	case proto.Message:
		return classToMediaType[typedMsg.ProtoReflect().Descriptor().Name()]
	case Typeable:
		return typedMsg.GetMediaType()
	default:
		return ""
	}
}
