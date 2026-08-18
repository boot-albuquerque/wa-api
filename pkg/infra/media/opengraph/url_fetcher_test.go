package opengraph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestURLFetcher_FetchBytes_DelegatesToFetchURLBytes prova que URLFetcher
// não reimplementa a busca — FetchURLBytes já tem cobertura própria
// (fetch_test.go) para limite, Content-Length falso, corpo interrompido,
// status inesperado, e sniffing de Content-Type ausente.
func TestURLFetcher_FetchBytes_DelegatesToFetchURLBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("bytes-de-imagem"))
	}))
	defer srv.Close()

	f := NewURLFetcher(srv.Client())
	data, contentType, err := f.FetchBytes(context.Background(), srv.URL, 4096)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if string(data) != "bytes-de-imagem" {
		t.Errorf("data = %q", data)
	}
	if contentType != "image/png" {
		t.Errorf("contentType = %q, want %q", contentType, "image/png")
	}
}

// TestURLFetcher_FetchBytes_LimitEnforcedDuringRead prova que o limite é
// aplicado DURANTE a leitura, não só contra um Content-Length declarado: o
// servidor não declara Content-Length (chunked) e envia mais bytes do que o
// limite permite.
func TestURLFetcher_FetchBytes_LimitEnforcedDuringRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter nao suporta streaming")
		}
		for i := 0; i < 10; i++ {
			_, _ = w.Write([]byte(strings.Repeat("a", 1024)))
			flusher.Flush()
		}
	}))
	defer srv.Close()

	f := NewURLFetcher(srv.Client())
	_, _, err := f.FetchBytes(context.Background(), srv.URL, 1024)

	if err == nil {
		t.Fatal("resposta streamed acima do limite foi aceita sem Content-Length")
	}
	if !strings.Contains(err.Error(), "response exceeds allowed size") {
		t.Errorf("erro = %v, quero limite excedido", err)
	}
}
