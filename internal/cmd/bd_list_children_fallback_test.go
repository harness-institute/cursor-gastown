package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBdListChildren_FallsBackToDepsTable is the regression test for GH #3700:
// `gt mountain <epic>` failed with "no slingable tasks in DAG" because
// `bd list --parent=<epic>` returned `[]` even though parent-child links
// existed. The deps table query (`bd sql ... type='parent-child'`) is
// authoritative; the new fallback in bdListChildren consults it whenever
// the primary --parent index returns empty.
func TestBdListChildren_FallsBackToDepsTable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stub")
	}

	townRoot, _ := makeRoutingTownWorkspace(t)

	// Route the "ha-" prefix to a rig directory so beadsDirForID can resolve.
	routes := `{"prefix":"ha-","path":"happyhour"}` + "\n"
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"),
		[]byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "happyhour", ".beads"), 0755); err != nil {
		t.Fatalf("mkdir rig: %v", err)
	}

	chdirConvoyTest(t, townRoot)

	// bd stub:
	//   list --parent=ha-epic   → []   (the buggy/empty primary path)
	//   sql "...parent-child..." → child IDs (the authoritative deps table)
	//   show ha-c1 / ha-c2      → child rows
	scriptBody := `
case "$*" in
  *"list --parent=ha-epic"*)
    echo '[]'
    ;;
  *"sql"*"parent-child"*"--json"*)
    # bd sql --json returns rows as a JSON array of {col: val} maps
    echo '[{"depends_on_id":"ha-c1"},{"depends_on_id":"ha-c2"}]'
    ;;
  "show ha-c1 --json")
    echo '[{"id":"ha-c1","title":"Child one","status":"open","issue_type":"task"}]'
    ;;
  "show ha-c2 --json")
    echo '[{"id":"ha-c2","title":"Child two","status":"open","issue_type":"task"}]'
    ;;
  "--allow-stale version")
    exit 0
    ;;
  *)
    echo "unexpected bd invocation: $*" >&2
    exit 1
    ;;
esac
`
	writeRoutingBdStub(t, scriptBody)

	got, err := bdListChildren("ha-epic")
	if err != nil {
		t.Fatalf("bdListChildren error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 children from deps fallback, got %d: %+v", len(got), got)
	}
	seen := map[string]bool{}
	for _, c := range got {
		seen[c.ID] = true
	}
	if !seen["ha-c1"] || !seen["ha-c2"] {
		t.Errorf("expected children ha-c1 and ha-c2, got %v", seen)
	}
}

// TestBdListChildren_PrimaryPathStillUsed verifies the fallback only kicks in
// when the primary `bd list --parent` path returns empty. When --parent
// returns rows directly, we use them and never invoke the SQL fallback.
func TestBdListChildren_PrimaryPathStillUsed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — shell stub")
	}

	townRoot, _ := makeRoutingTownWorkspace(t)
	routes := `{"prefix":"ha-","path":"happyhour"}` + "\n"
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"),
		[]byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "happyhour", ".beads"), 0755); err != nil {
		t.Fatalf("mkdir rig: %v", err)
	}
	chdirConvoyTest(t, townRoot)

	scriptBody := `
case "$*" in
  *"list --parent=ha-epic"*)
    echo '[{"id":"ha-direct","title":"Direct child","status":"open","issue_type":"task"}]'
    ;;
  *"sql"*)
    echo "fallback was called when primary returned rows" >&2
    exit 1
    ;;
  *)
    echo "unexpected bd invocation: $*" >&2
    exit 1
    ;;
esac
`
	writeRoutingBdStub(t, scriptBody)

	got, err := bdListChildren("ha-epic")
	if err != nil {
		t.Fatalf("bdListChildren error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ha-direct" {
		t.Fatalf("expected single ha-direct child from primary path, got %+v", got)
	}
}
