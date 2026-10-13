package client_test

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
	"github.com/zummed/corbie/secret"
)

// A stand-in plugin, built on the public package as an extension is. Its
// one call prints what it is given and ends the job's process there and
// then, as a plugin that went badly wrong would.
func init() {
	plugin.Register(&plugin.Module{Name: "dying", Calls: map[string]*plugin.Call{
		"now": {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			text, err := job.Reveal(first, "exec:dying")
			if err != nil {
				return nil, err
			}
			os.Stderr.WriteString(text)
			os.Exit(7)
			return nil, nil
		}},
	}})
}

// withSecrets runs one script as a job that is allowed what is listed and
// was started with the secrets given, and gives the run with what it
// logged.
func withSecrets(t *testing.T, l *client.Local, source string, secrets map[string]string, allow ...string) (*client.Run, []string) {
	t.Helper()
	list := make([]string, len(allow))
	for i, entry := range allow {
		list[i] = fmt.Sprintf("%q", entry)
	}
	id, err := l.Start(ctx, draft(t, `
		flow: one
		stages:
		  it: { job: { script: ./it.js, timeout: 20s, allow: [`+strings.Join(list, ", ")+`] } }
		output: ${stages.it.output}
	`, map[string]string{"it.js": source}), client.Start{Secrets: secrets})
	if err != nil {
		t.Fatal(err)
	}
	var logged []string
	r, err := l.Follow(ctx, id, 20*time.Millisecond, func(e client.Entry) {
		if e.Name == "log" {
			logged = append(logged, fmt.Sprint(e.Data))
		}
	})
	if err != nil {
		t.Fatalf("following %s: %v", id, err)
	}
	return r, logged
}

// holds says which of the forms a value leaves in a text holds.
func holds(text, value string) string {
	for what, form := range map[string]string{
		"the value":           value,
		"its second half":     value[len(value)/2:],
		"the value as base64": base64.StdEncoding.EncodeToString([]byte(value)),
		"the value escaped":   url.QueryEscape(value),
	} {
		if strings.Contains(text, form) {
			return what
		}
	}
	return ""
}

func Test_sec_mask_errors_and_refusals(t *testing.T) {
	const value = "tok-en-1234567890abcdef"
	secrets := map[string]string{"TOKEN": value}
	said := func(r *client.Run) string { return strings.Join([]string{r.Error, r.Refused, r.Fix}, " | ") }
	l := local(t, "all")

	// An error from the other side that quotes what it was sent.
	closes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_, sent, _ := conn.Read(r.Context())
		conn.Close(websocket.StatusPolicyViolation, "no such key: "+string(sent))
	}))
	defer closes.Close()
	host := strings.TrimPrefix(closes.URL, "http://")
	r, _ := withSecrets(t, l, `
		const ws = websocket.connect("ws://`+host+`/")
		ws.send(secrets.get("TOKEN"))
		ws.receive()
	`, secrets, "net:"+host, "secret:TOKEN@*")
	if r.Status != "failed" || r.Failure != "error" || !strings.Contains(r.Error, "no such key: [secret TOKEN]") || holds(said(r), value) != "" {
		t.Errorf("an error that quotes the secret left the job as: %s/%s %s", r.Status, r.Failure, said(r))
	}
	// A script that catches the error sees the name too. Whatever leaves a
	// job is searched again on its way out, so the script says here what it
	// saw.
	r, _ = withSecrets(t, l, `
		const ws = websocket.connect("ws://`+host+`/")
		ws.send(secrets.get("TOKEN"))
		try { ws.receive() } catch (e) { return e.message.includes("no such key: [secret TOKEN]") && !e.message.includes("tok-en") }
	`, secrets, "net:"+host, "secret:TOKEN@*")
	if r.Status != "done" || r.Output != true {
		t.Errorf("a script that caught an error quoting the secret saw the value: %s %v %s", r.Status, r.Output, r.Error)
	}

	// A refusal of something the other side chose: a server that was sent
	// the secret redirects to a host named after it. The server is allowed
	// by its name, so no list allows an address and the name it redirects
	// to is refused without being looked up.
	redirects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Header.Get("X-Key")+".example/", http.StatusFound)
	}))
	defer redirects.Close()
	host = strings.Replace(strings.TrimPrefix(redirects.URL, "http://"), "127.0.0.1", "localhost", 1)
	r, _ = withSecrets(t, l, `
		http.get("http://`+host+`", { headers: { "X-Key": secrets.get("TOKEN") } })
	`, secrets, "net:"+host, "secret:TOKEN@*")
	if r.Status != "failed" || r.Failure != "denied" || r.Refused != "net:[secret TOKEN].example:80" ||
		!strings.Contains(r.Fix, "net:[secret TOKEN].example:80") || holds(said(r), value) != "" {
		t.Errorf("a refusal that names the secret left the job as: %s/%s %s", r.Status, r.Failure, said(r))
	}

	// What a job's process printed before it died: in each form the value
	// leaves in, and where a cut of what is kept would fall inside it.
	for what, printed := range map[string]string{
		"the value in its forms":              `secrets.text` + "`died holding ${t} and ${secrets.encoded(t)} and ${secrets.encoded(t, { as: \"url\" })}`",
		"the value where what is kept begins": `secrets.text` + "`${\"x\".repeat(9000)}${t}${\"y\".repeat(8192 - 14)}`",
	} {
		r, _ = withSecrets(t, l, `
			const t = secrets.get("TOKEN")
			dying.now(`+printed+`)
		`, secrets, "secret:TOKEN@*")
		if r.Status != "failed" || !strings.Contains(r.Error, "ended without a result") || holds(said(r), value) != "" {
			t.Errorf("a process that printed %s and died left: %s/%s holding %s", what, r.Status, r.Failure, holds(said(r), value))
		}
		if what == "the value in its forms" && strings.Count(r.Error, "[secret TOKEN]") != 3 {
			t.Errorf("a process that printed %s and died left: %s", what, r.Error)
		}
	}
}

func Test_sec_mask_untyped_body_as_text(t *testing.T) {
	const value = "tok-en-1234567890abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No type is stated, and none is worked out from the body.
		w.Header()["Content-Type"] = nil
		if r.URL.Path == "/bytes" {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		io.WriteString(w, "you sent "+r.Header.Get("Authorization"))
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	l := local(t, "all")
	r, _ := withSecrets(t, l, `
		const headers = { Authorization: secrets.text`+"`Bearer ${secrets.get(\"TOKEN\")}`"+` }
		const none = http.get("`+server.URL+`/", { headers })
		const bytes = http.get("`+server.URL+`/bytes", { headers })
		// Whatever leaves a job is searched again on its way out, so the
		// script says here what it was handed.
		const want = "you sent Bearer [secret TOKEN]"
		return { none: none.body === want, stated: none.headers["content-type"] || "no type", bytes: bytes.body === want,
		         sizes: [none.body.length, bytes.body.length] }
	`, map[string]string{"TOKEN": value}, "net:"+host, "secret:TOKEN@*")
	if r.Status != "done" {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	out := r.Output.(map[string]any)
	if out["stated"] != "no type" || out["none"] != true {
		t.Errorf("a body with no stated type was not searched before the script had it: %v", out)
	}
	// Nor does a type that says bytes keep a body read as text from being
	// searched.
	if out["bytes"] != true {
		t.Errorf("a body stated to be bytes, read as text, was not searched before the script had it: %v", out)
	}
}

func Test_sec_nothing_prints_a_value(t *testing.T) {
	const value = "tok-en-1234567890abcdef"
	l := local(t, "all")
	r, logged := withSecrets(t, l, `
		const t = secrets.get("TOKEN")
		const joined = secrets.text`+"`Bearer ${t}`"+`
		log(t)
		log("joined:", joined, "and encoded:", secrets.encoded(joined))
		log({ inside: t, list: [joined, 1] })
		// What each failure says of a secret is its name.
		const ways = [() => String(t), () => t + "", () => JSON.stringify({ t }), () => emit("out", t),
			() => crypto.hash(t), () => fs.write("out.json", { json: { t } }), () => secrets.encoded(t, { as: "rot13" }),
			() => secrets.get("ABSENT"), () => http.get("http://unbound.example/", { headers: { A: joined } }).status]
		for (const way of ways) {
			try { way(); log("nothing was thrown") } catch (e) { log("thrown: " + (e && e.message || e)) }
		}
	`, map[string]string{"TOKEN": value}, "disk", "secret:TOKEN@net:api.example")
	// The last of the ways is refused, which ends the job: its refusal is
	// what the run failed with.
	text := strings.Join(logged, "\n") + "\n" + r.Error + " " + r.Refused + " " + r.Fix
	if r.Status != "failed" || r.Failure != "denied" || r.Refused != "secret:TOKEN@net.unencrypted:unbound.example:80" {
		t.Fatalf("the run ended %s/%s: %s", r.Status, r.Failure, r.Error)
	}
	if got := holds(text, value); got != "" || strings.Contains(text, "tok-en") {
		t.Fatalf("what was logged and said holds %s:\n%s", got, text)
	}
	// A handle is shown by the names of its secrets, wherever it is.
	want := []string{"[secret TOKEN]", "joined: [secret TOKEN] and encoded: [secret TOKEN]", "map[inside:[secret TOKEN] list:[[secret TOKEN] 1]]"}
	if len(logged) != len(want)+8 {
		t.Fatalf("the script logged %q", logged)
	}
	for i, line := range logged {
		switch {
		case i < len(want) && line != want[i]:
			t.Errorf("log line %d is %q, want %q", i+1, line, want[i])
		case i >= len(want) && !strings.HasPrefix(line, "thrown: "):
			t.Errorf("log line %d is %q", i+1, line)
		}
	}
	// And outside a script: however a handle is printed, it shows names.
	handle, _ := plugin.Join("Bearer ", plugin.NewSecret("TOKEN"), " for ", plugin.NewSecret("USER"))
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
		if got := fmt.Sprintf(verb, handle); !strings.Contains(got, "TOKEN") && !strings.Contains(got, fmt.Sprintf("%x", "TOKEN")) {
			t.Errorf("printed with %s a handle shows %q", verb, got)
		}
	}
	if got := fmt.Sprint(handle); got != "[secret TOKEN USER]" {
		t.Errorf("a handle prints as %q", got)
	}
}

func Test_sec_name_splits_one_way(t *testing.T) {
	l := local(t, "all")
	const def = `
		flow: one
		stages:
		  it: { job: { script: ./it.js, allow: [%s] } }
	`
	files := map[string]string{"it.js": "return 1"}
	for _, name := range []string{"GITHUB_TOKEN", "deploy-key.2", "k"} {
		if err := secret.CheckName(name); err != nil {
			t.Errorf("the name %q: %v", name, err)
		}
		if _, err := machineParse(fmt.Sprintf(def, fmt.Sprintf("%q", "secret:"+name+"@net:api.example")), files); err != nil {
			t.Errorf("an entry for the name %q: %v", name, err)
		}
	}
	// A name is not empty and holds no @, no : and no blank, wherever one
	// is written: for a controller secret, in an entry, with a start.
	for _, name := range []string{"", "A@B", "A:B", "A B", " A", "A ", "A\tB", "A\nB", "A\rB", "A\u00a0B", "A\x00B"} {
		if err := secret.CheckName(name); err == nil {
			t.Errorf("%q was taken as a secret's name", name)
		}
		if err := secret.Set(ctx, l.Memory.As(ownerOf()), l.Controller.ID, name, "value"); err == nil {
			t.Errorf("a controller secret was set under the name %q", name)
		}
		if err := permission.CheckWritten("secret:" + name + "@*"); err == nil {
			t.Errorf("the entry %q was taken", "secret:"+name+"@*")
		}
		if _, err := machineParse(fmt.Sprintf(def, fmt.Sprintf("%q", "secret:"+name+"@net:api.example")), files); err == nil {
			t.Errorf("a definition with an entry for the name %q passed its check", name)
		}
		id, err := l.Start(ctx, draft(t, fmt.Sprintf(def, ""), files), client.Start{Secrets: map[string]string{name: "value"}})
		if err == nil {
			t.Errorf("a run was started with a secret named %q: %s", name, id)
		}
	}
	if names, _ := secret.List(ctx, l.Memory.As(ownerOf()), l.Controller.ID.Name); len(names) != 0 {
		t.Errorf("the controller holds secrets named %q", names)
	}
	// So an entry is read in one way: the name ends at the first @, and
	// what follows is the destination, whatever it holds.
	const entry = "secret:K@exec:deploy@v2"
	if err := permission.CheckWritten(entry); err != nil {
		t.Fatal(err)
	}
	if names := secret.Named([]string{entry, "secret:K@B@*"}); len(names) != 1 || names[0] != "K" {
		t.Errorf("the entries name %q", names)
	}
	set := &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{entry}}}}
	if set.Check(permission.Secret, "K@exec:deploy@v2") != nil {
		t.Error("the secret may not go to the program the entry names")
	}
	for _, to := range []string{"K@v2", "K@exec:deploy", "K@exec:v2", "K@exec:deploy@v2@v2"} {
		if set.Check(permission.Secret, to) == nil {
			t.Errorf("the entry %s was read as allowing %s", entry, to)
		}
	}
}
