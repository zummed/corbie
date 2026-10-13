package plugin

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/zummed/corbie/permission"
)

func job(secrets map[string]string) *Job { return &Job{Secrets: secrets} }

func Test_sec_mask_knows_every_form(t *testing.T) {
	value := "s3cr3t/value+with=odd&chars \"and\\more"
	j := job(map[string]string{"TOKEN": value})
	inJSON, _ := inJSON(value)
	for what, text := range map[string]string{
		"as it is":            "the server echoed " + value + " back",
		"escaped for a query": "q=" + url.QueryEscape(value),
		"escaped for a path":  "/x/" + url.PathEscape(value),
		"base64":              base64.StdEncoding.EncodeToString([]byte(value)),
		"base64 no padding":   base64.RawStdEncoding.EncodeToString([]byte(value)),
		"base64 url":          base64.RawURLEncoding.EncodeToString([]byte(value)),
		"escaped for JSON":    `{"echo":"` + inJSON + `"}`,
	} {
		masked := j.Mask(text)
		if strings.Contains(masked, value) || !strings.Contains(masked, "[secret TOKEN]") {
			t.Errorf("%s: %q was masked as %q", what, text, masked)
		}
	}
	if got := j.Mask("nothing of it here"); got != "nothing of it here" {
		t.Errorf("text with no secret was changed: %q", got)
	}
	// A form a handle was told to send is known from the moment it is sent,
	// whatever text the secret was joined with before it was encoded.
	j = &Job{Secrets: map[string]string{"TOKEN": value, "USER": "someone"},
		Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"secret:*@*"}}}}}
	both, _ := Join(NewSecret("USER"), ":", NewSecret("TOKEN"))
	basic, _ := both.Encoded("base64")
	header, _ := Join("Basic ", basic)
	sent, err := j.RevealOver(header, "api.example:443", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := j.Mask("you sent " + sent); got != "you sent Basic [secret USER TOKEN]" {
		t.Errorf("a form the handle was told to send came back as %q", got)
	}
}

func Test_sec_escaped_at_most_twice(t *testing.T) {
	value := "s3cr3t/value+with=odd&chars \"and\\more"
	ways := []string{"base64", "url", "json"}
	for _, first := range ways {
		for _, second := range ways {
			j := &Job{Secrets: map[string]string{"TOKEN": value},
				Perms: &permission.Set{Layers: []permission.Layer{{Name: "test", Allow: []string{"secret:*@*"}}}}}
			once, err := NewSecret("TOKEN").Encoded(first)
			if err != nil {
				t.Fatal(err)
			}
			// Joined into text, the handle is still escaped once, and the
			// whole may be escaped once more.
			joined, _ := Join("k=", once, ";")
			twice, err := joined.Encoded(second)
			if err != nil {
				t.Fatalf("%s then %s was refused: %v", first, second, err)
			}
			// A third escaping is refused, of the handle and of anything
			// it is joined into.
			for _, third := range ways {
				around, _ := Join("x", twice)
				if _, err := twice.Encoded(third); err == nil {
					t.Errorf("%s, %s and then %s was taken", first, second, third)
				}
				if _, err := around.Encoded(third); err == nil {
					t.Errorf("%s, %s, joined, and then %s was taken", first, second, third)
				}
			}
			// And each form two escapings give is one masking knows.
			sent, err := j.RevealOver(twice, "api.example:443", true)
			if err != nil {
				t.Fatal(err)
			}
			if got := j.Mask("echo " + sent + " end"); got != "echo [secret TOKEN] end" {
				t.Errorf("%s then %s: %q came back as %q", first, second, sent, got)
			}
		}
	}
	if _, err := NewSecret("TOKEN").Encoded("rot13"); err == nil {
		t.Error("an escaping that is not one was taken")
	}
}

func Test_sec_mask_short_value_only_as_it_is(t *testing.T) {
	j := job(map[string]string{"PIN": "abc"})
	if got := j.Mask("the pin abc was echoed"); got != "the pin [secret PIN] was echoed" {
		t.Fatalf("a short value as it is: %q", got)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("abc"))
	if got := j.Mask("unrelated " + encoded + " text"); strings.Contains(got, "[secret") {
		t.Fatalf("a short value's encoded form was replaced: %q", got)
	}
}

// A value of one or two characters turns up in every text, and is not
// searched for.
func TestValueOfOneOrTwoCharactersIsNotSearchedFor(t *testing.T) {
	j := job(map[string]string{"FLAG": "x", "PAIR": "re", "PIN": "abc"})
	if got := j.Mask("reports/a.txt"); got != "reports/a.txt" {
		t.Fatalf("a text holding a value of one character and one of two came back as %q", got)
	}
	if got := j.Mask("reports/abc.txt"); got != "reports/[secret PIN].txt" {
		t.Fatalf("a text holding a value of three characters came back as %q", got)
	}
}

func Test_sec_mask_longest_form_first(t *testing.T) {
	// One secret's value is the start of another's.
	j := job(map[string]string{"SHORT": "abcdefgh", "LONG": "abcdefghijklmnop"})
	if got := j.Mask("echo abcdefghijklmnop"); got != "echo [secret LONG]" {
		t.Fatalf("the longer value was not replaced whole: %q", got)
	}
}

func Test_sec_mask_each_line_of_a_value_of_several(t *testing.T) {
	key := "-----BEGIN KEY-----\nAAAABBBBCCCCDDDDEEEE\nFFFFGGGGHHHHIIII\n-----END KEY-----"
	j := job(map[string]string{"KEY": key})
	if got := j.Mask("error near line: FFFFGGGGHHHHIIII"); strings.Contains(got, "FFFFGGGG") {
		t.Fatalf("one line of a value of several lines shows: %q", got)
	}
}

func TestSecretHandles(t *testing.T) {
	j := &Job{Secrets: map[string]string{"TOKEN": "tok", "USER": "me"}}
	handle, _ := Join("Bearer ", NewSecret("TOKEN"))
	if names := handle.Names(); len(names) != 1 || names[0] != "TOKEN" {
		t.Fatalf("a joined handle names %v", names)
	}
	if handle.Shape() != "Bearer " {
		t.Fatalf("a handle's shape is %q", handle.Shape())
	}
	if got, err := handle.reveal(j.Secrets, j.Note); err != nil || got != "Bearer tok" {
		t.Fatalf("a handle reveals %q (%v)", got, err)
	}
	both, _ := Join(NewSecret("USER"), ":", NewSecret("TOKEN"))
	encoded, err := both.Encoded("base64")
	if err != nil {
		t.Fatal(err)
	}
	basic, _ := Join("Basic ", encoded)
	if got, err := basic.reveal(j.Secrets, j.Note); err != nil || got != "Basic "+base64.StdEncoding.EncodeToString([]byte("me:tok")) {
		t.Fatalf("an encoded handle joined into text reveals %q (%v)", got, err)
	}
	if _, err := NewSecret("MISSING").reveal(j.Secrets, j.Note); err == nil {
		t.Fatal("a secret the job was not sent was revealed")
	}
	if _, err := both.Encoded("rot13"); err == nil {
		t.Fatal("an encoding that is not one was taken")
	}
}

// said is a channel that keeps what was emitted.
type said struct {
	Channel
	pieces []string
}

func (s *said) Emit(name string, data any) error {
	s.pieces = append(s.pieces, data.(string))
	return nil
}

func Test_sec_mask_across_pieces(t *testing.T) {
	value := "s3cr3t/value+with=odd&chars"
	text := "the server echoed " + value + " and then " + base64.StdEncoding.EncodeToString([]byte(value)) + " — and no more."
	// However the text is cut into pieces, no piece and no join shows the
	// value, and nothing else of the text is lost.
	want := job(map[string]string{"TOKEN": value}).Mask(text)
	for size := 1; size <= len(text); size++ {
		out := &said{}
		j := job(map[string]string{"TOKEN": value})
		j.Channel = out
		telling := j.Telling("text")
		for at := 0; at < len(text); at += size {
			telling.Add(text[at:min(at+size, len(text))])
		}
		telling.End()
		if got := strings.Join(out.pieces, ""); got != want {
			t.Fatalf("in pieces of %d the text went out as %q", size, got)
		}
	}
	// A watcher is not kept waiting for the end: text goes out as soon as
	// it can no longer be part of a value.
	out := &said{}
	j := job(map[string]string{"TOKEN": value})
	j.Channel = out
	telling := j.Telling("text")
	telling.Add(strings.Repeat("word ", 20))
	if len(out.pieces) != 1 || len(out.pieces[0]) < 50 {
		t.Fatalf("before the end, %q had gone out", out.pieces)
	}
	// With no secret nothing is held back.
	out = &said{}
	plain := &Job{Channel: out}
	plain.Telling("text").Add("at once")
	if len(out.pieces) != 1 || out.pieces[0] != "at once" {
		t.Fatalf("with no secret, %q went out", out.pieces)
	}
}
