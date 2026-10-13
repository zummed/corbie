package machine

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

var stepTime = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func eval(t *testing.T) *Evaluator { return NewEvaluator(stepTime, []byte("seed")) }

func TestValues(t *testing.T) {
	e := eval(t)
	scope := Scope{"input": map[string]any{"n": 3.0, "list": []any{1.0, 2.0}, "topic": "the disk",
		"urls": []any{"https://a", "http://b", "https://c"}}}
	for _, c := range []struct {
		value any
		want  any
	}{
		{"${input.n}", 3.0},
		{"${input.list}", []any{1.0, 2.0}},
		{"Look into ${input.topic}.", "Look into the disk."},
		{"n is ${input.n} and list is ${input.list}", "n is 3 and list is [1,2]"},
		{3.0, 3.0},
		{"hello", "hello"},
		{map[string]any{"a": map[string]any{"b": "${input.n + 1}"}, "c": []any{"x", "${input.topic}"}},
			map[string]any{"a": map[string]any{"b": 4.0}, "c": []any{"x", "the disk"}}},
		{`${input.urls.filter(u => u.startsWith("https")).length}`, 2.0},
		{"${input.missing}", nil},
		{"${ {a: 1, b: {c: [1,2]}} }", map[string]any{"a": 1.0, "b": map[string]any{"c": []any{1.0, 2.0}}}},
		{"${`a}b${input.n}`}", "a}b3"},
	} {
		got, err := e.Fill(c.value, scope, "place")
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v gave %#v (%v), want %#v", c.value, got, err, c.want)
		}
	}
}

func TestExprRefused(t *testing.T) {
	names := []string{"input"}
	for src, want := range map[string]string{
		"states.x.output":                       `reads "states"`,
		"require('fs')":                         `reads "require"`,
		"process.env":                           `reads "process"`,
		"input.a +":                             "",
		"(() => { let y = 1; return y + z })()": `reads "z"`,
	} {
		err := CheckExpr(src, names)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", src, err)
		}
	}
	for _, src := range []string{"input.a.b", "input.list.map(x => x * 2)", "Math.max(1, input.n)", "JSON.stringify(input)",
		"(() => { const {a, b: [c]} = input; return a + c })()", "input.items.reduce((sum, {n}) => sum + n, 0)",
		"new Date().toISOString()", "(function f(n) { return n ? f(n - 1) : 0 })(3)",
		"(() => { for (const x of input.l) { if (x) return x } })()", "(() => { try { return 1 } catch (e) { return e } })()"} {
		if err := CheckExpr(src, names); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}

// A mistake in how an expression is written is reported with no line and
// column: the only ones known are of the function the expression is read
// inside, and no line of the definition.
func TestMistakeInAnExpressionGivesNoPlaceThatMeansNothing(t *testing.T) {
	for text, want := range map[string]string{
		"${input.n +}":      "in ${input.n +}: Unexpected token )",
		"n is ${input.a b}": "in ${input.a b}: Unexpected identifier",
		"${(() => { const a = 1; const a = 2 })()}": "in ${(() => { const a = 1; const a = 2 })()}: Identifier 'a' has already been declared",
	} {
		if err := CheckText(text, []string{"input"}); err == nil || err.Error() != want {
			t.Errorf("%s: %v\nwant: %s", text, err, want)
		}
	}
}

func Test_mach_expression_cannot_alter_shared_names(t *testing.T) {
	e := eval(t)
	scope := Scope{"input": map[string]any{"list": []any{1.0}, "o": map[string]any{"a": 1.0}}}
	for _, src := range []string{"input.list.push(2)", "(input.o.a = 5)", "(Array.prototype.x = 1)", "(Object.prototype.y = 2)",
		"(JSON.z = 3)", "(Math.random = () => 1)", "delete input.o.a"} {
		if _, err := e.Eval(src, scope); err == nil {
			t.Errorf("%s was allowed", src)
		}
	}
	got, err := e.Eval("[input.list.length, input.o.a, [].x, ({}).y, JSON.z]", scope)
	if err != nil || !reflect.DeepEqual(got, []any{1.0, 1.0, nil, nil, nil}) {
		t.Fatalf("after the attempts an expression sees %v (%v)", got, err)
	}
}

func TestTimeAndRandom(t *testing.T) {
	a, b := eval(t), eval(t)
	scope := Scope{"input": nil}
	t1, _ := a.Eval("Date.now()", scope)
	t2, _ := a.Eval("new Date().getTime()", scope)
	if t1 != float64(stepTime.UnixMilli()) || t1 != t2 {
		t.Fatalf("the time an expression sees is %v and %v", t1, t2)
	}
	r1, _ := a.Eval("Math.random()", scope)
	r2, _ := a.Eval("Math.random()", scope)
	if r1 == r2 {
		t.Fatal("two expressions got the same random number")
	}
	// The step taken again sees the same.
	b.Eval("Date.now()", scope)
	b.Eval("new Date().getTime()", scope)
	s1, _ := b.Eval("Math.random()", scope)
	s2, _ := b.Eval("Math.random()", scope)
	if r1 != s1 || r2 != s2 {
		t.Fatal("the step taken again got other random numbers")
	}
}

func Test_mach_expression_timeout_stops_one_expression(t *testing.T) {
	e := eval(t)
	scope := Scope{"input": map[string]any{"n": 1.0}}
	start := time.Now()
	_, err := e.Eval("(() => { for (;;) {} })()", scope)
	if err == nil || !strings.Contains(err.Error(), "one second") {
		t.Fatalf("an endless expression: %v", err)
	}
	if took := time.Since(start); took < ExprLimit || took > 3*ExprLimit {
		t.Fatalf("it ran for %v", took)
	}
	if v, err := e.Eval("input.n + 1", scope); err != nil || v != 2.0 {
		t.Fatalf("the next expression: %v %v", v, err)
	}
}

func Test_mach_thrown_value_is_shown_within_the_limit(t *testing.T) {
	e := eval(t)
	start := time.Now()
	_, err := e.Eval("(() => { throw { toString() { for (;;) {} } } })()", Scope{"input": nil})
	if err == nil {
		t.Fatal("a thrown value was not an error")
	}
	if took := time.Since(start); took > 3*ExprLimit {
		t.Fatalf("showing a thrown value ran for %v", took)
	}
	_, err = e.Eval("(() => { throw new Error('went\\nwrong') })()", Scope{"input": nil})
	if err == nil || !strings.Contains(err.Error(), "went wrong") {
		t.Fatalf("a thrown error reads %v", err)
	}
}

func Test_mach_expression_depth_is_bounded(t *testing.T) {
	e := eval(t)
	_, err := e.Eval("(function f() { return f() })()", Scope{"input": nil})
	if err == nil || !strings.Contains(err.Error(), "without end") {
		t.Fatalf("endless recursion: %v", err)
	}
	if v, err := e.Eval("1 + 1", Scope{"input": nil}); err != nil || v != 2.0 {
		t.Fatalf("the next expression: %v %v", v, err)
	}
}

func Test_flow_reference_spellings_draw_the_same_arrow(t *testing.T) {
	for src, want := range map[string][]Read{
		"stages.a.output.n + 1":                   {{"a", "output"}},
		`stages["a"].output`:                      {{"a", "output"}},
		`stages['a']["output"]`:                   {{"a", "output"}},
		"stages.a?.output":                        {{"a", "output"}},
		"stages.a":                                {{"a", ""}},
		"stages.a.failed ? stages.b.error : 0":    {{"a", "failed"}, {"b", "error"}},
		"input.list.map(x => stages.c.output[x])": {{"c", "output"}},
		"stages[input.which].output":              {{"*", "output"}},
		"input.stages.a":                          nil,
	} {
		if got := ReadsOf(src, "stages"); !reflect.DeepEqual(got, want) {
			t.Errorf("%s reads %v, want %v", src, got, want)
		}
	}
	if got := Reads("x ${stages.a.output} y ${stages.b.skipped}", "stages"); len(got) != 2 {
		t.Errorf("a text with two expressions reads %v", got)
	}
}

func Test_perm_filled_scope_refuses_missing_empty_nontext_nested(t *testing.T) {
	e := eval(t)
	scope := Scope{"input": map[string]any{"dir": "/srv/reports", "n": 5.0, "empty": "", "list": []any{"a"}, "nested": "${input.dir}"}}
	if got, err := e.Entry("fs.read:${input.dir}/**", scope, "allow[0]"); err != nil || got != "fs.read:/srv/reports/**" {
		t.Fatalf("a filled scope: %q %v", got, err)
	}
	if got, err := e.Entry("net:host:${input.n}", scope, "allow[0]"); err != nil || got != "net:host:5" {
		t.Fatalf("a scope filled from a number: %q %v", got, err)
	}
	for _, entry := range []string{"fs.read:${input.missing}/**", "fs.read:${input.empty}/**", "fs.read:${input.list}/**", "fs.read:${input.nested}/**"} {
		if got, err := e.Entry(entry, scope, "allow[0]"); err == nil || !strings.Contains(err.Error(), entry) {
			t.Errorf("%s was filled as %q (%v)", entry, got, err)
		}
	}
}
