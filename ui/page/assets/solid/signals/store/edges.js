import { CONFIG_SLOT_NODE } from "../core/constants.js";

import { signalName } from "../core/identities.js";

import { $TARGET } from "./types.js";

/**
 * Deferred hydration, dependency edges for stores. Only the frames client
 * imports this module, so nothing here ships without it.
 *
 * A plain store root (`createStore` over an object) is named like a plain
 * signal (core/identities.ts: `<owner id>~<n>`, on the same count), and the
 * whole store goes by that one name: the server records it for any read of
 * the store, at any depth, and a setter call on the store — on its root or
 * on a proxy nested in it — is a write to it. Its readers mount at the
 * setter's entry, before the draft runs, as a plain signal's do.
 */
/**
 * @internal The name of the store `x` belongs to — `x` a store proxy (a
 * setter's entry) or one of its property nodes (the setter notifying it) —
 * or undefined when it has none or it cannot be placed: a projection or
 * optimistic family, a store chained over another store's proxy, a root
 * named twice, or a proxy reached through a store other than the one it
 * was named as the root of.
 */ function storeName(e) {
    if (e === undefined) return undefined;
    let n = e.l & CONFIG_SLOT_NODE ? e.O : e[$TARGET];
    if (n === undefined) return undefined;
    // Up to the root; a target on the way named as a root of its own is one
    // object in two stores.
        while (n.u !== null) {
        if (n.rr !== null || n.ch || signalName(n) !== undefined) return undefined;
        n = n.u;
    }
    if (n.rr !== null || n.ch) return undefined;
    return signalName(n) || undefined;
}

export { storeName };