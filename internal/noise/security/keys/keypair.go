// Package keys contains a utility struct for elliptic curve keypairs.
package keys

import (
	"go.mau.fi/libsignal/ecc"
	"go.mau.fi/util/random"
	"golang.org/x/crypto/curve25519"
)

type KeyPair struct {
	Pub  *[KeyLength]byte
	Priv *[KeyLength]byte
}

var _ ecc.ECPublicKeyable

func NewKeyPairFromPrivateKey(priv [KeyLength]byte) *KeyPair {
	var kp KeyPair
	kp.Priv = &priv
	var pub [KeyLength]byte
	curve25519.ScalarBaseMult(&pub, kp.Priv)
	kp.Pub = &pub
	return &kp
}

func NewKeyPair() *KeyPair {
	priv := *(*[KeyLength]byte)(random.Bytes(KeyLength))

	priv[clampFirstByte] &= clampLowBitsMask
	priv[clampLastByte] &= clampHighBitMask
	priv[clampLastByte] |= clampSecondHighBit

	return NewKeyPairFromPrivateKey(priv)
}

func (kp *KeyPair) CreateSignedPreKey(keyID uint32) *PreKey {
	newKey := NewPreKey(keyID)
	newKey.Signature = kp.Sign(&newKey.KeyPair)
	return newKey
}

func (kp *KeyPair) Sign(keyToSign *KeyPair) *[SignatureLength]byte {
	pubKeyForSignature := make([]byte, signedKeyLength)
	pubKeyForSignature[0] = ecc.DjbType
	copy(pubKeyForSignature[keyTypePrefixLength:], keyToSign.Pub[:])

	signature := ecc.CalculateSignature(ecc.NewDjbECPrivateKey(*kp.Priv), pubKeyForSignature)
	return &signature
}

type PreKey struct {
	KeyPair
	KeyID     uint32
	Signature *[SignatureLength]byte
}

func NewPreKey(keyID uint32) *PreKey {
	return &PreKey{
		KeyPair: *NewKeyPair(),
		KeyID:   keyID,
	}
}
