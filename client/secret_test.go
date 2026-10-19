package client_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zummed/corbie/client"
)

// echo is a server that says back what it was sent.
func echo(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Seen", r.Header.Get("Authorization"))
		io.WriteString(w, "you sent "+r.Header.Get("Authorization")+" and "+string(body)+" to "+r.URL.RawQuery)
	}))
	t.Cleanup(s.Close)
	return s
}

func runWithSecret(t *testing.T, l *client.Local, def string, files map[string]string, input any) *client.Run {
	t.Helper()
	id, err := l.Start(ctx, draft(t, def, files), client.Start{Input: input, Secrets: map[string]string{"TOKEN": "tok-en/123456789"}})
	if err != nil {
		t.Fatal(err)
	}
	return wait(t, l, id)
}

const usesSecret = `
	flow: uses
	input: { url: any, host: any }
	stages:
	  call:
	    job:
	      script: ./call.js
	      input: { url: "${input.url}" }
	      allow: ["net:${input.host}", "secret:TOKEN@*"]
	output: ${stages.call.output}
`

// A script never has a secret's value: it uses a handle, and what comes
// back with the value in it is masked.
func TestSecretIsUsedAndNeverSeen(t *testing.T) {
	server := echo(t)
	host := strings.TrimPrefix(server.URL, "http://")
	l := local(t, "all")
	r := runWithSecret(t, l, usesSecret, map[string]string{"call.js": `
		const token = secrets.get("TOKEN")
		const r = http.post(input.url, {
			headers: { Authorization: secrets.text` + "`Bearer ${token}`" + ` },
			query: { key: token },
			body: "plain",
		})
		return { status: r.status, body: r.body, header: r.headers["x-seen"] }
	`}, map[string]any{"url": server.URL, "host": host})
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := jsonOf(r.Output)
	if strings.Contains(out, "tok-en") || strings.Contains(out, "123456789") {
		t.Fatalf("the secret's value reached the script's output: %s", out)
	}
	if !strings.Contains(out, "Bearer [secret TOKEN]") || !strings.Contains(out, "key=[secret TOKEN]") {
		t.Fatalf("what the server echoed was not masked by name: %s", out)
	}
	if !strings.Contains(strings.Join(r.Used, " "), "secret:TOKEN@net.unencrypted:"+host) {
		t.Fatalf("the secret is not among what was used: %v", r.Used)
	}
}

func Test_sec_every_conversion_throws(t *testing.T) {
	l := local(t, "all")
	r := runWithSecret(t, l, usesSecret, map[string]string{"call.js": `
		const token = secrets.get("TOKEN")
		const tried = {}
		const ways = {
			string: () => String(token),
			template: () => ` + "`${token}`" + `,
			concat: () => "x" + token,
			json: () => JSON.stringify({ token }),
			compare: () => token == "tok-en/123456789",
			number: () => +token,
			toString: () => token.toString(),
			length: () => token.length,
			keys: () => Object.keys(token).length,
		}
		for (const [name, way] of Object.entries(ways)) {
			try { tried[name] = "gave " + way() } catch (e) { tried[name] = "threw" }
		}
		return tried
	`}, map[string]any{"url": "http://unused.example", "host": "unused.example"})
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	for _, way := range []string{"string", "template", "concat", "json", "compare", "number", "toString"} {
		if out[way] != "threw" {
			t.Errorf("turning a handle into text by %s %v", way, out[way])
		}
	}
	// Measuring it or reading it a piece at a time finds nothing.
	if out["length"] != "gave undefined" || out["keys"] != "gave 0" {
		t.Errorf("a handle can be measured: length %v, keys %v", out["length"], out["keys"])
	}
	if strings.Contains(jsonOf(out), "tok-en") {
		t.Fatalf("a value got out: %s", jsonOf(out))
	}
}

func TestSecretGoesOnlyWhereItIsBound(t *testing.T) {
	server := echo(t)
	host := strings.TrimPrefix(server.URL, "http://")
	l := local(t, "all")
	// Bound to another host: nothing is sent.
	def := strings.Replace(usesSecret, `"secret:TOKEN@*"`, `"secret:TOKEN@net:api.example.com"`, 1)
	r := runWithSecret(t, l, def, map[string]string{"call.js": `
		return http.get(input.url, { headers: { Authorization: secrets.get("TOKEN") } }).status
	`}, map[string]any{"url": server.URL, "host": host})
	if r.Status != "failed" || r.Failure != "denied" || !strings.HasPrefix(r.Refused, "secret:TOKEN@") {
		t.Fatalf("a secret sent where it is not bound: %s/%s refused %q", r.Status, r.Failure, r.Refused)
	}
	// A handle cannot leave the job, nor be the thing that is hashed.
	for script, want := range map[string]string{
		`return { t: secrets.get("TOKEN") }`:           "a secret cannot leave a job",
		`return crypto.hash(secrets.get("TOKEN"))`:     "a secret is not contents",
		`emit("leak", secrets.get("TOKEN")); return 1`: "a secret cannot leave a job",
		`return secrets.get("OTHER")`:                  `was not sent a secret named "OTHER"`,
	} {
		r := runWithSecret(t, l, usesSecret, map[string]string{"call.js": script}, map[string]any{"url": server.URL, "host": host})
		if r.Status != "failed" || !strings.Contains(r.Error, want) {
			t.Errorf("%s: ended %s with %q, want an error holding %q", script, r.Status, r.Error, want)
		}
	}
}

func Test_sec_secret_file_private(t *testing.T) {
	dir := t.TempDir()
	l := local(t, "all")
	def := strings.Replace(usesSecret, `"net:${input.host}", "secret:TOKEN@*"`, `"fs.write:${input.host}/**", "secret:TOKEN@fs.write:${input.host}/key"`, 1)
	r := runWithSecret(t, l, def, map[string]string{"call.js": `
		return fs.write(input.url, { secret: secrets.get("TOKEN") })
	`}, map[string]any{"url": filepath.Join(dir, "key"), "host": dir})
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	info, err := os.Stat(filepath.Join(dir, "key"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the file a secret was written into has mode %v (%v)", info.Mode().Perm(), err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "key")); string(got) != "tok-en/123456789" {
		t.Fatalf("the file holds %q", got)
	}
}

// A program a job starts is not handed the job's own channel to its
// runner.
func TestStartedProgramDoesNotInheritTheJobsChannel(t *testing.T) {
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("needs /proc")
	}
	l := local(t, "all")
	r := run(t, l, `
		flow: fds
		stages:
		  look:
		    job: { script: ./look.js, allow: ["exec:sh"] }
		output: ${stages.look.output}
	`, map[string]string{"look.js": `
		return exec.run("sh", { args: ["-c", "for fd in 3 4; do test -e /proc/$$/fd/$fd && echo open || echo closed; done"] }).out
	`}, nil)
	if r.Status != "done" || r.Output != "closed\nclosed\n" {
		t.Fatalf("a started program sees the job's channel: %s %q %s", r.Status, r.Output, r.Error)
	}
}

// A script sends an upload: a field, a file made of bytes, and a file that
// is a blob.
func TestScriptSendsAnUpload(t *testing.T) {
	var fields, names []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		for name, values := range r.MultipartForm.Value {
			fields = append(fields, name+"="+values[0])
		}
		for name, files := range r.MultipartForm.File {
			file, _ := files[0].Open()
			contents, _ := io.ReadAll(file)
			names = append(names, name+"="+files[0].Filename+":"+files[0].Header.Get("Content-Type")+":"+string(contents))
		}
	}))
	defer server.Close()
	l := local(t, "network")
	r := script(t, l, `
		const r = http.post("`+server.URL+`", { upload: {
		  title: "the report",
		  raw: { data: new Uint8Array([97, 98, 99]), name: "abc.bin" },
		  file: { data: blobs.make("made in the job", { type: "text/markdown" }), name: "report.md" },
		} })
		return r.status`, `allow: ["network"]`)
	check(t, r.Status, r.Output, "done", `200`)
	sort.Strings(names)
	if strings.Join(fields, ",") != "title=the report" ||
		strings.Join(names, ",") != "file=report.md:text/markdown:made in the job,raw=abc.bin:application/octet-stream:abc" {
		t.Fatalf("the server received fields %q and files %q", fields, names)
	}
}
