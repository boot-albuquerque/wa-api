package handlers

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"wa-api/pkg/application/usecase/group"
	"wa-api/pkg/presentation/http/contracttest"

	customhttp "wa-api/pkg/presentation/http"

	"github.com/gorilla/mux"
)

// This file guards the SECOND half of the error contract. The first half —
// `error` is always an object, never a string, and never leaks the wrapped
// error's text — is asserted in pkg/presentation/http/response_test.go and is
// NOT repeated here.
//
// What is asserted here is what that file cannot see: the codes the handlers
// actually PUT into the envelope. RespondJSON is happy to serialise
// `error.code = "GroupJID"`; only a test that reads real codes out of real
// routes catches that.

// ---------------------------------------------------------------------------
// (1) Every error code in the repository is canonical snake_case
// ---------------------------------------------------------------------------

// TestErrorCodesAreCanonicalSnakeCase walks the whole of pkg/ and asserts that
// every literal error code — whether built with apperr.New or written as an
// AppError composite literal — matches the same rule as every other public
// enumerated value on this API's wire (docs/HTTP-DTO-CONVENTIONS.md §8).
//
// It is a REPOSITORY-wide gate and not a package-wide one on purpose: the code
// travels from wherever the use case constructs it all the way to
// `error.code`, so a camelCase code born in pkg/application ends up on the
// wire exactly as a camelCase code born here would. Restricting the scan to
// the handler package would leave the 284 call sites that actually raise these
// codes unguarded.
func TestErrorCodesAreCanonicalSnakeCase(t *testing.T) {
	root := repositoryPkgDir(t)

	type finding struct {
		position string
		code     string
	}
	var offenders []finding
	seen := map[string]bool{}

	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			for _, lit := range errorCodeLiterals(n) {
				code, unquoteErr := strconv.Unquote(lit.Value)
				if unquoteErr != nil {
					// A non-constant code (a concatenation, a variable). The
					// canonical-naming rule cannot be decided statically for
					// it; the round-trip tests below are what cover those.
					continue
				}
				seen[code] = true
				if !contracttest.IsCanonicalKey(code) {
					offenders = append(offenders, finding{fset.Position(lit.Pos()).String(), code})
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking %s: %v", root, walkErr)
	}

	if len(seen) == 0 {
		// A scan that finds nothing passes vacuously, which is the failure
		// mode this whole gate exists to avoid — a renamed constructor would
		// silently switch it off.
		t.Fatal("the scan found no error code at all: the AST matcher no longer recognises how codes are written")
	}
	if len(offenders) > 0 {
		sort.Slice(offenders, func(i, j int) bool { return offenders[i].position < offenders[j].position })
		var b strings.Builder
		for _, o := range offenders {
			b.WriteString("\n\t" + o.position + ": " + strconv.Quote(o.code))
		}
		t.Fatalf("%d error code(s) are not canonical snake_case (^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$):%s",
			len(offenders), b.String())
	}
	t.Logf("scanned %d distinct error codes, all canonical", len(seen))
}

// errorCodeLiterals returns the string literals that a node contributes as
// error CODES: the first argument of apperr.New, and the Code field of an
// AppError composite literal.
func errorCodeLiterals(n ast.Node) []*ast.BasicLit {
	switch node := n.(type) {
	case *ast.CallExpr:
		sel, ok := node.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "New" {
			return nil
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "apperr" || len(node.Args) == 0 {
			return nil
		}
		if lit, ok := node.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			return []*ast.BasicLit{lit}
		}
	case *ast.CompositeLit:
		if !isAppErrorType(node.Type) {
			return nil
		}
		var out []*ast.BasicLit
		for _, elt := range node.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Code" {
				continue
			}
			if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				out = append(out, lit)
			}
		}
		return out
	}
	return nil
}

// isAppErrorType reports whether an expression names apperr.AppError, with or
// without the package qualifier (the apperr package writes it unqualified).
func isAppErrorType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		pkg, ok := t.X.(*ast.Ident)
		return ok && pkg.Name == "apperr" && t.Sel.Name == "AppError"
	case *ast.Ident:
		return t.Name == "AppError"
	}
	return false
}

// repositoryPkgDir locates pkg/ from this test's own directory, so the gate
// does not depend on where `go test` was invoked from.
func repositoryPkgDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// .../pkg/presentation/http/handlers -> .../pkg
	dir := filepath.Join(wd, "..", "..", "..")
	if base := filepath.Base(dir); base != "pkg" {
		t.Fatalf("expected to find pkg/ at %s, found %q", dir, base)
	}
	return dir
}

// ---------------------------------------------------------------------------
// (2) Round trip: the codes this change introduced, read off a real response
// ---------------------------------------------------------------------------

// errorCodeOf serves one request through a router and returns the envelope's
// error.code, failing if the envelope is not the canonical error shape.
func errorCodeOf(t *testing.T, router http.Handler, method, path, body string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))

	var envelope struct {
		Success bool             `json:"success"`
		Code    int              `json:"code"`
		Error   *json.RawMessage `json:"error"`
	}
	raw := rec.Body.Bytes()
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("response is not the canonical envelope (%v): %s", err, raw)
	}
	if envelope.Success {
		t.Fatalf("success=true on an error response: %s", raw)
	}
	if envelope.Code != rec.Code {
		t.Fatalf("envelope.code=%d but the status line says %d", envelope.Code, rec.Code)
	}
	if envelope.Error == nil {
		t.Fatalf("error key missing: %s", raw)
	}
	var errObj struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(*envelope.Error, &errObj); err != nil {
		t.Fatalf("error is not an object {code,message} (%v): %s", err, raw)
	}
	if errObj.Code == "" || errObj.Message == "" {
		t.Fatalf("error.code or error.message empty: %s", raw)
	}
	if !contracttest.IsCanonicalKey(errObj.Code) {
		t.Fatalf("error.code %q is not canonical snake_case", errObj.Code)
	}
	return rec.Code, errObj.Code
}

// groupMgmtRouter mounts the group-management write routes the way
// wiring_routes.go does, so the assertions below run through the registered
// route and not through a bare handler (ARMADILHAS #2).
func groupMgmtRouter(t *testing.T, f *grpMgmtFakes) *mux.Router {
	t.Helper()
	h := f.handlers()
	registry := customhttp.NewHandlerRegistry()
	for path, handler := range map[string]http.Handler{
		"/group/create":       h.CreateGroup,
		"/group/join":         h.GroupJoin,
		"/group/leave":        h.GroupLeave,
		"/group/name":         h.SetGroupName,
		"/group/topic":        h.SetGroupTopic,
		"/group/photo":        h.SetGroupPhoto,
		"/group/participants": h.UpdateGroupParticipants,
	} {
		registry.Register(path, withContractUser(handler), http.MethodPost)
	}
	router := mux.NewRouter()
	registry.Apply(router)
	return router
}

// TestGroupMgmtErrorCodes_AreSpecific is the round trip for every code this
// change introduced on the group-management family.
//
// The point is the SPECIFICITY, not the shape: each of these conditions used
// to answer `invalid_request`, which told the caller only that "something was
// wrong" with a body that has up to four fields. The `want` column is the
// stable identifier a client branches on.
func TestGroupMgmtErrorCodes_AreSpecific(t *testing.T) {
	cases := []struct {
		name string
		path string
		body string
		want string
	}{
		{"create sem name", "/group/create", `{"participants":["5511999999999"]}`, CodeMissingName},
		{"create sem participants", "/group/create", `{"name":"squad"}`, CodeMissingParticipants},
		{"create com participante vazio", "/group/create", `{"name":"squad","participants":[""]}`, CodeEmptyParticipant},
		{"create com is_parent e linked_parent_jid", "/group/create", `{"name":"x","is_parent":true,"linked_parent_jid":"120363@g.us"}`, CodeMutuallyExclusiveParent},
		{"join sem code", "/group/join", `{}`, CodeMissingInviteCode},
		{"leave sem groupJID", "/group/leave", `{}`, CodeMissingGroupJID},
		{"name sem name", "/group/name", `{"GroupJID":"120363@g.us"}`, CodeMissingName},
		{"topic sem topic", "/group/topic", `{"GroupJID":"120363@g.us"}`, CodeMissingTopic},
		{"photo sem photo", "/group/photo", `{"GroupJID":"120363@g.us"}`, CodeMissingPhoto},
		{"photo nao-base64", "/group/photo", `{"GroupJID":"120363@g.us","Photo":"not-base64!!!"}`, CodeInvalidPhotoEncoding},
		{"participants sem phones", "/group/participants", `{"GroupJID":"120363@g.us"}`, CodeMissingPhones},
		{"participants com phone vazio", "/group/participants", `{"GroupJID":"120363@g.us","Phone":[""],"Action":"add"}`, CodeEmptyPhone},
		{"participants sem action", "/group/participants", `{"GroupJID":"120363@g.us","Phone":["5511999999999"]}`, CodeMissingAction},
		// The two spellings of the group JID on the wire — `groupJID` on
		// /group/leave, `groupjid` on /group/participants — must answer with
		// ONE code. Deriving the code from the field name would have produced
		// two, and neither of them canonical.
		{"participants sem groupjid", "/group/participants", `{"Phone":["5511999999999"],"Action":"add"}`, CodeMissingGroupJID},
		{"corpo ilegivel", "/group/create", `{`, CodeDecodePayload},
	}

	router := groupMgmtRouter(t, newGrpMgmtFakes())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, code := errorCodeOf(t, router, http.MethodPost, tc.path, tc.body)
			if status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", status)
			}
			if code != tc.want {
				t.Errorf("error.code = %q, want %q", code, tc.want)
			}
		})
	}
}

// TestGroupMgmtErrorCodes_MessageDoesNotLeakInternals proves the base64 branch
// keeps the decoder's own text — which quotes a byte of the REQUEST — out of
// error.message. That branch is the only one here whose cause is derived from
// user data, so it is the only one where a careless `%w` into Message would
// have shipped request bytes back out.
func TestGroupMgmtErrorCodes_MessageDoesNotLeakInternals(t *testing.T) {
	router := groupMgmtRouter(t, newGrpMgmtFakes())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/group/photo",
		strings.NewReader(`{"GroupJID":"120363@g.us","Photo":"AAAA$SECRET$"}`)))

	body := rec.Body.String()
	for _, forbidden := range []string{"SECRET", "illegal base64", "base64.CorruptInputError"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("error.message leaked %q: %s", forbidden, body)
		}
	}
}

// TestGroupMgmtErrorCodes_SuccessStillCarriesNoError is the positive control
// for the table above: if the fakes rejected everything, every row would pass
// for the wrong reason.
func TestGroupMgmtErrorCodes_SuccessStillCarriesNoError(t *testing.T) {
	f := newGrpMgmtFakes()
	router := groupMgmtRouter(t, f)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/group/create",
		strings.NewReader(`{"name":"squad","participants":["5511999999999"]}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if envelope["success"] != true {
		t.Errorf("success = %v, want true", envelope["success"])
	}
	if _, has := envelope["error"]; has {
		t.Errorf("success envelope carries an error key: %s", rec.Body.String())
	}
	if len(f.lifecycle.CreateGroupCalls) != 1 {
		t.Errorf("the port was called %d times, want 1", len(f.lifecycle.CreateGroupCalls))
	}
}

// groupUseCaseUnused keeps the group import honest if the table above ever
// stops needing it; it is referenced by groupMgmtRouter's construction path.
var _ = group.NewGroupManagementUseCase

// ---------------------------------------------------------------------------
// (3) Round trip: the webhook family's codes
// ---------------------------------------------------------------------------

// webhookErrorRouter mounts the four /webhook verbs on one registered path,
// which is how wiring_routes.go mounts them — and the reason the assertion has
// to go through the router at all: the four handlers share a path and differ
// only by method, so a bare handler would never prove which one answered.
func webhookErrorRouter(t *testing.T, ctx *WebhookHandlerContext) *mux.Router {
	t.Helper()
	registry := customhttp.NewHandlerRegistry()
	for method, h := range webhookHandlers(ctx) {
		registry.Register("/webhook", withContractUser(h), method)
	}
	router := mux.NewRouter()
	registry.Apply(router)
	return router
}

// TestWebhookErrorCodes_AreSpecificPerOperation proves the four persistence
// codes are DISTINCT per verb. Collapsing them into one `webhook_failed` would
// still satisfy every shape assertion in response_test.go, and would still be
// a regression: a caller cannot tell a failed read from a failed delete.
func TestWebhookErrorCodes_AreSpecificPerOperation(t *testing.T) {
	const boom = "connection refused by postgres at 10.0.0.7:5432"

	cases := []struct {
		name   string
		method string
		body   string
		db     *webhookFakeDB
		want   string
	}{
		{"GET com query quebrada", http.MethodGet, "", &webhookFakeDB{queryErr: errors.New(boom)}, CodeWebhookReadFailed},
		{"POST com escrita quebrada", http.MethodPost, `{"webhookurl":"https://example.com/hook","events":["Message"]}`, &webhookFakeDB{execErr: errors.New(boom)}, CodeWebhookWriteFailed},
		{"PUT com escrita quebrada", http.MethodPut, `{"webhook":"https://example.com/hook","events":["Message"],"active":true}`, &webhookFakeDB{execErr: errors.New(boom)}, CodeWebhookUpdateFailed},
		{"DELETE com escrita quebrada", http.MethodDelete, "", &webhookFakeDB{execErr: errors.New(boom)}, CodeWebhookDeleteFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := webhookErrorRouter(t, newWebhookTestContext(tc.db))
			status, code := errorCodeOf(t, router, tc.method, "/webhook", tc.body)
			if status != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", status)
			}
			if code != tc.want {
				t.Errorf("error.code = %q, want %q", code, tc.want)
			}
		})
	}

	// The driver's text names a host and a port. It reaches the log through
	// the WRAPPED cause and must never reach the body.
	t.Run("o texto do driver nao chega ao corpo", func(t *testing.T) {
		router := webhookErrorRouter(t, newWebhookTestContext(&webhookFakeDB{queryErr: errors.New(boom)}))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/webhook", nil))
		if strings.Contains(rec.Body.String(), "10.0.0.7") || strings.Contains(rec.Body.String(), "postgres") {
			t.Fatalf("o corpo vazou o texto do driver: %s", rec.Body.String())
		}
	})
}

// TestWebhookErrorCodes_MalformedBody proves the two decode branches answer
// with the SHARED code, not with a per-route invention: the same condition on
// two routes must not produce two identifiers.
func TestWebhookErrorCodes_MalformedBody(t *testing.T) {
	router := webhookErrorRouter(t, newWebhookTestContext(&webhookFakeDB{}))
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			status, code := errorCodeOf(t, router, method, "/webhook", `{"webhookurl":`)
			if status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", status)
			}
			if code != CodeDecodePayload {
				t.Errorf("error.code = %q, want %q", code, CodeDecodePayload)
			}
		})
	}
}
