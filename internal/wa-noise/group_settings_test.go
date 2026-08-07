// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"strings"
	"testing"

	"wa-api/internal/wa-noise/types"
)

// SetGroupMemberAddMode e' a unica funcao de group_settings.go com logica
// antes da rede: a validacao do modo. Os modos validos nao dao para exercitar
// aqui (caem em sendGroupIQ), mas o rejeitado retorna antes de tocar o socket.
func TestSetGroupMemberAddModeRejectsInvalidMode(t *testing.T) {
	cli := groupTestClient()
	err := cli.SetGroupMemberAddMode(context.Background(), groupTestJID, types.GroupMemberAddMode("qualquer_coisa"))
	if err == nil {
		t.Fatal("esperado erro para modo invalido")
	}
	for _, want := range []types.GroupMemberAddMode{types.GroupMemberAddModeAdmin, types.GroupMemberAddModeAllMember} {
		if !strings.Contains(err.Error(), string(want)) {
			t.Errorf("mensagem de erro %q nao cita o modo valido %q", err, want)
		}
	}
}
