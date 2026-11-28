package client

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A Label is one label as it is written: a name, or name=value.
type Label struct {
	Name, Value string
	// Valued is a label written with "=". In a listing it keeps only the
	// runs whose label has that value, and a name alone keeps any value.
	Valued bool
}

// String gives the label as it is written.
func (l Label) String() string {
	if l.Valued {
		return l.Name + "=" + l.Value
	}
	return l.Name
}

// Keeps says whether a run's labels have this one.
func (l Label) Keeps(labels map[string]string) bool {
	got, ok := labels[l.Name]
	return ok && (!l.Valued || got == l.Value)
}

// ParseLabels reads labels as they are written, each name once.
func ParseLabels(texts []string) ([]Label, error) {
	var labels []Label
	given := map[string]bool{}
	for _, text := range texts {
		name, value, valued := strings.Cut(text, "=")
		if err := checkLabel(name); err != nil {
			return nil, fmt.Errorf("the label %q: %w", text, err)
		}
		if given[name] {
			return nil, fmt.Errorf("the label %q is given twice", name)
		}
		given[name] = true
		labels = append(labels, Label{Name: name, Value: value, Valued: valued})
	}
	return labels, nil
}

// checkLabel refuses a label's name that is empty, longer than 256
// characters, or holds whitespace or "=".
func checkLabel(name string) error {
	if n := utf8.RuneCountInString(name); n == 0 || n > 256 || strings.ContainsFunc(name, func(r rune) bool { return r == '=' || unicode.IsSpace(r) }) {
		return errors.New(`a label's name is 1 to 256 characters, with no whitespace and no "="`)
	}
	return nil
}
