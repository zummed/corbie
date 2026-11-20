import { NOT_PENDING, STATUS_UNINITIALIZED, CONFIG_GUESS, CONFIG_SLOT_NODE, STATUS_PENDING, REACTIVE_STAGED_READ, REACTIVE_FRAME_READ, REACTIVE_DISPOSED, CONFIG_VERDICT, REACTIVE_PROBE_UNANSWERED, REACTIVE_LANE_DIRTY, REACTIVE_LANE_READ, CONFIG_ASYNC, CONFIG_OVERRIDE, CONFIG_HELD, REACTIVE_RECOMPUTING_DEPS, CONFIG_AUTHORITATIVE, STATUS_ERROR, CONFIG_CHILDREN_FORBIDDEN, EFFECT_RENDER, CONFIG_MOUNT_AHEAD, REACTIVE_SCREEN_READ, CONFIG_FRAME, CONFIG_STAGED, CONFIG_LAGS, CONFIG_HELD_FRAME, REACTIVE_JOINED, REACTIVE_IN_HEAP, REACTIVE_DIRTY, REACTIVE_CHECK, CONFIG_INPUTS_PUBLISHED, CONFIG_STAGED_ERROR } from "./constants.js";

import { bornInto, ext, tracking, creatorReads, stagedRead } from "./core.js";

import { NotReadyError } from "./error.js";

import { removePendingSource, settlePendingSource } from "./async.js";

import { covers, latestOf, underOf } from "./truth.js";

import { enqueueSub } from "./heap.js";

import { installLaneHooks, GlobalQueue, newTransaction, staleReader, flushTransaction, resolveTx, liveTx, r5Hook, stagedReaders, blocked, clock, txOf, laneDirty, schedule, insertSubs, queuePendingNode, list, actionDepth, passLane, reasks, setPassLane, sameLane, hiddenHook, ownFlights, globalQueue, reruns, commitPendingNode, releaseQueues, joinFuture, transactions, holdNode, question } from "./scheduler.js";

// Lanes (plan sec. 28 replay, 2026-10-02; maintainer rulings 2026-10-01/02). A lane
// is "a new base of a transition": the sub-frame an optimistic write opens
// in the transition of the frame it is made in. It sees the screen plus its
// own guesses (plan sec. 19), breaks out of the parent's hold — its effects run now —
// and holds itself if its own derivations hit async (and, blocked, holds its
// parent). It ends when the parent lands: a guess reverts to the truth it
// covered, or lands as the truth that superseded it. A frame that does not
// park has nothing to be optimistic over: the write is void.
// Two places per node, as for any held node: `_value` is what the screen
// shows, `_pendingValue` the staging of the transaction holding it — for a
// lane's node, the lane's: a written guess or a lane pass's derivation,
// until the lane reveals (`laneSeam`), when it moves into `_value` and the
// lane records what it covered (`under`). A truth landing beneath a guess is
// the lane's parent's, kept on the lane too (`under`, CONFIG_HELD) for the
// parent's landing to commit. The lane carries the state machine: `_shown`
// once it has revealed. A direct read sees the lane's latest throughout
// (A17); an authoritative reader (`until`) sees the truth beneath it (Q3,
// 2026-10-02: "optimistic overrides are intentionally not part of
// authoritative truth").
// The seat of a pass is its node's (core.ts `recompute`): a node carrying a
// lane's derived value runs as the lane's whoever dirtied it; a derivation's
// tracked read of a lane's value moves its pass into the lane (`enterLane`);
// a leaf's never does — it reads the screen, a stale reader of a lane that
// has not shown (#3460). Lane work's values are the lane's staging, revealed
// at the next seam; its runs go to the lane's queues.
// This module is the lane engine, installed on `GlobalQueue` when something
// imports it (`createOptimistic`, or verdict.ts for its verdict lanes). The
// core keeps only bit tests and `passLane`: a program with no optimism
// carries none of this.
/** Live lanes (parented). Scanned at every seam while non-empty. */
const lanes = [];

/** A pass left a shown lane (`laneStage`): the node's notify stamp
 * (`_notifiedAt`, never negative once it notifies — `insertSubs` overwrites
 * it, so a pass that does notify clears the mark). The seam wakes its
 * readers (#3892). */ const LEFT_SHOWN = -2;

function newLane(e) {
    const n = newTransaction(true, e);
    lanes.push(n);
    return n;
}

/** `v` is lane `l`'s value of `n` from now — a guess written or re-asked, a
 * store's composed row, a guess coming back: on screen at once once the
 * lane has revealed (what it covers kept for the lane's end, `cover`),
 * staged for the reveal before. */ function setLaneValue(e, n, s) {
    if (n.Ue || e.T.Ue) {
        cover(e);
        e.le = s;
        e.T.Ue = true;
        if (e.m & STATUS_UNINITIALIZED) e.m &= ~STATUS_UNINITIALIZED;
    } else if (e.l & CONFIG_GUESS) covers.set(e, s); else e.ue = s;
}

/** What the screen shows of a node: NOT_PENDING when nothing (uninitialized). */ function screenOf(e) {
    return e.m & STATUS_UNINITIALIZED ? NOT_PENDING : guessScreen ? guessScreen(e) : e.le;
}

/** The guess layer's screen of a node (guesses.ts): a shown guess re-asked
 * keeps the shown one until its lane's next reveal (#4775); `revealed`
 * forgets it. */ let guessScreen = null;

/** The lane's record of the value the screen showed `n` before the lane
 * revealed it (a guess reverts to it, `covered`) — kept once. */ function cover(e) {
    if (!covers.has(e)) covers.set(e, e.le);
}

/** A truth landed beneath a guess: the parent's staging, held for its
 * landing to commit (CONFIG_HELD) — an older question's answer (A18), or the
 * guess's confirmation (A24). The guess stays the value. */ function landBeneath(e, n) {
    e.ue = n;
    e.l |= CONFIG_HELD;
}

/** This module's copy of scheduler.ts `guessing`: its readers here stay
 * foldable when a bundler splits lanes.ts from scheduler.ts into separate
 * chunks (a cross-chunk binding is not folded; the compiled live page). */ let guessed = false;

/** A guess was written (guesses.ts `optimisticWrite`). */ function armGuessed() {
    guessed = true;
}

// The guess layer (guesses.ts; installed with `createOptimistic` or
// `createOptimisticStore`): the arms only a written guess reaches. `take`
// — a lane takes another action's guess (`laneStage`); `guessWrite` — a
// write landing on a guess (`laneWrite`); `revertLanding` — a revert
// landing gives back the guesses its lane took (`endLanes`). Each site is
// behind CONFIG_GUESS or `guessed`, which nothing but the layer sets: a
// bundle without it folds the sites.
let takeHook = null;

let guessWriteHook = null;

let revertLandingHook = null;

function installGuessLayer(e, n, s, t) {
    guessScreen = t;
    takeHook = e;
    guessWriteHook = n;
    revertLandingHook = s;
}

/** Two kinds of member, one set (a lane is never a node). Lanes a revert
 * dissolved: the guesses they took come back at their parent's landing
 * (`endLanes`). Shown guesses a correction re-homed into their hold
 * (guesses.ts `supersede`, `dissolveLane`): the slot's value for the screen and
 * untracked reads until that hold lands (D-34), each keeps it open on its
 * own flight as it did while a guess (`lanesBlocked`, #4690); the hold's
 * landing clears them (`endLanes`). */ const marked = /* @__PURE__ */ new WeakSet;

/** A shown guess a correction re-homed into its hold (guesses.ts `supersede`,
 * `dissolveLane`): the screen's and the untracked reads' value until the
 * commit, and `latest`'s (D-34, decision 3 of 2026-11-05, #4689) — the
 * displayed slot's newest value is the guess, the truth its staging. */ function keptGuess(e) {
    return guessed && marked.has(e);
}

/** `n`'s readers are stale readers of `t` (re-derived at its landing). */ function rerun(e, n) {
    for (let s = e.o; s !== null; s = s.t) staleReader(s.u, n);
    // A store's guess carrier: the readers of the keys it changed read their
    // leaves (store/optimistic.ts).
        if (e.l & CONFIG_SLOT_NODE) GlobalQueue._n?.(e, n);
}

/** A guess's own truth is in flight: the node's own refetch — or, a store
 * leaf's, its family's derive (the slot's flight; store/optimistic.ts
 * installs the resolver). A17: the guess stands in for the flight. */ function inFlight(e) {
    return e.m & STATUS_PENDING || e.l & CONFIG_SLOT_NODE && GlobalQueue.k?.(e);
}

/** The running pass read lane `l`'s work as a derivation: it is `l`'s — or,
 * already another lane's, the frame it computes derives from both guesses
 * and cannot show one without the other (#3335, A15 for lanes): the two
 * lanes are linked — one reveal unit, each blocked while any is — while
 * their lifetimes stay their own (#2912: a guess reverts with its own
 * action, not with the one it shares a reader with). (Lanes never nest: a
 * lane is never the flush's transaction, D-37, so no guess is parented to
 * one.) */ function enterLane(e, n) {
    const s = n.R |= REACTIVE_LANE_READ, t = passLane;
    if (t === null) {
        const t = isVerdict(e);
        // A verdict lane's value (a memo of `latest` / `isPending`, F-6) read by
        // a derivation that already read the frame's staging or joined its
        // hold, or by any pass a pass that did is creating: the verdict read
        // does not break it out — it stays the frame's, as the same read inline
        // does (verdict.ts `markVerdictReader`, `route`; D-25, D-26). An
        // effect's own reads do not count (its `_type`, 1–3, carries neither bit).
        // Nor by a member of the frame a held pass of that lane's hold built,
        // on any later run (CONFIG_HELD_FRAME, `bornInto`): born in the hold's
        // world, it is the hold's own work and its run is the frame's, at the
        // landing (D-25, D-27) — as `route` keeps it for the inline `latest()`.
        // Nor by a pass that re-entered the hold holding its node at its head
        // (core.ts `recompute`: held, with no verdict posture — an async memo's
        // was dropped there — not a frame, not lagging, not a lane's node), when
        // `l` is another hold's verdict lane: re-run by the write its hold
        // holds, the pass is that hold's work and reads plainly in either read
        // order (D-25), as it does reading its hold's staging before `l`'s
        // value. Moved into `l`, it read its own hold's landing as the screen
        // (D-26) and asked a question already asked (#4399: `m1:15` again, over
        // `m0b`'s committed value beside the landing of its next question). Its
        // own hold's verdict lane is its display-ahead read: it enters that one
        // (D-25, #4050).
                let r;
        if (!(t && (creatorReads(n, n.i || s) || (r = resolveTx(e._parent), (n.l & (CONFIG_HELD | CONFIG_VERDICT | CONFIG_FRAME | CONFIG_LAGS | CONFIG_OVERRIDE)) === CONFIG_HELD && txOf(n) !== r) || n.l & CONFIG_HELD_FRAME && bornInto(n, r)))) {
            setPassLane(e);
            // The pass read a held write before its first lane read (it joined the
            // frame's future as mainline would): lane work sees the screen — the
            // seam re-derives it on the committed world next round (`stagedReaders`,
            // the park's repair), as it does a lane pass that read a staging. Not a
            // pass whose held reads were all of nodes born held (no staged read,
            // `stagedRead`): it has no committed world to re-derive on (#3969).
            // (A pass not joined: a held write beside a guess, #4336; a leaf of a
            // verdict lane — its plain reads are the screen's, D-26 `[s(), L()]`.)
                        if (s & REACTIVE_STAGED_READ && (guessed || t || s & REACTIVE_JOINED)) stagedRead(n);
        }
    } else if (sameLane(t, e)) setPassLane(e); else {
        linkLanes(t, e);
        setPassLane(e);
    }
}

/** Is `l` a verdict lane (its parent's `_verdict`)? False for none. */ function isVerdict(e) {
    return e?._parent?.Pe === e;
}

/** One link group (a shared array) for both lanes' groups. */ function linkLanes(e, n) {
    const s = e.nn ?? (e.nn = [ e ]);
    const t = n.nn;
    if (t === s) return;
    if (t === null) {
        s.push(n);
        n.nn = s;
    } else for (let e = 0; e < t.length; e++) {
        s.push(t[e]);
        t[e].nn = s;
    }
}

/** A linked lane blocked on its own account blocks the group — `blocked(k)`
 * without the group recursion. (A guess in `k` whose own truth is in flight
 * blocks `k`'s parent, not `k` — nor, through the link, `l`: the guess
 * shows, A17.) */ function linkBlocked(e) {
    const n = e.nn;
    if (n !== null) for (let s = 0; s < n.length; s++) {
        const t = n[s];
        if (t !== e && (t.tt !== 0 || ownFlights(t) || nestedBlocked(t))) return true;
    }
    return false;
}

function unlink(e) {
    const n = e.nn;
    if (n === null) return;
    const s = n.indexOf(e);
    if (s !== -1) n.splice(s, 1);
    e.nn = null;
}

/** Verdicts (maintainer, 2026-10-01): `isPending` and `latest` are
 * optimistic state the system supplies — a verdict is a guess ("pending,
 * until this lands"; "the proposal is the value") decided at the seam like
 * a user's, and its readers break out of the hold the same way: work of a
 * lane under the holder, shown now, re-derived at the landing, ending with
 * it. Judged at the seam like any lane: blocked while a verdict reader's
 * own derivation is in flight (lane rule 3), shown otherwise. */ function verdictLane(e) {
    return e.Pe ??= newLane(e);
}

/** A lane pass's node (core.ts `recompute`, before its staging; a pending
 * propagation under a lane pass, async.ts): listed, carrying the lane's
 * value from now (a derivation of a guess — its readers become lane work).
 * Entering the lane supersedes a frame staging the node carried: this
 * pass's answer replaces it, and the frame's hold on it is over. An
 * effect's value slot is private (no lane value). A pass in a lane's seat
 * that read none of the lane's world has left it (false): its result is the
 * frame's — a derivation whose branch no longer reaches the guess — and no
 * hold the lane took over from a transaction stays on it (#3698: lane work
 * never makes its node transaction work; a born-held effect's hold went to
 * the lane). A pass interrupted before it got there (pending, errored) did
 * not leave (A30: it never got there). A first pass under a lane is the
 * lane's (ruling A); a guess is written, not derived — it never leaves this
 * way. `errored`: the pass has no answer — or, a lane pass's pending
 * propagation (async.ts `propagateStatus`), the node gone pending. */ function laneStage(e, n, s, t) {
    // F-2's producer clause (S-57): a pass this flush dirtied — it read the
    // flush's staging — that ran as a lane's and asked: the flush holds on
    // that flight and reveals with the lane, whatever the async's source (the
    // lane took the only question).
    if (!s && e.m & STATUS_PENDING && e.R & REACTIVE_STAGED_READ) {
        // Pending on a flight it waits on as the frame's work (core.ts `read`:
        // it read the flush's staging, then the flight that staging re-asked)
        // with none of the lane's world read: it left the lane, its pending the
        // frame's — a boundary catches it as a mainline pass's (F-4).
        if (!(e.R & REACTIVE_LANE_READ) && !(e.l & CONFIG_ASYNC)) t = false; else laneWait(e, n);
    }
    if (!(s || t || e.R & REACTIVE_LANE_READ || e.l & CONFIG_GUESS)) {
        const s = e.T;
        if (s !== null) {
            const n = s.Re;
            if (n?.P) {
                // Left a shown lane: this pass's answer is the frame's. While a
                // transaction's flush runs the pass and the lane's frame is still
                // held, what the screen shows stays the screen until the landing
                // (`_value` keeps the lane's value), and the answer — a change
                // against it — reaches the screen with the frame's commit. Otherwise
                // the answer is judged against the committed value the lane covered
                // (`under`), and the seam wakes the readers (#3892; always for a pass
                // outside any transaction's flush, #4367): a return to the committed
                // value notifies nobody itself, and a pass that does notify has
                // woken them (an effect has no comparator, #4557).
                if (n.Ue && !(e.m & STATUS_PENDING) && (!flushTransaction || !blocked(resolveTx(n._parent)))) {
                    if (e.T.Ue) e.le = covers.get(e);
                    e.we = LEFT_SHOWN;
                }
                covers.delete(e);
                s.Ue = false;
                s.Re = null;
            }
            // The lane's staging of it is void, and so is what its readers derived
            // from it in the lane: they re-derive, or a lane run released after
            // this pass shows the void value and a re-derivation equal to their
            // committed one notifies nobody (a Show's condition read the hold's
            // selection through the lane, F-1, F-7, D-34, #4990).
                        if (e.ue !== NOT_PENDING) insertSubs(e);
            e.ue = NOT_PENDING;
        }
        e.l &= -1050881;
        // A store derive's pass (a projection's): the leaves it wrote in the
        // lane's seat leave with it, what it wrote this pass the frame's staging
        // (the screen kept until the landing, as above). Left in the lane, they
        // revealed with it ahead of the frame they derive from (F-1, D-33).
                if (guessed) for (let s = 0; s < n.ln.length; s++) {
            const t = n.ln[s];
            if (t.O?.rr?.node === e && t.T.Re === n) {
                const e = t.ue;
                laneStage(t, n, false, false);
                if (e !== NOT_PENDING) queuePendingNode(t), t.ue = e;
            }
        }
        return false;
    }
    if (!e.i) {
        if (!(e.l & CONFIG_OVERRIDE)) {
            // A hold's staging of it is void in a guess lane: the readers that
            // derived from that staging re-derive, though the lane's value may
            // equal the screen (no change to notify them by) — their held runs
            // carry the void staging, and the hold's landing would apply them over
            // the guess (#4825; as a mainline pass superseding one does, core.ts
            // `recompute`, #4378). A verdict lane's display-ahead pass leaves the
            // frame's question with its readers (S-51).
            if (e.l & CONFIG_HELD && e.ue !== NOT_PENDING && n._parent.Pe !== n) insertSubs(e);
            e.ue = NOT_PENDING;
            e.l = e.l & -1048833 | CONFIG_OVERRIDE;
            ext(e);
        }
        // A written guess whose own pass is the lane's work — its source is a
        // derivation of another guess (a guess over `config().courier` under a
        // country guess) — is confirmed or corrected by the lane: a derivation
        // of the lane from here (its value the lane's, its fate the lane's), no
        // longer a written guess. A pass with no answer (pending, errored)
        // leaves the guess.
        // Nor a pass of another action's guess: the re-ask is this lane's new
        // question, which the guess (over the old one) does not answer — the
        // lane waits for its own answer (F-1, F-6, #4336). (A guess of the
        // lane's own action, D-33's entangled slot, still stands in.)
        // (Read behind `guessed`: a bundle without optimism drops the reads.)
                if (guessed && e.l & CONFIG_GUESS && e.T.Re?.P && resolveTx(e.T.Re._parent) !== resolveTx(n._parent)) takeHook(e, n); else if (!t) e.l &= ~CONFIG_GUESS;
    }
    // A leaf has no lane value to seat it (`recompute`): one left waiting on
    // the lane's flight runs its next pass as the lane's — the landing that
    // wakes it is the lane's re-staging (A31), and the lane holds its render
    // effects until its derivations land (A17; #3766, F5). Not a leaf whose
    // seat is its frame's, reached by a propagation from a node the screen has
    // a value of: one a transaction holds (born held — its frame waits on it,
    // A29), or one whose own mainline pass is running (it pulled the node;
    // `laneRead` decides — its frame holds). Its pending is the frame's: queued
    // for the commit sweep as mainline's (async.ts). A node born in the lane
    // has nothing on screen: its leaf waits as the lane's. A held leaf stays
    // its frame's even when the node reached it has no value yet: a mount born
    // held whose derivation now waits on a lane's flight (#4998, D-2, F-1).
     else if (t) {
        if (t !== true && (e.l & CONFIG_HELD || !(t.m & STATUS_UNINITIALIZED) && e.R & REACTIVE_RECOMPUTING_DEPS) && !e.T?.Re?.P) {
            queuePendingNode(e);
            return false;
        }
        e.R |= REACTIVE_LANE_DIRTY;
    }
    list(e, n);
    return true;
}

/** A lane's end, two ways. `into === null`: the parent landed — the lane's
 * values are the truth now: a guess lands the truth staged beneath it or
 * reverts to the one it covered (`_value`, untouched by the guess); a
 * derivation's latest value commits (its frame with it); what the screen
 * showed and changes notifies. Otherwise the lane dissolves into `into`
 * (the parent, on a correction): its values are void — what it showed stays
 * the screen (`_value` takes it, so the parent's re-derivation compares
 * against what is seen), a truth staged beneath a guess is the parent's to
 * land, and the runs it held (showing the void guess) are dropped: the
 * parent's re-derivation makes the runs that show. */ function dissolveLane(e, n, s) {
    const t = lanes.indexOf(e);
    if (t !== -1) lanes.splice(t, 1);
    unlink(e);
    // Its verdicts end with it (a correction's dissolution reaches here before
    // `endLanes` would): the readers re-derive from the truth's notification.
        const r = e.Pe;
    if (r !== null) {
        e.Pe = null;
        if (lanes.indexOf(r) !== -1) dissolveLane(r, null);
    }
    for (let t = 0; t < e.ln.length; t++) {
        const r = e.ln[t];
        const l = r.T;
        // A member disposed since it joined the lane is no reader any more
        // (F-3, D-11): dead, it stays at its last committed value (#3024). Nothing
        // here may queue it: recompute's head wipe would drop REACTIVE_DISPOSED
        // and run it again in a torn-down tree (#4613).
                if (r === s || l.Re !== e || r.R & REACTIVE_DISPOSED) continue;
        const o = r.i;
        const i = r.l & CONFIG_GUESS;
        // What the screen shows of it.
                const u = r.le;
        // A guess's truth: a landing held beneath it, else the value it covered.
                const E = i ? underOf(r) : NOT_PENDING;
        r.l &= -10241;
        r.T.Ue = false;
        covers.delete(r);
        if (n === null) {
            // The parent landed: a guess lands the truth beneath it or reverts to
            // the one it covered; a derivation's latest (its staging, else what it
            // shows) commits, its frame with it; what the screen showed and
            // changes notifies.
            // A derivation the revert re-derives (dirtied by the guess's
            // notification — listed before it), or one still in flight, shows the
            // lane's answer beside inputs that are the truth now: a fresh reader
            // of its flight observes it (#3648, #3651; #3305's commit beneath a
            // flight). A first load the lane produced nothing for stays loading
            // until its flight answers (D-22, A19 exc. 1): nothing is staged.
            l.Re = null;
            if (i) r.ue = E;
            commitPendingNode(r);
            if (!o) {
                if (r.R & (REACTIVE_IN_HEAP | REACTIVE_DIRTY | REACTIVE_CHECK) || r.m & STATUS_PENDING) r.l |= CONFIG_INPUTS_PUBLISHED;
                if (!r.ye || !r.ye(u, r.le)) insertSubs(r);
            }
            // A stale reader's own error — its pass (a memo's or a render
            // effect's) threw over the committed value of what the parent held
            // (A10), served as the screen — is no answer of the world landing now:
            // it re-derives on it (A15; D-39, the landing is the new answer). The
            // landing's `reruns` skips an errored reader: its error is an errored
            // source's (#3945).
                        if (r.R & REACTIVE_FRAME_READ && r.m & STATUS_ERROR && l.U.source === r) enqueueSub(r);
        } else if (i || guessed && r.l & CONFIG_SLOT_NODE && r.ue !== NOT_PENDING) {
            // A guess of a dissolving lane: its truth — a landing held beneath it,
            // else the value it covered — is the parent's to land, and its readers
            // re-derive from it there. What the screen showed stays the screen
            // until then (A18 (c)): `_value` has it. A store slot the lane
            // composed (a keyed re-base) over a truth staged on it is no
            // derivation anything re-derives: it lands that truth the same way
            // (D-34, #4642).
            if (i) r.ue = E;
            if (e.Ue && guessed) marked.add(r);
            holdNode(r, n);
            insertSubs(r);
        } else {
            // A correction: the lane's derivations are void. Shown, what the
            // screen showed stays the screen (committed, inputs published — every
            // one re-derives); never shown, nothing of it reached the screen (a
            // node born in the lane stays uninitialized; its frame goes). One in
            // flight is asking the void world's question: its landing is nobody's
            // answer — it re-asks from the truth (whose notification may not reach
            // it: an input's truth equal to its committed value is silent).
            l.Re = null;
            if (!o) {
                // Its readers re-derive unless the lane was held: a shown lane's runs,
                // released below, show the void staging, and its own re-derivation
                // may land on what the screen shows and notify nobody (F-1, D-34,
                // #4971).
                if (!e.un && r.ue !== NOT_PENDING) insertSubs(r);
                // A staged error is a staging too: void with it, or the commit below
                // would publish the empty slot as the node's error (#4748).
                                r.ue = NOT_PENDING;
                r.l &= ~CONFIG_STAGED_ERROR;
                if (r.m & STATUS_PENDING) {
                    l.Ee = null;
                    enqueueSub(r);
                }
            }
            if (e.Ue) {
                // A derivation born in the lane (its staging, voided above, was its
                // only value) whose frame stays: nothing in the truth may notify it
                // (a row the truth keeps, its fields equal) — it re-derives on the
                // truth now (D-34, F-1; #4861).
                const n = !o && r.m & STATUS_UNINITIALIZED;
                commitPendingNode(r);
                if (!o) {
                    r.l |= CONFIG_INPUTS_PUBLISHED;
                    if (n) enqueueSub(r);
                }
                // A render effect born in the lane's frame (it never ran) whose
                // first run the held lane parked, dropped below: the frame stays, so
                // the row it draws must too — it runs on the committed world (F-1,
                // #4831). (A lane's pass of an effect always owes a run.)
                 else if (e.un && r.ge === undefined) enqueueSub(r);
            } else {
                if (l.me !== null || l.Le !== null) GlobalQueue.Ce(r, false, true);
                r.l &= -1048961;
            }
        }
    }
    // The parent landed: the runs the lane held (blocked on its own judgment
    // alone — a zombie it could not dispose) are the landing's. A correction
    // of a SHOWN lane keeps its runs too: "what it showed stays the screen"
    // means the runs this round's lane passes queued (the body-end seam judges
    // before the lane seam would have released them) must reach the screen —
    // their derivations' values are the committed ones now, and a
    // re-derivation that lands on the same value notifies nobody (a mapArray
    // whose rows the lane pass already built). A never-shown lane's runs are
    // void: nothing of it reached the screen. So are the runs a blocked lane
    // parked: they show a re-guess the seam never revealed, beside derivations
    // that keep what the screen showed.
        if (n === null || e.Ue && !e.un) releaseQueues(e); else e.rn[0].length = e.rn[1].length = 0;
}

/** Lanes. A lane's node is read by its reader's kind: an authoritative
 * reader (`until`) sees the truth — staged beneath a guess, else committed
 * — and is no lane's; an untracked read sees the lane's latest value (A17:
 * "direct read shows optimistic"; A28); a children-forbidden reader sees
 * the frame (A32). A render effect in the frame's seat reading a lane the
 * seam found blocked, or a member whose own flight is up (held or not — the
 * lane-membership ruling, 2026-10-05), sees the screen, re-derived at the
 * reveal (#3460's stale reader) — or, the node a
 * flight that has never shown (nothing to show), waits on it: its own frame
 * holds (A15, #3334), not the lane's parent. Any other tracked reader is
 * the lane's work from this read — a derivation; a leaf reading a lane that
 * has shown, or one the seam has not judged yet (its run waits on the
 * verdict: released if the lane shows, held if it blocks) — and sees the
 * lane's latest. NOT_PENDING falls through: lane work's read of a pending
 * member throws like any. A verdict reader entering a lane the seam has
 * not judged reads ahead of it like a staging read: if the seam holds the
 * lane, the pass re-derives on the screen (`entered`, #4284) — `latest()`
 * beside the lane's nodes tears ahead of the hold however the reads are
 * grouped into effects. */
/** #4788: the lanes whose guesses an action read imperatively (its body, a
 * setter: `laneRead`, a store setter's draft), each with the question the
 * reading action asks (`question`): a guess that action writes next derives
 * from them (guesses.ts `optimisticWrite`); another action's reads are not
 * its own. Kept while the lane read lives (guesses.ts `applyGuessesAt`). */ const readGuesses = [];

function laneRead(e, n) {
    const s = n.l & CONFIG_GUESS;
    const t = txOf(n);
    // A lane a correction folded under (D-34): store/optimistic.ts reads it.
    // Before the authoritative arm: an `until` outside the lane reads the
    // same truth there, and joins the hold it folded into — its posture over
    // a foreign hold (D-31), which the truth read alone would not take.
        const r = e !== null && t.folded;
    if (r) {
        const s = r(e, n);
        if (s !== NOT_PENDING) {
            return s;
        }
    }
    if (e !== null && e.l & CONFIG_AUTHORITATIVE) {
        return GlobalQueue.En(n);
    }
    const l = n.m;
    // An errored member's lane value predates its error; the error is the answer (#4106).
        if (!s && l & STATUS_ERROR) {
        // The error still belongs to this lane. A downstream memo must select
        // the same update before read() throws, as it does for a lane value.
        if (e !== null && tracking && !(e.l & CONFIG_CHILDREN_FORBIDDEN)) enterLane(t, e);
        return NOT_PENDING;
    }
    if (e === null) {
        // #4788 (D-33): an action's imperative read of a guess — its body, a
        // setter — that a guess it writes next may derive from.
        // A store target's carrier is noted per key (store/optimistic.ts).
        if (s && actionDepth && !(n.l & CONFIG_SLOT_NODE)) readGuesses.push(t, question);
        return latestOf(n);
    }
    const o = !s && l & STATUS_PENDING;
    if (!tracking || e.l & CONFIG_CHILDREN_FORBIDDEN) {
        // An untracked read inside lane work derives from the lane like a
        // tracked one, minus the subscription — `untrack` is about
        // dependencies, not about which world a pass derives from (a mapper's
        // row reads under its root, store/map): the pass enters the lane (two
        // lanes read by one pass link, as for tracked reads). Outside lane
        // work, the screen.
        if (passLane === null || e.l & CONFIG_CHILDREN_FORBIDDEN) {
            return screenOf(n);
        }
    } else if (e.i === EFFECT_RENDER) {
        // A flight whose value has never shown: nothing to show — the leaf
        // waits on it, and its own frame holds (A15, #3334), not the lane's
        // parent. The landing lands the frame: its own pass (the flush joined
        // its transaction) reads the answer it waited on — the lane's staging,
        // revealed at this seam. Not a member pending only on `refresh()`
        // re-asks: a re-ask asks no new question and holds nothing new (D-34
        // ¶3), so another lane's leaf reads its screen and shows that lane's
        // guess beside it (F-11, #4889).
        const r = o && !n.T.Ue && !reasks(n);
        // D-28: a leaf a verdict read moved into the lane still throws on such
        // a flight — the pass leaves the lane, and its frame holds as if the
        // read had come first (S-39, #4275). Nor does a verdict lane's seat carry
        // the leaf into another lane (no link): it reads that lane as from the
        // frame's seat (#4284).
                const l = passLane;
        const i = isVerdict(l);
        if (i && (r || l !== t)) setPassLane(null);
        if (passLane === null || !sameLane(passLane, t)) {
            // (A lane that has never shown: the screen is the committed value.)
            if (r && t.Ue) return NOT_PENDING;
            // A held leaf reads a shown verdict lane's staging as its frame's
            // (#4275, S-39). Not a guess lane's: there it is the lane's, for its
            // next reveal — read as the frame's, the leaf's run waited with the
            // frame and the reveal never re-ran it; a guess into a log whose lane
            // has shown stayed hidden behind another action's refresh (F-11,
            // D-26, #4666). It reads the screen and re-derives at the reveal
            // (below), or enters the lane.
                        if (!s && t.Ue && isVerdict(t) && n.ue !== NOT_PENDING && e.l & CONFIG_HELD && flushTransaction !== null && resolveTx(flushTransaction) === txOf(e)) {
                return n.ue;
            }
            if (t.un || o) {
                // S-58 (D-25): a mount a probing pass opened (CONFIG_MOUNT_AHEAD) is
                // born held on its first pass's plain read of the lane's flight, not
                // served the screen: served, it initializes with S-46's frame, which
                // the lane reveals at its landing. It waits on the flight instead
                // (`read` throws the flight's NotReady below the lane arm).
                if (o && e.l & CONFIG_MOUNT_AHEAD && e.m & STATUS_UNINITIALIZED) return NOT_PENDING;
                // Re-derived at the reveal; once per pass (the pass may also be a stale
                // reader of a transaction, REACTIVE_FRAME_READ — both reruns apply).
                // Before the throw below: a leaf that waits on a lane that has never
                // shown has no flight to settle it — only the reveal re-derives it,
                // and its pending holds its frame until then (#4338). Not a member's
                // own flight in a lane that showed and is not held: the reveal has
                // nothing new for the leaf, which re-derives when that flight lands
                // — re-queued at every reveal, each re-derivation read the same
                // flight and queued the next, and the flush never settled (#4840).
                                if (!(e.R & REACTIVE_SCREEN_READ) && (t.un || !t.Ue)) {
                    e.R |= REACTIVE_FRAME_READ | REACTIVE_SCREEN_READ;
                    t.fn.push(e);
                }
                // Nothing on screen yet (uninitialized: NOT_PENDING).
                                const s = screenOf(n);
                if (s === NOT_PENDING) throw new NotReadyError(null);
                return s;
            }
            // A verdict reader reads ahead of the seam's verdict: re-derived on the
            // screen if the lane holds (#4284).
                        if (e.l & CONFIG_VERDICT && !isVerdict(t)) entered.push(e);
        }
    }
    // A render effect of the lane reading a member that waits on `refresh()`
    // re-asks only reads what it shows: a re-ask holds nothing new (D-34 ¶3,
    // `blockedBy` lets such a member's lane reveal), so a row the lane builds
    // binds beside the reader that shows it (F-1, #4864). A boundary's tree
    // (CONFIG_FRAME, not an effect) flattens what its insert effects would show: a shown
    // member reads as it shows, the lane's guesses on the screen (D-33, #4999).
        enterLane(t, e);
    return o && !(guessed && e.i === EFFECT_RENDER && reasks(n)) && !(e.l & CONFIG_FRAME && !e.i && n.T.Ue) ? NOT_PENDING : latestOf(n);
}

/** A guess in `l` whose own truth is in flight (its refetch — authoritative:
 * the landing supersedes, not the body's end). */ function guessFlights(e) {
    for (let n = 0; n < e.ln.length; n++) {
        const s = e.ln[n];
        if (s.l & CONFIG_GUESS && s.T.Re === e && inFlight(s)) return true;
    }
    return false;
}

/** The seam's verdict on a lane. Blocked — its own flight is up — nothing
 * moves: the screen keeps what it shows, the lane's runs wait. Unblocked,
 * the lane shows: its values are the screen from now (`_shown`), the
 * staging its passes made since the last reveal is promoted, their frames
 * (children) apply, the stale readers re-derive (#3460), its runs go. */ function laneSeam(e, n) {
    // Blocked — or one of its passes read a write the frame holds this seam:
    // its runs wait, the pass re-derives on the screen next round.
    // Re-based over a question still parked on another read (#4525): its rows
    // are that answer's, shown only with the question (F-1, D-34 ¶3, #4559).
    if (n || blocked(e) || e.over && blocked(liveTx(e.over) || e)) {
        e.un = true;
        return;
    }
    e.un = false;
    e.Ue = true;
    const s = resolveTx(e._parent), t = s.fn.filter(n => n.Ae && n.T?.Re === e);
    if (reruns(e)) revealing.push(s);
    // The parent lands this seam and re-derives readers whose runs the lane
    // stashed, voiding them (`reruns`: they are for the frame being replaced).
    // Beside other runs of the lane they show first: the lane's readers show
    // its answer together (F-6), and the re-derivation follows next round
    // (#4677: `pending: true` on an async memo over `isPending(id)` beside a
    // binding still `false`). Restoring a run `reruns` skipped changes nothing.
     else if (e.rn[0].length > t.length) e.rn[0].unshift(() => t.forEach(e => e.Ae = true));
    for (let n = 0; n < e.ln.length; n++) {
        const s = e.ln[n];
        // (Not once a pass since has notified them — D-37: a lane does not
        // replay a run the screen made.)
                if (s.we === LEFT_SHOWN) insertSubs(s);
        const t = s.T;
        if (t.Re !== e) continue;
        // The lane's value is the screen's: a guess moves from the lane's
        // record to `_value` (what it covered takes its place there, for the
        // revert); a derivation's staging commits (a staged error publishes,
        // D-41; the frame it built retires the one it replaces). A truth held
        // beneath a guess stays its staging. An effect's value slot is private.
                if (!s.i) if (s.l & CONFIG_GUESS) {
            guessScreen(s, true);
            if (!s.T.Ue) {
                const e = covers.get(s);
                covers.set(s, s.le);
                s.le = e;
                s.T.Ue = true;
                if (s.m & STATUS_UNINITIALIZED) s.m &= ~STATUS_UNINITIALIZED;
            }
        } else if (s.ue !== NOT_PENDING) {
            cover(s);
            commitPendingNode(s);
            s.T.Ue = true;
        }
        // The frame this pass built is the screen's now; the one it replaced
        // goes (as `commitPendingNode` retires a committed pass's).
                if (t.me !== null || t.Le !== null) GlobalQueue.Ce(s, false, true);
        s.l &= ~CONFIG_STAGED;
    }
    releaseQueues(e);
}

/** The parent landed: its lanes end, nested first. (None is blocked: a
 * blocked lane blocks its parent.) */ function endLanes(e) {
    if (guessed) for (const n of e.ln) marked.delete(n);
    // A revert landing (#4336): the guesses its lane took from another
    // action come back in this frame (guesses.ts `revertLanding`).
        if (guessed) revertLandingHook(e);
    for (let n = lanes.length - 1; n >= 0; n--) {
        const s = lanes[n];
        if (resolveTx(s._parent) !== e) continue;
        endLanes(s);
        dissolveLane(s, null);
    }
}

/** A blocked lane blocks its parent (maintainer, 2026-10-01): the lane's
 * pending work derives from the frame's write through the guess, so the
 * frame is not complete without it (A15) — landing it would commit the write
 * beside a derivation still showing the old one. A guess whose own flight is
 * up likewise: the question was asked in this frame. So does a shown guess
 * a correction re-homed into `t` (`marked`): still the screen's and the
 * untracked reads' value (F-11, D-34), its flight — the action's `refresh()`
 * after an outside re-ask corrected it — is the answer it waits for, as
 * before the correction (D-7, D-35). With no render effect pending on that
 * flight, `t` landed at the body's end: effects and untracked reads saw the
 * re-ask's answer until the refresh landed, while rendered readers kept the
 * guess (#4690). Not a lane's: guessed again there, its flight blocks the
 * parent, not the lane (A17, `linkBlocked`). */ function lanesBlocked(e) {
    if (guessed && !e.P && e.ln.some(e => marked.has(e) && inFlight(e))) return true;
    // A lane flight one of the frame's own passes asked (`laneWait`, S-57):
    // the frame holds on it and reveals with the lane. Kept on the lane, and
    // the frame found through `over`, so a frame merged since still waits.
        if (!e.P) for (let n = 0; n < lanes.length; n++) {
        const s = lanes[n];
        if (s.waits !== undefined && s.over !== undefined && liveTx(s.over) === e && s.waits.some(e => e.m & STATUS_PENDING)) return true;
    }
    // S-46 (D-25): a mount a display-ahead read gates attaches at the
    // landing; the verdict lane that opened it reveals with that landing, not
    // ahead of it (shown now, the branch attached with its hole empty, #4417,
    // or with the committed value a re-derived mount read).
        if (e.P && e._parent.Pe === e && (e.Gt && holdsOn(resolveTx(e._parent)) || heldMountUnder(null, e))) return true;
    // A frame flight the seam parked beside its reader's lane pass (R5) is the
    // hold's question still: the hold waits on it (`park`).
        if (!e.P && r5Hook && r5Hook(null, e)) return true;
    return e.P && linkBlocked(e) || nestedBlocked(e);
}

/** Mounts a probing pass opened and a hold holds (CONFIG_MOUNT_AHEAD,
 * born held render effects): collected from the live transactions' node
 * lists — each list scanned once past where it was last scanned, so a
 * node is looked at once in its life — and pruned as they attach or go
 * away. Almost always empty: a born-held mount is rare, and every check
 * below is a walk of this list, never of the holds. */ const aheadMounts = [];

function collectMounts() {
    for (let e = 0; e < transactions.length; e++) {
        const n = transactions[e], s = n.ln;
        for (let e = n.scanned ?? 0; e < s.length; e++) {
            const n = s[e];
            // (A node re-held into a second transaction is listed twice; the
            // second entry is pruned with the first.)
                        if (n.l & CONFIG_MOUNT_AHEAD) aheadMounts.push(n);
        }
        n.scanned = s.length;
    }
}

/** The hold `t` waits: an action open in its frame, or a flight it holds. */ function holdsOn(e) {
    return e.tt !== 0 || e.ln.some(e => !e.i && e.m & STATUS_PENDING && !(e.R & REACTIVE_DISPOSED));
}

function shownBy(e) {
    for (let n = e.o; n !== null; n = n.t) if (n.u.i === EFFECT_RENDER && !hiddenHook?.(n.u)) return 1;
}

/** A collected mount still held, uninitialized, on screen and waiting —
 * on its own flight, an action open in its frame, or a flight its frame
 * holds — under `o`, or under one of lane `l`'s nodes (by its owner
 * chain). */ function heldMountUnder(e, n) {
    collectMounts();
    for (let s = aheadMounts.length - 1; s >= 0; s--) {
        const t = aheadMounts[s];
        if (!(t.m & STATUS_UNINITIALIZED) || !(t.l & CONFIG_HELD) || t.R & REACTIVE_DISPOSED) {
            aheadMounts.splice(s, 1);
            continue;
        }
        const r = resolveTx(t.T.Re);
        if (
        // A memo mount is on screen through the render effect that reads it
        // (#4806: a fragment, text beside a hole, returned to the slot). One
        // a memo reads is that memo's: a boundary the mount builds around it
        // shows its fallback (#3851).
        (t.i ? hiddenHook?.(t) : !shownBy(t)) || !(t.m & STATUS_PENDING || holdsOn(r))) continue;
        for (let s = t._parent; s !== null; s = s._parent) if (n !== null ? s.T?.Re === n : s === e) return true;
    }
    return false;
}

/** F-2 (S-57): a pass this flush dirtied — it read the flush's staging —
 * ran as a lane's (a derivation of a guess, or a `latest`-only asker in a
 * verdict lane) and asked: its flight is the lane's, so the flush would
 * have nothing of its own to wait for and would publish the write beside a
 * stale derivation of it (F-1). The flush joins a frame instead and holds
 * on the flight, revealing with the lane at its landing. */ function laneWait(e, n) {
    // A guess's lane (the row's shape). A verdict lane's `latest`-only asker
    // re-asked by an outside write is the same sentence, not built: the
    // write publishes as before.
    if (!globalQueue.Y || n._parent.Pe === n) return;
    const s = flushTransaction;
    // The hold's own work (the lane's parent is this flush's frame): it
    // already waits on its lane (`nestedBlocked`), a round after the reveal.
        if (s !== null && resolveTx(n._parent) === s) return;
    // Entangled: the lane's work reads the frame's staging as its own world
    // (core.ts `frameRead`, the `over` arm) and the lane holds while the
    // frame does (`laneSeam`) — one question, one reveal. A lane already
    // over another live frame (a keyed rebase over a landing's answer,
    // #4536) cannot take a second: the write publishes as before, beside the
    // lane's answer to its old question (the limit of this build).
        const t = n.over && liveTx(n.over);
    if (t && t !== s) return;
    joinFuture(null);
    n.over = flushTransaction;
    (n.waits ??= []).push(e);
    // Not a staged reader to re-derive on the committed world at this seam:
    // that would ask the question of the old world again.
        const r = stagedReaders.indexOf(e);
    if (r !== -1) stagedReaders.splice(r, 1);
}

/** Parents whose lane revealed this seam with readers to re-derive there
 * (`laneSeam`): the lane shows ahead of its parent (F-6), so the parent lands
 * a round later — after those readers have shown the lane's answer — even
 * when nothing else holds it (#4393). */ const revealing = [];

/** The lanes under `t` (its own; a link group's are each other's). */ function nestedBlocked(e) {
    if (revealing.indexOf(e) !== -1) return true;
    for (let n = 0; n < lanes.length; n++) {
        const s = lanes[n];
        if (resolveTx(s._parent) !== e) continue;
        if (blocked(s) || guessFlights(s)) return true;
    }
    return false;
}

/** A write landing on a lane's node (core.ts `setSignal`: a user's write, an
 * async landing). On a guess it is its truth (A18 — an async guess's own
 * landing), judged against the guess. On a derivation it is the lane's
 * latest: its staging once the lane has shown (promoted at the next
 * reveal), its slot before; the lane's members re-derive as its work. */ function laneWrite(e, n) {
    // A write on a guess: the guess layer's (guesses.ts `guessWrite`) —
    // judged against the guess, or held beneath it; false: a store slot's
    // guess its derive's re-ask in another action's lane writes is that
    // lane's derivation from here (store `_slotTake`, #4336).
    if (e.l & CONFIG_GUESS && guessWriteHook(e, n)) return n;
    const s = txOf(e);
    if (e.ye && e.ye(latestOf(e), n)) return n;
    e.ue = n;
    if (e.X !== undefined) e.ie = clock;
    laneDirty(e, s);
    schedule();
    return n;
}

/** The lanes alive before this seam's guesses opened theirs (a prefix of
 * `lanes`; nothing dissolves one between here and the seam loop): a lane
 * born at the seam has no work to judge yet (its passes run next round) —
 * it is neither shown nor blocked until it has. */ let judged = 0;

/** The guess layer's seam (guesses.ts `applyGuessesAt`), before its
 * guesses open their lanes. */ function markJudged() {
    judged = lanes.length;
}

/** The effects that derive plainly from a landing this round that
 * re-derives a stale reader (A15) reading it as committed: its held runs
 * wait for the re-derivation (`land`), and these runs are that frame too —
 * they wait with them (verdict.ts `attachWaits`; F-1, F-7, #4677). The walk
 * stops at verdict readers: what derives from a display-ahead read shows
 * now (F-6, D-25, #4398). Dropped at the end of the round's effect phase. */ let waiting = null;

/** A transaction lands: its lanes end, nested first (`endLanes`). */ function landing(e) {
    walk(e);
    endLanes(e);
}

/** Read before the lanes end: a lane's runs are its reveal's (F-6). */ function walk(e) {
    // As `reruns` judges it: a stale-reader memo re-derives onto a question of
    // its own, and the landing's held runs wait for a re-derivation.
    if (!e.fn.some(e => !e.i && e.R & REACTIVE_FRAME_READ) || !e.fn.some(e => (e.R & REACTIVE_FRAME_READ || e.l & CONFIG_VERDICT) && (e.i || e.l & CONFIG_HELD))) return;
    if (!waiting) waiting = new Set, globalQueue.rn[1].push(() => waiting = null);
    const n = e.ln.filter(e => !(e.l & CONFIG_VERDICT));
    for (let e; e = n.pop(); ) for (let s = e.o; s; s = s.t) {
        const e = s.u;
        if (waiting.has(e) || e.l & CONFIG_VERDICT || e.T?.Re?.P) continue;
        waiting.add(e);
        if (!e.i) n.push(e);
    }
}

/** `r`'s pass read a node a transaction holds (#4439). */ function readsHeld(e) {
    for (let n = e.C; n; n = n.G) if (n.S.l & CONFIG_HELD) return true;
    return false;
}

const laneSeams = e => {
    // Passes that read a write the parked frame holds as the screen
    // (`stagedReaders`: lane work, a verdict reader before the verdict): they
    // re-derive on the committed world next round (and register as stale
    // readers of the transaction there); a lane's runs wait that round — a
    // lane sees the screen plus its own guesses.
    if (revealing.length) revealing.length = 0;
    // #4661 (F-6): a verdict reader whose pass read a flight of the frame
    // ahead of it (`latest`, verdict.ts `pendingVerdict`) before any plain
    // read of one is no frame reader of it, though its stale read of the
    // frame's staging (A10, `stagedScreen`) marked it one
    // (REACTIVE_FRAME_READ, which `blockedBy` counts): the mark comes off and
    // the reader holds nothing. It keeps what it read — the committed value
    // ("It should read the committed value, we can't reveal early outside of
    // latest. This is intentional tearing.", maintainer, 2026-11-03) — and the
    // flight's own landing re-derives it, not the frame's.
        for (const n of ahead.splice(0)) n.R & REACTIVE_FRAME_READ && (n.R ^= REACTIVE_FRAME_READ, 
    e?.fn.splice(e.fn.indexOf(n) >>> 0, 1));
    const n = [];
    if (e !== null) for (let s = 0; s < stagedReaders.length; s++) {
        const t = stagedReaders[s], 
        // A pass no transaction holds that read the frame's staging (an
        // effect, a `[s(), latest(s)]` reader watched before the frame had
        // a transaction): judged by the frame's — it re-derives only if the
        // frame stays parked; landing here, what it read is the screen (one
        // run, D-22).
        r = t.T?.Re || (t.R & REACTIVE_STAGED_READ ? e : null);
        // A verdict reader a later read of the same pass routed into a verdict
        // lane answered for itself: it re-derives at the holder's landing, not
        // now (one run — #3322, #3540). Other verdict-lane work re-derives only
        // if the frame stays parked: a transaction landing at this seam makes
        // what it read the screen (one run). The frame is the transaction that
        // holds what it read (`t`, this flush's — not always the lane's parent:
        // a landing that reached a frame held elsewhere joined that one). A reader a
        // transaction holds that read none of its stagings (a verdict reader
        // that probed only a flight, held as the frame's pending observer or
        // its staged derivation) has nothing to re-derive: its re-run
        // re-enters that transaction and parks it again, every round — it
        // runs at the landing (#4349). Not one whose probe this seam's park
        // answered: a flight it asked about before anything joined is held
        // now, so its re-run routes into the holder's verdict lane and shows
        // the verdict now (F-6, D-23); run only at the landing, its
        // `isPending` read `false` for the whole hold (#4439). One that read a
        // staging, or such a flight, re-derives only if its frame stays
        // parked: landing at this seam, what it read is the screen (one run,
        // D-22). Not one born into that frame (a held pass's child, ruling A):
        // it has no screen, its reads are the transaction's either way — it
        // runs again at the landing, not now. Nor lane work re-based over this
        // frame's answer (#4536, merged since or not): its rows are that
        // answer's, so its other reads are too (core.ts `frameRead`), and the
        // lane holds while the frame does (`laneSeam`).
        // Re-derived on the committed world, it paired the new rows with the
        // old inputs, and the landing revealed that beside them (F-1, D-34 ¶3,
        // #4577). A lane linked to such a lane counts (one reveal unit): a
        // memo seated in one whose pass read through the other read the
        // frame's staging there, and, re-derived, it re-entered that lane,
        // read it again and parked again, every round (#4643).
                if (t.R & REACTIVE_DISPOSED || (r?.P ? isVerdict(r) && (t.l & CONFIG_VERDICT ? !(t.R & REACTIVE_PROBE_UNANSWERED) : !blocked(e)) || (r.nn || [ r ]).some(n => n.over && liveTx(n.over) == e) : r && !(t.R & REACTIVE_PROBE_UNANSWERED || (t.R & REACTIVE_STAGED_READ || readsHeld(t)) && blocked(r) && !bornInto(t, r)))) continue;
        // A leaked effect re-derives seated in its lane from its first read: it
        // carries no lane value (`laneStage`), so without this its pass starts
        // as mainline, joins the parked frame on its first held read and reads
        // the staging, then re-enters the lane at its guess read and leaks
        // again — every round (#4014). A computed is already seated by its
        // override, and a written guess's own pass stays the frame's.
                if (r?.P && t.i) t.R |= REACTIVE_LANE_DIRTY;
        // R5 for an async reader: its frame pass asked the frame's question
        // (F-6's lane gets its own pass now). That flight is the frame's
        // answer: the landing adopts it rather than asking it again (F-9: a
        // re-ask of the same question is no new question; `reruns`). A frame
        // landing at this seam has nothing to re-derive: the pass was the
        // landing's, its flight the screen's answer. Sync or async, the
        // reader's lane pass is still to come: the verdict lane it joins does
        // not reveal this seam (F-6: the lane's readers show its answer
        // together, D-23: atomic with the value) — revealed now, it showed a
        // verdict beside the frame pass's value, before the pass that answers
        // for the lane, and a reader of the re-derived memo read it outside
        // the shown lane and waited for the frame (#4393).
         else if (!r?.P && t.R & REACTIVE_PROBE_UNANSWERED) {
            // (verdict.ts `r5`: only the verdict engine leaves a probe unanswered.)
            if (r5Hook(t, e)) continue;
            if (e?.Pe && n.indexOf(e.Pe) === -1) n.push(e.Pe);
        }
        enqueueSub(t);
        if (r?.P && n.indexOf(r) === -1) n.push(r);
    }
    // A verdict lane born at this seam is judged with it: it exists only once
    // a pass routed into it — the park seam's zombie verdict readers re-run
    // as its work so that this seam reveals them (#3444).
    // A lane a guess re-asked at this seam is judged with that guess's passes,
    // next round: its reveal is whole (#4775).
        for (let e = lanes.length - 1; e >= 0; e--) if (e < judged && lanes[e].es !== clock || isVerdict(lanes[e])) laneSeam(lanes[e], n.indexOf(lanes[e]) !== -1);
    // A verdict reader that entered a lane this seam held: its run waits with
    // the lane, and the verdict beside it would freeze — it re-derives next
    // round from the frame's seat, a stale reader of the lane (#4284).
        for (let e; e = entered.pop(); ) if (e.T.Re?.un) {
        e.R &= ~REACTIVE_LANE_DIRTY;
        enqueueSub(e);
    }
    // (verdict.ts `r5`: the round's armed readers and re-asked passes ran.)
        if (r5Hook) r5Hook(null, null);
};

/** A15: lane work or a verdict reader was served `el`'s committed value
 * (core.ts `frameRead`), so its pass does not wait on `el`'s flight: what
 * `el`'s pending propagated onto it before the pass (its origins, `el`
 * included) is not this pass's — the landing re-derives it (`reruns`).
 * Kept, a pass that went pending on another source never woke when that
 * source settled (#4397). Only a reader something made pending has anything
 * to drop. What the drop retires is retired from the dependents that carried
 * it through `c` too: the source's own landing walk stops at `c`, which no
 * longer has it, and a render effect left marked would hold its transaction
 * on a flight that has landed (D-11, F-2, #4576; a nested action's refresh). */
/** Each lane pass served a flight's committed value: that flight, and the
 * pass (`_depGen`), so a later pass of the same node did not read it. Kept
 * per node: a pass that runs another node's pass inside its own (a nested
 * memo created or pulled mid-body) must still find its own record after the
 * inner one recorded (#4927). */ const served = new WeakMap;

const laneServed = (e, n, s) => {
    // Lane work read a staging of the flush after it was served the committed
    // value of a flight (core.ts `stagedRead`): it derives from the write that
    // re-asked the flight, so it waits on it — never the new value beside the
    // old answer (F-1, F-2; S-57).
    // Asked with `flight` (core.ts `recompute`): whether it was so served.
    if (n === null) {
        const n = served.get(e);
        if (n && n[1] === e.de && n[0].m & STATUS_PENDING) {
            if (s) return true;
            throw n[0].T.U;
        }
        return;
    }
    // Lane work reading a flight that is not the lane's own (core.ts `read`,
    // A31): served its committed value — the lane breaks out of its parent's
    // hold — unless the pass read a staging of the flush, which makes it that
    // write's work: it waits on the flight as mainline does (F-2; S-57).
    // Served, the pass still derives from the flight: its stale reader (A15,
    // #3494), so the frame holds on it and re-derives it at the landing.
        if (s) {
        if (e.R & REACTIVE_STAGED_READ) return false;
        const s = n.T?.Re || flushTransaction;
        if (s) staleReader(e, resolveTx(s));
        served.set(e, [ n, e.de ]);
        return true;
    }
    const t = n.T?.se;
    if (t && e.T?.se) for (const n of t) if (removePendingSource(e, n)) settlePendingSource(e, n);
};

/** This flush's verdict readers that read a flight ahead of the frame
 * (#4661, the seam). */ const ahead = [];

/** Render effects that read a verdict and entered a lane before its seam (#4284). */ const entered = [];

installLaneHooks(laneRead, laneStage, laneWrite, laneSeams, landing, lanesBlocked, laneServed);

GlobalQueue.J = verdictLane;

export { ahead, armGuessed, dissolveLane, enterLane, guessFlights, guessScreen, guessWriteHook, heldMountUnder, inFlight, installGuessLayer, isVerdict, keptGuess, landBeneath, laneRead, laneStage, lanes, latestOf, markJudged, marked, newLane, readGuesses, rerun, revertLandingHook, screenOf, setLaneValue, takeHook, verdictLane, waiting };