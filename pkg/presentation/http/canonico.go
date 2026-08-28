package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// This file used to implement path standardization (F269) WITHOUT breaking
// clients: every old route stayed registered, and the canonical form was
// added alongside it, forever.
//
// REVERT (2026-08-27, see HOUSEKEEP.md): explicit decision by the user — the
// project has no real consumers before launch, so there is no client to
// protect, and keeping both forms registered was paying the compatibility
// cost with nobody using it. CanonicalizeRoutes now RENAMES the route
// instead of adding an alias to it: the old path stops responding (404),
// only the canonical one stays registered. The table and the matching
// mechanism survive because they remain the only way to apply the change to
// ninety-one routes without hand-editing every `Register` call.
//
// WHY THE TABLE AND NOT 91 HAND EDITS. There are ninety-one routes. Editing
// every Register call would be ninety-one chances to typo a character, and
// the mistake would only surface when someone called the wrong route. With
// the table, the transformation is a single one, and the gate compares what
// was registered against what the table mandates.

// CanonicalRoute is one row of the table: the old route and its canonical
// form.
type CanonicalRoute struct {
	LegacyMethod    string
	LegacyPath      string
	CanonicalMethod string
	CanonicalPath   string
}

// CanonicalizeRoutes replaces, for every registered entry that has a
// canonical form in the table, the old registration with the canonical one —
// same handler, new path. The old route stops being in the registry and
// therefore stops responding.
//
// ORDER MATTERS: call this AFTER all the old routes have been registered. A
// table entry with no matching old route is a caller error — and it is
// returned, not swallowed, because a table that points at nonexistent
// routes is a table that no longer describes the service.
func (r *HandlerRegistry) CanonicalizeRoutes(table []CanonicalRoute) []string {
	byKey := map[string]routeEntry{}
	for _, entry := range r.routes {
		for _, method := range entry.methods {
			byKey[strings.ToUpper(method)+" "+entry.path] = entry
		}
	}

	consumed := map[string]bool{}
	var canonicalEntries []routeEntry
	var orphans []string
	for _, row := range table {
		key := strings.ToUpper(row.LegacyMethod) + " " + row.LegacyPath
		entry, exists := byKey[key]
		if !exists {
			orphans = append(orphans, key)
			continue
		}
		consumed[key] = true
		handler := entry.handler
		// When the canonical path carries parameters, the identifier no
		// longer comes in the body. The handler still reads it from the
		// body — so the adapter injects it before passing the request on.
		if strings.Contains(row.CanonicalPath, "{") {
			handler = InjectPathParams(handler)
		}
		canonicalEntries = append(canonicalEntries, routeEntry{
			path:    row.CanonicalPath,
			handler: handler,
			methods: []string{row.CanonicalMethod},
		})
	}

	// Remove from the registry the methods the table consumed, one by one —
	// a single entry can combine several methods in the same Register call
	// (e.g. "/user/privacy" with GET and POST), and only some of them may
	// have a row in the table. An entry with ALL of its methods consumed
	// disappears; with only SOME, it stays registered with whichever
	// methods are left.
	var remaining []routeEntry
	for _, entry := range r.routes {
		var kept []string
		for _, method := range entry.methods {
			key := strings.ToUpper(method) + " " + entry.path
			if !consumed[key] {
				kept = append(kept, method)
			}
		}
		if len(kept) == 0 {
			continue
		}
		if len(kept) != len(entry.methods) {
			entry.methods = kept
		}
		remaining = append(remaining, entry)
	}
	r.routes = append(remaining, canonicalEntries...)
	return orphans
}

// bodyFieldForPathParam says which body field each path parameter must be
// injected into.
//
// The destination names are the ones the handlers ALREADY read — not a new
// normalization. Changing a field name here would change the body contract,
// and the whole point of this layer is exactly the opposite: new path, same
// body.
//
// INTEGRATION (2026-08-27): this table used to point at the OLD names
// (`groupJID`, `ChatPhone`, `PollMessageId`, `Code`) because it was written
// before the group and message families' DTO migrations were integrated
// into this tree. After those migrations, the handlers started reading
// exclusively the canonical names — the table was left pointing at fields
// that no longer existed, and the cut-over routes (worktree http-dto-paths)
// that rely on this injection to fill in group_jid/chat_phone/poll_message_id
// from the path stopped resolving (404→400 missing_*). Fixed to the current
// names.
var bodyFieldForPathParam = map[string]string{
	"group_jid":       "group_jid",
	"community_jid":   "community_jid",
	"chat_jid":        "chat_phone",
	"poll_message_id": "poll_message_id",
	"invite_code":     "code",
}

// InjectPathParams copies the path parameters into the JSON body, and only
// then calls the original handler.
//
// DOES NOT OVERWRITE. If the body already carries the field, the body wins —
// the path is a new way to say the same thing, not an authority over
// whoever already said it. That keeps the canonical route usable by a
// client that still sends the identifier in the body, during the migration.
//
// Exported because it is also used directly by cut-over routes going
// straight to the canonical form (without going through
// CanonicalizeRoutes) — see pkg/bootstrap/wiring_routes.go.
func InjectPathParams(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		if len(vars) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			// Reading the body failed: let the original handler respond
			// with the error, using the message it already gives. Making
			// one up here would be a second way of saying the same
			// failure.
			next.ServeHTTP(w, r)
			return
		}
		_ = r.Body.Close()

		body := map[string]any{}
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				// Unreadable body: put it back as-is, so the rejection
				// comes from the handler's own decoder and carries the
				// usual code.
				r.Body = io.NopCloser(bytes.NewReader(raw))
				next.ServeHTTP(w, r)
				return
			}
		}

		for param, value := range vars {
			field, known := bodyFieldForPathParam[param]
			if !known {
				continue
			}
			if _, alreadyPresent := body[field]; alreadyPresent {
				continue
			}
			body[field] = value
		}

		updated, err := json.Marshal(body)
		if err != nil {
			r.Body = io.NopCloser(bytes.NewReader(raw))
			next.ServeHTTP(w, r)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(updated))
		r.ContentLength = int64(len(updated))
		next.ServeHTTP(w, r)
	})
}
