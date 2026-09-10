package git_test

import (
	"github.com/harness-institute/cursor-gastown/internal/beads"
	"github.com/harness-institute/cursor-gastown/internal/git"
)

// Compile-time assertion: Git must satisfy BranchChecker.
var _ beads.BranchChecker = (*git.Git)(nil)
