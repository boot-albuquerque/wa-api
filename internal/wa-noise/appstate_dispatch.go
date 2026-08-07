// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"wa-api/internal/wa-noise/appstate"
	"wa-api/internal/wa-noise/appstatesync"
	"wa-api/internal/wa-noise/store"
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
