package permission

import "testing"

func windowsSet(layers ...Layer) *Set { return &Set{Layers: layers, Scratch: `C:\scratch\job`} }

func Test_perm_windows_path_has_a_drive(t *testing.T) {
	for _, c := range []struct {
		entry, kind, value string
		want               bool
	}{
		{"fs.read:C:/srv/reports/**", Read, "C:/srv/reports/2026/a.md", true},
		{`fs.read:C:\srv\reports\**`, Read, "C:/srv/reports/2026/a.md", true},
		{"fs.read:C:/srv/reports/**", Read, "D:/srv/reports/a.md", false},
		{"fs.read:C:/srv/*/a.md", Read, "C:/srv/x/y/a.md", false},
		{"fs.read://server/share/**", Read, "//server/share/x/a.md", true},
		{"fs.read:C:/**", Read, "C:/srv/a.md", true},
		{"fs.read:C:/*", Read, "C:/srv", true},
		{"fs.read:C:/**", Read, "D:/srv/a.md", false},
		{"fs.read://server/share/**", Read, "//server/other/a.md", false},
		// A path with no drive is on whichever drive is current: it names
		// no place, and allows nothing.
		{"fs.read:/srv/**", Read, "C:/srv/a.md", false},
		{"fs.read:/**", Read, "C:/srv/a.md", false},
		{"fs.write:out/**", Write, "C:/scratch/job/out/a", true},
		{"fs.write:out/**", Write, "C:/out/a", false},
		{"exec:C:/Tools/deploy.exe", Exec, "C:/Tools/deploy.exe", true},
		{`exec:C:\Tools\deploy.exe`, Exec, "C:/Tools/deploy.exe", true},
		{"exec:git", Exec, "C:/tmp/x/git.exe", false},
		{"exec:C:/Program Files/Git/cmd/git.exe", Exec, "git", false},
	} {
		got := windowsSet(allow(c.entry)).Check(c.kind, c.value) == nil
		if got != c.want {
			t.Errorf("%s for %s %s: passed=%v, want %v", c.entry, c.kind, c.value, got, c.want)
		}
	}
}

// A path is the same file in any case, and a program's name is the same
// program with .exe or without: a deny entry written one way refuses the
// others.
func Test_perm_windows_name_in_any_case(t *testing.T) {
	deny := Layer{Name: "the layer", Allow: []string{"all"}, Deny: []string{"fs.write:C:/x/secret/**", "exec:powershell", "exec:C:/Tools/run.exe"}}
	for kind, values := range map[string][]string{
		Write: {"C:/x/secret/a", "C:/X/SECRET/a", "c:/x/Secret/a"},
		Exec:  {"powershell", "PowerShell", "powershell.exe", "POWERSHELL.EXE", "c:/tools/RUN.exe"},
	} {
		for _, value := range values {
			if windowsSet(deny).Check(kind, value) == nil {
				t.Errorf("%s %s passed a deny entry", kind, value)
			}
		}
	}
	for _, value := range []string{"git", "Git", "git.exe", "GIT.EXE"} {
		if r := windowsSet(allow("exec:git")).Check(Exec, value); r != nil {
			t.Errorf("exec:git refused %s: %v", value, r)
		}
	}
	if windowsSet(allow("exec:git")).Check(Exec, "git.bat") == nil {
		t.Error("exec:git allowed a batch file")
	}
}

func Test_perm_windows_scratch_written_relative(t *testing.T) {
	r := windowsSet(allow()).Check(Write, "C:/scratch/job/out/a.txt")
	if r == nil || r.What != "fs.write:out/a.txt" {
		t.Fatalf("a refusal in the scratch directory says %v", r)
	}
	r = windowsSet(allow()).Check(Write, "c:/SCRATCH/job/out/a.txt")
	if r == nil || r.What != "fs.write:out/a.txt" {
		t.Fatalf("a refusal in the scratch directory, in other case, says %v", r)
	}
}

func TestProgramFiles(t *testing.T) {
	for p, want := range map[string]string{
		`C:\bin\git`:     `C:\bin\git.exe C:\bin\git.com`,
		`C:\bin\git.exe`: `C:\bin\git.exe`,
		`C:\bin\GIT.EXE`: `C:\bin\GIT.EXE`,
	} {
		got := ""
		for i, f := range ProgramFiles(p) {
			if i > 0 {
				got += " "
			}
			got += f
		}
		if got != want {
			t.Errorf("%s is looked for as %s, want %s", p, got, want)
		}
	}
}
