package chat

import (
	"context"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/noise/client"
	wajid "wa-api/pkg/infra/noise/mapping/jid"
)

// carouselMessageVersion acompanha nativeFlowMessageVersion: os dois viajam no
// mesmo envelope e o cliente do WhatsApp lê-os juntos.
const carouselMessageVersion = 1

// SendCarousel monta um InteractiveMessage cujo `oneof` é CarouselMessage.
//
// A ESTRUTURA é recursiva, e é isso que a torna diferente de tudo o resto na
// superfície de envio: **cada cartão é ele próprio um InteractiveMessage
// completo**, com o seu Header (imagem), Body, Footer e os seus botões de
// fluxo nativo. O carrossel é a moldura; os cartões são mensagens interativas
// aninhadas.
//
//	InteractiveMessage
//	└── carouselMessage          (oneof, campo 7)
//	    ├── cards []InteractiveMessage   <- cada um com header/body/footer/botões
//	    ├── messageVersion
//	    └── carouselCardType     HSCROLL_CARDS | ALBUM_IMAGE
//
// HSCROLL_CARDS é o carrossel clássico, com deslize horizontal. ALBUM_IMAGE
// agrupa as imagens como álbum. O tipo é do CARROSSEL, não do cartão — não se
// misturam os dois no mesmo envio.
//
// Os `bizNativeFlowNodes()` são os mesmos de SendButtons e NÃO são decoração:
// são os nós de negócio que fazem o cliente do WhatsApp tratar a mensagem como
// interativa em vez de a mostrar como "atualize o WhatsApp".
//
// O upload das imagens dos cartões acontece ANTES do envio, um por cartão. Se
// o envio falhar depois de uploads bem-sucedidos, o erro sobe sem tentativa de
// desfazer — o protocolo não oferece essa operação, e é a mesma disciplina de
// SendImage e SendButtons.
func (a *ChatMessengerAdapter) SendCarousel(ctx context.Context, txtID string, target domain.JID, payload domain.CarouselPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	cards := make([]*waE2E.InteractiveMessage, 0, len(payload.Cards))
	for i := range payload.Cards {
		card, cardErr := a.buildCarouselCard(ctx, client, payload.Cards[i])
		if cardErr != nil {
			return domain.MessageSendResult{}, cardErr
		}
		cards = append(cards, card)
	}

	tipo := waE2E.InteractiveMessage_CarouselMessage_HSCROLL_CARDS
	if payload.CardType == domain.CarouselAlbumImage {
		tipo = waE2E.InteractiveMessage_CarouselMessage_ALBUM_IMAGE
	}

	interactive := &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{Text: proto.String(payload.Body)},
		InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{
			CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
				Cards:            cards,
				MessageVersion:   proto.Int32(carouselMessageVersion),
				CarouselCardType: tipo.Enum(),
			},
		},
	}
	if payload.Footer != "" {
		interactive.Footer = &waE2E.InteractiveMessage_Footer{Text: proto.String(payload.Footer)}
	}
	if ci := buildContextInfo(replyTo, mentionedJID); ci != nil {
		interactive.ContextInfo = ci
	}

	msg := &waE2E.Message{InteractiveMessage: interactive}

	nodes := bizNativeFlowNodes()
	extra := noise.SendRequestExtra{AdditionalNodes: &nodes}
	if id != "" {
		extra.ID = types.MessageID(id)
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	a.recordOutgoing(client, txtID, recipient.String(), string(resp.ID), "buttons", payload.Body, msg, resp.Timestamp)
	return domain.MessageSendResult{ID: string(resp.ID), Timestamp: resp.Timestamp}, nil
}

// buildCarouselCard monta UM cartão, que é um InteractiveMessage completo.
func (a *ChatMessengerAdapter) buildCarouselCard(ctx context.Context, client client.Client, card domain.CarouselCard) (*waE2E.InteractiveMessage, error) {
	header := &waE2E.InteractiveMessage_Header{}
	if card.Title != "" { // decorative: iOS ignores Header.Title (F217)
		header.Title = proto.String(card.Title)
	}
	if len(card.Image) > 0 {
		uploaded, err := client.Upload(ctx, card.Image, noise.MediaImage)
		if err != nil {
			return nil, err
		}
		header.HasMediaAttachment = proto.Bool(true)
		header.Media = &waE2E.InteractiveMessage_Header_ImageMessage{
			ImageMessage: &waE2E.ImageMessage{
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(card.ImageMimeType),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(card.Image))),
			},
		}
	}

	out := &waE2E.InteractiveMessage{
		Header: header,
		Body:   &waE2E.InteractiveMessage_Body{Text: proto.String(card.Body)},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons:        nativeFlowButtons(card.Buttons),
				MessageVersion: proto.Int32(nativeFlowMessageVersion),
			},
		},
	}
	if card.Footer != "" {
		out.Footer = &waE2E.InteractiveMessage_Footer{Text: proto.String(card.Footer)}
	}
	return out, nil
}
