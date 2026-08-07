package appstatesync

import (
	"context"
	"fmt"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// CollectEvents percorre as mutacoes de um patch, aplica cada uma ao store e
// acumula em eventsToDispatch os eventos que o chamador deve despachar.
// eventsToDispatch nil significa "aplique, mas nao emita nada" — e' o caso do
// full sync quando EmitEventsOnFullSync esta' desligado.
func CollectEvents(
	ctx context.Context,
	t Transport,
	name appstate.WAPatchName,
	mutations []appstate.Mutation,
	fullSync bool,
	eventsToDispatch *[]any,
) error {
	if name == appstate.WAPatchCriticalUnblockLow && fullSync && !t.EmitEventsOnFullSync() {
		var contacts []store.ContactEntry
		mutations, contacts = FilterContacts(mutations)
		t.Log().Debugf("Mass inserting app state snapshot with %d contacts into the store", len(contacts))
		err := t.Store().Contacts.PutAllContactNames(ctx, contacts)
		if err != nil {
			// This is a fairly serious failure, so just abort the whole thing
			return fmt.Errorf("failed to update contact store with data from snapshot: %v", err)
		}
	}
	for _, mutation := range mutations {
		if eventsToDispatch != nil && mutation.Operation == waServerSync.SyncdMutation_SET {
			*eventsToDispatch = append(*eventsToDispatch, &events.AppState{Index: mutation.Index, SyncActionValue: mutation.Action})
		}
		evt := DispatchMutation(ctx, t, name, mutation, fullSync)
		if eventsToDispatch != nil && evt != nil {
			*eventsToDispatch = append(*eventsToDispatch, evt)
		}
	}
	return nil
}

// FilterContacts separa as mutacoes de contato das demais, devolvendo as que
// sobraram e as entradas de contato prontas para a insercao em massa. Uma
// mutacao de contato sem JID no indice nao vira ContactEntry: fica entre as
// mutacoes restantes.
func FilterContacts(mutations []appstate.Mutation) ([]appstate.Mutation, []store.ContactEntry) {
	filteredMutations := mutations[:0]
	contacts := make([]store.ContactEntry, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.Index[0] == appstate.IndexContact && len(mutation.Index) > 1 {
			jid, _ := types.ParseJID(mutation.Index[1])
			act := mutation.Action.GetContactAction()
			contacts = append(contacts, store.ContactEntry{
				JID:       jid,
				FirstName: act.GetFirstName(),
				FullName:  act.GetFullName(),
			})
		} else {
			filteredMutations = append(filteredMutations, mutation)
		}
	}
	return filteredMutations, contacts
}
