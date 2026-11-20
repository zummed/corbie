import { isEqual } from "../core/core.js";

import { isOwned, $OWNER, storeLookup, lookupTarget } from "./target.js";

import { userWriting, unwrapValue, adoptPB, sameKey, materializePB, notifyFold, sameRow, notifyKeyValue, notifyKeyDiff, notifyFoldTail, hasAccessorFlag, deepUsed, targetsEqual, bumpDeep } from "./store.js";

import { optHooks } from "./hooks.js";

import { $TARGET, markRawIngest, isWrappable, rawValuesUsed, isRawValue } from "./types.js";

/**
 * Store — reconcile: the immutable-diff adoption channel (INTERNALS §3).
 *
 * Reconcile never merge-writes into backing objects: it adopts `incoming`
 * as the backing at every proxied level (an internal pointer swap at the
 * fold — no clones, no user-object mutation), value-notifies only where
 * nodes exist, and descends into a changed child pair only where a target
 * with nodes or the sticky descendants flag exists below (§6d: diff the
 * listened path, not every cell). Reference-equal unowned pairs skip
 * outright (an owned backing is setter-diverged and must diff).
 *
 * Keyed arrays: the aligned prefix is walked positionally; from the first
 * misalignment rows match by key (surviving rows keep their proxy
 * identity), unkeyed items fall back to position.
 */ function reconcileState(e, n, t, o = false) {
    if (n == null) throw new Error("");
    const f = n[$TARGET];
    if (f?.px !== n) throw new Error("");
    const i = t === null ? null : typeof t === "string" ? e => e?.[t] : t;
    // A user's reconcile on an optimistic family (S4): not an adoption of
    // truth but an optimistic edit — the keyed diff is written into the draft
    // (a matched row keeps its proxy, its changed leaves become its guesses),
    // and the setter's exit turns the draft into guesses like any.
        if (optHooks !== null && f.o && userWriting()) {
        optHooks.reconcile(n, unwrapValue(e), i);
        return;
    }
    // Replace-mode root handed another store's proxy: chain to it (§7b).
        if (o && e !== n && e?.[$TARGET] !== undefined) {
        const n = f.pb ?? f.v;
        if (n === e) return;
 // already chained to this store
                adoptPB(f, e);
        return;
    }
    const l = unwrapValue(e);
    if (i) {
        const e = f.pb ?? f.v;
        const n = i(e);
        if (n !== undefined && !sameKey(i(l), n)) {
            if (!o) throw new Error("");
            // A different entity at the root: the old backing is disowned (wraps
            // fresh if re-handed) and the new one adopted whole.
                        if (isOwned(e)) delete e[$OWNER]; else (f.rr?.map ?? storeLookup).delete(e);
            adoptPB(f, l);
            return;
        }
    }
    applyAdopt(f, l, i, o);
}

/** Adopt `incoming` at `t` and notify its nodes — one leaf write per
 * changed observed key, the structural nodes once — descending into
 * changed child pairs that are proxied below. */ function applyAdopt(e, n, t, o = false) {
    // The diff base is the view the nodes were last told (#3296). A nested
    // overlay draft (`Object.create(v)`, #3044) reads a key the draft deleted
    // as present through its prototype; materialized first — `adoptPB` does
    // so anyway, after the base was taken — so a deletion the draft made is
    // a deletion here (#3743: the presence and value diffs both read `old`).
    const f = e.ovl ? materializePB(e) : e.pb ?? e.v;
    if (n === f && !isOwned(f)) return;
    // Q-D: a landing on an optimistic family is reconciled against the VIEW
    // — key matching from the arrangement the guesses made (R28), so a row
    // the guess added keeps its proxy when the server returns it.
    // A returned landing pairs its new objects with the row targets first, as
    // the derive's draft does (store.ts `notifyWrites`): a guessed row the view
    // no longer has (an answer that dropped it) keeps its target when the
    // answer brings its key back (Q-D, F-11, #4907). A row another store lent
    // is in the view already, and keeps its slot this store's (#4918).
        const i = optHooks !== null && e.o ? (optHooks.pair(e, n, true), optHooks.view(e, f)) : f;
    const l = Array.isArray(n);
    const s = e.s === true;
    adoptPB(e, n, false);
    if (s) markRawIngest(n);
    // The deep witness is bumped once per adoption (none to bump: done).
        let u = e.dk === null;
    // A live chained base (a store proxy, §7b) reads the inner store's
    // current frame, not what the nodes were last told — nothing to diff
    // against: the fold writes every node (#4361).
        if (Array.isArray(i) !== l || f[$TARGET]) {
        notifyFold(e, f, n);
        return;
    }
    if (l) {
        const l = e.n;
        let r = 0;
        if (t && !s) {
            const s = i.length;
            const a = n.length;
            let c = 0;
            // Aligned prefix: same row (by reference or key) at the same index.
                        for (const d = Math.min(s, a); c < d; c++) {
                const s = n[c];
                const a = i[c];
                if (a !== s && !sameRow(a, s, t)) break;
 // misaligned: fall to the keyed remainder below
                                if (s !== null && typeof s === "object" && (a !== s || isOwned(s))) descend(unwrapValue(a), s, t, o, e, c);
                if (deepUsed && !u && !(s !== null && typeof s === "object" ? targetsEqual(a, s) : isEqual(a, s))) {
                    bumpDeep(e);
                    u = true;
                }
                if (l !== null) {
                    const e = l[c];
                    if (e !== undefined) {
                        r++;
                        notifyKeyValue(e, c, f[c], s, f, n);
                    }
                }
            }
            if (deepUsed && !u && c < n.length) bumpDeep(e);
            const d = c;
 // misalignment point (== nlen on aligned ticks)
                        let p = null;
            for (;c < n.length; c++) {
                const s = n[c];
                if (s !== null && typeof s === "object") {
                    const n = t(s);
                    let f;
                    if (n !== undefined) {
                        if (p === null) {
                            p = new Map;
                            for (let e = d; e < i.length; e++) {
                                const n = unwrapValue(i[e]);
                                if (n !== null && typeof n === "object") {
                                    const o = t(n);
                                    if (o === undefined) continue;
                                    const f = p.get(o);
                                    if (f === undefined) p.set(o, e); else if (Array.isArray(f)) f.push(e); else p.set(o, [ f, e ]);
                                }
                            }
                        }
                        const e = p.get(n);
                        if (e === undefined) f = undefined; else if (Array.isArray(e)) {
                            f = unwrapValue(i[e.shift()]);
                            if (e.length === 1) p.set(n, e[0]);
                        } else {
                            f = unwrapValue(i[e]);
                            p.delete(n);
                        }
                    } else {
                        f = unwrapValue(i[c]);
 // keyless item: positional fallback
                                        }
                    descend(f, s, t, o, e, c);
                }
                if (l !== null) {
                    const e = l[c];
                    if (e !== undefined) {
                        r++;
                        notifyKeyDiff(e, c, f, n, false);
                    }
                }
            }
        } else {
            const a = Math.min(i.length, n.length);
            const c = n.length;
            for (let d = 0; d < c; d++) {
                const c = n[d];
                if (!s && d < a && c !== null && typeof c === "object") descend(unwrapValue(i[d]), c, t, o, e, d);
                if (deepUsed && !u && !(c !== null && typeof c === "object" ? targetsEqual(i[d], c) : isEqual(i[d], c))) {
                    bumpDeep(e);
                    u = true;
                }
                if (l !== null) {
                    const e = l[d];
                    if (e !== undefined) {
                        r++;
                        notifyKeyDiff(e, d, f, n, false);
                    }
                }
            }
        }
        // Indices past the new length (and `length` itself) still have nodes.
                if (l !== null && r < e.nc) {
            for (const e of Reflect.ownKeys(l)) {
                const t = typeof e === "string" ? +e : NaN;
                if (!(t >= 0 && t < n.length)) notifyKeyDiff(l[e], e, f, n, false);
            }
        }
        notifyFoldTail(e, f, n);
        return;
    }
    const r = e.n;
    let a = 0;
    for (const l in n) {
        const c = n[l];
        let d = f[l];
        // A key under a guess is judged against the guess too (D-34): a truth
        // equal to the value the guess covered still answers it.
                if (optHooks && d === c && i !== f) d = i[l];
        const p = c !== null && typeof c === "object";
        if (d === c && (!p || !isOwned(c)) && (r === null || r[l] === undefined || !hasAccessorFlag(r[l]))) {
            if (r?.[l] !== undefined) a++;
            continue;
        }
        if (p && !s) descend(unwrapValue(i[l]), c, t, o, e, l);
        if (deepUsed && !u && !(p ? targetsEqual(d, c) : isEqual(d, c))) {
            bumpDeep(e);
            u = true;
        }
        if (r !== null) {
            const e = r[l];
            if (e !== undefined) {
                a++;
                notifyKeyValue(e, l, d, c, f, n);
            }
        }
    }
    const c = Object.getOwnPropertySymbols(n);
    for (let i = 0; i < c.length; i++) {
        const l = c[i];
        if (l === $OWNER) continue;
        const u = n[l];
        if (!s && u !== null && typeof u === "object") descend(unwrapValue(f[l]), u, t, o, e, l);
        if (r !== null) {
            const e = r[l];
            if (e !== undefined) {
                a++;
                notifyKeyValue(e, l, f[l], u, f, n);
            }
        }
    }
    // Keys the incoming object no longer has still have nodes.
        if (r !== null && a < e.nc) {
        for (const e of Reflect.ownKeys(r)) {
            // A removed `__proto__` reads absent, not the prototype (core R30,
            // #4672).
            if (!hasOwnP.call(n, e)) notifyKeyDiff(r[e], e, f, e === "__proto__" ? Object.create(null) : n, false);
        }
    }
    notifyFoldTail(e, f, n);
}

const hasOwnP = Object.prototype.hasOwnProperty;

/** Descend into a changed child pair — only where something is proxied
 * below (§6d), never into a raw-marked leaf, never across array/object or a
 * key mismatch (that is a replacement, handled by the parent's slot).
 *
 * A raw in both frames (the same objects swapped, reordered or shifted,
 * #4001) must not leave two slots on one row. A raw already a live row keeps
 * it at the slot it moved to; a row's `pk` follows the slot it sits at (a
 * move or a pairing writes it), so a row whose raw is at that slot of the
 * incoming frame is spoken for and not paired again here; a raw still
 * resolving to a row that moved on drops the lookup (it wraps fresh, or is
 * paired here). One lookup of `nv` on the common path, no allocation. */ function descend(e, n, t, o, f, i) {
    // `nv` is an object at every call site.
    if (e === null || typeof e !== "object") return;
    const l = f.rr;
    n = unwrapValue(n);
    let s = lookupTarget(e, l);
    const u = lookupTarget(n, l);
    if (s !== undefined && s.pk !== i) {
        if (s.pk != i && f.v[s.pk] === s.v) s = undefined; else if (s.u === f) s.pk = i;
    }
    if (u !== undefined && u !== s) {
        if (u.v === n) {
            if (u.u === f) u.pk = i;
            return;
        }
        (l?.map ?? storeLookup).delete(n);
    }
    if (s === undefined) return;
 // nothing proxied below this pair
        if (!isWrappable(n)) return;
    if (rawValuesUsed && isRawValue(n)) return;
    if (Array.isArray(e) !== Array.isArray(n)) return;
    // An array slot was paired by key already (or both rows are keyless).
        if (typeof i !== "number" && t) {
        const o = t(e);
        const f = t(n);
        if (o !== undefined && f !== undefined && !sameKey(o, f)) return;
    }
    if (!o && t !== null && !s.d) return;
    applyAdopt(s, n, t, o);
}

export { reconcileState };