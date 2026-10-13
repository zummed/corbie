package store

import (
	"context"
	"io"
	"slices"
	"sync"
)

// Memory is a store held in memory, for `corbie test` and Corbie's own
// tests. Each party opens it with As and is held to its row of the table,
// as a bucket would hold it.
type Memory struct {
	mu     sync.Mutex
	values map[string][]byte
	notices

	// Note, when set, is told of everything done to the store. What a party
	// was refused was not done, and is not told.
	Note func(Op)
}

// An Op is one thing a party did to a memory store.
type Op struct {
	Party Party
	Act   Act
	Name  string
	How   How
	Value []byte // what was written, or read
	Done  bool   // false for "not there", "already there" or "changed"
}

// NewMemory makes an empty memory store.
func NewMemory() *Memory {
	return &Memory{values: map[string][]byte{}}
}

// As opens the store as a party, held to its row of the table.
func (m *Memory) As(p Party) Store { return held{memoryAs{m, p}, p} }

// memoryAs is the memory store as one party uses it: what is done is noted
// as that party's. It holds the party to nothing, which is As's to do.
type memoryAs struct {
	m *Memory
	p Party
}

func (m *Memory) note(op Op) {
	if m.Note != nil {
		m.Note(op)
	}
}

func (s memoryAs) Read(ctx context.Context, name string) ([]byte, Mark, error) {
	if err := ctx.Err(); err != nil {
		return nil, None, err
	}
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	value, ok := s.m.values[name]
	s.m.note(Op{Party: s.p, Act: ActRead, Name: name, Value: value, Done: ok})
	if !ok {
		return nil, None, nil
	}
	return slices.Clone(value), markOf(value), nil
}

func (s memoryAs) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	value, mark, err := s.Read(ctx, name)
	if err != nil || mark == None || from >= int64(len(value)) {
		return nil, err
	}
	return value[from:min(from+length, int64(len(value)))], nil
}

func (s memoryAs) Write(ctx context.Context, name string, value []byte, how How) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	act := actOf(how, false)
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	old, there := s.m.values[name]
	done := true
	switch how.kind {
	case onlyIfNew:
		done = !there
	case onlyIfUnchanged:
		done = there && markOf(old) == how.mark
	}
	if done {
		s.m.values[name] = slices.Clone(value)
		s.m.wrote(name)
	}
	s.m.note(Op{Party: s.p, Act: act, Name: name, How: how, Value: value, Done: done})
	return done, nil
}

func (s memoryAs) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	value, err := io.ReadAll(r)
	if err != nil {
		return false, err
	}
	return s.Write(ctx, name, value, OnlyIfNew)
}

func (s memoryAs) Copy(ctx context.Context, from, to string) (bool, error) {
	value, mark, err := s.Read(ctx, from)
	if err != nil {
		return false, err
	}
	if mark == None {
		return false, notThere(from)
	}
	return s.Write(ctx, to, value, OnlyIfNew)
}

func (s memoryAs) Delete(ctx context.Context, name string, how How) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	old, there := s.m.values[name]
	done := true
	if how.kind == onlyIfUnchanged {
		done = there && markOf(old) == how.mark
	}
	if done && there {
		delete(s.m.values, name)
		s.m.wrote(name)
	}
	s.m.note(Op{Party: s.p, Act: ActDelete, Name: name, How: how, Done: done})
	return done, nil
}

func (s memoryAs) List(ctx context.Context, under string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	var names []string
	for name := range s.m.values {
		if within(name, under) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	s.m.note(Op{Party: s.p, Act: ActList, Name: under, Done: true})
	return names, nil
}

func (s memoryAs) First(ctx context.Context, under string, n int) ([]string, error) {
	names, err := s.List(ctx, under)
	return names[:max(min(len(names), n, firstMost), 0)], err
}

func (s memoryAs) Notice(under string) (<-chan struct{}, func()) {
	return s.m.notice(under)
}
