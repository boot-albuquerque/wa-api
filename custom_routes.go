package main

import (
	"net/http"
	customhttp "wuzapi/internal/interfaces/http"

	"github.com/justinas/alice"
)

// securityHeadersMiddleware aplica headers de segurança a todas as rotas custom.
// Cache-Control é aplicado per-handler (não aqui) para permitir caching seletivo.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// registerCustomRoutes registra todas as rotas custom do disparazaap-wuzapi.
// Recebe a chain de middleware já configurada (authalice + logging) do upstream.
// Chamado uma única vez em routes.go.
func (s *server) registerCustomRoutes(c alice.Chain) {
	customChain := c.Append(securityHeadersMiddleware)

	registry := customhttp.NewHandlerRegistry()

	// Session routes
	registry.Register("/session/profile", customChain.Then(customHandlerSet.Profile), "GET")
	registry.Register("/session/connect", customChain.Then(customHandlerSet.Session.Connect), "GET")
	registry.Register("/session/disconnect", customChain.Then(customHandlerSet.Session.Disconnect), "GET")
	registry.Register("/session/qr", customChain.Then(customHandlerSet.Session.GetQR), "GET")
	registry.Register("/session/logout", customChain.Then(customHandlerSet.Session.Logout), "POST")
	registry.Register("/session/pairphone", customChain.Then(customHandlerSet.Session.PairPhone), "POST")
	registry.Register("/session/status", customChain.Then(customHandlerSet.Session.GetStatus), "GET")
	registry.Register("/user/status", customChain.Then(customHandlerSet.Session.SetStatusMessage), "POST")
	registry.Register("/user/history/sync", customChain.Then(customHandlerSet.Session.RequestHistorySync), "POST")

	// Message routes
	registry.Register("/chat/send/text", customChain.Then(customHandlerSet.Message.SendMessage), "POST")
	registry.Register("/chat/send/image", customChain.Then(customHandlerSet.Message.SendImage), "POST")
	registry.Register("/chat/send/document", customChain.Then(customHandlerSet.Message.SendDocument), "POST")
	registry.Register("/chat/send/audio", customChain.Then(customHandlerSet.Message.SendAudio), "POST")
	registry.Register("/chat/send/sticker", customChain.Then(customHandlerSet.Message.SendSticker), "POST")
	registry.Register("/chat/send/video", customChain.Then(customHandlerSet.Message.SendVideo), "POST")
	registry.Register("/chat/send/contact", customChain.Then(customHandlerSet.Message.SendContact), "POST")
	registry.Register("/chat/send/location", customChain.Then(customHandlerSet.Message.SendLocation), "POST")
	registry.Register("/chat/send/buttons", customChain.Then(customHandlerSet.Message.SendButtons), "POST")
	registry.Register("/chat/send/list", customChain.Then(customHandlerSet.Message.SendList), "POST")
	registry.Register("/chat/send/poll", customChain.Then(customHandlerSet.Message.SendPoll), "POST")
	registry.Register("/chat/delete/message", customChain.Then(customHandlerSet.Message.DeleteMessage), "POST")
	registry.Register("/chat/send/edit", customChain.Then(customHandlerSet.Message.SendEditMessage), "POST")
	registry.Register("/chat/send/template", customChain.Then(customHandlerSet.Message.SendTemplate), "POST")

	// Webhook routes
	registry.Register("/webhook", customChain.Then(customHandlerSet.Webhook.GetWebhook), "GET")
	registry.Register("/webhook", customChain.Then(customHandlerSet.Webhook.SetWebhook), "POST")
	registry.Register("/webhook", customChain.Then(customHandlerSet.Webhook.UpdateWebhook), "PUT")
	registry.Register("/webhook", customChain.Then(customHandlerSet.Webhook.DeleteWebhook), "DELETE")

	// User routes (admin)
	registry.Register("/admin/users", customChain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			customHandlerSet.User.ListUsers().ServeHTTP(w, r)
		} else if r.Method == "POST" {
			customHandlerSet.User.AddUser().ServeHTTP(w, r)
		}
	})), "GET", "POST")

	registry.Register("/admin/users/{id}", customChain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			customHandlerSet.User.GetUser().ServeHTTP(w, r)
		} else if r.Method == "PUT" {
			customHandlerSet.User.EditUser().ServeHTTP(w, r)
		} else if r.Method == "DELETE" {
			customHandlerSet.User.DeleteUser().ServeHTTP(w, r)
		}
	})), "GET", "PUT", "DELETE")

	// User routes (user)
	registry.Register("/user/check", customChain.Then(customHandlerSet.User.CheckUser()), "POST")
	registry.Register("/user/block", customChain.Then(customHandlerSet.User.BlockUser()), "POST")
	registry.Register("/user/unblock", customChain.Then(customHandlerSet.User.UnblockUser()), "POST")
	registry.Register("/user/lid/{jid}", customChain.Then(customHandlerSet.User.GetUserLID()), "GET")

	// Group routes
	registry.Register("/group/requestparticipants", customChain.Then(customHandlerSet.Group.GetGroupRequestParticipants), "GET")
	registry.Register("/group/list", customChain.Then(customHandlerSet.Group.ListGroups), "POST")
	registry.Register("/group/info", customChain.Then(customHandlerSet.Group.GetGroupInfo), "POST")
	registry.Register("/group/invitelink", customChain.Then(customHandlerSet.Group.GetGroupInviteLink), "POST")
	registry.Register("/group/inviteinfo", customChain.Then(customHandlerSet.Group.GetGroupInviteInfo), "POST")

	// Storage routes (S3, HMAC, Proxy, History)
	registry.Register("/s3/configure", customChain.Then(customHandlerSet.Storage.ConfigureS3), "POST")
	registry.Register("/s3/config", customChain.Then(customHandlerSet.Storage.GetS3Config), "GET")
	registry.Register("/s3/test", customChain.Then(customHandlerSet.Storage.TestS3Connection), "POST")
	registry.Register("/s3/config", customChain.Then(customHandlerSet.Storage.DeleteS3Config), "DELETE")
	registry.Register("/hmac/configure", customChain.Then(customHandlerSet.Storage.ConfigureHmac), "POST")
	registry.Register("/hmac/config", customChain.Then(customHandlerSet.Storage.GetHmacConfig), "GET")
	registry.Register("/hmac/config", customChain.Then(customHandlerSet.Storage.DeleteHmacConfig), "DELETE")
	registry.Register("/proxy/set", customChain.Then(customHandlerSet.Storage.SetProxy), "POST")
	registry.Register("/webhook/history", customChain.Then(customHandlerSet.Storage.SetHistory), "POST")
	registry.Register("/webhook/history", customChain.Then(customHandlerSet.Storage.GetHistory), "GET")

	// Blocklist route
	registry.Register("/user/blocklist", customChain.Then(customHandlerSet.Blocklist.GetBlocklist), "GET")

	// Health route (public, no auth chain needed)
	registry.Register("/health", c.Then(http.HandlerFunc(s.GetHealth())), "GET")

	registry.Apply(s.router)
}
