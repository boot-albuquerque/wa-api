package wuzapi

import (
	appport "wuzapi/internal/application/port"
	"wuzapi/internal/application/usecase"
	"wuzapi/internal/infrastructure/whatsmeow"
	customhttp "wuzapi/internal/interfaces/http"
	"wuzapi/internal/interfaces/http/handlers"

	"github.com/rs/zerolog/log"
	wa "go.mau.fi/whatsmeow"
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
	Profile  *customhttp.ProfileHandler
	Message  *MessageHandlers
	Session  *SessionHandlers
	Webhook  *WebhookHandlers
	User     *handlers.UserHandlers
	Group    *handlers.GroupHandlers
	Storage  *handlers.StorageHandlers
	Misc     *handlers.MiscHandlers
	Blocklist *handlers.BlocklistHandlers
	Extended  *handlers.ExtendedHandlers
	GroupMgmt *handlers.GroupManagementHandlers
}

var customHandlerSet = &customHandlers{}

// initCustomHandlers faz o wiring entre usecases, adapters e handlers custom.
// Chamado em main() logo após s.routes(), antes de connectOnStartup().
func initCustomHandlers(s *server) {
	// Adapters
	clientProvider := whatsmeow.NewClientProviderAdapter(clientManager.GetWhatsmeowClient)
	logger := whatsmeow.NewZerologAdapter(log.Logger)
	zerologLogger := log.Logger

	// Factory que cria ProfileDataAccess a partir de *whatsmeow.Client
	dataAccessFactory := func(c *wa.Client) appport.ProfileDataAccess {
		return whatsmeow.NewProfileDataAccess(c)
	}

	// Profile UseCase
	getProfileUC := usecase.NewGetProfileUseCase(clientProvider, dataAccessFactory, logger)

	// Session UseCases
	connectUC := usecase.NewConnectUseCase(clientProvider, logger)
	disconnectUC := usecase.NewDisconnectUseCase(clientProvider, logger)
	getQRUC := usecase.NewGetQRUseCase(clientProvider, logger)
	logoutUC := usecase.NewLogoutUseCase(clientProvider, logger)
	pairPhoneUC := usecase.NewPairPhoneUseCase(clientProvider, logger)
	getStatusUC := usecase.NewGetStatusUseCase(clientProvider, logger)
	setStatusMessageUC := usecase.NewSetStatusMessageUseCase(clientProvider, logger)
	requestHistorySyncUC := usecase.NewRequestHistorySyncUseCase(clientProvider, logger)

	// Message UseCases
	sendMessageUC := usecase.NewSendMessageUseCase(clientProvider, logger)
	sendImageUC := usecase.NewSendImageUseCase(clientProvider, logger)
	sendDocumentUC := usecase.NewSendDocumentUseCase(clientProvider, logger)
	sendAudioUC := usecase.NewSendAudioUseCase(clientProvider, logger)
	sendStickerUC := usecase.NewSendStickerUseCase(clientProvider, logger)
	sendVideoUC := usecase.NewSendVideoUseCase(clientProvider, logger)
	sendContactUC := usecase.NewSendContactUseCase(clientProvider, logger)
	sendLocationUC := usecase.NewSendLocationUseCase(clientProvider, logger)
	sendButtonsUC := usecase.NewSendButtonsUseCase(clientProvider, logger)
	sendListUC := usecase.NewSendListUseCase(clientProvider, logger)
	sendPollUC := usecase.NewSendPollUseCase(clientProvider, logger)
	deleteMessageUC := usecase.NewDeleteMessageUseCase(clientProvider, logger)
	sendEditMessageUC := usecase.NewSendEditMessageUseCase(clientProvider, logger)
	sendTemplateUC := usecase.NewSendTemplateUseCase(clientProvider, logger)

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
		Connect:            handlers.NewConnectHandler(connectUC),
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
		DB:              s.db,
		UserCache:       userinfocache,
		SupportedEvents: supportedEventTypes,
		FindInSlice:     Find,
		UpdateUserInfo:  updateUserInfo,
		RespondJSON:     respondJSON,
	}
	webhookHandlers := &WebhookHandlers{
		GetWebhook:    handlers.NewGetWebhookHandler(whCtx),
		SetWebhook:    handlers.NewSetWebhookHandler(whCtx),
		UpdateWebhook: handlers.NewUpdateWebhookHandler(whCtx),
		DeleteWebhook: handlers.NewDeleteWebhookHandler(whCtx),
	}

	// User UseCases
	cmAdapter := &ClientManagerAdapterImpl{cm: clientManager}
	listUsersUC := usecase.NewListUsersUseCase(s.db, zerologLogger, cmAdapter)
	addUserUC := usecase.NewAddUserUseCase(s.db, zerologLogger)
	editUserUC := usecase.NewEditUserUseCase(s.db, zerologLogger)
	deleteUserUC := usecase.NewDeleteUserUseCase(s.db, zerologLogger)
	checkUserUC := usecase.NewCheckUserUseCase(clientProvider, zerologLogger)
	getUserUC := usecase.NewGetUserUseCase(clientProvider, zerologLogger)
	getUserLIDUC := usecase.NewGetUserLIDUseCase(clientProvider, zerologLogger)
	blockUserUC := usecase.NewBlockUserUseCase(clientProvider, zerologLogger)
	unblockUserUC := usecase.NewUnblockUserUseCase(clientProvider, zerologLogger)
	getBlocklistUC := usecase.NewGetBlocklistUseCase(clientProvider, zerologLogger)

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
	groupRequestUC := usecase.NewGroupRequestUseCase(clientProvider, zerologLogger)
	listGroupsUC := usecase.NewListGroupsUseCase(clientProvider, logger)
	getGroupInfoUC := usecase.NewGetGroupInfoUseCase(clientProvider, logger)
	getGroupInviteLinkUC := usecase.NewGetGroupInviteLinkUseCase(clientProvider, logger)
	getGroupInviteInfoUC := usecase.NewGetGroupInviteInfoUseCase(clientProvider, logger)

	// Misc UseCases (Health, Newsletter, Privacy, Call, Archive, DeleteUserComplete)
	healthProvider := whatsmeow.NewHealthClientProviderAdapter(clientManager)
	getHealthUC := usecase.NewGetHealthUseCase(s.db.DB, healthProvider, zerologLogger, version)
	listNewsletterUC := usecase.NewListNewsletterUseCase(clientProvider, zerologLogger)
	deleteUserCompleteUC := usecase.NewDeleteUserCompleteUseCase(s.db.DB, clientProvider, healthProvider, zerologLogger, s.exPath)
	rejectCallUC := usecase.NewRejectCallUseCase(clientProvider, zerologLogger)
	getPrivacySettingsUC := usecase.NewGetPrivacySettingsUseCase(clientProvider, zerologLogger)
	setPrivacySettingUC := usecase.NewSetPrivacySettingUseCase(clientProvider, zerologLogger)
	requestUnavailableMessageUC := usecase.NewRequestUnavailableMessageUseCase(clientProvider, zerologLogger)
	archiveChatUC := usecase.NewArchiveChatUseCase(clientProvider, zerologLogger)

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
	configureS3UC := usecase.NewConfigureS3UseCase(clientProvider, logger)
	getS3ConfigUC := usecase.NewGetS3ConfigUseCase(clientProvider, logger)
	testS3ConnectionUC := usecase.NewTestS3ConnectionUseCase(clientProvider, logger)
	deleteS3ConfigUC := usecase.NewDeleteS3ConfigUseCase(clientProvider, logger)
	configureHmacUC := usecase.NewConfigureHmacUseCase(clientProvider, logger)
	getHmacConfigUC := usecase.NewGetHmacConfigUseCase(clientProvider, logger)
	deleteHmacConfigUC := usecase.NewDeleteHmacConfigUseCase(clientProvider, logger)
	setProxyUC := usecase.NewSetProxyUseCase(clientProvider, logger)
	setHistoryUC := usecase.NewSetHistoryUseCase(clientProvider, logger)
	getHistoryUC := usecase.NewGetHistoryUseCase(clientProvider, logger)

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
	groupMgmtUC := usecase.NewGroupManagementUseCase(clientProvider, logger)
	groupMgmtHandlers := handlers.NewGroupManagementHandlers(groupMgmtUC)

	// Extended Handlers (downloads, presence, user-info, react, mark-read)
	extendedHandlers := &handlers.ExtendedHandlers{
		DownloadImage:     handlers.NewDownloadImageHandler(usecase.NewDownloadImageUseCase(clientProvider, logger)),
		DownloadVideo:     handlers.NewDownloadVideoHandler(usecase.NewDownloadVideoUseCase(clientProvider, logger)),
		DownloadAudio:     handlers.NewDownloadAudioHandler(usecase.NewDownloadAudioUseCase(clientProvider, logger)),
		DownloadDocument:  handlers.NewDownloadDocumentHandler(usecase.NewDownloadDocumentUseCase(clientProvider, logger)),
		DownloadSticker:   handlers.NewDownloadStickerHandler(usecase.NewDownloadStickerUseCase(clientProvider, logger)),
		SendPresence:      handlers.NewSendPresenceHandler(usecase.NewSendPresenceUseCase(clientProvider, log.Logger)),
		SubscribePresence: handlers.NewSubscribePresenceHandler(usecase.NewSubscribePresenceUseCase(clientProvider, log.Logger)),
		ChatPresence:      handlers.NewChatPresenceHandler(usecase.NewChatPresenceUseCase(clientProvider, log.Logger)),
		MarkRead:          handlers.NewMarkReadHandler(usecase.NewMarkReadUseCase(clientProvider, log.Logger)),
		React:             handlers.NewReactHandler(usecase.NewReactUseCase(clientProvider, log.Logger)),
		GetAvatar:         handlers.NewGetAvatarHandler(usecase.NewGetAvatarUseCase(clientProvider, log.Logger)),
		GetContacts:       handlers.NewGetContactsHandler(usecase.NewGetContactsUseCase(clientProvider, log.Logger)),
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
