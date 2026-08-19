package chat

import (
	"context"
	"time"
	waclient "wa-api/pkg/infra/wa-noise/client"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"
	wasession "wa-api/pkg/infra/wa-noise/runtime/session"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"

	"google.golang.org/protobuf/proto"
)

// ChatMessengerAdapter implementa appport.ChatMessenger.
type ChatMessengerAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewChatMessengerAdapter cria o adapter com a função de lookup.
func NewChatMessengerAdapter(getClient waclient.Getter) *ChatMessengerAdapter {
	return &ChatMessengerAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// MarkRead confirma a leitura das mensagens ids.
func (a *ChatMessengerAdapter) MarkRead(ctx context.Context, txtID string, ids []string, at time.Time, chat, sender domain.JID) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jidChat, err := wajid.ToJID(chat)
	if err != nil {
		return err
	}
	jidSender, err := wajid.ToJID(sender)
	if err != nil {
		return err
	}
	return client.MarkRead(ctx, ids, at, jidChat, jidSender)
}

// SendReaction monta a mensagem de reação no formato do SDK e a envia.
//
// Esta montagem — MessageKey mais waE2E.ReactionMessage — vivia dentro de
// ReactUseCase. É exatamente o tipo de lógica que a ADR-001 previu migrar
// para o adapter, e o ponto em que o compilador deixa de cobrir a mudança.
func (a *ChatMessengerAdapter) SendReaction(ctx context.Context, txtID string, target domain.JID, reaction domain.Reaction) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	key := &waCommon.MessageKey{
		RemoteJID: proto.String(recipient.String()),
		FromMe:    proto.Bool(reaction.FromMe),
		ID:        proto.String(reaction.TargetMessageID),
	}
	if !reaction.FromMe && reaction.Participant != "" {
		key.Participant = proto.String(string(reaction.Participant))
	}

	msg := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key:               key,
			Text:              proto.String(reaction.Text),
			GroupingKey:       proto.String(reaction.Text),
			SenderTimestampMS: proto.Int64(reaction.SentAt.UnixMilli()),
		},
	}

	resp, err := client.SendMessage(ctx, recipient, msg)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp}, nil
}

// SendText monta uma mensagem de texto e a envia. Sem preview (nil), monta
// Conversation — a forma canônica do protocolo para texto sem link preview
// nem contexto, ver ARMADILHAS.md sobre não introduzir ExtendedTextMessage
// sem necessidade. Com preview (CAP-01.1), monta ExtendedTextMessage com a
// metadata de Open Graph que o chamador resolveu. Quando id não é vazio, é
// repassado como RequestExtra.ID para que o SDK use exatamente esse
// identificador; o ID devolvido em domain.MessageSendResult.ID vem sempre
// de resp.ID — o identificador que o SDK REALMENTE usou — nunca do id de
// entrada por construção própria.
func (a *ChatMessengerAdapter) SendText(ctx context.Context, txtID string, target domain.JID, text string, preview *domain.LinkPreviewData, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	if preview != nil {
		msg = &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:          proto.String(text),
				MatchedText:   proto.String(preview.MatchedURL),
				Title:         proto.String(preview.Title),
				Description:   proto.String(preview.Description),
				JPEGThumbnail: preview.ThumbnailJPEG,
			},
		}
	}

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// SendImage sobe payload.Bytes para os servidores do WhatsApp e envia uma
// ImageMessage para target (CAP-02). O upload acontece ANTES do envio; se
// SendMessage falhar depois de um upload bem-sucedido, o erro é propagado
// sem tentativa de desfazer o upload — o protocolo não oferece essa
// operação, e um upload órfão não é uma mensagem entregue.
func (a *ChatMessengerAdapter) SendImage(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	uploaded, err := client.Upload(ctx, payload.Bytes, wanoise.MediaImage)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String(payload.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(payload.MimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(payload.Bytes))),
		},
	}

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// SendDocument sobe payload.Bytes (wanoise.MediaDocument) e envia uma
// DocumentMessage para target (CAP-04). payload.FileName é metadata pura —
// vai direto para DocumentMessage.FileName, sem sanitização e sem tocar o
// sistema de arquivos (mesma disciplina de SendImage quanto a upload
// órfão sem tentativa de desfazer).
func (a *ChatMessengerAdapter) SendDocument(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	uploaded, err := client.Upload(ctx, payload.Bytes, wanoise.MediaDocument)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			FileName:      proto.String(payload.FileName),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(payload.MimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(payload.Bytes))),
			Caption:       proto.String(payload.Caption),
		},
	}

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// SendAudio sobe payload.Bytes (wanoise.MediaAudio) e envia uma
// AudioMessage para target (CAP-05). payload.PTT e payload.Seconds são
// metadata de protocolo pura — vão direto para AudioMessage.PTT e
// AudioMessage.Seconds, sem transcoding nem cálculo (mesma disciplina de
// SendImage/SendDocument quanto a upload órfão sem tentativa de desfazer).
func (a *ChatMessengerAdapter) SendAudio(ctx context.Context, txtID string, target domain.JID, payload domain.AudioPayload, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	uploaded, err := client.Upload(ctx, payload.Bytes, wanoise.MediaAudio)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	ptt := payload.PTT
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(payload.MimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(payload.Bytes))),
			PTT:           proto.Bool(ptt),
			Seconds:       proto.Uint32(payload.Seconds),
		},
	}

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// SendVideo sobe payload.Bytes (wanoise.MediaVideo) e envia uma
// VideoMessage para target (CAP-06). payload.Caption é metadata pura — vai
// direto para VideoMessage.Caption, sem transcoding e sem geração de
// thumbnail (JPEGThumbnail vinha do request no histórico e não está no DTO
// atual — achado reportado, não implementado). Mesma disciplina de
// SendImage/SendDocument/SendAudio quanto a upload órfão sem tentativa de
// desfazer.
func (a *ChatMessengerAdapter) SendVideo(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	uploaded, err := client.Upload(ctx, payload.Bytes, wanoise.MediaVideo)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			Caption:       proto.String(payload.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(payload.MimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(payload.Bytes))),
		},
	}

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// SendSticker sobe payload.Bytes (wanoise.MediaImage — sticker não tem
// MediaType próprio) e envia uma StickerMessage para target (CAP-07).
// payload.Bytes/payload.MimeType já chegam aqui processados (WebP
// convertido) pelo usecase, via appport.StickerProcessor — este adapter não
// faz conversão nenhuma, só upload+envio, mesma disciplina de SendImage/
// SendDocument/SendAudio/SendVideo. PngThumbnail e os quatro campos de
// metadata de pacote (PackId/PackName/PackPublisher/Emojis) não são
// preenchidos: PngThumbnail viria do request histórico e não existe em
// domain.SendStickerRequest (achado CAP-07, reportado); os quatro de pacote
// já foram consumidos como EXIF dentro da conversão, não são campos de
// StickerMessage.
func (a *ChatMessengerAdapter) SendSticker(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	uploaded, err := client.Upload(ctx, payload.Bytes, wanoise.MediaImage)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(payload.MimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(payload.Bytes))),
		},
	}

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// SendLocation monta um LocationMessage a partir de payload e o envia para
// target (CAP-08A). Só DegreesLatitude/DegreesLongitude/Name são
// preenchidos — nenhum outro campo do protobuf (Address/URL/IsLive/
// AccuracyInMeters/SpeedInMps/DegreesClockwiseFromMagneticNorth/Comment/
// JPEGThumbnail), mesma disciplina do histórico
// (`git show 41bc8e2^:handlers.go`). Sem upload, sem fetch: os três campos
// já chegam prontos no payload.
func (a *ChatMessengerAdapter) SendLocation(ctx context.Context, txtID string, target domain.JID, payload domain.LocationPayload, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := &waE2E.Message{
		LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(payload.Latitude),
			DegreesLongitude: proto.Float64(payload.Longitude),
			Name:             proto.String(payload.Name),
		},
	}

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// SendContact monta um ContactMessage a partir de payload e o envia para
// target (CAP-08B). Só DisplayName/Vcard são preenchidos — nenhum outro
// campo do protobuf (IsSelfContact), mesma disciplina do histórico
// (`git show 41bc8e2^:handlers.go`). payload.Vcard é repassado como STRING
// crua, sem parse nem validação de formato.
func (a *ChatMessengerAdapter) SendContact(ctx context.Context, txtID string, target domain.JID, payload domain.ContactPayload, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String(payload.Name),
			Vcard:       proto.String(payload.Vcard),
		},
	}

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// RevokeMessage revoga a mensagem messageID na conversa target (CAP-10).
//
// O sender passado a BuildRevoke é types.EmptyJID de propósito: é o que
// marca a revogação como sendo de mensagem PRÓPRIA (ver
// internal/wa-noise/capabilities/message/builders.go:29 — com sender vazio
// a MessageKey sai com FromMe=true e sem Participant). Trocá-lo por
// qualquer outro JID muda a operação para "revogar mensagem de terceiro
// como admin de grupo", que esta API nunca expôs. É a mesma montagem do
// histórico (`git show 41bc8e2^:handlers.go`, linha 2870).
func (a *ChatMessengerAdapter) RevokeMessage(ctx context.Context, txtID string, target domain.JID, messageID string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := client.BuildRevoke(recipient, types.EmptyJID, types.MessageID(messageID))

	resp, err := client.SendMessage(ctx, recipient, msg)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// EditMessage substitui o texto da mensagem messageID na conversa target
// (CAP-10). O conteúdo novo é um ExtendedTextMessage só com Text, como no
// histórico (`git show 41bc8e2^:handlers.go`, na SendEditMessage) — o
// ContextInfo que aquele handler aceitava não existe em
// domain.SendEditMessageRequest e NÃO é reintroduzido aqui (achado
// reportado, ver HOUSEKEEP.md F134).
func (a *ChatMessengerAdapter) EditMessage(ctx context.Context, txtID string, target domain.JID, messageID, newText string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	newContent := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(newText),
		},
	}

	msg := client.BuildEdit(recipient, types.MessageID(messageID), newContent)

	resp, err := client.SendMessage(ctx, recipient, msg)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// Verificação em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.ChatMessenger   = (*ChatMessengerAdapter)(nil)
	_ appport.TextMessenger   = (*ChatMessengerAdapter)(nil)
	_ appport.MediaMessenger  = (*ChatMessengerAdapter)(nil)
	_ appport.SimpleMessenger = (*ChatMessengerAdapter)(nil)
)
