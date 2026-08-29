package user

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waVnameCert"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// ParseBusinessProfile le o <business_profile> da resposta de w:biz.
func ParseBusinessProfile(node *waBinary.Node) (*types.BusinessProfile, error) {
	profileNode := node.GetChildByTag(profileNodeTag)
	jid, ok := profileNode.AttrGetter().GetJID("jid", true)
	if !ok {
		return nil, errors.New("missing jid in business profile")
	}
	address := NodeContentString(profileNode.GetChildByTag("address"))
	email := NodeContentString(profileNode.GetChildByTag("email"))
	businessHour := profileNode.GetChildByTag("business_hours")
	businessHourTimezone := businessHour.AttrGetter().String("timezone")
	businessHoursConfigs := businessHour.GetChildren()
	businessHours := make([]types.BusinessHoursConfig, 0)
	for _, config := range businessHoursConfigs {
		if config.Tag != businessHoursConfigTag {
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
		if category.Tag != businessCategoryTag {
			continue
		}
		id := category.AttrGetter().String("id")
		categories = append(categories, types.Category{
			ID:   id,
			Name: NodeContentString(category),
		})
	}
	profileOptionsNode := profileNode.GetChildByTag("profile_options")
	profileOptions := make(map[string]string)
	for _, option := range profileOptionsNode.GetChildren() {
		profileOptions[option.Tag] = NodeContentString(option)
		// TODO parse bot_fields
	}
	return &types.BusinessProfile{
		JID:                   jid,
		Email:                 email,
		Address:               address,
		Categories:            categories,
		ProfileOptions:        profileOptions,
		BusinessHoursTimeZone: businessHourTimezone,
		BusinessHours:         businessHours,
	}, nil
}

// GetBusinessProfile gets the profile info of a WhatsApp business account
func GetBusinessProfile(ctx context.Context, t Transport, jid types.JID) (*types.BusinessProfile, error) {
	resp, err := t.SendIQ(ctx, IQ{
		Type:      IQGet,
		To:        types.ServerJID,
		Namespace: businessIQNamespace,
		Content: []waBinary.Node{{
			Tag: businessProfileNodeTag,
			Attrs: waBinary.Attrs{
				"v": businessProfileVersion,
			},
			Content: []waBinary.Node{{
				Tag: profileNodeTag,
				Attrs: waBinary.Attrs{
					"jid": jid,
				},
			}},
		}},
	})
	if err != nil {
		return nil, err
	}
	node, ok := resp.GetOptionalChildByTag(businessProfileNodeTag)
	if !ok {
		return nil, t.ElementMissing(businessProfileNodeTag, "response to business profile query")
	}
	return ParseBusinessProfile(&node)
}

// UpdateBusinessName grava o nome verificado de uma conta business e, se mudou,
// replica para o JID alternativo (LID <-> PN) e emite events.BusinessName.
func UpdateBusinessName(
	ctx context.Context, t Transport,
	jid, jidAlt types.JID, messageInfo *types.MessageInfo, name string,
) {
	if t.Store().Contacts == nil {
		return
	}
	changed, previousName, err := t.Store().Contacts.PutBusinessName(ctx, jid, name)
	if err != nil {
		t.Log().Errorf("Failed to save business name of %s in device store: %v", jid, err)
	} else if changed {
		jidAlt = jidAlt.ToNonAD()
		if jidAlt.IsEmpty() {
			jidAlt, _ = t.Store().GetAltJID(ctx, jid)
		}
		if !jidAlt.IsEmpty() {
			_, _, err = t.Store().Contacts.PutBusinessName(ctx, jidAlt, name)
			if err != nil {
				// Dizia "push name" aqui, o que mandava quem investiga o log
				// para o caminho errado — a chamada e' PutBusinessName (F39).
				t.Log().Errorf("Failed to save business name of %s in device store: %v", jidAlt, err)
			}
		}
		t.Log().Debugf("Business name of %s changed from %s to %s, dispatching event", jid, previousName, name)
		t.DispatchEvent(&events.BusinessName{
			JID:             jid,
			Message:         messageInfo,
			OldBusinessName: previousName,
			NewBusinessName: name,
		})
	}
}

// ParseVerifiedName le o <business><verified_name> de uma resposta usync.
// Ausencia nao e' erro: um usuario comum, sem conta business, cai aqui.
func ParseVerifiedName(businessNode waBinary.Node) (*types.VerifiedName, error) {
	if businessNode.Tag != businessNodeTag {
		return nil, nil
	}
	verifiedNameNode, ok := businessNode.GetOptionalChildByTag(verifiedNameNodeTag)
	if !ok {
		return nil, nil
	}
	return ParseVerifiedNameContent(verifiedNameNode)
}

// ParseVerifiedNameContent desserializa o certificado de nome verificado.
// Tambem e' chamado pelo dominio de mensagem (message_parse.go, na raiz), que
// recebe o no ja' desembrulhado.
func ParseVerifiedNameContent(verifiedNameNode waBinary.Node) (*types.VerifiedName, error) {
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
