package stdio

import "github.com/rs/zerolog/log"

// Rotas de grupo (`group.*`): criação, metadados, foto, convites e
// participantes.

var groupStaticRoutes = map[string]staticRoute{
	"group.list":   {httpMethod: "POST", httpPath: "/group/list"},
	"group.create": {httpMethod: "POST", httpPath: "/group/create"},

	"group.photo":        {httpMethod: "POST", httpPath: "/group/photo"},
	"group.photo.remove": {httpMethod: "POST", httpPath: "/group/photo/remove"},

	"group.leave": {httpMethod: "POST", httpPath: "/group/leave"},

	"group.join":               {httpMethod: "POST", httpPath: "/group/join"},
	"group.updateparticipants": {httpMethod: "POST", httpPath: "/group/updateparticipants"},
}

// groupDynamicRoutes: métodos cujo caminho HTTP canónico carrega o group_jid
// (ou o invite_code) no PRÓPRIO caminho — ver pkg/bootstrap/wiring_routes.go
// e a política de "/" para hierarquia de recurso em CLAUDE.md. O nome do
// param RPC é o mesmo campo que o corpo já usava (groupJID/Code), porque o
// stdio continua a enviar TODOS os params como corpo — só o path muda.
var groupDynamicRoutes = map[string]dynamicRoute{
	"group.info":       {httpMethod: "GET", buildPath: groupInfoPath},
	"group.invitelink": {httpMethod: "GET", buildPath: groupInviteLinkPath},
	"group.inviteinfo": {httpMethod: "GET", buildPath: groupInviteInfoPath},
	"group.name":       {httpMethod: "PUT", buildPath: groupNamePath},
	"group.topic":      {httpMethod: "PUT", buildPath: groupTopicPath},
	"group.announce":   {httpMethod: "PUT", buildPath: groupAnnouncePath},
	"group.locked":     {httpMethod: "PUT", buildPath: groupLockedPath},
	"group.ephemeral":  {httpMethod: "PUT", buildPath: groupEphemeralPath},
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
