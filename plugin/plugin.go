// Package plugin is the modules a script calls, the one shape they all
// have, and the few ways out they all go through. Everything that comes
// with Corbie is built through this package, and an extension uses the
// same.
package plugin

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// A Module is a plugin, or a library written in script. A script gets it
// with require and its name.
type Module struct {
	Name string
	// Page is the module's page of the manual, which is its specification:
	// each call, its options and the permissions it needs, with examples.
	Page string
	// Calls are what a plugin gives a script, by name.
	Calls map[string]*Call
	// Library is the source of a library written in script: the body of a
	// function that fills in exports.
	Library string
}

// A Call is one thing a script can ask of a plugin: the one obvious thing
// first, then named options. An option the call does not list is refused.
type Call struct {
	Options []string
	// Variadic is for the rare call that the language itself makes with
	// several things, as a tag on a template is called: first is then the
	// list of everything it was called with.
	Variadic bool
	Do       func(job *Job, first any, options Options) (any, error)
}

// An Object is something that stays open, such as a connection: its own
// calls, and a close.
type Object struct {
	Calls map[string]*Call
	Close func() error
}

// Bytes is the one type bytes have everywhere.
type Bytes []byte

var modules = map[string]*Module{}

// own are the names of a script's own things, which no module can take.
var own = []string{"input", "job", "emit", "log", "progress", "require"}

// Register adds a module. It refuses a name a script's own things have.
func Register(m *Module) {
	if slices.Contains(own, m.Name) {
		panic(fmt.Sprintf("plugin: a module cannot be named %q, which is one of a script's own things", m.Name))
	}
	if _, dup := modules[m.Name]; dup {
		panic(fmt.Sprintf("plugin: two modules are named %q", m.Name))
	}
	modules[m.Name] = m
}

// Find gives the module of a name.
func Find(name string) (*Module, bool) {
	m, ok := modules[name]
	return m, ok
}

// Names lists the modules.
func Names() []string {
	var names []string
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// The codes an error has.
const (
	NotFound     = "not_found"     // the thing asked for is not there
	Refused      = "refused"       // the other side said no
	TimedOut     = "timeout"       // it took too long
	NotPermitted = "not_permitted" // the system, not Corbie, would not allow it
	BadInput     = "bad_input"     // the call was written wrong
	Failed       = "failed"        // anything else
)

// An Error is the one shape a plugin's error has: a short code from a
// short list, and what happened.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Errorf makes an error with a code.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Options are the named options of a call. Each getter gives the default
// where the option was not given, and an error where it is the wrong kind
// of thing.
type Options map[string]any

func (o Options) Has(name string) bool {
	v, ok := o[name]
	return ok && v != nil
}

func wrong(name, want string, got any) *Error {
	return Errorf(BadInput, "the option %s should be %s, and is %T", name, want, got)
}

func (o Options) Text(name, otherwise string) (string, error) {
	switch v := o[name].(type) {
	case nil:
		return otherwise, nil
	case string:
		return v, nil
	default:
		return "", wrong(name, "text", v)
	}
}

func (o Options) Int(name string, otherwise int) (int, error) {
	switch v := o[name].(type) {
	case nil:
		return otherwise, nil
	case float64:
		if v == float64(int(v)) {
			return int(v), nil
		}
	}
	return 0, wrong(name, "a whole number", o[name])
}

func (o Options) Bool(name string, otherwise bool) (bool, error) {
	switch v := o[name].(type) {
	case nil:
		return otherwise, nil
	case bool:
		return v, nil
	default:
		return false, wrong(name, "true or false", v)
	}
}

// Duration reads a length of time: a number of seconds, or text such as
// "30s" or "5m".
func (o Options) Duration(name string, otherwise time.Duration) (time.Duration, error) {
	switch v := o[name].(type) {
	case nil:
		return otherwise, nil
	case float64:
		return time.Duration(v * float64(time.Second)), nil
	case string:
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			return d, nil
		}
	}
	return 0, wrong(name, "a length of time, such as 30 or \"30s\"", o[name])
}

func (o Options) Map(name string) (map[string]any, error) {
	switch v := o[name].(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return v, nil
	default:
		return nil, wrong(name, "a map", v)
	}
}

func (o Options) List(name string) ([]any, error) {
	switch v := o[name].(type) {
	case nil:
		return nil, nil
	case []any:
		return v, nil
	default:
		return nil, wrong(name, "a list", v)
	}
}

// Texts reads a list of texts.
func (o Options) Texts(name string) ([]string, error) {
	list, err := o.List(name)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, wrong(name, "a list of texts", item)
		}
		out = append(out, s)
	}
	return out, nil
}

// Text reads the first thing of a call as text.
func Text(first any, what string) (string, error) {
	s, ok := first.(string)
	if !ok || s == "" {
		return "", Errorf(BadInput, "the call needs %s, as text, first", what)
	}
	return s, nil
}

// check refuses an option the call does not list.
func (c *Call) check(module, name string, options Options) error {
	for option := range options {
		if !slices.Contains(c.Options, option) {
			known := "it takes no options"
			if len(c.Options) > 0 {
				known = "its options are: " + strings.Join(c.Options, ", ")
			}
			return Errorf(BadInput, "%s.%s does not know the option %q (%s)", module, name, option, known)
		}
	}
	return nil
}

// Invoke makes a call for a script, refusing options it does not list.
func (c *Call) Invoke(job *Job, module, name string, first any, options Options) (any, error) {
	if err := c.check(module, name, options); err != nil {
		return nil, err
	}
	return c.Do(job, first, options)
}
