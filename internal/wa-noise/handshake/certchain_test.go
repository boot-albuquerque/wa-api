// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package handshake

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/libsignal/ecc"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waCert"
	"wa-api/internal/wa-noise/util/keys"
)

// Os testes deste arquivo trocam WACertPubKey por uma chave que o teste
// controla, para poder montar cadeias que **passam** pela verificacao de
// assinatura e assim exercitar tudo o que vem depois dela — serial do emissor,
// tamanho da chave, assinatura da folha, janela de validade e a comparacao
// final com a estatica decifrada.
//
// Sem essa troca so' daria para chegar ate' a primeira verificacao de
// assinatura, porque a chave privada correspondente a WACertPubKey de producao
// so' existe nos servidores da WhatsApp. A troca e' restaurada por t.Cleanup, e
// WACertPubKey nunca e' lida em paralelo por estes testes.
func useTestRootKey(t *testing.T) *keys.KeyPair {
	t.Helper()
	root := keys.NewKeyPair()
	original := WACertPubKey
	WACertPubKey = *root.Pub
	t.Cleanup(func() { WACertPubKey = original })
	return root
}

func signDetails(kp *keys.KeyPair, details []byte) []byte {
	sig := ecc.CalculateSignature(ecc.NewDjbECPrivateKey(*kp.Priv), details)
	return sig[:]
}

// chainOpts descreve a cadeia a montar. Os zero-values produzem uma cadeia
// **valida** ponta a ponta; cada teste estraga exatamente um ponto.
type chainOpts struct {
	interSerial      uint32
	interKey         []byte // chave publica do intermediario (assina a folha)
	interKeyOverride []byte // se != nil, e' o que vai no campo Key
	leafIssuerSerial uint32
	leafKey          []byte
	interNotBefore   time.Time
	interNotAfter    time.Time
	leafNotBefore    time.Time
	leafNotAfter     time.Time
	// signLeafWith assina os detalhes da folha; nil usa a chave do intermediario.
	signLeafWith *keys.KeyPair
}

func buildChain(t *testing.T, root *keys.KeyPair, o chainOpts) []byte {
	t.Helper()
	inter := keys.NewKeyPair()
	if o.interKey == nil {
		o.interKey = inter.Pub[:]
	}
	interKeyField := o.interKey
	if o.interKeyOverride != nil {
		interKeyField = o.interKeyOverride
	}
	interDetails := mustMarshal(&waCert.CertChain_NoiseCertificate_Details{
		Serial:       proto.Uint32(o.interSerial),
		IssuerSerial: proto.Uint32(WACertIssuerSerial),
		Key:          interKeyField,
		NotBefore:    proto.Uint64(uint64(o.interNotBefore.Unix())),
		NotAfter:     proto.Uint64(uint64(o.interNotAfter.Unix())),
	})
	leafDetails := mustMarshal(&waCert.CertChain_NoiseCertificate_Details{
		Serial:       proto.Uint32(o.interSerial + 1),
		IssuerSerial: proto.Uint32(o.leafIssuerSerial),
		Key:          o.leafKey,
		NotBefore:    proto.Uint64(uint64(o.leafNotBefore.Unix())),
		NotAfter:     proto.Uint64(uint64(o.leafNotAfter.Unix())),
	})
	leafSigner := o.signLeafWith
	if leafSigner == nil {
		leafSigner = inter
	}
	return mustMarshal(&waCert.CertChain{
		Intermediate: &waCert.CertChain_NoiseCertificate{
			Details:   interDetails,
			Signature: signDetails(root, interDetails),
		},
		Leaf: &waCert.CertChain_NoiseCertificate{
			Details:   leafDetails,
			Signature: signDetails(leafSigner, leafDetails),
		},
	})
}

// validOpts devolve opcoes que produzem uma cadeia aceita, dado o material
// estatico que o servidor apresentaria.
func validOpts(static []byte) chainOpts {
	now := time.Now()
	return chainOpts{
		interSerial:      7,
		leafIssuerSerial: 7,
		leafKey:          static,
		interNotBefore:   now.Add(-time.Hour),
		interNotAfter:    now.Add(time.Hour),
		leafNotBefore:    now.Add(-time.Hour),
		leafNotAfter:     now.Add(time.Hour),
	}
}

// O caminho feliz: cadeia bem formada, assinada pela raiz que o teste controla,
// dentro da validade, com a chave da folha batendo com a estatica decifrada.
func TestVerifyServerCertAcceptsWellFormedChain(t *testing.T) {
	root := useTestRootKey(t)
	static := keys.NewKeyPair().Pub[:]

	err := VerifyServerCert(buildChain(t, root, validOpts(static)), static)

	if err != nil {
		t.Fatalf("cadeia valida foi rejeitada: %v", err)
	}
}

func TestVerifyServerCertRejectsBadChains(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name         string
		mutate       func(o *chainOpts)
		wantContains string
	}{
		{
			name:         "serial do emissor do intermediario errado",
			mutate:       func(o *chainOpts) { o.interSerial = 3 }, // leafIssuerSerial continua 7
			wantContains: "unexpected leaf issuer serial",
		},
		{
			name:         "chave do intermediario com tamanho errado",
			mutate:       func(o *chainOpts) { o.interKeyOverride = make([]byte, 8) },
			wantContains: "unexpected length of intermediate cert key",
		},
		{
			name:         "folha assinada por outra chave",
			mutate:       func(o *chainOpts) { o.signLeafWith = keys.NewKeyPair() },
			wantContains: "verify intermediate cert signature",
		},
		{
			name: "intermediario expirado",
			mutate: func(o *chainOpts) {
				o.interNotBefore = now.Add(-2 * time.Hour)
				o.interNotAfter = now.Add(-time.Hour)
			},
			wantContains: "intermediate cert certificate expired",
		},
		{
			name: "folha ainda nao valida",
			mutate: func(o *chainOpts) {
				o.leafNotBefore = now.Add(time.Hour)
				o.leafNotAfter = now.Add(2 * time.Hour)
			},
			wantContains: "leaf cert cert certificate not valid yet",
		},
		{
			name:         "chave da folha nao bate com a estatica",
			mutate:       func(o *chainOpts) { o.leafKey = keys.NewKeyPair().Pub[:] },
			wantContains: "cert key doesn't match decrypted static",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := useTestRootKey(t)
			static := keys.NewKeyPair().Pub[:]
			opts := validOpts(static)
			tc.mutate(&opts)

			err := VerifyServerCert(buildChain(t, root, opts), static)

			if err == nil {
				t.Fatal("deveria ser rejeitada")
			}
			if !strings.Contains(err.Error(), tc.wantContains) {
				t.Errorf("err = %v, queria conter %q", err, tc.wantContains)
			}
		})
	}
}

// Detalhes do intermediario que assinam corretamente mas nao sao protobuf
// valido: o unmarshal tem que virar erro. Sem a assinatura de teste seria
// impossivel chegar nesta linha.
func TestVerifyServerCertRejectsUnparseableIntermediateDetails(t *testing.T) {
	root := useTestRootKey(t)
	// 0x08 abre um campo varint e nada vem depois: assinatura valida, protobuf
	// truncado.
	bogus := []byte{0x08}
	chain := mustMarshal(&waCert.CertChain{
		Intermediate: &waCert.CertChain_NoiseCertificate{
			Details:   bogus,
			Signature: signDetails(root, bogus),
		},
		Leaf: &waCert.CertChain_NoiseCertificate{
			Details:   bogus,
			Signature: make([]byte, CertSignatureLength),
		},
	})

	err := VerifyServerCert(chain, make([]byte, NoiseKeyLength))

	if err == nil {
		t.Fatal("detalhes ilegiveis deveriam ser rejeitados")
	}
	if !strings.Contains(err.Error(), "unmarshal noise certificate details") {
		t.Errorf("err = %v, queria falha de unmarshal dos detalhes", err)
	}
}
