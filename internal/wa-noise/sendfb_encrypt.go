// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/libsignal/keys/prekey"
	"go.mau.fi/libsignal/protocol"
	"go.mau.fi/libsignal/session"
	"go.mau.fi/libsignal/signalerror"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waMsgTransport"
	"wa-api/internal/wa-noise/types"
)

func (cli *Client) encryptMessageForDevicesV3(
	ctx context.Context,
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
	existingSessions, ctx, err := cli.Store.WithCachedSessions(ctx, sessionAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to prefetch sessions: %w", err)
	}
	var retryDevices []types.JID
	for addr, exists := range existingSessions {
		if !exists {
			retryDevices = append(retryDevices, sessionAddressToJID[addr])
		}
	}
	bundles := cli.fetchPreKeysNoError(ctx, retryDevices)

	for _, jid := range allDevices {
		var dsmForDevice *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage
		if jid.User == ownID.User {
			if jid == ownID {
				continue
			}
			dsmForDevice = dsm
		}
		encrypted, err := cli.encryptMessageForDeviceAndWrapV3(ctx, payload, skdm, dsmForDevice, jid, bundles[jid], encAttrs)
		if err != nil {
			// TODO return these errors if it's a fatal one (like context cancellation or database)
			cli.Log.Warnf("Failed to encrypt %s for %s: %v", id, jid, err)
			if ctx.Err() != nil {
				return nil, err
			}
			continue
		}
		participantNodes = append(participantNodes, *encrypted)
	}
	err = cli.Store.PutCachedSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to save cached sessions: %w", err)
	}
	return participantNodes, nil
}

func (cli *Client) encryptMessageForDeviceAndWrapV3(
	ctx context.Context,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	to types.JID,
	bundle *prekey.Bundle,
	encAttrs waBinary.Attrs,
) (*waBinary.Node, error) {
	node, err := cli.encryptMessageForDeviceV3(ctx, payload, skdm, dsm, to, bundle, encAttrs)
	if err != nil {
		return nil, err
	}
	return &waBinary.Node{
		Tag:     "to",
		Attrs:   waBinary.Attrs{"jid": to},
		Content: []waBinary.Node{*node},
	}, nil
}

func (cli *Client) encryptMessageForDeviceV3(
	ctx context.Context,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	to types.JID,
	bundle *prekey.Bundle,
	extraAttrs waBinary.Attrs,
) (*waBinary.Node, error) {
	builder := session.NewBuilderFromSignal(cli.Store, to.SignalAddress(), pbSerializer)
	if bundle != nil {
		cli.Log.Debugf("Processing prekey bundle for %s", to)
		err := builder.ProcessBundle(ctx, bundle)
		if cli.AutoTrustIdentity && errors.Is(err, signalerror.ErrUntrustedIdentity) {
			cli.Log.Warnf("Got %v error while trying to process prekey bundle for %s, clearing stored identity and retrying", err, to)
			err = cli.clearUntrustedIdentity(ctx, to)
			if err != nil {
				return nil, fmt.Errorf("failed to clear untrusted identity: %w", err)
			}
			err = builder.ProcessBundle(ctx, bundle)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to process prekey bundle: %w", err)
		}
	} else if contains, err := cli.Store.ContainsSession(ctx, to.SignalAddress()); err != nil {
		return nil, err
	} else if !contains {
		return nil, ErrNoSession
	}
	cipher := session.NewCipher(builder, to.SignalAddress())
	plaintext, err := proto.Marshal(&waMsgTransport.MessageTransport{
		Payload: payload,
		Protocol: &waMsgTransport.MessageTransport_Protocol{
			Integral: &waMsgTransport.MessageTransport_Protocol_Integral{
				Padding: padMessage(nil),
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

	encAttrs := waBinary.Attrs{
		"v":    FBMessageVersion,
		"type": "msg",
	}
	if ciphertext.Type() == protocol.PREKEY_TYPE {
		encAttrs["type"] = "pkmsg"
	}
	copyAttrs(extraAttrs, encAttrs)

	return &waBinary.Node{
		Tag:     "enc",
		Attrs:   encAttrs,
		Content: ciphertext.Serialize(),
	}, nil
}
