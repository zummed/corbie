import { CONFIG_MANUAL_WRITE, CONFIG_OVERRIDE, CONFIG_HELD, NOT_PENDING, CONFIG_GUESS, STATUS_PENDING, STATUS_ERROR, CONFIG_STAGED_ERROR, EFFECT_RENDER, CONFIG_VERDICT, CONFIG_PROMOTED, REACTIVE_STAGED_READ, CONFIG_CHILDREN_FORBIDDEN, REACTIVE_DIRTY, REACTIVE_CHECK, $REFRESH, REACTIVE_RECOMPUTING_DEPS, STATUS_UNINITIALIZED } from "../core/constants.js";

import { writeHook, setSignal, isEqual, read, snapshotCaptureActive, captureWriteSnapshot, slotSignal, context, notePromotedWrite, untrack, verdict, tracking, pullComputed, stagedScreen, stagedRead } from "../core/core.js";

import { NotReadyError } from "../core/error.js";

import { signalHook } from "../core/identities.js";

import { link, setSlotUnobserved } from "../core/graph.js";

import { getObserver } from "../core/owner.js";

import { insertIntoHeap } from "../core/heap.js";

import { passLane, txOf, flushTransaction, laneStageHook, list, holdNode, joinFuture, insertSubs, schedule, globalQueue, queuePendingNode, observeFlightHook, resolveTx, joinPassTx, GlobalQueue, installStoreHooks, dirtyQueue } from "../core/scheduler.js";

import { lookupTarget, $OWNER, setLookup, isOwned, markDescendants } from "./target.js";

import { optHooks, affectsHooks } from "./hooks.js";

import { $TARGET, writeOverride, isWrappable, markRawIngest, rawValuesUsed, isRawValue, $TRACK, installShallowView, markRawOne, $RECORD, $PROXY, $PATCH } from "./types.js";

export { markRaw } from "./types.js";

/**
 * Store — plain stores on L2 (plan sec. 31, S1).
 *
 * A store is a tree of L2 nodes and nothing else carries reactive state:
 *
 * - **Leaf nodes** (one per tracked-or-written key, lazy; `slotSignal`, one
 *   literal): `_value` committed, `_pendingValue` this batch's staging.
 * - **Presence nodes** (`in` tracks presence, not value — R13) and the
 *   **deep witness** (`deep()` subscribes one node per record).
 * - The **container node** `k` (plan sec. 31.3, Q-A): the `$TRACK` node given a
 *   value. `_value` is the committed backing, `_pendingValue` the pending
 *   backing the draft mutates — so a write to a key nobody reads has a
 *   staging home the scheduler owns, and park/commit/revert of the backing
 *   is the hold model's, not a fold ledger of the store's. Structural
 *   readers (`ownKeys`, iteration, `$TRACK`, `length`, `deep`) subscribe to
 *   it; value readers never do — a key with no leaf reads through the
 *   container's visible backing without subscribing (R1: node existence is
 *   unobservable).
 *
 * The committed backing (`t.v`) is owned raw (CoW: a user object is shared
 * until the first write privatizes it and its ancestor chain). It mutates at
 * exactly one moment — the flush's commit (`_storeCommit`), after the
 * container node committed its staging. Reads: context-free readers see the
 * committed backing until the flush; a pass sees the staging (signal
 * parity, RUL-1); the draft reads its own staging.
 *
 * Perf contracts (plan sec. 31.4): lazy nodes; one literal per node; O(written) per
 * setter (`wk`); overlay drafts for large plain records; zero allocation
 * for adopted-but-unread objects.
 */
/** Key of the deep-witness slot node (never a user key). */ const $DEEP = Symbol(0);

// ---------------------------------------------------------------------------
// wrap / dedupe
/** Pre-shaped constructor for store targets: V8 tips a bare `{}` into
 * dictionary mode once ~19 named properties are assigned onto it (the #3044
 * `ovl`/`del` fields crossed that line — every trap's field read became a
 * hash lookup, a 15% deep-dbmon tick regression). Declaring every field in a
 * constructor pre-allocates in-object slots so the map stays fast, with
 * headroom for future fields. It is the proxy's handler: the traps live on
 * its prototype and read it as `this`. */ function TargetShape(e, t, n, r) {
    // Direct field assignment in one fixed order (no Object.assign literal
    // copy): every target shares a hidden-class transition chain — createTarget
    // was the #2 store cost in the uibench creation profile.
    this.v = e;
    // Chained-backing flag (backing IS another store's proxy, §7b) — cached so
    // the hot read path never does a per-read symbol lookup on the backing.
        this.ch = e[$TARGET] !== undefined;
    this.pb = null;
    this.n = null;
    this.h = null;
    this.k = null;
    this.dk = null;
    this.u = t;
    this.pk = n;
    this.px = null;
    this.d = false;
    this.a = false;
    this.sc = 0;
    this.kc = 0;
    this.nc = 0;
    this.rr = r;
    this.o = r !== null && r.opt === true;
    this.s = false;
    this.ovl = false;
    this.del = null;
    this.wk = null;
}

function createTarget(e, t, n, r = t?.rr ?? null) {
    const o = new TargetShape(e, t, n, r);
    // Empty: V8 checks trap results against the proxy target's own keys.
        o.px = new Proxy(Array.isArray(e) ? [] : {}, o);
    setLookup(e, o);
    return o;
}

function wrap(e, t = null, n = null, r = t?.rr ?? null) {
    // markRaw'd values never wrap through ANY store (R42; sticky raw-marking
    // is one half of the never-both-wrapped-and-raw invariant, RUL-12).
    if (rawValuesUsed && isRawValue(e)) return e;
    const o = lookupTarget(e, r);
    if (o !== undefined) return o.px;
    const i = e[$TARGET];
    if (i?.px === e) {
        // Foreign-family proxies re-wrap into THIS family (writes stay isolated);
        // same-family and plain-store proxies pass through.
        if (r === null || i.rr === r) return e;
        return createTarget(e, t, n, r).px;
    }
    return createTarget(e, t, n, r).px;
}

/** Unwrap our own proxies to their current backing; leave everything else.
 * A projection draft wrapper is registered as its target's alias
 * (projection.ts `wrapDraft`) — adopted as a backing, it would forward to
 * itself (#3767). */ function unwrapValue(e) {
    if (e == null || typeof e !== "object") return e;
    const t = e[$TARGET];
    if (t !== undefined && (t.px === e || lookupTarget(e, t.rr) === t) && t.v !== undefined) {
        // A draft escaping into other storage must be a REAL container that
        // becomes this target's committed backing at fold (the shared-raw
        // contract) — a prototype overlay is neither.
        if (t.ovl) materializePB(t);
        return t.pb ?? t.v;
    }
    return e;
}

// ---------------------------------------------------------------------------
// nodes: pure subscription points (values used only for equality gating)
// Shared slot-node equality (create-floor diet): ONE function for every
// leaf — `this` is the node (method-call convention at every _equals site),
// `_host` is the baked-in target backref. Logical-slot equality: values
// resolving to the same child target are the same slot (privatization/
// adoption swap raw identity without changing the logical value — only
// changed leaves notify, R9).
const slotNodeEquals = function(e, t) {
    return isEqual(e, t) || sameLogicalSlot(this.O, e, t);
};

/** The deep witness: the store decides when it notifies. */ const never = false;

function sameKey(e, t) {
    return e === t || e !== e && t !== t;
}

/** The container node's comparator: a staging is a backing (identity — a
 * pending backing is never "equal" to the committed one). A guess never
 * sits on the container: it is the target's carrier's (optimistic.ts, S2). */ const containerEquals = function(e, t) {
    return e === t;
};

function sameLogicalSlot(e, t, n) {
    if (t === null || typeof t !== "object" || n === null || typeof n !== "object") return false;
    const r = lookupTarget(t, e.rr);
    return r !== undefined && r === lookupTarget(n, e.rr);
}

/** Slot nodes whose last subscriber left while they still held a staging:
 * released at the commit that resolves it (`drainFolds`). */ const deferredReleases = new Set;

function releaseSlot(e) {
    const t = e.O;
    const n = e.Xe;
    if (n === $TRACK) {
        if (t.k === e) t.k = null;
    } else if (n === $PATCH) {
        return;
 // a guess carrier lives and dies with its lane (optimistic.ts sweep)
        } else if (n === $DEEP) {
        if (t.dk === e) t.dk = null;
    } else if (t.n?.[n] === e) {
        delete t.n[n];
        // An emptied dictionary is dropped (#5024): the target outlives its
        // readers while the app holds the raw object.
                if (--t.nc === 0) t.n = null;
    } else if (t.h?.[n] === e) {
        delete t.h[n];
        if (Reflect.ownKeys(t.h).length === 0) t.h = null;
    }
    // The family's index keeps only targets that still carry a node.
        if (t.rr !== null && t.nc === 0 && t.k === null && t.dk === null && t.h === null) t.rr.live.delete(t);
}

/** A node was born on `target`: a family target enters the live index. */ function noteNode(e, t, n) {
    if (e.rr !== null) e.rr.live.add(e);
    markDescendants(e);
    if (affectsHooks !== null) affectsHooks.ro(e, t, n);
}

// Shared slot-node release handler: registered once; the core sweep
// dispatches CONFIG_SLOT_NODE nodes here (graph.ts) instead of holding a
// per-node closure in a per-node NodeExtension. A dropped node is
// unreachable through the store — a fresh read makes a fresh node.
// A staged write — or a lane's value (a guess, S4) — is state only the node
// holds: its release waits for the commit that resolves it. So does a leaf of
// a held container (#4146): a mainline write it carried is in the held
// staging too, where it would read as the hold's until the landing.
const slotKept = e => e.ue !== NOT_PENDING || e.l & CONFIG_OVERRIDE || e.O.k?.l & CONFIG_HELD;

setSlotUnobserved(e => {
    if (slotKept(e)) deferredReleases.add(e); else releaseSlot(e);
});

/** A node born while its container has a staging is born from the two
 * frames (A29): committed from the committed backing, the staging from the
 * pending one — and held by the container's transaction when the container
 * is held (#3706: the unit of the hold is the key — a key the batch left
 * unchanged stages nothing and holds no one). */ function bornStaged(e, t, n, r) {
    if (t.ye && t.ye(t.le, n)) return;
    queuePendingNode(t);
    t.ue = n;
    const o = e.k;
    if (o === null) return;
 // a node-less staging holds nothing (`stageOn`)
    // The staging is a derive's lane pass's (`stageOn` → `holdWithDerive`
    // listed the container in the lane, #4478), not a guess's: the leaf takes
    // it as the lane's derivation, as the write would have left a leaf that
    // existed then — a lane reader sees the lane's value, the frame the
    // committed one, as a reader of the memo the derive reads does (E3,
    // D-25). Born a staging instead, the park held it with the frame, and an
    // Errored the memo's lane value heals stayed on its fallback over the
    // store (#4451, D-39).
        if (laneStageHook && (o.l & (CONFIG_OVERRIDE | CONFIG_GUESS)) === CONFIG_OVERRIDE) holdWithDerive(e, t, txOf(o));
    // Held by the container's transaction only for a key the HELD staging
    // changed — a mainline layer's change above it stages mainline. (A
    // container carrying a guess is a lane's: the flush's park decides —
    // unless a correction folded under the guess, D-34: the key is that
    // correction's hold's.)
     else if (o.l & CONFIG_HELD && heldKeyChanged(e, r)) {
        const e = txOf(o);
        if (!(o.l & CONFIG_OVERRIDE) || e.folded) holdNode(t, resolveTx(e._parent ?? e));
    }
    // A28 (4): a batch written inside a creation-time pass is promoted — its
    // late-materialized leaf with it.
        if (o.l & CONFIG_PROMOTED) notePromotedWrite(t);
}

function getNode(e, t, 
// First-read dedupe (create-floor slice 2): the get trap probes
// accessor-ness right before creating the node — pass the verdict through
// so creation skips the second descriptor scan. -1 = unknown (other
// callers), 0/1 = probed.
n = -1) {
    const r = e.n ??= Object.create(null);
    let o = r[t];
    if (o === undefined) {
        // Accessor-ness resolved ONCE per node (no per-object descriptor scan
        // on reads): accessor keys serve through Reflect.get with the proxy
        // receiver.
        const i = e.pb;
        const l = o = slotSignal(committed(e)[t], slotNodeEquals, e, t, n === -1 ? isAccessorKey(i ?? e.v, t) : n === 1);
        if (i !== null && !l.acc) bornStaged(e, l, e.del?.has(t) ? undefined : i[t], t);
        r[t] = o;
        e.nc++;
        noteNode(e, l, t);
    }
    return o;
}

function getHasNode(e, t) {
    const n = e.h ??= Object.create(null);
    let r = n[t];
    if (r === undefined) {
        const o = r = slotSignal(t in committed(e), isEqual, e, t, false);
        const i = e.pb;
        if (i !== null) bornStaged(e, o, t in i && !e.del?.has(t), t);
        n[t] = r;
        noteNode(e, o, t);
    }
    return r;
}

/** The container node (plan sec. 31.3): created by the first structural subscription
 * or the first write that needs to stage. Its committed value is the
 * committed backing. */ function getContainerNode(e) {
    let t = e.k;
    if (t === null) {
        t = e.k = slotSignal(e.v, containerEquals, e, $TRACK, false);
        noteNode(e, t, $TRACK);
        // Born while a batch is staged without a node (`stageOn`): the node
        // takes the staging as its own — the pre-batch backing committed, the
        // pending backing staged — so whoever asked for it reads the frame it
        // would have read had the node been there from the write.
                if (e.pb !== null && !directCommit) {
            t.le = preBatch(e);
            t.ue = e.pb;
            queuePendingNode(t);
        }
    }
    return t;
}

/** Q-A's escape hatch (plan sec. 33; measured plan sec. 41.4): a staging needs the container
 * node as its home only when something can read the frame through it — a
 * structural subscriber (the node exists), lane work (the staging is the
 * lane's), a pass outside a flush (its write is promoted at the pass's end,
 * A28 (4)), or a derive a transaction holds (the staging is the derive's,
 * `holdWithDerive`). Otherwise the backing and the fold queue are the whole
 * staging: nothing allocates, nothing is swept at the commit (dbmon's tick
 * re-created, queued, swept and dropped ~7000 container nodes). If the
 * flush parks, the seam materializes and holds the node then
 * (`storeParkHook`) — the one case the staging home exists for. */ function stageOn(e, t) {
    if (e.k === null && passLane === null && (context === null || globalQueue.Y) && !deriveHolds(e)) return;
    const n = getContainerNode(e);
    if (n.ue === NOT_PENDING) queuePendingNode(n);
    n.ue = t;
    // A28 (4): a write issued inside a pass outside a flush (a creation-time
    // derive) is promoted at the pass's end — the batch with it.
        if (context !== null) notePromotedWrite(n);
    if (e.rr !== null && !userWrite) holdWithDerive(e, n);
}

/** The projection's derive is held by a transaction and this is its own
 * write (not a user setter's): the staging is the derive's (`holdWithDerive`). */ function deriveHolds(e) {
    const t = e.rr?.node;
    return t != null && !userWrite && (t.l & CONFIG_HELD) !== 0;
}

const storePark = e => {
    for (let t = 0; t < foldList.length; t++) {
        const n = foldList[t];
        if (n.k === null && n.pb !== null) holdNode(getContainerNode(n), e);
    }
    optHooks?.park(e);
};

// Set where the only deep node is created, so a bundle without deep() folds its guards away.
let deepUsed = false;

function getDeepNode(e) {
    let t = e.dk;
    if (t === null) {
        deepUsed = true;
        t = e.dk = slotSignal(0, never, e, $DEEP, false);
        noteNode(e, t, $DEEP);
    }
    return t;
}

function bumpDeep(e) {
    if (e.dk !== null) setSignal(e.dk, 1);
}

/** The structural readers' one notification: a membership/arrangement
 * change on the container (the staging is already on the node; `setSignal`
 * re-stages the same backing and walks the subscribers — and, on later
 * steps, routes a held or lane-owned container the way any node is). */ function notifyContainer(e, t) {
    // A container carrying an arrangement guess takes the write as a landing
    // on the guess (`laneWrite` → `supersede`, judged by `containerEquals`);
    // otherwise the structural readers are walked (the staging is already on
    // the node; the comparator is the store's, done).
    // Not a derive's lane pass that listed the container already (`stageOn` →
    // `holdWithDerive`, #4478): there is no guess to land on, the staging is
    // the lane's derivation, and its structural readers are walked like any
    // (their reads of it are the lane's, `laneRead`). Taken as a landing, the
    // write equalled the staging it already was and nobody heard of it: a
    // `mapArray` over the projection kept the old arrangement for good (#4843).
    if ((e.l & (CONFIG_OVERRIDE | CONFIG_GUESS)) === (CONFIG_OVERRIDE | CONFIG_GUESS)) setSignal(e, t); else {
        if (e.l & CONFIG_HELD) joinFuture(txOf(e));
        if (snapshotCaptureActive) captureWriteSnapshot(e, e.le);
        insertSubs(e);
        schedule();
    }
}

// ---------------------------------------------------------------------------
// clones (CoW privatization)
const hasOwn = Object.prototype.hasOwnProperty;

// Aliased: each spelled-out use costs ~20 minified bytes on every store row.
const protoOf = Object.getPrototypeOf;

const setProto = Object.setPrototypeOf;

const lookupGetter = Object.prototype.__lookupGetter__;

const lookupSetter = Object.prototype.__lookupSetter__;

const propertyIsEnumerable = Object.prototype.propertyIsEnumerable;

function isOwnAccessor(e, t) {
    return hasOwn.call(e, t) && (lookupGetter.call(e, t) !== undefined || lookupSetter.call(e, t) !== undefined);
}

// Own accessors, and a class's prototype getters (they run on the proxy).
function isAccessorKey(e, t) {
    return isOwnAccessor(e, t) || t !== "__proto__" && !hasOwn.call(e, t) && lookupGetter.call(e, t) !== undefined;
}

/** Shallow clone into store ownership. Plain data (grade 2) spreads; anything
 * else copies descriptors (accessors kept, everything made configurable). */ function cloneRaw(e, t) {
    t.sc || scanAccessorsOnce(t);
    if (t.sc === 2) {
        const n = {
            ...e
        };
        n[$OWNER] = t;
        return n;
    }
    // A plain array with no accessors and no named keys beyond `length`
    // (`kc` counts own keys) clones by `slice()`: the descriptor path
    // allocates one descriptor per element, O(n) per first draft write (10k
    // rows: ~10 ms per push).
        let n;
    if (!t.a && t.kc <= e.length + 1 && protoOf(e) === Array.prototype) n = e.slice(); else {
        const r = Object.getOwnPropertyDescriptors(e);
        for (const n of Reflect.ownKeys(r)) {
            const o = r[n];
            if (n === "length" && Array.isArray(e)) continue;
            o.configurable = true;
            if (!o.get && !o.set) o.writable = true; else t.a = true;
        }
        // An array subclass keeps its prototype (#4670): the copy is a real
        // array (`Array.isArray`, exotic `length`) re-prototyped to the source's.
                n = Object.defineProperties(Array.isArray(e) ? setProto([], protoOf(e)) : Object.create(protoOf(e)), r);
    }
    n[$OWNER] = t;
    return n;
}

/** Wide plain clone (many keys): null-prototype build, then reset. */ function wideClone(e, t) {
    const n = Object.create(null);
    for (const t of Reflect.ownKeys(e)) n[t] = e[t];
    setProto(n, Object.prototype);
    n[$OWNER] = t;
    return n;
}

function copyOwn(e, t, n) {
    const r = Object.getOwnPropertyDescriptor(t, n);
    if (r.get || r.set || !r.enumerable || !r.writable || !r.configurable) Object.defineProperty(e, n, r); else e[n] = r.value;
}

/** One descriptor scan per container (grade 1/2, `a`, `kc`). */ function scanAccessorsOnce(e) {
    const t = e.v;
    const n = Reflect.ownKeys(t);
    let r = protoOf(t) === Object.prototype;
    for (const o of n) {
        if (isOwnAccessor(t, o)) {
            e.a = true;
            r = false;
            break;
        }
        if (r && !propertyIsEnumerable.call(t, o)) r = false;
    }
    e.sc = r ? 2 : 1;
    e.kc = n.length;
    return !e.a;
}

// ---------------------------------------------------------------------------
// the pending backing
const OVERLAY_MIN_KEYS = 32;

const OVERLAY_REBUILD_MAX_KEYS = 1024;

const OVERLAY_REBUILD_MIN_ADDS = 16;

const WK_ALL = new Set;

const plainProto = e => {
    const t = protoOf(e);
    return t === Object.prototype || t === Array.prototype || t === null;
};

/** An overlay with many adds (or any delete) rebuilds the committed
 * container at flatten so its shape stays fast (#3689). */ function overlayRebuilds(e, t) {
    if (e.kc > OVERLAY_REBUILD_MAX_KEYS) return false;
    if (e.del?.size) return true;
    const n = e.wk;
    if (n !== null && n !== WK_ALL && n.u === undefined && n.size <= OVERLAY_REBUILD_MIN_ADDS) return false;
    const r = e.v;
    let o = 0;
    for (const e of Reflect.ownKeys(t)) if (!hasOwn.call(r, e) && ++o > OVERLAY_REBUILD_MIN_ADDS) return true;
    return false;
}

/** Downgrade an overlay to a real clone (a consumer needs a container). */ function materializePB(e) {
    const t = e.pb;
    if (!e.ovl) return t;
    const n = overlayOnto(e, e.sc === 2 ? wideClone(e.v, e) : cloneRaw(e.v, e), t);
    e.pb = n;
    // The staging IS the pending backing: re-point the container node.
        if (e.k?.ue === t) e.k.ue = n;
    return n;
}

/** The batch's pending backing, created on the first draft write: an overlay
 * of an owned plain-data record above the key threshold, a clone otherwise.
 * Staged on the container node (the scheduler owns its lifetime) and queued
 * for the fold at the flush's commit. */ function ensurePB(e) {
    let t = e.pb;
    // An optimistic edit composes on the view the user saw and becomes
    // guesses at the setter's exit: nothing is staged, nothing folds
    // (optimistic.ts).
        if (optHooks !== null && e.o && userWrite && !pendingNotify.has(e)) return e.pb = optHooks.draft(e);
    if (t !== null && !isOwned(t)) {
        // An adopted object awaiting its fold is the user's: the draft writes a
        // clone of it. The clone is the batch's staging — unless the adoption is
        // HELD (another transaction's): then the held staging stays the
        // adoption and the draft is a mainline layer above it (#3612/#3688: a
        // mainline setter mid-hold publishes mainline; the held keys stay held).
        t = e.pb = cloneRaw(t, e);
        const n = e.k;
        if (n !== null && !(n.l & CONFIG_HELD)) n.ue = t;
    } else if (t === null) {
        const n = e.v;
        if (!directCommit) queueFold(e);
        if (!e.ch && !Array.isArray(n) && (e.sc !== 0 ? !e.a : scanAccessorsOnce(e)) && e.kc > OVERLAY_MIN_KEYS && isOwned(n)) {
            t = e.pb = Object.create(n);
            e.ovl = true;
        } else try {
            t = e.pb = cloneRaw(n, e);
        } catch (e) {
            // #4808: a nested write through a chained store still loading its
            // first answer (the clone reads it) is dropped, as a write to a
            // loading store is — the first answer replaces it (A25) — and the
            // outer keeps following the inner store.
            if (!(e instanceof NotReadyError)) throw e;
            // The write lands on a backing no one reads.
                        return {};
        }
        if (!directCommit) stageOn(e, t);
    }
    return t;
}

/** Privatize `target`'s committed backing (and its ancestor chain — a shared
 * parent cannot point at an owned child). Not a reactive event. */ function privatizeCommitted(e) {
    if (isOwned(e.v)) return;
    const t = e.v;
    const n = cloneRaw(t, e);
    // Mid-batch (a staging is open on this target) the fold still sees the
    // backing the batch started from: the queue recorded it (`foldOlds`).
        e.v = n;
    e.ch = false;
    if (e.u) {
        privatizeCommitted(e.u);
        const n = e.u.v;
        const r = parentSlotKey(e, t);
        if (n[r] === t) n[r] = e.v;
    }
}

/** The key this target sits under in its parent — re-resolved by identity
 * when an array parent moved it (#3282). Not searched when the slot already
 * holds this target's backing (a parent adopted whole carries its children's
 * new raws before the children fold — the miss was O(rows) per changed row
 * of a keyed reconcile). */ function parentSlotKey(e, t) {
    const n = e.pk;
    const r = e.u.v;
    if (r[n] === t || r[n] === e.v || !Array.isArray(r)) return n;
    const o = r.indexOf(t);
    if (o === -1) return n;
    e.pk = o;
    return o;
}

/** Overlay flatten (#3044): own keys onto the owned committed backing in
 * place — the backing keeps its identity. */ function flattenOverlay(e, t) {
    privatizeCommitted(e);
    overlayOnto(e, e.v, t);
 // the caller (`foldTarget`) closes the staging
}

/** An overlay's own keys and deletions onto `to` (the two are disjoint: a
 * write un-deletes its key); the overlay is closed. */ function overlayOnto(e, t, n) {
    for (const r of Reflect.ownKeys(n)) e.sc === 2 ? t[r] = n[r] : copyOwn(t, n, r);
    if (e.del !== null) {
        for (const n of e.del) delete t[n];
        e.kc -= e.del.size;
        e.del = null;
    }
    e.ovl = false;
    return t;
}

/** Adoption (a setter's returned replacement; reconcile and projection
 * landings in S3): the incoming object becomes the backing by reference at
 * the fold — no clones, no user-object mutation, ownership resets to
 * shared. It is the container's staging until then (signal parity: a
 * handler reads the committed frame until the flush; a hold keeps it
 * staged with the rest). The nodes are told now, diffed against the view
 * they were LAST told (#3296): the draft's pending backing when one
 * preceded the adoption this batch — its node writes were real, and the
 * adoption's re-stage cancels or confirms each — else the committed
 * backing. */ function adoptPB(e, t, n = true) {
    if (!directCommit) queueFold(e);
    let r;
    if (e.pb !== null) {
        if (e.ovl) materializePB(e);
        r = e.pb;
    } else r = e.v;
    // #4146 for an adoption (#5005): before a user's adoption swaps a HELD
    // container's staging, its keys get their leaves, born from the two frames,
    // so a change stages on its key as a trap write's would (an array's keys
    // stage as its setter's do).
        if (userWrite && e.k?.l & CONFIG_HELD && !Array.isArray(r)) for (const n in {
        ...r,
        ...t
    }) deferredReleases.add(getNode(e, n));
    if (directCommit) {
        e.pb = null;
        if (e.k !== null) e.k.le = t;
    } else {
        // Eager (INTERNALS §3): the backing swaps now — handlers read the
        // adopted object before the flush — while the container node, when
        // there is one (`stageOn`), keeps the committed frame (`_value`) for
        // readers a hold keeps on it, and stages the adoption for the fold's
        // path copy.
        e.pb = t;
        stageOn(e, t);
    }
    e.v = t;
    e.ch = t[$TARGET] !== undefined;
    e.ovl = false;
    e.del = null;
    e.sc = 0;
    e.a = false;
    e.wk = null;
 // adoption supersedes staged trap writes
        const o = t[$OWNER];
    if (o?.rr === e.rr) t[$OWNER] = e; else setLookup(t, e);
    // S2: the landing reaches each live guess carrier as the truth it is
    // judged against (a returned-rows derive never passes `notifyWrites`).
        if (optHooks !== null && e.o) optHooks.judge(e, t);
    if (n && r !== t) notifyFold(e, r, t);
}

// ---------------------------------------------------------------------------
// the fold: owned raw's one mutation point, at the flush's commit
/** The fold queue: the targets with a batch open (`pb !== null` IS the
 * membership — a target is queued exactly while it carries a staging; the
 * drain skips one already folded), in two reusable arrays. Nothing here
 * allocates per batch once warm: a dbmon tick queues ~7000 containers, and
 * a map built and drained per tick cost ~540 KB of table allocation — most
 * of the tick's young-generation GC, and the whole of its gap to `next`
 * (plan sec. 41.5).
 *
 * Beside each target, its PRE-BATCH committed backing — `t.v` as the batch
 * opened on it (`foldOlds`, parallel to `foldList`). That one record is the
 * fold's base in every case: a draft's backing does not move until the fold
 * (so the record is `t.v` still), an adoption swaps it eagerly (the record
 * is the frame committed readers keep), a mid-batch privatization or a
 * draft over an adopted raw moves it again (the record is what the batch
 * started from, which is all the fold wants to know). The record lives
 * exactly as long as the batch: a weak map keyed by target kept every
 * container's LAST adopted-away backing alive until its next adoption —
 * for a keyed reconcile, the whole previous tree promoted out of the
 * nursery every tick (the saturated listened-paths shape ran 15–25% over
 * the fork, all of it this; plan sec. 43.2) — and for a store adopted
 * once, for its lifetime.
 *
 * The target carries no slot for its record,
 * so a lookup by target (`preBatch`: a container node born onto an open
 * staging, a committed-frame read of a node-less staging, a park) indexes
 * the list through a map built on first use per batch — the drain itself
 * walks the two lists in step and never builds it. */ let foldList = [];

let foldOlds = [];

let foldSpare = [];

let foldOldsSpare = [];

let foldIndex = null;

/** Before the staging is set and before an adoption swaps the backing. */ function queueFold(e) {
    if (e.pb !== null) return;
    schedule();
    if (foldIndex !== null) foldIndex.set(e, foldList.length);
    foldList.push(e);
    foldOlds.push(e.v);
}

/** The committed backing before this batch (valid while `pb !== null`). */ function preBatch(e) {
    if (foldIndex === null) {
        foldIndex = new Map;
        for (let e = 0; e < foldList.length; e++) foldIndex.set(foldList[e], e);
    }
    const t = foldIndex.get(e);
    return t === undefined ? e.v : foldOlds[t];
}

/** The flush committed its pending nodes: fold every pending backing whose
 * container node committed (its staging is gone — `_value` is the backing
 * now) into the committed home; one a later step holds (its node still
 * staged) waits for the flush that commits it. Then the deferred slot
 * releases. */ function drainFolds() {
    if (foldList.length !== 0) {
        const e = foldList;
        const t = foldOlds;
        foldList = foldSpare;
        foldOlds = foldOldsSpare;
        foldSpare = e;
        foldOldsSpare = t;
        foldIndex = null;
        for (let n = 0; n < e.length; n++) {
            const r = e[n];
            if (r.pb === null) continue;
 // folded already (a direct commit)
                        const o = r.k;
            if (o !== null && o.ue !== NOT_PENDING) {
                // held: commits with its hold
                foldList.push(r);
                foldOlds.push(t[n]);
            } else foldTarget(r, t[n]);
        }
        e.length = 0;
        t.length = 0;
    }
    if (deferredReleases.size !== 0) {
        for (const e of deferredReleases) {
            if (e.o === null && !slotKept(e)) {
                deferredReleases.delete(e);
                releaseSlot(e);
            }
        }
    }
}

/** Core asks: is a slot node's truth in flight? Its family's derive is —
 * lanes (A17, #2864: a guess mid-refetch stands in for the flight) and
 * marks (an ambient `affects()` over a declared reload lives to the
 * landing) read it. */ GlobalQueue.k = e => {
    const t = e.O?.rr;
    return (t?.node?.m & STATUS_PENDING) !== 0;
};

installStoreHooks(storePark, () => {
    drainFolds();
    optHooks?.sweep();
});

/** The store half of `affects()` (store/affects.ts) makes its marks' nodes
 * and walks records through these: it never imports the engine. */ GlobalQueue.so = getNode;

GlobalQueue.fo = isWrappable;

/** A projection's creation run commits directly (a memo's first value is
 * its `_value`, not a staging): the draft still writes a clone of the seed,
 * nothing is staged on the nodes, and the setter's exit folds the touched
 * containers at once. Only the synchronous first run. */ let directCommit = false;

function runDirect(e) {
    const t = directCommit;
    directCommit = true;
    try {
        e();
    } finally {
        directCommit = t;
    }
}

/** Fold one target's pending backing into its committed home. */ function foldTarget(e, t) {
    {
        {
            const n = e.k;
            const r = e.pb;
            if (r !== null) {
                if (r === e.v) ; else if (e.ovl && (e.v !== t || !overlayRebuilds(e, r) && n?.T?.De !== t)) {
                    // Overlay flatten (#3044): the backing keeps its identity, so the
                    // `t.v === old` gate below skips path copying (the parent slot
                    // already points here) and the adopted-notify (setter
                    // notifications happened at write time). A backing privatized
                    // mid-batch is a fresh clone the parent already points at.
                    flattenOverlay(e, r);
                } else if (e.v !== t) {
                    // Privatized mid-batch (a child's path copy repointed us): the
                    // owned backing takes the written keys.
                    privatizeCommitted(e);
                    const n = e.v;
                    const o = e.wk;
                    if (o !== null && o !== WK_ALL) {
                        for (let e = o; e !== undefined; e = e.u) for (const t of e) {
                            if (hasOwn.call(r, t)) copyOwn(n, r, t); else delete n[t];
                        }
                    } else {
                        for (const e of Reflect.ownKeys(r)) if (e !== $OWNER) copyOwn(n, r, e);
                        for (const e of Reflect.ownKeys(t)) {
                            if (e !== $OWNER && !hasOwn.call(r, e)) delete n[e];
                        }
                    }
                } else {
                    // The clone (or adopted object) becomes the committed backing
                    // (path copy below).
                    e.v = e.ovl ? materializePB(e) : r;
                    e.ch = e.v[$TARGET] !== undefined;
                }
                e.pb = null;
                e.wk = null;
 // the written-keys window closes with the commit
                        }
            if (n !== null) {
                // The node's committed value is the backing. A container nobody
                // subscribes to was a transient staging home (INTERNALS §5: an
                // observer-less write holds a transient record until the fold,
                // discarded with it) — gone with the batch.
                if (n.o === null) e.k = null; else n.le = e.v;
            }
            if (e.v !== t && e.u) {
                const n = parentSlotKey(e, t);
                if (e.u.v[n] === t) {
                    privatizeCommitted(e.u);
                    e.u.v[n] = e.v;
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// write-time notification (setter exit)
/** A user setter wrote a key whose staging is a derivation another
 * transaction holds (A34 (3), #3612): the hold re-derives over the write. */ let heldDerivationHit = false;

/** The open setter is a USER'S (`storeSetter(…, guard = true)`), not the
 * derive's draft or landing. */ let userWrite = false;

/** A hold whose staging the outermost user setter read from its draft (#5046). */ let draftReadTx = null;

/** Is the open setter a user's? (reconcile.ts: a user's reconcile on an
 * optimistic family writes the draft.) */ function userWriting() {
    return userWrite;
}

function heldDerivation(e, t, n) {
    if (e.rr === null || !userWrite) return false;
    if (t !== undefined) return (t.l & (CONFIG_HELD | CONFIG_MANUAL_WRITE)) === CONFIG_HELD && txOf(t) !== flushTransaction;
    const r = e.k;
    return r !== null && (r.l & CONFIG_HELD) !== 0 && txOf(r) !== flushTransaction && heldKeyChanged(e, n);
}

/** Setter exit: one node write per changed observed key (the leaves stage
 * like signals — holds and lanes engage here on later steps), presence and
 * deep witnesses, and the container when membership changed. */ function notifyWrites(e) {
    let t = e.pb;
    if (t === null) return;
    // Optimistic channel: a user's writes on an optimistic family become
    // guesses on the nodes (lanes); the draft is discarded, the committed
    // backing never touched.
        if (optHooks !== null && e.o) {
        if (userWrite) {
            // The draft is discarded first: the diff reads the target (its nodes
            // are born from the committed frame, not the draft).
            e.pb = null;
            e.wk = null;
            e.pb = optHooks.writes(e, t);
            return;
        }
        // The derive's draft: new rows under guessed rows are paired with them
        // first (D-34, #4329); the frame then reaches each live guess carrier as
        // the truth it is judged against (S2).
                optHooks.pair(e, t);
        optHooks.judge(e, t);
    }
    const n = e.v;
    const r = e.n;
    // Written-keys bound: trap writes record their keys, so the notify visits
    // O(written) nodes instead of every subscription on the record (a selection
    // map with thousands of per-key subscribers pays two visits per select,
    // not a full scan). Falls back to the full node scan when the bound can't
    // hold: no trap granularity (wk null), an array length write (WK_ALL —
    // implicit index deletes), accessors on the record (t.a — a getter node's
    // value can change when ANY key is written), or a non-plain prototype
    // (class instances: prototype getters derive from arbitrary fields).
    // Overlay pbs chain to the COMMITTED object (#3044) — plainness is the
    // committed container's prototype, not the overlay's. THIS setter's keys
    // only (D-8, #3743): a key an earlier setter of the batch wrote and this
    // one did not is no second proposal — its node was told then, and telling
    // a held node its unchanged value again would join this tick to the hold.
        const o = e.wk;
    const i = o === WK_ALL || e.a === true || !plainProto(e.ovl ? e.v : t) ? null : o;
    if (e.rr !== null && !heldDerivationHit && i !== null) for (const t of i) if (heldDerivation(e, r?.[t], t)) {
        heldDerivationHit = true;
        break;
    }
    if (r !== null) {
        const o = i ?? Reflect.ownKeys(r);
        for (const i of o) {
            const o = r[i];
            if (o === undefined) continue;
            let l = false;
            if (o.acc === true || (l = hasOwn.call(t, i)) && lookupGetter.call(t, i) !== undefined) {
                // Accessor keys: the node is linked for shape-change notification,
                // its value is never served (the getter runs with the proxy
                // receiver on read) — FORCE wakes the readers. A data descriptor
                // falls through so the cleared-acc hot read serves it (#3949);
                // an own getter's descriptor has no `value`, so the compare below
                // would otherwise keep the getter's result (or FORCE) for `undefined`.
                o.acc = isAccessorKey(t, i);
                const e = Object.getOwnPropertyDescriptor(n, i);
                const r = Object.getOwnPropertyDescriptor(t, i);
                if (!r || r.get || r.set) {
                    if (e?.get !== r?.get || e?.set !== r?.set || e?.value !== r?.value) setSignal(o, () => FORCE);
                    continue;
                }
                if (e?.get || e?.set || !isEqual(e?.value, r.value)) setSignal(o, () => r.value);
                continue;
            }
            // A removed `__proto__` is told `undefined`, never the prototype (core
            // R30, #4672); an own key pays nothing. (An overlay's written keys are
            // own on it or in `del`.)
                        const f = l || !(e.del?.has(i) || i === "__proto__") ? t[i] : undefined;
            // A key a live guess covers: its readers read the carrier, and the
            // carrier's judgement of this landing tells them — a correction through
            // the carrier, a confirmation silently (A17). The leaf records the
            // value and says nothing (S2).
                        if (optHooks !== null && e.o && optHooks.covers(e, i, false)) {
                coveredSet(o, f);
                continue;
            }
            setSignal(o, () => f);
            if (e.rr !== null) {
                // The derive's write is a derivation; a user setter's is a proposal
                // (A34 (3)'s discriminator).
                if (!userWrite) {
                    o.l &= ~CONFIG_MANUAL_WRITE;
                    holdWithDerive(e, o);
                } else o.l |= CONFIG_MANUAL_WRITE;
            }
        }
    }
    const l = e.h;
    if (l !== null) {
        const n = i ?? Reflect.ownKeys(l);
        for (const r of n) {
            const n = l[r];
            if (n !== undefined) {
                if (optHooks !== null && e.o && optHooks.covers(e, r, true)) {
                    coveredSet(n, r in t && !e.del?.has(r));
                    continue;
                }
                setSignal(n, r in t && !e.del?.has(r));
                if (e.rr !== null && !userWrite) holdWithDerive(e, n);
            }
        }
    }
    if (deepUsed && e.dk !== null) {
        if (e.del?.size) bumpDeep(e); else for (const r of i ?? Reflect.ownKeys(t)) {
            if (r === $OWNER) continue;
            const o = t[r];
            const i = n[r];
            if (o !== null && typeof o === "object" ? !targetsEqual(i, o) : !isEqual(i, o)) {
                bumpDeep(e);
                break;
            }
        }
    }
    // The container: structural readers hear membership/arrangement changes
    // only (R9) — the compare is the store's, against the committed frame. A
    // container carrying an arrangement guess is told whether or not anyone
    // subscribes: the write is a landing on the guess (`notifyContainer`).
        const f = e.k;
    if (f !== null && (f.o !== null || f.l & CONFIG_OVERRIDE)) {
        let r;
        if (e.ovl) {
            r = !!e.del?.size;
            if (!r) {
                for (const e of Reflect.ownKeys(t)) {
                    if (!hasOwn.call(n, e)) {
                        r = true;
                        break;
                    }
                }
            }
        } else {
            r = Array.isArray(t) && Array.isArray(n) ? arrayStructureChanged(n, t) : membershipChanged(n, t);
        }
        if (r) notifyContainer(f, t);
    }
}

/** A covered leaf records the truth and says nothing (S2). A staging it
 * was born with or held from an older landing (`bornStaged`) is that older
 * truth: the hold's commit lands this one, never the older one once the
 * guess ends (#4904). */ function coveredSet(e, t) {
    e.le = t;
    if (e.ue !== NOT_PENDING) e.ue = t;
}

/** Accessor-node wake sentinel: readers re-run and re-invoke the getter. */ const FORCE = Symbol();

/** Two raws that wrap to the same target are the same logical value. */ function targetsEqual(e, t) {
    if (e === null || typeof e !== "object" || t === null || typeof t !== "object") return false;
    const n = lookupTarget(e, null);
    return n !== undefined && n === lookupTarget(t, null);
}

/** Positional identity IS content for arrays (R15). */ function arrayStructureChanged(e, t) {
    if (e.length !== t.length) return true;
    for (let n = 0; n < t.length; n++) {
        const r = e[n];
        const o = t[n];
        if (!isEqual(r, o) && !targetsEqual(r, o)) return true;
    }
    return false;
}

function membershipChanged(e, t) {
    const n = Reflect.ownKeys(t);
    if (Reflect.ownKeys(e).length - +isOwned(e) !== n.length - +isOwned(t)) return true;
    for (const t of n) if (t !== $OWNER && !(t in e)) return true;
    return false;
}

/** An adoption's leaf write. A derive's pass that is a lane's work (a
 * display-ahead pass, F-6) writes the lane's derivation (D-25), as its draft
 * writes do (`notifyWrites` → `holdWithDerive`): the lane's value, the
 * committed value — and a truth a transaction holds beneath — untouched
 * (#4631: staged into the hold instead, the leaf showed the screen beside a
 * container showing the lane). */ function leafSet(e, t) {
    // A key a live guess covers: its readers read the carrier, and the
    // carrier's judgement of this landing tells them — a correction through
    // the carrier, a confirmation silently (A17). The leaf records the value
    // and says nothing (S2).
    const n = e.O;
    if (optHooks !== null && n.o && optHooks.covers(n, e.Xe, n.h?.[e.Xe] === e)) {
        coveredSet(e, typeof t === "function" ? t() : t);
        return;
    }
    setSignal(e, t);
    if (passLane !== null && !userWrite && e.O.rr !== null) holdWithDerive(e.O, e);
}

/** One leaf's notification for an adoption diff (descriptor-aware). */ function notifyKeyDiff(e, t, n, r, o = true) {
    if (e.acc === true || o && hasOwn.call(r, t) && lookupGetter.call(r, t) !== undefined) {
        e.acc = isAccessorKey(r, t);
        const o = Object.getOwnPropertyDescriptor(n, t);
        const i = Object.getOwnPropertyDescriptor(r, t);
        // Data descriptors fall through, as in notifyWrites (#3949).
                if (!i || i.get || i.set) {
            if (o?.get !== i?.get || o?.set !== i?.set || o?.value !== i?.value) leafSet(e, () => FORCE);
            return;
        }
        const l = o?.value;
        const f = i?.value;
        if (o?.get || o?.set || !isEqual(l, f) && !targetsEqual(l, f)) leafSet(e, typeof f === "function" ? () => f : f);
    } else {
        const o = n[t];
        const i = r[t];
        // A live chained `old` (a store proxy, §7b) already reads the inner
        // store's next frame, not what the node was last told: written, the
        // node's own equality judging it (#4361).
                if (n[$TARGET] || !isEqual(o, i) && !targetsEqual(o, i)) leafSet(e, typeof i === "function" ? () => i : i);
    }
}

function hasAccessorFlag(e) {
    return e.acc === true;
}

/** One leaf's notification when the caller already has both values. */ function notifyKeyValue(e, t, n, r, o, i) {
    if (e.acc === true) {
        notifyKeyDiff(e, t, o, i, false);
        return;
    }
    if (!isEqual(n, r) && !targetsEqual(n, r)) leafSet(e, typeof r === "function" ? () => r : r);
}

/** The structural half of an adoption's notifications (reconcile does the
 * leaves itself): presence by `in`, the container by membership. Presence
 * is DIFFED `old` → `neu` like the leaves (#3743): a presence node is
 * written only where `in` changed between the view it was last told
 * (#3296) and the adoption — `setSignal` joins a held node's transaction
 * before its equality gate (A34 (1)), so repeating an unchanged absence to
 * a node an action holds would make the whole tick the action's (an
 * unrelated `a.value` stayed stale until the action settled). A real
 * change still writes, and on a held node still proposes. A live chained
 * `old` (a store proxy, §7b) reflects the inner store, not what the node
 * was last told — written unconditionally, as before; its container too
 * (#4361).
 *
 * The container is told of an arrangement change when it has structural
 * subscribers OR carries an arrangement guess (S4): for a guessed
 * container the write is the landing that judges the guess (Q-D, plan
 * sec. 39 — confirm, supersede, or hold beneath), not a subscriber
 * notification, and a landing whose truth changed no guessed key's value
 * or presence (a newer question's rows beneath an optimistic push) still
 * answers the arrangement — before, only the presence write reached the
 * lane, by accident of being unconditional. */ function notifyFoldTail(e, t, n) {
    const r = e.h;
    const o = t[$TARGET];
    if (r !== null) {
        for (const e of Reflect.ownKeys(r)) if (o || e in t !== e in n) leafSet(r[e], e in n);
    }
    const i = e.k;
    if (i !== null && (i.o !== null || i.l & CONFIG_OVERRIDE)) {
        const e = o || (Array.isArray(n) && Array.isArray(t) ? arrayStructureChanged(t, n) : membershipChanged(t, n));
        if (e) notifyContainer(i, n);
    }
}

/** An adoption's notifications: every node of the target, diffed `old` →
 * `neu` (value nodes by key, presence by `in`, the container by
 * membership, the deep witness by identity). */ function notifyFold(e, t, n) {
    if (deepUsed && e.dk !== null && t !== n) bumpDeep(e);
    const r = e.n;
    if (r !== null) {
        for (const e of Reflect.ownKeys(r)) notifyKeyDiff(r[e], e, t, n);
    }
    notifyFoldTail(e, t, n);
}

// ---------------------------------------------------------------------------
// visibility
let writing = 0;

/** Roots of the stores with an open setter: reads of THEIR drafts are the
 * writer's (no tracking, pending backing served); every other store reads
 * normally — a derive's external reads are its dependencies (#3037). */ let writeScopes = null;

const pendingNotify = new Set;

function scopeKey(e) {
    if (e.rr !== null) return e.rr;
    let t = e;
    while (t.u !== null) t = t.u;
    return t;
}

function inDraft(e) {
    return writeScopes !== null && writeScopes.has(scopeKey(e));
}

/** Shallow store: values served verbatim (a nested store proxy still wraps
 * into the family so writes stay isolated). */ function serveShallow(e, t, n) {
    if (n !== null && typeof n === "object" && n[$TARGET] !== undefined) return draftServe(e, wrap(n, e, t));
    return n;
}

/** A child proxy served to a draft joins the draft's scope. */ function draftServe(e, t) {
    if (writeScopes !== null && inDraft(e)) {
        const e = t?.[$TARGET];
        if (e?.v !== undefined) writeScopes.add(scopeKey(e));
    }
    return t;
}

/** The committed backing: the container node's committed value while a
 * batch is staged on it (an adoption swaps `t.v` eagerly — INTERNALS §3,
 * "adoption is eager by contract" — and the node keeps the frame committed
 * readers under a hold still see), `t.v` otherwise. */ function committed(e) {
    const t = e.k;
    if (t !== null) return t.ue !== NOT_PENDING ? t.le : e.v;
    // A node-less staging (`stageOn`): the fold queue remembers the frame.
        return e.pb !== null ? preBatch(e) : e.v;
}

/** The pass that sees this flush's staging (signal parity — `read()`'s
 * staged arm): the computation running (`context`, never the owner — a
 * handler, an effect callback or `onSettled` has an owner but no pass),
 * unless it is a frame reader (children-forbidden, A32). Null = the
 * committed frame. */ function stagingReader() {
    let e = context;
    if (e === null) return null;
    if (e.Qe) e = e.Ke ?? null;
    return e === null || e.l & CONFIG_CHILDREN_FORBIDDEN ? null : e;
}

/** The backing a read serves (RUL-1, signal parity): the draft and the write
 * override read the pending backing; a tracked reader reads it too — only
 * for presence and descriptors, its leaf (born from the two frames) decides
 * the value through core; a context-free or frame reader the committed
 * backing; a pass reading UNTRACKED asks the container node — `read()`
 * without a link: the node's value is the backing, chosen by the hold rules
 * (a render effect outside the parking flush sees committed and re-derives
 * at the landing; a memo joins the future; a staging is marked read). */
/** The backing a read of `target` is served from — the truth (committed,
 * or the staging a pass may see). An optimistic family's lane values are
 * NOT composed here: a guessed key always has its node, and the node is
 * what serves its value (`nodeValue`); presence and enumeration compose
 * per key / per walk (`optimisticHas`, `enumerationSource`). */ function readSource(e, t, n = false) {
    // An optimistic setter's read before its first write sees what the writer
    // sees — the lanes' values and the tick's own guesses over the truth
    // (#3665), never a staging awaiting its fold: the draft is born on the
    // read (and notified at the setter's exit like any).
    if (optHooks !== null && userWrite && e.o && !pendingNotify.has(e) && inDraft(e)) {
        const t = ensurePB(e);
        pendingNotify.add(e);
        return t;
    }
    const r = e.pb;
    if (r === null) return e.v;
    if (inDraft(e) || writeOverride) {
        // The derive reads its own store through its draft: a key (or the
        // container) a transaction holds — a user's write inside an action,
        // held with it — is the future, and the pass that reads it derives
        // from the future (L2: a read of something already held joins it): the
        // derive's whole result reveals with the action (core R31 / #3733:
        // "under the transaction and revealed with it"), not key by key. A
        // user's setter reading its draft is no pass: it notes the hold, and
        // its writes join it at the setter's exit (#5046, as D-33's guess
        // written after reading another hold's guess does).
        if (userWrite !== writeOverride) {
            const n = t !== undefined ? e.n?.[t] : undefined;
            const r = n ?? e.k;
            if (r != null && (r.l & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD && (n !== undefined || t === undefined || heldKeyChanged(e, t))) userWrite ? draftReadTx = txOf(r) : joinPassTx(txOf(r));
        }
        return r;
    }
    const o = e.k;
    // A key whose leaf carries a lane's value (a guess, S4): the leaf serves
    // it — the container's frame is not consulted (a reader is not made a
    // stale reader of the container's hold for a key it does not see through
    // it). Unless the container carries the same lane's value: then the
    // container's frame is the one the leaf is in, and presence and keys read
    // it, as the leaf's value does (#4821: a projection a lane pass derived).
    // (A leaf whose lane is gone matches no container: `|| 0`.)
        if (t !== undefined) {
        const n = e.n?.[t];
        if (n?.l & CONFIG_OVERRIDE && o?.T?.Re !== (n.T.Re || 0)) return committed(e);
    }
    const i = o !== null && (o.l & CONFIG_HELD) !== 0;
    // A held container: the hold's unit is the key (#3706). A key the HELD
    // staging left unchanged reads the ambient view — a mainline layer's
    // write above the hold publishes mainline (#3688, #3612); a key it
    // changed reads the frame core serves this reader.
        if (i && t !== undefined && !heldKeyChanged(e, t, n)) return r;
    const l = stagingReader();
    if (l === null) {
        if (getObserver() !== null) return r;
 // a frame reader's leaf decides the value
        // A projection's writes are its truth as soon as made (the derive is
        // the authority, nothing it writes is a proposal): a context-free reader
        // sees them before the flush — unless a hold keeps them staged.
                if (familyAhead(e)) return r;
        // A verdict window (`isPending`/`latest`) with no reader (a top-level
        // probe) judges the container like any node — for a key the batch
        // changed; an unchanged key is committed and final (A22: pending is per
        // key). A held container's committed frame is the node's.
                if (verdict === null || !i && t !== undefined && !keyChanged(e, t, n)) return i ? o.le : e.v;
        return o === null ? committed(e) : read(o);
    }
    // A pass reading a key the batch left unchanged: the same value in both
    // frames — committed, holding no one. Otherwise the container's frame by
    // core's rules — `read()` of the node WITHOUT a link (a value reader never
    // subscribes to the container): a render effect outside a hold's flush
    // sees committed and re-derives at the landing, a memo joins the future,
    // a staging read is marked. The key's own leaf, when it has one, decides
    // the value the same way.
        if (!i) {
        // A plain pass this flush already served a staging as the screen
        // (REACTIVE_STAGED_READ — what `read()` would set again here) reads an
        // unheld staging directly: an unchanged key is the same value in both
        // frames, a changed one is what the mark already stands for. The
        // per-key question is asked once per pass, not once per key (a
        // `mapArray` over 1k shifted rows asked it 1k times).
        if (l.R & REACTIVE_STAGED_READ && !(l.l & (CONFIG_VERDICT | CONFIG_CHILDREN_FORBIDDEN))) return r;
        if (t !== undefined && !keyChanged(e, t, n)) return committed(e);
        // A node-less staging (`stageOn`): what `read()` of its node would do
        // for this pass — a verdict reader sees the screen, any other derives
        // from the staging and is marked as having read it.
                if (o === null) {
            if (l.l & CONFIG_VERDICT && stagedScreen(l)) return committed(e);
            stagedRead(l);
            return r;
        }
    }
    return untrack(() => read(o));
}

/** Did the HELD staging (the container node's, not a mainline layer above
 * it) change `key`? */ function heldKeyChanged(e, t, n = true) {
    return changedBetween(e, e.k.le, e.k.ue, t, n) && !adoptChild(e, t);
}

/** The values of a held frame, built once per frame: neither the committed
 * frame nor the held staging changes while the hold stands (a draft writes a
 * clone), so the set stays true for every first read of the hold. */ const frameRaws = new WeakMap;

function rawsOf(e) {
    let t = frameRaws.get(e);
    if (t === undefined) frameRaws.set(e, t = new Set(Object.values(e)));
    return t;
}

/** The same row at a slot, as the adoption pairs rows (reconcile.ts): two
 * objects whose keys agree — positionally for an unkeyed family. */ function sameRow(e, t, n) {
    return e !== null && typeof e === "object" && t !== null && typeof t === "object" && (n === null || sameKey(n(e), n(t)));
}

/** A key of a held adoption whose old and new values are the same row that
 * nothing had proxied when the adoption ran, so `descend` never paired
 * them: paired now, as it would have been (#3712) — one target, the old
 * raw its committed frame, the new raw its staging, held by the same
 * transaction. Both raws resolve to it, so the slot reads as unchanged and
 * the row's own leaves decide: `id` the same in both frames holds no one,
 * `version` holds. A record's keys pair by name; an array's by the family's
 * key — without a family the pairing is not known here and the hold stands. */ function adoptChild(e, t) {
    const n = e.k;
    const r = n.le;
    const o = r[t];
    const i = n.ue[t];
    const l = Array.isArray(r) ? e.rr?.key : null;
    if (e.s || l === undefined || !sameRow(o, i, l) || !isWrappable(o) || !isWrappable(i) || Array.isArray(o) !== Array.isArray(i) || lookupTarget(o, e.rr) !== undefined || lookupTarget(i, e.rr) !== undefined || 
    // A raw in both frames (the same objects swapped or reordered) is its
    // own row: pairing it with another would alias the two after the commit.
    rawsOf(r).has(i) || rawsOf(n.ue).has(o)) return false;
    // A raw-marked value wraps to itself, a proxy to its own target: neither
    // is a fresh target around `ov`.
        const f = wrap(o, e, t)[$TARGET];
    if (f?.v !== o) return false;
    adoptPB(f, i, false);
    holdNode(getContainerNode(f), txOf(n));
    return true;
}

/** A family target's unheld staging, read with no pass (a handler). */ function familyAhead(e) {
    return e.rr !== null && context === null && !(e.k?.l & CONFIG_HELD);
}

/** Did this batch change `key` — value (slot equality), presence, and (for
 * a `shape` reader — a descriptor read, a node's hold) enumerability or
 * accessor-ness? A swapped or non-plain prototype, or a chained backing,
 * changes every key (#3706: those hold the whole container).
 *
 * A VALUE read asks without `shape`: a key whose value is the same in both
 * frames serves the same value from either, whatever its descriptor did —
 * and the descriptor probes (`__lookupGetter__`, `propertyIsEnumerable`,
 * twice each) are the cost of the question on a wide container read under
 * a staging (a keyed reconcile's 1k rows, read by `mapArray` in the flush
 * that staged them: ~3× the tick). A container with accessors seen (`a`)
 * keeps the full test — a getter's answer is not a slot's. */ function keyChanged(e, t, n = true) {
    return changedBetween(e, committed(e), e.pb, t, n);
}

function changedBetween(e, t, n, r, o) {
    if (e.ch || n[$TARGET] !== undefined || protoOf(n) !== protoOf(t) || !plainProto(t)) return true;
    if (e.del?.has(r)) return r in t;
    const i = hasOwn.call(n, r) || e.ovl && hasOwn.call(e.v, r);
    if (i !== hasOwn.call(t, r)) return true;
    if (!i) return false;
    if ((o || e.a) && (isOwnAccessor(t, r) || isOwnAccessor(n, r) || propertyIsEnumerable.call(t, r) !== propertyIsEnumerable.call(n, r))) return true;
    const l = n[r];
    const f = t[r];
    return !isEqual(f, l) && !sameLogicalSlot(e, f, l);
}

/** Value of a key with a leaf: core's — `read()` links a tracked reader and,
 * tracked or not, applies the hold and A28 rules a signal's read does. A
 * projection's unheld staging is ahead for a handler (`familyAhead`), and is
 * the only value while its derive has never committed: the committed side
 * is the seed, never observable (A25) — a reader core serves the screen (a
 * verdict reader, #4355) is served the staging, as for a memo born staged
 * (core `serve`). */ function nodeValue(e, t, n) {
    let r;
    if (e.ue !== NOT_PENDING && !(e.l & CONFIG_HELD) && n.rr !== null && (context === null || n.rr.node?.m & STATUS_UNINITIALIZED)) {
        if (context !== null && tracking) link(e, context);
        r = e.ue;
    } else r = read(e);
    return r === FORCE ? t : r;
}

/** Serve a present data key: link the leaf for a tracked reader (creating it
 * on first read), wrap a wrappable child through the per-node wrap cache. */ function serveDataKey(e, t, n, r, o, i = -1) {
    // Read-through (§7b): the inner store's node is the truth; the outer's is
    // a subscription point only (an optimistic target's guesses are served by
    // its carrier before this, #4762).
    const l = e.ch && r === e.v;
    let f = n;
    // A write-override read (a derive's draft) serves the backing: the derive
    // composes on the truth, never on a lane's or a hold's frame.
        if (!inDraft(e) && !writeOverride) {
        if (o === undefined && getObserver() !== null) o = getNode(e, t, i);
        if (o !== undefined) {
            const t = nodeValue(o, n, e);
            if (l) {
                // The link's committed value follows the inner store's (what a lane
                // over it covers — its reveal and revert compare against it, S4).
                o.le = n;
            } else f = t;
        }
    }
    // The tail is `serveKeyValue`, inlined: this is every store read's path,
    // and a call here showed on CodSpeed as −6 to −9% on the store reads
    // (#4729, third round); the optimistic hook's carrier read takes the
    // function.
        if (e.ch && !l && f !== null && typeof f === "object" && f[$TARGET]?.px !== f) f = resolveChainedRaw(e, t, f);
    if (e.s) return serveShallow(e, t, f);
    if (o !== undefined) {
        if (o.pxv === f && f !== undefined) return draftServe(e, o.px);
        if (!isWrappable(f)) return f;
        const n = wrap(f, e, t);
        o.px = n;
        o.pxv = f;
        return draftServe(e, n);
    }
    if (!isWrappable(f)) return f;
    return draftServe(e, wrap(f, e, t));
}

/** The value of `key` as the reader gets it (the tail of `serveDataKey`, for
 * the optimistic hook's carrier read): shallow-served, a chained raw
 * resolved to the inner store's (#3767, #3859), a wrappable wrapped (cached
 * on the key's node by value identity). */ function serveKeyValue(e, t, n, r, o) {
    // A derive's draft wrapper answers `$TARGET` without being a proxy: the
    // inner store's alias (#3767), resolved like a raw (#3859).
    if (e.ch && true && n !== null && typeof n === "object" && n[$TARGET]?.px !== n) n = resolveChainedRaw(e, t, n);
    if (e.s) return serveShallow(e, t, n);
    if (o !== undefined) {
        if (o.pxv === n && n !== undefined) return draftServe(e, o.px);
        if (!isWrappable(n)) return n;
        const r = wrap(n, e, t);
        o.px = r;
        o.pxv = n;
        return draftServe(e, r);
    }
    if (!isWrappable(n)) return n;
    return draftServe(e, wrap(n, e, t));
}

/** Is `c` among `n`'s subscribers (any pass's link — a kept tail included)?
 * `n`'s subs, not `c`'s deps: the derive's are its few flight observers. */ function linkedTo(e, t) {
    for (let n = t.o; n !== null; n = n.t) if (n.u === e) return true;
    return false;
}

/** Does `n` read `c`, through any chain of computations? Only a chain of
 * nodes above `c` can reach it, so the walk stops at `c`'s height (a signal
 * has no height and ends it too). */ function feeds(e, t) {
    for (let n = e.C; n; n = n.G) {
        const e = n.S;
        if (e === t || e.he > t.he && feeds(e, t)) return true;
    }
    return false;
}

/** The projection's obligation (plan sec. 31.7 Q-B): before any value is served
 * through a family target, the derive — the firewall — is brought up to
 * date WITHOUT the reader subscribing to it. Core's `read()` of the
 * firewall with no link does all of it: the pull, the status gate
 * (uninitialized / pending → NotReady, errored → the error; a verdict
 * window judges it; a pending source registers the reader for the landing)
 * and the reader's height. The derive's own draft ops are exempt (the write
 * override): the derive is the author. */ function pullFamily(e) {
    const t = e.rr.node;
    if (t === null || writeOverride || inDraft(e)) return;
    // A reader the derive itself reads through (its row handed back into its
    // own source, #4970 / #4972) is the derive's input, not its reader: it
    // reads the leaves as the derive last wrote them, like any signal. Pulling
    // or linking the derive from it would be a cycle: each pass lifted it
    // above the derive that sits above it, without bound.
        const n = context;
    if (n?.o && feeds(t, n)) return;
    // A settled derive: the pull alone, no link (the barrier). A derive with a
    // flight up (or errored): the reader observes the flight exactly as a
    // reader of a memo would — linked, so it re-runs at the landing (the
    // next settled pass pulls without linking and the stale link trims), a
    // verdict reader registered, a tracked pass suspended.
    // A staged error (D-41) throws at the pull, before any leaf would link the reader.
        if (t.m & (STATUS_PENDING | STATUS_ERROR) || t.l & CONFIG_STAGED_ERROR) {
        // An optimistic family carrying guesses that stand in for the flight
        // (A17): the guesses are the display — a reader is served the leaves
        // (committed plus the guesses) and does not suspend on the derive; a
        // verdict window still asks it (`isPending` is true while the flight is
        // up). The family's, not the target's: the edit stands in for the one
        // flight the family has — not over a new question they do not answer
        // (optimistic.ts `stands`, #4336). A reader observing the flight keeps
        // observing it (`stands`, #4536). Judged first: the hook sees the frame's
        // readers too.
        if (verdict === null && t.m & STATUS_PENDING && optHooks?.stands(e)) return;
        // A render effect outside the flight's own flush is the frame, not a
        // derivation: it keeps what it shows and learns of the landing from the
        // leaves the landing changes (unchanged leaves say nothing — it is not a
        // stale reader of the derive). Inside the flush it suspends like any.
        // One that already observes the flight (linked by the pass that saw it
        // go up — the frame's hold, `blocked`) keeps observing it: a re-run for
        // another reason (a guess it read, the seam) that dropped the link would
        // release the hold with the flight still up. An errored derive has no
        // landing to learn of: the error is what the frame shows.
                if (n !== null && n.i === EFFECT_RENDER && t.m & STATUS_PENDING && t.l & CONFIG_HELD && flushTransaction !== txOf(t) && !linkedTo(n, t)) return;
        // A pass the pull throws a staged error (D-41) is linked to the derive,
        // untracked too — `handleAsync` asking a returned proxy whether it is a
        // flight: the pass carries the error, and the re-ask or answer that
        // supersedes it must reach it (#4527; a tracked read links anyway, #4519).
        // Only a computation has deps to link: a bare owner (`mapArray`'s row
        // update under `runWithOwner`, #4805) reads as untracked.
                if (n?.C !== undefined && t.l & CONFIG_STAGED_ERROR) link(t, n);
        // D-30: a derive that is a verdict reader only through what it reads
        // (below) and waits on a flight it read as a lane's work: its reader
        // reads that flight as the memo's reader would. A branch a display-ahead
        // read mounted is born held on it (S-46) — read through the derive
        // alone, it waited with the lane and attached at the lane's reveal,
        // ahead of the landing its memo form attaches at.
        // (The condition is spelled out, as below: behind a helper the branch
        // does not fold away without the verdict engine.)
                if (observeFlightHook && t.l & CONFIG_VERDICT && !(t.R & EFFECT_RENDER << 21) && tracking) for (let e = t.C; e; e = e.G) if (e.S.m & STATUS_PENDING) read(e.S);
        read(t);
    } else {
        // #4782: the pull a tracked read makes — the derive brought up to date
        // now and the reader lifted above it — not only a woken one: a reader
        // of the leaves carries no height from the derive (a leaf has no `_fn`),
        // so the heap can run it first, against the derive's last answer.
        let n = context;
        if (n?.Qe) n = n.Ke;
        if (n) pullComputed(t, n);
        untrack(() => read(t));
        // #4491: a derive that is a verdict reader only through what it reads (a
        // verdict memo's value, D-25 / F-6; no window of its own) is served the
        // committed value of a flight it reads (`observeFlight`, A15's stale
        // reader) and settles over it: its leaves are the screen, not the
        // flight's answer. A derived store reads like its memo (D-30): a tracked
        // reader of the leaves reads that flight as the memo's reader would, so
        // one that mounts mid-flight consumes unanswered async and is held (F-2)
        // instead of shown the old question's answer. The derive is that hold's
        // work until it answers (D-25): the read marks its next pass, which reads
        // plainly (`fam.w`, projection.ts), so the landing it computes is the one
        // the held reader shows. The mark is all it writes — the derive's posture
        // stays its passes' own (core `recompute`) — and a read that finds no
        // flight up clears it: a landing equal to the screen re-runs nothing, and
        // the mark does not outlive the flight. Only the verdict engine makes a
        // verdict reader (`observeFlightHook`: without it the branch folds away).
        // (The window flag is REACTIVE_WINDOW, spelled from EFFECT_RENDER: an
        // import of it keeps a dead copy in bundles without the store.)
                if (observeFlightHook && t.l & CONFIG_VERDICT && !(t.R & EFFECT_RENDER << 21) && tracking) {
            const n = e.rr;
            n.w = 0;
            for (let e = t.C; e; e = e.G) if (e.S.m & STATUS_PENDING) n.w = 1, read(e.S);
        }
    }
}

/** A projection's writes are its derive's: a staging made while the derive
 * is held (a flight up — its continuation writes after an `await`, a
 * callback's late write) belongs to the derive's transaction and reveals
 * with it, never drained early — and answers the derive's question: a quiet
 * re-ask's landing is quiet through its reveal (A19 exc. 2; the leaf's own
 * classification, as core reads it — `heldNotFinal`). Callers pass only the
 * derive's own writes (its draft or landing), never a user setter's
 * (`userWrite`): a user's is a proposal (A34 (3)'s discriminator). */ function holdWithDerive(e, t, 
// The lane the staging is work of: the pass's, or — a leaf born from a lane
// container's staging (`bornStaged`, #4451) — the container's.
n = passLane) {
    const r = e.rr?.node;
    // A pass that is a lane's work (a display-ahead read, F-6): its write is
    // the lane's derivation, as a memo's value would be (D-25) — the lane's
    // value, the committed value untouched beneath; the lane's end commits it.
        if (laneStageHook && n !== null && t.ue !== NOT_PENDING && !(t.l & CONFIG_OVERRIDE)) {
        list(t, n);
        t.l |= CONFIG_OVERRIDE;
        return;
    }
    if (r !== undefined && r !== null && r.l & CONFIG_HELD && t.ue !== NOT_PENDING && 
    // A node carrying a guess is the lane's: its staging is the truth beneath
    // the guess (`setSignal` → `laneWrite`), never re-homed by the store.
    !(t.l & (CONFIG_HELD | CONFIG_OVERRIDE))) {
        holdNode(t, txOf(r));
        t.T.Ve = r.T.Ve;
    }
}

/** A chained backing's child (§7b): a raw the inner family owns, or holds
 * at this key, is served as the inner store's wrapper — never a fresh
 * raw-keyed one — so the chained targets are the same objects across
 * settled and pending views. */ function resolveChainedRaw(e, t, n) {
    const r = e.v[$TARGET];
    if (r.ch) {
        const e = resolveChainedRaw(r, t, n);
        return e === n ? n : wrap(e, r, t);
    }
    const o = lookupTarget(n, r.rr);
    if (o !== undefined) return o.px;
    if ((r.v[t] === n || r.pb?.[t] === n) && isWrappable(n)) return wrap(n, r, t);
    return n;
}

/** The backing a structural read serves: a tracked reader subscribes to the
 * container and reads the frame core serves it — committed or staging by
 * the hold rules (`read()` on the node: the node's value IS the backing);
 * anyone else reads by `readSource`. */ function enumerationSource(e) {
    if (!inDraft(e) && !writeOverride && getObserver() !== null) {
        const t = read(getContainerNode(e));
        if (optHooks !== null && e.o) {
            const t = optHooks.frame(e);
            if (t !== NOT_PENDING) return t;
        }
        return t;
    }
    const t = readSource(e);
    return optRead(e) ? optHooks.view(e, t) : t;
}

/** A read on an optimistic family that composes the lanes' values: not the
 * draft's (it composed already), not a derive's write-override read (the
 * derive composes on the truth). */ function optRead(e) {
    return optHooks !== null && e.o && !inDraft(e) && !writeOverride;
}

/** Pollution keys are never served from the prototype (core R30). */ const UNSAFE_KEYS = new Set([ "__proto__", "prototype", "constructor" ]);

// ---------------------------------------------------------------------------
// traps
/** A trap write: the target is told at the setter's exit, the key recorded
 * in this setter's `wk`. A setter's first write to a target still staged by
 * an earlier setter of the batch opens a set of its own (the notify visits
 * it, D-8) and chains the earlier one behind for the fold's union. */ function touch(e, t) {
    let n = e.wk;
    if (!pendingNotify.has(e)) {
        pendingNotify.add(e);
        if (n !== null && n !== WK_ALL) {
            n = new Set;
            n.u = e.wk;
            e.wk = n;
        }
    }
    if (n !== WK_ALL) (e.wk ??= new Set).add(t);
}

const traps = {
    get(e, t, n) {
        const r = this;
        // One typeof gates every brand-symbol compare off the hot string path.
                if (typeof t !== "string") {
            if (t === $TARGET) return r;
            if (t === $PROXY) return n;
            if (t === $OWNER) return undefined;
 // ownership stamp: never a user key
                        if (t === $RECORD) return undefined;
 // a store is no view (see `viewOf`)
            // refresh()/isPending resolve the projection computed through $REFRESH.
                        if (t === $REFRESH) return r.rr?.node ?? undefined;
            if (t === $TRACK) {
                if (r.rr !== null) pullFamily(r);
                if (!inDraft(r) && getObserver() !== null) {
                    read(getContainerNode(r));
                    // Link the optimistic carrier the same way ownKeys/`enumerationSource`
                    // does. Without this, a $TRACK-only reader never sees structural
                    // guesses (push/splice/reorder) and is not re-run on revert (#5007).
                                        if (optHooks !== null && r.o && !writeOverride) optHooks.frame(r);
                    // Structural chaining (§7b): a chained backing's $TRACK reads
                    // through to the INNER store's container — structural
                    // notifications land on the source's own node.
                                        const e = readSource(r);
                    if (e[$TARGET] !== undefined) e[$TRACK];
                }
                return undefined;
            }
            // user symbols fall through to the generic path
                }
        // (The witness before the pull: a mark on an uninitialized derived
        // store is witnessed, then the pull throws — loading, declared pending.)
                if (verdict !== null && affectsHooks !== null && getObserver() === null) affectsHooks.io(r, t);
        if (r.rr !== null) pullFamily(r);
        // S2: a key a live guess changed reads through the target's carrier,
        // tracked — the lane's seat, a verdict window's judgment of the guess
        // (A17/A24): the frame core serves this reader, indexed. The key's own
        // leaf is silent while the guess covers it (optimistic.ts).
        // S2: an optimistic family's target (one field test on a plain store's
        // read path; the arms are the optimistic hooks').
        // `optHooks !== null` first: a bundle without optimistic stores never
        // assigns the hook, and the minifier folds the whole arm away.
        // In the user's draft: a read of another action's live guess (#4788).
                if (optHooks !== null && r.o && !writeOverride) {
            if (inDraft(r)) optHooks.drafted(r, t); else {
                const e = optHooks.read(r, t);
                if (e !== NOT_PENDING) return e;
            }
        }
        // Hot inline case: existing PLAIN node (non-accessor) read outside any
        // draft — the dbmon/uibench effect re-read shape. Core's `read()` serves
        // it (linking a tracked reader; the hold rules either way). Skips
        // serveDataKey's frame, the FORCE compare (only accessor keys ever hold
        // the sentinel), and isWrappable for primitives. ONE node-map lookup
        // serves this block and the accessor probe below. Before the backing is
        // consulted: a deleted key's node serves `undefined` (notifyWrites). A
        // chained target's node is a subscription point and reads through below,
        // a lane value on it included: the derive's lane pass that wrote it
        // predates the chain, and serving it covered the inner store's truth
        // (#4762: `length` and a row of the old list beside the new list's
        // rows). An optimistic target's guesses are its carrier's (above).
                const o = r.n?.[t];
        if (writeScopes === null && !writeOverride) {
            const e = o;
            if (e !== undefined && e.acc !== true && !r.ch) {
                const n = nodeValue(e, undefined, r);
                if (n === null || typeof n !== "object") return n;
                if (r.s) return serveShallow(r, t, n);
                if (e.pxv === n) return e.px;
                if (isWrappable(n)) {
                    const o = wrap(n, r, t);
                    e.px = o;
                    e.pxv = n;
                    return o;
                }
                return n;
            }
        }
        const i = readSource(r, t);
        // Overlay delete (#3044): a prototype overlay cannot shadow a delete, so
        // deleted keys are tracked aside and read as absent in the pending view.
                if (i === r.pb && r.del?.has(t)) {
            if (!inDraft(r) && getObserver() !== null) read(getNode(r, t));
            return undefined;
        }
        // Accessor keys serve through Reflect.get with the PROXY receiver
        // (R20/R29: internal reads track; the node is linked for shape-change
        // notification but its value is never served). Accessor-ness comes from
        // the node's cached flag; the first TRACKED read (which creates the
        // node) probes once — untracked node-less reads take the plain path,
        // where a raw-receiver getter still returns correct committed values.
        // Tracking suppression is PER-TARGET (inDraft), never global (#3037).
                let l = -1;
        {
            let e;
            if (o !== undefined) e = o.acc === true; else if (!inDraft(r) && getObserver() !== null) {
                e = isAccessorKey(i, t);
                if (i === (r.pb ?? r.v)) l = e ? 1 : 0;
            } else e = false;
            if (e) {
                if (!inDraft(r) && getObserver() !== null) read(o ?? getNode(r, t, l));
                const e = Reflect.get(i, t, n);
                if (r.s) return serveShallow(r, t, e);
                return isWrappable(e) ? draftServe(r, wrap(e, r, t)) : e;
            }
        }
        // Plain-data fast path: no descriptor allocation per read. Inherited
        // pollution keys are never served (core R30) — checked before the
        // proto-function branch can leak `constructor`. An inherited
        // `constructor` is withheld from drafts only: a plain read answers the
        // source's class (`s.o.constructor === Obj`, #4670). Overlay pending backings
        // chain to the committed backing, so "own in the view" means own on
        // either layer — a genuine prototype method is one that is own on
        // NEITHER.
                const f = r.ovl && i === r.pb;
        if ((t === "constructor" ? writeOverride || inDraft(r) : t === "__proto__" || t === "prototype") && !hasOwn.call(i, t) && !(f && hasOwn.call(r.v, t))) return undefined;
        let u = i[t];
        if (u === undefined ? !hasOwn.call(i, t) && !(f && hasOwn.call(r.v, t)) : false) {
            // Inherited: prototype getters/methods run with the proxy receiver.
            u = Reflect.get(i, t, n);
            if (typeof u === "function") return u;
 // proto methods untracked
            // Read-through (§7b): the inner store answers an absent key too — the
            // outer's node is a subscription point unless it carries a guess.
                        if (r.ch && i === r.v) return serveDataKey(r, t, u, i, o, l);
            // Reading a currently-absent own key subscribes to it (R12) — for any
            // target OUTSIDE its own draft scope, even mid-setter (#3037, above).
                        if (u === undefined && !inDraft(r)) {
                // (`nodeValue` reads — and links — the node it creates.)
                if (getObserver() !== null) getNode(r, t, l);
                const e = r.n?.[t];
                if (e) {
                    const n = nodeValue(e, undefined, r);
                    if (r.s) return serveShallow(r, t, n);
                    return isWrappable(n) ? draftServe(r, wrap(n, r, t)) : n;
                }
            }
            if (r.s) return serveShallow(r, t, u);
            return isWrappable(u) ? draftServe(r, wrap(u, r, t)) : u;
        }
        if (typeof u === "function" && !hasOwn.call(i, t) && !(f && hasOwn.call(r.v, t))) return u;
 // proto method
                return serveDataKey(r, t, u, i, o, l);
    },
    has(e, t) {
        const n = this;
        if (t === $TARGET || t === $PROXY || t === $TRACK) return true;
        if (t === $OWNER || t === $RECORD) return false;
        // (The witness before the pull: a mark on an uninitialized derived
        // store is witnessed, then the pull throws — loading, declared pending.)
                if (verdict !== null && affectsHooks !== null && getObserver() === null) affectsHooks.io(n, t);
        if (n.rr !== null) pullFamily(n);
        // In the user's draft: presence a live guess changed (#4812).
                if (optHooks !== null && n.o && !writeOverride && inDraft(n)) optHooks.drafted(n, t, true);
        // S2: presence of a key a live guess changed is the carrier frame's.
                if (optRead(n)) {
            const e = optHooks.has(n, t);
            if (e !== undefined) return e;
        }
        const r = readSource(n, t);
        // A tracked reader's presence node answers (born from the two frames;
        // core decides which it sees); everyone else reads the backing. A
        // chained target's node is a subscription point (§7b): the inner store
        // answers through the backing — unless the outer node carries a guess.
                if (!inDraft(n) && !writeOverride) {
            if (getObserver() !== null) {
                const e = getHasNode(n, t);
                const o = read(e);
                if (!n.ch || r !== n.v) return !!o;
                const i = t in r;
                e.le = i;
 // the link follows the inner store (S4)
                                return i;
            }
        }
        return t in r && !(r === n.pb && n.del?.has(t));
    },
    ownKeys() {
        const e = this;
        if (verdict !== null && affectsHooks !== null && getObserver() === null) affectsHooks.io(e, undefined);
        if (e.rr !== null) pullFamily(e);
        if (optHooks !== null && e.o && !writeOverride && inDraft(e)) optHooks.drafted(e, undefined, true);
        return visibleKeys(e, enumerationSource(e));
    },
    getOwnPropertyDescriptor(e, t) {
        const n = this;
        if (t === $OWNER || t === $RECORD) return undefined;
        const r = getObserver();
        if (verdict !== null && affectsHooks !== null && r === null) affectsHooks.io(n, t);
        if (n.rr !== null) pullFamily(n);
        // An enumerator (spread, Object.entries) already holds the container
        // node and reads its frame; a descriptor read on its own tracks
        // presence (R13).
                const o = r !== null && !inDraft(n) && !writeOverride;
        const i = o && observerHoldsContainer(n, r);
        // S2: a key a live guess changed is described from the carrier's frame —
        // before the presence node answers (silent while the guess covers it).
                if (!i && optRead(n)) {
            const r = optHooks.descriptor(n, t, Array.isArray(e));
            if (r !== undefined) return r === null ? undefined : r;
        }
        let l;
        if (i) l = enumerationSource(n); else if (o) {
            // The presence node's frame (core decides which) is the one described
            // (a chained target's inner store answers through the backing, §7b);
            // the descriptor itself comes from the backing the container's frame
            // serves this reader (`readSource`: a held container's changed key —
            // enumerability, accessor-ness — reads committed for a stale reader,
            // the staging for a member; #3706 contrast).
            const e = getHasNode(n, t);
            const r = read(e);
            l = readSource(n, t, true);
            if (!r && (!n.ch || !(t in l))) return undefined;
        } else l = readSource(n, t, true);
        let f;
        f ??= visibleDescriptor(n, l, t);
        if (f === undefined) return undefined;
        // The shell's `length` is writable and non-configurable, and a proxy must
        // report it that way (a frozen source array's is non-writable).
                if (t === "length" && Array.isArray(e)) f.writable = true; else f.configurable = true;
        return f;
    },
    set(e, t, n) {
        const r = this;
        const o = inDraft(r);
        if (!o && !writeOverride) {
            return true;
        }
        if (t === "__proto__") return true;
 // pollution guard (core R30)
        // A derived store's draft keeps a proxy of another store (a plain store
        // or another family; its own drafts still unwrap), and the key chains to
        // it (§7b read-through; CS-R3, CS-R5) as the return form's backing does.
        // Unwrapped, the family re-wrapped a raw that the source store's next
        // write left behind (#4845). A plain store's raw dedupes to the source's
        // own node (CS-R2), so plain stores unwrap as before.
                const i = r.s || r.rr && n?.[$TARGET]?.rr !== r.rr ? n : unwrapValue(n);
        const l = ensurePB(r);
        // #4146 (R1: node existence is unobservable; #3706: the hold's unit is
        // the key): a user setter writing a key a HELD container's staging left
        // unchanged would write it into the held staging — read as the hold's
        // until its landing, while the same key with a leaf (a live reader)
        // publishes at the flush. The key gets its leaf first, born from the two
        // frames (`bornStaged`: a key the hold left unchanged — or added — is
        // unstaged and held by no one; a key the hold changed is born held, A34
        // (1)), and the write stages on it (A28; a write by the holder joins it,
        // A34) — as it would with a reader's leaf. Kept while the container is
        // held (`deferredReleases`).
                if (userWrite && r.k?.l & CONFIG_HELD) deferredReleases.add(getNode(r, t));
        if (Array.isArray(l)) {
            touch(r, t);
            if (t === "length") r.wk = WK_ALL; else touch(r, "length");
        } else {
            touch(r, t);
            if (!(t in l)) r.kc++;
        }
        const f = UNSAFE_KEYS.has(t);
        if (f || r.ovl && r.sc !== 2 && !hasOwn.call(l, t)) {
            Object.defineProperty(l, t, {
                value: i,
                writable: true,
                enumerable: true,
                configurable: true
            });
        } else l[t] = i;
        r.del?.delete(t);
        if (f) return true;
        if (r.s && i !== null && typeof i === "object") markRawOne(i);
        if (!o) notifyWrites(r);
 // a write-override write
                return true;
    },
    defineProperty(e, t, n) {
        const r = this;
        const o = inDraft(r);
        if (!o && !writeOverride) {
            return true;
        }
        if (t === "__proto__") return true;
        if (n.get || n.set) r.a = true;
        if ("value" in n) n = {
            ...n,
            value: unwrapValue(n.value)
        };
        const i = ensurePB(r);
        if (r.a || !(n.enumerable && n.writable && n.configurable)) r.sc = 1;
        touch(r, t);
        Object.defineProperty(i, t, n);
        r.del?.delete(t);
        if (!o) notifyWrites(r);
 // a write-override write
                return true;
    },
    deleteProperty(e, t) {
        const n = this;
        const r = inDraft(n);
        if (!r && !writeOverride) {
            return true;
        }
        const o = ensurePB(n);
        touch(n, t);
        // The key count (`kc`, the overlay gate and the rebuild cap) follows a
        // delete the way it follows an add: an own key of the draft leaving — a
        // clone's key, or an overlay's own add — is one fewer now. An overlay's
        // override of a committed key is the exception: the committed key's
        // deletion is recorded in `del` and counted at the rebuild, as before.
        // Without the decrement the natural "delete the old key, set the new
        // one" shape drifted the count up by one per write until a three-key
        // object crossed the overlay threshold and paid a rebuild on every
        // commit after.
                const i = n.ovl && hasOwn.call(n.v, t);
        if (hasOwn.call(o, t)) {
            delete o[t];
            i || n.kc--;
        }
        if (i) (n.del ??= new Set).add(t);
        if (!r) notifyWrites(n);
 // a write-override write
                return true;
    },
    // The shell is empty: a store cannot be frozen or re-prototyped. It stays
    // extensible, so the source's prototype may be reported (#4670: instanceof).
    getPrototypeOf() {
        return protoOf(this.v);
    },
    preventExtensions: () => false,
    setPrototypeOf: () => false
};

TargetShape.prototype = traps;

/** Is `obs` subscribed to the container BY THIS PASS (its enumerator holds
 * it)? A link left from a previous pass is stale — the reader must subscribe
 * to the key's presence node or a later add never re-runs it. */ function observerHoldsContainer(e, t) {
    const n = e.k;
    if (n === null) return false;
    const r = n.Ze;
    return r !== null && r.u === t && (!(t.R & REACTIVE_RECOMPUTING_DEPS) || r.Ne === t.de);
}

function storeSetter(e, t, n = true) {
    if (writeHook !== null) writeHook(e);
    const r = e[$TARGET];
    const o = writeScopes;
    writeScopes = new Set;
    writeScopes.add(scopeKey(r));
    const i = userWrite;
    userWrite = n;
    // An outermost setter's exit notifies the targets it wrote. A
    // write-override write (a derive's continuation after an `await`, no
    // setter open) is notified at once by its trap but stays listed (its keys
    // share one written set while the continuation writes): dropped here, or
    // this setter's exit would notify the derive's staging as its own — a
    // user's, on an optimistic family, as guesses (#4484). Read before the
    // clear: the list is empty here but for those, and clearing an empty set
    // allocates.
        if (writing === 0 && pendingNotify.size) pendingNotify.clear();
    writing++;
    let l;
    try {
        // No untrack: the writing flag already disables store-node linking
        // (draft reads never self-track, proj R2), while EXTERNAL reads (signals
        // inside a projection derive) must keep tracking — they are the derive's
        // dependencies.
        l = t(e);
        // A returned replacement on an optimistic family is a user's edit, and
        // its slot is the point of write (S-59): written into the draft as the
        // keyed diff — a same-key row recurses into its own draft — so it makes
        // the writes the field edit would, and the exit below notifies them.
                if (optHooks !== null && n && r.o && !writeOverride && l !== undefined && l !== e && isWrappable(l)) {
            optHooks.reconcile(e, unwrapValue(l), r.rr?.key ?? null);
            l = undefined;
        }
    } finally {
        writing--;
        writeScopes = o;
        // Outermost setter exit: emit write-time notifications (one node write
        // per changed observed key) so holds and lanes engage now.
                if (writing === 0 && pendingNotify.size) {
            if (draftReadTx !== null) joinFuture(draftReadTx);
            const e = [ ...pendingNotify ];
            pendingNotify.clear();
            for (const t of e) notifyWrites(t);
            // A direct commit (a projection's creation run) folds at the setter's
            // exit, unqueued: the backing is the pre-batch one (nothing swapped).
                        if (directCommit) for (const t of e) if (t.pb !== null) foldTarget(t, t.v);
        }
        userWrite = i;
        if (writing === 0) draftReadTx = null;
    }
    if (l !== undefined && l !== e && isWrappable(l)) {
        // A returned replacement off the optimistic path is adopted as the
        // incoming truth.
        adoptPB(r, unwrapValue(l));
    }
    // A34 (3), #3612 — the derived store's twin of `setMemo`: a user write to
    // a derivation another transaction holds is nobody's proposal; it becomes
    // the draft's prior state and the hold re-derives over it (the writes
    // joined the hold through `setSignal`; the derive re-runs under it).
        if (n && heldDerivationHit) {
        heldDerivationHit = false;
        const e = r.rr.node;
        if (!(e.R & REACTIVE_DIRTY)) {
            e.R = e.R & ~REACTIVE_CHECK | REACTIVE_DIRTY;
            insertIntoHeap(e, dirtyQueue);
            schedule();
        }
    }
}

function createStore(e, t) {
    const n = t?.shallow === true;
    const r = wrap(e);
    // Dependency edges (store/edges.ts): the root is named like a plain signal.
        if (signalHook !== null) signalHook(r[$TARGET]);
    if (n) {
        r[$TARGET].s = true;
        markRawIngest(e);
    }
    const setter = e => storeSetter(r, e);
    return [ r, setter ];
}

/** The list driver's slot view of a shallow store (types.ts `shallowView`):
 * one `readSource` for the container instead of a trap per slot. A shallow
 * store's slots hold raw records, so the backing the container read serves
 * is what each slot read would — except where a slot's own leaf can answer
 * differently: an optimistic family's guesses, a chained backing, accessor
 * keys, or a read inside a draft or write scope. Those keep the trap. */ installShallowView(e => {
    // Any open write scope keeps the trap (it covers `inDraft`).
    if (e?.s && !(e.ch || e.a || writeScopes || writeOverride || e.rr?.opt)) return e.rr && pullFamily(e), 
    readSource(e);
});

/** True when `proxy` is a SHALLOW store (children served verbatim, slots
 * replaced by reference — #2932). The list driver uses this to choose the
 * slot-patch channel (collected row bodies) over per-record registration. */ function storeIsShallow(e) {
    const t = e?.[$TARGET];
    return t?.s === true;
}

/** True when `proxy` belongs to a projection family (S3). */ function storeHasFamily(e) {
    const t = e?.[$TARGET];
    return t?.rr != null;
}

/** True when `proxy` belongs to an OPTIMISTIC family (S4). */ function storeHasOptimisticFamily(e) {
    const t = e?.[$TARGET];
    return t?.o === true;
}

// ---------------------------------------------------------------------------
// enumeration views (one visibility rule for the traps AND the deep walk)
function visibleKeys(e, t) {
    let n;
    if (e.ovl && t === e.pb) {
        n = Reflect.ownKeys(e.v);
        const r = e.del;
        if (r?.size) n = n.filter(e => !r.has(e));
        for (const r of Reflect.ownKeys(t)) {
            if (!hasOwn.call(e.v, r)) n.push(r);
        }
    } else n = Reflect.ownKeys(t);
    // The ownership stamp is the last symbol on an owned backing: hide it.
        for (let e = n.length - 1; e >= 0 && typeof n[e] === "symbol"; e--) {
        if (n[e] === $OWNER) {
            n.splice(e, 1);
            break;
        }
    }
    return n;
}

function visibleDescriptor(e, t, n) {
    let r = Object.getOwnPropertyDescriptor(t, n);
    if (e.ovl && t === e.pb) {
        if (e.del?.has(n)) return undefined;
        if (r === undefined) r = Object.getOwnPropertyDescriptor(e.v, n);
    }
    return r;
}

// ---------------------------------------------------------------------------
// deep / snapshot
/** Tracking snapshot: subscribes every reachable record's container and deep
 * witness (one node per record, not one per path), then snapshots. */ function deep(e) {
    const t = e?.[$TARGET];
    if (t?.px !== e) return e;
    const n = new Set;
    const childTarget = (e, t, n) => {
        let r = lookupTarget(t, e.rr);
        if (r === undefined) {
            if (!isWrappable(t)) return undefined;
            wrap(t, e, n);
            r = lookupTarget(t, e.rr) ?? t[$TARGET];
        }
        return r;
    };
    const walkT = e => {
        if (e.rr !== null) pullFamily(e);
        const t = enumerationSource(e);
        if (n.has(t)) return;
        n.add(t);
        read(getDeepNode(e));
        // Through a chain (#3323): every inner record's container AND deep
        // witness — base writes bump the inner witnesses.
                for (let t = e; t.ch; ) {
            t = t.v[$TARGET];
            read(getContainerNode(t));
            read(getDeepNode(t));
        }
        for (const n of visibleKeys(e, t)) {
            const r = visibleDescriptor(e, t, n);
            if (r === undefined) continue;
            if (r.get || r.set) {
                e.a = true;
                continue;
 // accessors track through their own reads when invoked
                        }
            let o = r.value;
            if (o === null || typeof o !== "object") continue;
            if (e.ch && o[$TARGET] === undefined) o = resolveChainedRaw(e, n, o);
            const i = childTarget(e, o, n);
            if (i === undefined) continue;
 // raw-marked: leaf by contract
                        walkT(i);
        }
    };
    // One pass where the tracked view is the snapshot's backing; else two walks.
        const r = new Map;
    const fast = (e, t) => {
        if (e.rr !== null || e.ch || e.ovl || t !== e.px && lookupTarget(t, null) !== e) return;
        const o = enumerationSource(e);
        if (o !== snapshotBacking(e) || n.has(o) || r.has(o) || !isWrappable(o)) return;
        // snapshotWalk's resolution must stop at `src` too.
                if (o !== t) {
            const t = lookupTarget(o, null);
            if (t !== undefined && t !== e) return;
        }
        n.add(o);
        read(getDeepNode(e));
        return o;
    };
    const step = (e, t, n, o) => {
        const i = childTarget(e, t, n);
        const l = i && !o ? fast(i, t) : undefined;
        if (l) return snapshotRecord(l, r, null, i, step);
        if (i) walkT(i);
        if (!o) return snapshotWalk(t, r, null);
    };
    try {
        const n = fast(t, e);
        if (n) return snapshotRecord(n, r, null, t, step);
    } catch (e) {
        // A snapshot read threw mid-pass: subscribe the rest, as the two walks did.
        n.clear();
        walkT(t);
        throw e;
    }
    walkT(t);
    return snapshotWalk(e, r, t.rr);
}

/** Non-tracking snapshot: source identity for unowned subtrees (zero copy),
 * a copy for owned ones (they are by definition modified relative to the
 * source). Sees the pending backing (R27). */ function snapshot(e) {
    const t = e?.[$TARGET];
    return snapshotWalk(e, new Map, t?.rr ?? null);
}

/** R27: a snapshot sees the batch's pending backing — a held one (another
 * transaction's future) only from its own draft, or from a pass the
 * container node serves the future (its landing flush, a memo joining). */ function snapshotBacking(e) {
    let t = e.v;
    const n = e.pb;
    if (n !== null) {
        const r = e.k;
        if (inDraft(e) || writeOverride || r === null || !(r.l & CONFIG_HELD)) t = n; else if (stagingReader() !== null) t = untrack(() => read(r));
        // materializePB is a no-op (returns pb) without an overlay.
                if (t === n) t = materializePB(e);
    }
    return optRead(e) ? optHooks.view(e, t) : t;
}

function snapshotWalk(e, t, n) {
    if (e === null || typeof e !== "object") return e;
    let r = e;
    for (let e = true; ;e = false) {
        const t = r?.[$TARGET]?.v !== undefined;
        let o = t ? r[$TARGET] : undefined;
        if (o === undefined && n !== null) o = lookupTarget(r, n);
        if (o === undefined) o = lookupTarget(r, null);
        if (o === undefined) break;
        if (e && !t && n !== null && o.rr !== n) {
            const e = n.map.get(o.px);
            if (e !== undefined) o = e;
        }
        if (o.rr !== null) n = o.rr;
        const i = snapshotBacking(o);
        if (i === r) break;
        r = i;
    }
    if (!isWrappable(r)) return r;
    const o = t.get(r);
    return o !== undefined ? o : snapshotRecord(r, t, n, null, null);
}

/** `t`/`step` set: `deep()`'s one pass, which also tracks `t`'s children. */ function snapshotRecord(e, t, n, r, o) {
    if (isOwned(e)) {
        const i = Array.isArray(e);
        const l = i ? [] : Object.create(protoOf(e));
        t.set(e, l);
        for (const f of Reflect.ownKeys(e)) {
            if (i && f === "length" || f === $OWNER) continue;
            const u = Object.getOwnPropertyDescriptor(e, f);
            const s = u.value;
            const a = s !== null && typeof s === "object";
            if (typeof f === "symbol" && !u.enumerable) {
                if (o && a) o(r, s, f, true);
                continue;
            }
            if (u.get || u.set) {
                if (r) r.a = true;
                Object.defineProperty(l, f, u);
                continue;
            }
            const c = a ? o ? o(r, s, f) : snapshotWalk(s, t, n) : s;
            if (u.enumerable && u.writable && u.configurable) l[f] = c; else Object.defineProperty(l, f, {
                ...u,
                value: c
            });
        }
        if (i && l.length !== e.length) l.length = e.length;
        return l;
    }
    t.set(e, e);
    let i = null;
    for (const l of Reflect.ownKeys(e)) {
        const f = Object.getOwnPropertyDescriptor(e, l);
        if (!f) continue;
        if (f.get || f.set) {
            if (r) r.a = true;
            continue;
        }
        const u = f.value;
        if (u === null || typeof u !== "object") continue;
        const s = o ? o(r, u, l) : snapshotWalk(u, t, n);
        if (s !== u) {
            if (i === null) {
                i = Array.isArray(e) ? [ ...e ] : Object.create(protoOf(e), Object.getOwnPropertyDescriptors(e));
                t.set(e, i);
            }
            i[l] = s;
        }
    }
    return i ?? e;
}

export { $PROXY, $TARGET, $TRACK, adoptPB, arrayStructureChanged, bumpDeep, cloneRaw, committed, containerEquals, createStore, deep, deepUsed, getContainerNode, getHasNode, getNode, hasAccessorFlag, linkedTo, materializePB, membershipChanged, notifyFold, notifyFoldTail, notifyKeyDiff, notifyKeyValue, protoOf, resolveChainedRaw, runDirect, sameKey, sameRow, serveKeyValue, snapshot, storeHasFamily, storeHasOptimisticFamily, storeIsShallow, storeSetter, targetsEqual, unwrapValue, userWriting, wrap };