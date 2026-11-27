# Toolkit

The detailed rules behind the [Toolkit overview](../toolkit.md). Area code: `kit`. What a toolkit shares with every machine type (values, jobs and their names, events, children, hooks, locations, limits) is under Machines and is not repeated here.

## The definition

**R-kit-01** A toolkit's definition has a `description:`, and either `tools:`, with `instructions:` if it gives any, or one `job:`. A toolkit with both, with neither, with no description or with `lifetime: singleton` is refused when the definition is checked. A tool has a `description:`, `args:` and a `job:`, and a job that reads an argument its `args:` does not name is refused.
*Accept:* `TestToolkitDefinitionIsChecked`: each of these is refused, and the refusal names its place.

## Requests

**R-kit-02** A run of a toolkit reads `request:` in its input. Its `do:` is `open`, `call`, `resume` or `cancel`; a request holds `do`, `toolkit`, `tool`, `args`, `state` and `responses` and no other field; every request but an open names its `tool`; a resume and a cancel give `state`. A request of any other form fails the run with an error that names the field, and no job starts.
*Accept:* `TestToolkitRefusesARequestItCannotRead`: a run with no request, with `do: list`, with a field `at`, a call with no tool and a resume with no state each end failed with no job asked for.

**R-kit-03** A toolkit with `tools:` answers an open with `{ instructions, entries }`: its `instructions:` filled from the input, and one entry `{ kind: tool, name, description, args }` for each tool, in the order of their names, `args` in full form.
*Accept:* parity case 153: the open gives the instructions with the shelf filled in, and the entry of `add_note` with the full shape of its arguments.

**R-kit-04** A toolkit with `tools:` answers a call by checking `args` against the tool's `args:`, then starting the tool's job, which reads `input` and `args`. The run ends with `{ result }`, the job's output, or fails as the job did. A tool the toolkit does not have, arguments that do not fit, a resume, a cancel and a request with `toolkit:` each fail the run before any job starts.
*Accept:* parity case 153: the call's result is the job's output, made from the input's shelf and the argument's text. Parity case 157: an argument of the wrong type, a tool the toolkit does not have and a resume each fail, and no script is asked for.

**R-kit-05** A toolkit with `job:` starts that job for every request, with the toolkit's input or the `input:` the job writes, and the job's output is the answer. An answer to an open is `{ instructions, entries }`; to a call or a resume, `{ result }`, `{ working, state }` or `{ requests, state }`; to a cancel, `{ result }`. An answer of any other form fails the run.
*Accept:* parity case 154: the open, a call that gives a result and a call that gives `{ working, state }` each end with the job's answer, and an answer of `{ result, at }` fails the run.

**R-kit-06** The job a toolkit starts for a call, a resume or a cancel carries `job.tool`, `{ toolkit, name, args }`: the toolkit's name, the request's tool and its arguments. The toolkit's hooks read it.
*Accept:* parity case 156: the toolkit's hook, whose `when:` reads `job.tool.name`, is given `{ toolkit: notes, name: add_note, args: { text: milk } }`, and the caller's hook does not run for the tool's job.

**C-kit-01** An entry in a job's answer to an open keeps its kind, its name, its description and, for a tool, the shape of its arguments, and nothing else. An entry that kept a field of its writer's could name a location or a job for a reader of the list to run. *Test:* `toolkit_entry_keeps_only_its_four_fields`

## Interfaces

Uses:

- **Machines.** Values and expressions, jobs, events, hooks, locations, `receive:` and limits. A toolkit adds `args` to what an expression in a tool's job reads.
- **Controller.** Takes a toolkit's step, starts its job, and runs the hooks before and after it.
- **Plugins.** `server.toolkit` of `mcp` answers a request for an MCP server (R-plug-60).

Used by:

- **Machines.** A toolkit is one of the five types, and supplies the shape of its definitions, the filling-out of its short forms, its check, its step and its manual page.
- **Flow, Statechart.** Either runs a toolkit as a job, by its file or by its location.
