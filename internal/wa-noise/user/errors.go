// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import "errors"

// Os sentinelas deste dominio. A raiz os reexporta pelos nomes historicos
// (ErrProfilePictureUnauthorized, ErrContactQRLinkNotFound, ...) por
// **atribuicao**, o que faz deles o MESMO valor, nao copias: um errors.New
// proprio la' quebraria errors.Is para quem compara com o nome da raiz. E' a
// mesma armadilha de aliasing documentada nos erros de midia, app state,
// pareamento e grupo (lotes 1, 3, 4 e 6).
var (
	// ErrProfilePictureUnauthorized is returned by GetProfilePictureInfo when trying to get the profile picture of a user
	// whose privacy settings prevent you from seeing their profile picture (status code 401).
	ErrProfilePictureUnauthorized = errors.New("the user has hidden their profile picture from you")
	// ErrProfilePictureNotSet is returned by GetProfilePictureInfo when the given user or group doesn't have a profile
	// picture (status code 404).
	ErrProfilePictureNotSet = errors.New("that user or group does not have a profile picture")
	// ErrBusinessMessageLinkNotFound is returned by ResolveBusinessMessageLink if the link doesn't exist or has been revoked.
	ErrBusinessMessageLinkNotFound = errors.New("that business message link does not exist or has been revoked")
	// ErrContactQRLinkNotFound is returned by ResolveContactQRLink if the link doesn't exist or has been revoked.
	ErrContactQRLinkNotFound = errors.New("that contact QR link does not exist or has been revoked")
)
