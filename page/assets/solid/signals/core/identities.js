import { CONFIG_TRANSPARENT } from "./constants.js";

import "./core.js";

import { getOwner } from "./owner.js";

// Deferred hydration, dependency edges: plain-signal identities.
// While on (`setSignalIdentities`; the server-components client turns it on
// before hydrating), a plain signal created under an id-bearing owner is
// named `<owner id>~<n>`, n counting the plain signals (and plain store
// roots: `createStore` over an object, store/edges.ts) created under that
// owner in its current pass (transparent owners defer to the nearest
// id-bearing one, as child ids do). The server names the same signal the
// same way and restarts the count on every re-run, so an edge it shipped (a
// slot fill read it) matches the client node; the write hook receives the
// node and looks its name up (`signalName`). The count restarts when the owner's pass
// generation (`_depGen`, bumped by every recompute) moves on. Only the
// frames client imports this module, so nothing here ships without it.
// Names live in a side table, not on the node: frames reads them across the
// package boundary, where a mangled field would not survive.
let counts = null;

const names = new WeakMap;

/** @internal The identity of a plain signal named while identities were on. */ function signalName(n) {
    return n === undefined ? undefined : names.get(n);
}

function identify(n) {
    let e = getOwner();
    if (!e) return;
    while (e.l & CONFIG_TRANSPARENT && e._parent) e = e._parent;
    if (e.id == null) return;
    const t = e.de;
    let i = counts.get(e);
    if (i === undefined || i[0] !== t) counts.set(e, i = [ t, 0 ]);
    const s = e.id + "~" + i[1]++;
    // A store root named twice (one object made into two stores, or a store
    // handed to `createStore`) is one target under two server names: its
    // writes cannot be attributed (an empty name).
        names.set(n, names.has(n) ? "" : s);
}

/** @internal Name plain signals from now on (`on`), or stop. */ function setSignalIdentities(n) {
    counts = n ? counts || new WeakMap : null;
    signalHook = n ? identify : null;
}

/**
 * @internal Called with every plain signal `createSignal` makes, and every
 * plain store root `createStore` makes, while set (the namer above). Never
 * set unless `setSignalIdentities` is imported, so a bundle without it keeps
 * only a null check.
 */ let signalHook = null;

/**
 * @internal Dev only: `watch` is called with every node read while set (the
 * dev hook `onRead`) — the frames client's identity-drift check watches the
 * named signals a deferred occurrence reads as it mounts. A no-op in prod.
 */ function setReadWatch(n) {}

export { setReadWatch, setSignalIdentities, signalHook, signalName };