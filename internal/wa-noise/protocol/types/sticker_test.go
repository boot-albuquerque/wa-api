package types

import (
	"bytes"
	"testing"
)

// Os getters de StickerPackItem existem para satisfazer a interface de
// download de midia, que o pacote de download consome. Os NOMES nao batem com
// os campos (GetFileSHA256 le' FileHash, GetFileEncSHA256 le' EncFileHash), e
// e' exatamente por isso que ha teste: trocar os dois no corpo do getter
// passaria despercebido e faria o download validar o hash errado.
func TestStickerPackItemGettersMapToTheRightFields(t *testing.T) {
	item := &StickerPackItem{
		DirectPath:  "/v/t62.1",
		MediaKey:    []byte("chave"),
		FileHash:    []byte("hash-do-claro"),
		EncFileHash: []byte("hash-do-cifrado"),
		FileSize:    4096,
	}

	if got := item.GetDirectPath(); got != item.DirectPath {
		t.Errorf("GetDirectPath = %q", got)
	}
	if got := item.GetMediaKey(); !bytes.Equal(got, item.MediaKey) {
		t.Errorf("GetMediaKey = %q", got)
	}
	if got := item.GetFileSHA256(); !bytes.Equal(got, item.FileHash) {
		t.Errorf("GetFileSHA256 = %q, esperado FileHash", got)
	}
	if got := item.GetFileEncSHA256(); !bytes.Equal(got, item.EncFileHash) {
		t.Errorf("GetFileEncSHA256 = %q, esperado EncFileHash", got)
	}
	if got := item.GetFileSizeBytes(); got != item.FileSize {
		t.Errorf("GetFileSizeBytes = %d", got)
	}
	// E os dois hashes nao podem vir do mesmo campo.
	if bytes.Equal(item.GetFileSHA256(), item.GetFileEncSHA256()) {
		t.Error("os dois getters de hash devolveram o mesmo campo")
	}
}

func TestStickerPackItemZeroValueGetters(t *testing.T) {
	var item StickerPackItem
	if item.GetDirectPath() != "" || item.GetMediaKey() != nil || item.GetFileSizeBytes() != 0 {
		t.Error("os getters do valor zero deveriam devolver zeros")
	}
}
