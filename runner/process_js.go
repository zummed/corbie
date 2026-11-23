//go:build js

package runner

import (
	"slices"

	"github.com/zummed/corbie/controller"
	"github.com/zummed/corbie/role"
)

// Built for js/wasm a job runs beside its runner, and a controller's step
// beside its controller. A runner that goes away stops the script.
func init() {
	role.Work = func(command []string, w *role.Wire) {
		if slices.Contains(command, controller.StepArg) {
			controller.StepOn(w)
			return
		}
		jobOn(wire{w}, func(int) {}, nil)
	}
}
