import { CONFIG_AUTO_DISPOSE, CONFIG_VERDICT, STATUS_PENDING, STATUS_ERROR, STATUS_UNINITIALIZED } from "../core/constants.js";

import { computed } from "../core/core.js";

import { NotReadyError } from "../core/error.js";

import { landBySetter, handleAsync, addPendingSource, propagateStatus, forEachDependent, settlePendingSource } from "../core/async.js";

import { enqueueSub } from "../core/heap.js";

import { getOwner, isDisposed } from "../core/owner.js";

import { observeFlightHook, passLane, setPassLane, question, setQuestion, laneServedHook, insertSubs, schedule } from "../core/scheduler.js";

import { optHooks } from "./hooks.js";

import { reconcileState } from "./reconcile.js";

import { wrap, storeSetter, runDirect, protoOf } from "./store.js";

import { setLookup } from "./target.js";

import { $TARGET, markRawIngest, isWrappable, setWriteOverride } from "./types.js";

/**
 * Store — projections (plan sec. 31, S3; INTERNALS §7/§7b).
 *
 * A projection is a computed store: the derive runs inside a computed — the
 * **firewall** — whose pass writes the store through a draft; its output is
 * adopted through the reconcile channel (replace-mode root: entity changes
 * merge in place, the root proxy is stable for life). Children wrap into
 * the projection's own FAMILY (writes land here, never in a source family).
 *
 * The firewall exists so writes from inside a computation are safe (the
 * derive owns its leaves); the price is the ordering obligation: before any
 * value is served through the projection, the derive is brought up to date
 * — **without** the reader subscribing to it (that is the propagation
 * barrier). The store's traps do that pull (`pullFamily`, store.ts): core's
 * `read()` of the firewall with no link — freshness, the status gate
 * (uninitialized / pending / errored) and the reader's height, by the
 * rules a memo's read follows. No per-leaf back-pointer, nothing in core.
 */
/**
 * Wrap a store proxy as a projection DRAFT: every operation carries the write
 * override (the derive is the author — its ops must not hit the status gate,
 * even in a continuation after an `await`/`yield` where the sync write scope
 * has closed).
 *
 * FAKE TARGET, not the store proxy itself (#3060): after a proxy trap
 * returns, the engine runs spec invariant validation against the proxy's
 * TARGET — [[OwnPropertyKeys]] after ownKeys, [[GetOwnProperty]] after
 * set/getOwnPropertyDescriptor/defineProperty. With the store proxy as
 * target those checks re-enter the store's traps OUTSIDE the override
 * bracket, so `Object.keys(state)` in a derive continuation fired the gate.
 */ function wrapDraft(e, t, r, o, n, s) {
    const mutate = e => {
        if (!t()) return true;
        setWriteOverride(true);
        // A continuation's write is the flight's answer: it asks the flight's
        // question (A18 provenance — a landing beneath a newer guess is held,
        // not a correction; lanes.ts `laneWrite`).
                const o = question;
        if (s !== undefined) setQuestion(s());
        try {
            r ? r(e) : e();
        } finally {
            setWriteOverride(false);
            setQuestion(o);
        }
        if (n) n();
        return true;
    };
    const read = e => {
        setWriteOverride(true);
        try {
            return e();
        } finally {
            setWriteOverride(false);
        }
    };
    const i = {
        get(i, a) {
            const l = read(() => e[a]);
            return !o && typeof l === "object" && l !== null && a !== $TARGET ? wrapDraft(l, t, r, false, n, s) : l;
        },
        has: (t, r) => read(() => r in e),
        set: (t, r, o) => mutate(() => {
            e[r] = o;
        }),
        deleteProperty: (t, r) => mutate(() => {
            delete e[r];
        }),
        ownKeys: () => read(() => Reflect.ownKeys(e)),
        getPrototypeOf: () => protoOf(e),
        getOwnPropertyDescriptor(t, r) {
            const o = read(() => Reflect.getOwnPropertyDescriptor(e, r));
            if (o) o.configurable = true;
            return o;
        },
        defineProperty: (t, r, o) => mutate(() => {
            Reflect.defineProperty(e, r, o);
        })
    };
    const a = new Proxy(Array.isArray(e) ? [] : {}, i);
    // The wrapper stands for its store proxy wherever the derive's result
    // carries it (`wrap`, `unwrapValue`, reconcile's slot matching).
        const l = e[$TARGET];
    if (l !== undefined) setLookup(a, l);
    return a;
}

function cloneState(e, t) {
    return t ? Array.isArray(e) ? e.slice() : {
        ...e
    } : JSON.parse(JSON.stringify(e));
}

/** The derive's pass went pending (a new question in flight): every reader
 * of a leaf learns it as a reader of the derive would (A9: any leaf reports
 * the firewall's refetch) — core's own rule for a dependency going pending
 * (`notifyStatus`): a verdict or re-derive reader re-runs, any other is
 * pending derivatively on the derive (its readers hold; it re-runs at the
 * landing). The leaves do not subscribe to the derive, so the family does
 * the walk. */ function wakeFamily(e) {
    const t = e.node;
    // Pending on the derive itself, whatever source took it there (an upstream
    // memo's flight reaches it as a NotReady of that memo, #4271): the leaves
    // are not that source's dependents, so only the family's settle — which
    // retires the derive (`settleFamily`) — reaches the readers parked here. A
    // reader the landing leaves unchanged does not re-run to clear it (#4305).
        const r = new NotReadyError(t);
    e.Tt = true;
    // A leaf the optimistic engine keeps off the wake (`ask`).
        const o = optHooks?.ask(e);
    forEachFamilyNode(e, e => {
        if (e.o === null || o?.(e)) return;
        // The leaf carries the derive's pending as a propagated copy, as a memo
        // between them would: a dependent recovering one branch stays pending
        // through a leaf it still reads (core `retryReaches`), and the landing's
        // walk retires the copy first (`settleFamily` → `settlePendingSource`).
                addPendingSource(e, t);
        // Its readers: core's propagation from the leaf — a verdict or re-derive
        // reader re-runs, any other is pending derivatively on the derive and
        // queued for the frame (held with it: the frame waits on the flight
        // through them, `blocked`).
                propagateStatus(e, STATUS_PENDING, r);
    });
}

/** The derive's flight rejected: every live reader of the family
 * re-derives (its pull throws the derive's error) — but a reader that
 * already shows this very `error` has had its answer: re-run, its error arm
 * would see the one error twice (D-41: the arm runs once, at the reveal; as
 * core `propagateStatus` re-notifies no reader of an error it carries). A
 * re-run of the derive meets an upstream error again, the same object, when
 * a reader's pull retries it or a held error's commit dirties it (#4457). */ function errorFamily(e, t) {
    forEachFamilyNode(e, e => {
        if (e.o === null) return;
        forEachDependent(e, e => {
            e.m & STATUS_ERROR && e.T.U === t || enqueueSub(e);
        });
    });
    schedule();
}

/** The flight landed: the readers the wake left pending on the derive
 * settle — core's walk, from each leaf (the leaves are not the derive's
 * dependents, so its own settle walk does not reach them). */ function settleFamily(e) {
    if (!e.Tt) return;
    e.Tt = false;
    const t = e.node;
    forEachFamilyNode(e, e => {
        if (e.o !== null) settlePendingSource(e, t);
    });
    // The landing is the derive's change for a reader parked on the derive
    // itself, whatever status the landing pass began from (an errored derive
    // re-asked: a reader that met the error has no leaf to learn it from).
        insertSubs(t);
}

/** A source's status reached the derive by propagation (core
 * `propagateStatus`), its pass not re-run: the family learns it as from the
 * pass's own throw. */ function familyStatus(e, t) {
    if (e & STATUS_PENDING) wakeFamily(this);
    // No status: the source's settle walk retired the derive's pending, its
    // pass not re-run (core `settlePendingSource`) — the readers waiting on the
    // derive settle with it (F-9). A re-run would ask again: an async derive's
    // new flight is a second wait, and would run ahead of a staged rejection's
    // publish, which reaches the derive at the commit (F-12, D-41).
     else if (!e) settleFamily(this);
    // The error rides along: a reader the derive's own pass already handed it
    // (a sync derive re-run in the same flush) is not handed it again.
     else if (e & STATUS_ERROR) errorFamily(this, t);
}

function forEachFamilyNode(e, t) {
    for (const r of e.live) {
        const e = r.n;
        if (e !== null) for (const r of Reflect.ownKeys(e)) t(e[r]);
        const o = r.h;
        if (o !== null) for (const e of Reflect.ownKeys(o)) t(o[e]);
        if (r.k !== null) t(r.k);
        if (r.dk !== null) t(r.dk);
    }
}

/** The derive's pass: run `fn` against a draft of the store; commit its
 * result (sync, or each async landing) through the reconcile channel.
 * Draft validity is per run (R37): live from here until the NEXT run starts
 * — a sync derive that subscribes to an external source and pushes into
 * the draft from the callback (#3585) is the model use. */ function runProjectionComputed(e, t, r, o, n) {
    const s = getOwner();
    const i = e[$TARGET];
    const a = i.rr;
    const l = a.run = (a.run || 0) + 1;
    // A verdict reader's pass that is the hold's work reads plainly, as a
    // pending node's is (core `recompute`, #3884, D-25), so the landing it
    // computes is the one a reader held on it shows, not the screen a frame
    // reader is served (#4491; D-30, F-2): its pending is on its leaves'
    // readers (`wakeFamily`) with no landing to settle them yet — the settle
    // walk clears the derive's own pending before the landing re-runs it — or
    // a reader of the leaves reads a flight it was served the committed value
    // of (`fam.w`, store.ts `pullFamily`; this pass consumes the mark). For
    // this pass only: its reads set the posture again at its end, and a window
    // in it re-stamps it (`markVerdictReader`). Only the verdict engine makes
    // a verdict reader (`observeFlightHook`).
    // (Out of the hold's verdict lane too, where its seat put the pass — never
    // out of a guess's lane: a derivation of a guess is that lane's, F-6.)
        if (observeFlightHook && (a.Tt || a.w)) {
        a.w = 0;
        s.l &= ~CONFIG_VERDICT;
        if (passLane?._parent.Pe === passLane) setPassLane(null);
    }
    let c;
    // Open loading window (seedLoadingValue): the observable store IS commit #0
    // for the whole first flight — the derive works a detached shadow of the
    // seed so draft writes cannot tear through to readers (#2988). Every
    // commit point reconciles the shadow through the normal commit path.
        const u = s.Ie ? cloneState(i.v, i.s) : null;
    // The question a write of this run answers: inside a slice, the slice's
    // (the sync pass); in a continuation — the flight still registered — the
    // flight's (`handleAsync` stamped it with the action that asked).
        const asking = () => question || (s.T !== null && s.T.Ee !== null ? s.T.ce : 0);
    const f = wrapDraft(e, () => a.run === l && !isDisposed(s), n, i.s, 
    // A write after the run returned (a continuation, a callback) arms the
    // drain itself when no landing will.
    () => {
        if (!(s.m & STATUS_PENDING) && !s.Ie) schedule();
    }, asking);
    // The creation run commits directly (a memo's first value is its
    // `_value`); every later run — a re-derive in a flush, an async landing
    // — stages and commits with the flush like a memo's recompute.
        const S = l === 1;
    const pass = () => storeSetter(f, n => {
        c = t(u ?? n);
        const commit = (t, l = 1) => {
            // Shadow run: commit a detached snapshot, never the shadow itself
            // (adoption takes the value by identity — handing it the live
            // shadow would fuse the draft to the observable store).
            if (u && (t === undefined || t === u)) t = cloneState(u, i.s);
            if (t !== n && t !== undefined) {
                const write = () => {
                    const o = question;
                    setQuestion(asking());
                    try {
                        storeSetter(e, e => reconcileState(t, e, r, true), false);
                    } finally {
                        setQuestion(o);
                    }
                };
                o ? o(write, t) : write();
            }
            // A landing (not the sync pass's own commit): the readers held
            // pending on the derive settle — unless core keeps the derive
            // pending: lane work this pass served the committed value of a
            // flight still in the air (core.ts `recompute`, #4862). The lane
            // breaks out of the flight, the family's readers do not (F-4,
            // #4927); they settle at the flight's landing.
                        if ((!S || !(s.m & STATUS_UNINITIALIZED)) && !(laneServedHook && s.m & STATUS_PENDING && laneServedHook(s, null, true))) settleFamily(a);
            // A commit is the derive's change for the readers that observed its
            // flight (linked by `pullFamily`; a settled pull does not link), as
            // a memo's landing is for its readers: one that caught the NotReady
            // (a fallback value, not pending) re-runs too (#4070). A pass of a
            // settled derive has none (`wake` 0): a link left is a stale one a
            // held frame keeps (A30), from a reader that re-ran since. Woken by
            // every pass, a reader feeding a key the pass peeks at looped (#4342).
                        if (l) insertSubs(s);
        };
        const land = (e, t, r) => landBySetter(s, e, commit, t, r);
        const l = handleAsync(s, c, land, (e, t) => {
            // The flight rejected: the leaves' readers learn it from here —
            // they do not subscribe to the derive. Pending again (a NotReady
            // rejection): the wake, as for the pass. Errored: every live
            // reader re-derives and meets the error at its pull (memo parity,
            // #2897) — and the flush that runs them advances the clock, so a
            // later tracked re-read may retry (core `read`). A held derive's
            // error is staged (D-41, #4489): a reader in the hold derives from
            // the staging and stages its own error with the frame; one served
            // the screen (an untracked read included) reads the committed
            // leaves until the frame reveals.
            if (S && s.m & STATUS_UNINITIALIZED) return;
            if (t) wakeFamily(a); else errorFamily(a, e);
        });
        // (The status is the one before this pass: `recompute` sets it after.)
                if (!s.Ie) commit(l, s.m & (STATUS_PENDING | STATUS_ERROR));
    }, false);
    try {
        S ? runDirect(pass) : pass();
    } catch (e) {
        // A flight went up (NotReady out of the pass — the derive's own, with
        // the derive as its source): the leaves' readers learn it from here —
        // they do not subscribe to the derive.
        // A synchronous error the same way (#3956: a rerun that finds the
        // entry failed throws): every live reader re-derives and meets it.
        if (!S) e instanceof NotReadyError ? wakeFamily(a) : errorFamily(a, e);
        throw e;
    }
    // A pass that returned with a flight up (an iterator's sync first yield):
    // the wake, as for the throw. Not a sync pass that superseded one — the
    // status it reads is the dead flight's, cleared when this pass settles
    // (#3181: a reader the settle walk had just released, re-parked for good).
        if (s.m & STATUS_PENDING && s.T.Ee !== null && !s.Ie && !S) wakeFamily(a);
    return s;
}

/** A derived store's family and root (`opt`: an optimistic store's, its
 * derive `fn` optional — the plain form has none). */ function createProjectionInternal(e, t, r, o) {
    // Seedless (`fn` takes no draft): the root starts as an empty record. It is
    // a draft like any seed, never observable before the first landing (A25).
    t ||= {};
    const n = r?.key === undefined ? "id" : r.key;
    const s = {
        map: new WeakMap,
        node: null,
        live: new Set,
        st: familyStatus,
        It: !!r?.shallow,
        opt: o,
        // The row key, for a child paired after the adoption (store.ts
        // `adoptChild`, #3712) as the adoption pairs them.
        key: typeof n === "function" ? n : n === null ? null : e => isWrappable(e) ? e[n] : undefined
    };
    const i = wrap(t, null, null, s);
    if (s.It) {
        i[$TARGET].s = true;
        markRawIngest(t);
    }
    let a;
    if (r?.seedLoadingValue) a = {
        loadingValue: undefined
    };
    if (e) {
        const t = computed(() => {
            if (!s.node) s.node = getOwner();
            runProjectionComputed(i, e, n);
        }, a);
        t.l &= ~CONFIG_AUTO_DISPOSE;
        s.node = t;
        // The derive's leaves are not its dependents: a mark's repoll
        // (affects-marks.ts) and a source's status (core `propagateStatus`,
        // `familyStatus`) reach their readers through this.
                t.rr = s;
    }
    return i;
}

function createProjection(e, t, r) {
    return createProjectionInternal(e, t, r);
}

/** `createStore(fn, seed, options)`: a projection with a user setter. */ function createStoreDerived(e, t, r) {
    const o = createProjectionInternal(e, t, r);
    return [ o, e => storeSetter(o, e) ];
}

export { createProjection, createProjectionInternal, createStoreDerived, runProjectionComputed };