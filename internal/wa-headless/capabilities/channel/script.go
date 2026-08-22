package channel

import "strconv"

// The page side. Parked answer, synchronous outer function, no clock.

const modMetadataQuery = "WAWebNewsletterMetadataQueryJob"

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
			const nameM = mix("newsletterNameMetadataMixin");
			const subsM = mix("newsletterSubscribersMetadataMixin");
			const stateM = mix("newsletterStateMetadataMixin");
			const verM = mix("newsletterVerificationMetadataMixin");
			const timeM = mix("newsletterCreationTimeMetadataMixin");
			const inviteM = mix("newsletterInviteLinkMetadataMixin");
			const picM = mix("newsletterPictureMetadataMixin");
			const descM = mix("newsletterDescriptionMetadataMixin");

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
				member: !!mix("newsletterMembershipMetadataMixin"),
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
