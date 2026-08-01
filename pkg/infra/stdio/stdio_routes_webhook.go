package stdio

// Rotas de webhook (`webhook.*`): o CRUD do webhook da sessão, quatro verbos
// HTTP sobre o mesmo caminho.

var webhookStaticRoutes = map[string]staticRoute{
	"webhook.get":    {httpMethod: "GET", httpPath: "/webhook"},
	"webhook.set":    {httpMethod: "POST", httpPath: "/webhook"},
	"webhook.update": {httpMethod: "PUT", httpPath: "/webhook"},
	"webhook.delete": {httpMethod: "DELETE", httpPath: "/webhook"},
}
