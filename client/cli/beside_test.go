package cli

import (
	"os"
	"testing"
)

// corbie start runs a command that runs beside the roles from this
// binary, with its name as the first word, and looks for no program.
func TestStartRunsACommandBesideTheRolesFromThisBinary(t *testing.T) {
	kept := Commands
	defer func() { Commands = kept }()
	Commands = append(Commands[:len(Commands):len(Commands)], Command{Name: "ui", Beside: true})
	t.Setenv("PATH", t.TempDir())
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path, args, err := programOf("ui")
	if err != nil || path != self || len(args) != 1 || args[0] != "ui" {
		t.Fatalf("ui is run as %q %q: %v", path, args, err)
	}
	if !name("ui") {
		t.Fatal("ui is no name on corbie start's line")
	}
	// A command that does not run beside the roles is no name.
	if name("version") || name("frob") {
		t.Fatal("a command, or a word that is nothing, is a name on corbie start's line")
	}
}
