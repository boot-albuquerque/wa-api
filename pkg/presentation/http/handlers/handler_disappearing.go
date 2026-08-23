package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/hlog"

	customhttp "wa-api/pkg/presentation/http"

	"wa-api/pkg/application/usecase/chat"
)

// SetDisappearingTimerHandler handles POST /chat/ephemeral.
type SetDisappearingTimerHandler struct {
	usecase *chat.SetDisappearingTimerUseCase
}

func NewSetDisappearingTimerHandler(uc *chat.SetDisappearingTimerUseCase) *SetDisappearingTimerHandler {
	return &SetDisappearingTimerHandler{uc}
}

func (h *SetDisappearingTimerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/ephemeral"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Chat     string  `json:"chat"`
		Duration *string `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(errDecodePayload).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if req.Chat == "" {
		err := &simpleErr{"missing chat"}
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, err)
		return
	}
	if req.Duration == nil {
		err := &simpleErr{"missing duration"}
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, err)
		return
	}

	if err := h.usecase.Execute(r.Context(), id, req.Chat, *req.Duration); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Disappearing timer set"}, nil)
}

// SetDefaultDisappearingTimerHandler handles POST /chat/ephemeral/default.
type SetDefaultDisappearingTimerHandler struct {
	usecase *chat.SetDefaultDisappearingTimerUseCase
}

func NewSetDefaultDisappearingTimerHandler(uc *chat.SetDefaultDisappearingTimerUseCase) *SetDefaultDisappearingTimerHandler {
	return &SetDefaultDisappearingTimerHandler{uc}
}

func (h *SetDefaultDisappearingTimerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/ephemeral/default"

	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Duration *string `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(errDecodePayload).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if req.Duration == nil {
		err := &simpleErr{"missing duration"}
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, err)
		return
	}

	if err := h.usecase.Execute(r.Context(), id, *req.Duration); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Default disappearing timer set"}, nil)
}
