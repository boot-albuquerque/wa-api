// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package types

import (
	"strings"
	"testing"
)

// IsIncomingBroadcast decide se a mensagem aparece no chat direto com o
// remetente. A condicao tem duas partes e a interacao entre elas nao e' obvia:
// uma transmissao que EU enviei so' conta como "entrando" se tiver dono de
// lista preenchido (o eco das minhas outras sessoes).
func TestIsIncomingBroadcast(t *testing.T) {
	list := NewJID("123", BroadcastServer)
	owner := NewJID("5511999999999", DefaultUserServer)

	tests := []struct {
		name string
		src  MessageSource
		want bool
	}{
		{"recebida numa lista", MessageSource{Chat: list, IsFromMe: false}, true},
		{"enviada por mim, sem dono", MessageSource{Chat: list, IsFromMe: true}, false},
		{"enviada por mim, com dono", MessageSource{Chat: list, IsFromMe: true, BroadcastListOwner: owner}, true},
		{"status nao e lista", MessageSource{Chat: StatusBroadcastJID, IsFromMe: false}, false},
		{"chat direto", MessageSource{Chat: owner, IsFromMe: false}, false},
		{"grupo", MessageSource{Chat: NewJID("120363", GroupServer), IsFromMe: false}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.src.IsIncomingBroadcast(); got != tc.want {
				t.Errorf("= %v, esperado %v", got, tc.want)
			}
		})
	}
}

// SourceString colapsa remetente e chat quando sao o mesmo (chat direto) e os
// mostra separados quando diferem (grupo). E' so' para log, mas e' o que torna
// o log de grupo legivel.
func TestSourceStringCollapsesDirectChats(t *testing.T) {
	user := NewJID("5511999999999", DefaultUserServer)
	group := NewJID("120363", GroupServer)

	direct := MessageSource{Chat: user, Sender: user}
	if got := direct.SourceString(); got != user.String() {
		t.Errorf("chat direto = %q, esperado %q", got, user.String())
	}

	inGroup := MessageSource{Chat: group, Sender: user}
	got := inGroup.SourceString()
	if !strings.Contains(got, user.String()) || !strings.Contains(got, group.String()) {
		t.Errorf("grupo = %q, esperado conter remetente e chat", got)
	}
	if !strings.Contains(got, " in ") {
		t.Errorf("grupo = %q, esperado o separador \" in \"", got)
	}
}

// Os valores de EditAttribute sao os codigos que vao no atributo `edit` do
// frame. Sao strings numericas e nao consecutivas (falta o "3"..."6"), entao
// nao da' para derivar nenhum deles.
func TestEditAttributeValuesAreDistinct(t *testing.T) {
	seen := map[EditAttribute]string{}
	for name, value := range map[string]EditAttribute{
		"Empty":        EditAttributeEmpty,
		"MessageEdit":  EditAttributeMessageEdit,
		"PinInChat":    EditAttributePinInChat,
		"SenderRevoke": EditAttributeSenderRevoke,
		"AdminRevoke":  EditAttributeAdminRevoke,
	} {
		if other, dup := seen[value]; dup {
			t.Errorf("%s e %s tem o mesmo valor %q", name, other, value)
		}
		seen[value] = name
	}
}

func TestAddressingModeValues(t *testing.T) {
	if AddressingModePN == AddressingModeLID {
		t.Fatal("os dois modos de enderecamento colidiram")
	}
	if AddressingModePN != "pn" || AddressingModeLID != "lid" {
		t.Errorf("valores mudaram: %q / %q", AddressingModePN, AddressingModeLID)
	}
}
