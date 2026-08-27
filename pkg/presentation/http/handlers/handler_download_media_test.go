package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// CAP-10: a rota consolidada /chats/download/{kind} dispatcha, pelo segmento
// de caminho, para a mesma primitive de download que as cinco rotas
// anteriores já exercitam em handler_download_test.go. Este arquivo cobre só
// o que é NOVO nela: o despacho por {kind} e a recusa de kind desconhecido —
// não repete os oito eixos já cobertos para as cinco rotas antigas.

func newDownloadMediaRouter(md *contractsfake.MediaDownloader) http.Handler {
	uc := message.NewDownloadMediaUseCase(
		message.NewDownloadImageUseCase(md, silentLogger{}),
		message.NewDownloadVideoUseCase(md, silentLogger{}),
		message.NewDownloadAudioUseCase(md, silentLogger{}),
		message.NewDownloadDocumentUseCase(md, silentLogger{}),
		message.NewDownloadStickerUseCase(md, silentLogger{}),
	)
	r := mux.NewRouter()
	r.Handle("/chats/download/{kind}", NewDownloadMediaHandler(uc)).Methods(http.MethodPost)
	return r
}

func downloadMediaBody(mime string) string {
	return `{"Url":"https://mmg.whatsapp.net/d/f/AbCdEf.enc",` +
		`"DirectPath":"/v/t62.7118-24/12345_678_90.enc",` +
		`"MediaKey":"` + base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04}) + `",` +
		`"Mimetype":"` + mime + `",` +
		`"FileEncSHA256":"` + base64.StdEncoding.EncodeToString([]byte{0xaa, 0xbb}) + `",` +
		`"FileSHA256":"` + base64.StdEncoding.EncodeToString([]byte{0xcc, 0xdd}) + `",` +
		`"FileLength":4242}`
}

// TestDownloadMedia_DispatchByPathKind_ViaRegisteredRoute prova, para cada um
// dos cinco kinds, que o segmento {kind} do caminho — não o Mimetype do
// corpo — é o que a porta de download recebe.
func TestDownloadMedia_DispatchByPathKind_ViaRegisteredRoute(t *testing.T) {
	cases := []struct {
		kind domain.MediaKind
		mime string
	}{
		{domain.MediaKindImage, "image/jpeg"},
		{domain.MediaKindVideo, "video/mp4"},
		{domain.MediaKindAudio, "audio/ogg"},
		{domain.MediaKindDocument, "application/pdf"},
		{domain.MediaKindSticker, "image/webp"},
	}
	payload := []byte{0x00, 0x01, 0xff, 0xfe, 'o', 'k'}

	for _, c := range cases {
		t.Run(string(c.kind), func(t *testing.T) {
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(_ context.Context, txtID string, desc domain.MediaDescriptor) ([]byte, error) {
					if desc.Kind != c.kind {
						t.Errorf("Kind: got %q, want %q (do caminho, nao do Mimetype)", desc.Kind, c.kind)
					}
					return payload, nil
				},
			}
			route := "/chats/download/" + string(c.kind)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(downloadMediaBody(c.mime)))
			newDownloadMediaRouter(md).ServeHTTP(rec, msgAuthed(req))

			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status %d (corpo: %s)", route, rec.Code, rec.Body.String())
			}
			if n := len(md.DownloadCalls); n != 1 {
				t.Fatalf("Download chamado %d vez(es) pela rota registrada, quero 1", n)
			}
		})
	}
}

// TestDownloadMedia_UnknownKind_400 prova que um {kind} fora do enum é
// 400 unknown_media_kind, e a porta nunca é alcançada — nem EnsureSession.
func TestDownloadMedia_UnknownKind_400(t *testing.T) {
	md := &contractsfake.MediaDownloader{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chats/download/carrierpigeon", strings.NewReader(downloadMediaBody("image/jpeg")))
	newDownloadMediaRouter(md).ServeHTTP(rec, msgAuthed(req))

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(md.EnsureSessionCalls); n != 0 {
		t.Errorf("kind desconhecido alcancou EnsureSession %d vez(es)", n)
	}
	if n := len(md.DownloadCalls); n != 0 {
		t.Errorf("kind desconhecido alcancou Download %d vez(es)", n)
	}

	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true num 400: %s", rec.Body.String())
	}
}

// TestDownloadMedia_BodyKindWinsOverPath: um Kind já presente no corpo NÃO é
// sobrescrito pelo {kind} do caminho — mesma convenção de
// pkg/presentation/http/canonico.go: o caminho só preenche o que falta, não
// tem autoridade sobre quem já dizia a mesma coisa.
func TestDownloadMedia_BodyKindWinsOverPath(t *testing.T) {
	payload := []byte{0x01}
	md := &contractsfake.MediaDownloader{
		DownloadFunc: func(_ context.Context, _ string, desc domain.MediaDescriptor) ([]byte, error) {
			if desc.Kind != domain.MediaKindImage {
				t.Errorf("Kind: got %q, want %q (o do corpo)", desc.Kind, domain.MediaKindImage)
			}
			return payload, nil
		},
	}
	body := `{"Kind":"image","Url":"https://mmg.whatsapp.net/d/f/AbCdEf.enc",` +
		`"MediaKey":"AQID",` +
		`"Mimetype":"video/mp4",` +
		`"FileEncSHA256":"qrs=",` +
		`"FileSHA256":"tuv=",` +
		`"FileLength":1}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chats/download/video", strings.NewReader(body))
	newDownloadMediaRouter(md).ServeHTTP(rec, msgAuthed(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (corpo: %s)", rec.Code, rec.Body.String())
	}
}
