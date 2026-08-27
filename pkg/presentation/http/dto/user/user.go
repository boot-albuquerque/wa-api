// Package user holds the PUBLIC wire types for the user, contact and
// blocklist family of routes, plus the hand-written presenters that build them
// from domain values.
//
// Nothing here may be reused by the domain or by a use case: these types exist
// to be serialized, and every field name in them is a promise to a client.
// See docs/HTTP-DTO-CONVENTIONS.md.
//
// No `omitempty` on any response field. A client has to be able to tell "this
// contact has no status" from "this key was dropped", and omitempty makes the
// two identical on the wire.
package user

// --- /user/check -----------------------------------------------------------

// WhatsAppCheckResponse answers "does this number have an account?" for one
// queried phone.
type WhatsAppCheckResponse struct {
	// Query echoes what the client asked about, so an answer can be matched
	// to its question without relying on position.
	Query string `json:"query"`

	IsInWhatsapp bool   `json:"is_in_whatsapp"`
	JID          string `json:"jid"`
	VerifiedName string `json:"verified_name"`
}

// CheckUserResponse is the `data` body of POST /user/check.
type CheckUserResponse struct {
	Users []WhatsAppCheckResponse `json:"users"`
}

// --- /user/info and the user_info block of /user/profile -------------------

// UserInfoResponse is one contact's metadata, as served.
//
// Fields the engine could not answer come back at their zero value rather than
// missing: the wa-headless build knows names and identities and no status, and
// wa-noise the reverse. A client that has to branch on which engine served it
// would need to know which engine served it, which it does not.
type UserInfoResponse struct {
	JID string `json:"jid"`
	LID string `json:"lid"`

	Status       string `json:"status"`
	PictureID    string `json:"picture_id"`
	VerifiedName string `json:"verified_name"`

	// Devices is `[]` and never null for a contact with no paired device:
	// only one of the two can be ranged over without a check.
	Devices []string `json:"devices"`

	PushName     string `json:"push_name"`
	BusinessName string `json:"business_name"`
}

// GetUserInfoResponse is the `data` body of POST /user/info.
type GetUserInfoResponse struct {
	Users []UserInfoResponse `json:"users"`
}

// --- /user/lid/{jid} -------------------------------------------------------

// GetUserLIDResponse is the `data` body of GET /user/lid/{jid}.
type GetUserLIDResponse struct {
	JID string `json:"jid"`
	LID string `json:"lid"`
}

// --- /user/profile/{jid} ---------------------------------------------------

// UserProfileResponse is a contact's consolidated profile, as served.
type UserProfileResponse struct {
	// JID is the PHONE identity and LID the privacy one. Both are emitted
	// whenever a mapping exists, whichever was asked about — that is what
	// spares the client a second call.
	JID string `json:"jid"`
	LID string `json:"lid"`

	// Query echoes the identifier the client passed in the path.
	Query string `json:"query"`

	// OnWhatsApp is a POINTER because null and false are different answers:
	// null means the question could not be asked (see Unavailable), false
	// means it was asked and the number has no account.
	OnWhatsApp *bool `json:"on_whatsapp"`

	VerifiedName string `json:"verified_name"`

	AvatarURL string `json:"avatar_url"`
	AvatarID  string `json:"avatar_id"`

	UserInfo []UserInfoResponse `json:"user_info"`

	// Unavailable names what could NOT be fetched, with the reason. The
	// profile degrades instead of failing, but degrading in SILENCE is worse
	// than failing: the client would see empty fields without knowing whether
	// the datum is absent or the network is down.
	//
	// Its keys are fixed and canonical (`jid`, `lid`, `on_whatsapp`,
	// `user_info`, `avatar`) — they are field names, not data.
	Unavailable map[string]string `json:"unavailable"`
}

// --- /user/avatar ----------------------------------------------------------

// AvatarResponse is the `data` body of POST /user/avatar.
type AvatarResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// --- /user/contacts --------------------------------------------------------

// ContactResponse is one person in the session's address book.
type ContactResponse struct {
	// JID is the identity to address this contact by.
	JID string `json:"jid"`

	// PhoneNumber and LID are the two identities of the SAME person. Which
	// one a caller already holds depends on where it read it — the message
	// history speaks phone, the message collection speaks lid — so both are
	// served and the empty one means "this engine does not know it".
	PhoneNumber string `json:"phone_number"`
	LID         string `json:"lid"`

	Found bool `json:"found"`

	FirstName    string `json:"first_name"`
	FullName     string `json:"full_name"`
	PushName     string `json:"push_name"`
	BusinessName string `json:"business_name"`
	IsBusiness   bool   `json:"is_business"`
}

// GetContactsResponse is the `data` body of GET /user/contacts.
type GetContactsResponse struct {
	Contacts []ContactResponse `json:"contacts"`
	Count    int               `json:"count"`
}

// --- /user/contacts/last-activity ------------------------------------------

// ChatLastActivityResponse is the most recent message timestamp of one chat.
type ChatLastActivityResponse struct {
	JID string `json:"jid"`
	// LastActivity is RFC 3339 in UTC, or null when the timestamp is the zero
	// time — "0001-01-01T00:00:00Z" is a real date on the wire and a client
	// that parses it gets a chat that predates the Gregorian calendar.
	LastActivity *string `json:"last_activity"`
}

// GetContactsLastActivityResponse is the `data` body of
// GET /user/contacts/last-activity.
//
// An ARRAY, and not the map keyed by chat JID that this route used to serve.
// The keys of that map were JIDs — `5511999999999@s.whatsapp.net` — which are
// DATA, and the naming rule of docs/HTTP-DTO-CONVENTIONS.md §8 applies to
// every object key on the wire. A payload whose keys are data can never
// satisfy it, and the fix is to move the datum into a value.
type GetContactsLastActivityResponse struct {
	Chats []ChatLastActivityResponse `json:"chats"`
}

// --- /user/blocklist, /user/block, /user/unblock ---------------------------

// GetBlocklistResponse is the `data` body of GET /user/blocklist.
type GetBlocklistResponse struct {
	// Blocklist is `[]` and never null when nobody is blocked.
	Blocklist []string `json:"blocklist"`
	// DHash is the version hash WhatsApp keeps for the list. Empty means the
	// engine does not version it — the headless build does not.
	DHash string `json:"dhash"`
}

// BlocklistUpdateResponse is the `data` body of POST /user/block and
// POST /user/unblock. One type for both: the two answers carry exactly the
// same fields, and a second type would be the same shape waiting to diverge.
type BlocklistUpdateResponse struct {
	Details string `json:"details"`

	// JID is the identity the blocklist ACTUALLY used, and RequestedJID the
	// one that was asked for. They differ when a LID had to be translated to
	// a phone number, and a client that only saw one could not tell that a
	// translation happened.
	JID          string `json:"jid"`
	RequestedJID string `json:"requested_jid"`

	Blocklist []string `json:"blocklist"`
	DHash     string   `json:"dhash"`
}

// --- /user/privacy ---------------------------------------------------------

// PrivacySettingsResponse is the account's privacy choices.
//
// An empty value means the engine did not report that setting, which is
// distinguishable from every accepted value — "all" would not be.
type PrivacySettingsResponse struct {
	GroupAdd     string `json:"group_add"`
	LastSeen     string `json:"last_seen"`
	Status       string `json:"status"`
	Profile      string `json:"profile"`
	ReadReceipts string `json:"read_receipts"`
	CallAdd      string `json:"call_add"`
	Online       string `json:"online"`
	Messages     string `json:"messages"`
	Defense      string `json:"defense"`
	Stickers     string `json:"stickers"`
}
