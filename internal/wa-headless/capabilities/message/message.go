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
// WHY NOT MENTIONS. getMentions was the obvious neighbour and it was measured
// first: 395 loaded messages, ZERO carrying a mention under any of five
// candidate field names. Shipping a reader never seen returning anything is the
// trap this repository catalogued in H93, so it was left alone and the
// measurement written down instead (H106).
package message

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

const stateKey = "__waHeadlessMessage"

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
	raw, err := r.parked(ctx, originScript(messageID), label+"/origin")
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

func (r *Reader) parked(ctx context.Context, kick, label string) (string, error) {
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
			return r.eval(c, `window.`+stateKey+` || ""`, &raw)
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

// ShapeOf reports the FIELD NAMES the page has on one message, and never a
// value. See shapeScript for why this is a deliberate divergence from the
// reference's Message.rawData rather than a thin version of it.
//
// Sorted, so two reads of the same message compare.
func (r *Reader) ShapeOf(ctx context.Context, messageID, label string) ([]string, error) {
	if strings.TrimSpace(messageID) == "" {
		return nil, ErrNoMessage
	}
	raw, err := r.parked(ctx, shapeScript(messageID), label+"/shape")
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
