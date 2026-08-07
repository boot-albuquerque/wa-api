// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/internal/wa-noise/user"
)

// Continuacao da fachada do dominio de usuario (ver user.go): as consultas de
// foto de perfil, lista de bloqueio, bots, perfil business e links de QR.

// GetProfilePictureInfo gets the URL where you can download a WhatsApp user's profile picture or group's photo.
//
// Optionally, you can pass the last known profile picture ID.
// If the profile picture hasn't changed, this will return nil with no error.
//
// To get a community photo, you should pass `IsCommunity: true`, as otherwise you may get a 401 error.
func (cli *Client) GetProfilePictureInfo(
	ctx context.Context, jid types.JID, params *GetProfilePictureParams,
) (*types.ProfilePictureInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.GetProfilePictureInfo(ctx, cli.userT(), jid, params)
}

func (cli *Client) parseBlocklist(node *waBinary.Node) *types.Blocklist {
	return user.ParseBlocklist(cli.userT(), node)
}

// GetBlocklist gets the list of users that this user has blocked.
func (cli *Client) GetBlocklist(ctx context.Context) (*types.Blocklist, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.GetBlocklist(ctx, cli.userT())
}

// UpdateBlocklist updates the user's block list and returns the updated list.
func (cli *Client) UpdateBlocklist(
	ctx context.Context, jid types.JID, action events.BlocklistChangeAction,
) (*types.Blocklist, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.UpdateBlocklist(ctx, cli.userT(), jid, action)
}

// GetBotListV2 lists the bots available to this account.
func (cli *Client) GetBotListV2(ctx context.Context) ([]types.BotListInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.GetBotListV2(ctx, cli.userT())
}

// GetBotProfiles fetches the profiles of the given bots.
func (cli *Client) GetBotProfiles(ctx context.Context, botInfo []types.BotListInfo) ([]types.BotProfileInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.GetBotProfiles(ctx, cli.userT(), botInfo)
}

func (cli *Client) parseBusinessProfile(node *waBinary.Node) (*types.BusinessProfile, error) {
	return user.ParseBusinessProfile(node)
}

// GetBusinessProfile gets the profile info of a WhatsApp business account
func (cli *Client) GetBusinessProfile(ctx context.Context, jid types.JID) (*types.BusinessProfile, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.GetBusinessProfile(ctx, cli.userT(), jid)
}

// ResolveBusinessMessageLink resolves a business message short link and returns the target JID, business name and
// text to prefill in the input field (if any).
//
// The links look like https://wa.me/message/<code> or https://api.whatsapp.com/message/<code>. You can either provide
// the full link, or just the <code> part.
func (cli *Client) ResolveBusinessMessageLink(
	ctx context.Context, code string,
) (*types.BusinessMessageLinkTarget, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.ResolveBusinessMessageLink(ctx, cli.userT(), code)
}

// ResolveContactQRLink resolves a link from a contact share QR code and returns the target JID and push name.
//
// The links look like https://wa.me/qr/<code> or https://api.whatsapp.com/qr/<code>. You can either provide
// the full link, or just the <code> part.
func (cli *Client) ResolveContactQRLink(ctx context.Context, code string) (*types.ContactQRLinkTarget, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return user.ResolveContactQRLink(ctx, cli.userT(), code)
}

// GetContactQRLink gets your own contact share QR link that can be resolved using ResolveContactQRLink
// (or scanned with the official apps when encoded as a QR code).
//
// If the revoke parameter is set to true, it will ask the server to revoke the previous link and generate a new one.
func (cli *Client) GetContactQRLink(ctx context.Context, revoke bool) (string, error) {
	if cli == nil {
		return "", ErrClientIsNil
	}
	return user.GetContactQRLink(ctx, cli.userT(), revoke)
}
