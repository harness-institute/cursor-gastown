package deacon

import (
	"os"
	"path/filepath"

	"github.com/harness-institute/cursor-gastown/internal/beads"
)

func deaconReadOnlyRoutingEnv(townRoot string) []string {
	return beads.BuildReadOnlyRoutingBDEnv(os.Environ(), townBeadsDir(townRoot))
}

func deaconMutationRoutingEnv(townRoot string) []string {
	return beads.BuildMutationRoutingBDEnv(os.Environ(), townBeadsDir(townRoot))
}

func townBeadsDir(townRoot string) string {
	if townRoot == "" {
		return ""
	}
	return filepath.Join(townRoot, ".beads")
}
