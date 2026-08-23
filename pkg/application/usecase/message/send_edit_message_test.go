package message_test

import (
	"context"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

func strPtr(s string) *string { return &s }

// TestSendEditMessage_ContextInfoFlowsToPort (F134) proves that
// StanzaID, Participant and MentionedJID from SendEditMessageRequest
// reach the ChatMessenger.EditMessage port as an EditContextInfo.
// The historical handler wired these into ExtendedTextMessage.ContextInfo;
// the reconstructed route dropped them.
func TestSendEditMessage_ContextInfoFlowsToPort(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendEditMessageUseCase(cm, jr, logger).
		Execute(context.Background(), userID, domain.SendEditMessageRequest{
			Phone:        "5511987654321",
			Body:         "texto corrigido",
			ID:           "3EB0ABC123",
			StanzaID:     strPtr("STANZA-42"),
			Participant:  strPtr("5511888888888@s.whatsapp.net"),
			MentionedJID: []string{"5511777777777@s.whatsapp.net"},
		})

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n := len(cm.EditMessageCalls); n != 1 {
		t.Fatalf("EditMessage chamado %d vez(es), quero 1", n)
	}
	call := cm.EditMessageCalls[0]
	if call.CtxInfo == nil {
		t.Fatal("CtxInfo e' nil — StanzaID/Participant/MentionedJID nao chegaram a' porta")
	}
	if call.CtxInfo.StanzaID != "STANZA-42" {
		t.Errorf("StanzaID: got %q, want %q", call.CtxInfo.StanzaID, "STANZA-42")
	}
	if call.CtxInfo.Participant != "5511888888888@s.whatsapp.net" {
		t.Errorf("Participant: got %q, want %q", call.CtxInfo.Participant, "5511888888888@s.whatsapp.net")
	}
	if len(call.CtxInfo.MentionedJID) != 1 || call.CtxInfo.MentionedJID[0] != "5511777777777@s.whatsapp.net" {
		t.Errorf("MentionedJID: got %v, want [5511777777777@s.whatsapp.net]", call.CtxInfo.MentionedJID)
	}
}

// TestSendEditMessage_NoContextInfoWhenFieldsAbsent verifies that when
// none of StanzaID/Participant/MentionedJID are set, ctxInfo stays nil.
func TestSendEditMessage_NoContextInfoWhenFieldsAbsent(t *testing.T) {
	cm := &contractsfake.ChatMessenger{}
	jr := &contractsfake.JIDResolver{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendEditMessageUseCase(cm, jr, logger).
		Execute(context.Background(), userID, domain.SendEditMessageRequest{
			Phone: "5511987654321",
			Body:  "so texto",
			ID:    "3EB0ABC456",
		})

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n := len(cm.EditMessageCalls); n != 1 {
		t.Fatalf("EditMessage chamado %d vez(es), quero 1", n)
	}
	if cm.EditMessageCalls[0].CtxInfo != nil {
		t.Errorf("CtxInfo devia ser nil quando nenhum campo de contexto e' preenchido, got %+v", cm.EditMessageCalls[0].CtxInfo)
	}
}
