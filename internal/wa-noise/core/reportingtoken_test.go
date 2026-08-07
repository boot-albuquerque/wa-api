// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"encoding/binary"
	"testing"

	"wa-api/internal/wa-noise/protocol/proto/waE2E"
)

// encodeTag monta a tag varint de um campo protobuf a partir do numero do campo
// e do wire type — a operacao inversa do que extractReportingTokenContent faz.
func encodeTag(fieldNum, wireType int) []byte {
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, uint64(fieldNum)<<wireFieldNumShift|uint64(wireType))
	return buf[:n]
}

func encodeBytesField(fieldNum int, value []byte) []byte {
	out := encodeTag(fieldNum, wireBytes)
	lenBuf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(lenBuf, uint64(len(value)))
	out = append(out, lenBuf[:n]...)
	return append(out, value...)
}

func encodeVarintField(fieldNum int, value uint64) []byte {
	out := encodeTag(fieldNum, wireVarint)
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, value)
	return append(out, buf[:n]...)
}

// Campos que nao estao na config sao descartados: o token so' cobre o
// subconjunto canonico declarado em reportingfields.json.
func TestExtractReportingTokenContentKeepsOnlyConfiguredFields(t *testing.T) {
	config := []reportingField{{FieldNumber: 1}}
	kept := encodeBytesField(1, []byte("mantido"))
	data := append(append([]byte{}, kept...), encodeBytesField(2, []byte("descartado"))...)

	got := extractReportingTokenContent(data, config)

	if !bytes.Equal(got, kept) {
		t.Errorf("= %x, esperado %x", got, kept)
	}
}

// A saida e' ordenada por numero de campo, nao pela ordem em que os campos
// aparecem no wire — e' o que torna o token estavel entre serializacoes.
func TestExtractReportingTokenContentSortsByFieldNumber(t *testing.T) {
	config := []reportingField{{FieldNumber: 1}, {FieldNumber: 2}, {FieldNumber: 3}}
	first := encodeBytesField(1, []byte("a"))
	second := encodeBytesField(2, []byte("b"))
	third := encodeBytesField(3, []byte("c"))

	// Entrada fora de ordem.
	data := append(append(append([]byte{}, third...), first...), second...)

	got := extractReportingTokenContent(data, config)

	want := append(append(append([]byte{}, first...), second...), third...)
	if !bytes.Equal(got, want) {
		t.Errorf("= %x, esperado %x", got, want)
	}
}

// Campos aninhados sao extraidos recursivamente e re-serializados com o
// comprimento recalculado, ja que os subcampos nao configurados sumiram.
func TestExtractReportingTokenContentRecursesIntoSubfields(t *testing.T) {
	inner := append(encodeBytesField(1, []byte("guarda")), encodeBytesField(9, []byte("joga fora"))...)
	config := []reportingField{{
		FieldNumber: 5,
		IsMessage:   true,
		Subfields:   []reportingField{{FieldNumber: 1}},
	}}

	got := extractReportingTokenContent(encodeBytesField(5, inner), config)

	want := encodeBytesField(5, encodeBytesField(1, []byte("guarda")))
	if !bytes.Equal(got, want) {
		t.Errorf("= %x, esperado %x", got, want)
	}
}

// Se todos os subcampos forem descartados, o campo pai tambem some — nao vai
// para o token um no vazio.
func TestExtractReportingTokenContentDropsEmptyNestedField(t *testing.T) {
	config := []reportingField{{
		FieldNumber: 5,
		IsMessage:   true,
		Subfields:   []reportingField{{FieldNumber: 1}},
	}}

	got := extractReportingTokenContent(encodeBytesField(5, encodeBytesField(9, []byte("x"))), config)

	if len(got) != 0 {
		t.Errorf("= %x, esperado vazio", got)
	}
}

func TestExtractReportingTokenContentHandlesFixedWidthWireTypes(t *testing.T) {
	config := []reportingField{{FieldNumber: 1}, {FieldNumber: 2}, {FieldNumber: 3}}

	varint := encodeVarintField(1, 300)
	fixed64 := append(encodeTag(2, wire64bit), bytes.Repeat([]byte{0xAA}, wire64bitLength)...)
	fixed32 := append(encodeTag(3, wire32bit), bytes.Repeat([]byte{0xBB}, wire32bitLength)...)
	data := append(append(append([]byte{}, varint...), fixed64...), fixed32...)

	got := extractReportingTokenContent(data, config)

	want := append(append(append([]byte{}, varint...), fixed64...), fixed32...)
	if !bytes.Equal(got, want) {
		t.Errorf("= %x, esperado %x", got, want)
	}
}

// Regressao das guardas de limite adicionadas no lote 4. Cada entrada abaixo
// fazia o extrator fatiar `data` fora dos limites — panico, nao erro.
// A funcao roda no caminho de envio de mensagem, entao panico ali derruba a
// goroutine de envio.
func TestExtractReportingTokenContentDoesNotPanicOnTruncatedInput(t *testing.T) {
	config := []reportingField{{FieldNumber: 1}, {FieldNumber: 2}, {FieldNumber: 3}}

	for name, data := range map[string][]byte{
		"comprimento maior que o buffer":   append(encodeTag(1, wireBytes), 0x7F),
		"fixed64 truncado":                 append(encodeTag(2, wire64bit), 0x01, 0x02),
		"fixed32 truncado":                 append(encodeTag(3, wire32bit), 0x01),
		"campo nao configurado truncado":   append(encodeTag(8, wireBytes), 0x7F),
		"fixed64 nao configurado truncado": append(encodeTag(8, wire64bit), 0x01),
		"wire type invalido":               encodeTag(1, 6),
		"tag sozinha":                      encodeTag(1, wireBytes),
		"buffer vazio":                     {},
		"varint truncado":                  append(encodeTag(1, wireVarint), 0x80),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("entrou em panico: %v", r)
				}
			}()
			extractReportingTokenContent(data, config)
		})
	}
}

// getReportingToken sobre um protobuf real e determinista: a mesma mensagem
// sempre produz o mesmo token, senao o servidor nao consegue correlacionar.
func TestGetReportingTokenIsDeterministic(t *testing.T) {
	msg := []byte("\x0a\x05hello")

	first := getReportingToken(msg)
	second := getReportingToken(msg)

	if !bytes.Equal(first, second) {
		t.Errorf("nao determinista: %x != %x", first, second)
	}
}

func TestGetConfigForField(t *testing.T) {
	fields := []reportingField{{FieldNumber: 1}, {FieldNumber: 7}}

	if got := getConfigForField(fields, 7); got == nil || got.FieldNumber != 7 {
		t.Errorf("= %v, esperado o campo 7", got)
	}
	if got := getConfigForField(fields, 99); got != nil {
		t.Errorf("= %v, esperado nil para campo ausente", got)
	}
}

// reportingfields.json e' embutido no binario e desserializado com
// PanicIfNotNil: se estiver malformado, o processo morre no primeiro envio.
// Este teste falha em vez de deixar isso chegar em producao.
func TestReportingFieldsJSONIsValid(t *testing.T) {
	fields := getReportingFields()
	if len(fields) == 0 {
		t.Fatal("reportingfields.json nao produziu nenhum campo")
	}
	seen := make(map[int]bool, len(fields))
	for _, f := range fields {
		if f.FieldNumber <= 0 {
			t.Errorf("numero de campo invalido: %d", f.FieldNumber)
		}
		if seen[f.FieldNumber] {
			t.Errorf("campo %d duplicado — getConfigForField devolveria sempre o primeiro", f.FieldNumber)
		}
		seen[f.FieldNumber] = true
	}
}

// shouldIncludeReportingToken recusa os tipos de mensagem que nao carregam
// conteudo denunciavel proprio (reacoes, votos, respostas a evento).
func TestShouldIncludeReportingToken(t *testing.T) {
	for name, tc := range map[string]struct {
		enabled bool
		msg     *waE2E.Message
		want    bool
	}{
		"desligado no cliente": {false, &waE2E.Message{}, false},
		"mensagem comum":       {true, &waE2E.Message{Conversation: ptrTo("oi")}, true},
		"reacao":               {true, &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{}}, false},
		"reacao cifrada":       {true, &waE2E.Message{EncReactionMessage: &waE2E.EncReactionMessage{}}, false},
		"resposta a evento":    {true, &waE2E.Message{EncEventResponseMessage: &waE2E.EncEventResponseMessage{}}, false},
		"voto em enquete":      {true, &waE2E.Message{PollUpdateMessage: &waE2E.PollUpdateMessage{}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			cli := &Client{SendReportingTokens: tc.enabled}
			if got := cli.shouldIncludeReportingToken(tc.msg); got != tc.want {
				t.Errorf("= %v, esperado %v", got, tc.want)
			}
		})
	}
}

func ptrTo[T any](v T) *T { return &v }
