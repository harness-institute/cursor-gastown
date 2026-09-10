package polecat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTargetCleanPolicy(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    TargetCleanPolicy
		wantErr bool
	}{
		{"empty defaults to per_bead", "", TargetCleanPolicy{Mode: TargetCleanModePerBead}, false},
		{"per_bead", "per_bead", TargetCleanPolicy{Mode: TargetCleanModePerBead}, false},
		{"never", "never", TargetCleanPolicy{Mode: TargetCleanModeNever}, false},
		{"every_n_beads:5", "every_n_beads:5", TargetCleanPolicy{Mode: TargetCleanModeEveryNBeads, EveryN: 5}, false},
		{"every_n_beads:1", "every_n_beads:1", TargetCleanPolicy{Mode: TargetCleanModeEveryNBeads, EveryN: 1}, false},
		{"whitespace tolerated", "  per_bead  ", TargetCleanPolicy{Mode: TargetCleanModePerBead}, false},
		{"whitespace in N tolerated", "every_n_beads: 3", TargetCleanPolicy{Mode: TargetCleanModeEveryNBeads, EveryN: 3}, false},
		{"unknown mode rejected", "yolo", TargetCleanPolicy{}, true},
		{"N=0 rejected", "every_n_beads:0", TargetCleanPolicy{}, true},
		{"negative N rejected", "every_n_beads:-3", TargetCleanPolicy{}, true},
		{"non-numeric N rejected", "every_n_beads:abc", TargetCleanPolicy{}, true},
		{"missing colon rejected", "every_n_beads", TargetCleanPolicy{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTargetCleanPolicy(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseTargetCleanPolicy(%q) err=%v wantErr=%v", tc.input, err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("ParseTargetCleanPolicy(%q) = %+v, want %+v", tc.input, got, tc.want)
			}
		})
	}
}

func TestTargetCleanPolicyString(t *testing.T) {
	tests := []struct {
		policy TargetCleanPolicy
		want   string
	}{
		{TargetCleanPolicy{Mode: TargetCleanModePerBead}, "per_bead"},
		{TargetCleanPolicy{Mode: TargetCleanModeNever}, "never"},
		{TargetCleanPolicy{Mode: TargetCleanModeEveryNBeads, EveryN: 7}, "every_n_beads:7"},
		{TargetCleanPolicy{}, "per_bead"}, // unknown falls back to per_bead
	}
	for _, tc := range tests {
		if got := tc.policy.String(); got != tc.want {
			t.Errorf("policy %+v String() = %q, want %q", tc.policy, got, tc.want)
		}
	}
}

func TestShouldCleanTarget(t *testing.T) {
	perBead := TargetCleanPolicy{Mode: TargetCleanModePerBead}
	never := TargetCleanPolicy{Mode: TargetCleanModeNever}
	every3 := TargetCleanPolicy{Mode: TargetCleanModeEveryNBeads, EveryN: 3}

	// per_bead: always clean
	for i := 1; i <= 5; i++ {
		if !ShouldCleanTarget(perBead, i) {
			t.Errorf("per_bead at counter=%d: want clean=true", i)
		}
	}
	// never: never clean
	for i := 1; i <= 5; i++ {
		if ShouldCleanTarget(never, i) {
			t.Errorf("never at counter=%d: want clean=false", i)
		}
	}
	// every_n_beads:3 — clean at counter=3, 6, 9
	wantClean := map[int]bool{1: false, 2: false, 3: true, 4: false, 5: false, 6: true, 7: false, 8: false, 9: true}
	for c, want := range wantClean {
		if got := ShouldCleanTarget(every3, c); got != want {
			t.Errorf("every_n_beads:3 at counter=%d: got %v, want %v", c, got, want)
		}
	}

	// Guard: every_n_beads with EveryN<=0 must not panic and must not clean.
	bad := TargetCleanPolicy{Mode: TargetCleanModeEveryNBeads, EveryN: 0}
	if ShouldCleanTarget(bad, 5) {
		t.Errorf("every_n_beads:0 should never clean (defensive)")
	}
}

func TestCounterPersistence(t *testing.T) {
	dir := t.TempDir()
	// Missing file → 0
	if got := readTargetCleanCounter(dir); got != 0 {
		t.Errorf("missing counter file: got %d, want 0", got)
	}
	// Write + read round-trip
	if err := writeTargetCleanCounter(dir, 7); err != nil {
		t.Fatalf("write counter: %v", err)
	}
	if got := readTargetCleanCounter(dir); got != 7 {
		t.Errorf("after write 7: got %d", got)
	}
	// Garbage file → 0 (don't error)
	if err := os.WriteFile(targetCleanCounterFile(dir), []byte("not a number"), 0o644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	if got := readTargetCleanCounter(dir); got != 0 {
		t.Errorf("garbage counter file: got %d, want 0", got)
	}
	// Negative file → 0
	if err := os.WriteFile(targetCleanCounterFile(dir), []byte("-5"), 0o644); err != nil {
		t.Fatalf("write negative: %v", err)
	}
	if got := readTargetCleanCounter(dir); got != 0 {
		t.Errorf("negative counter file: got %d, want 0", got)
	}
}

func TestCleanTargetDir(t *testing.T) {
	clonePath := t.TempDir()
	targetDir := filepath.Join(clonePath, "target")
	debugDir := filepath.Join(targetDir, "debug")
	if err := os.MkdirAll(debugDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	payload := []byte(strings.Repeat("X", 4096))
	if err := os.WriteFile(filepath.Join(debugDir, "artifact.o"), payload, 0o644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	bytes, didClean, err := cleanTargetDir(clonePath)
	if err != nil {
		t.Fatalf("cleanTargetDir: %v", err)
	}
	if !didClean {
		t.Errorf("didClean: want true")
	}
	if bytes < int64(len(payload)) {
		t.Errorf("bytes freed = %d, expected >= %d", bytes, len(payload))
	}
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Errorf("target dir still exists after clean: %v", err)
	}

	// Re-running on a missing target/ must be a no-op, not an error.
	bytes2, didClean2, err := cleanTargetDir(clonePath)
	if err != nil {
		t.Fatalf("second cleanTargetDir on missing target/: %v", err)
	}
	if didClean2 || bytes2 != 0 {
		t.Errorf("expected no-op on missing target/, got didClean=%v bytes=%d", didClean2, bytes2)
	}
}

func TestCleanTargetDirSafety(t *testing.T) {
	// Empty path rejected.
	if _, _, err := cleanTargetDir(""); err == nil {
		t.Error("empty clonePath should error")
	}
	// Relative path rejected (defense-in-depth — caller should pass absolute).
	if _, _, err := cleanTargetDir("relative/path"); err == nil {
		t.Error("relative clonePath should error")
	}
	// "target" being a regular file (not a dir) is refused.
	clonePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(clonePath, "target"), []byte("oops"), 0o644); err != nil {
		t.Fatalf("writefile: %v", err)
	}
	_, didClean, err := cleanTargetDir(clonePath)
	if err == nil {
		t.Error("file named target/ (not a dir) should error")
	}
	if didClean {
		t.Error("file named target/ should not be deleted")
	}
	if _, err := os.Stat(filepath.Join(clonePath, "target")); err != nil {
		t.Errorf("file target should still exist: %v", err)
	}
}

func TestRunTargetCleanHook_PerBead(t *testing.T) {
	polecatDir := t.TempDir()
	clonePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(clonePath, "target", "debug"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clonePath, "target", "debug", "x.o"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg, err := RunTargetCleanHook(polecatDir, clonePath, TargetCleanPolicy{Mode: TargetCleanModePerBead})
	if err != nil {
		t.Fatalf("RunTargetCleanHook: %v", err)
	}
	if msg == "" {
		t.Errorf("per_bead with existing target/: expected log message, got empty")
	}
	if _, err := os.Stat(filepath.Join(clonePath, "target")); !os.IsNotExist(err) {
		t.Errorf("target/ should be gone")
	}
	if readTargetCleanCounter(polecatDir) != 1 {
		t.Errorf("counter should be 1, got %d", readTargetCleanCounter(polecatDir))
	}
}

func TestRunTargetCleanHook_Never(t *testing.T) {
	polecatDir := t.TempDir()
	clonePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(clonePath, "target"), 0o755); err != nil {
		t.Fatal(err)
	}

	msg, err := RunTargetCleanHook(polecatDir, clonePath, TargetCleanPolicy{Mode: TargetCleanModeNever})
	if err != nil {
		t.Fatalf("RunTargetCleanHook: %v", err)
	}
	if msg != "" {
		t.Errorf("never: expected no log message, got %q", msg)
	}
	if _, err := os.Stat(filepath.Join(clonePath, "target")); err != nil {
		t.Errorf("never: target/ should still exist: %v", err)
	}
	// never mode should NOT touch the counter
	if got := readTargetCleanCounter(polecatDir); got != 0 {
		t.Errorf("never: counter should stay 0, got %d", got)
	}
}

func TestRunTargetCleanHook_EveryNBeads(t *testing.T) {
	polecatDir := t.TempDir()
	clonePath := t.TempDir()
	policy := TargetCleanPolicy{Mode: TargetCleanModeEveryNBeads, EveryN: 3}

	// Helper: re-stage target/ before each reuse (real worktree gets rebuilt).
	restage := func() {
		if err := os.MkdirAll(filepath.Join(clonePath, "target"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Beads 1, 2: counter increments but no clean.
	for i := 1; i <= 2; i++ {
		restage()
		msg, err := RunTargetCleanHook(polecatDir, clonePath, policy)
		if err != nil {
			t.Fatalf("reuse %d: %v", i, err)
		}
		if msg != "" {
			t.Errorf("reuse %d (every_n:3): expected no clean, got %q", i, msg)
		}
		if _, err := os.Stat(filepath.Join(clonePath, "target")); err != nil {
			t.Errorf("reuse %d: target/ should still exist", i)
		}
		if got := readTargetCleanCounter(polecatDir); got != i {
			t.Errorf("reuse %d: counter = %d, want %d", i, got, i)
		}
	}
	// Bead 3: clean fires.
	restage()
	msg, err := RunTargetCleanHook(polecatDir, clonePath, policy)
	if err != nil {
		t.Fatalf("reuse 3: %v", err)
	}
	if msg == "" {
		t.Errorf("reuse 3 (every_n:3): expected clean log, got empty")
	}
	if _, err := os.Stat(filepath.Join(clonePath, "target")); !os.IsNotExist(err) {
		t.Errorf("reuse 3: target/ should be gone")
	}
	if got := readTargetCleanCounter(polecatDir); got != 3 {
		t.Errorf("reuse 3: counter = %d, want 3", got)
	}
	// Bead 4: increments to 4, no clean.
	restage()
	msg, err = RunTargetCleanHook(polecatDir, clonePath, policy)
	if err != nil {
		t.Fatalf("reuse 4: %v", err)
	}
	if msg != "" {
		t.Errorf("reuse 4: expected no clean, got %q", msg)
	}
	if got := readTargetCleanCounter(polecatDir); got != 4 {
		t.Errorf("reuse 4: counter = %d, want 4", got)
	}
}

func TestRunTargetCleanHook_MissingTargetIsNoop(t *testing.T) {
	polecatDir := t.TempDir()
	clonePath := t.TempDir()
	// No target/ directory at all (first-ever reuse / non-Rust rig).
	msg, err := RunTargetCleanHook(polecatDir, clonePath, TargetCleanPolicy{Mode: TargetCleanModePerBead})
	if err != nil {
		t.Fatalf("RunTargetCleanHook with no target/: %v", err)
	}
	if msg != "" {
		t.Errorf("missing target/: expected no log message, got %q", msg)
	}
	// Counter still increments — that's by design (policy says "clean every reuse").
	if got := readTargetCleanCounter(polecatDir); got != 1 {
		t.Errorf("counter should be 1 even when target/ was missing, got %d", got)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{30 * 1024 * 1024 * 1024, "30.0 GiB"},
	}
	for _, tc := range tests {
		if got := formatBytes(tc.n); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
