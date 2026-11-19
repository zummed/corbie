package machines

import (
	"errors"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/zummed/corbie/machine"
)

// A problem is one of Corbie's, at the file it names: the file, its line,
// the place, what is wrong, and the machines of the directory it breaks. Top says the file is a directory
// below the one the UI reads that is the top of a tree. included says the
// problem is a file an `include:` names that cannot be read.
type problem struct {
	File, Place, Text string
	Line              int
	Breaks            []string
	Top               bool
	included          bool
}

// cannotRead is how Corbie says a file a definition names cannot be read.
var cannotRead = regexp.MustCompile(`^("(?:[^"\\]|\\.)*") cannot be read: (.*)$`)

// includeAt matches the place of an `include:`.
var includeAt = regexp.MustCompile(`(^|\.)include(\[\d+\])?$`)

// problemsOf gives the problems Corbie found reading the machine in a file,
// each at the file and line it names, with its paths from the directory. A file an `include:` names that cannot be read
// is the problem of that file, and the problems that follow from it are
// left out.
func (d *dir) problemsOf(file string, err error) []problem {
	from := strings.NewReplacer(d.real+string(filepath.Separator), "", d.root+string(filepath.Separator), "")
	var found machine.Problems
	if !errors.As(err, &found) {
		return []problem{{File: file, Text: from.Replace(err.Error())}}
	}
	var out, included []problem
	for _, p := range found {
		at := file
		if p.File != "" {
			at = path.Clean(strings.TrimPrefix(p.File, "./"))
		}
		if m := cannotRead.FindStringSubmatch(p.Text); m != nil && includeAt.MatchString(p.Place) {
			if name, err := strconv.Unquote(m[1]); err == nil {
				included = append(included, problem{File: fromFile(at, name), Text: "cannot be read: " + from.Replace(m[2]), included: true})
				continue
			}
		}
		out = append(out, problem{File: at, Line: p.Line, Place: p.Place, Text: from.Replace(p.Text)})
	}
	if len(included) > 0 {
		return included
	}
	return out
}
