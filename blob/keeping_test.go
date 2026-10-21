package blob_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"

	_ "github.com/zummed/corbie/flow"
	_ "github.com/zummed/corbie/statechart"
)

var ctx = context.Background()

// noting is a store that notes whether anything in it was changed.
type noting struct {
	store.Store
	changes atomic.Int64
}

func (n *noting) Write(ctx context.Context, name string, value []byte, how store.How) (bool, error) {
	done, err := n.Store.Write(ctx, name, value, how)
	if done {
		n.changes.Add(1)
	}
	return done, err
}

func (n *noting) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	done, err := n.Store.WriteFrom(ctx, name, r)
	if done {
		n.changes.Add(1)
	}
	return done, err
}

func (n *noting) Copy(ctx context.Context, from, to string) (bool, error) {
	done, err := n.Store.Copy(ctx, from, to)
	if done {
		n.changes.Add(1)
	}
	return done, err
}

func (n *noting) Delete(ctx context.Context, name string, how store.How) (bool, error) {
	done, err := n.Store.Delete(ctx, name, how)
	if done {
		n.changes.Add(1)
	}
	return done, err
}

// kept is a controller and a client on one store, with a clock the test
// moves by hand, for what a controller keeps and what it lets go.
type kept struct {
	t      *testing.T
	st     *noting
	ctl    *identity.Identity
	ctrl   *controller.Controller
	client *client.Client
	now    time.Time
}

func keeping(t *testing.T, st store.Store) *kept {
	t.Helper()
	// Blobs are named by the real clock, so the test's own starts there.
	k := &kept{t: t, st: &noting{Store: st}, now: time.Now()}
	clock := func() time.Time { return k.now }
	var err error
	if k.ctl, err = identity.New("ctl", identity.Controller); err != nil {
		t.Fatal(err)
	}
	c1, err := identity.New("c1", identity.Client)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetUp(ctx, k.st); err != nil {
		t.Fatal(err)
	}
	k.ctrl = controller.New(k.ctl, k.st)
	k.ctrl.Now = clock
	if err := identity.WriteTrust(ctx, k.st, k.ctl, identity.Trust{Card: c1.Card, Ceiling: permission.Layer{Allow: []string{"all"}}}); err != nil {
		t.Fatal(err)
	}
	k.client = client.New(c1, k.ctl.Card, k.st)
	k.client.Now = clock
	return k
}

// later moves the clock on and lets the controller do what is then due.
func (k *kept) later(d time.Duration) {
	k.t.Helper()
	k.now = k.now.Add(d)
	for range 200 {
		before := k.st.changes.Load()
		if _, err := k.ctrl.Pass(ctx); err != nil {
			k.t.Fatal("pass:", err)
		}
		if k.st.changes.Load() == before {
			return
		}
	}
	k.t.Fatal("the controller does not come to rest")
}

// start starts a run under a name chosen beforehand.
func (k *kept) start(run, src string, files map[string]string) {
	k.t.Helper()
	draft, err := machine.Parse([]byte(src), func(path string) ([]byte, error) {
		if contents, ok := files[path]; ok {
			return []byte(contents), nil
		}
		return nil, fmt.Errorf("no such file")
	})
	if err != nil {
		k.t.Fatalf("the definition does not pass:\n%v", err)
	}
	if _, err := k.client.Start(ctx, draft, client.Start{Run: run}); err != nil {
		k.t.Fatal(err)
	}
	k.later(0)
}

func (k *kept) blobs(run string) []string {
	k.t.Helper()
	names, err := store.Names(k.st.List(ctx, store.Blobs("ctl", run)))
	if err != nil {
		k.t.Fatal(err)
	}
	return names
}

// A chart that stays where it is for as long as nobody stops it, and has
// not yet run its script or its child, each a blob its definition names.
const waiting = `statechart: waiting
keep: 1d
initial: idle
states:
  idle:
    on: { work: working, tick: idle }
  working:
    job: { script: ./work.js, files: [./lib.js] }
    on: { done: helping }
  helping:
    job: { machine: ./child.yaml }
    on: { done: stopped }
  stopped: { end: done }
`

var waitingFiles = map[string]string{
	"work.js":    `return require("./lib.js").work`,
	"lib.js":     `exports.work = "done"`,
	"child.yaml": "flow: child\nstages:\n  it: { job: { script: ./inner.js } }\n",
	"inner.js":   `return "inside"`,
}

func Test_blob_trim_follows_references_inside_blobs(t *testing.T) {
	mem, _ := store.Open("memory:")
	k := keeping(t, mem)
	run, _ := k.client.NewRun()
	// A blob in the run's record that nothing refers to.
	loose, err := blob.Write(ctx, k.st, store.Blob("ctl", run, blob.NewName()), "text/plain", blob.NewKey(), strings.NewReader("referred to by nothing"))
	if err != nil {
		t.Fatal(err)
	}
	k.start(run, waiting, waitingFiles)
	before := k.blobs(run)
	// The definition, and inside it two scripts and the child's definition,
	// and inside that the child's script.
	if len(before) != 6 {
		t.Fatalf("the record holds %d blobs at the start: %v", len(before), before)
	}
	for range 5 {
		k.later(13 * time.Hour)
		k.client.Send(ctx, run, "tick", nil, 0)
		k.later(0)
	}
	// The older history has been trimmed, and the blob nothing refers to
	// with it.
	after := k.blobs(run)
	if slices.Contains(after, loose.At) {
		t.Fatal("after two days and a half with a keep of a day, a blob nothing refers to is still there: nothing was trimmed")
	}
	if steps, _ := store.Names(k.st.List(ctx, store.Steps("ctl", run))); len(steps) > 4 {
		t.Fatalf("%d steps remain", len(steps))
	}
	// What the state refers to only through the stored definition is all
	// there, and can be read.
	if len(after) != 5 {
		t.Fatalf("of the five blobs the definition is and names, %d remain", len(after))
	}
	latest, err := instance.Current(ctx, k.st, k.ctl.Card, k.ctl, run)
	if err != nil || latest == nil {
		t.Fatalf("the run's latest step: %v", err)
	}
	named, err := blob.Named(ctx, k.st, latest.State.Definition)
	if err != nil || len(named) != 4 {
		t.Fatalf("the definition names %d blobs: %v", len(named), err)
	}
	for _, ref := range named {
		if _, err := blob.ReadAll(ctx, k.st, ref, blob.StepLimit); err != nil {
			t.Errorf("a blob the definition names: %v", err)
		}
	}
}

func Test_blob_age_not_from_file_times(t *testing.T) {
	dir := t.TempDir()
	directory, err := store.OpenDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	k := keeping(t, directory)
	// touch sets the time of the file a blob is kept in.
	touch := func(name string, at time.Time) {
		t.Helper()
		if err := os.Chtimes(filepath.Join(dir, filepath.FromSlash(name)), at, at); err != nil {
			t.Fatal(err)
		}
	}
	there := func(name string) bool {
		_, mk, err := k.st.Read(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		return mk != store.None
	}
	start := k.now

	// An upload for a run that never starts, in a file ten days old: it is
	// as old as its run's name says, which is not yet a day.
	never, _ := k.client.NewRun()
	upload, err := blob.Write(ctx, k.st, store.Blob("ctl", never, blob.NewName()), "text/plain", blob.NewKey(), strings.NewReader("never started"))
	if err != nil {
		t.Fatal(err)
	}
	touch(upload.At, start.AddDate(0, 0, -10))

	// A run that goes on, with a keep of a day.
	run, _ := k.client.NewRun()
	k.start(run, waiting, waitingFiles)

	k.later(20 * time.Hour)
	if !there(upload.At) {
		t.Fatal("an upload not a day old went, because its file is older")
	}
	// A blob nothing refers to is made in the run's record now, as a job
	// might make one, and its file is ten days old too.
	loose, err := blob.Write(ctx, k.st, store.Blob("ctl", run, store.Stamp(k.now)+"-0123456789abcdef0123"), "text/plain", blob.NewKey(), strings.NewReader("referred to by nothing"))
	if err != nil {
		t.Fatal(err)
	}
	touch(loose.At, start.AddDate(0, 0, -10))
	// The upload's file is new from here on, as after a copy or a backup
	// put back: what is in it is as old as it was.
	touch(upload.At, k.now.AddDate(0, 0, 10))

	// At a day the upload goes, and the run's older history is trimmed:
	// the blob is five hours old, and stays.
	k.later(5 * time.Hour)
	if there(upload.At) {
		t.Error("an upload for a run not started within a day is still there, because its file is new")
	}
	if !there(loose.At) {
		t.Fatal("a blob five hours old was trimmed, because its file is older")
	}
	// A day on it is older than the run's keep, whatever its file says.
	touch(loose.At, k.now.AddDate(0, 0, 10))
	k.later(25 * time.Hour)
	if there(loose.At) {
		t.Error("a blob older than the run's keep, which nothing refers to, is still there, because its file is new")
	}
}
