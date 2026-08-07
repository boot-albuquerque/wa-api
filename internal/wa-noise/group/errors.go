// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package group

import "errors"

// Os sentinelas deste dominio. A raiz os reexporta pelos nomes historicos
// (ErrGroupNotFound, ErrNotInGroup, ...) por **atribuicao**, o que faz deles o
// MESMO valor, nao copias: um errors.New proprio la' quebraria errors.Is para
// quem compara com o nome da raiz. E' a mesma armadilha de aliasing
// documentada nos erros de midia, app state e pareamento (lotes 1, 3 e 4).
var (
	// ErrInviteLinkUnauthorized is returned by GetGroupInviteLink if you don't have the permission to get the link (status code 401).
	ErrInviteLinkUnauthorized = errors.New("you don't have the permission to get the group's invite link")
	// ErrNotInGroup is returned by group info getting methods if you're not in the group (status code 403).
	ErrNotInGroup = errors.New("you're not participating in that group")
	// ErrNotFound is returned by group info getting methods if the group doesn't exist (status code 404).
	ErrNotFound = errors.New("that group does not exist")
	// ErrInviteLinkInvalid is returned by methods that use group invite links if the invite link is malformed.
	ErrInviteLinkInvalid = errors.New("that group invite link is not valid")
	// ErrInviteLinkRevoked is returned by methods that use group invite links if the invite link was valid, but has been revoked and can no longer be used.
	ErrInviteLinkRevoked = errors.New("that group invite link has been revoked")
	// ErrInvalidImageFormat is returned by SetPhoto if the given photo is not in the correct format.
	ErrInvalidImageFormat = errors.New("the given data is not a valid image")
)
