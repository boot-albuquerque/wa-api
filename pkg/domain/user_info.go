package domain

// This file holds the contact/user metadata that used to cross the ports as
// `any`, which meant the ENGINE decided the JSON shape: the wa-noise adapter
// handed the SDK's map[types.JID]types.UserInfo straight through, and the
// headless adapter handed a map[JID]ContactName. Two different bodies for the
// same route, neither declared anywhere. See docs/HTTP-DTO-CONVENTIONS.md §1.
//
// The types below are Go-idiomatic on purpose — PascalCase, NO `json` tags.
// They are not the wire any more; pkg/presentation/http/dto/user is.

// UserInfo is what one contact's metadata looks like once both engines have
// normalized it.
//
// Fields an engine cannot know stay at their zero value. That is a measured
// property, not a gap: the headless build reads the page's contact collection,
// which carries no status and no device list, while wa-noise asks the server
// and gets both but knows no local names.
type UserInfo struct {
	// JID is the identity the caller asked about, echoed so a client that
	// requested several can match answers to questions without relying on
	// order.
	JID JID

	// LID is the privacy identity the server reports for this contact, empty
	// when the engine does not know it.
	LID JID

	// Status is the "about" text.
	Status string

	// PictureID identifies the current profile photo without downloading it.
	PictureID string

	// VerifiedName is the business's verified name, empty for a person.
	VerifiedName string

	// Devices are the contact's paired devices, as the server reports them.
	Devices []JID

	// PushName and BusinessName come from the LOCAL roster, which is what the
	// headless engine can answer with.
	PushName     string
	BusinessName string
}

// Contact is one person in the session's address book.
//
// PN and LID are both carried because a person is ONE entry with TWO
// identities, and which one a caller holds depends on where it read it — the
// message history speaks phone, the message collection speaks lid (H129).
type Contact struct {
	// JID is the identity to address this contact by. It is the LID when the
	// engine knows one, because that is what the send path measured as
	// working; the phone identity otherwise.
	JID JID

	// PN is the phone identity, empty when only a lid is known.
	PN JID
	// LID is the server identity, empty when only a phone is known.
	LID JID

	// Found reports that the engine actually has a record for this contact,
	// as opposed to having synthesized an entry from a bare identity.
	Found bool

	FirstName    string
	FullName     string
	PushName     string
	BusinessName string

	// IsBusiness is only answered by the headless engine today; wa-noise's
	// contact store does not carry the flag.
	IsBusiness bool
}

// PrivacySettings are the account's privacy choices, one string per setting.
//
// Strings, and not an enum type per setting, because the accepted values are
// already declared once in privacy.go (ValidatePrivacySetting) and a second
// declaration here would be the same table waiting to diverge.
//
// An empty value means the engine did not report that setting — the headless
// build answers none of them, and "" is distinguishable from every accepted
// value, which "all" would not be.
type PrivacySettings struct {
	GroupAdd     string
	LastSeen     string
	Status       string
	Profile      string
	ReadReceipts string
	CallAdd      string
	Online       string
	Messages     string
	Defense      string
	Stickers     string
}
