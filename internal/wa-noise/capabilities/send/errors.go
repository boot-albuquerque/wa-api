// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import "errors"

// Os sentinelas deste dominio. A raiz os reexporta pelos nomes historicos
// (ErrNoSession, ErrUnknownServer, ...) por **atribuicao**, o que faz deles o
// MESMO valor, nao copias: um errors.New proprio la' quebraria errors.Is para
// quem compara com o nome da raiz. E' a mesma armadilha de aliasing
// documentada nos erros de midia, app state, pareamento e grupo (lotes 1, 3, 4
// e 6).
//
// Os seis abaixo sao os que a auditoria confirmou serem exclusivos do caminho
// de envio: fora de errors.go e dos testes, nenhum arquivo da raiz alem dos 11
// deste lote os cita. ErrClientIsNil e ErrNotLoggedIn ficaram na raiz por serem
// do fork inteiro — o primeiro porque a checagem de receptor nil so' pode
// existir na raiz (um *Client nil nao produz Transport), o segundo pela
// interface Errors (ver transport.go).
var (
	// ErrNoSession is returned by SendMessage if there is no Signal session with the recipient.
	ErrNoSession = errors.New("can't encrypt message for device: no signal session established")
	// ErrMessageTimedOut is returned by SendMessage if the server doesn't acknowledge the message within the timeout.
	ErrMessageTimedOut = errors.New("timed out waiting for message send response")
	// ErrUnknownServer is returned by SendMessage if the recipient JID has an unknown server.
	ErrUnknownServer = errors.New("can't send message to unknown server")
	// ErrRecipientADJID is returned by SendMessage if the recipient JID has a device part.
	ErrRecipientADJID = errors.New("message recipient must be a user JID with no device part")
	// ErrServerReturnedError is returned by SendMessage if the server's ack carries an error code.
	ErrServerReturnedError = errors.New("server returned error")
	// ErrInvalidInlineBotID is returned by SendMessage if the inline bot JID is not a bot JID.
	ErrInvalidInlineBotID = errors.New("invalid inline bot ID")
)
