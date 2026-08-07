// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"errors"
	"fmt"

	"wa-api/internal/wa-noise/capabilities/appstatesync"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/group"
	"wa-api/internal/wa-noise/capabilities/media"
	"wa-api/internal/wa-noise/capabilities/message"
	"wa-api/internal/wa-noise/capabilities/pairing"
	"wa-api/internal/wa-noise/capabilities/send"
	"wa-api/internal/wa-noise/capabilities/user"
)

// Miscellaneous errors
var (
	ErrClientIsNil  = errors.New("client is nil")
	ErrIQTimedOut   = errors.New("info query timed out")
	ErrNotConnected = errors.New("websocket not connected")
	ErrNotLoggedIn  = errors.New("the store doesn't contain a device JID")

	// Os dois sao o MESMO valor de send.*, nao copias — a mesma armadilha de
	// aliasing documentada em ErrAppStateUpdate e nos erros de grupo.
	ErrNoSession       = send.ErrNoSession
	ErrMessageTimedOut = send.ErrMessageTimedOut

	ErrAlreadyConnected = errors.New("websocket is already connected")

	// Os dois sao o MESMO valor de pairing.*, nao copias — a mesma armadilha
	// de aliasing documentada em ErrAppStateUpdate.
	ErrPhoneNumberTooShort           = pairing.ErrPhoneNumberTooShort
	ErrPhoneNumberIsNotInternational = pairing.ErrPhoneNumberIsNotInternational

	ErrQRAlreadyConnected = errors.New("GetQRChannel must be called before connecting")
	ErrQRStoreContainsID  = errors.New("GetQRChannel can only be called when there's no user ID in the client's Store")

	ErrNoPushName = errors.New("can't send presence without PushName set")

	ErrNoPrivacyToken = errors.New("no privacy token stored")

	// ErrAppStateUpdate e' o MESMO valor que appstatesync.ErrUpdate, nao uma
	// copia — trocar por um errors.New proprio quebraria errors.Is para quem
	// compara com este nome (mesma armadilha de aliasing dos erros de midia,
	// documentada logo abaixo).
	ErrAppStateUpdate = appstatesync.ErrUpdate
)

// Errors that happen while confirming device pairing
var (
	// Os tres sao o MESMO valor de pairing.*, pelo mesmo racional de aliasing
	// dos erros de app state e de midia.
	ErrPairInvalidDeviceIdentityHMAC = pairing.ErrInvalidDeviceIdentityHMAC
	ErrPairInvalidDeviceSignature    = pairing.ErrInvalidDeviceSignature
	ErrPairRejectedLocally           = pairing.ErrRejectedLocally
)

// PairProtoError is included in an events.PairError if the pairing failed due to a protobuf error.
//
// Apelido de tipo: a definicao vive em internal/wa-noise/pairing/. A ordem dos
// campos (Message, ProtoErr) e' preservada porque ha' literais compostos
// posicionais no codigo movido.
type PairProtoError = pairing.ProtoError

// PairDatabaseError is included in an events.PairError if the pairing failed due to being unable to save the credentials to the device store.
//
// Apelido de tipo, mesmo racional de PairProtoError.
type PairDatabaseError = pairing.DatabaseError

var (
	// Os dois sao o MESMO valor de user.*, nao copias — a mesma armadilha de
	// aliasing documentada em ErrAppStateUpdate e nos erros de grupo.
	//
	// ErrProfilePictureUnauthorized is returned by GetProfilePictureInfo when trying to get the profile picture of a user
	// whose privacy settings prevent you from seeing their profile picture (status code 401).
	ErrProfilePictureUnauthorized = user.ErrProfilePictureUnauthorized
	// ErrProfilePictureNotSet is returned by GetProfilePictureInfo when the given user or group doesn't have a profile
	// picture (status code 404).
	ErrProfilePictureNotSet = user.ErrProfilePictureNotSet
	// Os cinco sao o MESMO valor de group.*, nao copias — a mesma armadilha de
	// aliasing documentada em ErrAppStateUpdate.
	//
	// ErrGroupInviteLinkUnauthorized is returned by GetGroupInviteLink if you don't have the permission to get the link (status code 401).
	ErrGroupInviteLinkUnauthorized = group.ErrInviteLinkUnauthorized
	// ErrNotInGroup is returned by group info getting methods if you're not in the group (status code 403).
	ErrNotInGroup = group.ErrNotInGroup
	// ErrGroupNotFound is returned by group info getting methods if the group doesn't exist (status code 404).
	ErrGroupNotFound = group.ErrNotFound
	// ErrInviteLinkInvalid is returned by methods that use group invite links if the invite link is malformed.
	ErrInviteLinkInvalid = group.ErrInviteLinkInvalid
	// ErrInviteLinkRevoked is returned by methods that use group invite links if the invite link was valid, but has been revoked and can no longer be used.
	ErrInviteLinkRevoked = group.ErrInviteLinkRevoked
	// Os dois sao o MESMO valor de user.*; ver acima.
	//
	// ErrBusinessMessageLinkNotFound is returned by ResolveBusinessMessageLink if the link doesn't exist or has been revoked.
	ErrBusinessMessageLinkNotFound = user.ErrBusinessMessageLinkNotFound
	// ErrContactQRLinkNotFound is returned by ResolveContactQRLink if the link doesn't exist or has been revoked.
	ErrContactQRLinkNotFound = user.ErrContactQRLinkNotFound
	// ErrInvalidImageFormat is returned by SetGroupPhoto if the given photo is not in the correct format.
	//
	// MESMO valor que group.ErrInvalidImageFormat; ver acima.
	ErrInvalidImageFormat = group.ErrInvalidImageFormat
	// ErrInvalidDisappearingTimer is returned by SetDisappearingTimer if the given timer is not one of the allowed values.
	ErrInvalidDisappearingTimer = errors.New("invalid disappearing timer provided")
)

// Some errors that Client.SendMessage can return
var (
	ErrBroadcastListUnsupported = errors.New("sending to non-status broadcast lists is not yet supported")
	// Os quatro sao o MESMO valor de send.*, nao copias. Ver ErrNoSession.
	ErrUnknownServer       = send.ErrUnknownServer
	ErrRecipientADJID      = send.ErrRecipientADJID
	ErrServerReturnedError = send.ErrServerReturnedError
	ErrInvalidInlineBotID  = send.ErrInvalidInlineBotID
)

// DownloadHTTPError is returned when the media server answers with an
// unexpected status code.
//
// A definicao vive em internal/wa-noise/media (ADR-0004, Fase F/G lote 1); aqui
// fica um apelido para preservar a API historica do pacote raiz.
type DownloadHTTPError = media.DownloadHTTPError

// Some errors that Client.Download can return. Sao os mesmos valores do pacote
// internal/wa-noise/media — nao copias — entao errors.Is atravessa a fronteira.
//
// NAO troque nenhuma destas linhas por um errors.New proprio. A identidade e'
// load-bearing: DownloadMediaWithPath decide encerrar o laco de hosts com
// errors.Is contra ErrMediaDownloadFailedWith403/404/410. Se a raiz declarasse
// valores proprios, a comparacao feita por quem usa os nomes da raiz falharia e
// um 404 passaria a ser tratado como falha retentavel — o download tentaria
// todos os hosts da mediaConn para um arquivo que nao existe.
var (
	ErrMediaDownloadFailedWith403 = media.ErrMediaDownloadFailedWith403
	ErrMediaDownloadFailedWith404 = media.ErrMediaDownloadFailedWith404
	ErrMediaDownloadFailedWith410 = media.ErrMediaDownloadFailedWith410
	ErrNoURLPresent               = media.ErrNoURLPresent
	ErrFileLengthMismatch         = media.ErrFileLengthMismatch
	ErrTooShortFile               = media.ErrTooShortFile
	ErrInvalidMediaHMAC           = media.ErrInvalidMediaHMAC
	ErrInvalidMediaEncSHA256      = media.ErrInvalidMediaEncSHA256
	ErrInvalidMediaSHA256         = media.ErrInvalidMediaSHA256
	ErrUnknownMediaType           = media.ErrUnknownMediaType
	ErrNothingDownloadableFound   = media.ErrNothingDownloadableFound

	// ErrMediaNotAvailableOnPhone is returned by DecryptMediaRetryNotification if the given event contains error code 2.
	ErrMediaNotAvailableOnPhone = media.ErrMediaNotAvailableOnPhone
	// ErrUnknownMediaRetryError is returned by DecryptMediaRetryNotification if the given event contains an unknown error code.
	ErrUnknownMediaRetryError = media.ErrUnknownMediaRetryError
)

// Os cinco sentinelas de segredo de mensagem vivem em message/errors.go desde a
// Fase F/G lote 9, e sao reexportados aqui por ATRIBUICAO — o MESMO valor, nao
// copias. Um `errors.New` proprio deste lado quebraria `errors.Is` para quem
// comparasse com o nome historico. Travado por TestMessageErrorsAreTheSameValues.
var (
	ErrOriginalMessageSecretNotFound = message.ErrOriginalMessageSecretNotFound
	ErrNotEncryptedReactionMessage   = message.ErrNotEncryptedReactionMessage
	ErrNotEncryptedCommentMessage    = message.ErrNotEncryptedCommentMessage
	ErrNotSecretEncryptedMessage     = message.ErrNotSecretEncryptedMessage
	ErrNotPollUpdateMessage          = message.ErrNotPollUpdateMessage
)

type wrappedIQError struct {
	HumanError error
	IQError    error
}

func (err *wrappedIQError) Error() string {
	return err.HumanError.Error()
}

func (err *wrappedIQError) Is(other error) bool {
	return errors.Is(other, err.HumanError)
}

func (err *wrappedIQError) Unwrap() error {
	return err.IQError
}

func wrapIQError(human, iq error) error {
	return &wrappedIQError{human, iq}
}

// IQError is a generic error container for info queries
type IQError struct {
	Code      int
	Text      string
	ErrorNode *waBinary.Node
	RawNode   *waBinary.Node
}

// Common errors returned by info queries for use with errors.Is
var (
	ErrIQBadRequest          error = &IQError{Code: 400, Text: "bad-request"}
	ErrIQNotAuthorized       error = &IQError{Code: 401, Text: "not-authorized"}
	ErrIQForbidden           error = &IQError{Code: 403, Text: "forbidden"}
	ErrIQNotFound            error = &IQError{Code: 404, Text: "item-not-found"}
	ErrIQNotAllowed          error = &IQError{Code: 405, Text: "not-allowed"}
	ErrIQNotAcceptable       error = &IQError{Code: 406, Text: "not-acceptable"}
	ErrIQGone                error = &IQError{Code: 410, Text: "gone"}
	ErrIQResourceLimit       error = &IQError{Code: 419, Text: "resource-limit"}
	ErrIQLocked              error = &IQError{Code: 423, Text: "locked"}
	ErrIQRateOverLimit       error = &IQError{Code: 429, Text: "rate-overlimit"}
	ErrIQInternalServerError error = &IQError{Code: 500, Text: "internal-server-error"}
	ErrIQServiceUnavailable  error = &IQError{Code: 503, Text: "service-unavailable"}
	ErrIQPartialServerError  error = &IQError{Code: 530, Text: "partial-server-error"}
)

func parseIQError(node *waBinary.Node) error {
	var err IQError
	err.RawNode = node
	val, ok := node.GetOptionalChildByTag("error")
	if ok {
		err.ErrorNode = &val
		ag := val.AttrGetter()
		err.Code = ag.OptionalInt("code")
		err.Text = ag.OptionalString("text")
	}
	return &err
}

func (iqe *IQError) Error() string {
	if iqe.Code == 0 {
		if iqe.ErrorNode != nil {
			return fmt.Sprintf("info query returned unknown error: %s", iqe.ErrorNode.XMLString())
		} else if iqe.RawNode != nil {
			return fmt.Sprintf("info query returned unexpected response: %s", iqe.RawNode.XMLString())
		} else {
			return "unknown info query error"
		}
	}
	return fmt.Sprintf("info query returned status %d: %s", iqe.Code, iqe.Text)
}

func (iqe *IQError) Is(other error) bool {
	otherIQE, ok := other.(*IQError)
	if !ok {
		return false
	} else if iqe.Code != 0 && otherIQE.Code != 0 {
		return otherIQE.Code == iqe.Code && otherIQE.Text == iqe.Text
	} else if iqe.ErrorNode != nil && otherIQE.ErrorNode != nil {
		return iqe.ErrorNode.XMLString() == otherIQE.ErrorNode.XMLString()
	} else {
		return false
	}
}

// ElementMissingError is returned by various functions that parse XML elements when a required element is missing.
type ElementMissingError struct {
	Tag string
	In  string
}

func (eme *ElementMissingError) Error() string {
	return fmt.Sprintf("missing <%s> element in %s", eme.Tag, eme.In)
}

var ErrIQDisconnected = &DisconnectedError{Action: "info query"}

// DisconnectedError is returned if the websocket disconnects before an info query or other request gets a response.
type DisconnectedError struct {
	Action string
	Node   *waBinary.Node
}

func (err *DisconnectedError) Error() string {
	return fmt.Sprintf("websocket disconnected before %s returned response", err.Action)
}

func (err *DisconnectedError) Is(other error) bool {
	otherDisc, ok := other.(*DisconnectedError)
	if !ok {
		return false
	}
	return otherDisc.Action == err.Action
}
