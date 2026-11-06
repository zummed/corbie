package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// This file is the whole layout: every name Corbie writes is made here.
//
// An instance's name is "<client>/<id>": the client that owns its root's
// tree, then an id that begins with the time it was named. So anything that names
// an instance says where its head and its record are and how old it is.

const (
	// stamp is the one form a time has in a name: fixed width, UTC, so that
	// names sort by time.
	stamp      = "20060102T150405.000Z"
	stampMicro = "20060102T150405.000000Z"
)

// Stamp writes a time as it appears in names.
func Stamp(t time.Time) string { return t.UTC().Format(stamp) }

func Inbox(party string) string                    { return party + "/inbox" }
func Message(party, sender, message string) string { return party + "/inbox/" + sender + "/" + message }
func Receipts(ctl, sender string) string           { return ctl + "/receipts/" + sender }
func Running(ctl string) string                    { return ctl + "/running" }
func Finished(ctl string) string                   { return ctl + "/finished" }
func RunningHead(ctl, instance string) string      { return ctl + "/running/" + instance }
func FinishedHead(ctl, instance string) string     { return ctl + "/finished/" + instance }
func Records(ctl string) string                    { return ctl + "/records" }
func Record(ctl, instance string) string           { return ctl + "/records/" + instance }
func Steps(ctl, instance string) string            { return ctl + "/records/" + instance + "/steps" }
func Emits(ctl, instance string) string            { return ctl + "/records/" + instance + "/emits" }
func Carried(ctl, instance string) string          { return ctl + "/records/" + instance + "/carried" }
func Tried(ctl, instance string) string            { return ctl + "/records/" + instance + "/tried" }
func Blobs(ctl, instance string) string            { return ctl + "/records/" + instance + "/blobs" }
func Blob(ctl, instance, name string) string       { return ctl + "/records/" + instance + "/blobs/" + name }
func Wakes(ctl string) string                      { return ctl + "/wakes" }
func TrustList(ctl string) string                  { return ctl + "/trust" }
func Trust(ctl, party string) string               { return ctl + "/trust/" + party }
func TakenList(runner string) string               { return runner + "/taken" }
func Uploads(ctl, client string) string            { return ctl + "/uploads/" + client }

// Uploaded is where the files a client sends with one message are.
func Uploaded(ctl, client, message string) string {
	return ctl + "/uploads/" + client + "/" + message
}

// Upload is the name of a file a client sends with a message.
func Upload(ctl, client, message, name string) string {
	return Uploaded(ctl, client, message) + "/" + name
}

// Receipt is the name of the controller's answer to a client's message: in
// a directory of the message's own name.
func Receipt(ctl, sender, message string) string {
	return ctl + "/receipts/" + sender + "/" + message + "/receipt"
}

// Result is the name of what became of the run a client called in a tree
// it does not own, beside the call's receipt.
func Result(ctl, sender, message string) string {
	return ctl + "/receipts/" + sender + "/" + message + "/result"
}

// ReceiptBlobs is where the blobs a call's result names are.
func ReceiptBlobs(ctl, sender, message string) string {
	return ctl + "/receipts/" + sender + "/" + message + "/blobs"
}

// ReceiptBlob is the name of a blob a call's result names.
func ReceiptBlob(ctl, sender, message, name string) string {
	return ReceiptBlobs(ctl, sender, message) + "/" + name
}

// ReceiptMessage reads the message out of the name of a receipt or of one
// of its blobs.
func ReceiptMessage(ctl, name string) (message string, ok bool) {
	rest, ok := strings.CutPrefix(name, ctl+"/receipts/")
	parts := strings.Split(rest, "/")
	if !ok || len(parts) < 3 {
		return "", false
	}
	return parts[1], true
}

// Locations is where everything set at a location is kept.
func Locations(ctl string) string { return ctl + "/locations" }

// LocationDir is where the entries of one location are: under "+" after
// its path, and "+" is never a part of a location, so no location's entries
// are a path above another's. The entries of "/" are under locations/+.
func LocationDir(ctl, location string) string {
	if location == "/" {
		return ctl + "/locations/+"
	}
	parts := strings.Split(strings.TrimPrefix(location, "/"), "/")
	for i, part := range parts {
		parts[i] = stored(part)
	}
	return ctl + "/locations/" + strings.Join(parts, "/") + "/+"
}

// stored is a part of a location as it is kept in the store: one that
// starts with a dot, as .rook does, after a "#", so that a directory does
// not hide it. No part of a location holds "#", so no other part is kept
// starting with one.
func stored(part string) string {
	if strings.HasPrefix(part, ".") {
		return "#" + part
	}
	return part
}

// Tree is the name of the entry of a tree submitted at a prefix.
func Tree(ctl, prefix string) string            { return LocationDir(ctl, prefix) + "/tree" }
func LocationFile(ctl, location string) string  { return LocationDir(ctl, location) + "/file" }
func LocationBlobs(ctl, location string) string { return LocationDir(ctl, location) + "/blobs" }
func LocationBlob(ctl, location, name string) string {
	return LocationBlobs(ctl, location) + "/" + name
}
func Keys(ctl, location string) string   { return LocationDir(ctl, location) + "/keys" }
func Secrets(ctl, at string) string      { return LocationDir(ctl, at) + "/secrets" }
func Secret(ctl, at, name string) string { return Secrets(ctl, at) + "/" + name }
func Values(ctl, at string) string       { return LocationDir(ctl, at) + "/values" }
func Value(ctl, at, name string) string  { return Values(ctl, at) + "/" + name }

// Key is the name of a key's file among a location's. A key is any text, so
// the name holds its hash and never the key itself. A singleton's is the
// empty key.
func Key(ctl, location, key string) string {
	return Keys(ctl, location) + "/" + keyHash(key)
}

func keyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// ParseLocation reads the location out of the name of one of its entries,
// and what follows the location's "+".
func ParseLocation(ctl, name string) (location, entry string, ok bool) {
	rest, ok := strings.CutPrefix(name, Locations(ctl)+"/")
	if !ok {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	at := slices.Index(parts, "+")
	if at < 0 || at == len(parts)-1 {
		return "", "", false
	}
	for i, part := range parts[:at] {
		parts[i] = strings.TrimPrefix(part, "#")
	}
	return "/" + strings.Join(parts[:at], "/"), strings.Join(parts[at+1:], "/"), true
}

// CheckLocation says whether a text can be a location: "/", or "/" then
// parts, none of them empty, ".", ".." or "+", and none holding "#", which
// separates a key from its location, "*", which makes a pattern, a slash, a
// backslash or a control character.
func CheckLocation(location string) error {
	if location == "/" {
		return nil
	}
	rest, ok := strings.CutPrefix(location, "/")
	if !ok {
		return fmt.Errorf("%w: the location %q does not start with /", ErrName, location)
	}
	for part := range strings.SplitSeq(rest, "/") {
		if part == "." || part == ".." || part == "+" || strings.ContainsAny(part, "#*") || CheckPart(stored(part)) != nil {
			return fmt.Errorf("%w: the location %q has the part %q: a part is not empty, ., .. or +, and holds no #, *, backslash or control character", ErrName, location, part)
		}
	}
	return nil
}

// Above gives a location and each path above it, nearest first, ending
// with "/".
func Above(location string) []string {
	above := []string{location}
	for len(location) > 1 {
		location = location[:max(strings.LastIndexByte(location, '/'), 1)]
		above = append(above, location)
	}
	return above
}

// Taken is the name of the mark a runner leaves for a job it has taken. The
// name carries the job's due time, so the marks whose time has passed are
// found by listing alone.
func Taken(runner string, due time.Time, job string) string {
	return runner + "/taken/" + Stamp(due) + "/" + job
}

// TakenOf gives the job a mark is for, and the job's due time.
func TakenOf(name string) (job string, due time.Time, ok bool) {
	parts := strings.Split(name, "/")
	if len(parts) < 4 || parts[len(parts)-3] != "taken" {
		return "", time.Time{}, false
	}
	due, err := time.Parse(stamp, parts[len(parts)-2])
	return parts[len(parts)-1], due, err == nil
}

// Step is the name of a step entry. Numbers are written so that names sort
// in number order.
func Step(ctl, instance string, number int) string {
	return fmt.Sprintf("%s/records/%s/steps/%08d", ctl, instance, number)
}

// Emit is the name of an emit entry of a job.
func Emit(ctl, instance, job string, number int) string {
	return fmt.Sprintf("%s/records/%s/emits/%s/%08d", ctl, instance, job, number)
}

// Number reads the number at the end of a step's or an emit's name.
func Number(name string) int {
	var n int
	fmt.Sscanf(name[strings.LastIndexByte(name, '/')+1:], "%d", &n)
	return n
}

// Wake is the name of a wake: wakes list soonest first.
func Wake(ctl string, at time.Time, instance, what string) string {
	return ctl + "/wakes/" + Stamp(at) + "/" + instance + "/" + what
}

// House is the name the controller's own wake is under, where an
// instance's name would be: the wake for its hourly housekeeping.
func House(ctl string) string { return ctl + "/house" }

// ParseWake reads a wake's name.
func ParseWake(ctl, name string) (at time.Time, instance, what string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(name, ctl+"/wakes/"), "/")
	if len(parts) != 4 {
		return at, "", "", false
	}
	at, err := time.Parse(stamp, parts[0])
	return at, parts[1] + "/" + parts[2], parts[3], err == nil
}

// NewInstance makes the name of a new root instance for a client.
func NewInstance(client string, now time.Time) string {
	random := make([]byte, 5)
	rand.Read(random)
	return client + "/" + Stamp(now) + "-" + hex.EncodeToString(random)
}

// ChildInstance makes the name of an instance below another. It follows
// from what asked for it, so that asking twice names one child.
func ChildInstance(parent string, at time.Time, job string) string {
	sum := sha256.Sum256([]byte(parent + "\x00" + job))
	return InstanceClient(parent) + "/" + Stamp(at) + "-" + hex.EncodeToString(sum[:5])
}

// RootInstance makes the name of a root instance that the controller starts
// for a client, as a submit starts a singleton. It follows from what asked
// for it, so that asking twice names one instance.
func RootInstance(client string, at time.Time, from string) string {
	sum := sha256.Sum256([]byte(client + "\x00" + from))
	return client + "/" + Stamp(at) + "-" + hex.EncodeToString(sum[:5])
}

// CheckInstance says whether name has the form of an instance's name.
func CheckInstance(name string) error {
	client, id, ok := strings.Cut(name, "/")
	if !ok || CheckPart(client) != nil || CheckPart(id) != nil {
		return fmt.Errorf("%q is not the name of a run", name)
	}
	if _, err := InstanceTime(name); err != nil {
		return fmt.Errorf("%q is not the name of a run", name)
	}
	return nil
}

// InstanceClient is the client that owns the tree of the instance's root.
func InstanceClient(instance string) string {
	client, _, _ := strings.Cut(instance, "/")
	return client
}

// InstanceTime is the time the instance was named.
func InstanceTime(instance string) (time.Time, error) {
	_, id, _ := strings.Cut(instance, "/")
	if len(id) < len(stamp) {
		return time.Time{}, fmt.Errorf("%q carries no time", instance)
	}
	return time.Parse(stamp, id[:len(stamp)])
}

// InstanceOf reads the instance out of a head's or a record entry's name.
func InstanceOf(ctl, name string) string {
	for _, under := range []string{Running(ctl), Finished(ctl), Records(ctl)} {
		if rest, ok := strings.CutPrefix(name, under+"/"); ok {
			parts := strings.SplitN(rest, "/", 3)
			if len(parts) >= 2 {
				return parts[0] + "/" + parts[1]
			}
		}
	}
	return ""
}

// flat writes an instance's name as one part of a name.
func flat(instance string) string { return strings.Replace(instance, "/", ".", 1) }

// JobMessage is the name a job has in its runner's inbox and among its
// marks: fixed by its instance and its name.
func JobMessage(instance, job string) string { return flat(instance) + "." + job }

// LinkMessage is the name of a message the controller sends itself for one
// instance on behalf of another: fixed by the step that asked for it, and
// sorting in the order that instance's steps asked.
func LinkMessage(from string, step, index int, what string) string {
	return fmt.Sprintf("%s.%08d.%04d.%s", flat(from), step, index, what)
}

// A Sender names one sender's messages so that they sort in the order they
// were sent, also when two are sent in the same instant or the clock steps
// back.
type Sender struct {
	mu   sync.Mutex
	last time.Time
}

// Next gives the name of the next message.
func (s *Sender) Next(now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC().Truncate(time.Microsecond)
	if !now.After(s.last) {
		now = s.last.Add(time.Microsecond)
	}
	s.last = now
	random := make([]byte, 3)
	rand.Read(random)
	return now.Format(stampMicro) + "-" + hex.EncodeToString(random)
}
