// Package capability holds the wire DTOs for GET /session/capabilities and
// GET /admin/capabilities.
package capability

// SessionCapabilitiesResponse is the body of `data` for
// GET /session/capabilities: what the AUTHENTICATED session's engine can do,
// one status per capability — no evidence/reason, per item 92 of the
// architectural prompt ("business knows capability, admin knows engine").
type SessionCapabilitiesResponse struct {
	AccountType  string            `json:"account_type"`
	Capabilities map[string]string `json:"capabilities"`
}

// PresentSessionCapabilities maps the resolved account type and per-capability
// status map onto the wire response.
func PresentSessionCapabilities(accountType string, caps map[string]string) SessionCapabilitiesResponse {
	return SessionCapabilitiesResponse{AccountType: accountType, Capabilities: caps}
}

// AdminCapabilityRow is one row of the /admin/capabilities matrix: the full
// CapabilityDecision, flattened for JSON. Unlike SessionCapabilitiesResponse,
// this DOES carry reason/evidence — the admin surface is diagnostic (item 56
// of the architectural prompt), and reason/evidence are exactly what a
// diagnosis needs.
type AdminCapabilityRow struct {
	Capability            string   `json:"capability"`
	Engine                string   `json:"engine"`
	AccountType           string   `json:"account_type"`
	Supported             bool     `json:"supported"`
	Status                string   `json:"status"`
	Reason                string   `json:"reason"`
	Evidence              string   `json:"evidence"`
	RequiredPreconditions []string `json:"required_preconditions,omitempty"`
}

// AdminCapabilitiesResponse is the body of `data` for GET /admin/capabilities:
// the full capability × engine × account_type matrix, unfiltered by session.
type AdminCapabilitiesResponse struct {
	Capabilities []AdminCapabilityRow `json:"capabilities"`
	Total        int                  `json:"total"`
}

// PresentAdminCapabilities wraps the flattened matrix rows onto the wire
// response, with Total following len(rows) by construction — a caller can
// never pass the two out of sync.
func PresentAdminCapabilities(rows []AdminCapabilityRow) AdminCapabilitiesResponse {
	return AdminCapabilitiesResponse{Capabilities: rows, Total: len(rows)}
}
