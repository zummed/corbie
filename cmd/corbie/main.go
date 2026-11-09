// Command corbie is the one binary: what a process does is set by the
// roles it is started with.
package main

import (
	"os"
	"time"

	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/runner"

	_ "github.com/zummed/corbie/machine/agent"
	_ "github.com/zummed/corbie/machine/flow"
	_ "github.com/zummed/corbie/machine/statechart"
	_ "github.com/zummed/corbie/plugin/bundled"
)

func main() {
	// Every process of the binary keeps time in UTC, whatever zone its
	// computer is set to: scripts, expressions and what the command prints.
	time.Local = time.UTC
	if len(os.Args) > 1 && os.Args[1] == runner.JobArg {
		runner.JobMain()
		return
	}
	os.Exit(cli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
