package plugin

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// framing are the headers that say where a web request goes and how it is
// framed. Whatever sends the request writes them itself, from the address
// and the body, so one a script gave would be dropped or rewritten on the
// way.
var framing = []string{"Host", "Content-Length", "Transfer-Encoding", "Trailer", "Connection"}

// Headers gives the headers a script wrote for a web request that goes
// over a connection to an address, host:port. Each is sent as written,
// name and value, with a secret or without, and a value that holds secrets
// is revealed for the connection, as RevealOver does it. A header that is
// not sent as written is refused before any value is taken: the framing
// ones, and those the plugin says are its own to write. So is a value that
// holds a line break or a zero byte, which would start a header of its
// own.
func (j *Job) Headers(given map[string]any, address string, encrypted bool, own ...string) (http.Header, error) {
	notAScripts := slices.Concat(framing, own)
	names := make([]string, 0, len(given))
	for name := range given {
		if !httpguts.ValidHeaderFieldName(name) {
			return nil, Errorf(BadInput, "%q cannot be the name of a header", name)
		}
		if slices.ContainsFunc(notAScripts, func(written string) bool { return strings.EqualFold(written, name) }) {
			return nil, Errorf(BadInput, "the header %s is not a script's to set: it is written from the address and the body as the request is sent", name)
		}
		names = append(names, name)
	}
	slices.Sort(names)
	headers := http.Header{}
	for _, name := range names {
		text, err := j.RevealOver(TextOf(given[name]), address, encrypted)
		if err != nil {
			return nil, err
		}
		if !httpguts.ValidHeaderFieldValue(text) {
			return nil, Errorf(BadInput, "the header %s holds a line break, a zero byte or another character no header can hold", name)
		}
		// The name is kept as it was written, not in the usual capitals.
		headers[name] = []string{text}
	}
	return headers, nil
}

// EscapedAddress gives an address as it goes out, with what cannot go out
// as it is escaped, byte for byte: a blank and a letter beyond ASCII
// anywhere, and before the query the marks no path may hold. An address
// left with one of those in its path would have the whole path written out
// afresh as it is sent, and what was escaped in it already, such as %2F,
// would come out unescaped.
func EscapedAddress(address string) string {
	var out strings.Builder
	query := false
	for i := range len(address) {
		c := address[i]
		query = query || c == '?' || c == '#'
		if c == ' ' || c >= 0x80 || (!query && strings.IndexByte("\"<>\\^`{|}", c) >= 0) {
			fmt.Fprintf(&out, "%%%02X", c)
		} else {
			out.WriteByte(c)
		}
	}
	return out.String()
}
