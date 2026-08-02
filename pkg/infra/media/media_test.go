package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// ---------------------------------------------------------------------------
// Duplos
// ---------------------------------------------------------------------------

type fakeMyClient struct {
	userID string
	wa     *whatsmeow.Client
}

func (f fakeMyClient) GetWAClient() *whatsmeow.Client { return f.wa }
func (f fakeMyClient) GetUserID() string              { return f.userID }

type fakeS3Manager struct {
	result map[string]interface{}
	err    error
	calls  int
	gotFn  string
	gotIn  bool
}

func (f *fakeS3Manager) ProcessMediaForS3(_ context.Context, _, _, _ string, _ []byte, _, fileName string, isIncoming bool) (map[string]interface{}, error) {
	f.calls++
	f.gotFn = fileName
	f.gotIn = isIncoming
	return f.result, f.err
}

// mediaServer devolve `body` como midia nao criptografada. Uma mensagem sem
// MediaKey e sem FileEncSHA256 faz o whatsmeow entregar os bytes crus, o que
// da' um seam de download real sem sessao nem criptografia.
func mediaServer(t *testing.T, body []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(body); err != nil {
			t.Errorf("escrever corpo: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func downloadableFrom(url string) whatsmeow.DownloadableMessage {
	return &waE2E.ImageMessage{URL: proto.String(url)}
}

// resetHandler restaura o handler global depois do teste — ele e' estado de
// pacote e vazaria entre casos.
func resetHandler(t *testing.T) {
	t.Helper()
	previous := defaultHandler
	t.Cleanup(func() { SetProcessMediaHandler(previous) })
}

// userIDForTmp devolve um userID unico e o diretorio /tmp correspondente,
// removido ao fim do teste. ProcessMedia grava sempre sob /tmp/user_<id>.
func userIDForTmp(t *testing.T) string {
	t.Helper()
	userID := "wa-api-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	t.Cleanup(func() {
		if err := os.RemoveAll(filepath.Join("/tmp", "user_"+userID)); err != nil {
			t.Errorf("limpar tmpdir: %v", err)
		}
	})
	return userID
}

// ---------------------------------------------------------------------------
// SetProcessMediaHandler
// ---------------------------------------------------------------------------

func TestSetProcessMediaHandler(t *testing.T) {
	resetHandler(t)

	h := &ProcessMediaHandler{}
	SetProcessMediaHandler(h)

	if defaultHandler != h {
		t.Fatal("handler global nao foi trocado")
	}
}

// ---------------------------------------------------------------------------
// ProcessMedia
// ---------------------------------------------------------------------------

func TestProcessMediaFalhaAoCriarDiretorio(t *testing.T) {
	resetHandler(t)
	SetProcessMediaHandler(nil)

	// /tmp/user_<id> ja' existe como ARQUIVO: MkdirAll falha.
	blocker := filepath.Join("/tmp", "user_wa-api-test-blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("criar bloqueador: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(blocker); err != nil {
			t.Errorf("remover bloqueador: %v", err)
		}
	})

	postmap := map[string]interface{}{}
	ProcessMedia(
		fakeMyClient{userID: "wa-api-test-blocker"},
		downloadableFrom(mediaServer(t, []byte("x"))),
		"image/png", ".png", time.Second, true, "chat@s.whatsapp.net", "msg-1",
		MediaS3Config{}, postmap, nil,
	)

	if len(postmap) != 0 {
		t.Fatalf("postmap deveria ficar intacto, tenho %v", postmap)
	}
}

func TestProcessMediaFalhaNoDownload(t *testing.T) {
	resetHandler(t)
	SetProcessMediaHandler(nil)
	userID := userIDForTmp(t)

	postmap := map[string]interface{}{}
	// Cliente whatsmeow nil: Download devolve ErrClientIsNil.
	ProcessMedia(
		fakeMyClient{userID: userID, wa: nil},
		downloadableFrom("https://exemplo.invalido/x.png"),
		"image/png", ".png", time.Second, true, "chat@s.whatsapp.net", "msg-1",
		MediaS3Config{}, postmap, nil,
	)

	if len(postmap) != 0 {
		t.Fatalf("postmap deveria ficar intacto, tenho %v", postmap)
	}
}

func TestProcessMediaFalhaAoGravarArquivoTemporario(t *testing.T) {
	resetHandler(t)
	SetProcessMediaHandler(nil)
	userID := userIDForTmp(t)

	postmap := map[string]interface{}{}
	// messageID com separador aponta para um subdiretorio inexistente.
	ProcessMedia(
		fakeMyClient{userID: userID, wa: whatsmeow.NewClient(nil, nil)},
		downloadableFrom(mediaServer(t, []byte("conteudo"))),
		"image/png", ".png", time.Second, true, "chat@s.whatsapp.net", "sub/msg-1",
		MediaS3Config{}, postmap, nil,
	)

	if len(postmap) != 0 {
		t.Fatalf("postmap deveria ficar intacto, tenho %v", postmap)
	}
}

func TestProcessMediaSemHandlerGlobal(t *testing.T) {
	resetHandler(t)
	SetProcessMediaHandler(nil)
	userID := userIDForTmp(t)

	postmap := map[string]interface{}{}
	ProcessMedia(
		fakeMyClient{userID: userID, wa: whatsmeow.NewClient(nil, nil)},
		downloadableFrom(mediaServer(t, []byte("conteudo"))),
		"image/png", ".png", time.Second, true, "chat@s.whatsapp.net", "msg-1",
		MediaS3Config{Enabled: "true", MediaDelivery: "both"}, postmap,
		map[string]interface{}{"extra": 42},
	)

	if postmap["extra"] != 42 {
		t.Fatalf("extraKeys nao foram copiadas: %v", postmap)
	}
	if _, ok := postmap["base64"]; ok {
		t.Fatal("sem handler global nao deveria haver base64")
	}
}

func TestProcessMediaUsaExtensaoDoMimeType(t *testing.T) {
	resetHandler(t)
	userID := userIDForTmp(t)

	var gotPath string
	SetProcessMediaHandler(&ProcessMediaHandler{
		FileToBase64Func: func(path string) (string, string, error) {
			gotPath = path
			return "ZGFkb3M=", "image/png", nil
		},
	})

	postmap := map[string]interface{}{}
	ProcessMedia(
		fakeMyClient{userID: userID, wa: whatsmeow.NewClient(nil, nil)},
		downloadableFrom(mediaServer(t, []byte("conteudo"))),
		"image/png", ".fallback", time.Second, true, "chat@s.whatsapp.net", "msg-ext",
		MediaS3Config{MediaDelivery: "base64"}, postmap, nil,
	)

	if filepath.Ext(gotPath) != ".png" {
		t.Fatalf("extensao = %q, quero .png derivada do mimeType", filepath.Ext(gotPath))
	}
	if postmap["base64"] != "ZGFkb3M=" || postmap["mimeType"] != "image/png" {
		t.Fatalf("postmap = %v", postmap)
	}
	if postmap["fileName"] != "msg-ext.png" {
		t.Fatalf("fileName = %v, quero msg-ext.png", postmap["fileName"])
	}
}

func TestProcessMediaUsaFallbackExtQuandoMimeDesconhecido(t *testing.T) {
	resetHandler(t)
	userID := userIDForTmp(t)

	var gotPath string
	SetProcessMediaHandler(&ProcessMediaHandler{
		FileToBase64Func: func(path string) (string, string, error) {
			gotPath = path
			return "", "", nil
		},
	})

	postmap := map[string]interface{}{}
	ProcessMedia(
		fakeMyClient{userID: userID, wa: whatsmeow.NewClient(nil, nil)},
		downloadableFrom(mediaServer(t, []byte("conteudo"))),
		"application/x-wa-api-inexistente", ".bin", time.Second, true, "chat@s.whatsapp.net", "msg-fb",
		MediaS3Config{MediaDelivery: "base64"}, postmap, nil,
	)

	if filepath.Ext(gotPath) != ".bin" {
		t.Fatalf("extensao = %q, quero o fallback .bin", filepath.Ext(gotPath))
	}
}

func TestProcessMediaS3(t *testing.T) {
	tests := []struct {
		name          string
		enabled       string
		mediaDelivery string
		wantCalls     int
	}{
		{"s3 desligado nao chama o manager", "false", "s3", 0},
		{"entrega base64 nao chama o manager", "true", "base64", 0},
		{"entrega s3 chama o manager", "true", "s3", 1},
		{"entrega both chama o manager", "true", "both", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetHandler(t)
			userID := userIDForTmp(t)
			s3 := &fakeS3Manager{result: map[string]interface{}{"url": "s3://bucket/key"}}
			SetProcessMediaHandler(&ProcessMediaHandler{
				S3Manager: s3,
				FileToBase64Func: func(string) (string, string, error) {
					return "Yg==", "image/png", nil
				},
			})

			postmap := map[string]interface{}{}
			ProcessMedia(
				fakeMyClient{userID: userID, wa: whatsmeow.NewClient(nil, nil)},
				downloadableFrom(mediaServer(t, []byte("conteudo"))),
				"image/png", ".png", time.Second, false, "chat@s.whatsapp.net", "msg-s3",
				MediaS3Config{Enabled: tc.enabled, MediaDelivery: tc.mediaDelivery}, postmap, nil,
			)

			if s3.calls != tc.wantCalls {
				t.Fatalf("chamadas ao S3 = %d, quero %d", s3.calls, tc.wantCalls)
			}
			if tc.wantCalls > 0 {
				if postmap["s3"] == nil {
					t.Fatalf("postmap sem chave s3: %v", postmap)
				}
				if s3.gotFn != "msg-s3.png" {
					t.Fatalf("fileName recebido = %q", s3.gotFn)
				}
				if s3.gotIn {
					t.Fatal("isIncoming deveria ser false")
				}
			}
		})
	}
}

func TestProcessMediaErroDoS3NaoInterrompe(t *testing.T) {
	resetHandler(t)
	userID := userIDForTmp(t)
	s3 := &fakeS3Manager{err: errors.New("bucket indisponivel")}
	SetProcessMediaHandler(&ProcessMediaHandler{
		S3Manager: s3,
		FileToBase64Func: func(string) (string, string, error) {
			return "Yg==", "image/png", nil
		},
	})

	postmap := map[string]interface{}{}
	ProcessMedia(
		fakeMyClient{userID: userID, wa: whatsmeow.NewClient(nil, nil)},
		downloadableFrom(mediaServer(t, []byte("conteudo"))),
		"image/png", ".png", time.Second, true, "chat@s.whatsapp.net", "msg-s3-err",
		MediaS3Config{Enabled: "true", MediaDelivery: "both"}, postmap,
		map[string]interface{}{"extra": "v"},
	)

	if _, ok := postmap["s3"]; ok {
		t.Fatalf("erro do S3 nao deveria popular a chave s3: %v", postmap)
	}
	if postmap["base64"] != "Yg==" {
		t.Fatalf("a entrega base64 deveria seguir apos o erro do S3: %v", postmap)
	}
	if postmap["extra"] != "v" {
		t.Fatalf("extraKeys nao foram copiadas: %v", postmap)
	}
}

func TestProcessMediaErroNoBase64Interrompe(t *testing.T) {
	resetHandler(t)
	userID := userIDForTmp(t)
	SetProcessMediaHandler(&ProcessMediaHandler{
		FileToBase64Func: func(string) (string, string, error) {
			return "", "", errors.New("falha ao converter")
		},
	})

	postmap := map[string]interface{}{}
	ProcessMedia(
		fakeMyClient{userID: userID, wa: whatsmeow.NewClient(nil, nil)},
		downloadableFrom(mediaServer(t, []byte("conteudo"))),
		"image/png", ".png", time.Second, true, "chat@s.whatsapp.net", "msg-b64-err",
		MediaS3Config{MediaDelivery: "base64"}, postmap,
		map[string]interface{}{"extra": "v"},
	)

	if _, ok := postmap["base64"]; ok {
		t.Fatalf("nao deveria haver base64: %v", postmap)
	}
	if _, ok := postmap["extra"]; ok {
		t.Fatal("o erro de base64 interrompe antes de copiar extraKeys")
	}
}

func TestProcessMediaHandlerComFuncoesNulas(t *testing.T) {
	resetHandler(t)
	userID := userIDForTmp(t)
	SetProcessMediaHandler(&ProcessMediaHandler{}) // S3Manager e FileToBase64Func nil

	postmap := map[string]interface{}{}
	ProcessMedia(
		fakeMyClient{userID: userID, wa: whatsmeow.NewClient(nil, nil)},
		downloadableFrom(mediaServer(t, []byte("conteudo"))),
		"image/png", ".png", time.Second, true, "chat@s.whatsapp.net", "msg-nulos",
		MediaS3Config{Enabled: "true", MediaDelivery: "both"}, postmap, nil,
	)

	if len(postmap) != 0 {
		t.Fatalf("postmap deveria ficar vazio, tenho %v", postmap)
	}
}

func TestProcessMediaLimpezaDoTemporarioFalhaSemQuebrar(t *testing.T) {
	resetHandler(t)
	userID := userIDForTmp(t)
	SetProcessMediaHandler(&ProcessMediaHandler{
		FileToBase64Func: func(path string) (string, string, error) {
			// Remover o temporario por baixo do defer exercita o ramo de erro
			// da limpeza sem mascarar o resultado da conversao.
			if err := os.Remove(path); err != nil {
				return "", "", err
			}
			return "Yg==", "image/png", nil
		},
	})

	postmap := map[string]interface{}{}
	ProcessMedia(
		fakeMyClient{userID: userID, wa: whatsmeow.NewClient(nil, nil)},
		downloadableFrom(mediaServer(t, []byte("conteudo"))),
		"image/png", ".png", time.Second, true, "chat@s.whatsapp.net", "msg-limpeza",
		MediaS3Config{MediaDelivery: "base64"}, postmap, nil,
	)

	if postmap["base64"] != "Yg==" {
		t.Fatalf("postmap = %v", postmap)
	}
}
