package binary

import (
	"bytes"
	"strings"
	"testing"

	"wa-api/internal/wa-noise/protocol/binary/token"
)

// O primeiro byte de todo frame e' o flag de compressao, que Unpack le' na
// ponta oposta. Marshal nunca comprime, entao ele e' sempre 0. Se deixar de
// ser, Unpack tentaria descomprimir um frame cru.
func TestEncoderStartsWithUncompressedFlag(t *testing.T) {
	data := mustMarshal(t, Node{Tag: "x"})
	if data[0] != 0 {
		t.Errorf("primeiro byte = %#x, esperado 0 (nao comprimido)", data[0])
	}
	if _, err := Unpack(data); err != nil {
		t.Errorf("Unpack do proprio frame do encoder falhou: %v", err)
	}
}

func TestPushIntNHonoursByteOrder(t *testing.T) {
	w := newEncoder()
	w.pushIntN(0x01020304, 4, false)
	if got := w.getData()[1:]; !bytes.Equal(got, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Errorf("big endian = %x", got)
	}
	w = newEncoder()
	w.pushIntN(0x01020304, 4, true)
	if got := w.getData()[1:]; !bytes.Equal(got, []byte{0x04, 0x03, 0x02, 0x01}) {
		t.Errorf("little endian = %x", got)
	}
}

func TestPushIntWidthsAndRoundTripThroughDecoder(t *testing.T) {
	tests := []struct {
		name  string
		push  func(*binaryEncoder, int)
		read  func(*binaryDecoder) (int, error)
		value int
		size  int
	}{
		{"int8", (*binaryEncoder).pushInt8, func(r *binaryDecoder) (int, error) { return r.readInt8(false) }, 0xFF, int8Size},
		{"int16", (*binaryEncoder).pushInt16, func(r *binaryDecoder) (int, error) { return r.readInt16(false) }, 0xFFFF, int16Size},
		{"int20", (*binaryEncoder).pushInt20, func(r *binaryDecoder) (int, error) { return r.readInt20() }, 0xFFFFF, int20Size},
		{"int32", (*binaryEncoder).pushInt32, func(r *binaryDecoder) (int, error) { return r.readInt32(false) }, 0x7FFFFFFF, int32Size},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newEncoder()
			tc.push(w, tc.value)
			raw := w.getData()[1:]
			if len(raw) != tc.size {
				t.Errorf("escreveu %d bytes, esperado %d", len(raw), tc.size)
			}
			got, err := tc.read(newDecoder(raw))
			if err != nil || got != tc.value {
				t.Errorf("round trip = (%#x, %v), esperado %#x", got, err, tc.value)
			}
		})
	}
}

// pushInt20 tem que mascarar o nibble alto. Sem a mascara, um valor com bits
// acima de 20 vazaria para o byte de tag do proximo campo.
func TestPushInt20MasksTheHighNibble(t *testing.T) {
	w := newEncoder()
	w.pushInt20(0xFFFFFFF)
	if got := w.getData()[1]; got != 0x0F {
		t.Errorf("primeiro byte = %#x, esperado 0x0F", got)
	}
}

// writeByteLength escolhe a tag pelo tamanho. As fronteiras sao onde um erro
// de <= vs < produziria um frame que o servidor rejeita.
func TestWriteByteLengthPicksTagByBoundary(t *testing.T) {
	tests := []struct {
		length int
		tag    byte
		size   int
	}{
		{0, token.Binary8, int8Size},
		{token.SingleByteMax - 1, token.Binary8, int8Size},
		{token.SingleByteMax, token.Binary20, int20Size},
		{int20Max - 1, token.Binary20, int20Size},
		{int20Max, token.Binary32, int32Size},
	}
	for _, tc := range tests {
		w := newEncoder()
		w.writeByteLength(tc.length)
		raw := w.getData()[1:]
		if raw[0] != tc.tag {
			t.Errorf("tamanho %d: tag = %d, esperado %d", tc.length, raw[0], tc.tag)
		}
		if len(raw) != 1+tc.size {
			t.Errorf("tamanho %d: escreveu %d bytes de tamanho, esperado %d", tc.length, len(raw)-1, tc.size)
		}
	}
}

// As tres larguras de bloco tem que sobreviver ao round trip com o tamanho
// exato — e' o que prova que a tag escolhida e o tamanho escrito concordam.
func TestBinaryBlockLengthsRoundTrip(t *testing.T) {
	for _, size := range []int{0, 1, token.SingleByteMax - 1, token.SingleByteMax, int20Max - 1, int20Max} {
		back := mustRoundTrip(t, Node{Tag: "x", Content: make([]byte, size)})
		got, ok := back.Content.([]byte)
		if size == 0 {
			// Conteudo vazio nao e' distinguivel de conteudo ausente:
			// o encoder escreve um bloco de tamanho 0 e o decoder o
			// devolve como slice vazio.
			if ok && len(got) != 0 {
				t.Errorf("tamanho 0 voltou com %d bytes", len(got))
			}
			continue
		}
		if !ok {
			t.Fatalf("tamanho %d: conteudo voltou como %T", size, back.Content)
		}
		if len(got) != size {
			t.Errorf("tamanho %d voltou com %d bytes", size, len(got))
		}
	}
}

func TestWriteListStartPicksTagByBoundary(t *testing.T) {
	tests := []struct {
		size int
		tag  byte
		len  int
	}{
		{0, token.ListEmpty, 1},
		{1, token.List8, 2},
		{token.SingleByteMax - 1, token.List8, 2},
		{token.SingleByteMax, token.List16, 3},
	}
	for _, tc := range tests {
		w := newEncoder()
		w.writeListStart(tc.size)
		raw := w.getData()[1:]
		if raw[0] != tc.tag {
			t.Errorf("lista de %d: tag = %d, esperado %d", tc.size, raw[0], tc.tag)
		}
		if len(raw) != tc.len {
			t.Errorf("lista de %d: escreveu %d bytes, esperado %d", tc.size, len(raw), tc.len)
		}
	}
}

func TestWriteBytesAlwaysUsesRawBlock(t *testing.T) {
	// Uma sequencia de bytes que POR ACASO e' um token conhecido nao pode
	// virar token: writeBytes e' para dados binarios, e o decoder que le'
	// um token de volta devolveria string, nao []byte.
	w := newEncoder()
	w.writeBytes([]byte("type"))
	if got := w.getData()[1]; got != token.Binary8 {
		t.Errorf("tag = %d, esperado Binary8 (%d)", got, token.Binary8)
	}
}

func TestWriteStringRawWritesLengthThenBytes(t *testing.T) {
	w := newEncoder()
	w.writeStringRaw("oi")
	if got := w.getData()[1:]; !bytes.Equal(got, []byte{token.Binary8, 0x02, 'o', 'i'}) {
		t.Errorf("= %x", got)
	}
}

// writePackedBytes e readPacked8 tem que concordar sobre o bit de paridade:
// tamanho par nao liga o flag, tamanho impar liga.
func TestWritePackedBytesSetsParityFlagOnOddLength(t *testing.T) {
	w := newEncoder()
	w.writePackedBytes("1234", token.Nibble8)
	if got := w.getData()[2]; got&packedOddLengthFlag != 0 {
		t.Errorf("tamanho par ligou o flag de impar: %#x", got)
	}
	w = newEncoder()
	w.writePackedBytes("123", token.Nibble8)
	if got := w.getData()[2]; got&packedOddLengthFlag == 0 {
		t.Errorf("tamanho impar nao ligou o flag: %#x", got)
	}
}

func TestWritePackedBytesHalvesTheLength(t *testing.T) {
	// 6 caracteres viram 3 bytes, mais a tag e o byte de tamanho.
	w := newEncoder()
	w.writePackedBytes("123456", token.Nibble8)
	if got := len(w.getData()[1:]); got != 5 {
		t.Errorf("6 caracteres produziram %d bytes, esperado 5", got)
	}
	// 5 caracteres tambem viram 3 bytes (o ultimo com padding).
	w = newEncoder()
	w.writePackedBytes("12345", token.Nibble8)
	if got := len(w.getData()[1:]); got != 5 {
		t.Errorf("5 caracteres produziram %d bytes, esperado 5", got)
	}
}

func TestWritePackedBytesPanicsAbovePackedMax(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("string acima de PackedMax deveria entrar em panic")
		}
	}()
	w := newEncoder()
	w.writePackedBytes(strings.Repeat("1", token.PackedMax+1), token.Nibble8)
}

func TestWritePackedBytesPanicsOnUnknownDataType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("tipo que nao e' nibble8/hex8 deveria entrar em panic")
		}
	}()
	w := newEncoder()
	w.writePackedBytes("1", token.Binary8)
}

// Uma string acima de PackedMax nao pode ser empacotada, mas TAMBEM nao pode
// entrar em panic: validateNibble/validateHex barram o tamanho antes, e
// writeString cai no bloco cru. E' o par que impede o panic acima de ser
// alcancavel pelo caminho normal.
func TestLongNumericStringFallsBackToRawBlock(t *testing.T) {
	value := strings.Repeat("1", token.PackedMax+1)
	back := mustRoundTrip(t, Node{Tag: "x", Attrs: Attrs{"v": value}})
	if got := back.Attrs["v"]; got != value {
		t.Errorf("string de %d digitos nao sobreviveu ao round trip", len(value))
	}
}

func TestPackBytePairPacksHighThenLow(t *testing.T) {
	w := newEncoder()
	if got := w.packBytePair(packNibble, '1', '2'); got != 0x12 {
		t.Errorf("packBytePair('1','2') = %#x, esperado 0x12", got)
	}
	if got := w.packBytePair(packHex, 'A', 'F'); got != 0xAF {
		t.Errorf("packBytePair('A','F') = %#x, esperado 0xAF", got)
	}
}
