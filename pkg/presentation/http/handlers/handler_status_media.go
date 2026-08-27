package handlers

import (
	"encoding/json"
	"net/http"

	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"
	dtomessage "wa-api/pkg/presentation/http/dto/message"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/status"
)

// PublishStatusImageHandler handles POST /status/set/image.
type PublishStatusImageHandler struct {
	usecase *status.PublishStatusImageUseCase
}

func NewPublishStatusImageHandler(uc *status.PublishStatusImageUseCase) *PublishStatusImageHandler {
	return &PublishStatusImageHandler{uc}
}

func (h *PublishStatusImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.PublishStatusImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("path", r.URL.Path).Msg("status media request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "PublishStatusImage").Str("user_id", id).Msg("status media use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "PublishStatusImage").Str("user_id", id).Msg("status media use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentPublishStatusImage(rsp), nil)
}

// PublishStatusVideoHandler handles POST /status/set/video.
type PublishStatusVideoHandler struct {
	usecase *status.PublishStatusVideoUseCase
}

func NewPublishStatusVideoHandler(uc *status.PublishStatusVideoUseCase) *PublishStatusVideoHandler {
	return &PublishStatusVideoHandler{uc}
}

func (h *PublishStatusVideoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.PublishStatusVideoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("path", r.URL.Path).Msg("status media request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "PublishStatusVideo").Str("user_id", id).Msg("status media use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "PublishStatusVideo").Str("user_id", id).Msg("status media use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentPublishStatusVideo(rsp), nil)
}

// PublishStatusAudioHandler handles POST /status/set/audio.
type PublishStatusAudioHandler struct {
	usecase *status.PublishStatusAudioUseCase
}

func NewPublishStatusAudioHandler(uc *status.PublishStatusAudioUseCase) *PublishStatusAudioHandler {
	return &PublishStatusAudioHandler{uc}
}

func (h *PublishStatusAudioHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.PublishStatusAudioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("path", r.URL.Path).Msg("status media request rejected")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "PublishStatusAudio").Str("user_id", id).Msg("status media use case failed")
			customhttp.RespondJSON(w, 500, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "PublishStatusAudio").Str("user_id", id).Msg("status media use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, dtomessage.PresentPublishStatusAudio(rsp), nil)
}
