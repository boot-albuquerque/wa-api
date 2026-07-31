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
type SendImageHandler struct {
	usecase *usecase.SendImageUseCase
}

// NewSendImageHandler cria o handler com o usecase injetado.
func NewSendImageHandler(uc *usecase.SendImageUseCase) *SendImageHandler {
	return &SendImageHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/image.
func (h *SendImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req domain.SendImageRequest
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

// SendDocumentHandler é o handler HTTP para POST /chat/send/document.
type SendDocumentHandler struct {
	usecase *usecase.SendDocumentUseCase
}

// NewSendDocumentHandler cria o handler com o usecase injetado.
func NewSendDocumentHandler(uc *usecase.SendDocumentUseCase) *SendDocumentHandler {
	return &SendDocumentHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/document.
func (h *SendDocumentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req domain.SendDocumentRequest
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

// SendAudioHandler é o handler HTTP para POST /chat/send/audio.
type SendAudioHandler struct {
	usecase *usecase.SendAudioUseCase
}

// NewSendAudioHandler cria o handler com o usecase injetado.
func NewSendAudioHandler(uc *usecase.SendAudioUseCase) *SendAudioHandler {
	return &SendAudioHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/audio.
func (h *SendAudioHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req domain.SendAudioRequest
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

// SendStickerHandler é o handler HTTP para POST /chat/send/sticker.
