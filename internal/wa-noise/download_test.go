// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waMediaTransport"
)

func TestDownloadAnyEscolheAPrimeiraParteNaoNula(t *testing.T) {
	var nilClient *Client

	if _, err := nilClient.DownloadAny(context.Background(), nil); !errors.Is(err, ErrNothingDownloadableFound) {
		t.Fatalf("mensagem nil = %v, esperado ErrNothingDownloadableFound", err)
	}
	if _, err := nilClient.DownloadAny(context.Background(), &waE2E.Message{}); !errors.Is(err, ErrNothingDownloadableFound) {
		t.Fatalf("mensagem sem midia = %v, esperado ErrNothingDownloadableFound", err)
	}

	// Com uma parte baixavel presente, DownloadAny delega para Download, que
	// num client nil devolve ErrClientIsNil — e' o marcador de que a parte foi
	// de fato selecionada.
	msgs := map[string]*waE2E.Message{
		"imagem":    {ImageMessage: &waE2E.ImageMessage{}},
		"video":     {VideoMessage: &waE2E.VideoMessage{}},
		"audio":     {AudioMessage: &waE2E.AudioMessage{}},
		"documento": {DocumentMessage: &waE2E.DocumentMessage{}},
		"figurinha": {StickerMessage: &waE2E.StickerMessage{}},
	}
	for name, msg := range msgs {
		t.Run(name, func(t *testing.T) {
			if _, err := nilClient.DownloadAny(context.Background(), msg); !errors.Is(err, ErrClientIsNil) {
				t.Fatalf("erro = %v, esperado ErrClientIsNil (parte selecionada)", err)
			}
		})
	}
}

func TestDownloadRejeitaMensagemSemURLEsemDirectPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	if _, err := cli.Download(context.Background(), &waE2E.ImageMessage{}); !errors.Is(err, ErrNoURLPresent) {
		t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
	}
	// URL de web.whatsapp.net sem directPath tambem cai em ErrNoURLPresent:
	// esse host nao serve midia.
	msg := &waE2E.ImageMessage{URL: proto.String(webWhatsappNetURLPrefix + "/qualquer")}
	if _, err := cli.Download(context.Background(), msg); !errors.Is(err, ErrNoURLPresent) {
		t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
	}
}

func TestDownloadRejeitaTipoDesconhecido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	if _, err := cli.Download(context.Background(), notDownloadable{}); !errors.Is(err, ErrUnknownMediaType) {
		t.Fatalf("erro = %v, esperado ErrUnknownMediaType", err)
	}
}

func TestDownloadThumbnail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)
	ctx := context.Background()

	t.Run("sem thumbnail direct path", func(t *testing.T) {
		if _, err := cli.DownloadThumbnail(ctx, &waE2E.ExtendedTextMessage{}); !errors.Is(err, ErrNoURLPresent) {
			t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
		}
	})
	t.Run("mensagem sem thumbnail conhecido", func(t *testing.T) {
		if _, err := cli.DownloadThumbnail(ctx, &waE2E.ImageMessage{}); !errors.Is(err, ErrUnknownMediaType) {
			t.Fatalf("erro = %v, esperado ErrUnknownMediaType", err)
		}
	})
}

func TestDownloadMediaWithPathExigeBarraInicial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	_, err := cli.DownloadMediaWithPath(
		context.Background(), "sem-barra", nil, nil, nil, unknownFileLength, MediaImage, "")
	if err == nil || !strings.Contains(err.Error(), "does not start with slash") {
		t.Fatalf("erro = %v, esperado recusa por falta de barra inicial", err)
	}
}

func TestDownloadMediaWithPathMontaAURLEBaixa(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x06}, mediaKeyLength)
	plaintext := []byte("conteudo via direct path")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, MediaImage)
	const directPath = "/v/t62.7118-24/abc?ccb=11-4"

	var gotURI, gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		gotHost = r.Header.Get(testOrigHostHeader)
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv, "primeiro.example")

	got, err := cli.DownloadMediaWithPath(
		context.Background(), directPath, encSHA, plainSHA, mediaKey, len(plaintext), MediaImage, "")
	if err != nil {
		t.Fatalf("DownloadMediaWithPath devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("conteudo = %q, esperado %q", got, plaintext)
	}
	if gotHost != "primeiro.example" {
		t.Errorf("host = %q, esperado o primeiro host da mediaConn", gotHost)
	}
	// mmsType vazio precisa cair no padrao do mediaType.
	if !strings.Contains(gotURI, "mms-type="+mediaTypeToMMSType[MediaImage]) {
		t.Errorf("URI %q nao contem o mms-type padrao de MediaImage", gotURI)
	}
	if !strings.Contains(gotURI, "hash="+base64.URLEncoding.EncodeToString(encSHA)) {
		t.Errorf("URI %q nao contem o hash do arquivo cifrado", gotURI)
	}
	if !strings.HasPrefix(gotURI, directPath) {
		t.Errorf("URI %q nao comeca com o directPath", gotURI)
	}
}

func TestDownloadMediaWithPathTrocaDeHostEmFalhaRetentavel(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x07}, mediaKeyLength)
	plaintext := []byte("so' o segundo host responde")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, MediaImage)

	var hostsVistos atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(testOrigHostHeader) == "quebrado.example" {
			hostsVistos.Add(1)
			w.Header().Set("Retry-After", "0") // mantem o teste rapido
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv, "quebrado.example", "bom.example")

	got, err := cli.DownloadMediaWithPath(
		context.Background(), "/v/x", encSHA, plainSHA, mediaKey, len(plaintext), MediaImage, "image")
	if err != nil {
		t.Fatalf("DownloadMediaWithPath devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("conteudo = %q, esperado %q", got, plaintext)
	}
	if hostsVistos.Load() == 0 {
		t.Error("o primeiro host nunca foi tentado")
	}
}

// 404 e' erro terminal: nao adianta tentar outro host, o arquivo nao existe.
func TestDownloadMediaWithPathNaoTrocaDeHostEm404(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv, "a.example", "b.example")

	_, err := cli.DownloadMediaWithPath(
		context.Background(), "/v/x", bytes.Repeat([]byte{0x00}, sha256HashLength),
		nil, bytes.Repeat([]byte{0x08}, mediaKeyLength), unknownFileLength, MediaImage, "image")
	if !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("erro = %v, esperado ErrMediaDownloadFailedWith404", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("houve %d requisicoes, esperado 1 (sem troca de host)", got)
	}
}

func TestFetchStickerPack(t *testing.T) {
	var gotURI string
	body := `[{"name":"Pacote","publisher":"Alguem"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	pack, err := cli.FetchStickerPack(context.Background(), "pack-123")
	if err != nil {
		t.Fatalf("FetchStickerPack devolveu erro: %v", err)
	}
	if pack.Name != "Pacote" {
		t.Errorf("Name = %q, esperado \"Pacote\"", pack.Name)
	}
	if !strings.Contains(gotURI, "id=pack-123") {
		t.Errorf("URI %q nao contem o id do pacote", gotURI)
	}

	body = `[]`
	if _, err = cli.FetchStickerPack(context.Background(), "vazio"); err == nil {
		t.Error("resposta sem pacotes deveria virar erro")
	}
}

func TestDownloadFBUsaODirectPathDoTransporte(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x0B}, mediaKeyLength)
	plaintext := []byte("midia via WAMediaTransport")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, MediaImage)

	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	cli := newMediaTestClient(t, srv)

	transport := &waMediaTransport.WAMediaTransport_Integral{
		DirectPath:    proto.String("/v/fb-path"),
		MediaKey:      mediaKey,
		FileEncSHA256: encSHA,
		FileSHA256:    plainSHA,
	}
	got, err := cli.DownloadFB(context.Background(), transport, MediaImage)
	if err != nil {
		t.Fatalf("DownloadFB devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("conteudo = %q, esperado %q", got, plaintext)
	}
	if !strings.HasPrefix(gotURI, "/v/fb-path") {
		t.Errorf("URI = %q, esperado comecar com o directPath do transporte", gotURI)
	}
}
