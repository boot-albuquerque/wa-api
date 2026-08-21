package send

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Sending a poll. NOT PROVEN AGAINST THE LIVE BUILD — see H69.
//
// TWO LIVE ATTEMPTS BOTH THREW "Cannot read properties of undefined (reading
// 'name')", identically, with correctOptionKey nulled and with it omitted. By
// the rule H58 paid for, an error that does not move when the varied argument
// moves is evidence that the varied argument is not the cause.
//
// WHAT THE SECOND FAILURE REVEALED is an assumption made without noticing. At
// the UI call site the poll object comes from a local helper `b(...)`, and I
// read that helper's argument list as createPollCreationMsgData's. It is not:
// the message data that helper produces carries correctOptionIndex, while the
// object I was copying names correctOptionKey. Different names mean different
// functions, and the whole shape below is therefore the UI helper's, not the
// API's.
//
// WHAT IS ACTUALLY KNOWN, and worth keeping:
//
//	an option is {name, localId}         (from the add-option merge path)
//	the message carries pollName, pollOptions, pollSelectableOptionsCount,
//	    pollContentType, pollType, correctOptionIndex
//	sendPollCreation({poll, chat, quotedMsg, isWamoSub})   (the call site)
//
// The unknown is what createPollCreationMsgData itself destructures. Reading
// that needs the head of its generator body, which the bundle grep truncated.
//
// This file is kept rather than deleted because it encodes those measurements
// and its tests lock them. It is NOT wired to anything.
//
// The original (mistaken) reading, kept for the record:
//
//	const poll = createPollCreationMsgData({correctOptionKey, filteredOptions,
//	    isPhotoPoll, isSingleOption, pollEndTime, pollType, question,
//	    hideVoterNames});
//	await sendPollCreation({poll, chat, quotedMsg, isWamoSub});
//
// AN OPTION IS AN OBJECT — {name, localId} — not a string. That came from the
// add-option merge path, which builds exactly that and keys a Set on
// option.name. Passing strings would have been the obvious guess and wrong.
//
// pollType IS RESOLVED IN THE PAGE AND REPORTED BACK, not chosen here. Which
// enum holds it was never measured on this build, so the script looks in the
// plausible places and says which one answered — the same self-measuring shape
// the group rename used to find that a subject lives in chat.formattedTitle.

// Bounds. Var, not const, so tests can compress the clock.
var (
	pollBudget = 30 * time.Second
	pollTick   = 500 * time.Millisecond
)

// Poll limits. MinOptions is two because a poll with one answer is not a
// question, and the page would refuse it later and less clearly.
const (
	MinPollOptions   = 2
	MaxPollOptions   = 12
	MaxQuestionBytes = 512
	MaxOptionBytes   = 256
)

var (
	// ErrPoll is the page refusing or throwing.
	ErrPoll = fmt.Errorf("send: the page refused the poll")
	// ErrPollQuestion is a missing or oversized question.
	ErrPollQuestion = fmt.Errorf("send: a poll needs a question")
	// ErrPollOptions is a bad option list.
	ErrPollOptions = fmt.Errorf("send: a poll needs between 2 and 12 distinct, non-empty options")
	// ErrPollNotDelivered is the postcondition: no poll of ours appeared in the
	// chat.
	ErrPollNotDelivered = fmt.Errorf("send: the page accepted the poll and none appeared in the chat")
)

// PollResult is a poll this session sent.
type PollResult struct {
	// ID is the created poll message.
	ID string
	// Options is how many answers it offers. A count, not the text.
	Options int
	// PollTypeSource says where the page's poll-type value was found, or
	// "none". Reported because it was never measured on this build, and a
	// capability that cannot say where it looked cannot be checked.
	PollTypeSource string
	Waited         time.Duration
}

func (p PollResult) String() string {
	return fmt.Sprintf("send.PollResult(id=%s options=%d pollTypeFrom=%s waited=%s)",
		p.ID, p.Options, p.PollTypeSource, p.Waited.Round(time.Millisecond))
}

const pollStateKey = "__waHeadlessPoll"

// PollTo sends a poll to a chat that is already loaded.
//
// multi decides whether voters may pick more than one answer. It is a parameter
// because a single-answer poll and a multi-answer one ask different questions,
// and choosing on the caller's behalf would change what the recipients are
// being asked.
func PollTo(ctx context.Context, runner *engine.Runner, eval spa.Evaluator,
	toJID, question string, options []string, multi bool, label string) (PollResult, error) {

	if strings.TrimSpace(toJID) == "" {
		return PollResult{}, fmt.Errorf("send: empty recipient")
	}
	if strings.TrimSpace(question) == "" || len(question) > MaxQuestionBytes {
		return PollResult{}, ErrPollQuestion
	}
	// VALIDATED HERE so the refusal names the problem. The page would reject
	// most of these too, with a message about a validation error code.
	seen := map[string]bool{}
	for _, o := range options {
		if strings.TrimSpace(o) == "" || len(o) > MaxOptionBytes || seen[o] {
			return PollResult{}, ErrPollOptions
		}
		seen[o] = true
	}
	if len(options) < MinPollOptions || len(options) > MaxPollOptions {
		return PollResult{}, ErrPollOptions
	}
	start := time.Now()

	optsJSON, err := json.Marshal(options)
	if err != nil {
		return PollResult{}, fmt.Errorf("send: encoding poll options: %w", err)
	}

	var kicked string
	if err := runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return eval(ctx, pollScript(toJID, question, string(optsJSON), multi), &kicked)
	}); err != nil {
		return PollResult{}, fmt.Errorf("%w: %v", ErrPoll, err)
	}

	var out struct {
		Stage      string `json:"stage"`
		OK         bool   `json:"ok"`
		Why        string `json:"why"`
		ID         string `json:"id"`
		Options    int    `json:"options"`
		TypeSource string `json:"typeSource"`
	}
	deadline := time.Now().Add(pollBudget)
	for {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return eval(ctx, pollResultScript, &raw)
		}); err != nil {
			return PollResult{}, fmt.Errorf("%w: %v", ErrPoll, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return PollResult{}, fmt.Errorf("send: unexpected poll answer: %w", err)
		}
		if out.Stage != "pending" && out.Stage != "settling" {
			break
		}
		if !time.Now().Before(deadline) {
			if out.Stage == "settling" {
				return PollResult{}, fmt.Errorf("%w within %s", ErrPollNotDelivered, pollBudget)
			}
			return PollResult{}, fmt.Errorf("%w: the page never settled within %s", ErrPoll, pollBudget)
		}
		time.Sleep(pollTick)
	}

	switch {
	case out.Why == "NO_CHAT":
		return PollResult{}, fmt.Errorf("%w: no such chat is loaded", ErrPoll)
	case !out.OK:
		return PollResult{}, fmt.Errorf("%w at %s (%s)", ErrPoll, out.Stage, out.Why)
	}
	if out.ID == "" {
		return PollResult{}, ErrPollNotDelivered
	}
	return PollResult{ID: out.ID, Options: out.Options,
		PollTypeSource: out.TypeSource, Waited: time.Since(start)}, nil
}

func pollScript(toJID, question, optionsJSON string, multi bool) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(pollStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(pollStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(toJID) + `);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			const names = ` + optionsJSON + `;
			// AN OPTION IS AN OBJECT. {name, localId} is what the app's own
			// add-option path builds, and it keys a Set on option.name.
			const filteredOptions = names.map((name, i) => ({ name: name, localId: i }));

			// WHERE pollType LIVES WAS NEVER MEASURED HERE. Look in the
			// plausible places and report which answered, rather than picking
			// one and hoping.
			let pollType = null, typeSource = 'none';
			for (const [mod, key] of [['WAWebMsgType', 'PollType'],
			                          ['WAWebMsgType', 'POLL_TYPE'],
			                          ['WAWebPollsGatingUtils', 'PollType']]) {
				try {
					const m = window.require(mod);
					const e = m && m[key];
					if (e) {
						pollType = e.POLL || e.Poll || e.DEFAULT || e[Object.keys(e)[0]];
						typeSource = mod + '.' + key;
						break;
					}
				} catch (err) {}
			}

			// EVERY ID ALREADY IN THIS CHAT, so the created poll is identified
			// by absence-from-a-set rather than by time — the check that
			// produced a false positive in this module once.
			const chatKey = chat.id.toString();
			const MC = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			const seen = new Set();
			for (const m of MC.getModelsArray()) {
				try {
					if (m.id && m.id.remote && m.id.remote.toString() === chatKey) { seen.add(m.id.id); }
				} catch (e) {}
			}

			stage = 'apply';
			const A = window.require('` + string(spa.ModulePollsSendPollCreationMsgAction) + `');
			// correctOptionKey IS OMITTED, not nulled. Passing null threw
			// "Cannot read properties of undefined (reading 'name')" — and
			// 'name' lives on an OPTION, so the only thing that could have been
			// undefined is filteredOptions[correctOptionKey]. A quiz poll names
			// its right answer by key; an ordinary poll has none, and null is
			// not the same as absent when the value is used as an index.
			const poll = await A.createPollCreationMsgData({
				filteredOptions: filteredOptions,
				isPhotoPoll: false,
				isSingleOption: ` + strconv.FormatBool(!multi) + `,
				pollEndTime: null,
				pollType: pollType,
				question: ` + strconv.Quote(question) + `,
				hideVoterNames: false
			});
			await A.sendPollCreation({ poll: poll, chat: chat,
				quotedMsg: null, isWamoSub: false });

			stage = 'verify';
			park({ stage: 'settling', ok: true, why: '',
				chatKey: chatKey, seen: seen,
				options: filteredOptions.length, typeSource: typeSource });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 200) });
		}
		})();
		return { started: true };
	})())`
}

const pollResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + pollStateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage === 'settling') {
		const MC = window.require('WAWebMsgCollection').MsgCollection;
		for (const m of MC.getModelsArray()) {
			try {
				if (!m.id || !m.id.fromMe) { continue; }
				if (!m.id.remote || m.id.remote.toString() !== s.chatKey) { continue; }
				if (s.seen.has(m.id.id)) { continue; }
				// IT HAS TO BE A POLL. A new message of ours in the chat is not
				// proof the POLL landed — anything else sent meanwhile would
				// satisfy that.
				if (!m.pollName && m.type !== 'poll_creation') { continue; }
				return { stage: 'done', ok: true, why: '', id: m.id.id,
					options: s.options, typeSource: s.typeSource };
			} catch (e) {}
		}
		return { stage: 'settling', ok: false, why: '' };
	}
	return s;
})())`
