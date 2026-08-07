// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"

	"wa-api/internal/wa-noise/types"
)

// --- isValidLIDMapping ---

// O guarda que o lote 7 acrescentou em `GetUserInfo`: so' vai para
// `PutManyLIDMappings` o par que o store aceita (PN em s.whatsapp.net, LID em
// lid). Espelha `CachedLIDMap.PutManyLIDMappings`
// (`store/sqlstore/lidmap.go:220`), que descarta e loga qualquer outro formato.
func TestIsValidLIDMapping(t *testing.T) {
	pn := types.NewJID("5511999", types.DefaultUserServer)
	lid := types.NewJID("8877", types.HiddenUserServer)

	cases := []struct {
		name string
		pn   types.JID
		lid  types.JID
		want bool
	}{
		{"par valido", pn, lid, true},
		{"LID no lugar do PN", lid, lid, false},
		{"PN no lugar do LID", pn, pn, false},
		{"LID vazio", pn, types.EmptyJID, false},
		{"PN vazio", types.EmptyJID, lid, false},
		{"PN legado c.us", types.NewJID("5511999", types.LegacyUserServer), lid, false},
		{"PN msgr", types.NewJID("123", types.MessengerServer), lid, false},
		{"user do PN vazio", types.NewJID("", types.DefaultUserServer), lid, false},
		{"user do LID vazio", pn, types.NewJID("", types.HiddenUserServer), false},
		{"PN com device (AD JID)", types.JID{User: pn.User, Server: pn.Server, Device: 3}, lid, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidLIDMapping(tc.pn, tc.lid); got != tc.want {
				t.Errorf("isValidLIDMapping(%s, %s) = %v, want %v", tc.pn, tc.lid, got, tc.want)
			}
		})
	}
}
