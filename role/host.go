package role

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zummed/corbie/store"
)

// On a serverless host the host calls for a pass when something is written
// to the role's inbox, and calls the controller once a minute so that wakes
// are noticed. Both are services the host and the bucket already offer;
// Corbie has no signalling of its own. Each call is one pass of each role
// the process was started with, finished a margin before the host's limit.

// HostMargin is how long before the host's limit a pass stops taking up
// new work, to finish what it has in hand.
const HostMargin = 10 * time.Second

// lambdaAPI is where AWS Lambda tells a process of its calls.
const lambdaAPI = "AWS_LAMBDA_RUNTIME_API"

// Serverless says whether the process is on a host that calls for passes
// itself.
func Serverless() bool { return os.Getenv(lambdaAPI) != "" }

// Serve answers the host's calls until the context ends: for each call,
// one pass of each role, and then the answer. Nothing of a pass goes on
// after it has answered.
func Serve(ctx context.Context, report func(role string, err error), roles ...Role) error {
	return serveLambda(ctx, "http://"+os.Getenv(lambdaAPI), report, roles)
}

func serveLambda(ctx context.Context, api string, report func(string, error), roles []Role) error {
	client := &http.Client{} // a call can be long in coming: no time limit
	for ctx.Err() == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"/2018-06-01/runtime/invocation/next", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("the host could not be asked for its next call: %w", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		id := resp.Header.Get("Lambda-Runtime-Aws-Request-Id")
		if resp.StatusCode != http.StatusOK || id == "" {
			return fmt.Errorf("the host answered %d to being asked for its next call", resp.StatusCode)
		}
		// A pass knows how long the host will let it run: the host says
		// when it kills the copy, and a call that does not say is not taken.
		ms, err := strconv.ParseInt(resp.Header.Get("Lambda-Runtime-Deadline-Ms"), 10, 64)
		if err != nil {
			return fmt.Errorf("the host did not say until when its call may run")
		}
		until := time.UnixMilli(ms)
		answer := map[string]any{}
		for _, r := range roles {
			// Each role in turn has what is left of the one call's time.
			next, err := Once(ctx, until, HostMargin, r.Pass)
			if err != nil {
				// What was waiting is still waiting for the next call.
				if report != nil {
					report(r.Name, err)
				}
				answer[r.Name] = "the pass ended early: " + err.Error()
				continue
			}
			answer[r.Name] = "done"
			if !next.IsZero() {
				answer[r.Name+"_next"] = next.UTC().Format(time.RFC3339)
			}
		}
		body, _ := json.Marshal(answer, json.Deterministic(true))
		// A call that was taken is answered, also when the process is being
		// stopped.
		post, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodPost, api+"/2018-06-01/runtime/invocation/"+id+"/response", bytes.NewReader(body))
		if err != nil {
			return err
		}
		if resp, err = client.Do(post); err != nil {
			return fmt.Errorf("the host could not be answered: %w", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return nil
}

// StoreFits refuses, on a serverless host, a store that is a directory or
// memory. Copies on such a host share no disk and no memory, so each would
// see a store of its own.
func StoreFits(address string) error {
	switch {
	case !Serverless():
		return nil
	case address == "memory:":
		return fmt.Errorf("a store in memory cannot be used on a serverless host: its copies share no memory, and each would have a store of its own; the store must be a bucket")
	case store.IsServed(address):
		return fmt.Errorf("a directory another computer serves cannot be used on a serverless host: nothing calls a function when it is written to; the store must be a bucket")
	case store.IsDirectory(address):
		return fmt.Errorf("a store that is a directory cannot be used on a serverless host: its copies share no disk, and each would have a store of its own; the store must be a bucket")
	}
	return nil
}

// OwnConfig gives the config directory a role is to use. A serverless host
// unpacks its package read-only and readable by everyone, and an identity
// that others can read is not accepted: there the config is copied to a
// place of the copy's own, made its owner's alone, and that place is given.
// Anywhere else the config is used where it is.
func OwnConfig(dir string) (string, error) {
	if !Serverless() {
		return dir, nil
	}
	own, err := os.MkdirTemp("", "corbie-config-")
	if err != nil {
		return "", err
	}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(own, rel), 0o700)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(own, rel), contents, 0o600)
	})
	return own, err
}
