package handlers

import (
	"encoding/json"
	"net/http"

	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/message"
)

type DownloadImageHandler struct{ uc *message.DownloadImageUseCase }

func NewDownloadImageHandler(uc *message.DownloadImageUseCase) *DownloadImageHandler {
	return &DownloadImageHandler{uc: uc}
}
func (h *DownloadImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("download failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadVideoHandler struct{ uc *message.DownloadVideoUseCase }

func NewDownloadVideoHandler(uc *message.DownloadVideoUseCase) *DownloadVideoHandler {
	return &DownloadVideoHandler{uc: uc}
}
func (h *DownloadVideoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("download failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadAudioHandler struct{ uc *message.DownloadAudioUseCase }

func NewDownloadAudioHandler(uc *message.DownloadAudioUseCase) *DownloadAudioHandler {
	return &DownloadAudioHandler{uc: uc}
}
func (h *DownloadAudioHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("download failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadDocumentHandler struct {
	uc *message.DownloadDocumentUseCase
}

func NewDownloadDocumentHandler(uc *message.DownloadDocumentUseCase) *DownloadDocumentHandler {
	return &DownloadDocumentHandler{uc: uc}
}
func (h *DownloadDocumentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("download failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

type DownloadStickerHandler struct {
	uc *message.DownloadStickerUseCase
}

func NewDownloadStickerHandler(uc *message.DownloadStickerUseCase) *DownloadStickerHandler {
	return &DownloadStickerHandler{uc: uc}
}
func (h *DownloadStickerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("download failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// DownloadHandlers agrupa os handlers de download de midia (/chat/download*).
type DownloadHandlers struct {
	Image    *DownloadImageHandler
	Video    *DownloadVideoHandler
	Audio    *DownloadAudioHandler
	Document *DownloadDocumentHandler
	Sticker  *DownloadStickerHandler
}
