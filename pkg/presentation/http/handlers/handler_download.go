package handlers

import (
	"encoding/json"
	"net/http"

	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/wa-noise/errmap"
	customhttp "wa-api/pkg/presentation/http"
	dtomessage "wa-api/pkg/presentation/http/dto/message"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/hlog"
)

type DownloadMediaHandler struct{ uc *message.DownloadMediaUseCase }

func NewDownloadMediaHandler(uc *message.DownloadMediaUseCase) *DownloadMediaHandler {
	return &DownloadMediaHandler{uc: uc}
}

// ServeHTTP decodifica o corpo e, se ele não trouxer Kind, preenche-o com o
// segmento {kind} do caminho — na mesma convenção de
// pkg/presentation/http/canonico.go: o caminho é uma forma nova de dizer a
// mesma coisa, não uma autoridade sobre quem já a dizia, então só preenche o
// campo se o corpo não o tiver feito.
func (h *DownloadMediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Msg("download payload rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if req.Kind == "" {
		if kind, ok := mux.Vars(r)["kind"]; ok {
			req.Kind = domain.MediaKind(kind)
		}
	}
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		err = errmap.ClassifyDownload(err)
		hlog.FromRequest(r).Error().Err(err).Msg("download failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentDownload(rsp), nil)
}

// DownloadHandlers agrupa os handlers de download de mídia. Media é a única
// rota (/chats/download/{kind}) — as cinco formas anteriores por-kind foram
// removidas (HOUSEKEEP.md F297).
type DownloadHandlers struct {
	Media *DownloadMediaHandler
}
