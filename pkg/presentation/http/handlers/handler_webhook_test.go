package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"

	"github.com/patrickmn/go-cache"
)

// webhookMockUserInfo implementa a interface userInfo injetada pelo middleware.
type webhookMockUserInfo struct {
	id    string
	token string
}

func (m *webhookMockUserInfo) Get(key string) string {
	switch key {
	case "Id":
		return m.id
	case "Token":
		return m.token
	default:
		return ""
	}
}

// webhookFakeDB implementa WebhookHandlerDB sem depender de um driver real.
type webhookFakeDB struct {
	execCalls int
	execErr   error
	queryErr  error
}

func (d *webhookFakeDB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	if d.queryErr != nil {
		return nil, d.queryErr
	}
	return nil, errors.New("no rows source configured")
}

type webhookFakeResult struct{}

func (webhookFakeResult) LastInsertId() (int64, error) { return 0, nil }
func (webhookFakeResult) RowsAffected() (int64, error) { return 1, nil }

func (d *webhookFakeDB) Exec(query string, args ...interface{}) (sql.Result, error) {
	d.execCalls++
	if d.execErr != nil {
		return nil, d.execErr
	}
	return webhookFakeResult{}, nil
}

func newWebhookTestContext(db *webhookFakeDB) *WebhookHandlerContext {
	return &WebhookHandlerContext{
		DB:              db,
		UserCache:       cache.New(5*time.Minute, 10*time.Minute),
		SupportedEvents: []string{"Message", "ReadReceipt"},
		FindInSlice: func(slice []string, val string) bool {
			for _, s := range slice {
				if s == val {
					return true
				}
			}
			return false
		},
		UpdateUserInfo: func(info interface{}, key, value string) interface{} { return info },
		RespondJSON: func(w http.ResponseWriter, status int, data interface{}) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			payload := map[string]interface{}{"code": status}
			if err, isErr := data.(error); isErr {
				payload["error"] = err.Error()
			} else {
				payload["data"] = data
			}
			_ = json.NewEncoder(w).Encode(payload)
		},
	}
}

// withTypedUserInfo popula o contexto com a chave TIPADA appport.UserInfoKey,
// exatamente como o middleware AuthAlice faz.
func withTypedUserInfo(r *http.Request) *http.Request {
	ctx := context.WithValue(r.Context(), appport.UserInfoKey, &webhookMockUserInfo{
		id:    "42",
		token: "tok-42",
	})
	return r.WithContext(ctx)
}

func webhookHandlers(ctx *WebhookHandlerContext) map[string]http.Handler {
	return map[string]http.Handler{
		http.MethodGet:    NewGetWebhookHandler(ctx),
		http.MethodPost:   NewSetWebhookHandler(ctx),
		http.MethodPut:    NewUpdateWebhookHandler(ctx),
		http.MethodDelete: NewDeleteWebhookHandler(ctx),
	}
}

func webhookBody(method string) string {
	switch method {
	case http.MethodPost:
		return `{"webhookurl":"https://example.com/hook","events":["Message"]}`
	case http.MethodPut:
		return `{"webhook":"https://example.com/hook","events":["Message"],"active":true}`
	default:
		return ""
	}
}

// TestWebhookHandlers_TypedUserInfo_NoPanic garante que as 4 rotas /webhook
// leem o userinfo pela chave tipada appport.UserInfoKey sem entrar em pânico.
func TestWebhookHandlers_TypedUserInfo_NoPanic(t *testing.T) {
	for method, handler := range webhookHandlers(newWebhookTestContext(&webhookFakeDB{})) {
		t.Run(method, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("%s /webhook panicked: %v", method, rec)
				}
			}()

			req := httptest.NewRequest(method, "/webhook", strings.NewReader(webhookBody(method)))
			req = withTypedUserInfo(req)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code == http.StatusUnauthorized {
				t.Fatalf("%s /webhook returned 401 with typed userinfo in context", method)
			}
			if method != http.MethodGet && w.Code != http.StatusOK {
				t.Fatalf("%s /webhook expected 200, got %d (body=%s)", method, w.Code, w.Body.String())
			}
		})
	}
}

// TestWebhookHandlers_RawStringKey_401 garante que a chave crua string
// "userinfo" (que NÃO é appport.UserInfoKey) resulta em 401 e não em pânico.
func TestWebhookHandlers_RawStringKey_401(t *testing.T) {
	for method, handler := range webhookHandlers(newWebhookTestContext(&webhookFakeDB{})) {
		t.Run(method, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("%s /webhook panicked instead of responding 401: %v", method, rec)
				}
			}()

			req := httptest.NewRequest(method, "/webhook", strings.NewReader(webhookBody(method)))
			//nolint:staticcheck // intencional: chave crua para provar que não é aceita
			req = req.WithContext(context.WithValue(req.Context(), "userinfo", &webhookMockUserInfo{id: "42"}))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s /webhook expected 401, got %d", method, w.Code)
			}
		})
	}
}

// TestWebhookHandlers_NoUserInfo_401 garante 401 quando o middleware não rodou.
func TestWebhookHandlers_NoUserInfo_401(t *testing.T) {
	for method, handler := range webhookHandlers(newWebhookTestContext(&webhookFakeDB{})) {
		t.Run(method, func(t *testing.T) {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("%s /webhook panicked instead of responding 401: %v", method, rec)
				}
			}()

			req := httptest.NewRequest(method, "/webhook", strings.NewReader(webhookBody(method)))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s /webhook expected 401, got %d", method, w.Code)
			}
		})
	}
}
