package stdio

// Métodos avulsos: cada um é o único do seu domínio, e um arquivo por método
// seria pior do que juntá-los aqui.

var miscStaticRoutes = map[string]staticRoute{
	"health":           {httpMethod: "GET", httpPath: "/health"},
	"status.set.image": {httpMethod: "POST", httpPath: "/status/set/image"},
	"status.set.video": {httpMethod: "POST", httpPath: "/status/set/video"},
	"status.set.audio": {httpMethod: "POST", httpPath: "/status/set/audio"},
	"call.reject":      {httpMethod: "POST", httpPath: "/call/reject"},
	"newsletter.list":  {httpMethod: "GET", httpPath: "/newsletters/list"},

	// As onze operações de newsletter acrescentadas em 2026-08-20. Ficam aqui,
	// e não num ficheiro próprio, porque partilham o use case e a forma do
	// pedido: separá-las daria a impressão de domínios distintos.
	"newsletter.create":       {httpMethod: "POST", httpPath: "/newsletters/create"},
	"newsletter.info":         {httpMethod: "POST", httpPath: "/newsletters/info"},
	"newsletter.info.invite":  {httpMethod: "POST", httpPath: "/newsletters/info-invite"},
	"newsletter.follow":       {httpMethod: "POST", httpPath: "/newsletters/follow"},
	"newsletter.unfollow":     {httpMethod: "POST", httpPath: "/newsletters/unfollow"},
	"newsletter.mute":         {httpMethod: "POST", httpPath: "/newsletters/mute"},
	"newsletter.messages":     {httpMethod: "POST", httpPath: "/newsletters/messages"},
	"newsletter.updates":      {httpMethod: "POST", httpPath: "/newsletters/updates"},
	"newsletter.mark.viewed":  {httpMethod: "POST", httpPath: "/newsletters/mark-viewed"},
	"newsletter.react":        {httpMethod: "POST", httpPath: "/newsletters/react"},
	"newsletter.subscribe":    {httpMethod: "POST", httpPath: "/newsletters/subscribe"},
	"newsletter.demote":       {httpMethod: "POST", httpPath: "/newsletters/demote"},
	"newsletter.change.owner": {httpMethod: "POST", httpPath: "/newsletters/change-owner"},
	"newsletter.delete":       {httpMethod: "DELETE", httpPath: "/newsletters/delete"},

	// F233(b): convite de administrador de canal. Mesma forma de pedido das
	// restantes operações de newsletter, logo o mesmo lugar.
	"newsletter.admin.invite":        {httpMethod: "POST", httpPath: "/newsletters/admin-invite"},
	"newsletter.admin.invite.accept": {httpMethod: "POST", httpPath: "/newsletters/admin-invite/accept"},
	"newsletter.admin.invite.revoke": {httpMethod: "POST", httpPath: "/newsletters/admin-invite/revoke"},

	// F191: leitura de etiquetas. Não há método de escrita porque a
	// biblioteca não sabe criá-las (LIB-01).
	"label.list":       {httpMethod: "GET", httpPath: "/labels"},
	"label.list.chats": {httpMethod: "GET", httpPath: "/labels/{id}/chats"},
}
