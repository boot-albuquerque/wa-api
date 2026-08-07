// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package notification

import (
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/util/log"
)

// fakeTransport e' o duble de Transport. Guarda os eventos despachados na
// ordem em que sairam — a ordem e' asserida por varios testes (um evento por
// filho de <picture>, no maximo um por <update> de mex).
type fakeTransport struct {
	events []any
}

func (f *fakeTransport) Log() waLog.Logger { return waLog.Noop }

func (f *fakeTransport) DispatchEvent(evt any) { f.events = append(f.events, evt) }

var (
	testPeerJID = types.NewJID("5511999999999", types.DefaultUserServer)
	testOwnJID  = types.NewJID("5511888888888", types.DefaultUserServer)
)
