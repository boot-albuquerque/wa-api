package media

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileToBase64(t *testing.T) {
	// Um PNG 1x1 real: prova que o MIME vem do sniffing do conteudo e nao da
	// extensao do arquivo, que aqui e' deliberadamente ".bin".
	png1x1 := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89,
	}
	path := filepath.Join(t.TempDir(), "fixture.bin")
	if err := os.WriteFile(path, png1x1, 0o600); err != nil {
		t.Fatalf("escrever fixture: %v", err)
	}

	encoded, mimeType, err := FileToBase64(path)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if mimeType != "image/png" {
		t.Fatalf("mimeType = %q, quero image/png", mimeType)
	}
	if encoded != base64.StdEncoding.EncodeToString(png1x1) {
		t.Fatalf("base64 = %q", encoded)
	}
}

func TestFileToBase64ArquivoInexistente(t *testing.T) {
	encoded, mimeType, err := FileToBase64(filepath.Join(t.TempDir(), "nao-existe"))

	if err == nil {
		t.Fatal("quero erro para arquivo inexistente")
	}
	if !strings.Contains(err.Error(), "no such file") && !os.IsNotExist(err) {
		t.Fatalf("erro = %v, quero ENOENT", err)
	}
	if encoded != "" || mimeType != "" {
		t.Fatalf("quero retornos vazios, tenho (%q, %q)", encoded, mimeType)
	}
}
