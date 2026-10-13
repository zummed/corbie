package plugin_test

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// An extension kept in a repository of its own names Corbie as it names
// any module, and Go fetches Corbie by that name. Here the fetching is from
// a directory laid out as a module proxy serves one, holding this source as a
// version of Corbie: no line in the extension says where Corbie is.
func Test_plug_extension_builds_from_its_own_repository(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	named := regexp.MustCompile(`(?m)^module (\S+)$`).FindSubmatch(mod)
	if named == nil {
		t.Fatal("go.mod names no module")
	}
	corbie, version := string(named[1]), "v0.0.1"
	if strings.Contains(string(mod), "\nreplace ") {
		t.Fatal("Corbie's go.mod replaces a module, which no module that depends on Corbie would see")
	}

	// Corbie as a proxy would serve it.
	work := t.TempDir()
	served := filepath.Join(work, "proxy", filepath.FromSlash(corbie), "@v")
	if err := os.MkdirAll(served, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(served, "list"), []byte(version+"\n"), 0o644)
	os.WriteFile(filepath.Join(served, version+".info"), []byte(`{"Version":"`+version+`","Time":"2026-01-01T00:00:00Z"}`), 0o644)
	os.WriteFile(filepath.Join(served, version+".mod"), mod, 0o644)
	packed, err := os.Create(filepath.Join(served, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	source := zip.NewWriter(packed)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		rel, _ := filepath.Rel(root, path)
		switch {
		case err != nil:
			return err
		case entry.IsDir() && (entry.Name() == ".git" || rel == "bin"):
			return filepath.SkipDir
		case !entry.Type().IsRegular():
			return nil
		}
		into, err := source.Create(corbie + "@" + version + "/" + filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		from, err := os.Open(path)
		if err != nil {
			return err
		}
		defer from.Close()
		_, err = io.Copy(into, from)
		return err
	})
	if err == nil {
		err = source.Close()
	}
	if cerr := packed.Close(); err != nil || cerr != nil {
		t.Fatal(err, cerr)
	}

	// The extension: a module of its own, with one plugin.
	ext := filepath.Join(work, "ext")
	os.MkdirAll(ext, 0o755)
	os.WriteFile(filepath.Join(ext, "go.mod"), []byte("module ext.example/pg\n\ngo 1.27\n\nrequire "+corbie+" "+version+"\n"), 0o644)
	os.WriteFile(filepath.Join(ext, "pg.go"), []byte(`// Package pg is an extension with one plugin.
package pg

import "`+corbie+`/plugin"

func init() {
	plugin.Register(&plugin.Module{Name: "pg", Page: "pg: a stand-in.", Calls: map[string]*plugin.Call{
		"query": {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) { return first, nil }},
	}})
}
`), 0o644)

	// Everything else Corbie is built from is fetched from what this
	// machine already holds, so the build needs no network.
	cache, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	held := filepath.Join(strings.TrimSpace(string(cache)), "cache", "download")
	build := exec.Command("go", "build", "-trimpath", "./...")
	build.Dir = ext
	build.Env = append(os.Environ(),
		"GOPROXY=file://"+filepath.ToSlash(filepath.Join(work, "proxy"))+",file://"+filepath.ToSlash(held),
		"GOMODCACHE="+filepath.Join(work, "mod"), "GOFLAGS=-mod=mod -modcacherw", "GOSUMDB=off", "GONOSUMDB=", "GONOSUMCHECK=",
		"GOPRIVATE=", "GONOPROXY=", "GOWORK=off", "GOTOOLCHAIN=local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the extension does not build with Corbie named as an ordinary dependency: %v\n%s", err, out)
	}
	// It was Corbie as fetched that the extension was built against, and
	// its own go.mod still says nothing of where Corbie is.
	if _, err := os.Stat(filepath.Join(work, "mod", filepath.FromSlash(corbie)+"@"+version, "plugin", "plugin.go")); err != nil {
		t.Fatalf("Corbie was not fetched by its name: %v", err)
	}
	if after, _ := os.ReadFile(filepath.Join(ext, "go.mod")); strings.Contains(string(after), "replace") || !strings.Contains(string(after), corbie+" "+version) {
		t.Fatalf("the extension's go.mod is now:\n%s", after)
	}
}
