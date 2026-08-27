// Package webhook holds the PUBLIC wire types for the webhook-configuration
// routes (/webhook and /webhook/history), plus the hand-written presenters
// that build them.
//
// See docs/HTTP-DTO-CONVENTIONS.md.
package webhook

import "wa-api/pkg/domain"

// GetWebhookResponse is the body of `data` for GET /webhook.
//
// `subscribe` is the historical name of the event list on this route — it is
// NOT `events`, which is what PUT answers. The two names are kept apart on
// purpose: unifying them here would silently change what a client that already
// reads `subscribe` sees, and the path canonicalization pass is where that
// decision belongs, not this one.
type GetWebhookResponse struct {
	Webhook   string   `json:"webhook"`
	Subscribe []string `json:"subscribe"`
}

// SetWebhookResponse is the body of `data` for POST /webhook.
type SetWebhookResponse struct {
	Webhook string `json:"webhook"`
}

// UpdateWebhookResponse is the body of `data` for PUT /webhook.
type UpdateWebhookResponse struct {
	Webhook string   `json:"webhook"`
	Events  []string `json:"events"`
	Active  bool     `json:"active"`
}

// DeleteWebhookResponse is the body of `data` for DELETE /webhook.
//
// The key was `Details` — PascalCase on the wire, from a map literal in the
// handler that no struct tag ever described.
type DeleteWebhookResponse struct {
	Details string `json:"details"`
}

// HistoryResponse is the body of `data` for POST and GET /webhook/history
// (and for the /session/history alias).
//
// `history` has NO omitempty, and that is the contract, not a preference
// (HOUSEKEEP F165): zero means "recording disabled", which is a legitimate and
// probably the most common answer of both the write and the read. The key was
// `History`, and `Details` was `Details`.
type HistoryResponse struct {
	Details string `json:"details"`
	History int    `json:"history"`
}

// PresentGetWebhook maps the webhook read.
//
// The event list is allocated even when empty: `[]` and `null` are different
// values to every client, and only one of them can be ranged over without a
// check.
func PresentGetWebhook(webhookURL string, subscribe []string) GetWebhookResponse {
	return GetWebhookResponse{Webhook: webhookURL, Subscribe: nonNilStrings(subscribe)}
}

// nonNilStrings devolve uma cópia que é `[]` e nunca `null` no fio.
//
// Uma função, e não a alocação repetida nos dois apresentadores, por duas
// razões: a regra é a mesma nos dois, e o corpo de UMA expressão mantém cada
// apresentador dentro da regra X1 do cmd/logcov — um apresentador com três
// statements entra no denominador do gate de log e baixa a cobertura sem que
// haja nada que registar nele.
func nonNilStrings(in []string) []string {
	return append(make([]string, 0, len(in)), in...)
}

// PresentSetWebhook maps the webhook write.
func PresentSetWebhook(webhookURL string) SetWebhookResponse {
	return SetWebhookResponse{Webhook: webhookURL}
}

// PresentUpdateWebhook maps the webhook update.
//
// validEvents arrives nil from the handler whenever the client sent no event
// the server recognizes — the append-to-nil idiom — and nil would serialize as
// `null`. It becomes `[]` here.
func PresentUpdateWebhook(webhookURL string, validEvents []string, active bool) UpdateWebhookResponse {
	return UpdateWebhookResponse{Webhook: webhookURL, Events: nonNilStrings(validEvents), Active: active}
}

// PresentDeleteWebhook maps the webhook deletion.
func PresentDeleteWebhook(details string) DeleteWebhookResponse {
	return DeleteWebhookResponse{Details: details}
}

// PresentHistory maps the history-limit read or write.
func PresentHistory(r *domain.WebhookHistoryResult) HistoryResponse {
	if r == nil {
		return HistoryResponse{}
	}
	return HistoryResponse{Details: r.Details, History: r.History}
}
