// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"wa-api/internal/wa-noise/util/cbcutil"
	waLog "wa-api/internal/wa-noise/util/log"
)

// rewriteTransport redireciona qualquer requisicao (inclusive as https que o
// codigo de midia monta) para o host de um httptest.Server local, preservando
// path e query. Assim da' para exercitar o caminho HTTP real do fork —
// incluindo a URL montada — sem rede nem TLS.
const testOrigHostHeader = "X-Test-Orig-Host"

type rewriteTransport struct {
	target *url.URL
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	// Preserva o host original para que o handler de teste possa distinguir
	// qual host da mediaConn o codigo escolheu.
	clone.Header.Set(testOrigHostHeader, req.URL.Host)
	clone.URL.Scheme = rt.target.Scheme
	clone.URL.Host = rt.target.Host
	clone.Host = ""
	return http.DefaultTransport.RoundTrip(clone)
}

// newMediaTestClient devolve um Client minimo, sem socket nem store, com o
// mediaHTTP apontado para srv e um mediaConnCache pre-populado (valido) com os
// hosts dados — o suficiente para os caminhos de download/upload de midia.
func newMediaTestClient(t *testing.T, srv *httptest.Server, hosts ...string) *Client {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("URL do servidor de teste invalida: %v", err)
	}
	if len(hosts) == 0 {
		hosts = []string{"mmg.whatsapp.net"}
	}
	connHosts := make([]MediaConnHost, len(hosts))
	for i, h := range hosts {
		connHosts[i] = MediaConnHost{Hostname: h}
	}
	return &Client{
		Log:       waLog.Noop,
		mediaHTTP: &http.Client{Transport: &rewriteTransport{target: target}},
		mediaConnCache: &MediaConn{
			Auth:      "test-auth",
			TTL:       3600,
			FetchedAt: time.Now(),
			Hosts:     connHosts,
		},
	}
}

// decryptForTest e' um atalho para cbcutil.Decrypt, usado para conferir que o
// que foi enviado no upload volta ao plaintext original.
func decryptForTest(cipherKey, iv, ciphertext []byte) ([]byte, error) {
	return cbcutil.Decrypt(cipherKey, iv, ciphertext)
}

// encryptedMediaBlob produz o corpo que o servidor de midia devolveria para o
// plaintext dado: ciphertext CBC + HMAC truncado, junto do hash do ciphertext
// (fileEncSHA256) e do hash do plaintext (fileSHA256).
func encryptedMediaBlob(t *testing.T, mediaKey, plaintext []byte, appInfo MediaType) (blob, fileEncSHA256, fileSHA256 []byte) {
	t.Helper()
	iv, cipherKey, macKey, _ := getMediaKeys(mediaKey, appInfo)
	ciphertext, err := cbcutil.Encrypt(cipherKey, iv, plaintext)
	if err != nil {
		t.Fatalf("falha ao cifrar o plaintext de teste: %v", err)
	}
	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(ciphertext)
	blob = append(append([]byte{}, ciphertext...), h.Sum(nil)[:mediaHMACLength]...)
	encHash := sha256.Sum256(blob)
	plainHash := sha256.Sum256(plaintext)
	return blob, encHash[:], plainHash[:]
}
