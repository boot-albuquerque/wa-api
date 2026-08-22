package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wa-api/pkg/presentation/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"wa-api/pkg/application/usecase/message"

	"github.com/rs/zerolog/hlog"
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
	const route = "/chat/send/contact"

	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
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
	const route = "/chat/send/location"

	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
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
	const route = "/chat/send/poll"

	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendPollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}

// DeleteMessageHandler é o handler HTTP para POST /chat/delete/message.
