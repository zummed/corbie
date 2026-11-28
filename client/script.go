package client

import (
	"encoding/json/v2"
	"path"
	"strings"

	"github.com/zummed/corbie/machine"
	_ "github.com/zummed/corbie/machine/flow"
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
	// Files are the files the script loads, each path as a job's files:
	// writes it.
	Files []string
}

// Script wraps the script file at file, a path from the context, in a flow
// of one stage, whose job runs it with the run's input and whose output is
// the flow's. The flow is named after the file, and read as a definition
// beside it. load gives a file by its path from the context: the script and
// each of job.Files. Nothing is sent.
func Script(file string, load func(path string) ([]byte, error), job ScriptJob) (*machine.Draft, error) {
	src, err := scriptFlow(path.Base(file), job)
	if err != nil {
		return nil, err
	}
	return machine.ParseIn(strings.TrimSuffix(file, ".js")+".yaml", src, load)
}

// scriptFlow writes the flow a script is wrapped in. It is JSON, which is
// YAML too, and its first name is the type.
func scriptFlow(name string, job ScriptJob) ([]byte, error) {
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
	if len(job.Files) > 0 {
		written["files"] = job.Files
	}
	return json.Marshal(map[string]any{
		"flow":   sanitize(strings.TrimSuffix(name, ".js")),
		"stages": map[string]any{ScriptStage: map[string]any{"job": written}},
		"output": "${stages." + ScriptStage + ".output}",
	}, json.Deterministic(true))
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
