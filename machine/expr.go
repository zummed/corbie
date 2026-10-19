package machine

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
)

// Any value in a definition may be an expression, written ${…}. A value
// that is exactly one expression takes the expression's value, whatever its
// type; text with expressions inside it is text with the values filled in.
// Expressions are JavaScript and read only what their scope names.

// ExprLimit is how long one expression may run.
const ExprLimit = time.Second

// A part of a text is literal text or an expression.
type part struct {
	text string
	expr bool
}

// split cuts a text into its literal parts and its expressions.
func split(text string) ([]part, error) {
	var parts []part
	for {
		i := strings.Index(text, "${")
		if i < 0 {
			break
		}
		end, err := exprEnd(text[i+2:])
		if err != nil {
			return nil, err
		}
		if i > 0 {
			parts = append(parts, part{text: text[:i]})
		}
		parts = append(parts, part{text: text[i+2 : i+2+end], expr: true})
		text = text[i+2+end+1:]
	}
	if text != "" {
		parts = append(parts, part{text: text})
	}
	return parts, nil
}

// exprEnd finds the brace that closes an expression, passing over braces
// inside it and inside its strings.
func exprEnd(src string) (int, error) {
	depth := 0
	for i := 0; i < len(src); i++ {
		switch c := src[i]; c {
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return i, nil
			}
			depth--
		case '"', '\'', '`':
			for i++; i < len(src) && src[i] != c; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		}
	}
	return 0, errors.New("an expression is opened with ${ and never closed")
}

// IsOneExpr says whether a text is exactly one expression.
func IsOneExpr(text string) bool {
	_, ok := OneExpr(text)
	return ok
}

// OneExpr gives the source of a text that is exactly one expression.
func OneExpr(text string) (string, bool) {
	parts, err := split(text)
	if err != nil || len(parts) != 1 || !parts[0].expr {
		return "", false
	}
	return parts[0].text, true
}

// HasExpr says whether a text holds an expression.
func HasExpr(text string) bool { return strings.Contains(text, "${") }

// builtins are the names the language itself gives an expression.
var builtins = map[string]bool{}

func init() {
	for _, name := range strings.Fields(`undefined NaN Infinity Object Array String Number Boolean Math JSON Date
		RegExp Map Set Symbol BigInt Error TypeError RangeError parseInt parseFloat isNaN isFinite
		encodeURIComponent decodeURIComponent encodeURI decodeURI arguments`) {
		builtins[name] = true
	}
}

// compiled holds expressions already compiled, by their source.
var compiled sync.Map

// compile checks an expression and gives it as a program that makes a
// function of the scope's names. It is compiled, not only parsed, and an
// expression that names anything outside the language and its scope is
// refused.
func compile(src string, names []string) (*goja.Program, error) {
	key := strings.Join(names, ",") + "\x00" + src
	if p, ok := compiled.Load(key); ok {
		return p.(*goja.Program), nil
	}
	wrapped := "(function(" + strings.Join(names, ",") + "){\"use strict\";return (\n" + src + "\n)})"
	tree, err := parser.ParseFile(nil, "", wrapped, 0)
	if err != nil {
		return nil, mistake(err)
	}
	declared, used := map[string]bool{}, map[string]bool{}
	names2(reflect.ValueOf(tree.Body), declared, used, false)
	for name := range used {
		if !declared[name] && !builtins[name] {
			return nil, fmt.Errorf("it reads %q, which is not something an expression here can read (it can read: %s)",
				name, strings.Join(names, ", "))
		}
	}
	program, err := goja.CompileAST(tree, true)
	if err != nil {
		return nil, mistake(err)
	}
	compiled.Store(key, program)
	return program, nil
}

// mistake says what is wrong with how an expression is written, without
// where: the expression is read inside a function put round it, and a line
// and column of that are no place in the definition.
func mistake(err error) error {
	var read parser.ErrorList
	var syntax *goja.CompilerSyntaxError
	switch {
	case errors.As(err, &read) && len(read) > 0:
		return errors.New(oneLine(read[0].Message))
	case errors.As(err, &syntax):
		return errors.New(oneLine(syntax.Message))
	}
	return errors.New(oneLine(err.Error()))
}

var (
	identType   = reflect.TypeFor[*ast.Identifier]()
	bindingType = reflect.TypeFor[*ast.Binding]()
)

// names2 walks a syntax tree and notes every name it declares and every
// name it uses. binding says the walk is inside something that declares.
func names2(v reflect.Value, declared, used map[string]bool, binding bool) {
	switch v.Kind() {
	case reflect.Interface:
		if !v.IsNil() {
			names2(v.Elem(), declared, used, binding)
		}
	case reflect.Slice:
		for i := range v.Len() {
			names2(v.Index(i), declared, used, binding)
		}
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		switch node := v.Interface().(type) {
		case *ast.Identifier:
			if binding {
				declared[string(node.Name)] = true
			} else {
				used[string(node.Name)] = true
			}
		case *ast.Binding:
			names2(reflect.ValueOf(node.Target), declared, used, true)
			names2(reflect.ValueOf(node.Initializer), declared, used, false)
		case *ast.DotExpression:
			names2(reflect.ValueOf(node.Left), declared, used, false)
		case *ast.PropertyKeyed:
			if node.Computed {
				names2(reflect.ValueOf(node.Key), declared, used, false)
			}
			names2(reflect.ValueOf(node.Value), declared, used, binding)
		case *ast.PropertyShort:
			if binding {
				declared[string(node.Name.Name)] = true
			} else {
				used[string(node.Name.Name)] = true
			}
			names2(reflect.ValueOf(node.Initializer), declared, used, false)
		case *ast.FunctionLiteral:
			if node.Name != nil {
				declared[string(node.Name.Name)] = true
			}
			names2(reflect.ValueOf(node.ParameterList), declared, used, false)
			names2(reflect.ValueOf(node.Body), declared, used, false)
		case *ast.ParameterList:
			names2(reflect.ValueOf(node.List), declared, used, false)
			names2(reflect.ValueOf(node.Rest), declared, used, true)
		case *ast.CatchStatement:
			names2(reflect.ValueOf(node.Parameter), declared, used, true)
			names2(reflect.ValueOf(node.Body), declared, used, false)
		case *ast.ForDeclaration:
			names2(reflect.ValueOf(node.Target), declared, used, true)
		case *ast.LabelledStatement:
			names2(reflect.ValueOf(node.Statement), declared, used, false)
		case *ast.BranchStatement, *ast.MetaProperty:
		default:
			names2(v.Elem(), declared, used, binding)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			f := v.Field(i)
			if f.CanInterface() {
				names2(f, declared, used, binding)
			}
		}
	}
}

// CheckExpr says what is wrong with an expression that may read the names.
func CheckExpr(src string, names []string) error {
	_, err := compile(src, names)
	return err
}

// CheckText says what is wrong with the expressions in a text.
func CheckText(text string, names []string) error {
	parts, err := split(text)
	if err != nil {
		return err
	}
	for _, p := range parts {
		if p.expr {
			if err := CheckExpr(p.text, names); err != nil {
				return fmt.Errorf("in ${%s}: %w", oneLine(p.text), err)
			}
		}
	}
	return nil
}

// A Scope is what the expressions of one place read, by name.
type Scope map[string]any

// An Evaluator fills in the expressions of one step. The time expressions
// see is the time of the step, and their random numbers follow from the
// step's seed: taken again, the step sees the same.
type Evaluator struct {
	time  time.Time
	seed  []byte
	count uint64
	vm    *goja.Runtime
	once  map[string]goja.Value // scope values already given to the engine
	prep  goja.Callable
}

// NewEvaluator makes the evaluator of a step.
func NewEvaluator(at time.Time, seed []byte) *Evaluator {
	return &Evaluator{time: at, seed: seed}
}

// freeze makes the engine's own objects, and a value given to it, unable to
// change, so one expression cannot alter what the next one sees.
const prepSrc = `(function(globalThis){
	"use strict";
	const deep = (v) => { if (v !== null && typeof v === "object" && !Object.isFrozen(v)) {
		Object.freeze(v); for (const k of Object.keys(v)) deep(v[k]) } return v };
	for (const name of Object.getOwnPropertyNames(globalThis)) {
		const v = globalThis[name];
		if (v !== null && (typeof v === "object" || typeof v === "function")) {
			Object.freeze(v); if (v.prototype) Object.freeze(v.prototype);
		}
	}
	Object.freeze(Object.getPrototypeOf(function(){}));
	Object.freeze(globalThis);
	return (text) => deep(JSON.parse(text));
})`

var prepProgram = goja.MustCompile("", prepSrc, true)

func (e *Evaluator) start() error {
	if e.vm != nil {
		return nil
	}
	vm := goja.New()
	vm.SetMaxCallStackSize(200)
	vm.SetTimeSource(func() time.Time { return e.time })
	vm.SetRandSource(e.random)
	for _, name := range []string{"eval", "Function", "Proxy", "Reflect", "WeakRef", "FinalizationRegistry", "Promise", "globalThis"} {
		vm.GlobalObject().Delete(name)
	}
	v, err := vm.RunProgram(prepProgram)
	if err != nil {
		return err
	}
	prep, _ := goja.AssertFunction(v)
	if v, err = prep(goja.Undefined(), vm.GlobalObject()); err != nil {
		return err
	}
	e.prep, _ = goja.AssertFunction(v)
	e.vm, e.once = vm, map[string]goja.Value{}
	return nil
}

// random gives the random numbers of the expression now running: they
// differ from one expression to the next and are the same if the step is
// taken again.
func (e *Evaluator) random() float64 {
	e.count++
	mac := hmac.New(sha256.New, e.seed)
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], e.count)
	mac.Write([]byte("random\x00"))
	mac.Write(n[:])
	return float64(binary.BigEndian.Uint64(mac.Sum(nil))>>11) / (1 << 53)
}

// give hands a scope's value to the engine, once for each value.
func (e *Evaluator) give(name string, v any) (goja.Value, error) {
	text, err := json.Marshal(v, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	key := name + "\x00" + string(text)
	if value, ok := e.once[key]; ok {
		return value, nil
	}
	value, err := e.prep(goja.Undefined(), e.vm.ToValue(string(text)))
	if err != nil {
		return nil, err
	}
	e.once[key] = value
	return value, nil
}

// Eval gives the value of one expression in a scope.
func (e *Evaluator) Eval(src string, scope Scope) (any, error) {
	names := scopeNames(scope)
	program, err := compile(src, names)
	if err != nil {
		return nil, err
	}
	if err := e.start(); err != nil {
		return nil, err
	}
	args := make([]goja.Value, len(names))
	for i, name := range names {
		if args[i], err = e.give(name, scope[name]); err != nil {
			return nil, err
		}
	}
	// The limit covers the expression and the turning of what it gives, or
	// throws, into a value: both run the definition's own code.
	timer := time.AfterFunc(ExprLimit, func() { e.vm.Interrupt("an expression may run for one second") })
	text, err := e.run(program, args)
	timer.Stop()
	e.vm.ClearInterrupt()
	if err != nil {
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			// The engine may be anywhere: the next expression gets a new one.
			e.vm = nil
			return nil, fmt.Errorf("%v", interrupted.Value())
		}
		return nil, err
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (e *Evaluator) run(program *goja.Program, args []goja.Value) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	fn, err := e.vm.RunProgram(program)
	if err != nil {
		return "", err
	}
	call, _ := goja.AssertFunction(fn)
	result, err := call(goja.Undefined(), args...)
	if err == nil {
		stringify, _ := goja.AssertFunction(e.vm.Get("JSON").ToObject(e.vm).Get("stringify"))
		var out goja.Value
		if out, err = stringify(goja.Undefined(), result); err == nil {
			if goja.IsUndefined(out) {
				return "null", nil
			}
			return out.String(), nil
		}
	}
	var thrown *goja.Exception
	if errors.As(err, &thrown) {
		// The text of what was thrown, made inside the limit.
		message := thrown.Value().String()
		return "", errors.New(oneLine(message))
	}
	var overflow *goja.StackOverflowError
	if errors.As(err, &overflow) {
		return "", errors.New("it calls itself without end")
	}
	return "", err
}

func scopeNames(scope Scope) []string {
	names := make([]string, 0, len(scope))
	for name := range scope {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Fill gives a value with every expression in it filled in. Place says
// where in the definition the value is, for an error to name.
func (e *Evaluator) Fill(value any, scope Scope, place string) (any, error) {
	switch v := value.(type) {
	case string:
		return e.Text(v, scope, place)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			filled, err := e.Fill(item, scope, place+"."+k)
			if err != nil {
				return nil, err
			}
			out[k] = filled
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			filled, err := e.Fill(item, scope, fmt.Sprintf("%s[%d]", place, i))
			if err != nil {
				return nil, err
			}
			out[i] = filled
		}
		return out, nil
	}
	return value, nil
}

// Text fills in one text: the value of its one expression, or text with
// the values of its expressions in it.
func (e *Evaluator) Text(text string, scope Scope, place string) (any, error) {
	if !HasExpr(text) {
		return text, nil
	}
	parts, err := split(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", place, err)
	}
	var b strings.Builder
	for _, p := range parts {
		if !p.expr {
			b.WriteString(p.text)
			continue
		}
		v, err := e.Eval(p.text, scope)
		if err != nil {
			return nil, fmt.Errorf("%s: ${%s}: %w", place, oneLine(p.text), err)
		}
		if len(parts) == 1 {
			return v, nil
		}
		if s, ok := v.(string); ok {
			b.WriteString(s)
		} else if v != nil {
			shown, _ := json.Marshal(v, json.Deterministic(true))
			b.Write(shown)
		}
	}
	return b.String(), nil
}

// Cond says whether a condition, a bare expression, holds.
func (e *Evaluator) Cond(src string, scope Scope, place string) (bool, error) {
	v, err := e.Eval(src, scope)
	if err != nil {
		return false, fmt.Errorf("%s: %s: %w", place, oneLine(src), err)
	}
	return truthy(v), nil
}

func truthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0 && v == v
	case string:
		return v != ""
	}
	return true
}

// oneLine makes a text fit on one line.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// A Read is one thing an expression reads of a scope's name that holds
// named things, as stages holds a flow's stages: the thing's name, and
// which part of it.
type Read struct {
	Name string
	Part string
}

// Reads lists what the expressions of a text read of one of the scope's
// names. However a reference is spelt, with a dot or with brackets, it is
// found. A thing named by a computed value is given the name "*".
func Reads(text, root string) []Read {
	parts, err := split(text)
	if err != nil {
		return nil
	}
	var reads []Read
	for _, p := range parts {
		if p.expr {
			reads = append(reads, ReadsOf(p.text, root)...)
		}
	}
	return reads
}

// ReadsOf is Reads for one bare expression.
func ReadsOf(src, root string) []Read {
	tree, err := parser.ParseFile(nil, "", "("+src+"\n)", 0)
	if err != nil {
		return nil
	}
	var reads []Read
	findReads(reflect.ValueOf(tree.Body), root, &reads)
	return reads
}

// member says whether a node is root.name or root["name"], and gives the
// name.
func member(node any, root string) (string, bool) {
	switch n := node.(type) {
	case *ast.Optional:
		return member(n.Expression, root)
	case *ast.DotExpression:
		if id, ok := n.Left.(*ast.Identifier); ok && string(id.Name) == root {
			return string(n.Identifier.Name), true
		}
	case *ast.BracketExpression:
		if id, ok := n.Left.(*ast.Identifier); ok && string(id.Name) == root {
			if lit, ok := n.Member.(*ast.StringLiteral); ok {
				return string(lit.Value), true
			}
			return "*", true
		}
	}
	return "", false
}

func findReads(v reflect.Value, root string, reads *[]Read) {
	switch v.Kind() {
	case reflect.Interface:
		if !v.IsNil() {
			findReads(v.Elem(), root, reads)
		}
	case reflect.Slice:
		for i := range v.Len() {
			findReads(v.Index(i), root, reads)
		}
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		node := v.Interface()
		if _, isOptional := node.(*ast.Optional); isOptional {
			findReads(v.Elem(), root, reads)
			return
		}
		if name, ok := member(node, root); ok {
			*reads = append(*reads, Read{Name: name})
			if b, isBracket := node.(*ast.BracketExpression); isBracket {
				findReads(reflect.ValueOf(b.Member), root, reads)
			}
			return
		}
		switch n := node.(type) {
		case *ast.DotExpression:
			if name, ok := member(n.Left, root); ok {
				*reads = append(*reads, Read{Name: name, Part: string(n.Identifier.Name)})
				return
			}
			findReads(reflect.ValueOf(n.Left), root, reads)
			return
		case *ast.BracketExpression:
			if name, ok := member(n.Left, root); ok {
				part := "*"
				if lit, isLit := n.Member.(*ast.StringLiteral); isLit {
					part = string(lit.Value)
				}
				*reads = append(*reads, Read{Name: name, Part: part})
				findReads(reflect.ValueOf(n.Member), root, reads)
				return
			}
		case *ast.OptionalChain:
			findReads(reflect.ValueOf(n.Expression), root, reads)
			return
		}
		findReads(v.Elem(), root, reads)
	case reflect.Struct:
		for i := range v.NumField() {
			if f := v.Field(i); f.CanInterface() {
				findReads(f, root, reads)
			}
		}
	}
}

// ReadsIn lists what every expression in a value reads of a scope's name.
func ReadsIn(value any, root string) []Read {
	var reads []Read
	switch v := value.(type) {
	case string:
		reads = Reads(v, root)
	case map[string]any:
		for _, item := range v {
			reads = append(reads, ReadsIn(item, root)...)
		}
	case []any:
		for _, item := range v {
			reads = append(reads, ReadsIn(item, root)...)
		}
	}
	return reads
}

// Duration fills in a length of time from a scope's names. Empty text
// gives none.
func (e *Evaluator) Duration(text string, scope Scope, place string) (Duration, error) {
	if text == "" {
		return 0, nil
	}
	filled, err := e.Text(text, scope, place)
	if err != nil {
		return 0, err
	}
	written, ok := filled.(string)
	if !ok {
		return 0, fmt.Errorf("%s: %s is %s, not a length of time (write it like 30s, 5m, 48h or 7d)", place, text, describe(filled))
	}
	d, err := ParseDuration(written)
	if err != nil {
		return 0, fmt.Errorf("%s: %v", place, err)
	}
	return d, nil
}

// Entry fills in a permission's scope from a scope's names. What fills it
// must be there, and be text that is not empty or a number: an empty value
// would turn ${input.dir}/** into /**, the whole disk, and a value that
// itself holds ${ would be read a second time.
func (e *Evaluator) Entry(entry string, scope Scope, place string) (string, error) {
	if !HasExpr(entry) {
		return entry, nil
	}
	parts, err := split(entry)
	if err != nil {
		return "", fmt.Errorf("%s: %w", place, err)
	}
	var b strings.Builder
	for _, p := range parts {
		if !p.expr {
			b.WriteString(p.text)
			continue
		}
		v, err := e.Eval(p.text, scope)
		if err != nil {
			return "", fmt.Errorf("%s: ${%s}: %w", place, oneLine(p.text), err)
		}
		text := ""
		switch v := v.(type) {
		case string:
			text = v
		case float64:
			text = strconv.FormatFloat(v, 'f', -1, 64)
		default:
			return "", fmt.Errorf("%s: the permission %s cannot be filled: ${%s} is not text or a number", place, entry, oneLine(p.text))
		}
		if text == "" || HasExpr(text) {
			return "", fmt.Errorf("%s: the permission %s cannot be filled: ${%s} is empty, or itself holds an expression", place, entry, oneLine(p.text))
		}
		b.WriteString(text)
	}
	return b.String(), nil
}
