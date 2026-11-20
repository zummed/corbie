import { GlobalQueue, releaseMarksHook } from "../core/scheduler.js";

import { optHooks, installAffectsHooks } from "./hooks.js";

import { lookupTarget, $OWNER } from "./target.js";

import { $TARGET } from "./types.js";

/**
 * Store — the store half of `affects()` (plan sec. 29, plan sec. 37; A24 (4), #2882,
 * #2893, #2904).
 *
 * A mark on a store is a mark on nodes (affects.ts: a count on the node, a
 * dependency walk at probe time): `affects(record, key)` marks the slot's
 * leaf; `affects(record)` marks a carrier (the record's `$AFFECTS` leaf)
 * and every live node in the record's subtree — the edges existing readers
 * subscribed through. Coverage is by RAW IDENTITY, not read path (#2882: a
 * captured row proxy, a derived store sharing the source's raw, never read
 * through the declaration's proxy): the identities reachable at the
 * declaration are the mark's `scope`, so a node born inside the window on
 * a covered record inherits the mark (released with the carrier's last),
 * and an untracked probe through a record with no nodes is witnessed into
 * the verdict. A mark declared on a draft (inside a setter) or over an
 * optimistic family walks the view the writer sees.
 *
 * Imported by `affects()`, which installs it on its first store target
 * (#3891, after #3912): a program with stores that never declares a mark
 * carries none of it. It never imports store.ts — the engine's top level —
 * so a program declaring marks without stores carries this module alone:
 * the store's nodes and its wrappability test come through `GlobalQueue`
 * (`_storeNode`, `_storeWrappable`), and node birth and the untracked traps
 * reach back through `affectsHooks` (hooks.ts).
 */
/** The keyless mark's carrier key: a leaf of the record no read serves. */ const $AFFECTS = Symbol(0);

const scopes = new Map;

function scopeOf(e, o) {
    let n = scopes.get(e);
    if (n === undefined) scopes.set(e, n = {
        oo: new Set,
        no: [],
        Xe: o
    });
    return n;
}

/** The nodes an `affects(store[, key])` declaration marks (affects.ts
 * registers each). */ function storeMarks(e, o) {
    const n = GlobalQueue.so(e, o === undefined ? $AFFECTS : o);
    const t = scopeOf(n, o);
    if (o === undefined) {
        const o = [ n ];
        walk(e.px, t, o, e.rr, new Set);
        return o;
    }
    t.oo.add(e.v);
    if (e.pb !== null) t.oo.add(e.pb);
    return [ n ];
}

/** The identities reachable from `value` into the scope (the draft's and
 * the committed backing both, when a draft is open; an optimistic family's
 * view), every live node under each record into `found`. Untracked by
 * construction: raws, never traps. */ function walk(e, o, n, t, f) {
    if (!GlobalQueue.fo(e)) return;
    let s = e[$TARGET];
    // Another store's proxy the record holds (§7b): the mark covers this
    // store's view of it — the chained target, keyed by that proxy — and never
    // that store's own records: a derived store's work cannot change its
    // input's data (#4914). Its children are walked as that store's proxies,
    // the identities this store's chained children are keyed by.
        if (s !== undefined && t !== null && s.rr !== t && s.px === e) {
        if (f.has(e)) return;
        f.add(e);
        o.oo.add(e);
        const r = lookupTarget(e, t);
        if (r !== undefined) nodesOf(r, n);
        const i = s.pb ?? s.v;
        for (const e of Reflect.ownKeys(i)) {
            const r = Object.getOwnPropertyDescriptor(i, e).value;
            const l = GlobalQueue.fo(r) ? lookupTarget(r, s.rr) : undefined;
            if (l !== undefined) walk(l.px, o, n, t, f);
        }
        return;
    }
    s ??= lookupTarget(e, t);
    let r = s !== undefined ? s.pb ?? s.v : e;
    if (f.has(r)) return;
    f.add(r);
    o.oo.add(r);
    if (s !== undefined) {
        if (s.pb !== null) o.oo.add(s.v);
        // The writer's view: the tick's own unflushed guesses are in motion too.
                if (optHooks !== null && s.o) r = optHooks.view(s, r, true);
        nodesOf(s, n);
        t = s.rr ?? t;
    }
    for (const e of Reflect.ownKeys(r)) if (e !== $OWNER) {
        const s = Object.getOwnPropertyDescriptor(r, e);
        if (s.get === undefined) walk(s.value, o, n, t, f);
    }
}

/** Every live node of `t` into `found`. */ function nodesOf(e, o) {
    const n = e.n;
    if (n !== null) for (const e of Reflect.ownKeys(n)) 
    // Another mark's carrier is its own channel: counting it here would
    // extend that scope's lifetime to this declaration's.
    if (e !== $AFFECTS) o.push(n[e]);
    const t = e.h;
    if (t !== null) for (const e of Reflect.ownKeys(t)) o.push(t[e]);
    if (e.k !== null) o.push(e.k);
    if (e.dk !== null) o.push(e.dk);
}

/** A live scope covering `t`'s identity (keyed: for `key`). Chained
 * backings (§7b): a view's backing is another store's proxy — marks cover
 * the BASE raw, so every identity along the chain is checked. */ function covering(e, o, n) {
    for (const [t, f] of scopes) {
        if (t === n || f.Xe !== undefined && f.Xe !== o) continue;
        for (let o = e.v; ;) {
            if (f.oo.has(o)) return t;
            const e = o?.[$TARGET];
            if (e === undefined) break;
            const n = e.pb ?? e.v;
            if (n === o) break;
            o = n;
        }
    }
    return null;
}

/** The store's side: node birth and the untracked traps call it (store.ts,
 * through `affectsHooks`) — with no live mark too: an empty scope map is
 * no work. */ const hooks = {
    // A node born on a record a live mark covers inherits it (released with
    // the carrier's last registration).
    ro(e, o, n) {
        if (scopes.size === 0 || n === $AFFECTS) return;
        const t = covering(e, n, o);
        if (!t) return;
        GlobalQueue.N(o);
        scopes.get(t).no.push(o);
    },
    // An untracked probe (`isPending(() => s.x)` with no observer) reading a
    // record a live mark covers: no node carries the mark for it — the
    // verdict is told directly.
    io(e, o) {
        if (e.n?.[$AFFECTS]?.T?.I || covering(e, o)) GlobalQueue.rt();
    }
};

/** `affects()` calls it before a store-targeted declaration (idempotent). */ function installStoreAffects() {
    installAffectsHooks(hooks);
}

/** The carrier's last mark released (affects.ts): its scope dies, and the
 * nodes that inherited the mark release theirs. */ function releaseMarkScope(e) {
    const o = scopes.get(e);
    if (o === undefined) return;
    scopes.delete(e);
    releaseMarksHook(o.no);
}

export { installStoreAffects, releaseMarkScope, storeMarks };