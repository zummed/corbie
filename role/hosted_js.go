//go:build js

package role

import (
	"bytes"
	"context"
	"net/http"
	"syscall/js"
)

// Hosted gives an HTTP server to a host that has no port to listen on, and
// says whether there is such a host. A browser has none: the page's code
// calls the server, as a serverless host calls a function.
//
//	corbieGateway.fetch(method, path, headers, body, head, part, end)
//
// head is called with the status and the answer's type, part with each part
// of the body as a Uint8Array, and end when the answer is whole. What fetch
// gives is a function that stops the request, as a caller that leaves does.
// A call with other arguments gives an Error and asks nothing.
func Hosted(handler http.Handler) bool {
	js.Global().Set("corbieGateway", map[string]any{"fetch": js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) != 7 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString ||
			args[4].Type() != js.TypeFunction || args[5].Type() != js.TypeFunction || args[6].Type() != js.TypeFunction ||
			!(args[3].IsNull() || args[3].IsUndefined() || args[3].InstanceOf(js.Global().Get("Uint8Array"))) {
			return js.Global().Get("Error").New("corbieGateway.fetch takes a method, a path, the headers, a body as a Uint8Array or null, and the functions head, part and end")
		}
		method, path, headers := args[0].String(), args[1].String(), args[2]
		var body []byte
		if !args[3].IsNull() && !args[3].IsUndefined() {
			body = make([]byte, args[3].Get("length").Int())
			js.CopyBytesToGo(body, args[3])
		}
		a := &hostedAnswer{head: http.Header{}, onHead: args[4], onPart: args[5]}
		end := args[6]
		ctx, stop := context.WithCancel(context.Background())
		req, err := http.NewRequestWithContext(ctx, method, "http://corbie"+path, bytes.NewReader(body))
		if err == nil && headers.Type() == js.TypeObject {
			names := js.Global().Get("Object").Call("keys", headers)
			for i := range names.Length() {
				req.Header.Set(names.Index(i).String(), headers.Get(names.Index(i).String()).String())
			}
		}
		// The function that stops the request is the program's until the
		// answer is whole, and nothing after that.
		stopIt := js.FuncOf(func(js.Value, []js.Value) any { stop(); return nil })
		given := js.Global().Get("Function").New("stop", "return [() => { if (stop) stop() }, () => { stop = null }]").Invoke(stopIt)
		go func() {
			defer stopIt.Release()
			defer given.Index(1).Invoke()
			defer end.Invoke()
			defer stop()
			// One request that cannot be answered ends that request, as on
			// a connection, and not the program.
			defer func() {
				if recover() != nil {
					a.WriteHeader(http.StatusInternalServerError)
				}
			}()
			if err != nil {
				a.WriteHeader(http.StatusBadRequest)
				return
			}
			handler.ServeHTTP(a, req)
			a.WriteHeader(http.StatusOK)
		}()
		// A call after the answer is whole does nothing.
		return given.Index(0)
	})})
	return true
}

// hostedAnswer gives the page's code an answer as the server writes it.
type hostedAnswer struct {
	head           http.Header
	sent           bool
	onHead, onPart js.Value
}

func (a *hostedAnswer) Header() http.Header { return a.head }

func (a *hostedAnswer) WriteHeader(status int) {
	if !a.sent {
		a.sent = true
		a.onHead.Invoke(status, a.head.Get("Content-Type"))
	}
}

func (a *hostedAnswer) Write(p []byte) (int, error) {
	a.WriteHeader(http.StatusOK)
	part := js.Global().Get("Uint8Array").New(len(p))
	js.CopyBytesToJS(part, p)
	a.onPart.Invoke(part)
	return len(p), nil
}

func (a *hostedAnswer) Flush() {}
