// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/types"
)

// Fachada; ver o cabecalho de group.go.

// ReqCreateGroup contains the request data for CreateGroup.
//
// A definicao vive em internal/wa-noise/group; aqui fica um apelido, que e' o
// mesmo tipo — chamadores externos (pkg/infra/wa-noise/group, entre outros)
// continuam compilando sem conversao.
type ReqCreateGroup = group.ReqCreate

// CreateGroup creates a group on WhatsApp with the given name and participants.
//
// See ReqCreateGroup for parameters.
func (cli *Client) CreateGroup(ctx context.Context, req ReqCreateGroup) (*types.GroupInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return group.Create(ctx, cli.groupT(), req)
}

// UnlinkGroup removes a child group from a parent community.
func (cli *Client) UnlinkGroup(ctx context.Context, parent, child types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.Unlink(ctx, cli.groupT(), parent, child)
}

// LinkGroup adds an existing group as a child group in a community.
//
// To create a new group within a community, set LinkedParentJID in the CreateGroup request.
func (cli *Client) LinkGroup(ctx context.Context, parent, child types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.Link(ctx, cli.groupT(), parent, child)
}

// LeaveGroup leaves the specified group on WhatsApp.
func (cli *Client) LeaveGroup(ctx context.Context, jid types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.Leave(ctx, cli.groupT(), jid)
}
