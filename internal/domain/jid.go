// Package domain contém as entidades centrais do domínio disparazaap-wuzapi.
// Entities são imutáveis e não dependem de frameworks ou bibliotecas externas.
package domain

// JID representa um WhatsApp JID (Jabber ID) no domínio.
// Abstrai go.mau.fi/whatsmeow/types.JID para evitar vazamento de
// dependência de infraestrutura na camada de aplicação.
type JID string
