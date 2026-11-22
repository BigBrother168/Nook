package netguard

import (
	"os"
	"strings"
	"testing"
)

// Windows tells this test's own command line and file.
func TestCommandLine(t *testing.T) {
	cmd := CommandLine(uint32(os.Getpid()))
	if !strings.Contains(cmd, "netguard.test") {
		t.Fatalf("command line %q", cmd)
	}
	if p := ImagePath(uint32(os.Getpid())); !strings.HasSuffix(strings.ToLower(p), ".exe") {
		t.Fatalf("image path %q", p)
	}
}
