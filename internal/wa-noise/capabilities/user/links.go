// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"context"
	"errors"
	"strings"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// ResolveBusinessMessageLink resolves a business message short link and returns the target JID, business name and
// text to prefill in the input field (if any).
//
// The links look like https://wa.me/message/<code> or https://api.whatsapp.com/message/<code>. You can either provide
// the full link, or just the <code> part.
func ResolveBusinessMessageLink(
	ctx context.Context, t Transport, code string,
) (*types.BusinessMessageLinkTarget, error) {
	code = strings.TrimPrefix(code, BusinessMessageLinkPrefix)
	code = strings.TrimPrefix(code, BusinessMessageLinkDirectPrefix)

	resp, err := t.SendIQ(ctx, IQ{
		Namespace: qrIQNamespace,
		Type:      IQGet,
		// WhatsApp android doesn't seem to have a "to" field for this one at all, not sure why but it works
		Content: []waBinary.Node{{
			Tag: qrNodeTag,
			Attrs: waBinary.Attrs{
				"code": code,
			},
		}},
	})
	if errors.Is(err, t.IQErrors().NotFound) {
		return nil, t.WrapIQError(ErrBusinessMessageLinkNotFound, err)
	} else if err != nil {
		return nil, err
	}
	qrChild, ok := resp.GetOptionalChildByTag(qrNodeTag)
	if !ok {
		return nil, t.ElementMissing(qrNodeTag, "response to business message link query")
	}
	var target types.BusinessMessageLinkTarget
	ag := qrChild.AttrGetter()
	target.JID = ag.JID("jid")
	target.PushName = ag.String("notify")
	messageChild, ok := qrChild.GetOptionalChildByTag("message")
	if ok {
		messageBytes, _ := messageChild.Content.([]byte)
		target.Message = string(messageBytes)
	}
	businessChild, ok := qrChild.GetOptionalChildByTag(businessNodeTag)
	if ok {
		bag := businessChild.AttrGetter()
		target.IsSigned = bag.OptionalBool("is_signed")
		target.VerifiedName = bag.OptionalString("verified_name")
		target.VerifiedLevel = bag.OptionalString("verified_level")
	}
	return &target, ag.Error()
}

// ResolveContactQRLink resolves a link from a contact share QR code and returns the target JID and push name.
//
// The links look like https://wa.me/qr/<code> or https://api.whatsapp.com/qr/<code>. You can either provide
// the full link, or just the <code> part.
func ResolveContactQRLink(
	ctx context.Context, t Transport, code string,
) (*types.ContactQRLinkTarget, error) {
	code = strings.TrimPrefix(code, ContactQRLinkPrefix)
	code = strings.TrimPrefix(code, ContactQRLinkDirectPrefix)

	resp, err := t.SendIQ(ctx, IQ{
		Namespace: qrIQNamespace,
		Type:      IQGet,
		Content: []waBinary.Node{{
			Tag: qrNodeTag,
			Attrs: waBinary.Attrs{
				"code": code,
			},
		}},
	})
	if errors.Is(err, t.IQErrors().NotFound) {
		return nil, t.WrapIQError(ErrContactQRLinkNotFound, err)
	} else if err != nil {
		return nil, err
	}
	qrChild, ok := resp.GetOptionalChildByTag(qrNodeTag)
	if !ok {
		return nil, t.ElementMissing(qrNodeTag, "response to contact link query")
	}
	var target types.ContactQRLinkTarget
	ag := qrChild.AttrGetter()
	target.JID = ag.JID("jid")
	target.PushName = ag.OptionalString("notify")
	target.Type = ag.String("type")
	return &target, ag.Error()
}

// GetContactQRLink gets your own contact share QR link that can be resolved using ResolveContactQRLink
// (or scanned with the official apps when encoded as a QR code).
//
// If the revoke parameter is set to true, it will ask the server to revoke the previous link and generate a new one.
func GetContactQRLink(ctx context.Context, t Transport, revoke bool) (string, error) {
	action := qrActionGet
	if revoke {
		action = qrActionRevoke
	}
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: qrIQNamespace,
		Type:      IQSet,
		Content: []waBinary.Node{{
			Tag: qrNodeTag,
			Attrs: waBinary.Attrs{
				"type":   qrTypeContact,
				"action": action,
			},
		}},
	})
	if err != nil {
		return "", err
	}
	qrChild, ok := resp.GetOptionalChildByTag(qrNodeTag)
	if !ok {
		return "", t.ElementMissing(qrNodeTag, "response to own contact link fetch")
	}
	ag := qrChild.AttrGetter()
	return ag.String("code"), ag.Error()
}
