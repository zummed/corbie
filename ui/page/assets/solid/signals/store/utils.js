import { SUPPORTS_PROXY } from "../core/constants.js";

import { createMemo } from "../signals.js";

import { $PROXY, $RECORD, $TARGET, ownEnumerableKeys } from "./types.js";

function trueFn() {
    return true;
}

/** @internal What a source ENTRY is, decided once when the view is built
 * (`merge()` learns it while flattening; `omit()` from its argument) and
 * carried beside the entry — `MergeView.kinds[i]`, `OmitView.kind` — so no
 * read has to ask. Asking is the cost: any brand check on a Proxy is a trap
 * (`instanceof` is a `getPrototypeOf` trap, ~20 ns on a store — as much as
 * the read itself), and a merge over a store did two per read. */ const SOURCE_PLAIN = 0;

 // a plain object: own keys fixed, data is data
const SOURCE_OMIT = 1;

 // an `OmitView` record (only as a merge entry)
const SOURCE_PROXY = 2;

 // a store or foreign proxy: everything is a trap
const SOURCE_MEMO = 3;

 // a function source (merge's memo): swaps objects
const SOURCE_MERGE = 4;

 // a `MergeView` record (only as an omit's source)
const EMPTY = Object.freeze({});

// The object behind a LEAF entry (any kind but OMIT): a memo is read
// (tracked, as merge's own reads are) and a nullish result has no keys.
function leafOf(e, n) {
    return n === SOURCE_MEMO ? (e = e()) || EMPTY : e;
}

/** @internal The record behind an `omit()` proxy: `source` with `hidden`
 * keys removed. It is the proxy's TARGET, so the shared handler reads it as
 * plain fields — no per-instance closures — and it is what props consumers
 * walk directly (`merge`, `spread`, `ssrElement`): a view never materializes
 * a copy, and a consumer that knows the record never goes through its traps
 * (a `getOwnPropertyDescriptor` trap per key allocates a descriptor and a
 * getter, so enumerating a proxy costs more than the copy it was avoiding).
 * `hidden` is a key list or a predicate (`omit(props, k => k[0] === "$")`).
 *
 * An omit over a `merge()` holds the merge's RECORD (`MergeView`, kind
 * `SOURCE_MERGE`) — never its proxy, so no read hops through a trap — and is
 * one record however many leaves the merge has. A consumer walks it as ONE
 * filtered entry (`sourceKeys` / `sourceGet` recurse into the merge's
 * sources by function call), and a later `merge()` over it carries the
 * record as one entry instead of copying its leaves: on a component chain of
 * defaults + omit + spread (`merge(omit(merge(omit(props))))`) the layers
 * nest as records, each a few fields, where a flatten to leaf views built a
 * view and a combined key list per leaf per layer — the largest allocation
 * of a Kobalte-shaped render. A merge leaf may be merge's memo for a
 * function source; it is resolved on access. */ class OmitView {
    source;
    kind;
    hidden;
    /** see `MergeView.table` */
    table=0;
    /** see `tableOwnKeys` / `tableDescriptor` */
    keys=undefined;
    descs=undefined;
    constructor(e, 
    /** of `source` — PLAIN, PROXY (a store or a foreign proxy), MEMO, or
     * MERGE (a `MergeView` record); never OMIT, a view over a view folds
     * into one. */
    n, t) {
        this.source = e;
        this.kind = n;
        this.hidden = t;
    }
}

function isHidden(e, n) {
    const t = e.hidden;
    return typeof t === "function" ? t(n) : t.includes(n);
}

// Both filters as one. Two key lists stay a key list (one `includes`, no
// closure); a predicate on either side needs a closure. An omit over a
// merge builds one combined list per leaf, per component layer, so the
// copy's form matters in every tier: `concat` runs the species/spreadable
// protocol (2–3× the cost of a copy once optimized), a hand loop is 2–4×
// `concat` in the interpreter and baseline tiers (a bytecode per element
// against one builtin), and a presized `new Array(n)` is holey, which takes
// `includes` off its fast path. `slice` + `push` of the (short) second list
// is within a third of the best form in every tier, and packed.
function combineHidden(e, n) {
    if (typeof e !== "function" && typeof n !== "function") {
        const t = e.slice();
        for (let e = 0; e < n.length; e++) t.push(n[e]);
        return t;
    }
    return t => (typeof e === "function" ? e(t) : e.includes(t)) || (typeof n === "function" ? n(t) : n.includes(t));
}

// The object a view filters (see `leafOf`).
function viewSource(e) {
    return leafOf(e.source, e.kind);
}

// The record behind a `$PROXY`-marked object that is one of OUR views, else
// undefined for a store or a foreign proxy. ONE read: the view traps answer
// `$RECORD` first thing and a store's `get` trap answers it `undefined` on
// its symbol fast path, so the question never reaches a store's generic
// read path (firewall gate, tracked key read), which is what an unknown
// symbol would take — and a foreign proxy forwards it to a target that has
// no such key. Asking `$TARGET` first and then each view kind was two to
// four trap hops per source at every `merge()`, `omit()` and `ssrElement`.
// Call only after `$PROXY in o` is known true.
function recordOf(e) {
    return e[$RECORD];
}

/** @internal The `OmitView` behind an `omit()` proxy, or undefined. */ function omitView(e) {
    if (!e || !($PROXY in e)) return undefined;
    const n = e[$RECORD];
    return n instanceof OmitView ? n : undefined;
}

// A props SOURCE ENTRY is a plain object, a proxy (store, merge, omit — the
// last two are normally unwrapped first: `mergeView` / `omitView`), a memo,
// or an `OmitView` record, and travels with its kind. These three answer for
// an entry what `Object.keys` / `in` / `[]` answer for an object, so every
// consumer walks entries with one code path, an `OmitView` is filtered
// rather than materialized, and nothing is asked of a proxy but the read.
// Own keys of a leaf: a proxy answers through ONE `ownKeys` trap (a store's
// keeps the key set tracked; `Object.keys` on a proxy would add a descriptor
// trap per key); a plain object its enumerable string keys. What a memo
// holds is only known once read.
function leafKeys(e, n) {
    if (n === SOURCE_PLAIN) return Object.keys(e);
    if (n === SOURCE_PROXY || e[$PROXY] === e) {
        // A store's handler answers directly: the Proxy `ownKeys` invariant checks cost twice the trap.
        const n = e[$TARGET];
        return (n?.px === e ? n : Reflect).ownKeys(e);
    }
    return Object.keys(e);
}

/** @internal Own string keys of a source entry — every consumer skips
 * symbols itself. */ function sourceKeys(e, n) {
    if (n === SOURCE_OMIT) {
        if (e.kind === SOURCE_MERGE) return mergeKeysOf(e.source, false, e);
        const n = leafKeys(viewSource(e), e.kind);
        const t = [];
        for (let r = 0; r < n.length; r++) if (!isHidden(e, n[r])) t.push(n[r]);
        return t;
    }
    return leafKeys(leafOf(e, n), n);
}

/** @internal `key in entry`. */ function sourceHas(e, n, t) {
    if (n === SOURCE_OMIT) {
        if (isHidden(e, t)) return false;
        return e.kind === SOURCE_MERGE ? mergeHas(e.source, t) : t in viewSource(e);
    }
    return t in leafOf(e, n);
}

/** @internal `entry[key]` — the source's getter runs once, here. */ function sourceGet(e, n, t) {
    if (n === SOURCE_OMIT) {
        if (isHidden(e, t)) return undefined;
        return e.kind === SOURCE_MERGE ? mergeGet(e.source, t) : viewSource(e)[t];
    }
    return leafOf(e, n)[t];
}

// An entry whose own key set is fixed: a plain object, or a view over one —
// an omit of a plain object, or of a merge whose entries all are. Not a
// store (its key set is a tracked signal), not a merge memo source (it
// swaps whole objects), and not any proxy that declares itself with
// `$PROXY in s` — a frames slot proxy answers `has` for every key and lists
// none, so only the `in` walk is right for it.
function entryHasStaticKeys(e, n) {
    if (n === SOURCE_PLAIN) return true;
    if (n !== SOURCE_OMIT) return false;
    if (e.kind === SOURCE_PLAIN) return true;
    return e.kind === SOURCE_MERGE && mergeHasStaticKeys(e.source);
}

function mergeHasStaticKeys(e) {
    const n = e.sources, t = e.kinds;
    for (let e = 0; e < n.length; e++) if (!entryHasStaticKeys(n[e], t[e])) return false;
    return true;
}

/** @internal Whether the own key set of a props object cannot change
 * reactively: a plain object, or a merge/omit view over plain objects only.
 * A consumer may then decide from `Object.getOwnPropertyDescriptor` once —
 * "no `children` key" or "a data `children`" holds for the object's lifetime,
 * so no tracking scope is needed for it (#3388). For a store, or a view with
 * a store or memo leaf, keys can appear later and the reactive path is the
 * only correct one. */ function hasStaticKeys(e) {
    if (!($PROXY in e)) return true;
    const n = recordOf(e);
    if (n instanceof MergeView) return mergeHasStaticKeys(n);
    return n !== undefined && entryHasStaticKeys(n, SOURCE_OMIT);
}

/**
 * Whether `o[key]` can never change for the lifetime of `o`: the key is a
 * data property of a plain object, or is absent from an object whose key set
 * is fixed. A getter, a key on a store, a memo-backed `merge()` source, or
 * any key of an object whose keys can appear later (a store) is not static.
 *
 * Looks through `merge()`/`omit()` views to the leaf that owns the key. Any
 * object will do, but props are the case it exists for: the compiler encodes
 * a literal at the call site (`as="button"`) as a data property and an
 * expression (`as={isLink() ? "a" : "button"}`) as a getter, so a component
 * library reads the caller's own static/dynamic classification of a prop at
 * runtime — identically on server and client, the compiler emitting the same
 * own DESCRIPTORS on both (the object behind them may differ: the server
 * builds props as a plain-prototype instance with shared getters) — and can
 * take a no-computation path for the literal:
 *
 * ```tsx
 * const Tag = dynamic(() => props.as, { static: isStatic(props, "as") });
 * ```
 *
 * One descriptor lookup; no read of the value, nothing tracked.
 */ function isStatic(e, n) {
    if ($PROXY in e) {
        // A store answers its descriptor trap with a value; through a view the
        // descriptor is truthful (see `sourceDescriptor`). A foreign proxy is
        // opaque: nothing about it is known to be fixed.
        if (viewOf(e) === undefined) return false;
        const t = Reflect.getOwnPropertyDescriptor(e, n);
        return t === undefined ? hasStaticKeys(e) : t.get === undefined;
    }
    const t = Reflect.getOwnPropertyDescriptor(e, n);
    return t === undefined || t.get === undefined && t.set === undefined;
}

function accessorDescriptor(e, n = true) {
    return {
        configurable: true,
        enumerable: n,
        get: e,
        set: trueFn
    };
}

/** The descriptor a consumer should see for `key` on an entry —
 * the view proxies answer `getOwnPropertyDescriptor` with it, so it tells the
 * truth through any depth of merge/omit layers.
 *
 * A DATA descriptor means "nothing reactive can hide behind this value": the
 * key is a data property of a plain-object leaf — the compiler's own
 * encoding of a static attribute. Everything else is an accessor: a getter
 * on a leaf, a key on a store proxy (its "data" is a signal), or a key on a
 * merge memo source (the whole object is reactive). That is what lets a
 * consumer skip a reactive node for a static prop at the bottom of a
 * component chain, and it is why the store case must NOT forward the store's
 * own descriptor, which reports a value.
 *
 * `configurable: true` always — the target has no such property, and the
 * Proxy invariants forbid reporting a non-configurable one. */
// `present` says the caller has already established `key in s` (a trap's
// shadowing walk did), so a store is not asked a second time.
function sourceDescriptor(e, n, t, r = false) {
    if (n === SOURCE_OMIT) {
        if (isHidden(e, t)) return undefined;
        return sourceDescriptor(e.source, e.kind, t, r);
    }
    // An omit's merge record: the descriptor of the last entry that has the
    // key, as the merge proxy's own trap answers (see `mergeDescriptor`).
        if (n === SOURCE_MERGE) return mergeDescriptor(e, t);
    // A memo source (`merge(() => …)`) is reactive wholesale: whatever shape
    // the memo's current object has, the key is an accessor here.
        if (n === SOURCE_MEMO) {
        return r || t in leafOf(e, n) ? accessorDescriptor(() => leafOf(e, n)[t]) : undefined;
    }
    if (n === SOURCE_PROXY) {
        // Another view (an omit's source may be a merge proxy) already answers
        // truthfully. A store's reported "data" is a signal, and a foreign proxy
        // (frames slot props) has no own descriptors: for both, existence is
        // `in` and the kind is accessor — one trap, and never the store's
        // descriptor path.
        if (recordOf(e) !== undefined) return Reflect.getOwnPropertyDescriptor(e, t);
        return r || t in e ? accessorDescriptor(() => e[t]) : undefined;
    }
    const i = Reflect.getOwnPropertyDescriptor(e, t);
    if (i === undefined) return undefined;
    if (i.get !== undefined || i.set !== undefined) return accessorDescriptor(() => e[t], i.enumerable);
    // The proxy target has no such key, so the descriptor must be configurable;
    // Reflect's is a fresh object, so a configurable one is handed out as is.
        if (i.configurable) return i;
    return {
        configurable: true,
        enumerable: i.enumerable,
        writable: true,
        value: i.value
    };
}

// Own ENUMERABLE keys, symbols included, of an entry — the user-facing key
// set (`Object.keys(merged)`), where enumerability matters (#2769).
function sourceEnumerableKeys(e, n) {
    {
        if (e.kind === SOURCE_MERGE) return mergeEnumerableKeys(e.source, e);
        const n = ownEnumerableKeys(viewSource(e));
        const t = [];
        for (let r = 0; r < n.length; r++) if (!isHidden(e, n[r])) t.push(n[r]);
        return t;
    }
}

// The target of a merge() proxy: the flattened sources, read by one shared
// handler — like OmitView, no per-instance closures. `sources` is what
// `mergeSources` answers.
/** @internal */ class MergeView {
    sources;
    kinds;
    /** key → the plain leaf that owns it (later sources win), built by an
     * enumeration or once the reads have paid for it (see `resolvedTable`)
     * when every leaf has static keys; `null` when one doesn't. Until then
     * the slot counts the per-key trap reads so far. One slot rather than a
     * counter field of its own: a view is built per source per component
     * layer, and each field initializer is a measurable share of a
     * constructor that small in the lower JIT tiers. */
    table=0;
    /** see `tableOwnKeys` / `tableDescriptor` */
    keys=undefined;
    descs=undefined;
    constructor(e, 
    /** `kinds[i]` is what `sources[i]` is (see `SourceKind`). */
    n) {
        this.sources = e;
        this.kinds = n;
    }
}

/** @internal The `MergeView` behind a `merge()` proxy — its flattened
 * `sources` with their `kinds` — or undefined. */ function mergeView(e) {
    if (!e || !($PROXY in e)) return undefined;
    const n = e[$RECORD];
    return n instanceof MergeView ? n : undefined;
}

/** @internal The record behind a merge() or omit() proxy — a `MergeView`
 * (flattened `sources` with their `kinds`) or an `OmitView` — or undefined
 * for anything else (a plain object, a store, a foreign proxy). Two fast
 * traps on a proxy (`has`, then `$RECORD`), none on a plain object. */ function viewOf(e) {
    return e && $PROXY in e ? e[$RECORD] : undefined;
}

/** @internal The resolved key table of a merge/omit view — every own key of
 * the view mapped to the plain object that owns it, in merged order (a key
 * at the position of the last source that carries it, see `tableSet`) — or
 * undefined when it has none: a leaf is a store or a memo source, whose
 * keys can change, or the object is not a view at all.
 *
 * This is the flat object the eager copy used to build, made lazily and
 * without copying: one pass over the leaves' own keys, then every
 * `get`/`has`/descriptor is one lookup plus one read of the owning leaf, and
 * a consumer (`spread` rerunning its effect) walks the table instead of
 * re-deriving shadowing from the leaves each time. Own keys only, as the
 * copy's were: a plain source's key set is fixed once merged (keys added to
 * it later are not seen — the copy didn't see them either).
 *
 * It is built by an ENUMERATION — the `ownKeys` trap, or a consumer asking
 * for it here — or once per-key reads have paid for it (`READS_FOR_TABLE`),
 * not on the first read. A per-key read has a direct answer (a walk of the
 * sources, last to first, one `in` each) whose cost is the source count,
 * while the table's is every key of every leaf, so the walk wins until a
 * view has been read about as many times as it has keys. On the server it
 * never is: a component reads its props a few times, the element enumerates
 * them once through its own source walk, and the view is gone — building on
 * first read there cost a component chain a table per layer (profiled on
 * the Kobalte-shaped chain: a third of SSR time in the table code and its
 * garbage). On the
 * client a view read on every reactive rerun crosses the threshold in its
 * first few updates and is one lookup per read from then on, as before.
 * Once built — by a `spread`, `Object.keys`, `{...props}`, or the count —
 * every trap uses it. */ function resolvedTable(e) {
    if (!e || !($PROXY in e)) return undefined;
    const n = recordOf(e);
    if (n instanceof OmitView) return omitTable(n);
    return n === undefined ? undefined : mergeTable(n);
}

function mergeTable(e) {
    let n = e.table;
    if (typeof n !== "object") {
        // a read count: not decided yet
        const t = e.sources, r = e.kinds;
        for (let n = 0; n < t.length; n++) {
            if (!entryHasStaticKeys(t[n], r[n])) {
                e.table = null;
                return undefined;
            }
        }
        n = new Map;
        collectTable(n, e, undefined);
        e.table = n;
    }
    return n === null ? undefined : n;
}

// One pass over a merge record's leaves into `table`, through the nested
// omit-over-merge entries — the filters enclosing the current leaf are the
// `filters` stack — so a component chain builds ONE table at the view that
// asked, not one per layer. A key an entry hides that an EARLIER entry owned
// must stay: a filter applies to its own entry's contribution, not to the
// merge, which is exactly what the stack expresses. Every entry has static
// keys (the caller checked), so a leaf's own keys are the truth.
function collectTable(e, n, t) {
    const r = n.sources, i = n.kinds;
    for (let n = 0; n < r.length; n++) {
        const f = r[n];
        if (i[n] === SOURCE_OMIT) {
            if (f.kind === SOURCE_MERGE) {
                if (t === undefined) t = [ f ]; else t.push(f);
                collectTable(e, f.source, t);
                t.pop();
                continue;
            }
            const n = f.source;
            const r = Reflect.ownKeys(n);
            for (let i = 0; i < r.length; i++) {
                const u = r[i];
                if (!isHidden(f, u) && !hiddenByAny(t, u)) tableSet(e, u, n);
            }
        } else {
            const n = Reflect.ownKeys(f);
            for (let r = 0; r < n.length; r++) {
                const i = n[r];
                if (!hiddenByAny(t, i)) tableSet(e, i, f);
            }
        }
    }
}

function hiddenByAny(e, n) {
    if (e !== undefined) for (let t = e.length - 1; t >= 0; t--) if (isHidden(e[t], n)) return true;
    return false;
}

// Key order is the merged one — every key at the position of the LAST source
// that carries it — the order `ssrElement`'s array form serializes in and the
// eager copy enumerated in, so a spread through a view and a spread over the
// sources emit the same attribute order.
function tableSet(e, n, t) {
    if (e.has(n)) e.delete(n);
    e.set(n, t);
}

// An omit view's table: its source's (a merge's table, or a plain object's
// own keys) minus the hidden keys. Cached on the record.
function omitTable(e) {
    let n = e.table;
    if (typeof n !== "object") {
        const t = e.source;
        if (e.kind === SOURCE_MERGE) {
            // One pass over the merge's leaves with this filter on the stack —
            // the merge record builds no table of its own for it.
            if (!mergeHasStaticKeys(t)) {
                e.table = null;
                return undefined;
            }
            n = new Map;
            collectTable(n, t, [ e ]);
        } else if (e.kind === SOURCE_PLAIN) {
            n = new Map;
            const r = Reflect.ownKeys(t);
            for (let i = 0; i < r.length; i++) {
                const f = r[i];
                if (!isHidden(e, f)) n.set(f, t);
            }
        } else {
            // a store or a memo: keys can change, no table
            e.table = null;
            return undefined;
        }
        e.table = n;
    }
    return n === null ? undefined : n;
}

// The user-facing key set of a resolved table: its keys that are enumerable
// on the leaf that owns them (`Object.keys(merged)`, #2769). Fixed, like the
// table, so it is built once per view: an `ownKeys` trap may hand back the
// same array every time (the engine copies it).
const propertyIsEnumerable = Object.prototype.propertyIsEnumerable;

function tableOwnKeys(e, n) {
    let t = e.keys;
    if (t === undefined) {
        t = e.keys = [];
        for (const [e, r] of n) if (propertyIsEnumerable.call(r, e)) t.push(e);
    }
    return t;
}

// The descriptor for a table key — its owning leaf's, truthful (see
// `sourceDescriptor`) — with the shape cached per key so an enumeration
// (`for…in`, `Object.keys`, `{...props}`: a descriptor trap per key, on
// every pass) does not re-read the leaf's descriptor and re-allocate a
// getter each time. An accessor reads live, so its descriptor is reused as
// is; a data descriptor is rebuilt with the current value.
function tableDescriptor(e, n, t) {
    const r = n.get(t);
    if (r === undefined) return undefined;
    let i = e.descs;
    if (i === undefined) i = e.descs = new Map;
    let f = i.get(t);
    if (f === undefined) {
        f = sourceDescriptor(r, SOURCE_PLAIN, t);
        if (f === undefined) return undefined;
        i.set(t, f);
        return f;
    }
    if (f.get !== undefined) return f;
    return {
        configurable: true,
        enumerable: f.enumerable,
        writable: f.writable,
        value: r[t]
    };
}

// Per-key trap reads a view takes before building its table, from the
// break-even: a build is ~60 ns per key (an `ownKeys` share, a `has`, a
// `set`), a walk ~20 ns per source (an `in`, a hidden-list check), and a
// leaf carries about five keys — so the table has paid for itself after
// ~15 reads. Measured on the Kobalte-shaped chain: a walk is 2× a lookup at
// depth 1 and up to 10× for a first-source key at depth 7 (a walk through
// seven nested layers, a hidden-list check at each), so a view read on
// every update wants the table; a view read a handful of times (every
// server-side view) never wants it.
const READS_FOR_TABLE = 16;

// The table for a per-key trap: the one a view HAS (built by an enumeration
// or an earlier read, see `resolvedTable`), or the one this read pays for,
// or none — a walk answers. One per view type, so a trap pays no type check.
// A settled slot is an object (the Map, or `null`); a number is the count.
function mergeReadTable(e) {
    const n = e.table;
    if (typeof n === "object") return n === null ? undefined : n;
    if (n + 1 < READS_FOR_TABLE) {
        e.table = n + 1;
        return undefined;
    }
    return mergeTable(e);
}

// Only for an omit over a merge (`kind === SOURCE_MERGE`; the caller checks,
// inline — a call is not free in every tier): an omit over one object reads
// it directly — a list check and a property read, nothing a table would
// shorten.
function omitReadTable(e) {
    const n = e.table;
    if (typeof n === "object") return n === null ? undefined : n;
    if (n + 1 < READS_FOR_TABLE) {
        e.table = n + 1;
        return undefined;
    }
    return omitTable(e);
}

// The table a record HAS — built already by an enumeration or a trap's read
// count — or undefined. What a nested walk asks: a record reached through an
// outer view's entry counts no reads of its own (the outer view decides for
// the whole tree, and its table build then builds the inner ones), so the
// inner merges of a component chain build nothing on the server where the
// leaves are read a few times each.
function tableOf(e) {
    const n = e.table;
    return typeof n === "object" && n !== null ? n : undefined;
}

// "no entry has the key" — distinct from an entry that holds `undefined`.
const MISSING = Symbol();

// The read: the value of the last entry that has the key, or MISSING. ONE
// walk — a nested omit-over-merge entry answers presence and value together,
// so a chain of layers is walked once per read, not once per layer per
// level.
function mergeLookup(e, n) {
    const t = tableOf(e);
    if (t !== undefined) {
        const e = t.get(n);
        return e === undefined ? MISSING : e[n];
    }
    const r = e.sources, i = e.kinds;
    for (let e = r.length - 1; e >= 0; e--) {
        const t = i[e];
        // The common leaf first, read in place: a component's props view is a
        // few plain objects, read once per key on the server.
                if (t === SOURCE_PLAIN) {
            const t = r[e];
            if (n in t) return t[n];
            continue;
        }
        if (t === SOURCE_OMIT) {
            const t = r[e];
            if (isHidden(t, n)) continue;
            if (t.kind === SOURCE_MERGE) {
                const e = mergeLookup(t.source, n);
                if (e !== MISSING) return e;
                continue;
            }
            const i = viewSource(t);
            if (n in i) return i[n];
        } else {
            const i = leafOf(r[e], t);
            if (n in i) return i[n];
        }
    }
    return MISSING;
}

function mergeGet(e, n) {
    const t = mergeLookup(e, n);
    return t === MISSING ? undefined : t;
}

// `key in merge`, on the record.
function mergeHas(e, n) {
    const t = tableOf(e);
    if (t !== undefined) return t.has(n);
    const r = e.sources, i = e.kinds;
    for (let e = r.length - 1; e >= 0; e--) if (sourceHas(r[e], i[e], n)) return true;
    return false;
}

// The proxy's `getOwnPropertyDescriptor`, on the record.
function mergeDescriptor(e, n) {
    const t = tableOf(e);
    if (t !== undefined) return tableDescriptor(e, t, n);
    const r = e.sources, i = e.kinds;
    for (let t = r.length - 1; t >= 0; t--) {
        if (!sourceHas(r[t], i[t], n)) continue;
        // `in` also answers for inherited keys, which have no own descriptor.
                return sourceDescriptor(r[t], i[t], n, true) ?? accessorDescriptor(() => mergeGet(e, n));
    }
    return undefined;
}

// Own keys of a merge record in merged order — every key at the position
// of the LAST entry that carries it, the order the table keeps and
// `ssrElement` serializes in. `enumerable` selects the user-facing set
// (`Object.keys`, #2769) over every own string key (a consumer's walk);
// `filter` is the omit this record is read through, applied as the keys
// are gathered so an omit over a merge builds ONE list per layer. A list
// with `indexOf` rather than a Set: a props object has a dozen keys, and a
// Set's hash store was 2 KB per row on the Kobalte-shaped chain.
function mergeKeysOf(e, n, t) {
    const r = [];
    collectKeys(e, t === undefined ? undefined : [ t ], n, r, null);
    return r;
}

// One pass over a merge record's leaves — through its nested omit-over-merge
// entries, `filters` the omits enclosing the current leaf (see
// `collectTable`) — appending each leaf's keys to `keys` in merged order
// (a key already listed moves to the end: later wins) and, when `owners` is
// given, the object that owns the key at the same index. A consumer that
// reads every key once (`ssrElement`) then reads `owners[i][keys[i]]`: no
// `in` walk per key, no table. A memo leaf is resolved once here.
function collectKeys(e, n, t, r, i) {
    const f = e.sources, u = e.kinds;
    for (let e = 0; e < f.length; e++) {
        let o = f[e], c = u[e];
        let s;
        if (c === SOURCE_OMIT) {
            if (o.kind === SOURCE_MERGE) {
                if (n === undefined) n = [ o ]; else n.push(o);
                collectKeys(o.source, n, t, r, i);
                n.pop();
                continue;
            }
            s = o;
            c = o.kind;
            o = o.source;
        }
        o = leafOf(o, c);
        const d = t ? ownEnumerableKeys(o) : leafKeys(o, c);
        for (let e = 0; e < d.length; e++) {
            const t = d[e];
            if (s !== undefined && isHidden(s, t)) continue;
            if (hiddenByAny(n, t)) continue;
            addKey(r, i, t, o);
        }
    }
}

// Append `key` owned by `owner`, moving an earlier listing to the end: later
// wins, and the position is the last owner's (the merged order).
function addKey(e, n, t, r) {
    const i = e.indexOf(t);
    if (i !== -1) {
        e.splice(i, 1);
        if (n !== null) n.splice(i, 1);
    }
    e.push(t);
    if (n !== null) n.push(r);
}

/** @internal Every own string key of a props SOURCE — a plain object, a
 * store or foreign proxy, or a merge/omit view — appended to `keys` in
 * merged order with the object that owns each at the same index of
 * `owners`: a key already listed (by this source or an earlier one) moves
 * to the end, so several sources collected in turn give the order and the
 * winners a merge of them would. A consumer that reads each key once
 * (`ssrElement`) then reads `owners[i][keys[i]]` — the owner's getter runs
 * there, once — and asks nothing else of a view: no table, no `in` walk per
 * key through the merge/omit layers, no key list per leaf. One pass,
 * however deep the layers nest. Symbols are listed; the consumer skips
 * them. */ function sourceOwners(e, n, t) {
    if ($PROXY in e) {
        const r = viewOf(e);
        if (r === undefined) {
            // a store: one `ownKeys` trap, reads through `[]`
            const r = Reflect.ownKeys(e);
            for (let i = 0; i < r.length; i++) addKey(n, t, r[i], e);
            return;
        }
        if (r instanceof OmitView) {
            if (r.kind === SOURCE_MERGE) return collectKeys(r.source, [ r ], false, n, t);
            const e = viewSource(r);
            const i = leafKeys(e, r.kind);
            for (let f = 0; f < i.length; f++) if (!isHidden(r, i[f])) addKey(n, t, i[f], e);
            return;
        }
        return collectKeys(r, undefined, false, n, t);
    }
    const r = Object.keys(e);
    for (let i = 0; i < r.length; i++) addKey(n, t, r[i], e);
}

// The user-facing key set of a merge record, read through `filter` if given.
function mergeEnumerableKeys(e, n) {
    const t = mergeTable(e);
    if (t === undefined) return mergeKeysOf(e, true, n);
    const r = tableOwnKeys(e, t);
    if (n === undefined) return r;
    const i = [];
    for (let e = 0; e < r.length; e++) if (!isHidden(n, r[e])) i.push(r[e]);
    return i;
}

const mergeTraps = {
    get(e, n, t) {
        // The private keys are symbols; a string read (every prop) skips the
        // compares. `$TARGET` is answered so a store check never walks the
        // sources (a leaf that is a store would answer it).
        if (typeof n === "symbol") {
            if (n === $PROXY) return t;
            if (n === $RECORD) return e;
            if (n === $TARGET) return undefined;
        }
        // A trap read counts toward the table (see `mergeReadTable`); the walk
        // itself is the record's. `mergeReadTable` and the plain walk of
        // `mergeLookup` are inlined here: a trap is entered from the runtime, so
        // nothing below it is inlined for it, and a component's props view is a
        // few plain objects read once per key on the server — the walk is the
        // whole read. A leaf that is not plain hands the walk to `mergeGet`,
        // which starts over (a plain leaf walked twice is two `in` checks).
                const r = e.table;
        let i;
        if (typeof r !== "object") {
            if (r + 1 < READS_FOR_TABLE) {
                e.table = r + 1;
                const t = e.sources, i = e.kinds;
                for (let r = t.length - 1; r >= 0; r--) {
                    if (i[r] !== SOURCE_PLAIN) return mergeGet(e, n);
                    // Read first, `in` only to tell a missing key from one holding
                    // undefined: the last source is the one that usually has the key.
                                        const f = t[r][n];
                    if (f !== undefined || n in t[r]) return f;
                }
                return undefined;
            }
            i = mergeTable(e);
            if (i === undefined) return mergeGet(e, n);
        } else if (r === null) return mergeGet(e, n); else i = r;
        const f = i.get(n);
        return f === undefined ? undefined : f[n];
    },
    has(e, n) {
        if (n === $PROXY) return true;
        if (n === $TARGET || n === $RECORD) return false;
        const t = mergeReadTable(e);
        if (t !== undefined) return t.has(n);
        return mergeHas(e, n);
    },
    set: trueFn,
    deleteProperty: trueFn,
    getOwnPropertyDescriptor(e, n) {
        if (n === $PROXY || n === $TARGET || n === $RECORD) return undefined;
        const t = mergeReadTable(e);
        if (t !== undefined) return tableDescriptor(e, t, n);
        return mergeDescriptor(e, n);
    },
    ownKeys(e) {
        return mergeEnumerableKeys(e);
    }
};

// An omit view reads its source directly — a hidden-key check and one
// property read; over a merge record, the merge's own walk by function call,
// never a trap. Once an enumeration or the read count has built its table
// (over a merge with plain leaves only: the merge's, filtered) every trap
// answers from that instead.
const omitTraps = {
    get(e, n, t) {
        if (n === $PROXY) return t;
        // `$RECORD` is THIS view — never the underlying merge's record, which is
        // unfiltered: handing a re-merge the merge's own sources would leak the
        // omitted keys (#3014). A consumer walks the omit record as ONE filtered
        // entry (`recordOf` tells the two records apart by class).
                if (n === $RECORD) return e;
        if (n === $TARGET) return undefined;
        if (e.kind === SOURCE_MERGE) {
            const t = omitReadTable(e);
            if (t !== undefined) {
                const e = t.get(n);
                return e === undefined ? undefined : e[n];
            }
            if (isHidden(e, n)) return undefined;
            return mergeGet(e.source, n);
        }
        if (isHidden(e, n)) return undefined;
        return viewSource(e)[n];
    },
    has(e, n) {
        if (n === $PROXY) return true;
        if (n === $TARGET || n === $RECORD) return false;
        if (e.kind === SOURCE_MERGE) {
            const t = omitReadTable(e);
            if (t !== undefined) return t.has(n);
            if (isHidden(e, n)) return false;
            return mergeHas(e.source, n);
        }
        if (isHidden(e, n)) return false;
        return n in viewSource(e);
    },
    set: trueFn,
    deleteProperty: trueFn,
    getOwnPropertyDescriptor(e, n) {
        if (n === $PROXY || n === $TARGET || n === $RECORD) return undefined;
        if (e.kind === SOURCE_MERGE) {
            const t = omitReadTable(e);
            if (t !== undefined) return tableDescriptor(e, t, n);
        }
        return sourceDescriptor(e, SOURCE_OMIT, n);
    },
    ownKeys(e) {
        if (e.kind === SOURCE_MERGE) {
            const n = omitTable(e);
            if (n !== undefined) return tableOwnKeys(e, n);
            // No table (a store or memo leaf): the merge's own key set, filtered.
                        return sourceEnumerableKeys(e);
        }
        const n = Reflect.ownKeys(viewSource(e));
        const t = [];
        for (let r = 0; r < n.length; r++) if (!isHidden(e, n[r])) t.push(n[r]);
        return t;
    }
};

/** @internal The flattened sources behind a `merge()` proxy, or undefined.
 * A merge's writes are no-ops, so its sources are the whole truth. A COPY of
 * a merge (`{...merged}`, a descriptor copy) is a plain object that carries
 * no sources — `ownKeys` never lists $RECORD — so what is on the copy is
 * the truth there and every consumer reads it directly (#3384). */ function mergeSources(e) {
    const n = mergeView(e);
    return n === undefined ? undefined : n.sources;
}

/**
 * Merges multiple props-like objects into a single proxy that *preserves
 * reactivity*. Reads are forwarded to the right-most source that defines the
 * property, so later sources override earlier ones (like `Object.assign`).
 *
 * Function arguments are treated as memo-backed sources — useful for passing
 * derived defaults whose computation should track reactively.
 *
 * The result is a live VIEW of its sources, never a copy: creating it costs
 * nothing per key, every read goes to the source that owns the key (a getter
 * runs there, a data property is read live), and writing to it is a no-op.
 * A single non-function source is returned as is. To own a mutable object,
 * copy it: `{ ...merged }` snapshots the current values.
 *
 * Use this in component bodies to merge defaults / overrides without losing
 * Solid's per-property tracking.
 *
 * Reading props, here and anywhere: a prop's getter is defined only for a
 * read through its own object (`props.x`, `{ ...props }`, `Reflect.get`,
 * these views). Forwarding its descriptor onto another object and reading it
 * there is not supported — the compiler's server-side props keep their state
 * on the instance, so the getter needs its object as receiver. A copy that
 * must stay live defines its own getter that reads through the source, as
 * the no-Proxy paths of merge() and omit() do.
 *
 * @example
 * ```tsx
 * function Button(_props: { label: string; type?: string; disabled?: boolean }) {
 *   const props = merge({ type: "button", disabled: false }, _props);
 *
 *   return <button type={props.type} disabled={props.disabled}>{props.label}</button>;
 * }
 * ```
 */ function merge(...e) {
    if (e.length === 1 && typeof e[0] !== "function") return e[0];
    // Sized to the argument count up front: a view is built per source per
    // component layer, and growing two empty arrays by `push` was a third of
    // its construction. A nested view's sources may run past the count (the
    // array grows), a falsy source leaves it short (trimmed below).
        const n = new Array(e.length);
    const t = new Array(e.length);
    let r = 0;
    // The one non-falsy source, if there is exactly one: it IS the merge.
        let i = undefined;
    let f = 0;
    for (let u = 0; u < e.length; u++) {
        const o = e[u];
        if (!o) continue;
        f++;
        i = o;
        if (typeof o === "function") {
            // Observe tiers name it and exempt it from UNSTABLE_MEMO_OUTPUT (see
            // CONFIG_KEY_BAG); prod passes no options.
            n[r] = createMemo(o);
            t[r++] = SOURCE_MEMO;
            continue;
        }
        if ($PROXY in o) {
            // A store (or a foreign proxy) is a leaf as it is. A merge() proxy is
            // flattened through: its writes are no-ops, so its sources are exactly
            // what it reads. An omit() proxy joins as its view record — ONE entry,
            // its filter travelling with it, whether it is over a plain object or
            // a whole merge (never the merge's own sources, which would leak the
            // omitted keys, #3014). A consumer's walk recurses into the record.
            const e = recordOf(o);
            if (e instanceof MergeView) {
                for (let i = 0; i < e.sources.length; i++) {
                    n[r] = e.sources[i];
                    t[r++] = e.kinds[i];
                }
            } else if (e !== undefined) {
                n[r] = e;
                t[r++] = SOURCE_OMIT;
            } else {
                n[r] = o;
                t[r++] = SOURCE_PROXY;
            }
            continue;
        }
        n[r] = o;
        t[r++] = SOURCE_PLAIN;
    }
    if (r !== n.length) {
        n.length = r;
        t.length = r;
    }
    if (SUPPORTS_PROXY) {
        if (f === 1 && typeof i !== "function") return i;
        // Always a view, never a copy. Building a plain object here costs a
        // descriptor read, a bound getter and a defineProperty per key per
        // layer, and component libraries stack several layers per element
        // (defaults → omit → call-site statics → …), so the copies dominated
        // their render cost while every consumer that matters — `spread`,
        // `ssrElement`, a nested merge — reads the flattened sources directly
        // anyway (#3448). The view is O(1) to create and reads through to the
        // sources, so a data property on a source is read live, like a getter.
        // Writes to the result are no-ops (a consumer that needs its own object
        // copies: `{...merged}`, which the traps answer truthfully). Copies of
        // the result never carry $RECORD (#3384): `ownKeys` answers only the
        // sources' keys.
                return new Proxy(new MergeView(n, t), mergeTraps);
    }
    // No Proxy: an eager descriptor copy, semantics as close to the view as a
    // plain object allows (getters stay live; data properties are snapshots).
        const u = Object.create(null);
    let o = false;
    let c = n.length - 1;
    for (let e = c; e >= 0; e--) {
        const t = n[e];
        if (!t) {
            e === c && c--;
            continue;
        }
        const r = Object.getOwnPropertyNames(t);
        for (let n = r.length - 1; n >= 0; n--) {
            const i = r[n];
            if (i === "__proto__" || i === "constructor") continue;
            if (!u[i]) {
                o = o || e !== c;
                const n = Object.getOwnPropertyDescriptor(t, i);
                u[i] = n.get ? {
                    enumerable: true,
                    configurable: true,
                    get: n.get.bind(t)
                } : n;
            }
        }
    }
    if (!o) return n[c];
    const s = {};
    const d = Object.keys(u);
    for (let e = d.length - 1; e >= 0; e--) {
        const n = d[e], t = u[n];
        if (t.get) Object.defineProperty(s, n, t); else s[n] = t.value;
    }
    return s;
}

function omit(e, ...n) {
    let t = n.length === 1 && typeof n[0] === "function" ? n[0] : n;
    if (SUPPORTS_PROXY) {
        // A view over a view folds: one record, both filters, the original
        // source — so a consumer walks the real object however deep the omits go.
        // Over a merge() proxy the source is the merge's RECORD (see OmitView):
        // one record whatever the leaf count, read by function call.
        let n = e;
        let r = SOURCE_PLAIN;
        if (typeof e === "function") r = SOURCE_MEMO; else if ($PROXY in e) {
            r = SOURCE_PROXY;
            const i = recordOf(e);
            if (i instanceof OmitView) {
                n = i.source;
                r = i.kind;
                t = combineHidden(i.hidden, t);
            } else if (i !== undefined) {
                n = i;
                r = SOURCE_MERGE;
            }
        }
        return new Proxy(new OmitView(n, r, t), omitTraps);
    }
    const r = {};
    const i = Object.getOwnPropertyNames(e);
    const f = typeof t === "function" ? t : t.length > 4 && i.length > t.length ? (e => n => e.has(n))(new Set(t)) : e => t.includes(e);
    for (const n of i) {
        if (!f(n)) {
            const t = Object.getOwnPropertyDescriptor(e, n);
            if (!t.get && !t.set && t.enumerable && t.writable && t.configurable) {
                r[n] = t.value;
            } else if (t.get || t.set) {
                // An accessor is re-homed with its source as receiver, never copied
                // as-is: a props getter is only defined for a read THROUGH its own
                // object (the compiler's server props keep their state on the
                // instance, so a forwarded descriptor read on the copy throws). Same
                // rule as merge()'s copy path above.
                Object.defineProperty(r, n, {
                    enumerable: t.enumerable,
                    configurable: true,
                    get: t.get && t.get.bind(e),
                    set: t.set && t.set.bind(e)
                });
            } else Object.defineProperty(r, n, t);
        }
    }
    return r;
}

export { MergeView, OmitView, SOURCE_MEMO, SOURCE_MERGE, SOURCE_OMIT, SOURCE_PLAIN, SOURCE_PROXY, hasStaticKeys, isStatic, merge, mergeSources, mergeView, omit, omitView, resolvedTable, sourceGet, sourceHas, sourceKeys, sourceOwners, viewOf };