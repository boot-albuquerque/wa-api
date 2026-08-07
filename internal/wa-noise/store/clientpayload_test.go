// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package store

import (
	"crypto/md5"
	"testing"

	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/security/keys"
)

// waVersion, waVersionHash, BaseClientPayload e DeviceProps sao estado GLOBAL
// mutavel do pacote. Qualquer teste que chame SetWAVersion ou SetOSInfo precisa
// restaurar o que havia antes, senao contamina os demais.
func restoreGlobals(t *testing.T) {
	t.Helper()
	originalVersion := waVersion
	originalOS := *DeviceProps.Os
	originalOSVersion := BaseClientPayload.UserAgent.GetOsVersion()
	originalPropsVersion := DeviceProps.Version
	t.Cleanup(func() {
		waVersion = originalVersion
		waVersionHash = originalVersion.Hash()
		BaseClientPayload.UserAgent.AppVersion = originalVersion.ProtoAppVersion()
		DeviceProps.Os = &originalOS
		DeviceProps.Version = originalPropsVersion
		BaseClientPayload.UserAgent.OsVersion = &originalOSVersion
		BaseClientPayload.UserAgent.OsBuildNumber = &originalOSVersion
	})
}

func TestParseVersion(t *testing.T) {
	got, err := ParseVersion("2.3000.1039406452")
	if err != nil {
		t.Fatalf("ParseVersion: %v", err)
	}
	if got != (WAVersionContainer{2, 3000, 1039406452}) {
		t.Fatalf("ParseVersion = %v", got)
	}
}

func TestParseVersionRejectsMalformed(t *testing.T) {
	for _, input := range []string{"", "1", "1.2", "1.2.3.4", "a.2.3", "1.b.3", "1.2.c"} {
		if _, err := ParseVersion(input); err == nil {
			t.Errorf("ParseVersion(%q) deveria falhar", input)
		}
	}
}

func TestWAVersionContainerLessThan(t *testing.T) {
	base := WAVersionContainer{2, 3000, 100}
	for _, tc := range []struct {
		other WAVersionContainer
		want  bool
	}{
		{WAVersionContainer{3, 0, 0}, true},
		{WAVersionContainer{2, 3001, 0}, true},
		{WAVersionContainer{2, 3000, 101}, true},
		{WAVersionContainer{2, 3000, 100}, false},
		{WAVersionContainer{2, 3000, 99}, false},
		{WAVersionContainer{1, 9999, 9999}, false},
	} {
		if got := base.LessThan(tc.other); got != tc.want {
			t.Errorf("%v.LessThan(%v) = %v, esperava %v", base, tc.other, got, tc.want)
		}
	}
}

func TestWAVersionContainerIsZeroAndString(t *testing.T) {
	if !(WAVersionContainer{}).IsZero() {
		t.Fatal("a versao zerada deveria ser IsZero")
	}
	if (WAVersionContainer{0, 0, 1}).IsZero() {
		t.Fatal("uma versao com parte nao-zero nao e' IsZero")
	}
	if got := (WAVersionContainer{2, 3000, 42}).String(); got != "2.3000.42" {
		t.Fatalf("String = %q", got)
	}
}

func TestParseVersionStringRoundTrip(t *testing.T) {
	const raw = "2.3000.1039406452"
	parsed, err := ParseVersion(raw)
	if err != nil {
		t.Fatalf("ParseVersion: %v", err)
	}
	if parsed.String() != raw {
		t.Fatalf("round trip: %q != %q", parsed.String(), raw)
	}
}

// O hash e' md5 da representacao em string — e' o BuildHash enviado ao servidor
// no payload de registro, entao a formula precisa estar travada.
func TestWAVersionContainerHashIsMD5OfString(t *testing.T) {
	v := WAVersionContainer{2, 3000, 42}
	want := md5.Sum([]byte("2.3000.42"))
	if v.Hash() != want {
		t.Fatal("Hash deveria ser md5 da String")
	}
}

func TestProtoAppVersionMirrorsParts(t *testing.T) {
	v := WAVersionContainer{2, 3000, 42}
	pv := v.ProtoAppVersion()
	if pv.GetPrimary() != 2 || pv.GetSecondary() != 3000 || pv.GetTertiary() != 42 {
		t.Fatalf("ProtoAppVersion = %v", pv)
	}
}

func TestGetWAVersionIsNonZero(t *testing.T) {
	if GetWAVersion().IsZero() {
		t.Fatal("a versao embarcada nao pode ser zero")
	}
}

// SetWAVersion tem que atualizar TRES coisas: a variavel, o hash derivado e o
// AppVersion dentro de BaseClientPayload. Esquecer o hash faria o servidor
// receber build hash de uma versao e numero de outra.
func TestSetWAVersionUpdatesVersionHashAndPayload(t *testing.T) {
	restoreGlobals(t)
	novo := WAVersionContainer{9, 8, 7}
	SetWAVersion(novo)

	if GetWAVersion() != novo {
		t.Fatalf("GetWAVersion = %v", GetWAVersion())
	}
	if waVersionHash != novo.Hash() {
		t.Fatal("SetWAVersion nao atualizou waVersionHash")
	}
	pv := BaseClientPayload.UserAgent.GetAppVersion()
	if pv.GetPrimary() != 9 || pv.GetSecondary() != 8 || pv.GetTertiary() != 7 {
		t.Fatalf("BaseClientPayload.UserAgent.AppVersion = %v", pv)
	}
}

func TestSetWAVersionIgnoresZero(t *testing.T) {
	restoreGlobals(t)
	antes := GetWAVersion()
	SetWAVersion(WAVersionContainer{})
	if GetWAVersion() != antes {
		t.Fatalf("SetWAVersion(zero) alterou a versao para %v", GetWAVersion())
	}
}

func TestSetOSInfoUpdatesDevicePropsAndUserAgent(t *testing.T) {
	restoreGlobals(t)
	SetOSInfo("meuOS", [3]uint32{1, 2, 3})

	if *DeviceProps.Os != "meuOS" {
		t.Fatalf("DeviceProps.Os = %q", *DeviceProps.Os)
	}
	if DeviceProps.Version.GetPrimary() != 1 || DeviceProps.Version.GetSecondary() != 2 ||
		DeviceProps.Version.GetTertiary() != 3 {
		t.Fatalf("DeviceProps.Version = %v", DeviceProps.Version)
	}
	if got := BaseClientPayload.UserAgent.GetOsVersion(); got != "1.2.3" {
		t.Fatalf("OsVersion = %q", got)
	}
	if BaseClientPayload.UserAgent.GetOsBuildNumber() != "1.2.3" {
		t.Fatalf("OsBuildNumber = %q", BaseClientPayload.UserAgent.GetOsBuildNumber())
	}
}

func newPayloadDevice() *Device {
	device := &Device{
		IdentityKey:    keys.NewKeyPair(),
		NoiseKey:       keys.NewKeyPair(),
		RegistrationID: 0x01020304,
	}
	device.SignedPreKey = device.IdentityKey.CreateSignedPreKey(0x00AABBCC)
	return device
}

// Sem JID, GetClientPayload monta o payload de REGISTRO (pareamento). Com JID,
// o de LOGIN. Trocar os dois quebraria o pareamento ou a reconexao.
func TestGetClientPayloadWithoutJIDIsRegistration(t *testing.T) {
	device := newPayloadDevice()
	payload := device.GetClientPayload()

	if payload.GetDevicePairingData() == nil {
		t.Fatal("sem JID o payload deveria ser de registro (DevicePairingData preenchido)")
	}
	if payload.GetPassive() {
		t.Fatal("o payload de registro deveria ter Passive=false")
	}
	if payload.GetPull() {
		t.Fatal("o payload de registro deveria ter Pull=false")
	}
	if payload.Username != nil {
		t.Fatal("o payload de registro nao deveria ter Username")
	}
}

func TestRegistrationPayloadEncodesIDsBigEndian(t *testing.T) {
	device := newPayloadDevice()
	pairing := device.GetClientPayload().GetDevicePairingData()

	if want := []byte{0x01, 0x02, 0x03, 0x04}; string(pairing.GetERegid()) != string(want) {
		t.Fatalf("ERegid = %x, esperava %x", pairing.GetERegid(), want)
	}
	// ESkeyID e' o uint32 big-endian SEM o primeiro byte (o protocolo usa 3 bytes).
	if want := []byte{0x00, 0xAA, 0xBB, 0xCC}[1:]; string(pairing.GetESkeyID()) != string(want) {
		t.Fatalf("ESkeyID = %x, esperava %x", pairing.GetESkeyID(), want)
	}
	if string(pairing.GetEIdent()) != string(device.IdentityKey.Pub[:]) {
		t.Fatal("EIdent deveria ser a identity key publica")
	}
	if string(pairing.GetESkeyVal()) != string(device.SignedPreKey.Pub[:]) {
		t.Fatal("ESkeyVal deveria ser a signed pre key publica")
	}
	if string(pairing.GetESkeySig()) != string(device.SignedPreKey.Signature[:]) {
		t.Fatal("ESkeySig deveria ser a assinatura da signed pre key")
	}
	if string(pairing.GetBuildHash()) != string(waVersionHash[:]) {
		t.Fatal("BuildHash deveria ser o hash da versao vigente")
	}
	if len(pairing.GetDeviceProps()) == 0 {
		t.Fatal("DeviceProps deveria vir serializado")
	}
}

func TestGetClientPayloadWithJIDIsLogin(t *testing.T) {
	device := newPayloadDevice()
	jid := types.JID{User: "5511999999999", Device: 3, Server: types.DefaultUserServer}
	device.ID = &jid

	payload := device.GetClientPayload()
	if payload.GetDevicePairingData() != nil {
		t.Fatal("com JID o payload nao deveria ter DevicePairingData")
	}
	if payload.GetUsername() != jid.UserInt() {
		t.Fatalf("Username = %d, esperava %d", payload.GetUsername(), jid.UserInt())
	}
	if payload.GetDevice() != 3 {
		t.Fatalf("Device = %d", payload.GetDevice())
	}
	if !payload.GetPassive() || !payload.GetPull() {
		t.Fatal("o payload de login deveria ter Passive=true e Pull=true")
	}
	if !payload.GetLidDbMigrated() {
		t.Fatal("o payload de login deveria declarar LidDbMigrated")
	}
	if payload.GetLc() != 1 {
		t.Fatalf("Lc = %d, esperava 1", payload.GetLc())
	}
}

// GetClientPayload clona BaseClientPayload: mexer no payload devolvido nao pode
// contaminar o global usado pela proxima conexao.
func TestGetClientPayloadDoesNotMutateBase(t *testing.T) {
	device := newPayloadDevice()
	jid := types.JID{User: "5511999999999", Server: types.DefaultUserServer}
	device.ID = &jid

	payload := device.GetClientPayload()
	payload.Username = nil
	payload.UserAgent.Device = nil

	if BaseClientPayload.Username != nil {
		t.Fatal("BaseClientPayload foi contaminado com Username")
	}
	if BaseClientPayload.UserAgent.GetDevice() == "" {
		t.Fatal("BaseClientPayload.UserAgent.Device foi zerado pelo payload derivado")
	}
}

func TestGetClientPayloadPanicsOnEmptyJID(t *testing.T) {
	device := newPayloadDevice()
	device.ID = &types.EmptyJID

	defer func() {
		if recover() == nil {
			t.Fatal("GetClientPayload com JID vazio deveria panicar")
		}
	}()
	device.GetClientPayload()
}
