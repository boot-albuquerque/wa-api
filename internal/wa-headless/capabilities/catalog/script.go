package catalog

import "strconv"

// The page side. Parked answer, synchronous outer function, no clock.

const (
	modCatalogBridge = "WAWebBizProductCatalogBridge"
	modWidFactory    = "WAWebWidFactory"
)

func catalogScript(sellerJID string) string {
	return `(() => {
	window.` + stateKey + ` = null;
	const park = v => { window.` + stateKey + ` = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 140);
	const num = v => (typeof v === "number" ? v : 0);
	const str = v => (typeof v === "string" ? v : "");
	(async () => {
		try {
			const W = window.require("` + modWidFactory + `");
			const Cat = window.require("` + modCatalogBridge + `");
			const wid = W.createWid(` + strconv.Quote(sellerJID) + `);

			// POSICIONAL, e isso foi medido em vez de copiado. A aridade
			// declarada e 10, e a referencia passa 2; o objeto de opcoes devolve
			// CatalogUnknownError enquanto a forma posicional CHEGA ao servidor.
			let r;
			try {
				r = await Cat.queryCatalog(wid);
			} catch (e) {
				// SEM VITRINE NAO E FALHA DA CHAMADA. O servidor recusa por
				// vendedor com ServerStatusCodeError — medido em dois de tres
				// vendedores, enquanto o terceiro devolveu um produto. Sem esse
				// terceiro, esta recusa seria indistinguivel de uma capacidade
				// quebrada.
				const m = String((e && e.message) || e);
				if (m.indexOf("ServerStatusCode") >= 0 || m.indexOf("CatalogUnknown") >= 0) {
					park({ ok: true, noShop: true });
					return;
				}
				park({ ok: false, why: safe(e) });
				return;
			}

			// OS ITENS VIVEM EM "data". "products" era o palpite herdado da
			// referencia; a resposta real e
			// {data, catalog_id, catalog_name, catalog_type, paging}.
			const items = (r && Array.isArray(r.data)) ? r.data : [];
			const products = items.map(p => {
				const imgs = (p && Array.isArray(p.image_cdn_urls)) ? p.image_cdn_urls.length : 0;
				const extra = (p && Array.isArray(p.additional_image_cdn_urls))
					? p.additional_image_cdn_urls.length : 0;
				return {
					id: str(p && p.id),
					retailerId: str(p && p.retailer_id),
					name: str(p && p.name),
					description: str(p && p.description),
					// INTEIRO CRU, na menor unidade da moeda. Dividir aqui pelo
					// expoente errado e um defeito que so aparece quando alguem
					// e cobrado.
					price: num(p && p.price),
					currency: str(p && p.currency),
					hidden: !!(p && p.is_hidden),
					availability: str(p && p.availability),
					images: imgs + extra,
				};
			});

			// PAGINA E REPORTADA, NAO SEGUIDA. Seguir em silencio transforma uma
			// leitura em um numero indeterminado delas.
			let more = false;
			try {
				const pg = r && r.paging;
				more = !!(pg && (pg.cursors && (pg.cursors.after || pg.cursors.before)) || (pg && pg.next));
			} catch (e) {}

			park({
				ok: true, noShop: false,
				id: str(r && r.catalog_id),
				name: str(r && r.catalog_name),
				kind: str(r && r.catalog_type),
				more: more,
				products: products,
			});
		} catch (e) {
			park({ ok: false, why: safe(e) });
		}
	})();
	return "kicked";
	})()`
}
