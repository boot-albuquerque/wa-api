// Package admin holds the PUBLIC wire types for the /admin family of routes,
// plus the hand-written presenters that build them from domain values.
//
// It is imported as `dtoadmin`. The prefix is not decoration: the
// architectural gate in handlers/respondjson_ledger_test.go classifies a
// RespondJSON call by the FORM of the expression it serializes, and `dto…` is
// what tells it the value went through a presenter.
//
// Nothing here may be reused by the domain or by a use case: these types
// exist to be serialized, and every field name in them is a promise to a
// client. See docs/HTTP-DTO-CONVENTIONS.md.
package admin

// UserProxyConfigResponse is one user's outbound-proxy configuration, as
// served.
//
// `proxy_url` and `webhook_use_proxy` were `proxyUrl` and `webhookUseProxy`:
// the listing built a map[string]any inside the use case, so these key names
// were chosen by the application layer and declared by no type at all.
type UserProxyConfigResponse struct {
	Enabled         bool   `json:"enabled"`
	ProxyURL        string `json:"proxy_url"`
	WebhookUseProxy bool   `json:"webhook_use_proxy"`
}

// UserS3ConfigResponse is one user's media-storage configuration, as served.
//
// There is no `access_key`, and its absence is deliberate. The listing used
// to serve the literal "***" for it — the same three characters whether a key
// was configured or not, and the repository did not read the column at all,
// so the value carried no information. It also became actively dangerous
// once this migration aligned request and response names: echoing the
// response back into a PUT would have written "***" over the real
// credential.
//
// `access_key_configured` (F308) reports WHETHER a key is present, as a
// derived boolean from `COALESCE(s3_access_key,”) <> ”`
// (pkg/infra/db/user_repository.go, userS3Config) — the key itself never
// reaches this struct.
type UserS3ConfigResponse struct {
	Enabled             bool   `json:"enabled"`
	Endpoint            string `json:"endpoint"`
	Region              string `json:"region"`
	Bucket              string `json:"bucket"`
	PathStyle           bool   `json:"path_style"`
	PublicURL           string `json:"public_url"`
	MediaDelivery       string `json:"media_delivery"`
	RetentionDays       int    `json:"retention_days"`
	AccessKeyConfigured bool   `json:"access_key_configured"`
}

// UserResponse is one provisioned API user, as served by the /admin/users
// routes.
//
// No `omitempty` anywhere, and that is what changes most here: `jid`,
// `qrcode`, `logged_in`, `expiration`, `events` and `hmac_configured` used to
// VANISH from the body whenever they were empty, false or zero, so a client
// could not tell "this user has no JID" from "this build stopped sending the
// key". Emitting always is cheaper than documenting the absence.
//
// `logged_in` was `loggedIn`, which the canonical-naming rule rejects
// outright.
//
// Token is empty on the listing and filled on creation, and the difference is
// intentional: GET /admin/users used to return every user's token in
// cleartext (sec/F20), turning one read into the leak of every credential in
// the installation. The creation response is the only place the caller can
// still learn the token it just set.
type UserResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Token   string `json:"token"`
	Webhook string `json:"webhook"`
	JID     string `json:"jid"`
	QRCode  string `json:"qrcode"`

	Connected bool `json:"connected"`
	LoggedIn  bool `json:"logged_in"`

	Expiration int64  `json:"expiration"`
	Events     string `json:"events"`

	HmacConfigured bool `json:"hmac_configured"`

	// Engine is the transport this session was created with
	// (domain.EngineNoise or domain.EngineWaHeadless).
	Engine string `json:"engine"`

	ProxyConfig UserProxyConfigResponse `json:"proxy_config"`
	S3Config    UserS3ConfigResponse    `json:"s3_config"`
}

// StatusResponse is the body of the routes whose only answer is "it worked":
// PUT and DELETE on /admin/users/{id}.
//
// It stays a declared type rather than the map[string]string the handlers
// built inline, because a map has no field the compiler can check and no
// place to write down what the values may be.
type StatusResponse struct {
	Status string `json:"status"`
}

// DeleteUserCompleteResponse is the body of DELETE /admin/users/{id}/full.
//
// `details` is new on the wire: the use case always computed it and the
// handler always dropped it, which left the caller unable to tell the full
// removal apart from the plain one by looking at the answer.
type DeleteUserCompleteResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	JID     string `json:"jid"`
	Details string `json:"details"`
}
