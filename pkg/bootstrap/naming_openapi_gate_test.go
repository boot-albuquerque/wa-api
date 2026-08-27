package bootstrap

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"wa-api/pkg/presentation/http/contracttest"
)

// This file is the permanent regression gate for JSON naming inside the
// OpenAPI specification itself — the sibling of naming_paths_gate_test.go
// (URL paths) and pkg/presentation/http/handlers/naming_gate_live_test.go
// (the actual HTTP responses). Three surfaces, three gates, because a
// document can promise the right shape while the code serves a different
// one (or vice versa) — see docs/HTTP-DTO-CONVENTIONS.md §8 and §10 for why
// the live gate exists alongside this one.
//
// It walks the embedded, GENERATED specification
// (pkg/presentation/http/apidocs/openapi.yaml, via the same especificacao(t)
// helper openapi_contrato_test.go already uses) rather than the source
// fragments under api/openapi/{base,paths,schemas}: the generated file is
// what a client actually reads, and `go run ./cmd/openapidoc` can drop or
// mangle a fragment on the way to the merged document — checking the
// SOURCE would miss that class of bug entirely.

// deepCheckJSONKeys applies contracttest.IsCanonicalKey to every object key
// found anywhere inside a decoded JSON-shaped value — nested objects and
// objects inside arrays included. It exists here (rather than importing an
// unexported helper from contracttest) because an OpenAPI "example"/
// "examples" value is a literal JSON payload embedded inside a YAML
// document, not a JSON response body from an HTTP call — the two gates walk
// structurally identical trees for different reasons, so duplicating this
// ~15-line walker is cheaper than exporting internals of contracttest just
// to share it.
func deepCheckJSONKeys(path string, node any, record func(path, key string)) {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			record(path+"."+k, k)
			deepCheckJSONKeys(path+"."+k, child, record)
		}
	case []any:
		for i, child := range v {
			deepCheckJSONKeys(fmt.Sprintf("%s[%d]", path, i), child, record)
		}
	}
}

// walkOpenAPIDoc recurses the whole decoded specification once, invoking
// the three callbacks wherever the corresponding OpenAPI construct appears
// — regardless of whether it's under components.schemas, an inline schema
// on a request/response body, a parameter, or an example. That "regardless
// of where" is the point: a schema inlined directly on an operation instead
// of under components.schemas is legal OpenAPI, and a gate that only looked
// under components.schemas would miss it.
func walkOpenAPIDoc(
	path string,
	node any,
	onSchemaProperties func(path string, props map[string]any),
	onEnum func(path string, values []any),
	onExamplePayload func(path string, payload any),
	onParameterName func(path string, name string),
) {
	switch v := node.(type) {
	case map[string]any:
		if props, ok := v["properties"].(map[string]any); ok {
			onSchemaProperties(path+".properties", props)
		}
		if enumVals, ok := v["enum"].([]any); ok {
			onEnum(path+".enum", enumVals)
		}
		if example, ok := v["example"]; ok {
			onExamplePayload(path+".example", example)
		}
		if examples, ok := v["examples"].(map[string]any); ok {
			for name, ex := range examples {
				exMap, ok := ex.(map[string]any)
				if !ok {
					continue
				}
				if val, ok := exMap["value"]; ok {
					onExamplePayload(path+".examples."+name+".value", val)
				}
			}
		}
		// A Parameter Object (and, structurally identically, a Security
		// Scheme Object) carries "name" alongside "in". Header names
		// (Authorization, token via securitySchemes) follow HTTP header
		// convention, not the JSON-key alphabet, so they're excluded —
		// path and query parameter names are not.
		if in, ok := v["in"].(string); ok && in != "header" {
			if name, ok := v["name"].(string); ok {
				onParameterName(path+".name", name)
			}
		}
		for k, child := range v {
			walkOpenAPIDoc(path+"."+k, child, onSchemaProperties, onEnum, onExamplePayload, onParameterName)
		}
	case []any:
		for i, child := range v {
			walkOpenAPIDoc(fmt.Sprintf("%s[%d]", path, i), child, onSchemaProperties, onEnum, onExamplePayload, onParameterName)
		}
	}
}

// TestOpenAPISchemaPropertyNamesAreCanonical asserts every schema property
// name in the generated specification — components.schemas, and any schema
// inlined on a request body, response, or parameter — matches the same
// snake_case rule the live JSON gate enforces
// (docs/HTTP-DTO-CONVENTIONS.md §8). Recursive: a property whose own schema
// has nested "properties", or is an array of objects with "properties", is
// still reached, because walkOpenAPIDoc recurses into every map and array
// unconditionally.
func TestOpenAPISchemaPropertyNamesAreCanonical(t *testing.T) {
	doc := especificacao(t)

	var offenders []string
	walkOpenAPIDoc("$", doc,
		func(path string, props map[string]any) {
			for name := range props {
				if !contracttest.IsCanonicalKey(name) {
					offenders = append(offenders, name+"  em  "+path)
				}
			}
		},
		func(string, []any) {},
		func(string, any) {},
		func(string, string) {},
	)
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("%d propriedade(s) de esquema fora do snake_case minúsculo exigido "+
			"(docs/HTTP-DTO-CONVENTIONS.md §8):\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// nonCodeEnumValue reports whether an enum member is NOT the kind of value
// §8's rule targets — a machine-readable code the server writes to the wire
// (admin_add, invalid_request, ...) — so checking it against
// contracttest.IsCanonicalKey would be a false positive rather than a
// finding.
//
// Measured against the real spec on 2026-08-27, four shapes show up that
// are legitimately not codes, and none is a case the rule was ever meant to
// cover:
//
//   - a free-text human message ("User blocked", "contact roster sync
//     requested" — ResultadoBloqueio.Details, ResultadoSincronizacaoContactos.details):
//     contains a space, which no identifier ever does;
//   - a redaction placeholder ("***" — the enum on hmac_key/access_key
//     response fields, documenting "this comes back masked", not a value
//     the field actually takes): the literal masking sentinel, not a code;
//   - a duration literal ("0", "24h", "7d", "90d" — the disappearing-timer
//     enums): starts with a digit, which an identifier never does either;
//   - the empty string, DOCUMENTED as its own enum member meaning "unset" —
//     e.g. api/openapi/schemas/infra.yaml:240
//     (enum: [empty string, base64, s3, both], media_delivery, "Como a mídia
//     recebida é entregue") and grupo.yaml:433
//     (enum: [admin_add, all_member_add, empty string], MemberAddMode). It is a
//     real, intentional sentinel repeated across five schemas, not an
//     omission.
//
// A value that is none of these four either IS meant to be a code (and gets
// checked) or is a NEW shape nobody has seen yet — in which case this
// function's false-positive list is the place to extend, with the same
// evidence standard as the entries above: cite the schema and field, and
// say why it isn't a code.
func nonCodeEnumValue(s string) bool {
	if s == "" || s == "***" {
		return true
	}
	if strings.ContainsAny(s, " \t") {
		return true
	}
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		return true
	}
	return false
}

// TestOpenAPIEnumValuesAreCanonical covers the second half of §8's rule:
// "aplica-se também aos valores enumerados que o servidor escreve" — an
// enum member is a value the server actually writes to the wire
// (error.code, a status string, ...), so it has to obey the same alphabet a
// JSON key does, not just look like one. nonCodeEnumValue carves out the
// enum members that are documented text or data, not codes — see its
// comment for the measured evidence.
func TestOpenAPIEnumValuesAreCanonical(t *testing.T) {
	doc := especificacao(t)

	var offenders []string
	walkOpenAPIDoc("$", doc,
		func(string, map[string]any) {},
		func(path string, values []any) {
			for _, v := range values {
				s, ok := v.(string)
				if !ok || nonCodeEnumValue(s) {
					continue
				}
				if !contracttest.IsCanonicalKey(s) {
					offenders = append(offenders, s+"  em  "+path)
				}
			}
		},
		func(string, any) {},
		func(string, string) {},
	)
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("%d valor(es) enumerado(s) fora do snake_case minúsculo exigido "+
			"(docs/HTTP-DTO-CONVENTIONS.md §8):\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestOpenAPIExampleKeysAreCanonical walks every literal example payload
// (both the singular "example" field and the OpenAPI Examples Object form)
// and checks every object key inside it, recursively — an example is meant
// to show a client exactly what the wire looks like, so a stale key here is
// either a lie about the contract or a preview of a regression.
// dynamicExampleKey reconhece uma chave de MAPA DINÂMICO usada como
// identificador — telefone/JID (contém "@") ou hash hexadecimal de 32
// caracteres — dentro de um exemplo. Estas chaves são DADOS, não nomes
// estruturais: "5516981818244@s.whatsapp.net" não é um campo que a nossa API
// escolheu chamar assim, é o valor real que o WhatsApp devolve como chave de
// um mapa (ex.: users/contacts, session/profile/full.user_info,
// IndiceDeConversas). A regra de nomes canónicos (item #26 da especificação:
// "Maps/dynamic objects... não altere valores dinâmicos arbitrariamente")
// aplica-se aos campos ESTRUTURAIS ao lado dela, não à chave em si.
var dynamicExampleKey = regexp.MustCompile(`^([0-9]+@[a-z.]+|[0-9a-f]{32})$`)

func TestOpenAPIExampleKeysAreCanonical(t *testing.T) {
	doc := especificacao(t)

	var offenders []string
	record := func(path, key string) {
		if dynamicExampleKey.MatchString(key) {
			return
		}
		if !contracttest.IsCanonicalKey(key) {
			offenders = append(offenders, key+"  em  "+path)
		}
	}
	walkOpenAPIDoc("$", doc,
		func(string, map[string]any) {},
		func(string, []any) {},
		func(path string, payload any) { deepCheckJSONKeys(path, payload, record) },
		func(string, string) {},
	)
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("%d chave(s) de exemplo fora do snake_case minúsculo exigido "+
			"(docs/HTTP-DTO-CONVENTIONS.md §8):\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestOpenAPIParameterNamesAreCanonical checks the "name" of every
// path/query parameter (Parameter Objects with in != "header") against the
// same rule. Path parameters are ALSO checked by
// TestPathParamsAreSnakeCase in naming_paths_gate_test.go, against the
// route table instead of the spec — this test catches the spec drifting
// from the route table (a parameter renamed in one but not the other), not
// just a bad name in isolation.
func TestOpenAPIParameterNamesAreCanonical(t *testing.T) {
	doc := especificacao(t)

	var offenders []string
	walkOpenAPIDoc("$", doc,
		func(string, map[string]any) {},
		func(string, []any) {},
		func(string, any) {},
		func(path string, name string) {
			if !contracttest.IsCanonicalKey(name) {
				offenders = append(offenders, name+"  em  "+path)
			}
		},
	)
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("%d nome(s) de parâmetro fora do snake_case minúsculo exigido "+
			"(docs/HTTP-DTO-CONVENTIONS.md §8):\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
