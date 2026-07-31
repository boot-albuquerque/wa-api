package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wa-api/internal/presentation/http"
	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"

	"wa-api/internal/application/usecase/user"
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
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	result, err := h.uc.Execute(r.Context(), txtID, domain.GetBlocklistRequest{})
	if err != nil {
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	// Serialize as JSON string to match the legacy s.Respond format
	responseJSON, err := json.Marshal(result)
	if err != nil {
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseJSON)
	_, _ = w.Write([]byte("\n"))
}
