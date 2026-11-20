import { NOT_PENDING, EFFECT_USER, CONFIG_AUTO_DISPOSE, CONFIG_CHILDREN_FORBIDDEN, EFFECT_TRACKED, EFFECT_RENDER, REACTIVE_DISPOSED, STATUS_ERROR, CONFIG_OVERRIDE, CONFIG_HELD, STATUS_PENDING } from "./constants.js";

import { createEffectNode, recompute, computed, staleValues, ext, setEffectStatusNotify } from "./core.js";

import { unwrapStatusError, StatusError } from "./error.js";

import { trimStaleDeps } from "./graph.js";

import { enqueueSub } from "./heap.js";

import { applyStaged, globalQueue, schedule, heldRunHook, attachHook, txOf, haltReactivity, notifyEpoch, GlobalQueue, queueEffectRun } from "./scheduler.js";

/**
 * Effects are the leaf nodes of our reactive graph. When their sources change, they are
 * automatically added to the queue of effects to re-execute, which will cause them to fetch their
 * sources and recompute
 */ function effect(e, t, E, f) {
    const u = !!f?.user;
    const o = createEffectNode(e, t, E, u ? EFFECT_USER : EFFECT_RENDER, f);
    recompute(o, true);
    // A first pass that read a staged value was staged itself (recompute: born
    // staged); the flush's commit replays this effect. Its first run is not this
    // creation's (F-7). A lane's node's run is the lane's: queued, not run
    // now — into its own pass's lane (its transaction: the creator's lane, or
    // the one a lane read moved the pass into, F-6/F-7) — and released at the
    // reveal (lanes.ts). Not a lane's node: the creator's (`enqueue`'s default
    // for no transaction; the frame for a transaction's).
    // `defer` skips the first run, whoever owes it. A node born staged
    // (#4543): its staged first value applies now, so the commit replays
    // nothing and the next pass gets it as `prev`, as on a plain mount (#4556;
    // D-12). A lane's node (#4557): it queues none.
        f?.defer ? applyStaged(o) : o.ue === NOT_PENDING && (o.T?.Re?.P || o.i === EFFECT_USER || f?.schedule ? globalQueue.enqueue(o.i, runEffect.bind(null, o), o.T?.Re) : // Type 0: the synchronous first render, not a queued run (D-19).
    runEffect(o, 0));
}

function notifyEffectStatus() {
    const e = this.m;
    const t = this.T?.U;
    if (e & STATUS_ERROR) {
        globalQueue.notify(this, STATUS_PENDING, 0);
        if (this.i === EFFECT_USER) {
            // The error handler is the error arm of the effect phase (#2840 ruling):
            // queue it like the effect function. It runs in the same imperative,
            // writable scope and throws escalate the same way. No payload is queued — the node already carries `_statusFlags`/`_error`,
            // and the runner dispatches on them, so a recovery before the effect
            // phase takes the success arm instead. (Re-read: the boundary
            // notify above can run a redraw.)
            if (this.m & STATUS_ERROR) queueEffectRun(this);
            return;
        }
        // Dev locates the halt at the node that threw: the StatusError's
        // source, upstream of this effect when a computed failed. The rethrow is
        // the user's error, not the wrapper: the wrapper's `source` is a node, and
        // a reporter that serializes the throw walks the reactive graph.
                escalate(this, unwrapStatusError(t));
    } else if (
    // A user effect's run owed for a pass a new question has since made
    // pending is for a world that will not commit (F-1, F-2): void — the
    // effect re-runs when that question settles (`settlePendingSource`). A
    // quiet re-ask keeps it: the pass still answers the question asked (F-9).
    this.i === EFFECT_USER) {
        if (e & STATUS_PENDING && this.Ae && !t?.source?.T?.Ve) ext(this).fe = !(this.Ae = false);
    } else if (this.i === EFFECT_RENDER) {
        globalQueue.notify(this, STATUS_PENDING | STATUS_ERROR, e, t);
    }
}

/** An effect's error no boundary takes halts the system (boundary-or-halt):
 * halted with `cause` and rethrown; dev locates it at `subject` (default:
 * the effect). */ function escalate(e, t, E) {
    if (!globalQueue.notify(e, STATUS_ERROR, STATUS_ERROR)) {
        haltReactivity(t);
        throw t;
    }
}

function runEffect(e, t) {
    if (!e.Ae || e.R & REACTIVE_DISPOSED) return;
    // A queued run behind a fallback (boundaries.ts) waits for the reveal: a
    // user effect's, which would read a DOM that is not attached, and a render
    // effect's update too (maintainer, 2026-10-02: "we held the queue but let
    // the sync renders through… we definitely shouldn't be showing portals
    // early") — a Portal's or head-tag's render effect writes outside the
    // hidden subtree. The synchronous first render on creation (`type` 0) is
    // not a queued run and goes through, whichever boundary above is on its
    // fallback then — an outer one about to reveal in this flush included
    // (#4343). `_modified` stays set; the boundary re-queues the run
    // (`release`).
        if (t && heldRunHook?.(e)) return;
    // An attach whose subtree holds a hole born into an open hold waits for
    // that hold (#4574, verdict.ts `attachWaits`).
        if (t && attachHook?.(e)) return;
    // Error arm (#2840), user effects only: a compute-phase error that is still
    // the node's settled state at effect time runs the bundle's error handler in
    // this same imperative, writable scope. Unwrap the StatusError used for
    // source tracking — user code gets the error it threw, as boundaries do. No
    // handler: log and keep the system alive (the run was skipped). A handler
    // (or logging) consumes the error; a handler throw falls to the shared
    // catch below and escalates boundary-or-halt like any effect-phase throw.
    // Render effects bypass: their errors route to boundaries synchronously in
    // notifyEffectStatus, and a runner queued by an earlier valueChanged in the
    // same flush must not be hijacked by a later-arriving error status.
        if (e.m & STATUS_ERROR && e.i === EFFECT_USER) {
        const t = unwrapStatusError(e.T?.U);
        e.ge = e.le;
        e.Ae = false;
        try {
            e.Je ? e.Je(t, () => {
                const t = e.ze;
                e.ze = undefined;
                t?.();
            }) : console.error(t);
        } catch (t) {
            escalate(e, t);
        }
        return;
    }
    // Captured before the callback: its own throw errors the node below, but
    // the compute pass that produced `_value` was clean, so its tail still goes.
        const E = e.T?.U == null;
    const f = e.ze;
    e.ze = undefined;
    try {
        f?.();
        const t = e.qe(e.le, e.ge);
        if (false && t !== undefined && typeof t !== "function") ;
        // The final cleanup is invoked by disposeChildren at true disposal.
                e.ze = t;
    } catch (t) {
        ext(e).U = new StatusError(e, t);
        e.m |= STATUS_ERROR;
        escalate(e, t);
    } finally {
        e.ge = e.le;
        e.Ae = false;
        // The run applied: this is the frame now, so the dependency tail the
        // compute pass left linked goes (D-10, #3438 — `recompute` defers an
        // effect's trim while a run is owed; the twin of `commitPendingNode`'s
        // trim for a staged pass). An errored compute kept its full list with
        // `_depsTail` marking where it stopped; leave it, as the commit does.
                if (E) trimStaleDeps(e);
    }
}

GlobalQueue.q = runEffect;

/**
 * Internal tracked effect - bypasses heap, goes directly to effect queue.
 * Runs as a leaf owner: child primitives and onCleanup are forbidden (false throws).
 * Uses stale reads.
 */ function trackedEffect(e, t) {
    // A run owed at a hold's landing (#4607): `land`, made on first need, is
    // queued with the hold's stashed runs; `owed` dedups it, and a later run
    // that reads nothing held clears it.
    let E = false, f;
    const run = () => {
        // `_modified` is NOT redundant with the heap: the heap dedups within a
        // pass, but several passes in one flush each enqueue `_run` into the same
        // user queue, and this gate is what collapses them into one run.
        if (!u.Ae || u.R & REACTIVE_DISPOSED) return;
        if (heldRunHook?.(u)) return;
        const e = notifyEpoch;
        E = false;
        try {
            u.Ae = false;
            recompute(u);
        } finally {}
        // A dep this run was served the committed value of (D-29) while a
        // different value is staged commits without a notify to it: its
        // subscriber walk predates the link. Re-run where that value commits
        // (F-1). Writes that net to the committed value stage no change.
                for (let t = u.C; t !== null; t = t.G) {
            const o = t.S;
            if (o.ue !== NOT_PENDING && !(o.l & CONFIG_OVERRIDE) && (o.ue !== o.le || !Object.is(o.ue, o.le))) {
                // Held: it commits at its hold's landing, which runs the hold's
                // stashed runs — but a run during the hold, for another dep, had
                // consumed the one queued for it (#4607). Re-run there, as a
                // stale reader would be (D-9); effect-phase code holds nothing (D-29).
                if (o.l & CONFIG_HELD) {
                    if (!E) {
                        E = true;
                        txOf(o).rn[EFFECT_USER - 1].push(f ||= () => {
                            if (E) {
                                u.Ae = true;
                                run();
                            }
                        });
                    }
                }
                // Staged this flush: the next round commits it. A memo is staged
                // by this run's own pull (`read` links, then pulls), whose walk
                // the recomputing run refuses (#4606); a signal written since the
                // epoch is the run's own write, never observed (#3006).
                 else if (o.we <= e || o.X) {
                    enqueueSub(u);
                    schedule();
                    // The re-run re-checks every dep, held ones included.
                                        break;
                }
            }
        }
    };
    const u = computed(() => {
        const t = u.ze;
        u.ze = undefined;
        t?.();
        const E = staleValues(e);
        u.ze = E;
    }, {
        ...t,
        lazy: true
    });
    u.ze = undefined;
    u.l = u.l & ~CONFIG_AUTO_DISPOSE | CONFIG_CHILDREN_FORBIDDEN;
    u.Ae = true;
    u.i = EFFECT_TRACKED;
    // Status dispatch rides the SHARED notifier (statusNotifierOf keys off
    // _type): its error arm is behavior-identical to the closure that used to
    // live here, without the per-node NodeExtension allocation.
        u.Z = run;
    // The first run rides the heap like every wake (GlobalQueue._update), so a
    // tracked effect created inside a render-effect callback runs after that
    // pass's staged writes commit, not before.
        enqueueSub(u);
    schedule();
}

// Install the shared effect status notifier (statusNotifierOf serves it to
// every effect node) — module-scope: any bundle that creates effects
// evaluates this module.
setEffectStatusNotify(notifyEffectStatus);

export { effect, trackedEffect };