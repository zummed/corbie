//go:build !windows

package store

import "os"

func readFile(path string) ([]byte, error)   { return os.ReadFile(path) }
func openFile(path string) (*os.File, error) { return os.Open(path) }
func rename(from, to string) error           { return os.Rename(from, to) }
func remove(path string) error               { return os.Remove(path) }
