package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func installMockBdInitOnly(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	if runtime.GOOS == "windows" {
		psPath := filepath.Join(binDir, "bd.ps1")
		psScript := `$target = Join-Path (Get-Location) '.beads'
foreach ($arg in $args) {
  if ($arg -eq 'init') {
    New-Item -ItemType Directory -Force -Path $target | Out-Null
    Set-Content -Path (Join-Path $target 'config.yaml') -Value @('prefix: tr', 'issue-prefix: tr-')
    exit 0
  }
}
exit 0
`
		cmdScript := "@echo off\r\npwsh -NoProfile -NoLogo -File \"" + psPath + "\" %*\r\n"
		if err := os.WriteFile(psPath, []byte(psScript), 0644); err != nil {
			t.Fatalf("write mock bd ps1: %v", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "bd.cmd"), []byte(cmdScript), 0644); err != nil {
			t.Fatalf("write mock bd cmd: %v", err)
		}
	} else {
		script := `#!/bin/sh
	if [ -n "$BD_ARGS_LOG" ]; then
	  printf 'args=%s env=%s beads=%s db=%s\n' "$*" "${BEADS_DOLT_SERVER_DATABASE:-<unset>}" "${BEADS_DIR:-<unset>}" "${BEADS_DB:-<unset>}" >> "$BD_ARGS_LOG"
	fi
	target="$(pwd)/.beads"
	mkdir -p "$target"
	printf 'prefix: tr\nissue-prefix: tr-\n' > "$target/config.yaml"
exit 0
`
		if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0755); err != nil {
			t.Fatalf("write mock bd: %v", err)
		}
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestBeadsRedirectCheck_FixInitBeadsUsesCanonicalDatabase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock bd arg logging is shell-specific")
	}
	installMockBdInitOnly(t)

	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{
		"version": 1,
		"rigs": {
			"testrig": {
				"git_url": "https://example.com/test.git",
				"beads": {"prefix": "tr"}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	argsLog := filepath.Join(t.TempDir(), "bd-args.log")
	t.Setenv("BD_ARGS_LOG", argsLog)
	t.Setenv("BEADS_DIR", filepath.Join(tmpDir, "wrong", ".beads"))
	t.Setenv("BEADS_DB", filepath.Join(tmpDir, "wrong.db"))
	t.Setenv("BEADS_DOLT_SERVER_DATABASE", "wrong_db")

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	logData, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("reading bd args log: %v", err)
	}
	log := string(logData)
	if !strings.Contains(log, "args=init --prefix tr --database testrig --server --server-port") {
		t.Fatalf("bd init did not use canonical rig database; log:\n%s", log)
	}
	if !strings.Contains(log, "env=testrig") || !strings.Contains(log, "beads="+filepath.Join(rigDir, ".beads")) {
		t.Fatalf("bd init did not receive canonical env; log:\n%s", log)
	}
	if strings.Contains(log, "wrong_db") || strings.Contains(log, "wrong.db") || strings.Contains(log, filepath.Join(tmpDir, "wrong", ".beads")) {
		t.Fatalf("stale BEADS env leaked into bd subprocess; log:\n%s", log)
	}
}

func TestNewBeadsRedirectCheck(t *testing.T) {
	check := NewBeadsRedirectCheck()

	if check.Name() != "beads-redirect" {
		t.Errorf("expected name 'beads-redirect', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestBeadsRedirectCheck_NoRigSpecified(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: ""}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rig specified, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "skipping") {
		t.Errorf("expected message about skipping, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_NoBeadsAtAll(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError when no beads exist (fixable), got %v", result.Status)
	}
}

func TestBeadsRedirectCheck_LocalBeadsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create local beads at rig root (no mayor/rig/.beads)
	localBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(localBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for local beads (no redirect needed), got %v", result.Status)
	}
	if !strings.Contains(result.Message, "local beads") {
		t.Errorf("expected message about local beads, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_TrackedBeadsMissingRedirect(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing redirect, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "Missing") {
		t.Errorf("expected message about missing redirect, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_TrackedBeadsCorrectRedirect(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig-level .beads with correct redirect
	rigBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	redirectPath := filepath.Join(rigBeads, "redirect")
	if err := os.WriteFile(redirectPath, []byte("mayor/rig/.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for correct redirect, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "correctly configured") {
		t.Errorf("expected message about correct config, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_TrackedBeadsWrongRedirect(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig-level .beads with wrong redirect
	rigBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	redirectPath := filepath.Join(rigBeads, "redirect")
	if err := os.WriteFile(redirectPath, []byte("wrong/path\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong redirect (fixable), got %v", result.Status)
	}
	if !strings.Contains(result.Message, "wrong/path") {
		t.Errorf("expected message to contain wrong path, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_FixWrongRedirect(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig-level .beads with wrong redirect
	rigBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	redirectPath := filepath.Join(rigBeads, "redirect")
	if err := os.WriteFile(redirectPath, []byte("wrong/path\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify redirect was corrected
	content, err := os.ReadFile(redirectPath)
	if err != nil {
		t.Fatalf("redirect file not found: %v", err)
	}
	if string(content) != "mayor/rig/.beads\n" {
		t.Errorf("redirect content = %q, want 'mayor/rig/.beads\\n'", string(content))
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestBeadsRedirectCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify redirect file was created
	redirectPath := filepath.Join(rigDir, ".beads", "redirect")
	content, err := os.ReadFile(redirectPath)
	if err != nil {
		t.Fatalf("redirect file not created: %v", err)
	}

	expected := "mayor/rig/.beads\n"
	if string(content) != expected {
		t.Errorf("redirect content = %q, want %q", string(content), expected)
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestBeadsRedirectCheck_FixNoOp_LocalBeads(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create only local beads (no tracked beads)
	localBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(localBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Fix should be a no-op
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify no redirect was created
	redirectPath := filepath.Join(rigDir, ".beads", "redirect")
	if _, err := os.Stat(redirectPath); !os.IsNotExist(err) {
		t.Error("redirect file should not be created for local beads")
	}
}

func TestBeadsRedirectCheck_FixInitBeads(t *testing.T) {
	installMockBdInitOnly(t)

	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create rig directory (no beads at all)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mayor/rigs.json with prefix for the rig
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{
		"version": 1,
		"rigs": {
			"testrig": {
				"git_url": "https://example.com/test.git",
				"beads": {
					"prefix": "tr"
				}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - this will run 'bd init' if available, otherwise create config.yaml
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify .beads directory was created
	beadsDir := filepath.Join(rigDir, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		t.Fatal(".beads directory not created")
	}

	// Verify beads was initialized (either by bd init or fallback)
	// bd init creates config.yaml, fallback creates config.yaml with prefix
	configPath := filepath.Join(beadsDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config.yaml not created")
	}

	// Verify check now passes (local beads exist)
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestBeadsRedirectCheck_ConflictingLocalBeads(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}
	// Add some content to tracked beads
	if err := os.WriteFile(filepath.Join(trackedBeads, "issues.jsonl"), []byte(`{"id":"tr-1"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create conflicting local beads with actual data
	localBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(localBeads, 0755); err != nil {
		t.Fatal(err)
	}
	// Add data to local beads (this is the conflict)
	if err := os.WriteFile(filepath.Join(localBeads, "issues.jsonl"), []byte(`{"id":"local-1"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localBeads, "config.yaml"), []byte("prefix: local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Check should detect conflicting beads
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected StatusError for conflicting beads, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "Conflicting") {
		t.Errorf("expected message about conflicting beads, got %q", result.Message)
	}
}

func TestDefaultBranchExistsCheck_NoRig(t *testing.T) {
	check := NewDefaultBranchExistsCheck()
	ctx := &CheckContext{TownRoot: t.TempDir(), RigName: ""}

	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected StatusError with no rig, got %v", result.Status)
	}
}

func TestDefaultBranchExistsCheck_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewDefaultBranchExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning with no config, got %v", result.Status)
	}
}

func TestDefaultBranchExistsCheck_EmptyDefaultBranch(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write config with no default_branch
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"name":"testrig"}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewDefaultBranchExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK with no default_branch, got %v", result.Status)
	}
}

func TestDefaultBranchExistsCheck_NoBareRepo(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{"default_branch":"main"}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewDefaultBranchExistsCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no bare repo, got %v", result.Status)
	}
}

func TestDefaultBranchExistsCheck_NotFixable(t *testing.T) {
	check := NewDefaultBranchExistsCheck()
	if check.CanFix() {
		t.Error("DefaultBranchExistsCheck should not be fixable")
	}
}

func TestBeadsRedirectCheck_FixConflictingLocalBeads(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads with config.yaml as data marker
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trackedBeads, "config.yaml"), []byte("prefix: tr\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create conflicting local beads with actual data
	localBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(localBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localBeads, "config.yaml"), []byte("prefix: local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - should remove conflicting local beads and create redirect
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify redirect was created
	redirectPath := filepath.Join(localBeads, "redirect")
	content, err := os.ReadFile(redirectPath)
	if err != nil {
		t.Fatalf("redirect file not created: %v", err)
	}
	if string(content) != "mayor/rig/.beads\n" {
		t.Errorf("redirect content = %q, want 'mayor/rig/.beads\\n'", string(content))
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}
