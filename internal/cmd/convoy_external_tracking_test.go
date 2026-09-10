package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func writeExternalTrackingBdStub(t *testing.T, scriptBody string) {
	t.Helper()

	binDir := t.TempDir()
	bdPath := filepath.Join(binDir, "bd")
	script := "#!/bin/sh\n" + scriptBody
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func chdirExternalTrackingTest(t *testing.T, dir string) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
}

func makeExternalTrackingTownWorkspace(t *testing.T) (string, string, string) {
	t.Helper()

	townRoot := t.TempDir()
	townBeads := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeads, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	expectedWD := townRoot
	if resolved, err := filepath.EvalSymlinks(townRoot); err == nil && resolved != "" {
		expectedWD = resolved
	}
	return townRoot, townBeads, expectedWD
}

func TestGetIssueDetailsBatchRoutesTrackedIDsByPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, _, expectedTownWD := makeExternalTrackingTownWorkspace(t)
	rigDir := filepath.Join(townRoot, "worker", "mayor", "rig")
	if err := os.MkdirAll(filepath.Join(rigDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir rig beads: %v", err)
	}
	routes := `{"prefix":"hq-","path":"."}
{"prefix":"ws-","path":"worker/mayor/rig"}
`
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}
	chdirExternalTrackingTest(t, townRoot)
	t.Setenv("BEADS_DIR", "/wrong/.beads")
	t.Setenv("BEADS_DOLT_SERVER_DATABASE", "wrong-db")

	expectedRigWD := rigDir
	if resolved, err := filepath.EvalSymlinks(rigDir); err == nil && resolved != "" {
		expectedRigWD = resolved
	}
	logPath := filepath.Join(t.TempDir(), "bd-calls.log")
	scriptBody := fmt.Sprintf(`
printf '%%s|%%s|%%s\n' "$PWD" "$BEADS_DIR" "$*" >> %q

case "$*" in
	  "--allow-stale version")
	    echo 'bd 1.0.0'
	    exit 0
	    ;;
	  "show --json hq-town"|"--allow-stale show --json hq-town"|"show hq-town --json"|"--allow-stale show hq-town --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town dir, got $PWD" >&2
      exit 1
    fi
	    if [ "$BEADS_DIR" != "%s/.beads" ]; then
	      echo "expected town BEADS_DIR, got $BEADS_DIR" >&2
	      exit 1
	    fi
	    if [ "$BEADS_DOLT_SERVER_DATABASE" = "wrong-db" ]; then
	      echo "stale Dolt database leaked" >&2
	      exit 1
	    fi
	    echo '[{"id":"hq-town","title":"Town task","status":"open","issue_type":"task","assignee":"mayor","labels":["kind/bug"]}]'
	    ;;
	  "show --json ws-rig"|"--allow-stale show --json ws-rig"|"show ws-rig --json"|"--allow-stale show ws-rig --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected rig dir, got $PWD" >&2
      exit 1
    fi
	    if [ "$BEADS_DIR" != "%s/.beads" ]; then
	      echo "expected rig BEADS_DIR, got $BEADS_DIR" >&2
	      exit 1
	    fi
	    if [ "$BEADS_DOLT_SERVER_DATABASE" = "wrong-db" ]; then
	      echo "stale Dolt database leaked" >&2
	      exit 1
	    fi
	    echo '[{"id":"ws-rig","title":"Rig task","status":"closed","issue_type":"task","assignee":"gastown/polecats/chrome","labels":["cleanup"],"dependencies":[{"id":"ws-blocker","status":"open","dependency_type":"blocks"}]}]'
	    ;;
	  *"show hq-town ws-rig --json"*|*"show ws-rig hq-town --json"*|*"show --json hq-town ws-rig"*|*"show --json ws-rig hq-town"*)
    echo "mixed-prefix batch should not be used" >&2
    exit 1
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`, logPath, expectedTownWD, expectedTownWD, expectedRigWD, expectedRigWD)
	writeExternalTrackingBdStub(t, scriptBody)

	got := getIssueDetailsBatch([]string{"hq-town", "ws-rig"})
	if len(got) != 2 {
		t.Fatalf("expected 2 details, got %d: %#v", len(got), got)
	}
	if got["hq-town"] == nil || got["hq-town"].Status != "open" || got["hq-town"].Assignee != "mayor" {
		t.Fatalf("town detail not resolved through town route: %#v", got["hq-town"])
	}
	if got["ws-rig"] == nil || got["ws-rig"].Status != "closed" || got["ws-rig"].IssueType != "task" {
		t.Fatalf("rig detail not resolved through rig route: %#v", got["ws-rig"])
	}
	if !got["ws-rig"].IsBlocked() {
		t.Fatalf("rig dependencies were not preserved: %#v", got["ws-rig"])
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd call log: %v", err)
	}
	log := string(logBytes)
	if strings.Contains(log, "show hq-town ws-rig --json") ||
		strings.Contains(log, "show ws-rig hq-town --json") ||
		strings.Contains(log, "show --json hq-town ws-rig") ||
		strings.Contains(log, "show --json ws-rig hq-town") {
		t.Fatalf("mixed-prefix batch call was used:\n%s", log)
	}
}

func TestGetIssueDetailsBatchPreservesSuccessfulRouteGroupsOnFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, _, _ := makeExternalTrackingTownWorkspace(t)
	rigDir := filepath.Join(townRoot, "worker", "mayor", "rig")
	if err := os.MkdirAll(filepath.Join(rigDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir rig beads: %v", err)
	}
	routes := `{"prefix":"hq-","path":"."}
{"prefix":"ws-","path":"worker/mayor/rig"}
`
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}
	chdirExternalTrackingTest(t, townRoot)

	logPath := filepath.Join(t.TempDir(), "bd-calls.log")
	scriptBody := fmt.Sprintf(`
printf '%%s\n' "$*" >> %q

case "$*" in
  "--allow-stale version")
    echo 'bd 1.0.0'
    exit 0
    ;;
  "show --json hq-town"|"--allow-stale show --json hq-town")
    echo '[{"id":"hq-town","title":"Town task","status":"open","issue_type":"task"}]'
    ;;
  "show --json ws-one ws-missing"|"--allow-stale show --json ws-one ws-missing")
    echo "missing issue in routed batch" >&2
    exit 1
    ;;
  "show ws-one --json"|"--allow-stale show ws-one --json")
    echo '[{"id":"ws-one","title":"Recovered worker task","status":"open","issue_type":"task"}]'
    ;;
  "show ws-missing --json"|"--allow-stale show ws-missing --json")
    echo "missing issue" >&2
    exit 1
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`, logPath)
	writeExternalTrackingBdStub(t, scriptBody)

	got := getIssueDetailsBatch([]string{"hq-town", "ws-one", "ws-missing"})
	if len(got) != 2 {
		t.Fatalf("expected 2 recovered details, got %d: %#v", len(got), got)
	}
	if got["hq-town"] == nil || got["hq-town"].Status != "open" {
		t.Fatalf("town detail was not preserved from successful route group: %#v", got["hq-town"])
	}
	if got["ws-one"] == nil || got["ws-one"].Title != "Recovered worker task" {
		t.Fatalf("worker detail was not recovered through single lookup: %#v", got["ws-one"])
	}
	if got["ws-missing"] != nil {
		t.Fatalf("missing issue should be omitted, got %#v", got["ws-missing"])
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd call log: %v", err)
	}
	log := string(logBytes)
	if strings.Count(log, "show --json hq-town") != 1 || strings.Contains(log, "show hq-town --json") {
		t.Fatalf("successful town route group should not be retried:\n%s", log)
	}
}

func TestGetTrackedIssues_RoutesShowByPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, townBeads, expectedTownWD := makeExternalTrackingTownWorkspace(t)
	rigDir := filepath.Join(townRoot, "worker", "mayor", "rig")
	if err := os.MkdirAll(filepath.Join(rigDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir rig beads: %v", err)
	}
	routes := `{"prefix":"hq-","path":"."}
{"prefix":"ws-","path":"worker/mayor/rig"}
`
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}
	chdirExternalTrackingTest(t, townRoot)
	t.Setenv("BEADS_DIR", "/wrong/.beads")
	t.Setenv("BEADS_DOLT_SERVER_DATABASE", "wrong-db")

	expectedRigWD := rigDir
	if resolved, err := filepath.EvalSymlinks(rigDir); err == nil && resolved != "" {
		expectedRigWD = resolved
	}
	scriptBody := fmt.Sprintf(`
case "$*" in
	  "--allow-stale version")
	    echo 'bd 1.0.0'
	    exit 0
	    ;;
	  *sql*dependencies*)
	    if [ "$BEADS_DOLT_SERVER_DATABASE" = "wrong-db" ]; then
	      echo "stale Dolt database leaked into sql" >&2
	      exit 1
	    fi
	    echo '[{"depends_on_id":"ws-123"},{"depends_on_id":"hq-456"}]'
	    ;;
	  "show --json ws-123"|"--allow-stale show --json ws-123"|"show ws-123 --json"|"--allow-stale show ws-123 --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected rig dir, got $PWD" >&2
      exit 1
    fi
	    if [ "$BEADS_DIR" != "%s/.beads" ]; then
	      echo "expected rig BEADS_DIR, got $BEADS_DIR" >&2
	      exit 1
	    fi
	    if [ "$BEADS_DOLT_SERVER_DATABASE" = "wrong-db" ]; then
	      echo "stale Dolt database leaked" >&2
	      exit 1
	    fi
	    echo '[{"id":"ws-123","title":"Worker issue","status":"open","issue_type":"task"}]'
	    ;;
	  "show --json hq-456"|"--allow-stale show --json hq-456"|"show hq-456 --json"|"--allow-stale show hq-456 --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town dir, got $PWD" >&2
      exit 1
    fi
	    if [ "$BEADS_DIR" != "%s/.beads" ]; then
	      echo "expected town BEADS_DIR, got $BEADS_DIR" >&2
	      exit 1
	    fi
	    if [ "$BEADS_DOLT_SERVER_DATABASE" = "wrong-db" ]; then
	      echo "stale Dolt database leaked" >&2
	      exit 1
	    fi
	    echo '[{"id":"hq-456","title":"Town issue","status":"closed","issue_type":"task"}]'
	    ;;
	  *"show ws-123 hq-456 --json"*|*"show hq-456 ws-123 --json"*|*"show --json ws-123 hq-456"*|*"show --json hq-456 ws-123"*)
    echo "mixed-prefix batch should not be used" >&2
    exit 1
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`, expectedRigWD, expectedRigWD, expectedTownWD, expectedTownWD)
	writeExternalTrackingBdStub(t, scriptBody)

	tracked, err := getTrackedIssues(townBeads, "hq-cv-route")
	if err != nil {
		t.Fatalf("getTrackedIssues: %v", err)
	}
	if len(tracked) != 2 {
		t.Fatalf("expected 2 tracked issues, got %d: %#v", len(tracked), tracked)
	}

	statusByID := map[string]string{}
	for _, item := range tracked {
		statusByID[item.ID] = item.Status
	}
	if statusByID["ws-123"] != "open" {
		t.Fatalf("ws-123 status = %q, want %q", statusByID["ws-123"], "open")
	}
	if statusByID["hq-456"] != "closed" {
		t.Fatalf("hq-456 status = %q, want %q", statusByID["hq-456"], "closed")
	}
}

func TestGetTrackedIssues_FallsBackToShowTrackedDependencies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, townBeads, _ := makeExternalTrackingTownWorkspace(t)
	chdirExternalTrackingTest(t, townRoot)

	scriptBody := fmt.Sprintf(`
case "$*" in
  "--allow-stale version")
    exit 0
    ;;
  "dep list hq-cv-ext --direction=down --type=tracks --json")
    echo '[]'
    ;;
  "show hq-cv-ext --json")
    echo '[{"id":"hq-cv-ext","title":"External convoy","status":"open","issue_type":"convoy","dependencies":[{"id":"external:ghostty:ghostty-123","title":"Ghost 123","status":"open","type":"task","dependency_type":"tracks"},{"id":"external:ghostty:ghostty-456","title":"Ghost 456","status":"closed","type":"task","dependency_type":"tracks"},{"id":"gt-ignore","title":"Ignore me","status":"open","type":"task","dependency_type":"blocks"}]}]'
    ;;
  "show ghostty-123 ghostty-456 --json"|"show ghostty-456 ghostty-123 --json")
    echo '[{"id":"ghostty-123","title":"Ghost 123","status":"open","issue_type":"task"},{"id":"ghostty-456","title":"Ghost 456","status":"closed","issue_type":"task"}]'
    ;;
  "show ghostty-123 --json")
    echo '[{"id":"ghostty-123","title":"Ghost 123","status":"open","issue_type":"task"}]'
    ;;
  "show ghostty-456 --json")
    echo '[{"id":"ghostty-456","title":"Ghost 456","status":"closed","issue_type":"task"}]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`)
	writeExternalTrackingBdStub(t, scriptBody)

	tracked, err := getTrackedIssues(townBeads, "hq-cv-ext")
	if err != nil {
		t.Fatalf("getTrackedIssues: %v", err)
	}
	if len(tracked) != 2 {
		t.Fatalf("expected 2 tracked issues, got %d", len(tracked))
	}

	ids := []string{tracked[0].ID, tracked[1].ID}
	sort.Strings(ids)
	if ids[0] != "ghostty-123" || ids[1] != "ghostty-456" {
		t.Fatalf("unexpected tracked IDs: %v", ids)
	}

	statusByID := map[string]string{}
	for _, item := range tracked {
		statusByID[item.ID] = item.Status
	}
	if statusByID["ghostty-123"] != "open" || statusByID["ghostty-456"] != "closed" {
		t.Fatalf("unexpected tracked statuses: %#v", statusByID)
	}
}

// TestGetTrackedIssues_UnknownStatusForUnreachableCrossRig verifies the (gt-bs6)
// contract: when the tracked bead lives in a cross-rig DB that cannot be
// resolved from the convoy owner's cwd (routes.jsonl missing, rig parked, or
// rig beads DB unreachable), the returned tracked entry carries status
// trackedStatusUnknown instead of an empty string. Empty status was
// indistinguishable from a legitimately open bead and silenced the real
// failure mode noted in #2786.
func TestGetTrackedIssues_UnknownStatusForUnreachableCrossRig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, townBeads, _ := makeExternalTrackingTownWorkspace(t)
	chdirExternalTrackingTest(t, townRoot)

	// bd sql returns a single cross-rig tracks edge. `bd show` fails for the
	// target bead (simulating an unreachable / unrouted rig DB). The function
	// must still return the tracked dep, with Status = trackedStatusUnknown.
	scriptBody := `
case "$*" in
  "--allow-stale version")
    exit 0
    ;;
  *sql*dependencies*)
    echo '[{"depends_on_id":"ws-foo"}]'
    ;;
  "show ws-foo --json")
    echo "no issue found matching \"ws-foo\"" >&2
    exit 1
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`
	writeExternalTrackingBdStub(t, scriptBody)

	tracked, err := getTrackedIssues(townBeads, "hq-cv-unreach")
	if err != nil {
		t.Fatalf("getTrackedIssues: %v", err)
	}
	if len(tracked) != 1 {
		t.Fatalf("expected 1 tracked issue, got %d: %#v", len(tracked), tracked)
	}
	if tracked[0].ID != "ws-foo" {
		t.Fatalf("tracked[0].ID = %q, want %q", tracked[0].ID, "ws-foo")
	}
	if tracked[0].Status != trackedStatusUnknown {
		t.Fatalf("tracked[0].Status = %q, want %q", tracked[0].Status, trackedStatusUnknown)
	}
}

// TestCloseConvoyIfComplete_UnknownBlocksAutoClose verifies (gt-bs6) that an
// unknown-status tracked bead prevents convoy auto-close. The rig DB being
// temporarily unreachable must not be mistaken for a completed bead.
func TestCloseConvoyIfComplete_UnknownBlocksAutoClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	// No bd stub — closeConvoyIfComplete does not shell out when the convoy
	// isn't closable, which is exactly the scenario under test.
	townBeads := t.TempDir()
	tracked := []trackedIssueInfo{
		{ID: "ws-foo", Status: trackedStatusUnknown},
		{ID: "ws-bar", Status: "closed"},
	}

	out, err := captureConvoyStdoutErr(t, func() error {
		ready, err := closeConvoyIfComplete(townBeads, "hq-cv-unreach", "Mixed", tracked, false)
		if ready {
			t.Fatalf("closeConvoyIfComplete reported ready with unknown tracked status")
		}
		return err
	})
	if err != nil {
		t.Fatalf("closeConvoyIfComplete: %v", err)
	}
	if !strings.Contains(out, "unknown") {
		t.Fatalf("diagnostic missing 'unknown' label: %q", out)
	}
}
