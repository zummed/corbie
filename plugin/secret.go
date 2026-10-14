package plugin

import (
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// A Secret is a handle to a secret: it stands for the value and can be put
// wherever a value goes out. It cannot be turned into text, compared,
// measured or read a piece at a time. Text and handles are joined into a
// new handle, which is how a secret is put inside other text. A handle
// holds names and never a value, so however it is printed it shows only
// names.
type Secret struct {
	parts []secretPart
}

// A secretPart is one of three things: text as it was written, a secret by
// its name, or a handle whose whole text is written another way.
type secretPart struct {
	text   string
	name   string
	inner  *Secret
	encode string // how inner is written: "base64", "url" or "json"
}

// maxEscapes is how many times a value can be escaped. Masking has to know
// every form a value can leave in, and each further escaping multiplies
// the forms.
const maxEscapes = 2

// NewSecret makes the handle to the secret of a name.
func NewSecret(name string) *Secret {
	return &Secret{parts: []secretPart{{name: name}}}
}

// Join makes one handle of texts and handles. Each piece keeps the way it
// is written: a handle told to be encoded is encoded by itself, and the
// text around it is left as it was written.
func Join(pieces ...any) (*Secret, error) {
	out := &Secret{}
	for _, piece := range pieces {
		switch p := piece.(type) {
		case string:
			out.parts = append(out.parts, secretPart{text: p})
		case *Secret:
			out.parts = append(out.parts, p.parts...)
		case nil:
		default:
			out.parts = append(out.parts, secretPart{text: fmt.Sprint(p)})
		}
	}
	return out, nil
}

// Encoded gives the handle with its whole text written another way:
// "base64", "url", as for a part of an address, or "json", as for the
// inside of a text in JSON. A value is escaped at most twice.
func (s *Secret) Encoded(how string) (*Secret, error) {
	if how != "base64" && how != "url" && how != "json" {
		return nil, Errorf(BadInput, "a secret can be written as base64, url or json, not %q", how)
	}
	if s.escapes() >= maxEscapes {
		return nil, Errorf(BadInput, "a secret is escaped at most %d times, and this one already is", maxEscapes)
	}
	return &Secret{parts: []secretPart{{inner: s, encode: how}}}, nil
}

// escapes is how many times the most escaped value in the handle is
// escaped.
func (s *Secret) escapes() int {
	most := 0
	for _, p := range s.parts {
		if p.inner != nil {
			most = max(most, 1+p.inner.escapes())
		}
	}
	return most
}

// Names lists the secrets the handle holds.
func (s *Secret) Names() []string {
	var names []string
	for _, p := range s.parts {
		found := []string{p.name}
		if p.inner != nil {
			found = p.inner.Names()
		}
		for _, name := range found {
			if name != "" && !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	return names
}

// String shows the handle by the names of its secrets.
func (s *Secret) String() string {
	return "[secret " + strings.Join(s.Names(), " ") + "]"
}

// GoString shows the handle as String does.
func (s *Secret) GoString() string { return s.String() }

// MarshalJSON refuses: nothing stands in for a secret where data is
// written out.
func (s *Secret) MarshalJSON() ([]byte, error) {
	return nil, Errorf(BadInput, "a secret is not written out as data: a handle can go only where a plugin's page says a secret may")
}

// reveal gives the handle's real text. Each text an encoding wrote that
// holds a secret is handed to told, with the names of its secrets: it is a
// form the value leaves in.
func (s *Secret) reveal(values map[string]string, told func(form, names string)) (string, error) {
	text := ""
	for _, p := range s.parts {
		switch {
		case p.inner != nil:
			inner, err := p.inner.reveal(values, told)
			if err != nil {
				return "", err
			}
			written, err := encoded(inner, p.encode)
			if err != nil {
				return "", err
			}
			if names := p.inner.Names(); len(names) > 0 {
				told(written, strings.Join(names, " "))
			}
			text += written
		case p.name != "":
			value, ok := values[p.name]
			if !ok {
				return "", Errorf(NotFound, "the job was not sent a secret named %q: a job names each secret it uses in its allow list", p.name)
			}
			text += value
		default:
			text += p.text
		}
	}
	return text, nil
}

func encoded(text, how string) (string, error) {
	switch how {
	case "base64":
		return base64.StdEncoding.EncodeToString([]byte(text)), nil
	case "url":
		return url.QueryEscape(text), nil
	}
	return inJSON(text)
}

// inJSON gives a text escaped as for the inside of a text in JSON, without
// the quotes around it.
func inJSON(text string) (string, error) {
	quoted, err := json.Marshal(text)
	if err != nil {
		return "", Errorf(BadInput, "a value that is not text cannot be escaped for JSON")
	}
	return string(quoted[1 : len(quoted)-1]), nil
}

// Shape gives the handle's text with nothing where its secrets are, for a
// plugin to learn where a value is going before it asks for the value.
func (s *Secret) Shape() string {
	text := ""
	for _, p := range s.parts {
		text += p.text
	}
	return text
}
