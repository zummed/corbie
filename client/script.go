package client

import (
	"encoding/json/v2"
	"strings"

	_ "github.com/zummed/corbie/flow"
	"github.com/zummed/corbie/machine"
)

// ScriptStage is the name of the one stage a script is wrapped in.
const ScriptStage = "script"

// ScriptJob is what a script wrapped by Script may do, and its limits, as
// a definition writes them on a job.
type ScriptJob struct {
	Allow, Deny []string
	Timeout     string // a length of time, such as 7s
	Memory      string // a size, such as 64MB
	Tags        []string
}

// Script wraps a script in a flow of one stage, whose job runs it with
// the run's input and whose output is the flow's. name is the script's
// file name, and the flow is named after it. Nothing is sent.
func Script(name string, script []byte, job ScriptJob) (*machine.Draft, error) {
	written := map[string]any{"script": "./" + name, "input": "${input}"}
	if len(job.Allow) > 0 {
		written["allow"] = job.Allow
	}
	if len(job.Deny) > 0 {
		written["deny"] = job.Deny
	}
	if job.Timeout != "" {
		written["timeout"] = job.Timeout
	}
	if job.Memory != "" {
		written["memory"] = job.Memory
	}
	if len(job.Tags) > 0 {
		written["tags"] = job.Tags
	}
	// Written as JSON, which is YAML too; its first name is the type.
	src, err := json.Marshal(map[string]any{
		"flow":   sanitize(strings.TrimSuffix(name, ".js")),
		"stages": map[string]any{ScriptStage: map[string]any{"job": written}},
		"output": "${stages." + ScriptStage + ".output}",
	}, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	return machine.Parse(src, machine.Given(map[string]string{"./" + name: string(script)}))
}

// sanitize makes a file's name into a machine's name.
func sanitize(name string) string {
	var b strings.Builder
	for i, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' || (i > 0 && (r >= '0' && r <= '9' || r == '-'))
		if ok {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "script"
	}
	return b.String()
}
