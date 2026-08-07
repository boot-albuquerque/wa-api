package appstatesync

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/protocol/proto/waSyncAction"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// Testes relocados de internal/wa-noise/appstate_dispatch_test.go (Fase E lote
// 3) e adaptados ao duble de Transport. O contrato exercitado e' o mesmo; o que
// mudou e' que nao e' mais preciso montar um *Client.

// dispatchTestTransport monta o minimo que DispatchMutation precisa: um Store
// sem nenhum sub-store, para que os ramos que persistem (ChatSettings,
// Contacts) caiam nas guardas de nil e o teste exercite so' a construcao do
// evento.
func dispatchTestTransport() *fakeTransport {
	return newFakeTransport()
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
// derrubava o processo (o handler de nos roda em goroutine sem recover). A
// extracao da Fase F/G lote 3 preservou a guarda; este teste atravessou junto.
func TestDispatchAppStateMalformedIndexDoesNotPanic(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
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
					t.Errorf("DispatchMutation entrou em panico para indice %v: %v", index, r)
				}
			}()
			DispatchMutation(context.Background(), tp, appstate.WAPatchRegular, mutation, false)
		}()
	}
}

// TestDispatchAppStateLabelEditShortIndex e' o caso especifico do bug: com o
// indice minimo valido o evento sai; com um a menos, sai nil em vez de panico.
func TestDispatchAppStateLabelEditShortIndex(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()

	got := DispatchMutation(context.Background(), tp, appstate.WAPatchRegular,
		setMutation([]string{appstate.IndexLabelEdit}, &waSyncAction.SyncActionValue{}), false)
	if got != nil {
		t.Fatalf("esperado nil para label_edit sem labelID, veio %T", got)
	}

	got = DispatchMutation(context.Background(), tp, appstate.WAPatchRegular,
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
	tp := dispatchTestTransport()
	mutation := appstate.Mutation{
		Operation: waServerSync.SyncdMutation_REMOVE,
		Index:     []string{appstate.IndexPin, dispatchTestChatJID},
		Action:    &waSyncAction.SyncActionValue{},
	}
	if got := DispatchMutation(context.Background(), tp, appstate.WAPatchRegularHigh, mutation, false); got != nil {
		t.Errorf("REMOVE deveria ser ignorado, veio %T", got)
	}
}

func TestDispatchAppStateUnknownIndexReturnsNil(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
	mutation := setMutation([]string{"indice_que_nao_existe", dispatchTestChatJID}, &waSyncAction.SyncActionValue{})
	if got := DispatchMutation(context.Background(), tp, appstate.WAPatchRegular, mutation, false); got != nil {
		t.Errorf("indice desconhecido deveria dar nil, veio %T", got)
	}
}

func TestDispatchAppStateStar(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
	action := &waSyncAction.SyncActionValue{
		Timestamp:  proto.Int64(1700000000000),
		StarAction: &waSyncAction.StarAction{Starred: proto.Bool(true)},
	}

	t.Run("com sender explicito", func(t *testing.T) {
		mutation := setMutation([]string{
			appstate.IndexStar, dispatchTestChatJID, "MSG1", "1", dispatchTestSenderJID,
		}, action)
		evt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchRegularHigh, mutation, true).(*events.Star)
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
			appstate.IndexStar, dispatchTestChatJID, "MSG2", "0", indexSelfSender,
		}, action)
		evt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchRegularHigh, mutation, false).(*events.Star)
		if !ok {
			t.Fatalf("esperado *events.Star")
		}
		if evt.IsFromMe {
			t.Errorf("IsFromMe deveria ser false para flag %q", "0")
		}
		if !evt.SenderJID.IsEmpty() {
			t.Errorf("SenderJID deveria ficar zerado para a sentinela %q, veio %q", indexSelfSender, evt.SenderJID)
		}
	})
}

func TestDispatchAppStateDeleteForMe(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
	mutation := setMutation([]string{
		appstate.IndexDeleteMessageForMe, dispatchTestChatJID, "MSG3", "1", dispatchTestSenderJID,
	}, &waSyncAction.SyncActionValue{Timestamp: proto.Int64(1)})
	evt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchRegularHigh, mutation, false).(*events.DeleteForMe)
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

// TestDispatchAppStateDeleteForMeSenderSentinela cobre o ramo simetrico ao de
// star: sender "0" deixa o SenderJID zerado.
func TestDispatchAppStateDeleteForMeSenderSentinela(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
	evt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchRegularHigh, setMutation([]string{
		appstate.IndexDeleteMessageForMe, dispatchTestChatJID, "MSG4", "0", indexSelfSender,
	}, &waSyncAction.SyncActionValue{Timestamp: proto.Int64(1)}), false).(*events.DeleteForMe)
	if !ok {
		t.Fatalf("esperado *events.DeleteForMe")
	}
	if !evt.SenderJID.IsEmpty() {
		t.Errorf("SenderJID deveria ficar zerado, veio %q", evt.SenderJID)
	}
}

// TestDispatchAppStateDeleteMediaFlags fixa em qual posicao do indice a flag
// `deleteMedia` mora em cada um dos dois tipos — sao posicoes diferentes, e a
// diferenca nao e' obvia lendo o codigo.
func TestDispatchAppStateDeleteMediaFlags(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
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
			evt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchRegular, setMutation(tc.index, action), false).(*events.ClearChat)
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
			evt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchRegular, setMutation(tc.index, action), false).(*events.DeleteChat)
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
	tp := dispatchTestTransport()
	action := &waSyncAction.SyncActionValue{Timestamp: proto.Int64(1)}

	chatEvt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchRegular, setMutation([]string{
		appstate.IndexLabelAssociationChat, "lbl-7", dispatchTestChatJID,
	}, action), false).(*events.LabelAssociationChat)
	if !ok {
		t.Fatalf("esperado *events.LabelAssociationChat")
	}
	if chatEvt.LabelID != "lbl-7" || chatEvt.JID.String() != dispatchTestChatJID {
		t.Errorf("campos inesperados: %+v", chatEvt)
	}

	msgEvt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchRegular, setMutation([]string{
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
	tp := dispatchTestTransport()
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
			tc.check(t, DispatchMutation(context.Background(), tp, appstate.WAPatchRegular, setMutation(tc.index, action), false))
		})
	}
}

// TestDispatchAppStateMuteForever trava a traducao de `MuteEndTimestamp`
// negativo para `store.MutedForever`.
func TestDispatchAppStateMuteForever(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
	settings := newFakeChatSettings()
	tp.store.ChatSettings = settings
	action := &waSyncAction.SyncActionValue{
		Timestamp: proto.Int64(1),
		MuteAction: &waSyncAction.MuteAction{
			Muted:            proto.Bool(true),
			MuteEndTimestamp: proto.Int64(-1),
		},
	}
	evt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchRegularHigh,
		setMutation([]string{appstate.IndexMute, dispatchTestChatJID}, action), false).(*events.Mute)
	if !ok {
		t.Fatalf("esperado *events.Mute")
	}
	if !evt.Action.GetMuted() || evt.Action.GetMuteEndTimestamp() != -1 {
		t.Errorf("acao de mute nao preservada: %+v", evt.Action)
	}
	if !settings.mutedUntil[evt.JID].Equal(store.MutedForever) {
		t.Errorf("mutedUntil = %v, esperado MutedForever", settings.mutedUntil[evt.JID])
	}
}

// TestDispatchAppStateEscreveNosSubStores cobre os quatro ramos que persistem:
// mute com prazo, pin, archive e contact.
func TestDispatchAppStateEscreveNosSubStores(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
	settings := newFakeChatSettings()
	contacts := &fakeContactStore{}
	tp.store.ChatSettings = settings
	tp.store.Contacts = contacts
	ctx := context.Background()

	muteUntil := int64(1700000000000)
	DispatchMutation(ctx, tp, appstate.WAPatchRegularHigh, setMutation(
		[]string{appstate.IndexMute, dispatchTestChatJID},
		&waSyncAction.SyncActionValue{MuteAction: &waSyncAction.MuteAction{
			Muted: proto.Bool(true), MuteEndTimestamp: proto.Int64(muteUntil),
		}}), false)
	DispatchMutation(ctx, tp, appstate.WAPatchRegularHigh, setMutation(
		[]string{appstate.IndexPin, dispatchTestChatJID},
		&waSyncAction.SyncActionValue{PinAction: &waSyncAction.PinAction{Pinned: proto.Bool(true)}}), false)
	DispatchMutation(ctx, tp, appstate.WAPatchRegularHigh, setMutation(
		[]string{appstate.IndexArchive, dispatchTestChatJID},
		&waSyncAction.SyncActionValue{ArchiveChatAction: &waSyncAction.ArchiveChatAction{Archived: proto.Bool(true)}}), false)
	DispatchMutation(ctx, tp, appstate.WAPatchRegular, setMutation(
		[]string{appstate.IndexContact, dispatchTestChatJID},
		&waSyncAction.SyncActionValue{ContactAction: &waSyncAction.ContactAction{FullName: proto.String("Ana")}}), false)

	for jid, until := range settings.mutedUntil {
		if !until.Equal(time.UnixMilli(muteUntil)) {
			t.Errorf("mutedUntil[%s] = %v, esperado %v", jid, until, time.UnixMilli(muteUntil))
		}
	}
	if len(settings.pinned) != 1 || len(settings.archived) != 1 {
		t.Errorf("pin/archive nao chegaram no store: %v %v", settings.pinned, settings.archived)
	}
	if len(contacts.names) != 1 {
		t.Errorf("PutContactName nao foi chamado: %v", contacts.names)
	}
}

// TestDispatchAppStateErroDeStoreSoLoga trava que uma falha ao persistir nao
// impede o evento de sair: o evento e' o dado do protocolo, o store e' cache.
func TestDispatchAppStateErroDeStoreSoLoga(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
	settings := newFakeChatSettings()
	settings.err = errors.New("boom")
	tp.store.ChatSettings = settings
	evt := DispatchMutation(context.Background(), tp, appstate.WAPatchRegularHigh, setMutation(
		[]string{appstate.IndexPin, dispatchTestChatJID},
		&waSyncAction.SyncActionValue{PinAction: &waSyncAction.PinAction{Pinned: proto.Bool(true)}}), false)
	if _, ok := evt.(*events.Pin); !ok {
		t.Fatalf("evento deveria sair mesmo com o store falhando, veio %T", evt)
	}
}

// TestDispatchAppStatePushName cobre o unico ramo que grava no Device em si.
func TestDispatchAppStatePushName(t *testing.T) {
	t.Parallel()
	container := &fakeContainer{}
	tp := dispatchTestTransport()
	tp.store.Container = container
	evt, ok := DispatchMutation(context.Background(), tp, appstate.WAPatchCriticalBlock, setMutation(
		[]string{appstate.IndexSettingPushName},
		&waSyncAction.SyncActionValue{PushNameSetting: &waSyncAction.PushNameSetting{
			Name: proto.String("Ana"),
		}}), false).(*events.PushNameSetting)
	if !ok {
		t.Fatalf("esperado *events.PushNameSetting")
	}
	if evt.Action.GetName() != "Ana" {
		t.Errorf("Name = %q", evt.Action.GetName())
	}
	if tp.store.PushName != "Ana" {
		t.Errorf("Store.PushName = %q, esperado Ana", tp.store.PushName)
	}
	if container.saved != 1 {
		t.Errorf("Save chamado %d vezes, esperado 1", container.saved)
	}
}

// TestDispatchAppStatePushNameSaveFalha: a falha ao salvar so' loga.
func TestDispatchAppStatePushNameSaveFalha(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
	tp.store.Container = &fakeContainer{err: errors.New("disco cheio")}
	evt := DispatchMutation(context.Background(), tp, appstate.WAPatchCriticalBlock, setMutation(
		[]string{appstate.IndexSettingPushName},
		&waSyncAction.SyncActionValue{PushNameSetting: &waSyncAction.PushNameSetting{Name: proto.String("Ana")}}), false)
	if _, ok := evt.(*events.PushNameSetting); !ok {
		t.Fatalf("evento deveria sair mesmo com Save falhando, veio %T", evt)
	}
}

// TestDispatchAppStateNCTSalt cobre o prefixo de nct_salt_sync: SET grava,
// REMOVE apaga, e nenhum dos dois gera evento.
func TestDispatchAppStateNCTSalt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("set grava o salt", func(t *testing.T) {
		tp := dispatchTestTransport()
		got := DispatchMutation(ctx, tp, appstate.WAPatchRegular, setMutation(
			[]string{appstate.IndexNCTSaltSync},
			&waSyncAction.SyncActionValue{NctSaltSyncAction: &waSyncAction.NctSaltSyncAction{
				Salt: []byte{1, 2, 3},
			}}), false)
		if got != nil {
			t.Errorf("nct_salt_sync nao deveria gerar evento, veio %T", got)
		}
		if len(tp.nctSalt) != 3 {
			t.Errorf("salt = %v", tp.nctSalt)
		}
	})

	t.Run("remove apaga o salt", func(t *testing.T) {
		tp := dispatchTestTransport()
		DispatchMutation(ctx, tp, appstate.WAPatchRegular, appstate.Mutation{
			Operation: waServerSync.SyncdMutation_REMOVE,
			Index:     []string{appstate.IndexNCTSaltSync},
			Action:    &waSyncAction.SyncActionValue{},
		}, false)
		if !tp.nctCleared {
			t.Error("ClearNCTSalt nao foi chamado")
		}
	})

	t.Run("erro so loga", func(t *testing.T) {
		tp := dispatchTestTransport()
		tp.nctStoreErr = errors.New("boom")
		DispatchMutation(ctx, tp, appstate.WAPatchRegular, setMutation(
			[]string{appstate.IndexNCTSaltSync},
			&waSyncAction.SyncActionValue{}), false)
	})

	// Uma operacao que nao e' nem SET nem REMOVE nao toca em nada.
	t.Run("operacao neutra nao toca no salt", func(t *testing.T) {
		tp := dispatchTestTransport()
		DispatchMutation(ctx, tp, appstate.WAPatchRegular, appstate.Mutation{
			Operation: waServerSync.SyncdMutation_SyncdOperation(99),
			Index:     []string{appstate.IndexNCTSaltSync},
			Action:    &waSyncAction.SyncActionValue{},
		}, false)
		if tp.nctSalt != nil || tp.nctCleared {
			t.Error("operacao desconhecida nao deveria mexer no salt")
		}
	})
}

// TestDispatchAppStateDebugLogs exercita o ramo de nivel de log elevado.
func TestDispatchAppStateDebugLogs(t *testing.T) {
	t.Parallel()
	tp := dispatchTestTransport()
	tp.debugLogs = true
	DispatchMutation(context.Background(), tp, appstate.WAPatchRegular, setMutation(
		[]string{appstate.IndexMarkChatAsRead, dispatchTestChatJID},
		&waSyncAction.SyncActionValue{}), false)
}
