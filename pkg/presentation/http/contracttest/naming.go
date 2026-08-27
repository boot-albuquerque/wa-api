// Package contracttest holds the assertions every public HTTP payload has to
// satisfy, in one place, so that the six route families migrating to DTOs
// assert the SAME rule instead of six approximations of it.
//
// It is a normal (non-_test) package on purpose: a _test package cannot be
// imported by another package's tests, and these helpers exist precisely to be
// imported from pkg/presentation/http/handlers, pkg/bootstrap, and the dto
// packages.
package contracttest

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// TestingT is the slice of *testing.T these helpers use.
//
// An interface, and not *testing.T, for ONE reason: it lets this package's own
// tests assert that the helpers actually FAIL on a bad payload. A helper that
// silently passes everything is worse than no helper — it hands false
// confidence to the six route families that call it. testing.TB cannot be
// implemented outside the standard library (it has an unexported method), so
// the narrow interface is the only way to get a negative control here.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// canonicalKey is the naming rule for every key on this API's wire:
// lowercase, digits allowed after the first character, words joined by single
// underscores. It rejects `GroupJID`, `groupJid`, `group-jid`, `group__jid`,
// `_group`, and `group_`.
var canonicalKey = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)

// IsCanonicalKey reports whether one key name obeys the rule. Exported so a
// test can assert on a name it built rather than on a whole payload.
func IsCanonicalKey(key string) bool { return canonicalKey.MatchString(key) }

// AssertPublicJSONUsesCanonicalNaming fails t if any object key anywhere in
// body — nested objects and objects inside arrays included — is not
// snake_case lowercase.
//
// It walks the DECODED payload rather than grepping the bytes, because a
// struct tag is not the only way a key reaches the wire: a map[string]any
// built in a handler produces keys no `json:` tag ever saw, and those are
// exactly the ones that have drifted.
func AssertPublicJSONUsesCanonicalNaming(t TestingT, body []byte) {
	t.Helper()

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("resposta não é JSON válido: %v\ncorpo: %s", err, string(body))
	}

	offenders := map[string]string{} // key -> first path where it appeared
	walkJSON("$", payload, func(path, key string) {
		if !IsCanonicalKey(key) {
			if _, seen := offenders[key]; !seen {
				offenders[key] = path
			}
		}
	})
	if len(offenders) == 0 {
		return
	}

	names := make([]string, 0, len(offenders))
	for k := range offenders {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(strconv.Itoa(len(names)))
	b.WriteString(" chave(s) fora do snake_case minúsculo exigido pelo contrato público")
	b.WriteString(" (docs/HTTP-DTO-CONVENTIONS.md):\n")
	for _, n := range names {
		b.WriteString("  ")
		b.WriteString(n)
		b.WriteString("  em  ")
		b.WriteString(offenders[n])
		b.WriteString("\n")
	}
	t.Errorf("%s", b.String())
}

// AssertNoKeys fails t if any of the named keys appears anywhere in body.
//
// It is the companion of the rule above for the hard cutover: after a family
// migrates, the OLD key must be gone, and "the new key is present" does not
// prove that — a struct can carry both.
func AssertNoKeys(t TestingT, body []byte, keys ...string) {
	t.Helper()

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("resposta não é JSON válido: %v\ncorpo: %s", err, string(body))
	}
	banned := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		banned[k] = struct{}{}
	}
	var found []string
	walkJSON("$", payload, func(path, key string) {
		if _, bad := banned[key]; bad {
			found = append(found, key+" em "+path)
		}
	})
	sort.Strings(found)
	if len(found) > 0 {
		t.Errorf("chave(s) que deviam ter desaparecido na migração ainda presentes:\n  %s",
			strings.Join(found, "\n  "))
	}
}

// walkJSON visits every object key in a decoded payload, reporting the JSON
// path it was found at so a failure names WHERE, not just what.
func walkJSON(path string, node any, visit func(path, key string)) {
	switch value := node.(type) {
	case map[string]any:
		// Sorted so the report is identical between runs: Go randomizes map
		// iteration order by design, and a test whose failure text shuffles is
		// a test nobody can diff.
		keys := make([]string, 0, len(value))
		for k := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			visit(path+"."+k, k)
			walkJSON(path+"."+k, value[k], visit)
		}
	case []any:
		for i, child := range value {
			walkJSON(path+"["+strconv.Itoa(i)+"]", child, visit)
		}
	}
}
