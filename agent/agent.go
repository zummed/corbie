// Package agent is the machine whose decisions are made by a model: the
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

	"go.yaml.in/yaml/v3"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/plugin/llm"
)

// Agent is the full form of an agent's own part of a definition.
type Agent struct {
	Model  Model            `json:"model"`
	System string           `json:"system,omitzero"`
	Prompt any              `json:"prompt,omitzero"`
	Tools  map[string]*Tool `json:"tools,omitzero"`
	// Turns is how many turns the agent has; twenty if it says nothing.
	Turns int `json:"turns,omitzero"`
	// Reply is the shape of the reply in the agent's result, where it is
	// data and not text: each name with the type of its value.
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
}

// Tool is a named job a model may pick, with what it does and the shape of
// its arguments.
type Tool struct {
	Does string           `json:"does"`
	Args map[string]any   `json:"args,omitzero"`
	Job  *machine.JobSpec `json:"job"`
	// Agent says the tool's job is an agent: the model is shown what that
	// agent replied, and not the conversation it had.
	Agent bool `json:"agent,omitzero"`
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
	return append(held, "address", "reply_tokens")
}

// check says what is wrong with how a model is named.
func (m Model) check() error {
	name := m.serviceOf()
	service, ok := llm.Find(name)
	holds := strings.Join(llm.Names(), ", ")
	switch {
	case m.Name == "":
		return fmt.Errorf("an agent names its model, as model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }")
	case name == "":
		return fmt.Errorf("it is not clear which service the model %q is of: give service: one of %s", m.Name, holds)
	case !ok:
		return fmt.Errorf("%q is not a model service this Corbie holds (it holds: %s)", name, holds)
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
	case m.ReplyTokens != 0 && !slices.Contains(held, "reply_tokens"):
		return fmt.Errorf("the service %q is a program, which writes what it writes: it cannot be held to a size of reply, and takes no reply_tokens:", name)
	}
	if service.Program != "" {
		return nil
	}
	u, err := url.Parse(m.addressOf())
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
// signed in; and, for each tool of its own the program may use, the whole
// of what such a tool could use.
func (m Model) permissions() []string {
	if service, _ := llm.Find(m.serviceOf()); service != nil && service.Program != "" {
		allow := []string{"exec:" + service.Program, "env:HOME"}
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
	return []machine.Field{{Name: "turns", Kind: machine.KindNumber, Like: "20", Does: "how many turns it has; twenty by default"}}
}

// Shape is what an agent's definition holds of its own. What a model may
// be named, and what it may be asked for, is what the services this Corbie
// holds say of themselves: the same that a model is checked against.
func (agentType) Shape() *machine.Shape {
	// What follows a service, for each service, and under "" for a model
	// whose service follows from its name: every model known of the
	// services that claim names, and every level any of them takes. Which
	// of those the model's own service takes is for the check to say:
	// nothing one of them would take is kept from a person.
	names, efforts, tools, sorts := map[string][]string{}, map[string][]string{}, map[string][]string{}, map[string][]string{}
	for _, name := range llm.Names() {
		service, _ := llm.Find(name)
		names[name], efforts[name], sorts[name] = service.Known, service.Efforts, heldBy(service)
		if len(service.Tools) > 0 {
			tools[name] = service.Tools
		}
		if len(service.Models) == 0 {
			continue
		}
		names[""] = append(names[""], service.Known...)
		for _, level := range service.Efforts {
			if !slices.Contains(efforts[""], level) {
				efforts[""] = append(efforts[""], level)
			}
		}
	}
	// A model is of one service or of another, and holds what that
	// service's models hold. Which it is of, its service says, or its name.
	which := func(written map[string]any) string {
		name, _ := written["name"].(string)
		return llm.Of(name)
	}
	model := &machine.Shape{Name: "model", What: "a model", Sorts: &machine.Sorts{By: "service", Fields: sorts, Which: which}, Fields: []machine.Field{
		{Name: "name", Kind: machine.KindWord, Like: "claude-sonnet-5-5", Does: "the model's name, as its service knows it",
			Open: true, By: "service", ValuesBy: names},
		{Name: "secret", Kind: machine.KindWord, Like: "ANTHROPIC_KEY", Does: "the name of the secret that is the service's key"},
		{Name: "service", Kind: machine.KindWord, Like: "anthropic", Values: llm.Names(),
			Does: "which service's way of talking is spoken; it follows from the name for claude-… and gpt-… models"},
		{Name: "address", Kind: machine.KindWord, Like: "https://models.example",
			Does: "where the service lives, for a model that is not at its service's usual place: self-hosted, or offered by a cloud"},
		{Name: "effort", Kind: machine.KindWord, Like: "high", Does: "how much effort the model is to take", By: "service", ValuesBy: efforts},
		{Name: "reply_tokens", Kind: machine.KindNumber, Like: "4000", Does: "the most a reply may hold, in tokens"},
		{Name: "own", Kind: machine.KindList, Like: "[WebSearch]", Open: true, By: "service", ValuesBy: tools,
			Does: "for a service that is a program: the tools of its own it may use. Corbie does not run them, so each needs leave for all it could do: network to browse, disk to read or write files, all for a shell or a skill"},
	}}
	tool := &machine.Shape{Name: "tool", What: "a tool", Fields: []machine.Field{
		{Name: "does", Kind: machine.KindText, Like: "Give back every file on one shelf", Does: "what it does, for the model to choose by"},
		{Name: "args", Kind: machine.KindTypes, Like: "{ shelf: string }", Does: "the shape of its arguments: each name with the type of its value"},
		{Name: "job", Kind: machine.KindJob, Like: "{ script: ./shelf.js }", Does: "the job that is started when the model picks it; it reads input and args"},
	}}
	return &machine.Shape{Name: "agent", What: "an agent", Fields: []machine.Field{
		{Name: "model", Kind: machine.KindFields, Of: model, Like: "{ name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }", Does: "the model its turns are sent to"},
		{Name: "system", Kind: machine.KindText, Like: "You answer questions about a library of files.", Does: "the system text"},
		{Name: "prompt", Kind: machine.KindValue, Like: "${input.question}", Does: "what the agent is asked; an expression of its input"},
		{Name: "tools", Kind: machine.KindNamed, Of: tool, Does: "the tools the model may pick, each a named job"},
		{Name: "tools_file", Kind: machine.KindList, Like: "[./tools.yaml]", Does: "a file of tools shared with other agents, or a list of them"},
		{Name: "reply", Kind: machine.KindTypes, Like: "{ outcome: string }", Does: "the shape of its reply where it is data: each name with the type of its value"},
		{Name: "conversation", Kind: machine.KindValue, Like: "${input.conversation}",
			Does: "a conversation to carry on from: the conversation of an earlier agent's result, given whole"},
		{Name: "turn", Kind: machine.KindJob, Like: "./turn.js", Does: "a script of its own to make each turn"},
	}}
}

// Fill reads an agent as written and gives its full form.
func (agentType) Fill(p *machine.Parser, f *machine.Fields) any {
	a := &Agent{System: f.Text("system"), Turns: p.Limits().Int("turns")}
	// With no model named, the check says an agent names its model.
	if mf := f.Fields("model"); mf != nil {
		a.Model = Model{Name: mf.Text("name"), Secret: mf.Text("secret"), Service: mf.Text("service"),
			Address: mf.Text("address"), Effort: mf.Text("effort"), ReplyTokens: mf.Int("reply_tokens"), Own: mf.Strings("own")}
		mf.Done()
	}
	a.Prompt, _ = f.Value("prompt")
	a.Conversation, _ = f.Value("conversation")
	a.Reply, _ = f.Map("reply")
	tools, order := f.Map("tools")
	// Files of tools shared with other agents are filled in here. A tool
	// of the definition's own stands before one a file gives the same name.
	for _, file := range f.Strings("tools_file") {
		shared, err := p.Load(file)
		if err != nil {
			p.Problem("tools_file", "%q cannot be read: %v", file, err)
			continue
		}
		var more map[string]any
		if err := yaml.Unmarshal(shared, &more); err != nil {
			p.Problem("tools_file", "%q is not YAML that can be read: %v", file, err)
		}
		if tools == nil {
			tools = map[string]any{}
		}
		for name, tool := range more {
			if _, own := tools[name]; !own {
				tools[name] = tool
				order = append(order, name)
			}
		}
		sort.Strings(order)
	}
	a.Tools = map[string]*Tool{}
	for _, name := range order {
		place := "tools." + name
		tf := p.Fields(place, tools[name], f.Of("tools"))
		if tf == nil {
			continue
		}
		tool := &Tool{Does: tf.Text("does")}
		tool.Args, _ = tf.Map("args")
		if job, ok := tf.Value("job"); ok {
			tool.Job = p.Job(place+".job", job)
			tool.Agent = tool.Job != nil && p.MachineType(tool.Job.Machine) == "agent"
		}
		tf.Done()
		a.Tools[name] = tool
	}
	// The blob tools every agent has are ordinary scripts. They need no
	// permission, and the code tool runs what the model wrote, so they get
	// nothing from the definition's lists.
	for name, tool := range blobTools {
		if _, own := a.Tools[name]; own {
			p.Problem("tools."+name, "the tool %q has the name of one of the tools every agent has for blobs", name)
			continue
		}
		p.Builtin("corbie:"+name+".js", tool.source)
		a.Tools[name] = &Tool{Does: tool.does, Args: tool.args,
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

// types are the types a tool's argument, or a part of an agent's output,
// may have.
var types = []string{"string", "number", "boolean", "list", "map", "blob", "any"}

// shapeProblem says what is wrong with a shape: each name with the type of
// its value, a ? after the type for one that may be left out.
func shapeProblem(shape map[string]any) error {
	for name, t := range shape {
		text, ok := t.(string)
		switch {
		case !ok:
			return fmt.Errorf("%q: a type is written as a word (the types are %s; add ? for one that may be left out)", name, strings.Join(types, ", "))
		case !slices.Contains(types, strings.TrimSuffix(text, "?")):
			return fmt.Errorf("%q: %q is not a type (the types are %s; add ? for one that may be left out)", name, text, strings.Join(types, ", "))
		}
	}
	return nil
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
	if err := shapeProblem(a.Reply); err != nil {
		c.Problem("reply", "%v", err)
	}
	if a.Turn == nil {
		c.Problem("turn", "an agent needs the job that makes its turns")
	}
	c.Job("turn", a.Turn, "input")
	for _, name := range sortedNames(a.Tools) {
		tool, place := a.Tools[name], "tools."+name
		c.Name(place, name)
		if name == turnJob || name == previewJob {
			c.Problem(place, "the tool %q has the name of a job the agent itself makes", name)
		}
		if tool == nil || tool.Job == nil {
			c.Problem(place, "a tool needs a job")
			continue
		}
		if tool.Does == "" {
			c.Problem(place+".does", "a tool says what it does, for the model to choose by")
		}
		if err := shapeProblem(tool.Args); err != nil {
			c.Problem(place+".args", "%v", err)
		}
		c.Job(place+".job", tool.Job, "args", "input")
		// A job that reads an argument the shape does not name would always
		// find it missing: the model is never told of it.
		for _, read := range machine.ReadsIn(tool.Job.Input, "args") {
			if _, named := tool.Args[read.Name]; !named {
				c.Problem(place+".job", "it reads \"args.%s\", which the tool's args do not name", read.Name)
			}
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

// Show gives what a watcher is shown of an agent's state: which turn it is
// on and what it is waiting for.
func (agentType) Show(def *machine.Definition, raw jsontext.Value) any {
	var st state
	if raw == nil || json.Unmarshal(raw, &st) != nil {
		return nil
	}
	return map[string]any{"turn": st.Turn, "waiting": st.Waiting}
}

func (agentType) Manual() string { return manual }
