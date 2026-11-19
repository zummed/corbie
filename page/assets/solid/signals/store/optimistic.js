import { CONFIG_GUESS, CONFIG_OVERRIDE, EFFECT_RENDER, CONFIG_CHILDREN_FORBIDDEN, NOT_PENDING, STATUS_PENDING, CONFIG_HELD, REACTIVE_REASK } from "../core/constants.js";

import { isEqual, joinPassAt, context, read, setSignal, untrack, slotSignal } from "../core/core.js";

import { link } from "../core/graph.js";

import { getObserver } from "../core/owner.js";

import { setLaneValue, readGuesses } from "../core/lanes.js";

import { take, pendingGuessOf, optimisticWrite, guessedValueOf } from "../core/guesses.js";

import { latestOf, covered } from "../core/truth.js";

import { GlobalQueue, txOf, question, laneDirty, insertSubs, joinFuture, resolveTx, passLane, staleReader, clock, merge, actionDepth, holdNode } from "../core/scheduler.js";

import { createProjectionInternal } from "./projection.js";

import { reconcileState } from "./reconcile.js";

import { storeSetter, unwrapValue, committed, linkedTo, getHasNode, getNode, serveKeyValue, targetsEqual, cloneRaw, sameKey } from "./store.js";

import { installOptHooks } from "./hooks.js";

import { lookupTarget, $OWNER } from "./target.js";

import { $PATCH, isWrappable, $TARGET } from "./types.js";

/**
 * Store — optimistic stores on lanes (plan sec. 31, S4; INTERNALS §3/§7, RUL-3;
 * S2: a guess is a patch).
 *
 * No store-side layer, no backup snapshots, no retaining ledger: a user
 * write to an optimistic store is one GUESS per target — the user's draft,
 * carried by the target's guess carrier (`kg`, a `$PATCH` slot node like a
 * hold's), with the keys the draft changed recorded on it. Lanes do the rest
 * on that one node (`optimisticWrite`, the same write `createOptimistic`'s
 * setter makes): the guess shows under the action's hold, a landing beneath
 * it stages as truth and confirms it when equal (A18/A24), the action's
 * settle dissolves it (a revert to the frame it covered, or the landed
 * truth). The committed backing is never touched by a guess; a guess folds
 * nothing (its truth lands through its own batch).
 *
 * Reads find the carrier by key (store.ts `frameOf`: a key the guess
 * changed reads through it, tracked, by core's lane rules — what the lane
 * shows; presence, descriptors and enumeration are the patch frame's). A
 * chained target's committed frame is the inner store's proxy: an unguessed
 * key reads through to the inner store's current frame by construction.
 *
 * Landings (Q-D, plan sec. 31.7): the derive's output is reconciled against the
 * optimistic VIEW — a row the guess added and the server returns with the
 * same key in the same position keeps its proxy (the row target adopts the
 * server object) — and reaches the carrier as a truth frame (store.ts
 * `notifyWrites` → `setSignal` → lanes `laneWrite`), judged over the guess's
 * own keys (`echoes`): confirmed beneath, re-based over an older truth
 * (`_laneRebase`), folded (D-34), or a correction.
 */ const hasOwn = Object.prototype.hasOwnProperty;

/** S2 — a target's guess carriers, one per lane (per action): `$PATCH` slot
 * nodes whose lane value is the user's draft, the keys it changed recorded
 * (`wk`, `del`; a keyed list's rows as written and removed keys with it).
 * Oldest first; a carrier that is no lane's is dropped (`sweepOverlaid`).
 * Off the target (a plain store carries nothing for the optimistic engine). */ const carriers = new WeakMap;

const kgOf = e => carriers.get(e) ?? null;

const INDEX = /^(0|[1-9]\d*)$/;

/** The newest live guess carrier for `key`, if any: a lane's guess, or one a
 * correction re-homed into its hold (HELD) — the screen keeps what it showed
 * until that hold lands (A18 (c)), a member reads the truth beneath;
 * `read()` of the carrier decides. Its record (`wk`, `del`) has the key —
 * or, for a read (`reads`), the key is an index or the length of a keyed
 * list's arrangement guess (`g`): a list reader enters the lane through
 * them, as it did through the guessed container. An arrangement guess does
 * not claim a key it only carried over from another live guess's carrier
 * (`borrowed`, #4784): the read falls through to that carrier — or to the
 * truth once it dissolves — instead of serving this frame's stale copy of
 * the expired guess. A row carried over verbatim in a keyed list is not
 * borrowed (`carriedVerbatim`, #4724): the row target decides its contents.
 * A landing asks without `reads`: a key the guess only carried over is the
 * truth's, and its leaf tells its readers as ever (store.ts `leafSet`). */ function guessFor(e, n, t = false, r = true) {
    const o = kgOf(e);
    if (o !== null) for (let e = o.length - 1; e >= 0; e--) {
        const t = o[e];
        if (t.l & (CONFIG_OVERRIDE | CONFIG_HELD) && (t.wk?.has(n) || t.del?.has(n) || r && t.g !== undefined && !t.borrowed?.has(n) && (n === "length" || typeof n === "string" && INDEX.test(n)))) return t;
    }
    return null;
}

/** The newest live guess carrier (the frame a structural read sees). */ function newestGuess(e) {
    const n = kgOf(e);
    if (n !== null) for (let e = n.length - 1; e >= 0; e--) if (n[e].l & (CONFIG_OVERRIDE | CONFIG_HELD)) return n[e];
    return null;
}

/** The hold's carrier of a truth staged beneath the screen: the container
 * node while a transaction holds its staging (a landing adopted under an
 * action), else null. A guess made over it carries that truth beneath. */ function heldCarrier(e) {
    const n = e.k;
    return n !== null && n.l & CONFIG_HELD && n.ue !== NOT_PENDING ? n : null;
}

/** A guessed key's value from the carrier's frame. */ function carrierValue(e, n) {
    return hasOwn.call(e, n) || n !== "__proto__" ? e[n] : undefined;
}

/** An optimistic setter's targets whose staging was set aside (`draft`),
 * restored when the setter's writes become guesses (`writes`). */ const optStaged = new Map;

/** The view the setter composes on (`writer`): the committed frame — a
 * chained backing resolved through the inner store's — with the lane's
 * value and the tick's own guess over it; `plain`: the committed frame
 * alone (the comparator's world). */ function writerBacking(e, n = false) {
    const t = committed(e);
    const r = t[$TARGET];
    const o = r === undefined ? t : writerBacking(r, n);
    return n ? o : guessView(e, o, true);
}

/** `src` with the target's guess over it: the carrier's unflushed guess or
 * lane value when it carries one, else `src` itself. */ function guessView(e, n, t = false) {
    const r = kgOf(e);
    if (r !== null) for (let n = r.length - 1; n >= 0; n--) {
        const o = r[n];
        if (t) {
            // The writer composes on the screen with the tick's own guesses: the
            // unflushed guess, the lane's value, a corrected guess's screen until
            // its hold lands (D-34, D-37 — never the truth beneath it).
            const n = writerFrame(o);
            if (n !== NOT_PENDING) return ownedKeys(e, o, r, n);
        } else if (o.l & (CONFIG_OVERRIDE | CONFIG_HELD)) {
            // A snapshot, `deep()`, a reconcile's key matching: the carrier as
            // core reads it for this reader (the lane's world, the screen, the
            // truth beneath — the same frame the per-key reads compose).
            const e = untrack(() => read(o));
            if (e !== NOT_PENDING) return e;
        }
    }
    return n;
}

/** A carrier's frame as the writer sees it: the unflushed guess or the
 * lane's value, a corrected guess's screen until its hold lands (D-34). */ function writerFrame(e) {
    if (e.l & CONFIG_OVERRIDE || pendingGuessOf(e) !== NOT_PENDING) {
        const n = guessedValueOf(e);
        if (n !== NOT_PENDING) return n;
    }
    return e.l & CONFIG_HELD ? e.le : NOT_PENDING;
}

/** The newest carrier's frame with each key another carrier owns taken from
 * that carrier, as a read of the key finds it (`guessFor`): the newest frame
 * only copied those keys when it was written, and an older carrier a later
 * guess on the same key went to has moved on since (#4988). A list's frame
 * is its arrangement, taken whole. */ function ownedKeys(e, n, t, r) {
    let o = r;
    if (!Array.isArray(r)) for (const i of t) if (i !== n && i.wk !== null) for (const n of i.wk) if (guessFor(e, n, false, false) === i) {
        const e = writerFrame(i);
        if (e !== NOT_PENDING && e[n] !== o[n]) (o === r ? o = {
            ...r
        } : o)[n] = e[n];
    }
    return o;
}

/** The user's draft on an optimistic family: a clone of the view the user
 * saw (#3665). A staging already on the target (a landing adopted this
 * flush) is set aside for the setter's duration: the guess goes over it, it
 * stays the truth beneath. */ function optimisticDraft(e) {
    if (e.pb !== null) optStaged.set(e, e.pb);
    return cloneRaw(writerBacking(e), e);
}

/** The target's guess carrier, made at the first guess: its committed value
 * the frame the guess is shown over. Its comparator judges a truth against
 * the guess over the keys the guess changed (`echoes`: the server's echo of a
 * guessed row is a new object with the same contents, #3898). */ function guessCarrier(e, n) {
    let t = kgOf(e);
    if (t === null) carriers.set(e, t = []);
    // The carrier a guess goes on: one with a guess still unflushed (this
    // flush's: one lane), or a live one whose record shares a key (two
    // suggestions for one slot entangle their frames, A34 (1) — as the one
    // leaf node did); else a new one — another action's guess of other keys
    // is its own lane's. A corrected guess whose screen stays until its hold
    // lands (D-34) counts for its keys: the setter composed on that screen,
    // and the guess over it ends with that hold (#4869) — a new carrier over
    // the committed frame would drop a write back to it as no change.
        for (let r = t.length - 1; r >= 0; r--) {
        const o = t[r];
        const i = o.l & (CONFIG_OVERRIDE | CONFIG_GUESS);
        if (!i && pendingGuessOf(o) === NOT_PENDING && !(o.l & CONFIG_HELD && n)) continue;
        if (pendingGuessOf(o) !== NOT_PENDING || i && n === undefined) return entangle(e, o, n);
        if (o.wk !== null) for (const e of n) if (o.wk.has(e) || o.del?.has(e)) return o;
    }
    const r = slotSignal(committed(e), patchEquals, e, $PATCH, false);
    // Written over a held truth (a landing the target's hold keeps beneath
    // the screen): that truth is the one beneath the guess, judged at the
    // body's end (A18, #3330) — as it was the leaf's staging.
        const o = heldCarrier(e);
    if (o !== null) {
        r.ue = o.ue;
        holdNode(r, txOf(o));
    }
    r.guess = true;
    r.wk = null;
    r.del = null;
    r.removed = null;
    r.borrowed = null;
    r.g = undefined;
    t.push(r);
    return r;
}

/** This flush's carrier writes a key another action's live guess holds: two
 * suggestions for one slot, and their frames entangle (A34 (1)) — as the one
 * leaf node's lanes merged at the seam (`applyGuesses`): an unapplied
 * carrier is held on that guess's frame, so its lane is born under it; an
 * applied one's frame merges now. */ function entangle(e, n, t) {
    if (t !== undefined) for (const r of kgOf(e)) if (r !== n && r.l & CONFIG_OVERRIDE && r.wk !== null) for (const e of t) if (r.wk.has(e) || r.del?.has(e)) {
        const e = resolveTx(txOf(r)._parent);
        if (n.l & CONFIG_OVERRIDE) merge(resolveTx(txOf(n)._parent), e); else if (!(n.l & CONFIG_HELD)) holdNode(n, e);
        break;
    }
    return n;
}

/** The derive's open question is a re-ask of the same one (D-34 ¶3): its
 * own `refresh()`, or every source it waits on re-asked (`refresh(list)`
 * ahead of a sync derive over `list`, #4724) — an input re-asked is not an
 * input changed. A guess written over it stands in for the flight (A17). */ function reasked(e) {
    if (e.R & REACTIVE_REASK || e.T?.Ve) return true;
    const n = e.T?.se;
    if (!n?.size) return false;
    for (const t of n) if (t === e || !(t.R & REACTIVE_REASK || t.T?.Ve)) return false;
    return true;
}

function patchEquals(e, n) {
    if (e === n) return true;
    if (e === null || n === null || typeof e !== "object" || typeof n !== "object") return false;
    // A chained backing (a proxy) is judged as the inner store's committed
    // frame: the proxy would serve the inner's staging, and a guess equal to
    // that staging is still a guess over the committed world.
        const t = e[$TARGET];
    if (t !== undefined) e = writerBacking(t, true);
    const r = n[$TARGET];
    if (r !== undefined) n = writerBacking(r, true);
    const o = this.O.rr;
    // A structural guess (a list's, the only kind that records `g`): the
    // arrangement, rows by identity or key, as the container judged it.
        if (this.g !== undefined && (e.length !== n.length || e.some((e, t) => !rowSame(e, n[t], o)))) return false;
    // Each key the guess changed, by content: the same object, or the
    // server's echo of a guessed one — a new object with the same contents
    // (#3898). A row the adoption paired by key with a guessed one is not
    // thereby its equal: the leaves judged the content, so does the carrier.
        const i = this.wk;
    if (i !== null) for (const t of i) {
        if (t in e !== t in n) return false;
        if (!GlobalQueue.sn(e[t], n[t])) return false;
    }
    const f = this.del;
    if (f !== null) for (const t of f) if (t in e !== t in n) return false;
    return true;
}

// A keyed arrangement guess carries another guess's row verbatim when the
// setter returned the view's row unchanged: the frame holds the view's row
// object (the `echoes` normalization), and the row target — not the frame
// copy — decides the row's contents, so the arrangement read serves this
// frame's copy instead of falling through to the lender. Falling through
// re-links the reader to the lender's carrier, which loses the
// notification that carries the new arrangement to the DOM (#4724).
// Primitives always borrow: the lender may rewrite the value, and this
// frame's copy would go stale (#4784).
function carriedVerbatim(e, n, t, r) {
    if (typeof r !== "string" || !INDEX.test(r)) return false;
    const o = e.rr?.key;
    if (o === null || o === undefined) return false;
    const i = unwrapValue(t[r]);
    return isWrappable(i) && i === n[r];
}

function notifyOptimisticWrites(e, n) {
    // The view the user saw: the committed frame (a staging adopted eagerly is
    // not it) with the lane's value and the tick's own guess over it.
    const t = writerBacking(e);
    const r = Array.isArray(n);
    const o = e.ch ? e.v : null;
    const i = new Set;
    let f = null;
    let s = false, l = false;
    for (const e of Reflect.ownKeys(n)) {
        if (e === $OWNER) continue;
        const f = unwrapValue(n[e]);
        if (!(e in t)) {
            i.add(e);
            s = l = true;
        } else {
            const l = t[e];
            // A chained draft holds the inner store's raws: the inner's own frame.
                        const u = unwrapValue(o !== null && l ? l : l);
            // A row the setter carried over is not a write (#4724, the point of
            // write): the draft's rows are clones of the view's, so a carried row
            // is a new object with the view's contents — the same row, as the
            // server's echo of a guessed one is (`echoes`, #3898).
                        if (!isEqual(u, f) && !targetsEqual(u, f)) {
                if (r && echoes(u, f)) {
                    // The frame carries the row the view showed, not the draft's clone
                    // of it: the same object wraps as the same proxy (a list keeps its
                    // row identities across a structural guess, deep-chained-view).
                    n[e] = l;
                } else {
                    i.add(e);
                    if (r) s = true;
                }
            }
        }
    }
    for (const e of Reflect.ownKeys(t)) {
        if (e === $OWNER || e in n) continue;
        (f ??= new Set).add(e);
        s = l = true;
    }
    const u = optStaged.get(e);
    if (u !== undefined) optStaged.delete(e);
    if (i.size === 0 && f === null) return u ?? null;
    const a = guessCarrier(e, f === null ? i : new Set([ ...i, ...f ]));
    // The questions of every action that guessed on this carrier: same-slot
    // guesses share it (D-33), and its `_q` is only the newest's (#4874).
        (a.qs ??= new Set).add(question);
    // The record accumulates across a lane's writes to the same target.
        if (a.wk === null) a.wk = i; else for (const e of i) a.wk.add(e);
    if (f !== null) {
        if (a.del === null) a.del = f; else for (const e of f) a.del.add(e);
    }
    // The keys this guess's frame only carries over from another live guess's
    // carrier (#4784): the draft was composed on the view, so its frame holds
    // the other guess's values at keys this guess did not itself write. The
    // arrangement read falls through on those keys — to the lender's carrier
    // while it lives, to the truth once it dissolves — instead of serving
    // this frame's copy of the expired guess. A row the setter carried over
    // verbatim is not borrowed (`carriedVerbatim`): the row target decides
    // its contents, and falling through would lose the DOM notification
    // (#4724). The record accumulates across a lane's writes, like `wk`: a
    // key this guess later writes itself is its own, and the `wk`/`del`
    // match above takes precedence regardless. Not a guess that changed the
    // keys or the length: it writes the list itself, the slot the other
    // structural guesses write, so they entangle and its frame holds until
    // they all settle (D-33) — a fall-through there served the truth's
    // length beside this frame's rows.
        const c = kgOf(e);
    if (c !== null && c.length > 1 && !l && !i.has("length")) for (const r of c) {
        if (r === a) continue;
        const o = r.l & (CONFIG_OVERRIDE | CONFIG_GUESS | CONFIG_HELD) || pendingGuessOf(r) !== NOT_PENDING;
        if (!o) continue;
        for (const o of r.wk ?? []) if (!i.has(o) && !f?.has(o) && !carriedVerbatim(e, t, n, o)) (a.borrowed ??= new Set).add(o);
        for (const o of r.del ?? []) if (!i.has(o) && !f?.has(o) && !carriedVerbatim(e, t, n, o)) (a.borrowed ??= new Set).add(o);
    }
    if (s && r) {
        const r = n.map(arrangementRow, e.rr);
        // The keys of the view the setter saw that are not the guess's own
        // rows, which a re-base over another truth takes from that truth
        // (`_laneRebase`): the rows the setter removed, which it never brings
        // back, and the rows it only carried over from the truth beneath (the
        // committed row itself), which the new truth decides. Rows it added, or
        // carried from another action's guess, are the guesses' own: their
        // landings judge them. A set (empty or not) marks a keyed arrangement
        // guess (`fold`).
                let o = null;
        const i = e.rr.key;
        if (i !== null && i !== undefined) {
            o = new Set;
            const n = new Set;
            const f = committed(e);
            const s = new Set(f);
            for (let e = 0; e < r.length; e++) {
                const t = isWrappable(r[e]) ? i(r[e]) : undefined;
                if (t !== undefined) n.add(t);
            }
            // The view's rows, then the truth's: a truth row the view lacked was
            // removed by another action's guess, whose landing judges it — a
            // re-base never brings it back beside that guess (F-11, #4656).
                        const l = t.length;
            const u = [ ...t, ...f ];
            for (let e = 0; e < u.length; e++) {
                const t = unwrapValue(u[e]);
                const r = isWrappable(t) ? i(t) : undefined;
                if (r !== undefined && (!n.has(r) || e < l && s.has(t))) o.add(r);
            }
        }
        a.removed = o;
        a.g = r;
        // Written while the derive asks a new question (an input changed, not a
        // `refresh()`): the guess's base answers the old one, so the rows it only
        // carried over are that answer's — the new answer re-bases it like an
        // older truth instead of folding beneath it (`_laneRebase`, F-1, #4525).
        // A refresh's flight already up (`_reask`, the pass consumed the flag)
        // asks nothing new either (D-34 ¶3, #4656).
                const f = e.rr.node;
        if (o && f?.m & STATUS_PENDING && !reasked(f)) a.g = null;
    }
    // By value: the draft is the guess as written (a function leaf is a
    // value, not an updater, #3017).
        optimisticWrite(a, () => n);
    if (e.dk !== null) optimisticWrite(e.dk, {});
    markOverlaid(e);
    return u ?? null;
}

// A borrowed row must keep following its source through a re-base. Unwrapping
// a foreign proxy here would retain its old backing after that source adopts
// a new answer (#4931). Local rows still use the family's raw identity.
// (`this`: the family, as `rows.map(arrangementRow, fam)`.)
function arrangementRow(e) {
    return e?.[$TARGET]?.rr !== this ? e : unwrapValue(e);
}

/** Two rows of a keyed list's arrangement, by identity — by key when the
 * family has one. */ function rowSame(e, n, t) {
    if (isEqual(e, n) || targetsEqual(e, n)) return true;
    const r = t.key;
    if (!r || !isWrappable(e) || !isWrappable(n)) return false;
    // A row the store already serves is its proxy's row: a reader (a list)
    // holds that proxy, so another object at the index is another row, by key
    // or contents alike — a guess's fresh object wraps a new proxy. A landing's
    // object has been adopted by the row it pairs with, so it serves that
    // row's proxy. Only rows no proxy serves yet pair by key, keyless rows by
    // position — reconcile's fallback (#3898).
        const o = lookupTarget(e = unwrapValue(e), t);
    const i = lookupTarget(n = unwrapValue(n), t);
    if (o || i) return o === i;
    // (Two keyless rows: `undefined` is the same key.)
        return sameKey(r(e), r(n));
}

/** Whether reconcile would write anything into `a` from `b`: wrappable
 * pairs descend, leaves compare as its writes do. A key mismatch is a
 * replacement, which a content difference already says. Plain data — the
 * server's echo of a guessed row is a new object with the same contents. */ function echoes(e, n) {
    if (e === n) return true;
    if (!isWrappable(e) || !isWrappable(n)) return false;
    e = unwrapValue(e);
    n = unwrapValue(n);
    const t = Object.keys(e);
    if (Array.isArray(e) !== Array.isArray(n) || t.length !== Object.keys(n).length) return false;
    for (const r of t) if (!hasOwn.call(n, r) || !echoes(e[r], n[r])) return false;
    return true;
}

// A cycle (a row that references itself) is not plain data: no match, so
// the landing stays a correction.
GlobalQueue.sn = (e, n) => {
    try {
        return echoes(e, n);
    } catch {
        return false;
    }
};

/** Write `incoming` into the draft `draft` (a store proxy inside a user's
 * setter): arrays match rows by key (position when unkeyed) and recurse into
 * the matched row's draft; objects recurse into a wrappable pair, assign
 * otherwise; keys `incoming` lacks are deleted. */ function reconcileDraft(e, n, t) {
    if (Array.isArray(n)) {
        const r = e.length;
        const o = new Array(r);
        for (let n = 0; n < r; n++) o[n] = e[n];
        let i = null;
        if (t !== null) {
            i = new Map;
            for (let e = 0; e < r; e++) {
                const n = unwrapValue(o[e]);
                if (!isWrappable(n)) continue;
                const r = t(n);
                if (r !== undefined && !i.has(r)) i.set(r, o[e]);
            }
        }
        for (let f = 0; f < n.length; f++) {
            const s = n[f];
            let l;
            if (isWrappable(s)) {
                if (i !== null) {
                    const e = t(s);
                    if (e !== undefined) l = i.get(e);
                } else if (f < r && isWrappable(unwrapValue(o[f]))) l = o[f];
            }
            if (l !== undefined) {
                if (unwrapValue(l) !== s) reconcileDraft(l, s, t);
                if (e[f] !== l) e[f] = l;
            } else if (unwrapValue(e[f]) !== s) e[f] = s;
        }
        if (e.length !== n.length) e.length = n.length;
        return;
    }
    for (const r of Object.keys(n)) {
        const o = n[r];
        const i = e[r];
        const f = unwrapValue(i);
        if (isWrappable(o) && isWrappable(f) && Array.isArray(o) === Array.isArray(f) && (t === null || Array.isArray(o) || sameKey(t(o), t(f)))) {
            if (f !== o) reconcileDraft(i, o, t);
        } else if (f !== o) e[r] = o;
    }
    for (const t of Object.keys(e)) if (!(t in n)) delete e[t];
}

/** Core asks (scheduler `laneDirty`): the lane dirtied the target's guess
 * carrier — its readers are the readers of the leaves of the keys the guess
 * changed (they link their leaves, and the carrier once they read through
 * it): dirtied as the lane's work, with the presence nodes and the
 * container (a structural guess). */
/** The nodes the readers of a guess carrier's keys link: the leaves and
 * presence nodes of the keys it changed, and the container. */ function readersOf(e, n) {
    if (e.Xe !== $PATCH || !e.guess) return;
    const t = e.O;
    const each = e => {
        for (const r of e) {
            const e = t.n?.[r];
            if (e !== undefined && e.o !== null) n(e);
            const o = t.h?.[r];
            if (o !== undefined && o.o !== null) n(o);
        }
    };
    each(e.wk ?? []);
    if (e.del !== null) each(e.del);
    const r = t.k;
    if (r !== null && r.o !== null) n(r);
}

GlobalQueue.lo = (e, n) => readersOf(e, e => laneDirty(e, n));

/** Core asks (lanes `rerun`): a guess coming back at a landing re-derives
 * the carrier's readers there — the readers of its keys' leaves. */ GlobalQueue._n = (e, n) => readersOf(e, e => {
    for (let t = e.o; t !== null; t = t.t) staleReader(t.u, n);
});

/** Core asks (lanes `laneWrite`): the family's derive, re-asking in a lane of
 * another action than the one that guessed this target, writes it — that
 * lane's new question, which the guess (over the old one) does not answer:
 * the carrier is the lane's derivation from here (F-6, #4336; lanes
 * `laneStage` for a node's own pass). */ GlobalQueue.Nn = (e, n) => {
    const t = e.O;
    if (n) {
        // The guess comes back (lanes `endLanes`): on the target's carrier — the
        // node itself, re-attached if the taker lane's end swept it (as the leaf
        // path re-made its node).
        markOverlaid(t);
        let n = kgOf(t);
        if (n === null) carriers.set(t, n = []);
        if (n.indexOf(e) === -1) n.push(e);
        return e;
    }
    const r = t?.rr?.node?.T?.Re;
    const o = txOf(e);
    if (!r?.P || sameAction(r, o)) return false;
    // The re-ask takes the target's guess whole (the carrier and the deep
    // witness): a node other than the written one shows its truth in the
    // lane's world (lanes `take`).
        for (const n of [ ...kgOf(t) ?? [], t.dk ]) if (n?.l & CONFIG_GUESS && txOf(n) === o) take(n, r, n !== e);
    return true;
};

/** Q-D (plan sec. 39): an OLDER question's truth landed beneath a guess
 * (`laneWrite` held it — another action's landing is not this guess's
 * answer). The guess's edit is re-based over it: the truth frame with the
 * guess's own keys over it; a keyed list's order from the guess, rows from
 * the truth by key (a guessed row the truth has is the truth's object — the
 * row target adopted it, its proxy survives), rows the guess added — or
 * carried from another action's guess — kept (their landings judge them),
 * truth rows the guess lacks appended unless it removed them. A row the
 * guess only carried over from the truth it was written on is that truth's,
 * not the guess's: the new truth decides it, so after a new question the
 * old list's rows never show beside the new one's (F-1, D-35, #4482;
 * `removed`). Unkeyed lists compose positionally — the guess stands as
 * written (the documented residue). A current truth re-bases, the same way,
 * a guess written while its question was being asked (`g` null, #4525):
 * asked before the guess, it is the older question; its rows are held
 * beneath the guess. True: the lane holds the landing (`laneWrite`). */ GlobalQueue.In = (e, n, t) => {
    const r = e.O;
    // Not a store's: the lane judges the write (`supersede`).
        if (r === undefined) return false;
    if (e.Xe !== $PATCH) {
        // The deep witness's guess: a bump beneath a shown keyed arrangement
        // guess folds that lane (D-34), else the lane judges it.
        if (t) return true;
        const e = kgOf(r)?.find(e => e.l & CONFIG_OVERRIDE && e.removed);
        return e !== undefined && e.g !== null ? fold(e, n) : false;
    }
    const o = latestOf(e);
    // A current truth: folded beneath a shown keyed arrangement guess (D-34).
    // Not folded (an object's guess, a lane that has not shown): held beneath
    // the guess for the hold's end, as a leaf's landing was — the screen keeps
    // the guess until the commit (D-34; #4690, the outside re-ask).
        if (!t && e.g !== null) return fold(e, n);
    // The truth of an action whose own guess is on this carrier, entangled
    // with a later one's (same slot, D-33): the hold's own staging, beneath
    // the guess until the hold commits — none of it shows before (#4874).
        if (t && e.qs?.has(question)) return true;
    // The truth answers another carrier's guess (`by`, its rows): the rows
    // this guess carried over from that guess are judged by it — its own
    // landing — so a placeholder it added never shows beside the row its
    // answer wrote (#4897). Entangled with this guess's hold (an action that
    // read that guess, D-33, #4788), that truth is the hold's own staging, as
    // a same-carrier one above: none of it shows before the hold commits.
        let i;
    if (t) for (const n of kgOf(r)) if (n !== e && n.qs?.has(question)) {
        if (sameAction(txOf(n), txOf(e))) return true;
        i = n.g;
    }
    // An older question's truth, or the #4525 re-base: the guess's edit over it.
        if (!Array.isArray(n) || !Array.isArray(o)) return true;
    const f = r.rr?.key;
    if (!f) return;
    const s = new Map;
    for (let e = 0; e < n.length; e++) {
        const t = n[e];
        const r = isWrappable(t) ? f(t) : undefined;
        if (r !== undefined && !s.has(r)) s.set(r, t);
    }
    const l = new Set;
    const u = e.removed ?? new Set;
    const a = [];
    // From the guess as written, not an earlier re-base's composition: a
    // derive's landing writes key by key, and a truth row one partial write
    // showed must not stay appended once the landing drops it (#4441).
    // The #4525 re-base (`g` null): the lane's rows are the new question's
    // answer now; its work reads that question's hold as the hold's own
    // flush does (core.ts `frameRead`, F-1, #4536).
        if (e.g === null) txOf(e).over = r.rr.node.T.Re;
    const c = e.g ??= o.map(arrangementRow, r.rr);
    for (let e = 0; e < c.length; e++) {
        const n = c[e];
        const t = isWrappable(n) ? f(n) : undefined;
        const o = t !== undefined ? s.get(t) : undefined;
        if (o !== undefined) {
            // The truth's row under the guessed one's key: the guessed row's
            // target adopts it (the same proxy from here; its leaves tell its
            // readers what changed), as the derive's draft pairs new rows under
            // guessed ones (`pairRows`).
            const e = isWrappable(n) ? lookupTarget(unwrapValue(n), r.rr) : undefined;
            if (e !== undefined && lookupTarget(o, r.rr) === undefined) reconcileState(o, e.px, f, true);
            a.push(o);
            l.add(t);
        } else if (!u.has(t) && !i?.includes(n)) a.push(n);
    }
    for (let e = 0; e < n.length; e++) {
        const t = n[e];
        const r = isWrappable(t) ? f(t) : undefined;
        if (r !== undefined && !l.has(r) && !u.has(r)) a.push(t);
    }
    // Every index of the composition is the guess's from here (a new row at
    // an index, the length): the record covers the frame.
        const d = e.wk ?? (e.wk = new Set);
    const O = Math.max(a.length, o.length, committed(r).length);
    for (let e = 0; e < O; e++) d.add(String(e));
    d.add("length");
    const p = txOf(e);
    setLaneValue(e, p, a);
    // The composition changed what the lane shows (a truth row's content
    // under a guessed key, a row appended): the carrier's readers re-derive
    // as the lane's work (`laneDirty`; the leaf's `laneSet` did).
        if (!sameRows(o, a, r.rr)) laneDirty(e, p);
    return true;
};

/** Two row lists show the same through the proxies: each slot the same
 * target (a row the guessed one's target adopted is the same proxy) or
 * the same primitive. */ function sameRows(e, n, t) {
    if (e.length !== n.length) return false;
    for (let r = 0; r < e.length; r++) {
        const o = unwrapValue(e[r]), i = unwrapValue(n[r]);
        if (isWrappable(o)) {
            const e = lookupTarget(o, t);
            if (e === undefined || e !== lookupTarget(i, t)) return false;
        } else if (!isEqual(o, i)) return false;
    }
    return true;
}

/** D-34 for a keyed arrangement guess: rows by key, not positions. A
 * current truth that differs folds into the hold beneath it instead of
 * voiding the lane: the guess stays the lane's value and the screen's until
 * the hold commits, where rows match by key, and the graph outside the lane
 * re-derives from the truth now (`foldedRead`). A confirmation is silent
 * (A17): only truth readers are told. True: the lane stages the truth
 * beneath (`laneWrite`). */ function fold(e, n) {
    const t = txOf(e);
    // A guess the screen never showed is void at the correction (D-34); a
    // keyed arrangement guess's carrier carries its removed-keys set.
        if (!t.Ue || !e.removed) return false;
    const r = latestOf(e);
    if ((!e.ye || !e.ye(r, n)) && !GlobalQueue.sn(r, n)) t.folded = foldedRead; else GlobalQueue.tn?.(t);
    // A folded lane's `latest()` serves the truth beneath the guess
    // (`foldedRead`): every truth landing there is news to its readers, a
    // confirming one too — they re-derive from it (D-34, F-6).
        if (t.folded) insertSubs(e);
    joinFuture(resolveTx(t._parent ?? t));
    return true;
}

/** A folded lane's guess read by a deriving pass outside the lane, tracked
 * or not (`untrack` is about dependencies, not about which world a pass
 * derives from), or by `latest()` (`c` null): the hold's world — the truth
 * staged beneath the guess, else the value it covered — and the pass joins
 * the hold. Lane passes and the screen keep the guess: held truth is masked
 * from lane passes only (#3164). The store's own derive reconciles its
 * landings against the arrangement the guesses made: it is the author, not
 * a reader. */ function foldedRead(e, n) {
    if (e !== null) {
        if (!(n.l & CONFIG_GUESS) || passLane !== null || e.i === EFFECT_RENDER || e.l & CONFIG_CHILDREN_FORBIDDEN || n.O?.rr?.node === e) return NOT_PENDING;
        const t = txOf(n);
        joinPassAt(e, resolveTx(t._parent ?? t));
    }
    if (n.ue !== NOT_PENDING) return n.ue;
    return covered(n);
}

/** D-34 for a per-key guess (#4329): the derive's draft wrote new objects
 * over a target holding guessed rows (`d.length = 0; d.push(...rows)`, or
 * `d.issues = rows`). The container on the way down to each guessed row
 * reconciles the new object at its slot — in a keyed list, the one with
 * the row's key — exactly as a returned landing would: rows match by key,
 * a guessed row keeps its target, and its leaves are judged against their
 * guesses. With no guess out, rows nothing guessed are left as the draft
 * wrote them. */ function pairRows(e, n, t) {
    const r = e.rr;
    const o = r.key;
    const i = Array.isArray(n);
    // One rule (Q-D, plan sec. 31.7): a row with a target keeps it under the
    // landing's new object at its slot — in a keyed list, the one with its
    // key (an unkeyed list pairs nothing). The target adopts the object; a
    // row another store lent keeps its proxy in the slot.
        let f;
    const pair = (e, s, l) => {
        if (i && f === undefined) {
            f = new Map;
            n.forEach((e, n) => {
                const t = isWrappable(e) && o ? o(e) : undefined;
                if (t !== undefined && !f.has(t)) f.set(t, n);
            });
        }
        // A returned landing leaves a row another store lent alone (its target
        // here is chained over that store's proxy, §7b): the diff against the
        // view keeps it by key, and reconciling the answer into it ties the
        // lender's row to this landing, so two stores that lent each other rows
        // waited on each other's flight forever (#4928).
                if (t && s[$TARGET].ch) return;
        const u = i ? f.get(e) : e;
        const a = u === undefined ? undefined : n[u];
        // A new object only: not the row's own, nor one another row serves
        // (pairing it would alias the two).
                if (isWrappable(a) && !lookupTarget(a, r)) {
            reconcileState(a, s, o, true);
            if (l) n[u] = s;
        }
    };
    // A guessed row, or its ancestor directly under `t`.
        for (let n of r.cn ?? []) {
        while (n.u !== e) if ((n = n.u) === null) break;
        if (n !== null) pair(i ? o?.(n.v) : n.pk, n.px);
    }
    if (!o || !i) return;
    // While guesses are out, every committed row, not only a guessed one
    // (#4738): a row whose guess settled can be guessed again, and the setter
    // writes the committed row's target.
        if (r.cn?.size) for (const n of committed(e)) {
        const e = isWrappable(n) ? lookupTarget(n, r) : undefined;
        if (e !== undefined) pair(o(n), e.px);
    }
    // A row another store's guess added as it is (that store's proxy, not a
    // copy): a guess of that store's still out on the row shows on through
    // its own carrier until its action ends (F-11, #4902). A returned landing
    // (`viewed`) is diffed against the view, which already holds the lent row
    // by key: its slot stays this store's row (#4918).
        if (!t) kgOf(e)?.forEach(e => {
        const n = guessedValueOf(e);
        if (Array.isArray(n)) for (const e of n) {
            const n = e?.[$TARGET];
            if (n?.px === e && n.rr !== r) pair(o(e), e, true);
        }
    });
}

/** #4788 (D-33): an action read `key` of `t` (`v`, else the writer's view
 * of it): when a live guess wrote that key, a guess the action writes next
 * derives from it (core lanes.ts `readGuesses`). A row reached through a
 * guessed index, or the length of a guessed arrangement, is navigation, not
 * a value the write is computed from: the row's own keys say what was read
 * of it. Nor is the keyed diff a returned array is written by (`diffing`):
 * the user's own reads are the setter's. */ let diffing = false;

/** #4812: a record a live guess put in place (a replaced child), with the
 * record it replaced: what the setter reads under it derives from that
 * guess, however deep — where it differs from what the replaced record
 * holds (a rewritten row's unchanged `id` is navigation, #4724). */ const placed = new WeakMap;

function noteRead(e, n, t) {
    if (!actionDepth || diffing) return;
    const r = guessFor(e, n, false, false);
    const o = r && r.l & CONFIG_OVERRIDE ? null : placed.get(e.v);
    if (!o && !(r && r.l & CONFIG_OVERRIDE)) return;
    const i = o ? o[0] : txOf(r), f = unwrapValue(o ? o[1]?.[n] : e.v[n]), s = unwrapValue(t ?? (r ? carrierValue(guessedValueOf(r), n) : e.v[n]));
    if (isWrappable(s)) s !== f && placed.set(s, [ i, f ]); else if (!o || !Object.is(s, f)) readGuesses.push(i, question);
}

/** #4812: presence (`in`) and enumeration in a setter's draft read the key
 * set a live guess changed — a key it added or deleted. */ function noteShape(e, n) {
    const t = actionDepth && !diffing ? kgOf(e) : null;
    if (t !== null) for (let r = t.length - 1; r >= 0; r--) {
        const o = t[r];
        if (o.l & CONFIG_OVERRIDE && (n !== undefined ? o.del?.has(n) || o.wk?.has(n) && !(n in e.v) : o.del?.size || [ ...o.wk ?? [] ].some(n => !(n in e.v)))) readGuesses.push(txOf(o), question);
    }
}

installOptHooks({
    pair: pairRows,
    draft: optimisticDraft,
    writes: notifyOptimisticWrites,
    view: guessView,
    reconcile: (e, n, t) => {
        diffing = true;
        reconcileDraft(e, n, t);
        diffing = false;
    },
    sweep: sweepOverlaid,
    // The carrier arms (store.ts calls them for an optimistic family's target
    // outside a draft and the write override): a key a live guess changed
    // reads through the carrier, tracked — the lane's seat, a verdict
    // window's judgment of the guess (A17/A24) — and the key's leaf stays the
    // reader's subscription point (silent while the guess covers it: the next
    // guess on the key, on a carrier of its own, reaches the reader through
    // it, `readersOf`). Presence and descriptors are the carrier frame's;
    // enumeration reads the newest carrier, tracked.
    read: (e, n) => {
        const t = guessFor(e, n);
        const r = getObserver();
        if (t === null) {
            // A key an arrangement guess only borrowed, its lender gone, reads the
            // truth (#4784), but that guess still orders the rows read beside it:
            // its end must reach this reader, as when it served the key (#4897).
            if (r !== null) kgOf(e)?.forEach(e => e.borrowed?.has(n) && link(e, r));
            return NOT_PENDING;
        }
        const o = read(t);
        const i = r !== null ? getNode(e, n) : e.n?.[n];
        if (r !== null) link(i, r);
        const f = carrierValue(o, n);
        if (r === null) noteRead(e, n, f);
        return serveKeyValue(e, n, f, false, i);
    },
    drafted: (e, n, t) => t ? noteShape(e, n) : noteRead(e, n),
    has: (e, n) => {
        const t = guessFor(e, n, true);
        if (t === null) return undefined;
        const r = getObserver();
        if (r !== null) link(getHasNode(e, n), r);
        return n in read(t);
    },
    descriptor: (e, n, t) => {
        const r = guessFor(e, n, true);
        if (r === null) return undefined;
        const o = getObserver();
        if (o !== null) link(getHasNode(e, n), o);
        const i = read(r);
        if (!(n in i)) return null;
        const f = i[n];
        const s = n === "length" && Array.isArray(i);
        return {
            value: f,
            writable: true,
            enumerable: !s,
            configurable: !(s && t)
        };
    },
    frame: e => {
        const n = newestGuess(e);
        return n === null ? NOT_PENDING : read(n);
    },
    // A key a live guess changed: its leaf is silent at a landing — its
    // readers read the carrier, whose judgement of the landing tells them (a
    // correction through the carrier, a confirmation silently, A17).
    covers: (e, n, t) => guessFor(e, n, t, false) !== null,
    // A landing reaches each live carrier as the truth it is judged against
    // (`laneWrite`: confirmed beneath, re-based, folded, or a correction) —
    // the frame as landed, not the backing by reference: a later batch writes
    // the backing in place, and a truth staged beneath a guess (a correction
    // held for the hold's end) must stay the answer it was.
    judge: (e, n) => {
        const t = kgOf(e);
        if (t === null) return;
        const r = Array.isArray(n) ? n.slice() : {
            ...n
        };
        for (let e = 0; e < t.length; e++) if (t[e].l & (CONFIG_OVERRIDE | CONFIG_HELD)) setSignal(t[e], r);
    },
    // A new question (not a `refresh()`) asked over guesses an earlier frame
    // opened, which answer the old one (D-34, #4336). Stamped on `fam._asked`
    // with this flush's `clock`, so it lapses when the flush ends: `clock + 1`
    // from mainline over a live guess — a frame still reading a guess at the
    // seam shows it beside the new input, so the flush holds with the guess's
    // action (`park`); `~clock` from a lane with no guess of its own, or with
    // no live guess left — the lane waits for its answer (F-6), as for a
    // guessed `createOptimistic` (lanes `laneStage`). The guess's own lane
    // asks nothing new and stamps nothing.
    // A `refresh()` re-asks the guesses' own question (D-34 ¶3) — the
    // derive's own, or every source it waits on re-asked (`reasked`: a
    // refreshed memo dirties a sync derive ahead of its own refresh, #4801):
    // a guess
    // stands in for its flight (A17), so a leaf lane work reads is kept off
    // the wake — the lane shows its guesses and the flight holds the lane's
    // parent, not the lane (F-11, #4656).
    ask: e => {
        if (!e.cn?.size) return;
        if (reasked(e.node)) return e => e.o.u.T?.Re?.P;
        // Stamped with the flush (`clock`): it lapses when the flush ends.
                let n = ~clock;
        for (const t of e.cn) {
            const e = laneNodeOf(t);
            if (e) {
                if (passLane === null) n = clock + 1; else if (sameAction(txOf(e), passLane)) return;
            }
        }
        if ((e.dn = n) > 0) joinFuture(null);
    },
    // Over a new question the guesses do not answer (`_asked`): from mainline
    // only a target carrying a guess stands in — a read of one with none is an
    // ordinary pending read (F-2), so a keyed store's other keys wait for their
    // answer, not for the guess's action; in a lane with no guess of its own,
    // or with no live guess left, none does (F-6) — whoever asked: a reader
    // that joined another action's lane (it reads that action's guess
    // elsewhere) over mainline's new question waits for its answer, so a
    // `Loading on` region remounted under it shows its fallback (#4929).
    // A reader already observing the flight (linked) keeps observing it:
    // served the guess, a re-run (the guess's own, the seam) would drop its
    // link and release the hold beside a guess over the previous answer
    // (F-1, #4536) — the ordinary read serves its committed frame or keeps it
    // pending. So does a reader in the frame's hold pending on a flight a list
    // guess was written over (`g` null, the #4525 mark; D-34: the guess does
    // not answer it): a render effect is read here, as `pullFamily`'s own read
    // would, since the frame's guard there returns early for an unlinked one.
    // A derivation a pending render effect reads (#4561), through any chain
    // of derivations (`held`), keeps observing a held flight as a stale reader
    // of its hold — linked, re-derived at the landing, so the hold still sees
    // its frame (`blockedBy`) — and is served the guess like lane work (a
    // `<For>` keeps showing it, #4525), its every read this pass (D-34 ¶3:
    // however the reads are grouped); a flight no transaction holds (a
    // `Loading on` fallback) it reads plainly.
    stands: e => {
        const n = e.rr;
        const t = n.cn;
        const r = n.dn;
        const o = n.node;
        const i = context;
        if (!t?.size || r === clock + 1 && (passLane === null ? !t.has(e) : ![ ...t ].some(e => (e = laneNodeOf(e)) && sameAction(txOf(e), passLane))) || r === ~clock) return false;
        // Linked by this pass's hold (below): its other reads keep the guess.
                const f = heldC === i && heldAt === clock;
        if (linkedTo(i, o)) return f && heldV > 1;
        if (kgOf(e)?.some(e => e.g === null) && i?.m & STATUS_PENDING) {
            if (i.i === EFFECT_RENDER) read(o); else if (f ? heldV : (heldC = i, heldAt = clock, 
            heldV = held(i))) o.T?.Re ? (link(o, i), staleReader(i, txOf(o)), heldV = 2) : read(o);
        }
        // #4786: guesses of one action standing in (A17) for a `refresh()` that
        // another action holds (D-34 ¶3: the same question) still leave the
        // pass derived from that refetch: a derivation reading them is a stale
        // reader of it (A15). Linked, the refetch's hold keeps waiting for it
        // (F-11) and it re-derives at the landing. A render effect reads its
        // memo's frame; a new question is the branches above.
         else if (i?._ !== undefined && i.i !== EFFECT_RENDER && o.T?.Ve && o.T.Re && !ownFlight(t, txOf(o))) link(o, i), 
        staleReader(i, txOf(o));
        return true;
    },
    // Mainline hold (`clock + 1`): the flush joins the action of each guess a
    // reader still reads at the seam; a reader that moved off it (a keyed
    // store's other key) holds nothing.
    park: e => {
        for (const n of overlaidFamilies) if (n.dn === clock + 1) for (const t of n.cn) {
            const n = laneNodeOf(t, true);
            if (n) merge(txOf(n)._parent, e);
        }
    }
});

/** A derivation in the frame's hold (scheduler.ts `blockedBy`): one a
 * pending render effect reads, through any chain of pending derivations
 * (#4561: memo -> memo -> binding, `mapArray` -> memo -> binding). Each
 * derivation is walked once (`seen`): a diamond graph stays linear. */ function held(e, n = new Set) {
    for (let t = e.o; t; t = t.t) {
        const e = t.u;
        if (e.m & STATUS_PENDING && (e.i === EFFECT_RENDER || !e.i && !n.has(e) && (n.add(e), 
        held(e, n)))) return true;
    }
}

/** The last reader judged (`heldC`) this pass (`heldAt`, the pass's `clock`)
 * and its verdict (`heldV`: 2 once this pass linked it): a reader's key reads
 * come in one run, so each does not re-walk its subscribers. */ let heldC, heldAt, heldV;

/** A guess in `o` (live or held) belongs to the transaction `f` that holds
 * the flight it stands in for. */ function ownFlight(e, n) {
    for (const t of e) for (const e of [ ...kgOf(t) ?? [], t.dk ]) if (e?.T?.Re) {
        const t = txOf(e);
        if (resolveTx(t._parent ?? t) === n) return true;
    }
    return false;
}

function sameAction(e, n) {
    return resolveTx(e._parent ?? e) === resolveTx(n._parent ?? n);
}

/** Families with a target carrying a lane value — swept at the flush's
 * commit (`sweepOverlaid`): a target whose carrier is no lane's any more
 * leaves its family's `_overlaid` and drops the carrier. (Not on the read
 * path: a guess is pending between the setter and the seam, and a read in
 * between must not take the target for clean.) */ const overlaidFamilies = new Set;

function markOverlaid(e) {
    const n = e.rr;
    (n.cn ??= new Set).add(e);
    overlaidFamilies.add(n);
}

function sweepOverlaid() {
    if (overlaidFamilies.size === 0) return;
    for (const e of overlaidFamilies) {
        const n = e.cn;
        for (const e of n) if (!laneNodeOf(e)) {
            n.delete(e);
            const t = kgOf(e)?.filter(e => e.l & CONFIG_HELD || e.ue !== NOT_PENDING);
            if (!t || t.length === 0) carriers.delete(e); else carriers.set(e, t);
        } else if (kgOf(e) !== null) {
            // A carrier that is no lane's and no hold's (a corrected guess waits
            // for its hold's landing, A18 (c)) is done.
            const n = kgOf(e).filter(e => e.l & (CONFIG_OVERRIDE | CONFIG_GUESS | CONFIG_HELD) || e.ue !== NOT_PENDING);
            if (n.length === 0) carriers.delete(e); else carriers.set(e, n);
        }
        if (n.size === 0) overlaidFamilies.delete(e);
    }
}

/** The node of `t` carrying a lane value, if any — the guess carrier or the
 * deep witness (`read`: one a reader's last pass read). */ function laneNodeOf(e, n) {
    for (const t of [ ...kgOf(e) ?? [], e.dk ]) if (t !== null && t.l & CONFIG_OVERRIDE) {
        if (!n) return t;
        for (let e = t.o; e !== null; e = e.t) if (e.Ne === e.u.de) return t;
    }
}

function createOptimisticStore(e, n, t) {
    const r = typeof e === "function";
    // A derived store's family (projection.ts): the derive's status reaches
    // its leaves' readers as for `createStore(fn)` (F-12, #4437).
        const o = createProjectionInternal(r ? e : null, r ? n : e, r ? t : n, true);
    return [ o, e => storeSetter(o, e) ];
}

export { createOptimisticStore, notifyOptimisticWrites, sweepOverlaid };