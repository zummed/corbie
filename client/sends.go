package client

import (
	"fmt"
	"slices"
	"strings"

	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
)

// Unsent gives, by machine, each send and call of the tree's machines that
// the controller would refuse or not deliver, whatever else it holds: one
// to a location whose lifetime the address does not fit, or whose receive:
// does not list the sender, or for which the sender's own allow: and deny:
// do not hold send:. A send whose location is an expression or in another
// tree, or that names a run, is not checked, and neither is a list with an
// expression in it. empty gives each location of the tree with no machine
// that a send or a call names, with where: such a send may be one a
// condition leaves out, or wait for a file still to come.
func (t *Tree) Unsent() (problems map[*Machine][]string, empty map[string][]string) {
	at := map[string]*Machine{}
	for _, m := range t.Machines {
		at[m.Location] = m
	}
	problems, empty = map[*Machine][]string{}, map[string][]string{}
	for _, m := range t.Machines {
		if m.Draft == nil {
			continue
		}
		s := &sender{tree: t, at: at, from: m.Location, path: m.Path}
		s.draft(m.Draft, m.Path, "", nil)
		if len(s.problems) > 0 {
			problems[m] = s.problems
		}
		for _, e := range s.empty {
			if !slices.Contains(empty[e.at], e.place) {
				empty[e.at] = append(empty[e.at], e.place)
			}
		}
	}
	return problems, empty
}

// A sender checks the sends of one machine of a tree, and of the machines
// it starts, which run at its location.
type sender struct {
	tree     *Tree
	at       map[string]*Machine
	from     string
	path     string // the machine's file
	problems []string
	empty    []struct{ at, place string }
}

// draft checks a definition's sends and those of its children. file and
// place say where the definition is written, and above are the lists of the
// jobs that started it, each of which narrows it.
func (s *sender) draft(d *machine.Draft, file, place string, above []permission.Layer) {
	def := d.Definition
	name := fmt.Sprintf("the definition of %q", def.Name)
	own := permission.Layer{Name: name, Allow: def.Allow, Deny: def.Deny, Fix: "add the line %s to the allow list of " + name}
	for _, send := range machine.Sends(def) {
		layer := own
		if job := send.Job; job != nil {
			// A job has the definition's lists with its own.
			at := strings.TrimSuffix(strings.TrimSuffix(send.Place, ".send.to"), ".call")
			layer.Name = fmt.Sprintf("the job %s in the definition of %q", at, def.Name)
			layer.Fix = "add the line %s to the allow list of " + layer.Name
			layer.Allow = append(append([]string{}, def.Allow...), job.Allow...)
			layer.Deny = append(append([]string{}, def.Deny...), job.Deny...)
		}
		why, empty := s.check(send, append([]permission.Layer{layer}, above...))
		if why != "" {
			line := place + send.Place + ": " + why
			if file != s.path {
				line = file + ": " + line
			}
			if !slices.Contains(s.problems, line) {
				s.problems = append(s.problems, line)
			}
		}
		if empty != "" {
			s.empty = append(s.empty, struct{ at, place string }{empty, file + " " + place + send.Place})
		}
	}
	for _, job := range machine.Jobs(def) {
		child := d.Children[job.Job.Machine]
		if child == nil {
			continue
		}
		narrowed := above
		if len(job.Job.Allow)+len(job.Job.Deny) > 0 {
			name := fmt.Sprintf("the job %s in the definition of %q", job.Place, def.Name)
			narrowed = append([]permission.Layer{{Name: name, Allow: job.Job.Allow, Deny: job.Job.Deny, Fix: "add the line %s to the allow list of " + name}}, above...)
		}
		if strings.HasPrefix(job.Job.Machine, "in-place-") {
			s.draft(child, file, place+job.Place+".machine.", narrowed)
		} else {
			s.draft(child, strings.TrimPrefix(job.Job.Machine, "./"), "", narrowed)
		}
	}
}

// check gives why a send or a call is refused or not delivered, or "", and
// the location it names where the tree has no machine there.
func (s *sender) check(send machine.PlacedSend, layers []permission.Layer) (why, empty string) {
	to := send.To
	if to.Expr != "" || to.Run != "" || machine.HasExpr(to.At) {
		return "", ""
	}
	at := machine.Resolve(s.tree.Prefix, to.At)
	if at != s.tree.Prefix && !strings.HasPrefix(at, s.tree.Prefix+"/") {
		return "", ""
	}
	var known []permission.Layer
	for _, layer := range layers {
		if !hasExpr(layer.Allow) {
			known = append(known, layer)
		}
	}
	if r := (&permission.Set{Layers: known, Prefix: s.tree.Prefix}).Check(permission.Send, at); r != nil {
		return r.Error(), ""
	}
	target := s.at[at]
	switch {
	case target == nil:
		return "", at
	case target.Draft == nil:
		return "", ""
	}
	def := target.Draft.Definition
	sender := &machine.Address{At: s.from}
	if send.Job != nil && send.Job.IsCall() {
		switch {
		case def.Lifetime != machine.LifetimeTransient:
			return fmt.Sprintf("%s is %s: a call starts a run only of a transient location", at, def.Lifetime), ""
		case !machine.ReceivesFrom(def, s.tree.Prefix, sender, ""):
			return fmt.Sprintf("%s does not receive a call from %s", at, s.from), ""
		}
		return "", ""
	}
	if why := machine.Addressable(at, def.Lifetime, to.Key != ""); why != "" {
		return why, ""
	}
	if !machine.Receives(def) && def.Lifetime != machine.LifetimeTransient {
		// A send to a transient flow or agent starts a run of it.
		return fmt.Sprintf("%s is a %s, which takes no sent event", at, def.Type), ""
	}
	if machine.Owns(def, send.Event) {
		return fmt.Sprintf("%q is the name of an event %s gives itself", send.Event, at), ""
	}
	if !machine.ReceivesFrom(def, s.tree.Prefix, sender, send.Event) {
		return fmt.Sprintf("%s does not receive %s from %s", at, send.Event, s.from), ""
	}
	return "", ""
}

// hasExpr says whether a list has an entry with an expression, which only
// a run fills in.
func hasExpr(list []string) bool {
	for _, entry := range list {
		if machine.HasExpr(entry) {
			return true
		}
	}
	return false
}
