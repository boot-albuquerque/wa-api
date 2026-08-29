package binary

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"testing"
)

func zlibFrame(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteByte(zlibCompressedFlag)
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUnpackStripsFlagByteWhenUncompressed(t *testing.T) {
	got, err := Unpack([]byte{0x00, 'o', 'i'})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("oi")) {
		t.Errorf("= %q, esperado \"oi\"", got)
	}
}

func TestUnpackDecompressesWhenFlagIsSet(t *testing.T) {
	payload := bytes.Repeat([]byte("dados repetidos "), 100)
	frame := zlibFrame(t, payload)
	if len(frame) >= len(payload) {
		t.Fatalf("o frame comprimido (%d) nao ficou menor que o payload (%d)", len(frame), len(payload))
	}
	got, err := Unpack(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload nao sobreviveu a descompressao")
	}
}

// O flag e' um BIT, nao o byte inteiro: qualquer byte com o bit 1 ligado pede
// descompressao. Trocar o teste de bit por igualdade quebraria os frames em
// que o servidor liga outros bits junto.
func TestUnpackTestsTheBitNotTheWholeByte(t *testing.T) {
	payload := []byte("conteudo comprimido")
	frame := zlibFrame(t, payload)
	frame[0] = zlibCompressedFlag | 0x01

	got, err := Unpack(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("= %q, esperado %q", got, payload)
	}

	// E um byte sem o bit 1 nao descomprime, mesmo com outros bits ligados.
	raw, err := Unpack([]byte{0x01, 'o', 'i'})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, []byte("oi")) {
		t.Errorf("= %q, esperado \"oi\" sem descompressao", raw)
	}
}

func TestUnpackReportsBrokenZlibStream(t *testing.T) {
	if _, err := Unpack([]byte{zlibCompressedFlag, 0xFF, 0xFF, 0xFF}); err == nil {
		t.Error("cabecalho zlib invalido deveria falhar")
	}
	// Cabecalho valido, corpo truncado: o erro vem do io.ReadAll, nao do
	// NewReader, e o codigo tem que propagar os dois.
	frame := zlibFrame(t, bytes.Repeat([]byte("x"), 500))
	if _, err := Unpack(frame[:len(frame)-5]); err == nil {
		t.Error("stream zlib truncado deveria falhar")
	}
}

// ACHADO (HOUSEKEEP F25): Unpack indexa data[0] sem checar o tamanho, entao um
// frame VAZIO vindo da rede entra em panic em vez de devolver erro. O teste
// trava o comportamento atual, como o F24 em decoder_node_test.go.
// Unpack roda sobre o payload decifrado do socket, antes de qualquer parsing:
// e' o ponto mais raso exposto a dado nao confiavel. Um frame de zero bytes
// dava panic no data[0] ate' a correcao da F25.
func TestUnpackRejeitaFrameVazioSemPanic(t *testing.T) {
	for name, data := range map[string][]byte{"nil": nil, "vazio": {}} {
		t.Run(name, func(t *testing.T) {
			out, err := Unpack(data)
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("err = %v, esperava io.ErrUnexpectedEOF", err)
			}
			if out != nil {
				t.Errorf("out = %v, esperava nil", out)
			}
		})
	}
}
