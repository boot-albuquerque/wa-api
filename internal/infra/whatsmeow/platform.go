package whatsmeow

import (
	"strings"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
)

// GetPlatformTypeEnum converts a platform type string to the corresponding DeviceProps enum
// Returns DESKTOP as default if the string doesn't match any known type
func GetPlatformTypeEnum(platformType string) *waCompanionReg.DeviceProps_PlatformType {
	platformType = strings.ToUpper(strings.TrimSpace(platformType))

	switch platformType {
	case "UNKNOWN":
		return waCompanionReg.DeviceProps_UNKNOWN.Enum()
	case "CHROME":
		return waCompanionReg.DeviceProps_CHROME.Enum()
	case "FIREFOX":
		return waCompanionReg.DeviceProps_FIREFOX.Enum()
	case "IE":
		return waCompanionReg.DeviceProps_IE.Enum()
	case "OPERA":
		return waCompanionReg.DeviceProps_OPERA.Enum()
	case "SAFARI":
		return waCompanionReg.DeviceProps_SAFARI.Enum()
	case "EDGE":
		return waCompanionReg.DeviceProps_EDGE.Enum()
	case "DESKTOP":
		return waCompanionReg.DeviceProps_DESKTOP.Enum()
	case "IPAD":
		return waCompanionReg.DeviceProps_IPAD.Enum()
	case "ANDROID_TABLET":
		return waCompanionReg.DeviceProps_ANDROID_TABLET.Enum()
	case "OHANA":
		return waCompanionReg.DeviceProps_OHANA.Enum()
	case "ALOHA":
		return waCompanionReg.DeviceProps_ALOHA.Enum()
	case "CATALINA":
		return waCompanionReg.DeviceProps_CATALINA.Enum()
	case "TCL_TV":
		return waCompanionReg.DeviceProps_TCL_TV.Enum()
	case "IOS_PHONE":
		return waCompanionReg.DeviceProps_IOS_PHONE.Enum()
	case "IOS_CATALYST":
		return waCompanionReg.DeviceProps_IOS_CATALYST.Enum()
	case "ANDROID_PHONE":
		return waCompanionReg.DeviceProps_ANDROID_PHONE.Enum()
	case "ANDROID_AMBIGUOUS":
		return waCompanionReg.DeviceProps_ANDROID_AMBIGUOUS.Enum()
	case "WEAR_OS":
		return waCompanionReg.DeviceProps_WEAR_OS.Enum()
	case "AR_WRIST":
		return waCompanionReg.DeviceProps_AR_WRIST.Enum()
	case "AR_DEVICE":
		return waCompanionReg.DeviceProps_AR_DEVICE.Enum()
	case "UWP":
		return waCompanionReg.DeviceProps_UWP.Enum()
	case "VR":
		return waCompanionReg.DeviceProps_VR.Enum()
	default:
		log.Warn().Str("platformType", platformType).Msg("Unknown platform type, defaulting to DESKTOP")
		return waCompanionReg.DeviceProps_DESKTOP.Enum()
	}
}
