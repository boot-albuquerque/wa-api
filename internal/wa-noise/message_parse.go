// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

func (cli *Client) parseMessageSource(node *waBinary.Node, requireParticipant bool) (source types.MessageSource, err error) {
	clientID := cli.getOwnID()
	clientLID := cli.getOwnLID()
	if clientID.IsEmpty() {
		err = ErrNotLoggedIn
		return
	}
	ag := node.AttrGetter()
	from := ag.JID("from")
	source.AddressingMode = types.AddressingMode(ag.OptionalString("addressing_mode"))
	if from.Server == types.GroupServer || from.Server == types.BroadcastServer {
		source.IsGroup = true
		source.Chat = from
		if requireParticipant {
			source.Sender = ag.JID("participant")
		} else {
			source.Sender = ag.OptionalJIDOrEmpty("participant")
		}
		if source.AddressingMode == types.AddressingModeLID {
			source.SenderAlt = ag.OptionalJIDOrEmpty("participant_pn")
		} else {
			source.SenderAlt = ag.OptionalJIDOrEmpty("participant_lid")
		}
		if source.Sender.User == clientID.User || source.Sender.User == clientLID.User {
			source.IsFromMe = true
		}
		if from.Server == types.BroadcastServer {
			source.BroadcastListOwner = ag.OptionalJIDOrEmpty("recipient")
			participants, ok := node.GetOptionalChildByTag("participants")
			if ok && source.IsFromMe {
				children := participants.GetChildren()
				source.BroadcastRecipients = make([]types.BroadcastRecipient, 0, len(children))
				for _, child := range children {
					if child.Tag != "to" {
						continue
					}
					cag := child.AttrGetter()
					mainJID := cag.JID("jid")
					if mainJID.Server == types.HiddenUserServer {
						source.BroadcastRecipients = append(source.BroadcastRecipients, types.BroadcastRecipient{
							LID: mainJID,
							PN:  cag.OptionalJIDOrEmpty("peer_recipient_pn"),
						})
					} else {
						source.BroadcastRecipients = append(source.BroadcastRecipients, types.BroadcastRecipient{
							LID: cag.OptionalJIDOrEmpty("peer_recipient_lid"),
							PN:  mainJID,
						})
					}
				}
			}
		}
	} else if from.Server == types.NewsletterServer {
		source.Chat = from
		source.Sender = from
		// TODO IsFromMe?
	} else if from.User == clientID.User || from.User == clientLID.User {
		if from.Server == types.HostedServer {
			from.Server = types.DefaultUserServer
		} else if from.Server == types.HostedLIDServer {
			from.Server = types.HiddenUserServer
		}
		source.IsFromMe = true
		source.Sender = from
		recipient := ag.OptionalJID("recipient")
		if recipient != nil {
			source.Chat = *recipient
		} else {
			source.Chat = from.ToNonAD()
		}
		if source.Chat.Server == types.HiddenUserServer || source.Chat.Server == types.HostedLIDServer {
			source.RecipientAlt = ag.OptionalJIDOrEmpty("peer_recipient_pn")
		} else {
			source.RecipientAlt = ag.OptionalJIDOrEmpty("peer_recipient_lid")
		}
	} else if from.IsBot() {
		source.Sender = from
		meta := node.GetChildByTag("meta")
		ag = meta.AttrGetter()
		targetChatJID := ag.OptionalJID("target_chat_jid")
		if targetChatJID != nil {
			source.Chat = targetChatJID.ToNonAD()
		} else {
			source.Chat = from
		}
	} else {
		if from.Server == types.HostedServer {
			from.Server = types.DefaultUserServer
		} else if from.Server == types.HostedLIDServer {
			from.Server = types.HiddenUserServer
		}
		source.Chat = from.ToNonAD()
		source.Sender = from
		if source.Sender.Server == types.HiddenUserServer || source.Chat.Server == types.HostedLIDServer {
			source.SenderAlt = ag.OptionalJIDOrEmpty("sender_pn")
		} else {
			source.SenderAlt = ag.OptionalJIDOrEmpty("sender_lid")
		}
	}
	if !source.SenderAlt.IsEmpty() && source.SenderAlt.Device == 0 {
		source.SenderAlt.Device = source.Sender.Device
	}
	err = ag.Error()
	return
}

func (cli *Client) parseMsgBotInfo(node waBinary.Node) (botInfo types.MsgBotInfo, err error) {
	botNode := node.GetChildByTag("bot")

	ag := botNode.AttrGetter()
	botInfo.EditType = types.BotEditType(ag.String("edit"))
	if botInfo.EditType == types.EditTypeInner || botInfo.EditType == types.EditTypeLast {
		botInfo.EditTargetID = types.MessageID(ag.String("edit_target_id"))
		botInfo.EditSenderTimestampMS = ag.UnixMilli("sender_timestamp_ms")
	}
	err = ag.Error()
	return
}

func (cli *Client) parseMsgMetaInfo(node waBinary.Node) (metaInfo types.MsgMetaInfo, err error) {
	metaNode := node.GetChildByTag("meta")

	ag := metaNode.AttrGetter()
	metaInfo.TargetID = types.MessageID(ag.OptionalString("target_id"))
	metaInfo.TargetSender = ag.OptionalJIDOrEmpty("target_sender_jid")
	metaInfo.TargetChat = ag.OptionalJIDOrEmpty("target_chat_jid")
	deprecatedLIDSession, ok := ag.GetBool("deprecated_lid_session", false)
	if ok {
		metaInfo.DeprecatedLIDSession = &deprecatedLIDSession
	}
	metaInfo.ThreadMessageID = types.MessageID(ag.OptionalString("thread_msg_id"))
	metaInfo.ThreadMessageSenderJID = ag.OptionalJIDOrEmpty("thread_msg_sender_jid")
	err = ag.Error()
	return
}

func (cli *Client) parseMessageInfo(node *waBinary.Node) (*types.MessageInfo, error) {
	var info types.MessageInfo
	var err error
	info.MessageSource, err = cli.parseMessageSource(node, true)
	if err != nil {
		return nil, err
	}
	ag := node.AttrGetter()
	info.ID = types.MessageID(ag.String("id"))
	info.ServerID = types.MessageServerID(ag.OptionalInt("server_id"))
	info.Timestamp = ag.UnixTime("t")
	info.PushName = ag.OptionalString("notify")
	info.Category = ag.OptionalString("category")
	info.Type = ag.OptionalString("type")
	info.Edit = types.EditAttribute(ag.OptionalString("edit"))
	if !ag.OK() {
		return nil, ag.Error()
	}

	for _, child := range node.GetChildren() {
		switch child.Tag {
		case "multicast":
			info.Multicast = true
		case "verified_name":
			info.VerifiedName, err = parseVerifiedNameContent(child)
			if err != nil {
				cli.Log.Warnf("Failed to parse verified_name node in %s: %v", info.ID, err)
			}
		case "bot":
			info.MsgBotInfo, err = cli.parseMsgBotInfo(child)
			if err != nil {
				cli.Log.Warnf("Failed to parse <bot> node in %s: %v", info.ID, err)
			}
		case "meta":
			info.MsgMetaInfo, err = cli.parseMsgMetaInfo(child)
			if err != nil {
				cli.Log.Warnf("Failed to parse <meta> node in %s: %v", info.ID, err)
			}
		case "franking":
			// TODO
		case "trace":
			// TODO
		default:
			if mediaType, ok := child.AttrGetter().GetString("mediatype", false); ok {
				info.MediaType = mediaType
			}
		}
	}

	return &info, nil
}
