package client_test

import (
	"strings"
	"testing"

	"github.com/zummed/corbie/store"
)

// A value one run keeps is read by a later run, and in the store it is
// encrypted, at its own name, where only the runner and the controller
// may reach it.
func TestKeptValueOutlivesTheRunThatKeptIt(t *testing.T) {
	const marker = "MARKER-IN-A-KEPT-VALUE"
	l := local(t, "all")
	keeps := `
		flow: keeps
		stages:
		  it: { job: { script: ./it.js, allow: ["kept.write:notes/**", "kept.read:notes/**"] } }
		output: ${stages.it.output}
	`
	r := run(t, l, keeps, map[string]string{"it.js": `kept.write("notes/tea", { text: "` + marker + `" }); return kept.list("notes")`}, nil)
	if r.Status != "done" {
		t.Fatalf("the run that keeps a value ended %s: %s %s", r.Status, r.Error, r.Refused)
	}
	r = run(t, l, keeps, map[string]string{"it.js": `return kept.read("notes/tea")`}, nil)
	if r.Status != "done" || r.Output != marker {
		t.Fatalf("a later run read %v (%s %s)", r.Output, r.Status, r.Error)
	}
	owner := l.Memory.As(store.Party{Kind: store.Owner})
	sealed, mark, err := owner.Read(ctx, store.Kept(l.Controller.ID.Name, "notes/tea"))
	if err != nil || mark == store.None || strings.Contains(string(sealed), marker) {
		t.Fatalf("in the store the kept value is %q (%v)", sealed, err)
	}
	client := l.Memory.As(store.Party{Kind: store.Client, Name: l.Client.ID.Name, Controller: l.Controller.ID.Name})
	if value, _, _ := client.Read(ctx, store.Kept(l.Controller.ID.Name, "notes/tea")); value != nil {
		t.Fatal("a client read a kept value from the store")
	}
}

// A job that some layer allows no kept value is sent no key, and its
// call is refused, naming that layer.
func TestKeptValueRefusedByTheClientsCeiling(t *testing.T) {
	l := local(t, "disk")
	r := run(t, l, `
		flow: reads
		stages:
		  it: { job: { script: ./it.js, allow: ["kept.read:notes/**"] } }
	`, map[string]string{"it.js": `return kept.read("notes/tea")`}, nil)
	if r.Status != "failed" || r.Failure != "denied" || r.Refused != "kept.read:notes/tea" || !strings.Contains(r.Layer, "the ceiling of the client") {
		t.Fatalf("a read the client's ceiling does not allow ended %s/%s: %s by %s", r.Status, r.Failure, r.Refused, r.Layer)
	}
}
