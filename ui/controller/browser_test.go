package controller

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zummed/corbie/client/cli"
	"github.com/zummed/corbie/ui/page"
	"github.com/zummed/corbie/ui/page/pagetest"
)

// Test_controller_in_the_browser runs controller_test.js on the Controller
// page of a lab, whose actions run Corbie's commands in this process, as
// the UI runs them. The page reads the store again only when an action
// pokes it: its interval is an hour.
func Test_controller_in_the_browser(t *testing.T) {
	config, ctl, s := labbed(t)
	changed := &signal{}
	command := func(ctx context.Context, c page.Call) page.Answer {
		args := append(append(append(append([]string{}, c.Words...), c.Args...), c.Flags...), "--json", "--config", config)
		var out, said bytes.Buffer
		status := cli.Run(ctx, args, strings.NewReader(c.Stdin), &out, &said)
		changed.poke()
		return page.Answer{Status: status, Out: out.String(), Said: said.String()}
	}
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets", page.Assets()))
	mux.Handle("/", New(Config{Controller: ctl, Store: s, Command: command, Interval: time.Hour, Changed: changed.next, Shell: page.Shell{Offered: []string{"Controller"}}}))
	pagetest.Run(t, pagetest.Config{Handler: mux, Page: "/controller", Module: "controller/controller_test.js"})
}
