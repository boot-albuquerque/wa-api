package waheadless

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	ledgerPath  = "LEDGER-WWEBJS.md"
	surfacePath = "testdata/wwebjs-v1.34.7-surface.txt"
)

// TestTheLedgerCoversTheWholeUpstreamSurface makes the completeness rule a GATE
// rather than an intention.
//
// The rule is that no public capability of the pinned upstream commit may go
// missing from the ledger in silence. An intention cannot enforce that: the
// failure mode is somebody adding a capability, updating the ledger row they
// were looking at, and never noticing the twelve rows they were not.
//
// The upstream surface is CHECKED IN, extracted once from the pinned tag. That
// is deliberate: a gate that fetched GitHub would fail offline, and would also
// silently start measuring a different upstream the day the tag moved — which
// is exactly the drift the pin exists to prevent.
func TestTheLedgerCoversTheWholeUpstreamSurface(t *testing.T) {
	surface, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatalf("read %s: %v", surfacePath, err)
	}
	ledger, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read %s: %v", ledgerPath, err)
	}
	led := string(ledger)

	method := regexp.MustCompile(`^\s+(\S+)\s+\[`)
	event := regexp.MustCompile(`^\s+(\w+) = '`)

	var missing []string
	var total int
	for _, line := range strings.Split(string(surface), "\n") {
		var name string
		if m := method.FindStringSubmatch(line); m != nil {
			name = m[1]
		} else if m := event.FindStringSubmatch(line); m != nil {
			name = m[1]
		} else {
			continue
		}
		total++
		// The ledger names every item in a table cell as `name`.
		if !strings.Contains(led, "`"+name+"`") {
			missing = append(missing, name)
		}
	}
	if total == 0 {
		t.Fatalf("%s produced no items; this test is guarding nothing", surfacePath)
	}
	if len(missing) > 0 {
		t.Fatalf("%d upstream item(s) are absent from %s — the completeness rule says a "+
			"capability we do not implement still needs a line saying why: %s",
			len(missing), ledgerPath, strings.Join(missing, ", "))
	}
	t.Logf("the ledger accounts for all %d items of the pinned upstream surface", total)
}

// TestTheLedgerUsesOnlyTheDeclaredVocabulary. A state outside the list cannot be
// counted, and a ledger that cannot be counted is a document rather than a gate.
func TestTheLedgerUsesOnlyTheDeclaredVocabulary(t *testing.T) {
	body, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read %s: %v", ledgerPath, err)
	}
	allowed := map[string]bool{
		"PROVEN": true, "PARTIAL": true, "MISSING": true,
		"BLOCKED": true, "INTENTIONAL_DIFFERENCE": true,
	}
	// THE STATE IS A COLUMN, NOT A SHAPE.
	//
	// This test used to find states by looking for all-caps in backticks
	// anywhere in the file, and then excluded the upstream's event names —
	// which are also all-caps — by listing them from the pinned surface. That
	// worked until a NOTE quoted this module's own liveness vocabulary
	// (`ALIVE`, `PROCESS_GONE`, `APP_ABSENT`) and the gate reported three
	// states that were never states. The exclusion list was a patch on a
	// pattern that was matching the wrong thing.
	//
	// A parity row has four columns and the third one IS the state. Reading
	// that column is structural, needs no exclusion list, and cannot be fooled
	// by prose. It is the same correction the HOUSEKEEP status pattern needed,
	// and the second time this file has learned it — which is why it is written
	// down here rather than remembered.
	seen := ledgerStates(string(body))
	if len(seen) == 0 {
		t.Fatal("the ledger declares no states at all")
	}
	for s := range seen {
		if !allowed[s] {
			t.Errorf("state %q is not in the declared vocabulary; adding one is a "+
				"decision, not a typo", s)
		}
	}
}

// TestTheLedgerScoreboardMatchesItsRows. A scoreboard maintained by hand drifts
// from the table above it, and then the number people quote is the stale one.
func TestTheLedgerScoreboardMatchesItsRows(t *testing.T) {
	body, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read %s: %v", ledgerPath, err)
	}
	text := string(body)
	i := strings.Index(text, "## Placar")
	if i < 0 {
		t.Fatal("the ledger has no scoreboard")
	}
	scoreboard := text[i:]

	// THE VOCABULARY TABLE DECLARES EVERY STATE ONCE, and counting it as data
	// made each total exactly one too high — a bug that looks like a drifting
	// scoreboard and is really a mis-scoped scan. Rows start after that section
	// ends, located by heading rather than by guessing a line number.
	vocab := strings.Index(text, "## Vocabulário de estado")
	if vocab < 0 {
		t.Fatal("the ledger has no vocabulary section; the scan cannot exclude it")
	}
	next := strings.Index(text[vocab+10:], "\n## ")
	if next < 0 {
		t.Fatal("the vocabulary section is not followed by a data section")
	}
	rows := text[vocab+10+next : i]

	// Count states in the ROWS only, from the trailing state cell of each table
	// line, so the vocabulary table at the top is not counted as data.
	counted := ledgerStates(rows)
	declared := map[string]int{}
	scoreRow := regexp.MustCompile("\\| `([A-Z][A-Z_]{3,})` \\| (\\d+) \\|")
	for _, m := range scoreRow.FindAllStringSubmatch(scoreboard, -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("scoreboard count %q is not a number", m[2])
		}
		declared[m[1]] = n
	}
	if len(declared) == 0 {
		t.Fatal("the scoreboard has no counts")
	}
	for st, want := range declared {
		if got := counted[st]; got != want {
			t.Errorf("the scoreboard says %d %s and the rows contain %d", want, st, got)
		}
	}
	var sum int
	for _, n := range declared {
		sum += n
	}
	if !strings.Contains(scoreboard, "**"+strconv.Itoa(sum)+"**") {
		t.Errorf("the scoreboard total does not equal the sum of its states (%d)", sum)
	}
}

// TestTheLedgerPinsAnExactUpstream. A ledger without a pin measures a moving
// target, and "we have parity" would mean a different thing every week.
func TestTheLedgerPinsAnExactUpstream(t *testing.T) {
	body, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read %s: %v", ledgerPath, err)
	}
	text := string(body)
	for _, want := range []string{
		"v1.34.7",
		"f935b500117e264c2b3abc25b63a280bd98182a7",
		"2026-04-24",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the ledger does not record %q; the pin is version, SHA and date", want)
		}
	}
	// And the checked-in surface must be the one the pin names.
	if !strings.Contains(surfacePath, "v1.34.7") {
		t.Error("the checked-in surface file is not named for the pinned version")
	}
}

// ledgerStates counts the state of every parity row in text.
//
// THE STATE COLUMN IS FOUND FROM EACH TABLE'S OWN HEADER, and that is the whole
// trick. The ledger has THREE table shapes — seven columns for the families with
// per-item evidence, four for Events, two for the long tails that are entirely
// unattacked — and every attempt to hard-code a position gets one of them wrong.
// The first version of this scan assumed the third cell and silently lost 28
// rows; the version before it matched all-caps in backticks anywhere and
// invented three states out of a NOTE that quoted this module's liveness
// vocabulary.
//
// A header cell says "estado". Reading its index, and then reading that index in
// the rows beneath it, is structural, survives a new table shape, and needs no
// list of exceptions. Same lesson as the HOUSEKEEP status pattern, now learned
// three times in this one file: anchor on what the document SAYS about itself,
// never on what its rows happen to look like.
func ledgerStates(text string) map[string]int {
	counted := map[string]int{}
	stateCol := -1
	for _, line := range strings.Split(text, "\n") {
		cells := tableCells(line)
		if cells == nil {
			stateCol = -1 // a non-row ends the table; the next one re-declares.
			continue
		}
		if i := indexOfCell(cells, "estado"); i >= 0 {
			stateCol = i
			continue
		}
		first := strings.TrimSpace(cells[0])
		if stateCol < 0 || stateCol >= len(cells) ||
			!strings.HasPrefix(first, "`") || !strings.HasSuffix(first, "`") {
			continue
		}
		counted[strings.Trim(strings.TrimSpace(cells[stateCol]), "`")]++
	}
	return counted
}

// tableCells returns a markdown row's cells, or nil if the line is not a row.
func tableCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") || len(line) < 3 {
		return nil
	}
	parts := strings.Split(line, "|")
	return parts[1 : len(parts)-1]
}

func indexOfCell(cells []string, want string) int {
	for i, c := range cells {
		if strings.TrimSpace(c) == want {
			return i
		}
	}
	return -1
}
