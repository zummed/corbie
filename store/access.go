package store

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Kind is what a party is to the store.
type Kind int

const (
	// Nobody may read the layout's version and nothing else.
	Nobody Kind = iota
	Controller
	Runner
	Client
	// Viewer is a party given heads only.
	Viewer
	// Owner is whoever runs the store: no rule holds them.
	Owner
)

// A Party is one row of the table of who may touch what. Controller is the
// controller whose area a runner, a client or a viewer works in.
type Party struct {
	Kind       Kind
	Name       string
	Controller string
}

// Act is one thing a party does to a name.
type Act int

const (
	ActRead Act = 1 << iota
	ActList
	ActAdd     // write only-if-new
	ActRewrite // overwrite or write only-if-unchanged
	ActDelete

	actsAll = ActRead | ActList | ActAdd | ActRewrite | ActDelete
)

func (a Act) String() string {
	switch a {
	case ActRead:
		return "read"
	case ActList:
		return "list"
	case ActAdd:
		return "add"
	case ActRewrite:
		return "rewrite"
	case ActDelete:
		return "delete"
	}
	return "act"
}

// A grant allows some acts on the names a pattern matches. In a pattern "*"
// is any one part and a final "**" is any parts at all.
type grant struct {
	acts    Act
	pattern string
}

// Grants is the party's row of the table, as patterns. It is the one source
// for what the memory store enforces and what a bucket's policy says.
func (p Party) grants() []grant {
	c, n := p.Controller, p.Name
	version := grant{ActRead, VersionName}
	switch p.Kind {
	case Owner:
		return []grant{{actsAll, "**"}}
	case Controller:
		return []grant{
			{ActRead | ActAdd, VersionName},
			{actsAll, n + "/**"},
			{ActAdd, "*/inbox/" + n + "/*"},
		}
	case Runner:
		return []grant{
			version,
			{ActAdd, c + "/inbox/" + n + "/*"},
			{ActAdd, c + "/records/*/*/emits/**"},
			{ActAdd | ActRead, c + "/records/*/*/blobs/*"},
			{actsAll, n + "/inbox/**"},
			{actsAll, n + "/taken/**"},
		}
	case Client:
		return []grant{
			version,
			{ActAdd, c + "/inbox/" + n + "/*"},
			{ActAdd, c + "/records/" + n + "/*/blobs/*"},
			{ActRead | ActList, c + "/receipts/" + n + "/**"},
			{ActRead | ActList, c + "/running/" + n + "/**"},
			{ActRead | ActList, c + "/finished/" + n + "/**"},
			{ActRead | ActList, c + "/records/" + n + "/**"},
		}
	case Viewer:
		return []grant{
			version,
			{ActRead | ActList, c + "/running/**"},
			{ActRead | ActList, c + "/finished/**"},
		}
	}
	return []grant{version}
}

// Allows says whether the party may do act to name. For ActList, name is
// the path listed.
func (p Party) Allows(act Act, name string) bool {
	parts := strings.Split(name, "/")
	for _, g := range p.grants() {
		if g.acts&act == 0 {
			continue
		}
		pattern := strings.Split(g.pattern, "/")
		if act == ActList {
			if coversPath(pattern, parts) {
				return true
			}
		} else if matches(pattern, parts) {
			return true
		}
	}
	return false
}

// matches says whether the parts of a name fit a pattern.
func matches(pattern, parts []string) bool {
	for i, p := range pattern {
		if p == "**" {
			return true
		}
		if i >= len(parts) || (p != "*" && p != parts[i]) {
			return false
		}
	}
	return len(parts) == len(pattern)
}

// coversPath says whether every name under a path fits a pattern.
func coversPath(pattern, path []string) bool {
	for i, p := range pattern {
		if p == "**" {
			return true
		}
		if i >= len(path) || (p != "*" && p != path[i]) {
			return false
		}
	}
	return false
}

// check gives ErrRefused unless the party may do act to name.
func (p Party) check(act Act, name string) error {
	if p.Allows(act, name) {
		return nil
	}
	return fmt.Errorf("%w to %s %s", ErrRefused, act, name)
}

// actOf is the act a write or delete in the way how is.
func actOf(how How, deleting bool) Act {
	switch {
	case deleting:
		return ActDelete
	case how.kind == onlyIfNew:
		return ActAdd
	}
	return ActRewrite
}

// held is a store opened as a party: every call is held to that party's row
// of the table before the store sees it. It is the one place the table is
// enforced by Corbie itself: for a memory store, and for a directory served to
// other computers.
type held struct {
	s Store
	p Party
}

func (h held) Read(ctx context.Context, name string) ([]byte, Mark, error) {
	if err := checkName(name); err != nil {
		return nil, None, err
	}
	// A name the party may not read holds nothing, as far as it can tell.
	if !h.p.Allows(ActRead, name) {
		return nil, None, nil
	}
	return h.s.Read(ctx, name)
}

func (h held) ReadPart(ctx context.Context, name string, from, length int64) ([]byte, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	if !h.p.Allows(ActRead, name) {
		return nil, nil
	}
	return h.s.ReadPart(ctx, name, from, length)
}

func (h held) Write(ctx context.Context, name string, value []byte, how How) (bool, error) {
	if err := h.may(actOf(how, false), name); err != nil {
		return false, err
	}
	return h.s.Write(ctx, name, value, how)
}

func (h held) WriteFrom(ctx context.Context, name string, r io.Reader) (bool, error) {
	if err := h.may(ActAdd, name); err != nil {
		return false, err
	}
	return h.s.WriteFrom(ctx, name, r)
}

func (h held) Copy(ctx context.Context, from, to string) (bool, error) {
	if err := checkName(from); err != nil {
		return false, err
	}
	if err := h.may(ActAdd, to); err != nil {
		return false, err
	}
	if !h.p.Allows(ActRead, from) {
		return false, notThere(from)
	}
	return h.s.Copy(ctx, from, to)
}

func (h held) Delete(ctx context.Context, name string, how How) (bool, error) {
	if err := h.may(ActDelete, name); err != nil {
		return false, err
	}
	return h.s.Delete(ctx, name, how)
}

func (h held) List(ctx context.Context, under string) ([]Listed, error) {
	if err := checkUnder(under); err != nil {
		return nil, err
	}
	if err := h.p.check(ActList, under); err != nil {
		return nil, err
	}
	return h.s.List(ctx, under)
}

func (h held) First(ctx context.Context, under string, n int) ([]Listed, error) {
	if err := checkUnder(under); err != nil {
		return nil, err
	}
	if err := h.p.check(ActList, under); err != nil {
		return nil, err
	}
	return h.s.First(ctx, under, n)
}

// may says whether the party may do act to name, a name no store takes
// being said to be that first.
func (h held) may(act Act, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	return h.p.check(act, name)
}

// Notice says that something was written under a path, and not what: a
// party learns no name by it.
func (h held) Notice(under string) (<-chan struct{}, func()) { return h.s.Notice(under) }
