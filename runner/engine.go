package runner

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// engine runs one script. A script is JavaScript and is the body of a
// function: its input is the job's input, and what it returns is the job's
// output. It starts with nothing; everything it can do outside itself is a
// call to a plugin.
type engine struct {
	vm  *goja.Runtime
	b   *begin
	job *plugin.Job
	ch  *channel
	// behind is what stands behind each value of the script's that the
	// script cannot see into: a blob, a secret, something open. It is kept
	// here, outside the script's reach, and nothing of it shows on the value.
	behind map[*goja.Object]any
	u8     *goja.Object // the constructor of the one type bytes have

	mu      sync.Mutex
	why     *machine.Error // why the script was stopped
	modules map[string]goja.Value
}

// The start of the function a script is the body of. It has no line break,
// so a script's lines are counted as they were written.
const scriptHead = `(function(input,job,emit,log,progress,require){"use strict";`

func newEngine(b *begin, job *plugin.Job, ch *channel) *engine {
	vm := goja.New()
	vm.SetMaxCallStackSize(2000)
	// A source map named in a script is never read.
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	e := &engine{vm: vm, b: b, job: job, ch: ch, modules: map[string]goja.Value{}, behind: map[*goja.Object]any{}}
	e.u8 = vm.Get("Uint8Array").ToObject(vm)
	return e
}

// stop interrupts the script. A stop is not something a script can catch.
func (e *engine) stop(why *machine.Error) {
	e.mu.Lock()
	if e.why == nil {
		e.why = why
	}
	e.mu.Unlock()
	e.vm.Interrupt(why)
}

func (e *engine) stopped() *machine.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.why
}

// run runs the script and gives how it ended.
func (e *engine) run() (result *instance.Result) {
	defer func() {
		if r := recover(); r != nil {
			result = failed(&machine.Error{Status: machine.StatusError, Error: e.job.Mask(fmt.Sprintf("the script's engine failed: %v", r))})
		}
	}()
	src := scriptHead + e.b.Script + "\n})"
	// A script is exactly one function body: text that closes the function
	// and opens something after it is refused before any of it runs.
	tree, err := parser.ParseFile(nil, "script", src, 0, parser.WithDisableSourceMaps)
	if err != nil {
		return failed(&machine.Error{Status: machine.StatusError, Error: "the script cannot be read: " + e.position(err.Error())})
	}
	if !oneFunction(tree) {
		return failed(&machine.Error{Status: machine.StatusError,
			Error: "the script is not the body of one function: it closes the function it is given and opens something after it"})
	}
	program, err := goja.CompileAST(tree, true)
	if err != nil {
		return failed(&machine.Error{Status: machine.StatusError, Error: "the script cannot be read: " + e.position(err.Error())})
	}
	for _, name := range plugin.Names() {
		module, err := e.module(name)
		if err != nil {
			return failed(&machine.Error{Status: machine.StatusError, Error: err.Error()})
		}
		e.vm.Set(name, module)
	}
	fn, err := e.vm.RunProgram(program)
	if err != nil {
		return e.failure(err)
	}
	call, _ := goja.AssertFunction(fn)
	// The input is what the machine gave the job, not what a plugin hands
	// back: it reaches the script as it is.
	input, err := e.toJS(e.b.Job.Input, asItIs)
	if err != nil {
		return e.failure(err)
	}
	info := e.vm.NewObject()
	info.Set("request", e.b.Job.Request)
	info.Set("name", e.b.Job.Name)
	info.Set("attempt", e.b.Job.Attempt)
	info.Set("instance", e.b.Job.Instance)
	value, err := call(goja.Undefined(), input, info, e.vm.ToValue(e.emit), e.vm.ToValue(e.log), e.vm.ToValue(e.progress),
		e.vm.ToValue(e.require))
	if err != nil {
		return e.failure(err)
	}
	output, err := e.leaving(value)
	if err != nil {
		return e.failure(err)
	}
	return &instance.Result{Status: instance.Done, Output: output}
}

// oneFunction says whether a parsed script is one function and nothing
// else.
func oneFunction(tree *ast.Program) bool {
	if len(tree.Body) != 1 {
		return false
	}
	statement, ok := tree.Body[0].(*ast.ExpressionStatement)
	if !ok {
		return false
	}
	_, ok = statement.Expression.(*ast.FunctionLiteral)
	return ok
}

var where = regexp.MustCompile(`(?:Line |:)(\d+):(\d+)`)

// position gives an error's place as the script was written: the function
// Corbie adds is on the script's first line and shifts only that line's
// columns.
func (e *engine) position(text string) string {
	m := where.FindStringSubmatchIndex(text)
	if m == nil {
		return oneLine(text)
	}
	line, _ := strconv.Atoi(text[m[2]:m[3]])
	column, _ := strconv.Atoi(text[m[4]:m[5]])
	if line == 1 {
		column -= len(scriptHead)
	}
	message := strings.TrimSpace(text[:m[0]])
	message = strings.TrimSuffix(strings.TrimSuffix(message, "at script"), "script")
	if i := strings.Index(message, "\n"); i >= 0 {
		message = message[:i]
	}
	if strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(message), ":")) == "" {
		// The engine says where first and what after.
		message, _, _ = strings.Cut(strings.TrimSpace(text[m[1]:]), " (and ")
	}
	return fmt.Sprintf("%s at line %d, column %d", oneLine(strings.TrimSuffix(strings.TrimSpace(message), ":")), line, max(column, 1))
}

func oneLine(text string) string { return strings.Join(strings.Fields(text), " ") }

// failure turns what stopped a script into the job's result.
func (e *engine) failure(err error) *instance.Result {
	if why := e.stopped(); why != nil {
		return failed(why)
	}
	var refusal *permission.Refusal
	if errors.As(err, &refusal) {
		return failed(e.denial(refusal))
	}
	var thrown *goja.Exception
	if errors.As(err, &thrown) {
		text := thrown.Value().String()
		if obj, ok := thrown.Value().(*goja.Object); ok {
			if stack := obj.Get("stack"); stack != nil && !goja.IsUndefined(stack) {
				text = stack.String()
			}
		}
		// The message is searched for secrets before anything is cut from
		// it.
		message := e.job.Mask(thrown.Value().String())
		if m := where.FindStringSubmatch(strings.TrimPrefix(text, thrown.Value().String())); m != nil {
			message = e.position(message + " at script" + m[0])
		}
		return failed(&machine.Error{Status: machine.StatusError, Error: oneLine(message)})
	}
	var overflow *goja.StackOverflowError
	if errors.As(err, &overflow) {
		return failed(&machine.Error{Status: machine.StatusError, Error: "the script calls itself without end"})
	}
	return failed(&machine.Error{Status: machine.StatusError, Error: oneLine(e.job.Mask(err.Error()))})
}

// denial is a refusal as a job's failure: what was tried, which layer
// refused it and what would fix it. What was tried can be something the
// other side chose, so it is searched for secrets like any text that
// leaves the job.
func (e *engine) denial(r *permission.Refusal) *machine.Error {
	return &machine.Error{Status: machine.StatusDenied, Error: e.job.Mask(r.Error()), Refused: e.job.Mask(r.What),
		Layer: r.Layer, Fix: e.job.Mask(r.Fix)}
}

// throw raises a plugin's error in the script. A refusal stops the job: the
// script cannot catch it and carry on.
func (e *engine) throw(err error) {
	var refusal *permission.Refusal
	if errors.As(err, &refusal) {
		e.stop(e.denial(refusal))
		panic(e.vm.NewGoError(err))
	}
	code, message := plugin.Failed, err.Error()
	var known *plugin.Error
	if errors.As(err, &known) {
		code, message = known.Code, known.Message
	}
	thrown, _ := e.vm.New(e.vm.Get("Error"), e.vm.ToValue(e.job.Mask(message)))
	thrown.Set("code", code)
	panic(thrown)
}

// module gives a module as a script holds it: an object with its calls.
func (e *engine) module(name string) (goja.Value, error) {
	if v, ok := e.modules[name]; ok {
		return v, nil
	}
	m, ok := plugin.Find(name)
	if !ok {
		return nil, fmt.Errorf("this runner does not have the plugin %q (it has: %s)", name, strings.Join(plugin.Names(), ", "))
	}
	obj := e.vm.NewObject()
	e.modules[name] = obj
	for _, call := range sortedCalls(m.Calls) {
		obj.Set(call, e.wrap(name, call, m.Calls[call]))
	}
	if m.Library != "" {
		if err := e.load(name, m.Library, obj); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

func sortedCalls(calls map[string]*plugin.Call) []string {
	names := make([]string, 0, len(calls))
	for name := range calls {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// load runs a library or a file written in script, which fills in exports.
func (e *engine) load(name, source string, exports *goja.Object) error {
	fn, err := e.vm.RunScript(name, "(function(exports,require){\"use strict\";"+source+"\n})")
	if err != nil {
		return fmt.Errorf("%s cannot be read: %s", name, oneLine(err.Error()))
	}
	call, _ := goja.AssertFunction(fn)
	_, err = call(goja.Undefined(), exports, e.vm.ToValue(e.require))
	return err
}

// require gives a plugin or a library by its bare name, or one of the files
// the job lists beside the script by its path. A path is looked up only
// among those files, never on the runner's disk.
func (e *engine) require(call goja.FunctionCall) goja.Value {
	name := call.Argument(0).String()
	if !strings.HasPrefix(name, "./") && !strings.HasPrefix(name, "../") {
		module, err := e.module(name)
		if err != nil {
			e.throw(plugin.Errorf(plugin.NotFound, "%v", err))
		}
		return module
	}
	if v, ok := e.modules[name]; ok {
		return v
	}
	source, ok := e.b.Files[name]
	if !ok {
		e.throw(plugin.Errorf(plugin.NotFound, "%s is not among the files the job lists beside its script", name))
	}
	exports := e.vm.NewObject()
	e.modules[name] = exports
	if err := e.load(name, source, exports); err != nil {
		var thrown *goja.Exception
		if errors.As(err, &thrown) {
			panic(thrown.Value())
		}
		e.throw(err)
	}
	return exports
}

// wrap makes a plugin's call a function of the script's: the one obvious
// thing first, then named options.
func (e *engine) wrap(module, name string, c *plugin.Call) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if c.Variadic {
			all := make([]any, len(call.Arguments))
			for i, arg := range call.Arguments {
				v, err := e.toGo(arg, 0)
				if err != nil {
					e.throw(err)
				}
				all[i] = v
			}
			result, err := c.Do(e.job, all, plugin.Options{})
			if err != nil {
				e.throw(err)
			}
			value, err := e.toJS(result, e.job.Mask)
			if err != nil {
				e.throw(err)
			}
			return value
		}
		first, err := e.toGo(call.Argument(0), 0)
		if err != nil {
			e.throw(err)
		}
		options := plugin.Options{}
		if second := call.Argument(1); !goja.IsUndefined(second) && !goja.IsNull(second) {
			given, err := e.toGo(second, 0)
			if err != nil {
				e.throw(err)
			}
			named, ok := given.(map[string]any)
			if !ok {
				e.throw(plugin.Errorf(plugin.BadInput, "%s.%s takes one thing and then named options, as %s.%s(thing, { option: value })", module, name, module, name))
			}
			options = named
		}
		if len(call.Arguments) > 2 {
			e.throw(plugin.Errorf(plugin.BadInput, "%s.%s takes one thing and then named options", module, name))
		}
		// Nothing is noted as used until every check for the call has
		// passed.
		mark := e.job.Perms.Mark()
		result, err := c.Invoke(e.job, module, name, first, options)
		if err != nil {
			e.job.Perms.Reset(mark)
			e.throw(err)
		}
		value, err := e.toJS(result, e.job.Mask)
		if err != nil {
			e.throw(err)
		}
		return value
	}
}

// emit sends a named value up while the script runs.
func (e *engine) emit(call goja.FunctionCall) goja.Value {
	name := call.Argument(0).String()
	if goja.IsUndefined(call.Argument(0)) || name == "" {
		e.throw(plugin.Errorf(plugin.BadInput, "emit takes a name and a value"))
	}
	data, err := e.leaving(call.Argument(1))
	if err != nil {
		e.throw(err)
	}
	if err := e.ch.Emit(name, data); err != nil {
		e.throw(err)
	}
	return goja.Undefined()
}

func (e *engine) log(call goja.FunctionCall) goja.Value {
	var parts []string
	for _, arg := range call.Arguments {
		if _, isText := arg.(goja.String); isText {
			parts = append(parts, arg.String())
			continue
		}
		v, err := e.toGo(arg, 0)
		if err != nil {
			e.throw(err)
		}
		parts = append(parts, fmt.Sprint(show(v)))
	}
	if err := e.ch.Emit(instance.EmitLog, e.job.Mask(strings.Join(parts, " "))); err != nil {
		e.throw(err)
	}
	return goja.Undefined()
}

func (e *engine) progress(call goja.FunctionCall) goja.Value {
	data := map[string]any{"done": call.Argument(0).ToFloat()}
	if total := call.Argument(1); !goja.IsUndefined(total) {
		data["total"] = total.ToFloat()
	}
	if err := e.ch.Emit(instance.EmitProgress, data); err != nil {
		e.throw(err)
	}
	return goja.Undefined()
}

// leaving turns a script's value into data that leaves the job, in an emit
// or in the result. A blob the job made is sent to the store now, and its
// reference stands in for it.
func (e *engine) leaving(v goja.Value) (any, error) {
	data, err := e.toGo(v, 0)
	if err != nil {
		return nil, err
	}
	return e.outward(data)
}

func (e *engine) outward(v any) (any, error) {
	switch v := v.(type) {
	case *plugin.Blob:
		ref, err := v.Store(e.job)
		if err != nil {
			return nil, err
		}
		return ref.Value(), nil
	case *plugin.Secret:
		return nil, plugin.Errorf(plugin.BadInput, "a secret cannot leave a job: a handle can go only into a plugin's call")
	case *plugin.Object:
		return nil, plugin.Errorf(plugin.BadInput, "something that stays open cannot leave a job")
	case plugin.Bytes:
		ref, err := plugin.NewBlob(blob.Bytes, v).Store(e.job)
		if err != nil {
			return nil, err
		}
		return ref.Value(), nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			w, err := e.outward(item)
			if err != nil {
				return nil, err
			}
			out[k] = w
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			w, err := e.outward(item)
			if err != nil {
				return nil, err
			}
			out[i] = w
		}
		return out, nil
	case string:
		return e.job.Mask(v), nil
	}
	return v, nil
}

// show gives a value as a log line shows it.
func show(v any) any {
	switch v := v.(type) {
	case *plugin.Blob:
		return fmt.Sprintf("blob(%s, %d bytes)", v.Type, v.Size)
	case *plugin.Secret:
		return v.String()
	case plugin.Bytes:
		return fmt.Sprintf("bytes(%d)", len(v))
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = show(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = show(item)
		}
		return out
	}
	return v
}

var (
	bufferType = reflect.TypeFor[goja.ArrayBuffer]()
	listType   = reflect.TypeFor[[]any]()
)

// maxDepth is how deep a value may nest and still be turned into data.
const maxDepth = 200

// toGo turns a script's value into data: text, numbers, lists, maps, bytes,
// handles to secrets and blobs. The job's time limit is checked as it goes.
func (e *engine) toGo(v goja.Value, depth int) (any, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, nil
	}
	if depth > maxDepth {
		return nil, plugin.Errorf(plugin.BadInput, "a value is nested too deep to be turned into data")
	}
	if why := e.stopped(); why != nil {
		return nil, plugin.Errorf(plugin.TimedOut, "%s", why.Error)
	}
	obj, isObject := v.(*goja.Object)
	if !isObject {
		switch x := v.Export().(type) {
		case string, bool, float64:
			return x, nil
		case int64:
			return float64(x), nil
		case int:
			return float64(x), nil
		}
		return v.String(), nil
	}
	// A value is asked its type; it is never first converted out of the
	// engine to find out.
	if held, ok := e.behind[obj]; ok {
		return held, nil
	}
	// Bytes: a buffer, or any view of one, from its offset for its length.
	kind := obj.ExportType()
	switch {
	case kind == bufferType:
		if buffer, ok := obj.Export().(goja.ArrayBuffer); ok {
			return plugin.Bytes(slices.Clone(buffer.Bytes())), nil
		}
	case kind != nil && kind.Kind() == reflect.Slice && kind != listType, obj.ClassName() == "DataView":
		if buffer, ok := obj.Get("buffer").(*goja.Object); ok && buffer.ExportType() == bufferType {
			all := buffer.Export().(goja.ArrayBuffer).Bytes()
			offset, length := int(obj.Get("byteOffset").ToInteger()), int(obj.Get("byteLength").ToInteger())
			if length == 0 || offset < 0 || offset+length > len(all) {
				return plugin.Bytes{}, nil
			}
			return plugin.Bytes(slices.Clone(all[offset : offset+length])), nil
		}
	}
	switch class := obj.ClassName(); {
	case class == "Function":
		return nil, plugin.Errorf(plugin.BadInput, "a function is not data")
	case class == "Date":
		if at, ok := obj.Export().(time.Time); ok {
			return at.UTC().Format(time.RFC3339Nano), nil
		}
	case class == "Array":
		length := int(obj.Get("length").ToInteger())
		list := make([]any, 0, min(length, 1024))
		for i := range length {
			if i%4096 == 4095 {
				if why := e.stopped(); why != nil {
					return nil, plugin.Errorf(plugin.TimedOut, "%s", why.Error)
				}
			}
			item, err := e.toGo(obj.Get(strconv.Itoa(i)), depth+1)
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		}
		return list, nil
	}
	out := map[string]any{}
	for _, key := range obj.Keys() {
		item, err := e.toGo(obj.Get(key), depth+1)
		if err != nil {
			return nil, err
		}
		out[key] = item
	}
	return out, nil
}

// holding makes a script's value that stands for something the script
// cannot see into: a blob, a secret, something open. It shows only what is
// set on it.
func (e *engine) holding(behind any) *goja.Object {
	obj := e.vm.NewObject()
	e.behind[obj] = behind
	return obj
}

// asItIs is text that goes to the script unchanged.
func asItIs(text string) string { return text }

// toJS turns data into a script's value. Each text in it is given as text
// says: what a plugin hands back is searched for secrets on its way in.
func (e *engine) toJS(v any, text func(string) string) (goja.Value, error) {
	switch v := v.(type) {
	case nil:
		return goja.Null(), nil
	case string:
		return e.vm.ToValue(text(v)), nil
	case bool, float64, int, int64:
		return e.vm.ToValue(v), nil
	case plugin.Bytes:
		return e.vm.New(e.u8, e.vm.ToValue(e.vm.NewArrayBuffer(slices.Clone(v))))
	case []byte:
		return e.toJS(plugin.Bytes(v), text)
	case blob.Ref:
		return e.toJS(plugin.StoredBlob(v), text)
	case *plugin.Blob:
		obj := e.holding(v)
		obj.Set("type", v.Type)
		obj.Set("size", v.Size)
		return obj, nil
	case *plugin.Secret:
		// Every way of turning a handle into text throws; it never gives a
		// stand-in text.
		obj := e.holding(v)
		refuse := e.vm.ToValue(func(goja.FunctionCall) goja.Value {
			panic(e.vm.NewTypeError("a secret cannot be turned into text, compared, measured or read: a handle goes only into a plugin's call"))
		})
		for _, name := range []string{"toString", "toJSON", "valueOf", "toLocaleString"} {
			obj.DefineDataProperty(name, refuse, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
		}
		obj.DefineDataPropertySymbol(goja.SymToPrimitive, refuse, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
		return obj, nil
	case *plugin.Object:
		obj := e.holding(v)
		for _, name := range sortedCalls(v.Calls) {
			obj.Set(name, e.wrap("object", name, v.Calls[name]))
		}
		closeIt := v.Close
		if closeIt != nil {
			e.job.OnEnd(func() { closeIt() })
		}
		obj.Set("close", func(goja.FunctionCall) goja.Value {
			if closeIt != nil {
				if err := closeIt(); err != nil {
					e.throw(err)
				}
			}
			return goja.Undefined()
		})
		if v.Calls["next"] != nil {
			// Something that gives one thing after another is walked as
			// the language walks anything: for (const item of it).
			obj.SetSymbol(goja.SymIterator, func(goja.FunctionCall) goja.Value { return obj })
		}
		return obj, nil
	case []string:
		list := make([]any, len(v))
		for i, item := range v {
			list[i] = item
		}
		return e.toJS(list, text)
	case []any:
		items := make([]any, len(v))
		for i, item := range v {
			value, err := e.toJS(item, text)
			if err != nil {
				return nil, err
			}
			items[i] = value
		}
		return e.vm.NewArray(items...), nil
	case map[string]any:
		if ref, ok := blob.AsRef(v); ok {
			return e.toJS(plugin.StoredBlob(ref), text)
		}
		obj := e.vm.NewObject()
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			value, err := e.toJS(v[k], text)
			if err != nil {
				return nil, err
			}
			obj.Set(k, value)
		}
		return obj, nil
	}
	return nil, fmt.Errorf("a plugin gave a %T, which is not data a script can hold", v)
}
