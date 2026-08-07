// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package handshake

import (
	"crypto/hmac"
	"fmt"
	"time"

	"go.mau.fi/libsignal/ecc"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waCert"
)

func checkCertValidity(cert *waCert.CertChain_NoiseCertificate_Details) error {
	notBefore := time.Unix(int64(cert.GetNotBefore()), 0)
	notAfter := time.Unix(int64(cert.GetNotAfter()), 0)
	now := time.Now()
	if now.Before(notBefore) {
		return fmt.Errorf("certificate not valid yet (current time %s is before %s)", now, notBefore)
	} else if now.After(notAfter) {
		return fmt.Errorf("certificate expired (current time %s is after %s)", now, notAfter)
	}
	return nil
}

// VerifyServerCert confere a cadeia de certificados que o servidor mandou
// cifrada no <ServerHello> contra WACertPubKey, e confere que a chave do
// certificado folha e' exatamente a chave estatica que ja' foi decifrada.
//
// Era a funcao nao exportada verifyServerCert da raiz. Nao tem estado, nao toca
// lock nenhum e nao le nada do Client.
func VerifyServerCert(certDecrypted, staticDecrypted []byte) error {
	var certChain waCert.CertChain
	err := proto.Unmarshal(certDecrypted, &certChain)
	if err != nil {
		return fmt.Errorf("failed to unmarshal noise certificate: %w", err)
	}
	var intermediateCertDetails, leafCertDetails waCert.CertChain_NoiseCertificate_Details
	intermediateCertDetailsRaw := certChain.GetIntermediate().GetDetails()
	intermediateCertSignature := certChain.GetIntermediate().GetSignature()
	leafCertDetailsRaw := certChain.GetLeaf().GetDetails()
	leafCertSignature := certChain.GetLeaf().GetSignature()
	if intermediateCertDetailsRaw == nil || intermediateCertSignature == nil || leafCertDetailsRaw == nil || leafCertSignature == nil {
		return fmt.Errorf("missing parts of noise certificate")
	} else if len(intermediateCertSignature) != CertSignatureLength {
		return fmt.Errorf("unexpected length of intermediate cert signature %d (expected %d)", len(intermediateCertSignature), CertSignatureLength)
	} else if len(leafCertSignature) != CertSignatureLength {
		return fmt.Errorf("unexpected length of leaf cert signature %d (expected %d)", len(leafCertSignature), CertSignatureLength)
	} else if !ecc.VerifySignature(ecc.NewDjbECPublicKey(WACertPubKey), intermediateCertDetailsRaw, [CertSignatureLength]byte(intermediateCertSignature)) {
		return fmt.Errorf("failed to verify intermediate cert signature")
	} else if err = proto.Unmarshal(intermediateCertDetailsRaw, &intermediateCertDetails); err != nil {
		return fmt.Errorf("failed to unmarshal noise certificate details: %w", err)
	} else if intermediateCertDetails.GetIssuerSerial() != WACertIssuerSerial {
		return fmt.Errorf("unexpected intermediate issuer serial %d (expected %d)", intermediateCertDetails.GetIssuerSerial(), WACertIssuerSerial)
	} else if len(intermediateCertDetails.GetKey()) != NoiseKeyLength {
		return fmt.Errorf("unexpected length of intermediate cert key %d (expected %d)", len(intermediateCertDetails.GetKey()), NoiseKeyLength)
	} else if !ecc.VerifySignature(ecc.NewDjbECPublicKey([NoiseKeyLength]byte(intermediateCertDetails.GetKey())), leafCertDetailsRaw, [CertSignatureLength]byte(leafCertSignature)) {
		return fmt.Errorf("failed to verify intermediate cert signature")
	} else if err = checkCertValidity(&intermediateCertDetails); err != nil {
		return fmt.Errorf("intermediate cert %w", err)
	} else if err = proto.Unmarshal(leafCertDetailsRaw, &leafCertDetails); err != nil {
		return fmt.Errorf("failed to unmarshal noise certificate details: %w", err)
	} else if leafCertDetails.GetIssuerSerial() != intermediateCertDetails.GetSerial() {
		return fmt.Errorf("unexpected leaf issuer serial %d (expected %d)", leafCertDetails.GetIssuerSerial(), intermediateCertDetails.GetSerial())
	} else if !hmac.Equal(leafCertDetails.GetKey(), staticDecrypted) {
		return fmt.Errorf("cert key doesn't match decrypted static")
	} else if err = checkCertValidity(&leafCertDetails); err != nil {
		return fmt.Errorf("leaf cert cert %w", err)
	}
	return nil
}
