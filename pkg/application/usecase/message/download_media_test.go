package message_test

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// CAP-09B: as cinco capabilities de download deixaram de devolver
// domain.DownloadResult{} vazio e passam a baixar de verdade pela porta
// port.MediaDownloader. Este arquivo cobre o use case; a rota registrada é
// coberta em pkg/presentation/http/handlers/handler_download_test.go, e a
// montagem protobuf em pkg/infra/wa-noise/adapters/chat/downloader_test.go.
//
// A tabela é ENUMERADA nome por nome (capability + kind esperado), não um
// laço sobre uma lista anônima: o defeito que CAP-09B corrigiu foi
// exatamente cinco cópias divergindo em silêncio.

// downloadCapability é uma das cinco capabilities, com o kind que ela TEM de
// passar à porta.
type downloadCapability struct {
	capability string           // nome da capability, como no packet
	route      string           // rota registrada correspondente
	wantKind   domain.MediaKind // kind esperado no descritor
	newUC      func(md port.MediaDownloader, l port.Logger) func(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error)
}

func downloadCapabilities() []downloadCapability {
	return []downloadCapability{
		{"Download Image", "/chat/downloadimage", domain.MediaKindImage,
			func(md port.MediaDownloader, l port.Logger) func(context.Context, string, domain.DownloadRequest) (*domain.DownloadResult, error) {
				return message.NewDownloadImageUseCase(md, l).Execute
			}},
		{"Download Video", "/chat/downloadvideo", domain.MediaKindVideo,
			func(md port.MediaDownloader, l port.Logger) func(context.Context, string, domain.DownloadRequest) (*domain.DownloadResult, error) {
				return message.NewDownloadVideoUseCase(md, l).Execute
			}},
		{"Download Audio", "/chat/downloadaudio", domain.MediaKindAudio,
			func(md port.MediaDownloader, l port.Logger) func(context.Context, string, domain.DownloadRequest) (*domain.DownloadResult, error) {
				return message.NewDownloadAudioUseCase(md, l).Execute
			}},
		{"Download Document", "/chat/downloaddocument", domain.MediaKindDocument,
			func(md port.MediaDownloader, l port.Logger) func(context.Context, string, domain.DownloadRequest) (*domain.DownloadResult, error) {
				return message.NewDownloadDocumentUseCase(md, l).Execute
			}},
		{"Download Sticker", "/chat/downloadsticker", domain.MediaKindSticker,
			func(md port.MediaDownloader, l port.Logger) func(context.Context, string, domain.DownloadRequest) (*domain.DownloadResult, error) {
				return message.NewDownloadStickerUseCase(md, l).Execute
			}},
	}
}

// downloadFullRequest é um payload com os SETE campos preenchidos com valores
// distinguíveis entre si — é o que permite provar que nenhum deles é perdido
// ou trocado por outro no caminho até a porta.
func downloadFullRequest() domain.DownloadRequest {
	return domain.DownloadRequest{
		URL:           "https://mmg.whatsapp.net/d/f/AbCdEf.enc",
		DirectPath:    "/v/t62.7118-24/12345_678_90.enc",
		MediaKey:      []byte{0x01, 0x02, 0x03, 0x04},
		Mimetype:      "image/jpeg",
		FileEncSHA256: []byte{0xaa, 0xbb},
		FileSHA256:    []byte{0xcc, 0xdd},
		FileLength:    4242,
	}
}

// TestDownloadUseCases_AllSevenFieldsReachThePort é a auditoria nominal de
// campo: os SETE campos de domain.DownloadRequest — Url, DirectPath,
// MediaKey, Mimetype, FileEncSHA256, FileSHA256 e FileLength — têm de chegar
// ao descritor da porta, um a um, com o valor que entrou. Antes de CAP-09B
// SEIS deles eram aceitos e ignorados.
func TestDownloadUseCases_AllSevenFieldsReachThePort(t *testing.T) {
	for _, c := range downloadCapabilities() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{}
			exec := c.newUC(md, &contractsfake.Logger{})

			req := downloadFullRequest()
			if _, err := exec(context.Background(), txtID, req); err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}

			if n := len(md.DownloadCalls); n != 1 {
				t.Fatalf("Download chamado %d vez(es) por %s, quero 1", n, c.route)
			}
			got := md.DownloadCalls[0].Descriptor

			if got.Kind != c.wantKind {
				t.Errorf("Kind: got %q, want %q", got.Kind, c.wantKind)
			}
			if got.URL != req.URL {
				t.Errorf("URL: got %q, want %q", got.URL, req.URL)
			}
			if got.DirectPath != req.DirectPath {
				t.Errorf("DirectPath: got %q, want %q", got.DirectPath, req.DirectPath)
			}
			if string(got.MediaKey) != string(req.MediaKey) {
				t.Errorf("MediaKey: got %x, want %x", got.MediaKey, req.MediaKey)
			}
			if got.Mimetype != req.Mimetype {
				t.Errorf("Mimetype: got %q, want %q", got.Mimetype, req.Mimetype)
			}
			if string(got.FileEncSHA256) != string(req.FileEncSHA256) {
				t.Errorf("FileEncSHA256: got %x, want %x", got.FileEncSHA256, req.FileEncSHA256)
			}
			if string(got.FileSHA256) != string(req.FileSHA256) {
				t.Errorf("FileSHA256: got %x, want %x", got.FileSHA256, req.FileSHA256)
			}
			if got.FileLength != req.FileLength {
				t.Errorf("FileLength: got %d, want %d", got.FileLength, req.FileLength)
			}
		})
	}
}

// TestDownloadUseCases_DataURLCarriesBytesAndAgreesWithMimetype trava as duas
// metades do contrato de resposta: Data é a Data URL dos bytes REAIS que a
// porta devolveu (não um envelope vazio), e o campo Mimetype concorda com o
// prefixo de Data — os dois vêm do mesmo valor por construção, e este teste é
// o que impede que um deles passe a vir de outro lugar.
func TestDownloadUseCases_DataURLCarriesBytesAndAgreesWithMimetype(t *testing.T) {
	payload := []byte{0x00, 0x01, 0xff, 0xfe, 'h', 'i'}

	for _, c := range downloadCapabilities() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(context.Context, string, domain.MediaDescriptor) ([]byte, error) {
					return payload, nil
				},
			}
			exec := c.newUC(md, &contractsfake.Logger{})

			req := downloadFullRequest()
			req.Mimetype = "application/octet-stream"

			res, err := exec(context.Background(), txtID, req)
			if err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}

			if res.Mimetype != req.Mimetype {
				t.Errorf("Mimetype: got %q, want %q", res.Mimetype, req.Mimetype)
			}

			wantPrefix := "data:" + req.Mimetype + ";base64,"
			if !strings.HasPrefix(res.Data, wantPrefix) {
				t.Fatalf("Data: got %q, want prefixo %q", res.Data, wantPrefix)
			}
			// O prefixo declarado em Data e o campo Mimetype têm de nomear o
			// MESMO tipo — asserção explícita, não implícita no prefixo.
			gotMime := strings.TrimSuffix(strings.TrimPrefix(res.Data[:len(wantPrefix)], "data:"), ";base64,")
			if gotMime != res.Mimetype {
				t.Errorf("Data anuncia %q, campo Mimetype diz %q", gotMime, res.Mimetype)
			}

			decoded, err := base64.StdEncoding.DecodeString(res.Data[len(wantPrefix):])
			if err != nil {
				t.Fatalf("payload de Data nao e' base64 valido: %v", err)
			}
			if string(decoded) != string(payload) {
				t.Errorf("Data decodificado: got %x, want %x", decoded, payload)
			}
		})
	}
}

// TestDownloadUseCases_MissingURL_NoPortCall: campo obrigatório ausente é
// recusa de CLIENTE e precede qualquer conversa com a porta — nem
// EnsureSession nem Download podem acontecer. Eixo herdado da tabela de
// session_guard_test.go, realocado nome por nome.
func TestDownloadUseCases_MissingURL_NoPortCall(t *testing.T) {
	for _, c := range downloadCapabilities() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{}
			logger := &contractsfake.Logger{}
			exec := c.newUC(md, logger)

			_, err := exec(context.Background(), txtID, domain.DownloadRequest{Mimetype: "image/jpeg"})

			if err == nil {
				t.Fatal("request sem Url foi aceito")
			}
			if n := len(md.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas EnsureSession foi consultado %d vez(es)", n)
			}
			if n := len(md.DownloadCalls); n != 0 {
				t.Errorf("validacao falhou mas Download foi chamado %d vez(es)", n)
			}
			if n := logger.Len(); n != 0 {
				t.Errorf("erro de validacao gerou %d registro(s) de log: %v", n, logger.Messages())
			}
		})
	}
}

// TestDownloadUseCases_SessionFailure_NoDownload: a ORDEM entre EnsureSession
// e Download. Inverter as duas chamadas passaria em todos os outros testes
// deste arquivo — aqui não passa, porque com a sessão recusada o download não
// pode ter acontecido.
func TestDownloadUseCases_SessionFailure_NoDownload(t *testing.T) {
	for _, c := range downloadCapabilities() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{SessionGuard: contractsfake.FailSession(errSession)}
			logger := &contractsfake.Logger{}
			exec := c.newUC(md, logger)

			_, err := exec(context.Background(), txtID, downloadFullRequest())

			if !errors.Is(err, errSession) {
				t.Fatalf("erro da sessao nao chegou ao chamador: got %#v", err)
			}
			if n := len(md.DownloadCalls); n != 0 {
				t.Fatalf("sessao recusada mas Download foi chamado %d vez(es)", n)
			}
			assertSessionLog(t, logger, "txtID", txtID)
		})
	}
}

// TestDownloadUseCases_DownloaderFailure_NotSuccess: falha do downloader NÃO
// pode virar 200. O erro é propagado (com a causa preservada em Unwrap) e
// nenhum resultado sai.
func TestDownloadUseCases_DownloaderFailure_NotSuccess(t *testing.T) {
	errDownload := errors.New("download-refused-by-sdk")

	for _, c := range downloadCapabilities() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(context.Context, string, domain.MediaDescriptor) ([]byte, error) {
					return nil, errDownload
				},
			}
			exec := c.newUC(md, &contractsfake.Logger{})

			res, err := exec(context.Background(), txtID, downloadFullRequest())

			if err == nil {
				t.Fatal("falha do downloader virou sucesso")
			}
			if res != nil {
				t.Fatalf("resultado devolvido junto com erro: %+v", res)
			}
			if !errors.Is(err, errDownload) {
				t.Fatalf("causa perdida no caminho: got %#v", err)
			}
		})
	}
}

// TestDownloadUseCases_EmptyBytes_NotSuccess trava a decisão de CAP-09B sobre
// bytes vazios com erro nil (HOUSEKEEP.md F126): NÃO viram 200 com Data URL
// vazia — o contrato público promete conteúdo. Divergência consciente do
// fluxo histórico, que respondia 200 com "data:image/jpeg;base64,".
func TestDownloadUseCases_EmptyBytes_NotSuccess(t *testing.T) {
	for _, c := range downloadCapabilities() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(context.Context, string, domain.MediaDescriptor) ([]byte, error) {
					return []byte{}, nil
				},
			}
			exec := c.newUC(md, &contractsfake.Logger{})

			res, err := exec(context.Background(), txtID, downloadFullRequest())

			if err == nil {
				t.Fatalf("download vazio virou sucesso: %+v", res)
			}
			if res != nil {
				t.Fatalf("resultado devolvido junto com erro: %+v", res)
			}
		})
	}
}

// TestDownloadUseCases_SessionCheckedOnce: a sessão é consultada exatamente
// uma vez, com o txtID recebido — EnsureSession CONTINUA no CAP-09B, porque
// download é operação protocolar. Eixo herdado da tabela de
// session_guard_test.go.
func TestDownloadUseCases_SessionCheckedOnce(t *testing.T) {
	for _, c := range downloadCapabilities() {
		t.Run(c.capability, func(t *testing.T) {
			md := &contractsfake.MediaDownloader{}
			logger := &contractsfake.Logger{}
			exec := c.newUC(md, logger)

			if _, err := exec(context.Background(), txtID, downloadFullRequest()); err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}

			if n := len(md.EnsureSessionCalls); n != 1 {
				t.Fatalf("EnsureSession chamado %d vez(es), esperava 1", n)
			}
			if got := md.EnsureSessionCalls[0].TxtID; got != txtID {
				t.Errorf("EnsureSession recebeu txtID %q, esperava %q", got, txtID)
			}
			if got := md.DownloadCalls[0].TxtID; got != txtID {
				t.Errorf("Download recebeu txtID %q, esperava %q", got, txtID)
			}
			requireLog(t, logger, contractsfake.LevelInfo, "media downloaded")
		})
	}
}
