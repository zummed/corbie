import { EFFECT_RENDER, EFFECT_USER, CONFIG_HELD, CONFIG_OVERRIDE, NOT_PENDING, STATUS_UNINITIALIZED, STATUS_PENDING, CONFIG_STAGED, REACTIVE_DISPOSED, REACTIVE_ZOMBIE, CONFIG_VERDICT, CONFIG_HAS_SNAPSHOT, REACTIVE_RECOMPUTING_DEPS, REACTIVE_MISSED_WAKE, CONFIG_IN_SNAPSHOT_SCOPE, REACTIVE_SNAPSHOT_STALE, REACTIVE_OWNER_READS, CONFIG_HELD_FRAME, CONFIG_SLOT_NODE, NewSet, REACTIVE_LANE_DIRTY, CONFIG_STAGED_ERROR, CONFIG_INPUTS_PUBLISHED, STATUS_ERROR, EFFECT_TRACKED, REACTIVE_MANUAL_WRITE, CONFIG_GUESS, CONFIG_AHEAD, REACTIVE_FRAME_READ, CONFIG_REDERIVE, CONFIG_FRAME, REACTIVE_CHECK } from "./constants.js";

import { attrHooks } from "./attribution-hooks.js";

import { notifyStatus } from "./async.js";

import { resyncUnflushedCompanions, isSame, recompute, adopter, markUnflushedStaged, ext } from "./core.js";

import { DEV } from "./dev.js";

import { sweepDormant, trimStaleDeps } from "./graph.js";

import { runHeap, enqueueSub, markNode, deleteFromHeap } from "./heap.js";

import { devCheckQuiescent } from "./invariants.js";

import { rethrowCleanupError } from "./owner.js";

// Optional-module hooks: module bindings, not `GlobalQueue` statics, so a
// bundle that never imports the installing module never assigns them — its
// minifier folds every guarded call site (and the branch behind it) away.
// Each module installs its own set at evaluation (pay-for-use).
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
 * (`adoptParked`; true: adopted, nothing to queue). */ let laneAdoptHook = null;

/** R5 at the seam (verdict.ts `r5`): a reader whose probe is unanswered, a
 * hold's parked flight still up (`r` null), the seam's end (both null). */ let r5Hook = null;

/** core.ts `frameRead` served lane work or a verdict reader a held node's
 * committed value (lanes.ts `laneServed`). */ let laneServedHook = null;

/** A guess was written (guesses.ts `optimisticWrite`): the arms only a guess
 * reaches (CONFIG_GUESS) — a build without `createOptimistic` or
 * `createOptimisticStore` never sets it, and its bundler folds them. */ let guessing = false;

function armGuessing() {
    guessing = true;
}

function installLaneHooks(e, o, n, t, l, s, u) {
    laneReadHook = e;
    laneStageHook = o;
    laneWriteHook = n;
    laneSeamsHook = t;
    endLanesHook = l;
    // The transaction being judged (`judge`): a lane consulted on its behalf
    // judges for it. Only this hook nests one `blocked` in another, so the
    // bookkeeping lives here and a build without lanes folds `judge` away.
        lanesBlockedHook = e => {
        const o = judge;
        judge ??= e;
        const n = s(e);
        judge = o;
        return n;
    };
    laneServedHook = u;
}

// The guess layer (guesses.ts; installed when `createOptimistic` or
// `createOptimisticStore` is imported): a written guess's own pass judged
// (`laneOutcome`, core.ts `recompute`), the seam's guesses opening their
// lanes (`applyGuessesAt`), the body-end corrections (`laneCorrections`).
// Null without it: every site tolerates it.
function installGuessHooks(e, o, n) {
    laneOutcomeHook = e;
    applyGuessesHook = o;
    laneCorrectionsHook = n;
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

function installBoundaryHooks(e, o, n, t, l) {
    catchHook = e;
    freshHook = o;
    hiddenHook = n;
    boundarySeamHook = t;
    heldRunHook = l;
}

/** `@solidjs/web`'s head registry: told when a boundary swaps between its
 * fallback and its content, and asks whether a registration's node is
 * behind a fallback — content there is not on screen (F-3), nor are its
 * head tags (#4873). Never set without the registry. */ let swapHook = null;

function setSwapHook(e) {
    swapHook = e;
}

function isHidden(e) {
    return !!hiddenHook?.(e);
}

/** Store (store/store.ts): `park` — the flush parked into `t`: the store
 * materializes and holds the container node of every staging that had none
 * (a staging needs the node as its home only when something can read the
 * frame through it; a hold can); `commit` — fold the pending backings whose
 * container nodes this flush committed (the owned-raw model's one mutation
 * point). */ let storeParkHook = null;

let storeCommitHook = null;

function installStoreHooks(e, o) {
    storeParkHook = e;
    storeCommitHook = o;
}

/** `affects()` marks (affects.ts): the releases at a landing and at the seam
 * (ambient marks). */ let releaseMarksHook = null;

let releaseAmbientMarksHook = null;

function installMarkHooks(e, o) {
    releaseMarksHook = e;
    releaseAmbientMarksHook = o;
}

/** Verdicts (verdict.ts). */ let observeFlightHook = null;

/** D-25's hold work (verdict.ts `holdWork`): on `source` going pending,
 * whether verdict reader `n` waits on it instead of re-deriving. */ let holdWorkHook = null;

/** D-41's staged-error mark (verdict.ts `stageMark`): the settle walk's
 * reached node `n` carries staged error source `el`'s staging. */ let stageMarkHook = null;

/** #4417 (verdict.ts `keepReaders`); a landing's held re-derivation run (verdict.ts `attachWaits`). */ let keepReadersHook = null;

let attachHook = null;

/** #4544 (verdict.ts). Null without the verdict engine. */ let holdStagingHook = null;

/** D-8 (verdict engine), at the flush: the readers a tick netting to its
 * starting value woke are let go (verdict.ts `revert`). */ let revertHook = null;

/** Verdict engine, a pass's head (D-22, D-41: the held error `latest` keeps
 * showing) and, with `tail`, its end
 * (A19 exc. 2: whether it asked the committed question again). */ let passHook = null;

function installVerdictHooks(e, o, n, t, l, s, u, r, a, i) {
    revertHook = a;
    passHook = i;
    observeFlightHook = e;
    holdWorkHook = o;
    stageMarkHook = n;
    keepReadersHook = t;
    attachHook = l;
    holdStagingHook = s;
    laneAdoptHook = u;
    r5Hook = r;
}

/** External-source bridge (wired by enableExternalSource(); undefined while
 * no config is active — including after _resetExternalSourceConfig()). */ let wireExternalSourceHook = undefined;

let externalUntrackHook = undefined;

function setExternalHooks(e, o) {
    wireExternalSourceHook = e;
    externalUntrackHook = o;
}

/** Live transactions — opened, unmerged, not landed. Scanned at every seam
 * while non-empty; nothing on the plain path. */ const transactions = [];

/** Observe tier: this flush parked its transaction (`holdStart` fired). */ let holding = false;

/** A transaction (listed here) or a lane (lanes.ts lists its own). */ function newTransaction(e, o = null) {
    const n = {
        ln: [],
        rn: [ [], [] ],
        fn: [],
        eo: null,
        _parent: o,
        P: e,
        Pe: null,
        tt: 0,
        nt: false,
        nn: null,
        Ue: false,
        un: false,
        I: null
    };
    if (!e) transactions.push(n);
    return n;
}

// Lanes (plan sec. 28, 2026-10-02; lanes.ts). A lane is "a new base of a transition":
// the sub-frame an optimistic write opens in the transition of the frame it
// is made in. It sees the screen plus its own guesses (plan sec. 19), breaks out of
// the parent's hold — its effects run now — and holds itself if its own
// derivations hit async. It ends when the parent lands: a guess reverts to
// the truth it covered, or lands as the truth that superseded it. A frame
// that does not park has nothing to be optimistic over: the write is void.

// The seat of a pass is its node's: a node carrying a lane's value runs as
// the lane's whoever dirtied it; a derivation's tracked read of a lane's
// value moves its pass into the lane; a leaf's never does (it reads the
// screen). Lane work's values go to the lane slot, its runs to the lane's
// queues; the rest of the frame is the flush's.
/** The lane the running pass is work of; set at `recompute`'s head from the
 * node, moved by a derivation's lane read, saved and restored around the
 * pass. */ let passLane = null;

function setPassLane(e) {
    passLane = e;
}

/** The transaction this flush parks into: set by the first join, merged
 * with every further one. Consumed at the seam. A join outside a flush (a
 * mount's pending observer, a write to a held node, an action opening)
 * schedules the seam that consumes it: membership is the tick's — the same
 * synchronous frame — and the next tick's writes are not this one's. */ let flushTransaction = null;

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
 * set it. */ let passTx = null;

function joinPassTx(e, o) {
    if (globalQueue.Y && !o || flushTransaction !== null) return joinFuture(e);
    if (passTx === null) passTx = resolveTx(e); else merge(resolveTx(e), passTx);
    schedule();
}

/** The transaction holding a node (CONFIG_HELD set), resolved through
 * merges; the path is compressed. */ function txOf(e) {
    return e.T.Re = resolveTx(e.T.Re);
}

/** `t` resolved through merges while it is still parked or open; null once
 * it has landed. */ function liveTx(e) {
    e = resolveTx(e);
    return transactions.indexOf(e) !== -1 ? e : null;
}

function resolveTx(e) {
    while (e.eo !== null) e = e.eo;
    return e;
}

/** The flush joins transaction `t` — or needs one of its own (`null`: a pass
 * went pending with nothing held). Joining a second one merges it into the
 * flush's: the work that touched both entangles them (A15). */
/** #4777: the flush gives back the hold a pass's plain read adopted
 * (`joinPass`) before that pass starts over (verdict.ts `markVerdictReader`),
 * and every hold the pass's later plain reads merged into it (`merge`, in
 * reverse): each is its own again, as before the pass. */ function dropFlushTx() {
    for (let e = adopted.length - 2; e >= 0; e -= 2) {
        const o = adopted[e], n = o.eo;
        o.eo = null;
        n.tt -= o.tt;
        n.nt = adopted[e + 1];
        unappend(n.ln, o.ln);
        unappend(n.rn[0], o.rn[0]);
        unappend(n.rn[1], o.rn[1]);
        unappend(n.fn, o.fn);
        unappend(n.I, o.I);
        transactions.push(o);
    }
    adopted.length = 0;
    flushTransaction = null;
}

function unappend(e, o) {
    if (o) for (let n = o.length; n--; ) e.splice(e.lastIndexOf(o[n]) >>> 0, 1);
}

function joinFuture(e) {
    // A lane is never the flush's transaction (D-37, #4440).
    if (laneStageHook && e?.P) return;
    const o = flushTransaction;
    if (o === null) {
        flushTransaction = e === null ? newTransaction(false) : resolveTx(e);
    } else if (e !== null) merge(resolveTx(e), o);
    if (!globalQueue.Y) schedule();
}

/** `t` merges into `f`: the work that touched both entangles them (A15).
 * A lane never merges: it is a sub-frame, not a peer — its work is routed
 * per pass (`passLane`). */
/** #4777: the merges made since a pass adopted the flush's hold (core.ts
 * `adopter`), as [t, f._acted before]: a restart gives them back too. */ const adopted = [];

function merge(e, o) {
    // Both ends resolved: a stale pointer to a transaction already merged
    // into the other would close a cycle `resolveTx` never leaves.
    e = resolveTx(e);
    o = resolveTx(o);
    if (e === o || e.P || o.P) return;
    if (observeFlightHook && adopter) adopted.push(e, o.nt);
    e.eo = o;
    o.tt += e.tt;
    o.nt ||= e.nt;
    append(o.ln, e.ln);
    append(o.rn[0], e.rn[0]);
    append(o.rn[1], e.rn[1]);
    append(o.fn, e.fn);
    if (e.I !== null) append(o.I ??= [], e.I);
    transactions.splice(transactions.indexOf(e), 1);
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
 * in for the flight (A17: "visible until its own fetch settles"). */ function blocked(e) {
    // An action still running in it holds it open (action.ts). Its lanes
    // (lanes.ts): a lane under `t` blocks it while blocked itself.
    return e.tt !== 0 || blockedBy(e.ln, e) || !!lanesBlockedHook?.(e);
}

/** The transaction being judged — the outermost `blocked` of a nest (a lane
 * consulted on its behalf judges for it): a zombie whose removal it stages
 * is moot for it alone (`onScreen`). Set around the lanes' hook
 * (`installLaneHooks`), the only path that nests; null otherwise, where the
 * transaction judged is `blockedBy`'s own `owner`. */ let judge = null;

/** `t`'s own flights only — not its lanes': a flight the frame asked for
 * (a refetch, a plain load) is authoritative; a lane's derivation flight is
 * not (lanes.ts, the body-end corollary). A pending reader counts only if
 * one of the sources it waits on is `t`'s (a render effect marked pending
 * by a lane flight, never re-run, is the lane's wait, not the frame's). */ function ownFlights(e) {
    return blockedBy(e.ln, e, ownSource);
}

/** A source the pending node waits on that is `t`'s (held by it, or by no
 * one) — not another transaction's or lane's work. Itself, when it is the
 * source. */ function ownSource(e, o) {
    const n = e.T?.se;
    if (n === undefined) return true;
    for (const e of n) {
        const n = e.T?.Re;
        if (n == null || resolveTx(n) === o) return true;
    }
    return false;
}

/** `own`: the pending node's filter (`ownFlights` passes `ownSource`) — a
 * parameter, not a flag, so a build without lanes or guesses (the only
 * callers of `ownFlights`) does not retain `ownSource`. */ function blockedBy(e, o, n) {
    for (let t = 0; t < e.length; t++) {
        const l = e[t];
        if (!(l.m & STATUS_PENDING) || l.R & REACTIVE_DISPOSED || l.l & CONFIG_GUESS || l.T.Re !== null && txOf(l) !== o || n && !n(l, o) || 
        // A guess lane's member waiting on re-asks only (`reasks`) keeps what
        // it shows, and a `refresh()` holds nothing new (D-34 ¶3): it does not
        // keep the lane's other work off the screen (#4834: a memo over the
        // shown guess and a fresh answer stayed on its stale committed value).
        guessing && o.P && l.T.se && reasks(l)) continue;
        // The frame's own pending observer (off screen: this one holds nothing;
        // the next listed node may).
                if (l.i === EFFECT_RENDER) {
            // #4544 (F-6): a frame born into the hold that is pending on what a
            // settled verdict reader's display-ahead value asked for (`read`,
            // CONFIG_AHEAD) does not hold it; the reader re-derives at the
            // landing, and the frame with it. A lane's reveal still waits (S-29).
            if (!(observeFlightHook && l.l & CONFIG_AHEAD && !o.P) && onScreen(l, judge ?? o)) return true;
            continue;
        }
        for (let e = l.o; e !== null; e = e.t) {
            const n = e.u;
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
                        if ((n.i === EFFECT_RENDER && n.m & STATUS_PENDING && !(observeFlightHook && n.l & CONFIG_AHEAD && !o.P) || n.R & REACTIVE_FRAME_READ || n.l & CONFIG_REDERIVE && n.m === STATUS_PENDING) && (e.Ne === n.de || n.T?.U != null) && !(n.R & REACTIVE_DISPOSED) && onScreen(n, judge ?? o)) return true;
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
 * nothing — boundaries.ts). */ function onScreen(e, o) {
    return !hiddenHook?.(e) && !(e.R & REACTIVE_ZOMBIE && removalStagedBy(e, o));
}

/** A15 (#3463): a reader whose removal is staged — a zombie, parked by a
 * pass awaiting its commit — is still on screen and live for every hold,
 * except the transaction staging its removal: for that one its say is moot
 * (done, and the commit disposes it; not done, and it stays parked). The
 * staging transaction is the one holding the pass that parked the frame:
 * the nearest non-zombie ancestor. */ function removalStagedBy(e, o) {
    while (e?.R & REACTIVE_ZOMBIE) e = e._parent;
    return e?.l & CONFIG_HELD && e.T?.Re != null && txOf(e) === resolveTx(o);
}

/** Unchanged passes this flush whose stale dependency tail awaits the
 * flush's verdict (A30, #3469): trimmed when the flush commits, kept when
 * it parks — the committed frame still derives from the previous pass's
 * dependencies, and a write to one of them must reach the node until a
 * committing pass replaces it. */ const heldTrims = [];

function heldTrim(e) {
    heldTrims.push(e);
}

/** Zombies popped from the dirty heap this flush (`deferZombie`). The seam
 * decides their fate: a park cancels them (the writes that dirtied them are
 * held with it); a commit runs the survivors — the ones whose owner is still
 * awaiting its commit — in height order, after the commits that would have
 * disposed them (#3546). No second heap: the list is the deferral. */ const deferredZombies = [];

function deferZombie(e) {
    deleteFromHeap(e, dirtyQueue);
    e.R &= -4;
    deferredZombies.push(e);
}

/** Put a staged node in a transaction: flagged, stamped, on its landing
 * list. */ function holdNode(e, o) {
    e.l |= CONFIG_HELD;
    list(e, o);
}

/** Put a node on a transaction's list, once: a node's `_transaction` is the
 * transaction whose `_nodes` it is on (every site that sets one does the
 * other), so a re-pass, a re-landing or a re-staging adds nothing. */ function list(e, o) {
    const n = ext(e);
    if (n.Re === o) return;
    n.Re = o;
    o.ln.push(e);
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
 * retired at the lane's seam. */ function holdFrame(e, o) {
    for (let n = e.pe; n !== null; n = n.Ge) {
        if (n.T?.Re?.P) continue;
        if (n.X !== undefined) {
            if (!(n.l & CONFIG_HELD)) holdNode(n, o);
            n.l |= CONFIG_HELD_FRAME;
        }
        holdFrame(n, o);
    }
}

/** The nodes under `owner` a transaction other than `t` holds (boundaries.ts:
 * what an earlier hold staged under a re-armed boundary). */ function heldUnder(e, o, n) {
    for (let t = e.pe; t !== null; t = t.Ge) {
        if ((t.l & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD && txOf(t) !== o) n(t);
        heldUnder(t, o, n);
    }
}

/** `n` leaves the hold it was staged in: its next pass publishes mainline. */ function unhold(e) {
    e.T.Re = null;
    e.l &= ~CONFIG_HELD;
}

const dirtyQueue = {
    et: new Array(2e3).fill(undefined),
    p: false,
    Fe: 0,
    Et: 0
};

let clock = 0;

let scheduled = false;

let halted = false;

let haltNotified = false;

let syncDepth = 0;

/**
 * Toggles the dev-mode "must be inside a `<Loading>` boundary" enforcement
 * window. Only `render()` calls this — wrapping the initial mount so that a
 * top-level uncaught async read surfaces the diagnostic. Not part of the
 * user-facing API.
 *
 * @internal
 */ function enforceLoadingBoundary(e) {}

// Dev-only marker for the effect half of createEffect/createRenderEffect, so
// flush() can report the no-op instead of failing silently (React parity).
/** @internal Action bodies on the stack (action.ts): `flush()` is refused
 * inside one — the body's writes are its transaction's and commit when it
 * settles; a drain mid-step cannot reveal them, and dev says so. Read live
 * by `@solidjs/web`'s server function transport: a call made while it is
 * nonzero is the action's mutation (#4913). */ let actionDepth = 0;

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
 * silently, it does not move the graph. */ let question = 0;

let questions = 0;

function setQuestion(e) {
    question = e;
}

function nextQuestion() {
    return ++questions;
}

const MAINLINE_QUESTION = 2147483647;

let inEffectCallback = false;

/** Dev-only: matches no event, so a window or phase holding it applies
 * whatever event is current (see `strictReadEvent`, core.ts). */ const ANY_EVENT = {};

/** Dev-only: the DOM event current when the effect phase was entered. A
 * handler dispatched synchronously from that phase (`el.focus()`) runs under
 * a different event and is not in it (#4235). */ let effectCallbackEvent = ANY_EVENT;

/** Dev-only: the context a callback scope runs in — null for an effect
 * callback, the creating pass's for a ref callback (`runRefCallback`, #4761).
 * A primitive created in that context is created in the callback; one
 * created by a computation the callback pulls is that computation's. */ let effectCallbackOwner = null;

function setEffectCallback(e, o = ANY_EVENT, n = null) {}

function schedule() {
    if (halted) {
        notifyHalted();
        return;
    }
    if (scheduled) return;
    scheduled = true;
    if (!syncDepth && !globalQueue.Y) {
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
 */ const ROOT_ERROR_HOOK = Symbol.for("solid-js/root-error-hook");

const HALTED = "[REACTIVITY_HALTED]";

function haltReactivity(e) {
    if (halted) return;
    halted = true;
    // A cleanup error collected before the halt is dropped (owner.ts).
        rethrowCleanupError(true);
    let o = HALTED;
    // Surface the cause here too: callers rethrow it, but a creation-time throw
    // unwinds through ancestor recomputes that convert it to status instead of
    // surfacing it (#2884), so the rethrow alone cannot guarantee visibility.
    // Where the platform has one, hand the cause to its uncaught-error channel
    // (`reportError` → `error` event → window.onerror / error monitoring): a
    // halt that only reaches console.error leaves a page that LOOKS alive with
    // nothing an app or its telemetry can act on (#3338 — an uncaught throw
    // during the hydration render). The rethrow may reach the top as well in
    // the non-swallowed cases; a duplicate report beats a silent one.
        const n = e !== undefined && globalThis.reportError;
    n || e === undefined ? console.error(o) : console.error(o, e);
    n && n(e);
}

/**
 * Dev: `haltReactivity`, located at `subject` — the computation the uncaught
 * error escaped from — for the REACTIVITY_HALTED message's owner path. Call
 * sites pick it in dev builds only (`haltReactivity(e)` in production), so
 * production keeps the one-argument halt and its bytes unchanged.
 */ function haltReactivityAt(e, o) {
    haltReactivity(e);
}

// Logs on the first write after a halt so a frozen interaction is traceable.
function notifyHalted() {
    if (haltNotified) return;
    haltNotified = true;
    console.error(HALTED);
}

/** @internal Test/dev-reload hook. Revives scheduling after a halt. */ function resetErrorHalt() {
    halted = false;
    haltNotified = false;
    // A throw out of the pure phase skips the `scheduled` recompute; re-arm it.
        if (scheduled) {
        scheduled = false;
        schedule();
    }
}

class GlobalQueue {
    rn=[ [], [] ];
    Y=false;
    flush() {
        if (this.Y) return;
        this.Y = true;
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
            runHeap(dirtyQueue, GlobalQueue._e);
            this.settle();
            clock++;
            // A write the commit sweep staged with no subscriber to dirty is work
            // too — the next round commits it.
                        scheduled = dirtyQueue.Et >= dirtyQueue.Fe || pendingNodes.length !== 0;
            this.run(EFFECT_RENDER);
            this.run(EFFECT_USER);
            if (false && holding) ;
            if (false && !scheduled) ;
            if (false) ;
        } finally {
            this.Y = false;
            passTx = null;
        }
    }
    /** L2 — the seam: end of the pure phase. Commit this flush's staged nodes,
     * or park them with the transaction the flush joined; then land every
     * transaction no frame is waiting on. The landing flush is an ordinary
     * flush: the settle walk (async.ts) re-ran the parked readers in its pure
     * phase, and here no flight the transaction holds has a frame deriving
     * from it (`blocked`). */    settle() {
        // Lanes: an ended action body judges its guesses now that their passes
        // have run (lanes.ts); a correction's re-derivations run in one more
        // pure round — the parent's flights before the parent is judged below.
        if (laneCorrectionsHook?.()) runHeap(dirtyQueue, GlobalQueue._e);
        const e = flushTransaction;
        flushTransaction = null;
        // The frame's verdict is in: the guesses written since the last seam
        // open their lane under the frame's transaction, or are dropped — the
        // frame commits, and there is nothing to be optimistic over. (A lane is
        // never the flush's transaction, D-37: lanes do not nest.) Their passes run next round;
        // a parent landing at this very seam ends the lane before they do.
                applyGuessesHook?.(e);
        // A lane's own flush (its flight landing, its reveal) parks nothing: the
        // lane's work reached it per pass; the frame's plain writes commit.
        // (Resolved: a guess over another transaction's guess merged them.)
                const o = e !== null && !e.P ? resolveTx(e) : null;
        if (o !== null) {
            // `t._nodes` grows under the walk (holdFrame pushes); the bound is
            // this flush's list, which does not.
            const e = pendingNodes.length;
            for (let n = 0; n < e; n++) {
                const e = pendingNodes[n];
                // Already held: the pass that re-staged it joined its transaction,
                // which is `t` (or merged into it). Lane work is the lane's (a
                // pending mark propagated onto it queued it here too).
                                if (!(e.l & (CONFIG_HELD | CONFIG_OVERRIDE)) && (e.T === null || e.T.Re === null || !e.T.Re.P)) {
                    // A loading source — pending with nothing staged and no committed
                    // value (A16, A19 exc. 1: uninitialized is loading, not pending)
                    // that is not an observer — is nobody's frame: not held. Its
                    // first landing is commit #0, not a resumption of this frame, and
                    // a write that reaches it later overlaps nothing (#2937: the
                    // stale stamp was the bridge that entangled unrelated updates
                    // with a never-resolving flight). Its observers are held and
                    // wait on its flight (`blocked`).
                    if (e.ue === NOT_PENDING && e.m & STATUS_UNINITIALIZED && e.i !== EFFECT_RENDER) continue;
                    // A34 (2): a tick whose writes net to the committed value made no
                    // proposal (`setShow(false); setShow(true)` on a committed true) —
                    // not staged, not held, pends nothing; a later write to it is
                    // plain mainline. (A node already held stays held when written
                    // back: A34 (1).)
                                        if (e.ue !== NOT_PENDING && !(e.m & (STATUS_PENDING | STATUS_UNINITIALIZED)) && e.T?.me == null && e.T?.Le == null && 
                    // A recovery to the last good value is a change of answer
                    // (D-41): the error it replaces stays the display until it
                    // commits, so it is held, not netted.
                    !e.T?.Te && 
                    // The comparator last: a pass that created children (an object
                    // owning a fresh async memo, #4111) must not have it read them.
                    // A NotReady read there is a change (`isSame`).
                    isSame(e, e.le, e.ue)) {
                        e.ue = NOT_PENDING;
                        e.l &= ~CONFIG_STAGED;
                        continue;
                    }
                    holdNode(e, o);
                }
                // A held pass's frame is held with it (ruling A, O2): the children it
                // created are the transaction's — they compute, and a pass over them
                // or a write that reaches them joins; they effect at the landing.
                                if (e.l & CONFIG_STAGED) holdFrame(e, o);
            }
            pendingNodes.length = 0;
            storeParkHook?.(o);
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
                        const n = deferredZombies.splice(0);
            if (observeFlightHook) {
                n.sort((e, o) => e.he - o.he);
                for (let e = 0; e < n.length; e++) if ((n[e].R & (REACTIVE_DISPOSED | REACTIVE_ZOMBIE)) === REACTIVE_ZOMBIE && n[e].l & CONFIG_VERDICT) recompute(n[e]);
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
                        for (let e = 0; e < heldTrims.length; e++) {
                const o = heldTrims[e];
                if (o.T?.U == null && !o.Ae) trimStaleDeps(o);
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
                const n = this.rn;
        let t = null;
        // The passes that read this frame's stagings as the screen read held
        // writes if it parked: the lane seam re-derives them on the committed
        // world (a lane's runs wait this round).
                if (laneSeamsHook) {
            this.rn = t = [ [], [] ];
            laneSeamsHook(o);
            if (stagedReaders.length) stagedReaders.length = 0;
        }
        this.rn = [ [], [] ];
        for (let e = transactions.length - 1; e >= 0; e--) {
            const o = transactions[e];
            if (blocked(o)) continue;
            transactions.splice(e, 1);
            endLanesHook?.(o);
            land(o);
            // A landing's commits can dispose a zombie a transaction judged above
            // was blocked on (#3463: live "until the commit that disposes it") —
            // judge them again.
                        e = transactions.length;
        }
        // The store folds the pending backings whose container nodes committed
        // — this flush's, or a landing's (store/store.ts installs it).
                storeCommitHook?.();
        // Marks declared outside a transaction: the flush that carried them
        // parked (something async below) — the window is the frame's, the marks
        // are its transaction's; else they release at the seam, verdict-only,
        // nothing to show (affects.ts).
                const l = o && liveTx(o);
        releaseAmbientMarksHook?.(l);
        // The effect phase: lane work first (displayed ahead of the frame), then
        // this flush's runs, then the runs the landings held from earlier
        // flushes — as one pass would have queued them (#3540: a shell
        // re-derived at the landing before a boundary swap held since the
        // write; #3528: a display-ahead swap before the count it is shown
        // beside). A parked frame's runs wait with its transaction instead: the
        // effect phase applies a committed frame.
                const s = l !== null;
        for (let e = 0; e < 2; e++) {
            if (s) append(l.rn[e], n[e]);
            // The plain flush — no reveal, no landing, not parked — has nothing to
            // order around its own runs: they are the queue as they stand (the
            // same array, not a copy; `run` takes it whole). Every other seam
            // builds the ordered queue above.
                        this.rn[e] = s || t?.[e].length || this.rn[e].length ? (t ? t[e] : []).concat(s ? [] : n[e], this.rn[e]) : n[e];
        }
        if (deferredZombies.length !== 0) {
            // Survivors are the displayed frame of a held pass: they rerun for the
            // live write, parents before children.
            const e = deferredZombies.splice(0).sort((e, o) => e.he - o.he);
            for (let o = 0; o < e.length; o++) {
                if (!(e[o].R & REACTIVE_DISPOSED)) recompute(e[o]);
            }
        }
        // Boundaries: readers that settled, landed or died this flush are
        // dropped; a fallback with none left reveals next round.
                boundarySeamHook?.();
    }
    run(e) {
        const o = this.rn[e - 1];
        if (o.length) {
            this.rn[e - 1] = [];
            runQueue(o, e);
        }
    }
    enqueue(e, o, n = passLane) {
        // Lane work's runs are the lane's: they run at its reveal, not with
        // the frame (which may park). A lane pass that read the frame's own
        // staging is the frame's (`recompute` passes null): its run waits with
        // the frame. So is a run given a transaction that is not a lane (a
        // first run born held, `effect()`).
        if (e) (n?.P ? n : this).rn[e - 1].push(o);
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
     * never hears of it. */    notify(e, o, n, t) {
        if (catchHook?.(e, n & o, t)) return true;
        if (o & STATUS_PENDING) {
            if (n & STATUS_PENDING) {
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
 * it. Truthy when an effect re-derives (`land`). */ function reruns(e) {
    let o;
    for (let n = 0; n < e.fn.length; n++) {
        const t = e.fn[n];
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
                const l = t.T?.U?.source;
        if (t.R & REACTIVE_DISPOSED || t.m & STATUS_ERROR && (t.i || !(t.l & CONFIG_FRAME)) && l !== t && (l?.m ?? STATUS_ERROR) & STATUS_ERROR) continue;
        if (t.R & REACTIVE_FRAME_READ || observeFlightHook && t.l & CONFIG_VERDICT) {
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
            o = 1;
            // A lane's reveal leaves the mark to the transaction: a reader that
            // also read a held node as committed (`staleReader`) still observes
            // that flight until it re-runs — cleared here, the transaction landed
            // at this seam with the flight up and voided the reveal's run (#4397).
                        if (!e.P) t.R &= ~REACTIVE_FRAME_READ;
            t.Ae = false;
            // Its frame is replaced by the re-derivation: a run one of its
            // children queued this flush (re-run by the same landing) is for the
            // frame being replaced — void too (#3404).
                        for (let e = t.pe; e !== null; e = e.Ge) if (e.i) e.Ae = false;
            // A frame flight the seam parked (verdict.ts `r5`, R5) asked the
            // question this landing would: adopted, it is asked once (F-9).
                        laneAdoptHook?.(t);
            enqueueSub(t);
        }
    }
    e.fn.length = 0;
    return o;
}

/** The landing: the held frame becomes the frame. */ function land(e) {
    const o = reruns(e);
    if (releaseMarksHook && e.I !== null) releaseMarksHook(e.I);
    // Old children die in the commits (cleanups first), then the stashed
    // effects run ahead of this flush's own. A node a lane took over since
    // (a guess written over the staged truth) is the lane's to land.
        for (let o = 0; o < e.ln.length; o++) {
        const n = e.ln[o];
        if (n.T.Re === null || txOf(n) !== e) continue;
        n.T.Re = null;
        commitPendingNode(n);
    }
    // A cleanup those commits ran that wrote a node `u` still held joined
    // `u`, which has left `transactions`: the write is this landing's, and
    // the next flush must not park in it (#4089).
        if (flushTransaction === e) flushTransaction = null;
    // An effect re-derives next round (`reruns`): the held runs wait for it
    // (D-9, the A15 reveal corollary, #4071). Its re-derivation can discover a
    // flight — a stale reader whose mainline pass stopped reading it, now
    // reading the landed world — and a landing that showed its runs first
    // would reveal the held write beside a frame that can only show its old
    // value. Queued last in this effect phase, the release hands them to the
    // next round's own runs, ahead of the re-derivation's: they show with it,
    // or park with it in the hold its pending opens. The commits stand.
        if (o) globalQueue.rn[1].push(() => releaseQueues(e)); else releaseQueues(e);
}

/** The held runs join this flush's effect phase after its own runs (the
 * landing's re-passes, in height order): the frame's effects run as one
 * pass would have queued them — a shell re-derived at the landing before a
 * boundary swap held since the write (#3540's log order). */ function releaseQueues(e) {
    for (let o = 0; o < 2; o++) {
        append(globalQueue.rn[o], e.rn[o]);
        e.rn[o] = [];
    }
}

/** `a.push(...b)` without the argument-count limit. */ function append(e, o) {
    for (let n = 0; n < o.length; n++) e.push(o[n]);
}

/** Nodes staged this flush (`_pendingValue` set), committed at its end. */ const pendingNodes = [];

/** Held slots rewritten since the last flush, with the verdict engine (D-8: `revertHook`). */ const heldRewrites = [];

/** Passes that read a staging of this flush as the screen — lane work
 * (core.ts `stagedRead`), a verdict reader before the frame's verdict
 * (verdict.ts): if the frame parks they read a held write — re-derived next
 * round (lanes.ts; a lane's runs wait that round). */ const stagedReaders = [];

/** A15's stale reader: `c` was served the committed value of a node `t`
 * holds (a render effect, a verdict reader, lane work) and is re-derived
 * after `t`'s landing (`_reruns`). Called per read of a held node, so `t`'s
 * list holds one entry per such read, not one per pass: `reruns` re-runs the
 * reader at its first entry and skips the rest by the cleared flag. (A dedupe
 * here costs ~17-30 B minified in every client bundle, which the 10 KB
 * compiled floor cannot take; the repeats are inert and short-lived — the
 * list empties at the landing.) */ function staleReader(e, o) {
    // On each hold the pass reads committed from: one pass can read two (a memo
    // the flush's hold stages, a row another action's hold keeps), and each
    // landing re-runs it. Guarded by the flag alone, the reader was listed on
    // the first hold only and missed the second's landing (#4932).
    o.fn.push(e);
    e.R |= REACTIVE_FRAME_READ;
}

function queuePendingNode(e) {
    pendingNodes.push(e);
    if (!globalQueue.Y) markUnflushedStaged();
 // A28
}

/** §12d: bumped by every recompute and every new subscriber edge. A node's
 * staged-rewrite skip is sound only while NOTHING recomputed or linked since
 * its last notify — a mid-batch pull can clean a marked subscriber, and a
 * skipped re-write would leave it stale. */ let notifyEpoch = 0;

function bumpNotifyEpoch() {
    notifyEpoch++;
}

function insertSubs(e) {
    // §12d: stamp before walking — setSignal's staged-rewrite fast path skips
    // the next walk for this node while the epoch holds (marking is idempotent).
    e.we = notifyEpoch;
    // Presence bits gate the optional-slot probes (see constants.ts): one
    // masked read of the always-present _config instead of missing-property
    // lookups in the hottest notify loop.
        const o = e.l;
    const n = (o & CONFIG_HAS_SNAPSHOT) !== 0 && e.T?.De !== undefined;
    for (let o = e.o; o !== null; o = o.t) {
        const t = o.u;
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
                if (t.R & REACTIVE_RECOMPUTING_DEPS && o.Ne === t.de && (o !== t._ || !e.X)) t.R |= REACTIVE_MISSED_WAKE;
        if (n && t.l & CONFIG_IN_SNAPSHOT_SCOPE) {
            t.R |= REACTIVE_SNAPSHOT_STALE;
            continue;
        }
        enqueueSub(t);
        if (t.R & REACTIVE_OWNER_READS) markNode(t._parent, REACTIVE_CHECK);
    }
}

/** Every flight `c` waits on is a `refresh()` re-ask (`_reask`): its own,
 * with no sources, or each source's, through a source that waits on
 * another's flight (a projection over the re-asked memo, #4824). The
 * propagated copies can form a cycle: two keyed projections whose derives
 * each read a row lent from the other's store are each pending on the
 * other's derive (D-1, D-9: a read of a pending leaf is pending on it), and
 * both on the real flight (#4969). A source already walked adds no flight of
 * its own; the flights at the ends of the walk decide. */ function reasks(e, o = new NewSet) {
    return [ ...e.T.se ?? [ e ] ].every(n => n === e ? n.T.Ve : o.has(n) || (o.add(n), 
    reasks(n, o)));
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
 * `laneRead` re-derives it on the screen if that lane holds (`entered`). */ function laneDirty(e, o) {
    insertSubs(e);
    // A store's guess carrier: the readers of the keys it changed read their
    // leaves — dirtied as the lane's work too (store/optimistic.ts).
        if (e.l & CONFIG_SLOT_NODE) GlobalQueue.lo?.(e, o);
    for (let n = e.o; n !== null; n = n.t) {
        const t = n.u;
        let l = t.T?.Re, s = t.i === EFFECT_RENDER, u;
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
                if ((t.m & (STATUS_PENDING | STATUS_UNINITIALIZED)) === STATUS_PENDING && (
        // A render effect pending on another hold's re-asks, held by it or
        // by none, the same (#4824): its first read of the re-asked rows threw
        // it back out of the lane, and the guess beside a reader that showed
        // it waited on the refresh (F-1, F-11).
        l ? (t.l & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD : s) && (u = !!l && resolveTx(l) === resolveTx(o._parent), 
        t.i ? s && !u && reasks(t) : u || reasks(t))) list(t, l = o);
        if (s && e.l & CONFIG_VERDICT) t.l |= CONFIG_VERDICT, t.R &= ~REACTIVE_LANE_DIRTY;
        if (l?.P ? sameLane(l, o) : t.i) t.R |= REACTIVE_LANE_DIRTY;
    }
}

/** An effect owes a run: marked, and its bound runner queued in its phase. */ function queueEffectRun(e) {
    e.Ae = true;
    globalQueue.enqueue(e.i, e.W ??= GlobalQueue.q.bind(null, e));
}

/** One lane, or two of one link group (#3335): one reveal unit. */ function sameLane(e, o) {
    return e === o || e.nn !== null && e.nn === o.nn;
}

/** A staged value becomes the node's value: truthy when one was staged. */ function applyStaged(e) {
    return e.ue !== NOT_PENDING && (e.le = e.ue, e.ue = NOT_PENDING);
}

function superseded(e) {
    return !(e.m & STATUS_UNINITIALIZED) && !!e.T?.Ee && !e.T.Ve && (e.ue = NOT_PENDING, 
    true);
}

function commitPendingNode(e) {
    // L2: the commit is where a pass, held or not, stops being uncommitted —
    // unless it has no answer yet. A pass still in flight (pending, nothing
    // staged) stays uncommitted: a re-pass disposes its children on the spot,
    // and the frame it parked stays the frame until a pass of this node
    // commits a value.
    if (!e.X) {
        e.l &= -385;
        applyStaged(e);
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
        const o = (e.m & STATUS_PENDING) !== 0 && (e.ue === NOT_PENDING || superseded(e));
    const n = e.l & CONFIG_STAGED_ERROR;
    // An answer commits: the error it replaces leaves the display (D-41).
        if (!o && e.T) e.T.Te = null;
    e.l = o ? e.l & -1048833 | CONFIG_INPUTS_PUBLISHED : e.l & -3147137;
    // D-41: the staged answer is an error — published now, when a value would
    // show, through the ordinary error path (`stageError`). Its readers first
    // see it in the cycle after this seam: a tracked re-read there is not a
    // retry (`read`), or a boundary re-derived by it would retry it forever.
        if (n) {
        const o = e.ue;
        e.ue = NOT_PENDING;
        e.ie = clock + 1;
        notifyStatus(e, STATUS_ERROR, o);
    } else if (applyStaged(e)) {
        // A node born staged (recompute) initializes at this commit.
        e.m &= ~STATUS_UNINITIALIZED;
        // A quiet re-ask's classification survives its landing and dies with
        // the landed value's commit (A19 exc. 2, #3178): verdict-quiet through
        // the reveal. (The sweep of a node still in flight is not a landing.)
        // The A28 stash goes with it.
                if (e.T != null) e.T.Ve = false, e.T.je = NOT_PENDING;
        // An effect with a staged value was born held (L2): its first run is
        // this commit's, not its creation's (A29) — queue it now.
                if (e.i && e.i !== EFFECT_TRACKED) queueEffectRun(e);
    }
    // The committed value is the first observable answer for a loading-window
    // node — the window closes here, not at compute time (#2990). Unconditional
    // store to an always-present computed slot.
        e.Ie = false;
    e.R &= ~REACTIVE_MANUAL_WRITE;
    // The dependencies of the pass that produced the value are the frame's now:
    // the previous frame's tail goes (A30, #3410; `recompute` left it for a
    // staged pass). Only after a clean pass: `_error` is cleared by a clean
    // pass or by the node's own landing (whose pass was clean), so a set
    // `_error` means the last pass threw, kept its full list, and `_depsTail`
    // marks where it stopped.
        if (e.T?.U == null) trimStaleDeps(e);
    if (!(e.m & STATUS_PENDING)) e.m &= ~STATUS_UNINITIALIZED;
    // L2: the children this commit publishes are the frame's now — the frame
    // they replace, parked by the pass (`recompute`), goes.
        if (!o && e.T != null && (e.T.me !== null || e.T.Le !== null)) GlobalQueue.Ce(e, false, true);
}

function commitPendingNodes() {
    for (let e = 0; e < pendingNodes.length; e++) {
        const o = pendingNodes[e];
        // A node a lane pass re-staged this round is the lane's to reveal; one
        // a transaction holds (born held by a mainline pass, `passTx`) is that
        // transaction's to commit, at its landing.
                if (!(o.l & (CONFIG_OVERRIDE | CONFIG_HELD))) commitPendingNode(o);
    }
    pendingNodes.length = 0;
}

const globalQueue = new GlobalQueue;

function flush(e) {
    if (actionDepth > 0) {
        return e ? e() : undefined;
    }
    if (e) {
        syncDepth++;
        try {
            return e();
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
    if (globalQueue.Y) {
        return;
    }
    if (halted) return;
    while (scheduled) {
        globalQueue.flush();
    }
    // A cleanup that threw during the drain, now that the drain is complete
    // (owner.ts).
        rethrowCleanupError();
}

function runQueue(e, o) {
    for (let n = 0; n < e.length; n++) e[n](o);
}

export { ANY_EVENT, GlobalQueue, MAINLINE_QUESTION, ROOT_ERROR_HOOK, actionDepth, adopted, applyGuessesHook, applyStaged, armGuessing, attachHook, blocked, boundarySeamHook, bumpNotifyEpoch, catchHook, clock, commitPendingNode, deferZombie, dirtyQueue, dropFlushTx, effectCallbackEvent, effectCallbackOwner, endLanesHook, enforceLoadingBoundary, enterAction, exitAction, externalUntrackHook, flush, flushTransaction, freshHook, globalQueue, guessing, haltReactivity, haltReactivityAt, heldRewrites, heldRunHook, heldTrim, heldUnder, hiddenHook, holdNode, holdStagingHook, holdWorkHook, inEffectCallback, insertSubs, installBoundaryHooks, installGuessHooks, installLaneHooks, installMarkHooks, installStoreHooks, installVerdictHooks, isHidden, joinFuture, joinPassTx, keepReadersHook, laneAdoptHook, laneCorrectionsHook, laneDirty, laneOutcomeHook, laneReadHook, laneSeamsHook, laneServedHook, laneStageHook, laneWriteHook, lanesBlockedHook, list, liveTx, merge, newTransaction, nextQuestion, notifyEpoch, observeFlightHook, ownFlights, passHook, passLane, passTx, pendingNodes, question, queueEffectRun, queuePendingNode, r5Hook, reasks, releaseAmbientMarksHook, releaseMarksHook, releaseQueues, reruns, resetErrorHalt, resolveTx, revertHook, sameLane, schedule, setEffectCallback, setExternalHooks, setPassLane, setQuestion, setSwapHook, stageMarkHook, stagedReaders, staleReader, storeCommitHook, storeParkHook, swapHook, transactions, txOf, unhold, wireExternalSourceHook };