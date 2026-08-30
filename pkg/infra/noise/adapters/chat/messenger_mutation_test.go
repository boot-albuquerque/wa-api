package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"

	"wa-api/pkg/domain"

	"wa-api/internal/noise"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
)

// Este arquivo cobre RevokeMessage e EditMessage (CAP-10) no nível em que os
// argumentos do wire ficam visíveis. É o único nível em que dá para provar
// que BuildRevoke recebe types.EmptyJID como sender e que BuildEdit recebe o
// Id da mensagem ALVO — acima daqui os dois são strings/JIDs que trocados de
// lugar compilariam e passariam.
//
// O fake NÃO monta um protobuf próprio: testkit.Fake.BuildRevoke/BuildEdit
// delegam para os construtores REAIS
// (internal/noise/capabilities/message/builders.go), que é para onde
// (*core.Client).BuildRevoke/BuildEdit também delegam. ARMADILHA 1: um dublê
// mais permissivo que a produção esconderia justamente a troca de argumento
// que este arquivo existe para pegar.

const mutationChatJID = "5511999999999@s.whatsapp.net"

func mutationGetter(f *testkit.Fake) client.Getter {
	return testkit.GetterWith(map[string]client.Client{"u1": f})
}

// TestChatMessengerAdapter_RevokeMessage_SenderIsEmptyJID é o teste do
// argumento que um engano silencioso trocaria: o sender passado a
// BuildRevoke tem de ser types.EmptyJID (mensagem PRÓPRIA). Qualquer outro
// JID muda a operação para "revogar mensagem de terceiro como admin", que
// esta API nunca expôs.
func TestChatMessengerAdapter_RevokeMessage_SenderIsEmptyJID(t *testing.T) {
	var gotChat, gotSender types.JID
	var gotID types.MessageID
	f := &testkit.Fake{
		BuildRevokeFn: func(chat, sender types.JID, id types.MessageID) *waE2E.Message {
			gotChat, gotSender, gotID = chat, sender, id
			return &waE2E.Message{}
		},
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{ID: "revoke-wire-id", Timestamp: time.Unix(1755500110, 0)}, nil
		},
	}
	a := NewChatMessengerAdapter(mutationGetter(f))

	res, err := a.RevokeMessage(context.Background(), "u1", domain.JID(mutationChatJID), "3EB0ABC123")
	if err != nil {
		t.Fatalf("RevokeMessage: %v", err)
	}
	if !gotSender.IsEmpty() {
		t.Errorf("sender: got %q, want types.EmptyJID — sender nao vazio revoga mensagem de TERCEIRO", gotSender)
	}
	if gotChat.String() != mutationChatJID {
		t.Errorf("chat: got %q, want %q", gotChat, mutationChatJID)
	}
	if gotID != types.MessageID("3EB0ABC123") {
		t.Errorf("id: got %q, want %q", gotID, "3EB0ABC123")
	}
	if res.ID != "revoke-wire-id" {
		t.Errorf("ID: got %q, want %q (o que o SDK devolveu)", res.ID, "revoke-wire-id")
	}
	if res.Timestamp.Unix() != 1755500110 {
		t.Errorf("Timestamp: got %d, want %d", res.Timestamp.Unix(), 1755500110)
	}
}

// TestChatMessengerAdapter_RevokeMessage_WireShape prova, com os
// construtores REAIS (nenhum BuildRevokeFn instalado), que a mensagem que
// chega a SendMessage é um ProtocolMessage REVOKE cuja MessageKey tem
// FromMe=true, sem Participant, e o ID da mensagem alvo. É esta MessageKey
// que o servidor usa para decidir O QUE apagar.
func TestChatMessengerAdapter_RevokeMessage_WireShape(t *testing.T) {
	var sent *waE2E.Message
	f := &testkit.Fake{
		SendMessageFn: func(_ context.Context, _ types.JID, msg *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			sent = msg
			return noise.SendResponse{}, nil
		},
	}
	a := NewChatMessengerAdapter(mutationGetter(f))

	if _, err := a.RevokeMessage(context.Background(), "u1", domain.JID(mutationChatJID), "3EB0ABC123"); err != nil {
		t.Fatalf("RevokeMessage: %v", err)
	}
	if sent.GetProtocolMessage() == nil {
		t.Fatalf("mensagem enviada nao e' um ProtocolMessage: %v", sent)
	}
	pm := sent.GetProtocolMessage()
	if pm.GetType() != waE2E.ProtocolMessage_REVOKE {
		t.Errorf("Type: got %v, want REVOKE", pm.GetType())
	}
	key := pm.GetKey()
	if !key.GetFromMe() {
		t.Errorf("MessageKey.FromMe=false: a revogacao deixou de ser de mensagem propria")
	}
	if key.GetParticipant() != "" {
		t.Errorf("MessageKey.Participant=%q: revogacao de mensagem propria nao carrega participant", key.GetParticipant())
	}
	if key.GetID() != "3EB0ABC123" {
		t.Errorf("MessageKey.ID: got %q, want %q", key.GetID(), "3EB0ABC123")
	}
	if key.GetRemoteJID() != mutationChatJID {
		t.Errorf("MessageKey.RemoteJID: got %q, want %q", key.GetRemoteJID(), mutationChatJID)
	}
}

// TestChatMessengerAdapter_EditMessage_WireShape prova que BuildEdit recebe
// o ID da mensagem ALVO (não o texto novo) e que o conteúdo novo é o texto
// do payload — os dois são strings, e invertê-los compila.
func TestChatMessengerAdapter_EditMessage_WireShape(t *testing.T) {
	var sent *waE2E.Message
	f := &testkit.Fake{
		SendMessageFn: func(_ context.Context, _ types.JID, msg *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			sent = msg
			return noise.SendResponse{ID: "edit-wire-id", Timestamp: time.Unix(1755500111, 0)}, nil
		},
	}
	a := NewChatMessengerAdapter(mutationGetter(f))

	res, err := a.EditMessage(context.Background(), "u1", domain.JID(mutationChatJID), "3EB0ABC123", "texto corrigido", nil)
	if err != nil {
		t.Fatalf("EditMessage: %v", err)
	}

	inner := sent.GetEditedMessage().GetMessage().GetProtocolMessage()
	if inner == nil {
		t.Fatalf("mensagem enviada nao e' um EditedMessage/ProtocolMessage: %v", sent)
	}
	if inner.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT {
		t.Errorf("Type: got %v, want MESSAGE_EDIT", inner.GetType())
	}
	if got := inner.GetKey().GetID(); got != "3EB0ABC123" {
		t.Errorf("MessageKey.ID: got %q, want %q (o Id da mensagem alvo, nao o texto novo)", got, "3EB0ABC123")
	}
	if got := inner.GetKey().GetRemoteJID(); got != mutationChatJID {
		t.Errorf("MessageKey.RemoteJID: got %q, want %q", got, mutationChatJID)
	}
	if got := inner.GetEditedMessage().GetExtendedTextMessage().GetText(); got != "texto corrigido" {
		t.Errorf("texto novo: got %q, want %q", got, "texto corrigido")
	}
	if res.ID != "edit-wire-id" {
		t.Errorf("ID: got %q, want %q", res.ID, "edit-wire-id")
	}
	if res.Timestamp.Unix() != 1755500111 {
		t.Errorf("Timestamp: got %d, want %d", res.Timestamp.Unix(), 1755500111)
	}
}

// TestChatMessengerAdapter_EditMessage_ContextInfoWireShape (F134) proves
// that StanzaID/Participant/MentionedJID in EditContextInfo reach
// ExtendedTextMessage.ContextInfo in the protobuf.
func TestChatMessengerAdapter_EditMessage_ContextInfoWireShape(t *testing.T) {
	var sent *waE2E.Message
	f := &testkit.Fake{
		SendMessageFn: func(_ context.Context, _ types.JID, msg *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
			sent = msg
			return noise.SendResponse{ID: "edit-ctx-id", Timestamp: time.Unix(1755500112, 0)}, nil
		},
	}
	a := NewChatMessengerAdapter(mutationGetter(f))

	ctxInfo := &domain.EditContextInfo{
		StanzaID:     "STANZA-99",
		Participant:  "5511888888888@s.whatsapp.net",
		MentionedJID: []string{"5511777777777@s.whatsapp.net"},
	}
	_, err := a.EditMessage(context.Background(), "u1", domain.JID(mutationChatJID), "3EB0ABC123", "com citacao", ctxInfo)
	if err != nil {
		t.Fatalf("EditMessage: %v", err)
	}

	inner := sent.GetEditedMessage().GetMessage().GetProtocolMessage()
	if inner == nil {
		t.Fatalf("mensagem enviada nao e' um EditedMessage/ProtocolMessage: %v", sent)
	}
	ext := inner.GetEditedMessage().GetExtendedTextMessage()
	if ext == nil {
		t.Fatal("ExtendedTextMessage e' nil")
	}
	ci := ext.GetContextInfo()
	if ci == nil {
		t.Fatal("ContextInfo e' nil — F134 nao foi aplicado no wire")
	}
	if ci.GetStanzaID() != "STANZA-99" {
		t.Errorf("StanzaID: got %q, want %q", ci.GetStanzaID(), "STANZA-99")
	}
	if ci.GetParticipant() != "5511888888888@s.whatsapp.net" {
		t.Errorf("Participant: got %q, want %q", ci.GetParticipant(), "5511888888888@s.whatsapp.net")
	}
	if len(ci.GetMentionedJID()) != 1 || ci.GetMentionedJID()[0] != "5511777777777@s.whatsapp.net" {
		t.Errorf("MentionedJID: got %v, want [5511777777777@s.whatsapp.net]", ci.GetMentionedJID())
	}
}

// TestChatMessengerAdapter_Mutation_SendFailurePropagates: falha do envio
// não vira resultado válido em NENHUMA das duas — é o defeito que o CAP-10
// consertou, travado também na camada mais baixa.
func TestChatMessengerAdapter_Mutation_SendFailurePropagates(t *testing.T) {
	sentinel := errors.New("mutation-adapter-sentinel")
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...noise.SendRequestExtra) (noise.SendResponse, error) {
			return noise.SendResponse{}, sentinel
		},
	}
	a := NewChatMessengerAdapter(mutationGetter(f))

	t.Run("RevokeMessage", func(t *testing.T) {
		res, err := a.RevokeMessage(context.Background(), "u1", domain.JID(mutationChatJID), "3EB0ABC123")
		if !errors.Is(err, sentinel) {
			t.Fatalf("erro do envio nao propagou: got %#v", err)
		}
		if res != (domain.MessageSendResult{}) {
			t.Errorf("envio falhou mas o resultado nao e' zero: %#v", res)
		}
	})
	t.Run("EditMessage", func(t *testing.T) {
		res, err := a.EditMessage(context.Background(), "u1", domain.JID(mutationChatJID), "3EB0ABC123", "corrigido", nil)
		if !errors.Is(err, sentinel) {
			t.Fatalf("erro do envio nao propagou: got %#v", err)
		}
		if res != (domain.MessageSendResult{}) {
			t.Errorf("envio falhou mas o resultado nao e' zero: %#v", res)
		}
	})
}

// TestChatMessengerAdapter_Mutation_NoSession: sem sessão, nenhuma das duas
// chega a montar mensagem nenhuma.
func TestChatMessengerAdapter_Mutation_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(nil))

	if _, err := a.RevokeMessage(context.Background(), "desconhecido", domain.JID(mutationChatJID), "3EB0ABC123"); err == nil {
		t.Error("RevokeMessage sem sessao devolveu nil")
	}
	if _, err := a.EditMessage(context.Background(), "desconhecido", domain.JID(mutationChatJID), "3EB0ABC123", "x", nil); err == nil {
		t.Error("EditMessage sem sessao devolveu nil")
	}
}

// TestChatMessengerAdapter_Mutation_InvalidJID: alvo que não parseia nunca
// alcança SendMessage.
func TestChatMessengerAdapter_Mutation_InvalidJID(t *testing.T) {
	calls := 0
	f := &testkit.Fake{
		SendMessageFn: func(context.Context, types.JID, *waE2E.Message, ...noise.SendRequestExtra) (noise.SendResponse, error) {
			calls++
			return noise.SendResponse{}, nil
		},
	}
	a := NewChatMessengerAdapter(mutationGetter(f))

	const invalid = domain.JID("@@")
	if _, err := a.RevokeMessage(context.Background(), "u1", invalid, "3EB0ABC123"); err == nil {
		t.Error("RevokeMessage com JID invalido devolveu nil")
	}
	if _, err := a.EditMessage(context.Background(), "u1", invalid, "3EB0ABC123", "x", nil); err == nil {
		t.Error("EditMessage com JID invalido devolveu nil")
	}
	if calls != 0 {
		t.Errorf("JID invalido alcancou SendMessage %d vez(es)", calls)
	}
}

// TestChatMessengerAdapter_Mutation_UnknownIDIsNotValidated documenta o que
// a primitiva FAZ com um Id inexistente ou sintaticamente lixo: nada. Nem
// BuildRevoke nem BuildEdit consultam armazenamento algum
// (internal/noise/capabilities/message/builders.go:39 e :101 só montam
// protobuf a partir da string recebida), e types.MessageID é um alias de
// string sem validação. O resultado é uma mensagem BEM FORMADA para um alvo
// que não existe, que o envio aceita e a rota devolve como sucesso.
//
// Este teste não aprova esse comportamento — ele o TRAVA como conhecido,
// para que ninguém o descubra de novo em produção. Ver HOUSEKEEP F135.
func TestChatMessengerAdapter_Mutation_UnknownIDIsNotValidated(t *testing.T) {
	ids := []string{"", "id-que-nao-existe", "not a message id at all", "../../etc/passwd"}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			var sent *waE2E.Message
			f := &testkit.Fake{
				SendMessageFn: func(_ context.Context, _ types.JID, msg *waE2E.Message, _ ...noise.SendRequestExtra) (noise.SendResponse, error) {
					sent = msg
					return noise.SendResponse{}, nil
				},
			}
			a := NewChatMessengerAdapter(mutationGetter(f))

			if _, err := a.RevokeMessage(context.Background(), "u1", domain.JID(mutationChatJID), id); err != nil {
				t.Fatalf("RevokeMessage recusou o id %q: %v — se passou a validar, atualize a F135", id, err)
			}
			if got := sent.GetProtocolMessage().GetKey().GetID(); got != id {
				t.Errorf("MessageKey.ID: got %q, want %q — o id vai cru para o wire", got, id)
			}
		})
	}
}
