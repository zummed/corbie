package role

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoopSleepsUntilTheNextThingIsDue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var passes []time.Time
	var mu sync.Mutex
	start := time.Now()
	pass := func(ctx context.Context) (time.Time, error) {
		mu.Lock()
		defer mu.Unlock()
		passes = append(passes, time.Now())
		switch len(passes) {
		case 1:
			return time.Now().Add(50 * time.Millisecond), nil // a wake due soon
		case 3:
			cancel()
		}
		return time.Time{}, nil
	}
	notice := make(chan struct{}, 1)
	go func() {
		time.Sleep(120 * time.Millisecond)
		notice <- struct{}{} // another role in the process wrote something
	}()
	Run(ctx, time.Now, nil, Role{Name: "r", Pass: pass, Interval: time.Hour, Notice: notice})
	if len(passes) != 3 {
		t.Fatalf("%d passes were made", len(passes))
	}
	if second := passes[1].Sub(start); second < 40*time.Millisecond || second > 110*time.Millisecond {
		t.Fatalf("the pass for a wake due in 50ms came after %v", second)
	}
	if third := passes[2].Sub(start); third < 110*time.Millisecond || third > time.Second {
		t.Fatalf("the pass woken by a notice came after %v", third)
	}
}

// A stand-in for a serverless host: it makes calls, each with a time by
// which it would kill the copy.
func TestServerlessHostCallsForPasses(t *testing.T) {
	var asked, answered atomic.Int32
	var answers []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/invocation/next"):
			if asked.Add(1) > 2 {
				<-r.Context().Done() // no more calls
				return
			}
			w.Header().Set("Lambda-Runtime-Aws-Request-Id", "call-"+strconv.Itoa(int(asked.Load())))
			w.Header().Set("Lambda-Runtime-Deadline-Ms", strconv.FormatInt(time.Now().Add(30*time.Second).UnixMilli(), 10))
		case strings.HasSuffix(r.URL.Path, "/response"):
			body, _ := io.ReadAll(r.Body)
			answers = append(answers, r.URL.Path+" "+string(body))
			if answered.Add(1) == 2 {
				cancel()
			}
		}
	}))
	defer host.Close()
	var limits []time.Duration
	pass := func(ctx context.Context) (time.Time, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("a pass on a serverless host was not told how long it may run")
		}
		limits = append(limits, time.Until(deadline))
		return time.Time{}, nil
	}
	if err := serveLambda(ctx, host.URL, nil, []Role{{Name: "controller", Pass: pass}}); err != nil {
		t.Fatal(err)
	}
	if len(limits) != 2 || len(answers) != 2 || !strings.Contains(answers[0], "call-1/response") || !strings.Contains(answers[0], `"controller":"done"`) {
		t.Fatalf("two calls gave %d passes and the answers %v", len(limits), answers)
	}
	// The pass finishes a margin before the host's limit.
	if limits[0] > 30*time.Second-HostMargin+time.Second || limits[0] < 15*time.Second {
		t.Fatalf("with thirty seconds from the host, the pass was given %v", limits[0])
	}
}

// The host says when it kills the copy, once for a call. Each role in turn
// is told to stop a margin before that same time, so the second has what
// the first left; and a call that does not say is not taken.
func TestEachRoleHasWhatIsLeftOfTheCall(t *testing.T) {
	until := time.Now().Add(40 * time.Second).Truncate(time.Millisecond)
	says := strconv.FormatInt(until.UnixMilli(), 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	asked := 0
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/invocation/next"):
			if asked++; asked > 1 {
				<-r.Context().Done()
				return
			}
			w.Header().Set("Lambda-Runtime-Aws-Request-Id", "call-1")
			w.Header().Set("Lambda-Runtime-Deadline-Ms", says)
		case strings.HasSuffix(r.URL.Path, "/response"):
			cancel()
		}
	}))
	defer host.Close()
	told := map[string]time.Time{}
	pass := func(name string) Pass {
		return func(ctx context.Context) (time.Time, error) {
			told[name], _ = ctx.Deadline()
			time.Sleep(20 * time.Millisecond) // the first role takes some of the call's time
			return time.Time{}, nil
		}
	}
	if err := serveLambda(ctx, host.URL, nil, []Role{{Name: "controller", Pass: pass("controller")}, {Name: "runner", Pass: pass("runner")}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"controller", "runner"} {
		if !told[name].Equal(until.Add(-HostMargin)) {
			t.Fatalf("the host kills the copy at %s, and the %s was told to stop at %s", until.Format(time.StampMilli), name, told[name].Format(time.StampMilli))
		}
	}

	// A call with no time on it.
	says, asked, told = "soon", 0, map[string]time.Time{}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	err := serveLambda(ctx, host.URL, nil, []Role{{Name: "controller", Pass: pass("controller")}})
	if err == nil || !strings.Contains(err.Error(), "until when") || len(told) != 0 {
		t.Fatalf("a call that did not say until when it may run: %v, and %d passes were made", err, len(told))
	}

	// With less than two margins left, a pass has half of what there is.
	stop := time.Time{}
	left := time.Now().Add(HostMargin)
	Once(context.Background(), left, HostMargin, func(ctx context.Context) (time.Time, error) {
		stop, _ = ctx.Deadline()
		return time.Time{}, nil
	})
	if until := left.Sub(stop); until < HostMargin/2-time.Second || until > HostMargin/2 {
		t.Fatalf("with %s left, a pass was told to stop %s before the host's limit", HostMargin, until)
	}
}
