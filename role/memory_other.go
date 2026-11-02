//go:build !linux && !windows

package role

// LimitMemory does nothing here: the system has no limit it enforces, and
// what a process holds is watched from inside.
func LimitMemory(limit uint64) {}
