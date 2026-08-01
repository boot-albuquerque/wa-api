package stdio

import "fmt"

// Tabela de rotas JSON-RPC → HTTP.
//
// Cada grupo de métodos (`admin.*`, `session.*`, `chat.*`, ...) declara a sua
// própria tabela no arquivo `stdio_routes_<grupo>.go`. Este arquivo só junta as
// tabelas e faz o despacho — a lógica de roteamento não cresce quando um método
// novo é acrescentado.

// staticRoute é um método JSON-RPC cujo caminho HTTP é fixo.
type staticRoute struct {
	httpMethod string
	httpPath   string
}

// dynamicRoute é um método JSON-RPC cujo caminho HTTP depende dos params da
// requisição. buildPath devolve (path, false) quando o param obrigatório falta;
// nesse caso o erro JSON-RPC já foi enviado ao cliente e o despacho para.
type dynamicRoute struct {
	httpMethod string
	buildPath  func(ss *Server, req *JSONRpcRequest) (string, bool)
}

// staticRouteGroups / dynamicRouteGroups listam as tabelas por grupo. Um grupo
// novo entra aqui e em nenhum outro lugar.
var staticRouteGroups = []map[string]staticRoute{
	adminStaticRoutes,
	sessionStaticRoutes,
	chatStaticRoutes,
	userStaticRoutes,
}

var dynamicRouteGroups = []map[string]dynamicRoute{
	adminDynamicRoutes,
	chatDynamicRoutes,
	userDynamicRoutes,
}

var staticRoutes = mergeStaticRoutes(staticRouteGroups)

var dynamicRoutes = mergeDynamicRoutes(dynamicRouteGroups)

func mergeStaticRoutes(groups []map[string]staticRoute) map[string]staticRoute {
	all := make(map[string]staticRoute)
	for _, group := range groups {
		for method, route := range group {
			all[method] = route
		}
	}
	return all
}

func mergeDynamicRoutes(groups []map[string]dynamicRoute) map[string]dynamicRoute {
	all := make(map[string]dynamicRoute)
	for _, group := range groups {
		for method, route := range group {
			all[method] = route
		}
	}
	return all
}

// routeRequest despacha a requisição para o handler HTTP correspondente.
func (ss *Server) routeRequest(req *JSONRpcRequest) {
	if route, ok := staticRoutes[req.Method]; ok {
		ss.executeHTTPHandler(req, route.httpMethod, route.httpPath)
		return
	}
	if route, ok := dynamicRoutes[req.Method]; ok {
		httpPath, ok := route.buildPath(ss, req)
		if !ok {
			// Erro já enviado por buildPath.
			return
		}
		ss.executeHTTPHandler(req, route.httpMethod, httpPath)
		return
	}
	// Grupos ainda não migrados para a tabela continuam no switch legado.
	ss.routeRequestLegacySwitch(req)
}

// stringParam extrai um param string obrigatório, enviando o erro JSON-RPC
// padrão quando ele falta ou está vazio.
func (ss *Server) stringParam(req *JSONRpcRequest, name string) (string, bool) {
	value, ok := req.Params[name].(string)
	if !ok || value == "" {
		ss.sendError(req.ID, 400, fmt.Sprintf("missing or invalid %s parameter", name))
		return "", false
	}
	return value, true
}
