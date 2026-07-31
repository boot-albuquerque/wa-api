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
type SendStickerHandler struct {
	usecase *message.SendStickerUseCase
}

// NewSendStickerHandler cria o handler com o usecase injetado.
func NewSendStickerHandler(uc *message.SendStickerUseCase) *SendStickerHandler {
	return &SendStickerHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/sticker.
func (h *SendStickerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	usecase *message.SendVideoUseCase
}

// NewSendVideoHandler cria o handler com o usecase injetado.
func NewSendVideoHandler(uc *message.SendVideoUseCase) *SendVideoHandler {
	return &SendVideoHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/video.
func (h *SendVideoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
