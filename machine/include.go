package machine

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
)

// includeField is the field of a mapping that names files whose fields it
// takes. Every mapping read by a shape has it, and no shape names it.
const includeField = "include"

// mostIncludes is how many includes a definition and what is below it may
// write out, a file reached by two routes counting each time. A file that
// includes itself would never end, and files that include one another twice
// over grow faster than anyone could write.
const mostIncludes = 1000

// A node is one value of a definition as a file writes it, with the file and
// the line it is written at. A mapping keeps its names in the order written.
type node struct {
	keys  []string
	m     map[string]*node // a mapping's values; nil for anything else
	items []*node
	list  bool
	value any // text, a number, true or false, or nil
	file  string
	line  int
	// chain is every file being read where the value was written out,
	// outermost first.
	chain []string
}

func (n *node) isMap() bool { return n != nil && n.m != nil }

// set gives a mapping a value under a name, after those it has.
func (n *node) set(name string, v *node) {
	if _, ok := n.m[name]; !ok {
		n.keys = append(n.keys, name)
	}
	n.m[name] = v
}

// plain gives a node as the plain values a definition is read as.
func plain(n *node) any {
	switch {
	case n == nil:
		return nil
	case n.isMap():
		m := make(map[string]any, len(n.keys))
		for _, k := range n.keys {
			m[k] = plain(n.m[k])
		}
		return m
	case n.list:
		list := make([]any, len(n.items))
		for i, item := range n.items {
			list[i] = plain(item)
		}
		return list
	}
	return n.value
}

// at says where a node is written: its file and line, or the line alone in
// a definition read with no file name.
func (n *node) at() string {
	if n.file == "" {
		return strconv.Itoa(n.line)
	}
	return fmt.Sprintf("%s:%d", n.file, n.line)
}

// asWritten gives a node and what it holds as written, read where chain is.
func asWritten(n *node, chain []string) *node {
	out := *n
	out.chain = chain
	if n.isMap() {
		out.keys, out.m = nil, map[string]*node{}
		for _, k := range n.keys {
			out.set(k, asWritten(n.m[k], chain))
		}
	}
	if n.list {
		out.items = make([]*node, len(n.items))
		for i, item := range n.items {
			out.items[i] = asWritten(item, chain)
		}
	}
	return &out
}

// merged gives two mappings merged at every depth. Where both hold a name
// and the two values are not both mappings, the winner's value is taken
// whole: first's if firstWins, then. The names keep the order they are met
// in, first's before second's.
func merged(first, second *node, firstWins bool) *node {
	switch {
	case first == nil:
		return second
	case second == nil:
		return first
	case !first.isMap() || !second.isMap():
		if firstWins {
			return first
		}
		return second
	}
	out := *first
	if !firstWins {
		out.file, out.line, out.chain = second.file, second.line, second.chain
	}
	out.keys, out.m = nil, map[string]*node{}
	for _, k := range first.keys {
		out.set(k, merged(first.m[k], second.m[k], firstWins))
	}
	for _, k := range second.keys {
		if _, ok := first.m[k]; !ok {
			out.set(k, second.m[k])
		}
	}
	return &out
}

// data says a field holds data, which is kept as it is written: no include:
// in it is read and no path in it moves.
func data(f *Field) bool {
	return f.Kind == KindTypes || (f.Kind == KindValue || f.Kind == KindFields) && f.Of == nil && !f.Path
}

// out writes out a value as a field takes it: what it includes put in its
// place, and each path in a file it includes given from the context. A
// field no shape names is kept as it is written, for the reader to refuse.
func (p *Parser) out(n *node, field *Field, place string, chain []string) *node {
	if n == nil {
		return nil
	}
	if field == nil || data(field) {
		return asWritten(n, chain)
	}
	switch {
	case field.Kind == KindText:
		return p.text(n, place, chain)
	case n.list && (field.Kind == KindList || field.Many || field.Kind == KindFields || field.Kind == KindJob):
		item := *field
		item.Many = false
		if field.Kind == KindList {
			item.Kind = KindWord
		}
		return p.spliced(n, &item, place, chain)
	case n.list:
		return asWritten(n, chain)
	case n.isMap():
		switch {
		case field.Path:
			if t, ok := p.typeOf(n, chain, map[string]bool{}); ok {
				return p.mapping(n, whole(t).field, place, chain)
			}
			return asWritten(n, chain)
		case field.Kind == KindNamed:
			entry := field.entry()
			return p.mapping(n, func(string) *Field { return entry }, place, chain)
		case field.Kind == KindList:
			return p.mapping(n, func(string) *Field { return &Field{Kind: KindList} }, place, chain)
		case field.Holds() != nil:
			return p.mapping(n, field.Holds().field, place, chain)
		}
		return asWritten(n, chain)
	}
	out := asWritten(n, chain)
	if text, ok := n.value.(string); ok && (field.Path || field.Kind == KindJob) {
		out.value = p.moved(n, text)
	}
	return out
}

// moved gives a path written in a file as the definition reads it: one in
// a file the definition includes, from that file's directory, is given from
// the context.
func (p *Parser) moved(n *node, name string) string {
	if n.file != p.source && fromFile(name) && inContext(name) && !HasExpr(name) {
		return path.Join(path.Dir(n.file), name)
	}
	return name
}

// mapping writes out a mapping, with the fields of the files its include:
// names merged in at every depth. What the mapping writes wins over what a
// file gives, a file's own over what it includes, and an earlier file's over
// a later one's. held gives the field of a name.
func (p *Parser) mapping(n *node, held func(string) *Field, place string, chain []string) *node {
	before := &node{m: map[string]*node{}, file: n.file, line: n.line, chain: chain}
	after := &node{m: map[string]*node{}, file: n.file, line: n.line, chain: chain}
	var given *node
	own := before
	for _, k := range n.keys {
		if k == includeField {
			given = p.included(n.m[k], held, place, chain)
			own = after
			continue
		}
		own.set(k, p.out(n.m[k], held(k), placeOf(place, k), chain))
	}
	return merged(merged(before, given, true), after, false)
}

// included gives the fields of the files an include: in the mapping at a
// place names, one or a list, merged in order.
func (p *Parser) included(written *node, held func(string) *Field, mapPlace string, chain []string) *node {
	place := placeOf(mapPlace, includeField)
	names := []*node{written}
	if written.list {
		names = written.items
	}
	var all *node
	for _, name := range names {
		text, ok := name.value.(string)
		if !ok || name.isMap() || name.list {
			p.problemIn(name.file, place, "include: names a file or a list of them, and this is %s", describe(plain(name)))
			continue
		}
		v, inner := p.include(name, text, place, chain)
		switch {
		case v == nil:
			continue
		case !v.isMap():
			p.problemIn(name.file, place, "%q is %s, and only a mapping is merged into a mapping", text, describe(plain(v)))
			continue
		}
		all = merged(all, p.mapping(v, held, mapPlace, inner), true)
	}
	return all
}

// spliced writes out a list, with each item that is { include: <file> }
// replaced by what the file gives: a list's items, or one item.
func (p *Parser) spliced(n *node, item *Field, place string, chain []string) *node {
	out := &node{list: true, items: []*node{}, file: n.file, line: n.line, chain: chain}
	var add func(list *node, chain []string)
	add = func(list *node, chain []string) {
		for _, it := range list.items {
			at := fmt.Sprintf("%s[%d]", place, len(out.items))
			name, ok := includes(it)
			if !ok {
				out.items = append(out.items, p.out(it, item, at, chain))
				continue
			}
			v, inner := p.include(it.m[includeField], name, placeOf(at, includeField), chain)
			switch {
			case v == nil:
			case v.list:
				add(v, inner)
			default:
				out.items = append(out.items, p.out(v, item, at, inner))
			}
		}
	}
	add(n, chain)
	return out
}

// includes gives the name of the file an item that is { include: <file> }
// names.
func includes(n *node) (string, bool) {
	if !n.isMap() || len(n.keys) != 1 || n.keys[0] != includeField {
		return "", false
	}
	name, ok := n.m[includeField].value.(string)
	return name, ok && !n.m[includeField].list && !n.m[includeField].isMap()
}

// text writes out a text: one text, a list of pieces joined by one empty
// line, or { include: <file> }, a text of one piece. A piece is a text or
// { include: <file> }, which gives the file's text, or
// { include: <file>#<path> }, which gives the text at that path. Each
// piece's empty lines at its start and end are left out, so pieces are
// joined by exactly one.
func (p *Parser) text(n *node, place string, chain []string) *node {
	if !n.list && !n.isMap() {
		return asWritten(n, chain)
	}
	pieces := []*node{n}
	if n.list {
		pieces = n.items
	}
	var joined []string
	var first *node
	for i, piece := range pieces {
		at := place
		if n.list {
			at = fmt.Sprintf("%s[%d]", place, i)
		}
		if name, ok := includes(piece); ok {
			piece = p.includeText(piece.m[includeField], name, placeOf(at, includeField), chain)
		} else if _, ok := piece.value.(string); !ok || piece.list || piece.isMap() {
			p.problemIn(piece.file, at, "a piece of a text is text or { include: <file> }, and this is %s", describe(plain(piece)))
			piece = nil
		}
		if piece == nil {
			continue
		}
		if first == nil {
			first = piece
		}
		if text := strings.Trim(piece.value.(string), "\n"); text != "" {
			joined = append(joined, text)
		}
	}
	if first == nil {
		return asWritten(n, chain)
	}
	out := asWritten(first, chain)
	out.value = strings.Join(joined, "\n\n")
	return out
}

// includeText reads the text an include: in a text names.
func (p *Parser) includeText(n *node, name, place string, chain []string) *node {
	file, at, hasPath := strings.Cut(name, "#")
	if !hasPath {
		full, ok := p.including(n, file, place, chain)
		if !ok {
			return nil
		}
		src, ok := p.readFile(n.file, place, file, full)
		if !ok {
			return nil
		}
		return &node{value: string(src), file: full, line: 1}
	}
	v, _ := p.include(n, name, place, chain)
	if v == nil {
		return nil
	}
	if _, ok := v.value.(string); !ok || v.list || v.isMap() {
		p.problemIn(n.file, place, "%q#%s is %s, and a piece of a text is text", file, at, describe(plain(v)))
		return nil
	}
	return asWritten(v, chain)
}

// including checks the name of a file to include, and gives its path from
// the context. A file that is already being read, or one past the most a
// definition may include, is refused.
func (p *Parser) including(n *node, file, place string, chain []string) (string, bool) {
	moved := p.moved(n, file)
	full := kept(p.cleanPath(moved))
	switch {
	case !inContext(file):
		p.problemIn(n.file, place, "%q: %s", file, outside)
		return "", false
	case slices.Contains(chain, full):
		p.problemIn(n.file, place, "the definition leads back to itself: %s -> %s", strings.Join(chain, " -> "), full)
		return "", false
	}
	if *p.includes++; *p.includes > mostIncludes {
		p.problemIn(n.file, place, "more than %d includes are written out: a file reached by two routes counts each time", mostIncludes)
		return "", false
	}
	return full, true
}

// include reads what an include: names, as YAML: a whole file, or the
// value at a path in it, with the files being read where it is.
func (p *Parser) include(n *node, name, place string, chain []string) (*node, []string) {
	file, at, _ := strings.Cut(name, "#")
	full, ok := p.including(n, file, place, chain)
	if !ok {
		return nil, nil
	}
	v, ok := p.readNode(n.file, place, file, full)
	if !ok {
		return nil, nil
	}
	if at != "" {
		for _, part := range strings.Split(at, ".") {
			if !v.isMap() || v.m[part] == nil {
				p.problemIn(n.file, place, "%q has nothing at %s", file, at)
				return nil, nil
			}
			v = v.m[part]
		}
	}
	return v, append(slices.Clip(chain), full)
}

// A read is a file a definition includes, read once however often it is
// included.
type read struct {
	src  []byte
	node *node
	err  error
}

// readFile reads a file a definition includes, by its path from the
// context.
func (p *Parser) readFile(from, place, name, full string) ([]byte, bool) {
	if r, ok := p.reads[full]; ok {
		return r.src, true
	}
	src, err := p.load(full)
	if err != nil {
		p.problemIn(from, place, "%q cannot be read: %v", name, err)
		return nil, false
	}
	p.reads[full] = &read{src: src}
	return src, true
}

// readNode reads a file a definition includes as YAML.
func (p *Parser) readNode(from, place, name, full string) (*node, bool) {
	if _, ok := p.readFile(from, place, name, full); !ok {
		return nil, false
	}
	r := p.reads[full]
	if r.node == nil && r.err == nil {
		r.node, _, r.err = nodeYAML(r.src, full)
		if r.err == nil && r.node == nil {
			r.err = fmt.Errorf("it is empty")
		}
	}
	if r.err != nil {
		p.problemIn(from, place, "%q cannot be read: %v", name, r.err)
		return nil, false
	}
	return r.node, true
}

// typeOf gives the type of a machine written in place, which it writes
// itself or takes from a file it includes.
func (p *Parser) typeOf(n *node, chain []string, seen map[string]bool) (Type, bool) {
	for _, k := range n.keys {
		if t, ok := TypeOf(k); ok {
			return t, true
		}
	}
	inc := n.m[includeField]
	if inc == nil {
		return nil, false
	}
	names := []*node{inc}
	if inc.list {
		names = inc.items
	}
	for _, name := range names {
		text, ok := name.value.(string)
		file, at, _ := strings.Cut(text, "#")
		full := kept(p.cleanPath(p.moved(name, file)))
		if !ok || !inContext(file) || seen[full] || slices.Contains(chain, full) {
			continue
		}
		seen[full] = true
		src, err := p.load(full)
		if err != nil {
			continue
		}
		v, _, err := nodeYAML(src, full)
		for _, part := range strings.Split(at, ".") {
			if at != "" && v.isMap() {
				v = v.m[part]
			}
		}
		if err == nil && v.isMap() {
			if t, ok := p.typeOf(v, chain, seen); ok {
				return t, true
			}
		}
	}
	return nil, false
}

// index notes the place of every value written out, and of what it holds.
func (p *Parser) index(n *node, place string) {
	if n == nil {
		return
	}
	p.at[place] = n
	for _, k := range n.keys {
		p.index(n.m[k], placeOf(place, k))
	}
	for i, item := range n.items {
		p.index(item, fmt.Sprintf("%s[%d]", place, i))
	}
}

// below gives the places noted below a place, as places from it.
func below(at map[string]*node, place string) map[string]*node {
	out := map[string]*node{}
	for k, n := range at {
		if k == place {
			out[""] = n
		} else if rest, ok := strings.CutPrefix(k, place+"."); ok {
			out[rest] = n
		}
	}
	return out
}

// up gives the place that holds a place: the mapping or the list it is in.
func up(place string) string { return place[:max(strings.LastIndexAny(place, ".["), 0)] }

// written gives the value written out nearest a place: at it, or at what
// holds it.
func (p *Parser) written(place string) *node {
	for {
		if n := p.at[place]; n != nil {
			return n
		}
		if place == "" {
			return nil
		}
		place = up(place)
	}
}

// fileAt gives the file a place is written in, as a problem names it: the
// definition's own file is not named.
func (p *Parser) fileAt(place string) string {
	file := p.source
	if n := p.written(place); n != nil {
		file = n.file
	}
	if file == p.top {
		return ""
	}
	return file
}

// reading gives every file being read at a place, outermost first.
func (p *Parser) reading(place string) []string {
	if n := p.written(place); n != nil && n.chain != nil {
		return n.chain
	}
	return p.chain
}

// placeOf gives the place of a field of the mapping at a place.
func placeOf(place, name string) string {
	if place == "" {
		return name
	}
	return place + "." + name
}

// whole is the shape of a definition with its type's own fields.
func whole(t Type) *Shape {
	return &Shape{Name: "definition", What: fmt.Sprintf("a %s", t.Name()),
		Fields: append(definitionShape(t).Fields, t.Shape().Fields...)}
}
