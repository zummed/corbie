import "./core.js";

import { REACTIVE_IN_HEAP, REACTIVE_RECOMPUTING_DEPS, REACTIVE_CHECK, REACTIVE_DIRTY, REACTIVE_IN_HEAP_HEIGHT } from "./constants.js";

import { dirtyQueue } from "./scheduler.js";

/**
 * Schedule one subscriber to re-run on the next flush: inserted into the
 * dirty heap with the `_min` cursor pulled down. Tracked effects ride the
 * heap too — the heap visit is their (empty) compute phase, which hands the
 * callback to the user queue once the pass has committed (see
 * GlobalQueue._update, #3291).
 */ function enqueueSub(e) {
    if (dirtyQueue.Fe > e.he) dirtyQueue.Fe = e.he;
    insertIntoHeap(e, dirtyQueue);
}

function actualInsertIntoHeap(e, t) {
    const E = (e._parent?.Qe ? e._parent.Ke?.he : e._parent?.he) ?? -1;
    if (E >= e.he) e.he = E + 1;
    const n = e.he;
    const I = t.et[n];
    if (I === undefined) t.et[n] = e; else {
        const t = I.We;
        t.ve = e;
        e.We = t;
        I.We = e;
    }
    if (n > t.Et) t.Et = n;
}

function insertIntoHeap(e, t) {
    let E = e.R;
    // RECOMPUTING refusals are not always losses: a genuinely missed wake (a
    // write to a link this pass already validated) is latched link-side in
    // insertSubs as REACTIVE_MISSED_WAKE for recompute's tail (#3037).
        if (E & (REACTIVE_IN_HEAP | REACTIVE_RECOMPUTING_DEPS)) return;
    if (E & REACTIVE_CHECK) {
        e.R = E & -4 | REACTIVE_DIRTY | REACTIVE_IN_HEAP;
    } else {
        e.R = E | REACTIVE_IN_HEAP;
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
                if (t.p) markNode(e);
    }
    if (!(E & REACTIVE_IN_HEAP_HEIGHT)) actualInsertIntoHeap(e, t);
}

function insertIntoHeapHeight(e, t) {
    let E = e.R;
    if (E & (REACTIVE_IN_HEAP | REACTIVE_RECOMPUTING_DEPS | REACTIVE_IN_HEAP_HEIGHT)) return;
    e.R = E | REACTIVE_IN_HEAP_HEIGHT;
    actualInsertIntoHeap(e, t);
}

function deleteFromHeap(e, t) {
    const E = e.R;
    if (!(E & (REACTIVE_IN_HEAP | REACTIVE_IN_HEAP_HEIGHT))) return;
    e.R = E & -25;
    const n = e.he;
    if (e.We === e) t.et[n] = undefined; else {
        const E = e.ve;
        const I = t.et[n];
        const i = E ?? I;
        if (e === I) t.et[n] = E; else e.We.ve = E;
        i.We = e.We;
    }
    e.We = e;
    e.ve = undefined;
}

function markHeap(e) {
    if (e.p) return;
    e.p = true;
    for (let t = 0; t <= e.Et; t++) {
        for (let E = e.et[t]; E !== undefined; E = E.ve) {
            if (E.R & REACTIVE_IN_HEAP) markNode(E);
        }
    }
}

function markNode(e, t = REACTIVE_DIRTY) {
    const E = e.R;
    if ((E & (REACTIVE_CHECK | REACTIVE_DIRTY)) >= t) return;
    e.R = E & -4 | t;
    for (let t = e.o; t !== null; t = t.t) {
        markNode(t.u, REACTIVE_CHECK);
    }
}

function runHeap(e, t) {
    e.p = false;
    for (e.Fe = 0; e.Fe <= e.Et; e.Fe++) {
        let E = e.et[e.Fe];
        while (E !== undefined) {
            if (E.R & REACTIVE_IN_HEAP) t(E); else adjustHeight(E, e);
            E = e.et[e.Fe];
        }
    }
    e.Et = 0;
}

function adjustHeight(e, t) {
    deleteFromHeap(e, t);
    let E = e.he;
    for (let t = e.C; t; t = t.G) {
        const e = t.S;
        if (e.X && e.he >= E) E = e.he + 1;
    }
    if (e.he !== E) {
        e.he = E;
        liftOwned(e, E);
        // (Not an owner reading it: it sits below its child by construction,
        // REACTIVE_OWNER_READS, and a seat below the cursor is never visited.)
                for (let E = e.o; E !== null; E = E.t) if (E.u !== e._parent) insertIntoHeapHeight(E.u, t);
    }
}

/**
 * A raised node's owned computations (through roots) move above it (#4848).
 * The heap places a node above its direct owner only, at insert, so an owner
 * raised after its children's birth left a grandchild at or below it, to run
 * before the owner whose run disposes it. Paid when a height changes, never
 * per insert. A child waiting in the heap keeps its seat this pass (its next
 * insert lifts it above its owner); what it owns moves either way.
 */ function liftOwned(e, t) {
    for (let E = e.pe; E; E = E.Ge) if (E.Qe) liftOwned(E, t); else if (E.he <= t) {
        // A child waiting in the heap moves with it: left at its seat it runs
        // before the owner whose run disposes it.
        const e = E.R & (REACTIVE_IN_HEAP | REACTIVE_IN_HEAP_HEIGHT);
        deleteFromHeap(E, dirtyQueue);
        E.he = t + 1;
        if (e) (e & REACTIVE_IN_HEAP ? insertIntoHeap : insertIntoHeapHeight)(E, dirtyQueue);
        liftOwned(E, t + 1);
    }
}

export { deleteFromHeap, enqueueSub, insertIntoHeap, insertIntoHeapHeight, liftOwned, markHeap, markNode, runHeap };