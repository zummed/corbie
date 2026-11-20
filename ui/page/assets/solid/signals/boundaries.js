import { STATUS_PENDING, STATUS_ERROR, CONFIG_REDERIVE, CONFIG_OVERRIDE, REACTIVE_LANE_READ, CONFIG_HELD, CONFIG_FRAME, STATUS_UNINITIALIZED, EFFECT_RENDER, CONFIG_VERDICT, REACTIVE_DISPOSED, REACTIVE_PROBED, CONFIG_STAGED_ERROR, REACTIVE_IN_HEAP, EFFECT_USER, EFFECT_TRACKED, REACTIVE_ZOMBIE, CAUGHT_REARMED } from "./core/constants.js";

import { signal, runWithOwner, computed, read, untrack, setSignal, recompute } from "./core/core.js";

import { forgetThrownRead, reportDiagnostic, emitDiagnostic } from "./core/dev.js";

import "./core/lanes.js";

import { globalQueue, haltReactivityAt, haltReactivity, laneStageHook, txOf, setPassLane, passLane, swapHook, schedule, joinFuture, GlobalQueue, flushTransaction, heldUnder, unhold, installBoundaryHooks } from "./core/scheduler.js";

import { NotReadyError, unwrapStatusError } from "./core/error.js";

import { isThenable } from "./core/async.js";

import { reportClientError } from "./core/error-hooks.js";

import { link } from "./core/graph.js";

import { enqueueSub } from "./core/heap.js";

import { createOwner, getOwner, cleanup, dispose } from "./core/owner.js";

import { flatten } from "./flatten.js";

import { accessor } from "./signals.js";

/** Boundaries — the display consumers between a frame reader and the root:
 * `createLoadingBoundary` (`<Loading>`) and `createErrorBoundary`
 * (`<Errored>`).
 *
 * A boundary is an owner whose context names it (`BOUNDARY`), so every node
 * created under it finds it in O(1) and the enclosing boundaries through
 * `_parent`; a `tree` (the content, flattened) and an `output` that shows
 * the tree or the fallback. The output is a frame reader (CONFIG_REDERIVE):
 * status reaching it from its content re-derives it instead of marking it,
 * and its pass decides.
 *
 * Status: `GlobalQueue.notify` asks `catchStatus` before the root. The
 * nearest collecting boundary of that status records the reader and shows
 * its fallback, and the root never hears of it: a fallback-caught flight
 * holds nothing (F-3, F-5). A loading boundary showing content forwards.
 *
 * The hold asks `hiddenBy`: a reader behind any fallback is not on screen
 * and holds nothing (F-3).
 *
 * Reveal: the seam (`boundarySeam`) drops readers that settled, died, or
 * landed and committed; a fallback with none left re-derives its output. A
 * held reader is not settled: the content reveals at the commit (#3540).
 *
 * `on` (loading only): a pass of it after the mount arms the boundary for
 * the flush; anything pending under it is collected and the fallback shows
 * with the change's frame (D-17). */
/** Context key: the nearest boundary of a node, inherited at creation. */ const BOUNDARY = Symbol("");

/** Context key: the reveal controller a Loading boundary created here is a
 * slot of (reveal.ts); a boundary clears it for its content — only direct
 * children are slots. */ const REVEAL = Symbol("");

// Every call site checks the hooks themselves first (not only the slot or
// controller, which are never set without them): a build that does not
// import `createRevealOrder` folds the reveal-order arms away.
let revealHooks = null;

function setRevealHooks(e) {
    revealHooks = e;
}

/** Boundaries with readers to sweep (or an arm to resolve) at the seam. */ const collecting = new Set;

const boundaryOf = e => e.A[BOUNDARY];

/** An error boundary at or above `b` shows its fallback: content under it
 * is not on screen. */ const behindError = e => !!e && (e.i === STATUS_ERROR && e.L || behindError(e._parent));

/** Catches what reaches it: an error boundary always; a loading one until it
 * has shown content, or while re-armed. Behind an error fallback nothing is
 * on screen, so the error boundary's recovery re-arms the subtree: a loading
 * boundary there has not shown content (D-41, F-4, #4627) — a query; the
 * boundary is fresh again once it catches (`caught`). */ const isCollecting = e => e.i === STATUS_ERROR || !e.D || e.h || behindError(e._parent);

/** The nearest loading boundary at or above `b` catches what is pending
 * (#4926). */ const freshAbove = e => !!e && (e.i & STATUS_PENDING ? isCollecting(e) : freshAbove(e._parent));

/** The statuses that keep reader `r` of `b`: `b`'s own and pending — and,
 * for a loading boundary behind an error fallback, the error of a reader
 * directly under it (an error boundary between them shows that error
 * itself; a held signal it waits on has no context): there is nothing to
 * reveal, the error shows above. Revealing re-derived the content, whose
 * re-read retried the error's source: pending again, caught again, without
 * end (#4870). The error's recovery answers the reader, which reveals it
 * with the recovered content. */ const keeps = (e, n) => e.i | STATUS_PENDING | (behindError(e._parent) && n.A?.[BOUNDARY] === e ? STATUS_ERROR : 0);

/** A reader `b` waits on: recorded for its `on`, and `b` visited at the seam. */ function record(e, n, r = n) {
    e.F.set(n, r);
    collecting.add(e);
}

/** The reader goes to the fallback: recorded, and the output re-derives
 * (a no-op while the output's own pass is running — its post-check
 * decides). */ function caught(e, n, r, o = n) {
    if (e.i === STATUS_ERROR) {
        // The error's source is the retry target, but the reader's selected
        // outcome owns publication. Do not lose a latest/optimistic lane when
        // converting its error into fallback content.
        // (Only the lane engine makes lanes: without it the clause folds away.)
        const t = laneStageHook ? passLane ?? (n.T?.Re?.P ? txOf(n) : null) : null;
        if (laneStageHook && !e.L) e.P = t;
        // An error boundary waits on what THREW (`StatusError.source`), not on
        // the reader that saw it, while the error is settled only when its
        // source is neither errored nor in flight and the reader is not pending
        // on the answer that replaces it (`prune`) — or when the
        // reader is disposed: a source no live reader holds holds nothing
        // (#4373). A source that died (a memo its reader owned, kept past the
        // pass that disposed it) settles nothing: the reader is waited on.
        // `reset` re-runs the source.
                r ??= n.T?.U;
        o = r?.source ?? n;
        if (o.R & REACTIVE_DISPOSED) o = n;
        const l = unwrapStatusError(r);
        // Subsequent errors update the existing fallback through this signal;
        // its message must publish with the swap, including after a first show.
                if (laneStageHook && t) laneStageHook(e.U, t, true, false);
        setSignal(e.U, l);
        reportClientError(l, e.H, o);
    }
    // A loading boundary that catches has not shown content: fresh until it
    // does — already so unless it caught behind an error fallback, which the
    // recovery leaves catching (D-41, #4627); a re-arm's `flip` says it too.
     else e.D = false;
    record(e, n, o);
    // A loading boundary behind an error fallback that catches a pending
    // holds nothing above it: the error boundary's recovery shows its
    // fallback beside the recovered content (D-41, #4627).
        if (e.h) flip(e);
    // Already showing the fallback: one more reader to wait on changes
    // nothing on screen. The fallback is instantiated once per show — a later
    // error reaches it through the `error` accessor, which is why it is one.
        if (!e.L) redraw(e);
    if (revealHooks && e.v !== null) revealHooks.V(e.v);
}

/** A re-armed boundary found something pending: fresh again, and the swap
 * the arming pass asked for is that pass's lane's (if any): the output is
 * the lane's work from its next pass's head — not a re-pass of the
 * transaction holding its pending content, which would entangle that
 * transaction with the arming frame's (#3540 `on: () => latest(dep)`). */ function flip(e) {
    e.D = false;
    e.P = e.B;
    const n = e.j;
    if (!e.P && n.l & CONFIG_HELD && txOf(n) !== flushTransaction) unhold(n);
    if (laneStageHook && e.P !== null && e.j !== null) laneStageHook(e.j, e.P, true, false);
}

function redraw(e) {
    if (e.j !== null && !(e.j.R & (REACTIVE_DISPOSED | REACTIVE_ZOMBIE))) {
        enqueueSub(e.j);
        schedule();
    }
}

/** `catchHook` (scheduler.ts): status from a frame reader, nearest boundary first.
 * A loading boundary on the way records a pending reader whether or not it
 * catches it (its `on` may collect it later). */ function catchStatus(e, n, r) {
    if (n === 0) {
        // A status cleared. Judged by the node's status now, against each
        // boundary's own rule (`prune`'s mask) — the error path clears pending
        // first and the node is still errored then. The last reader a fallback
        // waited on settling re-derives the output now, in the same heap (a
        // held reader's landing has joined its transaction, so the content
        // lands with its run, not a round after). Others still waiting: nothing
        // on screen changes, the fallback stays as it is. `node` here is an
        // effect reporting its own status. An error boundary records an effect
        // only while it is errored, and a staged error keeps STATUS_ERROR on a
        // node that had it (`stageError`), so `prune`'s CONFIG_STAGED_ERROR case
        // cannot reach this path (#4364). "Last" is judged as the seam would
        // (`prune`): readers the same held pass disposed — a list re-mapped
        // under its parked pass drops the rows that read the error — are gone,
        // and left for the seam, they revealed the stale content outside the
        // hold (#4526).
        if (collecting.size) for (let n = boundaryOf(e); n; n = n._parent) if (!(n.F.get(e)?.m & keeps(n, e)) && n.F.delete(e) && !prune(n, 0)) {
            if (n.L) redraw(n);
            if (revealHooks && n.v !== null) revealHooks.V(n.v);
        }
        return false;
    }
    for (let o = boundaryOf(e); o; o = o._parent) {
        if (!(o.i & n)) {
            // A pending passing an error fallback by (D-38) is still a reader the
            // content needs: revealed beside its stale value, the frame tears (D-41,
            // F-1, #4592).
            if (o.L && o.i > n) record(o, e);
            continue;
        }
        if (isCollecting(o)) {
            caught(o, e, r);
            return o.h ? CAUGHT_REARMED : true;
        }
        record(o, e);
    }
    return false;
}

/** The nearest loading boundary that would catch `node` was re-armed this
 * flush. */ function armedOver(e) {
    for (let n = boundaryOf(e); n; n = n._parent) if (n.i & STATUS_PENDING && isCollecting(n)) return n.h;
}

/** The nearest boundary on `r`'s chain showing its fallback, if any
 * (`hiddenHook`: `r` is behind a fallback). */ function hiddenBy(e, n) {
    for (let n = boundaryOf(e); n; n = n._parent) if (n.L) return n;
    // An error boundary output passing a pending by (D-38) is on screen only
    // through the render effects it reaches (F-3). Reaching none, or only
    // effects behind fallbacks, it holds nothing (F-4, D-12, #4638); `h` is `1`
    // then. Each node is walked once (`seen`), so a memo lattice costs its size.
        let r, o = e.o;
    if (n || e.l & CONFIG_REDERIVE) for (n ||= new Set, r = e.i != EFFECT_RENDER; o && (r = n.size == n.add(o.u).size || hiddenBy(o.u, n)); ) o = o.t;
    return r;
}

/** `heldRunHook` (scheduler.ts): a queued run behind a fallback waits for the
 * reveal (F-3). A user effect would read a DOM that is not attached; a
 * render update mounts Portals and head tags outside the hidden subtree. A
 * render effect's first run is synchronous on creation and never comes
 * here. The reveal re-queues the run by type (`release`). */ function heldRun(e) {
    const n = hiddenBy(e);
    // A swap to the fallback still staged in a hold (an `on` re-arm the
    // frame holds) has not reached the screen: the content is still the
    // display, and a run there — a guess written over it — applies now
    // (D-17, F-1, #4987).
    // So is one whose content a guess lane carries beside it (#4999; lanes
    // only, `laneStageHook`).
        if (n === undefined || n.j.l & (laneStageHook ? CONFIG_HELD | CONFIG_OVERRIDE : CONFIG_HELD)) return false;
    (n.M ??= new Set).add(e);
    return true;
}

/** The reveal: the runs held behind the fallback are this flush's, each in
 * its own phase — render updates with the frame that attaches the content,
 * user effects after it. */ function release(e) {
    if (e.M === null) return;
    for (const n of e.M) {
        const e = n.i;
        globalQueue.enqueue(e === EFFECT_TRACKED ? EFFECT_USER : e, e === EFFECT_TRACKED ? n.Z : n.W ??= GlobalQueue.q.bind(null, n));
    }
    e.M = null;
}

/** `r`, held by error boundary `b` for the error `s` threw, is pending
 * again after `s` answered, and a loading boundary between them that has
 * not shown content catches it: that boundary is its display, beside the
 * recovered content (D-41, F-4, #4627). Not while that boundary's swap to
 * its fallback is staged in a hold (a recovery written in an action's tick):
 * the error stays up until the hold reveals both (D-41, F-1). */ function caughtBelow(e, n, r) {
    if (!(r.m & (STATUS_ERROR | STATUS_PENDING) || n.m & STATUS_ERROR)) for (let r = boundaryOf(n); r && r !== e; r = r._parent) if (r.i & STATUS_PENDING && isCollecting(r)) return !(r.j.l & CONFIG_HELD);
    return false;
}

/** `n` reads `s`, directly or through derivations wherever they live (a
 * hoisted memo, a derived store's derive, a loading boundary's output). Each
 * upstream node is walked once (`seen`), so a shared upstream graph costs its
 * size, not its paths. */ function reads(e, n, r) {
    for (let o = e.C, t; o; o = o.G) if ((t = o.S) === n || (r ||= new Set).size < r.add(t).size && reads(t, n, r)) return 1;
}

/** Drop the readers that settled or died. Settled: a loading reader no
 * longer pending; an error's source neither errored nor in flight, or no
 * longer read by the reader that caught it (D-41, #4644). A held reader is
 * not settled: the content reveals at the commit. A reader pending again
 * after the source that threw answered has no new answer yet: the error
 * stays the display (D-41), unless a loading boundary under the error one
 * catches it (`caughtBelow`, #4627).
 *
 * `pass` 0: a reader clearing asks whether it was the last (`catchStatus`).
 * The first kept reader ends the walk, so a recovery of N readers walks
 * each once (#4526). */ function prune(e, n) {
    const r = e.i | STATUS_PENDING;
    for (const [o, t] of e.F) {
        if (!((o.R | t.R) & REACTIVE_DISPOSED)) {
            const l = keeps(e, o);
            // An error staged on its source is not settled: it publishes with the
            // commit (D-41, #4364). Nor a verdict reader queued to re-derive: its pass,
            // not its source's settling, is its answer (F-9, #4568).
                        if ((o.m & l || t.m & l && reads(o, t)) && !(r & STATUS_ERROR && caughtBelow(e, o, t)) || r & STATUS_ERROR && t.l & CONFIG_STAGED_ERROR || o.l & CONFIG_VERDICT && o.R & REACTIVE_IN_HEAP) {
                if (n === 0) break;
                continue;
            }
            if ((t !== e.K || e.h) && (t.l & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD) {
                // Landed, held: the content is that transaction's — the output's
                // pass enters it and the reveal lands with the reader's run. The
                // seam, past its park decision, waits for the commit instead. (A
                // lane's reader reveals with the lane, this round.) A boundary that
                // has not shown content catches it instead: the fallback stays until
                // the reader is committed (#3540: what a fresh boundary catches holds
                // nothing outside it). Not an error boundary (born `_initialized`):
                // what it caught in a hold was that hold's staged error (D-41), and an
                // answer landing in the same hold replaces it there — the content
                // reveals with the commit, and nothing ever shows the superseded
                // error (D-41, #4527).
                if (!n || !e.D) continue;
                joinFuture(txOf(t));
            }
        }
        e.F.delete(o);
    }
    return e.F.size;
}

/** Reveal order: a slot is ready when its content can show — it has shown
 * content, or nothing under it is unready (the tree neither pending nor
 * born held, no reader waited on). */ function ready(e) {
    return e.D || !(e.K.m & STATUS_PENDING) && !(e.K.l & CONFIG_HELD && (e.j === null || e.j.m & STATUS_UNINITIALIZED)) && prune(e) === 0;
}

/** `boundarySeamHook` (scheduler.ts), end of the seam: readers that settled, died,
 * or landed and committed are dropped; a fallback with none left reveals
 * next round. An arm resolves: nothing collected, nothing happened. */ function boundarySeam() {
    for (const e of collecting) {
        const n = e.j.R;
        if (n & REACTIVE_DISPOSED) {
            collecting.delete(e);
            continue;
        }
        // A parked frame's boundary (zombie) keeps its state for its revival.
                if (n & REACTIVE_ZOMBIE) continue;
        // A fresh boundary's content that derives from a write a hold keeps
        // reveals with that hold's commit (F-7, D-12): the boundary waits on
        // the held node, kept like a held reader until it commits (`prune`).
        // The node a settled reader reads: an answer staged there — a staged
        // error marks no frame reader (D-41) — left the reader settled on its
        // stale committed value (#4591). The node a first load it waits on
        // reads: the seam keeps a loading source out of every hold (#2937,
        // D-6), so its answer landed mainline, torn from the frame a write's
        // hold keeps on screen (F-1, #4593); under an open action it still
        // reveals at its landing (#2937). Not a held reader: it reveals with its
        // own commit.
                if (!e.D) for (const [n] of e.F) for (const r of n.m & STATUS_PENDING ? n.T?.se || [] : [ n ]) if (!(r.l & CONFIG_HELD)) for (let o = r.C; o; o = o.G) {
            const t = o.S;
            if (t.l & CONFIG_HELD && (r === n || !txOf(t).tt)) record(e, t);
        }
        prune(e);
        const r = e.j;
        // A fallback staged into the very hold its readers landed in folds back:
        // the content is ready when that hold commits (D-17, #4379). A loading
        // fallback only: an error boundary's is that hold's staged error (D-41),
        // and folding back re-derived the output into it every round (#4480).
                if (e.i & STATUS_PENDING && !e.D && r.l & CONFIG_HELD && [ ...e.F ].every(([, e]) => !(e.m & STATUS_PENDING) && e.l & CONFIG_HELD && txOf(e) === txOf(r))) {
            e.F.clear();
            e.D = true;
        }
        e.h = false;
        if (e.F.size === 0) {
            collecting.delete(e);
            if (e.L) redraw(e);
            if (revealHooks && e.v !== null) revealHooks.V(e.v);
        }
    }
}

/** `on` changed after the mount: fresh again if anything under it is
 * pending now; collecting for the rest of the flush either way. */ function arm(e) {
    if (!e.D) {
        // Already re-armed and on its fallback, as the frame's work — and the
        // `on` pass arms again as lane work: its verdict was provisional (a
        // memo of `isPending(m2)` is a watcher until the flush parks, then the
        // holder's verdict lane's — verdict.ts), and the swap it asked for is
        // display-ahead after all (#3528). Re-home the swap: the output re-runs
        // as the lane's, shown now beside the held frame.
        if (e.L && e.P === null && e.B === null && passLane !== null) {
            e.B = passLane;
            // The swap the frame's pass staged (held with the frame) is void: the
            // lane's pass stages it anew (`flip` lists the output in the lane,
            // which voids its staging), as a watcher's provisional value is
            // voided at the seam (verdict.ts `verdictSeam`).
                        flip(e);
            redraw(e);
        }
        return;
    }
    e.h = true;
    // The swap is the change's work: the output joins it (D-12, #3540).
        e.j.l &= ~CONFIG_FRAME;
    // A verdict read in `on` (`isPending`, or a memo of one — #3528's
    // `createMemo(() => isPending(m2))`: the mark carries, core.ts `read`)
    // whose frame has no transaction yet: the swap is still display-ahead — the frame gets one now (it lands
    // at this very seam if nothing holds it), and the swap is its verdict
    // lane's work, shown ahead of the frame's own runs (#3528).
        let n = passLane;
    if (n === null && globalQueue.Y && getOwner().l & CONFIG_VERDICT) {
        joinFuture(null);
        n = GlobalQueue.J(flushTransaction);
    }
    e.B = n;
    collecting.add(e);
    const r = e.F.size;
    heldUnder(e.H, flushTransaction, n => record(e, n));
    for (const [n] of e.F) {
        if (e.F.size !== r || n.m & STATUS_PENDING && !(n.R & REACTIVE_DISPOSED)) {
            // What it collects was forwarded to the boundaries above too; it is this
            // boundary's alone now, the nearest catcher: an outer one re-armed by the
            // same tick does not wait for it (D-12).
            for (let n = e._parent; n !== null; n = n._parent) 
            // An outer error boundary keys its readers too, but waits on their
            // sources: a re-armed Loading taking the pending does not settle it.
            if (n.i & STATUS_PENDING) for (const [r] of e.F) n.F.delete(r);
            flip(e);
            redraw(e);
            return;
        }
    }
}

/** The error boundary's `reset`: re-run what threw (a comparator throw
 * names a signal — nothing to re-run). */ function reset(e) {
    // A snapshot: a source failing again is caught — and re-added — mid-walk.
    for (const n of new Set(e.F.values())) if (n.X !== undefined && !(n.R & REACTIVE_DISPOSED)) recompute(n);
    schedule();
}

/**
 * Dev CHECK: the `on` dependencies evaluated to a function — `<Loading
 * on={route}>` or `on={() => route()}` passes the accessor itself, a stable
 * value that is never called, so nothing is read and the boundary never
 * re-arms. Checked on the first run only: the shape is static.
 */ function reportOnAccessor(e) {
    reportDiagnostic(emitDiagnostic({
        code: "LOADING_ON_ACCESSOR",
        kind: "async",
        severity: "warn",
        message: "[LOADING_ON_ACCESSOR] A Loading boundary's `on` received a function. `on` takes the " + "dependency's value — `on={route()}`, or `on={[query(), page()]}` for several — and an " + "accessor passed instead is never called, so nothing is read and the boundary never re-arms."
    }, e.H));
}

function createBoundary(e, n, r, o) {
    const t = createOwner();
    const l = {
        i: e,
        _parent: boundaryOf(t) ?? null,
        H: t,
        K: null,
        j: null,
        F: new Map,
        // Loading only: an error boundary has no first-content wait (`prune`).
        D: e === STATUS_ERROR,
        h: false,
        B: null,
        P: null,
        L: false,
        M: null,
        v: null,
        $: false,
        ee: false,
        U: e === STATUS_ERROR ? signal(undefined, ERROR_SIGNAL) : null,
        ne: r,
        re: null
    };
    const a = e === STATUS_PENDING ? t.A[REVEAL] : null;
    if (getOwner()) cleanup(() => {
        collecting.delete(l);
        if (revealHooks && a) revealHooks.oe(a, l);
    });
    // The `on` dependencies live OUTSIDE the boundary, as the condition of a
    // `<Show>` wrapping it would: created before the owner's context names it.
    // A source of it going pending re-runs it (CONFIG_REDERIVE) rather than
    // marking it: the pass catches the NotReady — it is a notification, it
    // does not suspend the parent — and re-arms.
        if (o) {
        let e = false;
        runWithOwner(t, () => computed(() => {
            try {
                const n = o();
                if (false && !e && typeof n === "function") ;
            } catch (e) {
                // (D-42, dev: this catch is the re-arm's mechanism, not a user's.)
                if (false) ;
                if (!(e instanceof NotReadyError)) {
                    if (!globalQueue.notify(getOwner(), STATUS_ERROR, STATUS_ERROR, e)) {
                        if (false) ; else haltReactivity(e);
                        throw e;
                    }
                    return;
                }
            }
            if (e) arm(l); else e = true;
        })).l |= CONFIG_REDERIVE;
    }
    const i = {
        ...t.A,
        [BOUNDARY]: l
    };
    if (revealHooks !== null) i[REVEAL] = null;
    t.A = i;
    const s = runWithOwner(t, () => {
        const e = false ? computed(n, {
            name: "children"
        }) : computed(n);
        return false ? computed(() => flatten(read(e)), {
            name: "boundary"
        }) : computed(() => flatten(read(e)));
    });
    l.K = s;
    // A slot of the reveal order in its context (reveal.ts): gated until the
    // controller releases it — before the output's first pass.
        if (revealHooks && a) revealHooks.te(a, l);
    const u = computed(() => {
        // Structure, not a derivation of its content (CONFIG_FRAME, #4283): it
        // reads a hold it is not part of as the screen, as the render effect
        // flattening the same children without a boundary would. Not while a
        // re-arm's swap is the change's work (`arm`), until that swap commits
        // (D-12, #3540).
        const n = getOwner();
        // Reading the retained tree need not read the leaf that recovered.
        // Keep the ahead swap's lane for recovery as well as for the fallback;
        // otherwise that successful answer is parked in the input's hold.
                if (laneStageHook && l.i === STATUS_ERROR) {
            if (l.L && n.l & CONFIG_OVERRIDE) l.P ??= txOf(n);
            if (l.P !== null) {
                setPassLane(l.P);
                n.R |= REACTIVE_LANE_READ;
                l.P = null;
            }
        }
        if (!l.h && !(n.l & CONFIG_HELD)) n.l |= CONFIG_FRAME;
        // Reveal order (reveal.ts): a gated slot shows its fallback — or, in a
        // collapsed tail, nothing — whatever its content's state; the
        // controller is told the pass ran (its readiness may have changed) and
        // releases it in turn.
                if (l.$) {
            // Linked without being read: the tree settling (or changing)
            // re-derives this pass, which is how the controller hears of it.
            // It may release this very slot (the frontier whose content just
            // settled): the pass goes on to show it — the redraw it asks for is
            // refused mid-pass and not needed.
            link(s, n);
            if (revealHooks && l.v !== null) revealHooks.V(l.v);
            if (l.$) {
                if (l.ee) {
                    l.L = true;
                    return undefined;
                }
                return fallback(l);
            }
        }
        // A tree carrying a status is not read: an errored memo's re-read retries
        // it (F-9), and a pending one read as lane work would serve its committed
        // value (D-25) where the swap must still be the fallback. Linked, so its
        // recovery re-derives this pass. A swap to the fallback staged in a hold
        // leaves the content on screen: a lane pass derives it with its guesses
        // (D-17, D-33, F-1, #4999).
                const r = laneStageHook && !l.D && n.l & CONFIG_HELD && n.le !== l.re?.le;
        const o = s.m & (STATUS_PENDING | STATUS_ERROR);
        if (o !== 0) {
            link(s, n);
            if (o & e) {
                if (isCollecting(l)) {
                    caught(l, s, undefined);
                    return fallback(l);
                }
                // Forwarded: a loading boundary showing content keeps it and
                // holds the frame like any reader (recorded for its `on`).
                                record(l, s);
            }
            // An error boundary on its fallback is the display of everything
            // under it (F-3, F-12): a retry pending there (an error returned
            // above, so this status is a pending) keeps the fallback and
            // holds nothing outside it — the recovery re-derives this pass. Not
            // when the loading boundary above has not shown content (fresh, or
            // behind an error fallback, #4627): this fallback was never on
            // screen, and that boundary catches the pending (F-4, #4926).
                        if (e === STATUS_ERROR && l.L && !freshAbove(l)) return fallback(l);
            // A pending passes an error boundary by (D-38); an error, a loading one; a
            // loading boundary showing content forwards its pending.
                        throw s.T.U;
        }
        if (isCollecting(l)) {
            // A boundary mounted over a held value shows its fallback now and the
            // content at the commit (F-7, #3540): entering the transaction would make
            // the output born held. One with a committed value reads a held tree and
            // enters: the landing reveals the content.
            if (l.F.size !== 0 && n.m & STATUS_UNINITIALIZED && !(n.l & CONFIG_HELD)) return fallback(l);
            // Readers under it still unready: the fallback, the tree untouched.
            // The seam re-derives this pass when they settle.
                        if (prune(l, true) !== 0 && !r) return fallback(l);
        }
        let t;
        try {
            t = read(s);
            // Returned content that is a store carries its family's error: the
            // probe reads it here, where the boundary catches it (F-12, D-41).
            // Left to `handleAsync`'s probe, it is the output's own error.
                        if (l.U) untrack(() => isThenable(t));
        } catch (n) {
            if (n instanceof NotReadyError ? e === STATUS_PENDING : e === STATUS_ERROR) {
                if (isCollecting(l)) {
                    caught(l, s, n);
                    return fallback(l);
                }
                // Forwarded: a loading boundary showing content keeps it and
                // holds the frame like any reader (recorded for its `on`).
                                record(l, s);
            }
            // A pending passes an error boundary by (D-38).
                        throw n;
        }
        // Readers under it still unready: the fallback. So is a tree the read
        // itself ran into a pending: as lane work it is served the committed tree
        // (D-25), and the pass under it could not re-derive this one mid-pass
        // (#4856).
                if (isCollecting(l)) {
            if (s.m & e & STATUS_PENDING) caught(l, s);
            if (l.F.size !== 0) return r && passLane ? t : fallback(l);
        }
        l.D = true;
        if (l.L && swapHook) swapHook();
        l.L = false;
        // The fallback dies when the content that replaces it is on screen —
        // with this flush's commits when the pass is the frame's, at the
        // landing when the frame parks (the run is held with it, as the old
        // children of a held pass are) — not now: a held swap keeps the
        // fallback on screen, live, until it commits. A pass back to the
        // fallback before then finds it still built and keeps it. A fallback
        // the screen does not show (a swap staged with a held frame, replaced
        // before it commits) dies now: what it reads holds nothing (S-22 row
        // 3′, #4268).
                const a = l.re;
        const drop = () => {
            if (!l.L && l.re === a) {
                // A fallback that threw (a rethrow the boundary does not
                // handle) is what an enclosing boundary waits on: it died, and
                // the seam of another round drops it there (#4270).
                if (a.m & STATUS_ERROR) schedule();
                dispose(a);
                l.re = null;
            }
        };
        if (a !== null) l.j.le !== a.le ? drop() : globalQueue.enqueue(EFFECT_RENDER, drop);
        release(l);
        // Shown: the slot leaves its reveal order (a later re-arm gates nobody
        // else; the frontier moves on).
                if (revealHooks && l.v !== null) {
            const e = l.v;
            l.v = null;
            revealHooks.V(e);
        }
        return t;
    }, 
    // Boundary structure, not a user source: its value is fallback-or-content
    // and legitimately swaps mid-hydration (resume), so it must never be
    // frozen by snapshot capture.
    {
        ae: true
    });
    u.l |= CONFIG_REDERIVE;
    // The tree is structure too (the output's pass marks the output).
        s.l |= CONFIG_FRAME;
    l.j = u;
    return accessor(u);
}

function fallback(e) {
    // (The output's pass.)
    const n = getOwner();
    // The swap a display-ahead re-arm asked for: this pass is the lane's — a
    // lane read by construction (REACTIVE_LANE_READ: `recompute`'s tail keeps
    // the pass in the lane).
        if (e.P !== null) {
        setPassLane(e.P);
        n.R |= REACTIVE_LANE_READ;
        e.P = null;
    }
    // The content goes behind the fallback: off screen (F-3), and so are its
    // head tags (`swapHook`, #4873).
        if (!e.L && swapHook) swapHook();
    e.L = true;
    // Built in its own scope, like `Show`'s function child, once per show
    // (D-41: the fallback is the display until a new answer replaces the
    // error): a re-derive of the output while it shows — a second reader
    // caught, the same error propagated again through a sibling's re-run, a
    // retry pending after `reset()`, a later error — keeps the built
    // fallback, its DOM and focus with it (#3991). A later error reaches it
    // through the error accessor; only a fallback reading the accessor at its
    // top level is rebuilt by it. Created beside the output — in its owner,
    // outside the boundary, as the output is: its errors and runs are the
    // enclosing boundary's — with the output's id, so what it builds gets the
    // ids a build under the output would; disposed once content is on screen.
    // Lazy (`1`: a byte under `true`), so its first pass runs in the output's
    // read below: the pass that creates it is the output's, and born held asks
    // the output — what it read — not the owner the fallback lives in (D-25;
    // #4428: an enclosing pass that joined a hold, while the output read nothing
    // held). (Lazy also releases it when its one reader, the output, stops
    // reading it: where `drop` below disposes it, or with the commit of a held
    // swap.)
        if (e.re === null) e.re = runWithOwner(n._parent, () => computed(() => e.ne(e), {
        id: n.id,
        lazy: 1
    }));
    // The swap is structure (#3540): what the fallback reads ahead — a
    // verdict (`latest()`), a lane — is the fallback's display-ahead, not the
    // swap's: the output's pass stays where it was (F-6 stays inside the
    // fallback, #4337).
        const r = passLane;
    const o = n.R;
    const t = read(e.re);
    setPassLane(r);
    n.R ^= (n.R ^ o) & (REACTIVE_PROBED | REACTIVE_LANE_READ);
    return t;
}

const ERROR_SIGNAL = {
    ownedWrite: true,
    ae: true
};

/**
 * Lower-level primitive that backs the `<Loading>` flow control. Catches
 * pending async reads inside `fn` and renders `fallback` until they settle.
 *
 * App code should use `<Loading fallback={...}>` instead — reach for this only
 * when authoring custom boundary components.
 *
 * @param fn the tracked subtree
 * @param fallback the fallback shown while async reads in `fn` are unresolved
 * @param options `on` — a dependency list: a tracked function whose reads
 *   re-arm the boundary. Its return value is irrelevant (never compared);
 *   what matters is what it reads. Without `on`, a boundary that has shown
 *   content keeps it through a refetch (the pending holds with the
 *   transaction). With `on`, a write to anything it reads makes the boundary
 *   fresh again: it stops waiting on its current content, and if something
 *   under it is pending it shows `fallback` until the new content is ready;
 *   if nothing is pending, the notification is a no-op. The fallback lands
 *   with the same frame as the change that caused it — now, when nothing
 *   else holds that frame; together with the rest of the new page during a
 *   held navigation, not before it. If the same data is also read outside
 *   the boundary, the frame waits on it and the fallback can never be seen
 *   (DEV warns `LOADING_ON_OUTSIDE_HOLD`); the fix is structural — move the
 *   outside read under the boundary so one hold owns the data. A frame held
 *   past the content's landing by something else (the write's action, other
 *   pending data) also shows no fallback; that is a race the fallback may
 *   lose, a legitimate outcome, and not reported. A display-ahead read in
 *   `on` (`latest()`) shows the fallback now, beside the held frame; that
 *   is a capability, not the recommended shape. Optimistic writes and a
 *   source going pending notify like any other. The children are not
 *   re-created — they stay alive behind the fallback.
 *
 * @example
 * ```tsx
 * // Custom boundary component built on top of the primitive.
 * function MyLoading(props: { fallback: JSX.Element; children: JSX.Element }) {
 *   return createLoadingBoundary(
 *     () => props.children,
 *     () => props.fallback
 *   ) as unknown as JSX.Element;
 * }
 * ```
 */ function createLoadingBoundary(e, n, r) {
    return createBoundary(STATUS_PENDING, e, () => n(), r?.on);
}

/**
 * Lower-level primitive that backs the `<Errored>` flow control. Catches
 * thrown errors inside `fn` and invokes `fallback(error, reset)` instead.
 * `error` is an accessor for the latest captured error; `reset()` recomputes
 * the failing sources so the boundary can attempt to recover.
 *
 * App code should use `<Errored fallback={...}>` instead — reach for this only
 * when authoring custom boundary components.
 *
 * @example
 * ```tsx
 * // Custom boundary that wraps the primitive and adds telemetry.
 * function TracedErrored(props: { fallback: (e: () => unknown) => JSX.Element; children: JSX.Element }) {
 *   return createErrorBoundary(
 *     () => props.children,
 *     (err, reset) => {
 *       reportError(err());
 *       return props.fallback(err);
 *     }
 *   ) as unknown as JSX.Element;
 * }
 * ```
 */ function createErrorBoundary(e, n) {
    return createBoundary(STATUS_ERROR, e, e => n(accessor(e.U), () => reset(e)));
}

// Installed at module evaluation — present exactly when something imports a
// boundary. An app without one pays the three null checks and nothing else.
installBoundaryHooks(catchStatus, (e, n) => !n && (e.m & STATUS_UNINITIALIZED ? catchStatus(e, STATUS_PENDING) : armedOver(e)), hiddenBy, boundarySeam, heldRun);

export { REVEAL, createErrorBoundary, createLoadingBoundary, prune, ready, redraw, revealHooks, setRevealHooks };