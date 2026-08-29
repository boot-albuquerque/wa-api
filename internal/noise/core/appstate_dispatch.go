package core

import (
	"context"

	"wa-api/internal/noise/capabilities/appstatesync"
	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/appstate"
)

// Fachada da traducao de mutacoes em eventos. A logica vive em
// internal/wa-noise/appstatesync (dispatch.go e mutation.go); estes metodos
// continuam existindo porque internals.go (gerado) os embrulha.

func (cli *Client) collectEventsToDispatch(
	ctx context.Context,
	name appstate.WAPatchName,
	mutations []appstate.Mutation,
	fullSync bool,
	eventsToDispatch *[]any,
) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return appstatesync.CollectEvents(ctx, cli.appStateT(), name, mutations, fullSync, eventsToDispatch)
}

// filterContacts nao usa nada de *Client — o metodo existe so' para preservar a
// assinatura que internals.go embrulha.
func (cli *Client) filterContacts(mutations []appstate.Mutation) ([]appstate.Mutation, []store.ContactEntry) {
	return appstatesync.FilterContacts(mutations)
}

func (cli *Client) dispatchAppState(ctx context.Context, name appstate.WAPatchName, mutation appstate.Mutation, fullSync bool) (eventToDispatch any) {
	if cli == nil {
		return nil
	}
	return appstatesync.DispatchMutation(ctx, cli.appStateT(), name, mutation, fullSync)
}
