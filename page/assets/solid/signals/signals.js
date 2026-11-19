import { TimeoutError, NotReadyError, unwrapStatusError } from "./core/error.js";

import "./core/verdict.js";

import { CONFIG_CHILDREN_FORBIDDEN, EFFECT_USER, $REFRESH, CONFIG_AUTO_DISPOSE, CONFIG_AUTHORITATIVE, CONFIG_REDERIVE, REACTIVE_JOINED, CONFIG_FRESH_READ, CONFIG_GUESS, STATUS_PENDING, STATUS_ERROR } from "./core/constants.js";

import { computed, signal, runWithOwner, setMemo, setSignal, markRefresh, read, untrack } from "./core/core.js";

import { schedule, flush, globalQueue, flushTransaction, dirtyQueue, GlobalQueue, resolveTx, passTx, staleReader, joinFuture } from "./core/scheduler.js";

import { cleanup, createRoot, getOwner, dispose } from "./core/owner.js";

import { signalHook } from "./core/identities.js";

import { effect, trackedEffect } from "./core/effect.js";

import { optimisticWrite } from "./core/guesses.js";

import { truthRead, truthConfirmed } from "./core/truth.js";

/**
 * Low-level reactive-cleanup primitive. Registers a callback that runs when
 * the surrounding owner is disposed.
 *
 * **In 2.0 user code this is rare.** The two cases where you might reach for
 * it have better-shaped tools:
 *
 * - **Component lifecycle (mount/unmount, listeners, intervals):** use
 *   {@link onSettled} and **return** a cleanup function. Setup and teardown
 *   stay paired in one block. This replaces the 1.x `onMount` + `onCleanup`
 *   pairing.
 * - **Cleanup tied to an effect run:** `onCleanup` does not belong in
 *   `createEffect`'s apply phase. If a compute phase genuinely needs per-run
 *   teardown, that's usually a sign the work should be a memo/projection
 *   instead, or moved to `onSettled` if it's lifecycle-shaped.
 *
 * Where `onCleanup` is the right tool is **library / custom-primitive
 * internals** — coordinating disposal inside a `createRoot` body, or wiring
 * cleanup to a captured owner via `runWithOwner` from a custom factory.
 * Application code rarely needs to write any of those shapes directly.
 *
 * Must be called inside an owner. Calling outside an owner is a no-op (with a
 * dev-mode warning).
 *
 * Cannot be used inside `createTrackedEffect` or `onSettled` — return a
 * cleanup function from the callback body instead.
 *
 * Cleanups run in unwind order: an owner's children are disposed before its
 * own cleanups, and within one owner later registrations run before earlier
 * ones. In production a component body shares its enclosing owner, so
 * register cleanup before creating children when the order between them
 * matters.
 *
 * @example
 * ```ts
 * // Library shape: thread a resource's disposal into a *captured* owner
 * // from a factory that has no settle-phase setup of its own. `onSettled`
 * // would queue a callback we don't need; `onCleanup` is the leaner
 * // primitive when the only job is "register disposal on this owner".
 * function bindToOwner<T extends { dispose(): void }>(owner: Owner, resource: T): T {
 *   runWithOwner(owner, () => onCleanup(() => resource.dispose()));
 *   return resource;
 * }
 * ```
 */ function onCleanup(e) {
    return cleanup(e);
}

function accessor(e) {
    const t = read.bind(null, e);
    t[$REFRESH] = e;
    return t;
}

function createSignal(e, t) {
    if (typeof e === "function") {
        const r = computed(e, t);
        r.l &= ~CONFIG_AUTO_DISPOSE;
        return [ accessor(r), setMemo.bind(null, r) ];
    }
    const r = signal(e, t);
    if (signalHook !== null) signalHook(r);
    return [ accessor(r), setSignal.bind(null, r) ];
}

function createOptimistic(e, t) {
    if (typeof e === "function") {
        const r = computed(e, t);
        r.l &= ~CONFIG_AUTO_DISPOSE;
        return [ accessor(r), optimisticWrite.bind(null, r) ];
    }
    const r = signal(e, t);
    return [ accessor(r), optimisticWrite.bind(null, r) ];
}

function createMemo(e, t) {
    return accessor(computed(e, t));
}

/**
 * Creates a reactive effect with **separate compute and effect phases**.
 *
 * - `compute(prev)` runs reactively — *put all reactive reads here*. The
 *   returned value is passed to `effect` and is also the new "previous" value
 *   for the next run.
 * - `effect(next, prev?)` runs imperatively (untracked) after the queue
 *   flushes. *Put DOM writes / fetch / logging / subscriptions here.* It may
 *   return a cleanup function which runs before the next effect or on
 *   disposal.
 *
 * Reactive reads inside `effect` will *not* re-trigger this effect — that's
 * intentional. If you need a single-phase tracked effect, use
 * `createTrackedEffect` (with the tradeoffs noted there).
 *
 * Pass an `EffectBundle` (`{ effect, error }`) instead of a plain function to
 * intercept **compute-phase** errors — errors thrown by `compute` or arriving
 * from upstream reactive sources (including async rejections), which your own
 * code has no frame to `try/catch`. The `error` handler is the error arm of
 * the effect phase: it runs on the same schedule and in the same imperative,
 * writable scope as `effect` (setting error state via signals is fine), and
 * only for *settled* errors — a transient error that recovers before the
 * effect phase runs `effect` with the recovered value instead, and a held
 * transition defers it exactly as it defers `effect`. Without an `error`
 * handler a compute-phase error is logged and the effect simply skips that
 * run — a non-render effect's reactivity failing does not crash the app.
 * Rethrowing from `error` escalates it to the nearest error boundary
 * (halting the system if none exists).
 *
 * The **effect phase is different**: it is your own imperative code, so handle
 * failures with `try/catch` where they occur. An uncaught effect-phase throw
 * is treated as an unhandled application error — caught by the nearest
 * `createErrorBoundary`/`<Errored>`, and permanently halting the reactive
 * system if there is none. It is *not* routed to the bundle's `error` handler.
 *
 * ```typescript
 * createEffect<T>(compute, effectFn | { effect, error }, options?: EffectOptions);
 * ```
 * @param compute a function that receives its previous value and returns a new value used to react on a computation
 * @param effectFn a function that receives the new value and is used to perform side effects (return a cleanup function), or an `EffectBundle` with `effect` and `error` handlers
 * @param options `EffectOptions` -- name, defer, schedule, transparent
 *
 * @example
 * ```ts
 * const [count, setCount] = createSignal(0);
 *
 * createEffect(
 *   () => count(),                  // compute: tracks `count`
 *   value => console.log(value)     // effect: side effect
 * );
 *
 * setCount(1); // logs 1 after the next flush
 * ```
 *
 * @example
 * ```ts
 * createEffect(
 *   () => userId(),
 *   id => {
 *     const ctrl = new AbortController();
 *     fetch(`/users/${id}`, { signal: ctrl.signal });
 *     return () => ctrl.abort(); // cleanup before next run / disposal
 *   }
 * );
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/reactivity/create-effect
 */ function createEffect(e, t, r) {
    effect(e, t.effect || t, t.error, {
        user: true,
        ...r
    });
}

/**
 * Creates a reactive computation that runs during the render phase as DOM elements
 * are created and updated but not necessarily connected.
 *
 * Same compute / effect split as `createEffect`, but scheduled inside the render
 * queue rather than after it. Reach for this only when authoring renderer
 * plumbing (custom DOM bindings, JSX-generated `insert()` / `spread()` calls).
 * App code should use `createEffect`.
 *
 * ```typescript
 * createRenderEffect<T>(compute, effectFn, options?: EffectOptions);
 * ```
 * @param compute a function that receives its previous value and returns a new value used to react on a computation
 * @param effectFn a function that receives the new value and is used to perform side effects
 * @param options `EffectOptions` -- name, defer, schedule, transparent
 *
 * @example
 * ```ts
 * // Custom directive: bind an element's textContent to a reactive source.
 * function bindText(el: HTMLElement, source: () => string) {
 *   createRenderEffect(
 *     () => source(),
 *     value => { el.textContent = value; }
 *   );
 * }
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/advanced/specialized-reactivity/create-render-effect
 */ function createRenderEffect(e, t, r) {
    effect(e, t, undefined, r);
}

/**
 * Creates a tracked reactive effect where dependency tracking and side effects happen
 * in the same scope.
 *
 * @deprecated Do not use in new code. For a side effect that follows reactive
 * state, use `createEffect(compute, effect)` — it separates tracking from the
 * side effect, knows its dependencies before it runs, and participates in
 * async and transitions. For one-time DOM work after render (measuring,
 * attaching third-party widgets to a ref), use `onSettled`. Tracking from
 * inside the effect phase — the only thing this primitive adds — is retained
 * solely to ease 1.x migration: it runs beside user-effect callbacks after
 * values commit, never holds a transition, and cannot observe a write staged
 * earlier in the same flush by a signal it has not read yet.
 *
 * WARNING: Because tracking and effects happen in the same scope, this primitive
 * may run multiple times for a single change or show tearing (reading inconsistent
 * state). Use only when dynamic subscription patterns require same-scope tracking.
 *
 * The callback runs during the flush itself: writes made inside it are queued
 * into the same flush's continuation and are never visible to the callback's
 * own reads (reads return settled values, as in every effect-phase scope), and
 * `flush()` cannot be called from inside it (dev throws; production is a
 * no-op) — defer with `queueMicrotask(() => flush())` if needed.
 *
 * ```typescript
 * createTrackedEffect(compute, options?: { name?: string });
 * ```
 * @param compute a function that contains reactive reads to track and returns an optional cleanup function to run on disposal or before next execution
 * @param options -- name
 *
 * @example
 * ```ts
 * createTrackedEffect(() => {
 *   const target = focusedNode();
 *   if (!target) return;
 *
 *   const handler = () => log(target.value());
 *   target.on("change", handler);
 *
 *   return () => target.off("change", handler);
 * });
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/advanced/specialized-reactivity/create-tracked-effect
 */ function createTrackedEffect(e, t) {
    trackedEffect(e, t);
}

/**
 * Creates a reactive computation that runs after the render phase with flexible tracking.
 *
 * ```typescript
 * const track = createReaction(effectFn, options?: EffectOptions);
 * track(() => { // reactive reads });
 * ```
 * @param effectFn a function (or `EffectBundle`) that is called when tracked function is invalidated
 * @param options `EffectOptions` -- name, defer
 *
 * @example
 * ```ts
 * const [count, setCount] = createSignal(0);
 *
 * const track = createReaction(() => {
 *   console.log("count changed once, re-arm to listen again");
 *   track(() => count()); // re-arm
 * });
 *
 * track(() => count()); // initial arm
 *
 * setCount(1); // logs once, reaction re-armed for next change
 * ```
 *
 * @description https://docs.solidjs.com/reference/solid-js/advanced/specialized-reactivity/create-reaction
 */ function createReaction(e, t) {
    let r = undefined;
    cleanup(() => r?.());
    const n = getOwner();
    // The currently armed effect node. `track()` replaces the previous
    // subscription (1.x semantics): without disposing the superseded arm, its
    // sources stayed live (firing the callback for replaced dependencies), each
    // accumulated arm delivered its own fire, and un-fired arms leaked as live
    // effect nodes until the owner disposed (#2861).
        let o;
    return u => {
        if (o) {
            dispose(o);
            o = undefined;
        }
        runWithOwner(n, () => {
            effect(() => {
                const e = getOwner();
                o = e;
                u();
                return e;
            }, t => {
                o = undefined;
                r?.();
                const n = (e.effect || e)?.();
                if (false && n !== undefined && typeof n !== "function") ;
                r = n;
                dispose(t);
            }, e.error, {
                ...t,
                user: true,
                defer: true
            });
        });
    };
}

/** A promise over a reactive expression (`resolve`, `until`): a pass that
 * runs the expression and settles on a microtask. Not an effect — an
 * effect's run is the flush's effect phase, and a flush parked by the
 * action that is `yield`ing on this promise stashes its runs with the
 * transaction (#2930); the heap runs a pass parked or not. A pending read
 * re-runs it when the source lands (the pass is pending itself); the
 * expression throwing settles the rejection. */
/** The promise-delivery readers' pass (`resolve`, `until`, `refresh`'s
 * waiter): delivers what it read on a microtask. By posture (#3482, #3490):
 * - the reader of the transaction it was created in (`own`: an action's
 *   body, a `refresh()` called there — the waiter reads its slice's frame on
 *   a later microtask) delivers that frame — the staged landing an action
 *   holds (the action awaits the promise that would otherwise wait on its
 *   own settle);
 * - a mainline reader whose pass read a frame another transaction holds
 *   joined it (L2, born held — F-7) and delivers at that commit: the
 *   committed view, never a foreign hold's unrevealed frame. */ function watch(e, t, r, n = 0, o, u = flushTransaction) {
    if (n & CONFIG_AUTHORITATIVE) {
        GlobalQueue.En = truthRead;
        GlobalQueue.tn = truthConfirmed;
    }
    // `config` is on the node for its first pass too (the pass runs inside
    // `computed()`): `until`'s predicate is authoritative from its first read,
    // or an already-displayed guess would satisfy it in a later slice. (REDERIVE
    // after: the first pass's own error is the promise's rejection.)
        computed(() => {
        let c, i = false;
        try {
            c = e();
        } catch (e) {
            if (e instanceof NotReadyError) throw e;
            c = unwrapStatusError(e);
            i = true;
        }
        const s = getOwner();
        // A truth reader waits out a `createDeferred` flight its value derives
        // from, which serves everyone else the stale answer (D-40).
                const f = n & CONFIG_AUTHORITATIVE && GlobalQueue.er?.(s);
        if (f) throw new NotReadyError(f);
        // (Outside a flush a mainline pass joins through `passTx`.)
                const a = flushTransaction ?? passTx;
        if (s.R & REACTIVE_JOINED && a !== null && (u === null || resolveTx(u) !== resolveTx(a))) {
            staleReader(s, resolveTx(a));
            return;
        }
        if (i) return void queueMicrotask(() => r(c));
        o?.(c);
        queueMicrotask(() => t(c));
    }, {
        xe: n
    }).l |= CONFIG_REDERIVE;
}

/**
 * Awaits a reactive expression and returns its first fully-settled value as a
 * `Promise`. Pending async reads (`createMemo` returning a promise, etc.) are
 * waited on; once the expression returns synchronously without `NotReadyError`
 * the promise resolves with that value. If the expression settles with an
 * error instead — including an async source that rejects — the promise
 * rejects with it.
 *
 * Must be called *outside* a tracking scope — it doesn't subscribe, it just
 * resolves the current value once.
 *
 * @example
 * ```ts
 * const user = createMemo(() => fetch(`/users/${id()}`).then(r => r.json()));
 *
 * // outside any reactive scope
 * const initial = await resolve(() => user());
 * ```
 *
 * @param fn a reactive expression to resolve
 */ function resolve(e) {
    return new Promise((t, r) => {
        createRoot(n => watch(e, e => (t(e), n()), e => (r(e), n())));
    });
}

/**
 * Invalidates one reactive source, forcing it to re-execute even if its inputs
 * haven't changed, and returns a promise for the target's NEXT QUIESCENT
 * STATE — the re-ask (and anything that supersedes it) has settled.
 *
 * Pass either a Solid-created accessor or a projected store created from
 * `createStore(fn, ...)` / `createProjection(...)`. `refresh()` is a
 * write-like invalidation operation: it does not read the target's value, and
 * refreshing a plain signal accessor is a no-op that resolves immediately.
 *
 * The returned promise is safe to ignore (fire-and-forget refresh is
 * unchanged, and a failed refetch will not surface an unhandled rejection).
 * Awaiting it gives imperative flows the settle point without a reactive
 * read:
 * - Accessor targets resolve with the settled value; store targets resolve
 *   with the store node passed (reads through it are fresh after the await).
 * - A failed re-ask rejects with the error.
 * - Semantics are quiescence, not flight identity: if another refresh (or
 *   any invalidation) supersedes this one mid-flight, the promise waits for
 *   — and delivers — whatever finally lands.
 *
 * @example
 * ```ts
 * const user = createMemo(async () => fetch(`/users/${id()}`).then(r => r.json()));
 *
 * // Fire-and-forget re-fetch
 * <button onClick={() => refresh(user)}>Reload</button>;
 *
 * // Imperative settle point
 * const fresh = await refresh(user);
 * ```
 */ function refresh(e) {
    const t = e?.[$REFRESH];
    if (!t) {
        return Promise.resolve(undefined);
    }
    // Mark now, watch on a microtask. The waiter is resolve()'s machinery with
    // two extra reader bits, but it must NOT compute at call time (effects
    // recompute eagerly on creation): same-tick refreshes coalesce into ONE
    // re-ask only because every mark lands before anything pulls, and eager
    // per-call pulls turned three refreshes into three fetches. Deferred, the
    // waiter's first read sees the coalesced state: FRESH_READ pulls the node
    // through recompute if it is still dirty (self-deduping — a clean node
    // no-ops, so N waiters cost one pull; this also closes the race where a
    // waiter reads the PRE-re-ask value as settled and delivers stale), after
    // which the read either parks on the re-ask's pending window (async — the
    // settle walk re-runs it on every landing, equal-value included, and a
    // rejection arrives through the effect's error channel) or serves the sync
    // answer. resolve()'s own eager compute is untouched: created after a
    // refresh it still settles stale-while-revalidate (#2930) — its contract is
    // "first settled value", not "next quiescent state".
        markRefresh(t);
    // The transaction `refresh()` is called in (an action's slice): the
    // waiter, created on the microtask below, is its reader (`watch`).
        const r = flushTransaction;
    const n = new Promise((n, o) => {
        queueMicrotask(() => {
            // The root is load-bearing: an ownerless computed is auto-disposing,
            // and the settle walk releases an unobserved one instead of re-running
            // it, so the waiter would never see the refetch land (#3888). Settle
            // disposes the waiter directly; the capture runs inside the watcher's
            // own pass, where the ambient owner IS the watcher — exactly what
            // dispose() takes.
            let u = null;
            const make = () => watch(() => {
                if (u === null) u = getOwner();
                // Authoritative: the truth beneath a guess (`until`'s view). A
                // guessed node's own re-ask is not answered by it (#3895): park
                // on that flight, or reject with its error.
                                const e = read(t);
                if (t.l & CONFIG_GUESS && t.m & (STATUS_PENDING | STATUS_ERROR)) throw t.T?.U;
                return e;
            }, t => {
                n(typeof e === "function" ? t : e);
                dispose(u);
            }, e => {
                o(e);
                dispose(u);
            }, CONFIG_FRESH_READ | CONFIG_AUTHORITATIVE, undefined, r);
            createRoot(make);
        });
    });
    // Fire-and-forget refresh must not turn a failed refetch into an unhandled
    // rejection; awaiting callers attach their own handlers to `promise`.
        n.catch(() => {});
    return n;
}

/**
 * Awaits a reactive predicate and resolves the first time it settles *truthy*,
 * with that (narrowed) value. Falsy results and pending async reads both mean
 * "not yet": the subscription stays live and re-evaluates as sources change.
 * If the predicate settles with an error — a throw, or an async source that
 * rejects — the promise rejects with it, as do timeout and abort.
 *
 * Where {@link resolve} answers "what is this value" (first settled value,
 * whatever it is), `until` answers "when does the world confirm this
 * condition".
 *
 * This is the acknowledgment mechanism for mutations confirmed on a live data
 * channel (sockets, subscriptions, live queries) rather than by the mutation's
 * own response: correlate by a client-generated id or version in the predicate,
 * and let truth arrive however it arrives — push, refetch, or another tab.
 *
 * Must be called *outside* a tracking scope.
 *
 * @example
 * ```ts
 * const clientId = crypto.randomUUID();
 * await socket.send({ clientId, text }); // fire-and-forget transport
 * // Wait until the live source echoes the write:
 * await until(() => messages.some(m => m.clientId === clientId), { timeout: 10_000 });
 * ```
 *
 * @param fn a reactive predicate
 * @param options optional `timeout` (ms) and abort `signal`
 */ function until(e, t) {
    return new Promise((r, n) => {
        const o = t?.signal;
        if (o?.aborted) return n(o.reason);
        // The awaiting action's transaction, if any (its body runs joined to
        // it). `until()` is a declaration of relatedness (#3164 follow-up): the
        // predicate names the condition that confirms the action, so the frame
        // whose landing flips it truthy is the confirming event by the user's
        // own definition — it reveals WITH the action's settle, not before
        // (`saving=true` beside the confirmed `version=1` is a tear no timeline
        // contains). Updates that do not flip the predicate reveal freely.
                const u = flushTransaction;
        createRoot(c => {
            let i;
            let s;
            const settle = e => {
                if (i !== undefined) clearTimeout(i);
                if (s !== undefined) o.removeEventListener("abort", s);
                e();
                c();
            };
            // Authoritative: the predicate sees the truth, never the action's own
            // optimistic guess — a guess cannot satisfy the wait for its own
            // confirmation.
                        watch(e, e => {
                // Falsy is "not yet": keep the subscription live and wait for the
                // next evaluation. Only a truthy settled value resolves.
                if (e) settle(() => r(e));
            }, e => settle(() => n(e)), CONFIG_AUTHORITATIVE, e => {
                // The flip, inside the pass of the flush carrying it: the flush
                // joins the action's transaction, and what it staged is held and
                // revealed with the action.
                if (e && u !== null && globalQueue.Y && resolveTx(u).tt !== 0) joinFuture(u);
            });
            if (t?.timeout !== undefined) i = setTimeout(() => settle(() => n(new TimeoutError)), t.timeout);
            if (o !== undefined) {
                s = () => settle(() => n(o.reason));
                o.addEventListener("abort", s, {
                    once: true
                });
            }
        });
    });
}

/**
 * Schedules `callback` to run **once** after the reactive graph has fully
 * settled — i.e. once every pending async read inside the current owner has
 * resolved and the queue has flushed. Each call registers a single fire; it
 * does not create an ongoing subscription.
 *
 * The canonical lifecycle primitive in 2.0. Three main usages:
 *
 * - **Component-level setup-and-teardown** *(the most common shape)*: run
 *   setup after the component's first stable render and **return a cleanup
 *   function** to dispose it on owner disposal. This is the replacement for
 *   the 1.x `onMount` + `onCleanup` pairing — setup and teardown live in one
 *   block, and `onCleanup` is no longer the right tool for component
 *   bodies. (`onMount` no longer exists in 2.0.)
 * - **Post-settle "ready" hook:** run once after a component's first stable
 *   render — analytics ping, focus, scroll-into-view, etc. No cleanup needed.
 * - **Inside an event handler:** schedule work to run after the action /
 *   transition triggered by the event has completed.
 *
 * Reactive reads inside the callback are *not* tracked — to react to
 * subsequent settles, register a new `onSettled` each time.
 *
 * The callback runs during the settle flush itself, which gives it the same
 * write semantics as every other effect-phase scope (the effect half of
 * `createEffect`, event handlers):
 *
 * - **Writes** are queued into the same flush's continuation — dependent memos
 *   and effects update before the flush returns — but reads inside the
 *   callback keep returning the settled (pre-write) values. A callback never
 *   observes its own unsettled write. Functional setters still compose:
 *   `set(v => v + 1)` twice increments twice.
 * - **`flush()` cannot be called** from inside the callback — the flush is
 *   already running (dev throws; production is a no-op). To force a drain
 *   after this settle, defer it: `queueMicrotask(() => flush())`.
 *
 * `onCleanup` is **not** allowed inside the callback — return a cleanup
 * function instead. The returned cleanup runs on owner disposal.
 *
 * A cleanup return is only honored when `onSettled` is called from an **owned**
 * scope (e.g. a component body). When it fires out of band from an *unowned*
 * scope — an event handler, a tracked effect, or another `onSettled` — there is
 * no owner lifecycle to bind a cleanup to; returning one is a dev-mode error
 * (and is dropped in production). Use the post-settle/event-handler forms below
 * for one-shot work, and keep setup-with-teardown in an owned scope.
 *
 * @example
 * ```tsx
 * // Component-level setup + teardown — replaces onMount + onCleanup.
 * // Subscribe to an external source on mount, unsubscribe on dispose.
 * function useViewportWidth() {
 *   const [width, setWidth] = createSignal(window.innerWidth);
 *   onSettled(() => {
 *     const onResize = () => setWidth(window.innerWidth);
 *     window.addEventListener("resize", onResize);
 *     return () => window.removeEventListener("resize", onResize);
 *   });
 *   return width;
 * }
 * ```
 *
 * @example
 * ```tsx
 * // Post-settle "ready" hook — no cleanup needed.
 * function Dashboard() {
 *   const data = createMemo(async () => fetchData());
 *
 *   onSettled(() => {
 *     analytics.track("dashboard.ready");
 *   });
 *
 *   return <Loading fallback={<Spinner />}><pre>{data()}</pre></Loading>;
 * }
 * ```
 *
 * @example
 * ```tsx
 * // Event-handler — runs after the action settles.
 * function SaveButton() {
 *   const save = action(function* () {
 *     yield api.save();
 *   });
 *
 *   const handleClick = () => {
 *     save();
 *     onSettled(() => toast("Saved!"));
 *   };
 *
 *   return <button onClick={handleClick}>Save</button>;
 * }
 * ```
 *
 * @param callback Function to run; may return a cleanup function that fires
 *   on owner disposal
 */ function onSettled(e) {
    const t = getOwner();
    t && !(t.l & CONFIG_CHILDREN_FORBIDDEN) ? trackedEffect(() => untrack(e), undefined) : globalQueue.enqueue(EFFECT_USER, function fire() {
        // Settled means derived. A settle that reverts optimism (or replays
        // gated reads) only enqueues the affected subscribers; the pass after
        // the commit re-derives them. Fired in the commit pass, the callback
        // read the optimistic source already reverted beside a sync memo of it
        // still holding the optimistic value — reads do not pull (#3411). Fall
        // to the next pass while the heap has work; `run` swapped the queue,
        // so this lands there, and `enqueue` keeps the drain alive.
        if (dirtyQueue.Et >= dirtyQueue.Fe) return globalQueue.enqueue(EFFECT_USER, fire);
        // Unowned, out-of-band fire (no owner, or a children-forbidden one this
        // one-shot must not bind to): a returned cleanup has no lifecycle to
        // attach to. Reject it in dev; in production the return is simply
        // dropped — never bound to an unrelated owner or run eagerly.
                e();
    });
}

/**
 * Schedules `callback` to run **once**, right after the pending flush
 * finishes, in the same microtask checkpoint (before the browser paints).
 * Unlike `onSettled`, it never waits on async work: if that flush is held by
 * a pending transition, the callback still runs.
 *
 * Its main use is keeping a write out of another update's hold. Writes made
 * in the same synchronous frame are applied together, so when one of them
 * starts async work (a route change, a fetch), the others are held with it
 * until it resolves. A write made in `onFlush` happens after that flush, so
 * it is its own update: it commits on its own unless something it changes
 * also derives from the held work (a memo or `createEffect` reading both),
 * in which case it joins that hold as any later write would.
 *
 * Call it from event handlers and effect callbacks (including
 * `createTrackedEffect` and `onSettled` bodies). In an owned scope such as a
 * component body or a computation it warns in dev: use `onSettled` there.
 *
 * - A write in the callback lands after any write made inline in the same
 *   frame. Don't write the same signal both inside and outside it; use a
 *   functional setter if the order matters.
 * - Several `onFlush` callbacks run in call order, each in its own flush:
 *   async work one callback starts does not hold another's writes.
 * - `queueMicrotask` is not a substitute: called before the frame's first
 *   write, it runs before that flush and its write joins it.
 *
 * @example
 * ```tsx
 * // A menu closes now even when the consumer's onSelect starts async work.
 * const select = (id: string) => {
 *   onFlush(() => setOpen(false));
 *   props.onSelect?.(id);
 * };
 * ```
 *
 * @param callback Function to run once after the pending flush
 */ function onFlush(e) {
    // The frame's flush is queued (or already draining) ahead of this
    // microtask, so the callback's writes start a flush of their own. Each
    // callback flushes before the next runs, so async work one starts never
    // holds another's writes.
    schedule();
    queueMicrotask(() => {
        e();
        flush();
    });
}

export { accessor, createEffect, createMemo, createOptimistic, createReaction, createRenderEffect, createSignal, createTrackedEffect, onCleanup, onFlush, onSettled, refresh, resolve, until };