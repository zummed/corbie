// Package client starts runs and reads what happened to them. The command
// line, the gateway and an application written in Go all use it, so there
// is one behaviour and one set of words for all of them.
package client

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/identity"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/secret"
	"github.com/zummed/corbie/store"
)

// A Client is one party's way into a controller's store. With no identity
// it is a viewer: it can list the heads the store lets it read and nothing
// more.
type Client struct {
	ID         *identity.Identity
	Controller identity.Card
	Store      store.Store
	// Now is the client's clock.
	Now func() time.Time

	sender store.Sender
}

// New makes a client of a controller's store.
func New(id *identity.Identity, controller identity.Card, s store.Store) *Client {
	return &Client{ID: id, Controller: controller, Store: s, Now: time.Now}
}

func (c *Client) ctl() string { return c.Controller.Name }

func (c *Client) identity() error {
	if c.ID == nil {
		return fmt.Errorf("this needs a client's identity, and there is none here (corbie init, or corbie client card, makes one)")
	}
	return nil
}

// A Start is what a start may carry besides its definition.
type Start struct {
	// Run is the run's name, if it was chosen beforehand with NewRun.
	Run string
	// Input is the run's input. A blob of another of this client's runs is
	// carried over by writing it as it was shown, with the run it was shown
	// for added: { "$blob": <its path>, "run": <that run's ID> }, with or
	// without the type and size it was shown with. The blob is copied into
	// the new run's record, and the copy stands in the input.
	Input any
	// Files are files given as input. Each is put into the run's record
	// and stands in the input as a blob, under the name it has here; the
	// input is then a map, or nothing.
	Files  map[string]InputFile
	Labels map[string]string
	// Allow narrows what the run, and everything below it, may do.
	Allow *permission.Layer
	// Timeout is how long the run may take.
	Timeout time.Duration
	// Secrets are the run's own secrets.
	Secrets map[string]string
	// Life is how long the start is good for; a few minutes if it says
	// nothing.
	Life time.Duration
}

// ErrStart is a start that cannot be sent as it was given.
var ErrStart = errors.New("the start cannot be sent")

// An InputFile is a file given as input: the name it has as a file, from
// which the blob's type follows, and its contents.
type InputFile struct {
	Name     string
	Contents io.Reader
}

// NewRun chooses the name of a run before it is started, so that an
// application can note it before anything is sent.
func (c *Client) NewRun() (string, error) {
	if err := c.identity(); err != nil {
		return "", err
	}
	return store.NewInstance(c.ID.Name, c.Now()), nil
}

// Start starts a run of a definition that has been filled out and checked.
// The definition and its files are written into the run's record first,
// and then the start is sent. It gives the run's name.
func (c *Client) Start(ctx context.Context, draft *machine.Draft, start Start) (string, error) {
	if err := c.identity(); err != nil {
		return "", err
	}
	run := start.Run
	if run == "" {
		run = store.NewInstance(c.ID.Name, c.Now())
	}
	if err := store.CheckInstance(run); err != nil || store.InstanceClient(run) != c.ID.Name {
		return "", fmt.Errorf("%w: %q is not a name this client can give a run", ErrStart, run)
	}
	for name := range start.Secrets {
		if err := secret.CheckName(name); err != nil {
			return "", fmt.Errorf("%w: %w", ErrStart, err)
		}
	}
	// Everything is looked over before anything is written.
	input, err := c.carried(ctx, start)
	if err != nil {
		return "", err
	}
	// A value carried over from another run is copied into this one's
	// record: every blob lives in the record of one instance.
	if input, err = blob.CopyAll(ctx, c.Store, input, c.ctl(), run); err != nil {
		return "", err
	}
	ref, err := draft.Store(ctx, c.Store, c.ctl(), run)
	if err != nil {
		return "", err
	}
	for _, name := range sortedNames(start.Files) {
		file := start.Files[name]
		given, err := blob.Write(ctx, c.Store, store.Blob(c.ctl(), run, blob.NewName()), blob.TypeOf(file.Name), blob.NewKey(), file.Contents)
		if err != nil {
			return "", err
		}
		input.(map[string]any)[name] = given.Value()
	}
	seed := make([]byte, 32)
	rand.Read(seed)
	msg := &instance.Message{Kind: instance.KindStart, Instance: run, Start: &instance.Start{
		Definition: ref, Input: input, Labels: start.Labels, Allow: start.Allow,
		Timeout: machine.Duration(start.Timeout), Secrets: start.Secrets, Seed: seed}}
	return run, c.send(ctx, startMessage(run), start.Life, msg)
}

// carried gives a start's input as it is sent, but for its files: plain
// data, in which each blob carried over from another run is the reference
// to it where it is now. Nothing is written. With files to come, the input
// is a map with no value under a file's name.
func (c *Client) carried(ctx context.Context, start Start) (any, error) {
	input, err := plain(start.Input)
	if err != nil {
		return nil, err
	}
	if len(start.Files) > 0 {
		if input == nil {
			input = map[string]any{}
		}
		named, ok := input.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: a file is given as input under a name, and the input is not a map of names", ErrStart)
		}
		for name := range start.Files {
			if _, taken := named[name]; taken {
				return nil, fmt.Errorf("%w: the input has a value under %q, and a file is given under that name too", ErrStart, name)
			}
		}
	}
	return c.resolve(ctx, input)
}

// resolve gives a value in which each blob that names the run it was shown
// for is the reference to that blob, found as Fetch finds it.
func (c *Client) resolve(ctx context.Context, v any) (any, error) {
	switch v := v.(type) {
	case map[string]any:
		if run, path, ok := shownFor(v); ok {
			ref, err := c.find(ctx, run, path)
			if err != nil {
				return nil, fmt.Errorf("the blob at %q of the run %q: %w", path, run, err)
			}
			return ref.Value(), nil
		}
		out := make(map[string]any, len(v))
		for name, inner := range v {
			resolved, err := c.resolve(ctx, inner)
			if err != nil {
				return nil, err
			}
			out[name] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, inner := range v {
			resolved, err := c.resolve(ctx, inner)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	}
	return v, nil
}

// shownFor says whether a value is a blob as it is shown with the run it
// was shown for added, and gives the run and the blob's path in it.
func shownFor(v map[string]any) (run, path string, ok bool) {
	path, isPath := v["$blob"].(string)
	run, isRun := v["run"].(string)
	if !isPath || !isRun {
		return "", "", false
	}
	for name, value := range v {
		switch name {
		case "$blob", "run":
		case "type":
			if _, text := value.(string); !text {
				return "", "", false
			}
		case "size":
			if _, number := value.(float64); !number {
				return "", "", false
			}
		default:
			return "", "", false
		}
	}
	return run, path, true
}

// startMessage is the name of the message that starts a run: it follows
// from the run's name, so the answer to a start can be found by one who
// knows only the run.
func startMessage(run string) string {
	_, id, _ := strings.Cut(run, "/")
	return id
}

// Cancel asks for a run to be cancelled. It gives the name of the message,
// by which its receipt is found. Like a start, a cancel, an event and an
// answer are each good for the life given, or for a few minutes if it is
// zero.
func (c *Client) Cancel(ctx context.Context, run string, life time.Duration) (string, error) {
	return c.sendNext(ctx, life, &instance.Message{Kind: instance.KindCancel, Instance: run})
}

// Send sends a named event to a run.
func (c *Client) Send(ctx context.Context, run, event string, data any, life time.Duration) (string, error) {
	return c.sendNext(ctx, life, &instance.Message{Kind: instance.KindEvent, Instance: run,
		Event: &machine.Event{Name: event, Data: data}})
}

// Reply answers a ticket of a run. The ticket is the authority: any client
// that holds it may answer.
func (c *Client) Reply(ctx context.Context, run, ticket string, value any, life time.Duration) (string, error) {
	return c.sendNext(ctx, life, &instance.Message{Kind: instance.KindAnswer, Instance: run,
		Answer: &instance.Answer{Ticket: ticket, Value: value}})
}

func (c *Client) sendNext(ctx context.Context, life time.Duration, msg *instance.Message) (string, error) {
	if err := c.identity(); err != nil {
		return "", err
	}
	if err := store.CheckInstance(msg.Instance); err != nil {
		return "", err
	}
	name := c.sender.Next(c.Now())
	return name, c.send(ctx, name, life, msg)
}

// good is how long a message is good for: the life it was given, or a few
// minutes if it was given none.
func good(life time.Duration) time.Duration {
	if life <= 0 {
		return instance.MessageLife
	}
	return life
}

// send writes one message into the controller's inbox, signed by this
// client and sealed to the controller.
func (c *Client) send(ctx context.Context, name string, life time.Duration, msg *instance.Message) error {
	now := c.Now()
	at := store.Message(c.ctl(), c.ID.Name, name)
	data, err := c.ID.Write(at, now, now.Add(good(life)), msg, c.Controller)
	if err != nil {
		return err
	}
	_, err = c.Store.Write(ctx, at, data, store.OnlyIfNew)
	return err
}

// Receipt reads the controller's answer to a message this client sent. It
// gives nil while there is none.
func (c *Client) Receipt(ctx context.Context, message string) (*instance.Receipt, error) {
	if err := c.identity(); err != nil {
		return nil, err
	}
	at := store.Receipt(c.ctl(), c.ID.Name, message)
	data, mark, err := c.Store.Read(ctx, at)
	if err != nil || mark == store.None {
		return nil, err
	}
	var receipt instance.Receipt
	if _, err := identity.Read(data, at, c.Controller, c.ID, &receipt); err != nil {
		return nil, fmt.Errorf("the receipt for %s: %w", message, err)
	}
	return &receipt, nil
}

// ErrNotTaken is a message that stopped being good before a controller
// answered it.
var ErrNotTaken = errors.New("the message was not taken up: it is no longer good and no receipt has come (is a controller running?)")

// WaitReceipt stays until the controller has answered a message this
// client has just sent, and gives the receipt. life is the life the message
// was sent with. Once the message is no longer good and no receipt has
// come, the wait ends, saying the message was not taken up. It looks again
// when this process writes to the store, and otherwise at the interval.
func (c *Client) WaitReceipt(ctx context.Context, message string, life, interval time.Duration) (*instance.Receipt, error) {
	notice, stop := c.Store.Notice(c.ctl())
	defer stop()
	until := c.Now().Add(good(life))
	for {
		receipt, err := c.Receipt(ctx, message)
		if err != nil || receipt != nil {
			return receipt, err
		}
		left := until.Sub(c.Now())
		if left <= 0 {
			return nil, ErrNotTaken
		}
		if !pause(ctx, min(interval, left), notice) {
			return nil, ctx.Err()
		}
	}
}
