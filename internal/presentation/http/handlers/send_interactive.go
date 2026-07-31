package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wa-api/internal/presentation/http"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"

	"wa-api/internal/application/usecase/message"
)

// SendMessageHandler é o handler HTTP para POST /chat/send/text.
type SendContactHandler struct {
	usecase *message.SendContactUseCase
}

// NewSendContactHandler cria o handler com o usecase injetado.
func NewSendContactHandler(uc *message.SendContactUseCase) *SendContactHandler {
	return &SendContactHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/contact.
func (h *SendContactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req domain.SendContactRequest
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

// SendLocationHandler é o handler HTTP para POST /chat/send/location.
type SendLocationHandler struct {
	usecase *message.SendLocationUseCase
}

// NewSendLocationHandler cria o handler com o usecase injetado.
func NewSendLocationHandler(uc *message.SendLocationUseCase) *SendLocationHandler {
	return &SendLocationHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/location.
func (h *SendLocationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req domain.SendLocationRequest
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

// SendButtonsHandler é o handler HTTP para POST /chat/send/buttons.
type SendButtonsHandler struct {
	usecase *message.SendButtonsUseCase
}

// NewSendButtonsHandler cria o handler com o usecase injetado.
func NewSendButtonsHandler(uc *message.SendButtonsUseCase) *SendButtonsHandler {
	return &SendButtonsHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/buttons.
func (h *SendButtonsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req domain.SendButtonsRequest
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

// SendListHandler é o handler HTTP para POST /chat/send/list.
type SendListHandler struct {
	usecase *message.SendListUseCase
}

// NewSendListHandler cria o handler com o usecase injetado.
func NewSendListHandler(uc *message.SendListUseCase) *SendListHandler {
	return &SendListHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/list.
func (h *SendListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req domain.SendListRequest
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

// SendPollHandler é o handler HTTP para POST /chat/send/poll.
type SendPollHandler struct {
	usecase *message.SendPollUseCase
}

// NewSendPollHandler cria o handler com o usecase injetado.
func NewSendPollHandler(uc *message.SendPollUseCase) *SendPollHandler {
	return &SendPollHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/poll.
func (h *SendPollHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req domain.SendPollRequest
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

// DeleteMessageHandler é o handler HTTP para POST /chat/delete/message.
