package spa

// How this build confirms a write — and why every capability was paying to
// rediscover it.
//
// FOUR OF FIVE CAPABILITIES IN ONE SESSION FAILED THE SAME WAY: the page accepts
// the call, does not throw, and the postcondition never sees anything. Written
// out, the module has met three different behaviours and treated each as a
// surprise:
//
//	IMMEDIATE      the model moves in this session, within a second
//	               star, block, edit, mute, archive/pin chat, forward, send,
//	               react, revoke, labels, group subject, invite code
//
//	CROSS_SESSION  the change reaches the server and this session never sees it
//	               group participants (H58), group policies (H79),
//	               chat unread count (H78)
//
//	NOTHING        the call is accepted and nothing happens anywhere
//	               Cmd verbs with no listener (H78), message pin (H81)
//
// THE OBVIOUS HYPOTHESIS IS WRONG, and it is worth saying so because it is the
// one anybody would reach for: the module layer does NOT predict the class.
// WAWebSetSubjectGroupAction is IMMEDIATE and WAWebSetPropertyGroupAction is
// CROSS_SESSION — same suffix, same family, different answers. Cmd.sendStarMsgs
// is IMMEDIATE and Cmd.markChatUnread is NOTHING.
//
// WHAT DOES CORRELATE, across every case measured so far:
//
//	the app writes the model itself, optimistically   -> IMMEDIATE
//	the model is updated by an inbound server event   -> CROSS_SESSION
//	nothing is wired to the call at all               -> NOTHING
//
// Starring sets msg.star locally; archiving sets chat.archive locally; labels
// only moved once the local mirror was called explicitly (H72), which is the
// same fact seen from the other side. Unread is cleared by a seen receipt,
// participants by a group notification, policies by a property notification —
// all inbound, and this session does not apply them.
//
// THAT IS A HYPOTHESIS WITH EVIDENCE, NOT A LAW. It is written here so the next
// capability starts from a prediction it can test in thirty seconds instead of
// from a surprise it discovers in a failed live run. ClassifyWriteExpr is that
// test.

// WriteClass is how a write can be confirmed.
type WriteClass string

const (
	// ClassImmediate means a reader in this session will see the change.
	ClassImmediate WriteClass = "IMMEDIATE"
	// ClassCrossSession means only a later session will.
	ClassCrossSession WriteClass = "CROSS_SESSION"
	// ClassNothing means the write did not happen at all.
	ClassNothing WriteClass = "NOTHING"
	// ClassUnknown is the honest answer when the probe could not decide —
	// usually because the reader itself is wrong.
	ClassUnknown WriteClass = "UNKNOWN"
)

// ClassifyWriteExpr measures whether a write is visible to the session that made
// it.
//
// IT POLLS, AND THE FIRST VERSION DID NOT. That version read the value once,
// straight after the await, and reported the starring control — a write measured
// IMMEDIATE at 696ms — as "did not move here". The instrument built to detect
// the H61 defect had the H61 defect.
//
// The control caught it before any unmeasured case was classified, which is the
// whole reason the controls run first.
//
// It takes two page functions: one that performs the write and one that reads
// the value back. It records the reader BEFORE, performs the write, parks the
// reader, and Go polls — the waiting is bounded by the CALLER, because a clock
// inside the page is a duration nothing here can see (invariant 6).
//
// It answers IMMEDIATE or "did not move here". Distinguishing CROSS_SESSION from
// NOTHING needs a second session and therefore a second call, which is the
// caller's job and is exactly the step every failed capability skipped.
//
// It is a JavaScript function of (stateKey, writeFn, readFn); the caller then
// polls ClassifyReadExpr with the same key.
// The outer function is SYNCHRONOUS on purpose: engine.Tab.Evaluate does not
// await, so an async wrapper hands Go a Promise where it expects an answer. The
// await lives in the inner IIFE, which is the store-and-poll shape every
// capability here uses.
const ClassifyWriteExpr = `(function (stateKey, writeFn, readFn) {
	const snap = () => {
		try {
			const v = readFn();
			// Stringified so two reads compare cleanly whether the reader
			// returns a number, a boolean or a count.
			return (v === undefined || v === null) ? 'nil' : String(v);
		} catch (e) { return 'THREW:' + String((e && e.message) || e).slice(0, 80); }
	};
	const before = snap();
	if (before.indexOf('THREW:') === 0) {
		window[stateKey] = { stage: 'done', ok: false, why: 'READER_THREW: ' + before,
			before: before, after: before, moved: false };
		return 'started';
	}
	window[stateKey] = { stage: 'pending', ok: false, why: '', before: before };
	(async () => {
		try {
			await writeFn();
		} catch (e) {
			window[stateKey] = { stage: 'done', ok: false,
				why: 'WRITE_THREW: ' + String((e && e.message) || e).slice(0, 140),
				before: before, after: before, moved: false };
			return;
		}
		// THE READER IS PARKED, not its value. Go re-runs it each round, which
		// is what turns "not yet" into "not at all" only after a real wait.
		window[stateKey] = { stage: 'settling', ok: true, why: '',
			before: before, read: snap };
	})();
	return 'started';
})`

// ClassifyReadExpr is polled by the caller until it stops answering "settling".
// It re-runs the parked reader every round.
const ClassifyReadExpr = `(function (stateKey) {
	const s = window[stateKey];
	if (!s) { return JSON.stringify({ stage: 'done', ok: false, why: 'STATE_MISSING' }); }
	if (s.stage === 'settling') {
		const now = s.read();
		if (now !== s.before) {
			return JSON.stringify({ stage: 'done', ok: true, why: '',
				before: s.before, after: now, moved: true });
		}
		return JSON.stringify({ stage: 'settling', ok: true, why: '',
			before: s.before, after: now, moved: false });
	}
	return JSON.stringify(s);
})`
