package db

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// Estes testes rodam contra um SQLite real com o schema de producao aplicado —
// a mesma via de migrations_test.go e user_repository_test.go. A Fase 8 previa
// testcontainers; nao foram usados aqui de proposito: o que estas funcoes
// decidem (idempotencia por ON CONFLICT, escopo do DELETE, preservacao de
// events) e' logica de consulta, nao comportamento especifico do Postgres, e
// exigir Docker no `make check` trocaria um gate que roda por um que e' pulado.
// O que continua descoberto e' o RAMO postgres de TrimMessageHistory, que usa
// OFFSET sem LIMIT -1 — registrado como pendencia, nao como coberto.

// newHistoryDB aplica o schema de producao e cria a tabela que o SDK do
// whatsmeow normalmente cria por conta propria — TrimMessageHistory apaga
// dela, entao ela precisa existir.
func newHistoryDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS whatsmeow_message_secrets (
		our_jid TEXT, chat_jid TEXT, sender_jid TEXT, message_id TEXT, key BLOB)`); err != nil {
		t.Fatalf("create whatsmeow_message_secrets: %v", err)
	}
	return db
}

// countHistory conta as linhas de um par (user, chat).
func countHistory(t *testing.T, db *sqlx.DB, userID, chatJID string) int {
	t.Helper()
	var n int
	if err := db.Get(&n,
		"SELECT COUNT(*) FROM message_history WHERE user_id = ? AND chat_jid = ?",
		userID, chatJID); err != nil {
		t.Fatalf("count history: %v", err)
	}
	return n
}

func TestSaveMessageToHistory_PersistsEveryColumn(t *testing.T) {
	db := newHistoryDB(t)

	err := SaveMessageToHistory(db, "u1", "chat@s.whatsapp.net", "sender@s.whatsapp.net",
		"MSG-1", "image", "legenda", "https://cdn.example.com/img.jpg", "QUOTED-1", `{"k":"v"}`)
	if err != nil {
		t.Fatalf("SaveMessageToHistory: %v", err)
	}

	var got struct {
		ChatJID   string `db:"chat_jid"`
		SenderJID string `db:"sender_jid"`
		Type      string `db:"message_type"`
		Text      string `db:"text_content"`
		MediaLink string `db:"media_link"`
		Quoted    string `db:"quoted_message_id"`
		DataJSON  string `db:"datajson"`
	}
	if err := db.Get(&got,
		"SELECT chat_jid, sender_jid, message_type, text_content, media_link, quoted_message_id, datajson "+
			"FROM message_history WHERE user_id = ? AND message_id = ?", "u1", "MSG-1"); err != nil {
		t.Fatalf("select: %v", err)
	}

	// media_link tem nome proprio aqui: e' o campo que a Fase 7 apontou como
	// perdido no caminho de HistorySync. Se a coluna sumir do INSERT, isto
	// falha antes de virar mensagem de midia sem link em producao.
	if got.MediaLink != "https://cdn.example.com/img.jpg" {
		t.Fatalf("media_link: got %q", got.MediaLink)
	}
	if got.ChatJID != "chat@s.whatsapp.net" || got.SenderJID != "sender@s.whatsapp.net" {
		t.Fatalf("jids: chat=%q sender=%q", got.ChatJID, got.SenderJID)
	}
	if got.Type != "image" || got.Text != "legenda" {
		t.Fatalf("tipo/texto: %q / %q", got.Type, got.Text)
	}
	if got.Quoted != "QUOTED-1" {
		t.Fatalf("quoted_message_id: got %q", got.Quoted)
	}
	if got.DataJSON != `{"k":"v"}` {
		t.Fatalf("datajson: got %q", got.DataJSON)
	}
}

// TestSaveMessageToHistory_IsIdempotentPerUserAndMessage trava a correcao de
// #292: reentregar a MESMA mensagem nao pode duplicar a linha, e nao pode
// virar erro (o chamador trata erro como falha de processamento).
func TestSaveMessageToHistory_IsIdempotentPerUserAndMessage(t *testing.T) {
	db := newHistoryDB(t)

	for i := 0; i < 3; i++ {
		if err := SaveMessageToHistory(db, "u1", "chat@s", "sender@s",
			"MSG-1", "text", "oi", "", "", "{}"); err != nil {
			t.Fatalf("insercao %d: %v", i, err)
		}
	}

	if n := countHistory(t, db, "u1", "chat@s"); n != 1 {
		t.Fatalf("3 entregas da mesma mensagem produziram %d linhas, queria 1", n)
	}
}

// TestSaveMessageToHistory_ConflictIsScopedToUser: o mesmo message_id vindo
// para DOIS usuarios sao duas mensagens distintas. Um ON CONFLICT so em
// message_id faria o segundo usuario perder a mensagem em silencio.
func TestSaveMessageToHistory_ConflictIsScopedToUser(t *testing.T) {
	db := newHistoryDB(t)

	if err := SaveMessageToHistory(db, "u1", "chat@s", "sender@s", "MSG-1", "text", "a", "", "", "{}"); err != nil {
		t.Fatalf("u1: %v", err)
	}
	if err := SaveMessageToHistory(db, "u2", "chat@s", "sender@s", "MSG-1", "text", "b", "", "", "{}"); err != nil {
		t.Fatalf("u2: %v", err)
	}

	if n := countHistory(t, db, "u1", "chat@s"); n != 1 {
		t.Fatalf("u1: %d linhas, queria 1", n)
	}
	if n := countHistory(t, db, "u2", "chat@s"); n != 1 {
		t.Fatalf("u2: %d linhas, queria 1 — o ON CONFLICT engoliu a mensagem de outro usuario", n)
	}
}

// seedMessages grava n mensagens com timestamps crescentes e devolve os ids
// na ordem em que foram gravadas (mais antiga primeiro).
func seedMessages(t *testing.T, db *sqlx.DB, userID, chatJID string, n int) []string {
	t.Helper()
	base := time.Now().Add(-time.Duration(n) * time.Hour)
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := chatJID + "-MSG-" + string(rune('A'+i))
		if _, err := db.Exec(
			`INSERT INTO message_history (user_id, chat_jid, sender_jid, message_id, timestamp,
			 message_type, text_content, media_link, quoted_message_id, datajson)
			 VALUES (?, ?, '', ?, ?, 'text', '', '', '', '{}')`,
			userID, chatJID, id, base.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	return ids
}

// TestTrimMessageHistory_KeepsNewestAndDropsOldest e a propriedade central:
// o que sobra sao as MAIS NOVAS. Trocar DESC por ASC na subconsulta inverte a
// retencao e apaga exatamente o que deveria ficar.
func TestTrimMessageHistory_KeepsNewestAndDropsOldest(t *testing.T) {
	db := newHistoryDB(t)
	ids := seedMessages(t, db, "u1", "chat@s", 5)

	if err := TrimMessageHistory(db, "u1", "chat@s", 2); err != nil {
		t.Fatalf("TrimMessageHistory: %v", err)
	}

	var restantes []string
	if err := db.Select(&restantes,
		"SELECT message_id FROM message_history WHERE user_id = ? AND chat_jid = ? ORDER BY timestamp",
		"u1", "chat@s"); err != nil {
		t.Fatalf("select restantes: %v", err)
	}

	if len(restantes) != 2 {
		t.Fatalf("sobraram %d mensagens, queria 2 (%v)", len(restantes), restantes)
	}
	// As duas ultimas semeadas sao as mais novas.
	if restantes[0] != ids[3] || restantes[1] != ids[4] {
		t.Fatalf("sobraram %v, queria as duas mais NOVAS %v — a retencao esta invertida",
			restantes, ids[3:])
	}
}

func TestTrimMessageHistory_LimitZeroClearsTheChat(t *testing.T) {
	db := newHistoryDB(t)
	seedMessages(t, db, "u1", "chat@s", 3)

	if err := TrimMessageHistory(db, "u1", "chat@s", 0); err != nil {
		t.Fatalf("TrimMessageHistory: %v", err)
	}

	if n := countHistory(t, db, "u1", "chat@s"); n != 0 {
		t.Fatalf("limite 0 deixou %d mensagens", n)
	}
}

func TestTrimMessageHistory_LimitAboveCountDeletesNothing(t *testing.T) {
	db := newHistoryDB(t)
	seedMessages(t, db, "u1", "chat@s", 3)

	if err := TrimMessageHistory(db, "u1", "chat@s", 100); err != nil {
		t.Fatalf("TrimMessageHistory: %v", err)
	}

	if n := countHistory(t, db, "u1", "chat@s"); n != 3 {
		t.Fatalf("limite acima do total apagou mensagens: sobraram %d de 3", n)
	}
}

// TestTrimMessageHistory_IsScopedToUserAndChat: sem o filtro, o trim de uma
// conversa apagaria historico de OUTRO usuario. E' a mutacao mais cara desta
// funcao e a que nenhum teste de contagem pega.
func TestTrimMessageHistory_IsScopedToUserAndChat(t *testing.T) {
	db := newHistoryDB(t)
	seedMessages(t, db, "u1", "chatA@s", 5)
	seedMessages(t, db, "u1", "chatB@s", 4)
	seedMessages(t, db, "u2", "chatA@s", 3)

	if err := TrimMessageHistory(db, "u1", "chatA@s", 1); err != nil {
		t.Fatalf("TrimMessageHistory: %v", err)
	}

	if n := countHistory(t, db, "u1", "chatA@s"); n != 1 {
		t.Fatalf("u1/chatA: sobraram %d, queria 1", n)
	}
	if n := countHistory(t, db, "u1", "chatB@s"); n != 4 {
		t.Fatalf("o trim vazou para outra conversa do mesmo usuario: u1/chatB tem %d, queria 4", n)
	}
	if n := countHistory(t, db, "u2", "chatA@s"); n != 3 {
		t.Fatalf("o trim vazou para OUTRO usuario: u2/chatA tem %d, queria 3", n)
	}
}

// TestTrimMessageHistory_AlsoTrimsMessageSecrets: as duas tabelas tem que
// andar juntas, senao whatsmeow_message_secrets cresce sem teto — o vazamento
// que a funcao existe para evitar.
func TestTrimMessageHistory_AlsoTrimsMessageSecrets(t *testing.T) {
	db := newHistoryDB(t)
	ids := seedMessages(t, db, "u1", "chat@s", 4)
	for _, id := range ids {
		if _, err := db.Exec(
			"INSERT INTO whatsmeow_message_secrets (our_jid, chat_jid, sender_jid, message_id, key) VALUES ('', 'chat@s', '', ?, x'00')",
			id); err != nil {
			t.Fatalf("seed secret: %v", err)
		}
	}

	if err := TrimMessageHistory(db, "u1", "chat@s", 1); err != nil {
		t.Fatalf("TrimMessageHistory: %v", err)
	}

	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM whatsmeow_message_secrets"); err != nil {
		t.Fatalf("count secrets: %v", err)
	}
	if n != 1 {
		t.Fatalf("sobraram %d secrets, queria 1 — as duas tabelas sairam de sincronia", n)
	}
}

// TestSetDisconnectedState_KeepsEventsByDefault trava a correcao da issue #305:
// desconectar NAO pode zerar a assinatura de eventos, senao o usuario volta
// conectado e para de receber webhook sem ter mudado nada.
func TestSetDisconnectedState_KeepsEventsByDefault(t *testing.T) {
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO users (id, name, token, webhook, jid, qrcode, events, connected) VALUES ('u1','a','t','','','','Message,ReadReceipt',1)"); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if err := SetDisconnectedState(db, "u1", false); err != nil {
		t.Fatalf("SetDisconnectedState: %v", err)
	}

	var events string
	var connected int
	if err := db.QueryRow("SELECT events, connected FROM users WHERE id = 'u1'").Scan(&events, &connected); err != nil {
		t.Fatalf("select: %v", err)
	}
	if connected != 0 {
		t.Fatalf("connected: got %d, want 0", connected)
	}
	if events != "Message,ReadReceipt" {
		t.Fatalf("events foi zerado sem clearEvents: got %q (issue #305)", events)
	}
}

func TestSetDisconnectedState_ClearsEventsWhenAsked(t *testing.T) {
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO users (id, name, token, webhook, jid, qrcode, events, connected) VALUES ('u1','a','t','','','','Message',1)"); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if err := SetDisconnectedState(db, "u1", true); err != nil {
		t.Fatalf("SetDisconnectedState: %v", err)
	}

	var events string
	if err := db.QueryRow("SELECT events FROM users WHERE id = 'u1'").Scan(&events); err != nil {
		t.Fatalf("select: %v", err)
	}
	if events != "" {
		t.Fatalf("events: got %q, want vazio", events)
	}
}

// TestSetDisconnectedState_IsScopedToTheUser: sem o WHERE, desconectar um
// usuario derrubaria todos.
func TestSetDisconnectedState_IsScopedToTheUser(t *testing.T) {
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	for _, id := range []string{"u1", "u2"} {
		if _, err := db.Exec(
			"INSERT INTO users (id, name, token, webhook, jid, qrcode, events, connected) VALUES (?,'a',?,'','','','Message',1)",
			id, "token-"+id); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	if err := SetDisconnectedState(db, "u1", false); err != nil {
		t.Fatalf("SetDisconnectedState: %v", err)
	}

	var connected int
	if err := db.QueryRow("SELECT connected FROM users WHERE id = 'u2'").Scan(&connected); err != nil {
		t.Fatalf("select u2: %v", err)
	}
	if connected != 1 {
		t.Fatal("desconectar u1 desconectou u2 tambem — falta o WHERE")
	}
}

func TestDeleteUser_RemovesOnlyTheTarget(t *testing.T) {
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	for _, id := range []string{"u1", "u2"} {
		insertUser(t, db, id, "nome-"+id, "token-"+id)
	}
	repo := NewUserRepository(db)

	ok, err := repo.DeleteUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if !ok {
		t.Fatal("DeleteUser devolveu false para um usuario que existia")
	}

	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM users WHERE id = 'u2'"); err != nil {
		t.Fatalf("count u2: %v", err)
	}
	if n != 1 {
		t.Fatal("deletar u1 removeu u2 tambem")
	}
}

// TestDeleteUser_ReportsMissTruthfully: devolver true para um id inexistente
// faria o chamador reportar sucesso de uma remocao que nao aconteceu.
func TestDeleteUser_ReportsMissTruthfully(t *testing.T) {
	db := openTestDB(t)
	if err := InitializeSchema(db); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	repo := NewUserRepository(db)

	ok, err := repo.DeleteUser(context.Background(), "nao-existe")
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if ok {
		t.Fatal("DeleteUser devolveu true para um id que nao existe")
	}
}

func TestGetDatabaseConfig_UsesPostgresOnlyWhenFullyConfigured(t *testing.T) {
	vars := []string{"DB_USER", "DB_PASSWORD", "DB_NAME", "DB_HOST", "DB_PORT", "DB_SSLMODE"}
	completo := map[string]string{
		"DB_USER": "u", "DB_PASSWORD": "p", "DB_NAME": "n", "DB_HOST": "h", "DB_PORT": "5432",
	}

	// Faltando UMA variavel, o retorno tem que ser sqlite. Um OR no lugar do
	// AND faria a aplicacao tentar postgres com DSN incompleto.
	for _, faltante := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME", "DB_HOST", "DB_PORT"} {
		t.Run("sem "+faltante, func(t *testing.T) {
			for _, v := range vars {
				t.Setenv(v, "")
			}
			for k, val := range completo {
				if k == faltante {
					continue
				}
				t.Setenv(k, val)
			}
			if got := GetDatabaseConfig("/exec", ""); got.Type != "sqlite" {
				t.Fatalf("sem %s o tipo foi %q, queria sqlite", faltante, got.Type)
			}
		})
	}

	t.Run("completo", func(t *testing.T) {
		for _, v := range vars {
			t.Setenv(v, "")
		}
		for k, val := range completo {
			t.Setenv(k, val)
		}
		got := GetDatabaseConfig("/exec", "")
		if got.Type != "postgres" {
			t.Fatalf("tipo: got %q, want postgres", got.Type)
		}
		if got.SSLMode != "disable" {
			t.Fatalf("DB_SSLMODE vazio virou %q, queria disable", got.SSLMode)
		}
	})
}

func TestGetDatabaseConfig_MapsSSLModeAliases(t *testing.T) {
	for _, v := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME", "DB_HOST", "DB_PORT"} {
		t.Setenv(v, "x")
	}

	cases := map[string]string{
		"true":        "require",
		"false":       "disable",
		"":            "disable",
		"verify-full": "verify-full",
	}
	for in, want := range cases {
		t.Run("DB_SSLMODE="+in, func(t *testing.T) {
			t.Setenv("DB_SSLMODE", in)
			if got := GetDatabaseConfig("/exec", "").SSLMode; got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

// TestGetDatabaseConfig_DataDirFlagWinsOverExecPath: a flag existe para
// permitir montar o volume em outro lugar. Se ela for ignorada, o banco nasce
// dentro do container e some no proximo deploy.
func TestGetDatabaseConfig_DataDirFlagWinsOverExecPath(t *testing.T) {
	for _, v := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME", "DB_HOST", "DB_PORT", "DB_SSLMODE"} {
		t.Setenv(v, "")
	}

	comFlag := GetDatabaseConfig("/caminho/do/executavel", "/dados/persistentes")
	if comFlag.Path != "/dados/persistentes/dbdata" {
		t.Fatalf("com --datadir: got %q", comFlag.Path)
	}

	semFlag := GetDatabaseConfig("/caminho/do/executavel", "")
	if semFlag.Path != "/caminho/do/executavel/dbdata" {
		t.Fatalf("sem --datadir: got %q", semFlag.Path)
	}
}
