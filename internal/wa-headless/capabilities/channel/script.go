package channel

import "strconv"

// The page side. Parked answer, synchronous outer function, no clock.

const (
	modMetadataQuery = "WAWebNewsletterMetadataQueryJob"
	// modDirectorySearch is the channel directory. Measured answering on this
	// build 2026-08-22 with 50 results, unlike its neighbour
	// getRecommendedNewsletters, which hangs and is BLOCKED.
	modDirectorySearch = "WAWebNewsletterDirectorySearchAction"
	modL10N            = "WAWebL10N"
)

// The mixin names, in ONE place.
//
// They were inline literals in byInviteScript until the directory search needed
// the same vocabulary. Two copies of "newsletterNameMetadataMixin" are the same
// bug waiting to diverge, which is what ADR-0004 exists to prevent — and a
// divergence here would be invisible, because a missing mixin reads as an empty
// field rather than as an error.
const (
	mixName        = "newsletterNameMetadataMixin"
	mixSubscribers = "newsletterSubscribersMetadataMixin"
	mixState       = "newsletterStateMetadataMixin"
	mixVerify      = "newsletterVerificationMetadataMixin"
	mixTime        = "newsletterCreationTimeMetadataMixin"
	mixInvite      = "newsletterInviteLinkMetadataMixin"
	mixPicture     = "newsletterPictureMetadataMixin"
	mixDescription = "newsletterDescriptionMetadataMixin"
	mixMembership  = "newsletterMembershipMetadataMixin"

	// fieldNewsletterMetadata is where a DIRECTORY result keeps the mixins. A
	// metadata query answers with the mixins at the top level; a directory result
	// is a live chat model that carries them one level down, under __x_-prefixed
	// state. Measured 2026-08-22.
	fieldNewsletterMetadata = "__x_newsletterMetadata"

	// The fields a DIRECTORY result actually carries, measured 2026-08-22 over
	// 50 results. They are NOT the mixin names: those belong to the metadata
	// query, and reading them here returns empty for every channel.
	//
	// __x_state is deliberately absent from this list: it measured undefined on
	// 50 of 50, so there is no state to report and inventing one from
	// __x_suspended (an object, not a boolean) would be guessing.
	fieldName         = "__x_name"           // string, 50/50
	fieldDescription  = "__x_description"    // string, 50/50
	fieldSize         = "__x_size"           // number, 50/50 — the subscriber count
	fieldVerified     = "__x_verified"       // boolean, 50/50
	fieldMembership   = "__x_membershipType" // string, 50/50, e.g. "guest"
	fieldCreationTime = "__x_creationTime"   // number, 50/50
)

func byInviteScript(code string) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 150);
	const str = v => (typeof v === "string" ? v : "");
	const num = v => (typeof v === "number" ? v : 0);
	(async () => {
		try {
			const Q = window.require("` + modMetadataQuery + `");
			let r;
			try {
				// UM ARGUMENTO. A aridade declarada e 2, e a forma de um
				// argumento resolve — medido contra um canal real que esta conta
				// NAO segue.
				r = await Q.queryNewsletterMetadataByInviteCode(` + strconv.Quote(code) + `);
			} catch (e) {
				const m = String((e && e.message) || e);
				if (m.indexOf("NotFound") >= 0 || m.indexOf("not found") >= 0 ||
					m.indexOf("InvalidInvite") >= 0) {
					park({ ok: true, notFound: true });
					return;
				}
				park({ ok: false, why: safe(e) });
				return;
			}
			if (!r || typeof r !== "object") { park({ ok: true, notFound: true }); return; }

			// OS VALORES VIVEM NOS MIXINS. Nao existe campo "name" no topo:
			// existe newsletterNameMetadataMixin.nameElementValue. Ler o campo
			// obvio devolve undefined para sempre — quarta ocorrencia desta
			// classe neste repositorio.
			const mix = (k) => { try { return r[k] || null; } catch (e) { return null; } };
			const nameM = mix("` + mixName + `");
			const subsM = mix("` + mixSubscribers + `");
			const stateM = mix("` + mixState + `");
			const verM = mix("` + mixVerify + `");
			const timeM = mix("` + mixTime + `");
			const inviteM = mix("` + mixInvite + `");
			const picM = mix("` + mixPicture + `");
			const descM = mix("` + mixDescription + `");

			// A DESCRICAO ESTA UM NIVEL MAIS FUNDO ainda, dentro do seu proprio
			// mixin de resposta.
			let desc = "";
			try {
				const inner = descM && descM.descriptionQueryDescriptionResponseMixin;
				desc = str(inner && (inner.descriptionElementValue || inner.description));
			} catch (e) {}

			const id = r.idJid || r.id;
			park({
				ok: true, notFound: false,
				jid: (id && id._serialized) ? id._serialized : str(id),
				code: str(inviteM && inviteM.inviteCode),
				name: str(nameM && nameM.nameElementValue),
				description: desc,
				subscribers: num(subsM && subsM.subscribersCount),
				state: str(stateM && stateM.stateType),
				verification: str(verM && verM.verificationState),
				createdAt: num(timeM && timeM.creationTimeValue),
				// MEMBRO E DERIVADO DE O MIXIN EXISTIR. Ele vem null para um
				// canal que esta conta nao segue, e foi exatamente isso que
				// permitiu provar este leitor sem seguir nada.
				member: !!mix("` + mixMembership + `"),
				// A URL DA FOTO NAO CRUZA: e grande, expira, e ninguem aqui
				// precisa dela.
				picture: !!(picM && picM.picture),
			});
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}

// searchScript asks the channel directory.
//
// TWO DELIBERATE DIVERGENCES FROM THE REFERENCE, both measured:
//
//  1. NO LIMIT OPTION. The reference implements `limit` by MONKEY-PATCHING
//     WAWebNewsletterGatingUtils.getNewsletterDirectoryPageSize and restoring it
//     afterwards. That function DOES NOT EXIST on this build (measured
//     2026-08-22: pageSizeFn=false), so the patch would silently do nothing —
//     and even where it works, patching a page global means a failed restore
//     leaves the page altered for every later caller. The page's own page size
//     is accepted and reported instead.
//
//  2. THE MIXINS ARE ONE LEVEL DOWN. A metadata query answers with the mixins at
//     the top level; a directory result is a live chat model carrying them under
//     __x_newsletterMetadata. Reading the top level here would return empty
//     fields for every channel — an empty result that looks like an empty
//     directory rather than like a bug, which is the failure mode this whole
//     family keeps producing.
func searchScript(query, region string, skipSubscribed bool) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 150);
	const str = v => (typeof v === "string" ? v : "");
	const num = v => (typeof v === "number" ? v : 0);
	(async () => {
		try {
			const DIR = window.require("` + modDirectorySearch + `");
			let region = ` + strconv.Quote(region) + `;
			if (!region) {
				try { region = window.require("` + modL10N + `").getRegion(); } catch (e) {}
			}
			const res = await DIR.fetchNewsletterDirectories({
				searchText: ` + strconv.Quote(query) + `,
				countryCodes: region ? [region] : [],
				skipSubscribedNewsletters: ` + strconv.FormatBool(skipSubscribed) + `,
				view: "RECOMMENDED",
				categories: [],
				cursorToken: "",
			});
			const list = (res && res.newsletters) || [];
			const out = [];
			for (const n of list) {
				// UM RESULTADO DE DIRETORIO E' UM MODELO, NAO O SACO DE MIXINS.
				// A consulta de metadados devolve newsletterNameMetadataMixin e
				// companhia; aqui os valores vivem em campos __x_ do proprio
				// modelo. Medido 2026-08-22 sobre 50 resultados: __x_size numero
				// em 50/50, __x_verified booleano em 50/50, __x_membershipType
				// string em 50/50 — e __x_state UNDEFINED em 50/50, motivo por
				// que nao existe campo de estado aqui.
				const md = (n && n["` + fieldNewsletterMetadata + `"]) || {};
				const id = md.id || (n && n.id);
				out.push({
					jid: (id && id._serialized) ? id._serialized : str(id),
					name: str(md.` + fieldName + `) || str(n && n.name),
					description: str(md.` + fieldDescription + `),
					subscribers: num(md.` + fieldSize + `),
					verified: !!md.` + fieldVerified + `,
					membership: str(md.` + fieldMembership + `),
					createdAt: num(md.` + fieldCreationTime + `),
				});
			}
			park({ ok: true, results: out, returned: list.length });
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}

// The owner-side page modules and the edit flags. Measured present on this build
// 2026-08-22 with the arities noted; getMaxSubscriberNumber exists but the job
// that would USE it does not, which is why there is no subscriber reader here.
const (
	modCreate = "WAWebNewsletterCreateQueryJob" // createNewsletterQuery, arity 1
	modDelete = "WAWebNewsletterDeleteAction"   // deleteNewsletterAction, arity 1
	modEdit   = "WAWebEditNewsletterMetadataAction"
	modGating = "WAWebNewsletterGatingUtils"
	// modCollections is where the newsletter collection LIVES. It is not its own
	// module: WAWebNewsletterCollection is a member of WAWebCollections, and
	// requiring it by name returns undefined. Measured 2026-08-22 the hard way —
	// the first version asked WAWebChatCollection, which holds 384 chats and ZERO
	// newsletters, so every owner operation failed with "channel not loaded" on a
	// channel that had just been created successfully (H113).
	modCollections  = "WAWebCollections"
	collNewsletters = "WAWebNewsletterCollection"

	// editName and editDescription are the property flags the edit action takes.
	// They are constants because they are two halves of one vocabulary and a typo
	// in either would read as "the server ignored us".
	editName        = "editName"
	editDescription = "editDescription"
)

// createScript makes a channel and parks what the server said.
//
// THE GATE IS CHECKED FIRST AND REPORTED AS ITS OWN ANSWER. The reference
// returns the string 'CreateChannelError: A channel creation is not enabled',
// which a caller has to pattern-match; here it is a flag that becomes a distinct
// Go error.
func createScript(name, description string) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 150);
	const str = v => (typeof v === "string" ? v : "");
	const num = v => (typeof v === "number" ? v : 0);
	(async () => {
		try {
			if (!window.require("` + modGating + `").isNewsletterCreationEnabled()) {
				park({ ok: false, disabled: true });
				return;
			}
			const r = await window.require("` + modCreate + `").createNewsletterQuery({
				name: ` + strconv.Quote(name) + `,
				description: ` + strconv.Quote(description) + `,
				picture: null,
			});
			// A RESPOSTA DA CRIACAO USA OS MIXINS, nao os campos __x_ do
			// diretorio: e' uma resposta de consulta, nao um modelo vivo.
			const inviteM = (r && r["` + mixInvite + `"]) || null;
			const timeM = (r && r["` + mixTime + `"]) || null;
			const jid = r && r.idJid;
			park({
				ok: true, disabled: false,
				jid: (jid && jid._serialized) ? jid._serialized : str(jid),
				code: str(inviteM && inviteM.inviteCode),
				at: num(timeM && timeM.creationTimeValue),
			});
		} catch (e) {
			park({ ok: false, disabled: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}

// channelLookup finds the live chat model for a channel jid.
//
// The reference uses its own injected window.WWebJS.getChat helper; this stack
// does not inject anything (that row is an INTENTIONAL_DIFFERENCE), so the
// collection is asked directly.
const channelLookup = `
		const NC = window.require("` + modCollections + `").` + collNewsletters + `;
		let ch = null;
		try { ch = NC.get(JID); } catch (e) {}
		if (!ch) {
			const all = typeof NC.getModelsArray === "function" ? NC.getModelsArray() : [];
			for (const c of all) {
				try {
					const id = c.id;
					const s = (id && id._serialized) ? id._serialized : String(id);
					if (s === JID) { ch = c; break; }
				} catch (e) {}
			}
		}
		if (!ch) { park({ ok: false, why: "channel not loaded in this session" }); return; }
`

func editScript(jid, field, value string) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 150);
	(async () => {
		try {
			const JID = ` + strconv.Quote(jid) + `;` + channelLookup + `
			const property = {}; property[` + strconv.Quote(field) + `] = true;
			const value = {};
			value[` + strconv.Quote(fieldForFlag(field)) + `] = ` + strconv.Quote(value) + `;
			await window.require("` + modEdit + `").editNewsletterMetadataAction(ch, property, value);
			park({ ok: true });
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}

// fieldForFlag maps an edit FLAG to the value KEY it goes with. They differ —
// the flag is editName and the value key is name — and keeping the mapping in
// one function is what stops the two halves from drifting apart at a call site.
func fieldForFlag(flag string) string {
	switch flag {
	case editName:
		return "name"
	case editDescription:
		return "description"
	default:
		return flag
	}
}

func deleteScript(jid string) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 150);
	(async () => {
		try {
			const JID = ` + strconv.Quote(jid) + `;` + channelLookup + `
			await window.require("` + modDelete + `").deleteNewsletterAction(ch);
			park({ ok: true });
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}
