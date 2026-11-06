package machine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// lines gives the file and the line of each value written out, by its
// place.
func (p *Parser) lines() map[string]string {
	lines := map[string]string{}
	for place, n := range p.at {
		if !n.isMap() && !n.list && place != "" {
			lines[place] = n.at()
		}
	}
	return lines
}

// Hash gives what a definition runs, as a hash: its stored form, with each
// file it names by its contents and each child definition by its own hash.
// Where each value is written takes no part, so moving a line changes
// nothing.
func (d *Draft) Hash() string {
	def := *d.Definition
	scripts, machines := map[string]string{}, map[string]string{}
	for name := range def.Scripts {
		sum := sha256.Sum256(d.Files[name])
		scripts[name] = hex.EncodeToString(sum[:])
	}
	for name := range def.Machines {
		if child := d.Children[name]; child != nil {
			machines[name] = child.Hash()
		}
	}
	def.Scripts, def.Machines, def.Lines = nil, nil, nil
	text, _ := json.Marshal(struct {
		Definition *Definition       `json:"definition"`
		Scripts    map[string]string `json:"scripts"`
		Machines   map[string]string `json:"machines"`
	}{&def, scripts, machines}, json.Deterministic(true))
	sum := sha256.Sum256(text)
	return hex.EncodeToString(sum[:])
}

// commentAt is the column at which a value's file and line are printed.
const commentAt = 45

// WrittenOut gives the definition as it is written out, in the order it is
// written, each value with the file and the line it comes from. A machine
// written in place is printed in place, and one kept in its own file as its
// path.
func (d *Draft) WrittenOut() []byte {
	var b bytes.Buffer
	if d.written != nil {
		printMapping(&b, d.written, "", "")
	}
	return b.Bytes()
}

// printMapping prints a mapping's fields at an indent. The first is printed
// after first, which stands in for the indent.
func printMapping(b *bytes.Buffer, n *node, indent, first string) {
	for i, k := range n.keys {
		lead := indent
		if i == 0 {
			lead = first
		}
		printValue(b, lead+scalar(k)+":", n.m[k], indent)
	}
}

// printValue prints a value after what leads it, at an indent.
func printValue(b *bytes.Buffer, lead string, v *node, indent string) {
	switch {
	case v.isMap() && len(v.keys) > 0:
		b.WriteString(lead + "\n")
		printMapping(b, v, indent+"  ", indent+"  ")
	case v.isMap():
		printLine(b, lead+" {}", v)
	case v.list && len(v.items) > 0:
		b.WriteString(lead + "\n")
		for _, item := range v.items {
			printItem(b, item, indent+"  ")
		}
	case v.list:
		printLine(b, lead+" []", v)
	default:
		text := scalar(v.value)
		head, rest, block := strings.Cut(text, "\n")
		printLine(b, lead+" "+head, v)
		if block {
			for _, line := range strings.Split(rest, "\n") {
				if line = strings.TrimPrefix(line, "    "); line != "" {
					line = indent + "  " + line
				}
				b.WriteString(line + "\n")
			}
		}
	}
}

// printItem prints an item of a list at an indent.
func printItem(b *bytes.Buffer, item *node, indent string) {
	switch {
	case item.isMap() && len(item.keys) > 0:
		printMapping(b, item, indent+"  ", indent+"- ")
	case item.list && len(item.items) > 0:
		b.WriteString(indent + "-\n")
		for _, inner := range item.items {
			printItem(b, inner, indent+"  ")
		}
	default:
		printValue(b, indent+"-", item, indent)
	}
}

// printLine prints one line with the file and the line of its value.
func printLine(b *bytes.Buffer, line string, v *node) {
	b.WriteString(line)
	b.WriteString(strings.Repeat(" ", max(commentAt-len([]rune(line)), 2)))
	fmt.Fprintf(b, "# %s\n", v.at())
}

// scalar gives a value as YAML writes it, with no line break at its end.
func scalar(v any) string {
	text, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSuffix(string(text), "\n")
}
