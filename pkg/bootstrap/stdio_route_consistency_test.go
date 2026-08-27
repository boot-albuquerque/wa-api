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

		// F251: POST /s3/config and POST /hmac/config are aliases of
		// /s3/configure and /hmac/configure respectively — same handler,
		// shorter path. Whichever stdio entry covers /configure covers /config.
		"POST /s3/config":   {reason: "alias of POST /s3/configure (F251); same handler, same RPC method"},
		"POST /hmac/config": {reason: "alias of POST /hmac/configure (F251); same handler, same RPC method"},

		// Path-parameter routes dispatched by stdio DYNAMIC routes.
		// The static table cannot express parameterized paths; these are
		// served by buildPath functions in the stdio_routes_*.go files.
		// F237: community routes are HTTP-only for now; stdio entries will be
		// added when the stdio transport gains community support.
		"POST /community/subgroups":    {reason: "F237: new community route; stdio entry deferred"},
		"POST /community/participants": {reason: "F237: new community route; stdio entry deferred"},
		"POST /community/link":         {reason: "F237: new community route; stdio entry deferred"},
		"POST /community/unlink":       {reason: "F237: new community route; stdio entry deferred"},

		"POST /chats/download/{kind}": {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "chat.download.media", httpMethod: "POST"},
		"GET /user/lid/{jid}":         {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "user.lid", httpMethod: "GET"},

		// Rotas de caminho concatenado corrigidas nesta sessão (worktree
		// http-dto-paths): o group_jid/chat_jid/poll_message_id/invite_code
		// passou do corpo para o caminho, e o stdio ganhou rota dinâmica
		// correspondente — ver pkg/infra/stdio/stdio_routes_group.go e
		// stdio_routes_chat.go.
		"GET /groups/{group_jid}":                {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.info", httpMethod: "GET"},
		"GET /groups/{group_jid}/invite-link":    {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.invitelink", httpMethod: "GET"},
		"GET /groups/invite-links/{invite_code}": {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.inviteinfo", httpMethod: "GET"},
		"PUT /groups/{group_jid}/name":           {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.name", httpMethod: "PUT"},
		"PUT /groups/{group_jid}/topic":          {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.topic", httpMethod: "PUT"},
		"PUT /groups/{group_jid}/announce-only":  {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.announce", httpMethod: "PUT"},
		"PUT /groups/{group_jid}/locked":         {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.locked", httpMethod: "PUT"},
		"PUT /groups/{group_jid}/ephemeral":      {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.ephemeral", httpMethod: "PUT"},
		"POST /chats/{chat_jid}/read":            {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "chat.markread", httpMethod: "POST"},
		"POST /polls/{poll_message_id}/votes":    {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "chat.send.pollvote", httpMethod: "POST"},
		"GET /admin/users/{id}":                  {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "admin.users.get", httpMethod: "GET"},
		"PUT /admin/users/{id}":                  {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "admin.users.edit", httpMethod: "PUT"},
		"DELETE /admin/users/{id}":               {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "admin.users.delete", httpMethod: "DELETE"},
		"DELETE /admin/users/{id}/full":          {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "admin.users.delete.full", httpMethod: "DELETE"},

		// F269/CAP-10 reversão (2026-08-27): group.photo, group.photo.remove e
		// group.updateparticipants deixaram de ser rotas estáticas de stdio
		// porque o caminho canónico carrega o group_jid na RELAÇÃO — viraram
		// dynamicRoute (ver pkg/infra/stdio/stdio_routes_group.go).
		"PUT /groups/{group_jid}/photo":         {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.photo", httpMethod: "PUT"},
		"DELETE /groups/{group_jid}/photo":      {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.photo.remove", httpMethod: "DELETE"},
		"POST /groups/{group_jid}/participants": {reason: "path parameter; dispatched by stdio dynamic route", dynamicRPC: "group.updateparticipants", httpMethod: "POST"},
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
		"GET /chats/list":           true,
		"POST /chats/send/template": true,

		"GET /groups/{group_jid}/join-requests":          true,
		"POST /groups/{group_jid}/join-requests":         true,
		"PUT /groups/{group_jid}/settings/join-approval": true,

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

		"GET /users/blocklist":              true,
		"POST /users/presence/subscribe":    true,
		"GET /users/contacts/last-activity": true,
		"GET /users/privacy":                true,
		"POST /users/privacy":               true,
		"POST /users/status":                true,
		"POST /users/history/sync":          true,
		"POST /users/contacts/sync":         true,
		"GET /users/profile/{jid}":          true,

		"GET /session/profile":      true,
		"GET /session/profile/full": true,
	}

	// origemLegada mapeia cada caminho canónico de volta à rota antiga que ele
	// substitui, para que a cobertura stdio de uma valha pela outra.
	origemLegada := map[string]string{}
	for _, linha := range CaminhosCanonicos() {
		origemLegada[linha.CanonicalMethod+" "+linha.CanonicalPath] =
			linha.LegacyMethod + " " + linha.LegacyPath
	}

	type missing struct{ method, path string }
	var uncovered []missing
	routerKeys := make(map[string]bool)

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
			routerKeys[key] = true
			if coveredByStdio[key] || knownPending[key] {
				continue
			}
			if _, ok := structuralExceptions[key]; ok {
				continue
			}
			// FORMA CANÓNICA (F269): um caminho canónico é a MESMA rota, com o
			// mesmo manipulador, servida noutro caminho. O método RPC que cobre
			// a rota antiga cobre-o também — dar-lhe RPC próprio criaria dois
			// nomes para uma operação, que é precisamente o que a padronização
			// existe para eliminar.
			//
			// Derivado da tabela em vez de listado: noventa e uma excepções à
			// mão seriam noventa e uma linhas a desactualizar-se em silêncio.
			if legada, ehCanonica := origemLegada[key]; ehCanonica {
				if coveredByStdio[legada] || knownPending[legada] {
					continue
				}
				if _, ok := structuralExceptions[legada]; ok {
					continue
				}
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

	// Self-validation: knownPending entries that no longer exist in the
	// router are ghost entries — the route was removed but the pending
	// entry stayed. Catch them so the list doesn't silently grow stale.
	for key := range knownPending {
		if !routerKeys[key] {
			t.Errorf("knownPending entry %q no longer exists in the HTTP router — remove it", key)
		}
	}

	// Self-validation: a knownPending entry that is ALSO covered by
	// stdio means the gap was resolved but nobody cleaned the list.
	for key := range knownPending {
		if coveredByStdio[key] {
			t.Errorf("knownPending entry %q is now covered by the stdio table — remove it from knownPending", key)
		}
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
		Community:   &handlers.CommunityHandlers{},
		Newsletter:  &handlers.NewsletterHandlers{},
		Label:       &handlers.LabelHandlers{},
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
