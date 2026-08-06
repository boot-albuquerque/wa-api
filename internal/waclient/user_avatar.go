// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/base64"
	"errors"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/types"
)

type GetProfilePictureParams struct {
	Preview     bool
	ExistingID  string
	IsCommunity bool
	// This is a common group ID that you share with the target
	CommonGID types.JID
	// use this to query the profile photo of a group you don't have joined, but you have an invite code for
	InviteCode string
	// Persona ID when getting profile of Meta AI bots
	PersonaID string
}

// GetProfilePictureInfo gets the URL where you can download a WhatsApp user's profile picture or group's photo.
//
// Optionally, you can pass the last known profile picture ID.
// If the profile picture hasn't changed, this will return nil with no error.
//
// To get a community photo, you should pass `IsCommunity: true`, as otherwise you may get a 401 error.
func (cli *Client) GetProfilePictureInfo(ctx context.Context, jid types.JID, params *GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	attrs := waBinary.Attrs{
		"query": "url",
	}
	var target, to types.JID
	if params == nil {
		params = &GetProfilePictureParams{}
	}
	if params.Preview {
		attrs["type"] = "preview"
	} else {
		attrs["type"] = "image"
	}
	if params.ExistingID != "" {
		attrs["id"] = params.ExistingID
	}
	if params.InviteCode != "" {
		attrs["invite"] = params.InviteCode
	}

	var expectWrapped bool
	var content []waBinary.Node
	namespace := "w:profile:picture"
	if params.IsCommunity {
		target = types.EmptyJID
		namespace = "w:g2"
		to = jid
		attrs["parent_group_jid"] = jid
		expectWrapped = true
		content = []waBinary.Node{{
			Tag: "pictures",
			Content: []waBinary.Node{{
				Tag:   "picture",
				Attrs: attrs,
			}},
		}}
	} else {
		to = types.ServerJID
		target = jid

		if !params.CommonGID.IsEmpty() {
			attrs["common_gid"] = params.CommonGID
		}

		if params.PersonaID != "" {
			attrs["persona_id"] = params.PersonaID
		}

		var pictureContent []waBinary.Node
		if token, _ := cli.Store.PrivacyTokens.GetPrivacyToken(ctx, jid); token != nil {
			pictureContent = []waBinary.Node{{
				Tag:     "tctoken",
				Content: token.Token,
			}}
		}

		content = []waBinary.Node{{
			Tag:     "picture",
			Attrs:   attrs,
			Content: pictureContent,
		}}
	}
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: namespace,
		Type:      "get",
		To:        to,
		Target:    target,
		Content:   content,
	})
	if errors.Is(err, ErrIQNotAuthorized) {
		return nil, wrapIQError(ErrProfilePictureUnauthorized, err)
	} else if errors.Is(err, ErrIQNotFound) {
		return nil, wrapIQError(ErrProfilePictureNotSet, err)
	} else if err != nil {
		return nil, err
	}
	if expectWrapped {
		pics, ok := resp.GetOptionalChildByTag("pictures")
		if !ok {
			return nil, &ElementMissingError{Tag: "pictures", In: "response to profile picture query"}
		}
		resp = &pics
	}
	picture, ok := resp.GetOptionalChildByTag("picture")
	if !ok {
		if params.ExistingID != "" {
			return nil, nil
		}
		return nil, &ElementMissingError{Tag: "picture", In: "response to profile picture query"}
	}
	var info types.ProfilePictureInfo
	ag := picture.AttrGetter()
	if ag.OptionalInt("status") == 304 {
		return nil, nil
	} else if ag.OptionalInt("status") == 204 {
		return nil, ErrProfilePictureNotSet
	}
	info.ID = ag.String("id")
	info.URL = ag.String("url")
	info.Type = ag.String("type")
	info.DirectPath = ag.String("direct_path")
	info.Hash, _ = base64.StdEncoding.DecodeString(ag.OptionalString("hash"))
	if !ag.OK() {
		return &info, ag.Error()
	}
	return &info, nil
}
