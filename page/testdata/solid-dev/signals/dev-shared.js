/**
 * Thrown by a tracked read whose value is currently pending (an async memo /
 * `createSignal(asyncFn)` / projection / store derivation that hasn't settled
 * yet). Surfacing through the reactive graph is what suspends the consumer
 * scope — the nearest enclosing `<Loading>` boundary catches the throw and
 * renders its fallback until the source resolves.
 *
 * App code rarely catches this directly; `<Loading>` is the canonical
 * handler. The error type is exposed for advanced cases — e.g. interop layers
 * that bridge Solid's pending-throw protocol to a different async strategy,
 * or tests that want to assert on the suspension shape.
 *
 * @example
 * ```ts
 * // Advanced: distinguish "not ready yet" from a real error in custom
 * // boundary plumbing. App code should rely on `<Loading>` / `<Errored>`.
 * try {
 *   const value = readReactiveSource();
 * } catch (err) {
 *   if (err instanceof NotReadyError) throw err; // re-throw to suspend
 *   reportError(err);
 * }
 * ```
 */
class NotReadyError extends Error {
  constructor(source) {
    super();
    this.source = source;
  }
}
class StatusError extends Error {
  constructor(source, original) {
    // Best-effort message: formatting must never replace the original (#4112).
    try {
      var message = String(original instanceof Error ? original.message : original);
    } catch {}
    super(message, { cause: original });
    this.source = source;
  }
}
/** Return the user's error from an internal status wrapper. */
function unwrapStatusError(error) {
  return error instanceof StatusError ? error.cause : error;
}
/**
 * Rejection value of `until(fn, { timeout })` when the predicate does not turn
 * truthy within the window. Inside an `action()`, the rejection is thrown back
 * in at the `yield` point — catchable there, or the action fails and its
 * optimistic state reverts.
 */
class TimeoutError extends Error {
  constructor(message = "Timed out waiting for condition") {
    super(message);
    this.name = "TimeoutError";
  }
}
class NoOwnerError extends Error {
  constructor() {
    super("Context can only be accessed under a reactive root.");
  }
}
class ContextNotFoundError extends Error {
  constructor() {
    super(contextNotFoundMessage(arguments[0], arguments[1]));
  }
}
/**
 * Dev message for a context read with no value. `undefined` means "not
 * provided" for context, from either side — no default, or a provider
 * passing `undefined` — so the message says which happened and points at
 * `null` as the explicit empty value.
 */
function contextNotFoundMessage(context, providedUndefined) {
  if (!context)
    return "Context must either be created with a default value or a value must be provided before accessing it.";
  const description = context.id.description;
  const name = description ? `Context "${description}"` : "Context";
  if (providedUndefined)
    return `${name} was provided with \`undefined\`, which counts as not provided: \`undefined\` means "no value" for context. To provide an empty value, create the context with a \`null\` default (\`createContext<T | null>(null)\`) and pass \`null\`.`;
  if (context._undefinedDefault)
    return `${name} has no provider above this read, and \`createContext(undefined)\` does not give it a default: \`undefined\` means "no value" for context. Mount a provider above this read, or use \`null\` as an explicit empty default: \`createContext<T | null>(null)\`.`;
  return `${name} has no provider above this read and no default value. Mount a provider above this read, or create the context with a default; for an optional context, use \`null\` as the empty default (\`createContext<T | null>(null)\`), not \`undefined\`.`;
}

const REACTIVE_NONE = 0;
const REACTIVE_CHECK = 1 << 0;
const REACTIVE_DIRTY = 1 << 1;
const REACTIVE_RECOMPUTING_DEPS = 1 << 2;
const REACTIVE_IN_HEAP = 1 << 3;
const REACTIVE_IN_HEAP_HEIGHT = 1 << 4;
/** L2 — the node sits on its owner's parked frame (`_x._pendingFirstChild`):
 * it is the displayed frame until the owner's commit disposes it. Set by
 * `markDisposal` over the parked subtree; carried through every per-pass
 * flag wipe (#3543). Scheduling reads it once, at the heap pop: a zombie's
 * recompute is deferred past the seam (`deferZombie`). */
const REACTIVE_ZOMBIE = 1 << 5;
const REACTIVE_DISPOSED = 1 << 6;
/** L2 — this pass read the future (a tracked read of a CONFIG_HELD node).
 * Lives for the pass only: set by `read`, consumed by `recompute`'s publish
 * (a first pass that read the future is born held, F-7), wiped with the
 * pass's other per-pass bits. */
const REACTIVE_JOINED = 1 << 7;
const REACTIVE_SNAPSHOT_STALE = 1 << 8;
const REACTIVE_LAZY = 1 << 9;
const REACTIVE_MANUAL_WRITE = 1 << 10;
/** L2, rule 3 — the node's LAST pass was a render effect's read of the
 * future as committed (`frameRead`): the frame shows the committed world
 * beside a future it also derives from, and re-derives at the landing.
 * Survives its own pass's wipe; the next pass's wipe clears it (a pass that
 * re-derived the frame — in the future, or without the held read — owes the
 * landing nothing). The landing consumes it (the transaction's `_reruns`). */
const REACTIVE_FRAME_READ = 1 << 11;
/**
 * A dependency write landed while this subscriber was mid-recompute — a
 * nested pull committed beneath one of its reads (#3037). The heap refuses
 * RECOMPUTING nodes, so recompute's tail consumes this latch and reschedules:
 * values the pass read before the nested commit are stale. Only set for
 * links validated this pass (gen-current): a write to an untouched link is
 * either re-read later in the pass (fresh) or trimmed with it (not a dep).
 */
const REACTIVE_MISSED_WAKE = 1 << 12;
/** A `refresh()` asked this node to re-ask its question (F-9, D-23):
 * consumed by the next pass, which — going pending — classifies its flight
 * quiet (`_x._reask`). */
const REACTIVE_REASK = 1 << 13;
/** The pass was served a node's ambient staging — written this flush, not
 * held. If the flush parks, the pass is the frame's with it (the same sync
 * frame as the async): a verdict read cannot break it out. Per pass. */
const REACTIVE_STAGED_READ = 1 << 14;
/** The pass entered a verdict window (`isPending`/`latest`). Per pass: a
 * pass that did not keeps CONFIG_VERDICT no longer — the frame-reader
 * posture lasts exactly as long as the probing does. */
const REACTIVE_PROBED = 1 << 15;
/** Lanes — the node (a lane's member) was dirtied by the lane's own
 * re-staging (a re-guess, lane work re-derived): its next pass is the
 * lane's from the start (`recompute`), not a stale reader's re-run by an
 * unrelated write (#3460). Consumed by the pass. */
const REACTIVE_LANE_DIRTY = 1 << 16;
/** Lanes — the pass read a lane's value as lane work (`enterLane`): a
 * tracked derivation's read of displayed optimism, a verdict read routed
 * into the holder's verdict lane. A pass in a lane's seat that read none of
 * its world has left the lane (`recompute`'s tail). Per pass. */
const REACTIVE_LANE_READ = 1 << 17;
/** Lanes — a render effect in the frame's seat was served the screen
 * for a lane's node (`laneRead`: a stale reader of a lane that has not
 * shown). Its pass computing exactly what it last applied has nothing to
 * run — an effect has no comparator, and the guess's notification is not a
 * change it can see. Per pass. */
const REACTIVE_SCREEN_READ = 1 << 18;
/** A verdict reader whose probe was answered before the flush had a
 * transaction (verdict.ts `watchVerdict`): the seam re-runs it even if a
 * later read of the same pass routed it into a verdict lane — that was not
 * the probe's answer (lanes.ts). Per pass. */
const REACTIVE_PROBE_UNANSWERED = 1 << 19;
/** The pass opened a verdict window of its own (`isPending`/`latest`) — not
 * only read a verdict memo (F-6, core.ts `read`). An async memo whose pass
 * did not is D-25's hold work while its flight is up (verdict.ts
 * `holdWork`). Per pass. */
const REACTIVE_WINDOW = 1 << 21;
/** #4544: a render effect born into a hold read a settled verdict reader's
 * value this pass (core.ts `read`). Per pass: `recompute`'s tail keeps it as
 * CONFIG_AHEAD. */
const REACTIVE_AHEAD = 1 << 20;
/** An observation-lifecycle node found unobserved by a settle (#2934) while
 * a pass of it was already queued — a source's landing re-enqueued the
 * `_blocked` reader it walks (`settlePendingSource`). The release waits for
 * that pass: a memo nobody read has still produced what the flushes it
 * missed gave it (D-22, #4474), so the queued pass runs, then `recompute`'s
 * tail releases the node. Survives the head wipe; the tail consumes it. */
const REACTIVE_RELEASE_AFTER = 1 << 22;
/** A computed its own owner reads (a memo reading the memo it created): the
 * owner sits below it (heights put owners first), so a wake of it wakes the
 * owner too — the owner's pass re-creates it and is what its readers see. */
const REACTIVE_OWNER_READS = 1 << 23;
/** The windows the pass opened, by kind, from this bit up (`window <<
 * WINDOW_KINDS`): 1 `isPending`, 2 `latest`. Per pass (bits 24 and 25;
 * nothing sits above them). #4966: a first plain read that only the pass's
 * own `isPending` window gates is not a mount (core.ts `stagedScreen`). */
const WINDOW_KINDS = 24;
// Static configuration bits packed into Owner/Computed/Signal _config.
const CONFIG_OWNED_WRITE = 1 << 0;
const CONFIG_NO_SNAPSHOT = 1 << 1;
const CONFIG_TRANSPARENT = 1 << 2;
const CONFIG_IN_SNAPSHOT_SCOPE = 1 << 3;
const CONFIG_CHILDREN_FORBIDDEN = 1 << 4;
const CONFIG_AUTO_DISPOSE = 1 << 5;
const CONFIG_SYNC = 1 << 6;
// CARVE 3: REACTIVE_ZOMBIE, CONFIG_AUTHORITATIVE_READ, CONFIG_DIRECT_COMMIT,
// CONFIG_HELD_CHILDREN, CONFIG_INPUTS_PUBLISHED and CONFIG_ADOPTED_UNFLUSHED
// (transaction holds, zombies, until()'s authoritative view) went with the
// transactions.
// L2 (step 2): four bits, all maintained by the node's commit.
/** The node has a pass awaiting commit: its live children (and `_disposal`)
 * are that pass's and were never shown — a re-pass tears them down on the
 * spot, while the frame parked on `_x._pendingFirstChild` keeps waiting for
 * the commit. Set by `recompute` when it queues the node. */
const CONFIG_STAGED = 1 << 7;
/** The node's staged state is in the future: the flush that staged it
 * parked. A write to it, a tracked read of it by a derivation, or a pass
 * over a derivation holding it joins the future (the flush parks). Render
 * effects are the frame, not derivations (rule 3): reading it outside a
 * parking flush they see the committed value and hold nothing, and are
 * re-derived at the landing. Set at the park seam on every staged node. */
const CONFIG_HELD = 1 << 8;
/** A member of the uncommitted frame a held pass built (ruling A): set at
 * the park seam on every computed under a staged node (`holdFrame`). Never
 * shown, it has no screen: its reads are its transaction's, whichever flush
 * runs it (`bornInto`, #4010). Cleared by its commit. */
const CONFIG_HELD_FRAME = 1 << 20;
/** F-1 (#3305): a commit published this flight's inputs
 * beneath it — the node was pending, nothing staged, when it committed — so
 * its committed value is torn against the visible frame. A render effect
 * revealing it cannot be a stale reader: it observes the flight (pends,
 * holds) instead of showing the pre-flight value. Outlives the flight that
 * earned it for as long as the node stays pending, and no longer: the
 * commit that lands a value clears it. */
const CONFIG_INPUTS_PUBLISHED = 1 << 10;
/** Lanes — the node has a lane's value (`_x._lane`): a written guess
 * (CONFIG_GUESS) or a lane pass's derivation of one. Its lane is
 * `_x._transaction`. The gate to the lane arm of `read`; `_value` stays the
 * committed truth and `_pendingValue` a transaction's staging throughout —
 * a value's world is where it lives, never a bit. */
const CONFIG_OVERRIDE = 1 << 11;
/** A store slot node (`slotSignal`): a leaf, presence, container or deep
 * witness of a store target. One literal, no `_x` at birth; the last-one-out
 * sweep dispatches it to the store's shared release hook (graph.ts) instead
 * of a per-node `_unobserved` closure. */
const CONFIG_SLOT_NODE = 1 << 12;
/** A store slot node whose staging is a USER setter's proposal, not its
 * derive's derivation (D-8's discriminator for derived stores — the
 * leaf twin of REACTIVE_MANUAL_WRITE). Set by the user setter, cleared by
 * the derive's own write to the key. */
const CONFIG_MANUAL_WRITE = 1 << 18;
/** D-41: the staged value in `_pendingValue` is an error — an
 * answer of the held frame, published at its landing like a value. */
const CONFIG_STAGED_ERROR = 1 << 21;
/** Lanes — the node carries a written guess (`optimisticWrite`): its own
 * source recomputing it, or a plain write landing on it, is the truth
 * (`supersede`). A lane's derived staging is not. */
const CONFIG_GUESS = 1 << 13;
/** The node's pass read a verdict (`isPending`/`latest`): a dependency going
 * pending is a question for its next pass — the verdict changed — so the
 * propagation re-derives it instead of marking it pending (as a kept-tail
 * link does, D-10). Set at the window, never cleared: a reader that stopped
 * asking re-derives once and goes pending through its own read. The same
 * bit as REACTIVE_PROBED: a pass's probe (or a read of a verdict memo,
 * core.ts `read`) carries into the config at its end without a shift. */
const CONFIG_VERDICT = 1 << 15;
/** A dependency's status is a question for this node's pass, not a fact
 * about its current one: the propagation re-derives it instead of marking
 * it, for errors as for pending (as CONFIG_VERDICT). A boundary's output
 * (show the content, or the fallback) and its `on` node (boundaries.ts); a
 * promise over an expression (`resolve`/`until`, signals.ts: the pass reads
 * the error and rejects, or re-asks a pending source). */
const CONFIG_REDERIVE = 1 << 14;
/** An answered `createDeferred` node (deferred.ts): held because a
 * downstream hold staged its landing, its re-pass is a lagging question,
 * not an answer, and joins no transaction (D-40). */
const CONFIG_LAGS = 1 << 19;
/** The reader wants the truth, not the lane's guess (`until`, signals.ts):
 * a read of displayed optimism serves the base the guess covers, makes the
 * pass no lane's, and the truth landing wakes it even when it confirms the
 * guess (the one case ordinary subscribers are not told, F-11). */
const CONFIG_AUTHORITATIVE = 1 << 17;
// Presence bits (stage-3 hot-path monomorphism, DESIGN-PATCH-CHANNEL §11b):
// optional per-node slots (_overrideValue, _pendingSignal/_latestValueComputed,
// _snapshotValue, _optimisticLane) are NOT part of every node's hidden class —
// reading a missing property defeats V8's inline caches on the hottest write/
// notify loops. These bits live on the always-present `_config` so hot paths
// pay one monomorphic masked read and only touch the optional field when its
// installer flagged it. Bits are STICKY ("may be set") — the guarded field
// read remains authoritative.
const CONFIG_HAS_SNAPSHOT = 1 << 9;
/** Fresh-pull reader (awaitable `refresh()`'s waiter effect): a read of a
 * dirty source recomputes it inline even when the height gate defers to the
 * flush. Closes the same-flush ordering race where a waiter created
 * alongside a refresh() mark read the PRE-re-ask value as settled and
 * delivered stale; with the pull, the waiter either parks on the re-ask's
 * pending window (async — woken by the settle walk, which runs on every
 * landing including equal-value ones) or serves its sync answer. resolve()
 * deliberately keeps that race — its contract is "first settled value"
 * (#2930), not "next quiescent state". */
const CONFIG_FRESH_READ = 1 << 16;
/** F-10: the node was written inside a recompute that ran OUTSIDE a flush
 * (a creation-time compute — boundary machinery, a mapArray's first run). Such
 * a write is promoted at that recompute's end: readers in the same block see
 * it. Cleared when the next flush begins; set only on that rare path. */
const CONFIG_PROMOTED = 1 << 22;
/** Observe tiers only: the node is framework plumbing (the `solid-js/refresh`
 * HMR memo between a component's root and its body) — it has no name, is no
 * segment of any owner path, and the attribution engine records nothing about
 * it (creation, re-runs, checks) and does not count it toward another scope's
 * WIDE_SCOPE_DEPS or a write's engine-side HUGE_FAN_OUT, while the nodes it
 * owns stay fully observed.
 * Set from the internal `_plumbing` option at creation; never set in prod. */
const CONFIG_PLUMBING = 1 << 25;
/** Observe tiers only: a framework scope wide by construction (`@solidjs/web`'s
 * insert child-resolution pass, which must track each row's resolved child) —
 * the attribution engine's WIDE_SCOPE_DEPS skips it; the always-on HUGE_FAN_IN
 * does not. Set from the internal `_wide` option at creation by dev builds only
 * (the observe artifact mangles the option name, as it does `_plumbing`). */
const CONFIG_WIDE = 1 << 26;
/** Observe tiers only: a framework async node that supersedes its own flights
 * by design (`@solidjs/web/frames`' per-key slot-arg memo, re-run by every
 * re-shipped record while the arg is still pending) — the attribution
 * engine's ABANDONED_FLIGHTS skips it; its flight records and the feedback
 * fold's counts are unchanged. Set from the internal `_supersedes` option at
 * creation by dev builds only (the observe artifact mangles the option name,
 * as it does `_wide`). */
const CONFIG_SUPERSEDES = 1 << 27;
/** Observe tiers only: the memo `merge()` creates for a function source — a
 * key bag readers consume key by key, built by the framework (a compiled
 * component spread) with options no user can reach. The attribution engine's
 * UNSTABLE_MEMO_OUTPUT skips it: a fresh-but-equal bag is the idiom
 * (`{...(on() ? { a } : {})}`), and the advice (`equals`) cannot be applied
 * (#4256). Set through `_extraConfig` by observe builds only. */
const CONFIG_KEY_BAG = 1 << 28;
/** The frame (rule 3, D-3): a render effect (set at creation, as
 * `EFFECT_RENDER << 23` — keep the two in step), and a boundary's tree and
 * output (boundaries.ts) — structure between the content and the render
 * effect that shows it, not a user derivation of the content. Outside a
 * hold's own flush it reads that hold's values as committed and is
 * re-derived at its landing, and a re-pass of it publishes mainline: it
 * joins nothing, so two mounts under one boundary made in separate flushes
 * keep their holds apart (D-4, S-5 rows 1′–3′, #4283). */
const CONFIG_FRAME = 1 << 23;
/** A render effect created by a pass that probed (`isPending`/`latest`) —
 * the mount a display-ahead `Show` opens (S-46, D-25). Stamped at its first
 * pass; its first read of a hold's staging or flight is born held, not a
 * D-3 leaf of the screen, and the verdict lane that opened it reveals with
 * the landing (lanes.ts `mountPending`). */
const CONFIG_MOUNT_AHEAD = 1 << 24;
/** The node's last answering pass registered a flight: an async memo. Kept
 * by the verdict engine alone (verdict.ts `holdWork`), which reads it. */
const CONFIG_ASYNC = 1 << 29;
/** #4544: the node's last pass opened a verdict window of its own
 * (`isPending`/`latest`) — REACTIVE_WINDOW, kept by `recompute`'s tail
 * beside CONFIG_VERDICT and renewed by every pass. One whose posture is only
 * inherited (it read a verdict memo) is the hold's work where it reads the
 * hold's own write (verdict.ts `holdStaging`). */
const CONFIG_OWN_PROBE = REACTIVE_WINDOW << 9;
/** #4544 (F-6): the effect's last pass read a display-ahead value
 * (REACTIVE_AHEAD): what it went pending on does not hold its hold
 * (scheduler.ts `blockedBy`). The sign bit: tested by `&` only. */
const CONFIG_AHEAD = REACTIVE_AHEAD << 11;
const STATUS_NONE = 0;
const STATUS_PENDING = 1 << 0;
const STATUS_ERROR = 1 << 1;
const STATUS_UNINITIALIZED = 1 << 2;
/** An error that answered a node's first read (no value has ever landed):
 * the error, not UNINITIALIZED, is its status while shown, but a re-ask
 * makes it uninitialized-pending again — it still has no value (#4244). */
const STATUS_NO_VALUE = 1 << 3;
/** `catchHook`'s answer when the catcher is a boundary re-armed by
 * `on` (truthy, as any catch): what it collects is its own, not the hold's. */
const CAUGHT_REARMED = 2;
const EFFECT_RENDER = 1;
const EFFECT_USER = 2;
const EFFECT_TRACKED = 3;
const NOT_PENDING = {};
const NO_SNAPSHOT = {};
const SUPPORTS_PROXY = typeof Proxy === "function";
const defaultContext = {};
/**
 * Brand symbol used by `Refreshable<T>` values (projection stores, async
 * memos) to expose their underlying computation to `refresh()`. Not part of
 * the user-facing API.
 *
 * @internal
 */
const $REFRESH = Symbol("refresh");

const NewSet = Set;
/** Dev: the strict-read label compiled dev DOM output reads an event handler expression under (#1742). */
const EVENT_HANDLER_READ = "an event handler expression";

const COPY = { rooted: false };
let warned = false;
let shared;
{
  shared = globalThis.Solid$$copies ||= { copies: 0, running: undefined };
  shared.copies++;
}
function warnOnce(detail) {
  if (warned) return;
  warned = true;
  console.warn(
    `You appear to have multiple instances of Solid: ${detail} Make sure every package resolves the same solid-js (e.g. \`resolve.dedupe\` in Vite) and that libraries keep solid-js external.`
  );
}
function markCopyRooted() {
  COPY.rooted = true;
}
/**
 * Marks this copy as the one running; returns the slot's previous value. A
 * nested run of the same copy (most runs inside a flush) only reads the slot.
 */
function enterCopy() {
  if (shared.copies < 2) return COPY;
  const prev = shared.running;
  if (prev !== COPY) shared.running = COPY;
  return prev;
}
function exitCopy(prev) {
  if (prev !== COPY) shared.running = prev;
}
/** @internal Runs `fn` with this copy marked as the one running. */
function runAsSolidCopy(fn) {
  if (shared.copies < 2) return fn();
  const prev = enterCopy();
  try {
    return fn();
  } finally {
    exitCopy(prev);
  }
}
/** @internal An ownerless component or context read: warns if another copy is running. */
function checkForeignOwnerless() {
  if (shared.copies < 2) return;
  const running = shared.running;
  if (running && running !== COPY)
    warnOnce(
      "a component or useContext() from one copy ran inside a component rendered by another, so it has no owner and will not react or find its providers."
    );
}
/**
 * An untracked read: warns if another copy's computation is running, unless
 * both copies render through roots of their own (an app calling into an
 * isolated copy's API, which reads its own state untracked by design).
 */
function checkForeignRead() {
  if (shared.copies < 2) return;
  const running = shared.running;
  if (running && running !== COPY && !(running.rooted && COPY.rooted))
    warnOnce(
      "a signal or store from one copy was read inside a computation of another, so the read is not tracked and will not update."
    );
}

let attrHooks = null;
/**
 * The installed engine, registered on `globalThis` as well (the records
 * channel's reason, see `Records`): a wire layer bundled without a framework
 * import — `@solidjs/web`'s server-function client — stamps the records it
 * emits through the engine's `currentOrigin`, and this is its reach. The
 * module binding stays the core's own read (one null check per hook site);
 * the registration mirrors it.
 */
const INSTALLED = Symbol.for("@solidjs/signals/observe/attribution");
function setAttributionHooks(hooks) {
  attrHooks = hooks;
  globalThis[INSTALLED] = hooks ?? undefined;
}
let storeTools = null;
function registerStoreTools(tools) {
  storeTools = tools;
}
/**
 * Run `fn` as the handler of a user interaction: every root write it performs
 * (and every action step, effect or flight the write causes) is attributed to
 * `ref` by whichever engine is installed. The web runtime wraps event
 * dispatch in this; custom renderers and test harnesses call it themselves.
 * With no engine installed it is `fn()` — the wiring, not the engine, so it
 * lives in core and is reachable as `OBSERVE.attribution.withInteraction`.
 *
 * The `finally` is deliberate and safe under the try rule above: this
 * function is referenced only from the `OBSERVE` object, which prod builds
 * fold to `undefined`, so nothing retains it there.
 */
function withInteraction(ref, fn) {
  // Pin the engine for the frame: a handler that disables it mid-way must
  // still close the frame it opened (the engine tolerates a close after
  // disable()), and one that enables it mid-way opened no frame to close.
  const hooks = attrHooks;
  if (hooks === null) return fn();
  hooks.interactionStart(ref);
  let returned;
  try {
    return (returned = fn());
  } finally {
    hooks.interactionEnd(returned);
  }
}
/**
 * Run `fn` as a declared unit of work — a router's navigation: every root
 * write it performs is attributed to `ref` (and, through it, to the enclosing
 * interaction when there is one), so the hold those writes wait in, the
 * re-runs they cause and the verdicts on them all carry the route's name
 * instead of a bare signal's. Same contract as `withInteraction`: the wiring,
 * not the engine; `fn()` with no engine installed. Reachable as
 * `OBSERVE.attribution.withOrigin`.
 *
 * ```ts
 * OBSERVE
 *   ? OBSERVE.attribution.withOrigin(
 *       { kind: "navigation", name: match.pattern, to, from, params: match.params },
 *       () => setLocation(to)
 *     )
 *   : setLocation(to);
 * ```
 */
function withOrigin(ref, fn) {
  const hooks = attrHooks;
  if (hooks === null) return fn();
  hooks.originStart(ref);
  try {
    return fn();
  } finally {
    hooks.originEnd();
  }
}
/**
 * The provenance a root write performed now would carry, as the installed
 * engine sees it (`AttributionHooks.currentOrigin`); `undefined` with no
 * engine, or when nothing is in effect. Reachable as
 * `OBSERVE.attribution.currentOrigin` — how a runtime stamps a fact of its
 * own (a server-function call) with the interaction or navigation it ran
 * for, so an observer joins the two by identity.
 */
function currentOrigin() {
  const hooks = attrHooks;
  return hooks === null ? undefined : hooks.currentOrigin();
}

/** First warning when a change reaches (or a pass tracks) this many edges. */
const GRAPH_SIZE_WARN_AT = 2000;
/** Re-warn once the count has grown by this much since the last warning. */
const GRAPH_SIZE_WARN_EVERY = 500;
let writeDevStoreSlot;
/**
 * @internal The store package registers the slot-write path so this module
 * does not import the store (avoids a core ↔ store cycle).
 */
function registerDevStoreSlotWriter(fn) {
  writeDevStoreSlot = fn;
}
/**
 * Devtools write entry for a reactive node. Prefer the app's own setters in
 * application code; this exists for tools that only have the node.
 */
function setDevSignal(node, value) {
  runWithOwner(null, () => {
    if (node._config & CONFIG_SLOT_NODE && writeDevStoreSlot) {
      writeDevStoreSlot(node, value);
      return;
    }
    setSignal(node, value);
  });
}
const hooks = {};
const diagnosticListeners = new Set();
const diagnosticCaptures = new Set();
let diagnosticSequence = 0;
let consoleFooter;
const footeredCodes = new Set();
/**
 * Registers the console footer appended to the first console report of
 * each diagnostic code — a discovery pointer to deeper guidance. Reported
 * events carry it as trailing lines of the same console entry; events that
 * surface as a thrown error instead get it as a follow-up line. Returning
 * undefined for an event suppresses the footer. Passing undefined
 * unregisters and resets the once-per-code memory.
 *
 * @internal A seam for `solid-js`, which owns the repair skill the footer
 * names and installs it from both of its entries; not part of `DEV`. No-op
 * outside dev builds, where nothing reports to the console.
 */
function setConsoleFooter(footer) {
  consoleFooter = footer;
  footeredCodes.clear();
}
const diagnostics = {
  subscribe(listener) {
    diagnosticListeners.add(listener);
    return () => diagnosticListeners.delete(listener);
  },
  emit(event, subject = null) {
    return emitDiagnostic(event, subject);
  },
  capture() {
    const events = [];
    diagnosticCaptures.add(events);
    return {
      get events() {
        return events;
      },
      clear() {
        events.length = 0;
      },
      stop() {
        diagnosticCaptures.delete(events);
        return [...events];
      }
    };
  }
};
// Flight identity → earliest declared start (see `AttributionSlot.markFlight`).
// Weak: a stamp must not keep a settled promise alive.
const flightOrigins = new WeakMap();
const attributionSlot = {
  get installed() {
    return attrHooks;
  },
  withInteraction,
  withOrigin,
  currentOrigin,
  markFlight(flight, startedAt = performance.now()) {
    // Earliest wins (`undefined <= n` is false: the first mark always lands).
    if (!(flightOrigins.get(flight) <= startedAt)) flightOrigins.set(flight, startedAt);
  },
  flightOrigin: flight => flightOrigins.get(flight)
};
const RECORDS = Symbol.for("@solidjs/signals/observe/records");
function recordsChannel() {
  const g = globalThis;
  if (g[RECORDS]) return g[RECORDS];
  const listeners = new Map();
  const bodyListeners = new Map();
  return (g[RECORDS] = {
    subscribe(type, listener, options) {
      const current = listeners.get(type);
      // Set semantics: one entry per function, however often it is passed —
      // the first subscription's options stand for it.
      if (current === undefined || !current.includes(listener)) {
        listeners.set(type, current === undefined ? [listener] : [...current, listener]);
        if (options !== undefined && options.bodies) {
          let wanting = bodyListeners.get(type);
          if (wanting === undefined) bodyListeners.set(type, (wanting = new Set()));
          wanting.add(listener);
        }
      }
      return () => {
        const list = listeners.get(type);
        if (list === undefined || !list.includes(listener)) return;
        const next = list.filter(l => l !== listener);
        if (next.length > 0) listeners.set(type, next);
        else listeners.delete(type);
        const wanting = bodyListeners.get(type);
        if (wanting !== undefined && wanting.delete(listener) && wanting.size === 0)
          bodyListeners.delete(type);
      };
    },
    observed(type, facet) {
      return facet === "bodies" ? bodyListeners.has(type) : listeners.has(type);
    },
    emit(type, event, live) {
      const list = listeners.get(type);
      if (list === undefined) return;
      // A throwing listener is reported and the rest still hear the record.
      // The try/catch per call allocates nothing unless something throws.
      for (let i = 0; i < list.length; i++) {
        try {
          list[i](event, live);
        } catch (error) {
          console.error(error);
        }
      }
    }
  });
}
const OBSERVE = {
  diagnostics,
  records: recordsChannel(),
  attribution: attributionSlot,
  // Replaced by solid-js's server entry (see `ServerObserve`); on the
  // client the slot stays this placeholder. The cast: the interface is
  // empty HERE and gains its members by augmentation downstream.
  server: {},
  exclude(owner) {
    markedOwners.set(owner, true);
    hasExclusions = true;
  },
  include(owner) {
    // No flag flip: with nothing excluded there is nothing to re-admit,
    // and the walk stays short-circuited.
    markedOwners.set(owner, false);
  },
  isExcluded,
  ownerPath
};
// --- Excluded owners ---------------------------------------------------------------
//
// An observer that lives inside the observed app (an adapter's panel,
// devtools) marks its root; both channels check the subject's owner chain —
// the same walk `ownerPath` already makes — and stay silent under it. An
// observer that WRAPS the app (a toolbar around it) excludes its own root
// and includes the app's back: one map, `true` excluded / `false` included,
// and the nearest marker up the chain decides. The flag short-circuits the
// walk for the common case of no exclusions (an include with nothing
// excluded changes no verdict, so it does not flip it).
const markedOwners = new WeakMap();
let hasExclusions = false;
/** Events built for an excluded subject: never delivered, never reported. */
const suppressedEvents = new WeakSet();
function isExcluded(subject) {
  if (!hasExclusions || !subject) return false;
  let owner = "_parent" in subject ? subject : (subject._owner ?? null);
  for (; owner !== null; owner = owner._parent) {
    const marked = markedOwners.get(owner);
    if (marked !== undefined) return marked;
  }
  return false;
}
/** For engines that cache the verdict per node: is anything excluded at all? */
function anyExcluded() {
  return hasExclusions;
}
/** Was `entry` built for an excluded subject? Once-per-key reporters must not spend their slot on it. */
function isSuppressed(entry) {
  return suppressedEvents.has(entry);
}
// The repair guide: the `reactivity-diagnostics` skill `solid-js` ships,
// one page per code on the docs site. The slug is the code lowercased with
// `_` → `-` (`STRICT_READ_UNTRACKED` → `strict-read-untracked`).
const GUIDE_URL = "https://docs.solidjs.com/reference/solid-js/diagnostics/";
const DEV = {
  hooks,
  getChildren,
  getSignals,
  getParent,
  getSources,
  getObservers,
  setSignal: setDevSignal,
  report: reportDiagnostic,
  guideUrl(code) {
    return GUIDE_URL + code.toLowerCase().replace(/_/g, "-");
  }
};
/**
 * Dev-mode internal consistency check. A failure means the reactive system
 * contradicted itself (not that user code misbehaved) — see
 * INTERNALS-ASYNC-STATE.md for the invariant catalog. Throws under false
 * so the suite (and fuzzing) treats any violation as a hard failure; logs in
 * dev builds so user apps degrade instead of crashing.
 */
function assertInvariant(condition, name, message) {
  if (condition) return;
  const full = `[INVARIANT_VIOLATION] ${name}: ${message}`;
  const entry = emitDiagnostic({
    code: "INVARIANT_VIOLATION",
    kind: "error",
    severity: "error",
    message: full,
    data: { invariant: name }
  });
  reportDiagnostic(entry);
}
/**
 * Root-first names of the owners enclosing `subject` (inclusive when the
 * subject is itself a named owner). Signals hop to their registering owner
 * (`_owner`, set by registerGraph). Unnamed owners are skipped so the path
 * reads as the component tree plus the scope: `<App> › <TodoRow> › effect`.
 * Public as `OBSERVE.ownerPath`; the core's own sites import it directly.
 */
function ownerPath(subject) {
  if (!subject) return undefined;
  let owner = "_parent" in subject ? subject : (subject._owner ?? null);
  const path = [];
  for (; owner !== null; owner = owner._parent) {
    const name = owner._name;
    if (typeof name === "string" && name.length) path.push(name);
  }
  return path.length ? path.reverse() : undefined;
}
/**
 * The innermost source location on the subject's chain: the subject's own
 * `_loc` (a signal or computation created with `{ loc }`), else the first
 * `_loc` up the owners — a signal hops to its registering owner, then
 * `_parent` upward (a component root carries its JSX site). The same walk
 * `ownerPath` makes; `undefined` when nothing on the chain was located.
 */
function locationOf(subject) {
  // Owners have a `_parent` key (null on a root); a signal has none and hops
  // to its registering `_owner` instead.
  for (let node = subject; node; node = node._parent !== undefined ? node._parent : node._owner)
    if (node._loc) return node._loc;
  return undefined;
}
/**
 * Records a diagnostic on the structured channel (listeners, captures) and
 * returns the entry. `subject` locates it: the current reactive `context` by
 * default (right for the synchronous rule checks — they fire inside the
 * scope that misbehaved); pass the node for scheduler-time findings whose
 * ambient context is the flush, or `null` for events that have no location
 * by nature. An `ownerPath` (or `loc`) already on the event wins over the
 * subject walk (hosts whose owners are not signals' owners compute their
 * own). Console output is a separate, dev-tier step — see `reportDiagnostic`.
 */
const OPTIMISTIC_WRITE_BEFORE_LOAD_MESSAGE =
  "[OPTIMISTIC_WRITE_BEFORE_LOAD] Optimistic write to a source that has not loaded yet. " +
  "Before its first answer the source is loading, not showing a value to be optimistic over. " +
  "Keep the pending items in a separate list (an outbox) until the source loads, " +
  "or disable the action until the source has loaded.";
/** DEV (#4656): an optimistic write to a derived optimistic store whose
 * first answer has not landed (`source`: the store's derive). An optimistic
 * memo does not guard (S-56); the store form keeps the guard until the store
 * engine can show such a guess: its screen stays loading while an untracked
 * read sees the guess. Called under `true` only. */
function devGuardOptimisticBeforeLoad(source) {
  if (source && source._statusFlags & STATUS_UNINITIALIZED) {
    emitDiagnostic({
      code: "OPTIMISTIC_WRITE_BEFORE_LOAD",
      kind: "write",
      severity: "error",
      message: OPTIMISTIC_WRITE_BEFORE_LOAD_MESSAGE,
      data: { operation: "optimistic" }
    });
    throw new Error(OPTIMISTIC_WRITE_BEFORE_LOAD_MESSAGE);
  }
}
const OPTIMISTIC_WRITE_WHILE_LOADING_MESSAGE =
  "[OPTIMISTIC_WRITE_WHILE_LOADING] Optimistic write to a source that is loading a new question. " +
  "The guess is an edit of the previous answer, and the new answer replaces it (D-34). " +
  "Keep the control unavailable until the source has loaded (inside its `Loading`, or disabled " +
  "while it is pending), or keep the pending change in a separate list until it lands.";
/** A `refresh()` re-ask is the same question (D-34): its own, or every
 * source the node waits on re-asked. */
const reasked = n =>
  !!(n._flags & REACTIVE_REASK || n._x?._reask) ||
  (!!n._x?._pendingSources?.size &&
    [...n._x._pendingSources].every(s => s !== n && (s._flags & REACTIVE_REASK || s._x?._reask)));
const warnedWhileLoading = new WeakSet();
/** DEV (#4795): an optimistic write to a source that has
 * answered before and is loading a new question (an input changed): the
 * setter composes on the previous answer, and the new one replaces the
 * guess (D-34). Not before the first answer (S-56: the setter gets
 * `undefined`), not over a `refresh()` (the same question), not a node
 * already carrying a guess. Once per source. Called under `true` only. */
function devWarnOptimisticWhileLoading(n) {
  if (
    n &&
    (n._statusFlags & (STATUS_PENDING | STATUS_UNINITIALIZED)) === STATUS_PENDING &&
    !(n._config & CONFIG_OVERRIDE) &&
    !reasked(n) &&
    !warnedWhileLoading.has(n)
  ) {
    warnedWhileLoading.add(n);
    reportDiagnostic(
      emitDiagnostic(
        {
          code: "OPTIMISTIC_WRITE_WHILE_LOADING",
          kind: "write",
          severity: "warn",
          message: OPTIMISTIC_WRITE_WHILE_LOADING_MESSAGE,
          nodeName: n._name,
          data: { operation: "optimistic" }
        },
        n
      )
    );
  }
}
function emitDiagnostic(event, subject = context) {
  const entry = {
    sequence: ++diagnosticSequence,
    ...event
  };
  // The observer's own subtree: build the entry (the caller may throw its
  // message) but tell nobody.
  if (isExcluded(subject)) {
    suppressedEvents.add(entry);
    return entry;
  }
  if (entry.ownerPath === undefined) {
    const path = ownerPath(subject);
    if (path) entry.ownerPath = path;
  }
  if (entry.loc === undefined) {
    const loc = locationOf(subject);
    if (loc !== undefined) entry.loc = loc;
  }
  const live = subject ?? undefined;
  if (live !== undefined) eventSubjects.set(entry, live);
  for (const listener of diagnosticListeners) listener(entry, live);
  for (const capture of diagnosticCaptures) capture.push(entry);
  // Footer for events that never reach reportDiagnostic because the call site
  // throws the message instead (every such site is severity "error"): a
  // microtask lands it below the thrown error. Sites that DO report consume
  // the once-per-code slot synchronously first, so this finds it taken and
  // stays silent — one console entry per finding. Advisory (`info`) events
  // are structured-channel only and get no footer: nothing on the console
  // for it to follow. Dev-tier: the footer is console guidance.
  if (entry.severity === "error" && consoleFooter && !footeredCodes.has(entry.code)) {
    queueMicrotask(() => {
      const footer = takeFooter(entry);
      if (footer) console.warn(footer);
    });
  }
  return entry;
}
/** The once-per-code footer text, consuming the slot. Undefined if taken or unregistered. */
function takeFooter(entry) {
  if (!consoleFooter || footeredCodes.has(entry.code)) return undefined;
  footeredCodes.add(entry.code);
  return consoleFooter(entry);
}
/**
 * The subject each emitted event was about, for the console step, which
 * runs after `emitDiagnostic` returned and can show what the node knows: a
 * rendering runtime may stamp a binding effect with the DOM element it
 * writes (`_devElement`), and a live element reference beside the message
 * is the most addressable pointer a console can print. Listeners get the
 * subject as their second argument instead; the map exists only to carry
 * it from `emitDiagnostic` to `reportDiagnostic` across the call site's
 * `reportDiagnostic(emitDiagnostic(…))`. Weak, keyed by the entry.
 */
const eventSubjects = new WeakMap();
/**
 * The `at` line of a console report: the event's `loc` as the compiler wrote
 * it (`src/App.tsx:18:3`), or — in a browser, where a dev server serves the
 * project root at `location.origin` — `http://localhost:5173/src/App.tsx:18:3`,
 * which Chrome and Firefox link to the Sources panel. A path that leaves the
 * root (`../`) or is absolute is not under the origin and prints as-is.
 */
function locationLine(loc) {
  if (
    typeof document !== "undefined" &&
    typeof location !== "undefined" &&
    !loc.startsWith("..") &&
    !loc.startsWith("/")
  )
    return `${location.origin}/${loc}`;
  return loc;
}
/**
 * The locating lines of a console report: `\n  in <App> › <TodoRow> › effect`
 * (the owner path) and `\n  at src/App.tsx:18:3` (see `locationLine`), each
 * when the entry carries it; `""` when it carries neither. Shared by
 * `reportDiagnostic` and the sites that print their own console entry (the
 * halt, which logs its cause beside the message).
 */
function locationLines(entry) {
  let text = "";
  if (entry.ownerPath) text += `\n  in ${entry.ownerPath.join(" › ")}`;
  if (entry.loc) text += `\n  at ${locationLine(entry.loc)}`;
  return text;
}
/**
 * The console face of a diagnostic — ONE entry per finding: the message, the
 * owner path (`in <App> › <TodoRow> › effect`) so a human can locate it, the
 * source location (`at src/App.tsx:18:3` — see `locationLine`) when a node on
 * the chain was compiled with one, the once-per-code footer as trailing
 * lines, and — when the subject is a binding effect the rendering runtime
 * tagged — the element it writes, as a second console argument (hover
 * highlights it, click jumps to Elements).
 * Severity picks the console method. Call sites report the entry
 * `emitDiagnostic` returned so the structured and console channels never
 * disagree. Dev-tier: in an observe build this is a no-op, so wiring paths
 * that both emit and report (graph-size warnings) reach the channel only —
 * production observability never writes to the console.
 */
function reportDiagnostic(entry) {
  if (suppressedEvents.has(entry)) return;
  let text = entry.message + locationLines(entry);
  const footer = takeFooter(entry);
  if (footer) text += `\n${footer}`;
  const element = eventSubjects.get(entry)?._devElement;
  const args = element !== undefined ? [text, element] : [text];
  entry.severity === "error" ? console.error(...args) : console.warn(...args);
}
/**
 * Shared strict-read diagnostics for core read() and the store proxy traps.
 * Single source for the message text — the #2897 safeguard parity between
 * memos and stores is exactly these firing identically from both paths.
 */
function throwPendingUntrackedRead(strictReadLabel, fields) {
  const message =
    `[PENDING_ASYNC_UNTRACKED_READ] Reading a pending async value directly in ${strictReadLabel}. ` +
    (strictReadLabel === EVENT_HANDLER_READ
      ? `Event handlers are not reactive: read the value inside the handler function instead.`
      : `Async values must be read within a tracking scope (JSX, a memo, or an effect's compute function).`);
  emitDiagnostic({
    code: "PENDING_ASYNC_UNTRACKED_READ",
    kind: "async",
    severity: "error",
    message,
    ...fields,
    data: { strictRead: strictReadLabel }
  });
  throw new Error(message);
}
function warnStrictReadUntracked(strictReadLabel, fields) {
  // Name the value when the name says something — a `name` option or a store
  // key (#3675). The constructors' defaults ("signal", "computed") would only
  // restate what "reactive value" already says.
  const nodeName = fields?.nodeName;
  const named = nodeName !== undefined && nodeName !== "signal" && nodeName !== "computed";
  const value = `[STRICT_READ_UNTRACKED] Reactive value${named ? ` "${nodeName}"` : ""} read `;
  const message =
    strictReadLabel === EVENT_HANDLER_READ
      ? // #1742: the read is tracked (the element re-mounts when it changes),
        // but a handler is bound once: the repair is to read inside it.
        value +
        `in ${strictReadLabel}. Event handlers are not reactive: ` +
        `read the value inside the handler function instead.`
      : value +
        `directly in ${strictReadLabel} will not update. ` +
        `Move it into a tracking scope (JSX, a memo, or an effect's compute function).`;
  reportDiagnostic(
    emitDiagnostic({
      code: "STRICT_READ_UNTRACKED",
      kind: "strict-read",
      severity: "warn",
      message,
      data: { strictRead: strictReadLabel },
      ...fields
    })
  );
}
/**
 * Observe-tier: stamp a signal with its creating owner so `ownerPath` can
 * locate signal subjects. The per-owner `_signals` list and the devtools
 * `onGraph` hook are dev-tier — the observe build pays one property write.
 */
function registerGraph(value, owner) {
  value._owner = owner;
  {
    if (owner) {
      if (!owner._signals) owner._signals = [];
      owner._signals.push(value);
    }
    DEV.hooks.onGraph?.(value, owner);
  }
}
function clearSignals(node) {
  node._signals = undefined;
}
/**
 * Observe-tier: the live top-level roots — owners created with no parent
 * (`render()`'s root, a `createRoot()` at module scope, a devtools panel).
 * Held weakly so an undisposed root nothing references still collects; a
 * root a subscription keeps alive is exactly the leak `graphSize()` exists
 * to count. Registered by `createOwner` and released by its disposal, so
 * the cost is one Set write per top-level root, never per node.
 */
const liveRoots = new Set();
const rootRefs = new WeakMap();
const rootReaper =
  typeof FinalizationRegistry === "function"
    ? new FinalizationRegistry(ref => liveRoots.delete(ref))
    : null;
function registerRoot(owner) {
  // Observe-tier bodies: folded to a bare return in prod so the mangler and
  // the bundle never see the walk.
  if (typeof WeakRef !== "function") return;
  const ref = new WeakRef(owner);
  rootRefs.set(owner, ref);
  liveRoots.add(ref);
  rootReaper?.register(owner, ref, ref);
}
function unregisterRoot(owner) {
  const ref = rootRefs.get(owner);
  if (ref === undefined) return;
  rootRefs.delete(owner);
  liveRoots.delete(ref);
  rootReaper?.unregister(ref);
}
/**
 * The live top-level roots, for a walk of the owner tree (the engine's
 * `graphSize`): dead refs are dropped as they are met. Empty in prod.
 */
function liveRootOwners() {
  const out = [];
  for (const ref of liveRoots) {
    const root = ref.deref();
    if (root === undefined) liveRoots.delete(ref);
    else out.push(root);
  }
  return out;
}
// Graph traversal helpers
function getChildren(owner) {
  const children = [];
  let child = owner._firstChild;
  while (child) {
    children.push(child);
    child = child._nextSibling;
  }
  return children;
}
function getSignals(owner) {
  return owner._signals ? [...owner._signals] : [];
}
function getParent(owner) {
  return owner._parent;
}
function getSources(computation) {
  const sources = [];
  let link = computation._deps;
  while (link) {
    sources.push(link._dep);
    link = link._nextDep;
  }
  return sources;
}
function getObservers(node) {
  const observers = [];
  let link = node._subs;
  while (link) {
    observers.push(link._sub);
    link = link._nextSub;
  }
  return observers;
}
/**
 * Graph-size warnings are once per node, re-warning only when the count has
 * grown by GRAPH_SIZE_WARN_EVERY since the last one — off-node, so the
 * pathological handful of nodes that ever reach the threshold are the only
 * ones that cost anything, and no node carries a bookkeeping field for it.
 */
const graphSizeWarnedAt = new WeakMap();
function shouldWarnGraphSize(node, count) {
  const last = graphSizeWarnedAt.get(node);
  if (last !== undefined && count < last + GRAPH_SIZE_WARN_EVERY) return false;
  graphSizeWarnedAt.set(node, count);
  return true;
}
/**
 * Observe-tier: a committed change on `node` is about to re-run `count`
 * subscribers. Two reporters, one finding, one dedupe: the core counts the
 * notify walk in `insertSubs` as it goes (fan-out costs exactly one local
 * increment in a loop that already visits every edge, and nothing at link
 * time) and fires from GRAPH_SIZE_WARN_AT up, always-on wherever the
 * channel exists — a graph-size pathology should surface without asking.
 * The attribution engine, while enabled, reports the same code from its
 * lower `fanOut` threshold (default 250) on the root writes it stamps, with
 * the write kind it knows, and hands over to the core at
 * GRAPH_SIZE_WARN_AT so one change never carries two findings. Both fire on
 * the write rather than the link: a fan-out that is never written costs
 * nothing, and one that is re-runs every subscriber this flush. Once per
 * node, re-warning only once the count has grown by GRAPH_SIZE_WARN_EVERY.
 */
function noteFanOut(node, count, write) {
  if (!shouldWarnGraphSize(node, count)) return;
  const name = node._name;
  const message =
    `[HUGE_FAN_OUT] ${name ? `Signal "${name}"` : "A signal"} changed with ${count} subscribers — ` +
    `every one re-runs this flush. If many independent computations read the same value ` +
    `(for example every row of a list comparing against one selected id), prefer a per-key ` +
    `store or projection so only the items whose result flipped update.`;
  reportDiagnostic(
    emitDiagnostic(
      {
        code: "HUGE_FAN_OUT",
        kind: "graph",
        severity: "warn",
        message,
        nodeName: name,
        ownerId: node.id,
        ownerName: name,
        data: write === undefined ? { count } : { count, write }
      },
      node
    )
  );
}
/**
 * Observe-tier: a recompute pass of `node` tracked `count` distinct sources
 * (its trimmed dep list, walked once at the end of the pass — see recompute;
 * no per-link work, no pass bracket). Fires from GRAPH_SIZE_WARN_AT up.
 */
function noteFanIn(node, count) {
  if (!shouldWarnGraphSize(node, count)) return;
  const name = node._name;
  const message =
    `[HUGE_FAN_IN] ${name ? `Computation "${name}"` : "A computation"} tracked ${count} sources. ` +
    `It will re-run when any of them change. Narrow the read or split the derivation so each ` +
    `computation tracks only what it needs.`;
  reportDiagnostic(
    emitDiagnostic(
      {
        code: "HUGE_FAN_IN",
        kind: "graph",
        severity: "warn",
        message,
        nodeName: name,
        ownerId: node.id,
        ownerName: name,
        data: { count }
      },
      node
    )
  );
}
/**
 * Dev-only: attribute reads that run after an `await` inside an async compute.
 * JS has no async context, so a native-promise flight is consumed by a uniquely
 * named async function instead of `.then`; V8's async stack traces then carry a
 * `__solidAsyncCompute_<id>` frame on every read in the compute's continuation.
 * `await` on a native promise settles on the same tick as `.then`, so dev and
 * prod timing match. Other thenables, and engines without async frames
 * (Firefox, Safari), are left alone and never warn.
 */
let asyncTailFlights = 0;
// Weak so a flight that never settles cannot pin its computation (the promise's
// reactions close over it); dead or superseded entries are swept as the registry grows.
const tailFlights = new Map();
const warnedTailReads = new WeakMap();
let tailFlightId = 0;
const TAIL_FRAME = /^__solidAsyncCompute_(\d+)$/;
function watchAsyncTail(el, flight) {
  if (!(flight instanceof Promise) || flight.constructor !== Promise) return flight;
  return {
    then(onFulfilled, onRejected) {
      const id = ++tailFlightId;
      const name = `__solidAsyncCompute_${id}`;
      // Retire the entry before settling so reads made by the landing itself stay quiet.
      const done = () => {
        if (tailFlights.delete(id)) asyncTailFlights--;
      };
      if (tailFlights.size >= nextTailSweep) sweepTailFlights();
      asyncTailFlights++;
      tailFlights.set(id, { el: new WeakRef(el), flight: new WeakRef(flight) });
      ({
        async [name]() {
          let value;
          try {
            value = await flight;
          } catch (error) {
            done();
            return onRejected(error);
          }
          done();
          onFulfilled(value);
        }
      })[name]();
      return undefined;
    }
  };
}
/**
 * `dep` is the node a tracked read would have linked (undefined when a store key
 * was never tracked); `holder`/`key` identify the read for once-only reporting.
 */
function checkPostAwaitRead(dep, holder, key, nodeName, throwsPending) {
  // Continuations resume with no owner; mount, flush, and effect reads always have one.
  if (context !== null) return;
  // Code Solid itself runs synchronously on the continuation's stack also has
  // no owner: effect callbacks (the continuation called flush()), cleanups (it
  // called dispose()), and an action's body (it invoked the action). Their
  // reads are imperative by design, and the async frame below them belongs to
  // the caller, not to them.
  if (callbackDepth !== 0) return;
  if (disposalDepth !== 0) {
    // Teardown is synchronous, so a depth still raised on the next microtask
    // was left behind by a throw that escaped it (owner.ts brackets it
    // without try/finally, which would survive into prod; a cleanup's own
    // throw is caught there). Clear it then, or every later check would stay
    // silent.
    if (!disposalHealQueued) {
      disposalHealQueued = true;
      queueMicrotask(healDisposalDepth);
    }
    return;
  }
  // One capture per microtask window answers "no flight on the stack" for
  // every read until the queue turns: the common case — untracked reads
  // elsewhere while a flight is open — costs one capture. A positive answer is
  // never reused: one drain resumes every sibling continuation whose promise
  // settled (N memos awaiting one fetch), so the flight on the stack changes
  // between reads inside the same window. Each read inside a flight captures
  // afresh, which is what makes the attribution causal rather than "some
  // flight is open". The reset can land a few jobs late, so a window that
  // opened on an unowned read hides a continuation queued before the reset
  // (pinned as a known false negative); it can never invent a warning.
  let id = windowFlight;
  if (id === undefined) {
    id = windowFlight = attributedFlight();
    queueMicrotask(resetWindowFlight);
  } else if (id !== 0) id = attributedFlight();
  if (id === 0) return;
  const entry = tailFlights.get(id);
  const el = entry?.el.deref();
  // A never-resolved pending read throws, and async.ts reports it on rejection.
  // A refetching source serves its old value instead, so that read still warns.
  if (!el || throwsPending || el === dep) return;
  const flight = entry.flight.deref();
  if (!flight || el._x?._inFlight !== flight) return;
  if (dep !== undefined)
    for (let d = el._deps; d !== null; d = d._nextDep) if (d._dep === dep) return;
  let byHolder = warnedTailReads.get(el);
  if (!byHolder) warnedTailReads.set(el, (byHolder = new WeakMap()));
  let keys = byHolder.get(holder);
  if (!keys) byHolder.set(holder, (keys = new Set()));
  if (keys.has(key)) return;
  keys.add(key);
  reportDiagnostic(
    emitDiagnostic(
      {
        code: "UNTRACKED_READ_AFTER_AWAIT",
        kind: "async",
        severity: "warn",
        message:
          `[UNTRACKED_READ_AFTER_AWAIT] ${nodeName ? `"${nodeName}"` : "A reactive value"} was ` +
          `first read after an \`await\` in an async computation, so it is not a dependency: ` +
          `the computation will not re-run when it changes. Read it before the first \`await\`, ` +
          `or wrap the read in untrack() if a one-time value is intended.`,
        ownerId: el.id,
        ownerName: el._name,
        nodeName
      },
      el
    )
  );
}
const thrownReads = new WeakMap();
const caughtReadReported = new WeakSet();
/** The pass a read's throw lands in: `context`, through a root to the
 * computed that created it (as `read()` resolves `c`). `untrack` does not
 * null it: an untracked read inside a pass still throws into that pass. */
function runningPass() {
  const c = context;
  return c?._root ? c._parentComputed : c;
}
/** A reactive read threw `error` (NotReady or `source`'s error) into the
 * running pass. Called at `read()`'s throw sites, dev only. */
function noteThrownRead(source, error) {
  const c = runningPass();
  if (c !== null) thrownReads.set(c, { error, source, flight: undefined });
}
/** The runtime's own catch swallowed (or re-routed) the last throw into the
 * running pass: it is the mechanism, not a user catch. Dev only.
 * @internal Also solid-js's: its hydration trace runs a compute whose
 * throws the tracer, not user code, observes. */
function forgetThrownRead() {
  const c = runningPass();
  if (c !== null) thrownReads.delete(c);
}
/** The head of `el`'s pass: whatever an earlier pass left is not this one's. */
function clearThrownRead(el) {
  thrownReads.delete(el);
}
/** Flight shape, as `handleAsync` probes it (untracked: a store proxy's
 * `then` is a read). */
function isFlight(result) {
  if (typeof result !== "object" || result === null) return false;
  return untrack(() => !!result[Symbol.asyncIterator] || typeof result.then === "function");
}
/** `el`'s `_fn` returned `result`. A throw still recorded against the pass
 * was caught inside the body — unless `result` is the flight the body
 * became (an `async` function's rejected promise where a sync body would
 * have thrown): then the flight's settle judges it (`caughtReadFlightSettled`).
 * Called before `handleAsync` sees the result: a pending flight throws
 * NotReady out of it, and the verdict must already be pinned to the flight. */
function checkCaughtRead(el, result) {
  const thrown = thrownReads.get(el);
  if (thrown === undefined) return;
  if (thrown.flight === undefined && isFlight(result)) {
    thrown.flight = result;
    return;
  }
  if (thrown.flight !== undefined) return;
  thrownReads.delete(el);
  reportCaughtRead(el, thrown);
}
/** The flight `el`'s body became settled (async.ts; the caller has checked
 * it is still `el`'s registered flight). `landed`: a value — the body
 * swallowed the read's throw and answered. A rejection is the throw
 * continuing: forgotten. A record never pinned is this flight's too: the
 * pass registered it after the throw and then threw out (a projection's
 * self-registration suspending, an async iterable's live first pull) — a
 * later pass would have cleared the record at its head. */
function caughtReadFlightSettled(el, flight, landed) {
  const thrown = thrownReads.get(el);
  if (thrown === undefined || (thrown.flight !== undefined && thrown.flight !== flight)) return;
  thrownReads.delete(el);
  if (landed) reportCaughtRead(el, thrown);
}
function reportCaughtRead(el, thrown) {
  if (caughtReadReported.has(el)) return;
  caughtReadReported.add(el);
  const ownerName = el._name;
  const nodeName = thrown.source._name;
  const owner = ownerName ? `"${ownerName}"` : "A computation";
  const source = nodeName ? `"${nodeName}"` : "a reactive value";
  const message =
    thrown.error instanceof NotReadyError
      ? `[CAUGHT_REACTIVE_READ] ${owner} caught the not-ready throw of ${source} and completed with a value. ` +
        `A reactive read is not an await: the throw is how the read suspends to a <Loading> boundary or holds the change, ` +
        `and when the source changes later this computation is marked pending and not re-run, so the catch will not fire again. ` +
        `Branch on readiness with isPending(), show ahead with latest(), or let the read suspend.`
      : `[CAUGHT_REACTIVE_READ] ${owner} caught the error of ${source} and completed with a value. ` +
        `An error is the source's status, not a throw this computation owns: the computation is errored with it regardless, ` +
        `and today's runtime may complete with the caught value until the source is retried, ` +
        `when the error reaches <Errored> (or halts) and the catch does not fire again. ` +
        `Handle errors with <Errored> or createEffect's error option.`;
  reportDiagnostic(
    emitDiagnostic(
      {
        code: "CAUGHT_REACTIVE_READ",
        kind: "async",
        severity: "warn",
        message,
        ownerId: el.id,
        ownerName,
        nodeName
      },
      el
    )
  );
}
// Sweeping only when the registry doubles keeps registration O(1) amortized.
let nextTailSweep = 64;
function sweepTailFlights() {
  for (const [id, { el, flight }] of tailFlights) {
    const node = el.deref();
    const current = flight.deref();
    if (!node || !current || node._x?._inFlight !== current) {
      tailFlights.delete(id);
      asyncTailFlights--;
    }
  }
  nextTailSweep = Math.max(64, tailFlights.size * 2);
}
let windowFlight;
function resetWindowFlight() {
  windowFlight = undefined;
}
let disposalHealQueued = false;
function healDisposalDepth() {
  disposalHealQueued = false;
  if (disposalDepth !== 0) resetDisposalDepth();
}
/** The innermost tail flight on the async stack, or 0. Reads V8 call sites, skipping `.stack` formatting. */
function attributedFlight() {
  const V8Error = Error;
  if (!V8Error.captureStackTrace) return 0;
  const prevPrepare = V8Error.prepareStackTrace;
  const prevLimit = V8Error.stackTraceLimit;
  const target = {};
  let sites;
  V8Error.prepareStackTrace = (_, callSites) => callSites;
  // The tail frame sits BELOW every synchronous frame between the read and
  // the continuation (helpers, array callbacks, the store trap, read() and
  // this function itself), plus one async frame per awaited helper on the way
  // back to the compute. Node's default of 10 loses it behind a modest helper
  // chain; 50 covers a deep one without paying for the frame walk in the
  // common case, where the stack is far shorter and the limit is never
  // reached. A read more than ~40 synchronous frames deep is a false negative.
  V8Error.stackTraceLimit = 50;
  try {
    V8Error.captureStackTrace(target, attributedFlight);
    sites = target.stack;
  } finally {
    V8Error.prepareStackTrace = prevPrepare;
    V8Error.stackTraceLimit = prevLimit;
  }
  if (!Array.isArray(sites)) return 0;
  for (const site of sites) {
    const match = TAIL_FRAME.exec(site.getFunctionName?.() ?? "");
    if (match) return +match[1];
  }
  return 0;
}

/**
 * Quiescence checks. Run only when the system is fully drained: nothing
 * scheduled, no active/stashed transitions, no live lanes. At that point no
 * transition-scoped state may survive, and the lazily-created companions must
 * agree with a fresh computation of their owner's state.
 */
function devCheckQuiescent(isQueuedForCommit) {
  return;
}

// L2 (step 2): the parked frame. A pass parks the previous frame's children
// and `_disposal` list on `_x._pendingFirstChild` / `_x._pendingDisposal`
// (`parkChildren`, core.ts); they keep reacting until the node's commit
// disposes them (`zombie` walk below) or the owner dies. `markDisposal` flags
// the parked subtree REACTIVE_ZOMBIE: the splice guard and the reawaken path
// key off it, and the heap defers a zombie's recompute past the seam (no
// second heap — the deferral list in scheduler.ts).
function markDisposal(el) {
  let child = el._firstChild;
  while (child) {
    child._flags |= REACTIVE_ZOMBIE;
    markDisposal(child);
    child = child._nextSibling;
  }
}
function dispose(node) {
  // Direct disposal is death, not dormancy: strip the observation lifecycle
  // so a later read freezes at the last committed value instead of
  // reawakening the node (#3024). The teardown itself (heap removal — a node
  // left queued would be recomputed and resurrected by the next flush (#2983)
  // — dep unlinking, child disposal) is exactly unobserved()'s body; only
  // this flag distinguishes death from dormancy.
  node._config &= ~CONFIG_AUTO_DISPOSE;
  unobserved(node);
}
// The first error a cleanup threw, boxed so a thrown `undefined` still
// counts. A throwing cleanup stops neither its siblings nor the rest of the
// tree: every cleanup runs. The error is rethrown once the teardown is over:
// by a root's dispose call outside a flush (`disposeRootSelf`), otherwise when
// the flush ends (`flush`; `runCleanup` schedules one), so a flush is never
// left half-applied. A halt drops it: the error that halted is the one
// reported, and a stale one must not surface from a later, unrelated dispose.
let cleanupError;
/** Runs a cleanup or `unobserved` hook; a throw is collected, not raised. @internal */
function runCleanup(fn) {
  try {
    fn();
  } catch (e) {
    // (D-42, dev: collected and rethrown when the teardown is over — not a
    // catch in the pass that happens to be disposing.)
    forgetThrownRead();
    cleanupError ||= [e];
    schedule();
  }
}
/** Rethrows the error `runCleanup` collected, once; `drop` discards it. @internal */
function rethrowCleanupError(drop) {
  const error = cleanupError;
  cleanupError = undefined;
  if (error && !drop) throw error[0];
}
function disposeChildren(node, self = false, zombie) {
  const flags = node._flags;
  if (flags & REACTIVE_DISPOSED) return;
  // A parked frame dies with its owner (#3024): the commit that would retire
  // it never comes once the node is DISPOSED, so it drains here or its
  // cleanups never run and it stays subscribed, rerunning in a torn-down
  // tree (#3561). Death only (`self`): a re-pass leaves the frame reacting
  // until the commit.
  if (self) {
    if (
      !zombie &&
      node._x !== null &&
      (node._x._pendingFirstChild !== null || node._x._pendingDisposal !== null)
    )
      disposeChildren(node, false, true);
    node._flags = flags | REACTIVE_DISPOSED;
    if (node._parent === null && node._root) unregisterRoot(node);
    clearSignals(node);
    if (node._fn) {
      clearInFlight(node);
      // A death that can unblock the future (L2, `futureBlocked`): a held
      // flight's — a transaction's or a lane's (its `_x._transaction`) — or a
      // stale reader's (REACTIVE_FRAME_READ — a frame that derived from one).
      // An idle pending queue would also pin it until an unrelated flush.
      // Nothing else re-examines a dead node, so make sure a seam comes.
      if (
        (node._statusFlags & STATUS_PENDING && node._x?._transaction != null) ||
        flags & REACTIVE_FRAME_READ ||
        pendingNodes.length
      )
        schedule();
    }
  }
  // Typed as a computed so the loop needs no cast alias (the minifier keeps one).
  let child = zombie ? node._x._pendingFirstChild : node._firstChild;
  if (zombie) node._x._pendingFirstChild = null;
  else node._firstChild = null;
  while (child) {
    // Owner teardown is death regardless of the child's own lifecycle
    // (#3024): strip AUTO_DISPOSE so a post-disposal read freezes at the
    // last committed value instead of reawakening in a torn-down tree.
    // Runs before the recursion so already-dormant children (whose
    // disposeChildren call early-returns on REACTIVE_DISPOSED) die too.
    // Only unobserved()'s own node keeps its dormancy — it is never in
    // this loop; its children are rebuilt fresh on reawaken.
    child._config &= ~CONFIG_AUTO_DISPOSE;
    // Heap removal must not be gated on `_deps`: a dependency-free
    // computation queued by refresh() has a null dep list but still sits in
    // the dirty heap, and left there the post-disposal flush recomputes it —
    // recompute() rewriting `_flags` clears REACTIVE_DISPOSED and the node
    // comes back to life (post-unmount runs, leaked cleanups, #2983).
    // deleteFromHeap self-guards on the in-heap flags (and tolerates plain
    // Owners, whose _flags is undefined), so no gate here.
    deleteFromHeap(child, dirtyQueue);
    clearDeps(child);
    // The chain is detached above so a node a cleanup links mid-drain lands
    // on the fresh head and survives. Pointing each drained child's prev at
    // itself routes its later splice onto the detached chain, never the head,
    // and keeps the dev owner-chain-head invariant honest for those children.
    child._prevSibling = child;
    disposeChildren(child, true);
    // Read after, not before: a sibling this disposal made dormant spliced
    // itself out of the detached chain, and a cleanup may then have linked
    // it at the fresh head, which rewrote the `_nextSibling` a pre-read
    // would still be holding.
    child = child._nextSibling;
  }
  if (!zombie) node._childCount = 0;
  // O(1) splice out of parent's chain on individual dispose. Skipped during
  // batch dispose (parent already disposed) and for a zombie (it sits on the
  // parent's parked frame, drained whole by the commit). We leave
  // node._nextSibling intact so outer walks that already advanced past us
  // still reach later siblings.
  if (
    self &&
    !zombie &&
    !(flags & REACTIVE_ZOMBIE) &&
    node._parent !== null &&
    !(node._parent._flags & REACTIVE_DISPOSED)
  ) {
    const prev = node._prevSibling;
    const next = node._nextSibling;
    // A node with no predecessor must be the chain's head; otherwise the
    // write below would clobber the head with a stale `_nextSibling`,
    // orphaning every live child ahead of it (#3543).
    assertInvariant(
      prev !== null || node._parent._firstChild === node,
      "owner-chain-head",
      "head node is not parent._firstChild — a node was spliced while flagged live but not in the chain (see #3543)"
    );
    if (prev !== null) prev._nextSibling = next;
    else node._parent._firstChild = next;
    if (next !== null) next._prevSibling = prev;
    node._prevSibling = null;
  }
  runDisposal(node, zombie);
  // Final effect-returned cleanup fires at true disposal, after `_disposal`
  // to mirror rerun ordering (compute-phase teardown first, cleanup last).
  if (self && node._cleanup) {
    const effectCleanup = node._cleanup;
    node._cleanup = undefined;
    enterDisposal();
    runCleanup(effectCleanup);
    exitDisposal();
  }
}
function linkChild(parent, node) {
  const head = parent._firstChild;
  node._prevSibling = null;
  node._nextSibling = head;
  if (head !== null) head._prevSibling = node;
  parent._firstChild = node;
}
function runDisposal(node, zombie) {
  // Detach the list BEFORE running it (#3601), as `_cleanup` is (#2813). A
  // cleanup that disposes an ancestor re-enters this node through the death
  // walk while the loop is still running; with the list still attached, that
  // walk ran every entry a second time. Detached, the re-entrant drain finds
  // nothing. The same shape latched a throwing cleanup: the list survived the
  // throw and every later drain re-ran and re-threw it.
  const disposal = zombie ? node._x._pendingDisposal : node._disposal;
  if (!disposal) return;
  if (zombie) node._x._pendingDisposal = null;
  else node._disposal = null;
  // Each cleanup is guarded (`runCleanup`), so the depth is always lowered.
  enterDisposal();
  if (Array.isArray(disposal)) {
    // Unwind order (#3572, restores 1.x #1562): later registrations run
    // before earlier ones. Children have already been disposed by the caller,
    // so with LIFO a body that registers cleanup before creating its children
    // tears down after them — the same order a per-component owner gives.
    for (let i = disposal.length - 1; i >= 0; i--) {
      runCleanup(disposal[i]);
    }
  } else {
    runCleanup(disposal);
  }
  exitDisposal();
}
function childId(counter, consume) {
  while (counter._config & CONFIG_TRANSPARENT && counter._parent) counter = counter._parent;
  if (counter.id != null)
    return formatId(counter.id, consume ? counter._childCount++ : counter._childCount);
  throw new Error("Cannot get child id from owner without an id");
}
/**
 * Allocates and returns the next stable child id for `owner`. Used by
 * hydration plumbing and `createUniqueId`. Not part of the user-facing API.
 *
 * @internal
 */
function getNextChildId(owner) {
  return childId(owner, true);
}
/**
 * The id a freshly-created node inherits: an explicit `options.id` wins;
 * transparent nodes share their parent's id; otherwise the parent's next
 * child id is consumed (or `undefined` outside an id-carrying tree).
 */
function inheritId(options, transparent, parent) {
  return (
    options?.id ??
    (transparent ? parent?.id : parent?.id != null ? getNextChildId(parent) : undefined)
  );
}
/**
 * Returns the *next* child id for `owner` without consuming it. Used by
 * hydration plumbing to peek at the id a future child will receive.
 *
 * @internal
 */
function peekNextChildId(owner) {
  return childId(owner, false);
}
function formatId(prefix, id) {
  const num = id.toString(36),
    len = num.length - 1;
  return prefix + (len ? String.fromCharCode(64 + len) : "") + num;
}
/**
 * Returns the currently-tracking observer (the computation that subscribes to
 * reactive reads at this point), or `null` if reads here would be untracked.
 * Used by reactive primitives that need to know whether they're inside a
 * tracking scope. App code rarely needs this — see `getOwner()` for the
 * lifecycle owner instead.
 *
 * @example
 * ```ts
 * // Library predicate: only register a hot-path subscription when the
 * // caller is inside a tracking scope (memo / effect compute / JSX).
 * function trackIfTracked(source: () => unknown) {
 *   if (getObserver()) source();
 * }
 * ```
 */
function getObserver() {
  return tracking ? context : null;
}
/**
 * Returns the current reactive **owner** — the lifecycle node that the next
 * `cleanup()` / `onCleanup()` / `createSignal()` etc. will be attached to.
 *
 * Returns `null` if called outside any owner. Capture the owner with
 * `getOwner()` and re-enter it later with `runWithOwner(owner, fn)` to attach
 * disposables created from a callback (event handler, async resolution, etc.)
 * back to a component's lifecycle.
 *
 * @example
 * ```ts
 * function defer<T>(fn: () => T) {
 *   const owner = getOwner();
 *   queueMicrotask(() => runWithOwner(owner, fn));
 * }
 * ```
 */
function getOwner() {
  return context;
}
/**
 * Low-level: registers `fn` as a disposal callback on the current owner.
 * Most code should use `onCleanup()` from `solid-js`, which adds dev-mode
 * checks. `cleanup()` is the unchecked primitive used by internals; `owner`
 * (internal) targets a specific owner's disposal list instead of the ambient
 * one.
 */
function cleanup(fn, owner = context) {
  if (!owner) return fn;
  if (!owner._disposal) owner._disposal = fn;
  else if (Array.isArray(owner._disposal)) owner._disposal.push(fn);
  else owner._disposal = [owner._disposal, fn];
  return fn;
}
/**
 * Returns `true` if the owner has been disposed. Pair with a captured owner
 * to bail out of late callbacks whose surrounding component already
 * unmounted.
 *
 * @example
 * ```ts
 * function onSettleSafe(fn: () => void) {
 *   const owner = getOwner();
 *   queueMicrotask(() => {
 *     if (owner && isDisposed(owner)) return; // component unmounted; skip
 *     runWithOwner(owner, fn);
 *   });
 * }
 * ```
 */
function isDisposed(node) {
  return !!(node._flags & REACTIVE_DISPOSED);
}
function disposeRootSelf(self = true) {
  disposeChildren(this, self);
  // Outside a flush the dispose call rethrows; inside one, the flush does.
  // Kept out of `disposeChildren`, whose hot path it slows.
  if (cleanupError && !globalQueue._running) rethrowCleanupError();
}
/**
 * Creates a fresh owner attached as a child of the current owner (or as a
 * detached root if there is none). Used by framework internals to group
 * cleanups; app code should use `createRoot()` (host a reactive scope outside
 * a component) or `runWithOwner()` (re-enter a captured owner).
 *
 * @internal
 */
function createOwner(options) {
  const parent = context;
  const transparent = options?.transparent ?? false;
  // Prod and observe boilerplates (see core.ts computed()). The observe
  // literal carries the `_name` and `_loc` slots the rendering layer fills
  // with the component label and JSX site (`owner._name = "<App>"`,
  // `owner._loc = "src/App.tsx:3:5"`) — slots, so labelling a root is a
  // plain store rather than a shape fork between labelled and plain roots.
  const owner = {
    id: inheritId(options, transparent, parent),
    _config: transparent ? CONFIG_TRANSPARENT : 0,
    // Present from birth: disposeChildren marks death here on roots too, and
    // a first store of a missing property is a shape transition plus a
    // properties-store allocation per root — on a 100×4 table teardown that
    // was 12 KB and most of disposal's time.
    _flags: REACTIVE_NONE,
    _root: true,
    _parentComputed: parent?._root ? parent._parentComputed : parent,
    _firstChild: null,
    _nextSibling: null,
    _prevSibling: null,
    _disposal: null,
    _context: parent?._context || defaultContext,
    _childCount: 0,
    _x: null,
    _parent: parent,
    dispose: disposeRootSelf,
    _name: undefined,
    _loc: undefined
  };
  if (parent && parent._config & CONFIG_CHILDREN_FORBIDDEN) {
    emitDiagnostic({
      code: "PRIMITIVE_IN_FORBIDDEN_SCOPE",
      kind: "lifecycle",
      severity: "error",
      message: PRIMITIVE_IN_FORBIDDEN_SCOPE_MESSAGE,
      ownerId: parent.id,
      ownerName: parent._name
    });
    throw new Error(PRIMITIVE_IN_FORBIDDEN_SCOPE_MESSAGE);
  }
  assertNotInEffectCallback();
  if (parent) linkChild(parent, owner);
  else registerRoot(owner);
  DEV.hooks.onOwner?.(owner);
  return owner;
}
/**
 * Creates a reactive root — an owner scope with its own `dispose()`. A root
 * created inside an existing owner is owned by it and is disposed when the
 * parent is disposed; call `dispose()` to tear it down earlier. To create a
 * root that outlives its creator, detach explicitly:
 * `runWithOwner(null, () => createRoot(...))`. Pass `id` to seed hydration
 * ids for the tree it owns.
 *
 * `dispose()` tears down every signal, memo, effect, and `onCleanup`
 * registered inside the root.
 *
 * Use this to host long-lived reactive scopes outside of a component (custom
 * controllers, app bootstrapping, tests). Inside a component, prefer
 * letting Solid's component lifecycle own things.
 *
 * @example
 * ```ts
 * // At module level there is no owner, so this root lives until disposed.
 * const dispose = createRoot(dispose => {
 *   const [n, setN] = createSignal(0);
 *   createEffect(() => n(), value => console.log(value));
 *   setInterval(() => setN(x => x + 1), 1000);
 *   return dispose;
 * });
 *
 * // Later, to tear everything down:
 * dispose();
 *
 * // Inside an owner (component, effect, another root), detach explicitly
 * // if the root must outlive its creator:
 * const detached = runWithOwner(null, () => createRoot(d => d));
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/advanced/owner-introspection/create-root
 */
function createRoot(init, options) {
  const owner = createOwner(options);
  markCopyRooted();
  // A bound function, not an arrow: an arrow would share this scope, which
  // holds `init`, so keeping `dispose` would keep `init` and all it closes
  // over alive (#4095).
  return runWithOwner(owner, () => init(disposeRootSelf.bind(owner, true)));
}

// Optional-module hooks: module bindings, not `GlobalQueue` statics, so a
// bundle that never imports the installing module never assigns them — its
// minifier folds every guarded call site (and the branch behind it) away.
// Each module installs its own set at evaluation (pay-for-use).
//
// Lanes (lanes.ts; installed when `createOptimistic` — or verdict.ts — is
// imported; every call site is behind a bit or a `passLane` nothing else sets).
let laneReadHook = null;
let laneStageHook = null;
let laneOutcomeHook = null;
let laneWriteHook = null;
let applyGuessesHook = null;
let laneSeamsHook = null;
let laneCorrectionsHook = null;
let endLanesHook = null;
let lanesBlockedHook = null;
/** R5 (verdict.ts: the seam parks a frame flight of a reader that reads
 * ahead): the landing's re-derive of the reader adopts the flight instead
 * (`adoptParked`; true: adopted, nothing to queue). */
let laneAdoptHook = null;
/** R5 at the seam (verdict.ts `r5`): a reader whose probe is unanswered, a
 * hold's parked flight still up (`r` null), the seam's end (both null). */
let r5Hook = null;
/** core.ts `frameRead` served lane work or a verdict reader a held node's
 * committed value (lanes.ts `laneServed`). */
let laneServedHook = null;
/** A guess was written (guesses.ts `optimisticWrite`): the arms only a guess
 * reaches (CONFIG_GUESS) — a build without `createOptimistic` or
 * `createOptimisticStore` never sets it, and its bundler folds them. */
let guessing = false;
function armGuessing() {
  guessing = true;
}
function installLaneHooks(read, stage, write, seams, end, blocked, served) {
  laneReadHook = read;
  laneStageHook = stage;
  laneWriteHook = write;
  laneSeamsHook = seams;
  endLanesHook = end;
  // The transaction being judged (`judge`): a lane consulted on its behalf
  // judges for it. Only this hook nests one `blocked` in another, so the
  // bookkeeping lives here and a build without lanes folds `judge` away.
  lanesBlockedHook = t => {
    const prev = judge;
    judge ??= t;
    const r = blocked(t);
    judge = prev;
    return r;
  };
  laneServedHook = served;
}
// The guess layer (guesses.ts; installed when `createOptimistic` or
// `createOptimisticStore` is imported): a written guess's own pass judged
// (`laneOutcome`, core.ts `recompute`), the seam's guesses opening their
// lanes (`applyGuessesAt`), the body-end corrections (`laneCorrections`).
// Null without it: every site tolerates it.
function installGuessHooks(outcome, applyGuesses, corrections) {
  laneOutcomeHook = outcome;
  applyGuessesHook = applyGuesses;
  laneCorrectionsHook = corrections;
}
// Boundaries (boundaries.ts): the display consumers between an observer
// and the root. `catch` — status from a frame reader, nearest boundary
// first (true: caught, the root never hears of it; a clear — flags 0 —
// settles the reader there); `fresh` — a pass that read a hold (true: a
// first pass a loading boundary that has not shown content caught,
// `joinPass`; `held`: the flush's hold owns it, nothing catches it);
// `hidden` — a frame reader behind a fallback (the boundary showing it)
// is not on screen and holds nothing; `boundarySeam` — the seam's sweep (readers
// gone or settled without a pass reveal; an `on` re-arm resolves);
// `heldRun` — a queued run under a fallback-showing boundary waits for
// the reveal (true: held; the boundary re-queues it by type). The
// synchronous first render on creation builds the subtree, attached or
// not; its updates and the user effects wait.
let catchHook = null;
let freshHook = null;
let hiddenHook = null;
let boundarySeamHook = null;
let heldRunHook = null;
function installBoundaryHooks(caught, fresh, hidden, seam, heldRun) {
  catchHook = caught;
  freshHook = fresh;
  hiddenHook = hidden;
  boundarySeamHook = seam;
  heldRunHook = heldRun;
}
/** `@solidjs/web`'s head registry: told when a boundary swaps between its
 * fallback and its content, and asks whether a registration's node is
 * behind a fallback — content there is not on screen (F-3), nor are its
 * head tags (#4873). Never set without the registry. */
let swapHook = null;
function setSwapHook(fn) {
  swapHook = fn;
}
function isHidden(node) {
  return !!hiddenHook?.(node);
}
/** Store (store/store.ts): `park` — the flush parked into `t`: the store
 * materializes and holds the container node of every staging that had none
 * (a staging needs the node as its home only when something can read the
 * frame through it; a hold can); `commit` — fold the pending backings whose
 * container nodes this flush committed (the owned-raw model's one mutation
 * point). */
let storeParkHook = null;
let storeCommitHook = null;
function installStoreHooks(park, commit) {
  storeParkHook = park;
  storeCommitHook = commit;
}
/** `affects()` marks (affects.ts): the releases at a landing and at the seam
 * (ambient marks). */
let releaseMarksHook = null;
let releaseAmbientMarksHook = null;
function installMarkHooks(release, releaseAmbient) {
  releaseMarksHook = release;
  releaseAmbientMarksHook = releaseAmbient;
}
/** Verdicts (verdict.ts). */
let observeFlightHook = null;
/** D-25's hold work (verdict.ts `holdWork`): on `source` going pending,
 * whether verdict reader `n` waits on it instead of re-deriving. */
let holdWorkHook = null;
/** D-41's staged-error mark (verdict.ts `stageMark`): the settle walk's
 * reached node `n` carries staged error source `el`'s staging. */
let stageMarkHook = null;
/** #4417 (verdict.ts `keepReaders`); a landing's held re-derivation run (verdict.ts `attachWaits`). */
let keepReadersHook = null;
let attachHook = null;
/** #4544 (verdict.ts). Null without the verdict engine. */
let holdStagingHook = null;
/** D-8 (verdict engine), at the flush: the readers a tick netting to its
 * starting value woke are let go (verdict.ts `revert`). */
let revertHook = null;
/** Verdict engine, a pass's head (D-22, D-41: the held error `latest` keeps
 * showing) and, with `tail`, its end
 * (A19 exc. 2: whether it asked the committed question again). */
let passHook = null;
function installVerdictHooks(
  observeFlight,
  holdWork,
  stageMark,
  keepReaders,
  attach,
  holdStaging,
  adopt,
  r5,
  revert,
  pass
) {
  revertHook = revert;
  passHook = pass;
  observeFlightHook = observeFlight;
  holdWorkHook = holdWork;
  stageMarkHook = stageMark;
  keepReadersHook = keepReaders;
  attachHook = attach;
  holdStagingHook = holdStaging;
  laneAdoptHook = adopt;
  r5Hook = r5;
}
/** External-source bridge (wired by enableExternalSource(); undefined while
 * no config is active — including after _resetExternalSourceConfig()). */
let wireExternalSourceHook = undefined;
let externalUntrackHook = undefined;
function setExternalHooks(wire, untrack) {
  wireExternalSourceHook = wire;
  externalUntrackHook = untrack;
}
/** Live transactions — opened, unmerged, not landed. Scanned at every seam
 * while non-empty; nothing on the plain path. */
const transactions = [];
/** Observe tier: this flush parked its transaction (`holdStart` fired). */
let holding = false;
/** A transaction (listed here) or a lane (lanes.ts lists its own). */
function newTransaction(lane, parent = null) {
  const t = {
    _nodes: [],
    _queues: [[], []],
    _reruns: [],
    _into: null,
    _parent: parent,
    _lane: lane,
    _verdict: null,
    _open: 0,
    _acted: false,
    _links: null,
    _shown: false,
    _held: false,
    _marks: null
  };
  if (!lane) transactions.push(t);
  return t;
}
// Lanes (plan sec. 28, 2026-10-02; lanes.ts). A lane is "a new base of a transition":
// the sub-frame an optimistic write opens in the transition of the frame it
// is made in. It sees the screen plus its own guesses (plan sec. 19), breaks out of
// the parent's hold — its effects run now — and holds itself if its own
// derivations hit async. It ends when the parent lands: a guess reverts to
// the truth it covered, or lands as the truth that superseded it. A frame
// that does not park has nothing to be optimistic over: the write is void.
//
// The seat of a pass is its node's: a node carrying a lane's value runs as
// the lane's whoever dirtied it; a derivation's tracked read of a lane's
// value moves its pass into the lane; a leaf's never does (it reads the
// screen). Lane work's values go to the lane slot, its runs to the lane's
// queues; the rest of the frame is the flush's.
/** The lane the running pass is work of; set at `recompute`'s head from the
 * node, moved by a derivation's lane read, saved and restored around the
 * pass. */
let passLane = null;
function setPassLane(l) {
  passLane = l;
}
/** The transaction this flush parks into: set by the first join, merged
 * with every further one. Consumed at the seam. A join outside a flush (a
 * mount's pending observer, a write to a held node, an action opening)
 * schedules the seam that consumes it: membership is the tick's — the same
 * synchronous frame — and the next tick's writes are not this one's. */
let flushTransaction = null;
/** The transaction the passes running OUTSIDE a flush this tick derive from
 * (a mainline mount, a mainline re-pass): the passes' own (A29, creation-
 * time form — "the entry is the pass's alone"): their results are staged
 * into it, and nothing else in the tick is — a write made after the mount
 * is a mainline write, a render effect mounted beside it a stale reader.
 * Inside a flush, or in a tick that already has its transaction (an
 * action's body), the frame joins instead (`flushTransaction`) — except a
 * first pass a loading boundary that has not shown content catches (`own`,
 * A29's boundary exemption, #3540): it is the boundary's, not the tick's,
 * and a flush that has joined nothing keeps it pass-scoped as outside one.
 * Cleared by the flush the join schedules, and at the end of a flush that
 * set it. */
let passTx = null;
function joinPassTx(t, own) {
  if ((globalQueue._running && !own) || flushTransaction !== null) return joinFuture(t);
  if (passTx === null) passTx = resolveTx(t);
  else merge(resolveTx(t), passTx);
  schedule();
}
/** The transaction holding a node (CONFIG_HELD set), resolved through
 * merges; the path is compressed. */
function txOf(n) {
  return (n._x._transaction = resolveTx(n._x._transaction));
}
/** `t` resolved through merges while it is still parked or open; null once
 * it has landed. */
function liveTx(t) {
  t = resolveTx(t);
  return transactions.indexOf(t) !== -1 ? t : null;
}
function resolveTx(t) {
  while (t._into !== null) t = t._into;
  return t;
}
/** The flush joins transaction `t` — or needs one of its own (`null`: a pass
 * went pending with nothing held). Joining a second one merges it into the
 * flush's: the work that touched both entangles them (A15). */
/** #4777: the flush gives back the hold a pass's plain read adopted
 * (`joinPass`) before that pass starts over (verdict.ts `markVerdictReader`),
 * and every hold the pass's later plain reads merged into it (`merge`, in
 * reverse): each is its own again, as before the pass. */
function dropFlushTx() {
  for (let i = adopted.length - 2; i >= 0; i -= 2) {
    const t = adopted[i],
      f = t._into;
    t._into = null;
    f._open -= t._open;
    f._acted = adopted[i + 1];
    unappend(f._nodes, t._nodes);
    unappend(f._queues[0], t._queues[0]);
    unappend(f._queues[1], t._queues[1]);
    unappend(f._reruns, t._reruns);
    unappend(f._marks, t._marks);
    transactions.push(t);
  }
  adopted.length = 0;
  flushTransaction = null;
}
function unappend(into, own) {
  if (own) for (let j = own.length; j--;) into.splice(into.lastIndexOf(own[j]) >>> 0, 1);
}
function joinFuture(t) {
  // A lane is never the flush's transaction (D-37, #4440).
  if (laneStageHook && t?._lane) return;
  const f = flushTransaction;
  if (f === null) {
    flushTransaction = t === null ? newTransaction(false) : resolveTx(t);
  } else if (t !== null) merge(resolveTx(t), f);
  if (!globalQueue._running) schedule();
}
/** `t` merges into `f`: the work that touched both entangles them (A15).
 * A lane never merges: it is a sub-frame, not a peer — its work is routed
 * per pass (`passLane`). */
/** #4777: the merges made since a pass adopted the flush's hold (core.ts
 * `adopter`), as [t, f._acted before]: a restart gives them back too. */
const adopted = [];
function merge(t, f) {
  // Both ends resolved: a stale pointer to a transaction already merged
  // into the other would close a cycle `resolveTx` never leaves.
  t = resolveTx(t);
  f = resolveTx(f);
  if (t === f || t._lane || f._lane) return;
  if (attrHooks !== null) attrHooks.transitionMerged(f, t);
  if (observeFlightHook && adopter) adopted.push(t, f._acted);
  t._into = f;
  f._open += t._open;
  f._acted ||= t._acted;
  append(f._nodes, t._nodes);
  append(f._queues[0], t._queues[0]);
  append(f._queues[1], t._queues[1]);
  append(f._reruns, t._reruns);
  if (t._marks !== null) append((f._marks ??= []), t._marks);
  transactions.splice(transactions.indexOf(t), 1);
}
/** A15: a hold is a property of the async node — observed pending by a
 * render reader. A transaction is blocked while a node it holds, in flight,
 * has one: a pending render effect (the frame itself), or a pending held
 * node a render effect reads. A stale reader served the committed value
 * never went pending, but derives from the flight all the same (#3494) and
 * is still linked to it; a memo between them is pending, and held, too — so
 * one hop of `_subs` is the chain. Reads of the reader's LAST pass only (the
 * link's generation is the pass's): a tail kept for A30 — the committed
 * frame's dependency, awaiting the run that retires it — is not a read of
 * the flight, and a reader that stopped reading releases it (O3, #3494).
 * An errored pass (NotReady included) did not stop: it never got there, and
 * its full list stands (A30) — the reads it did not reach still observe. A
 * flight nobody renders holds nothing (A29). Only the nodes `t` still owns
 * count: one a lane took over since is the lane's to wait on. A guess whose
 * own truth is in flight blocks the lane's parent, not the lane — it stands
 * in for the flight (A17: "visible until its own fetch settles"). */
function blocked(t) {
  // An action still running in it holds it open (action.ts). Its lanes
  // (lanes.ts): a lane under `t` blocks it while blocked itself.
  return t._open !== 0 || blockedBy(t._nodes, t) || !!lanesBlockedHook?.(t);
}
/** The transaction being judged — the outermost `blocked` of a nest (a lane
 * consulted on its behalf judges for it): a zombie whose removal it stages
 * is moot for it alone (`onScreen`). Set around the lanes' hook
 * (`installLaneHooks`), the only path that nests; null otherwise, where the
 * transaction judged is `blockedBy`'s own `owner`. */
let judge = null;
/** Observe tier: the flights `t` waits on — its pending held sources a
 * frame reader observes (`blockedBy`'s predicate, collected; the readers
 * themselves are not sources). */
function blockersOf(t) {
  const out = [];
  for (let i = 0; i < t._nodes.length; i++) {
    const n = t._nodes[i];
    if (
      !n._type &&
      n._statusFlags & STATUS_PENDING &&
      !(n._flags & REACTIVE_DISPOSED) &&
      (n._x._transaction === null || txOf(n) === t) &&
      blockedBy([n], t)
    )
      out.push(n);
  }
  return out;
}
/** `t`'s own flights only — not its lanes': a flight the frame asked for
 * (a refetch, a plain load) is authoritative; a lane's derivation flight is
 * not (lanes.ts, the body-end corollary). A pending reader counts only if
 * one of the sources it waits on is `t`'s (a render effect marked pending
 * by a lane flight, never re-run, is the lane's wait, not the frame's). */
function ownFlights(t) {
  return blockedBy(t._nodes, t, ownSource);
}
/** A source the pending node waits on that is `t`'s (held by it, or by no
 * one) — not another transaction's or lane's work. Itself, when it is the
 * source. */
function ownSource(n, t) {
  const sources = n._x?._pendingSources;
  if (sources === undefined) return true;
  for (const s of sources) {
    const u = s._x?._transaction;
    if (u == null || resolveTx(u) === t) return true;
  }
  return false;
}
/** `own`: the pending node's filter (`ownFlights` passes `ownSource`) — a
 * parameter, not a flag, so a build without lanes or guesses (the only
 * callers of `ownFlights`) does not retain `ownSource`. */
function blockedBy(nodes, owner, own) {
  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    if (
      !(n._statusFlags & STATUS_PENDING) ||
      n._flags & REACTIVE_DISPOSED ||
      n._config & CONFIG_GUESS ||
      (n._x._transaction !== null && txOf(n) !== owner) ||
      (own && !own(n, owner)) ||
      // A guess lane's member waiting on re-asks only (`reasks`) keeps what
      // it shows, and a `refresh()` holds nothing new (D-34 ¶3): it does not
      // keep the lane's other work off the screen (#4834: a memo over the
      // shown guess and a fresh answer stayed on its stale committed value).
      (guessing && owner._lane && n._x._pendingSources && reasks(n))
    )
      continue;
    // The frame's own pending observer (off screen: this one holds nothing;
    // the next listed node may).
    if (n._type === EFFECT_RENDER) {
      // #4544 (F-6): a frame born into the hold that is pending on what a
      // settled verdict reader's display-ahead value asked for (`read`,
      // CONFIG_AHEAD) does not hold it; the reader re-derives at the
      // landing, and the frame with it. A lane's reveal still waits (S-29).
      if (
        !(observeFlightHook && n._config & CONFIG_AHEAD && !owner._lane) &&
        onScreen(n, judge ?? owner)
      )
        return true;
      continue;
    }
    for (let s = n._subs; s !== null; s = s._nextSub) {
      const r = s._sub;
      // A frame reader: a render effect, or a stale reader of the flight
      // (REACTIVE_FRAME_READ — a verdict reader, `observeFlight`; lane work,
      // `frameRead`): served committed instead of going pending, it derives
      // from the flight all the same and is re-derived at the landing — the
      // frame's observation survives its reader becoming a lane's (a guess
      // over a held window, V5/A17). A probe alone does not hold. A
      // boundary showing content forwards the pending (its output pending,
      // CONFIG_REDERIVE): transparent, its readers hold through it (A33; the
      // lane-membership ruling, 2026-10-05). An output that never committed
      // shows nothing — a mount the frame has not revealed — and holds
      // nothing.
      if (
        ((r._type === EFFECT_RENDER &&
          r._statusFlags & STATUS_PENDING &&
          !(observeFlightHook && r._config & CONFIG_AHEAD && !owner._lane)) ||
          r._flags & REACTIVE_FRAME_READ ||
          (r._config & CONFIG_REDERIVE && r._statusFlags === STATUS_PENDING)) &&
        (s._gen === r._depGen || r._x?._error != null) &&
        !(r._flags & REACTIVE_DISPOSED) &&
        onScreen(r, judge ?? owner)
      )
        return true;
    }
  }
  return false;
}
/** A frame reader whose say counts for `t`'s hold — `t` the transaction
 * being judged: not a zombie whose removal `t` stages (A15 #3463: a re-ask
 * in the unmount frame lands its pending on the zombie it unmounts — the
 * compiled <Show> of #3372; a zombie blocks every other judgment while it
 * is visible, a lane's reveal included: only the commit that disposes it
 * makes its say moot), and not behind a fallback (A33: a boundary showing its fallback is the display of
 * everything under it, so a reader there is not on screen and holds
 * nothing — boundaries.ts). */
function onScreen(r, t) {
  return !hiddenHook?.(r) && !(r._flags & REACTIVE_ZOMBIE && removalStagedBy(r, t));
}
/** A15 (#3463): a reader whose removal is staged — a zombie, parked by a
 * pass awaiting its commit — is still on screen and live for every hold,
 * except the transaction staging its removal: for that one its say is moot
 * (done, and the commit disposes it; not done, and it stays parked). The
 * staging transaction is the one holding the pass that parked the frame:
 * the nearest non-zombie ancestor. */
function removalStagedBy(o, t) {
  while (o?._flags & REACTIVE_ZOMBIE) o = o._parent;
  return o?._config & CONFIG_HELD && o._x?._transaction != null && txOf(o) === resolveTx(t);
}
/** Unchanged passes this flush whose stale dependency tail awaits the
 * flush's verdict (A30, #3469): trimmed when the flush commits, kept when
 * it parks — the committed frame still derives from the previous pass's
 * dependencies, and a write to one of them must reach the node until a
 * committing pass replaces it. */
const heldTrims = [];
function heldTrim(el) {
  heldTrims.push(el);
}
/** Zombies popped from the dirty heap this flush (`deferZombie`). The seam
 * decides their fate: a park cancels them (the writes that dirtied them are
 * held with it); a commit runs the survivors — the ones whose owner is still
 * awaiting its commit — in height order, after the commits that would have
 * disposed them (#3546). No second heap: the list is the deferral. */
const deferredZombies = [];
function deferZombie(el) {
  deleteFromHeap(el, dirtyQueue);
  el._flags &= -4;
  deferredZombies.push(el);
}
/** Put a staged node in a transaction: flagged, stamped, on its landing
 * list. */
function holdNode(n, t) {
  n._config |= CONFIG_HELD;
  list(n, t);
}
/** Put a node on a transaction's list, once: a node's `_transaction` is the
 * transaction whose `_nodes` it is on (every site that sets one does the
 * other), so a re-pass, a re-landing or a re-staging adds nothing. */
function list(n, t) {
  const x = ext(n);
  if (x._transaction === t) return;
  x._transaction = t;
  t._nodes.push(n);
  if (attrHooks !== null) attrHooks.staged(n);
}
/** Hold the (uncommitted) frame under a held pass: every computed in the
 * live child chain, recursively. Plain owners are walked through, not held
 * — nothing reads or re-passes them. Once per park, over the parked frames
 * only; recomputes pay nothing for it. A lane's node in the frame (listed on
 * a lane: a verdict reader broken out of this very hold — display-ahead — or
 * a derivation of a guess, CONFIG_OVERRIDE; a lane pass's effect) is the
 * lane's, as the seam's own loop has it: "lane work never makes its node
 * transaction work" (A15, #3698). Re-listed here it would carry the lane's
 * value on the transaction's list, where the landing nulls its transaction
 * and `dissolveLane` no longer finds it — a lane node with no lane, which the
 * next read of it trips over (`laneRead` → `txOf`). Its frame is the lane's
 * too (ruling A: a lane pass's children are the lane's), revealed and
 * retired at the lane's seam. */
function holdFrame(owner, t) {
  for (let c = owner._firstChild; c !== null; c = c._nextSibling) {
    if (c._x?._transaction?._lane) continue;
    if (c._fn !== undefined) {
      if (!(c._config & CONFIG_HELD)) holdNode(c, t);
      c._config |= CONFIG_HELD_FRAME;
    }
    holdFrame(c, t);
  }
}
/** The nodes under `owner` a transaction other than `t` holds (boundaries.ts:
 * what an earlier hold staged under a re-armed boundary). */
function heldUnder(owner, t, fn) {
  for (let c = owner._firstChild; c !== null; c = c._nextSibling) {
    if ((c._config & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD && txOf(c) !== t) fn(c);
    heldUnder(c, t, fn);
  }
}
/** `n` leaves the hold it was staged in: its next pass publishes mainline. */
function unhold(n) {
  n._x._transaction = null;
  n._config &= ~CONFIG_HELD;
}
const dirtyQueue = {
  _heap: new Array(2000).fill(undefined),
  _marked: false,
  _min: 0,
  _max: 0
};
let clock = 0;
let scheduled = false;
let halted = false;
let haltNotified = false;
// Dev: where the halt happened (the owner path and source lines of its
// diagnostic), repeated on the "Update ignored" notice so a frozen interaction
// points back at the computation that threw (#4178).
let haltedAt = "";
// Dev: the computation the halt in progress is located at (haltReactivityAt).
let haltSubject;
let syncDepth = 0;
let inTrackedQueueCallback = false;
let _enforceLoadingBoundary = false;
let _hitUnhandledAsync = false;
// Once per enforcement window: the ASYNC_OUTSIDE_LOADING_BOUNDARY finding is a
// fact about the MOUNT ("the root mount will be deferred"), not about each
// pending render effect — N async siblings at mount used to produce N copies.
let _reportedUnhandledAsync = false;
/**
 * Consume the unhandled-async hit. Returns whether this is the first report
 * of the current enforcement window — the caller warns only then.
 */
function resetUnhandledAsync() {
  _hitUnhandledAsync = false;
  if (_reportedUnhandledAsync) return false;
  _reportedUnhandledAsync = true;
  return true;
}
/**
 * Toggles the dev-mode "must be inside a `<Loading>` boundary" enforcement
 * window. Only `render()` calls this — wrapping the initial mount so that a
 * top-level uncaught async read surfaces the diagnostic. Not part of the
 * user-facing API.
 *
 * @internal
 */
function enforceLoadingBoundary(enabled) {
  _enforceLoadingBoundary = enabled;
  if (enabled) _reportedUnhandledAsync = false;
}
function setTrackedQueueCallback(value) {
  inTrackedQueueCallback = value;
}
// Dev-only marker for the effect half of createEffect/createRenderEffect, so
// flush() can report the no-op instead of failing silently (React parity).
/** @internal Action bodies on the stack (action.ts): `flush()` is refused
 * inside one — the body's writes are its transaction's and commit when it
 * settles; a drain mid-step cannot reveal them, and dev says so. Read live
 * by `@solidjs/web`'s server function transport: a call made while it is
 * nonzero is the action's mutation (#4913). */
let actionDepth = 0;
function enterAction() {
  actionDepth++;
}
function exitAction() {
  actionDepth--;
}
/** Provenance (A18, #3331): the question being asked — an action's
 * sequence while a slice of its body and the flush that carries it run, 0
 * otherwise. A guess is stamped with it (lanes.ts; a mainline guess takes a
 * fresh sequence: the newest intent), a flight with the question that
 * started it (async.ts; mainline is always the current question). An
 * answer from an older question than the guess it lands on is stale: held
 * silently, it does not move the graph. */
let question = 0;
let questions = 0;
function setQuestion(q) {
  question = q;
}
function nextQuestion() {
  return ++questions;
}
const MAINLINE_QUESTION = 0x7fffffff;
let inEffectCallback = false;
/** Dev-only: matches no event, so a window or phase holding it applies
 * whatever event is current (see `strictReadEvent`, core.ts). */
const ANY_EVENT = {};
/** Dev-only: the DOM event current when the effect phase was entered. A
 * handler dispatched synchronously from that phase (`el.focus()`) runs under
 * a different event and is not in it (#4235). */
let effectCallbackEvent = ANY_EVENT;
/** Dev-only: the context a callback scope runs in — null for an effect
 * callback, the creating pass's for a ref callback (`runRefCallback`, #4761).
 * A primitive created in that context is created in the callback; one
 * created by a computation the callback pulls is that computation's. */
let effectCallbackOwner = null;
function setEffectCallback(value, event = ANY_EVENT, owner = null) {
  {
    inEffectCallback = value;
    effectCallbackEvent = event;
    effectCallbackOwner = owner;
  }
}
function schedule() {
  if (halted) {
    notifyHalted();
    return;
  }
  if (scheduled) return;
  scheduled = true;
  if (!syncDepth && !globalQueue._running) {
    queueMicrotask(flush);
  }
}
/**
 * Permanently halts the reactive system. Called when a user error escapes
 * every boundary — app state is undefined at that point, so scheduling stops
 * entirely rather than limping along with a half-applied update.
 */
/**
 * The key a root owner carries its client error hook under (`render`'s
 * `onError`) — registered, so a runtime writes it with no import of the hook
 * module (core/error-hooks.ts) and no property mangling in the way. Defined
 * HERE, not there: a runtime that only writes the key must not retain the
 * hook machinery (pay-for-use).
 */
const ROOT_ERROR_HOOK = Symbol.for("solid-js/root-error-hook");
const HALTED = "[REACTIVITY_HALTED]";
function haltReactivity(cause) {
  if (halted) return;
  halted = true;
  // A cleanup error collected before the halt is dropped (owner.ts).
  rethrowCleanupError(true);
  let message = HALTED;
  {
    message +=
      " An uncaught error halted the reactive system. No further updates will be processed. Handle errors with <Errored> or treat this as a crash.";
    // Located at the computation the error escaped from when the call site
    // named it (haltReactivityAt), so the console says where it happened
    // (`in <App> › <Composer>`), not only that it did (#4178). Otherwise the
    // ambient owner, as every other diagnostic.
    const entry = emitDiagnostic(
      {
        code: "REACTIVITY_HALTED",
        kind: "error",
        severity: "error",
        message
      },
      haltSubject
    );
    haltedAt = locationLines(entry);
    message += haltedAt;
  }
  // Surface the cause here too: callers rethrow it, but a creation-time throw
  // unwinds through ancestor recomputes that convert it to status instead of
  // surfacing it (#2884), so the rethrow alone cannot guarantee visibility.
  // Where the platform has one, hand the cause to its uncaught-error channel
  // (`reportError` → `error` event → window.onerror / error monitoring): a
  // halt that only reaches console.error leaves a page that LOOKS alive with
  // nothing an app or its telemetry can act on (#3338 — an uncaught throw
  // during the hydration render). The rethrow may reach the top as well in
  // the non-swallowed cases; a duplicate report beats a silent one.
  const report = cause !== undefined && globalThis.reportError;
  report || cause === undefined ? console.error(message) : console.error(message, cause);
  report && report(cause);
}
/**
 * Dev: `haltReactivity`, located at `subject` — the computation the uncaught
 * error escaped from — for the REACTIVITY_HALTED message's owner path. Call
 * sites pick it in dev builds only (`haltReactivity(e)` in production), so
 * production keeps the one-argument halt and its bytes unchanged.
 */
function haltReactivityAt(cause, subject) {
  haltSubject = subject;
  haltReactivity(cause);
  haltSubject = undefined;
}
// Logs on the first write after a halt so a frozen interaction is traceable.
function notifyHalted() {
  if (haltNotified) return;
  haltNotified = true;
  console.error(
    "[REACTIVITY_HALTED] Update ignored: the reactive system was halted by an earlier uncaught error." +
      haltedAt
  );
}
/** @internal Test/dev-reload hook. Revives scheduling after a halt. */
function resetErrorHalt() {
  halted = false;
  haltNotified = false;
  haltedAt = "";
  // A throw out of the pure phase skips the `scheduled` recompute; re-arm it.
  if (scheduled) {
    scheduled = false;
    schedule();
  }
}
class GlobalQueue {
  _queues = [[], []];
  _running = false;
  flush() {
    if (this._running) return;
    this._running = true;
    // A28: companions of nodes written since the last flush mirror the
    // flushed world — re-synced inside the running window (the write is
    // "flushed" from here on).
    resyncUnflushedCompanions();
    passLane = null;
    passTx = null;
    try {
      // Sweep before the heap: unobserved() pulls swept nodes out of it, so
      // a dormant memo dirtied in the same tick is reclaimed instead of
      // recomputed; late subscribers (an effect reading a swept memo this
      // flush) revive it, which is the pay-for-use contract.
      sweepDormant();
      if (revertHook) revertHook();
      runHeap(dirtyQueue, GlobalQueue._update);
      this.settle();
      clock++;
      // A write the commit sweep staged with no subscriber to dirty is work
      // too — the next round commits it.
      scheduled = dirtyQueue._max >= dirtyQueue._min || pendingNodes.length !== 0;
      this.run(EFFECT_RENDER);
      this.run(EFFECT_USER);
      if (true && holding) {
        holding = false;
        attrHooks.holdEnd();
      }
      if (true && !scheduled) {
        // Fully drained: no staged value may survive this point unqueued or
        // unheld.
        devCheckQuiescent(
          n => pendingNodes.includes(n) || (n._config & (CONFIG_HELD | CONFIG_OVERRIDE)) !== 0
        );
      }
      if (true) DEV.hooks.onUpdate?.();
    } finally {
      this._running = false;
      passTx = null;
    }
  }
  /** L2 — the seam: end of the pure phase. Commit this flush's staged nodes,
   * or park them with the transaction the flush joined; then land every
   * transaction no frame is waiting on. The landing flush is an ordinary
   * flush: the settle walk (async.ts) re-ran the parked readers in its pure
   * phase, and here no flight the transaction holds has a frame deriving
   * from it (`blocked`). */
  settle() {
    // Lanes: an ended action body judges its guesses now that their passes
    // have run (lanes.ts); a correction's re-derivations run in one more
    // pure round — the parent's flights before the parent is judged below.
    if (laneCorrectionsHook?.()) runHeap(dirtyQueue, GlobalQueue._update);
    const joined = flushTransaction;
    flushTransaction = null;
    // The frame's verdict is in: the guesses written since the last seam
    // open their lane under the frame's transaction, or are dropped — the
    // frame commits, and there is nothing to be optimistic over. (A lane is
    // never the flush's transaction, D-37: lanes do not nest.) Their passes run next round;
    // a parent landing at this very seam ends the lane before they do.
    applyGuessesHook?.(joined);
    // A lane's own flush (its flight landing, its reveal) parks nothing: the
    // lane's work reached it per pass; the frame's plain writes commit.
    // (Resolved: a guess over another transaction's guess merged them.)
    const t = joined !== null && !joined._lane ? resolveTx(joined) : null;
    if (t !== null) {
      // `t._nodes` grows under the walk (holdFrame pushes); the bound is
      // this flush's list, which does not.
      const count = pendingNodes.length;
      for (let i = 0; i < count; i++) {
        const n = pendingNodes[i];
        // Already held: the pass that re-staged it joined its transaction,
        // which is `t` (or merged into it). Lane work is the lane's (a
        // pending mark propagated onto it queued it here too).
        if (
          !(n._config & (CONFIG_HELD | CONFIG_OVERRIDE)) &&
          (n._x === null || n._x._transaction === null || !n._x._transaction._lane)
        ) {
          // A loading source — pending with nothing staged and no committed
          // value (A16, A19 exc. 1: uninitialized is loading, not pending)
          // that is not an observer — is nobody's frame: not held. Its
          // first landing is commit #0, not a resumption of this frame, and
          // a write that reaches it later overlaps nothing (#2937: the
          // stale stamp was the bridge that entangled unrelated updates
          // with a never-resolving flight). Its observers are held and
          // wait on its flight (`blocked`).
          if (
            n._pendingValue === NOT_PENDING &&
            n._statusFlags & STATUS_UNINITIALIZED &&
            n._type !== EFFECT_RENDER
          )
            continue;
          // A34 (2): a tick whose writes net to the committed value made no
          // proposal (`setShow(false); setShow(true)` on a committed true) —
          // not staged, not held, pends nothing; a later write to it is
          // plain mainline. (A node already held stays held when written
          // back: A34 (1).)
          if (
            n._pendingValue !== NOT_PENDING &&
            !(n._statusFlags & (STATUS_PENDING | STATUS_UNINITIALIZED)) &&
            n._x?._pendingFirstChild == null &&
            n._x?._pendingDisposal == null &&
            // A recovery to the last good value is a change of answer
            // (D-41): the error it replaces stays the display until it
            // commits, so it is held, not netted.
            !n._x?._shownError &&
            // The comparator last: a pass that created children (an object
            // owning a fresh async memo, #4111) must not have it read them.
            // A NotReady read there is a change (`isSame`).
            isSame(n, n._value, n._pendingValue)
          ) {
            n._pendingValue = NOT_PENDING;
            n._config &= ~CONFIG_STAGED;
            continue;
          }
          holdNode(n, t);
        }
        // A held pass's frame is held with it (ruling A, O2): the children it
        // created are the transaction's — they compute, and a pass over them
        // or a write that reaches them joins; they effect at the landing.
        if (n._config & CONFIG_STAGED) holdFrame(n, t);
      }
      pendingNodes.length = 0;
      storeParkHook?.(t);
      // (This flush's runs are stashed with `t` below — after the landings,
      // so a `t` that lands at this very seam runs them first, ahead of
      // the runs it held from earlier flushes.)
      // The batch joins: the writes that dirtied these are held with it — a
      // zombie never displays that world. A verdict reader among them does
      // (#3444: `latest()` inside a branch a held Show is removing peers
      // through like the same read outside): it re-runs now, as the
      // holder's verdict lane's work — before the lane seams, which reveal
      // it (nothing commits this seam that could dispose it).
      // (Verdict readers exist only with the verdict engine installed.)
      const zombies = deferredZombies.splice(0);
      if (observeFlightHook) {
        zombies.sort((a, b) => a._height - b._height);
        for (let i = 0; i < zombies.length; i++)
          if (
            (zombies[i]._flags & (REACTIVE_DISPOSED | REACTIVE_ZOMBIE)) === REACTIVE_ZOMBIE &&
            zombies[i]._config & CONFIG_VERDICT
          )
            recompute(zombies[i]);
      }
      // The unchanged passes' tails stay linked (A30, #3469): their inputs
      // are held, and the committed frame still derives from them — no trim.
    } else {
      commitPendingNodes();
      // The flush committed: an unchanged pass's stale tail goes now. Not
      // after a later pass this flush that threw (NotReady included — it
      // keeps its full list, `_depsTail` marking where it stopped) or that
      // left an effect owing a run (`runEffect` trims when the run applies):
      // `recompute`'s own gates.
      for (let i = 0; i < heldTrims.length; i++) {
        const t = heldTrims[i];
        if (t._x?._error == null && !t._modified) trimStaleDeps(t);
      }
    }
    // The list is the flush's either way. Reset only when there is something
    // to reset (`stagedReaders` below likewise): `length = 0` is a runtime
    // call even on an empty array, and the plain flush — no unchanged pass,
    // no lane work — has nothing in either.
    if (heldTrims.length) heldTrims.length = 0;
    // Lanes: a blocked one parks its frame (its own flight is up); an
    // unblocked one reveals this round's work. Then land every transaction
    // no frame is waiting on — its lanes end with it. Backwards: a landing
    // removes its entry.
    // This flush's own runs, the lanes' reveals, and the landings' held runs
    // are collected apart and ordered below.
    const own = this._queues;
    let lanes = null;
    // The passes that read this frame's stagings as the screen read held
    // writes if it parked: the lane seam re-derives them on the committed
    // world (a lane's runs wait this round).
    if (laneSeamsHook) {
      this._queues = lanes = [[], []];
      laneSeamsHook(t);
      if (stagedReaders.length) stagedReaders.length = 0;
    }
    this._queues = [[], []];
    for (let k = transactions.length - 1; k >= 0; k--) {
      const u = transactions[k];
      if (blocked(u)) continue;
      transactions.splice(k, 1);
      endLanesHook?.(u);
      land(u);
      // A landing's commits can dispose a zombie a transaction judged above
      // was blocked on (#3463: live "until the commit that disposes it") —
      // judge them again.
      k = transactions.length;
    }
    // The store folds the pending backings whose container nodes committed
    // — this flush's, or a landing's (store/store.ts installs it).
    storeCommitHook?.();
    // Marks declared outside a transaction: the flush that carried them
    // parked (something async below) — the window is the frame's, the marks
    // are its transaction's; else they release at the seam, verdict-only,
    // nothing to show (affects.ts).
    const live = t && liveTx(t);
    releaseAmbientMarksHook?.(live);
    // The effect phase: lane work first (displayed ahead of the frame), then
    // this flush's runs, then the runs the landings held from earlier
    // flushes — as one pass would have queued them (#3540: a shell
    // re-derived at the landing before a boundary swap held since the
    // write; #3528: a display-ahead swap before the count it is shown
    // beside). A parked frame's runs wait with its transaction instead: the
    // effect phase applies a committed frame.
    const parked = live !== null;
    // Attribution hook: this flush found its transaction incomplete — its
    // writes stay staged, its runs are stashed below. Before the effect
    // phase (the lanes' display-ahead runs are the visible acknowledgers);
    // `holdEnd` fires after it (`flush`).
    if (parked && attrHooks !== null) {
      attrHooks.holdStart(live);
      holding = true;
    }
    for (let i = 0; i < 2; i++) {
      if (parked) append(live._queues[i], own[i]);
      // The plain flush — no reveal, no landing, not parked — has nothing to
      // order around its own runs: they are the queue as they stand (the
      // same array, not a copy; `run` takes it whole). Every other seam
      // builds the ordered queue above.
      this._queues[i] =
        parked || lanes?.[i].length || this._queues[i].length
          ? (lanes ? lanes[i] : []).concat(parked ? [] : own[i], this._queues[i])
          : own[i];
    }
    if (deferredZombies.length !== 0) {
      // Survivors are the displayed frame of a held pass: they rerun for the
      // live write, parents before children.
      const zombies = deferredZombies.splice(0).sort((a, b) => a._height - b._height);
      for (let i = 0; i < zombies.length; i++) {
        if (!(zombies[i]._flags & REACTIVE_DISPOSED)) recompute(zombies[i]);
      }
    }
    // Boundaries: readers that settled, landed or died this flush are
    // dropped; a fallback with none left reveals next round.
    boundarySeamHook?.();
  }
  run(type) {
    const effects = this._queues[type - 1];
    if (effects.length) {
      this._queues[type - 1] = [];
      runQueue(effects, type);
    }
  }
  enqueue(type, fn, lane = passLane) {
    // Lane work's runs are the lane's: they run at its reveal, not with
    // the frame (which may park). A lane pass that read the frame's own
    // staging is the frame's (`recompute` passes null): its run waits with
    // the frame. So is a run given a transaction that is not a lane (a
    // first run born held, `effect()`).
    if (type) (lane?._lane ? lane : this)._queues[type - 1].push(fn);
    schedule();
  }
  /** Status reaching the root: pending is absorbed (the root mount defers —
   * dev records it for the ASYNC_OUTSIDE_LOADING_BOUNDARY FYI); an error is
   * unhandled (`false` → the caller halts).
   *
   * L2: this is where a transaction is registered. Pending that reaches an
   * observer (a render effect: the frame) is a frame that cannot show — the
   * flush parks, and the transaction waits on the flight while a frame
   * derives from it (`blocked`). Pending nobody observes holds nothing: a
   * memo in flight that no frame reads is not a frame waiting. (Base: the
   * queue's `_asyncReporters`, INV-3's one registration site.)
   *
   * A boundary between the observer and the root (boundaries.ts) is asked
   * first: a fallback catches what is not ready under it, and the root
   * never hears of it. */
  notify(node, mask, flags, error) {
    if (catchHook?.(node, flags & mask, error)) return true;
    if (mask & STATUS_PENDING) {
      if (flags & STATUS_PENDING) {
        if (_enforceLoadingBoundary) _hitUnhandledAsync = true;
        // A pass that went pending holds its frame: the async's own
        // synchronous frame is not ready (A15 reveal corollary: a reveal
        // that discovers a flight whose inputs are visible holds and joins
        // it; one whose inputs are held is a stale reader and never gets
        // here — `read` serves it the committed value). The frame's, not the
        // observer's: a render effect belongs to whoever dirtied its pass
        // (A15 shared-hole, #3407) — one held by T1 and re-run by T2's write
        // pends for T2's flight, and the two stay parallel; a frame that is
        // T1's already joined T1 through the write or the memo that made it
        // so. Outside a flush — root setup, a mount — the frame is the
        // mount's: its pending observers are one transaction, and a flight
        // started in it resumes it when it lands (maintainer, #3461: "part
        // of the same one"); what the mount published synchronously stays
        // published. Lane work going pending holds the lane, not the frame:
        // `blocked(lane)` finds it in the lane's staging ("if further async
        // downstream is hit it holds like its own transition").
        if (passLane === null) joinFuture(null);
      }
      return true;
    }
    return false;
  }
}
/** A15's stale readers: a frame whose LAST pass read a held node as
 * committed re-derives on the landed world — next round, after the commits.
 * One re-derived since (any later pass cleared the bit: in the transaction,
 * or without the held read) owes nothing. A run this transaction stashed
 * for it is void (#3322: an effect has one value slot, and the stale pass
 * overwrote the value that run was for) — the re-derivation's run replaces
 * it. Truthy when an effect re-derives (`land`). */
function reruns(u) {
  let frame;
  for (let i = 0; i < u._reruns.length; i++) {
    const r = u._reruns[i];
    // An errored reader's display is its error (D-39): the landing gives it
    // nothing to re-derive, and a re-run would be a read that retries the
    // errored source (#3945) — unless that source is errored no longer: the
    // reader was served a shown error the landing replaces (`throwShown`).
    // Nor is the reader's own error: its pass threw over the committed value
    // of what the landing carries, served as the screen (A10; a lane's node
    // reading its parent's hold, or a pass with no value yet reading a
    // staging, `stagedScreen`) — the landing is the new answer, and the
    // reader re-derives on it (A15; D-39). A first load read so has no other
    // run: its error would stay the screen (async-gen 91501 #1158, #119).
    // A boundary's structure (its tree, its output: CONFIG_FRAME, no effect
    // type) re-derives all the same: what it read as the screen is the
    // content the landing replaces, and only its pass links the new content
    // whose recovery must reach it (D-3, D-39, #4405).
    const s = r._x?._error?.source;
    if (
      r._flags & REACTIVE_DISPOSED ||
      (r._statusFlags & STATUS_ERROR &&
        (r._type || !(r._config & CONFIG_FRAME)) &&
        s !== r &&
        (s?._statusFlags ?? STATUS_ERROR) & STATUS_ERROR)
    )
      continue;
    if (r._flags & REACTIVE_FRAME_READ || (observeFlightHook && r._config & CONFIG_VERDICT)) {
      // A memo re-deriving its own error holds the release as an effect
      // does (`land`): run first, the held runs showed that error beside a
      // frame about to replace it (#4407; #4284: the grouping never changes
      // the screen). So does a memo the landing holds (CONFIG_HELD: its
      // staged value is in this frame): the held runs read it, so shown
      // first they showed the stale pass the re-derivation replaces — one
      // frame reveals the change (F-1, A15; #4648). And any other: a memo
      // that asks, or one over it (D-25: a memo over a verdict memo is a
      // verdict reader), was served the screen and asked the screen's
      // question (F-6, S-51); it asks the landed world's now — the held
      // runs park with that flight, never shown beside the answer it
      // replaces.
      frame = 1;
      // A lane's reveal leaves the mark to the transaction: a reader that
      // also read a held node as committed (`staleReader`) still observes
      // that flight until it re-runs — cleared here, the transaction landed
      // at this seam with the flight up and voided the reveal's run (#4397).
      if (!u._lane) r._flags &= ~REACTIVE_FRAME_READ;
      r._modified = false;
      // Its frame is replaced by the re-derivation: a run one of its
      // children queued this flush (re-run by the same landing) is for the
      // frame being replaced — void too (#3404).
      for (let c = r._firstChild; c !== null; c = c._nextSibling) if (c._type) c._modified = false;
      // A frame flight the seam parked (verdict.ts `r5`, R5) asked the
      // question this landing would: adopted, it is asked once (F-9).
      laneAdoptHook?.(r);
      enqueueSub(r);
    }
  }
  u._reruns.length = 0;
  return frame;
}
/** The landing: the held frame becomes the frame. */
function land(u) {
  // Attribution hook: judged complete — its held writes commit next
  // (`_nodes` still lists them).
  if (attrHooks !== null) attrHooks.transitionSettled(u);
  const frame = reruns(u);
  if (releaseMarksHook && u._marks !== null) releaseMarksHook(u._marks);
  // Old children die in the commits (cleanups first), then the stashed
  // effects run ahead of this flush's own. A node a lane took over since
  // (a guess written over the staged truth) is the lane's to land.
  for (let i = 0; i < u._nodes.length; i++) {
    const n = u._nodes[i];
    if (n._x._transaction === null || txOf(n) !== u) continue;
    n._x._transaction = null;
    commitPendingNode(n);
  }
  // A cleanup those commits ran that wrote a node `u` still held joined
  // `u`, which has left `transactions`: the write is this landing's, and
  // the next flush must not park in it (#4089).
  if (flushTransaction === u) flushTransaction = null;
  // An effect re-derives next round (`reruns`): the held runs wait for it
  // (D-9, the A15 reveal corollary, #4071). Its re-derivation can discover a
  // flight — a stale reader whose mainline pass stopped reading it, now
  // reading the landed world — and a landing that showed its runs first
  // would reveal the held write beside a frame that can only show its old
  // value. Queued last in this effect phase, the release hands them to the
  // next round's own runs, ahead of the re-derivation's: they show with it,
  // or park with it in the hold its pending opens. The commits stand.
  if (frame) globalQueue._queues[1].push(() => releaseQueues(u));
  else releaseQueues(u);
}
/** The held runs join this flush's effect phase after its own runs (the
 * landing's re-passes, in height order): the frame's effects run as one
 * pass would have queued them — a shell re-derived at the landing before a
 * boundary swap held since the write (#3540's log order). */
function releaseQueues(u) {
  for (let i = 0; i < 2; i++) {
    append(globalQueue._queues[i], u._queues[i]);
    u._queues[i] = [];
  }
}
/** `a.push(...b)` without the argument-count limit. */
function append(a, b) {
  for (let i = 0; i < b.length; i++) a.push(b[i]);
}
/** Nodes staged this flush (`_pendingValue` set), committed at its end. */
const pendingNodes = [];
/** Held slots rewritten since the last flush, with the verdict engine (D-8: `revertHook`). */
const heldRewrites = [];
/** Passes that read a staging of this flush as the screen — lane work
 * (core.ts `stagedRead`), a verdict reader before the frame's verdict
 * (verdict.ts): if the frame parks they read a held write — re-derived next
 * round (lanes.ts; a lane's runs wait that round). */
const stagedReaders = [];
/** A15's stale reader: `c` was served the committed value of a node `t`
 * holds (a render effect, a verdict reader, lane work) and is re-derived
 * after `t`'s landing (`_reruns`). Called per read of a held node, so `t`'s
 * list holds one entry per such read, not one per pass: `reruns` re-runs the
 * reader at its first entry and skips the rest by the cleared flag. (A dedupe
 * here costs ~17-30 B minified in every client bundle, which the 10 KB
 * compiled floor cannot take; the repeats are inert and short-lived — the
 * list empties at the landing.) */
function staleReader(c, t) {
  // On each hold the pass reads committed from: one pass can read two (a memo
  // the flush's hold stages, a row another action's hold keeps), and each
  // landing re-runs it. Guarded by the flag alone, the reader was listed on
  // the first hold only and missed the second's landing (#4932).
  t._reruns.push(c);
  c._flags |= REACTIVE_FRAME_READ;
}
function queuePendingNode(node) {
  lastStagedNodeName = node._name ?? null;
  pendingNodes.push(node);
  if (!globalQueue._running) markUnflushedStaged(); // A28
  if (attrHooks !== null) attrHooks.staged(node);
}
// Dev-only attribution for the flush loop guard (#3140): when the guard
// trips, naming what the loop kept chewing on lets the app author attribute
// the runaway without patching dist.
let lastStagedNodeName = null;
/** §12d: bumped by every recompute and every new subscriber edge. A node's
 * staged-rewrite skip is sound only while NOTHING recomputed or linked since
 * its last notify — a mid-batch pull can clean a marked subscriber, and a
 * skipped re-write would leave it stale. */
let notifyEpoch = 0;
function bumpNotifyEpoch() {
  notifyEpoch++;
}
function insertSubs(node) {
  // §12d: stamp before walking — setSignal's staged-rewrite fast path skips
  // the next walk for this node while the epoch holds (marking is idempotent).
  node._notifiedAt = notifyEpoch;
  // Presence bits gate the optional-slot probes (see constants.ts): one
  // masked read of the always-present _config instead of missing-property
  // lookups in the hottest notify loop.
  const cfg = node._config;
  const hasSnapshot = (cfg & CONFIG_HAS_SNAPSHOT) !== 0 && node._x?._snapshotValue !== undefined;
  // Observe-tier fan-out: this walk visits every subscriber edge anyway, so
  // the graph-size count is one local increment here and no field anywhere.
  let fanOut = 0;
  for (let s = node._subs; s !== null; s = s._nextSub) {
    const sub = s._sub;
    fanOut++;
    // Missed-wake latch (#3037): this write is landing while the subscriber
    // is mid-recompute (a nested pull committing beneath its reads), and the
    // heap refuses RECOMPUTING nodes. A gen-current link means the pass
    // already validated this dep — the value it read is now stale — so latch
    // for recompute's tail to reschedule. Untouched links need no latch (the
    // pass either re-reads them fresh or trims them), and neither does the
    // tail link: it is the read IN FLIGHT — read() links before it pulls, so
    // this very commit is what that read returns.
    // A signal's read returned before any write to it — the pass's own
    // owned write to what it just read is a missed wake too (F-10).
    if (
      sub._flags & REACTIVE_RECOMPUTING_DEPS &&
      s._gen === sub._depGen &&
      (s !== sub._depsTail || !node._fn)
    )
      sub._flags |= REACTIVE_MISSED_WAKE;
    if (hasSnapshot && sub._config & CONFIG_IN_SNAPSHOT_SCOPE) {
      sub._flags |= REACTIVE_SNAPSHOT_STALE;
      continue;
    }
    enqueueSub(sub);
    if (sub._flags & REACTIVE_OWNER_READS) markNode(sub._parent, REACTIVE_CHECK);
  }
  if (fanOut >= GRAPH_SIZE_WARN_AT) noteFanOut(node, fanOut);
}
/** Every flight `c` waits on is a `refresh()` re-ask (`_reask`): its own,
 * with no sources, or each source's, through a source that waits on
 * another's flight (a projection over the re-asked memo, #4824). The
 * propagated copies can form a cycle: two keyed projections whose derives
 * each read a row lent from the other's store are each pending on the
 * other's derive (D-1, D-9: a read of a pending leaf is pending on it), and
 * both on the real flight (#4969). A source already walked adds no flight of
 * its own; the flights at the ends of the walk decide. */
function reasks(c, seen = new NewSet()) {
  return [...(c._x._pendingSources ?? [c])].every(p =>
    p === c ? p._x._reask : seen.has(p) || (seen.add(p), reasks(p, seen))
  );
}
/** Lane work re-staged (lanes.ts; `recompute`): the node's subscribers are
 * notified (`insertSubs`), and the lane's members among them re-derive as
 * the lane's work (REACTIVE_LANE_DIRTY), their runs held with it — not as
 * stale readers republishing the committed view (#3460). `l` is a lane, so
 * it has a parent. A render effect on no lane is marked too: inert for its
 * pass (`recompute` seats a lane only from its own `_x`), it is a live
 * write if the effect becomes a zombie later this flush — an
 * on-screen reader whose removal an adopted write stages follows the lane
 * (the lane-membership ruling, 2026-10-05; `_update`).
 *
 * A render effect re-run by a verdict memo's change (`createMemo(() =>
 * latest(x))`, CONFIG_VERDICT) is the verdict reader the same read inline
 * makes it, for that pass (F-6; D-26 `[s(), L()]`, #4284): a frame reader
 * of its other reads — a staging's screen, a flight's committed value. One
 * an earlier propagation this flush seated in another lane leaves that
 * seat: its pass starts from the frame's, as the inline effect's does, and
 * `laneRead` re-derives it on the screen if that lane holds (`entered`). */
function laneDirty(node, l) {
  insertSubs(node);
  // A store's guess carrier: the readers of the keys it changed read their
  // leaves — dirtied as the lane's work too (store/optimistic.ts).
  if (node._config & CONFIG_SLOT_NODE) GlobalQueue._slotDirty?.(node, l);
  for (let s = node._subs; s !== null; s = s._nextSub) {
    const c = s._sub;
    let t = c._x?._transaction,
      render = c._type === EFFECT_RENDER,
      same;
    // A reader a hold has pending on its re-ask (a correction's
    // re-derivation waiting on a refresh) re-derives from the guess as the
    // lane's work, seated there before its pass: lane work reads the
    // hold's flight as the screen (D-26, D-37), whether it reads the
    // flight before the guess or after (D-28), and whether it reads the
    // guess itself or a derivation of it — this walk runs at every lane
    // node's change. Seated at the guess read, the earlier read threw the
    // pass back into the hold (#4629). The hold is the lane's parent's (the
    // same slot, D-33), or any hold whose flights the reader waits on are all
    // `refresh()` re-asks (`_reask`; with none, its own flight): a re-ask
    // asks no new question and holds nothing new (D-34 ¶3), so a guess into
    // another log shows beside it at once (F-11, #4666). A reader pending on
    // another hold's new question stays with that hold (#4577: seated, it
    // would ask its question over the screen's input). One seated in another
    // lane this flush moves; its pass reads that lane's guess and links the
    // two (`enterLane`, A15), and `dissolveLane` skips a node it lost.
    if (
      (c._statusFlags & (STATUS_PENDING | STATUS_UNINITIALIZED)) === STATUS_PENDING &&
      // A render effect pending on another hold's re-asks, held by it or
      // by none, the same (#4824): its first read of the re-asked rows threw
      // it back out of the lane, and the guess beside a reader that showed
      // it waited on the refresh (F-1, F-11).
      (t ? (c._config & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD : render) &&
      ((same = !!t && resolveTx(t) === resolveTx(l._parent)),
      c._type ? render && !same && reasks(c) : same || reasks(c))
    )
      list(c, (t = l));
    if (render && node._config & CONFIG_VERDICT)
      ((c._config |= CONFIG_VERDICT), (c._flags &= ~REACTIVE_LANE_DIRTY));
    if (t?._lane ? sameLane(t, l) : c._type) c._flags |= REACTIVE_LANE_DIRTY;
  }
}
/** An effect owes a run: marked, and its bound runner queued in its phase. */
function queueEffectRun(node) {
  node._modified = true;
  globalQueue.enqueue(
    node._type,
    (node._boundRunEffect ??= GlobalQueue._runEffect.bind(null, node))
  );
}
/** One lane, or two of one link group (#3335): one reveal unit. */
function sameLane(a, b) {
  return a === b || (a._links !== null && a._links === b._links);
}
/** A staged value becomes the node's value: truthy when one was staged. */
function applyStaged(n) {
  return (
    n._pendingValue !== NOT_PENDING &&
    ((n._value = n._pendingValue), (n._pendingValue = NOT_PENDING))
  );
}
function superseded(n) {
  return (
    !(n._statusFlags & STATUS_UNINITIALIZED) &&
    !!n._x?._inFlight &&
    !n._x._reask &&
    ((n._pendingValue = NOT_PENDING), true)
  );
}
function commitPendingNode(n) {
  // L2: the commit is where a pass, held or not, stops being uncommitted —
  // unless it has no answer yet. A pass still in flight (pending, nothing
  // staged) stays uncommitted: a re-pass disposes its children on the spot,
  // and the frame it parked stays the frame until a pass of this node
  // commits a value.
  if (!n._fn) {
    n._config &= -385;
    applyStaged(n);
    return;
  }
  // Computeds only from here (`_statusFlags` is not a signal field). A
  // commit beneath a flight publishes its inputs (A15 reveal corollary,
  // #3305): a reveal of it must observe, not show the pre-flight value.
  // A staged answer to a question a later pass of this hold replaced, its
  // own flight up for the new one, is not the frame's: the hold lands on the
  // answer to its own question (F-2, 2026-11-12) — the node stays in flight
  // on the committed value, and the stale staging goes.
  // (A cold helper: inline, the arm cost the commit loop its inlining.)
  const inFlight =
    (n._statusFlags & STATUS_PENDING) !== 0 && (n._pendingValue === NOT_PENDING || superseded(n));
  const err = n._config & CONFIG_STAGED_ERROR;
  // An answer commits: the error it replaces leaves the display (D-41).
  if (!inFlight && n._x) n._x._shownError = null;
  n._config = inFlight ? (n._config & -1048833) | CONFIG_INPUTS_PUBLISHED : n._config & -3147137;
  // D-41: the staged answer is an error — published now, when a value would
  // show, through the ordinary error path (`stageError`). Its readers first
  // see it in the cycle after this seam: a tracked re-read there is not a
  // retry (`read`), or a boundary re-derived by it would retry it forever.
  if (err) {
    const e = n._pendingValue;
    n._pendingValue = NOT_PENDING;
    n._time = clock + 1;
    notifyStatus(n, STATUS_ERROR, e);
  } else if (applyStaged(n)) {
    // A node born staged (recompute) initializes at this commit.
    n._statusFlags &= ~STATUS_UNINITIALIZED;
    // A quiet re-ask's classification survives its landing and dies with
    // the landed value's commit (A19 exc. 2, #3178): verdict-quiet through
    // the reveal. (The sweep of a node still in flight is not a landing.)
    // The A28 stash goes with it.
    if (n._x != null) ((n._x._reask = false), (n._x._flushed = NOT_PENDING));
    // An effect with a staged value was born held (L2): its first run is
    // this commit's, not its creation's (A29) — queue it now.
    if (n._type && n._type !== EFFECT_TRACKED) queueEffectRun(n);
  }
  // The committed value is the first observable answer for a loading-window
  // node — the window closes here, not at compute time (#2990). Unconditional
  // store to an always-present computed slot.
  n._loading = false;
  n._flags &= ~REACTIVE_MANUAL_WRITE;
  // The dependencies of the pass that produced the value are the frame's now:
  // the previous frame's tail goes (A30, #3410; `recompute` left it for a
  // staged pass). Only after a clean pass: `_error` is cleared by a clean
  // pass or by the node's own landing (whose pass was clean), so a set
  // `_error` means the last pass threw, kept its full list, and `_depsTail`
  // marks where it stopped.
  if (n._x?._error == null) trimStaleDeps(n);
  if (!(n._statusFlags & STATUS_PENDING)) n._statusFlags &= ~STATUS_UNINITIALIZED;
  // L2: the children this commit publishes are the frame's now — the frame
  // they replace, parked by the pass (`recompute`), goes.
  if (
    !inFlight &&
    n._x != null &&
    (n._x._pendingFirstChild !== null || n._x._pendingDisposal !== null)
  )
    GlobalQueue._dispose(n, false, true);
}
function commitPendingNodes() {
  for (let i = 0; i < pendingNodes.length; i++) {
    const n = pendingNodes[i];
    // A node a lane pass re-staged this round is the lane's to reveal; one
    // a transaction holds (born held by a mainline pass, `passTx`) is that
    // transaction's to commit, at its landing.
    if (!(n._config & (CONFIG_OVERRIDE | CONFIG_HELD))) commitPendingNode(n);
  }
  pendingNodes.length = 0;
}
const globalQueue = new GlobalQueue();
function flush(fn) {
  if (actionDepth > 0) {
    {
      throw new Error(
        "[FLUSH_IN_ACTION] flush() inside an action body is not allowed. An action's writes are held in its " +
          "transaction and commit when the action settles: flush() cannot reveal them, and draining here would " +
          "detach the writes that follow from the transaction. Remove the flush(); to observe the result, read " +
          "after the action resolves."
      );
    }
  }
  if (fn) {
    syncDepth++;
    try {
      return fn();
    } finally {
      // Decrement even if the drain throws (a throwing effect): a leaked
      // syncDepth would stop `schedule()` from ever queuing a microtask again.
      try {
        flush();
      } finally {
        syncDepth--;
      }
    }
  }
  if (globalQueue._running) {
    if (inTrackedQueueCallback) {
      throw new Error(
        "Cannot call flush() from inside onSettled or createTrackedEffect. flush() is not reentrant there. " +
          "Writes made here are processed in the same flush's continuation; to force a drain afterwards, defer it: queueMicrotask(() => flush())."
      );
    }
    if (inEffectCallback) {
      const message =
        "[FLUSH_IN_EFFECT_CALLBACK] flush() called from inside an effect callback is a no-op: the flush that runs effects is already in progress. " +
        "Writes made here are processed in the same flush's continuation; to force a drain afterwards, defer it: queueMicrotask(() => flush()).";
      reportDiagnostic(
        emitDiagnostic({
          code: "FLUSH_IN_EFFECT_CALLBACK",
          kind: "lifecycle",
          severity: "warn",
          message
        })
      );
    }
    return;
  }
  if (halted) return;
  let count = 0;
  // Attribution: whether this call drained anything, so `flushEnd` fires once
  // per real drain and never for a no-op call. The declaration is dead in
  // prod (its only write is behind true) and rollup drops it.
  let drained = false;
  // The drain's opening instant, under the loop's own condition so it fires
  // exactly when `flushEnd` below will. Outside every try (see the rule in
  // attribution-hooks.ts).
  if (attrHooks !== null && scheduled) attrHooks.flushStart();
  while (scheduled) {
    if (++count === 1e5) {
      rethrowCleanupError(true);
      // Attribution beats a bare guard (#3140): say what kept the loop alive.
      throw new Error(
        `Potential Infinite Loop Detected. Kept alive by scheduled work${lastStagedNodeName ? `; last staged node: ${lastStagedNodeName}` : ""}`
      );
    }
    globalQueue.flush();
    drained = true;
  }
  // Outside every try in this function (see the rule in attribution-hooks.ts):
  // the drain loop above is the one place all scheduled work funnels through,
  // so this is the "committed and effects ran" instant for everything the
  // loop processed.
  if (drained && attrHooks !== null) attrHooks.flushEnd();
  // A cleanup that threw during the drain, now that the drain is complete
  // (owner.ts).
  rethrowCleanupError();
}
/** Dev-only: the DOM event current when the running effect queue started,
 * or `ANY_EVENT` outside one. Read once per queue rather than once per
 * effect (the getter is costly under jsdom); no event starts or ends between
 * two effects of one synchronous queue (#4235). */
let queueEvent = ANY_EVENT;
function runQueue(queue, type) {
  {
    const prev = queueEvent;
    queueEvent = dispatchingEvent();
    try {
      for (let i = 0; i < queue.length; i++) queue[i](type);
    } finally {
      queueEvent = prev;
    }
    return;
  }
}

/**
 * Schedule one subscriber to re-run on the next flush: inserted into the
 * dirty heap with the `_min` cursor pulled down. Tracked effects ride the
 * heap too — the heap visit is their (empty) compute phase, which hands the
 * callback to the user queue once the pass has committed (see
 * GlobalQueue._update, #3291).
 */
function enqueueSub(node) {
  if (dirtyQueue._min > node._height) dirtyQueue._min = node._height;
  insertIntoHeap(node, dirtyQueue);
}
function actualInsertIntoHeap(n, heap) {
  const parentHeight =
    (n._parent?._root ? n._parent._parentComputed?._height : n._parent?._height) ?? -1;
  if (parentHeight >= n._height) n._height = parentHeight + 1;
  const height = n._height;
  const heapAtHeight = heap._heap[height];
  if (heapAtHeight === undefined) heap._heap[height] = n;
  else {
    const tail = heapAtHeight._prevHeap;
    tail._nextHeap = n;
    n._prevHeap = tail;
    heapAtHeight._prevHeap = n;
  }
  if (height > heap._max) heap._max = height;
}
function insertIntoHeap(n, heap) {
  let flags = n._flags;
  // RECOMPUTING refusals are not always losses: a genuinely missed wake (a
  // write to a link this pass already validated) is latched link-side in
  // insertSubs as REACTIVE_MISSED_WAKE for recompute's tail (#3037).
  if (flags & (REACTIVE_IN_HEAP | REACTIVE_RECOMPUTING_DEPS)) return;
  if (flags & REACTIVE_CHECK) {
    n._flags = (flags & -4) | REACTIVE_DIRTY | REACTIVE_IN_HEAP;
  } else {
    n._flags = flags | REACTIVE_IN_HEAP;
    // An unmarked node entering an already-marked heap is marked on the
    // spot, keeping the markHeap memo valid. `_marked` is only reset by
    // runHeap, so a write between two mid-tick pulls (read-time markHeap +
    // updateIfNecessary) would otherwise leave this node unmarked and every
    // downstream pull stale until the next flush (#2922: the second
    // `latest()` returned the first write's value). Invalidating the memo
    // instead re-walked the WHOLE heap on the next pull — with N effects
    // parked in the heap for a synchronous mount (each row writing a ref
    // signal its effect subscribes to), mounting N rows was O(N²) (#3350).
    // markNode's own guard skips an already-DIRTY node.
    if (heap._marked) markNode(n);
  }
  if (!(flags & REACTIVE_IN_HEAP_HEIGHT)) actualInsertIntoHeap(n, heap);
}
function insertIntoHeapHeight(n, heap) {
  let flags = n._flags;
  if (flags & (REACTIVE_IN_HEAP | REACTIVE_RECOMPUTING_DEPS | REACTIVE_IN_HEAP_HEIGHT)) return;
  n._flags = flags | REACTIVE_IN_HEAP_HEIGHT;
  actualInsertIntoHeap(n, heap);
}
function deleteFromHeap(n, heap) {
  const flags = n._flags;
  if (!(flags & (REACTIVE_IN_HEAP | REACTIVE_IN_HEAP_HEIGHT))) return;
  n._flags = flags & -25;
  const height = n._height;
  if (n._prevHeap === n) heap._heap[height] = undefined;
  else {
    const next = n._nextHeap;
    const dhh = heap._heap[height];
    const end = next ?? dhh;
    if (n === dhh) heap._heap[height] = next;
    else n._prevHeap._nextHeap = next;
    end._prevHeap = n._prevHeap;
  }
  n._prevHeap = n;
  n._nextHeap = undefined;
}
function markHeap(heap) {
  if (heap._marked) return;
  heap._marked = true;
  for (let i = 0; i <= heap._max; i++) {
    for (let el = heap._heap[i]; el !== undefined; el = el._nextHeap) {
      if (el._flags & REACTIVE_IN_HEAP) markNode(el);
    }
  }
}
function markNode(el, newState = REACTIVE_DIRTY) {
  const flags = el._flags;
  if ((flags & (REACTIVE_CHECK | REACTIVE_DIRTY)) >= newState) return;
  el._flags = (flags & -4) | newState;
  for (let link = el._subs; link !== null; link = link._nextSub) {
    markNode(link._sub, REACTIVE_CHECK);
  }
}
function runHeap(heap, recompute) {
  heap._marked = false;
  for (heap._min = 0; heap._min <= heap._max; heap._min++) {
    let el = heap._heap[heap._min];
    while (el !== undefined) {
      if (el._flags & REACTIVE_IN_HEAP) recompute(el);
      else adjustHeight(el, heap);
      el = heap._heap[heap._min];
    }
  }
  heap._max = 0;
}
function adjustHeight(el, heap) {
  deleteFromHeap(el, heap);
  let newHeight = el._height;
  for (let d = el._deps; d; d = d._nextDep) {
    const dep = d._dep;
    if (dep._fn && dep._height >= newHeight) newHeight = dep._height + 1;
  }
  if (el._height !== newHeight) {
    el._height = newHeight;
    liftOwned(el, newHeight);
    // (Not an owner reading it: it sits below its child by construction,
    // REACTIVE_OWNER_READS, and a seat below the cursor is never visited.)
    for (let s = el._subs; s !== null; s = s._nextSub)
      if (s._sub !== el._parent) insertIntoHeapHeight(s._sub, heap);
  }
}
/**
 * A raised node's owned computations (through roots) move above it (#4848).
 * The heap places a node above its direct owner only, at insert, so an owner
 * raised after its children's birth left a grandchild at or below it, to run
 * before the owner whose run disposes it. Paid when a height changes, never
 * per insert. A child waiting in the heap keeps its seat this pass (its next
 * insert lifts it above its owner); what it owns moves either way.
 */
function liftOwned(o, h) {
  for (let c = o._firstChild; c; c = c._nextSibling)
    if (c._root) liftOwned(c, h);
    else if (c._height <= h) {
      // A child waiting in the heap moves with it: left at its seat it runs
      // before the owner whose run disposes it.
      const f = c._flags & (REACTIVE_IN_HEAP | REACTIVE_IN_HEAP_HEIGHT);
      deleteFromHeap(c, dirtyQueue);
      c._height = h + 1;
      if (f) (f & REACTIVE_IN_HEAP ? insertIntoHeap : insertIntoHeapHeight)(c, dirtyQueue);
      liftOwned(c, h + 1);
    }
}

/** The store's shared slot-node release handler (store/store.ts installs it
 * once) — a live binding read directly by the sweep site: no wrapper frame,
 * no null check (a CONFIG_SLOT_NODE node existing implies the store module
 * loaded and registered it). */
let slotUnobservedHook;
function setSlotUnobserved(fn) {
  slotUnobservedHook = fn;
}
// https://github.com/stackblitz/alien-signals/blob/v2.0.3/src/system.ts#L100
function unlinkSubs(link) {
  const dep = link._dep;
  const nextDep = link._nextDep;
  const nextSub = link._nextSub;
  const prevSub = link._prevSub;
  if (nextSub !== null) nextSub._prevSub = prevSub;
  else dep._subsTail = prevSub;
  if (prevSub !== null) prevSub._nextSub = nextSub;
  else {
    dep._subs = nextSub;
    if (nextSub === null) {
      // Store slot nodes dispatch to the ONE shared hook — no per-node
      // unobserved closure, no NodeExtension to hold it.
      // A user hook that throws is collected like a cleanup (owner.ts): the
      // unlink and the teardown around it carry on.
      if (dep._config & CONFIG_SLOT_NODE) slotUnobservedHook(dep);
      else if (dep._x?._unobserved) runCleanup(dep._x._unobserved);
      // No more subscribers; only tear down if CONFIG_AUTO_DISPOSE is set.
      // A pending node is exempt: its in-flight async work is an observer —
      // tearing down would orphan the work and re-execute it on the next
      // read. The settle path runs this same last-one-out check when that
      // observer releases (the untracked-read dormancy sweep guards on
      // pending identically).
      // During a pass's teardown of its unfinished frame, the release waits
      // for the pass to end (`deferredReleases`, #4142).
      // (No cast alias for `dep`: the minifier keeps one.)
      dep._fn &&
        dep._config & CONFIG_AUTO_DISPOSE &&
        !(dep._flags & REACTIVE_ZOMBIE) &&
        !(dep._statusFlags & STATUS_PENDING) &&
        (deferredReleases ? deferredReleases.push(dep) : unobserved(dep));
    }
  }
  return nextDep;
}
function trimStaleDeps(el) {
  const depsTail = el._depsTail;
  let toRemove = depsTail !== null ? depsTail._nextDep : el._deps;
  if (toRemove !== null) {
    do {
      toRemove = unlinkSubs(toRemove);
    } while (toRemove !== null);
    if (depsTail !== null) depsTail._nextDep = null;
    else el._deps = null;
  }
}
// Shared by unobserved() and the disposeChildren child loop. The truthy guard
// (not `!== null`) matters: plain Owners in a child chain have no _deps field,
// and skipping early also avoids adding one (hidden-class churn) via the
// null-out below. With `_depsTail` cleared, `trimStaleDeps` unlinks the whole
// list and nulls `_deps`.
function clearDeps(el) {
  if (el._deps) {
    el._depsTail = null;
    trimStaleDeps(el);
  }
}
/**
 * Last-one-out releases held back while a pass tears down the frame of an
 * earlier pass that never committed (#4142). The pass is about to rebuild
 * the same children, which read the same lazy nodes again. Released on the
 * spot, a lazy async memo whose only reader was the old frame's child memo
 * was torn down, re-ran on the new child's read, went pending, and the cycle
 * repeated forever. `recompute` collects them around that teardown and
 * releases what is still unobserved when the pass ends.
 */
let deferredReleases = null;
const deferReleases = list => (deferredReleases = list);
function unobserved(el) {
  deleteFromHeap(el, dirtyQueue);
  clearDeps(el);
  disposeChildren(el, true);
}
/**
 * Deferred dormancy for never-observed auto-dispose computeds (#3078).
 *
 * An untracked top-level read of a subscriber-less observation-lifecycle memo
 * used to call unobserved() inline at the end of read(). That kept the leak
 * closed (the compute links the memo into its deps' sub lists — without a
 * teardown point a never-observed memo is retained by its sources forever;
 * upstream alien-signals has exactly this retention), but it made reads
 * destructive: each read disposed the node, the next read revived it with a
 * full recompute in whatever ambient transition/lane context happened to be
 * current, so consecutive reads could return different answers with no write
 * in between.
 *
 * Instead, reads queue the node here and the scheduler sweeps at the top of
 * the next flush (before runHeap, so a same-tick dirtying is reclaimed
 * instead of recomputed). Reads become idempotent within a tick (the node
 * stays alive and serves its cache, uniform with observed memos) while
 * reclamation still happens within one microtask — the enqueue site arms
 * schedule(), so a flush is guaranteed even when no other work is queued.
 */
const dormantNodes = new NewSet();
function sweepDormant() {
  if (dormantNodes.size === 0) return;
  for (const el of dormantNodes) {
    // Re-validate at sweep time: the node may have gained a subscriber (its
    // lifecycle is the unlinkSubs cascade now), gone pending (in-flight async
    // is an observer; the settle path re-runs last-one-out), lost its
    // AUTO_DISPOSE bit (owner teardown strips it, #3024), or already been
    // torn down.
    if (
      !el._subs &&
      el._config & CONFIG_AUTO_DISPOSE &&
      !(el._statusFlags & STATUS_PENDING) &&
      !(el._flags & (REACTIVE_DISPOSED | REACTIVE_ZOMBIE))
    ) {
      unobserved(el);
    }
  }
  dormantNodes.clear();
}
// https://github.com/stackblitz/alien-signals/blob/v2.0.3/src/system.ts#L52
/** Truthy when it reuses the link `sub`'s last pass made: a re-read. */
function link(dep, sub) {
  const prevDep = sub._depsTail;
  if (prevDep !== null && prevDep._dep === dep) return;
  let nextDep = null;
  const isRecomputing = sub._flags & REACTIVE_RECOMPUTING_DEPS;
  if (isRecomputing) {
    nextDep = prevDep !== null ? prevDep._nextDep : sub._deps;
    if (nextDep !== null && nextDep._dep === dep) {
      nextDep._gen = sub._depGen;
      return (sub._depsTail = nextDep);
    }
  }
  // A link stamped with the current pass generation was created or reused
  // in-order during this recompute, i.e. it already sits in the validated
  // [deps.._depsTail] prefix — the O(1) equivalent of scanning the dep list
  // (the old alien-signals `isValidLink` walk, O(n²) when a computation
  // re-reads earlier deps non-consecutively, e.g. store leaf reads).
  const prevSub = dep._subsTail;
  if (prevSub !== null && prevSub._sub === sub && (!isRecomputing || prevSub._gen === sub._depGen))
    return;
  const newLink =
    (sub._depsTail =
    dep._subsTail =
      {
        _dep: dep,
        _sub: sub,
        _nextDep: nextDep,
        _prevSub: prevSub,
        _nextSub: null,
        _gen: sub._depGen
      });
  if (prevDep !== null) prevDep._nextDep = newLink;
  else sub._deps = newLink;
  if (prevSub !== null) prevSub._nextSub = newLink;
  else dep._subs = newLink;
  // New subscriber edge: staged-rewrite skips (§12d) must not miss it.
  bumpNotifyEpoch();
}

// One Set, created lazily, holds every pending source: a single slot
// promoted to a Set had dual state that stranded pending entries (#2893).
function addPendingSource(el, source) {
  if (el._x?._pendingSources?.has(source)) return false;
  (ext(el)._pendingSources ??= new NewSet()).add(source);
  return true;
}
function removePendingSource(el, source) {
  const sources = el._x?._pendingSources;
  if (!sources?.delete(source)) return false;
  if (!sources.size) el._x._pendingSources = undefined;
  return true;
}
function clearInFlight(el) {
  if (el._x !== null) el._x._inFlight = null;
}
function clearPendingSources(el) {
  // Node-owned and never shared: dropping the reference is the clear.
  if (el._x !== null) el._x._pendingSources = undefined;
}
// A rejection-pending clears only through the settle walk over the source's
// subscribers, so it can retry only when a live dep is the source or carries
// it. A disposed dep is unlinked and holds nothing (F-3, #5003). A source that
// has settled has no walk to come: linked directly, it holds nothing (F-2).
function retryReaches(el, source) {
  for (let d = el._deps; d; d = d._nextDep) {
    const dep = d._dep;
    if (
      !(dep._flags & REACTIVE_DISPOSED) &&
      (dep === source ? dep._statusFlags & STATUS_PENDING : dep._x?._pendingSources?.has(source))
    )
      return true;
  }
  return false;
}
/** Park a loading-window node on an unready source: registered for the
 * source's settle, with no visible pending and no propagation. */
function parkLoadingWindow(el, e) {
  ext(el)._blocked = true;
  if (e.source) addPendingSource(el, e.source);
  // A settled error stays the answer: overwriting it here would leak a
  // NotReadyError from an invisible park (#2989).
  if (!(el._statusFlags & STATUS_ERROR)) setPendingError(el, e.source, e);
}
function setPendingError(el, source, error) {
  if (!source) {
    if (el._x !== null) el._x._error = null;
    return;
  }
  const current = error instanceof NotReadyError && error.source === source ? error : el._x?._error;
  ext(el)._error =
    current instanceof NotReadyError && current.source === source
      ? current
      : new NotReadyError(source);
}
function forEachDependent(el, fn) {
  for (let s = el._subs; s !== null; s = s._nextSub) fn(s._sub, s);
}
/** Depth first. `fn` sees every arrival (`seen` on a diamond's second);
 * returning false leaves the node unmarked so a later branch may walk it. */
function walkDependents(el, fn) {
  const visited = new NewSet();
  const visit = node => {
    const seen = visited.has(node);
    if (fn(node, seen) !== false && !seen) {
      visited.add(node);
      forEachDependent(node, visit);
    }
  };
  forEachDependent(el, visit);
}
// A lazy node left with no subscribers while pending is kept alive by its
// flight, so whatever clears that pending must release it (#2934). A node
// already queued releases at its pass's tail, or it would miss the landing
// it was queued for (D-22, #4474).
function releaseIfSettledUnobserved(node) {
  if (
    node._fn &&
    node._config & CONFIG_AUTO_DISPOSE &&
    !node._subs &&
    !(node._flags & REACTIVE_ZOMBIE) &&
    !(node._statusFlags & STATUS_PENDING)
  )
    node._flags & REACTIVE_IN_HEAP ? (node._flags |= REACTIVE_RELEASE_AFTER) : unobserved(node);
}
// After an error propagates, release stranded lazy nodes. Collect first so
// `unobserved()` never unlinks under the walk.
function releaseSettledDependents(el) {
  let candidates;
  walkDependents(el, (node, seen) => {
    if (!seen && !node._subs && node._config & CONFIG_AUTO_DISPOSE) (candidates ??= []).push(node);
  });
  if (candidates) for (const node of candidates) releaseIfSettledUnobserved(node);
}
/** An error a held node's pass found is an answer of that frame, staged like
 * a value and published at the commit (D-41). Not for a lane pass, nor a
 * node born held. Returns whether it was staged. */
function stageError(el, error, inPass) {
  // Staged even before the flush joins a hold: the seam decides, as for a
  // value (#4109, S-38). Not a projection's derive: its family never sees the
  // commit's publish (#4189).
  if (
    !(
      el._config & CONFIG_HELD ||
      (inPass && el._flags & REACTIVE_STAGED_READ && globalQueue._running && !el._fam)
    ) ||
    passLane
  )
    return false;
  // An error ends the wait (D-41); a shown error stays until the commit.
  clearPendingSources(el);
  if (el._x) el._x._blocked = false;
  if (!((el._statusFlags &= STATUS_ERROR | STATUS_UNINITIALIZED) & STATUS_ERROR))
    setPendingError(el);
  el._pendingValue = error instanceof StatusError ? error : new StatusError(el, error);
  el._config |= CONFIG_STAGED | CONFIG_STAGED_ERROR;
  // Readers derive from the staging; verdict readers the walk wakes need a flush (#4106).
  settlePendingSource(el);
  schedule();
  return true;
}
function settleErroredDependents(el, error) {
  let scheduled = false;
  walkDependents(el, (node, seen) => {
    if (!seen && node._x?._error === error) {
      enqueueSub(node);
      scheduled = true;
    }
  });
  if (scheduled) schedule();
}
// Retire `source` from pending along the dependents of `el`. With a distinct
// `source`, `el` recovered without it, so dependents with another path to it
// stay pending.
function settlePendingSource(el, source = el) {
  // Dev invariant: a settle needs an answer. An uninitialized root with no
  // staged value and no error announces a settle that never happened, and its
  // readers would wake into nothing (#3648).
  {
    const sources = el._x?._pendingSources;
    if (
      el._statusFlags & STATUS_UNINITIALIZED &&
      el._pendingValue === NOT_PENDING &&
      !el._x?._error &&
      !(sources?.size && (sources.size > 1 || !sources.has(el)))
    ) {
      // Reported, not thrown: promise machinery has no caller to surface it (#3648).
      reportDiagnostic(
        emitDiagnostic(
          {
            code: "SETTLE_WALK_UNINITIALIZED_SOURCE",
            kind: "lifecycle",
            severity: "error",
            message:
              "[SETTLE_WALK_UNINITIALIZED_SOURCE] settlePendingSource was called on a source that " +
              "never produced a value. Settling parked readers requires truth to reveal — an " +
              "uninitialized source waking its dependents serves them its initial face instead of " +
              "settled data.",
            ownerId: el.id,
            ownerName: el._name
          },
          el
        )
      );
    }
  }
  // A superseded re-park may keep an abandoned self entry; retire it with its copies.
  removePendingSource(el, source);
  let scheduled = false;
  let released;
  walkDependents(el, (node, seen) => {
    if (
      seen ||
      // Another dependency may still carry the source. Not marked visited: that
      // dependency may be a later branch of this walk.
      (source !== el && retryReaches(node, source)) ||
      !removePendingSource(node, source)
    )
      // A verdict reader holds no pending of its own: the source settling is its
      // verdict changing, so it re-runs (F-9). A derivation of a staged error
      // carries it (D-41).
      return (
        holdWorkHook &&
          (node._config & CONFIG_VERDICT ? enqueueSub(node) : stageMarkHook(node, el)),
        false
      );
    node._time = clock;
    const remaining = node._x?._pendingSources?.values().next().value;
    // An errored loading-window park keeps its error through the settle (#2989).
    const errored = node._statusFlags & STATUS_ERROR;
    if (remaining) {
      if (!errored) setPendingError(node, remaining);
    } else {
      node._statusFlags &= ~STATUS_PENDING;
      if (!errored) setPendingError(node);
      // A held reader's re-pass over the landing is its hold's work (D-1); never a
      // lane's (#4440). A source still in flight has not landed: a reader that
      // stopped carrying it joins nothing (F-2).
      if (node._config & CONFIG_HELD && !(source._statusFlags & STATUS_PENDING))
        joinFuture(txOf(node));
      if (node._x?._blocked) {
        enqueueSub(node);
        scheduled = true;
      }
      // A derived store's readers wait on the derive itself, so they settle with it (F-9).
      else if (storeParkHook) node._fam?.st();
      if (node._x !== null) node._x._blocked = false;
      // Settled with no subscribers: released after the walk (#2934).
      if (!node._subs && node._config & CONFIG_AUTO_DISPOSE) (released ??= []).push(node);
      // A chain's single arrival marks what a diamond's second does: graph shape
      // is not observable (D-41, #4562).
      stageMarkHook?.(node, el);
    }
  });
  // Queued nodes release at their pass's tail (#4474).
  if (released) for (const node of released) releaseIfSettledUnobserved(node);
  if (scheduled) schedule();
}
function isThenable(value) {
  return value != null && typeof value === "object" && typeof value.then === "function";
}
function releaseFlightTeardown(el) {
  const teardown = el._x?._flightTeardown;
  if (teardown != null) {
    el._x._flightTeardown = null;
    teardown();
  }
}
/** Internal: run `fn` when the current pass's flight is superseded, with the
 * iterator close, rather than when the replacing pass commits. A no-op
 * outside a computed's own pass. */
function onSupersede(fn) {
  const el = context;
  if (el === null || el._fn === undefined || !(el._flags & REACTIVE_RECOMPUTING_DEPS)) return;
  const fresh = el._x === null;
  const x = ext(el);
  // Allocating `_x` mid-run turns `_inFlight` from undefined to null, which
  // `recompute` would read as a flight this body registered.
  if (fresh) x._inFlight = undefined;
  const prev = x._flightTeardown;
  x._flightTeardown =
    prev === null
      ? fn
      : () => {
          try {
            prev();
          } finally {
            fn();
          }
        };
}
/** A flight's landing through a setter (bound by store/projection.ts). A throw
 * from `commit` is the flight's error (`fail`, false returned). */
function landBySetter(el, value, commit, wasUninitialized, fail) {
  // Observe: what a setter landing moves on the node, to say whether it committed.
  const observed = attrHooks !== null;
  const before = observed ? el._value : undefined;
  const beforeTime = observed ? el._time : 0;
  try {
    commit(value);
  } catch (error) {
    fail(error);
    return false;
  }
  // Unheld, a store derive's landing clears the shown error and published-inputs
  // mark now (#4269); held, at the commit.
  if (!(el._config & CONFIG_HELD)) {
    el._x._shownError = null;
    el._config &= ~CONFIG_INPUTS_PUBLISHED;
  }
  if (wasUninitialized) landStatus(el, true);
  // Observe: a setter landing bypasses `setSignal`, so the flight closes here
  // (#3947). Outside the try (attribution-hooks.ts).
  if (observed)
    attrHooks.asyncLanded(
      el,
      value,
      el._value !== before || el._time !== beforeTime || el._pendingValue === value
    );
  return true;
}
function handleAsync(
  el,
  result,
  // A store derive's landing (`landBySetter`); a callback, so a build
  // without stores retains none of the arm (bundle size).
  setter,
  // `pending`: a NotReady rejection, not an error. A projection tells its leaves' readers.
  onError
) {
  let iterator = false;
  let thenable = false;
  if (typeof result === "object" && result !== null) {
    untrack(() => {
      iterator = result[Symbol.asyncIterator];
      thenable = !iterator && isThenable(result);
    });
  }
  if (!thenable && !iterator) {
    if (el._x !== null) {
      el._x._inFlight = null;
      if (holdWorkHook) el._config &= ~CONFIG_ASYNC;
    }
    el._loading = false;
    return result;
  }
  // Dev: a `sync: true` node that returns async is diagnosed; prod skips this call.
  if (el._config & CONFIG_SYNC) {
    const message =
      `[SYNC_NODE_RECEIVED_ASYNC] A computed/effect created with \`sync: true\` returned ` +
      `${thenable ? "a Promise" : "an AsyncIterable"}. The value would be stored as-is and ` +
      `never awaited in production; remove \`sync: true\` to use async-aware behavior, or ` +
      `unwrap the value before returning.`;
    emitDiagnostic({
      code: "SYNC_NODE_RECEIVED_ASYNC",
      kind: "lifecycle",
      severity: "error",
      message,
      ownerId: el.id,
      ownerName: el._name
    });
    throw new Error(message);
  }
  // Iterator teardown already ran in `recompute`'s supersede release (#3122).
  // A registration path outside `recompute` must release it here.
  ext(el)._inFlight = result;
  // Upkept only with the verdict engine (`holdWorkHook`); see `holdWork` (D-25).
  if (holdWorkHook) el._config |= CONFIG_ASYNC;
  // Provenance (D-34): the question this flight answers.
  el._x._q = question || MAINLINE_QUESTION;
  // The asking pass read every input, so earlier propagated pending does not
  // describe this answer; left, it would hold the node past it (#3373).
  el._x._pendingSources = undefined;
  // Observe: announced once per flight shape, while the causing pass is on the
  // stack. Not inside a try (attribution-hooks.ts).
  if (attrHooks !== null) attrHooks.flightStart(el, result);
  let syncValue;
  const handleError = error => {
    if (el._x?._inFlight !== result) return;
    // Dev: a rejection continues the read's throw; it is not a catch (D-42).
    caughtReadFlightSettled(el, result, false);
    let stillPending = error instanceof NotReadyError;
    // Dev: no edge means a first read after `await`, which can never settle and
    // would wedge the boundary (#2987). Checked before the loading-window park.
    if (stillPending && !retryReaches(el, error.source)) {
      stillPending = false;
      error = new Error(
        "Read of an unresolved async source after an `await`. Reads inside async " +
          "computations only register as dependencies before the first `await`; a source " +
          "first read after it cannot retry when it settles. Read it before the first " +
          "`await` (or restructure so the value is an input)."
      );
    }
    if (stillPending && el._loading) {
      // Loading window: keep serving commit #0; release the flight so the error
      // retry can ask again.
      clearInFlight(el);
      parkLoadingWindow(el, error);
      el._time = clock;
      return;
    }
    // A held node's error is staged (D-41), a derived store's derive's too (#4489).
    if (stillPending || !stageError(el, error)) {
      notifyStatus(el, stillPending ? STATUS_PENDING : STATUS_ERROR, error);
      // Published now, the error replaces the shown frame as a value's commit would.
      if (!stillPending) disposeChildren(el, false, true);
    }
    // A NotReady rejection lands into another pending source: hand over the self entry.
    if (stillPending) settlePendingSource(el);
    el._time = clock;
    // Release lazy dependents the error settled (#2934).
    if (!stillPending) releaseSettledDependents(el);
    onError?.(error, stillPending);
  };
  const asyncWrite = (value, then) => {
    if (el._x?._inFlight !== result) return;
    // Dev: a value after a throw into this pass means the body caught it (D-42).
    caughtReadFlightSettled(el, result, true);
    // A newer write re-runs the node. A first answer is still a produced value:
    // it lands before the re-ask (D-22, F-9). Only the verdict engine can tell.
    if (el._flags & REACTIVE_DIRTY && !(holdWorkHook && el._statusFlags & STATUS_UNINITIALIZED))
      return;
    // A write's flush is queued: it carries this landing (#4495).
    const carried = holdWorkHook && pendingNodes.some(n => n._pendingValue !== NOT_PENDING);
    const wasUninitialized = el._statusFlags & STATUS_UNINITIALIZED;
    landStatus(el);
    const observed = attrHooks !== null;
    if (setter) {
      // False: the derive's commit threw, taken as the flight's error.
      if (!setter(value, wasUninitialized, handleError)) return;
    } else if (observed) {
      const changed = landThroughSetSignal(el, value);
      attrHooks.asyncLanded(el, value, changed);
    } else {
      // A held node stages the answer into its hold (`setSignal`).
      setSignal(el, () => value);
    }
    // The loading window closes when the answer is observable: at the commit if
    // staged, so no one-frame pulse leaks (#2990).
    if (el._pendingValue === NOT_PENDING) {
      el._loading = false;
      // Trim the old dependency tail once published; a staging trims at its commit (#3410).
      trimStaleDeps(el);
    }
    settlePendingSource(el);
    schedule();
    // A write's flush already queued carries this landing with the others ahead
    // of it (F-10, D-22). Flushing now would re-ask before their reactions ran (#4495).
    if (then || !carried) flush();
    then?.();
  };
  // A pending node's flight counts as an observer, so settling runs the
  // last-one-out check. Returns whether it released, so a stream stops (#2935).
  const settleAutodispose = () => {
    if (el._config & CONFIG_AUTO_DISPOSE && !el._subs && !(el._statusFlags & STATUS_PENDING)) {
      unobserved(el);
      return true;
    }
    return false;
  };
  // Consume an AsyncIterable as this flight's values. Live (called inside the
  // read): a sync first yield is stashed for the caller. Deferred (a thenable
  // resolved to an iterable later): every step writes through `asyncWrite`
  // and close registers on `owner`. Returns whether a sync answer landed.
  const consumeIterator = (source, owner) => {
    const it = source[Symbol.asyncIterator]();
    let hadValue = false;
    let completed = false;
    let initialRead = !owner;
    const close = () => {
      if (completed) return;
      completed = true;
      try {
        const returned = it.return?.();
        if (isThenable(returned)) returned.then(undefined, () => {});
      } catch {}
    };
    cleanup(close, owner);
    // Supersede stops the stream at once (#3122). Assigned, not chained, for bytes:
    // it replaces an earlier `onSupersede` teardown, which falls back to owner cleanup.
    ext(el)._flightTeardown = close;
    // Release before each pull, or an unobserved stream pumps forever (#2935).
    // A protocol throw settles as this flight's error, not a rejection (#4112).
    const iterateOrRelease = () => {
      try {
        if (!settleAutodispose()) iterate();
      } catch (e) {
        handleError(e);
      }
    };
    const iterate = () => {
      let syncResult,
        syncError,
        resolved = false,
        rejected = false,
        isSync = true;
      // Like `for await`, accept a bare IteratorResult as a settled step.
      const step = it.next();
      const settled = isThenable(step) ? step : { then: onSettle => void onSettle(step) };
      settled.then(
        r => {
          // Only the initial drain has a caller for the stash; later sync steps write
          // through, or the value is dropped.
          if (isSync && initialRead) {
            syncResult = r;
            resolved = true;
            if (r.done) completed = true;
          } else if (el._x?._inFlight !== result) {
            return;
          } else if (!r.done) {
            hadValue = true;
            asyncWrite(r.value, iterateOrRelease);
          } else {
            completed = true;
            if (hadValue) {
              schedule();
              flush();
            } else {
              asyncWrite(undefined);
            }
            settleAutodispose();
          }
        },
        e => {
          if (isSync && initialRead) {
            syncError = e;
            rejected = true;
          } else if (el._x?._inFlight === result) {
            completed = true;
            handleError(e);
            settleAutodispose();
          }
        }
      );
      isSync = false;
      if (rejected) {
        completed = true;
        handleError(syncError);
        if (initialRead) throw syncError;
        return true;
      }
      if (resolved && !syncResult.done) {
        syncValue = syncResult.value;
        hadValue = true;
        return iterate();
      }
      return resolved && syncResult.done;
    };
    const immediatelyDone = iterate();
    // Later calls run from `asyncWrite`, where a rethrow would be unhandled.
    initialRead = false;
    return hadValue || immediatelyDone;
  };
  // Whether a live drain landed synchronously; null when none ran.
  let liveLanded = null;
  // A thenable that resolves to an AsyncIterable is consumed as the stream.
  // One level: A+ `then` already collapses nested thenables.
  const flattenIfIterable = (value, owner) => {
    let innerIterator = false;
    if (typeof value === "object" && value !== null) {
      untrack(() => {
        innerIterator = value[Symbol.asyncIterator];
      });
    }
    if (!innerIterator) return false;
    syncValue = undefined;
    const landed = consumeIterator(value, owner);
    if (!owner) liveLanded = landed;
    return true;
  };
  if (thenable) {
    let resolved = false,
      rejected = false,
      syncError,
      isSync = true;
    watchAsyncTail(el, result).then(
      v => {
        if (isSync) {
          syncValue = v;
          resolved = true;
        } else {
          // The first yield clears pending as a resolution would. A disposed node never
          // starts a pump. The ambient owner is gone, so close registers on `el`; a
          // protocol throw settles as this flight's error (#4112).
          try {
            if (
              el._x?._inFlight === result &&
              !(el._flags & REACTIVE_DISPOSED) &&
              flattenIfIterable(v, el)
            )
              return;
            asyncWrite(v);
          } catch (e) {
            handleError(e);
          }
          settleAutodispose();
        }
      },
      e => {
        if (isSync) {
          syncError = e;
          rejected = true;
        } else {
          handleError(e);
          settleAutodispose();
        }
      }
    );
    isSync = false;
    if (rejected) {
      // Settle as a rejection does, then unwind so the read is not `undefined`.
      handleError(syncError);
      throw syncError;
    } else if (!resolved) {
      // Not landed: the tail suspends, or a loading window serves commit #0.
      liveLanded = false;
    } else if (!flattenIfIterable(syncValue)) {
      el._loading = false;
    }
  }
  if (iterator) flattenIfIterable(result);
  if (liveLanded !== null) {
    if (!liveLanded) {
      if (el._loading) return el._value;
      throw new NotReadyError(context);
    }
    el._loading = false;
  }
  // Dev: a sync answer landed a value (D-42).
  caughtReadFlightSettled(el, result, true);
  return syncValue;
}
function clearStatus(el, clearUninitialized = false) {
  if (el._x?._pendingSources) clearPendingSources(el);
  if (el._x?._blocked) el._x._blocked = false;
  el._statusFlags = clearUninitialized ? 0 : el._statusFlags & STATUS_UNINITIALIZED;
  if (el._x?._error) setPendingError(el);
  const notify = statusNotifierOf(el);
  if (notify) notify.call(el);
}
/** A landing answers the node's own question: it retires the self entry, not
 * pending its sources propagated. With another source still pending the
 * node stays pending on it (#3373, #3376). `_blocked` clears, so an input
 * settling unchanged does not ask again; `_error` moves off self. */
function landStatus(el, clearUninitialized = false) {
  const sources = el._x?._pendingSources;
  if (sources && (sources.delete(el), sources.size)) {
    el._x._blocked = false;
    if (clearUninitialized) el._statusFlags = STATUS_PENDING;
    setPendingError(el, sources.values().next().value);
  } else clearStatus(el, clearUninitialized);
}
function notifyStatus(el, status, error) {
  if (
    status === STATUS_ERROR &&
    !(error instanceof StatusError) &&
    !(error instanceof NotReadyError)
  )
    error = new StatusError(el, error);
  const pendingSource = status === STATUS_PENDING && error instanceof NotReadyError && error.source;
  if (pendingSource) {
    // A re-ask of a shown error keeps it on display (D-41).
    if (el._statusFlags & STATUS_ERROR) el._x._shownError = el._x._error;
    addPendingSource(el, pendingSource);
  } else clearPendingSources(el);
  // An error answering a node with no value keeps it uninitialized; a re-ask
  // is uninitialized again; a write gives it a value.
  el._statusFlags =
    status |
    (status === STATUS_ERROR
      ? el._statusFlags & (STATUS_UNINITIALIZED | STATUS_NO_VALUE)
        ? STATUS_NO_VALUE
        : 0
      : status === STATUS_PENDING && el._statusFlags & STATUS_NO_VALUE
        ? STATUS_UNINITIALIZED
        : el._statusFlags & STATUS_UNINITIALIZED);
  // Readers park on the propagated source.
  if (pendingSource) setPendingError(el, pendingSource, error);
  else ext(el)._error = error;
  // A guess is verdict-inert: its own flight is the hold's to wait on, not its
  // readers' (F-9, F-11). An input changed outside the guess's hold asks a
  // new question, held with that hold (D-34, #4336); a `refresh()` does not.
  if (guessing && status === STATUS_PENDING && el._config & CONFIG_GUESS) {
    if (passLane === null) joinFuture(el._flags & REACTIVE_REASK ? null : txOf(el)._parent);
    return;
  }
  const elNotify = statusNotifierOf(el);
  if (elNotify) {
    elNotify.call(el);
    return;
  }
  propagateStatus(el, status, error);
}
/** The dependents' half of `notifyStatus`. Also used alone for a store
 * family's leaves, which are not the derive's dependents. */
function propagateStatus(el, status, error) {
  const pendingSource = status === STATUS_PENDING && error instanceof NotReadyError && error.source;
  forEachDependent(el, (sub, link) => {
    sub._time = clock;
    // Re-derive instead of marking when:
    // - a pending mark arrives on a kept-tail link: it is a question for the next
    //   pass, not this one (#3494);
    // - the subscriber is a verdict reader or a boundary's output: its pass
    //   decides what the change means (D-23, D-25);
    // - it carries a staged error: the source asks again (#4109).
    // Not an async memo D-25 makes the hold's work: it waits like any
    // derivation and asks once, at the landing (F-9, #4397).
    if (
      sub._config & CONFIG_REDERIVE ||
      (holdWorkHook &&
        (sub._config & CONFIG_VERDICT || (pendingSource && sub._config & CONFIG_STAGED_ERROR)) &&
        !(pendingSource && holdWorkHook(sub, el))) ||
      (status === STATUS_PENDING && link._gen !== sub._depGen)
    ) {
      // A re-derived subscriber no longer waits on a settled source: retire the
      // entry there and on its dependents, or the subscriber stays pending on a
      // source that never settles again.
      if (status !== STATUS_PENDING && removePendingSource(sub, el)) settlePendingSource(sub, el);
      enqueueSub(sub);
      schedule();
      return;
    }
    // Already pending on this source: nothing re-notifies, but the re-asking
    // write is held with the flight the observer waits on (D-9).
    if (pendingSource && sub._x?._pendingSources?.has(pendingSource)) {
      if (
        passLane === null &&
        (sub._config & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD &&
        globalQueue._running &&
        !freshHook?.(sub, pendingSource._time === clock)
      )
        joinFuture(txOf(sub));
      return;
    }
    if (
      pendingSource ||
      (status !== STATUS_PENDING && (sub._x?._error !== error || sub._x?._pendingSources))
    ) {
      // Queued for the commit sweep, with the lane when the pass is lane work.
      if (laneStageHook && passLane !== null) laneStageHook(sub, passLane, true, el);
      else {
        queuePendingNode(sub);
        // Pending enters a held memo's hold without re-running it, so the write that
        // started the flight is held with it (D-9). A render effect's membership is
        // its pass's.
        if (
          status === STATUS_PENDING &&
          (sub._config & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD &&
          !sub._type
        )
          if (guessing && sub._statusFlags & STATUS_PENDING && reasks(sub))
            // A hold waiting only on re-asks holds nothing new (D-34, F-9): the memo
            // leaves it and publishes with the write's frame. Only guesses can keep such a
            // hold open (#4828).
            unhold(sub);
          else joinFuture(txOf(sub));
      }
      notifyStatus(sub, status, error);
      // A derived store's derive is marked, not re-run: its family walks the leaves.
      sub._fam?.st(status, error);
    }
  });
}

// Truth reads (`until`, `refresh`; CONFIG_AUTHORITATIVE) over lane nodes,
// and the two views of a lane node every reader is served from. Side-effect
// free and apart from lanes.ts so a truth reader alone does not retain the
// lane engine: its reads reach these only through `laneRead`, which exists
// once something installs lanes (`createOptimistic`, verdicts).
/** A lane's record of a node beside the node's two slots: its guess until
 * the lane reveals it (its staging is a truth's), and after the reveal the
 * value the screen showed before (a guess reverts to it at the lane's end;
 * once shown (CONFIG_SHOWN). Dropped when the node leaves its lane. */
const covers = /* @__PURE__ */ new WeakMap();
/** A lane node's latest value. A guess: the guess — the lane's record of it
 * until the reveal (`under`), the screen's value after. A derivation: its
 * staging under the lane (a lane pass's answer, until the reveal), else
 * what the screen shows — NOT_PENDING for a node with neither (born in the
 * lane, uninitialized: its reads throw NotReady, D-22; #4363). */
function latestOf(el) {
  if (el._config & CONFIG_GUESS) return el._x._shown ? el._value : covers.get(el);
  return el._pendingValue !== NOT_PENDING
    ? el._pendingValue
    : el._statusFlags & STATUS_UNINITIALIZED
      ? NOT_PENDING
      : el._value;
}
/** The value a guess covered: the committed value until the lane reveals
 * the guess, the lane's record of it after (`under`) */
function covered(n) {
  return n._x._shown ? covers.get(n) : n._value;
}
/** The truth beneath a guess: a landing staged there — the parent's (lanes.ts
 * `landBeneath`), or the same tick's — else the value it covered. */
function underOf(n) {
  return n._pendingValue !== NOT_PENDING ? n._pendingValue : covered(n);
}
/** Truth readers (`until`, `refresh`; CONFIG_AUTHORITATIVE) — pay-for-use:
 * the first one installs these on GlobalQueue (signals.ts `watch`).
 *
 * A truth reader's read of a lane node: the lane's value when it is the
 * truth (#3943), else the truth beneath a guess. */
function truthRead(el) {
  if (truth(el)) return latestOf(el);
  if (!(el._config & CONFIG_GUESS)) throw new NotReadyError(null);
  return underOf(el);
}
/** A guess confirmed: its lane's truth readers re-read — of the lane's
 * derivations too, which may be the truth now (#3943). */
function truthConfirmed(l) {
  for (let i = 0; i < l._nodes.length; i++)
    for (let s = l._nodes[i]._subs; s !== null; s = s._nextSub)
      if (s._sub._config & CONFIG_AUTHORITATIVE) enqueueSub(s._sub);
}
/** A lane node's value is the truth when every guess beneath it is
 * confirmed — its source landed a value equal to the guess beneath it — and
 * nothing beneath it is held in a transaction: same inputs, same value. */
function truth(n) {
  if (n._config & CONFIG_GUESS)
    return n._pendingValue !== NOT_PENDING && n._equals && n._equals(latestOf(n), n._pendingValue);
  if (n._config & CONFIG_OVERRIDE) {
    for (let d = n._deps; d; d = d._nextDep) if (!truth(d._dep)) return;
    return 1;
  }
  return n._pendingValue === NOT_PENDING ? 1 : undefined;
}

// Lanes (plan sec. 28 replay, 2026-10-02; maintainer rulings 2026-10-01/02). A lane
// is "a new base of a transition": the sub-frame an optimistic write opens
// in the transition of the frame it is made in. It sees the screen plus its
// own guesses (plan sec. 19), breaks out of the parent's hold — its effects run now —
// and holds itself if its own derivations hit async (and, blocked, holds its
// parent). It ends when the parent lands: a guess reverts to the truth it
// covered, or lands as the truth that superseded it. A frame that does not
// park has nothing to be optimistic over: the write is void.
//
// Two places per node, as for any held node: `_value` is what the screen
// shows, `_pendingValue` the staging of the transaction holding it — for a
// lane's node, the lane's: a written guess or a lane pass's derivation,
// until the lane reveals (`laneSeam`), when it moves into `_value` and the
// lane records what it covered (`under`). A truth landing beneath a guess is
// the lane's parent's, kept on the lane too (`under`, CONFIG_HELD) for the
// parent's landing to commit. The lane carries the state machine: `_shown`
// once it has revealed. A direct read sees the lane's latest throughout
// (A17); an authoritative reader (`until`) sees the truth beneath it (Q3,
// 2026-10-02: "optimistic overrides are intentionally not part of
// authoritative truth").
//
// The seat of a pass is its node's (core.ts `recompute`): a node carrying a
// lane's derived value runs as the lane's whoever dirtied it; a derivation's
// tracked read of a lane's value moves its pass into the lane (`enterLane`);
// a leaf's never does — it reads the screen, a stale reader of a lane that
// has not shown (#3460). Lane work's values are the lane's staging, revealed
// at the next seam; its runs go to the lane's queues.
//
// This module is the lane engine, installed on `GlobalQueue` when something
// imports it (`createOptimistic`, or verdict.ts for its verdict lanes). The
// core keeps only bit tests and `passLane`: a program with no optimism
// carries none of this.
/** Live lanes (parented). Scanned at every seam while non-empty. */
const lanes = [];
/** A pass left a shown lane (`laneStage`): the node's notify stamp
 * (`_notifiedAt`, never negative once it notifies — `insertSubs` overwrites
 * it, so a pass that does notify clears the mark). The seam wakes its
 * readers (#3892). */
const LEFT_SHOWN = -2;
function newLane(parent) {
  const l = newTransaction(true, parent);
  lanes.push(l);
  return l;
}
/** `v` is lane `l`'s value of `n` from now — a guess written or re-asked, a
 * store's composed row, a guess coming back: on screen at once once the
 * lane has revealed (what it covers kept for the lane's end, `cover`),
 * staged for the reveal before. */
function setLaneValue(n, l, v) {
  if (l._shown || n._x._shown) {
    cover(n);
    n._value = v;
    n._x._shown = true;
    if (n._statusFlags & STATUS_UNINITIALIZED) n._statusFlags &= ~STATUS_UNINITIALIZED;
  } else if (n._config & CONFIG_GUESS) covers.set(n, v);
  else n._pendingValue = v;
}
/** What the screen shows of a node: NOT_PENDING when nothing (uninitialized). */
function screenOf(el) {
  return el._statusFlags & STATUS_UNINITIALIZED
    ? NOT_PENDING
    : guessScreen
      ? guessScreen(el)
      : el._value;
}
/** The guess layer's screen of a node (guesses.ts): a shown guess re-asked
 * keeps the shown one until its lane's next reveal (#4775); `revealed`
 * forgets it. */
let guessScreen = null;
/** The lane's record of the value the screen showed `n` before the lane
 * revealed it (a guess reverts to it, `covered`) — kept once. */
function cover(n) {
  if (!covers.has(n)) covers.set(n, n._value);
}
/** A truth landed beneath a guess: the parent's staging, held for its
 * landing to commit (CONFIG_HELD) — an older question's answer (A18), or the
 * guess's confirmation (A24). The guess stays the value. */
function landBeneath(n, truth) {
  n._pendingValue = truth;
  n._config |= CONFIG_HELD;
}
/** This module's copy of scheduler.ts `guessing`: its readers here stay
 * foldable when a bundler splits lanes.ts from scheduler.ts into separate
 * chunks (a cross-chunk binding is not folded; the compiled live page). */
let guessed = false;
/** A guess was written (guesses.ts `optimisticWrite`). */
function armGuessed() {
  guessed = true;
}
// The guess layer (guesses.ts; installed with `createOptimistic` or
// `createOptimisticStore`): the arms only a written guess reaches. `take`
// — a lane takes another action's guess (`laneStage`); `guessWrite` — a
// write landing on a guess (`laneWrite`); `revertLanding` — a revert
// landing gives back the guesses its lane took (`endLanes`). Each site is
// behind CONFIG_GUESS or `guessed`, which nothing but the layer sets: a
// bundle without it folds the sites.
let takeHook = null;
let guessWriteHook = null;
let revertLandingHook = null;
function installGuessLayer(take, write, revert, screen) {
  guessScreen = screen;
  takeHook = take;
  guessWriteHook = write;
  revertLandingHook = revert;
}
/** Two kinds of member, one set (a lane is never a node). Lanes a revert
 * dissolved: the guesses they took come back at their parent's landing
 * (`endLanes`). Shown guesses a correction re-homed into their hold
 * (guesses.ts `supersede`, `dissolveLane`): the slot's value for the screen and
 * untracked reads until that hold lands (D-34), each keeps it open on its
 * own flight as it did while a guess (`lanesBlocked`, #4690); the hold's
 * landing clears them (`endLanes`). */
const marked = /* @__PURE__ */ new WeakSet();
/** A shown guess a correction re-homed into its hold (guesses.ts `supersede`,
 * `dissolveLane`): the screen's and the untracked reads' value until the
 * commit, and `latest`'s (D-34, decision 3 of 2026-11-05, #4689) — the
 * displayed slot's newest value is the guess, the truth its staging. */
function keptGuess(n) {
  return guessed && marked.has(n);
}
/** `n`'s readers are stale readers of `t` (re-derived at its landing). */
function rerun(n, t) {
  for (let s = n._subs; s !== null; s = s._nextSub) staleReader(s._sub, t);
  // A store's guess carrier: the readers of the keys it changed read their
  // leaves (store/optimistic.ts).
  if (n._config & CONFIG_SLOT_NODE) GlobalQueue._slotRerun?.(n, t);
}
/** A guess's own truth is in flight: the node's own refetch — or, a store
 * leaf's, its family's derive (the slot's flight; store/optimistic.ts
 * installs the resolver). A17: the guess stands in for the flight. */
function inFlight(n) {
  return (
    n._statusFlags & STATUS_PENDING ||
    (n._config & CONFIG_SLOT_NODE && GlobalQueue._slotFlight?.(n))
  );
}
/** The running pass read lane `l`'s work as a derivation: it is `l`'s — or,
 * already another lane's, the frame it computes derives from both guesses
 * and cannot show one without the other (#3335, A15 for lanes): the two
 * lanes are linked — one reveal unit, each blocked while any is — while
 * their lifetimes stay their own (#2912: a guess reverts with its own
 * action, not with the one it shares a reader with). (Lanes never nest: a
 * lane is never the flush's transaction, D-37, so no guess is parented to
 * one.) */
function enterLane(l, c) {
  const f = (c._flags |= REACTIVE_LANE_READ),
    p = passLane;
  if (p === null) {
    const verdict = isVerdict(l);
    // A verdict lane's value (a memo of `latest` / `isPending`, F-6) read by
    // a derivation that already read the frame's staging or joined its
    // hold, or by any pass a pass that did is creating: the verdict read
    // does not break it out — it stays the frame's, as the same read inline
    // does (verdict.ts `markVerdictReader`, `route`; D-25, D-26). An
    // effect's own reads do not count (its `_type`, 1–3, carries neither bit).
    // Nor by a member of the frame a held pass of that lane's hold built,
    // on any later run (CONFIG_HELD_FRAME, `bornInto`): born in the hold's
    // world, it is the hold's own work and its run is the frame's, at the
    // landing (D-25, D-27) — as `route` keeps it for the inline `latest()`.
    // Nor by a pass that re-entered the hold holding its node at its head
    // (core.ts `recompute`: held, with no verdict posture — an async memo's
    // was dropped there — not a frame, not lagging, not a lane's node), when
    // `l` is another hold's verdict lane: re-run by the write its hold
    // holds, the pass is that hold's work and reads plainly in either read
    // order (D-25), as it does reading its hold's staging before `l`'s
    // value. Moved into `l`, it read its own hold's landing as the screen
    // (D-26) and asked a question already asked (#4399: `m1:15` again, over
    // `m0b`'s committed value beside the landing of its next question). Its
    // own hold's verdict lane is its display-ahead read: it enters that one
    // (D-25, #4050).
    let hold;
    if (!(
      verdict &&
      (creatorReads(c, c._type || f) ||
        ((hold = resolveTx(l._parent)),
        (c._config &
          (CONFIG_HELD | CONFIG_VERDICT | CONFIG_FRAME | CONFIG_LAGS | CONFIG_OVERRIDE)) ===
          CONFIG_HELD && txOf(c) !== hold) ||
        (c._config & CONFIG_HELD_FRAME && bornInto(c, hold)))
    )) {
      setPassLane(l);
      // The pass read a held write before its first lane read (it joined the
      // frame's future as mainline would): lane work sees the screen — the
      // seam re-derives it on the committed world next round (`stagedReaders`,
      // the park's repair), as it does a lane pass that read a staging. Not a
      // pass whose held reads were all of nodes born held (no staged read,
      // `stagedRead`): it has no committed world to re-derive on (#3969).
      // (A pass not joined: a held write beside a guess, #4336; a leaf of a
      // verdict lane — its plain reads are the screen's, D-26 `[s(), L()]`.)
      if (f & REACTIVE_STAGED_READ && (guessed || verdict || f & REACTIVE_JOINED)) stagedRead(c);
    }
  } else if (sameLane(p, l)) setPassLane(l);
  else {
    linkLanes(p, l);
    setPassLane(l);
  }
}
/** Is `l` a verdict lane (its parent's `_verdict`)? False for none. */
function isVerdict(l) {
  return l?._parent?._verdict === l;
}
/** One link group (a shared array) for both lanes' groups. */
function linkLanes(a, b) {
  const ga = a._links ?? (a._links = [a]);
  const gb = b._links;
  if (gb === ga) return;
  if (gb === null) {
    ga.push(b);
    b._links = ga;
  } else
    for (let i = 0; i < gb.length; i++) {
      ga.push(gb[i]);
      gb[i]._links = ga;
    }
}
/** A linked lane blocked on its own account blocks the group — `blocked(k)`
 * without the group recursion. (A guess in `k` whose own truth is in flight
 * blocks `k`'s parent, not `k` — nor, through the link, `l`: the guess
 * shows, A17.) */
function linkBlocked(l) {
  const g = l._links;
  if (g !== null)
    for (let i = 0; i < g.length; i++) {
      const k = g[i];
      if (k !== l && (k._open !== 0 || ownFlights(k) || nestedBlocked(k))) return true;
    }
  return false;
}
function unlink(l) {
  const g = l._links;
  if (g === null) return;
  const k = g.indexOf(l);
  if (k !== -1) g.splice(k, 1);
  l._links = null;
}
/** Verdicts (maintainer, 2026-10-01): `isPending` and `latest` are
 * optimistic state the system supplies — a verdict is a guess ("pending,
 * until this lands"; "the proposal is the value") decided at the seam like
 * a user's, and its readers break out of the hold the same way: work of a
 * lane under the holder, shown now, re-derived at the landing, ending with
 * it. Judged at the seam like any lane: blocked while a verdict reader's
 * own derivation is in flight (lane rule 3), shown otherwise. */
function verdictLane(t) {
  return (t._verdict ??= newLane(t));
}
/** A lane pass's node (core.ts `recompute`, before its staging; a pending
 * propagation under a lane pass, async.ts): listed, carrying the lane's
 * value from now (a derivation of a guess — its readers become lane work).
 * Entering the lane supersedes a frame staging the node carried: this
 * pass's answer replaces it, and the frame's hold on it is over. An
 * effect's value slot is private (no lane value). A pass in a lane's seat
 * that read none of the lane's world has left it (false): its result is the
 * frame's — a derivation whose branch no longer reaches the guess — and no
 * hold the lane took over from a transaction stays on it (#3698: lane work
 * never makes its node transaction work; a born-held effect's hold went to
 * the lane). A pass interrupted before it got there (pending, errored) did
 * not leave (A30: it never got there). A first pass under a lane is the
 * lane's (ruling A); a guess is written, not derived — it never leaves this
 * way. `errored`: the pass has no answer — or, a lane pass's pending
 * propagation (async.ts `propagateStatus`), the node gone pending. */
function laneStage(el, l, create, errored) {
  // F-2's producer clause (S-57): a pass this flush dirtied — it read the
  // flush's staging — that ran as a lane's and asked: the flush holds on
  // that flight and reveals with the lane, whatever the async's source (the
  // lane took the only question).
  if (!create && el._statusFlags & STATUS_PENDING && el._flags & REACTIVE_STAGED_READ) {
    // Pending on a flight it waits on as the frame's work (core.ts `read`:
    // it read the flush's staging, then the flight that staging re-asked)
    // with none of the lane's world read: it left the lane, its pending the
    // frame's — a boundary catches it as a mainline pass's (F-4).
    if (!(el._flags & REACTIVE_LANE_READ) && !(el._config & CONFIG_ASYNC)) errored = false;
    else laneWait(el, l);
  }
  if (!(create || errored || el._flags & REACTIVE_LANE_READ || el._config & CONFIG_GUESS)) {
    const x = el._x;
    if (x !== null) {
      const tx = x._transaction;
      if (tx?._lane) {
        // Left a shown lane: this pass's answer is the frame's. While a
        // transaction's flush runs the pass and the lane's frame is still
        // held, what the screen shows stays the screen until the landing
        // (`_value` keeps the lane's value), and the answer — a change
        // against it — reaches the screen with the frame's commit. Otherwise
        // the answer is judged against the committed value the lane covered
        // (`under`), and the seam wakes the readers (#3892; always for a pass
        // outside any transaction's flush, #4367): a return to the committed
        // value notifies nobody itself, and a pass that does notify has
        // woken them (an effect has no comparator, #4557).
        if (
          tx._shown &&
          !(el._statusFlags & STATUS_PENDING) &&
          (!flushTransaction || !blocked(resolveTx(tx._parent)))
        ) {
          if (el._x._shown) el._value = covers.get(el);
          el._notifiedAt = LEFT_SHOWN;
        }
        covers.delete(el);
        x._shown = false;
        x._transaction = null;
      }
      // The lane's staging of it is void, and so is what its readers derived
      // from it in the lane: they re-derive, or a lane run released after
      // this pass shows the void value and a re-derivation equal to their
      // committed one notifies nobody (a Show's condition read the hold's
      // selection through the lane, F-1, F-7, D-34, #4990).
      if (el._pendingValue !== NOT_PENDING) insertSubs(el);
      el._pendingValue = NOT_PENDING;
    }
    el._config &= -1050881;
    // A store derive's pass (a projection's): the leaves it wrote in the
    // lane's seat leave with it, what it wrote this pass the frame's staging
    // (the screen kept until the landing, as above). Left in the lane, they
    // revealed with it ahead of the frame they derive from (F-1, D-33).
    if (guessed)
      for (let i = 0; i < l._nodes.length; i++) {
        const n = l._nodes[i];
        if (n._host?._fam?.node === el && n._x._transaction === l) {
          const v = n._pendingValue;
          laneStage(n, l, false, false);
          if (v !== NOT_PENDING) (queuePendingNode(n), (n._pendingValue = v));
        }
      }
    return false;
  }
  if (!el._type) {
    if (!(el._config & CONFIG_OVERRIDE)) {
      // A hold's staging of it is void in a guess lane: the readers that
      // derived from that staging re-derive, though the lane's value may
      // equal the screen (no change to notify them by) — their held runs
      // carry the void staging, and the hold's landing would apply them over
      // the guess (#4825; as a mainline pass superseding one does, core.ts
      // `recompute`, #4378). A verdict lane's display-ahead pass leaves the
      // frame's question with its readers (S-51).
      if (el._config & CONFIG_HELD && el._pendingValue !== NOT_PENDING && l._parent._verdict !== l)
        insertSubs(el);
      el._pendingValue = NOT_PENDING;
      el._config = (el._config & -1048833) | CONFIG_OVERRIDE;
      ext(el);
    }
    // A written guess whose own pass is the lane's work — its source is a
    // derivation of another guess (a guess over `config().courier` under a
    // country guess) — is confirmed or corrected by the lane: a derivation
    // of the lane from here (its value the lane's, its fate the lane's), no
    // longer a written guess. A pass with no answer (pending, errored)
    // leaves the guess.
    // Nor a pass of another action's guess: the re-ask is this lane's new
    // question, which the guess (over the old one) does not answer — the
    // lane waits for its own answer (F-1, F-6, #4336). (A guess of the
    // lane's own action, D-33's entangled slot, still stands in.)
    // (Read behind `guessed`: a bundle without optimism drops the reads.)
    if (
      guessed &&
      el._config & CONFIG_GUESS &&
      el._x._transaction?._lane &&
      resolveTx(el._x._transaction._parent) !== resolveTx(l._parent)
    )
      takeHook(el, l);
    else if (!errored) el._config &= ~CONFIG_GUESS;
  }
  // A leaf has no lane value to seat it (`recompute`): one left waiting on
  // the lane's flight runs its next pass as the lane's — the landing that
  // wakes it is the lane's re-staging (A31), and the lane holds its render
  // effects until its derivations land (A17; #3766, F5). Not a leaf whose
  // seat is its frame's, reached by a propagation from a node the screen has
  // a value of: one a transaction holds (born held — its frame waits on it,
  // A29), or one whose own mainline pass is running (it pulled the node;
  // `laneRead` decides — its frame holds). Its pending is the frame's: queued
  // for the commit sweep as mainline's (async.ts). A node born in the lane
  // has nothing on screen: its leaf waits as the lane's. A held leaf stays
  // its frame's even when the node reached it has no value yet: a mount born
  // held whose derivation now waits on a lane's flight (#4998, D-2, F-1).
  else if (errored) {
    if (
      errored !== true &&
      (el._config & CONFIG_HELD ||
        (!(errored._statusFlags & STATUS_UNINITIALIZED) &&
          el._flags & REACTIVE_RECOMPUTING_DEPS)) &&
      !el._x?._transaction?._lane
    ) {
      queuePendingNode(el);
      return false;
    }
    el._flags |= REACTIVE_LANE_DIRTY;
  }
  list(el, l);
  return true;
}
/** A lane's end, two ways. `into === null`: the parent landed — the lane's
 * values are the truth now: a guess lands the truth staged beneath it or
 * reverts to the one it covered (`_value`, untouched by the guess); a
 * derivation's latest value commits (its frame with it); what the screen
 * showed and changes notifies. Otherwise the lane dissolves into `into`
 * (the parent, on a correction): its values are void — what it showed stays
 * the screen (`_value` takes it, so the parent's re-derivation compares
 * against what is seen), a truth staged beneath a guess is the parent's to
 * land, and the runs it held (showing the void guess) are dropped: the
 * parent's re-derivation makes the runs that show. */
function dissolveLane(l, into, except) {
  const k = lanes.indexOf(l);
  if (k !== -1) lanes.splice(k, 1);
  unlink(l);
  // Its verdicts end with it (a correction's dissolution reaches here before
  // `endLanes` would): the readers re-derive from the truth's notification.
  const v = l._verdict;
  if (v !== null) {
    l._verdict = null;
    if (lanes.indexOf(v) !== -1) dissolveLane(v, null);
  }
  for (let i = 0; i < l._nodes.length; i++) {
    const n = l._nodes[i];
    const x = n._x;
    // A member disposed since it joined the lane is no reader any more
    // (F-3, D-11): dead, it stays at its last committed value (#3024). Nothing
    // here may queue it: recompute's head wipe would drop REACTIVE_DISPOSED
    // and run it again in a torn-down tree (#4613).
    if (n === except || x._transaction !== l || n._flags & REACTIVE_DISPOSED) continue;
    const effect = n._type;
    const guess = n._config & CONFIG_GUESS;
    // What the screen shows of it.
    const shown = n._value;
    // A guess's truth: a landing held beneath it, else the value it covered.
    const truth = guess ? underOf(n) : NOT_PENDING;
    n._config &= -10241;
    n._x._shown = false;
    covers.delete(n);
    if (into === null) {
      // The parent landed: a guess lands the truth beneath it or reverts to
      // the one it covered; a derivation's latest (its staging, else what it
      // shows) commits, its frame with it; what the screen showed and
      // changes notifies.
      // A derivation the revert re-derives (dirtied by the guess's
      // notification — listed before it), or one still in flight, shows the
      // lane's answer beside inputs that are the truth now: a fresh reader
      // of its flight observes it (#3648, #3651; #3305's commit beneath a
      // flight). A first load the lane produced nothing for stays loading
      // until its flight answers (D-22, A19 exc. 1): nothing is staged.
      x._transaction = null;
      if (guess) n._pendingValue = truth;
      commitPendingNode(n);
      if (!effect) {
        if (
          n._flags & (REACTIVE_IN_HEAP | REACTIVE_DIRTY | REACTIVE_CHECK) ||
          n._statusFlags & STATUS_PENDING
        )
          n._config |= CONFIG_INPUTS_PUBLISHED;
        if (!n._equals || !n._equals(shown, n._value)) insertSubs(n);
      }
      // A stale reader's own error — its pass (a memo's or a render
      // effect's) threw over the committed value of what the parent held
      // (A10), served as the screen — is no answer of the world landing now:
      // it re-derives on it (A15; D-39, the landing is the new answer). The
      // landing's `reruns` skips an errored reader: its error is an errored
      // source's (#3945).
      if (n._flags & REACTIVE_FRAME_READ && n._statusFlags & STATUS_ERROR && x._error.source === n)
        enqueueSub(n);
    } else if (
      guess ||
      (guessed && n._config & CONFIG_SLOT_NODE && n._pendingValue !== NOT_PENDING)
    ) {
      // A guess of a dissolving lane: its truth — a landing held beneath it,
      // else the value it covered — is the parent's to land, and its readers
      // re-derive from it there. What the screen showed stays the screen
      // until then (A18 (c)): `_value` has it. A store slot the lane
      // composed (a keyed re-base) over a truth staged on it is no
      // derivation anything re-derives: it lands that truth the same way
      // (D-34, #4642).
      if (guess) n._pendingValue = truth;
      if (l._shown && guessed) marked.add(n);
      holdNode(n, into);
      insertSubs(n);
    } else {
      // A correction: the lane's derivations are void. Shown, what the
      // screen showed stays the screen (committed, inputs published — every
      // one re-derives); never shown, nothing of it reached the screen (a
      // node born in the lane stays uninitialized; its frame goes). One in
      // flight is asking the void world's question: its landing is nobody's
      // answer — it re-asks from the truth (whose notification may not reach
      // it: an input's truth equal to its committed value is silent).
      x._transaction = null;
      if (!effect) {
        // Its readers re-derive unless the lane was held: a shown lane's runs,
        // released below, show the void staging, and its own re-derivation
        // may land on what the screen shows and notify nobody (F-1, D-34,
        // #4971).
        if (!l._held && n._pendingValue !== NOT_PENDING) insertSubs(n);
        // A staged error is a staging too: void with it, or the commit below
        // would publish the empty slot as the node's error (#4748).
        n._pendingValue = NOT_PENDING;
        n._config &= ~CONFIG_STAGED_ERROR;
        if (n._statusFlags & STATUS_PENDING) {
          x._inFlight = null;
          enqueueSub(n);
        }
      }
      if (l._shown) {
        // A derivation born in the lane (its staging, voided above, was its
        // only value) whose frame stays: nothing in the truth may notify it
        // (a row the truth keeps, its fields equal) — it re-derives on the
        // truth now (D-34, F-1; #4861).
        const born = !effect && n._statusFlags & STATUS_UNINITIALIZED;
        commitPendingNode(n);
        if (!effect) {
          n._config |= CONFIG_INPUTS_PUBLISHED;
          if (born) enqueueSub(n);
        }
        // A render effect born in the lane's frame (it never ran) whose
        // first run the held lane parked, dropped below: the frame stays, so
        // the row it draws must too — it runs on the committed world (F-1,
        // #4831). (A lane's pass of an effect always owes a run.)
        else if (l._held && n._prevValue === undefined) enqueueSub(n);
      } else {
        if (x._pendingFirstChild !== null || x._pendingDisposal !== null)
          GlobalQueue._dispose(n, false, true);
        n._config &= -1048961;
      }
    }
  }
  // The parent landed: the runs the lane held (blocked on its own judgment
  // alone — a zombie it could not dispose) are the landing's. A correction
  // of a SHOWN lane keeps its runs too: "what it showed stays the screen"
  // means the runs this round's lane passes queued (the body-end seam judges
  // before the lane seam would have released them) must reach the screen —
  // their derivations' values are the committed ones now, and a
  // re-derivation that lands on the same value notifies nobody (a mapArray
  // whose rows the lane pass already built). A never-shown lane's runs are
  // void: nothing of it reached the screen. So are the runs a blocked lane
  // parked: they show a re-guess the seam never revealed, beside derivations
  // that keep what the screen showed.
  if (into === null || (l._shown && !l._held)) releaseQueues(l);
  else l._queues[0].length = l._queues[1].length = 0;
}
/** Lanes. A lane's node is read by its reader's kind: an authoritative
 * reader (`until`) sees the truth — staged beneath a guess, else committed
 * — and is no lane's; an untracked read sees the lane's latest value (A17:
 * "direct read shows optimistic"; A28); a children-forbidden reader sees
 * the frame (A32). A render effect in the frame's seat reading a lane the
 * seam found blocked, or a member whose own flight is up (held or not — the
 * lane-membership ruling, 2026-10-05), sees the screen, re-derived at the
 * reveal (#3460's stale reader) — or, the node a
 * flight that has never shown (nothing to show), waits on it: its own frame
 * holds (A15, #3334), not the lane's parent. Any other tracked reader is
 * the lane's work from this read — a derivation; a leaf reading a lane that
 * has shown, or one the seam has not judged yet (its run waits on the
 * verdict: released if the lane shows, held if it blocks) — and sees the
 * lane's latest. NOT_PENDING falls through: lane work's read of a pending
 * member throws like any. A verdict reader entering a lane the seam has
 * not judged reads ahead of it like a staging read: if the seam holds the
 * lane, the pass re-derives on the screen (`entered`, #4284) — `latest()`
 * beside the lane's nodes tears ahead of the hold however the reads are
 * grouped into effects. */
/** #4788: the lanes whose guesses an action read imperatively (its body, a
 * setter: `laneRead`, a store setter's draft), each with the question the
 * reading action asks (`question`): a guess that action writes next derives
 * from them (guesses.ts `optimisticWrite`); another action's reads are not
 * its own. Kept while the lane read lives (guesses.ts `applyGuessesAt`). */
const readGuesses = [];
function laneRead(c, el) {
  const guess = el._config & CONFIG_GUESS;
  const l = txOf(el);
  // A lane a correction folded under (D-34): store/optimistic.ts reads it.
  // Before the authoritative arm: an `until` outside the lane reads the
  // same truth there, and joins the hold it folded into — its posture over
  // a foreign hold (D-31), which the truth read alone would not take.
  const folded = c !== null && l.folded;
  if (folded) {
    const v = folded(c, el);
    if (v !== NOT_PENDING) {
      return v;
    }
  }
  if (c !== null && c._config & CONFIG_AUTHORITATIVE) {
    return GlobalQueue._truth(el);
  }
  const status = el._statusFlags;
  // An errored member's lane value predates its error; the error is the answer (#4106).
  if (!guess && status & STATUS_ERROR) {
    // The error still belongs to this lane. A downstream memo must select
    // the same update before read() throws, as it does for a lane value.
    if (c !== null && tracking && !(c._config & CONFIG_CHILDREN_FORBIDDEN)) enterLane(l, c);
    return NOT_PENDING;
  }
  if (c === null) {
    // #4788 (D-33): an action's imperative read of a guess — its body, a
    // setter — that a guess it writes next may derive from.
    // A store target's carrier is noted per key (store/optimistic.ts).
    if (guess && actionDepth && !(el._config & CONFIG_SLOT_NODE)) readGuesses.push(l, question);
    return latestOf(el);
  }
  const pending = !guess && status & STATUS_PENDING;
  if (!tracking || c._config & CONFIG_CHILDREN_FORBIDDEN) {
    // An untracked read inside lane work derives from the lane like a
    // tracked one, minus the subscription — `untrack` is about
    // dependencies, not about which world a pass derives from (a mapper's
    // row reads under its root, store/map): the pass enters the lane (two
    // lanes read by one pass link, as for tracked reads). Outside lane
    // work, the screen.
    if (passLane === null || c._config & CONFIG_CHILDREN_FORBIDDEN) {
      return screenOf(el);
    }
  } else if (c._type === EFFECT_RENDER) {
    // A flight whose value has never shown: nothing to show — the leaf
    // waits on it, and its own frame holds (A15, #3334), not the lane's
    // parent. The landing lands the frame: its own pass (the flush joined
    // its transaction) reads the answer it waited on — the lane's staging,
    // revealed at this seam. Not a member pending only on `refresh()`
    // re-asks: a re-ask asks no new question and holds nothing new (D-34
    // ¶3), so another lane's leaf reads its screen and shows that lane's
    // guess beside it (F-11, #4889).
    const never = pending && !el._x._shown && !reasks(el);
    // D-28: a leaf a verdict read moved into the lane still throws on such
    // a flight — the pass leaves the lane, and its frame holds as if the
    // read had come first (S-39, #4275). Nor does a verdict lane's seat carry
    // the leaf into another lane (no link): it reads that lane as from the
    // frame's seat (#4284).
    const p = passLane;
    const seated = isVerdict(p);
    if (seated && (never || p !== l)) setPassLane(null);
    if (passLane === null || !sameLane(passLane, l)) {
      // (A lane that has never shown: the screen is the committed value.)
      if (never && l._shown) return NOT_PENDING;
      // A held leaf reads a shown verdict lane's staging as its frame's
      // (#4275, S-39). Not a guess lane's: there it is the lane's, for its
      // next reveal — read as the frame's, the leaf's run waited with the
      // frame and the reveal never re-ran it; a guess into a log whose lane
      // has shown stayed hidden behind another action's refresh (F-11,
      // D-26, #4666). It reads the screen and re-derives at the reveal
      // (below), or enters the lane.
      if (
        !guess &&
        l._shown &&
        isVerdict(l) &&
        el._pendingValue !== NOT_PENDING &&
        c._config & CONFIG_HELD &&
        flushTransaction !== null &&
        resolveTx(flushTransaction) === txOf(c)
      ) {
        return el._pendingValue;
      }
      if (l._held || pending) {
        // S-58 (D-25): a mount a probing pass opened (CONFIG_MOUNT_AHEAD) is
        // born held on its first pass's plain read of the lane's flight, not
        // served the screen: served, it initializes with S-46's frame, which
        // the lane reveals at its landing. It waits on the flight instead
        // (`read` throws the flight's NotReady below the lane arm).
        if (pending && c._config & CONFIG_MOUNT_AHEAD && c._statusFlags & STATUS_UNINITIALIZED)
          return NOT_PENDING;
        // Re-derived at the reveal; once per pass (the pass may also be a stale
        // reader of a transaction, REACTIVE_FRAME_READ — both reruns apply).
        // Before the throw below: a leaf that waits on a lane that has never
        // shown has no flight to settle it — only the reveal re-derives it,
        // and its pending holds its frame until then (#4338). Not a member's
        // own flight in a lane that showed and is not held: the reveal has
        // nothing new for the leaf, which re-derives when that flight lands
        // — re-queued at every reveal, each re-derivation read the same
        // flight and queued the next, and the flush never settled (#4840).
        if (!(c._flags & REACTIVE_SCREEN_READ) && (l._held || !l._shown)) {
          c._flags |= REACTIVE_FRAME_READ | REACTIVE_SCREEN_READ;
          l._reruns.push(c);
        }
        // Nothing on screen yet (uninitialized: NOT_PENDING).
        const v = screenOf(el);
        if (v === NOT_PENDING) throw new NotReadyError(null);
        return v;
      }
      // A verdict reader reads ahead of the seam's verdict: re-derived on the
      // screen if the lane holds (#4284).
      if (c._config & CONFIG_VERDICT && !isVerdict(l)) entered.push(c);
    }
  }
  // A render effect of the lane reading a member that waits on `refresh()`
  // re-asks only reads what it shows: a re-ask holds nothing new (D-34 ¶3,
  // `blockedBy` lets such a member's lane reveal), so a row the lane builds
  // binds beside the reader that shows it (F-1, #4864). A boundary's tree
  // (CONFIG_FRAME, not an effect) flattens what its insert effects would show: a shown
  // member reads as it shows, the lane's guesses on the screen (D-33, #4999).
  enterLane(l, c);
  return pending &&
    !(guessed && c._type === EFFECT_RENDER && reasks(el)) &&
    !(c._config & CONFIG_FRAME && !c._type && el._x._shown)
    ? NOT_PENDING
    : latestOf(el);
}
/** A guess in `l` whose own truth is in flight (its refetch — authoritative:
 * the landing supersedes, not the body's end). */
function guessFlights(l) {
  for (let i = 0; i < l._nodes.length; i++) {
    const n = l._nodes[i];
    if (n._config & CONFIG_GUESS && n._x._transaction === l && inFlight(n)) return true;
  }
  return false;
}
/** The seam's verdict on a lane. Blocked — its own flight is up — nothing
 * moves: the screen keeps what it shows, the lane's runs wait. Unblocked,
 * the lane shows: its values are the screen from now (`_shown`), the
 * staging its passes made since the last reveal is promoted, their frames
 * (children) apply, the stale readers re-derive (#3460), its runs go. */
function laneSeam(l, leaked) {
  // Blocked — or one of its passes read a write the frame holds this seam:
  // its runs wait, the pass re-derives on the screen next round.
  // Re-based over a question still parked on another read (#4525): its rows
  // are that answer's, shown only with the question (F-1, D-34 ¶3, #4559).
  if (leaked || blocked(l) || (l.over && blocked(liveTx(l.over) || l))) {
    l._held = true;
    return;
  }
  l._held = false;
  l._shown = true;
  const p = resolveTx(l._parent),
    v = p._reruns.filter(r => r._modified && r._x?._transaction === l);
  if (reruns(l)) revealing.push(p);
  // The parent lands this seam and re-derives readers whose runs the lane
  // stashed, voiding them (`reruns`: they are for the frame being replaced).
  // Beside other runs of the lane they show first: the lane's readers show
  // its answer together (F-6), and the re-derivation follows next round
  // (#4677: `pending: true` on an async memo over `isPending(id)` beside a
  // binding still `false`). Restoring a run `reruns` skipped changes nothing.
  else if (l._queues[0].length > v.length)
    l._queues[0].unshift(() => v.forEach(r => (r._modified = true)));
  for (let i = 0; i < l._nodes.length; i++) {
    const n = l._nodes[i];
    // (Not once a pass since has notified them — D-37: a lane does not
    // replay a run the screen made.)
    if (n._notifiedAt === LEFT_SHOWN) insertSubs(n);
    const x = n._x;
    if (x._transaction !== l) continue;
    // The lane's value is the screen's: a guess moves from the lane's
    // record to `_value` (what it covered takes its place there, for the
    // revert); a derivation's staging commits (a staged error publishes,
    // D-41; the frame it built retires the one it replaces). A truth held
    // beneath a guess stays its staging. An effect's value slot is private.
    if (!n._type)
      if (n._config & CONFIG_GUESS) {
        guessScreen(n, true);
        if (!n._x._shown) {
          const g = covers.get(n);
          covers.set(n, n._value);
          n._value = g;
          n._x._shown = true;
          if (n._statusFlags & STATUS_UNINITIALIZED) n._statusFlags &= ~STATUS_UNINITIALIZED;
        }
      } else if (n._pendingValue !== NOT_PENDING) {
        cover(n);
        commitPendingNode(n);
        n._x._shown = true;
      }
    // The frame this pass built is the screen's now; the one it replaced
    // goes (as `commitPendingNode` retires a committed pass's).
    if (x._pendingFirstChild !== null || x._pendingDisposal !== null)
      GlobalQueue._dispose(n, false, true);
    n._config &= ~CONFIG_STAGED;
  }
  releaseQueues(l);
}
/** The parent landed: its lanes end, nested first. (None is blocked: a
 * blocked lane blocks its parent.) */
function endLanes(u) {
  if (guessed) for (const n of u._nodes) marked.delete(n);
  // A revert landing (#4336): the guesses its lane took from another
  // action come back in this frame (guesses.ts `revertLanding`).
  if (guessed) revertLandingHook(u);
  for (let k = lanes.length - 1; k >= 0; k--) {
    const l = lanes[k];
    if (resolveTx(l._parent) !== u) continue;
    endLanes(l);
    dissolveLane(l, null);
  }
}
/** A blocked lane blocks its parent (maintainer, 2026-10-01): the lane's
 * pending work derives from the frame's write through the guess, so the
 * frame is not complete without it (A15) — landing it would commit the write
 * beside a derivation still showing the old one. A guess whose own flight is
 * up likewise: the question was asked in this frame. So does a shown guess
 * a correction re-homed into `t` (`marked`): still the screen's and the
 * untracked reads' value (F-11, D-34), its flight — the action's `refresh()`
 * after an outside re-ask corrected it — is the answer it waits for, as
 * before the correction (D-7, D-35). With no render effect pending on that
 * flight, `t` landed at the body's end: effects and untracked reads saw the
 * re-ask's answer until the refresh landed, while rendered readers kept the
 * guess (#4690). Not a lane's: guessed again there, its flight blocks the
 * parent, not the lane (A17, `linkBlocked`). */
function lanesBlocked(t) {
  if (guessed && !t._lane && t._nodes.some(n => marked.has(n) && inFlight(n))) return true;
  // A lane flight one of the frame's own passes asked (`laneWait`, S-57):
  // the frame holds on it and reveals with the lane. Kept on the lane, and
  // the frame found through `over`, so a frame merged since still waits.
  if (!t._lane)
    for (let i = 0; i < lanes.length; i++) {
      const l = lanes[i];
      if (
        l.waits !== undefined &&
        l.over !== undefined &&
        liveTx(l.over) === t &&
        l.waits.some(n => n._statusFlags & STATUS_PENDING)
      )
        return true;
    }
  // S-46 (D-25): a mount a display-ahead read gates attaches at the
  // landing; the verdict lane that opened it reveals with that landing, not
  // ahead of it (shown now, the branch attached with its hole empty, #4417,
  // or with the committed value a re-derived mount read).
  if (
    t._lane &&
    t._parent._verdict === t &&
    ((t._opened && holdsOn(resolveTx(t._parent))) || heldMountUnder(null, t))
  )
    return true;
  // A frame flight the seam parked beside its reader's lane pass (R5) is the
  // hold's question still: the hold waits on it (`park`).
  if (!t._lane && r5Hook && r5Hook(null, t)) return true;
  return (t._lane && linkBlocked(t)) || nestedBlocked(t);
}
/** Mounts a probing pass opened and a hold holds (CONFIG_MOUNT_AHEAD,
 * born held render effects): collected from the live transactions' node
 * lists — each list scanned once past where it was last scanned, so a
 * node is looked at once in its life — and pruned as they attach or go
 * away. Almost always empty: a born-held mount is rare, and every check
 * below is a walk of this list, never of the holds. */
const aheadMounts = [];
function collectMounts() {
  for (let k = 0; k < transactions.length; k++) {
    const t = transactions[k],
      nodes = t._nodes;
    for (let i = t.scanned ?? 0; i < nodes.length; i++) {
      const r = nodes[i];
      // (A node re-held into a second transaction is listed twice; the
      // second entry is pruned with the first.)
      if (r._config & CONFIG_MOUNT_AHEAD) aheadMounts.push(r);
    }
    t.scanned = nodes.length;
  }
}
/** The hold `t` waits: an action open in its frame, or a flight it holds. */
function holdsOn(t) {
  return (
    t._open !== 0 ||
    t._nodes.some(
      n => !n._type && n._statusFlags & STATUS_PENDING && !(n._flags & REACTIVE_DISPOSED)
    )
  );
}
function shownBy(r) {
  for (let s = r._subs; s !== null; s = s._nextSub)
    if (s._sub._type === EFFECT_RENDER && !hiddenHook?.(s._sub)) return 1;
}
/** A collected mount still held, uninitialized, on screen and waiting —
 * on its own flight, an action open in its frame, or a flight its frame
 * holds — under `o`, or under one of lane `l`'s nodes (by its owner
 * chain). */
function heldMountUnder(o, l) {
  collectMounts();
  for (let i = aheadMounts.length - 1; i >= 0; i--) {
    const r = aheadMounts[i];
    if (
      !(r._statusFlags & STATUS_UNINITIALIZED) ||
      !(r._config & CONFIG_HELD) ||
      r._flags & REACTIVE_DISPOSED
    ) {
      aheadMounts.splice(i, 1);
      continue;
    }
    const t = resolveTx(r._x._transaction);
    if (
      // A memo mount is on screen through the render effect that reads it
      // (#4806: a fragment, text beside a hole, returned to the slot). One
      // a memo reads is that memo's: a boundary the mount builds around it
      // shows its fallback (#3851).
      (r._type ? hiddenHook?.(r) : !shownBy(r)) ||
      !(r._statusFlags & STATUS_PENDING || holdsOn(t))
    )
      continue;
    for (let p = r._parent; p !== null; p = p._parent)
      if (l !== null ? p._x?._transaction === l : p === o) return true;
  }
  return false;
}
/** F-2 (S-57): a pass this flush dirtied — it read the flush's staging —
 * ran as a lane's (a derivation of a guess, or a `latest`-only asker in a
 * verdict lane) and asked: its flight is the lane's, so the flush would
 * have nothing of its own to wait for and would publish the write beside a
 * stale derivation of it (F-1). The flush joins a frame instead and holds
 * on the flight, revealing with the lane at its landing. */
function laneWait(el, l) {
  // A guess's lane (the row's shape). A verdict lane's `latest`-only asker
  // re-asked by an outside write is the same sentence, not built: the
  // write publishes as before.
  if (!globalQueue._running || l._parent._verdict === l) return;
  const t = flushTransaction;
  // The hold's own work (the lane's parent is this flush's frame): it
  // already waits on its lane (`nestedBlocked`), a round after the reveal.
  if (t !== null && resolveTx(l._parent) === t) return;
  // Entangled: the lane's work reads the frame's staging as its own world
  // (core.ts `frameRead`, the `over` arm) and the lane holds while the
  // frame does (`laneSeam`) — one question, one reveal. A lane already
  // over another live frame (a keyed rebase over a landing's answer,
  // #4536) cannot take a second: the write publishes as before, beside the
  // lane's answer to its old question (the limit of this build).
  const o = l.over && liveTx(l.over);
  if (o && o !== t) return;
  joinFuture(null);
  l.over = flushTransaction;
  (l.waits ??= []).push(el);
  // Not a staged reader to re-derive on the committed world at this seam:
  // that would ask the question of the old world again.
  const k = stagedReaders.indexOf(el);
  if (k !== -1) stagedReaders.splice(k, 1);
}
/** Parents whose lane revealed this seam with readers to re-derive there
 * (`laneSeam`): the lane shows ahead of its parent (F-6), so the parent lands
 * a round later — after those readers have shown the lane's answer — even
 * when nothing else holds it (#4393). */
const revealing = [];
/** The lanes under `t` (its own; a link group's are each other's). */
function nestedBlocked(t) {
  if (revealing.indexOf(t) !== -1) return true;
  for (let i = 0; i < lanes.length; i++) {
    const l = lanes[i];
    if (resolveTx(l._parent) !== t) continue;
    if (blocked(l) || guessFlights(l)) return true;
  }
  return false;
}
/** A write landing on a lane's node (core.ts `setSignal`: a user's write, an
 * async landing). On a guess it is its truth (A18 — an async guess's own
 * landing), judged against the guess. On a derivation it is the lane's
 * latest: its staging once the lane has shown (promoted at the next
 * reveal), its slot before; the lane's members re-derive as its work. */
function laneWrite(el, v) {
  // A write on a guess: the guess layer's (guesses.ts `guessWrite`) —
  // judged against the guess, or held beneath it; false: a store slot's
  // guess its derive's re-ask in another action's lane writes is that
  // lane's derivation from here (store `_slotTake`, #4336).
  if (el._config & CONFIG_GUESS && guessWriteHook(el, v)) return v;
  const l = txOf(el);
  if (el._equals && el._equals(latestOf(el), v)) return v;
  el._pendingValue = v;
  if (el._fn !== undefined) el._time = clock;
  laneDirty(el, l);
  schedule();
  return v;
}
/** The lanes alive before this seam's guesses opened theirs (a prefix of
 * `lanes`; nothing dissolves one between here and the seam loop): a lane
 * born at the seam has no work to judge yet (its passes run next round) —
 * it is neither shown nor blocked until it has. */
let judged = 0;
/** The guess layer's seam (guesses.ts `applyGuessesAt`), before its
 * guesses open their lanes. */
function markJudged() {
  judged = lanes.length;
}
/** The effects that derive plainly from a landing this round that
 * re-derives a stale reader (A15) reading it as committed: its held runs
 * wait for the re-derivation (`land`), and these runs are that frame too —
 * they wait with them (verdict.ts `attachWaits`; F-1, F-7, #4677). The walk
 * stops at verdict readers: what derives from a display-ahead read shows
 * now (F-6, D-25, #4398). Dropped at the end of the round's effect phase. */
let waiting = null;
/** A transaction lands: its lanes end, nested first (`endLanes`). */
function landing$1(u) {
  walk(u);
  endLanes(u);
}
/** Read before the lanes end: a lane's runs are its reveal's (F-6). */
function walk(u) {
  // As `reruns` judges it: a stale-reader memo re-derives onto a question of
  // its own, and the landing's held runs wait for a re-derivation.
  if (
    !u._reruns.some(r => !r._type && r._flags & REACTIVE_FRAME_READ) ||
    !u._reruns.some(
      r =>
        (r._flags & REACTIVE_FRAME_READ || r._config & CONFIG_VERDICT) &&
        (r._type || r._config & CONFIG_HELD)
    )
  )
    return;
  if (!waiting) ((waiting = new Set()), globalQueue._queues[1].push(() => (waiting = null)));
  const q = u._nodes.filter(n => !(n._config & CONFIG_VERDICT));
  for (let n; (n = q.pop());)
    for (let s = n._subs; s; s = s._nextSub) {
      const c = s._sub;
      if (waiting.has(c) || c._config & CONFIG_VERDICT || c._x?._transaction?._lane) continue;
      waiting.add(c);
      if (!c._type) q.push(c);
    }
}
/** `r`'s pass read a node a transaction holds (#4439). */
function readsHeld(r) {
  for (let d = r._deps; d; d = d._nextDep) if (d._dep._config & CONFIG_HELD) return true;
  return false;
}
const laneSeams = t => {
  // Passes that read a write the parked frame holds as the screen
  // (`stagedReaders`: lane work, a verdict reader before the verdict): they
  // re-derive on the committed world next round (and register as stale
  // readers of the transaction there); a lane's runs wait that round — a
  // lane sees the screen plus its own guesses.
  if (revealing.length) revealing.length = 0;
  // #4661 (F-6): a verdict reader whose pass read a flight of the frame
  // ahead of it (`latest`, verdict.ts `pendingVerdict`) before any plain
  // read of one is no frame reader of it, though its stale read of the
  // frame's staging (A10, `stagedScreen`) marked it one
  // (REACTIVE_FRAME_READ, which `blockedBy` counts): the mark comes off and
  // the reader holds nothing. It keeps what it read — the committed value
  // ("It should read the committed value, we can't reveal early outside of
  // latest. This is intentional tearing.", maintainer, 2026-11-03) — and the
  // flight's own landing re-derives it, not the frame's.
  for (const c of ahead.splice(0))
    c._flags & REACTIVE_FRAME_READ &&
      ((c._flags ^= REACTIVE_FRAME_READ), t?._reruns.splice(t._reruns.indexOf(c) >>> 0, 1));
  const leaked = [];
  if (t !== null)
    for (let i = 0; i < stagedReaders.length; i++) {
      const r = stagedReaders[i],
        // A pass no transaction holds that read the frame's staging (an
        // effect, a `[s(), latest(s)]` reader watched before the frame had
        // a transaction): judged by the frame's — it re-derives only if the
        // frame stays parked; landing here, what it read is the screen (one
        // run, D-22).
        l = r._x?._transaction || (r._flags & REACTIVE_STAGED_READ ? t : null);
      // A verdict reader a later read of the same pass routed into a verdict
      // lane answered for itself: it re-derives at the holder's landing, not
      // now (one run — #3322, #3540). Other verdict-lane work re-derives only
      // if the frame stays parked: a transaction landing at this seam makes
      // what it read the screen (one run). The frame is the transaction that
      // holds what it read (`t`, this flush's — not always the lane's parent:
      // a landing that reached a frame held elsewhere joined that one). A reader a
      // transaction holds that read none of its stagings (a verdict reader
      // that probed only a flight, held as the frame's pending observer or
      // its staged derivation) has nothing to re-derive: its re-run
      // re-enters that transaction and parks it again, every round — it
      // runs at the landing (#4349). Not one whose probe this seam's park
      // answered: a flight it asked about before anything joined is held
      // now, so its re-run routes into the holder's verdict lane and shows
      // the verdict now (F-6, D-23); run only at the landing, its
      // `isPending` read `false` for the whole hold (#4439). One that read a
      // staging, or such a flight, re-derives only if its frame stays
      // parked: landing at this seam, what it read is the screen (one run,
      // D-22). Not one born into that frame (a held pass's child, ruling A):
      // it has no screen, its reads are the transaction's either way — it
      // runs again at the landing, not now. Nor lane work re-based over this
      // frame's answer (#4536, merged since or not): its rows are that
      // answer's, so its other reads are too (core.ts `frameRead`), and the
      // lane holds while the frame does (`laneSeam`).
      // Re-derived on the committed world, it paired the new rows with the
      // old inputs, and the landing revealed that beside them (F-1, D-34 ¶3,
      // #4577). A lane linked to such a lane counts (one reveal unit): a
      // memo seated in one whose pass read through the other read the
      // frame's staging there, and, re-derived, it re-entered that lane,
      // read it again and parked again, every round (#4643).
      if (
        r._flags & REACTIVE_DISPOSED ||
        (l?._lane
          ? (isVerdict(l) &&
              (r._config & CONFIG_VERDICT
                ? !(r._flags & REACTIVE_PROBE_UNANSWERED)
                : !blocked(t))) ||
            (l._links || [l]).some(u => u.over && liveTx(u.over) == t)
          : l &&
            !(
              r._flags & REACTIVE_PROBE_UNANSWERED ||
              ((r._flags & REACTIVE_STAGED_READ || readsHeld(r)) && blocked(l) && !bornInto(r, l))
            ))
      )
        continue;
      // A leaked effect re-derives seated in its lane from its first read: it
      // carries no lane value (`laneStage`), so without this its pass starts
      // as mainline, joins the parked frame on its first held read and reads
      // the staging, then re-enters the lane at its guess read and leaks
      // again — every round (#4014). A computed is already seated by its
      // override, and a written guess's own pass stays the frame's.
      if (l?._lane && r._type) r._flags |= REACTIVE_LANE_DIRTY;
      // R5 for an async reader: its frame pass asked the frame's question
      // (F-6's lane gets its own pass now). That flight is the frame's
      // answer: the landing adopts it rather than asking it again (F-9: a
      // re-ask of the same question is no new question; `reruns`). A frame
      // landing at this seam has nothing to re-derive: the pass was the
      // landing's, its flight the screen's answer. Sync or async, the
      // reader's lane pass is still to come: the verdict lane it joins does
      // not reveal this seam (F-6: the lane's readers show its answer
      // together, D-23: atomic with the value) — revealed now, it showed a
      // verdict beside the frame pass's value, before the pass that answers
      // for the lane, and a reader of the re-derived memo read it outside
      // the shown lane and waited for the frame (#4393).
      else if (!l?._lane && r._flags & REACTIVE_PROBE_UNANSWERED) {
        // (verdict.ts `r5`: only the verdict engine leaves a probe unanswered.)
        if (r5Hook(r, t)) continue;
        if (t?._verdict && leaked.indexOf(t._verdict) === -1) leaked.push(t._verdict);
      }
      enqueueSub(r);
      if (l?._lane && leaked.indexOf(l) === -1) leaked.push(l);
    }
  // A verdict lane born at this seam is judged with it: it exists only once
  // a pass routed into it — the park seam's zombie verdict readers re-run
  // as its work so that this seam reveals them (#3444).
  // A lane a guess re-asked at this seam is judged with that guess's passes,
  // next round: its reveal is whole (#4775).
  for (let k = lanes.length - 1; k >= 0; k--)
    if ((k < judged && lanes[k]._reguessed !== clock) || isVerdict(lanes[k]))
      laneSeam(lanes[k], leaked.indexOf(lanes[k]) !== -1);
  // A verdict reader that entered a lane this seam held: its run waits with
  // the lane, and the verdict beside it would freeze — it re-derives next
  // round from the frame's seat, a stale reader of the lane (#4284).
  for (let r; (r = entered.pop());)
    if (r._x._transaction?._held) {
      r._flags &= ~REACTIVE_LANE_DIRTY;
      enqueueSub(r);
    }
  // (verdict.ts `r5`: the round's armed readers and re-asked passes ran.)
  if (r5Hook) r5Hook(null, null);
};
/** A15: lane work or a verdict reader was served `el`'s committed value
 * (core.ts `frameRead`), so its pass does not wait on `el`'s flight: what
 * `el`'s pending propagated onto it before the pass (its origins, `el`
 * included) is not this pass's — the landing re-derives it (`reruns`).
 * Kept, a pass that went pending on another source never woke when that
 * source settled (#4397). Only a reader something made pending has anything
 * to drop. What the drop retires is retired from the dependents that carried
 * it through `c` too: the source's own landing walk stops at `c`, which no
 * longer has it, and a render effect left marked would hold its transaction
 * on a flight that has landed (D-11, F-2, #4576; a nested action's refresh). */
/** Each lane pass served a flight's committed value: that flight, and the
 * pass (`_depGen`), so a later pass of the same node did not read it. Kept
 * per node: a pass that runs another node's pass inside its own (a nested
 * memo created or pulled mid-body) must still find its own record after the
 * inner one recorded (#4927). */
const served = new WeakMap();
const laneServed = (c, el, flight) => {
  // Lane work read a staging of the flush after it was served the committed
  // value of a flight (core.ts `stagedRead`): it derives from the write that
  // re-asked the flight, so it waits on it — never the new value beside the
  // old answer (F-1, F-2; S-57).
  // Asked with `flight` (core.ts `recompute`): whether it was so served.
  if (el === null) {
    const s = served.get(c);
    if (s && s[1] === c._depGen && s[0]._statusFlags & STATUS_PENDING) {
      if (flight) return true;
      throw s[0]._x._error;
    }
    return;
  }
  // Lane work reading a flight that is not the lane's own (core.ts `read`,
  // A31): served its committed value — the lane breaks out of its parent's
  // hold — unless the pass read a staging of the flush, which makes it that
  // write's work: it waits on the flight as mainline does (F-2; S-57).
  // Served, the pass still derives from the flight: its stale reader (A15,
  // #3494), so the frame holds on it and re-derives it at the landing.
  if (flight) {
    if (c._flags & REACTIVE_STAGED_READ) return false;
    const u = el._x?._transaction || flushTransaction;
    if (u) staleReader(c, resolveTx(u));
    served.set(c, [el, c._depGen]);
    return true;
  }
  const from = el._x?._pendingSources;
  if (from && c._x?._pendingSources)
    for (const s of from) if (removePendingSource(c, s)) settlePendingSource(c, s);
};
/** This flush's verdict readers that read a flight ahead of the frame
 * (#4661, the seam). */
const ahead = [];
/** Render effects that read a verdict and entered a lane before its seam (#4284). */
const entered = [];
installLaneHooks(laneRead, laneStage, laneWrite, laneSeams, landing$1, lanesBlocked, laneServed);
GlobalQueue._laneGuesses = t => {
  const out = [];
  for (let i = 0; i < lanes.length; i++) {
    const l = lanes[i];
    if (resolveTx(l._parent) !== t) continue;
    for (let j = 0; j < l._nodes.length; j++)
      if (l._nodes[j]._config & CONFIG_GUESS && l._nodes[j]._x._transaction === l)
        out.push(l._nodes[j]);
  }
  return out;
};
GlobalQueue._verdictLane = verdictLane;

// A tracked effect's heap visit is its empty compute phase, then the user
// queue: routing through the heap orders the run after the commit whatever
// phase the write came from (#3291).
GlobalQueue._update = el => {
  if (el._type === EFFECT_TRACKED) {
    deleteFromHeap(el, dirtyQueue);
    el._modified = true;
    globalQueue.enqueue(EFFECT_USER, el._run);
  }
  // A zombie's fate is the seam's; it runs now only for its lane's re-staging,
  // which shows ahead of any park (#3463).
  else if (el._flags & REACTIVE_ZOMBIE && !(el._flags & REACTIVE_LANE_DIRTY)) deferZombie(el);
  // A lane frame member follows mainline writes only: the lane's dirty is
  // cancelled (#3698, #4137). Folded away without the lane engine.
  else if (laneStageHook && el._flags & REACTIVE_ZOMBIE && laneFrameMember(el)) {
    deleteFromHeap(el, dirtyQueue);
    el._flags &= -65540;
  }
  // A reader the landing re-derives whose frame flight the seam parked
  // adopts it at its turn (verdict.ts `adoptParked`, D-25, F-9).
  else if (!(laneAdoptHook && el._x?._parked && laneAdoptHook(el, 1))) recompute(el);
};
/** The nearest non-zombie owner carries a lane's value or, an effect, is
 * listed in a lane. */
function laneFrameMember(el) {
  let o = el._parent;
  while (o._flags & REACTIVE_ZOMBIE) o = o._parent;
  return (
    (o._config & (CONFIG_OVERRIDE | CONFIG_GUESS)) === CONFIG_OVERRIDE ||
    (o._type && o._x?._transaction?._lane)
  );
}
GlobalQueue._dispose = disposeChildren;
const PRIMITIVE_IN_FORBIDDEN_SCOPE_MESSAGE =
  "[PRIMITIVE_IN_FORBIDDEN_SCOPE] Cannot create reactive primitives inside createTrackedEffect or owner-backed onSettled";
const PRIMITIVE_IN_EFFECT_CALLBACK_MESSAGE =
  "[PRIMITIVE_IN_EFFECT_CALLBACK] Cannot create a memo, effect or root with no owner inside an effect callback. The effect " +
  "phase runs with no ambient owner, so a computation created here belongs to nobody: it is never disposed and never held. " +
  "Create it in the compute (a flow component owns its children), or parent it explicitly with runWithOwner.";
const PRIMITIVE_IN_REF_CALLBACK_MESSAGE =
  "[PRIMITIVE_IN_EFFECT_CALLBACK] Cannot create a memo, effect or root inside a ref callback. A ref callback runs like " +
  "a render effect's callback; create it in the factory that returns the ref (the component body).";
/** Dev: a computed or root created ownerless in an effect callback is outside
 * every owner and hold. `runWithOwner` there is deliberate and allowed. */
function assertNotInEffectCallback() {
  if (
    inEffectCallback &&
    context === effectCallbackOwner &&
    (effectCallbackEvent === ANY_EVENT || dispatchingEvent() === effectCallbackEvent)
  ) {
    const message = context
      ? PRIMITIVE_IN_REF_CALLBACK_MESSAGE
      : PRIMITIVE_IN_EFFECT_CALLBACK_MESSAGE;
    emitDiagnostic({
      code: "PRIMITIVE_IN_EFFECT_CALLBACK",
      kind: "lifecycle",
      severity: "error",
      message
    });
    throw new Error(message);
  }
}
const REACTIVE_WRITE_IN_OWNED_SCOPE_SIGNAL_MESSAGE =
  "[REACTIVE_WRITE_IN_OWNED_SCOPE] Writing to reactive state inside an owned scope (component, computation) is not allowed. " +
  "Move the write outside or set the `ownedWrite` option if this is intentional.";
const REACTIVE_WRITE_IN_OWNED_SCOPE_REFRESH_MESSAGE =
  "[REACTIVE_WRITE_IN_OWNED_SCOPE] Calling refresh() inside an owned scope (component, computation) is not allowed. " +
  "Move the invalidation outside pure computation.";
let tracking = false;
let context = null;
let snapshotCaptureActive = false;
/** The lane at the root of the running chain of first passes, inherited by
 * every first pass under it (#4166). */
let laneRoot;
let snapshotSources = null;
function ownerInSnapshotScope(owner) {
  while (owner) {
    if (owner._snapshotScope) return true;
    owner = owner._parent;
  }
  return false;
}
function setSnapshotCapture(active) {
  snapshotCaptureActive = active;
  if (active && !snapshotSources) snapshotSources = new NewSet();
}
/** Mark `owner` a snapshot scope: readers created under it serve each
 * source's value from when hydration began and replay at
 * `releaseSnapshotScope`. With `served`, `owner` is a hydrated computed and
 * `served` its server answer: it becomes the node's snapshot if it has none. */
function markSnapshotScope(owner, served) {
  if (arguments.length < 2) owner._snapshotScope = true;
  else if (snapshotCaptureActive && owner._x?._snapshotValue === undefined)
    takeSnapshot(owner, served);
}
function releaseSnapshotScope(owner) {
  owner._snapshotScope = false;
  releaseSubtree(owner);
  schedule();
}
function releaseSubtree(owner) {
  let child = owner._firstChild;
  while (child) {
    if (child._snapshotScope) {
      child = child._nextSibling;
      continue;
    }
    if (child._fn) {
      const comp = child;
      comp._config &= ~CONFIG_IN_SNAPSHOT_SCOPE;
      if (comp._flags & REACTIVE_SNAPSHOT_STALE) {
        comp._flags &= ~REACTIVE_SNAPSHOT_STALE;
        comp._flags |= REACTIVE_DIRTY;
        if (dirtyQueue._min > comp._height) dirtyQueue._min = comp._height;
        insertIntoHeap(comp, dirtyQueue);
      }
    }
    releaseSubtree(child);
    child = child._nextSibling;
  }
}
function clearSnapshots() {
  if (snapshotSources) {
    for (const source of snapshotSources) {
      // Assign, never `delete`: deleting pushes `_x` to dictionary mode.
      const x = source._x;
      if (x != null) x._snapshotValue = undefined;
    }
    snapshotSources = null;
  }
  snapshotCaptureActive = false;
}
function recompute(el, create = false) {
  // Any recompute can clean a marked subscriber: invalidate the skip (§12d).
  bumpNotifyEpoch();
  const isEffect = el._type;
  let released;
  let superseded = NOT_PENDING;
  // Lanes: a pass is seated by its node. A node carrying a lane's value runs
  // as the lane's whoever dirtied it; a guess's own pass is its truth arriving,
  // the frame's (F-11). A first pass takes its creator's lane, except under a
  // verdict lane: a mount there is mainline (D-25, #3851). Restored at the end.
  // Folded away without the lane engine.
  const prevLane = passLane;
  // The mount a first pass belongs to, inherited from its creator (#4166).
  const prevMount = laneRoot;
  const mount = laneStageHook && (laneRoot = create && (prevLane || prevMount));
  // The seam's re-derive runs with its parked flight current; a later pass
  // asks its own question (D-25, verdict.ts `reask`).
  if (laneAdoptHook && !create && el._x) {
    const p = el._x._parked;
    if (
      p
        ? p !== el._x._inFlight && ((el._x._parked = undefined), 1)
        : el._config & CONFIG_OVERRIDE && !(el._flags & REACTIVE_LANE_DIRTY)
    )
      laneAdoptHook(el, 2);
  }
  if (laneStageHook) {
    const cp = create ? creatorPass(context) : null;
    const cf = cp?._flags | 0;
    // A first pass under a pass that probed is the mount the probe gates, and so
    // is what that pass creates (D-25, S-46, S-58, #4806).
    if (
      cf & REACTIVE_PROBED ||
      (cp &&
        ((cp._config & CONFIG_MOUNT_AHEAD && cp._statusFlags & STATUS_UNINITIALIZED) ||
          (cp._type === EFFECT_RENDER &&
            cp._depsTail &&
            hasDep(cp, d => d._config & CONFIG_VERDICT, cp._depsTail))))
    )
      el._config |= CONFIG_MOUNT_AHEAD;
    setPassLane(
      (el._flags & REACTIVE_LANE_DIRTY ||
        (el._config & (CONFIG_OVERRIDE | CONFIG_GUESS)) === CONFIG_OVERRIDE) &&
        el._x?._transaction?._lane
        ? el._x._transaction
        : // Loose `!=`: false with no creator lane (undefined == null).
          prevLane?._parent._verdict != prevLane && cf & REACTIVE_RECOMPUTING_DEPS
          ? prevLane
          : null
    );
  }
  // Observe: fired while `_deps` still holds the previous run's links.
  let devChanged = false;
  {
    el._devWindows = 0;
    if (attrHooks !== null) attrHooks.recomputeStart(el, create);
  }
  // An error a pass staged, which this pass replaces.
  const replaced = el._config & CONFIG_STAGED_ERROR;
  let linked;
  if (!create) {
    deleteFromHeap(el, dirtyQueue);
    // A pending flight is the hold's own work (#3884), and so is a held async
    // memo's re-run with no window of its own (D-25, `holdWork`): clear the
    // verdict posture before the flight is superseded, or a frame read would
    // settle it and drop the hold. Not an effect. Folded away without the lane
    // engine.
    if (
      laneStageHook &&
      !isEffect &&
      (el._statusFlags & STATUS_PENDING ||
        (el._config & (CONFIG_ASYNC | CONFIG_OWN_PROBE | CONFIG_HELD)) ===
          (CONFIG_ASYNC | CONFIG_HELD))
    )
      el._config &= ~CONFIG_VERDICT;
    if (el._x !== null) {
      el._x._inFlight = null;
      // Supersede kills an iterator flight (#3122).
      releaseFlightTeardown(el);
    }
    // A pass over a held derivation joins its hold (D-1); a node held with nothing
    // staged joins too. Not a render effect or boundary structure: outside its
    // hold's flush the frame publishes mainline (#4283). A frame born into the
    // hold keeps staging (#4010).
    if (el._config & CONFIG_HELD) {
      const tx = txOf(el);
      // Under a blocked lane the frame joins nothing: a lane never holds a sync
      // write (#3460). An answered deferred node's re-pass lags and joins nothing
      // (D-40). A verdict reader born into the hold is its own work (D-25, D-12,
      // #4467). `EFFECT_RENDER << 23` is CONFIG_FRAME.
      if (tx._lane || el._config & CONFIG_LAGS);
      else if (
        !(el._config & (CONFIG_VERDICT | (EFFECT_RENDER << 23))) ||
        (observeFlightHook && !(el._config & (EFFECT_RENDER << 23)) && bornInto(el, tx))
      )
        joinPassTx(tx, freshHook?.(el, el._statusFlags & STATUS_UNINITIALIZED));
      else if (
        tx !== flushTransaction &&
        !(el._statusFlags & STATUS_UNINITIALIZED || el._config & CONFIG_HELD_FRAME)
      ) {
        // Published mainline, the frame is replaced on the spot and the landing
        // re-derives a held one (#3404). Readers of the superseded staging re-derive
        // even if the value is unchanged (#4378).
        if ((superseded = el._pendingValue) !== NOT_PENDING) insertSubs(el);
        el._pendingValue = NOT_PENDING;
        el._config &= ~CONFIG_HELD;
      }
    }
    // Children of a pass still awaiting its commit were never shown and die now;
    // otherwise they are the frame and park until this pass commits.
    if (isEffect === EFFECT_TRACKED || el._config & CONFIG_STAGED) {
      // Lazy nodes only those children read release at this pass's end unless
      // re-read (#4142). A throwing cleanup is rethrown when the flush ends.
      const prev = deferredReleases;
      deferReleases((released = []));
      disposeChildren(el);
      deferReleases(prev);
    } else if (el._firstChild !== null || el._disposal !== null) parkChildren(el);
    // This pass replaces a staged error (D-41, #4109, #4145); a held one stays
    // `latest`'s answer while this pass's flight is up (D-22). `linked`: the pass
    // had sources.
    if (passHook) {
      linked = el._deps !== null;
      // Only a node with an error to keep pays the call.
      if (el._x !== null && (el._config & CONFIG_STAGED_ERROR || el._x._aheadError !== undefined))
        passHook(el);
    }
    if (el._config & CONFIG_STAGED_ERROR) {
      el._pendingValue = NOT_PENDING;
      el._config ^= CONFIG_STAGED_ERROR;
    }
    clearSignals(el);
  }
  const wasUninitialized = el._statusFlags & STATUS_UNINITIALIZED;
  // Capture error and pending before the compute clears them: a dropped source
  // can leave blocked dependents outside its settle walk.
  const outgoingError = el._statusFlags & STATUS_ERROR && el._x?._error;
  const wasPending = (el._statusFlags & STATUS_PENDING) !== 0;
  const outgoingPendingSources = wasPending && el._x?._pendingSources;
  // A node whose own flight parked dependents, superseded and settled
  // synchronously here, settles them here: its landing never will (#3181).
  const wasPendingSource = el._x?._pendingSources?.has(el);
  // This pass's error is staged, not published (D-41).
  let stagedErr = false;
  // A `refresh()` is quiet unless the question in flight is new (F-9), or a
  // guess's new question is still held (#4336). A held user write is not an
  // answer, so a refresh over it is a re-ask (D-34, #4887, #4888).
  let reask =
    (el._flags & REACTIVE_REASK) !== 0 &&
    !(
      (el._statusFlags & STATUS_PENDING ||
        (el._pendingValue !== NOT_PENDING && !(el._flags & REACTIVE_MANUAL_WRITE))) &&
      el._x !== null &&
      !el._x._reask
    );
  const oldcontext = context;
  context = el;
  el._depsTail = null;
  el._depGen++;
  // REACTIVE_ZOMBIE is position (on its owner's parked frame), so every per-pass
  // wipe keeps it (#3543). A release found while queued is this pass's tail's (#4474).
  const releaseAfter = el._flags & REACTIVE_RELEASE_AFTER;
  el._flags = REACTIVE_RECOMPUTING_DEPS | (el._flags & (REACTIVE_ZOMBIE | REACTIVE_REASK));
  el._time = clock;
  // The previous value: the staging, else the committed value.
  let value = el._pendingValue !== NOT_PENDING ? el._pendingValue : el._value;
  let oldHeight = el._height;
  let missedWake = false;
  // Whether this pass read the future, or for a lane's mount, this flush's staging (#3851, #4166).
  let joined = false;
  let prevTracking = tracking;
  let prevStrictRead = false;
  let prevStrictPending = false;
  {
    prevStrictRead = strictRead;
    prevStrictPending = strictPending;
    strictRead = strictPending = false;
  }
  const prevCopy = enterCopy();
  tracking = true;
  // Dev: only this pass's body is judged (D-42).
  clearThrownRead(el);
  try {
    // The sync fast arm is prod-only, so dev checks sync nodes too (D-42).
    if (!true && el._config & CONFIG_SYNC);
    else {
      // A projection self-registers its flight; its undefined return is not an answer.
      const prevInFlight = el._x?._inFlight;
      const fnResult = el._fn(value);
      const isAsyncResult = typeof fnResult === "object" && fnResult !== null;
      const selfRegistered = el._x?._inFlight !== prevInFlight;
      // Dev: a body that returned after a read threw into it caught the throw,
      // unless it returned the flight the throw became (D-42).
      if (true) checkCaughtRead(el, selfRegistered ? el._x._inFlight : fnResult);
      value = selfRegistered || !isAsyncResult ? fnResult : handleAsync(el, fnResult);
      if (!selfRegistered && !isAsyncResult) {
        if (el._x !== null) {
          el._x._inFlight = null;
          if (holdWorkHook) el._config &= ~CONFIG_ASYNC;
        }
        // A sync return is the first real answer.
        el._loading = false;
      }
    }
    // No `_x` means no status to clear. Lane work served the committed value of a
    // flight in the air keeps its pending, so a boundary keeps its fallback
    // (F-4, #4862).
    if (
      (el._statusFlags !== 0 || el._x !== null) &&
      !(laneServedHook && el._statusFlags & STATUS_PENDING && laneServedHook(el, null, true))
    )
      clearStatus(el, create);
  } catch (e) {
    const notReady = e instanceof NotReadyError;
    // An error or a loading window's unready read answers: no flight is its own
    // (D-25, `holdWork`). A pending pass keeps the last answer's.
    if (holdWorkHook && (!notReady || el._loading)) el._config &= ~CONFIG_ASYNC;
    if (notReady && el._loading) {
      // Loading window: register for the source's settle with no visible pending;
      // the loading value keeps serving and an error stays the answer.
      parkLoadingWindow(el, e);
    } else {
      // A held pass, or one that read this flush's staging, stages its error (D-41).
      if (notReady || !(stagedErr = stageError(el, e, true))) {
        if (notReady) ext(el)._blocked = true;
        notifyStatus(el, notReady ? STATUS_PENDING : STATUS_ERROR, e);
        // If no new flight re-owned self, retire the superseded flight's copies.
        if (notReady && wasPendingSource && !el._x?._inFlight) settlePendingSource(el);
      }
      // A re-park drops sources this pass no longer reaches, from dependents that
      // reached them only through here; their landing walk stops at this node and
      // would never find them (#3456, #4541). Dependents with another path keep
      // the source. A still-pending source has its own walk to come.
      if ((notReady || stagedErr) && outgoingPendingSources)
        for (const source of outgoingPendingSources)
          if (
            source !== el &&
            !(
              el._x?._pendingSources?.has(source) &&
              (source._statusFlags & STATUS_PENDING || retryReaches(el, source))
            )
          )
            settlePendingSource(el, source);
    }
  } finally {
    tracking = prevTracking;
    if (laneStageHook) laneRoot = prevMount;
    {
      strictRead = prevStrictRead;
      strictPending = prevStrictPending;
      exitCopy(prevCopy);
    }
    // Consume the missed-wake latch (#3037). The wipe below must not key off
    // DIRTY/CHECK, which `read`'s pull sets as bookkeeping.
    missedWake = (el._flags & REACTIVE_MISSED_WAKE) !== 0;
    // A mount's staged read joins it, except a nested derivation left pending: no
    // render effect reaches it, so it holds nothing (F-3, #4203, #4340).
    joined = !!(
      el._flags &
      (mount
        ? prevLane || isEffect || !(el._statusFlags & STATUS_PENDING)
          ? REACTIVE_JOINED | REACTIVE_STAGED_READ
          : 0
        : REACTIVE_JOINED)
    );
    // A verdict reader is one only while it probes or reads a memo that does
    // (D-25). Folded away without the verdict engine.
    if (observeFlightHook)
      el._config =
        (el._config & 1073709055) |
        (el._flags & REACTIVE_PROBED) |
        ((el._flags & REACTIVE_WINDOW) << 9) |
        ((el._flags & REACTIVE_AHEAD) << 11);
    // REACTIVE_DISPOSED survives: the pass may have disposed its own owner (#3621).
    // REACTIVE_FRAME_READ is this pass's verdict for the landing.
    el._flags =
      (el._flags &
        (REACTIVE_ZOMBIE |
          REACTIVE_DISPOSED |
          REACTIVE_FRAME_READ |
          REACTIVE_STAGED_READ |
          REACTIVE_LANE_READ |
          REACTIVE_SCREEN_READ |
          REACTIVE_PROBE_UNANSWERED)) |
      (create ? el._flags & REACTIVE_SNAPSHOT_STALE : 0);
    context = oldcontext;
    if (released) for (const node of released) releaseIfSettledUnobserved(node);
    // A pending pass on a `refresh()`, or one that read only committed values,
    // is a quiet re-ask (F-9). Asked only of a pending pass: the dep walk is too dear.
    if (linked && !reask && el._statusFlags & STATUS_PENDING) reask = !!passHook(el, 1);
    if (el._x !== null) el._x._reask = reask && (el._statusFlags & STATUS_PENDING) !== 0;
  }
  // A node that died during its own pass is void: unlink the reads made after
  // `dispose()`, retire its flight, and publish nothing (#3024, #3621).
  if (el._flags & REACTIVE_DISPOSED) {
    clearDeps(el);
    clearInFlight(el);
    if (attrHooks !== null) attrHooks.recomputeEnd(el, create, false, "plain", false);
    if (laneStageHook) setPassLane(prevLane);
    return;
  }
  // The lane this pass is work of. A pass that read none of the lane's world
  // has left it; a guess never leaves this way.
  let lane = !laneStageHook ? null : passLane;
  // False: the pass left the lane.
  const errored = !!el._x?._error || stagedErr;
  if (laneStageHook && lane !== null) {
    if (laneStageHook(el, lane, create, errored)) setPassLane(lane);
    else lane = null;
  }
  if (!errored) {
    // Observe (HUGE_FAN_IN): count the validated prefix here, not per link;
    // per-link counting cost ~6% on create benches.
    {
      let fanIn = 0;
      for (let d = el._deps; d !== null; d = d._nextDep) {
        fanIn++;
        if (d === el._depsTail) break;
      }
      if (fanIn >= GRAPH_SIZE_WARN_AT) noteFanIn(el, fanIn);
    }
    // INV-11 (#3330): compare against the slot this run publishes to.
    const unstaged = el._pendingValue === NOT_PENDING;
    const compareValue = !unstaged ? el._pendingValue : el._value;
    // A throwing comparator is contained in `isSame` (#2837, #4111). Replacing a
    // staged error is a new answer even at the committed value (D-41, #4403); a
    // re-pass equal to a first value staged this frame changes nothing (#4540).
    const tx = flushTransaction ?? passTx;
    const valueChanged =
      (unstaged && wasUninitialized && !isEffect) ||
      !!el._x?._shownError ||
      // In a flush that joined a hold, a recovery to the last good value is a
      // change of answer (D-41, S-53).
      !!(outgoingError && tx) ||
      !!replaced ||
      // Lane work on a node a held pass created has no screen: its value is a
      // change (F-11, #3835).
      (lane !== null && el._parent?._config & CONFIG_HELD) ||
      !isSame(el, compareValue, value);
    // Observe: a committed derived change becomes a cause for this node's subscribers.
    if (attrHooks !== null) devChanged = !!valueChanged && !el._x?._error;
    // Effects use `_equals: false`; flag and enqueue here. `!create`: `effect()`
    // runs the first pass itself.
    if (
      isEffect &&
      valueChanged &&
      // A stale lane reader that computed what it last applied owes no run.
      !(el._flags & REACTIVE_SCREEN_READ && !el._modified && Object.is(el._prevValue, value)) &&
      // A re-staging pass owes no run: the commit replays the effect (#3802).
      unstaged
    ) {
      el._modified = !el._x?._error;
      // One bound runner per effect. Lane work's run is the lane's; a first pass's
      // run is `effect()`'s to queue (#4557).
      if (!create)
        globalQueue.enqueue(
          isEffect,
          (el._boundRunEffect ??= GlobalQueue._runEffect.bind(null, el)),
          lane
        );
    }
    // A guess's own pass from the frame is its truth: staged under the lane's
    // parent, notifying only a correction (F-11, D-34). Hook first, so a build
    // without lanes folds it.
    if (
      laneOutcomeHook &&
      !create &&
      lane === null &&
      el._config & CONFIG_GUESS &&
      laneOutcomeHook(el, value, errored)
    );
    else if (errored);
    else if (valueChanged) {
      // A first pass is born held (staged, committed with the hold) when the flush
      // has joined a hold and the pass read a held node, or a verdict lane's mount
      // read this flush's staging (#3851), or a held pass created it (D-1). Any
      // other first pass publishes and re-derives if the flush parks. An effect
      // with an uncommitted staging re-stages.
      if (lane !== null) {
        // Lane work stages for the lane's next seam. An effect born held re-stages.
        if (isEffect && el._pendingValue === NOT_PENDING) el._value = value;
        else el._pendingValue = value;
        // A derivation born in the lane has no committed value until the lane lands.
        // Not an effect: marked uninitialized it re-ran without end (#4871).
        if (create && !isEffect) el._statusFlags |= STATUS_UNINITIALIZED;
      } else if (
        create
          ? !(tx !== null && (joined || creatorPass(oldcontext)?._flags & REACTIVE_JOINED))
          : isEffect && el._pendingValue === NOT_PENDING
      ) {
        // A first pass or an effect publishes directly. A memo stages even in a quiet
        // world: mid-batch pulls must see the fresh value while plain reads stay
        // committed until flush (#3009).
        el._value = value;
      } else {
        el._pendingValue = value;
        // A staged recovery keeps the error on display until it commits (D-41).
        if (outgoingError) el._x._shownError = outgoingError;
        // Born held: a read of it this tick is a read of the hold's world.
        if (create) {
          holdNode(el, tx);
          // No committed value until the landing (F-9).
          el._statusFlags |= STATUS_UNINITIALIZED;
          // A boundary that has not shown content catches it (#3540), unless the mount
          // is part of the hold itself: then it appears at the commit (D-12).
          freshHook?.(el, !mount && flushTransaction);
        }
      }
      // Lane work notifies its members as lane work (`laneDirty`).
      if (el._subs !== null) lane !== null ? laneDirty(el, lane) : insertSubs(el);
    } else if (el._height != oldHeight) {
      for (let s = el._subs; s !== null; s = s._nextSub) insertIntoHeapHeight(s._sub, dirtyQueue);
    }
    // Silent recovery to an unchanged value: re-run dependents still holding the
    // propagated error (#2949).
    if (!valueChanged && !el._x?._error) {
      if (outgoingError) settleErroredDependents(el, outgoingError);
      // Self-registration is the #3181 sweep's below.
      if (outgoingPendingSources)
        for (const source of outgoingPendingSources)
          if (source !== el) settlePendingSource(el, source);
    }
    // A synchronous settle supersedes the old landing callback, so this pass
    // sweeps (#3181). An uninitialized node with no replacement stays parked.
    if (wasPendingSource && !(el._statusFlags & (STATUS_PENDING | STATUS_UNINITIALIZED)))
      settlePendingSource(el);
  }
  // A first pass gone pending where it would have been born held is held: its
  // answer is the hold's (D-1). Under a boundary that has not shown content it
  // is held but the hold does not wait for it (F-4, #3540); an `on` re-arm owns
  // it (D-12). A derivation left pending reaches no boundary (#4542).
  if (create && lane === null && joined && el._statusFlags & STATUS_PENDING) {
    const t = flushTransaction ?? passTx;
    t && !t._lane && ((isEffect && freshHook?.(el) === CAUGHT_REARMED) || holdNode(el, t));
  }
  // Dependencies are the committed frame's until it is replaced (D-10, #3410):
  // a staging leaves the old tail for `commitPendingNode` to trim. An errored
  // pass keeps its list. An effect trims when its run applies (#3438). An
  // unchanged pass waits on the flush's verdict (#3469). Creation passes and
  // tracked effects trim now; lane work counts as unchanged.
  if (
    !el._x?._error &&
    (lane !== null || el._pendingValue === NOT_PENDING) &&
    !(isEffect && el._modified)
  ) {
    if (create || isEffect === EFFECT_TRACKED) trimStaleDeps(el);
    else heldTrim(el);
  }
  // Observe: the posture the run executed under.
  if (attrHooks !== null)
    attrHooks.recomputeEnd(
      el,
      create,
      devChanged,
      lane !== null
        ? "optimistic"
        : flushTransaction !== null || passTx !== null || (el._config & CONFIG_HELD) !== 0
          ? "held"
          : "plain",
      el._pendingValue !== NOT_PENDING
    );
  // Queue for this flush's commit: a staged value, a parked frame, or status
  // to settle. A first pass only when pending or born held. Its children stay
  // CONFIG_STAGED until then.
  if (
    el._pendingValue !== NOT_PENDING ||
    (el._x !== null && (el._x._pendingFirstChild !== null || el._x._pendingDisposal !== null)) ||
    ((el._statusFlags & (STATUS_PENDING | STATUS_UNINITIALIZED)) !== 0 &&
      (!create || (el._statusFlags & STATUS_PENDING) !== 0))
  ) {
    el._config |= CONFIG_STAGED;
    // Lane work is the lane's to reveal.
    if (lane === null) queuePendingNode(el);
  }
  if (laneStageHook) setPassLane(prevLane);
  // Missed wake: values read before the nested commit are stale, so run again.
  // Equality gates keep it convergent.
  if (missedWake) {
    enqueueSub(el);
    schedule();
  }
  // Re-derived to the staging it superseded: held readers keep their pass (#4417).
  if (keepReadersHook && superseded !== NOT_PENDING && Object.is(value, superseded))
    keepReadersHook(el);
  // A release a settle deferred past this pass (#2934, #4474).
  if (releaseAfter) releaseIfSettledUnobserved(el);
}
function updateIfNecessary(el) {
  // Never re-enter a computing node: its dep bookkeeping is live (#3037).
  // Never recompute a disposed node: it would come back alive (#2983).
  if (el._flags & (REACTIVE_RECOMPUTING_DEPS | REACTIVE_DISPOSED)) return;
  if (el._flags & REACTIVE_CHECK) {
    for (let d = el._deps; d; d = d._nextDep) {
      const dep = d._dep;
      if (dep._fn) {
        updateIfNecessary(dep);
      }
      if (el._flags & REACTIVE_DIRTY) {
        break;
      }
    }
  }
  if (el._flags & REACTIVE_DIRTY || (el._x?._error && el._time < clock && !el._x?._inFlight)) {
    recompute(el);
  }
  // The pass may have disposed the node (#3621): carry the flag. The manual
  // write mark is state, lifted only by a re-deriving pass or the commit
  // (D-8, #3612); keep the last pass's landing verdicts too.
  el._flags =
    el._flags &
    (REACTIVE_SNAPSHOT_STALE |
      REACTIVE_IN_HEAP |
      REACTIVE_IN_HEAP_HEIGHT |
      REACTIVE_ZOMBIE |
      REACTIVE_DISPOSED |
      REACTIVE_MANUAL_WRITE |
      REACTIVE_FRAME_READ |
      REACTIVE_STAGED_READ |
      REACTIVE_LANE_READ |
      REACTIVE_SCREEN_READ |
      REACTIVE_PROBE_UNANSWERED);
}
function computed(fn, options) {
  const transparent = options?.transparent ?? false;
  // `in`: an explicit `loadingValue: undefined` is a real commit #0.
  const loading = options !== null && typeof options === "object" && "loadingValue" in options;
  // One literal per tier, chosen at build time. Observe adds `_name`/`_loc` as
  // literal slots: a later write forces a hidden-class transition on every
  // node. Keep the two in sync; the dist artifact test pins the key sets.
  const self = {
    id: inheritId(options, transparent, context),
    _config:
      (transparent ? CONFIG_TRANSPARENT : 0) |
      (options?.ownedWrite ? CONFIG_OWNED_WRITE : 0) |
      (!context || options?.lazy ? CONFIG_AUTO_DISPOSE : 0) |
      (options?.sync ? CONFIG_SYNC : 0) |
      (options?._noSnapshot ? CONFIG_NO_SNAPSHOT : 0) |
      // Plumbing is observe-only; prod never carries the bit.
      (options?._plumbing ? CONFIG_PLUMBING : 0) |
      (options?._extraConfig ?? 0) |
      (snapshotCaptureActive && ownerInSnapshotScope(context) ? CONFIG_IN_SNAPSHOT_SCOPE : 0),
    _equals: options?.equals ?? isEqual,
    _disposal: null,
    _context: context?._context ?? defaultContext,
    _childCount: 0,
    _fn: fn,
    _value: loading ? options.loadingValue : undefined,
    _height: 0,
    _nextHeap: undefined,
    _prevHeap: null,
    _deps: null,
    _depsTail: null,
    _depGen: 0,
    _subs: null,
    _subsTail: null,
    _parent: context,
    _nextSibling: null,
    _prevSibling: null,
    _firstChild: null,
    _flags: options?.lazy ? REACTIVE_LAZY : REACTIVE_NONE,
    _statusFlags: loading ? 0 : STATUS_UNINITIALIZED,
    _time: clock,
    _pendingValue: NOT_PENDING,
    _notifiedAt: -1,
    _loading: loading,
    _x: null,
    // Always present for the hidden class; plumbing leaves it unset.
    _name: options?._plumbing ? undefined : (options?.name ?? "computed"),
    _loc: options?.loc,
    // Observe: windows this pass entered; a slot, never a later write.
    _devWindows: 0
  };
  if (options?.unobserved) ext(self)._unobserved = options.unobserved;
  if (options?._supersedes) self._config |= CONFIG_SUPERSEDES;
  setupComputedNode(self, options);
  return self;
}
/** Lazily allocate the cold extension. Hot paths read `el._x?._field` behind
 * `_config` bits; never call `ext()` just to store a default. */
function ext(el) {
  return (el._x ??= {
    _inFlight: null,
    _flightTeardown: null,
    _error: undefined,
    _blocked: undefined,
    _pendingSources: undefined,
    _unobserved: undefined,
    _snapshotValue: undefined,
    _pendingFirstChild: null,
    _pendingDisposal: null,
    _transaction: null,
    _reask: false,
    _flushed: NOT_PENDING,
    _flushedAt: -1,
    _q: 0,
    _shown: false,
    _marks: 0,
    _shownError: null
  });
}
/** A tracked read of a held node joins the flush to its hold (D-1), unless the
 * reader is a render effect: the frame reads the committed value outside its
 * hold's flush (D-25, `frameRead`). A node born held has no committed value,
 * so every reader joins. Under a boundary that has not shown content, a first
 * pass is the boundary's (F-4, #3540). */
/** The pass whose plain read made this flush adopt a hold: a window it then
 * opens can give the hold back (#4777). */
let adopter = null;
function setAdopter(c) {
  adopter = c;
}
function joinPass(c, el) {
  if (observeFlightHook && !flushTransaction) ((adopter = c), (adopted.length = 0));
  c._flags |= REACTIVE_JOINED;
  // A new memo's first pass is the hold's work but not the tick's: the render
  // effect reading it decides (F-3, #3540).
  if (el._statusFlags & STATUS_UNINITIALIZED || c._type !== EFFECT_RENDER)
    joinPassTx(txOf(el), (!c._type && c._statusFlags & STATUS_UNINITIALIZED) || freshHook?.(c));
}
/** `joinPass` into hold `t` (a folded guess, D-34); never a render effect. */
function joinPassAt(c, t) {
  c._flags |= REACTIVE_JOINED;
  joinPassTx(t, (!c._type && c._statusFlags & STATUS_UNINITIALIZED) || freshHook?.(c));
}
/** A frame reader of a node another hold holds is served the committed value,
 * publishes mainline and re-derives at that landing (D-25). Not one born
 * into that hold (#4010), nor a flight whose inputs already show: that would
 * tear (F-1, #3305). Lane work is a frame reader too: a lane sees the screen
 * plus its own guesses (F-6). */
function frameRead(c, el) {
  // Frame readers: render effects, verdict readers and lane work (D-25); a
  // boundary's tree and output too (#4283). Not a mount's memo: the hold's own
  // later passes read it (D-1). Not a pass with only an inherited posture
  // reading the hold's own write (D-25, #4544).
  const verdict = observeFlightHook && c._config & CONFIG_VERDICT;
  const lane = laneStageHook && passLane !== null;
  // A render effect a probing pass is mounting is born held on the hold's
  // staging or flight, whatever else it read (D-25, S-46, S-58). Not lane work
  // reading a held node that is not pending. Folded away without lanes.
  if (
    laneStageHook &&
    c._config & CONFIG_MOUNT_AHEAD &&
    c._statusFlags & STATUS_UNINITIALIZED &&
    (!lane || el._statusFlags & STATUS_PENDING)
  ) {
    joinPass(c, el);
    return false;
  }
  if (
    !(verdict && holdStagingHook(c, el)) &&
    !(
      (!lane || (c._statusFlags & STATUS_UNINITIALIZED && el._statusFlags & STATUS_PENDING)) &&
      !verdict &&
      (!(c._config & (EFFECT_RENDER << 23)) || el._config & CONFIG_INPUTS_PUBLISHED)
    )
  ) {
    const t = txOf(el);
    // Lane work and verdict readers are never the hold's pass, except a lane
    // re-based over this hold's answer: it reads that staging without joining
    // (F-1, F-7, D-34; #4536, #4576, #4643).
    if (lane && passLane.over && resolveTx(passLane.over) === t && !verdict) return false;
    if (!((!lane && t === flushTransaction && !verdict) || bornInto(c, t))) {
      staleReader(c, t);
      // They no longer wait on `el`'s flight.
      if (lane || verdict) laneServedHook(c, el);
      return true;
    }
  }
  joinPass(c, el);
  return false;
}
/** Born into `t`: held and never committed, or a member of a frame its held
 * pass built. It reads `t`'s values in any flush and keeps staging (#4010). */
function bornInto(c, t) {
  return (
    (c._statusFlags & STATUS_UNINITIALIZED || c._config & CONFIG_HELD_FRAME) &&
    c._config & CONFIG_HELD &&
    txOf(c) === t
  );
}
/** With `tail`, this pass's reads only. */
function hasDep(c, f, tail) {
  for (let d = c._deps; d !== null; d = d === tail ? null : d._nextDep) if (f(d._dep)) return 1;
}
/** A staging a verdict pass reads for the first time is content it opens,
 * not a value it keeps (D-25, S-46, #4806). */
let fresh;
function readsFresh(c, el) {
  return (
    el._pendingValue !== NOT_PENDING && c._config & CONFIG_VERDICT && !hasDep(c, d => d === el)
  );
}
/** The computed a creation under `owner` belongs to, or null. */
function creatorPass(owner) {
  return owner !== null && owner._root ? owner._parentComputed : owner;
}
/** Whether the passes running around `p` read the future or a staging: a pass
 * they created has no screen (#4341). */
function creatorReads(p, born) {
  return (p = creatorPass(p._parent))?._flags & REACTIVE_RECOMPUTING_DEPS
    ? creatorReads(p, born | p._flags)
    : born & (REACTIVE_JOINED | REACTIVE_STAGED_READ);
}
/** Park the previous pass's children and disposal list until this node's
 * commit replaces them. */
function parkChildren(el) {
  markDisposal(el);
  const x = ext(el);
  x._pendingDisposal = el._disposal;
  x._pendingFirstChild = el._firstChild;
  el._disposal = null;
  el._firstChild = null;
  el._childCount = 0;
}
/** One literal with every effect field, so V8 sees the full shape at construction. */
function createEffectNode(fn, effectFn, errorFn, type, options) {
  const transparent = options?.transparent ?? false;
  // See `computed()` for the observe slots.
  const self = {
    id: inheritId(options, transparent, context),
    _config:
      // CONFIG_FRAME for a render effect.
      (type << 23) |
      (transparent ? CONFIG_TRANSPARENT : 0) |
      (options?.ownedWrite ? CONFIG_OWNED_WRITE : 0) |
      (options?.sync ? CONFIG_SYNC : 0) |
      (options?._extraConfig ?? 0) |
      (snapshotCaptureActive && ownerInSnapshotScope(context) ? CONFIG_IN_SNAPSHOT_SCOPE : 0),
    _equals: false,
    _disposal: null,
    _context: context?._context ?? defaultContext,
    _childCount: 0,
    _fn: fn,
    _value: undefined,
    _height: 0,
    _nextHeap: undefined,
    _prevHeap: null,
    _deps: null,
    _depsTail: null,
    _depGen: 0,
    _subs: null,
    _subsTail: null,
    _parent: context,
    _nextSibling: null,
    _prevSibling: null,
    _firstChild: null,
    _flags: REACTIVE_LAZY,
    _statusFlags: STATUS_UNINITIALIZED,
    _time: clock,
    _pendingValue: NOT_PENDING,
    _notifiedAt: -1,
    _loading: false,
    _modified: false,
    _prevValue: undefined,
    _effectFn: effectFn,
    _errorFn: errorFn,
    _cleanup: undefined,
    _type: type,
    _x: null,
    _name: options?.name ?? "effect",
    _loc: options?.loc,
    _devWindows: 0
  };
  // Effects share one status notifier (`statusNotifierOf`): a per-node one
  // forced an `_x` on every effect (+23% creation).
  if (options?.unobserved) ext(self)._unobserved = options.unobserved;
  // Dev only: observe mangles `_` option names (CONFIG_WIDE).
  if (options?._wide) self._config |= CONFIG_WIDE;
  setupComputedNode(self, lazyOptions);
  return self;
}
/** The shared effect status notifier, installed by effect.ts. */
let effectStatusNotify = null;
function setEffectStatusNotify(fn) {
  effectStatusNotify = fn;
}
/** Effect nodes get the shared notifier; presence also marks a display consumer. */
function statusNotifierOf(el) {
  return el._type ? effectStatusNotify : null;
}
const lazyOptions = { lazy: true };
function setupComputedNode(self, options) {
  self._prevHeap = self;
  const parent = context?._root ? context._parentComputed : context;
  if (context && context._config & CONFIG_CHILDREN_FORBIDDEN) {
    emitDiagnostic({
      code: "PRIMITIVE_IN_FORBIDDEN_SCOPE",
      kind: "lifecycle",
      severity: "error",
      message: PRIMITIVE_IN_FORBIDDEN_SCOPE_MESSAGE,
      ownerId: context.id,
      ownerName: context._name
    });
    throw new Error(PRIMITIVE_IN_FORBIDDEN_SCOPE_MESSAGE);
  }
  assertNotInEffectCallback();
  if (context) linkChild(context, self);
  DEV.hooks.onOwner?.(self);
  if (parent) self._height = parent._height + 1;
  wireExternalSourceHook?.(self);
  !options?.lazy && recompute(self, true);
  if (snapshotCaptureActive && !options?.lazy) {
    if (!(self._statusFlags & STATUS_PENDING) && !(self._config & CONFIG_NO_SNAPSHOT))
      takeSnapshot(self, self._value);
  }
}
function signal(v, options) {
  // See `computed()`. Observe adds `_name`, `_loc` and `_owner` (for owner paths).
  const s = {
    _equals: options?.equals ?? isEqual,
    _config:
      (options?.ownedWrite ? CONFIG_OWNED_WRITE : 0) |
      (options?._noSnapshot ? CONFIG_NO_SNAPSHOT : 0),
    _value: v,
    _subs: null,
    _subsTail: null,
    _time: clock,
    _pendingValue: NOT_PENDING,
    _notifiedAt: -1,
    _x: null,
    _name: options?.name ?? "signal",
    _loc: options?.loc,
    _owner: null
  };
  s._internal = false;
  if (options?.unobserved) ext(s)._unobserved = options.unobserved;
  if (snapshotCaptureActive && !(s._config & CONFIG_NO_SNAPSHOT)) takeSnapshot(s, v);
  return s;
}
function isEqual(a, b) {
  return a === b;
}
/** A store slot node in one literal: target and key as back-refs, the wrap
 * cache (`px`/`pxv`) and accessor verdict pre-shaped. No options, no `_x`,
 * no expandos. Observe adds label and location slots. */
function slotSignal(v, equals, host, key, acc) {
  const s = {
    _equals: equals,
    _config: CONFIG_OWNED_WRITE | CONFIG_SLOT_NODE,
    _value: v,
    _subs: null,
    _subsTail: null,
    _time: clock,
    _pendingValue: NOT_PENDING,
    _notifiedAt: -1,
    _x: null,
    _host: host,
    _key: key,
    acc,
    px: undefined,
    pxv: undefined,
    _name: "signal",
    _loc: undefined,
    _owner: null
  };
  s._internal = false;
  return s;
}
/** Dev: when set to a component name, a reactive read outside a nested
 * tracking scope warns. Managed by `untrack(fn, strictReadLabel)`. */
/** The verdict windows' read, set while a window is open. */
let verdict = null;
function setVerdict(fn) {
  verdict = fn;
}
let strictRead = false;
/** Dev: the label a plain `untrack` keeps from its strict scope, so a pending
 * read of a memo the same body created throws `PENDING_ASYNC_UNTRACKED_READ`
 * (it would loop). A memo created outside is a sanctioned read (D-23). */
let strictPending = false;
let untrackDepth = 0;
/** Dev: > 0 while user code that is imperative by construction runs (effect
 * callback, action slice), so the post-await check does not blame the caller. */
let callbackDepth = 0;
function enterCallback() {
  callbackDepth++;
}
function exitCallback() {
  callbackDepth--;
}
/** Dev: > 0 while teardown runs cleanups: their writes are imperative too, so
 * the owned-scope guard stands down (#4165). No try/finally here (prod), so
 * dev.ts resets it on the next microtask. */
let disposalDepth = 0;
function enterDisposal() {
  disposalDepth++;
}
function exitDisposal() {
  disposalDepth--;
}
function resetDisposalDepth() {
  disposalDepth = 0;
}
/** Dev: the event current when the strict-read window opened, or `ANY_EVENT`.
 * A handler dispatched synchronously from an effect callback is a snapshot,
 * so a read under a different event is outside the window (#4235). Read
 * once per queue: the getter is costly under jsdom. */
let strictReadEvent;
// `?? null`: no current event must not read as a missing argument.
const dispatchingEvent = () => globalThis.event ?? null;
function setStrictRead(v, event = ANY_EVENT) {
  const prev = strictRead;
  strictRead = v;
  strictReadEvent = event;
  return prev;
}
/** Dev-only: runs `fn` inside the strict-read window labelled `label`, tracking
 * unchanged. Compiled dev DOM output wraps event handler expressions in it
 * (#1742). Production builds just call `fn`. */
function strictReadTracked(fn, label = EVENT_HANDLER_READ) {
  const prevStrictRead = strictRead;
  const prevStrictReadEvent = strictReadEvent;
  strictRead = label;
  strictReadEvent = ANY_EVENT;
  try {
    return fn();
  } finally {
    strictRead = prevStrictRead;
    strictReadEvent = prevStrictReadEvent;
  }
}
function strictReadActive() {
  return !!strictRead && (strictReadEvent === ANY_EVENT || dispatchingEvent() === strictReadEvent);
}
/**
 * Runs `fn` outside of any reactive tracking — reads inside `fn` will not
 * subscribe the current scope. Returns whatever `fn` returns.
 *
 * Use `untrack` inside a memo or effect when you need to read a signal once
 * without making the surrounding computation depend on its future changes.
 *
 * In a component body, `untrack` marks a read as a deliberate one-time
 * snapshot and silences `STRICT_READ_UNTRACKED`. Use it only when the value
 * must never update and is never downstream of async: an async-derived value
 * can still be pending (the read throws) or mid-transition (the snapshot is
 * the old committed value). When it should follow its source, move the read
 * to where the value is used (JSX, a memo, an effect's compute, the event
 * handler, or an accessor passed to the hook that needs it) instead.
 *
 * `untrack` stops dependency tracking, not async settlement. Reading an
 * uninitialized async source still suspends the owning computation; it runs
 * again when that source first resolves. Afterward, the untracked read is a
 * snapshot and does not subscribe the computation to later changes.
 *
 * If the first read must return immediately, give the async source a
 * `loadingValue` before reading it with `untrack`. `isPending` and `latest`
 * inspect updates to an existing value; neither supplies a missing initial
 * value.
 *
 * Pass a `strictReadLabel` string to enable a dev-mode warning: any reactive
 * read inside `fn` that isn't inside a nested tracking scope will log a
 * warning naming the label.
 *
 * @example
 * ```ts
 * createEffect(
 *   () => trigger(),                 // tracks `trigger` only
 *   () => {
 *     const snapshot = untrack(() => state); // read once, untracked
 *     log(snapshot);
 *   }
 * );
 *
 * const color = createMemo<string>(() => fetchColor(), { loadingValue: "gray" });
 * const initialColor = untrack(color); // "gray" while the fetch is unresolved
 * ```
 */
function untrack(fn, strictReadLabel) {
  const prevTracking = tracking;
  const prevStrictRead = strictRead;
  const prevStrictReadEvent = strictReadEvent;
  const prevStrictPending = strictPending;
  tracking = false;
  {
    if (strictReadLabel === undefined) {
      if (strictReadActive()) strictPending = strictRead;
    } else strictPending = false;
    strictRead = strictReadLabel || false;
    strictReadEvent = ANY_EVENT;
    untrackDepth++;
  }
  try {
    if (externalUntrackHook) return externalUntrackHook(fn);
    return fn();
  } finally {
    tracking = prevTracking;
    {
      strictRead = prevStrictRead;
      strictReadEvent = prevStrictReadEvent;
      strictPending = prevStrictPending;
      untrackDepth--;
    }
  }
}
/** > 0 while `runRefCallback` runs: a ref born into a held node's hold may
 * read its staging (#4767, F-7). */
let refCallbackDepth = 0;
/** A write, refresh, action call or `onFlush` here is an effect callback's
 * imperative work, not a computation's. */
function inRefCallback() {
  return (
    refCallbackDepth > 0 ||
    (inEffectCallback && context !== null && context === effectCallbackOwner)
  );
}
/** Runs a compiled `ref()` callback untracked in the creating pass's context
 * (#4761); in dev, creating a primitive there is PRIMITIVE_IN_EFFECT_CALLBACK.
 *
 * @internal */
function runRefCallback(fn) {
  refCallbackDepth++;
  try {
    if (true) {
      const prev = inEffectCallback,
        prevEvent = effectCallbackEvent,
        prevOwner = effectCallbackOwner;
      setEffectCallback(true, ANY_EVENT, context);
      try {
        untrack(fn, "a ref callback");
      } finally {
        setEffectCallback(prev, prevEvent, prevOwner);
      }
    }
  } finally {
    refCallbackDepth--;
  }
}
/** A bookkeeping read on behalf of no node: it records no re-run link (#3528)
 * and no transaction entry (#3540) on `context`. */
let spectating = false;
function prepareComputed(comp, refresh) {
  if (comp._flags & REACTIVE_LAZY) {
    comp._flags &= ~REACTIVE_LAZY;
    recompute(comp, true);
  } else if (comp._flags & REACTIVE_DISPOSED) {
    // Two disposal lifecycles share the flag (#3024): a dormant auto-dispose node
    // wakes on read (pay-for-use); an owner-disposed one is dead and serves its
    // last committed value, since recomputing runs user code in a torn-down tree.
    if (comp._config & CONFIG_AUTO_DISPOSE) {
      const parent = comp._parent;
      if (parent !== null) {
        // A dormant node was off the chain when its owner died (#3024).
        if (parent._flags & REACTIVE_DISPOSED) {
          comp._config &= ~CONFIG_AUTO_DISPOSE;
          return;
        }
        // A zombie never left its chain: relinking would put it on the live one too.
        if (!(comp._flags & REACTIVE_ZOMBIE)) linkChild(parent, comp);
      }
      recompute(comp, true);
    }
  } else;
}
/** Does reader `c` see a staged node's committed value? Read's fast block
 * carries the same test inline. An untracked read and a frame-only reader
 * (createTrackedEffect / onSettled, D-29) see the committed frame. */
function readerSeesCommitted(el, c) {
  return !c || el._pendingValue === NOT_PENDING || c._config & CONFIG_CHILDREN_FORBIDDEN;
}
/** Set when a node is staged outside a flush, cleared when a flush begins:
 * one module boolean keeps the F-10 arm free inside a flush. */
let unflushedStaged = false;
function markUnflushedStaged() {
  unflushedStaged = true;
}
/** A write is visible from the flush that carries it (F-10): outside a flush
 * a staged node serves its committed value. Promoted writes are exempt. */
function unflushedValue(el) {
  if (globalQueue._running || el._pendingValue === NOT_PENDING || el._config & CONFIG_PROMOTED)
    return NOT_PENDING;
  // A held node was flushed and parked: not unflushed unless rewritten since,
  // then the flushed staging is what the world sees until the next flush.
  if (el._config & CONFIG_HELD) return el._x._flushedAt === clock ? el._x._flushed : NOT_PENDING;
  return el._value;
}
/** The value a signal had when this tick began writing it (F-10). */
function tickBase(el) {
  return el._config & CONFIG_HELD && el._x?._flushedAt === clock ? el._x._flushed : el._value;
}
/** A held node rewritten outside a flush keeps the last flushed staging until
 * the flush that carries the rewrite (F-10); the clock stamps the stash. */
function stashFlushed(el) {
  const x = el._x;
  if (x._flushedAt !== clock) {
    // D-8: the flush checks whether the rewrite nets (verdict.ts `revert`).
    // Once per node per flush: the stamp below dedupes it.
    if (revertHook) heldRewrites.push(el);
    x._flushed = el._pendingValue;
    x._flushedAt = clock;
    unflushedStaged = true;
  }
}
const promotedWrites = [];
/** A derivation served the committed value because of an unflushed write runs
 * again in the flush that carries it: it linked after the write walked (F-10, #3337). */
function markLateLinker(c) {
  // A direct enqueue here would be wiped by the pass's flag reset.
  c._flags |= REACTIVE_MISSED_WAKE;
  return true;
}
function resyncUnflushedCompanions() {
  unflushedStaged = false;
  // Length-guarded so the common flush allocates nothing.
  if (promotedWrites.length !== 0) {
    for (const el of promotedWrites) el._config &= ~CONFIG_PROMOTED;
    promotedWrites.length = 0;
  }
}
function pullComputed(owner, c) {
  if (owner._height >= dirtyQueue._min || owner._flags & REACTIVE_CHECK) {
    markNode(c);
    markHeap(dirtyQueue);
    updateIfNecessary(owner);
  }
  // A fresh-pull waiter (awaitable refresh) recomputes a dirty source inline:
  // it must park on the re-ask, never read the pre-re-ask value as settled.
  else if (c._config & CONFIG_FRESH_READ) updateIfNecessary(owner);
  const height = owner._height;
  // parent check is shallow, might need to be recursive
  if (owner._parent === c) owner._flags |= REACTIVE_OWNER_READS;
  else if (height >= c._height) liftOwned(c, (c._height = height + 1));
}
/** Dev: every read's diagnostics. `nodeValue`'s first landing serves a store
 * leaf without `read`, so it calls this too. */
function devRead(el) {
  DEV.hooks.onRead?.(el);
  if (!tracking) checkForeignRead();
  if (asyncTailFlights !== 0 && !tracking && untrackDepth === 0)
    checkPostAwaitRead(
      el,
      el,
      undefined,
      el._name,
      (el._statusFlags & (STATUS_PENDING | STATUS_UNINITIALIZED)) ===
        (STATUS_PENDING | STATUS_UNINITIALIZED)
    );
}
/** A first answer staged this flush and re-asked in it (D-22, F-9): a memo
 * with no value derives from that answer once, as a flush of the answer
 * alone would have let it, then waits on the re-ask. One served pass a
 * flush (`_served`), or a pass that throws anyway re-runs without end. Not a
 * render effect, lane work, a verdict lane's pass (D-25) or a held node. */
function servesStaged(c, el, owner) {
  return (
    passLane === null &&
    c._statusFlags & owner._statusFlags & STATUS_UNINITIALIZED &&
    el._pendingValue !== NOT_PENDING &&
    !(el._config & (CONFIG_HELD | CONFIG_STAGED_ERROR)) &&
    !c._type &&
    (c._flags & REACTIVE_MISSED_WAKE ||
      (c._pendingValue === NOT_PENDING &&
        c._x?._served !== clock &&
        (!(c._statusFlags & STATUS_PENDING) ||
          c._x?._pendingSources?.delete(owner._x._error.source)) &&
        ((ext(c)._served = clock), (c._flags |= REACTIVE_MISSED_WAKE))))
  );
}
/** @internal Compiler output (`@solidjs/web`): truthy while the running pass is
 * a stale reader of an open hold (D-9). A grouped stateful prop binding then
 * skips an unchanged write, so a sibling's re-run does not put the committed
 * value back over the field (#4872). */
function heldRead() {
  return context._flags & REACTIVE_FRAME_READ;
}
// Typed `any` so `el` needs no cast alias: an alias the minifier keeps.
function read(el) {
  devRead(el);
  let c = context;
  if (c?._root) c = c._parentComputed;
  // The verdict windows' read (verdict.ts), off this path. It wakes a lazy or
  // dormant memo outside the window (#4366).
  if (verdict !== null) {
    // Dev: the window's throws are this read's throws (D-42).
    {
      try {
        return verdict(el, c);
      } catch (e) {
        noteThrownRead(el, e);
        throw e;
      }
    }
    return verdict(el, c);
  }
  if (typeof el._fn === "function") prepareComputed(el);
  if (
    !el._fn &&
    el._x?._snapshotValue === undefined &&
    !snapshotCaptureActive &&
    (!unflushedStaged || el._pendingValue === NOT_PENDING) && // F-10, see readNodeFast
    !(el._config & CONFIG_OVERRIDE) && // lanes: the slow path's one arm
    !strictRead
  ) {
    if (c) {
      if (laneStageHook) fresh = tracking && readsFresh(c, el);
      if (tracking) link(el, c);
      // A pass that reads a held value joins its hold, tracked or not (D-1): a
      // mainline derivation of a held write would tear (F-1). A first pass that
      // did is born held (F-7). Frame-only readers (D-29) and frame readers
      // (`frameRead`, D-25) see the committed value instead.
      if (el._config & CONFIG_HELD && !(c._config & CONFIG_CHILDREN_FORBIDDEN)) {
        if (frameRead(c, el)) {
          return el._value;
        }
      }
    }
    if (!c || el._pendingValue === NOT_PENDING || c._config & CONFIG_CHILDREN_FORBIDDEN) {
      return el._value;
    }
    // Writes that netted are no proposal (D-8): not a read of the future, nor a
    // stale verdict read to re-run at the landing (#4485). `Object.is` too, for
    // signed zeros; `===` keeps an accepted NaN rewrite a proposal.
    const netted = el._pendingValue === el._value && Object.is(el._pendingValue, el._value);
    // Lane work over a folded lane, once the frame has a transaction, reads the
    // screen (D-34).
    if (
      netted ||
      (((observeFlightHook && c._config & CONFIG_VERDICT) || (laneStageHook && passLane?.folded)) &&
        stagedScreen(c, el))
    ) {
      return el._value;
    }
    stagedRead(c);
    return el._pendingValue;
  }
  if (
    el._statusFlags & STATUS_PENDING &&
    ((strictReadActive() && strictRead) || (strictPending && el._parent === context))
  )
    throwPendingUntrackedRead((strictReadActive() && strictRead) || strictPending, {
      ownerId: c?.id,
      ownerName: c?._name,
      nodeName: el?._name
    });
  // Decided after the pull below: a pull can join (D-25).
  let committed = false;
  // Only a re-read retries an errored source (F-9).
  let reread;
  if (c) {
    if (laneStageHook) fresh = tracking && readsFresh(c, el);
    if (tracking) {
      reread = link(el, c);
      if (el._fn) {
        pullComputed(el, c);
        // A memo over a verdict reader's value reads ahead as if it probed inline
        // (F-6, #4050). Not an effect, and not the hold's own work (born in its
        // world, or over its own staged pass): there a pending read throws (D-25,
        // #4504, #3884). The posture lasts until a pass completes having read no
        // verdict reader (D-25, #4408). A render effect never committed reading a
        // settled verdict reader's value reads ahead too (F-6, #4544).
        if (
          observeFlightHook &&
          (c._statusFlags & STATUS_UNINITIALIZED
            ? c._config & CONFIG_HELD || flushTransaction
            : c._config & CONFIG_HELD_FRAME)
        ) {
          if (
            c._type === EFFECT_RENDER &&
            el._config & CONFIG_VERDICT &&
            !(el._statusFlags & (STATUS_PENDING | STATUS_UNINITIALIZED))
          )
            c._flags |= REACTIVE_AHEAD;
          // A sync verdict memo is display while only the display reads it (D-25,
          // #4776). The hold's own work reading it makes it the hold's: it re-derives
          // now on the hold's writes (`holdStaging`). A memo that probes in its own
          // body stays the screen's (F-6, #4544). An async derivation's read seats
          // nothing (S-51): the lane and the mount it gates would wait on each other
          // (#4965).
          if (
            (el._config & (CONFIG_VERDICT | CONFIG_FRAME | CONFIG_ASYNC | CONFIG_OWN_PROBE)) ===
              CONFIG_VERDICT &&
            !(c._config & CONFIG_ASYNC)
          ) {
            const t = resolveTx(c._config & CONFIG_HELD ? txOf(c) : flushTransaction),
              x = ext(el);
            if (x._readIn !== t) {
              x._readIn = t;
              recompute(el);
            }
          }
        }
        if (observeFlightHook && !c._type)
          c._flags |=
            el._config &
            CONFIG_VERDICT &
            (el._statusFlags & STATUS_PENDING &&
            (c._config & CONFIG_STAGED || c._statusFlags & STATUS_UNINITIALIZED)
              ? c._config
              : -1);
      }
    }
    // A node born into the future has no committed value: every reader derives
    // from the future, a render effect included (D-1, F-7).
    if (
      el._config & CONFIG_HELD &&
      !(el._config & CONFIG_OVERRIDE) &&
      !(c._config & CONFIG_CHILDREN_FORBIDDEN)
    ) {
      if (el._statusFlags & STATUS_UNINITIALIZED) joinPass(c, el);
      else committed = frameRead(c, el);
    }
  }
  // After the pull, so the node is current. A pending member throws as any.
  if (laneReadHook && el._config & CONFIG_OVERRIDE) {
    const v = laneReadHook(c, el);
    if (v !== NOT_PENDING) return v;
  }
  // Ahead of the snapshot serve: hydrating or not, a body's direct read is
  // wrong the same way (#3675).
  if (strictRead && strictReadActive())
    warnStrictReadUntracked(strictRead, {
      ownerId: c?.id,
      ownerName: c?._name,
      nodeName: el?._name
    });
  // A hydration claim is served the value the server rendered with, and
  // replays at the scope's release. An errored node still throws below.
  if (snapshotCaptureActive && c && c._config & CONFIG_IN_SNAPSHOT_SCOPE) {
    const sv = el._x?._snapshotValue;
    if (sv !== undefined && !(el._statusFlags & STATUS_ERROR)) {
      const snapshot = sv === NO_SNAPSHOT ? undefined : sv;
      const current = el._pendingValue !== NOT_PENDING ? el._pendingValue : el._value;
      if (current !== snapshot) c._flags |= REACTIVE_SNAPSHOT_STALE;
      return snapshot;
    }
  }
  // A stale reader of a held flight is served the committed value and does
  // not go pending (D-9, D-25); so is lane work reading a flight not its own
  // (D-25), and a verdict reader (`[isPending(x), x()]` is `[true, stale]` in
  // either order, D-23). Not a flight committed beneath inputs on screen
  // (F-1), nor a reader born into its transaction (#4010).
  if (
    laneStageHook &&
    el._statusFlags & STATUS_PENDING &&
    !committed &&
    !(el._config & CONFIG_OVERRIDE) &&
    !(el._statusFlags & STATUS_UNINITIALIZED)
  ) {
    // A verdict reader first, routed or not (D-25, #4661). A reader with no
    // committed value sees the flight pending, so a boundary a lane mounts
    // shows its fallback (F-7). A mount a probing pass opened is born held
    // (S-46, S-58).
    if (
      c !== null &&
      c._config & CONFIG_VERDICT &&
      !(c._config & CONFIG_MOUNT_AHEAD && c._statusFlags & STATUS_UNINITIALIZED)
    )
      committed = observeFlightHook(c, el);
    else if (passLane !== null && (!(c?._statusFlags & STATUS_UNINITIALIZED) || bornOfLaneRerun(c)))
      // Lane work is served the committed value unless it read this flush's
      // staging (F-2, S-57).
      committed = laneServedHook(c, el, true);
  }
  if (el._statusFlags & STATUS_PENDING && !committed) {
    // Untracked, an uninitialized node has nothing to serve and throws. A
    // frame-only reader sees the frame (D-29).
    if (c) {
      if (c._config & CONFIG_CHILDREN_FORBIDDEN) {
        const message =
          "[PENDING_ASYNC_FORBIDDEN_SCOPE] Reading a pending async value inside createTrackedEffect or onSettled serves the committed value (and throws while uninitialized). " +
          "Use createEffect instead which supports async-aware reactivity.";
        reportDiagnostic(
          emitDiagnostic(
            {
              code: "PENDING_ASYNC_FORBIDDEN_SCOPE",
              kind: "async",
              severity: "warn",
              message,
              ownerId: c.id,
              ownerName: c._name,
              nodeName: el?._name
            },
            c
          )
        );
      }
      if (!(c._config & CONFIG_CHILDREN_FORBIDDEN) || el._statusFlags & STATUS_UNINITIALIZED) {
        // Out of line, so `read` gains a call only where a pending read throws (D-22, F-9).
        if (observeFlightHook && servesStaged(c, el, el)) {
          return el._pendingValue;
        }
        // An untracked read of a pending node still re-runs its reader when it
        // settles, unless nobody reads (`spectating`, #3528) or it reads itself.
        if (!tracking && !spectating && el !== c) link(el, c);
        noteThrownRead(el, el._x?._error);
        throw el._x?._error;
      }
    } else if (el._statusFlags & STATUS_UNINITIALIZED) {
      throw el._x?._error;
    }
  }
  // An errored derive throws for every reader, memo parity (D-41, #2897).
  if (el._fn && el._statusFlags & STATUS_ERROR) {
    // Only a re-read retries an errored source (F-9): tracked, by a pass whose
    // last pass read it (#5002), on a later cycle, and not by a zombie, whose
    // retry livelocked the live frame's readers (S-58, #4405).
    if (reread && el._time < clock && !(el._flags & REACTIVE_ZOMBIE)) {
      recompute(el);
      return read(el);
    }
    // A staged error is this flush's answer (D-41): a reader served the staging
    // throws it and stages its own; one served the screen throws the shown one
    // (#4404).
    if (!committed && el._config & CONFIG_STAGED_ERROR) serve(el, c);
    noteThrownRead(el, el._x?._error);
    throw el._x?._error;
  }
  // A reader served the screen throws the shown error (D-41).
  if (committed || !c) throwShown(el);
  const value = serve(el, committed ? null : c);
  if (
    !c &&
    typeof el._fn === "function" &&
    el._config & CONFIG_AUTO_DISPOSE &&
    !(el._statusFlags & STATUS_PENDING) &&
    !el._subs
  ) {
    // Deferred: an inline unobserved() made consecutive untracked reads answer
    // differently with no write between (#3078). The flush sweep reclaims it.
    dormantNodes.add(el);
    schedule();
  }
  return value;
}
/** A first pass a guess lane's re-run creates stands where the memo it
 * replaces stood, so lane work's read of a parent's flight is the screen's
 * for it too (#4824). A boundary's structure (F-7, #3835) and a mount a
 * display-ahead read opens (S-58, D-25) still see the flight. */
function bornOfLaneRerun(c) {
  const p = c._parent;
  return (
    passLane._parent?._verdict !== passLane &&
    p?._fn &&
    !((p._statusFlags & STATUS_UNINITIALIZED) | (p._config & CONFIG_FRAME))
  );
}
/** A reader served the screen reads a shown error until its replacement
 * commits (D-41). */
function throwShown(el) {
  if (el._x?._shownError) {
    noteThrownRead(el, el._x._shownError);
    throw el._x._shownError;
  }
}
/** The value reader `c` (null: untracked) is served from `el`; read's fast
 * block keeps its trivial ternary for speed. */
function serve(el, c) {
  // A node born staged has no committed value: an untracked, bookkeeping or
  // frame-only reader has nothing to serve and holds (F-9, D-29). A ref born
  // into this node's hold reads it as the hold's own work (D-25, #4767); one
  // mounted outside must not see it ahead of the commit (F-7).
  if (
    el._pendingValue !== NOT_PENDING &&
    el._statusFlags & STATUS_UNINITIALIZED &&
    (!c || spectating || c._config & CONFIG_CHILDREN_FORBIDDEN)
  ) {
    if (refCallbackDepth > 0) {
      const pass = creatorPass(context);
      if (pass && el._config & CONFIG_HELD && bornInto(pass, txOf(el))) {
        return el._pendingValue;
      }
    }
    {
      const e = new NotReadyError(null);
      noteThrownRead(el, e);
      throw e;
    }
  }
  const u = c && unflushedStaged ? unflushedValue(el) : NOT_PENDING;
  if (u !== NOT_PENDING) {
    markLateLinker(c);
    return u;
  }
  if (
    readerSeesCommitted(el, c) ||
    (observeFlightHook &&
      c._config & CONFIG_VERDICT &&
      !(el._statusFlags & STATUS_UNINITIALIZED) &&
      stagedScreen(c, el) &&
      !throwShown(el))
  ) {
    return el._value;
  }
  // Writes that netted are no proposal (D-8, #4452), as in read's fast block.
  if (
    !strictRead ||
    el._fn ||
    el._pendingValue !== el._value ||
    !Object.is(el._pendingValue, el._value)
  )
    stagedRead(c, el);
  if (el._config & CONFIG_STAGED_ERROR) {
    noteThrownRead(el, el._pendingValue);
    throw el._pendingValue;
  }
  return el._pendingValue;
}
/** A pass read a staging of this flush: the seam decides (D-25). Lane work
 * too: if the flush parks, the pass re-derives on the committed world and
 * its lane waits that round, so the held write never shows through it. A
 * folded lane only before the frame has a transaction (D-34). Not a node
 * born staged: a re-run would read the same staging every round (#3969). */
function stagedRead(c, el) {
  if (!(el?._statusFlags & STATUS_UNINITIALIZED)) {
    // Lane work that was served a flight's committed value now reads a staging:
    // it throws (#4859).
    if (laneServedHook && passLane !== null) laneServedHook(c, null);
    c._flags |= REACTIVE_STAGED_READ;
    // A verdict effect's staging read is the frame's, re-derived once if the
    // flush parks (D-22, D-25). A memo is not a leaf (`frameRead`).
    if (laneSeamsHook && (passLane !== null || (c._type && c._config & CONFIG_VERDICT)))
      stagedReaders.push(c);
  }
}
/** A verdict reader's plain read of a staged node sees the screen once the
 * frame has a transaction, and re-derives at its landing (D-25, #3322).
 * Before that the seam answers. A node born into the future (F-7), a reader
 * born into the transaction (#4010) or into the hold (D-25) has no screen. */
function stagedScreen(c, el) {
  if (
    flushTransaction === null ||
    bornInto(c, flushTransaction) ||
    (el?._config & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD ||
    // This flush's own write read by a pass with an inherited posture is the
    // frame's derivation (D-25, #4544).
    (observeFlightHook && holdStagingHook(c, el))
  )
    return false;
  // The staging is the frame's read in the frame's seat; the seam re-derives
  // only if the flush parks (D-22, F-10). Content a display-ahead gate opens
  // is a mount (#4806), but not under the pass's own `isPending` window: a
  // read's place beside the probe never changes what shows (F-6, D-23, #4966).
  if (laneStageHook && el && fresh && passLane && c._flags >> WINDOW_KINDS != 1)
    passLane._opened = 1;
  staleReader(c, flushTransaction);
  return true;
}
function ownedScopeWriteMessage(owner) {
  const name = owner._name;
  return name
    ? `${REACTIVE_WRITE_IN_OWNED_SCOPE_SIGNAL_MESSAGE} (in ${name})`
    : REACTIVE_WRITE_IN_OWNED_SCOPE_SIGNAL_MESSAGE;
}
/** A write inside a recompute outside a flush is promoted at that pass's end. */
function notePromotedWrite(el) {
  if (globalQueue._running || el._config & CONFIG_PROMOTED) return;
  el._config |= CONFIG_PROMOTED;
  promotedWrites.push(el);
}
/** The first write during snapshot capture to a plain user signal records the
 * pre-write value. Async landings reveal, and projection leaves are their
 * compute's, so neither qualifies. */
function captureWriteSnapshot(el, current) {
  if (
    el._config & CONFIG_NO_SNAPSHOT ||
    el._fn !== undefined ||
    el._x?._snapshotValue !== undefined
  )
    return;
  takeSnapshot(el, current);
}
function takeSnapshot(el, value) {
  ext(el)._snapshotValue = value === undefined ? NO_SNAPSHOT : value;
  el._config |= CONFIG_HAS_SNAPSHOT;
  snapshotSources.add(el);
}
/** Called before every signal or store write while set. The server-components
 * client mounts still-deferred slots before a write applies, then clears it. */
let writeHook = null;
/** @internal */
function setWriteHook(hook) {
  writeHook = hook;
}
/** The one guarded comparator call. A comparator that read unresolved async
 * reports a change (#4111); any other throw is `el`'s error, routed to its
 * boundary and answered "same" (D-38, #2837).
 * @internal */
function isSame(el, a, b) {
  if (el._equals === isEqual) return a === b;
  try {
    return el._equals && el._equals(a, b);
  } catch (e) {
    // A comparator's throw is `el`'s, not a catch in the running pass (D-42).
    forgetThrownRead();
    if (!(e instanceof NotReadyError)) return (notifyStatus(el, STATUS_ERROR, e), true);
  }
}
/** Observe: the async node landing through `setSignal`, and whether it passed
 * the equality gate. */
let landing = null;
let landed = false;
/** Observe: the landing reports this write as `asyncLanded`, not `write`. */
function landThroughSetSignal(el, value) {
  // Saved, not reset: a nested landing must not clear the outer's.
  const outerLanding = landing;
  const outerLanded = landed;
  landing = el;
  landed = false;
  try {
    setSignal(el, () => value);
    return landed;
  } finally {
    landing = outerLanding;
    landed = outerLanded;
  }
}
function setSignal(el, v) {
  if (writeHook !== null) writeHook(el);
  if (
    !(el._config & CONFIG_OWNED_WRITE) &&
    !(context && context._config & CONFIG_CHILDREN_FORBIDDEN) &&
    context &&
    !disposalDepth &&
    !inRefCallback()
  ) {
    emitDiagnostic({
      code: "REACTIVE_WRITE_IN_OWNED_SCOPE",
      kind: "write",
      severity: "error",
      message: REACTIVE_WRITE_IN_OWNED_SCOPE_SIGNAL_MESSAGE,
      ownerId: context.id,
      ownerName: context._name,
      nodeName: el._name,
      data: { operation: "setSignal" }
    });
    throw new Error(ownedScopeWriteMessage(context));
  }
  const currentValue = el._pendingValue === NOT_PENDING ? el._value : el._pendingValue;
  if (typeof v === "function") v = v(currentValue);
  // A write landing on a guess is its truth (F-11); one on a lane's
  // derivation is the lane's staging.
  if (laneWriteHook && el._config & CONFIG_OVERRIDE) return laneWriteHook(el, v);
  // A write to a held slot is a second proposal: the writer's tick joins the
  // hold, before the equality gate (D-8).
  if (el._config & CONFIG_HELD) joinFuture(txOf(el));
  // The first commit has no previous value: the comparator never sees
  // `undefined`. A write over a shown error is a change (D-41). `custom` is
  // taken once for the gate and the netting so the default `===` runs inline
  // (CodSpeed `propagation: avoidable`); without the engine it is `null` and
  // the netting folds away.
  const custom = observeFlightHook ? el._equals !== isEqual : null;
  const valueChanged =
    !!(el._statusFlags & STATUS_UNINITIALIZED) ||
    !!el._x?._shownError ||
    (custom === false ? currentValue !== v : !isSame(el, currentValue, v));
  if (!valueChanged) return v;
  // Observe: a committed write begins a re-run chain; a landing reports itself.
  if (attrHooks !== null) {
    if (el === landing) landed = true;
    else attrHooks.write(el, currentValue, v);
  }
  // A write during snapshot capture to a source created before capture
  // captures the pre-write value now, so claims keep the server's value and
  // replay at release. Store leaves qualify too.
  if (snapshotCaptureActive) captureWriteSnapshot(el, currentValue);
  const wasStaged = el._pendingValue !== NOT_PENDING;
  if (!wasStaged) queuePendingNode(el);
  // F-10: a held node rewritten outside a flush keeps its flushed staging.
  else if (el._config & CONFIG_HELD && !globalQueue._running) stashFlushed(el);
  // A staged rewrite equal to the committed value nets to it (D-8, #4483).
  // Only the verdict engine reads the netting; under the default comparator
  // `v` already is the committed object.
  el._pendingValue =
    custom &&
    wasStaged &&
    !(el._statusFlags & STATUS_UNINITIALIZED || el._x?._shownError) &&
    isSame(el, el._value, v)
      ? el._value
      : v;
  if (context !== null) notePromotedWrite(el);
  // `_time` is computed-only: writing it on a signal forks the lean shape.
  if (el._fn !== undefined) el._time = clock;
  // A re-write whose subscribers were walked with nothing recomputed or linked
  // since re-stages and stops: the walk is idempotent.
  if (wasStaged && el._notifiedAt === notifyEpoch) return v;
  insertSubs(el);
  schedule();
  return v;
}
/** User-facing setter for the memo form of `createSignal(fn)`. The write
 * stages like any other, and a later source change re-derives with it as
 * `prev` (D-8, CS-R31). REACTIVE_MANUAL_WRITE marks it a user proposal until
 * a pass re-derives it or the commit lands it. */
function setMemo(el, v) {
  // Is `el`'s staging a derivation another transaction holds? Then a setter
  // composes on the committed frame it was written against and becomes `prev`
  // for the hold's re-derivation (D-8, #3612).
  const held =
    el._config & CONFIG_HELD &&
    !(el._flags & REACTIVE_MANUAL_WRITE) &&
    txOf(el) !== flushTransaction;
  if (held && typeof v === "function") {
    const value = v(el._value);
    v = () => value;
  }
  // A write replaces a failed derivation's outcome without re-running `_fn`,
  // so clear STATUS_ERROR here (#4110). The error shows until the write
  // commits (D-41).
  if (el._statusFlags & STATUS_ERROR) {
    el._x._shownError = el._x._error;
    notifyStatus(el, STATUS_NONE);
  }
  v = setSignal(el, v);
  // A write replaces a staged error (D-41).
  el._config &= ~CONFIG_STAGED_ERROR;
  if (held) {
    // The staging it replaced came from inputs the writer never saw: the hold
    // re-derives over the write (D-8).
    el._flags = (el._flags & ~REACTIVE_CHECK) | REACTIVE_DIRTY;
    insertIntoHeap(el, dirtyQueue);
    schedule();
  } else if (el._pendingValue !== NOT_PENDING) el._flags |= REACTIVE_MANUAL_WRITE;
  return v;
}
/**
 * Executes `fn` with the given `owner` set as the current owner. Any reactive
 * primitives (`createSignal`, `createMemo`, `createEffect`, `onCleanup`,
 * `cleanup`, etc.) created inside `fn` are attached to that owner, so they
 * are disposed when the owner is disposed.
 *
 * The classic pattern: capture the current owner with `getOwner()` inside a
 * component, then re-enter it from a callback (event handler, async resolve,
 * setTimeout) so disposables created in the callback get cleaned up with the
 * component.
 *
 * @example
 * ```ts
 * function delayed<T>(ms: number, fn: () => T) {
 *   const owner = getOwner();
 *   setTimeout(() => runWithOwner(owner, fn), ms);
 * }
 * ```
 */
function runWithOwner(owner, fn) {
  if (owner && owner._flags & REACTIVE_DISPOSED) {
    const message =
      "[RUN_WITH_DISPOSED_OWNER] runWithOwner called with a disposed owner. Children created inside will never be disposed.";
    reportDiagnostic(
      emitDiagnostic(
        {
          code: "RUN_WITH_DISPOSED_OWNER",
          kind: "owner",
          severity: "warn",
          message,
          ownerId: owner.id,
          ownerName: owner._name
        },
        owner
      )
    );
  }
  const oldContext = context;
  const prevTracking = tracking;
  context = owner;
  tracking = false;
  try {
    return fn();
  } finally {
    context = oldContext;
    tracking = prevTracking;
  }
}
function staleValues(fn, set = true) {
  try {
    return fn();
  } finally {
  }
}
/** Core half of `refresh()` (signals.ts): marks the next recompute a quiet
 * re-ask and schedules it. */
function markRefresh(node) {
  if (
    context &&
    !((node._config ?? 0) & CONFIG_OWNED_WRITE) &&
    !(context._config & CONFIG_CHILDREN_FORBIDDEN) &&
    !disposalDepth &&
    !inRefCallback()
  ) {
    emitDiagnostic({
      code: "REACTIVE_WRITE_IN_OWNED_SCOPE",
      kind: "write",
      severity: "error",
      message: REACTIVE_WRITE_IN_OWNED_SCOPE_REFRESH_MESSAGE,
      ownerId: context.id,
      ownerName: context._name,
      nodeName: node._name,
      data: { operation: "refresh" }
    });
    throw new Error(REACTIVE_WRITE_IN_OWNED_SCOPE_REFRESH_MESSAGE);
  }
  if (typeof node._fn === "function" && !(node._flags & REACTIVE_DISPOSED)) {
    // A refresh in a manual write's frame re-asks with the write as `prev`
    // (D-8, #3026). The next pass's flight is a quiet re-ask (F-9), unless an
    // input changed this tick.
    node._flags =
      (node._flags & ~REACTIVE_CHECK) |
      REACTIVE_DIRTY |
      (node._flags & (REACTIVE_DIRTY | REACTIVE_CHECK | REACTIVE_IN_HEAP) ? 0 : REACTIVE_REASK);
    // Observe: a refresh is a root cause; the run points here.
    if (attrHooks !== null) attrHooks.refreshed(node);
    insertIntoHeap(node, dirtyQueue);
    schedule();
  }
}

export {
  CONFIG_STAGED_ERROR as $,
  STATUS_ERROR as A,
  NO_SNAPSHOT as B,
  CONFIG_VERDICT as C,
  REACTIVE_SNAPSHOT_STALE as D,
  CONFIG_OVERRIDE as E,
  CONFIG_GUESS as F,
  GlobalQueue as G,
  STATUS_PENDING as H,
  latestOf as I,
  screenOf as J,
  passLane as K,
  sameLane as L,
  resolveTx as M,
  NotReadyError as N,
  staleReader as O,
  strictRead as P,
  strictReadActive as Q,
  REACTIVE_JOINED as R,
  STATUS_UNINITIALIZED as S,
  warnStrictReadUntracked as T,
  EFFECT_RENDER as U,
  blocked as V,
  WINDOW_KINDS as W,
  keptGuess as X,
  globalQueue as Y,
  joinFuture as Z,
  joinPass as _,
  adopter as a,
  inEffectCallback as a$,
  markLateLinker as a0,
  spectating as a1,
  REACTIVE_DISPOSED as a2,
  installVerdictHooks as a3,
  unflushedValue as a4,
  REACTIVE_LANE_READ as a5,
  ahead as a6,
  REACTIVE_STAGED_READ as a7,
  CONFIG_HELD_FRAME as a8,
  bornInto as a9,
  EFFECT_TRACKED as aA,
  settlePendingSource as aB,
  REACTIVE_REASK as aC,
  CONFIG_INPUTS_PUBLISHED as aD,
  tickBase as aE,
  REACTIVE_IN_HEAP as aF,
  REACTIVE_DIRTY as aG,
  REACTIVE_CHECK as aH,
  list as aI,
  REACTIVE_LANE_DIRTY as aJ,
  schedule as aK,
  covers as aL,
  newTransaction as aM,
  attrHooks as aN,
  enterAction as aO,
  setEffectCallback as aP,
  exitAction as aQ,
  actionDepth as aR,
  flush as aS,
  nextQuestion as aT,
  setQuestion as aU,
  question as aV,
  inRefCallback as aW,
  emitDiagnostic as aX,
  enterCallback as aY,
  exitCallback as aZ,
  getOwner as a_,
  setPassLane as aa,
  verdictLane as ab,
  isSame as ac,
  stagedReaders as ad,
  guessing as ae,
  pendingNodes as af,
  REACTIVE_MANUAL_WRITE as ag,
  heldRewrites as ah,
  isThenable as ai,
  REACTIVE_FRAME_READ as aj,
  enqueueSub as ak,
  isVerdict as al,
  deleteFromHeap as am,
  dirtyQueue as an,
  releaseFlightTeardown as ao,
  handleAsync as ap,
  notifyStatus as aq,
  CONFIG_ASYNC as ar,
  CONFIG_OWN_PROBE as as,
  CONFIG_FRAME as at,
  waiting as au,
  CONFIG_STAGED as av,
  heldMountUnder as aw,
  queuePendingNode as ax,
  insertSubs as ay,
  CONFIG_REDERIVE as az,
  REACTIVE_PROBED as b,
  markRefresh as b$,
  effectCallbackEvent as b0,
  effectCallbackOwner as b1,
  setExternalHooks as b2,
  signal as b3,
  setSignal as b4,
  cleanup as b5,
  read as b6,
  DEV as b7,
  CONFIG_TRANSPARENT as b8,
  NoOwnerError as b9,
  devWarnOptimisticWhileLoading as bA,
  armGuessing as bB,
  armGuessed as bC,
  installGuessLayer as bD,
  installGuessHooks as bE,
  rerun as bF,
  covered as bG,
  setLaneValue as bH,
  lanes as bI,
  marked as bJ,
  commitPendingNode as bK,
  landBeneath as bL,
  ownFlights as bM,
  guessFlights as bN,
  underOf as bO,
  markJudged as bP,
  merge as bQ,
  readGuesses as bR,
  dissolveLane as bS,
  holdNode as bT,
  newLane as bU,
  inFlight as bV,
  laneDirty as bW,
  registerGraph as bX,
  runWithOwner as bY,
  setMemo as bZ,
  $REFRESH as b_,
  ContextNotFoundError as ba,
  createEffectNode as bb,
  recompute as bc,
  applyStaged as bd,
  EFFECT_USER as be,
  reportDiagnostic as bf,
  computed as bg,
  staleValues as bh,
  CONFIG_AUTO_DISPOSE as bi,
  heldRunHook as bj,
  attachHook as bk,
  unwrapStatusError as bl,
  setStrictRead as bm,
  queueEvent as bn,
  StatusError as bo,
  trimStaleDeps as bp,
  haltReactivityAt as bq,
  notifyEpoch as br,
  strictReadEvent as bs,
  setTrackedQueueCallback as bt,
  setEffectStatusNotify as bu,
  queueEffectRun as bv,
  _hitUnhandledAsync as bw,
  resetUnhandledAsync as bx,
  ROOT_ERROR_HOOK as by,
  CONFIG_SLOT_NODE as bz,
  creatorPass as c,
  heldRead as c$,
  createRoot as c0,
  getObserver as c1,
  dispose as c2,
  TimeoutError as c3,
  untrack as c4,
  CONFIG_AUTHORITATIVE as c5,
  truthRead as c6,
  truthConfirmed as c7,
  passTx as c8,
  CONFIG_FRESH_READ as c9,
  registerStoreTools as cA,
  isDisposed as cB,
  laneServedHook as cC,
  landBySetter as cD,
  addPendingSource as cE,
  propagateStatus as cF,
  forEachDependent as cG,
  devGuardOptimisticBeforeLoad as cH,
  joinPassAt as cI,
  SUPPORTS_PROXY as cJ,
  CONFIG_KEY_BAG as cK,
  releaseMarksHook as cL,
  installMarkHooks as cM,
  CONFIG_LAGS as cN,
  onSupersede as cO,
  haltReactivity as cP,
  swapHook as cQ,
  heldUnder as cR,
  unhold as cS,
  REACTIVE_ZOMBIE as cT,
  installBoundaryHooks as cU,
  CAUGHT_REARMED as cV,
  OBSERVE as cW,
  checkForeignOwnerless as cX,
  clearSnapshots as cY,
  enforceLoadingBoundary as cZ,
  getNextChildId as c_,
  createOwner as ca,
  writeHook as cb,
  REACTIVE_WRITE_IN_OWNED_SCOPE_SIGNAL_MESSAGE as cc,
  ownedScopeWriteMessage as cd,
  isEqual as ce,
  CONFIG_MANUAL_WRITE as cf,
  laneStageHook as cg,
  captureWriteSnapshot as ch,
  slotSignal as ci,
  notePromotedWrite as cj,
  observeFlightHook as ck,
  CONFIG_PROMOTED as cl,
  joinPassTx as cm,
  stagedScreen as cn,
  stagedRead as co,
  setSlotUnobserved as cp,
  installStoreHooks as cq,
  disposalDepth as cr,
  insertIntoHeap as cs,
  registerDevStoreSlotWriter as ct,
  checkForeignRead as cu,
  asyncTailFlights as cv,
  untrackDepth as cw,
  checkPostAwaitRead as cx,
  REACTIVE_RECOMPUTING_DEPS as cy,
  devRead as cz,
  REACTIVE_WINDOW as d,
  isHidden as d0,
  markSnapshotScope as d1,
  peekNextChildId as d2,
  releaseSnapshotScope as d3,
  resetErrorHalt as d4,
  runAsSolidCopy as d5,
  runRefCallback as d6,
  setConsoleFooter as d7,
  setSnapshotCapture as d8,
  setSwapHook as d9,
  setWriteHook as da,
  strictReadTracked as db,
  setAttributionHooks as dc,
  liveRootOwners as dd,
  ownerPath as de,
  storeTools as df,
  blockersOf as dg,
  CONFIG_PLUMBING as dh,
  anyExcluded as di,
  isExcluded as dj,
  CONFIG_WIDE as dk,
  isSuppressed as dl,
  GRAPH_SIZE_WARN_AT as dm,
  noteFanOut as dn,
  CONFIG_SUPERSEDES as dp,
  creatorReads as e,
  flushTransaction as f,
  setAdopter as g,
  dropFlushTx as h,
  REACTIVE_MISSED_WAKE as i,
  context as j,
  REACTIVE_PROBE_UNANSWERED as k,
  forgetThrownRead as l,
  CONFIG_CHILDREN_FORBIDDEN as m,
  link as n,
  NOT_PENDING as o,
  prepareComputed as p,
  clock as q,
  ext as r,
  setVerdict as s,
  tracking as t,
  CONFIG_HELD as u,
  verdict as v,
  txOf as w,
  pullComputed as x,
  snapshotCaptureActive as y,
  CONFIG_IN_SNAPSHOT_SCOPE as z
};
