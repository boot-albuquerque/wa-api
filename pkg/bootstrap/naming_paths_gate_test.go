package bootstrap

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This file is the permanent regression gate for URL path naming — part of
// the naming gate built alongside the six-family HTTP DTO migration
// (docs/HTTP-DTO-CONVENTIONS.md). It walks every route bootstrap.Routes()
// enumerates — the SAME table pkg/bootstrap/router.go builds for production
// and cmd/listroutes prints — and asserts each path segment against the
// naming rule.
//
// A plain regex cannot tell "requestparticipants" (two words glued
// together, WRONG) from "communities" (one real word, RIGHT): both are
// lowercase runs with no separator for the regex to see. So this gate
// carries two curated word lists alongside the regex, and neither an
// addition nor a removal from either list is free — see the comments on
// each map below.

// pathSegmentNamingPattern accepts a plain lowercase word or a kebab-case
// compound: digits allowed after the first letter of each hyphen-joined
// piece. Rejects PascalCase, camelCase, snake_case, and doubled/leading/
// trailing separators.
var pathSegmentNamingPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// pathParamNamingPattern is the naming rule for a {braced} path parameter:
// snake_case, the same alphabet a JSON key uses
// (docs/HTTP-DTO-CONVENTIONS.md §8).
var pathParamNamingPattern = regexp.MustCompile(`^\{[a-z][a-z0-9]*(?:_[a-z0-9]+)*\}$`)

// suspiciousSegmentLength is the point past which a lowercase-only segment
// (no hyphen, so pathSegmentNamingPattern cannot see word boundaries) has to be
// resolved by a human into one of the two lists below instead of silently
// passing.
//
// Chosen empirically: measured with `go run ./cmd/listroutes` on
// 2026-08-27, every KNOWN concatenated compound in this API's route table
// is 8 characters or longer, and the shortest legitimate single word this
// threshold catches (contacts, carousel, ...) still reads fine as one line
// in the allowlist below.
const suspiciousSegmentLength = 8

// knownBadConcatenatedSegments are path segments that PASS
// pathSegmentNamingPattern (they're a lowercase run with no separator) but are
// actually two or more words glued together with no hyphen. Present so a
// regex-only gate can't be fooled by them, and so a route that reintroduces
// one after a rename fails loud instead of sliding back in unnoticed.
//
// The value is the fix, so a failure output tells you where to look instead
// of just what's wrong.
//
// Measured against `go run ./cmd/listroutes` on 2026-08-27. The five
// download* entries were NOT in the sibling worktrees' F296-F301 findings —
// found independently while building this gate: /chat/downloadimage,
// /chat/downloadvideo, /chat/downloadaudio, /chat/downloaddocument,
// /chat/downloadsticker (and their /chats/... registry-canonical siblings,
// see pkg/bootstrap/wiring_routes.go) all glue "download" straight onto the
// media kind. Cross-referenced in HOUSEKEEP.md.
//
// profilepicture and requesthistorysync are not registered by any route
// today — this worktree's equivalents (/user/profile/{jid},
// /user/history/sync) already split correctly with a slash. Kept here per
// the naming-gate task spec so a FUTURE route that reintroduces either
// compound fails immediately instead of waiting for someone to notice.
var knownBadConcatenatedSegments = map[string]string{
	"inviteinfo":                "invite-info (or its own sub-resource)",
	"invitelink":                "invite-link",
	"joinapprovalmode":          "already fixed on the canonical path: /groups/{group_jid}/settings/join-approval",
	"requestparticipants":       "already fixed on the canonical path: /groups/{group_jid}/join-requests",
	"updaterequestparticipants": "already fixed on the canonical path: /groups/{group_jid}/join-requests",
	"updateparticipants":        "already fixed on the canonical path: /groups/{group_jid}/participants",
	"pollvote":                  "poll-vote",
	"markread":                  "mark-read",
	"pairphone":                 "already fixed on the canonical path: /session/pair/phone",
	"profilepicture":            "profile-picture (no current route; guards a future one)",
	"requesthistorysync":        "request-history-sync (no current route; guards a future one)",
	"downloadaudio":             "download/audio, or the {kind} path param /chats/download/{kind} already uses",
	"downloaddocument":          "download/document",
	"downloadimage":             "download/image",
	"downloadsticker":           "download/sticker",
	"downloadvideo":             "download/video",
}

// allowlistedSingleWordSegments are lowercase runs of
// suspiciousSegmentLength characters or more that ARE one real word (or an
// established product noun of this API), confirmed by hand so this gate
// does not have to re-litigate them on every run.
//
// Adding a word here is a claim: "I checked, this is not two words glued
// together." Get it wrong and TestPathSegmentsAreNotConcatenatedCompounds
// stops meaning anything — so justify the addition in the commit/PR, not
// just here.
var allowlistedSingleWordSegments = map[string]bool{
	"announce":     true,
	"blocklist":    true,
	"capabilities": true,
	"carousel":     true,
	"communities":  true,
	"community":    true,
	"configure":    true,
	"contacts":     true,
	"disconnect":   true,
	"document":     true,
	"download":     true,
	"ephemeral":    true,
	"location":     true,
	"messages":     true,
	"newsletter":   true,
	"newsletters":  true,
	"participants": true,
	"presence":     true,
	"settings":     true,
	"subgroups":    true,
	"subscribe":    true,
	"template":     true,
	"unfollow":     true,
}

// pathsFromRoutes is the shared enumeration every test in this file walks:
// every distinct registered path, deduplicated (several methods can share a
// path) and sorted so a failure list reads the same on every run.
func pathsFromRoutes(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	var out []string
	for _, r := range Routes(Deps{}) {
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		out = append(out, r.Path)
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatal("bootstrap.Routes(Deps{}) returned zero paths; this gate would pass vacuously")
	}
	return out
}

// TestPathsAreLowercaseKebabCase is the base rule: every non-parameter
// segment of every registered path is lowercase, and if it has more than
// one word, the words are hyphen-joined. Rejects PascalCase (/Group/Info),
// camelCase (/group/groupInfo), snake_case (/group/group_info), and
// doubled/stray hyphens.
func TestPathsAreLowercaseKebabCase(t *testing.T) {
	var offenders []string
	for _, path := range pathsFromRoutes(t) {
		for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
			if seg == "" {
				continue
			}
			if strings.HasPrefix(seg, "{") {
				continue // path parameters: checked by TestPathParamsAreSnakeCase
			}
			if !pathSegmentNamingPattern.MatchString(seg) {
				offenders = append(offenders, seg+"  em  "+path)
			}
		}
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("%d segmento(s) de caminho fora de lowercase/kebab-case:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestPathParamsAreSnakeCase asserts every {braced} path parameter is
// snake_case — {group_jid}, not {groupId} or {groupJid}. Path params share
// the JSON-key alphabet on purpose: a client that already knows the
// canonical key name for a resource's identifier should never have to learn
// a second spelling for the same identifier in the URL.
func TestPathParamsAreSnakeCase(t *testing.T) {
	var offenders []string
	for _, path := range pathsFromRoutes(t) {
		for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
			if !strings.HasPrefix(seg, "{") {
				continue
			}
			if !pathParamNamingPattern.MatchString(seg) {
				offenders = append(offenders, seg+"  em  "+path)
			}
		}
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("%d parametro(s) de caminho fora de snake_case:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestPathSegmentsAreNotConcatenatedCompounds is the half of the gate a
// regex alone cannot do: it separates "one real word" from "two words with
// no separator between them" using the two curated lists above, instead of
// a heuristic that would guess wrong on real API nouns like "communities".
//
// Two failure modes, both reported:
//
//  1. a segment on the known-bad list is still present in the route table —
//     the naming-gate task spec's item #44 denylist, plus five downloadX
//     compounds found independently (see the map's comment);
//  2. a NEW long lowercase-only segment shows up that is neither on the bad
//     list nor the allowlist — someone has to look at it and decide, once,
//     which list it belongs on. This is what stops the gate from going
//     blind the next time a family adds a route.
func TestPathSegmentsAreNotConcatenatedCompounds(t *testing.T) {
	var bad []string
	var unresolved []string
	for _, path := range pathsFromRoutes(t) {
		for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
			if seg == "" || strings.HasPrefix(seg, "{") || strings.Contains(seg, "-") {
				continue // hyphenated segments already show their word boundaries
			}
			if fix, isBad := knownBadConcatenatedSegments[seg]; isBad {
				bad = append(bad, seg+"  em  "+path+"  -> "+fix)
				continue
			}
			if len(seg) >= suspiciousSegmentLength && !allowlistedSingleWordSegments[seg] {
				unresolved = append(unresolved, seg+"  em  "+path)
			}
		}
	}
	sort.Strings(bad)
	sort.Strings(unresolved)
	if len(bad) > 0 {
		t.Errorf("%d segmento(s) de caminho são compostos concatenados conhecidos:\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
	if len(unresolved) > 0 {
		t.Errorf("%d segmento(s) de caminho novo(s), com %d+ caracteres, não estão em nenhuma "+
			"das duas listas (knownBadConcatenatedSegments / allowlistedSingleWordSegments) — "+
			"resolva manualmente em pkg/bootstrap/naming_paths_gate_test.go antes de prosseguir:\n  %s",
			len(unresolved), suspiciousSegmentLength, strings.Join(unresolved, "\n  "))
	}
}
