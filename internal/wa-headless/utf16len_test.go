package waheadless

import (
	"testing"
	"unicode/utf16"
)

// utf16Len counts what JavaScript's String.length counts.
//
// IT EXISTS BECAUSE A LIVE TEST FAILED ON IT. The group rename asked for a
// subject containing an em dash and asserted the page's reported length against
// Go's len(): 74 bytes against 72 UTF-16 units. The rename had WORKED — the
// assertion was wrong, which is the worse of the two outcomes, because a wrong
// assertion in a live test spends a whole run to say nothing.
//
// The same comparison sits in the edit and forward live tests and passed there
// only because those strings were ASCII. Latent, not absent.
func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// TestUTF16LenMatchesJavaScript pins the cases that differ from len(), so the
// helper cannot quietly become a synonym for len().
func TestUTF16LenMatchesJavaScript(t *testing.T) {
	for _, tc := range []struct {
		in        string
		utf16, by int
	}{
		{"abc", 3, 3},
		{"—", 1, 3},          // em dash: one unit, three bytes
		{"ação", 4, 6},       // combining-free latin: two 2-byte runes
		{"wa — x", 6, 8},     // the shape that failed live
		{"\U0001F600", 2, 4}, // emoji: a surrogate PAIR is TWO units
	} {
		if got := utf16Len(tc.in); got != tc.utf16 {
			t.Errorf("utf16Len(%q) = %d, want %d", tc.in, got, tc.utf16)
		}
		if len(tc.in) != tc.by {
			t.Errorf("len(%q) = %d, want %d — the fixture is wrong", tc.in, len(tc.in), tc.by)
		}
		if tc.utf16 == tc.by && tc.in != "abc" {
			t.Errorf("fixture %q does not distinguish the two units", tc.in)
		}
	}
}
