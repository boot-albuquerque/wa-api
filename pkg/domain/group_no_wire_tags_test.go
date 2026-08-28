package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// F267 — "a struct de domínio não é o contrato da rota, e documentar a partir
// dela erra em dez sítios" (HOUSEKEEP.md).
//
// group.go's own package comment states the rule this test enforces:
//
//	"The types in this file are use-case inputs and results. They are NOT
//	the wire format: the public shape of every group route lives in
//	pkg/presentation/http/dto/group, and the mapping between the two is
//	hand-written. [...] Hence: Go-idiomatic names, and no `json` tags."
//
// F267 measured the ten routes where a domain struct, walked as if it were
// the wire contract, would produce the WRONG shape (code vs inviteLink, title
// vs displayText, and so on). Since that entry the group family migrated to
// dedicated request DTOs (pkg/presentation/http/dto/group) that own the wire
// tags — but the group.go comment above is a PROMISE, not an enforced
// invariant, and it was already broken: GetGroupInfoRequest.GroupJID still
// carried `json:"groupJID"`, a dead tag left over from before the DTO
// migration (nothing decodes JSON into domain.GetGroupInfoRequest — see
// pkg/presentation/http/handlers/handler_group.go:170, which decodes
// dtogroup.GetGroupInfoRequest instead). A dead tag on a domain struct is
// exactly the trap F267 described: a future doc generator, SDK exporter, or
// engineer skimming pkg/domain has no way to tell it apart from a live one.
//
// This test is the COMPARISON the F267 correction (option 1) asked for,
// scoped to what group.go itself already promises: it parses the file's own
// AST and fails if ANY field of ANY type declared there carries a `json`
// struct tag. It does not touch handler wiring or change any route's served
// shape (F267's option 2, explicitly out of scope for this fix) — it makes
// the divergence machine-checked so it cannot silently reappear.
//
// WHY AST AND NOT reflect: package-level struct literals cannot be listed by
// reflection without hand-maintaining the list (and a hand-maintained list is
// exactly the kind of thing that goes stale, which is the disease this test
// treats). Parsing the source file directly means a new type added to
// group.go is covered automatically. The same technique is already this
// repository's convention for architectural gates over Go source
// (pkg/presentation/http/handlers/respondjson_ledger_test.go).
// ---------------------------------------------------------------------------

// TestGroupDomainTypesCarryNoWireTags is the test of the defect: it parses
// group.go and asserts that no field of any type declared there has a `json`
// struct tag. Before the fix it fails on GetGroupInfoRequest.GroupJID.
func TestGroupDomainTypesCarryNoWireTags(t *testing.T) {
	violations := jsonTaggedFieldsIn(t, "group.go")
	if len(violations) != 0 {
		t.Errorf("group.go declares it carries no `json` tags (see the file's own "+
			"package comment), but %d field(s) do:\n%s\nEach one is a domain "+
			"struct that would mislead a naive \"walk pkg/domain to document the "+
			"route\" pass — the exact F267 defect. The wire shape belongs in "+
			"pkg/presentation/http/dto/group instead.",
			len(violations), formatViolations(violations))
	}
}

// jsonTaggedFieldsIn parses the named file in this package's directory and
// returns "Type.Field" for every struct field carrying a `json` tag,
// sorted for a deterministic failure message.
func jsonTaggedFieldsIn(t *testing.T, filename string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("could not parse %s: %v", filename, err)
	}

	var violations []string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok || st.Fields == nil {
			return true
		}
		for _, field := range st.Fields.List {
			if field.Tag == nil {
				continue
			}
			// field.Tag.Value includes the surrounding backticks; a bare
			// substring check is enough to detect a `json:"..."` key without
			// needing a full reflect.StructTag parse — this is source text,
			// not a runtime tag, so there is nothing to look up by key.
			if containsJSONTagKey(field.Tag.Value) {
				name := ts.Name.Name + ".<embedded>"
				if len(field.Names) > 0 {
					name = ts.Name.Name + "." + field.Names[0].Name
				}
				violations = append(violations, name+" "+field.Tag.Value)
			}
		}
		return true
	})
	sort.Strings(violations)
	return violations
}

// containsJSONTagKey reports whether a struct tag literal names the `json`
// key. Written as an explicit scan instead of strings.Contains(tag, "json:")
// so the intent — this is scanning SOURCE TEXT of a tag literal, not
// interpreting a live reflect.StructTag — is visible at the call site.
func containsJSONTagKey(tagLiteral string) bool {
	const key = "json:"
	for i := 0; i+len(key) <= len(tagLiteral); i++ {
		if tagLiteral[i:i+len(key)] == key {
			return true
		}
	}
	return false
}

func formatViolations(v []string) string {
	out := ""
	for _, line := range v {
		out += "  " + line + "\n"
	}
	return out
}
