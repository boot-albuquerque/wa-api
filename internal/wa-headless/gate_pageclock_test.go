package waheadless

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoClockInProductionPageScripts makes invariant 6 a GATE.
//
// The rule is that the page does not decide how long to wait. It is not
// fussiness: a page that counts its own timeout counts it during a reload, a
// throttled tab and a hung renderer, in a place where Go can neither see the
// decision nor cancel it. Every deadline in this module is on the Go side so
// that a caller's context means something, and a single setTimeout in a page
// script quietly opts one operation out of that.
//
// IT EXISTS BECAUSE THE RULE WAS BROKEN BY SOMEBODY WHO KNEW IT. The first
// version of the address-book save polled the contact record with setTimeout
// inside the page. It would have worked. Nothing in the suite would have
// noticed, which is precisely the definition of a rule that needs a gate rather
// than a paragraph (H90).
//
// STAMPING IS NOT DECIDING. Date.now() writing a timestamp onto an event is
// allowed and is listed below: recording when something happened does not move
// a decision into the page, and events/ingress.go says so where it does it.
func TestNoClockInProductionPageScripts(t *testing.T) {
	// Each entry is a KNOWN occurrence with the reason it is allowed. An
	// allowlist with no reasons is a list of things nobody has to think about
	// again.
	//
	// IT HELD A SECOND ENTRY FOR ABOUT AN HOUR. capabilities/group/group.go
	// slept in the page and was listed here as a pre-existing finding. The
	// instruction that removed it is worth keeping: a known violation must not
	// stabilise inside an allowlist, and an exception may exist only while the
	// fix is in the same block. It was fixed in that block (H90).
	allowed := map[string]string{
		"events/ingress.go": "Date.now() STAMPS an event's time; Event.At documents that it " +
			"is reported for diagnosis and never used to decide anything",
	}

	clock := regexp.MustCompile(`\b(setTimeout|setInterval|requestAnimationFrame|Date\.now)\s*\(`)
	var offenders []string
	roots := []string{"capabilities", "spa", "events", "core", "engine", "runtime", "observability"}
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(body), "\n") {
				trimmed := strings.TrimSpace(line)
				// A COMMENT EXPLAINING THE RULE IS NOT A BREACH OF IT. Two
				// files in this module say "a setTimeout here would break
				// invariant 6" at exactly the place somebody would write one,
				// and a gate that flagged those would teach people to delete
				// the warning.
				if strings.HasPrefix(trimmed, "//") {
					continue
				}
				if !clock.MatchString(line) {
					continue
				}
				key := filepath.ToSlash(path)
				if _, ok := allowed[key]; ok {
					continue
				}
				offenders = append(offenders, fmt.Sprintf("%s:%d", key, i+1))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("invariant 6: a page script decides its own waiting at %s. "+
			"Park the answer and poll from Go under the caller's context, the way every "+
			"other write in this module does. If the use really is a STAMP rather than a "+
			"decision, add it to this test's allowlist WITH the reason.",
			strings.Join(offenders, ", "))
	}

	// AND THE ALLOWLIST MUST STILL BE TRUE. An exception for a file that no
	// longer has the construct is an exception nobody will notice has expired,
	// and it would silently permit the next one.
	for file, why := range allowed {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("allowlisted %s cannot be read: %v", file, err)
			continue
		}
		found := false
		for _, line := range strings.Split(string(body), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "//") && clock.MatchString(line) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s is allowlisted (%s) and no longer contains a clock; remove the exception", file, why)
		}
	}
}
