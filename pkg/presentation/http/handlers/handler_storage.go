package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wa-api/pkg/presentation/http"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"wa-api/pkg/application/usecase/storage"

	"github.com/rs/zerolog/hlog"
)

// StorageHandlers agrupa os handlers de armazenamento (S3, HMAC, proxy, history)
type StorageHandlers struct {
	ConfigureS3      *ConfigureS3Handler
	GetS3Config      *GetS3ConfigHandler
	TestS3Connection *TestS3ConnectionHandler
	DeleteS3Config   *DeleteS3ConfigHandler
	ConfigureHmac    *ConfigureHmacHandler
	GetHmacConfig    *GetHmacConfigHandler
	DeleteHmacConfig *DeleteHmacConfigHandler
	SetProxy         *SetProxyHandler
	SetHistory       *SetHistoryHandler
	GetHistory       *GetHistoryHandler
}

// ConfigureS3Handler handles POST /storage/s3/configure
type ConfigureS3Handler struct{ usecase *storage.ConfigureS3UseCase }

func NewConfigureS3Handler(uc *storage.ConfigureS3UseCase) *ConfigureS3Handler {
	return &ConfigureS3Handler{uc}
}
func (h *ConfigureS3Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	var req domain.S3ConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Msg("could not decode payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetS3ConfigHandler handles GET /storage/s3/config
type GetS3ConfigHandler struct{ usecase *storage.GetS3ConfigUseCase }

func NewGetS3ConfigHandler(uc *storage.GetS3ConfigUseCase) *GetS3ConfigHandler {
	return &GetS3ConfigHandler{uc}
}
func (h *GetS3ConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// TestS3ConnectionHandler handles POST /storage/s3/test
type TestS3ConnectionHandler struct {
	usecase *storage.TestS3ConnectionUseCase
}

func NewTestS3ConnectionHandler(uc *storage.TestS3ConnectionUseCase) *TestS3ConnectionHandler {
	return &TestS3ConnectionHandler{uc}
}
func (h *TestS3ConnectionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	var req domain.S3TestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Msg("could not decode payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// DeleteS3ConfigHandler handles DELETE /storage/s3/config
type DeleteS3ConfigHandler struct {
	usecase *storage.DeleteS3ConfigUseCase
}

func NewDeleteS3ConfigHandler(uc *storage.DeleteS3ConfigUseCase) *DeleteS3ConfigHandler {
	return &DeleteS3ConfigHandler{uc}
}
func (h *DeleteS3ConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ConfigureHmacHandler handles POST /storage/hmac/configure
type ConfigureHmacHandler struct{ usecase *storage.ConfigureHmacUseCase }

func NewConfigureHmacHandler(uc *storage.ConfigureHmacUseCase) *ConfigureHmacHandler {
	return &ConfigureHmacHandler{uc}
}
func (h *ConfigureHmacHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	var req domain.HmacConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Msg("could not decode payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetHmacConfigHandler handles GET /storage/hmac/config
type GetHmacConfigHandler struct{ usecase *storage.GetHmacConfigUseCase }

func NewGetHmacConfigHandler(uc *storage.GetHmacConfigUseCase) *GetHmacConfigHandler {
	return &GetHmacConfigHandler{uc}
}
func (h *GetHmacConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// DeleteHmacConfigHandler handles DELETE /storage/hmac/config
type DeleteHmacConfigHandler struct {
	usecase *storage.DeleteHmacConfigUseCase
}

func NewDeleteHmacConfigHandler(uc *storage.DeleteHmacConfigUseCase) *DeleteHmacConfigHandler {
	return &DeleteHmacConfigHandler{uc}
}
func (h *DeleteHmacConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SetProxyHandler handles POST /storage/proxy
type SetProxyHandler struct{ usecase *storage.SetProxyUseCase }

func NewSetProxyHandler(uc *storage.SetProxyUseCase) *SetProxyHandler { return &SetProxyHandler{uc} }
func (h *SetProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	var req domain.ProxyConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Msg("could not decode payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SetHistoryHandler handles POST /storage/history
type SetHistoryHandler struct{ usecase *storage.SetHistoryUseCase }

func NewSetHistoryHandler(uc *storage.SetHistoryUseCase) *SetHistoryHandler {
	return &SetHistoryHandler{uc}
}
func (h *SetHistoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	var req domain.WebhookHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Msg("could not decode payload")
		customhttp.RespondJSON(w, 400, nil, errDecodePayload)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetHistoryHandler handles GET /storage/history
type GetHistoryHandler struct{ usecase *storage.GetHistoryUseCase }

func NewGetHistoryHandler(uc *storage.GetHistoryUseCase) *GetHistoryHandler {
	return &GetHistoryHandler{uc}
}
func (h *GetHistoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil {
		hlog.FromRequest(r).Warn().Err(errUnauthorized).Msg("unauthenticated request rejected")
		customhttp.RespondJSON(w, 401, nil, errUnauthorized)
		return
	}
	txtID := info.Get("Id")
	if txtID == "" {
		hlog.FromRequest(r).Warn().Err(errMissingSessionID).Msg("request without session id rejected")
		customhttp.RespondJSON(w, 400, nil, errMissingSessionID)
		return
	}
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("user", txtID).Msg("storage use case failed")
		customhttp.RespondJSON(w, 500, nil, err)
		return
	}
	customhttp.RespondJSON(w, 200, rsp, nil)
}
