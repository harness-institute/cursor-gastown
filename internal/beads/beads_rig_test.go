package beads

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFormatRigDescription(t *testing.T) {
	tests := []struct {
		name    string
		rigName string
		fields  *RigFields
		want    []string
	}{
		{
			name:    "nil fields",
			rigName: "gastown",
			fields:  nil,
			want:    nil, // empty string
		},
		{
			name:    "all fields",
			rigName: "gastown",
			fields: &RigFields{
				Repo:   "git@github.com:user/gastown.git",
				Prefix: "gt",
				State:  RigStateActive,
			},
			want: []string{
				"Rig identity bead for gastown.",
				"repo: git@github.com:user/gastown.git",
				"prefix: gt",
				"state: active",
			},
		},
		{
			name:    "partial fields",
			rigName: "beads",
			fields: &RigFields{
				Prefix: "bd",
			},
			want: []string{
				"Rig identity bead for beads.",
				"prefix: bd",
			},
		},
		{
			name:    "empty fields no repo/prefix/state lines",
			rigName: "empty",
			fields:  &RigFields{},
			want: []string{
				"Rig identity bead for empty.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRigDescription(tt.rigName, tt.fields)
			if tt.want == nil {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
				return
			}
			for _, line := range tt.want {
				if !strings.Contains(got, line) {
					t.Errorf("missing line %q in output:\n%s", line, got)
				}
			}
		})
	}
}

func TestParseRigFields(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want *RigFields
	}{
		{
			name: "empty description",
			desc: "",
			want: &RigFields{},
		},
		{
			name: "full rig description",
			desc: `Rig identity bead for gastown.

repo: git@github.com:user/gastown.git
prefix: gt
state: active`,
			want: &RigFields{
				Repo:   "git@github.com:user/gastown.git",
				Prefix: "gt",
				State:  RigStateActive,
			},
		},
		{
			name: "null values become empty",
			desc: "repo: null\nprefix: bd\nstate: null",
			want: &RigFields{
				Repo:   "",
				Prefix: "bd",
				State:  "",
			},
		},
		{
			name: "only prefix",
			desc: "prefix: bd",
			want: &RigFields{
				Prefix: "bd",
			},
		},
		{
			name: "state maintenance",
			desc: "state: maintenance\nprefix: gt",
			want: &RigFields{
				State:  RigStateMaintenance,
				Prefix: "gt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRigFields(tt.desc)
			if got.Repo != tt.want.Repo {
				t.Errorf("Repo = %q, want %q", got.Repo, tt.want.Repo)
			}
			if got.Prefix != tt.want.Prefix {
				t.Errorf("Prefix = %q, want %q", got.Prefix, tt.want.Prefix)
			}
			if got.State != tt.want.State {
				t.Errorf("State = %q, want %q", got.State, tt.want.State)
			}
		})
	}
}

func TestRigFieldsRoundTrip(t *testing.T) {
	original := &RigFields{
		Repo:   "git@github.com:user/gastown.git",
		Prefix: "gt",
		State:  RigStateActive,
	}

	formatted := FormatRigDescription("gastown", original)
	parsed := ParseRigFields(formatted)

	if parsed.Repo != original.Repo {
		t.Errorf("Repo: got %q, want %q", parsed.Repo, original.Repo)
	}
	if parsed.Prefix != original.Prefix {
		t.Errorf("Prefix: got %q, want %q", parsed.Prefix, original.Prefix)
	}
	if parsed.State != original.State {
		t.Errorf("State: got %q, want %q", parsed.State, original.State)
	}
}

func TestRigBeadID(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"gastown", "gt-rig-gastown"},
		{"beads", "gt-rig-beads"},
		{"my-rig", "gt-rig-my-rig"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RigBeadID(tt.name); got != tt.want {
				t.Errorf("RigBeadID(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestRigBeadIDWithPrefix(t *testing.T) {
	tests := []struct {
		prefix string
		name   string
		want   string
	}{
		{"gt", "gastown", "gt-rig-gastown"},
		{"bd", "beads", "bd-rig-beads"},
		{"hq", "town", "hq-rig-town"},
	}

	for _, tt := range tests {
		t.Run(tt.prefix+"-"+tt.name, func(t *testing.T) {
			if got := RigBeadIDWithPrefix(tt.prefix, tt.name); got != tt.want {
				t.Errorf("RigBeadIDWithPrefix(%q, %q) = %q, want %q", tt.prefix, tt.name, got, tt.want)
			}
		})
	}
}

func TestValidRigState(t *testing.T) {
	tests := []struct {
		state RigState
		want  bool
	}{
		{RigStateActive, true},
		{RigStateArchived, true},
		{RigStateMaintenance, true},
		{"", false},
		{"invalid", false},
		{"ACTIVE", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := ValidRigState(tt.state); got != tt.want {
				t.Errorf("ValidRigState(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestCreateRigBeadUsesDurableRigType(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses POSIX shell fake bd")
	}

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "bd.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$BD_LOG"
case "$1:$2:$3" in
  config:get:types.custom)
    printf '%s\n' 'agent,role,rig,convoy,slot,queue,event,message,molecule,gate,merge-request'
    exit 0
    ;;
  config:get:types.infra)
    printf '%s\n' 'agent,role,message'
    exit 0
    ;;
esac
if [ "$1" = "config" ]; then
  exit 0
fi
if [ "$1" = "create" ]; then
  printf '%s\n' '{"id":"gt-rig-gastown","title":"gastown","issue_type":"rig","status":"open","priority":2,"created_at":"2026-07-07T00:00:00Z","updated_at":"2026-07-07T00:00:00Z","labels":["gt:rig"]}'
  exit 0
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0755); err != nil {
		t.Fatalf("write fake bd: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BD_LOG", logPath)

	workDir := t.TempDir()
	beadsDir := filepath.Join(workDir, ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDir, "dolt"), 0755); err != nil {
		t.Fatal(err)
	}
	ResetEnsuredDirs()

	issue, err := NewIsolated(workDir).CreateRigBead("gastown", &RigFields{Prefix: "gt", State: RigStateActive})
	if err != nil {
		t.Fatalf("CreateRigBead: %v", err)
	}
	if issue.Type != "rig" || issue.Ephemeral {
		t.Fatalf("created issue type/ephemeral = %q/%v, want rig/false", issue.Type, issue.Ephemeral)
	}

	log := string(mustReadFile(t, logPath))
	for _, want := range []string{"config set types.custom", "config set types.infra agent,role,message", "create --json", "--labels=gt:rig", "--type=rig"} {
		if !strings.Contains(log, want) {
			t.Fatalf("bd log missing %q in:\n%s", want, log)
		}
	}
	if strings.Contains(log, "--type=task") {
		t.Fatalf("rig bead creation used task type:\n%s", log)
	}
}

func TestCreateRigBeadFailsClosedWhenTypeConfigFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses POSIX shell fake bd")
	}

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "bd.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$BD_LOG"
if [ "$1" = "config" ]; then
  case "$2:$3" in
    get:types.custom)
      printf '%s\n' 'agent,role,rig,convoy,slot,queue,event,message,molecule,gate,merge-request'
      exit 0
      ;;
    get:types.infra)
      printf '%s\n' 'agent,rig,role,message'
      exit 0
      ;;
  esac
  exit 0
fi
if [ "$1" = "create" ]; then
  printf '%s\n' '{"id":"should-not-create"}'
  exit 0
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0755); err != nil {
		t.Fatalf("write fake bd: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BD_LOG", logPath)

	workDir := t.TempDir()
	beadsDir := filepath.Join(workDir, ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDir, "dolt"), 0755); err != nil {
		t.Fatal(err)
	}
	ResetEnsuredDirs()

	_, err := NewIsolated(workDir).CreateRigBead("gastown", &RigFields{Prefix: "gt"})
	if err == nil {
		t.Fatal("expected CreateRigBead to fail when types.infra verification fails")
	}
	if !strings.Contains(err.Error(), "types.infra not persisted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if log := string(mustReadFile(t, logPath)); strings.Contains(log, "create --json") {
		t.Fatalf("CreateRigBead called bd create after config failure:\n%s", log)
	}
}
