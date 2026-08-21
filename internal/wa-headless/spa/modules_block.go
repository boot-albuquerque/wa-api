package spa

// The blocking modules, measured with TestProbeBlockShape against the live
// build rather than copied from whatsapp-web.js. Two of the three facts below
// contradict what a reader would assume from the names.
const (
	// ModuleBlockContactAction holds blockContact and unblockContact, and
	// THEIR SIGNATURES DISAGREE — read from each function's own toString():
	//
	//	blockContact({bizOptOutArgs, blockEntryPoint, contact,
	//	              skipCtwa1pdNbfSignal})              // ONE object
	//	unblockContact(contact, blockEntryPoint)           // TWO positional
	//
	// This is the SECOND sibling pair in this module found to be asymmetric —
	// the first was addParticipantsJob/removeParticipantsJob (H58), where
	// assuming symmetry cost a live failure. Two independent occurrences is a
	// property of this codebase, not a coincidence: read both, always.
	//
	// Both take the CONTACT MODEL, not the wid and not the chat. The app
	// unproxies it internally, so a model straight from the collection is what
	// they expect.
	ModuleBlockContactAction = Module("WAWebBlockContactAction")

	// ModuleBlocklistCollection is what makes blocking VERIFIABLE. Without it
	// this capability could only report that the call returned, which is the
	// class of silent success the send verifier was built to refuse.
	ModuleBlocklistCollection = Module("WAWebBlocklistCollection")

	// ModuleBlockConstants carries BlockEntryPoint — the app's vocabulary for
	// WHERE a block was initiated, which it reports to its own telemetry. It is
	// not decorative: getBlockEventMetricFromBlockEntryPoint is called on it
	// before anything else happens, so a value outside the enum walks into that
	// call. The measured values are strings ("chat", "profile", "block_list",
	// …); this package uses the one for an ordinary chat.
	//
	// The name is the app's, misspelling included ("Contants").
	ModuleBlockConstants = Module("WAWebBlockContants")
)

// The editing modules, measured with TestProbeEditShape.
const (
	// ModuleSendMessageEditAction holds sendMessageEdit, and this one IS
	// readable from toString() because it is synchronous:
	//
	//	sendMessageEdit(msg, text, options)
	//
	// Its first act is to refuse: it rejects unless canEditText or
	// canEditCaption says yes. Consulting the same predicate before calling is
	// not politeness — it turns a rejected promise into an answer that names
	// the reason.
	ModuleSendMessageEditAction = Module("WAWebSendMessageEditAction")

	// ModuleMessageEditUtils carries the WINDOW, and the measured value is
	// 1200 seconds. That number is why the live proof has to SEND before it
	// edits: every message already in the collection was hours or days old and
	// canEditText was false for all of them.
	//
	// isParentWithinEditProcessingWindow lives here too and THREW when given a
	// message, so it is not the predicate it sounds like. canEditText is.
	ModuleMessageEditUtils = Module("WAWebMessageEditUtils")
)

// The starring modules, settled by EXPERIMENT rather than reading, because
// sendStarMsgs is an async wrapper whose toString() shows only that it discards
// its first argument (probe_star_test.go).
const (
	// ModuleCmdForStar is the layer that WORKS, and it is not the bridge the
	// name search suggested. Three shapes were tried one at a time against a
	// real message; the first moved the flag and the other two were never
	// needed:
	//
	//	Cmd.sendStarMsgs(chat, [msg], true)
	//	Cmd.sendUnstarMsgs(chat, [msg], true)
	//
	// Both take the CHAT MODEL and an ARRAY OF MESSAGE MODELS.
	ModuleCmdForStar = ModuleCmd

	// ModuleStarredMsgCollection is here to record a NEGATIVE result, which is
	// why it has no use anywhere in this package.
	//
	// PARIDADE-WWEBJS.md §6.14 listed it as the postcondition for starring, on
	// the strength of its name. The experiment measured it THROWING — the
	// probe's count came back -1 both before and after a star that demonstrably
	// worked. The real signal is msg.star on the model.
	//
	// The entry stays so the next reader does not spend the same probe
	// rediscovering that a plausible-sounding collection is the wrong question.
	ModuleStarredMsgCollection = Module("WAWebStarredMsgCollection")
)

// The muting modules, measured with TestProbeMuteShape.
const (
	// ModuleMuteCollection holds the Mute MODEL, and the model is the layer to
	// drive — not WAWebChatMuteBridge, which the enumeration nominated first.
	//
	// The bundle showed the bridge being called with an object carrying a key
	// named `$MuteImpl3`, and the model's own key list confirms what that is:
	// `$MuteImpl$p_4`, `$MuteImpl$p_5`, `$MuteImpl$p_6` are minifier artefacts
	// of private methods. Passing an artefact as a contract is the guess H58
	// paid for.
	//
	// The model's methods are SYNCHRONOUS, so their shape is fully legible:
	//
	//	mute({expiration, fromMultiselect, isAutoMuted, sendDevice, showToast, toastId})
	//	unmute({fromMultiselect, sendDevice, showToast, toastId})
	//
	// TWO THINGS IN THAT BODY DECIDE WHETHER THIS WORKS AT ALL:
	//
	//	sendDevice === true   is what makes it reach the bridge. Without it the
	//	                      change is LOCAL, and every postcondition still
	//	                      passes — a silent half-success by construction.
	//	expiration            must be a number or the call rejects with
	//	                      ActionError, and the app logs "wrong units?" above
	//	                      2e9, which is how it says EPOCH SECONDS.
	ModuleMuteCollection = Module("WAWebMuteCollection")

	// ModuleMuteExpirations converts hours to that epoch value, including the
	// sentinel for "always". Reimplementing it in Go would mean reimplementing
	// the sentinel, so the page's own function is used and its RESULT is
	// reported — the number is visible to the caller rather than hidden in a
	// decision the page made alone.
	ModuleMuteExpirations = Module("WAWebMuteExpirations")

	// ModuleMuteUtils carries canMute, which refuses this account's own chat
	// and non-member groups before anything is attempted.
	ModuleMuteUtils = Module("WAWebMuteUtils")
)
