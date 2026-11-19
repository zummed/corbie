import { STATUS_ERROR, STATUS_PENDING, STATUS_UNINITIALIZED, STATUS_NO_VALUE, CONFIG_GUESS, REACTIVE_REASK, REACTIVE_RECOMPUTING_DEPS, CONFIG_ASYNC, CONFIG_HELD, REACTIVE_STAGED_READ, CONFIG_STAGED, CONFIG_STAGED_ERROR, REACTIVE_DISPOSED, CONFIG_AUTO_DISPOSE, REACTIVE_ZOMBIE, REACTIVE_IN_HEAP, REACTIVE_RELEASE_AFTER, NewSet, CONFIG_INPUTS_PUBLISHED, REACTIVE_DIRTY, NOT_PENDING, CONFIG_REDERIVE, CONFIG_VERDICT, CONFIG_OVERRIDE } from "./constants.js";

import { ext, statusNotifierOf, context, untrack, setSignal } from "./core.js";

import { StatusError, NotReadyError } from "./error.js";

import { unobserved, trimStaleDeps } from "./graph.js";

import { enqueueSub } from "./heap.js";

import { disposeChildren, cleanup } from "./owner.js";

import { guessing, passLane, joinFuture, txOf, holdWorkHook, question, MAINLINE_QUESTION, globalQueue, schedule, clock, flush, pendingNodes, freshHook, laneStageHook, queuePendingNode, reasks, unhold, stageMarkHook, storeParkHook } from "./scheduler.js";

// One Set, created lazily, holds every pending source: a single slot
// promoted to a Set had dual state that stranded pending entries (#2893).
function addPendingSource(e, n) {
    if (e.T?.se?.has(n)) return false;
    (ext(e).se ??= new NewSet).add(n);
    return true;
}

function removePendingSource(e, n) {
    const t = e.T?.se;
    if (!t?.delete(n)) return false;
    if (!t.size) e.T.se = undefined;
    return true;
}

function clearInFlight(e) {
    if (e.T !== null) e.T.Ee = null;
}

function clearPendingSources(e) {
    // Node-owned and never shared: dropping the reference is the clear.
    if (e.T !== null) e.T.se = undefined;
}

// A rejection-pending clears only through the settle walk over the source's
// subscribers, so it can retry only when a live dep is the source or carries
// it. A disposed dep is unlinked and holds nothing (F-3, #5003). A source that
// has settled has no walk to come: linked directly, it holds nothing (F-2).
function retryReaches(e, n) {
    for (let t = e.C; t; t = t.G) {
        const e = t.S;
        if (!(e.R & REACTIVE_DISPOSED) && (e === n ? e.m & STATUS_PENDING : e.T?.se?.has(n))) return true;
    }
    return false;
}

/** Park a loading-window node on an unready source: registered for the
 * source's settle, with no visible pending and no propagation. */ function parkLoadingWindow(e, n) {
    ext(e).fe = true;
    if (n.source) addPendingSource(e, n.source);
    // A settled error stays the answer: overwriting it here would leak a
    // NotReadyError from an invisible park (#2989).
        if (!(e.m & STATUS_ERROR)) setPendingError(e, n.source, n);
}

function setPendingError(e, n, t) {
    if (!n) {
        if (e.T !== null) e.T.U = null;
        return;
    }
    const r = t instanceof NotReadyError && t.source === n ? t : e.T?.U;
    ext(e).U = r instanceof NotReadyError && r.source === n ? r : new NotReadyError(n);
}

function forEachDependent(e, n) {
    for (let t = e.o; t !== null; t = t.t) n(t.u, t);
}

/** Depth first. `fn` sees every arrival (`seen` on a diamond's second);
 * returning false leaves the node unmarked so a later branch may walk it. */ function walkDependents(e, n) {
    const t = new NewSet;
    const visit = e => {
        const r = t.has(e);
        if (n(e, r) !== false && !r) {
            t.add(e);
            forEachDependent(e, visit);
        }
    };
    forEachDependent(e, visit);
}

// A lazy node left with no subscribers while pending is kept alive by its
// flight, so whatever clears that pending must release it (#2934). A node
// already queued releases at its pass's tail, or it would miss the landing
// it was queued for (D-22, #4474).
function releaseIfSettledUnobserved(e) {
    if (e.X && e.l & CONFIG_AUTO_DISPOSE && !e.o && !(e.R & REACTIVE_ZOMBIE) && !(e.m & STATUS_PENDING)) e.R & REACTIVE_IN_HEAP ? e.R |= REACTIVE_RELEASE_AFTER : unobserved(e);
}

// After an error propagates, release stranded lazy nodes. Collect first so
// `unobserved()` never unlinks under the walk.
function releaseSettledDependents(e) {
    let n;
    walkDependents(e, (e, t) => {
        if (!t && !e.o && e.l & CONFIG_AUTO_DISPOSE) (n ??= []).push(e);
    });
    if (n) for (const e of n) releaseIfSettledUnobserved(e);
}

/** An error a held node's pass found is an answer of that frame, staged like
 * a value and published at the commit (D-41). Not for a lane pass, nor a
 * node born held. Returns whether it was staged. */ function stageError(e, n, t) {
    // Staged even before the flush joins a hold: the seam decides, as for a
    // value (#4109, S-38). Not a projection's derive: its family never sees the
    // commit's publish (#4189).
    if (!(e.l & CONFIG_HELD || t && e.R & REACTIVE_STAGED_READ && globalQueue.Y && !e.rr) || passLane) return false;
    // An error ends the wait (D-41); a shown error stays until the commit.
        clearPendingSources(e);
    if (e.T) e.T.fe = false;
    if (!((e.m &= STATUS_ERROR | STATUS_UNINITIALIZED) & STATUS_ERROR)) setPendingError(e);
    e.ue = n instanceof StatusError ? n : new StatusError(e, n);
    e.l |= CONFIG_STAGED | CONFIG_STAGED_ERROR;
    // Readers derive from the staging; verdict readers the walk wakes need a flush (#4106).
        settlePendingSource(e);
    schedule();
    return true;
}

function settleErroredDependents(e, n) {
    let t = false;
    walkDependents(e, (e, r) => {
        if (!r && e.T?.U === n) {
            enqueueSub(e);
            t = true;
        }
    });
    if (t) schedule();
}

// Retire `source` from pending along the dependents of `el`. With a distinct
// `source`, `el` recovered without it, so dependents with another path to it
// stay pending.
function settlePendingSource(e, n = e) {
    // A superseded re-park may keep an abandoned self entry; retire it with its copies.
    removePendingSource(e, n);
    let t = false;
    let r;
    walkDependents(e, (o, s) => {
        if (s || 
        // Another dependency may still carry the source. Not marked visited: that
        // dependency may be a later branch of this walk.
        n !== e && retryReaches(o, n) || !removePendingSource(o, n)) 
        // A verdict reader holds no pending of its own: the source settling is its
        // verdict changing, so it re-runs (F-9). A derivation of a staged error
        // carries it (D-41).
        return holdWorkHook && (o.l & CONFIG_VERDICT ? enqueueSub(o) : stageMarkHook(o, e)), 
        false;
        o.ie = clock;
        const i = o.T?.se?.values().next().value;
        // An errored loading-window park keeps its error through the settle (#2989).
                const f = o.m & STATUS_ERROR;
        if (i) {
            if (!f) setPendingError(o, i);
        } else {
            o.m &= ~STATUS_PENDING;
            if (!f) setPendingError(o);
            // A held reader's re-pass over the landing is its hold's work (D-1); never a
            // lane's (#4440). A source still in flight has not landed: a reader that
            // stopped carrying it joins nothing (F-2).
                        if (o.l & CONFIG_HELD && !(n.m & STATUS_PENDING)) joinFuture(txOf(o));
            if (o.T?.fe) {
                enqueueSub(o);
                t = true;
            }
            // A derived store's readers wait on the derive itself, so they settle with it (F-9).
             else if (storeParkHook) o.rr?.st();
            if (o.T !== null) o.T.fe = false;
            // Settled with no subscribers: released after the walk (#2934).
                        if (!o.o && o.l & CONFIG_AUTO_DISPOSE) (r ??= []).push(o);
            // A chain's single arrival marks what a diamond's second does: graph shape
            // is not observable (D-41, #4562).
                        stageMarkHook?.(o, e);
        }
    });
    // Queued nodes release at their pass's tail (#4474).
        if (r) for (const e of r) releaseIfSettledUnobserved(e);
    if (t) schedule();
}

function isThenable(e) {
    return e != null && typeof e === "object" && typeof e.then === "function";
}

function releaseFlightTeardown(e) {
    const n = e.T?.Se;
    if (n != null) {
        e.T.Se = null;
        n();
    }
}

/** Internal: run `fn` when the current pass's flight is superseded, with the
 * iterator close, rather than when the replacing pass commits. A no-op
 * outside a computed's own pass. */ function onSupersede(e) {
    const n = context;
    if (n === null || n.X === undefined || !(n.R & REACTIVE_RECOMPUTING_DEPS)) return;
    const t = n.T === null;
    const r = ext(n);
    // Allocating `_x` mid-run turns `_inFlight` from undefined to null, which
    // `recompute` would read as a flight this body registered.
        if (t) r.Ee = undefined;
    const o = r.Se;
    r.Se = o === null ? e : () => {
        try {
            o();
        } finally {
            e();
        }
    };
}

/** A flight's landing through a setter (bound by store/projection.ts). A throw
 * from `commit` is the flight's error (`fail`, false returned). */ function landBySetter(e, n, t, r, o) {
    try {
        t(n);
    } catch (e) {
        o(e);
        return false;
    }
    // Unheld, a store derive's landing clears the shown error and published-inputs
    // mark now (#4269); held, at the commit.
        if (!(e.l & CONFIG_HELD)) {
        e.T.Te = null;
        e.l &= ~CONFIG_INPUTS_PUBLISHED;
    }
    if (r) landStatus(e, true);
    return true;
}

function handleAsync(e, n, 
// A store derive's landing (`landBySetter`); a callback, so a build
// without stores retains none of the arm (bundle size).
t, 
// `pending`: a NotReady rejection, not an error. A projection tells its leaves' readers.
r) {
    let o = false;
    let s = false;
    if (typeof n === "object" && n !== null) {
        untrack(() => {
            o = n[Symbol.asyncIterator];
            s = !o && isThenable(n);
        });
    }
    if (!s && !o) {
        if (e.T !== null) {
            e.T.Ee = null;
            if (holdWorkHook) e.l &= ~CONFIG_ASYNC;
        }
        e.Ie = false;
        return n;
    }
    // Iterator teardown already ran in `recompute`'s supersede release (#3122).
    // A registration path outside `recompute` must release it here.
        ext(e).Ee = n;
    // Upkept only with the verdict engine (`holdWorkHook`); see `holdWork` (D-25).
        if (holdWorkHook) e.l |= CONFIG_ASYNC;
    // Provenance (D-34): the question this flight answers.
        e.T.ce = question || MAINLINE_QUESTION;
    // The asking pass read every input, so earlier propagated pending does not
    // describe this answer; left, it would hold the node past it (#3373).
        e.T.se = undefined;
    let i;
    const handleError = t => {
        if (e.T?.Ee !== n) return;
        let o = t instanceof NotReadyError;
        if (o && e.Ie) {
            // Loading window: keep serving commit #0; release the flight so the error
            // retry can ask again.
            clearInFlight(e);
            parkLoadingWindow(e, t);
            e.ie = clock;
            return;
        }
        // A held node's error is staged (D-41), a derived store's derive's too (#4489).
                if (o || !stageError(e, t)) {
            notifyStatus(e, o ? STATUS_PENDING : STATUS_ERROR, t);
            // Published now, the error replaces the shown frame as a value's commit would.
                        if (!o) disposeChildren(e, false, true);
        }
        // A NotReady rejection lands into another pending source: hand over the self entry.
                if (o) settlePendingSource(e);
        e.ie = clock;
        // Release lazy dependents the error settled (#2934).
                if (!o) releaseSettledDependents(e);
        r?.(t, o);
    };
    const asyncWrite = (r, o) => {
        if (e.T?.Ee !== n) return;
        // A newer write re-runs the node. A first answer is still a produced value:
        // it lands before the re-ask (D-22, F-9). Only the verdict engine can tell.
                if (e.R & REACTIVE_DIRTY && !(holdWorkHook && e.m & STATUS_UNINITIALIZED)) return;
        // A write's flush is queued: it carries this landing (#4495).
                const s = holdWorkHook && pendingNodes.some(e => e.ue !== NOT_PENDING);
        const i = e.m & STATUS_UNINITIALIZED;
        landStatus(e);
        if (t) {
            // False: the derive's commit threw, taken as the flight's error.
            if (!t(r, i, handleError)) return;
        } else {
            // A held node stages the answer into its hold (`setSignal`).
            setSignal(e, () => r);
        }
        // The loading window closes when the answer is observable: at the commit if
        // staged, so no one-frame pulse leaks (#2990).
                if (e.ue === NOT_PENDING) {
            e.Ie = false;
            // Trim the old dependency tail once published; a staging trims at its commit (#3410).
                        trimStaleDeps(e);
        }
        settlePendingSource(e);
        schedule();
        // A write's flush already queued carries this landing with the others ahead
        // of it (F-10, D-22). Flushing now would re-ask before their reactions ran (#4495).
                if (o || !s) flush();
        o?.();
    };
    // A pending node's flight counts as an observer, so settling runs the
    // last-one-out check. Returns whether it released, so a stream stops (#2935).
        const settleAutodispose = () => {
        if (e.l & CONFIG_AUTO_DISPOSE && !e.o && !(e.m & STATUS_PENDING)) {
            unobserved(e);
            return true;
        }
        return false;
    };
    // Consume an AsyncIterable as this flight's values. Live (called inside the
    // read): a sync first yield is stashed for the caller. Deferred (a thenable
    // resolved to an iterable later): every step writes through `asyncWrite`
    // and close registers on `owner`. Returns whether a sync answer landed.
        const consumeIterator = (t, r) => {
        const o = t[Symbol.asyncIterator]();
        let s = false;
        let f = false;
        let u = !r;
        const close = () => {
            if (f) return;
            f = true;
            try {
                const e = o.return?.();
                if (isThenable(e)) e.then(undefined, () => {});
            } catch {}
        };
        cleanup(close, r);
        // Supersede stops the stream at once (#3122). Assigned, not chained, for bytes:
        // it replaces an earlier `onSupersede` teardown, which falls back to owner cleanup.
                ext(e).Se = close;
        // Release before each pull, or an unobserved stream pumps forever (#2935).
        // A protocol throw settles as this flight's error, not a rejection (#4112).
                const iterateOrRelease = () => {
            try {
                if (!settleAutodispose()) iterate();
            } catch (e) {
                handleError(e);
            }
        };
        const iterate = () => {
            let t, r, l = false, a = false, S = true;
            // Like `for await`, accept a bare IteratorResult as a settled step.
                        const E = o.next();
            const c = isThenable(E) ? E : {
                then: e => void e(E)
            };
            c.then(r => {
                // Only the initial drain has a caller for the stash; later sync steps write
                // through, or the value is dropped.
                if (S && u) {
                    t = r;
                    l = true;
                    if (r.done) f = true;
                } else if (e.T?.Ee !== n) {
                    return;
                } else if (!r.done) {
                    s = true;
                    asyncWrite(r.value, iterateOrRelease);
                } else {
                    f = true;
                    if (s) {
                        schedule();
                        flush();
                    } else {
                        asyncWrite(undefined);
                    }
                    settleAutodispose();
                }
            }, t => {
                if (S && u) {
                    r = t;
                    a = true;
                } else if (e.T?.Ee === n) {
                    f = true;
                    handleError(t);
                    settleAutodispose();
                }
            });
            S = false;
            if (a) {
                f = true;
                handleError(r);
                if (u) throw r;
                return true;
            }
            if (l && !t.done) {
                i = t.value;
                s = true;
                return iterate();
            }
            return l && t.done;
        };
        const l = iterate();
        // Later calls run from `asyncWrite`, where a rethrow would be unhandled.
                u = false;
        return s || l;
    };
    // Whether a live drain landed synchronously; null when none ran.
        let f = null;
    // A thenable that resolves to an AsyncIterable is consumed as the stream.
    // One level: A+ `then` already collapses nested thenables.
        const flattenIfIterable = (e, n) => {
        let t = false;
        if (typeof e === "object" && e !== null) {
            untrack(() => {
                t = e[Symbol.asyncIterator];
            });
        }
        if (!t) return false;
        i = undefined;
        const r = consumeIterator(e, n);
        if (!n) f = r;
        return true;
    };
    if (s) {
        let t = false, r = false, o, s = true;
        n.then(r => {
            if (s) {
                i = r;
                t = true;
            } else {
                // The first yield clears pending as a resolution would. A disposed node never
                // starts a pump. The ambient owner is gone, so close registers on `el`; a
                // protocol throw settles as this flight's error (#4112).
                try {
                    if (e.T?.Ee === n && !(e.R & REACTIVE_DISPOSED) && flattenIfIterable(r, e)) return;
                    asyncWrite(r);
                } catch (e) {
                    handleError(e);
                }
                settleAutodispose();
            }
        }, e => {
            if (s) {
                o = e;
                r = true;
            } else {
                handleError(e);
                settleAutodispose();
            }
        });
        s = false;
        if (r) {
            // Settle as a rejection does, then unwind so the read is not `undefined`.
            handleError(o);
            throw o;
        } else if (!t) {
            // Not landed: the tail suspends, or a loading window serves commit #0.
            f = false;
        } else if (!flattenIfIterable(i)) {
            e.Ie = false;
        }
    }
    if (o) flattenIfIterable(n);
    if (f !== null) {
        if (!f) {
            if (e.Ie) return e.le;
            throw new NotReadyError(context);
        }
        e.Ie = false;
    }
    return i;
}

function clearStatus(e, n = false) {
    if (e.T?.se) clearPendingSources(e);
    if (e.T?.fe) e.T.fe = false;
    e.m = n ? 0 : e.m & STATUS_UNINITIALIZED;
    if (e.T?.U) setPendingError(e);
    const t = statusNotifierOf(e);
    if (t) t.call(e);
}

/** A landing answers the node's own question: it retires the self entry, not
 * pending its sources propagated. With another source still pending the
 * node stays pending on it (#3373, #3376). `_blocked` clears, so an input
 * settling unchanged does not ask again; `_error` moves off self. */ function landStatus(e, n = false) {
    const t = e.T?.se;
    if (t && (t.delete(e), t.size)) {
        e.T.fe = false;
        if (n) e.m = STATUS_PENDING;
        setPendingError(e, t.values().next().value);
    } else clearStatus(e, n);
}

function notifyStatus(e, n, t) {
    if (n === STATUS_ERROR && !(t instanceof StatusError) && !(t instanceof NotReadyError)) t = new StatusError(e, t);
    const r = n === STATUS_PENDING && t instanceof NotReadyError && t.source;
    if (r) {
        // A re-ask of a shown error keeps it on display (D-41).
        if (e.m & STATUS_ERROR) e.T.Te = e.T.U;
        addPendingSource(e, r);
    } else clearPendingSources(e);
    // An error answering a node with no value keeps it uninitialized; a re-ask
    // is uninitialized again; a write gives it a value.
        e.m = n | (n === STATUS_ERROR ? e.m & (STATUS_UNINITIALIZED | STATUS_NO_VALUE) ? STATUS_NO_VALUE : 0 : n === STATUS_PENDING && e.m & STATUS_NO_VALUE ? STATUS_UNINITIALIZED : e.m & STATUS_UNINITIALIZED);
    // Readers park on the propagated source.
        if (r) setPendingError(e, r, t); else ext(e).U = t;
    // A guess is verdict-inert: its own flight is the hold's to wait on, not its
    // readers' (F-9, F-11). An input changed outside the guess's hold asks a
    // new question, held with that hold (D-34, #4336); a `refresh()` does not.
        if (guessing && n === STATUS_PENDING && e.l & CONFIG_GUESS) {
        if (passLane === null) joinFuture(e.R & REACTIVE_REASK ? null : txOf(e)._parent);
        return;
    }
    const o = statusNotifierOf(e);
    if (o) {
        o.call(e);
        return;
    }
    propagateStatus(e, n, t);
}

/** The dependents' half of `notifyStatus`. Also used alone for a store
 * family's leaves, which are not the derive's dependents. */ function propagateStatus(e, n, t) {
    const r = n === STATUS_PENDING && t instanceof NotReadyError && t.source;
    forEachDependent(e, (o, s) => {
        o.ie = clock;
        // Re-derive instead of marking when:
        // - a pending mark arrives on a kept-tail link: it is a question for the next
        //   pass, not this one (#3494);
        // - the subscriber is a verdict reader or a boundary's output: its pass
        //   decides what the change means (D-23, D-25);
        // - it carries a staged error: the source asks again (#4109).
        // Not an async memo D-25 makes the hold's work: it waits like any
        // derivation and asks once, at the landing (F-9, #4397).
                if (o.l & CONFIG_REDERIVE || holdWorkHook && (o.l & CONFIG_VERDICT || r && o.l & CONFIG_STAGED_ERROR) && !(r && holdWorkHook(o, e)) || n === STATUS_PENDING && s.Ne !== o.de) {
            // A re-derived subscriber no longer waits on a settled source: retire the
            // entry there and on its dependents, or the subscriber stays pending on a
            // source that never settles again.
            if (n !== STATUS_PENDING && removePendingSource(o, e)) settlePendingSource(o, e);
            enqueueSub(o);
            schedule();
            return;
        }
        // Already pending on this source: nothing re-notifies, but the re-asking
        // write is held with the flight the observer waits on (D-9).
                if (r && o.T?.se?.has(r)) {
            if (passLane === null && (o.l & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD && globalQueue.Y && !freshHook?.(o, r.ie === clock)) joinFuture(txOf(o));
            return;
        }
        if (r || n !== STATUS_PENDING && (o.T?.U !== t || o.T?.se)) {
            // Queued for the commit sweep, with the lane when the pass is lane work.
            if (laneStageHook && passLane !== null) laneStageHook(o, passLane, true, e); else {
                queuePendingNode(o);
                // Pending enters a held memo's hold without re-running it, so the write that
                // started the flight is held with it (D-9). A render effect's membership is
                // its pass's.
                                if (n === STATUS_PENDING && (o.l & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD && !o.i) if (guessing && o.m & STATUS_PENDING && reasks(o)) 
                // A hold waiting only on re-asks holds nothing new (D-34, F-9): the memo
                // leaves it and publishes with the write's frame. Only guesses can keep such a
                // hold open (#4828).
                unhold(o); else joinFuture(txOf(o));
            }
            notifyStatus(o, n, t);
            // A derived store's derive is marked, not re-run: its family walks the leaves.
                        o.rr?.st(n, t);
        }
    });
}

export { addPendingSource, clearInFlight, clearStatus, forEachDependent, handleAsync, isThenable, landBySetter, notifyStatus, onSupersede, parkLoadingWindow, propagateStatus, releaseFlightTeardown, releaseIfSettledUnobserved, releaseSettledDependents, removePendingSource, retryReaches, setPendingError, settleErroredDependents, settlePendingSource, stageError };