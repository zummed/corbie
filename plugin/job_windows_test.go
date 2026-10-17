package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

// A path with a leading / and no drive is on whichever drive is current,
// and names no place.
func TestWindowsPathNeedsADrive(t *testing.T) {
	j := allowed(t, []string{"all"}, nil)
	for _, path := range []string{`/srv/a`, `\srv\a`, `C:srv\a`} {
		if _, err := j.Path(path, false); err == nil {
			t.Errorf("%s was given a place", path)
		}
	}
	if _, err := j.Path(`out\a.txt`, true); err != nil {
		t.Errorf("a path in the scratch directory: %v", err)
	}
}

// A program named without its extension is found as its .exe, and a
// batch file is not found.
func TestWindowsProgramFoundAsItsExe(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tool.exe"), nil, 0o600)
	os.WriteFile(filepath.Join(dir, "script.cmd"), nil, 0o600)
	j := allowed(t, []string{"all"}, nil)
	j.SearchPath = dir
	if got := j.find("tool"); got != filepath.Join(dir, "tool.exe") {
		t.Errorf("tool was found as %q", got)
	}
	if got := j.find("tool.exe"); got != filepath.Join(dir, "tool.exe") {
		t.Errorf("tool.exe was found as %q", got)
	}
	if got := j.find("script"); got != "" {
		t.Errorf("a batch file was found: %q", got)
	}
}
