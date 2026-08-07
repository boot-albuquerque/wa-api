package binary

import (
	"strings"
	"testing"

	"wa-api/internal/wa-noise/binary/token"
)

func TestValidateNibbleAcceptsOnlyItsAlphabet(t *testing.T) {
	for _, ok := range []string{"", "0", "9", "0123456789", "-", ".", "55-11.999", strings.Repeat("1", token.PackedMax)} {
		if !validateNibble(ok) {
			t.Errorf("validateNibble(%q) = false, esperado true", ok)
		}
	}
	for _, bad := range []string{"A", "a", "+", " ", "12:34", "1/2", strings.Repeat("1", token.PackedMax+1)} {
		if validateNibble(bad) {
			t.Errorf("validateNibble(%q) = true, esperado false", bad)
		}
	}
}

func TestValidateHexAcceptsOnlyItsAlphabet(t *testing.T) {
	for _, ok := range []string{"", "0", "F", "0123456789ABCDEF", strings.Repeat("A", token.PackedMax)} {
		if !validateHex(ok) {
			t.Errorf("validateHex(%q) = false, esperado true", ok)
		}
	}
	// Minusculas e G em diante ficam de fora: hex8 e' maiusculo. Se o
	// validador aceitasse "a", packHex entraria no ramo de panic.
	for _, bad := range []string{"a", "f", "G", "-", ".", strings.Repeat("A", token.PackedMax+1)} {
		if validateHex(bad) {
			t.Errorf("validateHex(%q) = true, esperado false", bad)
		}
	}
}

// packNibble e unpackNibble tem que ser inversas exatas sobre o alfabeto
// inteiro. E' o invariante que o corte de packing.go existe para proteger.
func TestNibblePackIsInverseOfUnpack(t *testing.T) {
	for _, char := range []byte("0123456789-.") {
		packed := packNibble(char)
		back, err := unpackNibble(packed)
		if err != nil {
			t.Fatalf("unpackNibble(packNibble(%q)) devolveu erro: %v", char, err)
		}
		if back != char {
			t.Errorf("round trip de %q: obtido %q", char, back)
		}
	}
}

func TestHexPackIsInverseOfUnpack(t *testing.T) {
	for _, char := range []byte("0123456789ABCDEF") {
		packed := packHex(char)
		back, err := unpackHex(packed)
		if err != nil {
			t.Fatalf("unpackHex(packHex(%q)) devolveu erro: %v", char, err)
		}
		if back != char {
			t.Errorf("round trip de %q: obtido %q", char, back)
		}
	}
}

func TestNibblePaddingMapsToZeroByte(t *testing.T) {
	if got := packNibble(0); got != nibblePadding {
		t.Errorf("packNibble(0) = %d, esperado %d", got, nibblePadding)
	}
	got, err := unpackNibble(nibblePadding)
	if err != nil || got != 0 {
		t.Errorf("unpackNibble(%d) = (%d, %v), esperado (0, nil)", nibblePadding, got, err)
	}
	// packHex mapeia o padding para o mesmo valor, mas unpackHex NAO o
	// reconhece: 15 cai no ramo 'A'+15-10 = 'F'. E' assimetrico no upstream
	// e nao foi mexido — o padding de hex8 e' descartado pelo bit de
	// tamanho impar antes de chegar aqui.
	if got := packHex(0); got != nibblePadding {
		t.Errorf("packHex(0) = %d, esperado %d", got, nibblePadding)
	}
	if hexPad, err := unpackHex(nibblePadding); err != nil || hexPad != 'F' {
		t.Errorf("unpackHex(%d) = (%q, %v), esperado ('F', nil)", nibblePadding, hexPad, err)
	}
}

func TestUnpackNibbleRejectsValuesOutsideAlphabet(t *testing.T) {
	for _, bad := range []byte{12, 13, 14, 16, 200} {
		if _, err := unpackNibble(bad); err == nil {
			t.Errorf("unpackNibble(%d) nao devolveu erro", bad)
		}
	}
}

func TestUnpackHexRejectsValuesOutsideAlphabet(t *testing.T) {
	for _, bad := range []byte{hexDigitCount, 20, 255} {
		if _, err := unpackHex(bad); err == nil {
			t.Errorf("unpackHex(%d) nao devolveu erro", bad)
		}
	}
}

func TestUnpackByteRoutesByTag(t *testing.T) {
	// O mesmo valor cru significa coisas diferentes em cada alfabeto: 10 e'
	// '-' em nibble8 e 'A' em hex8. E' o que unpackByte decide.
	if got, err := unpackByte(token.Nibble8, nibbleDash); err != nil || got != '-' {
		t.Errorf("unpackByte(Nibble8, %d) = (%q, %v), esperado ('-', nil)", nibbleDash, got, err)
	}
	if got, err := unpackByte(token.Hex8, decimalDigitCount); err != nil || got != 'A' {
		t.Errorf("unpackByte(Hex8, %d) = (%q, %v), esperado ('A', nil)", decimalDigitCount, got, err)
	}
	if _, err := unpackByte(token.Binary8, 0); err == nil {
		t.Error("unpackByte com tag que nao e' empacotada deveria falhar")
	}
}

func TestPackNibblePanicsOnUnvalidatedInput(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("packNibble('A') deveria entrar em panic")
		}
	}()
	packNibble('A')
}

func TestPackHexPanicsOnUnvalidatedInput(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("packHex('z') deveria entrar em panic")
		}
	}()
	packHex('z')
}

// O caminho completo: uma string do alfabeto nibble8/hex8 vira metade dos
// bytes e volta identica, com e sem o nibble de padding.
func TestPackedStringsSurviveRoundTrip(t *testing.T) {
	for _, value := range []string{"1", "12", "123", "5511999999999", "55-11.9", "ABCDEF", "ABCDE", "0F"} {
		back := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"v": value}})
		if got := back.Attrs["v"]; got != value {
			t.Errorf("round trip de %q devolveu %v", value, got)
		}
	}
}
