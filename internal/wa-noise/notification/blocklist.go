// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package notification

import (
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types/events"
)

// HandleBlocklist traduz o <blocklist> (que chega dentro de um
// <notification type="account_sync">) em um events.Blocklist.
//
// Um filho com atributos invalidos e' pulado com aviso, mas nao aborta o
// evento: o resto das mudancas ainda e' entregue. Comportamento do upstream,
// preservado verbatim.
func HandleBlocklist(t Transport, node *waBinary.Node) {
	ag := node.AttrGetter()
	evt := events.Blocklist{
		Action:    events.BlocklistAction(ag.OptionalString("action")),
		DHash:     ag.String("dhash"),
		PrevDHash: ag.OptionalString("prev_dhash"),
	}
	for _, child := range node.GetChildren() {
		ag := child.AttrGetter()
		change := events.BlocklistChange{
			JID:    ag.JID("jid"),
			Action: events.BlocklistChangeAction(ag.String("action")),
		}
		if !ag.OK() {
			t.Log().Warnf("Unexpected data in blocklist event child %v: %v", child.XMLString(), ag.Error())
			continue
		}
		evt.Changes = append(evt.Changes, change)
	}
	t.DispatchEvent(&evt)
}
