package bootstrap

import (
	"slices"

	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/whatsmeow"
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
	Profile   *customhttp.ProfileHandler
	Message   *MessageHandlers
	Session   *SessionHandlers
	Webhook   *WebhookHandlers
	User      *handlers.UserHandlers
	Group     *handlers.GroupHandlers
	Storage   *handlers.StorageHandlers
	Misc      *handlers.MiscHandlers
	Blocklist *handlers.BlocklistHandlers
	Download  *handlers.DownloadHandlers
	Presence  *handlers.PresenceHandlers
	Reaction  *handlers.ReactionHandlers
	Contact   *handlers.ContactHandlers
	GroupMgmt *handlers.GroupManagementHandlers
}

var customHandlerSet = &customHandlers{}

// initCustomHandlers faz o wiring entre usecases, adapters e handlers custom.
// Chamado em main() ANTES de s.routes() (main.go:330-331) — registerCustomRoutes
// lê customHandlerSet, então a ordem é obrigatória: invertida, os campos de
// customHandlerSet estariam nil quando as rotas fossem registradas.
func initCustomHandlers(s *server) {
	// Adapters
	waClientLookup := whatsmeow.ClientForGetter(clientManager.GetWhatsmeowClient)
	messageComposer := whatsmeow.NewMessageComposerAdapter(waClientLookup)
	presenceController := whatsmeow.NewPresenceControllerAdapter(waClientLookup)
	chatMessenger := whatsmeow.NewChatMessengerAdapter(waClientLookup)
	jidResolver := whatsmeow.NewJIDResolverAdapter()
	groupAdapter := whatsmeow.NewGroupAdapter(waClientLookup)
	miscAdapter := whatsmeow.NewMiscAdapter(waClientLookup)
	userAdapter := whatsmeow.NewUserAdapter(waClientLookup)
	userRepo := db.NewUserRepository(s.DB)
	sessionGuard := whatsmeow.NewSessionGuardAdapter(waClientLookup)
	logger := whatsmeow.NewZerologAdapter(log.Logger)

	// Profile UseCase
	getProfileUC := profile.NewGetProfileUseCase(miscAdapter, logger)

	// Session UseCases
	connectUC := session.NewConnectUseCase(logger)
	disconnectUC := session.NewDisconnectUseCase(sessionGuard, logger)
	getQRUC := session.NewGetQRUseCase(sessionGuard, userRepo, logger)
	logoutUC := session.NewLogoutUseCase(sessionGuard, logger)
	pairPhoneUC := session.NewPairPhoneUseCase(sessionGuard, logger)
	getStatusUC := session.NewGetStatusUseCase(sessionGuard, sessionGuard, userRepo, logger)
	setStatusMessageUC := session.NewSetStatusMessageUseCase(sessionGuard, logger)
	requestHistorySyncUC := session.NewRequestHistorySyncUseCase(sessionGuard, logger)

	// Message UseCases
	sendMessageUC := message.NewSendMessageUseCase(messageComposer, logger)
	sendImageUC := message.NewSendImageUseCase(messageComposer, logger)
	sendDocumentUC := message.NewSendDocumentUseCase(messageComposer, logger)
	sendAudioUC := message.NewSendAudioUseCase(messageComposer, logger)
	sendStickerUC := message.NewSendStickerUseCase(messageComposer, logger)
	sendVideoUC := message.NewSendVideoUseCase(messageComposer, logger)
	sendContactUC := message.NewSendContactUseCase(messageComposer, logger)
	sendLocationUC := message.NewSendLocationUseCase(messageComposer, logger)
	sendButtonsUC := message.NewSendButtonsUseCase(messageComposer, logger)
	sendListUC := message.NewSendListUseCase(messageComposer, logger)
	sendPollUC := message.NewSendPollUseCase(messageComposer, logger)
	deleteMessageUC := message.NewDeleteMessageUseCase(sessionGuard, logger)
	sendEditMessageUC := message.NewSendEditMessageUseCase(sessionGuard, logger)
	sendTemplateUC := message.NewSendTemplateUseCase(messageComposer, logger)

	// Handlers
	profileHandler := customhttp.NewProfileHandler(getProfileUC)
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
	}

	// Webhook Handlers — standalone with direct DI, no longer delegate to *server.
	whCtx := &handlers.WebhookHandlerContext{
		DB:              s.DB,
		UserCache:       userinfocache,
		SupportedEvents: supportedEventTypes,
		FindInSlice:     slices.Contains[[]string, string],
		UpdateUserInfo:  updateUserInfo,
	}
	webhookHandlers := &WebhookHandlers{
		GetWebhook:    handlers.NewGetWebhookHandler(whCtx),
		SetWebhook:    handlers.NewSetWebhookHandler(whCtx),
		UpdateWebhook: handlers.NewUpdateWebhookHandler(whCtx),
		DeleteWebhook: handlers.NewDeleteWebhookHandler(whCtx),
	}

	// User UseCases
	listUsersUC := user.NewListUsersUseCase(userRepo, logger, sessionGuard)
	addUserUC := user.NewAddUserUseCase(userRepo, logger)
	editUserUC := user.NewEditUserUseCase(userRepo, logger)
	deleteUserUC := user.NewDeleteUserUseCase(userRepo, logger)
	checkUserUC := user.NewCheckUserUseCase(userAdapter, logger)
	getUserUC := user.NewGetUserUseCase(userAdapter, jidResolver, logger)
	getUserLIDUC := user.NewGetUserLIDUseCase(userAdapter, jidResolver, logger)
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
	sessionCounter := whatsmeow.NewSessionCounterAdapter(clientManager)
	getHealthUC := notification.NewGetHealthUseCase(s.DB.DB, sessionCounter, logger, version)
	listNewsletterUC := notification.NewListNewsletterUseCase(miscAdapter, logger)
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
	configureS3UC := storage.NewConfigureS3UseCase(sessionGuard, logger)
	getS3ConfigUC := storage.NewGetS3ConfigUseCase(sessionGuard, logger)
	testS3ConnectionUC := storage.NewTestS3ConnectionUseCase(sessionGuard, logger)
	deleteS3ConfigUC := storage.NewDeleteS3ConfigUseCase(sessionGuard, logger)
	configureHmacUC := storage.NewConfigureHmacUseCase(sessionGuard, logger)
	getHmacConfigUC := storage.NewGetHmacConfigUseCase(sessionGuard, logger)
	deleteHmacConfigUC := storage.NewDeleteHmacConfigUseCase(sessionGuard, logger)
	setProxyUC := storage.NewSetProxyUseCase(sessionGuard, logger)
	setHistoryUC := storage.NewSetHistoryUseCase(sessionGuard, logger)
	getHistoryUC := storage.NewGetHistoryUseCase(sessionGuard, logger)

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

	// Blocklist Handlers
	blocklistHandlers := &handlers.BlocklistHandlers{
		GetBlocklist: handlers.NewGetBlocklistHandler(getBlocklistUC),
	}

	// Group Management UseCase + Handlers
	groupMgmtUC := group.NewGroupManagementUseCase(groupAdapter, groupAdapter, jidResolver, logger)
	groupMgmtHandlers := handlers.NewGroupManagementHandlers(groupMgmtUC)

	// Download Handlers (/chat/download*)
	downloadHandlers := &handlers.DownloadHandlers{
		Image:    handlers.NewDownloadImageHandler(message.NewDownloadImageUseCase(sessionGuard, logger)),
		Video:    handlers.NewDownloadVideoHandler(message.NewDownloadVideoUseCase(sessionGuard, logger)),
		Audio:    handlers.NewDownloadAudioHandler(message.NewDownloadAudioUseCase(sessionGuard, logger)),
		Document: handlers.NewDownloadDocumentHandler(message.NewDownloadDocumentUseCase(sessionGuard, logger)),
		Sticker:  handlers.NewDownloadStickerHandler(message.NewDownloadStickerUseCase(sessionGuard, logger)),
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
		Avatar:   handlers.NewGetAvatarHandler(user.NewGetAvatarUseCase(userAdapter, jidResolver, logger)),
		Contacts: handlers.NewGetContactsHandler(user.NewGetContactsUseCase(userAdapter, logger)),
		UserInfo: handlers.NewGetUserInfoHandler(getUserUC),
	}

	customHandlerSet = &customHandlers{
		Profile:   profileHandler,
		Message:   messageHandlers,
		Session:   sessionHandlers,
		Webhook:   webhookHandlers,
		User:      userHandlers,
		Group:     groupHandlers,
		Storage:   storageHandlers,
		Misc:      miscHandlers,
		Blocklist: blocklistHandlers,
		Download:  downloadHandlers,
		Presence:  presenceHandlers,
		Reaction:  reactionHandlers,
		Contact:   contactHandlers,
		GroupMgmt: groupMgmtHandlers,
	}
}

// initConnectHandler creates a ConnectHandler wired to server.startClient.
func initConnectHandler(uc *session.ConnectUseCase, s *server) *handlers.ConnectHandler {
	h := handlers.NewConnectHandler(uc)
	return h.WithStartClient(s.startClient)
}
