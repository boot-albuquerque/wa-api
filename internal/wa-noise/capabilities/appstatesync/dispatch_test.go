package appstatesync

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/protocol/proto/waSyncAction"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// Testes de FilterContacts relocados de appstate_dispatch_test.go (Fase E lote
// 3): a funcao virou livre, entao nao precisa mais de um *Client.

func TestFilterContacts(t *testing.T) {
	t.Parallel()
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

	filtered, contacts := FilterContacts(mutations)

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
	filtered, contacts := FilterContacts(nil)
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
// entra assim mesmo. Nao e' o ideal, mas e' o que o codigo faz — o teste existe
// para que uma mudanca aqui seja consciente.
func TestFilterContactsJIDSemServidor(t *testing.T) {
	t.Parallel()
	_, contacts := FilterContacts([]appstate.Mutation{
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

// --- CollectEvents ---

func TestCollectEventsAcumulaAppStateEEvento(t *testing.T) {
	t.Parallel()
	tp := newFakeTransport()
	var out []any
	err := CollectEvents(context.Background(), tp, appstate.WAPatchRegular, []appstate.Mutation{
		setMutation([]string{appstate.IndexMarkChatAsRead, dispatchTestChatJID}, &waSyncAction.SyncActionValue{}),
	}, false, &out)
	if err != nil {
		t.Fatalf("CollectEvents: %v", err)
	}
	// Cada mutacao SET gera dois itens: o events.AppState cru e o evento tipado.
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, esperado 2: %#v", len(out), out)
	}
	if _, ok := out[0].(*events.AppState); !ok {
		t.Errorf("out[0] = %T, esperado *events.AppState", out[0])
	}
	if _, ok := out[1].(*events.MarkChatAsRead); !ok {
		t.Errorf("out[1] = %T, esperado *events.MarkChatAsRead", out[1])
	}
}

// TestCollectEventsPonteiroNilNaoEmite: com eventsToDispatch nil a mutacao
// ainda e' aplicada, mas nada e' acumulado. E' o caminho do full sync com
// EmitEventsOnFullSync desligado.
func TestCollectEventsPonteiroNilNaoEmite(t *testing.T) {
	t.Parallel()
	tp := newFakeTransport()
	settings := newFakeChatSettings()
	tp.store.ChatSettings = settings
	err := CollectEvents(context.Background(), tp, appstate.WAPatchRegular, []appstate.Mutation{
		setMutation([]string{appstate.IndexPin, dispatchTestChatJID},
			&waSyncAction.SyncActionValue{PinAction: &waSyncAction.PinAction{Pinned: proto.Bool(true)}}),
	}, false, nil)
	if err != nil {
		t.Fatalf("CollectEvents: %v", err)
	}
	if len(settings.pinned) != 1 {
		t.Error("a mutacao deveria ter sido aplicada mesmo sem emitir evento")
	}
}

// TestCollectEventsOperacaoRemoveNaoGeraAppState: so' SET gera o events.AppState
// cru.
func TestCollectEventsOperacaoRemoveNaoGeraAppState(t *testing.T) {
	t.Parallel()
	tp := newFakeTransport()
	var out []any
	err := CollectEvents(context.Background(), tp, appstate.WAPatchRegular, []appstate.Mutation{{
		Operation: waServerSync.SyncdMutation_REMOVE,
		Index:     []string{appstate.IndexPin, dispatchTestChatJID},
		Action:    &waSyncAction.SyncActionValue{},
	}}, false, &out)
	if err != nil {
		t.Fatalf("CollectEvents: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("REMOVE nao deveria gerar nada, veio %#v", out)
	}
}

// TestCollectEventsSnapshotDeContatos cobre a insercao em massa: so' acontece
// para critical_unblock_low, em full sync, com a emissao desligada.
func TestCollectEventsSnapshotDeContatos(t *testing.T) {
	t.Parallel()
	contacts := &fakeContactStore{}
	tp := newFakeTransport()
	tp.store.Contacts = contacts
	contactAction := &waSyncAction.SyncActionValue{
		ContactAction: &waSyncAction.ContactAction{FullName: proto.String("Ana")},
	}
	err := CollectEvents(context.Background(), tp, appstate.WAPatchCriticalUnblockLow, []appstate.Mutation{
		setMutation([]string{appstate.IndexContact, dispatchTestChatJID}, contactAction),
		setMutation([]string{appstate.IndexPin, dispatchTestChatJID}, &waSyncAction.SyncActionValue{}),
	}, true, nil)
	if err != nil {
		t.Fatalf("CollectEvents: %v", err)
	}
	if len(contacts.allNames) != 1 {
		t.Fatalf("PutAllContactNames recebeu %d contatos, esperado 1", len(contacts.allNames))
	}
	// O contato saiu das mutacoes: PutContactName (o caminho um-a-um) nao roda.
	if len(contacts.names) != 0 {
		t.Errorf("contato nao deveria passar tambem pelo caminho individual: %v", contacts.names)
	}
}

// TestCollectEventsSnapshotDeContatosAborta trava que uma falha na insercao em
// massa aborta tudo: e' o unico erro que CollectEvents propaga.
func TestCollectEventsSnapshotDeContatosAborta(t *testing.T) {
	t.Parallel()
	tp := newFakeTransport()
	tp.store.Contacts = &fakeContactStore{allErr: errors.New("db fora do ar")}
	err := CollectEvents(context.Background(), tp, appstate.WAPatchCriticalUnblockLow, []appstate.Mutation{
		setMutation([]string{appstate.IndexContact, dispatchTestChatJID}, &waSyncAction.SyncActionValue{}),
	}, true, nil)
	if err == nil {
		t.Fatal("esperado erro de PutAllContactNames")
	}
}

// TestCollectEventsSnapshotSoComEmissaoDesligada: com EmitEventsOnFullSync
// ligado, o caminho de insercao em massa NAO e' tomado — os contatos viram
// eventos um a um.
func TestCollectEventsSnapshotSoComEmissaoDesligada(t *testing.T) {
	t.Parallel()
	contacts := &fakeContactStore{}
	tp := newFakeTransport()
	tp.emitOnFullSync = true
	tp.store.Contacts = contacts
	var out []any
	err := CollectEvents(context.Background(), tp, appstate.WAPatchCriticalUnblockLow, []appstate.Mutation{
		setMutation([]string{appstate.IndexContact, dispatchTestChatJID}, &waSyncAction.SyncActionValue{}),
	}, true, &out)
	if err != nil {
		t.Fatalf("CollectEvents: %v", err)
	}
	if len(contacts.allNames) != 0 {
		t.Errorf("insercao em massa nao deveria rodar: %v", contacts.allNames)
	}
	if len(out) != 2 {
		t.Errorf("esperado AppState + Contact, veio %#v", out)
	}
}
