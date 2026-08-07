// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// Fachada do dominio de grupos. A logica vive em internal/wa-noise/group e
// opera sobre group.Transport; aqui ficam so' os metodos de *Client que
// delegam, mais os apelidos de tipo que preservam a API historica do pacote.
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 6".
//
// Os metodos **exportados** recusam receiver nil com ErrClientIsNil, como as
// fachadas dos lotes 1-5. Os **nao exportados** (sendGroupIQ, cacheGroupInfo,
// getGroupInfo, getCachedGroupData, parseGroupNode, parseGroupCreate,
// parseGroupChange, updateGroupParticipantCache, parseGroupNotification) NAO
// ganham essa guarda: eles sao citados por internals.go (gerado, fora de
// escopo — F29) e chamados de dentro do proprio cliente, onde `cli` nunca e'
// nil; adicionar a guarda mudaria assinaturas que o gerador copia.

// groupMetaCache e' apelido de tipo, e nao um tipo novo, porque internals.go
// (gerado, fora do escopo deste lote) cita o nome antigo na assinatura de
// DangerousInternalClient.GetCachedGroupData, e sendfb_transport.go declara uma
// variavel com ele.
type groupMetaCache = group.Meta

func (cli *Client) sendGroupIQ(ctx context.Context, iqType infoQueryType, jid types.JID, content waBinary.Node) (*waBinary.Node, error) {
	return group.SendIQ(ctx, cli.groupT(), group.IQType(iqType), jid, content)
}

// GetJoinedGroups returns the list of groups the user is participating in.
func (cli *Client) GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return group.GetJoined(ctx, cli.groupT())
}

// GetSubGroups gets the subgroups of the given community.
func (cli *Client) GetSubGroups(ctx context.Context, community types.JID) ([]*types.GroupLinkTarget, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return group.GetSubGroups(ctx, cli.groupT(), community)
}

// GetLinkedGroupsParticipants gets all the participants in the groups of the given community.
func (cli *Client) GetLinkedGroupsParticipants(ctx context.Context, community types.JID) ([]types.JID, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return group.GetLinkedParticipants(ctx, cli.groupT(), community)
}

// GetGroupInfo requests basic info about a group chat from the WhatsApp servers.
func (cli *Client) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return cli.getGroupInfo(ctx, jid, true)
}

func (cli *Client) cacheGroupInfo(groupInfo *types.GroupInfo, lock bool) ([]store.LIDMapping, []store.RedactedPhoneEntry) {
	return group.CacheInfo(cli.groupT(), groupInfo, lock)
}

func (cli *Client) getGroupInfo(ctx context.Context, jid types.JID, lockParticipantCache bool) (*types.GroupInfo, error) {
	return group.GetInfo(ctx, cli.groupT(), jid, lockParticipantCache)
}

func (cli *Client) getCachedGroupData(ctx context.Context, jid types.JID) (*groupMetaCache, error) {
	return group.GetOrFetch(ctx, cli.groupT(), jid)
}
