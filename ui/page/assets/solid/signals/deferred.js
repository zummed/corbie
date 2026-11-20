import { CONFIG_REDERIVE, STATUS_UNINITIALIZED, CONFIG_LAGS, CONFIG_SLOT_NODE, REACTIVE_RECOMPUTING_DEPS, REACTIVE_DISPOSED, REACTIVE_REASK, CONFIG_HELD, NOT_PENDING } from "./core/constants.js";

import { isThenable, handleAsync } from "./core/async.js";

import { NotReadyError } from "./core/error.js";

import "./core/verdict.js";

import { computed } from "./core/core.js";

import { cleanup, getOwner } from "./core/owner.js";

import { GlobalQueue, holdNode, txOf, schedule } from "./core/scheduler.js";

import "./core/effect.js";

import { mark, repoll, unmark } from "./affects-marks.js";

import { accessor } from "./signals.js";

// `createDeferred` — an async memo that may lag, but never leads (D-40,
// docs/create-deferred.md).
// Once the node has a committed value, its own non-final work is invisible
// to the graph: its readers keep the committed value and hold nothing, its
// landings are changes of their own, and only the verdict (`isPending`) and
// the truth readers (`until`, `refresh`'s waiter) see the flight.
// Built from parts the engine already has, so core pays nothing for it:
// - **Read side: the loading window** (`_loading`, async.ts). Every pass of
//   a node with a committed value opens the window before it asks, so a
//   flight serves the committed value instead of throwing, and an unready
//   source parks the node instead of pending it. `CONFIG_REDERIVE` makes a
//   source going pending a question for the node's pass (which parks) rather
//   than a status pushed through it to its readers.
// - **Verdict: a mark** (affects-marks.ts) whose lifetime is the flight —
//   started when a pass asks a new question (a `refresh()` re-ask is quiet, F-9),
//   dropped at the landing, the rejection, a sync answer or the node's death.
// - **Never leads:** a flight that lands while an input it read is held
//   derived from that hold's world, so it lands into the hold (`holdNode`;
//   `setSignal` stages it there). An input that changed after the pass
//   would have superseded the flight, so a held input at the landing is one
//   the pass read held.
// - **Truth readers** wait the flight out (`GlobalQueue._lagging`, signals.ts
//   `watch`): `await refresh(d)` delivers the landing, `until` sees only
//   settled values.
/** Deferred nodes with a flight up. */
const lagging = new Set;

/** The deferred node with a flight up that `c`'s value derives from, if
 * any: `c` itself, or one reached through its current dependencies. */ function lagOf(e, r) {
    if (lagging.has(e)) return e;
    if (r.has(e)) return;
    r.add(e);
    if (e.l & CONFIG_SLOT_NODE) return lagOf(e.O?.rr?.node ?? e, r);
    const t = e.R & REACTIVE_RECOMPUTING_DEPS ? e._ : undefined;
    if (t !== null) for (let o = e.C ?? null; o !== null; o = o.G) {
        const e = lagOf(o.S, r);
        if (e) return e;
        if (o === t) break;
    }
}

GlobalQueue.er = e => lagging.size !== 0 ? lagOf(e, new Set) : undefined;

/**
 * Creates an async memo that may lag, but never leads. Before its first
 * answer it is an ordinary async memo: readers suspend to the nearest
 * `<Loading>`. After it, a refetch holds nothing: readers keep the last
 * committed value until the new one lands, and the rest of the screen moves
 * on without it. `isPending` reports the refetch, on the node and on what
 * derives from it. Use it for independent widgets (dashboards, feeds,
 * search results) where the previous answer with a "refreshing" indicator is
 * better than waiting with everything else.
 *
 * - It never shows a value ahead of a held change: a refetch that read a
 *   held value lands with that hold.
 * - A rejected refetch errors like any memo; the stale value never masks it.
 * - `refresh(d)` is a quiet re-ask, and `await refresh(d)` and `until`
 *   wait for the landing rather than the stale value.
 * - Combine with `loadingValue` for a node that never suspends at all.
 *
 * @example
 * ```tsx
 * const rows = createDeferred(() => fetchRows(store.period));
 *
 * <section class={{ stale: isPending(() => rows()) }}>
 *   <For each={rows()}>{row => <Row row={row} />}</For>
 * </section>
 * ```
 */ function createDeferred(e, r) {
    let t;
    /** The flight in force: a promise, a stream, or the error an unready
     * source parked the node with. */    let o = null;
    let n = false;
    let l = false;
    /** The flight `r` is over. An async answer (`held`) arriving while an
     * input is held lands into that hold first. */    const land = (e, r) => {
        if (e === null || o !== e) return;
        o = null;
        lagging.delete(t);
        if (r) for (let e = t.C; e !== null; e = e.G) if (e.S.l & CONFIG_HELD) {
            holdNode(t, txOf(e.S));
            break;
        }
        if (n) {
            n = false;
            unmark(t);
            schedule();
        }
    };
    const ask = e => {
        o = e;
        lagging.add(t);
        if (!n && !(t.R & REACTIVE_REASK)) {
            n = true;
            mark(t);
            repoll(t);
        }
    };
    /** Registers the flight and serves the pass's value: the committed one,
     * or over a held landing the landing itself, which a lagging question
     * does not replace (D-40) — restaged as the committed value, the hold's
     * readers would re-derive against the old input. */    const serve = e => {
        const r = handleAsync(t, e);
        return t.l & CONFIG_HELD && t.ue !== NOT_PENDING ? t.ue : r;
    };
    const end = () => t.R & REACTIVE_DISPOSED && land(o);
    return accessor(computed(r => {
        t = getOwner();
        t.l |= CONFIG_REDERIVE;
        cleanup(end);
        // Answered: the node has a real committed answer — not a first load,
        // not a `loadingValue` window still open (both are loading, F-9).
        // Sticky, and read before this wrapper ever opens the window itself.
                const n = l ||= !(t.m & STATUS_UNINITIALIZED) && !t.Ie;
        if (n) t.Ie = true, t.l |= CONFIG_LAGS;
        try {
            const t = e(r);
            if (n && typeof t === "object" && t !== null) {
                if (isThenable(t)) {
                    ask(t);
                    const done = () => land(t, true);
                    t.then(done, done);
                    return serve(t);
                }
                const e = t[Symbol.asyncIterator];
                if (e) {
                    // A stream's flight ends at its first yield (or its end).
                    const r = e.call(t);
                    let o = true;
                    const done = () => land(n, true);
                    const n = {
                        // Registered ahead of the engine's own reaction to the same
                        // promise, as for a plain flight: verdict and value in one flush.
                        next: e => {
                            const t = r.next(e);
                            if (o) {
                                o = false;
                                if (isThenable(t)) t.then(done, done); else done();
                            }
                            return t;
                        },
                        return: e => r.return?.(e) ?? Promise.resolve({
                            done: true,
                            value: e
                        })
                    };
                    ask(n);
                    return serve({
                        [Symbol.asyncIterator]: () => n
                    });
                }
            }
            land(o);
            return t;
        } catch (e) {
            // The wrapping form: an unready source parks the node in the window,
            // and the wait is its flight until the source's settle re-runs it.
            if (n && e instanceof NotReadyError) ask(e); else land(o);
            throw e;
        }
    }, r));
}

export { createDeferred };