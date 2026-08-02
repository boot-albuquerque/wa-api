package stdio

import "github.com/rs/zerolog/log"

// Rotas de usuário do WhatsApp (`user.*`): contatos, presença, avatar,
// bloqueio e resolução de LID.

var userStaticRoutes = map[string]staticRoute{
	"user.contacts": {httpMethod: "GET", httpPath: "/user/contacts"},
	"user.presence": {httpMethod: "POST", httpPath: "/user/presence"},
	"user.info":     {httpMethod: "POST", httpPath: "/user/info"},
	"user.check":    {httpMethod: "POST", httpPath: "/user/check"},
	"user.avatar":   {httpMethod: "POST", httpPath: "/user/avatar"},
	"user.block":    {httpMethod: "POST", httpPath: "/user/block"},
	"user.unblock":  {httpMethod: "POST", httpPath: "/user/unblock"},
}

var userDynamicRoutes = map[string]dynamicRoute{
	"user.lid": {httpMethod: "GET", buildPath: userLidPath},
}

func userLidPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	jid, ok := ss.stringParam(req, "jid")
	if !ok {
		return "", false
	}
	httpPath := "/user/lid/" + jid
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de usuario resolvida")
	return httpPath, true
}
