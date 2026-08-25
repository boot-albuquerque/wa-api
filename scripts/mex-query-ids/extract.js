// Extracts the WhatsApp Web `mex` GraphQL query IDs from the live JS bundles.
//
// WHY THIS EXISTS
// ---------------
// Every `mex` operation (newsletter follow, create, delete, demote admin, ...)
// is addressed by a numeric `query_id`. WhatsApp ROTATES those IDs without
// notice. When an ID is retired the server answers
//
//     graphql error: 400 Bad Request (CRITICAL)
//
// and the operation stops working while everything else keeps going. That is
// the symptom this script answers.
//
// The IDs cannot be read off the wire: `mex` travels inside the Noise-encrypted
// WebSocket. They CAN be read from the JS bundle, which is served in the clear.
// This is the same method the Baileys and whatsmeow authors use.
//
// HOW TO RUN
// ----------
//   1. Open https://web.whatsapp.com and sign in (a linked device is enough).
//   2. Open DevTools -> Console.
//   3. Paste this whole file and press Enter.
//   4. Copy the printed table.
//
// It only reads: it fetches the page's own <script src> URLs and searches them.
// It sends nothing and changes nothing.
//
// DO NOT copy IDs from another client (Baileys, wwebjs) without checking them
// here first. Measured 2026-08-25: Baileys' demote/change-owner IDs were stale
// and produced the 400 above, while its delete ID matched the bundle exactly.
// Same-looking sources are not interchangeable.
(async () => {
  const scripts = [...document.querySelectorAll('script[src]')]
    .map((s) => s.src)
    .filter((u) => u.startsWith('http'));

  // Matches names like WAWebMexDeleteNewsletterJobMutation.graphql. Widen the
  // inner group to cover surfaces other than newsletter.
  const NAME = /WAWebMex(\w+?)(?:Job)?(?:Mutation|Query)\.graphql/g;
  const ID = /"(\d{15,18})"/g;
  const WINDOW = 1200; // chars around the name; the ID sits close by

  const found = new Map();

  for (const url of scripts) {
    let body;
    try {
      body = await fetch(url).then((r) => r.text());
    } catch {
      continue; // a blocked or expired bundle is not fatal
    }
    for (const m of body.matchAll(NAME)) {
      const around = body.slice(
        Math.max(0, m.index - WINDOW),
        m.index + WINDOW,
      );
      const ids = [...new Set([...around.matchAll(ID)].map((x) => x[1]))];
      if (!ids.length) continue;
      const key = m[0].replace('.graphql', '');
      const seen = found.get(key) ?? new Set();
      ids.forEach((i) => seen.add(i));
      found.set(key, seen);
    }
  }

  const rows = [...found.entries()]
    .map(([name, ids]) => ({ name, ids: [...ids].join(' | ') }))
    .sort((a, b) => a.name.localeCompare(b.name));

  console.table(rows);

  // CAVEAT, and it matters: a name may print MORE THAN ONE id. The window is a
  // heuristic — neighbouring operations bleed into it. When that happens, widen
  // WINDOW, or confirm the candidate against the server: the wrong id answers
  // 400 Bad Request, the right one answers the operation's own error.
  //
  // Also: a working id is not proof the bundle's id is the only valid one. On
  // 2026-08-25 our `follow` (9926858900719341) worked in the field while the
  // bundle carried 24404358912487870 for the same operation. WhatsApp appears
  // to keep older generations alive. Do not "fix" an id that works.
  return rows;
})();
