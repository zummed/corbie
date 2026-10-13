package http

import (
	"context"
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

func Test_plug_http_held_body_bounded(t *testing.T) {
	const size = plugin.MaxWhole + 1<<20
	var received int64
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method == "PUT" {
			received, _ = io.Copy(io.Discard, r.Body)
			return
		}
		megabyte := make([]byte, 1<<20)
		for range size >> 20 {
			w.Write(megabyte)
		}
	}))
	defer server.Close()
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(),
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network"}}}}}
	t.Cleanup(job.End)
	module, _ := plugin.Find("http")
	get := func(opt plugin.Options) (any, error) {
		return module.Calls["get"].Invoke(job, "http", "get", server.URL, opt)
	}
	// Held whole, to be handed to the script, a body has a largest size.
	if _, err := get(nil); err == nil || !strings.Contains(err.Error(), `ask for it as a blob, with { as: "blob" }`) {
		t.Fatalf("a body past the largest that is held gave %v", err)
	}
	// Going into a blob it is passed on as it arrives, whatever its size:
	// it is a file in the scratch directory, and no more.
	out, err := get(plugin.Options{"as": "blob"})
	if err != nil {
		t.Fatal(err)
	}
	body := out.(map[string]any)["body"].(*plugin.Blob)
	files, _ := os.ReadDir(job.Scratch)
	if body.Size != size || len(files) != 1 {
		t.Fatalf("the blob is %d bytes, and the scratch directory holds %d files", body.Size, len(files))
	}
	// And a blob sent as a body is sent as it is read.
	if _, err := module.Calls["put"].Invoke(job, "http", "put", server.URL, plugin.Options{"body": body}); err != nil || received != size {
		t.Fatalf("sending the blob: %v, and the server received %d bytes", err, received)
	}
}

// A secret was allowed for the place the address names without it, so its
// value cannot move the request to another.
func Test_sec_value_cannot_choose_where_it_goes(t *testing.T) {
	asked := 0
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { asked++ }))
	defer server.Close()
	host, port, _ := strings.Cut(strings.TrimPrefix(server.URL, "http://"), ":")
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(),
		Secrets: map[string]string{"PORT": port, "TAIL": ".example", "WAY": "s", "PATH": "/a/b"},
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network", "secret:*@*"}}}}}
	t.Cleanup(job.End)
	module, _ := plugin.Find("http")
	get := func(pieces ...any) error {
		address, _ := plugin.Join(pieces...)
		_, err := module.Calls["get"].Invoke(job, "http", "get", address, nil)
		return err
	}
	for name, pieces := range map[string][]any{
		"port":           {"http://" + host + ":", plugin.NewSecret("PORT"), "/"},
		"tail of a host": {"http://" + host, plugin.NewSecret("TAIL"), ":" + port + "/"},
		"whole host":     {"http://", plugin.NewSecret("TAIL"), ":" + port + "/"},
		"scheme":         {"http", plugin.NewSecret("WAY"), "://" + host + ":" + port + "/"},
		"user":           {"http://", plugin.NewSecret("WAY"), "@" + host + ":" + port + "/"},
	} {
		var refused *plugin.Error
		if err := get(pieces...); !errors.As(err, &refused) || refused.Code != plugin.BadInput {
			t.Errorf("a secret as the %s gave %v", name, err)
		}
	}
	if asked != 0 {
		t.Fatalf("%d requests were sent", asked)
	}
	// In the path, and after it, a secret is in its place.
	if err := get(server.URL, plugin.NewSecret("PATH"), "?q=", plugin.NewSecret("WAY")); err != nil || asked != 1 {
		t.Fatalf("a secret in the path gave %v, and %d requests", err, asked)
	}
}

// A redirect is followed to a web address only, and twenty at most.
func Test_plug_http_redirect_kinds_and_limit(t *testing.T) {
	asked := 0
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		asked++
		switch {
		case strings.HasPrefix(r.URL.Path, "/to/"):
			w.Header().Set("Location", strings.TrimPrefix(r.URL.Path, "/to/")+":///etc/passwd")
			w.WriteHeader(302)
		case r.URL.Path == "/left/0":
			w.Write([]byte("there"))
		case strings.HasPrefix(r.URL.Path, "/left/"):
			n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/left/"))
			nethttp.Redirect(w, r, "/left/"+strconv.Itoa(n-1), 302)
		default:
			nethttp.Redirect(w, r, r.URL.Path, 302)
		}
	}))
	defer server.Close()
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(),
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network"}}}}}
	t.Cleanup(job.End)
	module, _ := plugin.Find("http")
	get := func(path string) (any, error) {
		return module.Calls["get"].Invoke(job, "http", "get", server.URL+path, nil)
	}
	for _, kind := range []string{"file", "ftp", "javascript"} {
		if _, err := get("/to/" + kind); err == nil || !strings.Contains(err.Error(), "not a web address") {
			t.Errorf("a redirect to a %s address gave %v", kind, err)
		}
	}
	// Twenty redirects are followed, and the twenty-first is not.
	asked = 0
	out, err := get("/left/20")
	if err != nil || out.(map[string]any)["body"] != "there" || asked != 21 {
		t.Fatalf("twenty redirects gave %v, %v after %d requests", out, err, asked)
	}
	asked = 0
	if _, err := get("/left/21"); err == nil || !strings.Contains(err.Error(), "redirected more than 20 times") || asked != 21 {
		t.Fatalf("twenty-one redirects gave %v after %d requests", err, asked)
	}
	asked = 0
	if _, err := get("/round"); err == nil || asked != 21 {
		t.Fatalf("a server that redirects to itself gave %v after %d requests", err, asked)
	}
}

// uploaded reads an upload as the server received it: each field's text,
// and each file's name, type and contents.
type uploaded struct {
	fields map[string]string
	names  map[string]string
	types  map[string]string
	raw    string
}

func uploadServer(t *testing.T) (*httptest.Server, *uploaded) {
	got := &uploaded{}
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		*got = uploaded{fields: map[string]string{}, names: map[string]string{}, types: map[string]string{}}
		raw, _ := io.ReadAll(r.Body)
		got.raw = string(raw)
		if r.ContentLength != int64(len(raw)) {
			t.Errorf("the upload said it was %d bytes and was %d", r.ContentLength, len(raw))
		}
		r.Body = io.NopCloser(strings.NewReader(got.raw))
		form, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		for {
			part, err := form.NextPart()
			if err != nil {
				break
			}
			contents, _ := io.ReadAll(part)
			got.fields[part.FormName()] = string(contents)
			if part.FileName() != "" {
				got.names[part.FormName()] = part.FileName()
				got.types[part.FormName()] = part.Header.Get("Content-Type")
			}
		}
	}))
	t.Cleanup(server.Close)
	return server, got
}

// A form with a field and a file reaches the server as an upload, the file
// given as bytes and again as a blob that is sent as it is read.
func TestUploadCarriesFieldsAndFiles(t *testing.T) {
	server, got := uploadServer(t)
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(), Secrets: map[string]string{"TOKEN": "s3cret-value"},
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network", "secret:*@*"}}}}}
	t.Cleanup(job.End)
	module, _ := plugin.Find("http")
	post := func(fields map[string]any) error {
		_, err := module.Calls["post"].Invoke(job, "http", "post", server.URL, plugin.Options{"upload": fields})
		return err
	}
	if err := post(map[string]any{"title": "the report", "count": 3.0, "token": plugin.NewSecret("TOKEN"),
		"file": map[string]any{"data": plugin.Bytes("\x00\x01 bytes \xff"), "name": "report.pdf", "type": "application/pdf"}}); err != nil {
		t.Fatal(err)
	}
	if got.fields["title"] != "the report" || got.fields["count"] != "3" || got.fields["token"] != "s3cret-value" ||
		got.fields["file"] != "\x00\x01 bytes \xff" || got.names["file"] != "report.pdf" || got.types["file"] != "application/pdf" || len(got.names) != 1 {
		t.Fatalf("the server received %+v", got)
	}
	large := strings.Repeat("every byte of it\n", 200000)
	file := plugin.NewBlob("text/csv", []byte(large))
	if err := post(map[string]any{"title": "again", "file": map[string]any{"data": file, "name": "table.csv"}, "bare": file, "text": map[string]any{"data": "as text", "name": "note.txt", "type": "text/plain"}}); err != nil {
		t.Fatal(err)
	}
	if got.fields["file"] != large || got.names["file"] != "table.csv" || got.types["file"] != "text/csv" ||
		got.fields["bare"] != large || got.names["bare"] != "bare" || got.fields["text"] != "as text" || got.names["text"] != "note.txt" {
		t.Fatalf("the server received a file named %v of type %v, %d bytes", got.names, got.types, len(got.fields["file"]))
	}
	// A request that only fetches carries no upload, and a file is no more
	// than its three parts.
	if _, err := module.Calls["get"].Invoke(job, "http", "get", server.URL, plugin.Options{"upload": map[string]any{"a": "b"}}); err == nil {
		t.Error("a GET carried an upload")
	}
	if err := post(map[string]any{"file": map[string]any{"data": "x", "filename": "a"}}); err == nil {
		t.Error("a file with a part that is not one of the three was sent")
	}
}

func Test_plug_http_upload_names_escaped(t *testing.T) {
	server, got := uploadServer(t)
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(),
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network"}}}}}
	t.Cleanup(job.End)
	module, _ := plugin.Find("http")
	post := func(fields map[string]any) error {
		_, err := module.Calls["post"].Invoke(job, "http", "post", server.URL, plugin.Options{"upload": fields})
		return err
	}
	field := "a\"; filename=\"x\r\nX-Made-Up: 1\r\n\r\nbody"
	name := "b\".txt\r\nContent-Type: made-up\r\n\r\n<script>"
	if err := post(map[string]any{field: "one", "file": map[string]any{"data": plugin.Bytes("two"), "name": name}}); err != nil {
		t.Fatal(err)
	}
	// Each arrives as one part, under a name with no quote or line break
	// in it, and nothing written into a name became a header or contents.
	escaped := func(s string) string {
		return strings.NewReplacer("\"", "%22", "\r", "%0D", "\n", "%0A").Replace(s)
	}
	if len(got.fields) != 2 || got.fields[escaped(field)] != "one" || got.fields["file"] != "two" || got.names["file"] != escaped(name) || len(got.names) != 1 {
		t.Fatalf("the server received fields %q and files %q", got.fields, got.names)
	}
	if strings.Contains(got.raw, "\r\nX-Made-Up") || strings.Contains(got.raw, "\r\nContent-Type: made-up") {
		t.Fatalf("a name started a header of its own:\n%s", got.raw)
	}
	// In a part's type there is nothing to escape it as: it is refused.
	for _, typ := range []string{"text/plain\r\nX-Made-Up: 1", "text/plain\nX: 1", "a\x00b"} {
		err := post(map[string]any{"file": map[string]any{"data": plugin.Bytes("two"), "name": "a.txt", "type": typ}})
		var refused *plugin.Error
		if !errors.As(err, &refused) || refused.Code != plugin.BadInput {
			t.Errorf("a type of %q gave %v", typ, err)
		}
	}
}

// Only request takes a method as an option. A call named for its method
// refuses one: it would otherwise be taken and not acted on.
func TestOnlyRequestTakesAMethod(t *testing.T) {
	var sent []string
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { sent = append(sent, r.Method) }))
	defer server.Close()
	job := &plugin.Job{Context: context.Background(), Scratch: t.TempDir(),
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"network"}}}}}
	t.Cleanup(job.End)
	module, _ := plugin.Find("http")
	if _, err := module.Calls["get"].Invoke(job, "http", "get", server.URL, plugin.Options{"method": "DELETE"}); err == nil || !strings.Contains(err.Error(), `does not know the option "method"`) {
		t.Fatalf("a get given a method gave %v, and sent %v", err, sent)
	}
	if _, err := module.Calls["request"].Invoke(job, "http", "request", server.URL, plugin.Options{"method": "delete"}); err != nil || strings.Join(sent, " ") != "DELETE" {
		t.Fatalf("a request given the method delete gave %v, and sent %v", err, sent)
	}
}
