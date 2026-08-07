// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/security/cbc"
	waLog "wa-api/internal/wa-noise/observability/log"
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

// fakeTransport e' o duble de media.Transport usado nos testes: implementa a
// interface inteira sem precisar de um *whatsmeow.Client (nem de socket, store
// ou sessao). E' exatamente o ganho de testabilidade que a extracao buscava.
type fakeTransport struct {
	httpClient *http.Client
	log        waLog.Logger
	messenger  bool
	userAgent  string
	warnings   bool
	conn       ConnCache
	// iq responde ao IQ <media_conn>. Se for nil, o teste falha ao chegar la',
	// o que denuncia um caminho que deveria ter usado o cache.
	iq func(ctx context.Context) (*waBinary.Node, error)
}

var _ Transport = (*fakeTransport)(nil)

func (f *fakeTransport) HTTPClient() *http.Client { return f.httpClient }
func (f *fakeTransport) Log() waLog.Logger        { return f.log }
func (f *fakeTransport) IsMessenger() bool        { return f.messenger }
func (f *fakeTransport) MessengerUserAgent() string {
	return f.userAgent
}
func (f *fakeTransport) ReturnDownloadWarnings() bool { return f.warnings }
func (f *fakeTransport) MediaConnCache() *ConnCache   { return &f.conn }

func (f *fakeTransport) SendMediaConnIQ(ctx context.Context) (*waBinary.Node, error) {
	if f.iq == nil {
		return nil, errNoIQConfigured
	}
	return f.iq(ctx)
}

var errNoIQConfigured = &iqNotConfiguredError{}

type iqNotConfiguredError struct{}

func (*iqNotConfiguredError) Error() string { return "fakeTransport: nenhum IQ configurado" }

// newTestTransport devolve um fakeTransport com o http apontado para srv e o
// cache de media connection ja' populado (valido) com os hosts dados — o
// suficiente para os caminhos de download/upload de midia.
func newTestTransport(t *testing.T, srv *httptest.Server, hosts ...string) *fakeTransport {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("URL do servidor de teste invalida: %v", err)
	}
	if len(hosts) == 0 {
		hosts = []string{"mmg.whatsapp.net"}
	}
	connHosts := make([]ConnHost, len(hosts))
	for i, h := range hosts {
		connHosts[i] = ConnHost{Hostname: h}
	}
	tr := &fakeTransport{
		httpClient: &http.Client{Transport: &rewriteTransport{target: target}},
		log:        waLog.Noop,
		warnings:   true,
	}
	tr.conn.Set(&Conn{
		Auth:      "test-auth",
		TTL:       3600,
		FetchedAt: time.Now(),
		Hosts:     connHosts,
	})
	return tr
}

// decryptForTest e' um atalho para cbcutil.Decrypt, usado para conferir que o
// que foi enviado no upload volta ao plaintext original.
func decryptForTest(cipherKey, iv, ciphertext []byte) ([]byte, error) {
	return cbcutil.Decrypt(cipherKey, iv, ciphertext)
}

// encryptedMediaBlob produz o corpo que o servidor de midia devolveria para o
// plaintext dado: ciphertext CBC + HMAC truncado, junto do hash do ciphertext
// (fileEncSHA256) e do hash do plaintext (fileSHA256).
func encryptedMediaBlob(t *testing.T, mediaKey, plaintext []byte, appInfo Type) (blob, fileEncSHA256, fileSHA256 []byte) {
	t.Helper()
	iv, cipherKey, macKey, _ := GetKeys(mediaKey, appInfo)
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
