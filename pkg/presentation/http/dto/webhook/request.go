package webhook

import "wa-api/pkg/domain"

// ConfigRequest is the body of POST and PUT /webhook.
//
// TWO url fields, and they stay: POST clients historically send `webhookurl`
// and PUT clients send `webhook`. Both spellings already satisfy the canonical
// naming rule (lowercase, no separator), so this is not a naming defect and
// dropping one here would be a BEHAVIOUR change smuggled into a naming
// migration. ResolveURL picks whichever is non-empty, `webhookurl` winning
// when both are.
type ConfigRequest struct {
	WebhookURL      string   `json:"webhook"`
	WebhookURLField string   `json:"webhookurl"`
	Events          []string `json:"events"`
	Active          bool     `json:"active"`
}

// ResolveURL returns the webhook URL from whichever field the client
// populated. When both are present, WebhookURLField ("webhookurl") wins.
func (r ConfigRequest) ResolveURL() string {
	if r.WebhookURLField != "" {
		return r.WebhookURLField
	}
	return r.WebhookURL
}

// Validate: the handlers accept any URL, including the empty one, which is how
// PUT with active=false clears the configuration.
func (r ConfigRequest) Validate() error { return nil }

// ToDomain produces the domain-side request.
func (r ConfigRequest) ToDomain() domain.WebhookConfigRequest {
	return domain.WebhookConfigRequest{
		WebhookURL:      r.WebhookURL,
		WebhookURLField: r.WebhookURLField,
		Events:          r.Events,
		Active:          r.Active,
	}
}

// HistoryRequest is the body of POST /webhook/history (and /session/history).
type HistoryRequest struct {
	History int `json:"history"`
}

// Validate: SetHistoryUseCase owns the bounds of the limit.
func (r HistoryRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r HistoryRequest) ToDomain() domain.WebhookHistoryRequest {
	return domain.WebhookHistoryRequest{History: r.History}
}
