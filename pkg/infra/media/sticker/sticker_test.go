package sticker

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func base64Of(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// installFakeFFmpeg escreve um `ffmpeg` de mentira num diretorio temporario e
// coloca esse diretorio na frente do PATH. Assim os testes exercitam
// RunFFmpegConversion sem depender do binario real estar instalado — o que
// tornaria a suite dependente do ambiente.
func installFakeFFmpeg(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("o stub de ffmpeg depende de shebang POSIX")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("escrever ffmpeg falso: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// withFakeFFmpeg instala um ffmpeg que copia um payload fixo para o ultimo
// argumento — que e' sempre o caminho de saida nas duas invocacoes de producao.
func withFakeFFmpeg(t *testing.T, output []byte) {
	t.Helper()
	dir := t.TempDir()
	payload := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(payload, output, 0o600); err != nil {
		t.Fatalf("escrever payload do ffmpeg falso: %v", err)
	}
	installFakeFFmpeg(t, "#!/bin/sh\nfor a in \"$@\"; do out=\"$a\"; done\n"+
		"cp '"+payload+"' \"$out\"\n")
}

// withFailingFFmpeg instala um ffmpeg que sempre sai com codigo 1.
func withFailingFFmpeg(t *testing.T) {
	t.Helper()
	installFakeFFmpeg(t, "#!/bin/sh\necho 'stub ffmpeg failure' >&2\nexit 1\n")
}

// ---------------------------------------------------------------------------
// EncodeJPEGThumbnail
// ---------------------------------------------------------------------------

func TestEncodeJPEGThumbnail(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))

	out := EncodeJPEGThumbnail(img)

	if len(out) == 0 {
		t.Fatal("quero bytes JPEG, tenho vazio")
	}
	if !bytes.HasPrefix(out, []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("magic number = % x, quero FF D8 FF", out[:3])
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decodificar de volta: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("formato = %q, quero jpeg", format)
	}
	if cfg.Width != 16 || cfg.Height != 12 {
		t.Fatalf("dimensoes = %dx%d, quero 16x12", cfg.Width, cfg.Height)
	}
}

// oversizedImage anuncia dimensoes acima do limite de 16 bits do JPEG sem
// alocar nada. E' o unico caminho de erro real de jpeg.Encode quando o writer
// e' um bytes.Buffer, que nunca falha.
type oversizedImage struct{}

func (oversizedImage) ColorModel() color.Model { return color.RGBAModel }
func (oversizedImage) Bounds() image.Rectangle { return image.Rect(0, 0, 1<<16, 1<<16) }
func (oversizedImage) At(x, y int) color.Color { return color.RGBA{} }

func TestEncodeJPEGThumbnailErro(t *testing.T) {
	// Guarda: se o encoder deixar de rejeitar a dimensao, o teste vira ruido.
	if err := jpeg.Encode(&bytes.Buffer{}, oversizedImage{}, nil); err == nil {
		t.Fatal("jpeg.Encode deveria rejeitar dimensao >= 1<<16")
	}

	if out := EncodeJPEGThumbnail(oversizedImage{}); out != nil {
		t.Fatalf("quero nil quando o encode falha, tenho %d bytes", len(out))
	}
}

// ---------------------------------------------------------------------------
// RunFFmpegConversion
// ---------------------------------------------------------------------------

func TestRunFFmpegConversionSucesso(t *testing.T) {
	want := []byte("saida-webp-de-mentira")
	withFakeFFmpeg(t, want)

	var gotIn, gotOut string
	out, err := RunFFmpegConversion([]byte("entrada"), ".mp4", func(inPath, outPath string) []string {
		gotIn, gotOut = inPath, outPath
		return []string{"-i", inPath, outPath}
	}, "nao deveria falhar")

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !bytes.Equal(out, want) {
		t.Fatalf("saida = %q, quero %q", out, want)
	}
	if !strings.Contains(gotIn, "sticker-input-") || !strings.HasSuffix(gotIn, ".mp4") {
		t.Fatalf("caminho de entrada inesperado: %q", gotIn)
	}
	if !strings.Contains(gotOut, "sticker-output-") || !strings.HasSuffix(gotOut, ".webp") {
		t.Fatalf("caminho de saida inesperado: %q", gotOut)
	}
	// Os arquivos temporarios sao removidos nos defers.
	if _, err := os.Stat(gotIn); !os.IsNotExist(err) {
		t.Fatalf("arquivo de entrada nao foi removido: %v", err)
	}
	if _, err := os.Stat(gotOut); !os.IsNotExist(err) {
		t.Fatalf("arquivo de saida nao foi removido: %v", err)
	}
}

func TestRunFFmpegConversionFalhaDoFFmpeg(t *testing.T) {
	withFailingFFmpeg(t)

	out, err := RunFFmpegConversion([]byte("entrada"), ".mp4", func(inPath, outPath string) []string {
		return []string{"-i", inPath, outPath}
	}, "ffmpeg falhou no teste")

	if err == nil {
		t.Fatal("quero erro quando o ffmpeg sai diferente de zero")
	}
	if out != nil {
		t.Fatalf("quero saida nil, tenho %d bytes", len(out))
	}
}

func TestRunFFmpegConversionFFmpegAusente(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := RunFFmpegConversion([]byte("entrada"), ".mp4", func(inPath, outPath string) []string {
		return []string{"-i", inPath, outPath}
	}, "ffmpeg nao encontrado")

	if err == nil {
		t.Fatal("quero erro quando o binario ffmpeg nao existe no PATH")
	}
}

func TestRunFFmpegConversionTemporariosJaRemovidos(t *testing.T) {
	// Remover os temporarios por baixo do defer exercita os ramos de erro de
	// limpeza: a conversao ainda falha, mas sem panico e sem mascarar o erro
	// original do ffmpeg.
	withFailingFFmpeg(t)

	_, err := RunFFmpegConversion([]byte("entrada"), ".mp4", func(inPath, outPath string) []string {
		if rmErr := os.Remove(inPath); rmErr != nil {
			t.Fatalf("remover entrada: %v", rmErr)
		}
		if rmErr := os.Remove(outPath); rmErr != nil {
			t.Fatalf("remover saida: %v", rmErr)
		}
		return []string{"-i", inPath, outPath}
	}, "ffmpeg falhou no teste")

	if err == nil {
		t.Fatal("quero o erro do ffmpeg mesmo com os temporarios ja removidos")
	}
}

func TestRunFFmpegConversionFalhaAoCriarTemporario(t *testing.T) {
	// Uma barra na extensao faz o padrao de os.CreateTemp conter separador,
	// que e' rejeitado antes de qualquer I/O.
	out, err := RunFFmpegConversion([]byte("entrada"), "/invalido", func(inPath, outPath string) []string {
		t.Fatal("ffmpegArgs nao deveria ser chamado")
		return nil
	}, "nao deveria chegar ao ffmpeg")

	if err == nil {
		t.Fatal("quero erro ao criar o arquivo temporario de entrada")
	}
	if out != nil {
		t.Fatalf("quero saida nil, tenho %d bytes", len(out))
	}
}

// ---------------------------------------------------------------------------
// ConvertVideoStickerToWebP / ConvertImageToWebP
// ---------------------------------------------------------------------------

func TestConvertVideoStickerToWebP(t *testing.T) {
	want := []byte("webp-de-video")
	withFakeFFmpeg(t, want)

	out, err := ConvertVideoStickerToWebP([]byte("mp4"))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !bytes.Equal(out, want) {
		t.Fatalf("saida = %q, quero %q", out, want)
	}
}

func TestConvertVideoStickerToWebPErro(t *testing.T) {
	withFailingFFmpeg(t)

	if _, err := ConvertVideoStickerToWebP([]byte("mp4")); err == nil {
		t.Fatal("quero erro quando o ffmpeg falha")
	}
}

func TestConvertImageToWebP(t *testing.T) {
	want := []byte("webp-de-imagem")
	withFakeFFmpeg(t, want)

	out, err := ConvertImageToWebP([]byte("png"))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !bytes.Equal(out, want) {
		t.Fatalf("saida = %q, quero %q", out, want)
	}
}

func TestConvertImageToWebPErro(t *testing.T) {
	withFailingFFmpeg(t)

	if _, err := ConvertImageToWebP([]byte("png")); err == nil {
		t.Fatal("quero erro quando o ffmpeg falha")
	}
}
