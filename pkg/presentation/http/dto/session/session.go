// Package session holds the PUBLIC wire types for the session family of
// routes (/session/*, /status/set/*), plus the hand-written presenters that
// build them from domain values.
//
// Nothing here may be reused by the domain or by a use case: these types exist
// to be serialized, and every field name in them is a promise to a client.
// See docs/HTTP-DTO-CONVENTIONS.md.
package session

// ConnectResponse is the body of `data` for GET /session/connect.
//
// The route answers before the WhatsApp client is up — StartSession is
// fire-and-forget — so "connecting" is a state, not a result. It used to be an
// inline map[string]interface{} in the handler, which is exactly the shape no
// schema ever described.
type ConnectResponse struct {
	Status string `json:"status"`
}

// ConnectingStatus is the only value ConnectResponse.Status takes today.
//
// A constant and not a literal because the same string is the handler's
// answer and the contract test's assertion (ADR-0004).
const ConnectingStatus = "connecting"

// DisconnectResponse is the body of `data` for GET /session/disconnect.
type DisconnectResponse struct {
	Details string `json:"details"`
}

// GetQRResponse is the body of `data` for GET /session/pair/qr.
//
// The key was `QRCode` — the Go field name, PascalCase on the wire. It is
// `qr_code` now, and the old spelling is gone (hard cutover, no dual key).
//
// CodeAgeSeconds (F375/F377): how long, in seconds, THIS EXACT code has
// been the one this route returns — 0 the moment it changes (or the moment
// qr_code first becomes non-empty). It is NOT an expiry countdown: neither
// engine can honestly promise one (HOUSEKEEP F374 measured the SPA's own
// rotation gap varying 10-60s, server-driven). What a client CAN do with
// this: distinguish "the same code, camping normally" from "something is
// stuck" without the API inventing a number it cannot back up.
type GetQRResponse struct {
	QRCode         string `json:"qr_code"`
	CodeAgeSeconds int    `json:"code_age_seconds"`
}

// LogoutResponse is the body of `data` for POST /session/logout.
type LogoutResponse struct {
	Details string `json:"details"`
}

// PairPhoneResponse is the body of `data` for POST /session/pairphone.
//
// The key was `LinkingCode`.
type PairPhoneResponse struct {
	LinkingCode string `json:"linking_code"`
}

// ProxyConfigResponse is the proxy summary embedded in the session status.
//
// It replaces a map[string]interface{} built in the use case whose keys were
// `enabled` and `proxyUrl` — the second one camelCase, and invisible to any
// struct-tag audit because no tag ever produced it.
type ProxyConfigResponse struct {
	Enabled  bool   `json:"enabled"`
	ProxyURL string `json:"proxy_url"`
}

// S3ConfigSummaryResponse is the S3 summary embedded in the session status.
//
// Deliberately NOT the same type as storage.S3ConfigViewResponse: this one
// carries no access key at all, and merging them would be one refactor away
// from serving a credential from a status poll.
type S3ConfigSummaryResponse struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	PathStyle     bool   `json:"path_style"`
	PublicURL     string `json:"public_url"`
	MediaDelivery string `json:"media_delivery"`
	RetentionDays int    `json:"retention_days"`
}

// GetStatusResponse is the body of `data` for GET /session/status.
//
// `loggedIn` became `logged_in` and the nested proxy summary lost its
// `proxyUrl`. No omitempty: a client polling this route has to be able to tell
// "not connected" from "the key is gone in this build".
type GetStatusResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"logged_in"`
	JID       string `json:"jid"`
	Webhook   string `json:"webhook"`
	Events    string `json:"events"`
	ProxyURL  string `json:"proxy_url"`
	QRCode    string `json:"qr_code"`
	History   string `json:"history"`

	ProxyConfig    ProxyConfigResponse     `json:"proxy_config"`
	S3Config       S3ConfigSummaryResponse `json:"s3_config"`
	HMACConfigured bool                    `json:"hmac_configured"`
}

// SetStatusMessageResponse is the body of `data` for POST /session/statusmessage.
type SetStatusMessageResponse struct {
	Details string `json:"details"`
}

// RequestHistorySyncResponse is the body of `data` for POST /session/historysync.
type RequestHistorySyncResponse struct {
	Details            string `json:"details"`
	Timestamp          int64  `json:"timestamp"`
	Count              int    `json:"count"`
	ChatJID            string `json:"chat_jid"`
	OldestMsgID        string `json:"oldest_msg_id"`
	OldestMsgFromMe    bool   `json:"oldest_msg_from_me"`
	OldestMsgTimestamp int64  `json:"oldest_msg_timestamp"`
}

// SyncContactRosterResponse is the body of `data` for POST /user/contacts/sync.
type SyncContactRosterResponse struct {
	Details string `json:"details"`
	Mode    string `json:"mode"`
}

// PublishStatusResponse is the body of `data` for the three POST
// /status/set/{image,video,audio} routes.
//
// One type for the three because the three results were already field-for-field
// identical in the domain; three copies would only be three places to forget.
type PublishStatusResponse struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}
