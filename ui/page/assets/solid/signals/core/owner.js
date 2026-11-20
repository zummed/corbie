import { defaultContext, CONFIG_TRANSPARENT, REACTIVE_NONE, REACTIVE_DISPOSED, CONFIG_AUTO_DISPOSE, STATUS_PENDING, REACTIVE_FRAME_READ, REACTIVE_ZOMBIE } from "./constants.js";

import { context, runWithOwner, tracking } from "./core.js";

import { clearInFlight } from "./async.js";

import { unobserved, clearDeps } from "./graph.js";

import { deleteFromHeap } from "./heap.js";

import { globalQueue, pendingNodes, schedule, dirtyQueue } from "./scheduler.js";

// L2 (step 2): the parked frame. A pass parks the previous frame's children
// and `_disposal` list on `_x._pendingFirstChild` / `_x._pendingDisposal`
// (`parkChildren`, core.ts); they keep reacting until the node's commit
// disposes them (`zombie` walk below) or the owner dies. `markDisposal` flags
// the parked subtree REACTIVE_ZOMBIE: the splice guard and the reawaken path
// key off it, and the heap defers a zombie's recompute past the seam (no
// second heap — the deferral list in scheduler.ts).
function markDisposal(e) {
    let n = e.pe;
    while (n) {
        n.R |= REACTIVE_ZOMBIE;
        markDisposal(n);
        n = n.Ge;
    }
}

function dispose(e) {
    // Direct disposal is death, not dormancy: strip the observation lifecycle
    // so a later read freezes at the last committed value instead of
    // reawakening the node (#3024). The teardown itself (heap removal — a node
    // left queued would be recomputed and resurrected by the next flush (#2983)
    // — dep unlinking, child disposal) is exactly unobserved()'s body; only
    // this flag distinguishes death from dormancy.
    e.l &= ~CONFIG_AUTO_DISPOSE;
    unobserved(e);
}

// The first error a cleanup threw, boxed so a thrown `undefined` still
// counts. A throwing cleanup stops neither its siblings nor the rest of the
// tree: every cleanup runs. The error is rethrown once the teardown is over:
// by a root's dispose call outside a flush (`disposeRootSelf`), otherwise when
// the flush ends (`flush`; `runCleanup` schedules one), so a flush is never
// left half-applied. A halt drops it: the error that halted is the one
// reported, and a stale one must not surface from a later, unrelated dispose.
let cleanupError;

/** Runs a cleanup or `unobserved` hook; a throw is collected, not raised. @internal */ function runCleanup(e) {
    try {
        e();
    } catch (e) {
        cleanupError ||= [ e ];
        schedule();
    }
}

/** Rethrows the error `runCleanup` collected, once; `drop` discards it. @internal */ function rethrowCleanupError(e) {
    const n = cleanupError;
    cleanupError = undefined;
    if (n && !e) throw n[0];
}

function disposeChildren(e, n = false, r) {
    const t = e.R;
    if (t & REACTIVE_DISPOSED) return;
    // A parked frame dies with its owner (#3024): the commit that would retire
    // it never comes once the node is DISPOSED, so it drains here or its
    // cleanups never run and it stays subscribed, rerunning in a torn-down
    // tree (#3561). Death only (`self`): a re-pass leaves the frame reacting
    // until the commit.
        if (n) {
        if (!r && e.T !== null && (e.T.me !== null || e.T.Le !== null)) disposeChildren(e, false, true);
        e.R = t | REACTIVE_DISPOSED;
        if (e.X) {
            clearInFlight(e);
            // A death that can unblock the future (L2, `futureBlocked`): a held
            // flight's — a transaction's or a lane's (its `_x._transaction`) — or a
            // stale reader's (REACTIVE_FRAME_READ — a frame that derived from one).
            // An idle pending queue would also pin it until an unrelated flush.
            // Nothing else re-examines a dead node, so make sure a seam comes.
                        if (e.m & STATUS_PENDING && e.T?.Re != null || t & REACTIVE_FRAME_READ || pendingNodes.length) schedule();
        }
    }
    // Typed as a computed so the loop needs no cast alias (the minifier keeps one).
        let l = r ? e.T.me : e.pe;
    if (r) e.T.me = null; else e.pe = null;
    while (l) {
        // Owner teardown is death regardless of the child's own lifecycle
        // (#3024): strip AUTO_DISPOSE so a post-disposal read freezes at the
        // last committed value instead of reawakening in a torn-down tree.
        // Runs before the recursion so already-dormant children (whose
        // disposeChildren call early-returns on REACTIVE_DISPOSED) die too.
        // Only unobserved()'s own node keeps its dormancy — it is never in
        // this loop; its children are rebuilt fresh on reawaken.
        l.l &= ~CONFIG_AUTO_DISPOSE;
        // Heap removal must not be gated on `_deps`: a dependency-free
        // computation queued by refresh() has a null dep list but still sits in
        // the dirty heap, and left there the post-disposal flush recomputes it —
        // recompute() rewriting `_flags` clears REACTIVE_DISPOSED and the node
        // comes back to life (post-unmount runs, leaked cleanups, #2983).
        // deleteFromHeap self-guards on the in-heap flags (and tolerates plain
        // Owners, whose _flags is undefined), so no gate here.
                deleteFromHeap(l, dirtyQueue);
        clearDeps(l);
        // The chain is detached above so a node a cleanup links mid-drain lands
        // on the fresh head and survives. Pointing each drained child's prev at
        // itself routes its later splice onto the detached chain, never the head,
        // and keeps the dev owner-chain-head invariant honest for those children.
                l.Me = l;
        disposeChildren(l, true);
        // Read after, not before: a sibling this disposal made dormant spliced
        // itself out of the detached chain, and a cleanup may then have linked
        // it at the fresh head, which rewrote the `_nextSibling` a pre-read
        // would still be holding.
                l = l.Ge;
    }
    if (!r) e.be = 0;
    // O(1) splice out of parent's chain on individual dispose. Skipped during
    // batch dispose (parent already disposed) and for a zombie (it sits on the
    // parent's parked frame, drained whole by the commit). We leave
    // node._nextSibling intact so outer walks that already advanced past us
    // still reach later siblings.
        if (n && !r && !(t & REACTIVE_ZOMBIE) && e._parent !== null && !(e._parent.R & REACTIVE_DISPOSED)) {
        const n = e.Me;
        const r = e.Ge;
        if (n !== null) n.Ge = r; else e._parent.pe = r;
        if (r !== null) r.Me = n;
        e.Me = null;
    }
    runDisposal(e, r);
    // Final effect-returned cleanup fires at true disposal, after `_disposal`
    // to mirror rerun ordering (compute-phase teardown first, cleanup last).
        if (n && e.ze) {
        const n = e.ze;
        e.ze = undefined;
        runCleanup(n);
    }
}

function linkChild(e, n) {
    const r = e.pe;
    n.Me = null;
    n.Ge = r;
    if (r !== null) r.Me = n;
    e.pe = n;
}

function runDisposal(e, n) {
    // Detach the list BEFORE running it (#3601), as `_cleanup` is (#2813). A
    // cleanup that disposes an ancestor re-enters this node through the death
    // walk while the loop is still running; with the list still attached, that
    // walk ran every entry a second time. Detached, the re-entrant drain finds
    // nothing. The same shape latched a throwing cleanup: the list survived the
    // throw and every later drain re-ran and re-threw it.
    const r = n ? e.T.Le : e.ke;
    if (!r) return;
    if (n) e.T.Le = null; else e.ke = null;
    if (Array.isArray(r)) {
        // Unwind order (#3572, restores 1.x #1562): later registrations run
        // before earlier ones. Children have already been disposed by the caller,
        // so with LIFO a body that registers cleanup before creating its children
        // tears down after them — the same order a per-component owner gives.
        for (let e = r.length - 1; e >= 0; e--) {
            runCleanup(r[e]);
        }
    } else {
        runCleanup(r);
    }
}

function childId(e, n) {
    while (e.l & CONFIG_TRANSPARENT && e._parent) e = e._parent;
    if (e.id != null) return formatId(e.id, n ? e.be++ : e.be);
    throw new Error;
}

/**
 * Allocates and returns the next stable child id for `owner`. Used by
 * hydration plumbing and `createUniqueId`. Not part of the user-facing API.
 *
 * @internal
 */ function getNextChildId(e) {
    return childId(e, true);
}

/**
 * The id a freshly-created node inherits: an explicit `options.id` wins;
 * transparent nodes share their parent's id; otherwise the parent's next
 * child id is consumed (or `undefined` outside an id-carrying tree).
 */ function inheritId(e, n, r) {
    return e?.id ?? (n ? r?.id : r?.id != null ? getNextChildId(r) : undefined);
}

/**
 * Returns the *next* child id for `owner` without consuming it. Used by
 * hydration plumbing to peek at the id a future child will receive.
 *
 * @internal
 */ function peekNextChildId(e) {
    return childId(e, false);
}

function formatId(e, n) {
    const r = n.toString(36), t = r.length - 1;
    return e + (t ? String.fromCharCode(64 + t) : "") + r;
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
 */ function getObserver() {
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
 */ function getOwner() {
    return context;
}

/**
 * Low-level: registers `fn` as a disposal callback on the current owner.
 * Most code should use `onCleanup()` from `solid-js`, which adds dev-mode
 * checks. `cleanup()` is the unchecked primitive used by internals; `owner`
 * (internal) targets a specific owner's disposal list instead of the ambient
 * one.
 */ function cleanup(e, n = context) {
    if (!n) return e;
    if (!n.ke) n.ke = e; else if (Array.isArray(n.ke)) n.ke.push(e); else n.ke = [ n.ke, e ];
    return e;
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
 */ function isDisposed(e) {
    return !!(e.R & REACTIVE_DISPOSED);
}

function disposeRootSelf(e = true) {
    disposeChildren(this, e);
    // Outside a flush the dispose call rethrows; inside one, the flush does.
    // Kept out of `disposeChildren`, whose hot path it slows.
        if (cleanupError && !globalQueue.Y) rethrowCleanupError();
}

/**
 * Creates a fresh owner attached as a child of the current owner (or as a
 * detached root if there is none). Used by framework internals to group
 * cleanups; app code should use `createRoot()` (host a reactive scope outside
 * a component) or `runWithOwner()` (re-enter a captured owner).
 *
 * @internal
 */ function createOwner(e) {
    const n = context;
    const r = e?.transparent ?? false;
    // Prod and observe boilerplates (see core.ts computed()). The observe
    // literal carries the `_name` and `_loc` slots the rendering layer fills
    // with the component label and JSX site (`owner._name = "<App>"`,
    // `owner._loc = "src/App.tsx:3:5"`) — slots, so labelling a root is a
    // plain store rather than a shape fork between labelled and plain roots.
        const t = {
        id: inheritId(e, r, n),
        l: r ? CONFIG_TRANSPARENT : 0,
        // Present from birth: disposeChildren marks death here on roots too, and
        // a first store of a missing property is a shape transition plus a
        // properties-store allocation per root — on a 100×4 table teardown that
        // was 12 KB and most of disposal's time.
        R: REACTIVE_NONE,
        Qe: true,
        Ke: n?.Qe ? n.Ke : n,
        pe: null,
        Ge: null,
        Me: null,
        ke: null,
        A: n?.A || defaultContext,
        be: 0,
        T: null,
        _parent: n,
        dispose: disposeRootSelf
    };
    if (n) linkChild(n, t);
    return t;
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
 */ function createRoot(e, n) {
    const r = createOwner(n);
    // A bound function, not an arrow: an arrow would share this scope, which
    // holds `init`, so keeping `dispose` would keep `init` and all it closes
    // over alive (#4095).
        return runWithOwner(r, () => e(disposeRootSelf.bind(r, true)));
}

export { cleanup, createOwner, createRoot, dispose, disposeChildren, getNextChildId, getObserver, getOwner, inheritId, isDisposed, linkChild, markDisposal, peekNextChildId, rethrowCleanupError, runCleanup };