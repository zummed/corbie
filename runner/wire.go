package runner

import (
	"io"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/role"
)

// A job's process and its runner talk over a channel of their own, which
// nothing a script, a plugin or a library prints can reach.

// piece is one thing said on the channel.
type piece struct {
	Op string `json:"op"`

	Begin  *begin           `json:"begin,omitzero"`
	Result *instance.Result `json:"result,omitzero"`

	Name   string    `json:"name,omitzero"` // of an emit, a variable, or a form's secrets
	Data   any       `json:"data,omitzero"`
	Ref    *blob.Ref `json:"ref,omitzero"`
	From   int64     `json:"from,omitzero"`
	Length int64     `json:"length,omitzero"`
	Type   string    `json:"type,omitzero"`
	Size   int64     `json:"size,omitzero"`

	// In an answer, and the text of a form.
	Value string         `json:"value,omitzero"`
	Set   bool           `json:"set,omitzero"`
	Error string         `json:"error,omitzero"`
	Code  string         `json:"code,omitzero"`
	Why   *machine.Error `json:"why,omitzero"`
}

// The things said on the channel.
const (
	opBegin  = "begin"  // runner: here is the job
	opStop   = "stop"   // runner: stop the script, and why
	opAnswer = "answer" // runner: the answer to what was asked
	opEmit   = "emit"   // job: a value sent up
	opEnv    = "env"    // job: one of the runner's variables
	opRead   = "read"   // job: part of a blob in the store
	opBlob   = "blob"   // job: a blob to put in the store; its bytes follow
	opForm   = "form"   // job: one more form a secret's value is about to leave in
	opResult = "result" // job: how the script ended
)

// begin is everything a job's process is given.
type begin struct {
	Job    *instance.Job     `json:"job"`
	Script string            `json:"script"`
	Files  map[string]string `json:"files,omitzero"`
	// Layers are every layer that applies, the runner's own ceiling last.
	Layers     []permission.Layer `json:"layers"`
	Scratch    string             `json:"scratch"`
	Wrapper    []string           `json:"wrapper,omitzero"`
	SearchPath string             `json:"search_path,omitzero"`
}

// maxPiece bounds one piece; a blob's bytes are not counted.
const maxPiece = role.MaxPiece

// wire is one end of the channel.
type wire struct{ *role.Wire }

func (w wire) send(p *piece, contents io.Reader) error { return w.Send(p, p.Size, contents) }

func (w wire) receive() (*piece, error) {
	var p piece
	if err := w.Receive(&p); err != nil {
		return nil, err
	}
	return &p, nil
}
