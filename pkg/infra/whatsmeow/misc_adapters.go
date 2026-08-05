package whatsmeow

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"

	wa "go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

// appStateFetchTimeout é o teto de espera do pull de app-state — mais
// generoso que os 30s de ArchiveChat/RequestUnavailableMessage porque o modo
// "full" pode reprocessar um snapshot inteiro vindo do servidor.
const appStateFetchTimeout = 45 * time.Second

// MiscAdapter implementa ChatOperations, ProfileAccessProvider e
// NewsletterReader sobre o clientManager.
type MiscAdapter struct {
	*SessionGuardAdapter
}

// NewMiscAdapter cria o adapter com a função de lookup.
func NewMiscAdapter(getClient waClientGetter) *MiscAdapter {
	return &MiscAdapter{SessionGuardAdapter: NewSessionGuardAdapter(getClient)}
}

func (a *MiscAdapter) client(txtID string) (waClient, error) {
	client := a.getClient(txtID)
	if client == nil {
		return nil, ErrNoSession(txtID, nil)
	}
	return client, nil
}

// ArchiveChat arquiva ou desarquiva uma conversa.
//
// O timeout de 30s vinha do use case; ele é característica do transporte
// (SendAppState é uma ida ao servidor), não regra de negócio, e por isso
// desceu junto com a chamada.
func (a *MiscAdapter) ArchiveChat(ctx context.Context, txtID string, chat domain.JID, archive bool) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(chat)
	if err != nil {
		return err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return client.SendAppState(ctxWithTimeout, appstate.BuildArchive(jid, archive, time.Time{}, nil))
}

// RejectCall rejeita uma chamada recebida.
func (a *MiscAdapter) RejectCall(ctx context.Context, txtID string, from domain.JID, callID string) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(from)
	if err != nil {
		return err
	}
	return client.RejectCall(ctx, jid, callID)
}

// RequestUnavailableMessage pede ao par o reenvio de uma mensagem.
func (a *MiscAdapter) RequestUnavailableMessage(ctx context.Context, txtID string, chat, sender domain.JID, messageID string) (domain.UnavailableMessageAck, error) {
	client, err := a.client(txtID)
	if err != nil {
		return domain.UnavailableMessageAck{}, err
	}
	chatJID, err := toJID(chat)
	if err != nil {
		return domain.UnavailableMessageAck{}, err
	}
	senderJID, err := toJID(sender)
	if err != nil {
		return domain.UnavailableMessageAck{}, err
	}

	unavailableMessage := client.BuildUnavailableMessageRequest(chatJID, senderJID, messageID)

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.SendMessage(ctxWithTimeout, chatJID, unavailableMessage, wa.SendRequestExtra{Peer: true})
	if err != nil {
		return domain.UnavailableMessageAck{}, err
	}
	return domain.UnavailableMessageAck{RequestID: resp.ID, Timestamp: resp.Timestamp}, nil
}

// ProfileAccess devolve o acesso ao perfil da sessão.
func (a *MiscAdapter) ProfileAccess(_ context.Context, txtID string) (appport.ProfileDataAccess, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	return NewProfileDataAccessFromInterface(client), nil
}

// ListSubscribed devolve as newsletters assinadas pela sessão.
func (a *MiscAdapter) ListSubscribed(ctx context.Context, txtID string) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetSubscribedNewsletters(ctx)
	if err != nil {
		return nil, err
	}

	newsletter := make([]types.NewsletterMetadata, 0, len(resp))
	for _, info := range resp {
		if info != nil {
			newsletter = append(newsletter, *info)
		}
	}
	return newsletter, nil
}

// SyncContactRoster força o pull do patch de app-state que carrega a agenda
// de contatos (critical_unblock_low). Não mexe em histórico de mensagens —
// capacidade distinta de qualquer fluxo de history sync.
func (a *MiscAdapter) SyncContactRoster(ctx context.Context, txtID string, mode string) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, appStateFetchTimeout)
	defer cancel()

	switch mode {
	case "if_unsynced":
		return client.FetchAppState(ctxWithTimeout, appstate.WAPatchCriticalUnblockLow, false, true)
	case "incremental":
		return client.FetchAppState(ctxWithTimeout, appstate.WAPatchCriticalUnblockLow, false, false)
	case "full":
		return client.FetchAppState(ctxWithTimeout, appstate.WAPatchCriticalUnblockLow, true, false)
	default:
		return apperr.New("invalid_sync_mode", apperr.CategoryValidation,
			fmt.Sprintf("unknown contact roster sync mode %q", mode), false, nil)
	}
}

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.ChatOperations        = (*MiscAdapter)(nil)
	_ appport.ProfileAccessProvider = (*MiscAdapter)(nil)
	_ appport.NewsletterReader      = (*MiscAdapter)(nil)
	_ appport.AppStateSyncer        = (*MiscAdapter)(nil)
)
