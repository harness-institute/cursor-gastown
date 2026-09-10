package doltserver

import (
	"errors"
	"testing"
)

// TestDefaultConfig_TimeZoneEmptyEnvOptsOut verifies that an explicitly empty
// GT_DOLT_TIME_ZONE disables the override (caller wants Dolt's host-TZ default).
func TestDefaultConfig_TimeZoneEmptyEnvOptsOut(t *testing.T) {
	townRoot := t.TempDir()
	t.Setenv("GT_DOLT_TIME_ZONE", "")

	config := DefaultConfig(townRoot)

	if config.TimeZone != "" {
		t.Errorf("TimeZone = %q with explicit empty env, want empty (opt-out)", config.TimeZone)
	}
}

// TestDefaultConfig_TimeZoneEnvOverride verifies that GT_DOLT_TIME_ZONE
// replaces the default.
func TestDefaultConfig_TimeZoneEnvOverride(t *testing.T) {
	townRoot := t.TempDir()
	t.Setenv("GT_DOLT_TIME_ZONE", "America/Los_Angeles")

	config := DefaultConfig(townRoot)

	if config.TimeZone != "America/Los_Angeles" {
		t.Errorf("TimeZone = %q, want America/Los_Angeles", config.TimeZone)
	}
}

// TestBuildTimeZoneQuery verifies the exact SQL emitted by applyTimeZone.
func TestBuildTimeZoneQuery(t *testing.T) {
	cases := []struct {
		tz   string
		want string
	}{
		{"+00:00", "SET GLOBAL time_zone = '+00:00'"},
		{"UTC", "SET GLOBAL time_zone = 'UTC'"},
		{"America/Los_Angeles", "SET GLOBAL time_zone = 'America/Los_Angeles'"},
	}
	for _, tc := range cases {
		got := buildTimeZoneQuery(tc.tz)
		if got != tc.want {
			t.Errorf("buildTimeZoneQuery(%q) = %q, want %q", tc.tz, got, tc.want)
		}
	}
}

// TestApplyTimeZone_EmptyShortCircuits verifies that an empty TimeZone opts
// out of the SET GLOBAL — the SQL seam must never be invoked.
func TestApplyTimeZone_EmptyShortCircuits(t *testing.T) {
	prev := applyTimeZoneFn
	t.Cleanup(func() { applyTimeZoneFn = prev })

	called := false
	applyTimeZoneFn = func(_, _ string) error {
		called = true
		return nil
	}

	applyTimeZone("/some/town", &Config{TimeZone: ""})

	if called {
		t.Errorf("applyTimeZoneFn called for empty TimeZone")
	}
}

// TestApplyTimeZone_DispatchesQuery verifies that a non-empty TimeZone
// dispatches the expected SET GLOBAL statement against the seam.
func TestApplyTimeZone_DispatchesQuery(t *testing.T) {
	prev := applyTimeZoneFn
	t.Cleanup(func() { applyTimeZoneFn = prev })

	var gotTownRoot, gotQuery string
	applyTimeZoneFn = func(townRoot, query string) error {
		gotTownRoot = townRoot
		gotQuery = query
		return nil
	}

	applyTimeZone("/town/root", &Config{TimeZone: "+00:00"})

	if gotTownRoot != "/town/root" {
		t.Errorf("townRoot = %q, want /town/root", gotTownRoot)
	}
	if want := "SET GLOBAL time_zone = '+00:00'"; gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

// TestApplyTimeZone_ErrorIsBestEffort verifies that a SQL failure does not
// panic or propagate.
func TestApplyTimeZone_ErrorIsBestEffort(t *testing.T) {
	prev := applyTimeZoneFn
	t.Cleanup(func() { applyTimeZoneFn = prev })

	applyTimeZoneFn = func(_, _ string) error {
		return errors.New("simulated SQL failure")
	}

	applyTimeZone("/town/root", &Config{TimeZone: "+00:00"})
}
