package sticker

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/vincent-petithory/dataurl"
)

// ProcessStickerData decodes a data URI sticker and converts to WebP.
func ProcessStickerData(stickerData, mimeOverride, packID, packName, packPublisher string, emojis []string) ([]byte, string, error) {
	if !strings.HasPrefix(stickerData, "data") {
		return nil, "", fmt.Errorf("data should start with \"data:mime/type;base64,\"")
	}
	dataURL, err := dataurl.DecodeString(stickerData)
	if err != nil {
		return nil, "", fmt.Errorf("could not decode base64 encoded data from payload")
	}
	filedata, mimeType, err := ConvertToWebPSticker(dataURL.Data, mimeOverride)
	if err != nil {
		return nil, "", err
	}
	if mimeType == "image/webp" {
		filedata = EmbedStickerEXIF(filedata, packID, packName, packPublisher, emojis)
	}
	return filedata, mimeType, nil
}

// ConvertToWebPSticker converts input data to WebP sticker format.
func ConvertToWebPSticker(data []byte, mimeOverride string) ([]byte, string, error) {
	mimeType := http.DetectContentType(data)
	if mimeOverride != "" {
		mimeType = mimeOverride
	}
	switch {
	case strings.HasPrefix(mimeType, "video/"), mimeType == "image/gif":
		converted, err := ConvertVideoStickerToWebP(data)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert video/gif sticker to webp: %w", err)
		}
		return converted, "image/webp", nil
	case mimeType == "image/jpeg", mimeType == "image/png", mimeType == "image/jpg":
		converted, err := ConvertImageToWebP(data)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert image sticker to webp: %w", err)
		}
		return converted, "image/webp", nil
	default:
		return data, mimeType, nil
	}
}

// EmbedStickerEXIF injects WhatsApp sticker metadata into a WebP.
func EmbedStickerEXIF(inputWebP []byte, packID, packName, packPublisher string, emojis []string) []byte {
	meta := BuildStickerMetadata(packID, packName, packPublisher, emojis)
	if meta == nil {
		return inputWebP
	}
	exifData := BuildWhatsAppEXIF(meta)
	out, err := InjectWebPEXIF(inputWebP, exifData)
	if err != nil {
		log.Warn().Err(err).Msg("failed to inject EXIF chunk; sending sticker without metadata")
		return inputWebP
	}
	return out
}

// BuildStickerMetadata creates a sticker metadata map.
func BuildStickerMetadata(packID, packName, packPublisher string, emojis []string) map[string]interface{} {
	if packID == "" && packName == "" && packPublisher == "" && len(emojis) == 0 {
		return nil
	}
	meta := make(map[string]interface{})
	if packID != ""       { meta["sticker-pack-id"] = packID }
	if packName != ""     { meta["sticker-pack-name"] = packName }
	if packPublisher != "" { meta["sticker-pack-publisher"] = packPublisher }
	if len(emojis) > 0    { meta["emojis"] = emojis }
	return meta
}

// BuildWhatsAppEXIF encodes sticker metadata as WhatsApp EXIF bytes.
func BuildWhatsAppEXIF(meta map[string]interface{}) []byte {
	jsonBytes, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	header := []byte{
		0x49, 0x49, 0x2A, 0x00,
		0x08, 0x00, 0x00, 0x00,
		0x01, 0x00,
		0x41, 0x57,
		0x07, 0x00,
	}
	footer := []byte{0x16, 0x00, 0x00, 0x00}
	var buf bytes.Buffer
	buf.Write(header)
	binary.Write(&buf, binary.LittleEndian, uint32(len(jsonBytes)))
	buf.Write(footer)
	buf.Write(jsonBytes)
	return buf.Bytes()
}

// InjectWebPEXIF injects EXIF data into a WebP file.
func InjectWebPEXIF(in, exif []byte) ([]byte, error) {
	if !IsValidWebP(in) {
		return nil, fmt.Errorf("not a RIFF WEBP file")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(in))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image config: %w", err)
	}
	chunks, vp8xIndex, err := ParseWebPChunks(in)
	if err != nil {
		return nil, err
	}
	chunks = EnsureVP8XWithEXIF(chunks, vp8xIndex, cfg.Width, cfg.Height)
	return AssembleWebP(chunks, exif), nil
}

// IsValidWebP checks if data is a valid RIFF WEBP.
func IsValidWebP(data []byte) bool {
	return len(data) >= RiffHeaderSize &&
		string(data[0:4]) == "RIFF" &&
		string(data[8:12]) == "WEBP"
}

// ParseWebPChunks parses WebP chunks, skipping EXIF.
func ParseWebPChunks(in []byte) (chunks [][]byte, vp8xIndex int, err error) {
	vp8xIndex = -1
	pos := RiffHeaderSize
	for pos+ChunkHeaderSize <= len(in) {
		tag := string(in[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(in[pos+4 : pos+8]))
		dataEnd := pos + ChunkHeaderSize + size
		if dataEnd > len(in) {
			return nil, -1, fmt.Errorf("truncated webp chunk: %s", tag)
		}
		pad := size & 1
		if tag == "VP8X" && size >= Vp8xPayloadSize {
			vp8xIndex = len(chunks)
		}
		if tag != "EXIF" {
			chunk := make([]byte, ChunkHeaderSize+size+pad)
			copy(chunk, in[pos:dataEnd])
			if pad == 1 {
				chunk[ChunkHeaderSize+size] = 0
			}
			chunks = append(chunks, chunk)
		}
		pos = dataEnd + pad
	}
	return chunks, vp8xIndex, nil
}

// EnsureVP8XWithEXIF ensures a VP8X chunk with EXIF flag exists.
func EnsureVP8XWithEXIF(chunks [][]byte, vp8xIndex, width, height int) [][]byte {
	if vp8xIndex >= 0 {
		chunks[vp8xIndex][Vp8xFlagsOffset] |= Vp8xFlagEXIF
		return chunks
	}
	return append([][]byte{CreateVP8XChunk(width, height)}, chunks...)
}

// CreateVP8XChunk creates a VP8X chunk for the given dimensions.
func CreateVP8XChunk(width, height int) []byte {
	chunk := make([]byte, Vp8xChunkSize)
	copy(chunk[0:4], "VP8X")
	binary.LittleEndian.PutUint32(chunk[4:8], Vp8xPayloadSize)
	chunk[Vp8xFlagsOffset] = Vp8xFlagEXIF
	PutUint24LE(chunk[Vp8xWidthOffset:], width-1)
	PutUint24LE(chunk[Vp8xHeightOffset:], height-1)
	return chunk
}

// PutUint24LE writes a 24-bit little-endian unsigned int.
func PutUint24LE(b []byte, v int) {
	b[0] = uint8(v)
	b[1] = uint8(v >> 8)
	b[2] = uint8(v >> 16)
}

// AssembleWebP assembles chunk list + EXIF into a WebP file.
func AssembleWebP(chunks [][]byte, exif []byte) []byte {
	var out bytes.Buffer
	out.WriteString("RIFF")
	out.Write([]byte{0, 0, 0, 0})
	out.WriteString("WEBP")
	for _, c := range chunks {
		out.Write(c)
	}
	WriteChunk(&out, "EXIF", exif)
	b := out.Bytes()
	binary.LittleEndian.PutUint32(b[RiffSizeOffset:], uint32(len(b)-8))
	return b
}

// WriteChunk writes a tagged chunk to the buffer.
func WriteChunk(buf *bytes.Buffer, tag string, data []byte) {
	buf.WriteString(tag)
	sz := make([]byte, 4)
	binary.LittleEndian.PutUint32(sz, uint32(len(data)))
	buf.Write(sz)
	buf.Write(data)
	if len(data)%2 == 1 {
		buf.WriteByte(0)
	}
}
