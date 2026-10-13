package xml

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zummed/corbie/permission"
	"github.com/zummed/corbie/plugin"
)

// A job no layer allows anything: the plugin needs no permission.
func call(t *testing.T, name string, first any, opt plugin.Options) (any, error) {
	t.Helper()
	job := &plugin.Job{
		Context: context.Background(),
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test"}}},
		Scratch: t.TempDir(),
	}
	module, _ := plugin.Find("xml")
	return module.Calls[name].Invoke(job, "xml", name, first, opt)
}

func code(err error) string {
	var e *plugin.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

const feed = `<?xml version="1.0"?>
<!-- the news -->
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <item id="a1"><title>Rain &amp; wind</title><dc:creator><![CDATA[Ann <ann@example.com>]]></dc:creator></item>
  <item id="a2"><title>Sun</title><empty/></item>
</rss>`

func Test_xml_element_text_and_attribute(t *testing.T) {
	doc, err := call(t, "parse", feed, nil)
	if err != nil {
		t.Fatal(err)
	}
	top := doc.(map[string]any)
	if top["tag"] != "rss" || !reflect.DeepEqual(top["attrs"], map[string]any{"version": "2.0", "xmlns:dc": "http://purl.org/dc/elements/1.1/"}) {
		t.Fatalf("the top element is %v with %v", top["tag"], top["attrs"])
	}
	items := top["children"].([]any)
	if len(items) != 2 {
		t.Fatalf("the top element has %d children", len(items))
	}
	want := map[string]any{
		"tag": "item", "attrs": map[string]any{"id": "a1"}, "text": "Rain & windAnn <ann@example.com>",
		"children": []any{
			map[string]any{"tag": "title", "attrs": map[string]any{}, "text": "Rain & wind", "children": []any{}},
			map[string]any{"tag": "dc:creator", "attrs": map[string]any{}, "text": "Ann <ann@example.com>", "children": []any{}},
		},
	}
	if !reflect.DeepEqual(items[0], want) {
		t.Fatalf("the first item is %v", items[0])
	}
}

func Test_xml_bytes_in_another_character_set(t *testing.T) {
	doc, err := call(t, "parse", plugin.Bytes("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><a>caf\xe9</a>"), nil)
	if err != nil || doc.(map[string]any)["text"] != "café" {
		t.Fatalf("parse gave %v, %v", doc, err)
	}
	// Text is read as it is, whatever it says it was written in.
	doc, err = call(t, "parse", `<?xml version="1.0" encoding="ISO-8859-1"?><a>café</a>`, nil)
	if err != nil || doc.(map[string]any)["text"] != "café" {
		t.Fatalf("parse gave %v, %v", doc, err)
	}
}

func Test_xml_not_well_formed(t *testing.T) {
	for document, why := range map[string]string{
		"<a><b></a>":       "line 1: </a> closes <b>",
		"<a>\n<b>":         "line 2: <b> is not closed",
		"<a/>\n\n<b/>":     "line 3: <b> starts a second top element",
		"</a>":             "</a> closes nothing",
		"<a x=1/>":         "line 1",
		"<a>&nbsp;</a>":    "line 1",
		"":                 "it has no element",
		"<!-- nothing -->": "it has no element",
	} {
		_, err := call(t, "parse", document, nil)
		if code(err) != plugin.BadInput || !strings.Contains(err.Error(), why) {
			t.Fatalf("%q gave %v, which does not say %q", document, err, why)
		}
	}
}

func Test_xml_write(t *testing.T) {
	tree := map[string]any{
		"tag": "order", "attrs": map[string]any{"id": 7.0, "paid": true, "note": `a "b" <c>`},
		"children": []any{
			map[string]any{"tag": "item", "text": "tea & cake"},
			map[string]any{"tag": "dc:when", "attrs": map[string]any{}, "text": 2026.5, "children": []any{}},
			map[string]any{"tag": "none"},
		},
	}
	got, err := call(t, "write", tree, nil)
	want := `<order id="7" note="a &#34;b&#34; &lt;c&gt;" paid="true"><item>tea &amp; cake</item><dc:when>2026.5</dc:when><none></none></order>`
	if err != nil || got != want {
		t.Fatalf("write gave %v, %v", got, err)
	}
	got, err = call(t, "write", tree, plugin.Options{"indent": 2.0})
	want = "<order id=\"7\" note=\"a &#34;b&#34; &lt;c&gt;\" paid=\"true\">\n  <item>tea &amp; cake</item>\n  <dc:when>2026.5</dc:when>\n  <none></none>\n</order>"
	if err != nil || got != want {
		t.Fatalf("write gave %v, %v", got, err)
	}
}

func Test_xml_written_reads_back_the_same(t *testing.T) {
	first, err := call(t, "parse", `<a x="1&amp;2"><b>one &lt; two</b><c><d k="v"/></c></a>`, nil)
	if err != nil {
		t.Fatal(err)
	}
	text, err := call(t, "write", first, nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := call(t, "parse", text, nil)
	if err != nil || !reflect.DeepEqual(again, first) {
		t.Fatalf("%v was written as %v and read back as %v, %v", first, text, again, err)
	}
}

func Test_xml_write_refuses_names_that_are_markup(t *testing.T) {
	for _, tree := range []any{
		"text",
		map[string]any{"text": "no tag"},
		map[string]any{"tag": "a><script"},
		map[string]any{"tag": "1st"},
		map[string]any{"tag": "a", "attrs": map[string]any{`x="1" y`: "2"}},
		map[string]any{"tag": "a", "attrs": map[string]any{"x": []any{}}},
		map[string]any{"tag": "a", "text": map[string]any{}},
		map[string]any{"tag": "a", "children": []any{"loose text"}},
	} {
		if got, err := call(t, "write", tree, nil); code(err) != plugin.BadInput {
			t.Fatalf("writing %v gave %v, %v", tree, got, err)
		}
	}
	_, err := call(t, "write", map[string]any{"tag": "a"}, plugin.Options{"pretty": true})
	if code(err) != plugin.BadInput || !strings.Contains(err.Error(), `"pretty"`) {
		t.Fatalf("an unknown option gave %v", err)
	}
}
