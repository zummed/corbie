// Package xml is the plugin for reading a document as a tree, and writing
// a tree as a document.
package xml

import (
	"bytes"
	"encoding/xml"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html/charset"

	"github.com/zummed/corbie/plugin"
)

func init() {
	plugin.Register(&plugin.Module{Name: "xml", Page: page, Calls: map[string]*plugin.Call{
		"parse": {Do: parse},
		"write": {Options: []string{"indent"}, Do: write},
	}})
}

// An open element is one whose end has not been read yet.
type open struct {
	tag      string
	attrs    map[string]any
	text     strings.Builder
	children []any
}

// name gives a name as it was written, prefix and all.
func name(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func parse(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	data, _, err := job.Contents(first)
	if err != nil {
		return nil, err
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	d.CharsetReader = charset.NewReaderLabel
	if _, isText := first.(string); isText {
		// Text is read as it is, whatever its first line says it was
		// written in.
		d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	}
	bad := func(why string) (any, error) {
		line, _ := d.InputPos()
		return nil, plugin.Errorf(plugin.BadInput, "the document cannot be read: line %d: %s", line, why)
	}
	var stack []*open
	var top map[string]any
	for {
		// Names are taken as written, so what closes what is checked here.
		token, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, plugin.Errorf(plugin.BadInput, "the document cannot be read: %v", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			if top != nil {
				return bad("<" + name(token.Name) + "> starts a second top element, and a document has one")
			}
			attrs := map[string]any{}
			for _, a := range token.Attr {
				attrs[name(a.Name)] = a.Value
			}
			stack = append(stack, &open{tag: name(token.Name), attrs: attrs, children: []any{}})
		case xml.EndElement:
			if len(stack) == 0 {
				return bad("</" + name(token.Name) + "> closes nothing")
			}
			done := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if done.tag != name(token.Name) {
				return bad("</" + name(token.Name) + "> closes <" + done.tag + ">")
			}
			node := map[string]any{"tag": done.tag, "attrs": done.attrs, "text": done.text.String(), "children": done.children}
			if len(stack) == 0 {
				top = node
				continue
			}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, node)
			parent.text.WriteString(done.text.String())
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(token)
			}
		}
	}
	if len(stack) > 0 {
		return bad("<" + stack[len(stack)-1].tag + "> is not closed")
	}
	if top == nil {
		return bad("it has no element")
	}
	return top, nil
}

func write(job *plugin.Job, first any, opt plugin.Options) (any, error) {
	indent, err := opt.Int("indent", 0)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	if indent > 0 {
		enc.Indent("", strings.Repeat(" ", indent))
	}
	if err := element(enc, first); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, plugin.Errorf(plugin.BadInput, "the document cannot be written: %v", err)
	}
	return out.String(), nil
}

// element writes an element and what is inside it: its children, or, when
// it has none, its text.
func element(enc *xml.Encoder, v any) error {
	node, _ := v.(map[string]any)
	tag, _ := node["tag"].(string)
	if !isName(tag) {
		return plugin.Errorf(plugin.BadInput, "an element is { tag, attrs, text, children }, and its tag a name an element can have, not %q", tag)
	}
	start := xml.StartElement{Name: xml.Name{Local: tag}}
	attrs, _ := node["attrs"].(map[string]any)
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value, err := textOf(attrs[name])
		if err != nil || !isName(name) {
			return plugin.Errorf(plugin.BadInput, "<%s>: an attribute has a name an attribute can have, not %q, and its value is text, a number or true or false", tag, name)
		}
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: value})
	}
	enc.EncodeToken(start)
	children, _ := node["children"].([]any)
	if len(children) == 0 {
		text, err := textOf(node["text"])
		if err != nil {
			return plugin.Errorf(plugin.BadInput, "<%s>: an element's text is text, a number or true or false", tag)
		}
		enc.EncodeToken(xml.CharData(text))
	}
	for _, child := range children {
		if err := element(enc, child); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

// isName says whether a name is one XML allows, so that a name taken from
// data writes no markup of its own.
func isName(name string) bool {
	for i, r := range name {
		switch {
		case unicode.IsLetter(r) || r == '_':
		case i > 0 && (unicode.IsDigit(r) || r == '-' || r == '.' || r == ':'):
		default:
			return false
		}
	}
	return name != ""
}

func textOf(v any) (string, error) {
	switch v := v.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	}
	return "", plugin.Errorf(plugin.BadInput, "this should be text, a number or true or false, and is %T", v)
}

const page = `xml: this plugin parses an XML document into a tree of elements and writes a tree as XML.

  xml.parse(document)               returns the document's top element
  xml.write(element, { indent })    returns the element and all inside it as text

document is text, bytes or a blob. Text is read as it is. Bytes and a blob are
decoded from the character set that their XML declaration names.

Options:
  indent   spaces of indent for each level; default 0, which writes one line

An element is plain data with these fields.

  tag        its name as written, with any prefix ("dc:creator")
  attrs      a map of its attributes; a namespace declaration ("xmlns:dc") is an attribute
  text       all the text inside it, its children's included, with entities decoded
  children   the elements directly inside it

- parse leaves comments out.
- write takes the same fields, and attrs, text and children may be omitted.
- write writes an element's children, or its text when it has none.
- write escapes text and attribute values, each of which is text, a number,
  true or false.

Errors:
  bad_input   the document is not well formed (the message gives the line), or
              a tag or an attribute has a name that XML does not allow

Needs nothing.

This script parses a document and writes another.

    const feed = xml.parse('<feed lang="en"><entry><title>One</title></entry></feed>')
    feed.attrs.lang                       // gives "en"
    feed.children[0].children[0].text     // gives "One"

    xml.write({ tag: "order", attrs: { id: 7 }, children: [{ tag: "item", text: "tea & cake" }] })
    // gives '<order id="7"><item>tea &amp; cake</item></order>'
`
