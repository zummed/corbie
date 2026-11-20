//go:build !noui

package main

import (
	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/ui/serve"
)

// The UI is built in, and a build with the tag noui leaves it out.
func init() { cli.Commands = append(cli.Commands, serve.Commands...) }
