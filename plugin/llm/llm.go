// Package llm is the plugin that calls a model. Its chat gives the
// conversation so far, the tools on offer and what is wanted, and gets back
// what the model said, the tools it asked for, why it stopped and how many
// tokens were used. Its choose gives a System One model text or data and
// named questions about it, and gets back the answers.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"io"
	"net"
	nethttp "net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/zummed/corbie/plugin"
)

// A Service is one way of reaching a model: an extension. One on the web
// speaks one service's way of talking; one that is a program starts it.
type Service struct {
	Name string
	// Address is where a service on the web usually lives. Program is,
	// for one that is a program, the program that is started.
	Address string
	Program string
	// Models are how the names of the models it is the usual service of
	// begin. Known are models it is known to offer, by the names it knows
	// them by: for a person choosing one. It takes any name, and what is
	// not there is not listed.
	Models []string
	Known  []string
	// Tools are, for a program, the tools of its own it is known to have:
	// for a person choosing which it may use. Any may be named.
	Tools []string
	// Efforts are the levels of effort it takes, and Accepts the kinds of
	// content it takes beside text.
	Efforts []string
	Accepts []Accept
	// Needs says, for a program, what a tool of the program's own could
	// use: a group of permissions. Corbie does not run such a tool, so it
	// is allowed only where that whole group is.
	Needs func(tool string) string
	// Shapes says the service has a way of its own to be given the shape
	// of answer wanted. One that has none is told it in the system text.
	Shapes bool
	// Chat holds a conversation: one call. Choose puts questions with fixed
	// answers to a System One model. A service has one or both.
	Chat   func(job *plugin.Job, c *Call) (*Reply, error)
	Choose func(job *plugin.Job, q *Questions) (*Answers, error)
}

// Accept is one type of content a service takes beside text, and the
// largest of it that it takes.
type Accept struct {
	Type string
	Most int64
}

// Call is one call to a model.
type Call struct {
	Model  string
	System string
	Said   []any // the conversation so far
	Tools  []any
	Effort string
	Room   int // the most a reply may hold
	// Address and Key are where a service on the web is and what opens it.
	Address string
	Key     any
	Limit   time.Duration
	// Own are the tools of its own that a program may use.
	Own []string
	// Shape is the shape of answer wanted, for a service that takes it
	// itself.
	Shape map[string]any
}

// Reply is what a model gave back.
type Reply struct {
	Text  string
	Calls []any // each { id, name, args }
	Stop  string
	Usage map[string]any
	Kept  any // parts the service wants back unchanged
}

// Questions is one call to a System One model: the text or data the
// questions are about, and the questions under their names.
type Questions struct {
	Model     string
	State     any
	Questions map[string]any
	Address   string
	Key       any
	Limit     time.Duration
}

// Answers is what a System One model gave back: an answer under each
// question's name, as the service wrote it.
type Answers struct {
	Model   string
	Answers map[string]any
	Usage   map[string]any
}

// Why a model stopped.
const (
	StopEnd    = "end"    // it finished
	StopTools  = "tools"  // it asked for tools
	StopLength = "length" // it ran out of room
)

// defaultRoom is the most a reply may hold where the definition gives no
// figure and the service must be told one.
const defaultRoom = 8192

var services = map[string]*Service{}

// Register adds a model service.
func Register(s *Service) { services[s.Name] = s }

// Find gives a service by its name.
func Find(name string) (*Service, bool) {
	s, ok := services[name]
	return s, ok
}

// Names are the services this Corbie holds.
func Names() []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Having gives the services that make a call: those with Chat, or those
// with Choose.
func Having(call func(*Service) bool) []string {
	var names []string
	for _, name := range Names() {
		if call(services[name]) {
			names = append(names, name)
		}
	}
	return names
}

// Of gives the service a model is usually of, from its name; none if no
// service claims it.
func Of(model string) string {
	for _, name := range Names() {
		for _, start := range services[name].Models {
			if strings.HasPrefix(model, start) {
				return name
			}
		}
	}
	return ""
}

// Takes says whether a service takes content of a type and size, and, if
// it does not, why.
func (s *Service) Takes(typ string, size int64) error {
	for _, a := range s.Accepts {
		if a.Type != typ {
			continue
		}
		if size > a.Most {
			return plugin.Errorf(plugin.BadInput, "content of type %q may be %d bytes for the service %q, and this is %d", typ, a.Most, s.Name, size)
		}
		return nil
	}
	return plugin.Errorf(plugin.BadInput, "the service %q does not take content of type %q", s.Name, typ)
}

func init() {
	plugin.Register(&plugin.Module{Name: "llm", Page: page, Calls: map[string]*plugin.Call{
		"chat":   {Options: []string{"service", "model", "address", "key", "system", "tools", "shape", "effort", "reply_tokens", "timeout", "own"}, Do: chat},
		"choose": {Options: []string{"service", "model", "address", "key", "questions", "timeout"}, Do: choose},
		"takes": {Options: []string{"service"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			b, ok := first.(*plugin.Blob)
			name, err := opt.Text("service", "anthropic")
			svc, known := services[name]
			if err != nil || !ok || !known {
				return nil, plugin.Errorf(plugin.BadInput, "llm.takes takes a blob first, and service: a service this Corbie holds")
			}
			return svc.Takes(b.Type, b.Size) == nil, nil
		}},
	}})
}

func chat(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	said, ok := first.([]any)
	if !ok {
		return nil, plugin.Errorf(plugin.BadInput, "llm.chat takes the conversation so far, a list, first")
	}
	name, err := opt.Text("service", "anthropic")
	if err != nil {
		return nil, err
	}
	svc, ok := services[name]
	if !ok || svc.Chat == nil {
		return nil, plugin.Errorf(plugin.BadInput, "service: is one of %s, not %q", strings.Join(Having(func(s *Service) bool { return s.Chat != nil }), ", "), name)
	}
	c := &Call{Said: ownKept(said, name), Key: opt["key"]}
	if c.Model, err = opt.Text("model", ""); err != nil || c.Model == "" {
		return nil, plugin.Errorf(plugin.BadInput, "llm.chat needs model:, the model's name")
	}
	if c.System, err = opt.Text("system", ""); err != nil {
		return nil, err
	}
	if c.Tools, err = opt.List("tools"); err != nil {
		return nil, err
	}
	// The shape of answer wanted is given to a service that has a way to
	// take it, and otherwise told to the model in the system text, which
	// every service takes.
	shape, err := opt.Map("shape")
	if err != nil {
		return nil, err
	}
	switch {
	case len(shape) > 0 && svc.Shapes:
		c.Shape = shape
	case len(shape) > 0:
		c.System = strings.TrimSpace(c.System + "\n\n" + wanted(shape))
	}
	if c.Effort, err = opt.Text("effort", ""); err != nil {
		return nil, err
	}
	if c.Room, err = opt.Int("reply_tokens", 0); err != nil {
		return nil, err
	}
	if c.Address, err = opt.Text("address", svc.Address); err != nil {
		return nil, err
	}
	if c.Limit, err = opt.Duration("timeout", 3*time.Minute); err != nil {
		return nil, err
	}
	own, err := opt.List("own")
	if err != nil {
		return nil, err
	}
	for _, tool := range own {
		c.Own = append(c.Own, text(tool))
	}
	// Content the service does not take, or that is too large for it, is
	// refused before anything is sent.
	for _, part := range said {
		for _, holder := range append([]any{part}, list(field(part, "results"))...) {
			for _, file := range Files(holder) {
				if err := svc.Takes(file.Blob.Type, file.Blob.Size); err != nil {
					return nil, err
				}
			}
		}
	}
	r, err := svc.Chat(job, c)
	if err != nil {
		return nil, err
	}
	// A request for a tool whose arguments were cut off is never handed
	// back as a request; and a reply that holds requests for tools counts
	// as having stopped to ask for them, whatever the service says.
	switch {
	case r.Stop == StopLength:
		r.Calls = nil
	case len(r.Calls) > 0:
		r.Stop = StopTools
	}
	if r.Calls == nil {
		r.Calls = []any{}
	}
	out := map[string]any{"text": r.Text, "calls": r.Calls, "stop": r.Stop, "usage": r.Usage, "kept": nil}
	// What the service wants back unchanged is marked as that service's:
	// it is sent back to no other.
	if r.Kept != nil {
		out["kept"] = map[string]any{"service": name, "parts": r.Kept}
	}
	return out, nil
}

func choose(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	if first == nil {
		return nil, plugin.Errorf(plugin.BadInput, "llm.choose takes the state first: the text or data the questions are about")
	}
	name, err := opt.Text("service", "systemone")
	if err != nil {
		return nil, err
	}
	svc, ok := services[name]
	if !ok || svc.Choose == nil {
		return nil, plugin.Errorf(plugin.BadInput, "service: is one of %s, not %q", strings.Join(Having(func(s *Service) bool { return s.Choose != nil }), ", "), name)
	}
	q := &Questions{State: first, Key: opt["key"]}
	if q.Model, err = opt.Text("model", ""); err != nil || q.Model == "" {
		return nil, plugin.Errorf(plugin.BadInput, "llm.choose needs model:, the model's name")
	}
	if q.Questions, err = opt.Map("questions"); err != nil {
		return nil, err
	}
	if len(q.Questions) == 0 {
		return nil, plugin.Errorf(plugin.BadInput, "llm.choose needs questions:, each under its name with its type, instructions and criteria")
	}
	if q.Address, err = opt.Text("address", svc.Address); err != nil {
		return nil, err
	}
	if q.Limit, err = opt.Duration("timeout", 3*time.Minute); err != nil {
		return nil, err
	}
	a, err := svc.Choose(job, q)
	if err != nil {
		return nil, err
	}
	return map[string]any{"answers": a.Answers, "model": a.Model, "usage": a.Usage}, nil
}

// wanted says what shape of answer is wanted, as the model is told it: each
// name with the type of its value.
func wanted(shape map[string]any) string {
	names := make([]string, 0, len(shape))
	for name := range shape {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		kind, optional := strings.CutSuffix(text(shape[name]), "?")
		if kind == "blob" {
			kind = "the name of a blob, as it was shown"
		}
		if optional {
			kind += ", may be left out"
		}
		names[i] = name + " (" + kind + ")"
	}
	return "Give your final reply as JSON only, with these names: " + strings.Join(names, ", ") + "."
}

// ownKept gives the conversation as one service is to see it: the parts it
// wanted back unchanged as they came, and those another service wanted
// dropped. A service turns away parts it did not write, and what such a
// reply said and asked for is in the conversation without them.
func ownKept(said []any, service string) []any {
	out := make([]any, len(said))
	for i, part := range said {
		out[i] = part
		kept := field(part, "kept")
		if kept == nil {
			continue
		}
		seen := map[string]any{}
		for name, value := range fields(part) {
			seen[name] = value
		}
		delete(seen, "kept")
		if field(kept, "service") == service {
			seen["kept"] = field(kept, "parts")
		}
		out[i] = seen
	}
	return out
}

// A File is content beside text that a part of the conversation carries:
// an image or a document, under the name the model knows it by.
type File struct {
	Name string
	Blob *plugin.Blob
}

// Files gives the files a part of the conversation, or one tool's result,
// carries, in the order of their names.
func Files(holder any) []File {
	given := fields(field(holder, "files"))
	names := make([]string, 0, len(given))
	for name := range given {
		names = append(names, name)
	}
	sort.Strings(names)
	var files []File
	for _, name := range names {
		if b, ok := given[name].(*plugin.Blob); ok {
			files = append(files, File{Name: name, Blob: b})
		}
	}
	return files
}

// Data gives a file's contents as base64, which is how every service
// takes them.
func (f File) Data(job *plugin.Job) (string, error) {
	data, err := f.Blob.All(job)
	return base64.StdEncoding.EncodeToString(data), err
}

// web is a model service on the web: where its calls go, and how what is
// sent and what comes back are written in its way of talking.
type web struct {
	path string
	// headers gives the request's headers, given the key.
	headers func(key string) map[string]string
	// request builds what is sent, and reply reads the whole answer.
	request func(job *plugin.Job, c *Call) (map[string]any, error)
	reply   func(body map[string]any) (*Reply, error)
	// gather puts one event of the answer, as it arrives, into the whole
	// answer. It gives the text the event added, says whether the service
	// has said the answer is complete, and gives the service's own failure.
	gather func(whole, event map[string]any) (text string, complete bool, err error)
}

// send makes one request to a service on the web, and gives its answer to
// be read. The key is an ordinary secret, and goes only where it is bound
// to: a key bound to a host never goes over a plain connection. A call with
// no key sends none.
func (svc *web) send(ctx context.Context, job *plugin.Job, address string, key any, limit time.Duration, asked map[string]any) (io.ReadCloser, error) {
	target, err := url.Parse(strings.TrimSuffix(address, "/") + svc.path)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, plugin.Errorf(plugin.BadInput, "the model's address is not a web address")
	}
	if target.User != nil {
		return nil, plugin.Errorf(plugin.BadInput, "the model's address may not hold a user name or a password")
	}
	port := target.Port()
	if port == "" {
		port = "443"
		if target.Scheme == "http" {
			port = "80"
		}
	}
	revealed, err := job.RevealOver(key, net.JoinHostPort(target.Hostname(), port), target.Scheme == "https")
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(asked, json.Deterministic(true))
	if err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "the request cannot be sent: %v", err)
	}
	client := &nethttp.Client{
		Transport: &nethttp.Transport{Proxy: nil, DisableKeepAlives: true,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) { return job.Dial(ctx, address) }},
		// A call to a model service follows no redirect: an answer from
		// anywhere else is not the service's.
		CheckRedirect: func(*nethttp.Request, []*nethttp.Request) error { return nethttp.ErrUseLastResponse },
	}
	req, err := nethttp.NewRequestWithContext(ctx, "POST", target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "the request cannot be made")
	}
	req.Header.Set("Content-Type", "application/json")
	if revealed != "" {
		for name, value := range svc.headers(revealed) {
			req.Header.Set(name, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		if refusal := plugin.Refusal(err); refusal != nil {
			return nil, refusal
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, plugin.Errorf(plugin.TimedOut, "the model did not answer within %s", limit)
		}
		return nil, plugin.Errorf(plugin.Refused, "the model service could not be reached: %v", errors.Unwrap(err))
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, plugin.Errorf(plugin.Refused, "the model service answered %d: %s", resp.StatusCode, clip(job.Mask(string(data))))
	}
	return resp.Body, nil
}

// chat makes one call to a service on the web.
func (svc *web) chat(job *plugin.Job, c *Call) (*Reply, error) {
	// Every answer is asked for as it is written, and read that way.
	asked, err := svc.request(job, c)
	if err != nil {
		return nil, err
	}
	asked["stream"] = true
	ctx, cancel := context.WithTimeout(job.Context, c.Limit)
	defer cancel()
	body, err := svc.send(ctx, job, c.Address, c.Key, c.Limit, asked)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	answer, err := arriving(job, svc, body)
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, plugin.Errorf(plugin.TimedOut, "the model did not finish its answer within %s", c.Limit)
	}
	if err != nil {
		return nil, err
	}
	return svc.reply(answer)
}

// choose puts questions to a System One model on the web. Its answer comes
// whole, as one JSON value, and the answers in it go back as they came.
func (svc *web) choose(job *plugin.Job, q *Questions) (*Answers, error) {
	ctx, cancel := context.WithTimeout(job.Context, q.Limit)
	defer cancel()
	body, err := svc.send(ctx, job, q.Address, q.Key, q.Limit, map[string]any{"model": q.Model, "state": q.State, "questions": q.Questions})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, 64<<20))
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, plugin.Errorf(plugin.TimedOut, "the model did not finish its answer within %s", q.Limit)
	}
	if err != nil {
		return nil, plugin.Errorf(plugin.Refused, "the model's answer could not be read: %v", err)
	}
	var answer map[string]any
	if err := json.Unmarshal(data, &answer); err != nil {
		return nil, plugin.Errorf(plugin.Refused, "the model service's answer is not JSON")
	}
	answers := fields(answer["answers"])
	if answers == nil {
		return nil, plugin.Errorf(plugin.Refused, "the model service's answer holds no answers")
	}
	usage := answer["usage"]
	return &Answers{Model: text(answer["model"]), Answers: answers,
		Usage: map[string]any{"input": number(field(usage, "input_tokens")), "output": number(field(usage, "output_tokens"))}}, nil
}

// arriving reads a service's answer as it arrives, a stream of events, and
// gives the whole of it. The text each event adds is emitted as it comes,
// so a watcher sees the reply being written. An answer that stops before
// the service has said it is complete is not an answer.
func arriving(job *plugin.Job, svc *web, body io.Reader) (map[string]any, error) {
	whole := map[string]any{}
	complete := false
	said := job.Telling("text")
	defer said.End()
	in := bufio.NewReader(io.LimitReader(body, 64<<20))
	var data []string
	for {
		line, readErr := in.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimPrefix(rest, " "))
		}
		// An event ends at an empty line, or where the stream does.
		if (line == "" || readErr != nil) && len(data) > 0 {
			payload := strings.Join(data, "\n")
			data = nil
			if payload != "[DONE]" {
				var event map[string]any
				if err := json.Unmarshal([]byte(payload), &event); err != nil {
					return nil, plugin.Errorf(plugin.Refused, "the model service's answer is not JSON")
				}
				text, done, err := svc.gather(whole, event)
				if err != nil {
					return nil, err
				}
				said.Add(text)
				complete = complete || done
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if errors.Is(readErr, context.DeadlineExceeded) {
				return nil, readErr
			}
			return nil, plugin.Errorf(plugin.Refused, "the model's answer could not be read: %v", readErr)
		}
	}
	if !complete {
		return nil, plugin.Errorf(plugin.Refused, "the model's answer was cut short: the service stopped before it had finished")
	}
	return whole, nil
}

// failed is a failure the service reported in the middle of its answer.
func failed(said any) error {
	return plugin.Errorf(plugin.Refused, "the model service failed while answering: %s", clip(text(said)))
}

func clip(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 400 {
		return text[:400] + "…"
	}
	return text
}

// Schema, Text and ResultText are for a service kept in a package of its
// own.
func Schema(args any) map[string]any       { return schema(args) }
func Text(v any) string                    { return text(v) }
func ResultText(result any) (string, bool) { return resultText(result) }

// schema gives the shape of a tool's arguments as the services want it.
// Where a shape asks for a blob, the model gives the name it was shown.
func schema(args any) map[string]any {
	shape, _ := args.(map[string]any)
	properties := map[string]any{}
	required := []any{}
	names := make([]string, 0, len(shape))
	for name := range shape {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t, _ := shape[name].(string)
		if !strings.HasSuffix(t, "?") {
			required = append(required, name)
		}
		var p map[string]any
		switch strings.TrimSuffix(t, "?") {
		case "string":
			p = map[string]any{"type": "string"}
		case "number":
			p = map[string]any{"type": "number"}
		case "boolean":
			p = map[string]any{"type": "boolean"}
		case "list":
			p = map[string]any{"type": "array"}
		case "map":
			p = map[string]any{"type": "object"}
		case "blob":
			p = map[string]any{"type": "string", "description": "the name of a blob, as it was shown: blob1, blob2, …"}
		default:
			p = map[string]any{}
		}
		properties[name] = p
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

// text gives a part of the conversation as text for a service that takes
// only text there.
func text(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	data, _ := json.Marshal(v, json.Deterministic(true))
	return string(data)
}

func field(m any, name string) any {
	if mm, ok := m.(map[string]any); ok {
		return mm[name]
	}
	return nil
}

func fields(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

// resultText is what a tool's result reads as to the model.
func resultText(result any) (string, bool) {
	if e := field(result, "error"); e != nil {
		out := "error: " + text(e)
		if r := field(result, "result"); r != nil {
			out += "\noutput: " + text(r)
		}
		return out, true
	}
	return text(field(result, "result")), false
}

func number(v any) float64 {
	n, _ := v.(float64)
	return n
}

func argsOf(raw any) any {
	if s, ok := raw.(string); ok {
		var v any
		if json.Unmarshal([]byte(s), &v) == nil {
			return v
		}
		return map[string]any{}
	}
	if raw == nil {
		return map[string]any{}
	}
	return raw
}

func argsText(args any) string {
	if args == nil {
		return "{}"
	}
	data, _ := json.Marshal(args, json.Deterministic(true))
	return string(data)
}
