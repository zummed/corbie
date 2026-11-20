import { ContextNotFoundError, NoOwnerError } from "./error.js";

import { getOwner } from "./owner.js";

/**
 * Context provides a form of dependency injection. It is used to save from needing to pass
 * data as props through intermediate components. This function creates a new context object
 * that can be used with `getContext` and `setContext`.
 *
 * A default value can be provided here which will be used when a specific value is not provided
 * via a `setContext` call.
 */ function createContext(e, t) {
    return {
        id: Symbol(t),
        defaultValue: e
    };
}

/**
 * Low-level owner-targeted context read. The user-facing read API is
 * `useContext` (in `solid-js`), which wraps this primitive. Exposed here for
 * cross-package wiring (e.g. hydration-aware context plumbing).
 *
 * @throws `NoOwnerError` in dev if there's no owner at the time of call; production
 *   reads the default instead (see below).
 * @throws `ContextNotFoundError` if a context value has not been set yet.
 *
 * @internal
 */ function getContext(e, t = getOwner()) {
    // `undefined` alone means unset — a provided `null` is a value (no `??`).
    let r;
    if (t) r = t.A[e.id];
    if (r === undefined) r = e.defaultValue;
    if (r === undefined) {
        // Dev names the context and why it has no value: no provider, or a
        // provider that passed `undefined` (the key is set, to `undefined`).
        throw new ContextNotFoundError;
    }
    return r;
}

/**
 * Low-level owner-targeted context write. The user-facing API is
 * `createContext` (in `solid-js`); its provider component wraps this
 * primitive. Exposed here for cross-package wiring.
 *
 * @throws `NoOwnerError` if there's no owner at the time of call.
 *
 * @internal
 */ function setContext(e, t, r = getOwner()) {
    if (!r) {
        throw new NoOwnerError;
    }
    // We're creating a new object to avoid child context values being exposed to parent owners. If
    // we don't do this, everything will be a singleton and all hell will break lose.
        r.A = {
        ...r.A,
        [e.id]: t === undefined ? e.defaultValue : t
    };
}

export { createContext, getContext, setContext };