package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain/apperr"
	customhttp "wa-api/pkg/presentation/http"
	dtomessage "wa-api/pkg/presentation/http/dto/message"
)

// SetDisappearingTimerHandler handles POST /chat/ephemeral.
type SetDisappearingTimerHandler struct {
	usecase *chat.SetDisappearingTimerUseCase
}

func NewSetDisappearingTimerHandler(uc *chat.SetDisappearingTimerUseCase) *SetDisappearingTimerHandler {
	return &SetDisappearingTimerHandler{uc}
}

func (h *SetDisappearingTimerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chats/ephemeral"

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
		err := apperr.New("missing_chat", apperr.CategoryValidation, "missing chat in payload", false, nil)
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, err)
		return
	}
	if req.Duration == nil {
		err := apperr.New("missing_duration", apperr.CategoryValidation, "missing duration in payload", false, nil)
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, err)
		return
	}

	if err := h.usecase.Execute(r.Context(), id, req.Chat, *req.Duration); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentAction(dtomessage.DetailsDisappearingSet), nil)
}

// SetDefaultDisappearingTimerHandler handles POST /chat/ephemeral/default.
type SetDefaultDisappearingTimerHandler struct {
	usecase *chat.SetDefaultDisappearingTimerUseCase
}

func NewSetDefaultDisappearingTimerHandler(uc *chat.SetDefaultDisappearingTimerUseCase) *SetDefaultDisappearingTimerHandler {
	return &SetDefaultDisappearingTimerHandler{uc}
}

func (h *SetDefaultDisappearingTimerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chats/ephemeral/default"

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
		err := apperr.New("missing_duration", apperr.CategoryValidation, "missing duration in payload", false, nil)
		hlog.FromRequest(r).Warn().Err(err).Str("route", route).Msg("request rejected")
		customhttp.RespondJSON(w, 400, nil, err)
		return
	}

	if err := h.usecase.Execute(r.Context(), id, *req.Duration); err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", route).Msg("request failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentAction(dtomessage.DetailsDefaultDisappearingSet), nil)
}
