package bootstrap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"

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
	evh.saveMessageHistory(evt, &eventState{postmap: map[string]any{}})

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
