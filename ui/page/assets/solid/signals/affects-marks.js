import { CONFIG_VERDICT, CONFIG_AUTHORITATIVE, STATUS_ERROR, CONFIG_SLOT_NODE, REACTIVE_RECOMPUTING_DEPS } from "./core/constants.js";

import { ext } from "./core/core.js";

import { enqueueSub } from "./core/heap.js";

import { GlobalQueue } from "./core/scheduler.js";

// The mark core: a declared not-final state on a node that reads pending
// (`isPending` → true) on the node and everything derived from it, without
// holding anything or entangling a reader. Two declarers share it:
// `affects()` (affects.ts — a mark scoped to a transaction, with a store
// half) and `createDeferred` (deferred.ts — a mark whose lifetime is the
// node's own flight). Split out so the second does not ship the first.
// Representation: a count on the marked node (`_x._marks`). Coverage of the
// derivations is pull-derived at probe time — `isPending`'s read walks the
// probed node's dependencies for a marked one (verdict.ts → `_marked`) — so
// nothing is stored downstream and a mid-window recompute strands nothing.
// The push half is the verdict readers' re-derivation when a node's marks
// start and end (`repoll`): the only notification a mark makes.
/** Live marks, every declarer — the gate on the probe's walk. */
let active = 0;

/** The verdict readers downstream of `node` — through derivations, stopping
 * at effects — re-derive: the mark's one notification (a probe's answer
 * changed with no value notifying anyone). Truth readers (`until`, the
 * `refresh` waiter) too, which a lagging flight parked (deferred.ts) — but
 * not through a derived store, whose leaves change only once its derive
 * re-runs: they re-derive at that write. */ function repoll(e, r = new Set, o = CONFIG_VERDICT | CONFIG_AUTHORITATIVE) {
    for (let t = e.o; t !== null; t = t.t) {
        const e = t.u;
        if (r.has(e)) continue;
        r.add(e);
        if (e.l & o) enqueueSub(e);
        if (!e.i) repoll(e, r, o);
        // A derived store's derive: its leaves' readers derive from it too
        // (store/projection.ts; a live target's property, has and keys nodes).
                const n = e.rr;
        if (n) for (const e of n.live) for (const o of [ e.n, e.h, {
            k: e.k,
            d: e.dk
        } ]) if (o) for (const e of Reflect.ownKeys(o)) o[e] && repoll(o[e], r, CONFIG_VERDICT);
    }
}

/** A node is covered by a live mark iff it carries one, or derives — through
 * its current dependencies — from one that does. A store slot derives from
 * its family's derive (a derived store's leaves are not the derive's
 * dependents, store/projection.ts), so coverage hops there. A real error
 * outranks an inherited mark (D-23): coverage does not flow through an
 * errored node (a direct mark on one still reads pending). Mid-recompute,
 * only the validated prefix of the dependency list is this pass's. */ function marked(e, r) {
    if (e.T !== null && e.T.I !== 0) return true;
    const o = e;
    if (o.m & STATUS_ERROR || r.has(e)) return false;
    r.add(e);
    if (o.l & CONFIG_SLOT_NODE) {
        const e = o.O?.rr?.node;
        return e != null && marked(e, r);
    }
    const t = o.R & REACTIVE_RECOMPUTING_DEPS ? o._ : undefined;
    if (t !== null) for (let e = o.C ?? null; e !== null; e = e.G) {
        if (marked(e.S, r)) return true;
        if (e === t) break;
    }
    return false;
}

/** One mark on `node`. The declarer repolls once its bookkeeping is done. */ function mark(e) {
    ext(e).I++;
    active++;
}

/** Drops one mark from `node`: true when it was the last — the node's
 * verdict readers have re-derived. */ function unmark(e) {
    active--;
    if (--e.T.I !== 0) return false;
    repoll(e);
    return true;
}

GlobalQueue.p = e => active !== 0 && marked(e, new Set);

GlobalQueue.N = mark;

export { mark, repoll, unmark };