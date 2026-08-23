package domain

// List of supported event types
var SupportedEventTypes = []string{
	// Messages and Communication
	"Message",
	"UndecryptableMessage",
	"Receipt",
	"MediaRetry",
	"ReadReceipt",

	// Groups and Contacts
	"GroupInfo",
	"JoinedGroup",
	"Picture",
	"BlocklistChange",
	"Blocklist",

	// Connection and Session
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

	// Privacy and Settings
	"PrivacySettings",
	"PushNameSetting",
	// F73: PushName e BusinessName anunciam que um CONTACTO mudou de nome —
	// caso distinto do PushNameSetting, que é o nome do PRÓPRIO utilizador.
	// Sem eles aqui, um integrador não tinha como reagir a "o contacto mudou
	// de nome", e os eventos caíam no `default` do handler, poluindo o
	// "Unhandled event" que devia significar "apareceu algo que não
	// previmos".
	"PushName",
	"BusinessName",
	// F191: etiquetas. A biblioteca emite os três quando outro dispositivo
	// mexe numa etiqueta; sem eles aqui, o handler despacharia para ninguém.
	"LabelEdit",
	"LabelAssociationChat",
	"LabelAssociationMessage",
	"UserAbout",

	// Synchronization and State
	"AppState",
	"AppStateSyncComplete",
	"HistorySync",
	"OfflineSyncCompleted",
	"OfflineSyncPreview",

	// Calls
	"CallOffer",
	"CallAccept",
	"CallTerminate",
	"CallOfferNotice",
	"CallRelayLatency",

	// Presence and Activity
	"Presence",
	"ChatPresence",

	// Identity
	"IdentityChange",

	// Erros
	"CATRefreshError",

	// Newsletter (WhatsApp Channels)
	"NewsletterJoin",
	"NewsletterLeave",
	"NewsletterMuteChange",
	"NewsletterLiveUpdate",

	// Facebook/Meta Bridge
	"FBMessage",

	// Special - receives all events
	"All",
}

// Map for quick validation
var eventTypeMap map[string]bool

func init() {
	eventTypeMap = make(map[string]bool)
	for _, eventType := range SupportedEventTypes {
		eventTypeMap[eventType] = true
	}
}

// Auxiliary function to validate event type
func IsValidEventType(eventType string) bool {
	return eventTypeMap[eventType]
}
