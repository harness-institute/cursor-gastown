package mail

import (
	"os"
	"testing"

	"github.com/harness-institute/cursor-gastown/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.TerminateDoltContainer()
	os.Exit(code)
}
