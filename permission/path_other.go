//go:build !windows

package permission

import (
	"path"
	"strings"
)

// splitPath gives the parts of a path written with /. A path from the root
// has an empty first part. It says whether the path names a place.
func splitPath(p string) ([]string, bool) {
	if p = path.Clean(p); p == "/" {
		return []string{""}, true
	}
	return strings.Split(p, "/"), true
}

// rooted says whether a path written with / starts at the root of the
// disk, and not in the scratch directory.
func rooted(p string) bool { return strings.HasPrefix(p, "/") }

// fold gives a part of a path as it is compared.
func fold(part string) string { return part }

// programName gives a program's name as it is compared.
func programName(name string) string { return name }

// ProgramFiles gives the files a program at a path may be.
func ProgramFiles(p string) []string { return []string{p} }
