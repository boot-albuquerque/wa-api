package bootstrap

import (
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"

	stdiopkg "wa-api/pkg/infra/stdio"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/handlers"
)

// F99: o stdio declarava POST para /session/connect e /session/disconnect,
// que o HTTP registra como GET. stdio.go monta a requisição com
// httptest.NewRequest(httpMethod, httpPath, body) e a executa contra o MESMO
// mux — que aplica route.Methods(...) —, então o método divergente nunca chega
// ao handler. As duas operações mais básicas do ciclo de sessão falhavam.
//
// O defeito não é interessante; o que é interessante é que NADA comparava as
// duas tabelas. Elas eram mantidas à mão, em arquivos distintos, e divergir era
// invisível até alguém exercitar o caminho stdio. Este teste é esse mecanismo:
// percorre a tabela do stdio e pergunta ao roteador real se aquele par
// (método, caminho) casa.
//
// Deliberadamente NÃO é uma lista de rotas esperadas. Uma lista seria uma
// TERCEIRA tabela a manter, e a próxima rota nova entraria divergente nas três.
func TestStdioRoutesMatchRegisteredHTTPRoutes(t *testing.T) {
	router := newRouterForRouteCheck()

	targets := stdiopkg.StaticRouteTargets()
	if len(targets) == 0 {
		t.Fatal("a tabela estática do stdio veio vazia; o teste não estaria conferindo nada")
	}

	for rpcMethod, target := range targets {
		req := httptest.NewRequest(target.Method, target.Path, nil)

		var match mux.RouteMatch
		if router.Match(req, &match) {
			continue
		}

		// A distinção importa para quem for consertar: método divergente é uma
		// linha numa das duas tabelas; caminho inexistente é rota que ninguém
		// registrou.
		if match.MatchErr == mux.ErrMethodMismatch {
			t.Errorf("%s: o stdio despacha %s %s, mas o caminho está registrado com OUTRO método; o mux recusa antes do handler",
				rpcMethod, target.Method, target.Path)
			continue
		}
		t.Errorf("%s: o stdio despacha %s %s, e não existe rota HTTP para esse caminho",
			rpcMethod, target.Method, target.Path)
	}
}

// TestRegisteredHTTPRoutesHaveStdioEntry is the INVERSE of the test above:
// it walks every route registered in the real HTTP router and requires that
// each (method, path) pair is covered by the stdio route table, listed as a
// structural exception, or tracked as a known pending gap.
//
// A route that appears in NONE of the three lists fails the test and blocks
// the build. This prevents new HTTP routes from silently lacking stdio
// support — the exact blind spot that let 28 routes slip through (F173).
//
// The pending list tracks historical gaps and must SHRINK as routes gain
// stdio entries (F173 part 2). It must NEVER grow: a new route that lacks
// stdio support must either get an entry or a structural exception.
func TestRegisteredHTTPRoutesHaveStdioEntry(t *testing.T) {
	router := newRouterForRouteCheck()

	// One of the two existing sources: the stdio static route table.
	staticTargets := stdiopkg.StaticRouteTargets()
	coveredByStdio := make(map[string]bool, len(staticTargets))
	for _, target := range staticTargets {
		coveredByStdio[target.Method+" "+target.Path] = true
	}

	// Structural exceptions: routes that CANNOT have stdio static entries.
	// Each reason must be structural — "not implemented yet" is not a
	// valid reason and belongs in knownPending instead.
	//
	// Prose-only exceptions have an empty dynamicRPC field; the reason
	// alone justifies the exemption. Dynamic-route exceptions name the
	// RPC method that covers them, and the test verifies it exists with
	// the expected HTTP method — so the exception becomes orphaned (and
	// the test fails) if the dynamic route is ever removed.
	type exception struct {
		reason     string
		dynamicRPC string
		httpMethod string
	}
	structuralExceptions := map[string]exception{
		// WebSocket upgrade requires a persistent bidirectional connection;
		// the stdio transport is request/response and cannot represent it.
		"GET /session/ws": {reason: "WebSocket upgrade; incompatible with request/response stdio transport"},

		// Path-parameter routes dispatched by stdio DYNAMIC routes.
		// The static table cannot express parameterized paths; these are
		// served by buildPath functions in the stdio_routes_*.go files.
		"GET /user/lid/{jid}":           {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "user.lid", httpMethod: "GET"},
		"GET /admin/users/{id}":         {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "admin.users.get", httpMethod: "GET"},
		"PUT /admin/users/{id}":         {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "admin.users.edit", httpMethod: "PUT"},
		"DELETE /admin/users/{id}":      {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "admin.users.delete", httpMethod: "DELETE"},
		"DELETE /admin/users/{id}/full": {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "admin.users.delete.full", httpMethod: "DELETE"},
	}

	dynamicTargets := stdiopkg.DynamicRouteTargets()
	for routeKey, exc := range structuralExceptions {
		if exc.dynamicRPC == "" {
			continue
		}
		gotMethod, exists := dynamicTargets[exc.dynamicRPC]
		if !exists {
			t.Errorf("structural exception %q cites dynamic route %q, but it does not exist in the stdio dynamic table — exception is orphaned",
				routeKey, exc.dynamicRPC)
			continue
		}
		if gotMethod != exc.httpMethod {
			t.Errorf("structural exception %q expects dynamic route %q with HTTP method %s, but the dynamic table has %s",
				routeKey, exc.dynamicRPC, exc.httpMethod, gotMethod)
		}
	}

	// Known gaps awaiting contract decision (F173 part 2).
	// This list must SHRINK as routes gain stdio entries — NEVER grow.
	// Adding a route here means acknowledging a KNOWN omission, not
	// granting a permanent exemption.
	knownPending := map[string]bool{
		"GET /chat/list":             true,
		"POST /chat/delete/message":  true,
		"POST /chat/downloadsticker": true,
		"POST /chat/send/template":   true,

		"GET /group/requestparticipants":        true,
		"POST /group/updaterequestparticipants": true,
		"POST /group/joinapprovalmode":          true,

		"POST /s3/configure":        true,
		"GET /s3/config":            true,
		"DELETE /s3/config":         true,
		"POST /s3/test":             true,
		"POST /session/s3/config":   true,
		"GET /session/s3/config":    true,
		"DELETE /session/s3/config": true,
		"POST /session/s3/test":     true,

		"POST /hmac/configure": true,
		"GET /hmac/config":     true,
		"DELETE /hmac/config":  true,

		"POST /proxy/set":       true,
		"POST /webhook/history": true,
		"GET /webhook/history":  true,

		"GET /user/blocklist":              true,
		"POST /user/presence/subscribe":    true,
		"GET /user/contacts/last-activity": true,
		"GET /user/privacy":                true,
		"POST /user/privacy":               true,
		"POST /user/status":                true,
		"POST /user/history/sync":          true,
		"POST /user/contacts/sync":         true,
		"GET /user/profile/{jid}":          true,

		"GET /session/profile":      true,
		"GET /session/profile/full": true,
	}

	type missing struct{ method, path string }
	var uncovered []missing

	err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		methods, err := route.GetMethods()
		if err != nil {
			return nil
		}
		pathTpl, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		for _, m := range methods {
			key := m + " " + pathTpl
			if coveredByStdio[key] || knownPending[key] {
				continue
			}
			if _, ok := structuralExceptions[key]; ok {
				continue
			}
			uncovered = append(uncovered, missing{method: m, path: pathTpl})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("router.Walk failed: %v", err)
	}

	sort.Slice(uncovered, func(i, j int) bool {
		if uncovered[i].path != uncovered[j].path {
			return uncovered[i].path < uncovered[j].path
		}
		return uncovered[i].method < uncovered[j].method
	})

	for _, r := range uncovered {
		t.Errorf("registered HTTP route %s %s has no stdio entry and is not in any known list.\n"+
			"  Two valid options:\n"+
			"  (1) add an RPC method to the stdio route table (public contract addition), or\n"+
			"  (2) add it to structuralExceptions with a structural justification.\n"+
			"  Do NOT add it to knownPending — that list tracks historical gaps and must shrink, not grow.",
			r.method, r.path)
	}
}

// newRouterForRouteCheck monta o roteador REAL de rotas custom.
//
// Os handlers são zero-value de propósito: o que está sob teste é o casamento
// de método e caminho, que o mux decide antes de chamar handler nenhum. Um
// handler falso responderia a mesma coisa e exigiria manutenção.
//
// Os ponteiros de grupo precisam existir (registerCustomRoutes lê campos deles),
// mas os http.Handler dentro podem ser nil.
func newRouterForRouteCheck() *mux.Router {
	ch := &customHandlers{
		Profile:     &customhttp.ProfileHandler{},
		ProfileFull: &customhttp.ProfileFullHandler{},
		Message:     &MessageHandlers{},
		Session:     &SessionHandlers{},
		Webhook:     &WebhookHandlers{},
		User:        &handlers.UserHandlers{},
		Group:       &handlers.GroupHandlers{},
		Storage:     &handlers.StorageHandlers{},
		Misc:        &handlers.MiscHandlers{},
		Blocklist:   &handlers.BlocklistHandlers{},
		Download:    &handlers.DownloadHandlers{},
		Presence:    &handlers.PresenceHandlers{},
		Reaction:    &handlers.ReactionHandlers{},
		Contact:     &handlers.ContactHandlers{},
		GroupMgmt:   &handlers.GroupManagementHandlers{},
		ChatHistory: &handlers.ChatHistoryHandlers{},
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(), ch)

	// /admin/* não passa por registerCustomRoutes: vive num subrouter próprio,
	// com authAdmin em vez da chain de usuário. Sem esta linha o teste
	// reportaria admin.users.list e admin.users.add como "rota inexistente" —
	// um falso positivo que ensinaria a ignorar o teste.
	registerAdminRoutes(router.PathPrefix("/admin").Subrouter(), ch)

	return router
}
