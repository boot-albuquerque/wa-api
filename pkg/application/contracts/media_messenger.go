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
	SendImage(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error)

	// SendDocument sobe payload.Bytes (com noise.MediaDocument, não
	// MediaImage) e envia uma DocumentMessage para target, usando
	// payload.FileName como metadata pura (CAP-04). Mesma disciplina de
	// SendImage quanto a upload/envio e a ausência de rollback de upload.
	SendDocument(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error)

	// SendAudio sobe payload.Bytes (com noise.MediaAudio) e envia uma
	// AudioMessage para target, usando payload.PTT e payload.Seconds como
	// metadata de protocolo (CAP-05). Mesma disciplina de SendImage/
	// SendDocument quanto a upload/envio e a ausência de rollback de
	// upload.
	//
	// domain.AudioPayload — não domain.MediaPayload — de propósito:
	// AudioMessage carrega PTT e Seconds, campos de protocolo que
	// ImageMessage e DocumentMessage não têm. Acrescentar esses campos a
	// MediaPayload obrigaria SendImage e SendDocument a ignorá-los
	// silenciosamente, o que é exatamente a incoerência que motiva uma
	// assinatura própria em vez de reaproveitar o tipo. A PORTA continua
	// única (upload+envio de mídia é uma capacidade coerente) — só o
	// payload de áudio é um tipo à parte.
	SendAudio(ctx context.Context, txtID string, target domain.JID, payload domain.AudioPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)

	// SendVideo sobe payload.Bytes (com noise.MediaVideo, não
	// MediaImage/MediaDocument/MediaAudio) e envia uma VideoMessage para
	// target, usando payload.Caption como metadata pura (CAP-06). Mesma
	// disciplina de SendImage/SendDocument/SendAudio quanto a upload/envio
	// e a ausência de rollback de upload.
	//
	// domain.MediaPayload — não um tipo próprio como AudioPayload — de
	// propósito: VideoMessage não carrega nenhum campo de protocolo que
	// MediaPayload não já tenha (sem PTT, sem Seconds; Seconds/Width/
	// Height/GIF/playback nunca eram preenchidos historicamente e não são
	// implementados aqui — sem probing).
	SendVideo(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error)

	// SendSticker sobe payload.Bytes (com noise.MediaImage — sticker NÃO
	// tem MediaType próprio no SDK) e envia uma StickerMessage para target
	// (CAP-07). payload.Bytes/payload.MimeType TÊM de ser os bytes/MIME já
	// processados pelo pipeline de sticker (appport.StickerProcessor) — o
	// WebP convertido, nunca o input cru. Mesma disciplina de SendImage/
	// SendDocument/SendAudio/SendVideo quanto a upload órfão sem tentativa
	// de desfazer.
	//
	// domain.MediaPayload — não um tipo próprio — de propósito: o único
	// campo de protocolo de StickerMessage além de URL/DirectPath/MediaKey/
	// Mimetype/FileEncSHA256/FileSHA256/FileLength é PngThumbnail, que viria
	// do REQUEST histórico e não existe em domain.SendStickerRequest (achado
	// CAP-07, reportado — não implementado por conta própria). O mesmo vale
	// para os quatro campos de metadata de pacote (PackId/PackName/
	// PackPublisher/Emojis), que alimentam a EXIF dentro do pipeline de
	// conversão, não StickerMessage diretamente.
	SendSticker(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)
}
