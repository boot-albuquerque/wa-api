package socket

import (
	"testing"
)

// TestEncodeDecodeFrameLengthRoundTrip cobre o prefixo de 24 bits big-endian
// que abre todo frame do protocolo: encode seguido de decode tem que devolver o
// valor original para toda a faixa representavel.
func TestEncodeDecodeFrameLengthRoundTrip(t *testing.T) {
	lengths := []int{
		0,
		1,
		0xFF,
		0x100,
		0xFFFF,
		0x10000,
		0xABCDEF,
		FrameMaxSize - 1,
	}
	for _, length := range lengths {
		buf := make([]byte, FrameLengthSize)
		encodeFrameLength(buf, length)
		if got := decodeFrameLength(buf); got != length {
			t.Errorf("round trip de %d devolveu %d (bytes %v)", length, got, buf)
		}
	}
}

// TestEncodeFrameLengthByteOrder trava a ordem dos bytes: um decoder do outro
// lado (o servidor do WhatsApp) le big-endian, entao inverter a ordem quebraria
// a conexao sem quebrar o round trip do teste anterior.
func TestEncodeFrameLengthByteOrder(t *testing.T) {
	buf := make([]byte, FrameLengthSize)
	encodeFrameLength(buf, 0x010203)
	want := []byte{0x01, 0x02, 0x03}
	for i := range want {
		if buf[i] != want[i] {
			t.Fatalf("encodeFrameLength(0x010203) = %v, esperado %v", buf, want)
		}
	}
}

// TestDecodeFrameLengthIgnoresTrailingBytes garante que o decoder le apenas o
// cabecalho: no read pump ele recebe a mensagem inteira (cabecalho + payload +
// possivelmente o proximo frame) e nao pode considerar nada alem dos primeiros
// FrameLengthSize bytes.
func TestDecodeFrameLengthIgnoresTrailingBytes(t *testing.T) {
	src := []byte{0x00, 0x00, 0x04, 'p', 'a', 'y', 'l', 0xFF, 0xFF}
	if got := decodeFrameLength(src); got != 4 {
		t.Fatalf("decodeFrameLength = %d, esperado 4", got)
	}
}

// TestFrameLengthSizeMatchesMaxSize documenta a relacao entre as duas
// constantes: FrameMaxSize e' exatamente o primeiro comprimento que o prefixo
// de FrameLengthSize bytes nao consegue representar.
func TestFrameLengthSizeMatchesMaxSize(t *testing.T) {
	if want := 1 << (FrameLengthSize * 8); FrameMaxSize != want {
		t.Fatalf("FrameMaxSize = %d, esperado %d para um prefixo de %d bytes", FrameMaxSize, want, FrameLengthSize)
	}
}
