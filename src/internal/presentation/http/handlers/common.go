package handlers

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"strings"

	"github.com/nfnt/resize"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/types"
)

// ParseJID parses a WhatsApp JID from a string argument. If the string starts
// with '+', the leading '+' is stripped. If the string doesn't contain '@',
// DefaultUserServer is appended. Returns the parsed JID and whether parsing
// succeeded.
func ParseJID(arg string) (types.JID, bool) {
	if arg[0] == '+' {
		arg = arg[1:]
	}
	if !strings.ContainsRune(arg, '@') {
		return types.NewJID(arg, types.DefaultUserServer), true
	}

	recipient, err := types.ParseJID(arg)
	if err != nil {
		log.Error().Err(err).Msg("Invalid JID")
		return recipient, false
	}
	if recipient.User == "" {
		log.Error().Err(err).Msg("Invalid JID no server specified")
		return recipient, false
	}
	return recipient, true
}

// GetPlatformTypeEnum converts a platform type string to the corresponding
// DeviceProps_PlatformType enum. Returns DESKTOP as default for unknown types.
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

// JPEGThumbnail creates a JPEG thumbnail of the given image at the specified
// width and height using Lanczos resampling.
func JPEGThumbnail(img image.Image, width, height uint) ([]byte, error) {
	if img == nil {
		return nil, errors.New("cannot create thumbnail from a nil image")
	}
	thumb := resize.Thumbnail(width, height, img, resize.Lanczos3)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// FormatBlocklist formats a *types.Blocklist into a map suitable for JSON
// serialization.
func FormatBlocklist(blocklist *types.Blocklist) map[string]interface{} {
	jids := []string{}
	dhash := ""
	if blocklist != nil {
		jids = make([]string, len(blocklist.JIDs))
		for i, blockedJID := range blocklist.JIDs {
			jids[i] = blockedJID.String()
		}
		dhash = blocklist.DHash
	}
	return map[string]interface{}{
		"Blocklist": jids,
		"DHash":     dhash,
	}
}

// ValidateMessageFields validates the phone, stanza ID, and participant fields
// of a WhatsApp message payload. Returns the parsed recipient JID.
func ValidateMessageFields(phone string, stanzaid *string, participant *string) (types.JID, error) {
	recipient, ok := ParseJID(phone)
	if !ok {
		return types.NewJID("", types.DefaultUserServer), errors.New("could not parse Phone")
	}
	if stanzaid != nil {
		if participant == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing Participant in ContextInfo")
		}
	}
	if participant != nil {
		if stanzaid == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing StanzaID in ContextInfo")
		}
	}
	return recipient, nil
}

// userInfo interface matches the type from profile_handler.go
type userInfo interface {
	Get(key string) string
}

// Common error definitions used across handlers
var (
	errUnauthorized     = &simpleErr{"unauthorized"}
	errMissingSessionID = &simpleErr{"missing session id"}
	errMissingPhone     = &simpleErr{"missing phone"}
	errMissingJID       = &simpleErr{"missing jid"}
	errMissingID        = &simpleErr{"missing ID"}
	errDecodePayload    = &simpleErr{"could not decode payload"}
)

type simpleErr struct {
	msg string
}

func (e simpleErr) Error() string {
	return e.msg
}
