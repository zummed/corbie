// Package html is the plugin for reading a page as a tree.
package html

import (
	"bytes"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"github.com/zummed/corbie/plugin"
)

func init() {
	plugin.Register(&plugin.Module{Name: "html", Page: page, Calls: map[string]*plugin.Call{
		"parse": {Do: parse},
		"text":  {Do: text},
	}})
}

// read reads a page. Text is read as it is; bytes and a blob are read in
// the character set their start or the page itself names.
func read(job *plugin.Job, first any) (*html.Node, error) {
	data, typ, err := job.Contents(first)
	if err != nil {
		return nil, err
	}
	var doc *html.Node
	if _, isText := first.(string); isText {
		doc, err = html.Parse(bytes.NewReader(data))
	} else {
		r, cerr := charset.NewReader(bytes.NewReader(data), typ)
		if cerr != nil {
			return nil, plugin.Errorf(plugin.BadInput, "the page cannot be read: %v", cerr)
		}
		doc, err = html.Parse(r)
	}
	if err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "the page cannot be read: %v", err)
	}
	return doc, nil
}

func parse(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	doc, err := read(job, first)
	if err != nil {
		return nil, err
	}
	found := func(first any, one bool) (any, error) {
		selector, err := plugin.Text(first, "a selector")
		if err != nil {
			return nil, err
		}
		steps, err := readSelector(selector)
		if err != nil {
			return nil, err
		}
		list := []any{}
		for n := range doc.Descendants() {
			if n.Type != html.ElementNode || !matches(n, steps) {
				continue
			}
			node, _ := element(n)
			if one {
				return node, nil
			}
			list = append(list, node)
		}
		if one {
			return nil, nil
		}
		return list, nil
	}
	return &plugin.Object{Calls: map[string]*plugin.Call{
		"find": {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) { return found(first, true) }},
		"all":  {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) { return found(first, false) }},
		"tree": {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			for n := range doc.ChildNodes() {
				if n.Type == html.ElementNode {
					node, _ := element(n)
					return node, nil
				}
			}
			return nil, nil
		}},
	}}, nil
}

// element gives an element as plain data, and all the text inside it.
func element(n *html.Node) (map[string]any, string) {
	attrs := map[string]any{}
	for _, a := range n.Attr {
		attrs[a.Key] = a.Val
	}
	children := []any{}
	var inside strings.Builder
	for c := range n.ChildNodes() {
		switch c.Type {
		case html.TextNode:
			inside.WriteString(c.Data)
		case html.ElementNode:
			child, within := element(c)
			children = append(children, child)
			inside.WriteString(within)
		}
	}
	return map[string]any{"tag": n.Data, "attrs": attrs, "text": inside.String(), "children": children}, inside.String()
}

// A step is one part of a selector, between spaces: a tag, an id, classes
// and attributes, all of which an element must have.
type step struct {
	tag, id string
	classes []string
	attrs   []attr
}

type attr struct {
	name, value string
	any         bool // the attribute need only be there
}

// readSelector reads a selector: steps with spaces between, each a step
// further inside the one before.
func readSelector(selector string) ([]step, error) {
	bad := func(why string) ([]step, error) {
		return nil, plugin.Errorf(plugin.BadInput, "the selector %q cannot be read: %s", selector, why)
	}
	var all []step
	for _, written := range strings.Fields(selector) {
		var s step
		for rest := written; rest != ""; {
			sign := rest[0]
			if sign == '[' {
				inside, after, closed := strings.Cut(rest[1:], "]")
				name, value, has := strings.Cut(inside, "=")
				if !closed || name == "" {
					return bad("an attribute is written [name] or [name=value], with no space in it")
				}
				s.attrs = append(s.attrs, attr{name: strings.ToLower(name), value: strings.Trim(value, `"'`), any: !has})
				rest = after
				continue
			}
			if sign == '#' || sign == '.' {
				rest = rest[1:]
			}
			end := strings.IndexAny(rest, "#.[")
			if end < 0 {
				end = len(rest)
			}
			name := rest[:end]
			rest = rest[end:]
			switch {
			case name == "" || strings.ContainsAny(name, ">+~,:()"):
				return bad("it may hold a tag, #id, .class, [attribute] and [attribute=value], and spaces between for one inside another")
			case sign == '#':
				s.id = name
			case sign == '.':
				s.classes = append(s.classes, name)
			case name != "*":
				s.tag = strings.ToLower(name)
			}
		}
		all = append(all, s)
	}
	if len(all) == 0 {
		return bad("it is empty")
	}
	return all, nil
}

// matches says whether an element is what a selector's last step asks for,
// inside elements that are what the steps before ask for, in their order.
func matches(n *html.Node, steps []step) bool {
	last := len(steps) - 1
	if !steps[last].matches(n) {
		return false
	}
	for up := n.Parent; last > 0; up = up.Parent {
		if up == nil {
			return false
		}
		if up.Type == html.ElementNode && steps[last-1].matches(up) {
			last--
		}
	}
	return true
}

func (s step) matches(n *html.Node) bool {
	value := func(name string) (string, bool) {
		for _, a := range n.Attr {
			if a.Key == name {
				return a.Val, true
			}
		}
		return "", false
	}
	if s.tag != "" && s.tag != n.Data {
		return false
	}
	if id, _ := value("id"); s.id != "" && s.id != id {
		return false
	}
	class, _ := value("class")
	for _, want := range s.classes {
		if !slices.Contains(strings.Fields(class), want) {
			return false
		}
	}
	for _, want := range s.attrs {
		if got, has := value(want.name); !has || (!want.any && got != want.value) {
			return false
		}
	}
	return true
}

// unread are the elements whose text is not for a reader.
var unread = []string{"script", "style", "noscript", "template", "head"}

// breaks are the elements that start a line of their own.
var breaks = []string{
	"address", "article", "aside", "blockquote", "br", "dd", "div", "dl", "dt", "figcaption", "figure", "footer",
	"form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "ol", "p", "pre", "section",
	"table", "td", "th", "tr", "ul",
}

// text gives the text of a page as a reader sees it: a line for each
// block, with the space inside a line made one space.
func text(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	doc, err := read(job, first)
	if err != nil {
		return nil, err
	}
	var all strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			all.WriteString(strings.ReplaceAll(n.Data, "\n", " "))
			return
		case n.Type == html.ElementNode && slices.Contains(unread, n.Data):
			return
		}
		block := n.Type == html.ElementNode && slices.Contains(breaks, n.Data)
		if block {
			all.WriteString("\n")
		}
		for c := range n.ChildNodes() {
			walk(c)
		}
		if block {
			all.WriteString("\n")
		}
	}
	walk(doc)
	var lines []string
	for line := range strings.SplitSeq(all.String(), "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n"), nil
}

const page = `html: this plugin parses an HTML page into a tree of elements.

  html.parse(page)     the parsed page, doc, which has the next three calls
  doc.find(selector)   the first element that matches, or null
  doc.all(selector)    every element that matches, in the page's order
  doc.tree()           the page's root element
  html.text(page)      the page's text, a line for each block

page is text, bytes or a blob of at most 64 MiB. Bytes and a blob are decoded
from the character set that a byte order mark, a blob's type or the page's
meta tag names.

Any page parses, as in a browser: html, head and body are added when the page
omits them, and an element left open is closed. No script on the page runs.
html.text omits script, style, noscript, template and head.

An element is { tag, attrs, text, children }.

  tag        its name, in lower case
  attrs      a map of its attributes
  text       all the text inside it
  children   the elements directly inside it, each an element

A selector is one of these.

  td                 an element of that name
  *                  any element
  #main              the element with that id
  .price             an element with that word in its class
  [href]             an element that has the attribute
  [type=text]        an element whose attribute has that value
  a.external[href]   an element that matches every part
  table tr           a tr anywhere inside a table

Errors:
  bad_input   a selector of any other form, such as one with > or a comma

Needs nothing.

These calls parse text.

    html.parse("<title>T</title>").find("title").text       // gives "T"
    html.parse("<title>T</title>").tree().tag               // gives "html"
    html.text("<h1>News</h1><p>Tea is up.</p>")             // gives "News\nTea is up."

This script parses a page fetched with http.

    allow: [net:example.com]

    const doc = html.parse(http.get("https://example.com/news").body)
    return doc.all("a[href]").map((a) => ({ text: a.text.trim(), href: a.attrs.href }))
    // gives [{ text: "Tea is up", href: "/news/tea" }, { text: "Cake is down", href: "/news/cake" }]
`
