package stdio

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

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
	"chat.send.forward":  {httpMethod: "POST", httpPath: "/chat/send/forward"},
	"chat.send.buttons":  {httpMethod: "POST", httpPath: "/chat/send/buttons"},
	"chat.send.carousel": {httpMethod: "POST", httpPath: "/chat/send/carousel"},
	"chat.send.list":     {httpMethod: "POST", httpPath: "/chat/send/list"},
	"chat.send.edit":     {httpMethod: "POST", httpPath: "/chat/send/edit"},

	"chat.delete":                      {httpMethod: "POST", httpPath: "/chat/delete/message"},
	"chat.react":                       {httpMethod: "POST", httpPath: "/chat/react"},
	"chat.archive":                     {httpMethod: "POST", httpPath: "/chat/archive"},
	"chat.pin":                         {httpMethod: "POST", httpPath: "/chat/pin"},
	"chat.mute":                        {httpMethod: "POST", httpPath: "/chat/mute"},
	"chat.presence":                    {httpMethod: "POST", httpPath: "/chat/presence"},
	"chat.request-unavailable-message": {httpMethod: "POST", httpPath: "/chat/request-unavailable-message"},
	"chat.ephemeral":                   {httpMethod: "POST", httpPath: "/chat/ephemeral"},
	"chat.ephemeral.default":           {httpMethod: "POST", httpPath: "/chat/ephemeral/default"},
	"message.star":                     {httpMethod: "POST", httpPath: "/message/star"},
}

var chatDynamicRoutes = map[string]dynamicRoute{
	"chat.history": {httpMethod: "GET", buildPath: chatHistoryPath},

	// CAP-10: a forma consolidada, com o kind na RELAÇÃO do caminho — ver
	// api/openapi/paths/conversa.yaml, "/chats/download/{kind}".
	"chat.download.media": {httpMethod: "POST", buildPath: chatDownloadMediaPath},

	// chat_jid e poll_message_id passam a viver no caminho — ver
	// pkg/bootstrap/wiring_routes.go e a nota em groupDynamicRoutes.
	"chat.markread":      {httpMethod: "POST", buildPath: chatMarkReadPath},
	"chat.send.pollvote": {httpMethod: "POST", buildPath: chatSendPollVotePath},
}

func chatMarkReadPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	chatJID, ok := ss.stringParam(req, "ChatPhone")
	if !ok {
		return "", false
	}
	httpPath := "/chats/" + chatJID + "/read"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de chat resolvida")
	return httpPath, true
}

func chatSendPollVotePath(ss *Server, req *JSONRpcRequest) (string, bool) {
	pollMessageID, ok := ss.stringParam(req, "PollMessageId")
	if !ok {
		return "", false
	}
	httpPath := "/polls/" + pollMessageID + "/votes"
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de chat resolvida")
	return httpPath, true
}

func chatDownloadMediaPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	kind, ok := ss.stringParam(req, "kind")
	if !ok {
		return "", false
	}
	httpPath := "/chats/download/" + kind
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de download consolidado resolvida")
	return httpPath, true
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
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de chat resolvida")
	return httpPath, true
}
