package runner

import (
	"encoding/binary"
	"encoding/json/v2"
	"fmt"
	"io"
	"sync"

	"github.com/zummed/corbie/blob"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/permission"
)

// A job's process and its runner talk over a channel of their own, which
// nothing a script, a plugin or a library prints can reach. Each piece is a
// length and then that many bytes of JSON; a blob's bytes follow the piece
// that announces them.

// piece is one thing said on the channel.
type piece struct {
	Op string `json:"op"`

	Begin  *begin           `json:"begin,omitzero"`
	Result *instance.Result `json:"result,omitzero"`

	Name   string            `json:"name,omitzero"` // of an emit, a variable, or a form's secrets
	Data   any               `json:"data,omitzero"`
	Ref    *blob.Ref         `json:"ref,omitzero"`
	From   int64             `json:"from,omitzero"`
	Length int64             `json:"length,omitzero"`
	Type   string            `json:"type,omitzero"`
	Size   int64             `json:"size,omitzero"`
	Source string            `json:"source,omitzero"`
	Files  map[string]string `json:"files,omitzero"`
	Path   string            `json:"path,omitzero"` // a kept path

	// In an answer, and the text of a form.
	Value    string           `json:"value,omitzero"`
	Set      bool             `json:"set,omitzero"`
	Error    string           `json:"error,omitzero"`
	Code     string           `json:"code,omitzero"`
	Why      *machine.Error   `json:"why,omitzero"`
	Problems machine.Problems `json:"problems,omitzero"`
	Paths    []string         `json:"paths,omitzero"`
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
	opDefine = "define" // job: a machine to fill out, check and store
	opForm   = "form"   // job: one more form a secret's value is about to leave in
	opResult = "result" // job: how the script ended

	opKeptRead   = "kept.read"   // job: the value kept at a path
	opKeptWrite  = "kept.write"  // job: a value to keep at a path; its bytes follow
	opKeptList   = "kept.list"   // job: the kept paths below a path
	opKeptRemove = "kept.remove" // job: the value kept at a path, to remove
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
const maxPiece = 64 << 20

// wire is one end of the channel.
type wire struct {
	r  io.Reader
	w  io.Writer
	mu sync.Mutex // one piece is written at a time
}

func (w *wire) send(p *piece, contents io.Reader) error {
	data, err := json.Marshal(p, json.Deterministic(true))
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(data)))
	if _, err := w.w.Write(append(size[:], data...)); err != nil {
		return err
	}
	if contents != nil {
		_, err = io.CopyN(w.w, contents, p.Size)
	}
	return err
}

func (w *wire) receive() (*piece, error) {
	var size [4]byte
	if _, err := io.ReadFull(w.r, size[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(size[:])
	if n > maxPiece {
		return nil, fmt.Errorf("a piece of %d bytes is not one the channel carries", n)
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(w.r, data); err != nil {
		return nil, err
	}
	var p piece
	if err := json.Unmarshal(data, &p); err != nil {
		// Its bytes all came: this is not the channel ending, whatever the
		// reading of them says.
		return nil, fmt.Errorf("a piece that cannot be read: %v", err)
	}
	return &p, nil
}
