package handlers

import (
	"net/http"

	customhttp "wa-api/pkg/presentation/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"wa-api/pkg/application/usecase/message"

	"github.com/rs/zerolog/hlog"
)

// SendCarouselHandler is the HTTP handler for POST /chat/send/carousel.
//
// Only HSCROLL_CARDS is exposed. ALBUM_IMAGE exists in the domain and in
// the adapter (it is protocol truth), but does NOT render on current WhatsApp
// clients (HOUSEKEEP F211). The request therefore carries no card-type field:
// the use case hardcodes HSCROLL_CARDS, and this handler does not offer a way
// to override it. If ALBUM_IMAGE starts rendering in the future, add the
// field here — do not silently enable it by removing this guard.
//
// Card Title is decorative: iOS ignores it entirely; only Android renders it.
// Put required information in Body, not Title. See HOUSEKEEP F217 for the
// measured evidence across six matrix directions and two business accounts.
type SendCarouselHandler struct {
	usecase *message.SendCarouselUseCase
}

// NewSendCarouselHandler creates the handler with the injected use case.
func NewSendCarouselHandler(uc *message.SendCarouselUseCase) *SendCarouselHandler {
	return &SendCarouselHandler{usecase: uc}
}

// ServeHTTP implements http.Handler for POST /chat/send/carousel.
func (h *SendCarouselHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const route = "/chat/send/carousel"

	info, ok := r.Context().Value(appport.UserInfoKey).(userInfo)
	if !ok || info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).
			Str("route", route).
			Msg("send carousel rejected at the auth boundary")
		customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
		return
	}

	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).
			Str("route", route).
			Msg("send carousel rejected without session id")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errMissingSessionID)
		return
	}

	var req domain.SendCarouselRequest
	if err := domain.DecodeRequest(r.Body, &req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).
			Str("route", route).
			Str("user_id", txtID).
			Msg("send carousel payload could not be decoded")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, errDecodePayload)
		return
	}

	result, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).
			Str("route", route).
			Str("user_id", txtID).
			Msg("send carousel use case failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	customhttp.RespondJSON(w, http.StatusOK, result, nil)
}
