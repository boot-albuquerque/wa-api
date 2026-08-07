package newsletter

import (
	"context"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// GetMessagesParams e' reexportado pela raiz como GetNewsletterMessagesParams.
type GetMessagesParams struct {
	Count  int
	Before types.MessageServerID
}

// messagesAttrs monta os atributos do no <messages>. Params nil ou com campos
// zerados omite o atributo correspondente, deixando o servidor usar o padrao
// dele.
func messagesAttrs(jid types.JID, params *GetMessagesParams) waBinary.Attrs {
	attrs := waBinary.Attrs{
		"type": "jid",
		"jid":  jid,
	}
	if params != nil {
		if params.Count != 0 {
			attrs["count"] = params.Count
		}
		if params.Before != 0 {
			attrs["before"] = params.Before
		}
	}
	return attrs
}

// GetMessages busca mensagens de um canal.
func GetMessages(ctx context.Context, t Transport, jid types.JID, params *GetMessagesParams) ([]*types.NewsletterMessage, error) {
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: Namespace,
		Type:      IQGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:   messagesTag,
			Attrs: messagesAttrs(jid, params),
		}},
	})
	if err != nil {
		return nil, err
	}
	messages, ok := resp.GetOptionalChildByTag(messagesTag)
	if !ok {
		return nil, t.ElementMissing(messagesTag, messagesErrContext)
	}
	return t.ParseMessages(&messages), nil
}

// GetUpdatesParams e' reexportado pela raiz como GetNewsletterUpdatesParams.
type GetUpdatesParams struct {
	Count int
	Since time.Time
	After types.MessageServerID
}

// messageUpdatesAttrs monta os atributos do no <message_updates>. Assim como em
// messagesAttrs, campo zerado vira atributo ausente.
func messageUpdatesAttrs(params *GetUpdatesParams) waBinary.Attrs {
	attrs := waBinary.Attrs{}
	if params != nil {
		if params.Count != 0 {
			attrs["count"] = params.Count
		}
		if !params.Since.IsZero() {
			attrs["since"] = params.Since.Unix()
		}
		if params.After != 0 {
			attrs["after"] = params.After
		}
	}
	return attrs
}

// GetMessageUpdates busca updates (contadores de reacao e visualizacao) de um
// canal.
//
// Repare na assimetria com GetMessages, herdada do upstream e preservada: o
// <iq> de updates vai para o JID do canal, o de mensagens vai para o servidor; e
// o ElementMissingError de updates reporta a tag "messages" (a interna), nao
// "message_updates".
func GetMessageUpdates(ctx context.Context, t Transport, jid types.JID, params *GetUpdatesParams) ([]*types.NewsletterMessage, error) {
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: Namespace,
		Type:      IQGet,
		To:        jid,
		Content: []waBinary.Node{{
			Tag:   messageUpdatesTag,
			Attrs: messageUpdatesAttrs(params),
		}},
	})
	if err != nil {
		return nil, err
	}
	messages, ok := resp.GetOptionalChildByTag(messageUpdatesTag, messagesTag)
	if !ok {
		return nil, t.ElementMissing(messagesTag, messagesErrContext)
	}
	return t.ParseMessages(&messages), nil
}
