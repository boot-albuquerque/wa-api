package media

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
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waMediaTransport"
)

func TestDownloadAnyEscolheAPrimeiraParteNaoNula(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	ctx := context.Background()

	if _, err := DownloadAny(ctx, tr, nil); !errors.Is(err, ErrNothingDownloadableFound) {
		t.Fatalf("mensagem nil = %v, esperado ErrNothingDownloadableFound", err)
	}
	if _, err := DownloadAny(ctx, tr, &waE2E.Message{}); !errors.Is(err, ErrNothingDownloadableFound) {
		t.Fatalf("mensagem sem midia = %v, esperado ErrNothingDownloadableFound", err)
	}

	// Com uma parte baixavel presente, DownloadAny delega para DownloadMessage,
	// que numa mensagem vazia devolve ErrNoURLPresent — e' o marcador de que a
	// parte foi de fato selecionada (e nao ErrNothingDownloadableFound).
	msgs := map[string]*waE2E.Message{
		"imagem":    {ImageMessage: &waE2E.ImageMessage{}},
		"video":     {VideoMessage: &waE2E.VideoMessage{}},
		"audio":     {AudioMessage: &waE2E.AudioMessage{}},
		"documento": {DocumentMessage: &waE2E.DocumentMessage{}},
		"figurinha": {StickerMessage: &waE2E.StickerMessage{}},
	}
	for name, msg := range msgs {
		t.Run(name, func(t *testing.T) {
			if _, err := DownloadAny(ctx, tr, msg); !errors.Is(err, ErrNoURLPresent) {
				t.Fatalf("erro = %v, esperado ErrNoURLPresent (parte selecionada)", err)
			}
		})
	}
}

func TestDownloadMessageRejeitaMensagemSemURLEsemDirectPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	if _, err := DownloadMessage(context.Background(), tr, &waE2E.ImageMessage{}); !errors.Is(err, ErrNoURLPresent) {
		t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
	}
	// URL de web.whatsapp.net sem directPath tambem cai em ErrNoURLPresent:
	// esse host nao serve midia.
	msg := &waE2E.ImageMessage{URL: proto.String(webWhatsappNetURLPrefix + "/qualquer")}
	if _, err := DownloadMessage(context.Background(), tr, msg); !errors.Is(err, ErrNoURLPresent) {
		t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
	}
}

func TestDownloadMessageRejeitaTipoDesconhecido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	if _, err := DownloadMessage(context.Background(), tr, notDownloadable{}); !errors.Is(err, ErrUnknownMediaType) {
		t.Fatalf("erro = %v, esperado ErrUnknownMediaType", err)
	}
}

// Uma mensagem com URL direta (nao web.whatsapp.net) baixa sem consultar a
// media connection.
func TestDownloadMessageUsaAURLDireta(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x0C}, mediaKeyLength)
	plaintext := []byte("baixado pela URL direta")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	// Zera o cache e nao configura IQ: se o codigo tentasse resolver a
	// mediaConn, o download falharia.
	tr.conn.Set(nil)

	msg := &waE2E.ImageMessage{
		URL:           proto.String("https://mmg.whatsapp.net/direta"),
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
}

func TestDownloadThumbnail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	ctx := context.Background()

	t.Run("sem thumbnail direct path", func(t *testing.T) {
		if _, err := DownloadThumbnail(ctx, tr, &waE2E.ExtendedTextMessage{}); !errors.Is(err, ErrNoURLPresent) {
			t.Fatalf("erro = %v, esperado ErrNoURLPresent", err)
		}
	})
	t.Run("mensagem sem thumbnail conhecido", func(t *testing.T) {
		if _, err := DownloadThumbnail(ctx, tr, &waE2E.ImageMessage{}); !errors.Is(err, ErrUnknownMediaType) {
			t.Fatalf("erro = %v, esperado ErrUnknownMediaType", err)
		}
	})
}

// O caminho feliz de DownloadThumbnail: usa o thumbnail direct path e o
// mms-type de MediaLinkThumbnail.
func TestDownloadThumbnailBaixaPeloDirectPath(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x0D}, mediaKeyLength)
	plaintext := []byte("thumb")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeLinkThumbnail)

	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	msg := &waE2E.ExtendedTextMessage{
		ThumbnailDirectPath: proto.String("/v/thumb"),
		ThumbnailEncSHA256:  encSHA,
		ThumbnailSHA256:     plainSHA,
		MediaKey:            mediaKey,
	}
	got, err := DownloadThumbnail(context.Background(), tr, msg)
	if err != nil {
		t.Fatalf("DownloadThumbnail devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("conteudo = %q, esperado %q", got, plaintext)
	}
	if !strings.Contains(gotURI, "mms-type="+mediaTypeToMMSType[TypeLinkThumbnail]) {
		t.Errorf("URI %q nao usa o mms-type de thumbnail", gotURI)
	}
}

func TestDownloadWithPathExigeBarraInicial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	_, err := DownloadWithPath(
		context.Background(), tr, "sem-barra", nil, nil, nil, UnknownFileLength, TypeImage, "")
	if err == nil || !strings.Contains(err.Error(), "does not start with slash") {
		t.Fatalf("erro = %v, esperado recusa por falta de barra inicial", err)
	}
}

// Se a media connection nao puder ser resolvida, o erro sobe embrulhado.
func TestDownloadWithPathPropagaErroDaMediaConn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	tr.conn.Set(nil)

	_, err := DownloadWithPath(
		context.Background(), tr, "/v/x", nil, nil, nil, UnknownFileLength, TypeImage, "")
	if err == nil || !strings.Contains(err.Error(), "failed to refresh media connections") {
		t.Fatalf("erro = %v, esperado embrulho de falha na mediaConn", err)
	}
}

func TestDownloadWithPathMontaAURLEBaixa(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x06}, mediaKeyLength)
	plaintext := []byte("conteudo via direct path")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)
	const directPath = "/v/t62.7118-24/abc?ccb=11-4"

	var gotURI, gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		gotHost = r.Header.Get(testOrigHostHeader)
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv, "primeiro.example")

	got, err := DownloadWithPath(
		context.Background(), tr, directPath, encSHA, plainSHA, mediaKey, len(plaintext), TypeImage, "")
	if err != nil {
		t.Fatalf("DownloadWithPath devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("conteudo = %q, esperado %q", got, plaintext)
	}
	if gotHost != "primeiro.example" {
		t.Errorf("host = %q, esperado o primeiro host da mediaConn", gotHost)
	}
	// mmsType vazio precisa cair no padrao do mediaType.
	if !strings.Contains(gotURI, "mms-type="+mediaTypeToMMSType[TypeImage]) {
		t.Errorf("URI %q nao contem o mms-type padrao de TypeImage", gotURI)
	}
	if !strings.Contains(gotURI, "hash="+base64.URLEncoding.EncodeToString(encSHA)) {
		t.Errorf("URI %q nao contem o hash do arquivo cifrado", gotURI)
	}
	if !strings.HasPrefix(gotURI, directPath) {
		t.Errorf("URI %q nao comeca com o directPath", gotURI)
	}
}

func TestDownloadWithPathTrocaDeHostEmFalhaRetentavel(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x07}, mediaKeyLength)
	plaintext := []byte("so' o segundo host responde")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)

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
	tr := newTestTransport(t, srv, "quebrado.example", "bom.example")

	got, err := DownloadWithPath(
		context.Background(), tr, "/v/x", encSHA, plainSHA, mediaKey, len(plaintext), TypeImage, "image")
	if err != nil {
		t.Fatalf("DownloadWithPath devolveu erro: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("conteudo = %q, esperado %q", got, plaintext)
	}
	if hostsVistos.Load() == 0 {
		t.Error("o primeiro host nunca foi tentado")
	}
}

// Quando o ultimo host tambem falha com erro retentavel, o erro sai embrulhado
// como "failed to download media from last host".
func TestDownloadWithPathFalhaNoUltimoHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv, "a.example", "b.example")

	_, err := DownloadWithPath(
		context.Background(), tr, "/v/x", nil, nil, bytes.Repeat([]byte{0x01}, mediaKeyLength),
		UnknownFileLength, TypeImage, "image")
	if err == nil || !strings.Contains(err.Error(), "from last host") {
		t.Fatalf("erro = %v, esperado falha no ultimo host", err)
	}
}

// 404 e' erro terminal: nao adianta tentar outro host, o arquivo nao existe.
func TestDownloadWithPathNaoTrocaDeHostEm404(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv, "a.example", "b.example")

	_, err := DownloadWithPath(
		context.Background(), tr, "/v/x", bytes.Repeat([]byte{0x00}, sha256HashLength),
		nil, bytes.Repeat([]byte{0x08}, mediaKeyLength), UnknownFileLength, TypeImage, "image")
	if !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("erro = %v, esperado ErrMediaDownloadFailedWith404", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("houve %d requisicoes, esperado 1 (sem troca de host)", got)
	}
}

// Uma mediaConn sem nenhum host faz o laco nao executar: sem dado e sem erro.
func TestDownloadWithPathSemHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	tr := newTestTransport(t, srv)
	tr.conn.Set(&Conn{TTL: 3600, FetchedAt: time.Now()})

	data, err := DownloadWithPath(
		context.Background(), tr, "/v/x", nil, nil, nil, UnknownFileLength, TypeImage, "image")
	if err != nil || data != nil {
		t.Fatalf("data = %v, err = %v; esperado ambos nil com lista de hosts vazia", data, err)
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
	tr := newTestTransport(t, srv)

	pack, err := FetchStickerPack(context.Background(), tr, "pack-123")
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
	if _, err = FetchStickerPack(context.Background(), tr, "vazio"); err == nil {
		t.Error("resposta sem pacotes deveria virar erro")
	}

	body = `nao e json`
	if _, err = FetchStickerPack(context.Background(), tr, "quebrado"); err == nil ||
		!strings.Contains(err.Error(), "failed to decode response") {
		t.Errorf("erro = %v, esperado falha de decodificacao", err)
	}
}

func TestFetchStickerPackPropagaErroHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	if _, err := FetchStickerPack(context.Background(), tr, "x"); !errors.Is(err, ErrMediaDownloadFailedWith404) {
		t.Fatalf("erro = %v, esperado ErrMediaDownloadFailedWith404", err)
	}
}

func TestDownloadFBUsaODirectPathDoTransporte(t *testing.T) {
	mediaKey := bytes.Repeat([]byte{0x0B}, mediaKeyLength)
	plaintext := []byte("midia via WAMediaTransport")
	blob, encSHA, plainSHA := encryptedMediaBlob(t, mediaKey, plaintext, TypeImage)

	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		_, _ = w.Write(blob)
	}))
	defer srv.Close()
	tr := newTestTransport(t, srv)

	transport := &waMediaTransport.WAMediaTransport_Integral{
		DirectPath:    proto.String("/v/fb-path"),
		MediaKey:      mediaKey,
		FileEncSHA256: encSHA,
		FileSHA256:    plainSHA,
	}
	got, err := DownloadFB(context.Background(), tr, transport, TypeImage)
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

func TestBuildDownloadURL(t *testing.T) {
	got := buildDownloadURL("host.example", "/v/x?a=1", []byte{0x01, 0x02}, "image")
	const want = "https://host.example/v/x?a=1&hash=AQI=&mms-type=image&__wa-mms="
	if got != want {
		t.Fatalf("buildDownloadURL() = %q, esperado %q", got, want)
	}
}

func TestIsTerminalDownloadResult(t *testing.T) {
	terminais := []error{
		nil, ErrFileLengthMismatch, ErrInvalidMediaSHA256,
		ErrMediaDownloadFailedWith403, ErrMediaDownloadFailedWith404,
		ErrMediaDownloadFailedWith410, context.Canceled,
	}
	for _, err := range terminais {
		if !isTerminalDownloadResult(err) {
			t.Errorf("isTerminalDownloadResult(%v) = false, esperado true", err)
		}
	}
	if isTerminalDownloadResult(ErrTooShortFile) {
		t.Error("ErrTooShortFile nao deveria encerrar o laco de hosts")
	}
}

func TestDirectURL(t *testing.T) {
	// Mensagem sem GetURL cai no ramo "sem URL".
	if url, isWeb := directURL(notDownloadable{}); url != "" || isWeb {
		t.Fatalf("directURL() = (%q, %v), esperado (\"\", false)", url, isWeb)
	}
	msg := &waE2E.ImageMessage{URL: proto.String(webWhatsappNetURLPrefix + "/x")}
	if url, isWeb := directURL(msg); !isWeb || url == "" {
		t.Fatalf("directURL() = (%q, %v), esperado URL web.whatsapp.net detectada", url, isWeb)
	}
}
