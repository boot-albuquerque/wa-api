package port

import (
	"context"

	"wa-api/pkg/domain"
)

// MediaMessenger envia uma mensagem de mídia NOVA para um JID, cobrindo o
// upload do anexo e a montagem do protobuf correspondente.
//
// Porta separada de TextMessenger de propósito: enviar mídia exige uma
// etapa de upload que enviar texto não tem, e o resultado só é
// domain.StatusSent depois que o envio (não o upload) retorna sucesso.
// CAP-02 prova a fronteira estreita que CAP-01 estabeleceu com o segundo
// caso real — a condição para introduzir uma porta nova, não a última;
// por isso SendImage fica aqui, e não estica TextMessenger num
// "SendAnything" que carregaria parâmetros que o envio de texto não usa.
type MediaMessenger interface {
	SessionGuard

	// SendImage sobe payload.Bytes para os servidores do WhatsApp e envia
	// uma ImageMessage para target. id, quando não-vazio, é o
	// identificador de mensagem que o chamador quer usar; o resultado
	// devolvido traz o ID e o Timestamp que a sessão REALMENTE usou —
	// nunca fabricados localmente. Upload bem-sucedido sem SendMessage
	// bem-sucedido NÃO produz domain.MessageSendResult algum: o erro do
	// SendMessage é propagado, sem tentativa de desfazer o upload (o
	// protocolo não oferece essa operação).
	SendImage(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error)

	// SendDocument sobe payload.Bytes (com wanoise.MediaDocument, não
	// MediaImage) e envia uma DocumentMessage para target, usando
	// payload.FileName como metadata pura (CAP-04). Mesma disciplina de
	// SendImage quanto a upload/envio e a ausência de rollback de upload.
	SendDocument(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error)
}
