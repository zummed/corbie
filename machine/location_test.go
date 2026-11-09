package machine

import (
	"encoding/json/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// listener is a probe whose runs take events sent to them.
type listener struct{ probe }

func (listener) Name() string { return "listener" }
func (listener) Receives()    {}

func init() { Register(listener{}) }

func unmarshalBody(def *Definition, into any) error { return json.Unmarshal(def.Body, into) }

func parsed(t *testing.T, src string, contents map[string]string) *Draft {
	t.Helper()
	d, err := Parse([]byte(src), files(contents))
	if err != nil {
		t.Fatalf("the definition does not pass:\n%s\n%v", src, err)
	}
	return d
}

// A file says how runs start at its location, transient by default, and only
// a type whose runs take sent events may be keyed. A machine written in
// place or made while running is no file at a location, and says none of it.
func Test_mach_a_file_says_how_its_runs_start(t *testing.T) {
	js := map[string]string{"a.js": ""}
	for src, want := range map[string][2]string{
		"probe: p\n":                      {LifetimeTransient, UpdateDrain},
		"probe: p\nlifetime: singleton\n": {LifetimeSingleton, UpdateRestart},
		"listener: l\nlifetime: keyed\n":  {LifetimeKeyed, UpdateDrain},
		"probe: p\nupdate: restart\n":     {LifetimeTransient, UpdateRestart},
	} {
		d := parsed(t, src, js)
		if got := [2]string{d.Definition.Lifetime, d.Definition.Update}; got != want {
			t.Errorf("%q is stored as %v", src, got)
		}
	}
	for src, want := range map[string]string{
		"probe: p\nlifetime: keyed\n":   `lifetime: a probe takes no events sent to it, so it cannot be keyed`,
		"probe: p\nlifetime: forever\n": `lifetime: "forever": lifetime is singleton, keyed or transient`,
		"probe: p\nwork:\n  a: { machine: { probe: inner, lifetime: singleton, receive: [x] } }\n": "lifetime: lifetime belongs to a file at a location",
	} {
		if got := problems(t, src, js); !strings.Contains(got, want) {
			t.Errorf("%q gave:\n%s", src, got)
		}
	}
	if _, err := ParseMade([]byte("probe: p\nlifetime: singleton\nlimits: { runs: 2 }\n"), files(nil)); err == nil || !strings.Contains(err.Error(), "limits.runs: runs belongs to a file") {
		t.Errorf("a machine made while running gave: %v", err)
	}
	d := parsed(t, "probe: p\nwork:\n  a: { machine: { probe: inner } }\n", js)
	for _, child := range d.Children {
		if child.Definition.Lifetime != "" || child.Definition.Update != "" {
			t.Errorf("a machine written in place is stored with %q and %q", child.Definition.Lifetime, child.Definition.Update)
		}
	}
}

// A singleton always restarts.
func Test_mach_update_says_what_a_change_does(t *testing.T) {
	if got := problems(t, "probe: p\nlifetime: singleton\nupdate: drain\n", nil); !strings.Contains(got, "update: a singleton always restarts, and cannot drain") {
		t.Errorf("a singleton that drains gave:\n%s", got)
	}
	if got := problems(t, "probe: p\nupdate: later\n", nil); !strings.Contains(got, `update: "later": update is restart or drain`) {
		t.Errorf("an update that is not a word it takes gave:\n%s", got)
	}
}

// limits: { runs: } caps the runs of a keyed or a transient file.
func Test_mach_runs_limits_a_keyed_or_transient_file(t *testing.T) {
	for _, src := range []string{"listener: l\nlifetime: keyed\nlimits: { runs: 50 }\n", "probe: p\nlimits: { runs: 50 }\n"} {
		if d := parsed(t, src, nil); d.Definition.Runs != 50 {
			t.Errorf("%q: runs is stored as %d", src, d.Definition.Runs)
		}
	}
	for src, want := range map[string]string{
		"probe: p\nlifetime: singleton\nlimits: { runs: 5 }\n": "limits.runs: runs limits the runs of a keyed or transient file, and a singleton has one run",
		"listener: l\nlifetime: keyed\nlimits: { runs: 0 }\n":  "limits.runs: runs is at least 1",
	} {
		if got := problems(t, src, nil); !strings.Contains(got, want) {
			t.Errorf("%q gave:\n%s", src, got)
		}
	}
}

// A runs that is no whole number is reported once, as that.
func Test_mach_runs_not_whole_reported_once(t *testing.T) {
	got := problems(t, "listener: l\nlifetime: keyed\nlimits: { runs: 1.5 }\n", nil)
	if !strings.Contains(got, "limits.runs: this should be a whole number") || strings.Contains(got, "at least 1") {
		t.Errorf("runs: 1.5 gave:\n%s", got)
	}
}

// A singleton is given no input: one whose input needs a value is refused.
func Test_mach_singleton_needing_input_refused(t *testing.T) {
	got := problems(t, "probe: p\nlifetime: singleton\ninput: { name: string }\n", nil)
	if !strings.Contains(got, "input: a singleton is given no input, and this input needs some") {
		t.Errorf("a singleton needing input gave:\n%s", got)
	}
	for _, src := range []string{
		"probe: p\nlifetime: singleton\ninput:\n  name: string?\n",
		"probe: p\nlifetime: singleton\ninput: { type: object, properties: { days: { type: number, default: 3 } }, required: [days] }\n",
		"probe: p\ninput: { name: string }\n",
	} {
		parsed(t, src, nil)
	}
}

// receive: is a list for every event, or a mapping from an event's name to a
// list; an entry is a location pattern or client:<name>.
func Test_mach_receive_says_who_may_send(t *testing.T) {
	d := parsed(t, "probe: p\nreceive: [thermostats/*, /work/**, client:panel]\n", nil)
	if got := d.Definition.Receive[""]; !slices.Equal(got, []string{"thermostats/*", "/work/**", "client:panel"}) {
		t.Errorf("a list is stored as %v", d.Definition.Receive)
	}
	d = parsed(t, "probe: p\nreceive: { set: [thermostats/*, client:panel], stop: client:panel }\n", nil)
	if got := d.Definition.Receive; len(got) != 2 || !slices.Equal(got["stop"], []string{"client:panel"}) {
		t.Errorf("a mapping is stored as %v", got)
	}
	// A receive: written empty, in either form, accepts nobody, and is
	// stored apart from none.
	for _, written := range []string{"[]", "{}"} {
		d = parsed(t, "probe: p\nreceive: "+written+"\n", nil)
		if d.Definition.Receive == nil || len(d.Definition.Receive[""]) != 0 {
			t.Errorf("receive: %s is stored as %#v", written, d.Definition.Receive)
		}
	}
	if d := parsed(t, "probe: p\n", nil); d.Definition.Receive != nil {
		t.Errorf("no receive: is stored as %#v", d.Definition.Receive)
	}
	got := problems(t, "probe: p\nreceive:\n  set: [\"${input.who}\", \"client:\", ../up, a//b, \"x#y\", +/z]\n  done: [a]\n", nil)
	for _, want := range []string{
		`receive.set[0]: "${input.who}": a receive: entry is written out`,
		"receive.set[1]: client: names a client",
		`receive.set[2]: "../up": a location is read from the tree's prefix`,
		`receive.set[3]: "a//b": a location has no empty name`,
		`receive.set[4]: "x#y": a name in a location is not + and holds no #, backslash or control character`,
		`receive.set[5]: "+/z"`,
		`receive.done: "done": no run is sent an event named`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the mistake %q is not said in:\n%s", want, got)
		}
	}
}

// A file's read: names the clients that read its runs' records, each as
// client:<name>, written out, each once. A machine written in place or made while
// running has no location, and refuses it.
func Test_mach_read_names_clients(t *testing.T) {
	d := parsed(t, "probe: p\nread: [client:wall, client:phone]\n", nil)
	if got := d.Definition.Read; !slices.Equal(got, []string{"client:wall", "client:phone"}) {
		t.Errorf("read: is stored as %v", got)
	}
	got := problems(t, "probe: p\nread: [wall, \"client:\", \"client:${input.who}\", client:wall, client:wall]\nwork:\n  a: { machine: { probe: inner, read: [client:wall] } }\n", nil)
	for _, want := range []string{
		`read[0]: "wall": read: names clients, as client:wall`,
		"read[1]: client: names a client",
		`read[2]: "client:${input.who}": a read: entry is written out`,
		`read[4]: "client:wall" is listed twice`,
		"read: read belongs to a file at a location",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the mistake %q is not said in:\n%s", want, got)
		}
	}
	if _, err := ParseMade([]byte("probe: p\nread: [client:wall]\n"), files(nil)); err == nil || !strings.Contains(err.Error(), "read belongs to a file") {
		t.Errorf("a machine made while running gave: %v", err)
	}
}

// An address is { at }, { at, key } or { at, run }, or one expression that
// gives one. A location written out is checked with the definition, and one
// filled in when it is filled.
func Test_mach_address_names_a_run(t *testing.T) {
	kitchen, empty, long, k := "kitchen", "", strings.Repeat("k", KeyLimit+1), "k"
	r1, r2 := "c1/20261010T120000.000Z-0000000001", "c1/20261010T120000.000Z-0000000002"
	for _, c := range []struct {
		to    string
		scope Scope
		want  Address
	}{
		{"{ at: devices/boiler }", nil, Address{At: "devices/boiler"}},
		{`{ at: rooms/thermostat, key: "${input.room}" }`, Scope{"input": map[string]any{"room": "kitchen"}}, Address{At: "rooms/thermostat", Key: &kitchen}},
		// A key filled in empty or too long is kept, so its send is
		// undelivered and fails nothing; a key that is no text is empty.
		{`{ at: rooms/thermostat, key: "${input.room}" }`, Scope{"input": map[string]any{"room": ""}}, Address{At: "rooms/thermostat", Key: &empty}},
		{`{ at: rooms/thermostat, key: "${input.room}" }`, Scope{"input": map[string]any{"room": map[string]any{}}}, Address{At: "rooms/thermostat", Key: &empty}},
		{`{ at: rooms/thermostat, key: "${input.room}" }`, Scope{"input": map[string]any{"room": long}}, Address{At: "rooms/thermostat", Key: &long}},
		{`{ at: "/home/${input.place}", run: "${input.id}" }`, Scope{"input": map[string]any{"place": "boiler", "id": r1}}, Address{At: "/home/boiler", Run: r1}},
		{`"${input.from}"`, Scope{"input": map[string]any{"from": map[string]any{"at": "/home/x", "run": r2, "key": "k"}}}, Address{At: "/home/x", Run: r2, Key: &k}},
		{`"${input.from}"`, Scope{"input": map[string]any{"from": map[string]any{"at": "/home/x", "key": ""}}}, Address{At: "/home/x", Key: &empty}},
		{`"${input.from}"`, Scope{"input": map[string]any{"from": map[string]any{"client": "panel"}}}, Address{Client: "panel"}},
	} {
		d := parsed(t, "probe: p\nwork:\n  a: { send: { to: "+c.to+", event: set } }\n", nil)
		var body probeBody
		if err := unmarshalBody(d.Definition, &body); err != nil {
			t.Fatal(err)
		}
		scope := c.scope
		if scope == nil {
			scope = Scope{"input": map[string]any{}}
		}
		job, err := body.Jobs["a"].Fill(eval(t), scope, "work.a", "a")
		if err != nil {
			t.Errorf("%s: %v", c.to, err)
			continue
		}
		if !job.IsSend() || !reflect.DeepEqual(*job.Send.At, c.want) || job.Send.Event != "set" {
			t.Errorf("%s was filled as %+v", c.to, job.Send)
		}
	}
	got := problems(t, "probe: p\nwork:\n  a: { send: { to: { at: ../up }, event: e } }\n  b: { send: { to: { at: x, key: k, run: r }, event: e } }\n  c: { send: { to: \"to ${input.x}\", event: e } }\n  d: { send: { to: { key: k }, event: e } }\n  e: { send: { to: { at: x, run: ../../y }, event: e } }\n", nil)
	for _, want := range []string{
		`work.e.send.to.run: "../../y" is not the name of a run`,
		`work.a.send.to.at: "../up": a location is read from the tree's prefix`,
		"work.b.send.to: an address names a key or a run, not both",
		`work.c.send.to: "to ${input.x}": an address is { at }, { at, key } or { at, run }, or exactly one expression that gives one`,
		"work.d.send.to: an address names its location with at",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the mistake %q is not said in:\n%s", want, got)
		}
	}
	// One filled in is checked as it is filled.
	spec := &AddressSpec{At: "${input.at}"}
	for at, want := range map[string]string{"../x": "a location is read from the tree's prefix", "": "a location is not empty"} {
		if _, err := spec.Fill(eval(t), Scope{"input": map[string]any{"at": at}}, "work.a.send.to"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q filled in gave: %v", at, err)
		}
	}
	if _, err := (&AddressSpec{At: "x", Run: "${input.id}"}).Fill(eval(t), Scope{"input": map[string]any{"id": "c1/../y"}}, "to"); err == nil || !strings.Contains(err.Error(), "is not the name of a run") {
		t.Errorf("a run filled in as a path gave: %v", err)
	}
	if _, err := (&AddressSpec{Expr: "${input}"}).Fill(eval(t), Scope{"input": "x"}, "to"); err == nil || !strings.Contains(err.Error(), "an address is") {
		t.Errorf("an expression that gives text gave: %v", err)
	}
	if _, err := (&AddressSpec{Expr: "${input}"}).Fill(eval(t), Scope{"input": map[string]any{"at": "/x", "room": "k"}}, "to"); err == nil || !strings.Contains(err.Error(), "an address is") {
		t.Errorf("an expression that gives a field no address has gave: %v", err)
	}
	// A location holds no name the store cannot hold.
	for _, at := range []string{"/t/ro\\om", "/t/ro\x01om"} {
		if _, err := spec.Fill(eval(t), Scope{"input": map[string]any{"at": at}}, "to"); err == nil || !strings.Contains(err.Error(), "backslash or control character") {
			t.Errorf("%q filled in gave: %v", at, err)
		}
	}
	for loc, want := range map[string]string{"devices/boiler": "/home/devices/boiler", "/work/x": "/work/x"} {
		if got := Resolve("/home", loc); got != want {
			t.Errorf("%s from /home is %s", loc, got)
		}
	}
}

// Expressions and an allow entry's scope read the values set at the run's
// location by name. A value that is not set fails what reads it, naming the
// value and the location.
func Test_mach_values_are_read_by_name(t *testing.T) {
	d := parsed(t, "probe: p\nvalue: \"${values.GREETING} ${run.location}\"\nwork:\n  a: { script: ./a.js, input: \"${values['ROOM']}\", allow: [\"net:${values.HUB}:443\"] }\n", map[string]string{"a.js": ""})
	if got := Values(d.Definition); !slices.Equal(got, []string{"GREETING", "HUB", "ROOM"}) {
		t.Errorf("the definition reads the values %v", got)
	}
	withChild := parsed(t, "probe: p\nvalue: ${values.TOP}\nwork:\n  a: { machine: { probe: kid, value: \"${values.KID}\" } }\n", nil)
	if got := withChild.Values(); !slices.Equal(got, []string{"KID", "TOP"}) {
		t.Errorf("the draft and its child read the values %v", got)
	}
	var body probeBody
	unmarshalBody(d.Definition, &body)
	if got := body.Jobs["a"].Values(); !slices.Equal(got, []string{"HUB", "ROOM"}) {
		t.Errorf("the job reads the values %v", got)
	}
	run := Run{ID: "r1", Location: "/home/devices/boiler", Values: map[string]string{"HUB": "hub.lan", "ROOM": "kitchen", "GREETING": "hi"}}
	e := NewEvaluator(stepTime, []byte("seed"), run)
	job, err := body.Jobs["a"].Fill(e, Scope{"input": map[string]any{}}, "work.a", "a")
	if err != nil || job.Input != "kitchen" || job.Allow[0] != "net:hub.lan:443" {
		t.Errorf("the job was filled as %+v (%v)", job, err)
	}
	if v, err := e.Text(body.Value.(string), Scope{}, "value"); err != nil || v != "hi /home/devices/boiler" {
		t.Errorf("the value is %v (%v)", v, err)
	}
	delete(run.Values, "HUB")
	_, err = body.Jobs["a"].Fill(NewEvaluator(stepTime, []byte("seed"), run), Scope{"input": map[string]any{}}, "work.a", "a")
	if err == nil || !strings.Contains(err.Error(), "the value HUB is not set at /home/devices/boiler or above") {
		t.Errorf("a value not set gave: %v", err)
	}
	got := problems(t, "probe: p\nvalue: \"${values[input.name]}\"\nwork:\n  a: { script: ./a.js, input: \"${Object.keys(values)}\" }\n", map[string]string{"a.js": ""})
	for _, want := range []string{"value: in ${values[input.name]}: a value is read by its name", "work.a.input: in ${Object.keys(values)}: a value is read by its name, written out, as values.NAME, and values alone is not read"} {
		if !strings.Contains(got, want) {
			t.Errorf("the mistake %q is not said in:\n%s", want, got)
		}
	}
}

// A job may send an event, and then takes to, event and data, and approve,
// allow and deny, and nothing else. It is one of a script, a machine, a send
// or a call.
func Test_mach_a_job_sends(t *testing.T) {
	parsed(t, "probe: p\nwork:\n  a: { send: { to: { at: devices/boiler }, event: set, data: { target: 12 } }, approve: true, allow: [\"net:hub.lan\"] }\nask: ./ask.js\n", map[string]string{"ask.js": ""})
	got := problems(t, "probe: p\nwork:\n  a: { send: { to: { at: x }, event: e }, input: 1, timeout: 5s }\n  b: { send: { to: { at: x }, event: e }, script: ./a.js }\n  c: { send: { to: { at: x }, event: done } }\n  d: { send: { to: { at: x }, event: \"${input.e}\" } }\n", map[string]string{"a.js": ""})
	for _, want := range []string{
		"work.a: a send takes to, event and data, and approve, allow and deny: it has no input, timeout",
		"work.b: a job is one of a script, a machine, a send or a call",
		`work.c.send.event: "done": no machine can send an event named`,
		`work.d.send.event: "${input.e}": the event's name is written out`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the mistake %q is not said in:\n%s", want, got)
		}
	}
}

// A job may call a location, with an input, and waits for its run's result.
func Test_mach_a_job_calls(t *testing.T) {
	d := parsed(t, "probe: p\nwork:\n  a: { call: { at: services/weather }, input: { days: \"${input.days}\" }, timeout: 1m }\n", nil)
	var body probeBody
	unmarshalBody(d.Definition, &body)
	job, err := body.Jobs["a"].Fill(eval(t), Scope{"input": map[string]any{"days": 2.0}}, "work.a", "a")
	if err != nil || !job.IsCall() || job.Call.At != "services/weather" || job.IsMachine() || job.Input.(map[string]any)["days"] != 2.0 {
		t.Errorf("the call was filled as %+v (%v)", job, err)
	}
	got := problems(t, "probe: p\nwork:\n  a: { call: { at: x }, tags: [gpu] }\n  b: { call: { at: \"x/../y\" } }\n", nil)
	for _, want := range []string{"work.a: memory, tags and files belong to a script; a call has none", `work.b.call.at: "x/../y"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the mistake %q is not said in:\n%s", want, got)
		}
	}
}
