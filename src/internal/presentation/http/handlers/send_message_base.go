package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "disparazap/internal/presentation/http"

	appport "disparazap/internal/contracts"
	"disparazap/internal/application/usecase"
	"disparazap/internal/shared/domain"
)

// SendMessageHandler é o handler HTTP para POST /chat/send/text.
type SendMessageHandler struct {
	usecase *usecase.SendMessageUseCase
}

// NewSendMessageHandler cria o handler com o usecase injetado.
func NewSendMessageHandler(uc *usecase.SendMessageUseCase) *SendMessageHandler {
	return &SendMessageHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/text.
func (h *SendMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}

// SendImageHandler é o handler HTTP para POST /chat/send/image.
type DeleteMessageHandler struct {
	usecase *usecase.DeleteMessageUseCase
}

// NewDeleteMessageHandler cria o handler com o usecase injetado.
func NewDeleteMessageHandler(uc *usecase.DeleteMessageUseCase) *DeleteMessageHandler {
	return &DeleteMessageHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/delete/message.
func (h *DeleteMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.DeleteMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}

// SendEditMessageHandler é o handler HTTP para POST /chat/send/edit.
type SendEditMessageHandler struct {
	usecase *usecase.SendEditMessageUseCase
}

// NewSendEditMessageHandler cria o handler com o usecase injetado.
func NewSendEditMessageHandler(uc *usecase.SendEditMessageUseCase) *SendEditMessageHandler {
	return &SendEditMessageHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/edit.
func (h *SendEditMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendEditMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}

// SendTemplateHandler é o handler HTTP para POST /chat/send/template.
type SendTemplateHandler struct {
	usecase *usecase.SendTemplateUseCase
}

// NewSendTemplateHandler cria o handler com o usecase injetado.
func NewSendTemplateHandler(uc *usecase.SendTemplateUseCase) *SendTemplateHandler {
	return &SendTemplateHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/template.
func (h *SendTemplateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}
