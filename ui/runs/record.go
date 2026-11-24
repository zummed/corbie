package runs

import (
	"cmp"
	"encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zummed/corbie/client"
	"github.com/zummed/corbie/instance"
	"github.com/zummed/corbie/machine"
	"github.com/zummed/corbie/ui/page"
)

// A line is one line of a run's record, in the words `corbie run show
// --record` prints: when, which run by its machine, the step, which way it
// went, and what. The line a run ends with has its status.
type line struct {
	Time    string
	Machine string
	Step    int
	Way     string // "in" for what arrived, "out" for what the step asked
	Text    string
	End     string
}

// lines gives an entry of a record as Corbie's command line prints it,
// which it keeps to itself, with the run named as name, and its time as
// the steps of a run that started on day show it.
func lines(e client.Entry, name string, day time.Time) []line {
	at := page.Clock(e.Time, day)
	if e.Step == 0 {
		data, _ := json.Marshal(e.Data, json.Deterministic(true))
		text := string(data)
		if s, ok := e.Data.(string); ok {
			text = s
		}
		return []line{{Time: at, Machine: name, Text: fmt.Sprintf("%s %s: %s", e.Job, e.Name, text)}}
	}
	var out []line
	add := func(way, text string) {
		out = append(out, line{Time: at, Machine: name, Step: e.Step, Way: way, Text: text})
	}
	for _, event := range e.Events {
		text := event.Name
		switch from := event.Sender; {
		case from != nil && from.Client != "":
			text += " from client:" + from.Client
		case from != nil:
			text += " from " + senderOf(event.Event) + " " + from.Run
		case event.From != "":
			text += " from " + event.From
		}
		if failed, _ := event.Data.(map[string]any); failed["refused"] != nil {
			text += fmt.Sprintf(" denied: %v", failed["refused"])
		} else if event.Data != nil {
			data, _ := json.Marshal(event.Data, json.Deterministic(true))
			text += " " + clip(string(data))
		}
		add("in", text)
	}
	for _, ask := range e.Asks {
		text := ""
		switch ask.Kind {
		case instance.AskScript:
			text = fmt.Sprintf("job %s: a script, on %s", ask.Name, ask.Runner)
		case instance.AskChild:
			text = fmt.Sprintf("job %s: a machine, %s", ask.Name, ask.Child)
		case instance.AskCancel:
			text = "cancel " + ask.Name
		case instance.AskEvent:
			to := "its parent"
			switch {
			case ask.Location != "":
				to = sentTo(&ask)
			case ask.Client != "":
				to = "client:" + ask.Client
			case ask.Direct:
				to = ask.To
			case ask.Name != "":
				to = ask.Name
			}
			text = fmt.Sprintf("send %s to %s", ask.Event.Name, to)
			switch {
			case ask.Undelivered:
				text += " · undelivered: " + cmp.Or(ask.Text, "it is not running")
			case ask.Run != "":
				text += " · started " + ask.Run
			case ask.Location != "":
				text += " · " + ask.To
			}
		case instance.AskFail:
			text = fmt.Sprintf("%s fails: %s", ask.Name, ask.Error.Error)
			if ask.Error.Refused != "" {
				text = fmt.Sprintf("%s fails, denied: %s", ask.Name, ask.Error.Refused)
			}
		case instance.AskHold:
			text = fmt.Sprintf("hold %s for its hooks", ask.Name)
		case instance.AskNote:
			text = ask.Text
		default:
			continue
		}
		add("out", text)
	}
	if e.Status != instance.Running {
		add("", e.Status)
		out[len(out)-1].End = e.Status
	}
	return out
}

func clip(text string) string {
	if len(text) > 200 {
		return text[:200] + "…"
	}
	return text
}

// senderOf gives the address of the run or client that sent an event, as
// the command line writes it, or nothing for an event from within the run.
func senderOf(ev machine.Event) string {
	from := ev.Sender
	switch {
	case from == nil:
		return ""
	case from.Client != "":
		return "client:" + from.Client
	case from.Key != nil:
		return from.At + "#" + *from.Key
	}
	return from.At
}

// sentTo gives the address a send went to: its location, with #key where it
// went to a key.
func sentTo(a *instance.Ask) string {
	if a.Key != "" {
		return a.Location + "#" + a.Key
	}
	return a.Location
}

// A stepRow is one thing that happened in a run, as Steps lists it: when,
// which run, and what, in words. A row that started a child holds the
// child's rows, and a send links to the run it reached. A send or a call to
// a run in a tree of the UI's client has a trace, whose rows are read when
// its group is opened.
type stepRow struct {
	Key   string
	Run   link
	Step  int
	At    time.Time
	Time  string
	Exact string
	Depth int
	What  []word
	// Kind colours a row: send and got for a send between runs, with
	// undelivered for a send that was not, held, failed, and the status a
	// run ended with.
	Kind  string
	Group *group
	Trace *trace
	// Job is the job the row is about, by the name its definition gives
	// it, and Took how long it took, where the row is its end. Full is the
	// job's name as its ask gave it, with its ask's and attempt's numbers,
	// under which its runner files what it logs.
	Job, Took string
	Full      string `json:"-"`
	// Cards are the cards of the page's drawing the row is about, each by
	// its path: the names of the cards from the page's run down to it.
	Cards []string
	// What the row's details show: its facts, and what went in and came
	// out. In and Out reach the page as the details' Input and Output.
	Facts   []fact
	In, Out any `json:"-"`
}

// A word is part of what a row says; a name is bold, and the run a send
// went to or came from is a link to it.
type word struct {
	Text string
	Bold bool
	Run  *link
}

// A fact is one line of a row's details, with the run it names.
type fact struct {
	Name, Text string
	Run        *link
}

// A group is a child run under the row that started it: its status, how
// many steps it and the runs under it took, how long it took, and its
// rows.
type group struct {
	Run             link
	Status, Failure string
	Ended           bool
	Steps           int
	Took            string
	Rows            []stepRow
}

// A trace is the run a send or a call reached in a tree of the UI's
// client, as the row's group shows it before its rows are read: its status
// and how long it took. A send's trace names its cause: the sending run and
// the step that sent it.
type trace struct {
	Run             link
	Status, Failure string
	Ended           bool
	Took            string
	Call            bool
	Sent            instance.Cause
}

// Unix is when a row happened, as the page orders rows.
func (r stepRow) Unix() int64 { return r.At.UnixMilli() }

// Said is what a row says, as plain words.
func (r stepRow) Said() string {
	var b strings.Builder
	for _, w := range r.What {
		b.WriteString(w.Text)
	}
	return b.String()
}

// linkTo is a link to a run: by its machine, with its title and the
// tooltip of its name.
func linkTo(r *client.Run) link {
	return link{ID: r.ID, Machine: r.Machine, Title: r.Title, Tip: tipOf(r), Type: r.Type}
}

// linkOf is a run as a row names it: by its machine where the list holds
// it, and by the end of its ID where it does not.
func linkOf(id string, byID map[string]*client.Run) link {
	if r := byID[id]; r != nil {
		return linkTo(r)
	}
	return link{ID: id, Machine: id[strings.LastIndexByte(id, '-')+1:], Tip: id}
}

// rowsOf gives a run's rows, from its record and its children's, each
// child's under the row that started it. A send or a call to a run that
// readable says the UI may read has a trace. A row's time has its date
// where it is not on day's date.
func rowsOf(id string, entries []client.Entry, byID map[string]*client.Run, readable func(string) bool, now, day time.Time) []stepRow {
	return rowsAt(id, entries, byID, readable, now, day, 0)
}

// rowsAt gives a run's rows as rowsOf does, the run's own at depth.
func rowsAt(id string, entries []client.Entry, byID map[string]*client.Run, readable func(string) bool, now, day time.Time, depth int) []stepRow {
	of := map[string][]client.Entry{}
	for _, e := range entries {
		if e.Step > 0 {
			of[e.ID] = append(of[e.ID], e)
		}
	}
	b := &rowBuilder{of: of, byID: byID, readable: readable, top: id, now: now, day: day, notes: notesOf(entries)}
	return b.rows(id, depth, "")
}

// cardPath gives the path of a card below the card a run is drawn in.
func cardPath(in, name string) string {
	if in == "" {
		return name
	}
	return in + "/" + name
}

// A sendNote is what the controller noted in a sender's record of an event
// it sent to an address: why it was not delivered, or the run it went to in
// place of the run the step named.
type sendNote struct {
	undelivered, movedTo string
}

// notesOf gives the controller's notes of events sent to addresses, by the
// message that sent each. A note's message is named for the message it is
// about: <message>.note, or <message>.moved.<the end of the run's ID>.
func notesOf(entries []client.Entry) map[string]sendNote {
	notes := map[string]sendNote{}
	for _, e := range entries {
		for _, ev := range e.Events {
			if !ev.Own || ev.Name != instance.KindNote {
				continue
			}
			name := ev.Source[strings.LastIndexByte(ev.Source, '/')+1:]
			text, _ := ev.Data.(string)
			if message, ok := strings.CutSuffix(name, ".note"); ok {
				n := notes[message]
				_, n.undelivered, _ = strings.Cut(text, " was not delivered: ")
				n.undelivered = cmp.Or(n.undelivered, text)
				notes[message] = n
			} else if message, tail, ok := strings.Cut(name, ".moved."); ok {
				n := notes[message]
				n.movedTo = tail
				notes[message] = n
			}
		}
	}
	return notes
}

// noted says whether a note in a run's record is about an event the run
// sent to an address, which the row of that send says.
func (b *rowBuilder) noted(e client.Entry, text string) bool {
	for _, ev := range e.Events {
		name := ev.Source[strings.LastIndexByte(ev.Source, '/')+1:]
		if ev.Own && ev.Name == instance.KindNote && ev.Data == text && (strings.HasSuffix(name, ".note") || strings.Contains(name, ".moved.")) {
			return true
		}
	}
	return false
}

type rowBuilder struct {
	of       map[string][]client.Entry
	byID     map[string]*client.Run
	readable func(string) bool
	// top is the run whose rows these are, which has a row for its start.
	top      string
	now, day time.Time
	notes    map[string]sendNote
}

// steps counts the steps of a run and of every run under it.
func (b *rowBuilder) steps(id string) int {
	n := len(b.of[id])
	for _, e := range b.of[id] {
		for _, a := range e.Asks {
			if a.Kind == instance.AskChild && a.Child != "" {
				n += b.steps(a.Child)
			}
		}
	}
	return n
}

// rows gives a run's rows, the run drawn in the card at the path in, none
// for the page's run.
func (b *rowBuilder) rows(id string, depth int, in string) []stepRow {
	run := linkOf(id, b.byID)
	own := b.of[id]
	var out []stepRow
	var prev machine.Shown
	jobs := map[string]*instance.Ask{} // the latest ask that started each job, by its name
	began := map[string]time.Time{}    // when the latest ask of each job was made, by its name
	// ended fills in, for a row about a job that ended at step e, how long
	// the job took.
	ended := func(r *stepRow, e client.Entry, job string) {
		r.Job, r.Full = jobOf(job), job
		if at, ok := began[named(job)]; ok {
			r.Took = took(e.Time.Sub(at))
		}
	}
	for i, e := range own {
		n := 0
		add := func(r stepRow) *stepRow {
			if r.Key == "" {
				r.Key = e.ID + "/" + strconv.Itoa(e.Step) + "/" + strconv.Itoa(n)
				n++
			}
			for k, c := range r.Cards {
				r.Cards[k] = cardPath(in, c)
			}
			r.Run, r.Step, r.At, r.Time, r.Depth = run, e.Step, e.Time, page.Clock(e.Time, b.day), depth
			r.Exact = e.Time.Local().Format("2 Jan 2006 15:04:05.000")
			out = append(out, r)
			return &out[len(out)-1]
		}
		if e.Step == 1 && id == b.top {
			r := stepRow{Key: e.ID + "/started", What: say("started")}
			if h := b.byID[id]; h != nil && h.Cause.Run != "" && h.Cause.Run != h.Parent {
				by := linkOf(h.Cause.Run, b.byID)
				if s := b.byID[h.Cause.Run]; s != nil {
					by.Machine = cmp.Or(addressOf(s, ""), s.Location, s.Machine)
				}
				r.What = say("started by ", linked(&by))
				r.Facts = []fact{{Name: "cause", Text: h.Cause.Run + " step " + strconv.Itoa(h.Cause.Step), Run: &by}}
			}
			for _, ev := range e.Events {
				if ev.Name == machine.Start {
					r.In = ev.Data
				}
			}
			if h := b.byID[id]; h != nil {
				r.Out = h.Output
			}
			add(r)
		}
		for _, ev := range e.Events {
			switch {
			case ev.Name == machine.Start:
			case ev.Sender != nil:
				add(b.gotOf(ev, e))
			case ev.Name == machine.Timer:
				r := stepRow{What: say("timer ", bold(jobOf(ev.From)), " fired"), Facts: []fact{{Name: "timer", Text: jobOf(ev.From)}}}
				if s, _ := ev.Data.(string); s != "" {
					if due, err := time.Parse(time.RFC3339Nano, s); err == nil {
						r.Facts = append(r.Facts, fact{Name: "due", Text: page.When(due, b.now)})
					}
				}
				add(r)
			case ev.Answer:
				add(stepRow{What: say("⇠ ", bold(ev.Name), " answered"), Kind: "got", In: ev.Data, Facts: []fact{{Name: "ticket", Text: ev.Name}}})
			case ev.Name == machine.Failed && run.Type != "flow":
				r := stepRow{What: say(bold(jobOf(ev.From)), " failed"), Kind: "failed", Out: ev.Data, Facts: jobFacts(jobs[named(ev.From)], b.byID), Cards: []string{jobOf(ev.From)}}
				if a := jobs[named(ev.From)]; a != nil && a.Job != nil {
					r.In = a.Job.Input
				}
				r.Facts = append(r.Facts, failed(ev.Data)...)
				ended(&r, e, ev.From)
				add(r)
			case run.Type == "agent" && ev.Name == machine.Done && isTurn(ev.From):
				turn := prev.Turn
				data, _ := ev.Data.(map[string]any)
				calls, _ := data["calls"].([]any)
				for _, c := range calls {
					call, _ := c.(map[string]any)
					name, _ := call["name"].(string)
					add(stepRow{What: say("turn "+strconv.Itoa(turn)+" called ", bold(name), "("+argsOf(call["args"])+")"), In: call["args"], Out: b.resultOf(own[i+1:], name), Cards: []string{name},
						Facts: []fact{{Name: "tool", Text: name}, {Name: "args", Text: compact(call["args"])}, {Name: "turn " + strconv.Itoa(turn), Text: turnFacts(data)}}})
				}
				if len(calls) == 0 {
					text, _ := data["text"].(string)
					add(stepRow{What: say("turn "+strconv.Itoa(turn)+" replied ", clip(text)), Out: ev.Data,
						Facts: []fact{{Name: "turn " + strconv.Itoa(turn), Text: turnFacts(data)}}})
				}
			case run.Type == "agent" && ev.Name == machine.Done:
				r := stepRow{What: say(bold(jobOf(ev.From)), " done"), Out: ev.Data, Facts: jobFacts(jobs[named(ev.From)], b.byID), Cards: []string{jobOf(ev.From)}}
				if a := jobs[named(ev.From)]; a != nil && a.Job != nil {
					r.In = a.Job.Input
				}
				ended(&r, e, ev.From)
				add(r)
			}
		}
		switch run.Type {
		case "statechart":
			was, now := prev.States, e.Shown.States
			left := slices.DeleteFunc(slices.Clone(was), func(s string) bool { return slices.Contains(now, s) })
			entered := slices.DeleteFunc(slices.Clone(now), func(s string) bool { return slices.Contains(was, s) })
			var w []word
			if len(left) > 0 {
				w = append(say("left "), names(left)...)
			}
			if len(entered) > 0 {
				if w != nil {
					w = append(w, word{Text: ", "})
				}
				w = append(append(w, word{Text: "entered "}), names(entered)...)
			}
			if w != nil {
				r := stepRow{What: w, Facts: []fact{{Name: "states", Text: strings.Join(now, ", ")}}, Cards: append(slices.Clone(left), entered...)}
				// The row is about the job that ended at the step, or else the
				// job of the state it entered.
				for _, ev := range e.Events {
					if ev.From != "" && (ev.Name == machine.Done || ev.Name == machine.Failed) && slices.Contains(left, jobOf(ev.From)) {
						ended(&r, e, ev.From)
					}
				}
				for _, a := range e.Asks {
					if r.Job == "" && (a.Kind == instance.AskScript || a.Kind == instance.AskChild) && slices.Contains(entered, jobOf(a.Name)) {
						r.Job, r.Full = jobOf(a.Name), a.Name
					}
				}
				add(r)
			}
		case "flow":
			was, now := prev.Stages, e.Shown.Stages
			for _, s := range slices.Sorted(maps.Keys(now)) {
				st := now[s]
				if st == was[s] || (st != instance.Done && st != instance.Failed && st != "skipped") {
					continue
				}
				r := stepRow{What: say("stage ", bold(s), " "+st), Facts: []fact{{Name: "stage", Text: s}, {Name: "status", Text: st}}, Cards: []string{s}, Job: s}
				if st == instance.Failed {
					r.Kind = "failed"
				}
				for _, ev := range e.Events {
					if jobOf(ev.From) == s && (ev.Name == machine.Done || ev.Name == machine.Failed) {
						ended(&r, e, ev.From)
						r.Out = ev.Data
						if ev.Name == machine.Failed {
							r.Facts = append(r.Facts, failed(ev.Data)...)
						}
						if a := jobs[named(ev.From)]; a != nil && a.Job != nil {
							r.In = a.Job.Input
						}
					}
				}
				add(r)
			}
		}
		for k := range e.Asks {
			a := &e.Asks[k]
			if a.Name != "" && (a.Kind == instance.AskScript || a.Kind == instance.AskChild || a.Kind == instance.AskHold || a.Kind == instance.AskCall) {
				jobs[named(a.Name)] = a
			}
			if a.Name != "" && (a.Kind == instance.AskScript || a.Kind == instance.AskChild || a.Kind == instance.AskCall) {
				began[named(a.Name)] = e.Time
			}
			switch a.Kind {
			case instance.AskChild:
				child := linkOf(a.Child, b.byID)
				name := jobOf(a.Name)
				var w []word
				switch {
				case a.Job != nil && a.Job.Hook != "":
					w = say("hook ", bold(a.Job.Hook), " started ", bold(child.Machine))
				case run.Type == "flow":
					w = say("stage ", bold(name), " started ", bold(child.Machine))
				case run.Type == "statechart":
					w = say("state ", bold(name), " started ", bold(child.Machine))
				default:
					w = say(bold(name), " started ", bold(child.Machine))
				}
				r := stepRow{What: w, Facts: jobFacts(a, b.byID), Out: b.resultOf(own[i+1:], a.Name), Job: name, Full: a.Name, Cards: []string{name}}
				if a.Job != nil {
					r.In = a.Job.Input
				}
				g := &group{Run: child, Steps: b.steps(a.Child), Rows: b.rows(a.Child, depth+1, cardPath(in, name))}
				if h := b.byID[a.Child]; h != nil {
					g.Status, g.Failure, g.Ended = h.Status, h.Failure, h.HasEnded()
					if g.Ended {
						g.Took = took(h.Ended.Sub(h.Started))
					}
				}
				r.Group = g
				add(r)
			case instance.AskCall:
				called := linkOf(a.Run, b.byID)
				r := stepRow{Kind: "send", What: say("⇢ called ", bold(a.Location), " · ", linked(&called)),
					Facts: append(jobFacts(a, b.byID), fact{Name: "location", Text: a.Location}), Out: b.resultOf(own[i+1:], a.Name), Job: jobOf(a.Name), Full: a.Name, Cards: []string{jobOf(a.Name)}}
				if a.Job != nil {
					r.In = a.Job.Input
				}
				r.Trace = b.traceOf(called, &trace{Call: true})
				add(r)
			case instance.AskScript:
				if run.Type == "agent" && isTurn(a.Name) {
					turn, model := strconv.Itoa(e.Shown.Turn), modelOf(a.Job)
					add(stepRow{What: say("turn "+turn+" to ", bold(model)), Facts: []fact{{Name: "turn", Text: turn}, {Name: "model", Text: model}, {Name: "runner", Text: a.Runner}}})
				}
			case instance.AskWake:
				if name, ok := strings.CutPrefix(a.What, "timer."); ok {
					name = jobOf(name)
					add(stepRow{What: say("timer ", bold(name), " set, due "+page.When(a.At, b.now)), Facts: []fact{{Name: "timer", Text: name}, {Name: "due", Text: page.When(a.At, b.now)}}})
				}
			case instance.AskEvent:
				r := b.sendOf(a)
				if r.Trace != nil {
					r.Trace.Sent = instance.Cause{Run: id, Step: e.Step}
				}
				add(r)
			case instance.AskHold:
				// The hook the job is held for is the one the same step started
				// for it.
				r := stepRow{What: say("held ", bold(jobOf(a.Name))), Kind: "held", Facts: jobFacts(a, b.byID), Cards: []string{jobOf(a.Name)}}
				if a.Job != nil {
					r.In = a.Job.Input
				}
				for _, c := range e.Asks {
					if a.Job == nil || c.Job == nil || c.Job.Hook == "" || c.Job.Name != a.Job.Name+"."+c.Job.Hook {
						continue
					}
					r.What = say("held ", bold(jobOf(a.Name)), " for ", bold(c.Job.Hook))
					r.Facts = append(r.Facts, fact{Name: "hook", Text: c.Job.Hook})
					if c.Kind == instance.AskChild {
						hook := linkOf(c.Child, b.byID)
						r.Facts = append(r.Facts, fact{Name: "held for", Text: c.Child + statusOf(b.byID[c.Child]), Run: &hook})
					}
				}
				add(r)
			case instance.AskFail:
				r := stepRow{What: say(bold(jobOf(a.Name)), " failed"), Kind: "failed", Facts: []fact{{Name: "job", Text: a.Name}}, Job: jobOf(a.Name), Full: a.Name, Cards: []string{jobOf(a.Name)}}
				if a.Error != nil {
					r.Facts = append(r.Facts, fact{Name: "error", Text: a.Error.Error})
					if a.Error.Refused != "" {
						r.What = say(bold(jobOf(a.Name)), " denied")
						r.Facts = append(r.Facts, fact{Name: "refused", Text: a.Error.Refused}, fact{Name: "fix", Text: a.Error.Fix})
					}
				}
				add(r)
			case instance.AskCancel:
				add(stepRow{What: say("cancelled ", bold(jobOf(a.Name))), Facts: []fact{{Name: "job", Text: a.Name}}, Cards: []string{jobOf(a.Name)}})
			case instance.AskNote:
				if !b.noted(e, a.Text) {
					add(stepRow{What: say(a.Text), Facts: []fact{{Name: "note", Text: a.Text}}})
				}
			}
		}
		if e.Status != instance.Running && e.Status != "" {
			r := stepRow{What: say(e.Status), Kind: e.Status, Facts: []fact{{Name: "status", Text: e.Status}}}
			if h := b.byID[id]; h != nil {
				r.Out = h.Output
				if h.Failure != "" {
					r.What = say(e.Status + " (" + h.Failure + ")")
					r.Facts = append(r.Facts, fact{Name: "failure", Text: h.Failure})
				}
				if h.Error != "" {
					r.Facts = append(r.Facts, fact{Name: "error", Text: h.Error})
				}
				if h.Refused != "" {
					r.Facts = append(r.Facts, fact{Name: "refused", Text: h.Refused}, fact{Name: "by", Text: h.Layer}, fact{Name: "fix", Text: h.Fix})
				}
			}
			add(r)
		}
		prev = e.Shown
	}
	return out
}

// gotOf is the row of an event a run or a client sent to the run by its
// address, at step e, linked to the run that sent it. A client's event the
// machine had no rule for says so, as the step's receipt for it gives.
func (b *rowBuilder) gotOf(ev instance.Arrived, e client.Entry) stepRow {
	from := senderOf(ev.Event)
	r := stepRow{Kind: "got", In: ev.Data, Facts: []fact{{Name: "event", Text: ev.Name}, {Name: "from", Text: from}}}
	if ev.Sender.Run == "" {
		r.What = say("⇠ ", bold(ev.Name), " from ", bold(from))
		for _, a := range e.Asks {
			if a.Kind == instance.AskReceipt && a.Message != "" && a.Message == ev.Cause.Message && a.Receipt != nil && a.Receipt.Acted != nil && !*a.Receipt.Acted {
				in := strings.Join(e.Shown.States, ", ")
				r.What = append(r.What, word{Text: " · no rule in "}, bold(in))
				r.Facts = append(r.Facts, fact{Name: "no rule", Text: "the machine has no rule for " + ev.Name + " in " + in})
				r.Kind = "got unruled"
			}
		}
		return r
	}
	by := linkOf(ev.Sender.Run, b.byID)
	r.What = say("⇠ ", bold(ev.Name), " from ", word{Text: from, Bold: true, Run: &by})
	r.Facts = append(r.Facts, fact{Name: "run", Text: by.ID + statusOf(b.byID[by.ID]), Run: &by})
	return r
}

// sendOf is the row of an event a run sent: to an address, to a client,
// to a job's child, or to the run that started it, linked to the run it
// went to.
func (b *rowBuilder) sendOf(a *instance.Ask) stepRow {
	event := ""
	var data any
	if a.Event != nil {
		event, data = a.Event.Name, a.Event.Data
	}
	r := stepRow{In: data, Facts: []fact{{Name: "event", Text: event}}}
	note := b.notes[a.Message]
	undelivered := a.Undelivered || note.undelivered != ""
	var to *link
	if id := cmp.Or(a.Run, a.To); id != "" {
		if note.movedTo != "" {
			id = id[:strings.LastIndexByte(id, '/')+1] + note.movedTo
		}
		l := linkOf(id, b.byID)
		to = &l
	}
	switch {
	case a.Location != "":
		address := sentTo(a)
		if h := b.byID[a.To]; a.Key == "" && h != nil && h.Key != "" && h.Location == a.Location {
			address += "#" + h.Key
		}
		r.Kind = "send"
		r.What = say("⇢ sent ", bold(event), " to ", bold(address))
		r.Facts = append(r.Facts, fact{Name: "to", Text: address})
		switch {
		case undelivered:
		case a.Run != "" && to != nil:
			r.What = append(r.What, word{Text: " · started "}, linked(to))
		case to != nil:
			r.What = append(r.What, word{Text: " · "}, linked(to))
		}
	case a.Client != "":
		r.Kind = "send"
		r.What = say("⇢ sent ", bold(event), " to ", bold("client:"+a.Client))
		r.Facts = append(r.Facts, fact{Name: "to", Text: "client:" + a.Client})
	case to != nil:
		r.What = say("sent ", bold(event), " to ", linked(to))
	case a.Name != "":
		r.What = say("sent ", bold(event), " to ", bold(a.Name))
	default:
		r.What = say("sent ", bold(event), " to its parent")
	}
	if to != nil && !undelivered {
		name := "reached"
		if a.Run != "" {
			name = "started"
		}
		if note.movedTo != "" {
			name = "went to"
		}
		r.Facts = append(r.Facts, fact{Name: name, Text: to.ID + statusOf(b.byID[to.ID]), Run: to})
		if a.Location != "" {
			r.Trace = b.traceOf(*to, &trace{})
		}
	}
	if undelivered {
		r.Kind = strings.TrimSpace(r.Kind + " undelivered")
		r.What = append(r.What, word{Text: " · undelivered"})
		r.Facts = append(r.Facts, fact{Name: "undelivered", Text: cmp.Or(a.Text, note.undelivered, "it is not running")})
	}
	return r
}

// traceOf fills in a trace of the run a send or a call reached, with its
// status, where the UI may read that run; it gives none where it may not.
func (b *rowBuilder) traceOf(run link, t *trace) *trace {
	if b.readable == nil || !b.readable(run.ID) {
		return nil
	}
	t.Run = run
	if h := b.byID[run.ID]; h != nil {
		t.Status, t.Failure, t.Ended = h.Status, h.Failure, h.HasEnded()
		if t.Ended {
			t.Took = took(h.Ended.Sub(h.Started))
		}
	}
	return t
}

// traced gives the rows of a trace's group, from the record of the run it
// reached: for a call every row of the run, and for a send the rows of the
// steps the send caused. Its rows are at depth.
func traced(t *trace, entries []client.Entry, byID map[string]*client.Run, readable func(string) bool, now, day time.Time, depth int) []stepRow {
	rows := rowsAt(t.Run.ID, entries, byID, readable, now, day, depth)
	if t.Call {
		return rows
	}
	caused := causedBy(t.Sent, t.Run.ID, entries, byID)
	return slices.DeleteFunc(rows, func(r stepRow) bool { return !caused[r.Step] })
}

// causedBy gives the steps of a run that a cause outside it caused: each
// step that took an event with that cause, and each step that took an event
// whose cause is one of those steps, a job, a timer or a run one of them
// asked for, or a run whose cause leads back to one of them.
func causedBy(cause instance.Cause, id string, entries []client.Entry, byID map[string]*client.Run) map[int]bool {
	caused := map[int]bool{}
	asked := map[string]int{} // the step that asked for each job, timer and run
	// stepOf gives the step of the run that caused an event, or none.
	stepOf := func(ev instance.Arrived) int {
		c := ev.Cause
		if c == (instance.Cause{}) && ev.Name == machine.Timer {
			return asked["timer "+ev.From]
		}
		if c == (instance.Cause{}) {
			return cmp.Or(asked["job "+ev.From], asked["job "+named(ev.From)])
		}
		// Another run's cause names the step of the run that started it.
		for c.Run != "" && c.Run != id {
			if n := asked["run "+c.Run]; n > 0 {
				return n
			}
			h := byID[c.Run]
			if h == nil {
				return 0
			}
			c = h.Cause
		}
		return c.Step
	}
	for _, e := range entries {
		if e.ID != id || e.Step == 0 {
			continue
		}
		for _, ev := range e.Events {
			if ev.Cause == cause || caused[stepOf(ev)] {
				caused[e.Step] = true
			}
		}
		for _, a := range e.Asks {
			switch a.Kind {
			case instance.AskScript, instance.AskHold:
				asked["job "+a.Name] = e.Step
			case instance.AskChild:
				asked["job "+a.Name], asked["run "+a.Child] = e.Step, e.Step
			case instance.AskCall:
				asked["job "+a.Name], asked["run "+a.Run] = e.Step, e.Step
			case instance.AskWake:
				if name, ok := strings.CutPrefix(a.What, "timer."); ok {
					asked["timer "+name] = e.Step
				}
			}
		}
	}
	return caused
}

// resultOf gives what a job gave, from the first of entries that took its
// end: its output, or its error.
func (b *rowBuilder) resultOf(entries []client.Entry, job string) any {
	for _, e := range entries {
		for _, ev := range e.Events {
			if (ev.Name == machine.Done || ev.Name == machine.Failed) && (named(ev.From) == named(job) || jobOf(ev.From) == job) {
				return ev.Data
			}
		}
	}
	return nil
}

// jobFacts are what a row's details say of the job an ask started.
func jobFacts(a *instance.Ask, byID map[string]*client.Run) []fact {
	if a == nil {
		return nil
	}
	facts := []fact{{Name: "job", Text: a.Name}}
	if j := a.Job; j != nil {
		switch {
		case j.Machine != "":
			facts = append(facts, fact{Name: "machine", Text: j.Machine})
		case j.Script != "":
			facts = append(facts, fact{Name: "script", Text: j.Script})
		}
		if j.Timeout > 0 {
			facts = append(facts, fact{Name: "timeout", Text: j.Timeout.String()})
		}
		if j.Approve {
			facts = append(facts, fact{Name: "approve", Text: "true"})
		}
		if shown, _ := j.Tool.(map[string]any); shown != nil {
			if toolkit, _ := shown["toolkit"].(string); toolkit != "" {
				facts = append(facts, fact{Name: "toolkit", Text: toolkit})
			}
		}
	}
	if a.Runner != "" {
		facts = append(facts, fact{Name: "runner", Text: a.Runner})
	}
	if id := cmp.Or(a.Child, callee(*a)); id != "" {
		child := linkOf(id, byID)
		facts = append(facts, fact{Name: "run", Text: id + statusOf(byID[id]), Run: &child})
	}
	return facts
}

// statusOf is a run's status after its ID, as a fact gives it.
func statusOf(r *client.Run) string {
	if r == nil {
		return ""
	}
	if r.Failure != "" {
		return ", " + r.Status + " (" + r.Failure + ")"
	}
	return ", " + r.Status
}

// isTurn says whether a job of an agent is a turn: the job named turn.
func isTurn(name string) bool { return jobOf(name) == "turn" }

// modelOf gives the name of the model a turn asks, which its input names.
func modelOf(job *machine.Job) string {
	var input any
	if job != nil {
		input = job.Input
	}
	in, _ := input.(map[string]any)
	model, _ := in["model"].(map[string]any)
	name, _ := model["name"].(string)
	return cmp.Or(name, "the model")
}

// turnFacts are what a turn's details say of it: why it stopped, its
// text, and what it used.
func turnFacts(data map[string]any) string {
	var facts []string
	if stop, _ := data["stop"].(string); stop != "" {
		facts = append(facts, "stop: "+stop)
	}
	if text, _ := data["text"].(string); text == "" {
		facts = append(facts, "no text")
	} else {
		facts = append(facts, clip(text))
	}
	if usage, _ := data["usage"].(map[string]any); usage != nil {
		facts = append(facts, fmt.Sprintf("%v in, %v out", usage["input"], usage["output"]))
	}
	return strings.Join(facts, " · ")
}

// argsOf gives a call's arguments in a few words: name: value, each value
// cut short.
func argsOf(args any) string {
	m, ok := args.(map[string]any)
	if !ok {
		return compact(args)
	}
	var out []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		v := compact(m[k])
		if s, ok := m[k].(string); ok {
			v = "“" + s + "”"
		}
		if len(v) > 40 {
			v = v[:40] + "…"
		}
		out = append(out, k+": "+v)
	}
	return strings.Join(out, ", ")
}

// compact gives a value as JSON on one line.
func compact(v any) string {
	if v == nil {
		return ""
	}
	data, _ := json.Marshal(v, json.Deterministic(true))
	return string(data)
}

// failed gives the facts of a job's failure: where a permission refused
// it, what was refused, which layer refused it and the fix; otherwise its
// error.
func failed(data any) []fact {
	m, _ := data.(map[string]any)
	if refused, _ := m["refused"].(string); refused != "" {
		layer, _ := m["layer"].(string)
		fix, _ := m["fix"].(string)
		return []fact{{Name: "refused", Text: refused}, {Name: "by", Text: layer}, {Name: "fix", Text: fix}}
	}
	return []fact{{Name: "error", Text: failure(data)}}
}

// failure gives the error a failed event carries.
func failure(data any) string {
	m, _ := data.(map[string]any)
	if refused, _ := m["refused"].(string); refused != "" {
		return "denied: " + refused
	}
	if text, _ := m["error"].(string); text != "" {
		return text
	}
	return compact(data)
}

// say makes words of strings and words.
func say(parts ...any) []word {
	var out []word
	for _, p := range parts {
		switch p := p.(type) {
		case string:
			out = append(out, word{Text: p})
		case word:
			out = append(out, p)
		}
	}
	return out
}

func bold(text string) word { return word{Text: text, Bold: true} }

// linked is a run named in bold, as a link to it.
func linked(l *link) word { return word{Text: l.Machine, Bold: true, Run: l} }

// named gives a job's name as the event of its end gives it: without the
// number of the ask or the attempt that Corbie writes after it.
func named(name string) string {
	if i := strings.IndexAny(name, "#@"); i > 0 {
		return name[:i]
	}
	return name
}

// names gives names in bold, joined by "and".
func names(list []string) []word {
	var out []word
	for i, s := range list {
		if i > 0 {
			out = append(out, word{Text: " and "})
		}
		out = append(out, bold(s))
	}
	return out
}
