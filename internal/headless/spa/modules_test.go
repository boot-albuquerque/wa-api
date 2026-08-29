package spa

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

func inventoryRunner() *engine.Runner {
	p := engine.DefaultDeadlines
	p.Query = 200 * time.Millisecond
	return &engine.Runner{Policy: p}
}

// answering returns an evaluator that reports the given modules as missing.
func answering(missing ...Module) Evaluator {
	return func(ctx context.Context, expression string, out *string) error {
		names := make([]string, len(missing))
		for i, m := range missing {
			names[i] = string(m)
		}
		b, _ := json.Marshal(names)
		*out = string(b)
		return nil
	}
}

func TestVerifyInventoryPassesWhenEveryModuleResolves(t *testing.T) {
	if err := VerifyInventory(context.Background(), inventoryRunner(), answering(), RequiredAtStartup); err != nil {
		t.Fatalf("VerifyInventory: %v", err)
	}
}

// The requirement of ADR-0006 D4, in one test: a renamed module stops the boot
// and the message NAMES it. The alternative is an exception halfway through
// whichever capability ran first, complaining about a property of undefined.
func TestVerifyInventoryFailsLoudlyAndNamesTheMissingModules(t *testing.T) {
	err := VerifyInventory(context.Background(), inventoryRunner(),
		answering(ModuleCmd, ModuleConnModel), RequiredAtStartup)

	var missing *ErrModulesMissing
	if !errors.As(err, &missing) {
		t.Fatalf("got %T (%v), want *ErrModulesMissing", err, err)
	}
	if len(missing.Missing) != 2 {
		t.Fatalf("reported %d missing modules, want 2", len(missing.Missing))
	}
	for _, want := range []string{string(ModuleCmd), string(ModuleConnModel)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message does not name %s: %v", want, err)
		}
	}
}

// The operational response to a missing module is a code change, and the error
// has to say so. A fleet that reads this as transient restarts every session
// forever over something no retry fixes.
func TestMissingModulesErrorSaysItIsNotARetry(t *testing.T) {
	err := &ErrModulesMissing{Missing: []Module{ModuleCmd}}
	if !strings.Contains(err.Error(), "not a retry") {
		t.Errorf("the message does not tell the operator this is not transient: %v", err)
	}
}

// A page that never had window.require is a page that is not the application.
// Reporting everything missing is right, and it must not look like success.
func TestVerifyInventoryFailsWhenRequireItselfIsAbsent(t *testing.T) {
	all := func(ctx context.Context, expression string, out *string) error {
		names := make([]string, len(RequiredAtStartup))
		for i, m := range RequiredAtStartup {
			names[i] = string(m)
		}
		b, _ := json.Marshal(names)
		*out = string(b)
		return nil
	}

	err := VerifyInventory(context.Background(), inventoryRunner(), all, RequiredAtStartup)
	var missing *ErrModulesMissing
	if !errors.As(err, &missing) {
		t.Fatalf("got %v, want *ErrModulesMissing", err)
	}
	if len(missing.Missing) != len(RequiredAtStartup) {
		t.Fatalf("reported %d missing, want all %d", len(missing.Missing), len(RequiredAtStartup))
	}
}

// An answer in the wrong shape is not an empty answer. Unmarshalling it into an
// empty list would report a healthy inventory from a page that said something
// else entirely.
func TestVerifyInventoryRefusesAnUnexpectedAnswer(t *testing.T) {
	garbage := func(ctx context.Context, expression string, out *string) error {
		*out = `{"unexpected": true}`
		return nil
	}

	err := VerifyInventory(context.Background(), inventoryRunner(), garbage, RequiredAtStartup)
	if err == nil {
		t.Fatal("an answer in the wrong shape was accepted as a healthy inventory")
	}
	var missing *ErrModulesMissing
	if errors.As(err, &missing) {
		t.Error("a malformed answer was reported as missing modules; the page said " +
			"something we do not understand, which is a different problem")
	}
}

// The script must ask about the names we hold, and only those. Sending a name
// this code does not know would mean the list is not the single source it
// claims to be.
func TestResolveScriptCarriesExactlyTheRequestedNames(t *testing.T) {
	script := resolveScript([]Module{ModuleCmd, ModuleSocketModel})

	for _, want := range []string{string(ModuleCmd), string(ModuleSocketModel)} {
		if !strings.Contains(script, "'"+want+"'") {
			t.Errorf("the script does not ask for %s", want)
		}
	}
	if strings.Contains(script, string(ModuleConnModel)) {
		t.Error("the script asks for a module that was not requested")
	}
	// window.require throws on an unknown name, so the check has to survive it.
	if !strings.Contains(script, "catch") {
		t.Error("the script does not catch: window.require throws for an unknown " +
			"name, so the first missing module would abort the whole check")
	}
}

func TestVerifyInventoryIsBoundedByTheQueryBudget(t *testing.T) {
	slow := func(ctx context.Context, expression string, out *string) error {
		<-ctx.Done()
		return ctx.Err()
	}

	start := time.Now()
	err := VerifyInventory(context.Background(), inventoryRunner(), slow, RequiredAtStartup)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a page that never answered passed the inventory check")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("VerifyInventory took %v on a 200ms Query budget", elapsed)
	}
}

func TestVerifyInventoryOnAnEmptyListIsANoop(t *testing.T) {
	never := func(ctx context.Context, expression string, out *string) error {
		t.Fatal("the page was asked about an empty module list")
		return nil
	}
	if err := VerifyInventory(context.Background(), inventoryRunner(), never, nil); err != nil {
		t.Fatalf("VerifyInventory on an empty list: %v", err)
	}
}
