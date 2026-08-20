package bootstrap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"

	waCommon "wa-api/internal/wa-noise/protocol/proto/waCommon"
	waE2E "wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/pkg/infra/db"
)

// F184. Enquete e mensagem com botões chegam, são logadas como recebidas, e
// nunca alcançam a tabela de histórico: a cadeia de classificação de
// saveMessageHistory não tem ramo para elas, o tipo fica "text", o texto fica
// vazio, e a guarda de gravação descarta.
//
// O que se trava aqui é o REGISTRO do descarte — o descarte em si continua a
// acontecer, porque acrescentar os ramos em falta é a etapa (b) do canal e
// mexe no que é gravado.
//
// A ARMADILHA deste teste, e a razão de o nível ser asserido explicitamente:
// captureLogInto liga um zerolog SEM filtro de nível, logo ele vê até Debug
// (está dito em eventhandler_qr_test.go:23). O registro que existia antes já
// era um Debug, e um teste que só procurasse o texto passaria com o defeito
// no lugar — Debug é invisível no nível de produção, que é exatamente por que
// a F184 sobreviveu. Sem a asserção de "level":"warn", este teste não morde.

func discardTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("sqlite", t.TempDir()+"/discard.db"+db.SQLitePragmas)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	if err := db.InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return database
}

// handlerComHistorico devolve um UserEventHandler com histórico LIGADO. O
// limite vem do cache porque é de lá que saveMessageHistory o lê primeiro.
func handlerComHistorico(t *testing.T, userID string) *UserEventHandler {
	t.Helper()
	if appCtx.UserInfoCache == nil {
		appCtx.UserInfoCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	}
	appCtx.UserInfoCache.Set(userID, *userValues(userID, 50), cache.NoExpiration)
	t.Cleanup(func() { appCtx.UserInfoCache.Delete(userID) })
	return &UserEventHandler{UserID: userID, DB: discardTestDB(t)}
}

// eventoNaoClassificavel é uma mensagem que nenhum ramo da cadeia reconhece —
// a forma que uma enquete e uma mensagem interativa têm hoje, do ponto de vista
// da classificação. `wire_type` é o que o servidor do WhatsApp disse que ela
// era, e é o campo que denuncia o defeito: ele diz "poll" enquanto a nossa
// classificação diz "text".
func eventoNaoClassificavel(id, wireType string) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			ID:   id,
			Type: wireType,
			MessageSource: types.MessageSource{
				Chat:   types.NewJID("5511999999999", types.DefaultUserServer),
				Sender: types.NewJID("5511888888888", types.DefaultUserServer),
			},
		},
		Message: &waE2E.Message{},
	}
}

func TestHistorico_DescarteDeixaRastro(t *testing.T) {
	evh := handlerComHistorico(t, "u-descarte")
	buf := capturarLog(t)

	evh.saveMessageHistory(eventoNaoClassificavel("MSG-POLL", "poll"), &eventState{postmap: map[string]any{}})

	saida := buf.String()
	if saida == "" {
		t.Fatal("mensagem recebida descartada sem registro nenhum (F184)")
	}

	linha := linhaComMensagem(t, saida, "dropped from history")
	if linha == nil {
		t.Fatalf("nenhum registro de descarte: %s", saida)
	}

	// O NÍVEL é a metade da correção que um teste de texto não pega. Debug é
	// invisível em produção, e foi assim que a F184 passou despercebida.
	if got := linha["level"]; got != "warn" {
		t.Errorf("descarte registrado em nível %v, quero warn: em Debug o operador nunca o vê", got)
	}

	// Os três campos que o canal exigiu, mais o wire_type que nomeia a causa.
	for campo, quero := range map[string]string{
		"message_id":   "MSG-POLL",
		"wire_type":    "poll",
		"message_type": "text",
		"reason":       discardReasonUnclassified,
	} {
		got, ok := linha[campo]
		if !ok {
			t.Errorf("campo %q ausente do registro de descarte: %s", campo, saida)
			continue
		}
		if got != quero {
			t.Errorf("campo %q = %v, quero %q", campo, got, quero)
		}
	}
}

// TestHistorico_MensagemGravadaNaoViraRuido é o controle positivo, e é a
// Armadilha 2 aplicada: sem ele, um aviso emitido em TODA mensagem passaria no
// teste acima e transformaria a correção no defeito da F180 — um diagnóstico
// que dispara no caminho normal deixa de ser lido.
func TestHistorico_MensagemGravadaNaoViraRuido(t *testing.T) {
	evh := handlerComHistorico(t, "u-normal")
	buf := capturarLog(t)

	evt := eventoNaoClassificavel("MSG-TEXTO", "text")
	evt.Message = &waE2E.Message{Conversation: proto("oi")}
	gravar(evh, evt)

	if saida := buf.String(); strings.Contains(saida, "dropped from history") {
		t.Fatalf("mensagem COM conteudo produziu aviso de descarte: %s", saida)
	}

	var n int
	if err := evh.DB.Get(&n, "SELECT COUNT(*) FROM message_history WHERE message_id = 'MSG-TEXTO'"); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 1 {
		t.Fatalf("mensagem com conteudo gravou %d linha(s), quero 1", n)
	}
}

func proto(s string) *string { return &s }

// linhaComMensagem devolve o primeiro registro JSON cuja "message" contenha
// trecho. Decodificar em vez de casar substring é o que permite asserir o
// NÍVEL e os campos separadamente, sem depender da ordem em que zerolog os
// serializa.
func linhaComMensagem(t *testing.T, saida, trecho string) map[string]any {
	t.Helper()
	for _, linha := range strings.Split(strings.TrimSpace(saida), "\n") {
		if linha == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(linha), &m); err != nil {
			continue
		}
		if msg, ok := m["message"].(string); ok && strings.Contains(msg, trecho) {
			return m
		}
	}
	return nil
}

// --- F184, etapa (b): os ramos que faltavam ---------------------------------

// TestHistorico_EnqueteGravaComAPergunta trava o defeito medido em campo: a
// enquete `3EB06ABF289C9D46888E88` foi enviada, recebida, logada, e a contagem
// na tabela deu ZERO.
//
// A asserção é sobre a PERGUNTA, não sobre o tipo. Gravar a enquete como
// ":poll:" faria o teste do tipo passar com metade do defeito no lugar — e essa
// metade é justamente a que a F187 explica: escrever em textContent em vez de
// caption seria apagado pelo bloco de extração, e o resultado seria ":poll:".
func TestHistorico_EnqueteGravaComAPergunta(t *testing.T) {
	evh := handlerComHistorico(t, "u-poll")
	buf := capturarLog(t)

	pergunta := "Qual o melhor dia?"
	evt := eventoNaoClassificavel("MSG-POLL-B", "poll")
	evt.Message = &waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{
		Name: proto(pergunta),
		Options: []*waE2E.PollCreationMessage_Option{
			{OptionName: proto("Segunda")}, {OptionName: proto("Terca")},
		},
	}}

	gravar(evh, evt)

	if saida := buf.String(); strings.Contains(saida, "dropped from history") {
		t.Fatalf("enquete continua a ser descartada: %s", saida)
	}

	tipo, txt := lerLinha(t, evh, "MSG-POLL-B")
	if tipo != "poll" {
		t.Errorf("message_type = %q, quero \"poll\"", tipo)
	}
	if txt != pergunta {
		t.Errorf("text_content = %q, quero a pergunta %q (\":poll:\" aqui significa que o texto foi escrito em textContent e o bloco de extracao o apagou — F187)", txt, pergunta)
	}
}

// TestHistorico_BotoesGravaComOCorpo cobre o formato que o NOSSO
// /chat/send/buttons produz de facto: waE2E.Message{InteractiveMessage},
// montado em messenger_buttons.go:217. Foi verificado no adapter antes de o
// ramo ser escrito — um ramo de recepção que não case com o emissor real é a
// Armadilha 1 na direção oposta.
func TestHistorico_BotoesGravaComOCorpo(t *testing.T) {
	evh := handlerComHistorico(t, "u-btn")
	corpo := "Escolha uma opcao"

	evt := eventoNaoClassificavel("MSG-BTN-B", "text")
	evt.Message = &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{Text: proto(corpo)},
	}}

	gravar(evh, evt)

	tipo, txt := lerLinha(t, evh, "MSG-BTN-B")
	if tipo != "buttons" {
		t.Errorf("message_type = %q, quero \"buttons\"", tipo)
	}
	if txt != corpo {
		t.Errorf("text_content = %q, quero %q", txt, corpo)
	}
}

// TestHistorico_BotoesLegadoTambemGrava cobre o formato ButtonsMessage, que nós
// NÃO enviamos mas que pode chegar de outro cliente. Sem ele, o teste de campo
// passaria com os nossos próprios envios e continuaria a perder os de terceiros.
func TestHistorico_BotoesLegadoTambemGrava(t *testing.T) {
	evh := handlerComHistorico(t, "u-btn-legado")
	corpo := "Corpo legado"

	evt := eventoNaoClassificavel("MSG-BTN-LEG", "text")
	evt.Message = &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{
		ContentText: proto(corpo),
	}}

	gravar(evh, evt)

	tipo, txt := lerLinha(t, evh, "MSG-BTN-LEG")
	if tipo != "buttons" {
		t.Errorf("message_type = %q, quero \"buttons\"", tipo)
	}
	if txt != corpo {
		t.Errorf("text_content = %q, quero %q", txt, corpo)
	}
}

// TestHistorico_EnqueteSemPerguntaCaiNoPlaceholder é a fronteira: sem nome, o
// ramo não pode voltar a produzir conteúdo vazio, senão a guarda descarta e o
// defeito volta pela porta dos fundos.
func TestHistorico_EnqueteSemPerguntaCaiNoPlaceholder(t *testing.T) {
	evh := handlerComHistorico(t, "u-poll-vazia")
	buf := capturarLog(t)

	evt := eventoNaoClassificavel("MSG-POLL-VAZIA", "poll")
	evt.Message = &waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{}}

	gravar(evh, evt)

	if saida := buf.String(); strings.Contains(saida, "dropped from history") {
		t.Fatalf("enquete sem pergunta voltou a ser descartada: %s", saida)
	}
	if _, txt := lerLinha(t, evh, "MSG-POLL-VAZIA"); txt != ":poll:" {
		t.Errorf("text_content = %q, quero \":poll:\"", txt)
	}
}

func lerLinha(t *testing.T, evh *UserEventHandler, id string) (tipo, texto string) {
	t.Helper()
	err := evh.DB.QueryRow(
		"SELECT message_type, text_content FROM message_history WHERE message_id = ?", id,
	).Scan(&tipo, &texto)
	if err != nil {
		t.Fatalf("linha %s nao gravada: %v", id, err)
	}
	return tipo, texto
}

// --- template e list: a etapa (a) da DECISÃO 20 -------------------------------

// TestHistorico_ListaEmbrulhadaGrava parte da forma que o NOSSO adapter
// constrói — a lista dentro de DocumentWithCaptionMessage
// (messenger_list.go:116) — e verifica que ela chega ao histórico.
//
// O que ele prova mudou depois de eu medir, e o nome ficou porque a ENTRADA
// continua a ser a lista embrulhada. O que ele NÃO prova é que exista um
// desembrulho nosso: UnwrapRaw já tratou o invólucro antes de o evento nos
// chegar, e o ramo que dispara é o `GetListMessage()` do topo.
//
// A primeira versão deste teste montava o evento à mão, sem UnwrapRaw, e por
// isso exercitava um ajudante que desembrulhava — código que a produção nunca
// executa. Passava, com controlo negativo e tudo, num caminho imaginário
// (HOUSEKEEP F188). Agora passa pelo `gravar`, que refaz o caminho real.
func TestHistorico_ListaEmbrulhadaGrava(t *testing.T) {
	evh := handlerComHistorico(t, "u-list")
	buf := capturarLog(t)

	evt := eventoNaoClassificavel("MSG-LIST", "media") // wire_type medido em campo
	evt.Message = &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{ListMessage: &waE2E.ListMessage{
				Title:       proto("Menu do dia"),
				Description: proto("escolha"),
				ButtonText:  proto("Ver"),
			}},
		},
	}

	gravar(evh, evt)

	if saida := buf.String(); strings.Contains(saida, "dropped from history") {
		t.Fatalf("lista embrulhada continua a ser descartada: %s", saida)
	}
	tipo, txt := lerLinha(t, evh, "MSG-LIST")
	if tipo != "list" {
		t.Errorf("message_type = %q, quero \"list\"", tipo)
	}
	if txt != "Menu do dia" {
		t.Errorf("text_content = %q, quero \"Menu do dia\"", txt)
	}
}

// TestHistorico_ListaNoTopoTambemGrava cobre a lista NÃO embrulhada, que outro
// cliente pode enviar. O ramo existe para o que chega, não só para o que sai.
func TestHistorico_ListaNoTopoTambemGrava(t *testing.T) {
	evh := handlerComHistorico(t, "u-list-topo")

	evt := eventoNaoClassificavel("MSG-LIST-TOPO", "text")
	evt.Message = &waE2E.Message{ListMessage: &waE2E.ListMessage{
		Description: proto("so descricao"),
	}}

	gravar(evh, evt)

	tipo, txt := lerLinha(t, evh, "MSG-LIST-TOPO")
	if tipo != "list" {
		t.Errorf("message_type = %q, quero \"list\"", tipo)
	}
	// Sem título, cai na descrição — a precedência do listText.
	if txt != "so descricao" {
		t.Errorf("text_content = %q, quero \"so descricao\"", txt)
	}
}

// TestHistorico_TemplateGrava cobre o TemplateMessage, que o nosso
// /chat/send/template envia SEM invólucro (messenger.go:633) — a assimetria com
// a lista está verificada no adapter, não suposta.
func TestHistorico_TemplateGrava(t *testing.T) {
	evh := handlerComHistorico(t, "u-tpl")

	evt := eventoNaoClassificavel("MSG-TPL", "text")
	evt.Message = &waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{
		HydratedTemplate: &waE2E.TemplateMessage_HydratedFourRowTemplate{
			// O título é um oneof no proto, não um campo simples — por isso
			// vai embrulhado. Descoberto pelo compilador, e vale a nota: o
			// getter GetHydratedTitleText() esconde o oneof, então a leitura
			// do código de produção não denuncia a forma da escrita.
			Title: &waE2E.TemplateMessage_HydratedFourRowTemplate_HydratedTitleText{
				HydratedTitleText: "Titulo do template",
			},
			HydratedContentText: proto("corpo"),
		},
	}}

	gravar(evh, evt)

	tipo, txt := lerLinha(t, evh, "MSG-TPL")
	if tipo != "template" {
		t.Errorf("message_type = %q, quero \"template\"", tipo)
	}
	if txt != "Titulo do template" {
		t.Errorf("text_content = %q, quero o titulo", txt)
	}
}

// TestHistorico_TemplateSemTituloUsaOCorpo trava a precedência do templateText.
// Sem este teste, devolver sempre o corpo passaria no teste acima se o título
// fosse ignorado — e a precedência é o que decide o que o operador lê na lista.
func TestHistorico_TemplateSemTituloUsaOCorpo(t *testing.T) {
	evh := handlerComHistorico(t, "u-tpl-sem-titulo")

	evt := eventoNaoClassificavel("MSG-TPL-2", "text")
	evt.Message = &waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{
		HydratedTemplate: &waE2E.TemplateMessage_HydratedFourRowTemplate{
			HydratedContentText: proto("so corpo"),
		},
	}}

	gravar(evh, evt)

	if _, txt := lerLinha(t, evh, "MSG-TPL-2"); txt != "so corpo" {
		t.Errorf("text_content = %q, quero \"so corpo\"", txt)
	}
}

// TestHistorico_InvolucroSemListaNaoViraLista guarda a fronteira do invólucro:
// DocumentWithCaptionMessage é GENÉRICO, e nem tudo o que vem dentro dele é
// lista.
//
// Nasceu como controlo negativo de um ajudante que já não existe, e sobrevive
// porque a pergunta continua a valer depois do UnwrapRaw: uma conversa que
// viajou dentro do invólucro tem de ser classificada como TEXTO, não como
// lista. Se alguém voltar a escrever um desembrulho manual — e a tentação
// existe, porque o nome do campo sugere documento — este teste morde.
func TestHistorico_InvolucroSemListaNaoViraLista(t *testing.T) {
	evh := handlerComHistorico(t, "u-inv")

	evt := eventoNaoClassificavel("MSG-INV", "media")
	evt.Message = &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{Conversation: proto("nao sou lista")},
		},
	}

	gravar(evh, evt)

	var tipo string
	err := evh.DB.QueryRow("SELECT message_type FROM message_history WHERE message_id = 'MSG-INV'").Scan(&tipo)
	if err == nil && tipo == "list" {
		t.Fatal("invólucro SEM lista foi classificado como lista: listMessageInside devolve não-nil a mais")
	}
}

// --- edição: a única perda REAL da F188 ---------------------------------------

// TestHistorico_EdicaoGravaComOTextoNovo trava o defeito com a causa CERTA.
//
// A edição não se perdia por vir embrulhada — UnwrapRaw desembrulha-a. Perdia-se
// porque, depois do desembrulho, ela é uma ProtocolMessage, e a cadeia só
// reconhecia GetType() == 0 (REVOKE, o apagar). MESSAGE_EDIT é 14.
//
// Este teste passa pelo `gravar`, ou seja, pelo desembrulho real — se montasse
// a ProtocolMessage à mão no topo, provaria um caminho que a produção não tem.
func TestHistorico_EdicaoGravaComOTextoNovo(t *testing.T) {
	evh := handlerComHistorico(t, "u-edit")
	buf := capturarLog(t)

	evt := eventoNaoClassificavel("MSG-EDIT", "text")
	evt.Message = &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key:           &waCommon.MessageKey{ID: proto("MSG-ORIGINAL")},
			EditedMessage: &waE2E.Message{Conversation: proto("texto EDITADO")},
		}},
	}}
	gravar(evh, evt)

	if saida := buf.String(); strings.Contains(saida, "dropped from history") {
		t.Fatalf("edicao continua a ser descartada: %s", saida)
	}
	tipo, txt := lerLinha(t, evh, "MSG-EDIT")
	if tipo != "edit" {
		t.Errorf("message_type = %q, quero \"edit\"", tipo)
	}
	if txt != "texto EDITADO" {
		t.Errorf("text_content = %q, quero o texto novo", txt)
	}
}

// TestHistorico_EdicaoLigaAMensagemOriginal é o que separa uma linha ÚTIL de uma
// linha solta. Sem a ligação, o cliente vê uma edição e não sabe o que ela edita
// — o que é quase tão inútil como não a ter.
func TestHistorico_EdicaoLigaAMensagemOriginal(t *testing.T) {
	evh := handlerComHistorico(t, "u-edit-key")

	evt := eventoNaoClassificavel("MSG-EDIT-2", "text")
	evt.Message = &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key:           &waCommon.MessageKey{ID: proto("MSG-ORIGINAL-2")},
			EditedMessage: &waE2E.Message{Conversation: proto("novo")},
		}},
	}}
	gravar(evh, evt)

	var quoted string
	if err := evh.DB.QueryRow(
		"SELECT COALESCE(quoted_message_id,'') FROM message_history WHERE message_id = 'MSG-EDIT-2'",
	).Scan(&quoted); err != nil {
		t.Fatalf("linha nao gravada: %v", err)
	}
	if quoted != "MSG-ORIGINAL-2" {
		t.Errorf("quoted_message_id = %q, quero \"MSG-ORIGINAL-2\": sem a ligacao a edicao e' uma linha solta", quoted)
	}
}

// TestHistorico_ApagarContinuaAApagar é o controle que a edição obriga: os dois
// casos são ProtocolMessage e passam pelo MESMO if. Trocar a ordem dos ramos, ou
// alargar a condição do apagar, faria uma edição ser gravada como delete — o que
// seria trocar perda silenciosa por CORRUPÇÃO silenciosa, estritamente pior.
func TestHistorico_ApagarContinuaAApagar(t *testing.T) {
	evh := handlerComHistorico(t, "u-del")

	evt := eventoNaoClassificavel("MSG-DEL", "text")
	evt.Message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto("MSG-APAGADA")},
	}}
	gravar(evh, evt)

	tipo, txt := lerLinha(t, evh, "MSG-DEL")
	if tipo != "delete" {
		t.Fatalf("message_type = %q, quero \"delete\": a edicao roubou o ramo do apagar", tipo)
	}
	if txt != "MSG-APAGADA" {
		t.Errorf("text_content = %q, quero o id da mensagem apagada", txt)
	}
}

// gravar entrega o evento ao histórico PELO CAMINHO DA PRODUÇÃO: o que o teste
// montou em Message passa por RawMessage e por UnwrapRaw antes de chegar ao
// classificador.
//
// Isto não é cerimónia, é a correção de um defeito medido. UnwrapRaw
// (events/message.go:119-162) desembrulha NOVE invólucros — deviceSent,
// ephemeral, as três variantes de viewOnce, lottieSticker, documentWithCaption,
// botInvoke e edited — antes de o evento nos chegar. Um teste que atribua
// Message à mão vê uma forma que a produção NUNCA vê.
//
// A primeira versão destes testes fazia exatamente isso, e o custo está na
// HOUSEKEEP F188: abençoou um ramo de código morto que desembrulhava listas,
// com controlo negativo e tudo — mordendo num caminho imaginário. É a Armadilha
// 1, o dublê que diverge da produção, numa forma difícil de ver: o dublê era
// mais SIMPLES que a produção, não mais permissivo.
func gravar(evh *UserEventHandler, evt *events.Message) {
	evt.RawMessage = evt.Message
	evh.saveMessageHistory(evt.UnwrapRaw(), &eventState{postmap: map[string]any{}})
}
