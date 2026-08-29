// Package status reads WhatsApp Status — the twenty-four-hour stories.
//
// THE UPSTREAM CALLS THIS "BROADCAST" AND IT IS NOT A BROADCAST LIST. Our own
// ledger described these three rows as "família de listas de transmissão", and
// three rows were about to be built against the wrong idea. Reading the
// reference settles it: Client.getBroadcasts is getAllStatuses, which is
// WAWebCollections.Status.getModelsArray, and the Broadcast structure carries
// msgs, totalCount and unreadCount keyed by a CONTACT id. That is a person's
// story feed (H100).
//
// THIS PACKAGE ONLY READS, and the omission is deliberate rather than
// unfinished. This build exports WAWebSendStatusMsgAction with
// sendStatusTextMsgAction and sendStatusMediaMsgAction — it can POST a status,
// which the upstream surface has no equivalent for at all. It is not wired here.
//
// A status is visible to EVERY CONTACT IN THE ADDRESS BOOK, and this account's
// roster was measured at 944. Every outward effect this module has ever produced
// landed on one known peer or one lab group; posting a status would land on
// nine hundred people who never agreed to be part of a test. That is not a
// scope decision, it is a blast radius, and it belongs to a human.
package status

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

var (
	// ErrRead is the page refusing or failing the read.
	ErrRead = fmt.Errorf("status: the page refused the status read")
	// ErrNoContact is an empty contact id.
	ErrNoContact = fmt.Errorf("status: no contact id given")
	// ErrNotFound is a contact with no status feed in this session.
	//
	// IT IS NOT "THAT PERSON HAS NO STATUS". The collection holds what this
	// session knows, and a feed nobody has looked at is absent rather than
	// empty — the same distinction poll.ErrNotFound draws for messages.
	ErrNotFound = fmt.Errorf("status: this session holds no status feed for that contact")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 250 * time.Millisecond
)

const stateKeyPrefix = "__headlessStatus"

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other and each polled until
// non-empty, so one could take the other's answer. The nonce comes from Go: a
// page-side Math.random or Date.now would put a decision and a clock where
// invariant 6 forbids them.
var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Feed is one contact's status feed.
//
// IT CARRIES COUNTS AND TIMES, NEVER CONTENT. A status is a photo, a video or a
// caption somebody chose to publish for a day; none of that crosses this
// boundary. The contact id is here because a caller needs to route on it.
type Feed struct {
	// ContactJID is whose feed this is.
	ContactJID string
	// Total is how many status messages the feed holds.
	Total int
	// Unread is how many this account has not seen.
	Unread int
	// Read is how many it has.
	Read int
	// At is the feed's own timestamp — the most recent post, as the page
	// reports it. Zero when absent.
	At time.Time
	// Loading says the page is still filling this feed, so the counts above are
	// a snapshot of an incomplete thing. Reported rather than waited out,
	// because the caller's budget is the caller's.
	Loading bool
}

func (f Feed) String() string {
	return fmt.Sprintf("status.Feed(contact=%t total=%d unread=%d read=%d at=%t loading=%t)",
		f.ContactJID != "", f.Total, f.Unread, f.Read, !f.At.IsZero(), f.Loading)
}

// Reader reads status feeds.
type Reader struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Reader {
	return &Reader{runner: runner, eval: eval}
}

// List reports every status feed this session holds.
//
// AN EMPTY LIST IS A VALID ANSWER and does not mean the account has no contacts
// posting: it means this session knows of none. Statuses arrive with the
// account's own sync, and a session that has just booted may legitimately hold
// nothing.
func (r *Reader) List(ctx context.Context, label string) ([]Feed, error) {
	key := nextStateKey()
	raw, err := r.parked(ctx, listScript(key), key, label+"/list")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK    bool       `json:"ok"`
		Why   string     `json:"why"`
		Feeds []wireFeed `json:"feeds"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return nil, fmt.Errorf("status: unexpected list answer: %w", e)
	}
	if !out.OK {
		return nil, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	feeds := make([]Feed, 0, len(out.Feeds))
	for _, f := range out.Feeds {
		feeds = append(feeds, f.feed())
	}
	return feeds, nil
}

// ByContact reports one contact's feed.
func (r *Reader) ByContact(ctx context.Context, contactJID, label string) (Feed, error) {
	if strings.TrimSpace(contactJID) == "" {
		return Feed{}, ErrNoContact
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, byContactScript(contactJID, key), key, label+"/by-contact")
	if err != nil {
		return Feed{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK    bool      `json:"ok"`
		Why   string    `json:"why"`
		Found bool      `json:"found"`
		Feed  *wireFeed `json:"feed"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Feed{}, fmt.Errorf("status: unexpected read answer: %w", e)
	}
	if !out.OK {
		return Feed{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	if !out.Found || out.Feed == nil {
		return Feed{}, ErrNotFound
	}
	return out.Feed.feed(), nil
}

type wireFeed struct {
	ID      string `json:"id"`
	Total   int    `json:"total"`
	Unread  int    `json:"unread"`
	Read    int    `json:"read"`
	T       int64  `json:"t"`
	Loading bool   `json:"loading"`
}

func (w wireFeed) feed() Feed {
	f := Feed{ContactJID: w.ID, Total: w.Total, Unread: w.Unread,
		Read: w.Read, Loading: w.Loading}
	if w.T > 0 {
		f.At = time.Unix(w.T, 0)
	}
	return f
}

func (r *Reader) parked(ctx context.Context, kick, key, label string) (string, error) {
	var started string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(c context.Context) error {
		return r.eval(c, kick, &started)
	}); err != nil {
		return "", err
	}
	deadline := time.Now().Add(Budget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/read", func(c context.Context) error {
			return r.eval(c, `window.`+key+` || ""`, &raw)
		}); err != nil {
			return "", err
		}
		if raw != "" {
			// A CHAVE E' LIBERADA ao ser lida (H177).
			var ignored string
			_ = r.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return r.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
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
