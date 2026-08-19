package port

import (
	"context"

	"wa-api/pkg/domain"
)

// SimpleMessenger envia uma mensagem de protocolo NOVA montada inteiramente
// a partir de dados já presentes no payload — sem upload, sem fetch
// externo, sem conversão (CAP-08A/CAP-08B).
//
// Porta separada de TextMessenger e MediaMessenger de propósito: Location e
// Contact não têm etapa de obtenção de bytes (TextMessenger.SendText não
// serve — o resultado não é Conversation/ExtendedTextMessage) nem etapa de
// upload (MediaMessenger enfiaria aqui um upload que estas duas capacidades
// nunca fazem, tornando o contrato dele falso). As duas convergem na MESMA
// forma — montar um protobuf a partir de campos escalares do request e
// enviar — o que é exatamente a condição para uma porta própria em vez de
// duas, seguindo a mesma disciplina de TextMessenger/MediaMessenger
// (CAP-01/CAP-02): porta nova só no segundo caso real que prova a
// fronteira, nunca antes.
type SimpleMessenger interface {
	SessionGuard

	// SendLocation monta um LocationMessage a partir de payload
	// (DegreesLatitude/DegreesLongitude/Name — nenhum outro campo) e o
	// envia para target. id, quando não-vazio, é o identificador de
	// mensagem que o chamador quer usar; o resultado devolvido traz o ID e
	// o Timestamp que a sessão REALMENTE usou, nunca fabricados
	// localmente.
	SendLocation(ctx context.Context, txtID string, target domain.JID, payload domain.LocationPayload, id string) (domain.MessageSendResult, error)

	// SendContact monta um ContactMessage a partir de payload
	// (DisplayName/Vcard — nenhum outro campo) e o envia para target.
	// Mesma disciplina de SendLocation quanto a id e ao resultado
	// devolvido.
	SendContact(ctx context.Context, txtID string, target domain.JID, payload domain.ContactPayload, id string) (domain.MessageSendResult, error)

	// SendPoll monta um PollCreationMessage a partir de payload
	// (Name/Options) e o envia para target, que é sempre um grupo nesta
	// rota. Mesma disciplina de SendLocation quanto a id e ao resultado
	// devolvido (CAP-14).
	//
	// Poll cabe AQUI, e não em ChatMessenger, porque enquete é CRIAÇÃO de
	// mensagem — ChatMessenger cobre operação sobre mensagem que JÁ
	// EXISTE (marcar lida, reagir, revogar, editar). E cabe aqui em vez de
	// numa porta nova porque tem exatamente a forma que define esta:
	// campos escalares do request viram protobuf e são enviados, sem
	// etapa de obtenção de bytes e sem upload. É o terceiro caso real da
	// mesma fronteira, depois de Location e Contact.
	//
	// A implementação DEVE, depois de o envio retornar sucesso, memorizar
	// o texto em claro de Options associado ao ID que a sessão realmente
	// usou. Isto não é detalhe de implementação opcional: o voto chega
	// como SHA-256 do texto da opção (ver
	// internal/wa-noise/capabilities/message/poll.go:38), e sem o texto
	// guardado o consumidor do webhook recebe hashes sem significado (ver
	// pkg/bootstrap/eventhandler_message.go:130). Onde essa memória vive é
	// escolha da infra; QUE ela exista é contrato desta porta.
	SendPoll(ctx context.Context, txtID string, target domain.JID, payload domain.PollPayload, id string) (domain.MessageSendResult, error)
}
