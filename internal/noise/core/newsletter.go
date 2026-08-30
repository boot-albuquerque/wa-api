package core

import (
	"context"
	"encoding/json"
	"time"

	"wa-api/internal/noise/capabilities/newsletter"
	"wa-api/internal/noise/protocol/types"
)

// Fachada do dominio de canais (newsletters). A logica vive em
// internal/noise/newsletter e opera sobre newsletter.Transport; aqui ficam
// so' os metodos de *Client que delegam, mais os apelidos de tipo que preservam
// a API historica do pacote.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 2".
//
// Todos os metodos abaixo recusam receiver nil com ErrClientIsNil. Antes da
// extracao so' NewsletterMarkViewed fazia essa checagem; os demais estouravam
// nil deref. E' a mesma correcao que as Fases A-E aplicaram nos outros dominios.

// NewsletterLinkPrefix e' o MESMO valor de newsletter.LinkPrefix, nao uma
// copia: newsletter.InviteInput corta o prefixo antes de mandar a chave para o
// wire, e as duas pontas precisam concordar.
//
// Vivia em user_links.go ate' o lote 7; e' do dominio de canais, nao do de
// usuario, e mudou de arquivo junto com a extracao de user/.
const NewsletterLinkPrefix = newsletter.LinkPrefix

// CreateNewsletterParams sao os parametros de CreateNewsletter.
//
// A definicao vive em internal/noise/newsletter; aqui fica um apelido, que e'
// o mesmo tipo — chamadores externos continuam compilando sem conversao.
type CreateNewsletterParams = newsletter.CreateParams

// GetNewsletterMessagesParams sao os parametros de paginacao de
// GetNewsletterMessages. Apelido, ver CreateNewsletterParams.
type GetNewsletterMessagesParams = newsletter.GetMessagesParams

// GetNewsletterUpdatesParams sao os parametros de paginacao de
// GetNewsletterMessageUpdates. Apelido, ver CreateNewsletterParams.
type GetNewsletterUpdatesParams = newsletter.GetUpdatesParams

// NewsletterSubscribeLiveUpdates subscribes to receive live updates from a WhatsApp channel temporarily (for the duration returned).
func (cli *Client) NewsletterSubscribeLiveUpdates(ctx context.Context, jid types.JID) (time.Duration, error) {
	if cli == nil {
		return 0, ErrClientIsNil
	}
	return newsletter.SubscribeLiveUpdates(ctx, cli.newsletterT(), jid)
}

// NewsletterMarkViewed marks a channel message as viewed, incrementing the view counter.
//
// This is not the same as marking the channel as read on your other devices, use the usual MarkRead function for that.
func (cli *Client) NewsletterMarkViewed(ctx context.Context, jid types.JID, serverIDs []types.MessageServerID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.MarkViewed(ctx, cli.newsletterT(), jid, serverIDs)
}

// NewsletterSendReaction sends a reaction to a channel message.
// To remove a reaction sent earlier, set reaction to an empty string.
//
// The last parameter is the message ID of the reaction itself. It can be left empty to let noise generate a random one.
func (cli *Client) NewsletterSendReaction(ctx context.Context, jid types.JID, serverID types.MessageServerID, reaction string, messageID types.MessageID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.SendReaction(ctx, cli.newsletterT(), jid, serverID, reaction, messageID)
}

// CreateNewsletter creates a new WhatsApp channel.
func (cli *Client) CreateNewsletter(ctx context.Context, params CreateNewsletterParams) (*types.NewsletterMetadata, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return newsletter.Create(ctx, cli.newsletterT(), params)
}

// AcceptTOSNotice accepts a ToS notice.
//
// To accept the terms for creating newsletters, use
//
//	cli.AcceptTOSNotice("20601218", "5")
func (cli *Client) AcceptTOSNotice(ctx context.Context, noticeID, stage string) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.AcceptTOSNotice(ctx, cli.newsletterT(), noticeID, stage)
}

// NewsletterToggleMute changes the mute status of a newsletter.
func (cli *Client) NewsletterToggleMute(ctx context.Context, jid types.JID, mute bool) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.ToggleMute(ctx, cli.newsletterT(), jid, mute)
}

// FollowNewsletter makes the user follow (join) a WhatsApp channel.
func (cli *Client) FollowNewsletter(ctx context.Context, jid types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.Follow(ctx, cli.newsletterT(), jid)
}

// UnfollowNewsletter makes the user unfollow (leave) a WhatsApp channel.
func (cli *Client) UnfollowNewsletter(ctx context.Context, jid types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.Unfollow(ctx, cli.newsletterT(), jid)
}

// NewsletterDemoteAdmin demotes an admin of a channel to subscriber.
func (cli *Client) NewsletterDemoteAdmin(ctx context.Context, channelJID, userJID types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.DemoteAdmin(ctx, cli.newsletterT(), channelJID, userJID)
}

// NewsletterChangeOwner transfers ownership of a channel to another user.
func (cli *Client) NewsletterChangeOwner(ctx context.Context, channelJID, newOwnerJID types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.ChangeOwner(ctx, cli.newsletterT(), channelJID, newOwnerJID)
}

// NewsletterDelete permanently deletes a channel. This is IRREVERSIBLE —
// once deleted, the channel and all its messages are gone.
func (cli *Client) NewsletterDelete(ctx context.Context, channelJID types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.Delete(ctx, cli.newsletterT(), channelJID)
}

// NewsletterAdminInvite is the server's answer to an admin-invite creation:
// the invite's own ID and how long it is valid for. F261. Alias, not a
// copy: the parsing lives in newsletter.AdminInvite, where the rest of this
// capability's response parsing already lives.
type NewsletterAdminInvite = newsletter.AdminInvite

// NewsletterCreateAdminInvite creates an admin invite for a channel.
func (cli *Client) NewsletterCreateAdminInvite(ctx context.Context, channelJID, userJID types.JID) (NewsletterAdminInvite, error) {
	if cli == nil {
		return NewsletterAdminInvite{}, ErrClientIsNil
	}
	return newsletter.CreateAdminInvite(ctx, cli.newsletterT(), channelJID, userJID)
}

// NewsletterAcceptAdminInvite accepts an admin invite for a channel.
func (cli *Client) NewsletterAcceptAdminInvite(ctx context.Context, channelJID types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.AcceptAdminInvite(ctx, cli.newsletterT(), channelJID)
}

// NewsletterRevokeAdminInvite revokes an admin invite for a channel.
func (cli *Client) NewsletterRevokeAdminInvite(ctx context.Context, channelJID, userJID types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return newsletter.RevokeAdminInvite(ctx, cli.newsletterT(), channelJID, userJID)
}

// GetNewsletterInfo gets the info of a newsletter that you're joined to.
func (cli *Client) GetNewsletterInfo(ctx context.Context, jid types.JID) (*types.NewsletterMetadata, error) {
	return cli.getNewsletterInfo(ctx, newsletter.JIDInput(jid), true)
}

// GetNewsletterInfoWithInvite gets the info of a newsletter with an invite link.
//
// You can either pass the full link (https://whatsapp.com/channel/...) or just the `...` part.
//
// Note that the ViewerMeta field of the returned NewsletterMetadata will be nil.
func (cli *Client) GetNewsletterInfoWithInvite(ctx context.Context, key string) (*types.NewsletterMetadata, error) {
	return cli.getNewsletterInfo(ctx, newsletter.InviteInput(key), false)
}

// getNewsletterInfo continua existindo como metodo nao exportado porque
// internals.go (gerado) o embrulha em DangerousInternalClient.
func (cli *Client) getNewsletterInfo(ctx context.Context, input map[string]any, fetchViewerMeta bool) (*types.NewsletterMetadata, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return newsletter.GetInfo(ctx, cli.newsletterT(), input, fetchViewerMeta)
}

// GetSubscribedNewsletters gets the info of all newsletters that you're joined to.
func (cli *Client) GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return newsletter.GetSubscribed(ctx, cli.newsletterT())
}

// GetNewsletterMessages gets messages in a WhatsApp channel.
func (cli *Client) GetNewsletterMessages(ctx context.Context, jid types.JID, params *GetNewsletterMessagesParams) ([]*types.NewsletterMessage, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return newsletter.GetMessages(ctx, cli.newsletterT(), jid, params)
}

// GetNewsletterMessageUpdates gets updates in a WhatsApp channel.
//
// These are the same kind of updates that NewsletterSubscribeLiveUpdates triggers (reaction and view counts).
func (cli *Client) GetNewsletterMessageUpdates(ctx context.Context, jid types.JID, params *GetNewsletterUpdatesParams) ([]*types.NewsletterMessage, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return newsletter.GetMessageUpdates(ctx, cli.newsletterT(), jid, params)
}

// sendMexIQ continua existindo como metodo nao exportado porque internals.go
// (gerado) o embrulha em DangerousInternalClient.
func (cli *Client) sendMexIQ(ctx context.Context, queryID string, variables any) (json.RawMessage, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return newsletter.SendMexIQ(ctx, cli.newsletterT(), queryID, variables)
}
