package chat

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

// Este arquivo cobre SendButtons (CAP-21) no nível em que a TRADUÇÃO fica
// visível: qual `Name` cada domain.ButtonType* vira, o CONTEÚDO do JSON de
// parâmetros de cada tipo, a ordem dos botões, o header (imagem ou título) e
// o nó BIZ sem o qual o aparelho não desenha botão nenhum. Acima daqui nada
// disso é observável — o use case entrega botões de domínio e não conhece
// protobuf.
//
// As asserções sobre os parâmetros são sobre o CONTEÚDO do JSON, chave por
// chave, e não sobre a presença do botão: `ButtonParamsJSON` é uma STRING, e
// um teste que só contasse botões passaria com `display_text` grafado errado
// — o aparelho é quem descobriria.

const buttonsChatJID = "5511987654321@s.whatsapp.net"

func buttonsAdapter(f *testkit.Fake) *ChatMessengerAdapter {
	return NewChatMessengerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": f}))
}

// sendButtonsCapturing envia payload e devolve a mensagem que chegou a
// SendMessage, mais os extras.
func sendButtonsCapturing(t *testing.T, payload domain.ButtonsPayload, _ *domain.ReplyContext, id string) (*waE2E.Message, []wanoise.SendRequestExtra) {
	t.Helper()
	sent, extra, _ := sendButtonsCapturingWithFake(t, &testkit.Fake{}, payload, nil, id)
	return sent, extra
}

// sendButtonsCapturingWithFake é o mesmo, mas deixa o chamador configurar o
// dublê do cliente (para exercitar o Upload) e devolve os bytes que foram a
// ele.
func sendButtonsCapturingWithFake(t *testing.T, f *testkit.Fake, payload domain.ButtonsPayload, _ *domain.ReplyContext, id string) (*waE2E.Message, []wanoise.SendRequestExtra, error) {
	t.Helper()

	var sent *waE2E.Message
	var gotExtra []wanoise.SendRequestExtra
	f.SendMessageFn = func(_ context.Context, _ types.JID, m *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
		sent, gotExtra = m, extra
		return wanoise.SendResponse{ID: "buttons-wire-id", Timestamp: time.Unix(1755500140, 0)}, nil
	}

	_, err := buttonsAdapter(f).SendButtons(context.Background(), "u1", buttonsChatJID, payload, nil, id)
	return sent, gotExtra, err
}

// nativeFlow extrai os botões nativos da mensagem, falhando com mensagem
// útil se ela não tiver a forma esperada.
func nativeFlow(t *testing.T, m *waE2E.Message) *waE2E.InteractiveMessage_NativeFlowMessage {
	t.Helper()
	im := m.GetInteractiveMessage()
	if im == nil {
		t.Fatalf("a mensagem enviada nao e' um InteractiveMessage: %+v", m)
	}
	nf := im.GetNativeFlowMessage()
	if nf == nil {
		t.Fatalf("InteractiveMessage sem NativeFlowMessage: %+v", im)
	}
	return nf
}

// buttonParams decodifica o ButtonParamsJSON do botão i. É o único caminho
// desta suite para os parâmetros: eles viajam como STRING, e comparar a
// string inteira travaria o teste na ORDEM das chaves, que o encoder de Go
// não garante ser estável entre versões.
func buttonParams(t *testing.T, nf *waE2E.InteractiveMessage_NativeFlowMessage, i int) map[string]string {
	t.Helper()
	btns := nf.GetButtons()
	if i >= len(btns) {
		t.Fatalf("botao %d nao existe; ha' %d", i, len(btns))
	}
	var params map[string]string
	if err := json.Unmarshal([]byte(btns[i].GetButtonParamsJSON()), &params); err != nil {
		t.Fatalf("ButtonParamsJSON do botao %d nao e' um objeto JSON: %v (%q)",
			i, err, btns[i].GetButtonParamsJSON())
	}
	return params
}

func TestChatMessengerAdapter_SendButtons_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))

	_, err := a.SendButtons(context.Background(), "u1", buttonsChatJID,
		domain.ButtonsPayload{Body: "corpo"}, nil, "")

	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendButtons code = %q, quero no_session", testkit.AppErrCode(err))
	}
}

// TestChatMessengerAdapter_SendButtons_QuickReplyParams: `reply` vira
// `quick_reply`, com display_text e id.
func TestChatMessengerAdapter_SendButtons_QuickReplyParams(t *testing.T) {
	sent, _ := sendButtonsCapturing(t, domain.ButtonsPayload{
		Body:    "Escolha",
		Buttons: []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "btn-sim"}},
	}, nil, "")

	nf := nativeFlow(t, sent)
	if got := nf.GetButtons()[0].GetName(); got != "quick_reply" {
		t.Errorf("Name = %q, quero %q (o tipo publico e' \"reply\", o do wire NAO)", got, "quick_reply")
	}
	params := buttonParams(t, nf, 0)
	if params["display_text"] != "Sim" {
		t.Errorf("display_text = %q, quero %q", params["display_text"], "Sim")
	}
	if params["id"] != "btn-sim" {
		t.Errorf("id = %q, quero %q", params["id"], "btn-sim")
	}
	if len(params) != 2 {
		t.Errorf("quick_reply tem %d parametro(s) (%v), quero exatamente display_text e id", len(params), params)
	}
}

// TestChatMessengerAdapter_SendButtons_CTAURLWritesURLTwice: em `cta_url` a
// MESMA url vai em DOIS campos, `url` e `merchant_url`. Preencher só um é o
// erro mais fácil de cometer e o mais difícil de ver — o botão fica sem
// destino em parte dos clientes.
func TestChatMessengerAdapter_SendButtons_CTAURLWritesURLTwice(t *testing.T) {
	const url = "https://example.invalid/promo"
	sent, _ := sendButtonsCapturing(t, domain.ButtonsPayload{
		Body:    "Escolha",
		Buttons: []domain.InteractiveButton{{Type: domain.ButtonTypeCTAURL, Title: "Site", ID: "Site", URL: url}},
	}, nil, "")

	nf := nativeFlow(t, sent)
	if got := nf.GetButtons()[0].GetName(); got != "cta_url" {
		t.Errorf("Name = %q, quero cta_url", got)
	}
	params := buttonParams(t, nf, 0)
	if params["url"] != url {
		t.Errorf("url = %q, quero %q", params["url"], url)
	}
	if params["merchant_url"] != url {
		t.Errorf("merchant_url = %q, quero %q (a MESMA url dos dois lados)", params["merchant_url"], url)
	}
	if params["display_text"] != "Site" {
		t.Errorf("display_text = %q, quero %q", params["display_text"], "Site")
	}
	if len(params) != 3 {
		t.Errorf("cta_url tem %d parametro(s) (%v), quero display_text, url e merchant_url", len(params), params)
	}
}

// TestChatMessengerAdapter_SendButtons_CTACallParams: `cta_call` leva
// phone_number, e nada de url.
func TestChatMessengerAdapter_SendButtons_CTACallParams(t *testing.T) {
	sent, _ := sendButtonsCapturing(t, domain.ButtonsPayload{
		Body: "Escolha",
		Buttons: []domain.InteractiveButton{
			{Type: domain.ButtonTypeCTACall, Title: "Ligar", ID: "Ligar", PhoneNumber: "+5511987654321"},
		},
	}, nil, "")

	nf := nativeFlow(t, sent)
	if got := nf.GetButtons()[0].GetName(); got != "cta_call" {
		t.Errorf("Name = %q, quero cta_call", got)
	}
	params := buttonParams(t, nf, 0)
	if params["phone_number"] != "+5511987654321" {
		t.Errorf("phone_number = %q, quero %q", params["phone_number"], "+5511987654321")
	}
	if len(params) != 2 {
		t.Errorf("cta_call tem %d parametro(s) (%v), quero display_text e phone_number", len(params), params)
	}
}

// TestChatMessengerAdapter_SendButtons_CopyBecomesCTACopy é o eixo em que o
// mapa tipo->Name deixa de ser identidade: o tipo público é `copy` e o Name
// do wire é `cta_copy`. Um `Name` igual ao tipo público produz um botão que
// o aparelho não sabe desenhar.
func TestChatMessengerAdapter_SendButtons_CopyBecomesCTACopy(t *testing.T) {
	sent, _ := sendButtonsCapturing(t, domain.ButtonsPayload{
		Body: "Escolha",
		Buttons: []domain.InteractiveButton{
			{Type: domain.ButtonTypeCopy, Title: "Copiar", ID: "Copiar", CopyCode: "PROMO10"},
		},
	}, nil, "")

	nf := nativeFlow(t, sent)
	if got := nf.GetButtons()[0].GetName(); got != "cta_copy" {
		t.Errorf("Name = %q, quero %q — o tipo publico e' \"copy\", o do wire NAO e' igual", got, "cta_copy")
	}
	params := buttonParams(t, nf, 0)
	if params["copy_code"] != "PROMO10" {
		t.Errorf("copy_code = %q, quero %q", params["copy_code"], "PROMO10")
	}
	if len(params) != 2 {
		t.Errorf("cta_copy tem %d parametro(s) (%v), quero display_text e copy_code", len(params), params)
	}
}

// TestChatMessengerAdapter_SendButtons_OrderIsPreserved: os quatro tipos, na
// ordem em que chegaram. É a ordem em que aparecem no aparelho de quem
// recebe.
func TestChatMessengerAdapter_SendButtons_OrderIsPreserved(t *testing.T) {
	sent, _ := sendButtonsCapturing(t, domain.ButtonsPayload{
		Body: "Escolha",
		Buttons: []domain.InteractiveButton{
			{Type: domain.ButtonTypeReply, Title: "A", ID: "a"},
			{Type: domain.ButtonTypeCTAURL, Title: "B", ID: "b", URL: "https://example.invalid/b"},
			{Type: domain.ButtonTypeCTACall, Title: "C", ID: "c", PhoneNumber: "+55119"},
			{Type: domain.ButtonTypeCopy, Title: "D", ID: "d", CopyCode: "X"},
		},
	}, nil, "")

	nf := nativeFlow(t, sent)
	want := []string{"quick_reply", "cta_url", "cta_call", "cta_copy"}
	btns := nf.GetButtons()
	if len(btns) != len(want) {
		t.Fatalf("chegaram %d botao(oes), quero %d", len(btns), len(want))
	}
	for i, name := range want {
		if got := btns[i].GetName(); got != name {
			t.Errorf("Buttons[%d].Name = %q, quero %q (a ORDEM importa)", i, got, name)
		}
	}
	if got := nf.GetMessageVersion(); got != 1 {
		t.Errorf("MessageVersion = %d, quero 1 (o do historico)", got)
	}
}

// TestChatMessengerAdapter_SendButtons_BodyAndFooter: o corpo sempre; o
// rodapé só quando não vazio (o histórico só montava o Footer nesse caso).
func TestChatMessengerAdapter_SendButtons_BodyAndFooter(t *testing.T) {
	sent, _ := sendButtonsCapturing(t, domain.ButtonsPayload{
		Body:    "Escolha uma opcao",
		Footer:  "Equipe wa-api",
		Buttons: []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "s"}},
	}, nil, "")

	im := sent.GetInteractiveMessage()
	if got := im.GetBody().GetText(); got != "Escolha uma opcao" {
		t.Errorf("Body.Text = %q, quero o Body do payload", got)
	}
	if got := im.GetFooter().GetText(); got != "Equipe wa-api" {
		t.Errorf("Footer.Text = %q, quero o Footer do payload", got)
	}

	semFooter, _ := sendButtonsCapturing(t, domain.ButtonsPayload{
		Body:    "Escolha",
		Buttons: []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "s"}},
	}, nil, "")
	if semFooter.GetInteractiveMessage().GetFooter() != nil {
		t.Error("Footer foi montado com o campo vazio; o historico so' o monta quando ha' texto")
	}
}

// TestChatMessengerAdapter_SendButtons_HeaderTitleWhenNoImage: sem imagem, o
// Title vira texto de header, e nenhum anexo é declarado.
func TestChatMessengerAdapter_SendButtons_HeaderTitleWhenNoImage(t *testing.T) {
	sent, _ := sendButtonsCapturing(t, domain.ButtonsPayload{
		Body:    "Escolha",
		Title:   "Cabecalho",
		Buttons: []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "s"}},
	}, nil, "")

	header := sent.GetInteractiveMessage().GetHeader()
	if got := header.GetTitle(); got != "Cabecalho" {
		t.Errorf("Header.Title = %q, quero o Title do payload", got)
	}
	if header.GetHasMediaAttachment() {
		t.Error("HasMediaAttachment=true sem imagem no payload")
	}
	if header.GetImageMessage() != nil {
		t.Error("Header carregou ImageMessage sem imagem no payload")
	}
}

// TestChatMessengerAdapter_SendButtons_HeaderImageIsUploaded: com bytes no
// payload, o adapter sobe a imagem com wanoise.MediaImage e monta o header
// de mídia a partir do que o Upload devolveu. A PRIORIDADE é a do histórico:
// com imagem, o Title NÃO vira texto de header.
func TestChatMessengerAdapter_SendButtons_HeaderImageIsUploaded(t *testing.T) {
	var gotBytes []byte
	var gotAppInfo wanoise.MediaType
	f := &testkit.Fake{
		UploadFn: func(_ context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
			gotBytes, gotAppInfo = plaintext, appInfo
			return wanoise.UploadResponse{
				URL:           "https://mmg.whatsapp.net/header",
				DirectPath:    "/header",
				MediaKey:      []byte("chave"),
				FileEncSHA256: []byte("enc"),
				FileSHA256:    []byte("sha"),
			}, nil
		},
	}

	bytes := []byte("bytes-do-header")
	sent, _, err := sendButtonsCapturingWithFake(t, f, domain.ButtonsPayload{
		Body:                "Escolha",
		Title:               "Cabecalho que NAO deve aparecer",
		Buttons:             []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "s"}},
		HeaderImage:         bytes,
		HeaderImageMimeType: "image/png",
	}, nil, "")
	if err != nil {
		t.Fatalf("SendButtons: %v", err)
	}

	if string(gotBytes) != string(bytes) {
		t.Errorf("bytes enviados ao Upload = %q, quero os do payload", gotBytes)
	}
	if gotAppInfo != wanoise.MediaImage {
		t.Errorf("appInfo do Upload = %v, quero wanoise.MediaImage", gotAppInfo)
	}

	header := sent.GetInteractiveMessage().GetHeader()
	if !header.GetHasMediaAttachment() {
		t.Error("HasMediaAttachment=false com imagem no payload")
	}
	img := header.GetImageMessage()
	if img == nil {
		t.Fatal("Header sem ImageMessage apesar do upload")
	}
	if img.GetURL() != "https://mmg.whatsapp.net/header" || img.GetDirectPath() != "/header" {
		t.Errorf("ImageMessage nao carrega o que o Upload devolveu: %+v", img)
	}
	if img.GetMimetype() != "image/png" {
		t.Errorf("Mimetype = %q, quero o do payload", img.GetMimetype())
	}
	if img.GetFileLength() != uint64(len(bytes)) {
		t.Errorf("FileLength = %d, quero %d", img.GetFileLength(), len(bytes))
	}
	if header.GetTitle() != "" {
		t.Errorf("Header.Title = %q; com imagem o historico NAO escreve titulo", header.GetTitle())
	}
}

// TestChatMessengerAdapter_SendButtons_UploadFailureNeverSends: o upload
// falha, e a mensagem não sai. Sem isso, um header quebrado viraria uma
// mensagem enviada sem header e ninguém saberia.
func TestChatMessengerAdapter_SendButtons_UploadFailureNeverSends(t *testing.T) {
	uploadErr := errors.New("upload recusado")
	sendCalled := false
	f := &testkit.Fake{
		UploadFn: func(context.Context, []byte, wanoise.MediaType) (wanoise.UploadResponse, error) {
			return wanoise.UploadResponse{}, uploadErr
		},
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			sendCalled = true
			return wanoise.SendResponse{}, nil
		},
	}

	_, err := buttonsAdapter(f).SendButtons(context.Background(), "u1", buttonsChatJID, domain.ButtonsPayload{
		Body:        "Escolha",
		Buttons:     []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "s"}},
		HeaderImage: []byte("bytes"),
	}, nil, "")

	if !errors.Is(err, uploadErr) {
		t.Fatalf("erro do upload nao chegou ao chamador: got %#v", err)
	}
	if sendCalled {
		t.Error("upload falhou mas SendMessage foi chamado")
	}
}

// TestChatMessengerAdapter_SendButtons_BizNodeIsAlwaysSent trava o nó BIZ,
// que o histórico chamava de "fundamental para renderizar os botões": sem
// ele o aparelho recebe a mensagem e NÃO desenha botão nenhum. Nada acima do
// adapter observa isto.
func TestChatMessengerAdapter_SendButtons_BizNodeIsAlwaysSent(t *testing.T) {
	_, extra := sendButtonsCapturing(t, domain.ButtonsPayload{
		Body:    "Escolha",
		Buttons: []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "s"}},
	}, nil, "")

	if len(extra) != 1 {
		t.Fatalf("SendMessage recebeu %d extra(s), quero 1", len(extra))
	}
	if extra[0].AdditionalNodes == nil {
		t.Fatal("AdditionalNodes nil: o no' BIZ nao foi enviado e os botoes nao renderizam")
	}
	nodes := *extra[0].AdditionalNodes
	if len(nodes) != 1 || nodes[0].Tag != "biz" {
		t.Fatalf("no' raiz = %+v, quero um unico no' \"biz\"", nodes)
	}
	interactive, ok := nodes[0].Content.([]waBinary.Node)
	if !ok || len(interactive) != 1 || interactive[0].Tag != "interactive" {
		t.Fatalf("conteudo do \"biz\" = %+v, quero um unico no' \"interactive\"", nodes[0].Content)
	}
	if got := interactive[0].Attrs["type"]; got != "native_flow" {
		t.Errorf("interactive.type = %v, quero native_flow", got)
	}
	if got := interactive[0].Attrs["v"]; got != "1" {
		t.Errorf("interactive.v = %v, quero \"1\"", got)
	}
	flow, ok := interactive[0].Content.([]waBinary.Node)
	if !ok || len(flow) != 1 || flow[0].Tag != "native_flow" {
		t.Fatalf("conteudo do \"interactive\" = %+v, quero um unico no' \"native_flow\"", interactive[0].Content)
	}
	if got := flow[0].Attrs["v"]; got != "9" {
		t.Errorf("native_flow.v = %v, quero \"9\"", got)
	}
	if got := flow[0].Attrs["name"]; got != "mixed" {
		t.Errorf("native_flow.name = %v, quero \"mixed\"", got)
	}
}

// TestChatMessengerAdapter_SendButtons_CallerIDIsForwarded: o id pedido pelo
// chamador vai no extra, junto do nó BIZ — e a AUSÊNCIA dele não pode
// derrubar o nó, que é o erro fácil de cometer com `SendRequestExtra` sendo
// UM struct e não dois.
func TestChatMessengerAdapter_SendButtons_CallerIDIsForwarded(t *testing.T) {
	payload := domain.ButtonsPayload{
		Body:    "Escolha",
		Buttons: []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "s"}},
	}

	_, comID := sendButtonsCapturing(t, payload, nil, "id-do-cliente")
	if got := string(comID[0].ID); got != "id-do-cliente" {
		t.Errorf("extra.ID = %q, quero o id do chamador", got)
	}
	if comID[0].AdditionalNodes == nil {
		t.Error("com id do chamador, o no' BIZ sumiu")
	}

	_, semID := sendButtonsCapturing(t, payload, nil, "")
	if got := string(semID[0].ID); got != "" {
		t.Errorf("extra.ID = %q sem id do chamador, quero vazio", got)
	}
	if semID[0].AdditionalNodes == nil {
		t.Error("sem id do chamador, o no' BIZ sumiu")
	}
}

// TestChatMessengerAdapter_SendButtons_ResultComesFromTheWire: ID e
// Timestamp são os que a sessão REALMENTE usou, nunca fabricados
// localmente.
func TestChatMessengerAdapter_SendButtons_ResultComesFromTheWire(t *testing.T) {
	sentAt := time.Unix(1755500140, 0)
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			return wanoise.SendResponse{ID: "id-do-wire", Timestamp: sentAt}, nil
		},
	}

	got, err := buttonsAdapter(f).SendButtons(context.Background(), "u1", buttonsChatJID, domain.ButtonsPayload{
		Body:    "Escolha",
		Buttons: []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "s"}},
	}, nil, "id-do-cliente")
	if err != nil {
		t.Fatalf("SendButtons: %v", err)
	}
	if got.ID != "id-do-wire" {
		t.Errorf("ID = %q, quero o do wire (e nao o do chamador)", got.ID)
	}
	if !got.Timestamp.Equal(sentAt) {
		t.Errorf("Timestamp = %v, quero %v", got.Timestamp, sentAt)
	}
}

// TestChatMessengerAdapter_SendButtons_InvalidJIDNeverUploads: JID que não
// parseia é recusado ANTES do upload — subir bytes para uma mensagem que
// nunca vai sair é lixo que o protocolo não deixa desfazer.
func TestChatMessengerAdapter_SendButtons_InvalidJIDNeverUploads(t *testing.T) {
	f := &testkit.Fake{
		UploadFn: func(context.Context, []byte, wanoise.MediaType) (wanoise.UploadResponse, error) {
			t.Error("SendButtons chamou client.Upload com JID invalido")
			return wanoise.UploadResponse{}, nil
		},
	}

	_, err := buttonsAdapter(f).SendButtons(context.Background(), "u1", "@@@", domain.ButtonsPayload{
		Body:        "Escolha",
		Buttons:     []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "s"}},
		HeaderImage: []byte("bytes"),
	}, nil, "")
	if err == nil {
		t.Fatal("JID invalido foi aceito")
	}
}

// TestChatMessengerAdapter_SendButtons_WithReplyTo: ContextInfo set on
// InteractiveMessage when replyTo is present (CAP-46B).
func TestChatMessengerAdapter_SendButtons_WithReplyTo(t *testing.T) {
	var gotMsg *waE2E.Message
	f := &testkit.Fake{
		SendMessageFn: func(_ context.Context, _ types.JID, m *waE2E.Message, _ ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			gotMsg = m
			return wanoise.SendResponse{ID: "wire-reply-btn"}, nil
		},
	}
	reply := &domain.ReplyContext{StanzaID: "q-btn", Participant: "5511888888888@s.whatsapp.net"}
	payload := domain.ButtonsPayload{
		Body:    "Escolha",
		Buttons: []domain.InteractiveButton{{Type: domain.ButtonTypeReply, Title: "Sim", ID: "btn-sim"}},
	}
	_, err := buttonsAdapter(f).SendButtons(context.Background(), "u1", buttonsChatJID, payload, reply, "")
	if err != nil {
		t.Fatalf("SendButtons = %v", err)
	}
	interactive := gotMsg.GetInteractiveMessage()
	if interactive == nil {
		t.Fatal("InteractiveMessage nil")
	}
	ci := interactive.GetContextInfo()
	if ci == nil {
		t.Fatal("ContextInfo nil with replyTo present")
	}
	if ci.GetStanzaID() != "q-btn" {
		t.Errorf("StanzaID = %q, want %q", ci.GetStanzaID(), "q-btn")
	}
}
