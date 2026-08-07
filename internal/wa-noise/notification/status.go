// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package notification

import (
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// HandleStatus traduz um <notification type="status"> com filho <set> em um
// events.UserAbout (o texto de "recado" do contato).
//
// Sem o filho <set> ou com conteudo que nao seja []byte, nada e' despachado. A
// diferenca de nivel de log entre os dois casos e' do upstream: a ausencia do
// <set> e' esperada (debug), o conteudo de tipo errado nao e' (warn).
func HandleStatus(t Transport, node *waBinary.Node) {
	ag := node.AttrGetter()
	child, found := node.GetOptionalChildByTag("set")
	if !found {
		t.Log().Debugf("Status notification did not contain child with tag 'set'")
		return
	}
	status, ok := child.Content.([]byte)
	if !ok {
		t.Log().Warnf("Set status notification has unexpected content (%T)", child.Content)
		return
	}
	t.DispatchEvent(&events.UserAbout{
		JID:       ag.JID("from"),
		Timestamp: ag.UnixTime("t"),
		Status:    string(status),
	})
}
