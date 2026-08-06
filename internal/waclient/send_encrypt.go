// Copyright (c) 2022 Tulir Asokan
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

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/types"
)

func (cli *Client) encryptMessageForDevices(
	ctx context.Context,
	allDevices []types.JID,
	id string,
	msgPlaintext, dsmPlaintext []byte,
	encAttrs waBinary.Attrs,
) ([]waBinary.Node, bool, error) {
	ownJID := cli.getOwnID()
	ownLID := cli.getOwnLID()
	includeIdentity := false
	participantNodes := make([]waBinary.Node, 0, len(allDevices))

	var pnDevices []types.JID
	for _, jid := range allDevices {
		if jid.Server == types.DefaultUserServer {
			pnDevices = append(pnDevices, jid)
		}
	}
	lidMappings, err := cli.Store.LIDs.GetManyLIDsForPNs(ctx, pnDevices)
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
				cli.migrateSessionStore(ctx, jid, lidForPN)
				encryptionIdentity = lidForPN
			}
		}
		encryptionIdentities[jid] = encryptionIdentity
		addr := encryptionIdentity.SignalAddress().String()
		sessionAddresses = append(sessionAddresses, addr)
		sessionAddressToJID[addr] = jid
	}

	existingSessions, ctx, err := cli.Store.WithCachedSessions(ctx, sessionAddresses)
	if err != nil {
		return nil, false, fmt.Errorf("failed to prefetch sessions: %w", err)
	}
	var retryDevices []types.JID
	for addr, exists := range existingSessions {
		if !exists {
			retryDevices = append(retryDevices, sessionAddressToJID[addr])
		}
	}
	bundles := cli.fetchPreKeysNoError(ctx, retryDevices)

	for _, jid := range allDevices {
		plaintext := msgPlaintext
		if (jid.User == ownJID.User || jid.User == ownLID.User) && dsmPlaintext != nil {
			if jid == ownJID || jid == ownLID {
				continue
			}
			plaintext = dsmPlaintext
		}
		encrypted, isPreKey, err := cli.encryptMessageForDeviceAndWrap(
			ctx, plaintext, jid, encryptionIdentities[jid], bundles[jid], encAttrs, existingSessions,
		)
		if err != nil {
			// TODO return these errors if it's a fatal one (like context cancellation or database)
			cli.Log.Warnf("Failed to encrypt %s for %s: %v", id, jid, err)
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
	err = cli.Store.PutCachedSessions(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to save cached sessions: %w", err)
	}
	return participantNodes, includeIdentity, nil
}

func (cli *Client) encryptMessageForDeviceAndWrap(
	ctx context.Context,
	plaintext []byte,
	wireIdentity,
	encryptionIdentity types.JID,
	bundle *prekey.Bundle,
	encAttrs waBinary.Attrs,
	existingSessions map[string]bool,
) (*waBinary.Node, bool, error) {
	node, includeDeviceIdentity, err := cli.encryptMessageForDevice(
		ctx, plaintext, encryptionIdentity, bundle, encAttrs, existingSessions,
	)
	if err != nil {
		return nil, false, err
	}
	return &waBinary.Node{
		Tag:     "to",
		Attrs:   waBinary.Attrs{"jid": wireIdentity},
		Content: []waBinary.Node{*node},
	}, includeDeviceIdentity, nil
}

func copyAttrs(from, to waBinary.Attrs) {
	for k, v := range from {
		to[k] = v
	}
}

func (cli *Client) encryptMessageForDevice(
	ctx context.Context,
	plaintext []byte,
	to types.JID,
	bundle *prekey.Bundle,
	extraAttrs waBinary.Attrs,
	existingSessions map[string]bool,
) (*waBinary.Node, bool, error) {
	builder := session.NewBuilderFromSignal(cli.Store, to.SignalAddress(), pbSerializer)
	if bundle != nil {
		cli.Log.Debugf("Processing prekey bundle for %s", to)
		err := builder.ProcessBundle(ctx, bundle)
		if cli.AutoTrustIdentity && errors.Is(err, signalerror.ErrUntrustedIdentity) {
			cli.Log.Warnf("Got %v error while trying to process prekey bundle for %s, clearing stored identity and retrying", err, to)
			err = cli.clearUntrustedIdentity(ctx, to)
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
			sessionExists, err = cli.Store.ContainsSession(ctx, to.SignalAddress())
			if err != nil {
				return nil, false, err
			}
		}
		if !sessionExists {
			return nil, false, fmt.Errorf("%w with %s", ErrNoSession, to.SignalAddress().String())
		}
	}
	cipher := session.NewCipher(builder, to.SignalAddress())
	ciphertext, err := cipher.Encrypt(ctx, padMessage(plaintext))
	if err != nil {
		return nil, false, fmt.Errorf("cipher encryption failed: %w", err)
	}

	encAttrs := waBinary.Attrs{
		"v":    "2",
		"type": "msg",
	}
	if ciphertext.Type() == protocol.PREKEY_TYPE {
		encAttrs["type"] = "pkmsg"
	}
	copyAttrs(extraAttrs, encAttrs)

	includeDeviceIdentity := encAttrs["type"] == "pkmsg" && cli.MessengerConfig == nil
	return &waBinary.Node{
		Tag:     "enc",
		Attrs:   encAttrs,
		Content: ciphertext.Serialize(),
	}, includeDeviceIdentity, nil
}
