package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/role"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// cmdUp is corbie up: a controller, a runner, a client and a gateway in one
// process, set up as corbie init sets them up, in the config .corbie of the
// current directory with the store in it, and a directory submitted. A
// command run in the current directory uses that config.
func cmdUp(e *Env, args []string) error {
	var s starting
	var in, at, ceilingText string
	var values, valuesFromEnv list
	e.Flags.StringVar(&in, "in", ".", "")
	e.Flags.StringVar(&at, "at", "", "")
	e.Flags.StringVar(&ceilingText, "ceiling", "all", "")
	e.Flags.Var(&values, "value", "")
	e.Flags.Var(&valuesFromEnv, "value-env", "")
	e.Flags.Var(&s.secrets, "secret", "")
	e.Flags.Var(&s.secretsFromEnv, "secret-env", "")
	mine, named, err := items(e, args)
	if err != nil {
		return err
	}
	if _, err := e.Parse(mine); err != nil {
		return err
	}
	if role.Serverless() {
		return fmt.Errorf("corbie up needs a process that stays up, and this is a serverless host")
	}
	if e.Given("at") && at == "" {
		return Mistake("--at is empty: a prefix is / and the names of one or more directories, as /home")
	}
	dir, err := filepath.Abs(in)
	if err != nil {
		return err
	}
	tree, err := client.ReadTree(dir, at)
	if err != nil {
		return Mistake("%v", err)
	}
	// A file with a problem is found before anything starts.
	problems := 0
	for _, m := range tree.Machines {
		if m.Problem != nil {
			problems++
			e.Say("%s", m.Path)
			for _, line := range strings.Split(m.Problem.Error(), "\n") {
				e.Say("    %s", line)
			}
		}
	}
	if problems > 0 {
		return fmt.Errorf("%d of %d files have problems, and nothing is started", problems, len(tree.Machines))
	}
	set, err := valuesGiven(values, valuesFromEnv)
	if err != nil {
		return err
	}
	limit, err := ceiling(ceilingText, nil, nil, permission.Layer{}, permission.CheckList)
	if err != nil {
		return err
	}
	if e.config == "" {
		if e.config, err = filepath.Abs(client.ConfigName); err != nil {
			return err
		}
	}
	cfg := identity.Config{Dir: e.config}
	if !cfg.Has(identity.Controller) {
		// Nothing in the config is for a repository.
		if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(cfg.Dir, ".gitignore"), []byte("*\n"), 0o600); err != nil {
			return err
		}
		address := e.store
		if address == "" {
			address = filepath.Join(cfg.Dir, "store")
		}
		if err := ownStore(address); err != nil {
			return err
		}
		if store.IsDirectory(address) {
			if address, err = filepath.Abs(strings.TrimPrefix(address, "file://")); err != nil {
				return err
			}
		}
		if _, _, _, err := e.setUp(cfg, address, hostName(), "", limit); err != nil {
			return err
		}
	} else if e.Given("ceiling") {
		return Mistake("--ceiling sets up a new config, and %s is set up: corbie init --ceiling changes it", cfg.Dir)
	}
	// The secrets are typed after every mistake on the line is found.
	secrets, err := s.testSecrets(e)
	if err != nil {
		return err
	}
	if len(set)+len(secrets) > 0 {
		ctl, s, err := e.Controller()
		if err != nil {
			return err
		}
		for name, value := range set {
			if err := secret.SetValue(e.ctx, s, ctl, "/", name, value); err != nil {
				return err
			}
		}
		for name, value := range secrets {
			if err := secret.Set(e.ctx, s, ctl, "/", name, value); err != nil {
				return err
			}
		}
	}
	// The controller, the runner and the gateway run whether or not they
	// are named. The gateway listens at a free port by default.
	roles := []item{{name: "controller"}, {name: "runner"}, {name: "gateway", words: []string{"--listen", "127.0.0.1:0"}}}
	for i := len(roles) - 1; i >= 0; i-- {
		if !slices.ContainsFunc(named, func(it item) bool { return it.name == roles[i].name }) {
			named = slices.Insert(named, 0, roles[i])
		}
	}
	return e.start(cfg, named, func() error {
		if len(tree.Machines) == 0 {
			e.Say("%s holds no machine: nothing is submitted", dir)
			return nil
		}
		return e.submit(tree)
	})
}
