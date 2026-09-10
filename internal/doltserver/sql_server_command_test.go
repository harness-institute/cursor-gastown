package doltserver

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestNewSQLServerCommandAddsRuntimeDefaults(t *testing.T) {
	unsetEnv(t, "GOMEMLIMIT")
	unsetEnv(t, "GOGC")

	dataDir := t.TempDir()
	configPath := dataDir + "/config.yaml"
	cmd := NewSQLServerCommand("dolt", dataDir, configPath)

	if cmd.Dir != dataDir {
		t.Fatalf("cmd.Dir = %q, want %q", cmd.Dir, dataDir)
	}
	wantArgs := []string{"dolt", "sql-server", "--config", configPath}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("cmd.Args = %v, want %v", cmd.Args, wantArgs)
	}
	for i, want := range wantArgs {
		if cmd.Args[i] != want {
			t.Fatalf("cmd.Args[%d] = %q, want %q", i, cmd.Args[i], want)
		}
	}

	if got := envValue(cmd.Env, "GOMEMLIMIT"); got != defaultDoltSQLServerGoMemLimit {
		t.Fatalf("GOMEMLIMIT = %q, want %q", got, defaultDoltSQLServerGoMemLimit)
	}
	if got := envValue(cmd.Env, "GOGC"); got != defaultDoltSQLServerGOGC {
		t.Fatalf("GOGC = %q, want %q", got, defaultDoltSQLServerGOGC)
	}
	if runtime.GOOS != "windows" {
		if got := envValue(cmd.Env, "PWD"); got != dataDir {
			t.Fatalf("PWD = %q, want %q", got, dataDir)
		}
	}
}

func TestDoltSQLServerEnvPreservesRuntimeOverrides(t *testing.T) {
	env := doltSQLServerEnv([]string{"GOMEMLIMIT=24GiB", "GOGC=100"})

	if got := envValue(env, "GOMEMLIMIT"); got != "24GiB" {
		t.Fatalf("GOMEMLIMIT = %q, want override", got)
	}
	if got := envValue(env, "GOGC"); got != "100" {
		t.Fatalf("GOGC = %q, want override", got)
	}
	if got := envKeyCount(env, "GOMEMLIMIT"); got != 1 {
		t.Fatalf("GOMEMLIMIT count = %d, want 1", got)
	}
	if got := envKeyCount(env, "GOGC"); got != 1 {
		t.Fatalf("GOGC count = %d, want 1", got)
	}
}

func TestDoltSQLServerEnvTreatsEmptyAndOffAsOverrides(t *testing.T) {
	env := doltSQLServerEnv([]string{"GOMEMLIMIT=off", "GOGC="})

	if got := envValue(env, "GOMEMLIMIT"); got != "off" {
		t.Fatalf("GOMEMLIMIT = %q, want off", got)
	}
	if got := envValue(env, "GOGC"); got != "" {
		t.Fatalf("GOGC = %q, want empty override", got)
	}
	if got := envKeyCount(env, "GOGC"); got != 1 {
		t.Fatalf("GOGC count = %d, want 1", got)
	}
}

func TestDoltSQLServerEnvPreservesWindowsCaseInsensitiveRuntimeOverrides(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want map[string]string
	}{
		{
			name: "lowercase",
			env:  []string{"gomemlimit=24GiB", "gogc=100"},
			want: map[string]string{"GOMEMLIMIT": "24GiB", "GOGC": "100"},
		},
		{
			name: "mixed empty off",
			env:  []string{"GoMemLimit=off", "GoGc="},
			want: map[string]string{"GOMEMLIMIT": "off", "GOGC": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := doltSQLServerEnvForGOOS(tt.env, "windows")
			for key, want := range tt.want {
				got, ok := envValueForGOOS(env, key, "windows")
				if !ok || got != want {
					t.Fatalf("%s = %q (found %v), want %q", key, got, ok, want)
				}
				if got := envKeyCountForGOOS(env, key, "windows"); got != 1 {
					t.Fatalf("%s count = %d, want 1", key, got)
				}
			}
		})
	}
}

func TestDoltSQLServerEnvKeepsPOSIXRuntimeKeysCaseSensitive(t *testing.T) {
	env := doltSQLServerEnvForGOOS([]string{"gomemlimit=24GiB", "GoGc=100"}, "linux")

	if got := envValue(env, "GOMEMLIMIT"); got != defaultDoltSQLServerGoMemLimit {
		t.Fatalf("GOMEMLIMIT = %q, want default", got)
	}
	if got := envValue(env, "GOGC"); got != defaultDoltSQLServerGOGC {
		t.Fatalf("GOGC = %q, want default", got)
	}
	if got := envValue(env, "gomemlimit"); got != "24GiB" {
		t.Fatalf("gomemlimit = %q, want preserved lowercase value", got)
	}
	if got := envValue(env, "GoGc"); got != "100" {
		t.Fatalf("GoGc = %q, want preserved mixed-case value", got)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			return entry[len(prefix):]
		}
	}
	return ""
}

func envKeyCount(env []string, key string) int {
	return envKeyCountForGOOS(env, key, "")
}

func envValueForGOOS(env []string, key, goos string) (string, bool) {
	for _, entry := range env {
		entryKey, value, ok := strings.Cut(entry, "=")
		if ok && envKeyMatches(entryKey, key, goos) {
			return value, true
		}
	}
	return "", false
}

func envKeyCountForGOOS(env []string, key, goos string) int {
	count := 0
	for _, entry := range env {
		entryKey, _, ok := strings.Cut(entry, "=")
		if ok && envKeyMatches(entryKey, key, goos) {
			count++
		}
	}
	return count
}
