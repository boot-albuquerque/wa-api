package stdio

import "github.com/rs/zerolog/log"

// Rotas de usuário do WhatsApp (`user.*`): contatos, presença, avatar,
// bloqueio e resolução de LID.

var userStaticRoutes = map[string]staticRoute{
	"user.contacts": {httpMethod: "GET", httpPath: "/users/contacts"},
	"user.presence": {httpMethod: "POST", httpPath: "/users/presence"},
	"user.info":     {httpMethod: "POST", httpPath: "/users/info"},
	"user.check":    {httpMethod: "POST", httpPath: "/users/check"},
	"user.avatar":   {httpMethod: "POST", httpPath: "/users/avatar"},
	"user.block":    {httpMethod: "POST", httpPath: "/users/block"},
	"user.unblock":  {httpMethod: "POST", httpPath: "/users/unblock"},
}

var userDynamicRoutes = map[string]dynamicRoute{
	"user.lid": {httpMethod: "GET", buildPath: userLidPath},
}

func userLidPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	jid, ok := ss.stringParam(req, "jid")
	if !ok {
		return "", false
	}
	httpPath := "/users/lid/" + jid
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de usuario resolvida")
	return httpPath, true
}
