package drawing

import (
	"encoding/json/v2"
	"fmt"
	"html/template"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/zummed/corbie/machine"
)

// The tests lay a scene out with layout.js, as the browser does, run as a
// script: the file with its export words removed.
var layoutJS = struct {
	once sync.Once
	vm   *goja.Runtime
	err  error
}{}

// laid gives what layout.js lays a scene out as.
func laid(s Scene) (laidOut, error) {
	layoutJS.once.Do(func() {
		src, err := os.ReadFile("layout.js")
		if err != nil {
			layoutJS.err = err
			return
		}
		layoutJS.vm = goja.New()
		_, layoutJS.err = layoutJS.vm.RunString(regexp.MustCompile(`(?m)^export `).ReplaceAllString(string(src), ""))
	})
	var out laidOut
	if layoutJS.err != nil {
		return out, layoutJS.err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return out, err
	}
	layoutJS.vm.Set("scene", string(data))
	v, err := layoutJS.vm.RunString("JSON.stringify(layout(JSON.parse(scene)))")
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(v.String()), &out, json.MatchCaseInsensitiveNames(true))
	return out, err
}

// drawn gives a definition's drawing as SVG, as drawing.js draws it.
func drawn(def *machine.Definition, run *Run) (template.HTML, error) {
	s, err := Draw(def, run)
	if err != nil {
		return "", err
	}
	return svgOf(s)
}

// unfolded gives a draft's drawing as SVG, with its cards unfolded.
func unfolded(draft *machine.Draft, u Unfold) (template.HTML, error) {
	s, err := Unfolded(draft, u)
	if err != nil {
		return "", err
	}
	return svgOf(s)
}

func svgOf(s Scene) (template.HTML, error) {
	l, err := laid(s)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	w, h := num(l.Width), num(l.Height)
	fmt.Fprintf(&b, `<svg class="machine" viewBox="%s 0 %s %s" width="%s" height="%s" role="img" aria-label="%s">`, num(l.Left), w, h, w, h, esc(l.Label))
	b.WriteString(`<defs><marker id="ah" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path class="arrow" d="M0 0L10 5L0 10z"/></marker>` +
		`<marker id="at" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto-start-reverse"><path class="arrow taken" d="M0 0L10 5L0 10z"/></marker></defs>`)
	l.parts.write(&b)
	b.WriteString(`</svg>`)
	return template.HTML(b.String()), nil
}

// laidOut is what layout.js gives: the box the drawing is in, and its parts.
type laidOut struct {
	Label  string
	Left   float64
	Width  float64
	Height float64
	parts
}

type parts struct {
	Holders []struct {
		Class      string
		X, Y, W, H float64
		Name       text
		Sub        *text
		Title      string
	}
	Edges []struct {
		Class string
		D     string
		Head  string
	}
	Chips []struct {
		Class       string
		X, Y, W     float64
		TX, TY      float64
		Text, Title string
	}
	Shapes []shape
}

type text struct {
	Class  string
	X      float64
	Y      float64
	Anchor string
	Text   string
}

type laidRect struct {
	Class      string
	X, Y, W, H float64
	RX         float64
}

type shape struct {
	Kind    string
	Class   string
	Href    string
	Behind  []laidRect
	Box     laidRect
	Title   text
	Subs    []text
	Notes   []text
	Tooltip string
	X, Y    float64
	D       string
	Text    string
	Tile    *struct {
		Icon  string
		X, Y  float64
		Spark string
	}
	Fold *text
	In   *struct {
		X, Y float64
		parts
	}
}

func (p parts) write(b *strings.Builder) {
	for _, h := range p.Holders {
		fmt.Fprintf(b, `<g class="%s"><rect x="%s" y="%s" width="%s" height="%s" rx="14"/><text class="name" x="%s" y="%s">%s</text>`, h.Class, num(h.X), num(h.Y), num(h.W), num(h.H), num(h.Name.X), num(h.Name.Y), esc(h.Name.Text))
		if h.Sub != nil {
			fmt.Fprintf(b, `<text class="sub" x="%s" y="%s">%s</text>`, num(h.Sub.X), num(h.Sub.Y), esc(h.Sub.Text))
		}
		if h.Title != "" {
			fmt.Fprintf(b, `<title>%s</title>`, esc(h.Title))
		}
		b.WriteString(`</g>`)
	}
	for _, e := range p.Edges {
		fmt.Fprintf(b, `<path class="%s" d="%s" marker-end="url(#%s)"/>`, esc(e.Class), e.D, e.Head)
	}
	for _, c := range p.Chips {
		title := ""
		if c.Title != "" {
			title = "<title>" + esc(c.Title) + "</title>"
		}
		fmt.Fprintf(b, `<rect class="%s" x="%s" y="%s" width="%s" height="18" rx="9"/><text class="on" x="%s" y="%s" text-anchor="middle">%s%s</text>`, esc(c.Class), num(c.X), num(c.Y), num(c.W), num(c.TX), num(c.TY), esc(c.Text), title)
	}
	for _, s := range p.Shapes {
		switch s.Kind {
		case "start":
			fmt.Fprintf(b, `<circle class="start" cx="%s" cy="%s" r="4"/><path class="edge" d="%s" marker-end="url(#ah)"/>`, num(s.X), num(s.Y), s.D)
		case "note":
			fmt.Fprintf(b, `<text class="note" x="%s" y="%s">%s</text>`, num(s.X), num(s.Y), esc(s.Text))
		case "tag":
			fmt.Fprintf(b, `<text class="tag" x="%s" y="%s" text-anchor="start">%s</text>`, num(s.X), num(s.Y), esc(s.Text))
		case "card":
			s.write(b)
		}
	}
}

func (s shape) write(b *strings.Builder) {
	if s.Href != "" {
		fmt.Fprintf(b, `<a href="%s">`, esc(s.Href))
	}
	fmt.Fprintf(b, `<g class="%s">`, s.Class)
	for _, r := range s.Behind {
		fmt.Fprintf(b, `<rect class="%s" x="%s" y="%s" width="%s" height="%s" rx="%s"/>`, r.Class, num(r.X), num(r.Y), num(r.W), num(r.H), num(r.RX))
	}
	x := s.Box
	fmt.Fprintf(b, `<rect class="box" x="%s" y="%s" width="%s" height="%s" rx="%s"/><text class="title" x="%s" y="%s" text-anchor="%s">%s</text>`,
		num(x.X), num(x.Y), num(x.W), num(x.H), num(x.RX), num(s.Title.X), num(s.Title.Y), s.Title.Anchor, esc(s.Title.Text))
	for _, t := range s.Subs {
		fmt.Fprintf(b, `<text class="sub" x="%s" y="%s" text-anchor="%s">%s</text>`, num(t.X), num(t.Y), t.Anchor, esc(t.Text))
	}
	for _, t := range s.Notes {
		fmt.Fprintf(b, `<text class="%s" x="%s" y="%s" text-anchor="%s">%s</text>`, t.Class, num(t.X), num(t.Y), t.Anchor, esc(t.Text))
	}
	if t := s.Tile; t != nil {
		fmt.Fprintf(b, `<rect class="tile %s" x="%s" y="%s" width="28" height="28" rx="7"/>`, t.Icon, num(t.X), num(t.Y))
		if t.Spark != "" {
			fmt.Fprintf(b, `<path class="spark" d="%s"/>`, t.Spark)
		} else {
			fmt.Fprintf(b, `<use class="icon" href="#g-%s" x="%s" y="%s" width="20" height="14"/>`, t.Icon, num(t.X+4), num(t.Y+7))
		}
	}
	if f := s.Fold; f != nil {
		fmt.Fprintf(b, `<text class="fold" x="%s" y="%s" text-anchor="middle">%s</text>`, num(f.X), num(f.Y), f.Text)
	}
	if s.Tooltip != "" {
		fmt.Fprintf(b, `<title>%s</title>`, esc(s.Tooltip))
	}
	b.WriteString(`</g>`)
	if s.Href != "" {
		b.WriteString(`</a>`)
	}
	if s.In != nil {
		fmt.Fprintf(b, `<g transform="translate(%s %s)">`, num(s.In.X), num(s.In.Y))
		s.In.parts.write(b)
		b.WriteString(`</g>`)
	}
}

func num(f float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}

func esc(s string) string { return template.HTMLEscapeString(s) }
