package role

import (
	"encoding/binary"
	"encoding/json/v2"
	"fmt"
	"io"
	"sync"
)

// A process and its starter talk over a channel of their own. Each piece is
// a length and then that many bytes of JSON; a blob's bytes follow the piece
// that announces them.

// MaxPiece bounds one piece; a blob's bytes are not counted.
const MaxPiece = 64 << 20

// A Wire is one end of the channel.
type Wire struct {
	R  io.Reader
	W  io.Writer
	mu sync.Mutex // one piece is written at a time
}

// Send writes one piece, and the size bytes of contents after it.
func (w *Wire) Send(piece any, size int64, contents io.Reader) error {
	data, err := json.Marshal(piece, json.Deterministic(true))
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	if _, err := w.W.Write(append(length[:], data...)); err != nil {
		return err
	}
	if contents != nil {
		_, err = io.CopyN(w.W, contents, size)
	}
	return err
}

// ReceiveUpTo reads one piece of at most a number of bytes, as it was sent.
func (w *Wire) ReceiveUpTo(limit uint32) ([]byte, error) {
	var length [4]byte
	if _, err := io.ReadFull(w.R, length[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(length[:])
	if n > limit {
		return nil, fmt.Errorf("a piece of %d bytes is not one the channel carries", n)
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(w.R, data); err != nil {
		return nil, err
	}
	return data, nil
}

// Receive reads one piece.
func (w *Wire) Receive(piece any) error {
	data, err := w.ReceiveUpTo(MaxPiece)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, piece); err != nil {
		// Its bytes all came: this is not the channel ending, whatever the
		// reading of them says.
		return fmt.Errorf("a piece that cannot be read: %v", err)
	}
	return nil
}
