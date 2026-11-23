//go:build !js

package websocket

import (
	"context"
	"net"
	"net/http"

	"github.com/coder/websocket"

	"github.com/zummed/corbie/plugin"
)

// dial opens the socket over a connection the job made, with the headers
// the job may send.
func dial(ctx context.Context, job *plugin.Job, to string, header http.Header) (*websocket.Conn, error) {
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:             nil, // nothing of the machine's own settings is used
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return job.Dial(ctx, address)
			},
		},
		// A redirect would carry the headers to a place they were not
		// revealed for.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	conn, _, err := websocket.Dial(ctx, to, &websocket.DialOptions{HTTPClient: client, HTTPHeader: header})
	return conn, err
}
