// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package pairing

import (
	"errors"
	"strings"
	"testing"

	"wa-api/internal/wa-noise/protocol/proto/waCompanionReg"
	"wa-api/internal/wa-noise/protocol/proto/waWa6"
	"wa-api/internal/wa-noise/store"
)

// O tipo configurado pelo chamador tem precedencia sobre qualquer deducao.
func TestDetectClientTypeUsaOConfigurado(t *testing.T) {
	if got := DetectClientType(ClientElectron); got != ClientElectron {
		t.Errorf("= %q, esperado %q", got, ClientElectron)
	}
}

// Sem tipo configurado, o tipo sai do PlatformType das DeviceProps globais.
//
// O teste mexe em globais de `store` e os restaura; por isso nao usa
// t.Parallel().
func TestDetectClientTypeDeduzDoPlatformType(t *testing.T) {
	originalPlatform := store.DeviceProps.PlatformType
	t.Cleanup(func() { store.DeviceProps.PlatformType = originalPlatform })

	for name, tc := range map[string]struct {
		platform waCompanionReg.DeviceProps_PlatformType
		want     ClientType
	}{
		"chrome":  {waCompanionReg.DeviceProps_CHROME, ClientChrome},
		"firefox": {waCompanionReg.DeviceProps_FIREFOX, ClientFirefox},
		"edge":    {waCompanionReg.DeviceProps_EDGE, ClientEdge},
		"ie":      {waCompanionReg.DeviceProps_IE, ClientIE},
		"opera":   {waCompanionReg.DeviceProps_OPERA, ClientOpera},
		"safari":  {waCompanionReg.DeviceProps_SAFARI, ClientSafari},
		"uwp":     {waCompanionReg.DeviceProps_UWP, ClientUWP},
		"android": {waCompanionReg.DeviceProps_ANDROID_PHONE, ClientAndroid},
	} {
		t.Run(name, func(t *testing.T) {
			store.DeviceProps.PlatformType = tc.platform.Enum()
			if got := DetectClientType(""); got != tc.want {
				t.Errorf("= %q, esperado %q", got, tc.want)
			}
		})
	}
}

// Quando o PlatformType nao cai em nenhum dos casos acima, a deducao passa
// para o UserAgent do client payload.
func TestDetectClientTypeCaiNoUserAgent(t *testing.T) {
	originalPlatform := store.DeviceProps.PlatformType
	originalUA := store.BaseClientPayload.UserAgent.Platform
	t.Cleanup(func() {
		store.DeviceProps.PlatformType = originalPlatform
		store.BaseClientPayload.UserAgent.Platform = originalUA
	})
	// DESKTOP nao esta' no primeiro switch, entao a deducao continua.
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_DESKTOP.Enum()

	for name, tc := range map[string]struct {
		platform waWa6.ClientPayload_UserAgent_Platform
		want     ClientType
	}{
		"web":          {waWa6.ClientPayload_UserAgent_WEB, ClientOtherWebClient},
		"macos":        {waWa6.ClientPayload_UserAgent_MACOS, ClientMacOS},
		"desconhecido": {waWa6.ClientPayload_UserAgent_ANDROID, ClientUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			store.BaseClientPayload.UserAgent.Platform = tc.platform.Enum()
			if got := DetectClientType(""); got != tc.want {
				t.Errorf("= %q, esperado %q", got, tc.want)
			}
		})
	}
}

// MakeQRData monta os cinco campos separados por virgula, na ordem do
// protocolo. Uma mudanca aqui quebra a leitura pelo celular.
func TestMakeQRData(t *testing.T) {
	tr := newFakeTransport(t)

	got := MakeQRData(tr, []byte("REF"), ClientSafari)

	const prefix = "https://wa.me/settings/linked_devices#"
	if len(got) <= len(prefix) || got[:len(prefix)] != prefix {
		t.Fatalf("= %q, sem o prefixo esperado", got)
	}
	fields := strings.Split(got[len(prefix):], ",")
	if len(fields) != 5 {
		t.Fatalf("%d campos, esperado 5: %q", len(fields), got)
	}
	if fields[0] != "REF" {
		t.Errorf("campo 0 = %q, esperado a ref crua", fields[0])
	}
	if fields[4] != string(ClientSafari) {
		t.Errorf("campo 4 = %q, esperado o tipo de cliente", fields[4])
	}
}

// Os dois tipos de erro do pareamento embrulham a causa, para que errors.Is
// funcione a partir do events.PairError que chega ao chamador.
func TestErrosDePareamentoDesembrulham(t *testing.T) {
	cause := errors.New("causa raiz")

	if got := (&ProtoError{"msg", cause}).Unwrap(); !errors.Is(got, cause) {
		t.Errorf("ProtoError.Unwrap = %v, esperado %v", got, cause)
	}
	if got := (&DatabaseError{"msg", cause}).Unwrap(); !errors.Is(got, cause) {
		t.Errorf("DatabaseError.Unwrap = %v, esperado %v", got, cause)
	}
	if got := (&ProtoError{"msg", cause}).Error(); got != "msg: causa raiz" {
		t.Errorf("ProtoError.Error = %q", got)
	}
	if got := (&DatabaseError{"msg", cause}).Error(); got != "msg: causa raiz" {
		t.Errorf("DatabaseError.Error = %q", got)
	}
}
