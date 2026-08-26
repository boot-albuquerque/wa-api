package bootstrap

import (
	"net/http"
	"strings"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/devui"

	"github.com/gorilla/mux"
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
// Recebe a chain de middleware já configurada (authalice + logging) e o
// conjunto de handlers já construído — ambos vindos de buildRouter (router.go),
// sem ler variáveis globais de pacote.
func registerCustomRoutes(router *mux.Router, c alice.Chain, ch *customHandlers) {
	customChain := c.Append(securityHeadersMiddleware)

	registry := customhttp.NewHandlerRegistry()

	// Páginas de teste manual (devui). Registradas SÓ com WA_API_DEV_UI
	// ligado, e deliberadamente FORA de customChain: o que elas servem é
	// HTML estático, sem dado de usuário. O token é digitado na própria
	// página e viaja nas chamadas que ela faz à API — essas sim passam pela
	// chain. Exigir token para baixar o HTML impediria a página de existir
	// antes de haver token.
	//
	// Os headers de segurança CONTINUAM valendo: só a autenticação sai.
	if devui.Enabled() {
		devChain := alice.New(securityHeadersMiddleware).Then(devui.Handler(ch.AdminToken))
		registry.Register(devui.BasePath+"{rest:.*}", devChain, "GET")
		registry.Register(strings.TrimSuffix(devui.BasePath, "/"), devChain, "GET")
	}

	// Session routes
	registry.Register("/session/profile", customChain.Then(ch.Profile), "GET")
	registry.Register("/session/profile/full", customChain.Then(ch.ProfileFull), "GET")
	registry.Register("/session/connect", customChain.Then(ch.Session.Connect), "GET")
	registry.Register("/session/disconnect", customChain.Then(ch.Session.Disconnect), "GET")
	registry.Register("/session/qr", customChain.Then(ch.Session.GetQR), "GET")
	registry.Register("/session/logout", customChain.Then(ch.Session.Logout), "POST")
	registry.Register("/session/pairphone", customChain.Then(ch.Session.PairPhone), "POST")
	registry.Register("/session/status", customChain.Then(ch.Session.GetStatus), "GET")
	// Real-time push counterpart of /session/status + /session/qr — see
	// handler_session_ws.go. Same auth chain (customChain), REST untouched.
	registry.Register("/session/ws", customChain.Then(ch.Session.WS), "GET")
	registry.Register("/user/status", customChain.Then(ch.Session.SetStatusMessage), "POST")
	registry.Register("/user/history/sync", customChain.Then(ch.Session.RequestHistorySync), "POST")
	// Additive: forces a re-fetch of the contact roster app-state patch —
	// a separate capability from /user/history/sync (message history), not
	// a variant of it. See handler_session.go: SyncContactRosterHandler.
	registry.Register("/user/contacts/sync", customChain.Then(ch.Session.SyncContactRoster), "POST")

	// Message routes
	registry.Register("/chat/send/text", customChain.Then(ch.Message.SendMessage), "POST")
	registry.Register("/chat/send/image", customChain.Then(ch.Message.SendImage), "POST")
	registry.Register("/chat/send/document", customChain.Then(ch.Message.SendDocument), "POST")
	registry.Register("/chat/send/audio", customChain.Then(ch.Message.SendAudio), "POST")
	registry.Register("/chat/send/sticker", customChain.Then(ch.Message.SendSticker), "POST")
	registry.Register("/chat/send/video", customChain.Then(ch.Message.SendVideo), "POST")
	registry.Register("/chat/send/contact", customChain.Then(ch.Message.SendContact), "POST")
	registry.Register("/chat/send/location", customChain.Then(ch.Message.SendLocation), "POST")
	registry.Register("/chat/send/buttons", customChain.Then(ch.Message.SendButtons), "POST")
	registry.Register("/chat/send/carousel", customChain.Then(ch.Message.SendCarousel), "POST")
	registry.Register("/chat/send/list", customChain.Then(ch.Message.SendList), "POST")
	registry.Register("/chat/send/poll", customChain.Then(ch.Message.SendPoll), "POST")
	registry.Register("/chat/send/pollvote", customChain.Then(ch.Message.SendPollVote), "POST")
	registry.Register("/chat/send/forward", customChain.Then(ch.Message.SendForward), "POST")
	registry.Register("/chat/delete/message", customChain.Then(ch.Message.DeleteMessage), "POST")
	registry.Register("/chat/send/edit", customChain.Then(ch.Message.SendEditMessage), "POST")
	registry.Register("/chat/send/template", customChain.Then(ch.Message.SendTemplate), "POST")

	// Webhook routes
	registry.Register("/webhook", customChain.Then(ch.Webhook.GetWebhook), "GET")
	registry.Register("/webhook", customChain.Then(ch.Webhook.SetWebhook), "POST")
	registry.Register("/webhook", customChain.Then(ch.Webhook.UpdateWebhook), "PUT")
	registry.Register("/webhook", customChain.Then(ch.Webhook.DeleteWebhook), "DELETE")

	// User routes (user)
	// Admin routes (/admin/users, /admin/users/{id}) are registered
	// directly in routes.go with authAdmin middleware — see routes.go:51-58.
	registry.Register("/user/check", customChain.Then(ch.User.CheckUser()), "POST")
	registry.Register("/user/block", customChain.Then(ch.User.BlockUser()), "POST")
	registry.Register("/user/unblock", customChain.Then(ch.User.UnblockUser()), "POST")
	registry.Register("/user/lid/{jid}", customChain.Then(ch.User.GetUserLID()), "GET")
	registry.Register("/user/profile/{jid}", customChain.Then(ch.User.GetUserProfile()), "GET")
	registry.Register("/chat/list", customChain.Then(ch.User.ListChats()), "GET")

	// Group routes (migrated from internal/)
	registry.Register("/group/requestparticipants", customChain.Then(ch.Group.GetGroupRequestParticipants), "GET")
	registry.Register("/group/list", customChain.Then(ch.Group.ListGroups), "POST")
	registry.Register("/group/info", customChain.Then(ch.Group.GetGroupInfo), "POST")
	registry.Register("/group/invitelink", customChain.Then(ch.Group.GetGroupInviteLink), "POST")
	registry.Register("/group/inviteinfo", customChain.Then(ch.Group.GetGroupInviteInfo), "POST")

	// Group management routes (still using server methods as migration in-progress)
	registry.Register("/group/create", customChain.Then(ch.GroupMgmt.CreateGroup), "POST")
	registry.Register("/group/join", customChain.Then(ch.GroupMgmt.GroupJoin), "POST")
	registry.Register("/group/leave", customChain.Then(ch.GroupMgmt.GroupLeave), "POST")
	registry.Register("/group/name", customChain.Then(ch.GroupMgmt.SetGroupName), "POST")
	registry.Register("/group/topic", customChain.Then(ch.GroupMgmt.SetGroupTopic), "POST")
	registry.Register("/group/photo", customChain.Then(ch.GroupMgmt.SetGroupPhoto), "POST")
	registry.Register("/group/photo/remove", customChain.Then(ch.GroupMgmt.RemoveGroupPhoto), "POST")
	registry.Register("/group/announce", customChain.Then(ch.GroupMgmt.SetGroupAnnounce), "POST")
	registry.Register("/group/locked", customChain.Then(ch.GroupMgmt.SetGroupLocked), "POST")
	registry.Register("/group/ephemeral", customChain.Then(ch.GroupMgmt.SetDisappearingTimer), "POST")
	registry.Register("/group/updateparticipants", customChain.Then(ch.GroupMgmt.UpdateGroupParticipants), "POST")
	registry.Register("/group/updaterequestparticipants", customChain.Then(ch.Group.UpdateGroupRequestParticipants), "POST")
	registry.Register("/group/joinapprovalmode", customChain.Then(ch.Group.SetGroupJoinApprovalMode), "POST")

	// Storage routes (S3, HMAC, Proxy, History)
	registry.Register("/s3/configure", customChain.Then(ch.Storage.ConfigureS3), "POST")
	registry.Register("/s3/config", customChain.Then(ch.Storage.GetS3Config), "GET")
	registry.Register("/s3/test", customChain.Then(ch.Storage.TestS3Connection), "POST")
	registry.Register("/s3/config", customChain.Then(ch.Storage.DeleteS3Config), "DELETE")
	registry.Register("/hmac/configure", customChain.Then(ch.Storage.ConfigureHmac), "POST")
	registry.Register("/hmac/config", customChain.Then(ch.Storage.GetHmacConfig), "GET")
	registry.Register("/hmac/config", customChain.Then(ch.Storage.DeleteHmacConfig), "DELETE")
	registry.Register("/proxy/set", customChain.Then(ch.Storage.SetProxy), "POST")
	registry.Register("/webhook/history", customChain.Then(ch.Storage.SetHistory), "POST")
	registry.Register("/webhook/history", customChain.Then(ch.Storage.GetHistory), "GET")

	// Blocklist route
	registry.Register("/user/blocklist", customChain.Then(ch.Blocklist.GetBlocklist), "GET")

	// Download routes
	registry.Register("/chat/downloadimage", customChain.Then(ch.Download.Image), "POST")
	registry.Register("/chat/downloadvideo", customChain.Then(ch.Download.Video), "POST")
	registry.Register("/chat/downloadaudio", customChain.Then(ch.Download.Audio), "POST")
	registry.Register("/chat/downloaddocument", customChain.Then(ch.Download.Document), "POST")
	registry.Register("/chat/downloadsticker", customChain.Then(ch.Download.Sticker), "POST")

	// Presence & Chat routes
	registry.Register("/user/presence", customChain.Then(ch.Presence.Send), "POST")
	registry.Register("/user/presence/subscribe", customChain.Then(ch.Presence.Subscribe), "POST")
	registry.Register("/chat/presence", customChain.Then(ch.Presence.Chat), "POST")
	registry.Register("/chat/markread", customChain.Then(ch.Presence.MarkRead), "POST")

	// React route
	registry.Register("/chat/react", customChain.Then(ch.Reaction.React), "POST")

	// User info routes
	registry.Register("/user/info", customChain.Then(ch.Contact.UserInfo), "POST")
	registry.Register("/user/avatar", customChain.Then(ch.Contact.Avatar), "POST")
	registry.Register("/user/contacts", customChain.Then(ch.Contact.Contacts), "GET")
	registry.Register("/user/contacts/last-activity", customChain.Then(ch.Contact.LastActivity), "GET")

	// Misc routes (newsletter, privacy, call, archive)
	registry.Register("/newsletter/list", customChain.Then(ch.Misc.ListNewsletter), "GET")

	// Newsletter operation routes. The parity survey of 2026-08-20 found the
	// library exposing twelve capabilities against the one route we had
	// (/newsletter/list); these are the other eleven.
	// Etiquetas (F191). Só LEITURA: a biblioteca não sabe criá-las (LIB-01),
	// e uma rota de escrita responderia 200 sem fazer nada — o defeito da
	// F198, que não se acrescenta de propósito.
	registry.Register("/labels", customChain.Then(ch.Label.ListLabels), "GET")
	registry.Register("/labels/{id}/chats", customChain.Then(ch.Label.ListLabelChat), "GET")

	registry.Register("/newsletter/create", customChain.Then(ch.Newsletter.Create), "POST")
	registry.Register("/newsletter/info", customChain.Then(ch.Newsletter.Info), "POST")
	registry.Register("/newsletter/info-invite", customChain.Then(ch.Newsletter.InfoInvite), "POST")
	registry.Register("/newsletter/follow", customChain.Then(ch.Newsletter.Follow), "POST")
	registry.Register("/newsletter/unfollow", customChain.Then(ch.Newsletter.Unfollow), "POST")
	registry.Register("/newsletter/mute", customChain.Then(ch.Newsletter.Mute), "POST")
	registry.Register("/newsletter/messages", customChain.Then(ch.Newsletter.Messages), "POST")
	registry.Register("/newsletter/updates", customChain.Then(ch.Newsletter.Updates), "POST")
	registry.Register("/newsletter/mark-viewed", customChain.Then(ch.Newsletter.MarkViewed), "POST")
	registry.Register("/newsletter/react", customChain.Then(ch.Newsletter.React), "POST")
	registry.Register("/newsletter/subscribe", customChain.Then(ch.Newsletter.Subscribe), "POST")
	registry.Register("/newsletter/demote", customChain.Then(ch.Newsletter.Demote), "POST")
	registry.Register("/newsletter/change-owner", customChain.Then(ch.Newsletter.ChangeOwner), "POST")
	registry.Register("/newsletter/delete", customChain.Then(ch.Newsletter.Delete), "DELETE")
	registry.Register("/call/reject", customChain.Then(ch.Misc.RejectCall), "POST")
	registry.Register("/chat/mute", customChain.Then(ch.Misc.MuteChat), "POST")
	registry.Register("/chat/archive", customChain.Then(ch.Misc.ArchiveChat), "POST")
	registry.Register("/chat/pin", customChain.Then(ch.Misc.PinChat), "POST")
	registry.Register("/chat/ephemeral", customChain.Then(ch.Misc.SetDisappearingTimer), "POST")
	registry.Register("/chat/ephemeral/default", customChain.Then(ch.Misc.SetDefaultDisappearingTimer), "POST")
	registry.Register("/chat/request-unavailable-message", customChain.Then(ch.Misc.RequestUnavailableMessage), "POST")
	registry.Register("/message/star", customChain.Then(ch.Misc.StarMessage), "POST")
	registry.Register("/user/privacy", customChain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			ch.Misc.GetPrivacySettings.ServeHTTP(w, r)
		} else {
			ch.Misc.SetPrivacySetting.ServeHTTP(w, r)
		}
	})), "GET", "POST")

	// Health route via internal handler — behind auth (chain c), unlike the
	// unauthenticated container liveness probe /livez (router_setup.go).
	registry.Register("/health", c.Then(http.HandlerFunc(ch.Misc.Health.ServeHTTP)), "GET")

	// Legacy URL paths for storage/session — use same internal handlers
	registry.Register("/session/history", customChain.Then(ch.Storage.SetHistory), "POST")
	registry.Register("/session/proxy", customChain.Then(ch.Storage.SetProxy), "POST")
	registry.Register("/session/s3/config", customChain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			ch.Storage.ConfigureS3.ServeHTTP(w, r)
		case "GET":
			ch.Storage.GetS3Config.ServeHTTP(w, r)
		default:
			ch.Storage.DeleteS3Config.ServeHTTP(w, r)
		}
	})), "POST", "GET", "DELETE")
	registry.Register("/session/s3/test", customChain.Then(ch.Storage.TestS3Connection), "POST")
	registry.Register("/session/hmac/config", customChain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			ch.Storage.ConfigureHmac.ServeHTTP(w, r)
		case "GET":
			ch.Storage.GetHmacConfig.ServeHTTP(w, r)
		default:
			ch.Storage.DeleteHmacConfig.ServeHTTP(w, r)
		}
	})), "POST", "GET", "DELETE")
	// /chat/history reads the local MESSAGE history and answers messages;
	// /webhook/history above answers the webhook history configuration. They
	// are separate handlers, and TestChatHistoryAndWebhookHistoryAreDistinctHandlers
	// exists to keep them separate: pointing both at Storage.GetHistory is the
	// exact defect of HOUSEKEEP F124, and no assertion about a response body
	// would have caught it.
	registry.Register("/chat/history", customChain.Then(ch.ChatHistory.GetChatHistory), "GET")
	registry.Register("/chat/delete", customChain.Then(ch.Message.DeleteMessage), "POST")
	registry.Register("/status/set/image", customChain.Then(ch.Session.PublishStatusImage), "POST")
	registry.Register("/status/set/video", customChain.Then(ch.Session.PublishStatusVideo), "POST")
	registry.Register("/status/set/audio", customChain.Then(ch.Session.PublishStatusAudio), "POST")
	// Static files — keep in routes.go only, not reregistered here

	registry.Apply(router)
}

// registerAdminRoutes declara a tabela de rotas de /admin.
//
// Separada de buildRouter (router.go), onde vivia inline, por dois motivos. O
// primeiro é que o teste de consistência com a tabela do stdio precisa montar
// as MESMAS rotas sem precisar de banco, token ou middleware de auditoria —
// e replicá-las no teste criaria uma terceira tabela para divergir (F99). O
// segundo é o teto de complexidade do lint, que trava a PIOR função: tirar
// um bloco de buildRouter melhora o teto.
//
// Recebe o subrouter já com PathPrefix("/admin") e os middlewares aplicados;
// aqui só entram caminho, método e handler.
func registerAdminRoutes(adminRoutes *mux.Router, ch *customHandlers) {
	adminRoutes.Handle("/users", ch.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users/{id}", ch.User.ListUsers()).Methods("GET")
	adminRoutes.Handle("/users", ch.User.AddUser()).Methods("POST")
	adminRoutes.Handle("/users/{id}", ch.User.EditUser()).Methods("PUT")
	adminRoutes.Handle("/users/{id}", ch.User.DeleteUser()).Methods("DELETE")
	adminRoutes.Handle("/users/{id}/full", ch.Misc.DeleteUserComplete).Methods("DELETE")
}
