package bootstrap

import (
	"net/http"
	customhttp "wa-api/pkg/presentation/http"

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

// registerCustomRoutes registra todas as rotas custom do disparazaap-wa-api.
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

	// User routes (user)
	// Admin routes (/admin/users, /admin/users/{id}) are registered
	// directly in routes.go with authAdmin middleware — see routes.go:51-58.
	registry.Register("/user/check", customChain.Then(customHandlerSet.User.CheckUser()), "POST")
	registry.Register("/user/block", customChain.Then(customHandlerSet.User.BlockUser()), "POST")
	registry.Register("/user/unblock", customChain.Then(customHandlerSet.User.UnblockUser()), "POST")
	registry.Register("/user/lid/{jid}", customChain.Then(customHandlerSet.User.GetUserLID()), "GET")

	// Group routes (migrated from internal/)
	registry.Register("/group/requestparticipants", customChain.Then(customHandlerSet.Group.GetGroupRequestParticipants), "GET")
	registry.Register("/group/list", customChain.Then(customHandlerSet.Group.ListGroups), "POST")
	registry.Register("/group/info", customChain.Then(customHandlerSet.Group.GetGroupInfo), "POST")
	registry.Register("/group/invitelink", customChain.Then(customHandlerSet.Group.GetGroupInviteLink), "POST")
	registry.Register("/group/inviteinfo", customChain.Then(customHandlerSet.Group.GetGroupInviteInfo), "POST")

	// Group management routes (still using server methods as migration in-progress)
	registry.Register("/group/create", customChain.Then(customHandlerSet.GroupMgmt.CreateGroup), "POST")
	registry.Register("/group/join", customChain.Then(customHandlerSet.GroupMgmt.GroupJoin), "POST")
	registry.Register("/group/leave", customChain.Then(customHandlerSet.GroupMgmt.GroupLeave), "POST")
	registry.Register("/group/name", customChain.Then(customHandlerSet.GroupMgmt.SetGroupName), "POST")
	registry.Register("/group/topic", customChain.Then(customHandlerSet.GroupMgmt.SetGroupTopic), "POST")
	registry.Register("/group/photo", customChain.Then(customHandlerSet.GroupMgmt.SetGroupPhoto), "POST")
	registry.Register("/group/photo/remove", customChain.Then(customHandlerSet.GroupMgmt.RemoveGroupPhoto), "POST")
	registry.Register("/group/announce", customChain.Then(customHandlerSet.GroupMgmt.SetGroupAnnounce), "POST")
	registry.Register("/group/locked", customChain.Then(customHandlerSet.GroupMgmt.SetGroupLocked), "POST")
	registry.Register("/group/ephemeral", customChain.Then(customHandlerSet.GroupMgmt.SetDisappearingTimer), "POST")
	registry.Register("/group/updateparticipants", customChain.Then(customHandlerSet.GroupMgmt.UpdateGroupParticipants), "POST")
	registry.Register("/group/updaterequestparticipants", customChain.Then(customHandlerSet.GroupMgmt.UpdateGroupParticipants), "POST")
	registry.Register("/group/joinapprovalmode", customChain.Then(customHandlerSet.GroupMgmt.SetGroupLocked), "POST")

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

	// Download routes
	registry.Register("/chat/downloadimage", customChain.Then(customHandlerSet.Extended.DownloadImage), "POST")
	registry.Register("/chat/downloadvideo", customChain.Then(customHandlerSet.Extended.DownloadVideo), "POST")
	registry.Register("/chat/downloadaudio", customChain.Then(customHandlerSet.Extended.DownloadAudio), "POST")
	registry.Register("/chat/downloaddocument", customChain.Then(customHandlerSet.Extended.DownloadDocument), "POST")
	registry.Register("/chat/downloadsticker", customChain.Then(customHandlerSet.Extended.DownloadSticker), "POST")

	// Presence & Chat routes
	registry.Register("/user/presence", customChain.Then(customHandlerSet.Extended.SendPresence), "POST")
	registry.Register("/user/presence/subscribe", customChain.Then(customHandlerSet.Extended.SubscribePresence), "POST")
	registry.Register("/chat/presence", customChain.Then(customHandlerSet.Extended.ChatPresence), "POST")
	registry.Register("/chat/markread", customChain.Then(customHandlerSet.Extended.MarkRead), "POST")

	// React route
	registry.Register("/chat/react", customChain.Then(customHandlerSet.Extended.React), "POST")

	// User info routes
	registry.Register("/user/info", customChain.Then(customHandlerSet.Extended.GetUserInfo), "POST")
	registry.Register("/user/avatar", customChain.Then(customHandlerSet.Extended.GetAvatar), "POST")
	registry.Register("/user/contacts", customChain.Then(customHandlerSet.Extended.GetContacts), "GET")

	// Misc routes (newsletter, privacy, call, archive)
	registry.Register("/newsletter/list", customChain.Then(customHandlerSet.Misc.ListNewsletter), "GET")
	registry.Register("/call/reject", customChain.Then(customHandlerSet.Misc.RejectCall), "POST")
	registry.Register("/chat/archive", customChain.Then(customHandlerSet.Misc.ArchiveChat), "POST")
	registry.Register("/chat/request-unavailable-message", customChain.Then(customHandlerSet.Misc.RequestUnavailableMessage), "POST")
	registry.Register("/user/privacy", customChain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			customHandlerSet.Misc.GetPrivacySettings.ServeHTTP(w, r)
		} else {
			customHandlerSet.Misc.SetPrivacySetting.ServeHTTP(w, r)
		}
	})), "GET", "POST")

	// Health route via internal handler — behind auth (chain c), unlike the
	// unauthenticated container liveness probe /livez (router_setup.go).
	registry.Register("/health", c.Then(http.HandlerFunc(customHandlerSet.Misc.Health.ServeHTTP)), "GET")

	// Legacy URL paths for storage/session — use same internal handlers
	registry.Register("/session/history", customChain.Then(customHandlerSet.Storage.SetHistory), "POST")
	registry.Register("/session/proxy", customChain.Then(customHandlerSet.Storage.SetProxy), "POST")
	registry.Register("/session/s3/config", customChain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" { customHandlerSet.Storage.ConfigureS3.ServeHTTP(w, r) } else if r.Method == "GET" { customHandlerSet.Storage.GetS3Config.ServeHTTP(w, r) } else { customHandlerSet.Storage.DeleteS3Config.ServeHTTP(w, r) }
	})), "POST", "GET", "DELETE")
	registry.Register("/session/s3/test", customChain.Then(customHandlerSet.Storage.TestS3Connection), "POST")
	registry.Register("/session/hmac/config", customChain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" { customHandlerSet.Storage.ConfigureHmac.ServeHTTP(w, r) } else if r.Method == "GET" { customHandlerSet.Storage.GetHmacConfig.ServeHTTP(w, r) } else { customHandlerSet.Storage.DeleteHmacConfig.ServeHTTP(w, r) }
	})), "POST", "GET", "DELETE")
	registry.Register("/chat/history", customChain.Then(customHandlerSet.Storage.GetHistory), "GET")
	registry.Register("/chat/delete", customChain.Then(customHandlerSet.Message.DeleteMessage), "POST")
	registry.Register("/status/set/text", customChain.Then(customHandlerSet.Session.SetStatusMessage), "POST")
	// Static files — keep in routes.go only, not reregistered here

	registry.Apply(s.Router)
}
