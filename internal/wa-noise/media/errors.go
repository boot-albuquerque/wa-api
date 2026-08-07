// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package media

import (
	"errors"
	"fmt"
	"net/http"
)

// DownloadHTTPError embrulha uma resposta HTTP de status inesperado vinda do
// servidor de midia. A comparacao por errors.Is olha somente o status code,
// o que permite comparar com os sentinelas ErrMediaDownloadFailedWithNNN.
type DownloadHTTPError struct {
	*http.Response
}

func (dhe DownloadHTTPError) Error() string {
	return fmt.Sprintf("download failed with status code %d", dhe.StatusCode)
}

func (dhe DownloadHTTPError) Is(other error) bool {
	var otherDHE DownloadHTTPError
	return errors.As(other, &otherDHE) && dhe.StatusCode == otherDHE.StatusCode
}

// Erros que o caminho de download pode devolver.
var (
	ErrMediaDownloadFailedWith403 = DownloadHTTPError{Response: &http.Response{StatusCode: 403}}
	ErrMediaDownloadFailedWith404 = DownloadHTTPError{Response: &http.Response{StatusCode: 404}}
	ErrMediaDownloadFailedWith410 = DownloadHTTPError{Response: &http.Response{StatusCode: 410}}
	ErrNoURLPresent               = errors.New("no url present")
	ErrFileLengthMismatch         = errors.New("file length does not match")
	ErrTooShortFile               = errors.New("file too short")
	ErrInvalidMediaHMAC           = errors.New("invalid media hmac")
	ErrInvalidMediaEncSHA256      = errors.New("hash of media ciphertext doesn't match")
	ErrInvalidMediaSHA256         = errors.New("hash of media plaintext doesn't match")
	ErrUnknownMediaType           = errors.New("unknown media type")
	ErrNothingDownloadableFound   = errors.New("didn't find any attachments in message")
)

// Erros do retry de midia.
var (
	// ErrMediaNotAvailableOnPhone e' devolvido por DecryptRetryNotification
	// quando o evento traz o codigo de erro 2.
	ErrMediaNotAvailableOnPhone = errors.New("media no longer available on phone")
	// ErrUnknownMediaRetryError e' devolvido por DecryptRetryNotification
	// quando o evento traz um codigo de erro desconhecido.
	ErrUnknownMediaRetryError = errors.New("unknown media retry error")
)
