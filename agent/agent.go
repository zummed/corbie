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
	"github.com/zummed/corbie/plugin/llm"
)

// Agent is the full form of the fields of a definition that only an agent has.
type Agent struct {
	Model  Model            `json:"model"`
	System string           `json:"system,omitzero"`
	Prompt any              `json:"prompt,omitzero"`
	Tools  map[string]*Tool `json:"tools,omitzero"`
	// Toolkits are groups of tools the model is shown once it opens them.
	// A toolkit's tools are among Tools, each naming its toolkit.
	Toolkits map[string]*Toolkit `json:"toolkits,omitzero"`
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
}

// Tool is a named job a model may pick, with its description and the shape
// of its arguments, a JSON Schema of an object.
type Tool struct {
	Description string           `json:"description"`
	Args        map[string]any   `json:"args,omitzero"`
	Job         *machine.JobSpec `json:"job"`
	// Agent says the tool's job is an agent: the model is shown what that
	// agent replied, and not the conversation it had.
	Agent bool `json:"agent,omitzero"`
	// List is a job that lists tools, each with its description and the
	// shape of its arguments, in place of Description and Args: Job runs
	// each of them.
	List *machine.JobSpec `json:"list,omitzero"`
	// Toolkit names the toolkit the tool is in, if it is in one.
	Toolkit string `json:"toolkit,omitzero"`
	// Call is the location of the machine the tool calls, where it is
	// written as call:. Its description and its arguments are that
	// machine's; Job calls it, with the input the tool fixes, and the
	// model's arguments are added to that input.
	Call string `json:"call,omitzero"`
	// When is a condition on the agent's input, checked when the run
	// starts: the tool is offered only if it holds.
	When string `json:"when,omitzero"`
}

// Toolkit is a group of tools the model is shown once it opens it.
type Toolkit struct {
	Description string `json:"description"`
	// Instructions are the text the model is given when it opens the
	// toolkit, filled from the agent's input then.
	Instructions string `json:"instructions,omitzero"`
	// Ask is the ask for the jobs of the toolkit's tools and for opening
	// it, before the agent's own.
	Ask *machine.JobSpec `json:"ask,omitzero"`
	// Approve holds the toolkit's opening for the ask.
	Approve bool `json:"approve,omitzero"`
	// When is a condition on the agent's input, checked when the run
	// starts: the toolkit is offered only if it holds.
	When string `json:"when,omitzero"`
	// Meta is data Corbie keeps and never reads.
	Meta any `json:"meta,omitzero"`
}

// place is where a tool of a name is in the definition.
func (t *Tool) place(name string) string {
	if t.Toolkit != "" {
		return toolkitPlace(t.Toolkit) + ".tools." + name
	}
	return "tools." + name
}

// toolkitPlace is where a toolkit is in the definition.
func toolkitPlace(name string) string { return "toolkits." + name }

// askOf gives a toolkit's ask and where it is written. With none, its jobs
// go to the agent's.
func (a *Agent) askOf(toolkit string) (*machine.JobSpec, string) {
	if t := a.Toolkits[toolkit]; t != nil && t.Ask != nil {
		return t.Ask, toolkitPlace(toolkit) + ".ask"
	}
	return nil, ""
}

// Lists says an entry of the agent's tools has a list, whose job gives shapes
// from outside the definition.
func (agentType) Lists(def *machine.Definition) bool {
	var a Agent
	if json.Unmarshal(def.Body, &a) != nil {
		return false
	}
	for _, tool := range a.Tools {
		if tool != nil && tool.List != nil {
			return true
		}
	}
	return false
}

// Asks gives the ask of each toolkit, by the place it is written.
func (agentType) Asks(def *machine.Definition) map[string]*machine.JobSpec {
	asks := map[string]*machine.JobSpec{}
	var a Agent
	if json.Unmarshal(def.Body, &a) != nil {
		return asks
	}
	for name, toolkit := range a.Toolkits {
		if toolkit != nil && toolkit.Ask != nil {
			asks[toolkitPlace(name)+".ask"] = toolkit.Ask
		}
	}
	return asks
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
	for _, name := range chats() {
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
		{Name: "service", Kind: machine.KindWord, Like: "anthropic", Values: chats(),
			Does: "which service's way of talking is spoken; it follows from the name for claude-… and gpt-… models"},
		{Name: "address", Kind: machine.KindWord, Like: "https://models.example",
			Does: "where the service lives, for a model that is not at its service's usual place: self-hosted, or offered by a cloud"},
		{Name: "effort", Kind: machine.KindWord, Like: "high", Does: "how much effort the model is to take", By: "service", ValuesBy: efforts},
		{Name: "reply_tokens", Kind: machine.KindNumber, Like: "4000", Does: "the most a reply may hold, in tokens"},
		{Name: "own", Kind: machine.KindList, Like: "[WebSearch]", Open: true, By: "service", ValuesBy: tools,
			Does: "for a service that is a program: the tools of its own it may use. Corbie does not run them, so each needs leave for all it could do: network to browse, disk to read or write files, all for a shell or a skill"},
	}}
	when := machine.Field{Name: "when", Kind: machine.KindWord, Like: "${input.asker.scopes.includes('orders:read')}",
		Does: "a condition, one expression of the agent's input, checked when the run starts: it is offered only if it holds"}
	tool := &machine.Shape{Name: "tool", What: "a tool", Fields: []machine.Field{
		{Name: "description", Kind: machine.KindText, Like: "Give back every file on one shelf", Does: "what the tool is for, for the model to choose by"},
		{Name: "args", Kind: machine.KindTypes, Like: "{ shelf: string }", Does: "the shape of its arguments: each name with the type of its value, or a JSON Schema with type: object"},
		{Name: "job", Kind: machine.KindJob, Like: "{ script: ./shelf.js }", Does: "the job that is started when the model picks it; it reads input and args"},
		{Name: "list", Kind: machine.KindJob, Like: "{ script: ./github.js }",
			Does: "a job that lists tools, in place of description and args; job runs each of them, and reads tool, the name the list gave"},
		{Name: "call", Kind: machine.KindWord, Like: "gitlab/issues/read",
			Does: "a location in the same tree, whose machine the tool calls, in place of description, args and job: the description and the arguments are that machine's"},
		{Name: "input", Kind: machine.KindValue, Like: "{ project: group/orders }",
			Does: "of a tool with call: the names of the machine's input the tool fixes, which the model is not given; it reads the agent's input"},
		{Name: "approve", Kind: machine.KindFlag, Like: "true", Does: "of a tool with call: the call waits for the ask to approve it"},
		when,
	}}
	toolkit := &machine.Shape{Name: "toolkit", What: "a toolkit", Fields: []machine.Field{
		{Name: "description", Kind: machine.KindText, Like: "The firewall and the switch", Does: "what the toolkit is for, which the model reads before it opens it"},
		{Name: "instructions", Kind: machine.KindText, Like: "{ include: ./net.md }",
			Does: "the text the model is given when it opens the toolkit; it reads the agent's input"},
		{Name: "ask", Kind: machine.KindJob, Like: "{ machine: ./approval.yaml }",
			Does: "the ask for the jobs of the toolkit's tools and for opening it, before the agent's own; it reads the agent's input"},
		{Name: "approve", Kind: machine.KindFlag, Like: "true", Does: "opening the toolkit waits for the ask to approve it"},
		when,
		{Name: "tools", Kind: machine.KindNamed, Of: tool, Does: "the toolkit's tools, each a named job"},
		{Name: "meta", Kind: machine.KindValue, Like: "{ people: [alice] }", Does: "data of any kind, which Corbie keeps and never reads"},
	}}
	return &machine.Shape{Name: "agent", What: "an agent", Fields: []machine.Field{
		{Name: "model", Kind: machine.KindFields, Of: model, Like: "{ name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }", Does: "the model its turns are sent to"},
		{Name: "system", Kind: machine.KindText, Like: "You answer questions about a library of files.", Does: "the system text"},
		{Name: "prompt", Kind: machine.KindValue, Like: "${input.question}", Does: "what the agent is asked; an expression of its input"},
		{Name: "tools", Kind: machine.KindNamed, Of: tool, Does: "the tools the model may pick, each a named job"},
		{Name: "toolkits", Kind: machine.KindNamed, Of: toolkit, Like: "{ orders: { include: toolkits/orders.yaml } }", Does: "groups of tools the model is shown once it opens them"},
		{Name: "reply", Kind: machine.KindTypes, Like: "{ outcome: string }", Does: "the shape of its reply where it is data: each name with the type of its value, or a JSON Schema with type: object"},
		{Name: "conversation", Kind: machine.KindValue, Like: "${input.conversation}",
			Does: "a conversation to carry on from: the conversation of an earlier agent's result, given whole"},
		{Name: "turn", Kind: machine.KindJob, Like: "./turn.js", Does: "a script of its own to make each turn"},
	}}
}

// fillToolkit reads one toolkit as written. A tool of a toolkit may not
// take the name of any other tool.
func fillToolkit(p *machine.Parser, a *Agent, shape *machine.Shape, name string, written any) {
	place := toolkitPlace(name)
	ff := p.Fields(place, written, shape)
	if ff == nil {
		return
	}
	toolkit := &Toolkit{Description: ff.Text("description"), Instructions: ff.Text("instructions"),
		Approve: ff.Bool("approve"), When: ff.Cond("when")}
	a.Toolkits[name] = toolkit
	toolkit.Meta, _ = ff.Value("meta")
	if ask, ok := ff.Value("ask"); ok {
		toolkit.Ask = p.Job(place+".ask", ask)
		p.Ask(place+".ask", toolkit.Ask)
	}
	tools, order := ff.Map("tools")
	read := func() {
		for _, toolName := range order {
			if _, taken := a.Tools[toolName]; taken {
				p.Problem(place+".tools."+toolName, "the tool %q has the name of another tool of the agent", toolName)
				continue
			}
			if t := fillTool(p, a, ff.Of("tools"), place+".tools."+toolName, toolName, tools[toolName]); t != nil {
				t.Toolkit = name
			}
		}
	}
	// A machine a tool of the toolkit runs takes the toolkit's ask as the
	// nearest above it.
	if toolkit.Ask != nil {
		p.Nearer(read)
	} else {
		read()
	}
	ff.Done()
}

// fillTool reads one tool as written and adds it to the agent's tools.
func fillTool(p *machine.Parser, a *Agent, shape *machine.Shape, place, name string, written any) *Tool {
	tf := p.Fields(place, written, shape)
	if tf == nil {
		return nil
	}
	if tf.Has("call") {
		tool := fillCall(p, tf, place)
		tf.Done()
		a.Tools[name] = tool
		return tool
	}
	for _, field := range []string{"input", "approve"} {
		if tf.Has(field) {
			p.Problem(place+"."+field, "%s: belongs to a tool with call:; a tool with job: writes it in the job", field)
		}
	}
	tf.Value("input")
	tf.Bool("approve")
	tool := &Tool{Description: tf.Text("description"), When: tf.Cond("when")}
	if list, ok := tf.Value("list"); ok {
		tool.List = p.Job(place+".list", list)
	}
	if args, _ := tf.Map("args"); args != nil || tool.List == nil {
		tool.Args = filled(p, place+".args", args)
	}
	if job, ok := tf.Value("job"); ok {
		tool.Job = p.Job(place+".job", job)
		tool.Agent = tool.Job != nil && p.MachineType(tool.Job.Machine) == "agent"
	}
	tf.Done()
	a.Tools[name] = tool
	return tool
}

// fillCall reads a tool written with call:. It holds send: for the
// location, and nothing else is written for it. Its job calls the location
// with the input the tool fixes.
func fillCall(p *machine.Parser, tf *machine.Fields, place string) *Tool {
	for _, field := range []string{"description", "args", "job", "list"} {
		if tf.Has(field) {
			p.Problem(place+"."+field, "a tool with call: has no %s: the description and arguments come from the machine called", field)
		}
	}
	tf.Text("description")
	tf.Map("args")
	tf.Value("job")
	tf.Value("list")
	at := tf.Text("call")
	tool := &Tool{Call: at, When: tf.Cond("when"),
		Job: &machine.JobSpec{Call: &machine.AddressSpec{At: at}, Approve: tf.Bool("approve")}}
	fixed := map[string]any{}
	if written, ok := tf.Value("input"); ok {
		if fixed, ok = written.(map[string]any); !ok {
			p.Problem(place+".input", "this should be a mapping of names of the called machine's input, and is not one")
			fixed = map[string]any{}
		}
	}
	if len(fixed) > 0 {
		tool.Job.Input = fixed
	}
	target := p.MachineAt(place+".call", at)
	if target == nil {
		tool.Args = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
		return tool
	}
	tool.Job.Allow = []string{"send:" + at}
	tool.Description, tool.Agent = target.Description, target.Type == "agent"
	if target.Description == "" {
		p.Problem(place+".call", "the machine at %s has no description:, which the model is given as the tool's", at)
	}
	if target.Lifetime != machine.LifetimeTransient {
		p.Problem(place+".call", "the machine at %s is %s: a call starts a run only of a transient location", at, target.Lifetime)
	}
	tool.Args = callArgs(p, place, at, target.Input, fixed)
	return tool
}

// callArgs gives the arguments of a tool that calls a machine: the
// machine's input without the names the tool fixes, and no other name.
// A blob in that input is the name the model was shown.
func callArgs(p *machine.Parser, place, at string, input, fixed map[string]any) map[string]any {
	properties := map[string]any{}
	var required []any
	for _, key := range sortedNames(input) {
		if !slices.Contains([]string{"type", "properties", "required", "additionalProperties", "description", "title"}, key) {
			p.Problem(place+".call", "the input of the machine at %s is written with %s, and a tool takes its arguments only from an input written with properties", at, key)
		}
	}
	if given, ok := input["properties"].(map[string]any); ok {
		for name, property := range given {
			properties[name] = asArg(property)
		}
	}
	required, _ = input["required"].([]any)
	for _, name := range sortedNames(fixed) {
		if _, named := properties[name]; !named {
			p.Problem(place+".input."+name, "the machine at %s takes no input named %q", at, name)
		}
		delete(properties, name)
		required = slices.DeleteFunc(slices.Clone(required), func(r any) bool { return r == name })
	}
	args := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		args["required"] = required
	}
	return args
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
			Address: mf.Text("address"), Effort: mf.Text("effort"), ReplyTokens: mf.Int("reply_tokens"), Own: mf.Strings("own")}
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
	// Toolkits are filled in after every other tool, so that a tool of a
	// toolkit is checked against all of them.
	a.Toolkits = map[string]*Toolkit{}
	toolkits, order := f.Map("toolkits")
	for _, name := range order {
		fillToolkit(p, a, f.Of("toolkits"), name, toolkits[name])
	}
	// The tool that opens a toolkit is Corbie's, as the blob tools are. It
	// needs no permission and gets nothing from the definition's lists.
	if _, taken := a.Tools[openTool]; taken {
		p.Problem(a.Tools[openTool].place(openTool), "the tool %q has the name of the tool every agent with toolkits has for opening them", openTool)
	} else if len(a.Toolkits) > 0 {
		p.Builtin(openScript, openSource)
		a.Tools[openTool] = &Tool{Description: openDescription, Args: filled(p, "tools."+openTool+".args", map[string]any{"toolkit": "string"}),
			Job: &machine.JobSpec{Script: openScript, Input: map[string]any{"toolkit": "${args.toolkit}"}, Alone: true}}
	}
	// The blob tools every agent has are ordinary scripts. They need no
	// permission, and the code tool runs what the model wrote, so they get
	// nothing from the definition's lists.
	for name, tool := range blobTools {
		if _, own := a.Tools[name]; own {
			p.Problem(a.Tools[name].place(name), "the tool %q has the name of one of the tools every agent has for blobs", name)
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
	for _, name := range sortedNames(a.Toolkits) {
		toolkit, place := a.Toolkits[name], toolkitPlace(name)
		c.Name(place, name)
		if toolkit == nil || toolkit.Description == "" {
			c.Problem(place+".description", "a toolkit says what it is for, for the model to choose by")
			continue
		}
		c.Value(place+".instructions", toolkit.Instructions, "input")
		if toolkit.When != "" {
			c.Cond(place+".when", toolkit.When, "input")
		}
		// A toolkit's own ask is the ask for opening it and for the jobs of
		// its tools: they need none from the agent.
		ask, _ := a.askOf(name)
		c.Asked(ask, func() {
			c.Job(place+".ask", toolkit.Ask, "input")
			if toolkit.Approve {
				c.Approves()
			}
		})
	}
	for _, name := range sortedNames(a.Tools) {
		tool := a.Tools[name]
		if tool == nil {
			c.Problem("tools."+name, "a tool needs a job")
			continue
		}
		ask, _ := a.askOf(tool.Toolkit)
		c.Asked(ask, func() { checkTool(c, &a, name, tool) })
	}
}

// checkTool reports what is wrong with one tool.
func checkTool(c *machine.Checker, a *Agent, name string, tool *Tool) {
	place := tool.place(name)
	c.Name(place, name)
	if _, ok := a.Toolkits[tool.Toolkit]; tool.Toolkit != "" && !ok {
		c.Problem(place, "the tool is in the toolkit %q, which the agent does not have", tool.Toolkit)
	}
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
	if tool.Call != "" {
		// What the machine called gives was checked as it was read.
		if _, err := machine.CompileShape(tool.Args); err != nil {
			c.Problem(place+".call", "%v", err)
		}
		c.Job(place, tool.Job, "input")
		return
	}
	if tool.List != nil {
		if tool.Description != "" || tool.Args != nil {
			c.Problem(place, "a tool with list: is given each tool's description and args by the list, and has no description: or args: of its own")
		}
		c.Job(place+".list", tool.List, "input")
		c.Job(place+".job", tool.Job, "args", "input", "tool")
		// The arguments' fields are not known until the list is made, so
		// one read here could be missing every time.
		for _, read := range machine.ReadsIn(append([]any{tool.Job.Input}, tool.Job.Addressed()...), "args") {
			c.Problem(place+".job", "it reads \"args.%s\"; with list:, the job reads ${args} only whole, since its fields are not known until the run", read.Name)
		}
		return
	}
	if tool.Description == "" {
		c.Problem(place+".description", "a tool says what it is for, for the model to choose by")
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
