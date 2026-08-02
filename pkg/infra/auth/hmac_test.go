package auth_test

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"wa-api/pkg/infra/auth"
)

// aes256Key tem exatamente 32 bytes: AES-256. As chaves de encriptação deste
// repositório vêm de configuração, então o teste fixa o tamanho válido em vez
// de depender do acaso.
const aes256Key = "0123456789abcdef0123456789abcdef"

func TestEncryptDecryptHMACKey_RoundTrip(t *testing.T) {
	ct, err := auth.EncryptHMACKey("minha-chave-hmac", aes256Key)
	if err != nil {
		t.Fatalf("EncryptHMACKey: %v", err)
	}

	got, err := auth.DecryptHMACKey(ct, []byte(aes256Key))
	if err != nil {
		t.Fatalf("DecryptHMACKey: %v", err)
	}
	if got != "minha-chave-hmac" {
		t.Fatalf("round-trip: got %q, want %q", got, "minha-chave-hmac")
	}
}

// TestEncryptHMACKey_IsNotDeterministic mata a mutação em que o nonce vira
// um valor fixo (ou zero): dois ciphertexts do mesmo plaintext têm que diferir,
// senão o AES-GCM perde a propriedade que justifica usá-lo.
func TestEncryptHMACKey_IsNotDeterministic(t *testing.T) {
	a, err := auth.EncryptHMACKey("mesmo-plaintext", aes256Key)
	if err != nil {
		t.Fatalf("EncryptHMACKey: %v", err)
	}
	b, err := auth.EncryptHMACKey("mesmo-plaintext", aes256Key)
	if err != nil {
		t.Fatalf("EncryptHMACKey: %v", err)
	}

	if string(a) == string(b) {
		t.Fatal("dois ciphertexts do mesmo plaintext sao identicos — o nonce virou constante")
	}
}

// TestEncryptHMACKey_CiphertextDoesNotContainPlaintext mata a mutação mais
// grosseira de todas: encriptar virando um no-op que devolve o plaintext.
func TestEncryptHMACKey_CiphertextDoesNotContainPlaintext(t *testing.T) {
	const plain = "plaintext-que-nao-pode-vazar"
	ct, err := auth.EncryptHMACKey(plain, aes256Key)
	if err != nil {
		t.Fatalf("EncryptHMACKey: %v", err)
	}

	if string(ct) == plain {
		t.Fatal("o ciphertext e' o proprio plaintext")
	}
	for i := 0; i+len(plain) <= len(ct); i++ {
		if string(ct[i:i+len(plain)]) == plain {
			t.Fatal("o plaintext aparece literalmente dentro do ciphertext")
		}
	}
}

func TestEncryptHMACKey_RejectsBadKeys(t *testing.T) {
	cases := map[string]string{
		"chave vazia":           "",
		"chave curta demais":    "curta",
		"tamanho invalido (31)": "0123456789abcdef0123456789abcde",
		"tamanho invalido (33)": "0123456789abcdef0123456789abcdefX",
	}

	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			ct, err := auth.EncryptHMACKey("x", key)
			if err == nil {
				t.Fatal("chave invalida foi aceita")
			}
			if ct != nil {
				t.Fatal("erro veio acompanhado de ciphertext")
			}
		})
	}
}

func TestDecryptHMACKey_RejectsWrongKey(t *testing.T) {
	ct, err := auth.EncryptHMACKey("segredo", aes256Key)
	if err != nil {
		t.Fatalf("EncryptHMACKey: %v", err)
	}

	const outraChave = "fedcba9876543210fedcba9876543210"
	got, err := auth.DecryptHMACKey(ct, []byte(outraChave))
	if err == nil {
		t.Fatalf("ciphertext decriptou com a chave errada, resultado %q", got)
	}
	if got != "" {
		t.Fatalf("erro veio acompanhado de plaintext %q", got)
	}
}

// TestDecryptHMACKey_RejectsTamperedCiphertext é a propriedade de autenticação
// do GCM: um bit trocado tem que ser rejeitado, não decriptado em lixo.
func TestDecryptHMACKey_RejectsTamperedCiphertext(t *testing.T) {
	ct, err := auth.EncryptHMACKey("segredo", aes256Key)
	if err != nil {
		t.Fatalf("EncryptHMACKey: %v", err)
	}

	tampered := make([]byte, len(ct))
	copy(tampered, ct)
	tampered[len(tampered)-1] ^= 0x01

	if _, err := auth.DecryptHMACKey(tampered, []byte(aes256Key)); err == nil {
		t.Fatal("ciphertext adulterado foi aceito — a tag do GCM nao esta sendo verificada")
	}
}

func TestDecryptHMACKey_RejectsMalformedInput(t *testing.T) {
	cases := map[string][]byte{
		"vazio":                       {},
		"menor que o nonce":           []byte("curto"),
		"exatamente o nonce, sem tag": make([]byte, 12),
		"lixo aleatorio":              []byte("0123456789abcdefghijklmnopqrstuvwxyz"),
	}

	for name, ct := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := auth.DecryptHMACKey(ct, []byte(aes256Key))
			if err == nil {
				t.Fatalf("entrada malformada foi aceita, resultado %q", got)
			}
		})
	}
}

func TestDecryptHMACKey_RejectsEmptyEncryptionKey(t *testing.T) {
	if _, err := auth.DecryptHMACKey([]byte("qualquer-coisa"), nil); err == nil {
		t.Fatal("chave de encriptacao vazia foi aceita")
	}
}

// TestDecryptHMACKey_RejectsKeysOfInvalidLength fecha a contrapartida do lado da
// decriptação: uma chave não-vazia mas de tamanho inválido para AES tem que
// virar erro, e não passar adiante para o GCM. O caminho não estava coberto —
// só o caso de chave vazia estava.
func TestDecryptHMACKey_RejectsKeysOfInvalidLength(t *testing.T) {
	ct, err := auth.EncryptHMACKey("segredo", aes256Key)
	if err != nil {
		t.Fatalf("EncryptHMACKey: %v", err)
	}

	cases := map[string]string{
		"chave curta demais":    "curta",
		"tamanho invalido (31)": "0123456789abcdef0123456789abcde",
		"tamanho invalido (33)": "0123456789abcdef0123456789abcdefX",
	}

	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := auth.DecryptHMACKey(ct, []byte(key))
			if err == nil {
				t.Fatalf("chave de tamanho invalido foi aceita, resultado %q", got)
			}
			if got != "" {
				t.Fatalf("erro veio acompanhado de plaintext %q", got)
			}
		})
	}
}

// failingReader é uma fonte de entropia quebrada.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropia indisponivel") }

// TestEncryptHMACKey_AbortsWhenEntropyFails prova que, se a fonte de entropia
// falhar, EncryptHMACKey aborta em vez de selar com um nonce zerado. Nonce
// previsível em AES-GCM destrói a confidencialidade sob reuso de chave, então
// "seguir mesmo assim" seria pior que falhar.
//
// Não usa t.Parallel: troca a variável global crypto/rand.Reader.
func TestEncryptHMACKey_AbortsWhenEntropyFails(t *testing.T) {
	original := rand.Reader
	rand.Reader = failingReader{}
	t.Cleanup(func() { rand.Reader = original })

	ct, err := auth.EncryptHMACKey("segredo", aes256Key)
	if err == nil {
		t.Fatal("encriptacao seguiu adiante com a fonte de entropia quebrada")
	}
	if ct != nil {
		t.Fatalf("erro veio acompanhado de ciphertext de %d bytes", len(ct))
	}
}

// TestGenerateHmacSignature_MatchesReferenceHMAC compara contra um HMAC-SHA256
// calculado à parte. É o que impede a assinatura de virar qualquer outra coisa
// (um SHA256 simples, um HMAC sobre a chave errada) sem ninguém notar.
func TestGenerateHmacSignature_MatchesReferenceHMAC(t *testing.T) {
	const hmacKey = "chave-do-webhook"
	payload := []byte(`{"event":"Message"}`)

	encKey, err := auth.EncryptHMACKey(hmacKey, aes256Key)
	if err != nil {
		t.Fatalf("EncryptHMACKey: %v", err)
	}

	got, err := auth.GenerateHmacSignature(payload, encKey, []byte(aes256Key))
	if err != nil {
		t.Fatalf("GenerateHmacSignature: %v", err)
	}

	h := hmac.New(sha256.New, []byte(hmacKey))
	h.Write(payload)
	want := hex.EncodeToString(h.Sum(nil))

	if got != want {
		t.Fatalf("assinatura: got %q, want %q", got, want)
	}
}

// TestGenerateHmacSignature_ChangesWithPayload é a propriedade que o webhook
// consumidor depende: assinatura constante não assina nada.
func TestGenerateHmacSignature_ChangesWithPayload(t *testing.T) {
	encKey, err := auth.EncryptHMACKey("chave-do-webhook", aes256Key)
	if err != nil {
		t.Fatalf("EncryptHMACKey: %v", err)
	}

	a, err := auth.GenerateHmacSignature([]byte(`{"a":1}`), encKey, []byte(aes256Key))
	if err != nil {
		t.Fatalf("GenerateHmacSignature: %v", err)
	}
	b, err := auth.GenerateHmacSignature([]byte(`{"a":2}`), encKey, []byte(aes256Key))
	if err != nil {
		t.Fatalf("GenerateHmacSignature: %v", err)
	}

	if a == b {
		t.Fatal("payloads diferentes produziram a mesma assinatura")
	}
	if a == "" {
		t.Fatal("assinatura vazia com chave hmac configurada")
	}
}

// TestGenerateHmacSignature_EmptyKeyMeansUnsigned trava o contrato de
// "usuário sem hmac_key": string vazia e SEM erro, porque o chamador
// (dispatch_webhook.go) usa isso para decidir não mandar o header.
func TestGenerateHmacSignature_EmptyKeyMeansUnsigned(t *testing.T) {
	sig, err := auth.GenerateHmacSignature([]byte("payload"), nil, []byte(aes256Key))
	if err != nil {
		t.Fatalf("chave hmac ausente virou erro: %v", err)
	}
	if sig != "" {
		t.Fatalf("chave hmac ausente produziu assinatura %q", sig)
	}
}

func TestGenerateHmacSignature_ReportsUndecryptableKey(t *testing.T) {
	// Chave hmac presente mas indecifrável não pode virar assinatura vazia:
	// isso mandaria o webhook sem assinatura silenciosamente.
	sig, err := auth.GenerateHmacSignature([]byte("payload"), []byte("nao-e-ciphertext"), []byte(aes256Key))
	if err == nil {
		t.Fatalf("chave hmac corrompida nao reportou erro, assinatura %q", sig)
	}
	if sig != "" {
		t.Fatalf("erro veio acompanhado de assinatura %q", sig)
	}
}
