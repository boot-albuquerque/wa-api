package chat

import (
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"

	"context"

	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"

	"google.golang.org/protobuf/proto"
)

// Os literais do nó BIZ que acompanha o stanza de uma lista. Sem ele o
// aparelho recebe a mensagem, mas o servidor não a processa como lista
// interativa (`git show 41bc8e2^:handlers.go`, comentário na função
// SendList: "Lists require: biz > list(type=\"product_list\", v=\"2\")").
// Mesma disciplina de bizNativeFlowNodes em messenger_buttons.go: metadata
// de stanza, não de mensagem, então vive aqui e não no protobuf.
const (
	listBizNodeTag       = "biz"
	listNodeTag          = "list"
	listNodeAttrType     = "type"
	listNodeAttrVersion  = "v"
	listNodeTypeValue    = "product_list"
	listNodeVersionValue = "2"
)

// listBizNodes monta o nó BIZ que faz o servidor do WhatsApp processar a
// mensagem como lista interativa.
func listBizNodes() []waBinary.Node {
	return []waBinary.Node{{
		Tag: listBizNodeTag,
		Content: []waBinary.Node{{
			Tag: listNodeTag,
			Attrs: waBinary.Attrs{
				listNodeAttrType:    listNodeTypeValue,
				listNodeAttrVersion: listNodeVersionValue,
			},
		}},
	}}
}

// listRows traduz as linhas JÁ NORMALIZADAS do use case para o wire,
// preservando a ORDEM (é a ordem em que aparecem no aparelho de quem
// recebe). Nenhum fallback nem descarte é reaplicado aqui: o use case já
// filtrou linha sem título e já resolveu RowId pela cadeia — reaplicar
// mudaria o id que volta no clique de quem recebeu a mensagem.
func listRows(rows []domain.ListRow) []*waE2E.ListMessage_Row {
	out := make([]*waE2E.ListMessage_Row, 0, len(rows))
	for _, row := range rows {
		wireRow := &waE2E.ListMessage_Row{
			RowID: proto.String(row.RowId),
			Title: proto.String(row.Title),
		}
		if row.Description != "" {
			wireRow.Description = proto.String(row.Description)
		}
		out = append(out, wireRow)
	}
	return out
}

// listSections traduz as seções JÁ NORMALIZADAS do use case para o wire,
// preservando a ORDEM.
func listSections(sections []domain.ListSection) []*waE2E.ListMessage_Section {
	out := make([]*waE2E.ListMessage_Section, 0, len(sections))
	for _, sec := range sections {
		wireSection := &waE2E.ListMessage_Section{Rows: listRows(sec.Rows)}
		if sec.Title != "" {
			wireSection.Title = proto.String(sec.Title)
		}
		out = append(out, wireSection)
	}
	return out
}

// SendList monta um ListMessage a partir de payload e o envia para target
// (CAP-22).
//
// O EMBRULHO é o histórico e é o que faz a lista renderizar de verdade
// (`git show 41bc8e2^:handlers.go`, comentário na função SendList):
// DocumentWithCaptionMessage > FutureProofMessage > ListMessage. O embrulho
// óbvio — ViewOnceMessage > FutureProofMessage — é o que o histórico marca
// como ERRADO: sem o embrulho CORRETO a lista chega como texto simples ou
// nem chega. Vide também o nó BIZ (listBizNodes), sem o qual o servidor não
// processa a mensagem como lista.
func (a *ChatMessengerAdapter) SendList(ctx context.Context, txtID string, target domain.JID, payload domain.ListPayload, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	listMsg := &waE2E.ListMessage{
		Description: proto.String(payload.Body),
		ButtonText:  proto.String(payload.ButtonText),
		ListType:    waE2E.ListMessage_SINGLE_SELECT.Enum(),
		Sections:    listSections(payload.Sections),
	}
	if payload.Title != "" {
		listMsg.Title = proto.String(payload.Title)
	}
	if payload.Footer != "" {
		listMsg.FooterText = proto.String(payload.Footer)
	}

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{ListMessage: listMsg},
		},
	}

	nodes := listBizNodes()
	extra := wanoise.SendRequestExtra{AdditionalNodes: &nodes}
	if id != "" {
		extra.ID = types.MessageID(id)
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}
