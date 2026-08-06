// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waVnameCert"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

func (cli *Client) parseBusinessProfile(node *waBinary.Node) (*types.BusinessProfile, error) {
	profileNode := node.GetChildByTag("profile")
	jid, ok := profileNode.AttrGetter().GetJID("jid", true)
	if !ok {
		return nil, errors.New("missing jid in business profile")
	}
	address, _ := profileNode.GetChildByTag("address").Content.([]byte)
	email, _ := profileNode.GetChildByTag("email").Content.([]byte)
	businessHour := profileNode.GetChildByTag("business_hours")
	businessHourTimezone := businessHour.AttrGetter().String("timezone")
	businessHoursConfigs := businessHour.GetChildren()
	businessHours := make([]types.BusinessHoursConfig, 0)
	for _, config := range businessHoursConfigs {
		if config.Tag != "business_hours_config" {
			continue
		}
		dow := config.AttrGetter().String("day_of_week")
		mode := config.AttrGetter().String("mode")
		openTime := config.AttrGetter().String("open_time")
		closeTime := config.AttrGetter().String("close_time")
		businessHours = append(businessHours, types.BusinessHoursConfig{
			DayOfWeek: dow,
			Mode:      mode,
			OpenTime:  openTime,
			CloseTime: closeTime,
		})
	}
	categoriesNode := profileNode.GetChildByTag("categories")
	categories := make([]types.Category, 0)
	for _, category := range categoriesNode.GetChildren() {
		if category.Tag != "category" {
			continue
		}
		id := category.AttrGetter().String("id")
		name, _ := category.Content.([]byte)
		categories = append(categories, types.Category{
			ID:   id,
			Name: string(name),
		})
	}
	profileOptionsNode := profileNode.GetChildByTag("profile_options")
	profileOptions := make(map[string]string)
	for _, option := range profileOptionsNode.GetChildren() {
		optValueBytes, _ := option.Content.([]byte)
		profileOptions[option.Tag] = string(optValueBytes)
		// TODO parse bot_fields
	}
	return &types.BusinessProfile{
		JID:                   jid,
		Email:                 string(email),
		Address:               string(address),
		Categories:            categories,
		ProfileOptions:        profileOptions,
		BusinessHoursTimeZone: businessHourTimezone,
		BusinessHours:         businessHours,
	}, nil
}

// GetBusinessProfile gets the profile info of a WhatsApp business account
func (cli *Client) GetBusinessProfile(ctx context.Context, jid types.JID) (*types.BusinessProfile, error) {
	resp, err := cli.sendIQ(ctx, infoQuery{
		Type:      iqGet,
		To:        types.ServerJID,
		Namespace: "w:biz",
		Content: []waBinary.Node{{
			Tag: "business_profile",
			Attrs: waBinary.Attrs{
				"v": "244",
			},
			Content: []waBinary.Node{{
				Tag: "profile",
				Attrs: waBinary.Attrs{
					"jid": jid,
				},
			}},
		}},
	})
	if err != nil {
		return nil, err
	}
	node, ok := resp.GetOptionalChildByTag("business_profile")
	if !ok {
		return nil, &ElementMissingError{Tag: "business_profile", In: "response to business profile query"}
	}
	return cli.parseBusinessProfile(&node)
}

func (cli *Client) updateBusinessName(ctx context.Context, user, userAlt types.JID, messageInfo *types.MessageInfo, name string) {
	if cli.Store.Contacts == nil {
		return
	}
	changed, previousName, err := cli.Store.Contacts.PutBusinessName(ctx, user, name)
	if err != nil {
		cli.Log.Errorf("Failed to save business name of %s in device store: %v", user, err)
	} else if changed {
		userAlt = userAlt.ToNonAD()
		if userAlt.IsEmpty() {
			userAlt, _ = cli.Store.GetAltJID(ctx, user)
		}
		if !userAlt.IsEmpty() {
			_, _, err = cli.Store.Contacts.PutBusinessName(ctx, userAlt, name)
			if err != nil {
				cli.Log.Errorf("Failed to save push name of %s in device store: %v", userAlt, err)
			}
		}
		cli.Log.Debugf("Business name of %s changed from %s to %s, dispatching event", user, previousName, name)
		cli.dispatchEvent(&events.BusinessName{
			JID:             user,
			Message:         messageInfo,
			OldBusinessName: previousName,
			NewBusinessName: name,
		})
	}
}

func parseVerifiedName(businessNode waBinary.Node) (*types.VerifiedName, error) {
	if businessNode.Tag != "business" {
		return nil, nil
	}
	verifiedNameNode, ok := businessNode.GetOptionalChildByTag("verified_name")
	if !ok {
		return nil, nil
	}
	return parseVerifiedNameContent(verifiedNameNode)
}

func parseVerifiedNameContent(verifiedNameNode waBinary.Node) (*types.VerifiedName, error) {
	rawCert, ok := verifiedNameNode.Content.([]byte)
	if !ok {
		return nil, nil
	}

	var cert waVnameCert.VerifiedNameCertificate
	err := proto.Unmarshal(rawCert, &cert)
	if err != nil {
		return nil, err
	}
	var certDetails waVnameCert.VerifiedNameCertificate_Details
	err = proto.Unmarshal(cert.GetDetails(), &certDetails)
	if err != nil {
		return nil, err
	}
	return &types.VerifiedName{
		Certificate: &cert,
		Details:     &certDetails,
	}, nil
}
