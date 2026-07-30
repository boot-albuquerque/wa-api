package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wuzapi/internal/interfaces/http"

	"wuzapi/internal/application/port"
	"wuzapi/internal/application/usecase"
	"wuzapi/internal/domain"
)

// SendMessageHandler é o handler HTTP para POST /chat/send/text.
type SendStickerHandler struct {
	usecase *usecase.SendStickerUseCase
}

// NewSendStickerHandler cria o handler com o usecase injetado.
func NewSendStickerHandler(uc *usecase.SendStickerUseCase) *SendStickerHandler {
	return &SendStickerHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/sticker.
func (h *SendStickerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(port.UserInfoKey).(userInfo)
	if !ok || info == nil {
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendStickerRequest
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

// SendVideoHandler é o handler HTTP para POST /chat/send/video.
type SendVideoHandler struct {
	usecase *usecase.SendVideoUseCase
}

// NewSendVideoHandler cria o handler com o usecase injetado.
func NewSendVideoHandler(uc *usecase.SendVideoUseCase) *SendVideoHandler {
	return &SendVideoHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/video.
func (h *SendVideoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(port.UserInfoKey).(userInfo)
	if !ok || info == nil {
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendVideoRequest
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

// SendContactHandler é o handler HTTP para POST /chat/send/contact.
