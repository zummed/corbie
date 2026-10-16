package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/zummed/corbie/client/cli"
)

// The first word of several commands, with nothing after it or asked for
// help, prints the words that follow it, each once and in the table's
// order: a command's word with its summary, and a word that begins several
// commands with the words that follow it.
func TestAGroupOfCommandsPrintsItsCommands(t *testing.T) {
	row := regexp.MustCompile(`(?m)^  (\S+)   `)
	for _, group := range []string{"run", "controller", "runner", "client", "controller secret"} {
		for _, asked := range [][]string{nil, {"--help"}} {
			args := append(strings.Fields(group), asked...)
			var out, said bytes.Buffer
			status := cli.Run(context.Background(), args, strings.NewReader(""), &out, &said)
			if status != 0 || said.Len() != 0 {
				t.Errorf("corbie %s ended %d and said %q", strings.Join(args, " "), status, said.String())
			}
			text := out.String()
			flat := strings.Join(strings.Fields(text), " ")
			var words, printed []string
			for _, c := range cli.Commands {
				rest, in := strings.CutPrefix(c.Name, group+" ")
				if !in {
					continue
				}
				word, after, deeper := strings.Cut(rest, " ")
				if !slices.Contains(words, word) {
					words = append(words, word)
				}
				next, _, _ := strings.Cut(after, " ")
				listed := strings.Contains(flat, " "+word+" "+c.Summary)
				if deeper {
					listed = strings.Contains(flat, " "+word+" ") && strings.Contains(flat, next)
				}
				if !listed {
					t.Errorf("corbie %s does not list %s as it should:\n%s", strings.Join(args, " "), c.Name, out.String())
				}
			}
			for _, m := range row.FindAllStringSubmatch(text, -1) {
				printed = append(printed, m[1])
			}
			if !slices.Equal(words, printed) {
				t.Errorf("corbie %s lists %v, and the words that follow it are %v:\n%s", strings.Join(args, " "), printed, words, out.String())
			}
		}
	}
	// A word that begins no command is a mistake all the same.
	var out, said bytes.Buffer
	if status := cli.Run(context.Background(), []string{"frobnicate", "--help"}, strings.NewReader(""), &out, &said); status != 2 || !strings.Contains(said.String(), "is not a command") {
		t.Errorf("corbie frobnicate --help ended %d and said %q", status, said.String())
	}
}

// A second machine set up under a name that a controller already has on the
// store is refused: nothing is made, and the first controller's trust is as
// it was.
func TestInitRefusesANameAnotherControllerHasOnTheStore(t *testing.T) {
	first := machine{t, filepath.Join(t.TempDir(), "first")}
	second := machine{t, filepath.Join(t.TempDir(), "second")}
	storeDir := filepath.Join(t.TempDir(), "store")
	first.ok("", "init", "--store", storeDir, "--name", "box", "--ceiling", "network")
	parties, held := first.ok("", "controller", "parties"), stored(t, storeDir)

	status, out, said := second.run("", "init", "--store", storeDir, "--name", "box", "--ceiling", "all")
	if status != 1 || out != "" || !strings.Contains(said, `the name "box-controller" is taken on that store`) {
		t.Fatalf("init under a name taken on the store ended %d: %s%s", status, out, said)
	}
	if after := first.ok("", "controller", "parties"); after != parties || !slices.Equal(stored(t, storeDir), held) {
		t.Fatalf("after the refused init the first controller trusts:\n%s", after)
	}
	if made, _ := os.ReadDir(second.config); len(made) != 0 {
		t.Fatalf("the refused init made %v in its config", made)
	}
	// Another name is free there, and the first machine sets itself up again.
	second.ok("", "init", "--store", storeDir, "--name", "other")
	first.ok("", "init", "--store", storeDir, "--name", "box", "--ceiling", "all")
	// A name stays tied to its key here as at an enrolment: a runner made
	// anew under the old name is not trusted in the old one's place.
	if err := os.Remove(filepath.Join(first.config, "runner", "identity")); err != nil {
		t.Fatal(err)
	}
	parties = first.ok("", "controller", "parties")
	if status, _, said := first.run("", "init", "--store", storeDir, "--name", "box"); status != 1 || !strings.Contains(said, "already used with another key") {
		t.Fatalf("init with a new key under a runner's name ended %d: %s", status, said)
	}
	if after := first.ok("", "controller", "parties"); after != parties {
		t.Fatalf("after the refused init the controller trusts:\n%s", after)
	}
}

// The controller's cancel of an ID that is no run says so, fails, and puts
// nothing in the inbox.
func TestControllerCancelOfNoRunSaysSo(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	before := stored(t, storeDir)
	status, out, said := m.run("", "controller", "cancel", "box-client/20200101T000000.000Z-0000000000")
	if status != 1 || out != "" || !strings.Contains(said, "there is no such run") {
		t.Fatalf("the cancel of no run ended %d: %s%s", status, out, said)
	}
	if after := stored(t, storeDir); !slices.Equal(before, after) {
		t.Fatalf("the cancel of no run left something in the store: %v", after)
	}
}

// A directory store that is not there is said to be not there; a directory that
// is there and holds no store is said to be no store.
func TestDirectoryStoreThatIsNotThereIsSaidToBe(t *testing.T) {
	m := machine{t, filepath.Join(t.TempDir(), "config")}
	storeDir := filepath.Join(t.TempDir(), "store")
	m.ok("", "init", "--store", storeDir, "--name", "box")
	if err := os.RemoveAll(storeDir); err != nil {
		t.Fatal(err)
	}
	if status, _, said := m.run("", "run", "ls"); status != 1 || !strings.Contains(said, "the directory "+strconv.Quote(storeDir)+" is not there") {
		t.Fatalf("with its store's directory gone, ls ended %d: %s", status, said)
	}
	if err := os.Mkdir(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if status, _, said := m.run("", "run", "ls"); status != 1 || !strings.Contains(said, "this is not a Corbie store") {
		t.Fatalf("with an empty directory for its store, ls ended %d: %s", status, said)
	}
}
