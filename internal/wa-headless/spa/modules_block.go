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
