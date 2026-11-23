//go:build js

package websocket

import (
	"context"
	"net/http"

	"github.com/coder/websocket"

	"github.com/zummed/corbie/plugin"
)

// Built for js/wasm, the browser opens a socket itself, over a connection
// that no permission of the job was asked for. The plugin opens none there.
func dial(context.Context, *plugin.Job, string, http.Header) (*websocket.Conn, error) {
	return nil, plugin.Errorf(plugin.Failed, "the websocket plugin opens no connection in a build for js/wasm")
}
