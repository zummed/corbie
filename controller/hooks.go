package controller

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/store"
)

// A hook is a job a definition names that runs before a job starts or after
// it has ended done, for the jobs of its machine and of the machines below
// it. Each runs once for one job.

// hookSets gives the hooks that run for the instance's jobs, the machine's
// own first and then those of each machine above, the nearest first, with
// the input each is filled from and where it is: -1 for the machine's own,
// and otherwise its place among those from above.
func (b *build) hookSets() (sets []map[string]*machine.HookSpec, inputs []any, levels []int) {
	if len(b.st.Hooks) > 0 {
		sets, inputs, levels = append(sets, b.st.Hooks), append(inputs, b.st.Input), append(levels, -1)
	}
	for i, above := range b.st.Above.Hooks {
		sets, inputs, levels = append(sets, above.Hooks), append(inputs, above.Input), append(levels, i)
	}
	return sets, inputs, levels
}

// hasBefore says whether any hook, the machine's own or one from above,
// runs before a job.
func (b *build) hasBefore() bool {
	sets, _, _ := b.hookSets()
	return slices.ContainsFunc(sets, machine.HasBefore)
}

// seen is what a hook reads of the job it runs for, as job.
func (b *build) seen(job *instance.Going) map[string]any {
	name, _, _ := strings.Cut(job.Name, ".")
	what := map[string]any{
		"name":    name,
		"run":     map[string]any{"id": b.inst, "machine": b.st.Machine},
		"approve": b.marked(job),
	}
	if job.Input != nil {
		what["input"] = job.Input
	}
	if len(job.Allow) > 0 {
		what["allow"] = toList(job.Allow)
	}
	if job.Meta != nil {
		what["meta"] = job.Meta
	}
	if job.Tool != nil {
		what["tool"] = job.Tool
	}
	switch {
	case job.IsMachine():
		what["machine"] = job.Machine
	case job.IsCall():
		what["call"] = map[string]any{"at": job.Call.At}
	case job.IsSend():
		to := map[string]any{"at": job.Send.At.At}
		if job.Send.At.Key != nil {
			to["key"] = *job.Send.At.Key
		}
		if job.Send.At.Run != "" {
			to["run"] = job.Send.At.Run
		}
		if job.Send.At.Client != "" {
			to["client"] = job.Send.At.Client
		}
		what["send"] = map[string]any{"to": to, "event": job.Send.Event, "data": job.Send.Data}
	default:
		what["script"] = job.Script
	}
	return what
}

// hookRun is what a hook's expressions read as run: the run of the machine
// that declares the hook.
func (b *build) hookRun(level int) machine.Run {
	run := b.run()
	if level >= 0 {
		above := b.st.Above.Hooks[level]
		run.ID, run.Key, run.Principal = above.Run, above.Key, above.Principal
	}
	return run
}

// hookScope is what the expressions of a hook read: the input of the
// machine that declares it, the job, and after a job its output.
func (b *build) hookScope(job *instance.Going, input any, after bool) machine.Scope {
	scope := machine.Scope{"input": input, "job": b.seen(job)}
	if after {
		scope["output"] = job.Outcome.Data
	}
	return scope
}

// hooksFor gives the hooks that run for a job, before it or after it, in
// the order they run: the machine's own in the order of their names, then
// those of each machine above. A hook's own job takes no hook. A when: that
// cannot be evaluated is an error that names its hook.
func (b *build) hooksFor(job *instance.Going, after bool) ([]instance.HookRef, error) {
	if job.Hook != "" {
		return nil, nil
	}
	var refs []instance.HookRef
	sets, inputs, levels := b.hookSets()
	for i, set := range sets {
		for _, name := range sortedKeys(set) {
			h := set[name]
			if (after && h.After == nil) || (!after && h.Before == nil) {
				continue
			}
			if h.When != "" {
				eval := machine.NewEvaluator(b.time, b.seed("hook-when", fmt.Sprint(b.step.Number), job.Full(), name), b.hookRun(levels[i]))
				holds, err := eval.Cond(h.When, b.hookScope(job, inputs[i], after), "hooks."+name+".when")
				if err != nil {
					return nil, fmt.Errorf("the hook %s could not be started: %v", name, err)
				}
				if !holds {
					continue
				}
			}
			refs = append(refs, instance.HookRef{Level: levels[i], Name: name, After: after})
		}
	}
	return refs, nil
}

// startHook starts the next hook of a job. One that cannot be started
// refuses the job as one that said no does. a is the arrival the step is
// acting on, or nil while a step's requests are taken.
func (b *build) startHook(job *instance.Going, a *arrival) {
	ref := job.Hooks[0]
	hook, err := b.fillHook(job, ref)
	if err != nil {
		if b.err != nil {
			return
		}
		job.Hooks = job.Hooks[1:]
		b.hookSaid(job, ref, false, fmt.Sprintf("the hook %s could not be started: %v", ref.Name, err), nil, false, a)
		return
	}
	if b.st.Asked == nil {
		b.st.Asked = map[string]int{}
	}
	b.st.Asked[hook.Name]++
	going := &instance.Going{Job: *hook, Asked: b.st.Asked[hook.Name], Attempt: 1, For: job.Key()}
	b.st.Jobs[going.Key()] = going
	if going.IsMachine() && b.st.Above.Depth+1 > b.st.Above.MaxDepth {
		b.fail(going, "depth limit reached: a root may have %d levels of machines below it", b.st.Above.MaxDepth)
		return
	}
	b.launch(going)
}

// fillHook fills a hook's job in, from the input of the machine that
// declares it and from the job it runs for. Its script or machine is in the
// definition that declares it, and its job has the layers of that machine
// and, as a job of that definition does, the lists written for the whole of
// it.
func (b *build) fillHook(job *instance.Going, ref instance.HookRef) (*machine.Job, error) {
	hooks, input, def, beside, run := b.st.Hooks, b.st.Input, b.def, b.st.Definition, b.hookRun(ref.Level)
	// The lists written for the whole definition, and why they could not be
	// filled in where they could not.
	var allow []string
	var unfilled error
	var err error
	if ref.Level >= 0 {
		above := b.st.Above.Hooks[ref.Level]
		hooks, input, beside = above.Hooks, above.Input, above.Definition
		if def, err = b.p.definitionAt(beside); err != nil {
			if !errors.Is(err, machine.ErrDefinition) {
				b.err = err
			}
			return nil, err
		}
		allow, unfilled = b.allowOf(def, input, run, b.seed("hook-allow", fmt.Sprint(ref.Level)))
	} else {
		if err = b.ownDefinition(); err != nil {
			return nil, err
		}
		if def, allow = b.def, b.read.Allow; b.read.Unfilled != "" {
			unfilled = errors.New(b.read.Unfilled)
		}
	}
	spec, place := hooks[ref.Name].Before, "hooks."+ref.Name+".before"
	if ref.After {
		spec, place = hooks[ref.Name].After, "hooks."+ref.Name+".after"
	}
	scope := b.hookScope(job, input, ref.After)
	name := job.Name + "." + ref.Name
	eval := machine.NewEvaluator(b.time, b.seed("hook", fmt.Sprint(b.step.Number), job.Full(), ref.Name), run)
	filled, err := spec.Fill(eval, scope, place, name)
	if err != nil {
		return nil, err
	}
	// A hook that writes no input is given the job, and after it the output.
	if spec.Input == nil && !filled.IsSend() {
		given := map[string]any{"job": scope["job"]}
		if ref.After {
			given["output"] = scope["output"]
		}
		filled.Input = given
	}
	filled.Name, filled.Hook, filled.Above, filled.Level = name, ref.Name, ref.Level >= 0, max(ref.Level, 0)
	if !filled.IsMachine() {
		// A script, a call or a send of the definition has the lists written
		// for the whole of it.
		if unfilled != nil {
			return nil, unfilled
		}
		filled.Allow = append(slices.Clone(allow), filled.Allow...)
		filled.Deny = append(slices.Clone(def.Deny), filled.Deny...)
	}
	if !filled.IsCall() && !filled.IsSend() {
		if filled.Found, err = found(filled, def, beside); err != nil {
			return nil, err
		}
		return filled, nil
	}
	// A call or a send is checked against the hook's own lists and the
	// layers of the machine that declares it.
	own := permission.Layer{Name: fmt.Sprintf("the hook %q in the definition of %q", ref.Name, def.Name), Allow: filled.Allow, Deny: filled.Deny}
	own.Fix = "add the line %s to the allow list of " + own.Name
	at, why := "", ""
	if filled.IsCall() {
		if at = filled.Call.At; !strings.HasPrefix(at, "/") && b.st.Tree == "" {
			return nil, fmt.Errorf("%s is read from the tree's prefix, and the run is in no tree", at)
		}
		at = machine.Resolve(b.st.Tree, at)
		if err := store.CheckLocation(at); err != nil {
			return nil, err
		}
	} else if at, why = b.addressAt(filled.Send.At); why != "" {
		// A send that cannot be delivered is no failure: it is noted as the
		// job is carried out.
		return filled, nil
	}
	refused, err := b.refusedSend(at, own, layersAbove(b.st, filled))
	if err != nil {
		b.err = err
		return nil, err
	}
	if refused != nil {
		return nil, errors.New(refused.Error)
	}
	if filled.IsCall() {
		filled.Call = &machine.Address{At: at}
	} else {
		to := *filled.Send.At
		to.At = at
		filled.Send = &machine.Send{At: &to, Event: filled.Send.Event, Data: filled.Send.Data}
	}
	return filled, nil
}

// ownDefinition reads the instance's definition and the lists written for
// the whole of it, where no step of the machine has read them.
func (b *build) ownDefinition() error {
	var err error
	if b.def == nil {
		if b.def, err = b.p.definition(b.st); err != nil {
			if !errors.Is(err, machine.ErrDefinition) {
				b.err = err
			}
			return err
		}
	}
	if b.read == nil {
		b.read = &stepOut{}
		if b.read.Allow, err = b.definitionAllow(); err != nil {
			b.read.Unfilled = err.Error()
		}
	}
	return nil
}

// hookEnded acts on how a hook's job ended. { ok: true } is a yes. Any other
// result, and a failure, is a no. A send returns nothing, so a hook that is
// one says yes.
func (b *build) hookEnded(hook *instance.Going, outcome machine.Event, a *arrival) {
	b.use(a)
	b.record(machine.Event{Name: outcome.Name, From: hook.Full(), Data: outcome.Data}, a, true)
	job, ok := b.st.Jobs[hook.For]
	if !ok || len(job.Hooks) == 0 || hook.Hook != job.Hooks[0].Name || hook.Name != job.Name+"."+hook.Hook {
		return
	}
	ref := job.Hooks[0]
	job.Hooks = job.Hooks[1:]
	said, _ := outcome.Data.(map[string]any)
	yes := outcome.Name == machine.Done && (said["ok"] == true || hook.IsSend())
	note, _ := said["note"].(string)
	switch {
	case yes:
	case outcome.Name == machine.Failed:
		note = fmt.Sprintf("the hook %s failed: %v", ref.Name, said["error"])
	case said["ok"] != false:
		note = fmt.Sprintf("the hook %s gave neither { ok: true } nor { ok: false, note }", ref.Name)
	case note == "":
		note = fmt.Sprintf("the hook %s refused it", ref.Name)
	default:
		note = fmt.Sprintf("the hook %s refused it: %s", ref.Name, note)
	}
	key := "input"
	if ref.After {
		key = "output"
	}
	replace, has := said[key]
	b.hookSaid(job, ref, yes, note, replace, has && yes, a)
}

// hookSaid acts on a hook's answer for a job. Before a job, a yes lets it go
// on to its next hook and then start, with the input the hook gave in place
// of its own, and a no denies it. After a job, a yes gives the machine the
// job's outcome, with the output the hook gave in place of its own, and a no
// fails the job.
func (b *build) hookSaid(job *instance.Going, ref instance.HookRef, yes bool, note string, replace any, has bool, a *arrival) {
	if !ref.After {
		switch {
		case !yes:
			job.Held, job.Hooks = false, nil
			b.outcome(job, &machine.Error{Status: machine.StatusDenied, Error: note}, a)
			return
		case has:
			if !job.Changed {
				job.Changed, job.Given = true, job.Input
			}
			job.Input = replace
		}
		if len(job.Hooks) > 0 {
			b.startHook(job, a)
			return
		}
		job.Held = false
		b.launch(job)
		return
	}
	switch {
	case !yes:
		job.Hooks = nil
		b.outcome(job, &machine.Error{Status: machine.StatusError, Error: note}, a)
		return
	case has:
		job.Outcome = &machine.Event{Name: machine.Done, From: job.Name, Data: replace}
	}
	if len(job.Hooks) > 0 {
		b.startHook(job, a)
		return
	}
	b.forget(job)
	b.record(*job.Outcome, a, false)
	b.events = append(b.events, *job.Outcome)
	b.sources = append(b.sources, a)
}

// outcome gives the machine the failure of a job a hook refused: in this
// step where the step is acting on an arrival, and as a message otherwise.
func (b *build) outcome(job *instance.Going, failure *machine.Error, a *arrival) {
	if a == nil {
		b.ask(instance.Ask{Kind: instance.AskFail, Name: job.Full(), Message: b.message("fail"), Error: failure})
		return
	}
	b.forget(job)
	event := machine.Event{Name: machine.Failed, From: job.Name, Data: failure.Value()}
	b.record(event, a, false)
	b.events = append(b.events, event)
	b.sources = append(b.sources, a)
}

// dropHooks cancels the hooks running for a job that is no longer wanted.
func (b *build) dropHooks(job *instance.Going) {
	for _, key := range sortedKeys(b.st.Jobs) {
		if hook := b.st.Jobs[key]; hook.For == job.Key() {
			b.cancel(hook)
			b.unwakeJob(hook)
			b.forget(hook)
		}
	}
}
