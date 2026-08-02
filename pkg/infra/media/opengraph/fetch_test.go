package opengraph

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.RGBA{R: uint8(x), G: 0x40, B: 0x80, A: 0xFF})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// truncatedPNG mantem o IHDR intacto (DecodeConfig passa) e corta o resto
// (image.Decode falha). E' o unico jeito de separar os dois ramos de erro.
func truncatedPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	full := pngBytes(t, w, h)
	const pngSignature = 8
	const ihdrChunk = 25 // length(4) + "IHDR"(4) + payload(13) + crc(4)
	return full[:pngSignature+ihdrChunk+4]
}

func newServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func serveBytes(t *testing.T, contentType string, body []byte) *httptest.Server {
	t.Helper()
	return newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if contentType == "" {
			w.Header()["Content-Type"] = nil
		} else {
			w.Header().Set("Content-Type", contentType)
		}
		if _, err := w.Write(body); err != nil {
			t.Errorf("escrever corpo: %v", err)
		}
	})
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

func isZeroResult(r Result) bool {
	return r.Title == "" && r.Description == "" && r.ImageData == nil &&
		r.HQImageData == nil && r.HQWidth == 0 && r.HQHeight == 0
}

// ---------------------------------------------------------------------------
// FetchURLBytes
// ---------------------------------------------------------------------------

func TestFetchURLBytesEnviaCabecalhosDePreview(t *testing.T) {
	var gotUA, gotAccept, gotLang string
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		gotLang = r.Header.Get("Accept-Language")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write([]byte("<html></html>")); err != nil {
			t.Errorf("escrever: %v", err)
		}
	})

	data, contentType, err := FetchURLBytes(context.Background(), srv.Client(), srv.URL, 1024)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if string(data) != "<html></html>" {
		t.Fatalf("corpo = %q", data)
	}
	if contentType != "text/html; charset=utf-8" {
		t.Fatalf("contentType = %q", contentType)
	}
	if gotUA != "WhatsApp/2.23.20.0" {
		t.Fatalf("User-Agent = %q", gotUA)
	}
	if strings.Contains(gotAccept, "image/avif") {
		t.Fatalf("Accept nao pode anunciar avif: %q", gotAccept)
	}
	if !strings.HasPrefix(gotLang, "pt-BR") {
		t.Fatalf("Accept-Language = %q", gotLang)
	}
}

func TestFetchURLBytesDetectaContentTypeQuandoAusente(t *testing.T) {
	srv := serveBytes(t, "", pngBytes(t, 4, 4))

	_, contentType, err := FetchURLBytes(context.Background(), srv.Client(), srv.URL, 4096)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if contentType != "image/png" {
		t.Fatalf("contentType = %q, quero image/png detectado do conteudo", contentType)
	}
}

func TestFetchURLBytesURLInvalida(t *testing.T) {
	_, _, err := FetchURLBytes(context.Background(), http.DefaultClient, "http://exemplo\x7f/x", 1024)

	if err == nil {
		t.Fatal("quero erro ao montar a requisicao")
	}
}

func TestFetchURLBytesErroDeTransporte(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rawURL := srv.URL
	client := srv.Client()
	srv.Close()

	_, _, err := FetchURLBytes(context.Background(), client, rawURL, 1024)

	if err == nil {
		t.Fatal("quero erro de transporte com o servidor fechado")
	}
}

func TestFetchURLBytesStatusInesperado(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, _, err := FetchURLBytes(context.Background(), srv.Client(), srv.URL, 1024)

	if err == nil || !strings.Contains(err.Error(), "unexpected status code 404") {
		t.Fatalf("erro = %v, quero 'unexpected status code 404'", err)
	}
}

func TestFetchURLBytesAcimaDoLimite(t *testing.T) {
	srv := serveBytes(t, "text/plain", bytes.Repeat([]byte("a"), 100))

	_, _, err := FetchURLBytes(context.Background(), srv.Client(), srv.URL, 10)

	if err == nil || !strings.Contains(err.Error(), "response exceeds allowed size (10 bytes)") {
		t.Fatalf("erro = %v", err)
	}
}

func TestFetchURLBytesCorpoInterrompido(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		if _, err := w.Write([]byte("curto")); err != nil {
			t.Errorf("escrever: %v", err)
		}
	})

	_, _, err := FetchURLBytes(context.Background(), srv.Client(), srv.URL, 4096)

	if err == nil {
		t.Fatal("quero erro de leitura quando o corpo e' menor que o Content-Length")
	}
}

// failingCloser entrega o corpo normalmente e falha no Close. O transporte
// padrao do net/http engole erros de Close, entao um RoundTripper proprio e' o
// unico jeito de exercitar o defer de fechamento.
type failingCloser struct{ r io.Reader }

func (f failingCloser) Read(p []byte) (int, error) { return f.r.Read(p) }
func (failingCloser) Close() error                 { return errors.New("falha ao fechar o corpo") }

type closeFailingTransport struct{ body string }

func (t closeFailingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       failingCloser{r: strings.NewReader(t.body)},
	}, nil
}

func TestFetchURLBytesErroAoFecharCorpo(t *testing.T) {
	client := &http.Client{Transport: closeFailingTransport{body: "conteudo"}}

	data, contentType, err := FetchURLBytes(context.Background(), client, "http://exemplo.invalido/x", 1024)

	// O erro de fechamento e' registrado, nao propagado: o corpo ja' foi lido.
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if string(data) != "conteudo" {
		t.Fatalf("corpo = %q", data)
	}
	if contentType != "text/plain" {
		t.Fatalf("contentType = %q", contentType)
	}
}

// ---------------------------------------------------------------------------
// FetchOpenGraphImage
// ---------------------------------------------------------------------------

func TestFetchOpenGraphImageSemURL(t *testing.T) {
	var result Result

	FetchOpenGraphImage(context.Background(), http.DefaultClient, mustParseURL(t, "http://pagina.exemplo/"), "", &result)

	if !isZeroResult(result) {
		t.Fatalf("sem URL de imagem nada deveria ser preenchido: %+v", result)
	}
}

func TestFetchOpenGraphImageURLInvalida(t *testing.T) {
	var result Result

	FetchOpenGraphImage(context.Background(), http.DefaultClient, mustParseURL(t, "http://pagina.exemplo/"), "://invalida", &result)

	if !isZeroResult(result) {
		t.Fatalf("URL de imagem invalida nao deveria preencher nada: %+v", result)
	}
}

func TestFetchOpenGraphImageFalhaNoDownload(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	var result Result

	FetchOpenGraphImage(context.Background(), srv.Client(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result)

	if !isZeroResult(result) {
		t.Fatalf("falha de download nao deveria preencher nada: %+v", result)
	}
}

func TestFetchOpenGraphImageConfigIndecodificavel(t *testing.T) {
	srv := serveBytes(t, "image/png", []byte("isso nao e' uma imagem"))
	var result Result

	FetchOpenGraphImage(context.Background(), srv.Client(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result)

	if !isZeroResult(result) {
		t.Fatalf("config indecodificavel nao deveria preencher nada: %+v", result)
	}
}

func TestFetchOpenGraphImageDimensoesGrandesDemais(t *testing.T) {
	srv := serveBytes(t, "image/png", pngBytes(t, MaxImageDim+1, 1))
	var result Result

	FetchOpenGraphImage(context.Background(), srv.Client(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result)

	if !isZeroResult(result) {
		t.Fatalf("imagem acima do limite deveria ser recusada: %+v", result)
	}
}

func TestFetchOpenGraphImageDecodeFalha(t *testing.T) {
	srv := serveBytes(t, "image/png", truncatedPNG(t, 32, 32))
	var result Result

	FetchOpenGraphImage(context.Background(), srv.Client(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result)

	if !isZeroResult(result) {
		t.Fatalf("PNG truncado deveria falhar no Decode: %+v", result)
	}
}

func TestFetchOpenGraphImageSucesso(t *testing.T) {
	srv := serveBytes(t, "image/png", pngBytes(t, 1200, 800))
	var result Result

	FetchOpenGraphImage(context.Background(), srv.Client(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result)

	if len(result.HQImageData) == 0 || len(result.ImageData) == 0 {
		t.Fatal("quero as duas miniaturas preenchidas")
	}
	if result.HQWidth != HQThumbnailDim {
		t.Fatalf("HQWidth = %d, quero %d", result.HQWidth, HQThumbnailDim)
	}
	if result.HQHeight == 0 || result.HQHeight > HQThumbnailDim {
		t.Fatalf("HQHeight = %d, fora do esperado", result.HQHeight)
	}
	small, _, err := image.DecodeConfig(bytes.NewReader(result.ImageData))
	if err != nil {
		t.Fatalf("decodificar miniatura pequena: %v", err)
	}
	if small.Width > ThumbnailWidth || small.Height > ThumbnailHeight {
		t.Fatalf("miniatura inline = %dx%d, acima de %dx%d",
			small.Width, small.Height, ThumbnailWidth, ThumbnailHeight)
	}
}

func TestFetchOpenGraphImageResolveURLRelativa(t *testing.T) {
	var gotPath string
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(pngBytes(t, 40, 40)); err != nil {
			t.Errorf("escrever: %v", err)
		}
	})
	var result Result

	FetchOpenGraphImage(context.Background(), srv.Client(), mustParseURL(t, srv.URL+"/dir/pagina.html"), "capa.png", &result)

	if gotPath != "/dir/capa.png" {
		t.Fatalf("caminho resolvido = %q, quero /dir/capa.png", gotPath)
	}
	if len(result.ImageData) == 0 {
		t.Fatal("miniatura vazia")
	}
}

// ---------------------------------------------------------------------------
// FetchOpenGraphData
// ---------------------------------------------------------------------------

func TestFetchOpenGraphDataFalhaNoFetch(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	got := FetchOpenGraphData(context.Background(), srv.Client(), srv.URL)

	if !isZeroResult(got) {
		t.Fatalf("quero resultado zero, tenho %+v", got)
	}
}

func TestFetchOpenGraphDataHTMLIrreparavel(t *testing.T) {
	// x/net/html recusa HTML aninhado alem de 512 elementos, e goquery propaga
	// esse erro. E' o unico jeito de exercitar o ramo de parse falho.
	deep := strings.Repeat("<div>", 600)
	srv := serveBytes(t, "text/html", []byte("<html><body>"+deep))

	got := FetchOpenGraphData(context.Background(), srv.Client(), srv.URL)

	if !isZeroResult(got) {
		t.Fatalf("quero resultado zero quando o HTML nao parseia, tenho %+v", got)
	}
}

func TestFetchOpenGraphDataMetadados(t *testing.T) {
	tests := []struct {
		name            string
		html            string
		wantTitle       string
		wantDescription string
	}{
		{
			name:            "og:title e og:description",
			html:            `<html><head><meta property="og:title" content="Titulo OG"><meta property="og:description" content="Descricao OG"><title>Titulo tag</title></head></html>`,
			wantTitle:       "Titulo OG",
			wantDescription: "Descricao OG",
		},
		{
			name:            "fallback para <title> e meta description",
			html:            `<html><head><title>  Titulo tag  </title><meta name="description" content="Descricao meta"></head></html>`,
			wantTitle:       "Titulo tag",
			wantDescription: "Descricao meta",
		},
		{
			name:            "sem metadados",
			html:            `<html><head></head><body>nada</body></html>`,
			wantTitle:       "",
			wantDescription: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := serveBytes(t, "text/html", []byte(tc.html))

			got := FetchOpenGraphData(context.Background(), srv.Client(), srv.URL)

			if got.Title != tc.wantTitle {
				t.Fatalf("Title = %q, quero %q", got.Title, tc.wantTitle)
			}
			if got.Description != tc.wantDescription {
				t.Fatalf("Description = %q, quero %q", got.Description, tc.wantDescription)
			}
			if got.ImageData != nil {
				t.Fatal("sem imagem na pagina nada deveria ser baixado")
			}
		})
	}
}

func TestFetchOpenGraphDataSeletoresDeImagem(t *testing.T) {
	tests := []struct {
		name string
		tag  string
	}{
		{"og:image", `<meta property="og:image" content="/capa.png">`},
		{"twitter:image", `<meta property="twitter:image" content="/capa.png">`},
		{"apple-touch-icon", `<link rel="apple-touch-icon" href="/capa.png">`},
		{"icon", `<link rel="icon" href="/capa.png">`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/capa.png" {
					w.Header().Set("Content-Type", "image/png")
					if _, err := w.Write(pngBytes(t, 64, 64)); err != nil {
						t.Errorf("escrever imagem: %v", err)
					}
					return
				}
				w.Header().Set("Content-Type", "text/html")
				if _, err := w.Write([]byte(`<html><head>` + tc.tag + `</head></html>`)); err != nil {
					t.Errorf("escrever html: %v", err)
				}
			})

			got := FetchOpenGraphData(context.Background(), srv.Client(), srv.URL+"/pagina")

			if len(got.ImageData) == 0 {
				t.Fatalf("miniatura vazia para o seletor %s", tc.name)
			}
			if got.HQWidth == 0 || got.HQHeight == 0 {
				t.Fatalf("dimensoes HQ zeradas: %+v", got)
			}
		})
	}
}
