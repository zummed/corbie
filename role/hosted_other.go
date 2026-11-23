//go:build !js

package role

import "net/http"

// Hosted gives an HTTP server to a host that has no port to listen on, and
// says whether there is such a host. A computer has ports.
func Hosted(http.Handler) bool { return false }
