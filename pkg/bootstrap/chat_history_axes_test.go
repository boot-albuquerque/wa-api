package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/wa-noise/observability/applog"
	"wa-api/pkg/presentation/http/handlers"
)

// CAP-19 — os eixos de fronteira de GET /chats/history.
//
// A capability e' de LEITURA, entao a matriz difere da das capabilities de
// acao:
//
//	1. missing session id     — autenticado, `Id` vazio => 400, sem consulta
//	2. corpo malformado       — NAO SE APLICA: a rota e' GET e o handler nunca
//	                            le r.Body (handler_chat_history.go:38-93). Nao
//	                            ha' decode para falhar, e um teste de corpo
//	                            truncado aqui mediria o parser de nada.
//	3. sucesso nao loga       — caminho feliz sem registro warn/error
//	4. wrong type in context  — valor que nao satisfaz userInfo => 401, sem panico
//	5. txtID na consulta      — o userID que chega ao ChatHistoryReader e' o do
//	                            CONTEXTO. Aqui isto e' isolamento de TENANT
//	                            (familia da F125), nao so' propagacao de campo.
//
// O que chat_history_route_test.go ja' cobria, e por isso NAO se repete aqui:
// o gate de History nos tres estados, o isolamento de tenant do ramo INDEX
// (nas duas formas), os parametros de consulta, a ordenacao, a fiacao contra
// /webhook/history e a ausencia de segredo no log.
//
// O que ele NAO cobria, e este arquivo fecha:
//
//   - o eixo 1 asseverava so' o status 400, sem a CAUSA e sem provar que a
//     consulta nao aconteceu;
//   - o eixo 3 nao existia: o fixture de la' nao instala a cadeia hlog, entao
//     hlog.FromRequest devolve um logger desabilitado e NADA do que o handler
//     registra e' observavel — inclusive em TestChatHistoryRoute_DoesNotLogSecrets,
//     que so' ve' o que os use cases logam;
//   - o eixo 4 nao existia: `injectUser` devolve *Values, entao nenhum teste
//     conseguia por no contexto um valor de outro tipo;
//   - o eixo 5 estava provado apenas no ramo `chat_jid=index`. O ramo de
//     MENSAGENS — o normal — nunca foi exercitado com dois tenants no MESMO
//     chat_jid, que e' exatamente a forma que um `WHERE user_id` esquecido
//     teria deixado passar.

// countingChatHistoryReader e' o repositorio REAL com um contador em volta.
//
// Delegar ao repositorio de producao (e nao substitui-lo por um dublê) e'
// deliberado: o isolamento de tenant mora no `WHERE user_id` do SQL, e um
// dublê nao tem clausula para esquecer — seria o dublê mais permissivo que a
// producao que a ARMADILHA 1 descreve. O contador so' acrescenta a
// observacao de QUANTAS vezes e com QUAL userID a consulta foi feita.
type countingChatHistoryReader struct {
	inner appport.ChatHistoryReader

	listCalls  []string // userID de cada ListChatMessages
	indexCalls []string // userID de cada ChatIndexByUser
	limitCalls []string // userID de cada HistoryLimit
}

func (r *countingChatHistoryReader) ListChatMessages(ctx context.Context, userID, chatJID string, limit int) ([]appport.ChatHistoryMessage, error) {
	r.listCalls = append(r.listCalls, userID)
	return r.inner.ListChatMessages(ctx, userID, chatJID, limit)
}

func (r *countingChatHistoryReader) ChatIndexByUser(ctx context.Context, userID string) (map[string][]appport.ChatIndexEntry, error) {
	r.indexCalls = append(r.indexCalls, userID)
	return r.inner.ChatIndexByUser(ctx, userID)
}

func (r *countingChatHistoryReader) HistoryLimit(ctx context.Context, userID string) (int, error) {
	r.limitCalls = append(r.limitCalls, userID)
	return r.inner.HistoryLimit(ctx, userID)
}

// reads e' o total de consultas ao historico — as tres, porque qualquer uma
// delas ja' significa que a requisicao passou da fronteira.
func (r *countingChatHistoryReader) reads() int {
	return len(r.listCalls) + len(r.indexCalls) + len(r.limitCalls)
}

// historyAxisFixture e' o chatHistoryFixture com duas coisas que os eixos
// deste arquivo exigem e que ele nao tem: a cadeia hlog de producao (sem ela
// o log do HANDLER e' invisivel) e um injetor de contexto que aceita QUALQUER
// valor, e nao so' *Values.
type historyAxisFixture struct {
	*chatHistoryFixture
	reader *countingChatHistoryReader
	logs   *bytes.Buffer
}

// newHistoryAxisFixture monta banco real, roteador real e cadeia hlog real.
//
// injectValue devolve o valor a guardar sob appport.UserInfoKey e se ha'
// valor algum; um `false` NAO poe nada no contexto, que e' o chamador sem
// identidade.
func newHistoryAxisFixture(t *testing.T, injectValue func() (any, bool)) *historyAxisFixture {
	t.Helper()

	database := newChatHistoryDB(t)
	logs := &bytes.Buffer{}
	zl := zerolog.New(logs).With().Timestamp().Logger()
	logger := applog.NewZerologAdapter(zl)

	reader := &countingChatHistoryReader{inner: db.NewChatHistoryRepository(database)}

	ch := emptyCustomHandlers()
	ch.Storage = &handlers.StorageHandlers{
		GetHistory: handlers.NewGetHistoryHandler(
			storage.NewGetHistoryUseCase(alwaysSessionGuard{}, db.NewSessionConfigRepository(database), logger)),
	}
	ch.ChatHistory = &handlers.ChatHistoryHandlers{
		GetChatHistory: handlers.NewGetChatHistoryHandler(
			chat.NewGetChatHistoryUseCase(reader, logger)),
	}

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v, ok := injectValue()
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, v)))
		})
	}

	// A MESMA cadeia request-scoped que router.go:206-217 instala. Sem ela
	// hlog.FromRequest devolve um logger desabilitado e o eixo 3 seria vacuo:
	// "nenhum registro warn/error" passaria porque nao ha' registro nenhum.
	logging := func(next http.Handler) http.Handler {
		return hlog.NewHandler(zl)(hlog.RequestIDHandler("req_id", "Request-Id")(next))
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(logging, inject), ch)

	return &historyAxisFixture{
		chatHistoryFixture: &chatHistoryFixture{db: database, router: router},
		reader:             reader,
		logs:               logs,
	}
}

// historyAxisUser e' a identidade cacheada do middleware de auth, na forma
// que o injetor deste arquivo aceita.
func historyAxisUser(id string, history int) func() (any, bool) {
	return func() (any, bool) { return *userValues(id, history), true }
}

// historyAxisOutcomeRecords devolve so' os registros de caminho de saida
// (warn/error). A selecao e' por NIVEL, e nao pela presenca do campo `error`:
// um Warn de ruido no caminho feliz nao carrega `error` nenhum, e a forma
// fraca o deixaria passar (licao do FIX-10 e da F143).
func (f *historyAxisFixture) outcomeRecords(t *testing.T) []logRecord {
	t.Helper()
	var out []logRecord
	for _, rec := range decodeRecords(t, f.logs) {
		if lvl := rec.str("level"); lvl == "warn" || lvl == "error" {
			out = append(out, rec)
		}
	}
	return out
}

// requireOutcomeCause exige ao menos um registro de saida cujo campo `error`
// contenha a substring, e que ele correlacione por req_id — o mesmo co-gate D
// que logassert.OutcomeLogged aplica na camada de handlers.
func (f *historyAxisFixture) requireOutcomeCause(t *testing.T, want string) {
	t.Helper()
	recs := f.outcomeRecords(t)
	if len(recs) == 0 {
		t.Fatalf("nenhum registro warn/error: o caminho de saida nao logou nada (log: %q)", f.logs.String())
	}
	for _, rec := range recs {
		if !strings.Contains(rec.str("error"), want) {
			continue
		}
		if rec.str("req_id") == "" {
			t.Fatalf("registro com a causa %q nao carrega req_id: %v", want, rec)
		}
		return
	}
	t.Fatalf("nenhum registro de saida carrega a causa %q; log: %q", want, f.logs.String())
}

// --- eixo 1: session id ausente -------------------------------------------

// TestChatHistoryAxis_MissingSessionID trava as tres metades que
// TestChatHistoryRoute_MissingSessionIDIsRejected nao tinha: a CAUSA, a
// ausencia de consulta ao historico e o req_id.
func TestChatHistoryAxis_MissingSessionID(t *testing.T) {
	f := newHistoryAxisFixture(t, historyAxisUser("", 10))
	f.seedUser(t, "A", 10)
	f.seedHistoryRow(t, "A", historyChatA, "A-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := f.get(t, "/chats/history?chat_jid="+historyChatA)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	f.requireOutcomeCause(t, "missing session id")
	if n := f.reader.reads(); n != 0 {
		t.Fatalf("requisicao sem session id consultou o historico %d vez(es)", n)
	}
}

// --- eixo 4: tipo errado no contexto ---------------------------------------

// TestChatHistoryAxis_WrongTypeInContext: a chave do contexto e' tipada mas o
// VALOR e' `any`. Um valor que nao satisfaz userInfo tem de virar 401 — nao
// panico, e muito menos leitura com userID vazio.
func TestChatHistoryAxis_WrongTypeInContext(t *testing.T) {
	f := newHistoryAxisFixture(t, func() (any, bool) { return 42, true })
	f.seedUser(t, "A", 10)
	f.seedHistoryRow(t, "A", historyChatA, "A-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := f.get(t, "/chats/history?chat_jid="+historyChatA)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, quero 401 (corpo: %s)", rec.Code, rec.Body.String())
	}
	f.requireOutcomeCause(t, "unauthorized")
	if n := f.reader.reads(); n != 0 {
		t.Fatalf("contexto com tipo errado consultou o historico %d vez(es)", n)
	}
	if strings.Contains(rec.Body.String(), "A-1") {
		t.Fatalf("dado de tenant vazou numa resposta 401: %s", rec.Body.String())
	}
}

// --- eixo 3: o caminho feliz nao registra desfecho -------------------------

// TestChatHistoryAxis_SuccessEmitsNoOutcomeLog cobre os DOIS ramos, porque
// eles percorrem caminhos diferentes do use case: `index` e uma leitura
// normal de mensagens.
//
// A assercao e' por NIVEL. E ela so' nao e' vacua porque a cadeia hlog esta'
// instalada: com hlog.FromRequest desabilitado (o que o fixture de
// chat_history_route_test.go produz) o buffer ficaria vazio de qualquer jeito.
// A contagem de leituras > 0 e' o que prova que houve requisicao de verdade.
func TestChatHistoryAxis_SuccessEmitsNoOutcomeLog(t *testing.T) {
	for _, caso := range []struct {
		nome  string
		alvo  string
		reads func(*countingChatHistoryReader) int
	}{
		{"mensagens", "/chats/history?chat_jid=" + historyChatA,
			func(r *countingChatHistoryReader) int { return len(r.listCalls) }},
		{"index", "/chats/history?chat_jid=index",
			func(r *countingChatHistoryReader) int { return len(r.indexCalls) }},
	} {
		t.Run(caso.nome, func(t *testing.T) {
			f := newHistoryAxisFixture(t, historyAxisUser("A", 10))
			f.seedUser(t, "A", 10)
			f.seedHistoryRow(t, "A", historyChatA, "A-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

			rec := f.get(t, caso.alvo)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if n := caso.reads(f.reader); n != 1 {
				t.Fatalf("o historico foi consultado %d vez(es), quero 1", n)
			}
			if recs := f.outcomeRecords(t); len(recs) != 0 {
				t.Fatalf("caminho de sucesso emitiu %d registro(s) warn/error: %q", len(recs), f.logs.String())
			}
		})
	}
}

// --- eixo 5: o txtID da consulta e' o do contexto --------------------------

// TestChatHistoryAxis_ReadsUseContextTxtID prova, nos TRES metodos do
// ChatHistoryReader, que o userID entregue e' o do contexto autenticado.
//
// Sem isto, um handler que lesse o tenant de um parametro de consulta —
// `?user=B`, digamos — passaria em todo o resto da suite, porque nenhum outro
// teste olha para o argumento que a porta recebe.
func TestChatHistoryAxis_ReadsUseContextTxtID(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	t.Run("mensagens", func(t *testing.T) {
		f := newHistoryAxisFixture(t, historyAxisUser("A", 10))
		f.seedUser(t, "A", 10)
		f.seedHistoryRow(t, "A", historyChatA, "A-1", base)

		if rec := f.get(t, "/chats/history?chat_jid="+historyChatA+"&user=B"); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
		}
		if got := f.reader.listCalls; len(got) != 1 || got[0] != "A" {
			t.Fatalf("ListChatMessages recebeu %v, quero exatamente [A] — o Id do contexto", got)
		}
	})

	t.Run("index", func(t *testing.T) {
		f := newHistoryAxisFixture(t, historyAxisUser("A", 10))
		f.seedUser(t, "A", 10)
		f.seedHistoryRow(t, "A", historyChatA, "A-1", base)

		if rec := f.get(t, "/chats/history?chat_jid=index&user=B"); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
		}
		if got := f.reader.indexCalls; len(got) != 1 || got[0] != "A" {
			t.Fatalf("ChatIndexByUser recebeu %v, quero exatamente [A]", got)
		}
	})

	// A revalidacao do gate tambem le' por tenant: um `History` cacheado em 0
	// manda o use case perguntar a' tabela users, e a pergunta e' sobre o
	// chamador.
	t.Run("revalidacao do gate", func(t *testing.T) {
		f := newHistoryAxisFixture(t, historyAxisUser("A", 0))
		f.seedUser(t, "A", 10)
		f.seedHistoryRow(t, "A", historyChatA, "A-1", base)

		if rec := f.get(t, "/chats/history?chat_jid="+historyChatA); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
		}
		if got := f.reader.limitCalls; len(got) != 1 || got[0] != "A" {
			t.Fatalf("HistoryLimit recebeu %v, quero exatamente [A]", got)
		}
	})
}

// TestChatHistoryAxis_MessagesIsolateOnUserID e' o eixo 5 na sua forma de
// TENANT, no ramo que faltava.
//
// TestChatHistoryRoute_IndexIsolatesOnUserIDNotChatJID prova isto para
// `chat_jid=index`; o ramo de MENSAGENS — o caminho normal — nunca foi
// exercitado com dois tenants no MESMO chat_jid. E' precisamente a forma que
// um `WHERE user_id` esquecido no ListChatMessages deixaria passar: com
// chat_jid diferente por tenant, o filtro por chat ja' esconderia o defeito.
func TestChatHistoryAxis_MessagesIsolateOnUserID(t *testing.T) {
	const compartilhado = "mesmo@s.whatsapp.net"
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	f := newHistoryAxisFixture(t, historyAxisUser("A", 10))
	f.seedUser(t, "A", 10)
	f.seedUser(t, "B", 10)
	f.seedHistoryRow(t, "A", compartilhado, "A-1", base)
	f.seedHistoryRow(t, "B", compartilhado, "B-1", base.Add(time.Hour))
	f.seedHistoryRow(t, "B", compartilhado, "B-2", base.Add(2*time.Hour))

	rec := f.get(t, "/chats/history?chat_jid="+compartilhado)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	corpo := rec.Body.String()
	if strings.Contains(corpo, "B-1") || strings.Contains(corpo, "B-2") {
		t.Fatalf("mensagens do tenant B na resposta de A: %s", corpo)
	}

	var msgs []appport.ChatHistoryMessage
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &msgs); err != nil {
		t.Fatalf("data nao e' a lista de mensagens: %v (corpo: %s)", err, corpo)
	}
	// As de B sao MAIS NOVAS de proposito: a ordenacao e' timestamp DESC,
	// entao uma consulta sem o filtro por tenant traria as de B PRIMEIRO, e
	// um teste que so' checasse `len > 0` ou a primeira posicao passaria.
	if len(msgs) != 1 || msgs[0].MessageID != "A-1" {
		t.Fatalf("mensagens = %v, quero exatamente [A-1]", msgs)
	}
	if got := f.reader.listCalls; len(got) != 1 || got[0] != "A" {
		t.Fatalf("ListChatMessages recebeu %v, quero exatamente [A]", got)
	}
}

// --- eixo 2: nao se aplica, e a ausencia e' verificavel --------------------

// TestChatHistoryAxis_MalformedBodyIsIgnoredBecauseRouteIsGET e' o registro
// EXECUTAVEL de por que o eixo 2 nao entra na matriz desta capability: o
// handler nunca le' r.Body, entao um corpo ilegivel numa requisicao valida e'
// simplesmente ignorado, e nao 400.
//
// Ele existe para que a decisao caia junto com a premissa: se algum dia o
// handler passar a decodificar corpo, este teste falha e o eixo 2 volta a ser
// obrigatorio aqui.
func TestChatHistoryAxis_MalformedBodyIsIgnoredBecauseRouteIsGET(t *testing.T) {
	f := newHistoryAxisFixture(t, historyAxisUser("A", 10))
	f.seedUser(t, "A", 10)
	f.seedHistoryRow(t, "A", historyChatA, "A-1", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chats/history?chat_jid="+historyChatA,
		strings.NewReader(`{"Phone":"5511`))
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200: o handler de /chats/history nao le' corpo, "+
			"entao um corpo truncado nao pode mudar o desfecho (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(f.reader.listCalls); n != 1 {
		t.Fatalf("o historico foi consultado %d vez(es), quero 1", n)
	}
}
