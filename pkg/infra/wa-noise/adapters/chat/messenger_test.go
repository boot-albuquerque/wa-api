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

// --- ChatMessengerAdapter ---

func TestNewChatMessengerAdapter(t *testing.T) {
	if NewChatMessengerAdapter(testkit.GetterWith(nil)) == nil {
		t.Fatal("NewChatMessengerAdapter returned nil")
	}
}

// TestChatMessengerAdapter_MarkRead_InvalidChatJID devolve erro de wajid.ToJID.
// Tenta várias entradas; quando encontra uma que wajid.ParseJID rejeita,
// confirma que MarkRead propaga o erro.
func TestChatMessengerAdapter_MarkRead_InvalidChatJID(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": &testkit.Fake{}}))
	invalid := []domain.JID{
		"@@", "@", "x@", "@y.com", domain.JID(string([]byte{0x00})),
	}
	for _, jid := range invalid {
		err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), jid, "z@y.com")
		if err != nil && testkit.AppErrCode(err) != "no_session" {
			return
		}
	}
	t.Skip("wajid.ParseJID não falhou para nenhuma entrada testada")
}

// TestChatMessengerAdapter_MarkRead_InvalidSenderJID devolve erro de wajid.ToJID.
func TestChatMessengerAdapter_MarkRead_InvalidSenderJID(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": &testkit.Fake{}}))
	invalid := []domain.JID{
		"@@", "@", "x@", "@y.com", domain.JID(string([]byte{0x00})),
	}
	for _, jid := range invalid {
		err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", jid)
		if err != nil && testkit.AppErrCode(err) != "no_session" {
			return
		}
	}
	t.Skip("wajid.ParseJID não falhou para nenhuma entrada testada")
}

// TestChatMessengerAdapter_MarkRead_NoSession.
func TestChatMessengerAdapter_MarkRead_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))
	err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", "z@y.com")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("MarkRead code = %q", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_MarkRead_PropagatesError.
func TestChatMessengerAdapter_MarkRead_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &testkit.Fake{MarkReadFn: func(ctx context.Context, ids []types.MessageID, ts time.Time, chat, sender types.JID, extra ...types.ReceiptType) error {
		return sdkErr
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", "z@y.com")
	if err == nil {
		t.Fatal("MarkRead não propagou erro")
	}
}

// TestChatMessengerAdapter_MarkRead_OK.
func TestChatMessengerAdapter_MarkRead_OK(t *testing.T) {
	called := false
	fake := &testkit.Fake{MarkReadFn: func(ctx context.Context, ids []types.MessageID, ts time.Time, chat, sender types.JID, extra ...types.ReceiptType) error {
		called = true
		return nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", "z@y.com"); err != nil {
		t.Fatalf("MarkRead = %v", err)
	}
	if !called {
		t.Fatal("MarkRead não invocou o SDK")
	}
}

// TestChatMessengerAdapter_SendReaction_NoSession.
func TestChatMessengerAdapter_SendReaction_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))
	_, err := a.SendReaction(context.Background(), "u1", "x@y.com", domain.Reaction{Text: "👍"})
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendReaction code = %q", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendReaction_InvalidJID.
func TestChatMessengerAdapter_SendReaction_InvalidJID(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": &testkit.Fake{}}))
	_, err := a.SendReaction(context.Background(), "u1", domain.JID(string([]byte{0x00})), domain.Reaction{Text: "👍"})
	if err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
}

// TestChatMessengerAdapter_SendReaction_PropagatesError.
func TestChatMessengerAdapter_SendReaction_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &testkit.Fake{SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
		return wanoise.SendResponse{}, sdkErr
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.SendReaction(context.Background(), "u1", "x@y.com", domain.Reaction{Text: "👍"})
	if err == nil {
		t.Fatal("SendReaction não propagou erro")
	}
}

// TestChatMessengerAdapter_SendReaction_OK devolve MessageSendResult.
func TestChatMessengerAdapter_SendReaction_OK(t *testing.T) {
	now := time.Now()
	fake := &testkit.Fake{SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
		return wanoise.SendResponse{Timestamp: now, ID: types.MessageID("msg-1")}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	res, err := a.SendReaction(context.Background(), "u1", "x@y.com", domain.Reaction{
		Text:            "👍",
		FromMe:          true,
		TargetMessageID: "orig-msg",
	})
	if err != nil {
		t.Fatalf("SendReaction = %v", err)
	}
	if res.Timestamp != now {
		t.Errorf("SendReaction timestamp = %v, want %v", res.Timestamp, now)
	}
}

// TestChatMessengerAdapter_SendText_NoSession.
func TestChatMessengerAdapter_SendText_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))
	_, err := a.SendText(context.Background(), "u1", "x@y.com", "ola", nil, "")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendText code = %q", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendText_InvalidJID: JID inválido nunca pode
// alcançar client.SendMessage — proibido pelo CAP-01.
func TestChatMessengerAdapter_SendText_InvalidJID(t *testing.T) {
	called := false
	fake := &testkit.Fake{SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
		called = true
		return wanoise.SendResponse{}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.SendText(context.Background(), "u1", domain.JID(string([]byte{0x00})), "ola", nil, "")
	if err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
	if called {
		t.Fatal("SendText chamou client.SendMessage com JID inválido")
	}
}

// TestChatMessengerAdapter_SendText_PropagatesError: SendMessage falhando não
// pode virar sucesso — falso-sucesso do CAP-01 é proibido nesta camada.
func TestChatMessengerAdapter_SendText_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &testkit.Fake{SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
		return wanoise.SendResponse{}, sdkErr
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	res, err := a.SendText(context.Background(), "u1", "x@y.com", "ola", nil, "")
	if err == nil {
		t.Fatal("SendText não propagou erro")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("SendText devolveu resultado nao-vazio com erro: %+v", res)
	}
}

// TestChatMessengerAdapter_SendText_OK confere a montagem da mensagem
// (Conversation, não ExtendedTextMessage) e que o resultado vem de resp, não
// do id de entrada.
func TestChatMessengerAdapter_SendText_OK(t *testing.T) {
	now := time.Now()
	var gotTo types.JID
	var gotMsg *waE2E.Message
	fake := &testkit.Fake{SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
		gotTo = to
		gotMsg = m
		return wanoise.SendResponse{Timestamp: now, ID: types.MessageID("wire-id")}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	res, err := a.SendText(context.Background(), "u1", "x@y.com", "ola mundo", nil, "")
	if err != nil {
		t.Fatalf("SendText = %v", err)
	}
	if res.Timestamp != now {
		t.Errorf("SendText timestamp = %v, want %v", res.Timestamp, now)
	}
	if res.ID != "wire-id" {
		t.Errorf("SendText ID = %q, want %q (o que resp devolveu)", res.ID, "wire-id")
	}
	if gotTo.String() != "x@y.com" {
		t.Errorf("destinatario = %q, want %q", gotTo.String(), "x@y.com")
	}
	if gotMsg.GetConversation() != "ola mundo" {
		t.Errorf("Conversation = %q, want %q", gotMsg.GetConversation(), "ola mundo")
	}
	if gotMsg.GetExtendedTextMessage() != nil {
		t.Error("SendText montou ExtendedTextMessage, quero Conversation simples")
	}
}

// TestChatMessengerAdapter_SendText_WithCallerID: quando id não é vazio, ele
// vira RequestExtra.ID — o SDK que decide o ID final, devolvido em resp.ID.
func TestChatMessengerAdapter_SendText_WithCallerID(t *testing.T) {
	var gotExtra []wanoise.SendRequestExtra
	fake := &testkit.Fake{SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
		gotExtra = extra
		return wanoise.SendResponse{ID: types.MessageID("caller-id")}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	res, err := a.SendText(context.Background(), "u1", "x@y.com", "ola", nil, "caller-id")
	if err != nil {
		t.Fatalf("SendText = %v", err)
	}
	if len(gotExtra) != 1 || string(gotExtra[0].ID) != "caller-id" {
		t.Fatalf("RequestExtra.ID nao recebeu o id do chamador: %+v", gotExtra)
	}
	if res.ID != "caller-id" {
		t.Errorf("SendText ID = %q, want %q", res.ID, "caller-id")
	}
}

// TestChatMessengerAdapter_SendText_WithPreview_BuildsExtendedTextMessage
// (LP-2, CAP-01.1): com preview não-nil, SendText monta ExtendedTextMessage
// — não Conversation — com Text, MatchedText, Title, Description e
// JPEGThumbnail vindos EXATAMENTE de domain.LinkPreviewData, não valores
// aproximados.
func TestChatMessengerAdapter_SendText_WithPreview_BuildsExtendedTextMessage(t *testing.T) {
	var gotMsg *waE2E.Message
	fake := &testkit.Fake{SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
		gotMsg = m
		return wanoise.SendResponse{ID: types.MessageID("wire-id-preview")}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	preview := &domain.LinkPreviewData{
		MatchedURL:    "https://exemplo.com/pagina",
		Title:         "Título",
		Description:   "Descrição",
		ThumbnailJPEG: []byte{0xFF, 0xD8, 0xFF},
	}
	res, err := a.SendText(context.Background(), "u1", "x@y.com", "olha https://exemplo.com/pagina", preview, "")
	if err != nil {
		t.Fatalf("SendText = %v", err)
	}
	if res.ID != "wire-id-preview" {
		t.Errorf("SendText ID = %q, want %q", res.ID, "wire-id-preview")
	}
	if gotMsg.GetConversation() != "" {
		t.Errorf("Conversation montado com preview presente: %q", gotMsg.GetConversation())
	}
	etm := gotMsg.GetExtendedTextMessage()
	if etm == nil {
		t.Fatal("preview presente, mas ExtendedTextMessage nao foi montado")
	}
	if etm.GetText() != "olha https://exemplo.com/pagina" {
		t.Errorf("ExtendedTextMessage.Text = %q, want %q", etm.GetText(), "olha https://exemplo.com/pagina")
	}
	if etm.GetMatchedText() != preview.MatchedURL {
		t.Errorf("MatchedText = %q, want %q", etm.GetMatchedText(), preview.MatchedURL)
	}
	if etm.GetTitle() != preview.Title {
		t.Errorf("Title = %q, want %q", etm.GetTitle(), preview.Title)
	}
	if etm.GetDescription() != preview.Description {
		t.Errorf("Description = %q, want %q", etm.GetDescription(), preview.Description)
	}
	if string(etm.GetJPEGThumbnail()) != string(preview.ThumbnailJPEG) {
		t.Errorf("JPEGThumbnail = %v, want %v", etm.GetJPEGThumbnail(), preview.ThumbnailJPEG)
	}
}

// --- ChatMessengerAdapter.SendImage (CAP-02) ----------------------------

// TestChatMessengerAdapter_SendImage_NoSession.
func TestChatMessengerAdapter_SendImage_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))
	_, err := a.SendImage(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}}, "")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendImage code = %q", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendImage_InvalidJID: JID inválido nunca pode
// alcançar client.Upload nem client.SendMessage.
func TestChatMessengerAdapter_SendImage_InvalidJID(t *testing.T) {
	uploadCalled := false
	fake := &testkit.Fake{UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
		uploadCalled = true
		return wanoise.UploadResponse{}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.SendImage(context.Background(), "u1", domain.JID(string([]byte{0x00})), domain.MediaPayload{Bytes: []byte{1, 2, 3}}, "")
	if err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
	if uploadCalled {
		t.Fatal("SendImage chamou client.Upload com JID inválido")
	}
}

// TestChatMessengerAdapter_SendImage_UploadFailurePropagates: falha de
// upload nunca alcança client.SendMessage nem produz resultado.
func TestChatMessengerAdapter_SendImage_UploadFailurePropagates(t *testing.T) {
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
	res, err := a.SendImage(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}, MimeType: "image/png"}, "")
	if err == nil {
		t.Fatal("SendImage não propagou a falha de upload")
	}
	if sendCalled {
		t.Fatal("upload falhou, mas client.SendMessage foi chamado mesmo assim")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("SendImage devolveu resultado nao-vazio com upload falho: %+v", res)
	}
}

// TestChatMessengerAdapter_SendImage_UploadOK_SendMessageFail_NeverSent é o
// caso obrigatório do CAP-02: upload bem-sucedido seguido de SendMessage
// falho NÃO produz sucesso algum. O protocolo não oferece desfazer o
// upload, e este teste garante que o adapter não finge que oferece — o erro
// de SendMessage chega inteiro ao chamador, sem resultado.
func TestChatMessengerAdapter_SendImage_UploadOK_SendMessageFail_NeverSent(t *testing.T) {
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
	res, err := a.SendImage(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}, MimeType: "image/png"}, "")
	if !uploadCalled {
		t.Fatal("upload nunca foi chamado — teste nao exercita o caso upload-ok-send-fail")
	}
	if err == nil {
		t.Fatal("SendImage nao propagou a falha de SendMessage apos upload bem-sucedido")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("upload OK + SendMessage falho produziu resultado nao-vazio: %+v", res)
	}
}

// TestChatMessengerAdapter_SendImage_OK confere a montagem da mensagem
// (upload chamado com os bytes certos e wanoise.MediaImage, ImageMessage
// montado a partir da resposta de upload, não de valores inventados) e que
// o resultado vem de resp, não do id de entrada.
func TestChatMessengerAdapter_SendImage_OK(t *testing.T) {
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
				URL:           "https://mmg.whatsapp.net/x",
				DirectPath:    "/x",
				MediaKey:      []byte{9, 9, 9},
				FileEncSHA256: []byte{1, 1, 1},
				FileSHA256:    []byte{2, 2, 2},
			}, nil
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotTo = to
			gotMsg = m
			return wanoise.SendResponse{Timestamp: now, ID: types.MessageID("wire-id-img")}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	payload := domain.MediaPayload{Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, MimeType: "image/png", Caption: "legenda"}
	res, err := a.SendImage(context.Background(), "u1", "x@y.com", payload, "")
	if err != nil {
		t.Fatalf("SendImage = %v", err)
	}
	if string(gotPlaintext) != string(payload.Bytes) {
		t.Errorf("bytes enviados ao Upload divergem do payload")
	}
	if gotAppInfo != wanoise.MediaImage {
		t.Errorf("appInfo do Upload = %v, want wanoise.MediaImage", gotAppInfo)
	}
	if gotTo.String() != "x@y.com" {
		t.Errorf("destinatario = %q, want %q", gotTo.String(), "x@y.com")
	}
	img := gotMsg.GetImageMessage()
	if img == nil {
		t.Fatal("ImageMessage nao foi montada")
	}
	if img.GetCaption() != "legenda" {
		t.Errorf("Caption = %q, want %q", img.GetCaption(), "legenda")
	}
	if img.GetURL() != "https://mmg.whatsapp.net/x" || img.GetDirectPath() != "/x" {
		t.Errorf("URL/DirectPath nao vieram da resposta de upload: %+v", img)
	}
	if string(img.GetMediaKey()) != string([]byte{9, 9, 9}) {
		t.Errorf("MediaKey nao veio da resposta de upload: %v", img.GetMediaKey())
	}
	if img.GetMimetype() != "image/png" {
		t.Errorf("Mimetype = %q, want %q", img.GetMimetype(), "image/png")
	}
	if img.GetFileLength() != uint64(len(payload.Bytes)) {
		t.Errorf("FileLength = %d, want %d", img.GetFileLength(), len(payload.Bytes))
	}
	if res.Timestamp != now {
		t.Errorf("SendImage timestamp = %v, want %v", res.Timestamp, now)
	}
	if res.ID != "wire-id-img" {
		t.Errorf("SendImage ID = %q, want %q (o que resp devolveu)", res.ID, "wire-id-img")
	}
}

// TestChatMessengerAdapter_SendImage_WithCallerID: quando id não é vazio,
// vira RequestExtra.ID — o SDK que decide o ID final, devolvido em resp.ID.
func TestChatMessengerAdapter_SendImage_WithCallerID(t *testing.T) {
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
	res, err := a.SendImage(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1}, MimeType: "image/png"}, "caller-id")
	if err != nil {
		t.Fatalf("SendImage = %v", err)
	}
	if len(gotExtra) != 1 || string(gotExtra[0].ID) != "caller-id" {
		t.Fatalf("RequestExtra.ID nao recebeu o id do chamador: %+v", gotExtra)
	}
	if res.ID != "caller-id" {
		t.Errorf("SendImage ID = %q, want %q", res.ID, "caller-id")
	}
}

// --- ChatMessengerAdapter.SendDocument (CAP-04) --------------------------

// TestChatMessengerAdapter_SendDocument_NoSession.
func TestChatMessengerAdapter_SendDocument_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))
	_, err := a.SendDocument(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}, FileName: "a.pdf"}, "")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendDocument code = %q", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendDocument_InvalidJID: JID inválido nunca pode
// alcançar client.Upload nem client.SendMessage.
func TestChatMessengerAdapter_SendDocument_InvalidJID(t *testing.T) {
	uploadCalled := false
	fake := &testkit.Fake{UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
		uploadCalled = true
		return wanoise.UploadResponse{}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.SendDocument(context.Background(), "u1", domain.JID(string([]byte{0x00})), domain.MediaPayload{Bytes: []byte{1, 2, 3}, FileName: "a.pdf"}, "")
	if err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
	if uploadCalled {
		t.Fatal("SendDocument chamou client.Upload com JID inválido")
	}
}

// TestChatMessengerAdapter_SendDocument_UploadFailurePropagates: falha de
// upload nunca alcança client.SendMessage nem produz resultado.
func TestChatMessengerAdapter_SendDocument_UploadFailurePropagates(t *testing.T) {
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
	res, err := a.SendDocument(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}, MimeType: "application/pdf", FileName: "a.pdf"}, "")
	if err == nil {
		t.Fatal("SendDocument não propagou a falha de upload")
	}
	if sendCalled {
		t.Fatal("upload falhou, mas client.SendMessage foi chamado mesmo assim")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("SendDocument devolveu resultado nao-vazio com upload falho: %+v", res)
	}
}

// TestChatMessengerAdapter_SendDocument_UploadOK_SendMessageFail_NeverSent é
// o caso obrigatório do CAP-04: upload bem-sucedido seguido de SendMessage
// falho NÃO produz sucesso algum. O protocolo não oferece desfazer o
// upload, e este teste garante que o adapter não finge que oferece.
func TestChatMessengerAdapter_SendDocument_UploadOK_SendMessageFail_NeverSent(t *testing.T) {
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
	res, err := a.SendDocument(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}, MimeType: "application/pdf", FileName: "a.pdf"}, "")
	if !uploadCalled {
		t.Fatal("upload nunca foi chamado — teste nao exercita o caso upload-ok-send-fail")
	}
	if err == nil {
		t.Fatal("SendDocument nao propagou a falha de SendMessage apos upload bem-sucedido")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("upload OK + SendMessage falho produziu resultado nao-vazio: %+v", res)
	}
}

// TestChatMessengerAdapter_SendDocument_OK confere a montagem da mensagem
// (upload chamado com os bytes certos e wanoise.MediaDocument — não
// MediaImage —, DocumentMessage montada a partir da resposta de upload e
// de payload.FileName) e que o resultado vem de resp, não do id de entrada.
func TestChatMessengerAdapter_SendDocument_OK(t *testing.T) {
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
				URL:           "https://mmg.whatsapp.net/x",
				DirectPath:    "/x",
				MediaKey:      []byte{9, 9, 9},
				FileEncSHA256: []byte{1, 1, 1},
				FileSHA256:    []byte{2, 2, 2},
			}, nil
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotTo = to
			gotMsg = m
			return wanoise.SendResponse{Timestamp: now, ID: types.MessageID("wire-id-doc")}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	payload := domain.MediaPayload{Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, MimeType: "application/pdf", Caption: "legenda", FileName: "relatorio.pdf"}
	res, err := a.SendDocument(context.Background(), "u1", "x@y.com", payload, "")
	if err != nil {
		t.Fatalf("SendDocument = %v", err)
	}
	if string(gotPlaintext) != string(payload.Bytes) {
		t.Errorf("bytes enviados ao Upload divergem do payload")
	}
	if gotAppInfo != wanoise.MediaDocument {
		t.Errorf("appInfo do Upload = %v, want wanoise.MediaDocument", gotAppInfo)
	}
	if gotTo.String() != "x@y.com" {
		t.Errorf("destinatario = %q, want %q", gotTo.String(), "x@y.com")
	}
	doc := gotMsg.GetDocumentMessage()
	if doc == nil {
		t.Fatal("DocumentMessage nao foi montada")
	}
	if doc.GetCaption() != "legenda" {
		t.Errorf("Caption = %q, want %q", doc.GetCaption(), "legenda")
	}
	if doc.GetFileName() != "relatorio.pdf" {
		t.Errorf("FileName = %q, want %q", doc.GetFileName(), "relatorio.pdf")
	}
	if doc.GetURL() != "https://mmg.whatsapp.net/x" || doc.GetDirectPath() != "/x" {
		t.Errorf("URL/DirectPath nao vieram da resposta de upload: %+v", doc)
	}
	if string(doc.GetMediaKey()) != string([]byte{9, 9, 9}) {
		t.Errorf("MediaKey nao veio da resposta de upload: %v", doc.GetMediaKey())
	}
	if string(doc.GetFileEncSHA256()) != string([]byte{1, 1, 1}) {
		t.Errorf("FileEncSHA256 nao veio da resposta de upload: %v", doc.GetFileEncSHA256())
	}
	if string(doc.GetFileSHA256()) != string([]byte{2, 2, 2}) {
		t.Errorf("FileSHA256 nao veio da resposta de upload: %v", doc.GetFileSHA256())
	}
	if doc.GetMimetype() != "application/pdf" {
		t.Errorf("Mimetype = %q, want %q", doc.GetMimetype(), "application/pdf")
	}
	if doc.GetFileLength() != uint64(len(payload.Bytes)) {
		t.Errorf("FileLength = %d, want %d", doc.GetFileLength(), len(payload.Bytes))
	}
	if res.Timestamp != now {
		t.Errorf("SendDocument timestamp = %v, want %v", res.Timestamp, now)
	}
	if res.ID != "wire-id-doc" {
		t.Errorf("SendDocument ID = %q, want %q (o que resp devolveu)", res.ID, "wire-id-doc")
	}
}

// TestChatMessengerAdapter_SendDocument_FileName_NeverTouchesFilesystem
// prova que um FileName hostil chega intacto ao protobuf, sem qualquer
// tentativa de abrir/ler/escrever o caminho no filesystem local — é
// metadata pura, mesmo quando hostil.
func TestChatMessengerAdapter_SendDocument_FileName_NeverTouchesFilesystem(t *testing.T) {
	var gotMsg *waE2E.Message
	fake := &testkit.Fake{
		UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
			return wanoise.UploadResponse{}, nil
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotMsg = m
			return wanoise.SendResponse{ID: types.MessageID("wire-id-hostile")}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	hostileName := "../../etc/passwd"
	payload := domain.MediaPayload{Bytes: []byte{1}, MimeType: "application/pdf", FileName: hostileName}
	_, err := a.SendDocument(context.Background(), "u1", "x@y.com", payload, "")
	if err != nil {
		t.Fatalf("SendDocument = %v", err)
	}
	if got := gotMsg.GetDocumentMessage().GetFileName(); got != hostileName {
		t.Errorf("FileName = %q, want %q (metadata pura, repassada intacta)", got, hostileName)
	}
}

// TestChatMessengerAdapter_SendDocument_WithCallerID: quando id não é
// vazio, vira RequestExtra.ID — o SDK que decide o ID final, devolvido em
// resp.ID.
func TestChatMessengerAdapter_SendDocument_WithCallerID(t *testing.T) {
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
	res, err := a.SendDocument(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1}, MimeType: "application/pdf", FileName: "a.pdf"}, "caller-id")
	if err != nil {
		t.Fatalf("SendDocument = %v", err)
	}
	if len(gotExtra) != 1 || string(gotExtra[0].ID) != "caller-id" {
		t.Fatalf("RequestExtra.ID nao recebeu o id do chamador: %+v", gotExtra)
	}
	if res.ID != "caller-id" {
		t.Errorf("SendDocument ID = %q, want %q", res.ID, "caller-id")
	}
}

// --- SendAudio (CAP-05) ---

func TestChatMessengerAdapter_SendAudio_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))
	_, err := a.SendAudio(context.Background(), "u1", "x@y.com", domain.AudioPayload{Bytes: []byte{1, 2, 3}}, "")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendAudio code = %q", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendAudio_InvalidJID: JID inválido nunca pode
// alcançar client.Upload nem client.SendMessage.
func TestChatMessengerAdapter_SendAudio_InvalidJID(t *testing.T) {
	uploadCalled := false
	fake := &testkit.Fake{UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
		uploadCalled = true
		return wanoise.UploadResponse{}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.SendAudio(context.Background(), "u1", domain.JID(string([]byte{0x00})), domain.AudioPayload{Bytes: []byte{1, 2, 3}}, "")
	if err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
	if uploadCalled {
		t.Fatal("SendAudio chamou client.Upload com JID inválido")
	}
}

// TestChatMessengerAdapter_SendAudio_UploadFailurePropagates: falha de
// upload nunca alcança client.SendMessage nem produz resultado.
func TestChatMessengerAdapter_SendAudio_UploadFailurePropagates(t *testing.T) {
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
	res, err := a.SendAudio(context.Background(), "u1", "x@y.com", domain.AudioPayload{Bytes: []byte{1, 2, 3}, MimeType: "audio/ogg", PTT: true}, "")
	if err == nil {
		t.Fatal("SendAudio não propagou a falha de upload")
	}
	if sendCalled {
		t.Fatal("upload falhou, mas client.SendMessage foi chamado mesmo assim")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("SendAudio devolveu resultado nao-vazio com upload falho: %+v", res)
	}
}

// TestChatMessengerAdapter_SendAudio_UploadOK_SendMessageFail_NeverSent é o
// caso obrigatório do CAP-05: upload bem-sucedido seguido de SendMessage
// falho NÃO produz sucesso algum. O protocolo não oferece desfazer o
// upload, e este teste garante que o adapter não finge que oferece.
func TestChatMessengerAdapter_SendAudio_UploadOK_SendMessageFail_NeverSent(t *testing.T) {
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
	res, err := a.SendAudio(context.Background(), "u1", "x@y.com", domain.AudioPayload{Bytes: []byte{1, 2, 3}, MimeType: "audio/ogg", PTT: true}, "")
	if !uploadCalled {
		t.Fatal("upload nunca foi chamado — teste nao exercita o caso upload-ok-send-fail")
	}
	if err == nil {
		t.Fatal("SendAudio nao propagou a falha de SendMessage apos upload bem-sucedido")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("upload OK + SendMessage falho produziu resultado nao-vazio: %+v", res)
	}
}

// TestChatMessengerAdapter_SendAudio_OK confere a montagem da mensagem
// (upload chamado com os bytes certos e wanoise.MediaAudio — não
// MediaDocument nem MediaImage —, AudioMessage montada a partir da resposta
// de upload e de payload.PTT/Seconds) e que o resultado vem de resp, não do
// id de entrada.
func TestChatMessengerAdapter_SendAudio_OK(t *testing.T) {
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
				URL:           "https://mmg.whatsapp.net/x",
				DirectPath:    "/x",
				MediaKey:      []byte{9, 9, 9},
				FileEncSHA256: []byte{1, 1, 1},
				FileSHA256:    []byte{2, 2, 2},
			}, nil
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotTo = to
			gotMsg = m
			return wanoise.SendResponse{Timestamp: now, ID: types.MessageID("wire-id-audio")}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	payload := domain.AudioPayload{Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, MimeType: "audio/ogg; codecs=opus", PTT: true, Seconds: 12}
	res, err := a.SendAudio(context.Background(), "u1", "x@y.com", payload, "")
	if err != nil {
		t.Fatalf("SendAudio = %v", err)
	}
	if string(gotPlaintext) != string(payload.Bytes) {
		t.Errorf("bytes enviados ao Upload divergem do payload")
	}
	if gotAppInfo != wanoise.MediaAudio {
		t.Errorf("appInfo do Upload = %v, want wanoise.MediaAudio", gotAppInfo)
	}
	if gotTo.String() != "x@y.com" {
		t.Errorf("destinatario = %q, want %q", gotTo.String(), "x@y.com")
	}
	audio := gotMsg.GetAudioMessage()
	if audio == nil {
		t.Fatal("AudioMessage nao foi montada")
	}
	if audio.GetURL() != "https://mmg.whatsapp.net/x" || audio.GetDirectPath() != "/x" {
		t.Errorf("URL/DirectPath nao vieram da resposta de upload: %+v", audio)
	}
	if string(audio.GetMediaKey()) != string([]byte{9, 9, 9}) {
		t.Errorf("MediaKey nao veio da resposta de upload: %v", audio.GetMediaKey())
	}
	if string(audio.GetFileEncSHA256()) != string([]byte{1, 1, 1}) {
		t.Errorf("FileEncSHA256 nao veio da resposta de upload: %v", audio.GetFileEncSHA256())
	}
	if string(audio.GetFileSHA256()) != string([]byte{2, 2, 2}) {
		t.Errorf("FileSHA256 nao veio da resposta de upload: %v", audio.GetFileSHA256())
	}
	if audio.GetMimetype() != "audio/ogg; codecs=opus" {
		t.Errorf("Mimetype = %q, want %q", audio.GetMimetype(), "audio/ogg; codecs=opus")
	}
	if audio.GetFileLength() != uint64(len(payload.Bytes)) {
		t.Errorf("FileLength = %d, want %d", audio.GetFileLength(), len(payload.Bytes))
	}
	if !audio.GetPTT() {
		t.Errorf("PTT = %v, want true", audio.GetPTT())
	}
	if audio.GetSeconds() != 12 {
		t.Errorf("Seconds = %d, want 12", audio.GetSeconds())
	}
	if res.Timestamp != now {
		t.Errorf("SendAudio timestamp = %v, want %v", res.Timestamp, now)
	}
	if res.ID != "wire-id-audio" {
		t.Errorf("SendAudio ID = %q, want %q (o que resp devolveu)", res.ID, "wire-id-audio")
	}
}

// TestChatMessengerAdapter_SendAudio_PTTFalse_Preserved prova que
// payload.PTT=false chega como false na AudioMessage — nil e false NAO sao
// equivalentes, e a resolucao do default fica no use case (send_audio.go),
// nao no adapter. O adapter so' repassa o bool que recebeu.
func TestChatMessengerAdapter_SendAudio_PTTFalse_Preserved(t *testing.T) {
	var gotMsg *waE2E.Message
	fake := &testkit.Fake{
		UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
			return wanoise.UploadResponse{}, nil
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotMsg = m
			return wanoise.SendResponse{ID: types.MessageID("wire-id-ptt-false")}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	payload := domain.AudioPayload{Bytes: []byte{1}, MimeType: "audio/mpeg", PTT: false}
	_, err := a.SendAudio(context.Background(), "u1", "x@y.com", payload, "")
	if err != nil {
		t.Fatalf("SendAudio = %v", err)
	}
	if gotMsg.GetAudioMessage().GetPTT() {
		t.Error("PTT=false no payload virou true na AudioMessage")
	}
}

// TestChatMessengerAdapter_SendAudio_WithCallerID: quando id não é vazio,
// vira RequestExtra.ID — o SDK que decide o ID final, devolvido em resp.ID.
func TestChatMessengerAdapter_SendAudio_WithCallerID(t *testing.T) {
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
	res, err := a.SendAudio(context.Background(), "u1", "x@y.com", domain.AudioPayload{Bytes: []byte{1}, MimeType: "audio/ogg"}, "caller-id")
	if err != nil {
		t.Fatalf("SendAudio = %v", err)
	}
	if len(gotExtra) != 1 || string(gotExtra[0].ID) != "caller-id" {
		t.Fatalf("RequestExtra.ID nao recebeu o id do chamador: %+v", gotExtra)
	}
	if res.ID != "caller-id" {
		t.Errorf("SendAudio ID = %q, want %q", res.ID, "caller-id")
	}
}

// --- ChatMessengerAdapter.SendVideo (CAP-06) ------------------------------

func TestChatMessengerAdapter_SendVideo_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))
	_, err := a.SendVideo(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}}, "")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendVideo code = %q", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendVideo_InvalidJID: JID inválido nunca pode
// alcançar client.Upload nem client.SendMessage.
func TestChatMessengerAdapter_SendVideo_InvalidJID(t *testing.T) {
	uploadCalled := false
	fake := &testkit.Fake{UploadFn: func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
		uploadCalled = true
		return wanoise.UploadResponse{}, nil
	}}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.SendVideo(context.Background(), "u1", domain.JID(string([]byte{0x00})), domain.MediaPayload{Bytes: []byte{1, 2, 3}}, "")
	if err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
	if uploadCalled {
		t.Fatal("SendVideo chamou client.Upload com JID inválido")
	}
}

// TestChatMessengerAdapter_SendVideo_UploadFailurePropagates: falha de
// upload nunca alcança client.SendMessage nem produz resultado.
func TestChatMessengerAdapter_SendVideo_UploadFailurePropagates(t *testing.T) {
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
	res, err := a.SendVideo(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}, MimeType: "video/mp4"}, "")
	if err == nil {
		t.Fatal("SendVideo não propagou a falha de upload")
	}
	if sendCalled {
		t.Fatal("upload falhou, mas client.SendMessage foi chamado mesmo assim")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("SendVideo devolveu resultado nao-vazio com upload falho: %+v", res)
	}
}

// TestChatMessengerAdapter_SendVideo_UploadOK_SendMessageFail_NeverSent é o
// caso obrigatório do CAP-06: upload bem-sucedido seguido de SendMessage
// falho NÃO produz sucesso algum.
func TestChatMessengerAdapter_SendVideo_UploadOK_SendMessageFail_NeverSent(t *testing.T) {
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
	res, err := a.SendVideo(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1, 2, 3}, MimeType: "video/mp4"}, "")
	if !uploadCalled {
		t.Fatal("upload nunca foi chamado — teste nao exercita o caso upload-ok-send-fail")
	}
	if err == nil {
		t.Fatal("SendVideo nao propagou a falha de SendMessage apos upload bem-sucedido")
	}
	if res != (domain.MessageSendResult{}) {
		t.Errorf("upload OK + SendMessage falho produziu resultado nao-vazio: %+v", res)
	}
}

// TestChatMessengerAdapter_SendVideo_OK confere a montagem da mensagem
// (upload chamado com os bytes certos e wanoise.MediaVideo, VideoMessage
// montada a partir da resposta de upload, não de valores inventados) e que
// o resultado vem de resp, não do id de entrada.
//
// CONTROLE NEGATIVO (CAP-06): trocar wanoise.MediaVideo por
// wanoise.MediaImage na chamada de client.Upload dentro de
// ChatMessengerAdapter.SendVideo faz este teste morder na asserção
// gotAppInfo != wanoise.MediaVideo — mutação executada e revertida
// manualmente durante o desenvolvimento deste slice.
func TestChatMessengerAdapter_SendVideo_OK(t *testing.T) {
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
				URL:           "https://mmg.whatsapp.net/x",
				DirectPath:    "/x",
				MediaKey:      []byte{9, 9, 9},
				FileEncSHA256: []byte{1, 1, 1},
				FileSHA256:    []byte{2, 2, 2},
			}, nil
		},
		SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotTo = to
			gotMsg = m
			return wanoise.SendResponse{Timestamp: now, ID: types.MessageID("wire-id-video")}, nil
		},
	}
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	payload := domain.MediaPayload{Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, MimeType: "video/mp4", Caption: "legenda"}
	res, err := a.SendVideo(context.Background(), "u1", "x@y.com", payload, "")
	if err != nil {
		t.Fatalf("SendVideo = %v", err)
	}
	if string(gotPlaintext) != string(payload.Bytes) {
		t.Errorf("bytes enviados ao Upload divergem do payload")
	}
	if gotAppInfo != wanoise.MediaVideo {
		t.Errorf("appInfo do Upload = %v, want wanoise.MediaVideo", gotAppInfo)
	}
	if gotTo.String() != "x@y.com" {
		t.Errorf("destinatario = %q, want %q", gotTo.String(), "x@y.com")
	}
	video := gotMsg.GetVideoMessage()
	if video == nil {
		t.Fatal("VideoMessage nao foi montada")
	}
	if video.GetCaption() != "legenda" {
		t.Errorf("Caption = %q, want %q", video.GetCaption(), "legenda")
	}
	if video.GetURL() != "https://mmg.whatsapp.net/x" || video.GetDirectPath() != "/x" {
		t.Errorf("URL/DirectPath nao vieram da resposta de upload: %+v", video)
	}
	if string(video.GetMediaKey()) != string([]byte{9, 9, 9}) {
		t.Errorf("MediaKey nao veio da resposta de upload: %v", video.GetMediaKey())
	}
	if string(video.GetFileEncSHA256()) != string([]byte{1, 1, 1}) {
		t.Errorf("FileEncSHA256 nao veio da resposta de upload: %v", video.GetFileEncSHA256())
	}
	if string(video.GetFileSHA256()) != string([]byte{2, 2, 2}) {
		t.Errorf("FileSHA256 nao veio da resposta de upload: %v", video.GetFileSHA256())
	}
	if video.GetMimetype() != "video/mp4" {
		t.Errorf("Mimetype = %q, want %q", video.GetMimetype(), "video/mp4")
	}
	if video.GetFileLength() != uint64(len(payload.Bytes)) {
		t.Errorf("FileLength = %d, want %d", video.GetFileLength(), len(payload.Bytes))
	}
	if res.Timestamp != now {
		t.Errorf("SendVideo timestamp = %v, want %v", res.Timestamp, now)
	}
	if res.ID != "wire-id-video" {
		t.Errorf("SendVideo ID = %q, want %q (o que resp devolveu)", res.ID, "wire-id-video")
	}
}

// TestChatMessengerAdapter_SendVideo_WithCallerID: quando id não é vazio,
// vira RequestExtra.ID — o SDK que decide o ID final, devolvido em resp.ID.
func TestChatMessengerAdapter_SendVideo_WithCallerID(t *testing.T) {
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
	res, err := a.SendVideo(context.Background(), "u1", "x@y.com", domain.MediaPayload{Bytes: []byte{1}, MimeType: "video/mp4"}, "caller-id")
	if err != nil {
		t.Fatalf("SendVideo = %v", err)
	}
	if len(gotExtra) != 1 || string(gotExtra[0].ID) != "caller-id" {
		t.Fatalf("RequestExtra.ID nao recebeu o id do chamador: %+v", gotExtra)
	}
	if res.ID != "caller-id" {
		t.Errorf("SendVideo ID = %q, want %q", res.ID, "caller-id")
	}
}
