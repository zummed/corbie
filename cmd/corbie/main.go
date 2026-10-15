// Command corbie is the one binary: what a process does is set by the
// roles it is started with.
package main

import (
	"os"

	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/runner"

	_ "github.com/zummed/corbie/agent"
	_ "github.com/zummed/corbie/flow"
	_ "github.com/zummed/corbie/plugin/bundled"
	_ "github.com/zummed/corbie/statechart"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	os.Exit(cli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
