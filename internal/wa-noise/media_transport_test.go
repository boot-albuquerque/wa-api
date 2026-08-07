// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"wa-api/internal/wa-noise/media"
	"wa-api/internal/wa-noise/proto/waE2E"
	waLog "wa-api/internal/wa-noise/util/log"
)

// A logica de midia mora em internal/wa-noise/media e e' testada la'. O que
// este arquivo cobre e' a camada fina do pacote raiz: o adaptador
// *Client -> media.Transport e as guardas de receiver nil dos wrappers.

func TestMediaTransportExpoeOsCamposDoClient(t *testing.T) {
	httpCli := &http.Client{}
	cli := &Client{
		Log:       waLog.Noop,
		mediaHTTP: httpCli,
	}
	tr := cli.mediaT()

	if tr.HTTPClient() != httpCli {
		t.Error("HTTPClient() nao devolveu o mediaHTTP do Client")
	}
	if tr.Log() == nil {
		t.Error("Log() devolveu nil")
	}
	if tr.IsMessenger() {
		t.Error("IsMessenger() = true sem MessengerConfig")
	}
	if got := tr.MessengerUserAgent(); got != "" {
		t.Errorf("MessengerUserAgent() = %q sem MessengerConfig, esperado vazio", got)
	}
	if tr.MediaConnCache() != &cli.mediaConn {
		t.Error("MediaConnCache() nao aponta para o cache do Client")
	}

	cli.MessengerConfig = &MessengerConfig{UserAgent: "UA/1"}
	tr = cli.mediaT()
	if !tr.IsMessenger() || tr.MessengerUserAgent() != "UA/1" {
		t.Errorf("com MessengerConfig: IsMessenger=%v UA=%q", tr.IsMessenger(), tr.MessengerUserAgent())
	}
}

// ReturnDownloadWarnings continua sendo a variavel global do pacote raiz; o
// adaptador so' a le'.
func TestMediaTransportLeReturnDownloadWarnings(t *testing.T) {
	cli := &Client{Log: waLog.Noop}
	original := ReturnDownloadWarnings
	t.Cleanup(func() { ReturnDownloadWarnings = original })

	ReturnDownloadWarnings = true
	if !cli.mediaT().ReturnDownloadWarnings() {
		t.Error("esperado true")
	}
	ReturnDownloadWarnings = false
	if cli.mediaT().ReturnDownloadWarnings() {
		t.Error("esperado false")
	}
}

// Os apelidos de tipo precisam continuar sendo apelidos (=), nao definicoes
// novas: e' o que mantem a API historica do pacote raiz compativel.
func TestApelidosDeTipoDeMidia(t *testing.T) {
	var mt MediaType = media.TypeImage
	if mt != MediaImage {
		t.Error("MediaImage nao e' media.TypeImage")
	}
	var conn *MediaConn = (*media.Conn)(nil)
	_ = conn
	var host MediaConnHost = media.ConnHost{Hostname: "x"}
	if host.Hostname != "x" {
		t.Error("MediaConnHost nao e' media.ConnHost")
	}
	var resp UploadResponse = media.UploadResponse{Handle: "h"}
	if resp.Handle != "h" {
		t.Error("UploadResponse nao e' media.UploadResponse")
	}
	var httpErr DownloadHTTPError = media.DownloadHTTPError{Response: &http.Response{StatusCode: 404}}
	if !errors.Is(httpErr, ErrMediaDownloadFailedWith404) {
		t.Error("DownloadHTTPError nao e' media.DownloadHTTPError")
	}
}

// Os sentinelas exportados do pacote raiz precisam ser os MESMOS valores do
// pacote media — nao copias — senao errors.Is falha para quem compara com os
// nomes do pacote raiz.
func TestSentinelasDeMidiaSaoOsMesmosValores(t *testing.T) {
	pares := []struct {
		name       string
		raiz, base error
	}{
		{"ErrNoURLPresent", ErrNoURLPresent, media.ErrNoURLPresent},
		{"ErrFileLengthMismatch", ErrFileLengthMismatch, media.ErrFileLengthMismatch},
		{"ErrTooShortFile", ErrTooShortFile, media.ErrTooShortFile},
		{"ErrInvalidMediaHMAC", ErrInvalidMediaHMAC, media.ErrInvalidMediaHMAC},
		{"ErrInvalidMediaEncSHA256", ErrInvalidMediaEncSHA256, media.ErrInvalidMediaEncSHA256},
		{"ErrInvalidMediaSHA256", ErrInvalidMediaSHA256, media.ErrInvalidMediaSHA256},
		{"ErrUnknownMediaType", ErrUnknownMediaType, media.ErrUnknownMediaType},
		{"ErrNothingDownloadableFound", ErrNothingDownloadableFound, media.ErrNothingDownloadableFound},
		{"ErrMediaNotAvailableOnPhone", ErrMediaNotAvailableOnPhone, media.ErrMediaNotAvailableOnPhone},
		{"ErrUnknownMediaRetryError", ErrUnknownMediaRetryError, media.ErrUnknownMediaRetryError},
	}
	for _, p := range pares {
		if p.raiz != p.base {
			t.Errorf("%s do pacote raiz nao e' o mesmo valor do pacote media", p.name)
		}
	}
}

func TestGetMediaTypeDelegaParaMedia(t *testing.T) {
	if got := GetMediaType(&waE2E.ImageMessage{}); got != MediaImage {
		t.Fatalf("GetMediaType() = %q, esperado MediaImage", got)
	}
}

// Todo wrapper exportado de midia precisa recusar receiver nil com
// ErrClientIsNil em vez de estourar em nil deref. Antes da extracao apenas
// Download, DownloadToFile e SendMediaRetryReceipt faziam isso.
func TestWrappersDeMidiaRecusamClientNil(t *testing.T) {
	var cli *Client
	ctx := context.Background()

	checks := map[string]func() error{
		"DownloadAny": func() error { _, err := cli.DownloadAny(ctx, nil); return err },
		"DownloadThumbnail": func() error {
			_, err := cli.DownloadThumbnail(ctx, &waE2E.ExtendedTextMessage{})
			return err
		},
		"FetchStickerPack": func() error { _, err := cli.FetchStickerPack(ctx, "x"); return err },
		"Download":         func() error { _, err := cli.Download(ctx, &waE2E.ImageMessage{}); return err },
		"DownloadFB":       func() error { _, err := cli.DownloadFB(ctx, nil, MediaImage); return err },
		"DownloadMediaWithPath": func() error {
			_, err := cli.DownloadMediaWithPath(ctx, "/v/x", nil, nil, nil, 0, MediaImage, "")
			return err
		},
		"DownloadToFile": func() error { return cli.DownloadToFile(ctx, &waE2E.ImageMessage{}, nil) },
		"DownloadFBToFile": func() error {
			return cli.DownloadFBToFile(ctx, nil, MediaImage, nil)
		},
		"DownloadMediaWithPathToFile": func() error {
			return cli.DownloadMediaWithPathToFile(ctx, "/v/x", nil, nil, nil, 0, MediaImage, "", nil)
		},
		"Upload":                 func() error { _, err := cli.Upload(ctx, nil, MediaImage); return err },
		"UploadReader":           func() error { _, err := cli.UploadReader(ctx, nil, nil, MediaImage); return err },
		"UploadNewsletter":       func() error { _, err := cli.UploadNewsletter(ctx, nil, MediaImage); return err },
		"UploadNewsletterReader": func() error { _, err := cli.UploadNewsletterReader(ctx, nil, MediaImage); return err },
		"DeleteMedia":            func() error { return cli.DeleteMedia(ctx, MediaImage, "/v/x", nil, "") },
		"refreshMediaConn":       func() error { _, err := cli.refreshMediaConn(ctx, false); return err },
		"queryMediaConn":         func() error { _, err := cli.queryMediaConn(ctx); return err },
	}
	for name, fn := range checks {
		t.Run(name, func(t *testing.T) {
			if err := fn(); !errors.Is(err, ErrClientIsNil) {
				t.Fatalf("erro = %v, esperado ErrClientIsNil", err)
			}
		})
	}
}

// O cache de media connection do Client e' o mesmo objeto que o pacote media
// enxerga pelo adaptador: prepopular por um lado aparece do outro.
func TestClientCompartilhaOCacheDeMediaConn(t *testing.T) {
	cli := &Client{Log: waLog.Noop}
	cached := &media.Conn{
		Auth:      "cacheado",
		TTL:       3600,
		FetchedAt: time.Now(),
		Hosts:     []MediaConnHost{{Hostname: "a.example"}},
	}
	cli.mediaConn.Set(cached)

	got, err := cli.refreshMediaConn(context.Background(), false)
	if err != nil {
		t.Fatalf("refreshMediaConn devolveu erro: %v", err)
	}
	if got != cached {
		t.Fatal("refreshMediaConn nao devolveu a mediaConn cacheada")
	}
}
