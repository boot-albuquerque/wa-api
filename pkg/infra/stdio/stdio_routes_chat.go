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
	"chat.send.pollvote": {httpMethod: "POST", httpPath: "/chats/send/pollvote"},
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
	"chat.markread":                    {httpMethod: "POST", httpPath: "/chats/markread"},
	"chat.request-unavailable-message": {httpMethod: "POST", httpPath: "/chats/request-unavailable-message"},
	"chat.ephemeral":                   {httpMethod: "POST", httpPath: "/chats/ephemeral"},
	"chat.ephemeral.default":           {httpMethod: "POST", httpPath: "/chats/ephemeral/default"},
	"message.star":                     {httpMethod: "POST", httpPath: "/messages/star"},

	// chat.download.image/video/audio/document continuam a apontar para as
	// rotas /chat/download{tipo} (singular): NÃO fazem parte da tabela de
	// padronização (caminhos.tsv) — foram consolidadas à parte em
	// /chats/download/{kind} (CAP-10), e essas cinco rotas legadas continuam
	// vivas por decisão própria (HOUSEKEEP F297). Não tocar aqui.

	"chat.download.image":    {httpMethod: "POST", httpPath: "/chat/downloadimage"},
	"chat.download.video":    {httpMethod: "POST", httpPath: "/chat/downloadvideo"},
	"chat.download.audio":    {httpMethod: "POST", httpPath: "/chat/downloadaudio"},
	"chat.download.document": {httpMethod: "POST", httpPath: "/chat/downloaddocument"},
}

var chatDynamicRoutes = map[string]dynamicRoute{
	"chat.history": {httpMethod: "GET", buildPath: chatHistoryPath},

	// CAP-10: a forma consolidada, com o kind na RELAÇÃO do caminho — ver
	// api/openapi/paths/conversa.yaml, "/chats/download/{kind}".
	"chat.download.media": {httpMethod: "POST", buildPath: chatDownloadMediaPath},
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
