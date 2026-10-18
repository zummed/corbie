// Package bundled is every plugin that comes with Corbie. A binary imports
// it to have them.
package bundled

import (
	_ "github.com/zummed/corbie/plugin/blobs"
	_ "github.com/zummed/corbie/plugin/compress"
	_ "github.com/zummed/corbie/plugin/crypto"
	_ "github.com/zummed/corbie/plugin/env"
	_ "github.com/zummed/corbie/plugin/exec"
	_ "github.com/zummed/corbie/plugin/fs"
	_ "github.com/zummed/corbie/plugin/html"
	_ "github.com/zummed/corbie/plugin/http"
	_ "github.com/zummed/corbie/plugin/llm"
	_ "github.com/zummed/corbie/plugin/llm/claudecode"
	_ "github.com/zummed/corbie/plugin/machines"
	_ "github.com/zummed/corbie/plugin/mcp"
	_ "github.com/zummed/corbie/plugin/net"
	_ "github.com/zummed/corbie/plugin/secrets"
	_ "github.com/zummed/corbie/plugin/ssh"
	_ "github.com/zummed/corbie/plugin/websocket"
	_ "github.com/zummed/corbie/plugin/xml"
)
