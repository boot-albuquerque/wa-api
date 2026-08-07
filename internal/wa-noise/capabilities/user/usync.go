package user

import (
	"context"
	"errors"
	"fmt"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// QueryExtras sao os parametros opcionais da consulta usync. A raiz mantem
// `type UsyncQueryExtras = user.QueryExtras` (apelido, nao tipo novo) porque
// internals.go (gerado, fora de escopo — F29) cita o nome antigo na assinatura
// de DangerousInternalClient.Usync.
type QueryExtras struct {
	BotListInfo []types.BotListInfo
}

// USync monta e envia a consulta usync, devolvendo o no <list> da resposta.
//
// A guarda de cliente nil ficou na fachada da raiz (cli.usync), onde ela sempre
// esteve: aqui nao ha' *Client para ser nil.
func USync(
	ctx context.Context, t Transport,
	jids []types.JID, mode, queryContext string,
	query []waBinary.Node, extra ...QueryExtras,
) (*waBinary.Node, error) {
	var extras QueryExtras
	if len(extra) > 1 {
		return nil, errors.New("only one extra parameter may be provided to usync()")
	} else if len(extra) == 1 {
		extras = extra[0]
	}

	userList := make([]waBinary.Node, len(jids))
	for i, jid := range jids {
		userList[i].Tag = usyncUserTag
		jid = jid.ToNonAD()

		switch jid.Server {
		case types.LegacyUserServer:
			userList[i].Content = []waBinary.Node{{
				Tag:     contactNodeTag,
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
					Tag: botIQNamespace,
					Content: []waBinary.Node{{
						Tag:   profileNodeTag,
						Attrs: waBinary.Attrs{"persona_id": personaID},
					}},
				}}
			}
		default:
			return nil, fmt.Errorf("unknown user server '%s'", jid.Server)
		}
	}
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: usyncIQNamespace,
		Type:      IQGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: usyncNodeTag,
			Attrs: waBinary.Attrs{
				"sid":     t.GenerateRequestID(),
				"mode":    mode,
				"last":    usyncLastValue,
				"index":   usyncIndexValue,
				"context": queryContext,
			},
			Content: []waBinary.Node{
				{Tag: usyncQueryTag, Content: query},
				{Tag: usyncListTag, Content: userList},
			},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send usync query: %w", err)
	} else if list, ok := resp.GetOptionalChildByTag(usyncNodeTag, usyncListTag); !ok {
		return nil, t.ElementMissing(usyncListTag, "response to usync query")
	} else {
		return &list, err
	}
}
