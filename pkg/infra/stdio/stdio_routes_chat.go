package stdio

import "fmt"

// Rotas de conversa (`chat.*`): envio, ações sobre mensagens, download de
// mídia e leitura de histórico.

var chatStaticRoutes = map[string]staticRoute{
	"chat.send.text":     {httpMethod: "POST", httpPath: "/chat/send/text"},
	"chat.send.image":    {httpMethod: "POST", httpPath: "/chat/send/image"},
	"chat.send.video":    {httpMethod: "POST", httpPath: "/chat/send/video"},
	"chat.send.document": {httpMethod: "POST", httpPath: "/chat/send/document"},
	"chat.send.audio":    {httpMethod: "POST", httpPath: "/chat/send/audio"},
	"chat.send.sticker":  {httpMethod: "POST", httpPath: "/chat/send/sticker"},
	"chat.send.location": {httpMethod: "POST", httpPath: "/chat/send/location"},
	"chat.send.contact":  {httpMethod: "POST", httpPath: "/chat/send/contact"},
	"chat.send.poll":     {httpMethod: "POST", httpPath: "/chat/send/poll"},
	"chat.send.buttons":  {httpMethod: "POST", httpPath: "/chat/send/buttons"},
	"chat.send.list":     {httpMethod: "POST", httpPath: "/chat/send/list"},
	"chat.send.edit":     {httpMethod: "POST", httpPath: "/chat/send/edit"},

	"chat.delete":                      {httpMethod: "POST", httpPath: "/chat/delete"},
	"chat.react":                       {httpMethod: "POST", httpPath: "/chat/react"},
	"chat.archive":                     {httpMethod: "POST", httpPath: "/chat/archive"},
	"chat.presence":                    {httpMethod: "POST", httpPath: "/chat/presence"},
	"chat.markread":                    {httpMethod: "POST", httpPath: "/chat/markread"},
	"chat.request-unavailable-message": {httpMethod: "POST", httpPath: "/chat/request-unavailable-message"},

	"chat.download.image":    {httpMethod: "POST", httpPath: "/chat/downloadimage"},
	"chat.download.video":    {httpMethod: "POST", httpPath: "/chat/downloadvideo"},
	"chat.download.audio":    {httpMethod: "POST", httpPath: "/chat/downloadaudio"},
	"chat.download.document": {httpMethod: "POST", httpPath: "/chat/downloaddocument"},
}

var chatDynamicRoutes = map[string]dynamicRoute{
	"chat.history": {httpMethod: "GET", buildPath: chatHistoryPath},
}

func chatHistoryPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	chatJID, ok := ss.stringParam(req, "chat_jid")
	if !ok {
		return "", false
	}
	httpPath := "/chat/history?chat_jid=" + chatJID
	// Add optional limit parameter
	if limit, ok := req.Params["limit"].(float64); ok {
		httpPath += fmt.Sprintf("&limit=%d", int(limit))
	}
	return httpPath, true
}
