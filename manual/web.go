package manual

import (
	"embed"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// What a browser is given beside the pages: the layout of a page and its
// styles.
//
//go:embed web
var site embed.FS

var layout = template.Must(template.New("page.html").Funcs(template.FuncMap{"title": title}).ParseFS(site, "web/page.html"))

// marked is what a sentence sets apart as a command, a field or a name.
var marked = regexp.MustCompile("`([^`]+)`")

// item is the first line of an entry in a list, and numbered that of a
// numbered paragraph.
var (
	item     = regexp.MustCompile(`^(- |[0-9]+\. )`)
	numbered = regexp.MustCompile(`^[0-9]+\. `)
)

// point is the first line of a point for or against, under an entry of a
// list.
var point = regexp.MustCompile(`^(✓|✗) `)

// named is where a text names a page, and nameWords the words after it
// that may be the page's name.
var (
	named     = regexp.MustCompile(`corbie manual `)
	nameWords = regexp.MustCompile(`^[a-z][a-z0-9-]*( [a-z][a-z0-9-]*)*`)
)

// A mention is a page a text names, as corbie manual and the page's name:
// text[at:end].
type mention struct {
	page    Page
	name    string
	at, end int
}

// mentions gives each page a text names. A command's page has a name of
// several words, such as run show, so of the words after corbie manual the
// most that name a page are taken.
func mentions(text string, known func(string) (Page, bool)) []mention {
	var found []mention
	for _, at := range named.FindAllStringIndex(text, -1) {
		// A name that begins inside the one before it is part of that one.
		if n := len(found); n > 0 && at[0] < found[n-1].end {
			continue
		}
		name := nameWords.FindString(text[at[1]:])
		for name != "" {
			if page, ok := known(name); ok {
				found = append(found, mention{page, name, at[0], at[1] + len(name)})
				break
			}
			last := strings.LastIndex(name, " ")
			if last < 0 {
				break
			}
			name = name[:last]
		}
	}
	return found
}

// A group is the pages of one kind, as the list shows them together. The
// long groups of the reference are folded away until one is opened.
type group struct {
	Name   string
	Pages  []Page
	Folded bool
	Open   bool
}

// A part is the pages a reader takes together: the lessons, setting up, the
// tasks, how it works, and the reference. First is the page a part opens at.
type part struct {
	Name, First string
	Says        string // what the part is for, in a sentence
	Groups      []group
}

// What each part is for, as the first page says it.
var says = map[string]string{
	"Start":        "Five short pages go from what Corbie is to a run on a controller.",
	"Setting up":   "You get Corbie, choose where it runs, and check that it works.",
	"Tasks":        "Each of these pages shows how to do one particular thing.",
	"How it works": "These pages follow a real run through the store, for a reader who wants to see how Corbie works inside.",
	"Reference":    "Every field, plugin and command is described in full. Your assistant reads these pages.",
}

// partOf gives the part a group of pages belongs to.
func partOf(group string) string {
	switch group {
	case "Start", "Setting up", "Tasks", "How it works":
		return group
	case "Where it runs":
		return "Setting up"
	}
	return "Reference"
}

// covers is the sentence a page's first line gives after its title.
func covers(page Page) string {
	_, after, _ := strings.Cut(page.Summary, ": ")
	first, size := utf8.DecodeRuneInString(after)
	return string(unicode.ToUpper(first)) + after[size:]
}

// Title is what a page is called where a reader sees it named.
func (p Page) Title() string { return title(p) }

// title is what a page is called where pages are listed for a reader: what
// its first line gives as its title, or its name.
func title(page Page) string {
	before, _, ok := strings.Cut(page.Summary, ": ")
	if first, _ := utf8.DecodeRuneInString(before); ok && (unicode.IsUpper(first) || before == page.Name) {
		return before
	}
	return page.Name
}

// Site serves the manual to a browser: each page under its name, as the
// text that is printed, and the first page at /.
func Site() http.Handler {
	files := http.FileServerFS(site)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasPrefix(name, "web/") {
			files.ServeHTTP(w, r)
			return
		}
		pages := Pages()
		var shown struct {
			Parts      []part // every page, part by part
			Part       string // the part the page asked for is in
			Groups     []group
			Page       *Page
			Title      string
			Lede       template.HTML
			Text       template.HTML
			Prev, Next *Page
		}
		at := -1
		for i, page := range pages {
			if page.Name == name || page.Name == "corbie "+name {
				at = i
			}
			of := partOf(page.Group)
			if len(shown.Parts) == 0 || shown.Parts[len(shown.Parts)-1].Name != of {
				shown.Parts = append(shown.Parts, part{Name: of, First: page.Name, Says: says[of]})
			}
			in := &shown.Parts[len(shown.Parts)-1]
			if n := len(in.Groups); n == 0 || in.Groups[n-1].Name != page.Group {
				in.Groups = append(in.Groups, group{Name: page.Group, Folded: slices.Contains([]string{"Plugins", "Libraries", "Commands"}, page.Group)})
			}
			last := &in.Groups[len(in.Groups)-1]
			last.Pages = append(last.Pages, page)
		}
		if name == "" {
			at = 0
		}
		if at < 0 {
			http.NotFound(w, r)
			return
		}
		{
			page := pages[at]
			shown.Page, shown.Part = &page, partOf(page.Group)
			// The side lists the pages of the part the page is in.
			for _, in := range shown.Parts {
				if in.Name == shown.Part {
					shown.Groups = slices.Clone(in.Groups)
				}
			}
			for i := range shown.Groups {
				shown.Groups[i].Open = shown.Groups[i].Name == page.Group
			}
			// The guide and how it works are each read in order.
			if shown.Part != "Reference" {
				if at > 0 && partOf(pages[at-1].Group) == shown.Part {
					shown.Prev = &pages[at-1]
				}
				if at+1 < len(pages) && partOf(pages[at+1].Group) == shown.Part {
					shown.Next = &pages[at+1]
				}
			}
			var lede string
			shown.Title, lede, shown.Text = set(page, Find)
			// What is between backticks in a page's first line is set apart
			// as it is in its text.
			shown.Lede = template.HTML(marked.ReplaceAllString(html.EscapeString(lede), "<code>$1</code>"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := layout.Execute(w, shown); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// fileNamed is the line of an example that names a file.
var fileNamed = regexp.MustCompile(`^(# |// )([\w./-]+\.[a-z]+)$`)

// example gives one example as its parts, each shown as what it is: a file
// under its name, and the commands with what each prints.
func example(lines []string, inline func(string) string) string {
	var b strings.Builder
	var part []string
	kind := "" // "file" or "run"
	flush := func() {
		for len(part) > 0 && strings.TrimSpace(part[len(part)-1]) == "" {
			part = part[:len(part)-1]
		}
		if len(part) == 0 {
			return
		}
		if kind == "run" {
			b.WriteString(`<div class="eg run"><pre>`)
			for _, line := range part {
				class := "out"
				if strings.HasPrefix(line, "corbie ") {
					class = "cmd"
				}
				b.WriteString(`<span class="` + class + `">` + inline(line) + `</span>`)
			}
			b.WriteString("</pre></div>\n")
		} else if name := fileNamed.FindStringSubmatch(part[0]); name != nil {
			lang := "yaml"
			if strings.HasSuffix(name[2], ".js") {
				lang = "js"
			}
			b.WriteString(`<div class="eg" data-name="` + name[2] + `" data-lang="` + lang + `"><p class="name"><span class="mark">` + name[1] + `</span>` + name[2] + `</p><pre>` +
				inline(strings.TrimLeft(strings.Join(part[1:], "\n"), "\n")) + "</pre></div>\n")
		} else {
			b.WriteString(`<div class="eg"><pre>` + inline(strings.Join(part, "\n")) + "</pre></div>\n")
		}
		part = nil
	}
	for _, line := range lines {
		switch {
		case fileNamed.MatchString(line):
			flush()
			kind = "file"
		case strings.HasPrefix(line, "corbie "):
			if kind != "run" {
				flush()
			}
			kind = "run"
		case kind == "run" && strings.TrimSpace(line) != "" && !strings.HasPrefix(line, " "):
			flush()
			kind = "file"
		}
		part = append(part, line)
	}
	flush()
	return b.String()
}

// gap is the space between the columns of a row.
var gap = regexp.MustCompile(`\S( {2,})\S`)

// pairs reads rows as two columns: a term, and what is said of it, which
// may run on to further lines under where it began. It gives nil for rows
// that are not written so: a table of more columns, or lines of commands.
func pairs(block []string) [][2]string {
	var rows [][2]string
	base, at := -1, -1
	for _, line := range block {
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if base < 0 {
			base = indent
		}
		// A line that begins where the second column does runs the row on.
		if at >= 0 && indent == at && len(rows) > 0 {
			rows[len(rows)-1][1] += " " + strings.TrimSpace(line)
			continue
		}
		found := gap.FindStringSubmatchIndex(line[indent:])
		if found == nil || indent < base {
			return nil
		}
		second := indent + found[3]
		if at < 0 {
			at = second
		}
		term, said := line[base:indent+found[2]], line[second:]
		if second != at || gap.MatchString(said) {
			return nil
		}
		rows = append(rows, [2]string{term, said})
	}
	if len(rows) < 2 {
		return nil
	}
	return rows
}

// set gives a page's text as a browser shows it. The text is the text that
// is printed, and each of its shapes is shown as what it is: the first line
// as the title and what the page covers, a line with a rule under it as a
// heading, lines set in by four spaces as an example, part by part, lines
// set in by less as rows that keep their columns, and the rest as
// paragraphs. Under an entry of a list, a line that begins with a tick or a
// cross is a point for or against. In a paragraph, what stands between
// backticks is shown as a command, a field or a name. A page another names,
// by the command that prints it, is shown by its title and is a way to it;
// rows that each begin with such a page are a list of pages.
func set(page Page, known func(string) (Page, bool)) (title, lede string, body template.HTML) {
	lines := strings.Split(strings.TrimRight(page.Text, "\n"), "\n")
	title = page.Name
	if _, _, ok := strings.Cut(lines[0], ": "); ok && (len(lines) == 1 || lines[1] == "") {
		title, lede, lines = strings.SplitN(lines[0], ": ", 2)[0], covers(page), lines[1:]
	}
	// A page another names is a way to it, in an example as in a sentence,
	// and is shown by its title.
	inline := func(text string) string {
		text = html.EscapeString(text)
		var b strings.Builder
		from := 0
		for _, to := range mentions(text, known) {
			b.WriteString(text[from:to.at] + `<a class="to" href="` + url.PathEscape(to.name) + `">` + html.EscapeString(to.page.Title()) + `</a>`)
			from = to.end
		}
		return b.String() + text[from:]
	}
	// In a sentence, what is written between backticks is a command, a
	// field or a name, and is set apart from the words around it.
	prose := func(text string) string { return marked.ReplaceAllString(inline(text), "<code>$1</code>") }
	var b strings.Builder
	for i := 0; i < len(lines); {
		line := lines[i]
		// A block runs to the line that ends it.
		run := func(more func(string) bool) []string {
			from := i
			for i++; i < len(lines) && more(lines[i]); i++ {
			}
			return lines[from:i]
		}
		blank := func(l string) bool { return strings.TrimSpace(l) == "" }
		switch {
		case blank(line):
			i++
		case i+1 < len(lines) && len(lines[i+1]) >= 3 && strings.Trim(lines[i+1], "-") == "" && !strings.HasPrefix(line, " "):
			b.WriteString("<h2>" + prose(line) + "</h2>\n")
			i += 2
		case item.MatchString(line):
			// Lines that begin with a dash are a list, and numbered
			// paragraphs a numbered one.
			list := "ul"
			if numbered.MatchString(line) {
				list = "ol"
			}
			b.WriteString("<" + list + ">\n")
			for i < len(lines) && item.MatchString(lines[i]) && numbered.MatchString(lines[i]) == (list == "ol") {
				one := slices.Clone(run(func(l string) bool { return strings.HasPrefix(l, "  ") }))
				one[0] = item.ReplaceAllString(one[0], "")
				// Under an entry, a line that begins with a tick or a
				// cross is a point for or against what the entry says.
				says, points := "", []string{}
				for _, l := range one {
					l = strings.TrimSpace(l)
					switch {
					case point.MatchString(l):
						points = append(points, l)
					case len(points) > 0:
						points[len(points)-1] += " " + l
					default:
						says += " " + l
					}
				}
				b.WriteString("<li>" + prose(strings.TrimSpace(says)))
				if len(points) > 0 {
					b.WriteString(`<ul class="points">`)
					for _, p := range points {
						sign, text, _ := strings.Cut(p, " ")
						class := "for"
						if sign == "✗" {
							class = "against"
						}
						b.WriteString(`<li class="` + class + `"><span>` + sign + `</span> ` + prose(text) + "</li>")
					}
					b.WriteString("</ul>")
				}
				b.WriteString("</li>\n")
				for i+1 < len(lines) && blank(lines[i]) && item.MatchString(lines[i+1]) {
					i++
				}
			}
			b.WriteString("</" + list + ">\n")
		case strings.HasPrefix(line, "    "):
			block := slices.Clone(run(func(l string) bool { return blank(l) || strings.HasPrefix(l, "    ") }))
			for blank(block[len(block)-1]) {
				block = block[:len(block)-1]
			}
			for n, l := range block {
				block[n] = strings.TrimPrefix(l, "    ")
			}
			b.WriteString(example(block, inline))
		case strings.HasPrefix(line, " "):
			block := run(func(l string) bool { return !blank(l) })
			// Rows that each begin with a page are a list of pages, each
			// with what the row says of it.
			var pages []string
			for _, l := range block {
				to := mentions(l, known)
				switch {
				case len(to) > 0 && strings.TrimSpace(l[:to[0].at]) == "":
					pages = append(pages, strings.TrimSpace(l))
				case len(pages) > 0 && len(to) == 0:
					pages[len(pages)-1] += " " + strings.TrimSpace(l)
				default:
					pages = nil
				}
				if pages == nil {
					break
				}
			}
			if pages != nil {
				b.WriteString(`<ul class="pages">` + "\n")
				for _, row := range pages {
					end := mentions(row, known)[0].end
					b.WriteString("<li>" + inline(row[:end]) + " <span>" + prose(strings.TrimSpace(row[end:])) + "</span></li>\n")
				}
				b.WriteString("</ul>\n")
				break
			}
			// Rows of two columns, a term and what is said of it, are set
			// as pairs: the term as it is written, and the words beside it
			// as words.
			if rows := pairs(block); rows != nil {
				b.WriteString(`<div class="pairs">` + "\n")
				for _, row := range rows {
					b.WriteString("<code>" + inline(row[0]) + "</code><span>" + prose(row[1]) + "</span>\n")
				}
				b.WriteString("</div>\n")
				break
			}
			b.WriteString(`<pre class="rows">` + inline(strings.Join(block, "\n")) + "</pre>\n")
		default:
			block := run(func(l string) bool { return !blank(l) && !strings.HasPrefix(l, " ") })
			b.WriteString("<p>" + prose(strings.Join(block, " ")) + "</p>\n")
		}
	}
	return title, lede, template.HTML(b.String())
}
