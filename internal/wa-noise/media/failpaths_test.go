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

	"wa-api/internal/wa-noise/proto/waE2E"
	waLog "wa-api/internal/wa-noise/util/log"
)

// Este arquivo cobre os ramos de erro que so' aparecem quando o transporte HTTP
// ou o arquivo de destino falham — casos que nao da' para provocar com um
// httptest.Server bem comportado.

var errTransportBoom = errors.New("transporte falhou")

// errorRoundTripper falha toda requisicao, exercitando os ramos de erro de
// http.Client.Do sem depender de rede.
type errorRoundTripper struct{}

func (errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errTransportBoom
}

// newFailingTransport devolve um Transport cujo http sempre falha, com o cache
// de media connection ja' populado.
func newFailingTransport(hosts ...string) *fakeTransport {
	if len(hosts) == 0 {
		hosts = []string{"mmg.whatsapp.net"}
	}
	connHosts := make([]ConnHost, len(hosts))
	for i, h := range hosts {
		connHosts[i] = ConnHost{Hostname: h}
	}
	tr := &fakeTransport{
		httpClient: &http.Client{Transport: errorRoundTripper{}},
		log:        waLog.Noop,
		warnings:   true,
	}
	tr.conn.Set(&Conn{Auth: "a", TTL: 3600, FetchedAt: time.Now(), Hosts: connHosts})
	return tr
}

func TestDoDownloadRequestPropagaErroDoTransporte(t *testing.T) {
	tr := newFailingTransport()
	if _, err := DoDownloadRequest(context.Background(), tr, "https://mmg.whatsapp.net/x"); !errors.Is(err, errTransportBoom) {
		t.Fatalf("erro = %v, esperado o erro do transporte", err)
	}
}

func TestRawUploadPropagaErroDoTransporte(t *testing.T) {
	tr := newFailingTransport()
	var resp UploadResponse
	err := RawUpload(context.Background(), tr, bytes.NewReader(nil), 0, nil, TypeImage, false, &resp)
	if err == nil || !strings.Contains(err.Error(), "failed to execute request") {
		t.Fatalf("erro = %v, esperado falha ao executar a requisicao", err)
	}
}

func TestDeletePropagaErroDoTransporte(t *testing.T) {
	tr := newFailingTransport()
	err := Delete(context.Background(), tr, TypeHistory, "/v/x", nil, "")
	if err == nil || !strings.Contains(err.Error(), "failed to execute request") {
		t.Fatalf("erro = %v, esperado falha ao executar a requisicao", err)
	}
}

// Um hostname invalido faz a montagem da requisicao falhar antes do envio.
func TestRawUploadRejeitaHostInvalido(t *testing.T) {
	tr := newFailingTransport("host invalido\n")
	var resp UploadResponse
	err := RawUpload(context.Background(), tr, bytes.NewReader(nil), 0, nil, TypeImage, false, &resp)
	if err == nil || !strings.Contains(err.Error(), "failed to prepare request") {
		t.Fatalf("erro = %v, esperado falha ao montar a requisicao", err)
	}
}

func TestDeleteRejeitaHostInvalido(t *testing.T) {
	tr := newFailingTransport("host invalido\n")
	err := Delete(context.Background(), tr, TypeHistory, "/v/x", nil, "")
	if err == nil || !strings.Contains(err.Error(), "failed to prepare request") {
		t.Fatalf("erro = %v, esperado falha ao montar a requisicao", err)
	}
}

// failingFile permite ligar falhas seletivas em cada operacao de File.
type failingFile struct {
	*os.File
	failSeek     bool
	failReadAt   bool
	failTruncate bool
	failStat     bool
}

var errFileBoom = errors.New("arquivo falhou")

func (f *failingFile) Seek(offset int64, whence int) (int64, error) {
	if f.failSeek {
		return 0, errFileBoom
	}
	return f.File.Seek(offset, whence)
}

func (f *failingFile) ReadAt(p []byte, off int64) (int, error) {
	if f.failReadAt {
		return 0, errFileBoom
	}
	return f.File.ReadAt(p, off)
}

func (f *failingFile) Truncate(size int64) error {
	if f.failTruncate {
		return errFileBoom
	}
	return f.File.Truncate(size)
}

func (f *failingFile) Stat() (os.FileInfo, error) {
	if f.failStat {
		return nil, errFileBoom
	}
	return f.File.Stat()
}

func newFailingFile(t *testing.T) *failingFile {
	t.Helper()
	return &failingFile{File: tempMediaFile(t)}
}

// blobServer devolve sempre o mesmo corpo.
func blobServer(t *testing.T, blob []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
}

func TestDownloadEncryptedToFileFalhaAoLerOMAC(t *testing.T) {
	blob := append(bytes.Repeat([]byte{0xEE}, 64), bytes.Repeat([]byte{0xFF}, mediaHMACLength)...)
	hash := sha256.Sum256(blob)
	srv := blobServer(t, blob)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	file := newFailingFile(t)
	file.failReadAt = true
	if _, err := DownloadEncryptedToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", hash[:], file,
	); err == nil || !strings.Contains(err.Error(), "failed to read MAC from file") {
		t.Fatalf("erro = %v, esperado falha ao ler o MAC", err)
	}
}

func TestDownloadEncryptedToFileFalhaAoTruncar(t *testing.T) {
	blob := append(bytes.Repeat([]byte{0xEE}, 64), bytes.Repeat([]byte{0xFF}, mediaHMACLength)...)
	hash := sha256.Sum256(blob)
	srv := blobServer(t, blob)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	file := newFailingFile(t)
	file.failTruncate = true
	if _, err := DownloadEncryptedToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", hash[:], file,
	); err == nil || !strings.Contains(err.Error(), "failed to truncate file") {
		t.Fatalf("erro = %v, esperado falha ao truncar", err)
	}
}

func TestDownloadAndDecryptToFileFalhaAoRebobinarAposOMAC(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x30}, mediaKeyLength)
	plaintext := []byte("conteudo qualquer")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)
	srv := blobServer(t, blob)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	// seekAfterMAC falha somente a partir do N-esimo Seek, deixando o download
	// e a validacao de MAC acontecerem antes.
	file := &countingSeekFile{File: tempMediaFile(t), failFrom: 2}
	err := DownloadAndDecryptToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", mediaKey, TypeImage,
		UnknownFileLength, encSHA, plainSHA, file)
	if err == nil || !strings.Contains(err.Error(), "failed to seek to start of file") {
		t.Fatalf("erro = %v, esperado falha de seek", err)
	}
}

// Com Stat falhando desde o inicio, quem denuncia primeiro e' o proprio
// cbcutil.DecryptFile (que faz Stat antes de decifrar): o erro sai embrulhado
// como "failed to decrypt file".
func TestDownloadAndDecryptToFileFalhaNaDecriptacaoPorStat(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x31}, mediaKeyLength)
	plaintext := []byte("conteudo qualquer")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)
	srv := blobServer(t, blob)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	file := newFailingFile(t)
	file.failStat = true
	err := DownloadAndDecryptToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", mediaKey, TypeImage,
		UnknownFileLength, encSHA, plainSHA, file)
	if err == nil || !strings.Contains(err.Error(), "failed to decrypt file") {
		t.Fatalf("erro = %v, esperado falha de decriptacao", err)
	}
}

// phasedFile falha em Stat, Seek ou Read somente DEPOIS que o Stat interno de
// cbcutil.DecryptFile ja' aconteceu. E' o que permite alcancar os ramos de
// validacao pos-decriptacao de DownloadAndDecryptToFile.
type phasedFile struct {
	*os.File
	statCalls    int
	failStatFrom int
	failLateSeek bool
	failLateRead bool
}

func (f *phasedFile) late() bool { return f.statCalls >= 2 }

func (f *phasedFile) Stat() (os.FileInfo, error) {
	f.statCalls++
	if f.failStatFrom > 0 && f.statCalls >= f.failStatFrom {
		return nil, errFileBoom
	}
	return f.File.Stat()
}

func (f *phasedFile) Seek(offset int64, whence int) (int64, error) {
	if f.failLateSeek && f.late() {
		return 0, errFileBoom
	}
	return f.File.Seek(offset, whence)
}

func (f *phasedFile) Read(p []byte) (int, error) {
	if f.failLateRead && f.late() {
		return 0, errFileBoom
	}
	return f.File.Read(p)
}

// WriteTo precisa ser sobrescrito junto com Read: io.Copy prefere o WriteTo do
// *os.File e passaria por cima do Read acima.
func (f *phasedFile) WriteTo(w io.Writer) (int64, error) {
	if f.failLateRead && f.late() {
		return 0, errFileBoom
	}
	return f.File.WriteTo(w)
}

func newPhasedFile(t *testing.T) *phasedFile {
	t.Helper()
	return &phasedFile{File: tempMediaFile(t)}
}

// runPhased executa um download+decriptacao valido com o phasedFile dado.
func runPhased(t *testing.T, file File) error {
	t.Helper()
	mediaKey := bytes.Repeat([]byte{0x32}, mediaKeyLength)
	plaintext := []byte("conteudo para validacao pos-decriptacao")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)
	srv := blobServer(t, blob)
	defer srv.Close()
	tr := newTestTransport(t, srv)
	return DownloadAndDecryptToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", mediaKey, TypeImage,
		UnknownFileLength, encSHA, plainSHA, file)
}

func TestDownloadAndDecryptToFileFalhaNoStatDeValidacao(t *testing.T) {
	file := newPhasedFile(t)
	file.failStatFrom = 2
	if err := runPhased(t, file); err == nil || !strings.Contains(err.Error(), "failed to stat file") {
		t.Fatalf("erro = %v, esperado falha no stat de validacao", err)
	}
}

func TestDownloadAndDecryptToFileFalhaAoRebobinarAposDecriptar(t *testing.T) {
	file := newPhasedFile(t)
	file.failLateSeek = true
	if err := runPhased(t, file); err == nil ||
		!strings.Contains(err.Error(), "failed to seek to start of file after decrypting") {
		t.Fatalf("erro = %v, esperado falha ao rebobinar apos decriptar", err)
	}
}

func TestDownloadAndDecryptToFileFalhaAoHashearAposDecriptar(t *testing.T) {
	file := newPhasedFile(t)
	file.failLateRead = true
	if err := runPhased(t, file); err == nil || !strings.Contains(err.Error(), "failed to hash file") {
		t.Fatalf("erro = %v, esperado falha ao hashear apos decriptar", err)
	}
}

// countingSeekFile falha no Seek a partir da chamada failFrom (1-based).
type countingSeekFile struct {
	*os.File
	seeks    int
	failFrom int
}

func (f *countingSeekFile) Seek(offset int64, whence int) (int64, error) {
	f.seeks++
	if f.failFrom > 0 && f.seeks >= f.failFrom {
		return 0, errFileBoom
	}
	return f.File.Seek(offset, whence)
}

// Entre tentativas, o laco rebobina o arquivo; se o Seek falhar, o erro sobe.
func TestDownloadPossiblyEncryptedWithRetriesToFileFalhaAoRebobinar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	file := newFailingFile(t)
	file.failSeek = true
	_, err := DownloadPossiblyEncryptedWithRetriesToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", nil, file)
	if err == nil || !strings.Contains(err.Error(), "failed to seek to start of file to retry download") {
		t.Fatalf("erro = %v, esperado falha ao rebobinar para nova tentativa", err)
	}
}

// UploadReader propaga a falha do arquivo temporario ao rebobinar.
func TestUploadReaderFalhaAoRebobinarOTempFile(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	tempFile := &countingSeekFile{File: tempMediaFile(t), failFrom: 1}
	_, err := UploadReader(context.Background(), tr, bytes.NewReader([]byte("dados")), tempFile, TypeImage)
	if err == nil || !strings.Contains(err.Error(), "failed to seek to start of temporary file") {
		t.Fatalf("erro = %v, esperado falha ao rebobinar o arquivo temporario", err)
	}
}

// UploadReader propaga a falha da cifragem em stream.
func TestUploadReaderFalhaAoCifrar(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	_, err := UploadReader(context.Background(), tr, failingReader{}, tempMediaFile(t), TypeImage)
	if err == nil || !strings.Contains(err.Error(), "failed to encrypt file") {
		t.Fatalf("erro = %v, esperado falha ao cifrar", err)
	}
}

// failingReader falha em qualquer leitura.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errFileBoom }

// ValidateMediaFile propaga a falha de leitura do arquivo.
func TestValidateMediaFileFalhaAoLer(t *testing.T) {
	iv := bytes.Repeat([]byte{0x40}, mediaIVLength)
	macKey := bytes.Repeat([]byte{0x41}, mediaMACKeyLength)
	err := ValidateMediaFile(readErrorSeeker{}, iv, macKey, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to hash file") {
		t.Fatalf("erro = %v, esperado falha ao hashear o arquivo", err)
	}
}

// readErrorSeeker rebobina bem, mas falha na leitura.
type readErrorSeeker struct{}

func (readErrorSeeker) Read([]byte) (int, error)       { return 0, errFileBoom }
func (readErrorSeeker) Seek(int64, int) (int64, error) { return 0, nil }

var _ io.ReadSeeker = readErrorSeeker{}

// Com TMPDIR apontando para um diretorio inexistente, a criacao do arquivo
// temporario interno de UploadReader falha.
func TestUploadReaderFalhaAoCriarTempFile(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "nao-existe"))
	_, err := UploadReader(context.Background(), tr, bytes.NewReader([]byte("x")), nil, TypeImage)
	if err == nil || !strings.Contains(err.Error(), "failed to create temporary file") {
		t.Fatalf("erro = %v, esperado falha ao criar o arquivo temporario", err)
	}
}

// Mensagem sem URL mas com directPath baixa pela media connection (o ramo que
// DownloadAny/DownloadMessage compartilham com DownloadWithPath).
func TestDownloadMessagePeloDirectPath(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x50}, mediaKeyLength)
	plaintext := []byte("baixado pelo direct path")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)
	srv := blobServer(t, blob)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	msg := &waE2E.ImageMessage{
		DirectPath:    proto.String("/v/direct"),
		MediaKey:      mediaKey,
		FileEncSHA256: encSHA,
		FileSHA256:    plainSHA,
		FileLength:    proto.Uint64(uint64(len(plaintext))),
	}
	got, err := DownloadMessage(context.Background(), tr, msg)
	if err != nil {
		t.Fatalf("DownloadMessage devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("conteudo = %q, esperado %q", got, plaintext)
	}

	file := tempMediaFile(t)
	if err = DownloadMessageToFile(context.Background(), tr, msg, file); err != nil {
		t.Fatalf("DownloadMessageToFile devolveu erro: %v", err)
	}
	if data := readAllFromStart(t, file); !bytes.Equal(data, plaintext) {
		t.Fatalf("arquivo = %q, esperado %q", data, plaintext)
	}
}

// Ciphertext com tamanho invalido para CBC faz a decriptacao em arquivo falhar
// depois de o MAC ja' ter validado.
func TestDownloadAndDecryptToFileFalhaNaDecriptacao(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x51}, mediaKeyLength)
	iv, _, macKey, _ := GetKeys(mediaKey, TypeImage)
	ciphertext := bytes.Repeat([]byte{0x00}, 7) // nao e' multiplo do bloco AES
	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(ciphertext)
	blob := append(append([]byte{}, ciphertext...), h.Sum(nil)[:mediaHMACLength]...)
	blobHash := sha256.Sum256(blob)

	srv := blobServer(t, blob)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	err := DownloadAndDecryptToFile(
		context.Background(), tr, "https://mmg.whatsapp.net/x", mediaKey, TypeImage,
		UnknownFileLength, blobHash[:], nil, tempMediaFile(t))
	if err == nil || !strings.Contains(err.Error(), "failed to decrypt file") {
		t.Fatalf("erro = %v, esperado falha de decriptacao", err)
	}
}

// Contexto cancelado durante o backoff aborta o laco da versao em arquivo.
func TestDownloadPossiblyEncryptedWithRetriesToFileRespeitaContexto(t *testing.T) {
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
	_, err := DownloadPossiblyEncryptedWithRetriesToFile(ctx, tr, "https://mmg.whatsapp.net/x", nil, tempMediaFile(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, esperado context.Canceled", err)
	}
}
