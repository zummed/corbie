import { CONFIG_GUESS, NOT_PENDING, CONFIG_OVERRIDE, CONFIG_SLOT_NODE, CONFIG_INPUTS_PUBLISHED, CONFIG_HELD } from "./constants.js";

import { ext } from "./core.js";

import { latestOf, covered, covers, underOf } from "./truth.js";

import { schedule, question, nextQuestion, armGuessing, installGuessHooks, txOf, list, resolveTx, GlobalQueue, commitPendingNode, ownFlights, merge, holdNode, insertSubs, clock, newTransaction, laneDirty } from "./scheduler.js";

import { armGuessed, installGuessLayer, rerun, setLaneValue, lanes, marked, landBeneath, guessFlights, markJudged, readGuesses, dissolveLane, newLane, inFlight } from "./lanes.js";

// The guess layer: written optimistic values (F-11, D-33, D-34). lanes.ts
// holds what every lane needs; this module holds what only a guess reaches.
// Installed through `installGuessLayer` and `installGuessHooks`, and only
// this module sets CONFIG_GUESS, so a bundle without optimism drops it all.
/** Guesses written since the last seam, as `[node, value, question, …]`.
 * Applied at the seam: only then is it known whether there is a hold to
 * guess over (D-33). */
const pendingGuesses = [];

/** The lanes this seam's guesses read from (D-33). */ const readFrom = [];

/** What an optimistic updater composes on (F-10: the writer sees its own
 * unflushed writes). */ function guessedValueOf(e) {
    let s = pendingGuessOf(e);
    return s !== NOT_PENDING || e.l & CONFIG_OVERRIDE && (s = latestOf(e)) !== NOT_PENDING ? s : e.le;
}

function pendingGuessOf(e) {
    for (let s = pendingGuesses.length - 3; s >= 0; s -= 3) if (pendingGuesses[s] === e) return pendingGuesses[s + 1];
    return NOT_PENDING;
}

/** Guesses another action's lane took over, as `[node, guess, lane,
 * question, taker, …]`, kept to give back if the taker reverts (#4336). */ const kept = [];

/** `d`: taken without being written (a store target's other slots). */ function take(e, s, n) {
    const t = e.T;
    txOf(e);
    const r = latestOf(e);
    kept.push(e, r, t.Re, t.ce, s);
    rerun(e, s);
    if (t.Ue) e.le = covered(e);
    covers.delete(e);
    t.Ue = false;
    e.l &= n ? -10241 : ~CONFIG_GUESS;
    if (n) t.Re = null; else {
        list(e, s);
        if (e.ue === NOT_PENDING) setLaneValue(e, s, r);
    }
}

function optimisticWrite(e, s) {
    const n = guessedValueOf(e);
    if (typeof s === "function") s = s(n);
    // Equal is no guess, unless the node carries one: re-asking renews its question (#3347).
        if (e.ye && e.ye(n, s) && !(e.l & CONFIG_GUESS)) return s;
    pendingGuesses.push(e, s, question || nextQuestion());
    for (let e = 0; e < readGuesses.length; e += 2) if (readGuesses[e + 1] === question) readFrom.push(readGuesses[e]);
    armGuessing();
    armGuessed();
    schedule();
    return s;
}

/** Each guess becomes its node's lane value. With no hold to guess over
 * and no flight of its own, the guess is dropped (D-33). */ function applyGuesses(e) {
    let s = null;
    for (let n = 0; n < pendingGuesses.length; n += 3) {
        const t = pendingGuesses[n];
        const r = pendingGuesses[n + 1];
        let l = true;
        // A guess over a slot another frame holds entangles the frames (D-8, D-33).
                let u = null;
        if (t.l & CONFIG_OVERRIDE) {
            l = !t.ye || !t.ye(latestOf(t), r);
            if (!(t.l & CONFIG_GUESS)) {
                t.ue = NOT_PENDING;
                t.l |= CONFIG_GUESS;
            } else if (t.T.Ue) {
                // The screen keeps the shown guess until the lane's next reveal (F-1, S-30).
                screens.has(t) || screens.set(t, [ t.le, t.T.Re ]);
                txOf(t).es = clock;
            }
            setLaneValue(t, txOf(t), r);
            u = txOf(t)._parent;
        } else {
            let n;
            // Over a held node the guess composes on the held value, so it lives
            // until both holds end (F-11).
                        if (t.l & CONFIG_HELD) n = newLane(u = txOf(t)); else {
                if (e === null && inFlight(t)) e = newTransaction(false);
                if (e === null) continue;
                n = s ??= newLane(e);
            }
            // Even a guess equal to the truth beneath it re-derives its readers in
            // the lane: a lane derivation must not show the hold's other writes (F-1).
                        t.l |= CONFIG_OVERRIDE | CONFIG_GUESS;
            list(t, n);
            setLaneValue(t, n, r);
        }
        if (e !== null && u !== null) merge(e, u);
        ext(t).ce = pendingGuesses[n + 2];
        if (l) {
            // As lane work, so a blocked lane's re-guess does not republish the committed view (#3460).
            laneDirty(t, txOf(t));
        }
    }
    pendingGuesses.length = 0;
}

/** `markJudged` first: a lane born at this seam has no work to judge yet. */ const applyGuessesAt = e => {
    markJudged();
    if (pendingGuesses.length !== 0) {
        // A guess written after reading another hold's guess entangles the holds (D-33).
        if (e !== null) for (const s of readFrom) if (lanes.includes(s)) merge(e, s._parent);
        applyGuesses(e);
    }
    // Keep reads of live lanes: the reading action may guess after a yield.
        let s = 0;
    for (let e = 0; e < readGuesses.length; e += 2) if (lanes.includes(readGuesses[e])) readGuesses[s++] = readGuesses[e], 
    readGuesses[s++] = readGuesses[e + 1];
    readFrom.length = 0;
    readGuesses.length = s;
};

/** The correction is a revert (no truth landed beneath): the lane gives
 * back the guesses it took (#4336). */ let reverting = false;

/** A guess's truth arrived: a different one corrects, an equal one confirms
 * (F-11, D-34). The truth is held by the guess's hold, not by the tick that
 * carried it, so one landing answering two actions' guesses does not
 * entangle them (D-33). */ function supersede(e, s, n) {
    const t = txOf(e);
    // The parent may have merged since the lane opened (D-33).
        const r = resolveTx(t._parent ?? t);
    // A new server object that reconciles to the guessed row unchanged confirms (#3898).
        if (n && e.l & CONFIG_SLOT_NODE && GlobalQueue.sn?.(latestOf(e), s)) n = false;
    if (n) {
        dissolveLane(t, r, e);
        if (reverting) marked.add(t);
        // A shown guess stays on screen until the commit (D-34), so the
        // published-inputs mark no longer applies (#4795).
                if (t.Ue) marked.add(e), e.l &= ~CONFIG_INPUTS_PUBLISHED;
        e.l &= -10241;
        e.T.Ue = false;
        covers.delete(e);
        holdNode(e, r);
        e.ue = s;
        if (e.o !== null) insertSubs(e);
    } else {
        landBeneath(e, s);
        GlobalQueue.tn?.(t);
    }
}

/** Body-end correction (D-34): once the action body has returned and no
 * truth is in flight, the truth at hand judges each guess. Runs before the
 * park decision so the correction's flights are the parent's (#3409). True
 * when a lane was judged. */ function laneCorrections() {
    let e = false;
    const s = lanes.slice();
    for (let n = 0; n < s.length; n++) {
        const t = s[n];
        if (lanes.indexOf(t) === -1) continue;
        const r = resolveTx(t._parent);
        if (!r.nt || r.tt !== 0 || r.P || ownFlights(r) || guessFlights(t)) continue;
        e = true;
        // A correction dissolves the lane, so re-check membership each step.
                for (let e = 0; e < t.ln.length && lanes.indexOf(t) !== -1; e++) {
            const s = t.ln[e];
            if (s.l & CONFIG_GUESS && s.T.Re === t) {
                const e = underOf(s);
                reverting = s.ue === NOT_PENDING;
                supersede(s, e, !s.ye || !s.ye(latestOf(s), e));
                reverting = false;
            }
        }
    }
    return e;
}

/** A write landing on a guess. False when a store slot's guess is taken by
 * another action's lane instead (#4336). */ function guessWrite(e, s) {
    if (GlobalQueue.Nn?.(e)) return false;
    // Provenance (D-34): an older question's answer is held beneath, not judged.
        const n = stale(e, question);
    if (GlobalQueue.In?.(e, s, n) || n) landBeneath(e, s); else supersede(e, s, !e.ye || !e.ye(latestOf(e), s));
    schedule();
    return true;
}

/** A guess's own pass finished: whatever recomputes the node is its
 * source (D-34). An errored pass stages nothing; a stale answer is held
 * beneath (provenance, D-34). */ function laneOutcome(e, s, n) {
    if (n) return true;
    if (stale(e, answered(e))) {
        landBeneath(e, s);
        return true;
    }
    supersede(e, s, !e.ye || !e.ye(latestOf(e), s));
    return true;
}

/** Provenance (D-34): unstamped (0) answers are current. */ function stale(e, s) {
    return s !== 0 && s < e.T.ce;
}

/** The newest question among the sources that changed this round; 0 when none is stamped. */ function answered(e) {
    let s = 0;
    for (let n = e.C; n !== null; n = n.G) {
        const e = n.S;
        if (e.T !== null && e.T.ce > s && e.ie === clock) s = e.T.ce;
    }
    return s;
}

/** A revert landing gives back the guesses its lane took from an action
 * still out, in the landing's frame (F-1, #4336). */ function revertLanding(e) {
    for (let s = kept.length - 5; s >= 0; s -= 5) if (resolveTx(kept[s + 4]._parent) === e) {
        let [n, t, r, l, u] = kept.splice(s, 5);
        if (marked.has(u) && lanes.includes(r) && (!(n.l & CONFIG_SLOT_NODE) || (n = GlobalQueue.Nn(n, true)))) {
            const s = ext(n);
            if (s.Re === null || txOf(n) === e) {
                commitPendingNode(n);
                s.ce = l;
                n.l |= CONFIG_OVERRIDE | CONFIG_GUESS;
                list(n, r);
                setLaneValue(n, r, t);
                rerun(n, e);
            }
        }
    }
}

/** A re-asked shown guess: what the screen showed, and its lane, until the next reveal (#4775). */ const screens = /* @__PURE__ */ new WeakMap;

installGuessLayer(take, guessWrite, revertLanding, (e, s) => {
    const n = screens.get(e);
    if (s) screens.delete(e);
    return n && !s && e.l & CONFIG_GUESS && e.T.Ue && e.T.Re === n[1] ? n[0] : e.le;
});

installGuessHooks(laneOutcome, applyGuessesAt, () => lanes.length !== 0 && laneCorrections());

export { guessedValueOf, optimisticWrite, pendingGuessOf, supersede, take };