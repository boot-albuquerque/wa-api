package domain

// As structs deste ficheiro JÁ NÃO SÃO o formato de fio. Elas são
// Go-idiomáticas por dentro — PascalCase, sem etiquetas `json` — e quem serve
// HTTP passa por pkg/presentation/http/dto/webhook.
// Ver docs/HTTP-DTO-CONVENTIONS.md.

// WebhookConfigRequest represents a webhook configuration request.
//
// Clients historically used different field names: POST sent "webhookurl",
// PUT sent "webhook". Both fields are always decoded; ResolveURL picks
// whichever is non-empty (preferring "webhookurl" when both are present,
// matching the POST-first convention).
type WebhookConfigRequest struct {
	WebhookURL      string
	WebhookURLField string
	Events          []string
	Active          bool
}

// ResolveURL returns the webhook URL from whichever field the client
// populated. When both are present, WebhookURLField ("webhookurl") wins.
func (r WebhookConfigRequest) ResolveURL() string {
	if r.WebhookURLField != "" {
		return r.WebhookURLField
	}
	return r.WebhookURL
}

// WebhookHistoryRequest representa a requisição para configuração de histórico.
type WebhookHistoryRequest struct {
	History int
}

// WebhookHistoryResult representa o resultado de operação de histórico.
//
// History has NO `omitempty`, and that is the contract, not a preference
// (HOUSEKEEP F165). Zero means "history disabled" — a legitimate, and probably
// the most common, answer of both the write and the read. Omitting it made
// `POST /session/history {"history":0}` reply without the field at all, which
// is not what the historical handler did: it marshalled a
// `map[string]interface{}` carrying both keys unconditionally
// (`41bc8e2^:handlers.go:6072-6075`). Dropping the tag RESTORES that shape and
// is what lets `GET /webhook/history` report a disabled limit at all.
type WebhookHistoryResult struct {
	Details string
	History int
}
