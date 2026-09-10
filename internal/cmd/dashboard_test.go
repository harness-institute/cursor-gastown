package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestDashboardCmd_FlagsExist(t *testing.T) {
	// Verify required flags exist with correct defaults
	portFlag := dashboardCmd.Flags().Lookup("port")
	if portFlag == nil {
		t.Fatal("--port flag should exist")
	}
	if portFlag.DefValue != "8080" {
		t.Errorf("--port default should be 8080, got %s", portFlag.DefValue)
	}

	bindFlag := dashboardCmd.Flags().Lookup("bind")
	if bindFlag == nil {
		t.Fatal("--bind flag should exist")
	}
	wantBind := "127.0.0.1"
	if os.Getenv("IS_SANDBOX") != "" {
		wantBind = "0.0.0.0"
	}
	if bindFlag.DefValue != wantBind {
		t.Errorf("--bind default should be %s, got %s", wantBind, bindFlag.DefValue)
	}

	openFlag := dashboardCmd.Flags().Lookup("open")
	if openFlag == nil {
		t.Fatal("--open flag should exist")
	}
	if openFlag.DefValue != "false" {
		t.Errorf("--open default should be false, got %s", openFlag.DefValue)
	}
}

func TestDashboardCmd_IsRegistered(t *testing.T) {
	// Verify command is registered under root
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "dashboard" {
			found = true
			break
		}
	}
	if !found {
		t.Error("dashboard command should be registered with rootCmd")
	}
}

func TestDashboardCmd_HasCorrectGroup(t *testing.T) {
	if dashboardCmd.GroupID != GroupDiag {
		t.Errorf("dashboard should be in diag group, got %s", dashboardCmd.GroupID)
	}
}

func TestDashboardCmd_RequiresWorkspace(t *testing.T) {
	// Create a test command that simulates running outside workspace
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})

	// The actual workspace check happens in runDashboard
	// This test verifies the command structure is correct
	if dashboardCmd.RunE == nil {
		t.Error("dashboard command should have RunE set")
	}
}

func TestEnsureDoltPortEnv_ReadsConfigYAML(t *testing.T) {
	// Create a temporary town root with durable Dolt config.
	townRoot := t.TempDir()
	doltDataDir := filepath.Join(townRoot, ".dolt-data")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(doltDataDir, "config.yaml"), []byte("listener:\n  host: 127.0.0.2\n  port: 13307\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Clear any existing env vars
	t.Setenv("GT_DOLT_HOST", "")
	t.Setenv("GT_DOLT_PORT", "")
	t.Setenv("BEADS_DOLT_SERVER_HOST", "stale-host")
	t.Setenv("BEADS_DOLT_SERVER_PORT", "")
	t.Setenv("BEADS_DOLT_PORT", "")

	ensureDoltPortEnv(townRoot)

	if got := os.Getenv("GT_DOLT_PORT"); got != "13307" {
		t.Errorf("GT_DOLT_PORT = %q, want %q", got, "13307")
	}
	if got := os.Getenv("BEADS_DOLT_PORT"); got != "13307" {
		t.Errorf("BEADS_DOLT_PORT = %q, want %q", got, "13307")
	}
	if got := os.Getenv("BEADS_DOLT_SERVER_PORT"); got != "13307" {
		t.Errorf("BEADS_DOLT_SERVER_PORT = %q, want %q", got, "13307")
	}
	if got := os.Getenv("GT_DOLT_HOST"); got != "127.0.0.2" {
		t.Errorf("GT_DOLT_HOST = %q, want %q", got, "127.0.0.2")
	}
	if got := os.Getenv("BEADS_DOLT_SERVER_HOST"); got != "127.0.0.2" {
		t.Errorf("BEADS_DOLT_SERVER_HOST = %q, want %q", got, "127.0.0.2")
	}
}

func TestEnsureDoltPortEnv_FallsBackToDefault(t *testing.T) {
	// Use a temp dir with no durable Dolt config.
	townRoot := t.TempDir()

	t.Setenv("GT_DOLT_HOST", "")
	t.Setenv("GT_DOLT_PORT", "")
	t.Setenv("BEADS_DOLT_SERVER_HOST", "stale-host")
	t.Setenv("BEADS_DOLT_SERVER_PORT", "")
	t.Setenv("BEADS_DOLT_PORT", "")

	ensureDoltPortEnv(townRoot)

	want := "3307"
	if got := os.Getenv("GT_DOLT_PORT"); got != want {
		t.Errorf("GT_DOLT_PORT = %q, want %q (default)", got, want)
	}
	if got := os.Getenv("BEADS_DOLT_PORT"); got != want {
		t.Errorf("BEADS_DOLT_PORT = %q, want %q (default)", got, want)
	}
	if got := os.Getenv("BEADS_DOLT_SERVER_PORT"); got != want {
		t.Errorf("BEADS_DOLT_SERVER_PORT = %q, want %q (default)", got, want)
	}
	if got, ok := os.LookupEnv("BEADS_DOLT_SERVER_HOST"); ok {
		t.Errorf("BEADS_DOLT_SERVER_HOST = %q, want unset", got)
	}
}

func TestEnsureDoltPortEnv_GTDoltPortOverridesWrongBeadsPort(t *testing.T) {
	// Simulate the bug: Beads port aliases set to dashboard HTTP port (8080)
	// while GT_DOLT_PORT carries the explicit Dolt endpoint.
	t.Setenv("GT_DOLT_PORT", "3307")
	t.Setenv("BEADS_DOLT_SERVER_PORT", "8080")
	t.Setenv("BEADS_DOLT_PORT", "8080")

	// Create durable config with a different port. Explicit GT_DOLT_PORT is still
	// authoritative for dashboard-spawned subprocesses.
	townRoot := t.TempDir()
	doltDataDir := filepath.Join(townRoot, ".dolt-data")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(doltDataDir, "config.yaml"), []byte("listener:\n  port: 3308\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ensureDoltPortEnv(townRoot)

	if got := os.Getenv("GT_DOLT_PORT"); got != "3307" {
		t.Errorf("GT_DOLT_PORT = %q, want %q", got, "3307")
	}
	if got := os.Getenv("BEADS_DOLT_PORT"); got != "3307" {
		t.Errorf("BEADS_DOLT_PORT = %q, want %q", got, "3307")
	}
	if got := os.Getenv("BEADS_DOLT_SERVER_PORT"); got != "3307" {
		t.Errorf("BEADS_DOLT_SERVER_PORT = %q, want %q", got, "3307")
	}
}
