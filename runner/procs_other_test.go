//go:build !unix

package runner_test

func standIn(as string, args []string) bool { return false }
