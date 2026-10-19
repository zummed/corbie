package manual_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
	llmstandin "github.com/zummed/corbie/plugin/llm/standin"
	"github.com/zummed/corbie/plugin/mcp/standin"
)

// The world is the outside as the manual's examples take it to be: the
// hosts they connect to, the programs they start, the files they read and
// the secrets they use, each a stand-in of the tests' own. An example
// that reaches for anything else finds nothing there.
type world struct {
	dir string
	// srv stands in for /srv, where the examples keep files: an example
	// is run with that directory's name in place of /srv.
	srv string
	// tmp is where jobs' scratch directories are made, and where a job's
	// process finds what stands in for the network.
	tmp     string
	secrets map[string]string
	closers []func()
}

// outside is what a job's process is told of the world: where each host's
// stand-in listens, and the certificates its stand-ins show.
type outside struct {
	Hosts map[string]string `json:"hosts"` // "name:port" to a real address
	Roots string            `json:"roots"`
}

// The seed of the key the stand-in for build.example shows, so that the
// ssh page can give the key as a script would.
var buildHostSeed = []byte("corbie manual: build.example key")

// The tool a stand-in model asks for when it is offered one, with the
// arguments it gives; and what it says when it has nothing to ask.
var modelAsks = map[string]string{"shelf": `{"shelf": "top"}`, "title": `{"url": "https://example.com"}`}

const modelSays = "Hello."

func newWorld() (*world, error) {
	dir, err := os.MkdirTemp("", "corbie-manual-")
	if err != nil {
		return nil, err
	}
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		return nil, err
	}
	w := &world{dir: dir, srv: filepath.Join(dir, "srv"), tmp: filepath.Join(dir, "tmp"), secrets: map[string]string{
		"GITHUB_TOKEN":  "ghp_stand1nT0kenForTheManual",
		"FEED_TOKEN":    "feed-stand1n-t0ken",
		"ANTHROPIC_KEY": "sk-ant-stand1n-key",
		"MODELS_KEY":    "sk-models-stand1n-key",
		"TYPESAFE_KEY":  "ts-stand1n-key",
		"SIGNING":       "a-stand1n-signing-key",
		"DEPLOY_TOKEN":  "deploy-stand1n-t0ken",
		"QUEUE_TOKEN":   "queue-stand1n-t0ken",
		"APP_KEY":       "app-stand1n-key",
		"NOTES_KEY":     "notes-stand1n-key",
		"NOTES_TOKEN":   "notes-stand1n-t0ken",
		// A password with a quote and a backslash in it, which a body
		// written as JSON must escape.
		"PORTAL_PASSWORD": `a "quoted" pass\word`,
		// A key that encrypts is 32 bytes, and one that signs is a key
		// file's text: this one's public half is on the crypto page.
		"VAULT_KEY":   "vault-stand1n-key-of-32-letters!",
		"RELEASE_KEY": "-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEIGNvcmJpZSBtYW51YWw6IHRoZSByZWxlYXNlIGtleSEh\n-----END PRIVATE KEY-----\n",
	}}
	for name, text := range map[string]string{
		"srv/reports/latest.md":   "# Sales, third quarter\n\nTea is up and cake is down.\n",
		"srv/repo/README":         "A repository.\n",
		"srv/books/top/dune.txt":  "A beginning is a very delicate time.\n",
		"srv/books/top/emma.txt":  "Emma Woodhouse, handsome, clever, and rich.\n",
		"srv/books/lower/ulysses": "Stately, plump Buck Mulligan.\n",
		"tmp/.keep":               "",
		"home/.keep":              "",
		"work/.keep":              "",
		"bin/.keep":               "",
		"srv/app/.keep":           "",
		"srv/reports/2026/q3.csv": "item,amount\ntea,12\ncake,30\n",
		"srv/reports/2026/q2.csv": "item,amount\ntea,10\ncake,31\n",
		"srv/reports/chart.png":   "\x89PNG\r\n\x1a\n a chart",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return nil, err
		}
	}
	// Each program is this test binary under the program's name.
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	for name := range programs {
		if err := os.Symlink(self, filepath.Join(dir, "bin", name)); err != nil {
			return nil, err
		}
	}
	out := outside{Hosts: map[string]string{}}
	listen := func(host string) (net.Listener, error) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		w.closers = append(w.closers, func() { l.Close() })
		out.Hosts[host] = l.Addr().String()
		return l, nil
	}
	// The hosts that speak over an encrypted connection share one
	// certificate, signed by an authority of the world's own.
	web := map[string]http.HandlerFunc{
		"api.github.com":    w.github,
		"example.com":       w.news,
		"www.iana.org":      w.iana,
		"feed.example.com":  w.feed,
		"api.anthropic.com": (&llmstandin.Server{Key: w.secrets["ANTHROPIC_KEY"], Answer: anthropic}).ServeHTTP,
		"models.example":    w.chatModel,
		"api.typesafe.ai":   w.systemOneModel,
		"notes.example.com": (&standin.Server{Token: "notes-stand1n-t0ken", Only: notesTools}).ServeHTTP,
	}
	names := []string{"queue.example.com", "mail.example.com"}
	for name := range web {
		names = append(names, name)
	}
	sort.Strings(names)
	certificate, roots, err := certify(names)
	if err != nil {
		return nil, err
	}
	out.Roots = roots
	encrypted := &tls.Config{Certificates: []tls.Certificate{certificate}}
	for name, handler := range web {
		l, err := listen(name + ":443")
		if err != nil {
			return nil, err
		}
		server := &http.Server{Handler: handler}
		go server.Serve(tls.NewListener(l, encrypted))
		w.closers = append(w.closers, func() { server.Close() })
	}
	queue, err := listen("queue.example.com:6380")
	if err != nil {
		return nil, err
	}
	go accept(tls.NewListener(queue, encrypted), w.queue)
	mail, err := listen("mail.example.com:587")
	if err != nil {
		return nil, err
	}
	go accept(mail, func(conn net.Conn) { w.mail(conn, encrypted) })
	whois, err := listen("whois.iana.org:43")
	if err != nil {
		return nil, err
	}
	go accept(whois, w.whois)
	build, err := listen("build.example:22")
	if err != nil {
		return nil, err
	}
	config, err := w.buildMachine()
	if err != nil {
		return nil, err
	}
	go accept(build, func(conn net.Conn) { serveSSH(conn, config) })

	told, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(w.tmp, "outside.json"), told, 0o600); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *world) close() {
	for _, do := range w.closers {
		do()
	}
	os.RemoveAll(w.dir)
}

// lines gives the lines a job's process noted in one of the world's
// files, in the order they were noted.
func (w *world) lines(name string) []string {
	data, _ := os.ReadFile(filepath.Join(w.tmp, name))
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func accept(l net.Listener, serve func(net.Conn)) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go serve(conn)
	}
}

// certify makes an authority and one certificate for the names, signed by
// it. It gives the certificate and the authority's, as PEM.
func certify(names []string) (tls.Certificate, string, error) {
	var none tls.Certificate
	authorityKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return none, "", err
	}
	authority := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "the manual's examples"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	authorityDER, err := x509.CreateCertificate(rand.Reader, authority, authority, &authorityKey.PublicKey, authorityKey)
	if err != nil {
		return none, "", err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return none, "", err
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, authority, &key.PublicKey, authorityKey)
	if err != nil {
		return none, "", err
	}
	roots := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: authorityDER})
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, string(roots), nil
}

// github answers as api.github.com does to the one who holds the token.
func (w *world) github(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	_, password, _ := r.BasicAuth()
	switch {
	case r.Header.Get("Authorization") != "Bearer "+w.secrets["GITHUB_TOKEN"] && password != w.secrets["GITHUB_TOKEN"]:
		rw.WriteHeader(401)
		io.WriteString(rw, `{"message":"Bad credentials"}`)
	case r.URL.Path == "/user":
		io.WriteString(rw, `{"login":"octocat","id":1}`)
	default:
		rw.WriteHeader(404)
		io.WriteString(rw, `{"message":"Not Found"}`)
	}
}

// iana is www.iana.org: the one page about example domains, as far as its
// title.
func (w *world) iana(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/help/example-domains" {
		http.NotFound(rw, r)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(rw, "<!doctype html>\n<html><head><title>Example Domains</title></head>\n<body><h1>Example Domains</h1></body></html>\n")
}

// news is example.com: a page of news, orders that are taken, and a
// sign-in written as JSON.
func (w *world) news(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/login" {
		// Taken only if the body can be read as JSON and holds the password
		// itself.
		var signIn struct{ User, Password string }
		switch {
		case json.NewDecoder(r.Body).Decode(&signIn) != nil:
			rw.WriteHeader(400)
		case signIn.Password != w.secrets["PORTAL_PASSWORD"]:
			rw.WriteHeader(401)
		}
		return
	}
	if strings.HasPrefix(r.URL.Path, "/orders") {
		switch r.Method {
		case "POST":
			var order map[string]any
			json.NewDecoder(r.Body).Decode(&order)
			order["id"] = 1
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(201)
			json.NewEncoder(rw).Encode(order)
		case "DELETE", "OPTIONS":
			rw.WriteHeader(204)
		}
		return
	}
	if r.URL.Path == "/" {
		// The page the real example.com gives, as far as its title.
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(rw, "<!doctype html>\n<html><head><title>Example Domain</title></head>\n<body><h1>Example Domain</h1></body></html>\n")
		return
	}
	if r.URL.Path != "/news" {
		http.NotFound(rw, r)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(rw, `<!doctype html>
<html><head><title>News</title></head>
<body>
<h1>News</h1>
<p><a href="/news/tea"> Tea is up </a></p>
<p><a href="/news/cake">Cake is down</a></p>
<p><a name="end">The end</a></p>
</body></html>
`)
}

// feed is a websocket that sends prices to one who subscribes to them.
func (w *world) feed(rw http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+w.secrets["FEED_TOKEN"] {
		http.Error(rw, "no token", 401)
		return
	}
	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	for {
		_, message, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var asked struct{ Subscribe string }
		if json.Unmarshal(message, &asked) == nil && asked.Subscribe == "prices" {
			conn.Write(r.Context(), websocket.MessageText, []byte(`{"prices":{"tea":4.2,"cake":3}}`))
		}
	}
}

// whois answers one question, a line, with a few lines, and closes.
func (w *world) whois(conn net.Conn) {
	defer conn.Close()
	asked, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	fmt.Fprintf(conn, "%% IANA WHOIS server\r\ndomain:       %s\r\nstatus:       ACTIVE\r\n", strings.ToUpper(strings.TrimSpace(asked)))
}

// queue takes a line that gives its token, and says whether it was right.
func (w *world) queue(conn net.Conn) {
	defer conn.Close()
	asked, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	if strings.TrimSpace(asked) == "AUTH "+w.secrets["QUEUE_TOKEN"] {
		io.WriteString(conn, "+OK\r\n")
	} else {
		io.WriteString(conn, "-ERR invalid password\r\n")
	}
}

// mail starts plain and, asked to, carries on encrypted.
func (w *world) mail(conn net.Conn, encrypted *tls.Config) {
	defer conn.Close()
	// A line is read a byte at a time: what follows it is the other
	// side's first encrypted bytes, which are not this reader's.
	line := func(from net.Conn) string {
		var read []byte
		for one := make([]byte, 1); !strings.HasSuffix(string(read), "\n"); read = append(read, one[0]) {
			if _, err := from.Read(one); err != nil {
				return ""
			}
		}
		return strings.TrimSpace(string(read))
	}
	io.WriteString(conn, "220 mail.example.com ready\r\n")
	if line(conn) != "STARTTLS" {
		return
	}
	io.WriteString(conn, "220 go ahead\r\n")
	secure := tls.Server(conn, encrypted)
	if line(secure) == "NOOP" {
		io.WriteString(secure, "250 OK\r\n")
	}
}

// buildMachine is the machine build.example: the key it shows, and the
// one user it takes, by the key the secret BUILD_KEY holds.
func (w *world) buildMachine() (*ssh.ServerConfig, error) {
	host, err := ssh.NewSignerFromKey(ed25519.NewKeyFromSeed(buildHostSeed))
	if err != nil {
		return nil, err
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		return nil, err
	}
	w.secrets["BUILD_KEY"] = string(pem.EncodeToMemory(block))
	taken, err := ssh.NewPublicKey(public)
	if err != nil {
		return nil, err
	}
	config := &ssh.ServerConfig{PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if c.User() != "deploy" || string(key.Marshal()) != string(taken.Marshal()) {
			return nil, fmt.Errorf("not taken")
		}
		return nil, nil
	}}
	config.AddHostKey(host)
	return config, nil
}

func serveSSH(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()
	_, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)
	for asked := range channels {
		if asked.ChannelType() != "session" {
			asked.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		channel, requests, err := asked.Accept()
		if err != nil {
			return
		}
		go func() {
			defer channel.Close()
			for request := range requests {
				if request.Type != "exec" {
					request.Reply(false, nil)
					continue
				}
				var exec struct{ Command string }
				ssh.Unmarshal(request.Payload, &exec)
				request.Reply(true, nil)
				var status uint32
				if exec.Command == "uptime" {
					io.WriteString(channel, " 12:00:00 up 3 days,  2 users,  load average: 0.10, 0.12, 0.09\n")
				} else {
					io.WriteString(channel.Stderr(), exec.Command+": not found\n")
					status = 127
				}
				channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
				return
			}
		}()
	}
}

// anthropic is a model that speaks Anthropic's way. Offered a tool it
// knows and not yet shown a result, it asks for the tool; shown results,
// it says what they were; shown a picture, it says its name; told to
// answer as data with a greeting, it does; otherwise it says hello.
func anthropic(c llmstandin.Call) llmstandin.Reply {
	var results []string
	picture := ""
	for _, message := range c.Messages {
		content, _ := message.(map[string]any)
		blocks, _ := content["content"].([]any)
		for i, block := range blocks {
			switch b, _ := block.(map[string]any); b["type"] {
			case "tool_result":
				results = append(results, fmt.Sprint(b["content"]))
			case "image":
				// A picture follows a line that gives its name.
				named, _ := blocks[i-1].(map[string]any)
				picture = strings.TrimSuffix(fmt.Sprint(named["text"]), ":")
			}
		}
	}
	switch {
	case len(results) > 0:
		return llmstandin.Reply{Text: "It holds " + strings.Join(results, " and ") + "."}
	case picture != "":
		return llmstandin.Reply{Text: "A picture, named " + picture + "."}
	case strings.Contains(c.System, `as JSON only, fitting this JSON Schema: {"properties":{"greeting":{"type":"string"}},"type":"object"}`):
		return llmstandin.Reply{Text: fmt.Sprintf(`{"greeting": %q}`, modelSays)}
	}
	for _, tool := range c.Tools {
		name, _ := tool.(map[string]any)["name"].(string)
		if args, known := modelAsks[name]; known {
			return llmstandin.Reply{Text: modelSays, Calls: []llmstandin.ToolCall{{ID: "call_1", Name: name, Args: args}}}
		}
	}
	return llmstandin.Reply{Text: modelSays}
}

// systemOneModel is a model that speaks the System One format. It says a
// statement is true, and picks the option named technical where there is
// one and the first otherwise.
func (w *world) systemOneModel(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer "+w.secrets["TYPESAFE_KEY"] {
		http.Error(rw, `{"error":{"message":"invalid api key"}}`, 401)
		return
	}
	var sent struct {
		Questions map[string]struct {
			Type     string         `json:"type"`
			Criteria map[string]any `json:"criteria"`
		} `json:"questions"`
	}
	body, _ := io.ReadAll(r.Body)
	json.Unmarshal(body, &sent)
	answers := map[string]any{}
	for name, q := range sent.Questions {
		switch q.Type {
		case "noul":
			answers[name] = map[string]any{"type": "noul", "noul": 0.99}
		case "choice":
			options := make([]string, 0, len(q.Criteria))
			for option := range q.Criteria {
				options = append(options, option)
			}
			sort.Strings(options)
			picked := options[0]
			if slices.Contains(options, "technical") {
				picked = "technical"
			}
			probabilities := map[string]any{}
			for _, option := range options {
				probabilities[option] = 0.0
			}
			probabilities[picked] = 1.0
			answers[name] = map[string]any{"type": "choice", "choice": picked, "probabilities": probabilities, "confidence": 1.0}
		}
	}
	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]any{"model": "jev-1.13.0", "answers": answers,
		"usage": map[string]any{"input_tokens": 174, "output_tokens": 2}})
}

// chatModel is a model that speaks OpenAI's Chat Completions way, and
// says hello.
func (w *world) chatModel(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+w.secrets["MODELS_KEY"] {
		http.Error(rw, `{"error":{"message":"Incorrect API key provided"}}`, 401)
		return
	}
	rw.Header().Set("Content-Type", "text/event-stream")
	for _, data := range []string{
		fmt.Sprintf(`{"choices":[{"index":0,"delta":{"role":"assistant","content":%q},"finish_reason":null}]}`, modelSays),
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		`[DONE]`,
	} {
		fmt.Fprintf(rw, "data: %s\n\n", data)
	}
}

// The programs of the world. Each is this test binary, started under the
// program's name, and gives the status it ends with.
// notesTools are the tools of the MCP servers of notes.
var notesTools = []string{"add_note", "find_notes"}

var programs = map[string]func(args []string) int{
	"git": func(args []string) int {
		if len(args) == 0 || args[0] != "log" {
			fmt.Fprintln(os.Stderr, "git: the stand-in knows only log")
			return 1
		}
		fmt.Print("3f2a1c9 Count the cake\n9d0e7b2 Add the third quarter\n51c8a04 Tea, by region\n0b7f3de Tidy the headings\ne4a9c16 First reports\n")
		return 0
	},
	"pg_dump": func(args []string) int {
		fmt.Printf("-- PostgreSQL database dump of %s\nCREATE TABLE orders (id integer, item text);\n", strings.Join(args, " "))
		return 0
	},
	"pdftotext": func(args []string) int {
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: pdftotext <in.pdf> <out.txt>")
			return 99
		}
		document, err := os.ReadFile(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		text := strings.TrimPrefix(string(document), "%PDF ")
		if err := os.WriteFile(args[1], []byte(text), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	},
	// notes-server is an MCP server of notes, which needs its key.
	"notes-server": func(args []string) int {
		if os.Getenv("NOTES_KEY") == "" {
			fmt.Fprintln(os.Stderr, "notes-server: NOTES_KEY is not set")
			return 1
		}
		(&standin.Server{Only: notesTools}).Serve(os.Stdin, os.Stdout)
		return 0
	},
	"deploy": func(args []string) int {
		fmt.Printf("deployed %s\n", strings.Join(args, " "))
		return 0
	},
	// claude prints as the claude program does when it is asked for one
	// reply in a given shape: the reply a piece at a time, then the result.
	"claude": func(args []string) int {
		io.Copy(io.Discard, os.Stdin)
		answer := fmt.Sprintf(`{"text":%q,"tool_calls":[]}`, modelSays)
		piece, _ := json.Marshal(answer)
		fmt.Println(`{"type":"system","subtype":"init"}`)
		fmt.Println(`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","name":"StructuredOutput"}}}`)
		fmt.Printf(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%s}}}`+"\n", piece)
		fmt.Printf(`{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":%s,"usage":{"input_tokens":10,"output_tokens":5}}`+"\n", answer)
		return 0
	},
}

// inWorld is run by a job's process before the job. It puts the world's
// stand-ins in place of the network, so that nothing a script does goes
// further than this machine, and has every call of every plugin note the
// kinds of permission it asked Corbie for.
func inWorld() {
	// A job runs in its scratch directory, which is made where the world
	// says; what the job is told of the world lies beside it.
	scratch, _ := os.Getwd()
	tmp := filepath.Dir(scratch)
	note := func(file, line string) {
		f, err := os.OpenFile(filepath.Join(tmp, file), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		fmt.Fprintln(f, line)
		f.Close()
	}
	var out outside
	told, _ := os.ReadFile(filepath.Join(tmp, "outside.json"))
	json.Unmarshal(told, &out)
	var names []string
	for host := range out.Hosts {
		name, _, _ := net.SplitHostPort(host)
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	// Each name the world has is given an address no machine has, and a
	// connection to that address goes to the name's stand-in.
	plugin.StandIn(func(ctx context.Context, host string) ([]string, error) {
		at := slices.Index(names, host)
		if at < 0 {
			note("reached", "nothing "+host)
			return nil, fmt.Errorf("the examples' world has no host of that name")
		}
		return []string{"192.0.2." + strconv.Itoa(at+1)}, nil
	}, func(ctx context.Context, address string) (net.Conn, error) {
		host, port, _ := net.SplitHostPort(address)
		at, err := strconv.Atoi(strings.TrimPrefix(host, "192.0.2."))
		if err != nil || !strings.HasPrefix(host, "192.0.2.") || at < 1 || at > len(names) {
			note("reached", "nothing "+address)
			return nil, fmt.Errorf("the examples' world has nothing at %s", address)
		}
		standIn, ok := out.Hosts[net.JoinHostPort(names[at-1], port)]
		if !ok {
			note("reached", "nothing "+net.JoinHostPort(names[at-1], port))
			return nil, fmt.Errorf("the examples' world has nothing at %s", net.JoinHostPort(names[at-1], port))
		}
		note("reached", "stand-in "+net.JoinHostPort(names[at-1], port))
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", standIn)
	})
	// The world's own authority is the only one a job trusts.
	os.Setenv("GODEBUG", "x509usefallbackroots=1")
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM([]byte(out.Roots))
	x509.SetFallbackRoots(roots)

	for _, module := range plugin.Names() {
		m, _ := plugin.Find(module)
		for name, call := range m.Calls {
			noting(module, name, call, func(line string) { note("asked", line) })
		}
	}
}

// noting has a call note each kind of permission it asks Corbie for and
// is given. The call is made with a list of its own for what it used, and
// what that list holds afterwards is what this call asked for; it is then
// noted for the job as it would have been.
func noting(module, name string, call *plugin.Call, note func(string)) {
	do := call.Do
	call.Do = func(job *plugin.Job, first any, options plugin.Options) (any, error) {
		whole := job.Perms
		job.Perms = &permission.Set{Layers: whole.Layers, Scratch: whole.Scratch}
		result, err := do(job, first, options)
		asked := job.Perms.Used()
		job.Perms = whole
		if err != nil {
			return nil, err
		}
		note(module + " " + name + " made")
		for _, entry := range asked {
			// An entry with no scope is a whole group, which is no kind.
			kind, value, scoped := strings.Cut(entry, ":")
			if !scoped {
				whole.CheckGroup(entry)
				continue
			}
			whole.Check(kind, value)
			note(module + " " + name + " " + kind)
		}
		// What stays open asks in the name of the plugin that opened it.
		if open, ok := result.(*plugin.Object); ok {
			for inner, c := range open.Calls {
				note(module + " " + name + "." + inner + " has")
				noting(module, name+"."+inner, c, note)
			}
		}
		return result, nil
	}
}
