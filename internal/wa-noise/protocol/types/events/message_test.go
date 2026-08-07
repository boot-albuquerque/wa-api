// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package events

import (
	"testing"

	"google.golang.org/protobuf/proto"

	armadillo "wa-api/internal/wa-noise/protocol/proto"
	"wa-api/internal/wa-noise/protocol/proto/waArmadilloApplication"
	"wa-api/internal/wa-noise/protocol/proto/waConsumerApplication"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
)

// textMessage devolve uma mensagem de texto com o conteudo dado. E' o "fundo"
// de todos os embrulhos: UnwrapRaw tem que chegar nele.
func textMessage(text string) *waE2E.Message {
	return &waE2E.Message{Conversation: proto.String(text)}
}

func unwrap(msg *waE2E.Message) *Message {
	return (&Message{RawMessage: msg}).UnwrapRaw()
}

// UnwrapRaw desembrulha uma mensagem camada por camada e marca uma flag por
// camada removida. Cada flag alimenta uma decisao diferente do wa-api (view
// once nao pode ser reencaminhada, edicao substitui a original), entao
// desembrulhar sem marcar e' pior que nao desembrulhar.
func TestUnwrapRawUnwrapsEachLayerAndSetsItsFlag(t *testing.T) {
	tests := []struct {
		name    string
		raw     *waE2E.Message
		checkOn func(*Message) bool
	}{
		{
			"ephemeral",
			&waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: textMessage("oi")}},
			func(m *Message) bool { return m.IsEphemeral },
		},
		{
			"view once",
			&waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{Message: textMessage("oi")}},
			func(m *Message) bool { return m.IsViewOnce && !m.IsViewOnceV2 },
		},
		{
			"view once v2",
			&waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: textMessage("oi")}},
			func(m *Message) bool { return m.IsViewOnce && m.IsViewOnceV2 && !m.IsViewOnceV2Extension },
		},
		{
			"view once v2 extension",
			&waE2E.Message{ViewOnceMessageV2Extension: &waE2E.FutureProofMessage{Message: textMessage("oi")}},
			func(m *Message) bool { return m.IsViewOnce && m.IsViewOnceV2 && m.IsViewOnceV2Extension },
		},
		{
			"lottie sticker",
			&waE2E.Message{LottieStickerMessage: &waE2E.FutureProofMessage{Message: textMessage("oi")}},
			func(m *Message) bool { return m.IsLottieSticker },
		},
		{
			"documento com legenda",
			&waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: textMessage("oi")}},
			func(m *Message) bool { return m.IsDocumentWithCaption },
		},
		{
			"edicao",
			&waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{Message: textMessage("oi")}},
			func(m *Message) bool { return m.IsEdit },
		},
		{
			"bot invoke",
			&waE2E.Message{BotInvokeMessage: &waE2E.FutureProofMessage{Message: textMessage("oi")}},
			func(m *Message) bool { return m.IsBotInvoke },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unwrap(tc.raw)
			if got.Message.GetConversation() != "oi" {
				t.Errorf("nao chegou ao texto: %+v", got.Message)
			}
			if !tc.checkOn(got) {
				t.Errorf("a flag da camada nao foi marcada: %+v", got)
			}
		})
	}
}

// Uma mensagem sem embrulho nenhum passa direto e nao marca flag nenhuma.
func TestUnwrapRawLeavesAPlainMessageAlone(t *testing.T) {
	got := unwrap(textMessage("oi"))
	if got.Message.GetConversation() != "oi" {
		t.Errorf("= %+v", got.Message)
	}
	for name, flag := range map[string]bool{
		"IsEphemeral": got.IsEphemeral, "IsViewOnce": got.IsViewOnce,
		"IsViewOnceV2": got.IsViewOnceV2, "IsViewOnceV2Extension": got.IsViewOnceV2Extension,
		"IsDocumentWithCaption": got.IsDocumentWithCaption, "IsLottieSticker": got.IsLottieSticker,
		"IsBotInvoke": got.IsBotInvoke, "IsEdit": got.IsEdit,
	} {
		if flag {
			t.Errorf("%s foi marcada numa mensagem sem embrulho", name)
		}
	}
}

// As camadas se aninham, e UnwrapRaw as remove NA ORDEM em que aparecem no
// codigo. Uma mensagem view-once dentro de uma efemera tem que sair com as
// duas flags — perder uma faria o wa-api tratar uma midia de visualizacao
// unica como midia comum.
func TestUnwrapRawRemovesNestedLayers(t *testing.T) {
	raw := &waE2E.Message{
		EphemeralMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: textMessage("segredo")},
			},
		},
	}
	got := unwrap(raw)
	if got.Message.GetConversation() != "segredo" {
		t.Fatalf("nao chegou ao texto: %+v", got.Message)
	}
	if !got.IsEphemeral {
		t.Error("IsEphemeral nao foi marcada")
	}
	if !got.IsViewOnce || !got.IsViewOnceV2 {
		t.Error("as flags de view once nao foram marcadas")
	}
}

// DeviceSentMessage e' a primeira camada e a unica que produz METADADOS em vez
// de flag: o destino real da mensagem que eu mandei de outro dispositivo. Sem
// ele, o eco apareceria como mensagem para mim mesmo.
func TestUnwrapRawExtractsDeviceSentMetadata(t *testing.T) {
	raw := &waE2E.Message{
		DeviceSentMessage: &waE2E.DeviceSentMessage{
			DestinationJID: proto.String("5511999999999@s.whatsapp.net"),
			Phash:          proto.String("2:abc"),
			Message:        textMessage("oi"),
		},
	}
	got := unwrap(raw)
	if got.Message.GetConversation() != "oi" {
		t.Fatalf("nao desembrulhou: %+v", got.Message)
	}
	if got.Info.DeviceSentMeta == nil {
		t.Fatal("DeviceSentMeta nao foi preenchido")
	}
	if got.Info.DeviceSentMeta.DestinationJID != "5511999999999@s.whatsapp.net" {
		t.Errorf("DestinationJID = %q", got.Info.DeviceSentMeta.DestinationJID)
	}
	if got.Info.DeviceSentMeta.Phash != "2:abc" {
		t.Errorf("Phash = %q", got.Info.DeviceSentMeta.Phash)
	}
}

// Um device-sent que embrulha uma view-once tem que produzir os metadados E a
// flag: sao camadas independentes e o codigo as processa em sequencia.
func TestUnwrapRawCombinesDeviceSentWithOtherLayers(t *testing.T) {
	raw := &waE2E.Message{
		DeviceSentMessage: &waE2E.DeviceSentMessage{
			DestinationJID: proto.String("123@s.whatsapp.net"),
			Message: &waE2E.Message{
				ViewOnceMessage: &waE2E.FutureProofMessage{Message: textMessage("oi")},
			},
		},
	}
	got := unwrap(raw)
	if got.Info.DeviceSentMeta == nil {
		t.Fatal("DeviceSentMeta nao foi preenchido")
	}
	if !got.IsViewOnce {
		t.Error("IsViewOnce nao foi marcada")
	}
	if got.Message.GetConversation() != "oi" {
		t.Errorf("= %+v", got.Message)
	}
}

// O MessageContextInfo mora na camada de FORA, e desembrulhar o descartaria.
// UnwrapRaw o promove para a mensagem interna — e' o que carrega o
// DeviceListMetadata usado para decidir para quais dispositivos cifrar.
func TestUnwrapRawPromotesContextInfoFromTheOuterLayer(t *testing.T) {
	contextInfo := &waE2E.MessageContextInfo{MessageSecret: []byte("segredo")}
	raw := &waE2E.Message{
		MessageContextInfo: contextInfo,
		EphemeralMessage:   &waE2E.FutureProofMessage{Message: textMessage("oi")},
	}
	got := unwrap(raw)
	if got.Message.MessageContextInfo == nil {
		t.Fatal("o contexto da camada externa foi perdido")
	}
	if string(got.Message.MessageContextInfo.MessageSecret) != "segredo" {
		t.Errorf("= %v", got.Message.MessageContextInfo)
	}
}

// A promocao so' vale quando a mensagem interna NAO tem contexto proprio: o
// contexto de dentro e' mais especifico e nao pode ser sobrescrito.
func TestUnwrapRawKeepsTheInnerContextInfoWhenPresent(t *testing.T) {
	raw := &waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("de-fora")},
		EphemeralMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				Conversation:       proto.String("oi"),
				MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("de-dentro")},
			},
		},
	}
	got := unwrap(raw)
	if string(got.Message.MessageContextInfo.MessageSecret) != "de-dentro" {
		t.Errorf("o contexto interno foi sobrescrito: %v", got.Message.MessageContextInfo)
	}
}

// UnwrapRaw devolve o proprio evento (para encadear) e preserva RawMessage
// intacto: e' a copia crua que o wa-api guarda para retry e para o historico.
func TestUnwrapRawReturnsItselfAndPreservesTheRawMessage(t *testing.T) {
	raw := &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: textMessage("oi")}}
	evt := &Message{RawMessage: raw}

	got := evt.UnwrapRaw()
	if got != evt {
		t.Error("UnwrapRaw deveria devolver o proprio evento")
	}
	if evt.RawMessage != raw {
		t.Error("RawMessage foi alterado")
	}
	if evt.RawMessage.GetEphemeralMessage() == nil {
		t.Error("a camada foi removida do RawMessage, que deveria ficar cru")
	}
}

// Os getters tipados de FBMessage devolvem nil quando o payload e' de outro
// tipo, em vez de entrar em panic no type assertion.
func TestFBMessageTypedGetters(t *testing.T) {
	consumer := &waConsumerApplication.ConsumerApplication{}
	arma := &waArmadilloApplication.Armadillo{}

	withConsumer := &FBMessage{Message: consumer}
	if withConsumer.GetConsumerApplication() != consumer {
		t.Error("GetConsumerApplication nao devolveu o payload")
	}
	if withConsumer.GetArmadillo() != nil {
		t.Error("GetArmadillo deveria devolver nil para um payload de consumer")
	}

	withArmadillo := &FBMessage{Message: arma}
	if withArmadillo.GetArmadillo() != arma {
		t.Error("GetArmadillo nao devolveu o payload")
	}
	if withArmadillo.GetConsumerApplication() != nil {
		t.Error("GetConsumerApplication deveria devolver nil para um payload armadillo")
	}
}

func TestFBMessageGettersOnNilPayload(t *testing.T) {
	var empty FBMessage
	if empty.GetConsumerApplication() != nil || empty.GetArmadillo() != nil {
		t.Error("os getters de um payload nil deveriam devolver nil")
	}
	var _ armadillo.MessageApplicationSub = (*waConsumerApplication.ConsumerApplication)(nil)
}

// Os aliases descontinuados tem que continuar apontando para as constantes de
// types. Sao API publica: quebra-los quebra consumidores sem aviso de
// compilacao no lado deles ate' recompilarem.
func TestDeprecatedReceiptAliasesStillMatch(t *testing.T) {
	if ReceiptTypeDelivered != "" {
		t.Errorf("ReceiptTypeDelivered = %q", string(ReceiptTypeDelivered))
	}
	if ReceiptTypeRead != "read" || ReceiptTypeReadSelf != "read-self" {
		t.Errorf("aliases de leitura mudaram: %q / %q", string(ReceiptTypeRead), string(ReceiptTypeReadSelf))
	}
	if ReceiptTypeSender != "sender" || ReceiptTypeRetry != "retry" || ReceiptTypePlayed != "played" {
		t.Error("os demais aliases mudaram")
	}
}

// Os dois enums de string do arquivo usam a string VAZIA como valor padrao. E'
// o que distingue "atributo ausente no frame" de um valor de verdade.
func TestEmptyStringIsTheDefaultForBothEnums(t *testing.T) {
	if DecryptFailShow != "" {
		t.Errorf("DecryptFailShow = %q", string(DecryptFailShow))
	}
	if UnavailableTypeUnknown != "" {
		t.Errorf("UnavailableTypeUnknown = %q", string(UnavailableTypeUnknown))
	}
	if DecryptFailHide == DecryptFailShow || UnavailableTypeViewOnce == UnavailableTypeUnknown {
		t.Error("um enum colidiu com o proprio valor padrao")
	}
}
