package bootstrap

import (
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/infra/whatsmeow"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/handlers"

	"github.com/rs/zerolog/log"
	wa "go.mau.fi/whatsmeow"

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
	getProfileUC := misc.NewGetProfileUseCase(clientProvider, dataAccessFactory, logger)

	// Session UseCases
	connectUC := session.NewConnectUseCase(clientProvider, logger)
	disconnectUC := session.NewDisconnectUseCase(clientProvider, logger)
	getQRUC := session.NewGetQRUseCase(clientProvider, logger)
	logoutUC := session.NewLogoutUseCase(clientProvider, logger)
	pairPhoneUC := session.NewPairPhoneUseCase(clientProvider, logger)
	getStatusUC := session.NewGetStatusUseCase(clientProvider, logger)
	setStatusMessageUC := session.NewSetStatusMessageUseCase(clientProvider, logger)
	requestHistorySyncUC := session.NewRequestHistorySyncUseCase(clientProvider, logger)

	// Message UseCases
	sendMessageUC := message.NewSendMessageUseCase(clientProvider, logger)
	sendImageUC := message.NewSendImageUseCase(clientProvider, logger)
	sendDocumentUC := message.NewSendDocumentUseCase(clientProvider, logger)
	sendAudioUC := message.NewSendAudioUseCase(clientProvider, logger)
	sendStickerUC := message.NewSendStickerUseCase(clientProvider, logger)
	sendVideoUC := message.NewSendVideoUseCase(clientProvider, logger)
	sendContactUC := message.NewSendContactUseCase(clientProvider, logger)
	sendLocationUC := message.NewSendLocationUseCase(clientProvider, logger)
	sendButtonsUC := message.NewSendButtonsUseCase(clientProvider, logger)
	sendListUC := message.NewSendListUseCase(clientProvider, logger)
	sendPollUC := message.NewSendPollUseCase(clientProvider, logger)
	deleteMessageUC := message.NewDeleteMessageUseCase(clientProvider, logger)
	sendEditMessageUC := message.NewSendEditMessageUseCase(clientProvider, logger)
	sendTemplateUC := message.NewSendTemplateUseCase(clientProvider, logger)

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
		DB:              s.DB,
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
	listUsersUC := user.NewListUsersUseCase(s.DB, zerologLogger, cmAdapter)
	addUserUC := user.NewAddUserUseCase(s.DB, zerologLogger)
	editUserUC := user.NewEditUserUseCase(s.DB, zerologLogger)
	deleteUserUC := user.NewDeleteUserUseCase(s.DB, zerologLogger)
	checkUserUC := user.NewCheckUserUseCase(clientProvider, zerologLogger)
	getUserUC := user.NewGetUserUseCase(clientProvider, zerologLogger)
	getUserLIDUC := user.NewGetUserLIDUseCase(clientProvider, zerologLogger)
	blockUserUC := user.NewBlockUserUseCase(clientProvider, zerologLogger)
	unblockUserUC := user.NewUnblockUserUseCase(clientProvider, zerologLogger)
	getBlocklistUC := user.NewGetBlocklistUseCase(clientProvider, zerologLogger)

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
	groupRequestUC := group.NewGroupRequestUseCase(clientProvider, zerologLogger)
	listGroupsUC := group.NewListGroupsUseCase(clientProvider, logger)
	getGroupInfoUC := group.NewGetGroupInfoUseCase(clientProvider, logger)
	getGroupInviteLinkUC := group.NewGetGroupInviteLinkUseCase(clientProvider, logger)
	getGroupInviteInfoUC := group.NewGetGroupInviteInfoUseCase(clientProvider, logger)

	// Misc UseCases (Health, Newsletter, Privacy, Call, Archive, DeleteUserComplete)
	healthProvider := whatsmeow.NewHealthClientProviderAdapter(clientManager)
	getHealthUC := notification.NewGetHealthUseCase(s.DB.DB, healthProvider, zerologLogger, version)
	listNewsletterUC := notification.NewListNewsletterUseCase(clientProvider, zerologLogger)
	deleteUserCompleteUC := user.NewDeleteUserCompleteUseCase(s.DB.DB, clientProvider, healthProvider, zerologLogger, s.ExPath)
	rejectCallUC := misc.NewRejectCallUseCase(clientProvider, zerologLogger)
	getPrivacySettingsUC := user.NewGetPrivacySettingsUseCase(clientProvider, zerologLogger)
	setPrivacySettingUC := user.NewSetPrivacySettingUseCase(clientProvider, zerologLogger)
	requestUnavailableMessageUC := misc.NewRequestUnavailableMessageUseCase(clientProvider, zerologLogger)
	archiveChatUC := misc.NewArchiveChatUseCase(clientProvider, zerologLogger)

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
	configureS3UC := storage.NewConfigureS3UseCase(clientProvider, logger)
	getS3ConfigUC := storage.NewGetS3ConfigUseCase(clientProvider, logger)
	testS3ConnectionUC := storage.NewTestS3ConnectionUseCase(clientProvider, logger)
	deleteS3ConfigUC := storage.NewDeleteS3ConfigUseCase(clientProvider, logger)
	configureHmacUC := storage.NewConfigureHmacUseCase(clientProvider, logger)
	getHmacConfigUC := storage.NewGetHmacConfigUseCase(clientProvider, logger)
	deleteHmacConfigUC := storage.NewDeleteHmacConfigUseCase(clientProvider, logger)
	setProxyUC := storage.NewSetProxyUseCase(clientProvider, logger)
	setHistoryUC := storage.NewSetHistoryUseCase(clientProvider, logger)
	getHistoryUC := storage.NewGetHistoryUseCase(clientProvider, logger)

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
	groupMgmtUC := group.NewGroupManagementUseCase(clientProvider, logger)
	groupMgmtHandlers := handlers.NewGroupManagementHandlers(groupMgmtUC)

	// Extended Handlers (downloads, presence, user-info, react, mark-read)
	extendedHandlers := &handlers.ExtendedHandlers{
		DownloadImage:     handlers.NewDownloadImageHandler(message.NewDownloadImageUseCase(clientProvider, logger)),
		DownloadVideo:     handlers.NewDownloadVideoHandler(message.NewDownloadVideoUseCase(clientProvider, logger)),
		DownloadAudio:     handlers.NewDownloadAudioHandler(message.NewDownloadAudioUseCase(clientProvider, logger)),
		DownloadDocument:  handlers.NewDownloadDocumentHandler(message.NewDownloadDocumentUseCase(clientProvider, logger)),
		DownloadSticker:   handlers.NewDownloadStickerHandler(message.NewDownloadStickerUseCase(clientProvider, logger)),
		SendPresence:      handlers.NewSendPresenceHandler(message.NewSendPresenceUseCase(clientProvider, log.Logger)),
		SubscribePresence: handlers.NewSubscribePresenceHandler(message.NewSubscribePresenceUseCase(clientProvider, log.Logger)),
		ChatPresence:      handlers.NewChatPresenceHandler(message.NewChatPresenceUseCase(clientProvider, log.Logger)),
		MarkRead:          handlers.NewMarkReadHandler(message.NewMarkReadUseCase(clientProvider, log.Logger)),
		React:             handlers.NewReactHandler(message.NewReactUseCase(clientProvider, log.Logger)),
		GetAvatar:         handlers.NewGetAvatarHandler(user.NewGetAvatarUseCase(clientProvider, log.Logger)),
		GetContacts:       handlers.NewGetContactsHandler(user.NewGetContactsUseCase(clientProvider, log.Logger)),
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
