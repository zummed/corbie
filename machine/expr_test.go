package machine

import (
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

var stepTime = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func eval(t *testing.T) *Evaluator { return NewEvaluator(stepTime, []byte("seed"), Run{}) }

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

// The fields of a mapping are filled in the order of their names, so the
// step taken again gives each field the same random number and fails at
// the same field.
func TestFillOrderIsFixed(t *testing.T) {
	scope := Scope{"input": nil}
	draws := map[string]any{"a": "${Math.random()}", "b": "${Math.random()}", "c": "${Math.random()}", "d": "${Math.random()}"}
	first, err := eval(t).Fill(draws, scope, "input")
	if err != nil {
		t.Fatal(err)
	}
	fails := map[string]any{"a": "${input.x.y}", "b": "${input.x.y}", "c": "${input.x.y}", "d": "${input.x.y}"}
	_, firstErr := eval(t).Fill(fails, scope, "input")
	for range 20 {
		again, err := eval(t).Fill(draws, scope, "input")
		if err != nil || !reflect.DeepEqual(again, first) {
			t.Fatalf("the step taken again filled %v, not %v (%v)", again, first, err)
		}
		if _, err := eval(t).Fill(fails, scope, "input"); err == nil || err.Error() != firstErr.Error() {
			t.Fatalf("the step taken again failed with %v, not %v", err, firstErr)
		}
	}
	if !strings.Contains(firstErr.Error(), "input.a") {
		t.Fatalf("the fill failed with %v, not at input.a", firstErr)
	}
}

// An expression on a computer set to a zone nine hours east reads the
// step's time in UTC. The test runs itself again in that zone.
func TestLocalTimeIsUTC(t *testing.T) {
	if os.Getenv("TZ") != "Asia/Tokyo" {
		again := exec.Command(os.Args[0], "-test.run=^TestLocalTimeIsUTC$")
		again.Env = append(os.Environ(), "TZ=Asia/Tokyo")
		if out, err := again.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	got, err := eval(t).Eval("[new Date().getHours(), new Date().getTimezoneOffset()]", Scope{"input": nil})
	if err != nil || !reflect.DeepEqual(got, []any{float64(12), float64(0)}) {
		t.Fatalf("an expression in Tokyo at 12:00 UTC reads %v, %v", got, err)
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

func Test_mach_expression_value_that_is_not_data_fails(t *testing.T) {
	e := eval(t)
	scope := Scope{"input": map[string]any{"items": []any{map[string]any{"price": 4.0, "qty": 2.0}, map[string]any{"price": 3.0}}}}
	for text, want := range map[string]string{
		"${input.items.reduce((s, i) => s + i.price * i.qty, 0)}": "stages.total.job.input.total: ${input.items.reduce((s, i) => s + i.price * i.qty, 0)}: NaN is not data",
		"${ {a: [1, 1 / 0]} }":  "stages.total.job.input.total: ${{a: [1, 1 / 0]}}: Infinity is not data",
		"n is ${-1 / 0}":        "stages.total.job.input.total: ${-1 / 0}: -Infinity is not data",
		"${10n}":                "stages.total.job.input.total: ${10n}: a BigInt is not data",
		"${input.n > 0 || NaN}": "stages.total.job.input.total: ${input.n > 0 || NaN}: NaN is not data",
		"${ {f: () => 1} }":     "stages.total.job.input.total: ${{f: () => 1}}: a function is not data",
		"${[Math.max]}":         "stages.total.job.input.total: ${[Math.max]}: a function is not data",
		"${Symbol('x')}":        "stages.total.job.input.total: ${Symbol('x')}: a Symbol is not data",
		"${ {s: Symbol()} }":    "stages.total.job.input.total: ${{s: Symbol()}}: a Symbol is not data",
	} {
		if got, err := e.Fill(text, scope, "stages.total.job.input.total"); err == nil || err.Error() != want {
			t.Errorf("%s gave %v (%v)\nwant: %s", text, got, err, want)
		}
	}
	for text, want := range map[string]any{
		"${input.missing}":                    nil,
		"${ {u: input.missing, n: 1} }":       map[string]any{"u": nil, "n": 1.0},
		"${[undefined, {a: {b: undefined}}]}": []any{nil, map[string]any{"a": map[string]any{"b": nil}}},
	} {
		if got, err := e.Fill(text, scope, "place"); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s gave %v (%v), want %v", text, got, err, want)
		}
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

func Test_mach_step_reports_every_failing_expression(t *testing.T) {
	spec := &JobSpec{Script: "./a.js", Timeout: "${input.t.s}", Input: map[string]any{
		"y": "${input.c.d}", "x": "${input.a.b}", "ok": "${input.n}",
		"list": []any{"${1n}", "at ${input.e.f} and ${input.g.h}"}}}
	want := strings.Join([]string{
		`stages.s.job.input.list[0]: ${1n}: a BigInt is not data`,
		`stages.s.job.input.list[1]: ${input.e.f}: TypeError: Cannot read property 'f' of undefined`,
		`stages.s.job.input.list[1]: ${input.g.h}: TypeError: Cannot read property 'h' of undefined`,
		`stages.s.job.input.x: ${input.a.b}: TypeError: Cannot read property 'b' of undefined`,
		`stages.s.job.input.y: ${input.c.d}: TypeError: Cannot read property 'd' of undefined`,
		`stages.s.job.timeout: ${input.t.s}: TypeError: Cannot read property 's' of undefined`,
	}, "\n")
	for range 5 {
		_, err := spec.Fill(eval(t), Scope{"input": map[string]any{"n": 1.0}}, "stages.s.job", "s")
		if err == nil || err.Error() != want {
			t.Fatalf("the job's error reads\n%v\nwant\n%s", err, want)
		}
	}
}
