package pulsemodules

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These tests are the invariant the 2026-07-29 refactor lacked. Merging three
// modules into stores_health desynchronized four independently maintained
// surfaces at once — two of which caused production failures — while the build
// and the entire test suite stayed green.
//
// Every test here asserts that some consumer agrees with the registry. A new
// module that reaches one surface and not another must fail here.

func TestRegistryIsInternallyConsistent(t *testing.T) {
	seenID := map[string]bool{}
	seenStep := map[string]bool{}
	seenAlias := map[string]string{}

	// Collect every canonical ID before checking aliases. Checking only IDs
	// encountered earlier would miss an alias that collides with a module
	// declared later in All.
	for _, m := range All {
		if seenID[m.ID] {
			t.Fatalf("duplicate module ID %q", m.ID)
		}
		seenID[m.ID] = true
	}

	for _, m := range All {
		if m.ID == "" || m.Label == "" || m.StepLabel == "" {
			t.Fatalf("module %+v has an empty required field", m)
		}

		if seenStep[m.StepLabel] {
			t.Fatalf("duplicate StepLabel %q", m.StepLabel)
		}
		seenStep[m.StepLabel] = true

		for _, a := range m.Aliases {
			if a == m.ID {
				t.Fatalf("module %q lists its own ID as an alias", m.ID)
			}
			if prev, dup := seenAlias[a]; dup {
				t.Fatalf("alias %q maps to both %q and %q", a, prev, m.ID)
			}
			seenAlias[a] = m.ID
			// An alias colliding with a different module's canonical ID would
			// silently rewrite that module's identity.
			if seenID[a] {
				t.Fatalf("alias %q collides with canonical module ID %q", a, a)
			}
		}
	}

	// Every retired ID must normalize onto a current module, or historical
	// state would resolve to nothing.
	for _, r := range RetiredIDs {
		if IsValid(r) {
			t.Fatalf("retired ID %q is still listed as a current module", r)
		}
		if got := Normalize(r); !IsValid(got) {
			t.Fatalf("retired ID %q normalizes to %q, which is not a current module", r, got)
		}
	}

	// Pseudo IDs are HTML-only classifications and must never be scheduled.
	for _, p := range PseudoIDs {
		if IsValid(p) {
			t.Fatalf("pseudo ID %q must not be a scheduled module", p)
		}
	}
}

func TestNormalizeAndStepLabelRoundTrip(t *testing.T) {
	for _, m := range All {
		if got := Normalize(m.ID); got != m.ID {
			t.Fatalf("Normalize(%q) = %q", m.ID, got)
		}
		// Case and hyphen tolerance is relied on by agent-supplied payloads.
		if got := Normalize(strings.ToUpper(strings.ReplaceAll(m.ID, "_", "-"))); got != m.ID {
			t.Fatalf("Normalize did not tolerate upper/hyphen form of %q", m.ID)
		}
		if got := ForStepLabel(m.StepLabel); got != m.ID {
			t.Fatalf("ForStepLabel(%q) = %q, want %q", m.StepLabel, got, m.ID)
		}
		for _, a := range m.Aliases {
			if got := Normalize(a); got != m.ID {
				t.Fatalf("Normalize(alias %q) = %q, want %q", a, got, m.ID)
			}
		}
	}
	// Non-module stages must not resolve to a module.
	for _, notAModule := range []string{"gate", "finalize", ""} {
		if got := ForStepLabel(notAModule); got != "" {
			t.Fatalf("ForStepLabel(%q) = %q, want \"\"", notAModule, got)
		}
	}
}

func TestAcceptedForReviewReceiptsCoversCurrentAndRetired(t *testing.T) {
	accepted := map[string]bool{}
	for _, id := range AcceptedForReviewReceipts() {
		accepted[id] = true
	}
	for _, m := range All {
		if !accepted[m.ID] {
			t.Fatalf("current module %q is not accepted for reviewer artifacts — its results would silently fail to persist", m.ID)
		}
	}
	for _, r := range RetiredIDs {
		if !accepted[r] {
			t.Fatalf("retired module %q is not accepted — historical reviewer artifacts become unreadable", r)
		}
	}
}

// repoFile reads a path relative to the repository root.
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	// pkg/pulsemodules -> agent_go -> repo root
	path := filepath.Join("..", "..", "..", rel)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// TestFrontendPulseModuleCommandsMatchRegistry keeps the TypeScript module list
// in sync. Go cannot own a TS constant, so this asserts parity by reading the
// source. The database-native Pulse workspace renders directly from
// PULSE_MODULE_COMMANDS; the retired PULSE_SECTIONS tab grouping no longer
// exists.
func TestFrontendPulseModuleCommandsMatchRegistry(t *testing.T) {
	src := repoFile(t, "frontend/src/components/workflow/canvas/pulseSections.ts")

	commandBlock := regexp.MustCompile(`(?s)export const PULSE_MODULE_COMMANDS[^=]*=\s*\[(.*?)\]\s*\n`).FindStringSubmatch(src)
	if len(commandBlock) != 2 {
		t.Fatal("could not parse PULSE_MODULE_COMMANDS from pulseSections.ts")
	}
	idPattern := regexp.MustCompile(`id:\s*'([^']+)'`)
	commandIDs := map[string]bool{}
	for _, match := range idPattern.FindAllStringSubmatch(commandBlock[1], -1) {
		commandIDs[match[1]] = true
	}

	assertExactModuleSet(t, "PULSE_MODULE_COMMANDS", commandIDs)
}

func assertExactModuleSet(t *testing.T, surface string, got map[string]bool) {
	t.Helper()
	want := map[string]bool{}
	for _, m := range All {
		want[m.ID] = true
		if !got[m.ID] {
			t.Fatalf("%s has no entry for current module %q", surface, m.ID)
		}
	}
	for id := range got {
		if !want[id] {
			t.Fatalf("%s contains unknown or retired module %q", surface, id)
		}
	}
}

// TestFrontendTimelineClassifierEmitsOnlyKnownModules guards the HTML
// classifier used to group Pulse cards by module.
//
// Note its actual contract: an explicit data-module attribute wins outright
// (`if (explicit) return explicit`), and the string heuristics below it are a
// best-effort fallback for legacy cards written before attribution existed.
// So a current module does NOT need its own heuristic branch — correctly
// attributed cards resolve via the attribute.
//
// The real drift risk is the opposite direction: a heuristic that *emits* a
// retired or unknown ID silently files cards under a module with no tab, where
// they become unreachable. This asserts every emitted ID is one the registry
// still recognizes.
func TestFrontendTimelineClassifierEmitsOnlyKnownModules(t *testing.T) {
	src := repoFile(t, "frontend/src/components/workflow/pulseTimelineHtml.ts")

	known := map[string]bool{"": true}
	for _, id := range IDs() {
		known[id] = true
	}
	for _, p := range PseudoIDs {
		known[p] = true
	}

	const marker = "return '"
	emitted := 0
	for rest := src; ; {
		i := strings.Index(rest, marker)
		if i < 0 {
			break
		}
		rest = rest[i+len(marker):]
		j := strings.Index(rest, "'")
		if j < 0 {
			break
		}
		id := rest[:j]
		rest = rest[j:]

		// Skip the pass-through of an explicit attribute and any non-ID return.
		if id == "" || strings.ContainsAny(id, " .()") {
			continue
		}
		emitted++
		if !known[id] {
			t.Fatalf("pulseTimelineHtml.ts classifies cards as %q, which the registry does not recognize — those cards would land under a module with no tab", id)
		}
		if IsRetired(id) {
			t.Fatalf("pulseTimelineHtml.ts still emits retired module %q", id)
		}
	}
	if emitted == 0 {
		t.Fatal("found no module classifications to check — the parser or the file shape changed")
	}

	// The pseudo classifications must remain, or Gate/run rows and applied
	// fixes lose their grouping entirely.
	for _, p := range PseudoIDs {
		if !strings.Contains(src, marker+p+"'") {
			t.Fatalf("pulseTimelineHtml.ts no longer classifies pseudo module %q", p)
		}
	}
}

// TestGuidanceDocsNameCurrentModules catches the stale-prose class of drift:
// pulse-gate.md enumerates the modules Gate must decide on, and a retired name
// there sends Gate a module the backend will reject.
func TestGuidanceDocsNameCurrentModules(t *testing.T) {
	gate := repoFile(t, "agent_go/cmd/server/guidance/templates/system/pulse-gate.md")

	for _, m := range All {
		if !strings.Contains(gate, m.ID) {
			t.Fatalf("pulse-gate.md does not name current module %q in its worklist contract", m.ID)
		}
	}
	for _, r := range RetiredIDs {
		if strings.Contains(gate, r) {
			t.Fatalf("pulse-gate.md still names retired module %q — Gate would record a module the backend rejects", r)
		}
	}
}

func TestPulseDashboardSkeletonHasOneCoverageChipPerCurrentModule(t *testing.T) {
	skeleton := repoFile(t, "agent_go/cmd/server/guidance/templates/system/review-improve-log-skeleton.md")
	const chip = `<div class="covitem `
	if got, want := strings.Count(skeleton, chip), len(All); got != want {
		t.Fatalf("Pulse coverage chips = %d, want one for each of %d current modules", got, want)
	}
	for _, retiredLabel := range []string{">Cost + time<", ">Steps &amp; setup<"} {
		if strings.Contains(skeleton, retiredLabel) {
			t.Fatalf("Pulse coverage still contains pre-merge module label %q", retiredLabel)
		}
	}
	if !strings.Contains(skeleton, `class="cl">Engineering review</span>`) {
		t.Fatal("Pulse coverage is missing the combined correctness review chip")
	}
}
