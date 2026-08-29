package headless

import (
	"os"
	"strings"
	"testing"
)

// readHousekeep reads the file the gate reads, from the same path constant, so
// this test cannot drift onto a different copy.
func readHousekeep(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(housekeepPath)
	if err != nil {
		t.Fatalf("read %s: %v", housekeepPath, err)
	}
	return string(b)
}

// TestTheStatusPatternMatchesStatusesAndNothingElse tests the pattern the gate
// actually uses — housekeepStatusLine — rather than a copy of it. A copy would
// be a double that cannot diverge from the thing it imitates by construction,
// and the first entry in ARMADILHAS.md is about doubles that are more permissive
// than production.
//
// The negatives are the cases that were MEASURED failing, not invented ones:
// the word "Status" inside receiveTextStatusEnabled and getTextStatus, and prose
// whose colon lives lines away from it.
func TestTheStatusPatternMatchesStatusesAndNothingElse(t *testing.T) {
	positives := []struct {
		name, in, want string
	}{
		{"plain", "**Status**: entregue.", "entregue."},
		{"no bold", "Status: corrigido nesta sessão.", "corrigido nesta sessão."},
		{"indented", "  **Status**: aberto.", "aberto."},
		{"bulleted", "- **Status**: não entregue.", "não entregue."},
		{"quoted", "> **Status**: entregue.", "entregue."},
		{"qualified label", "**Status atual**: parcialmente entregue.", "parcialmente entregue."},
		{"after a separator", "**Status**: · **entregue**", "entregue"},
		{"inside a document", "prosa qualquer\n\n**Status**: entregue, com ressalvas.\n\nmais prosa",
			"entregue, com ressalvas."},
	}
	for _, tc := range positives {
		t.Run("match/"+tc.name, func(t *testing.T) {
			m := housekeepStatusLine.FindStringSubmatch(tc.in)
			if m == nil {
				t.Fatalf("no match on a real status line: %q", tc.in)
			}
			got := strings.Trim(strings.TrimSpace(m[1]), " ·*—-–:")
			if got != strings.Trim(tc.want, " ·*—-–:") {
				t.Fatalf("captured %q, want %q", got, tc.want)
			}
		})
	}

	negatives := []struct{ name, in string }{
		// THE TWO THAT ACTUALLY HAPPENED, on 2026-08-21, hours apart.
		{"receiveTextStatusEnabled with a distant colon",
			"`receiveTextStatusEnabled()` é consultada ANTES. Um build com o recurso\n" +
				"desligado seria indistinguível de um contato que não escreveu nada.\n\n" +
				"### O PASS ao vivo\n\n```\npeer about:  contacts.About(len=0)\n```"},
		{"getTextStatus with a distant colon",
			"`WAWebTextStatusAction`, que exporta `getTextStatus` e `setMyTextStatus`.\n" +
				"A ESCRITA continua bloqueada, por um motivo mais duro:\n\n```\nsetMyTextStatus(e, t)\n```"},

		// The shape of the bug, isolated: the label must not run onto another
		// line to find its colon.
		{"label split across lines", "**Status**\ncoisa qualquer: entregue."},
		{"Status mid-word, same line", "o campo textStatus: vazio"},
		{"Status inside an identifier in a code fence",
			"```\nconst s = getTextStatus(wid);\n```\n\nalguma prosa: com dois-pontos"},
	}
	for _, tc := range negatives {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if m := housekeepStatusLine.FindStringSubmatch(tc.in); m != nil {
				t.Fatalf("matched something that is not a status: captured %q from\n%s", m[1], tc.in)
			}
		})
	}
}

// TestTheEmphasisWorkaroundIsGone. The old pattern was survivable only by
// wrapping identifiers in ** so the "*" would stop the scan — a tax on anyone
// writing "Status" in an entry. Once the pattern is anchored the workaround must
// not linger, or the next reader will copy it as a convention.
func TestTheEmphasisWorkaroundIsGone(t *testing.T) {
	body := readHousekeep(t)
	for _, w := range []string{
		"**`receiveTextStatusEnabled()`**",
		"**`getTextStatus`**",
		"**`setMyTextStatus`**",
	} {
		if strings.Contains(body, w) {
			t.Errorf("the emphasis workaround %s is still in the file; the pattern is "+
				"anchored now and identifiers do not need to be bolded to survive it", w)
		}
	}
}
