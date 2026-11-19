import { ROOT_ERROR_HOOK } from "./scheduler.js";

let ambientHook;

const reported = new WeakSet;

/**
 * Registers the ambient client error hook — the one call a browser `init()`
 * makes to see every failure a boundary renders a fallback for, in
 * production. (Uncaught errors reach `reportError` / `window.onerror`.)
 *
 * ```ts
 * configureClientErrors({
 *   onError: (error, { ownerPath }) =>
 *     Sentry.captureException(error, {
 *       mechanism: { type: "solid.error_boundary", handled: true }
 *     })
 * });
 * ```
 */ function configureClientErrors(e) {
    if (e && e.onError !== undefined && typeof e.onError !== "function") {
        throw new TypeError(`Invalid onError: expected a function, received ${typeof e.onError}.`);
    }
    ambientHook = e ? e.onError : undefined;
}

/** The nearest root's hook above `owner` (parked under `ROOT_ERROR_HOOK`), else the ambient one. */ function hookFor(e) {
    for (let n = e; n; n = n._parent) {
        const e = n[ROOT_ERROR_HOOK];
        if (e !== undefined) return e;
    }
    return ambientHook;
}

/** Component labels up the owner chain, root first — `_name` where the runtime keeps it. */ function labels(e) {
    const n = [];
    for (let o = e; o; o = o._parent) {
        const e = o._name;
        if (typeof e === "string" && e.length) n.push(e);
    }
    return n.length ? n.reverse() : undefined;
}

/**
 * Tells the client error hook about `error`, caught by the boundary whose
 * owner is `owner`, thrown by `thrower` (the computation the engine's status
 * wrapper named; unknown for a value that never crossed one) — once per
 * error object. A throwing hook is reported on the console and otherwise
 * ignored — a monitor must never take the app down.
 * @internal
 */ function reportClientError(e, n, o) {
    const r = e !== null && (typeof e === "object" || typeof e === "function");
    if (r) {
        if (reported.has(e)) return;
        reported.add(e);
    }
    const t = hookFor(n);
    if (t === undefined) return;
    const i = {};
    const f = labels(n);
    const c = labels(o) ?? f;
    if (c !== undefined) i.ownerPath = c;
    if (f !== undefined) i.boundaryPath = f;
    try {
        t(e, i);
    } catch (e) {
        console.error(e);
    }
}

export { configureClientErrors, reportClientError };