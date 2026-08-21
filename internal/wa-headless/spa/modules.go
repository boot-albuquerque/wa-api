package spa

// The module inventory: what this stack asks web.whatsapp.com for, in one
// place, verified at session start.
//
// The integration surface is window.require('<ModuleName>'), and those names
// are undocumented Meta contract — they change when Meta ships a build, with no
// notice and no deprecation. ADR-0006 D4 governs this, and its non-obvious rule
// is the one implemented here: the inventory is verified AT STARTUP, failing
// loudly with the list of missing names.
//
// Without that, a rename surfaces as an exception halfway through a dispatch,
// on whichever capability happened to run first, with a message about a
// property of undefined. With it, the boot stops in a predictable place and
// names the cause.
//
// Names are constants HERE and nowhere else. A literal at a call site is the
// defect this file exists to prevent (ADR-0004).
//
// Source: whatsapp-web.js 1.34.7, as installed in the product
// (services/wa-worker/node_modules/whatsapp-web.js). Read from that tree, not
// from memory. Note that ../spa/doc.go cites main @ 942d236a11ad; the installed
// version is what the product actually runs, and it is what this list follows.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"wa-api/internal/wa-headless/engine"
)

// Module is a name passed to window.require.
type Module string

// The modules whatsapp-web.js resolves in its AuthStore at startup.
//
// This is a deliberate MINIMUM, not the library's full surface. wwebjs 1.34.7
// requires 41 distinct modules across its whole feature set; this stack needs
// six capabilities, so importing all 41 would be inventing dependencies to
// break on. Each capability adds the modules it actually uses, and the parity
// matrix says which capabilities there are.
//
// These are the ones that make a session a session — the socket, the
// connection state, the command channel, the identity. If any is gone, nothing
// above it can work, so they are the right thing to fail the boot on.
const (
	ModuleBase64                = Module("WABase64")
	ModuleAdvSignatureAPI       = Module("WAWebAdvSignatureApi")
	ModuleCmd                   = Module("WAWebCmd")
	ModuleCompanionRegClientUtl = Module("WAWebCompanionRegClientUtils")
	ModuleConnModel             = Module("WAWebConnModel")
	ModuleSignalStoreAPI        = Module("WAWebSignalStoreApi")
	ModuleSocketModel           = Module("WAWebSocketModel")
	ModuleUserPrefsInfoStore    = Module("WAWebUserPrefsInfoStore")
	// ModuleUserPrefsMeUser is where the OWNER IDENTITY lives, and it is a
	// different module from ModuleUserPrefsInfoStore above — a distinction that
	// cost a lookup to notice, because the names differ by one word.
	//
	// It is what whatsapp-web.js reads for the account's wid (Client.js:351-364
	// in 1.34.7: getMaybeMePnUser() || getMaybeMeLidUser()), and EVIDENCIA-SPA.md
	// M3 measured it here: the getters answer EMPTY for a full 75s on an
	// unpaired profile and PRESENT at T+0.01s on a paired one, so the module
	// discriminates rather than merely existing.
	ModuleUserPrefsMeUser = Module("WAWebUserPrefsMeUser")
	// ModuleMsgCollection holds the message store, and it is NOT in
	// RequiredAtStartup on purpose.
	//
	// The rule this file states is that each capability adds the modules it
	// uses; the unstated half is that a module only belongs at STARTUP when
	// nothing above it can work without it. A session with no message stream is
	// still a session that can be checked for liveness, asked who it belongs to
	// and stopped cleanly — so failing every boot over it would trade a working
	// degraded session for none at all.
	//
	// capabilities/messagemeta verifies it at install time instead, and names
	// it when it is missing. Measured 2026-08-19: resolves with MsgCollection
	// carrying on/off/getModelsArray (PARIDADE-WWEBJS.md §6.4).
	ModuleMsgCollection = Module("WAWebMsgCollection")
	// ModuleChatCollection and ModuleSendTextMsgChatAction are what
	// capabilities/send needs, and like the message store they are NOT in
	// RequiredAtStartup: a session that cannot send is still a session that can
	// be checked, identified and stopped, so failing every boot over them would
	// trade a working degraded session for none.
	//
	// MEASURED 2026-08-20 against this build: ChatCollection carries get/find/
	// add/getModelsArray, and SendTextMsgChatAction carries sendTextMsgToChat/3
	// plus addAndSendTextMsg/3. Four names taken from other builds
	// (WAWebSendMsg, WAWebMsgSend, WAWebSendMessage, WAWebComposeMessage) do
	// not resolve here at all.
	ModuleChatCollection        = Module("WAWebChatCollection")
	ModuleSendTextMsgChatAction = Module("WAWebSendTextMsgChatAction")
	// ModuleFindChatAction is how a chat is OBTAINED, and it is a different
	// module from the collection that STORES chats.
	//
	// Measured 2026-08-20: ChatCollection.get returns null for a correspondent
	// this account has never talked to, and ChatCollection.find throws
	// "this.findImpl is not a function". WAWebFindChatAction carries
	// findExistingChat and findOrCreateLatestChat — the second is the one that
	// works when there is no chat yet, which is the ordinary case for a first
	// message.
	ModuleFindChatAction = Module("WAWebFindChatAction")
	// ModuleQueryExistsJob resolves a phone number to the identity the SERVER
	// knows, which on this build is a LID.
	//
	// This is the piece that unblocks sending, and it is a QUERY — it goes to
	// WhatsApp, it does not convert locally. Measured 2026-08-20 against a
	// number never spoken to: queryWidExists returns
	// {biz, bizInfo, isUsernameSearch, wid} with wid.server === "lid".
	//
	// Baileys has to build this itself — a LIDMappingStore with a USync
	// fallback — because it speaks the protocol. Driving the SPA, the mapping
	// is already there and this is how it is asked. Same understanding, a
	// fraction of the machinery, which is the whole reason the parity document
	// says to copy the UNDERSTANDING and not the code.
	ModuleQueryExistsJob = Module("WAWebQueryExistsJob")
	// ModuleContactCollection is the roster.
	//
	// Measured 2026-08-20 on the lab profile: 944 models, split 454 "c.us" /
	// 489 "lid" / 1 "g.us". That split is nothing like the message collection's
	// 397-of-399 lid, and the reason is duplication — see ModuleContactGetters.
	ModuleContactCollection = Module("WAWebContactCollection")
	// ModuleContactGetters reads a contact model's fields.
	//
	// Measured 2026-08-20 over all 944 models, and the numbers decide what a
	// listing may promise:
	//
	//	getName          1 of 944    <- the address book, and the lab profile has one entry
	//	getShortName     0 of 944
	//	getPushname    456 of 944
	//	getVerifiedName 22 of 944
	//	getIsBusiness   25 of 944
	//
	// So pushname is the only name worth returning here, and getName being
	// empty is a property of the PROFILE (nothing saved to the address book),
	// not of the build. A listing that promised "name" would return nothing
	// for 943 of 944 people.
	ModuleContactGetters = Module("WAWebContactGetters")
	// ModuleContactProfilePicThumbBridge asks the SERVER for an avatar.
	//
	// THE ARGUMENT IS NOT A WID. Measured 2026-08-20: passing a wid to
	// requestProfilePicFromServer throws "Cannot read properties of undefined
	// (reading 'isNewsletter')" — the page reading a field off a `.id` a wid
	// does not have. Reading profilePicResync's own source settled the shape:
	//
	//	function k(t){ ... t.map(... yield v(t.id, {tcToken, commonGid}) ...) }
	//
	// so the call takes an object CARRYING .id, and resync takes an array of
	// them. Both were then confirmed against the live account.
	//
	// The local ProfilePicThumbCollection is not a substitute: it held 68
	// models for 545 people, and only 33 of those carried an eurl.
	ModuleContactProfilePicThumbBridge = Module("WAWebContactProfilePicThumbBridge")
	// ModuleWidFactory BUILDS an identity from text.
	//
	// It was a bare literal inside the send script until a second capability
	// needed it. A module name repeated in two places is the same bug waiting
	// to diverge — the page renames it once and only one call site is fixed —
	// which is why ADR-0004 has no exception for "it is only used twice".
	//
	// createWid BUILDS from a string; asChatWid only VALIDATES an existing wid,
	// and handing it a string fails with "e.isUser is not a function".
	ModuleWidFactory = Module("WAWebWidFactory")
	// ModuleContactSyncBridge asks the page to refresh the roster.
	//
	// MEASURED COST AND MEASURED EFFECT, 2026-08-20, on the lab profile:
	// doFullContactSync() took 41.9 SECONDS, returned undefined, and moved one
	// number — verified business names, 52 to 56. Total contacts unchanged at
	// 944; getName unchanged at 1; pushname unchanged at 456.
	//
	// So it is a refresh, not a fetch. It does not add people and it cannot
	// invent address-book names the primary device does not have. The
	// capability built on it reports the before/after rather than claiming to
	// have "primed" anything.
	ModuleContactSyncBridge = Module("WAWebContactSyncBridge")
	// ModuleMediaOpaqueData wraps bytes for sending.
	//
	// THE NAME THE REFERENCE WOULD USE IS NULL HERE. Measured 2026-08-20:
	// WAWebOpaqueData and WAOpaqueData both resolve to null on this build; the
	// module that exists is WAWebMediaOpaqueData, and it was found by reading
	// WAWebMediaPrep.getMediaPropsNew, which names it in its own source.
	//
	// createFromData(data, type) returns a Promise and KEEPS the object it was
	// given when the type matches — which is why a File survives it and carries
	// its filename to the recipient, where a Blob would arrive nameless.
	ModuleMediaOpaqueData = Module("WAWebMediaOpaqueData")
	// ModulePrepRawMedia turns opaque data into a sendable MediaPrep.
	//
	// prepRawMedia(file, opts) branches on opts.isPtt, opts.asDocument,
	// opts.asGif, opts.isAudio, opts.asSticker and opts.asStickerPack, falling
	// through to UNKNOWN. The returned MediaPrep carries waitForPrep — where
	// the encryption and upload happen — and sendToChat.
	ModulePrepRawMedia = Module("WAWebPrepRawMedia")
	// ModuleMediaPrep is the class module; the send is a method on the
	// INSTANCE that prepRawMedia returns, and it takes ONE object:
	// sendToChat({chat, earlyUpload, options}). Reading that signature is what
	// avoided a fifth blind correction at this layer.
	ModuleMediaPrep = Module("WAWebMediaPrep")
	// ModuleGroupCreateJob creates a group, WITHOUT the interface.
	//
	// There are two doors, and the obvious one is wrong for us:
	// WAWebCreateGroupAction is the UI layer — its source opens a
	// WAWebToastManager toast and builds React elements, which a headless
	// driver has no business triggering. The JOB below is what that action
	// calls underneath.
	//
	// The call shape was read from the app's OWN code, not guessed:
	//
	//	const args = {title, thumb: null, full: null, restrict: false,
	//	              announce: false, membershipApprovalMode: false,
	//	              memberAddMode: false, memberShareGroupHistoryMode: false};
	//	const res = await GroupCreateJob.createGroup(args, participants, outContacts);
	//	const gid = WidFactory.asGroupWidOrThrow(res.wid);
	//
	// It also exports GroupAlreadyExistsError, which is the page's own notion
	// of "this one is already there".
	ModuleGroupCreateJob = Module("WAWebGroupCreateJob")
	// ModuleGroupMutationParticipantUtils turns a wid into the participant
	// shape createGroup expects. Read from the same source:
	// getGroupMutationParticipant(wid, true, "createGroup").
	ModuleGroupMutationParticipantUtils = Module("WAWebGroupMutationParticipantUtils")
	// ModuleGroupMetadataGetters reads a group's subject, which is how a
	// created group is recognised again without keeping state on this side.
	ModuleGroupMetadataGetters = Module("WAWebGroupMetadataGetters")
	// ModuleGroupMetadataCollection holds group metadata, including
	// participants — the postcondition a creation must satisfy.
	ModuleGroupMetadataCollection = Module("WAWebGroupMetadataCollection")
	// ModulePresenceChatAction is the app's OWN entry point for typing state.
	//
	// It takes a CHAT, not a wid — its source reads getIsNewsletter(e) and
	// e.id.isBot(), which are the app's guards against announcing typing where
	// that makes no sense. The layer underneath (WAWebChatStateBridge) takes a
	// wid and skips those checks; using it would mean deciding we know better
	// than the app about where a typing indicator belongs.
	//
	// It also manages resend timers (presenceResendTimerId, pausedTimerId), so
	// a composing state maintains itself the way a real client's does.
	ModulePresenceChatAction = Module("WAWebPresenceChatAction")
	// ModuleContactPresenceBridge carries the two halves presence needs that
	// the chat action does not: going online/offline for the ACCOUNT, and
	// subscribing to somebody else's presence.
	//
	// Measured 2026-08-20: of 384 presence models on the lab account, ZERO were
	// subscribed. Another person's presence is not free — it has to be asked
	// for, and a capability that read the collection without subscribing would
	// report everyone as permanently silent.
	ModuleContactPresenceBridge = Module("WAWebContactPresenceBridge")
	// ModulePresenceCollection holds what is known about each presence:
	// isOnline, isSubscribed, chatstate{type}, typingUserIds, recordingUserIds.
	ModulePresenceCollection = Module("WAWebPresenceCollection")
	// ModuleChatGetters reads a chat's fields.
	//
	// IT EXPORTS getName AND getName IS THE WRONG ONE. Measured 2026-08-20
	// over all 384 chats: getName answered for **1**, while formattedTitle
	// answered for **384**. A listing built on the obvious getter would show a
	// title for one conversation out of every 384 — and would look correct in
	// every unit test, because a double would return whatever it was told.
	//
	// The same trap as the contact roster, where getName also answered for 1 of
	// 944 (H39). On a companion device the address book lives on the phone;
	// formattedTitle is what the app itself renders.
	ModuleChatGetters = Module("WAWebChatGetters")
)

// RequiredAtStartup is verified before any capability runs.
var RequiredAtStartup = []Module{
	ModuleBase64,
	ModuleAdvSignatureAPI,
	ModuleCmd,
	ModuleCompanionRegClientUtl,
	ModuleConnModel,
	ModuleSignalStoreAPI,
	ModuleSocketModel,
	ModuleUserPrefsInfoStore,
	// Added 2026-08-19 with capabilities/owner, following the rule stated
	// above: each capability adds the modules it actually uses. Until then a
	// boot could reach READY while the module the owner identity lives in was
	// absent — and the failure would have surfaced later, inside a capability,
	// instead of at the boot with a named cause.
	//
	// Safe to require, measured rather than assumed: M3 recorded the exports of
	// this module as IDENTICAL on the paired and the unpaired profile, so
	// demanding it does not break an unpaired boot. What differs between the
	// profiles is what the getters ANSWER, not whether the module resolves.
	ModuleUserPrefsMeUser,
}

// ErrModulesMissing is a boot that stopped because the page no longer exposes
// what this stack needs.
//
// It is a distinct error because the operational response is distinct: a
// missing module is Meta shipping a build, and no amount of retrying or
// recycling fixes it. Treating it as a transient failure would make a fleet
// restart every session, forever, over something only a code change resolves.
type ErrModulesMissing struct {
	Missing []Module
}

func (e *ErrModulesMissing) Error() string {
	names := make([]string, len(e.Missing))
	for i, m := range e.Missing {
		names[i] = string(m)
	}
	return fmt.Sprintf("spa: web.whatsapp.com no longer exposes %d module(s): %s. "+
		"These names are undocumented Meta contract and change when Meta ships a build; "+
		"this is a code change, not a retry", len(e.Missing), strings.Join(names, ", "))
}

// resolveScript asks the page which of the given names resolve.
//
// It returns the MISSING ones, not the present ones, so the answer is short and
// carries no module contents — only names this code already knew. window.require
// throws for an unknown name, hence the try/catch; a name that resolves to a
// falsy value counts as missing too, because a module that is there but empty
// is not a module we can use.
func resolveScript(modules []Module) string {
	names := make([]string, len(modules))
	for i, m := range modules {
		names[i] = `'` + string(m) + `'`
	}
	return `JSON.stringify((() => {
		const wanted = [` + strings.Join(names, ",") + `];
		if (typeof window.require !== 'function') return wanted;
		const missing = [];
		for (const name of wanted) {
			try {
				if (!window.require(name)) missing.push(name);
			} catch (e) {
				missing.push(name);
			}
		}
		return missing;
	})())`
}

// VerifyInventory checks the modules at session start.
//
// It runs under the Query budget rather than StateProbe: this is a real
// question for the page to answer, not a liveness poke, and a page still
// booting can legitimately take longer than a probe allows.
func VerifyInventory(ctx context.Context, r *engine.Runner, eval Evaluator, modules []Module) error {
	if len(modules) == 0 {
		return nil
	}

	var raw string
	if err := r.Do(ctx, engine.OpQuery, "spa/verify-inventory", func(ctx context.Context) error {
		return eval(ctx, resolveScript(modules), &raw)
	}); err != nil {
		return fmt.Errorf("spa: verifying the module inventory: %w", err)
	}

	var missing []Module
	if err := json.Unmarshal([]byte(raw), &missing); err != nil {
		return fmt.Errorf("spa: the inventory check answered %q, which is not the agreed "+
			"shape: %w", raw, err)
	}
	if len(missing) > 0 {
		return &ErrModulesMissing{Missing: missing}
	}
	return nil
}
