// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
// Entities são imutáveis e não dependem de frameworks ou bibliotecas externas.
package domain

import "strings"

// JID representa um WhatsApp JID (Jabber ID) no domínio.
// Abstrai wa-api/internal/wa-noise/protocol/types.JID para evitar vazamento de
// dependência de infraestrutura na camada de aplicação.
type JID string

// Os dois servidores que distinguem as duas identidades de uma pessoa no
// WhatsApp. São constantes porque a armadilha nº6 do ARMADILHAS.md é
// exatamente esta: LID e PN são o MESMO tipo Go, e o único sinal que os separa
// é o sufixo — um literal divergente aqui faz a distinção falhar em silêncio.
const (
	// ServerPN é o sufixo do número de telefone.
	ServerPN = "@s.whatsapp.net"
	// ServerLID é o sufixo da identidade oculta.
	ServerLID = "@lid"
)

// IsLID diz se o JID está no espaço @lid.
func (j JID) IsLID() bool { return hasSuffix(string(j), ServerLID) }

// IsPN diz se o JID é um número de telefone.
//
// Não é a negação de IsLID: há outros servidores (grupos, newsletters,
// broadcast), e tratar "não é LID" como "é telefone" mandaria um JID de grupo
// para o caminho de resolução de contacto.
func (j JID) IsPN() bool { return hasSuffix(string(j), ServerPN) }

// ServerNewsletter is the suffix of a WhatsApp channel (newsletter) JID.
//
// It lives here, next to ServerPN and ServerLID, for the same reason they do:
// the suffix is the ONLY signal that separates a channel from a person or a
// group, so a divergent literal makes the distinction fail silently. The
// vendored protocol spells the bare server without the "@"
// (internal/wa-noise/protocol/types/jid.go:24, NewsletterServer = "newsletter");
// this constant carries the "@" because the domain compares whole suffixes.
const ServerNewsletter = "@newsletter"

// IsNewsletter reports whether the JID addresses a channel.
//
// Unlike IsLID and IsPN it also demands a non-blank user part: those two answer
// "which identity space is this", while this one is used to ADMIT a request
// (F271), so "@newsletter" and "   @newsletter" — which name no channel — must
// not pass. `at > 0` is what carries "there is a user part"; the TrimSpace
// comparison is what rejects one made only of blanks.
func (j JID) IsNewsletter() bool {
	at := strings.LastIndexByte(string(j), '@')
	return at > 0 && string(j)[at:] == ServerNewsletter &&
		strings.TrimSpace(string(j)[:at]) == string(j)[:at]
}

// StatusBroadcastJID is the well-known destination for ephemeral status
// stories (image, video, audio). Sending a message to this JID triggers
// the broadcast-list resolution inside wa-noise (core/broadcast.go),
// which fans the message out to contacts according to privacy settings.
const StatusBroadcastJID JID = "status@broadcast"

// hasSuffix é mantido em vez de strings.HasSuffix porque as duas identidades
// (LID e PN) são a armadilha nº6 e o seu teste vive aqui — não porque o
// domínio evite o import, que IsNewsletter passou a precisar.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
