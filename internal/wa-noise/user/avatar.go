// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"context"
	"encoding/base64"
	"errors"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/types"
)

// GetProfilePictureParams sao os parametros de GetProfilePictureInfo. A raiz
// mantem `type GetProfilePictureParams = user.GetProfilePictureParams`
// (apelido, nao tipo novo) para preservar a API publica historica.
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
func GetProfilePictureInfo(
	ctx context.Context, t Transport,
	jid types.JID, params *GetProfilePictureParams,
) (*types.ProfilePictureInfo, error) {
	attrs := waBinary.Attrs{
		"query": profilePictureQueryURL,
	}
	var target, to types.JID
	if params == nil {
		params = &GetProfilePictureParams{}
	}
	if params.Preview {
		attrs["type"] = profilePictureTypePreview
	} else {
		attrs["type"] = profilePictureTypeImage
	}
	if params.ExistingID != "" {
		attrs["id"] = params.ExistingID
	}
	if params.InviteCode != "" {
		attrs["invite"] = params.InviteCode
	}

	var expectWrapped bool
	var content []waBinary.Node
	namespace := profilePictureIQNamespace
	if params.IsCommunity {
		target = types.EmptyJID
		// Foto de comunidade sai pelo namespace de grupo, nao pelo de perfil.
		// Importar group/ aqui nao fecha ciclo: group nao importa user. A
		// alternativa (duplicar o literal "w:g2") foi recusada — e' um valor de
		// wire, e duas definicoes divergem em silencio.
		namespace = group.IQNamespace
		to = jid
		attrs["parent_group_jid"] = jid
		expectWrapped = true
		content = []waBinary.Node{{
			Tag: picturesNodeTag,
			Content: []waBinary.Node{{
				Tag:   pictureNodeTag,
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
		if token, _ := t.Store().PrivacyTokens.GetPrivacyToken(ctx, jid); token != nil {
			pictureContent = []waBinary.Node{{
				Tag:     profilePictureTokenNodeTag,
				Content: token.Token,
			}}
		}

		content = []waBinary.Node{{
			Tag:     pictureNodeTag,
			Attrs:   attrs,
			Content: pictureContent,
		}}
	}
	iqErrs := t.IQErrors()
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: namespace,
		Type:      IQGet,
		To:        to,
		Target:    target,
		Content:   content,
	})
	if errors.Is(err, iqErrs.NotAuthorized) {
		return nil, t.WrapIQError(ErrProfilePictureUnauthorized, err)
	} else if errors.Is(err, iqErrs.NotFound) {
		return nil, t.WrapIQError(ErrProfilePictureNotSet, err)
	} else if err != nil {
		return nil, err
	}
	if expectWrapped {
		pics, ok := resp.GetOptionalChildByTag(picturesNodeTag)
		if !ok {
			return nil, t.ElementMissing(picturesNodeTag, "response to profile picture query")
		}
		resp = &pics
	}
	picture, ok := resp.GetOptionalChildByTag(pictureNodeTag)
	if !ok {
		if params.ExistingID != "" {
			return nil, nil
		}
		return nil, t.ElementMissing(pictureNodeTag, "response to profile picture query")
	}
	var info types.ProfilePictureInfo
	ag := picture.AttrGetter()
	if ag.OptionalInt("status") == profilePictureStatusNotModified {
		return nil, nil
	} else if ag.OptionalInt("status") == profilePictureStatusNotSet {
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
