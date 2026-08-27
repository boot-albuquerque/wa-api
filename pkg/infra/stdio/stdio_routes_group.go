package stdio

import "github.com/rs/zerolog/log"

// Rotas de grupo (`group.*`): criação, metadados, foto, convites e
// participantes.

var groupStaticRoutes = map[string]staticRoute{
	"group.list":   {httpMethod: "POST", httpPath: "/groups/list"},
	"group.create": {httpMethod: "POST", httpPath: "/groups/create"},
	"group.leave":  {httpMethod: "POST", httpPath: "/groups/leave"},
	"group.join":   {httpMethod: "POST", httpPath: "/groups/join"},
}

// groupDynamicRoutes: métodos cujo caminho HTTP canónico carrega o group_jid
// (ou o invite_code) no PRÓPRIO caminho — ver pkg/bootstrap/wiring_routes.go
// e a política de "/" para hierarquia de recurso em CLAUDE.md.
//
// group.photo, group.photo.remove e group.updateparticipants entraram aqui
// na reversão de F269/CAP-10 (2026-08-27): tinham group_jid só no corpo,
// passaram a tê-lo também na RELAÇÃO do caminho — ver groupJIDParam abaixo,
// que tenta as grafias que o cliente já usava antes desta mudança
// (GroupJID/groupJID/groupjid/group_jid), para continuar a funcionar sem
// pedir ao cliente stdio para migrar já.
//
// Os restantes (info/invitelink/inviteinfo/name/topic/announce/locked/
// ephemeral) usam só a grafia `groupJID` porque foi essa a única que o
// handler HTTP alguma vez leu nestas rotas — ver ss.stringParam abaixo.
var groupDynamicRoutes = map[string]dynamicRoute{
	"group.info":               {httpMethod: "GET", buildPath: groupInfoPath},
	"group.invitelink":         {httpMethod: "GET", buildPath: groupInviteLinkPath},
	"group.inviteinfo":         {httpMethod: "GET", buildPath: groupInviteInfoPath},
	"group.name":               {httpMethod: "PUT", buildPath: groupNamePath},
	"group.topic":              {httpMethod: "PUT", buildPath: groupTopicPath},
	"group.announce":           {httpMethod: "PUT", buildPath: groupAnnouncePath},
	"group.locked":             {httpMethod: "PUT", buildPath: groupLockedPath},
	"group.ephemeral":          {httpMethod: "PUT", buildPath: groupEphemeralPath},
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

func groupInfoPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	groupJID, ok := ss.stringParam(req, "groupJID")
	if !ok {
		return "", false
	}
	httpPath := "/groups/" + groupJID
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de grupo resolvida")
	return httpPath, true
}

func groupInviteLinkPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	groupJID, ok := ss.stringParam(req, "groupJID")
	if !ok {
		return "", false
	}
	httpPath := "/groups/" + groupJID + "/invite-link"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de grupo resolvida")
	return httpPath, true
}

func groupInviteInfoPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	code, ok := ss.stringParam(req, "Code")
	if !ok {
		return "", false
	}
	httpPath := "/groups/invite-links/" + code
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de grupo resolvida")
	return httpPath, true
}

func groupNamePath(ss *Server, req *JSONRpcRequest) (string, bool) {
	groupJID, ok := ss.stringParam(req, "groupJID")
	if !ok {
		return "", false
	}
	httpPath := "/groups/" + groupJID + "/name"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de grupo resolvida")
	return httpPath, true
}

func groupTopicPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	groupJID, ok := ss.stringParam(req, "groupJID")
	if !ok {
		return "", false
	}
	httpPath := "/groups/" + groupJID + "/topic"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de grupo resolvida")
	return httpPath, true
}

func groupAnnouncePath(ss *Server, req *JSONRpcRequest) (string, bool) {
	groupJID, ok := ss.stringParam(req, "groupJID")
	if !ok {
		return "", false
	}
	httpPath := "/groups/" + groupJID + "/announce-only"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de grupo resolvida")
	return httpPath, true
}

func groupLockedPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	groupJID, ok := ss.stringParam(req, "groupJID")
	if !ok {
		return "", false
	}
	httpPath := "/groups/" + groupJID + "/locked"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de grupo resolvida")
	return httpPath, true
}

func groupEphemeralPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	groupJID, ok := ss.stringParam(req, "groupJID")
	if !ok {
		return "", false
	}
	httpPath := "/groups/" + groupJID + "/ephemeral"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de grupo resolvida")
	return httpPath, true
}
