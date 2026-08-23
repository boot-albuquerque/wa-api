package chat

import (
	"context"
	"errors"
	"strconv"
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

	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
)

// PollOptionRecorder memoriza o texto em claro das opções de uma enquete
// recém-enviada, indexado pelo ID da mensagem que a criou.
//
// Esta é a FRONTEIRA do guarda-opções, e ela fica aqui — na infra, ao lado
// da montagem do protobuf — de propósito. O motivo de guardar é puramente de
// wire: o voto chega como SHA-256 do texto da opção
// (internal/wa-noise/capabilities/message/poll.go:38), então quem recebe o
// webhook precisa do texto para casar hash com significado
// (pkg/bootstrap/eventhandler_message.go:130). Isso não é regra de negócio, e
// o use case não pode conhecer o ClientManager.
//
// Interface estreita, e não *registry.ClientManager: o adapter usa UM método
// dele, e depender do tipo concreto arrastaria o registry inteiro para dentro
// deste pacote (e para dentro dos testes dele). Em produção é
// clientManager.SetPollOptions.
type PollOptionRecorder interface {
	SetPollOptions(userID, msgID string, options []string)
}

// ChatMessengerAdapter implementa appport.ChatMessenger.
type ChatMessengerAdapter struct {
	*wasession.SessionGuardAdapter

	polls PollOptionRecorder
}

// NewChatMessengerAdapter cria o adapter com a função de lookup.
func NewChatMessengerAdapter(getClient waclient.Getter) *ChatMessengerAdapter {
	return &ChatMessengerAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// WithPollOptions liga o registrador de opções de enquete ao adapter e
// devolve o próprio adapter, para encadear no wiring.
//
// É opcional na CONSTRUÇÃO e obrigatório no USO: SendPoll recusa-se a enviar
// sem ele (ver errNoPollOptionRecorder). Enviar a enquete e não guardar as
// opções produziria exatamente o defeito que ninguém adivinha — enquete
// criada, votos ilegíveis —, e um envio que falha é preferível a um envio que
// mente.
func (a *ChatMessengerAdapter) WithPollOptions(rec PollOptionRecorder) *ChatMessengerAdapter {
	a.polls = rec
	return a
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

// replyContextInfo builds a ContextInfo from the domain ReplyContext.
// Returns nil when replyTo is nil, so callers can guard with a single
// nil check — the compatibility rule (no ContextInfo when field absent)
// falls out naturally.
func replyContextInfo(replyTo *domain.ReplyContext) *waE2E.ContextInfo {
	if replyTo == nil {
		return nil
	}
	ci := &waE2E.ContextInfo{}
	if replyTo.StanzaID != "" {
		ci.StanzaID = proto.String(replyTo.StanzaID)
	}
	if replyTo.Participant != "" {
		ci.Participant = proto.String(replyTo.Participant)
	}
	if replyTo.QuotedText != "" {
		ci.QuotedMessage = &waE2E.Message{
			Conversation: proto.String(replyTo.QuotedText),
		}
	}
	return ci
}

// SendText monta uma mensagem de texto e a envia. Sem preview (nil) nem
// replyTo (nil), monta Conversation — a forma canônica do protocolo para
// texto sem link preview nem contexto, ver ARMADILHAS.md sobre não
// introduzir ExtendedTextMessage sem necessidade. Com preview (CAP-01.1)
// ou replyTo (CAP-46A), monta ExtendedTextMessage; os dois podem coexistir.
// Quando id não é vazio, é repassado como RequestExtra.ID para que o SDK
// use exatamente esse identificador; o ID devolvido em
// domain.MessageSendResult.ID vem sempre de resp.ID — o identificador que
// o SDK REALMENTE usou — nunca do id de entrada por construção própria.
func (a *ChatMessengerAdapter) SendText(ctx context.Context, txtID string, target domain.JID, text string, preview *domain.LinkPreviewData, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	needsExtended := preview != nil || replyTo != nil

	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	if needsExtended {
		etm := &waE2E.ExtendedTextMessage{
			Text: proto.String(text),
		}
		if preview != nil {
			etm.MatchedText = proto.String(preview.MatchedURL)
			etm.Title = proto.String(preview.Title)
			etm.Description = proto.String(preview.Description)
			etm.JPEGThumbnail = preview.ThumbnailJPEG
			if len(preview.HQImageData) > 0 {
				uploaded, upErr := client.Upload(ctx, preview.HQImageData, wanoise.MediaLinkThumbnail)
				if upErr != nil {
					log.Warn().Err(upErr).Str("txtID", txtID).
						Msg("link preview HQ thumbnail upload failed, sending inline thumbnail only")
				} else {
					etm.ThumbnailDirectPath = proto.String(uploaded.DirectPath)
					etm.ThumbnailSHA256 = uploaded.FileSHA256
					etm.ThumbnailEncSHA256 = uploaded.FileEncSHA256
					etm.MediaKey = uploaded.MediaKey
					etm.MediaKeyTimestamp = proto.Int64(time.Now().Unix())
					etm.ThumbnailWidth = proto.Uint32(preview.HQWidth)
					etm.ThumbnailHeight = proto.Uint32(preview.HQHeight)
				}
			}
		}
		if ci := replyContextInfo(replyTo); ci != nil {
			etm.ContextInfo = ci
		}
		msg = &waE2E.Message{ExtendedTextMessage: etm}
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
func (a *ChatMessengerAdapter) SendImage(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
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
			JPEGThumbnail: payload.JPEGThumbnail,
		},
	}
	if ci := replyContextInfo(replyTo); ci != nil {
		msg.ImageMessage.ContextInfo = ci
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
func (a *ChatMessengerAdapter) SendDocument(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
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
	if ci := replyContextInfo(replyTo); ci != nil {
		msg.DocumentMessage.ContextInfo = ci
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
func (a *ChatMessengerAdapter) SendAudio(ctx context.Context, txtID string, target domain.JID, payload domain.AudioPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
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
			Waveform:      payload.Waveform,
		},
	}
	if ci := replyContextInfo(replyTo); ci != nil {
		msg.AudioMessage.ContextInfo = ci
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
func (a *ChatMessengerAdapter) SendVideo(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
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
			JPEGThumbnail: payload.JPEGThumbnail,
		},
	}
	if ci := replyContextInfo(replyTo); ci != nil {
		msg.VideoMessage.ContextInfo = ci
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
func (a *ChatMessengerAdapter) SendSticker(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
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
			PngThumbnail:  payload.PngThumbnail,
		},
	}
	if ci := replyContextInfo(replyTo); ci != nil {
		msg.StickerMessage.ContextInfo = ci
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
func (a *ChatMessengerAdapter) SendLocation(ctx context.Context, txtID string, target domain.JID, payload domain.LocationPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
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
	if ci := replyContextInfo(replyTo); ci != nil {
		msg.LocationMessage.ContextInfo = ci
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
func (a *ChatMessengerAdapter) SendContact(ctx context.Context, txtID string, target domain.JID, payload domain.ContactPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
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
	if ci := replyContextInfo(replyTo); ci != nil {
		msg.ContactMessage.ContextInfo = ci
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

// singleSelectablePollOption é o número de opções que o votante pode
// escolher: UMA. É o terceiro argumento que o histórico sempre passou a
// BuildPollCreation (`git show 41bc8e2^:handlers.go`, linha 2796), e o valor
// vive aqui — junto da montagem do protobuf — porque domain.PollPayload não
// o expõe: a API pública nunca ofereceu múltipla escolha, e oferecer agora
// seria mudança de contrato, não recuperação da capability.
const singleSelectablePollOption = 1

// errNoPollOptionRecorder é a recusa de enviar enquete sem onde guardar as
// opções em claro. Ver WithPollOptions.
var errNoPollOptionRecorder = errors.New("poll option recorder not configured")

// SendPoll monta um PollCreationMessage a partir de payload e o envia para
// target (CAP-14), memorizando em seguida o texto em claro das opções.
//
// A ORDEM importa e é a do histórico (`git show 41bc8e2^:handlers.go`, linhas
// 2796-2805): monta, envia, e só DEPOIS de o envio retornar sucesso memoriza
// as opções. Memorizar antes deixaria entrada órfã para uma enquete que nunca
// existiu; memorizar num envio falho é lixo que só cresce.
//
// A chave da memória é resp.ID — o ID que a sessão REALMENTE usou —, e não o
// id pedido pelo chamador. O voto que chega depois traz o ID da mensagem de
// criação como o servidor o conhece
// (pkg/bootstrap/eventhandler_message.go:117); indexar pelo id pedido faria a
// busca falhar sempre que o chamador não tivesse forçado um.
func (a *ChatMessengerAdapter) SendPoll(ctx context.Context, txtID string, target domain.JID, payload domain.PollPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
	if a.polls == nil {
		return domain.MessageSendResult{}, errNoPollOptionRecorder
	}

	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := client.BuildPollCreation(payload.Name, payload.Options, singleSelectablePollOption)
	if ci := replyContextInfo(replyTo); ci != nil {
		msg.PollCreationMessage.ContextInfo = ci
	}

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	a.polls.SetPollOptions(txtID, string(resp.ID), payload.Options)

	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// hydratedTemplateID é o TemplateId de HydratedFourRowTemplate. O histórico
// o escrevia como o literal "1" (`git show 41bc8e2^:handlers.go`, linha
// 3230) e nunca o expôs no payload público, então ele vive aqui — junto da
// montagem do protobuf —, e como constante nomeada em vez de literal solto
// (ADR-0004).
const hydratedTemplateID = "1"

// firstTemplateButtonID é o número do PRIMEIRO botão na numeração automática
// dos botões de resposta rápida sem ID. O histórico começava em 1 e
// incrementava a cada botão, INCLUSIVE os de url e de call — que não usam o
// número, mas consomem uma posição. Preservar esse consumo importa: um
// template com [url, quickreply] numera o quickreply como "2", e mudar isso
// mudaria o id que volta no clique de quem já tem a mensagem no aparelho.
const firstTemplateButtonID = 1

// templateButtons traduz os botões de domínio para os `Hydrated*Button` do
// wire, preservando a ORDEM (é a ordem em que aparecem no aparelho de quem
// recebe) e numerando os de resposta rápida sem ID.
//
// DIVERGÊNCIA CONSCIENTE DO HISTÓRICO, registrada em HOUSEKEEP F140: o ramo
// `default` do histórico escrevia `proto.String(string(id))`, e `string(int)`
// em Go converte para RUNE — `string(1)` é "\x01", um caractere de controle,
// não "1". O ramo `quickreply` do mesmo switch usava `strconv.Itoa(id)`, que
// está certo; só o `default` errava. Aqui os DOIS usam strconv.Itoa: o
// defeito não é reproduzido, e em Go moderno a conversão sequer compilaria
// sem `rune()` explícito.
//
// Type desconhecido cai em quickreply, como no histórico: recusá-lo agora
// rejeitaria payloads que a rota sempre aceitou.
func templateButtons(buttons []domain.TemplateButton) []*waE2E.HydratedTemplateButton {
	out := make([]*waE2E.HydratedTemplateButton, 0, len(buttons))

	id := firstTemplateButtonID
	for _, item := range buttons {
		switch item.Type {
		case domain.TemplateButtonURL:
			out = append(out, &waE2E.HydratedTemplateButton{
				HydratedButton: &waE2E.HydratedTemplateButton_UrlButton{
					UrlButton: &waE2E.HydratedTemplateButton_HydratedURLButton{
						DisplayText: proto.String(item.DisplayText),
						URL:         proto.String(item.URL),
					},
				},
			})
		case domain.TemplateButtonCall:
			out = append(out, &waE2E.HydratedTemplateButton{
				HydratedButton: &waE2E.HydratedTemplateButton_CallButton{
					CallButton: &waE2E.HydratedTemplateButton_HydratedCallButton{
						DisplayText: proto.String(item.DisplayText),
						PhoneNumber: proto.String(item.PhoneNumber),
					},
				},
			})
		default:
			buttonID := item.ID
			if buttonID == "" {
				buttonID = strconv.Itoa(id)
			}
			out = append(out, &waE2E.HydratedTemplateButton{
				HydratedButton: &waE2E.HydratedTemplateButton_QuickReplyButton{
					QuickReplyButton: &waE2E.HydratedTemplateButton_HydratedQuickReplyButton{
						DisplayText: proto.String(item.DisplayText),
						ID:          proto.String(buttonID),
					},
				},
			})
		}
		id++
	}

	return out
}

// SendTemplate monta um TemplateMessage com HydratedFourRowTemplate a partir
// de payload e o envia para target (CAP-15). Só HydratedContentText,
// HydratedFooterText, HydratedButtons e TemplateId são preenchidos — nenhum
// outro campo do protobuf (HydratedTitleText, TitleText, LocationMessage,
// DocumentMessage, ImageMessage, VideoMessage), mesma disciplina do histórico
// (`git show 41bc8e2^:handlers.go`, linha 3226). Sem upload, sem fetch: tudo
// já chega pronto no payload.
func (a *ChatMessengerAdapter) SendTemplate(ctx context.Context, txtID string, target domain.JID, payload domain.TemplatePayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	msg := &waE2E.Message{
		TemplateMessage: &waE2E.TemplateMessage{
			HydratedTemplate: &waE2E.TemplateMessage_HydratedFourRowTemplate{
				HydratedContentText: proto.String(payload.Content),
				HydratedFooterText:  proto.String(payload.Footer),
				HydratedButtons:     templateButtons(payload.Buttons),
				TemplateID:          proto.String(hydratedTemplateID),
			},
		},
	}
	if ci := replyContextInfo(replyTo); ci != nil {
		msg.TemplateMessage.ContextInfo = ci
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
// (CAP-10). ctxInfo, quando não nil, monta ContextInfo no
// ExtendedTextMessage (citação e menções — F134).
func (a *ChatMessengerAdapter) EditMessage(ctx context.Context, txtID string, target domain.JID, messageID, newText string, ctxInfo *domain.EditContextInfo) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	ext := &waE2E.ExtendedTextMessage{
		Text: proto.String(newText),
	}
	if ctxInfo != nil {
		ci := &waE2E.ContextInfo{}
		if ctxInfo.StanzaID != "" {
			ci.StanzaID = proto.String(ctxInfo.StanzaID)
		}
		if ctxInfo.Participant != "" {
			ci.Participant = proto.String(ctxInfo.Participant)
		}
		if len(ctxInfo.MentionedJID) > 0 {
			ci.MentionedJID = ctxInfo.MentionedJID
		}
		ext.ContextInfo = ci
	}

	newContent := &waE2E.Message{ExtendedTextMessage: ext}

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
