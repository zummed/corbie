import { isThenable, onSupersede } from "./core/async.js";

import { CONFIG_CHILDREN_FORBIDDEN } from "./core/constants.js";

import { cleanup, getOwner } from "./core/owner.js";

// `from` — a push source (callback subscription, Observable, AsyncIterable)
// as an AsyncIterable a computation can return or `for await` over:
//   const value = createMemo(() => from(source));
// A subscription keeps the latest value: values that arrive between pulls
// coalesce. An AsyncIterable streams through unchanged. Either way the source
// opens when the result is iterated and closes on return(), on supersede (the
// owning computation re-runs) and on owner disposal, whichever comes first.
function toSubscribe(e) {
    if (typeof e === "function") return e;
    // TC39 / RxJS interop: `Symbol.observable` (or "@@observable") returns the observable.
    // Read per call, not at module scope, so the module stays side-effect free.
        const n = Symbol.observable;
    const r = (n && e?.[n]) ?? e?.["@@observable"];
    const t = typeof r === "function" ? r.call(e) : e;
    if (t && typeof t.subscribe === "function") return (e, n, r) => t.subscribe({
        next: e,
        error: n,
        complete: r
    });
    return null;
}

const DONE = {
    value: undefined,
    done: true
};

/**
 * Ties an iterator's `finish` to the computation iterating it. Solid reaches
 * an iterator's return() only when the compute returns the iterable directly.
 * Inside an async generator the generator's own return() is queued behind its
 * pending `await`, and owner cleanups wait for the replacing pass to commit,
 * so close on supersede, with owner disposal as the backstop. No owner (an
 * async compute iterating it after its first `await`, or plain iteration):
 * no registration and no diagnostic, so only return() closes it. Scopes that
 * forbid cleanups rely on supersede and return().
 */ function bindToOwner(e) {
    const n = getOwner();
    if (!n) return;
    onSupersede(e);
    if (!(n.l & CONFIG_CHILDREN_FORBIDDEN)) cleanup(e);
}

/**
 * An AsyncIterable as a stream bound to the iterating computation: every
 * value passes through (no coalescing). `finish` calls the inner return()
 * once and settles pending pulls as done, so a `for await` over it exits as
 * soon as the computation re-runs — even when the inner iterator is an async
 * generator whose own return() the language queues behind its pending
 * `await`.
 */ function wrapIterable(e) {
    return {
        [Symbol.asyncIterator]() {
            const n = e[Symbol.asyncIterator]();
            let r = false;
            const t = new Set;
            const finish = () => {
                if (r) return DONE;
                r = true;
                for (const e of t) e(DONE);
                t.clear();
                try {
                    const e = n.return?.();
                    if (isThenable(e)) e.then(undefined, () => {});
                } catch {}
                return DONE;
            };
            bindToOwner(finish);
            return {
                next(...e) {
                    if (r) return Promise.resolve(DONE);
                    const o = n.next(...e);
                    // A bare result (the producer's buffered fast path) passes through.
                                        if (!isThenable(o)) return o;
                    return new Promise((e, n) => {
                        t.add(e);
                        o.then(n => t.delete(e) && e(n), r => t.delete(e) && n(r));
                    });
                },
                return() {
                    return Promise.resolve(finish());
                }
            };
        }
    };
}

const toCleanup = e => typeof e === "function" ? e : e && typeof e.unsubscribe === "function" ? () => e.unsubscribe() : () => {};

/**
 * Converts a callback subscription, an Observable or an AsyncIterable into an
 * AsyncIterable for a Solid computation. A subscription keeps only the latest
 * value, and a value the source emits during `subscribe` is served without a
 * loading frame; an AsyncIterable streams every value through.
 *
 * The source closes on supersede — the owning computation re-running — and
 * on owner disposal. Inside an async generator memo that holds for a
 * `for await` that starts before the first `await`; one that starts after it
 * has no owner to bind to, so its source closes only when the loop ends.
 * Prefer returning `from(...)` from its own memo, with awaited inputs read
 * from upstream memos and transforms done in a downstream memo.
 *
 * Iterate the result with `for await` or return it from a computation: its
 * `next()` returns an already-buffered value as a bare result, not a promise.
 *
 * @example
 * ```ts
 * const position = createMemo(() =>
 *   from<Position>((next, error) => {
 *     const id = navigator.geolocation.watchPosition(next, error);
 *     return () => navigator.geolocation.clearWatch(id);
 *   })
 * );
 * const ticks = createMemo(() => from(interval$(rate())));
 * ```
 */ function from(e) {
    const n = toSubscribe(e);
    if (!n) {
        if (e && typeof e[Symbol.asyncIterator] === "function") return wrapIterable(e);
        throw new TypeError("from(): expected a subscribe function, an Observable or an AsyncIterable");
    }
    return {
        [Symbol.asyncIterator]() {
            let e;
            let r = false;
            let t = false;
            let o = false;
            let u;
            let f = null;
            let l = null;
            let s = false;
            const close = () => {
                if (s) return;
                s = true;
                t = true;
                const e = l;
                l = null;
                // A throwing unsubscribe must not escape: at supersede this runs
                // inside flush(), ahead of the replacing pass (the returned-iterator
                // path swallows it the same way).
                                try {
                    e?.();
                } catch {}
            };
            const settle = () => {
                if (!f) return;
                const n = f;
                if (r) {
                    f = null;
                    r = false;
                    n.resolve({
                        value: e,
                        done: false
                    });
                } else if (o) {
                    f = null;
                    o = false;
                    n.reject(u);
                } else if (t) {
                    f = null;
                    n.resolve({
                        value: undefined,
                        done: true
                    });
                }
            };
            const i = n(n => {
                if (t) return;
                e = n;
                r = true;
                settle();
            }, e => {
                if (t) return;
                o = true;
                u = e;
                t = true;
                settle();
                close();
            }, () => {
                if (t) return;
                t = true;
                settle();
                close();
            });
            l = toCleanup(i);
            // Completed or errored during subscribe: release what it just returned.
                        if (s) {
                const e = l;
                l = null;
                try {
                    e();
                } catch {}
            }
            // Idempotent: return(), supersede and disposal may all reach it.
                        const finish = () => {
                close();
                r = false;
                if (f) {
                    const e = f;
                    f = null;
                    e.resolve(DONE);
                }
                return DONE;
            };
            bindToOwner(finish);
            return {
                next() {
                    // A buffered value is returned as a bare result, not a promise: the
                    // iterator consumer lands it in the same pass, so a source that
                    // emits during subscribe (BehaviorSubject-style) shows its value
                    // with no loading frame. `for await` accepts either form.
                    if (r) {
                        r = false;
                        return {
                            value: e,
                            done: false
                        };
                    }
                    if (o) {
                        o = false;
                        return Promise.reject(u);
                    }
                    if (t) return Promise.resolve({
                        value: undefined,
                        done: true
                    });
                    return new Promise((e, n) => {
                        f = {
                            resolve: e,
                            reject: n
                        };
                    });
                },
                return() {
                    return Promise.resolve(finish());
                }
            };
        }
    };
}

export { from };