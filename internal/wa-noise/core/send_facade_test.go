// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"

	"wa-api/internal/wa-noise/capabilities/send"
	"wa-api/internal/wa-noise/protocol/types"
)

// Este arquivo guarda o que sobrou dos testes de envio na raiz depois da
// extracao do lote 8: os helpers de cache que outros testes da raiz usam, e as
// travas da fronteira (apelidos de tipo e reexportacao de constantes/erros).
// Os testes de COMPORTAMENTO do envio moram em internal/wa-noise/send.

// msgTypeTextValue e' o valor historico do atributo `type` de uma mensagem de
// texto. message_parse_test.go o compara ao classificar o que ENTRA; a
// constante em si mora em send/constants.go.
const msgTypeText = "text"

// putDeviceCache/getDeviceCache encapsulam o par Lock/SetLocked (e
// Lock/GetLocked) de user.DeviceCache. O cache de dispositivos deixou de ser um
// mapa nu em *Client no lote 7, pelo mesmo motivo que o de grupo no lote 6.
func putDeviceCache(cli *Client, jid types.JID, entry deviceCache) {
	cli.userDevicesCache.Lock()
	defer cli.userDevicesCache.Unlock()
	cli.userDevicesCache.SetLocked(jid, entry)
}

func getDeviceCache(cli *Client, jid types.JID) (deviceCache, bool) {
	cli.userDevicesCache.Lock()
	defer cli.userDevicesCache.Unlock()
	return cli.userDevicesCache.GetLocked(jid)
}

// A fronteira raiz <-> send e' feita de apelidos de tipo, nao de tipos novos.
// Se alguem trocar `=` por definicao, os quatro tipos viram tipos distintos e a
// API publica quebra em silencio para quem monta um whatsmeow.SendResponse.
func TestSendTypesAreAliases(t *testing.T) {
	var resp SendResponse = send.Response{}
	var req SendRequestExtra = send.RequestExtra{}
	var timings MessageDebugTimings = send.DebugTimings{}
	var extra nodeExtraParams = send.NodeExtraParams{}
	_, _, _, _ = resp, req, timings, extra

	var back send.Response = resp
	_ = back
}

// Os seis sentinelas de envio sao o MESMO ponteiro que os de send/errors.go —
// nao copias. Um errors.New proprio na raiz quebraria errors.Is para quem
// compara com o nome historico.
func TestSendErrorsAreTheSameValues(t *testing.T) {
	cases := map[string]struct{ root, sub error }{
		"ErrNoSession":           {ErrNoSession, send.ErrNoSession},
		"ErrMessageTimedOut":     {ErrMessageTimedOut, send.ErrMessageTimedOut},
		"ErrUnknownServer":       {ErrUnknownServer, send.ErrUnknownServer},
		"ErrRecipientADJID":      {ErrRecipientADJID, send.ErrRecipientADJID},
		"ErrServerReturnedError": {ErrServerReturnedError, send.ErrServerReturnedError},
		"ErrInvalidInlineBotID":  {ErrInvalidInlineBotID, send.ErrInvalidInlineBotID},
	}
	for name, tc := range cases {
		if tc.root != tc.sub {
			t.Errorf("%s: raiz e send/ divergiram (%v != %v)", name, tc.root, tc.sub)
		}
	}
}

// As constantes de wire reexportadas tem que continuar batendo com o que
// send/constants.go define. Redeclara-las na raiz e' o erro que este teste
// existe para pegar.
func TestSendConstantsAreReexported(t *testing.T) {
	cases := map[string]struct{ root, sub any }{
		"encNodeTag":         {encNodeTag, send.EncNodeTag},
		"encAttrVersion":     {encAttrVersion, send.EncAttrVersion},
		"encAttrType":        {encAttrType, send.EncAttrType},
		"encAttrDecryptFail": {encAttrDecryptFail, send.EncAttrDecryptFail},
		"encTypeMsg":         {encTypeMsg, send.EncTypeMsg},
		"encTypePreKeyMsg":   {encTypePreKeyMsg, send.EncTypePreKeyMsg},
		"encTypeSenderKey":   {encTypeSenderKey, send.EncTypeSenderKey},
		"msgCategoryPeer":    {msgCategoryPeer, send.MsgCategoryPeer},
		"messageSecretSize":  {messageSecretSize, send.MessageSecretSize},
		"FBMessageVersion":   {FBMessageVersion, send.FBMessageVersion},
	}
	for name, tc := range cases {
		if tc.root != tc.sub {
			t.Errorf("%s: raiz = %v, send/ = %v", name, tc.root, tc.sub)
		}
	}
}

// A fachada participantListHashV2 continua sendo o unico caminho que
// notification_device.go e user_transport.go usam; ela nao pode divergir da
// implementacao do subpacote.
func TestParticipantListHashFacadeMatchesSubpackage(t *testing.T) {
	devices := []types.JID{
		{User: "1", Server: types.DefaultUserServer},
		{User: "2", Server: types.DefaultUserServer, Device: 3},
	}
	if got, want := participantListHashV2(devices), send.ParticipantListHashV2(devices); got != want {
		t.Errorf("fachada = %q, send/ = %q", got, want)
	}
}
