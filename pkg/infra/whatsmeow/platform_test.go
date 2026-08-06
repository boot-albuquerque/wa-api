package whatsmeow

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
)

// TestGetPlatformTypeEnum_Chrome mapeia "CHROME" para CHROME enum.
func TestGetPlatformTypeEnum_Chrome(t *testing.T) {
	e := GetPlatformTypeEnum("CHROME")
	if e == nil {
		t.Fatal("GetPlatformTypeEnum(CHROME) = nil")
	}
	if e.Number() != waCompanionReg.DeviceProps_CHROME.Number() {
		t.Errorf("CHROME number = %v, want %v", e.Number(), waCompanionReg.DeviceProps_CHROME.Number())
	}
}

// TestGetPlatformTypeEnum_ChromeLowerCase é case-insensitive.
func TestGetPlatformTypeEnum_ChromeLowerCase(t *testing.T) {
	e := GetPlatformTypeEnum("chrome")
	if e.Number() != waCompanionReg.DeviceProps_CHROME.Number() {
		t.Errorf("chrome number = %v, want CHROME", e.Number())
	}
}

// TestGetPlatformTypeEnum_WithWhitespace trim entrada.
func TestGetPlatformTypeEnum_WithWhitespace(t *testing.T) {
	e := GetPlatformTypeEnum("  FIREFOX  ")
	if e.Number() != waCompanionReg.DeviceProps_FIREFOX.Number() {
		t.Errorf("trimmed FIREFOX number = %v, want FIREFOX", e.Number())
	}
}

// TestGetPlatformTypeEnum_UnknownIsKnown: "UNKNOWN" é um platform type válido,
// não cai no default. O default é só para strings realmente desconhecidas.
func TestGetPlatformTypeEnum_UnknownIsKnown(t *testing.T) {
	e := GetPlatformTypeEnum("UNKNOWN")
	if e.Number() != waCompanionReg.DeviceProps_UNKNOWN.Number() {
		t.Errorf("UNKNOWN number = %v, want %v", e.Number(), waCompanionReg.DeviceProps_UNKNOWN.Number())
	}
}

// TestGetPlatformTypeEnum_EmptyStringDefaultsToDesktop.
func TestGetPlatformTypeEnum_EmptyStringDefaultsToDesktop(t *testing.T) {
	e := GetPlatformTypeEnum("")
	if e.Number() != waCompanionReg.DeviceProps_DESKTOP.Number() {
		t.Errorf("empty number = %v, want DESKTOP", e.Number())
	}
}

// TestGetPlatformTypeEnum_RandomStringDefaultsToDesktop.
func TestGetPlatformTypeEnum_RandomStringDefaultsToDesktop(t *testing.T) {
	e := GetPlatformTypeEnum("not-a-platform")
	if e.Number() != waCompanionReg.DeviceProps_DESKTOP.Number() {
		t.Errorf("garbage number = %v, want DESKTOP", e.Number())
	}
}

// TestGetPlatformTypeEnum_AllKnownDevices: itera pelos tipos conhecidos
// para garantir que o switch não tem regressão silenciosa.
func TestGetPlatformTypeEnum_AllKnownDevices(t *testing.T) {
	cases := []struct {
		in   string
		want waCompanionReg.DeviceProps_PlatformType
	}{
		{"UNKNOWN", waCompanionReg.DeviceProps_UNKNOWN},
		{"CHROME", waCompanionReg.DeviceProps_CHROME},
		{"FIREFOX", waCompanionReg.DeviceProps_FIREFOX},
		{"IE", waCompanionReg.DeviceProps_IE},
		{"OPERA", waCompanionReg.DeviceProps_OPERA},
		{"SAFARI", waCompanionReg.DeviceProps_SAFARI},
		{"EDGE", waCompanionReg.DeviceProps_EDGE},
		{"DESKTOP", waCompanionReg.DeviceProps_DESKTOP},
		{"IPAD", waCompanionReg.DeviceProps_IPAD},
		{"ANDROID_TABLET", waCompanionReg.DeviceProps_ANDROID_TABLET},
		{"OHANA", waCompanionReg.DeviceProps_OHANA},
		{"ALOHA", waCompanionReg.DeviceProps_ALOHA},
		{"CATALINA", waCompanionReg.DeviceProps_CATALINA},
		{"TCL_TV", waCompanionReg.DeviceProps_TCL_TV},
		{"IOS_PHONE", waCompanionReg.DeviceProps_IOS_PHONE},
		{"IOS_CATALYST", waCompanionReg.DeviceProps_IOS_CATALYST},
		{"ANDROID_PHONE", waCompanionReg.DeviceProps_ANDROID_PHONE},
		{"ANDROID_AMBIGUOUS", waCompanionReg.DeviceProps_ANDROID_AMBIGUOUS},
		{"WEAR_OS", waCompanionReg.DeviceProps_WEAR_OS},
		{"AR_WRIST", waCompanionReg.DeviceProps_AR_WRIST},
		{"AR_DEVICE", waCompanionReg.DeviceProps_AR_DEVICE},
		{"UWP", waCompanionReg.DeviceProps_UWP},
		{"VR", waCompanionReg.DeviceProps_VR},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			e := GetPlatformTypeEnum(tc.in)
			if e.Number() != tc.want.Number() {
				t.Errorf("%s number = %v, want %v", tc.in, e.Number(), tc.want.Number())
			}
		})
	}
}
