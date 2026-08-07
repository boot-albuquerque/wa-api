package binary

import (
	"errors"
	"io"
	"strings"
	"testing"

	"wa-api/internal/wa-noise/protocol/binary/token"
)

func TestCheckEOSGuardsEveryRead(t *testing.T) {
	r := newDecoder([]byte{1, 2, 3})
	if err := r.checkEOS(3); err != nil {
		t.Errorf("checkEOS(3) em buffer de 3 bytes: %v", err)
	}
	if err := r.checkEOS(4); !errors.Is(err, io.EOF) {
		t.Errorf("checkEOS(4) em buffer de 3 bytes = %v, esperado io.EOF", err)
	}
	// checkEOS e' relativo ao indice atual, nao ao inicio do buffer.
	r.index = 2
	if err := r.checkEOS(2); !errors.Is(err, io.EOF) {
		t.Errorf("checkEOS(2) com indice em 2 = %v, esperado io.EOF", err)
	}
}

func TestReadByteAdvancesTheCursor(t *testing.T) {
	r := newDecoder([]byte{0xAA, 0xBB})
	for i, want := range []byte{0xAA, 0xBB} {
		got, err := r.readByte()
		if err != nil || got != want {
			t.Fatalf("leitura %d = (%#x, %v), esperado (%#x, nil)", i, got, err, want)
		}
	}
	if _, err := r.readByte(); !errors.Is(err, io.EOF) {
		t.Errorf("leitura apos o fim = %v, esperado io.EOF", err)
	}
}

// readIntN monta o inteiro nas duas ordens de byte. O formato usa big endian
// em tudo, mas o parametro existe e os dois ramos tem que estar certos.
func TestReadIntNHonoursByteOrder(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}

	big, err := newDecoder(data).readIntN(4, false)
	if err != nil || big != 0x01020304 {
		t.Errorf("big endian = (%#x, %v), esperado 0x01020304", big, err)
	}
	little, err := newDecoder(data).readIntN(4, true)
	if err != nil || little != 0x04030201 {
		t.Errorf("little endian = (%#x, %v), esperado 0x04030201", little, err)
	}
	if _, err := newDecoder(data).readIntN(5, false); !errors.Is(err, io.EOF) {
		t.Errorf("readIntN alem do buffer = %v, esperado io.EOF", err)
	}
}

func TestReadIntWidths(t *testing.T) {
	data := []byte{0xFF, 0xFF, 0xFF, 0xFF}

	if got, _ := newDecoder(data).readInt8(false); got != 0xFF {
		t.Errorf("readInt8 = %#x", got)
	}
	if got, _ := newDecoder(data).readInt16(false); got != 0xFFFF {
		t.Errorf("readInt16 = %#x", got)
	}
	if got, _ := newDecoder(data).readInt32(false); got != 0xFFFFFFFF {
		t.Errorf("readInt32 = %#x", got)
	}
}

// int20 usa so' os 4 bits baixos do primeiro byte. Os 4 altos sao ignorados —
// e' o que int20HighNibbleMask garante. Sem a mascara, um byte com bits altos
// sujos produziria um tamanho gigante e a leitura estouraria o buffer.
func TestReadInt20MasksTheHighNibble(t *testing.T) {
	got, err := newDecoder([]byte{0x0F, 0xFF, 0xFF}).readInt20()
	if err != nil || got != 0xFFFFF {
		t.Errorf("readInt20 do valor maximo = (%#x, %v), esperado 0xFFFFF", got, err)
	}
	dirty, err := newDecoder([]byte{0xFF, 0xFF, 0xFF}).readInt20()
	if err != nil || dirty != 0xFFFFF {
		t.Errorf("readInt20 com nibble alto sujo = (%#x, %v), esperado 0xFFFFF", dirty, err)
	}
	if _, err := newDecoder([]byte{0x00, 0x00}).readInt20(); !errors.Is(err, io.EOF) {
		t.Errorf("readInt20 com 2 bytes = %v, esperado io.EOF", err)
	}
}

func TestReadInt20ConsumesExactlyThreeBytes(t *testing.T) {
	r := newDecoder([]byte{0x00, 0x00, 0x01, 0x42})
	if _, err := r.readInt20(); err != nil {
		t.Fatal(err)
	}
	if r.index != int20Size {
		t.Errorf("indice apos readInt20 = %d, esperado %d", r.index, int20Size)
	}
	if next, _ := r.readByte(); next != 0x42 {
		t.Errorf("byte seguinte = %#x, esperado 0x42", next)
	}
}

func TestReadListSizeByTag(t *testing.T) {
	if got, err := newDecoder(nil).readListSize(token.ListEmpty); err != nil || got != 0 {
		t.Errorf("ListEmpty = (%d, %v), esperado (0, nil)", got, err)
	}
	if got, err := newDecoder([]byte{0x07}).readListSize(token.List8); err != nil || got != 7 {
		t.Errorf("List8 = (%d, %v), esperado (7, nil)", got, err)
	}
	if got, err := newDecoder([]byte{0x01, 0x00}).readListSize(token.List16); err != nil || got != 256 {
		t.Errorf("List16 = (%d, %v), esperado (256, nil)", got, err)
	}
	if _, err := newDecoder(nil).readListSize(token.Binary8); err == nil {
		t.Error("readListSize com tag que nao e' de lista deveria falhar")
	}
}

func TestReadRawSlicesWithoutCopying(t *testing.T) {
	r := newDecoder([]byte{1, 2, 3, 4, 5})
	got, err := r.readRaw(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Errorf("readRaw(3) = %v", got)
	}
	if r.index != 3 {
		t.Errorf("indice = %d, esperado 3", r.index)
	}
	if _, err := r.readRaw(3); !errors.Is(err, io.EOF) {
		t.Errorf("readRaw alem do buffer = %v, esperado io.EOF", err)
	}
}

func TestReadBytesOrStringSwitchesRepresentation(t *testing.T) {
	asString, err := newDecoder([]byte("oi")).readBytesOrString(2, true)
	if err != nil || asString != "oi" {
		t.Errorf("asString = (%v, %v), esperado (\"oi\", nil)", asString, err)
	}
	asBytes, err := newDecoder([]byte("oi")).readBytesOrString(2, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := asBytes.([]byte); !ok {
		t.Errorf("asBytes voltou como %T, esperado []byte", asBytes)
	}
	if _, err := newDecoder([]byte("oi")).readBytesOrString(3, true); !errors.Is(err, io.EOF) {
		t.Errorf("leitura alem do buffer = %v, esperado io.EOF", err)
	}
}

// readPacked8 le' o byte de tamanho, e o bit alto desse byte diz que o ultimo
// caractere e' padding. Sem esse descarte, todo numero de telefone com um
// numero impar de digitos voltaria com um caractere a mais.
func TestReadPacked8DropsPaddingOnOddLength(t *testing.T) {
	// 0x82 = 2 bytes empacotados com o flag de tamanho impar ligado.
	// 0x12 0x34 = '1','2','3','4'; o '4' e' o padding descartado.
	got, err := newDecoder([]byte{0x82, 0x12, 0x34}).readPacked8(token.Nibble8)
	if err != nil || got != "123" {
		t.Errorf("com flag de impar = (%q, %v), esperado (\"123\", nil)", got, err)
	}
	// O mesmo payload sem o flag mantem os 4 caracteres.
	got, err = newDecoder([]byte{0x02, 0x12, 0x34}).readPacked8(token.Nibble8)
	if err != nil || got != "1234" {
		t.Errorf("sem flag de impar = (%q, %v), esperado (\"1234\", nil)", got, err)
	}
}

func TestReadPacked8SplitsEachByteIntoTwoChars(t *testing.T) {
	got, err := newDecoder([]byte{0x02, 0x01, 0x23}).readPacked8(token.Nibble8)
	if err != nil || got != "0123" {
		t.Errorf("= (%q, %v), esperado (\"0123\", nil)", got, err)
	}
	got, err = newDecoder([]byte{0x02, 0xAB, 0xCD}).readPacked8(token.Hex8)
	if err != nil || got != "ABCD" {
		t.Errorf("hex8 = (%q, %v), esperado (\"ABCD\", nil)", got, err)
	}
}

func TestReadPacked8PropagatesErrors(t *testing.T) {
	if _, err := newDecoder(nil).readPacked8(token.Nibble8); !errors.Is(err, io.EOF) {
		t.Errorf("sem byte de tamanho = %v, esperado io.EOF", err)
	}
	if _, err := newDecoder([]byte{0x02, 0x01}).readPacked8(token.Nibble8); !errors.Is(err, io.EOF) {
		t.Errorf("payload truncado = %v, esperado io.EOF", err)
	}
	// 0xC no nibble alto nao pertence ao alfabeto nibble8.
	if _, err := newDecoder([]byte{0x01, 0xC0}).readPacked8(token.Nibble8); err == nil {
		t.Error("nibble alto invalido deveria falhar")
	}
	if _, err := newDecoder([]byte{0x01, 0x0C}).readPacked8(token.Nibble8); err == nil {
		t.Error("nibble baixo invalido deveria falhar")
	}
	if _, err := newDecoder([]byte{0x01, 0x00}).readPacked8(token.Binary8); err == nil {
		t.Error("tag que nao e' empacotada deveria falhar")
	}
}

// packedLengthMask isola o tamanho do bit de paridade. Um tamanho no maximo
// (PackedMax) com o flag ligado nao pode ser lido como 255 bytes.
func TestReadPacked8MasksLengthFromParityBit(t *testing.T) {
	value := strings.Repeat("1", token.PackedMax)
	back := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"v": value}})
	if got := back.Attrs["v"]; got != value {
		t.Errorf("string de %d caracteres nao sobreviveu: %v", token.PackedMax, got)
	}
}
