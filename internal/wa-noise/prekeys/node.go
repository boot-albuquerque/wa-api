// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package prekeys

import (
	"encoding/binary"
	"fmt"

	"go.mau.fi/libsignal/ecc"
	"go.mau.fi/libsignal/keys/identity"
	"go.mau.fi/libsignal/keys/prekey"
	"go.mau.fi/libsignal/util/optional"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/util/keys"
)

// ToNode serializa uma prekey para o no <key> (ou <skey>, quando ela tem
// assinatura) que o servidor espera. Era preKeyToNode na raiz.
func ToNode(key *keys.PreKey) waBinary.Node {
	var keyID [RegistrationIDLength]byte
	binary.BigEndian.PutUint32(keyID[:], key.KeyID)
	node := waBinary.Node{
		Tag: "key",
		Content: []waBinary.Node{
			{Tag: "id", Content: keyID[idPadLength:]},
			{Tag: "value", Content: key.Pub[:]},
		},
	}
	if key.Signature != nil {
		node.Tag = "skey"
		node.Content = append(node.GetChildren(), waBinary.Node{
			Tag:     "signature",
			Content: key.Signature[:],
		})
	}
	return node
}

// ToNodes serializa uma lista de prekeys. Era preKeysToNodes na raiz.
func ToNodes(prekeys []*keys.PreKey) []waBinary.Node {
	nodes := make([]waBinary.Node, len(prekeys))
	for i, key := range prekeys {
		nodes[i] = ToNode(key)
	}
	return nodes
}

// NodeToBundle le' o <user> de uma resposta de busca de prekeys e monta o
// bundle Signal correspondente. Era nodeToPreKeyBundle na raiz.
func NodeToBundle(deviceID uint32, node waBinary.Node) (*prekey.Bundle, error) {
	errorNode, ok := node.GetOptionalChildByTag("error")
	if ok && errorNode.Tag == "error" {
		return nil, fmt.Errorf("got error getting prekeys: %s", errorNode.XMLString())
	}

	registrationBytes, ok := node.GetChildByTag("registration").Content.([]byte)
	if !ok || len(registrationBytes) != RegistrationIDLength {
		return nil, fmt.Errorf("invalid registration ID in prekey response")
	}
	registrationID := binary.BigEndian.Uint32(registrationBytes)

	keysNode, ok := node.GetOptionalChildByTag("keys")
	if !ok {
		keysNode = node
	}

	identityKeyRaw, ok := keysNode.GetChildByTag("identity").Content.([]byte)
	if !ok || len(identityKeyRaw) != pubLength {
		return nil, fmt.Errorf("invalid identity key in prekey response")
	}
	identityKeyPub := *(*[pubLength]byte)(identityKeyRaw)

	preKeyNode, ok := keysNode.GetOptionalChildByTag("key")
	preKey := &keys.PreKey{}
	if ok {
		var err error
		preKey, err = NodeToPreKey(preKeyNode)
		if err != nil {
			return nil, fmt.Errorf("invalid prekey in prekey response: %w", err)
		}
	}

	signedPreKey, err := NodeToPreKey(keysNode.GetChildByTag("skey"))
	if err != nil {
		return nil, fmt.Errorf("invalid signed prekey in prekey response: %w", err)
	}

	var bundle *prekey.Bundle
	if ok {
		bundle = prekey.NewBundle(registrationID, deviceID,
			optional.NewOptionalUint32(preKey.KeyID), signedPreKey.KeyID,
			ecc.NewDjbECPublicKey(*preKey.Pub), ecc.NewDjbECPublicKey(*signedPreKey.Pub), *signedPreKey.Signature,
			identity.NewKey(ecc.NewDjbECPublicKey(identityKeyPub)))
	} else {
		bundle = prekey.NewBundle(registrationID, deviceID, optional.NewEmptyUint32(), signedPreKey.KeyID,
			nil, ecc.NewDjbECPublicKey(*signedPreKey.Pub), *signedPreKey.Signature,
			identity.NewKey(ecc.NewDjbECPublicKey(identityKeyPub)))
	}

	return bundle, nil
}

// NodeToPreKey le' um no <key>/<skey> e devolve a prekey. Era nodeToPreKey na
// raiz.
func NodeToPreKey(node waBinary.Node) (*keys.PreKey, error) {
	key := keys.PreKey{
		KeyPair:   keys.KeyPair{},
		KeyID:     0,
		Signature: nil,
	}
	if id := node.GetChildByTag("id"); id.Tag != "id" {
		return nil, fmt.Errorf("prekey node doesn't contain ID tag")
	} else if idBytes, ok := id.Content.([]byte); !ok {
		return nil, fmt.Errorf("prekey ID has unexpected content (%T)", id.Content)
	} else if len(idBytes) != idLength {
		return nil, fmt.Errorf("prekey ID has unexpected number of bytes (%d, expected %d)", len(idBytes), idLength)
	} else {
		key.KeyID = binary.BigEndian.Uint32(append(make([]byte, idPadLength), idBytes...))
	}
	if pubkey := node.GetChildByTag("value"); pubkey.Tag != "value" {
		return nil, fmt.Errorf("prekey node doesn't contain value tag")
	} else if pubkeyBytes, ok := pubkey.Content.([]byte); !ok {
		return nil, fmt.Errorf("prekey value has unexpected content (%T)", pubkey.Content)
	} else if len(pubkeyBytes) != pubLength {
		return nil, fmt.Errorf("prekey value has unexpected number of bytes (%d, expected %d)", len(pubkeyBytes), pubLength)
	} else {
		key.KeyPair.Pub = (*[pubLength]byte)(pubkeyBytes)
	}
	if node.Tag == "skey" {
		if sig := node.GetChildByTag("signature"); sig.Tag != "signature" {
			return nil, fmt.Errorf("prekey node doesn't contain signature tag")
		} else if sigBytes, ok := sig.Content.([]byte); !ok {
			return nil, fmt.Errorf("prekey signature has unexpected content (%T)", sig.Content)
		} else if len(sigBytes) != signatureLength {
			return nil, fmt.Errorf("prekey signature has unexpected number of bytes (%d, expected %d)", len(sigBytes), signatureLength)
		} else {
			key.Signature = (*[signatureLength]byte)(sigBytes)
		}
	}
	return &key, nil
}
