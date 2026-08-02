package media

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
	"time"

	"github.com/patrickmn/go-cache"
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

// oversizedImage anuncia dimensoes acima do limite de 16 bits do JPEG sem
// alocar nada; e' o unico caminho de erro real de jpeg.Encode aqui, ja' que o
// destino e' um bytes.Buffer, que nunca falha na escrita.
type oversizedImage struct{}

func (oversizedImage) ColorModel() color.Model { return color.RGBAModel }
func (oversizedImage) Bounds() image.Rectangle { return image.Rect(0, 0, 1<<16, 1<<16) }
func (oversizedImage) At(_, _ int) color.Color { return color.RGBA{} }

// isZeroResult: OpenGraphResult contem slices e nao e' comparavel com ==.
func isZeroResult(r OpenGraphResult) bool {
	return r.Title == "" && r.Description == "" && r.ImageData == nil &&
		r.HQImageData == nil && r.HQWidth == 0 && r.HQHeight == 0
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

// ---------------------------------------------------------------------------
// UserSemaphoreManager
// ---------------------------------------------------------------------------

func TestUserSemaphoreManagerForUser(t *testing.T) {
	usm := NewUserSemaphoreManager()

	a1 := usm.ForUser("usuario-a")
	a2 := usm.ForUser("usuario-a")
	b := usm.ForUser("usuario-b")

	if a1 == nil {
		t.Fatal("quero um pool nao nil")
	}
	if a1 != a2 {
		t.Fatal("o mesmo usuario deve reusar o mesmo pool")
	}
	if a1 == b {
		t.Fatal("usuarios distintos devem ter pools distintos")
	}
	if cap(a1) != openGraphUserFetchLimit {
		t.Fatalf("capacidade = %d, quero %d", cap(a1), openGraphUserFetchLimit)
	}
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

	data, contentType, err := FetchURLBytes(context.Background(), srv.URL, 1024, srv.Client())

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

	_, contentType, err := FetchURLBytes(context.Background(), srv.URL, 4096, srv.Client())

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if contentType != "image/png" {
		t.Fatalf("contentType = %q, quero image/png detectado do conteudo", contentType)
	}
}

func TestFetchURLBytesURLInvalida(t *testing.T) {
	_, _, err := FetchURLBytes(context.Background(), "http://exemplo\x7f/x", 1024, http.DefaultClient)

	if err == nil {
		t.Fatal("quero erro ao montar a requisicao")
	}
}

func TestFetchURLBytesErroDeTransporte(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	client := srv.Client()
	srv.Close()

	_, _, err := FetchURLBytes(context.Background(), url, 1024, client)

	if err == nil {
		t.Fatal("quero erro de transporte com o servidor fechado")
	}
}

func TestFetchURLBytesStatusInesperado(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, _, err := FetchURLBytes(context.Background(), srv.URL, 1024, srv.Client())

	if err == nil || !strings.Contains(err.Error(), "unexpected status code 404") {
		t.Fatalf("erro = %v, quero 'unexpected status code 404'", err)
	}
}

func TestFetchURLBytesAcimaDoLimite(t *testing.T) {
	srv := serveBytes(t, "text/plain", bytes.Repeat([]byte("a"), 100))

	_, _, err := FetchURLBytes(context.Background(), srv.URL, 10, srv.Client())

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

	_, _, err := FetchURLBytes(context.Background(), srv.URL, 4096, srv.Client())

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

	data, contentType, err := FetchURLBytes(context.Background(), "http://exemplo.invalido/x", 1024, client)

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
// ExtractFirstURL
// ---------------------------------------------------------------------------

func TestExtractFirstURL(t *testing.T) {
	tests := []struct {
		name, text, want string
	}{
		{"texto vazio", "", ""},
		{"sem URL", "so texto sem link nenhum", ""},
		{"http simples", "veja http://exemplo.com agora", "http://exemplo.com"},
		{"https com caminho", "link https://exemplo.com/a/b?c=1 fim", "https://exemplo.com/a/b?c=1"},
		{"primeira de varias", "http://um.com e http://dois.com", "http://um.com"},
		{"pontuacao final e' descartada", "acesse https://exemplo.com.", "https://exemplo.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractFirstURL(tc.text); got != tc.want {
				t.Fatalf("ExtractFirstURL(%q) = %q, quero %q", tc.text, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// encodeJPEGThumbnail
// ---------------------------------------------------------------------------

func TestEncodeJPEGThumbnail(t *testing.T) {
	out := encodeJPEGThumbnail(image.NewRGBA(image.Rect(0, 0, 8, 6)))

	if !bytes.HasPrefix(out, []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("magic number = % x, quero FF D8 FF", out[:3])
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decodificar de volta: %v", err)
	}
	if cfg.Width != 8 || cfg.Height != 6 {
		t.Fatalf("dimensoes = %dx%d, quero 8x6", cfg.Width, cfg.Height)
	}
}

func TestEncodeJPEGThumbnailErro(t *testing.T) {
	if out := encodeJPEGThumbnail(oversizedImage{}); out != nil {
		t.Fatalf("quero nil quando o encode falha, tenho %d bytes", len(out))
	}
}

// ---------------------------------------------------------------------------
// fetchOpenGraphImage
// ---------------------------------------------------------------------------

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

func TestFetchOpenGraphImageSemURL(t *testing.T) {
	var result OpenGraphResult

	fetchOpenGraphImage(context.Background(), mustParseURL(t, "http://pagina.exemplo/"), "", &result, http.DefaultClient)

	if result.ImageData != nil || result.HQImageData != nil {
		t.Fatal("sem URL de imagem nada deveria ser preenchido")
	}
}

func TestFetchOpenGraphImageURLInvalida(t *testing.T) {
	var result OpenGraphResult

	fetchOpenGraphImage(context.Background(), mustParseURL(t, "http://pagina.exemplo/"), "://invalida", &result, http.DefaultClient)

	if result.ImageData != nil {
		t.Fatal("URL de imagem invalida nao deveria preencher nada")
	}
}

func TestFetchOpenGraphImageFalhaNoDownload(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	var result OpenGraphResult

	fetchOpenGraphImage(context.Background(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result, srv.Client())

	if result.ImageData != nil {
		t.Fatal("falha de download nao deveria preencher nada")
	}
}

func TestFetchOpenGraphImageConfigIndecodificavel(t *testing.T) {
	srv := serveBytes(t, "image/png", []byte("isso nao e' uma imagem"))
	var result OpenGraphResult

	fetchOpenGraphImage(context.Background(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result, srv.Client())

	if result.ImageData != nil {
		t.Fatal("config indecodificavel nao deveria preencher nada")
	}
}

func TestFetchOpenGraphImageDimensoesGrandesDemais(t *testing.T) {
	srv := serveBytes(t, "image/png", pngBytes(t, openGraphMaxImageDim+1, 1))
	var result OpenGraphResult

	fetchOpenGraphImage(context.Background(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result, srv.Client())

	if result.ImageData != nil || result.HQWidth != 0 {
		t.Fatal("imagem acima do limite deveria ser recusada")
	}
}

func TestFetchOpenGraphImageDecodeFalha(t *testing.T) {
	srv := serveBytes(t, "image/png", truncatedPNG(t, 32, 32))
	var result OpenGraphResult

	fetchOpenGraphImage(context.Background(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result, srv.Client())

	if result.ImageData != nil {
		t.Fatal("PNG truncado deveria falhar no Decode sem preencher nada")
	}
}

func TestFetchOpenGraphImageSucesso(t *testing.T) {
	srv := serveBytes(t, "image/png", pngBytes(t, 1200, 800))
	var result OpenGraphResult

	fetchOpenGraphImage(context.Background(), mustParseURL(t, srv.URL+"/pagina"), "/img.png", &result, srv.Client())

	if len(result.HQImageData) == 0 || len(result.ImageData) == 0 {
		t.Fatal("quero as duas miniaturas preenchidas")
	}
	if result.HQWidth != openGraphHQThumbnailDim {
		t.Fatalf("HQWidth = %d, quero %d", result.HQWidth, openGraphHQThumbnailDim)
	}
	if result.HQHeight == 0 || result.HQHeight > openGraphHQThumbnailDim {
		t.Fatalf("HQHeight = %d, fora do esperado", result.HQHeight)
	}
	small, _, err := image.DecodeConfig(bytes.NewReader(result.ImageData))
	if err != nil {
		t.Fatalf("decodificar miniatura pequena: %v", err)
	}
	if small.Width > openGraphThumbnailWidth || small.Height > openGraphThumbnailHeight {
		t.Fatalf("miniatura inline = %dx%d, acima de %dx%d",
			small.Width, small.Height, openGraphThumbnailWidth, openGraphThumbnailHeight)
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
	var result OpenGraphResult

	fetchOpenGraphImage(context.Background(), mustParseURL(t, srv.URL+"/dir/pagina.html"), "capa.png", &result, srv.Client())

	if gotPath != "/dir/capa.png" {
		t.Fatalf("caminho resolvido = %q, quero /dir/capa.png", gotPath)
	}
	if len(result.ImageData) == 0 {
		t.Fatal("miniatura vazia")
	}
}

// ---------------------------------------------------------------------------
// fetchOpenGraphDataInternal
// ---------------------------------------------------------------------------

func TestFetchOpenGraphDataInternalFalhaNoFetch(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	got := fetchOpenGraphDataInternal(context.Background(), srv.URL, srv.Client())

	if !isZeroResult(got) {
		t.Fatalf("quero resultado zero, tenho %+v", got)
	}
}

func TestFetchOpenGraphDataInternalHTMLIrreparavel(t *testing.T) {
	// x/net/html recusa HTML aninhado alem de 512 elementos, e goquery propaga
	// esse erro. E' o unico jeito de exercitar o ramo de parse falho.
	deep := strings.Repeat("<div>", 600)
	srv := serveBytes(t, "text/html", []byte("<html><body>"+deep))

	got := fetchOpenGraphDataInternal(context.Background(), srv.URL, srv.Client())

	if !isZeroResult(got) {
		t.Fatalf("quero resultado zero quando o HTML nao parseia, tenho %+v", got)
	}
}

func TestFetchOpenGraphDataInternalMetadados(t *testing.T) {
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

			got := fetchOpenGraphDataInternal(context.Background(), srv.URL, srv.Client())

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

func TestFetchOpenGraphDataInternalSeletoresDeImagem(t *testing.T) {
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

			got := fetchOpenGraphDataInternal(context.Background(), srv.URL+"/pagina", srv.Client())

			if len(got.ImageData) == 0 {
				t.Fatalf("miniatura vazia para o seletor %s", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetOpenGraphData
// ---------------------------------------------------------------------------

func TestGetOpenGraphDataCacheHit(t *testing.T) {
	key := "https://cache-hit.exemplo/artigo"
	want := OpenGraphResult{Title: "Do cache"}
	openGraphCache.Set(key, want, cache.DefaultExpiration)
	t.Cleanup(func() { openGraphCache.Delete(key) })

	// httpClient nil provaria a falta do cache com um panico.
	got := GetOpenGraphData(context.Background(), key, "usuario-cache", nil)

	if got.Title != "Do cache" {
		t.Fatalf("Title = %q, quero 'Do cache'", got.Title)
	}
}

func TestGetOpenGraphDataCacheComTipoErrado(t *testing.T) {
	key := "https://cache-tipo-errado.exemplo/artigo"
	openGraphCache.Set(key, "isso nao e' um OpenGraphResult", cache.DefaultExpiration)
	t.Cleanup(func() { openGraphCache.Delete(key) })
	srv := serveBytes(t, "text/html", []byte(`<html><head><title>Refetch</title></head></html>`))

	got := GetOpenGraphData(context.Background(), srv.URL+"?k=tipo-errado", "usuario-tipo", srv.Client())
	t.Cleanup(func() { openGraphCache.Delete(srv.URL + "?k=tipo-errado") })

	if got.Title != "Refetch" {
		t.Fatalf("Title = %q, quero 'Refetch' (cache invalido deve ser ignorado)", got.Title)
	}
}

func TestGetOpenGraphDataBuscaEArmazenaNoCache(t *testing.T) {
	var hits int
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html")
		if _, err := w.Write([]byte(`<html><head><title>Buscado</title></head></html>`)); err != nil {
			t.Errorf("escrever: %v", err)
		}
	})
	key := srv.URL + "?k=armazena"
	t.Cleanup(func() { openGraphCache.Delete(key) })

	first := GetOpenGraphData(context.Background(), key, "usuario-store", srv.Client())
	second := GetOpenGraphData(context.Background(), key, "usuario-store", srv.Client())

	if first.Title != "Buscado" || second.Title != "Buscado" {
		t.Fatalf("titulos = %q / %q", first.Title, second.Title)
	}
	if hits != 1 {
		t.Fatalf("requisicoes ao servidor = %d, quero 1 (a segunda vem do cache)", hits)
	}
}

func TestGetOpenGraphDataRecuperaDePanico(t *testing.T) {
	key := "https://panico.exemplo/artigo"
	t.Cleanup(func() { openGraphCache.Delete(key) })

	// httpClient nil faz httpClient.Do entrar em panico dentro da closure do
	// singleflight; o resultado tem de ser um zero value, nao um crash.
	got := GetOpenGraphData(context.Background(), key, "usuario-panico", nil)

	if !isZeroResult(got) {
		t.Fatalf("quero resultado zero apos o panico, tenho %+v", got)
	}
}

func TestGetOpenGraphDataSemVagaNoPool(t *testing.T) {
	userID := "usuario-lotado"
	pool := userSemaphoreManager.ForUser(userID)
	for i := 0; i < openGraphUserFetchLimit; i++ {
		pool <- struct{}{}
	}
	t.Cleanup(func() {
		for i := 0; i < openGraphUserFetchLimit; i++ {
			<-pool
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	key := "https://pool-lotado.exemplo/artigo"
	t.Cleanup(func() { openGraphCache.Delete(key) })

	got := GetOpenGraphData(ctx, key, userID, http.DefaultClient)

	if !isZeroResult(got) {
		t.Fatalf("quero resultado zero quando nao ha vaga, tenho %+v", got)
	}
}
