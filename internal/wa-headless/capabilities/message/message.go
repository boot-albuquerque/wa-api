// Package message resolves a message to the things around it: the conversation
// it belongs to, and the person who sent it.
//
// THEY LOOK TRIVIAL AND ARE NOT. In the reference, Message.getChat and
// Message.getContact are one-liners that hand an id to the client. Here the id
// is the problem: this build files under LID, a message's own `_serialized` is
// null (which is why the poll family had to skip a conversion), and the sender
// of a group message is NOT the chat it arrived in — conflating the two is how a
// group message gets attributed to the group instead of to a person.
//
// MENTIONS TOOK TWO PASSES, and the first one was right to refuse. getMentions
// was measured before it was written: 395 loaded messages, ZERO carrying a
// mention under any of five candidate field names, so shipping a reader never
// seen returning anything would have been the trap catalogued in H93, and it was
// left alone with the measurement written down (H106).
//
// What unblocked it was not a better guess at the field name. It was noticing
// the zero was about the DATA, not about the page: nobody in this account had
// ever mentioned anybody. Producing one mention in the lab group made both
// fields appear at once (H142) — the same manoeuvre that closed the quote, the
// vote and the group events.
package message

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

var (
	// ErrNoMessage is an empty message id.
	ErrNoMessage = fmt.Errorf("message: no message id given")
	// ErrNotFound is a message this session has not loaded.
	//
	// IT IS NOT "THAT MESSAGE DOES NOT EXIST". MsgCollection holds what the
	// session has looked at; loading is the caller's job, the same contract
	// capabilities/poll draws.
	ErrNotFound = fmt.Errorf("message: this session has not loaded that message")
	// ErrRead is the page refusing or failing.
	ErrRead = fmt.Errorf("message: the page refused the read")
)

// Budgets. Var so a test can compress them.
var (
	Budget = 30 * time.Second
	Tick   = 250 * time.Millisecond
)

// stateKeyPrefix names the page global each read parks its answer on.
//
// IT IS A PREFIX, NOT A KEY, AND THAT IS A FIX (H177). Every reader here used
// ONE global. Two concurrent calls on the same session therefore wrote the same
// variable and each polled it until non-empty, so whichever polled first could
// take the OTHER call's answer — measured at 12 crossings in 12 concurrent
// rounds, every round, with a well-formed wrong answer that nothing detected.
//
// The nonce comes from Go, not from the page: a page-side Math.random or
// Date.now would put a decision — and a clock — where invariant 6 forbids them.
const stateKeyPrefix = "__waHeadlessMessage"

// nextStateKey hands out a key nobody else is using.
//
// The counter is monotonic per process, which is enough: the key only has to be
// unique among the reads ALIVE at one moment on one page, and it is cleared as
// soon as its answer is taken.
var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Origin is where a message came from: which conversation, and from whom.
type Origin struct {
	// ChatJID is the conversation. For a group message this is the GROUP.
	ChatJID string
	// IsGroup says so explicitly, because a caller that infers it from the jid
	// suffix is one build change away from being wrong.
	IsGroup bool
	// SenderJID is the person. For a group message it is the participant, NOT
	// the chat — and that distinction is the whole reason this type has two
	// fields instead of one.
	//
	// Empty when the message is this account's own outgoing message, where the
	// page does not name a participant.
	SenderJID string
	// FromMe says this account sent it.
	FromMe bool
	// SenderIsChat records that the two jids are the same, which is the normal
	// case for a one-to-one conversation and a red flag for a group one.
	SenderIsChat bool
	// At is the message's own timestamp. Zero when absent.
	At time.Time
}

func (o Origin) String() string {
	return fmt.Sprintf("message.Origin(chat=%t group=%t sender=%t fromMe=%t senderIsChat=%t at=%t)",
		o.ChatJID != "", o.IsGroup, o.SenderJID != "", o.FromMe, o.SenderIsChat, !o.At.IsZero())
}

// Reader resolves messages.
type Reader struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds one.
func New(runner *engine.Runner, eval spa.Evaluator) *Reader {
	return &Reader{runner: runner, eval: eval}
}

// OriginOf reports the chat and the sender of one message.
//
// IT ANSWERS BOTH AT ONCE ON PURPOSE. The reference has getChat and getContact
// as separate calls, and a caller wanting both pays two page round trips for one
// lookup — and worse, may get them from two different moments. They come from
// the same message model in the same read.
func (r *Reader) OriginOf(ctx context.Context, messageID, label string) (Origin, error) {
	if strings.TrimSpace(messageID) == "" {
		return Origin{}, ErrNoMessage
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, originScript(messageID, key), key, label+"/origin")
	if err != nil {
		return Origin{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		NotFound bool   `json:"notFound"`
		Chat     string `json:"chat"`
		Group    bool   `json:"group"`
		Sender   string `json:"sender"`
		FromMe   bool   `json:"fromMe"`
		T        int64  `json:"t"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Origin{}, fmt.Errorf("message: unexpected answer: %w", e)
	}
	if out.NotFound {
		return Origin{}, ErrNotFound
	}
	if !out.OK {
		return Origin{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	o := Origin{
		ChatJID: out.Chat, IsGroup: out.Group, SenderJID: out.Sender,
		FromMe: out.FromMe, SenderIsChat: out.Sender != "" && out.Sender == out.Chat,
	}
	if out.T > 0 {
		o.At = time.Unix(out.T, 0)
	}
	return o, nil
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
			// A CHAVE E' LIBERADA ASSIM QUE A RESPOSTA E' TOMADA. Sem isto, uma
			// sessao longa acumula um global por leitura — o vazamento que a
			// propria correcao criaria.
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

// ShapeOf reports the FIELD NAMES the page has on one message, and never a
// value. See shapeScript for why this is a deliberate divergence from the
// reference's Message.rawData rather than a thin version of it.
//
// Sorted, so two reads of the same message compare.
func (r *Reader) ShapeOf(ctx context.Context, messageID, label string) ([]string, error) {
	if strings.TrimSpace(messageID) == "" {
		return nil, ErrNoMessage
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, shapeScript(messageID, key), key, label+"/shape")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK       bool     `json:"ok"`
		Why      string   `json:"why"`
		NotFound bool     `json:"notFound"`
		Keys     []string `json:"keys"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return nil, fmt.Errorf("message: unexpected answer: %w", e)
	}
	if out.NotFound {
		return nil, ErrNotFound
	}
	if !out.OK {
		return nil, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	return out.Keys, nil
}

// Current is the part of a message that changes after it exists.
type Current struct {
	// Ack is the delivery state the page currently reports. Meaningless unless
	// HasAck.
	Ack int
	// HasAck separates "the page says ack 0" from "the page says nothing".
	//
	// NOT COSMETIC: of 395 loaded messages, 25 read ack 0 and 5 had no ack field
	// at all (H108). Merging them would report "never left" about a message the
	// page never spoke about.
	HasAck bool
	// Starred is the current star state.
	Starred bool
	// Type is the page's own type string, raw. A revoked message shows up here
	// as the page names it; this package does not own that vocabulary.
	Type string
}

func (c Current) String() string {
	return fmt.Sprintf("message.Current(hasAck=%t ack=%d starred=%t type=%q)",
		c.HasAck, c.Ack, c.Starred, c.Type)
}

// CurrentOf re-reads one message and reports what may have changed since the
// caller last looked. It is this module's Message.reload.
//
// A message that is gone reports ErrNotFound, which is the same answer OriginOf
// gives and the same one the reference expresses by returning null.
func (r *Reader) CurrentOf(ctx context.Context, messageID, label string) (Current, error) {
	if strings.TrimSpace(messageID) == "" {
		return Current{}, ErrNoMessage
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, currentScript(messageID, key), key, label+"/current")
	if err != nil {
		return Current{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		NotFound bool   `json:"notFound"`
		HasAck   bool   `json:"hasAck"`
		Ack      int    `json:"ack"`
		Starred  bool   `json:"starred"`
		Type     string `json:"type"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Current{}, fmt.Errorf("message: unexpected answer: %w", e)
	}
	if out.NotFound {
		return Current{}, ErrNotFound
	}
	if !out.OK {
		return Current{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	return Current{HasAck: out.HasAck, Ack: out.Ack, Starred: out.Starred, Type: out.Type}, nil
}

// Quoted is the message a message replies to.
type Quoted struct {
	// Quotes says this message references another at all.
	Quotes bool
	// MessageID is the quoted message's raw id, empty when Quotes is false.
	MessageID string
	// Loaded says the quoted message is IN this session.
	//
	// IT IS A SEPARATE FACT FROM Quotes, and merging them would say "no quote"
	// about a reply whose target simply has not hydrated — which is a lie about
	// the message rather than about the session.
	Loaded bool
	// ChatJID and SenderJID are where the quoted message lived. Empty when the
	// page does not carry them, which it does not for a quote inside the same
	// one-to-one conversation.
	ChatJID   string
	SenderJID string
}

func (q Quoted) String() string {
	return fmt.Sprintf("message.Quoted(quotes=%t id=%t loaded=%t chat=%t sender=%t)",
		q.Quotes, q.MessageID != "", q.Loaded, q.ChatJID != "", q.SenderJID != "")
}

// QuotedOf reports which message a message quotes.
//
// It is the reference's Message.getQuotedMessage, narrowed honestly: it answers
// WHICH message is quoted and whether this session holds it, and does not return
// the quoted message's content — reading that is what OriginOf and CurrentOf are
// for, on the id this returns.
func (r *Reader) QuotedOf(ctx context.Context, messageID, label string) (Quoted, error) {
	if strings.TrimSpace(messageID) == "" {
		return Quoted{}, ErrNoMessage
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, quotedScript(messageID, key), key, label+"/quoted")
	if err != nil {
		return Quoted{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		NotFound bool   `json:"notFound"`
		Quotes   bool   `json:"quotes"`
		QuotedID string `json:"quotedId"`
		Loaded   bool   `json:"quotedLoaded"`
		Chat     string `json:"quotedChat"`
		Sender   string `json:"quotedSender"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Quoted{}, fmt.Errorf("message: unexpected answer: %w", e)
	}
	if out.NotFound {
		return Quoted{}, ErrNotFound
	}
	if !out.OK {
		return Quoted{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	return Quoted{
		Quotes: out.Quotes, MessageID: out.QuotedID, Loaded: out.Loaded,
		ChatJID: out.Chat, SenderJID: out.Sender,
	}, nil
}

// GroupMention is a group named inside a message.
type GroupMention struct {
	// JID is the mentioned group.
	JID string
	// Subject is the group's name AS IT WAS when the message was written. The
	// page carries it on the message rather than resolving it, so a group renamed
	// afterwards still shows here under its old name — which is the honest answer
	// about what was said, not a staleness bug.
	Subject string
}

// Mentions is who and what a message names.
type Mentions struct {
	// People are the mentioned identities, serialized. On this LID-first build
	// they arrive under whichever namespace the sender used, exactly as the page
	// holds them — no conversion, because converting would invent an identity the
	// message never carried.
	People []string
	// Groups are mentioned groups. A SEPARATE FIELD, not people with a group
	// suffix: the page carries them in a different field with a different shape,
	// and merging them would lose the subject and lie about the kind.
	Groups []GroupMention
}

// Any says the message mentions anything at all.
func (m Mentions) Any() bool { return len(m.People) > 0 || len(m.Groups) > 0 }

func (m Mentions) String() string {
	return fmt.Sprintf("message.Mentions(people=%d groups=%d)", len(m.People), len(m.Groups))
}

// MentionsOf reports who and which groups a message names.
//
// It is the reference's Message.getMentions and Message.getGroupMentions in one
// read, for the reason OriginOf gives: two calls for one message model cost two
// round trips and may answer from two different moments.
//
// IT RETURNS IDENTITIES, NOT CONTACTS. The reference maps each id through
// getContactById and hands back Contact objects. Resolving a list of jids to
// contacts already exists here as capabilities/contacts.Recipients, which
// reports found and missing SEPARATELY — and that distinction is worth keeping,
// because a mention of somebody this session has no contact for is a normal
// thing that the reference's version silently turns into a half-empty object.
func (r *Reader) MentionsOf(ctx context.Context, messageID, label string) (Mentions, error) {
	if strings.TrimSpace(messageID) == "" {
		return Mentions{}, ErrNoMessage
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, mentionsScript(messageID, key), key, label+"/mentions")
	if err != nil {
		return Mentions{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK       bool     `json:"ok"`
		Why      string   `json:"why"`
		NotFound bool     `json:"notFound"`
		People   []string `json:"people"`
		Groups   []struct {
			JID     string `json:"jid"`
			Subject string `json:"subject"`
		} `json:"groups"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Mentions{}, fmt.Errorf("message: unexpected answer: %w", e)
	}
	if out.NotFound {
		return Mentions{}, ErrNotFound
	}
	if !out.OK {
		return Mentions{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	m := Mentions{People: out.People}
	for _, g := range out.Groups {
		m.Groups = append(m.Groups, GroupMention{JID: g.JID, Subject: g.Subject})
	}
	return m, nil
}

// ErrNotMine is a message this account did not send.
//
// IT IS NOT A PERMISSION ERROR. Message info is the server reporting on delivery
// of things THIS account sent; asking it about somebody else's message is a
// question with no meaning, and the reference refuses it before asking too.
var ErrNotMine = fmt.Errorf("message: message info is only about this account's own messages")

// Info is who received, read and played a message this account sent.
type Info struct {
	// Delivered, Read and Played are the identities that reached each state.
	//
	// THEY ARE LISTS, NOT COUNTS, and that is the entire difference between this
	// and ack. In a one-to-one, ack already says everything; in a group, "two of
	// five read it" is a different fact from "these two read it", and only the
	// second lets a caller act.
	Delivered, Read, Played []string
	// DeliveredRemaining, ReadRemaining and PlayedRemaining are how many
	// participants have NOT reached each state, as the page reports them.
	//
	// THEY ARE NOT len(list) SUBTRACTED FROM ANYTHING. The page carries them
	// separately, and deriving them here would require knowing the participant
	// count at the time of sending — which is not the same as the group's size
	// today, and would go quietly wrong the moment somebody leaves.
	//
	// -1 means the page did not report the number, which is distinguishable from
	// zero for the reason that distinction has been earned twice in this module
	// already (H90, H108).
	DeliveredRemaining, ReadRemaining, PlayedRemaining int
	// Answered says the store returned a record at all. A message too young, or
	// one the server has nothing to say about yet, comes back with Answered
	// false rather than as three empty lists — which would read as "nobody got
	// it" about a message nobody has been asked about.
	Answered bool
}

func (i Info) String() string {
	return fmt.Sprintf("message.Info(answered=%t delivered=%d read=%d played=%d remaining=%d/%d/%d)",
		i.Answered, len(i.Delivered), len(i.Read), len(i.Played),
		i.DeliveredRemaining, i.ReadRemaining, i.PlayedRemaining)
}

// InfoOf reports who received, read and played one of this account's messages.
//
// It is the reference's Message.getInfo, and it exists now because H71's
// conclusion was wrong about the cause: it measured WAWebMsgInfoCollection empty
// and read that as "this build gives ack, not who read it". The collection is
// still empty; the reference never reads it. See infoScript.
//
// A MESSAGE TOO YOUNG ANSWERS Answered=false RATHER THAN NOTHING. The reference
// handles that by sleeping inside the page for messages under 1250ms old; the
// wait belongs to the caller here, and this reports honestly instead of guessing
// how long to hold the page.
func (r *Reader) InfoOf(ctx context.Context, messageID, label string) (Info, error) {
	if strings.TrimSpace(messageID) == "" {
		return Info{}, ErrNoMessage
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, infoScript(messageID, key), key, label+"/info")
	if err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK        bool     `json:"ok"`
		Why       string   `json:"why"`
		NotFound  bool     `json:"notFound"`
		NotMine   bool     `json:"notMine"`
		Answered  bool     `json:"answered"`
		Delivered []string `json:"delivered"`
		Read      []string `json:"read"`
		Played    []string `json:"played"`
		DelRem    int      `json:"deliveredRemaining"`
		ReadRem   int      `json:"readRemaining"`
		PlayRem   int      `json:"playedRemaining"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Info{}, fmt.Errorf("message: unexpected answer: %w", e)
	}
	if out.NotFound {
		return Info{}, ErrNotFound
	}
	if out.NotMine {
		return Info{}, ErrNotMine
	}
	if !out.OK {
		return Info{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	return Info{
		Delivered: out.Delivered, Read: out.Read, Played: out.Played,
		DeliveredRemaining: out.DelRem, ReadRemaining: out.ReadRem,
		PlayedRemaining: out.PlayRem, Answered: out.Answered,
	}, nil
}

// Reaction is one emoji on a message, and everybody who put it there.
type Reaction struct {
	// Emoji is the reaction itself.
	Emoji string
	// ByMe says this account is among the senders. It comes from the page's own
	// hasReactionByMe rather than being derived by comparing jids, because this
	// build files under LID and "is my jid in this list" is precisely the
	// comparison that has gone wrong here before (H136, H148).
	ByMe bool
	// Senders are the identities that reacted with this emoji.
	Senders []string
}

// Reactions is every reaction on one message, grouped by emoji.
//
// GROUPED IS HOW THE PAGE HAS IT, and flattening would lose the grouping without
// gaining anything: the caller who wants a flat list can make one, and the caller
// who wants "how many liked it" cannot rebuild the groups from a flat list.
type Reactions struct {
	Groups []Reaction
}

// Total is how many people reacted, counting somebody twice if they used two
// emoji — which is what the page allows.
func (r Reactions) Total() int {
	n := 0
	for _, g := range r.Groups {
		n += len(g.Senders)
	}
	return n
}

// Any says the message carries any reaction.
func (r Reactions) Any() bool { return len(r.Groups) > 0 }

// String reports shape. The emoji is NOT rendered: it is the content of somebody
// else's act, and this module's renderings carry counts.
func (r Reactions) String() string {
	return fmt.Sprintf("message.Reactions(groups=%d senders=%d)", len(r.Groups), r.Total())
}

// ReactionsOf reports which reactions a message carries and who left them.
//
// It is the reference's Message.getReactions, and its ledger row was BLOCKED on
// two honest measurements that both looked at the loaded collection. See
// reactionsScript for what they missed: the record comes from an async fetch
// keyed by the id OBJECT, not from the models array, and not by the serialized
// id the reference passes — which is null on this build.
//
// A MESSAGE WITH NO REACTIONS IS NOT AN ERROR. It reports an empty Reactions,
// because "nobody reacted" is the ordinary state of most messages and turning it
// into a failure would make the healthy case indistinguishable from a broken read
// — the same distinction block.List had to draw.
func (r *Reader) ReactionsOf(ctx context.Context, messageID, label string) (Reactions, error) {
	if strings.TrimSpace(messageID) == "" {
		return Reactions{}, ErrNoMessage
	}
	key := nextStateKey()
	raw, err := r.parked(ctx, reactionsScript(messageID, key), key, label+"/reactions")
	if err != nil {
		return Reactions{}, fmt.Errorf("%w: %v", ErrRead, err)
	}
	var out struct {
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		NotFound bool   `json:"notFound"`
		Groups   []struct {
			Emoji   string   `json:"emoji"`
			ByMe    bool     `json:"byMe"`
			Senders []string `json:"senders"`
		} `json:"groups"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Reactions{}, fmt.Errorf("message: unexpected answer: %w", e)
	}
	if out.NotFound {
		return Reactions{}, ErrNotFound
	}
	if !out.OK {
		return Reactions{}, fmt.Errorf("%w (%s)", ErrRead, out.Why)
	}
	var res Reactions
	for _, g := range out.Groups {
		res.Groups = append(res.Groups, Reaction{Emoji: g.Emoji, ByMe: g.ByMe, Senders: g.Senders})
	}
	return res, nil
}
