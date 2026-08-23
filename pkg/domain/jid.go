// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
// Entities são imutáveis e não dependem de frameworks ou bibliotecas externas.
package domain

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

// hasSuffix evita importar "strings" no domínio por uma única chamada.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
