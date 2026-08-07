package socket

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

// TestGenerateIVShape trava o formato do nonce AES-GCM: 12 bytes, contador
// big-endian nos ultimos 4, zeros antes. Um nonce fora desse formato produz um
// texto cifrado que o servidor nao consegue abrir.
func TestGenerateIVShape(t *testing.T) {
	iv := generateIV(0x01020304)
	if len(iv) != gcmIVSize {
		t.Fatalf("len(iv) = %d, esperado %d", len(iv), gcmIVSize)
	}
	for i := 0; i < gcmIVCounterOffset; i++ {
		if iv[i] != 0 {
			t.Errorf("iv[%d] = %d, esperado 0", i, iv[i])
		}
	}
	if got := binary.BigEndian.Uint32(iv[gcmIVCounterOffset:]); got != 0x01020304 {
		t.Errorf("contador no nonce = %#x, esperado %#x", got, 0x01020304)
	}
}

// TestGenerateIVDistinctPerCounter: nonce repetido em AES-GCM quebra a
// confidencialidade, entao contadores distintos precisam gerar nonces
// distintos.
func TestGenerateIVDistinctPerCounter(t *testing.T) {
	if bytes.Equal(generateIV(0), generateIV(1)) {
		t.Fatal("generateIV(0) e generateIV(1) sao iguais")
	}
}

// TestStartHashesLongPattern: um pattern com tamanho diferente de noiseHashSize
// e' reduzido por SHA-256 antes de virar o hash inicial.
func TestStartHashesLongPattern(t *testing.T) {
	pattern := NoiseStartPattern + "_mais_longo_que_noiseHashSize"
	nh := NewNoiseHandshake()
	nh.Start(pattern, nil)
	want := sha256.Sum256([]byte(pattern))
	// Start chama Authenticate(nil), que re-hasheia o hash inicial.
	final := sha256.Sum256(want[:])
	if !bytes.Equal(nh.hash, final[:]) {
		t.Errorf("hash apos Start = %x, esperado %x", nh.hash, final)
	}
}

// TestStartUsesPatternVerbatimWhenExactSize: um pattern de exatamente
// noiseHashSize bytes vira o hash inicial sem passar por SHA-256.
func TestStartUsesPatternVerbatimWhenExactSize(t *testing.T) {
	pattern := string(bytes.Repeat([]byte("a"), noiseHashSize))
	nh := NewNoiseHandshake()
	nh.Start(pattern, nil)
	final := sha256.Sum256([]byte(pattern))
	if !bytes.Equal(nh.hash, final[:]) {
		t.Errorf("hash apos Start = %x, esperado %x", nh.hash, final)
	}
}

// TestNoiseStartPatternLength documenta por que o ramo verbatim existe: o
// pattern real do WhatsApp tem 32 bytes contando o padding de NULs.
func TestNoiseStartPatternLength(t *testing.T) {
	if len(NoiseStartPattern) != noiseHashSize {
		t.Fatalf("len(NoiseStartPattern) = %d, esperado %d", len(NoiseStartPattern), noiseHashSize)
	}
}

// TestEncryptDecryptRoundTrip: dois handshakes com o mesmo estado inicial
// precisam conseguir abrir o que o outro cifrou — e' o invariante que faz o
// handshake fechar.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	writer, reader := NewNoiseHandshake(), NewNoiseHandshake()
	header := []byte("cabecalho")
	writer.Start(NoiseStartPattern, header)
	reader.Start(NoiseStartPattern, header)

	plaintext := []byte("mensagem de handshake")
	ciphertext := writer.Encrypt(plaintext)
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("Encrypt devolveu o texto em claro")
	}

	got, err := reader.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt falhou: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("Decrypt = %q, esperado %q", got, plaintext)
	}
	if !bytes.Equal(writer.hash, reader.hash) {
		t.Error("hashes divergiram apos o round trip")
	}
}

// TestDecryptRejectsTamperedCiphertext: o hash corrente entra como dado
// autenticado, entao qualquer bit alterado tem que falhar a abertura.
func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	writer, reader := NewNoiseHandshake(), NewNoiseHandshake()
	writer.Start(NoiseStartPattern, nil)
	reader.Start(NoiseStartPattern, nil)

	ciphertext := writer.Encrypt([]byte("mensagem"))
	ciphertext[0] ^= 0xFF
	if _, err := reader.Decrypt(ciphertext); err == nil {
		t.Fatal("Decrypt aceitou um texto cifrado adulterado")
	}
}

// TestMixIntoKeyResetsCounter: cada mistura de chave reinicia o contador do
// nonce, senao os dois lados ficariam dessincronizados.
func TestMixIntoKeyResetsCounter(t *testing.T) {
	nh := NewNoiseHandshake()
	nh.Start(NoiseStartPattern, nil)
	nh.Encrypt([]byte("um"))
	nh.Encrypt([]byte("dois"))
	if nh.counter == 0 {
		t.Fatal("contador nao avancou apos dois Encrypt")
	}
	if err := nh.MixIntoKey([]byte("segredo compartilhado")); err != nil {
		t.Fatalf("MixIntoKey falhou: %v", err)
	}
	if nh.counter != 0 {
		t.Errorf("contador = %d apos MixIntoKey, esperado 0", nh.counter)
	}
}

// TestExtractAndExpandKeySizes: as duas chaves derivadas tem noiseHashSize
// bytes cada e sao distintas entre si.
func TestExtractAndExpandKeySizes(t *testing.T) {
	nh := NewNoiseHandshake()
	write, read, err := nh.extractAndExpand(bytes.Repeat([]byte("s"), noiseHashSize), []byte("dados"))
	if err != nil {
		t.Fatalf("extractAndExpand falhou: %v", err)
	}
	if len(write) != noiseHashSize || len(read) != noiseHashSize {
		t.Fatalf("tamanhos = (%d, %d), esperado (%d, %d)", len(write), len(read), noiseHashSize, noiseHashSize)
	}
	if bytes.Equal(write, read) {
		t.Error("chave de escrita e de leitura sao iguais")
	}
}
