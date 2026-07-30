package handlers

import (
	"encoding/json"
	"net/http"

	customhttp "wuzapi/internal/interfaces/http"

	"wuzapi/internal/application/port"
	"wuzapi/internal/application/usecase"
	"wuzapi/internal/domain"
)

func sessionUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	info, _ := r.Context().Value(port.UserInfoKey).(userInfo)
	if info == nil { customhttp.RespondJSON(w, 401, nil, errUnauthorized); return "", false }
	id := info.Get("Id")
	if id == "" { customhttp.RespondJSON(w, 400, nil, errMissingSessionID); return "", false }
	return id, true
}

// ConnectHandler handles POST /session/connect/{id}
type ConnectHandler struct{ usecase *usecase.ConnectUseCase }
func NewConnectHandler(uc *usecase.ConnectUseCase) *ConnectHandler { return &ConnectHandler{uc} }
func (h *ConnectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.usecase.Execute(r.Context(), id, domain.ConnectRequest{})
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// DisconnectHandler handles POST /session/disconnect/{id}
type DisconnectHandler struct{ usecase *usecase.DisconnectUseCase }
func NewDisconnectHandler(uc *usecase.DisconnectUseCase) *DisconnectHandler { return &DisconnectHandler{uc} }
func (h *DisconnectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.usecase.Execute(r.Context(), id, domain.DisconnectRequest{})
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetQRHandler handles GET /session/qr/{id}
type GetQRHandler struct{ usecase *usecase.GetQRUseCase }
func NewGetQRHandler(uc *usecase.GetQRUseCase) *GetQRHandler { return &GetQRHandler{uc} }
func (h *GetQRHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.usecase.Execute(r.Context(), id)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// LogoutHandler handles POST /session/logout/{id}
type LogoutHandler struct{ usecase *usecase.LogoutUseCase }
func NewLogoutHandler(uc *usecase.LogoutUseCase) *LogoutHandler { return &LogoutHandler{uc} }
func (h *LogoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.usecase.Execute(r.Context(), id, domain.LogoutRequest{})
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// PairPhoneHandler handles POST /session/pairphone/{id}
type PairPhoneHandler struct{ usecase *usecase.PairPhoneUseCase }
func NewPairPhoneHandler(uc *usecase.PairPhoneUseCase) *PairPhoneHandler { return &PairPhoneHandler{uc} }
func (h *PairPhoneHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.PairPhoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// GetStatusHandler handles GET /session/status/{id}
type GetStatusHandler struct{ usecase *usecase.GetStatusUseCase }
func NewGetStatusHandler(uc *usecase.GetStatusUseCase) *GetStatusHandler { return &GetStatusHandler{uc} }
func (h *GetStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.usecase.Execute(r.Context(), id)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// SetStatusMessageHandler handles POST /session/statusmessage/{id}
type SetStatusMessageHandler struct{ usecase *usecase.SetStatusMessageUseCase }
func NewSetStatusMessageHandler(uc *usecase.SetStatusMessageUseCase) *SetStatusMessageHandler { return &SetStatusMessageHandler{uc} }
func (h *SetStatusMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	var req domain.SetStatusMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { customhttp.RespondJSON(w, 400, nil, errDecodePayload); return }
	rsp, err := h.usecase.Execute(r.Context(), id, req)
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}

// RequestHistorySyncHandler handles POST /session/historysync/{id}
type RequestHistorySyncHandler struct{ usecase *usecase.RequestHistorySyncUseCase }
func NewRequestHistorySyncHandler(uc *usecase.RequestHistorySyncUseCase) *RequestHistorySyncHandler { return &RequestHistorySyncHandler{uc} }
func (h *RequestHistorySyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r); if !ok { return }
	rsp, err := h.usecase.Execute(r.Context(), id, domain.RequestHistorySyncRequest{})
	if err != nil { customhttp.RespondJSON(w, 500, nil, err); return }
	customhttp.RespondJSON(w, 200, rsp, nil)
}
