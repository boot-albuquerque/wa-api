package handlers

import (
	"net/http"
	"strconv"

	customhttp "wa-api/pkg/presentation/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain/apperr"

	"github.com/rs/zerolog/hlog"
)

// ChatHistoryHandlers groups the handlers of the local chat history.
type ChatHistoryHandlers struct {
	GetChatHistory *GetChatHistoryHandler
}

// GetChatHistoryHandler serves GET /chat/history.
//
// It is a DIFFERENT handler from StorageHandlers.GetHistory, which serves
// GET /webhook/history. Both routes pointed at the same handler in the legacy
// custom_routes.go, and the migration kept the wiring while losing the
// implementation — so /chat/history answered a fixed
// {"Details":"History configuration retrieved"} and never touched the
// database (HOUSEKEEP F124). See TestChatHistoryAndWebhookHistoryAreDistinctHandlers.
type GetChatHistoryHandler struct {
	usecase *chat.GetChatHistoryUseCase
}

// NewGetChatHistoryHandler creates the handler with the use case injected.
func NewGetChatHistoryHandler(uc *chat.GetChatHistoryUseCase) *GetChatHistoryHandler {
	return &GetChatHistoryHandler{usecase: uc}
}

// ServeHTTP implements http.Handler for GET /chat/history.
func (h *GetChatHistoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("chat history read rejected")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("chat history read rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	// The cached History value only ever seeds the gate; a 0 here is not the
	// verdict, the use case revalidates it against the users table. A value
	// the cache cannot parse counts as 0 for the same reason — it goes to
	// revalidation instead of being trusted.
	cachedHistory, _ := strconv.Atoi(info.Get("History"))

	query := chat.ChatHistoryQuery{ChatJID: r.URL.Query().Get("chat_jid")}

	// Absent `limit` stays nil so the use case applies the default of 50; an
	// unparseable `limit` is refused here, at the wire boundary, exactly as
	// the historical handler did.
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			hlog.FromRequest(r).Warn().Err(err).Msg("chat history read rejected: invalid limit")
			// apperr, and not a bare error: RespondJSON only carries the
			// message through for a typed error, and "invalid limit" is the
			// text the historical endpoint answered.
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, apperr.New(
				"invalid_limit", apperr.CategoryValidation, "invalid limit", false, nil))
			return
		}
		query.Limit = &parsed
	}

	result, err := h.usecase.Execute(r.Context(), txtID, cachedHistory, query)
	if err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("user", txtID).Msg("chat history read failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	// The two branches have different shapes on the wire and always did:
	// `index` answers a map of user id to chats, a normal chat answers the
	// array of messages.
	if result.IsIndex {
		customhttp.RespondJSON(w, http.StatusOK, result.Index, nil)
		return
	}
	customhttp.RespondJSON(w, http.StatusOK, result.Messages, nil)
}
