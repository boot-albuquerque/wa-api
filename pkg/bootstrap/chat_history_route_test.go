package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/justinas/alice"
	"github.com/rs/zerolog"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/wa-noise/observability/applog"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/handlers"
)

// CAP-09A — GET /chat/history.
//
// Estes testes exercitam a ROTA REGISTRADA (registerCustomRoutes + gorilla/mux),
// e nao o handler cru: a ARMADILHA 2 deste repo e' exatamente isso, e o defeito
// que eles travam (F124) era de FIACAO — /chat/history apontava para o handler
// de /webhook/history e respondia um literal sem tocar no banco.
//
// A persistencia e' SQLite real com o schema de producao. Um fake nao serve
// para o isolamento de tenant: ele nao tem `WHERE user_id` para esquecer.

const (
	historyChatA = "a1@s.whatsapp.net"
	historyChatB = "b1@s.whatsapp.net"
)

// chatHistoryFixture monta banco + roteador real.
//
// injectUser e' a identidade que a cadeia coloca no contexto sob
// appport.UserInfoKey, imitando o que middleware/auth.go faz depois de
// autenticar (auth.go:199-207 monta o mesmo Values com "Id" e "History"). Um
// injectUser nil NAO poe nada no contexto — e' o caso do chamador sem
// identidade.
type chatHistoryFixture struct {
	db       *sqlx.DB
	router   *mux.Router
	resolver *lidResolverStub
}

func newChatHistoryFixture(t *testing.T, injectUser func() *Values) *chatHistoryFixture {
	t.Helper()
	return newChatHistoryFixtureLogging(t, injectUser, nil)
}

// lidResolverStub é o dublê de appport.LIDResolver.
//
// Imita a REGRA REAL do adapter, não uma versão conveniente dela
// (adapters/user/adapter.go:119 e sqlstore/lidmap.go:125): mapeamento
// desconhecido devolve JID VAZIA com erro NIL, porque ausência é resposta e não
// falha; e um JID que não seja @lid é ERRO, porque o store recusa a chamada.
//
// A regra do erro importa: um dublê permissivo que devolvesse ""/nil para um PN
// deixaria passar um use case que chamasse o resolvedor com o identificador
// errado, e o teste ficaria verde contra produção vermelha (ARMADILHAS.md, 1).
type lidResolverStub struct {
	mapa   map[string]string
	falha  error
	chamou []string
}

func (r *lidResolverStub) GetPNForLID(_ context.Context, _ string, lid domain.JID) (domain.JID, error) {
	r.chamou = append(r.chamou, string(lid))
	if r.falha != nil {
		return "", r.falha
	}
	if !strings.HasSuffix(string(lid), "@lid") {
		return "", errors.New("store recusa: identificador nao e' @lid")
	}
	if pn, ok := r.mapa[string(lid)]; ok {
		return domain.JID(pn), nil
	}
	return "", nil
}

// newChatHistoryFixtureLogging permite capturar o que os use cases logam. Com
// logOut nil o logger e' Nop: a maioria dos testes nao olha para o log, e um
// logger silencioso mantem a saida do `go test` legivel.
func newChatHistoryFixtureLogging(t *testing.T, injectUser func() *Values, logOut io.Writer) *chatHistoryFixture {
	t.Helper()
	return newChatHistoryFixtureFull(t, injectUser, logOut, &lidResolverStub{})
}

// newChatHistoryFixtureFull permite escolher o resolvedor de LID. Os outros
// construtores passam um stub VAZIO — que resolve nada e devolve o
// comportamento anterior à F183 — para que os testes existentes continuem a
// medir o que mediam.
func newChatHistoryFixtureFull(t *testing.T, injectUser func() *Values, logOut io.Writer, resolver *lidResolverStub) *chatHistoryFixture {
	t.Helper()

	database := newChatHistoryDB(t)
	zl := zerolog.Nop()
	if logOut != nil {
		zl = zerolog.New(logOut)
	}
	logger := applog.NewZerologAdapter(zl)

	repo := db.NewChatHistoryRepository(database)
	ch := &customHandlers{
		Profile:     &customhttp.ProfileHandler{},
		ProfileFull: &customhttp.ProfileFullHandler{},
		Message:     &MessageHandlers{},
		Session:     &SessionHandlers{},
		Webhook:     &WebhookHandlers{},
		User:        &handlers.UserHandlers{},
		Group:       &handlers.GroupHandlers{},
		Misc:        &handlers.MiscHandlers{},
		Blocklist:   &handlers.BlocklistHandlers{},
		Download:    &handlers.DownloadHandlers{},
		Presence:    &handlers.PresenceHandlers{},
		Reaction:    &handlers.ReactionHandlers{},
		Contact:     &handlers.ContactHandlers{},
		GroupMgmt:   &handlers.GroupManagementHandlers{},

		// Os dois handlers sob teste, ambos REAIS.
		Storage: &handlers.StorageHandlers{
			GetHistory: handlers.NewGetHistoryHandler(
				storage.NewGetHistoryUseCase(alwaysSessionGuard{}, db.NewSessionConfigRepository(database), logger)),
		},
		ChatHistory: &handlers.ChatHistoryHandlers{
			GetChatHistory: handlers.NewGetChatHistoryHandler(
				chat.NewGetChatHistoryUseCase(repo, logger).WithLIDResolver(resolver)),
		},
	}

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if injectUser == nil {
				next.ServeHTTP(w, r)
				return
			}
			v := injectUser()
			if v == nil {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(
				context.WithValue(r.Context(), appport.UserInfoKey, *v)))
		})
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(inject), ch)
	return &chatHistoryFixture{db: database, router: router, resolver: resolver}
}

// newChatHistoryDB aplica o schema de producao num SQLite de arquivo
// temporario — mesma via de pkg/infra/db/migrations_test.go.
func newChatHistoryDB(t *testing.T) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("sqlite", t.TempDir()+"/chat_history.db"+db.SQLitePragmas)
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

// userValues monta a identidade cacheada como o middleware de auth a monta.
func userValues(id string, history int) *Values {
	return &Values{M: map[string]string{"Id": id, "History": fmt.Sprintf("%d", history)}}
}

// alwaysSessionGuard e' o dublê de sessao do handler de /webhook/history, que
// so' existe para o teste estrutural. Permissivo de proposito: o que esta sob
// teste ali e' QUAL handler a rota alcanca, nao a guarda de sessao dele.
type alwaysSessionGuard struct{}

func (alwaysSessionGuard) EnsureSession(context.Context, string) error { return nil }

// seedHistoryRow grava uma mensagem com timestamp escolhido pelo teste, com as
// MESMAS colunas do INSERT de producao (pkg/infra/db/message_history.go).
func (f *chatHistoryFixture) seedHistoryRow(t *testing.T, userID, chatJID, messageID string, ts time.Time) {
	t.Helper()
	query := f.db.Rebind(`INSERT INTO message_history
		(user_id, chat_jid, sender_jid, message_id, timestamp, message_type,
		 text_content, media_link, quoted_message_id, datajson, sender_push_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if _, err := f.db.Exec(query, userID, chatJID, "s@s.whatsapp.net", messageID, ts,
		"text", "texto "+messageID, "", "", "", ""); err != nil {
		t.Fatalf("seed %s/%s: %v", userID, messageID, err)
	}
}

// seedUser grava a linha de users com o valor PERSISTIDO de history — a metade
// do gate que o cache nao conhece.
func (f *chatHistoryFixture) seedUser(t *testing.T, id string, history int) {
	t.Helper()
	if _, err := f.db.Exec(f.db.Rebind(
		`INSERT INTO users (id, name, token, token_hash, history) VALUES (?, ?, ?, ?, ?)`),
		id, "tenant "+id, "token-"+id, "hash-"+id, history); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func (f *chatHistoryFixture) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// envelope e' a forma do ADR-002 devolvida por customhttp.RespondJSON.
type envelope struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Error   json.RawMessage `json:"error"`
	Success bool            `json:"success"`
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v (corpo: %s)", err, rec.Body.String())
	}
	return env
}

// --- gate de History: os TRES estados -------------------------------------

// Estado (i): userinfo.History > 0. Nao revalida como desabilitado e CONSULTA
// o historico.
func TestChatHistoryRoute_GateStateCachedHistoryEnabled(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	f.seedUser(t, "A", 10)
	f.seedHistoryRow(t, "A", historyChatA, "A-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := f.get(t, "/chat/history?chat_jid="+historyChatA)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var msgs []appport.ChatHistoryMessage
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &msgs); err != nil {
		t.Fatalf("data nao e' a lista de mensagens: %v (corpo: %s)", err, rec.Body.String())
	}
	if len(msgs) != 1 || msgs[0].MessageID != "A-1" {
		t.Fatalf("mensagens = %+v, quero exatamente [A-1]", msgs)
	}
}

// Estado (ii): userinfo.History = 0 e a persistencia continua 0 -> 501 com a
// causa canonica.
func TestChatHistoryRoute_GateStateDisabledEverywhereIs501(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 0) })
	f.seedUser(t, "A", 0)
	f.seedHistoryRow(t, "A", historyChatA, "A-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := f.get(t, "/chat/history?chat_jid="+historyChatA)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, quero 501 (corpo: %s)", rec.Code, rec.Body.String())
	}
	const causaCanonica = "message history is disabled for this user"
	if !strings.Contains(rec.Body.String(), causaCanonica) {
		t.Errorf("corpo = %s, quero conter a causa canonica %q", rec.Body.String(), causaCanonica)
	}
}

// Estado (iii): userinfo.History = 0, mas a persistencia JA foi alterada para
// > 0. A operacao CONTINUA — 501 aqui seria falso, e e' o que um gate
// "simplificado" para um unico check devolveria.
func TestChatHistoryRoute_GateStateStaleZeroRevalidatesAndProceeds(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 0) })
	f.seedUser(t, "A", 30) // o usuario LIGOU o historico; o cache ainda diz 0
	f.seedHistoryRow(t, "A", historyChatA, "A-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := f.get(t, "/chat/history?chat_jid="+historyChatA)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 — o 0 do cache foi revalidado como 30 (corpo: %s)",
			rec.Code, rec.Body.String())
	}
	var msgs []appport.ChatHistoryMessage
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &msgs); err != nil {
		t.Fatalf("data nao e' a lista de mensagens: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("mensagens = %+v, quero 1", msgs)
	}
}

// --- isolamento de tenant pela ROTA ---------------------------------------

// Caso 1: A e B tem historico; a chamada autenticada como A devolve SOMENTE a
// chave A e chats de A.
func TestChatHistoryRoute_IndexReturnsOnlyCallerTenant(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	f.seedUser(t, "A", 10)
	f.seedUser(t, "B", 10)
	f.seedHistoryRow(t, "A", historyChatA, "A-1", base)
	f.seedHistoryRow(t, "B", historyChatB, "B-1", base.Add(time.Hour))

	// Ordem de mapa em Go e' aleatoria por desenho: repetir e' o que separa
	// "isolado" de "deu sorte".
	for volta := 0; volta < 20; volta++ {
		rec := f.get(t, "/chat/history?chat_jid=index")
		if rec.Code != http.StatusOK {
			t.Fatalf("volta %d: status = %d, quero 200 (corpo: %s)", volta, rec.Code, rec.Body.String())
		}
		corpo := rec.Body.String()
		if strings.Contains(corpo, "\"B\"") || strings.Contains(corpo, historyChatB) {
			t.Fatalf("volta %d: dados do tenant B na resposta de A: %s", volta, corpo)
		}
		var index map[string][]appport.ChatIndexEntry
		if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &index); err != nil {
			t.Fatalf("volta %d: data nao e' o mapa do index: %v (corpo: %s)", volta, err, corpo)
		}
		if len(index) != 1 || len(index["A"]) != 1 || index["A"][0].ChatJID != historyChatA {
			t.Fatalf("volta %d: index = %v, quero somente {A: [%s]}", volta, index, historyChatA)
		}
	}
}

// Caso 2: B tem chat cujo JID COINCIDE com um de A. O isolamento e' por
// user_id, nao por chat_jid.
func TestChatHistoryRoute_IndexIsolatesOnUserIDNotChatJID(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	f.seedUser(t, "A", 10)
	f.seedUser(t, "B", 10)
	const compartilhado = "mesmo@s.whatsapp.net"
	f.seedHistoryRow(t, "A", compartilhado, "A-1", base)
	f.seedHistoryRow(t, "B", compartilhado, "B-1", base.Add(10*time.Hour))

	rec := f.get(t, "/chat/history?chat_jid=index")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var index map[string][]appport.ChatIndexEntry
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &index); err != nil {
		t.Fatalf("data nao e' o mapa do index: %v", err)
	}
	if len(index) != 1 || len(index["A"]) != 1 {
		t.Fatalf("index = %v, quero exatamente um chat sob a chave A", index)
	}
	quero := base.Format(time.RFC3339Nano)
	if index["A"][0].LastUpdated != quero {
		t.Errorf("last_updated = %q, quero %q — o timestamp de B vazou para o grupo de A",
			index["A"][0].LastUpdated, quero)
	}
}

// Caso 3: A nao tem historico e B tem -> resposta de A e' mapa vazio.
func TestChatHistoryRoute_IndexEmptyMapWhenCallerHasNoHistory(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	f.seedUser(t, "A", 10)
	f.seedUser(t, "B", 10)
	f.seedHistoryRow(t, "B", historyChatB, "B-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := f.get(t, "/chat/history?chat_jid=index")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	corpo := rec.Body.String()
	if strings.Contains(corpo, "\"B\"") || strings.Contains(corpo, historyChatB) {
		t.Fatalf("dados do tenant B na resposta de A: %s", corpo)
	}
	if got := string(decodeEnvelope(t, rec).Data); got != "{}" {
		t.Errorf("data = %s, quero {} (mapa vazio)", got)
	}
}

// Caso 4: identidade ausente -> fail-closed pelo mecanismo canonico de
// autenticacao dos handlers (info nil -> 401).
func TestChatHistoryRoute_MissingIdentityIsRejected(t *testing.T) {
	f := newChatHistoryFixture(t, nil)
	f.seedUser(t, "B", 10)
	f.seedHistoryRow(t, "B", historyChatB, "B-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := f.get(t, "/chat/history?chat_jid=index")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, quero 401 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), historyChatB) {
		t.Fatalf("dado de tenant vazou numa resposta nao autenticada: %s", rec.Body.String())
	}
}

// Identidade presente mas SEM id de sessao -> 400, mesmo mecanismo canonico.
func TestChatHistoryRoute_MissingSessionIDIsRejected(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("", 10) })

	rec := f.get(t, "/chat/history?chat_jid=index")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
}

// --- parametros de consulta ------------------------------------------------

func TestChatHistoryRoute_MissingChatJIDIs400(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	f.seedUser(t, "A", 10)

	rec := f.get(t, "/chat/history")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "chat_jid is required") {
		t.Errorf("corpo = %s, quero conter %q", rec.Body.String(), "chat_jid is required")
	}
}

func TestChatHistoryRoute_InvalidLimitIs400(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	f.seedUser(t, "A", 10)

	rec := f.get(t, "/chat/history?chat_jid="+historyChatA+"&limit=abc")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid limit") {
		t.Errorf("corpo = %s, quero conter %q", rec.Body.String(), "invalid limit")
	}
}

// TestChatHistoryRoute_DefaultLimitIs50 semeia 51 mensagens e nao passa
// `limit`: 50 e' o default historico, e um default de 100 (ou ausente)
// devolveria 51.
func TestChatHistoryRoute_DefaultLimitIs50(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 100) })
	f.seedUser(t, "A", 100)
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 51; i++ {
		f.seedHistoryRow(t, "A", historyChatA, fmt.Sprintf("A-%02d", i), base.Add(time.Duration(i)*time.Minute))
	}

	rec := f.get(t, "/chat/history?chat_jid="+historyChatA)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var msgs []appport.ChatHistoryMessage
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &msgs); err != nil {
		t.Fatalf("data nao e' a lista de mensagens: %v", err)
	}
	if len(msgs) != 50 {
		t.Fatalf("len = %d, quero 50 (default historico)", len(msgs))
	}
	// A mais antiga (A-00) e' a que ficou de fora — a ordenacao e' DESC.
	if msgs[0].MessageID != "A-50" || msgs[49].MessageID != "A-01" {
		t.Errorf("janela = [%s .. %s], quero [A-50 .. A-01]", msgs[0].MessageID, msgs[49].MessageID)
	}
}

// TestChatHistoryRoute_OrderIsTimestampDesc usa timestamps ASSIMETRICOS e fora
// da ordem de insercao: um `ORDER BY id` acidental passaria com timestamps
// crescentes.
func TestChatHistoryRoute_OrderIsTimestampDesc(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	f.seedUser(t, "A", 10)
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	f.seedHistoryRow(t, "A", historyChatA, "MEIO", base.Add(30*time.Minute))
	f.seedHistoryRow(t, "A", historyChatA, "NOVA", base.Add(9*time.Hour))
	f.seedHistoryRow(t, "A", historyChatA, "VELHA", base)

	rec := f.get(t, "/chat/history?chat_jid="+historyChatA)
	var msgs []appport.ChatHistoryMessage
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &msgs); err != nil {
		t.Fatalf("data nao e' a lista de mensagens: %v (corpo: %s)", err, rec.Body.String())
	}
	quero := []string{"NOVA", "MEIO", "VELHA"}
	if len(msgs) != len(quero) {
		t.Fatalf("len = %d, quero %d", len(msgs), len(quero))
	}
	for i, id := range quero {
		if msgs[i].MessageID != id {
			t.Fatalf("posicao %d = %q, quero %q", i, msgs[i].MessageID, id)
		}
	}
}

// Resultado vazio e' 200 com array vazio — ausencia de mensagem nao e' erro, e
// `[]` nao e' `null`.
func TestChatHistoryRoute_EmptyResultIs200WithEmptyArray(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	f.seedUser(t, "A", 10)

	rec := f.get(t, "/chat/history?chat_jid=vazio@s.whatsapp.net")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if got := string(decodeEnvelope(t, rec).Data); got != "[]" {
		t.Errorf("data = %s, quero [] (nunca null)", got)
	}
}

// --- teste ESTRUTURAL de fiacao -------------------------------------------

// TestChatHistoryAndWebhookHistoryAreDistinctHandlers e' o teste de FIACAO do
// CAP-09A.
//
// O defeito da F124 nao era de payload: as duas rotas estavam registradas no
// MESMO handler (custom_routes.go historico, linhas 118 e 170), e a migracao
// reproduziu a fiacao fielmente enquanto perdia a implementacao semantica.
// Nenhuma assercao sobre corpo de resposta o teria pego, porque o corpo que
// /chat/history devolvia era um corpo VALIDO — o do outro endpoint.
//
// O que ele impede: religar /chat/history a Storage.GetHistory (ou o inverso).
// A prova e' que cada rota devolve algo que SO' o seu handler sabe produzir —
// /chat/history, mensagens vindas do banco; /webhook/history, o LIMITE GRAVADO
// em users.history. Identidade de ponteiro nao serviria: a chain de middleware
// embrulha os dois, e dois wrappers distintos comparariam diferente mesmo se o
// handler embrulhado fosse o mesmo.
//
// O discriminador de /webhook/history MUDOU no CAP-32, e a troca e' o ponto da
// HOUSEKEEP F166. Ate' entao ele era o literal "History configuration
// retrieved" — uma string que o STUB da migracao inventou e que nunca existiu
// no contrato historico (`41bc8e2^:handlers.go:6497` lia historico de
// MENSAGENS). Um teste anti-regressao ancorado num valor que so' existe porque
// a implementacao e' stub perde a ancora no dia em que o contrato chega. O
// discriminador de agora e' um valor de CONTRATO: o numero que a rota le' do
// banco. Ele nao pode ser fabricado por outro handler porque nenhum outro
// handler le' aquela coluna para responde-la.
//
// `historyDoWebhook` (4242) e' de proposito DIFERENTE do History cacheado (10)
// que o injetor poe no contexto: assim o teste tambem morde se a leitura
// passar a sair do cache em vez do banco (F128/F164).
func TestChatHistoryAndWebhookHistoryAreDistinctHandlers(t *testing.T) {
	const (
		historyCacheado  = 10
		historyDoWebhook = 4242
	)

	f := newChatHistoryFixture(t, func() *Values { return userValues("A", historyCacheado) })
	f.seedUser(t, "A", historyDoWebhook)
	f.seedHistoryRow(t, "A", historyChatA, "SO-DO-CHAT", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	limiteGravado := fmt.Sprintf("%d", historyDoWebhook)

	chatRec := f.get(t, "/chat/history?chat_jid="+historyChatA)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("/chat/history: status = %d, quero 200 (corpo: %s)", chatRec.Code, chatRec.Body.String())
	}
	if strings.Contains(chatRec.Body.String(), limiteGravado) {
		t.Fatalf("/chat/history respondeu o limite de configuracao de /webhook/history — as duas rotas voltaram a apontar para o mesmo handler: %s",
			chatRec.Body.String())
	}
	if !strings.Contains(chatRec.Body.String(), "SO-DO-CHAT") {
		t.Fatalf("/chat/history nao devolveu a mensagem do banco; corpo: %s", chatRec.Body.String())
	}

	webhookRec := f.get(t, "/webhook/history")
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("/webhook/history: status = %d, quero 200 (corpo: %s)", webhookRec.Code, webhookRec.Body.String())
	}
	var lido domain.WebhookHistoryResult
	if err := json.Unmarshal(decodeEnvelope(t, webhookRec).Data, &lido); err != nil {
		t.Fatalf("/webhook/history: data nao e' a configuracao de historico: %v (corpo: %s)", err, webhookRec.Body.String())
	}
	if lido.History != historyDoWebhook {
		t.Fatalf("/webhook/history: History = %d, quero %d — a rota deixou de responder o limite gravado e o discriminador caiu; corpo: %s",
			lido.History, historyDoWebhook, webhookRec.Body.String())
	}
	if strings.Contains(webhookRec.Body.String(), "SO-DO-CHAT") {
		t.Fatalf("/webhook/history respondeu historico de mensagens — as duas rotas voltaram a apontar para o mesmo handler: %s",
			webhookRec.Body.String())
	}
}

// TestChatHistoryRoute_DoesNotLogSecrets: o handler loga user, caminho e erro.
// Nenhum deles pode carregar token nem o conteudo das mensagens.
func TestChatHistoryRoute_DoesNotLogSecrets(t *testing.T) {
	var capturado strings.Builder

	f := newChatHistoryFixtureLogging(t, func() *Values {
		v := userValues("A", 10)
		v.M["Token"] = "TOKEN-SUPER-SECRETO"
		return v
	}, &capturado)
	f.seedUser(t, "A", 10)
	f.seedHistoryRow(t, "A", historyChatA, "A-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	if rec := f.get(t, "/chat/history?chat_jid="+historyChatA); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", rec.Code)
	}
	for _, proibido := range []string{"TOKEN-SUPER-SECRETO", "texto A-1"} {
		if strings.Contains(capturado.String(), proibido) {
			t.Errorf("log vazou %q: %s", proibido, capturado.String())
		}
	}
}

// --- F183: tradução @lid→telefone na LEITURA ---------------------------------

// TestChatHistoryLID_TraduzEDevolveOQueEstaSobOTelefone é o defeito MEDIDO em
// campo: das 30 conversas que /chat/list devolve, 27 vinham vazias porque a
// listagem entrega @lid e as linhas estão gravadas sob o telefone.
//
// Pela ROTA REGISTADA, não pelo handler cru: a extração do parâmetro faz parte
// do defeito, e um handler montado sem padrão de rota não a exercita — foi
// assim que a F81 sobreviveu.
func TestChatHistoryLID_TraduzEDevolveOQueEstaSobOTelefone(t *testing.T) {
	const lid = "182699419517150@lid"
	const pn = "556799881100@s.whatsapp.net"

	f := newChatHistoryFixtureFull(t, func() *Values { return userValues("U-LID", 50) }, nil,
		&lidResolverStub{mapa: map[string]string{lid: pn}})
	f.seedHistoryRow(t, "U-LID", pn, "MSG-SOB-PN", time.Now().Add(-time.Minute))

	rec := f.get(t, "/chat/history?chat_jid="+url.QueryEscape(lid))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", rec.Code)
	}
	msgs := chatHistoryMessages(t, rec)
	if len(msgs) != 1 {
		t.Fatalf("registos = %d, quero 1: pedir pelo @lid devolveu vazio, que e' o defeito da F183", len(msgs))
	}
	if msgs[0].MessageID != "MSG-SOB-PN" {
		t.Errorf("message_id = %q, quero MSG-SOB-PN", msgs[0].MessageID)
	}
}

// TestChatHistoryLID_FundeAsDuasChaves trava a parte que separa "corrigido" de
// "meio corrigido". Três conversas reais têm linhas sob AMBAS as chaves;
// traduzir e ler só o telefone trocaria um vazio por METADE — pior, porque uma
// lista vazia pelo menos parece errada.
func TestChatHistoryLID_FundeAsDuasChaves(t *testing.T) {
	const lid = "182699419517150@lid"
	const pn = "556799881100@s.whatsapp.net"

	f := newChatHistoryFixtureFull(t, func() *Values { return userValues("U-LID", 50) }, nil,
		&lidResolverStub{mapa: map[string]string{lid: pn}})
	agora := time.Now()
	f.seedHistoryRow(t, "U-LID", pn, "MSG-ANTIGA-PN", agora.Add(-2*time.Hour))
	f.seedHistoryRow(t, "U-LID", lid, "MSG-RECENTE-LID", agora.Add(-time.Minute))

	msgs := chatHistoryMessages(t, f.get(t, "/chat/history?chat_jid="+url.QueryEscape(lid)))
	if len(msgs) != 2 {
		t.Fatalf("registos = %d, quero 2 (as duas chaves fundidas)", len(msgs))
	}
	// E fundidas em ORDEM: a mais recente primeiro, independentemente da chave.
	if msgs[0].MessageID != "MSG-RECENTE-LID" || msgs[1].MessageID != "MSG-ANTIGA-PN" {
		t.Errorf("ordem = [%s %s], quero a mais recente primeiro", msgs[0].MessageID, msgs[1].MessageID)
	}
}

// TestChatHistoryLID_SemMapeamentoDevolveOQueTem: LID desconhecido devolve JID
// vazia SEM erro, e a leitura tem de continuar a servir o que está sob o LID.
func TestChatHistoryLID_SemMapeamentoDevolveOQueTem(t *testing.T) {
	const lid = "999999999999@lid"

	f := newChatHistoryFixtureFull(t, func() *Values { return userValues("U-LID", 50) }, nil,
		&lidResolverStub{mapa: map[string]string{}})
	f.seedHistoryRow(t, "U-LID", lid, "MSG-SOB-LID", time.Now())

	msgs := chatHistoryMessages(t, f.get(t, "/chat/history?chat_jid="+url.QueryEscape(lid)))
	if len(msgs) != 1 || msgs[0].MessageID != "MSG-SOB-LID" {
		t.Fatalf("registos = %d: LID sem mapeamento nao pode perder o que ja' estava sob ele", len(msgs))
	}
}

// TestChatHistoryLID_ResolvedorEmFalhaNuncaPioraALeitura é a invariante que eu
// quero travada: este use case NÃO exige sessão viva, e o resolvedor passa pelo
// cliente da sessão. Se traduzir passasse a ser obrigatório, uma leitura
// puramente local começaria a falhar com o telemóvel offline — a correção
// tornaria um cenário ESTRITAMENTE PIOR, que é a falha de 2026-08-08 outra vez.
func TestChatHistoryLID_ResolvedorEmFalhaNuncaPioraALeitura(t *testing.T) {
	const lid = "182699419517150@lid"

	f := newChatHistoryFixtureFull(t, func() *Values { return userValues("U-LID", 50) }, nil,
		&lidResolverStub{falha: errors.New("sessao offline")})
	f.seedHistoryRow(t, "U-LID", lid, "MSG-SOB-LID", time.Now())

	rec := f.get(t, "/chat/history?chat_jid="+url.QueryEscape(lid))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200: resolvedor em falha nao pode derrubar uma leitura local", rec.Code)
	}
	msgs := chatHistoryMessages(t, rec)
	if len(msgs) != 1 || msgs[0].MessageID != "MSG-SOB-LID" {
		t.Fatalf("registos = %d, quero 1: com o resolvedor em falha a leitura tem de devolver o de antes", len(msgs))
	}
}

// TestChatHistoryLID_PedidoPorTelefoneNaoChamaOResolvedor é o controle na
// direção oposta, e não é zelo: o store REAL recusa GetPNForLID com um
// identificador que não seja @lid (sqlstore/lidmap.go:125). Chamar o resolvedor
// para um PN produziria erro em produção — e, com um dublê permissivo, verde no
// teste.
func TestChatHistoryLID_PedidoPorTelefoneNaoChamaOResolvedor(t *testing.T) {
	const pn = "556799881100@s.whatsapp.net"

	stub := &lidResolverStub{mapa: map[string]string{}}
	f := newChatHistoryFixtureFull(t, func() *Values { return userValues("U-LID", 50) }, nil, stub)
	f.seedHistoryRow(t, "U-LID", pn, "MSG-PN", time.Now())

	msgs := chatHistoryMessages(t, f.get(t, "/chat/history?chat_jid="+url.QueryEscape(pn)))
	if len(msgs) != 1 {
		t.Fatalf("registos = %d, quero 1", len(msgs))
	}
	if len(stub.chamou) != 0 {
		t.Errorf("resolvedor chamado com %v: o store real recusa quem nao e' @lid", stub.chamou)
	}
}

// chatHistoryMessages decodifica a lista de mensagens do envelope do ADR-002.
func chatHistoryMessages(t *testing.T, rec *httptest.ResponseRecorder) []appport.ChatHistoryMessage {
	t.Helper()
	var msgs []appport.ChatHistoryMessage
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &msgs); err != nil {
		t.Fatalf("data nao e' lista de mensagens: %v (corpo: %s)", err, rec.Body.String())
	}
	return msgs
}
