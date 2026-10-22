package store

import (
	"errors"
	"strings"
	"testing"
)

// table opens one store as the owner and as each party of a small world:
// controller ctl, runners r1 and r2, clients c1 and c2, a viewer.
type table struct {
	t                                  *testing.T
	owner, ctl, r1, r2, c1, c2, viewer Store
	nobody                             Store
}

// eachTable runs a check on the two stores that hold a party to its row:
// the memory store, and a stand-in for a bucket where each party has a key
// of its own.
func eachTable(t *testing.T, check func(*testing.T, table)) {
	for kind, open := range map[string]func(Party) Store{"memory": NewMemory().As, "bucket": newFakeS3(t).as} {
		t.Run(kind, func(t *testing.T) { check(t, newTable(t, open)) })
	}
}

func newTable(t *testing.T, open func(Party) Store) table {
	as := func(kind Kind, name string) Store {
		return open(Party{Kind: kind, Name: name, Controller: "ctl"})
	}
	w := table{t: t, owner: as(Owner, ""), ctl: as(Controller, "ctl"),
		r1: as(Runner, "r1"), r2: as(Runner, "r2"), c1: as(Client, "c1"), c2: as(Client, "c2"),
		viewer: as(Viewer, ""), nobody: as(Nobody, "")}
	for _, name := range []string{
		"version",
		"ctl/inbox/c1/m1", "ctl/inbox/r1/m1", "ctl/receipts/c1/m1", "ctl/receipts/c2/m1",
		"ctl/running/c1/i1", "ctl/running/c2/i2", "ctl/finished/c1/i0", "ctl/finished/c2/i9",
		"ctl/records/c1/i1/steps/1", "ctl/records/c1/i1/emits/j1/1", "ctl/records/c1/i1/blobs/b1",
		"ctl/records/c2/i2/steps/1", "ctl/records/c2/i2/blobs/b1",
		"ctl/wakes/t/c1/i1/w", "ctl/trust/r1", "ctl/secrets/s1", "ctl/kept/notes/tea=",
		"r1/inbox/ctl/j1", "r1/taken/j1", "r2/inbox/ctl/j2", "r2/taken/j2",
	} {
		if _, err := w.owner.Write(ctx, name, []byte("was"), Overwrite); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func (w table) allowed(s Store, how How, names ...string) {
	w.t.Helper()
	for _, name := range names {
		if done, err := s.Write(ctx, name, []byte("new"), how); err != nil || !done {
			w.t.Errorf("write %s %s: %v %v", how, name, done, err)
		}
	}
}

func (w table) refused(s Store, how How, names ...string) {
	w.t.Helper()
	for _, name := range names {
		if _, err := s.Write(ctx, name, []byte("bad"), how); !errors.Is(err, ErrRefused) {
			w.t.Errorf("write %s %s: %v, want refused", how, name, err)
		}
		if v, _, _ := w.owner.Read(ctx, name); string(v) == "bad" {
			w.t.Errorf("a refused write changed %s", name)
		}
	}
}

func (w table) noDelete(s Store, names ...string) {
	w.t.Helper()
	for _, name := range names {
		if _, err := s.Delete(ctx, name, Overwrite); !errors.Is(err, ErrRefused) {
			w.t.Errorf("delete %s: %v, want refused", name, err)
		}
		if _, mark, _ := w.owner.Read(ctx, name); mark == None {
			w.t.Errorf("a refused delete removed %s", name)
		}
	}
}

func (w table) reads(s Store, names ...string) {
	w.t.Helper()
	for _, name := range names {
		if _, mark, err := s.Read(ctx, name); err != nil || mark == None {
			w.t.Errorf("read %s: nothing (%v)", name, err)
		}
	}
}

func (w table) noRead(s Store, names ...string) {
	w.t.Helper()
	for _, name := range names {
		if v, mark, err := s.Read(ctx, name); err != nil || mark != None || v != nil {
			w.t.Errorf("read %s: given a value (%v)", name, err)
		}
	}
}

func (w table) lists(s Store, paths ...string) {
	w.t.Helper()
	for _, path := range paths {
		if names, err := Names(s.List(ctx, path)); err != nil || len(names) == 0 {
			w.t.Errorf("list %s: %v %v", path, names, err)
		}
	}
}

func (w table) noList(s Store, paths ...string) {
	w.t.Helper()
	for _, path := range paths {
		if names, err := Names(s.List(ctx, path)); !errors.Is(err, ErrRefused) || names != nil {
			w.t.Errorf("list %s: %v %v, want refused", path, names, err)
		}
	}
}

func TestControllerRow(t *testing.T) { eachTable(t, controllerRow) }

func controllerRow(t *testing.T, w table) {
	for _, name := range []string{"ctl/inbox/c1/x", "ctl/receipts/c1/x", "ctl/running/c1/x", "ctl/finished/c1/x",
		"ctl/records/c1/x/steps/1", "ctl/wakes/t/c1/x/w", "ctl/trust/x", "ctl/secrets/x", "ctl/kept/x="} {
		w.allowed(w.ctl, OnlyIfNew, name)
		w.allowed(w.ctl, Overwrite, name)
		w.reads(w.ctl, name)
		if done, err := w.ctl.Delete(ctx, name, Overwrite); err != nil || !done {
			t.Errorf("delete %s: %v", name, err)
		}
	}
	w.lists(w.ctl, "ctl", "ctl/inbox", "ctl/records/c1/i1")
	w.allowed(w.ctl, OnlyIfNew, "r1/inbox/ctl/job", "r1/inbox/ctl/cancel")
	w.refused(w.ctl, Overwrite, "r1/inbox/ctl/j1")
	w.noDelete(w.ctl, "r1/inbox/ctl/j1")
	w.refused(w.ctl, OnlyIfNew, "r1/inbox/c1/x", "r1/taken/x")
	w.noRead(w.ctl, "r1/inbox/ctl/j1", "r1/taken/j1")
	w.noList(w.ctl, "r1", "r1/inbox", "r1/taken")
}

func TestRunnerRow(t *testing.T) { eachTable(t, runnerRow) }

func runnerRow(t *testing.T, w table) {
	w.allowed(w.r1, OnlyIfNew, "ctl/inbox/r1/result")
	w.refused(w.r1, Overwrite, "ctl/inbox/r1/m1")
	w.noDelete(w.r1, "ctl/inbox/r1/m1")
	w.refused(w.r1, OnlyIfNew, "ctl/inbox/c1/x")

	// It adds emits and blobs to any record, and changes nothing there.
	w.allowed(w.r1, OnlyIfNew, "ctl/records/c1/i1/emits/j1/2", "ctl/records/c1/i1/blobs/b2",
		"ctl/records/c2/i2/emits/j9/1", "ctl/records/c2/i2/blobs/b2")
	w.refused(w.r1, Overwrite, "ctl/records/c1/i1/emits/j1/2", "ctl/records/c1/i1/blobs/b2")
	w.refused(w.r1, OnlyIfUnchanged("x"), "ctl/records/c1/i1/emits/j1/2")
	w.noDelete(w.r1, "ctl/records/c1/i1/emits/j1/2", "ctl/records/c1/i1/blobs/b2")
	w.refused(w.r1, OnlyIfNew, "ctl/records/c1/i1/steps/2")

	// Its own inbox and marks are its own; another runner's are not.
	w.allowed(w.r1, OnlyIfNew, "r1/taken/j5")
	w.allowed(w.r1, Overwrite, "r1/taken/j5")
	w.lists(w.r1, "r1/inbox", "r1/taken")
	w.reads(w.r1, "r1/inbox/ctl/j1", "r1/taken/j1", "ctl/records/c2/i2/blobs/b1")
	for _, name := range []string{"r1/inbox/ctl/j1", "r1/taken/j5"} {
		if done, err := w.r1.Delete(ctx, name, Overwrite); err != nil || !done {
			t.Errorf("delete %s: %v", name, err)
		}
	}
	w.refused(w.r1, OnlyIfNew, "r2/taken/x", "r2/inbox/ctl/x")
	w.noDelete(w.r1, "r2/inbox/ctl/j2", "r2/taken/j2")
	w.noRead(w.r1, "r2/inbox/ctl/j2", "r2/taken/j2")
	w.noList(w.r1, "r2/inbox", "r2/taken")

	// Every runner reads, writes, rewrites, deletes and lists kept values.
	for _, r := range []Store{w.r1, w.r2} {
		w.allowed(r, OnlyIfNew, "ctl/kept/notes/cake=")
		w.allowed(r, Overwrite, "ctl/kept/notes/tea=", "ctl/kept/notes/cake=")
		w.reads(r, "ctl/kept/notes/tea=")
		w.lists(r, "ctl/kept", "ctl/kept/notes")
		if done, err := r.Delete(ctx, "ctl/kept/notes/cake=", Overwrite); err != nil || !done {
			t.Errorf("delete a kept value: %v", err)
		}
	}

	// Nothing else is given: no value and no name.
	w.noRead(w.r1, "ctl/records/c1/i1/steps/1", "ctl/records/c1/i1/emits/j1/1", "ctl/running/c1/i1",
		"ctl/receipts/c1/m1", "ctl/wakes/t/c1/i1/w", "ctl/trust/r1", "ctl/secrets/s1", "ctl/inbox/r1/m1")
	w.noList(w.r1, "ctl", "ctl/inbox", "ctl/inbox/r1", "ctl/records", "ctl/records/c1/i1/blobs",
		"ctl/running", "ctl/wakes", "ctl/trust", "ctl/secrets", "")
}

func TestClientRow(t *testing.T) { eachTable(t, clientRow) }

func clientRow(t *testing.T, w table) {
	w.allowed(w.c1, OnlyIfNew, "ctl/inbox/c1/start")
	w.refused(w.c1, Overwrite, "ctl/inbox/c1/m1")
	w.noDelete(w.c1, "ctl/inbox/c1/m1")
	w.refused(w.c1, OnlyIfNew, "ctl/inbox/c2/x")

	w.allowed(w.c1, OnlyIfNew, "ctl/records/c1/new/blobs/up")
	w.refused(w.c1, Overwrite, "ctl/records/c1/new/blobs/up")
	w.noDelete(w.c1, "ctl/records/c1/new/blobs/up")
	w.refused(w.c1, OnlyIfNew, "ctl/records/c1/new/steps/1", "ctl/records/c1/new/emits/j/1",
		"ctl/records/c2/i2/blobs/x")

	w.reads(w.c1, "ctl/receipts/c1/m1", "ctl/running/c1/i1", "ctl/finished/c1/i0",
		"ctl/records/c1/i1/steps/1", "ctl/records/c1/i1/emits/j1/1", "ctl/records/c1/i1/blobs/b1")
	w.lists(w.c1, "ctl/receipts/c1", "ctl/running/c1", "ctl/finished/c1", "ctl/records/c1", "ctl/records/c1/i1/steps")
	w.noRead(w.c1, "ctl/receipts/c2/m1", "ctl/running/c2/i2", "ctl/finished/c2/i9", "ctl/records/c2/i2/steps/1",
		"ctl/inbox/c1/m1", "ctl/wakes/t/c1/i1/w", "ctl/trust/r1", "ctl/secrets/s1", "ctl/kept/notes/tea=", "r1/inbox/ctl/j1", "r1/taken/j1")
	w.noList(w.c1, "ctl/receipts/c2", "ctl/running/c2", "ctl/running", "ctl/records/c2", "ctl/inbox/c1",
		"ctl/wakes", "ctl/trust", "ctl/secrets", "ctl/kept", "r1", "r1/inbox")
	w.refused(w.c1, OnlyIfNew, "ctl/kept/notes/cake=")
	w.noDelete(w.c1, "ctl/kept/notes/tea=")
}

func TestViewerRow(t *testing.T) { eachTable(t, viewerRow) }

func viewerRow(t *testing.T, w table) {
	w.lists(w.viewer, "ctl/running", "ctl/finished", "ctl/running/c2")
	w.reads(w.viewer, "ctl/running/c1/i1", "ctl/running/c2/i2", "ctl/finished/c1/i0")
	w.noRead(w.viewer, "ctl/records/c1/i1/steps/1", "ctl/inbox/c1/m1", "ctl/receipts/c1/m1", "ctl/trust/r1", "ctl/kept/notes/tea=", "r1/taken/j1")
	w.noList(w.viewer, "ctl", "ctl/records", "ctl/inbox", "ctl/receipts", "ctl/kept", "r1", "")
	w.refused(w.viewer, OnlyIfNew, "ctl/kept/notes/cake=")
	w.refused(w.viewer, OnlyIfNew, "ctl/running/c1/x", "ctl/inbox/c1/x", "ctl/finished/c1/x")
	w.refused(w.viewer, Overwrite, "ctl/running/c1/i1")
	w.noDelete(w.viewer, "ctl/running/c1/i1", "ctl/finished/c1/i0")

	// With no access at all, nothing is given.
	w.noRead(w.nobody, "ctl/running/c1/i1")
	w.noList(w.nobody, "ctl/running", "")
}

func TestVersionRow(t *testing.T) { eachTable(t, versionRow) }

func versionRow(t *testing.T, w table) {
	for _, s := range []Store{w.ctl, w.r1, w.c1, w.viewer, w.nobody} {
		w.reads(s, "version")
		w.refused(s, Overwrite, "version")
		w.noDelete(s, "version")
	}
}

// A read the store refuses is "not there"; a refused write, delete or
// listing is refused, never done and never "already there".
func Test_store_refused_read_is_not_there_refused_write_stays_refused(t *testing.T) {
	eachTable(t, refusedReadRefusedWrite)
}

func refusedReadRefusedWrite(t *testing.T, w table) {
	w.noRead(w.c1, "ctl/trust/r1", "ctl/trust/nothing")
	if done, err := w.c1.Write(ctx, "ctl/trust/r1", []byte("x"), OnlyIfNew); done || !errors.Is(err, ErrRefused) {
		t.Fatalf("a refused only-if-new write over a value: %v %v", done, err)
	}
	if done, err := w.c1.Delete(ctx, "ctl/trust/nothing", Overwrite); done || !errors.Is(err, ErrRefused) {
		t.Fatalf("a refused delete of nothing: %v %v", done, err)
	}
	w.noList(w.c1, "ctl/trust")
}

// The whole wall between limited parties is that each of their writes is
// only-if-new, but in a runner's own area and among kept values.
func Test_store_policy_limited_party_cannot_replace(t *testing.T) {
	for _, kind := range []Kind{Runner, Client, Viewer, Nobody} {
		for _, g := range (Party{Kind: kind, Name: "p", Controller: "ctl"}).grants() {
			own := kind == Runner && (g.pattern == "p/inbox/**" || g.pattern == "p/taken/**" || g.pattern == "ctl/kept/**")
			if !own && g.acts&(ActRewrite|ActDelete) != 0 {
				t.Errorf("kind %d may replace or delete %s", kind, g.pattern)
			}
		}
	}

	// And so in the policy printed for each: outside a runner's own area
	// and the kept values, nothing is deleted, and nothing is put under a
	// name by a request that does not carry the condition.
	for _, who := range []string{"runner", "client", "viewer", "nobody"} {
		for _, s := range carry(t, policyParties[who]).Statement {
			_, onlyIfNew := s.Condition["Null"]["s3:if-none-match"]
			_, noValue := s.Condition["Bool"]["s3:ObjectCreationOperation"]
			changes := s.Action == "s3:DeleteObject" || (s.Action == "s3:PutObject" && !onlyIfNew && !noValue)
			for _, resource := range s.Resource {
				own := strings.HasPrefix(resource, "arn:aws:s3:::bkt/pre/fix/r1/") || (who == "runner" && strings.HasPrefix(resource, "arn:aws:s3:::bkt/pre/fix/ctl/kept/"))
				if changes && !own {
					t.Errorf("the %s's policy lets it replace or delete %s", who, resource)
				}
			}
		}
	}
}
