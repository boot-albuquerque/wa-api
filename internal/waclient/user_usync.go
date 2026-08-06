// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"fmt"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/types"
)

type UsyncQueryExtras struct {
	BotListInfo []types.BotListInfo
}

func (cli *Client) usync(ctx context.Context, jids []types.JID, mode, context string, query []waBinary.Node, extra ...UsyncQueryExtras) (*waBinary.Node, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	var extras UsyncQueryExtras
	if len(extra) > 1 {
		return nil, errors.New("only one extra parameter may be provided to usync()")
	} else if len(extra) == 1 {
		extras = extra[0]
	}

	userList := make([]waBinary.Node, len(jids))
	for i, jid := range jids {
		userList[i].Tag = "user"
		jid = jid.ToNonAD()

		switch jid.Server {
		case types.LegacyUserServer:
			userList[i].Content = []waBinary.Node{{
				Tag:     "contact",
				Content: jid.String(),
			}}
		case types.DefaultUserServer, types.HiddenUserServer:
			// NOTE: You can pass in an LID with a JID (<lid jid=...> user node)
			// Not sure if you can just put the LID in the jid tag here (works for <devices> queries mainly)
			userList[i].Attrs = waBinary.Attrs{"jid": jid}
			if jid.IsBot() {
				var personaID string
				for _, bot := range extras.BotListInfo {
					if bot.BotJID.User == jid.User {
						personaID = bot.PersonaID
					}
				}
				userList[i].Content = []waBinary.Node{{
					Tag: "bot",
					Content: []waBinary.Node{{
						Tag:   "profile",
						Attrs: waBinary.Attrs{"persona_id": personaID},
					}},
				}}
			}
		default:
			return nil, fmt.Errorf("unknown user server '%s'", jid.Server)
		}
	}
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "usync",
		Type:      "get",
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "usync",
			Attrs: waBinary.Attrs{
				"sid":     cli.generateRequestID(),
				"mode":    mode,
				"last":    "true",
				"index":   "0",
				"context": context,
			},
			Content: []waBinary.Node{
				{Tag: "query", Content: query},
				{Tag: "list", Content: userList},
			},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send usync query: %w", err)
	} else if list, ok := resp.GetOptionalChildByTag("usync", "list"); !ok {
		return nil, &ElementMissingError{Tag: "list", In: "response to usync query"}
	} else {
		return &list, err
	}
}
