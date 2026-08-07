// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sqlstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

func TestNewRejectsInvalidAddress(t *testing.T) {
	_, err := New(context.Background(), "sqlite", "file:/caminho/que/nao/existe/x.db?_pragma=foreign_keys(1)", nil)
	if err == nil {
		t.Fatal("esperava erro ao abrir banco em caminho invalido")
	}
}

// Container.Upgrade recusa SQLite sem foreign keys. Sem elas, apagar um device
// deixaria orfas as sessoes, prekeys e app state keys dele — dado de chave
// criptografica sobrevivendo ao dono.
func TestUpgradeRequiresForeignKeysOnSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sem-fk.db")
	_, err := New(context.Background(), "sqlite", "file:"+path, nil)
	if err == nil {
		t.Fatal("esperava erro ao migrar SQLite sem foreign keys habilitadas")
	}
}

func TestNewWithWrappedDBDefaultsToNoopLogger(t *testing.T) {
	container := newTestContainer(t)
	if container.log == nil {
		t.Fatal("log nil deveria virar waLog.Noop, nao ficar nil")
	}
	// Prova que o logger nulo e' usavel (nao panica ao ser chamado).
	container.log.Debugf("mensagem de teste")
}

func TestNewDeviceGeneratesFreshKeys(t *testing.T) {
	container := newTestContainer(t)
	a := container.NewDevice()
	b := container.NewDevice()

	if a.NoiseKey == nil || a.IdentityKey == nil || a.SignedPreKey == nil {
		t.Fatalf("NewDevice deixou chave nil: %+v", a)
	}
	if *a.NoiseKey.Priv == *b.NoiseKey.Priv {
		t.Fatal("dois devices novos nao podem compartilhar a noise key")
	}
	if *a.IdentityKey.Priv == *b.IdentityKey.Priv {
		t.Fatal("dois devices novos nao podem compartilhar a identity key")
	}
	if len(a.AdvSecretKey) != advSecretKeyLength {
		t.Fatalf("AdvSecretKey tem %d bytes, esperava %d", len(a.AdvSecretKey), advSecretKeyLength)
	}
	if a.SignedPreKey.KeyID != initialSignedPreKeyID {
		t.Fatalf("a primeira signed pre key deveria ter ID %d, veio %d", initialSignedPreKeyID, a.SignedPreKey.KeyID)
	}
	if a.SignedPreKey.Signature == nil {
		t.Fatal("a signed pre key deveria vir assinada")
	}
	if a.Container == nil {
		t.Fatal("NewDevice deveria ligar o device ao container")
	}
}

// NewDevice nao inicializa os stores; so' PutDevice (que conhece o JID) faz
// isso. Salvar sem JID tem que falhar de forma explicita.
func TestPutDeviceWithoutJIDFails(t *testing.T) {
	container := newTestContainer(t)
	device := container.NewDevice()
	err := container.PutDevice(context.Background(), device)
	if !errors.Is(err, ErrDeviceIDMustBeSet) {
		t.Fatalf("esperava ErrDeviceIDMustBeSet, veio %v", err)
	}
}

func TestDeleteDeviceWithoutJIDFails(t *testing.T) {
	container := newTestContainer(t)
	err := container.DeleteDevice(context.Background(), container.NewDevice())
	if !errors.Is(err, ErrDeviceIDMustBeSet) {
		t.Fatalf("esperava ErrDeviceIDMustBeSet, veio %v", err)
	}
}

func TestPutDeviceInitializesStores(t *testing.T) {
	_, device, _ := newTestDevice(t)
	if !device.Initialized {
		t.Fatal("PutDevice deveria marcar o device como inicializado")
	}
	if device.Identities == nil || device.Sessions == nil || device.PreKeys == nil ||
		device.SenderKeys == nil || device.AppStateKeys == nil || device.AppState == nil ||
		device.Contacts == nil || device.ChatSettings == nil || device.MsgSecrets == nil ||
		device.PrivacyTokens == nil || device.NCTSalt == nil || device.EventBuffer == nil {
		t.Fatalf("PutDevice deixou store por sessao nil: %+v", device)
	}
	if device.LIDs == nil {
		t.Fatal("PutDevice deveria ligar o LIDStore global do container")
	}
}

func TestGetDeviceRoundTrip(t *testing.T) {
	ctx := context.Background()
	container, saved, _ := newTestDevice(t)

	loaded, err := container.GetDevice(ctx, saved.GetJID())
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if loaded == nil {
		t.Fatal("GetDevice devolveu nil para um device salvo")
	}
	if loaded.GetJID() != saved.GetJID() {
		t.Fatalf("JID = %s, esperava %s", loaded.GetJID(), saved.GetJID())
	}
	if loaded.GetLID() != saved.GetLID() {
		t.Fatalf("LID = %s, esperava %s", loaded.GetLID(), saved.GetLID())
	}
	if loaded.RegistrationID != saved.RegistrationID {
		t.Fatalf("RegistrationID = %d, esperava %d", loaded.RegistrationID, saved.RegistrationID)
	}
	if *loaded.NoiseKey.Priv != *saved.NoiseKey.Priv {
		t.Fatal("a noise key nao voltou igual")
	}
	if *loaded.IdentityKey.Priv != *saved.IdentityKey.Priv {
		t.Fatal("a identity key nao voltou igual")
	}
	if *loaded.SignedPreKey.Priv != *saved.SignedPreKey.Priv {
		t.Fatal("a signed pre key nao voltou igual")
	}
	if *loaded.SignedPreKey.Signature != *saved.SignedPreKey.Signature {
		t.Fatal("a assinatura da signed pre key nao voltou igual")
	}
	if !loaded.Initialized {
		t.Fatal("o device carregado deveria vir com os stores ja' inicializados")
	}
}

func TestGetDeviceMissingReturnsNil(t *testing.T) {
	container := newTestContainer(t)
	device, err := container.GetDevice(context.Background(), mustJID(t, "5511000000000", "s.whatsapp.net"))
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if device != nil {
		t.Fatalf("device inexistente deveria devolver nil, veio %+v", device)
	}
}

// PutDevice tem ON CONFLICT (jid) DO UPDATE que atualiza SO' os campos
// mutaveis (lid, platform, business_name, push_name, lid_migration_ts). As
// chaves criptograficas ficam de fora de proposito: reescreve-las invalidaria
// todas as sessoes existentes.
func TestPutDeviceUpdatesMutableFieldsOnly(t *testing.T) {
	ctx := context.Background()
	container, device, _ := newTestDevice(t)
	originalNoiseKey := *device.NoiseKey.Priv

	device.PushName = "Novo Nome"
	device.Platform = "web"
	device.BusinessName = "Loja"
	device.LIDMigrationTimestamp = 12345
	if err := device.Save(ctx); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := container.GetDevice(ctx, device.GetJID())
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if loaded.PushName != "Novo Nome" || loaded.Platform != "web" || loaded.BusinessName != "Loja" {
		t.Fatalf("os campos mutaveis nao foram atualizados: %+v", loaded)
	}
	if loaded.LIDMigrationTimestamp != 12345 {
		t.Fatalf("LIDMigrationTimestamp = %d", loaded.LIDMigrationTimestamp)
	}
	if *loaded.NoiseKey.Priv != originalNoiseKey {
		t.Fatal("a noise key foi reescrita — isso invalidaria as sessoes existentes")
	}
}

func TestGetAllDevicesReturnsEverySavedDevice(t *testing.T) {
	ctx := context.Background()
	container, first, _ := newTestDevice(t)

	second := container.NewDevice()
	jid := types.JID{User: "5511444444444", Server: types.DefaultUserServer}
	second.ID = &jid
	second.Account = first.Account
	if err := second.Save(ctx); err != nil {
		t.Fatalf("Save segundo device: %v", err)
	}

	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		t.Fatalf("GetAllDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("esperava 2 devices, veio %d", len(devices))
	}
}

func TestGetAllDevicesOnEmptyContainerReturnsEmptySlice(t *testing.T) {
	devices, err := newTestContainer(t).GetAllDevices(context.Background())
	if err != nil {
		t.Fatalf("GetAllDevices: %v", err)
	}
	if devices == nil {
		t.Fatal("GetAllDevices deveria devolver slice vazio, nao nil")
	}
	if len(devices) != 0 {
		t.Fatalf("esperava 0 devices, veio %d", len(devices))
	}
}

// GetFirstDevice em banco vazio CRIA um device novo (ainda nao persistido) em
// vez de devolver nil — e' o atalho de bootstrap de sessao unica.
func TestGetFirstDeviceCreatesWhenEmpty(t *testing.T) {
	container := newTestContainer(t)
	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		t.Fatalf("GetFirstDevice: %v", err)
	}
	if device == nil {
		t.Fatal("GetFirstDevice devolveu nil em banco vazio")
	}
	if device.ID != nil {
		t.Fatalf("o device criado ainda nao deveria ter JID, veio %s", device.GetJID())
	}
}

func TestGetFirstDeviceReturnsExisting(t *testing.T) {
	container, saved, _ := newTestDevice(t)
	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		t.Fatalf("GetFirstDevice: %v", err)
	}
	if device.GetJID() != saved.GetJID() {
		t.Fatalf("GetFirstDevice = %s, esperava %s", device.GetJID(), saved.GetJID())
	}
}

func TestDeleteDeviceRemovesItAndItsSessions(t *testing.T) {
	ctx := context.Background()
	container, device, s := newTestDevice(t)
	if err := s.PutSession(ctx, "alice:0", []byte("sessao")); err != nil {
		t.Fatalf("PutSession: %v", err)
	}

	jid := device.GetJID()
	if err := device.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	loaded, err := container.GetDevice(ctx, jid)
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if loaded != nil {
		t.Fatalf("o device deveria ter sido apagado, veio %+v", loaded)
	}
	// ON DELETE CASCADE: a sessao tem que ir junto.
	sess, err := s.GetSession(ctx, "alice:0")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess != nil {
		t.Fatalf("a sessao deveria ter sido apagada em cascata, veio %q", sess)
	}
}

// Depois de Delete, o Device troca todos os stores por NoopStore{ErrDeviceDeleted}.
// Isso e' o que impede um Client sobrevivente de continuar gravando chaves de
// uma sessao ja' encerrada.
func TestDeletedDeviceRejectsFurtherUse(t *testing.T) {
	ctx := context.Background()
	_, device, _ := newTestDevice(t)
	if err := device.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := device.Sessions.PutSession(ctx, "alice:0", []byte("x")); !errors.Is(err, store.ErrDeviceDeleted) {
		t.Fatalf("esperava ErrDeviceDeleted, veio %v", err)
	}
	if err := device.Save(ctx); !errors.Is(err, store.ErrDeviceDeleted) {
		t.Fatalf("Save apos Delete deveria falhar com ErrDeviceDeleted, veio %v", err)
	}
	if device.ID != nil {
		t.Fatal("Delete deveria zerar o JID do device")
	}
}

func TestDeleteDeviceTwiceIsNoOp(t *testing.T) {
	ctx := context.Background()
	_, device, _ := newTestDevice(t)
	if err := device.Delete(ctx); err != nil {
		t.Fatalf("Delete 1: %v", err)
	}
	if err := device.Delete(ctx); err != nil {
		t.Fatalf("Delete 2 deveria ser no-op, veio %v", err)
	}
}

func TestCloseOnNilContainerIsSafe(t *testing.T) {
	var container *Container
	if err := container.Close(); err != nil {
		t.Fatalf("Close em container nil: %v", err)
	}
}

func TestUpgradeIsIdempotent(t *testing.T) {
	container := newTestContainer(t)
	if err := container.Upgrade(context.Background()); err != nil {
		t.Fatalf("Upgrade repetido: %v", err)
	}
}
