package spa

// ResolveIdentityExpr resolves a jid string to the identity THIS BUILD uses.
//
// It lives in this package because it is page knowledge, not capability logic —
// and because two capabilities needed it for opposite purposes and nearly grew
// two copies of it.
//
// WHAT IT DOES NOT DO: it does not obtain a chat. That distinction was learned
// rather than designed. Sending wants a chat CREATED when none exists, because
// a first message to somebody is the ordinary case. Announcing presence must
// never create one — telling somebody you are typing is not the same as opening
// a conversation with them. Factoring at "resolve the identity" is what lets
// both use the same measured steps without one of them growing a side effect
// the other cannot have.
//
// WHY EACH STEP EXISTS, all measured (HOUSEKEEP H34, H48):
//
//	createWid       BUILDS a wid from text; asChatWid only VALIDATES one, and
//	                handing it a string fails with "e.isUser is not a function"
//	group           short-circuits: queryWidExists resolves PEOPLE and answers
//	                NULL for a group, which made every group send die reporting
//	                NOT_ON_WHATSAPP for a group plainly present
//	queryWidExists  is the SPA's own resolution; this build is LID-first and the
//	                phone jid alone fails with "No LID for user"
//	_serialized     is what verification compares against — 397 of 399 messages
//	                live under "@lid", so comparing against the caller's phone
//	                jid matches nothing
//
// It is a JavaScript function of (jidString) returning
// {ok, why, wid, jid, isGroup}.
const ResolveIdentityExpr = `(async function (jidString) {
	const WidFactory = window.require('` + string(ModuleWidFactory) + `');
	const local = WidFactory.createWid(jidString);
	if (!local) { return { ok: false, why: 'WID_NULL' }; }

	// A group IS its own identity and has no lid counterpart.
	if (local.server === 'g.us') {
		const jid = (typeof local._serialized === 'string') ? local._serialized : jidString;
		return { ok: true, why: '', wid: local, jid: jid, isGroup: true };
	}

	const Query = window.require('` + string(ModuleQueryExistsJob) + `');
	const exists = await Query.queryWidExists(local);
	if (!exists || !exists.wid) { return { ok: false, why: 'NOT_ON_WHATSAPP' }; }
	const wid = exists.wid;
	const jid = (typeof wid._serialized === 'string') ? wid._serialized : '';
	if (!jid) { return { ok: false, why: 'WID_NOT_SERIALIZED' }; }
	return { ok: true, why: '', wid: wid, jid: jid, isGroup: false };
})`
