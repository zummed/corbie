package client

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/store"
)

// Remote makes a client that sends everything to a gateway, as the client
// the token names. It holds no key and reads no store: each thing the
// package does is one request to the gateway, which does it.
func Remote(address, token string) *Client {
	return &Client{Now: time.Now, remote: &remote{address: strings.TrimSuffix(address, "/"), token: token, receipts: map[string]*instance.Receipt{}}}
}

// A remote is a gateway a client sends to.
type remote struct {
	address, token string

	// receipts are the answers the gateway gave to what was sent, each
	// until WaitReceipt takes it: the gateway answers a message with its
	// receipt, where a store gives the two apart.
	mu       sync.Mutex
	receipts map[string]*instance.Receipt
}

// refused is a request the gateway refused, in the gateway's words. Is says
// which of the package's errors it is.
type refused struct {
	text string
	is   error
}

func (r *refused) Error() string { return r.text }
func (r *refused) Unwrap() error { return r.is }

// known are the package's errors a gateway's refusal may be, the narrower
// before the wider.
var known = []error{ErrNeverTaken, ErrOthers, ErrNoRun, ErrNoFile, ErrFilesTooLarge, ErrNoStep, ErrCalled, ErrNoRecord, ErrNotTaken, ErrStart}

// do sends one request and gives its answer, which the caller closes. A
// refusal is given as the error the package gives for it.
func (r *remote) do(ctx context.Context, method, path, kind string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, r.address+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	if kind != "" {
		req.Header.Set("Content-Type", kind)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the gateway %s: %w", r.address, err)
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var said struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &said) != nil || said.Error == "" {
		return nil, fmt.Errorf("the gateway %s answered %s", r.address, resp.Status)
	}
	out := &refused{text: said.Error}
	for _, err := range known {
		if strings.Contains(said.Error, err.Error()) {
			out.is = err
			break
		}
	}
	return nil, out
}

// get reads the JSON a route answers.
func (r *remote) get(ctx context.Context, path string, into any) error {
	resp, err := r.do(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.UnmarshalRead(resp.Body, into)
}

// post sends JSON to a route and reads the JSON it answers.
func (r *remote) post(ctx context.Context, path string, body, into any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := r.do(ctx, http.MethodPost, path, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.UnmarshalRead(resp.Body, into)
}

// sent keeps the receipt the gateway answered a message with, and gives the
// name WaitReceipt takes it by.
func (r *remote) sent(receipt *instance.Receipt) string {
	random := make([]byte, 8)
	rand.Read(random)
	name := "gateway-" + hex.EncodeToString(random)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.receipts[name] = receipt
	return name
}

// send posts a message to a route, which answers with its receipt.
func (r *remote) send(ctx context.Context, path string, body any) (string, error) {
	var receipt instance.Receipt
	if err := r.post(ctx, path, body, &receipt); err != nil {
		return "", err
	}
	return r.sent(&receipt), nil
}

// receipt takes the receipt of a message this client sent.
func (r *remote) receipt(message string) (*instance.Receipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	receipt, has := r.receipts[message]
	if !has {
		return nil, fmt.Errorf("%s is no message this client sent to the gateway", message)
	}
	delete(r.receipts, message)
	return receipt, nil
}

// lived gives a request's fields with the life a message is sent with,
// where it is given one.
func lived(fields map[string]any, life time.Duration) map[string]any {
	if life > 0 {
		fields["life"] = life.String()
	}
	return fields
}

// form writes an upload form: a part of JSON, and a part for each file.
func form(name string, value any, files map[string]io.Reader) (kind string, body *bytes.Buffer, err error) {
	body = &bytes.Buffer{}
	w := multipart.NewWriter(body)
	data, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	if err := w.WriteField(name, string(data)); err != nil {
		return "", nil, err
	}
	for part, contents := range files {
		file, filename, _ := strings.Cut(part, "\x00")
		out, err := w.CreateFormFile(file, filename)
		if err != nil {
			return "", nil, err
		}
		if _, err := io.Copy(out, contents); err != nil {
			return "", nil, err
		}
	}
	return w.FormDataContentType(), body, w.Close()
}

func (r *remote) start(ctx context.Context, start Start) (string, error) {
	if start.Run != "" {
		return "", fmt.Errorf("%w: a gateway names the run it starts", ErrStart)
	}
	request := map[string]any{"at": start.At}
	files := map[string]io.Reader{}
	input := start.Input
	if named, ok := input.(map[string]any); ok {
		// A file is a part of the form, under its name in the input.
		rest := map[string]any{}
		for name, value := range named {
			if file, isFile := value.(InputFile); isFile {
				files["input:"+name+"\x00"+file.Name] = file.Contents
			} else {
				rest[name] = value
			}
		}
		input = rest
		if len(rest) == 0 && len(files) > 0 {
			input = nil
		}
	}
	if input != nil {
		request["input"] = input
	}
	if len(start.Labels) > 0 {
		request["labels"] = start.Labels
	}
	if start.Allow != nil {
		if len(start.Allow.Allow) > 0 {
			request["allow"] = start.Allow.Allow
		}
		if len(start.Allow.Deny) > 0 {
			request["deny"] = start.Allow.Deny
		}
	}
	if start.Timeout > 0 {
		request["timeout"] = start.Timeout.String()
	}
	kind, body, err := form("start", lived(request, start.Life), files)
	if err != nil {
		// A file anywhere else in the input cannot be written as JSON.
		return "", err
	}
	resp, err := r.do(ctx, http.MethodPost, "/runs", kind, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var started struct {
		ID string `json:"id"`
	}
	return started.ID, json.UnmarshalRead(resp.Body, &started)
}

func (r *remote) submit(ctx context.Context, tree *Tree, life time.Duration) (string, error) {
	if tree.Files == nil {
		return "", errors.New("a tree sent to a gateway is one ReadTree or ReadFile read: the gateway is sent the files")
	}
	request := map[string]any{"at": tree.Prefix, "directory": tree.Directory}
	if tree.File != "" {
		request["file"] = tree.File
	}
	files := map[string]io.Reader{}
	for path, contents := range tree.Files {
		files["file:"+path+"\x00"+path] = bytes.NewReader(contents)
	}
	kind, body, err := form("submit", lived(request, life), files)
	if err != nil {
		return "", err
	}
	resp, err := r.do(ctx, http.MethodPost, "/trees", kind, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var answer struct {
		instance.Receipt
		// The gateway's own reading of each file, which this client has made
		// too.
		Problems map[string]string `json:"problems"`
	}
	if err := json.UnmarshalRead(resp.Body, &answer); err != nil {
		return "", err
	}
	answer.Receipt.Problems = nil
	return r.sent(&answer.Receipt), nil
}

func (r *remote) sendTo(ctx context.Context, to machine.Address, event string, data any, life time.Duration) (string, error) {
	body := map[string]any{"event": event}
	if data != nil {
		body["data"] = data
	}
	lived(body, life)
	if to.Run != "" {
		return r.send(ctx, "/runs/"+to.Run+"/events", body)
	}
	body["to"] = to.At
	if to.Key != nil {
		body["to"] = to.At + "#" + *to.Key
	}
	return r.send(ctx, "/events", body)
}

func (r *remote) setting(ctx context.Context, life time.Duration, set instance.Setting) (string, error) {
	body := lived(map[string]any{"act": set.Act}, life)
	for name, value := range map[string]string{"at": set.At, "name": set.Name, "value": set.Value} {
		if value != "" {
			body[name] = value
		}
	}
	return r.send(ctx, "/"+set.Of+"s", body)
}

// query writes a filter as a listing's parameters.
func (f Filter) query() url.Values {
	q := url.Values{}
	for name, value := range map[string]string{"status": f.Status, "parent": f.Parent, "at": f.Location, "key": f.Key} {
		if value != "" {
			q.Set(name, value)
		}
	}
	if f.Finished {
		q.Set("all", "")
	}
	for name, value := range f.Labels {
		if value != "" {
			name += "=" + value
		}
		q.Add("label", name)
	}
	return q
}

// stream reads a route's stream and gives each event's name and data, until
// the stream ends or give says to stop.
func (r *remote) stream(ctx context.Context, path string, give func(name string, data []byte) (bool, error)) error {
	resp, err := r.do(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	lines := bufio.NewReader(resp.Body)
	name := ""
	for {
		line, err := lines.ReadString('\n')
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("the gateway %s closed the stream without saying why", r.address)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := []byte(strings.TrimPrefix(line, "data: "))
			if name == "error" {
				var said struct {
					Error string `json:"error"`
				}
				json.Unmarshal(data, &said)
				return errors.New(said.Error)
			}
			if done, err := give(name, data); done || err != nil {
				return err
			}
		}
	}
}

func (r *remote) watch(ctx context.Context, filter Filter, tell func(Update)) error {
	q := filter.query()
	q.Set("watch", "")
	return r.stream(ctx, "/runs?"+q.Encode(), func(name string, data []byte) (bool, error) {
		var u Update
		if err := json.Unmarshal(data, &u); err != nil {
			return false, err
		}
		tell(u)
		return false, nil
	})
}

func (r *remote) follow(ctx context.Context, id string, tell func(Entry)) (*Run, error) {
	var ended *Run
	err := r.stream(ctx, "/runs/"+id+"/record?watch", func(name string, data []byte) (bool, error) {
		if name == "end" {
			ended = &Run{}
			return true, json.Unmarshal(data, ended)
		}
		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			return false, err
		}
		tell(entry)
		return false, nil
	})
	if ctx.Err() != nil {
		// A follow that is cut short gives the run as it stands.
		var run Run
		return &run, r.get(context.WithoutCancel(ctx), "/runs/"+id, &run)
	}
	return ended, err
}

func (r *remote) record(ctx context.Context, id string, tell func(Entry)) (*Run, error) {
	var record struct {
		Entries []Entry `json:"entries"`
		Run     *Run    `json:"run"`
	}
	if err := r.get(ctx, "/runs/"+id+"/record", &record); err != nil {
		return nil, err
	}
	for _, entry := range record.Entries {
		tell(entry)
	}
	return record.Run, nil
}

func (r *remote) fetch(ctx context.Context, id, path string) (io.Reader, File, error) {
	resp, err := r.do(ctx, http.MethodGet, "/runs/"+id+"/files/"+(&url.URL{Path: path}).EscapedPath(), "", nil)
	if err != nil {
		return nil, File{}, err
	}
	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	return &whole{resp.Body}, File{Type: resp.Header.Get("Content-Type"), Size: size}, nil
}

// whole reads an answer's body, and closes it once it is read to its end
// or fails.
type whole struct{ body io.ReadCloser }

func (w *whole) Read(p []byte) (int, error) {
	n, err := w.body.Read(p)
	if err != nil {
		w.body.Close()
	}
	return n, err
}

// address reads an address as a client writes it. A run is named by its ID,
// and the gateway finds where it is.
func (r *remote) addressOf(text string) (machine.Address, error) {
	if err := store.CheckInstance(text); err != nil {
		return machine.Address{}, fmt.Errorf("%q is neither a location, which starts with /, nor a run's ID", text)
	}
	return machine.Address{Run: text}, nil
}
