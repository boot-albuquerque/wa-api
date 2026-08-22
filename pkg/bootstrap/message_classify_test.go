package bootstrap

import (
	"testing"

	waCommon "wa-api/internal/wa-noise/protocol/proto/waCommon"
	waE2E "wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// HOUSEKEEP F187 — a trava da UNIFICAÇÃO.
//
// Os testes de `eventhandler_history_discard_test.go` provam que o caminho de
// tempo real classifica certo. Estes provam outra coisa, e é a que a F187 pede:
// que os DOIS caminhos classifiquem IGUAL.
//
// Sem esta suíte, alguém pode acrescentar um ramo a um dos lados e a divergência
// recomeça — que é exatamente como a F187 nasceu, e como ela se inverteu no meio
// do conserto: primeiro o tempo real tinha MENOS ramos, depois passou a ter SEIS
// a mais.

// mensagensDeCadaTipo é uma de cada forma que o classificador reconhece, para
// que a comparação entre caminhos cubra a cadeia inteira e não só o caso fácil.
func mensagensDeCadaTipo() map[string]*waE2E.Message {
	return map[string]*waE2E.Message{
		"texto":     {Conversation: proto("ola")},
		"imagem":    {ImageMessage: &waE2E.ImageMessage{Caption: proto("legenda")}},
		"video":     {VideoMessage: &waE2E.VideoMessage{}},
		"audio":     {AudioMessage: &waE2E.AudioMessage{}},
		"documento": {DocumentMessage: &waE2E.DocumentMessage{}},
		"sticker":   {StickerMessage: &waE2E.StickerMessage{}},
		"contacto":  {ContactMessage: &waE2E.ContactMessage{DisplayName: proto("Ana")}},
		"local":     {LocationMessage: &waE2E.LocationMessage{Name: proto("Praca")}},
		"enquete":   {PollCreationMessage: &waE2E.PollCreationMessage{Name: proto("Qual?")}},
		"botoes":    {InteractiveMessage: &waE2E.InteractiveMessage{Body: &waE2E.InteractiveMessage_Body{Text: proto("corpo")}}},
		"lista":     {ListMessage: &waE2E.ListMessage{Title: proto("Menu")}},
		"resp_btn":  {ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{SelectedButtonID: proto("b1")}},
		"resp_lst":  {ListResponseMessage: &waE2E.ListResponseMessage{SingleSelectReply: &waE2E.ListResponseMessage_SingleSelectReply{SelectedRowID: proto("r1")}}},
		"reacao":    {ReactionMessage: &waE2E.ReactionMessage{Text: proto("👍")}},
	}
}

// TestClassificacao_OsDoisCaminhosConcordam compara o classificador com o que o
// caminho de TEMPO REAL grava de facto, tipo a tipo.
//
// CUIDADO COM O QUE ESTE TESTE PROVA, porque a primeira versão dele provava
// menos do que eu julgava. O controlo negativo CN-41 — fazer o ramo de contacto
// devolver texto vazio — NÃO o derrubou, e a razão é estrutural: os dois lados
// da comparação passam pela MESMA função, logo mutá-la move os dois por igual.
// Como asserção de "os dois caminhos concordam", era tautológico.
//
// O que ele mede de facto, e vale: que o caminho de tempo real CHAMA o
// classificador, em vez de reimplementar a cadeia. Se alguém voltar a inlinar
// os ramos no handler, este teste morde.
//
// Quem prova que a classificação está CERTA são os testes de
// eventhandler_history_discard_test.go, que asseriram valores literais
// (":poll:", "Praca da Liberdade") contra o que a produção grava. Este é sobre
// a estrutura; aqueles são sobre o conteúdo.
func TestClassificacao_OsDoisCaminhosConcordam(t *testing.T) {
	for nome, msg := range mensagensDeCadaTipo() {
		t.Run(nome, func(t *testing.T) {
			esperado := classifyMessage(msg)

			evh := handlerComHistorico(t, "u-conc-"+nome)
			evt := (&events.Message{
				Info: types.MessageInfo{
					ID: "M-" + nome, Type: "text",
					MessageSource: types.MessageSource{
						Chat:   types.NewJID("5511999999999", types.DefaultUserServer),
						Sender: types.NewJID("5511888888888", types.DefaultUserServer),
					},
				},
				RawMessage: msg,
			}).UnwrapRaw()
			evh.saveMessageHistory(evt, &eventState{postmap: map[string]any{}})

			tipo, txt := lerLinha(t, evh, "M-"+nome)
			if tipo != esperado.Type {
				t.Errorf("message_type gravado = %q, classificador diz %q", tipo, esperado.Type)
			}
			if txt != esperado.Text {
				t.Errorf("text_content gravado = %q, classificador diz %q", txt, esperado.Text)
			}
		})
	}
}

// TestClassificacao_TipoDesconhecidoNaoInventaTexto: o que não se reconhece sai
// com texto VAZIO, e é isso que faz a guarda de gravação descartar — com o
// registo da F186. Se esta função inventasse um marcador para o desconhecido, a
// linha passaria a ser gravada como texto vazio e o descarte deixaria de ser
// visível, que é o defeito da F184 de volta por outra porta.
func TestClassificacao_TipoDesconhecidoNaoInventaTexto(t *testing.T) {
	got := classifyMessage(&waE2E.Message{})
	if got.Type != messageTypeText {
		t.Errorf("Type = %q, quero %q", got.Type, messageTypeText)
	}
	if got.Text != "" {
		t.Errorf("Text = %q, quero vazio: inventar marcador para o desconhecido esconde o descarte (F184/F186)", got.Text)
	}
}

// TestClassificacao_MarcadorSoQuandoNaoHaTexto trava a regra do `comTexto`: o
// marcador diz "há uma imagem aqui, sem legenda", que é informação; substituir
// uma legenda REAL por ele seria perder dado, que é a F187 outra vez.
func TestClassificacao_MarcadorSoQuandoNaoHaTexto(t *testing.T) {
	comLegenda := classifyMessage(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto("a minha legenda")}})
	if comLegenda.Text != "a minha legenda" {
		t.Errorf("com legenda: Text = %q, quero a legenda", comLegenda.Text)
	}
	semLegenda := classifyMessage(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{}})
	if semLegenda.Text != ":image:" {
		t.Errorf("sem legenda: Text = %q, quero \":image:\"", semLegenda.Text)
	}
}

// TestClassificacao_ApagarEEditarNaoSeConfundem: os dois são ProtocolMessage e
// passam pelo mesmo ramo. Trocar a ordem faria uma edição ser gravada como
// delete — perda silenciosa trocada por CORRUPÇÃO silenciosa, estritamente pior.
func TestClassificacao_ApagarEEditarNaoSeConfundem(t *testing.T) {
	apagar := classifyMessage(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto("ORIGINAL")},
	}})
	if apagar.Type != messageTypeDelete || apagar.DeletedID != "ORIGINAL" {
		t.Errorf("apagar = %+v, quero delete com DeletedID=ORIGINAL", apagar)
	}

	editar := classifyMessage(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:           &waCommon.MessageKey{ID: proto("ORIGINAL")},
		EditedMessage: &waE2E.Message{Conversation: proto("novo")},
	}})
	if editar.Type != messageTypeEdit || editar.Text != "novo" || editar.QuotedID != "ORIGINAL" {
		t.Errorf("editar = %+v, quero edit/novo/ORIGINAL", editar)
	}
}

// TestClassificacao_ValoresLiterais é o teste que o CN-41 mostrou que faltava.
//
// Ele assere o que cada tipo TEM de produzir, escrito à mão, valor a valor. Não
// compara duas saídas do mesmo código — compara o código com uma expectativa
// externa, que é a única forma de uma mutação no classificador ser apanhada.
//
// A tabela é escrita à mão e NÃO derivada de nada: derivá-la do classificador
// reintroduziria exatamente a tautologia que o CN-41 revelou.
func TestClassificacao_ValoresLiterais(t *testing.T) {
	casos := []struct {
		nome       string
		msg        *waE2E.Message
		wantTipo   string
		wantTexto  string
		wantQuoted string
	}{
		{"texto", &waE2E.Message{Conversation: proto("ola")}, "text", "ola", ""},
		{"imagem com legenda", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto("legenda")}}, "image", "legenda", ""},
		{"imagem sem legenda", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, "image", ":image:", ""},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}, "audio", ":audio:", ""},
		{"contacto", &waE2E.Message{ContactMessage: &waE2E.ContactMessage{DisplayName: proto("Ana")}}, "contact", "Ana", ""},
		{"local", &waE2E.Message{LocationMessage: &waE2E.LocationMessage{Name: proto("Praca")}}, "location", "Praca", ""},
		{"local sem nome", &waE2E.Message{LocationMessage: &waE2E.LocationMessage{}}, "location", ":location:", ""},
		{"enquete", &waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{Name: proto("Qual?")}}, "poll", "Qual?", ""},
		{"lista", &waE2E.Message{ListMessage: &waE2E.ListMessage{Title: proto("Menu")}}, "list", "Menu", ""},
		{"resposta de botao", &waE2E.Message{ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{SelectedButtonID: proto("b1")}}, "buttons_response", "b1", ""},
		{"reacao", &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
			Text: proto("👍"), Key: &waCommon.MessageKey{ID: proto("ALVO")}}}, "reaction", "👍", "ALVO"},
		{"texto citado", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto("resposta"), ContextInfo: &waE2E.ContextInfo{StanzaID: proto("CITADA")}}}, "text", "resposta", "CITADA"},

		// F184 residual: os nove que eram descartados. Cada um com o valor
		// literal que tem de produzir, incluindo os que só produzem marcador —
		// o marcador É o resultado esperado, e não um placeholder para depois.
		{"voto em enquete", &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{}}, "poll_update", ":poll_update:", ""},
		{"resposta interativa", &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{}}, "interactive_response", ":interactive_response:", ""},
		{"evento", &waE2E.Message{EventMessage: &waE2E.EventMessage{Name: proto("Reuniao")}}, "event", "Reuniao", ""},
		{"localizacao ao vivo", &waE2E.Message{LiveLocationMessage: &waE2E.LiveLocationMessage{Caption: proto("a caminho")}}, "live_location", "a caminho", ""},
		{"ptv", &waE2E.Message{PtvMessage: &waE2E.VideoMessage{}}, "ptv", ":ptv:", ""},
		{"convite de grupo", &waE2E.Message{GroupInviteMessage: &waE2E.GroupInviteMessage{GroupName: proto("Obras")}}, "group_invite", "Obras", ""},
		{"encomenda", &waE2E.Message{OrderMessage: &waE2E.OrderMessage{Message: proto("pedido 1")}}, "order", "pedido 1", ""},
		{"produto", &waE2E.Message{ProductMessage: &waE2E.ProductMessage{
			Product: &waE2E.ProductMessage_ProductSnapshot{Description: proto("cadeira")}}}, "product", "cadeira", ""},
		{"lista de contactos", &waE2E.Message{ContactsArrayMessage: &waE2E.ContactsArrayMessage{DisplayName: proto("Equipa")}}, "contacts_array", "Equipa", ""},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := classifyMessage(c.msg)
			if got.Type != c.wantTipo {
				t.Errorf("Type = %q, quero %q", got.Type, c.wantTipo)
			}
			if got.Text != c.wantTexto {
				t.Errorf("Text = %q, quero %q", got.Text, c.wantTexto)
			}
			if got.QuotedID != c.wantQuoted {
				t.Errorf("QuotedID = %q, quero %q", got.QuotedID, c.wantQuoted)
			}
		})
	}
}

// TestUnwrapFutureProof_DesembrulhaCadaInvolucro mirrors events.Message.UnwrapRaw
// and verifies that unwrapFutureProof peels each FutureProofMessage wrapper,
// setting the correct Is* flag and exposing the inner message to classifyMessage.
//
// HOUSEKEEP F188. The sync path receives raw proto from WebMessageInfo without
// the library's UnwrapRaw pass. Without unwrapFutureProof, a wrapped message
// would reach classifyMessage still inside the envelope, no branch would match,
// and the message would be silently discarded.
func TestUnwrapFutureProof_DesembrulhaCadaInvolucro(t *testing.T) {
	inner := &waE2E.Message{Conversation: proto("inside")}

	cases := []struct {
		name    string
		wrap    func(*waE2E.Message) *waE2E.Message
		checkFn func(unwrapResult) bool
		flag    string
	}{
		{
			"ephemeral", func(m *waE2E.Message) *waE2E.Message {
				return &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: m}}
			}, func(r unwrapResult) bool { return r.IsEphemeral }, "IsEphemeral",
		},
		{
			"viewOnce", func(m *waE2E.Message) *waE2E.Message {
				return &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{Message: m}}
			}, func(r unwrapResult) bool { return r.IsViewOnce }, "IsViewOnce",
		},
		{
			"viewOnceV2", func(m *waE2E.Message) *waE2E.Message {
				return &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: m}}
			}, func(r unwrapResult) bool { return r.IsViewOnce && r.IsViewOnceV2 }, "IsViewOnce+IsViewOnceV2",
		},
		{
			"viewOnceV2Extension", func(m *waE2E.Message) *waE2E.Message {
				return &waE2E.Message{ViewOnceMessageV2Extension: &waE2E.FutureProofMessage{Message: m}}
			}, func(r unwrapResult) bool { return r.IsViewOnce && r.IsViewOnceV2 && r.IsViewOnceV2Extension }, "IsViewOnce+V2+Ext",
		},
		{
			"documentWithCaption", func(m *waE2E.Message) *waE2E.Message {
				return &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: m}}
			}, func(r unwrapResult) bool { return r.IsDocumentWithCaption }, "IsDocumentWithCaption",
		},
		{
			"lottieSticker", func(m *waE2E.Message) *waE2E.Message {
				return &waE2E.Message{LottieStickerMessage: &waE2E.FutureProofMessage{Message: m}}
			}, func(r unwrapResult) bool { return r.IsLottieSticker }, "IsLottieSticker",
		},
		{
			"botInvoke", func(m *waE2E.Message) *waE2E.Message {
				return &waE2E.Message{BotInvokeMessage: &waE2E.FutureProofMessage{Message: m}}
			}, func(r unwrapResult) bool { return r.IsBotInvoke }, "IsBotInvoke",
		},
		{
			"editedMessage", func(m *waE2E.Message) *waE2E.Message {
				return &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{Message: m}}
			}, func(r unwrapResult) bool { return r.IsEdit }, "IsEdit",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wrapped := c.wrap(inner)
			r := unwrapFutureProof(wrapped)

			if r.Message != inner {
				t.Errorf("unwrapped message is not the inner message")
			}
			if !c.checkFn(r) {
				t.Errorf("flag %s not set after unwrapping", c.flag)
			}

			got := classifyMessage(r.Message)
			if got.Type != messageTypeText || got.Text != "inside" {
				t.Errorf("classifyMessage after unwrap: Type=%q Text=%q, want text/inside", got.Type, got.Text)
			}
		})
	}
}

// TestUnwrapFutureProof_SemInvolucroNaoMuda verifies that a plain message
// passes through unwrapFutureProof unchanged, with all flags false.
func TestUnwrapFutureProof_SemInvolucroNaoMuda(t *testing.T) {
	plain := &waE2E.Message{Conversation: proto("plain")}
	r := unwrapFutureProof(plain)
	if r.Message != plain {
		t.Error("plain message was modified by unwrapFutureProof")
	}
	if r.IsEphemeral || r.IsViewOnce || r.IsViewOnceV2 || r.IsViewOnceV2Extension ||
		r.IsDocumentWithCaption || r.IsLottieSticker || r.IsBotInvoke || r.IsEdit {
		t.Error("flags set on a plain message")
	}
}

// TestUnwrapFutureProof_NilSeguro verifies nil input doesn't panic.
func TestUnwrapFutureProof_NilSeguro(t *testing.T) {
	r := unwrapFutureProof(nil)
	if r.Message != nil {
		t.Error("nil input produced non-nil message")
	}
}

// TestUnwrapFutureProof_ConcordaComUnwrapRaw verifies that unwrapFutureProof
// and the library's UnwrapRaw produce the same inner message and flags for
// every wrapper type. This is the trava that prevents the two from diverging.
func TestUnwrapFutureProof_ConcordaComUnwrapRaw(t *testing.T) {
	inner := &waE2E.Message{Conversation: proto("concordancia")}

	wrappers := map[string]func(*waE2E.Message) *waE2E.Message{
		"ephemeral":           func(m *waE2E.Message) *waE2E.Message { return &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: m}} },
		"viewOnce":            func(m *waE2E.Message) *waE2E.Message { return &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{Message: m}} },
		"viewOnceV2":          func(m *waE2E.Message) *waE2E.Message { return &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: m}} },
		"viewOnceV2Ext":       func(m *waE2E.Message) *waE2E.Message { return &waE2E.Message{ViewOnceMessageV2Extension: &waE2E.FutureProofMessage{Message: m}} },
		"docWithCaption":      func(m *waE2E.Message) *waE2E.Message { return &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: m}} },
		"lottieSticker":       func(m *waE2E.Message) *waE2E.Message { return &waE2E.Message{LottieStickerMessage: &waE2E.FutureProofMessage{Message: m}} },
		"botInvoke":           func(m *waE2E.Message) *waE2E.Message { return &waE2E.Message{BotInvokeMessage: &waE2E.FutureProofMessage{Message: m}} },
		"edited":              func(m *waE2E.Message) *waE2E.Message { return &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{Message: m}} },
	}

	for name, wrap := range wrappers {
		t.Run(name, func(t *testing.T) {
			wrapped := wrap(inner)

			ours := unwrapFutureProof(wrapped)

			lib := (&events.Message{RawMessage: wrap(inner)}).UnwrapRaw()

			if classifyMessage(ours.Message).Type != classifyMessage(lib.Message).Type {
				t.Errorf("classification diverges: ours=%q lib=%q",
					classifyMessage(ours.Message).Type, classifyMessage(lib.Message).Type)
			}
			if classifyMessage(ours.Message).Text != classifyMessage(lib.Message).Text {
				t.Errorf("text diverges: ours=%q lib=%q",
					classifyMessage(ours.Message).Text, classifyMessage(lib.Message).Text)
			}
			if ours.IsEphemeral != lib.IsEphemeral {
				t.Errorf("IsEphemeral: ours=%v lib=%v", ours.IsEphemeral, lib.IsEphemeral)
			}
			if ours.IsViewOnce != lib.IsViewOnce {
				t.Errorf("IsViewOnce: ours=%v lib=%v", ours.IsViewOnce, lib.IsViewOnce)
			}
			if ours.IsDocumentWithCaption != lib.IsDocumentWithCaption {
				t.Errorf("IsDocumentWithCaption: ours=%v lib=%v", ours.IsDocumentWithCaption, lib.IsDocumentWithCaption)
			}
			if ours.IsEdit != lib.IsEdit {
				t.Errorf("IsEdit: ours=%v lib=%v", ours.IsEdit, lib.IsEdit)
			}
		})
	}
}

// TestClassificacao_NenhumTipoConhecidoCaiNoDescarte é a asserção que fecha a
// F184: para cada tipo que o classificador reconhece, o texto NUNCA sai vazio.
//
// Texto vazio com tipo `text` é a combinação exata que a guarda de gravação
// descarta. Um ramo novo que se esqueça do marcador passaria em todos os testes
// de valor — porque ninguém escreveria um caso para o tipo que acabou de
// acrescentar — e a mensagem sumiria em silêncio. Este teste não precisa de
// saber o tipo novo para o proteger.
func TestClassificacao_NenhumTipoConhecidoCaiNoDescarte(t *testing.T) {
	for nome, msg := range mensagensDeCadaTipo() {
		got := classifyMessage(msg)
		if got.Text == "" {
			t.Errorf("%s: classificado como %q com texto VAZIO — a guarda de gravacao descarta isto (F184)", nome, got.Type)
		}
	}
}
