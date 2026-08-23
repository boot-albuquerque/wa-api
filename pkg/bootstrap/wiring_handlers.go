package bootstrap

import (
	"slices"
	wachat "wa-api/pkg/infra/wa-noise/adapters/chat"
	wagroup "wa-api/pkg/infra/wa-noise/adapters/group"
	wamisc "wa-api/pkg/infra/wa-noise/adapters/misc"
	wapairing "wa-api/pkg/infra/wa-noise/adapters/pairing"
	wapresence "wa-api/pkg/infra/wa-noise/adapters/presence"
	wauser "wa-api/pkg/infra/wa-noise/adapters/user"
	wasession "wa-api/pkg/infra/wa-noise/runtime/session"

	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/egress"
	"wa-api/pkg/infra/media/opengraph"
	"wa-api/pkg/infra/media/sticker"
	"wa-api/pkg/infra/wa-noise/adapters/sessioncount"
	waclient "wa-api/pkg/infra/wa-noise/client"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"
	"wa-api/pkg/infra/wa-noise/observability/applog"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/handlers"

	"github.com/rs/zerolog/log"

	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/application/usecase/profile"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/application/usecase/user"
)

// MessageHandlers agrupa os handlers de mensagem.
type MessageHandlers struct {
	SendMessage     *handlers.SendMessageHandler
	SendImage       *handlers.SendImageHandler
	SendDocument    *handlers.SendDocumentHandler
	SendAudio       *handlers.SendAudioHandler
	SendSticker     *handlers.SendStickerHandler
	SendVideo       *handlers.SendVideoHandler
	SendContact     *handlers.SendContactHandler
	SendLocation    *handlers.SendLocationHandler
	SendButtons     *handlers.SendButtonsHandler
	SendCarousel    *handlers.SendCarouselHandler
	SendList        *handlers.SendListHandler
	SendPoll        *handlers.SendPollHandler
	DeleteMessage   *handlers.DeleteMessageHandler
	SendEditMessage *handlers.SendEditMessageHandler
	SendTemplate    *handlers.SendTemplateHandler
}

// SessionHandlers agrupa os handlers de sessão.
type SessionHandlers struct {
	Connect            *handlers.ConnectHandler
	Disconnect         *handlers.DisconnectHandler
	GetQR              *handlers.GetQRHandler
	Logout             *handlers.LogoutHandler
	PairPhone          *handlers.PairPhoneHandler
	GetStatus          *handlers.GetStatusHandler
	SetStatusMessage   *handlers.SetStatusMessageHandler
	RequestHistorySync *handlers.RequestHistorySyncHandler
	// SyncContactRoster is additive: a genuinely separate capability from
	// RequestHistorySync — see handler_session.go.
	SyncContactRoster *handlers.SyncContactRosterHandler
	// WS is the real-time counterpart of GetStatus/GetQR — see
	// handler_session_ws.go. Additive: REST polling keeps working unchanged.
	WS *handlers.WSHandler
}

// WebhookHandlers agrupa os handlers de webhook.
type WebhookHandlers struct {
	GetWebhook    *handlers.GetWebhookHandler
	SetWebhook    *handlers.SetWebhookHandler
	UpdateWebhook *handlers.UpdateWebhookHandler
	DeleteWebhook *handlers.DeleteWebhookHandler
}

// customHandlers agrupa todos os handlers custom disparazaap.
type customHandlers struct {
	// AdminToken existe aqui, e não como parâmetro de registerCustomRoutes,
	// por uma razão prática: o parâmetro obrigaria a tocar nas nove chamadas
	// de teste que a função já tem, e nenhuma delas se importa com este
	// valor. O campo vazio é o caso normal — só o devui o consome, e o devui
	// só existe com WA_API_DEV_UI ligado.
	AdminToken string

	Profile     *customhttp.ProfileHandler
	ProfileFull *customhttp.ProfileFullHandler
	Message     *MessageHandlers
	Session     *SessionHandlers
	Webhook     *WebhookHandlers
	User        *handlers.UserHandlers
	Group       *handlers.GroupHandlers
	Storage     *handlers.StorageHandlers
	Misc        *handlers.MiscHandlers
	Blocklist   *handlers.BlocklistHandlers
	Download    *handlers.DownloadHandlers
	Presence    *handlers.PresenceHandlers
	Reaction    *handlers.ReactionHandlers
	Contact     *handlers.ContactHandlers
	GroupMgmt   *handlers.GroupManagementHandlers
	ChatHistory *handlers.ChatHistoryHandlers
	Newsletter  *handlers.NewsletterHandlers
	Label       *handlers.LabelHandlers
}

var customHandlerSet = &customHandlers{}

// initCustomHandlers faz o wiring entre usecases, adapters e handlers custom.
// Chamado em main() ANTES de s.routes() (main.go:330-331) — registerCustomRoutes
// lê customHandlerSet, então a ordem é obrigatória: invertida, os campos de
// customHandlerSet estariam nil quando as rotas fossem registradas.
func initCustomHandlers(s *server) {
	// Adapters
	waClientLookup := waclient.ClientForGetter(clientManager.GetWaNoiseClient)
	presenceController := wapresence.NewPresenceControllerAdapter(waClientLookup)
	// WithPollOptions liga o guarda-opcoes de enquete: o adapter memoriza o
	// texto em claro das opcoes depois de cada envio, e o handler de eventos
	// o le' para casar o hash SHA-256 do voto com o texto
	// (eventhandler_message.go:130). Sem ele SendPoll RECUSA-SE a enviar, de
	// proposito — enquete criada com voto ilegivel e' pior que enquete nao
	// criada.
	chatMessenger := wachat.NewChatMessengerAdapter(waClientLookup).WithPollOptions(clientManager)
	mediaDownloader := wachat.NewMediaDownloaderAdapter(waClientLookup)
	jidResolver := wajid.NewJIDResolverAdapter()
	groupAdapter := wagroup.NewGroupAdapter(waClientLookup)
	// Declarado aqui, e nao junto dos ContactHandlers: ListChats tambem o
	// consome, e os dois tem de compartilhar a MESMA instancia.
	chatActivityRepo := db.NewChatActivityRepository(s.DB)
	miscAdapter := wamisc.NewMiscAdapter(waClientLookup)
	userAdapter := wauser.NewUserAdapter(waClientLookup)
	userRepo := db.NewUserRepository(s.DB)
	sessionGuard := wasession.NewSessionGuardAdapter(waClientLookup)
	phonePairer := wapairing.NewPhonePairerAdapter(waClientLookup)
	logger := applog.NewZerologAdapter(log.Logger)

	// Profile UseCase
	getProfileUC := profile.NewGetProfileUseCase(miscAdapter, logger)

	// Session UseCases
	connectUC := session.NewConnectUseCase(logger)
	disconnectUC := session.NewDisconnectUseCase(sessionGuard, logger)
	getQRUC := session.NewGetQRUseCase(sessionGuard, userRepo, logger)
	// O detacher e' o MESMO adapter que o orchestrator usa (Fase 2f): sem
	// ele, o logout pela API apagava o store e deixava o cliente
	// registrado, com /session/status mentindo loggedIn=true (F80).
	logoutUC := session.NewLogoutUseCase(sessionGuard, NewSessionAttachHook(s), logger)
	pairPhoneUC := session.NewPairPhoneUseCase(phonePairer, logger)
	getStatusUC := session.NewGetStatusUseCase(sessionGuard, userRepo, logger)
	setStatusMessageUC := session.NewSetStatusMessageUseCase(miscAdapter, logger)
	requestHistorySyncUC := session.NewRequestHistorySyncUseCase(miscAdapter, logger)
	syncContactRosterUC := session.NewSyncContactRosterUseCase(miscAdapter, logger)

	// Message UseCases
	linkPreviewFetcher := opengraph.NewFetcher(appCtx.GlobalHTTPClient)
	mediaFetcher := opengraph.NewURLFetcher(appCtx.GlobalHTTPClient)
	sendMessageUC := message.NewSendMessageUseCase(chatMessenger, jidResolver, linkPreviewFetcher, logger)
	sendImageUC := message.NewSendImageUseCase(chatMessenger, jidResolver, mediaFetcher, logger)
	sendDocumentUC := message.NewSendDocumentUseCase(chatMessenger, jidResolver, mediaFetcher, logger)
	// O chatMessenger entra DUAS vezes: como porta de media e como porta de
	// texto. A legenda do audio vai como mensagem de texto separada (F116),
	// porque o protocolo nao tem campo de legenda em audio.
	sendAudioUC := message.NewSendAudioUseCase(chatMessenger, jidResolver, mediaFetcher, chatMessenger, logger)
	stickerProcessor := sticker.NewProcessor()
	sendStickerUC := message.NewSendStickerUseCase(chatMessenger, jidResolver, mediaFetcher, stickerProcessor, logger)
	sendVideoUC := message.NewSendVideoUseCase(chatMessenger, jidResolver, mediaFetcher, logger)
	sendContactUC := message.NewSendContactUseCase(chatMessenger, jidResolver, logger)
	sendLocationUC := message.NewSendLocationUseCase(chatMessenger, jidResolver, logger)
	sendButtonsUC := message.NewSendButtonsUseCase(chatMessenger, jidResolver, mediaFetcher, logger)
	sendCarouselUC := message.NewSendCarouselUseCase(chatMessenger, jidResolver, mediaFetcher, logger)
	sendListUC := message.NewSendListUseCase(chatMessenger, jidResolver, logger)
	sendPollUC := message.NewSendPollUseCase(chatMessenger, jidResolver, logger)
	deleteMessageUC := message.NewDeleteMessageUseCase(chatMessenger, jidResolver, logger)
	sendEditMessageUC := message.NewSendEditMessageUseCase(chatMessenger, jidResolver, logger)
	sendTemplateUC := message.NewSendTemplateUseCase(chatMessenger, jidResolver, logger)

	// Handlers
	profileHandler := customhttp.NewProfileHandler(getProfileUC)
	// userAdapter satisfaz ContactDirectory E PrivacyManager; e' o mesmo
	// adapter que /user/privacy ja consome.
	profileFullHandler := customhttp.NewProfileFullHandler(
		profile.NewGetProfileFullUseCase(miscAdapter, userAdapter, userAdapter, logger))
	messageHandlers := &MessageHandlers{
		SendMessage:     handlers.NewSendMessageHandler(sendMessageUC),
		SendImage:       handlers.NewSendImageHandler(sendImageUC),
		SendDocument:    handlers.NewSendDocumentHandler(sendDocumentUC),
		SendAudio:       handlers.NewSendAudioHandler(sendAudioUC),
		SendSticker:     handlers.NewSendStickerHandler(sendStickerUC),
		SendVideo:       handlers.NewSendVideoHandler(sendVideoUC),
		SendContact:     handlers.NewSendContactHandler(sendContactUC),
		SendLocation:    handlers.NewSendLocationHandler(sendLocationUC),
		SendButtons:     handlers.NewSendButtonsHandler(sendButtonsUC),
		SendCarousel:    handlers.NewSendCarouselHandler(sendCarouselUC),
		SendList:        handlers.NewSendListHandler(sendListUC),
		SendPoll:        handlers.NewSendPollHandler(sendPollUC),
		DeleteMessage:   handlers.NewDeleteMessageHandler(deleteMessageUC),
		SendEditMessage: handlers.NewSendEditMessageHandler(sendEditMessageUC),
		SendTemplate:    handlers.NewSendTemplateHandler(sendTemplateUC),
	}
	sessionHandlers := &SessionHandlers{
		Connect:            initConnectHandler(connectUC, s),
		Disconnect:         handlers.NewDisconnectHandler(disconnectUC),
		GetQR:              handlers.NewGetQRHandler(getQRUC),
		Logout:             handlers.NewLogoutHandler(logoutUC),
		PairPhone:          handlers.NewPairPhoneHandler(pairPhoneUC),
		GetStatus:          handlers.NewGetStatusHandler(getStatusUC),
		SetStatusMessage:   handlers.NewSetStatusMessageHandler(setStatusMessageUC),
		RequestHistorySync: handlers.NewRequestHistorySyncHandler(requestHistorySyncUC),
		SyncContactRoster:  handlers.NewSyncContactRosterHandler(syncContactRosterUC),
		WS:                 handlers.NewWSHandler(clientManager),
	}

	// Webhook Handlers — standalone with direct DI, no longer delegate to *server.
	whCtx := &handlers.WebhookHandlerContext{
		DB:              s.DB,
		UserCache:       userinfocache,
		SupportedEvents: supportedEventTypes,
		FindInSlice:     slices.Contains[[]string, string],
		UpdateUserInfo:  updateUserInfo,
		PublishUserInfo: func(userID, token string, values interface{}) {
			v, ok := values.(Values)
			if !ok {
				log.Error().Str("userid", userID).
					Msg("webhook handler passed unexpected type to PublishUserInfo")
				return
			}
			publishUserInfo(userID, token, v)
		},
	}
	webhookHandlers := &WebhookHandlers{
		GetWebhook:    handlers.NewGetWebhookHandler(whCtx),
		SetWebhook:    handlers.NewSetWebhookHandler(whCtx),
		UpdateWebhook: handlers.NewUpdateWebhookHandler(whCtx),
		DeleteWebhook: handlers.NewDeleteWebhookHandler(whCtx),
	}

	// User UseCases
	listUsersUC := user.NewListUsersUseCase(userRepo, logger, sessionGuard)
	addUserUC := user.NewAddUserUseCase(userRepo, hmacKeyEncryptor{}, s3SecretCipher{}, logger)
	editUserUC := user.NewEditUserUseCase(userRepo, s3SecretCipher{}, userInfoRepublisher{db: s.DB}, logger)
	deleteUserUC := user.NewDeleteUserUseCase(userRepo, logger)
	checkUserUC := user.NewCheckUserUseCase(userAdapter, logger)
	getUserUC := user.NewGetUserUseCase(userAdapter, jidResolver, logger)
	getUserLIDUC := user.NewGetUserLIDUseCase(userAdapter, jidResolver, logger)
	getUserProfileUC := user.NewGetUserProfileUseCase(userAdapter, jidResolver, logger)
	listChatsUC := user.NewListChatsUseCase(chatActivityRepo, userAdapter, groupAdapter, logger)
	blockUserUC := user.NewBlockUserUseCase(userAdapter, jidResolver, logger)
	unblockUserUC := user.NewUnblockUserUseCase(userAdapter, jidResolver, logger)
	getBlocklistUC := user.NewGetBlocklistUseCase(userAdapter, logger)

	// User Handlers
	userHandlers := handlers.NewUserHandlers(
		listUsersUC,
		addUserUC,
		editUserUC,
		deleteUserUC,
		checkUserUC,
		getUserUC,
		getUserLIDUC,
		getUserProfileUC,
		listChatsUC,
		blockUserUC,
		unblockUserUC,
	)

	// Group UseCases
	groupRequestUC := group.NewGroupRequestUseCase(groupAdapter, jidResolver, logger)
	listGroupsUC := group.NewListGroupsUseCase(groupAdapter, logger)
	getGroupInfoUC := group.NewGetGroupInfoUseCase(groupAdapter, jidResolver, logger)
	getGroupInviteLinkUC := group.NewGetGroupInviteLinkUseCase(groupAdapter, jidResolver, logger)
	getGroupInviteInfoUC := group.NewGetGroupInviteInfoUseCase(groupAdapter, logger)

	// Misc UseCases (Health, Newsletter, Privacy, Call, Archive, DeleteUserComplete)
	sessionCounter := sessioncount.NewSessionCounterAdapter(clientManager)
	getHealthUC := notification.NewGetHealthUseCase(s.DB.DB, sessionCounter, logger, version)
	listNewsletterUC := notification.NewListNewsletterUseCase(miscAdapter, logger)
	newsletterOpsUC := notification.NewNewsletterOpsUseCase(miscAdapter, logger)
	deleteUserCompleteUC := user.NewDeleteUserCompleteUseCase(s.DB.DB, sessionGuard, logger, s.ExPath)
	rejectCallUC := chat.NewRejectCallUseCase(miscAdapter, jidResolver, logger)
	getPrivacySettingsUC := user.NewGetPrivacySettingsUseCase(userAdapter, logger)
	setPrivacySettingUC := user.NewSetPrivacySettingUseCase(userAdapter, logger)
	requestUnavailableMessageUC := chat.NewRequestUnavailableMessageUseCase(miscAdapter, jidResolver, logger)
	archiveChatUC := chat.NewArchiveChatUseCase(miscAdapter, jidResolver, logger)

	// Group Handlers
	groupHandlers := &handlers.GroupHandlers{
		GetGroupRequestParticipants:    handlers.NewGetGroupRequestParticipantsHandler(groupRequestUC),
		UpdateGroupRequestParticipants: handlers.NewUpdateGroupRequestParticipantsHandler(groupRequestUC),
		SetGroupJoinApprovalMode:       handlers.NewSetGroupJoinApprovalModeHandler(groupRequestUC),
		ListGroups:                     handlers.NewListGroupsHandler(listGroupsUC),
		GetGroupInfo:                   handlers.NewGetGroupInfoHandler(getGroupInfoUC),
		GetGroupInviteLink:             handlers.NewGetGroupInviteLinkHandler(getGroupInviteLinkUC),
		GetGroupInviteInfo:             handlers.NewGetGroupInviteInfoHandler(getGroupInviteInfoUC),
	}

	// Misc Handlers
	miscHandlers := &handlers.MiscHandlers{
		Health:                    handlers.NewGetHealthHandler(getHealthUC),
		ListNewsletter:            handlers.NewListNewsletterHandler(listNewsletterUC),
		DeleteUserComplete:        handlers.NewDeleteUserCompleteHandler(deleteUserCompleteUC),
		RejectCall:                handlers.NewRejectCallHandler(rejectCallUC),
		GetPrivacySettings:        handlers.NewGetPrivacySettingsHandler(getPrivacySettingsUC),
		SetPrivacySetting:         handlers.NewSetPrivacySettingHandler(setPrivacySettingUC),
		RequestUnavailableMessage: handlers.NewRequestUnavailableMessageHandler(requestUnavailableMessageUC),
		ArchiveChat:               handlers.NewArchiveChatHandler(archiveChatUC),
	}

	// Storage UseCases
	// S3 por usuário: banco + envelope de cifra + registro de clientes +
	// cache, os quatro REAIS. O stub que respondia 200 sem gravar nada é a
	// F151/F157 do HOUSEKEEP; o segredo cifrado é o ADR-0009.
	s3Store := db.NewS3ConfigRepository(s.DB)
	s3Cipher := s3SecretCipher{}
	s3Clients := s3ClientManager{}
	s3Cache := userInfoS3Cache{}
	configureS3UC := storage.NewConfigureS3UseCase(sessionGuard, s3Store, s3Cipher, s3Clients, s3Cache, logger)
	getS3ConfigUC := storage.NewGetS3ConfigUseCase(sessionGuard, s3Store, logger)
	testS3ConnectionUC := storage.NewTestS3ConnectionUseCase(sessionGuard, s3Store, s3Cipher, s3Clients, logger)
	deleteS3ConfigUC := storage.NewDeleteS3ConfigUseCase(sessionGuard, s3Store, s3Clients, s3Cache, logger)
	// HMAC por usuário: banco + cifra + cache, os três REAIS. O stub que
	// respondia 200 sem gravar nada é a F151/F157 do HOUSEKEEP.
	hmacKeyStore := db.NewHmacConfigRepository(s.DB)
	hmacEncryptor := hmacKeyEncryptor{}
	hmacCache := userInfoHmacCache{}
	configureHmacUC := storage.NewConfigureHmacUseCase(sessionGuard, hmacKeyStore, hmacEncryptor, hmacCache, logger)
	getHmacConfigUC := storage.NewGetHmacConfigUseCase(sessionGuard, hmacKeyStore, logger)
	deleteHmacConfigUC := storage.NewDeleteHmacConfigUseCase(sessionGuard, hmacKeyStore, hmacCache, logger)
	// History e proxy por usuário: banco + os DOIS caches de userinfo, todos
	// REAIS. O stub que respondia 200 sem gravar nada é a F151/F157, e a
	// publicação no cache é o que fecha a F128.
	sessionConfigStore := db.NewSessionConfigRepository(s.DB)
	sessionConfigCache := userInfoSessionCache{}
	setProxyUC := storage.NewSetProxyUseCase(sessionGuard, sessionConfigStore, sessionConfigCache,
		appCtx.GlobalWebhookUseProxy, egress.SystemResolver(), logger)
	setHistoryUC := storage.NewSetHistoryUseCase(sessionGuard, sessionConfigStore, sessionConfigCache, logger)
	getHistoryUC := storage.NewGetHistoryUseCase(sessionGuard, sessionConfigStore, logger)

	// Storage Handlers
	storageHandlers := &handlers.StorageHandlers{
		ConfigureS3:      handlers.NewConfigureS3Handler(configureS3UC),
		GetS3Config:      handlers.NewGetS3ConfigHandler(getS3ConfigUC),
		TestS3Connection: handlers.NewTestS3ConnectionHandler(testS3ConnectionUC),
		DeleteS3Config:   handlers.NewDeleteS3ConfigHandler(deleteS3ConfigUC),
		ConfigureHmac:    handlers.NewConfigureHmacHandler(configureHmacUC),
		GetHmacConfig:    handlers.NewGetHmacConfigHandler(getHmacConfigUC),
		DeleteHmacConfig: handlers.NewDeleteHmacConfigHandler(deleteHmacConfigUC),
		SetProxy:         handlers.NewSetProxyHandler(setProxyUC),
		SetHistory:       handlers.NewSetHistoryHandler(setHistoryUC),
		GetHistory:       handlers.NewGetHistoryHandler(getHistoryUC),
	}

	// Chat history handlers (/chat/history) — SEPARATE from Storage.GetHistory
	// (/webhook/history) on purpose: the two routes shared one handler and the
	// chat branch lost its implementation in the migration (HOUSEKEEP F124).
	chatHistoryRepo := db.NewChatHistoryRepository(s.DB)
	chatHistoryHandlers := &handlers.ChatHistoryHandlers{
		// WithLIDResolver liga a tradução @lid→telefone na LEITURA (F183).
		// userAdapter satisfaz LIDResolver pelo mesmo GetPNForLID que já
		// serve /user/lid/{jid} — é a peça existente, não uma nova.
		GetChatHistory: handlers.NewGetChatHistoryHandler(
			chat.NewGetChatHistoryUseCase(chatHistoryRepo, logger).
				WithLIDResolver(userAdapter)),
	}

	// Blocklist Handlers
	blocklistHandlers := &handlers.BlocklistHandlers{
		GetBlocklist: handlers.NewGetBlocklistHandler(getBlocklistUC),
	}

	// Group Management UseCase + Handlers
	groupMgmtUC := group.NewGroupManagementUseCase(
		groupAdapter, groupAdapter, groupAdapter, groupAdapter, groupAdapter,
		jidResolver, logger)
	groupMgmtHandlers := handlers.NewGroupManagementHandlers(groupMgmtUC)

	// Download Handlers (/chat/download*)
	downloadHandlers := &handlers.DownloadHandlers{
		Image:    handlers.NewDownloadImageHandler(message.NewDownloadImageUseCase(mediaDownloader, logger)),
		Video:    handlers.NewDownloadVideoHandler(message.NewDownloadVideoUseCase(mediaDownloader, logger)),
		Audio:    handlers.NewDownloadAudioHandler(message.NewDownloadAudioUseCase(mediaDownloader, logger)),
		Document: handlers.NewDownloadDocumentHandler(message.NewDownloadDocumentUseCase(mediaDownloader, logger)),
		Sticker:  handlers.NewDownloadStickerHandler(message.NewDownloadStickerUseCase(mediaDownloader, logger)),
	}

	// Presence Handlers (/user/presence, /chat/presence, /chat/markread)
	presenceHandlers := &handlers.PresenceHandlers{
		Send:      handlers.NewSendPresenceHandler(message.NewSendPresenceUseCase(presenceController, logger)),
		Subscribe: handlers.NewSubscribePresenceHandler(message.NewSubscribePresenceUseCase(presenceController, jidResolver, logger)),
		Chat:      handlers.NewChatPresenceHandler(message.NewChatPresenceUseCase(presenceController, jidResolver, logger)),
		MarkRead:  handlers.NewMarkReadHandler(message.NewMarkReadUseCase(chatMessenger, jidResolver, logger)),
	}

	// Reaction Handlers (/chat/react)
	reactionHandlers := &handlers.ReactionHandlers{
		React: handlers.NewReactHandler(message.NewReactUseCase(chatMessenger, jidResolver, logger)),
	}

	// Contact Handlers (/user/info, /user/avatar, /user/contacts)
	contactHandlers := &handlers.ContactHandlers{
		Avatar:       handlers.NewGetAvatarHandler(user.NewGetAvatarUseCase(userAdapter, jidResolver, logger)),
		Contacts:     handlers.NewGetContactsHandler(user.NewGetContactsUseCase(userAdapter, logger)),
		UserInfo:     handlers.NewGetUserInfoHandler(getUserUC),
		LastActivity: handlers.NewGetContactsLastActivityHandler(user.NewGetContactsLastActivityUseCase(chatActivityRepo, userAdapter, logger)),
	}

	customHandlerSet = &customHandlers{
		Profile:     profileHandler,
		ProfileFull: profileFullHandler,
		Message:     messageHandlers,
		Session:     sessionHandlers,
		Webhook:     webhookHandlers,
		User:        userHandlers,
		Group:       groupHandlers,
		Storage:     storageHandlers,
		Misc:        miscHandlers,
		Blocklist:   blocklistHandlers,
		Download:    downloadHandlers,
		Presence:    presenceHandlers,
		Reaction:    reactionHandlers,
		Contact:     contactHandlers,
		GroupMgmt:   groupMgmtHandlers,
		ChatHistory: chatHistoryHandlers,
		Newsletter:  handlers.NewNewsletterHandlers(newsletterOpsUC),
		Label:       handlers.NewLabelHandlers(db.NewLabelRepository(s.DB)),
	}
}

// initConnectHandler creates a ConnectHandler wired to the SessionOrchestrator.
func initConnectHandler(uc *session.ConnectUseCase, s *server) *handlers.ConnectHandler {
	h := handlers.NewConnectHandler(uc)
	return h.WithStartSession(s.startSession)
}
