// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sqlstore

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"wa-api/internal/wa-noise/protocol/proto/waAdv"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// Estes testes rodam contra SQLite real em t.TempDir(), com o mesmo schema de
// producao (upgrades.Table, aplicado por Container.Upgrade). Sem CGO e sem
// Docker, como o resto do repositorio (ver pkg/infra/history/sync_test.go).
//
// O ponto e' que store/sqlstore/ e' quase inteiramente SQL: um duplo em memoria
// provaria apenas que o duplo funciona. So' um banco de verdade prova que os
// ON CONFLICT, os CHECK() de tamanho e os JOINs de LID <-> PN fazem o que a
// assinatura Go promete.

const testDBFileName = "wa-noise-store-test.db"

// newTestContainer abre um Container SQLite novo e ja' migrado.
func newTestContainer(t *testing.T) *Container {
	t.Helper()
	path := filepath.Join(t.TempDir(), testDBFileName)
	container, err := New(
		context.Background(),
		"sqlite",
		fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", path),
		nil,
	)
	if err != nil {
		t.Fatalf("abrir container sqlite: %v", err)
	}
	t.Cleanup(func() { _ = container.Close() })
	return container
}

// newTestDevice cria e persiste um device, devolvendo o container e o SQLStore
// ja' ligado a ele. Persistir e' obrigatorio: quase toda tabela tem FK para
// whatsmeow_device(jid).
func newTestDevice(t *testing.T) (*Container, *store.Device, *SQLStore) {
	t.Helper()
	container := newTestContainer(t)
	device := container.NewDevice()
	jid := types.JID{User: "5511999999999", Device: 0, Server: types.DefaultUserServer}
	device.ID = &jid
	device.LID = types.JID{User: "111111111111111", Device: 0, Server: types.HiddenUserServer}
	// NewDevice nao preenche Account, mas PutDevice desreferencia os quatro
	// campos dele — um device so' e' salvo em producao depois do pareamento.
	device.Account = &waAdv.ADVSignedDeviceIdentity{
		Details:             []byte("details"),
		AccountSignature:    make([]byte, signedPreKeySignatureLength),
		AccountSignatureKey: make([]byte, curve25519KeyLength),
		DeviceSignature:     make([]byte, signedPreKeySignatureLength),
	}
	if err := device.Save(context.Background()); err != nil {
		t.Fatalf("salvar device: %v", err)
	}
	return container, device, NewSQLStore(container, jid)
}

// newTestStore e' o atalho para quem so' precisa do SQLStore.
func newTestStore(t *testing.T) *SQLStore {
	t.Helper()
	_, _, s := newTestDevice(t)
	return s
}

func mustJID(t *testing.T, user, server string) types.JID {
	t.Helper()
	return types.JID{User: user, Server: server}
}
