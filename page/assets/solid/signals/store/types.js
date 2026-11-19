/** Brand keys the proxy answers directly (never user data). */
const $TRACK = Symbol(0), 
/** A store target's guess carrier (optimistic.ts, S2): one per lane. */
$PATCH = Symbol(0), $TARGET = Symbol(0), $PROXY = Symbol(0), $RECORD = Symbol(0);

// ---------------------------------------------------------------------------
// raw marking (R42 / RUL-12: a value is never both wrapped and raw)
const rawValues = new WeakSet;

let rawValuesUsed = false;

function isRawValue(e) {
    return rawValuesUsed && rawValues.has(e);
}

/** Keep a value out of store wrapping: every store serves it as is. Pass a
 * class to keep all instances of it and its subclasses raw. */ function markRaw(e) {
    if (typeof e === "function") {
        if (e.prototype) wrappableProtos.set(e.prototype, false);
    } else if (isWrappable(e)) {
        rawValuesUsed = true;
        rawValues.add(e);
    }
    return e;
}

/** A shallow store's ingested record: raw by contract (values served
 * verbatim, slots replaced by reference). */ function markRawOne(e) {
    if (isWrappable(e)) {
        if (e[$TARGET] !== undefined) return;
        rawValuesUsed = true;
        rawValues.add(e);
    }
}

function markRawIngest(e) {
    if (Array.isArray(e)) {
        for (let a = 0, r = e.length; a < r; a++) markRawOne(e[a]);
    } else {
        for (const a in e) markRawOne(e[a]);
    }
}

// ---------------------------------------------------------------------------
// wrappability
const OBJECT_PROTO = Object.prototype;

const wrappableProtos = new WeakMap;

function isWrappable(e) {
    if (e == null || typeof e !== "object") return false;
    const a = Object.getPrototypeOf(e);
    if (a === OBJECT_PROTO || a === null) return true;
    if (Array.isArray(e)) return true;
    let r = wrappableProtos.get(a);
    if (r === undefined) {
        r = Object.prototype.toString.call(e) === "[object Object]" && (typeof Node === "undefined" || !(e instanceof Node)) && isWrappable(a);
        wrappableProtos.set(a, r);
    }
    return r;
}

function ownEnumerableKeys(e) {
    return Reflect.ownKeys(e).filter(a => Object.prototype.propertyIsEnumerable.call(e, a));
}

// ---------------------------------------------------------------------------
// write override: a projection's draft ops carry it (the derive is the
// author — its writes must land outside the sync setter scope too, in a
// continuation after an `await`/`yield`). Projections return in S3; the
// traps already honour it.
let writeOverride = false;

function setWriteOverride(e) {
    writeOverride = e;
}

/** The list driver's view of a shallow store (map.ts): the backing a read of
 * every slot serves this reader, or `undefined` when the list is no shallow
 * store or a slot read could differ from it. Installed by store.ts's top
 * level, so a `<For>` in an app without stores carries none of it; it lives
 * beside `$TRACK` so map.ts reaches no other store module. */ let shallowView = null;

function installShallowView(e) {
    shallowView = e;
}

export { $PATCH, $PROXY, $RECORD, $TARGET, $TRACK, installShallowView, isRawValue, isWrappable, markRaw, markRawIngest, markRawOne, ownEnumerableKeys, rawValuesUsed, setWriteOverride, shallowView, writeOverride };