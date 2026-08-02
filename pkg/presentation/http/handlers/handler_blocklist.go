package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/hlog"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"

	"wa-api/pkg/application/usecase/user"
)

// BlocklistHandlers groups handlers for blocklist operations.
type BlocklistHandlers struct {
	GetBlocklist *GetBlocklistHandler
}

// GetBlocklistHandler returns the current list of blocked users.
type GetBlocklistHandler struct {
	uc *user.GetBlocklistUseCase
}

// NewGetBlocklistHandler creates a new GetBlocklistHandler.
func NewGetBlocklistHandler(uc *user.GetBlocklistUseCase) *GetBlocklistHandler {
	return &GetBlocklistHandler{uc: uc}
}

// ServeHTTP implements http.Handler for GET /user/blocklist.
func (h *GetBlocklistHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).
			Str("path", r.URL.Path).
			Msg("request without user info in context")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).
			Str("path", r.URL.Path).
			Msg("request with empty session id")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	result, err := h.uc.Execute(r.Context(), txtID, domain.GetBlocklistRequest{})
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("user_id", txtID).
			Msg("get blocklist use case failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	// Serialize as JSON string to match the legacy s.Respond format
	responseJSON, err := json.Marshal(result)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("user_id", txtID).
			Msg("could not marshal blocklist response")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseJSON)
	_, _ = w.Write([]byte("\n"))
}
