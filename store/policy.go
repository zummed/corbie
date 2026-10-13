package store

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Policy prints the policy that limits a party's key for a bucket to its
// row of the table. bucket is the bucket's name and prefix the path the
// store's address gives after it. The policy is written to be attached to
// the party's key; set on the bucket itself, each statement needs that key
// as its principal as well.
//
// Every write a limited party may make is allowed only when the request
// carries If-None-Match, which is what makes it only-if-new. The steps of a
// large upload that put nothing under a name are allowed without it, as is
// giving such an upload up. Listing is allowed only under the paths the
// party may list.
//
// Amazon S3 enforces all of this. A service that takes the same requests
// but does not know the condition key s3:if-none-match cannot hold a party
// to only-if-new, and so cannot enforce the table whatever else it does.
//
// A policy cannot say "any one part of a name", so where the table has any
// one part the policy has any path. A limited party can therefore add names
// deeper than the layout draws; it still can never replace or remove one.
func Policy(p Party, bucket, prefix string) (string, error) {
	if err := policyName(bucket); err != nil {
		return "", err
	}
	if prefix = strings.Trim(prefix, "/"); prefix != "" {
		if err := policyName(prefix); err != nil {
			return "", err
		}
		prefix += "/"
	}
	arn := "arn:aws:s3:::" + bucket
	var read, add, rewrite, remove, list []string
	for _, g := range p.grants() {
		pattern, err := policyPattern(g.pattern)
		if err != nil {
			return "", err
		}
		pattern = prefix + pattern
		if g.acts&ActRead != 0 {
			read = append(read, arn+"/"+pattern)
		}
		switch {
		case g.acts&ActRewrite != 0:
			rewrite = append(rewrite, arn+"/"+pattern)
		case g.acts&ActAdd != 0:
			add = append(add, arn+"/"+pattern)
		}
		if g.acts&ActDelete != 0 {
			remove = append(remove, arn+"/"+pattern)
		}
		if g.acts&ActList != 0 {
			list = append(list, pattern)
		}
	}

	type conditions map[string]map[string]any
	type statement struct {
		Effect    string
		Action    string
		Resource  []string
		Condition conditions `json:",omitempty"`
	}
	var statements []statement
	allow := func(action string, resources []string, when conditions) {
		if len(resources) > 0 {
			statements = append(statements, statement{"Allow", action, resources, when})
		}
	}
	allow("s3:GetObject", read, nil)
	allow("s3:PutObject", rewrite, nil)
	allow("s3:DeleteObject", remove, nil)
	allow("s3:PutObject", add, conditions{"Null": {"s3:if-none-match": "false"}})
	allow("s3:PutObject", add, conditions{"Bool": {"s3:ObjectCreationOperation": "false"}})
	allow("s3:AbortMultipartUpload", slices.Concat(add, rewrite), nil)
	if len(list) > 0 {
		allow("s3:ListBucket", []string{arn}, conditions{"StringLike": {"s3:prefix": list}})
	}
	out, err := json.MarshalIndent(struct {
		Version   string
		Statement []statement
	}{"2012-10-17", statements}, "", "  ")
	return string(out), err
}

// policyPattern writes one of the table's patterns as a policy has it,
// where "*" is any run of characters.
func policyPattern(pattern string) (string, error) {
	parts := strings.Split(pattern, "/")
	for i, part := range parts {
		if part == "*" || part == "**" {
			parts[i] = "*"
		} else if err := policyName(part); err != nil {
			return "", err
		}
	}
	return strings.Join(parts, "/"), nil
}

// policyName refuses a name that cannot go into a policy as it is: one no
// store holds, or one a policy would read as a pattern, which would widen
// the row.
func policyName(name string) error {
	if err := checkName(name); err != nil {
		return fmt.Errorf("store: no policy for this: %w", err)
	}
	if strings.ContainsAny(name, "*?$") {
		return fmt.Errorf("store: no policy for this: a policy would read %q as a pattern", name)
	}
	return nil
}

// PolicyFor prints the policy for a party of the bucket an address names.
func PolicyFor(address string, p Party) (string, error) {
	b, err := newBucket(address, bucketKey{})
	if err != nil {
		return "", err
	}
	return Policy(p, b.name, strings.TrimSuffix(b.prefix, "/"))
}
