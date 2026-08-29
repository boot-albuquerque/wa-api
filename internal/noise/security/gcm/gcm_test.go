package gcmutil

import (
	"bytes"
	"testing"

	"go.mau.fi/util/random"
)

func gcmIV(t *testing.T) []byte {
	t.Helper()
	// 12 bytes e' o nonce padrao do GCM, que e' o que cipher.NewGCM cria.
	return random.Bytes(12)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, iv := random.Bytes(32), gcmIV(t)
	for _, size := range []int{0, 1, 16, 1000} {
		plaintext := random.Bytes(size)
		ciphertext, err := Encrypt(key, iv, plaintext, nil)
		if err != nil {
			t.Fatalf("tamanho %d: %v", size, err)
		}
		got, err := Decrypt(key, iv, ciphertext, nil)
		if err != nil {
			t.Fatalf("tamanho %d: %v", size, err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Errorf("tamanho %d nao sobreviveu ao round trip", size)
		}
	}
}

// GCM autentica: qualquer bit trocado no ciphertext tem que fazer o Open
// FALHAR, nao devolver lixo. E' a diferenca entre GCM e CBC e a razao de este
// pacote existir separado do cbcutil.
func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	key, iv := random.Bytes(32), gcmIV(t)
	ciphertext, err := Encrypt(key, iv, []byte("mensagem"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range ciphertext {
		tampered := append([]byte(nil), ciphertext...)
		tampered[i] ^= 0x01
		if _, err := Decrypt(key, iv, tampered, nil); err == nil {
			t.Errorf("byte %d alterado passou na autenticacao", i)
		}
	}
}

// Os dados adicionais entram na autenticacao mas nao no ciphertext. Decriptar
// com AD diferente tem que falhar — e' o que amarra o payload ao seu contexto.
func TestAdditionalDataIsAuthenticated(t *testing.T) {
	key, iv := random.Bytes(32), gcmIV(t)
	plaintext := []byte("mensagem")

	ciphertext, err := Encrypt(key, iv, plaintext, []byte("contexto-a"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(key, iv, ciphertext, []byte("contexto-b")); err == nil {
		t.Error("dados adicionais diferentes passaram na autenticacao")
	}
	if _, err := Decrypt(key, iv, ciphertext, nil); err == nil {
		t.Error("ausencia de dados adicionais passou na autenticacao")
	}
	got, err := Decrypt(key, iv, ciphertext, []byte("contexto-a"))
	if err != nil || !bytes.Equal(got, plaintext) {
		t.Errorf("= (%q, %v)", got, err)
	}
}

func TestDecryptRejectsWrongKeyAndIV(t *testing.T) {
	key, iv := random.Bytes(32), gcmIV(t)
	ciphertext, err := Encrypt(key, iv, []byte("mensagem"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(random.Bytes(32), iv, ciphertext, nil); err == nil {
		t.Error("chave errada passou")
	}
	if _, err := Decrypt(key, gcmIV(t), ciphertext, nil); err == nil {
		t.Error("IV errado passou")
	}
}

func TestPrepareRejectsInvalidKeyLength(t *testing.T) {
	// AES aceita 16, 24 e 32 bytes; qualquer outro tamanho falha na criacao
	// do cipher, antes do GCM.
	for _, size := range []int{0, 15, 17, 31, 33} {
		if _, err := Prepare(random.Bytes(size)); err == nil {
			t.Errorf("chave de %d bytes deveria falhar", size)
		}
	}
	for _, size := range []int{16, 24, 32} {
		if _, err := Prepare(random.Bytes(size)); err != nil {
			t.Errorf("chave de %d bytes deveria ser aceita: %v", size, err)
		}
	}
}

func TestEncryptAndDecryptPropagateKeyErrors(t *testing.T) {
	if _, err := Encrypt(random.Bytes(15), gcmIV(t), nil, nil); err == nil {
		t.Error("Encrypt com chave invalida deveria falhar")
	}
	if _, err := Decrypt(random.Bytes(15), gcmIV(t), nil, nil); err == nil {
		t.Error("Decrypt com chave invalida deveria falhar")
	}
}
