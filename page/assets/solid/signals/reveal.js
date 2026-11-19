import { runWithOwner, untrack, computed } from "./core/core.js";

import { createOwner, cleanup } from "./core/owner.js";

import { REVEAL, ready, redraw, setRevealHooks } from "./boundaries.js";

/**
 * Reveal order — `createRevealOrder` (`<Reveal>`): the order in which a group
 * of Loading boundaries (its direct slots) may show their content.
 *
 * A controller owns each slot until the slot has shown content, and forces
 * on the slots it owns the state the order dictates: gated (the fallback,
 * whatever the content's state) and, in a collapsed sequential tail, nothing
 * at all. A nested reveal order is a composite slot of its parent — gated,
 * it gates all of its own; released, it runs its own order.
 *
 * - `sequential`: slots reveal front to back. The first unready slot is the
 *   frontier — its fallback shows; the slots behind it are gated (collapsed
 *   when `collapsed`); the ones before it have revealed.
 * - `together`: every owned slot is gated until all are minimally ready;
 *   then all are released at once.
 * - `natural`: each slot reveals on its own as it becomes ready.
 *
 * A slot graduates when it shows content: a later re-arm of it gates nobody
 * else, and slots appended after graduation form a fresh frontier. The
 * boundary tells the controller when its readiness changed (boundaries.ts:
 * caught, settled, shown, disposed); `order`/`collapsed` are tracked by a
 * computed of the group's own.
 */ const FALSE_ACCESSOR = () => false;

const SEQUENTIAL_ACCESSOR = () => "sequential";

const isController = t => t instanceof RevealController;

const slotReady = t => isController(t) ? t.St() : ready(t);

const slotMinReady = t => isController(t) ? t.Rt() : ready(t);

/** Force a slot's state. A boundary re-derives its output when it changes;
 * a nested controller passes the state on to its own slots. Released
 * (`!gated`), a nested controller leaves its parent's ownership and runs its
 * own order; a boundary leaves when it has shown content (boundaries.ts). */ function setSlotState(t, e, s, r) {
    if (isController(t)) {
        t.$ = s;
        t.ee = r;
        t._t();
        // Released and ready — every slot of its own has shown — it graduates,
        // as a boundary does when it shows content. Released at the frontier
        // but not ready, it stays the parent's frontier.
                if (!s && t.Ct && t._parent === e) t._parent = null;
        return;
    }
    if (t.$ !== s || t.ee !== r) {
        t.$ = s;
        t.ee = r;
        redraw(t);
    }
}

class RevealController {
    yt;
    At;
    kt=[];
    /** The controller gating this one, until it releases this one. */
    _parent=null;
    /** The state a parent forces on this whole group. */
    $=false;
    ee=false;
    Ct=true;
    vt=true;
    gt=false;
    constructor(t, e) {
        this.yt = t;
        this.At = e;
    }
    /** A slot that has left this controller's ownership: a boundary that has
     * shown content, a nested group that is ready. Shown is shown — it counts
     * as ready whatever happens to it later (a re-arm gates nobody else). */    Lt(t) {
        return (isController(t) ? t._parent : t.v) !== this;
    }
    /** The slots this controller still owns, in registration order — the
     * ones the order's state is forced on. */    Mt(t) {
        for (let e = 0; e < this.kt.length; e++) {
            const s = this.kt[e];
            if (!this.Lt(s) && t(s) === false) return false;
        }
        return true;
    }
    /** Readiness is judged over every slot, done ones as ready. */    St() {
        for (let t = 0; t < this.kt.length; t++) {
            const e = this.kt[t];
            if (!this.Lt(e) && !slotReady(e)) return false;
        }
        return true;
    }
    /** Something visible to show under its own order — what an enclosing
     * `together` waits for: `together` — every slot minimally ready;
     * `sequential` — the first slot; `natural` — any. */    Rt() {
        const t = untrack(this.yt);
        const e = this.kt;
        if (e.length === 0) return true;
        if (t === "sequential") return this.Lt(e[0]) || slotMinReady(e[0]);
        for (let s = 0; s < e.length; s++) {
            const r = this.Lt(e[s]) || slotMinReady(e[s]);
            if (t === "natural" ? r : !r) return t === "natural";
        }
        return t !== "natural";
    }
    te(t) {
        if (this.kt.includes(t)) return;
        this.kt.push(t);
        // Gated from the start (collapsed in a collapsed sequential tail); the
        // evaluation releases what the order allows.
                const e = untrack(this.yt) === "sequential" && !!untrack(this.At);
        if (isController(t)) {
            t._parent = this;
            t.$ = true;
            t.ee = e;
            t._t();
        } else {
            t.v = this;
            t.$ = true;
            t.ee = e;
        }
        untrack(() => this._t());
    }
    oe(t) {
        const e = this.kt.indexOf(t);
        if (e >= 0) this.kt.splice(e, 1);
        untrack(() => this._t());
    }
    /** Apply the order to the owned slots — or, gated by a parent, its state
     * to all of them. */    _t() {
        if (this.gt) return;
        this.gt = true;
        const t = this.Ct;
        const e = this.vt;
        try {
            const t = untrack(this.yt);
            const e = t === "sequential" && !!untrack(this.At);
            if (this.$) {
                this.Mt(t => setSlotState(t, this, true, this.ee));
            } else if (t === "natural") {
                this.Mt(t => {
                    if (isController(t)) setSlotState(t, this, false, false); else setSlotState(t, this, !ready(t), false);
                });
            } else if (t === "together") {
                const t = this.Mt(slotMinReady);
                this.Mt(e => setSlotState(e, this, !t, false));
            } else {
                let t = false;
                this.Mt(s => {
                    if (t) return setSlotState(s, this, true, e);
                    if (slotReady(s)) return setSlotState(s, this, false, false);
                    t = true;
                    // The frontier: its fallback shows; a nested controller at the
                    // frontier runs its own order.
                                        setSlotState(s, this, isController(s) ? false : true, false);
                });
            }
        } finally {
            this.Ct = this.St();
            this.vt = this.Rt();
            this.gt = false;
        }
        if (this._parent !== null && (t !== this.Ct || e !== this.vt)) this._parent._t();
    }
}

/**
 * Lower-level primitive behind `<Reveal>`: coordinates the order in which the
 * Loading boundaries created directly inside `fn` reveal their content.
 *
 * @param fn the subtree; the Loading boundaries it creates directly are the
 *   group's slots, a nested `createRevealOrder` a composite slot
 * @param options `order` — `"sequential"` (default: front to back, the
 *   first unready slot showing its fallback), `"together"` (all at once),
 *   `"natural"` (each on its own); `collapsed` — in a sequential group, the
 *   slots behind the frontier render nothing instead of their fallbacks
 */ function createRevealOrder(t, e) {
    const s = createOwner();
    const r = s.A[REVEAL];
    const i = new RevealController(e?.order || SEQUENTIAL_ACCESSOR, e?.collapsed || FALSE_ACCESSOR);
    s.A = {
        ...s.A,
        [REVEAL]: i
    };
    return runWithOwner(s, () => {
        const e = t();
        const s = computed(() => {
            i.yt();
            i.At();
            i._t();
        });
        if (false) ;
        if (r) {
            r.te(i);
            cleanup(() => r.oe(i));
        }
        return e;
    });
}

// Installed at module evaluation — present exactly when something imports
// `createRevealOrder`; a Loading boundary without one pays a null check.
setRevealHooks({
    te: (t, e) => t.te(e),
    oe: (t, e) => t.oe(e),
    V: t => t._t()
});

export { createRevealOrder };