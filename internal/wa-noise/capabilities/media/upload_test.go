package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"wa-api/internal/wa-noise/protocol/socket"
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
	tr := newTestTransport(t, srv)

	plaintext := []byte("um anexo qualquer para subir")
	resp, err := Upload(context.Background(), tr, plaintext, TypeImage)
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
		iv, cipherKey, macKey, _ := GetKeys(resp.MediaKey, TypeImage)
		if err := ValidateMedia(iv, got.body[:len(got.body)-mediaHMACLength], macKey, got.body[len(got.body)-mediaHMACLength:]); err != nil {
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
	wantPath := "/" + uploadPrefixDefault + "/" + mediaTypeToMMSType[TypeImage] + "/" + token
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
	tr := newTestTransport(t, srv)

	plaintext := []byte("conteudo lido de um reader")
	resp, err := UploadReader(context.Background(), tr, bytes.NewReader(plaintext), nil, TypeDocument)
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
	if !strings.Contains(got.path, mediaTypeToMMSType[TypeDocument]) {
		t.Errorf("path = %q, esperado conter o mms-type de documento", got.path)
	}
}

// Passando um tempFile explicito, o arquivo temporario interno nao e' criado e
// o conteudo cifrado fica no arquivo do chamador.
func TestUploadReaderComTempFileExplicito(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	tempFile, err := os.CreateTemp(t.TempDir(), "upload-*")
	if err != nil {
		t.Fatalf("falha ao criar arquivo temporario: %v", err)
	}
	defer func() { _ = tempFile.Close() }()

	plaintext := []byte("conteudo com arquivo temporario do chamador")
	resp, err := UploadReader(context.Background(), tr, bytes.NewReader(plaintext), tempFile, TypeImage)
	if err != nil {
		t.Fatalf("UploadReader devolveu erro: %v", err)
	}
	if resp.FileLength != uint64(len(plaintext)) {
		t.Errorf("FileLength = %d, esperado %d", resp.FileLength, len(plaintext))
	}
	if len(got.body) == 0 {
		t.Error("nada foi enviado ao servidor")
	}
}

func TestUploadNewsletterNaoCifraEUsaOPrefixoDeNewsletter(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	data := []byte("midia de newsletter em claro")
	resp, err := UploadNewsletter(context.Background(), tr, data, TypeImage)
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
	wantPrefix := "/" + uploadPrefixNewsletter + "/newsletter-" + mediaTypeToMMSType[TypeImage] + "/"
	if !strings.HasPrefix(got.path, wantPrefix) {
		t.Errorf("path = %q, esperado prefixo %q", got.path, wantPrefix)
	}
}

func TestUploadNewsletterReaderRebobinaAntesDeEnviar(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	data := []byte("conteudo lido duas vezes: hash e envio")
	resp, err := UploadNewsletterReader(context.Background(), tr, bytes.NewReader(data), TypeImage)
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

// Um ReadSeeker que falha ao rebobinar produz erro explicito.
func TestUploadNewsletterReaderFalhaAoRebobinar(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, uploadOKResponse)
	defer srv.Close()
	tr := newTestTransport(t, srv)

	_, err := UploadNewsletterReader(context.Background(), tr, failingSeeker{}, TypeImage)
	if err == nil || !strings.Contains(err.Error(), "failed to seek to start of data") {
		t.Fatalf("erro = %v, esperado falha de seek", err)
	}
}

// Cliente Messenger muda prefixo, mms-type de audio e escolha de host.
func TestResolveUploadTarget(t *testing.T) {
	conn := &Conn{Hosts: []ConnHost{{Hostname: "primeiro"}, {Hostname: "ultimo"}}}

	t.Run("padrao usa o primeiro host", func(t *testing.T) {
		tr := &fakeTransport{}
		target := resolveUploadTarget(tr, conn, TypeAudio, false)
		if target.host != "primeiro" || target.prefix != uploadPrefixDefault || target.mmsType != mmsTypeAudio {
			t.Fatalf("target = %+v", target)
		}
	})
	t.Run("messenger usa o ultimo host e troca audio por ptt", func(t *testing.T) {
		tr := &fakeTransport{messenger: true}
		target := resolveUploadTarget(tr, conn, TypeAudio, false)
		if target.host != "ultimo" || target.prefix != uploadPrefixMessenger || target.mmsType != mmsTypePTT {
			t.Fatalf("target = %+v", target)
		}
	})
	t.Run("messenger nao troca mms-type de imagem", func(t *testing.T) {
		tr := &fakeTransport{messenger: true}
		if target := resolveUploadTarget(tr, conn, TypeImage, false); target.mmsType != "image" {
			t.Fatalf("mmsType = %q, esperado \"image\"", target.mmsType)
		}
	})
	t.Run("newsletter prefixa o mms-type", func(t *testing.T) {
		tr := &fakeTransport{}
		target := resolveUploadTarget(tr, conn, TypeImage, true)
		if target.prefix != uploadPrefixNewsletter || target.mmsType != "newsletter-image" {
			t.Fatalf("target = %+v", target)
		}
	})
}

func TestUploadPropagaStatusDeErro(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	_, err := Upload(context.Background(), tr, []byte("x"), TypeImage)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("erro = %v, esperado mencao ao status 403", err)
	}
}

func TestUploadRejeitaRespostaNaoJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("nao e json"))
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	_, err := Upload(context.Background(), tr, []byte("x"), TypeImage)
	if err == nil || !strings.Contains(err.Error(), "failed to parse upload response") {
		t.Fatalf("erro = %v, esperado falha de parse da resposta", err)
	}
}

func TestRawUploadPropagaErroDaMediaConn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	tr.conn.Set(nil)

	var resp UploadResponse
	err := RawUpload(context.Background(), tr, bytes.NewReader(nil), 0, nil, TypeImage, false, &resp)
	if err == nil || !strings.Contains(err.Error(), "failed to refresh media connections") {
		t.Fatalf("erro = %v, esperado embrulho de falha na mediaConn", err)
	}
}

func TestDeleteMontaAURLDeDelecao(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, "")
	defer srv.Close()
	tr := newTestTransport(t, srv)

	encFileHash := bytes.Repeat([]byte{0x09}, sha256HashLength)
	// O directPath vem com query string; Delete precisa corta-la antes de
	// codificar em d_md.
	err := Delete(context.Background(), tr, TypeHistory, "/v/t62/abc?ccb=11-4", encFileHash, "handle-1")
	if err != nil {
		t.Fatalf("Delete devolveu erro: %v", err)
	}

	if got.method != http.MethodDelete {
		t.Errorf("metodo = %s, esperado DELETE", got.method)
	}
	token := base64.URLEncoding.EncodeToString(encFileHash)
	wantPath := "/mms/" + mediaTypeToMMSType[TypeHistory] + "/" + token
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

func TestDeleteOmiteEHandleQuandoVazio(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got, "")
	defer srv.Close()
	tr := newTestTransport(t, srv)

	err := Delete(context.Background(), tr, TypeHistory, "/v/x", bytes.Repeat([]byte{0x0A}, sha256HashLength), "")
	if err != nil {
		t.Fatalf("Delete devolveu erro: %v", err)
	}
	if _, ok := got.query["e_handle"]; ok {
		t.Error("e_handle vazio nao deveria ir na query")
	}
}

func TestDeletePropagaStatusDeErro(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	err := Delete(context.Background(), tr, TypeHistory, "/v/x", nil, "")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("erro = %v, esperado mencao ao status 500", err)
	}
}

func TestDeletePropagaErroDaMediaConn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	tr.conn.Set(nil)

	err := Delete(context.Background(), tr, TypeHistory, "/v/x", nil, "")
	if err == nil || !strings.Contains(err.Error(), "failed to refresh media connections") {
		t.Fatalf("erro = %v, esperado embrulho de falha na mediaConn", err)
	}
}
