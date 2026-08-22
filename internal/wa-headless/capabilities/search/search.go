// Package search finds messages by text and reports WHERE they are, never what
// they say.
//
// THE RESULT CARRIES NO BODY. Searching is the one capability whose whole point
// is content, which makes it the one most likely to smuggle a body into Go — and
// invariant 12 is structural here for the same reason it is in
// capabilities/messagemeta: a hit is an ADDRESS (which conversation, which
// message, when, which direction), and a caller that wants more asks the
// capability that owns that.
//
// Two measurements shaped this (H114, 2026-08-22, against 395 loaded messages):
//
//  1. A SHORT TERM FINDS NOTHING. Searching "a" returned zero hits over a store
//     full of the letter; a four-letter word taken from a real message returned
//     twenty. The page has a minimum this package does not know, so it reports an
//     empty result as empty rather than pretending a short query is invalid.
//
//  2. SCOPING TO A CHAT DOES NOT WORK. The reference exposes options.chatId and
//     passes it as the fourth argument; doing exactly that returned 0 hits with
//     eof set, while the unscoped form returned 20. So there is no chat scope
//     here — an option that measurably returns nothing is worse than an absent
//     one, because a caller would read the empty answer as "no matches".
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

var (
	// ErrNoQuery is an empty search term.
	ErrNoQuery = fmt.Errorf("search: no query given")
	// ErrRead is the page refusing or failing.
	ErrRead = fmt.Errorf("search: the page refused the search")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 250 * time.Millisecond
)

const stateKey = "__waHeadlessSearch"

// Hit is where a matching message lives. No body, ever.
type Hit struct {
	// ChatJID is the conversation it is in.
	ChatJID string
	// MessageID is the raw id, which is what every other capability in this
	// module reports and therefore the only one a caller can act on.
	MessageID string
	// FromMe says this account sent it.
	FromMe bool
	// Type is the page's own message type, carried verbatim.
	Type string
	// At is the message's timestamp. Zero when absent.
	At time.Time
}

func (h Hit) String() string {
	return fmt.Sprintf("search.Hit(chat=%t id=%t fromMe=%t type=%q at=%t)",
		h.ChatJID != "", h.MessageID != "", h.FromMe, h.Type, !h.At.IsZero())
}

// Result is one page of hits.
type Result struct {
	Hits []Hit
	// EOF is the page's own end-of-results flag, carried rather than inferred
	// from the list being short.
	EOF bool
	// Returned is how many the page gave back, which is NOT necessarily how many
	// were asked for: every measured call answered 20 regardless of the count
	// requested.
	Returned int
}

// Searcher searches.
type Searcher struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Searcher {
	return &Searcher{runner: runner, eval: eval}
}

// Messages finds messages whose text matches the query.
//
// The query is caller-supplied content and is never logged by this package.
func (s *Searcher) Messages(ctx context.Context, query string, page int, label string) (Result, error) {
	if strings.TrimSpace(query) == "" {
		return Result{}, ErrNoQuery
	}
	if page < 1 {
		page = 1
	}
	raw, err := s.parked(ctx, searchScript(query, page), label+"/search")
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK   bool   `json:"ok"`
		Why  string `json:"why"`
		EOF  bool   `json:"eof"`
		Hits []struct {
			Chat   string `json:"chat"`
			ID     string `json:"id"`
			FromMe bool   `json:"fromMe"`
			Type   string `json:"type"`
			T      int64  `json:"t"`
		} `json:"hits"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Result{}, fmt.Errorf("search: unexpected answer: %w", e)
	}
	if !out.OK {
		return Result{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	// NO MATCHES IS NOT AN ERROR. A term nobody used returns nothing, and so does
	// a term the page considers too short — measured, and indistinguishable from
	// here.
	res := Result{Hits: make([]Hit, 0, len(out.Hits)), EOF: out.EOF, Returned: len(out.Hits)}
	for _, h := range out.Hits {
		hit := Hit{ChatJID: h.Chat, MessageID: h.ID, FromMe: h.FromMe, Type: h.Type}
		if h.T > 0 {
			hit.At = time.Unix(h.T, 0)
		}
		res.Hits = append(res.Hits, hit)
	}
	return res, nil
}

func (s *Searcher) parked(ctx context.Context, kick, label string) (string, error) {
	var started string
	if err := s.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return s.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(Budget)
	for {
		var raw string
		if err := s.runner.Do(ctx, engine.OpStateProbe, label+"/poll", func(c context.Context) error {
			return s.eval(c, `window.`+stateKey+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			return raw, nil
		}
		if !time.Now().Before(deadline) {
			return "", fmt.Errorf("the page never settled within %s", Budget)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(Tick):
		}
	}
}
