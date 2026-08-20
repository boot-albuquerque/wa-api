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
