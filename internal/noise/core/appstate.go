package core

import (
	"context"

	"wa-api/internal/noise/capabilities/appstatesync"
	"wa-api/internal/noise/protocol/appstate"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/proto/waServerSync"
	"wa-api/internal/noise/protocol/types"
)

// Fachada do dominio de sincronizacao de app state. A logica vive em
// internal/wa-noise/appstatesync; aqui ficam so' os metodos de *Client que
// delegam para la' (ver ADR-0004 e PATCHES.md, "Fase F/G — lote 3"). Os metodos
// nao exportados continuam existindo porque internals.go (gerado) os embrulha.

// FetchAppState fetches updates to the given type of app state. If fullSync is true, the current
// cached state will be removed and all app state patches will be re-fetched from the server.
func (cli *Client) FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	eventsToDispatch, err := cli.fetchAppState(ctx, name, fullSync, onlyIfNotSynced)
	if err != nil {
		return err
	}
	for _, evt := range eventsToDispatch {
		cli.dispatchEvent(evt)
	}
	return nil
}

func (cli *Client) fetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) ([]any, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return appstatesync.Fetch(ctx, cli.appStateT(), name, fullSync, onlyIfNotSynced)
}

func (cli *Client) handleAppStateRecovery(
	ctx context.Context,
	reqID types.MessageID,
	result []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult,
) bool {
	if cli == nil {
		return true
	}
	return appstatesync.HandleRecovery(ctx, cli.appStateT(), reqID, result)
}

func (cli *Client) applyAppStatePatches(
	ctx context.Context,
	name appstate.WAPatchName,
	state appstate.HashState,
	patches *appstate.PatchList,
	fullSync bool,
	eventsToDispatch *[]any,
) (appstate.HashState, error) {
	if cli == nil {
		return state, ErrClientIsNil
	}
	return appstatesync.ApplyPatches(ctx, cli.appStateT(), name, state, patches, fullSync, eventsToDispatch)
}

// downloadExternalAppStateBlob nao delega para o subpacote: e' a ponta que o
// subpacote chama de volta atraves de Transport, e o download de midia ja' e'
// dominio de internal/wa-noise/media.
func (cli *Client) downloadExternalAppStateBlob(ctx context.Context, ref *waServerSync.ExternalBlobReference) ([]byte, error) {
	return cli.Download(ctx, ref)
}

func (cli *Client) fetchAppStatePatches(ctx context.Context, name appstate.WAPatchName, fromVersion uint64, snapshot bool) (*appstate.PatchList, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return appstatesync.FetchPatches(ctx, cli.appStateT(), name, fromVersion, snapshot)
}
