import { CONFIG_GUESS, NOT_PENDING, STATUS_UNINITIALIZED, CONFIG_AUTHORITATIVE, CONFIG_OVERRIDE } from "./constants.js";

import { NotReadyError } from "./error.js";

import { enqueueSub } from "./heap.js";

import "./scheduler.js";

// Truth reads (`until`, `refresh`; CONFIG_AUTHORITATIVE) over lane nodes,
// and the two views of a lane node every reader is served from. Side-effect
// free and apart from lanes.ts so a truth reader alone does not retain the
// lane engine: its reads reach these only through `laneRead`, which exists
// once something installs lanes (`createOptimistic`, verdicts).
/** A lane's record of a node beside the node's two slots: its guess until
 * the lane reveals it (its staging is a truth's), and after the reveal the
 * value the screen showed before (a guess reverts to it at the lane's end;
 * once shown (CONFIG_SHOWN). Dropped when the node leaves its lane. */
const covers = /* @__PURE__ */ new WeakMap;

/** A lane node's latest value. A guess: the guess — the lane's record of it
 * until the reveal (`under`), the screen's value after. A derivation: its
 * staging under the lane (a lane pass's answer, until the reveal), else
 * what the screen shows — NOT_PENDING for a node with neither (born in the
 * lane, uninitialized: its reads throw NotReady, D-22; #4363). */ function latestOf(r) {
    if (r.l & CONFIG_GUESS) return r.T.Ue ? r.le : covers.get(r);
    return r.ue !== NOT_PENDING ? r.ue : r.m & STATUS_UNINITIALIZED ? NOT_PENDING : r.le;
}

/** The value a guess covered: the committed value until the lane reveals
 * the guess, the lane's record of it after (`under`) */ function covered(r) {
    return r.T.Ue ? covers.get(r) : r.le;
}

/** The truth beneath a guess: a landing staged there — the parent's (lanes.ts
 * `landBeneath`), or the same tick's — else the value it covered. */ function underOf(r) {
    return r.ue !== NOT_PENDING ? r.ue : covered(r);
}

/** Truth readers (`until`, `refresh`; CONFIG_AUTHORITATIVE) — pay-for-use:
 * the first one installs these on GlobalQueue (signals.ts `watch`).
 *
 * A truth reader's read of a lane node: the lane's value when it is the
 * truth (#3943), else the truth beneath a guess. */ function truthRead(r) {
    if (truth(r)) return latestOf(r);
    if (!(r.l & CONFIG_GUESS)) throw new NotReadyError(null);
    return underOf(r);
}

/** A guess confirmed: its lane's truth readers re-read — of the lane's
 * derivations too, which may be the truth now (#3943). */ function truthConfirmed(r) {
    for (let e = 0; e < r.ln.length; e++) for (let t = r.ln[e].o; t !== null; t = t.t) if (t.u.l & CONFIG_AUTHORITATIVE) enqueueSub(t.u);
}

/** A lane node's value is the truth when every guess beneath it is
 * confirmed — its source landed a value equal to the guess beneath it — and
 * nothing beneath it is held in a transaction: same inputs, same value. */ function truth(r) {
    if (r.l & CONFIG_GUESS) return r.ue !== NOT_PENDING && r.ye && r.ye(latestOf(r), r.ue);
    if (r.l & CONFIG_OVERRIDE) {
        for (let e = r.C; e; e = e.G) if (!truth(e.S)) return;
        return 1;
    }
    return r.ue === NOT_PENDING ? 1 : undefined;
}

export { covered, covers, latestOf, truthConfirmed, truthRead, underOf };