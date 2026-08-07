// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/appstate"
	"wa-api/internal/wa-noise/proto/waServerSync"
	"wa-api/internal/wa-noise/proto/waSyncAction"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types/events"
	waLog "wa-api/internal/wa-noise/util/log"
)

// dispatchTestClient monta o minimo de Client que `dispatchAppState` precisa:
// logger no-op e um Store sem nenhum sub-store, para que os ramos que
// persistem (ChatSettings, Contacts) caiam nas guardas de nil e o teste
// exercite so' a construcao do evento.
func dispatchTestClient() *Client {
	return &Client{Log: waLog.Noop, Store: &store.Device{}}
}

const dispatchTestChatJID = "1234@s.whatsapp.net"
const dispatchTestSenderJID = "5678@s.whatsapp.net"

func setMutation(index []string, action *waSyncAction.SyncActionValue) appstate.Mutation {
	return appstate.Mutation{
		Operation: waServerSync.SyncdMutation_SET,
		Index:     index,
		Action:    action,
	}
}

// TestDispatchAppStateMalformedIndexDoesNotPanic trava a correcao do panico de
// `label_edit`: o indice vem do servidor e todo acesso posicional precisa ser
// guardado. Antes da Fase E lote 3, um `label_edit` de indice unitario
// derrubava o processo (o handler de nos roda em goroutine sem recover).
func TestDispatchAppStateMalformedIndexDoesNotPanic(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	shortIndices := [][]string{
		{appstate.IndexLabelEdit},
		{appstate.IndexLabelEdit, ""},
		{appstate.IndexLabelAssociationChat},
		{appstate.IndexLabelAssociationChat, "lbl"},
		{appstate.IndexLabelAssociationMessage, "lbl", dispatchTestChatJID},
		{appstate.IndexStar, dispatchTestChatJID},
		{appstate.IndexDeleteMessageForMe, dispatchTestChatJID, "msg"},
		{appstate.IndexClearChat},
		{appstate.IndexDeleteChat, dispatchTestChatJID},
		{appstate.IndexMute},
		{appstate.IndexPin},
		{appstate.IndexArchive},
		{appstate.IndexContact},
		{appstate.IndexMarkChatAsRead},
		{appstate.IndexUserStatusMute},
	}
	for _, index := range shortIndices {
		mutation := setMutation(index, &waSyncAction.SyncActionValue{})
		// O contrato aqui e' "nao entra em panico"; o evento resultante pode
		// ser nil (indice curto demais) ou um evento parcial.
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("dispatchAppState entrou em panico para indice %v: %v", index, r)
				}
			}()
			cli.dispatchAppState(context.Background(), appstate.WAPatchRegular, mutation, false)
		}()
	}
}

// TestDispatchAppStateLabelEditShortIndex e' o caso especifico do bug: com o
// indice minimo valido o evento sai; com um a menos, sai nil em vez de panico.
func TestDispatchAppStateLabelEditShortIndex(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()

	got := cli.dispatchAppState(context.Background(), appstate.WAPatchRegular,
		setMutation([]string{appstate.IndexLabelEdit}, &waSyncAction.SyncActionValue{}), false)
	if got != nil {
		t.Fatalf("esperado nil para label_edit sem labelID, veio %T", got)
	}

	got = cli.dispatchAppState(context.Background(), appstate.WAPatchRegular,
		setMutation([]string{appstate.IndexLabelEdit, "lbl-1"}, &waSyncAction.SyncActionValue{}), false)
	evt, ok := got.(*events.LabelEdit)
	if !ok {
		t.Fatalf("esperado *events.LabelEdit, veio %T", got)
	}
	if evt.LabelID != "lbl-1" {
		t.Errorf("LabelID = %q, esperado %q", evt.LabelID, "lbl-1")
	}
}

func TestDispatchAppStateNonSetOperationIsIgnored(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	mutation := appstate.Mutation{
		Operation: waServerSync.SyncdMutation_REMOVE,
		Index:     []string{appstate.IndexPin, dispatchTestChatJID},
		Action:    &waSyncAction.SyncActionValue{},
	}
	if got := cli.dispatchAppState(context.Background(), appstate.WAPatchRegularHigh, mutation, false); got != nil {
		t.Errorf("REMOVE deveria ser ignorado, veio %T", got)
	}
}

func TestDispatchAppStateUnknownIndexReturnsNil(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	mutation := setMutation([]string{"indice_que_nao_existe", dispatchTestChatJID}, &waSyncAction.SyncActionValue{})
	if got := cli.dispatchAppState(context.Background(), appstate.WAPatchRegular, mutation, false); got != nil {
		t.Errorf("indice desconhecido deveria dar nil, veio %T", got)
	}
}

func TestDispatchAppStateStar(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	action := &waSyncAction.SyncActionValue{
		Timestamp:  proto.Int64(1700000000000),
		StarAction: &waSyncAction.StarAction{Starred: proto.Bool(true)},
	}

	t.Run("com sender explicito", func(t *testing.T) {
		mutation := setMutation([]string{
			appstate.IndexStar, dispatchTestChatJID, "MSG1", "1", dispatchTestSenderJID,
		}, action)
		evt, ok := cli.dispatchAppState(context.Background(), appstate.WAPatchRegularHigh, mutation, true).(*events.Star)
		if !ok {
			t.Fatalf("esperado *events.Star")
		}
		if evt.MessageID != "MSG1" || !evt.IsFromMe || !evt.FromFullSync {
			t.Errorf("campos inesperados: %+v", evt)
		}
		if evt.SenderJID.String() != dispatchTestSenderJID {
			t.Errorf("SenderJID = %q, esperado %q", evt.SenderJID, dispatchTestSenderJID)
		}
		if !evt.Timestamp.Equal(time.UnixMilli(1700000000000)) {
			t.Errorf("Timestamp = %v", evt.Timestamp)
		}
	})

	t.Run("sender sentinela vira JID zerado", func(t *testing.T) {
		mutation := setMutation([]string{
			appstate.IndexStar, dispatchTestChatJID, "MSG2", "0", appStateIndexSelfSender,
		}, action)
		evt, ok := cli.dispatchAppState(context.Background(), appstate.WAPatchRegularHigh, mutation, false).(*events.Star)
		if !ok {
			t.Fatalf("esperado *events.Star")
		}
		if evt.IsFromMe {
			t.Errorf("IsFromMe deveria ser false para flag %q", "0")
		}
		if !evt.SenderJID.IsEmpty() {
			t.Errorf("SenderJID deveria ficar zerado para a sentinela %q, veio %q", appStateIndexSelfSender, evt.SenderJID)
		}
	})
}

func TestDispatchAppStateDeleteForMe(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	mutation := setMutation([]string{
		appstate.IndexDeleteMessageForMe, dispatchTestChatJID, "MSG3", "1", dispatchTestSenderJID,
	}, &waSyncAction.SyncActionValue{Timestamp: proto.Int64(1)})
	evt, ok := cli.dispatchAppState(context.Background(), appstate.WAPatchRegularHigh, mutation, false).(*events.DeleteForMe)
	if !ok {
		t.Fatalf("esperado *events.DeleteForMe")
	}
	if evt.MessageID != "MSG3" || !evt.IsFromMe {
		t.Errorf("campos inesperados: %+v", evt)
	}
	if evt.ChatJID.String() != dispatchTestChatJID {
		t.Errorf("ChatJID = %q", evt.ChatJID)
	}
}

// TestDispatchAppStateDeleteMediaFlags fixa em qual posicao do indice a flag
// `deleteMedia` mora em cada um dos dois tipos — sao posicoes diferentes, e a
// diferenca nao e' obvia lendo o codigo.
func TestDispatchAppStateDeleteMediaFlags(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	action := &waSyncAction.SyncActionValue{Timestamp: proto.Int64(1)}

	clearCases := []struct {
		name  string
		index []string
		want  bool
	}{
		{"flag ligada na posicao 3", []string{appstate.IndexClearChat, dispatchTestChatJID, "x", "1"}, true},
		{"flag desligada", []string{appstate.IndexClearChat, dispatchTestChatJID, "x", "0"}, false},
		{"indice curto demais para ter a flag", []string{appstate.IndexClearChat, dispatchTestChatJID, "x"}, false},
	}
	for _, tc := range clearCases {
		t.Run("clear_chat/"+tc.name, func(t *testing.T) {
			evt, ok := cli.dispatchAppState(context.Background(), appstate.WAPatchRegular, setMutation(tc.index, action), false).(*events.ClearChat)
			if !ok {
				t.Fatalf("esperado *events.ClearChat")
			}
			if evt.DeleteMedia != tc.want {
				t.Errorf("DeleteMedia = %v, esperado %v", evt.DeleteMedia, tc.want)
			}
		})
	}

	deleteCases := []struct {
		name  string
		index []string
		want  bool
	}{
		{"flag ligada na posicao 2", []string{appstate.IndexDeleteChat, dispatchTestChatJID, "1"}, true},
		{"flag desligada", []string{appstate.IndexDeleteChat, dispatchTestChatJID, "0"}, false},
		{"indice curto demais para ter a flag", []string{appstate.IndexDeleteChat, dispatchTestChatJID}, false},
	}
	for _, tc := range deleteCases {
		t.Run("delete_chat/"+tc.name, func(t *testing.T) {
			evt, ok := cli.dispatchAppState(context.Background(), appstate.WAPatchRegular, setMutation(tc.index, action), false).(*events.DeleteChat)
			if !ok {
				t.Fatalf("esperado *events.DeleteChat")
			}
			if evt.DeleteMedia != tc.want {
				t.Errorf("DeleteMedia = %v, esperado %v", evt.DeleteMedia, tc.want)
			}
		})
	}
}

func TestDispatchAppStateLabelAssociations(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	action := &waSyncAction.SyncActionValue{Timestamp: proto.Int64(1)}

	chatEvt, ok := cli.dispatchAppState(context.Background(), appstate.WAPatchRegular, setMutation([]string{
		appstate.IndexLabelAssociationChat, "lbl-7", dispatchTestChatJID,
	}, action), false).(*events.LabelAssociationChat)
	if !ok {
		t.Fatalf("esperado *events.LabelAssociationChat")
	}
	if chatEvt.LabelID != "lbl-7" || chatEvt.JID.String() != dispatchTestChatJID {
		t.Errorf("campos inesperados: %+v", chatEvt)
	}

	msgEvt, ok := cli.dispatchAppState(context.Background(), appstate.WAPatchRegular, setMutation([]string{
		appstate.IndexLabelAssociationMessage, "lbl-8", dispatchTestChatJID, "MSG9", "1", "0",
	}, action), false).(*events.LabelAssociationMessage)
	if !ok {
		t.Fatalf("esperado *events.LabelAssociationMessage")
	}
	if msgEvt.LabelID != "lbl-8" || msgEvt.MessageID != "MSG9" || msgEvt.JID.String() != dispatchTestChatJID {
		t.Errorf("campos inesperados: %+v", msgEvt)
	}
}

// TestDispatchAppStateSimpleEvents cobre os ramos que so' constroem um evento,
// sem tocar em nenhum sub-store.
func TestDispatchAppStateSimpleEvents(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	action := &waSyncAction.SyncActionValue{Timestamp: proto.Int64(1)}
	cases := []struct {
		index []string
		check func(t *testing.T, evt any)
	}{
		{[]string{appstate.IndexMarkChatAsRead, dispatchTestChatJID}, func(t *testing.T, evt any) {
			if _, ok := evt.(*events.MarkChatAsRead); !ok {
				t.Errorf("esperado *events.MarkChatAsRead, veio %T", evt)
			}
		}},
		{[]string{appstate.IndexSettingUnarchiveChats}, func(t *testing.T, evt any) {
			if _, ok := evt.(*events.UnarchiveChatsSetting); !ok {
				t.Errorf("esperado *events.UnarchiveChatsSetting, veio %T", evt)
			}
		}},
		{[]string{appstate.IndexUserStatusMute, dispatchTestChatJID}, func(t *testing.T, evt any) {
			if _, ok := evt.(*events.UserStatusMute); !ok {
				t.Errorf("esperado *events.UserStatusMute, veio %T", evt)
			}
		}},
		{[]string{appstate.IndexMute, dispatchTestChatJID}, func(t *testing.T, evt any) {
			if _, ok := evt.(*events.Mute); !ok {
				t.Errorf("esperado *events.Mute, veio %T", evt)
			}
		}},
		{[]string{appstate.IndexPin, dispatchTestChatJID}, func(t *testing.T, evt any) {
			if _, ok := evt.(*events.Pin); !ok {
				t.Errorf("esperado *events.Pin, veio %T", evt)
			}
		}},
		{[]string{appstate.IndexArchive, dispatchTestChatJID}, func(t *testing.T, evt any) {
			if _, ok := evt.(*events.Archive); !ok {
				t.Errorf("esperado *events.Archive, veio %T", evt)
			}
		}},
		{[]string{appstate.IndexContact, dispatchTestChatJID}, func(t *testing.T, evt any) {
			if _, ok := evt.(*events.Contact); !ok {
				t.Errorf("esperado *events.Contact, veio %T", evt)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.index[0], func(t *testing.T) {
			tc.check(t, cli.dispatchAppState(context.Background(), appstate.WAPatchRegular, setMutation(tc.index, action), false))
		})
	}
}

// TestDispatchAppStateMuteForever trava a traducao de `MuteEndTimestamp`
// negativo para `store.MutedForever` (a mutacao so' e' observavel pelo evento,
// ja' que ChatSettings e' nil neste Client).
func TestDispatchAppStateMuteForever(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	action := &waSyncAction.SyncActionValue{
		Timestamp: proto.Int64(1),
		MuteAction: &waSyncAction.MuteAction{
			Muted:            proto.Bool(true),
			MuteEndTimestamp: proto.Int64(-1),
		},
	}
	evt, ok := cli.dispatchAppState(context.Background(), appstate.WAPatchRegularHigh,
		setMutation([]string{appstate.IndexMute, dispatchTestChatJID}, action), false).(*events.Mute)
	if !ok {
		t.Fatalf("esperado *events.Mute")
	}
	if !evt.Action.GetMuted() || evt.Action.GetMuteEndTimestamp() != -1 {
		t.Errorf("acao de mute nao preservada: %+v", evt.Action)
	}
}

func TestFilterContacts(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	contactAction := &waSyncAction.SyncActionValue{
		ContactAction: &waSyncAction.ContactAction{
			FirstName: proto.String("Ana"),
			FullName:  proto.String("Ana Silva"),
		},
	}
	mutations := []appstate.Mutation{
		setMutation([]string{appstate.IndexContact, dispatchTestChatJID}, contactAction),
		setMutation([]string{appstate.IndexPin, dispatchTestChatJID}, &waSyncAction.SyncActionValue{}),
		setMutation([]string{appstate.IndexContact, dispatchTestSenderJID}, contactAction),
		// `contact` sem JID nao vira ContactEntry: fica nas mutacoes.
		setMutation([]string{appstate.IndexContact}, contactAction),
	}

	filtered, contacts := cli.filterContacts(mutations)

	if len(contacts) != 2 {
		t.Fatalf("len(contacts) = %d, esperado 2", len(contacts))
	}
	if contacts[0].JID.String() != dispatchTestChatJID || contacts[0].FirstName != "Ana" || contacts[0].FullName != "Ana Silva" {
		t.Errorf("primeiro contato inesperado: %+v", contacts[0])
	}
	if contacts[1].JID.String() != dispatchTestSenderJID {
		t.Errorf("segundo contato inesperado: %+v", contacts[1])
	}
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, esperado 2", len(filtered))
	}
	if filtered[0].Index[0] != appstate.IndexPin {
		t.Errorf("primeira mutacao restante = %v, esperado pin", filtered[0].Index)
	}
	if len(filtered[1].Index) != 1 || filtered[1].Index[0] != appstate.IndexContact {
		t.Errorf("segunda mutacao restante = %v, esperado contact sem JID", filtered[1].Index)
	}
}

func TestFilterContactsEmpty(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	filtered, contacts := cli.filterContacts(nil)
	if len(filtered) != 0 {
		t.Errorf("len(filtered) = %d, esperado 0", len(filtered))
	}
	if contacts == nil {
		t.Error("contacts deveria ser slice vazia nao-nil (vai direto para PutAllContactNames)")
	}
}

// TestFilterContactsJIDSemServidor trava o comportamento atual para um indice
// cujo campo de JID nao tem "@": `types.ParseJID` trata a string inteira como
// *servidor* (usuario vazio), o erro e' descartado de proposito e o contato
// entra assim mesmo. Nao e' o ideal, mas e' o que o codigo faz — o teste
// existe para que uma mudanca aqui seja consciente.
func TestFilterContactsJIDSemServidor(t *testing.T) {
	t.Parallel()
	cli := dispatchTestClient()
	_, contacts := cli.filterContacts([]appstate.Mutation{
		setMutation([]string{appstate.IndexContact, "nao-e-um-jid"}, &waSyncAction.SyncActionValue{}),
	})
	if len(contacts) != 1 {
		t.Fatalf("len(contacts) = %d, esperado 1", len(contacts))
	}
	if contacts[0].JID.User != "" {
		t.Errorf("User deveria ficar vazio, veio %q", contacts[0].JID.User)
	}
	if contacts[0].JID.Server != "nao-e-um-jid" {
		t.Errorf("Server = %q, esperado %q", contacts[0].JID.Server, "nao-e-um-jid")
	}
}
