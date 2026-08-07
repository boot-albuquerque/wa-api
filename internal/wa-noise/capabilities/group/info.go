package group

import (
	"context"
	"errors"
	"fmt"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// GetJoined returns the list of groups the user is participating in.
func GetJoined(ctx context.Context, t Transport) ([]*types.GroupInfo, error) {
	resp, err := sendIQ(ctx, t, IQGet, types.GroupServerJID, waBinary.Node{
		Tag: "participating",
		Content: []waBinary.Node{
			{Tag: "participants"},
			{Tag: descriptionTag},
		},
	})
	if err != nil {
		return nil, err
	}
	groups, ok := resp.GetOptionalChildByTag("groups")
	if !ok {
		return nil, t.ElementMissing("groups", "response to group list query")
	}
	children := groups.GetChildren()
	infos := make([]*types.GroupInfo, 0, len(children))
	var allLIDPairs []store.LIDMapping
	var allRedactedPhones []store.RedactedPhoneEntry
	for _, child := range children {
		if child.Tag != nodeTag {
			t.Log().Debugf("Unexpected child in group list response: %s", child.XMLString())
			continue
		}
		parsed, parseErr := ParseNode(t, &child)
		if parseErr != nil {
			t.Log().Warnf("Error parsing group %s: %v", parsed.JID, parseErr)
		}
		lidPairs, redactedPhones := CacheInfo(t, parsed, true)
		allLIDPairs = append(allLIDPairs, lidPairs...)
		allRedactedPhones = append(allRedactedPhones, redactedPhones...)
		infos = append(infos, parsed)
	}
	err = t.Store().LIDs.PutManyLIDMappings(ctx, allLIDPairs)
	if err != nil {
		t.Log().Warnf("Failed to store LID mappings from joined groups: %v", err)
	}
	err = t.Store().Contacts.PutManyRedactedPhones(ctx, allRedactedPhones)
	if err != nil {
		t.Log().Warnf("Failed to store redacted phones from joined groups: %v", err)
	}
	return infos, nil
}

// GetSubGroups gets the subgroups of the given community.
func GetSubGroups(ctx context.Context, t Transport, community types.JID) ([]*types.GroupLinkTarget, error) {
	res, err := sendIQ(ctx, t, IQGet, community, waBinary.Node{Tag: "sub_groups"})
	if err != nil {
		return nil, err
	}
	groups, ok := res.GetOptionalChildByTag("sub_groups")
	if !ok {
		return nil, t.ElementMissing("sub_groups", "response to subgroups query")
	}
	var parsedGroups []*types.GroupLinkTarget
	for _, child := range groups.GetChildren() {
		if child.Tag == nodeTag {
			parsedGroup, err := ParseLinkTargetNode(&child)
			if err != nil {
				return parsedGroups, fmt.Errorf("failed to parse group in subgroups list: %w", err)
			}
			parsedGroups = append(parsedGroups, &parsedGroup)
		}
	}
	return parsedGroups, nil
}

// GetLinkedParticipants gets all the participants in the groups of the given community.
func GetLinkedParticipants(ctx context.Context, t Transport, community types.JID) ([]types.JID, error) {
	res, err := sendIQ(ctx, t, IQGet, community, waBinary.Node{Tag: "linked_groups_participants"})
	if err != nil {
		return nil, err
	}
	participants, ok := res.GetOptionalChildByTag("linked_groups_participants")
	if !ok {
		return nil, t.ElementMissing("linked_groups_participants", "response to community participants query")
	}
	members, lidPairs := ParseParticipantList(&participants)
	if len(lidPairs) > 0 {
		err = t.Store().LIDs.PutManyLIDMappings(ctx, lidPairs)
		if err != nil {
			t.Log().Warnf("Failed to store LID mappings for community participants: %v", err)
		}
	}
	return members, nil
}

// CacheInfo grava groupInfo no cache de metadados e devolve os mapeamentos
// LID/PN e os telefones redigidos que o chamador deve persistir.
//
// `lock` diz se esta funcao deve tomar o lock do cache. E' false quando o
// chamador ja' o segura (GetOrFetch), pelo racional documentado em Cache.
//
// Nota de regressao (Fase E lote 6): lidPairs e redactedPhones sao alocados com
// `make(..., 0, N)` e preenchidos por **append**, nao por indice. Antes do lote
// 6 lidPairs usava `make(..., len(participants))` e so' escrevia nos indices
// dos participantes que tinham LID **e** PN, entregando entradas zeradas a
// PutManyLIDMappings. Manter o append aqui e' o que preserva a correcao.
func CacheInfo(t Transport, groupInfo *types.GroupInfo, lock bool) ([]store.LIDMapping, []store.RedactedPhoneEntry) {
	participants := make([]types.JID, len(groupInfo.Participants))
	lidPairs := make([]store.LIDMapping, 0, len(groupInfo.Participants))
	redactedPhones := make([]store.RedactedPhoneEntry, 0)
	for i, part := range groupInfo.Participants {
		participants[i] = part.JID
		if !part.PhoneNumber.IsEmpty() && !part.LID.IsEmpty() {
			lidPairs = append(lidPairs, store.LIDMapping{
				LID: part.LID,
				PN:  part.PhoneNumber,
			})
		}
		if part.DisplayName != "" && !part.LID.IsEmpty() {
			redactedPhones = append(redactedPhones, store.RedactedPhoneEntry{
				JID:           part.LID,
				RedactedPhone: part.DisplayName,
			})
		}
	}
	cache := t.Cache()
	if lock {
		cache.Lock()
		defer cache.Unlock()
	}
	cache.SetLocked(groupInfo.JID, &Meta{
		AddressingMode:             groupInfo.AddressingMode,
		CommunityAnnouncementGroup: groupInfo.IsAnnounce && groupInfo.IsDefaultSubGroup,
		Members:                    participants,
	})
	return lidPairs, redactedPhones
}

// GetInfo requests basic info about a group chat from the WhatsApp servers.
//
// lockParticipantCache e' repassado a CacheInfo; ver Cache.
func GetInfo(ctx context.Context, t Transport, jid types.JID, lockParticipantCache bool) (*types.GroupInfo, error) {
	iqErrs := t.IQErrors()
	res, err := sendIQ(ctx, t, IQGet, jid, waBinary.Node{
		Tag:   "query",
		Attrs: waBinary.Attrs{"request": "interactive"},
	})
	if errors.Is(err, iqErrs.NotFound) {
		return nil, t.WrapIQError(ErrNotFound, err)
	} else if errors.Is(err, iqErrs.Forbidden) {
		return nil, t.WrapIQError(ErrNotInGroup, err)
	} else if err != nil {
		return nil, err
	}

	groupNode, ok := res.GetOptionalChildByTag(nodeTag)
	if !ok {
		return nil, t.ElementMissing("groups", "response to group info query")
	}
	groupInfo, err := ParseNode(t, &groupNode)
	if err != nil {
		return groupInfo, err
	}
	lidPairs, redactedPhones := CacheInfo(t, groupInfo, lockParticipantCache)
	err = t.Store().LIDs.PutManyLIDMappings(ctx, lidPairs)
	if err != nil {
		t.Log().Warnf("Failed to store LID mappings for members of %s: %v", jid, err)
	}
	err = t.Store().Contacts.PutManyRedactedPhones(ctx, redactedPhones)
	if err != nil {
		t.Log().Warnf("Failed to store redacted phones for members of %s: %v", jid, err)
	}
	return groupInfo, nil
}

// GetOrFetch devolve o metadado em cache de jid, consultando o servidor se ele
// ainda nao estiver la'.
//
// O lock do cache e' segurado pela funcao inteira, **incluindo** a ida a' rede
// de GetInfo — e' por isso que GetInfo e' chamado com lockParticipantCache
// false. Era exatamente assim no getCachedGroupData da raiz antes da extracao;
// mudar isso trocaria uma secao critica por duas e permitiria duas consultas
// concorrentes para o mesmo grupo.
//
// Pode devolver (nil, nil): quando a consulta teve sucesso mas o servidor
// ecoou um `id` diferente do consultado, a entrada foi gravada sob outra chave.
// Os dois chamadores (send_prepare.go e sendfb_transport.go, na raiz) tratam
// esse caso explicitamente.
func GetOrFetch(ctx context.Context, t Transport, jid types.JID) (*Meta, error) {
	cache := t.Cache()
	cache.Lock()
	defer cache.Unlock()
	if val, ok := cache.GetLocked(jid); ok {
		return val, nil
	}
	_, err := GetInfo(ctx, t, jid, false)
	if err != nil {
		return nil, err
	}
	val, _ := cache.GetLocked(jid)
	return val, nil
}
