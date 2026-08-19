package chat

import (
	"context"
	"errors"
	"fmt"
	"testing"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"

	"wa-api/pkg/domain"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"
)

// --- MediaDownloaderAdapter (CAP-09B) ---
//
// A tabela abaixo é a asserção central do adapter: para cada uma das cinco
// capabilities, qual TIPO de sub-mensagem protobuf tem de chegar a
// Client.Download. Uma cópia trocada (sticker montando ImageMessage, por
// exemplo) muda o MediaType que o SDK deriva por tipo
// (internal/wa-noise/capabilities/media, GetType) e, com ele, a chave HKDF de
// decifragem — falha que só apareceria contra o servidor real.

// downloadKindCase liga capability, rota, kind e o tipo protobuf esperado.
type downloadKindCase struct {
	capability string
	route      string
	kind       domain.MediaKind
	wantType   string // %T da sub-mensagem que Download tem de receber
	wantMime   string // MIME representativo daquele tipo de mídia
}

func downloadKindCases() []downloadKindCase {
	return []downloadKindCase{
		{"Download Image", "/chat/downloadimage", domain.MediaKindImage, "*waE2E.ImageMessage", "image/jpeg"},
		{"Download Video", "/chat/downloadvideo", domain.MediaKindVideo, "*waE2E.VideoMessage", "video/mp4"},
		{"Download Audio", "/chat/downloadaudio", domain.MediaKindAudio, "*waE2E.AudioMessage", "audio/ogg; codecs=opus"},
		{"Download Document", "/chat/downloaddocument", domain.MediaKindDocument, "*waE2E.DocumentMessage", "application/pdf"},
		{"Download Sticker", "/chat/downloadsticker", domain.MediaKindSticker, "*waE2E.StickerMessage", "image/webp"},
	}
}

// downloadDescriptor é um descritor com os SETE campos distinguíveis.
func downloadDescriptor(kind domain.MediaKind, mime string) domain.MediaDescriptor {
	return domain.MediaDescriptor{
		Kind:          kind,
		URL:           "https://mmg.whatsapp.net/d/f/AbCdEf.enc",
		DirectPath:    "/v/t62.7118-24/12345_678_90.enc",
		MediaKey:      []byte{0x01, 0x02, 0x03, 0x04},
		Mimetype:      mime,
		FileEncSHA256: []byte{0xaa, 0xbb},
		FileSHA256:    []byte{0xcc, 0xdd},
		FileLength:    4242,
	}
}

// downloadableFields é a leitura dos sete campos por meio dos getters que o
// PRÓPRIO SDK usa para baixar (media.Downloadable: GetMediaKey,
// GetFileEncSHA256, GetFileSHA256, GetFileLength; e GetURL/GetDirectPath/
// GetMimetype nos tipos concretos). Ler pelos getters, e não pelos campos, é
// o que garante que o teste mede o que o download mede.
type downloadableFields struct {
	url, directPath, mimetype        string
	mediaKey, encSHA256, plainSHA256 []byte
	fileLength                       uint64
}

// readDownloadable extrai os sete campos da sub-mensagem, seja ela qual for.
// O switch replica, tipo a tipo, os cinco ramos do fluxo histórico
// (`git show 41bc8e2^:handlers.go`, DownloadImage na linha 3836 e as quatro
// funções irmãs) — não é um dublê permissivo: se o adapter montar um tipo que
// não está aqui, o teste falha em vez de aceitar.
func readDownloadable(t *testing.T, msg wanoise.DownloadableMessage) downloadableFields {
	t.Helper()
	switch m := msg.(type) {
	case *waE2E.ImageMessage:
		return downloadableFields{m.GetURL(), m.GetDirectPath(), m.GetMimetype(), m.GetMediaKey(), m.GetFileEncSHA256(), m.GetFileSHA256(), m.GetFileLength()}
	case *waE2E.VideoMessage:
		return downloadableFields{m.GetURL(), m.GetDirectPath(), m.GetMimetype(), m.GetMediaKey(), m.GetFileEncSHA256(), m.GetFileSHA256(), m.GetFileLength()}
	case *waE2E.AudioMessage:
		return downloadableFields{m.GetURL(), m.GetDirectPath(), m.GetMimetype(), m.GetMediaKey(), m.GetFileEncSHA256(), m.GetFileSHA256(), m.GetFileLength()}
	case *waE2E.DocumentMessage:
		return downloadableFields{m.GetURL(), m.GetDirectPath(), m.GetMimetype(), m.GetMediaKey(), m.GetFileEncSHA256(), m.GetFileSHA256(), m.GetFileLength()}
	case *waE2E.StickerMessage:
		return downloadableFields{m.GetURL(), m.GetDirectPath(), m.GetMimetype(), m.GetMediaKey(), m.GetFileEncSHA256(), m.GetFileSHA256(), m.GetFileLength()}
	default:
		t.Fatalf("sub-mensagem de tipo inesperado: %T", msg)
		return downloadableFields{}
	}
}

func TestNewMediaDownloaderAdapter(t *testing.T) {
	if NewMediaDownloaderAdapter(testkit.GetterWith(nil)) == nil {
		t.Fatal("NewMediaDownloaderAdapter returned nil")
	}
}

// TestMediaDownloaderAdapter_BuildsTheRightProtobuf: cada kind vira o tipo
// protobuf correspondente, com os sete campos intactos, e os bytes do SDK
// voltam sem transformação (nada de base64 nem Data URL neste nível).
func TestMediaDownloaderAdapter_BuildsTheRightProtobuf(t *testing.T) {
	for _, c := range downloadKindCases() {
		t.Run(c.capability, func(t *testing.T) {
			want := []byte{0xde, 0xad, 0xbe, 0xef}
			var seen wanoise.DownloadableMessage
			calls := 0

			fake := &testkit.Fake{
				DownloadFn: func(_ context.Context, msg wanoise.DownloadableMessage) ([]byte, error) {
					calls++
					seen = msg
					return want, nil
				},
			}
			a := NewMediaDownloaderAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))

			desc := downloadDescriptor(c.kind, c.wantMime)
			got, err := a.Download(context.Background(), "u1", desc)
			if err != nil {
				t.Fatalf("Download (%s): %v", c.route, err)
			}
			if calls != 1 {
				t.Fatalf("Client.Download chamado %d vez(es), quero 1", calls)
			}
			if string(got) != string(want) {
				t.Errorf("bytes: got %x, want %x (o adapter nao pode transformar)", got, want)
			}
			if gotType := fmt.Sprintf("%T", seen); gotType != c.wantType {
				t.Fatalf("%s montou %s, want %s", c.capability, gotType, c.wantType)
			}

			f := readDownloadable(t, seen)
			if f.url != desc.URL {
				t.Errorf("URL: got %q, want %q", f.url, desc.URL)
			}
			if f.directPath != desc.DirectPath {
				t.Errorf("DirectPath: got %q, want %q", f.directPath, desc.DirectPath)
			}
			if f.mimetype != desc.Mimetype {
				t.Errorf("Mimetype: got %q, want %q", f.mimetype, desc.Mimetype)
			}
			if string(f.mediaKey) != string(desc.MediaKey) {
				t.Errorf("MediaKey: got %x, want %x", f.mediaKey, desc.MediaKey)
			}
			if string(f.encSHA256) != string(desc.FileEncSHA256) {
				t.Errorf("FileEncSHA256: got %x, want %x", f.encSHA256, desc.FileEncSHA256)
			}
			if string(f.plainSHA256) != string(desc.FileSHA256) {
				t.Errorf("FileSHA256: got %x, want %x", f.plainSHA256, desc.FileSHA256)
			}
			if f.fileLength != desc.FileLength {
				t.Errorf("FileLength: got %d, want %d", f.fileLength, desc.FileLength)
			}
		})
	}
}

// TestMediaDownloaderAdapter_EachKindBuildsADistinctType: os cinco kinds
// produzem cinco tipos DIFERENTES. Um adapter que montasse ImageMessage para
// tudo passaria em cada subteste da tabela acima isoladamente se o wantType
// fosse frouxo; aqui a distinção é medida no conjunto.
func TestMediaDownloaderAdapter_EachKindBuildsADistinctType(t *testing.T) {
	seenTypes := map[string]string{}

	for _, c := range downloadKindCases() {
		var seen wanoise.DownloadableMessage
		fake := &testkit.Fake{
			DownloadFn: func(_ context.Context, msg wanoise.DownloadableMessage) ([]byte, error) {
				seen = msg
				return []byte{0x01}, nil
			},
		}
		a := NewMediaDownloaderAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
		if _, err := a.Download(context.Background(), "u1", downloadDescriptor(c.kind, c.wantMime)); err != nil {
			t.Fatalf("Download (%s): %v", c.route, err)
		}
		gotType := fmt.Sprintf("%T", seen)
		if prev, dup := seenTypes[gotType]; dup {
			t.Fatalf("%s e %s montaram o MESMO tipo %s", prev, c.capability, gotType)
		}
		seenTypes[gotType] = c.capability
	}

	if len(seenTypes) != 5 {
		t.Fatalf("os cinco kinds produziram %d tipos distintos: %v", len(seenTypes), seenTypes)
	}
}

// TestMediaDownloaderAdapter_NoSession: sem cliente, erro tipado e o SDK nunca
// é alcançado.
func TestMediaDownloaderAdapter_NoSession(t *testing.T) {
	a := NewMediaDownloaderAdapter(testkit.GetterWith(nil))
	_, err := a.Download(context.Background(), "u1", downloadDescriptor(domain.MediaKindImage, "image/jpeg"))
	if err == nil {
		t.Fatal("sem sessao, Download devolveu nil")
	}
	if code := testkit.AppErrCode(err); code != "no_session" {
		t.Fatalf("codigo do erro: got %q, want no_session", code)
	}
}

// TestMediaDownloaderAdapter_SDKErrorPropagates: erro do SDK sobe como está —
// nunca vira bytes vazios com erro nil.
func TestMediaDownloaderAdapter_SDKErrorPropagates(t *testing.T) {
	sentinel := errors.New("sdk-download-failed")
	fake := &testkit.Fake{
		DownloadFn: func(context.Context, wanoise.DownloadableMessage) ([]byte, error) {
			return nil, sentinel
		},
	}
	a := NewMediaDownloaderAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))

	got, err := a.Download(context.Background(), "u1", downloadDescriptor(domain.MediaKindVideo, "video/mp4"))
	if !errors.Is(err, sentinel) {
		t.Fatalf("erro do SDK nao propagou: got %#v", err)
	}
	if got != nil {
		t.Errorf("bytes devolvidos junto com erro: %x", got)
	}
}

// TestMediaDownloaderAdapter_UnknownKind: kind fora dos cinco é defeito de
// programação e não pode virar chamada ao SDK com sub-mensagem nil.
func TestMediaDownloaderAdapter_UnknownKind(t *testing.T) {
	calls := 0
	fake := &testkit.Fake{
		DownloadFn: func(context.Context, wanoise.DownloadableMessage) ([]byte, error) {
			calls++
			return []byte{0x01}, nil
		},
	}
	a := NewMediaDownloaderAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))

	_, err := a.Download(context.Background(), "u1", domain.MediaDescriptor{Kind: domain.MediaKind("carrier-pigeon")})
	if err == nil {
		t.Fatal("kind desconhecido foi aceito")
	}
	if code := testkit.AppErrCode(err); code != "unknown_media_kind" {
		t.Fatalf("codigo do erro: got %q, want unknown_media_kind", code)
	}
	if calls != 0 {
		t.Fatalf("kind desconhecido alcancou o SDK %d vez(es)", calls)
	}
}
