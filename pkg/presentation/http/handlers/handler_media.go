package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wa-api/pkg/presentation/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/message"
)

// SendMessageHandler é o handler HTTP para POST /chat/send/text.
type SendImageHandler struct {
	usecase *message.SendImageUseCase
}

// NewSendImageHandler cria o handler com o usecase injetado.
func NewSendImageHandler(uc *message.SendImageUseCase) *SendImageHandler {
	return &SendImageHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/image.
func (h *SendImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("media send rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("media send rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Msg("media send payload rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("media send failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}

// SendDocumentHandler é o handler HTTP para POST /chat/send/document.
type SendDocumentHandler struct {
	usecase *message.SendDocumentUseCase
}

// NewSendDocumentHandler cria o handler com o usecase injetado.
func NewSendDocumentHandler(uc *message.SendDocumentUseCase) *SendDocumentHandler {
	return &SendDocumentHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/document.
func (h *SendDocumentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("media send rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("media send rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Msg("media send payload rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("media send failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}

// SendAudioHandler é o handler HTTP para POST /chat/send/audio.
type SendAudioHandler struct {
	usecase *message.SendAudioUseCase
}

// NewSendAudioHandler cria o handler com o usecase injetado.
func NewSendAudioHandler(uc *message.SendAudioUseCase) *SendAudioHandler {
	return &SendAudioHandler{usecase: uc}
}

// ServeHTTP implementa http.Handler para POST /chat/send/audio.
func (h *SendAudioHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("media send rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("media send rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendAudioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Msg("media send payload rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("media send failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}

// SendStickerHandler é o handler HTTP para POST /chat/send/sticker.
