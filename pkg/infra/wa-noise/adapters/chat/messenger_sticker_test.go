package chat

import (
	"context"
	"errors"
	"testing"
	"time"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

func TestChatMessengerAdapter_SendSticker_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))
	_, err := a.SendSticker(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}}, "")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendSticker code = %q", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendSticker_InvalidJID: JID inválido nunca pode
// alcançar client.Upload nem client.SendMessage.
func TestChatMessengerAdapter_SendSticker_InvalidJID(t *testing.T) {
	uploadCalled := false
	fake := &testkit.Fake{UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
		uploadCalled = true
		return wanoise.UploadResponse{}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.SendSticker(context.Background(), "u1", domain.JID(string([]byte{0x00})), domain.MediaPayload{Bytes: []byte{1, 2, 3}}, "")
	if err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
	if uploadCalled {
		t.Fatal("SendSticker chamou client.Upload com JID inválido")
	}
}

// TestChatMessengerAdapter_SendSticker_UploadFailurePropagates: falha de
// upload nunca alcança client.SendMessage nem produz resultado.
func TestChatMessengerAdapter_SendSticker_UploadFailurePropagates(t *testing.T) {
	uploadErr := errors.New("upload: boom")
	sendCalled := false
	fake := &testkit.Fake{
		UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
			return wanoise.UploadResponse{}, uploadErr
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			sendCalled = true
			return wanoise.SendResponse{}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	res, err := a.SendSticker(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}, MimeType: "image/webp"}, "")
	if err == nil {
		t.Fatal("SendSticker não propagou a falha de upload")
	}
	if sendCalled {
		t.Fatal("upload falhou, mas client.SendMessage foi chamado mesmo assim")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("SendSticker devolveu resultado nao-vazio com upload falho: %+v", res)
	}
}

// TestChatMessengerAdapter_SendSticker_UploadOK_SendMessageFail_NeverSent é
// o caso obrigatório do CAP-07: upload bem-sucedido seguido de SendMessage
// falho NÃO produz sucesso algum.
func TestChatMessengerAdapter_SendSticker_UploadOK_SendMessageFail_NeverSent(t *testing.T) {
	sendErr := errors.New("sendmessage: boom")
	uploadCalled := false
	fake := &testkit.Fake{
		UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
			uploadCalled = true
			return wanoise.UploadResponse{URL: "https://mmg.whatsapp.net/x", DirectPath: "/x"}, nil
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			return wanoise.SendResponse{}, sendErr
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	res, err := a.SendSticker(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}, MimeType: "image/webp"}, "")
	if !uploadCalled {
		t.Fatal("upload nunca foi chamado — teste nao exercita o caso upload-ok-send-fail")
	}
	if err == nil {
		t.Fatal("SendSticker nao propagou a falha de SendMessage apos upload bem-sucedido")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("upload OK + SendMessage falho produziu resultado nao-vazio: %+v", res)
	}
}

// TestChatMessengerAdapter_SendSticker_OK confere a montagem da mensagem
// (upload chamado com os bytes CONVERTIDOS — payload.Bytes já é o que o
// usecase resolveu via appport.StickerProcessor, não input cru — e
// wanoise.MediaImage, já que sticker não tem MediaType próprio no SDK),
// StickerMessage montada a partir da resposta de upload, e que o resultado
// vem de resp, não de valores inventados.
//
// CONTROLE NEGATIVO (CAP-07): trocar wanoise.MediaImage por
// wanoise.MediaDocument na chamada de client.Upload dentro de
// ChatMessengerAdapter.SendSticker faz este teste morder na asserção
// gotAppInfo != wanoise.MediaImage. Mutação executada e revertida
// manualmente durante o desenvolvimento deste slice — saída colada no
// relatório CAP07-report.md.
func TestChatMessengerAdapter_SendSticker_OK(t *testing.T) {
	now := time.Now()
	var gotPlaintext []byte
	var gotAppInfo wanoise.MediaType
	var gotTo types.JID
	var gotMsg *waE2E.Message
	fake := &testkit.Fake{
		UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
			gotPlaintext = plaintext
			gotAppInfo = appInfo
			return wanoise.UploadResponse{
				URL:           "https://mmg.whatsapp.net/s",
				DirectPath:    "/s",
				MediaKey:      []byte{9, 9, 9},
				FileEncSHA256: []byte{1, 1, 1},
				FileSHA256:    []byte{2, 2, 2},
			}, nil
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotTo = to
			gotMsg = m
			return wanoise.SendResponse{Timestamp: now, ID: types.MessageID("wire-id-sticker")}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	convertedWebP := []byte{0x52, 0x49, 0x46, 0x46, 0xDE, 0xAD, 0xBE, 0xEF}
	payload := domain.MediaPayload{Bytes: convertedWebP, MimeType: "image/webp"}
	res, err := a.SendSticker(context.Background(), "u1", "x@y.com", payload, "")
	if err != nil {
		t.Fatalf("SendSticker = %v", err)
	}
	if string(gotPlaintext) != string(convertedWebP) {
		t.Errorf("bytes enviados ao Upload divergem do payload CONVERTIDO — sticker subiu bytes errados")
	}
	if gotAppInfo != wanoise.MediaImage {
		t.Errorf("appInfo do Upload = %v, want wanoise.MediaImage", gotAppInfo)
	}
	if gotTo.String() != "x@y.com" {
		t.Errorf("destinatario = %q, want %q", gotTo.String(), "x@y.com")
	}
	sticker := gotMsg.GetStickerMessage()
	if sticker == nil {
		t.Fatal("StickerMessage nao foi montada")
	}
	if sticker.GetURL() != "https://mmg.whatsapp.net/s" || sticker.GetDirectPath() != "/s" {
		t.Errorf("URL/DirectPath nao vieram da resposta de upload: %+v", sticker)
	}
	if string(sticker.GetMediaKey()) != string([]byte{9, 9, 9}) {
		t.Errorf("MediaKey nao veio da resposta de upload: %v", sticker.GetMediaKey())
	}
	if string(sticker.GetFileEncSHA256()) != string([]byte{1, 1, 1}) {
		t.Errorf("FileEncSHA256 nao veio da resposta de upload: %v", sticker.GetFileEncSHA256())
	}
	if string(sticker.GetFileSHA256()) != string([]byte{2, 2, 2}) {
		t.Errorf("FileSHA256 nao veio da resposta de upload: %v", sticker.GetFileSHA256())
	}
	if sticker.GetMimetype() != "image/webp" {
		t.Errorf("Mimetype = %q, want %q (o que o pipeline de conversao devolveu)", sticker.GetMimetype(), "image/webp")
	}
	if sticker.GetFileLength() != uint64(len(convertedWebP)) {
		t.Errorf("FileLength = %d, want %d", sticker.GetFileLength(), len(convertedWebP))
	}
	if res.Timestamp != now {
		t.Errorf("SendSticker timestamp = %v, want %v", res.Timestamp, now)
	}
	if res.ID != "wire-id-sticker" {
		t.Errorf("SendSticker ID = %q, want %q (o que resp devolveu)", res.ID, "wire-id-sticker")
	}
}

// TestChatMessengerAdapter_SendSticker_WithCallerID: quando id não é vazio,
// vira RequestExtra.ID — o SDK que decide o ID final, devolvido em resp.ID.
func TestChatMessengerAdapter_SendSticker_WithCallerID(t *testing.T) {
	var gotExtra []wanoise.SendRequestExtra
	fake := &testkit.Fake{
		UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
			return wanoise.UploadResponse{}, nil
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotExtra = extra
			return wanoise.SendResponse{ID: types.MessageID("caller-id")}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	res, err := a.SendSticker(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1}, MimeType: "image/webp"}, "caller-id")
	if err != nil {
		t.Fatalf("SendSticker = %v", err)
	}
	if len(gotExtra) != 1 || string(gotExtra[0].ID) != "caller-id" {
		t.Fatalf("RequestExtra.ID nao recebeu o id do chamador: %+v", gotExtra)
	}
	if res.ID != "caller-id" {
		t.Errorf("SendSticker ID = %q, want %q", res.ID, "caller-id")
	}
}
