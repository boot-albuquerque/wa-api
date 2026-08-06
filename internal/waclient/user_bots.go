// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/types"
)

func (cli *Client) GetBotListV2(ctx context.Context) ([]types.BotListInfo, error) {
	resp, err := cli.sendIQ(ctx, infoQuery{
		To:        types.ServerJID,
		Namespace: "bot",
		Type:      iqGet,
		Content: []waBinary.Node{
			{Tag: "bot", Attrs: waBinary.Attrs{"v": "2"}},
		},
	})
	if err != nil {
		return nil, err
	}
	botNode, ok := resp.GetOptionalChildByTag("bot")
	if !ok {
		return nil, &ElementMissingError{Tag: "bot", In: "response to bot list query"}
	}

	var list []types.BotListInfo

	for _, section := range botNode.GetChildrenByTag("section") {
		if section.AttrGetter().String("type") == "all" {
			for _, bot := range section.GetChildrenByTag("bot") {
				ag := bot.AttrGetter()
				list = append(list, types.BotListInfo{
					PersonaID: ag.String("persona_id"),
					BotJID:    ag.JID("jid"),
				})
			}
		}
	}

	return list, nil
}

func (cli *Client) GetBotProfiles(ctx context.Context, botInfo []types.BotListInfo) ([]types.BotProfileInfo, error) {
	jids := make([]types.JID, len(botInfo))
	for i, bot := range botInfo {
		jids[i] = bot.BotJID
	}

	list, err := cli.usync(ctx, jids, "query", "interactive", []waBinary.Node{
		{Tag: "bot", Content: []waBinary.Node{{Tag: "profile", Attrs: waBinary.Attrs{"v": "1"}}}},
	}, UsyncQueryExtras{
		BotListInfo: botInfo,
	})

	if err != nil {
		return nil, err
	}

	var profiles []types.BotProfileInfo
	for _, user := range list.GetChildren() {
		jid := user.AttrGetter().JID("jid")
		bot := user.GetChildByTag("bot")
		profile := bot.GetChildByTag("profile")
		name := string(profile.GetChildByTag("name").Content.([]byte))
		attributes := string(profile.GetChildByTag("attributes").Content.([]byte))
		description := string(profile.GetChildByTag("description").Content.([]byte))
		category := string(profile.GetChildByTag("category").Content.([]byte))
		_, isDefault := profile.GetOptionalChildByTag("default")
		personaID := profile.AttrGetter().String("persona_id")
		commandsNode := profile.GetChildByTag("commands")
		commandDescription := string(commandsNode.GetChildByTag("description").Content.([]byte))
		var commands []types.BotProfileCommand
		for _, commandNode := range commandsNode.GetChildrenByTag("command") {
			commands = append(commands, types.BotProfileCommand{
				Name:        string(commandNode.GetChildByTag("name").Content.([]byte)),
				Description: string(commandNode.GetChildByTag("description").Content.([]byte)),
			})
		}

		promptsNode := profile.GetChildByTag("prompts")
		var prompts []string
		for _, promptNode := range promptsNode.GetChildrenByTag("prompt") {
			prompts = append(
				prompts,
				fmt.Sprintf(
					"%s %s",
					string(promptNode.GetChildByTag("emoji").Content.([]byte)),
					string(promptNode.GetChildByTag("text").Content.([]byte)),
				),
			)
		}

		profiles = append(profiles, types.BotProfileInfo{
			JID:                 jid,
			Name:                name,
			Attributes:          attributes,
			Description:         description,
			Category:            category,
			IsDefault:           isDefault,
			Prompts:             prompts,
			PersonaID:           personaID,
			Commands:            commands,
			CommandsDescription: commandDescription,
		})
	}

	return profiles, nil
}
