//go:build !windows

package permission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func set(layers ...Layer) *Set { return &Set{Layers: layers, Scratch: "/scratch/job"} }

func TestScopes(t *testing.T) {
	for _, c := range []struct {
		entry, kind, value string
		want               bool
	}{
		{"fs.read:/srv/reports/**", Read, "/srv/reports/2026/a.md", true},
		{"fs.read:/srv/reports/**", Read, "/srv/reports", true},
		{"fs.read:/srv/reports/**", Read, "/srv/other/a", false},
		{"fs.read:/srv/reports/**", Write, "/srv/reports/a", false},
		{"fs.read:/srv/*/a.md", Read, "/srv/x/a.md", true},
		{"fs.read:/srv/*/a.md", Read, "/srv/x/y/a.md", false},
		{"fs.read:/srv/*.md", Read, "/srv/notes.md", true},
		{"fs.read:/srv/**/a.md", Read, "/srv/x/y/a.md", true},
		{"fs.read:/**", Read, "/srv/x/a.md", true},
		{"fs.read:/*", Read, "/srv", true},
		{"fs.read:/*", Read, "/srv/a", false},
		{"fs.write:out/**", Write, "/scratch/job/out/a", true},
		{"fs.write:out/**", Write, "/out/a", false},
		{"net:api.github.com", Net, "api.github.com:443", true},
		{"net:api.github.com", Net, "api.github.com:22", true},
		{"net:api.github.com:443", Net, "api.github.com:22", false},
		{"net:*.github.com", Net, "api.github.com:443", true},
		{"net:*.github.com", Net, "a.b.github.com:443", false},
		{"net:*.github.com", Net, "github.com:443", false},
		{"net:10.0.0.5", Net, "10.0.0.5:5432", true},
		{"net:10.0.0.*", Net, "10.0.0.77:1", true},
		{"net:[::1]:8080", Net, "[::1]:8080", true},
		{"exec:git", Exec, "git", true},
		{"env:HOME", Env, "HOME", true},
		{"env:AWS_*", Env, "AWS_REGION", true},
		{"env:HOME", Env, "HOME2", false},
		{"network", Net, "anything.example:1", true},
		{"network", Read, "/x", false},
		{"disk", Write, "/x", true},
		{"all", Secret, "ANY@net:x.example:443", true},
		{"secret:TOKEN@net:api.github.com", Secret, "TOKEN@net:api.github.com:443", true},
		{"secret:TOKEN@net:api.github.com", Secret, "TOKEN@net:evil.example:443", false},
		{"secret:TOKEN@net:api.github.com", Secret, "OTHER@net:api.github.com:443", false},
		{"secret:TOKEN@*", Secret, "TOKEN@exec:deploy", true},
		{"secret:TOKEN@*", Secret, "TOKEN@net.unencrypted:ha.local:8123", true},
		{"secret:TOKEN@net.unencrypted:ha.local:8123", Secret, "TOKEN@net.unencrypted:ha.local:8123", true},
		{"secret:TOKEN@net.unencrypted:ha.local:8123", Secret, "TOKEN@net:ha.local:8123", true},
		{"secret:TOKEN@net.unencrypted:ha.local:8123", Secret, "TOKEN@net.unencrypted:other.local:8123", false},
		{"secret:TOKEN@net:ha.local:8123", Secret, "TOKEN@net.unencrypted:ha.local:8123", false},
		{"secret:TOKEN@net.unencrypted:ha.local", Secret, "TOKEN@exec:ha.local", false},
		{"secret:TOKEN@exec:deploy", Secret, "TOKEN@exec:deploy", true},
		{"secret:TOKEN@fs.write:/etc/app/key", Secret, "TOKEN@fs.write:/etc/app/key", true},
	} {
		if got := set().matches(c.entry, c.kind, c.value); got != c.want {
			t.Errorf("%s against %s:%s is %v, want %v", c.entry, c.kind, c.value, got, c.want)
		}
	}
}

func TestLayers(t *testing.T) {
	// What the deny list matches is refused whatever the allow list says;
	// what the allow list matches passes; anything else is refused.
	layer := Layer{Name: "one", Allow: []string{"fs.read:/srv/private/key", "disk"}, Deny: []string{"fs.read:/srv/private/**"}}
	s := set(layer)
	if r := s.Check(Read, "/srv/private/key"); r == nil || !strings.Contains(r.Fix, "take the deny entry fs.read:/srv/private/** out of one") {
		t.Fatalf("a denied read: %+v", r)
	}
	if r := s.Check(Read, "/srv/a"); r != nil {
		t.Fatalf("an allowed read: %v", r)
	}
	if r := s.Check(Net, "x.example:443"); r == nil {
		t.Fatal("a connection no entry allows passed")
	}
	// A job may do something only if every layer passes it, and a refusal
	// names every layer that refused.
	s = set(Layer{Name: "the definition", Allow: []string{"network", "fs.read:/srv/**", "exec:git"}},
		Layer{Name: "the start", Allow: []string{"network", "fs.read:/srv/reports/**"}},
		Layer{Name: "the client's ceiling", Allow: []string{"network", "disk"}, Fix: "job: enrol --allow %s"},
		Layer{Name: "the runner's ceiling", Allow: []string{"disk", "exec"}})
	if r := s.Check(Read, "/srv/reports/a"); r != nil {
		t.Fatalf("what every layer allows: %v", r)
	}
	if r := s.Check(Read, "/srv/a"); r == nil || r.Layer != "the start" || r.What != "fs.read:/srv/a" {
		t.Fatalf("refused by the start: %+v", r)
	}
	if r := s.Check(Net, "x.example:443"); r == nil || r.Layer != "the runner's ceiling" {
		t.Fatalf("refused by the runner's ceiling: %+v", r)
	}
	r := s.Check(Exec, "git")
	if r == nil || r.Layer != "the start and by the client's ceiling" || !strings.Contains(r.Fix, "job: enrol --allow exec:git") {
		t.Fatalf("refused by two layers: %+v", r)
	}
	if got := s.Used(); len(got) != 1 || got[0] != "fs.read:/srv/reports/a" {
		t.Fatalf("what was used: %v", got)
	}
}

func Test_perm_unreadable_deny_refuses_all(t *testing.T) {
	s := set(Layer{Name: "one", Allow: []string{"all"}, Deny: []string{"fs.raed:/x"}})
	if s.Check(Net, "x.example:1") == nil || s.Check(Env, "HOME") == nil {
		t.Fatal("a layer with a deny entry that cannot be read let something through")
	}
}

func Test_perm_program_name_is_not_a_path(t *testing.T) {
	if set(allow("exec:git")).Check(Exec, "/tmp/x/git") == nil {
		t.Fatal("exec:git allowed a path")
	}
	if set(allow("exec:/usr/bin/git")).Check(Exec, "git") == nil {
		t.Fatal("exec:/usr/bin/git allowed the bare name")
	}
	if set(allow("exec:*")).Check(Exec, "/tmp/x/git") == nil {
		t.Fatal("exec:* allowed a path")
	}
	if set(allow("exec:/usr/bin/git")).Check(Exec, "/usr/bin/git") != nil {
		t.Fatal("exec:/usr/bin/git did not allow its own path")
	}
}

func Test_perm_host_one_spelling_before_match(t *testing.T) {
	s := set(Layer{Name: "one", Allow: []string{"network"}, Deny: []string{"net:Meta.Internal.example", "net:169.254.169.254"}})
	for _, spelling := range []string{"meta.internal.example:80", "META.INTERNAL.EXAMPLE:80", "meta.internal.example.:80",
		"169.254.169.254:80", "[::ffff:169.254.169.254]:80"} {
		if s.Check(Net, spelling) == nil {
			t.Errorf("another spelling of a denied host slipped past: %s", spelling)
		}
	}
	if NormalHost("fe80::1%eth0") != "fe80::1" {
		t.Error("an address's zone is not dropped")
	}
}

func Test_perm_odd_host_name_refused_before_lookup(t *testing.T) {
	for _, host := range []string{"a b.example", "x.example/path", "user@x.example", "", "x.example\n"} {
		if CheckHost(host) == nil {
			t.Errorf("%q passed as a host's name", host)
		}
	}
	for _, host := range []string{"api.github.com", "db_1.internal", "10.0.0.5", "::1"} {
		if err := CheckHost(host); err != nil {
			t.Errorf("%q: %v", host, err)
		}
	}
}

func Test_perm_host_scope_not_an_address(t *testing.T) {
	for _, entry := range []string{"net:https://api.github.com", "net:api.github.com/v1", "net:user@host.example"} {
		if err := CheckWritten(entry); err == nil || !strings.Contains(err.Error(), "host alone") {
			t.Errorf("%s: %v", entry, err)
		}
	}
	for _, entry := range []string{"net:api.github.com", "net:db.internal:5432", "net:*.example.com", "all", "disk",
		"secret:T@net:x.example", "secret:T@net.unencrypted:x.example:8123", "secret:T@*", "fs.read:/srv/**", "exec:git", "env:HOME"} {
		if err := CheckWritten(entry); err != nil {
			t.Errorf("%s: %v", entry, err)
		}
	}
	for _, entry := range []string{"", "net", "net:", "file:/x", "secret:T", "secret:T@", "secret:T@env:HOME", "secret:T@net.unencrypted:http://x.example", "net.unencrypted:x.example", "secret:${x}@*"} {
		if CheckWritten(entry) == nil {
			t.Errorf("%q passed", entry)
		}
	}
}

func Test_perm_scope_leading_part_resolved(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	os.MkdirAll(filepath.Join(dir, "real", "sub"), 0o755)
	os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link"))
	// A scope written through a link matches the real location of what is
	// under it, since a value is checked where it really points.
	s := set(allow("fs.read:" + filepath.Join(dir, "link") + "/**"))
	if r := s.Check(Read, filepath.Join(dir, "real", "sub", "a")); r != nil {
		t.Fatalf("a scope through a link: %v", r)
	}
	if got, err := RealPath(filepath.Join(dir, "link", "sub", "not-there-yet")); err != nil || got != filepath.Join(dir, "real", "sub", "not-there-yet") {
		t.Fatalf("a path through a link resolves to %s (%v)", got, err)
	}
}

func Test_perm_resolved_name_is_not_a_pattern(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	os.MkdirAll(filepath.Join(dir, "x*"), 0o755)
	os.MkdirAll(filepath.Join(dir, "xyz"), 0o755)
	os.Symlink(filepath.Join(dir, "x*"), filepath.Join(dir, "link"))
	// The directory the link points to is called x*: its name is taken as it
	// is, and does not widen the scope to xyz.
	s := set(allow("fs.read:" + filepath.Join(dir, "link") + "/**"))
	if s.Check(Read, filepath.Join(dir, "x*", "a")) != nil {
		t.Fatal("the real directory is not allowed")
	}
	if s.Check(Read, filepath.Join(dir, "xyz", "a")) == nil {
		t.Fatal("a * in a directory's real name widened the scope")
	}
}

func Test_perm_address_scope_matches_only_an_address(t *testing.T) {
	s := set(allow("net:10.*.*.*", "secret:K@net:10.*.*.*"))
	if r := s.Check(Net, "10.0.3.7:443"); r != nil {
		t.Fatalf("an address pattern refused an address: %v", r)
	}
	// A name with the parts of the pattern is a name all the same, and
	// where it resolves to is anyone's choice.
	for _, value := range []string{"10.x.example.com:443", "10.0.3.example:443"} {
		if s.Check(Net, value) == nil {
			t.Fatalf("an address pattern passed the name %s", value)
		}
		if s.Check(Secret, "K@net:"+value) == nil {
			t.Fatalf("a secret bound to an address pattern was sent to the name %s", value)
		}
	}
}

func Test_perm_dangling_link_followed(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	os.MkdirAll(filepath.Join(dir, "out"), 0o755)
	os.Symlink(filepath.Join(dir, "elsewhere", "target"), filepath.Join(dir, "out", "report"))
	// A link to a place that does not exist yet points there all the
	// same, and writing the link makes the file there.
	if got, err := RealPath(filepath.Join(dir, "out", "report")); err != nil || got != filepath.Join(dir, "elsewhere", "target") {
		t.Fatalf("a dangling link resolves to %s (%v)", got, err)
	}
	os.Symlink("../elsewhere", filepath.Join(dir, "out", "rel"))
	if got, err := RealPath(filepath.Join(dir, "out", "rel", "sub", "a")); err != nil || got != filepath.Join(dir, "elsewhere", "sub", "a") {
		t.Fatalf("a relative dangling link resolves to %s (%v)", got, err)
	}
}

func Test_perm_link_ring_refused(t *testing.T) {
	dir := t.TempDir()
	os.Symlink(filepath.Join(dir, "b"), filepath.Join(dir, "a"))
	os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, "b"))
	if _, err := RealPath(filepath.Join(dir, "a", "file")); err == nil {
		t.Fatal("a path through a ring of links was given a real location")
	}
}

func Test_perm_scratch_needs_an_entry(t *testing.T) {
	if set(allow("fs.read:/srv/**")).Check(Write, "/scratch/job/notes.txt") == nil {
		t.Fatal("the scratch directory was written with no entry for it")
	}
	if r := set(allow("fs.write:**")).Check(Write, "/scratch/job/notes.txt"); r != nil {
		t.Fatalf("with an entry for the scratch directory: %v", r)
	}
	if set(allow("fs.write:**")).Check(Write, "/etc/passwd") == nil {
		t.Fatal("an entry for the scratch directory reached outside it")
	}
}

func Test_perm_refusal_line_pastes(t *testing.T) {
	s := set(allow())
	for kind, value := range map[string]string{Read: "/srv/a b/c", Net: "api.github.com:443", Exec: "git", Env: "HOME"} {
		r := s.Check(kind, value)
		if r == nil || CheckWritten(r.What) != nil {
			t.Fatalf("the line %v cannot be written in a list", r)
		}
		if set(allow(r.What)).Check(kind, value) != nil {
			t.Fatalf("the line %s, pasted, does not allow what it was refused for", r.What)
		}
	}
}

func Test_perm_refused_call_notes_nothing(t *testing.T) {
	s := set(allow("net:api.github.com"))
	mark := s.Mark()
	s.Check(Net, "api.github.com:443")
	if s.Check(Secret, "TOKEN@net:api.github.com:443") == nil {
		t.Fatal("a secret with no entry passed")
	}
	s.Reset(mark)
	if got := s.Used(); len(got) != 0 {
		t.Fatalf("a refused call left %v in the list of what was used", got)
	}
}

func TestDeclares(t *testing.T) {
	listed := []string{"fs.write:/srv/out/**"}
	for entry, want := range map[string]bool{"fs.write:/srv/out/a.md": true, "fs.write:/srv/**": true, "disk": true, "all": true,
		"fs.read:/srv/**": false, "network": false} {
		if got := Declares([]string{entry}, listed); got != want {
			t.Errorf("%s declares %v: %v, want %v", entry, listed, got, want)
		}
	}
	if !Declares([]string{"exec:git"}, []string{"exec"}) || Declares([]string{"net:x"}, []string{"exec"}) {
		t.Error("a listed group stands for every scope of its kinds")
	}
}

func TestAllowsSecret(t *testing.T) {
	layers := []Layer{allow("secret:TOKEN@net:x.example"), allow("all")}
	if !AllowsSecret(layers, "TOKEN") || AllowsSecret(layers, "OTHER") {
		t.Fatal("a secret is sent only where every layer has an entry for it")
	}
}

func Test_perm_used_entries_paste(t *testing.T) {
	s := set(allow("all"))
	s.Check(Read, "/srv/a")
	s.Check(Net, "api.github.com:443")
	s.Check(Secret, "TOKEN@net:api.github.com:443")
	for _, used := range s.Used() {
		if err := CheckWritten(used); err != nil {
			t.Errorf("%s, noted as used, cannot be pasted into a list: %v", used, err)
		}
	}
}

// The scratch directory has another name for every job. What a job used
// there, and what it was refused there, is written as a place in it, so
// the line allows the same thing in the next job.
func TestEntryForAScratchPathWorksInTheNextJob(t *testing.T) {
	tried := []struct{ kind, value, entry string }{
		{Write, "out/note.txt", "fs.write:out/note.txt"},
		{Read, "", "fs.read:."},
		{Exec, "tool", "exec:./tool"},
		{Exec, "bin/tool", "exec:bin/tool"},
		{Secret, "TOKEN@fs.write:key", "secret:TOKEN@fs.write:key"},
		{Secret, "TOKEN@exec:tool", "secret:TOKEN@exec:./tool"},
	}
	// in gives a value as it is checked in a job: its path where it really
	// is, in that job's scratch directory.
	in := func(scratch, kind, value string) string {
		if kind != Secret {
			return strings.TrimSuffix(scratch+"/"+value, "/")
		}
		name, to, _ := strings.Cut(value, "@")
		toKind, path, _ := strings.Cut(to, ":")
		return name + "@" + toKind + ":" + scratch + "/" + path
	}
	first, refusing := set(allow("all")), set(allow())
	var used []string
	for _, c := range tried {
		if r := first.Check(c.kind, in("/scratch/job", c.kind, c.value)); r != nil {
			t.Fatal(r)
		}
		used = append(used, c.entry)
		r := refusing.Check(c.kind, in("/scratch/job", c.kind, c.value))
		if r == nil || r.What != c.entry || !strings.Contains(r.Fix, "add the line "+c.entry+" to") {
			t.Errorf("%s in the scratch directory was refused as %v", c.value, r)
		}
	}
	if got := first.Used(); strings.Join(got, " ") != strings.Join(used, " ") {
		t.Fatalf("what was used in the scratch directory is noted as %v", got)
	}
	next := &Set{Layers: []Layer{allow(first.Used()...)}, Scratch: "/scratch/next"}
	for _, c := range tried {
		if err := CheckWritten(c.entry); err != nil {
			t.Errorf("%s cannot be pasted into a list: %v", c.entry, err)
		}
		if r := next.Check(c.kind, in("/scratch/next", c.kind, c.value)); r != nil {
			t.Errorf("the entries of one job, pasted, do not allow the same in the next: %v", r)
		}
	}
	// A path outside the scratch directory is written whole.
	if first.Check(Read, "/scratch/jobs/a"); !strings.Contains(strings.Join(first.Used(), " "), "fs.read:/scratch/jobs/a") {
		t.Fatalf("a path beside the scratch directory is noted as %v", first.Used())
	}
}

func Test_perm_all_only_in_a_list(t *testing.T) {
	// A layer with nothing in its lists allows nothing: there is no flag
	// or setting beside the lists.
	if (&Set{Layers: []Layer{{Name: "empty"}}}).Check(Env, "HOME") == nil {
		t.Fatal("an empty layer let something through")
	}
	if (&Set{Layers: []Layer{{Name: "all", Allow: []string{"all"}}}}).Check(Env, "HOME") != nil {
		t.Fatal("all in a list did not allow")
	}
}
