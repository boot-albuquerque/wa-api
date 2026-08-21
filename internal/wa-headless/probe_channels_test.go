package waheadless

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeChannels measures the Channel/newsletter surface before anything is
// designed. The whole family is MISSING in LEDGER-WWEBJS.md (16 items) and has
// never been attacked, so there is no prior HOUSEKEEP entry to build on.
//
// UPSTREAM READ (whatsapp-web.js @ f935b500117e264c2b3abc25b63a280bd98182a7):
// src/structures/Channel.js and src/Client.js drive every channel operation
// through window.require('WAWebNewsletterCollection').WAWebNewsletterCollection
// plus a family of single-purpose action/job modules (edit metadata, mute,
// admin invite, create, delete, transfer ownership, subscribe). None of it
// goes through a generic chat path — "newsletter" is the wire word, "channel"
// is only the product name, matching H100's lesson about "broadcast" not
// being what it sounds like. src/util/Injected/Utils.js (WWebJS.getChannels,
// WWebJS.getChatModel) is the reference's OWN injected page code, which this
// build does not have — window.WWebJS is a wwebjs invention, not a WhatsApp
// module, so this probe reads WAWebNewsletterCollection directly instead.
//
// READ ONLY. No channel is created, followed, muted, or posted to. Every
// value-returning call below is either a pure predicate (isNewsletterCreationEnabled,
// getMaxSubscriberNumber) or a plain module/arity inspection. Nothing here
// sends a stanza.
//
// PII: counts, shapes, and field NAMES only. Never a jid, channel name,
// invite code, or message text. Error strings are redacted before parking.
func TestProbeChannels(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHANNELS") == "" {
		t.Skip("set WA_PROBE_CHANNELS=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	const channelsProbeScript = `(() => {
		window.__channels = null;
		const park = v => { window.__channels = JSON.stringify(v); };
		const redact = s => String(s).replace(/[0-9]{5,}/g, '<redacted>').slice(0, 160);
		(async () => {
		const out = { modules: {}, arity: {}, collection: {}, model: {}, metadata: {} };

		// WHICH MODULES FROM THE REFERENCE'S CHANNEL/NEWSLETTER PATH EXIST HERE,
		// AND WHAT THEY EXPORT. This is the tax probe_modmap.go's comment names:
		// guessing names from a foreign build and finding out which survive.
		const load = name => {
			try {
				const m = window.require(name);
				out.modules[name] = m ? Object.keys(m).slice(0, 40) : 'falsy';
				return m;
			} catch (e) {
				out.modules[name] = 'ABSENT: ' + redact(e && e.message);
				return null;
			}
		};

		const NewsletterColl = load('WAWebNewsletterCollection');
		const Collections = load('WAWebCollections');
		const Gating = load('WAWebNewsletterGatingUtils');
		const FetchSubs = load('WAWebMexFetchNewsletterSubscribersJob');
		const SubListAction = load('WAWebNewsletterSubscriberListAction');
		const EditMeta = load('WAWebEditNewsletterMetadataAction');
		const UpdateUserSetting = load('WAWebNewsletterUpdateUserSettingJob');
		const ModelUtils = load('WAWebNewsletterModelUtils');
		const Jids = load('WAJids');
		const AcceptInvite = load('WAWebMexAcceptNewsletterAdminInviteJob');
		const RevokeInvite = load('WAWebMexRevokeNewsletterAdminInviteJob');
		const DemoteAdmin = load('WAWebDemoteNewsletterAdminAction');
		const SendAdminInvite = load('WAWebNewsletterSendMsgAction');
		const SendMessageJob = load('WAWebNewsletterSendMessageJob');
		const CreateQuery = load('WAWebNewsletterCreateQueryJob');
		const ChatGetters = load('WAWebChatGetters');
		const UpdateMsgsRecords = load('WAWebNewsletterUpdateMsgsRecordsJob');
		const LoadPreview = load('WAWebLoadNewsletterPreviewChatAction');
		const MetaQuery = load('WAWebNewsletterMetadataQueryJob');
		const Subscribe = load('WAWebNewsletterSubscribeAction');
		const Unsubscribe = load('WAWebNewsletterUnsubscribeAction');
		const DeleteAction = load('WAWebNewsletterDeleteAction');
		const ChangeOwner = load('WAWebChangeNewsletterOwnerAction');
		const DemoteAdminJob = load('WAWebNewsletterDemoteAdminJob');
		const UpdateMsgsRecordsMeta = load('WAWebLidMigrationUtils');

		// ARITY of every function the reference actually calls, so a design
		// later can compare "what we pass" against "what it expects" the way
		// the calls probe did for createEventCallLink.
		const arityOf = (mod, fn) => {
			try { return (mod && typeof mod[fn] === 'function') ? mod[fn].length : (mod ? 'not a function: ' + typeof mod[fn] : 'module absent'); }
			catch (e) { return 'threw'; }
		};
		out.arity = {
			'WAWebNewsletterGatingUtils.getMaxSubscriberNumber': arityOf(Gating, 'getMaxSubscriberNumber'),
			'WAWebNewsletterGatingUtils.isNewsletterCreationEnabled': arityOf(Gating, 'isNewsletterCreationEnabled'),
			'WAWebMexFetchNewsletterSubscribersJob.mexFetchNewsletterSubscribers': arityOf(FetchSubs, 'mexFetchNewsletterSubscribers'),
			'WAWebNewsletterSubscriberListAction.getSubscribersInContacts': arityOf(SubListAction, 'getSubscribersInContacts'),
			'WAWebEditNewsletterMetadataAction.editNewsletterMetadataAction': arityOf(EditMeta, 'editNewsletterMetadataAction'),
			'WAWebNewsletterUpdateUserSettingJob.updateNewsletterUserSetting': arityOf(UpdateUserSetting, 'updateNewsletterUserSetting'),
			'WAJids.toNewsletterJid': arityOf(Jids, 'toNewsletterJid'),
			'WAWebMexAcceptNewsletterAdminInviteJob.acceptNewsletterAdminInvite': arityOf(AcceptInvite, 'acceptNewsletterAdminInvite'),
			'WAWebMexRevokeNewsletterAdminInviteJob.revokeNewsletterAdminInvite': arityOf(RevokeInvite, 'revokeNewsletterAdminInvite'),
			'WAWebDemoteNewsletterAdminAction.demoteNewsletterAdminAction': arityOf(DemoteAdmin, 'demoteNewsletterAdminAction'),
			'WAWebNewsletterSendMsgAction.sendNewsletterAdminInviteMessage': arityOf(SendAdminInvite, 'sendNewsletterAdminInviteMessage'),
			'WAWebNewsletterSendMessageJob.sendNewsletterMessageJob': arityOf(SendMessageJob, 'sendNewsletterMessageJob'),
			'WAWebNewsletterCreateQueryJob.createNewsletterQuery': arityOf(CreateQuery, 'createNewsletterQuery'),
			'WAWebChatGetters.getIsNewsletter': arityOf(ChatGetters, 'getIsNewsletter'),
			'WAWebNewsletterUpdateMsgsRecordsJob.addNewsletterMsgsRecords': arityOf(UpdateMsgsRecords, 'addNewsletterMsgsRecords'),
			'WAWebNewsletterUpdateMsgsRecordsJob.updateNewsletterMsgRecord': arityOf(UpdateMsgsRecords, 'updateNewsletterMsgRecord'),
			'WAWebLoadNewsletterPreviewChatAction.loadNewsletterPreviewChat': arityOf(LoadPreview, 'loadNewsletterPreviewChat'),
			'WAWebNewsletterMetadataQueryJob.queryNewsletterMetadataByInviteCode': arityOf(MetaQuery, 'queryNewsletterMetadataByInviteCode'),
			'WAWebNewsletterSubscribeAction.subscribeToNewsletterAction': arityOf(Subscribe, 'subscribeToNewsletterAction'),
			'WAWebNewsletterUnsubscribeAction.unsubscribeFromNewsletterAction': arityOf(Unsubscribe, 'unsubscribeFromNewsletterAction'),
			'WAWebNewsletterDeleteAction.deleteNewsletterAction': arityOf(DeleteAction, 'deleteNewsletterAction'),
			'WAWebChangeNewsletterOwnerAction.changeNewsletterOwnerAction': arityOf(ChangeOwner, 'changeNewsletterOwnerAction'),
			'WAWebNewsletterDemoteAdminJob.demoteNewsletterAdmin': arityOf(DemoteAdminJob, 'demoteNewsletterAdmin'),
			'WAWebLidMigrationUtils.toPn': arityOf(UpdateMsgsRecordsMeta, 'toPn'),
		};

		// THE CONSTANTS the reference reads off WAWebNewsletterModelUtils for
		// mute/unmute — safe to report because they are protocol enum values,
		// not identity.
		if (ModelUtils) {
			out.model.constants = {};
			for (const k of ['ADMIN_NOTIFICATIONS', 'MUTED_STATE', 'UNMUTED_STATE']) {
				try { out.model.constants[k] = (k in ModelUtils) ? typeof ModelUtils[k] + ':' + String(ModelUtils[k]) : 'absent'; }
				catch (e) { out.model.constants[k] = 'threw'; }
			}
		}

		// THE COLLECTION ITSELF. The reference reaches it as
		// WAWebNewsletterCollection.WAWebNewsletterCollection (a module whose
		// default export shares its own name) OR bundled under
		// WAWebCollections.NewsletterMetadataCollection /
		// WAWebCollections.WAWebNewsletterMetadataCollection. Try every door
		// this build might use and report which one answers.
		try {
			const candidates = {};
			if (NewsletterColl) {
				candidates['WAWebNewsletterCollection.WAWebNewsletterCollection'] = NewsletterColl.WAWebNewsletterCollection;
				candidates['WAWebNewsletterCollection.default'] = NewsletterColl.default;
				candidates['WAWebNewsletterCollection<module>'] = (typeof NewsletterColl.getModelsArray === 'function') ? NewsletterColl : null;
			}
			if (Collections) {
				candidates['WAWebCollections.NewsletterMetadataCollection'] = Collections.NewsletterMetadataCollection;
				candidates['WAWebCollections.WAWebNewsletterMetadataCollection'] = Collections.WAWebNewsletterMetadataCollection;
			}
			out.collection.doors = {};
			let holder = null, holderName = null;
			for (const [name, cand] of Object.entries(candidates)) {
				const present = !!cand;
				out.collection.doors[name] = present ? 'present' : 'absent';
				if (present && !holder && typeof cand.getModelsArray === 'function') {
					holder = cand; holderName = name;
				}
			}
			out.collection.chosen = holderName || 'none had getModelsArray';
			if (holder) {
				const models = holder.getModelsArray();
				out.collection.modelCount = models.length;
				out.collection.hasGet = typeof holder.get === 'function';
				out.collection.hasFind = typeof holder.find === 'function';
				out.collection.hasOn = typeof holder.on === 'function';
				if (models.length > 0) {
					const first = models[0];
					// FIELD NAMES ONLY. Never the values — a channel name or
					// jid would be a value here, and PII rule is absolute.
					out.model.ownKeys = Object.keys(first).slice(0, 60);
					// THE __x_ GETTER PATTERN. H51 paid for reading a raw
					// field instead of the getter this build hides it behind;
					// report both so a later design knows which is safe.
					const proto = Object.getPrototypeOf(first) || {};
					out.model.protoGetters = Object.getOwnPropertyNames(proto)
						.filter(k => {
							try {
								const d = Object.getOwnPropertyDescriptor(proto, k);
								return d && typeof d.get === 'function';
							} catch (e) { return false; }
						}).slice(0, 60);
					out.model.rawUnderscoreXFields = Object.keys(first).filter(k => k.startsWith('__x_')).length;
					if (typeof first.serialize === 'function') {
						try {
							const serialized = first.serialize();
							out.model.serializedKeys = Object.keys(serialized).slice(0, 60);
						} catch (e) { out.model.serializedKeys = 'threw: ' + redact(e && e.message); }
					} else {
						out.model.serializedKeys = 'no serialize()';
					}
				}
			}
		} catch (e) {
			out.collection.error = redact(e && e.message);
		}
		park(out);

		// THE TWO PURE PREDICATES the reference gates channel creation and
		// subscriber-limit behind. Neither writes anything.
		try {
			out.gating = {};
			if (Gating && typeof Gating.isNewsletterCreationEnabled === 'function') {
				out.gating.isNewsletterCreationEnabled = String(Gating.isNewsletterCreationEnabled());
			}
			if (Gating && typeof Gating.getMaxSubscriberNumber === 'function') {
				out.gating.getMaxSubscriberNumber = String(Gating.getMaxSubscriberNumber());
			}
		} catch (e) {
			out.gating = 'threw: ' + redact(e && e.message);
		}
		park(out);

		out.finished = true;
		park(out);
		})();
		return 'kicked';
	})()`

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/channels-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, channelsProbeScript, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}

	// PARK-AND-POLL, LAST VALUE WINS. If a later step throws, everything
	// measured before it still survives in window.__channels.
	var raw string
	for i := 0; i < 60; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/channels-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__channels || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(raw, `"finished":true`) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if raw == "" {
		t.Fatal("the probe never parked anything at all")
	}
	if !strings.Contains(raw, `"finished":true`) {
		t.Logf("the probe did NOT finish; whatever is missing below is the call that hung")
	}
	t.Logf("%s", raw)
}
