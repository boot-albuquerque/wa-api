package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// --- MessageComposer ---------------------------------------------------

// MessageComposerNewMessageIDCall é uma chamada a NewMessageID.
type MessageComposerNewMessageIDCall struct {
	Ctx   context.Context
	TxtID string
}

// MessageComposer é o fake de port.MessageComposer — a porta dos 12 use cases
// send_*.
//
// Zero-value devolve o ID fixo DefaultMessageID, e não vazio: os use cases de
// envio tratam ID vazio como falha, e um fake que devolvesse "" faria o
// caminho feliz falhar por acidente.
type MessageComposer struct {
	SessionGuard

	NewMessageIDFunc  func(ctx context.Context, txtID string) (string, error)
	NewMessageIDCalls []MessageComposerNewMessageIDCall
}

// DefaultMessageID é o que MessageComposer devolve sem NewMessageIDFunc.
const DefaultMessageID = "generated-message-id"

var _ port.MessageComposer = (*MessageComposer)(nil)

// NewMessageID implementa port.MessageComposer.
func (f *MessageComposer) NewMessageID(ctx context.Context, txtID string) (string, error) {
	f.NewMessageIDCalls = append(f.NewMessageIDCalls, MessageComposerNewMessageIDCall{Ctx: ctx, TxtID: txtID})
	if f.NewMessageIDFunc != nil {
		return f.NewMessageIDFunc(ctx, txtID)
	}
	return DefaultMessageID, nil
}

// --- MessagingPort -----------------------------------------------------

// MessagingPortPublishMessageCall é uma chamada a PublishMessage.
type MessagingPortPublishMessageCall struct {
	Ctx        context.Context
	RoutingKey string
	Body       []byte
}

// MessagingPortPublishEventCall é uma chamada a PublishEvent.
type MessagingPortPublishEventCall struct {
	Ctx     context.Context
	Event   domain.Webhook
	Payload []byte
}

// MessagingPortPublishErrorCall é uma chamada a PublishError.
type MessagingPortPublishErrorCall struct {
	Ctx     context.Context
	ErrType string
	Payload []byte
}

// MessagingPort é o fake de port.MessagingPort.
type MessagingPort struct {
	PublishMessageFunc  func(ctx context.Context, routingKey string, body []byte) error
	PublishMessageCalls []MessagingPortPublishMessageCall

	PublishEventFunc  func(ctx context.Context, event domain.Webhook, payload []byte) error
	PublishEventCalls []MessagingPortPublishEventCall

	PublishErrorFunc  func(ctx context.Context, errType string, payload []byte) error
	PublishErrorCalls []MessagingPortPublishErrorCall
}

var _ port.MessagingPort = (*MessagingPort)(nil)

// PublishMessage implementa port.MessagingPort.
func (f *MessagingPort) PublishMessage(ctx context.Context, routingKey string, body []byte) error {
	f.PublishMessageCalls = append(f.PublishMessageCalls, MessagingPortPublishMessageCall{Ctx: ctx, RoutingKey: routingKey, Body: body})
	if f.PublishMessageFunc != nil {
		return f.PublishMessageFunc(ctx, routingKey, body)
	}
	return nil
}

// PublishEvent implementa port.MessagingPort.
func (f *MessagingPort) PublishEvent(ctx context.Context, event domain.Webhook, payload []byte) error {
	f.PublishEventCalls = append(f.PublishEventCalls, MessagingPortPublishEventCall{Ctx: ctx, Event: event, Payload: payload})
	if f.PublishEventFunc != nil {
		return f.PublishEventFunc(ctx, event, payload)
	}
	return nil
}

// PublishError implementa port.MessagingPort.
func (f *MessagingPort) PublishError(ctx context.Context, errType string, payload []byte) error {
	f.PublishErrorCalls = append(f.PublishErrorCalls, MessagingPortPublishErrorCall{Ctx: ctx, ErrType: errType, Payload: payload})
	if f.PublishErrorFunc != nil {
		return f.PublishErrorFunc(ctx, errType, payload)
	}
	return nil
}

// --- StoragePort -------------------------------------------------------

// StoragePortUploadFileCall é uma chamada a UploadFile.
type StoragePortUploadFileCall struct {
	Ctx      context.Context
	JID      domain.JID
	FileName string
	Data     []byte
	MimeType string
}

// StoragePortProcessMediaCall é uma chamada a ProcessMedia.
type StoragePortProcessMediaCall struct {
	Ctx context.Context
	Msg *domain.Message
}

// StoragePortGetS3ConfigCall é uma chamada a GetS3Config.
type StoragePortGetS3ConfigCall struct {
	UserID string
}

// StoragePort é o fake de port.StoragePort. Zero-value: UploadFile devolve
// URL vazia e GetS3Config devolve nil (S3 não configurado).
type StoragePort struct {
	UploadFileFunc  func(ctx context.Context, jid domain.JID, fileName string, data []byte, mimeType string) (string, error)
	UploadFileCalls []StoragePortUploadFileCall

	ProcessMediaFunc  func(ctx context.Context, msg *domain.Message) error
	ProcessMediaCalls []StoragePortProcessMediaCall

	GetS3ConfigFunc  func(userID string) *port.S3Config
	GetS3ConfigCalls []StoragePortGetS3ConfigCall
}

var _ port.StoragePort = (*StoragePort)(nil)

// UploadFile implementa port.StoragePort.
func (f *StoragePort) UploadFile(ctx context.Context, jid domain.JID, fileName string, data []byte, mimeType string) (string, error) {
	f.UploadFileCalls = append(f.UploadFileCalls, StoragePortUploadFileCall{Ctx: ctx, JID: jid, FileName: fileName, Data: data, MimeType: mimeType})
	if f.UploadFileFunc != nil {
		return f.UploadFileFunc(ctx, jid, fileName, data, mimeType)
	}
	return "", nil
}

// ProcessMedia implementa port.StoragePort.
func (f *StoragePort) ProcessMedia(ctx context.Context, msg *domain.Message) error {
	f.ProcessMediaCalls = append(f.ProcessMediaCalls, StoragePortProcessMediaCall{Ctx: ctx, Msg: msg})
	if f.ProcessMediaFunc != nil {
		return f.ProcessMediaFunc(ctx, msg)
	}
	return nil
}

// GetS3Config implementa port.StoragePort.
func (f *StoragePort) GetS3Config(userID string) *port.S3Config {
	f.GetS3ConfigCalls = append(f.GetS3ConfigCalls, StoragePortGetS3ConfigCall{UserID: userID})
	if f.GetS3ConfigFunc != nil {
		return f.GetS3ConfigFunc(userID)
	}
	return nil
}

// --- ProfileDataAccess -------------------------------------------------

// ProfileDataAccessProfilePictureURLCall é uma chamada a ProfilePictureURL.
type ProfileDataAccessProfilePictureURLCall struct {
	Ctx context.Context
	JID domain.JID
}

// ProfileDataAccessContactInfoCall é uma chamada a ContactInfo.
type ProfileDataAccessContactInfoCall struct {
	Ctx context.Context
	JID domain.JID
}

// ProfileDataAccess é o fake de port.ProfileDataAccess.
//
// PushName e OwnJID não gravam chamadas: são leituras puras sem argumento, e
// um contador ali não responderia a nenhuma pergunta que o teste faça. Use os
// campos PushNameValue / OwnJIDValue / OwnJIDOK para configurá-los.
type ProfileDataAccess struct {
	PushNameValue string
	OwnJIDValue   domain.JID
	OwnJIDOK      bool

	ProfilePictureURLFunc  func(ctx context.Context, jid domain.JID) (string, string, error)
	ProfilePictureURLCalls []ProfileDataAccessProfilePictureURLCall

	ContactInfoFunc  func(ctx context.Context, jid domain.JID) (string, string, error)
	ContactInfoCalls []ProfileDataAccessContactInfoCall
}

var _ port.ProfileDataAccess = (*ProfileDataAccess)(nil)

// PushName implementa port.ProfileDataAccess.
func (f *ProfileDataAccess) PushName() string { return f.PushNameValue }

// OwnJID implementa port.ProfileDataAccess.
func (f *ProfileDataAccess) OwnJID() (domain.JID, bool) { return f.OwnJIDValue, f.OwnJIDOK }

// ProfilePictureURL implementa port.ProfileDataAccess.
func (f *ProfileDataAccess) ProfilePictureURL(ctx context.Context, jid domain.JID) (string, string, error) {
	f.ProfilePictureURLCalls = append(f.ProfilePictureURLCalls, ProfileDataAccessProfilePictureURLCall{Ctx: ctx, JID: jid})
	if f.ProfilePictureURLFunc != nil {
		return f.ProfilePictureURLFunc(ctx, jid)
	}
	return "", "", nil
}

// ContactInfo implementa port.ProfileDataAccess.
func (f *ProfileDataAccess) ContactInfo(ctx context.Context, jid domain.JID) (string, string, error) {
	f.ContactInfoCalls = append(f.ContactInfoCalls, ProfileDataAccessContactInfoCall{Ctx: ctx, JID: jid})
	if f.ContactInfoFunc != nil {
		return f.ContactInfoFunc(ctx, jid)
	}
	return "", "", nil
}

// --- ProfileAccessProvider ---------------------------------------------

// ProfileAccessProviderProfileAccessCall é uma chamada a ProfileAccess.
type ProfileAccessProviderProfileAccessCall struct {
	Ctx   context.Context
	TxtID string
}

// ProfileAccessProvider é o fake de port.ProfileAccessProvider.
//
// Sem ProfileAccessFunc, devolve Access — que por sua vez é nil por padrão.
// Um teste do caminho feliz precisa preencher Access, e é justamente por isso
// que o campo existe em vez de só a Func: o caso comum é "esta sessão tem
// ESTE perfil".
type ProfileAccessProvider struct {
	SessionGuard

	// Access é o ProfileDataAccess devolvido quando ProfileAccessFunc é nil.
	Access port.ProfileDataAccess

	ProfileAccessFunc  func(ctx context.Context, txtID string) (port.ProfileDataAccess, error)
	ProfileAccessCalls []ProfileAccessProviderProfileAccessCall
}

var _ port.ProfileAccessProvider = (*ProfileAccessProvider)(nil)

// ProfileAccess implementa port.ProfileAccessProvider.
func (f *ProfileAccessProvider) ProfileAccess(ctx context.Context, txtID string) (port.ProfileDataAccess, error) {
	f.ProfileAccessCalls = append(f.ProfileAccessCalls, ProfileAccessProviderProfileAccessCall{Ctx: ctx, TxtID: txtID})
	if f.ProfileAccessFunc != nil {
		return f.ProfileAccessFunc(ctx, txtID)
	}
	return f.Access, nil
}
