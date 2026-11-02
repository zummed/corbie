package role

import (
	"fmt"
	"os"
	"syscall"
)

// LimitMemory has the system refuse this process more memory than a limit
// past what it has reserved so far, so that a request for more ends it at
// once and fills nothing. Go reserves about a gigabyte of addresses it never
// uses, and the system counts addresses.
func LimitMemory(limit uint64) {
	statm, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return
	}
	var pages uint64
	if _, err := fmt.Sscan(string(statm), &pages); err != nil {
		return
	}
	limit += pages * uint64(os.Getpagesize())
	var now syscall.Rlimit
	if syscall.Getrlimit(syscall.RLIMIT_AS, &now) == nil && now.Max < limit {
		return
	}
	syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: limit, Max: limit})
}
