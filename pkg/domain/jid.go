// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
// Entities são imutáveis e não dependem de frameworks ou bibliotecas externas.
package domain

// JID representa um WhatsApp JID (Jabber ID) no domínio.
// Abstrai wa-api/internal/wa-noise/types.JID para evitar vazamento de
// dependência de infraestrutura na camada de aplicação.
type JID string
