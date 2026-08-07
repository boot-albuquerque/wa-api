package send

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/libsignal/keys/prekey"
	"go.mau.fi/libsignal/protocol"
	"go.mau.fi/libsignal/session"
	"go.mau.fi/libsignal/signalerror"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/msgpad"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// Este arquivo e' o caminho de CIFRAGEM waE2E. A extracao foi deliberadamente
// mecanica: cada `cli.X` virou `t.X`, e nada mais mudou — nem a ordem das
// operacoes, nem o tratamento de erro, nem os atributos escritos no <enc>.
// Ver PATCHES.md, "Fase F/G — lote 8", secao de conservadorismo.

// EncryptForDevices cifra a mensagem para todos os dispositivos dados,
// devolvendo os nos <to> por dispositivo e se o <device-identity> precisa
// acompanhar o stanza. Era Client.encryptMessageForDevices.
func EncryptForDevices(
	ctx context.Context,
	t Transport,
	allDevices []types.JID,
	id string,
	msgPlaintext, dsmPlaintext []byte,
	encAttrs waBinary.Attrs,
) ([]waBinary.Node, bool, error) {
	ownJID := t.OwnID()
	ownLID := t.OwnLID()
	includeIdentity := false
	participantNodes := make([]waBinary.Node, 0, len(allDevices))

	var pnDevices []types.JID
	for _, jid := range allDevices {
		if jid.Server == types.DefaultUserServer {
			pnDevices = append(pnDevices, jid)
		}
	}
	lidMappings, err := t.Store().LIDs.GetManyLIDsForPNs(ctx, pnDevices)
	if err != nil {
		return nil, false, fmt.Errorf("failed to fetch LID mappings: %w", err)
	}

	encryptionIdentities := make(map[types.JID]types.JID, len(allDevices))
	sessionAddressToJID := make(map[string]types.JID, len(allDevices))
	sessionAddresses := make([]string, 0, len(allDevices))
	for _, jid := range allDevices {
		encryptionIdentity := jid
		if jid.Server == types.DefaultUserServer {
			// TODO query LID from server for missing entries
			if lidForPN, ok := lidMappings[jid]; ok && !lidForPN.IsEmpty() {
				t.MigrateSessionStore(ctx, jid, lidForPN)
				encryptionIdentity = lidForPN
			}
		}
		encryptionIdentities[jid] = encryptionIdentity
		addr := encryptionIdentity.SignalAddress().String()
		sessionAddresses = append(sessionAddresses, addr)
		sessionAddressToJID[addr] = jid
	}

	existingSessions, ctx, err := t.Store().WithCachedSessions(ctx, sessionAddresses)
	if err != nil {
		return nil, false, fmt.Errorf("failed to prefetch sessions: %w", err)
	}
	var retryDevices []types.JID
	for addr, exists := range existingSessions {
		if !exists {
			retryDevices = append(retryDevices, sessionAddressToJID[addr])
		}
	}
	bundles := t.FetchPreKeysNoError(ctx, retryDevices)

	for _, jid := range allDevices {
		plaintext := msgPlaintext
		if (jid.User == ownJID.User || jid.User == ownLID.User) && dsmPlaintext != nil {
			if jid == ownJID || jid == ownLID {
				continue
			}
			plaintext = dsmPlaintext
		}
		encrypted, isPreKey, err := encryptForDeviceAndWrap(
			ctx, t, plaintext, jid, encryptionIdentities[jid], bundles[jid], encAttrs, existingSessions,
		)
		if err != nil {
			// TODO return these errors if it's a fatal one (like context cancellation or database)
			t.Log().Warnf("Failed to encrypt %s for %s: %v", id, jid, err)
			if ctx.Err() != nil {
				return nil, false, err
			}
			continue
		}

		participantNodes = append(participantNodes, *encrypted)
		if isPreKey {
			includeIdentity = true
		}
	}
	err = t.Store().PutCachedSessions(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to save cached sessions: %w", err)
	}
	return participantNodes, includeIdentity, nil
}

// EncryptForDeviceAndWrap cifra para um dispositivo e embrulha o <enc> no <to>
// enderecado a wireIdentity. Era Client.encryptMessageForDeviceAndWrap.
func EncryptForDeviceAndWrap(
	ctx context.Context,
	t Transport,
	plaintext []byte,
	wireIdentity,
	encryptionIdentity types.JID,
	bundle *prekey.Bundle,
	encAttrs waBinary.Attrs,
	existingSessions map[string]bool,
) (*waBinary.Node, bool, error) {
	return encryptForDeviceAndWrap(
		ctx, t, plaintext, wireIdentity, encryptionIdentity, bundle, encAttrs, existingSessions,
	)
}

func encryptForDeviceAndWrap(
	ctx context.Context,
	t Transport,
	plaintext []byte,
	wireIdentity,
	encryptionIdentity types.JID,
	bundle *prekey.Bundle,
	encAttrs waBinary.Attrs,
	existingSessions map[string]bool,
) (*waBinary.Node, bool, error) {
	node, includeDeviceIdentity, err := EncryptForDevice(
		ctx, t, plaintext, encryptionIdentity, bundle, encAttrs, existingSessions,
	)
	if err != nil {
		return nil, false, err
	}
	return &waBinary.Node{
		Tag:     participantToNodeTag,
		Attrs:   waBinary.Attrs{participantToAttrJID: wireIdentity},
		Content: []waBinary.Node{*node},
	}, includeDeviceIdentity, nil
}

// CopyAttrs copia atributos de `from` para `to`, sobrescrevendo. Era copyAttrs.
func CopyAttrs(from, to waBinary.Attrs) {
	copyAttrs(from, to)
}

func copyAttrs(from, to waBinary.Attrs) {
	for k, v := range from {
		to[k] = v
	}
}

// EncryptForDevice cifra um payload waE2E para um dispositivo e monta o <enc>.
// Era Client.encryptMessageForDevice.
func EncryptForDevice(
	ctx context.Context,
	t Transport,
	plaintext []byte,
	to types.JID,
	bundle *prekey.Bundle,
	extraAttrs waBinary.Attrs,
	existingSessions map[string]bool,
) (*waBinary.Node, bool, error) {
	builder := session.NewBuilderFromSignal(t.Store(), to.SignalAddress(), store.SignalProtobufSerializer)
	if bundle != nil {
		t.Log().Debugf("Processing prekey bundle for %s", to)
		err := builder.ProcessBundle(ctx, bundle)
		if t.AutoTrustIdentity() && errors.Is(err, signalerror.ErrUntrustedIdentity) {
			t.Log().Warnf("Got %v error while trying to process prekey bundle for %s, clearing stored identity and retrying", err, to)
			err = t.ClearUntrustedIdentity(ctx, to)
			if err != nil {
				return nil, false, fmt.Errorf("failed to clear untrusted identity: %w", err)
			}
			err = builder.ProcessBundle(ctx, bundle)
		}
		if err != nil {
			return nil, false, fmt.Errorf("failed to process prekey bundle: %w", err)
		}
	} else {
		sessionExists, checked := existingSessions[to.SignalAddress().String()]
		if !checked {
			var err error
			sessionExists, err = t.Store().ContainsSession(ctx, to.SignalAddress())
			if err != nil {
				return nil, false, err
			}
		}
		if !sessionExists {
			return nil, false, fmt.Errorf("%w with %s", ErrNoSession, to.SignalAddress().String())
		}
	}
	cipher := session.NewCipher(builder, to.SignalAddress())
	ciphertext, err := cipher.Encrypt(ctx, msgpad.Pad(plaintext))
	if err != nil {
		return nil, false, fmt.Errorf("cipher encryption failed: %w", err)
	}

	encAttrs := waBinary.Attrs{
		encAttrVersion: encVersionSignal,
		encAttrType:    encTypeMsg,
	}
	if ciphertext.Type() == protocol.PREKEY_TYPE {
		encAttrs[encAttrType] = encTypePreKeyMsg
	}
	copyAttrs(extraAttrs, encAttrs)

	includeDeviceIdentity := encAttrs[encAttrType] == encTypePreKeyMsg && !t.IsMessenger()
	return &waBinary.Node{
		Tag:     encNodeTag,
		Attrs:   encAttrs,
		Content: ciphertext.Serialize(),
	}, includeDeviceIdentity, nil
}
