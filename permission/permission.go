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
	Read      = "fs.read"
	Write     = "fs.write"
	Net       = "net"
	Exec      = "exec"
	Env       = "env"
	Secret    = "secret"
	KeptRead  = "kept.read"
	KeptWrite = "kept.write"
)

// Unencrypted is a destination of a secret and not a kind: a connection to
// a host, whether it is encrypted or not.
const Unencrypted = "net.unencrypted"

var kinds = map[string]bool{Read: true, Write: true, Net: true, Exec: true, Env: true, Secret: true, KeptRead: true, KeptWrite: true}

// The groups, each standing for every scope of its kinds. "all" allows
// everything, secrets included, and means the same in every layer.
var groups = map[string][]string{
	"network": {Net}, "disk": {Read, Write}, "exec": {Exec}, "env": {Env}, "kept": {KeptRead, KeptWrite},
	"all": {Read, Write, Net, Exec, Env, Secret, KeptRead, KeptWrite},
}

// CheckWritten says what is wrong with an entry as it is written in a list:
// a group, or a kind and a scope. The kind is written out; the scope may
// still hold an expression to be filled in.
func CheckWritten(entry string) error {
	if _, ok := groups[entry]; ok {
		return nil
	}
	kind, scope, ok := strings.Cut(entry, ":")
	if !ok || !kinds[kind] {
		return fmt.Errorf("%q is not a permission: write a kind and a scope, such as net:api.github.com (the kinds are fs.read, fs.write, net, exec, env, secret, kept.read and kept.write; the groups are network, disk, exec, env, kept and all)", entry)
	}
	if scope == "" {
		return fmt.Errorf("%q has no scope", entry)
	}
	if (kind == KeptRead || kind == KeptWrite) && !strings.Contains(scope, "${") {
		if err := keptPath(scope, true); err != nil {
			return fmt.Errorf("%q: %v", entry, err)
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
		return CheckWritten(destination)
	case Unencrypted:
		return CheckWritten(Net + ":" + rest)
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

// CheckKeptPath says whether a value can be kept at the path. It is the
// one rule for a kept path, in a call, in a scope and in the store: a
// directory takes two names that differ only in case for one, and the
// store a value is kept in may be a directory.
func CheckKeptPath(path string) error { return keptPath(path, false) }

// keptPath says what is wrong with a kept path or, with pattern, a kept
// scope: there a * stands for any run of characters in a part, and ** for
// any number of whole parts.
func keptPath(path string, pattern bool) error {
	wrong := fmt.Errorf("%q is not a kept path: a kept path is parts joined by /, each of the lower-case letters a to z, digits, ., - and _, none empty and none starting with a dot, at most 256 characters", path)
	if path == "" || len(path) > 256 {
		return wrong
	}
	for part := range strings.SplitSeq(path, "/") {
		if part == "" || part[0] == '.' || (part != "**" && strings.Contains(part, "**")) {
			return wrong
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_' || pattern && c == '*') {
				return wrong
			}
		}
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
// as a ceiling or a start's list is.
func CheckList(entries []string) error {
	for _, entry := range entries {
		if strings.Contains(entry, "${") {
			return fmt.Errorf("%q: an entry here is written out in full, with no expression", entry)
		}
		if err := CheckWritten(entry); err != nil {
			return err
		}
	}
	return nil
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
			if slices.Contains(groups[group], kind) || CheckWritten(entry) != nil {
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
		// A deny entry that cannot be read refuses everything in its layer.
		if CheckWritten(entry) != nil || strings.Contains(entry, "${") {
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
	case KeptRead, KeptWrite:
		// A kept path is a name in the store, matched as it is written:
		// nothing in it is a file.
		return matchParts(strings.Split(scope, "/"), strings.Split(value, "/"))
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

// KeptBelow gives the deepest kept path at or below which lies every kept
// path that every layer allows: in each layer, the parts its kept scopes
// share before their first part with a * in it, and of those the deepest.
// Where some layer allows no kept path, or denies every one, or those
// parts lie on no one path, no kept path passes every layer, and there is
// none.
func KeptBelow(layers []Layer) (string, bool) {
	var deepest []string
	for _, layer := range layers {
		fixed, allows := keptFixed(layer)
		if !allows {
			return "", false
		}
		short, long := fixed, deepest
		if len(short) > len(long) {
			short, long = long, short
		}
		if !slices.Equal(short, long[:len(short)]) {
			return "", false
		}
		deepest = long
	}
	return strings.Join(deepest, "/"), len(layers) > 0
}

// keptFixed gives the parts a layer's kept scopes share before the first
// part with a * in it, and whether the layer allows any kept path. A kind
// the layer denies at every path, by a group or by the scope **, allows
// nothing there.
func keptFixed(layer Layer) ([]string, bool) {
	open := map[string]bool{KeptRead: true, KeptWrite: true}
	for _, entry := range layer.Deny {
		for _, kind := range []string{KeptRead, KeptWrite} {
			if slices.Contains(groups[entry], kind) || entry == kind+":**" {
				open[kind] = false
			}
		}
	}
	var shared []string
	allows := false
	for _, entry := range layer.Allow {
		var fixed []string
		if in, isGroup := groups[entry]; isGroup {
			if !slices.ContainsFunc(in, func(kind string) bool { return open[kind] }) {
				continue
			}
		} else {
			kind, scope, _ := strings.Cut(entry, ":")
			if !open[kind] || keptPath(scope, true) != nil {
				continue
			}
			for part := range strings.SplitSeq(scope, "/") {
				if strings.Contains(part, "*") {
					break
				}
				fixed = append(fixed, part)
			}
		}
		if !allows {
			shared, allows = fixed, true
			continue
		}
		same := 0
		for same < len(shared) && same < len(fixed) && shared[same] == fixed[same] {
			same++
		}
		shared = shared[:same]
	}
	return shared, allows
}

// Declares says whether a job's allow list declares a permission that is
// listed to be asked about: an entry of the same kind where either scope
// covers the other, or a group that contains it, or all.
func Declares(allow []string, listed []string) bool {
	set := &Set{}
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
			case kind == wantKind:
				// Either scope covers the other: a pattern is matched as
				// if the other were a value.
				if set.matches(want, kind, scope) || set.matches(entry, kind, wantScope) || covers(wantScope, scope) || covers(scope, wantScope) {
					return true
				}
			}
		}
	}
	return false
}

// covers says whether one path pattern reaches at least everything under
// the fixed start of another.
func covers(wide, narrow string) bool {
	fixed, _, _ := strings.Cut(narrow, "*")
	head, _, found := strings.Cut(wide, "*")
	return found && strings.HasPrefix(fixed, head)
}

// Denied says whether a deny entry of any layer matches the thing, without
// noting it as used: a denied name is refused before it is looked up.
func (s *Set) Denied(kind, value string) *Refusal {
	for _, layer := range s.Layers {
		for _, entry := range layer.Deny {
			if CheckWritten(entry) != nil || s.matches(entry, kind, value) {
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
