package handlers

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/infra/db"
	customhttp "wa-api/pkg/presentation/http"
)

// LabelHandlers serve a leitura de etiquetas (F191).
//
// Só LEITURA: a biblioteca não sabe criar etiquetas (LIB-01), portanto uma
// rota de escrita responderia 200 sem fazer nada — que é precisamente o
// defeito da F198, e não se acrescenta de propósito.
type LabelHandlers struct {
	ListLabels    http.Handler
	ListLabelChat http.Handler
}

// NewLabelHandlers cria os handlers de etiqueta.
func NewLabelHandlers(repo *db.LabelRepository) *LabelHandlers {
	return &LabelHandlers{
		ListLabels:    &listLabelsHandler{repo: repo},
		ListLabelChat: &listLabelChatsHandler{repo: repo},
	}
}

type listLabelsHandler struct{ repo *db.LabelRepository }

// ServeHTTP responde GET /labels.
func (h *listLabelsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	rotulos, err := h.repo.ListLabels(r.Context(), id)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("failed to list labels")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}
	// Lista vazia sai como [] e não como null: um cliente que faça
	// `for (const l of resp.data)` rebenta com null e não com [].
	if rotulos == nil {
		rotulos = []db.Label{}
	}
	customhttp.RespondJSON(w, http.StatusOK, rotulos, nil)
}

type listLabelChatsHandler struct{ repo *db.LabelRepository }

// ServeHTTP responde GET /labels/{id}/chats.
//
// O id vem de mux.Vars e NÃO de r.PathValue: o router deste projeto é o
// gorilla/mux, e PathValue devolveria vazio em silêncio — foi assim que a F81
// sobreviveu.
func (h *listLabelChatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	labelID := mux.Vars(r)["id"]
	if labelID == "" {
		rejectMissingField(w, r, CodeMissingID, "id", "list label chats request rejected")
		return
	}
	conversas, err := h.repo.ListChatsForLabel(r.Context(), id, labelID)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("failed to list label chats")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}
	if conversas == nil {
		conversas = []db.LabelChat{}
	}
	customhttp.RespondJSON(w, http.StatusOK, conversas, nil)
}
