import { notifyStatus, releaseFlightTeardown, handleAsync, clearStatus, parkLoadingWindow, stageError, settlePendingSource, retryReaches, releaseIfSettledUnobserved, clearInFlight, settleErroredDependents } from "./async.js";

import { EVENT_HANDLER_READ, REACTIVE_FRAME_READ, STATUS_UNINITIALIZED, REACTIVE_LAZY, REACTIVE_NONE, defaultContext, CONFIG_TRANSPARENT, CONFIG_OWNED_WRITE, CONFIG_AUTO_DISPOSE, CONFIG_SYNC, CONFIG_NO_SNAPSHOT, CONFIG_IN_SNAPSHOT_SCOPE, NOT_PENDING, CONFIG_HELD, REACTIVE_MANUAL_WRITE, STATUS_ERROR, REACTIVE_DIRTY, CONFIG_OVERRIDE, NO_SNAPSHOT, CONFIG_HAS_SNAPSHOT, REACTIVE_DISPOSED, REACTIVE_CHECK, REACTIVE_IN_HEAP, REACTIVE_REASK, REACTIVE_SNAPSHOT_STALE, CONFIG_CHILDREN_FORBIDDEN, CONFIG_VERDICT, CONFIG_HELD_FRAME, EFFECT_RENDER, STATUS_PENDING, REACTIVE_AHEAD, CONFIG_FRAME, CONFIG_ASYNC, CONFIG_OWN_PROBE, CONFIG_STAGED, CONFIG_MOUNT_AHEAD, REACTIVE_ZOMBIE, CONFIG_STAGED_ERROR, REACTIVE_LANE_DIRTY, REACTIVE_PROBED, CONFIG_GUESS, REACTIVE_RECOMPUTING_DEPS, CONFIG_LAGS, EFFECT_TRACKED, REACTIVE_RELEASE_AFTER, REACTIVE_MISSED_WAKE, REACTIVE_JOINED, REACTIVE_STAGED_READ, REACTIVE_WINDOW, REACTIVE_LANE_READ, REACTIVE_SCREEN_READ, REACTIVE_PROBE_UNANSWERED, CAUGHT_REARMED, CONFIG_PROMOTED, CONFIG_INPUTS_PUBLISHED, WINDOW_KINDS, CONFIG_FRESH_READ, REACTIVE_OWNER_READS, REACTIVE_IN_HEAP_HEIGHT, CONFIG_SLOT_NODE, EFFECT_USER, NewSet, STATUS_NONE } from "./constants.js";

import { NotReadyError } from "./error.js";

import { link, dormantNodes, clearDeps, trimStaleDeps, deferReleases, deferredReleases } from "./graph.js";

import { insertIntoHeap, deleteFromHeap, insertIntoHeapHeight, enqueueSub, markNode, markHeap, liftOwned } from "./heap.js";

import { checkCaughtRead } from "./dev.js";

import { inheritId, linkChild, disposeChildren, markDisposal } from "./owner.js";

import "./lanes.js";

import { schedule, inEffectCallback, effectCallbackEvent, effectCallbackOwner, setEffectCallback, ANY_EVENT, externalUntrackHook, clock, txOf, flushTransaction, laneWriteHook, joinFuture, observeFlightHook, queuePendingNode, globalQueue, notifyEpoch, insertSubs, dirtyQueue, laneStageHook, passLane, resolveTx, laneReadHook, laneServedHook, wireExternalSourceHook, bumpNotifyEpoch, laneAdoptHook, setPassLane, joinPassTx, freshHook, passHook, holdWorkHook, GlobalQueue, laneOutcomeHook, holdNode, laneDirty, passTx, heldTrim, keepReadersHook, holdStagingHook, staleReader, laneSeamsHook, adopted, deferZombie, stagedReaders, revertHook, heldRewrites } from "./scheduler.js";

// A tracked effect's heap visit is its empty compute phase, then the user
// queue: routing through the heap orders the run after the commit whatever
// phase the write came from (#3291).
GlobalQueue._e = e => {
    if (e.i === EFFECT_TRACKED) {
        deleteFromHeap(e, dirtyQueue);
        e.Ae = true;
        globalQueue.enqueue(EFFECT_USER, e.Z);
    }
    // A zombie's fate is the seam's; it runs now only for its lane's re-staging,
    // which shows ahead of any park (#3463).
     else if (e.R & REACTIVE_ZOMBIE && !(e.R & REACTIVE_LANE_DIRTY)) deferZombie(e);
    // A lane frame member follows mainline writes only: the lane's dirty is
    // cancelled (#3698, #4137). Folded away without the lane engine.
     else if (laneStageHook && e.R & REACTIVE_ZOMBIE && laneFrameMember(e)) {
        deleteFromHeap(e, dirtyQueue);
        e.R &= -65540;
    }
    // A reader the landing re-derives whose frame flight the seam parked
    // adopts it at its turn (verdict.ts `adoptParked`, D-25, F-9).
     else if (!(laneAdoptHook && e.T?.He && laneAdoptHook(e, 1))) recompute(e);
};

/** The nearest non-zombie owner carries a lane's value or, an effect, is
 * listed in a lane. */ function laneFrameMember(e) {
    let t = e._parent;
    while (t.R & REACTIVE_ZOMBIE) t = t._parent;
    return (t.l & (CONFIG_OVERRIDE | CONFIG_GUESS)) === CONFIG_OVERRIDE || t.i && t.T?.Re?.P;
}

GlobalQueue.Ce = disposeChildren;

let tracking = false;

let context = null;

let snapshotCaptureActive = false;

/** The lane at the root of the running chain of first passes, inherited by
 * every first pass under it (#4166). */ let laneRoot;

let snapshotSources = null;

function ownerInSnapshotScope(e) {
    while (e) {
        if (e.Oe) return true;
        e = e._parent;
    }
    return false;
}

function setSnapshotCapture(e) {
    snapshotCaptureActive = e;
    if (e && !snapshotSources) snapshotSources = new NewSet;
}

/** Mark `owner` a snapshot scope: readers created under it serve each
 * source's value from when hydration began and replay at
 * `releaseSnapshotScope`. With `served`, `owner` is a hydrated computed and
 * `served` its server answer: it becomes the node's snapshot if it has none. */ function markSnapshotScope(e, t) {
    if (arguments.length < 2) e.Oe = true; else if (snapshotCaptureActive && e.T?.De === undefined) takeSnapshot(e, t);
}

function releaseSnapshotScope(e) {
    e.Oe = false;
    releaseSubtree(e);
    schedule();
}

function releaseSubtree(e) {
    let t = e.pe;
    while (t) {
        if (t.Oe) {
            t = t.Ge;
            continue;
        }
        if (t.X) {
            const e = t;
            e.l &= ~CONFIG_IN_SNAPSHOT_SCOPE;
            if (e.R & REACTIVE_SNAPSHOT_STALE) {
                e.R &= ~REACTIVE_SNAPSHOT_STALE;
                e.R |= REACTIVE_DIRTY;
                if (dirtyQueue.Fe > e.he) dirtyQueue.Fe = e.he;
                insertIntoHeap(e, dirtyQueue);
            }
        }
        releaseSubtree(t);
        t = t.Ge;
    }
}

function clearSnapshots() {
    if (snapshotSources) {
        for (const e of snapshotSources) {
            // Assign, never `delete`: deleting pushes `_x` to dictionary mode.
            const t = e.T;
            if (t != null) t.De = undefined;
        }
        snapshotSources = null;
    }
    snapshotCaptureActive = false;
}

function recompute(e, t = false) {
    // Any recompute can clean a marked subscriber: invalidate the skip (§12d).
    bumpNotifyEpoch();
    const n = e.i;
    let o;
    let E = NOT_PENDING;
    // Lanes: a pass is seated by its node. A node carrying a lane's value runs
    // as the lane's whoever dirtied it; a guess's own pass is its truth arriving,
    // the frame's (F-11). A first pass takes its creator's lane, except under a
    // verdict lane: a mount there is mainline (D-25, #3851). Restored at the end.
    // Folded away without the lane engine.
        const s = passLane;
    // The mount a first pass belongs to, inherited from its creator (#4166).
        const l = laneRoot;
    const r = laneStageHook && (laneRoot = t && (s || l));
    // The seam's re-derive runs with its parked flight current; a later pass
    // asks its own question (D-25, verdict.ts `reask`).
        if (laneAdoptHook && !t && e.T) {
        const t = e.T.He;
        if (t ? t !== e.T.Ee && (e.T.He = undefined, 1) : e.l & CONFIG_OVERRIDE && !(e.R & REACTIVE_LANE_DIRTY)) laneAdoptHook(e, 2);
    }
    if (laneStageHook) {
        const n = t ? creatorPass(context) : null;
        const o = n?.R | 0;
        // A first pass under a pass that probed is the mount the probe gates, and so
        // is what that pass creates (D-25, S-46, S-58, #4806).
                if (o & REACTIVE_PROBED || n && (n.l & CONFIG_MOUNT_AHEAD && n.m & STATUS_UNINITIALIZED || n.i === EFFECT_RENDER && n._ && hasDep(n, e => e.l & CONFIG_VERDICT, n._))) e.l |= CONFIG_MOUNT_AHEAD;
        setPassLane((e.R & REACTIVE_LANE_DIRTY || (e.l & (CONFIG_OVERRIDE | CONFIG_GUESS)) === CONFIG_OVERRIDE) && e.T?.Re?.P ? e.T.Re : // Loose `!=`: false with no creator lane (undefined == null).
        s?._parent.Pe != s && o & REACTIVE_RECOMPUTING_DEPS ? s : null);
    }
    // An error a pass staged, which this pass replaces.
        const a = e.l & CONFIG_STAGED_ERROR;
    let i;
    if (!t) {
        deleteFromHeap(e, dirtyQueue);
        // A pending flight is the hold's own work (#3884), and so is a held async
        // memo's re-run with no window of its own (D-25, `holdWork`): clear the
        // verdict posture before the flight is superseded, or a frame read would
        // settle it and drop the hold. Not an effect. Folded away without the lane
        // engine.
                if (laneStageHook && !n && (e.m & STATUS_PENDING || (e.l & (CONFIG_ASYNC | CONFIG_OWN_PROBE | CONFIG_HELD)) === (CONFIG_ASYNC | CONFIG_HELD))) e.l &= ~CONFIG_VERDICT;
        if (e.T !== null) {
            e.T.Ee = null;
            // Supersede kills an iterator flight (#3122).
                        releaseFlightTeardown(e);
        }
        // A pass over a held derivation joins its hold (D-1); a node held with nothing
        // staged joins too. Not a render effect or boundary structure: outside its
        // hold's flush the frame publishes mainline (#4283). A frame born into the
        // hold keeps staging (#4010).
                if (e.l & CONFIG_HELD) {
            const t = txOf(e);
            // Under a blocked lane the frame joins nothing: a lane never holds a sync
            // write (#3460). An answered deferred node's re-pass lags and joins nothing
            // (D-40). A verdict reader born into the hold is its own work (D-25, D-12,
            // #4467). `EFFECT_RENDER << 23` is CONFIG_FRAME.
                        if (t.P || e.l & CONFIG_LAGS) ; else if (!(e.l & (CONFIG_VERDICT | EFFECT_RENDER << 23)) || observeFlightHook && !(e.l & EFFECT_RENDER << 23) && bornInto(e, t)) joinPassTx(t, freshHook?.(e, e.m & STATUS_UNINITIALIZED)); else if (t !== flushTransaction && !(e.m & STATUS_UNINITIALIZED || e.l & CONFIG_HELD_FRAME)) {
                // Published mainline, the frame is replaced on the spot and the landing
                // re-derives a held one (#3404). Readers of the superseded staging re-derive
                // even if the value is unchanged (#4378).
                if ((E = e.ue) !== NOT_PENDING) insertSubs(e);
                e.ue = NOT_PENDING;
                e.l &= ~CONFIG_HELD;
            }
        }
        // Children of a pass still awaiting its commit were never shown and die now;
        // otherwise they are the frame and park until this pass commits.
                if (n === EFFECT_TRACKED || e.l & CONFIG_STAGED) {
            // Lazy nodes only those children read release at this pass's end unless
            // re-read (#4142). A throwing cleanup is rethrown when the flush ends.
            const t = deferredReleases;
            deferReleases(o = []);
            disposeChildren(e);
            deferReleases(t);
        } else if (e.pe !== null || e.ke !== null) parkChildren(e);
        // This pass replaces a staged error (D-41, #4109, #4145); a held one stays
        // `latest`'s answer while this pass's flight is up (D-22). `linked`: the pass
        // had sources.
                if (passHook) {
            i = e.C !== null;
            // Only a node with an error to keep pays the call.
                        if (e.T !== null && (e.l & CONFIG_STAGED_ERROR || e.T.Ft !== undefined)) passHook(e);
        }
        if (e.l & CONFIG_STAGED_ERROR) {
            e.ue = NOT_PENDING;
            e.l ^= CONFIG_STAGED_ERROR;
        }
    }
    const I = e.m & STATUS_UNINITIALIZED;
    // Capture error and pending before the compute clears them: a dropped source
    // can leave blocked dependents outside its settle walk.
        const _ = e.m & STATUS_ERROR && e.T?.U;
    const N = (e.m & STATUS_PENDING) !== 0;
    const u = N && e.T?.se;
    // A node whose own flight parked dependents, superseded and settled
    // synchronously here, settles them here: its landing never will (#3181).
        const f = e.T?.se?.has(e);
    // This pass's error is staged, not published (D-41).
        let T = false;
    // A `refresh()` is quiet unless the question in flight is new (F-9), or a
    // guess's new question is still held (#4336). A held user write is not an
    // answer, so a refresh over it is a re-ask (D-34, #4887, #4888).
        let S = (e.R & REACTIVE_REASK) !== 0 && !((e.m & STATUS_PENDING || e.ue !== NOT_PENDING && !(e.R & REACTIVE_MANUAL_WRITE)) && e.T !== null && !e.T.Ve);
    const A = context;
    context = e;
    e._ = null;
    e.de++;
    // REACTIVE_ZOMBIE is position (on its owner's parked frame), so every per-pass
    // wipe keeps it (#3543). A release found while queued is this pass's tail's (#4474).
        const c = e.R & REACTIVE_RELEASE_AFTER;
    e.R = REACTIVE_RECOMPUTING_DEPS | e.R & (REACTIVE_ZOMBIE | REACTIVE_REASK);
    e.ie = clock;
    // The previous value: the staging, else the committed value.
        let R = e.ue !== NOT_PENDING ? e.ue : e.le;
    let C = e.he;
    let O = false;
    // Whether this pass read the future, or for a lane's mount, this flush's staging (#3851, #4166).
        let d = false;
    let D = tracking;
    tracking = true;
    try {
        // The sync fast arm is prod-only, so dev checks sync nodes too (D-42).
        if (!false && e.l & CONFIG_SYNC) {
            R = e.X(R);
            if (e.T !== null) e.T.Ee = null;
            e.Ie = false;
        } else {
            // A projection self-registers its flight; its undefined return is not an answer.
            const t = e.T?.Ee;
            const n = e.X(R);
            const o = typeof n === "object" && n !== null;
            const E = e.T?.Ee !== t;
            // Dev: a body that returned after a read threw into it caught the throw,
            // unless it returned the flight the throw became (D-42).
                        if (false) ;
            R = E || !o ? n : handleAsync(e, n);
            if (!E && !o) {
                if (e.T !== null) {
                    e.T.Ee = null;
                    if (holdWorkHook) e.l &= ~CONFIG_ASYNC;
                }
                // A sync return is the first real answer.
                                e.Ie = false;
            }
        }
        // No `_x` means no status to clear. Lane work served the committed value of a
        // flight in the air keeps its pending, so a boundary keeps its fallback
        // (F-4, #4862).
                if ((e.m !== 0 || e.T !== null) && !(laneServedHook && e.m & STATUS_PENDING && laneServedHook(e, null, true))) clearStatus(e, t);
    } catch (t) {
        const n = t instanceof NotReadyError;
        // An error or a loading window's unready read answers: no flight is its own
        // (D-25, `holdWork`). A pending pass keeps the last answer's.
                if (holdWorkHook && (!n || e.Ie)) e.l &= ~CONFIG_ASYNC;
        if (n && e.Ie) {
            // Loading window: register for the source's settle with no visible pending;
            // the loading value keeps serving and an error stays the answer.
            parkLoadingWindow(e, t);
        } else {
            // A held pass, or one that read this flush's staging, stages its error (D-41).
            if (n || !(T = stageError(e, t, true))) {
                if (n) ext(e).fe = true;
                notifyStatus(e, n ? STATUS_PENDING : STATUS_ERROR, t);
                // If no new flight re-owned self, retire the superseded flight's copies.
                                if (n && f && !e.T?.Ee) settlePendingSource(e);
            }
            // A re-park drops sources this pass no longer reaches, from dependents that
            // reached them only through here; their landing walk stops at this node and
            // would never find them (#3456, #4541). Dependents with another path keep
            // the source. A still-pending source has its own walk to come.
                        if ((n || T) && u) for (const t of u) if (t !== e && !(e.T?.se?.has(t) && (t.m & STATUS_PENDING || retryReaches(e, t)))) settlePendingSource(e, t);
        }
    } finally {
        tracking = D;
        if (laneStageHook) laneRoot = l;
        // Consume the missed-wake latch (#3037). The wipe below must not key off
        // DIRTY/CHECK, which `read`'s pull sets as bookkeeping.
                O = (e.R & REACTIVE_MISSED_WAKE) !== 0;
        // A mount's staged read joins it, except a nested derivation left pending: no
        // render effect reaches it, so it holds nothing (F-3, #4203, #4340).
                d = !!(e.R & (r ? s || n || !(e.m & STATUS_PENDING) ? REACTIVE_JOINED | REACTIVE_STAGED_READ : 0 : REACTIVE_JOINED));
        // A verdict reader is one only while it probes or reads a memo that does
        // (D-25). Folded away without the verdict engine.
                if (observeFlightHook) e.l = e.l & 1073709055 | e.R & REACTIVE_PROBED | (e.R & REACTIVE_WINDOW) << 9 | (e.R & REACTIVE_AHEAD) << 11;
        // REACTIVE_DISPOSED survives: the pass may have disposed its own owner (#3621).
        // REACTIVE_FRAME_READ is this pass's verdict for the landing.
                e.R = e.R & (REACTIVE_ZOMBIE | REACTIVE_DISPOSED | REACTIVE_FRAME_READ | REACTIVE_STAGED_READ | REACTIVE_LANE_READ | REACTIVE_SCREEN_READ | REACTIVE_PROBE_UNANSWERED) | (t ? e.R & REACTIVE_SNAPSHOT_STALE : 0);
        context = A;
        if (o) for (const e of o) releaseIfSettledUnobserved(e);
        // A pending pass on a `refresh()`, or one that read only committed values,
        // is a quiet re-ask (F-9). Asked only of a pending pass: the dep walk is too dear.
                if (i && !S && e.m & STATUS_PENDING) S = !!passHook(e, 1);
        if (e.T !== null) e.T.Ve = S && (e.m & STATUS_PENDING) !== 0;
    }
    // A node that died during its own pass is void: unlink the reads made after
    // `dispose()`, retire its flight, and publish nothing (#3024, #3621).
        if (e.R & REACTIVE_DISPOSED) {
        clearDeps(e);
        clearInFlight(e);
        if (laneStageHook) setPassLane(s);
        return;
    }
    // The lane this pass is work of. A pass that read none of the lane's world
    // has left it; a guess never leaves this way.
        let p = !laneStageHook ? null : passLane;
    // False: the pass left the lane.
        const G = !!e.T?.U || T;
    if (laneStageHook && p !== null) {
        if (laneStageHook(e, p, t, G)) setPassLane(p); else p = null;
    }
    if (!G) {
        // INV-11 (#3330): compare against the slot this run publishes to.
        const o = e.ue === NOT_PENDING;
        const E = !o ? e.ue : e.le;
        // A throwing comparator is contained in `isSame` (#2837, #4111). Replacing a
        // staged error is a new answer even at the committed value (D-41, #4403); a
        // re-pass equal to a first value staged this frame changes nothing (#4540).
                const s = flushTransaction ?? passTx;
        const l = o && I && !n || !!e.T?.Te || 
        // In a flush that joined a hold, a recovery to the last good value is a
        // change of answer (D-41, S-53).
        !!(_ && s) || !!a || 
        // Lane work on a node a held pass created has no screen: its value is a
        // change (F-11, #3835).
        p !== null && e._parent?.l & CONFIG_HELD || !isSame(e, E, R);
        // Effects use `_equals: false`; flag and enqueue here. `!create`: `effect()`
        // runs the first pass itself.
                if (n && l && 
        // A stale lane reader that computed what it last applied owes no run.
        !(e.R & REACTIVE_SCREEN_READ && !e.Ae && Object.is(e.ge, R)) && 
        // A re-staging pass owes no run: the commit replays the effect (#3802).
        o) {
            e.Ae = !e.T?.U;
            // One bound runner per effect. Lane work's run is the lane's; a first pass's
            // run is `effect()`'s to queue (#4557).
                        if (!t) globalQueue.enqueue(n, e.W ??= GlobalQueue.q.bind(null, e), p);
        }
        // A guess's own pass from the frame is its truth: staged under the lane's
        // parent, notifying only a correction (F-11, D-34). Hook first, so a build
        // without lanes folds it.
                if (laneOutcomeHook && !t && p === null && e.l & CONFIG_GUESS && laneOutcomeHook(e, R, G)) ; else if (G) ; else if (l) {
            // A first pass is born held (staged, committed with the hold) when the flush
            // has joined a hold and the pass read a held node, or a verdict lane's mount
            // read this flush's staging (#3851), or a held pass created it (D-1). Any
            // other first pass publishes and re-derives if the flush parks. An effect
            // with an uncommitted staging re-stages.
            if (p !== null) {
                // Lane work stages for the lane's next seam. An effect born held re-stages.
                if (n && e.ue === NOT_PENDING) e.le = R; else e.ue = R;
                // A derivation born in the lane has no committed value until the lane lands.
                // Not an effect: marked uninitialized it re-ran without end (#4871).
                                if (t && !n) e.m |= STATUS_UNINITIALIZED;
            } else if (t ? !(s !== null && (d || creatorPass(A)?.R & REACTIVE_JOINED)) : n && e.ue === NOT_PENDING) {
                // A first pass or an effect publishes directly. A memo stages even in a quiet
                // world: mid-batch pulls must see the fresh value while plain reads stay
                // committed until flush (#3009).
                e.le = R;
            } else {
                e.ue = R;
                // A staged recovery keeps the error on display until it commits (D-41).
                                if (_) e.T.Te = _;
                // Born held: a read of it this tick is a read of the hold's world.
                                if (t) {
                    holdNode(e, s);
                    // No committed value until the landing (F-9).
                                        e.m |= STATUS_UNINITIALIZED;
                    // A boundary that has not shown content catches it (#3540), unless the mount
                    // is part of the hold itself: then it appears at the commit (D-12).
                                        freshHook?.(e, !r && flushTransaction);
                }
            }
            // Lane work notifies its members as lane work (`laneDirty`).
                        if (e.o !== null) p !== null ? laneDirty(e, p) : insertSubs(e);
        } else if (e.he != C) {
            for (let t = e.o; t !== null; t = t.t) insertIntoHeapHeight(t.u, dirtyQueue);
        }
        // Silent recovery to an unchanged value: re-run dependents still holding the
        // propagated error (#2949).
                if (!l && !e.T?.U) {
            if (_) settleErroredDependents(e, _);
            // Self-registration is the #3181 sweep's below.
                        if (u) for (const t of u) if (t !== e) settlePendingSource(e, t);
        }
        // A synchronous settle supersedes the old landing callback, so this pass
        // sweeps (#3181). An uninitialized node with no replacement stays parked.
                if (f && !(e.m & (STATUS_PENDING | STATUS_UNINITIALIZED))) settlePendingSource(e);
    }
    // A first pass gone pending where it would have been born held is held: its
    // answer is the hold's (D-1). Under a boundary that has not shown content it
    // is held but the hold does not wait for it (F-4, #3540); an `on` re-arm owns
    // it (D-12). A derivation left pending reaches no boundary (#4542).
        if (t && p === null && d && e.m & STATUS_PENDING) {
        const t = flushTransaction ?? passTx;
        t && !t.P && (n && freshHook?.(e) === CAUGHT_REARMED || holdNode(e, t));
    }
    // Dependencies are the committed frame's until it is replaced (D-10, #3410):
    // a staging leaves the old tail for `commitPendingNode` to trim. An errored
    // pass keeps its list. An effect trims when its run applies (#3438). An
    // unchanged pass waits on the flush's verdict (#3469). Creation passes and
    // tracked effects trim now; lane work counts as unchanged.
        if (!e.T?.U && (p !== null || e.ue === NOT_PENDING) && !(n && e.Ae)) {
        if (t || n === EFFECT_TRACKED) trimStaleDeps(e); else heldTrim(e);
    }
    // Queue for this flush's commit: a staged value, a parked frame, or status
    // to settle. A first pass only when pending or born held. Its children stay
    // CONFIG_STAGED until then.
        if (e.ue !== NOT_PENDING || e.T !== null && (e.T.me !== null || e.T.Le !== null) || (e.m & (STATUS_PENDING | STATUS_UNINITIALIZED)) !== 0 && (!t || (e.m & STATUS_PENDING) !== 0)) {
        e.l |= CONFIG_STAGED;
        // Lane work is the lane's to reveal.
                if (p === null) queuePendingNode(e);
    }
    if (laneStageHook) setPassLane(s);
    // Missed wake: values read before the nested commit are stale, so run again.
    // Equality gates keep it convergent.
        if (O) {
        enqueueSub(e);
        schedule();
    }
    // Re-derived to the staging it superseded: held readers keep their pass (#4417).
        if (keepReadersHook && E !== NOT_PENDING && Object.is(R, E)) keepReadersHook(e);
    // A release a settle deferred past this pass (#2934, #4474).
        if (c) releaseIfSettledUnobserved(e);
}

function updateIfNecessary(e) {
    // Never re-enter a computing node: its dep bookkeeping is live (#3037).
    // Never recompute a disposed node: it would come back alive (#2983).
    if (e.R & (REACTIVE_RECOMPUTING_DEPS | REACTIVE_DISPOSED)) return;
    if (e.R & REACTIVE_CHECK) {
        for (let t = e.C; t; t = t.G) {
            const n = t.S;
            if (n.X) {
                updateIfNecessary(n);
            }
            if (e.R & REACTIVE_DIRTY) {
                break;
            }
        }
    }
    if (e.R & REACTIVE_DIRTY || e.T?.U && e.ie < clock && !e.T?.Ee) {
        recompute(e);
    }
    // The pass may have disposed the node (#3621): carry the flag. The manual
    // write mark is state, lifted only by a re-deriving pass or the commit
    // (D-8, #3612); keep the last pass's landing verdicts too.
        e.R = e.R & (REACTIVE_SNAPSHOT_STALE | REACTIVE_IN_HEAP | REACTIVE_IN_HEAP_HEIGHT | REACTIVE_ZOMBIE | REACTIVE_DISPOSED | REACTIVE_MANUAL_WRITE | REACTIVE_FRAME_READ | REACTIVE_STAGED_READ | REACTIVE_LANE_READ | REACTIVE_SCREEN_READ | REACTIVE_PROBE_UNANSWERED);
}

function computed(e, t) {
    const n = t?.transparent ?? false;
    // `in`: an explicit `loadingValue: undefined` is a real commit #0.
        const o = t !== null && typeof t === "object" && "loadingValue" in t;
    // One literal per tier, chosen at build time. Observe adds `_name`/`_loc` as
    // literal slots: a later write forces a hidden-class transition on every
    // node. Keep the two in sync; the dist artifact test pins the key sets.
        const E = {
        id: inheritId(t, n, context),
        l: (n ? CONFIG_TRANSPARENT : 0) | (t?.ownedWrite ? CONFIG_OWNED_WRITE : 0) | (!context || t?.lazy ? CONFIG_AUTO_DISPOSE : 0) | (t?.sync ? CONFIG_SYNC : 0) | (t?.ae ? CONFIG_NO_SNAPSHOT : 0) | (t?.xe ?? 0) | (snapshotCaptureActive && ownerInSnapshotScope(context) ? CONFIG_IN_SNAPSHOT_SCOPE : 0),
        ye: t?.equals ?? isEqual,
        ke: null,
        A: context?.A ?? defaultContext,
        be: 0,
        X: e,
        le: o ? t.loadingValue : undefined,
        he: 0,
        ve: undefined,
        We: null,
        C: null,
        _: null,
        de: 0,
        o: null,
        Ze: null,
        _parent: context,
        Ge: null,
        Me: null,
        pe: null,
        R: t?.lazy ? REACTIVE_LAZY : REACTIVE_NONE,
        m: o ? 0 : STATUS_UNINITIALIZED,
        ie: clock,
        ue: NOT_PENDING,
        we: -1,
        Ie: o,
        // Cold fields live in `_x`: past ~39 in-object fields creation cost ~4x (§12).
        T: null
    };
    if (t?.unobserved) ext(E).Ye = t.unobserved;
    setupComputedNode(E, t);
    return E;
}

/** Lazily allocate the cold extension. Hot paths read `el._x?._field` behind
 * `_config` bits; never call `ext()` just to store a default. */ function ext(e) {
    return e.T ??= {
        Ee: null,
        Se: null,
        U: undefined,
        fe: undefined,
        se: undefined,
        Ye: undefined,
        De: undefined,
        me: null,
        Le: null,
        Re: null,
        Ve: false,
        je: NOT_PENDING,
        Be: -1,
        ce: 0,
        Ue: false,
        I: 0,
        Te: null
    };
}

/** A tracked read of a held node joins the flush to its hold (D-1), unless the
 * reader is a render effect: the frame reads the committed value outside its
 * hold's flush (D-25, `frameRead`). A node born held has no committed value,
 * so every reader joins. Under a boundary that has not shown content, a first
 * pass is the boundary's (F-4, #3540). */
/** The pass whose plain read made this flush adopt a hold: a window it then
 * opens can give the hold back (#4777). */ let adopter = null;

function setAdopter(e) {
    adopter = e;
}

function joinPass(e, t) {
    if (observeFlightHook && !flushTransaction) adopter = e, adopted.length = 0;
    e.R |= REACTIVE_JOINED;
    // A new memo's first pass is the hold's work but not the tick's: the render
    // effect reading it decides (F-3, #3540).
        if (t.m & STATUS_UNINITIALIZED || e.i !== EFFECT_RENDER) joinPassTx(txOf(t), !e.i && e.m & STATUS_UNINITIALIZED || freshHook?.(e));
}

/** `joinPass` into hold `t` (a folded guess, D-34); never a render effect. */ function joinPassAt(e, t) {
    e.R |= REACTIVE_JOINED;
    joinPassTx(t, !e.i && e.m & STATUS_UNINITIALIZED || freshHook?.(e));
}

/** A frame reader of a node another hold holds is served the committed value,
 * publishes mainline and re-derives at that landing (D-25). Not one born
 * into that hold (#4010), nor a flight whose inputs already show: that would
 * tear (F-1, #3305). Lane work is a frame reader too: a lane sees the screen
 * plus its own guesses (F-6). */ function frameRead(e, t) {
    // Frame readers: render effects, verdict readers and lane work (D-25); a
    // boundary's tree and output too (#4283). Not a mount's memo: the hold's own
    // later passes read it (D-1). Not a pass with only an inherited posture
    // reading the hold's own write (D-25, #4544).
    const n = observeFlightHook && e.l & CONFIG_VERDICT;
    const o = laneStageHook && passLane !== null;
    // A render effect a probing pass is mounting is born held on the hold's
    // staging or flight, whatever else it read (D-25, S-46, S-58). Not lane work
    // reading a held node that is not pending. Folded away without lanes.
        if (laneStageHook && e.l & CONFIG_MOUNT_AHEAD && e.m & STATUS_UNINITIALIZED && (!o || t.m & STATUS_PENDING)) {
        joinPass(e, t);
        return false;
    }
    if (!(n && holdStagingHook(e, t)) && !((!o || e.m & STATUS_UNINITIALIZED && t.m & STATUS_PENDING) && !n && (!(e.l & EFFECT_RENDER << 23) || t.l & CONFIG_INPUTS_PUBLISHED))) {
        const E = txOf(t);
        // Lane work and verdict readers are never the hold's pass, except a lane
        // re-based over this hold's answer: it reads that staging without joining
        // (F-1, F-7, D-34; #4536, #4576, #4643).
                if (o && passLane.over && resolveTx(passLane.over) === E && !n) return false;
        if (!(!o && E === flushTransaction && !n || bornInto(e, E))) {
            staleReader(e, E);
            // They no longer wait on `el`'s flight.
                        if (o || n) laneServedHook(e, t);
            return true;
        }
    }
    joinPass(e, t);
    return false;
}

/** Born into `t`: held and never committed, or a member of a frame its held
 * pass built. It reads `t`'s values in any flush and keeps staging (#4010). */ function bornInto(e, t) {
    return (e.m & STATUS_UNINITIALIZED || e.l & CONFIG_HELD_FRAME) && e.l & CONFIG_HELD && txOf(e) === t;
}

/** With `tail`, this pass's reads only. */ function hasDep(e, t, n) {
    for (let o = e.C; o !== null; o = o === n ? null : o.G) if (t(o.S)) return 1;
}

/** A staging a verdict pass reads for the first time is content it opens,
 * not a value it keeps (D-25, S-46, #4806). */ let fresh;

function readsFresh(e, t) {
    return t.ue !== NOT_PENDING && e.l & CONFIG_VERDICT && !hasDep(e, e => e === t);
}

/** The computed a creation under `owner` belongs to, or null. */ function creatorPass(e) {
    return e !== null && e.Qe ? e.Ke : e;
}

/** Whether the passes running around `p` read the future or a staging: a pass
 * they created has no screen (#4341). */ function creatorReads(e, t) {
    return (e = creatorPass(e._parent))?.R & REACTIVE_RECOMPUTING_DEPS ? creatorReads(e, t | e.R) : t & (REACTIVE_JOINED | REACTIVE_STAGED_READ);
}

/** Park the previous pass's children and disposal list until this node's
 * commit replaces them. */ function parkChildren(e) {
    markDisposal(e);
    const t = ext(e);
    t.Le = e.ke;
    t.me = e.pe;
    e.ke = null;
    e.pe = null;
    e.be = 0;
}

/** One literal with every effect field, so V8 sees the full shape at construction. */ function createEffectNode(e, t, n, o, E) {
    const s = E?.transparent ?? false;
    // See `computed()` for the observe slots.
        const l = {
        id: inheritId(E, s, context),
        l: 
        // CONFIG_FRAME for a render effect.
        o << 23 | (s ? CONFIG_TRANSPARENT : 0) | (E?.ownedWrite ? CONFIG_OWNED_WRITE : 0) | (E?.sync ? CONFIG_SYNC : 0) | (E?.xe ?? 0) | (snapshotCaptureActive && ownerInSnapshotScope(context) ? CONFIG_IN_SNAPSHOT_SCOPE : 0),
        ye: false,
        ke: null,
        A: context?.A ?? defaultContext,
        be: 0,
        X: e,
        le: undefined,
        he: 0,
        ve: undefined,
        We: null,
        C: null,
        _: null,
        de: 0,
        o: null,
        Ze: null,
        _parent: context,
        Ge: null,
        Me: null,
        pe: null,
        R: REACTIVE_LAZY,
        m: STATUS_UNINITIALIZED,
        ie: clock,
        ue: NOT_PENDING,
        we: -1,
        Ie: false,
        Ae: false,
        ge: undefined,
        qe: t,
        Je: n,
        ze: undefined,
        i: o,
        T: null
    };
    // Effects share one status notifier (`statusNotifierOf`): a per-node one
    // forced an `_x` on every effect (+23% creation).
        if (E?.unobserved) ext(l).Ye = E.unobserved;
    setupComputedNode(l, lazyOptions);
    return l;
}

/** The shared effect status notifier, installed by effect.ts. */ let effectStatusNotify = null;

function setEffectStatusNotify(e) {
    effectStatusNotify = e;
}

/** Effect nodes get the shared notifier; presence also marks a display consumer. */ function statusNotifierOf(e) {
    return e.i ? effectStatusNotify : null;
}

const lazyOptions = {
    lazy: true
};

function setupComputedNode(e, t) {
    e.We = e;
    const n = context?.Qe ? context.Ke : context;
    if (context) linkChild(context, e);
    if (n) e.he = n.he + 1;
    wireExternalSourceHook?.(e);
    !t?.lazy && recompute(e, true);
    if (snapshotCaptureActive && !t?.lazy) {
        if (!(e.m & STATUS_PENDING) && !(e.l & CONFIG_NO_SNAPSHOT)) takeSnapshot(e, e.le);
    }
}

function signal(e, t) {
    // See `computed()`. Observe adds `_name`, `_loc` and `_owner` (for owner paths).
    const n = {
        ye: t?.equals ?? isEqual,
        l: (t?.ownedWrite ? CONFIG_OWNED_WRITE : 0) | (t?.ae ? CONFIG_NO_SNAPSHOT : 0),
        le: e,
        o: null,
        Ze: null,
        ie: clock,
        ue: NOT_PENDING,
        // No `_time`/`_fn`/`_statusFlags`: stores make one signal per touched leaf,
        // so signal bytes are store bytes. Missing reads falsy on shared paths (§12e).
        we: -1,
        T: null
    };
    if (t?.unobserved) ext(n).Ye = t.unobserved;
    if (snapshotCaptureActive && !(n.l & CONFIG_NO_SNAPSHOT)) takeSnapshot(n, e);
    return n;
}

function isEqual(e, t) {
    return e === t;
}

/** A store slot node in one literal: target and key as back-refs, the wrap
 * cache (`px`/`pxv`) and accessor verdict pre-shaped. No options, no `_x`,
 * no expandos. Observe adds label and location slots. */ function slotSignal(e, t, n, o, E) {
    const s = {
        ye: t,
        l: CONFIG_OWNED_WRITE | CONFIG_SLOT_NODE,
        le: e,
        o: null,
        Ze: null,
        ie: clock,
        ue: NOT_PENDING,
        we: -1,
        T: null,
        O: n,
        Xe: o,
        acc: E,
        px: undefined,
        pxv: undefined
    };
    return s;
}

/** Dev: when set to a component name, a reactive read outside a nested
 * tracking scope warns. Managed by `untrack(fn, strictReadLabel)`. */
/** The verdict windows' read, set while a window is open. */ let verdict = null;

function setVerdict(e) {
    verdict = e;
}

/** Dev-only: runs `fn` inside the strict-read window labelled `label`, tracking
 * unchanged. Compiled dev DOM output wraps event handler expressions in it
 * (#1742). Production builds just call `fn`. */ function strictReadTracked(e, t = EVENT_HANDLER_READ) {
    return e();
}

/**
 * Runs `fn` outside of any reactive tracking — reads inside `fn` will not
 * subscribe the current scope. Returns whatever `fn` returns.
 *
 * Use `untrack` inside a memo or effect when you need to read a signal once
 * without making the surrounding computation depend on its future changes.
 *
 * In a component body, `untrack` marks a read as a deliberate one-time
 * snapshot and silences `STRICT_READ_UNTRACKED`. Use it only when the value
 * must never update and is never downstream of async: an async-derived value
 * can still be pending (the read throws) or mid-transition (the snapshot is
 * the old committed value). When it should follow its source, move the read
 * to where the value is used (JSX, a memo, an effect's compute, the event
 * handler, or an accessor passed to the hook that needs it) instead.
 *
 * `untrack` stops dependency tracking, not async settlement. Reading an
 * uninitialized async source still suspends the owning computation; it runs
 * again when that source first resolves. Afterward, the untracked read is a
 * snapshot and does not subscribe the computation to later changes.
 *
 * If the first read must return immediately, give the async source a
 * `loadingValue` before reading it with `untrack`. `isPending` and `latest`
 * inspect updates to an existing value; neither supplies a missing initial
 * value.
 *
 * Pass a `strictReadLabel` string to enable a dev-mode warning: any reactive
 * read inside `fn` that isn't inside a nested tracking scope will log a
 * warning naming the label.
 *
 * @example
 * ```ts
 * createEffect(
 *   () => trigger(),                 // tracks `trigger` only
 *   () => {
 *     const snapshot = untrack(() => state); // read once, untracked
 *     log(snapshot);
 *   }
 * );
 *
 * const color = createMemo<string>(() => fetchColor(), { loadingValue: "gray" });
 * const initialColor = untrack(color); // "gray" while the fetch is unresolved
 * ```
 */ function untrack(e, t) {
    const n = tracking;
    tracking = false;
    try {
        if (externalUntrackHook) return externalUntrackHook(e);
        return e();
    } finally {
        tracking = n;
    }
}

/** > 0 while `runRefCallback` runs: a ref born into a held node's hold may
 * read its staging (#4767, F-7). */ let refCallbackDepth = 0;

/** Runs a compiled `ref()` callback untracked in the creating pass's context
 * (#4761); in dev, creating a primitive there is PRIMITIVE_IN_EFFECT_CALLBACK.
 *
 * @internal */ function runRefCallback(e) {
    refCallbackDepth++;
    try {
        if (false) ; else untrack(e);
    } finally {
        refCallbackDepth--;
    }
}

/** A bookkeeping read on behalf of no node: it records no re-run link (#3528)
 * and no transaction entry (#3540) on `context`. */ let spectating = false;

function prepareComputed(e, t) {
    if (e.R & REACTIVE_LAZY) {
        e.R &= ~REACTIVE_LAZY;
        recompute(e, true);
    } else if (e.R & REACTIVE_DISPOSED) {
        // Two disposal lifecycles share the flag (#3024): a dormant auto-dispose node
        // wakes on read (pay-for-use); an owner-disposed one is dead and serves its
        // last committed value, since recomputing runs user code in a torn-down tree.
        if (e.l & CONFIG_AUTO_DISPOSE) {
            const t = e._parent;
            if (t !== null) {
                // A dormant node was off the chain when its owner died (#3024).
                if (t.R & REACTIVE_DISPOSED) {
                    e.l &= ~CONFIG_AUTO_DISPOSE;
                    return;
                }
                // A zombie never left its chain: relinking would put it on the live one too.
                                if (!(e.R & REACTIVE_ZOMBIE)) linkChild(t, e);
            }
            recompute(e, true);
        }
    } else ;
}

/** Does reader `c` see a staged node's committed value? Read's fast block
 * carries the same test inline. An untracked read and a frame-only reader
 * (createTrackedEffect / onSettled, D-29) see the committed frame. */ function readerSeesCommitted(e, t) {
    return !t || e.ue === NOT_PENDING || t.l & CONFIG_CHILDREN_FORBIDDEN;
}

/** Set when a node is staged outside a flush, cleared when a flush begins:
 * one module boolean keeps the F-10 arm free inside a flush. */ let unflushedStaged = false;

function markUnflushedStaged() {
    unflushedStaged = true;
}

/** A write is visible from the flush that carries it (F-10): outside a flush
 * a staged node serves its committed value. Promoted writes are exempt. */ function unflushedValue(e) {
    if (globalQueue.Y || e.ue === NOT_PENDING || e.l & CONFIG_PROMOTED) return NOT_PENDING;
    // A held node was flushed and parked: not unflushed unless rewritten since,
    // then the flushed staging is what the world sees until the next flush.
        if (e.l & CONFIG_HELD) return e.T.Be === clock ? e.T.je : NOT_PENDING;
    return e.le;
}

/** The value a signal had when this tick began writing it (F-10). */ function tickBase(e) {
    return e.l & CONFIG_HELD && e.T?.Be === clock ? e.T.je : e.le;
}

/** A held node rewritten outside a flush keeps the last flushed staging until
 * the flush that carries the rewrite (F-10); the clock stamps the stash. */ function stashFlushed(e) {
    const t = e.T;
    if (t.Be !== clock) {
        // D-8: the flush checks whether the rewrite nets (verdict.ts `revert`).
        // Once per node per flush: the stamp below dedupes it.
        if (revertHook) heldRewrites.push(e);
        t.je = e.ue;
        t.Be = clock;
        unflushedStaged = true;
    }
}

const promotedWrites = [];

/** A derivation served the committed value because of an unflushed write runs
 * again in the flush that carries it: it linked after the write walked (F-10, #3337). */ function markLateLinker(e) {
    // A direct enqueue here would be wiped by the pass's flag reset.
    e.R |= REACTIVE_MISSED_WAKE;
    return true;
}

function resyncUnflushedCompanions() {
    unflushedStaged = false;
    // Length-guarded so the common flush allocates nothing.
        if (promotedWrites.length !== 0) {
        for (const e of promotedWrites) e.l &= ~CONFIG_PROMOTED;
        promotedWrites.length = 0;
    }
}

function pullComputed(e, t) {
    if (e.he >= dirtyQueue.Fe || e.R & REACTIVE_CHECK) {
        markNode(t);
        markHeap(dirtyQueue);
        updateIfNecessary(e);
    }
    // A fresh-pull waiter (awaitable refresh) recomputes a dirty source inline:
    // it must park on the re-ask, never read the pre-re-ask value as settled.
     else if (t.l & CONFIG_FRESH_READ) updateIfNecessary(e);
    const n = e.he;
    // parent check is shallow, might need to be recursive
        if (e._parent === t) e.R |= REACTIVE_OWNER_READS; else if (n >= t.he) liftOwned(t, t.he = n + 1);
}

/** A first answer staged this flush and re-asked in it (D-22, F-9): a memo
 * with no value derives from that answer once, as a flush of the answer
 * alone would have let it, then waits on the re-ask. One served pass a
 * flush (`_served`), or a pass that throws anyway re-runs without end. Not a
 * render effect, lane work, a verdict lane's pass (D-25) or a held node. */ function servesStaged(e, t, n) {
    return passLane === null && e.m & n.m & STATUS_UNINITIALIZED && t.ue !== NOT_PENDING && !(t.l & (CONFIG_HELD | CONFIG_STAGED_ERROR)) && !e.i && (e.R & REACTIVE_MISSED_WAKE || e.ue === NOT_PENDING && e.T?.$e !== clock && (!(e.m & STATUS_PENDING) || e.T?.se?.delete(n.T.U.source)) && (ext(e).$e = clock, 
    e.R |= REACTIVE_MISSED_WAKE));
}

/** @internal Compiler output (`@solidjs/web`): truthy while the running pass is
 * a stale reader of an open hold (D-9). A grouped stateful prop binding then
 * skips an unchanged write, so a sibling's re-run does not put the committed
 * value back over the field (#4872). */ function heldRead() {
    return context.R & REACTIVE_FRAME_READ;
}

// Typed `any` so `el` needs no cast alias: an alias the minifier keeps.
function read(e) {
    let t = context;
    if (t?.Qe) t = t.Ke;
    // The verdict windows' read (verdict.ts), off this path. It wakes a lazy or
    // dormant memo outside the window (#4366).
        if (verdict !== null) {
        return verdict(e, t);
    }
    if (typeof e.X === "function") prepareComputed(e);
    if (!e.X && e.T?.De === undefined && !snapshotCaptureActive && (!unflushedStaged || e.ue === NOT_PENDING) && // F-10, see readNodeFast
    !(e.l & CONFIG_OVERRIDE) && // lanes: the slow path's one arm
    true) {
        if (t) {
            if (laneStageHook) fresh = tracking && readsFresh(t, e);
            if (tracking) link(e, t);
            // A pass that reads a held value joins its hold, tracked or not (D-1): a
            // mainline derivation of a held write would tear (F-1). A first pass that
            // did is born held (F-7). Frame-only readers (D-29) and frame readers
            // (`frameRead`, D-25) see the committed value instead.
                        if (e.l & CONFIG_HELD && !(t.l & CONFIG_CHILDREN_FORBIDDEN)) {
                if (frameRead(t, e)) {
                    return e.le;
                }
            }
        }
        if (!t || e.ue === NOT_PENDING || t.l & CONFIG_CHILDREN_FORBIDDEN) {
            return e.le;
        }
        // Writes that netted are no proposal (D-8): not a read of the future, nor a
        // stale verdict read to re-run at the landing (#4485). `Object.is` too, for
        // signed zeros; `===` keeps an accepted NaN rewrite a proposal.
                const n = e.ue === e.le && Object.is(e.ue, e.le);
        // Lane work over a folded lane, once the frame has a transaction, reads the
        // screen (D-34).
                if (n || (observeFlightHook && t.l & CONFIG_VERDICT || laneStageHook && passLane?.folded) && stagedScreen(t, e)) {
            return e.le;
        }
        stagedRead(t);
        return e.ue;
    }
    // Decided after the pull below: a pull can join (D-25).
        let n = false;
    // Only a re-read retries an errored source (F-9).
        let o;
    if (t) {
        if (laneStageHook) fresh = tracking && readsFresh(t, e);
        if (tracking) {
            o = link(e, t);
            if (e.X) {
                pullComputed(e, t);
                // A memo over a verdict reader's value reads ahead as if it probed inline
                // (F-6, #4050). Not an effect, and not the hold's own work (born in its
                // world, or over its own staged pass): there a pending read throws (D-25,
                // #4504, #3884). The posture lasts until a pass completes having read no
                // verdict reader (D-25, #4408). A render effect never committed reading a
                // settled verdict reader's value reads ahead too (F-6, #4544).
                                if (observeFlightHook && (t.m & STATUS_UNINITIALIZED ? t.l & CONFIG_HELD || flushTransaction : t.l & CONFIG_HELD_FRAME)) {
                    if (t.i === EFFECT_RENDER && e.l & CONFIG_VERDICT && !(e.m & (STATUS_PENDING | STATUS_UNINITIALIZED))) t.R |= REACTIVE_AHEAD;
                    // A sync verdict memo is display while only the display reads it (D-25,
                    // #4776). The hold's own work reading it makes it the hold's: it re-derives
                    // now on the hold's writes (`holdStaging`). A memo that probes in its own
                    // body stays the screen's (F-6, #4544). An async derivation's read seats
                    // nothing (S-51): the lane and the mount it gates would wait on each other
                    // (#4965).
                                        if ((e.l & (CONFIG_VERDICT | CONFIG_FRAME | CONFIG_ASYNC | CONFIG_OWN_PROBE)) === CONFIG_VERDICT && !(t.l & CONFIG_ASYNC)) {
                        const n = resolveTx(t.l & CONFIG_HELD ? txOf(t) : flushTransaction), o = ext(e);
                        if (o.Dt !== n) {
                            o.Dt = n;
                            recompute(e);
                        }
                    }
                }
                if (observeFlightHook && !t.i) t.R |= e.l & CONFIG_VERDICT & (e.m & STATUS_PENDING && (t.l & CONFIG_STAGED || t.m & STATUS_UNINITIALIZED) ? t.l : -1);
            }
        }
        // A node born into the future has no committed value: every reader derives
        // from the future, a render effect included (D-1, F-7).
                if (e.l & CONFIG_HELD && !(e.l & CONFIG_OVERRIDE) && !(t.l & CONFIG_CHILDREN_FORBIDDEN)) {
            if (e.m & STATUS_UNINITIALIZED) joinPass(t, e); else n = frameRead(t, e);
        }
    }
    // After the pull, so the node is current. A pending member throws as any.
        if (laneReadHook && e.l & CONFIG_OVERRIDE) {
        const n = laneReadHook(t, e);
        if (n !== NOT_PENDING) return n;
    }
    // A hydration claim is served the value the server rendered with, and
    // replays at the scope's release. An errored node still throws below.
        if (snapshotCaptureActive && t && t.l & CONFIG_IN_SNAPSHOT_SCOPE) {
        const n = e.T?.De;
        if (n !== undefined && !(e.m & STATUS_ERROR)) {
            const o = n === NO_SNAPSHOT ? undefined : n;
            const E = e.ue !== NOT_PENDING ? e.ue : e.le;
            if (E !== o) t.R |= REACTIVE_SNAPSHOT_STALE;
            return o;
        }
    }
    // A stale reader of a held flight is served the committed value and does
    // not go pending (D-9, D-25); so is lane work reading a flight not its own
    // (D-25), and a verdict reader (`[isPending(x), x()]` is `[true, stale]` in
    // either order, D-23). Not a flight committed beneath inputs on screen
    // (F-1), nor a reader born into its transaction (#4010).
        if (laneStageHook && e.m & STATUS_PENDING && !n && !(e.l & CONFIG_OVERRIDE) && !(e.m & STATUS_UNINITIALIZED)) {
        // A verdict reader first, routed or not (D-25, #4661). A reader with no
        // committed value sees the flight pending, so a boundary a lane mounts
        // shows its fallback (F-7). A mount a probing pass opened is born held
        // (S-46, S-58).
        if (t !== null && t.l & CONFIG_VERDICT && !(t.l & CONFIG_MOUNT_AHEAD && t.m & STATUS_UNINITIALIZED)) n = observeFlightHook(t, e); else if (passLane !== null && (!(t?.m & STATUS_UNINITIALIZED) || bornOfLaneRerun(t))) 
        // Lane work is served the committed value unless it read this flush's
        // staging (F-2, S-57).
        n = laneServedHook(t, e, true);
    }
    if (e.m & STATUS_PENDING && !n) {
        // Untracked, an uninitialized node has nothing to serve and throws. A
        // frame-only reader sees the frame (D-29).
        if (t) {
            if (!(t.l & CONFIG_CHILDREN_FORBIDDEN) || e.m & STATUS_UNINITIALIZED) {
                // Out of line, so `read` gains a call only where a pending read throws (D-22, F-9).
                if (observeFlightHook && servesStaged(t, e, e)) {
                    return e.ue;
                }
                // An untracked read of a pending node still re-runs its reader when it
                // settles, unless nobody reads (`spectating`, #3528) or it reads itself.
                                if (!tracking && !spectating && e !== t) link(e, t);
                throw e.T?.U;
            }
        } else if (e.m & STATUS_UNINITIALIZED) {
            throw e.T?.U;
        }
    }
    // An errored derive throws for every reader, memo parity (D-41, #2897).
        if (e.X && e.m & STATUS_ERROR) {
        // Only a re-read retries an errored source (F-9): tracked, by a pass whose
        // last pass read it (#5002), on a later cycle, and not by a zombie, whose
        // retry livelocked the live frame's readers (S-58, #4405).
        if (o && e.ie < clock && !(e.R & REACTIVE_ZOMBIE)) {
            recompute(e);
            return read(e);
        }
        // A staged error is this flush's answer (D-41): a reader served the staging
        // throws it and stages its own; one served the screen throws the shown one
        // (#4404).
                if (!n && e.l & CONFIG_STAGED_ERROR) serve(e, t);
        throw e.T?.U;
    }
    // A reader served the screen throws the shown error (D-41).
        if (n || !t) throwShown(e);
    const E = serve(e, n ? null : t);
    if (!t && typeof e.X === "function" && e.l & CONFIG_AUTO_DISPOSE && !(e.m & STATUS_PENDING) && !e.o) {
        // Deferred: an inline unobserved() made consecutive untracked reads answer
        // differently with no write between (#3078). The flush sweep reclaims it.
        dormantNodes.add(e);
        schedule();
    }
    return E;
}

/** A first pass a guess lane's re-run creates stands where the memo it
 * replaces stood, so lane work's read of a parent's flight is the screen's
 * for it too (#4824). A boundary's structure (F-7, #3835) and a mount a
 * display-ahead read opens (S-58, D-25) still see the flight. */ function bornOfLaneRerun(e) {
    const t = e._parent;
    return passLane._parent?.Pe !== passLane && t?.X && !(t.m & STATUS_UNINITIALIZED | t.l & CONFIG_FRAME);
}

/** A reader served the screen reads a shown error until its replacement
 * commits (D-41). */ function throwShown(e) {
    if (e.T?.Te) {
        throw e.T.Te;
    }
}

/** The value reader `c` (null: untracked) is served from `el`; read's fast
 * block keeps its trivial ternary for speed. */ function serve(e, t) {
    // A node born staged has no committed value: an untracked, bookkeeping or
    // frame-only reader has nothing to serve and holds (F-9, D-29). A ref born
    // into this node's hold reads it as the hold's own work (D-25, #4767); one
    // mounted outside must not see it ahead of the commit (F-7).
    if (e.ue !== NOT_PENDING && e.m & STATUS_UNINITIALIZED && (!t || spectating || t.l & CONFIG_CHILDREN_FORBIDDEN)) {
        if (refCallbackDepth > 0) {
            const t = creatorPass(context);
            if (t && e.l & CONFIG_HELD && bornInto(t, txOf(e))) {
                return e.ue;
            }
        }
        throw new NotReadyError(null);
    }
    const n = t && unflushedStaged ? unflushedValue(e) : NOT_PENDING;
    if (n !== NOT_PENDING) {
        markLateLinker(t);
        return n;
    }
    if (readerSeesCommitted(e, t) || observeFlightHook && t.l & CONFIG_VERDICT && !(e.m & STATUS_UNINITIALIZED) && stagedScreen(t, e) && !throwShown(e)) {
        return e.le;
    }
    // Writes that netted are no proposal (D-8, #4452), as in read's fast block.
        stagedRead(t, e);
    if (e.l & CONFIG_STAGED_ERROR) {
        throw e.ue;
    }
    return e.ue;
}

/** A pass read a staging of this flush: the seam decides (D-25). Lane work
 * too: if the flush parks, the pass re-derives on the committed world and
 * its lane waits that round, so the held write never shows through it. A
 * folded lane only before the frame has a transaction (D-34). Not a node
 * born staged: a re-run would read the same staging every round (#3969). */ function stagedRead(e, t) {
    if (!(t?.m & STATUS_UNINITIALIZED)) {
        // Lane work that was served a flight's committed value now reads a staging:
        // it throws (#4859).
        if (laneServedHook && passLane !== null) laneServedHook(e, null);
        e.R |= REACTIVE_STAGED_READ;
        // A verdict effect's staging read is the frame's, re-derived once if the
        // flush parks (D-22, D-25). A memo is not a leaf (`frameRead`).
                if (laneSeamsHook && (passLane !== null || e.i && e.l & CONFIG_VERDICT)) stagedReaders.push(e);
    }
}

/** A verdict reader's plain read of a staged node sees the screen once the
 * frame has a transaction, and re-derives at its landing (D-25, #3322).
 * Before that the seam answers. A node born into the future (F-7), a reader
 * born into the transaction (#4010) or into the hold (D-25) has no screen. */ function stagedScreen(e, t) {
    if (flushTransaction === null || bornInto(e, flushTransaction) || (t?.l & (CONFIG_HELD | CONFIG_OVERRIDE)) === CONFIG_HELD || 
    // This flush's own write read by a pass with an inherited posture is the
    // frame's derivation (D-25, #4544).
    observeFlightHook && holdStagingHook(e, t)) return false;
    // The staging is the frame's read in the frame's seat; the seam re-derives
    // only if the flush parks (D-22, F-10). Content a display-ahead gate opens
    // is a mount (#4806), but not under the pass's own `isPending` window: a
    // read's place beside the probe never changes what shows (F-6, D-23, #4966).
        if (laneStageHook && t && fresh && passLane && e.R >> WINDOW_KINDS != 1) passLane.Gt = 1;
    staleReader(e, flushTransaction);
    return true;
}

/** A write inside a recompute outside a flush is promoted at that pass's end. */ function notePromotedWrite(e) {
    if (globalQueue.Y || e.l & CONFIG_PROMOTED) return;
    e.l |= CONFIG_PROMOTED;
    promotedWrites.push(e);
}

/** The first write during snapshot capture to a plain user signal records the
 * pre-write value. Async landings reveal, and projection leaves are their
 * compute's, so neither qualifies. */ function captureWriteSnapshot(e, t) {
    if (e.l & CONFIG_NO_SNAPSHOT || e.X !== undefined || e.T?.De !== undefined) return;
    takeSnapshot(e, t);
}

function takeSnapshot(e, t) {
    ext(e).De = t === undefined ? NO_SNAPSHOT : t;
    e.l |= CONFIG_HAS_SNAPSHOT;
    snapshotSources.add(e);
}

/** Called before every signal or store write while set. The server-components
 * client mounts still-deferred slots before a write applies, then clears it. */ let writeHook = null;

/** @internal */ function setWriteHook(e) {
    writeHook = e;
}

/** The one guarded comparator call. A comparator that read unresolved async
 * reports a change (#4111); any other throw is `el`'s error, routed to its
 * boundary and answered "same" (D-38, #2837).
 * @internal */ function isSame(e, t, n) {
    if (e.ye === isEqual) return t === n;
    try {
        return e.ye && e.ye(t, n);
    } catch (t) {
        if (!(t instanceof NotReadyError)) return notifyStatus(e, STATUS_ERROR, t), true;
    }
}

function setSignal(e, t) {
    if (writeHook !== null) writeHook(e);
    const n = e.ue === NOT_PENDING ? e.le : e.ue;
    if (typeof t === "function") t = t(n);
    // A write landing on a guess is its truth (F-11); one on a lane's
    // derivation is the lane's staging.
        if (laneWriteHook && e.l & CONFIG_OVERRIDE) return laneWriteHook(e, t);
    // A write to a held slot is a second proposal: the writer's tick joins the
    // hold, before the equality gate (D-8).
        if (e.l & CONFIG_HELD) joinFuture(txOf(e));
    // The first commit has no previous value: the comparator never sees
    // `undefined`. A write over a shown error is a change (D-41). `custom` is
    // taken once for the gate and the netting so the default `===` runs inline
    // (CodSpeed `propagation: avoidable`); without the engine it is `null` and
    // the netting folds away.
        const o = observeFlightHook ? e.ye !== isEqual : null;
    const E = !!(e.m & STATUS_UNINITIALIZED) || !!e.T?.Te || (o === false ? n !== t : !isSame(e, n, t));
    if (!E) return t;
    // A write during snapshot capture to a source created before capture
    // captures the pre-write value now, so claims keep the server's value and
    // replay at release. Store leaves qualify too.
        if (snapshotCaptureActive) captureWriteSnapshot(e, n);
    const s = e.ue !== NOT_PENDING;
    if (!s) queuePendingNode(e);
    // F-10: a held node rewritten outside a flush keeps its flushed staging.
     else if (e.l & CONFIG_HELD && !globalQueue.Y) stashFlushed(e);
    // A staged rewrite equal to the committed value nets to it (D-8, #4483).
    // Only the verdict engine reads the netting; under the default comparator
    // `v` already is the committed object.
        e.ue = o && s && !(e.m & STATUS_UNINITIALIZED || e.T?.Te) && isSame(e, e.le, t) ? e.le : t;
    if (context !== null) notePromotedWrite(e);
    // `_time` is computed-only: writing it on a signal forks the lean shape.
        if (e.X !== undefined) e.ie = clock;
    // A re-write whose subscribers were walked with nothing recomputed or linked
    // since re-stages and stops: the walk is idempotent.
        if (s && e.we === notifyEpoch) return t;
    insertSubs(e);
    schedule();
    return t;
}

/** User-facing setter for the memo form of `createSignal(fn)`. The write
 * stages like any other, and a later source change re-derives with it as
 * `prev` (D-8, CS-R31). REACTIVE_MANUAL_WRITE marks it a user proposal until
 * a pass re-derives it or the commit lands it. */ function setMemo(e, t) {
    // Is `el`'s staging a derivation another transaction holds? Then a setter
    // composes on the committed frame it was written against and becomes `prev`
    // for the hold's re-derivation (D-8, #3612).
    const n = e.l & CONFIG_HELD && !(e.R & REACTIVE_MANUAL_WRITE) && txOf(e) !== flushTransaction;
    if (n && typeof t === "function") {
        const n = t(e.le);
        t = () => n;
    }
    // A write replaces a failed derivation's outcome without re-running `_fn`,
    // so clear STATUS_ERROR here (#4110). The error shows until the write
    // commits (D-41).
        if (e.m & STATUS_ERROR) {
        e.T.Te = e.T.U;
        notifyStatus(e, STATUS_NONE);
    }
    t = setSignal(e, t);
    // A write replaces a staged error (D-41).
        e.l &= ~CONFIG_STAGED_ERROR;
    if (n) {
        // The staging it replaced came from inputs the writer never saw: the hold
        // re-derives over the write (D-8).
        e.R = e.R & ~REACTIVE_CHECK | REACTIVE_DIRTY;
        insertIntoHeap(e, dirtyQueue);
        schedule();
    } else if (e.ue !== NOT_PENDING) e.R |= REACTIVE_MANUAL_WRITE;
    return t;
}

/**
 * Executes `fn` with the given `owner` set as the current owner. Any reactive
 * primitives (`createSignal`, `createMemo`, `createEffect`, `onCleanup`,
 * `cleanup`, etc.) created inside `fn` are attached to that owner, so they
 * are disposed when the owner is disposed.
 *
 * The classic pattern: capture the current owner with `getOwner()` inside a
 * component, then re-enter it from a callback (event handler, async resolve,
 * setTimeout) so disposables created in the callback get cleaned up with the
 * component.
 *
 * @example
 * ```ts
 * function delayed<T>(ms: number, fn: () => T) {
 *   const owner = getOwner();
 *   setTimeout(() => runWithOwner(owner, fn), ms);
 * }
 * ```
 */ function runWithOwner(e, t) {
    const n = context;
    const o = tracking;
    context = e;
    tracking = false;
    try {
        return t();
    } finally {
        context = n;
        tracking = o;
    }
}

function staleValues(e, t = true) {
    try {
        return e();
    } finally {}
}

/** Core half of `refresh()` (signals.ts): marks the next recompute a quiet
 * re-ask and schedules it. */ function markRefresh(e) {
    if (typeof e.X === "function" && !(e.R & REACTIVE_DISPOSED)) {
        // A refresh in a manual write's frame re-asks with the write as `prev`
        // (D-8, #3026). The next pass's flight is a quiet re-ask (F-9), unless an
        // input changed this tick.
        e.R = e.R & ~REACTIVE_CHECK | REACTIVE_DIRTY | (e.R & (REACTIVE_DIRTY | REACTIVE_CHECK | REACTIVE_IN_HEAP) ? 0 : REACTIVE_REASK);
        insertIntoHeap(e, dirtyQueue);
        schedule();
    }
}

export { adopter, bornInto, captureWriteSnapshot, clearSnapshots, computed, context, createEffectNode, creatorPass, creatorReads, effectStatusNotify, ext, heldRead, isEqual, isSame, joinPass, joinPassAt, markLateLinker, markRefresh, markSnapshotScope, markUnflushedStaged, notePromotedWrite, prepareComputed, pullComputed, read, readerSeesCommitted, recompute, releaseSnapshotScope, resyncUnflushedCompanions, runRefCallback, runWithOwner, serve, setAdopter, setEffectStatusNotify, setMemo, setSignal, setSnapshotCapture, setVerdict, setWriteHook, signal, slotSignal, snapshotCaptureActive, snapshotSources, spectating, stagedRead, stagedScreen, staleValues, stashFlushed, statusNotifierOf, strictReadTracked, tickBase, tracking, unflushedStaged, unflushedValue, untrack, verdict, writeHook };