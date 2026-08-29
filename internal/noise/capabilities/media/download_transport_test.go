package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Golden do material de chave derivado: se GetKeys mudar de tamanho, de
// offset ou de rotulo HKDF, toda midia ja' trocada com o servidor deixa de
// decriptar. O vetor abaixo foi gerado com a implementacao pre-Fase E.
func TestGetKeysGolden(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x01}, mediaKeyLength)
	iv, cipherKey, macKey, refKey := GetKeys(mediaKey, TypeImage)

	if len(iv) != mediaIVLength {
		t.Errorf("len(iv) = %d, esperado %d", len(iv), mediaIVLength)
	}
	if len(cipherKey) != mediaCipherKeyLength {
		t.Errorf("len(cipherKey) = %d, esperado %d", len(cipherKey), mediaCipherKeyLength)
	}
	if len(macKey) != mediaMACKeyLength {
		t.Errorf("len(macKey) = %d, esperado %d", len(macKey), mediaMACKeyLength)
	}
	if total := len(iv) + len(cipherKey) + len(macKey) + len(refKey); total != mediaKeyExpandedLength {
		t.Errorf("as quatro fatias somam %d, esperado %d", total, mediaKeyExpandedLength)
	}

	const wantIV = "26cd9086031e8165f56558c2ffbf64b4"
	if got := hex.EncodeToString(iv); got != wantIV {
		t.Errorf("iv = %s, golden %s", got, wantIV)
	}
}

// Tipos de midia diferentes precisam derivar chaves diferentes da mesma
// mediaKey: e' o rotulo HKDF que separa um dominio do outro.
func TestGetKeysSeparaPorType(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x02}, mediaKeyLength)
	_, imageKey, _, _ := GetKeys(mediaKey, TypeImage)
	_, videoKey, _, _ := GetKeys(mediaKey, TypeVideo)
	if bytes.Equal(imageKey, videoKey) {
		t.Fatal("TypeImage e TypeVideo derivaram a mesma cipherKey")
	}
}

func TestValidateMedia(t *testing.T) {
	iv := bytes.Repeat([]byte{0x03}, mediaIVLength)
	macKey := bytes.Repeat([]byte{0x04}, mediaMACKeyLength)
	file := []byte("conteudo cifrado")

	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(file)
	goodMAC := h.Sum(nil)[:mediaHMACLength]

	if err := ValidateMedia(iv, file, macKey, goodMAC); err != nil {
		t.Fatalf("ValidateMedia com MAC correto devolveu erro: %v", err)
	}
	badMAC := append([]byte{}, goodMAC...)
	badMAC[0] ^= 0xFF
	if err := ValidateMedia(iv, file, macKey, badMAC); !errors.Is(err, ErrInvalidMediaHMAC) {
		t.Fatalf("ValidateMedia com MAC adulterado = %v, esperado ErrInvalidMediaHMAC", err)
	}
	if err := ValidateMedia(iv, append(file, '!'), macKey, goodMAC); !errors.Is(err, ErrInvalidMediaHMAC) {
		t.Fatal("ValidateMedia aceitou conteudo adulterado")
	}
}

func TestShouldRetryDownload(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"contexto cancelado nao repete", context.Canceled, false},
		{"erro de rede repete", &net.OpError{Op: "dial", Err: errors.New("timeout")}, true},
		{"stream error de http2 repete", errors.New("stream error: stream ID 5; INTERNAL_ERROR"), true},
		{"erro generico nao repete", errors.New("qualquer outra coisa"), false},
		{"403 nao repete", DownloadHTTPError{Response: &http.Response{StatusCode: http.StatusForbidden}}, false},
		{"404 nao repete", DownloadHTTPError{Response: &http.Response{StatusCode: http.StatusNotFound}}, false},
		{"500 nao repete", DownloadHTTPError{Response: &http.Response{StatusCode: http.StatusInternalServerError}}, false},
		{"502 repete", DownloadHTTPError{Response: &http.Response{StatusCode: http.StatusBadGateway}}, true},
		{"503 repete", DownloadHTTPError{Response: &http.Response{StatusCode: http.StatusServiceUnavailable}}, true},
		{"429 repete", DownloadHTTPError{Response: &http.Response{StatusCode: http.StatusTooManyRequests}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRetryDownload(tt.err); got != tt.want {
				t.Fatalf("ShouldRetryDownload(%v) = %v, esperado %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryDelay(t *testing.T) {
	// Sem header Retry-After: backoff linear (N+1) * passo.
	generic := errors.New("qualquer")
	for retryNum := 0; retryNum < 3; retryNum++ {
		want := time.Duration(retryNum+1) * mediaDownloadRetryStep
		if got := retryDelay(generic, retryNum); got != want {
			t.Errorf("retryDelay(_, %d) = %s, esperado %s", retryNum, got, want)
		}
	}
	// Com Retry-After, o servidor manda.
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
	resp.Header.Set("Retry-After", "7")
	if got := retryDelay(DownloadHTTPError{Response: resp}, 0); got != 7*time.Second {
		t.Errorf("retryDelay com Retry-After = %s, esperado 7s", got)
	}
}

func TestDownloadHTTPErrorMensagemEComparacao(t *testing.T) {
	err := DownloadHTTPError{Response: &http.Response{StatusCode: 404}}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error() = %q, esperado conter o status", err.Error())
	}
	if !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Error("errors.Is nao reconheceu o sentinela de 404")
	}
	if errors.Is(err, ErrMediaDownloadFailedWith403) {
		t.Error("404 nao deveria casar com o sentinela de 403")
	}
}

func TestDoDownloadRequestEnviaCabecalhosEPropagaStatus(t *testing.T) {
	var gotOrigin, gotReferer, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrigin = r.Header.Get("Origin")
		gotReferer = r.Header.Get("Referer")
		gotUA = r.Header.Get("User-Agent")
		if r.URL.Path == "/faltando" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	resp, err := DoDownloadRequest(context.Background(), tr, "https://mmg.whatsapp.net/existe")
	if err != nil {
		t.Fatalf("DoDownloadRequest devolveu erro: %v", err)
	}
	_ = resp.Body.Close()
	if gotOrigin == "" || gotReferer == "" {
		t.Errorf("Origin=%q Referer=%q, esperados nao vazios", gotOrigin, gotReferer)
	}

	// Cliente Messenger manda o User-Agent configurado; o padrao (WhatsApp) nao.
	tr.messenger = true
	tr.userAgent = "MeuAgente/1.0"
	resp, err = DoDownloadRequest(context.Background(), tr, "https://mmg.whatsapp.net/existe")
	if err != nil {
		t.Fatalf("DoDownloadRequest (messenger) devolveu erro: %v", err)
	}
	_ = resp.Body.Close()
	if gotUA != "MeuAgente/1.0" {
		t.Errorf("User-Agent = %q, esperado o do MessengerConfig", gotUA)
	}
	tr.messenger = false

	_, err = DoDownloadRequest(context.Background(), tr, "https://mmg.whatsapp.net/faltando")
	if !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("status 404 virou %v, esperado ErrMediaDownloadFailedWith404", err)
	}
}

func TestDoDownloadRequestRejeitaURLInvalida(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	_, err := DoDownloadRequest(context.Background(), tr, "://url quebrada")
	if err == nil || !strings.Contains(err.Error(), "failed to prepare request") {
		t.Fatalf("erro = %v, esperado falha ao montar a requisicao", err)
	}
}

func TestDownloadEncryptedSeparaMACEValidaHash(t *testing.T) {
	blob := append(bytes.Repeat([]byte{0xAA}, 32), bytes.Repeat([]byte{0xBB}, mediaHMACLength)...)
	blobHash := sha256.Sum256(blob)
	body := blob

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	ctx := context.Background()

	file, mac, err := DownloadEncrypted(ctx, tr, "https://mmg.whatsapp.net/x", blobHash[:])
	if err != nil {
		t.Fatalf("DownloadEncrypted devolveu erro: %v", err)
	}
	if !bytes.Equal(file, blob[:32]) {
		t.Error("o corpo do arquivo nao bate com o blob sem o MAC")
	}
	if !bytes.Equal(mac, blob[32:]) {
		t.Error("o MAC extraido nao bate com os ultimos bytes do blob")
	}

	wrongHash := bytes.Repeat([]byte{0x00}, sha256HashLength)
	if _, _, err = DownloadEncrypted(ctx, tr, "https://mmg.whatsapp.net/x", wrongHash); !errors.Is(err, ErrInvalidMediaEncSHA256) {
		t.Fatalf("hash divergente = %v, esperado ErrInvalidMediaEncSHA256", err)
	}

	body = bytes.Repeat([]byte{0xAA}, mediaHMACLength) // curto demais
	shortHash := sha256.Sum256(body)
	if _, _, err = DownloadEncrypted(ctx, tr, "https://mmg.whatsapp.net/x", shortHash[:]); !errors.Is(err, ErrTooShortFile) {
		t.Fatalf("arquivo curto = %v, esperado ErrTooShortFile", err)
	}
}

func TestDownloadEncryptedPropagaErroDeTransporte(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	if _, _, err := DownloadEncrypted(context.Background(), tr, "https://mmg.whatsapp.net/x", nil); !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("erro = %v, esperado ErrMediaDownloadFailedWith404", err)
	}
}

func TestDownloadAndDecryptRoundTrip(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x05}, mediaKeyLength)
	plaintext := []byte("um conteudo de midia qualquer, com tamanho nao multiplo de bloco")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	ctx := context.Background()
	url := "https://mmg.whatsapp.net/x"

	got, err := DownloadAndDecrypt(ctx, tr, url, mediaKey, TypeImage, len(plaintext), encSHA, plainSHA)
	if err != nil {
		t.Fatalf("DownloadAndDecrypt devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("plaintext decriptado = %q, esperado %q", got, plaintext)
	}

	_, err = DownloadAndDecrypt(ctx, tr, url, mediaKey, TypeImage, len(plaintext)+1, encSHA, plainSHA)
	if !errors.Is(err, ErrFileLengthMismatch) {
		t.Errorf("tamanho divergente = %v, esperado ErrFileLengthMismatch", err)
	}

	// Com os avisos desligados, tamanho e hash divergentes deixam de virar erro.
	tr.warnings = false
	if _, err = DownloadAndDecrypt(ctx, tr, url, mediaKey, TypeImage, len(plaintext)+1, encSHA, plainSHA); err != nil {
		t.Errorf("com ReturnDownloadWarnings=false nao deveria haver erro, veio %v", err)
	}
	tr.warnings = true

	_, err = DownloadAndDecrypt(ctx, tr, url, mediaKey, TypeImage, UnknownFileLength, encSHA, plainSHA)
	if err != nil {
		t.Errorf("UnknownFileLength deveria pular a validacao de tamanho, veio %v", err)
	}

	badPlainSHA := bytes.Repeat([]byte{0x00}, sha256HashLength)
	_, err = DownloadAndDecrypt(ctx, tr, url, mediaKey, TypeImage, UnknownFileLength, encSHA, badPlainSHA)
	if !errors.Is(err, ErrInvalidMediaSHA256) {
		t.Errorf("hash de plaintext divergente = %v, esperado ErrInvalidMediaSHA256", err)
	}
}

// Midia sem cifra (mediaKey, fileEncSHA256 e mac nulos) sai como veio.
func TestDownloadAndDecryptMidiaNaoCifrada(t *testing.T) {
	plaintext := []byte("sem cifra nenhuma")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(plaintext)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	got, err := DownloadAndDecrypt(
		context.Background(), tr, "https://mmg.whatsapp.net/x", nil, TypeImage, UnknownFileLength, nil, nil)
	if err != nil {
		t.Fatalf("DownloadAndDecrypt devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("conteudo = %q, esperado %q", got, plaintext)
	}
}

// Um ciphertext com tamanho invalido para CBC precisa virar erro de decriptacao.
func TestDownloadAndDecryptFalhaNaDecriptacao(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x09}, mediaKeyLength)
	iv, _, macKey, _ := GetKeys(mediaKey, TypeImage)
	ciphertext := bytes.Repeat([]byte{0x00}, 7) // nao e' multiplo do bloco AES
	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(ciphertext)
	blob := append(append([]byte{}, ciphertext...), h.Sum(nil)[:mediaHMACLength]...)
	blobHash := sha256.Sum256(blob)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	_, err := DownloadAndDecrypt(
		context.Background(), tr, "https://mmg.whatsapp.net/x", mediaKey, TypeImage,
		UnknownFileLength, blobHash[:], nil)
	if err == nil || !strings.Contains(err.Error(), "failed to decrypt file") {
		t.Fatalf("erro = %v, esperado falha de decriptacao", err)
	}
}

func TestDownloadPossiblyEncryptedWithRetriesDesiste(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	_, _, err := DownloadPossiblyEncryptedWithRetries(
		context.Background(), tr, "https://mmg.whatsapp.net/x", bytes.Repeat([]byte{0x00}, sha256HashLength))
	if err == nil {
		t.Fatal("esperado erro apos esgotar as tentativas")
	}
	if got := int(hits.Load()); got != mediaDownloadMaxRetries {
		t.Fatalf("o servidor recebeu %d tentativas, esperado mediaDownloadMaxRetries (%d)", got, mediaDownloadMaxRetries)
	}
}

// Erro que nao e' retentavel precisa sair na primeira tentativa.
func TestDownloadPossiblyEncryptedWithRetriesNaoRepete404(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	_, _, err := DownloadPossiblyEncryptedWithRetries(
		context.Background(), tr, "https://mmg.whatsapp.net/x", bytes.Repeat([]byte{0x00}, sha256HashLength))
	if !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("erro = %v, esperado ErrMediaDownloadFailedWith404", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("o servidor recebeu %d tentativas, esperado 1", got)
	}
}

// Sem checksum, o laco usa o caminho nao cifrado (DownloadRaw).
func TestDownloadPossiblyEncryptedWithRetriesSemChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("cru"))
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	file, mac, err := DownloadPossiblyEncryptedWithRetries(
		context.Background(), tr, "https://mmg.whatsapp.net/x", nil)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if string(file) != "cru" || mac != nil {
		t.Fatalf("file = %q, mac = %v; esperado o corpo cru e mac nil", file, mac)
	}
}

// Contexto cancelado durante a espera do backoff aborta o laco.
func TestDownloadPossiblyEncryptedWithRetriesRespeitaContexto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, _, err := DownloadPossiblyEncryptedWithRetries(ctx, tr, "https://mmg.whatsapp.net/x", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, esperado context.Canceled", err)
	}
}
