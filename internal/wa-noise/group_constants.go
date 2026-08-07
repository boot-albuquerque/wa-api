// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

// groupIQNamespace e' o namespace de todo IQ de grupo (sendGroupIQ).
const groupIQNamespace = "w:g2"

// Tags de no' do protocolo de grupo.
//
// Estas nao seguem a convencao dos lotes 1-5 ("tag usada uma vez fica
// literal") de proposito: cada uma e' **lida** pelo switch de parseGroupNode
// ou de parseGroupChange (group_parse.go / group_notification.go) e
// **escrita** pelos arquivos de acao (group_create.go, group_settings.go) —
// dois lados do mesmo valor de wire, em arquivos distintos. Como literais, um
// typo em qualquer um dos lados so' apareceria em producao: o `case`
// simplesmente nunca casaria, e a mudanca seria silenciosamente ignorada.
const (
	groupNodeTag                   = "group"
	groupParticipantTag            = "participant"
	groupDescriptionTag            = "description"
	groupDescriptionBodyTag        = "body"
	groupSubjectTag                = "subject"
	groupAnnouncementTag           = "announcement"
	groupNotAnnouncementTag        = "not_announcement"
	groupLockedTag                 = "locked"
	groupUnlockedTag               = "unlocked"
	groupEphemeralTag              = "ephemeral"
	groupNotEphemeralTag           = "not_ephemeral"
	groupMemberAddModeTag          = "member_add_mode"
	groupLinkedParentTag           = "linked_parent"
	groupParentTag                 = "parent"
	groupDefaultSubGroupTag        = "default_sub_group"
	groupIncognitoTag              = "incognito"
	groupMembershipApprovalModeTag = "membership_approval_mode"
	groupJoinTag                   = "group_join"
	groupSuspendedTag              = "suspended"
	groupUnsuspendedTag            = "unsuspended"
	groupCreateTag                 = "create"
	groupDeleteTag                 = "delete"
	groupInviteTag                 = "invite"
	groupLinkTag                   = "link"
	groupUnlinkTag                 = "unlink"
	groupAddRequestTag             = "add_request"
)

// Valores de atributo do protocolo de grupo.
const (
	// participantTypeAdmin / participantTypeSuperAdmin sao os valores do
	// atributo `type` de <participant>, lidos por parseParticipant.
	participantTypeAdmin      = "admin"
	participantTypeSuperAdmin = "superadmin"

	// groupJoinStateOn / groupJoinStateOff sao o atributo `state` de
	// <group_join>, escrito por CreateGroup e por SetGroupJoinApprovalMode.
	groupJoinStateOn  = "on"
	groupJoinStateOff = "off"

	// defaultMembershipApprovalMode e' o valor default de
	// `default_membership_approval_mode` ao criar uma comunidade.
	defaultMembershipApprovalMode = "request_required"

	// groupPhotoRemovedID e' o "ID" que SetGroupPhoto devolve quando a foto e'
	// removida em vez de trocada — o servidor nao devolve nenhum ID nesse caso.
	groupPhotoRemovedID = "remove"
)
