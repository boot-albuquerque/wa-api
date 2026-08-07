// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package pairing

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.mau.fi/util/random"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/pbkdf2"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/security/paircrypto"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/security/hkdf"
	"wa-api/internal/wa-noise/security/keys"
)

var notNumbers = regexp.MustCompile("[^0-9]")
var linkingBase32 = base32.NewEncoding(codeBase32Alphabet)

// generateCompanionEphemeralKey gera o par efemero deste companion e o embrulha
// com a chave derivada do codigo de pareamento de 8 caracteres.
func generateCompanionEphemeralKey() (ephemeralKeyPair *keys.KeyPair, ephemeralKey []byte, encodedLinkingCode string) {
	ephemeralKeyPair = keys.NewKeyPair()
	salt := random.Bytes(codeSaltLength)
	iv := random.Bytes(codeIVLength)
	linkingCode := random.Bytes(codeRawLength)
	encodedLinkingCode = linkingBase32.EncodeToString(linkingCode)
	linkCodeKey := pbkdf2.Key([]byte(encodedLinkingCode), salt, codePBKDF2Iterations, codeKeyLength, sha256.New)
	linkCipherBlock, _ := aes.NewCipher(linkCodeKey)
	encryptedPubkey := ephemeralKeyPair.Pub[:]
	cipher.NewCTR(linkCipherBlock, iv).XORKeyStream(encryptedPubkey, encryptedPubkey)
	ephemeralKey = make([]byte, codeWrappedKeyEnd)
	copy(ephemeralKey[0:codeSaltEnd], salt)
	copy(ephemeralKey[codeSaltEnd:codeIVEnd], iv)
	copy(ephemeralKey[codeIVEnd:codeWrappedKeyEnd], encryptedPubkey)
	return
}

// PairPhone gera o codigo de pareamento e abre a sessao com o servidor. Era
// Client.PairPhone; a guarda de `cli == nil` ficou na fachada da raiz.
func PairPhone(
	ctx context.Context,
	t Transport,
	phone string,
	showPushNotification bool,
	clientType ClientType,
	clientDisplayName string,
) (string, error) {
	ephemeralKeyPair, ephemeralKey, encodedLinkingCode := generateCompanionEphemeralKey()
	phone = notNumbers.ReplaceAllString(phone, "")
	if len(phone) < codePhoneMinLength {
		return "", ErrPhoneNumberTooShort
	} else if strings.HasPrefix(phone, codePhoneTrunkPrefix) {
		return "", ErrPhoneNumberIsNotInternational
	}
	jid := types.NewJID(phone, types.DefaultUserServer)
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: "md",
		Type:      IQSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "link_code_companion_reg",
			Attrs: waBinary.Attrs{
				"jid":   jid,
				"stage": "companion_hello",

				"should_show_push_notification": strconv.FormatBool(showPushNotification),
			},
			Content: []waBinary.Node{
				{Tag: "link_code_pairing_wrapped_companion_ephemeral_pub", Content: ephemeralKey},
				{Tag: "companion_server_auth_key_pub", Content: t.Store().NoiseKey.Pub[:]},
				{Tag: "companion_platform_id", Content: string(clientType)},
				{Tag: "companion_platform_display", Content: clientDisplayName},
				{Tag: "link_code_pairing_nonce", Content: []byte{0}},
			},
		}},
	})
	if err != nil {
		return "", err
	}
	pairingRefNode, ok := resp.GetOptionalChildByTag("link_code_companion_reg", "link_code_pairing_ref")
	if !ok {
		return "", t.ElementMissing("link_code_pairing_ref", "code link registration response")
	}
	pairingRef, ok := pairingRefNode.Content.([]byte)
	if !ok {
		return "", fmt.Errorf("unexpected type %T in content of link_code_pairing_ref tag", pairingRefNode.Content)
	}
	t.State().SetLinking(&LinkingCache{
		JID:         jid,
		KeyPair:     ephemeralKeyPair,
		LinkingCode: encodedLinkingCode,
		PairingRef:  string(pairingRef),
	})
	return encodedLinkingCode[0:codeGroupLength] + "-" + encodedLinkingCode[codeGroupLength:], nil
}

// TryHandleCodeNotification e' o wrapper que so' loga o erro. Era
// Client.tryHandleCodePairNotification.
func TryHandleCodeNotification(ctx context.Context, t Transport, parentNode *waBinary.Node) {
	err := HandleCodeNotification(ctx, t, parentNode)
	if err != nil {
		t.Log().Errorf("Failed to handle code pair notification: %s", err)
	}
}

// HandleCodeNotification fecha o pareamento por codigo: decifra a chave
// efemera do aparelho principal, deriva o adv secret e devolve o key bundle.
// Era Client.handleCodePairNotification.
func HandleCodeNotification(ctx context.Context, t Transport, parentNode *waBinary.Node) error {
	node, ok := parentNode.GetOptionalChildByTag("link_code_companion_reg")
	if !ok {
		return t.ElementMissing("link_code_companion_reg", "notification")
	}
	linkCache := t.State().Linking()
	if linkCache == nil {
		return ErrNoPendingPairing
	}
	linkCodePairingRef, _ := node.GetChildByTag("link_code_pairing_ref").Content.([]byte)
	if string(linkCodePairingRef) != linkCache.PairingRef {
		return ErrPairingRefMismatch
	}
	wrappedPrimaryEphemeralPub, ok := node.GetChildByTag("link_code_pairing_wrapped_primary_ephemeral_pub").Content.([]byte)
	if !ok {
		return t.ElementMissing("link_code_pairing_wrapped_primary_ephemeral_pub", "notification")
	} else if len(wrappedPrimaryEphemeralPub) < codeWrappedKeyEnd {
		return fmt.Errorf("unexpected length of link_code_pairing_wrapped_primary_ephemeral_pub: %d", len(wrappedPrimaryEphemeralPub))
	}
	primaryIdentityPub, ok := node.GetChildByTag("primary_identity_pub").Content.([]byte)
	if !ok {
		return t.ElementMissing("primary_identity_pub", "notification")
	}

	advSecretRandom := random.Bytes(codeAdvSecretRandomLength)
	keyBundleSalt := random.Bytes(codeKeyBundleSaltLength)
	keyBundleNonce := random.Bytes(codeKeyBundleNonceLength)

	// Decrypt the primary device's ephemeral public key, which was encrypted with the 8-character pairing code,
	// then compute the DH shared secret using our ephemeral private key we generated earlier.
	primarySalt := wrappedPrimaryEphemeralPub[0:codeSaltEnd]
	primaryIV := wrappedPrimaryEphemeralPub[codeSaltEnd:codeIVEnd]
	primaryEncryptedPubkey := wrappedPrimaryEphemeralPub[codeIVEnd:codeWrappedKeyEnd]
	linkCodeKey := pbkdf2.Key([]byte(linkCache.LinkingCode), primarySalt, codePBKDF2Iterations, codeKeyLength, sha256.New)
	linkCipherBlock, err := aes.NewCipher(linkCodeKey)
	if err != nil {
		return fmt.Errorf("failed to create link cipher: %w", err)
	}
	primaryDecryptedPubkey := make([]byte, codeKeyLength)
	cipher.NewCTR(linkCipherBlock, primaryIV).XORKeyStream(primaryDecryptedPubkey, primaryEncryptedPubkey)
	ephemeralSharedSecret, err := curve25519.X25519(linkCache.KeyPair.Priv[:], primaryDecryptedPubkey)
	if err != nil {
		return fmt.Errorf("failed to compute ephemeral shared secret: %w", err)
	}

	// Encrypt and wrap key bundle containing our identity key, the primary device's identity key and the randomness used for the adv key.
	keyBundleEncryptionKey := hkdfutil.SHA256(ephemeralSharedSecret, keyBundleSalt, []byte(codeKeyBundleHKDFInfo), codeKeyBundleKeyLength)
	keyBundleCipherBlock, err := aes.NewCipher(keyBundleEncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to create key bundle cipher: %w", err)
	}
	keyBundleGCM, err := cipher.NewGCM(keyBundleCipherBlock)
	if err != nil {
		return fmt.Errorf("failed to create key bundle GCM: %w", err)
	}
	plaintextKeyBundle := paircrypto.ConcatBytes(t.Store().IdentityKey.Pub[:], primaryIdentityPub, advSecretRandom)
	encryptedKeyBundle := keyBundleGCM.Seal(nil, keyBundleNonce, plaintextKeyBundle, nil)
	wrappedKeyBundle := paircrypto.ConcatBytes(keyBundleSalt, keyBundleNonce, encryptedKeyBundle)

	// Compute the adv secret key (which is used to authenticate the pair-success event later)
	identitySharedKey, err := curve25519.X25519(t.Store().IdentityKey.Priv[:], primaryIdentityPub)
	if err != nil {
		return fmt.Errorf("failed to compute identity shared key: %w", err)
	}
	advSecretInput := append(append(ephemeralSharedSecret, identitySharedKey...), advSecretRandom...)
	advSecret := hkdfutil.SHA256(advSecretInput, nil, []byte(codeAdvSecretHKDFInfo), codeAdvSecretLength)
	t.Store().AdvSecretKey = advSecret

	_, err = t.SendIQ(ctx, IQ{
		Namespace: "md",
		Type:      IQSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "link_code_companion_reg",
			Attrs: waBinary.Attrs{
				"jid":   linkCache.JID,
				"stage": "companion_finish",
			},
			Content: []waBinary.Node{
				{Tag: "link_code_pairing_wrapped_key_bundle", Content: wrappedKeyBundle},
				{Tag: "companion_identity_public", Content: t.Store().IdentityKey.Pub[:]},
				{Tag: "link_code_pairing_ref", Content: linkCodePairingRef},
			},
		}},
	})
	return err
}
