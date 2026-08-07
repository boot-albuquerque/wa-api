// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package media

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
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMediaTransport"
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

func TestDownloadWithPathToFileRoundTrip(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x20}, mediaKeyLength)
	plaintext := bytes.Repeat([]byte("streaming de midia para disco. "), 40)
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeVideo)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	file := tempMediaFile(t)

	err := DownloadWithPathToFile(
		context.Background(), tr, "/v/x", encSHA, plainSHA, mediaKey, len(plaintext), TypeVideo, "", file)
	if err != nil {
		t.Fatalf("DownloadWithPathToFile devolveu erro: %v", err)
	}
	if got := readAllFromStart(t, file); !bytes.Equal(got, plaintext) {
		t.Fatalf("arquivo tem %d bytes, esperado o plaintext de %d bytes", len(got), len(plaintext))
	}
}

func TestDownloadWithPathToFilePropagaErroDaMediaConn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	tr.conn.Set(nil)

	err := DownloadWithPathToFile(
		context.Background(), tr, "/v/x", nil, nil, nil, UnknownFileLength, TypeImage, "", tempMediaFile(t))
	if err == nil || !strings.Contains(err.Error(), "failed to refresh media connections") {
		t.Fatalf("erro = %v, esperado embrulho de falha na mediaConn", err)
	}
}

func TestDownloadWithPathToFileTrocaDeHost(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x25}, mediaKeyLength)
	plaintext := []byte("segundo host responde")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(testOrigHostHeader) == "quebrado.example" {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv, "quebrado.example", "bom.example")
	file := tempMediaFile(t)

	err := DownloadWithPathToFile(
		context.Background(), tr, "/v/x", encSHA, plainSHA, mediaKey, len(plaintext), TypeImage, "image", file)
	if err != nil {
		t.Fatalf("DownloadWithPathToFile devolveu erro: %v", err)
	}
	if got := readAllFromStart(t, file); !bytes.Equal(got, plaintext) {
		t.Fatalf("arquivo = %q, esperado %q", got, plaintext)
	}
}

func TestDownloadWithPathToFileFalhaNoUltimoHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv, "a.example", "b.example")

	err := DownloadWithPathToFile(
		context.Background(), tr, "/v/x", nil, nil, bytes.Repeat([]byte{0x01}, mediaKeyLength),
		UnknownFileLength, TypeImage, "image", tempMediaFile(t))
	if err == nil || !strings.Contains(err.Error(), "from last host") {
		t.Fatalf("erro = %v, esperado falha no ultimo host", err)
	}
}

func TestDownloadWithPathToFileSemHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	tr.conn.Set(&Conn{TTL: 3600, FetchedAt: time.Now()})

	if err := DownloadWithPathToFile(
		context.Background(), tr, "/v/x", nil, nil, nil, UnknownFileLength, TypeImage, "image", tempMediaFile(t),
	); err != nil {
		t.Fatalf("err = %v, esperado nil com lista de hosts vazia", err)
	}
}

func TestDownloadMessageToFileRejeitaMensagemSemURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	file := tempMediaFile(t)

	if err := DownloadMessageToFile(context.Background(), tr, &waE2E.ImageMessage{}, file); !errors.Is(err, ErrNoURLPresent) {
		t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
	}
	msg := &waE2E.ImageMessage{URL: proto.String(webWhatsappNetURLPrefix + "/x")}
	if err := DownloadMessageToFile(context.Background(), tr, msg, file); !errors.Is(err, ErrNoURLPresent) {
		t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
	}
	if err := DownloadMessageToFile(context.Background(), tr, notDownloadable{}, file); !errors.Is(err, ErrUnknownMediaType) {
		t.Fatalf("erro = %v, esperado ErrUnknownMediaType", err)
	}
}

// Mensagem com URL direta escreve no arquivo sem passar pela media connection.
func TestDownloadMessageToFileUsaAURLDireta(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x26}, mediaKeyLength)
	plaintext := []byte("arquivo pela URL direta")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	tr.conn.Set(nil)
	file := tempMediaFile(t)

	msg := &waE2E.ImageMessage{
		URL:           proto.String("https://mmg.whatsapp.net/direta"),
		MediaKey:      mediaKey,
		FileEncSHA256: encSHA,
		FileSHA256:    plainSHA,
		FileLength:    proto.Uint64(uint64(len(plaintext))),
	}
	if err := DownloadMessageToFile(context.Background(), tr, msg, file); err != nil {
		t.Fatalf("DownloadMessageToFile devolveu erro: %v", err)
	}
	if got := readAllFromStart(t, file); !bytes.Equal(got, plaintext) {
		t.Fatalf("arquivo = %q, esperado %q", got, plaintext)
	}
}

func TestDownloadAndDecryptToFileDetectaTamanhoEHashErrados(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x21}, mediaKeyLength)
	plaintext := []byte("conteudo com validacao de tamanho e hash")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	ctx := context.Background()
	const url = "https://mmg.whatsapp.net/x"

	err := DownloadAndDecryptToFile(ctx, tr, url, mediaKey, TypeImage, len(plaintext)+1, encSHA, plainSHA, tempMediaFile(t))
	if !errors.Is(err, ErrFileLengthMismatch) {
		t.Errorf("tamanho divergente = %v, esperado ErrFileLengthMismatch", err)
	}

	// Com os avisos desligados o mesmo download passa.
	tr.warnings = false
	if err = DownloadAndDecryptToFile(ctx, tr, url, mediaKey, TypeImage, len(plaintext)+1, encSHA, plainSHA, tempMediaFile(t)); err != nil {
		t.Errorf("com ReturnDownloadWarnings=false nao deveria haver erro, veio %v", err)
	}
	tr.warnings = true

	badPlainSHA := bytes.Repeat([]byte{0x00}, sha256HashLength)
	err = DownloadAndDecryptToFile(ctx, tr, url, mediaKey, TypeImage, UnknownFileLength, encSHA, badPlainSHA, tempMediaFile(t))
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
	tamperedTr := newTestTransport(t, tamperedSrv)
	err = DownloadAndDecryptToFile(
		ctx, tamperedTr, url, mediaKey, TypeImage, UnknownFileLength, tamperedHash[:], plainSHA, tempMediaFile(t))
	if !errors.Is(err, ErrInvalidMediaHMAC) {
		t.Errorf("MAC adulterado = %v, esperado ErrInvalidMediaHMAC", err)
	}
}

// Midia nao cifrada para arquivo sai sem validacao nenhuma.
func TestDownloadAndDecryptToFileMidiaNaoCifrada(t *testing.T) {
	plaintext := []byte("cru em arquivo")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(plaintext)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	file := tempMediaFile(t)

	if err := DownloadAndDecryptToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", nil, TypeImage, UnknownFileLength, nil, nil, file,
	); err != nil {
		t.Fatalf("DownloadAndDecryptToFile devolveu erro: %v", err)
	}
	if got := readAllFromStart(t, file); !bytes.Equal(got, plaintext) {
		t.Fatalf("arquivo = %q, esperado %q", got, plaintext)
	}
}

func TestDownloadAndDecryptToFilePropagaErroDeDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	err := DownloadAndDecryptToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", nil, TypeImage,
		UnknownFileLength, bytes.Repeat([]byte{0x00}, sha256HashLength), nil, tempMediaFile(t))
	if !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("erro = %v, esperado ErrMediaDownloadFailedWith404", err)
	}
}

func TestDownloadEncryptedToFileCortaOMAC(t *testing.T) {
	payload := bytes.Repeat([]byte{0xCC}, 64)
	blob := append(append([]byte{}, payload...), bytes.Repeat([]byte{0xDD}, mediaHMACLength)...)
	blobHash := sha256.Sum256(blob)
	body := blob

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	ctx := context.Background()
	const url = "https://mmg.whatsapp.net/x"

	file := tempMediaFile(t)
	mac, err := DownloadEncryptedToFile(ctx, tr, url, blobHash[:], file)
	if err != nil {
		t.Fatalf("DownloadEncryptedToFile devolveu erro: %v", err)
	}
	if !bytes.Equal(mac, blob[len(blob)-mediaHMACLength:]) {
		t.Error("o MAC extraido nao bate com o fim do blob")
	}
	if got := readAllFromStart(t, file); !bytes.Equal(got, payload) {
		t.Errorf("arquivo tem %d bytes, esperado %d (MAC truncado)", len(got), len(payload))
	}

	wrongHash := bytes.Repeat([]byte{0x00}, sha256HashLength)
	if _, err = DownloadEncryptedToFile(ctx, tr, url, wrongHash, tempMediaFile(t)); !errors.Is(err, ErrInvalidMediaEncSHA256) {
		t.Errorf("hash divergente = %v, esperado ErrInvalidMediaEncSHA256", err)
	}

	body = bytes.Repeat([]byte{0xCC}, mediaHMACLength) // curto demais
	shortHash := sha256.Sum256(body)
	if _, err = DownloadEncryptedToFile(ctx, tr, url, shortHash[:], tempMediaFile(t)); !errors.Is(err, ErrTooShortFile) {
		t.Errorf("arquivo curto = %v, esperado ErrTooShortFile", err)
	}
}

func TestDownloadEncryptedToFilePropagaErroDeDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	if _, err := DownloadEncryptedToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", nil, tempMediaFile(t),
	); !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("erro = %v, esperado ErrMediaDownloadFailedWith404", err)
	}
}

// DownloadRawToFile aceita qualquer io.Writer, nao so' *os.File — nesse caso o
// fallocate e' pulado.
func TestDownloadRawToFileEmWriterQualquer(t *testing.T) {
	body := []byte("conteudo cru")
	want := sha256.Sum256(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	var buf bytes.Buffer
	n, hash, err := DownloadRawToFile(context.Background(), tr, "https://mmg.whatsapp.net/x", &buf)
	if err != nil {
		t.Fatalf("DownloadRawToFile devolveu erro: %v", err)
	}
	if n != int64(len(body)) || !bytes.Equal(buf.Bytes(), body) {
		t.Fatalf("n = %d, conteudo = %q", n, buf.Bytes())
	}
	if !bytes.Equal(hash, want[:]) {
		t.Error("o hash devolvido nao bate com o SHA-256 do corpo")
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

	if err := ValidateMediaFile(bytes.NewReader(content), iv, macKey, goodMAC); err != nil {
		t.Fatalf("ValidateMediaFile com MAC correto devolveu erro: %v", err)
	}
	badMAC := append([]byte{}, goodMAC...)
	badMAC[0] ^= 0xFF
	if err := ValidateMediaFile(bytes.NewReader(content), iv, macKey, badMAC); !errors.Is(err, ErrInvalidMediaHMAC) {
		t.Fatalf("ValidateMediaFile com MAC adulterado = %v, esperado ErrInvalidMediaHMAC", err)
	}
	// Um ReadSeeker que falha ao rebobinar vira erro explicito.
	if err := ValidateMediaFile(failingSeeker{}, iv, macKey, goodMAC); err == nil ||
		!strings.Contains(err.Error(), "failed to seek to start of file") {
		t.Fatalf("erro = %v, esperado falha de seek", err)
	}
}

// failingSeeker falha em qualquer Seek, para exercitar o ramo de erro.
type failingSeeker struct{}

func (failingSeeker) Read([]byte) (int, error)          { return 0, io.EOF }
func (failingSeeker) Seek(int64, int) (int64, error)    { return 0, errSeekFailed }
func (failingSeeker) Write([]byte) (int, error)         { return 0, errSeekFailed }
func (failingSeeker) ReadAt([]byte, int64) (int, error) { return 0, errSeekFailed }
func (failingSeeker) WriteAt([]byte, int64) (int, error) {
	return 0, errSeekFailed
}
func (failingSeeker) Truncate(int64) error       { return errSeekFailed }
func (failingSeeker) Stat() (os.FileInfo, error) { return nil, errSeekFailed }
func (failingSeeker) Close() error               { return nil }

var errSeekFailed = errors.New("seek falhou")

func TestDownloadFBToFileUsaODirectPathDoTransporte(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x24}, mediaKeyLength)
	plaintext := []byte("midia FB direto para arquivo")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	file := tempMediaFile(t)

	transport := &waMediaTransport.WAMediaTransport_Integral{
		DirectPath:    proto.String("/v/fb-path"),
		MediaKey:      mediaKey,
		FileEncSHA256: encSHA,
		FileSHA256:    plainSHA,
	}
	if err := DownloadFBToFile(context.Background(), tr, transport, TypeImage, file); err != nil {
		t.Fatalf("DownloadFBToFile devolveu erro: %v", err)
	}
	if got := readAllFromStart(t, file); !bytes.Equal(got, plaintext) {
		t.Fatalf("arquivo = %q, esperado %q", got, plaintext)
	}
}
