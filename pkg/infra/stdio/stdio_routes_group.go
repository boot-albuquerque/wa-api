package stdio

import "github.com/rs/zerolog/log"

// Rotas de grupo (`group.*`): criação, metadados, foto, convites e
// participantes.

var groupStaticRoutes = map[string]staticRoute{
	"group.list":       {httpMethod: "POST", httpPath: "/groups/list"},
	"group.create":     {httpMethod: "POST", httpPath: "/groups/create"},
	"group.info":       {httpMethod: "POST", httpPath: "/groups/info"},
	"group.invitelink": {httpMethod: "POST", httpPath: "/groups/invitelink"},

	"group.leave":     {httpMethod: "POST", httpPath: "/groups/leave"},
	"group.name":      {httpMethod: "POST", httpPath: "/groups/name"},
	"group.topic":     {httpMethod: "POST", httpPath: "/groups/topic"},
	"group.announce":  {httpMethod: "POST", httpPath: "/groups/announce"},
	"group.locked":    {httpMethod: "POST", httpPath: "/groups/locked"},
	"group.ephemeral": {httpMethod: "POST", httpPath: "/groups/ephemeral"},

	"group.join":       {httpMethod: "POST", httpPath: "/groups/join"},
	"group.inviteinfo": {httpMethod: "POST", httpPath: "/groups/inviteinfo"},
}

// group.photo, group.photo.remove e group.updateparticipants viraram rotas
// canónicas com o group_jid na RELAÇÃO do caminho, não mais no corpo apenas
// (reversão F269/CAP-10, 2026-08-27). Passam de staticRoute a dynamicRoute
// porque o caminho HTTP já não é fixo — precisa do group_jid extraído do
// pedido JSON-RPC antes de montar a URL.
var groupDynamicRoutes = map[string]dynamicRoute{
	"group.photo":              {httpMethod: "PUT", buildPath: groupPhotoPath},
	"group.photo.remove":       {httpMethod: "DELETE", buildPath: groupPhotoPath},
	"group.updateparticipants": {httpMethod: "POST", buildPath: groupParticipantsPath},
}

// groupJIDParam extrai o group_jid do pedido JSON-RPC tentando as grafias que
// os manipuladores HTTP já aceitam (case-insensível no corpo, mas req.Params
// aqui é um mapa cru — a grafia que o cliente já usa hoje, ANTES desta
// mudança, continua a funcionar).
func groupJIDParam(ss *Server, req *JSONRpcRequest) (string, bool) {
	for _, nome := range []string{"GroupJID", "groupJID", "groupjid", "group_jid"} {
		if valor, ok := req.Params[nome].(string); ok && valor != "" {
			return valor, true
		}
	}
	ss.sendError(req.ID, 400, "missing or invalid GroupJID parameter")
	return "", false
}

func groupPhotoPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	jid, ok := groupJIDParam(ss, req)
	if !ok {
		return "", false
	}
	httpPath := "/groups/" + jid + "/photo"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de foto de grupo resolvida")
	return httpPath, true
}

func groupParticipantsPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	jid, ok := groupJIDParam(ss, req)
	if !ok {
		return "", false
	}
	httpPath := "/groups/" + jid + "/participants"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de participantes de grupo resolvida")
	return httpPath, true
}
