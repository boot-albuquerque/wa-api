package stdio

// Métodos avulsos: cada um é o único do seu domínio, e um arquivo por método
// seria pior do que juntá-los aqui.

var miscStaticRoutes = map[string]staticRoute{
	"health":           {httpMethod: "GET", httpPath: "/health"},
	"status.set.image": {httpMethod: "POST", httpPath: "/status/set/image"},
	"status.set.video": {httpMethod: "POST", httpPath: "/status/set/video"},
	"status.set.audio": {httpMethod: "POST", httpPath: "/status/set/audio"},
	"call.reject":      {httpMethod: "POST", httpPath: "/call/reject"},
	"newsletter.list":  {httpMethod: "GET", httpPath: "/newsletter/list"},

	// As onze operações de newsletter acrescentadas em 2026-08-20. Ficam aqui,
	// e não num ficheiro próprio, porque partilham o use case e a forma do
	// pedido: separá-las daria a impressão de domínios distintos.
	"newsletter.create":       {httpMethod: "POST", httpPath: "/newsletter/create"},
	"newsletter.info":         {httpMethod: "POST", httpPath: "/newsletter/info"},
	"newsletter.info.invite":  {httpMethod: "POST", httpPath: "/newsletter/info-invite"},
	"newsletter.follow":       {httpMethod: "POST", httpPath: "/newsletter/follow"},
	"newsletter.unfollow":     {httpMethod: "POST", httpPath: "/newsletter/unfollow"},
	"newsletter.mute":         {httpMethod: "POST", httpPath: "/newsletter/mute"},
	"newsletter.messages":     {httpMethod: "POST", httpPath: "/newsletter/messages"},
	"newsletter.updates":      {httpMethod: "POST", httpPath: "/newsletter/updates"},
	"newsletter.mark.viewed":  {httpMethod: "POST", httpPath: "/newsletter/mark-viewed"},
	"newsletter.react":        {httpMethod: "POST", httpPath: "/newsletter/react"},
	"newsletter.subscribe":    {httpMethod: "POST", httpPath: "/newsletter/subscribe"},
	"newsletter.demote":       {httpMethod: "POST", httpPath: "/newsletter/demote"},
	"newsletter.change.owner": {httpMethod: "POST", httpPath: "/newsletter/change-owner"},
	"newsletter.delete":       {httpMethod: "DELETE", httpPath: "/newsletter/delete"},

	// F191: leitura de etiquetas. Não há método de escrita porque a
	// biblioteca não sabe criá-las (LIB-01).
	"label.list":       {httpMethod: "GET", httpPath: "/labels"},
	"label.list.chats": {httpMethod: "GET", httpPath: "/labels/{id}/chats"},
}
