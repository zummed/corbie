package permission

import (
	"path"
	"path/filepath"
	"strings"
)

// splitPath gives the parts of a path written with /. The first part is
// its drive, C:, or its share, //server/share. A path with no drive or
// share names no place: /srv is on whichever drive is current.
func splitPath(p string) ([]string, bool) {
	volume := filepath.VolumeName(p)
	rest := p[len(volume):]
	if volume == "" || rest != "" && !strings.HasPrefix(rest, "/") {
		return nil, false
	}
	rest = path.Clean("/" + rest)
	if rest == "/" {
		return []string{volume}, true
	}
	parts := strings.Split(rest, "/")
	parts[0] = volume
	return parts, true
}

// rooted says whether a path written with / is not in the scratch
// directory: it has a drive or a share, or it starts with / and names no
// place.
func rooted(p string) bool { return filepath.VolumeName(p) != "" || strings.HasPrefix(p, "/") }

// fold gives a part of a path as it is compared: Windows takes a name in
// any case for the same file.
func fold(part string) string { return strings.ToLower(part) }

// programName gives a program's name as it is compared: in any case, and
// git.exe as git, since Windows starts the one for the other.
func programName(name string) string {
	name = strings.ToLower(name)
	for _, extension := range programExtensions {
		if trimmed, ok := strings.CutSuffix(name, extension); ok && trimmed != "" {
			return trimmed
		}
	}
	return name
}

// programExtensions are the files a program named without one is found
// as. A batch file is not one: cmd.exe reads its arguments again, as
// commands.
var programExtensions = []string{".exe", ".com"}

// ProgramFiles gives the files a program at a path may be: the path
// itself if it names its extension, and otherwise the path with each.
func ProgramFiles(p string) []string {
	for _, extension := range programExtensions {
		if strings.HasSuffix(strings.ToLower(p), extension) {
			return []string{p}
		}
	}
	var files []string
	for _, extension := range programExtensions {
		files = append(files, p+extension)
	}
	return files
}
