package stdio

// Rotas de sessão (`session.*`): conexão, QR, proxy e configuração de HMAC.

var sessionStaticRoutes = map[string]staticRoute{
	"session.connect":            {httpMethod: "POST", httpPath: "/session/connect"},
	"session.qr":                 {httpMethod: "GET", httpPath: "/session/qr"},
	"session.status":             {httpMethod: "GET", httpPath: "/session/status"},
	"session.disconnect":         {httpMethod: "POST", httpPath: "/session/disconnect"},
	"session.logout":             {httpMethod: "POST", httpPath: "/session/logout"},
	"session.pairphone":          {httpMethod: "POST", httpPath: "/session/pairphone"},
	"session.history":            {httpMethod: "GET", httpPath: "/session/history"},
	"session.history.set":        {httpMethod: "POST", httpPath: "/session/history"},
	"session.proxy":              {httpMethod: "POST", httpPath: "/session/proxy"},
	"session.hmac.config":        {httpMethod: "POST", httpPath: "/session/hmac/config"},
	"session.hmac.config.get":    {httpMethod: "GET", httpPath: "/session/hmac/config"},
	"session.hmac.config.delete": {httpMethod: "DELETE", httpPath: "/session/hmac/config"},
}
