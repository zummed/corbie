// Package secrets is the plugin that gives a script handles to secrets.
package secrets

import "github.com/zummed/corbie/plugin"

func init() {
	plugin.Register(&plugin.Module{Name: "secrets", Page: page, Calls: map[string]*plugin.Call{
		"get": {Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			name, err := plugin.Text(first, "the secret's name")
			if err != nil {
				return nil, err
			}
			if !job.Sent(name) {
				return nil, plugin.Errorf(plugin.NotFound, "the job was not sent a secret named %q: a job names each secret it uses in its allow list, as secret:%s@net:host", name, name)
			}
			return plugin.NewSecret(name), nil
		}},
		// text is used as a tag: secrets.text`Bearer ${token}`.
		"text": {Variadic: true, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			args, _ := first.([]any)
			if len(args) == 0 {
				return nil, plugin.Errorf(plugin.BadInput, "secrets.text is used as a tag: secrets.text`Bearer ${token}`")
			}
			texts, ok := args[0].([]any)
			if !ok {
				return plugin.Join(args...)
			}
			var pieces []any
			for i, text := range texts {
				pieces = append(pieces, text)
				if i+1 < len(args) {
					pieces = append(pieces, args[i+1])
				}
			}
			return plugin.Join(pieces...)
		}},
		"encoded": {Options: []string{"as"}, Do: func(job *plugin.Job, first any, opt plugin.Options) (any, error) {
			handle, ok := first.(*plugin.Secret)
			if !ok {
				return nil, plugin.Errorf(plugin.BadInput, "secrets.encoded takes a handle first")
			}
			how, err := opt.Text("as", "base64")
			if err != nil {
				return nil, err
			}
			return handle.Encoded(how)
		}},
	}})
}

const page = `secrets: this plugin gives a script handles to the job's secrets.

A handle stands for a secret's value, which the script never has. A plugin
that is passed a handle sends the value in its place.
` + "`corbie manual using-secrets`" + ` shows how a secret is set and named in a job.

  secrets.get(name)                  returns the handle to a secret the job's allow list names
  secrets.text` + "`Bearer ${handle}`" + `     returns a new handle: the text with the handles in it
  secrets.encoded(handle, { as })    returns a new handle: the handle, encoded

Options:
  as   "base64", "url", for a query value in a URL, or "json", for the inside
       of a JSON string; default "base64"

- A handle goes where a plugin's page says a call takes one: a header, part
  of a URL, a body, a password, a program's argument or environment variable,
  a file.
- A script cannot convert a handle to text, compare it, measure it, read part
  of it, return it or emit it.
- No plugin escapes a secret for the text around it: a script that writes
  that text encodes the handle first.
- secrets.text keeps an encoded handle encoded and the text around it as
  written.
- A value can be encoded at most twice.

Errors:
  not_found   secrets.get: the job's allow list does not name the secret
  bad_input   secrets.encoded: as: is not one of the three, or the value is
              already encoded twice

Needs nothing. The plugin that sends a handle checks where its secret may go.

This script signs in with a user's name and a secret, encoded together in base64.

    allow: [net:api.github.com, "secret:GITHUB_TOKEN@net:api.github.com"]

    const gh = secrets.get("GITHUB_TOKEN")
    const login = secrets.encoded(secrets.text` + "`octocat:${gh}`" + `, { as: "base64" })
    const me = http.get("https://api.github.com/user", {
      headers: { Authorization: secrets.text` + "`Basic ${login}`" + ` }
    })
    return me.status                    // gives 200

This script writes a secret into a JSON body. With as: "json", a quote or a
backslash in the value does not end the JSON string.

    allow: [net:example.com, "secret:PORTAL_PASSWORD@net:example.com"]

    const password = secrets.encoded(secrets.get("PORTAL_PASSWORD"), { as: "json" })
    const signIn = http.post("https://example.com/login", {
      headers: { "Content-Type": "application/json" },
      body: secrets.text` + "`{\"user\": \"ana\", \"password\": \"${password}\"}`" + `,
    })
    signIn.status                       // gives 200
`
