//go:build js

package controller

import "github.com/zummed/corbie/role"

// StepOn reads one step over a channel, beside the controller that asked
// for it: built for js/wasm, a process starts no other.
func StepOn(w *role.Wire) {
	var in stepIn
	if err := w.Receive(&in); err != nil {
		return
	}
	w.Send(in.take(), 0, nil)
}
