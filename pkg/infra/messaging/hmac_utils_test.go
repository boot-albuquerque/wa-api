package messaging

import (
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"testing/iotest"
)

// key32 e' uma chave AES-256 valida; key17 tem tamanho invalido para AES e e'
// o que faz aes.NewCipher falhar.
const (
	key32 = "0123456789abcdef0123456789abcdef"
	key17 = "0123456789abcdefg"
)

func ptr(s string) *string { return &s }

// TestEncryptDecryptHMACKey_RoundTrip fixa o contrato central: o que foi
// cifrado com a chave global volta identico, e duas cifragens do mesmo texto
// diferem (nonce aleatorio por chamada).
func TestEncryptDecryptHMACKey_RoundTrip(t *testing.T) {
	captureLogs(t)

	enc1, err := EncryptHMACKey("segredo-hmac", ptr(key32))
	if err != nil {
		t.Fatalf("EncryptHMACKey() = %v", err)
	}
	enc2, err := EncryptHMACKey("segredo-hmac", ptr(key32))
	if err != nil {
		t.Fatalf("EncryptHMACKey() segunda chamada = %v", err)
	}
	if string(enc1) == string(enc2) {
		t.Error("duas cifragens do mesmo texto sairam identicas: nonce nao e' aleatorio")
	}

	got, err := DecryptHMACKey(enc1, ptr(key32))
	if err != nil {
		t.Fatalf("DecryptHMACKey() = %v", err)
	}
	if got != "segredo-hmac" {
		t.Errorf("DecryptHMACKey() = %q, want %q", got, "segredo-hmac")
	}
}

func TestEncryptHMACKey_ErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		key     *string
		wantErr string
		wantMsg string
	}{
		{"chave nil", nil, "encryption key not configured", "encryption key not configured"},
		{"chave vazia", ptr(""), "encryption key not configured", "encryption key not configured"},
		{"tamanho invalido para AES", ptr(key17), "failed to create cipher", "failed to create AES cipher"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)

			got, err := EncryptHMACKey("segredo", tc.key)
			if err == nil {
				t.Fatalf("EncryptHMACKey() = %q, want erro", got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("erro = %q, want conter %q", err, tc.wantErr)
			}
			if got != nil {
				t.Errorf("EncryptHMACKey() devolveu %v junto com erro", got)
			}
			logs.requireLog(t, "error", tc.wantMsg, "operation")
		})
	}
}

func TestDecryptHMACKey_ErrorPaths(t *testing.T) {
	valid, err := EncryptHMACKey("segredo", ptr(key32))
	if err != nil {
		t.Fatalf("preparo: EncryptHMACKey() = %v", err)
	}

	tests := []struct {
		name    string
		data    []byte
		key     *string
		wantErr string
		wantMsg string
	}{
		{"chave nil", valid, nil, "encryption key not configured", "encryption key not configured"},
		{"chave vazia", valid, ptr(""), "encryption key not configured", "encryption key not configured"},
		{"tamanho invalido para AES", valid, ptr(key17), "failed to create cipher", "failed to create AES cipher"},
		{"ciphertext menor que o nonce", []byte{1, 2, 3}, ptr(key32), "ciphertext too short", "ciphertext too short"},
		{"chave errada", valid, ptr(strings.Repeat("z", 32)), "failed to decrypt", "failed to decrypt HMAC key"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)

			got, err := DecryptHMACKey(tc.data, tc.key)
			if err == nil {
				t.Fatalf("DecryptHMACKey() = %q, want erro", got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("erro = %q, want conter %q", err, tc.wantErr)
			}
			if got != "" {
				t.Errorf("DecryptHMACKey() devolveu %q junto com erro", got)
			}
			logs.requireLog(t, "error", tc.wantMsg, "operation")
		})
	}
}

// stubNewGCM troca o seam de cipher.NewGCM por uma falha, e restaura no fim.
func stubNewGCM(t *testing.T) {
	t.Helper()
	prev := newGCM
	newGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, errFake }
	t.Cleanup(func() { newGCM = prev })
}

// TestEncryptHMACKey_GCMFailure e TestDecryptHMACKey_GCMFailure exercitam o
// caminho que a biblioteca real nunca produz com AES: sem o seam, esses dois
// `if err != nil` seriam codigo permanentemente nao exercitado.
func TestEncryptHMACKey_GCMFailure(t *testing.T) {
	logs := captureLogs(t)
	stubNewGCM(t)

	got, err := EncryptHMACKey("segredo", ptr(key32))
	if err == nil {
		t.Fatalf("EncryptHMACKey() = %q, want erro", got)
	}
	if !strings.Contains(err.Error(), "failed to create GCM") {
		t.Errorf("erro = %q", err)
	}
	logs.requireLog(t, "error", "failed to create GCM", "error", "operation")
}

func TestDecryptHMACKey_GCMFailure(t *testing.T) {
	logs := captureLogs(t)
	stubNewGCM(t)

	got, err := DecryptHMACKey([]byte("qualquer coisa"), ptr(key32))
	if err == nil {
		t.Fatalf("DecryptHMACKey() = %q, want erro", got)
	}
	if !strings.Contains(err.Error(), "failed to create GCM") {
		t.Errorf("erro = %q", err)
	}
	logs.requireLog(t, "error", "failed to create GCM", "error", "operation")
}

// TestEncryptHMACKey_NonceFailure cobre a falha da fonte de entropia. Uma
// cifragem sem nonce aleatorio nao pode seguir em silencio: o teste fixa que
// ela falha, loga, e nao devolve ciphertext.
func TestEncryptHMACKey_NonceFailure(t *testing.T) {
	logs := captureLogs(t)
	prev := rand.Reader
	rand.Reader = iotest.ErrReader(errFake)
	t.Cleanup(func() { rand.Reader = prev })

	got, err := EncryptHMACKey("segredo", ptr(key32))
	if err == nil {
		t.Fatalf("EncryptHMACKey() = %q, want erro", got)
	}
	if !strings.Contains(err.Error(), "failed to generate nonce") {
		t.Errorf("erro = %q", err)
	}
	if got != nil {
		t.Errorf("EncryptHMACKey() devolveu ciphertext %v junto com erro", got)
	}
	logs.requireLog(t, "error", "failed to generate nonce", "error", "operation", "nonce_bytes")
}

// TestGenerateHmacSignature_EmptyKeyIsNotAnError fixa a decisao de projeto:
// webhook sem chave HMAC configurada sai sem assinatura, e nao com erro.
func TestGenerateHmacSignature_EmptyKeyIsNotAnError(t *testing.T) {
	logs := captureLogs(t)

	sig, err := GenerateHmacSignature([]byte("payload"), nil, ptr(key32))
	if err != nil {
		t.Fatalf("GenerateHmacSignature() = %v", err)
	}
	if sig != "" {
		t.Errorf("assinatura = %q, want vazia", sig)
	}
	logs.requireNoLog(t, "failed to decrypt HMAC key")
}

// TestGenerateHmacSignature_MatchesReference compara com HMAC-SHA256 calculado
// diretamente: a assinatura tem de ser sobre o payload com a chave DECIFRADA.
func TestGenerateHmacSignature_MatchesReference(t *testing.T) {
	captureLogs(t)

	const secret = "chave-hmac-do-cliente"
	encrypted, err := EncryptHMACKey(secret, ptr(key32))
	if err != nil {
		t.Fatalf("preparo: EncryptHMACKey() = %v", err)
	}

	payload := []byte(`{"event":"message"}`)
	got, err := GenerateHmacSignature(payload, encrypted, ptr(key32))
	if err != nil {
		t.Fatalf("GenerateHmacSignature() = %v", err)
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	if want := hex.EncodeToString(h.Sum(nil)); got != want {
		t.Errorf("assinatura = %q, want %q", got, want)
	}
}

// TestGenerateHmacSignature_DecryptFailureIsLogged cobre o unico caminho de
// erro da funcao: chave cifrada ilegivel com a chave global em uso.
func TestGenerateHmacSignature_DecryptFailureIsLogged(t *testing.T) {
	logs := captureLogs(t)

	got, err := GenerateHmacSignature([]byte("payload"), []byte("lixo-nao-cifrado"), ptr(key32))
	if err == nil {
		t.Fatalf("GenerateHmacSignature() = %q, want erro", got)
	}
	if !strings.Contains(err.Error(), "failed to decrypt HMAC key") {
		t.Errorf("erro = %q", err)
	}
	if got != "" {
		t.Errorf("GenerateHmacSignature() devolveu %q junto com erro", got)
	}
	logs.requireLog(t, "error", "failed to decrypt HMAC key while signing webhook payload",
		"error", "payload_bytes", "encrypted_key_bytes")
}

// TestHMACLogsNeverCarrySecrets e' o guarda de vazamento: nenhum registro
// emitido pelos caminhos de erro pode conter a chave de encriptacao nem o
// segredo HMAC em claro.
func TestHMACLogsNeverCarrySecrets(t *testing.T) {
	logs := captureLogs(t)

	const secret = "segredo-hmac-que-nunca-pode-vazar"
	encrypted, err := EncryptHMACKey(secret, ptr(key32))
	if err != nil {
		t.Fatalf("preparo: EncryptHMACKey() = %v", err)
	}
	if _, err := DecryptHMACKey(encrypted, ptr(strings.Repeat("z", 32))); err == nil {
		t.Fatal("preparo: decifragem com chave errada deveria falhar")
	}
	if _, err := GenerateHmacSignature([]byte("p"), []byte("lixo"), ptr(key32)); err == nil {
		t.Fatal("preparo: assinatura com chave ilegivel deveria falhar")
	}

	out := logs.String()
	for _, forbidden := range []string{secret, key32, strings.Repeat("z", 32)} {
		if strings.Contains(out, forbidden) {
			t.Errorf("segredo vazou no log: %s", out)
		}
	}
}
