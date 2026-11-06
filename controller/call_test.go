package controller_test

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// caller is the transient flow at /t/caller: it calls /t/lookup with its
// input, and its output is what the call gave.
const caller = `
	flow: caller
	allow: ["send:lookup"]
	stages:
	  look: { job: { call: { at: lookup }, input: "${input}" } }
	output: ${stages.look.output}
`

// lookup is the transient chart at /t/lookup: it ends at once with what it
// was given.
const lookup = `
	statechart: lookup
	initial: a
	states:
	  a: { end: done, output: { got: "${input}" } }
`

// calls submits the tree /t with the caller and the lookup, and any other
// files by their location, and runs the caller with an input. It gives the
// caller's run.
func (w *world) calls(files map[string]string, input any) string {
	w.t.Helper()
	all := map[string]string{"/t/caller": caller, "/t/lookup": lookup}
	for at, src := range files {
		all[at] = src
	}
	if r := w.submitTree(w.client, w.tree("/t", "/srv/t", all, nil)); !r.Taken {
		w.t.Fatalf("the tree was refused: %s", r.Refused)
	}
	run := w.startAt(w.client, "/t/caller", client.Start{Input: input})
	w.settle()
	return run
}

// called gives the runs a run's call jobs started, as its record names them.
func (w *world) called(run string) []string {
	w.t.Helper()
	var runs []string
	for _, step := range w.steps(run) {
		for _, ask := range step.Asks {
			if ask.Kind == instance.AskCall {
				runs = append(runs, ask.Run)
			}
		}
	}
	return runs
}

// failure gives the error a run ended with.
func (w *world) failure(run string) string {
	w.t.Helper()
	st := w.latest(run).State
	if st.Error == nil {
		return ""
	}
	return st.Error.Status + ": " + st.Error.Error
}

// A call job starts a run of a transient location and waits; its output is
// that run's result. The called run is a root of the location's tree client,
// one level below its caller, and its head names the run that called it.
func Test_ctl_call_returns_the_runs_result(t *testing.T) {
	w := newWorld(t)
	run := w.calls(nil, map[string]any{"n": "seven"})
	if head := w.head(run); head.Status != instance.Done {
		t.Fatalf("the caller is %s: %s", head.Status, w.failure(run))
	}
	got, _ := w.latest(run).State.Output.(map[string]any)
	if inner, _ := got["got"].(map[string]any); inner["n"] != "seven" {
		t.Fatalf("the caller's output is %v", w.latest(run).State.Output)
	}
	callee := w.called(run)
	if len(callee) != 1 {
		t.Fatalf("the caller's record names the runs %v", callee)
	}
	head := w.head(callee[0])
	if head == nil || head.Parent != "" || head.StartedBy != run || head.Location != "/t/lookup" || store.InstanceClient(callee[0]) != "c1" {
		t.Fatalf("the called run's head is %+v", head)
	}
	if st := w.latest(callee[0]).State; st.Above.Depth != 1 || st.Above.CallJob == "" {
		t.Fatalf("the called run is at depth %d for the job %q", st.Above.Depth, st.Above.CallJob)
	}
}

// What a call gives and what its result names are copied: the input's blobs
// into the called run's record, and its output's into the caller's.
func Test_ctl_call_copies_blobs_both_ways(t *testing.T) {
	w := newWorld(t)
	run := w.calls(nil, map[string]any{"file": client.InputFile{Name: "a.txt", Contents: strings.NewReader("hello")}})
	if head := w.head(run); head.Status != instance.Done {
		t.Fatalf("the caller is %s: %s", head.Status, w.failure(run))
	}
	callee := w.called(run)[0]
	given := blob.Refs(w.latest(callee).State.Input)
	back := blob.Refs(w.latest(run).State.Output)
	if len(given) != 1 || !strings.HasPrefix(given[0].At, store.Record("ctl", callee)+"/") {
		t.Fatalf("the called run's input names %v", given)
	}
	if len(back) != 1 || !strings.HasPrefix(back[0].At, store.Record("ctl", run)+"/") {
		t.Fatalf("the caller's output names %v", back)
	}
	data, err := blob.ReadAll(ctx, w.owner, back[0], 1<<20)
	if err != nil || string(data) != "hello" {
		t.Fatalf("the file came back as %q, %v", data, err)
	}
}

// A call fails its job when the location is not transient, when nothing is
// recorded there, when the run's own layers do not hold send: for it, when
// the receiver's receive: does not list the caller, and when the called
// run fails. Nothing of a run is written for a call that no run takes.
func Test_ctl_call_fails_its_job(t *testing.T) {
	for name, c := range map[string]struct {
		files  map[string]string
		denied bool
		want   string
	}{
		"singleton": {files: map[string]string{"/t/lookup": strings.Replace(lookup, "statechart: lookup", "statechart: lookup\n\tlifetime: singleton", 1)},
			want: "/t/lookup is singleton: a call starts a run only of a transient location"},
		"nothing": {files: map[string]string{"/t/caller": strings.Replace(strings.Replace(caller, "at: lookup", "at: other", 1), "send:lookup", "send:**", 1)},
			want: "nothing is recorded at /t/other"},
		"send": {files: map[string]string{"/t/caller": strings.Replace(caller, `allow: ["send:lookup"]`, `allow: ["send:elsewhere"]`, 1)},
			denied: true, want: "send:/t/lookup"},
		"receive": {files: map[string]string{"/t/lookup": strings.Replace(lookup, "statechart: lookup", "statechart: lookup\n\treceive: [nobody]", 1)},
			want: "/t/lookup does not receive a call from the run"},
		"failed": {files: map[string]string{"/t/lookup": strings.Replace(lookup, "end: done", "end: failed", 1)},
			want: "end: failed"},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			run := w.calls(c.files, map[string]any{"n": 1})
			failure := w.failure(run)
			if head := w.head(run); head.Status != instance.Failed || !strings.Contains(failure, c.want) {
				t.Fatalf("the caller is %s: %s", head.Status, failure)
			}
			if c.denied != strings.HasPrefix(failure, machine.StatusDenied) {
				t.Fatalf("the caller failed %s", failure)
			}
			runs := w.runsOf("lookup")
			if name == "failed" {
				if len(runs) != 1 || w.head(runs[0]).Status != instance.Failed {
					t.Fatalf("the called runs are %v", runs)
				}
			} else if len(runs) != 0 && name != "singleton" {
				t.Fatalf("a call no run took left the runs %v", runs)
			}
		})
	}
}

// A called run fails as it starts when it is one level more than its own
// file's depth limit allows, and its caller's job fails with it.
func Test_ctl_call_counts_one_level_below_its_caller(t *testing.T) {
	w := newWorld(t)
	run := w.calls(map[string]string{"/t/lookup": strings.Replace(lookup, "initial: a", "limits: { depth: 0 }\n\tinitial: a", 1)}, nil)
	if failure := w.failure(run); !strings.Contains(failure, "depth limit reached") {
		t.Fatalf("the caller failed %q", failure)
	}
}

// waiter is a transient chart that ends with its input when it is told go.
const waiter = `
	statechart: lookup
	limits: { timeout: 72h }
	initial: w
	states:
	  w: { on: { go: over } }
	  over: { end: done, output: "${input}" }
`

// Cancelling the caller cancels the job and the run it called.
func Test_ctl_cancelling_a_call_cancels_the_run(t *testing.T) {
	w := newWorld(t)
	run := w.calls(map[string]string{"/t/lookup": waiter}, nil)
	callee := w.called(run)
	if len(callee) != 1 || w.head(callee[0]).Status != instance.Running {
		t.Fatalf("the caller called %v", callee)
	}
	if _, err := w.client.Cancel(ctx, run, time.Hour); err != nil {
		t.Fatal(err)
	}
	w.settle()
	if a, b := w.head(run).Status, w.head(callee[0]).Status; a != instance.Cancelled || b != instance.Cancelled {
		t.Fatalf("the caller is %s and the called run %s", a, b)
	}
}

// Removing a caller once it is past keeping leaves the run it called, which
// is no child of it, and is kept as its own file says.
func Test_ctl_removing_a_caller_leaves_the_run_it_called(t *testing.T) {
	w := newWorld(t)
	run := w.calls(map[string]string{"/t/caller": strings.Replace(caller, "flow: caller", "flow: caller\n\tkeep: 1h", 1)}, nil)
	callee := w.called(run)
	if len(callee) != 1 || w.head(callee[0]).Status != instance.Done {
		t.Fatalf("the caller called %v", callee)
	}
	w.tick(2 * time.Hour)
	w.settle()
	if w.head(run) != nil || w.head(callee[0]) == nil {
		t.Fatalf("after the caller's keep the caller is %v and the called run %v", w.head(run), w.head(callee[0]))
	}
}

// A called run's held jobs go to its own file's ask, and one with none is
// refused as it starts, though its caller has an ask.
func Test_ctl_called_runs_held_jobs_go_to_its_own_ask(t *testing.T) {
	held := `
		flow: lookup
		ASK
		stages:
		  s: { job: { script: ./work.js, approve: true } }
	`
	asking := strings.Replace(caller, "flow: caller", "flow: caller\n\task: ./ask.js", 1)
	scripts := map[string]string{"work.js": "return 1", "ask.js": "return { approve: [] }"}
	for _, own := range []bool{true, false} {
		w := newWorld(t)
		w.newRunner()
		ask := ""
		if own {
			ask = "ask: ./ask.js"
		}
		all := map[string]string{"/t/caller": asking, "/t/lookup": strings.Replace(held, "ASK", ask, 1)}
		if r := w.submitTree(w.client, w.tree("/t", "/srv/t", all, scripts)); !r.Taken {
			t.Fatalf("the tree was refused: %s", r.Refused)
		}
		run := w.startAt(w.client, "/t/caller", client.Start{})
		w.settle()
		if !own {
			if failure := w.failure(run); !strings.Contains(failure, "no ask applies") {
				t.Fatalf("with no ask of its own the call gave %q", failure)
			}
			continue
		}
		callee := w.called(run)[0]
		var asked []string
		for _, step := range w.steps(callee) {
			for _, a := range step.Asks {
				if a.Kind == instance.AskScript {
					asked = append(asked, a.Job.Name+" "+a.Job.Found.In)
				}
			}
		}
		if !slices.Equal(asked, []string{"ask lookup"}) {
			t.Fatalf("the called run sent %v", asked)
		}
	}
}

// A caller's ask_about: [send:…] holds a call job for its ask; a
// statechart's send action with the same list is never held.
func Test_ctl_ask_about_send_holds_a_call(t *testing.T) {
	w := newWorld(t)
	held := strings.Replace(caller, "flow: caller", "flow: caller\n\task_about: [\"send:**\"]\n\task: ./ask.js", 1)
	all := map[string]string{"/t/caller": held, "/t/lookup": lookup, "/t/room": talk,
		"/t/router": strings.Replace(router, "statechart: router", "statechart: router\n\task_about: [\"send:**\"]\n\task: ./ask.js", 1)}
	if r := w.submitTree(w.client, w.tree("/t", "/srv/t", all, map[string]string{"ask.js": "return { approve: [] }"})); !r.Taken {
		t.Fatalf("the tree was refused: %s", r.Refused)
	}
	run := w.startAt(w.client, "/t/caller", client.Start{})
	w.settle()
	var holds int
	for _, step := range w.steps(run) {
		for _, ask := range step.Asks {
			if ask.Kind == instance.AskHold && ask.Job.IsCall() {
				holds++
			}
		}
	}
	if holds != 1 || len(w.called(run)) != 0 {
		t.Fatalf("the call was held %d times and called %v", holds, w.called(run))
	}
	sender := w.router()
	w.say(sender, "say", 1, "not held")
	w.settle()
	if runs := w.runsOf("talk"); len(runs) != 1 || !slices.Equal(w.told(runs[0]), []string{"not held"}) {
		t.Fatalf("the send action gave the runs %v", runs)
	}
}

// A client calls a location in a tree it does not own by starting it. The
// receipt says the call was taken, naming the run, while the run is running,
// and is kept while it runs; when the run ends its result is beside the
// receipt, with the blobs it names, and the two go a day after the result
// was written. The client is shown the call's status and output.
func Test_ctl_taken_call_receipt_waits_for_its_result(t *testing.T) {
	w := newWorld(t)
	tree := w.tree("/t", "/srv/t", map[string]string{"/t/lookup": strings.Replace(waiter, "initial: w", "receive: [client:c2]\n\tinitial: w", 1)}, nil)
	if r := w.submitTree(w.client, tree); !r.Taken {
		t.Fatalf("the tree was refused: %s", r.Refused)
	}
	c2 := w.other("c2", "send:/t/**")
	id, err := c2.Start(ctx, client.Start{At: "/t/lookup", Input: map[string]any{"file": client.InputFile{Name: "a.txt", Contents: strings.NewReader("marker")}}})
	if err != nil {
		t.Fatal(err)
	}
	w.settle()
	message := strings.SplitN(id, "/", 2)[1]
	r, err := c2.Receipt(ctx, message)
	if err != nil || r == nil || !r.Taken || r.Run == "" || store.InstanceClient(r.Run) != "c1" {
		t.Fatalf("the call's receipt is %+v, %v", r, err)
	}
	callee := r.Run
	if shown, err := c2.Show(ctx, id); err != nil || shown.Status != instance.Running || shown.Called != callee {
		t.Fatalf("the call is shown as %+v, %v", shown, err)
	}
	w.tick(48 * time.Hour)
	w.settle()
	if r, _ := c2.Receipt(ctx, message); r == nil {
		t.Fatal("the receipt of a call whose run is running went after a day")
	}
	if w.clientSend(w.client, callee, "go", nil); w.head(callee).Status != instance.Done {
		t.Fatalf("the called run is %s", w.head(callee).Status)
	}
	shown, err := c2.Show(ctx, id)
	if err != nil || shown.Status != instance.Done {
		t.Fatalf("the call is shown as %+v, %v", shown, err)
	}
	reader, _, err := c2.Fetch(ctx, id, "file")
	if err != nil {
		t.Fatalf("the call's file: %v", err)
	}
	if data, err := io.ReadAll(reader); string(data) != "marker" {
		t.Fatalf("the call's file holds %q: %v", data, err)
	}
	if beside := w.list(store.ReceiptBlobs("ctl", "c2", message)); len(beside) != 1 {
		t.Fatalf("beside the receipt are the blobs %v", beside)
	}
	// c1 cannot open the result.
	if data, _, _ := w.owner.Read(ctx, store.Result("ctl", "c2", message)); strings.Contains(string(data), "marker") || data == nil {
		t.Fatalf("the result read raw is %q", data)
	}
	w.tick(23 * time.Hour)
	w.settle()
	if r, _ := c2.Receipt(ctx, message); r == nil {
		t.Fatal("the receipt went within a day of the result")
	}
	w.tick(2 * time.Hour)
	w.settle()
	if left := w.list(store.Receipts("ctl", "c2")); len(left) != 0 {
		t.Fatalf("a day after the result is written there is still %v", left)
	}
}

// A client's call is refused in its receipt when its ceiling has no send:
// for the location, when the location is not transient, and when it gives
// labels or an allow list, which are for the runs of its own trees.
func Test_ctl_client_call_is_refused(t *testing.T) {
	w := newWorld(t)
	tree := w.tree("/t", "/srv/t", map[string]string{
		"/t/lookup": strings.Replace(lookup, "initial: a", "receive: [client:c2]\n\tinitial: a", 1),
		"/t/s":      strings.Replace(lookup, "initial: a", "lifetime: singleton\n\treceive: [client:c2]\n\tinitial: a", 1)}, nil)
	if r := w.submitTree(w.client, tree); !r.Taken {
		t.Fatalf("the tree was refused: %s", r.Refused)
	}
	refused := func(c *client.Client, start client.Start) string {
		id, err := c.Start(ctx, start)
		if err != nil {
			t.Fatal(err)
		}
		w.settle()
		r, err := c.Receipt(ctx, strings.SplitN(id, "/", 2)[1])
		if err != nil || r == nil {
			t.Fatalf("no receipt: %v", err)
		}
		return r.Refused
	}
	if why := refused(w.other("c2", "network"), client.Start{At: "/t/lookup"}); !strings.Contains(why, "send:/t/lookup") {
		t.Fatalf("a call with no send: gave %q", why)
	}
	c2 := w.other("c2", "send:/t/**")
	if why := refused(c2, client.Start{At: "/t/s"}); !strings.Contains(why, "only by calling a transient location") {
		t.Fatalf("a call of a singleton gave %q", why)
	}
	if why := refused(c2, client.Start{At: "/t/lookup", Labels: map[string]string{"a": "b"}}); !strings.Contains(why, "labels and an allow list") {
		t.Fatalf("a call with labels gave %q", why)
	}
	if why := refused(c2, client.Start{At: "/t/lookup"}); why != "" {
		t.Fatalf("a call was refused: %s", why)
	}
}
