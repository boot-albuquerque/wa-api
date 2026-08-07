package send

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/libsignal/keys/prekey"
	"go.mau.fi/libsignal/protocol"
	"go.mau.fi/libsignal/session"
	"go.mau.fi/libsignal/signalerror"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/msgpad"
	"wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// Este arquivo e' o caminho de CIFRAGEM v3/FB. Como encrypt.go, a extracao foi
// deliberadamente mecanica: cada `cli.X` virou `t.X` e nada mais mudou. Em
// particular NAO foi "harmonizado" com o caminho waE2E, apesar das quatro
// diferencas visiveis entre os dois (sem prefetch de LID, sem
// existingSessions no teste de sessao, ErrNoSession sem o endereco no texto, e
// o `v` numerico em vez de string). Cada uma dessas diferencas ou vai para o
// wire ou muda um erro observavel; nivela-las seria mudanca de comportamento
// disfarcada de limpeza. Ver PATCHES.md, "Fase F/G — lote 8".

// EncryptForDevicesV3 cifra um payload de transporte FB para todos os
// dispositivos dados. Era Client.encryptMessageForDevicesV3.
func EncryptForDevicesV3(
	ctx context.Context,
	t Transport,
	allDevices []types.JID,
	ownID types.JID,
	id string,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	encAttrs waBinary.Attrs,
) ([]waBinary.Node, error) {
	participantNodes := make([]waBinary.Node, 0, len(allDevices))

	sessionAddressToJID := make(map[string]types.JID, len(allDevices))
	sessionAddresses := make([]string, 0, len(allDevices))
	for _, jid := range allDevices {
		addr := jid.SignalAddress().String()
		sessionAddresses = append(sessionAddresses, addr)
		sessionAddressToJID[addr] = jid
	}
	existingSessions, ctx, err := t.Store().WithCachedSessions(ctx, sessionAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to prefetch sessions: %w", err)
	}
	var retryDevices []types.JID
	for addr, exists := range existingSessions {
		if !exists {
			retryDevices = append(retryDevices, sessionAddressToJID[addr])
		}
	}
	bundles := t.FetchPreKeysNoError(ctx, retryDevices)

	for _, jid := range allDevices {
		var dsmForDevice *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage
		if jid.User == ownID.User {
			if jid == ownID {
				continue
			}
			dsmForDevice = dsm
		}
		encrypted, err := encryptForDeviceAndWrapV3(ctx, t, payload, skdm, dsmForDevice, jid, bundles[jid], encAttrs)
		if err != nil {
			// TODO return these errors if it's a fatal one (like context cancellation or database)
			t.Log().Warnf("Failed to encrypt %s for %s: %v", id, jid, err)
			if ctx.Err() != nil {
				return nil, err
			}
			continue
		}
		participantNodes = append(participantNodes, *encrypted)
	}
	err = t.Store().PutCachedSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to save cached sessions: %w", err)
	}
	return participantNodes, nil
}

// EncryptForDeviceAndWrapV3 cifra para um dispositivo e embrulha o <enc> no
// <to>. Era Client.encryptMessageForDeviceAndWrapV3.
func EncryptForDeviceAndWrapV3(
	ctx context.Context,
	t Transport,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	to types.JID,
	bundle *prekey.Bundle,
	encAttrs waBinary.Attrs,
) (*waBinary.Node, error) {
	return encryptForDeviceAndWrapV3(ctx, t, payload, skdm, dsm, to, bundle, encAttrs)
}

func encryptForDeviceAndWrapV3(
	ctx context.Context,
	t Transport,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	to types.JID,
	bundle *prekey.Bundle,
	encAttrs waBinary.Attrs,
) (*waBinary.Node, error) {
	node, err := EncryptForDeviceV3(ctx, t, payload, skdm, dsm, to, bundle, encAttrs)
	if err != nil {
		return nil, err
	}
	return &waBinary.Node{
		Tag:     participantToNodeTag,
		Attrs:   waBinary.Attrs{participantToAttrJID: to},
		Content: []waBinary.Node{*node},
	}, nil
}

// EncryptForDeviceV3 cifra um payload de transporte FB para um dispositivo e
// monta o <enc>. Era Client.encryptMessageForDeviceV3.
func EncryptForDeviceV3(
	ctx context.Context,
	t Transport,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	to types.JID,
	bundle *prekey.Bundle,
	extraAttrs waBinary.Attrs,
) (*waBinary.Node, error) {
	builder := session.NewBuilderFromSignal(t.Store(), to.SignalAddress(), store.SignalProtobufSerializer)
	if bundle != nil {
		t.Log().Debugf("Processing prekey bundle for %s", to)
		err := builder.ProcessBundle(ctx, bundle)
		if t.AutoTrustIdentity() && errors.Is(err, signalerror.ErrUntrustedIdentity) {
			t.Log().Warnf("Got %v error while trying to process prekey bundle for %s, clearing stored identity and retrying", err, to)
			err = t.ClearUntrustedIdentity(ctx, to)
			if err != nil {
				return nil, fmt.Errorf("failed to clear untrusted identity: %w", err)
			}
			err = builder.ProcessBundle(ctx, bundle)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to process prekey bundle: %w", err)
		}
	} else if contains, err := t.Store().ContainsSession(ctx, to.SignalAddress()); err != nil {
		return nil, err
	} else if !contains {
		return nil, ErrNoSession
	}
	cipher := session.NewCipher(builder, to.SignalAddress())
	plaintext, err := proto.Marshal(&waMsgTransport.MessageTransport{
		Payload: payload,
		Protocol: &waMsgTransport.MessageTransport_Protocol{
			Integral: &waMsgTransport.MessageTransport_Protocol_Integral{
				Padding: msgpad.Pad(nil),
				DSM:     dsm,
			},
			Ancillary: &waMsgTransport.MessageTransport_Protocol_Ancillary{
				Skdm:               skdm,
				DeviceListMetadata: nil,
				Icdc:               nil,
				BackupDirective:    nil,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message transport: %w", err)
	}
	ciphertext, err := cipher.Encrypt(ctx, plaintext)
	if err != nil {
		return nil, fmt.Errorf("cipher encryption failed: %w", err)
	}

	// NOTE: unlike the waE2E path, `v` here is the *numeric* FBMessageVersion,
	// not a string. Kept verbatim: it is what goes on the wire.
	encAttrs := waBinary.Attrs{
		encAttrVersion: FBMessageVersion,
		encAttrType:    encTypeMsg,
	}
	if ciphertext.Type() == protocol.PREKEY_TYPE {
		encAttrs[encAttrType] = encTypePreKeyMsg
	}
	copyAttrs(extraAttrs, encAttrs)

	return &waBinary.Node{
		Tag:     encNodeTag,
		Attrs:   encAttrs,
		Content: ciphertext.Serialize(),
	}, nil
}
