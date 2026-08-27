package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
	"wa-api/pkg/presentation/http/contracttest"
)

// This file is the permanent LIVE regression gate for JSON key naming — the
// third of three naming gates built alongside the six-family HTTP DTO
// migration (docs/HTTP-DTO-CONVENTIONS.md), and the one an OpenAPI-only
// check cannot replace: the spec can promise a canonical shape while the
// running handler still serves the old one (or the other way around), and
// only calling the real handler through the real router catches that
// divergence.
//
// Companion gates:
//   - pkg/bootstrap/naming_paths_gate_test.go   — URL path segments
//   - pkg/bootstrap/naming_openapi_gate_test.go — the generated spec itself
//
// Every test below follows the same three rules §9 of
// docs/HTTP-DTO-CONVENTIONS.md sets for a per-route contract test:
//
//  1. through the ROUTE REGISTERED on a real *mux.Router, exactly as
//     wiring_routes.go registers it — never the bare handler
//     (ARMADILHAS.md #2: a handler mounted by hand exercises neither the
//     method, the path pattern, nor parameter extraction);
//  2. an authenticated session injected into the context, so the response
//     is the SUCCESS body, not the 401 guard;
//  3. contracttest.AssertPublicJSONUsesCanonicalNaming over the real body.
//
// One route per family: session, groups, messages, users, newsletters,
// admin. It is EXPECTED that most of these fail today — as of this gate's
// commit, only /group/info (CAP-DTO foundation) and the already-migrated
// send/* message routes serve canonical JSON; session, users, newsletters
// and admin still echo domain/PascalCase fields directly. That is the sibling
// worktrees' work to land, not this gate's job to hide — see HOUSEKEEP.md.

// liveGateRouter mounts a single handler at a single route on a fresh
// *mux.Router, the same way pkg/bootstrap/wiring_routes.go's
// customhttp.HandlerRegistry does under the hood (Register + Apply).
func liveGateRouter(method, path string, h http.Handler) *mux.Router {
	r := mux.NewRouter()
	r.Handle(path, h).Methods(method)
	return r
}

// liveGateAuthed wraps a handler with an authenticated session in context —
// the same appport.UserInfoKey value AuthAlice would install in production.
func liveGateAuthed(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, withUser(r, "naming-gate-user"))
	})
}

// liveGateBody runs method/path through router and returns the recorded
// response body, failing the test if the route did not answer 200 — a
// non-200 here means the fixture is broken (wrong body, wrong fake wiring),
// not that naming is being tested; fix the fixture, not this helper.
func liveGateBody(t *testing.T, router *mux.Router, method, path, body string) []byte {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fixture quebrada: %s %s respondeu %d, quero 200 (corpo: %s)",
			method, path, rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

// TestLiveNaming_Session covers the session family via GET /session/status
// — the same handler/use case TestGetStatus_ReportsLiveSessionState in
// handler_session_test.go exercises, wired with a live SessionStatusReader
// result so the body isn't an empty struct.
func TestLiveNaming_Session(t *testing.T) {
	status := &contractsfake.SessionStatusReader{
		SessionStatusFunc: func(context.Context, string) (bool, bool) { return true, true },
	}
	users := &contractsfake.UserRepository{
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: "naming-gate-user", JID: "5511999999999@s.whatsapp.net"}}, nil
		},
	}
	h := NewGetStatusHandler(session.NewGetStatusUseCase(status, users, &contractsfake.Logger{}))
	router := liveGateRouter(http.MethodGet, "/session/status", liveGateAuthed(h))

	body := liveGateBody(t, router, http.MethodGet, "/session/status", "")
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, body)
}

// TestLiveNaming_Groups covers the groups family's GetGroupInfoHandler — the
// REFERENCE handler (docs/HTTP-DTO-CONVENTIONS.md's implementação de
// referência) already migrated to the DTO/presenter pattern.
// handler_group_info_contract_test.go asserts the same handler in more
// depth (AssertNoKeys for the old PascalCase names, field-by-field value
// checks); this call exists so the naming gate is a complete, standalone
// per-family survey without having to know that the deeper test lives
// elsewhere.
//
// INTEGRAÇÃO (2026-08-27): a rota REAL registada mudou de
// `POST /group/info` para `GET /groups/{group_jid}` (corte a hard,
// worktree http-dto-paths, F326) — este teste continua a montar o SEU
// PRÓPRIO router isolado com `POST /group/info`, então continua válido
// como medição do handler, mas já não representa a rota que um cliente
// real chamaria. O pedido também passou a ler `group_jid`, não `GroupJID`.
func TestLiveNaming_Groups(t *testing.T) {
	f := newGrpFakes()
	f.directory.GetGroupInfoFunc = func(context.Context, string, domain.JID) (*domain.GroupInfo, error) {
		return grupoDeReferencia(), nil
	}
	h := NewGetGroupInfoHandler(group.NewGetGroupInfoUseCase(f.directory, f.jids, f.logger))
	router := liveGateRouter(http.MethodPost, "/group/info", liveGateAuthed(h))

	body := liveGateBody(t, router, http.MethodPost, "/group/info",
		`{"group_jid":"120363000000000000@g.us"}`)
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, body)
}

// TestLiveNaming_Messages covers the messages family via
// POST /chat/send/contact — already migrated (CAP-08B, port.SimpleMessenger)
// and already serving snake_case (message_id, timestamp, status; see
// TestSendContact_Success_ViaRegisteredRoute in handler_send_contact_test.go).
// A passing case here is as load-bearing as a failing one: it proves the
// gate does not cry wolf on a route that is already correct.
func TestLiveNaming_Messages(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendContactFunc: func(context.Context, string, domain.JID, domain.ContactPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "naming-gate-msg-1", Timestamp: time.Unix(1755500110, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	uc := message.NewSendContactUseCase(sm, jr, silentLogger{})
	h := NewSendContactHandler(uc)
	router := liveGateRouter(http.MethodPost, "/chat/send/contact", liveGateAuthed(h))

	body := liveGateBody(t, router, http.MethodPost, "/chat/send/contact",
		`{"Phone":"5511999999999","Name":"Alice","Vcard":"BEGIN:VCARD\nEND:VCARD"}`)
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, body)
}

// TestLiveNaming_Users covers the users family via GET /users/lid/{jid} —
// reuses uhNewFakes/uhFakes.handlers() from handler_user_test.go, the same
// fakes the 9-route admin/user handler suite is built on, so this gate does
// not maintain a second wiring for UserHandlers.
func TestLiveNaming_Users(t *testing.T) {
	f := uhNewFakes()
	h := f.handlers().GetUserLID()
	router := liveGateRouter(http.MethodGet, "/users/lid/{jid}", liveGateAuthed(h))

	body := liveGateBody(t, router, http.MethodGet, "/users/lid/5511999999999@s.whatsapp.net", "")
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, body)
}

// TestLiveNaming_Admin covers the admin family via GET /admin/users — same
// uhFakes as TestLiveNaming_Users. The zero-value fake's ListUsersFunc
// returns a nil slice, which renders "data":null — a naming check against
// that body would pass VACUOUSLY (nothing to check is not the same as
// nothing wrong), so this test wires a non-empty UserListEntry, same as
// TestLiveNaming_Session does for SessionStatusReader.
func TestLiveNaming_Admin(t *testing.T) {
	f := uhNewFakes()
	f.users.ListUsersFunc = func(context.Context, string) ([]domain.UserListEntry, error) {
		return []domain.UserListEntry{{
			ID: "naming-gate-user", Name: "Alice", JID: "5511999999999@s.whatsapp.net",
			ProxyURL: "http://proxy.example", WebhookUseProxy: true,
		}}, nil
	}
	h := f.handlers().ListUsers()
	router := liveGateRouter(http.MethodGet, "/admin/users", liveGateAuthed(h))

	body := liveGateBody(t, router, http.MethodGet, "/admin/users", "")
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, body)
}

// TestLiveNaming_Newsletters covers the newsletters family via
// GET /newsletter/list.
//
// INTEGRAÇÃO (2026-08-27): esta fixture usava `any` + o tipo vendorizado
// `types.NewsletterMetadata` directamente, porque nesta branch (isolada, sem
// a migração de canais) a porta ListSubscribed ainda devolvia o SDK sem
// transformação. Depois da integração com worktree/http-dto-newsletters, a
// porta passou a devolver `[]domain.NewsletterMetadata` — o adaptador já
// converte, e o handler já apresenta através de dtonewsletter. A fixture
// segue o tipo actual da porta.
func TestLiveNaming_Newsletters(t *testing.T) {
	nr := &contractsfake.NewsletterReader{
		ListSubscribedFunc: func(context.Context, string) ([]domain.NewsletterMetadata, error) {
			return []domain.NewsletterMetadata{{
				JID:  domain.JID("120363000000000000@newsletter"),
				Name: domain.NewsletterText{Text: "Canal X"},
			}}, nil
		},
	}
	h := NewListNewsletterHandler(notification.NewListNewsletterUseCase(nr, &contractsfake.Logger{}))
	router := liveGateRouter(http.MethodGet, "/newsletter/list", liveGateAuthed(h))

	body := liveGateBody(t, router, http.MethodGet, "/newsletter/list", "")
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, body)
}
