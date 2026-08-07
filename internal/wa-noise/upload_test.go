// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"wa-api/internal/wa-noise/socket"
)

type capturedUpload struct {
	path   string
	query  url.Values
	body   []byte
	origin string
	host   string
	method string
}

func uploadServer(t *testing.T, captured *capturedUpload, respBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured.path = r.URL.Path
		captured.query = r.URL.Query()
		captured.body = body
		captured.origin = r.Header.Get("Origin")
		captured.host = r.Header.Get(testOrigHostHeader)
		captured.method = r.Method
		_, _ = w.Write([]byte(respBody))
	}))
}

const uploadOKResponse = `{"url":"https://mmg.whatsapp.net/x","direct_path":"/v/x","handle":"h1","object_id":"o1"}`

func TestUploadCifraEPreencheAResposta(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	plaintext := []byte("um anexo qualquer para subir")
	resp, err := cli.Upload(context.Background(), plaintext, MediaImage)
	if err != nil {
		t.Fatalf("Upload devolveu erro: %v", err)
	}

	if resp.FileLength != uint64(len(plaintext)) {
		t.Errorf("FileLength = %d, esperado %d", resp.FileLength, len(plaintext))
	}
	if len(resp.MediaKey) != mediaKeyLength {
		t.Errorf("len(MediaKey) = %d, esperado %d", len(resp.MediaKey), mediaKeyLength)
	}
	plainHash := sha256.Sum256(plaintext)
	if !bytes.Equal(resp.FileSHA256, plainHash[:]) {
		t.Error("FileSHA256 nao e' o hash do plaintext")
	}
	encHash := sha256.Sum256(got.body)
	if !bytes.Equal(resp.FileEncSHA256, encHash[:]) {
		t.Error("FileEncSHA256 nao e' o hash do corpo efetivamente enviado")
	}
	if resp.DirectPath != "/v/x" || resp.Handle != "h1" || resp.ObjectID != "o1" {
		t.Errorf("resposta do servidor nao foi decodificada: %+v", resp)
	}

	// O corpo enviado precisa ser decriptavel de volta ao plaintext.
	roundTrip := func() []byte {
		iv, cipherKey, macKey, _ := getMediaKeys(resp.MediaKey, MediaImage)
		if err := validateMedia(iv, got.body[:len(got.body)-mediaHMACLength], macKey, got.body[len(got.body)-mediaHMACLength:]); err != nil {
			t.Fatalf("o HMAC do corpo enviado nao valida: %v", err)
		}
		data, err := decryptForTest(cipherKey, iv, got.body[:len(got.body)-mediaHMACLength])
		if err != nil {
			t.Fatalf("falha ao decriptar o corpo enviado: %v", err)
		}
		return data
	}
	if !bytes.Equal(roundTrip(), plaintext) {
		t.Error("o corpo enviado nao volta ao plaintext original")
	}

	if got.method != http.MethodPost {
		t.Errorf("metodo = %s, esperado POST", got.method)
	}
	if got.origin != socket.Origin {
		t.Errorf("Origin = %q, esperado %q", got.origin, socket.Origin)
	}
	token := base64.URLEncoding.EncodeToString(resp.FileEncSHA256)
	wantPath := "/" + uploadPrefixDefault + "/" + mediaTypeToMMSType[MediaImage] + "/" + token
	if got.path != wantPath {
		t.Errorf("path = %q, esperado %q", got.path, wantPath)
	}
	if got.query.Get("auth") != "test-auth" || got.query.Get("token") != token {
		t.Errorf("query = %v, esperado auth+token da mediaConn", got.query)
	}
}

func TestUploadReaderProduzOMesmoContrato(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	plaintext := []byte("conteudo lido de um reader")
	resp, err := cli.UploadReader(context.Background(), bytes.NewReader(plaintext), nil, MediaDocument)
	if err != nil {
		t.Fatalf("UploadReader devolveu erro: %v", err)
	}
	if resp.FileLength != uint64(len(plaintext)) {
		t.Errorf("FileLength = %d, esperado %d", resp.FileLength, len(plaintext))
	}
	plainHash := sha256.Sum256(plaintext)
	if !bytes.Equal(resp.FileSHA256, plainHash[:]) {
		t.Error("FileSHA256 nao e' o hash do plaintext")
	}
	if !strings.Contains(got.path, mediaTypeToMMSType[MediaDocument]) {
		t.Errorf("path = %q, esperado conter o mms-type de documento", got.path)
	}
}

func TestUploadNewsletterNaoCifraEUsaOPrefixoDeNewsletter(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	data := []byte("midia de newsletter em claro")
	resp, err := cli.UploadNewsletter(context.Background(), data, MediaImage)
	if err != nil {
		t.Fatalf("UploadNewsletter devolveu erro: %v", err)
	}
	if !bytes.Equal(got.body, data) {
		t.Error("o corpo enviado foi alterado — newsletter nao deve cifrar")
	}
	if resp.MediaKey != nil || resp.FileEncSHA256 != nil {
		t.Error("newsletter nao deve produzir MediaKey nem FileEncSHA256")
	}
	hash := sha256.Sum256(data)
	if !bytes.Equal(resp.FileSHA256, hash[:]) {
		t.Error("FileSHA256 nao e' o hash do conteudo")
	}
	wantPrefix := "/" + uploadPrefixNewsletter + "/newsletter-" + mediaTypeToMMSType[MediaImage] + "/"
	if !strings.HasPrefix(got.path, wantPrefix) {
		t.Errorf("path = %q, esperado prefixo %q", got.path, wantPrefix)
	}
}

func TestUploadNewsletterReaderRebobinaAntesDeEnviar(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	data := []byte("conteudo lido duas vezes: hash e envio")
	resp, err := cli.UploadNewsletterReader(context.Background(), bytes.NewReader(data), MediaImage)
	if err != nil {
		t.Fatalf("UploadNewsletterReader devolveu erro: %v", err)
	}
	if !bytes.Equal(got.body, data) {
		t.Errorf("corpo enviado = %q, esperado %q (reader nao rebobinou)", got.body, data)
	}
	if resp.FileLength != uint64(len(data)) {
		t.Errorf("FileLength = %d, esperado %d", resp.FileLength, len(data))
	}
}

func TestUploadPropagaStatusDeErro(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	_, err := cli.Upload(context.Background(), []byte("x"), MediaImage)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("erro = %v, esperado mencao ao status 403", err)
	}
}

func TestDeleteMediaMontaAURLDeDelecao(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, "")
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	encFileHash := bytes.Repeat([]byte{0x09}, sha256HashLength)
	// O directPath vem com query string; DeleteMedia precisa corta-la antes de
	// codificar em d_md.
	err := cli.DeleteMedia(context.Background(), MediaHistory, "/v/t62/abc?ccb=11-4", encFileHash, "handle-1")
	if err != nil {
		t.Fatalf("DeleteMedia devolveu erro: %v", err)
	}

	if got.method != http.MethodDelete {
		t.Errorf("metodo = %s, esperado DELETE", got.method)
	}
	token := base64.URLEncoding.EncodeToString(encFileHash)
	wantPath := "/mms/" + mediaTypeToMMSType[MediaHistory] + "/" + token
	if got.path != wantPath {
		t.Errorf("path = %q, esperado %q", got.path, wantPath)
	}
	wantDMD := base64.RawURLEncoding.EncodeToString([]byte("/v/t62/abc"))
	if got.query.Get("d_md") != wantDMD {
		t.Errorf("d_md = %q, esperado %q (query string do directPath removida)", got.query.Get("d_md"), wantDMD)
	}
	if got.query.Get("e_handle") != "handle-1" {
		t.Errorf("e_handle = %q, esperado \"handle-1\"", got.query.Get("e_handle"))
	}
}

func TestDeleteMediaOmiteEHandleQuandoVazio(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, "")
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	err := cli.DeleteMedia(context.Background(), MediaHistory, "/v/x", bytes.Repeat([]byte{0x0A}, sha256HashLength), "")
	if err != nil {
		t.Fatalf("DeleteMedia devolveu erro: %v", err)
	}
	if _, ok := got.query["e_handle"]; ok {
		t.Error("e_handle vazio nao deveria ir na query")
	}
}

func TestDeleteMediaPropagaStatusDeErro(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	err := cli.DeleteMedia(context.Background(), MediaHistory, "/v/x", nil, "")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("erro = %v, esperado mencao ao status 500", err)
	}
}
