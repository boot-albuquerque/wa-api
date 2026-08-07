// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package pairing

import (
	"errors"
	"fmt"
)

// Sentinelas do fluxo de pareamento. A raiz referencia ESTES valores, e nao
// copias: um errors.New proprio la' quebraria errors.Is para quem compara com
// o nome da raiz contra um erro produzido aqui dentro. Mesma armadilha de
// aliasing documentada no lote 1 (midia) e no lote 3 (app state).
var (
	ErrInvalidDeviceIdentityHMAC = errors.New("invalid device identity HMAC in pair success message")
	ErrInvalidDeviceSignature    = errors.New("invalid device signature in pair success message")
	ErrRejectedLocally           = errors.New("local PrePairCallback rejected pairing")

	ErrPhoneNumberTooShort           = errors.New("phone number too short")
	ErrPhoneNumberIsNotInternational = errors.New("international phone number required (must not start with 0)")

	// ErrNoPendingPairing e' devolvido quando chega uma notificacao de
	// pareamento por codigo sem que PairPhone tenha sido chamado antes.
	ErrNoPendingPairing = errors.New("received code pair notification without a pending pairing")
	// ErrPairingRefMismatch indica que a notificacao se refere a outra sessao
	// de pareamento que nao a pendente.
	ErrPairingRefMismatch = errors.New("pairing ref mismatch in code pair notification")
)

// ProtoError is included in an events.PairError if the pairing failed due to a protobuf error.
//
// A raiz continua expondo este tipo com o nome historico PairProtoError, por
// apelido de tipo. A ordem dos campos e' load-bearing: ha' literais compostos
// posicionais (&PairProtoError{"msg", err}) no codigo movido.
type ProtoError struct {
	Message  string
	ProtoErr error
}

func (err *ProtoError) Error() string {
	return fmt.Sprintf("%s: %v", err.Message, err.ProtoErr)
}

func (err *ProtoError) Unwrap() error {
	return err.ProtoErr
}

// DatabaseError is included in an events.PairError if the pairing failed due to being unable to save the credentials to the device store.
//
// Exposto na raiz como PairDatabaseError, pelo mesmo mecanismo de ProtoError.
type DatabaseError struct {
	Message string
	DBErr   error
}

func (err *DatabaseError) Error() string {
	return fmt.Sprintf("%s: %v", err.Message, err.DBErr)
}

func (err *DatabaseError) Unwrap() error {
	return err.DBErr
}
