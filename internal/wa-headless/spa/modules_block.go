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

// The forwarding module, measured from a REAL CALL SITE in the bundle
// (probe_forward_test.go plus a grep).
const (
	// ModuleForwardMessagesToChat is the layer the app's own forward flow
	// drives, and its shape could not be read from toString() — both exported
	// functions are async wrappers taking a single opaque `e`, and the chat
	// model has NO forward method at all, which is what sent this search to the
	// call sites:
	//
	//	forwardMessagesToChats({msgs, chats, includeCaption, appendedText})
	//	// and the lower one, for a single chat:
	//	forwardMessages({chat, msgs, multicast, includeCaption, appendedText})
	//
	// `chats` is an array of CHAT MODELS — the call site builds it from
	// findOrCreateLatestChat, not from ids. `msgs` is an array of MESSAGE
	// MODELS.
	//
	// It rejects with an error carrying `reasons`, which the app reads; this
	// package surfaces it rather than flattening it to "failed".
	ModuleForwardMessagesToChat = Module("WAWebForwardMessagesToChat")
)

// ModuleSetSubjectGroupAction renames a group. Its wrapper is SYNCHRONOUS, so
// the shape is exact:
//
//	setGroupSubject(chat, subject = "")
//
// The first argument is unproxied inside, which is this build's way of saying
// it wants a MODEL. The default of "" is the app's, not ours: this package
// refuses an empty subject rather than silently clearing a group's name.
const ModuleSetSubjectGroupAction = Module("WAWebSetSubjectGroupAction")

// ModuleExitGroupAction leaves a group: sendExitGroup(chat), one argument,
// unproxied inside, so a MODEL. Measured with TestProbeRemainingShapes.
//
// IT IS THE ONE CAPABILITY IN THIS MODULE WITH NO REVERSIBLE LIVE PROOF. An
// account that leaves a group it created cannot rejoin without an invite from
// somebody still inside, and the lab has two accounts. So it is built, unit
// tested, and deliberately never run against the lab group — recorded rather
// than quietly skipped.
const ModuleExitGroupAction = Module("WAWebExitGroupAction")

// Emptying and removing a conversation, measured with TestProbeRemainingShapes
// and confirmed at the app's own call sites.
const (
	// ModuleSendClearChatAction empties a conversation but keeps it in the
	// list: sendClear(chat, keepStarred). The second argument's meaning comes
	// from the UI that calls it — a checkbox beside the words "the conversation
	// will be empty but will stay in your list".
	ModuleSendClearChatAction = Module("WAWebSendClearChatAction")

	// ModuleDeleteChatAction removes the conversation itself:
	// sendDelete(chat, syncToDevices = true). The app's leave-group flow calls
	// sendExitGroup and then sendDelete, which is the order this package's
	// callers would need too — and is not something this package does on their
	// behalf.
	ModuleDeleteChatAction = Module("WAWebDeleteChatAction")

	// ModuleSetPushnameConnAction sets the account's display name:
	// setPushname(name, onDone). The second argument is a UI callback and is
	// omitted here.
	//
	// IT IS GUARDED BY THE BUILD, not by us: Conn.canSetMyPushname() is
	// !getIsSMB(this), and it measured FALSE on the lab account — which is
	// therefore a WhatsApp Business account. That is worth knowing beyond this
	// capability, because it may explain other behaviour measured on it.
	ModuleSetPushnameConnAction = Module("WAWebSetPushnameConnAction")

	// ModuleConnModel, which carries canSetMyPushname and the current
	// pushname, is already declared in modules.go.
)

// ModuleDownloadManager fetches and decrypts a message's media:
//
//	downloadManager.downloadAndMaybeDecrypt({signal, directPath, encFilehash,
//	                                         filehash, mediaKey,
//	                                         mediaKeyTimestamp, type})
//
// The shape came from the app's own call sites (history sync and a CSV export
// flow), which build it with WABase64-encoded hashes. Every field it needs is
// already on the message model.
//
// THIS ONE ALLOWS A CRYPTOGRAPHIC POSTCONDITION, which is rare here: filehash
// is the SHA-256 of the PLAINTEXT, so bytes that decrypt to something else fail
// a check that no amount of page weirdness can fake.
const ModuleDownloadManager = Module("WAWebDownloadManager")

// ModuleStartMediaDownloadQpl builds the performance-logging handle that
// downloadAndMaybeDecrypt REQUIRES. Omitting it throws
// "Cannot read properties of undefined (reading 'addAnnotations')" — telemetry
// plumbing presenting as a missing argument, which is the least guessable kind
// of required parameter there is.
const ModuleStartMediaDownloadQpl = Module("WAWebStartMediaDownloadQpl")

// ModuleFindCommonGroupsContactAction answers "which groups do this account and
// that contact both belong to". Its wrapper is SYNCHRONOUS and legible:
//
//	findCommonGroups(contact)   // the CONTACT MODEL
//
// Three things the body says that a caller would otherwise learn by accident:
//
//   - it returns null for THIS ACCOUNT's own contact, rather than an empty list
//     or a throw;
//   - it caches on the contact and reuses a pending promise, so asking twice is
//     cheap and asking a stale cache silently refilters it;
//   - it excludes parent (community) groups and locked ones, so the answer is
//     "groups you could talk in together", not "every group object shared".
const ModuleFindCommonGroupsContactAction = Module("WAWebFindCommonGroupsContactAction")

// ModulePollsSendPollCreationMsgAction creates a poll. Both its exports are
// async wrappers, so the shape came from the app's own call site:
//
//	const poll = createPollCreationMsgData({correctOptionKey, filteredOptions,
//	    isPhotoPoll, isSingleOption, pollEndTime, pollType, question,
//	    hideVoterNames});
//	await sendPollCreation({poll, chat, quotedMsg, isWamoSub});
//
// AN OPTION IS AN OBJECT, not a string: {name, localId}. That came from the
// merge path for added options, which builds exactly that shape and keys a Set
// on option.name.
const ModulePollsSendPollCreationMsgAction = Module("WAWebPollsSendPollCreationMsgAction")

// The "about" text — what this build calls a text status.
const (
	// ModuleTextStatusAction fetches one from the server. Its call sites are
	// MODULE-QUALIFIED, which is the check H69 cost two live runs for:
	//
	//	o("WAWebTextStatusAction").getTextStatus(contact.id)
	//
	// It takes the WID, not the model — unlike findCommonGroups next door,
	// which takes the model. There is no rule; there is only reading.
	ModuleTextStatusAction = Module("WAWebTextStatusAction")

	// ModuleTextStatusCollection is where the fetched value lands, found by
	// wid: TextStatusCollection.find(wid).
	ModuleTextStatusCollection = Module("WAWebTextStatusCollection")

	// ModuleTextStatusGatingUtils carries receiveTextStatusEnabled, which the
	// app checks before fetching at all. A build with the feature off would
	// otherwise look like a contact with no about text.
	ModuleTextStatusGatingUtils = Module("WAWebTextStatusGatingUtils")
)

// ModuleAck names the delivery states. The numbers are read FROM IT rather than
// written down here, because a constant copied from a blog post is a constant
// nobody can check.
//
// WHAT IS NOT AVAILABLE, measured: WAWebMsgInfoCollection is EMPTY in a fresh
// session (0 models against 368 sent messages), and MsgInfoCollection.get(id)
// returns nothing for every recent one. Per-participant delivery detail is
// populated when the app opens its message-info drawer, not before — so "who
// read it" is a different capability with a different cost, and msg.ack is what
// a session actually knows.
const ModuleAck = Module("WAWebAck")

// ModuleLabelCollection holds the account's labels. Labels are a WhatsApp
// Business feature, and the lab account measured as a business account (H66),
// which is the only reason this could be measured here at all.
//
// MEASURED: three labels exist with ids "1", "2" and "3" — the defaults — each
// with count 0, and no chat carries any. A chat's labels live on chat.labels as
// an array of id strings.
//
// WRITING IS NOT DONE HERE. editLabelAssociation(arg, chats) takes an array of
// CHAT MODELS as its second argument (it maps chat.id.toString()), and its first
// argument's shape is not readable from the wrapper. Guessing it is the mistake
// H69 charges for, so the write waits for a module-qualified call site.
const ModuleLabelCollection = Module("WAWebLabelCollection")
