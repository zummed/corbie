import { $REFRESH, STATUS_PENDING, CONFIG_SLOT_NODE } from "./core/constants.js";

import "./core/core.js";

import { schedule, resolveTx, flushTransaction, installMarkHooks, GlobalQueue } from "./core/scheduler.js";

import { mark, repoll, unmark } from "./affects-marks.js";

import { storeMarks, installStoreAffects, releaseMarkScope } from "./store/affects.js";

import { $TARGET } from "./store/types.js";

// `affects()` — the declaration verb of the pending model (A24 (4), ruled
// 2026-07-13; the #2893 audit corollaries; rebuilt on L2 2026-10-02 as ruled:
// transaction-inert).
// A mark is a declared in-flight change: the marked node — and everything
// derived from it — reads pending (`isPending` → true) from the declaration
// until the surrounding transaction lands (an ambient mark, declared outside
// one, releases at the next seam; either, on a node whose own flight is
// still up then, at the seam after that flight lands). The marked values stay readable: no reader
// suspends on a mark, a mark holds nothing and entangles nothing — it is not
// a node of its transaction (`blocked` never sees it), and a reader of marked
// data joins nothing through it. A mark is never a re-ask, so a declared
// reload's own `refresh()` cannot silence it; a real error outranks it.
// Representation: the mark core (affects-marks.ts) — a count on the marked
// node, coverage pull-derived at probe time, readers repolled at registration
// and release. Here: each mark's scope (the transaction's `_marks`, or the
// ambient list) and the store half.
// Installed on `GlobalQueue` when imported: a program that never declares a
// mark carries none of this.
/** Marks declared outside a transaction: released at the next seam. */
const ambient = [];

/** Releases one scope's marks (a landed transaction's, the ambient list at
 * the seam): a node whose last mark drops re-derives its verdict readers. */ function release(e) {
    for (let t = 0; t < e.length; t++) {
        const r = e[t];
        // A store mark's carrier: its scope (and the marks the nodes born in
        // its window inherited) goes with the last mark.
                if (unmark(r)) releaseMarkScope(r);
    }
    e.length = 0;
}

/** One registration: a count on the node, listed with the scope — the
 * flush's transaction, or the ambient list (released at the seam). */ function register(e) {
    mark(e);
    const t = flushTransaction;
    if (t === null) ambient.push(e); else {
        const r = resolveTx(t);
        (r.I ??= []).push(e);
    }
    repoll(e);
}

function affects(e, t) {
    // A store (the store half, store/affects.ts): the slot's leaf, or the
    // record's carrier and every live node under it.
    const r = e?.[$TARGET];
    if (r !== undefined) {
        installStoreAffects();
        const e = storeMarks(r, t);
        for (let t = 0; t < e.length; t++) register(e[t]);
        schedule();
        return;
    }
    const s = e?.[$REFRESH];
    if (!s) {
        return;
    }
    register(s);
    schedule();
}

installMarkHooks(e => {
    // A landing's mark on a node whose own flight is up (a declared reload's
    // quiet re-ask the action did not wait for) stays to the seam after
    // that flight lands, as an ambient one does: the reload's window is the
    // flight's (#4349). The rest release with the transaction.
    const t = [];
    for (let r = 0; r < e.length; r++) (inFlight(e[r]) ? ambient : t).push(e[r]);
    e.length = 0;
    release(t);
}, e => {
    if (ambient.length === 0) return;
    // The flush parked: the declaration's window is the frame's — the marks
    // are its transaction's ("nothing async below ⇒ no window").
        if (e !== null) {
        const t = e.I ??= [];
        for (let e = 0; e < ambient.length; e++) t.push(ambient[e]);
        ambient.length = 0;
        return;
    }
    // Nothing parked, but a marked node's own flight is up (a declared
    // reload's quiet re-ask — the node's, or a store leaf's family derive):
    // the window is the flight's; the mark stays to the seam after the
    // landing. The rest release: verdict-only, nothing to show.
        let t = 0;
    const r = [];
    for (let e = 0; e < ambient.length; e++) {
        const s = ambient[e];
        if (inFlight(s)) ambient[t++] = s; else r.push(s);
    }
    ambient.length = t;
    if (r.length !== 0) release(r);
});

function inFlight(e) {
    return (e.m & STATUS_PENDING) !== 0 || (e.l & CONFIG_SLOT_NODE) !== 0 && GlobalQueue.k?.(e) === true;
}

export { affects };