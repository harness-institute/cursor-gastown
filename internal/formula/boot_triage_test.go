package formula

import (
	"strings"
	"testing"
)

func TestBootTriageFormulaUsesNudgeForWake(t *testing.T) {
	content, err := formulasFS.ReadFile("formulas/mol-boot-triage.formula.toml")
	if err != nil {
		t.Fatalf("reading boot triage formula: %v", err)
	}

	formula := string(content)
	for _, want := range []string{
		`gt nudge --mode=immediate deacon "Boot wake: please check your inbox and pending work"`,
		"Raw tmux send-keys is blocked for Boot",
	} {
		if !strings.Contains(formula, want) {
			t.Fatalf("boot triage formula missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"tmux send-keys -t hq-deacon",
		"sleep 1",
		"escape + message",
	} {
		if strings.Contains(formula, forbidden) {
			t.Fatalf("boot triage formula still contains forbidden raw tmux wake guidance %q", forbidden)
		}
	}
}
