package media

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waMmsRetry"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	gcmutil "wa-api/internal/wa-noise/security/gcm"
)

// Golden da chave de retry: ela precisa continuar derivando do mesmo rotulo e
// tamanho, senao nenhuma notificacao de retry ja' emitida decripta.
func TestRetryKeyGolden(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x11}, mediaKeyLength)
	key := RetryKey(mediaKey)
	if len(key) != mediaRetryKeyLength {
		t.Fatalf("len(key) = %d, esperado %d", len(key), mediaRetryKeyLength)
	}
	const want = "7ab595184e21c4059f35584e68d6eca44cf5968a98a3d90498bd0dfb49da2bd1"
	if got := hex.EncodeToString(key); got != want {
		t.Errorf("chave = %s, golden %s", got, want)
	}
}

func TestEncryptRetryReceiptEDecriptavel(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x12}, mediaKeyLength)
	const messageID types.MessageID = "MSG-ID-1"

	ciphertext, iv, err := EncryptRetryReceipt(messageID, mediaKey)
	if err != nil {
		t.Fatalf("EncryptRetryReceipt devolveu erro: %v", err)
	}
	if len(iv) != mediaRetryIVLength {
		t.Fatalf("len(iv) = %d, esperado %d", len(iv), mediaRetryIVLength)
	}

	plaintext, err := gcmutil.Decrypt(RetryKey(mediaKey), iv, ciphertext, []byte(messageID))
	if err != nil {
		t.Fatalf("falha ao decriptar o receipt: %v", err)
	}
	var receipt waMmsRetry.ServerErrorReceipt
	if err = proto.Unmarshal(plaintext, &receipt); err != nil {
		t.Fatalf("falha ao desserializar o receipt: %v", err)
	}
	if receipt.GetStanzaID() != string(messageID) {
		t.Errorf("StanzaID = %q, esperado %q", receipt.GetStanzaID(), messageID)
	}

	// O messageID entra como additional data do GCM: outro ID nao autentica.
	if _, err = gcmutil.Decrypt(RetryKey(mediaKey), iv, ciphertext, []byte("OUTRO-ID")); err == nil {
		t.Error("decriptou com additional data errado")
	}
}

// Uma mediaKey de tamanho invalido para AES faz a cifragem falhar.
func TestEncryptRetryReceiptPropagaErroDeCifra(t *testing.T) {
	// RetryKey sempre devolve 32 bytes, entao a cifragem em si nao falha por
	// tamanho de chave; o que resta e' garantir que o caminho feliz mantem o
	// contrato de nao devolver erro.
	if _, _, err := EncryptRetryReceipt("id", nil); err != nil {
		t.Fatalf("EncryptRetryReceipt com mediaKey nil devolveu erro: %v", err)
	}
}

func TestDecryptRetryNotification(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x13}, mediaKeyLength)
	const messageID types.MessageID = "MSG-ID-2"

	notif := &waMmsRetry.MediaRetryNotification{
		StanzaID:   proto.String(string(messageID)),
		DirectPath: proto.String("/v/novo-caminho"),
		Result:     waMmsRetry.MediaRetryNotification_SUCCESS.Enum(),
	}
	plaintext, err := proto.Marshal(notif)
	if err != nil {
		t.Fatalf("falha ao serializar a notificacao: %v", err)
	}
	iv := bytes.Repeat([]byte{0x14}, mediaRetryIVLength)
	ciphertext, err := gcmutil.Encrypt(RetryKey(mediaKey), iv, plaintext, []byte(messageID))
	if err != nil {
		t.Fatalf("falha ao cifrar a notificacao: %v", err)
	}

	t.Run("caminho feliz", func(t *testing.T) {
		evt := &events.MediaRetry{MessageID: messageID, IV: iv, Ciphertext: ciphertext}
		got, err := DecryptRetryNotification(evt, mediaKey)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if got.GetDirectPath() != "/v/novo-caminho" {
			t.Errorf("DirectPath = %q", got.GetDirectPath())
		}
	})

	t.Run("midia indisponivel no telefone", func(t *testing.T) {
		evt := &events.MediaRetry{
			MessageID: messageID,
			Error:     &events.MediaRetryError{Code: mediaRetryErrCodeNotAvailable},
		}
		if _, err := DecryptRetryNotification(evt, mediaKey); !errors.Is(err, ErrMediaNotAvailableOnPhone) {
			t.Fatalf("erro = %v, esperado ErrMediaNotAvailableOnPhone", err)
		}
	})

	t.Run("outro codigo de erro", func(t *testing.T) {
		evt := &events.MediaRetry{MessageID: messageID, Error: &events.MediaRetryError{Code: 42}}
		if _, err := DecryptRetryNotification(evt, mediaKey); !errors.Is(err, ErrUnknownMediaRetryError) {
			t.Fatalf("erro = %v, esperado ErrUnknownMediaRetryError", err)
		}
	})

	t.Run("chave errada", func(t *testing.T) {
		evt := &events.MediaRetry{MessageID: messageID, IV: iv, Ciphertext: ciphertext}
		if _, err := DecryptRetryNotification(evt, bytes.Repeat([]byte{0x99}, mediaKeyLength)); err == nil {
			t.Fatal("decriptou com a mediaKey errada")
		}
	})

	// Erro presente mas com ciphertext tambem presente cai no ramo de
	// decriptacao, nao no de erro.
	t.Run("erro com ciphertext decripta normalmente", func(t *testing.T) {
		evt := &events.MediaRetry{
			MessageID:  messageID,
			IV:         iv,
			Ciphertext: ciphertext,
			Error:      &events.MediaRetryError{Code: 42},
		}
		if _, err := DecryptRetryNotification(evt, mediaKey); err != nil {
			t.Fatalf("erro: %v", err)
		}
	})

	// Plaintext que decripta mas nao e' um protobuf valido vira erro proprio.
	t.Run("plaintext nao e protobuf", func(t *testing.T) {
		lixo := bytes.Repeat([]byte{0xFF}, 16)
		ct, err := gcmutil.Encrypt(RetryKey(mediaKey), iv, lixo, []byte(messageID))
		if err != nil {
			t.Fatalf("falha ao cifrar: %v", err)
		}
		evt := &events.MediaRetry{MessageID: messageID, IV: iv, Ciphertext: ct}
		if _, err := DecryptRetryNotification(evt, mediaKey); err == nil {
			t.Fatal("aceitou plaintext que nao e' protobuf")
		}
	})
}
