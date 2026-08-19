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
}
