/**
 * Ownership stamp (#3360): every backing the store ALLOCATES (CoW clones,
 * privatized committed backings) carries its owning target under this
 * enumerable symbol. One property write replaces the two weak-collection
 * registrations (ownership set + raw→target map) a fresh object used to pay
 * per draft — V8's identity-hash + ephemeron cost dominated the one-key
 * write floor. Enumerable so a spread copy (the plain-data clone path) stays
 * on the fast path and carries the stamp along.
 *
 * Owned backings are never user-reachable (`snapshot` copies them, the traps
 * hide the key), so every raw key walk in the store must skip `$OWNER`, and
 * ownership is answered by `isOwned` — a user object never carries it.
 * Overlay drafts (`Object.create(v)` over an owned `v`) inherit the stamp.
 */
const $OWNER = Symbol(0);

/** raw → target for plain (family-less) stores. Family stores use their own
 * map so one raw can wrap once per family (§7b). */ const storeLookup = new WeakMap;

function setLookup(o, e) {
    (e.rr?.map ?? storeLookup).set(o, e);
}

function isOwned(o) {
    return o[$OWNER] !== undefined;
}

/** The target wrapping `raw` in `fam` (or the plain lookup). An owned raw
 * answers from its stamp; a user raw from the map. */ function lookupTarget(o, e) {
    const n = o[$OWNER];
    return n !== undefined && n.rr === e ? n : (e?.map ?? storeLookup).get(o);
}

/** Sticky descendants flag up the parent chain (§6d): adoption descends
 * into a changed child pair only where a target with nodes exists below. */ function markDescendants(o) {
    let e = o;
    while (e && !e.d) {
        e.d = true;
        e = e.u;
    }
}

export { $OWNER, isOwned, lookupTarget, markDescendants, setLookup, storeLookup };