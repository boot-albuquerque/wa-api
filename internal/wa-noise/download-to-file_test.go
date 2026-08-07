// Copyright (c) 2024 Tulir Asokan
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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waMediaTransport"
)

// tempMediaFile devolve um *os.File descartavel que satisfaz File.
func tempMediaFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "media.bin"))
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporario: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func readAllFromStart(t *testing.T, f File) []byte {
	t.Helper()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("falha ao rebobinar o arquivo: %v", err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("falha ao ler o arquivo: %v", err)
	}
	return data
}

// os.File precisa continuar satisfazendo File: e' o uso normal da API.
var _ File = (*os.File)(nil)

func TestDownloadMediaWithPathToFileRoundTrip(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x20}, mediaKeyLength)
	plaintext := bytes.Repeat([]byte("streaming de midia para disco. "), 40)
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, MediaVideo)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)
	file := tempMediaFile(t)

	err := cli.DownloadMediaWithPathToFile(
		context.Background(), "/v/x", encSHA, plainSHA, mediaKey, len(plaintext), MediaVideo, "", file)
	if err != nil {
		t.Fatalf("DownloadMediaWithPathToFile devolveu erro: %v", err)
	}
	if got := readAllFromStart(t, file); !bytes.Equal(got, plaintext) {
		t.Fatalf("arquivo tem %d bytes, esperado o plaintext de %d bytes", len(got), len(plaintext))
	}
}

func TestDownloadToFileRejeitaMensagemSemURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)
	file := tempMediaFile(t)

	if err := cli.DownloadToFile(context.Background(), &waE2E.ImageMessage{}, file); !errors.Is(err, ErrNoURLPresent) {
		t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
	}
	msg := &waE2E.ImageMessage{URL: proto.String(webWhatsappNetURLPrefix + "/x")}
	if err := cli.DownloadToFile(context.Background(), msg, file); !errors.Is(err, ErrNoURLPresent) {
		t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
	}
	if err := cli.DownloadToFile(context.Background(), notDownloadable{}, file); !errors.Is(err, ErrUnknownMediaType) {
		t.Fatalf("erro = %v, esperado ErrUnknownMediaType", err)
	}

	var nilClient *Client
	if err := nilClient.DownloadToFile(context.Background(), &waE2E.ImageMessage{}, file); !errors.Is(err, ErrClientIsNil) {
		t.Fatalf("erro = %v, esperado ErrClientIsNil", err)
	}
}

func TestDownloadAndDecryptToFileDetectaTamanhoEHashErrados(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x21}, mediaKeyLength)
	plaintext := []byte("conteudo com validacao de tamanho e hash")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, MediaImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)
	ctx := context.Background()
	const url = "https://mmg.whatsapp.net/x"

	err := cli.downloadAndDecryptToFile(ctx, url, mediaKey, MediaImage, len(plaintext)+1, encSHA, plainSHA, tempMediaFile(t))
	if !errors.Is(err, ErrFileLengthMismatch) {
		t.Errorf("tamanho divergente = %v, esperado ErrFileLengthMismatch", err)
	}

	badPlainSHA := bytes.Repeat([]byte{0x00}, sha256HashLength)
	err = cli.downloadAndDecryptToFile(ctx, url, mediaKey, MediaImage, unknownFileLength, encSHA, badPlainSHA, tempMediaFile(t))
	if !errors.Is(err, ErrInvalidMediaSHA256) {
		t.Errorf("hash divergente = %v, esperado ErrInvalidMediaSHA256", err)
	}

	// MAC adulterado precisa ser recusado antes de qualquer decriptacao.
	tampered := append([]byte{}, blob...)
	tampered[len(tampered)-1] ^= 0xFF
	tamperedHash := sha256.Sum256(tampered)
	tamperedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tampered)
	}))
	defer tamperedSrv.Close()
	tamperedCli := newMediaTestClient(t, tamperedSrv)
	err = tamperedCli.downloadAndDecryptToFile(
		ctx, url, mediaKey, MediaImage, unknownFileLength, tamperedHash[:], plainSHA, tempMediaFile(t))
	if !errors.Is(err, ErrInvalidMediaHMAC) {
		t.Errorf("MAC adulterado = %v, esperado ErrInvalidMediaHMAC", err)
	}
}

func TestDownloadEncryptedMediaToFileCortaOMAC(t *testing.T) {
	payload := bytes.Repeat([]byte{0xCC}, 64)
	blob := append(append([]byte{}, payload...), bytes.Repeat([]byte{0xDD}, mediaHMACLength)...)
	blobHash := sha256.Sum256(blob)
	body := blob

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)
	ctx := context.Background()
	const url = "https://mmg.whatsapp.net/x"

	file := tempMediaFile(t)
	mac, err := cli.downloadEncryptedMediaToFile(ctx, url, blobHash[:], file)
	if err != nil {
		t.Fatalf("downloadEncryptedMediaToFile devolveu erro: %v", err)
	}
	if !bytes.Equal(mac, blob[len(blob)-mediaHMACLength:]) {
		t.Error("o MAC extraido nao bate com o fim do blob")
	}
	if got := readAllFromStart(t, file); !bytes.Equal(got, payload) {
		t.Errorf("arquivo tem %d bytes, esperado %d (MAC truncado)", len(got), len(payload))
	}

	wrongHash := bytes.Repeat([]byte{0x00}, sha256HashLength)
	if _, err = cli.downloadEncryptedMediaToFile(ctx, url, wrongHash, tempMediaFile(t)); !errors.Is(err, ErrInvalidMediaEncSHA256) {
		t.Errorf("hash divergente = %v, esperado ErrInvalidMediaEncSHA256", err)
	}

	body = bytes.Repeat([]byte{0xCC}, mediaHMACLength) // curto demais
	shortHash := sha256.Sum256(body)
	if _, err = cli.downloadEncryptedMediaToFile(ctx, url, shortHash[:], tempMediaFile(t)); !errors.Is(err, ErrTooShortFile) {
		t.Errorf("arquivo curto = %v, esperado ErrTooShortFile", err)
	}
}

func TestValidateMediaFile(t *testing.T) {
	iv := bytes.Repeat([]byte{0x22}, mediaIVLength)
	macKey := bytes.Repeat([]byte{0x23}, mediaMACKeyLength)
	content := []byte("conteudo em arquivo")

	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(content)
	goodMAC := h.Sum(nil)[:mediaHMACLength]

	if err := validateMediaFile(bytes.NewReader(content), iv, macKey, goodMAC); err != nil {
		t.Fatalf("validateMediaFile com MAC correto devolveu erro: %v", err)
	}
	badMAC := append([]byte{}, goodMAC...)
	badMAC[0] ^= 0xFF
	if err := validateMediaFile(bytes.NewReader(content), iv, macKey, badMAC); !errors.Is(err, ErrInvalidMediaHMAC) {
		t.Fatalf("validateMediaFile com MAC adulterado = %v, esperado ErrInvalidMediaHMAC", err)
	}
}

func TestDownloadFBToFileUsaODirectPathDoTransporte(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x24}, mediaKeyLength)
	plaintext := []byte("midia FB direto para arquivo")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, MediaImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)
	file := tempMediaFile(t)

	transport := &waMediaTransport.WAMediaTransport_Integral{
		DirectPath:    proto.String("/v/fb-path"),
		MediaKey:      mediaKey,
		FileEncSHA256: encSHA,
		FileSHA256:    plainSHA,
	}
	if err := cli.DownloadFBToFile(context.Background(), transport, MediaImage, file); err != nil {
		t.Fatalf("DownloadFBToFile devolveu erro: %v", err)
	}
	if got := readAllFromStart(t, file); !bytes.Equal(got, plaintext) {
		t.Fatalf("arquivo = %q, esperado %q", got, plaintext)
	}
}
