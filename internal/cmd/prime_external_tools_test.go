package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func setupPrimeExternalToolTest(t *testing.T, bdScript, gtScript string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script subprocess test")
	}
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "calls.log")

	oldTimeout := primeExternalToolTimeout
	oldWaitDelay := primeExternalToolWaitDelay
	primeExternalToolTimeout = 100 * time.Millisecond
	primeExternalToolWaitDelay = 10 * time.Millisecond
	t.Cleanup(func() {
		primeExternalToolTimeout = oldTimeout
		primeExternalToolWaitDelay = oldWaitDelay
	})

	binDir := filepath.Join(tmpDir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	writePrimeToolScript(t, filepath.Join(binDir, "bd"), bdScript)
	writePrimeToolScript(t, filepath.Join(binDir, "gt"), gtScript)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PRIME_TOOL_CALL_LOG", logPath)
	t.Setenv("TMUX", "")
	primeDryRun = false

	return t.TempDir()
}

func writePrimeToolScript(t *testing.T, path, body string) {
	t.Helper()
	tool := filepath.Base(path)
	script := "#!/bin/sh\n" +
		"printf '%s\\n' '" + tool + ":'\"$*\" >> \"$PRIME_TOOL_CALL_LOG\"\n" +
		body + "\n" +
		"printf '%s\\n' 'unexpected args: '\"$*\" >&2\n" +
		"exit 99\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertElapsedUnder(t *testing.T, elapsed time.Duration, max time.Duration) {
	t.Helper()
	if elapsed > max {
		t.Fatalf("elapsed = %v, want under %v", elapsed, max)
	}
}

func assertPrimeToolCalled(t *testing.T, want string) {
	t.Helper()
	logPath := os.Getenv("PRIME_TOOL_CALL_LOG")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("call log missing %q:\n%s", want, string(data))
	}
}

func TestRunPrimeExternalTools_RunsMemoryAndMail(t *testing.T) {
	workDir := setupPrimeExternalToolTest(t, `
case "$*" in
  "kv list --json") printf '%s\n' '{"memory.feedback.test":"remembered"}'; exit 0 ;;
esac
`, `
case "$*" in
  "mail check --inject") printf '%s\n' 'MAIL OUTPUT'; exit 0 ;;
esac
`)

	start := time.Now()
	output := captureStdout(t, func() { runPrimeExternalTools(RoleContext{Role: RolePolecat}, workDir) })
	assertElapsedUnder(t, time.Since(start), time.Second)
	assertPrimeToolCalled(t, "bd:kv list --json")
	assertPrimeToolCalled(t, "gt:mail check --inject")

	if !strings.Contains(output, "remembered") {
		t.Fatalf("memory injection missing: %q", output)
	}
	if !strings.Contains(output, "MAIL OUTPUT") {
		t.Fatalf("mail injection missing: %q", output)
	}
}

func TestRunPrimeExternalTools_BoundsSlowMailCheck(t *testing.T) {
	markerDir := t.TempDir()
	startedPath := filepath.Join(markerDir, "child-started")
	survivedPath := filepath.Join(markerDir, "child-survived")
	workDir := setupPrimeExternalToolTest(t, `
case "$*" in
  "kv list --json") printf '%s\n' '{"memory.feedback.test":"remembered"}'; exit 0 ;;
esac
`, `
case "$*" in
  "mail check --inject")
    (: > "$PRIME_CHILD_STARTED"; sleep 0.5; : > "$PRIME_CHILD_SURVIVED") &
    while [ ! -f "$PRIME_CHILD_STARTED" ]; do sleep 0.01; done
    wait
    exit 0
    ;;
esac
`)
	t.Setenv("PRIME_CHILD_STARTED", startedPath)
	t.Setenv("PRIME_CHILD_SURVIVED", survivedPath)

	start := time.Now()
	output := captureStdout(t, func() { runPrimeExternalTools(RoleContext{Role: RolePolecat}, workDir) })
	assertElapsedUnder(t, time.Since(start), time.Second)
	assertPrimeToolCalled(t, "bd:kv list --json")
	assertPrimeToolCalled(t, "gt:mail check --inject")

	if !strings.Contains(output, "remembered") {
		t.Fatalf("memory output missing: %q", output)
	}
	if _, err := os.Stat(startedPath); err != nil {
		t.Fatalf("child did not start before timeout: %v", err)
	}

	time.Sleep(700 * time.Millisecond)
	if _, err := os.Stat(survivedPath); err == nil {
		t.Fatalf("child process survived command timeout and wrote %s", survivedPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("check survived marker: %v", err)
	}
}

func TestRunPrimeExternalTools_SkipsMailCheckForPatrolRoles(t *testing.T) {
	for _, role := range []string{string(RoleWitness), string(RoleRefinery), string(RoleDeacon), string(RoleBoot)} {
		t.Run(role, func(t *testing.T) {
			workDir := setupPrimeExternalToolTest(t, `
case "$*" in
  "kv list --json") printf '%s\n' '{}'; exit 0 ;;
esac
`, `
case "$*" in
  "mail check --inject") printf '%s\n' 'MAIL OUTPUT'; exit 0 ;;
esac
`)

			output := captureStdout(t, func() { runPrimeExternalTools(RoleContext{Role: Role(role)}, workDir) })
			assertPrimeToolCalled(t, "bd:kv list --json")
			logData, err := os.ReadFile(os.Getenv("PRIME_TOOL_CALL_LOG"))
			if err != nil {
				t.Fatalf("read call log: %v", err)
			}
			if strings.Contains(string(logData), "gt:mail check --inject") {
				t.Fatalf("patrol role %s should not run startup mail check:\n%s", role, string(logData))
			}
			if strings.Contains(output, "MAIL OUTPUT") {
				t.Fatalf("patrol role %s injected mail output: %q", role, output)
			}
		})
	}
}

func TestCheckPendingEscalations_BoundsSlowBdList(t *testing.T) {
	workDir := setupPrimeExternalToolTest(t, `
case "$*" in
	  "list --status=open --tag=escalation --json --flat") sleep 2; exit 0 ;;
esac
`, `
`)

	start := time.Now()
	output := captureStdout(t, func() {
		checkPendingEscalations(RoleContext{Role: RoleMayor, WorkDir: workDir})
	})
	assertElapsedUnder(t, time.Since(start), time.Second)
	assertPrimeToolCalled(t, "bd:list --status=open --tag=escalation --json --flat")

	if strings.Contains(output, "PENDING ESCALATIONS") {
		t.Fatalf("timed-out escalation output should not be emitted: %q", output)
	}
}
