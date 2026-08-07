package newsletter

import (
	"context"
	"encoding/json"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// SubscribeLiveUpdates assina os updates ao vivo de um canal temporariamente,
// pela duracao devolvida.
func SubscribeLiveUpdates(ctx context.Context, t Transport, jid types.JID) (time.Duration, error) {
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: Namespace,
		Type:      IQSet,
		To:        jid,
		Content: []waBinary.Node{{
			Tag: liveUpdatesTag,
		}},
	})
	if err != nil {
		return 0, err
	}
	child := resp.GetChildByTag(liveUpdatesTag)
	dur := child.AttrGetter().Int(liveUpdatesDurationAttr)
	return time.Duration(dur) * time.Second, nil
}

// viewedItems monta a lista de <item server_id="..."/> do recibo de
// visualizacao. Lista vazia continua produzindo um <list> vazio, como antes.
func viewedItems(serverIDs []types.MessageServerID) []waBinary.Node {
	items := make([]waBinary.Node, len(serverIDs))
	for i, id := range serverIDs {
		items[i] = waBinary.Node{
			Tag: "item",
			Attrs: waBinary.Attrs{
				"server_id": id,
			},
		}
	}
	return items
}

// MarkViewed marca mensagens de um canal como vistas, incrementando o contador
// de visualizacoes.
//
// O canal de resposta e' registrado ANTES do envio e a funcao bloqueia ate' a
// resposta chegar; o conteudo dela e' descartado (TODO herdado do upstream).
func MarkViewed(ctx context.Context, t Transport, jid types.JID, serverIDs []types.MessageServerID) error {
	items := viewedItems(serverIDs)
	reqID := t.GenerateRequestID()
	resp := t.WaitResponse(reqID)
	err := t.SendNode(ctx, waBinary.Node{
		Tag: "receipt",
		Attrs: waBinary.Attrs{
			"to":   jid,
			"type": "view",
			"id":   reqID,
		},
		Content: []waBinary.Node{{
			Tag:     "list",
			Content: items,
		}},
	})
	if err != nil {
		t.CancelResponse(reqID, resp)
		return err
	}
	// TODO handle response?
	<-resp
	return nil
}

// reactionAttrs monta os atributos do <message> e do <reaction>. Reacao vazia
// significa remover a reacao enviada antes: em vez de mandar um codigo vazio, o
// no vira uma edicao de revogacao do proprio remetente.
func reactionAttrs(jid types.JID, serverID types.MessageServerID, reaction string, messageID types.MessageID) (messageAttrs, rAttrs waBinary.Attrs) {
	rAttrs = waBinary.Attrs{}
	messageAttrs = waBinary.Attrs{
		"to":        jid,
		"id":        messageID,
		"server_id": serverID,
		"type":      "reaction",
	}
	if reaction != "" {
		rAttrs["code"] = reaction
	} else {
		messageAttrs["edit"] = string(types.EditAttributeSenderRevoke)
	}
	return messageAttrs, rAttrs
}

// SendReaction envia uma reacao a uma mensagem de canal. Para remover uma
// reacao enviada antes, passe reaction vazio.
//
// messageID e' o ID da propria reacao; vazio faz o Transport gerar um.
func SendReaction(ctx context.Context, t Transport, jid types.JID, serverID types.MessageServerID, reaction string, messageID types.MessageID) error {
	if messageID == "" {
		messageID = t.GenerateMessageID()
	}
	messageAttrs, rAttrs := reactionAttrs(jid, serverID, reaction, messageID)
	return t.SendNode(ctx, waBinary.Node{
		Tag:   "message",
		Attrs: messageAttrs,
		Content: []waBinary.Node{{
			Tag:   "reaction",
			Attrs: rAttrs,
		}},
	})
}

// CreateParams e' reexportado pela raiz como CreateNewsletterParams.
type CreateParams struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Picture     []byte `json:"picture,omitempty"`
}

type respCreateNewsletter struct {
	Newsletter *types.NewsletterMetadata `json:"xwa2_newsletter_create"`
}

// Create cria um novo canal do WhatsApp.
func Create(ctx context.Context, t Transport, params CreateParams) (*types.NewsletterMetadata, error) {
	resp, err := SendMexIQ(ctx, t, mutationCreateNewsletter, map[string]any{
		"newsletter_input": &params,
	})
	if err != nil {
		return nil, err
	}
	var respData respCreateNewsletter
	err = json.Unmarshal(resp, &respData)
	if err != nil {
		return nil, err
	}
	return respData.Newsletter, nil
}

// AcceptTOSNotice aceita um aviso de termos de uso.
//
// Para aceitar os termos de criacao de canais, use ("20601218", "5").
func AcceptTOSNotice(ctx context.Context, t Transport, noticeID, stage string) error {
	_, err := t.SendIQ(ctx, IQ{
		Namespace: "tos",
		Type:      IQSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "notice",
			Attrs: waBinary.Attrs{
				"id":    noticeID,
				"stage": stage,
			},
		}},
	})
	return err
}

// ToggleMute muda o estado de silenciamento de um canal.
func ToggleMute(ctx context.Context, t Transport, jid types.JID, mute bool) error {
	query := mutationUnmuteNewsletter
	if mute {
		query = mutationMuteNewsletter
	}
	_, err := SendMexIQ(ctx, t, query, map[string]any{
		"newsletter_id": jid.String(),
	})
	return err
}

// Follow faz o usuario seguir (entrar em) um canal.
func Follow(ctx context.Context, t Transport, jid types.JID) error {
	_, err := SendMexIQ(ctx, t, mutationFollowNewsletter, map[string]any{
		"newsletter_id": jid.String(),
	})
	return err
}

// Unfollow faz o usuario deixar de seguir (sair de) um canal.
func Unfollow(ctx context.Context, t Transport, jid types.JID) error {
	_, err := SendMexIQ(ctx, t, mutationUnfollowNewsletter, map[string]any{
		"newsletter_id": jid.String(),
	})
	return err
}
