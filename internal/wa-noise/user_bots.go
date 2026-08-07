// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// nodeContentString devolve o conteúdo textual de um nó do XML binário.
//
// Existe porque `Node.GetChildByTag` devolve o **próprio nó** quando não acha o
// filho (`binary/node.go:116`), e nesse caso `Content` é `[]waBinary.Node` ou
// `nil` — nunca `[]byte`. Fazer `.Content.([]byte)` sem comma-ok num campo
// opcional é, portanto, um panic disparável por resposta do servidor.
func nodeContentString(node waBinary.Node) string {
	content, _ := node.Content.([]byte)
	return string(content)
}

func (cli *Client) GetBotListV2(ctx context.Context) ([]types.BotListInfo, error) {
	resp, err := cli.sendIQ(ctx, infoQuery{
		To:        types.ServerJID,
		Namespace: botIQNamespace,
		Type:      iqGet,
		Content: []waBinary.Node{
			{Tag: botIQNamespace, Attrs: waBinary.Attrs{"v": botListVersion}},
		},
	})
	if err != nil {
		return nil, err
	}
	botNode, ok := resp.GetOptionalChildByTag(botIQNamespace)
	if !ok {
		return nil, &ElementMissingError{Tag: botIQNamespace, In: "response to bot list query"}
	}

	var list []types.BotListInfo

	for _, section := range botNode.GetChildrenByTag(botSectionTag) {
		if section.AttrGetter().String("type") == botSectionTypeAll {
			for _, bot := range section.GetChildrenByTag(botIQNamespace) {
				ag := bot.AttrGetter()
				info := types.BotListInfo{
					PersonaID: ag.String("persona_id"),
					BotJID:    ag.JID("jid"),
				}
				if !ag.OK() {
					cli.Log.Debugf("Ignoring bot list entry with unexpected attributes: %v", ag.Error())
					continue
				}
				list = append(list, info)
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

	list, err := cli.usync(ctx, jids, usyncModeQuery, usyncContextInteractive, []waBinary.Node{
		{Tag: botIQNamespace, Content: []waBinary.Node{{Tag: profileNodeTag, Attrs: waBinary.Attrs{"v": botProfileVersion}}}},
	}, UsyncQueryExtras{
		BotListInfo: botInfo,
	})

	if err != nil {
		return nil, err
	}

	var profiles []types.BotProfileInfo
	for _, user := range list.GetChildren() {
		jid := user.AttrGetter().JID("jid")
		bot := user.GetChildByTag(botIQNamespace)
		profile := bot.GetChildByTag(profileNodeTag)
		name := nodeContentString(profile.GetChildByTag("name"))
		attributes := nodeContentString(profile.GetChildByTag("attributes"))
		description := nodeContentString(profile.GetChildByTag("description"))
		category := nodeContentString(profile.GetChildByTag(businessCategoryTag))
		_, isDefault := profile.GetOptionalChildByTag("default")
		personaID := profile.AttrGetter().String("persona_id")
		commandsNode := profile.GetChildByTag(botCommandsTag)
		commandDescription := nodeContentString(commandsNode.GetChildByTag("description"))
		var commands []types.BotProfileCommand
		for _, commandNode := range commandsNode.GetChildrenByTag(botCommandTag) {
			commands = append(commands, types.BotProfileCommand{
				Name:        nodeContentString(commandNode.GetChildByTag("name")),
				Description: nodeContentString(commandNode.GetChildByTag("description")),
			})
		}

		promptsNode := profile.GetChildByTag(botPromptsTag)
		var prompts []string
		for _, promptNode := range promptsNode.GetChildrenByTag(botPromptTag) {
			prompts = append(
				prompts,
				fmt.Sprintf(
					"%s %s",
					nodeContentString(promptNode.GetChildByTag("emoji")),
					nodeContentString(promptNode.GetChildByTag("text")),
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
