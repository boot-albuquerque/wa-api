package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-noise/protocol/proto/waSyncAction"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/pkg/infra/db"
)

// F191. Os três eventos de etiqueta chegavam e eram deitados fora: nem
// gravados, nem despachados. Quem punha uma etiqueta no telemóvel não a via na
// API.
//
// O que se trava aqui é o PAR (webhook, persistência). Um sem o outro é meia
// capacidade: o webhook sozinho obriga o cliente a manter o estado, e a
// gravação sozinha não avisa ninguém.

func labelHandler(t *testing.T) *UserEventHandler {
	t.Helper()
	return &UserEventHandler{UserID: "u-label", DB: discardTestDB(t)}
}

func boolPtr(b bool) *bool    { return &b }
func i32Ptr(i int32) *int32   { return &i }
func strPtr(s string) *string { return &s }

func TestLabelEdit_GravaEDespacha(t *testing.T) {
	evh := labelHandler(t)
	st := &eventState{postmap: map[string]any{}}
	agora := time.Now().UTC().Truncate(time.Second)

	evh.handleLabelEdit(&events.LabelEdit{
		LabelID:   "L1",
		Timestamp: agora,
		Action: &waSyncAction.LabelEditAction{
			Name: strPtr("Clientes"), Color: i32Ptr(3), Deleted: boolPtr(false),
		},
	}, st)

	if st.dowebhook != 1 {
		t.Fatalf("dowebhook = %d, quero 1 — o evento não notifica ninguém (F191)", st.dowebhook)
	}
	if st.postmap["label_id"] != "L1" || st.postmap["name"] != "Clientes" {
		t.Fatalf("postmap = %+v", st.postmap)
	}

	rotulos, err := db.NewLabelRepository(evh.DB).ListLabels(context.Background(), "u-label")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(rotulos) != 1 {
		t.Fatalf("etiquetas gravadas = %d, quero 1 — o evento não foi persistido (F191)", len(rotulos))
	}
	if rotulos[0].Name != "Clientes" || rotulos[0].Color != 3 {
		t.Fatalf("etiqueta = %+v", rotulos[0])
	}
}

// TestLabelEdit_ApagadaSaiDaListagem: a linha FICA na tabela (o registo de que
// existiu tem valor para quem reconstrói histórico) mas não sai na listagem,
// que responde "o que existe hoje".
func TestLabelEdit_ApagadaSaiDaListagem(t *testing.T) {
	evh := labelHandler(t)
	agora := time.Now().UTC()

	evh.handleLabelEdit(&events.LabelEdit{
		LabelID: "L1", Timestamp: agora,
		Action: &waSyncAction.LabelEditAction{Name: strPtr("Antiga"), Deleted: boolPtr(false)},
	}, &eventState{postmap: map[string]any{}})

	evh.handleLabelEdit(&events.LabelEdit{
		LabelID: "L1", Timestamp: agora.Add(time.Second),
		Action: &waSyncAction.LabelEditAction{Name: strPtr("Antiga"), Deleted: boolPtr(true)},
	}, &eventState{postmap: map[string]any{}})

	rotulos, err := db.NewLabelRepository(evh.DB).ListLabels(context.Background(), "u-label")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(rotulos) != 0 {
		t.Fatalf("etiqueta apagada continua na listagem: %+v", rotulos)
	}
}

func TestLabelAssociationChat_GravaEDesmarca(t *testing.T) {
	evh := labelHandler(t)
	repo := db.NewLabelRepository(evh.DB)
	chat := types.NewJID("5511999999999", types.DefaultUserServer)
	agora := time.Now().UTC()

	evh.handleLabelAssociationChat(&events.LabelAssociationChat{
		LabelID: "L1", JID: chat, Timestamp: agora,
		Action: &waSyncAction.LabelAssociationAction{Labeled: boolPtr(true)},
	}, &eventState{postmap: map[string]any{}})

	conversas, err := repo.ListChatsForLabel(context.Background(), "u-label", "L1")
	if err != nil {
		t.Fatalf("ListChatsForLabel: %v", err)
	}
	if len(conversas) != 1 || conversas[0].ChatJID != chat.String() {
		t.Fatalf("conversas = %+v, quero uma com %s", conversas, chat)
	}

	// Desmarcar: a linha continua, mas sai da listagem. Apagá-la perderia o
	// instante em que a etiqueta foi retirada — o que distingue "nunca teve"
	// de "tinha e tiraram".
	evh.handleLabelAssociationChat(&events.LabelAssociationChat{
		LabelID: "L1", JID: chat, Timestamp: agora.Add(time.Second),
		Action: &waSyncAction.LabelAssociationAction{Labeled: boolPtr(false)},
	}, &eventState{postmap: map[string]any{}})

	conversas, err = repo.ListChatsForLabel(context.Background(), "u-label", "L1")
	if err != nil {
		t.Fatalf("ListChatsForLabel: %v", err)
	}
	if len(conversas) != 0 {
		t.Fatalf("conversa desmarcada continua na listagem: %+v", conversas)
	}
}

// TestLabel_FromFullSyncTambemGrava: a sincronização completa é justamente
// como se recupera o estado anterior ao pareamento. Filtrá-la deixaria a
// tabela vazia até alguém mexer numa etiqueta.
func TestLabel_FromFullSyncTambemGrava(t *testing.T) {
	evh := labelHandler(t)

	evh.handleLabelEdit(&events.LabelEdit{
		LabelID: "L9", Timestamp: time.Now().UTC(), FromFullSync: true,
		Action: &waSyncAction.LabelEditAction{Name: strPtr("Do sync")},
	}, &eventState{postmap: map[string]any{}})

	rotulos, _ := db.NewLabelRepository(evh.DB).ListLabels(context.Background(), "u-label")
	if len(rotulos) != 1 {
		t.Fatalf("etiqueta de fullSync foi descartada: %+v", rotulos)
	}
}

func TestLabelAssociationMessage_GravaEDespacha(t *testing.T) {
	evh := labelHandler(t)
	st := &eventState{postmap: map[string]any{}}
	chat := types.NewJID("5511999999999", types.DefaultUserServer)

	evh.handleLabelAssociationMessage(&events.LabelAssociationMessage{
		LabelID: "L1", JID: chat, MessageID: "MSG-7", Timestamp: time.Now().UTC(),
		Action: &waSyncAction.LabelAssociationAction{Labeled: boolPtr(true)},
	}, st)

	if st.dowebhook != 1 {
		t.Fatalf("dowebhook = %d, quero 1 (F191)", st.dowebhook)
	}
	if st.postmap["message_id"] != "MSG-7" || st.postmap["labeled"] != true {
		t.Fatalf("postmap = %+v", st.postmap)
	}

	var n int
	if err := evh.DB.Get(&n,
		`SELECT COUNT(*) FROM wa_label_messages WHERE user_id=? AND label_id=? AND message_id=? AND labeled=?`,
		"u-label", "L1", "MSG-7", true); err != nil {
		t.Fatalf("consulta: %v", err)
	}
	if n != 1 {
		t.Fatalf("linhas gravadas = %d, quero 1 — a etiqueta da MENSAGEM não foi persistida", n)
	}
}

// TestHandleAppStateChange_EncaminhaOsCinco trava o agrupamento criado para
// baixar a complexidade do switch principal: um tipo que chegue aqui sem ramo
// é falha silenciosa de encaminhamento, e o único sinal seria o webhook que
// não sai.
func TestHandleAppStateChange_EncaminhaOsCinco(t *testing.T) {
	chat := types.NewJID("5511999999999", types.DefaultUserServer)
	casos := []struct {
		nome  string
		evt   any
		quero string
	}{
		{"push_name", &events.PushName{JID: chat, NewPushName: "N"}, "PushName"},
		{"business_name", &events.BusinessName{JID: chat, NewBusinessName: "B"}, "BusinessName"},
		{"label_edit", &events.LabelEdit{LabelID: "L1", Timestamp: time.Now().UTC()}, "LabelEdit"},
		{"label_chat", &events.LabelAssociationChat{
			LabelID: "L1", JID: chat, Timestamp: time.Now().UTC(),
			Action: &waSyncAction.LabelAssociationAction{Labeled: boolPtr(true)},
		}, "LabelAssociationChat"},
		{"label_message", &events.LabelAssociationMessage{
			LabelID: "L1", JID: chat, MessageID: "M1", Timestamp: time.Now().UTC(),
			Action: &waSyncAction.LabelAssociationAction{Labeled: boolPtr(true)},
		}, "LabelAssociationMessage"},
	}
	if len(casos) != 5 {
		t.Fatalf("o grupo tem cinco tipos, a tabela tem %d", len(casos))
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			evh := labelHandler(t)
			st := &eventState{postmap: map[string]any{}}

			evh.handleAppStateChange(c.evt, st)

			if st.postmap["type"] != c.quero {
				t.Fatalf("type = %v, quero %q — o encaminhamento não chegou ao handler", st.postmap["type"], c.quero)
			}
			if st.dowebhook != 1 {
				t.Fatalf("dowebhook = %d, quero 1", st.dowebhook)
			}
		})
	}
}

// TestHandleAppStateChange_TipoSemRamoDeixaRastro: o `default` é inalcançável
// hoje, e é isso que o torna perigoso — um tipo novo acrescentado ao switch de
// cima e esquecido aqui sairia sem webhook e sem sinal.
func TestHandleAppStateChange_TipoSemRamoDeixaRastro(t *testing.T) {
	evh := labelHandler(t)
	buf := capturarLog(t)
	st := &eventState{postmap: map[string]any{}}

	evh.handleAppStateChange(&events.Picture{}, st)

	if st.dowebhook != 0 {
		t.Fatalf("dowebhook = %d: um tipo sem ramo não pode gerar webhook", st.dowebhook)
	}
	if !strings.Contains(buf.String(), "app-state change routed here without a branch") {
		t.Fatalf("tipo sem ramo passou sem registo: %s", buf.String())
	}
}
