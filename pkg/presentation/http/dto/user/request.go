package user

import (
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// Request DTOs decode the body, validate it, and produce the use case's input.
//
// The `json` tags are the CANONICAL names, same rule as the responses: this
// family used to read `{"Phone": …}` and `{"JID": …}`, which are Go field
// names that reached the wire because the domain struct WAS the request type.
//
// Each type embeds domain.ChatTarget so the `chat` alias keeps working —
// decodeRequest calls ResolveChat after decoding, and dropping that here would
// silently break every caller that uses the alias.

// Error codes of this family's validators, as named constants: the tests that
// lock the contract assert the SAME strings production returns (ADR-0004).
const (
	// CodeMissingPhone is answered when a route that needs a target got none.
	CodeMissingPhone = "missing_phone"
	// CodeMissingPhoneOrJID is answered when neither identity was supplied.
	CodeMissingPhoneOrJID = "missing_phone_or_jid"

	missingPhoneMsg      = "missing phone in payload"
	missingPhoneOrJIDMsg = "missing phone or jid in payload"
)

// CheckUserRequest is the body of POST /user/check and POST /user/info.
type CheckUserRequest struct {
	// Phone accepts bare numbers as well as qualified JIDs; the handler
	// appends the WhatsApp server to the bare ones (F242/F225).
	Phone []string `json:"phone"`
}

// Validate refuses a request that cannot produce a domain command.
func (r CheckUserRequest) Validate() error {
	if len(r.Phone) == 0 {
		return apperr.New(CodeMissingPhone, apperr.CategoryValidation,
			missingPhoneMsg, false, nil)
	}
	return nil
}

// ToDomain produces the use case input. Only called after Validate.
func (r CheckUserRequest) ToDomain() domain.CheckUserRequest {
	return domain.CheckUserRequest{Phone: r.Phone}
}

// BlockUserRequest is the body of POST /user/block and POST /user/unblock.
//
// One type for both routes: the payload is identical and the action comes from
// the path. Two types would be the same shape waiting to diverge.
type BlockUserRequest struct {
	domain.ChatTarget
	Phone string `json:"phone"`
	JID   string `json:"jid"`
}

// ResolveChat copies the `chat` alias into Phone when Phone is empty.
func (r *BlockUserRequest) ResolveChat() { domain.ResolveChatField(&r.Phone, r.ChatAlias) }

// Validate refuses a request with no target at all.
//
// It does NOT decide which of the two fields wins — that is the use case's
// rule (JID first, Phone as fallback) and duplicating it here would be the
// same rule in two places waiting to diverge.
func (r BlockUserRequest) Validate() error {
	if r.JID == "" && r.Phone == "" {
		return apperr.New(CodeMissingPhoneOrJID, apperr.CategoryValidation,
			missingPhoneOrJIDMsg, false, nil)
	}
	return nil
}

// ToBlockDomain produces the block use case's input.
func (r BlockUserRequest) ToBlockDomain() domain.BlockUserRequest {
	return domain.BlockUserRequest{Phone: r.Phone, JID: r.JID}
}

// ToUnblockDomain produces the unblock use case's input.
func (r BlockUserRequest) ToUnblockDomain() domain.UnblockUserRequest {
	return domain.UnblockUserRequest{Phone: r.Phone, JID: r.JID}
}

// GetAvatarRequest is the body of POST /user/avatar.
type GetAvatarRequest struct {
	domain.ChatTarget
	Phone string `json:"phone"`
	// Preview asks for the thumbnail instead of the full-size picture. A
	// plain bool and not a pointer: "not sent" and "sent false" both mean
	// the same thing here — give me the full picture.
	Preview bool `json:"preview"`
}

// ResolveChat copies the `chat` alias into Phone when Phone is empty.
func (r *GetAvatarRequest) ResolveChat() { domain.ResolveChatField(&r.Phone, r.ChatAlias) }

// Validate refuses a request with no target.
func (r GetAvatarRequest) Validate() error {
	if r.Phone == "" {
		return apperr.New(CodeMissingPhone, apperr.CategoryValidation,
			missingPhoneMsg, false, nil)
	}
	return nil
}

// ToDomain produces the use case input. Only called after Validate.
func (r GetAvatarRequest) ToDomain() domain.GetAvatarRequest {
	return domain.GetAvatarRequest{Phone: r.Phone, Preview: r.Preview}
}

// SetPrivacySettingRequest is the body of POST /user/privacy.
type SetPrivacySettingRequest struct {
	PrivacySetting string `json:"privacy_setting"`
	Value          string `json:"value"`
}

// Validate defers to the domain, which owns the table of accepted settings
// and values (domain.ValidatePrivacySetting). Re-listing them here would be
// the same table in two places.
func (r SetPrivacySettingRequest) Validate() error {
	return domain.ValidatePrivacySetting(r.PrivacySetting, r.Value)
}

// ToDomain produces the use case input. Only called after Validate.
func (r SetPrivacySettingRequest) ToDomain() domain.SetPrivacySettingRequest {
	return domain.SetPrivacySettingRequest{
		PrivacySetting: r.PrivacySetting,
		Value:          r.Value,
	}
}
