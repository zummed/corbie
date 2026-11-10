package agent

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/plugin/llm"
)

// Decision is the full form of the fields of a definition that only a
// decision has.
type Decision struct {
	Model Model `json:"model"`
	// State is the text or data the questions are about.
	State any `json:"state,omitzero"`
	// Questions are the questions, each under its name, as the model's
	// service takes them.
	Questions any `json:"questions,omitzero"`
	// Decide is the job that puts the questions to the model.
	Decide *machine.JobSpec `json:"decide"`
}

// The job every decision has, and Corbie's script for it.
const (
	decideJob    = "decide"
	decideScript = "corbie:decide.js"
	decideSource = `return llm.decide(input.state, {
  model: input.model.name, service: input.model.service, address: input.model.address,
  key: input.model.secret ? secrets.get(input.model.secret) : null,
  questions: input.questions,
})
`
)

// decides gives the services that have decide: those a decision's model
// may be of.
func decides() []string { return llm.Having(func(s *llm.Service) bool { return s.Decide != nil }) }

// checkDecides says what is wrong with how a decision's model is named.
func (m Model) checkDecides() error {
	name := m.serviceOf()
	service, ok := llm.Find(name)
	holds := strings.Join(decides(), ", ")
	switch {
	case m.Name == "":
		return fmt.Errorf("a decision names its model, as model: { name: jev-latest, secret: TYPESAFE_KEY }")
	case name == "":
		return fmt.Errorf("it is not clear which service the model %q is of: give service: one of %s", m.Name, holds)
	case !ok || service.Decide == nil:
		return fmt.Errorf("the service %q has no decide: a decision's model is of one of %s", name, holds)
	case machine.HasExpr(m.Secret) || machine.HasExpr(m.Name):
		return fmt.Errorf("a model's name and its secret's name are written out")
	}
	return checkAddress(m.addressOf())
}

type decisionType struct{}

func init() { machine.Register(decisionType{}) }

func (decisionType) Name() string { return "decision" }

// Shape is what a decision's definition holds of its own.
func (decisionType) Shape() *machine.Shape {
	var names []string
	for _, name := range decides() {
		service, _ := llm.Find(name)
		names = append(names, service.Known...)
	}
	model := &machine.Shape{Name: "model", What: "a model", Fields: []machine.Field{
		{Name: "name", Kind: machine.KindWord, Like: "jev-latest", Description: "the model's name, as its service knows it", Open: true, Values: names},
		{Name: "secret", Kind: machine.KindWord, Like: "TYPESAFE_KEY", Description: "the name of the secret that is the service's key"},
		{Name: "service", Kind: machine.KindWord, Like: "systemone", Values: decides(),
			Description: "which service's way of talking is spoken; it follows from the name for jev-… models"},
		{Name: "address", Kind: machine.KindWord, Like: "https://models.example",
			Description: "where the service lives, for a model that is not at its service's usual place"},
	}}
	return &machine.Shape{Name: "decision", What: "a decision", Fields: []machine.Field{
		{Name: "model", Kind: machine.KindFields, Of: model, Like: "{ name: jev-latest, secret: TYPESAFE_KEY }", Description: "the model the questions are put to"},
		{Name: "state", Kind: machine.KindValue, Like: "${input.text}", Description: "the text or data the questions are about; an expression of its input"},
		{Name: "questions", Kind: machine.KindValue, Like: "{ urgent: { type: noul, instructions: The customer cannot use the product. } }",
			Description: "the questions, each under its name with its type, its instructions and its criteria"},
	}}
}

// Fill reads a decision as written. Its one job is Corbie's own script,
// written exactly what it needs: the connection to the model's service,
// and the key bound to that connection.
func (decisionType) Fill(p *machine.Parser, f *machine.Fields) any {
	d := &Decision{}
	if mf := f.Fields("model"); mf != nil {
		d.Model = Model{Name: mf.Text("name"), Secret: mf.Text("secret"), Service: mf.Text("service"), Address: mf.Text("address")}
		mf.Done()
	}
	d.State, _ = f.Value("state")
	d.Questions, _ = f.Value("questions")
	p.Builtin(decideScript, decideSource)
	d.Decide = &machine.JobSpec{Script: decideScript, Alone: true}
	if d.Model.checkDecides() == nil {
		d.Decide.Allow = d.Model.permissions()
	}
	return d
}

// Check reports what is wrong with a decision in full form.
func (decisionType) Check(c *machine.Checker, def *machine.Definition) {
	var d Decision
	if !c.DecodeBody(def, &d) {
		return
	}
	if err := d.Model.checkDecides(); err != nil {
		c.Problem("model", "%v", err)
	}
	if d.State == nil {
		c.Problem("state", "a decision gives state:, the text or data its questions are about")
	}
	if questions, written := d.Questions.(map[string]any); d.Questions == nil || (written && len(questions) == 0) {
		c.Problem("questions", "a decision gives questions:, each under its name with its type, its instructions and its criteria")
	}
	c.Value("state", d.State, "input")
	c.Value("questions", d.Questions, "input")
	if d.Decide == nil {
		c.Problem("decide", "a decision needs the job that puts its questions")
	}
	c.Job("decide", d.Decide, "input")
}

// Step takes a decision's step. At its start it asks for its one job, and
// that job's end is the decision's: its output, or its failure.
func (decisionType) Step(in *machine.StepIn) (*machine.StepOut, error) {
	var d Decision
	if err := json.Unmarshal(in.Definition.Body, &d); err != nil {
		return nil, err
	}
	out := &machine.StepOut{}
	for i, event := range in.Events {
		switch {
		case event.Name == machine.Start:
			eval := machine.NewEvaluator(in.Time, in.Seed, in.Run)
			scope := machine.Scope{"input": in.Input}
			state, err := eval.Fill(d.State, scope, "state")
			if err != nil {
				return nil, err
			}
			questions, err := eval.Fill(d.Questions, scope, "questions")
			if err != nil {
				return nil, err
			}
			job, err := d.Decide.Fill(eval, scope, "decide", decideJob)
			if err != nil {
				return nil, err
			}
			job.Input = map[string]any{"state": state, "questions": questions,
				"model": map[string]any{"name": d.Model.Name, "service": d.Model.serviceOf(), "address": d.Model.addressOf(), "secret": d.Model.Secret}}
			out.Requests = append(out.Requests, machine.Request{Job: job})
		case event.From == decideJob && event.Name == machine.Done:
			out.Requests = append(out.Requests, machine.Request{End: &machine.Ending{Output: event.Data}})
		case event.From == decideJob && event.Name == machine.Failed:
			out.Requests = append(out.Requests, machine.Request{End: &machine.Ending{Error: machine.Unhandled(decideJob, event.Data)}})
		default:
			out.Ignored = append(out.Ignored, i)
		}
	}
	return out, nil
}

// Show gives nothing: a decision has one job, which its record shows.
func (decisionType) Show(jsontext.Value) machine.Shown { return machine.Shown{} }

func (decisionType) Manual() string { return decisionManual }
