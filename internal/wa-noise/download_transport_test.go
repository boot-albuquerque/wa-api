// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

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
	"sync/atomic"
	"testing"
)

// Golden do material de chave derivado: se getMediaKeys mudar de tamanho, de
// offset ou de rotulo HKDF, toda midia ja' trocada com o servidor deixa de
// decriptar. O vetor abaixo foi gerado com a implementacao pre-Fase E.
func TestGetMediaKeysGolden(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x01}, mediaKeyLength)
	iv, cipherKey, macKey, refKey := getMediaKeys(mediaKey, MediaImage)

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
func TestGetMediaKeysSeparaPorMediaType(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x02}, mediaKeyLength)
	_, imageKey, _, _ := getMediaKeys(mediaKey, MediaImage)
	_, videoKey, _, _ := getMediaKeys(mediaKey, MediaVideo)
	if bytes.Equal(imageKey, videoKey) {
		t.Fatal("MediaImage e MediaVideo derivaram a mesma cipherKey")
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

	if err := validateMedia(iv, file, macKey, goodMAC); err != nil {
		t.Fatalf("validateMedia com MAC correto devolveu erro: %v", err)
	}
	badMAC := append([]byte{}, goodMAC...)
	badMAC[0] ^= 0xFF
	if err := validateMedia(iv, file, macKey, badMAC); !errors.Is(err, ErrInvalidMediaHMAC) {
		t.Fatalf("validateMedia com MAC adulterado = %v, esperado ErrInvalidMediaHMAC", err)
	}
	if err := validateMedia(iv, append(file, '!'), macKey, goodMAC); !errors.Is(err, ErrInvalidMediaHMAC) {
		t.Fatal("validateMedia aceitou conteudo adulterado")
	}
}

func TestShouldRetryMediaDownload(t *testing.T) {
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
			if got := shouldRetryMediaDownload(tt.err); got != tt.want {
				t.Fatalf("shouldRetryMediaDownload(%v) = %v, esperado %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDoMediaDownloadRequestEnviaCabecalhosEPropagaStatus(t *testing.T) {
	var gotOrigin, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrigin = r.Header.Get("Origin")
		gotReferer = r.Header.Get("Referer")
		if r.URL.Path == "/faltando" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	resp, err := cli.doMediaDownloadRequest(context.Background(), "https://mmg.whatsapp.net/existe")
	if err != nil {
		t.Fatalf("doMediaDownloadRequest devolveu erro: %v", err)
	}
	_ = resp.Body.Close()
	if gotOrigin == "" || gotReferer == "" {
		t.Errorf("Origin=%q Referer=%q, esperados nao vazios", gotOrigin, gotReferer)
	}

	_, err = cli.doMediaDownloadRequest(context.Background(), "https://mmg.whatsapp.net/faltando")
	if !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("status 404 virou %v, esperado ErrMediaDownloadFailedWith404", err)
	}
}

func TestDownloadEncryptedMediaSeparaMACEValidaHash(t *testing.T) {
	blob := append(bytes.Repeat([]byte{0xAA}, 32), bytes.Repeat([]byte{0xBB}, mediaHMACLength)...)
	blobHash := sha256.Sum256(blob)
	body := blob

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)
	ctx := context.Background()

	file, mac, err := cli.downloadEncryptedMedia(ctx, "https://mmg.whatsapp.net/x", blobHash[:])
	if err != nil {
		t.Fatalf("downloadEncryptedMedia devolveu erro: %v", err)
	}
	if !bytes.Equal(file, blob[:32]) {
		t.Error("o corpo do arquivo nao bate com o blob sem o MAC")
	}
	if !bytes.Equal(mac, blob[32:]) {
		t.Error("o MAC extraido nao bate com os ultimos bytes do blob")
	}

	wrongHash := bytes.Repeat([]byte{0x00}, sha256HashLength)
	if _, _, err = cli.downloadEncryptedMedia(ctx, "https://mmg.whatsapp.net/x", wrongHash); !errors.Is(err, ErrInvalidMediaEncSHA256) {
		t.Fatalf("hash divergente = %v, esperado ErrInvalidMediaEncSHA256", err)
	}

	body = bytes.Repeat([]byte{0xAA}, mediaHMACLength) // curto demais
	shortHash := sha256.Sum256(body)
	if _, _, err = cli.downloadEncryptedMedia(ctx, "https://mmg.whatsapp.net/x", shortHash[:]); !errors.Is(err, ErrTooShortFile) {
		t.Fatalf("arquivo curto = %v, esperado ErrTooShortFile", err)
	}
}

func TestDownloadAndDecryptRoundTrip(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x05}, mediaKeyLength)
	plaintext := []byte("um conteudo de midia qualquer, com tamanho nao multiplo de bloco")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, MediaImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)
	ctx := context.Background()
	url := "https://mmg.whatsapp.net/x"

	got, err := cli.downloadAndDecrypt(ctx, url, mediaKey, MediaImage, len(plaintext), encSHA, plainSHA)
	if err != nil {
		t.Fatalf("downloadAndDecrypt devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("plaintext decriptado = %q, esperado %q", got, plaintext)
	}

	_, err = cli.downloadAndDecrypt(ctx, url, mediaKey, MediaImage, len(plaintext)+1, encSHA, plainSHA)
	if !errors.Is(err, ErrFileLengthMismatch) {
		t.Errorf("tamanho divergente = %v, esperado ErrFileLengthMismatch", err)
	}

	_, err = cli.downloadAndDecrypt(ctx, url, mediaKey, MediaImage, unknownFileLength, encSHA, plainSHA)
	if err != nil {
		t.Errorf("unknownFileLength deveria pular a validacao de tamanho, veio %v", err)
	}

	badPlainSHA := bytes.Repeat([]byte{0x00}, sha256HashLength)
	_, err = cli.downloadAndDecrypt(ctx, url, mediaKey, MediaImage, unknownFileLength, encSHA, badPlainSHA)
	if !errors.Is(err, ErrInvalidMediaSHA256) {
		t.Errorf("hash de plaintext divergente = %v, esperado ErrInvalidMediaSHA256", err)
	}
}

func TestDownloadPossiblyEncryptedMediaWithRetriesDesiste(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	_, _, err := cli.downloadPossiblyEncryptedMediaWithRetries(
		context.Background(), "https://mmg.whatsapp.net/x", bytes.Repeat([]byte{0x00}, sha256HashLength))
	if err == nil {
		t.Fatal("esperado erro apos esgotar as tentativas")
	}
	if got := int(hits.Load()); got != mediaDownloadMaxRetries {
		t.Fatalf("o servidor recebeu %d tentativas, esperado mediaDownloadMaxRetries (%d)", got, mediaDownloadMaxRetries)
	}
}

// Erro que nao e' retentavel precisa sair na primeira tentativa.
func TestDownloadPossiblyEncryptedMediaWithRetriesNaoRepete404(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	_, _, err := cli.downloadPossiblyEncryptedMediaWithRetries(
		context.Background(), "https://mmg.whatsapp.net/x", bytes.Repeat([]byte{0x00}, sha256HashLength))
	if !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("erro = %v, esperado ErrMediaDownloadFailedWith404", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("o servidor recebeu %d tentativas, esperado 1", got)
	}
}
