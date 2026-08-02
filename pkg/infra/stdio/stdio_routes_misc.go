package stdio

// Métodos avulsos: cada um é o único do seu domínio, e um arquivo por método
// seria pior do que juntá-los aqui.

var miscStaticRoutes = map[string]staticRoute{
	"health":          {httpMethod: "GET", httpPath: "/health"},
	"status.set.text": {httpMethod: "POST", httpPath: "/status/set/text"},
	"call.reject":     {httpMethod: "POST", httpPath: "/call/reject"},
	"newsletter.list": {httpMethod: "GET", httpPath: "/newsletter/list"},
}
