// Package machines is the plugin that defines a machine while a job runs.
package machines

import "github.com/zummed/corbie/plugin"

func init() {
	plugin.Register(&plugin.Module{Name: "machines", Page: page, Calls: map[string]*plugin.Call{
		"define": {Options: []string{"files"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			source, err := plugin.Text(first, "the definition, as YAML text")
			if err != nil {
				return nil, err
			}
			given, err := opt.Map("files")
			if err != nil {
				return nil, err
			}
			files := map[string]string{}
			for path, contents := range given {
				text, ok := contents.(string)
				if !ok {
					return nil, plugin.Errorf(plugin.BadInput, "files: maps each path to the file's text; %s is not text", path)
				}
				files[path] = text
			}
			ref, err := job.Channel.Define(source, files)
			if err != nil {
				return nil, err
			}
			return plugin.StoredBlob(ref), nil
		}},
	}})
}

const page = `machines: a script uses this plugin to define a machine while its job runs.

1. A script calls machines.define with a definition as YAML text.
2. The runner checks the definition as a submitted one is checked, and stores it.
3. The script returns the stored definition in its output.
4. A later job's machine: is one expression that gives that output, and the
   controller starts the machine as a child run.

A script cannot start a machine.

  machines.define(yaml, { files })    returns the stored definition

Options:
  files   a map from the path of each script the definition names to its text

Errors:
  bad_input   the definition fails its check; the message lists every problem

The child run is allowed only what the job that starts it lists in allow:,
whatever its definition allows. With no allow: there, it is allowed nothing.

Needs nothing: every job may define machines.

The first stage of this flow makes a flow, and the second runs it.

    # plan.yaml
    flow: plan
    stages:
      make: { job: ./make.js }
      made: { job: { machine: "${stages.make.output.definition}", allow: [net:example.com] } }
    output: ${stages.made.output}

    // make.js
    const text = "flow: made\nstages:\n  step: { job: ./step.js }\noutput: ${stages.step.output}\n"
    return { definition: machines.define(text, { files: { "./step.js": "return 1" } }) }

    corbie test plan.yaml
      output: 1
`
