package admin

import "wa-api/pkg/domain"

// Presenters are hand-written, one function per type, field by field.
//
// No reflection, no generic struct copier, no `json` round-trip. The property
// being bought is a COMPILE ERROR: rename a field in domain.UserAccount and
// this file stops building. A reflection-based mapper would keep building and
// would silently drop the key from every response, which is the exact failure
// the DTO layer exists to prevent.

// statusOK and statusDeleted are the two values StatusResponse.Status can
// take. Named constants and not literals: a client branches on them, and a
// literal repeated in two handlers is the same bug waiting to diverge
// (ADR-0004).
const (
	statusOK      = "ok"
	statusDeleted = "deleted"
)

// PresentUser maps one user account.
func PresentUser(u domain.UserAccount) UserResponse {
	return UserResponse{
		ID:      u.ID,
		Name:    u.Name,
		Token:   u.Token,
		Webhook: u.Webhook,
		JID:     u.JID,
		QRCode:  u.QRCode,

		Connected: u.Connected,
		LoggedIn:  u.LoggedIn,

		Expiration: u.Expiration,
		Events:     u.Events,

		HmacConfigured: u.HmacConfigured,

		Engine: u.Engine,

		ProxyConfig: UserProxyConfigResponse{
			Enabled:         u.Proxy.Enabled,
			ProxyURL:        u.Proxy.URL,
			WebhookUseProxy: u.Proxy.WebhookUseProxy,
		},
		S3Config: UserS3ConfigResponse{
			Enabled:             u.S3.Enabled,
			Endpoint:            u.S3.Endpoint,
			Region:              u.S3.Region,
			Bucket:              u.S3.Bucket,
			PathStyle:           u.S3.PathStyle,
			PublicURL:           u.S3.PublicURL,
			MediaDelivery:       u.S3.MediaDelivery,
			RetentionDays:       u.S3.RetentionDays,
			AccessKeyConfigured: u.S3.AccessKeyConfigured,
		},
	}
}

// PresentUserPointer maps a user account behind a pointer. A nil input
// presents as nil, so the caller does not have to branch before calling.
func PresentUserPointer(u *domain.UserAccount) *UserResponse {
	if u == nil {
		return nil
	}
	out := PresentUser(*u)
	return &out
}

// PresentListUsers maps the listing.
//
// The slice is allocated even when there is nothing to list: `[]` and `null`
// are different values to every client, and only one of them can be iterated
// without a check. The route used to answer `null` for an empty installation,
// because the use case appended to a nil slice.
func PresentListUsers(users []domain.UserAccount) []UserResponse {
	out := make([]UserResponse, 0, len(users))
	for _, u := range users {
		out = append(out, PresentUser(u))
	}
	return out
}

// PresentEditUser is the answer of a successful PUT /admin/users/{id}.
func PresentEditUser() StatusResponse { return StatusResponse{Status: statusOK} }

// PresentDeleteUser is the answer of a successful DELETE /admin/users/{id}.
func PresentDeleteUser() StatusResponse { return StatusResponse{Status: statusDeleted} }

// PresentDeleteUserComplete maps the result of the full removal.
func PresentDeleteUserComplete(r *domain.DeleteUserCompleteResult) *DeleteUserCompleteResponse {
	if r == nil {
		return nil
	}
	return &DeleteUserCompleteResponse{
		ID:      r.User.ID,
		Name:    r.User.Name,
		JID:     r.User.JID,
		Details: r.Details,
	}
}
