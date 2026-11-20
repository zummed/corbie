import { signal, setSignal, read } from "./core.js";

import { cleanup } from "./owner.js";

import { setExternalHooks } from "./scheduler.js";

let externalSourceConfig = null;

/**
 * Registers a factory that bridges external reactive systems (e.g. MobX, Vue refs)
 * into Solid's tracking graph. Every computation will be wrapped so that the
 * external library can track its own dependencies alongside Solid's.
 *
 * Multiple calls pipe together: each new factory wraps the previous one.
 *
 * @param config.factory receives `(fn, trigger)` — wrap fn execution in external tracking,
 *   call trigger when external deps change. Return `{ track, dispose }`.
 * @param config.untrack optional wrapper for `untrack` — disables external tracking too.
 *
 * @example
 * ```ts
 * // Bridge an external "subscribe / notify" library into Solid's graph.
 * // `factory` wraps every Solid compute so the external library can attach
 * // its own dependency tracker; `trigger` re-runs the compute on external
 * // change. `untrack` mirrors Solid's `untrack()` into the external library
 * // so that reads inside `untrack(...)` don't get tracked twice.
 * enableExternalSource({
 *   factory: (compute, trigger) => {
 *     const sub = externalLib.subscribe(trigger);
 *     return {
 *       track: prev => externalLib.run(() => compute(prev)),
 *       dispose: () => sub.unsubscribe()
 *     };
 *   },
 *   untrack: fn => externalLib.untracked(fn)
 * });
 * ```
 */
// Wires a freshly created computed through the active external-source bridge.
// Lives here (installed as a scheduler hook while a config is active) rather than
// inline in core: esbuild cannot literal-track the mutable config binding the
// way rollup does, so an inline `if (externalSourceConfig)` block ships in
// every bundle even though only enableExternalSource() can make it reachable.
function wireExternalSource(e) {
    const n = signal(undefined, {
        equals: false,
        ownedWrite: true
    });
    const r = externalSourceConfig.factory(e.X, () => {
        setSignal(n, undefined);
    });
    cleanup(() => r.dispose());
    e.X = e => {
        read(n);
        return r.track(e);
    };
}

function externalUntrack(e) {
    return externalSourceConfig.untrack(e);
}

// The hooks mirror the config's liveness exactly (installed on enable,
// removed on reset) so core's presence checks stay equivalent to the old
// `externalSourceConfig` truthiness checks.
function syncExternalHooks() {
    if (externalSourceConfig) setExternalHooks(wireExternalSource, externalUntrack); else setExternalHooks(undefined, undefined);
}

function enableExternalSource(e) {
    const {factory: n, untrack: r = e => e()} = e;
    if (externalSourceConfig) {
        const {factory: e, untrack: o} = externalSourceConfig;
        externalSourceConfig = {
            factory: (r, o) => {
                const t = e(r, o);
                const a = n(e => t.track(e), o);
                return {
                    track: e => a.track(e),
                    dispose() {
                        a.dispose();
                        t.dispose();
                    }
                };
            },
            untrack: e => o(() => r(e))
        };
    } else {
        externalSourceConfig = {
            factory: n,
            untrack: r
        };
    }
    syncExternalHooks();
}

export { enableExternalSource, externalSourceConfig };