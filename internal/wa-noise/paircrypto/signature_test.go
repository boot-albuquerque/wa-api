// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package paircrypto

import (
	"bytes"
	"testing"

	"go.mau.fi/libsignal/ecc"

	"wa-api/internal/wa-noise/proto/waAdv"
	"wa-api/internal/wa-noise/util/keys"
)

func TestConcatBytesJoinsInOrder(t *testing.T) {
	got := ConcatBytes([]byte{1, 2}, nil, []byte{3}, []byte{})
	if want := []byte{1, 2, 3}; !bytes.Equal(got, want) {
		t.Errorf("= %v, esperado %v", got, want)
	}
}

func TestConcatBytesWithNoArgumentsIsEmpty(t *testing.T) {
	if got := ConcatBytes(); len(got) != 0 {
		t.Errorf("= %v, esperado vazio", got)
	}
}

// A assinatura gerada aqui e' conferida pelo servidor com a chave publica do
// device; se o prefixo ou a ordem da concatenacao mudar, o pareamento quebra.
func TestGenerateDeviceSignatureVerifiesAgainstTheDevicePublicKey(t *testing.T) {
	ikp := keys.NewKeyPair()
	identity := &waAdv.ADVSignedDeviceIdentity{
		Details:             []byte("detalhes"),
		AccountSignatureKey: bytes.Repeat([]byte{7}, 32),
	}

	sig := GenerateDeviceSignature(identity, ikp)

	message := ConcatBytes(AdvDeviceSignaturePrefix, identity.Details, ikp.Pub[:], identity.AccountSignatureKey)
	if !ecc.VerifySignature(ecc.NewDjbECPublicKey(*ikp.Pub), message, *sig) {
		t.Error("a assinatura gerada nao verifica com a propria chave publica")
	}
}

func TestVerifyAccountSignatureRejectsMalformedIdentities(t *testing.T) {
	ikp := keys.NewKeyPair()
	for name, identity := range map[string]*waAdv.ADVSignedDeviceIdentity{
		"chave curta":      {AccountSignatureKey: []byte{1}, AccountSignature: bytes.Repeat([]byte{2}, 64)},
		"assinatura curta": {AccountSignatureKey: bytes.Repeat([]byte{1}, 32), AccountSignature: []byte{2}},
		"ambos vazios":     {},
	} {
		if VerifyAccountSignature(identity, ikp, false) {
			t.Errorf("%s: aceitou identidade malformada", name)
		}
	}
}

// Uma assinatura de conta valida so' verifica sob o prefixo com que foi feita:
// hosted e nao-hosted nao podem se aceitar mutuamente.
func TestVerifyAccountSignatureIsBoundToTheHostedPrefix(t *testing.T) {
	ikp := keys.NewKeyPair()
	accountKP := keys.NewKeyPair()
	details := []byte("detalhes")

	sign := func(prefix []byte) []byte {
		sig := ecc.CalculateSignature(
			ecc.NewDjbECPrivateKey(*accountKP.Priv),
			ConcatBytes(prefix, details, ikp.Pub[:]),
		)
		return sig[:]
	}

	plain := &waAdv.ADVSignedDeviceIdentity{
		Details:             details,
		AccountSignatureKey: accountKP.Pub[:],
		AccountSignature:    sign(AdvAccountSignaturePrefix),
	}
	if !VerifyAccountSignature(plain, ikp, false) {
		t.Error("assinatura nao-hosted valida foi recusada")
	}
	if VerifyAccountSignature(plain, ikp, true) {
		t.Error("assinatura nao-hosted foi aceita como hosted")
	}

	hosted := &waAdv.ADVSignedDeviceIdentity{
		Details:             details,
		AccountSignatureKey: accountKP.Pub[:],
		AccountSignature:    sign(AdvHostedAccountSignaturePrefix),
	}
	if !VerifyAccountSignature(hosted, ikp, true) {
		t.Error("assinatura hosted valida foi recusada")
	}
	if VerifyAccountSignature(hosted, ikp, false) {
		t.Error("assinatura hosted foi aceita como nao-hosted")
	}
}
