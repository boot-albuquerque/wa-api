package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain"
)

// --- SetDisappearingTimerHandler (POST /chat/ephemeral) -------------------

func disappearingFakes() (*contractsfake.ChatOperations, *contractsfake.JIDResolver, *contractsfake.Logger) {
	return &contractsfake.ChatOperations{}, &contractsfake.JIDResolver{}, &contractsfake.Logger{}
}

func disappearingHandler(ops *contractsfake.ChatOperations, jids *contractsfake.JIDResolver, logger *contractsfake.Logger) *SetDisappearingTimerHandler {
	return NewSetDisappearingTimerHandler(
		chat.NewSetDisappearingTimerUseCase(ops, jids, logger))
}

func disappearingRouter(h http.Handler) *mux.Router {
	r := mux.NewRouter()
	r.Handle("/chat/ephemeral", h).Methods("POST")
	return r
}

func serveDisappearing(ops *contractsfake.ChatOperations, jids *contractsfake.JIDResolver, logger *contractsfake.Logger, body string) (*httptest.ResponseRecorder, *logCapture) {
	h, capture := logassert.Wrap(disappearingHandler(ops, jids, logger))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/ephemeral", strings.NewReader(body))
	disappearingRouter(h).ServeHTTP(rec, withUser(r, "user-1"))
	return rec, capture
}

// assertAppErrCode decodes the error field of the ADR-002 envelope and
// asserts the apperr code matches wantCode.
func assertAppErrCode(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	env := decodeEnvelope(t, rec)
	var errObj struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(env.Error, &errObj); err != nil {
		t.Fatalf("error field is not a structured apperr object: %v (raw: %s)", err, env.Error)
	}
	if errObj.Code != wantCode {
		t.Fatalf("error.code: got %q, want %q", errObj.Code, wantCode)
	}
	if errObj.Message == "" {
		t.Fatal("error.message is empty")
	}
}

func TestSetDisappearingTimer_Success(t *testing.T) {
	for _, dur := range []string{"0", "off", "24h", "7d", "90d"} {
		t.Run(dur, func(t *testing.T) {
			ops, jids, logger := disappearingFakes()
			body := `{"chat":"5511999999999@s.whatsapp.net","duration":"` + dur + `"}`
			rec, _ := serveDisappearing(ops, jids, logger, body)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if !env.Success {
				t.Fatalf("envelope.success=false on happy path: %s", rec.Body.String())
			}
			if len(ops.SetDisappearingTimerCalls) != 1 {
				t.Fatalf("port calls: got %d, want 1", len(ops.SetDisappearingTimerCalls))
			}
			call := ops.SetDisappearingTimerCalls[0]
			if call.Chat != domain.JID("5511999999999@s.whatsapp.net") {
				t.Fatalf("chat JID: got %q, want 5511999999999@s.whatsapp.net", call.Chat)
			}
			wantDur := map[string]time.Duration{
				"0": 0, "off": 0,
				"24h": 24 * time.Hour,
				"7d":  7 * 24 * time.Hour,
				"90d": 90 * 24 * time.Hour,
			}
			if call.Duration != wantDur[dur] {
				t.Fatalf("duration: got %v, want %v", call.Duration, wantDur[dur])
			}
		})
	}
}

func TestSetDisappearingTimer_MalformedBody(t *testing.T) {
	ops, jids, logger := disappearingFakes()
	rec, capture := serveDisappearing(ops, jids, logger, `{"chat":`)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	logassert.OutcomeLogged(t, capture.Records(t))
	if len(ops.SetDisappearingTimerCalls) != 0 {
		t.Fatal("malformed body reached the port")
	}
}

func TestSetDisappearingTimer_MissingChat(t *testing.T) {
	ops, jids, logger := disappearingFakes()
	rec, capture := serveDisappearing(ops, jids, logger, `{"duration":"24h"}`)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	assertAppErrCode(t, rec, "missing_chat")
	got := logassert.OutcomeLogged(t, capture.Records(t), "missing chat")
	if got.str("level") != "warn" {
		t.Fatalf("level: got %q, want warn", got.str("level"))
	}
	if len(ops.SetDisappearingTimerCalls) != 0 {
		t.Fatal("missing chat reached the port")
	}
}

func TestSetDisappearingTimer_MissingDuration(t *testing.T) {
	ops, jids, logger := disappearingFakes()
	rec, capture := serveDisappearing(ops, jids, logger, `{"chat":"5511999999999@s.whatsapp.net"}`)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	assertAppErrCode(t, rec, "missing_duration")
	got := logassert.OutcomeLogged(t, capture.Records(t), "missing duration")
	if got.str("level") != "warn" {
		t.Fatalf("level: got %q, want warn", got.str("level"))
	}
	if len(ops.SetDisappearingTimerCalls) != 0 {
		t.Fatal("missing duration reached the port")
	}
}

func TestSetDisappearingTimer_ZeroVsAbsent(t *testing.T) {
	t.Run("explicit zero is accepted (duration=0)", func(t *testing.T) {
		ops, jids, logger := disappearingFakes()
		rec, _ := serveDisappearing(ops, jids, logger, `{"chat":"5511999999999@s.whatsapp.net","duration":"0"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 — explicit '0' must disable the timer", rec.Code)
		}
		if len(ops.SetDisappearingTimerCalls) != 1 {
			t.Fatal("explicit zero did not reach the port")
		}
		if ops.SetDisappearingTimerCalls[0].Duration != 0 {
			t.Fatalf("duration: got %v, want 0", ops.SetDisappearingTimerCalls[0].Duration)
		}
	})

	t.Run("absent duration is rejected (null)", func(t *testing.T) {
		ops, jids, logger := disappearingFakes()
		rec, _ := serveDisappearing(ops, jids, logger, `{"chat":"5511999999999@s.whatsapp.net","duration":null}`)

		assertErrorEnvelope(t, rec, http.StatusBadRequest)
		assertAppErrCode(t, rec, "missing_duration")
		if len(ops.SetDisappearingTimerCalls) != 0 {
			t.Fatal("null duration reached the port")
		}
	})

	t.Run("absent duration is rejected (omitted)", func(t *testing.T) {
		ops, jids, logger := disappearingFakes()
		rec, _ := serveDisappearing(ops, jids, logger, `{"chat":"5511999999999@s.whatsapp.net"}`)

		assertErrorEnvelope(t, rec, http.StatusBadRequest)
		assertAppErrCode(t, rec, "missing_duration")
		if len(ops.SetDisappearingTimerCalls) != 0 {
			t.Fatal("omitted duration reached the port")
		}
	})
}

func TestSetDisappearingTimer_InvalidDuration(t *testing.T) {
	ops, jids, logger := disappearingFakes()
	rec, _ := serveDisappearing(ops, jids, logger, `{"chat":"5511999999999@s.whatsapp.net","duration":"30d"}`)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	assertAppErrCode(t, rec, "invalid_duration")
	if len(ops.SetDisappearingTimerCalls) != 0 {
		t.Fatal("invalid duration reached the port")
	}
}

func TestSetDisappearingTimer_SessionFailure(t *testing.T) {
	ops, jids, logger := disappearingFakes()
	ops.EnsureSessionFunc = func(context.Context, string) error { return errors.New("no session") }
	rec, _ := serveDisappearing(ops, jids, logger, `{"chat":"5511999999999@s.whatsapp.net","duration":"24h"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
	if len(ops.SetDisappearingTimerCalls) != 0 {
		t.Fatal("session guard bypassed")
	}
}

func TestSetDisappearingTimer_UseCaseFailure(t *testing.T) {
	ops, jids, logger := disappearingFakes()
	ops.SetDisappearingTimerFunc = func(context.Context, string, domain.JID, time.Duration, time.Time) error {
		return errors.New("sdk error")
	}
	rec, capture := serveDisappearing(ops, jids, logger, `{"chat":"5511999999999@s.whatsapp.net","duration":"7d"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
	logassert.OutcomeLogged(t, capture.Records(t), "sdk error")
}

// --- SetDefaultDisappearingTimerHandler (POST /chat/ephemeral/default) ----

func defaultDisappearingFakes() (*contractsfake.DefaultDisappearingTimerSetter, *contractsfake.Logger) {
	return &contractsfake.DefaultDisappearingTimerSetter{}, &contractsfake.Logger{}
}

func defaultDisappearingHandler(setter *contractsfake.DefaultDisappearingTimerSetter, logger *contractsfake.Logger) *SetDefaultDisappearingTimerHandler {
	return NewSetDefaultDisappearingTimerHandler(
		chat.NewSetDefaultDisappearingTimerUseCase(setter, logger))
}

func defaultDisappearingRouter(h http.Handler) *mux.Router {
	r := mux.NewRouter()
	r.Handle("/chat/ephemeral/default", h).Methods("POST")
	return r
}

func serveDefaultDisappearing(setter *contractsfake.DefaultDisappearingTimerSetter, logger *contractsfake.Logger, body string) (*httptest.ResponseRecorder, *logCapture) {
	h, capture := logassert.Wrap(defaultDisappearingHandler(setter, logger))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/ephemeral/default", strings.NewReader(body))
	defaultDisappearingRouter(h).ServeHTTP(rec, withUser(r, "user-1"))
	return rec, capture
}

func TestSetDefaultDisappearingTimer_Success(t *testing.T) {
	for _, dur := range []string{"0", "off", "24h", "7d", "90d"} {
		t.Run(dur, func(t *testing.T) {
			setter, logger := defaultDisappearingFakes()
			rec, _ := serveDefaultDisappearing(setter, logger, `{"duration":"`+dur+`"}`)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if !env.Success {
				t.Fatalf("envelope.success=false on happy path: %s", rec.Body.String())
			}
			if len(setter.SetDefaultDisappearingTimerCalls) != 1 {
				t.Fatalf("port calls: got %d, want 1", len(setter.SetDefaultDisappearingTimerCalls))
			}
			wantDur := map[string]time.Duration{
				"0": 0, "off": 0,
				"24h": 24 * time.Hour,
				"7d":  7 * 24 * time.Hour,
				"90d": 90 * 24 * time.Hour,
			}
			if setter.SetDefaultDisappearingTimerCalls[0].Duration != wantDur[dur] {
				t.Fatalf("duration: got %v, want %v",
					setter.SetDefaultDisappearingTimerCalls[0].Duration, wantDur[dur])
			}
		})
	}
}

func TestSetDefaultDisappearingTimer_MalformedBody(t *testing.T) {
	setter, logger := defaultDisappearingFakes()
	rec, capture := serveDefaultDisappearing(setter, logger, `{bad`)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	logassert.OutcomeLogged(t, capture.Records(t))
	if len(setter.SetDefaultDisappearingTimerCalls) != 0 {
		t.Fatal("malformed body reached the port")
	}
}

func TestSetDefaultDisappearingTimer_MissingDuration(t *testing.T) {
	setter, logger := defaultDisappearingFakes()
	rec, capture := serveDefaultDisappearing(setter, logger, `{}`)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	assertAppErrCode(t, rec, "missing_duration")
	logassert.OutcomeLogged(t, capture.Records(t), "missing duration")
	if len(setter.SetDefaultDisappearingTimerCalls) != 0 {
		t.Fatal("missing duration reached the port")
	}
}

func TestSetDefaultDisappearingTimer_ZeroVsAbsent(t *testing.T) {
	t.Run("explicit zero is accepted", func(t *testing.T) {
		setter, logger := defaultDisappearingFakes()
		rec, _ := serveDefaultDisappearing(setter, logger, `{"duration":"0"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200", rec.Code)
		}
		if len(setter.SetDefaultDisappearingTimerCalls) != 1 {
			t.Fatal("explicit zero did not reach the port")
		}
		if setter.SetDefaultDisappearingTimerCalls[0].Duration != 0 {
			t.Fatalf("duration: got %v, want 0", setter.SetDefaultDisappearingTimerCalls[0].Duration)
		}
	})

	t.Run("null duration is rejected", func(t *testing.T) {
		setter, logger := defaultDisappearingFakes()
		rec, _ := serveDefaultDisappearing(setter, logger, `{"duration":null}`)

		assertErrorEnvelope(t, rec, http.StatusBadRequest)
		assertAppErrCode(t, rec, "missing_duration")
		if len(setter.SetDefaultDisappearingTimerCalls) != 0 {
			t.Fatal("null duration reached the port")
		}
	})

	t.Run("omitted duration is rejected", func(t *testing.T) {
		setter, logger := defaultDisappearingFakes()
		rec, _ := serveDefaultDisappearing(setter, logger, `{}`)

		assertErrorEnvelope(t, rec, http.StatusBadRequest)
		assertAppErrCode(t, rec, "missing_duration")
		if len(setter.SetDefaultDisappearingTimerCalls) != 0 {
			t.Fatal("omitted duration reached the port")
		}
	})
}

func TestSetDefaultDisappearingTimer_InvalidDuration(t *testing.T) {
	setter, logger := defaultDisappearingFakes()
	rec, _ := serveDefaultDisappearing(setter, logger, `{"duration":"30d"}`)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	assertAppErrCode(t, rec, "invalid_duration")
	if len(setter.SetDefaultDisappearingTimerCalls) != 0 {
		t.Fatal("invalid duration reached the port")
	}
}

func TestSetDefaultDisappearingTimer_SessionFailure(t *testing.T) {
	setter, logger := defaultDisappearingFakes()
	setter.EnsureSessionFunc = func(context.Context, string) error { return errors.New("no session") }
	rec, _ := serveDefaultDisappearing(setter, logger, `{"duration":"24h"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
	if len(setter.SetDefaultDisappearingTimerCalls) != 0 {
		t.Fatal("session guard bypassed")
	}
}

func TestSetDefaultDisappearingTimer_UseCaseFailure(t *testing.T) {
	setter, logger := defaultDisappearingFakes()
	setter.SetDefaultDisappearingTimerFunc = func(context.Context, string, time.Duration) error {
		return errors.New("sdk error")
	}
	rec, capture := serveDefaultDisappearing(setter, logger, `{"duration":"90d"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
	logassert.OutcomeLogged(t, capture.Records(t), "sdk error")
}
