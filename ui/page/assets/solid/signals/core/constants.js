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
 * recompute is deferred past the seam (`deferZombie`). */ const REACTIVE_ZOMBIE = 1 << 5;

const REACTIVE_DISPOSED = 1 << 6;

/** L2 — this pass read the future (a tracked read of a CONFIG_HELD node).
 * Lives for the pass only: set by `read`, consumed by `recompute`'s publish
 * (a first pass that read the future is born held, F-7), wiped with the
 * pass's other per-pass bits. */ const REACTIVE_JOINED = 1 << 7;

const REACTIVE_SNAPSHOT_STALE = 1 << 8;

const REACTIVE_LAZY = 1 << 9;

const REACTIVE_MANUAL_WRITE = 1 << 10;

/** L2, rule 3 — the node's LAST pass was a render effect's read of the
 * future as committed (`frameRead`): the frame shows the committed world
 * beside a future it also derives from, and re-derives at the landing.
 * Survives its own pass's wipe; the next pass's wipe clears it (a pass that
 * re-derived the frame — in the future, or without the held read — owes the
 * landing nothing). The landing consumes it (the transaction's `_reruns`). */ const REACTIVE_FRAME_READ = 1 << 11;

/**
 * A dependency write landed while this subscriber was mid-recompute — a
 * nested pull committed beneath one of its reads (#3037). The heap refuses
 * RECOMPUTING nodes, so recompute's tail consumes this latch and reschedules:
 * values the pass read before the nested commit are stale. Only set for
 * links validated this pass (gen-current): a write to an untouched link is
 * either re-read later in the pass (fresh) or trimmed with it (not a dep).
 */ const REACTIVE_MISSED_WAKE = 1 << 12;

/** A `refresh()` asked this node to re-ask its question (F-9, D-23):
 * consumed by the next pass, which — going pending — classifies its flight
 * quiet (`_x._reask`). */ const REACTIVE_REASK = 1 << 13;

/** The pass was served a node's ambient staging — written this flush, not
 * held. If the flush parks, the pass is the frame's with it (the same sync
 * frame as the async): a verdict read cannot break it out. Per pass. */ const REACTIVE_STAGED_READ = 1 << 14;

/** The pass entered a verdict window (`isPending`/`latest`). Per pass: a
 * pass that did not keeps CONFIG_VERDICT no longer — the frame-reader
 * posture lasts exactly as long as the probing does. */ const REACTIVE_PROBED = 1 << 15;

/** Lanes — the node (a lane's member) was dirtied by the lane's own
 * re-staging (a re-guess, lane work re-derived): its next pass is the
 * lane's from the start (`recompute`), not a stale reader's re-run by an
 * unrelated write (#3460). Consumed by the pass. */ const REACTIVE_LANE_DIRTY = 1 << 16;

/** Lanes — the pass read a lane's value as lane work (`enterLane`): a
 * tracked derivation's read of displayed optimism, a verdict read routed
 * into the holder's verdict lane. A pass in a lane's seat that read none of
 * its world has left the lane (`recompute`'s tail). Per pass. */ const REACTIVE_LANE_READ = 1 << 17;

/** Lanes — a render effect in the frame's seat was served the screen
 * for a lane's node (`laneRead`: a stale reader of a lane that has not
 * shown). Its pass computing exactly what it last applied has nothing to
 * run — an effect has no comparator, and the guess's notification is not a
 * change it can see. Per pass. */ const REACTIVE_SCREEN_READ = 1 << 18;

/** A verdict reader whose probe was answered before the flush had a
 * transaction (verdict.ts `watchVerdict`): the seam re-runs it even if a
 * later read of the same pass routed it into a verdict lane — that was not
 * the probe's answer (lanes.ts). Per pass. */ const REACTIVE_PROBE_UNANSWERED = 1 << 19;

/** The pass opened a verdict window of its own (`isPending`/`latest`) — not
 * only read a verdict memo (F-6, core.ts `read`). An async memo whose pass
 * did not is D-25's hold work while its flight is up (verdict.ts
 * `holdWork`). Per pass. */ const REACTIVE_WINDOW = 1 << 21;

/** #4544: a render effect born into a hold read a settled verdict reader's
 * value this pass (core.ts `read`). Per pass: `recompute`'s tail keeps it as
 * CONFIG_AHEAD. */ const REACTIVE_AHEAD = 1 << 20;

/** An observation-lifecycle node found unobserved by a settle (#2934) while
 * a pass of it was already queued — a source's landing re-enqueued the
 * `_blocked` reader it walks (`settlePendingSource`). The release waits for
 * that pass: a memo nobody read has still produced what the flushes it
 * missed gave it (D-22, #4474), so the queued pass runs, then `recompute`'s
 * tail releases the node. Survives the head wipe; the tail consumes it. */ const REACTIVE_RELEASE_AFTER = 1 << 22;

/** A computed its own owner reads (a memo reading the memo it created): the
 * owner sits below it (heights put owners first), so a wake of it wakes the
 * owner too — the owner's pass re-creates it and is what its readers see. */ const REACTIVE_OWNER_READS = 1 << 23;

/** The windows the pass opened, by kind, from this bit up (`window <<
 * WINDOW_KINDS`): 1 `isPending`, 2 `latest`. Per pass (bits 24 and 25;
 * nothing sits above them). #4966: a first plain read that only the pass's
 * own `isPending` window gates is not a mount (core.ts `stagedScreen`). */ const WINDOW_KINDS = 24;

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
 * the commit. Set by `recompute` when it queues the node. */ const CONFIG_STAGED = 1 << 7;

/** The node's staged state is in the future: the flush that staged it
 * parked. A write to it, a tracked read of it by a derivation, or a pass
 * over a derivation holding it joins the future (the flush parks). Render
 * effects are the frame, not derivations (rule 3): reading it outside a
 * parking flush they see the committed value and hold nothing, and are
 * re-derived at the landing. Set at the park seam on every staged node. */ const CONFIG_HELD = 1 << 8;

/** A member of the uncommitted frame a held pass built (ruling A): set at
 * the park seam on every computed under a staged node (`holdFrame`). Never
 * shown, it has no screen: its reads are its transaction's, whichever flush
 * runs it (`bornInto`, #4010). Cleared by its commit. */ const CONFIG_HELD_FRAME = 1 << 20;

/** F-1 (#3305): a commit published this flight's inputs
 * beneath it — the node was pending, nothing staged, when it committed — so
 * its committed value is torn against the visible frame. A render effect
 * revealing it cannot be a stale reader: it observes the flight (pends,
 * holds) instead of showing the pre-flight value. Outlives the flight that
 * earned it for as long as the node stays pending, and no longer: the
 * commit that lands a value clears it. */ const CONFIG_INPUTS_PUBLISHED = 1 << 10;

/** Lanes — the node has a lane's value (`_x._lane`): a written guess
 * (CONFIG_GUESS) or a lane pass's derivation of one. Its lane is
 * `_x._transaction`. The gate to the lane arm of `read`; `_value` stays the
 * committed truth and `_pendingValue` a transaction's staging throughout —
 * a value's world is where it lives, never a bit. */ const CONFIG_OVERRIDE = 1 << 11;

/** A store slot node (`slotSignal`): a leaf, presence, container or deep
 * witness of a store target. One literal, no `_x` at birth; the last-one-out
 * sweep dispatches it to the store's shared release hook (graph.ts) instead
 * of a per-node `_unobserved` closure. */ const CONFIG_SLOT_NODE = 1 << 12;

/** A store slot node whose staging is a USER setter's proposal, not its
 * derive's derivation (D-8's discriminator for derived stores — the
 * leaf twin of REACTIVE_MANUAL_WRITE). Set by the user setter, cleared by
 * the derive's own write to the key. */ const CONFIG_MANUAL_WRITE = 1 << 18;

/** D-41: the staged value in `_pendingValue` is an error — an
 * answer of the held frame, published at its landing like a value. */ const CONFIG_STAGED_ERROR = 1 << 21;

/** Lanes — the node carries a written guess (`optimisticWrite`): its own
 * source recomputing it, or a plain write landing on it, is the truth
 * (`supersede`). A lane's derived staging is not. */ const CONFIG_GUESS = 1 << 13;

/** The node's pass read a verdict (`isPending`/`latest`): a dependency going
 * pending is a question for its next pass — the verdict changed — so the
 * propagation re-derives it instead of marking it pending (as a kept-tail
 * link does, D-10). Set at the window, never cleared: a reader that stopped
 * asking re-derives once and goes pending through its own read. The same
 * bit as REACTIVE_PROBED: a pass's probe (or a read of a verdict memo,
 * core.ts `read`) carries into the config at its end without a shift. */ const CONFIG_VERDICT = 1 << 15;

/** A dependency's status is a question for this node's pass, not a fact
 * about its current one: the propagation re-derives it instead of marking
 * it, for errors as for pending (as CONFIG_VERDICT). A boundary's output
 * (show the content, or the fallback) and its `on` node (boundaries.ts); a
 * promise over an expression (`resolve`/`until`, signals.ts: the pass reads
 * the error and rejects, or re-asks a pending source). */ const CONFIG_REDERIVE = 1 << 14;

/** An answered `createDeferred` node (deferred.ts): held because a
 * downstream hold staged its landing, its re-pass is a lagging question,
 * not an answer, and joins no transaction (D-40). */ const CONFIG_LAGS = 1 << 19;

/** The reader wants the truth, not the lane's guess (`until`, signals.ts):
 * a read of displayed optimism serves the base the guess covers, makes the
 * pass no lane's, and the truth landing wakes it even when it confirms the
 * guess (the one case ordinary subscribers are not told, F-11). */ const CONFIG_AUTHORITATIVE = 1 << 17;

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
 * (#2930), not "next quiescent state". */ const CONFIG_FRESH_READ = 1 << 16;

/** F-10: the node was written inside a recompute that ran OUTSIDE a flush
 * (a creation-time compute — boundary machinery, a mapArray's first run). Such
 * a write is promoted at that recompute's end: readers in the same block see
 * it. Cleared when the next flush begins; set only on that rare path. */ const CONFIG_PROMOTED = 1 << 22;

/** The frame (rule 3, D-3): a render effect (set at creation, as
 * `EFFECT_RENDER << 23` — keep the two in step), and a boundary's tree and
 * output (boundaries.ts) — structure between the content and the render
 * effect that shows it, not a user derivation of the content. Outside a
 * hold's own flush it reads that hold's values as committed and is
 * re-derived at its landing, and a re-pass of it publishes mainline: it
 * joins nothing, so two mounts under one boundary made in separate flushes
 * keep their holds apart (D-4, S-5 rows 1′–3′, #4283). */ const CONFIG_FRAME = 1 << 23;

/** A render effect created by a pass that probed (`isPending`/`latest`) —
 * the mount a display-ahead `Show` opens (S-46, D-25). Stamped at its first
 * pass; its first read of a hold's staging or flight is born held, not a
 * D-3 leaf of the screen, and the verdict lane that opened it reveals with
 * the landing (lanes.ts `mountPending`). */ const CONFIG_MOUNT_AHEAD = 1 << 24;

/** The node's last answering pass registered a flight: an async memo. Kept
 * by the verdict engine alone (verdict.ts `holdWork`), which reads it. */ const CONFIG_ASYNC = 1 << 29;

/** #4544: the node's last pass opened a verdict window of its own
 * (`isPending`/`latest`) — REACTIVE_WINDOW, kept by `recompute`'s tail
 * beside CONFIG_VERDICT and renewed by every pass. One whose posture is only
 * inherited (it read a verdict memo) is the hold's work where it reads the
 * hold's own write (verdict.ts `holdStaging`). */ const CONFIG_OWN_PROBE = REACTIVE_WINDOW << 9;

/** #4544 (F-6): the effect's last pass read a display-ahead value
 * (REACTIVE_AHEAD): what it went pending on does not hold its hold
 * (scheduler.ts `blockedBy`). The sign bit: tested by `&` only. */ const CONFIG_AHEAD = REACTIVE_AHEAD << 11;

const STATUS_NONE = 0;

const STATUS_PENDING = 1 << 0;

const STATUS_ERROR = 1 << 1;

const STATUS_UNINITIALIZED = 1 << 2;

/** An error that answered a node's first read (no value has ever landed):
 * the error, not UNINITIALIZED, is its status while shown, but a re-ask
 * makes it uninitialized-pending again — it still has no value (#4244). */ const STATUS_NO_VALUE = 1 << 3;

/** `catchHook`'s answer when the catcher is a boundary re-armed by
 * `on` (truthy, as any catch): what it collects is its own, not the hold's. */ const CAUGHT_REARMED = 2;

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
 */ const $REFRESH = Symbol("refresh");

const NewSet = Set;

/** Dev: the strict-read label compiled dev DOM output reads an event handler expression under (#1742). */ const EVENT_HANDLER_READ = "an event handler expression";

export { $REFRESH, CAUGHT_REARMED, CONFIG_AHEAD, CONFIG_ASYNC, CONFIG_AUTHORITATIVE, CONFIG_AUTO_DISPOSE, CONFIG_CHILDREN_FORBIDDEN, CONFIG_FRAME, CONFIG_FRESH_READ, CONFIG_GUESS, CONFIG_HAS_SNAPSHOT, CONFIG_HELD, CONFIG_HELD_FRAME, CONFIG_INPUTS_PUBLISHED, CONFIG_IN_SNAPSHOT_SCOPE, CONFIG_LAGS, CONFIG_MANUAL_WRITE, CONFIG_MOUNT_AHEAD, CONFIG_NO_SNAPSHOT, CONFIG_OVERRIDE, CONFIG_OWNED_WRITE, CONFIG_OWN_PROBE, CONFIG_PROMOTED, CONFIG_REDERIVE, CONFIG_SLOT_NODE, CONFIG_STAGED, CONFIG_STAGED_ERROR, CONFIG_SYNC, CONFIG_TRANSPARENT, CONFIG_VERDICT, EFFECT_RENDER, EFFECT_TRACKED, EFFECT_USER, EVENT_HANDLER_READ, NOT_PENDING, NO_SNAPSHOT, NewSet, REACTIVE_AHEAD, REACTIVE_CHECK, REACTIVE_DIRTY, REACTIVE_DISPOSED, REACTIVE_FRAME_READ, REACTIVE_IN_HEAP, REACTIVE_IN_HEAP_HEIGHT, REACTIVE_JOINED, REACTIVE_LANE_DIRTY, REACTIVE_LANE_READ, REACTIVE_LAZY, REACTIVE_MANUAL_WRITE, REACTIVE_MISSED_WAKE, REACTIVE_NONE, REACTIVE_OWNER_READS, REACTIVE_PROBED, REACTIVE_PROBE_UNANSWERED, REACTIVE_REASK, REACTIVE_RECOMPUTING_DEPS, REACTIVE_RELEASE_AFTER, REACTIVE_SCREEN_READ, REACTIVE_SNAPSHOT_STALE, REACTIVE_STAGED_READ, REACTIVE_WINDOW, REACTIVE_ZOMBIE, STATUS_ERROR, STATUS_NONE, STATUS_NO_VALUE, STATUS_PENDING, STATUS_UNINITIALIZED, SUPPORTS_PROXY, WINDOW_KINDS, defaultContext };