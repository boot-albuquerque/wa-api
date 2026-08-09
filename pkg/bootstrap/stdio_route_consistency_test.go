package bootstrap

import (
	"net/http/httptest"
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
