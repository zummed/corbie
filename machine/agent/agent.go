// Package agent is the machine whose tools a model picks: the
// model is asked what to do, the tools it picks are run, the results go
// back to it, and this repeats until it answers without asking for a tool.
package agent

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"net"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/zummed/corbie/machine"
	_ "github.com/zummed/corbie/machine/toolkit" // every program that has agents has toolkits
	"github.com/zummed/corbie/plugin/llm"
)

// Agent is the full form of the fields of a definition that only an agent has.
type Agent struct {
	Model  Model  `json:"model"`
	System string `json:"system,omitzero"`
	Prompt any    `json:"prompt,omitzero"`
	// Tools are the agent's entries: each a tool written in place, or a
	// machine, which is one tool or, where it is a toolkit, a toolkit.
	Tools map[string]*Tool `json:"tools,omitzero"`
	// Turns is how many turns the agent has; twenty if it says nothing.
	Turns int `json:"turns,omitzero"`
	// Reply is the shape of the reply in the agent's result, where it is
	// data and not text: a JSON Schema of an object.
	Reply map[string]any `json:"reply,omitzero"`
	// Conversation is a conversation so far, to carry on from.
	Conversation any `json:"conversation,omitzero"`
	// Turn is the job that makes a turn.
	Turn *machine.JobSpec `json:"turn"`
}

// Model names the model an agent's turns are sent to.
type Model struct {
	Name string `json:"name"`
	// Secret is the name of the secret that is the service's key.
	Secret string `json:"secret,omitzero"`
	// Service is which service's way of talking is spoken; it follows from
	// the model's name where it is not given.
	Service string `json:"service,omitzero"`
	// Address is where the service lives, for one that is not at its usual
	// place.
	Address string `json:"address,omitzero"`
	Effort  string `json:"effort,omitzero"`
	// ReplyTokens is the most a reply may hold, in tokens.
	ReplyTokens int `json:"reply_tokens,omitzero"`
	// Own are the tools of its own that a model run as a program may use:
	// tools Corbie does not run.
	Own []string `json:"own,omitzero"`
	// ToolChanges says tools added or removed after the first turn are
	// sent to the service as messages, for a service that has that.
	ToolChanges bool `json:"tool_changes,omitzero"`
}

// Tool is an entry of an agent's tools. Written in place it is a named job
// a model may pick, with its description and the shape of its arguments, a
// JSON Schema of an object. Written machine: or call: it names a machine,
// whose description it takes: any machine is one tool, whose arguments are
// the machine's input, and a toolkit is a toolkit, which the model opens.
type Tool struct {
	Description string           `json:"description,omitzero"`
	Args        map[string]any   `json:"args,omitzero"`
	Job         *machine.JobSpec `json:"job"`
	// Agent says the tool's job is an agent: the model is shown what that
	// agent replied, and not the conversation it had.
	Agent bool `json:"agent,omitzero"`
	// Machine is the file of the machine the entry names, and Call its
	// location. Job runs it with the input the entry fixes.
	Machine string `json:"machine,omitzero"`
	Call    string `json:"call,omitzero"`
	// Toolkit says the machine the entry names is a toolkit.
	Toolkit bool `json:"toolkit,omitzero"`
	// Open says a toolkit is opened before the first turn.
	Open bool `json:"open,omitzero"`
	// When is a condition on the agent's input, checked when the run
	// starts: the entry is offered only if it holds.
	When string `json:"when,omitzero"`
}

// named says the entry names a machine, by its file or its location.
func (t *Tool) named() bool { return t.Machine != "" || t.Call != "" }

// place is where an entry of a name is in the definition.
func place(name string) string { return "tools." + name }

// Calls gives the locations the agent's entries written call: name in
// full, whose machines a run reads as it starts.
func (agentType) Calls(def *machine.Definition) []string {
	var a Agent
	if json.Unmarshal(def.Body, &a) != nil {
		return nil
	}
	var out []string
	for _, tool := range a.Tools {
		if tool != nil && strings.HasPrefix(tool.Call, "/") && !slices.Contains(out, tool.Call) {
			out = append(out, tool.Call)
		}
	}
	slices.Sort(out)
	return out
}

// DefaultTurns is how many turns an agent has unless it says otherwise.
const DefaultTurns = 20

// The jobs an agent itself makes, whose names no tool may take.
const (
	turnJob    = "turn"
	previewJob = "preview"
)

// serviceOf says which service a model is of, from what the definition
// gives or from the model's name.
func (m Model) serviceOf() string {
	if m.Service != "" {
		return m.Service
	}
	return llm.Of(m.Name)
}

func (m Model) addressOf() string {
	if m.Address != "" {
		return m.Address
	}
	service, _ := llm.Find(m.serviceOf())
	if service == nil {
		return ""
	}
	return service.Address
}

// heldBy gives the fields a model of a service holds. A service on the web
// lives at an address and may be held to a size of reply. One that is a
// program is on the runner's machine, writes what it writes, and has tools
// of its own. Only a service that takes levels of effort is asked for one.
func heldBy(service *llm.Service) []string {
	held := []string{"name", "secret", "service"}
	if len(service.Efforts) > 0 {
		held = append(held, "effort")
	}
	if service.Program != "" {
		return append(held, "own")
	}
	held = append(held, "address", "reply_tokens")
	if service.ToolChanges {
		held = append(held, "tool_changes")
	}
	return held
}

// chats gives the services that have chat: those an agent's model may be
// of. A System One model answers questions, and could take no turn.
func chats() []string { return llm.Having(func(s *llm.Service) bool { return s.Chat != nil }) }

// check says what is wrong with how a model is named.
func (m Model) check() error {
	name := m.serviceOf()
	service, ok := llm.Find(name)
	holds := strings.Join(chats(), ", ")
	switch {
	case m.Name == "":
		return fmt.Errorf("an agent names its model, as model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }")
	case name == "":
		return fmt.Errorf("it is not clear which service the model %q is of: give service: one of %s", m.Name, holds)
	case !ok:
		return fmt.Errorf("%q is not a model service an agent's model can be of (one of: %s)", name, holds)
	case service.Chat == nil:
		return fmt.Errorf("the service %q has no chat: an agent's model is of one of %s", name, holds)
	case m.Effort != "" && !slices.Contains(service.Efforts, m.Effort):
		return fmt.Errorf("the service %q does not take the effort %q (it takes: %s)", name, m.Effort, strings.Join(service.Efforts, ", "))
	case machine.HasExpr(m.Secret) || machine.HasExpr(m.Name):
		return fmt.Errorf("a model's name and its secret's name are written out")
	}
	// A model holds what its service's models hold, and nothing else.
	held := heldBy(service)
	switch {
	case m.Address != "" && !slices.Contains(held, "address"):
		return fmt.Errorf("the service %q is a program on the runner's computer, and has no address", name)
	case len(m.Own) > 0 && !slices.Contains(held, "own"):
		return fmt.Errorf("own: is for a service that is a program, which may have tools of its own; the service %q has none", name)
	case m.ToolChanges && !slices.Contains(held, "tool_changes"):
		return fmt.Errorf("the service %q has no way to be sent a change of tools in a message, and takes no tool_changes:", name)
	case m.ReplyTokens != 0 && !slices.Contains(held, "reply_tokens"):
		return fmt.Errorf("the service %q is a program, which writes what it writes: it cannot be held to a size of reply, and takes no reply_tokens:", name)
	}
	if service.Program != "" {
		return nil
	}
	return checkAddress(m.addressOf())
}

// checkAddress says what is wrong with the address of a model on the web.
func checkAddress(address string) error {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("the model's address is not a web address")
	}
	if u.User != nil {
		return fmt.Errorf("the model's address may not hold a user name or a password")
	}
	return nil
}

// permissions writes exactly what a turn needs. For a service on the web
// it is the connection to the service, and the key bound to that
// connection; for a plain address the key is bound to anywhere, since a
// key bound to a host never goes over a plain connection. For a service
// that is a program it is the start of the program, with the key bound to
// it or, with no key, the variable that says where the runner's user is
// signed in, which is HOME and on Windows USERPROFILE; and, for each tool of its own the program may use, the whole
// of what such a tool could use.
func (m Model) permissions() []string {
	if service, _ := llm.Find(m.serviceOf()); service != nil && service.Program != "" {
		allow := []string{"exec:" + service.Program, "env:HOME", "env:USERPROFILE"}
		if m.Secret != "" {
			allow = []string{"exec:" + service.Program, "secret:" + m.Secret + "@exec:" + service.Program}
		}
		for _, tool := range m.Own {
			if group := service.Needs(tool); !slices.Contains(allow, group) {
				allow = append(allow, group)
			}
		}
		return allow
	}
	u, err := url.Parse(m.addressOf())
	if err != nil {
		return nil
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	to := "net:" + net.JoinHostPort(u.Hostname(), port)
	allow := []string{to}
	switch {
	case m.Secret == "":
	case u.Scheme == "http":
		allow = append(allow, "secret:"+m.Secret+"@net.unencrypted:"+net.JoinHostPort(u.Hostname(), port))
	default:
		allow = append(allow, "secret:"+m.Secret+"@"+to)
	}
	return allow
}

type agentType struct{}

func init() { machine.Register(agentType{}) }

func (agentType) Name() string { return "agent" }

// Limits adds an agent's turns to the limits every type takes.
func (agentType) Limits() []machine.Field {
	return []machine.Field{{Name: "turns", Kind: machine.KindNumber, Description: "how many turns it has; twenty by default"}}
}

// Shape is what an agent's definition holds of its own. A model holds the
// fields its service's models hold, as the services this Corbie holds say.
func (agentType) Shape() *machine.Shape {
	// A model is of one service or of another, and holds what that
	// service's models hold. Which it is of, its service says, or its name.
	sorts := map[string][]string{}
	for _, name := range chats() {
		service, _ := llm.Find(name)
		sorts[name] = heldBy(service)
	}
	which := func(written map[string]any) string {
		name, _ := written["name"].(string)
		return llm.Of(name)
	}
	model := &machine.Shape{Name: "model", What: "a model", Sorts: &machine.Sorts{By: "service", Fields: sorts, Which: which}, Fields: []machine.Field{
		{Name: "name", Kind: machine.KindWord, Description: "the model's name, as its service knows it"},
		{Name: "secret", Kind: machine.KindWord, Description: "the name of the secret that is the service's key"},
		{Name: "service", Kind: machine.KindWord,
			Description: "which service's way of talking is spoken; it follows from the name for claude-… and gpt-… models"},
		{Name: "address", Kind: machine.KindWord,
			Description: "where the service lives, for a model that is not at its service's usual place: self-hosted, or offered by a cloud"},
		{Name: "effort", Kind: machine.KindWord, Description: "how much effort the model is to take"},
		{Name: "reply_tokens", Kind: machine.KindNumber, Description: "the most a reply may hold, in tokens"},
		{Name: "tool_changes", Kind: machine.KindFlag,
			Description: "for a service that has it: a tool added or removed after the first turn is sent as a message, so the service keeps its cache; the model must be one that takes it"},
		{Name: "own", Kind: machine.KindList,
			Description: "for a service that is a program: the tools of its own it may use. Corbie does not run them, so each needs leave for all it could do: network to browse, disk to read or write files, all for a shell or a skill"},
	}}
	tool := &machine.Shape{Name: "tool", What: "a tool", Fields: []machine.Field{
		{Name: "description", Kind: machine.KindText, Description: "what the tool is for, for the model to choose by"},
		{Name: "args", Kind: machine.KindTypes, Description: "the shape of its arguments: each name with the type of its value, or a JSON Schema with type: object"},
		{Name: "job", Kind: machine.KindJob, Description: "the job that is started when the model picks it; it reads input and args"},
		{Name: "machine", Kind: machine.KindWord, Path: true,
			Description: "the file of a machine, in place of description, args and job: the description is that machine's. A toolkit is a toolkit the model opens; any other machine is one tool, whose arguments are its input"},
		{Name: "call", Kind: machine.KindWord,
			Description: "the location of a machine, in place of machine:. One in the same tree is written from its prefix and read with the tree; one written in full from / is read when the run starts"},
		{Name: "input", Kind: machine.KindValue,
			Description: "of an entry with machine: or call:, the names of the machine's input the entry fixes, which the model is not given; it reads the agent's input"},
		{Name: "approve", Kind: machine.KindFlag, Description: "of an entry with machine: or call:, marks every job the entry starts for approval, so each starts only after a hook before it"},
		{Name: "meta", Kind: machine.KindValue, Description: "of an entry with machine: or call:, data of any kind, which hooks read as job.meta and nothing else reads"},
		{Name: "open", Kind: machine.KindFlag, Description: "of an entry that names a toolkit: the toolkit is opened before the first turn"},
		{Name: "when", Kind: machine.KindWord,
			Description: "a condition, one expression of the agent's input, checked when the run starts: the entry is offered only if it holds"},
	}}
	return &machine.Shape{Name: "agent", What: "an agent", Fields: []machine.Field{
		{Name: "model", Kind: machine.KindFields, Of: model, Description: "the model its turns are sent to"},
		{Name: "system", Kind: machine.KindText, Description: "the system text"},
		{Name: "prompt", Kind: machine.KindValue, Description: "what the agent is asked; an expression of its input"},
		{Name: "tools", Kind: machine.KindNamed, Of: tool,
			Description: "the agent's entries: each a tool written in place, or a machine named by its file or its location"},
		{Name: "reply", Kind: machine.KindTypes, Description: "the shape of its reply where it is data: each name with the type of its value, or a JSON Schema with type: object"},
		{Name: "conversation", Kind: machine.KindValue,
			Description: "a conversation to carry on from: the conversation of an earlier agent's result, given whole"},
		{Name: "turn", Kind: machine.KindJob, Description: "a script of its own to make each turn"},
	}}
}

// fillTool reads one entry as written and adds it to the agent's tools.
func fillTool(p *machine.Parser, a *Agent, shape *machine.Shape, place, name string, written any) {
	tf := p.Fields(place, written, shape)
	if tf == nil {
		return
	}
	if tf.Has("machine") || tf.Has("call") {
		a.Tools[name] = fillNamed(p, tf, place)
		tf.Done()
		return
	}
	for _, field := range []string{"input", "approve", "meta", "open"} {
		if tf.Has(field) {
			p.Problem(place+"."+field, "%s: belongs to an entry with machine: or call:; a tool with job: writes its input, approve and meta in the job", field)
		}
	}
	tf.Value("input")
	tf.Bool("approve")
	tf.Value("meta")
	tf.Bool("open")
	tool := &Tool{Description: tf.Text("description"), When: tf.Cond("when")}
	args, _ := tf.Map("args")
	tool.Args = filled(p, place+".args", args)
	if job, ok := tf.Value("job"); ok {
		tool.Job = p.Job(place+".job", job)
		tool.Agent = tool.Job != nil && p.MachineType(tool.Job.Machine) == "agent"
	}
	tf.Done()
	a.Tools[name] = tool
}

// fillNamed reads an entry written with machine: or call:. Nothing else of
// a tool is written for it: its job runs the machine with the input the
// entry fixes, and one written call: holds send: for the location.
func fillNamed(p *machine.Parser, tf *machine.Fields, place string) *Tool {
	for _, field := range []string{"description", "args", "job"} {
		if tf.Has(field) {
			p.Problem(place+"."+field, "an entry with machine: or call: has no %s: the description and arguments come from the machine it names", field)
		}
	}
	tf.Text("description")
	tf.Map("args")
	tf.Value("job")
	tool := &Tool{When: tf.Cond("when"), Open: tf.Bool("open"), Job: &machine.JobSpec{Approve: tf.Bool("approve")}}
	tool.Job.Meta, _ = tf.Value("meta")
	fixed := map[string]any{}
	if written, ok := tf.Value("input"); ok {
		if fixed, ok = written.(map[string]any); !ok {
			p.Problem(place+".input", "this should be a mapping of names of the machine's input, and is not one")
			fixed = map[string]any{}
		}
	}
	if len(fixed) > 0 {
		tool.Job.Input = fixed
	}
	byFile := tf.Has("machine")
	file, at := tf.Text("machine"), tf.Text("call")
	var target *machine.Definition
	switch {
	case byFile && at != "":
		p.Problem(place, "an entry names its machine by its file with machine:, or by its location with call:, and not both")
		return tool
	case byFile:
		tool.Machine = p.Machine(place+".machine", file)
		tool.Job.Machine = tool.Machine
		if target = p.MachineNamed(tool.Machine); target == nil {
			return tool
		}
	default:
		tool.Call = at
		tool.Job.Call = &machine.AddressSpec{At: at}
		target = p.MachineAt(place+".call", at)
		if target != nil || strings.HasPrefix(at, "/") {
			tool.Job.Allow = []string{"send:" + at}
		}
		if target == nil {
			// A location in full is read when the run starts.
			return tool
		}
	}
	for _, w := range tool.read(machine.Recorded{Description: target.Description, Input: target.Input, Type: target.Type, Lifetime: target.Lifetime}) {
		p.Problem(place+w.at, "%s", w.text)
	}
	return tool
}

// wrong is what is wrong with an entry that names a machine, at a place
// under the entry's: .machine or .call, or .input.<name> for a name it fixes.
type wrong struct{ at, text string }

// read gives an entry that names a machine its description from what is
// read of that machine, and says what is wrong. A toolkit makes the entry a
// toolkit. Any other machine makes it one tool, whose arguments are the
// machine's input without the names the entry fixes, and no other name. A
// blob in that input is the name the model was shown.
func (t *Tool) read(m machine.Recorded) []wrong {
	at, the := ".machine", "the machine "+t.Machine
	if t.Call != "" {
		at, the = ".call", "the machine at "+t.Call
	}
	var out []wrong
	if m.Refused != "" {
		return []wrong{{at, m.Refused}}
	}
	t.Description, t.Agent, t.Toolkit = m.Description, m.Type == "agent", m.Type == "toolkit"
	if m.Description == "" {
		out = append(out, wrong{at, fmt.Sprintf("%s has no description:, which the model is given as the entry's", the)})
	}
	if t.Call != "" && m.Lifetime != machine.LifetimeTransient {
		out = append(out, wrong{at, fmt.Sprintf("%s is %s: a call starts a run only of a transient location", the, m.Lifetime)})
	}
	if t.Open && !t.Toolkit {
		out = append(out, wrong{".open", fmt.Sprintf("%s is no toolkit: open: is for an entry that names a toolkit", the)})
	}
	properties := map[string]any{}
	if given, ok := m.Input["properties"].(map[string]any); ok {
		for name, property := range given {
			properties[name] = asArg(property)
		}
	}
	required, _ := m.Input["required"].([]any)
	fixed, _ := t.Job.Input.(map[string]any)
	for _, name := range sortedNames(fixed) {
		if _, named := properties[name]; !named || t.Toolkit && name == "request" {
			out = append(out, wrong{".input." + name, fmt.Sprintf("%s takes no input named %q", the, name)})
		}
		delete(properties, name)
		required = slices.DeleteFunc(slices.Clone(required), func(r any) bool { return r == name })
	}
	if t.Toolkit {
		return out
	}
	for _, key := range sortedNames(m.Input) {
		if !slices.Contains([]string{"type", "properties", "required", "additionalProperties", "description", "title"}, key) {
			out = append(out, wrong{at, fmt.Sprintf("the input of %s is written with %s, and a tool takes its arguments only from an input written with properties", the, key)})
		}
	}
	t.Args = map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		t.Args["required"] = required
	}
	return out
}

// asArg gives one property of a machine's input as a tool's argument: a
// blob, which the input takes as the blob itself, is the name the model was
// shown.
func asArg(property any) any {
	m, ok := property.(map[string]any)
	if !ok || m["type"] != "object" || !slices.Equal(asTexts(m["required"]), []string{"$blob"}) {
		return property
	}
	arg := map[string]any{"type": "string", "format": "blob"}
	if description, ok := m["description"]; ok {
		arg["description"] = description
	}
	return arg
}

func asTexts(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, item := range list {
		text, _ := item.(string)
		out = append(out, text)
	}
	return out
}

// Fill reads an agent as written and gives its full form.
func (agentType) Fill(p *machine.Parser, f *machine.Fields) any {
	a := &Agent{System: f.Text("system"), Turns: p.Limits().Int("turns")}
	// With no model named, the check says an agent names its model.
	if mf := f.Fields("model"); mf != nil {
		a.Model = Model{Name: mf.Text("name"), Secret: mf.Text("secret"), Service: mf.Text("service"),
			Address: mf.Text("address"), Effort: mf.Text("effort"), ReplyTokens: mf.Int("reply_tokens"), Own: mf.Strings("own"), ToolChanges: mf.Bool("tool_changes")}
		mf.Done()
	}
	a.Prompt, _ = f.Value("prompt")
	a.Conversation, _ = f.Value("conversation")
	if reply, _ := f.Map("reply"); reply != nil {
		a.Reply = filled(p, "reply", reply)
	}
	a.Tools = map[string]*Tool{}
	tools, order := f.Map("tools")
	for _, name := range order {
		fillTool(p, a, f.Of("tools"), "tools."+name, name, tools[name])
	}
	// The tool that opens a toolkit is Corbie's, and the agent itself
	// answers it: it starts the job of the entry that names the toolkit.
	for _, name := range []string{openTool, toolkitsList, resumeTool} {
		if _, taken := a.Tools[name]; taken {
			p.Problem(place(name), "the tool %q has a name Corbie's own tools for toolkits have: open, with the list toolkits, and resume", name)
		}
	}
	// The blob tools every agent has are ordinary scripts. They need no
	// permission, and the code tool runs what the model wrote, so they get
	// nothing from the definition's lists.
	for name, tool := range blobTools {
		if _, own := a.Tools[name]; own {
			p.Problem(place(name), "the tool %q has the name of one of the tools every agent has for blobs", name)
			continue
		}
		p.Builtin("corbie:"+name+".js", tool.source)
		a.Tools[name] = &Tool{Description: tool.description, Args: filled(p, "tools."+name+".args", tool.args),
			Job: &machine.JobSpec{Script: "corbie:" + name + ".js", Input: tool.input, Alone: true}}
	}
	// The job that makes a turn: Corbie's own script, or one the
	// definition puts in its place. It is written exactly what it needs and
	// gets nothing else from the definition's permissions.
	if turn, ok := f.Value("turn"); ok {
		a.Turn = p.Job("turn", turn)
	} else {
		p.Builtin(turnScript, turnSource)
		a.Turn = &machine.JobSpec{Script: turnScript}
	}
	p.Builtin(previewScript, previewSource)
	a.Turn.Alone = true
	if a.Model.check() == nil {
		a.Turn.Allow = append(a.Model.permissions(), a.Turn.Allow...)
	}
	return a
}

// filled gives a shape in full form, reporting a short form it cannot fill
// out at its place.
func filled(p *machine.Parser, place string, shape map[string]any) map[string]any {
	out, err := shortForm.Full(shape)
	if err != nil {
		p.Problem(place, "%v", err)
		return shape
	}
	return out
}

// Check reports what is wrong with an agent in full form.
func (agentType) Check(c *machine.Checker, def *machine.Definition) {
	var a Agent
	if !c.DecodeBody(def, &a) {
		return
	}
	// A model named wrongly is reported once.
	if err := a.Model.check(); err != nil {
		c.Problem("model", "%v", err)
	}
	c.Value("system", a.System, "input")
	c.Value("prompt", a.Prompt, "input")
	c.Value("conversation", a.Conversation, "input")
	if a.Turns < 0 {
		c.Problem("limits.turns", "an agent cannot have fewer turns than none")
	}
	if a.Reply != nil {
		if _, err := machine.CompileShape(a.Reply); err != nil {
			c.Problem("reply", "%v", err)
		}
	}
	if a.Turn == nil {
		c.Problem("turn", "an agent needs the job that makes its turns")
	}
	c.Job("turn", a.Turn, "input")
	for _, name := range sortedNames(a.Tools) {
		tool := a.Tools[name]
		if tool == nil {
			c.Problem("tools."+name, "a tool needs a job")
			continue
		}
		checkTool(c, name, tool)
	}
}

// checkTool reports what is wrong with one entry.
func checkTool(c *machine.Checker, name string, tool *Tool) {
	place := place(name)
	c.Name(place, name)
	if tool.When != "" {
		c.Cond(place+".when", tool.When, "input")
	}
	if name == turnJob || name == previewJob {
		c.Problem(place, "the tool %q has the name of a job the agent itself makes", name)
	}
	if tool.Job == nil {
		c.Problem(place, "a tool needs a job")
		return
	}
	if tool.named() {
		// What the machine gives was checked as it was read, and one at a
		// location in full is read when the run starts.
		if tool.Args != nil {
			if _, err := machine.CompileShape(tool.Args); err != nil {
				c.Problem(place, "%v", err)
			}
		}
		c.Job(place, tool.Job, "input")
		return
	}
	if tool.Description == "" {
		c.Problem(place+".description", "a tool says what it is for, for the model to choose by")
	} else if machine.HasExpr(tool.Description) {
		c.Problem(place+".description", "a description is plain text, with no expression")
	}
	if _, err := machine.CompileShape(tool.Args); err != nil {
		c.Problem(place+".args", "%v", err)
	}
	c.Job(place+".job", tool.Job, "args", "input")
	// A job that reads an argument the shape does not name would always
	// find it missing: the model is never told of it, whatever else the
	// shape allows.
	properties, _ := tool.Args["properties"].(map[string]any)
	for _, read := range machine.ReadsIn(append([]any{tool.Job.Input}, tool.Job.Addressed()...), "args") {
		if _, named := properties[read.Name]; !named {
			c.Problem(place+".job", "it reads \"args.%s\", which the tool's args do not name", read.Name)
		}
	}
}

func sortedNames[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Show gives an agent's state as a watcher is shown it: how many turns it
// has taken.
func (agentType) Show(raw jsontext.Value) machine.Shown {
	var st state
	if raw == nil || json.Unmarshal(raw, &st) != nil {
		return machine.Shown{}
	}
	return machine.Shown{Turn: st.Turn}
}

func (agentType) Manual() string { return manual }
