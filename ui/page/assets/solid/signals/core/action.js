import { isThenable } from "./async.js";

import "./core.js";

import { joinFuture, newTransaction, enterAction, exitAction, resolveTx, schedule, actionDepth, flush, nextQuestion, setQuestion, question } from "./scheduler.js";

/** A slice of the body runs in the action's transaction: the flush that
 * carries its writes parks into it (`joinFuture` — merging whatever the tick
 * already joined: O1, an action that opens in a tick owns it; a second
 * action in the tick, or one called from the body, merges into the first).
 * A resumed slice drains at once: the seam parks what it wrote (the action
 * holds the transaction open) or, the body having returned, lands it. A
 * nested action resuming synchronously inside the outer body does not drain
 * — that would park the outer's remaining writes' frame mid-slice; the outer
 * step's own return does. */ function inTransaction(t, n, e) {
    // The slice and its flush ask this action's question (F-11):
    // the guesses it writes and the flights it starts are stamped with it.
    const i = question;
    setQuestion(n);
    joinFuture(t);
    const o = e();
    if (actionDepth === 0) flush();
    setQuestion(i);
    return o;
}

/**
 * The primitive for mutations: imperative async workflows whose *writes span
 * an async gap* — optimistic write, server round-trip, reconciling write —
 * where intermediate state must not leak and failure must revert cleanly
 * (pair with `createOptimistic` / `createOptimisticStore`).
 *
 * Navigation-shaped updates do not need an action. A plain setter call is
 * enough: reads pull the async, and downstream async computeds hold their
 * previous values per-node until the new ones are ready (`isPending` /
 * `latest` expose the in-flight state). Reach for `action` only when writes
 * happen *after* async work, not merely upstream of it.
 *
 * Framework-level actions (router form actions, server actions) are
 * specializations of this primitive: they are actions in exactly this sense —
 * the same transactional semantics — with form binding, serialization, and
 * submission tracking layered on top. The shared name is deliberate.
 *
 * Wraps a generator function so each invocation runs as a single transaction
 * that holds every signal/store write between yields. The surrounding UI sees
 * one atomic update when the action completes; nothing is committed until
 * every action in the transaction has returned.
 *
 * `yield` is the transaction-safe suspension point: the action waits for a
 * yielded promise and re-enters the transaction before running the code after
 * it. A plain `await` does NOT — the runtime has no hook into an async
 * generator's internal await continuations, so code between an `await` and
 * the next `yield` runs OUTSIDE the transaction: writes to fresh signals
 * commit immediately, and anything that creates a reader there — `until()`,
 * `latest()`, a memo or effect, a mount — is created mainline, where a read of
 * this action's held state makes it born held (F-7): staged with the
 * transaction and replayed at its commit. `await` is still the ergonomic
 * choice for typed results; just put a bare `yield` before any write or
 * reader creation that follows it — including the expression of the next
 * `yield`, which is evaluated before the step re-enters:
 *
 * ```ts
 * const saved = await api.createTodo(text); // typed result
 * yield; // re-enter the transaction before writing or reading
 * setTodos(t => { ... });
 * yield until(() => todos.some(t => t.id === saved.id));
 * ```
 *
 * The same holds inside an async helper the action yields
 * (`yield (async () => { ... })()`): everything after its first `await` runs
 * outside the transaction. An `await refresh(x)` or `await until(...)` there
 * waits for this action's commit, which waits on the helper, so it hangs
 * whenever `x` re-ran during the action (#4263) and works otherwise. Call
 * `refresh(x)` in the generator once the helper returns; `yield` it only when
 * the action needs the fresh value.
 *
 * (For the same reason, don't call `flush()` inside an action body — it
 * cannot reveal the held writes and is refused.)
 *
 * Each call returns a `Promise` that resolves with the generator's return
 * value, or rejects if it throws. Pair with `createOptimistic` /
 * `createOptimisticStore` to apply tentative writes that auto-revert if the
 * action fails.
 *
 * @example
 * ```ts
 * const [todos, setTodos] = createOptimisticStore<Todo[]>([]);
 *
 * const addTodo = action(async function* (text: string) {
 *   const tempId = crypto.randomUUID();
 *   setTodos(t => { t.push({ id: tempId, text, pending: true }); }); // optimistic
 *   const saved = await api.createTodo(text); // network round-trip, typed
 *   yield; // re-enter the transaction
 *   setTodos(t => {
 *     const i = t.findIndex(x => x.id === tempId);
 *     if (i >= 0) t[i] = saved;
 *   });
 *   return saved;
 * });
 *
 * await addTodo("buy milk");
 * ```
 */ function action(t) {
    return (...n) => new Promise((e, i) => {
        const o = t(...n);
        // The action's transaction, held open until the body returns. The
        // first slice's writes are carried by the tick's scheduled flush.
                const r = newTransaction(false);
        r.tt = 1;
        r.nt = true;
        const s = nextQuestion();
        const done = (t, n, o = false) => {
            resolveTx(r).tt--;
            schedule();
            o ? i(n) : e(t);
        };
        const step = (t, n) => {
            let e;
            // The body is on the stack between these brackets: flush() is
            // refused inside (FLUSH_IN_ACTION, scheduler.ts).
                        enterAction();
            try {
                e = n ? o.throw(t) : o.next(t);
            } catch (t) {
                exitAction();
                return done(undefined, t, true);
            }
            exitAction();
            // A rejected iterator result (async generators) means the error already
            // escaped the generator body — it is completed, and throwing back in
            // would just reject again forever. Settle instead.
                        if (isThenable(e)) return void e.then(run, t => done(undefined, t, true));
            run(e);
        };
        const run = t => {
            if (t.done) return done(t.value);
            // Thenable assimilation can itself throw synchronously (a `then`
            // getter, or a `then()` method that throws — #2918). Match `await`
            // semantics: the failure is thrown back into the generator at the
            // yield point (catchable there); if uncaught, step()'s guard settles
            // the action. The settled flag implements A+ 2.3.3.3.4.1: a throw
            // after the thenable already called a callback is ignored.
                        let n = false;
            try {
                if (isThenable(t.value)) return void t.value.then(t => {
                    if (n) return;
                    n = true;
                    inTransaction(r, s, () => step(t));
                }, t => {
                    if (n) return;
                    n = true;
                    inTransaction(r, s, () => step(t, true));
                });
            } catch (t) {
                if (n) return;
                n = true;
                return void inTransaction(r, s, () => step(t, true));
            }
            inTransaction(r, s, () => step(t.value));
        };
        // The first slice's writes are carried by the tick's scheduled flush;
        // the slice itself asks the action's question (its guesses are stamped
        // at the write).
                joinFuture(r);
        const u = question;
        setQuestion(s);
        step();
        setQuestion(u);
    });
}

export { action };