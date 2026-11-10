// Package permission is how what a job may do is written, layered and
// checked.
package permission

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/zummed/corbie/store"
)

// A Layer is one hand's say over what a job may do: what its deny list
// matches is refused, what its allow list matches passes, and anything else
// is refused. Name says whose layer it is, for a refusal to name, and Fix
// says how an entry is added to it.
type Layer struct {
	Name  string   `json:"name,omitzero"`
	Allow []string `json:"allow,omitzero"`
	Deny  []string `json:"deny,omitzero"`
	// Fix is what would add an entry to the layer, with %s where the entry
	// goes: a place to write a line, or a command to run.
	Fix string `json:"fix,omitzero"`
}

// The eight kinds of permission.
const (
	Read   = "fs.read"
	Write  = "fs.write"
	Net    = "net"
	Exec   = "exec"
	Env    = "env"
	Secret = "secret"
	Send   = "send"
	At     = "at"
)

// Unencrypted is a destination of a secret and not a kind: a connection to
// a host, whether it is encrypted or not.
const Unencrypted = "net.unencrypted"

var kinds = map[string]bool{Read: true, Write: true, Net: true, Exec: true, Env: true, Secret: true, Send: true, At: true}

// The groups, each standing for every scope of its kinds. "all" allows
// everything, secrets, send and at included, and means the same in every
// layer. send and at are in no other group.
var groups = map[string][]string{
	"network": {Net}, "disk": {Read, Write}, "exec": {Exec}, "env": {Env},
	"all": {Read, Write, Net, Exec, Env, Secret, Send, At},
}

// CheckWritten says what is wrong with an entry as it is written in a list
// of a definition: a group, or a kind and a scope. The kind is written out;
// the scope may still hold an expression to be filled in. An at entry is
// written in a client's ceiling only (see CheckCeiling).
func CheckWritten(entry string) error {
	if kind, _, _ := strings.Cut(entry, ":"); kind == At {
		return fmt.Errorf("%q: an at entry is written in a client's ceiling only", entry)
	}
	return readable(entry)
}

// readable says what is wrong with an entry wherever it is written.
func readable(entry string) error {
	if _, ok := groups[entry]; ok {
		return nil
	}
	kind, scope, ok := strings.Cut(entry, ":")
	if !ok || !kinds[kind] {
		return fmt.Errorf("%q is not a permission: write a kind and a scope, such as net:api.github.com (the kinds are fs.read, fs.write, net, exec, env, secret, send and at; the groups are network, disk, exec, env and all)", entry)
	}
	if scope == "" {
		return fmt.Errorf("%q has no scope", entry)
	}
	if kind == At && !strings.HasPrefix(scope, "/") {
		return fmt.Errorf("%q: an at entry is a prefix starting with /, as in at:/home", entry)
	}
	if kind == Send || kind == At {
		// An expression is filled in later, and the parts around it are
		// checked now.
		if err := checkLocation(withoutExpressions(scope)); err != nil {
			return fmt.Errorf("%q: %w", entry, err)
		}
	}
	if kind == Net && !strings.Contains(scope, "${") && (strings.Contains(scope, "://") || strings.ContainsAny(scope, "/?#@")) {
		return fmt.Errorf("%q: a net scope is a host alone, with a port if it matters, as in net:api.github.com or net:db.internal:5432; leave out the scheme and the path", entry)
	}
	if kind != Secret {
		return nil
	}
	name, destination, ok := strings.Cut(scope, "@")
	if !ok || name == "" || destination == "" {
		return fmt.Errorf("%q: a secret's permission says where it may go, as in secret:NAME@net:host or secret:NAME@*", entry)
	}
	if strings.Contains(name, "${") {
		return fmt.Errorf("%q: a secret's name is written out", entry)
	}
	if !secretName(name, true) {
		return fmt.Errorf("%q: a secret's name holds no @, :, =, $, {, }, slash, backslash or blank, and does not start with - or .; a * stands for any part of a name", entry)
	}
	if destination == "*" {
		return nil
	}
	switch to, rest, _ := strings.Cut(destination, ":"); to {
	case Net, Exec, Write:
		return readable(destination)
	case Unencrypted:
		return readable(Net + ":" + rest)
	}
	return fmt.Errorf("%q: a secret may go to net:host, net.unencrypted:host, exec:program, fs.write:path or *", entry)
}

// CheckSecretName says whether a secret may have the name. An entry is read
// as the name up to the first @ and the destination after it, so a name that
// could hold : or a blank would leave more than one way to read it. The name
// is a part of a name in the store, and a command line reads a word starting
// with - as a flag or as standard input.
func CheckSecretName(name string) error {
	if !secretName(name, false) {
		return fmt.Errorf("%q is not a name a secret can have: a name holds no @, :, =, *, $, {, }, slash, backslash or blank, and does not start with - or .", name)
	}
	return nil
}

// secretName says whether name can be a secret's name or, with pattern, a
// pattern in which * stands for any part of a name.
func secretName(name string, pattern bool) bool {
	refused := "@:=*${}/\\"
	if pattern {
		refused = "@:=${}/\\"
	}
	blank := func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }
	return name != "" && name[0] != '-' && name[0] != '.' && !strings.ContainsAny(name, refused) && !strings.ContainsFunc(name, blank)
}

// CheckList says what is wrong with a list of entries written out in full,
// as a runner's ceiling or a start's list is.
func CheckList(entries []string) error {
	return checkList(entries, CheckWritten)
}

// CheckCeiling says what is wrong with a client's ceiling, the one list in
// which at entries are written.
func CheckCeiling(entries []string) error {
	return checkList(entries, readable)
}

func checkList(entries []string, check func(string) error) error {
	for _, entry := range entries {
		if strings.Contains(entry, "${") {
			return fmt.Errorf("%q: an entry here is written out in full, with no expression", entry)
		}
		if err := check(entry); err != nil {
			return err
		}
		if kind, scope, _ := strings.Cut(entry, ":"); kind == Send && !strings.HasPrefix(scope, "/") {
			return fmt.Errorf("%q: a location here is a full path, starting with /", entry)
		}
	}
	return nil
}

// checkLocation says what is wrong with a location or a pattern of
// locations, whole or read from a tree's prefix: each part is one the store
// takes.
func checkLocation(written string) error {
	for _, part := range strings.Split(strings.TrimPrefix(written, "/"), "/") {
		if err := store.CheckLocationPart(part); err != nil {
			return fmt.Errorf("a location has %w, and does not end with /", err)
		}
	}
	return nil
}

// checkFull says what is wrong with a location in full, as a tree's prefix
// or a place sent to is: "/", or a location starting with / that holds no
// *, so that it is never a pattern.
func checkFull(location string) error {
	switch {
	case location == "/":
		return nil
	case !strings.HasPrefix(location, "/"):
		return fmt.Errorf("%q: a location in full starts with /", location)
	case strings.Contains(location, "*"):
		return fmt.Errorf("%q: a location holds no *", location)
	}
	if err := checkLocation(location); err != nil {
		return fmt.Errorf("%q: %w", location, err)
	}
	return nil
}

// withoutExpressions gives a scope with each ${...} in it put as one
// letter, so that the parts around an expression can be checked.
func withoutExpressions(scope string) string {
	var b strings.Builder
	for {
		start := strings.Index(scope, "${")
		if start < 0 {
			return b.String() + scope
		}
		b.WriteString(scope[:start] + "x")
		depth, end := 0, len(scope)
		for i := start + 1; i < len(scope); i++ {
			if scope[i] == '{' {
				depth++
			} else if scope[i] == '}' {
				if depth--; depth == 0 {
					end = i + 1
					break
				}
			}
		}
		scope = scope[end:]
	}
}

// ReadLocation gives a location, or a pattern of locations, as it is
// written in a definition, in full: read from the tree's prefix, or as it
// is when it starts with /.
func ReadLocation(prefix, written string) (string, error) {
	if err := checkLocation(written); err != nil {
		return "", fmt.Errorf("%q: %w", written, err)
	}
	if prefix != "" {
		if err := checkFull(prefix); err != nil {
			return "", fmt.Errorf("the tree's prefix %w", err)
		}
	}
	switch {
	case strings.HasPrefix(written, "/"):
		return written, nil
	case prefix == "":
		return "", fmt.Errorf("%q is read from a tree's prefix, and there is none here: write the location in full, starting with /", written)
	}
	return strings.TrimSuffix(prefix, "/") + "/" + written, nil
}

// MatchLocation says whether a pattern of locations, in full, matches a
// location in full: * stands for any characters in one part, and ** for
// any number of parts. A location is a name in the store and never a path
// on a disk: no link is followed and case always counts. What is not a
// location in full matches nothing.
func MatchLocation(pattern, location string) bool {
	return checkFull(location) == nil && matchLocation(pattern, location)
}

// matchLocation matches a pattern of locations against a location, or
// against another pattern as if it were one.
func matchLocation(pattern, location string) bool {
	if !strings.HasPrefix(pattern, "/") || !strings.HasPrefix(location, "/") {
		return false
	}
	return matchParts(locationParts(pattern), locationParts(location))
}

func locationParts(location string) []string {
	if location == "/" {
		return nil
	}
	return strings.Split(location[1:], "/")
}

// A Refusal says three things: what was tried, which layer refused it, and
// what would fix it.
type Refusal struct {
	What  string // the kind and the value that was checked, as an entry
	Layer string // every layer that refused
	Fix   string
}

func (r *Refusal) Error() string {
	return fmt.Sprintf("%s was refused by %s. To allow it: %s", r.What, r.Layer, r.Fix)
}

// A Set is every layer that applies to one job, each apart under its name.
// It notes what the job used.
type Set struct {
	Layers []Layer
	// Scratch is the job's scratch directory: a path scope with no leading
	// slash names a place in it.
	Scratch string
	// Prefix is the prefix of the run's tree: a location scope with no
	// leading slash is read from it. With none, such a scope matches
	// nothing.
	Prefix string

	mu     sync.Mutex
	used   []string
	scopes map[string]pathScope // each path scope, resolved once
}

// Used lists the permissions the job used.
func (s *Set) Used() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.used)
}

func (s *Set) note(entry string) {
	s.mu.Lock()
	if !slices.Contains(s.used, entry) {
		s.used = append(s.used, entry)
	}
	s.mu.Unlock()
}

// Check says whether every layer passes one thing a job tries: a kind and
// the value checked. For a connection, give every form the destination has
// with Net; a path is given as it really is, links followed.
func (s *Set) Check(kind string, values ...string) *Refusal {
	what := kind + ":" + s.written(kind, values[0])
	var refused []string
	var fixes []string
	for _, layer := range s.Layers {
		passes, denied := s.passes(layer, kind, values)
		if passes {
			continue
		}
		refused = append(refused, layer.Name)
		switch {
		case denied != "":
			fixes = append(fixes, fmt.Sprintf("take the deny entry %s out of %s", denied, layer.Name))
		case layer.Fix != "":
			fixes = append(fixes, fmt.Sprintf(layer.Fix, what))
		default:
			fixes = append(fixes, fmt.Sprintf("add the line %s to %s", what, layer.Name))
		}
	}
	if len(refused) == 0 {
		s.note(what)
		return nil
	}
	return &Refusal{What: what, Layer: strings.Join(refused, " and by "), Fix: strings.Join(fixes, "; and ")}
}

// written gives a value that was checked as an entry writes it. A path in
// the job's scratch directory is written with no leading slash, as a scope
// names a place there: the directory has another name for every job, and an
// entry that held it would allow nothing in the next. A program there keeps
// a slash, which is what tells its path from a program's name.
func (s *Set) written(kind, value string) string {
	switch kind {
	case Secret:
		name, to, _ := strings.Cut(value, "@")
		if toKind, toValue, ok := strings.Cut(to, ":"); ok {
			return name + "@" + toKind + ":" + s.written(toKind, toValue)
		}
		return value
	case Read, Write, Exec:
	default:
		return value
	}
	if s.Scratch == "" {
		return value
	}
	scratch := strings.Join(s.pathScope(".").fixed, "/")
	if fold(value) == fold(scratch) {
		return "."
	}
	if len(value) <= len(scratch) || fold(value[:len(scratch)+1]) != fold(scratch+"/") {
		return value
	}
	inside := value[len(scratch)+1:]
	if kind == Exec && !strings.Contains(inside, "/") {
		return "./" + inside
	}
	return inside
}

// CheckGroup says whether every layer allows the whole of a group, for
// something that cannot be held to a scope: a layer passes it only by
// allowing the group, or all, and denying nothing of the group's kinds.
func (s *Set) CheckGroup(group string) *Refusal {
	var refused, fixes []string
	for _, layer := range s.Layers {
		passes := false
		for _, entry := range layer.Allow {
			passes = passes || entry == group || entry == "all"
		}
		for _, entry := range layer.Deny {
			kind, _, _ := strings.Cut(entry, ":")
			if slices.Contains(groups[group], kind) || readable(entry) != nil {
				passes = false
			}
			for _, in := range groups[entry] {
				passes = passes && !slices.Contains(groups[group], in)
			}
		}
		if passes {
			continue
		}
		refused = append(refused, layer.Name)
		if layer.Fix != "" {
			fixes = append(fixes, fmt.Sprintf(layer.Fix, group))
		} else {
			fixes = append(fixes, fmt.Sprintf("add the line %s to %s", group, layer.Name))
		}
	}
	if len(refused) == 0 {
		s.note(group)
		return nil
	}
	return &Refusal{What: group, Layer: strings.Join(refused, " and by "), Fix: strings.Join(fixes, "; and ")}
}

// passes says whether one layer passes the thing tried; if a deny entry is
// what refused it, that entry is given.
func (s *Set) passes(layer Layer, kind string, values []string) (bool, string) {
	for _, entry := range layer.Deny {
		if !s.readsDeny(entry) {
			return false, entry
		}
		for _, value := range values {
			if s.matches(entry, kind, value) {
				return false, entry
			}
		}
	}
	for _, entry := range layer.Allow {
		for _, value := range values {
			if s.matches(entry, kind, value) {
				return true, ""
			}
		}
	}
	return false, ""
}

// readsDeny says whether a deny entry can be read. One that cannot refuses
// everything in its layer: it could have been meant to deny anything.
func (s *Set) readsDeny(entry string) bool {
	if readable(entry) != nil || strings.Contains(entry, "${") {
		return false
	}
	if kind, scope, _ := strings.Cut(entry, ":"); kind == Send || kind == At {
		_, err := ReadLocation(s.Prefix, scope)
		return err == nil
	}
	return true
}

// matches says whether an entry covers a kind and a value.
func (s *Set) matches(entry, kind, value string) bool {
	if in, ok := groups[entry]; ok {
		return slices.Contains(in, kind)
	}
	entryKind, scope, ok := strings.Cut(entry, ":")
	if !ok || entryKind != kind {
		return false
	}
	switch kind {
	case Read, Write:
		return s.pathScope(scope).matches(value)
	case Exec:
		// A program's name and a program's path never pass for each other.
		scope = filepath.ToSlash(scope)
		if strings.Contains(scope, "/") != strings.Contains(value, "/") {
			return false
		}
		if strings.Contains(scope, "/") {
			return s.pathScope(scope).matches(value)
		}
		return matchPart(programName(scope), programName(value))
	case Net:
		return matchHost(scope, value)
	case Secret:
		return s.matchSecret(scope, value)
	case Send, At:
		full, err := ReadLocation(s.Prefix, scope)
		return err == nil && MatchLocation(full, value)
	}
	return matchPart(scope, value)
}

// A pathScope is a path scope ready to be matched: its fixed leading part,
// resolved through links as a value is and taken as it is, and the rest,
// from its first part with a * in it, which is a pattern.
type pathScope struct {
	fixed   []string
	pattern []string
	none    bool // the scope names no place, and matches nothing
}

// pathScope resolves a path scope, once. A scope that does not start at
// the root names a place in the scratch directory.
func (s *Set) pathScope(scope string) pathScope {
	s.mu.Lock()
	ps, ok := s.scopes[scope]
	s.mu.Unlock()
	if ok {
		return ps
	}
	full := filepath.ToSlash(scope)
	if !rooted(full) && s.Scratch != "" {
		full = path.Join(filepath.ToSlash(s.Scratch), full)
	}
	parts, ok := splitPath(full)
	if !ok {
		ps = pathScope{none: true}
		s.mu.Lock()
		if s.scopes == nil {
			s.scopes = map[string]pathScope{}
		}
		s.scopes[scope] = ps
		s.mu.Unlock()
		return ps
	}
	split := len(parts)
	for i, part := range parts {
		if strings.Contains(part, "*") {
			split = i
			break
		}
	}
	lead := strings.Join(parts[:split], "/")
	if lead == "" {
		lead = "/"
	} else if split == 1 {
		lead += "/" // a drive alone: C:/
	}
	if real, err := RealPath(filepath.FromSlash(lead)); err == nil {
		lead = filepath.ToSlash(real)
	}
	fixed, _ := splitPath(lead)
	ps = pathScope{fixed: fixed, pattern: parts[split:]}
	s.mu.Lock()
	if s.scopes == nil {
		s.scopes = map[string]pathScope{}
	}
	s.scopes[scope] = ps
	s.mu.Unlock()
	return ps
}

func (ps pathScope) matches(value string) bool {
	parts, ok := splitPath(value)
	if ps.none || !ok || len(parts) < len(ps.fixed) {
		return false
	}
	same := func(a, b string) bool { return fold(a) == fold(b) }
	if !slices.EqualFunc(parts[:len(ps.fixed)], ps.fixed, same) {
		return false
	}
	return matchParts(foldAll(ps.pattern), foldAll(parts[len(ps.fixed):]))
}

func foldAll(parts []string) []string {
	folded := make([]string, len(parts))
	for i, part := range parts {
		folded[i] = fold(part)
	}
	return folded
}

// RealPath gives where a path really points, following the links in as
// much of it as exists, and a link to a place that does not exist yet. It
// is the one way a path is resolved, for every kind that names one. A path
// that runs through a ring of links has no real location, and is an error.
func RealPath(p string) (string, error) {
	rest := ""
	for at := filepath.Clean(p); ; {
		real, err := filepath.EvalSymlinks(at)
		if err == nil {
			return filepath.Join(real, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%s has no real location: %w", p, err)
		}
		if target, err := os.Readlink(at); err == nil {
			// A dangling link: what is written to it lands at its target.
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(at), target)
			}
			at = filepath.Clean(target)
			continue
		}
		parent := filepath.Dir(at)
		if parent == at {
			return filepath.Clean(p), nil
		}
		rest = filepath.Join(filepath.Base(at), rest)
		at = parent
	}
}

// matchSecret says whether a secret's scope, NAME@destination, covers a
// secret going to a destination, itself written NAME@kind:value.
func (s *Set) matchSecret(scope, value string) bool {
	name, to, _ := strings.Cut(scope, "@")
	gotName, gotTo, _ := strings.Cut(value, "@")
	if !matchPart(name, gotName) {
		return false
	}
	if to == "*" {
		return true
	}
	kind, rest, _ := strings.Cut(gotTo, ":")
	if host, ok := strings.CutPrefix(to, Unencrypted+":"); ok {
		// Bound to a host over any connection, it is bound over an
		// encrypted one too.
		return (kind == Net || kind == Unencrypted) && matchHost(host, rest)
	}
	return s.matches(to, kind, rest)
}

// matchPart matches a name, or one part of a path or a host: * stands for
// any run of characters.
func matchPart(pattern, name string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == name
	}
	pieces := strings.Split(pattern, "*")
	if !strings.HasPrefix(name, pieces[0]) {
		return false
	}
	name = name[len(pieces[0]):]
	for _, piece := range pieces[1 : len(pieces)-1] {
		i := strings.Index(name, piece)
		if i < 0 {
			return false
		}
		name = name[i+len(piece):]
	}
	return strings.HasSuffix(name, pieces[len(pieces)-1])
}

// matchParts matches the parts of a path against a pattern: * stays inside
// one part, and ** stands for any number of parts.
func matchParts(pattern, parts []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			if len(pattern) == 1 {
				return true
			}
			for skip := 0; skip <= len(parts); skip++ {
				if matchParts(pattern[1:], parts[skip:]) {
					return true
				}
			}
			return false
		}
		if len(parts) == 0 || !matchPart(pattern[0], parts[0]) {
			return false
		}
		pattern, parts = pattern[1:], parts[1:]
	}
	return len(parts) == 0
}

// matchHost matches a host, with a port if the scope names one: a scope
// with no port means every port, * alone means every host, and otherwise *
// stays inside one part of a name.
func matchHost(scope, value string) bool {
	host, port := splitHost(scope)
	gotHost, gotPort := splitHost(value)
	if port != "" && !matchPart(port, gotPort) {
		return false
	}
	host, gotHost = NormalHost(host), NormalHost(gotHost)
	if host == "*" {
		return true
	}
	if addressScope(host) || net.ParseIP(gotHost) != nil {
		// An address is one part, and a scope written as an address
		// matches no name: 10.x.example.com has the parts of 10.*.*.*.
		return net.ParseIP(gotHost) != nil && matchPart(host, gotHost)
	}
	a, b := strings.Split(host, "."), strings.Split(gotHost, ".")
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !matchPart(a[i], b[i]) {
			return false
		}
	}
	return true
}

// splitHost cuts a host from its port, if it has one.
func splitHost(text string) (host, port string) {
	if strings.HasPrefix(text, "[") {
		if end := strings.Index(text, "]"); end > 0 {
			return text[1:end], strings.TrimPrefix(text[end+1:], ":")
		}
	}
	if strings.Count(text, ":") == 1 {
		host, port, _ = strings.Cut(text, ":")
		return host, port
	}
	return text, ""
}

// AllowsSecret says whether every layer lets the secret of a name travel
// somewhere at all: by an entry for it, or by all.
func AllowsSecret(layers []Layer, name string) bool {
	for _, layer := range layers {
		ok := false
		for _, entry := range layer.Allow {
			if entry == "all" {
				ok = true
			} else if rest, is := strings.CutPrefix(entry, "secret:"); is {
				pattern, _, _ := strings.Cut(rest, "@")
				ok = ok || matchPart(pattern, name)
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Declares says whether a job's allow list declares a permission that is
// listed in an approve: list: an entry of the same kind whose scope matches
// a value the listed scope matches, or a group that contains it, or all. A location in
// either list is read from the tree's prefix.
func Declares(allow []string, listed []string, prefix string) bool {
	set := &Set{Prefix: prefix}
	for _, want := range listed {
		wantKinds, wantGroup := groups[want]
		wantKind, wantScope, _ := strings.Cut(want, ":")
		for _, entry := range allow {
			in, isGroup := groups[entry]
			kind, scope, _ := strings.Cut(entry, ":")
			switch {
			case isGroup && wantGroup:
				for _, k := range in {
					if slices.Contains(wantKinds, k) {
						return true
					}
				}
			case isGroup:
				if slices.Contains(in, wantKind) {
					return true
				}
			case wantGroup:
				if slices.Contains(wantKinds, kind) {
					return true
				}
			case kind == wantKind && (kind == Send || kind == At):
				entryFull, err := ReadLocation(prefix, scope)
				wantFull, wantErr := ReadLocation(prefix, wantScope)
				if err == nil && wantErr == nil && (matchLocation(wantFull, entryFull) || matchLocation(entryFull, wantFull) || overlaps(kind, wantFull, entryFull)) {
					return true
				}
			case kind == wantKind:
				// The two scopes match a common value: a pattern is matched
				// as if the other were a value, and two patterns are
				// compared as texts.
				if set.matches(want, kind, scope) || set.matches(entry, kind, wantScope) || overlaps(kind, wantScope, scope) {
					return true
				}
			}
		}
	}
	return false
}

// overlaps says whether two scopes of a kind, read as texts in which * is
// any characters, match a common text. A host with no port has every port,
// and a secret's name and destination are compared apart.
func overlaps(kind, a, b string) bool {
	switch kind {
	case Net:
		hostA, portA := splitHost(a)
		hostB, portB := splitHost(b)
		return overlap(NormalHost(hostA), NormalHost(hostB)) && (portA == "" || portB == "" || overlap(portA, portB))
	case Secret:
		nameA, toA, _ := strings.Cut(a, "@")
		nameB, toB, _ := strings.Cut(b, "@")
		if !overlap(nameA, nameB) {
			return false
		}
		if toA == "*" || toB == "*" {
			return true
		}
		kindA, scopeA, _ := strings.Cut(toA, ":")
		kindB, scopeB, _ := strings.Cut(toB, ":")
		// Bound to a host over any connection, a secret is bound over an
		// encrypted one too.
		if kindA == Unencrypted {
			kindA = Net
		}
		if kindB == Unencrypted {
			kindB = Net
		}
		return kindA == kindB && overlaps(kindA, scopeA, scopeB)
	}
	return overlap(a, b)
}

// overlap says whether two patterns match a common text.
func overlap(a, b string) bool {
	// from[i][j] says it of a[i:] and b[j:].
	from := make([][]bool, len(a)+1)
	for i := len(a); i >= 0; i-- {
		from[i] = make([]bool, len(b)+1)
		for j := len(b); j >= 0; j-- {
			switch {
			case i == len(a) && j == len(b):
				from[i][j] = true
			case i == len(a):
				from[i][j] = b[j] == '*' && from[i][j+1]
			case j == len(b):
				from[i][j] = a[i] == '*' && from[i+1][j]
			case a[i] == '*' || b[j] == '*':
				from[i][j] = from[i+1][j] || from[i][j+1]
			default:
				from[i][j] = a[i] == b[j] && from[i+1][j+1]
			}
		}
	}
	return from[0][0]
}

// Denied says whether a deny entry of any layer matches the thing, without
// noting it as used: a denied name is refused before it is looked up.
func (s *Set) Denied(kind, value string) *Refusal {
	for _, layer := range s.Layers {
		for _, entry := range layer.Deny {
			if !s.readsDeny(entry) || s.matches(entry, kind, value) {
				return &Refusal{What: kind + ":" + value, Layer: layer.Name,
					Fix: fmt.Sprintf("take the deny entry %s out of %s", entry, layer.Name)}
			}
		}
	}
	return nil
}

// Peek says whether every layer would pass the thing, without noting it as
// used.
func (s *Set) Peek(kind string, values ...string) bool {
	for _, layer := range s.Layers {
		if ok, _ := s.passes(layer, kind, values); !ok {
			return false
		}
	}
	return true
}

// AllowsAnAddress says whether some layer's net entries name an address
// and not a host's name, so that a name no entry allows might still be
// allowed by where it resolves to.
func (s *Set) AllowsAnAddress() bool {
	for _, layer := range s.Layers {
		for _, entry := range layer.Allow {
			if scope, ok := strings.CutPrefix(entry, Net+":"); ok {
				host, _ := splitHost(scope)
				if addressScope(host) {
					return true
				}
			}
		}
	}
	return false
}

// addressScope says whether a host scope is written as an address, whole
// or with * for parts of it, and not as a name. * alone is every host.
func addressScope(host string) bool {
	if host == "*" {
		return false
	}
	return net.ParseIP(host) != nil || strings.Contains(host, ":") || strings.Trim(host, "0123456789.*") == ""
}

// NormalHost gives a host in the one spelling it is matched in: lower
// case, no trailing dot, an IPv4 address written inside an IPv6 one taken
// as the IPv4 address, and an address's zone dropped.
func NormalHost(host string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if zone := strings.IndexByte(host, '%'); zone >= 0 {
		host = host[:zone]
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	return host
}

// CheckHost says whether a text can be a host's name or an address. What
// is not must not be handed to a resolver.
func CheckHost(host string) error {
	if _, err := netip.ParseAddr(host); err == nil {
		return nil
	}
	if host == "" {
		return fmt.Errorf("the host has no name")
	}
	for _, r := range host {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("%q is not a host's name", host)
		}
	}
	return nil
}

// Mark gives a place in the list of what was used, to go back to.
func (s *Set) Mark() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.used)
}

// Reset forgets what was noted as used since a mark: nothing is noted
// until every check for a call has passed.
func (s *Set) Reset(mark int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mark < len(s.used) {
		s.used = s.used[:mark]
	}
}
