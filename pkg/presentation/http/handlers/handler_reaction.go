package handlers

import (
	"encoding/json"
	"net/http"

	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"

	"wa-api/pkg/application/usecase/message"
)

type ReactHandler struct{ uc *message.ReactUseCase }

func NewReactHandler(uc *message.ReactUseCase) *ReactHandler {
	return &ReactHandler{uc: uc}
}
func (h *ReactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.ReactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ReactionHandlers agrupa os handlers de reacao a mensagem (/chat/react).
type ReactionHandlers struct {
	React *ReactHandler
}
