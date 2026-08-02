package history

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	_ "modernc.org/sqlite"

	"wa-api/pkg/infra/db"
)

// A costura aqui e' a propria assinatura: SyncHistoryForChat recebe o *sqlx.DB e
// o SyncDeps, e SaveOutgoingMessageToHistory recebe as funcoes de persistencia.
// Nada disto precisa de socket. O que NAO da' para exercitar sem conexao real e'
// o retorno de sucesso de whatsmeow.Client.SendMessage — um *Client nil devolve
// ErrClientIsNil antes de qualquer I/O, o que cobre o ramo de erro, e o ramo de
// sucesso fica descoberto por construcao (esta' declarado no PR da fase).
//
// O schema vem de db.InitializeSchema, o mesmo de producao, sobre SQLite em
// t.TempDir() — sem CGO e sem Docker, como o resto do repositorio.

func openSQLite(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func newSyncDB(t *testing.T) *sqlx.DB {
	t.Helper()
	conn := openSQLite(t)
	if err := db.InitializeSchema(conn); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return conn
}

// asPostgres reaproveita a conexao SQLite declarando o driverName "postgres",
// que e' o que SyncHistoryForChat consulta para escolher o dialeto de
// placeholder. E' o unico jeito de provar que o ramo postgres monta a query com
// $1/$2 sem subir um Postgres de verdade.
func asPostgres(t *testing.T, conn *sqlx.DB) *sqlx.DB {
	t.Helper()
	return sqlx.NewDb(conn.DB, "postgres")
}

func insertMessage(t *testing.T, conn *sqlx.DB, userID, chatJID, senderJID, messageID string) {
	t.Helper()
	_, err := conn.Exec(`INSERT INTO message_history
		(user_id, chat_jid, sender_jid, message_id, message_type, text_content, timestamp)
		VALUES (?, ?, ?, ?, 'text', 'oi', CURRENT_TIMESTAMP)`,
		userID, chatJID, senderJID, messageID)
	if err != nil {
		t.Fatalf("inserir mensagem: %v", err)
	}
}

// mcWithClient implementa o waClientGetter anonimo esperado por SyncHistoryForChat.
type mcWithClient struct{ client *whatsmeow.Client }

func (m mcWithClient) GetWAClient() *whatsmeow.Client { return m.client }

// fakeSender implementa historySender sem socket: e' o que torna o ramo de
// sucesso (e o de pedido nao construido) alcancavel em teste.
type fakeSender struct {
	buildNil bool
	sendErr  error

	gotInfo  *types.MessageInfo
	gotCount int
	gotTo    types.JID
	gotPeer  bool
	sends    int
}

func (f *fakeSender) BuildHistorySyncRequest(info *types.MessageInfo, count int) *waE2E.Message {
	f.gotInfo = info
	f.gotCount = count
	if f.buildNil {
		return nil
	}
	return &waE2E.Message{}
}

func (f *fakeSender) SendMessage(_ context.Context, to types.JID, _ *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	f.sends++
	f.gotTo = to
	if len(extra) > 0 {
		f.gotPeer = extra[0].Peer
	}
	return whatsmeow.SendResponse{}, f.sendErr
}

func depsWith(wa interface{}, mc interface{}) SyncDeps {
	return SyncDeps{
		GetWA: func(string) interface{} { return wa },
		GetMC: func(string) interface{} { return mc },
	}
}

// clientWithStore devolve um *whatsmeow.Client cujo Store.ID esta' preenchido —
// o suficiente para passar da validacao e chegar no SendMessage.
func clientWithStore(t *testing.T) *whatsmeow.Client {
	t.Helper()
	jid, err := types.ParseJID("5511999999999:1@s.whatsapp.net")
	if err != nil {
		t.Fatalf("ParseJID: %v", err)
	}
	return &whatsmeow.Client{Store: &store.Device{ID: &jid}}
}

func TestSyncHistoryForChat_ErroDeConsultaPropagaCausa(t *testing.T) {
	conn := openSQLite(t) // sem schema: a tabela message_history nao existe
	chatJID, _ := types.ParseJID("5511888888888@s.whatsapp.net")

	err := SyncHistoryForChat(context.Background(), conn,
		depsWith(&fakeSender{}, mcWithClient{}), "u1", chatJID, 10)

	if err == nil {
		t.Fatal("esperava erro de leitura do historico")
	}
	if !strings.Contains(err.Error(), "failed to get last message from history") {
		t.Fatalf("erro = %v, quero o wrap de leitura do historico", err)
	}
	if !strings.Contains(err.Error(), "message_history") {
		t.Fatalf("erro = %v, quero preservar a causa do driver", err)
	}
}

func TestSyncHistoryForChat_DialetoPostgres(t *testing.T) {
	conn := asPostgres(t, newSyncDB(t))
	chatJID, _ := types.ParseJID("5511888888888@s.whatsapp.net")
	sender := &fakeSender{}

	err := SyncHistoryForChat(context.Background(), conn,
		depsWith(sender, mcWithClient{client: clientWithStore(t)}), "u1", chatJID, 10)

	if err != nil {
		t.Fatalf("SyncHistoryForChat: %v", err)
	}
	if sender.sends != 1 {
		t.Fatalf("envios = %d, quero 1", sender.sends)
	}
}

func TestSyncHistoryForChat_PedidoNaoConstruido(t *testing.T) {
	conn := newSyncDB(t)
	chatJID, _ := types.ParseJID("5511888888888@s.whatsapp.net")
	sender := &fakeSender{buildNil: true}

	err := SyncHistoryForChat(context.Background(), conn,
		depsWith(sender, mcWithClient{client: clientWithStore(t)}), "u1", chatJID, 10)

	if err == nil || !strings.Contains(err.Error(), "failed to build history sync request") {
		t.Fatalf("erro = %v, quero 'failed to build history sync request'", err)
	}
	if sender.sends != 0 {
		t.Fatal("nao deveria enviar sem pedido construido")
	}
}

func TestSyncHistoryForChat_EnvioComSucesso(t *testing.T) {
	conn := newSyncDB(t)
	chatJID, _ := types.ParseJID("5511888888888@s.whatsapp.net")
	insertMessage(t, conn, "u1", chatJID.String(), "5511777777777@s.whatsapp.net", "MSG-9")
	sender := &fakeSender{}
	self := clientWithStore(t)

	err := SyncHistoryForChat(context.Background(), conn,
		depsWith(sender, mcWithClient{client: self}), "u1", chatJID, 33)

	if err != nil {
		t.Fatalf("SyncHistoryForChat: %v", err)
	}
	if sender.gotCount != 33 {
		t.Fatalf("count = %d, quero 33", sender.gotCount)
	}
	if sender.gotInfo == nil || sender.gotInfo.ID != "MSG-9" {
		t.Fatalf("ancora = %+v, quero a ultima mensagem do historico", sender.gotInfo)
	}
	if sender.gotInfo.Chat != chatJID {
		t.Fatalf("chat da ancora = %v, quero %v", sender.gotInfo.Chat, chatJID)
	}
	if sender.gotTo != self.Store.ID.ToNonAD() {
		t.Fatalf("destino = %v, quero o proprio JID sem device", sender.gotTo)
	}
	if !sender.gotPeer {
		t.Fatal("o pedido de sync tem de ir como mensagem de peer")
	}
}

func TestSyncHistoryForChat_SemStoreDoCliente(t *testing.T) {
	tests := []struct {
		name string
		mc   interface{}
	}{
		{name: "GetWAClient devolve nil", mc: mcWithClient{}},
		{name: "cliente sem store", mc: mcWithClient{client: &whatsmeow.Client{}}},
		{name: "store sem ID", mc: mcWithClient{client: &whatsmeow.Client{Store: &store.Device{}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn := newSyncDB(t)
			chatJID, _ := types.ParseJID("5511888888888@s.whatsapp.net")

			err := SyncHistoryForChat(context.Background(), conn, depsWith(&fakeSender{}, tc.mc), "u1", chatJID, 10)

			if err == nil || !strings.Contains(err.Error(), "client store not available") {
				t.Fatalf("erro = %v, quero 'client store not available'", err)
			}
		})
	}
}

// anchor e' o resumo comparavel da ancora de sync que o pedido carrega.
type anchor struct {
	msgID   string
	sender  string
	isGroup bool
}

func jidText(jid types.JID) string {
	if jid.IsEmpty() {
		return ""
	}
	return jid.String()
}

func TestSyncHistoryForChat_MontaAncoraDoHistorico(t *testing.T) {
	tests := []struct {
		name       string
		chatJID    string
		seedRow    bool
		senderJID  string
		wantMsgID  string
		wantSender string
		wantGroup  bool
	}{
		{name: "sem historico anterior", chatJID: "5511888888888@s.whatsapp.net"},
		{name: "grupo sem historico", chatJID: "120363000000000000@g.us", wantGroup: true},
		{
			name: "com remetente identificado", chatJID: "5511888888888@s.whatsapp.net",
			seedRow: true, senderJID: "5511777777777@s.whatsapp.net",
			wantMsgID: "MSG-1", wantSender: "5511777777777@s.whatsapp.net",
		},
		{
			name: "com remetente proprio", chatJID: "5511888888888@s.whatsapp.net",
			seedRow: true, senderJID: "me", wantMsgID: "MSG-1",
		},
		{
			name: "com remetente vazio", chatJID: "5511888888888@s.whatsapp.net",
			seedRow: true, senderJID: "", wantMsgID: "MSG-1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn := newSyncDB(t)
			chatJID, err := types.ParseJID(tc.chatJID)
			if err != nil {
				t.Fatalf("ParseJID: %v", err)
			}
			if tc.seedRow {
				insertMessage(t, conn, "u1", tc.chatJID, tc.senderJID, "MSG-1")
			}
			sender := &fakeSender{}

			err = SyncHistoryForChat(context.Background(), conn,
				depsWith(sender, mcWithClient{client: clientWithStore(t)}), "u1", chatJID, 50)

			if err != nil {
				t.Fatalf("SyncHistoryForChat: %v", err)
			}
			got := anchor{
				msgID:   sender.gotInfo.ID,
				sender:  jidText(sender.gotInfo.Sender),
				isGroup: sender.gotInfo.IsGroup,
			}
			want := anchor{msgID: tc.wantMsgID, sender: tc.wantSender, isGroup: tc.wantGroup}
			if got != want {
				t.Fatalf("ancora = %+v, quero %+v", got, want)
			}
		})
	}
}

func TestSyncHistoryForChat_FalhaDeEnvioPropagaCausa(t *testing.T) {
	conn := newSyncDB(t)
	chatJID, _ := types.ParseJID("5511888888888@s.whatsapp.net")
	boom := errors.New("socket caiu")

	err := SyncHistoryForChat(context.Background(), conn,
		depsWith(&fakeSender{sendErr: boom}, mcWithClient{client: clientWithStore(t)}), "u1", chatJID, 10)

	if err == nil || !strings.Contains(err.Error(), "failed to send history sync request") {
		t.Fatalf("erro = %v, quero o wrap de envio", err)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("erro = %v, quero preservar a causa", err)
	}
}

// --- SaveOutgoingMessageToHistory -------------------------------------------

func TestSaveOutgoingMessageToHistory(t *testing.T) {
	errSave := errors.New("save falhou")
	errTrim := errors.New("trim falhou")

	tests := []struct {
		name      string
		limit     int
		saveErr   error
		trimErr   error
		wantSaves int
		wantTrims int
	}{
		{name: "limite zero nao persiste", limit: 0},
		{name: "limite negativo nao persiste", limit: -1},
		{name: "salva e apara", limit: 100, wantSaves: 1, wantTrims: 1},
		{name: "erro no save nao apara", limit: 100, saveErr: errSave, wantSaves: 1},
		{name: "erro no trim nao propaga", limit: 100, trimErr: errTrim, wantSaves: 1, wantTrims: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var saves, trims int
			var gotSender string
			var gotLimit int

			saveFn := func(_ *sqlx.DB, _, _, senderJID, _, _, _, _, _, _ string) error {
				saves++
				gotSender = senderJID
				return tc.saveErr
			}
			trimFn := func(_ *sqlx.DB, _, _ string, limit int) error {
				trims++
				gotLimit = limit
				return tc.trimErr
			}

			SaveOutgoingMessageToHistory(nil, saveFn, trimFn,
				"u1", "chat@s.whatsapp.net", "MSG-1", "text", "oi", "", tc.limit)

			if saves != tc.wantSaves || trims != tc.wantTrims {
				t.Fatalf("saves=%d trims=%d, quero %d/%d", saves, trims, tc.wantSaves, tc.wantTrims)
			}
			if tc.wantSaves > 0 && gotSender != "me" {
				t.Fatalf("sender = %q, quero \"me\" para mensagem de saida", gotSender)
			}
			if tc.wantTrims > 0 && gotLimit != tc.limit {
				t.Fatalf("limite repassado = %d, quero %d", gotLimit, tc.limit)
			}
		})
	}
}
