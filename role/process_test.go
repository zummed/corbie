package role_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/runner"
	"github.com/zummed/corbie/store"
)

// What a process started by a test to make one pass is told, in its
// environment: which role, and where its config and its store are.
const (
	passOf     = "CORBIE_TEST_PASS_OF"
	passConfig = "CORBIE_TEST_PASS_CONFIG"
	passStore  = "CORBIE_TEST_PASS_STORE"
)

// passFrom makes a role's pass from a config directory and a store's address,
// which is all a process has when it starts.
func passFrom(name, config, address string) (role.Pass, error) {
	cfg := identity.Config{Dir: config}
	s, err := store.Open(address)
	if err != nil {
		return nil, err
	}
	switch name {
	case "controller":
		id, err := cfg.Load(identity.Controller)
		if err != nil {
			return nil, err
		}
		return controller.New(id, s).Pass, nil
	case "runner":
		id, err := cfg.Load(identity.Runner)
		if err != nil {
			return nil, err
		}
		held, err := cfg.TrustList(identity.Runner)
		if err != nil {
			return nil, err
		}
		var serves []runner.Served
		for _, t := range held {
			serves = append(serves, runner.Served{Controller: t.Card, Ceiling: t.Ceiling, Store: s})
		}
		made := runner.New(id, serves...)
		made.Command = []string{os.Args[0], runner.JobArg}
		return made.Pass, nil
	}
	return nil, fmt.Errorf("%q is not a role", name)
}

// onePass is a process that makes one pass of a role, says it has
// answered, and stays as it is until it is killed.
func onePass(name string) {
	pass, err := passFrom(name, os.Getenv(passConfig), os.Getenv(passStore))
	if err == nil {
		_, err = pass(context.Background())
	}
	if err != nil {
		fmt.Println("failed:", err)
	} else {
		fmt.Println("answered")
	}
	for {
		time.Sleep(time.Hour)
	}
}

// inProcess has one pass of a role made by a process of its own, which is
// killed as soon as it has answered.
func inProcess(t *testing.T, name, config, address string) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), passOf+"="+name, passConfig+"="+config, passStore+"="+address)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	said, _ := bufio.NewReader(out).ReadString('\n')
	cmd.Process.Kill()
	cmd.Wait()
	if strings.TrimSpace(said) != "answered" {
		t.Fatalf("a process that was to make one pass of the %s said %q", name, said)
	}
}

var (
	aTime   = regexp.MustCompile(`\d{8}T\d{6}\.\d+Z`)
	aRandom = regexp.MustCompile(`-[0-9a-f]{6,}`)
)

func Test_role_fresh_process_every_pass_same_store(t *testing.T) {
	const oneJob = `
		statechart: one-job
		initial: working
		states:
		  working:
		    job: { script: ./work.js }
		    on: { done: over }
		  over: { end: done, output: "${states.working.output}" }
	`
	// work does the same things to a new store, with every pass of a role
	// made by a process of its own or with all of a role's passes made in
	// one process, and tells what the store then holds: every name, with
	// its times and its random parts taken out, and where each run stands.
	work := func(fresh bool) []string {
		config, directory := t.TempDir(), t.TempDir()
		cfg := identity.Config{Dir: config}
		ids := map[identity.Kind]*identity.Identity{}
		for kind, name := range map[identity.Kind]string{identity.Controller: "ctl", identity.Runner: "r1", identity.Client: "c1"} {
			id, err := cfg.Make(kind, name)
			if err != nil {
				t.Fatal(err)
			}
			ids[kind] = id
		}
		ctl, all := ids[identity.Controller], permission.Layer{Allow: []string{"all"}}
		s, err := store.Open(directory)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SetUp(ctx, s); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []identity.Kind{identity.Runner, identity.Client} {
			if err := identity.WriteTrust(ctx, s, ctl, identity.Trust{Card: ids[kind].Card, Ceiling: all}); err != nil {
				t.Fatal(err)
			}
		}
		if err := cfg.SetTrust(identity.Runner, identity.Trust{Card: ctl.Card, Ceiling: all, Store: directory}); err != nil {
			t.Fatal(err)
		}
		shared := map[string]role.Pass{}
		pass := func(name string) {
			t.Helper()
			if fresh {
				inProcess(t, name, config, directory)
				return
			}
			if shared[name] == nil {
				if shared[name], err = passFrom(name, config, directory); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := shared[name](ctx); err != nil {
				t.Fatal(err)
			}
		}
		c := client.New(ids[identity.Client], ctl.Card, s)
		start := func() string {
			t.Helper()
			draft, err := machine.Parse([]byte(dedent(oneJob)), machine.Given(map[string]string{"work.js": "return 7\n"}))
			if err != nil {
				t.Fatal(err)
			}
			run, err := c.Start(ctx, draft, client.Start{})
			if err != nil {
				t.Fatal(err)
			}
			return run
		}
		// A run whose job is done, and whose job then arrives a second time,
		// as it does when a controller carries out a step again: the runner
		// is shown a job that an earlier pass of its own has answered.
		again := start()
		pass("controller")
		jobs, err := s.List(ctx, store.Inbox("r1"))
		if err != nil || len(jobs) != 1 {
			t.Fatalf("the jobs sent are %v (%v)", jobs, err)
		}
		job, _, _ := s.Read(ctx, jobs[0])
		pass("runner")
		if _, err := s.Write(ctx, jobs[0], job, store.OnlyIfNew); err != nil {
			t.Fatal(err)
		}
		pass("runner")
		pass("controller")
		// A run whose job is done, and which is cancelled before the
		// controller has taken the result: the runner is then sent a cancel
		// for a job that an earlier pass of its own has finished.
		cancelled := start()
		pass("controller")
		pass("runner")
		if _, err := c.Cancel(ctx, cancelled, 0); err != nil {
			t.Fatal(err)
		}
		pass("controller")
		pass("runner")
		// And a run that goes to its end.
		done := start()
		for range 2 {
			pass("controller")
			pass("runner")
		}
		pass("controller")
		var told []string
		names, err := s.List(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			told = append(told, aRandom.ReplaceAllString(aTime.ReplaceAllString(name, "T"), "-x"))
		}
		for _, run := range []string{again, cancelled, done} {
			head, _, err := instance.ReadHead(ctx, s, ctl.Card, run)
			if err != nil || head == nil {
				t.Fatalf("the head of a run: %v", err)
			}
			told = append(told, fmt.Sprintf("a run is %s at step %d", head.Status, head.Step))
		}
		slices.Sort(told)
		return told
	}
	together := work(false)
	for _, part := range []string{"a run is cancelled at step 2", "a run is done at step 2", "r1/inbox/ctl/c1.T-x.working"} {
		if !slices.Contains(together, part) {
			t.Fatalf("with a role's passes in one process the work ended with no %q:\n%s", part, strings.Join(together, "\n"))
		}
	}
	apart := work(true)
	if !slices.Equal(apart, together) {
		t.Fatalf("with every pass in a process of its own the store holds\n%s\nand with a role's passes in one process\n%s",
			strings.Join(apart, "\n"), strings.Join(together, "\n"))
	}
}
