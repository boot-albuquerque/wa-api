// Package sticker provides WhatsApp sticker creation and WebP manipulation,
// extracted from root helpers.go (Phase 12a).
package sticker

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"os/exec"

	"github.com/rs/zerolog/log"
)

// JPEG quality for Open Graph thumbnails.
const JpegQuality = 80

// WebP RIFF container constants.
const (
	RiffHeaderSize  = 12
	ChunkHeaderSize = 8
	RiffSizeOffset  = 4
	Vp8xChunkSize   = ChunkHeaderSize + 10
	Vp8xPayloadSize = 10
	Vp8xFlagsOffset = ChunkHeaderSize
	Vp8xWidthOffset = ChunkHeaderSize + 4
	Vp8xHeightOffset = ChunkHeaderSize + 7
	Vp8xFlagEXIF    byte = 0x08
)

// EncodeJPEGThumbnail encodes an image as a JPEG byte slice.
func EncodeJPEGThumbnail(img image.Image) []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: JpegQuality}); err != nil {
		log.Warn().Err(err).Msg("Failed to encode thumbnail to JPEG")
		return nil
	}
	return buf.Bytes()
}

// RunFFmpegConversion runs ffmpeg with the given arguments.
func RunFFmpegConversion(input []byte, inputExt string, ffmpegArgs func(inPath, outPath string) []string, errMsg string) ([]byte, error) {
	inFile, err := os.CreateTemp("", "sticker-input-*"+inputExt)
	if err != nil {
		return nil, err
	}
	defer os.Remove(inFile.Name())
	defer inFile.Close()
	if _, err := inFile.Write(input); err != nil {
		return nil, err
	}
	outFile, err := os.CreateTemp("", "sticker-output-*.webp")
	if err != nil {
		return nil, err
	}
	outPath := outFile.Name()
	outFile.Close()
	defer os.Remove(outPath)
	args := ffmpegArgs(inFile.Name(), outPath)
	cmd := exec.Command("ffmpeg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Error().Err(err).Str("stderr", stderr.String()).Msg(errMsg)
		return nil, err
	}
	return os.ReadFile(outPath)
}

// ConvertVideoStickerToWebP converts a video sticker to WebP via ffmpeg.
func ConvertVideoStickerToWebP(input []byte) ([]byte, error) {
	return RunFFmpegConversion(input, ".mp4", func(inPath, outPath string) []string {
		return []string{"-y", "-t", "10", "-i", inPath, "-vf", "fps=15,scale=512:512", "-loop", "0", "-an", "-vsync", "0", "-fs", "1000000", "-c:v", "libwebp", "-qscale:v", "10", outPath}
	}, "ffmpeg failed converting video sticker")
}

// ConvertImageToWebP converts an image to WebP via ffmpeg.
func ConvertImageToWebP(input []byte) ([]byte, error) {
	return RunFFmpegConversion(input, ".img", func(inPath, outPath string) []string {
		return []string{"-y", "-i", inPath, "-vf", "scale=512:512", "-c:v", "libwebp", "-lossless", "1", outPath}
	}, "ffmpeg failed converting image sticker")
}
