// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstatesync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"wa-api/internal/wa-noise/appstate"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// ErrUpdate e' o sentinela de falha reportada pelo servidor ao aplicar um patch
// de app state. A raiz reexporta ESTE MESMO valor como ErrAppStateUpdate — nao
// uma copia — para que errors.Is atravesse a fronteira dos dois pacotes.
var ErrUpdate = errors.New("server returned error updating app state")

// Send envia o patch de app state dado e, em seguida, ressincroniza aquele tipo
// de app state para atualizar caches locais e emitir os eventos das mudancas.
//
// allowRetry controla a unica retentativa permitida: quando o servidor responde
// com conflito (versao desatualizada), os patches conflitantes vem na propria
// resposta, sao aplicados, e o envio e' refeito uma vez — com allowRetry false,
// para que um servidor que insista em conflitar nao gere recursao infinita.
func Send(ctx context.Context, t Transport, patch appstate.PatchInfo, allowRetry bool) error {
	version, hash, err := t.Store().AppState.GetAppStateVersion(ctx, string(patch.Type))
	if err != nil {
		return err
	}
	// TODO create new key instead of reusing the primary client's keys
	latestKeyID, err := t.Store().AppStateKeys.GetLatestAppStateSyncKeyID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest app state key ID: %w", err)
	} else if latestKeyID == nil {
		return fmt.Errorf("no app state keys found, creating app state keys is not yet supported")
	}

	state := appstate.HashState{Version: version, Hash: hash}

	encodedPatch, err := t.Proc().EncodePatch(ctx, latestKeyID, state, patch)
	if err != nil {
		return err
	}

	resp, err := t.SendIQ(ctx, IQ{
		Namespace: namespace,
		Type:      IQSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: syncTag,
			Content: []waBinary.Node{{
				Tag: collectionTag,
				Attrs: waBinary.Attrs{
					attrName:           string(patch.Type),
					attrVersion:        version,
					attrReturnSnapshot: false,
				},
				Content: []waBinary.Node{{
					Tag:     patchTag,
					Content: encodedPatch,
				}},
			}},
		}},
	})
	if err != nil {
		return err
	}

	respCollection, ok := resp.GetOptionalChildByTag(syncTag, collectionTag)
	if !ok {
		return t.ElementMissing(collectionTag, sendErrContext)
	}
	respCollectionAttr := respCollection.AttrGetter()
	if respCollectionAttr.OptionalString(attrType) == respTypeError {
		return handleSendError(ctx, t, patch, state, respCollection, allowRetry)
	}
	eventsToDispatch, err := Fetch(ctx, t, patch.Type, false, false)
	if err != nil {
		return fmt.Errorf("failed to fetch app state after sending update: %w", err)
	}
	go dispatchAll(t, eventsToDispatch)

	return nil
}

// handleSendError trata a `<collection type="error">` da resposta. Em caso de
// conflito (e com retentativa permitida) aplica os patches que vieram junto e
// refaz o envio; em qualquer outro caso devolve o erro embrulhando ErrUpdate.
func handleSendError(
	ctx context.Context,
	t Transport,
	patch appstate.PatchInfo,
	state appstate.HashState,
	respCollection waBinary.Node,
	allowRetry bool,
) error {
	errorTagNode, ok := respCollection.GetOptionalChildByTag(errorTag)

	mainErr := fmt.Errorf("%w: %s", ErrUpdate, respCollection.XMLString())
	if ok {
		mainErr = fmt.Errorf("%w (%s): %s", ErrUpdate, patch.Type, errorTagNode.XMLString())
	}
	if ok && errorTagNode.AttrGetter().Int(attrCode) == conflictCode && allowRetry {
		zerolog.Ctx(ctx).Warn().Err(mainErr).Msg("Failed to update app state, trying to apply conflicts and retry")
		var eventsToDispatch []any
		patches, err := appstate.ParsePatchList(ctx, &respCollection, t.DownloadExternalBlob)
		if err != nil {
			return fmt.Errorf("%w (also, parsing patches in the response failed: %w)", mainErr, err)
		} else if _, err = ApplyPatches(ctx, t, patch.Type, state, patches, false, &eventsToDispatch); err != nil {
			return fmt.Errorf("%w (also, applying patches in the response failed: %w)", mainErr, err)
		} else {
			zerolog.Ctx(ctx).Debug().Msg("Retrying app state send after applying conflicting patches")
			go dispatchAll(t, eventsToDispatch)
			return Send(ctx, t, patch, false)
		}
	}
	return mainErr
}

// dispatchAll entrega uma lista de eventos aos handlers. Os chamadores a
// disparam em goroutine porque o despacho pode bloquear em handler lento e o
// envio nao deve esperar por isso.
func dispatchAll(t Transport, evts []any) {
	for _, evt := range evts {
		t.DispatchEvent(evt)
	}
}

// MarkNotDirty marca uma colecao como "nao suja" no servidor. E' um protocolo
// separado do de app state, apesar de ser disparado pelas mesmas notificacoes.
func MarkNotDirty(ctx context.Context, t Transport, cleanType string, ts time.Time) error {
	_, err := t.SendIQ(ctx, IQ{
		Namespace: dirtyNamespace,
		Type:      IQSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: dirtyCleanTag,
			Attrs: waBinary.Attrs{
				dirtyCleanAttrType:      cleanType,
				dirtyCleanAttrTimestamp: ts.Unix(),
			},
		}},
	})
	return err
}
