// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package hkdfutil

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"golang.org/x/crypto/hkdf"
)

func TestSHA256ReturnsExactlyTheRequestedLength(t *testing.T) {
	key := []byte("chave")
	for _, length := range []uint8{0, 1, 32, 64, 255} {
		if got := SHA256(key, nil, nil, length); len(got) != int(length) {
			t.Errorf("length %d devolveu %d bytes", length, len(got))
		}
	}
}

// SHA256 tem que ser exatamente o HKDF da stdlib: e' o que garante que a chave
// derivada aqui bate com a que o outro lado deriva.
func TestSHA256MatchesTheStandardHKDF(t *testing.T) {
	key, salt, info := []byte("chave"), []byte("sal"), []byte("info")

	want := make([]byte, 32)
	if _, err := hkdf.New(sha256.New, key, salt, info).Read(want); err != nil {
		t.Fatal(err)
	}
	if got := SHA256(key, salt, info, 32); !bytes.Equal(got, want) {
		t.Errorf("= %x, esperado %x", got, want)
	}
}

func TestSHA256IsDeterministic(t *testing.T) {
	key, salt, info := []byte("chave"), []byte("sal"), []byte("info")
	first := SHA256(key, salt, info, 32)
	if second := SHA256(key, salt, info, 32); !bytes.Equal(first, second) {
		t.Error("a mesma entrada produziu saidas diferentes")
	}
}

// Cada parametro tem que influenciar a saida. Se salt ou info fossem ignorados,
// duas derivacoes de contexto diferente colidiriam — que e' exatamente o que o
// HKDF existe para impedir.
func TestEveryParameterChangesTheOutput(t *testing.T) {
	base := SHA256([]byte("chave"), []byte("sal"), []byte("info"), 32)

	tests := map[string][]byte{
		"chave diferente": SHA256([]byte("outra"), []byte("sal"), []byte("info"), 32),
		"sal diferente":   SHA256([]byte("chave"), []byte("outro"), []byte("info"), 32),
		"info diferente":  SHA256([]byte("chave"), []byte("sal"), []byte("outra"), 32),
		"sem sal":         SHA256([]byte("chave"), nil, []byte("info"), 32),
		"sem info":        SHA256([]byte("chave"), []byte("sal"), nil, 32),
	}
	for name, got := range tests {
		if bytes.Equal(base, got) {
			t.Errorf("%s produziu a mesma saida", name)
		}
	}
}

// Um prefixo do HKDF e' o mesmo para tamanhos diferentes: e' um stream. Trava a
// propriedade para quem for tentado a "otimizar" derivando de outro jeito.
func TestShorterOutputIsAPrefixOfTheLonger(t *testing.T) {
	long := SHA256([]byte("chave"), []byte("sal"), []byte("info"), 64)
	short := SHA256([]byte("chave"), []byte("sal"), []byte("info"), 32)
	if !bytes.Equal(long[:32], short) {
		t.Error("a saida curta nao e' prefixo da longa")
	}
}

// O tamanho e' uint8, entao o maximo pedivel e' 255 — abaixo do limite de
// 255*32 bytes do HKDF-SHA256. E' por isso que os dois panics do codigo sao
// inalcancaveis, como o proprio comentario do upstream diz.
func TestMaximumLengthIsWithinTheHKDFLimit(t *testing.T) {
	const hkdfMaxOutput = 255 * sha256.Size
	if int(^uint8(0)) > hkdfMaxOutput {
		t.Fatal("o tipo do parametro passou a permitir mais que o HKDF entrega")
	}
	if got := SHA256([]byte("chave"), nil, nil, ^uint8(0)); len(got) != 255 {
		t.Errorf("o tamanho maximo devolveu %d bytes", len(got))
	}
}
