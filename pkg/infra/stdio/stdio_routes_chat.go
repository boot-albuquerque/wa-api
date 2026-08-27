package stdio

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// Rotas de conversa (`chat.*`): envio, ações sobre mensagens, download de
// mídia e leitura de histórico.

var chatStaticRoutes = map[string]staticRoute{
	"chat.send.text":     {httpMethod: "POST", httpPath: "/chats/send/text"},
	"chat.send.image":    {httpMethod: "POST", httpPath: "/chats/send/image"},
	"chat.send.video":    {httpMethod: "POST", httpPath: "/chats/send/video"},
	"chat.send.document": {httpMethod: "POST", httpPath: "/chats/send/document"},
	"chat.send.audio":    {httpMethod: "POST", httpPath: "/chats/send/audio"},
	"chat.send.sticker":  {httpMethod: "POST", httpPath: "/chats/send/sticker"},
	"chat.send.location": {httpMethod: "POST", httpPath: "/chats/send/location"},
	"chat.send.contact":  {httpMethod: "POST", httpPath: "/chats/send/contact"},
	"chat.send.poll":     {httpMethod: "POST", httpPath: "/chats/send/poll"},
	"chat.send.forward":  {httpMethod: "POST", httpPath: "/chats/send/forward"},
	"chat.send.buttons":  {httpMethod: "POST", httpPath: "/chats/send/buttons"},
	"chat.send.carousel": {httpMethod: "POST", httpPath: "/chats/send/carousel"},
	"chat.send.list":     {httpMethod: "POST", httpPath: "/chats/send/list"},
	"chat.send.edit":     {httpMethod: "POST", httpPath: "/chats/send/edit"},

	"chat.delete":                      {httpMethod: "POST", httpPath: "/chats/delete/message"},
	"chat.react":                       {httpMethod: "POST", httpPath: "/chats/react"},
	"chat.archive":                     {httpMethod: "POST", httpPath: "/chats/archive"},
	"chat.pin":                         {httpMethod: "POST", httpPath: "/chats/pin"},
	"chat.mute":                        {httpMethod: "POST", httpPath: "/chats/mute"},
	"chat.presence":                    {httpMethod: "POST", httpPath: "/chats/presence"},
	"chat.request-unavailable-message": {httpMethod: "POST", httpPath: "/chats/request-unavailable-message"},
	"chat.ephemeral":                   {httpMethod: "POST", httpPath: "/chats/ephemeral"},
	"chat.ephemeral.default":           {httpMethod: "POST", httpPath: "/chats/ephemeral/default"},
	"message.star":                     {httpMethod: "POST", httpPath: "/messages/star"},

	// chat.markread e chat.send.pollvote NÃO estão aqui: o corte a hard das
	// rotas concatenadas (worktree http-dto-paths, F297) já as tornou
	// DINÂMICAS — ver chatDynamicRoutes abaixo (chat_jid/poll_message_id
	// vivem no caminho, não são mais um segmento estático).
	//
	// chat.download.image/video/audio/document NÃO estão aqui: as cinco
	// rotas legadas /chat/download{tipo} foram apagadas (reversão de
	// F269/CAP-10 para download, worktree http-dto-download-paths, F328) —
	// só a forma consolidada /chats/download/{kind} responde, em
	// chatDynamicRoutes (chat.download.media).
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
	httpPath := "/chats/history?chat_jid=" + chatJID
	// Add optional limit parameter
	if limit, ok := req.Params["limit"].(float64); ok {
		httpPath += fmt.Sprintf("&limit=%d", int(limit))
	}
	log.Debug().Str("method", req.Method).Str("path", httpPath).Msg("Rota dinamica de chat resolvida")
	return httpPath, true
}
