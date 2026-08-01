package bootstrap

import (
	"wa-api/pkg/infra/whatsmeow"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/handlers"

	"github.com/rs/zerolog/log"

	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/application/usecase/misc"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/application/usecase/user"
)

// ClientManagerAdapterImpl adapta o ClientManager global para a interface ClientManagerAdapter.
// Accepts the ClientLookup interface so both *main.ClientManager (root) and
// *whatsmeow.ClientManager satisfy it without a concrete-type dependency.
type ClientManagerAdapterImpl struct {
	cm whatsmeow.ClientLookup
}

func (a *ClientManagerAdapterImpl) GetWhatsmeowClient(id string) interface{} {
	return a.cm.GetWhatsmeowClient(id)
}

func (a *ClientManagerAdapterImpl) IsConnected(id string) bool {
	client := a.cm.GetWhatsmeowClient(id)
	if client == nil {
		return false
	}
	return client.IsConnected()
}

func (a *ClientManagerAdapterImpl) IsLoggedIn(id string) bool {
	client := a.cm.GetWhatsmeowClient(id)
	if client == nil {
		return false
	}
	return client.IsLoggedIn()
}

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
	Extended  *handlers.ExtendedHandlers
	GroupMgmt *handlers.GroupManagementHandlers
}

var customHandlerSet = &customHandlers{}

// initCustomHandlers faz o wiring entre usecases, adapters e handlers custom.
// Chamado em main() ANTES de s.routes() (main.go:330-331) — registerCustomRoutes
// lê customHandlerSet, então a ordem é obrigatória: invertida, os campos de
// customHandlerSet estariam nil quando as rotas fossem registradas.
func initCustomHandlers(s *server) {
	// Adapters
	clientProvider := whatsmeow.NewClientProviderAdapter(clientManager.GetWhatsmeowClient)
	messageComposer := whatsmeow.NewMessageComposerAdapter(clientManager.GetWhatsmeowClient)
	presenceController := whatsmeow.NewPresenceControllerAdapter(clientManager.GetWhatsmeowClient)
	chatMessenger := whatsmeow.NewChatMessengerAdapter(clientManager.GetWhatsmeowClient)
	jidResolver := whatsmeow.NewJIDResolverAdapter()
	groupAdapter := whatsmeow.NewGroupAdapter(clientManager.GetWhatsmeowClient)
	miscAdapter := whatsmeow.NewMiscAdapter(clientManager.GetWhatsmeowClient)
	userAdapter := whatsmeow.NewUserAdapter(clientManager.GetWhatsmeowClient)
	sessionGuard := whatsmeow.NewSessionGuardAdapter(clientManager.GetWhatsmeowClient)
	logger := whatsmeow.NewZerologAdapter(log.Logger)

	// Profile UseCase
	getProfileUC := misc.NewGetProfileUseCase(miscAdapter, logger)

	// Session UseCases
	connectUC := session.NewConnectUseCase(logger)
	disconnectUC := session.NewDisconnectUseCase(sessionGuard, logger)
	getQRUC := session.NewGetQRUseCase(sessionGuard, logger)
	logoutUC := session.NewLogoutUseCase(sessionGuard, logger)
	pairPhoneUC := session.NewPairPhoneUseCase(sessionGuard, logger)
	getStatusUC := session.NewGetStatusUseCase(sessionGuard, logger)
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
		FindInSlice:     Find,
		UpdateUserInfo:  updateUserInfo,
	}
	webhookHandlers := &WebhookHandlers{
		GetWebhook:    handlers.NewGetWebhookHandler(whCtx),
		SetWebhook:    handlers.NewSetWebhookHandler(whCtx),
		UpdateWebhook: handlers.NewUpdateWebhookHandler(whCtx),
		DeleteWebhook: handlers.NewDeleteWebhookHandler(whCtx),
	}

	// User UseCases
	cmAdapter := &ClientManagerAdapterImpl{cm: clientManager}
	listUsersUC := user.NewListUsersUseCase(s.DB, logger, cmAdapter)
	addUserUC := user.NewAddUserUseCase(s.DB, logger)
	editUserUC := user.NewEditUserUseCase(s.DB, logger)
	deleteUserUC := user.NewDeleteUserUseCase(s.DB, logger)
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
	deleteUserCompleteUC := user.NewDeleteUserCompleteUseCase(s.DB.DB, clientProvider, logger, s.ExPath)
	rejectCallUC := misc.NewRejectCallUseCase(miscAdapter, jidResolver, logger)
	getPrivacySettingsUC := user.NewGetPrivacySettingsUseCase(userAdapter, logger)
	setPrivacySettingUC := user.NewSetPrivacySettingUseCase(userAdapter, logger)
	requestUnavailableMessageUC := misc.NewRequestUnavailableMessageUseCase(miscAdapter, jidResolver, logger)
	archiveChatUC := misc.NewArchiveChatUseCase(miscAdapter, jidResolver, logger)

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

	// Extended Handlers (downloads, presence, user-info, react, mark-read)
	extendedHandlers := &handlers.ExtendedHandlers{
		DownloadImage:     handlers.NewDownloadImageHandler(message.NewDownloadImageUseCase(sessionGuard, logger)),
		DownloadVideo:     handlers.NewDownloadVideoHandler(message.NewDownloadVideoUseCase(sessionGuard, logger)),
		DownloadAudio:     handlers.NewDownloadAudioHandler(message.NewDownloadAudioUseCase(sessionGuard, logger)),
		DownloadDocument:  handlers.NewDownloadDocumentHandler(message.NewDownloadDocumentUseCase(sessionGuard, logger)),
		DownloadSticker:   handlers.NewDownloadStickerHandler(message.NewDownloadStickerUseCase(sessionGuard, logger)),
		SendPresence:      handlers.NewSendPresenceHandler(message.NewSendPresenceUseCase(presenceController, logger)),
		SubscribePresence: handlers.NewSubscribePresenceHandler(message.NewSubscribePresenceUseCase(presenceController, jidResolver, logger)),
		ChatPresence:      handlers.NewChatPresenceHandler(message.NewChatPresenceUseCase(presenceController, jidResolver, logger)),
		MarkRead:          handlers.NewMarkReadHandler(message.NewMarkReadUseCase(chatMessenger, jidResolver, logger)),
		React:             handlers.NewReactHandler(message.NewReactUseCase(chatMessenger, jidResolver, logger)),
		GetAvatar:         handlers.NewGetAvatarHandler(user.NewGetAvatarUseCase(userAdapter, jidResolver, logger)),
		GetContacts:       handlers.NewGetContactsHandler(user.NewGetContactsUseCase(userAdapter, logger)),
		GetUserInfo:       handlers.NewGetUserInfoHandler(getUserUC),
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
		Extended:  extendedHandlers,
		GroupMgmt: groupMgmtHandlers,
	}
}

// initConnectHandler creates a ConnectHandler wired to server.startClient.
func initConnectHandler(uc *session.ConnectUseCase, s *server) *handlers.ConnectHandler {
	h := handlers.NewConnectHandler(uc)
	return h.WithStartClient(s.startClient)
}
