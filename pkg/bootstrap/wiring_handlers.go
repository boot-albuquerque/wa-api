package bootstrap

import (
	"slices"
	"strconv"

	wachat "wa-api/pkg/infra/wa-noise/adapters/chat"
	wagroup "wa-api/pkg/infra/wa-noise/adapters/group"
	wamisc "wa-api/pkg/infra/wa-noise/adapters/misc"
	wapresence "wa-api/pkg/infra/wa-noise/adapters/presence"
	wauser "wa-api/pkg/infra/wa-noise/adapters/user"
	wasession "wa-api/pkg/infra/wa-noise/runtime/session"

	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/egress"
	wahistory "wa-api/pkg/infra/history"
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
	statusuc "wa-api/pkg/application/usecase/status"
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
	SendPollVote    *handlers.SendPollVoteHandler
	SendForward     *handlers.SendForwardHandler
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
	PublishStatusImage *handlers.PublishStatusImageHandler
	PublishStatusVideo *handlers.PublishStatusVideoHandler
	PublishStatusAudio *handlers.PublishStatusAudioHandler
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
	Community   *handlers.CommunityHandlers
	ChatHistory *handlers.ChatHistoryHandlers
	Newsletter  *handlers.NewsletterHandlers
	Label       *handlers.LabelHandlers
	Capability  *handlers.CapabilityHandlers
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
	pollSenderRepo := db.NewStoredMessageRepository(s.DB)
	outgoingRecorder := wahistory.NewOutgoingRecorder(
		s.DB, s.StoreDB,
		db.SaveMessageToHistory, db.TrimMessageHistory,
		func(userID string) int {
			info, found := appCtx.UserInfoCache.Get(userID)
			if !found {
				return 0
			}
			limit, _ := strconv.Atoi(info.(Values).Get("History"))
			return limit
		},
	)
	chatMessenger := wachat.NewChatMessengerAdapter(waClientLookup).
		WithPollOptions(clientManager).
		WithPollSenderLookup(pollSenderRepo).
		WithHistoryRecorder(outgoingRecorder)
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
	logger := applog.NewZerologAdapter(log.Logger)

	// Profile UseCase
	getProfileUC := profile.NewGetProfileUseCase(miscAdapter, logger)

	// Session UseCases
	connectUC := session.NewConnectUseCase(logger)
	disconnectUC := session.NewDisconnectUseCase(sessionGuard, logger)
	// The pairing surface (QR, phone code, connect) is resolved PER REQUEST
	// from the engine the request names and the target session records — see
	// pkg/pairing and HOUSEKEEP F273. There is deliberately no getQRUC/
	// pairPhoneUC here any more: a use case built at wiring time is a use case
	// bound to one engine's adapter forever, which is the defect itself.
	capabilities := capabilityregistry.NewCapabilityRegistry()
	pairingRegistry := buildPairingRegistry(s, userRepo, waClientLookup, capabilities)
	// O detacher e' o MESMO adapter que o orchestrator usa (Fase 2f): sem
	// ele, o logout pela API apagava o store e deixava o cliente
	// registrado, com /session/status mentindo loggedIn=true (F80).
	logoutUC := session.NewLogoutUseCase(sessionGuard, NewSessionAttachHook(s), logger)
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
	sendPollVoteUC := message.NewSendPollVoteUseCase(chatMessenger, jidResolver, logger)
	forwardedMsgSender := wachat.NewForwardedMessageAdapter(waClientLookup)
	storedMsgRepo := db.NewStoredMessageRepository(s.DB)
	sendForwardUC := message.NewSendForwardUseCase(chatMessenger, forwardedMsgSender, storedMsgRepo, jidResolver, logger)
	deleteMessageUC := message.NewDeleteMessageUseCase(chatMessenger, jidResolver, logger)
	sendEditMessageUC := message.NewSendEditMessageUseCase(chatMessenger, jidResolver, logger)
	sendTemplateUC := message.NewSendTemplateUseCase(chatMessenger, jidResolver, logger)

	// Status media UseCases
	publishStatusImageUC := statusuc.NewPublishStatusImageUseCase(chatMessenger, mediaFetcher, logger)
	publishStatusVideoUC := statusuc.NewPublishStatusVideoUseCase(chatMessenger, mediaFetcher, logger)
	publishStatusAudioUC := statusuc.NewPublishStatusAudioUseCase(chatMessenger, mediaFetcher, logger)

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
		SendPollVote:    handlers.NewSendPollVoteHandler(sendPollVoteUC),
		SendForward:     handlers.NewSendForwardHandler(sendForwardUC),
		DeleteMessage:   handlers.NewDeleteMessageHandler(deleteMessageUC),
		SendEditMessage: handlers.NewSendEditMessageHandler(sendEditMessageUC),
		SendTemplate:    handlers.NewSendTemplateHandler(sendTemplateUC),
	}
	sessionHandlers := &SessionHandlers{
		Connect:            handlers.NewConnectHandler(connectUC, pairingRegistry),
		Disconnect:         handlers.NewDisconnectHandler(disconnectUC),
		GetQR:              handlers.NewGetQRHandler(logger, pairingRegistry),
		Logout:             handlers.NewLogoutHandler(logoutUC),
		PairPhone:          handlers.NewPairPhoneHandler(logger, pairingRegistry),
		GetStatus:          handlers.NewGetStatusHandler(getStatusUC),
		SetStatusMessage:   handlers.NewSetStatusMessageHandler(setStatusMessageUC),
		PublishStatusImage: handlers.NewPublishStatusImageHandler(publishStatusImageUC),
		PublishStatusVideo: handlers.NewPublishStatusVideoHandler(publishStatusVideoUC),
		PublishStatusAudio: handlers.NewPublishStatusAudioHandler(publishStatusAudioUC),
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
	muteChatUC := chat.NewMuteChatUseCase(miscAdapter, jidResolver, logger)
	archiveChatUC := chat.NewArchiveChatUseCase(miscAdapter, jidResolver, logger)
	starMessageUC := chat.NewStarMessageUseCase(miscAdapter, jidResolver, logger)
	pinChatUC := chat.NewPinChatUseCase(miscAdapter, jidResolver, logger)
	setDisappearingTimerUC := chat.NewSetDisappearingTimerUseCase(miscAdapter, jidResolver, logger)
	setDefaultDisappearingTimerUC := chat.NewSetDefaultDisappearingTimerUseCase(miscAdapter, logger)

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
		Health:                      handlers.NewGetHealthHandler(getHealthUC),
		ListNewsletter:              handlers.NewListNewsletterHandler(listNewsletterUC),
		DeleteUserComplete:          handlers.NewDeleteUserCompleteHandler(deleteUserCompleteUC),
		RejectCall:                  handlers.NewRejectCallHandler(rejectCallUC),
		GetPrivacySettings:          handlers.NewGetPrivacySettingsHandler(getPrivacySettingsUC),
		SetPrivacySetting:           handlers.NewSetPrivacySettingHandler(setPrivacySettingUC),
		RequestUnavailableMessage:   handlers.NewRequestUnavailableMessageHandler(requestUnavailableMessageUC),
		MuteChat:                    handlers.NewMuteChatHandler(muteChatUC),
		ArchiveChat:                 handlers.NewArchiveChatHandler(archiveChatUC),
		PinChat:                     handlers.NewPinChatHandler(pinChatUC),
		SetDisappearingTimer:        handlers.NewSetDisappearingTimerHandler(setDisappearingTimerUC),
		SetDefaultDisappearingTimer: handlers.NewSetDefaultDisappearingTimerHandler(setDefaultDisappearingTimerUC),
		StarMessage:                 handlers.NewStarMessageHandler(starMessageUC),
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

	// Community UseCases + Handlers
	communityReadUC := group.NewCommunityReadUseCase(groupAdapter, jidResolver, logger)
	communityWriteUC := group.NewCommunityWriteUseCase(groupAdapter, jidResolver, logger)
	communityHandlers := &handlers.CommunityHandlers{
		GetSubGroups:    handlers.NewGetCommunitySubGroupsHandler(communityReadUC),
		GetParticipants: handlers.NewGetCommunityParticipantsHandler(communityReadUC),
		LinkGroup:       handlers.NewCommunityLinkGroupHandler(communityWriteUC),
		UnlinkGroup:     handlers.NewCommunityUnlinkGroupHandler(communityWriteUC),
	}

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
		Community:   communityHandlers,
		ChatHistory: chatHistoryHandlers,
		Newsletter:  handlers.NewNewsletterHandlers(newsletterOpsUC),
		Label:       handlers.NewLabelHandlers(db.NewLabelRepository(s.DB)),
		Capability:  handlers.NewCapabilityHandlers(userRepo, capabilities),
	}
}
