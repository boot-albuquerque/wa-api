package constants

// SupportedEventTypes lists all event types the system recognizes.
var SupportedEventTypes = []string{
	"Message",
	"UndecryptableMessage",
	"Receipt",
	"MediaRetry",
	"ReadReceipt",
	"GroupInfo",
	"JoinedGroup",
	"Picture",
	"BlocklistChange",
	"Blocklist",
	"Connected",
	"Disconnected",
	"ConnectFailure",
	"KeepAliveRestored",
	"KeepAliveTimeout",
	"QRTimeout",
	"LoggedOut",
	"ClientOutdated",
	"TemporaryBan",
	"StreamError",
	"StreamReplaced",
	"PairSuccess",
	"PairError",
	"QR",
	"QRScannedWithoutMultidevice",
	"PrivacySettings",
	"PushNameSetting",
	"UserAbout",
	"AppState",
	"AppStateSyncComplete",
	"HistorySync",
	"OfflineSyncCompleted",
	"OfflineSyncPreview",
	"CallOffer",
	"CallAccept",
	"CallTerminate",
	"CallOfferNotice",
	"CallRelayLatency",
	"Presence",
	"ChatPresence",
	"IdentityChange",
	"CATRefreshError",
	"NewsletterJoin",
	"NewsletterLeave",
	"NewsletterMuteChange",
	"NewsletterLiveUpdate",
	"FBMessage",
	"All",
}

var eventTypeMap map[string]bool

func init() {
	eventTypeMap = make(map[string]bool)
	for _, eventType := range SupportedEventTypes {
		eventTypeMap[eventType] = true
	}
}

// IsValidEventType reports whether name is a recognized event type.
func IsValidEventType(name string) bool {
	return eventTypeMap[name]
}
