// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package notification

import (
	"encoding/json"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// ParseNewsletterMessages le a lista de <message> de um <live_updates> de
// newsletter.
//
// Devolve sempre uma fatia nao-nil (ainda que vazia) — o `make` com capacidade
// e' do upstream e o teste da Fase E lote 5 trava isso, porque um nil aqui
// mudaria a serializacao JSON do evento de `[]` para `null`.
//
// Uma carga <plaintext> que nao desserializa **nao** descarta a mensagem: o
// campo Message volta a nil e o resto dos metadados (ID, tipo, contagens)
// continua sendo entregue.
func ParseNewsletterMessages(t Transport, node *waBinary.Node) []*types.NewsletterMessage {
	children := node.GetChildren()
	output := make([]*types.NewsletterMessage, 0, len(children))
	for _, child := range children {
		if child.Tag != "message" {
			continue
		}
		ag := child.AttrGetter()
		msg := types.NewsletterMessage{
			MessageServerID: ag.Int("server_id"),
			MessageID:       ag.String("id"),
			Type:            ag.String("type"),
			Timestamp:       ag.UnixTime("t"),
			ViewsCount:      0,
			ReactionCounts:  nil,
		}
		for _, subchild := range child.GetChildren() {
			switch subchild.Tag {
			case "plaintext":
				byteContent, ok := subchild.Content.([]byte)
				if ok {
					msg.Message = new(waE2E.Message)
					err := proto.Unmarshal(byteContent, msg.Message)
					if err != nil {
						t.Log().Warnf("Failed to unmarshal newsletter message: %v", err)
						msg.Message = nil
					}
				}
			case "views_count":
				msg.ViewsCount = subchild.AttrGetter().Int("count")
			case "reactions":
				msg.ReactionCounts = make(map[string]int)
				for _, reaction := range subchild.GetChildren() {
					rag := reaction.AttrGetter()
					msg.ReactionCounts[rag.String("code")] = rag.Int("count")
				}
			}
		}
		output = append(output, &msg)
	}
	return output
}

// HandleNewsletter traduz um <notification type="newsletter"> em um
// events.NewsletterLiveUpdate.
//
// GetChildByTag devolve o no zero quando <live_updates> nao existe, e
// ParseNewsletterMessages de um no vazio devolve fatia vazia — logo a ausencia
// do filho vira um evento com lista vazia, e nao um erro. Comportamento do
// upstream.
func HandleNewsletter(t Transport, node *waBinary.Node) {
	ag := node.AttrGetter()
	liveUpdates := node.GetChildByTag("live_updates")
	t.DispatchEvent(&events.NewsletterLiveUpdate{
		JID:      ag.JID("from"),
		Time:     ag.UnixTime("t"),
		Messages: ParseNewsletterMessages(t, &liveUpdates),
	})
}

// eventWrapper e' o envelope `{"data": {...}}` dos updates mex.
type eventWrapper struct {
	Data mexEvent `json:"data"`
}

// mexEvent e' o conjunto de eventos de newsletter que sabemos tratar. Os
// campos comentados sao os que o servidor manda e que o upstream ainda nao
// mapeou; ficam registrados para que quem for implementa-los saiba os nomes.
type mexEvent struct {
	Join       *events.NewsletterJoin       `json:"xwa2_notify_newsletter_on_join"`
	Leave      *events.NewsletterLeave      `json:"xwa2_notify_newsletter_on_leave"`
	MuteChange *events.NewsletterMuteChange `json:"xwa2_notify_newsletter_on_mute_change"`
	// _on_admin_metadata_update -> id, thread_metadata, messages
	// _on_metadata_update
	// _on_state_change -> id, is_requestor, state
}

// HandleMex roteia os <update> de um <notification type="mex">.
//
// No maximo **um** evento e' despachado por <update>, mesmo que o JSON traga
// mais de um campo preenchido: a cadeia else-if e' do upstream e a ordem
// (join, leave, mute) e' a prioridade efetiva. Filhos que nao sao <update>, que
// nao tem conteudo binario, ou cujo JSON nao desserializa sao pulados sem
// abortar os demais.
func HandleMex(t Transport, node *waBinary.Node) {
	for _, child := range node.GetChildren() {
		if child.Tag != "update" {
			continue
		}
		childData, ok := child.Content.([]byte)
		if !ok {
			continue
		}
		var wrapper eventWrapper
		err := json.Unmarshal(childData, &wrapper)
		if err != nil {
			t.Log().Errorf("Failed to unmarshal JSON in mex event: %v", err)
			continue
		}
		if wrapper.Data.Join != nil {
			t.DispatchEvent(wrapper.Data.Join)
		} else if wrapper.Data.Leave != nil {
			t.DispatchEvent(wrapper.Data.Leave)
		} else if wrapper.Data.MuteChange != nil {
			t.DispatchEvent(wrapper.Data.MuteChange)
		}
	}
}
