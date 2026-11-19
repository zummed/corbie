import { createStore as createStore$1, deep as deep$1, snapshot as snapshot$1 } from "./store.js";

export { storeHasFamily, storeHasOptimisticFamily, storeIsShallow } from "./store.js";

export { createOptimisticStore } from "./optimistic.js";

import { createStoreDerived } from "./projection.js";

export { createProjection } from "./projection.js";

import { reconcileState } from "./reconcile.js";

import "../core/verdict.js";

import "../core/core.js";

import "../core/scheduler.js";

import "../core/effect.js";

import "../core/guesses.js";

function createStore(e, r, t) {
    if (typeof e === "function") return createStoreDerived(e, r, t);
    return createStore$1(e, r);
}

/**
 * A setter transform that reconciles `value` into the store by key
 * (`"id"` by default; `null` positional), preserving store identity for
 * surviving rows.
 */ function reconcile(e, r = "id") {
    return t => reconcileState(e, t, r);
}

/** Non-tracking snapshot of a store's current value (source identity for
 * unmodified subtrees). */ function snapshot(e) {
    return snapshot$1(e);
}

/** Tracking snapshot: subscribes to every reachable record. */ function deep(e) {
    return deep$1(e);
}

export { createStore, deep, reconcile, snapshot };