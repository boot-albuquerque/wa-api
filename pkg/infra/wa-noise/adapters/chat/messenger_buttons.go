package chat

import (
	"context"
	"encoding/json"

	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"

	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"

	"google.golang.org/protobuf/proto"
)

// Os `Name` dos NativeFlowButton, um por tipo público de domain (CAP-21).
//
// O mapa NÃO é a identidade, e é aqui que isso fica visível:
// domain.ButtonTypeCopy ("copy") vira nativeFlowNameCTACopy ("cta_copy"). Os
// outros três coincidem por acaso do protocolo, não por regra — escrevê-los
// como constantes próprias em vez de reusar as de domínio é o que impede
// alguém de "simplificar" os quatro para uma passagem direta e quebrar
// justamente o `copy` (`git show 41bc8e2^:handlers.go`, linhas 2090-2104).
const (
	nativeFlowNameQuickReply = "quick_reply"
	nativeFlowNameCTAURL     = "cta_url"
	nativeFlowNameCTACall    = "cta_call"
	nativeFlowNameCTACopy    = "cta_copy"
)

// As chaves do JSON de ButtonParamsJSON. São vocabulário de WIRE — vão
// dentro de uma STRING, não como campos de protobuf, então nenhum
// compilador as protege e um erro de digitação só apareceria no aparelho de
// quem recebe. Constantes nomeadas, portanto (ADR-0004).
const (
	buttonParamDisplayText = "display_text"
	buttonParamID          = "id"
	buttonParamURL         = "url"
	buttonParamMerchantURL = "merchant_url"
	buttonParamPhoneNumber = "phone_number"
	buttonParamCopyCode    = "copy_code"
)

// nativeFlowMessageVersion e nativeFlowNodeVersion são as versões que o
// histórico escrevia: `MessageVersion: proto.Int32(1)` no protobuf e
// `"v": "9"` no nó `native_flow` do stanza (`git show 41bc8e2^:handlers.go`,
// linhas 2196 e 2224). Nunca foram expostas no payload público.
const (
	nativeFlowMessageVersion = int32(1)
	nativeFlowNodeVersion    = "9"
)

// Os literais do nó BIZ, que é o que faz o aparelho RENDERIZAR os botões em
// vez de mostrar só o corpo da mensagem (`git show 41bc8e2^:handlers.go`,
// linhas 2216-2227). É metadata de stanza, não de mensagem: vive aqui.
const (
	bizNodeTag              = "biz"
	bizInteractiveNodeTag   = "interactive"
	bizNativeFlowNodeTag    = "native_flow"
	bizNodeAttrType         = "type"
	bizNodeAttrVersion      = "v"
	bizNodeAttrName         = "name"
	bizInteractiveTypeValue = "native_flow"
	bizInteractiveVersion   = "1"
	bizNativeFlowNameValue  = "mixed"
)

// nativeFlowButtons traduz os botões de domínio JÁ NORMALIZADOS para os
// NativeFlowButton do wire, preservando a ORDEM (é a ordem em que aparecem
// no aparelho de quem recebe).
//
// Nenhum fallback é reaplicado aqui: Title e ID chegam resolvidos do use
// case, porque a cadeia deles depende do truncamento do título e inverter
// essa ordem mudaria o id que volta no clique (ver
// normalizeInteractiveButtons).
//
// O ramo `default` é inalcançável pelo contrato da porta — o use case só
// entrega os quatro tipos de domínio — e existe como recusa defensiva: um
// tipo que escapasse não pode virar um botão com `Name` vazio no aparelho de
// quem recebe.
func nativeFlowButtons(buttons []domain.InteractiveButton) []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton {
	out := make([]*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton, 0, len(buttons))

	for _, item := range buttons {
		var name string
		var params map[string]string

		switch item.Type {
		case domain.ButtonTypeReply:
			name = nativeFlowNameQuickReply
			params = map[string]string{
				buttonParamDisplayText: item.Title,
				buttonParamID:          item.ID,
			}
		case domain.ButtonTypeCTAURL:
			name = nativeFlowNameCTAURL
			// A MESMA URL vai nos DOIS campos, como no histórico: o
			// aparelho usa `url` para abrir e `merchant_url` para o
			// rótulo de comerciante. Preencher só um deixa o botão sem
			// destino em parte dos clientes.
			params = map[string]string{
				buttonParamDisplayText: item.Title,
				buttonParamURL:         item.URL,
				buttonParamMerchantURL: item.URL,
			}
		case domain.ButtonTypeCTACall:
			name = nativeFlowNameCTACall
			params = map[string]string{
				buttonParamDisplayText: item.Title,
				buttonParamPhoneNumber: item.PhoneNumber,
			}
		case domain.ButtonTypeCopy:
			name = nativeFlowNameCTACopy
			params = map[string]string{
				buttonParamDisplayText: item.Title,
				buttonParamCopyCode:    item.CopyCode,
			}
		default:
			continue
		}

		// O erro do Marshal é descartado como no histórico, e aqui isso é
		// seguro por construção: map[string]string sempre serializa.
		paramsJSON, _ := json.Marshal(params)
		out = append(out, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             proto.String(name),
			ButtonParamsJSON: proto.String(string(paramsJSON)),
		})
	}

	return out
}

// bizNativeFlowNodes monta o nó BIZ que acompanha o stanza. Sem ele o
// aparelho recebe a mensagem mas NÃO desenha os botões — o histórico o
// chamava de "fundamental para renderizar os botões".
func bizNativeFlowNodes() []waBinary.Node {
	return []waBinary.Node{{
		Tag: bizNodeTag,
		Content: []waBinary.Node{{
			Tag: bizInteractiveNodeTag,
			Attrs: waBinary.Attrs{
				bizNodeAttrType:    bizInteractiveTypeValue,
				bizNodeAttrVersion: bizInteractiveVersion,
			},
			Content: []waBinary.Node{{
				Tag: bizNativeFlowNodeTag,
				Attrs: waBinary.Attrs{
					bizNodeAttrVersion: nativeFlowNodeVersion,
					bizNodeAttrName:    bizNativeFlowNameValue,
				},
			}},
		}},
	}}
}

// SendButtons monta um InteractiveMessage com NativeFlowMessage a partir de
// payload e o envia para target (CAP-21).
//
// Quando payload.HeaderImage traz bytes, o upload acontece ANTES do envio e
// o header carrega a imagem; se SendMessage falhar depois de um upload
// bem-sucedido, o erro é propagado sem tentativa de desfazer o upload — o
// protocolo não oferece essa operação (mesma disciplina de SendImage).
//
// A PRIORIDADE do header é a do histórico: imagem primeiro; só na ausência
// dela o Title vira texto de header. Um header com os dois nunca existiu.
func (a *ChatMessengerAdapter) SendButtons(ctx context.Context, txtID string, target domain.JID, payload domain.ButtonsPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	header := &waE2E.InteractiveMessage_Header{}
	if len(payload.HeaderImage) > 0 {
		uploaded, uploadErr := client.Upload(ctx, payload.HeaderImage, wanoise.MediaImage)
		if uploadErr != nil {
			return domain.MessageSendResult{}, uploadErr
		}
		header.HasMediaAttachment = proto.Bool(true)
		header.Media = &waE2E.InteractiveMessage_Header_ImageMessage{
			ImageMessage: &waE2E.ImageMessage{
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(payload.HeaderImageMimeType),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(payload.HeaderImage))),
			},
		}
	} else if payload.Title != "" {
		header.Title = proto.String(payload.Title)
	}

	interactive := &waE2E.InteractiveMessage{
		Header: header,
		Body:   &waE2E.InteractiveMessage_Body{Text: proto.String(payload.Body)},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons:        nativeFlowButtons(payload.Buttons),
				MessageVersion: proto.Int32(nativeFlowMessageVersion),
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
	extra := wanoise.SendRequestExtra{AdditionalNodes: &nodes}
	if id != "" {
		extra.ID = types.MessageID(id)
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}
