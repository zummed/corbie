package llm

import (
	"encoding/json/v2"
	"slices"
)

// changed is how the tools one turn is offered differ from those the turn
// before was: the tools added, each whole, and the names of those removed.
// A tool whose definition differs is added again under its name.
type changed struct {
	added   []any
	removed []string
}

func (c changed) none() bool { return len(c.added) == 0 && len(c.removed) == 0 }

// value gives the change as a reply holds it, and as a part of the
// conversation keeps it.
func (c changed) value() map[string]any {
	removed := []any{}
	for _, name := range c.removed {
		removed = append(removed, name)
	}
	added := c.added
	if added == nil {
		added = []any{}
	}
	return map[string]any{"added": added, "removed": removed}
}

// changeOf reads a change from a part of the conversation, where the part
// holds one.
func changeOf(part any) (changed, bool) {
	held, ok := field(part, "offered").(map[string]any)
	if !ok {
		return changed{}, false
	}
	c := changed{added: list(held["added"])}
	for _, name := range list(held["removed"]) {
		c.removed = append(c.removed, text(name))
	}
	return c, true
}

// offering is the tools a turn is offered, by name: each as its definition
// written one way.
type offering map[string]string

func written(tool any) string {
	data, _ := json.Marshal(map[string]any{"name": field(tool, "name"), "description": field(tool, "description"), "args": field(tool, "args")}, json.Deterministic(true))
	return string(data)
}

func (o offering) apply(c changed) {
	for _, name := range c.removed {
		delete(o, name)
	}
	for _, tool := range c.added {
		o[text(field(tool, "name"))] = written(tool)
	}
}

// since gives how a list of tools differs from the offering.
func (o offering) since(tools []any) changed {
	var c changed
	now := map[string]bool{}
	for _, tool := range tools {
		name := text(field(tool, "name"))
		now[name] = true
		if o[name] != written(tool) {
			c.added = append(c.added, map[string]any{"name": name, "description": field(tool, "description"), "args": field(tool, "args")})
		}
	}
	for name := range o {
		if !now[name] {
			c.removed = append(c.removed, name)
		}
	}
	slices.Sort(c.removed)
	return c
}

// offeredSince gives how the tools of the turn being made differ from
// those the last turn of the conversation was offered. It is known only
// where every reply in the conversation holds what its turn was offered.
func offeredSince(said []any, tools []any) (changed, bool) {
	o := offering{}
	for _, part := range said {
		if field(part, "role") != "assistant" {
			continue
		}
		c, ok := changeOf(part)
		if !ok {
			return changed{}, false
		}
		o.apply(c)
	}
	return o.since(tools), true
}
