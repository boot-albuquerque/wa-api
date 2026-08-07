// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"context"
	"fmt"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// NodeContentString devolve o conteudo textual de um no do XML binario.
//
// Existe porque Node.GetChildByTag devolve o **proprio no** quando nao acha o
// filho (binary/node.go:116), e nesse caso Content e' []waBinary.Node ou nil —
// nunca []byte. Fazer .Content.([]byte) sem comma-ok num campo opcional e',
// portanto, um panic disparavel por resposta do servidor.
//
// Regressao do lote 7 da Fase E: eram 11 leituras diretas em GetBotProfiles,
// cada uma um panic remoto.
func NodeContentString(node waBinary.Node) string {
	content, _ := node.Content.([]byte)
	return string(content)
}

// GetBotListV2 lista os bots disponiveis para a conta.
func GetBotListV2(ctx context.Context, t Transport) ([]types.BotListInfo, error) {
	resp, err := t.SendIQ(ctx, IQ{
		To:        types.ServerJID,
		Namespace: botIQNamespace,
		Type:      IQGet,
		Content: []waBinary.Node{
			{Tag: botIQNamespace, Attrs: waBinary.Attrs{"v": botListVersion}},
		},
	})
	if err != nil {
		return nil, err
	}
	botNode, ok := resp.GetOptionalChildByTag(botIQNamespace)
	if !ok {
		return nil, t.ElementMissing(botIQNamespace, "response to bot list query")
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
					t.Log().Debugf("Ignoring bot list entry with unexpected attributes: %v", ag.Error())
					continue
				}
				list = append(list, info)
			}
		}
	}

	return list, nil
}

// GetBotProfiles busca os perfis dos bots da lista.
func GetBotProfiles(ctx context.Context, t Transport, botInfo []types.BotListInfo) ([]types.BotProfileInfo, error) {
	jids := make([]types.JID, len(botInfo))
	for i, bot := range botInfo {
		jids[i] = bot.BotJID
	}

	list, err := USync(ctx, t, jids, ModeQuery, ContextInteractive, []waBinary.Node{
		{Tag: botIQNamespace, Content: []waBinary.Node{{Tag: profileNodeTag, Attrs: waBinary.Attrs{"v": botProfileVersion}}}},
	}, QueryExtras{
		BotListInfo: botInfo,
	})

	if err != nil {
		return nil, err
	}

	var profiles []types.BotProfileInfo
	for _, u := range list.GetChildren() {
		jid := u.AttrGetter().JID("jid")
		bot := u.GetChildByTag(botIQNamespace)
		profile := bot.GetChildByTag(profileNodeTag)
		name := NodeContentString(profile.GetChildByTag("name"))
		attributes := NodeContentString(profile.GetChildByTag("attributes"))
		description := NodeContentString(profile.GetChildByTag("description"))
		category := NodeContentString(profile.GetChildByTag(businessCategoryTag))
		_, isDefault := profile.GetOptionalChildByTag("default")
		personaID := profile.AttrGetter().String("persona_id")
		commandsNode := profile.GetChildByTag(botCommandsTag)
		commandDescription := NodeContentString(commandsNode.GetChildByTag("description"))
		var commands []types.BotProfileCommand
		for _, commandNode := range commandsNode.GetChildrenByTag(botCommandTag) {
			commands = append(commands, types.BotProfileCommand{
				Name:        NodeContentString(commandNode.GetChildByTag("name")),
				Description: NodeContentString(commandNode.GetChildByTag("description")),
			})
		}

		promptsNode := profile.GetChildByTag(botPromptsTag)
		var prompts []string
		for _, promptNode := range promptsNode.GetChildrenByTag(botPromptTag) {
			prompts = append(
				prompts,
				fmt.Sprintf(
					"%s %s",
					NodeContentString(promptNode.GetChildByTag("emoji")),
					NodeContentString(promptNode.GetChildByTag("text")),
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
