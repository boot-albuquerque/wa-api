package msgattrs

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

func TestGetTypeFromMessageClassifiesEachFamily(t *testing.T) {
	for name, tc := range map[string]struct {
		msg  *waE2E.Message
		want string
	}{
		"conversa":  {&waE2E.Message{Conversation: proto.String("oi")}, "text"},
		"reacao":    {&waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{}}, "reaction"},
		"enquete":   {&waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{}}, "poll"},
		"midia":     {&waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, "media"},
		"vazia":     {&waE2E.Message{}, "text"},
		"protocolo": {&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}, "text"},
	} {
		if got := GetTypeFromMessage(tc.msg); got != tc.want {
			t.Errorf("%s: = %q, esperado %q", name, got, tc.want)
		}
	}
}

// Os wrappers (view-once, efemera, ...) tem que ser atravessados: o tipo e' o
// da mensagem de dentro, nao o do envelope.
func TestGetTypeFromMessageUnwrapsContainers(t *testing.T) {
	inner := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}
	for name, msg := range map[string]*waE2E.Message{
		"view once":      {ViewOnceMessage: &waE2E.FutureProofMessage{Message: inner}},
		"view once v2":   {ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: inner}},
		"efemera":        {EphemeralMessage: &waE2E.FutureProofMessage{Message: inner}},
		"doc c/ legenda": {DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: inner}},
	} {
		if got := GetTypeFromMessage(msg); got != "media" {
			t.Errorf("%s: = %q, esperado \"media\"", name, got)
		}
		if got := GetMediaTypeFromMessage(msg); got != "image" {
			t.Errorf("%s: media type = %q, esperado \"image\"", name, got)
		}
	}
}

func TestGetMediaTypeFromMessageDistinguishesVoiceAndGif(t *testing.T) {
	ptt := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{PTT: proto.Bool(true)}}
	if got := GetMediaTypeFromMessage(ptt); got != "ptt" {
		t.Errorf("audio PTT = %q, esperado \"ptt\"", got)
	}
	audio := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}
	if got := GetMediaTypeFromMessage(audio); got != "audio" {
		t.Errorf("audio = %q, esperado \"audio\"", got)
	}
	gif := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{GifPlayback: proto.Bool(true)}}
	if got := GetMediaTypeFromMessage(gif); got != "gif" {
		t.Errorf("video GIF = %q, esperado \"gif\"", got)
	}
	video := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}
	if got := GetMediaTypeFromMessage(video); got != "video" {
		t.Errorf("video = %q, esperado \"video\"", got)
	}
}

func TestGetMediaTypeFromMessageIsEmptyForPlainText(t *testing.T) {
	if got := GetMediaTypeFromMessage(&waE2E.Message{Conversation: proto.String("oi")}); got != "" {
		t.Errorf("= %q, esperado vazio", got)
	}
}

func TestGetButtonTypeAndAttributesForListMessages(t *testing.T) {
	msg := &waE2E.Message{ListMessage: &waE2E.ListMessage{
		ListType: waE2E.ListMessage_SINGLE_SELECT.Enum(),
	}}
	if got := GetButtonTypeFromMessage(msg); got != "list" {
		t.Errorf("button type = %q, esperado \"list\"", got)
	}
	attrs := GetButtonAttributes(msg)
	if attrs["v"] != "2" {
		t.Errorf("v = %v, esperado \"2\"", attrs["v"])
	}
	if attrs["type"] != "single_select" {
		t.Errorf("type = %v, esperado \"single_select\"", attrs["type"])
	}
}

func TestGetButtonAttributesIsEmptyWhenThereAreNoButtons(t *testing.T) {
	if got := GetButtonAttributes(&waE2E.Message{Conversation: proto.String("oi")}); len(got) != 0 {
		t.Errorf("= %v, esperado vazio", got)
	}
	if got := GetButtonTypeFromMessage(&waE2E.Message{Conversation: proto.String("oi")}); got != "" {
		t.Errorf("= %q, esperado vazio", got)
	}
}

// O atributo "edit" decide se o servidor trata o node como revogacao, edicao
// ou fixacao; trocar um pelo outro apaga a mensagem errada.
func TestGetEditAttributeMapsProtocolMessages(t *testing.T) {
	revoke := func(fromMe bool) *waE2E.Message {
		return &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key:  &waCommon.MessageKey{FromMe: proto.Bool(fromMe)},
		}}
	}
	edit := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:           &waCommon.MessageKey{FromMe: proto.Bool(true)},
		EditedMessage: &waE2E.Message{Conversation: proto.String("corrigido")},
	}}

	for name, tc := range map[string]struct {
		msg  *waE2E.Message
		want types.EditAttribute
	}{
		"revogacao propria":  {revoke(true), types.EditAttributeSenderRevoke},
		"revogacao de admin": {revoke(false), types.EditAttributeAdminRevoke},
		"edicao de mensagem": {edit, types.EditAttributeMessageEdit},
		"remocao de reacao": {
			&waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: proto.String(RemoveReactionText)}},
			types.EditAttributeSenderRevoke,
		},
		"reacao normal": {
			&waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: proto.String("👍")}},
			types.EditAttributeEmpty,
		},
		"pin in chat": {
			&waE2E.Message{PinInChatMessage: &waE2E.PinInChatMessage{}},
			types.EditAttributePinInChat,
		},
		"sem edicao": {
			&waE2E.Message{Conversation: proto.String("oi")},
			types.EditAttributeEmpty,
		},
	} {
		if got := GetEditAttribute(tc.msg); got != tc.want {
			t.Errorf("%s: = %q, esperado %q", name, got, tc.want)
		}
	}
}
