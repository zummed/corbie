import { computed, runWithOwner, signal, setSignal } from "./core/core.js";

import "./core/verdict.js";

import { NOT_PENDING, CONFIG_AUTO_DISPOSE } from "./core/constants.js";

import { attrHooks } from "./core/attribution-hooks.js";

import "./core/scheduler.js";

import { createOwner, getOwner } from "./core/owner.js";

import "./core/effect.js";

import { accessor } from "./signals.js";

import { $TRACK, shallowView, $TARGET } from "./store/types.js";

function mapArray(t, e, i) {
    const s = typeof i?.keyed === "function" ? i.keyed : undefined;
    const n = e.length > 1;
    const r = e;
    const o = {
        H: createOwner(),
        it: 0,
        st: t,
        ot: [],
        ft: r,
        ht: [],
        ln: [],
        Xe: s,
        ct: s || i?.keyed === false ? [] : undefined,
        lt: n && i?.keyed !== false ? [] : undefined,
        ut: i?.keyed === false,
        L: i?.fallback
    };
    const h = computed(updateKeyedMap.bind(o), undefined);
    h.l &= ~CONFIG_AUTO_DISPOSE;
    return accessor(h);
}

const pureOptions = {
    ownedWrite: true
};

function trySmallMove(t, e, i, s) {
    const n = t.ot;
    const r = t.it - 1;
    const o = [];
    const h = [];
    const f = [];
 // flat triples: oldStart, newStart, length
        let c = 256;
    let a = s;
    let l = s;
    while (a <= r && l <= i - 1) {
        const t = n[a];
        const s = e[l];
        if (t === s) {
            const t = a;
            do {
                l++;
            } while (++a <= r && l < i && n[a] === e[l]);
            f.push(t, l - a + t, a - t);
            continue;
        }
        // Bounded realignment lookahead, shorter distance wins.
                let u = -1;
        let w = Math.min(32 - o.length, r - a, c);
        for (let t = 1; t <= w; t++) {
            if (n[a + t] === s) {
                u = t;
                break;
            }
        }
        c -= u === -1 ? w : u;
        let d = -1;
        w = Math.min(32 - h.length, i - 1 - l, c);
        for (let i = 1; i <= w; i++) {
            if (e[l + i] === t) {
                d = i;
                break;
            }
        }
        c -= d === -1 ? w : d;
        if (u !== -1 && (d === -1 || u <= d)) {
            while (u-- > 0) o.push(a++);
            continue;
        }
        if (d !== -1) {
            while (d-- > 0) h.push(l++);
            continue;
        }
        if (c <= 0 || o.length === 32 || h.length === 32) return false;
        o.push(a++);
        h.push(l++);
    }
    for (;a <= r; a++) {
        if (o.length === 32) return false;
        o.push(a);
    }
    for (;l <= i - 1; l++) {
        if (h.length === 32) return false;
        h.push(l);
    }
    return commitSmallMove(t, e, i, s, o, h, f);
}

/** PHASE 2 of the small-move path, in its OWN function so that a pass which
 * only SCANS and bails (a full replace: the first structural pass of a page,
 * typically) compiles nothing but the scan — V8 parses and compiles lazily
 * per function, and cold `replace` measured +0.5 ms with both phases in one
 * body. Pairs displaced sources with destinations (an unmatched destination
 * is a replacement/insertion → general path), then commits: slice() the live
 * arrays, copy shifted runs, patch displaced pairs, dispose leftovers. */ function commitSmallMove(t, e, i, s, n, r, o) {
    const h = t.ot;
    let f;
    let c;
    let a;
    if (r.length !== 0) {
        a = new Array(n.length);
        for (c = 0; c < r.length; c++) {
            let t = -1;
            for (f = 0; f < n.length; f++) {
                if (!a[f] && h[n[f]] === e[r[c]]) {
                    t = f;
                    break;
                }
            }
            if (t === -1) return false;
            a[t] = true;
            r[c] = r[c] << 6 | t;
 // pack pairing (found < 32)
                }
    }
    // DUPLICATES: the general path pairs equal identities by OCCURRENCE ORDER
    // (the chained index map). Displaced↔displaced pairing above is ascending
    // on both sides, so it agrees; but an aligned run was matched by POSITION,
    // and if a displaced identity also occurs inside a run the two algorithms
    // can hand different occurrences different owners (row-local state moves;
    // a shrink could dispose the wrong one). Decline that case — general path.
    // Every displaced identity is a `srcPos` item; any other one in the window sits in a run.
        for (f = 0; f < n.length; f++) {
        const t = h[n[f]];
        // `indexOf` skips holes, which the scan above read as `undefined` rows.
                if (t === undefined) return false;
        for (c = s - 1; (c = h.indexOf(t, c + 1)) !== -1; ) if (!n.includes(c)) return false;
    }
    const l = t.ht;
    const u = t.ln;
    const w = l.slice(0, i);
    const d = u.slice(0, i);
    for (let t = 0; t < o.length; t += 3) {
        const e = o[t];
        const i = o[t + 1];
        if (e !== i) {
            for (let s = 0; s < o[t + 2]; s++) {
                w[i + s] = l[e + s];
                d[i + s] = u[e + s];
            }
        }
    }
    for (c = 0; c < r.length; c++) {
        const t = r[c] >> 6;
        const e = n[r[c] & 63];
        w[t] = l[e];
        d[t] = u[e];
    }
    t.ht = w;
    t.ln = d;
    t.it = i;
    t.ot = e;
    // Dispose unmatched sources LAST (general-path ordering).
        for (f = 0; f < n.length; f++) {
        if (a === undefined || !a[f]) u[n[f]].dispose();
    }
    return true;
}

/** Stage a PURE REMOVAL without the key map: when the new window is the old
 * window with items dropped, in order (one-item removals, every-nth, the
 * table filters), a two-pointer walk lands every survivor in the window
 * stage with one key call per item and returns the leavers; a shape that is
 * not an ordered subsequence returns null with the node stage cleared, and
 * the map path below starts from scratch (it rewrites every window slot of
 * `temp` / `rows` / `indexes`, so only `tempNodes`' `!== undefined` probe
 * needs the reset). The stage arrays are scratch — nothing live is touched. */ function stageRemoval(t, e, i, s, n, r, o, h, f) {
    const c = t.Xe;
    const a = t.ot;
    const l = t.ln;
    const u = [];
    let w = i;
    for (let d = i; d <= n; w++, d++) {
        const n = c ? c(e[d]) : e[d];
        while (w <= s && (c ? c(a[w]) : a[w]) !== n) u.push(l[w++]);
        if (w > s) {
            o.fill(undefined);
            return null;
        }
        r[d] = t.ht[w];
        o[d - i] = l[w];
        if (h) h[d - i] = t.ct[w];
        if (f) f[d - i] = t.lt[w];
    }
    for (;w <= s; w++) u.push(l[w]);
    return u;
}

/** Land a staged window into the live bookkeeping arrays in place: shift the
 * suffix by `dif` (a loop walked from the side that never overwrites an
 * unread slot), take the window from the stage, write survivors' row and
 * index signals. Its own function so the general path's body stays under
 * the JIT's optimization budget — the inlined commit measured ~10% slower on
 * full-window reorders (the small-move note has the same finding). */ function commitWindow(t, e, i, s, n, r, o, h, f) {
    const c = t.ln;
    const a = t.ct;
    const l = t.lt;
    const u = t.it;
    const w = i - u;
    let d, p;
    if (w > 0) {
        // Grow first: a store past the end from the top is a third slower than
        // filling a pre-sized tail (measured on 1k refs, 2026-10-15).
        c.length = i;
        if (a) a.length = i;
        if (l) l.length = i;
        for (d = u - 1, p = i - 1; d > n; d--, p--) {
            c[p] = c[d];
            if (a) a[p] = a[d];
            if (l) l[p] = l[d];
        }
    } else if (w < 0) {
        for (d = n + 1, p = r + 1; d < u; d++, p++) {
            c[p] = c[d];
            if (a) a[p] = a[d];
            if (l) l[p] = l[d];
        }
        c.length = i;
        if (a) a.length = i;
        if (l) l.length = i;
    }
    for (p = s; p <= r; p++) {
        c[p] = o[p - s];
        if (a) writeRow(a[p] = h[p - s], e[p]);
        if (l) setSignal(l[p] = f[p - s], p);
    }
    for (p = r + 1; p < i; p++) {
        if (a) writeRow(a[p], e[p]);
        if (l && w !== 0) setSignal(l[p], p);
    }
}

// Checks the row itself, not `_items`: an aborted pass can leave a row written or staged.
const writeRow = (t, e) => t.le === e && t.ue === NOT_PENDING || setSignal(t, e);

function updateKeyedMap() {
    // Untracked reads inside the internal owner resolve via _parentComputed:
    // routed through the node, a store-proxy lookup sees pending writes (not a
    // stale _value) and an async read in a row callback registers the node as
    // the flight's reader (pending tracking + post-settle retry) instead of
    // vanishing. Set per pass: the first pass runs inside `computed()`, before
    // the node exists to the caller.
    this.H.Ke = getOwner();
    let t = this.st() || [];
    t[$TRACK];
 // top level tracking (a store array's container node)
    // A shallow store array diffs its backing: its slots are raw records, and
    // a trap per slot read was most of a one-row write on a 1k-row list.
        t = shallowView?.(t[$TARGET]) ?? t;
    const e = t.length;
    runWithOwner(this.H, updateKeyedRows.bind(this, t, e));
    return this.ht;
}

// Not a per-call closure: V8 drops a dead closure's optimized code at each major GC.
function updateKeyedRows(t, e) {
    // The next pass's `_items`, taken first: a store list answers each read of a slot through a trap.
    const i = t.slice(0);
    let s, n, 
    // The stage slot a mapper writes its row/index signal into: `j` on the
    // create-all path, `j - start` on the general path, whose stage arrays
    // are sized to the changed WINDOW, not the list.
    r, o, h, 
    // Mappers write freshly-created row/index signals into the STAGE
    // arrays (`rows`/`indexes`), never into `this._rows`/`this._indexes`.
    f = this.ct ? this.ut ? () => {
        o[r] = signal(i[n], pureOptions);
        return this.ft(accessor(o[r]), n);
    } : () => {
        o[r] = signal(i[n], pureOptions);
        h && (h[r] = signal(n, pureOptions));
        return this.ft(accessor(o[r]), h ? accessor(h[r]) : undefined);
    } : this.lt ? () => {
        const t = i[n];
        h[r] = signal(n, pureOptions);
        return this.ft(t, accessor(h[r]));
    } : () => {
        const t = i[n];
        return this.ft(t);
    };
    // fast path for empty arrays
        if (e === 0) {
        if (this.it !== 0) {
            this.H.dispose(false);
            this.ln = [];
            this.ot = [];
            this.ht = [];
            this.it = 0;
            this.ct && (this.ct = []);
            this.lt && (this.lt = []);
        }
        if (this.L && !this.ht.length) {
            // an aborted fallback attempt leaves an owner without a mapping;
            // dispose it before re-creating
            this.ln[0]?.dispose();
            this.ht[0] = runWithOwner(this.ln[0] = createOwner(), this.L);
        }
    }
    // fast path for new create
     else if (this.it === 0) {
        const t = new Array(e);
        const c = new Array(e);
        o = this.ct && new Array(e);
        h = this.lt && new Array(e);
        try {
            for (n = 0; n < e; n++) {
                r = n;
                t[n] = runWithOwner(c[n] = createOwner(), f);
            }
        } catch (t) {
            for (s = 0; s <= n; s++) c[s]?.dispose();
            throw t;
        }
        // commit
                if (this.ln[0]) this.ln[0].dispose();
 // previous fallback
                this.ht = t;
        this.ln = c;
        o && (this.ct = o);
        h && (this.lt = h);
        this.ot = i;
        this.it = e;
    } else {
        let t, c, a, l, u, w, d, p, m, O;
        // The pass's write to a per-slot signal (a row accessor in index mode,
        // an index accessor in keyed mode). The list's frame lives in these
        // writes as much as in the computed's result, so under a LANE pass they
        // are the lane's frame too (F1): a plain `setSignal` staged them into
        // the action's transaction and the row readers were served the
        // committed value until the action landed — `<For>` without `keyed`
        // showed the pre-action list for the whole action while the keyed modes
        // showed the optimistic one. The engine publishes the write as a
        // derived override on the slot (lanes stage, A17) and lands or
        // supersedes it when a plain pass later writes a slot still carrying
        // one (A18) — `landOnOverride`'s slot arm. Decided once per pass, not
        // per slot: a lane pass marks the map (`_laneSlots`: its slots may
        // carry overrides), a marked map routes every slot write through the
        // engine while any lane is live — a slot's override resolves with its
        // lane's transaction, or with the batch for an orphan lane
        // (resolveOptimisticNodes, then cleanupCompletedLanes, both before the
        // completion wakes any recompute), so a pass that finds no lane live
        // knows the slots are clean and drops the mark — and an unmarked map
        // takes `writeRow` directly. The engine is installed whenever a lane
        // is active.
        // CARVE 2: the lane-slot arm (`_landOnOverride`, F1) went with the engine.
        // skip common prefix
                for (t = 0, c = Math.min(this.it, e); t < c && (this.ot[t] === i[t] || this.ct && compare(this.Xe, this.ot[t], i[t])); t++) {
            if (this.ct) writeRow(this.ct[t], i[t]);
        }
        // skip common suffix — counted only; retained entries land in one pass
        // at commit instead of being staged and copied twice
                for (c = this.it - 1, a = e - 1; c >= t && a >= t && (this.ot[c] === i[a] || this.ct && compare(this.Xe, this.ot[c], i[a])); c--, 
        a--) ;
        // no structural change (every position matched in place at equal
        // length — the common post-reconcile shape): keep the same mapped
        // array identity so downstream consumers don't re-run at all
                if (t === e && this.it === e) {
            this.ot = i;
            return;
        }
        // SMALL-MOVE FAST PATH: extracted to its own function — inlining it
        // here bloats updateKeyedMap past the JIT's optimization budget and
        // deoptimizes the GENERAL path (measured 2x on reverse). Gated to
        // LARGE trimmed windows: when the trims already shrank the window
        // (plain removals, tail edits), the general path is window-
        // proportional and cheap — the fast path would only re-walk what the
        // trims proved.
                if (e <= this.it && c - t > 64 && this.ct === undefined && this.lt === undefined) {
            // PROBE before the scan: a small move keeps a mid-window item within
            // ±32 of its old position; a REPLACE (all fresh items — the shape
            // every page's first structural pass usually is) has it nowhere.
            // ~65 compares, no allocation, and the scan function is never
            // compiled for a replace (its cold first-call compile was the cost).
            const s = t + (a - t >> 1);
            const n = i[s];
            const r = Math.min(c, s + 32);
            let o = Math.max(t, s - 32);
            while (o <= r && this.ot[o] !== n) o++;
            if (o <= r && trySmallMove(this, i, e, t)) return;
        }
        const _ = e - this.it;
        // WINDOW-SIZED STAGING, IN-PLACE COMMIT (2026-10-15). The strong-abort
        // contract (#2903) only needs the pass to touch nothing live before
        // every mapper has succeeded; it never needed four fresh newLen-sized
        // arrays. Only the changed window [start, newEnd] is staged — the
        // prefix and suffix are, by construction, the rows that did not move
        // relative to each other — and at commit the bookkeeping arrays
        // (`_nodes`, `_rows`, `_indexes`) take the window in place with the
        // suffix shifted by a plain loop (`copyWithin` is a generic
        // per-element builtin in V8 — 30× slower than the loop on 1k refs,
        // measured 2026-10-15). `_mappings` alone is
        // rebuilt: its fresh identity is the change signal downstream relies
        // on. A one-row edit on a 500-row list used to allocate ~20 KB of
        // staging plus a position-sized chain array per commit (uibench's
        // one-item tree ops at 4–6× Octane, jfb-reorder `removefirst` at
        // 2.5×); now it stages one slot.
                const y = a - t + 1;
        const A = new Array(e);
        const R = new Array(y);
        o = this.ct ? new Array(y) : undefined;
        h = this.lt ? new Array(y) : undefined;
        // A shrinking pass is tried as a pure removal first (stageRemoval: no
        // key map, one key call per item); anything else takes the map path.
                const g = _ < 0 ? stageRemoval(this, i, t, c, a, A, R, o, h) : null;
        if (g !== null) {
            // (The list-churn census below wants removals AND creations in one
            // pass; a pure removal has none, so no item ledger is kept here.)
            if (g.length !== 0) p = g;
        } else {
            // 0) prepare a map of all indices in the changed window of newItems,
            // scanning backwards so we encounter them in natural order
            w = new Map;
            d = new Array(y);
            for (n = a; n >= t; n--) {
                l = i[n];
                u = this.Xe ? this.Xe(l) : l;
                s = w.get(u);
                d[n - t] = s === undefined ? -1 : s;
                w.set(u, n);
            }
            // 1) step through the old changed window and see if items can be found
            // in the new set; if so, stage them at their new positions; if not,
            // queue them for disposal at commit
                        for (s = t; s <= c; s++) {
                l = this.ot[s];
                u = this.Xe ? this.Xe(l) : l;
                n = w.get(u);
                if (n !== undefined && n !== -1) {
                    A[n] = this.ht[s];
                    R[n - t] = this.ln[s];
                    o && (o[n - t] = this.ct[s]);
                    h && (h[n - t] = this.lt[s]);
                    n = d[n - t];
                    w.set(u, n);
                } else {
                    (p ??= []).push(this.ln[s]);
                }
            }
            // 2) create new rows into the window stage; an abort disposes only these
                        try {
                for (n = t; n <= a; n++) {
                    if (R[n - t] !== undefined) continue;
                    (m ??= []).push(R[n - t] = createOwner());
                    if (false && attrHooks !== null) ;
                    r = n - t;
                    A[n] = runWithOwner(R[n - t], f);
                }
            } catch (t) {
                if (m) for (s = 0; s < m.length; s++) m[s].dispose();
                throw t;
            }
        }
        // 3) commit: the retained prefix and suffix land in the fresh mapped
        // array; the bookkeeping arrays take the staged window IN PLACE (own
        // function: kept out of this body on purpose, see commitWindow); then
        // the exited rows dispose
                for (s = 0; s < t; s++) A[s] = this.ht[s];
        for (n = a + 1; n < e; n++) A[n] = this.ht[n - _];
        commitWindow(this, i, e, t, c, a, R, o, h);
        this.ht = A;
        this.it = e;
        this.ot = i;
        if (p) for (s = 0; s < p.length; s++) p[s].dispose();
    }
}

/**
 * Reactively renders a callback `count` times, reusing previously-rendered
 * entries when only the count changes. Underlying helper for `<Repeat>`.
 *
 * - `options.from` — start index (default `0`); useful for offset/windowed
 *   rendering.
 * - `options.fallback` — accessor returning a value to show when count is `0`.
 *
 * @example
 * ```ts
 * const view = repeat(count, i => `Item ${i}`, { fallback: () => "empty" });
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/advanced/jsx-component-primitives/repeat
 */ function repeat(t, e, i) {
    const s = e;
    const n = {
        H: createOwner(),
        it: 0,
        wt: 0,
        dt: t,
        ft: s,
        ln: [],
        ht: [],
        Ot: i?.from,
        L: i?.fallback
    };
    const r = computed(updateRepeat.bind(n));
    r.l &= ~CONFIG_AUTO_DISPOSE;
    return accessor(r);
}

// Same staged-commit discipline as `updateKeyedMap` (#2903): the retained
// window overlap is copied into fresh arrays, missing indexes are created
// into them, and `this` is only touched — including disposal of rows leaving
// the window — after every `_map` call succeeded. A NotReadyError mid-pass
// disposes only the owners this pass created and leaves prior state intact
// for the post-settle retry. The overlap math also subsumes the previous
// disjoint-window/front-clear/end-clear/shift special cases.
function updateRepeat() {
    // As in updateKeyedMap: the node is the row callbacks' reader, per pass.
    this.H.Ke = getOwner();
    const t = this.dt();
    const e = this.Ot?.() || 0;
    runWithOwner(this.H, updateRepeatRows.bind(this, t, e));
    return this.ht;
}

// Not a per-call closure: V8 drops a dead closure's optimized code at each major GC.
function updateRepeatRows(t, e) {
    if (t === 0) {
        if (this.it !== 0) {
            this.H.dispose(false);
            this.ln = [];
            this.ht = [];
            this.it = 0;
            // Reset offset to match the cleared data (#2767, repro 2).
                        this.wt = 0;
        }
        if (this.L && !this.ht.length) {
            // an aborted fallback attempt leaves an owner without a mapping;
            // dispose it before re-creating
            this.ln[0]?.dispose();
            this.ht[0] = runWithOwner(this.ln[0] = createOwner(), this.L);
        }
        return;
    }
    const i = e + t;
    const s = this.wt + this.it;
    // Retained overlap [keepStart, keepEnd) in global indexes; empty when the
    // windows are disjoint or when coming from empty/fallback.
        const n = Math.max(e, this.wt);
    const r = Math.min(i, s);
    const o = new Array(t);
    const h = new Array(t);
    for (let t = n; t < r; t++) {
        h[t - e] = this.ln[t - this.wt];
        o[t - e] = this.ht[t - this.wt];
    }
    try {
        for (let t = e; t < i; t++) {
            if (t >= n && t < r) continue;
            o[t - e] = runWithOwner(h[t - e] = createOwner(), () => this.ft(t));
        }
    } catch (t) {
        for (let t = e; t < i; t++) if ((t < n || t >= r) && h[t - e]) h[t - e].dispose();
        throw t;
    }
    // commit: dispose the previous fallback or the rows leaving the window
        if (this.it === 0) this.ln[0]?.dispose(); else for (let t = this.wt; t < s; t++) if (t < e || t >= i) this.ln[t - this.wt].dispose();
    this.ht = o;
    this.ln = h;
    this.wt = e;
    this.it = t;
}

function compare(t, e, i) {
    return t ? t(e) === t(i) : true;
}

export { mapArray, repeat };