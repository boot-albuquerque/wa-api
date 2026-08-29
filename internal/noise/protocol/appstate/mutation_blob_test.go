package appstate

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// splitValueMAC exige IV + MAC, nao so' MAC: quem chama corta
// content[:cbcIVLength] logo depois, entao validar apenas o MAC apenas mudaria
// o panic de lugar. Este teste trava exatamente essa fronteira.
func TestSplitValueMACExigeIVEMac(t *testing.T) {
	minimo := macLength + cbcIVLength

	for _, size := range []int{0, 1, macLength - 1, macLength, minimo - 1} {
		blob := fillBytes(size, 0x5A)
		content, mac, err := splitValueMAC(blob, "blob de teste")
		if !errors.Is(err, ErrShortMutationBlob) {
			t.Errorf("size=%d: err = %v, esperava ErrShortMutationBlob", size, err)
		}
		if content != nil || mac != nil {
			t.Errorf("size=%d: esperava (nil, nil), veio (%x, %x)", size, content, mac)
		}
		if err != nil && !strings.Contains(err.Error(), "blob de teste") {
			t.Errorf("size=%d: a mensagem deveria nomear a origem: %v", size, err)
		}
	}

	// No tamanho minimo exato o corte e' valido e o conteudo e' so' o IV.
	blob := fillBytes(minimo, 0x5A)
	content, mac, err := splitValueMAC(blob, "blob de teste")
	if err != nil {
		t.Fatalf("tamanho minimo deveria passar: %v", err)
	}
	if len(content) != cbcIVLength {
		t.Errorf("content tem %d bytes, esperava %d", len(content), cbcIVLength)
	}
	if !bytes.Equal(mac, blob[len(blob)-macLength:]) {
		t.Error("o MAC devolvido nao e' a cauda do blob")
	}
}

func TestTrailingValueMACExigeApenasOMac(t *testing.T) {
	for _, size := range []int{0, 1, macLength - 1} {
		mac, err := trailingValueMAC(fillBytes(size, 1), "blob de teste")
		if !errors.Is(err, ErrShortMutationBlob) {
			t.Errorf("size=%d: err = %v, esperava ErrShortMutationBlob", size, err)
		}
		if mac != nil {
			t.Errorf("size=%d: mac = %x, esperava nil", size, mac)
		}
	}

	// Ao contrario de splitValueMAC, aqui macLength cru ja' basta: os
	// chamadores (updateHash, generatePatchMAC) so' querem a cauda.
	blob := fillBytes(macLength, 7)
	mac, err := trailingValueMAC(blob, "blob de teste")
	if err != nil {
		t.Fatalf("blob de exatamente macLength deveria passar: %v", err)
	}
	if !bytes.Equal(mac, blob) {
		t.Error("com o tamanho exato, o MAC e' o blob inteiro")
	}
}
