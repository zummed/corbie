import { REACTIVE_JOINED, CONFIG_VERDICT, STATUS_UNINITIALIZED, REACTIVE_PROBED, REACTIVE_WINDOW, WINDOW_KINDS, REACTIVE_MISSED_WAKE, REACTIVE_PROBE_UNANSWERED, CONFIG_CHILDREN_FORBIDDEN, NOT_PENDING, CONFIG_HELD, CONFIG_IN_SNAPSHOT_SCOPE, STATUS_ERROR, NO_SNAPSHOT, REACTIVE_SNAPSHOT_STALE, CONFIG_OVERRIDE, CONFIG_GUESS, STATUS_PENDING, EFFECT_RENDER, CONFIG_STAGED_ERROR, REACTIVE_DISPOSED, REACTIVE_LANE_READ, REACTIVE_STAGED_READ, CONFIG_HELD_FRAME, REACTIVE_MANUAL_WRITE, REACTIVE_FRAME_READ, CONFIG_ASYNC, CONFIG_OWN_PROBE, CONFIG_FRAME, CONFIG_STAGED, CONFIG_REDERIVE, EFFECT_TRACKED, REACTIVE_REASK, CONFIG_INPUTS_PUBLISHED, REACTIVE_IN_HEAP, REACTIVE_DIRTY, REACTIVE_CHECK, REACTIVE_LANE_DIRTY } from "./constants.js";

import { setVerdict, context, creatorPass, tracking, adopter, creatorReads, setAdopter, verdict, prepareComputed, ext, pullComputed, snapshotCaptureActive, joinPass, markLateLinker, spectating, unflushedValue, bornInto, isSame, tickBase } from "./core.js";

import { isThenable, releaseFlightTeardown, handleAsync, notifyStatus, settlePendingSource } from "./async.js";

import { NotReadyError } from "./error.js";

import { link } from "./graph.js";

import { screenOf, keptGuess, ahead, verdictLane, isVerdict, waiting, heldMountUnder } from "./lanes.js";

import { latestOf, covers } from "./truth.js";

import { enqueueSub, deleteFromHeap } from "./heap.js";

import { flushTransaction, dropFlushTx, clock, txOf, GlobalQueue, passLane, sameLane, resolveTx, staleReader, blocked, globalQueue, joinFuture, installVerdictHooks, setPassLane, stagedReaders, guessing, pendingNodes, heldRewrites, dirtyQueue, queuePendingNode, insertSubs, list, schedule } from "./scheduler.js";

// Verdicts: `latest(fn)` and `isPending(fn)`, the display-ahead reads (F-6,
// D-22, D-23, D-25). A verdict reader runs in its holder's verdict lane:
// shown now, re-derived at the landing.
/** The seam decides this reader's answer: it re-runs if the flush parks. */
function watchVerdict(e, t = false) {
    if (t) e.R |= REACTIVE_PROBE_UNANSWERED;
    stagedReaders.push(e);
}

/** Seat the pass in the holder's verdict lane, unless it is already lane
 * work or the hold's own work (D-25: a pass the hold built reads plainly). */ function route(e, t) {
    if (!passLane && !(e.R & (REACTIVE_JOINED | REACTIVE_STAGED_READ)) && !(e.l & CONFIG_HELD_FRAME && bornInto(e, resolveTx(t)))) setPassLane(verdictLane(t));
    e.R |= REACTIVE_LANE_READ;
}

/** A verdict reader's plain read of a flight, served the committed value,
 * still holds the frame on that flight (D-23: `[isPending(x), x()]` holds
 * like `x()`; D-25). A flight nothing held yet is queued here, so the hold
 * has an entry for it (#3458; without it the reader re-derived every flush,
 * fuzzer F12). True when the reader is served the committed value: not
 * when that would tear against inputs already on screen (F-1, #3305), nor
 * for a reader born into the flight's hold (#4010). */ function observeFlight(e, t) {
    // Either read order holds the frame, a routed render effect too (D-25, S-52).
    if (t.l & CONFIG_INPUTS_PUBLISHED || t.l & CONFIG_HELD && bornInto(e, txOf(t))) return false;
    // A derived store's derive with no reader is no display (F-3).
        if (globalQueue.Y && e.rr?.live.size !== 0) {
        joinFuture(null);
        if (!(t.l & CONFIG_HELD)) queuePendingNode(t);
        ahead.splice(ahead.indexOf(e) >>> 0, 1);
        verdictRead(e, flushTransaction, true);
    }
    return true;
}

/** Which window is open, and whether an `isPending` probe found anything. */ let latestActive = false;

let probing = false;

let probeFound = false;

/** A verdict read of a held node. A probe re-runs at the landing because
 * nothing notifies its flip; `latest` needs no re-run, the landing reaches
 * it through its link. */ function verdictRead(e, t, r) {
    route(e, t);
    if (r) staleReader(e, t); else if (probing && t.fn[t.fn.length - 1] !== e) t.fn.push(e);
}

/** A verdict read of a node staged this flush but not held: whether it will
 * be held is the seam's decision. Before anything has joined a hold, the
 * reader re-runs if the flush parks, so a plain sync write never reads as
 * pending (F-9). `latest` opens no transaction for its answer (#4365). */ function provisionalVerdict(e, t) {
    if (flushTransaction) {
        if (probing && t) probeFound = true;
        verdictRead(e, flushTransaction);
        if (probing && e.R & REACTIVE_STAGED_READ) watchVerdict(e);
        return;
    }
    // Unanswered: a later plain read may route this pass, and the seam must
    // not take that as the probe's answer.
        watchVerdict(e, !probing || t);
}

/** A re-ask of the answered question is quiet (F-9); so is a node pending
 * only through quiet flights. */ function quietPending(e) {
    const t = e.T;
    if (!t) return false;
    if (t.se) {
        for (const e of t.se) if (!e.T?.Ve) return false;
        return true;
    }
    return t.Ve;
}

/** `isPending` of a held node with a committed value (F-9, D-23): a flight
 * is not final unless quiet; a staging is not final unless it is a quiet
 * re-ask, equal to the shown (non-error) value, or read inside `latest`,
 * whose view is the proposal. Truthy when not final. */ function heldNotFinal(e) {
    if ((e.m & STATUS_PENDING) !== 0 && (!quietPending(e) || heldBeneath(e))) return true;
    return e.ue !== NOT_PENDING && !latestActive && !e.T.Ve && (e.T.Te || !e.ye || !e.ye(e.le, e.ue));
}

/** While a re-ask flies, the error is still `latest`'s newest answer (D-22, D-41). */ function aheadError(e) {
    const t = e.m & STATUS_PENDING && (e.T?.Ft || e.T?.Te);
    if (t) throw t;
}

/** A source holds an answer to a new question, so a quiet flight does not make `el` final (F-9). */ function heldBeneath(e) {
    for (let t = e.C; t !== null; t = t.G) {
        const e = t.S;
        if (e.l & CONFIG_HELD && e.ue !== NOT_PENDING && !e.T?.Ve && !isSame(e, e.le, e.ue)) return true;
    }
    return false;
}

function pendingVerdict(e, t) {
    if (probing && (!quietPending(t) || heldBeneath(t))) probeFound = true;
    if (e && tracking) {
        if (t.l & CONFIG_HELD) verdictRead(e, txOf(t)); else if (globalQueue.Y) {
            // `latest` of a flight is the committed value in every world (D-22),
            // so the reader joins the verdict lane now; a seam re-derive would ask
            // its question twice (F-9, #4397). A probe in a transaction only
            // `latest` opened takes no landing re-run, which would reopen it every
            // flush (#4661).
            if (!probing) {
                e.R & REACTIVE_LANE_READ || ahead.push(e);
                flushTransaction || (joinFuture(null), flushTransaction.latest = 1);
            } else if (flushTransaction?.latest) return route(e, flushTransaction);
            provisionalVerdict(e, false);
 // decided above
                }
    }
}

/** Mark the pass entering a window as a verdict reader. An untracked window
 * is a snapshot like any untracked read (D-23, #3946). */ function markVerdictReader(e) {
    const t = creatorPass(context);
    if (t && tracking) {
        // A pass `reask` returned to the frame answers for the hold's world (D-25).
        if (left.has(t)) return watchVerdict(t, true), 1;
        // A pass created by one that read a held or staged value has no screen
        // and stays that world's (D-25, #4341). A first probing pass that
        // already joined a hold by a plain read restarts as a verdict reader,
        // so its reads agree in either order (D-25, #4777).
                const r = t === adopter && flushTransaction && t.R & REACTIVE_JOINED && !(t.l & CONFIG_VERDICT) && !(t.m & STATUS_UNINITIALIZED);
        t.R |= REACTIVE_PROBED | REACTIVE_WINDOW | e << WINDOW_KINDS | (creatorReads(t, 0) || (t.l |= CONFIG_VERDICT, 
        0));
        if (r && t.l & CONFIG_VERDICT) {
            // Parks the pass with no visible pending; its tail re-runs it this flush.
            setAdopter(null);
            dropFlushTx();
            t.R |= REACTIVE_MISSED_WAKE;
            t.Ie = true;
            throw new NotReadyError(null);
        }
    }
}

function latest(e) {
    if (markVerdictReader(2)) return e();
    const t = latestActive, r = verdict;
    latestActive = true;
    setVerdict(verdictValue);
    try {
        return e();
    } finally {
        latestActive = t;
        setVerdict(r);
    }
}

/**
 * Reports whether what `fn` reads is still waiting on an update. Read the
 * values in question inside `fn`: the probe answers `true` while any of
 * them depends on a change that is in flight — a write or an async source
 * that has started producing a new value whose result has not landed yet —
 * and `false` once the latest value is the one on screen. The answer is
 * tracked like any read, so a memo or effect that calls `isPending` re-runs
 * when it flips.
 *
 * A read that has no value yet is loading, not pending: inside `fn` it
 * follows the surrounding `Loading` path like any other read, and the probe
 * reports only updates to a value that already exists. A bare `refresh()`
 * does not flip `isPending`; pair it with `affects()` to make the re-ask
 * visible.
 *
 * @param fn expression to probe; read the values in question inside it
 *
 * @returns `true` while a read inside `fn` depends on a change that has not committed yet
 *
 * @example
 * ```ts
 * const [query, setQuery] = createSignal("");
 * const results = createMemo(() => searchProducts(query()));
 * const stale = createMemo(() => isPending(results));
 *
 * setQuery("shoes");
 * // stale() is true until the new results land, while results() keeps
 * // serving the previous list — dim it instead of showing a spinner.
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/reactivity/is-pending
 */ function isPending(e) {
    const t = markVerdictReader(1);
    const r = probing;
    const n = probeFound;
    const i = verdict;
    probing = !t;
    probeFound = false;
    if (!t) setVerdict(verdictValue);
    try {
        e();
        return probeFound;
    } catch (e) {
        // A node with no answer suspends when owned and reads false unowned; a
        // real error is swallowed and the probe keeps what it found (D-23, S-51).
        if (e instanceof NotReadyError && !probeFound && context !== null) throw e;
        return probeFound;
    } finally {
        probing = r;
        probeFound = n;
        setVerdict(i);
    }
}

/** A staged error throws (D-41). */ function staged(e) {
    if (e.l & CONFIG_STAGED_ERROR) throw e.ue;
    return e.ue;
}

/** `latest` of a held node: the flushed staging; an unflushed rewrite re-runs
 * its reader in the flush that carries it (F-10). */ function heldLatest(e, t) {
    const r = unflushedValue(e);
    if (r === NOT_PENDING) return staged(e);
    if (t) markLateLinker(t);
    return r;
}

/** The read inside a verdict window (D-22, D-23), off `read`'s hot path.
 * Links and pulls like a plain read, then answers by the node's state: a
 * lane's node, a held node, a staging this flush, a flight, or an error. */ function verdictValue(e, t) {
    const r = e;
    const n = !!t && !(t.l & CONFIG_CHILDREN_FORBIDDEN);
    const i = n && tracking;
    // A pulled memo's pass is its own, outside the window (D-25, #4366, #4429).
        const a = probing;
    const s = latestActive;
    probing = false;
    latestActive = false;
    setVerdict(null);
    prepareComputed(r);
    if (t && tracking) {
        link(e, t);
        // Record where this staging was asked, for `holdStaging` (#4544).
                if (e.ue !== NOT_PENDING) ext(e).Nt = e.l & CONFIG_HELD ? txOf(e) : flushTransaction || clock;
        if (e.X) pullComputed(r, t);
    }
    setVerdict(verdictValue);
    latestActive = s;
    probing = a;
    // A hydration claim reads the server's frame, whole (F-1, #4570).
        const o = snapshotCaptureActive && t && t.l & CONFIG_IN_SNAPSHOT_SCOPE ? e.T?.De : undefined, E = o !== undefined && !(r.m & STATUS_ERROR);
    if (E) {
        const r = o === NO_SNAPSHOT ? undefined : o;
        if ((e.ue !== NOT_PENDING ? e.ue : e.le) !== r) t.R |= REACTIVE_SNAPSHOT_STALE;
        return r;
    }
    // An `affects()` mark is a declared new question (F-9).
        if (probing && GlobalQueue.p?.(r)) {
        probeFound = true;
        if (i && flushTransaction) verdictRead(t, flushTransaction);
    }
    if (e.l & CONFIG_OVERRIDE && !(r.m & STATUS_ERROR && !(e.l & CONFIG_GUESS))) {
        const n = txOf(e), a = (r.m & STATUS_PENDING) !== 0 && !quietPending(r), s = e.l & CONFIG_GUESS, 
        // NOT_PENDING: born in the lane with no answer yet, so loading (F-9, #4363).
        o = s ? 0 : latestActive && !a ? latestOf(e) : screenOf(e);
        if (o !== NOT_PENDING) {
            const r = passLane !== null && sameLane(passLane, n);
            if (i) verdictRead(t, n._parent ?? n);
            if (s) {
                if (probing && (a || e.ue !== NOT_PENDING && (!e.ye || !e.ye(latestOf(e), e.ue)))) probeFound = true;
                // A folded keyed arrangement reads its world whole; D-22's guess
                // clause is not built for it (D-22 status).
                                if (latestActive && n.folded) {
                    return n.folded(null, e);
                }
                return latestOf(e);
            }
            // A landed answer the lane has not shown is still pending (D-23,
            // S-54), as is a parked frame flight of its hold (#4365).
                        if (probing && (a || parkUp(resolveTx(n._parent ?? n)))) probeFound = true; else if (probing && !r && n.un && !e.T.Ue && e.ue !== NOT_PENDING && (!e.ye || !e.ye(e.le, e.ue))) {
                probeFound = true;
                if (i) staleReader(t, n);
            }
            return o;
        }
    }
    const u = (r.m & STATUS_UNINITIALIZED) !== 0;
    if (e.l & CONFIG_HELD) {
        if (!u) {
            const a = txOf(e);
            // A render effect run off a blocked verdict lane shows the committed
            // value and re-derives at the reveal (D-25, S-30).
                        if (i && !probing && t.i === EFFECT_RENDER && !passLane && a.Pe?.un && flushTransaction !== a) {
                staleReader(t, a.Pe);
                return e.le;
            }
            // A pass that asks is display-ahead like any other (F-6, S-51).
                        if (i) verdictRead(t, a);
            // A pass the hold holds is the hold's work: its probe is final at the
            // landing, never true beside the fresh value (D-23, #4648). An async
            // memo is not re-run, which would ask again (F-9).
                        if (probing) if (t?.l & CONFIG_HELD && !t.T.Re?.P) {
                // In the landing flush the probe reads the landed world (F-1, #4662).
                if (e.ue !== NOT_PENDING && !blocked(a)) {
                    watchVerdict(t);
                    return staged(e);
                }
                if (!t.T.ce) staleReader(t, a);
            } else {
                if (heldNotFinal(r)) probeFound = true;
                // Not held yet: re-run if the flush parks (D-23). Not an async or
                // first pass, which would ask again (F-9).
                                if (n && !i && !t.T?.ce && !(t.m & STATUS_UNINITIALIZED)) watchVerdict(t);
            }
            if (latestActive && e.ue !== NOT_PENDING) {
                // A corrected shown guess stays `latest`'s value until the commit (D-34, #4689).
                if (keptGuess(e) && blocked(a)) {
                    return e.le;
                }
                // An action body reading another hold's proposal entangles with it (D-7).
                                if (!i && flushTransaction && !globalQueue.Y) joinFuture(a);
                return heldLatest(e, i ? t : null);
            }
            if (latestActive) aheadError(r);
            return e.le;
        }
        // Born held: no committed value, so no verdict (F-9); `latest` shows the staging (D-22).
                if (e.ue !== NOT_PENDING && (!(r.m & STATUS_PENDING) || latestActive && !probing)) {
            if (probing && n) joinPass(t, e);
            return staged(e);
        }
    } else if (e.ue !== NOT_PENDING) {
        // A first answer re-asked in its own tick has a verdict (F-9, D-22);
        // an error is no value (D-41).
        if (r.m & STATUS_PENDING && (!u || probing && !(e.l & CONFIG_STAGED_ERROR))) pendingVerdict(i ? t : null, r);
        // Writes that net to the committed value are no proposal: final (D-8, #4485).
         else if (i && globalQueue.Y) flushTransaction && !e.X && e.ue === e.le && Object.is(e.ue, e.le) ? route(t, flushTransaction) : provisionalVerdict(t, !latestActive && !u);
        // An unflushed write is invisible (F-10); watch the flush that carries it (#3078).
         else if (i && probing) watchVerdict(t);
        if (latestActive || u || n && !i) {
            // In a flush the staging is visible (F-10); outside one, re-run in the
            // carrying flush.
            if (globalQueue.Y) {
                return staged(e);
            }
            if (t) markLateLinker(t);
        }
        return e.le;
    }
    if (r.m & STATUS_PENDING) {
        if (!u) {
            pendingVerdict(i ? t : null, r);
            if (latestActive) aheadError(r);
            return e.le;
        }
        // As in `read`: an untracked read of a pending node still links.
                if (t && !tracking && !spectating && e !== t) link(e, t);
        throw r.T?.U;
    }
    if (e.X && r.m & STATUS_ERROR) throw r.T.U;
    return e.le;
}

/** An async memo with no window of its own (it read a verdict memo) is the
 * hold's work while its flight is up, in a lane, or re-asked: it drops the
 * verdict posture and waits like any derivation (D-25, F-9; #3884, #4408).
 * Called at a pass's head and when `source` goes pending. */ function holdWork(e, t) {
    // Truthy result: CONFIG_ASYNC stays set.
    if ((e.l & (CONFIG_ASYNC | CONFIG_OWN_PROBE)) === CONFIG_ASYNC && (!(e.m & STATUS_PENDING) || e.T.Re?.P || t.R & REACTIVE_REASK)) return e.l &= ~CONFIG_VERDICT;
}

/** A derivation of a staged error carries it with the same frame, marked
 * rather than re-run, so the frame its content owns is kept (D-41, #4562).
 * `EFFECT_TRACKED << 23` spans CONFIG_FRAME and every effect type. */ function stageMark(e, t) {
    if (t.l & CONFIG_STAGED_ERROR && !(e.l & (CONFIG_REDERIVE | CONFIG_STAGED_ERROR | EFFECT_TRACKED << 23))) {
        // Only a direct reader of the staged error carries it (#4640). A lane
        // reader does not: the staged truth is the hold's, shown at its reveal
        // (F-1, F-11, D-34; #4751, #4841).
        for (let r = e.C; r; r = r.G) if (r.S.l & CONFIG_STAGED_ERROR && !(e.T?.Re?.P && (!(r.S.l & CONFIG_GUESS) || r.S.T.Re === e.T.Re))) {
            e.ue = t.ue;
            e.l |= CONFIG_STAGED_ERROR;
            queuePendingNode(e);
            // A derived store's readers re-derive from the staging (#4189).
                        e.rr?.st(STATUS_ERROR);
            return settlePendingSource(e);
        }
    }
}

/** `el` was republished mainline with the value it had staged: a held reader
 * whose only held read was `el` keeps its pass and publishes as is (#4417). */ function keepReaders(e) {
    for (let t = e.o; t !== null; t = t.t) {
        const r = t.u;
        if ((r.l & (CONFIG_HELD | CONFIG_STAGED)) !== (CONFIG_HELD | CONFIG_STAGED) || r.i) continue;
        let n = r.C;
        while (n !== null && (n.S === e || !(n.S.l & CONFIG_HELD))) n = n.G;
        if (n !== null) continue;
        // A mount a probing pass opened attaches at the landing, not now (D-25, S-46).
                if (heldMountUnder(r, null)) continue;
        deleteFromHeap(r, dirtyQueue);
        r.R &= -4;
        r.l &= ~CONFIG_HELD;
        queuePendingNode(r);
        insertSubs(r);
    }
}

/** A run the landing holds (lanes.ts `waiting`) runs next round with its held runs. */ function attachWaits(e) {
    if (waiting?.has(e)) return globalQueue.enqueue(e.i, e.W ??= GlobalQueue.q.bind(null, e)), 
    true;
    return false;
}

/** Whether a plain read of `el` is served the hold's staging. A pass that
 * opened no window of its own inherits no posture (D-25, #4544): it is the
 * hold's work where it reads the hold's own sync write, unless a window
 * asked of that staging under the same hold (`_askedIn`, D-25 pair
 * atomicity). A frame reads the screen. No `el`: a plain store key with no
 * node, decided by posture alone, as for a signal. */ function holdStaging(e, t) {
    const r = e.T?.Dt, n = t?.T?.Nt, i = t && resolveTx(t.l & CONFIG_HELD ? t.T.Re : flushTransaction);
    // A sync verdict memo's value staged this flush is read plainly, as the
    // window inline would be (D-25, #4778).
        if (t && !e.i && !passLane && t.X && (t.l & (CONFIG_VERDICT | CONFIG_ASYNC | CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_VERDICT) return true;
    return r && !(e.l & (CONFIG_OWN_PROBE | CONFIG_FRAME) || e.R & (REACTIVE_WINDOW | REACTIVE_PROBE_UNANSWERED)) && (!t || t.ue !== NOT_PENDING && !(t.l & (CONFIG_ASYNC | CONFIG_OVERRIDE)) && n !== clock && !(typeof n == "object" && resolveTx(n) === i) && resolveTx(r) === i) && (!passLane || passLane._parent.Pe === passLane && (setPassLane(null), 
    true));
}

/** Parked frame flights. When the seam re-derives a verdict reader in its
 * lane, the frame pass's flight already asked the hold's question; the
 * landing adopts it instead of asking again (F-9, D-25; #4365, #4393).
 * Adopted at the reader's own turn in the next round, not at the landing,
 * so the hold is judged with the flight on it (F-1). Through a promise of
 * its own, so the frame pass's listener cannot land it twice. */ const armed = [];

/** Readers whose parked frame flight is up: the hold waits on it, however
 * the lane pass came out (F-2, #3884). */ const parks = [];

function park(e, t) {
    const r = e.T.He = e.T.Ee, done = () => {
        const t = parks.indexOf(e);
        if (t !== -1 && e.T.He === r) parks.splice(t, 1), schedule();
    };
    // Only a flight a reader on screen waits on, judged as `blockedBy` does
    // and before the lane's passes replace the frame's pending (F-3, #4504).
        if (t.ln.some(t => {
        if (t.m & STATUS_PENDING && (t === e || t.T.se?.has(e))) {
            if (t.i === EFFECT_RENDER) return true;
            for (let e = t.o; e; e = e.t) {
                const t = e.u;
                if (t.i === EFFECT_RENDER ? t.m & STATUS_PENDING || t.R & REACTIVE_FRAME_READ : t.l & CONFIG_REDERIVE && t.m === STATUS_PENDING) return true;
            }
        }
    })) {
        parks.indexOf(e) === -1 && parks.push(e);
        r.then(done, done);
    }
}

function parkUp(e) {
    for (let t = parks.length - 1; t >= 0; t--) {
        const r = parks[t].T, n = r.Re;
        if (!r.He || parks[t].R & REACTIVE_DISPOSED) parks.splice(t, 1); else if (n?.P && resolveTx(n._parent) === e) return true;
    }
    return false;
}

/** Passes `reask` returned to the frame; truthy when the lane pass was unanswered. */ const left = new Map;

/** A write the hold took reached lane member `c` while the hold waits on a
 * parked flight of it: that flight answers a superseded question, so the
 * new one is asked now by returning `c` (and the lane members it reads) to
 * the frame (F-2, F-9, D-25; #3884). */ function reask(e, t, r) {
    if (left.has(e)) return;
    const n = parks.indexOf(e);
    if (n === -1) {
        if (!e.i && e.T?.Re === t) for (let n = e.o; n; n = n.t) reask(n.u, t, r);
        return;
    }
    parks.splice(n, 1);
    leave(e, t, r);
}

/** Seat a lane pass from its first read. */ function seat(e, t) {
    list(e, verdictLane(t));
    e.R |= REACTIVE_LANE_DIRTY;
}

function leave(e, t, r) {
    const n = e.T;
    left.set(e, !n.Ue && e.m & STATUS_PENDING);
    covers.delete(e);
    n.Ue = false;
    n.Re = null;
    n.He = undefined;
    e.ue = NOT_PENDING;
    e.l &= -34817;
    if (e !== r) e.R |= REACTIVE_DIRTY, enqueueSub(e);
    for (let n = e.C; n; n = n.G) {
        const e = n.S;
        if (e.X && e.T?.Re === t && !left.has(e)) leave(e, t, r);
    }
}

const adoptParked = (e, t) => {
    if (t === 2) {
        const t = e.T.Re;
        if (parks.length && isVerdict(t)) reask(e, t, e);
        return false;
    }
    if (t) {
        const t = armed.indexOf(e);
        if (t < 0) return false;
        armed.splice(t, 1);
        const r = e.T.He;
        deleteFromHeap(e, dirtyQueue);
        e.R &= -4;
        e.T.He = undefined;
        releaseFlightTeardown(e);
        try {
            handleAsync(e, r.then());
        } catch {}
        e.T.fe = true;
        notifyStatus(e, STATUS_PENDING, new NotReadyError(e));
        return true;
    }
    // A sync memo re-derived at a landing voids its readers' held runs: they
    // re-run in the landing's frame (F-1).
        if (!(e.i || e.l & CONFIG_ASYNC)) for (let t = e.o; t !== null; t = t.t) {
        const e = t.u;
        if (e.i && e.Ae) {
            e.Ae = false;
            enqueueSub(e);
        }
        arm(e);
    }
    arm(e);
    return false;
};

const arm = e => e.T?.He && !(e.R & (REACTIVE_DIRTY | REACTIVE_CHECK | REACTIVE_IN_HEAP)) && !armed.includes(e) && armed.push(e);

/** The seam's park step (lanes.ts `laneSeams`, `lanesBlocked`). `r` set:
 * its probe is unanswered, truthy when nothing re-derives. `r` null: does
 * hold `t` wait on a parked flight. Both null: the seam's end, clear. */ function r5(e, t) {
    if (!e) {
        if (t) return parkUp(t);
        armed.length = 0;
        left.clear();
        return;
    }
    if (isThenable(e.T?.Ee)) {
        if (!blocked(t)) return 1;
        park(e, t);
        // Seated from its first read, so a plain read before its window does not ask again (D-25).
                seat(e, t);
    } else if (blocked(t)) {
        if (left.has(e)) seat(e, t);
        // Async memos over a sync reader: their frame flights are adopted too (F-9).
                for (let r = e.o, n; r; r = r.t) if (isThenable((n = r.u).T?.Ee) && n.T.se?.has(n) && n.l & CONFIG_HELD && txOf(n) === t && 
        // A flight asked over the screen is no frame question.
        !(n.R & REACTIVE_FRAME_READ)) {
            park(n, t);
            // An unanswered lane pass `reask` took asks again; nothing else would.
                        if (left.get(n)) seat(n, t), enqueueSub(n);
        }
    }
}

/** At the flush: writes that net to the tick's start made no change (D-8),
 * so readers with every source unchanged leave the heap and ask nothing
 * (F-9). Verdict readers still re-run. Done here rather than per write: an
 * extra arm in the setter cost it its inlining. */ function revert() {
    for (const e of pendingNodes) 
    // Netting writes are staged as the committed value itself, so only those
    // reach the comparator. A derivation's staging is an answer, not a write.
    if (e.ue === e.le && (!e.X || e.R & REACTIVE_MANUAL_WRITE)) unwake(e);
    // Held slots rewritten since the last flush are not in `pendingNodes` (F-10).
        if (heldRewrites.length !== 0) for (const e of heldRewrites.splice(0)) unwake(e);
}

function unwake(e) {
    if (e.ue === NOT_PENDING || !isSame(e, tickBase(e), e.ue)) return;
    for (let t = e.o; t !== null; t = t.t) {
        const e = t.u;
        if (!(e.R & REACTIVE_IN_HEAP) || e.R & REACTIVE_MISSED_WAKE || e.l & CONFIG_VERDICT) continue;
        let r = true;
        for (let t = e.C; t !== null && r; t = t.G) {
            const e = t.S;
            const n = e.X !== undefined;
            // A user write on a derivation counts as a written source (#4991).
                        r = !(n && e.R & (REACTIVE_IN_HEAP | REACTIVE_DIRTY | REACTIVE_CHECK)) && (e.ue === NOT_PENDING || n && !(e.R & REACTIVE_MANUAL_WRITE) || isSame(e, tickBase(e), e.ue));
        }
        if (r) {
            deleteFromHeap(e, dirtyQueue);
            e.R &= -4;
        }
    }
}

/** A pass whose every source is at its committed value is a quiet re-ask
 * (F-9). Not under a guess or in a lane, nor with no committed value. */ function pass(e, t) {
    if (!t) {
        // Not on the extension's literal: writing it on every `_x` would change its shape.
        e.T.Ft = (e.l & (CONFIG_STAGED_ERROR | CONFIG_HELD)) === (CONFIG_STAGED_ERROR | CONFIG_HELD) ? e.ue : undefined;
        return false;
    }
    if (guessing || passLane !== null || e.m & STATUS_UNINITIALIZED) return false;
    for (let t = e.C; t !== null; t = t.G) {
        const r = t.S;
        if (r.ue !== NOT_PENDING ? !isSame(r, r.le, r.ue) : r.X && r.m & STATUS_PENDING) return false;
        if (t === e._) break;
    }
    return true;
}

installVerdictHooks(observeFlight, holdWork, stageMark, keepReaders, attachWaits, holdStaging, adoptParked, r5, revert, pass);

/** An `affects()` mark on a store record with no node (store/affects.ts). */ GlobalQueue.rt = () => {
    if (probing) probeFound = true;
};

export { isPending, latest };