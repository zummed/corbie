// Package env is the plugin that gives a script the runner's variables.
package env

import "github.com/zummed/corbie/plugin"

func init() {
	plugin.Register(&plugin.Module{Name: "env", Page: page, Calls: map[string]*plugin.Call{
		"get": {Options: []string{"otherwise"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			name, err := plugin.Text(first, "the variable's name")
			if err != nil {
				return nil, err
			}
			value, set, err := job.Variable(name)
			if err != nil {
				return nil, err
			}
			if !set {
				return opt["otherwise"], nil
			}
			return value, nil
		}},
	}})
}

const page = `env: this plugin reads the runner's environment variables.

  env.get(name, { otherwise })   the value, or otherwise (default null) when the variable is not set

Needs env:NAME for each variable read.

This script reads a variable that is set and one that is not.

    allow: [env:HOME, env:LOG_LEVEL]

    const home = env.get("HOME")
    const level = env.get("LOG_LEVEL", { otherwise: "info" })    // gives "info"
    return { home, level }
`
