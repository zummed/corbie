package cli

import (
	"strings"
	"testing"
)

// A release build is given its version; any other build has one too.
func TestVersionIsGivenOrRead(t *testing.T) {
	was := Version
	defer func() { Version = was }()
	Version = "1.2.3"
	if v := version(); v != "1.2.3" {
		t.Errorf("a given version came back as %q", v)
	}
	Version = ""
	if v := version(); v == "" || strings.HasPrefix(v, "v") {
		t.Errorf("a build with no version given prints %q", v)
	}
}
