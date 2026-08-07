package group

// IQNamespace e' o namespace de todo IQ de grupo (sendIQ deste pacote).
//
// Exportado porque user_avatar.go, na raiz, tambem o usa: a foto de uma
// **comunidade** sai por este namespace em vez do de perfil. E' o unico ponto
// fora deste pacote que precisa do valor, e mantê-lo aqui evita duas
// definicoes do mesmo literal de wire.
const IQNamespace = "w:g2"

// iqNamespace e' o apelido interno, usado pelo resto do pacote.
const iqNamespace = IQNamespace

// pictureIQNamespace e' o namespace de <iq> de foto de perfil. SetPhoto e' o
// unico ponto deste dominio que nao usa iqNamespace: a foto de grupo passa
// pelo mesmo endpoint da foto de usuario.
const pictureIQNamespace = "w:profile:picture"

// Tags de no' do protocolo de grupo.
//
// Estas nao seguem a convencao dos lotes 1-5 ("tag usada uma vez fica
// literal") de proposito: cada uma e' **lida** pelo switch de ParseNode
// ou de ParseChange (parse.go / notification.go) e **escrita** pelos arquivos
// de acao (create.go, settings.go) — dois lados do mesmo valor de wire, em
// arquivos distintos. Como literais, um typo em qualquer um dos lados so'
// apareceria em producao: o `case` simplesmente nunca casaria, e a mudanca
// seria silenciosamente ignorada.
const (
	nodeTag                   = "group"
	participantTag            = "participant"
	descriptionTag            = "description"
	descriptionBodyTag        = "body"
	subjectTag                = "subject"
	announcementTag           = "announcement"
	notAnnouncementTag        = "not_announcement"
	lockedTag                 = "locked"
	unlockedTag               = "unlocked"
	ephemeralTag              = "ephemeral"
	notEphemeralTag           = "not_ephemeral"
	memberAddModeTag          = "member_add_mode"
	linkedParentTag           = "linked_parent"
	parentTag                 = "parent"
	defaultSubGroupTag        = "default_sub_group"
	incognitoTag              = "incognito"
	membershipApprovalModeTag = "membership_approval_mode"
	joinTag                   = "group_join"
	suspendedTag              = "suspended"
	unsuspendedTag            = "unsuspended"
	createTag                 = "create"
	deleteTag                 = "delete"
	inviteTag                 = "invite"
	linkTag                   = "link"
	unlinkTag                 = "unlink"
	addRequestTag             = "add_request"
)

// Valores de atributo do protocolo de grupo.
const (
	// participantTypeAdmin / participantTypeSuperAdmin sao os valores do
	// atributo `type` de <participant>, lidos por ParseParticipant.
	participantTypeAdmin      = "admin"
	participantTypeSuperAdmin = "superadmin"

	// joinStateOn / joinStateOff sao o atributo `state` de <group_join>,
	// escrito por Create e por SetJoinApprovalMode.
	joinStateOn  = "on"
	joinStateOff = "off"

	// defaultMembershipApprovalMode e' o valor default de
	// `default_membership_approval_mode` ao criar uma comunidade.
	defaultMembershipApprovalMode = "request_required"

	// photoRemovedID e' o "ID" que SetPhoto devolve quando a foto e' removida
	// em vez de trocada — o servidor nao devolve nenhum ID nesse caso.
	photoRemovedID = "remove"
)

// InviteLinkPrefix e' o prefixo dos links de convite de grupo. A raiz reexporta
// este valor como whatsmeow.InviteLinkPrefix.
const InviteLinkPrefix = "https://chat.whatsapp.com/"
