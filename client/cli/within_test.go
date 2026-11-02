package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// A context reached by a link holds what is in it, and a directory given by a
// link is the directory it names.
func Test_client_within_follows_a_directory(t *testing.T) {
	top := t.TempDir()
	real := filepath.Join(top, "real")
	for _, name := range []string{"real/bridges", "elsewhere"} {
		if err := os.MkdirAll(filepath.Join(top, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{"link": "real", "real/br": "bridges", "real/out": "../elsewhere"} {
		if err := os.Symlink(target, filepath.Join(top, link)); err != nil {
			t.Fatal(err)
		}
	}
	context := filepath.Join(top, "link")
	for name, want := range map[string]string{
		context:                             ".",
		filepath.Join(context, "bridges"):   "bridges",
		filepath.Join(real, "bridges"):      "bridges",
		filepath.Join(context, "br"):        "bridges",
		filepath.Join(context, "bridges/x"): "bridges/x",
		filepath.Join(top, "elsewhere"):     "",
		filepath.Join(context, "out"):       "",
	} {
		rel, inside, err := within(context, name)
		if err != nil || rel != want || inside != (want != "") {
			t.Errorf("%s gave %q, %v, %v", name, rel, inside, err)
		}
	}
}
