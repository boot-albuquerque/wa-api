package handlers

import (
	"net/http"
)

// ServerMethods defines the server methods needed by webhook handlers.
// This allows webhook handlers to delegate to the server's legacy implementations.
type ServerMethods interface {
	GetWebhook() http.HandlerFunc
	SetWebhook() http.HandlerFunc
	UpdateWebhook() http.HandlerFunc
	DeleteWebhook() http.HandlerFunc
	SetHistory() http.HandlerFunc
	GetHistory() http.HandlerFunc
}

// GetWebhookHandler é o handler HTTP para GET /webhook.
type GetWebhookHandler struct {
	server ServerMethods
}

// NewGetWebhookHandler cria o handler.
func NewGetWebhookHandler(server ServerMethods) *GetWebhookHandler {
	return &GetWebhookHandler{server: server}
}

// ServeHTTP implementa http.Handler para GET /webhook.
func (h *GetWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.server.GetWebhook()(w, r)
}

// SetWebhookHandler é o handler HTTP para POST /webhook.
type SetWebhookHandler struct {
	server ServerMethods
}

// NewSetWebhookHandler cria o handler.
func NewSetWebhookHandler(server ServerMethods) *SetWebhookHandler {
	return &SetWebhookHandler{server: server}
}

// ServeHTTP implementa http.Handler para POST /webhook.
func (h *SetWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.server.SetWebhook()(w, r)
}

// UpdateWebhookHandler é o handler HTTP para PUT /webhook.
type UpdateWebhookHandler struct {
	server ServerMethods
}

// NewUpdateWebhookHandler cria o handler.
func NewUpdateWebhookHandler(server ServerMethods) *UpdateWebhookHandler {
	return &UpdateWebhookHandler{server: server}
}

// ServeHTTP implementa http.Handler para PUT /webhook.
func (h *UpdateWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.server.UpdateWebhook()(w, r)
}

// DeleteWebhookHandler é o handler HTTP para DELETE /webhook.
type DeleteWebhookHandler struct {
	server ServerMethods
}

// NewDeleteWebhookHandler cria o handler.
func NewDeleteWebhookHandler(server ServerMethods) *DeleteWebhookHandler {
	return &DeleteWebhookHandler{server: server}
}

// ServeHTTP implementa http.Handler para DELETE /webhook.
func (h *DeleteWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.server.DeleteWebhook()(w, r)
}
