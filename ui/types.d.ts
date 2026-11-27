// The types of what the UI sends a page, written by `go test ./serve`
// from the Go types; do not edit.

export type Message = PartMessage | StoreMessage;

export interface PartMessage {
  type: "part";
  id: string;
  data: unknown;
}

export interface StoreMessage {
  type: "store";
  store: "lost" | "live";
}

export interface NarrowingPart {
  At: string;
  Window: string;
  Label: string;
}

export interface Choice {
  Name: string;
  Href: string;
  Current: boolean;
}

export interface Place {
  Name: string;
  Location: string;
  Href: string;
  Lifetime: string;
  Status: string;
  Says: string;
  Current: boolean;
}

export interface TreeView {
  Prefix: string;
  Href: string;
  Current: boolean;
  Places: Place[];
}

export interface ClientPlaces {
  Name: string;
  Trees: TreeView[];
}

export interface LocationsPart {
  Every: Choice;
  Clients: ClientPlaces[];
}

export interface PlaceHead {
  Location: string;
  Lifetime: string;
  Tree: string;
  Client: string;
  Cancel: string;
  Send: boolean;
  Start: boolean;
}

export interface Link {
  ID: string;
  Machine: string;
  Title: string;
  Tip: string;
  Type: string;
  Sent: string;
  This: boolean;
}

export interface Row {
  ID: string;
  Status: string;
  Failure: string;
  Name: string;
  Key: string;
  Tip: string;
  Machine: string;
  Type: string;
  Own: boolean;
  Front: Link[];
  Back: Link[];
  Between: string;
  Shown: string;
  Says: string;
  When: string;
  Took: string;
  Under: number;
}

export interface RootLine {
  ID: string;
  Status: string;
  Failure: string;
  Name: string;
  Tip: string;
  Machine: string;
  Type: string;
  When: string;
  Took: string;
  Key: string;
  Titled: boolean;
  Chain: Link[];
  Front: Link[];
  Back: Link[];
  Between: string;
  Shown: string;
  Waits: string;
  Ended: boolean;
  Under: number;
  Rows: Row[];
  More: number;
}

export interface Section {
  ID: string;
  Name: string;
  Lines: RootLine[];
  Windows: Choice[];
}

export interface RunsListParts {
  narrowing: NarrowingPart;
  locations: LocationsPart;
  here: PlaceHead | null;
  ended: Section;
  busy: Section;
  idle: Section;
  singletons: Section;
}

export interface PagePart {
  ID: string;
  Mine: boolean;
}

export interface DefinedAt {
  Href: string;
  Location: string;
  Cards: string;
}

export interface Fact {
  Name: string;
  Text: string;
  Run: Link | null;
}

export interface Why {
  Lead: string;
  Since?: string;
  Rest: string;
  Row: string;
  Facts: Fact[];
}

export interface SummaryPart {
  ID: string;
  Machine: string;
  Type: string;
  Status: string;
  Failure: string;
  Title: string;
  Location: string;
  Address: string;
  At: string;
  Defined: DefinedAt | null;
  Started: string;
  Ended: string;
  Took: string;
  Cancel: string;
  Why: Why | null;
  Differs: string[];
}

export interface LogLine {
  Time: string;
  Name: string;
  Text: string;
}

export interface Going {
  Name: string;
  Runner: string;
  Attempt: number;
  Began?: string;
  Sent: string;
  Due: string;
  Again: string;
  Restarts: number;
  Logs: LogLine[];
}

export interface Wait {
  Text: string;
  Ticket: string;
  Value: string;
  Run: Link | null;
  Job: Going | null;
}

export interface Label {
  Name: string;
  Value: string;
}

export interface WaitsPart {
  ID: string;
  Ended: boolean;
  Waits: Wait[];
  Reply: boolean;
  Send: boolean;
  Labels: Label[];
  Timeout: string;
}

export interface File {
  Path: string;
  Type: string;
  Size: string;
  Href: string;
}

export interface OutputPart {
  Ended: boolean;
  Refused: string;
  Layer: string;
  Fix: string;
  Error: string;
  Output: string;
  Used: string;
  Files: File[];
}

export interface Child {
  ID: string;
  Status: string;
  Failure: string;
  Name: string;
  Key: string;
  Tip: string;
  Machine: string;
  Type: string;
  When: string;
  Took: string;
  Ended: boolean;
}

export interface Word {
  Text: string;
  Bold: boolean;
  Run: Link | null;
}

export interface Group {
  Run: Link;
  Status: string;
  Failure: string;
  Ended: boolean;
  Steps: number;
  Took: string;
  Rows: StepRow[];
}

export interface Cause {
  run?: string;
  step?: number;
  client?: string;
  message?: string;
  principal?: string;
}

export interface Trace {
  Run: Link;
  Status: string;
  Failure: string;
  Ended: boolean;
  Took: string;
  Call: boolean;
  Sent: Cause;
}

export interface StepRow {
  Key: string;
  Run: Link;
  Step: number;
  At: string;
  Time: string;
  Exact: string;
  Depth: number;
  What: Word[];
  Kind: string;
  Group: Group | null;
  Trace: Trace | null;
  Job: string;
  Took: string;
  Cards: string[];
  Facts: Fact[];
}

export interface StepsPart {
  Ended: boolean;
  Rows: StepRow[];
  Count: number;
  Dated: boolean;
  Follows: string;
}

export interface Card {
  Icon: string;
  Name: string;
  Lines: string[];
  Title: string;
  Href: string;
  In?: Scene | null;
  Narrow: boolean;
  Line: string;
}

export interface Stage {
  Card: Card;
  After: string[];
  Why: Record<string, string>;
  When: boolean;
  Stack: boolean;
  Mark: string;
}

export interface Flow {
  Stages: Stage[];
}

export interface State {
  Card: Card;
  Name: string;
  Parent: string;
  Start: string;
  Final: boolean;
  Fails: boolean;
  Send: boolean;
  Mark: string;
}

export interface Rule {
  From: string;
  Event: string;
  To: string;
  When: string;
  Timer: string;
  Send: boolean;
  Taken: boolean;
}

export interface Chart {
  Start: string;
  States: State[];
  Rules: Rule[];
}

export interface Tool {
  Card: Card;
  When: boolean;
  Mark: string;
}

export interface Hook {
  Card: Card;
  When: boolean;
  Mark: string;
}

export interface Agent {
  Model: string;
  Mark: string;
  ModelLine: string;
  Turn: number;
  Turns: number;
  Tools: Tool[];
  Hooks: Hook[];
}

export interface Decision {
  Model: string;
  Mark: string;
  ModelLine: string;
  Answers: Card;
}

export interface Kit {
  Tools: Tool[];
  Hooks: Hook[];
}

export interface Scene {
  Type: string;
  Name: string;
  Flow?: Flow | null;
  Chart?: Chart | null;
  Agent?: Agent | null;
  Decision?: Decision | null;
  Kit?: Kit | null;
}

export interface Line {
  Time: string;
  Machine: string;
  Step: number;
  Way: string;
  Text: string;
  End: string;
}

export interface Shown {
  Row: StepRow | null;
  Defined: DefinedAt | null;
  Page: string;
  Type: string;
  Time: string;
  Drawing: Scene | null;
  Lines: Line[];
  Input: string;
  Output: string;
  Logs: LogLine[];
}

export interface RunParts {
  page: PagePart;
  summary: SummaryPart;
  path: Link[];
  waits: WaitsPart;
  output: OutputPart;
  children: Child[];
  steps?: StepsPart | null;
  shown?: Shown | null;
}

export interface Entry {
  Location: string;
  Name: string;
  Type: string;
  First: string;
  Lifetime: string;
  Description: string;
}

export interface Listed {
  Prefix: string;
  Client: string;
  Directory: string;
  Entries: Entry[];
}

export interface Index {
  Stamp: string;
  Trees: Listed[];
}

export interface AtLocation {
  Running: number;
  FailedToday: number;
  In: Record<string, number>;
}

export interface Span {
  Class: string;
  Text: string;
}

export interface CodeLine {
  Spans: Span[];
  From: string;
}

export interface MachinesPlace {
  Description: string;
  Location: string;
  Lifetime: string;
  Started: string;
  Update: string;
  Updated: string;
  Runs: number;
  Receives: string[];
  Sends: string[];
  Approve: string[];
}

export interface Use {
  Name: string;
  Kind: string;
  State: string;
}

export interface View {
  Open: string;
  Kind: string;
  First: string;
  Stamp: string;
  View: string;
  Text: CodeLine[];
  Place: MachinesPlace | null;
  Uses: Use[];
  Drawing: Scene | null;
}

export interface MachinesParts {
  list: Index | null;
  runs: Record<string, AtLocation>;
  view?: View | null;
}

export interface Party {
  Name: string;
  Kind: string;
  Fingerprint: string;
  Tags: string[];
  Ceiling: string[];
  Deny: string[];
  At: string[];
  Removed: boolean;
  Acts: string[];
}

export interface Setting {
  At: string;
  Name: string;
  Kind: string;
  Value: string;
}

export interface ControllerParts {
  parties: Party[];
  settings: Setting[];
}
