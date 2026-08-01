package stdio

// Rotas de grupo (`group.*`): criação, metadados, foto, convites e
// participantes.

var groupStaticRoutes = map[string]staticRoute{
	"group.list":       {httpMethod: "GET", httpPath: "/group/list"},
	"group.create":     {httpMethod: "POST", httpPath: "/group/create"},
	"group.info":       {httpMethod: "GET", httpPath: "/group/info"},
	"group.invitelink": {httpMethod: "GET", httpPath: "/group/invitelink"},

	"group.photo":        {httpMethod: "POST", httpPath: "/group/photo"},
	"group.photo.remove": {httpMethod: "POST", httpPath: "/group/photo/remove"},

	"group.leave":     {httpMethod: "POST", httpPath: "/group/leave"},
	"group.name":      {httpMethod: "POST", httpPath: "/group/name"},
	"group.topic":     {httpMethod: "POST", httpPath: "/group/topic"},
	"group.announce":  {httpMethod: "POST", httpPath: "/group/announce"},
	"group.locked":    {httpMethod: "POST", httpPath: "/group/locked"},
	"group.ephemeral": {httpMethod: "POST", httpPath: "/group/ephemeral"},

	"group.join":               {httpMethod: "POST", httpPath: "/group/join"},
	"group.inviteinfo":         {httpMethod: "POST", httpPath: "/group/inviteinfo"},
	"group.updateparticipants": {httpMethod: "POST", httpPath: "/group/updateparticipants"},
}
