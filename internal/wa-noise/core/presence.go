package wanoise

import (
	"context"
	"fmt"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

const (
	// presenceTypeUnavailable e o valor do atributo "type" que indica que o
	// usuario ficou offline; ausente significa disponivel.
	presenceTypeUnavailable = "unavailable"
	// presenceTypeSubscribe e o tipo do no enviado para pedir atualizacoes de
	// presenca de outro usuario.
	presenceTypeSubscribe = "subscribe"
	// presenceLastSeenDenied e o que o servidor manda em vez de um timestamp
	// quando a privacidade do outro usuario esconde o "visto por ultimo".
	presenceLastSeenDenied = "deny"
)

func (cli *Client) handleChatState(ctx context.Context, node *waBinary.Node) {
	source, err := cli.parseMessageSource(node, true)
	if err != nil {
		cli.Log.Warnf("Failed to parse chat state update: %v", err)
	} else if len(node.GetChildren()) != 1 {
		cli.Log.Warnf("Failed to parse chat state update: unexpected number of children in element (%d)", len(node.GetChildren()))
	} else {
		child := node.GetChildren()[0]
		presence := types.ChatPresence(child.Tag)
		if presence != types.ChatPresenceComposing && presence != types.ChatPresencePaused {
			cli.Log.Warnf("Unrecognized chat presence state %s", child.Tag)
		}
		media := types.ChatPresenceMedia(child.AttrGetter().OptionalString("media"))
		cli.dispatchEvent(&events.ChatPresence{
			MessageSource: source,
			State:         presence,
			Media:         media,
		})
	}
}

func (cli *Client) handlePresence(ctx context.Context, node *waBinary.Node) {
	var evt events.Presence
	ag := node.AttrGetter()
	evt.From = ag.JID("from")
	presenceType := ag.OptionalString("type")
	if presenceType == presenceTypeUnavailable {
		evt.Unavailable = true
	} else if presenceType != "" {
		cli.Log.Debugf("Unrecognized presence type '%s' in presence event from %s", presenceType, evt.From)
	}
	lastSeen := ag.OptionalString("last")
	if lastSeen != "" && lastSeen != presenceLastSeenDenied {
		evt.LastSeen = ag.UnixTime("last")
	}
	if !ag.OK() {
		cli.Log.Warnf("Error parsing presence event: %+v", ag.Errors)
	} else {
		cli.dispatchEvent(&evt)
	}
}

// SendPresence updates the user's presence status on WhatsApp.
//
// You should call this at least once after connecting so that the server has your pushname.
// Otherwise, other users will see "-" as the name.
func (cli *Client) SendPresence(ctx context.Context, state types.Presence) error {
	if cli == nil {
		return ErrClientIsNil
	} else if len(cli.Store.PushName) == 0 && cli.MessengerConfig == nil {
		return ErrNoPushName
	}
	if state == types.PresenceAvailable {
		go cli.sendUnifiedSession()
		cli.sendActiveReceipts.CompareAndSwap(0, 1)
	} else {
		cli.sendActiveReceipts.CompareAndSwap(1, 0)
	}
	attrs := waBinary.Attrs{
		"type": string(state),
	}
	// PushName not set when using WhatsApp for Messenger E2EE
	if cli.MessengerConfig == nil {
		attrs["name"] = cli.Store.PushName
	}
	return cli.sendNode(ctx, waBinary.Node{
		Tag:   "presence",
		Attrs: attrs,
	})
}

// SubscribePresence asks the WhatsApp servers to send presence updates of a specific user to this client.
//
// After subscribing to this event, you should start receiving *events.Presence for that user in normal event handlers.
//
// Also, it seems that the WhatsApp servers require you to be online to receive presence status from other users,
// so you should mark yourself as online before trying to use this function:
//
//	cli.SendPresence(types.PresenceAvailable)
func (cli *Client) SubscribePresence(ctx context.Context, jid types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	privacyToken, err := cli.Store.PrivacyTokens.GetPrivacyToken(ctx, jid)
	if err != nil {
		return fmt.Errorf("failed to get privacy token: %w", err)
	} else if privacyToken == nil {
		if cli.ErrorOnSubscribePresenceWithoutToken {
			return fmt.Errorf("%w for %v", ErrNoPrivacyToken, jid.ToNonAD())
		} else {
			cli.Log.Debugf("Trying to subscribe to presence of %s without privacy token", jid)
		}
	}
	req := waBinary.Node{
		Tag: "presence",
		Attrs: waBinary.Attrs{
			"type": presenceTypeSubscribe,
			"to":   jid,
		},
	}
	if privacyToken != nil {
		req.Content = []waBinary.Node{{
			Tag:     "tctoken",
			Content: privacyToken.Token,
		}}
	}
	return cli.sendNode(ctx, req)
}

// SendChatPresence updates the user's typing status in a specific chat.
//
// The media parameter can be set to indicate the user is recording media (like a voice message) rather than typing a text message.
func (cli *Client) SendChatPresence(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error {
	ownID := cli.getOwnID()
	if ownID.IsEmpty() {
		return ErrNotLoggedIn
	}
	content := []waBinary.Node{{Tag: string(state)}}
	if state == types.ChatPresenceComposing && len(media) > 0 {
		content[0].Attrs = waBinary.Attrs{
			"media": string(media),
		}
	}
	return cli.sendNode(ctx, waBinary.Node{
		Tag: "chatstate",
		Attrs: waBinary.Attrs{
			"from": ownID,
			"to":   jid,
		},
		Content: content,
	})
}
