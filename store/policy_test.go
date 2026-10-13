package store

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// A carried policy is one that Policy printed, read back as a bucket that
// carries it reads it.
type carried struct {
	t         *testing.T
	Statement []struct {
		Effect    string
		Action    string
		Resource  []string
		Condition map[string]map[string]any
	}
}

func carry(t *testing.T, p Party) carried {
	t.Helper()
	printed, err := Policy(p, "bkt", "/pre/fix/")
	if err != nil {
		t.Fatal(err)
	}
	c := carried{t: t}
	if err := json.Unmarshal([]byte(printed), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// like says whether s fits a pattern in which "*" is any run of characters.
func like(pattern any, s string) bool {
	re := strings.ReplaceAll(regexp.QuoteMeta(pattern.(string)), `\*`, ".*")
	return regexp.MustCompile("^" + re + "$").MatchString(s)
}

// allows says whether the policy allows an action on a resource, for a
// request with these condition keys.
func (c carried) allows(action, resource string, keys map[string]string) bool {
	for _, s := range c.Statement {
		if s.Effect != "Allow" || s.Action != action ||
			!slices.ContainsFunc(s.Resource, func(r string) bool { return like(r, resource) }) {
			continue
		}
		holds := true
		for test, conditions := range s.Condition {
			for key, want := range conditions {
				value, has := keys[key]
				switch test {
				case "Null":
					holds = holds && has == (want == "false")
				case "Bool":
					holds = holds && has && value == want
				case "StringLike":
					holds = holds && has && slices.ContainsFunc(want.([]any), func(p any) bool { return like(p, value) })
				default:
					c.t.Fatalf("a condition this stand-in does not know: %s", test)
				}
			}
		}
		if holds {
			return true
		}
	}
	return false
}

// may says whether the policy allows the request the bucket store makes to
// do act to name.
func (c carried) may(act Act, name string) bool {
	object := "arn:aws:s3:::bkt/pre/fix/" + name
	switch act {
	case ActRead:
		return c.allows("s3:GetObject", object, nil)
	case ActList:
		return c.allows("s3:ListBucket", "arn:aws:s3:::bkt", map[string]string{"s3:prefix": "pre/fix/" + name + "/"})
	case ActAdd:
		return c.allows("s3:PutObject", object, map[string]string{"s3:if-none-match": "*", "s3:ObjectCreationOperation": "true"})
	case ActRewrite:
		return c.allows("s3:PutObject", object, map[string]string{"s3:ObjectCreationOperation": "true"}) ||
			c.allows("s3:PutObject", object, map[string]string{"s3:if-match": `"mark"`, "s3:ObjectCreationOperation": "true"})
	}
	return c.allows("s3:DeleteObject", object, nil)
}

var policyParties = map[string]Party{
	"owner":      {Kind: Owner},
	"controller": {Kind: Controller, Name: "ctl"},
	"runner":     {Kind: Runner, Name: "r1", Controller: "ctl"},
	"client":     {Kind: Client, Name: "c1", Controller: "ctl"},
	"viewer":     {Kind: Viewer, Controller: "ctl"},
	"nobody":     {Kind: Nobody},
}

// The policy printed for each party allows what its row of the table
// allows and refuses what the row refuses, for every act on every place in
// the layout.
func TestPolicyIsTheTable(t *testing.T) {
	names := []string{
		"version", "elsewhere",
		"ctl/inbox/c1/m1", "ctl/inbox/c2/m1", "ctl/inbox/r1/m1", "ctl/inbox/r2/m1",
		"ctl/receipts/c1/m1", "ctl/receipts/c2/m1",
		"ctl/running/c1/i1", "ctl/running/c2/i2", "ctl/finished/c1/i0", "ctl/finished/c2/i9",
		"ctl/records/c1/i1/steps/1", "ctl/records/c1/i1/emits/j1/1", "ctl/records/c1/i1/blobs/b1",
		"ctl/records/c2/i2/steps/1", "ctl/records/c2/i2/emits/j1/1", "ctl/records/c2/i2/blobs/b1",
		"ctl/wakes/t/c1/i1/w", "ctl/trust/r1", "ctl/secrets/s1",
		"r1/inbox/ctl/j1", "r1/inbox/c1/x", "r1/taken/j1", "r2/inbox/ctl/j2", "r2/inbox/r1/x", "r2/taken/j2",
	}
	paths := []string{
		"", "ctl", "ctl/inbox", "ctl/inbox/c1", "ctl/inbox/r1", "ctl/receipts", "ctl/receipts/c1", "ctl/receipts/c2",
		"ctl/running", "ctl/running/c1", "ctl/running/c2", "ctl/finished", "ctl/finished/c1", "ctl/finished/c2",
		"ctl/records", "ctl/records/c1", "ctl/records/c1/i1", "ctl/records/c1/i1/steps", "ctl/records/c1/i1/blobs",
		"ctl/records/c2", "ctl/records/c2/i2/blobs", "ctl/wakes", "ctl/trust", "ctl/secrets",
		"r1", "r1/inbox", "r1/inbox/ctl", "r1/taken", "r2", "r2/inbox", "r2/taken",
	}
	for who, p := range policyParties {
		c := carry(t, p)
		for _, name := range names {
			for _, act := range []Act{ActRead, ActAdd, ActRewrite, ActDelete} {
				if got, want := c.may(act, name), p.Allows(act, name); got != want {
					t.Errorf("the %s's policy: %s %s is %v, and by the table %v", who, act, name, got, want)
				}
			}
			// The steps of a large upload that put nothing under the name,
			// and giving one up, go with the right to write there at all.
			object, want := "arn:aws:s3:::bkt/pre/fix/"+name, p.Allows(ActAdd, name) || p.Allows(ActRewrite, name)
			if got := c.allows("s3:PutObject", object, map[string]string{"s3:ObjectCreationOperation": "false"}); got != want {
				t.Errorf("the %s's policy: a part of an upload to %s is %v, want %v", who, name, got, want)
			}
			if got := c.allows("s3:AbortMultipartUpload", object, nil); got != want {
				t.Errorf("the %s's policy: giving up an upload to %s is %v, want %v", who, name, got, want)
			}
		}
		for _, path := range paths {
			if got, want := c.may(ActList, path), p.Allows(ActList, path); got != want {
				t.Errorf("the %s's policy: list %q is %v, and by the table %v", who, path, got, want)
			}
		}
		// A listing with no path given is one of the whole bucket.
		if c.allows("s3:ListBucket", "arn:aws:s3:::bkt", nil) {
			t.Errorf("the %s's policy allows a listing of the whole bucket", who)
		}
	}
}

// A name that would read as a pattern in a policy gets none.
func TestPolicyRefusesNamesThatWiden(t *testing.T) {
	for _, p := range []Party{
		{Kind: Runner, Name: "r*", Controller: "ctl"},
		{Kind: Runner, Name: "r1", Controller: "c?l"},
		{Kind: Client, Name: "${aws:username}", Controller: "ctl"},
		{Kind: Client, Name: "", Controller: "ctl"},
		{Kind: Viewer, Controller: ".."},
	} {
		if printed, err := Policy(p, "bkt", ""); err == nil {
			t.Errorf("a policy was printed for %+v:\n%s", p, printed)
		}
	}
	for _, at := range [][2]string{{"", ""}, {"b*", ""}, {"bkt", "pre/*"}, {"bkt", "a//b"}} {
		if _, err := Policy(policyParties["runner"], at[0], at[1]); err == nil {
			t.Errorf("a policy was printed for the bucket %q and the path %q", at[0], at[1])
		}
	}
	if _, err := Policy(policyParties["runner"], "bkt", ""); err != nil {
		t.Error(err)
	}
}
