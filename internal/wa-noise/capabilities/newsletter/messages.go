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
//
// Since deixou de ir para o fio (LIB-03, 2026-08-28): o servidor nao
// responde mais ao <message_updates> que carregava esse cursor por tempo
// (`since`), e a forma que o WA Web usa hoje so' pagina por ID de
// mensagem (`before`). O campo continua aqui para nao quebrar chamadores
// existentes, mas e' ignorado na montagem do pedido — ver o comentario em
// GetMessageUpdates.
type GetUpdatesParams struct {
	Count int
	Since time.Time
	After types.MessageServerID
}

// GetMessageUpdates busca updates (contadores de reacao e visualizacao) de um
// canal.
//
// CORRIGIDO em 2026-08-28 (LIB-03): o servidor deixou de responder ao IQ
// `<message_updates>` enderecado ao JID do canal — nao recusa, so' nao
// responde, e a chamada morria no timeout de 30s. A forma que o WA Web usa
// hoje e' identica ao IQ de GetMessages: destino o SERVIDOR (nao o
// canal), no filho `<messages type='jid' jid=… count=… before=…>` (nao
// `<message_updates>`). Reaproveita messagesAttrs/messagesTag em vez de
// duplicar o formato — as duas rotas emitem o MESMO stanza agora, so'
// diferindo na projecao da resposta que o wa-api devolve por cima (decisao
// deliberada, mantida separada por pedido explicito ao portar esta
// correcao — nao fundida com GetMessages).
//
// Since NAO tem equivalente na forma nova (que so' pagina por `before`,
// um ID de mensagem) e fica sem efeito no pedido — um chamador que so'
// preenchia Since deixa de filtrar por tempo e passa a receber a pagina
// mais recente, como se tivesse pedido sem cursor nenhum. Preenchido
// junto com After, After prevalece (e' o unico que o fio entende).
func GetMessageUpdates(ctx context.Context, t Transport, jid types.JID, params *GetUpdatesParams) ([]*types.NewsletterMessage, error) {
	var msgParams *GetMessagesParams
	if params != nil {
		msgParams = &GetMessagesParams{Count: params.Count, Before: params.After}
	}
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: Namespace,
		Type:      IQGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:   messagesTag,
			Attrs: messagesAttrs(jid, msgParams),
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
