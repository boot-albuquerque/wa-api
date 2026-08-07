// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package paircrypto

import (
	"go.mau.fi/libsignal/ecc"

	"wa-api/internal/wa-noise/protocol/proto/waAdv"
	"wa-api/internal/wa-noise/security/keys"
)

var (
	AdvAccountSignaturePrefix = []byte{6, 0}
	AdvDeviceSignaturePrefix  = []byte{6, 1}

	AdvHostedAccountSignaturePrefix = []byte{6, 5}
	AdvHostedDeviceSignaturePrefix  = []byte{6, 6}
)

func ConcatBytes(data ...[]byte) []byte {
	length := 0
	for _, item := range data {
		length += len(item)
	}
	output := make([]byte, length)
	ptr := 0
	for _, item := range data {
		ptr += copy(output[ptr:ptr+len(item)], item)
	}
	return output
}

func VerifyAccountSignature(deviceIdentity *waAdv.ADVSignedDeviceIdentity, ikp *keys.KeyPair, isHosted bool) bool {
	if len(deviceIdentity.AccountSignatureKey) != 32 || len(deviceIdentity.AccountSignature) != 64 {
		return false
	}

	signatureKey := ecc.NewDjbECPublicKey(*(*[32]byte)(deviceIdentity.AccountSignatureKey))
	signature := *(*[64]byte)(deviceIdentity.AccountSignature)

	prefix := AdvAccountSignaturePrefix
	if isHosted {
		prefix = AdvHostedAccountSignaturePrefix
	}
	message := ConcatBytes(prefix, deviceIdentity.Details, ikp.Pub[:])

	return ecc.VerifySignature(signatureKey, message, signature)
}

func GenerateDeviceSignature(deviceIdentity *waAdv.ADVSignedDeviceIdentity, ikp *keys.KeyPair) *[64]byte {
	prefix := AdvDeviceSignaturePrefix
	message := ConcatBytes(prefix, deviceIdentity.Details, ikp.Pub[:], deviceIdentity.AccountSignatureKey)
	sig := ecc.CalculateSignature(ecc.NewDjbECPrivateKey(*ikp.Priv), message)
	return &sig
}
