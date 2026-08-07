package appstatesync

import (
	"context"
	"encoding/hex"
	"time"

	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
)

// RequestMissingKeys pede ao dispositivo primario as chaves de app state que
// faltam para decodificar os patches dados, respeitando o throttle de
// KeyRequestInterval por chave.
//
// O calculo das chaves faltantes acontece DENTRO do lock (ver
// State.FilterKeyIDs); o envio, fora — como no codigo original.
func RequestMissingKeys(ctx context.Context, t Transport, patches *appstate.PatchList) {
	filteredKeyIDs := t.State().FilterKeyIDs(time.Now(), func() [][]byte {
		return t.Proc().GetMissingKeyIDs(ctx, patches)
	})
	RequestKeys(ctx, t, filteredKeyIDs)
}

// RequestKeys manda o pedido das chaves dadas ao dispositivo primario. Uma
// lista vazia nao gera envio nenhum: sem essa guarda, todo patch que falhasse
// por outro motivo mandaria uma mensagem de peer com zero key IDs.
func RequestKeys(ctx context.Context, t Transport, rawKeyIDs [][]byte) {
	keyIDs := make([]*waE2E.AppStateSyncKeyId, len(rawKeyIDs))
	debugKeyIDs := make([]string, len(rawKeyIDs))
	for i, keyID := range rawKeyIDs {
		keyIDs[i] = &waE2E.AppStateSyncKeyId{KeyID: keyID}
		debugKeyIDs[i] = hex.EncodeToString(keyID)
	}
	msg := &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_APP_STATE_SYNC_KEY_REQUEST.Enum(),
			AppStateSyncKeyRequest: &waE2E.AppStateSyncKeyRequest{
				KeyIDs: keyIDs,
			},
		},
	}
	if len(debugKeyIDs) == 0 {
		return
	}
	t.Log().Infof("Sending key request for app state keys %+v", debugKeyIDs)
	err := t.SendPeerMessage(ctx, msg)
	if err != nil {
		t.Log().Warnf("Failed to send app state key request: %v", err)
	}
}
