package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wa-api/internal/presentation/http"

	appport "wa-api/internal/contracts"
	"wa-api/internal/application/usecase"
	"wa-api/internal/shared/domain"
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
type ConfigureS3Handler struct{ usecase *usecase.ConfigureS3UseCase }
func NewConfigureS3Handler(uc *usecase.ConfigureS3UseCase) *ConfigureS3Handler { return &ConfigureS3Handler{uc} }
func (h *ConfigureS3Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	var req domain.S3ConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetS3ConfigHandler handles GET /storage/s3/config
type GetS3ConfigHandler struct{ usecase *usecase.GetS3ConfigUseCase }
func NewGetS3ConfigHandler(uc *usecase.GetS3ConfigUseCase) *GetS3ConfigHandler { return &GetS3ConfigHandler{uc} }
func (h *GetS3ConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// TestS3ConnectionHandler handles POST /storage/s3/test
type TestS3ConnectionHandler struct{ usecase *usecase.TestS3ConnectionUseCase }
func NewTestS3ConnectionHandler(uc *usecase.TestS3ConnectionUseCase) *TestS3ConnectionHandler { return &TestS3ConnectionHandler{uc} }
func (h *TestS3ConnectionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	var req domain.S3TestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// DeleteS3ConfigHandler handles DELETE /storage/s3/config
type DeleteS3ConfigHandler struct{ usecase *usecase.DeleteS3ConfigUseCase }
func NewDeleteS3ConfigHandler(uc *usecase.DeleteS3ConfigUseCase) *DeleteS3ConfigHandler { return &DeleteS3ConfigHandler{uc} }
func (h *DeleteS3ConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// ConfigureHmacHandler handles POST /storage/hmac/configure
type ConfigureHmacHandler struct{ usecase *usecase.ConfigureHmacUseCase }
func NewConfigureHmacHandler(uc *usecase.ConfigureHmacUseCase) *ConfigureHmacHandler { return &ConfigureHmacHandler{uc} }
func (h *ConfigureHmacHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	var req domain.HmacConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetHmacConfigHandler handles GET /storage/hmac/config
type GetHmacConfigHandler struct{ usecase *usecase.GetHmacConfigUseCase }
func NewGetHmacConfigHandler(uc *usecase.GetHmacConfigUseCase) *GetHmacConfigHandler { return &GetHmacConfigHandler{uc} }
func (h *GetHmacConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// DeleteHmacConfigHandler handles DELETE /storage/hmac/config
type DeleteHmacConfigHandler struct{ usecase *usecase.DeleteHmacConfigUseCase }
func NewDeleteHmacConfigHandler(uc *usecase.DeleteHmacConfigUseCase) *DeleteHmacConfigHandler { return &DeleteHmacConfigHandler{uc} }
func (h *DeleteHmacConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SetProxyHandler handles POST /storage/proxy
type SetProxyHandler struct{ usecase *usecase.SetProxyUseCase }
func NewSetProxyHandler(uc *usecase.SetProxyUseCase) *SetProxyHandler { return &SetProxyHandler{uc} }
func (h *SetProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	var req domain.ProxyConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SetHistoryHandler handles POST /storage/history
type SetHistoryHandler struct{ usecase *usecase.SetHistoryUseCase }
func NewSetHistoryHandler(uc *usecase.SetHistoryUseCase) *SetHistoryHandler { return &SetHistoryHandler{uc} }
func (h *SetHistoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	var req domain.WebhookHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetHistoryHandler handles GET /storage/history
type GetHistoryHandler struct{ usecase *usecase.GetHistoryUseCase }
func NewGetHistoryHandler(uc *usecase.GetHistoryUseCase) *GetHistoryHandler { return &GetHistoryHandler{uc} }
func (h *GetHistoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info, _ := r.Context().Value(appport.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return }
	txtID := info.Get("Id")
	if txtID == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return }
	rsp, err := h.usecase.Execute(r.Context(), txtID)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}
