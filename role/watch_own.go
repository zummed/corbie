//go:build !js

package role

import (
	"context"
	"runtime"
	"time"
)

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
