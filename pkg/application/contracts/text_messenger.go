package port

import (
	"context"

	"wa-api/pkg/domain"
)

// TextMessenger envia uma mensagem de texto NOVA para um JID.
//
// É uma porta separada de ChatMessenger de propósito: ChatMessenger cobre
// operações sobre uma mensagem que já existe na conversa (confirmar
// leitura, reagir); TextMessenger cria uma mensagem. Misturar as duas
// tornaria o contrato de ChatMessenger incoerente só para reaproveitar a
// interface — CAP-01 autoriza esta única operação estreita, não uma
// abstração de "enviar qualquer coisa" (guardrail: sem generalização
// especulativa antes de dois casos reais).
type TextMessenger interface {
	SessionGuard

	// SendText envia text para target na sessão txtID. id, quando
	// não-vazio, é o identificador de mensagem que o chamador quer usar; o
	// resultado devolvido traz o ID que a sessão REALMENTE usou para
	// enviar, que o chamador deve tratar como o identificador canônico.
	//
	// preview, quando não-nil, faz o adapter montar um ExtendedTextMessage
	// com a metadata de link preview em vez do Conversation simples
	// (CAP-01.1). nil preserva o caminho provado no CAP-01 sem nenhuma
	// mudança.
	SendText(ctx context.Context, txtID string, target domain.JID, text string, preview *domain.LinkPreviewData, id string) (domain.MessageSendResult, error)
}
