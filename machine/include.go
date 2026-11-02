package machine

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
)

// includeField is the field of a mapping that names files whose fields it
// takes. Every mapping read by a shape has it, and no shape names it.
const includeField = "include"

// mostIncludes is how many files a definition and what is below it may
// include. A file that includes itself would never end, and files that
// include one another twice over grow faster than anyone could write.
const mostIncludes = 1000

// including is a file an include: names, with the files that lead to it.
type including struct {
	name  string
	chain []string
}

// included gives a mapping with the fields of the files its include: names.
// What the mapping writes stands before what a file gives, a file's own
// fields before those of the files it includes, and an earlier file's before
// a later one's. held gives the field of a name, which says where the paths
// of what a file gives are.
func (p *Parser) included(place string, m map[string]any, held func(string) *Field) map[string]any {
	written, ok := m[includeField]
	if !ok {
		return m
	}
	all := maps.Clone(m)
	delete(all, includeField)
	at := placeOf(place, includeField)
	queue := p.includeNames(at, written, p.reading(place))
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		file := p.include(at, next, held)
		full := kept(p.cleanPath(next.name))
		if inner, ok := file[includeField]; ok {
			queue = append(p.includeNames(at, inner, append(slices.Clip(next.chain), full)), queue...)
			delete(file, includeField)
		}
		for key, value := range file {
			if _, own := all[key]; !own {
				all[key] = value
				p.keepFrom(placeOf(place, key), full)
			}
		}
	}
	return all
}

// includeNames gives the files an include: names: one, or a list.
func (p *Parser) includeNames(place string, written any, chain []string) []including {
	list, ok := written.([]any)
	if !ok {
		list = []any{written}
	}
	var names []including
	for _, item := range list {
		name, ok := item.(string)
		if !ok {
			p.Problem(place, "include: names a file or a list of them, and this is %s", describe(item))
			continue
		}
		names = append(names, including{name, chain})
	}
	return names
}

// include reads one file an include: names, with its paths given from the
// context.
func (p *Parser) include(place string, next including, held func(string) *Field) map[string]any {
	full := kept(p.cleanPath(next.name))
	switch {
	case !inContext(next.name):
		p.Problem(place, "%q: %s", next.name, outside)
		return nil
	case slices.Contains(next.chain, full):
		p.Problem(place, "the definition leads back to itself: %s -> %s", strings.Join(next.chain, " -> "), full)
		return nil
	}
	if *p.includes++; *p.includes > mostIncludes {
		p.Problem(place, "more than %d files are included", mostIncludes)
		return nil
	}
	src, ok := p.read(place, next.name, full)
	if !ok {
		return nil
	}
	written, err := readMapping(src)
	if err != nil {
		p.Problem(place, "%q cannot be read: %v", next.name, err)
		return nil
	}
	return movedFields(path.Dir(full), written, held)
}

// keepFrom notes that what is at a place came from a file, so that a problem
// there names that file.
func (p *Parser) keepFrom(place, file string) {
	if p.from == nil {
		p.from = map[string]string{}
	}
	p.from[place] = file
}

// above gives the place of the mapping that holds a place.
func above(place string) string { return place[:max(strings.LastIndexByte(place, '.'), 0)] }

// fileAt gives the file a place is written in.
func (p *Parser) fileAt(place string) string {
	for ; place != ""; place = above(place) {
		if file, ok := p.from[place]; ok {
			return file
		}
	}
	return p.source
}

// reading gives every file being read at a place, outermost first.
func (p *Parser) reading(place string) []string {
	var files []string
	for ; place != ""; place = above(place) {
		if file, ok := p.from[place]; ok {
			files = append(files, file)
		}
	}
	slices.Reverse(files)
	return append(slices.Clip(p.chain), files...)
}

// placeOf gives the place of a field of the mapping at a place.
func placeOf(place, name string) string {
	if place == "" {
		return name
	}
	return place + "." + name
}

// movedFields gives a mapping of a file in dir with each path written from
// that file given from the context. Only fields a shape gives as paths are paths: a value
// under input:, whatever it looks like, stays as it is written.
func movedFields(dir string, m map[string]any, held func(string) *Field) map[string]any {
	out := make(map[string]any, len(m))
	for name, value := range m {
		if name == includeField {
			out[name] = moved(dir, value, &Field{Path: true})
			continue
		}
		out[name] = moved(dir, value, held(name))
	}
	return out
}

// moved gives what a field holds, as movedFields does.
func moved(dir string, v any, field *Field) any {
	if field == nil {
		return v
	}
	switch v := v.(type) {
	case string:
		if (field.Path || field.Kind == KindJob) && fromFile(v) && inContext(v) && !HasExpr(v) {
			return path.Join(dir, v)
		}
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = moved(dir, item, field)
		}
		return out
	case map[string]any:
		switch {
		case field.Path:
			return movedMachine(dir, v)
		case field.Kind == KindNamed:
			entry := field.entry()
			return movedFields(dir, v, func(string) *Field { return entry })
		case field.Holds() != nil:
			return movedFields(dir, v, field.Holds().field)
		}
	}
	return v
}

// movedMachine gives a machine written in place, as movedFields does.
func movedMachine(dir string, m map[string]any) map[string]any {
	for _, kind := range Types() {
		if _, ok := m[kind]; ok {
			t, _ := TypeOf(kind)
			return movedFields(dir, m, whole(t).field)
		}
	}
	return m
}

// whole is the shape of a definition with its type's own fields.
func whole(t Type) *Shape {
	return &Shape{Name: "definition", What: fmt.Sprintf("a %s", t.Name()),
		Fields: append(definitionShape(t).Fields, t.Shape().Fields...)}
}
