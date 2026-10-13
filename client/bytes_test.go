package client_test

import "testing"

// Bytes are one type everywhere: a script's bytes reach a plugin as bytes.
func TestBytesFromAScriptReachAPluginAsBytes(t *testing.T) {
	l := local(t)
	r := run(t, l, `
		flow: bytes
		stages:
		  one: { job: { script: ./bytes.js } }
		output: ${stages.one.output}
	`, map[string]string{"bytes.js": `
		const abc = new Uint8Array([97, 98, 99])
		const view = new Uint8Array(new Uint8Array([0, 97, 98, 99, 0]).buffer, 1, 3)
		const back = crypto.decode(crypto.encode("abc"), { as: "bytes" })
		return {
			text: crypto.hash("abc"),
			typed: crypto.hash(abc),
			buffer: crypto.hash(abc.buffer),
			view: crypto.hash(view),
			back: crypto.hash(back),
			isBytes: back instanceof Uint8Array,
			empty: crypto.encode(new Uint8Array(0)),
		}
	`}, nil)
	out, _ := r.Output.(map[string]any)
	if r.Status != "done" || out == nil {
		t.Fatalf("the run ended %s: %s", r.Status, r.Error)
	}
	for _, name := range []string{"typed", "buffer", "view", "back"} {
		if out[name] != out["text"] {
			t.Errorf("%s hashes to %v, and the text to %v", name, out[name], out["text"])
		}
	}
	if out["isBytes"] != true || out["empty"] != "" {
		t.Errorf("bytes from a plugin: isBytes %v, empty %q", out["isBytes"], out["empty"])
	}
}
