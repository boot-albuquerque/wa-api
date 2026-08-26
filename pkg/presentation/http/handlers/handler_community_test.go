package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// --- community test scaffolding ------------------------------------------

var comErrPort = errors.New("community port exploded")
var comErrNoSession = errors.New("no wanoise session for user")

type comFakes struct {
	directory *contractsfake.CommunityDirectory
	lifecycle *contractsfake.CommunityLifecycle
	jids      *contractsfake.JIDResolver
	logger    *contractsfake.Logger
}

func newComFakes() *comFakes {
	return &comFakes{
		directory: &contractsfake.CommunityDirectory{},
		lifecycle: &contractsfake.CommunityLifecycle{},
		jids:      &contractsfake.JIDResolver{},
		logger:    &contractsfake.Logger{},
	}
}

func (f *comFakes) failSession(err error) {
	f.directory.EnsureSessionFunc = func(context.Context, string) error { return err }
	f.lifecycle.EnsureSessionFunc = func(context.Context, string) error { return err }
}

type comReadCase struct {
	name      string
	method    string
	path      string
	body      string
	readsBody bool
	build     func(*comFakes) http.Handler
	failOp    func(*comFakes, error)
}

func comReadCases() []comReadCase {
	return []comReadCase{
		{
			name:      "GetCommunitySubGroups",
			method:    http.MethodPost,
			path:      "/community/subgroups",
			body:      `{"communityJID":"120363@g.us"}`,
			readsBody: true,
			build: func(f *comFakes) http.Handler {
				return NewGetCommunitySubGroupsHandler(group.NewCommunityReadUseCase(f.directory, f.jids, f.logger))
			},
			failOp: func(f *comFakes, err error) {
				f.directory.GetSubGroupsFunc = func(context.Context, string, domain.JID) (any, error) {
					return nil, err
				}
			},
		},
		{
			name:      "GetCommunityParticipants",
			method:    http.MethodPost,
			path:      "/community/participants",
			body:      `{"communityJID":"120363@g.us"}`,
			readsBody: true,
			build: func(f *comFakes) http.Handler {
				return NewGetCommunityParticipantsHandler(group.NewCommunityReadUseCase(f.directory, f.jids, f.logger))
			},
			failOp: func(f *comFakes, err error) {
				f.directory.GetLinkedGroupsParticipantsFunc = func(context.Context, string, domain.JID) (any, error) {
					return nil, err
				}
			},
		},
		{
			name:      "CommunityLinkGroup",
			method:    http.MethodPost,
			path:      "/community/link",
			body:      `{"communityJID":"120363@g.us","groupJID":"999888@g.us"}`,
			readsBody: true,
			build: func(f *comFakes) http.Handler {
				return NewCommunityLinkGroupHandler(group.NewCommunityWriteUseCase(f.lifecycle, f.jids, f.logger))
			},
			failOp: func(f *comFakes, err error) {
				f.lifecycle.LinkGroupFunc = func(context.Context, string, domain.JID, domain.JID) error {
					return err
				}
			},
		},
		{
			name:      "CommunityUnlinkGroup",
			method:    http.MethodPost,
			path:      "/community/unlink",
			body:      `{"communityJID":"120363@g.us","groupJID":"999888@g.us"}`,
			readsBody: true,
			build: func(f *comFakes) http.Handler {
				return NewCommunityUnlinkGroupHandler(group.NewCommunityWriteUseCase(f.lifecycle, f.jids, f.logger))
			},
			failOp: func(f *comFakes, err error) {
				f.lifecycle.UnlinkGroupFunc = func(context.Context, string, domain.JID, domain.JID) error {
					return err
				}
			},
		},
	}
}

func comServe(tc comReadCase, f *comFakes, body string) (*httptest.ResponseRecorder, *logCapture) {
	h, capture := logassert.Wrap(tc.build(f))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
	h.ServeHTTP(rec, withUser(r, "user-1"))
	return rec, capture
}

// --- happy path ----------------------------------------------------------

func TestCommunityHandlers_Success(t *testing.T) {
	for _, tc := range comReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newComFakes()
			rec, _ := comServe(tc, f, tc.body)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if !env.Success {
				t.Fatalf("envelope.success=false on happy path: %s", rec.Body.String())
			}
		})
	}
}

// --- malformed body → 400 ------------------------------------------------

func TestCommunityHandlers_MalformedBody(t *testing.T) {
	for _, tc := range comReadCases() {
		if !tc.readsBody {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			f := newComFakes()
			rec, capture := comServe(tc, f, `{"communityJID": "120363`)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			got := logassert.OutcomeLogged(t, capture.Records(t))
			if got.str("level") != "warn" {
				t.Fatalf("client rejection logged at %q, want warn", got.str("level"))
			}
		})
	}
}

// --- missing required field → 400 ----------------------------------------

func TestCommunityHandlers_MissingRequiredField(t *testing.T) {
	for _, tc := range comReadCases() {
		if !tc.readsBody {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			f := newComFakes()
			rec, capture := comServe(tc, f, `{}`)

			if rec.Code < 400 {
				t.Fatalf("missing required field produced status %d", rec.Code)
			}
			logassert.OutcomeLogged(t, capture.Records(t), "missing")
		})
	}
}

// --- session failure → 500 -----------------------------------------------

func TestCommunityHandlers_SessionFailure(t *testing.T) {
	for _, tc := range comReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newComFakes()
			f.failSession(comErrNoSession)

			rec, capture := comServe(tc, f, tc.body)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			got := logassert.OutcomeLogged(t, capture.Records(t), comErrNoSession.Error())
			if got.str("level") != "error" {
				t.Fatalf("dependency failure logged at %q, want error", got.str("level"))
			}
		})
	}
}

// --- use case (port) failure → 500 ---------------------------------------

func TestCommunityHandlers_UseCaseFailure(t *testing.T) {
	for _, tc := range comReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newComFakes()
			tc.failOp(f, comErrPort)

			rec, capture := comServe(tc, f, tc.body)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			got := logassert.OutcomeLogged(t, capture.Records(t), comErrPort.Error())
			if got.str("level") != "error" {
				t.Fatalf("port failure logged at %q, want error", got.str("level"))
			}
		})
	}
}

// --- reject without session user → 401 -----------------------------------

func TestCommunityHandlers_RejectWithoutSession(t *testing.T) {
	for _, tc := range comReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newComFakes()
			h, _ := logassert.Wrap(tc.build(f))

			noUser := httptest.NewRecorder()
			h.ServeHTTP(noUser, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			assertErrorEnvelope(t, noUser, http.StatusUnauthorized)

			noID := httptest.NewRecorder()
			h.ServeHTTP(noID, withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)), ""))
			assertErrorEnvelope(t, noID, http.StatusBadRequest)

			dirCalls := len(f.directory.EnsureSessionCalls)
			lcCalls := len(f.lifecycle.EnsureSessionCalls)
			if n := dirCalls + lcCalls; n != 0 {
				t.Fatalf("request without session reached the port %d time(s)", n)
			}
		})
	}
}

// --- AppError → classified status, not 500 --------------------------------

func TestCommunityHandlers_AppErrorNotInternalServerError(t *testing.T) {
	for _, tc := range comReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newComFakes()
			appErr := apperr.New("invalid_community", apperr.CategoryValidation,
				"not a valid community JID", false, nil)
			tc.failOp(f, appErr)

			rec, _ := comServe(tc, f, tc.body)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
		})
	}
}

// --- no secrets in logs --------------------------------------------------

func TestCommunityHandlers_NeverLogSecrets(t *testing.T) {
	for _, tc := range comReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newComFakes()
			tc.failOp(f, comErrPort)

			h, capture := logassert.Wrap(tc.build(f))
			rec := httptest.NewRecorder()
			body := `{"communityJID":"120363@g.us","groupJID":"999888@g.us","secret":"` +
				logassertGlobalEncryptionKey + `","hmac":"` + logassertGlobalHMACKey + `"}`
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
			r.Header.Set("Authorization", logassertAdminToken)
			h.ServeHTTP(rec, withUser(r, "user-1"))

			if rec.Code < 400 {
				t.Fatalf("leak test needs an error path; status %d", rec.Code)
			}
			logassert.NoSecrets(t, capture.Records(t))
		})
	}
}
