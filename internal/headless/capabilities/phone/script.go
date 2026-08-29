package phone

import "strconv"

const (
	modPhoneUtils = "WAWebPhoneUtils"
	modFindCC     = "WAPhoneFindCC"

	// jidSuffix is what formattedPhoneNumber expects. The reference normalises to
	// it before asking, and so does this — but here the digits arrive already
	// stripped, so the suffix is appended rather than substituted.
	jidSuffix = "@s.whatsapp.net"
)

func lookupScript(digits string, key string) string {
	return `(() => {
	window[` + strconv.Quote(key) + `] = null;
	const park = v => { window[` + strconv.Quote(key) + `] = JSON.stringify(v); };
	const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 120);
	try {
		const d = ` + strconv.Quote(digits) + `;
		const f = window.require("` + modPhoneUtils + `").formattedPhoneNumber(d + ` + strconv.Quote(jidSuffix) + `);
		const c = window.require("` + modFindCC + `").findCC(d);
		park({
			ok: true,
			formatted: (typeof f === "string") ? f : "",
			cc: (typeof c === "string") ? c : "",
		});
	} catch (e) {
		park({ ok: false, why: safe(e) });
	}
	return "kicked";
	})()`
}
