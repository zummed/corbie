package client

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// A step is one step of a place in the input: a name in a map, or an item
// of a list.
type step struct {
	name   string
	item   int
	inList bool
}

// placeOf reads a place in the input, written as a run shows the place of
// a blob: names joined by ".", each followed by any items of a list as [n],
// such as files[0].file.
func placeOf(place string) ([]step, error) {
	bad := fmt.Errorf("%w: %q is not a place in the input: a place is names joined by \".\", each followed by the items of a list as [n], such as files[0].file", ErrStart, place)
	var steps []step
	rest := place
	for {
		end := strings.IndexAny(rest, ".[]")
		if end < 0 {
			end = len(rest)
		}
		if end == 0 {
			return nil, bad
		}
		steps = append(steps, step{name: rest[:end]})
		rest = rest[end:]
		for strings.HasPrefix(rest, "[") {
			digits, after, closed := strings.Cut(rest[1:], "]")
			n, err := strconv.Atoi(digits)
			if !closed || err != nil || strings.Trim(digits, "0123456789") != "" {
				return nil, bad
			}
			steps = append(steps, step{item: n, inList: true})
			rest = after
		}
		if rest == "" {
			return steps, nil
		}
		if rest[0] != '.' {
			return nil, bad
		}
		rest = rest[1:]
	}
}

// placeIn is the place of a name in the map at a place, or at the top.
func placeIn(at, name string) string {
	if at == "" {
		return name
	}
	return at + "." + name
}

// compareSteps orders places so that the items of a list come in order.
func compareSteps(a, b []step) int {
	for i := range min(len(a), len(b)) {
		if c := cmp.Or(cmp.Compare(a[i].name, b[i].name), cmp.Compare(a[i].item, b[i].item)); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}

// WithFiles gives an input with each file at its place, such as report or
// files[0].file. A map or a list the place goes through is made where the
// input has none, and an item of a list goes at its end or at an item that
// is null. The input is a map of names, or nothing, and a place that holds
// a value other than null is refused.
func WithFiles(input any, files map[string]InputFile) (any, error) {
	if len(files) == 0 {
		return input, nil
	}
	if input == nil {
		input = map[string]any{}
	}
	if _, ok := input.(map[string]any); !ok {
		return nil, fmt.Errorf("%w: a file is given as input at a place, and the input is not a map of names", ErrStart)
	}
	type placed struct {
		at    string
		steps []step
	}
	var all []placed
	for at := range files {
		steps, err := placeOf(at)
		if err != nil {
			return nil, err
		}
		all = append(all, placed{at, steps})
	}
	slices.SortFunc(all, func(a, b placed) int { return compareSteps(a.steps, b.steps) })
	for _, p := range all {
		var err error
		if input, err = put(input, "", p.steps, p.at, files[p.at]); err != nil {
			return nil, err
		}
	}
	return input, nil
}

// put gives a copy of v with the file at the steps below it, leaving v as
// it was. at is where v is in the input, and place is the file's whole
// place, for a refusal.
func put(v any, at string, steps []step, place string, file InputFile) (any, error) {
	if len(steps) == 0 {
		if v != nil {
			return nil, fmt.Errorf("%w: the input has a value at %q, and a file is given there too", ErrStart, place)
		}
		return file, nil
	}
	s := steps[0]
	if v == nil && s.inList {
		v = []any{}
	} else if v == nil {
		v = map[string]any{}
	}
	switch v := v.(type) {
	case map[string]any:
		if s.inList {
			return nil, fmt.Errorf("%w: a file is given at %q, and the input at %q is not a list", ErrStart, place, at)
		}
		inner, err := put(v[s.name], placeIn(at, s.name), steps[1:], place, file)
		if err != nil {
			return nil, err
		}
		out := maps.Clone(v)
		out[s.name] = inner
		return out, nil
	case []any:
		if !s.inList {
			return nil, fmt.Errorf("%w: a file is given at %q, and the input at %q is not a map", ErrStart, place, at)
		}
		if s.item > len(v) {
			return nil, fmt.Errorf("%w: a file is given at %q, and nothing is at %q", ErrStart, place, at+"["+strconv.Itoa(len(v))+"]")
		}
		out := slices.Clone(v)
		if s.item == len(v) {
			out = append(out, nil)
		}
		inner, err := put(out[s.item], at+"["+strconv.Itoa(s.item)+"]", steps[1:], place, file)
		if err != nil {
			return nil, err
		}
		out[s.item] = inner
		return out, nil
	}
	return nil, fmt.Errorf("%w: a file is given at %q, and the input at %q is not a map or a list", ErrStart, place, at)
}
