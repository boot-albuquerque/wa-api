// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package handshake

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waCert"
)

// Estes testes cobrem o **lado da rejeicao** de VerifyServerCert. Montar uma
// cadeia que passe exigiria a chave privada do emissor da WhatsApp, que nao
// existe fora dos servidores deles — e simular a verificacao com outra chave
// testaria o duplo, nao o codigo. O caminho de aceitacao esta' documentado como
// lacuna em PATCHES.md.
//
// O lado da rejeicao e' justamente o que importa para seguranca: e' aqui que se
// decide se um servidor que nao e' a WhatsApp consegue terminar o handshake.

func detailsBytes(t *testing.T, d *waCert.CertChain_NoiseCertificate_Details) []byte {
	t.Helper()
	raw, err := proto.Marshal(d)
	if err != nil {
		t.Fatalf("marshal dos detalhes: %v", err)
	}
	return raw
}

func certChainBytes(t *testing.T, chain *waCert.CertChain) []byte {
	t.Helper()
	raw, err := proto.Marshal(chain)
	if err != nil {
		t.Fatalf("marshal da cadeia: %v", err)
	}
	return raw
}

// Lixo que nao e' protobuf tem que virar erro, nao panic.
func TestVerifyServerCertRejectsUnparseable(t *testing.T) {
	err := VerifyServerCert([]byte{0xff, 0xff, 0xff, 0xff}, make([]byte, NoiseKeyLength))
	if err == nil {
		t.Fatal("cadeia ilegivel deveria ser rejeitada")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("err = %v, queria falar de unmarshal", err)
	}
}

// Cadeia vazia, ou com so' uma das duas pontas, tem que ser recusada antes de
// qualquer indexacao — e' o que impede um nil deref no caminho de handshake.
func TestVerifyServerCertRejectsMissingParts(t *testing.T) {
	valid := &waCert.CertChain_NoiseCertificate{
		Details:   detailsBytes(t, &waCert.CertChain_NoiseCertificate_Details{}),
		Signature: make([]byte, CertSignatureLength),
	}
	cases := []struct {
		name  string
		chain *waCert.CertChain
	}{
		{name: "cadeia vazia", chain: &waCert.CertChain{}},
		{name: "so' leaf", chain: &waCert.CertChain{Leaf: valid}},
		{name: "so' intermediate", chain: &waCert.CertChain{Intermediate: valid}},
		{name: "intermediate sem assinatura", chain: &waCert.CertChain{
			Leaf:         valid,
			Intermediate: &waCert.CertChain_NoiseCertificate{Details: valid.Details},
		}},
		{name: "leaf sem detalhes", chain: &waCert.CertChain{
			Leaf:         &waCert.CertChain_NoiseCertificate{Signature: valid.Signature},
			Intermediate: valid,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyServerCert(certChainBytes(t, tc.chain), make([]byte, NoiseKeyLength))
			if err == nil {
				t.Fatal("deveria ser rejeitada")
			}
			if !strings.Contains(err.Error(), "missing parts") {
				t.Errorf("err = %v, queria \"missing parts\"", err)
			}
		})
	}
}

// As conversoes de fatia para array em VerifyServerCert ([CertSignatureLength]byte(...))
// entram em panic se o comprimento nao bater. As checagens de tamanho sao o que
// as torna seguras: um servidor mandando uma assinatura de 3 bytes tem que
// receber erro, nao derrubar o processo.
func TestVerifyServerCertRejectsBadSignatureLengths(t *testing.T) {
	details := detailsBytes(t, &waCert.CertChain_NoiseCertificate_Details{})
	cases := []struct {
		name         string
		interSigLen  int
		leafSigLen   int
		wantContains string
	}{
		{name: "intermediate curta", interSigLen: 3, leafSigLen: CertSignatureLength, wantContains: "intermediate cert signature"},
		{name: "intermediate longa", interSigLen: 65, leafSigLen: CertSignatureLength, wantContains: "intermediate cert signature"},
		{name: "intermediate vazia mas nao nil", interSigLen: 1, leafSigLen: CertSignatureLength, wantContains: "intermediate cert signature"},
		{name: "leaf curta", interSigLen: CertSignatureLength, leafSigLen: 10, wantContains: "leaf cert signature"},
		{name: "leaf longa", interSigLen: CertSignatureLength, leafSigLen: 128, wantContains: "leaf cert signature"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chain := &waCert.CertChain{
				Intermediate: &waCert.CertChain_NoiseCertificate{
					Details:   details,
					Signature: make([]byte, tc.interSigLen),
				},
				Leaf: &waCert.CertChain_NoiseCertificate{
					Details:   details,
					Signature: make([]byte, tc.leafSigLen),
				},
			}
			err := VerifyServerCert(certChainBytes(t, chain), make([]byte, NoiseKeyLength))
			if err == nil {
				t.Fatal("deveria ser rejeitada")
			}
			if !strings.Contains(err.Error(), tc.wantContains) {
				t.Errorf("err = %v, queria conter %q", err, tc.wantContains)
			}
		})
	}
}

// Com os tamanhos certos mas assinatura invalida, a verificacao criptografica
// tem que reprovar. E' o teste que garante que a assinatura e' realmente
// conferida contra WACertPubKey, e nao apenas medida.
func TestVerifyServerCertRejectsInvalidSignature(t *testing.T) {
	details := detailsBytes(t, &waCert.CertChain_NoiseCertificate_Details{
		Serial: proto.Uint32(1),
		Key:    make([]byte, NoiseKeyLength),
	})
	chain := &waCert.CertChain{
		Intermediate: &waCert.CertChain_NoiseCertificate{
			Details:   details,
			Signature: bytes.Repeat([]byte{0xAA}, CertSignatureLength),
		},
		Leaf: &waCert.CertChain_NoiseCertificate{
			Details:   details,
			Signature: bytes.Repeat([]byte{0xBB}, CertSignatureLength),
		},
	}

	err := VerifyServerCert(certChainBytes(t, chain), make([]byte, NoiseKeyLength))

	if err == nil {
		t.Fatal("assinatura forjada deveria ser rejeitada")
	}
	if !strings.Contains(err.Error(), "verify intermediate cert signature") {
		t.Errorf("err = %v, queria falha de verificacao de assinatura", err)
	}
}

// --- checkCertValidity ---

// A janela de validade e' conferida nas duas pontas. Aceitar um certificado
// expirado (ou ainda nao valido) anula o proposito da cadeia.
func TestCheckCertValidity(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name         string
		notBefore    time.Time
		notAfter     time.Time
		wantErr      bool
		wantContains string
	}{
		{
			name:      "dentro da janela",
			notBefore: now.Add(-time.Hour),
			notAfter:  now.Add(time.Hour),
		},
		{
			name:         "ainda nao valido",
			notBefore:    now.Add(time.Hour),
			notAfter:     now.Add(2 * time.Hour),
			wantErr:      true,
			wantContains: "not valid yet",
		},
		{
			name:         "expirado",
			notBefore:    now.Add(-2 * time.Hour),
			notAfter:     now.Add(-time.Hour),
			wantErr:      true,
			wantContains: "expired",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cert := &waCert.CertChain_NoiseCertificate_Details{
				NotBefore: proto.Uint64(uint64(tc.notBefore.Unix())),
				NotAfter:  proto.Uint64(uint64(tc.notAfter.Unix())),
			}
			err := checkCertValidity(cert)
			if tc.wantErr {
				if err == nil {
					t.Fatal("deveria ser rejeitado")
				}
				if !strings.Contains(err.Error(), tc.wantContains) {
					t.Errorf("err = %v, queria conter %q", err, tc.wantContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, queria nil", err)
			}
		})
	}
}

// Um certificado sem NotBefore/NotAfter vira janela [epoch, epoch]: tem que ser
// tratado como expirado, nunca como "sempre valido".
func TestCheckCertValidityMissingBoundsIsExpired(t *testing.T) {
	err := checkCertValidity(&waCert.CertChain_NoiseCertificate_Details{})
	if err == nil {
		t.Fatal("certificado sem janela de validade nao pode ser aceito")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("err = %v, queria \"expired\"", err)
	}
}

// --- constantes do handshake ---

// Os tamanhos sao definidos pelas primitivas (Curve25519 / assinatura da
// cadeia). Se alguem mexer neles, as conversoes de fatia para array em
// handshake.go passam a entrar em panic com dados validos.
func TestHandshakeKeyLengthConstants(t *testing.T) {
	if NoiseKeyLength != 32 {
		t.Errorf("NoiseKeyLength = %d, queria 32 (Curve25519)", NoiseKeyLength)
	}
	if CertSignatureLength != 64 {
		t.Errorf("CertSignatureLength = %d, queria 64", CertSignatureLength)
	}
	if len(WACertPubKey) != NoiseKeyLength {
		t.Errorf("len(WACertPubKey) = %d, queria %d", len(WACertPubKey), NoiseKeyLength)
	}
	if WACertIssuerSerial != 0 {
		t.Errorf("WACertIssuerSerial = %d, queria 0", WACertIssuerSerial)
	}
	if ResponseTimeout <= 0 {
		t.Error("ResponseTimeout tem que ser positivo, senao o handshake desiste na hora")
	}
}
