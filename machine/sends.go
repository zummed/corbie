package machine

import (
	"fmt"
	"strings"

	"github.com/zummed/corbie/permission"
)

// A PlacedSend is a send or a call a definition writes, with its place: a
// send job, a call job or a chart's send action.
type PlacedSend struct {
	Place string
	To    *AddressSpec
	// Event is the name of the event sent, and empty for a call.
	Event string
	// Job is the send or call job, and nil for a chart's send action.
	Job *JobSpec
}

// Sends gives every send and call a definition writes, in the order the
// check visits them. Its child definitions are not read.
func Sends(def *Definition) []PlacedSend { return check(def).sends }

// SendTo checks the address of a chart's send action, as Address does, and
// notes the send.
func (c *Checker) SendTo(place string, to *AddressSpec, event string, names ...string) {
	c.sends = append(c.sends, PlacedSend{Place: place, To: to, Event: event})
	c.Address(place, to, names...)
}

// Addressable says why a send to { at }, or to { at, key } when keyed, goes
// to no run at a location of a lifetime, or "" where it may: { at } goes to
// a singleton's run or starts a new run of a transient location, and
// { at, key } goes to a keyed location's run for the key.
func Addressable(at, lifetime string, keyed bool) string {
	switch {
	case !keyed && lifetime == LifetimeKeyed:
		return fmt.Sprintf("%s is keyed: a send to it names a key, as %s#<key>", at, at)
	case keyed && lifetime != LifetimeKeyed:
		return fmt.Sprintf("%s is not keyed", at)
	}
	return ""
}

// ReceivesFrom says whether a definition's receive: lists a sender for an
// event, its locations read from the prefix of the tree the definition is
// in. With no receive:, every location of the tree is listed. A call is the
// event "": a list takes it, and a mapping, which names events, does not.
// The tree's own client, which is always listed, is the caller's to know.
func ReceivesFrom(def *Definition, tree string, sender *Address, event string) bool {
	if sender == nil {
		return false
	}
	list := []string{"**"}
	if len(def.Receive) > 0 {
		var ok bool
		if list, ok = def.Receive[event]; !ok {
			list = def.Receive[""]
		}
	}
	for _, entry := range list {
		if name, ok := strings.CutPrefix(entry, "client:"); ok {
			if sender.Client != "" && sender.Client == name {
				return true
			}
			continue
		}
		pattern, err := permission.ReadLocation(tree, entry)
		if err == nil && sender.At != "" && permission.MatchLocation(pattern, sender.At) {
			return true
		}
	}
	return false
}
