package daemon

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckpointDogInterval_Default(t *testing.T) {
	interval := checkpointDogInterval(nil)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultCheckpointDogInterval, interval)
	}
}

func TestCheckpointDogInterval_NilPatrols(t *testing.T) {
	config := &DaemonPatrolConfig{}
	interval := checkpointDogInterval(config)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultCheckpointDogInterval, interval)
	}
}

func TestCheckpointDogInterval_NilCheckpointDog(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{},
	}
	interval := checkpointDogInterval(config)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultCheckpointDogInterval, interval)
	}
}

func TestCheckpointDogInterval_Configured(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CheckpointDog: &CheckpointDogConfig{
				Enabled:     true,
				IntervalStr: "5m",
			},
		},
	}
	interval := checkpointDogInterval(config)
	if interval != 5*time.Minute {
		t.Errorf("expected 5m, got %v", interval)
	}
}

func TestCheckpointDogInterval_InvalidFallsBack(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CheckpointDog: &CheckpointDogConfig{
				Enabled:     true,
				IntervalStr: "not-a-duration",
			},
		},
	}
	interval := checkpointDogInterval(config)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval for invalid config, got %v", interval)
	}
}

func TestCheckpointDogInterval_ZeroFallsBack(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CheckpointDog: &CheckpointDogConfig{
				Enabled:     true,
				IntervalStr: "0s",
			},
		},
	}
	interval := checkpointDogInterval(config)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval for zero config, got %v", interval)
	}
}

func TestCheckpointDogEnabled(t *testing.T) {
	// Nil config → disabled (opt-in patrol)
	if IsPatrolEnabled(nil, "checkpoint_dog") {
		t.Error("expected checkpoint_dog disabled for nil config")
	}

	// Explicitly enabled
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CheckpointDog: &CheckpointDogConfig{
				Enabled: true,
			},
		},
	}
	if !IsPatrolEnabled(config, "checkpoint_dog") {
		t.Error("expected checkpoint_dog enabled")
	}

	// Explicitly disabled
	config.Patrols.CheckpointDog.Enabled = false
	if IsPatrolEnabled(config, "checkpoint_dog") {
		t.Error("expected checkpoint_dog disabled when Enabled=false")
	}
}

func TestResolveCheckpointWorkDir_NestedLayout(t *testing.T) {
	// New polecat layout: polecats/<name>/<rigName>/.git is the worktree.
	tmp := t.TempDir()
	rig := "myrig"
	polecat := "alice"
	polecatsDir := filepath.Join(tmp, "polecats")
	worktree := filepath.Join(polecatsDir, polecat, rig)
	if err := os.MkdirAll(filepath.Join(worktree, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := resolveCheckpointWorkDir(polecatsDir, polecat, rig)
	if got != worktree {
		t.Errorf("got %q, want %q", got, worktree)
	}
}

func TestResolveCheckpointWorkDir_LegacyFlatLayout(t *testing.T) {
	// Legacy layout: polecats/<name>/.git directly. polecat.Manager still
	// recognizes this; checkpoint_dog must too rather than silently skip.
	tmp := t.TempDir()
	rig := "myrig"
	polecat := "bob"
	polecatsDir := filepath.Join(tmp, "polecats")
	worktree := filepath.Join(polecatsDir, polecat)
	if err := os.MkdirAll(filepath.Join(worktree, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := resolveCheckpointWorkDir(polecatsDir, polecat, rig)
	if got != worktree {
		t.Errorf("got %q, want %q (legacy flat layout)", got, worktree)
	}
}

func TestResolveCheckpointWorkDir_NoGitNeitherLevel(t *testing.T) {
	// Critical regression case: polecat container exists but has no .git
	// at either level. Function MUST return "" so the caller skips, NOT
	// fall back to a parent dir (which would have the workspace's .git
	// and cause the wrong-branch checkpoint bug this code prevents).
	tmp := t.TempDir()
	rig := "myrig"
	polecat := "carol"
	polecatsDir := filepath.Join(tmp, "polecats")
	if err := os.MkdirAll(filepath.Join(polecatsDir, polecat, rig), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Simulate top-level workspace .git that git would walk up to find.
	// resolveCheckpointWorkDir must NOT return a path that lets git walk
	// to this — it should return "" so the caller skips entirely.
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0o755); err != nil {
		t.Fatalf("setup parent .git: %v", err)
	}
	got := resolveCheckpointWorkDir(polecatsDir, polecat, rig)
	if got != "" {
		t.Errorf("got %q, want empty (skip — no polecat-level .git)", got)
	}
}

func TestResolveCheckpointWorkDir_PrefersNestedOverFlat(t *testing.T) {
	// If both levels have .git (transitional state during a migration),
	// prefer the nested (newer) layout.
	tmp := t.TempDir()
	rig := "myrig"
	polecat := "dave"
	polecatsDir := filepath.Join(tmp, "polecats")
	flat := filepath.Join(polecatsDir, polecat)
	nested := filepath.Join(flat, rig)
	for _, d := range []string{flat, nested} {
		if err := os.MkdirAll(filepath.Join(d, ".git"), 0o755); err != nil {
			t.Fatalf("setup %s: %v", d, err)
		}
	}
	got := resolveCheckpointWorkDir(polecatsDir, polecat, rig)
	if got != nested {
		t.Errorf("got %q, want nested %q", got, nested)
	}
}

func TestIsGitWorktree(t *testing.T) {
	tmp := t.TempDir()
	if isGitWorktree(tmp) {
		t.Error("empty dir should not be a worktree")
	}
	// .git as directory (full clone)
	dirGit := filepath.Join(tmp, "fullclone")
	if err := os.MkdirAll(filepath.Join(dirGit, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isGitWorktree(dirGit) {
		t.Error(".git directory should count as worktree")
	}
	// .git as file (linked worktree — git uses a file pointing to commondir)
	fileGit := filepath.Join(tmp, "linked")
	if err := os.MkdirAll(fileGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fileGit, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isGitWorktree(fileGit) {
		t.Error(".git file (linked worktree) should count as worktree")
	}
}

func TestCheckpointWorktreeExcludesNestedRuntimeArtifacts(t *testing.T) {
	workDir := t.TempDir()
	mustRunGit(t, workDir, "init")
	mustRunGit(t, workDir, "config", "user.name", "Checkpoint Dog")
	mustRunGit(t, workDir, "config", "user.email", "checkpoint@example.com")

	if err := os.MkdirAll(filepath.Join(workDir, "src"), 0o755); err != nil {
		t.Fatalf("setup src: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "web", ".beads"), 0o755); err != nil {
		t.Fatalf("setup nested runtime dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "src", "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "web", ".beads", "redirect"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write runtime file: %v", err)
	}
	mustRunGit(t, workDir, "add", "src/app.go", "web/.beads/redirect")
	mustRunGit(t, workDir, "commit", "-m", "initial")

	if err := os.WriteFile(filepath.Join(workDir, "src", "app.go"), []byte("package main\n// checkpoint me\n"), 0o644); err != nil {
		t.Fatalf("modify source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "web", ".beads", "redirect"), []byte("after\n"), 0o644); err != nil {
		t.Fatalf("modify runtime file: %v", err)
	}

	d := &Daemon{logger: log.New(io.Discard, "", 0)}
	if !d.checkpointWorktree(workDir, "rig", "polecat") {
		t.Fatal("checkpointWorktree did not create a checkpoint commit")
	}

	if got := strings.TrimSpace(mustRunGit(t, workDir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")); got != "src/app.go" {
		t.Fatalf("checkpoint commit changed %q, want only src/app.go", got)
	}
	if got := strings.TrimSpace(mustRunGit(t, workDir, "diff", "--cached", "--name-only")); got != "" {
		t.Fatalf("runtime artifact remained staged: %q", got)
	}
	if got := strings.TrimSpace(mustRunGit(t, workDir, "diff", "--", "web/.beads/redirect")); got == "" {
		t.Fatal("nested runtime artifact change was committed or lost; want it left unstaged in worktree")
	}
}

func TestCheckpointWorktreeSkipsRuntimeOnlyNestedArtifacts(t *testing.T) {
	workDir := t.TempDir()
	mustRunGit(t, workDir, "init")
	mustRunGit(t, workDir, "config", "user.name", "Checkpoint Dog")
	mustRunGit(t, workDir, "config", "user.email", "checkpoint@example.com")

	if err := os.MkdirAll(filepath.Join(workDir, "web", ".beads"), 0o755); err != nil {
		t.Fatalf("setup nested runtime dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "web", ".beads", "redirect"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write runtime file: %v", err)
	}
	mustRunGit(t, workDir, "add", "web/.beads/redirect")
	mustRunGit(t, workDir, "commit", "-m", "initial")
	before := mustRunGit(t, workDir, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(workDir, "web", ".beads", "redirect"), []byte("after\n"), 0o644); err != nil {
		t.Fatalf("modify runtime file: %v", err)
	}

	d := &Daemon{logger: log.New(io.Discard, "", 0)}
	if d.checkpointWorktree(workDir, "rig", "polecat") {
		t.Fatal("checkpointWorktree created a checkpoint for runtime-only changes")
	}
	if after := mustRunGit(t, workDir, "rev-parse", "HEAD"); after != before {
		t.Fatalf("checkpointWorktree advanced HEAD to %s, want %s", after, before)
	}
	if got := strings.TrimSpace(mustRunGit(t, workDir, "diff", "--cached", "--name-only")); got != "" {
		t.Fatalf("runtime artifact remained staged: %q", got)
	}
	if got := strings.TrimSpace(mustRunGit(t, workDir, "diff", "--", "web/.beads/redirect")); got == "" {
		t.Fatal("nested runtime artifact change was lost; want it left unstaged in worktree")
	}
}

func mustRunGit(t *testing.T, workDir string, args ...string) string {
	t.Helper()
	out, err := runGitCmd(workDir, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}
