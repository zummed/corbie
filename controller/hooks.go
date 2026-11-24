package controller

import (
	"errors"
	"fmt"
	"slices"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
)

// A prototype of hooks: jobs a definition names that run before a job
// starts and after it has ended, for the jobs of its machine and of the
// machines below it.

// hooked says whether any hook applies to the instance's jobs: its own
// definition's or one from above.
func (b *build) hooked() bool { return len(b.st.Hooks)+len(b.st.Above.Hooks) > 0 }

// hookSets gives the hooks that apply, the machine's own first and then
// those of each machine above, the nearest first, with the input each is
// filled from.
func (b *build) hookSets() (sets []map[string]*machine.HookSpec, inputs []any, levels []int) {
	if len(b.st.Hooks) > 0 {
		sets, inputs, levels = append(sets, b.st.Hooks), append(inputs, b.st.Input), append(levels, -1)
	}
	for i, above := range b.st.Above.Hooks {
		sets, inputs, levels = append(sets, above.Hooks), append(inputs, above.Input), append(levels, i)
	}
	return sets, inputs, levels
}

// seen is what a hook reads of the job it runs for.
func (b *build) seen(job *instance.Going) map[string]any {
	what := map[string]any{"name": job.Name, "machine": b.st.Machine, "run": b.inst, "approve": b.holds(job)}
	if job.Input != nil {
		what["input"] = job.Input
	}
	if len(job.Allow) > 0 {
		what["allow"] = toList(job.Allow)
	}
	if job.Meta != nil {
		what["meta"] = job.Meta
	}
	if shown, ok := job.Shown.(map[string]any); ok {
		tool := map[string]any{}
		for k, v := range shown {
			tool[k] = v
		}
		if name, ok := tool["tool"]; ok {
			tool["name"] = name
			delete(tool, "tool")
		}
		what["tool"] = tool
	}
	switch {
	case job.IsMachine():
		what["machine_job"] = job.Machine
	case job.IsCall():
		what["call"] = map[string]any{"at": job.Call.At}
	case job.IsSend():
		what["send"] = map[string]any{"to": map[string]any{"at": job.Send.At.At}, "event": job.Send.Event, "data": job.Send.Data}
	default:
		what["script"] = job.Script
	}
	return what
}

// resultOf is what a hook after a job reads of how it ended.
func resultOf(outcome *machine.Event) map[string]any {
	if outcome.Name == machine.Done {
		return map[string]any{"status": "done", "output": outcome.Data}
	}
	result := map[string]any{"status": "failed"}
	if failure, ok := outcome.Data.(map[string]any); ok {
		for _, k := range []string{"error", "status", "output"} {
			if v, has := failure[k]; has {
				if k == "status" {
					k = "why"
				}
				result[k] = v
			}
		}
	}
	return result
}

// hooksFor gives the hooks that run for a job, before it or after it, in
// the order they run: the machine's own by name, then those of each machine
// above.
func (b *build) hooksFor(job *instance.Going, after bool) ([]instance.HookRef, error) {
	if job.For != "" || job.Name == instance.AskName {
		return nil, nil
	}
	scope := machine.Scope{"job": b.seen(job)}
	if after {
		scope["result"] = resultOf(job.Outcome)
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
				scope["input"] = inputs[i]
				eval := machine.NewEvaluator(b.time, b.seed("hook-when", fmt.Sprint(b.step.Number), job.Full(), name), b.run())
				holds, err := eval.Cond(h.When, scope, "hooks."+name+".when")
				if err != nil {
					return nil, err
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
// declares it and from the job it runs for.
func (b *build) fillHook(job *instance.Going, ref instance.HookRef) (*machine.Job, error) {
	hooks, input, def, beside := b.st.Hooks, b.st.Input, b.def, b.st.Definition
	if ref.Level >= 0 {
		above := b.st.Above.Hooks[ref.Level]
		hooks, input, beside = above.Hooks, above.Input, above.Definition
		var err error
		if def, err = b.p.definitionAt(beside); err != nil {
			if !errors.Is(err, machine.ErrDefinition) {
				b.err = err
			}
			return nil, err
		}
	} else if err := b.ownDefinition(); err != nil {
		return nil, err
	} else {
		def = b.def
	}
	spec, place := hooks[ref.Name].Before, "hooks."+ref.Name+".before"
	scope := machine.Scope{"input": input, "job": b.seen(job)}
	given := map[string]any{"job": scope["job"]}
	if ref.After {
		spec, place = hooks[ref.Name].After, "hooks."+ref.Name+".after"
		scope["result"] = resultOf(job.Outcome)
		given["result"] = scope["result"]
	}
	name := job.Name + "." + ref.Name
	eval := machine.NewEvaluator(b.time, b.seed("hook", fmt.Sprint(b.step.Number), job.Full(), ref.Name), b.run())
	filled, err := spec.Fill(eval, scope, place, name)
	if err != nil {
		return nil, err
	}
	// A hook that writes no input is given the job, and after it the result.
	if spec.Input == nil && !filled.IsSend() {
		filled.Input = given
	}
	filled.Name, filled.Hook, filled.Above, filled.Level = name, ref.Name, ref.Level >= 0, max(ref.Level, 0)
	if ref.Level < 0 && !filled.IsMachine() {
		// A script, a call or a send of the definition has the lists written
		// for the whole of it.
		if b.read.Unfilled != "" {
			return nil, errors.New(b.read.Unfilled)
		}
		filled.Allow = append(slices.Clone(b.read.Allow), filled.Allow...)
		filled.Deny = append(slices.Clone(b.def.Deny), filled.Deny...)
	}
	if filled.IsCall() {
		at, failure, err := b.callTo(&instance.Going{Job: *filled})
		if err != nil {
			b.err = err
			return nil, err
		}
		if failure != nil {
			return nil, errors.New(failure.Error)
		}
		filled.Call = &machine.Address{At: at}
		return filled, nil
	}
	if filled.IsSend() {
		return filled, nil
	}
	if filled.Found, err = found(filled, def, beside); err != nil {
		return nil, err
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

// hookEnded acts on how a hook's job ended.
func (b *build) hookEnded(hook *instance.Going, outcome machine.Event, a *arrival) {
	b.use(a)
	b.record(machine.Event{Name: outcome.Name, From: hook.Full(), Data: outcome.Data}, a, true)
	job, ok := b.st.Jobs[hook.For]
	if !ok || len(job.Hooks) == 0 || hook.Name != job.Name+"."+job.Hooks[0].Name {
		return
	}
	ref := job.Hooks[0]
	job.Hooks = job.Hooks[1:]
	said, _ := outcome.Data.(map[string]any)
	// A send returns nothing, so a hook that is one refuses nothing.
	yes := outcome.Name == machine.Done && (said["ok"] == true || hook.IsSend())
	note, _ := said["note"].(string)
	switch {
	case outcome.Name == machine.Failed:
		note = fmt.Sprintf("the hook %s failed: %v", ref.Name, said["error"])
	case !yes && said["ok"] != false:
		note = fmt.Sprintf("the hook %s gave no { ok: true } and no { ok: false, note }", ref.Name)
	case !yes && note == "":
		note = fmt.Sprintf("the hook %s refused it", ref.Name)
	case !yes:
		note = fmt.Sprintf("the hook %s refused it: %s", ref.Name, note)
	}
	key := "input"
	if ref.After {
		key = "output"
	}
	replace, has := said[key]
	b.hookSaid(job, ref, yes, note, replace, has && yes, a)
}

// hookSaid acts on a hook's answer for a job: before it, a yes starts the
// job once every hook has said yes, with the input one gave in place of its
// own; after it, a yes gives the machine the job's outcome, with the output
// one gave in place of its own. A no before a job denies it, and a no after
// it fails it.
func (b *build) hookSaid(job *instance.Going, ref instance.HookRef, yes bool, note string, replace any, has bool, a *arrival) {
	if !ref.After {
		switch {
		case !yes:
			job.Held, job.Hooks = false, nil
			b.outcome(job, machine.Event{Name: machine.Failed, From: job.Name, Data: (&machine.Error{Status: machine.StatusDenied, Error: note}).Value()}, a)
			return
		case has:
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
		job.Outcome = &machine.Event{Name: machine.Failed, From: job.Name, Data: (&machine.Error{Status: machine.StatusError, Error: note}).Value()}
	case has:
		job.Outcome = &machine.Event{Name: machine.Done, From: job.Name, Data: replace}
	}
	if len(job.Hooks) > 0 && yes {
		b.startHook(job, a)
		return
	}
	job.Hooks = nil
	b.outcome(job, *job.Outcome, a)
}

// outcome gives the machine a hooked job's outcome: in this step where the
// step is acting on an arrival, and as a message otherwise.
func (b *build) outcome(job *instance.Going, event machine.Event, a *arrival) {
	if a == nil {
		failure := &machine.Error{Status: machine.StatusDenied}
		if data, ok := event.Data.(map[string]any); ok {
			failure.Error, _ = data["error"].(string)
		}
		b.ask(instance.Ask{Kind: instance.AskFail, Name: job.Full(), Message: b.message("fail"), Error: failure})
		return
	}
	b.forget(job)
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

var _ = blob.Ref{}
