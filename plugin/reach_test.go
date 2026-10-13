package plugin_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// What a plugin that comes with Corbie may be built from. A package that
// cannot reach outside the process is used freely. One that can is used
// only by the names listed for it, none of which opens anything: the
// opening is done by the Job. Of Corbie itself a plugin uses the public
// package and no other part, as an extension does.
var (
	// inside are the packages with no way out of the process.
	inside = []string{
		"archive/tar", "archive/zip", "bufio", "bytes", "compress/gzip", "context",
		"crypto", "crypto/aes", "crypto/cipher", "crypto/ecdsa", "crypto/ed25519", "crypto/hmac", "crypto/md5", "crypto/rand",
		"crypto/rsa", "crypto/sha1", "crypto/sha256", "crypto/sha512",
		"encoding/base64", "encoding/csv", "encoding/hex", "encoding/json/jsontext", "encoding/json/v2", "encoding/pem", "encoding/xml",
		"errors", "fmt", "hash", "io", "io/fs", "net/url", "regexp", "slices", "sort", "strconv", "strings", "time", "unicode",
		"golang.org/x/net/html", "golang.org/x/net/html/charset",
		"github.com/zummed/corbie/plugin", "github.com/zummed/corbie/plugin/llm",
	}
	// outward are the packages that can reach outside, with the names of
	// each that a plugin may use.
	outward = map[string][]string{
		// A connection is a value the Job hands over, and an address is
		// text to take apart.
		"net": {"Conn", "Addr", "SplitHostPort", "JoinHostPort"},
		// A program is started by the Job; a plugin reads how it ended.
		"os/exec": {"ExitError"},
		"os":      {"ErrDeadlineExceeded"},
		// Encryption is put over a connection the Job opened.
		"crypto/tls": {"Config", "Client"},
		// A key is read from text a script gave. The certificates the
		// machine trusts are in its files, and are not a plugin's to read.
		"crypto/x509": {"ParsePKCS8PrivateKey", "ParsePKCS1PrivateKey", "ParseECPrivateKey", "ParsePKIXPublicKey"},
		// A web client is built over the Job's way to connect; see client.
		"net/http": {"Client", "Transport", "Request", "Header", "NewRequestWithContext", "ErrUseLastResponse"},
		// The libraries wrapped, each handed a connection or a client.
		"github.com/coder/websocket": {"Dial", "DialOptions", "Conn", "MessageType", "MessageText", "MessageBinary",
			"CloseStatus", "StatusNormalClosure", "StatusGoingAway"},
		"golang.org/x/crypto/ssh": {"NewClientConn", "NewClient", "ClientConfig", "AuthMethod", "Password", "KeyboardInteractive",
			"PublicKeys", "ParsePrivateKey", "ParseAuthorizedKey", "HostKeyCallback", "InsecureIgnoreHostKey", "PublicKey",
			"FingerprintSHA256", "ExitError", "KeyAlgoRSA", "KeyAlgoRSASHA256", "KeyAlgoRSASHA512"},
	}
	// files are the names only the plugin for files uses. Its work is
	// files, and each of its calls that touches one asks the Job where.
	files = map[string][]string{
		"os": {"Stat", "Lstat", "ReadFile", "ReadDir", "OpenFile", "MkdirAll", "Remove", "RemoveAll", "Rename",
			"O_WRONLY", "O_CREATE", "O_TRUNC", "O_APPEND"},
		"path/filepath": {"WalkDir", "Join", "Dir", "Base", "ToSlash"},
	}
	// asked are the Job's calls that give a path.
	asked = []string{"Path", "Entry"}
	// kept are the parts of the Job that are Corbie's to use and not a
	// plugin's: the layers every check is made against, the secrets'
	// values, and who is told the forms a value leaves in.
	kept = []string{"Perms", "Secrets", "Noted"}
)

// reach reads the source a plugin is built from, in one directory, and lists
// each place it uses something that is not on the lists above.
func reach(t *testing.T, dir string, forFiles bool) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		say := func(at token.Pos, format string, args ...any) {
			found = append(found, fmt.Sprintf("%s: %s", set.Position(at), fmt.Sprintf(format, args...)))
		}
		// The packages the file uses, by the name it knows each by.
		packages := map[string]string{}
		for _, imported := range file.Imports {
			path, _ := strconv.Unquote(imported.Path.Value)
			name := path[strings.LastIndex(path, "/")+1:]
			if path == "encoding/json/v2" {
				name = "json"
			}
			if imported.Name != nil {
				name = imported.Name.Name
			}
			_, reaches := outward[path]
			_, forFile := files[path]
			switch {
			case name == "." || name == "_":
				say(imported.Pos(), "%s is brought in without a name, so what is used of it cannot be told", path)
			case !slices.Contains(inside, path) && !reaches && !(forFiles && forFile):
				say(imported.Pos(), "%s is not a package a plugin is built from", path)
			}
			packages[name] = path
		}
		used := func(n ast.Node) (path, name string, ok bool) {
			sel, isSel := n.(*ast.SelectorExpr)
			if !isSel {
				return "", "", false
			}
			x, isIdent := sel.X.(*ast.Ident)
			if !isIdent || x.Obj != nil {
				return "", "", false
			}
			path, ok = packages[x.Name]
			return path, sel.Sel.Name, ok
		}
		// A web client is built only whole, with its way to connect given:
		// built any other way it connects by itself and reads the
		// machine's settings.
		whole := map[ast.Node]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			path, name, ok := used(lit.Type)
			if !ok {
				return true
			}
			keys := map[string]ast.Expr{}
			for _, element := range lit.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					if key, ok := pair.Key.(*ast.Ident); ok {
						keys[key.Name] = pair.Value
					}
				}
			}
			switch path + "." + name {
			case "net/http.Client":
				whole[lit.Type] = true
				if keys["Transport"] == nil {
					say(lit.Pos(), "a web client with no transport of its own connects by itself")
				}
			case "net/http.Transport":
				whole[lit.Type] = true
				if !callsJob(keys["DialContext"], []string{"Dial"}) {
					say(lit.Pos(), "a transport that does not connect through the Job's Dial connects by itself")
				}
				for _, key := range []string{"Dial", "DialTLS", "DialTLSContext"} {
					if keys[key] != nil {
						say(lit.Pos(), "a transport with %s connects by itself", key)
					}
				}
				if proxy, ok := keys["Proxy"].(*ast.Ident); keys["Proxy"] != nil && (!ok || proxy.Name != "nil") {
					say(lit.Pos(), "a transport with a proxy reads the machine's settings")
				}
			case "github.com/coder/websocket.DialOptions":
				whole[lit.Type] = true
				if keys["HTTPClient"] == nil {
					say(lit.Pos(), "a websocket opened with no web client of its own connects by itself")
				}
			}
			return true
		})
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && slices.Contains(kept, sel.Sel.Name) {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "job" {
					say(n.Pos(), "the Job's %s is Corbie's to use: a plugin asks the Job, and makes no check and takes no value by itself", sel.Sel.Name)
				}
			}
			path, name, ok := used(n)
			if !ok {
				return true
			}
			switch {
			case slices.Contains(inside, path), slices.Contains(outward[path], name), forFiles && slices.Contains(files[path], name):
			default:
				say(n.Pos(), "%s.%s is the operating system's own means, a library's, or a part of Corbie outside the public package, and not something the Job hands over", path, name)
			}
			if slices.Contains([]string{"net/http.Client", "net/http.Transport", "github.com/coder/websocket.DialOptions"}, path+"."+name) && !whole[n] {
				say(n.Pos(), "%s.%s is made here some way other than whole, with its way to connect given", path, name)
			}
			return true
		})
		if !forFiles {
			continue
		}
		// In the plugin for files, a call that touches the disk asks the
		// Job where, itself or by the one helper that does.
		helpers := []string{}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil && callsJob(fn.Body, asked) {
				helpers = append(helpers, fn.Name.Name)
			}
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			touches, asks := token.NoPos, false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if path, name, ok := used(n); ok && path == "os" && slices.Contains(files[path], name) && !strings.HasPrefix(name, "O_") {
					touches = n.Pos()
				}
				if call, ok := n.(*ast.CallExpr); ok {
					if helper, ok := call.Fun.(*ast.Ident); ok && slices.Contains(helpers, helper.Name) {
						asks = true
					}
				}
				return true
			})
			if touches.IsValid() && !asks && !callsJob(fn.Body, asked) {
				say(touches, "%s touches the disk and does not ask the Job for the path", fn.Name.Name)
			}
		}
	}
	return found
}

// callsJob says whether a piece of source calls one of the Job's named
// calls, on the value every plugin's call is handed as job.
func callsJob(n ast.Node, names []string) bool {
	calls := false
	if n == nil {
		return false
	}
	ast.Inspect(n, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && slices.Contains(names, sel.Sel.Name) {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "job" {
					calls = true
				}
			}
		}
		return true
	})
	return calls
}

func Test_plug_bundled_reach_only_through_ways_out(t *testing.T) {
	// Every plugin that comes with Corbie is one the bundle brings in.
	set := token.NewFileSet()
	bundle, err := parser.ParseFile(set, filepath.Join("bundled", "bundled.go"), nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	const own = "github.com/zummed/corbie/plugin/"
	checked := 0
	for _, imported := range bundle.Imports {
		path, _ := strconv.Unquote(imported.Path.Value)
		dir, isOwn := strings.CutPrefix(path, own)
		if !isOwn {
			t.Errorf("the bundle brings in %s, which is not a plugin of Corbie's", path)
			continue
		}
		checked++
		for _, finding := range reach(t, filepath.FromSlash(dir), dir == "fs") {
			t.Error(finding)
		}
	}
	if checked < 15 {
		t.Fatalf("only %d plugins were read", checked)
	}

	// The check finds each of the ways a plugin could reach outside by
	// itself, in a plugin written to do so.
	bad := t.TempDir()
	source := `package bad

import (
	"net"
	nethttp "net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"syscall"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"
	"github.com/zummed/corbie/plugin"
)

func reach(job *plugin.Job) {
	net.Dial("tcp", "example.com:80")                 // line 18
	(&net.Dialer{}).Dial("tcp", "example.com:80")     // line 19
	net.LookupHost("example.com")                     // line 20
	nethttp.Get("http://example.com/")                // line 21
	nethttp.DefaultClient.Do(nil)                     // line 22
	(&nethttp.Client{}).Do(nil)                       // line 23
	_ = &nethttp.Transport{Proxy: nethttp.ProxyFromEnvironment, DialContext: nil} // line 24
	var client nethttp.Client                         // line 25
	client.Do(nil)
	os.Getenv("HOME")                                 // line 27
	os.ReadFile("/etc/passwd")                        // line 28
	os.UserHomeDir()                                  // line 29
	exec.Command("sh").Run()                          // line 30
	user.Current()                                    // line 31
	filepath.Glob("/etc/*")                           // line 32
	syscall.Getenv("HOME")                            // line 33
	ssh.Dial("tcp", "example.com:22", nil)            // line 34
	websocket.Dial(nil, "wss://example.com/", &websocket.DialOptions{}) // line 35
	job.Dial(nil, "example.com:80")
}
`
	if err := os.WriteFile(filepath.Join(bad, "bad.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := strings.Join(reach(t, bad, false), "\n")
	for line := 18; line <= 35; line++ {
		if line == 26 {
			continue
		}
		if !strings.Contains(findings, fmt.Sprintf("bad.go:%d:", line)) {
			t.Errorf("line %d of the plugin written to reach outside was not found:\n%s", line, strings.Split(source, "\n")[line-1])
		}
	}
	// A plugin uses no part of Corbie but the public package, and of the
	// Job neither its layers nor its secrets' values.
	source = `package bad

import (
	"github.com/zummed/corbie/blob"       // line 4
	"github.com/zummed/corbie/permission" // line 5
	"github.com/zummed/corbie/plugin"
	"crypto/x509"
)

func core(job *plugin.Job, opt plugin.ProgramOptions) {
	job.Perms.Peek(permission.Read, "/etc/passwd") // line 11
	_ = job.Secrets["K"]                           // line 12
	_ = blob.TypeOf("a.txt")                       // line 13
	x509.SystemCertPool()                          // line 14
	job.Noted = nil                                // line 15
	job.Whole("network")
	job.MayRead("/etc/passwd")
	job.Sent("K")
	opt.Secrets = nil
	_ = plugin.Refusal(nil)
	_ = plugin.TypeOf("a.txt")
	x509.ParsePKIXPublicKey(nil)
}
`
	os.Remove(filepath.Join(bad, "bad.go"))
	if err := os.WriteFile(filepath.Join(bad, "core.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	got := reach(t, bad, false)
	findings = strings.Join(got, "\n")
	for _, line := range []int{4, 5, 11, 12, 13, 14, 15} {
		if !strings.Contains(findings, fmt.Sprintf("core.go:%d:", line)) {
			t.Errorf("line %d of the plugin written to use Corbie's own parts was not found:\n%s", line, strings.Split(source, "\n")[line-1])
		}
	}
	for _, finding := range got {
		if at := strings.SplitN(finding, ":", 3); len(at) < 3 || !slices.Contains([]string{"4", "5", "11", "12", "13", "14", "15"}, at[1]) {
			t.Errorf("what a plugin may use of the public package was found: %s", finding)
		}
	}
	os.Remove(filepath.Join(bad, "core.go"))
	// In the plugin for files, the disk is touched only where the Job was
	// asked for the path.
	source = `package fs

import (
	"os"

	"github.com/zummed/corbie/plugin"
)

func asks(job *plugin.Job, given string) (string, error) { return job.Path(given, false) }

func good(job *plugin.Job, given string) {
	real, _ := asks(job, given)
	os.ReadFile(real)
}

func alsoGood(job *plugin.Job, given string) {
	real, _ := job.Entry(given)
	os.Remove(real)
}

func bad(job *plugin.Job, given string) {
	os.ReadFile(given) // line 22
	os.Getenv("HOME")  // line 23
}
`
	if err := os.WriteFile(filepath.Join(bad, "fs.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	got = reach(t, bad, true)
	if len(got) != 2 || !strings.Contains(got[0], "fs.go:23:") || !strings.Contains(got[1], "fs.go:22:") {
		t.Errorf("in a plugin for files written to reach outside, the check found:\n%s", strings.Join(got, "\n"))
	}
}
