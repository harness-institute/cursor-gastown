package doltserver

import (
	"errors"
	"testing"
)

// TestDefaultConfig_WaitTimeoutDefault verifies that the default config
// applies the gh-3623 idle-session timeout.
func TestDefaultConfig_WaitTimeoutDefault(t *testing.T) {
	townRoot := t.TempDir()
	t.Setenv("GT_DOLT_WAIT_TIMEOUT", "")

	config := DefaultConfig(townRoot)

	if config.WaitTimeoutSec != DefaultWaitTimeoutSec {
		t.Errorf("WaitTimeoutSec = %d, want %d", config.WaitTimeoutSec, DefaultWaitTimeoutSec)
	}
}

// TestDefaultConfig_WaitTimeoutEnvOverride verifies the GT_DOLT_WAIT_TIMEOUT
// env var raises or lowers the configured timeout.
func TestDefaultConfig_WaitTimeoutEnvOverride(t *testing.T) {
	townRoot := t.TempDir()
	t.Setenv("GT_DOLT_WAIT_TIMEOUT", "120")

	config := DefaultConfig(townRoot)

	if config.WaitTimeoutSec != 120 {
		t.Errorf("WaitTimeoutSec = %d, want 120", config.WaitTimeoutSec)
	}
}

// TestDefaultConfig_WaitTimeoutNegativeDisables verifies that a negative
// value opts out of the override, leaving Dolt's default in place.
func TestDefaultConfig_WaitTimeoutNegativeDisables(t *testing.T) {
	townRoot := t.TempDir()
	t.Setenv("GT_DOLT_WAIT_TIMEOUT", "-1")

	config := DefaultConfig(townRoot)

	if config.WaitTimeoutSec != 0 {
		t.Errorf("WaitTimeoutSec = %d, want 0 (disabled)", config.WaitTimeoutSec)
	}
}

// TestDefaultConfig_WaitTimeoutInvalidIgnored verifies that a non-numeric
// env value falls back to the default rather than zeroing the timeout.
func TestDefaultConfig_WaitTimeoutInvalidIgnored(t *testing.T) {
	townRoot := t.TempDir()
	t.Setenv("GT_DOLT_WAIT_TIMEOUT", "not-a-number")

	config := DefaultConfig(townRoot)

	if config.WaitTimeoutSec != DefaultWaitTimeoutSec {
		t.Errorf("WaitTimeoutSec = %d, want default %d when env var is invalid", config.WaitTimeoutSec, DefaultWaitTimeoutSec)
	}
}

// TestBuildWaitTimeoutQuery verifies the exact SQL emitted by applyWaitTimeout.
func TestBuildWaitTimeoutQuery(t *testing.T) {
	got := buildWaitTimeoutQuery(30)
	want := "SET GLOBAL wait_timeout = 30"
	if got != want {
		t.Errorf("buildWaitTimeoutQuery(30) = %q, want %q", got, want)
	}
}

// TestApplyWaitTimeout_DisabledShortCircuits verifies that a non-positive
// WaitTimeoutSec opts out of the SET GLOBAL — the SQL seam must never be
// invoked.
func TestApplyWaitTimeout_DisabledShortCircuits(t *testing.T) {
	prev := applyWaitTimeoutFn
	t.Cleanup(func() { applyWaitTimeoutFn = prev })

	called := false
	applyWaitTimeoutFn = func(_, _ string) error {
		called = true
		return nil
	}

	for _, sec := range []int{0, -1, -3600} {
		applyWaitTimeout("/some/town", &Config{WaitTimeoutSec: sec})
	}

	if called {
		t.Errorf("applyWaitTimeoutFn called for non-positive WaitTimeoutSec")
	}
}

// TestApplyWaitTimeout_DispatchesQuery verifies that a positive timeout
// dispatches the expected SET GLOBAL statement against the seam.
func TestApplyWaitTimeout_DispatchesQuery(t *testing.T) {
	prev := applyWaitTimeoutFn
	t.Cleanup(func() { applyWaitTimeoutFn = prev })

	var gotTownRoot, gotQuery string
	applyWaitTimeoutFn = func(townRoot, query string) error {
		gotTownRoot = townRoot
		gotQuery = query
		return nil
	}

	applyWaitTimeout("/town/root", &Config{WaitTimeoutSec: 45})

	if gotTownRoot != "/town/root" {
		t.Errorf("townRoot = %q, want /town/root", gotTownRoot)
	}
	if want := "SET GLOBAL wait_timeout = 45"; gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

// TestApplyWaitTimeout_ErrorIsBestEffort verifies that a SQL failure does
// not panic or propagate. The Dolt server is already up by the time we
// apply this; failing here would needlessly fail the start.
func TestApplyWaitTimeout_ErrorIsBestEffort(t *testing.T) {
	prev := applyWaitTimeoutFn
	t.Cleanup(func() { applyWaitTimeoutFn = prev })

	applyWaitTimeoutFn = func(_, _ string) error {
		return errors.New("simulated SQL failure")
	}

	// Must not panic. No return value to check — best-effort by contract.
	applyWaitTimeout("/town/root", &Config{WaitTimeoutSec: 30})
}
