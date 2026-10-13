package html

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
func newJob(t *testing.T) *plugin.Job {
	return &plugin.Job{
		Context: context.Background(),
		Perms:   &permission.Set{Layers: []permission.Layer{{Name: "test"}}},
		Scratch: t.TempDir(),
	}
}

// parsed reads a page and gives a way to make the calls it then has.
func parsed(t *testing.T, page any) func(name string, first any) (any, error) {
	t.Helper()
	job := newJob(t)
	module, _ := plugin.Find("html")
	doc, err := module.Calls["parse"].Invoke(job, "html", "parse", page, nil)
	if err != nil {
		t.Fatalf("html.parse: %v", err)
	}
	return func(name string, first any) (any, error) {
		return doc.(*plugin.Object).Calls[name].Invoke(job, "object", name, first, nil)
	}
}

func code(err error) string {
	var e *plugin.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

const news = `<!doctype html>
<html><head><title>News</title><style>p { color: red }</style></head>
<body>
<div id="main" class="wide column">
  <h1>Today</h1>
  <p class="lead">Rain &amp; wind, <b>all</b> day.</p>
  <ul><li><a href="/one" class="external link">One</a><li><a name="two">Two</a></ul>
</div>
<div id="foot"><a href="/about" TITLE="About us">About</a></div>
<script>var hidden = "<p>not text</p>"</script>
</body></html>`

func Test_html_find_title(t *testing.T) {
	doc := parsed(t, "<title>T</title>")
	if got, err := doc("find", "title"); err != nil || got.(map[string]any)["text"] != "T" {
		t.Fatalf("the title is %v, %v", got, err)
	}
	if got, err := doc("find", "table"); err != nil || got != nil {
		t.Fatalf("an element that is not there gave %v, %v", got, err)
	}
	// A page that writes no html, head or body has them all the same.
	if got, err := doc("tree", nil); err != nil || got.(map[string]any)["tag"] != "html" {
		t.Fatalf("the top element is %v, %v", got, err)
	}
}

func Test_html_selectors(t *testing.T) {
	doc := parsed(t, news)
	for selector, want := range map[string][]string{
		"a":                     {"One", "Two", "About"},
		"A[HREF]":               {"One", "About"},
		"#main a":               {"One", "Two"},
		"div ul a.link":         {"One"},
		"a.external.link":       {"One"},
		".link.missing":         nil,
		"[name=two]":            {"Two"},
		"#foot [href='/about']": {"About"},
		`a[title="About"]`:      nil,
		"body * b":              {"all"},
		"p":                     {"Rain & wind, all day."},
		"ul p":                  nil,
		"html body div h1":      {"Today"},
	} {
		found, err := doc("all", selector)
		if err != nil {
			t.Fatalf("%q: %v", selector, err)
		}
		var got []string
		for _, node := range found.([]any) {
			got = append(got, node.(map[string]any)["text"].(string))
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%q matched %q, not %q", selector, got, want)
		}
	}
	for _, selector := range []string{"", "div > a", "a:first-child", "a, b", "a[href", "a..b", `a[title="About us"]`} {
		if _, err := doc("all", selector); code(err) != plugin.BadInput {
			t.Fatalf("the selector %q gave %v", selector, err)
		}
	}
}

func Test_html_element_is_plain_data(t *testing.T) {
	doc := parsed(t, news)
	want := map[string]any{
		"tag": "div", "attrs": map[string]any{"id": "foot"}, "text": "About",
		"children": []any{map[string]any{
			"tag": "a", "attrs": map[string]any{"href": "/about", "title": "About us"}, "text": "About", "children": []any{},
		}},
	}
	if got, _ := doc("find", "#foot"); !reflect.DeepEqual(got, want) {
		t.Fatalf("the element is %v", got)
	}
	// A script's text is its element's text, as any element's is.
	if got, _ := doc("find", "script"); got.(map[string]any)["text"] != `var hidden = "<p>not text</p>"` {
		t.Fatalf("the script is %v", got)
	}
}

func Test_html_bytes_in_another_character_set(t *testing.T) {
	page := plugin.Bytes("<meta charset=iso-8859-1><p>caf\xe9</p>")
	if got, _ := parsed(t, page)("find", "p"); got.(map[string]any)["text"] != "café" {
		t.Fatalf("the element is %v", got)
	}
	blob := plugin.NewBlob("text/html; charset=windows-1252", []byte("<p>\x80 5</p>"))
	if got, _ := parsed(t, blob)("find", "p"); got.(map[string]any)["text"] != "€ 5" {
		t.Fatalf("the element is %v", got)
	}
}

func Test_html_text_as_a_reader_sees_it(t *testing.T) {
	module, _ := plugin.Find("html")
	got, err := module.Calls["text"].Invoke(newJob(t), "html", "text", news, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Today\nRain & wind, all day.\nOne\nTwo\nAbout"; got != want {
		t.Fatalf("the text is %q, not %q", got, want)
	}
}

func Test_html_unknown_option(t *testing.T) {
	module, _ := plugin.Find("html")
	_, err := module.Calls["parse"].Invoke(newJob(t), "html", "parse", news, plugin.Options{"strict": true})
	if code(err) != plugin.BadInput || !strings.Contains(err.Error(), `"strict"`) {
		t.Fatalf("an unknown option gave %v", err)
	}
}
