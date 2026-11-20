import { NewSet, REACTIVE_RECOMPUTING_DEPS, CONFIG_AUTO_DISPOSE, STATUS_PENDING, REACTIVE_DISPOSED, REACTIVE_ZOMBIE, CONFIG_SLOT_NODE } from "./constants.js";

import { deleteFromHeap } from "./heap.js";

import { disposeChildren, runCleanup } from "./owner.js";

import { bumpNotifyEpoch, dirtyQueue } from "./scheduler.js";

/** The store's shared slot-node release handler (store/store.ts installs it
 * once) — a live binding read directly by the sweep site: no wrapper frame,
 * no null check (a CONFIG_SLOT_NODE node existing implies the store module
 * loaded and registered it). */ let slotUnobservedHook;

function setSlotUnobserved(e) {
    slotUnobservedHook = e;
}

// https://github.com/stackblitz/alien-signals/blob/v2.0.3/src/system.ts#L100
function unlinkSubs(e) {
    const n = e.S;
    const l = e.G;
    const o = e.t;
    const s = e.en;
    if (o !== null) o.en = s; else n.Ze = s;
    if (s !== null) s.t = o; else {
        n.o = o;
        if (o === null) {
            // Store slot nodes dispatch to the ONE shared hook — no per-node
            // unobserved closure, no NodeExtension to hold it.
            // A user hook that throws is collected like a cleanup (owner.ts): the
            // unlink and the teardown around it carry on.
            if (n.l & CONFIG_SLOT_NODE) slotUnobservedHook(n); else if (n.T?.Ye) runCleanup(n.T.Ye);
            // No more subscribers; only tear down if CONFIG_AUTO_DISPOSE is set.
            // A pending node is exempt: its in-flight async work is an observer —
            // tearing down would orphan the work and re-execute it on the next
            // read. The settle path runs this same last-one-out check when that
            // observer releases (the untracked-read dormancy sweep guards on
            // pending identically).
            // During a pass's teardown of its unfinished frame, the release waits
            // for the pass to end (`deferredReleases`, #4142).
            // (No cast alias for `dep`: the minifier keeps one.)
                        n.X && n.l & CONFIG_AUTO_DISPOSE && !(n.R & REACTIVE_ZOMBIE) && !(n.m & STATUS_PENDING) && (deferredReleases ? deferredReleases.push(n) : unobserved(n));
        }
    }
    return l;
}

function trimStaleDeps(e) {
    const n = e._;
    let l = n !== null ? n.G : e.C;
    if (l !== null) {
        do {
            l = unlinkSubs(l);
        } while (l !== null);
        if (n !== null) n.G = null; else e.C = null;
    }
}

// Shared by unobserved() and the disposeChildren child loop. The truthy guard
// (not `!== null`) matters: plain Owners in a child chain have no _deps field,
// and skipping early also avoids adding one (hidden-class churn) via the
// null-out below. With `_depsTail` cleared, `trimStaleDeps` unlinks the whole
// list and nulls `_deps`.
function clearDeps(e) {
    if (e.C) {
        e._ = null;
        trimStaleDeps(e);
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
 */ let deferredReleases = null;

const deferReleases = e => deferredReleases = e;

function unobserved(e) {
    deleteFromHeap(e, dirtyQueue);
    clearDeps(e);
    disposeChildren(e, true);
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
 */ const dormantNodes = new NewSet;

function sweepDormant() {
    if (dormantNodes.size === 0) return;
    for (const e of dormantNodes) {
        // Re-validate at sweep time: the node may have gained a subscriber (its
        // lifecycle is the unlinkSubs cascade now), gone pending (in-flight async
        // is an observer; the settle path re-runs last-one-out), lost its
        // AUTO_DISPOSE bit (owner teardown strips it, #3024), or already been
        // torn down.
        if (!e.o && e.l & CONFIG_AUTO_DISPOSE && !(e.m & STATUS_PENDING) && !(e.R & (REACTIVE_DISPOSED | REACTIVE_ZOMBIE))) {
            unobserved(e);
        }
    }
    dormantNodes.clear();
}

// https://github.com/stackblitz/alien-signals/blob/v2.0.3/src/system.ts#L52
/** Truthy when it reuses the link `sub`'s last pass made: a re-read. */ function link(e, n) {
    const l = n._;
    if (l !== null && l.S === e) return;
    let o = null;
    const s = n.R & REACTIVE_RECOMPUTING_DEPS;
    if (s) {
        o = l !== null ? l.G : n.C;
        if (o !== null && o.S === e) {
            o.Ne = n.de;
            return n._ = o;
        }
    }
    // A link stamped with the current pass generation was created or reused
    // in-order during this recompute, i.e. it already sits in the validated
    // [deps.._depsTail] prefix — the O(1) equivalent of scanning the dep list
    // (the old alien-signals `isValidLink` walk, O(n²) when a computation
    // re-reads earlier deps non-consecutively, e.g. store leaf reads).
        const r = e.Ze;
    if (r !== null && r.u === n && (!s || r.Ne === n.de)) return;
    const t = n._ = e.Ze = {
        S: e,
        u: n,
        G: o,
        en: r,
        t: null,
        Ne: n.de
    };
    if (l !== null) l.G = t; else n.C = t;
    if (r !== null) r.t = t; else e.o = t;
    // New subscriber edge: staged-rewrite skips (§12d) must not miss it.
        bumpNotifyEpoch();
}

export { clearDeps, deferReleases, deferredReleases, dormantNodes, link, setSlotUnobserved, slotUnobservedHook, sweepDormant, trimStaleDeps, unlinkSubs, unobserved };