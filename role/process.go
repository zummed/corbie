package role

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"
)

// The binary runs one piece of work in a process of its own: a job for a
// runner, a step for a controller. The process is one its starter can stop at
// any moment, and it cannot bring its starter down. It starts with nothing of
// the starter's environment, and the two talk over a channel of their own,
// which nothing it prints can reach.

// Arg is the argument that makes the binary run one piece of work, as the
// process that is started for it.
const Arg = "run-one-job"

// Grace is how long a process that is told to stop is given before it is
// killed, and how long its output is read after it has ended.
const Grace = 2 * time.Second

// Open is how a process started by Start begins: it takes its ends of the
// channel, and makes itself a group where the system needs that. Everything
// it starts is in that group, and goes when the group is ended.
func Open() (*Wire, error) {
	from, to := takeChannel()
	if err := startGroup(); err != nil {
		return nil, fmt.Errorf("the process could not make a group of its own: %w", err)
	}
	return &Wire{R: from, W: to}, nil
}

// Lost ends the process, and everything it started with it: its starter has
// gone away, and nothing else would stop them.
func Lost() {
	endGroup()
	os.Exit(3)
}

// WatchOwn calls over once, when what this process holds passes a limit.
func WatchOwn(ctx context.Context, limit int64, over func()) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if int64(m.HeapAlloc) > limit {
				// The count includes what is no longer held and has not
				// been collected yet, so that is collected before the
				// count is judged.
				runtime.GC()
				runtime.ReadMemStats(&m)
			}
			if int64(m.HeapAlloc) > limit {
				over()
				return
			}
		}
	}
}
