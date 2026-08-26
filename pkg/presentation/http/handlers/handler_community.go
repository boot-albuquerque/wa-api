package handlers

import (
	"errors"
	"net/http"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	customhttp "wa-api/pkg/presentation/http"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/group"
)

// CommunityHandlers groups community HTTP handlers.
type CommunityHandlers struct {
	GetSubGroups    *GetCommunitySubGroupsHandler
	GetParticipants *GetCommunityParticipantsHandler
	LinkGroup       *CommunityLinkGroupHandler
	UnlinkGroup     *CommunityUnlinkGroupHandler
}

// GetCommunitySubGroupsHandler lists subgroups of a community.
type GetCommunitySubGroupsHandler struct{ usecase *group.CommunityReadUseCase }

func NewGetCommunitySubGroupsHandler(uc *group.CommunityReadUseCase) *GetCommunitySubGroupsHandler {
	return &GetCommunitySubGroupsHandler{uc}
}
func (h *GetCommunitySubGroupsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.GetCommunitySubGroupsRequest
	if err := domain.DecodeRequest(r.Body, &req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode community subgroups payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.GetSubGroups(r.Context(), id, req)
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("community subgroups request rejected")
			customhttp.RespondJSON(w, 400, nil, err)
		} else {
			hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("get community subgroups failed")
			customhttp.RespondJSON(w, 500, nil, err)
		}
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetCommunityParticipantsHandler lists participants of linked groups.
type GetCommunityParticipantsHandler struct{ usecase *group.CommunityReadUseCase }

func NewGetCommunityParticipantsHandler(uc *group.CommunityReadUseCase) *GetCommunityParticipantsHandler {
	return &GetCommunityParticipantsHandler{uc}
}
func (h *GetCommunityParticipantsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.GetCommunityParticipantsRequest
	if err := domain.DecodeRequest(r.Body, &req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode community participants payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.GetParticipants(r.Context(), id, req)
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("community participants request rejected")
			customhttp.RespondJSON(w, 400, nil, err)
		} else {
			hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("get community participants failed")
			customhttp.RespondJSON(w, 500, nil, err)
		}
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// CommunityLinkGroupHandler links a group to a community.
type CommunityLinkGroupHandler struct{ usecase *group.CommunityWriteUseCase }

func NewCommunityLinkGroupHandler(uc *group.CommunityWriteUseCase) *CommunityLinkGroupHandler {
	return &CommunityLinkGroupHandler{uc}
}
func (h *CommunityLinkGroupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.CommunityLinkRequest
	if err := domain.DecodeRequest(r.Body, &req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode community link payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.usecase.LinkGroup(r.Context(), id, req); err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("community link rejected")
			customhttp.RespondJSON(w, 400, nil, err)
		} else {
			hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("link group to community failed")
			customhttp.RespondJSON(w, 500, nil, err)
		}
		return
	}
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Group linked to community successfully"}, nil)
}

// CommunityUnlinkGroupHandler unlinks a group from a community.
type CommunityUnlinkGroupHandler struct{ usecase *group.CommunityWriteUseCase }

func NewCommunityUnlinkGroupHandler(uc *group.CommunityWriteUseCase) *CommunityUnlinkGroupHandler {
	return &CommunityUnlinkGroupHandler{uc}
}
func (h *CommunityUnlinkGroupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}
	var req domain.CommunityUnlinkRequest
	if err := domain.DecodeRequest(r.Body, &req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("could not decode community unlink payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	if err := h.usecase.UnlinkGroup(r.Context(), id, req); err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("community unlink rejected")
			customhttp.RespondJSON(w, 400, nil, err)
		} else {
			hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("unlink group from community failed")
			customhttp.RespondJSON(w, 500, nil, err)
		}
		return
	}
	customhttp.RespondJSON(w, 200, map[string]string{"Details": "Group unlinked from community successfully"}, nil)
}
