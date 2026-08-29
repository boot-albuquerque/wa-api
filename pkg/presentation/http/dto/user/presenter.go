package user

import (
	"sort"
	"time"

	ucuser "wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// Presenters are hand-written, one function per type, field by field.
//
// No reflection, no generic struct copier, no `json` round-trip. The property
// being bought is a COMPILE ERROR: rename a field in domain.UserInfo and this
// file stops building. A reflection-based mapper would keep building and would
// silently drop the key from every response, which is the exact failure the
// DTO layer exists to prevent.

// PresentWhatsAppCheck maps one phone-check answer.
func PresentWhatsAppCheck(c ucuser.CheckUserResult) WhatsAppCheckResponse {
	return WhatsAppCheckResponse{
		Query:        c.Query,
		IsInWhatsapp: c.IsInWhatsapp,
		JID:          c.JID,
		VerifiedName: c.VerifiedName,
	}
}

// PresentCheckUser maps the use case result onto the route's `data`.
func PresentCheckUser(results []ucuser.CheckUserResult) CheckUserResponse {
	out := CheckUserResponse{Users: make([]WhatsAppCheckResponse, 0, len(results))}
	for _, c := range results {
		out.Users = append(out.Users, PresentWhatsAppCheck(c))
	}
	return out
}

// PresentUserInfo maps one contact's metadata.
func PresentUserInfo(u domain.UserInfo) UserInfoResponse {
	// Allocated even for a contact with no device: `[]` and `null` are
	// different values to every client, and only one of them can be ranged
	// over without a check.
	devices := make([]string, 0, len(u.Devices))
	for _, d := range u.Devices {
		devices = append(devices, string(d))
	}
	return UserInfoResponse{
		JID:          string(u.JID),
		LID:          string(u.LID),
		Status:       u.Status,
		PictureID:    u.PictureID,
		VerifiedName: u.VerifiedName,
		Devices:      devices,
		PushName:     u.PushName,
		BusinessName: u.BusinessName,
	}
}

// presentUserInfoList maps a slice, always allocating.
func presentUserInfoList(users []domain.UserInfo) []UserInfoResponse {
	out := make([]UserInfoResponse, 0, len(users))
	for _, u := range users {
		out = append(out, PresentUserInfo(u))
	}
	return out
}

// PresentGetUserInfo maps the use case result onto the route's `data`.
func PresentGetUserInfo(users []domain.UserInfo) GetUserInfoResponse {
	return GetUserInfoResponse{Users: presentUserInfoList(users)}
}

// PresentGetUserLID maps the LID lookup. A nil input presents as nil, so the
// caller does not have to branch before calling.
func PresentGetUserLID(r *ucuser.LIDResult) *GetUserLIDResponse {
	if r == nil {
		return nil
	}
	return &GetUserLIDResponse{JID: r.JID, LID: r.LID}
}

// PresentUserProfile maps the consolidated profile.
func PresentUserProfile(r *ucuser.UserProfileResult) *UserProfileResponse {
	if r == nil {
		return nil
	}
	out := &UserProfileResponse{
		JID:          r.JID,
		LID:          r.LID,
		Query:        r.Query,
		OnWhatsApp:   r.OnWhatsApp,
		VerifiedName: r.VerifiedName,
		AvatarURL:    r.AvatarURL,
		AvatarID:     r.AvatarID,
		UserInfo:     presentUserInfoList(r.UserInfo),
		// The use case sets Unavailable to nil when nothing failed. The wire
		// gets `{}` instead: a client iterating the reasons should not have to
		// null-check a map whose emptiness already says "nothing failed".
		Unavailable: map[string]string{},
	}
	for k, v := range r.Unavailable {
		out.Unavailable[k] = v
	}
	return out
}

// PresentAvatar maps a profile picture. A nil input presents as nil.
func PresentAvatar(a *domain.AvatarInfo) *AvatarResponse {
	if a == nil {
		return nil
	}
	return &AvatarResponse{ID: a.ID, URL: a.URL}
}

// PresentContact maps one address-book entry.
func PresentContact(c domain.Contact) ContactResponse {
	return ContactResponse{
		JID:          string(c.JID),
		PhoneNumber:  string(c.PN),
		LID:          string(c.LID),
		Found:        c.Found,
		FirstName:    c.FirstName,
		FullName:     c.FullName,
		PushName:     c.PushName,
		BusinessName: c.BusinessName,
		IsBusiness:   c.IsBusiness,
	}
}

// PresentGetContacts maps the address book onto the route's `data`.
func PresentGetContacts(contacts []domain.Contact) GetContactsResponse {
	out := GetContactsResponse{
		Contacts: make([]ContactResponse, 0, len(contacts)),
		Count:    len(contacts),
	}
	for _, c := range contacts {
		out.Contacts = append(out.Contacts, PresentContact(c))
	}
	return out
}

// PresentContactsLastActivity turns the per-chat timestamp map into an ARRAY
// sorted by JID.
//
// Sorted, and not in map order: Go randomizes map iteration by design, so an
// unsorted answer would put the same chats in a different order on every call
// and no client could diff two responses.
func PresentContactsLastActivity(activity map[string]time.Time) GetContactsLastActivityResponse {
	out := GetContactsLastActivityResponse{
		Chats: make([]ChatLastActivityResponse, 0, len(activity)),
	}
	jids := make([]string, 0, len(activity))
	for jid := range activity {
		jids = append(jids, jid)
	}
	sort.Strings(jids)
	for _, jid := range jids {
		out.Chats = append(out.Chats, ChatLastActivityResponse{
			JID:          jid,
			LastActivity: presentTime(activity[jid]),
		})
	}
	return out
}

// PresentGetBlocklist maps the blocklist onto the route's `data`.
func PresentGetBlocklist(b domain.Blocklist) GetBlocklistResponse {
	out := GetBlocklistResponse{
		Blocklist: make([]string, 0, len(b.JIDs)),
		DHash:     b.DHash,
	}
	out.Blocklist = append(out.Blocklist, b.JIDs...)
	return out
}

// PresentBlockResult maps the answer to POST /user/block.
func PresentBlockResult(r *ucuser.BlockResult) *BlocklistUpdateResponse {
	if r == nil {
		return nil
	}
	return presentBlocklistUpdate(r.Details, r.JID, r.RequestedJID, r.Blocklist, r.DHash)
}

// PresentUnblockResult maps the answer to POST /user/unblock.
func PresentUnblockResult(r *ucuser.UnblockResult) *BlocklistUpdateResponse {
	if r == nil {
		return nil
	}
	return presentBlocklistUpdate(r.Details, r.JID, r.RequestedJID, r.Blocklist, r.DHash)
}

func presentBlocklistUpdate(details, jid, requestedJID string, list []string, dhash string) *BlocklistUpdateResponse {
	out := &BlocklistUpdateResponse{
		Details: details,
		JID:     jid,
		// The use case leaves RequestedJID empty when no translation
		// happened. The wire echoes the effective JID instead, so the field
		// always answers "which identity did you ask about?" rather than
		// meaning two things depending on whether it is empty.
		RequestedJID: requestedJID,
		Blocklist:    make([]string, 0, len(list)),
		DHash:        dhash,
	}
	if out.RequestedJID == "" {
		out.RequestedJID = jid
	}
	out.Blocklist = append(out.Blocklist, list...)
	return out
}

// PresentPrivacySettings maps the account's privacy choices.
func PresentPrivacySettings(s domain.PrivacySettings) PrivacySettingsResponse {
	return PrivacySettingsResponse{
		GroupAdd:     s.GroupAdd,
		LastSeen:     s.LastSeen,
		Status:       s.Status,
		Profile:      s.Profile,
		ReadReceipts: s.ReadReceipts,
		CallAdd:      s.CallAdd,
		Online:       s.Online,
		Messages:     s.Messages,
		Defense:      s.Defense,
		Stickers:     s.Stickers,
	}
}

// presentTime renders a timestamp as RFC 3339 in UTC, or null when there is no
// value for it.
//
// The zero time is null and not "0001-01-01T00:00:00Z": that string is a real
// date on the wire, and a client that parses it gets a chat last active before
// the Gregorian calendar instead of a missing field.
func presentTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
